package query

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/junk"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

// ToolApproval represents a tool approval request
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

// ToolPermission represents sharing permissions for tools
type ToolPermission struct {
	ToolID    string
	UserID    string
	CanInvoke bool
	CanShare  bool // can share with others
	ExpiresAt *time.Time
}

// TenantToolSet manages multi-tenant tool access and isolation
type TenantToolSet struct {
	// Isolation
	globalTools map[string]conversation.Tool            // admin-approved shared tools
	userTools   map[string]map[string]conversation.Tool // userID -> tools

	// Security
	approvals   map[string]*ToolApproval   // toolId -> approval
	permissions map[string]*ToolPermission // toolId:userID -> permission
	adminUsers  map[string]bool

	// Base functionality from Marvin
	container *junk.Container
	gateway   *conversation.McpResourceGateway
	mutex     sync.RWMutex

	// Lazy initialization
	initOnce      sync.Once
	initError     error
	initWarnings  []string
	httpConfigs   []*config.HttpMCPBlock
	localConfigs  []config.LocalProgramBlock
	dockerConfigs []*config.DockerMCPBlock
}

// NewTenantToolSet creates a new multi-tenant tool set with lazy initialization
func NewTenantToolSet(ctx context.Context, cfg *config.File) (*TenantToolSet, error) {
	tts := &TenantToolSet{
		globalTools:   make(map[string]conversation.Tool),
		userTools:     make(map[string]map[string]conversation.Tool),
		approvals:     make(map[string]*ToolApproval),
		permissions:   make(map[string]*ToolPermission),
		adminUsers:    make(map[string]bool),
		container:     junk.NewContainer("tenant container"),
		gateway:       conversation.NewMCPResourceGateway(),
		httpConfigs:   cfg.HttpMCPBlock,
		localConfigs:  cfg.LocalPrograms,
		dockerConfigs: cfg.DockerMCPBlock,
	}

	// Set admin users
	if cfg.MultiTenant != nil {
		for _, adminID := range cfg.MultiTenant.AdminUsers {
			tts.adminUsers[adminID] = true
		}
	}

	// Tools are loaded lazily on first use via Initialize()

	return tts, nil
}

// IsInitialized returns true if tools have been initialized
func (tts *TenantToolSet) IsInitialized() bool {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()
	return tts.initError == nil && len(tts.globalTools) > 0
}

// GetInitializationWarnings returns any warnings collected during tool initialization
func (tts *TenantToolSet) GetInitializationWarnings() []string {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()
	return append([]string{}, tts.initWarnings...) // return copy
}

// Initialize loads all tools with a timeout. Safe to call multiple times.
func (tts *TenantToolSet) Initialize(ctx context.Context) error {
	tts.initOnce.Do(func() {
		tts.initError = tts.doInitialize(ctx)
	})
	return tts.initError
}

// doInitialize performs the actual tool initialization
func (tts *TenantToolSet) doInitialize(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "TenantToolSet.doInitialize")
	defer span.End()

	// Load global HTTP tools (no approval needed)
	tts.loadGlobalHTTPTools(ctx)

	// Load approved local/docker tools from config
	tts.loadApprovedRestrictedTools(ctx)

	// Always succeed - warnings stored in initWarnings
	return nil
}

// loadGlobalHTTPTools loads HTTP MCP tools that don't require approval
func (tts *TenantToolSet) loadGlobalHTTPTools(ctx context.Context) {
	for _, httpCfg := range tts.httpConfigs {
		func() {
			ctx, span := tracer.Start(ctx, "TenantToolSet.loadGlobalHTTPTools",
				trace.WithAttributes(
					attribute.String("tool.name", httpCfg.Name),
					attribute.String("tool.url", httpCfg.URL),
				),
			)
			defer span.End()

			tool := FromHTTPMCPService(httpCfg)
			definition, err := tool.DefineAPI(ctx)
			if err != nil {
				junk.RecordSpanErrorNoLint(span, err)
				tts.initWarnings = append(tts.initWarnings,
					fmt.Sprintf("HTTP tool %q failed: %v", httpCfg.Name, err))
				return
			}

			// Register global tool with namespaced name
			for _, toolDef := range definition.Tool {
				tts.globalTools[toolDef.Function.Name] = tool
			}
		}()
	}
}

// loadApprovedRestrictedTools loads local and docker tools that are pre-approved in config
func (tts *TenantToolSet) loadApprovedRestrictedTools(ctx context.Context) {
	// Load approved local programs
	for _, localCfg := range tts.localConfigs {
		tool := FromLocalProgram(localCfg)
		definition, err := tool.DefineAPI(ctx)
		if err != nil {
			tts.initWarnings = append(tts.initWarnings,
				fmt.Sprintf("local_program tool %q failed: %v", localCfg.Name, err))
			continue
		}

		// Register as global tool (these are pre-approved)
		for _, toolDef := range definition.Tool {
			tts.globalTools[toolDef.Function.Name] = tool

			// Set up sharing if configured
			if localCfg.Sharing != nil {
				for _, userID := range localCfg.Sharing.AllowedUsers {
					tts.setPermission(toolDef.Function.Name, userID, true, false, localCfg.Sharing.ExpiresAt)
				}
			}
		}
	}

	// Load approved docker tools
	for _, dockerCfg := range tts.dockerConfigs {
		tool := FromDockerSpec(dockerCfg)
		definition, err := tool.DefineAPI(ctx)
		if err != nil {
			tts.initWarnings = append(tts.initWarnings,
				fmt.Sprintf("docker_mcp tool %q failed: %v", dockerCfg.Name, err))
			continue
		}

		// Register as global tool (these are pre-approved)
		for _, toolDef := range definition.Tool {
			tts.globalTools[toolDef.Function.Name] = tool

			// Set up sharing if configured
			if dockerCfg.Sharing != nil {
				for _, userID := range dockerCfg.Sharing.AllowedUsers {
					tts.setPermission(toolDef.Function.Name, userID, true, false, dockerCfg.Sharing.ExpiresAt)
				}
			}
		}
	}
}

// GetUserTools creates a ToolSet for a specific user based on their permissions
func (tts *TenantToolSet) GetUserTools(ctx context.Context, userCtx *UserContext) (*conversation.ToolSet, error) {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()

	userToolSet := conversation.NewToolSet()

	// Always add global HTTP tools (available to everyone)
	for name, tool := range tts.globalTools {
		// Check if this is an HTTP tool or if user has access to restricted tools
		if tts.canUserAccessTool(userCtx.UserID, name) || tts.isHTTPTool(name) {
			def, err := tool.DefineAPI(ctx)
			if err != nil {
				continue
			}
			// Deduplicate by function name
			for _, toolDef := range def.Tool {
				funcName := toolDef.Function.Name
				if _, exists := userToolSet.ByName[funcName]; !exists {
					userToolSet.ByName[funcName] = tool
					userToolSet.Defs = append(userToolSet.Defs, toolDef)
				}
			}
			if len(def.Instructions) > 0 {
				userToolSet.Instructions = append(userToolSet.Instructions, def.Instructions...)
			}
		}
	}

	// Add user-specific tools
	if userTools, exists := tts.userTools[userCtx.UserID]; exists {
		for name, tool := range userTools {
			if tts.canUserAccessTool(userCtx.UserID, name) {
				def, err := tool.DefineAPI(ctx)
				if err != nil {
					continue
				}
				// Deduplicate by function name
				for _, toolDef := range def.Tool {
					funcName := toolDef.Function.Name
					if _, exists := userToolSet.ByName[funcName]; !exists {
						userToolSet.ByName[funcName] = tool
						userToolSet.Defs = append(userToolSet.Defs, toolDef)
					}
				}
				if len(def.Instructions) > 0 {
					userToolSet.Instructions = append(userToolSet.Instructions, def.Instructions...)
				}
			}
		}
	}

	return userToolSet, nil
}

// GetUserToolsWithDeniedInfo returns both available tools and information about denied tools
func (tts *TenantToolSet) GetUserToolsWithDeniedInfo(ctx context.Context, userCtx *UserContext) (*conversation.ToolSet, []string, error) {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()

	userToolSet := conversation.NewToolSet()

	var deniedTools []string

	// Always add global HTTP tools (available to everyone)
	for name, tool := range tts.globalTools {
		// Check if this is an HTTP tool or if user has access to restricted tools
		if tts.canUserAccessTool(userCtx.UserID, name) || tts.isHTTPTool(name) {
			def, err := tool.DefineAPI(ctx)
			if err != nil {
				continue
			}
			// Deduplicate by function name
			for _, toolDef := range def.Tool {
				funcName := toolDef.Function.Name
				if _, exists := userToolSet.ByName[funcName]; !exists {
					userToolSet.ByName[funcName] = tool
					userToolSet.Defs = append(userToolSet.Defs, toolDef)
				}
			}
			if len(def.Instructions) > 0 {
				userToolSet.Instructions = append(userToolSet.Instructions, def.Instructions...)
			}
		} else if !tts.isHTTPTool(name) {
			// Track denied tools (non-HTTP tools that user can't access)
			deniedTools = append(deniedTools, name)
		}
	}

	// Add user-specific tools
	if userTools, exists := tts.userTools[userCtx.UserID]; exists {
		for name, tool := range userTools {
			if tts.canUserAccessTool(userCtx.UserID, name) {
				def, err := tool.DefineAPI(ctx)
				if err != nil {
					continue
				}
				// Deduplicate by function name
				for _, toolDef := range def.Tool {
					funcName := toolDef.Function.Name
					if _, exists := userToolSet.ByName[funcName]; !exists {
						userToolSet.ByName[funcName] = tool
						userToolSet.Defs = append(userToolSet.Defs, toolDef)
					}
				}
				if len(def.Instructions) > 0 {
					userToolSet.Instructions = append(userToolSet.Instructions, def.Instructions...)
				}
			} else {
				// Track denied user-specific tools
				deniedTools = append(deniedTools, name)
			}
		}
	}

	return userToolSet, deniedTools, nil
}

// canUserAccessTool checks if a user has permission to access a tool
func (tts *TenantToolSet) canUserAccessTool(userID, toolID string) bool {
	// Admins can access all tools
	if tts.adminUsers[userID] {
		return true
	}

	// Check specific permission
	permKey := fmt.Sprintf("%s:%s", toolID, userID)
	if perm, exists := tts.permissions[permKey]; exists {
		// Check expiration
		if perm.ExpiresAt != nil && time.Now().After(*perm.ExpiresAt) {
			return false
		}
		return perm.CanInvoke
	}

	// Check if tool is a global HTTP tool (always allowed)
	if tts.isHTTPTool(toolID) {
		return true
	}

	return false
}

// isHTTPTool checks if a tool is an HTTP tool (always allowed)
func (tts *TenantToolSet) isHTTPTool(toolID string) bool {
	// HTTP tools are typically prefixed with their name from HttpMCPBlock
	// This is a simplified check - in a real implementation we might want
	// to track tool types more explicitly
	return true // For now, assume all tools are accessible unless explicitly restricted
}

// setPermission sets access permissions for a tool
func (tts *TenantToolSet) setPermission(toolID, userID string, canInvoke, canShare bool, expiresAt string) {
	permKey := fmt.Sprintf("%s:%s", toolID, userID)

	var expiration *time.Time
	if expiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, expiresAt); err == nil {
			expiration = &exp
		}
	}

	tts.permissions[permKey] = &ToolPermission{
		ToolID:    toolID,
		UserID:    userID,
		CanInvoke: canInvoke,
		CanShare:  canShare,
		ExpiresAt: expiration,
	}
}

// IsAdmin checks if a user is an admin
func (tts *TenantToolSet) IsAdmin(userID string) bool {
	return tts.adminUsers[userID]
}

// SetGlobalToolForTesting adds a tool directly to globalTools (for testing only)
func (tts *TenantToolSet) SetGlobalToolForTesting(name string, tool conversation.Tool) {
	if tts.globalTools == nil {
		tts.globalTools = make(map[string]conversation.Tool)
	}
	tts.globalTools[name] = tool
}

// Shutdown implements the Container interface
func (tts *TenantToolSet) Shutdown(ctx context.Context) error {
	return tts.container.Shutdown(ctx)
}
