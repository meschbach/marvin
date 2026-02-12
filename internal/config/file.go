package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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

// HelpSystemBlock contains configuration for the intelligent help system
type HelpSystemBlock struct {
	// Enable the help system (default: true)
	Enabled *bool `hcl:"enabled,optional"`
	// Model to use for help analysis (defaults to main model)
	Model *string `hcl:"model,optional"`
	// Maximum number of previous messages to analyze for context
	MaxContextMessages *int `hcl:"max_context_messages,optional"`
	// Maximum time to wait for help analysis (in seconds)
	AnalysisTimeout *int `hcl:"analysis_timeout,optional"`
	// Minimum confidence threshold for offering help (0.0-1.0)
	MinConfidenceThreshold *float32 `hcl:"min_confidence_threshold,optional"`
	// Whether to offer help for intent failures
	HelpOnIntentFailure *bool `hcl:"help_on_intent_failure,optional"`
	// Whether to offer help for model access denials
	HelpOnModelAccessDenied *bool `hcl:"help_on_model_access_denied,optional"`
	// Whether to offer help for tool configuration errors
	HelpOnToolConfigurationError *bool `hcl:"help_on_tool_configuration_error,optional"`
	// Whether to offer help for tool permission denials
	HelpOnToolPermissionDenied *bool `hcl:"help_on_tool_permission_denied,optional"`
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
	Model         string              `hcl:"model,optional"`
	Options       *ModelOptionsBlock  `hcl:"options,block"`
	LocalPrograms []LocalProgramBlock `hcl:"local_program,block"`
	SystemPrompt  *SystemPromptBlock  `hcl:"system_prompt,block"`
	// Documents represents blocks fo contextual documents to manage
	Documents      []*DocumentsBlock `hcl:"documents,block"`
	DockerMCPBlock []*DockerMCPBlock `hcl:"docker_mcp,block"`
	HttpMCPBlock   []*HttpMCPBlock   `hcl:"mcp_over_http,block"`
	MultiTenant    *MultiTenantBlock `hcl:"multi_tenant,block"`
	ModelAccess    *ModelAccessBlock `hcl:"model_access,block"`
	// Help system configuration for intelligent assistance
	HelpSystem *HelpSystemBlock `hcl:"help_system,block"`
	// Display preferences for output formatting
	Display *DisplayBlock `hcl:"display,block"`
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

type AssistantPromptBlock struct {
	FromString string `hcl:"from_string,optional"`
	FromFile   string `hcl:"from_file,optional"`
}

type MultiTenantBlock struct {
	AdminUsers        []string `hcl:"admin_users,optional"`
	AdminChannel      string   `hcl:"admin_channel,optional"`
	SessionStorePath  string   `hcl:"session_store_path,optional"`
	CredentialStore   string   `hcl:"credential_store,optional"`
	SlackerStatePath  string   `hcl:"slacker_state_path,optional"`
	SecurityLogFormat string   `hcl:"security_log_format,optional"`
	ApprovalTimeout   string   `hcl:"approval_timeout,optional"`
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

// HelpSystemEnabled returns whether the help system is enabled
func (f *File) HelpSystemEnabled() bool {
	if f.HelpSystem != nil && f.HelpSystem.Enabled != nil {
		return *f.HelpSystem.Enabled
	}
	return true // default: enabled
}

// HelpSystemModel returns the model to use for help analysis
func (f *File) HelpSystemModel() string {
	if f.HelpSystem != nil && f.HelpSystem.Model != nil {
		return *f.HelpSystem.Model
	}
	return f.LanguageModel() // default: use main model
}

// HelpSystemMaxContextMessages returns the maximum number of previous messages to analyze
func (f *File) HelpSystemMaxContextMessages() int {
	if f.HelpSystem != nil && f.HelpSystem.MaxContextMessages != nil {
		return *f.HelpSystem.MaxContextMessages
	}
	return 10 // default: analyze last 10 messages
}

// HelpSystemAnalysisTimeout returns the analysis timeout in seconds
func (f *File) HelpSystemAnalysisTimeout() time.Duration {
	if f.HelpSystem != nil && f.HelpSystem.AnalysisTimeout != nil {
		return time.Duration(*f.HelpSystem.AnalysisTimeout) * time.Second
	}
	return 30 * time.Second // default: 30 seconds
}

// HelpSystemMinConfidenceThreshold returns the minimum confidence threshold
func (f *File) HelpSystemMinConfidenceThreshold() float32 {
	if f.HelpSystem != nil && f.HelpSystem.MinConfidenceThreshold != nil {
		return *f.HelpSystem.MinConfidenceThreshold
	}
	return 0.7 // default: 70% confidence
}

// HelpSystemShouldHelpOnIntentFailure returns whether to help on intent failures
func (f *File) HelpSystemShouldHelpOnIntentFailure() bool {
	if f.HelpSystem != nil && f.HelpSystem.HelpOnIntentFailure != nil {
		return *f.HelpSystem.HelpOnIntentFailure
	}
	return true // default: enabled
}

// HelpSystemShouldHelpOnModelAccessDenied returns whether to help on model access denials
func (f *File) HelpSystemShouldHelpOnModelAccessDenied() bool {
	if f.HelpSystem != nil && f.HelpSystem.HelpOnModelAccessDenied != nil {
		return *f.HelpSystem.HelpOnModelAccessDenied
	}
	return true // default: enabled
}

// HelpSystemShouldHelpOnToolConfigurationError returns whether to help on tool configuration errors
func (f *File) HelpSystemShouldHelpOnToolConfigurationError() bool {
	if f.HelpSystem != nil && f.HelpSystem.HelpOnToolConfigurationError != nil {
		return *f.HelpSystem.HelpOnToolConfigurationError
	}
	return true // default: enabled
}

// HelpSystemShouldHelpOnToolPermissionDenied returns whether to help on tool permission denials
func (f *File) HelpSystemShouldHelpOnToolPermissionDenied() bool {
	if f.HelpSystem != nil && f.HelpSystem.HelpOnToolPermissionDenied != nil {
		return *f.HelpSystem.HelpOnToolPermissionDenied
	}
	return true // default: enabled
}

// LoadModelAccessState loads model access state from the slacker-state directory
func (f *File) LoadModelAccessState() (*ModelAccessState, error) {
	if f.MultiTenant == nil || f.MultiTenant.SlackerStatePath == "" {
		return nil, nil
	}

	stateFile := filepath.Join(f.MultiTenant.SlackerStatePath, "model-access.json")

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
	if err := os.MkdirAll(f.MultiTenant.SlackerStatePath, 0755); err != nil {
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
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempFile, stateFile)
}

// ValidateModelAccess checks if a model is allowed for Slacker operations.
// For CLI operations, this should not be called.
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
