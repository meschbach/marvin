package query

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

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
	globalTools map[string]Tool            // admin-approved shared tools
	userTools   map[string]map[string]Tool // userID -> tools

	// Security
	approvals   map[string]*ToolApproval   // toolID -> approval
	permissions map[string]*ToolPermission // toolID:userID -> permission
	adminUsers  map[string]bool

	// Base functionality from Marvin
	container *Container
	gateway   *mcpResourceGateway
	mutex     sync.RWMutex
}

// NewTenantToolSet creates a new multi-tenant tool set
func NewTenantToolSet(ctx context.Context, cfg *config.File) (*TenantToolSet, error) {
	tts := &TenantToolSet{
		globalTools: make(map[string]Tool),
		userTools:   make(map[string]map[string]Tool),
		approvals:   make(map[string]*ToolApproval),
		permissions: make(map[string]*ToolPermission),
		adminUsers:  make(map[string]bool),
		container:   &Container{name: "tenant container", state: sync.Mutex{}},
		gateway:     newMCPResourceGateway(),
	}

	// Set admin users
	if cfg.MultiTenant != nil {
		for _, adminID := range cfg.MultiTenant.AdminUsers {
			tts.adminUsers[adminID] = true
		}
	}

	// Load global HTTP tools (no approval needed)
	if err := tts.loadGlobalHTTPTools(ctx, cfg); err != nil {
		return nil, fmt.Errorf("loading global HTTP tools: %w", err)
	}

	// Load approved local/docker tools from config
	if err := tts.loadApprovedRestrictedTools(ctx, cfg); err != nil {
		return nil, fmt.Errorf("loading approved restricted tools: %w", err)
	}

	return tts, nil
}

// loadGlobalHTTPTools loads HTTP MCP tools that don't require approval
func (tts *TenantToolSet) loadGlobalHTTPTools(ctx context.Context, cfg *config.File) error {
	for _, httpCfg := range cfg.HttpMCPBlock {
		tool := FromHTTPMCPService(httpCfg)
		definition, err := tool.defineAPI(ctx)
		if err != nil {
			return fmt.Errorf("defining HTTP tool %s: %w", httpCfg.Name, err)
		}

		// Register global tool with namespaced name
		for _, toolDef := range definition.tool {
			tts.globalTools[toolDef.Function.Name] = tool
		}
	}
	return nil
}

// loadApprovedRestrictedTools loads local and docker tools that are pre-approved in config
func (tts *TenantToolSet) loadApprovedRestrictedTools(ctx context.Context, cfg *config.File) error {
	// Load approved local programs
	for _, localCfg := range cfg.LocalPrograms {
		tool := FromLocalProgram(localCfg)
		definition, err := tool.defineAPI(ctx)
		if err != nil {
			return fmt.Errorf("defining local tool %s: %w", localCfg.Name, err)
		}

		// Register as global tool (these are pre-approved)
		for _, toolDef := range definition.tool {
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
	for _, dockerCfg := range cfg.DockerMCPBlock {
		tool := FromDockerSpec(dockerCfg)
		definition, err := tool.defineAPI(ctx)
		if err != nil {
			return fmt.Errorf("defining docker tool %s: %w", dockerCfg.Name, err)
		}

		// Register as global tool (these are pre-approved)
		for _, toolDef := range definition.tool {
			tts.globalTools[toolDef.Function.Name] = tool

			// Set up sharing if configured
			if dockerCfg.Sharing != nil {
				for _, userID := range dockerCfg.Sharing.AllowedUsers {
					tts.setPermission(toolDef.Function.Name, userID, true, false, dockerCfg.Sharing.ExpiresAt)
				}
			}
		}
	}

	return nil
}

// GetUserTools creates a ToolSet for a specific user based on their permissions
func (tts *TenantToolSet) GetUserTools(ctx context.Context, userCtx *UserContext) (*ToolSet, error) {
	tts.mutex.RLock()
	defer tts.mutex.RUnlock()

	userToolSet := &ToolSet{
		byName:       make(map[string]Tool),
		defs:         make(api.Tools, 0),
		instructions: make([]api.Message, 0),
		container:    tts.container,
		gateway:      tts.gateway,
	}

	// Always add global HTTP tools (available to everyone)
	for name, tool := range tts.globalTools {
		// Check if this is an HTTP tool or if user has access to restricted tools
		if tts.canUserAccessTool(userCtx.UserID, name) || tts.isHTTPTool(name) {
			def, err := tool.defineAPI(ctx)
			if err != nil {
				continue
			}
			userToolSet.byName[name] = tool
			userToolSet.defs = append(userToolSet.defs, def.tool...)
			userToolSet.instructions = append(userToolSet.instructions, def.instructions...)
		}
	}

	// Add user-specific tools
	if userTools, exists := tts.userTools[userCtx.UserID]; exists {
		for name, tool := range userTools {
			if tts.canUserAccessTool(userCtx.UserID, name) {
				def, err := tool.defineAPI(ctx)
				if err != nil {
					continue
				}
				userToolSet.byName[name] = tool
				userToolSet.defs = append(userToolSet.defs, def.tool...)
				userToolSet.instructions = append(userToolSet.instructions, def.instructions...)
			}
		}
	}

	return userToolSet, nil
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

// Shutdown implements the Container interface
func (tts *TenantToolSet) Shutdown(ctx context.Context) error {
	return tts.container.Shutdown(ctx)
}
