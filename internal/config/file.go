package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/meschbach/go-junk-bucket/pkg"
	"github.com/meschbach/go-junk-bucket/pkg/observability"
)

const DefaultLanguageModel = "ministral-3:3b"
const DefaultEmbeddingModel = "mxbai-embed-large:latest"

// ProviderType defines the type of LLM provider
type ProviderType string

const (
	ProviderOllama     ProviderType = "ollama"
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderGemini     ProviderType = "gemini"
)

// GeminiBlock contains configuration for Google Gemini provider
type GeminiBlock struct {
	APIKeyBlock `hcl:",remain"`
}

func (g *GeminiBlock) ResolveKey() (value string, has bool, problem error) {
	var keyBlock *APIKeyBlock
	if g != nil {
		keyBlock = &g.APIKeyBlock
	}
	return keyBlock.Resolve("GEMINI_API_KEY")
}

// OpenRouterBlock contains configuration for OpenRouter provider
type OpenRouterBlock struct {
	APIKeyBlock `hcl:",remain"`
	// BaseURL allows overriding the default OpenRouter endpoint
	BaseURL string `hcl:"base_url,optional"`
	// Retry contains retry configuration for OpenRouter requests
	Retry *RetryBlock `hcl:"retry,block"`
}

func (o *OpenRouterBlock) ResolveKey() (value string, has bool, problem error) {
	var keyBlock *APIKeyBlock
	if o != nil {
		keyBlock = &o.APIKeyBlock
	}
	return keyBlock.Resolve("OPENROUTER_API_KEY")
}

const DefaultMaxRetries = 3
const DefaultInitialInterval = 1 * time.Second
const DefaultMaxInterval = 30 * time.Second

type RetryBlock struct {
	MaxAttempts     *int           `hcl:"max_attempts,optional"`
	InitialInterval *time.Duration `hcl:"initial_interval,optional"`
	MaxInterval     *time.Duration `hcl:"max_interval,optional"`
}

func (r *RetryBlock) MaxAttemptsValue() (int, error) {
	if r != nil && r.MaxAttempts != nil {
		if *r.MaxAttempts < 1 {
			return 0, fmt.Errorf("max_attempts must be >= 1, got %d", *r.MaxAttempts)
		}
		return *r.MaxAttempts, nil
	}
	return DefaultMaxRetries, nil
}

func (r *RetryBlock) InitialIntervalValue() (time.Duration, error) {
	if r != nil && r.InitialInterval != nil {
		if *r.InitialInterval <= 0 {
			return 0, fmt.Errorf("initial_interval must be > 0, got %s", *r.InitialInterval)
		}
		return *r.InitialInterval, nil
	}
	return DefaultInitialInterval, nil
}

func (r *RetryBlock) MaxIntervalValue() (time.Duration, error) {
	if r != nil && r.MaxInterval != nil {
		if *r.MaxInterval <= 0 {
			return 0, fmt.Errorf("max_interval must be > 0, got %s", *r.MaxInterval)
		}
		initial, err := r.InitialIntervalValue()
		if err != nil {
			return 0, err
		}
		if *r.MaxInterval < initial {
			return 0, fmt.Errorf("max_interval (%s) must be >= initial_interval (%s)", *r.MaxInterval, initial)
		}
		return *r.MaxInterval, nil
	}
	return DefaultMaxInterval, nil
}

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

// DisplayBlock contains configuration for output display preferences
type DisplayBlock struct {
	// Show model thinking process
	ShowThinking *bool `hcl:"show_thinking,optional"`
	// Show tool invocation details
	ShowTools *bool `hcl:"show_tools,optional"`
	// Show completion messages
	ShowDone *bool `hcl:"show_done,optional"`
	// Show verbose debugging
	Verbose *bool `hcl:"verbose,optional"`
	// Thinking display format ("plain", "markdown", "collapsed")
	ThinkingFormat *string `hcl:"thinking_format,optional"`
	// Tool display format ("simple", "detailed", "json")
	ToolFormat *string `hcl:"tool_format,optional"`
}

// File represents a parsed configuration file
type File struct {
	// Model is the large language model to use
	Model string `hcl:"model,optional"`
	// ProviderName specifies which LLM provider to use (ollama, openrouter, or gemini)
	ProviderName string `hcl:"provider,optional"`
	// OpenRouter contains OpenRouter-specific configuration
	OpenRouter *OpenRouterBlock `hcl:"openrouter,block"`
	// Gemini contains Google Gemini-specific configuration
	Gemini        *GeminiBlock        `hcl:"gemini,block"`
	Options       *ModelOptionsBlock  `hcl:"options,block"`
	LocalPrograms []LocalProgramBlock `hcl:"local_program,block"`
	SystemPrompt  *SystemPromptBlock  `hcl:"system_prompt,block"`
	// Documents represents blocks fo contextual documents to manage
	Documents      []*DocumentsBlock `hcl:"documents,block"`
	DockerMCPBlock []*DockerMCPBlock `hcl:"docker_mcp,block"`
	HttpMCPBlock   []*HttpMCPBlock   `hcl:"mcp_over_http,block"`
	MultiTenant    *MultiTenantBlock `hcl:"multi_tenant,block"`
	ModelAccess    *ModelAccessBlock `hcl:"model_access,block"`
	// Display preferences for output formatting
	Display *DisplayBlock `hcl:"display,block"`
	// Observability configuration for OTEL tracing
	Observability *ObservabilityBlock `hcl:"observability,block"`
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

// Provider returns the LLM provider type (defaults to "ollama")
func (f *File) Provider() ProviderType {
	if f.ProviderName == "" {
		return ProviderOllama
	}
	return ProviderType(f.ProviderName)
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

type AssistantPromptBlock struct {
	FromString string `hcl:"from_string,optional"`
	FromFile   string `hcl:"from_file,optional"`
}

// ObservabilityBlock contains configuration for OTEL tracing
type ObservabilityBlock struct {
	// Exporter is the OTEL exporter to use (none, stdout, grpc)
	Exporter string `hcl:"exporter,optional"`
	// ServiceName is the service name for tracing
	ServiceName string `hcl:"service_name,optional"`
	// Environment is the deployment environment
	Environment string `hcl:"environment,optional"`
	// Batched enables batched span exporting (default: true)
	Batched *bool `hcl:"batched,optional"`
	// Silent suppresses startup configuration output
	Silent *bool `hcl:"silent,optional"`
}

// HasObservabilityEnvVars returns true if any OTEL environment variables are set
func (f *File) HasObservabilityEnvVars() bool {
	otelVars := []string{
		"OTEL_EXPORTER",
		"OTEL_SERVICE_NAME",
		"OTEL_ENDPOINT",
		"OTEL_AUTH",
		"ENV",
	}
	for _, v := range otelVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

// ToObservabilityConfig converts ObservabilityBlock to observability.Config with env var overrides
func (o *ObservabilityBlock) ToObservabilityConfig() observability.Config {
	cfg := observability.DefaultConfig("marvin")

	if o.Exporter != "" {
		cfg.Exporter = o.Exporter
	}
	cfg.Exporter = pkg.EnvOrDefault("OTEL_EXPORTER", cfg.Exporter)

	if o.ServiceName != "" {
		cfg.ServiceName = o.ServiceName
	}
	cfg.ServiceName = pkg.EnvOrDefault("OTEL_SERVICE_NAME", cfg.ServiceName)

	if o.Environment != "" {
		cfg.Environment = o.Environment
	}
	cfg.Environment = pkg.EnvOrDefault("ENV", cfg.Environment)

	if o.Batched != nil {
		cfg.Batched = *o.Batched
	}

	if o.Silent != nil {
		cfg.Silent = *o.Silent
	}

	return cfg
}

// ModelAccessBlock contains model access control configuration
type ModelAccessBlock struct {
	AllowedModels []string `hcl:"allowed_models,optional"`
	DeniedModels  []string `hcl:"denied_models,optional"`
}

// ModelAccessState represents the runtime state for model access control
type ModelAccessState struct {
	AllowedModels []string `json:"allowed_models"`
	DeniedModels  []string `json:"denied_models"`
	DefaultModel  string   `json:"default_model"`
	LastUpdated   string   `json:"last_updated"`
	UpdatedBy     string   `json:"updated_by"`
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
// todo: cleanup
// nolint
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

// ShowThinking returns whether thinking should be displayed
func (f *File) ShowThinking() bool {
	if f.Display != nil && f.Display.ShowThinking != nil {
		return *f.Display.ShowThinking
	}
	return false // default: disabled for new installations
}

// ShowTools returns whether tool invocation details should be displayed
func (f *File) ShowTools() bool {
	if f.Display != nil && f.Display.ShowTools != nil {
		return *f.Display.ShowTools
	}
	return true // default: enabled
}

// ShowDone returns whether completion messages should be displayed
func (f *File) ShowDone() bool {
	if f.Display != nil && f.Display.ShowDone != nil {
		return *f.Display.ShowDone
	}
	return true // default: enabled
}

// Verbose returns whether verbose debugging should be displayed
func (f *File) Verbose() bool {
	if f.Display != nil && f.Display.Verbose != nil {
		return *f.Display.Verbose
	}
	return false // default: disabled
}

// ThinkingFormat returns the format for displaying thinking content
func (f *File) ThinkingFormat() string {
	if f.Display != nil && f.Display.ThinkingFormat != nil {
		return *f.Display.ThinkingFormat
	}
	return "plain" // default: plain text
}

// ToolFormat returns the format for displaying tool invocation details
func (f *File) ToolFormat() string {
	if f.Display != nil && f.Display.ToolFormat != nil {
		return *f.Display.ToolFormat
	}
	return "detailed" // default: detailed format
}

// LoadModelAccessState loads model access state from the slacker-state directory
func (f *File) LoadModelAccessState() (*ModelAccessState, error) {
	if f.MultiTenant == nil || f.MultiTenant.SlackerStatePath == "" {
		return nil, nil
	}

	stateFile := filepath.Join(f.MultiTenant.SlackerStatePath, "model-access.json")

	//nolint:gosec
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No state file exists
		}
		return nil, err
	}

	var state ModelAccessState
	err = json.Unmarshal(data, &state)
	if err != nil {
		return nil, err
	}

	return &state, nil
}

// SaveModelAccessState saves model access state to the slacker-state directory
func (f *File) SaveModelAccessState(state *ModelAccessState, updatedBy string) error {
	if f.MultiTenant == nil || f.MultiTenant.SlackerStatePath == "" {
		return fmt.Errorf("slacker state path not configured")
	}

	// Ensure directory exists
	if err := os.MkdirAll(f.MultiTenant.SlackerStatePath, 0700); err != nil {
		return err
	}

	// Update metadata
	state.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	state.UpdatedBy = updatedBy

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	stateFile := filepath.Join(f.MultiTenant.SlackerStatePath, "model-access.json")

	// Write to temporary file first, then rename for atomicity
	tempFile := stateFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return err
	}

	return os.Rename(tempFile, stateFile)
}

// ValidateModelAccess checks if a model is allowed for Slacker operations.
// For CLI operations, this should not be called.
// todo: cleanup
// nolint
func (f *File) ValidateModelAccess(model string, userID string) (bool, string) {
	// Check if user is admin - admins bypass all restrictions
	if f.MultiTenant != nil {
		for _, admin := range f.MultiTenant.AdminUsers {
			if admin == userID {
				return true, ""
			}
		}
	}

	// Load state configuration (takes priority over HCL)
	state, err := f.LoadModelAccessState()
	if err != nil {
		return false, fmt.Sprintf("Failed to load model access state: %v", err)
	}

	// If no state and no HCL config, allow all models
	if state == nil && f.ModelAccess == nil {
		return true, ""
	}

	// Get effective configuration - state overrides HCL
	var allowedModels, deniedModels []string

	if state != nil {
		allowedModels = state.AllowedModels
		deniedModels = state.DeniedModels
	} else if f.ModelAccess != nil {
		allowedModels = f.ModelAccess.AllowedModels
		deniedModels = f.ModelAccess.DeniedModels
	}

	// Default model is always allowed
	if model == DefaultLanguageModel {
		return true, ""
	}

	// Check deny list first (takes priority)
	for _, denied := range deniedModels {
		if denied == model {
			return false, fmt.Sprintf("Model '%s' is denied by access policy", model)
		}
	}

	// If allow list is not empty, model must be in it
	if len(allowedModels) > 0 {
		for _, allowed := range allowedModels {
			if allowed == model {
				return true, ""
			}
		}
		return false, fmt.Sprintf("Model '%s' is not in allowed list", model)
	}

	// No restrictions - allow
	return true, ""
}

// GetEffectiveModelAccess returns the effective model access configuration
// (state overrides HCL, with proper fallback)
func (f *File) GetEffectiveModelAccess() (*ModelAccessState, error) {
	// Try to load state first
	state, err := f.LoadModelAccessState()
	if err != nil {
		return nil, err
	}

	if state != nil {
		return state, nil
	}

	// Fall back to HCL configuration
	if f.ModelAccess != nil {
		return &ModelAccessState{
			AllowedModels: f.ModelAccess.AllowedModels,
			DeniedModels:  f.ModelAccess.DeniedModels,
			DefaultModel:  DefaultLanguageModel,
			LastUpdated:   "",
			UpdatedBy:     "",
		}, nil
	}

	// No configuration - return empty state (allow all)
	return &ModelAccessState{
		AllowedModels: []string{},
		DeniedModels:  []string{},
		DefaultModel:  DefaultLanguageModel,
		LastUpdated:   "",
		UpdatedBy:     "",
	}, nil
}
