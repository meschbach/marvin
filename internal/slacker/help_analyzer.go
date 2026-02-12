// Package slacker provides LLM-based intelligent help analysis for Marvin Slacker
package slacker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/ollama/ollama/api"
)

// HelpAnalyzer provides intelligent help analysis using LLM capabilities
type HelpAnalyzer struct {
	llm            query.LLM
	config         *config.File
	sessionManager *SessionManager
	toolManager    interface{} // ToolManager interface
	tenantToolSet  interface{} // TenantToolSet interface
}

// HelpContext contains contextual information for help analysis
type HelpContext struct {
	UserID          string
	ChannelID       string
	OriginalMessage string
	AvailableTools  []string
	AvailableModels []string
	IsAdmin         bool
	RecentCommands  []string              // Last 5 commands for context
	FailedIntent    *ToolManagementIntent // If intent recognition failed
	ModelAccessInfo *ModelAccessInfo
	ToolAccessInfo  *ToolAccessInfo
	SessionPrefs    *UserPreferences
}

// ModelAccessInfo contains model access details
type ModelAccessInfo struct {
	RequestedModel string
	AccessDenied   bool
	Reason         string
	AvailableAlts  []string
	DefaultModel   string
}

// ToolAccessInfo contains tool access details
type ToolAccessInfo struct {
	RequestedTool    string
	AccessDenied     bool
	Reason           string
	AvailableAlts    []string
	ApprovalWorkflow bool
}

// HelpAnalysis represents the result of help analysis
type HelpAnalysis struct {
	FailureType string       // "intent_failure", "model_access", "tool_config", "tool_access"
	Diagnosis   string       // What went wrong
	Suggestions []string     // How to fix it
	Examples    []string     // Usage examples
	Actions     []HelpAction // Interactive options
	ContextHelp string       // Additional context
	Confidence  float64      // Confidence in analysis
}

// HelpAction represents an interactive help action
type HelpAction struct {
	Type       string            // "suggest_command", "request_access", "show_examples"
	Label      string            // User-visible label
	Data       map[string]string // Action data
	Confidence float64           // Confidence this action is relevant
}

// llmHelpResponse represents the expected JSON response from the LLM
type llmHelpResponse struct {
	Diagnosis   string   `json:"diagnosis"`
	Suggestions []string `json:"suggestions"`
	Examples    []string `json:"examples"`
	ContextHelp string   `json:"context_help"`
	Confidence  float64  `json:"confidence"`
}

// NewHelpAnalyzer creates a new help analyzer
func NewHelpAnalyzer(llm query.LLM, config *config.File, sessionManager *SessionManager, toolManager interface{}, tenantToolSet interface{}) *HelpAnalyzer {
	return &HelpAnalyzer{
		llm:            llm,
		config:         config,
		sessionManager: sessionManager,
		toolManager:    toolManager,
		tenantToolSet:  tenantToolSet,
	}
}

// AnalyzeIntentFailure analyzes failed intent recognition using LLM
func (h *HelpAnalyzer) AnalyzeIntentFailure(ctx context.Context, message string, helpCtx *HelpContext) (*HelpAnalysis, error) {
	prompt := h.buildIntentFailurePrompt(message, helpCtx)

	response, err := h.callHelpLLM(ctx, prompt)
	if err != nil {
		return h.fallbackIntentAnalysis(message, helpCtx), err
	}

	return h.parseHelpResponse(response, "intent_failure"), nil
}

// AnalyzeModelAccess analyzes model access issues using LLM
func (h *HelpAnalyzer) AnalyzeModelAccess(ctx context.Context, model string, reason string, helpCtx *HelpContext) (*HelpAnalysis, error) {
	helpCtx.ModelAccessInfo = &ModelAccessInfo{
		RequestedModel: model,
		AccessDenied:   true,
		Reason:         reason,
		AvailableAlts:  h.getAvailableModels(helpCtx.UserID),
		DefaultModel:   config.DefaultLanguageModel,
	}

	prompt := h.buildModelAccessPrompt(model, reason, helpCtx)

	response, err := h.callHelpLLM(ctx, prompt)
	if err != nil {
		return h.fallbackModelAccessAnalysis(model, reason, helpCtx), err
	}

	return h.parseHelpResponse(response, "model_access"), nil
}

// AnalyzeToolConfig analyzes tool configuration errors using LLM
func (h *HelpAnalyzer) AnalyzeToolConfig(ctx context.Context, toolType string, configStr string, err error, helpCtx *HelpContext) (*HelpAnalysis, error) {
	prompt := h.buildToolConfigPrompt(toolType, configStr, err.Error(), helpCtx)

	response, err := h.callHelpLLM(ctx, prompt)
	if err != nil {
		return h.fallbackToolConfigAnalysis(toolType, configStr, err, helpCtx), err
	}

	return h.parseHelpResponse(response, "tool_config"), nil
}

// AnalyzeToolAccess analyzes tool permission issues using LLM
func (h *HelpAnalyzer) AnalyzeToolAccess(ctx context.Context, toolName string, reason string, helpCtx *HelpContext) (*HelpAnalysis, error) {
	helpCtx.ToolAccessInfo = &ToolAccessInfo{
		RequestedTool:    toolName,
		AccessDenied:     true,
		Reason:           reason,
		AvailableAlts:    h.findAlternativeTools(helpCtx.UserID, toolName),
		ApprovalWorkflow: !helpCtx.IsAdmin,
	}

	prompt := h.buildToolAccessPrompt(toolName, reason, helpCtx)

	response, err := h.callHelpLLM(ctx, prompt)
	if err != nil {
		return h.fallbackToolAccessAnalysis(toolName, reason, helpCtx), err
	}

	return h.parseHelpResponse(response, "tool_access"), nil
}

// AnalyzeAdminRequest analyzes admin-specific help requests
func (h *HelpAnalyzer) AnalyzeAdminRequest(ctx context.Context, request string, helpCtx *HelpContext) (*HelpAnalysis, error) {
	prompt := h.buildAdminPrompt(request, helpCtx)

	response, err := h.callHelpLLM(ctx, prompt)
	if err != nil {
		return h.fallbackAdminAnalysis(request, helpCtx), err
	}

	return h.parseHelpResponse(response, "admin_help"), nil
}

// buildIntentFailurePrompt creates a prompt for intent failure analysis
func (h *HelpAnalyzer) buildIntentFailurePrompt(message string, helpCtx *HelpContext) string {
	recentCommands := strings.Join(helpCtx.RecentCommands, "\n")
	availableTools := strings.Join(helpCtx.AvailableTools, ", ")

	return fmt.Sprintf(`You are an intelligent help assistant for Marvin, an AI agent platform.

USER QUERY: "%s"

CONTEXT:
- Available Tools: %s
- Recent Commands: %s
- User is Admin: %t
- User Preferences: %+v

TASK: Analyze the user's failed command attempt and provide intelligent help. The user's input did not match any known command patterns.

ANALYSIS REQUIREMENTS:
1. Identify what type of command the user was likely trying to execute
2. Detect typos, missing parameters, or incorrect syntax
3. Provide specific corrections with exact command syntax
4. Include relevant examples for the suggested commands
5. Consider the user's admin status and available tools

RESPONSE FORMAT (JSON):
{
  "diagnosis": "Brief explanation of what went wrong",
  "suggestions": ["exact command suggestions"],
  "examples": ["usage examples with context"],
  "context_help": "Additional guidance",
  "confidence": 0.85
}

Focus on being helpful, specific, and providing actionable guidance.`,
		message, availableTools, recentCommands, helpCtx.IsAdmin, helpCtx.SessionPrefs)
}

// buildModelAccessPrompt creates a prompt for model access analysis
func (h *HelpAnalyzer) buildModelAccessPrompt(model, reason string, helpCtx *HelpContext) string {
	availableModels := strings.Join(helpCtx.AvailableModels, ", ")

	return fmt.Sprintf(`You are an intelligent help assistant for Marvin, an AI agent platform.

MODEL ACCESS ISSUE:
- Requested Model: %s
- Access Denied: %t
- Reason: %s
- Default Model: %s
- Available Alternatives: %s
- User is Admin: %t

TASK: Provide helpful guidance about model access restrictions and alternatives.

ANALYSIS REQUIREMENTS:
1. Explain why access was denied in user-friendly terms
2. Suggest available alternatives if relevant
3. If user is admin, provide options to grant access
4. Include examples of how to switch models
5. Be transparent about the fallback behavior

RESPONSE FORMAT (JSON):
{
  "diagnosis": "Explanation of the access restriction",
  "suggestions": ["model alternatives or access commands"],
  "examples": ["commands to use available models"],
  "context_help": "Additional guidance on model usage",
  "confidence": 0.90
}

Focus on transparency and providing actionable alternatives.`,
		model, helpCtx.ModelAccessInfo.AccessDenied, reason, helpCtx.ModelAccessInfo.DefaultModel,
		availableModels, helpCtx.IsAdmin)
}

// buildToolConfigPrompt creates a prompt for tool configuration analysis
func (h *HelpAnalyzer) buildToolConfigPrompt(toolType, configStr, errorMsg string, helpCtx *HelpContext) string {
	supportedTypes := "http, local, docker"

	return fmt.Sprintf(`You are an intelligent help assistant for Marvin, an AI agent platform.

TOOL CONFIGURATION ERROR:
- Tool Type: %s
- Configuration: %s
- Error: %s
- Supported Types: %s
- User is Admin: %t

TASK: Analyze the tool configuration error and provide intelligent guidance.

ANALYSIS REQUIREMENTS:
1. Explain what went wrong in the configuration
2. Show correct syntax for the intended tool type
3. Provide working examples for each supported tool type
4. Guide the user to the correct format step by step
5. Include validation tips to prevent future errors

RESPONSE FORMAT (JSON):
{
  "diagnosis": "Clear explanation of the configuration issue",
  "suggestions": ["corrected configuration examples"],
  "examples": ["complete working examples for different tool types"],
  "context_help": "Best practices for tool configuration",
  "confidence": 0.95
}

Focus on educational guidance and preventing future mistakes.`,
		toolType, configStr, errorMsg, supportedTypes, helpCtx.IsAdmin)
}

// buildToolAccessPrompt creates a prompt for tool access analysis
func (h *HelpAnalyzer) buildToolAccessPrompt(toolName, reason string, helpCtx *HelpContext) string {
	alternatives := strings.Join(helpCtx.ToolAccessInfo.AvailableAlts, ", ")

	return fmt.Sprintf(`You are an intelligent help assistant for Marvin, an AI agent platform.

TOOL ACCESS RESTRICTION:
- Requested Tool: %s
- Access Denied: %t
- Reason: %s
- Available Alternatives: %s
- User is Admin: %t
- Approval Workflow Available: %t

TASK: Provide helpful guidance about tool access restrictions.

ANALYSIS REQUIREMENTS:
1. Explain why access was restricted in user-friendly terms
2. Suggest available alternative tools
3. If applicable, explain the permission request process
4. For admins, provide management options
5. Include examples of using alternative tools

RESPONSE FORMAT (JSON):
{
  "diagnosis": "Explanation of the access restriction",
  "suggestions": ["alternative tools or access requests"],
  "examples": ["commands to use available alternatives"],
  "context_help": "Security context and guidance",
  "confidence": 0.90
}

Focus on transparency and providing secure alternatives.`,
		toolName, helpCtx.ToolAccessInfo.AccessDenied, reason, alternatives,
		helpCtx.IsAdmin, helpCtx.ToolAccessInfo.ApprovalWorkflow)
}

// callHelpLLM makes a help-specific LLM call
func (h *HelpAnalyzer) callHelpLLM(ctx context.Context, prompt string) (string, error) {
	// Use a fast, capable model for help analysis
	helpModel := h.config.LanguageModel()
	if helpModel == "" {
		helpModel = config.DefaultLanguageModel
	}

	var response strings.Builder

	req := &api.ChatRequest{
		Model: helpModel,
		Messages: []api.Message{
			{Role: "system", Content: "You are a helpful assistant that provides JSON responses for intelligent help analysis. Always respond with valid JSON."},
			{Role: "user", Content: prompt},
		},
		Stream: new(bool), // Non-streaming for structured response
		Options: map[string]interface{}{
			"temperature": 0.3, // Lower temperature for consistent analysis
		},
	}

	err := h.llm.Chat(ctx, req, func(resp api.ChatResponse) error {
		response.WriteString(resp.Message.Content)
		return nil
	})

	return response.String(), err
}

// parseHelpResponse parses the LLM response into a HelpAnalysis
func (h *HelpAnalyzer) parseHelpResponse(response, failureType string) *HelpAnalysis {
	// Clean the response - extract JSON if it's wrapped in markdown code blocks
	cleanedResponse := h.extractJSON(response)

	// Try to parse as JSON
	var llmResp llmHelpResponse
	if err := json.Unmarshal([]byte(cleanedResponse), &llmResp); err != nil {
		// If JSON parsing fails, log the error and return fallback
		fmt.Printf("Warning: Failed to parse LLM help response as JSON: %v\n", err)
		fmt.Printf("Response was: %s\n", cleanedResponse)
		return h.fallbackHelpResponse(failureType, cleanedResponse)
	}

	// Validate and normalize the response
	if llmResp.Diagnosis == "" {
		llmResp.Diagnosis = "Unable to analyze the issue."
	}
	if len(llmResp.Suggestions) == 0 {
		llmResp.Suggestions = []string{"Please check your command syntax"}
	}
	if llmResp.Confidence <= 0 {
		llmResp.Confidence = 0.5 // Default confidence
	}
	if llmResp.Confidence > 1.0 {
		llmResp.Confidence = 1.0 // Clamp to valid range
	}

	// Generate actions based on failure type and suggestions
	actions := h.generateHelpActions(failureType, llmResp)

	return &HelpAnalysis{
		FailureType: failureType,
		Diagnosis:   llmResp.Diagnosis,
		Suggestions: llmResp.Suggestions,
		Examples:    llmResp.Examples,
		Actions:     actions,
		ContextHelp: llmResp.ContextHelp,
		Confidence:  llmResp.Confidence,
	}
}

// extractJSON extracts JSON content from LLM response, handling markdown code blocks
func (h *HelpAnalyzer) extractJSON(response string) string {
	// Check if response is wrapped in markdown code blocks
	jsonPattern := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)\\s*```\\s*$")
	matches := jsonPattern.FindStringSubmatch(response)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Try to find JSON-like content with braces
	bracePattern := regexp.MustCompile(`\{[\s\S]*\}`)
	matches = bracePattern.FindStringSubmatch(response)

	if len(matches) > 0 {
		return strings.TrimSpace(matches[0])
	}

	// Return original response if no patterns found
	return strings.TrimSpace(response)
}

// generateHelpActions creates actionable help buttons based on failure type and content
func (h *HelpAnalyzer) generateHelpActions(failureType string, resp llmHelpResponse) []HelpAction {
	var actions []HelpAction

	switch failureType {
	case "intent_failure":
		actions = append(actions, HelpAction{
			Type:       "show_examples",
			Label:      "📖 Show Examples",
			Data:       map[string]string{"type": "intent_examples"},
			Confidence: 0.8,
		})
		if len(resp.Suggestions) > 0 {
			actions = append(actions, HelpAction{
				Type:       "suggest_command",
				Label:      "💡 " + resp.Suggestions[0],
				Data:       map[string]string{"suggestion": resp.Suggestions[0]},
				Confidence: resp.Confidence,
			})
		}

	case "model_access":
		actions = append(actions, HelpAction{
			Type:       "suggest_command",
			Label:      "📋 List Available Models",
			Data:       map[string]string{"command": "model list"},
			Confidence: 0.9,
		})
		actions = append(actions, HelpAction{
			Type:       "request_access",
			Label:      "🔓 Request Model Access",
			Data:       map[string]string{"type": "model_access"},
			Confidence: 0.7,
		})

	case "tool_config":
		actions = append(actions, HelpAction{
			Type:       "show_examples",
			Label:      "📝 Tool Configuration Examples",
			Data:       map[string]string{"type": "tool_config_examples"},
			Confidence: 0.9,
		})

	case "tool_access":
		actions = append(actions, HelpAction{
			Type:       "suggest_command",
			Label:      "🔧 List Available Tools",
			Data:       map[string]string{"command": "list my tools"},
			Confidence: 0.8,
		})
		actions = append(actions, HelpAction{
			Type:       "request_access",
			Label:      "🔓 Request Tool Access",
			Data:       map[string]string{"type": "tool_access"},
			Confidence: 0.6,
		})

	case "admin_help":
		actions = append(actions, HelpAction{
			Type:       "suggest_command",
			Label:      "⚙️ Admin Commands",
			Data:       map[string]string{"command": "admin help"},
			Confidence: 0.9,
		})
		actions = append(actions, HelpAction{
			Type:       "escalate",
			Label:      "🚨 Escalate to Support",
			Data:       map[string]string{"type": "admin_escalation"},
			Confidence: 0.7,
		})
	}

	return actions
}

// fallbackHelpResponse provides basic help when LLM parsing fails
func (h *HelpAnalyzer) fallbackHelpResponse(failureType, rawResponse string) *HelpAnalysis {
	// Try to extract some useful information from the raw response
	var diagnosis string
	if strings.Contains(strings.ToLower(rawResponse), "command") {
		diagnosis = "Command not recognized. Please check syntax."
	} else if strings.Contains(strings.ToLower(rawResponse), "model") {
		diagnosis = "Model access issue occurred. Using default model."
	} else if strings.Contains(strings.ToLower(rawResponse), "tool") {
		diagnosis = "Tool configuration or access issue detected."
	} else {
		diagnosis = "An error occurred while processing your request."
	}

	// Generate basic suggestions based on failure type
	var suggestions []string
	var examples []string

	switch failureType {
	case "intent_failure":
		suggestions = []string{"Check command spelling", "Use '@marvin help' for assistance"}
		examples = []string{"list my tools", "show preferences", "add http tool at <url>"}
	case "model_access":
		suggestions = []string{"Use available model", "Request access from admin"}
		examples = []string{"@marvin model list", "@marvin thinking on"}
	case "tool_config":
		suggestions = []string{"Verify tool type", "Check configuration format"}
		examples = []string{"add http tool at https://api.example.com/mcp", "add local program /usr/bin/git"}
	case "tool_access":
		suggestions = []string{"Use available tools", "Request access if needed"}
		examples = []string{"list my tools", "add http tool at https://example.com/mcp"}
	case "admin_help":
		suggestions = []string{"Check admin commands", "Review system configuration", "Escalate if needed"}
		examples = []string{"list pending requests", "approve tool request <id>", "system status"}
	default:
		suggestions = []string{"Please try again or contact support"}
	}

	return &HelpAnalysis{
		FailureType: failureType,
		Diagnosis:   diagnosis,
		Suggestions: suggestions,
		Examples:    examples,
		Confidence:  0.5, // Lower confidence for fallback responses
	}
}

// Helper methods for context gathering
func (h *HelpAnalyzer) getAvailableModels(userID string) []string {
	// Get models available to this specific user
	// This would integrate with the model access control system
	return []string{"ministral-3:3b", "llama3.2:latest", "qwen2.5:7b"}
}

func (h *HelpAnalyzer) findAlternativeTools(userID, requestedTool string) []string {
	// Find similar tools available to the user
	// This would analyze tool capabilities and suggest alternatives
	return []string{"docker:" + requestedTool, "http:api-client"}
}

// Fallback methods when LLM is unavailable
func (h *HelpAnalyzer) fallbackIntentAnalysis(message string, helpCtx *HelpContext) *HelpAnalysis {
	return &HelpAnalysis{
		FailureType: "intent_failure",
		Diagnosis:   "Command not recognized. Here are some available options:",
		Suggestions: []string{"list my tools", "show preferences", "add http tool at <url>"},
		Examples:    []string{"list my tools", "show preferences", "add http tool at https://api.example.com/mcp"},
		Confidence:  0.6,
	}
}

func (h *HelpAnalyzer) fallbackModelAccessAnalysis(model, reason string, helpCtx *HelpContext) *HelpAnalysis {
	return &HelpAnalysis{
		FailureType: "model_access",
		Diagnosis:   fmt.Sprintf("Model '%s' not available. Using default instead.", model),
		Suggestions: []string{"Use available models", "Request access from admin"},
		Examples:    []string{"@marvin model list", "Continue with current model"},
		Confidence:  0.8,
	}
}

func (h *HelpAnalyzer) fallbackToolConfigAnalysis(toolType, configStr string, err error, helpCtx *HelpContext) *HelpAnalysis {
	return &HelpAnalysis{
		FailureType: "tool_config",
		Diagnosis:   fmt.Sprintf("Configuration error for '%s': %s", toolType, err.Error()),
		Suggestions: []string{"Check tool type", "Verify configuration format"},
		Examples:    []string{"add http tool at https://api.example.com/mcp", "add local program /usr/bin/git"},
		Confidence:  0.7,
	}
}

func (h *HelpAnalyzer) fallbackToolAccessAnalysis(toolName, reason string, helpCtx *HelpContext) *HelpAnalysis {
	return &HelpAnalysis{
		FailureType: "tool_access",
		Diagnosis:   fmt.Sprintf("Access to '%s' restricted: %s", toolName, reason),
		Suggestions: []string{"Use available tools", "Request access if needed"},
		Examples:    []string{"list my tools", "add http tool at https://api.example.com/mcp"},
		Confidence:  0.8,
	}
}

// buildAdminPrompt creates a prompt for admin-specific help analysis
func (h *HelpAnalyzer) buildAdminPrompt(request string, helpCtx *HelpContext) string {
	return fmt.Sprintf(`You are an intelligent help assistant for Marvin, an AI agent platform.

ADMIN HELP REQUEST:
- Request: %s
- User is Admin: %t
- Available Tools: %s

TASK: Provide intelligent guidance for administrative operations and escalation.

ANALYSIS REQUIREMENTS:
1. Identify the specific admin operation being requested
2. Provide step-by-step guidance for complex admin tasks
3. Include escalation options for advanced issues
4. Provide examples of admin commands and workflows
5. Include security considerations and best practices

RESPONSE FORMAT (JSON):
{
  "diagnosis": "Clear explanation of the admin request",
  "suggestions": ["admin command suggestions and escalation options"],
  "examples": ["complete admin command examples"],
  "context_help": "Administrative guidance and security notes",
  "confidence": 0.95
}

Focus on providing comprehensive admin guidance with escalation paths.`,
		request, helpCtx.IsAdmin, strings.Join(helpCtx.AvailableTools, ", "))
}

// fallbackAdminAnalysis provides basic admin help when LLM is unavailable
func (h *HelpAnalyzer) fallbackAdminAnalysis(request string, helpCtx *HelpContext) *HelpAnalysis {
	return &HelpAnalysis{
		FailureType: "admin_help",
		Diagnosis:   fmt.Sprintf("Admin help request: %s", request),
		Suggestions: []string{
			"Check available admin commands",
			"Review system configuration",
			"Contact support for complex issues",
		},
		Examples: []string{
			"list pending requests",
			"approve tool request <request-id>",
			"show system status",
		},
		Confidence: 0.7,
	}
}
