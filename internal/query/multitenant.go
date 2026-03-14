package query

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/junk"
	"github.com/meschbach/marvin/internal/query/tooling"
	"go.opentelemetry.io/otel/attribute"
)

// ToolInitializationError represents an error during tool initialization
type ToolInitializationError struct {
	ToolName string
	Cause    error
}

func (e ToolInitializationError) Error() string {
	return fmt.Sprintf("initializing tool %s: %v", e.ToolName, e.Cause)
}

func (e ToolInitializationError) Unwrap() error {
	return e.Cause
}

// UserContext represents the context for a specific user in the multi-tenant system
type UserContext struct {
	UserID      string
	SlackTeamID string
	IsAdmin     bool
	Credentials map[string]string // decrypted credentials for this user
}

// ToolApproval represents a tool approval request (kept for external slacker package compatibility)
type ToolApproval struct {
	ToolID      string
	RequesterID string
	ToolType    string         // "local_program", "docker_mcp", "mcp_over_http"
	Config      interface{}    // the actual tool config
	Status      ApprovalStatus // "pending", "approved", "rejected"
	ApprovedBy  string
	Timestamp   time.Time
	Reason      string
}

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
)

type toolFactory struct{}

func (f *toolFactory) CreateHTTPTool(block *config.HttpMCPBlock) conversation.Tool {
	return FromHTTPMCPService(block)
}

func (f *toolFactory) CreateLocalProgramTool(block config.LocalProgramBlock) conversation.Tool {
	return FromLocalProgram(block)
}

func (f *toolFactory) CreateDockerTool(block *config.DockerMCPBlock) conversation.Tool {
	return FromDockerSpec(block)
}

// TenantToolSet manages multi-tenant tool access and isolation
type TenantToolSet struct {
	registry *tooling.Registry
	policy   *tooling.AccessPolicy
	builder  *tooling.Builder

	container *junk.Container
	gateway   *conversation.McpResourceGateway
	mutex     sync.RWMutex

	initOnce     sync.RWMutex
	initError    error
	initWarnings []string

	httpConfigs   []*config.HttpMCPBlock
	localConfigs  []config.LocalProgramBlock
	dockerConfigs []*config.DockerMCPBlock
}

// NewTenantToolSet creates a new multi-tenant tool set with lazy initialization
func NewTenantToolSet(ctx context.Context, cfg *config.File) (*TenantToolSet, error) {
	tts := &TenantToolSet{
		container:     junk.NewContainer("tenant container"),
		gateway:       conversation.NewMCPResourceGateway(),
		httpConfigs:   cfg.HttpMCPBlock,
		localConfigs:  cfg.LocalPrograms,
		dockerConfigs: cfg.DockerMCPBlock,

		registry: tooling.NewRegistry(),
		policy:   tooling.NewAccessPolicy(nil),
		builder:  tooling.NewBuilder(),
	}

	if cfg.MultiTenant != nil {
		tts.policy = tooling.NewAccessPolicy(cfg.MultiTenant.AdminUsers)
	}

	return tts, nil
}

// IsInitialized returns true if tools have been initialized
func (tts *TenantToolSet) IsInitialized() bool {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()
	return tts.initError == nil && tts.registry != nil && len(tts.registry.All()) > 0
}

// GetInitializationWarnings returns any warnings collected during tool initialization
func (tts *TenantToolSet) GetInitializationWarnings() []string {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()
	return append([]string{}, tts.initWarnings...) // return copy
}

// Initialize loads all tools with a timeout. Safe to call multiple times.
func (tts *TenantToolSet) Initialize(ctx context.Context) error {
	tts.initOnce.Lock()
	defer tts.initOnce.Unlock()

	if tts.initError == nil && len(tts.registry.All()) > 0 {
		return nil // already initialized successfully
	}
	if tts.initError != nil {
		tts.initError = nil // allow retry
	}

	tts.initError = tts.doInitialize(ctx)
	return tts.initError
}

// doInitialize performs the actual tool initialization
func (tts *TenantToolSet) doInitialize(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "TenantToolSet.doInitialize")
	defer span.End()

	loader := tooling.NewLoader(tts.container, &toolFactory{})
	cfg := &config.File{
		HttpMCPBlock:   tts.getHttpConfigs(),
		LocalPrograms:  tts.getLocalConfigs(),
		DockerMCPBlock: tts.getDockerConfigs(),
	}

	reg, warnings, err := loader.LoadTools(ctx, cfg)
	if err != nil {
		junk.RecordSpanErrorNoLint(span, err)
		return err
	}

	tts.registry = reg
	tts.initWarnings = warnings

	span.SetAttributes(attribute.Int("init.warnings.count", len(warnings)))
	return nil
}

func (tts *TenantToolSet) getHttpConfigs() []*config.HttpMCPBlock {
	return tts.httpConfigs
}

func (tts *TenantToolSet) getLocalConfigs() []config.LocalProgramBlock {
	return tts.localConfigs
}

func (tts *TenantToolSet) getDockerConfigs() []*config.DockerMCPBlock {
	return tts.dockerConfigs
}

// GetUserTools creates a ToolSet for a specific user based on their permissions
func (tts *TenantToolSet) GetUserTools(ctx context.Context, userCtx *UserContext) (*conversation.ToolSet, error) {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()

	userInfo := &tooling.UserInfo{UserID: userCtx.UserID}
	return tts.builder.Build(ctx, userInfo, tts.registry, tts.policy)
}

// GetUserToolsWithDeniedInfo returns both available tools and information about denied tools
func (tts *TenantToolSet) GetUserToolsWithDeniedInfo(ctx context.Context, userCtx *UserContext) (*conversation.ToolSet, []string, error) {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()

	userInfo := &tooling.UserInfo{UserID: userCtx.UserID}
	return tts.builder.BuildWithDeniedInfo(ctx, userInfo, tts.registry, tts.policy)
}

// IsAdmin checks if a user is an admin
func (tts *TenantToolSet) IsAdmin(userID string) bool {
	return tts.policy.IsAdmin(userID)
}

// SetGlobalToolForTesting adds a tool directly to registry (for testing only)
func (tts *TenantToolSet) SetGlobalToolForTesting(name string, tool conversation.Tool) {
	tts.registry.RegisterToolDef(context.Background(), tool, name, nil)
}

// InjectToolForTesting adds a tool with access restrictions (for testing only)
func (tts *TenantToolSet) InjectToolForTesting(name string, tool conversation.Tool, allowedUsers []string) {
	tts.registry.RegisterToolDef(context.Background(), tool, name, allowedUsers)
}

// SetAdminUsersForTesting sets admin users (for testing only)
func (tts *TenantToolSet) SetAdminUsersForTesting(adminIDs []string) {
	tts.policy = tooling.NewAccessPolicy(adminIDs)
}

// Shutdown implements the Container interface
func (tts *TenantToolSet) Shutdown(ctx context.Context) error {
	return tts.container.Shutdown(ctx)
}
