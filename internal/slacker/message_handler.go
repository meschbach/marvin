package slacker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/meschbach/marvin/internal/slacker/commands"
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

// CommandHandler defines the interface for handling commands
type CommandHandler func(ctx context.Context, deps *CommandDeps, msg string) error

// CommandRegistry defines the interface for command registration and matching
type CommandRegistry interface {
	Match(input string) (string, CommandHandler, bool)
}

// SimpleCommandRegistry is a basic implementation of CommandRegistry
type SimpleCommandRegistry struct {
	handlers map[string]CommandHandler
}

func NewSimpleCommandRegistry() *SimpleCommandRegistry {
	return &SimpleCommandRegistry{
		handlers: make(map[string]CommandHandler),
	}
}

func (r *SimpleCommandRegistry) Register(name string, handler CommandHandler) {
	r.handlers[name] = handler
}

func (r *SimpleCommandRegistry) Match(input string) (string, CommandHandler, bool) {
	if input == "" {
		return "", nil, false
	}

	input = strings.ToLower(strings.TrimSpace(input))

	longestMatch := ""
	var handler CommandHandler

	for cmd := range r.handlers {
		if strings.HasPrefix(input, cmd) {
			if len(cmd) > len(longestMatch) {
				longestMatch = cmd
				handler = r.handlers[cmd]
			}
		}
	}

	if handler != nil {
		return longestMatch, handler, true
	}
	return "", nil, false
}

// CommandDeps provides dependencies for command handlers
type CommandDeps struct {
	ChannelID        string
	UserID           string
	SlackClient      *slack.Client
	Config           *config.File
	ToolManager      *ToolManagerImpl
	SessionManager   *SessionManager
	Connection       *SlackConnection
	ApprovalWorkflow *ApprovalWorkflow
	TenantToolSet    *query.TenantToolSet
	SecurityLogger   *sec.SecurityLogger
}

// MessageHandler processes incoming Slack messages and intents
type MessageHandler struct {
	intentProcessor *IntentProcessor
	connection      *SlackConnection
	queryHandler    QueryHandler
	toolManager     *ToolManagerImpl
	sessionManager  *SessionManager
	securityLogger  *sec.SecurityLogger
	config          *config.File
	tenantToolSet   *query.TenantToolSet
	commandRegistry CommandRegistry
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
	mh := &MessageHandler{
		intentProcessor: intentProcessor,
		connection:      connection,
		queryHandler:    queryHandler,
		toolManager:     toolManager,
		sessionManager:  sessionManager,
		securityLogger:  securityLogger,
		config:          config,
		tenantToolSet:   tenantToolSet,
	}

	registry := NewSimpleCommandRegistry()
	registry.Register("help", mh.wrapCommandsHandler(commands.HandleHelp))
	registry.Register("tools", mh.wrapCommandsHandler(commands.HandleTools))
	registry.Register("list tools", mh.wrapCommandsHandler(commands.HandleListTools))
	registry.Register("add tool", mh.wrapCommandsHandler(commands.HandleAddTool))
	registry.Register("remove tool", mh.wrapCommandsHandler(commands.HandleRemoveTool))
	registry.Register("reset session", mh.wrapCommandsHandler(commands.HandleResetSession))
	registry.Register("approve", mh.wrapCommandsHandler(commands.HandleApprove))
	registry.Register("reject", mh.wrapCommandsHandler(commands.HandleReject))
	registry.Register("share tool", mh.wrapCommandsHandler(commands.HandleShareTool))
	registry.Register("thinking", mh.wrapCommandsHandler(commands.HandleThinking))
	registry.Register("preferences", mh.wrapCommandsHandler(commands.HandlePreferences))
	registry.Register("admin", mh.wrapCommandsHandler(commands.HandleAdminHelp))
	registry.Register("model access", mh.wrapCommandsHandler(commands.HandleModelAccess))

	mh.commandRegistry = registry

	return mh
}

// wrapCommandHandler wraps a command handler function to work with the registry
func (mh *MessageHandler) wrapCommandHandler(handler func(ctx context.Context, deps *CommandDeps, msg string) error) CommandHandler {
	return handler
}

// wrapCommandsHandler wraps a commands handler to work with CommandDeps
func (mh *MessageHandler) wrapCommandsHandler(handler func(ctx context.Context, deps *commands.CommandsDependencies, msg string) error) CommandHandler {
	return func(ctx context.Context, deps *CommandDeps, msg string) error {
		return handler(ctx, mh.commandDepsToCommandsDepsAdapter(deps), msg)
	}
}

// commandDepsToCommandsDepsAdapter converts CommandDeps to commands.CommandsDependencies
func (mh *MessageHandler) commandDepsToCommandsDepsAdapter(deps *CommandDeps) *commands.CommandsDependencies {
	return &commands.CommandsDependencies{
		Context: &slackContextAdapter{
			userID:    deps.UserID,
			userName:  "", // TODO: populate if needed
			channelID: deps.ChannelID,
			teamID:    "",
		},
		ApprovalWorkflow: &approvalWorkflowAdapter{deps.ApprovalWorkflow},
		TenantToolSet:    deps.TenantToolSet,
		ToolSet:          &toolSetAdapter{tts: deps.TenantToolSet},
		SecurityLogger:   &securityLoggerAdapter{deps.SecurityLogger},
		SessionManager:   &sessionManagerAdapter{deps.SessionManager},
		SlackClient:      deps.SlackClient,
		Config:           deps.Config,
		Connection:       &slackConnectionAdapter{deps.Connection},
		ToolParser:       &toolParserAdapter{},
	}
}

// slackContextAdapter implements commands.Context
type slackContextAdapter struct {
	userID    string
	userName  string
	channelID string
	teamID    string
}

func (s *slackContextAdapter) UserID() string    { return s.userID }
func (s *slackContextAdapter) UserName() string  { return s.userName }
func (s *slackContextAdapter) ChannelID() string { return s.channelID }
func (s *slackContextAdapter) TeamID() string    { return s.teamID }

// slackConnectionAdapter implements commands.Connection
type slackConnectionAdapter struct {
	conn *SlackConnection
}

func (s *slackConnectionAdapter) GetBotUserID() string { return s.conn.GetBotUserID() }

// approvalWorkflowAdapter implements commands.ApprovalWorkflow
type approvalWorkflowAdapter struct {
	aw *ApprovalWorkflow
}

func (a *approvalWorkflowAdapter) RequestToolApproval(ctx context.Context, request *commands.ToolApprovalRequest) (string, error) {
	toolReq := &ToolApprovalRequest{
		ToolID:        request.ToolID,
		RequesterID:   request.RequesterID,
		ToolType:      request.ToolType,
		Config:        request.Config,
		RequesterName: request.RequesterName,
	}
	return a.aw.RequestToolApproval(ctx, toolReq)
}

func (a *approvalWorkflowAdapter) ApproveTool(ctx context.Context, approverID, requestID, reason string) error {
	return a.aw.ApproveTool(ctx, approverID, requestID, reason)
}

func (a *approvalWorkflowAdapter) RejectTool(ctx context.Context, approverID, requestID, reason string) error {
	return a.aw.RejectTool(ctx, approverID, requestID, reason)
}

func (a *approvalWorkflowAdapter) IsAdmin(userID string) bool {
	return a.aw.IsAdmin(userID)
}

// securityLoggerAdapter implements commands.SecurityLogger
type securityLoggerAdapter struct {
	sl *sec.SecurityLogger
}

func (s *securityLoggerAdapter) LogError(userID, operation, message string) {
	s.sl.LogError(userID, operation, message)
}

func (s *securityLoggerAdapter) LogSessionEvent(userID, channelID, event string) {
	s.sl.LogSessionEvent(userID, channelID, event)
}

func (s *securityLoggerAdapter) LogToolRequest(userID, toolType, config string) {
	s.sl.LogToolRequest(userID, toolType, config)
}

func (s *securityLoggerAdapter) LogToolAdded(userID, toolID, toolType string) {
	s.sl.LogToolAdded(userID, toolID, toolType)
}

func (s *securityLoggerAdapter) LogToolRemoved(userID, toolID string) {
	s.sl.LogToolRemoved(userID, toolID)
}

func (s *securityLoggerAdapter) LogToolShare(userID, toolID, targetWorkspace string) {
	s.sl.LogToolShare(userID, toolID, targetWorkspace)
}

// sessionManagerAdapter implements commands.SessionManager
type sessionManagerAdapter struct {
	sm *SessionManager
}

func (s *sessionManagerAdapter) ClearSession(userID, channelID string) error {
	return s.sm.ClearSession(userID, channelID)
}

func (s *sessionManagerAdapter) GetOrCreateSession(ctx context.Context, userID, channelID string, userCtx *query.UserContext) (*commands.UserSession, error) {
	session := s.sm.GetOrCreateSession(ctx, userID, channelID, userCtx)
	return &commands.UserSession{
		UserID:         session.UserID,
		Context:        session.UserContext,
		AvailableTools: session.AvailableTools,
	}, nil
}

// toolParserAdapter implements commands.ToolParser
type toolParserAdapter struct{}

func (t *toolParserAdapter) ParseToolConfig(toolType, config string) (interface{}, error) {
	return ParseToolConfig(toolType, config)
}

func (t *toolParserAdapter) GenerateToolID(userID, toolType, name string) string {
	return GenerateToolID(userID, toolType, name)
}

// toolSetAdapter implements commands.ToolSet for TenantToolSet
type toolSetAdapter struct {
	tts *query.TenantToolSet
}

func (t *toolSetAdapter) ToolsForUser(ctx context.Context, userID string) ([]string, error) {
	return nil, nil // Not implemented - use GetUserTools instead
}

func (t *toolSetAdapter) AddTool(ctx context.Context, userID, toolID string, toolType string, config interface{}) error {
	return nil // Not implemented
}

func (t *toolSetAdapter) RemoveTool(ctx context.Context, userID, toolID string) error {
	return t.tts.RemoveUserTool(ctx, userID, toolID)
}

func (t *toolSetAdapter) ShareTool(ctx context.Context, toolID, targetUserID string) error {
	return t.tts.ShareToolToUser(ctx, toolID, targetUserID)
}

// SetCommandRegistry sets the command registry for command processing
func (mh *MessageHandler) SetCommandRegistry(registry CommandRegistry) {
	mh.commandRegistry = registry
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

	if mh.hasCommandPrefix(ev) {
		return mh.handleCommand(ctx, ev)
	}

	cleanMessage := mh.extractBotMention(ev)
	if cleanMessage == "" {
		return nil
	}

	slackCtx, err := mh.createSlackContext(ev, cleanMessage)
	if err != nil {
		return err
	}

	session := mh.getOrCreateUserSession(ctx, ev, slackCtx)

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

// hasCommandPrefix checks if the message has a command prefix in a DM
func (mh *MessageHandler) hasCommandPrefix(ev *slackevents.MessageEvent) bool {
	if ev.ChannelType != "im" {
		return false
	}
	mention := fmt.Sprintf("<@%s>", mh.connection.GetBotUserID())
	return strings.HasPrefix(ev.Text, mention)
}

// extractCommand extracts the command portion after the prefix
func (mh *MessageHandler) extractCommand(ev *slackevents.MessageEvent) string {
	mention := fmt.Sprintf("<@%s>", mh.connection.GetBotUserID())
	command := strings.ReplaceAll(ev.Text, mention, "")
	return strings.TrimSpace(command)
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
func (mh *MessageHandler) getOrCreateUserSession(ctx context.Context, ev *slackevents.MessageEvent, slackCtx *SlackContext) *UserSession {
	userCtx := &query.UserContext{
		UserID:      ev.User,
		SlackTeamID: slackCtx.TeamID,
		IsAdmin:     false,
	}
	return mh.sessionManager.GetOrCreateSession(ctx, ev.User, ev.Channel, userCtx)
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

// handleCommand processes a command from a DM with prefix
func (mh *MessageHandler) handleCommand(ctx context.Context, ev *slackevents.MessageEvent) error {
	ctx, span := tracer.Start(ctx, "MessageHandler.handleCommand")
	defer span.End()

	if mh.commandRegistry == nil {
		mh.securityLogger.LogError(ev.User, "Command", "Command registry not configured")
		_, _, err := mh.connection.client.PostMessageContext(
			ctx,
			ev.Channel,
			slack.MsgOptionText("⚠️ Commands not configured. Please contact an administrator.", true),
		)
		return err
	}

	command := mh.extractCommand(ev)
	mh.securityLogger.LogSessionEvent(ev.User, ev.Channel, fmt.Sprintf("Command: %s", command))

	cmdName, handler, found := mh.commandRegistry.Match(command)
	if !found {
		return mh.handleUnknownCommand(ctx, ev, command)
	}

	if !mh.isAdminCommand(cmdName) || mh.tenantToolSet.IsAdmin(ev.User) {
		deps := &CommandDeps{
			ChannelID:        ev.Channel,
			UserID:           ev.User,
			SlackClient:      mh.connection.client,
			Config:           mh.config,
			ToolManager:      mh.toolManager,
			SessionManager:   mh.sessionManager,
			Connection:       mh.connection,
			ApprovalWorkflow: mh.toolManager.GetApprovalWorkflow(),
			TenantToolSet:    mh.tenantToolSet,
			SecurityLogger:   mh.securityLogger,
		}
		return handler(ctx, deps, command)
	}

	mh.securityLogger.LogError(ev.User, "Command", fmt.Sprintf("Unauthorized admin command: %s", cmdName))
	_, _, err := mh.connection.client.PostMessageContext(
		ctx,
		ev.Channel,
		slack.MsgOptionText("❌ You don't have permission to run admin commands.", true),
	)
	return err
}

// isAdminCommand checks if a command requires admin permissions
func (mh *MessageHandler) isAdminCommand(cmdName string) bool {
	adminCommands := map[string]bool{
		"admin":        true,
		"model access": true,
		"add tool":     true,
		"remove tool":  true,
	}
	return adminCommands[cmdName]
}

// handleUnknownCommand shows help when command is not recognized
func (mh *MessageHandler) handleUnknownCommand(ctx context.Context, ev *slackevents.MessageEvent, command string) error {
	helpText := "⚠️ Unknown command. Here are available commands:\n\n" +
		"• `help` - Show this help message\n" +
		"• `tools` - List available tools\n" +
		"• `thinking` - Show current thinking preference\n" +
		"• `preferences` - Manage your preferences\n" +
		"• `reset session` - Reset the current conversation session\n\n" +
		"Admin commands:\n" +
		"• `admin` - Show admin help\n" +
		"• `model access` - Manage model access\n" +
		"• `add tool` - Add a new tool\n" +
		"• `remove tool` - Remove a tool\n"

	_, _, err := mh.connection.client.PostMessageContext(
		ctx,
		ev.Channel,
		slack.MsgOptionText(helpText, true),
	)
	return err
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

	return mh.sendBasicHelpAndFlush(ctx, updater, message)
}

func (mh *MessageHandler) createHelpUpdater(session *UserSession, ev *slackevents.MessageEvent) *SlackUpdater {
	preferences := mh.sessionManager.ResolveUserPreferences(session.UserID, mh.config)
	return NewSlackUpdater(mh.connection.client, ev.Channel, NewSlackFormatter(), preferences)
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

// Command handler implementations

func handleHelpCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	helpText := "🤖 **Available Commands:**\n\n" +
		"• `help` - Show this help message\n" +
		"• `tools` or `list tools` - List available tools\n" +
		"• `thinking` - Show current thinking preference\n" +
		"• `preferences` - Manage your preferences\n" +
		"• `reset session` - Reset the current conversation session\n\n" +
		"Admin commands:\n" +
		"• `admin` - Show admin help\n" +
		"• `model access` - Manage model access\n" +
		"• `add tool` - Add a new tool\n" +
		"• `remove tool` - Remove a tool\n" +
		"• `approve <request-id>` - Approve a tool request\n" +
		"• `reject <request-id>` - Reject a tool request\n"

	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText(helpText, false),
	)
	return err
}

func handleToolsCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	return handleListToolsCommand(ctx, deps, msg)
}

func handleListToolsCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	session := deps.SessionManager.GetOrCreateSession(ctx, deps.UserID, deps.Connection.GetBotUserID(), &query.UserContext{
		UserID:      deps.UserID,
		SlackTeamID: "",
		IsAdmin:     deps.TenantToolSet.IsAdmin(deps.UserID),
	})
	tools := session.GetAvailableTools()

	if len(tools) == 0 {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText("You don't have any tools available yet. Try adding an HTTP MCP tool!", false),
		)
		return err
	}

	var toolList strings.Builder
	toolList.WriteString("🔧 **Your Available Tools:**\n\n")

	for i, tool := range tools {
		fmt.Fprintf(&toolList, "%d. `%s`\n", i+1, tool)
	}

	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText(toolList.String(), false),
	)
	return err
}

func handleAddToolCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText("To add a tool, please use natural language like: \"add an HTTP MCP tool with config...\"", false),
	)
	return err
}

func handleRemoveToolCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText("🗑️ Tool removal requested. Feature coming soon!", false),
	)
	return err
}

func handleResetSessionCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	if err := deps.SessionManager.ClearSession(deps.UserID, deps.Connection.GetBotUserID()); err != nil {
		deps.SecurityLogger.LogError(deps.UserID, "session", fmt.Sprintf("Failed to reset session: %v", err))
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText(fmt.Sprintf("❌ Error resetting session: %v", err), false),
		)
		return err
	}

	deps.SecurityLogger.LogSessionEvent(deps.UserID, deps.Connection.GetBotUserID(), "Session reset by user")
	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText("✅ Your conversation history has been cleared", false),
	)
	return err
}

func handleApproveCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	if !deps.ApprovalWorkflow.IsAdmin(deps.UserID) {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText("❌ Only admins can approve/reject tool requests", false),
		)
		return err
	}

	parts := strings.Fields(msg)
	if len(parts) < 2 {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText("❌ Please specify a request ID", false),
		)
		return err
	}

	requestID := parts[1]
	if err := deps.ApprovalWorkflow.ApproveTool(ctx, deps.UserID, requestID, "Approved via command"); err != nil {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText(fmt.Sprintf("❌ Error approving request: %s", err.Error()), false),
		)
		return err
	}

	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText(fmt.Sprintf("✅ Tool request %s approved", requestID), false),
	)
	if err != nil {
		return err
	}
	return nil
}

func handleRejectCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	if !deps.ApprovalWorkflow.IsAdmin(deps.UserID) {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText("❌ Only admins can approve/reject tool requests", false),
		)
		return err
	}

	parts := strings.Fields(msg)
	if len(parts) < 2 {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText("❌ Please specify a request ID", false),
		)
		return err
	}

	requestID := parts[1]
	reason := "No reason provided"
	if len(parts) > 2 {
		reason = strings.Join(parts[2:], " ")
	}

	if err := deps.ApprovalWorkflow.RejectTool(ctx, deps.UserID, requestID, reason); err != nil {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText(fmt.Sprintf("❌ Error rejecting request: %s", err.Error()), false),
		)
		return err
	}

	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText(fmt.Sprintf("❌ Tool request %s rejected", requestID), false),
	)
	return err
}

func handleShareToolCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	deps.SecurityLogger.LogToolShare(deps.UserID, "", "")
	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText("🔄 Tool sharing requested. Feature coming soon!", false),
	)
	return err
}

func handleThinkingCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText("To change your thinking preference, please use natural language like: \"turn on thinking\" or \"turn off thinking\"", false),
	)
	return err
}

func handlePreferencesCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText("To manage your preferences, please use natural language like: \"show my preferences\"", false),
	)
	return err
}

func handleAdminCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	if !deps.TenantToolSet.IsAdmin(deps.UserID) {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText("❌ Only admins can use admin commands", false),
		)
		return err
	}

	adminHelp := "👑 **Admin Help**\n\n" +
		"Here are some admin commands you can use:\n\n" +
		"• `list pending requests` - See tool approval requests\n" +
		"• `approve tool <request-id>` - Approve a tool request\n" +
		"• `reject tool <request-id>` - Reject a tool request\n" +
		"• `model access list` - Show model access settings\n" +
		"• `allow model <model-name>` - Allow a model\n" +
		"• `deny model <model-name>` - Deny a model\n" +
		"• `admin help <topic>` - Get admin-specific help\n" +
		"• `escalate <issue>` - Escalate to support"

	_, _, err := deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText(adminHelp, false),
	)
	return err
}

func handleModelAccessCommand(ctx context.Context, deps *CommandDeps, msg string) error {
	if !deps.TenantToolSet.IsAdmin(deps.UserID) {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText("❌ Only admins can manage model access", false),
		)
		return err
	}

	state, err := deps.Config.GetEffectiveModelAccess()
	if err != nil {
		_, _, err := deps.SlackClient.PostMessage(
			deps.UserID,
			slack.MsgOptionText(fmt.Sprintf("❌ Error getting model access config: %s", err.Error()), false),
		)
		return err
	}

	var response strings.Builder
	fmt.Fprintf(&response, "🤖 **Model Access Configuration**\n\n")

	hasNoRestrictions := len(state.AllowedModels) == 0 && len(state.DeniedModels) == 0
	if hasNoRestrictions {
		response.WriteString("No restrictions in place - all models are allowed.\n")
	} else {
		if len(state.AllowedModels) > 0 {
			response.WriteString("✅ **Allowed Models:**\n")
			for _, model := range state.AllowedModels {
				fmt.Fprintf(&response, "  • %s\n", model)
			}
		}
		if len(state.DeniedModels) > 0 {
			response.WriteString("❌ **Denied Models:**\n")
			for _, model := range state.DeniedModels {
				fmt.Fprintf(&response, "  • %s\n", model)
			}
		}
	}

	fmt.Fprintf(&response, "\n🔧 **Default Model:** %s\n", state.DefaultModel)

	_, _, err = deps.SlackClient.PostMessage(
		deps.UserID,
		slack.MsgOptionText(response.String(), false),
	)
	return err
}
