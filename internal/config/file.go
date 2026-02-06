package config

import (
	"context"
	"fmt"
	"path/filepath"
)

const DefaultLanguageModel = "ministral-3:3b"
const DefaultEmbeddingModel = "mxbai-embed-large:latest"

// ModelOptionsBlock contains advanced model configuration options
type ModelOptionsBlock struct {
	// Context window size in tokens (maps to num_ctx)
	ContextWindowSize *int `hcl:"context_window_size,optional"`
	// Sampling temperature (0.0-1.0, higher = more creative)
	Temperature *float32 `hcl:"temperature,optional"`
	// Nucleus sampling parameter (0.0-1.0)
	TopP *float32 `hcl:"top_p,optional"`
	// Top-k sampling parameter (-1 = no limit)
	TopK *int `hcl:"top_k,optional"`
	// Maximum number of tokens to predict (-1 = unlimited)
	NumPredict *int `hcl:"num_predict,optional"`
	// Repetition penalty to discourage repetitive text
	RepeatPenalty *float32 `hcl:"repeat_penalty,optional"`
	// How far back to look for repetitions (0 = disabled, -1 = context_size)
	RepeatLastN *int `hcl:"repeat_last_n,optional"`
	// Random seed for reproducible results
	Seed *int `hcl:"seed,optional"`
	// Stop sequences to end generation
	Stop []string `hcl:"stop,optional"`
}

// File represents a parsed configuration file
type File struct {
	// Model is the large language model to use
	Model         string              `hcl:"model,optional"`
	Options       *ModelOptionsBlock  `hcl:"options,block"`
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

// BuildAPIOptions constructs a map for api.ChatRequest.Options from the configuration.
// Only includes options that are explicitly set (non-nil), allowing
// Ollama to use its built-in defaults for unspecified options.
func (f *File) BuildAPIOptions() map[string]any {
	if f.Options == nil {
		return nil
	}

	opts := make(map[string]any)
	hasAnyOption := false

	if f.Options.ContextWindowSize != nil {
		opts["num_ctx"] = *f.Options.ContextWindowSize
		hasAnyOption = true
	}
	if f.Options.Temperature != nil {
		opts["temperature"] = *f.Options.Temperature
		hasAnyOption = true
	}
	if f.Options.TopP != nil {
		opts["top_p"] = *f.Options.TopP
		hasAnyOption = true
	}
	if f.Options.TopK != nil {
		opts["top_k"] = *f.Options.TopK
		hasAnyOption = true
	}
	if f.Options.NumPredict != nil {
		opts["num_predict"] = *f.Options.NumPredict
		hasAnyOption = true
	}
	if f.Options.RepeatPenalty != nil {
		opts["repeat_penalty"] = *f.Options.RepeatPenalty
		hasAnyOption = true
	}
	if f.Options.RepeatLastN != nil {
		opts["repeat_last_n"] = *f.Options.RepeatLastN
		hasAnyOption = true
	}
	if f.Options.Seed != nil {
		opts["seed"] = *f.Options.Seed
		hasAnyOption = true
	}
	if len(f.Options.Stop) > 0 {
		opts["stop"] = f.Options.Stop
		hasAnyOption = true
	}

	if !hasAnyOption {
		return nil
	}

	return opts
}
