package config

import (
	"time"
)

// GetSharing to resolve sharing in multi-tenant configuration
func (lp *LocalProgramBlock) GetSharing() *SharingBlock {
	return lp.Sharing
}

func (lp *LocalProgramBlock) SetSharing(sharing *SharingBlock) {
	lp.Sharing = sharing
}

func (dm *DockerMCPBlock) GetSharing() *SharingBlock {
	return dm.Sharing
}

func (dm *DockerMCPBlock) SetSharing(sharing *SharingBlock) {
	dm.Sharing = sharing
}

func (hm *HttpMCPBlock) GetSharing() *SharingBlock {
	return hm.Sharing
}

func (hm *HttpMCPBlock) SetSharing(sharing *SharingBlock) {
	hm.Sharing = sharing
}

// Helper method to check if a tool requires approval
func RequiresApproval(toolType string) bool {
	switch toolType {
	case "local_program", "docker_mcp":
		return true
	case "mcp_over_http":
		return false
	default:
		return true
	}
}

// Helper method to parse expiration time
func (s *SharingBlock) GetExpirationTime() (*time.Time, error) {
	if s.ExpiresAt == "" {
		return nil, nil
	}
	expiration, err := time.Parse(time.RFC3339, s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &expiration, nil
}

// Helper method to check if sharing is allowed
func (s *SharingBlock) CanShareWithUser(userID string, adminUsers []string) bool {
	// Check if user is in allowed users list
	for _, allowed := range s.AllowedUsers {
		if allowed == userID {
			return true
		}
	}

	// Check if user is admin (admins can access all shared tools)
	for _, admin := range adminUsers {
		if admin == userID {
			return true
		}
	}

	return false
}

// MultiTenantBlock describes the configuration for multi-tenant setup in scenarios like Slacker.
type MultiTenantBlock struct {
	AdminUsers        []string     `hcl:"admin_users,optional"`
	AdminChannel      string       `hcl:"admin_channel,optional"`
	SessionStorePath  string       `hcl:"session_store_path,optional"`
	CredentialStore   string       `hcl:"credential_store,optional"`
	SlackerStatePath  string       `hcl:"slacker_state_path,optional"`
	SecurityLogFormat string       `hcl:"security_log_format,optional"`
	ApprovalTimeout   string       `hcl:"approval_timeout,optional"`
	CronJobs          []CronStanza `hcl:"cron,block"`
}
