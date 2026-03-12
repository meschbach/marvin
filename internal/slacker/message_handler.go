package slacker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := tracer.Start(ctx, "MessageHandler.ProcessMessage",
		trace.WithAttributes(
			attribute.String("channel", ev.Channel),
			attribute.String("user", ev.User),
		))
	defer span.End()

	if !mh.shouldProcessMessage(ctx, ev) {
		return nil
	}

	cleanMessage := mh.extractBotMention(ev)
	if cleanMessage == "" {
		return nil
	}

	slackCtx, err := mh.createSlackContext(ev, cleanMessage)
	if err != nil {
		return err
	}

	session := mh.getOrCreateUserSession(ev, slackCtx)

	if err := mh.ensureToolsInitialized(ctx, slackCtx); err != nil {
		return err
	}

	return mh.routeMessage(ctx, ev, slackCtx, session, cleanMessage)
}

// shouldProcessMessage determines if the message should be processed
func (mh *MessageHandler) shouldProcessMessage(ctx context.Context, ev *slackevents.MessageEvent) bool {
	_, span := tracer.Start(ctx, "MessageHandler.shouldProcessMessage")
	defer span.End()

	shouldProcess := ev.BotID == "" && ev.SubType == "" && ev.Text != "" &&
		(ev.ChannelType == "im" || strings.Contains(ev.Text, fmt.Sprintf("<@%s>", mh.connection.GetBotUserID())))

	span.SetAttributes(attribute.Bool("shouldProcess", shouldProcess))
	return shouldProcess
}

// extractBotMention removes the bot mention from the message and returns clean text
func (mh *MessageHandler) extractBotMention(ev *slackevents.MessageEvent) string {
	mention := fmt.Sprintf("<@%s>", mh.connection.GetBotUserID())
	cleanMessage := strings.ReplaceAll(ev.Text, mention, "")
	return strings.TrimSpace(cleanMessage)
}

// createSlackContext creates a SlackContext from a message event
func (mh *MessageHandler) createSlackContext(ev *slackevents.MessageEvent, cleanMessage string) (*SlackContext, error) {
	slackCtx := &SlackContext{
		UserID:    ev.User,
		ChannelID: ev.Channel,
		Message:   cleanMessage,
		Timestamp: ev.TimeStamp,
		ThreadTS:  ev.ThreadTimeStamp,
		TeamID:    "",
	}

	user, err := mh.connection.GetClient().GetUserInfo(ev.User)
	if err == nil {
		slackCtx.UserName = user.Name
	}

	mh.securityLogger.LogSessionEvent(ev.User, ev.Channel, "Message received")

	return slackCtx, nil
}

// getOrCreateUserSession gets or creates a user session
func (mh *MessageHandler) getOrCreateUserSession(ev *slackevents.MessageEvent, slackCtx *SlackContext) *UserSession {
	userCtx := &query.UserContext{
		UserID:      ev.User,
		SlackTeamID: slackCtx.TeamID,
		IsAdmin:     false,
	}
	return mh.sessionManager.GetOrCreateSession(ev.User, ev.Channel, userCtx)
}

// ensureToolsInitialized initializes tools if needed
func (mh *MessageHandler) ensureToolsInitialized(ctx context.Context, slackCtx *SlackContext) error {
	if mh.tenantToolSet.IsInitialized() {
		return nil
	}

	_, _, err := mh.connection.client.PostMessageContext(
		ctx,
		slackCtx.ChannelID,
		slack.MsgOptionText("🔧 Initializing tools for first use, please wait...", true),
	)
	if err != nil {
		mh.securityLogger.LogError(slackCtx.UserID, "ToolInit", fmt.Sprintf("Failed to send init notification: %v", err))
	}

	initCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	initErr := mh.tenantToolSet.Initialize(initCtx)
	if initErr != nil {
		var initErrMsg string
		if errors.Is(initErr, context.DeadlineExceeded) {
			initErrMsg = "❌ Tool initialization timed out (4s). Please try again."
		} else {
			initErrMsg = fmt.Sprintf("❌ Tool initialization failed: %v", initErr)
		}

		mh.securityLogger.LogError(slackCtx.UserID, "ToolInit", initErrMsg)
		_, _, err = mh.connection.client.PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText(initErrMsg, true),
		)
		if err != nil {
			mh.securityLogger.LogError(slackCtx.UserID, "ToolInit", fmt.Sprintf("Failed to send error notification: %v", err))
		}
		return fmt.Errorf("tool initialization failed: %w", initErr)
	}

	_, _, err = mh.connection.client.PostMessageContext(
		ctx,
		slackCtx.ChannelID,
		slack.MsgOptionText("✅ Tools ready!", true),
	)
	if err != nil {
		mh.securityLogger.LogError(slackCtx.UserID, "ToolInit", fmt.Sprintf("Failed to send success notification: %v", err))
	}

	return nil
}

// routeMessage routes the message to the appropriate handler
func (mh *MessageHandler) routeMessage(ctx context.Context, ev *slackevents.MessageEvent, slackCtx *SlackContext, session *UserSession, message string) error {
	ctx, span := tracer.Start(ctx, "MessageHandler.routeMessage")
	defer span.End()

	intent, err := mh.intentProcessor.ProcessMessage(message)
	if err != nil {
		return fmt.Errorf("processing intent: %w", err)
	}

	if intent != nil {
		span.SetAttributes(
			attribute.String("intent.action", intent.Action),
			attribute.Float64("intent.confidence", intent.Confidence),
		)
	}

	if intent != nil && intent.Confidence >= 0.7 {
		return mh.handleHighConfidenceIntent(ctx, slackCtx, session, intent)
	}

	if intent != nil && intent.Confidence < 0.7 {
		return mh.handleIntentFailure(ctx, slackCtx, session, message, ev)
	}

	return mh.handleQuery(ctx, ev, slackCtx, session, message)
}

// handleHighConfidenceIntent routes high-confidence intents to appropriate handlers
func (mh *MessageHandler) handleHighConfidenceIntent(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	ctx, span := tracer.Start(ctx, "MessageHandler.handleHighConfidenceIntent",
		trace.WithAttributes(
			attribute.String("intent.action", intent.Action),
		))
	defer span.End()

	if mh.isPreferenceIntent(intent) {
		return mh.handlePreferenceIntent(ctx, slackCtx, session, intent)
	}

	if strings.HasPrefix(intent.Action, "model_access_") {
		return mh.handleModelAccessIntent(ctx, slackCtx, session, intent)
	}

	if strings.HasPrefix(intent.Action, "admin_") {
		return mh.handleAdminIntent(ctx, slackCtx, session, intent)
	}

	return mh.toolManager.HandleToolIntent(ctx, slackCtx, session, intent)
}

// isPreferenceIntent determines if the intent is a preference management intent
func (mh *MessageHandler) isPreferenceIntent(intent *ToolManagementIntent) bool {
	return strings.Contains(intent.Action, "thinking") ||
		strings.Contains(intent.Action, "tools") ||
		strings.Contains(intent.Action, "done") ||
		strings.Contains(intent.Action, "verbose") ||
		intent.Action == "show_preferences"
}

// handleQuery processes a regular query
func (mh *MessageHandler) handleQuery(ctx context.Context, ev *slackevents.MessageEvent, slackCtx *SlackContext, session *UserSession, message string) error {
	ctx, span := tracer.Start(ctx, "MessageHandler.handleQuery",
		trace.WithAttributes(
			attribute.String("user", slackCtx.UserID),
			attribute.String("channel", slackCtx.ChannelID),
		))
	defer span.End()

	preferences := mh.sessionManager.ResolveUserPreferences(session.UserID, mh.config)
	updater := NewSlackUpdater(mh.connection.client, ev.Channel, NewSlackFormatter(), preferences)
	queryError := mh.queryHandler.HandleQueryWithUpdater(ctx, slackCtx, session, message, updater)
	return queryError
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

	return formatModelAccessResponse(state), nil
}

// formatModelAccessResponse formats model access state into a response string
func formatModelAccessResponse(state *config.ModelAccessState) string {
	var response strings.Builder
	fmt.Fprintf(&response, "🤖 **Model Access Configuration**\n\n")

	hasNoRestrictions := len(state.AllowedModels) == 0 && len(state.DeniedModels) == 0
	if hasNoRestrictions {
		response.WriteString("No restrictions in place - all models are allowed.\n")
	} else {
		formatModelList(&response, state.AllowedModels, "✅ **Allowed Models:**")
		formatModelList(&response, state.DeniedModels, "❌ **Denied Models:**")
	}

	fmt.Fprintf(&response, "\n🔧 **Default Model:** %s\n", state.DefaultModel)

	if state.UpdatedBy != "" && state.LastUpdated != "" {
		fmt.Fprintf(&response, "📝 **Last Updated:** %s by %s\n", state.LastUpdated, state.UpdatedBy)
	}

	return response.String()
}

func formatModelList(builder *strings.Builder, models []string, header string) {
	if len(models) == 0 {
		return
	}
	fmt.Fprintf(builder, "%s\n", header)
	for _, model := range models {
		fmt.Fprintf(builder, "  • %s\n", model)
	}
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
	if !mh.tenantToolSet.IsAdmin(slackCtx.UserID) {
		_, _, err := mh.connection.client.PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText("❌ Only admins can use admin commands.", true),
		)
		return err
	}

	switch intent.Action {
	case "admin_help":
		return mh.handleAdminHelp(ctx, slackCtx, intent)
	case "admin_escalation":
		return mh.handleAdminEscalation(ctx, slackCtx, intent)
	default:
		_, _, err := mh.connection.client.PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText("❌ Unknown admin command.", true),
		)
		return err
	}
}

func (mh *MessageHandler) handleAdminHelp(ctx context.Context, slackCtx *SlackContext, intent *ToolManagementIntent) error {
	var helpIntegrator *HelpIntegrator
	if mh.helpAnalyzer != nil && mh.helpContextBuilder != nil {
		helpIntegrator = NewHelpIntegrator(mh.helpAnalyzer, mh.helpContextBuilder)
	}

	if helpIntegrator != nil {
		request, convertable := intent.Config.(string)
		if !convertable {
			return errors.New("config not convertable to string")
		}
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

	return mh.sendFallbackAdminHelp(ctx, slackCtx)
}

func (mh *MessageHandler) sendFallbackAdminHelp(ctx context.Context, slackCtx *SlackContext) error {
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
}

func (mh *MessageHandler) handleAdminEscalation(ctx context.Context, slackCtx *SlackContext, intent *ToolManagementIntent) error {
	issue, convertible := intent.Config.(string)
	if !convertible {
		return errors.New("config is not a string")
	}
	escalationMessage := fmt.Sprintf("🚨 **Admin Escalation**\n\n**User:** @%s\n**Issue:** %s\n\n"+
		"This escalation has been logged and support will contact you shortly.", slackCtx.UserID, issue)

	mh.securityLogger.LogAdminAction(slackCtx.UserID, "escalation", issue)

	_, _, err := mh.connection.client.PostMessageContext(
		ctx,
		slackCtx.ChannelID,
		slack.MsgOptionText(escalationMessage, true),
	)
	return err
}

// handleIntentFailure provides intelligent help when intent recognition fails
func (mh *MessageHandler) handleIntentFailure(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, ev *slackevents.MessageEvent) error {
	ctx, span := tracer.Start(ctx, "MessageHandler.handleIntentFailure",
		trace.WithAttributes(
			attribute.String("message", message),
		))
	defer span.End()

	updater := mh.createHelpUpdater(session, ev)

	if !mh.shouldShowHelpOnIntentFailure() {
		return mh.handleHelpDisabled(ctx, updater)
	}

	if mh.helpAnalyzer != nil && mh.helpContextBuilder != nil {
		return mh.handleIntelligentHelp(ctx, slackCtx, updater, message)
	}

	return mh.sendBasicHelpAndFlush(ctx, updater, message)
}

func (mh *MessageHandler) createHelpUpdater(session *UserSession, ev *slackevents.MessageEvent) *SlackUpdater {
	preferences := mh.sessionManager.ResolveUserPreferences(session.UserID, mh.config)
	return NewSlackUpdater(mh.connection.client, ev.Channel, NewSlackFormatter(), preferences)
}

func (mh *MessageHandler) shouldShowHelpOnIntentFailure() bool {
	return mh.config.HelpSystemEnabled() && mh.config.HelpSystemShouldHelpOnIntentFailure()
}

func (mh *MessageHandler) handleHelpDisabled(ctx context.Context, updater *SlackUpdater) error {
	if mh.config.HelpSystemEnabled() {
		if err := mh.sendBasicHelp(ctx, updater, ""); err != nil {
			return fmt.Errorf("failed to send basic help: %w", err)
		}
		return updater.Flush(ctx)
	}
	return nil
}

func (mh *MessageHandler) handleIntelligentHelp(ctx context.Context, slackCtx *SlackContext, updater *SlackUpdater, message string) error {
	helpCtx := mh.helpContextBuilder.BuildContext(ctx, slackCtx.UserID, slackCtx.ChannelID, message)

	analysis, err := mh.helpAnalyzer.AnalyzeIntentFailure(ctx, message, helpCtx)
	if err != nil {
		if sendErr := mh.sendBasicHelp(ctx, updater, message); sendErr != nil {
			return fmt.Errorf("failed to send basic help: %w", sendErr)
		}
		return fmt.Errorf("help analysis failed: %w", err)
	}

	if analysis.Confidence < float64(mh.config.HelpSystemMinConfidenceThreshold()) {
		if err := mh.sendBasicHelp(ctx, updater, message); err != nil {
			return fmt.Errorf("failed to send basic help: %w", err)
		}
		return nil
	}

	helpMessage := mh.formatHelpMessage(analysis)
	if err := updater.AddContent(ctx, helpMessage); err != nil {
		return fmt.Errorf("sending help message: %w", err)
	}

	if err := updater.Flush(ctx); err != nil {
		return fmt.Errorf("flushing help message: %w", err)
	}

	return nil
}

func (mh *MessageHandler) sendBasicHelpAndFlush(ctx context.Context, updater *SlackUpdater, message string) error {
	if err := mh.sendBasicHelp(ctx, updater, message); err != nil {
		return fmt.Errorf("failed to send basic help: %w", err)
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

	fmt.Fprintf(&builder, "🤖 **Intelligent Help**\n\n")
	fmt.Fprintf(&builder, "**Issue:** %s\n\n", analysis.Diagnosis)

	if len(analysis.Suggestions) > 0 {
		builder.WriteString("💡 **Suggestions:**\n")
		for i, suggestion := range analysis.Suggestions {
			fmt.Fprintf(&builder, "%d. %s\n", i+1, suggestion)
		}
		builder.WriteString("\n")
	}

	if len(analysis.Examples) > 0 {
		builder.WriteString("📋 **Examples:**\n")
		for _, example := range analysis.Examples {
			fmt.Fprintf(&builder, "• `%s`\n", example)
		}
		builder.WriteString("\n")
	}

	if analysis.ContextHelp != "" {
		builder.WriteString("ℹ️ **Additional Help:**\n")
		fmt.Fprintf(&builder, "%s\n\n", analysis.ContextHelp)
	}

	return builder.String()
}

// LogSessionEvent logs a session event
func (mh *MessageHandler) LogSessionEvent(userID, channelID, event string) {
	mh.securityLogger.LogSessionEvent(userID, channelID, event)
}
