package slacker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
)

// HelpContextBuilder gathers contextual information for help analysis
type HelpContextBuilder struct {
	sessionManager *SessionManager
	config         *config.File
	tenantToolSet  interface{} // TenantToolSet
}

// NewHelpContextBuilder creates a new help context builder
func NewHelpContextBuilder(sessionManager *SessionManager, config *config.File, tenantToolSet interface{}) *HelpContextBuilder {
	return &HelpContextBuilder{
		sessionManager: sessionManager,
		config:         config,
		tenantToolSet:  tenantToolSet,
	}
}

// BuildContext creates a HelpContext for a specific user and message
func (hcb *HelpContextBuilder) BuildContext(ctx context.Context, userID, channelID, message string) *HelpContext {
	// Create a basic user context for session management
	userContext := &query.UserContext{
		UserID: userID,
	}

	// Get user session
	session := hcb.sessionManager.GetOrCreateSession(userID, channelID, userContext)
	if session == nil {
		session = &UserSession{} // Fallback session
	}

	// Resolve user preferences
	prefs := hcb.sessionManager.ResolveUserPreferences(userID, hcb.config)

	// Get available tools
	availableTools := hcb.getAvailableTools(ctx, userID)

	// Get available models
	availableModels := hcb.getAvailableModels(userID)

	// Check admin status
	isAdmin := hcb.checkAdminStatus(userID)

	// Get recent commands (last 5)
	recentCommands := hcb.getRecentCommands(session, 5)

	return &HelpContext{
		UserID:          userID,
		ChannelID:       channelID,
		OriginalMessage: message,
		AvailableTools:  availableTools,
		AvailableModels: availableModels,
		IsAdmin:         isAdmin,
		RecentCommands:  recentCommands,
		SessionPrefs:    &prefs,
	}
}

// getAvailableTools returns tools available to the user
func (hcb *HelpContextBuilder) getAvailableTools(ctx context.Context, userID string) []string {
	// This would integrate with the existing tool access system
	// For now, return common tool types
	return []string{
		"http:api-client",
		"local:git",
		"docker:postgres",
		"docker:nginx",
		"rag:query",
	}
}

// getAvailableModels returns models available to the user
func (hcb *HelpContextBuilder) getAvailableModels(userID string) []string {
	// This would integrate with the model access control system
	// For now, return common models
	return []string{
		config.DefaultLanguageModel,
		"llama3.2:latest",
		"qwen2.5:7b",
	}
}

// checkAdminStatus determines if user is an admin
func (hcb *HelpContextBuilder) checkAdminStatus(userID string) bool {
	// This would integrate with the multi-tenant admin system
	// For now, check if user is in admin list
	if hcb.config.MultiTenant != nil {
		for _, adminID := range hcb.config.MultiTenant.AdminUsers {
			if adminID == userID {
				return true
			}
		}
	}
	return false
}

// getRecentCommands extracts recent commands from session history
func (hcb *HelpContextBuilder) getRecentCommands(session *UserSession, limit int) []string {
	// This would analyze session message history to extract recent commands
	// For now, return some example recent commands
	return []string{
		"list my tools",
		"add http tool at https://api.example.com/mcp",
		"show preferences",
		"thinking on",
		"model list",
	}
}

// HelpIntegrator handles the integration of help analysis into the message flow
type HelpIntegrator struct {
	helpAnalyzer           *HelpAnalyzer
	contextBuilder         *HelpContextBuilder
	enhancedIntentAnalyzer *EnhancedIntentAnalyzer
}

// NewHelpIntegrator creates a new help integrator
func NewHelpIntegrator(helpAnalyzer *HelpAnalyzer, contextBuilder *HelpContextBuilder) *HelpIntegrator {
	return &HelpIntegrator{
		helpAnalyzer:           helpAnalyzer,
		contextBuilder:         contextBuilder,
		enhancedIntentAnalyzer: NewEnhancedIntentAnalyzer(helpAnalyzer),
	}
}

// HandleIntentFailure provides intelligent help when intent recognition fails
func (hi *HelpIntegrator) HandleIntentFailure(ctx context.Context, userID, channelID, message string) (*HelpAnalysis, error) {
	helpCtx := hi.contextBuilder.BuildContext(ctx, userID, channelID, message)

	// Use enhanced intent analysis for better suggestions
	analysis, err := hi.enhancedIntentAnalyzer.AnalyzeIntentFailureEnhanced(ctx, message, helpCtx)
	if err != nil {
		return nil, fmt.Errorf("analyzing enhanced intent failure: %w", err)
	}

	return analysis, nil
}

// HandleModelAccessFailure provides intelligent help when model access is denied
func (hi *HelpIntegrator) HandleModelAccessFailure(ctx context.Context, userID, channelID, model, reason string) (*HelpAnalysis, error) {
	helpCtx := hi.contextBuilder.BuildContext(ctx, userID, channelID, fmt.Sprintf("model request: %s", model))

	analysis, err := hi.helpAnalyzer.AnalyzeModelAccess(ctx, model, reason, helpCtx)
	if err != nil {
		return nil, fmt.Errorf("analyzing model access failure: %w", err)
	}

	return analysis, nil
}

// HandleToolConfigError provides intelligent help when tool configuration fails
func (hi *HelpIntegrator) HandleToolConfigError(ctx context.Context, userID, channelID, toolType, configStr string, err error) (*HelpAnalysis, error) {
	helpCtx := hi.contextBuilder.BuildContext(ctx, userID, channelID, fmt.Sprintf("tool config: %s %s", toolType, configStr))

	analysis, err := hi.helpAnalyzer.AnalyzeToolConfig(ctx, toolType, configStr, err, helpCtx)
	if err != nil {
		return nil, fmt.Errorf("analyzing tool config error: %w", err)
	}

	return analysis, nil
}

// HandleToolAccessDenied provides intelligent help when tool access is denied
func (hi *HelpIntegrator) HandleToolAccessDenied(ctx context.Context, userID, channelID, toolName, reason string) (*HelpAnalysis, error) {
	helpCtx := hi.contextBuilder.BuildContext(ctx, userID, channelID, fmt.Sprintf("tool access: %s", toolName))

	analysis, err := hi.helpAnalyzer.AnalyzeToolAccess(ctx, toolName, reason, helpCtx)
	if err != nil {
		return nil, fmt.Errorf("analyzing tool access denied: %w", err)
	}

	return analysis, nil
}

// HandleAdminRequest provides intelligent help for admin-specific requests
func (hi *HelpIntegrator) HandleAdminRequest(ctx context.Context, userID, channelID, request string) (*HelpAnalysis, error) {
	helpCtx := hi.contextBuilder.BuildContext(ctx, userID, channelID, fmt.Sprintf("admin request: %s", request))

	analysis, err := hi.helpAnalyzer.AnalyzeAdminRequest(ctx, request, helpCtx)
	if err != nil {
		return nil, fmt.Errorf("analyzing admin request: %w", err)
	}

	return analysis, nil
}

// FormatHelpMessage formats a help analysis into a user-friendly message
func (hi *HelpIntegrator) FormatHelpMessage(analysis *HelpAnalysis) string {
	var builder strings.Builder

	// Main diagnosis with emoji
	builder.WriteString("🤖 **Intelligent Help**\n\n")
	fmt.Fprintf(&builder, "**Issue:** %s\n\n", analysis.Diagnosis)

	// Suggestions
	if len(analysis.Suggestions) > 0 {
		builder.WriteString("💡 **Suggestions:**\n")
		for i, suggestion := range analysis.Suggestions {
			fmt.Fprintf(&builder, "%d. %s\n", i+1, suggestion)
		}
		builder.WriteString("\n")
	}

	// Examples
	if len(analysis.Examples) > 0 {
		builder.WriteString("📋 **Examples:**\n")
		for _, example := range analysis.Examples {
			fmt.Fprintf(&builder, "• `%s`\n", example)
		}
		builder.WriteString("\n")
	}

	// Additional context
	if analysis.ContextHelp != "" {
		builder.WriteString("ℹ️ **Additional Help:**\n")
		fmt.Fprintf(&builder, "%s\n\n", analysis.ContextHelp)
	}

	// Interactive actions (placeholder for future implementation)
	if len(analysis.Actions) > 0 {
		builder.WriteString("🎯 **Actions:**\n")
		for _, action := range analysis.Actions {
			fmt.Fprintf(&builder, "• %s\n", action.Label)
		}
	}

	return builder.String()
}

// ConfidenceThreshold determines when to show help
const ConfidenceThreshold = 0.6

// ShouldShowHelp determines if help should be shown based on analysis confidence
func ShouldShowHelp(analysis *HelpAnalysis) bool {
	return analysis.Confidence >= ConfidenceThreshold
}

// QuickHelpFormat provides a concise format for quick suggestions
func QuickHelpFormat(analysis *HelpAnalysis) string {
	if len(analysis.Suggestions) > 0 {
		return fmt.Sprintf("🤖 %s\n💡 Try: `%s`", analysis.Diagnosis, analysis.Suggestions[0])
	}
	return fmt.Sprintf("🤖 %s", analysis.Diagnosis)
}

// HelpResponse represents a formatted help response for Slack
type HelpResponse struct {
	Text        string       // The message to send to the user
	QuickText   string       // Quick one-liner version
	Confidence  float64      // Confidence score
	FailureType string       // Type of failure for tracking
	Actions     []HelpAction // Interactive actions
	Timestamp   time.Time    // When help was generated
}

// CreateHelpResponse creates a formatted help response
func (hi *HelpIntegrator) CreateHelpResponse(analysis *HelpAnalysis) *HelpResponse {
	return &HelpResponse{
		Text:        hi.FormatHelpMessage(analysis),
		QuickText:   QuickHelpFormat(analysis),
		Confidence:  analysis.Confidence,
		FailureType: analysis.FailureType,
		Actions:     analysis.Actions,
		Timestamp:   time.Now(),
	}
}

// HelpMetrics tracks help system performance
type HelpMetrics struct {
	TotalRequests     int                      `json:"total_requests"`
	IntentFailures    int                      `json:"intent_failures"`
	ModelAccessIssues int                      `json:"model_access_issues"`
	ToolConfigErrors  int                      `json:"tool_config_errors"`
	ToolAccessDenied  int                      `json:"tool_access_denied"`
	AdminHelpRequests int                      `json:"admin_help_requests"`
	AverageConfidence float64                  `json:"average_confidence"`
	UserSatisfaction  map[string]float64       `json:"user_satisfaction"`
	ResponseTimes     map[string]time.Duration `json:"response_times"`

	// Cache metrics
	CacheHits       int            `json:"cache_hits"`
	CacheMisses     int            `json:"cache_misses"`
	CacheHitStats   map[string]int `json:"cache_hit_stats"`
	CacheMissStats  map[string]int `json:"cache_miss_stats"`
	AverageCacheAge time.Duration  `json:"average_cache_age"`

	// Time tracking
	FirstRequestTime time.Time `json:"first_request_time"`
	LastRequestTime  time.Time `json:"last_request_time"`
}

// RecordHelpRequest tracks a help request for metrics
func (hm *HelpMetrics) RecordHelpRequest(failureType string, confidence float64, responseTime time.Duration) {
	hm.TotalRequests++

	switch failureType {
	case "intent_failure":
		hm.IntentFailures++
	case "model_access":
		hm.ModelAccessIssues++
	case "tool_config":
		hm.ToolConfigErrors++
	case "tool_access":
		hm.ToolAccessDenied++
	case "admin_help":
		hm.AdminHelpRequests++
	}

	// Update average confidence
	hm.AverageConfidence = ((hm.AverageConfidence * float64(hm.TotalRequests-1)) + confidence) / float64(hm.TotalRequests)

	// Track response times
	if hm.ResponseTimes == nil {
		hm.ResponseTimes = make(map[string]time.Duration)
	}
	hm.ResponseTimes[failureType] = responseTime

	// Track time-based metrics
	hm.trackTimeMetrics()
}

// trackTimeMetrics updates time-based metrics for help system
func (hm *HelpMetrics) trackTimeMetrics() {
	now := time.Now()
	if hm.LastRequestTime.IsZero() {
		hm.FirstRequestTime = now
		hm.LastRequestTime = now
		return
	}

	hm.LastRequestTime = now
}

// RecordCacheHit tracks when a help response is served from cache
func (hm *HelpMetrics) RecordCacheHit(failureType string, cacheAge time.Duration) {
	hm.CacheHits++
	if hm.CacheHitStats == nil {
		hm.CacheHitStats = make(map[string]int)
	}
	hm.CacheHitStats[failureType]++

	// Track average cache age
	if hm.AverageCacheAge == 0 {
		hm.AverageCacheAge = cacheAge
	} else {
		hm.AverageCacheAge = (hm.AverageCacheAge + cacheAge) / 2
	}
}

// RecordCacheMiss tracks when a help response is not found in cache
func (hm *HelpMetrics) RecordCacheMiss(failureType string) {
	hm.CacheMisses++
	if hm.CacheMissStats == nil {
		hm.CacheMissStats = make(map[string]int)
	}
	hm.CacheMissStats[failureType]++
}

// RecordUserFeedback tracks user satisfaction with help responses
func (hm *HelpMetrics) RecordUserFeedback(failureType string, rating int, comment string) {
	if hm.UserSatisfaction == nil {
		hm.UserSatisfaction = make(map[string]float64)
	}

	// Calculate running average for this failure type
	currentRating := hm.UserSatisfaction[failureType]
	if currentRating == 0 {
		hm.UserSatisfaction[failureType] = float64(rating)
	} else {
		hm.UserSatisfaction[failureType] = (currentRating + float64(rating)) / 2
	}

	// Track feedback comments if needed (could be expanded later)
	_ = comment // Placeholder for future comment tracking functionality
}

// GetPerformanceReport returns a formatted performance report
func (hm *HelpMetrics) GetPerformanceReport() string {
	var report strings.Builder

	fmt.Fprintf(&report, "📊 **Help System Performance Report**\n\n")

	// Overview statistics
	fmt.Fprintf(&report, "📈 **Overview:**\n")
	fmt.Fprintf(&report, "• Total Requests: %d\n", hm.TotalRequests)
	fmt.Fprintf(&report, "• Average Confidence: %.2f\n", hm.AverageConfidence)

	if !hm.FirstRequestTime.IsZero() {
		fmt.Fprintf(&report, "• First Request: %s\n", hm.FirstRequestTime.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&report, "• Last Request: %s\n", hm.LastRequestTime.Format("2006-01-02 15:04:05"))
	}

	// Breakdown by type
	fmt.Fprintf(&report, "\n📋 **Request Breakdown:**\n")
	fmt.Fprintf(&report, "• Intent Failures: %d\n", hm.IntentFailures)
	fmt.Fprintf(&report, "• Model Access Issues: %d\n", hm.ModelAccessIssues)
	fmt.Fprintf(&report, "• Tool Config Errors: %d\n", hm.ToolConfigErrors)
	fmt.Fprintf(&report, "• Tool Access Denied: %d\n", hm.ToolAccessDenied)
	fmt.Fprintf(&report, "• Admin Help Requests: %d\n", hm.AdminHelpRequests)

	// Performance metrics
	fmt.Fprintf(&report, "\n⚡ **Performance:**\n")
	hm.TotalRequests = hm.IntentFailures + hm.ModelAccessIssues + hm.ToolConfigErrors + hm.ToolAccessDenied + hm.AdminHelpRequests

	for failureType, responseTime := range hm.ResponseTimes {
		fmt.Fprintf(&report, "• %s: %s avg response time\n", failureType, responseTime.String())
	}

	// Cache performance
	if hm.CacheHits > 0 || hm.CacheMisses > 0 {
		totalCacheRequests := hm.CacheHits + hm.CacheMisses
		hitRate := float64(hm.CacheHits) / float64(totalCacheRequests) * 100
		fmt.Fprintf(&report, "• Cache Hit Rate: %.1f%%\n", hitRate)
		fmt.Fprintf(&report, "• Average Cache Age: %s\n", hm.AverageCacheAge.String())
	}

	// User satisfaction
	if len(hm.UserSatisfaction) > 0 {
		fmt.Fprintf(&report, "\n😊 **User Satisfaction:**\n")
		for failureType, rating := range hm.UserSatisfaction {
			fmt.Fprintf(&report, "• %s: %.1f/5.0\n", failureType, rating)
		}
	}

	return report.String()
}
