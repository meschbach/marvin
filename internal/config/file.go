package config

import (
	"context"
	"fmt"
	"path/filepath"
)

const DefaultLanguageModel = "ministral-3:3b"
const DefaultEmbeddingModel = "mxbai-embed-large:latest"

// File represents a parsed configuration file
type File struct {
	// Model is the large language model to use
	Model         string              `hcl:"model,optional"`
	LocalPrograms []LocalProgramBlock `hcl:"local_program,block"`
	SystemPrompt  *SystemPromptBlock  `hcl:"system_prompt,block"`
	// Documents represents blocks fo contextual documents to manage
	Documents      []*DocumentsBlock `hcl:"documents,block"`
	DockerMCPBlock []*DockerMCPBlock `hcl:"docker_mcp,block"`
	HttpMCPBlock   []*HttpMCPBlock   `hcl:"mcp_over_http,block"`
	MultiTenant    *MultiTenantBlock `hcl:"multi_tenant,block"`
}

func (f *File) resolveWorkingDirectory(marvinFilePath string) (string, error) {
	relativeHoldingDirectory := filepath.Dir(marvinFilePath)
	workingDirectory, err := filepath.Abs(relativeHoldingDirectory)
	if err != nil {
		return "", err
	}
	for _, block := range f.DockerMCPBlock {
		block.EnsureWorkingDirectory(workingDirectory)
	}
	return workingDirectory, nil
}

// LanguageModel returns the language model to use for this configuration or the default if one is not set
func (f *File) LanguageModel() string {
	model := f.Model
	if model != "" {
		return model
	}
	return DefaultLanguageModel
}

func (f *File) QueryRAGDocuments(ctx context.Context, storeName, query string) ([]QueryResult, error) {
	var documentBlock *DocumentsBlock
	for _, doc := range f.Documents {
		if doc.Name == storeName {
			documentBlock = doc
		}
	}
	if documentBlock == nil {
		return nil, fmt.Errorf("no documents block with name %q", storeName)
	}
	result, err := documentBlock.Query(ctx, query)
	return result, err
}

type SystemPromptBlock struct {
	FromString string `hcl:"from_string,optional"`
	FromFile   string `hcl:"from_file,optional"`
}

type MultiTenantBlock struct {
	AdminUsers        []string `hcl:"admin_users,optional"`
	AdminChannel      string   `hcl:"admin_channel,optional"`
	SessionStorePath  string   `hcl:"session_store_path,optional"`
	CredentialStore   string   `hcl:"credential_store,optional"`
	SecurityLogFormat string   `hcl:"security_log_format,optional"`
	ApprovalTimeout   string   `hcl:"approval_timeout,optional"`
}

type SharingBlock struct {
	AllowedUsers      []string `hcl:"allowed_users,optional"`
	AllowedTeams      []string `hcl:"allowed_teams,optional"`
	CanShare          bool     `hcl:"can_share,optional"`
	ExpiresAt         string   `hcl:"expires_at,optional"`
	AutoApproveShares bool     `hcl:"auto_approve_shares,optional"`
}
