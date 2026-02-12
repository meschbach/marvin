package slacker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// QueryHandler defines the interface for handling queries
type QueryHandler interface {
	HandleQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, updater *SlackUpdater) error
}

// MessageProcessor defines the interface for processing messages
type MessageProcessor interface {
	ProcessMessage(ctx context.Context, ev *slackevents.MessageEvent) error
}

// MessageHandler processes incoming Slack messages and intents
type MessageHandler struct {
	intentProcessor    *IntentProcessor
	connection         *SlackConnection
	queryHandler       QueryHandler
	toolManager        *ToolManagerImpl
	sessionManager     *SessionManager
	securityLogger     *sec.SecurityLogger
	config             *config.File
	tenantToolSet      *query.TenantToolSet
	helpAnalyzer       *HelpAnalyzer       // New: Intelligent help system
	helpContextBuilder *HelpContextBuilder // New: Help context builder
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(
	intentProcessor *IntentProcessor,
	connection *SlackConnection,
	queryHandler QueryHandler,
	toolManager *ToolManagerImpl,
	sessionManager *SessionManager,
	securityLogger *sec.SecurityLogger,
	config *config.File,
	tenantToolSet *query.TenantToolSet,
) *MessageHandler {
	return &MessageHandler{
		intentProcessor:    intentProcessor,
		connection:         connection,
		queryHandler:       queryHandler,
		toolManager:        toolManager,
		sessionManager:     sessionManager,
		securityLogger:     securityLogger,
		config:             config,
		tenantToolSet:      tenantToolSet,
		helpAnalyzer:       nil, // HelpAnalyzer will be added separately
		helpContextBuilder: nil, // HelpContextBuilder will be added separately
	}
}

// SetHelpAnalyzer adds intelligent help analysis to the message handler
func (mh *MessageHandler) SetHelpAnalyzer(helpAnalyzer *HelpAnalyzer) {
	mh.helpAnalyzer = helpAnalyzer
}

// SetHelpContextBuilder adds help context building to the message handler
func (mh *MessageHandler) SetHelpContextBuilder(helpContextBuilder *HelpContextBuilder) {
	mh.helpContextBuilder = helpContextBuilder
}

// ProcessMessage processes incoming Slack messages
func (mh *MessageHandler) ProcessMessage(ctx context.Context, ev *slackevents.MessageEvent) error {
	// Ignore messages from bots or messages without text
	if ev.BotID != "" || ev.SubType != "" || ev.Text == "" {
		return nil
	}

	// Check if message is mentioning the bot
	if ev.ChannelType == "im" {
		// Direct message - always process
	} else {
		// Channel message - check for bot mention
		if !strings.Contains(ev.Text, fmt.Sprintf("<@%s>", mh.connection.GetBotUserID())) {
			return nil
		}
	}

	// Remove bot mention from message
	cleanMessage := strings.ReplaceAll(ev.Text, fmt.Sprintf("<@%s>", mh.connection.GetBotUserID()), "")
	cleanMessage = strings.TrimSpace(cleanMessage)

	if cleanMessage == "" {
		return nil
	}

	// Create Slack context
	slackCtx := &SlackContext{
		UserID:    ev.User,
		ChannelID: ev.Channel,
		Message:   cleanMessage,
		Timestamp: ev.TimeStamp,
		ThreadTS:  ev.ThreadTimeStamp,
		TeamID:    "", // TODO: Get from user info if needed
	}

	// Get user info
	user, err := mh.connection.GetClient().GetUserInfo(ev.User)
	if err == nil {
		slackCtx.UserName = user.Name
	}

	// Log session event
	mh.securityLogger.LogSessionEvent(ev.User, ev.Channel, "Message received")

	// Create user context
	userCtx := &query.UserContext{
		UserID:      ev.User,
		SlackTeamID: slackCtx.TeamID,
		IsAdmin:     false, // This would need to be determined from tenantToolSet
	}

	// Get or create user session
	session := mh.sessionManager.GetOrCreateSession(ev.User, ev.Channel, userCtx)

	// Check if this is a tool management request
	intent, err := mh.intentProcessor.ProcessMessage(cleanMessage)
	if err != nil {
		return fmt.Errorf("processing intent: %w", err)
	}

	if intent != nil && intent.Confidence >= 0.7 {
		// Check if this is a preference management intent
		if strings.Contains(intent.Action, "thinking") || strings.Contains(intent.Action, "tools") ||
			strings.Contains(intent.Action, "done") || strings.Contains(intent.Action, "verbose") ||
			intent.Action == "show_preferences" {
			return mh.handlePreferenceIntent(ctx, slackCtx, session, intent)
		}

		// Check if this is a model access management intent
		if strings.HasPrefix(intent.Action, "model_access_") {
			return mh.handleModelAccessIntent(ctx, slackCtx, session, intent)
		}

		// Check if this is an admin help request
		if strings.HasPrefix(intent.Action, "admin_") {
			return mh.handleAdminIntent(ctx, slackCtx, session, intent)
		}

		// Handle tool management request
		return mh.toolManager.HandleToolIntent(ctx, slackCtx, session, intent)
	}

	// If intent exists but confidence is too low, provide help
	if intent != nil && intent.Confidence < 0.7 {
		return mh.handleIntentFailure(ctx, slackCtx, session, cleanMessage, ev)
	}

	// Handle as a regular query (intent == nil means no specific intent, not a failure)
	preferences := mh.sessionManager.ResolveUserPreferences(session.UserID, mh.config)
	updater := NewSlackUpdater(mh.connection.client, ev.Channel, NewSlackFormatter(), preferences)
	queryError := mh.queryHandler.HandleQueryWithUpdater(ctx, slackCtx, session, cleanMessage, updater)
	return errors.Join(err, queryError)
}

// handlePreferenceIntent processes preference management commands
func (mh *MessageHandler) handlePreferenceIntent(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// Process the intent and get response message
	response, err := HandlePreferenceIntent(intent, mh.sessionManager, session.UserID)
	if err != nil {
		mh.securityLogger.LogError(slackCtx.UserID, "PreferenceIntent", err.Error())
		response = "❌ Error processing preference command. Please try again."
	}

	// Send response to user
	_, _, err = mh.connection.client.PostMessageContext(
		ctx,
		slackCtx.ChannelID,
		slack.MsgOptionText(response, true),
	)
	return err
}

// handleModelAccessIntent processes model access management commands
func (mh *MessageHandler) handleModelAccessIntent(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// Check if user is admin
	if !mh.tenantToolSet.IsAdmin(slackCtx.UserID) {
		mh.securityLogger.LogError(slackCtx.UserID, "ModelAccess", "Unauthorized model access attempt")
		response := "❌ Only administrators can manage model access."
		_, _, err := mh.connection.client.PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText(response, true),
		)
		return err
	}

	var response string
	var err error

	switch intent.Action {
	case "model_access_list":
		response, err = mh.handleModelAccessList(ctx, slackCtx)
	case "model_access_allow":
		response, err = mh.handleModelAccessAllow(ctx, slackCtx, intent.Target)
	case "model_access_deny":
		response, err = mh.handleModelAccessDeny(ctx, slackCtx, intent.Target)
	case "model_access_clear":
		response, err = mh.handleModelAccessClear(ctx, slackCtx)
	case "model_access_status":
		response, err = mh.handleModelAccessStatus(ctx, slackCtx, intent.TargetUser)
	default:
		response = "❌ Unknown model access command."
	}

	if err != nil {
		mh.securityLogger.LogError(slackCtx.UserID, "ModelAccess", err.Error())
		response = fmt.Sprintf("❌ Error processing model access command: %v", err)
	}

	// Send response to user
	_, _, err = mh.connection.client.PostMessageContext(
		ctx,
		slackCtx.ChannelID,
		slack.MsgOptionText(response, true),
	)
	return err
}

// handleModelAccessList shows current model access configuration
func (mh *MessageHandler) handleModelAccessList(ctx context.Context, slackCtx *SlackContext) (string, error) {
	state, err := mh.config.GetEffectiveModelAccess()
	if err != nil {
		return "", fmt.Errorf("getting model access config: %w", err)
	}

	response := "🤖 **Model Access Configuration**\n\n"

	if len(state.AllowedModels) == 0 && len(state.DeniedModels) == 0 {
		response += "No restrictions in place - all models are allowed.\n"
	} else {
		if len(state.AllowedModels) > 0 {
			response += "✅ **Allowed Models:**\n"
			for _, model := range state.AllowedModels {
				response += fmt.Sprintf("  • %s\n", model)
			}
		}
		if len(state.DeniedModels) > 0 {
			response += "❌ **Denied Models:**\n"
			for _, model := range state.DeniedModels {
				response += fmt.Sprintf("  • %s\n", model)
			}
		}
	}

	response += fmt.Sprintf("\n🔧 **Default Model:** %s\n", state.DefaultModel)

	if state.UpdatedBy != "" && state.LastUpdated != "" {
		response += fmt.Sprintf("📝 **Last Updated:** %s by %s\n", state.LastUpdated, state.UpdatedBy)
	}

	return response, nil
}

// handleModelAccessAllow adds a model to the allowed list
func (mh *MessageHandler) handleModelAccessAllow(ctx context.Context, slackCtx *SlackContext, model string) (string, error) {
	state, err := mh.config.GetEffectiveModelAccess()
	if err != nil {
		return "", fmt.Errorf("getting current model access config: %w", err)
	}

	// Remove from denied list if present
	deniedModels := []string{}
	for _, denied := range state.DeniedModels {
		if denied != model {
			deniedModels = append(deniedModels, denied)
		}
	}

	// Add to allowed list if not already present
	allowedModels := state.AllowedModels
	for _, allowed := range allowedModels {
		if allowed == model {
			return fmt.Sprintf("ℹ️ Model '%s' is already allowed.", model), nil
		}
	}
	allowedModels = append(allowedModels, model)

	// Save updated state
	newState := &config.ModelAccessState{
		AllowedModels: allowedModels,
		DeniedModels:  deniedModels,
		DefaultModel:  state.DefaultModel,
		LastUpdated:   "", // Will be set by SaveModelAccessState
		UpdatedBy:     "", // Will be set by SaveModelAccessState
	}

	err = mh.config.SaveModelAccessState(newState, slackCtx.UserID)
	if err != nil {
		return "", fmt.Errorf("saving model access state: %w", err)
	}

	mh.securityLogger.LogConfigChange(slackCtx.UserID, "model_access",
		fmt.Sprintf("Allowed model: %s", model))

	return fmt.Sprintf("✅ Model '%s' has been added to the allowed list.", model), nil
}

// handleModelAccessDeny adds a model to the denied list
func (mh *MessageHandler) handleModelAccessDeny(ctx context.Context, slackCtx *SlackContext, model string) (string, error) {
	state, err := mh.config.GetEffectiveModelAccess()
	if err != nil {
		return "", fmt.Errorf("getting current model access config: %w", err)
	}

	// Remove from allowed list if present
	allowedModels := []string{}
	for _, allowed := range state.AllowedModels {
		if allowed != model {
			allowedModels = append(allowedModels, allowed)
		}
	}

	// Add to denied list if not already present
	deniedModels := state.DeniedModels
	for _, denied := range deniedModels {
		if denied == model {
			return fmt.Sprintf("ℹ️ Model '%s' is already denied.", model), nil
		}
	}
	deniedModels = append(deniedModels, model)

	// Save updated state
	newState := &config.ModelAccessState{
		AllowedModels: allowedModels,
		DeniedModels:  deniedModels,
		DefaultModel:  state.DefaultModel,
		LastUpdated:   "", // Will be set by SaveModelAccessState
		UpdatedBy:     "", // Will be set by SaveModelAccessState
	}

	err = mh.config.SaveModelAccessState(newState, slackCtx.UserID)
	if err != nil {
		return "", fmt.Errorf("saving model access state: %w", err)
	}

	mh.securityLogger.LogConfigChange(slackCtx.UserID, "model_access",
		fmt.Sprintf("Denied model: %s", model))

	return fmt.Sprintf("❌ Model '%s' has been added to the denied list.", model), nil
}

// handleModelAccessClear clears all model access restrictions
func (mh *MessageHandler) handleModelAccessClear(ctx context.Context, slackCtx *SlackContext) (string, error) {
	// Create empty state (no restrictions)
	newState := &config.ModelAccessState{
		AllowedModels: []string{},
		DeniedModels:  []string{},
		DefaultModel:  config.DefaultLanguageModel,
		LastUpdated:   "", // Will be set by SaveModelAccessState
		UpdatedBy:     "", // Will be set by SaveModelAccessState
	}

	err := mh.config.SaveModelAccessState(newState, slackCtx.UserID)
	if err != nil {
		return "", fmt.Errorf("saving model access state: %w", err)
	}

	mh.securityLogger.LogConfigChange(slackCtx.UserID, "model_access", "Cleared all restrictions")

	return "✅ All model access restrictions have been cleared. All models are now allowed.", nil
}

// handleModelAccessStatus shows model access status for a specific user
func (mh *MessageHandler) handleModelAccessStatus(ctx context.Context, slackCtx *SlackContext, targetUserID string) (string, error) {
	// Get target user info
	user, err := mh.connection.GetClient().GetUserInfo(targetUserID)
	if err != nil {
		return "", fmt.Errorf("getting user info: %w", err)
	}

	// Check if user is admin
	isAdmin := mh.tenantToolSet.IsAdmin(targetUserID)

	response := fmt.Sprintf("👤 **Model Access Status for @%s**\n\n", user.Name)

	if isAdmin {
		response += "👑 **Administrator** - Can bypass all model access restrictions.\n"
	} else {
		response += "👤 **Regular User** - Subject to model access restrictions.\n"
	}

	// Show current model configuration
	model := mh.config.LanguageModel()
	allowed, reason := mh.config.ValidateModelAccess(model, targetUserID)

	response += fmt.Sprintf("🤖 **Current Model:** %s\n", model)

	if allowed {
		response += "✅ **Access:** Allowed\n"
	} else {
		response += fmt.Sprintf("❌ **Access:** Denied\n📝 **Reason:** %s\n", reason)
		response += fmt.Sprintf("🔄 **Fallback:** Would use %s\n", config.DefaultLanguageModel)
	}

	return response, nil
}

// handleAdminIntent provides admin-specific help and escalation
func (mh *MessageHandler) handleAdminIntent(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// Verify admin permissions
	if !mh.tenantToolSet.IsAdmin(slackCtx.UserID) {
		_, _, err := mh.connection.client.PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText("❌ Only admins can use admin commands.", true),
		)
		return err
	}

	// Create help integrator if available
	var helpIntegrator *HelpIntegrator
	if mh.helpAnalyzer != nil && mh.helpContextBuilder != nil {
		helpIntegrator = NewHelpIntegrator(mh.helpAnalyzer, mh.helpContextBuilder)
	}

	switch intent.Action {
	case "admin_help":
		if helpIntegrator != nil {
			request, _ := intent.Config.(string)
			analysis, err := helpIntegrator.HandleAdminRequest(ctx, slackCtx.UserID, slackCtx.ChannelID, request)
			if err != nil {
				mh.securityLogger.LogError(slackCtx.UserID, "admin_help", fmt.Sprintf("Failed to analyze admin request: %v", err))
				_, _, err := mh.connection.client.PostMessageContext(
					ctx,
					slackCtx.ChannelID,
					slack.MsgOptionText("❌ Error processing admin help request.", true),
				)
				return err
			}

			if ShouldShowHelp(analysis) {
				helpResponse := helpIntegrator.CreateHelpResponse(analysis)
				_, _, err := mh.connection.client.PostMessageContext(
					ctx,
					slackCtx.ChannelID,
					slack.MsgOptionText(helpResponse.Text, true),
				)
				return err
			}
		}

		// Fallback admin help
		fallbackHelp := "👑 **Admin Help**\n\n" +
			"Here are some admin commands you can use:\n\n" +
			"• `list pending requests` - See tool approval requests\n" +
			"• `approve tool <request-id>` - Approve a tool request\n" +
			"• `reject tool <request-id>` - Reject a tool request\n" +
			"• `model access list` - Show model access settings\n" +
			"• `allow model <model-name>` - Allow a model\n" +
			"• `deny model <model-name>` - Deny a model\n" +
			"• `admin help <topic>` - Get admin-specific help\n" +
			"• `escalate <issue>` - Escalate to support"

		_, _, err := mh.connection.client.PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText(fallbackHelp, true),
		)
		return err

	case "admin_escalation":
		issue, _ := intent.Config.(string)
		escalationMessage := fmt.Sprintf("🚨 **Admin Escalation**\n\n**User:** @%s\n**Issue:** %s\n\n"+
			"This escalation has been logged and support will contact you shortly.", slackCtx.UserID, issue)

		mh.securityLogger.LogAdminAction(slackCtx.UserID, "escalation", issue)

		_, _, err := mh.connection.client.PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText(escalationMessage, true),
		)
		return err

	default:
		_, _, err := mh.connection.client.PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText("❌ Unknown admin command.", true),
		)
		return err
	}
}

// handleIntentFailure provides intelligent help when intent recognition fails
func (mh *MessageHandler) handleIntentFailure(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, ev *slackevents.MessageEvent) error {
	preferences := mh.sessionManager.ResolveUserPreferences(session.UserID, mh.config)
	updater := NewSlackUpdater(mh.connection.client, ev.Channel, NewSlackFormatter(), preferences)

	// Check if help system is enabled for this failure type
	if !mh.config.HelpSystemEnabled() || !mh.config.HelpSystemShouldHelpOnIntentFailure() {
		// Only send basic help if help system is enabled but intent failure help is disabled
		if mh.config.HelpSystemEnabled() {
			mh.sendBasicHelp(ctx, updater, message)
			return updater.Flush(ctx)
		}
		// Help system completely disabled - don't send anything
		return nil
	}

	// If HelpAnalyzer and HelpContextBuilder are available, use intelligent analysis
	if mh.helpAnalyzer != nil && mh.helpContextBuilder != nil {
		// Build comprehensive help context
		helpCtx := mh.helpContextBuilder.BuildContext(ctx, slackCtx.UserID, slackCtx.ChannelID, message)

		// Analyze the intent failure
		analysis, err := mh.helpAnalyzer.AnalyzeIntentFailure(ctx, message, helpCtx)
		if err != nil {
			// Fallback to basic help if analysis fails
			mh.sendBasicHelp(ctx, updater, message)
			return fmt.Errorf("help analysis failed: %w", err)
		}

		// Check if analysis confidence meets minimum threshold
		if analysis.Confidence < float64(mh.config.HelpSystemMinConfidenceThreshold()) {
			mh.sendBasicHelp(ctx, updater, message)
			return nil
		}

		// Format and send the intelligent help response
		helpMessage := mh.formatHelpMessage(analysis)
		if err := updater.AddContent(ctx, helpMessage); err != nil {
			return fmt.Errorf("sending help message: %w", err)
		}
	} else {
		// Fallback to basic help when intelligent help components unavailable
		mh.sendBasicHelp(ctx, updater, message)
	}

	if err := updater.Flush(ctx); err != nil {
		return fmt.Errorf("flushing help message: %w", err)
	}

	return nil
}

// sendBasicHelp sends a basic help message when intelligent help is unavailable
func (mh *MessageHandler) sendBasicHelp(ctx context.Context, updater *SlackUpdater, message string) error {
	fallbackMessage := "🤖 I'm not sure what you're trying to do. Here are some things you can ask me:\n\n" +
		"• `list my tools` - See available tools\n" +
		"• `add http tool at <url>` - Add a new HTTP tool\n" +
		"• `show preferences` - Display your current settings\n" +
		"• `thinking on/off` - Toggle thinking display\n" +
		"• Just ask me anything! - I can help with many tasks\n\n" +
		"If you were trying a specific command, I can provide more targeted help."

	return updater.AddContent(ctx, fallbackMessage)
}

// formatHelpMessage formats a help analysis into a user-friendly message
func (mh *MessageHandler) formatHelpMessage(analysis *HelpAnalysis) string {
	var builder strings.Builder

	// Main diagnosis with emoji
	builder.WriteString("🤖 **Intelligent Help**\n\n")
	builder.WriteString(fmt.Sprintf("**Issue:** %s\n\n", analysis.Diagnosis))

	// Suggestions
	if len(analysis.Suggestions) > 0 {
		builder.WriteString("💡 **Suggestions:**\n")
		for i, suggestion := range analysis.Suggestions {
			builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
		}
		builder.WriteString("\n")
	}

	// Examples
	if len(analysis.Examples) > 0 {
		builder.WriteString("📋 **Examples:**\n")
		for _, example := range analysis.Examples {
			builder.WriteString(fmt.Sprintf("• `%s`\n", example))
		}
		builder.WriteString("\n")
	}

	// Additional context
	if analysis.ContextHelp != "" {
		builder.WriteString("ℹ️ **Additional Help:**\n")
		builder.WriteString(fmt.Sprintf("%s\n\n", analysis.ContextHelp))
	}

	return builder.String()
}

// LogSessionEvent logs a session event
func (mh *MessageHandler) LogSessionEvent(userID, channelID, event string) {
	mh.securityLogger.LogSessionEvent(userID, channelID, event)
}
