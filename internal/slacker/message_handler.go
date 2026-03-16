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
	SlackClient      SlackClientAPI
	Config           *config.File
	SessionManager   *SessionManager
	Connection       *SlackConnection
	ApprovalWorkflow *ApprovalWorkflow
	TenantToolSet    *query.TenantToolSet
	SecurityLogger   *sec.SecurityLogger
}

// MessageHandler processes incoming Slack messages and intents
type MessageHandler struct {
	connection       *SlackConnection
	queryHandler     QueryHandler
	approvalWorkflow *ApprovalWorkflow
	sessionManager   *SessionManager
	securityLogger   *sec.SecurityLogger
	config           *config.File
	tenantToolSet    *query.TenantToolSet
	commandRegistry  CommandRegistry
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(
	connection *SlackConnection,
	queryHandler QueryHandler,
	approvalWorkflow *ApprovalWorkflow,
	sessionManager *SessionManager,
	securityLogger *sec.SecurityLogger,
	config *config.File,
	tenantToolSet *query.TenantToolSet,
) *MessageHandler {
	mh := &MessageHandler{
		connection:       connection,
		queryHandler:     queryHandler,
		approvalWorkflow: approvalWorkflow,
		sessionManager:   sessionManager,
		securityLogger:   securityLogger,
		config:           config,
		tenantToolSet:    tenantToolSet,
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
	registry.Register("done", mh.wrapCommandsHandler(commands.HandleDone))
	registry.Register("verbose", mh.wrapCommandsHandler(commands.HandleVerbose))
	registry.Register("preferences", mh.wrapCommandsHandler(commands.HandlePreferences))
	registry.Register("admin", mh.wrapCommandsHandler(commands.HandleAdminHelp))
	registry.Register("model access", mh.wrapCommandsHandler(commands.HandleModelAccess))

	mh.commandRegistry = registry

	return mh
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

func (s *securityLoggerAdapter) LogConfigChange(userID, configType, details string) {
	s.sl.LogConfigChange(userID, configType, details)
}

func (s *securityLoggerAdapter) LogAdminAction(adminID, action, target string) {
	s.sl.LogAdminAction(adminID, action, target)
}

// sessionManagerAdapter implements commands.SessionManager
type sessionManagerAdapter struct {
	sm *SessionManager
}

func (s *sessionManagerAdapter) ClearSession(userID, channelID string) error {
	return s.sm.ClearSession(userID, channelID)
}

func (s *sessionManagerAdapter) GetPreferences(userID string) (commands.UserPreferences, bool) {
	prefs, found := s.sm.GetPreferences(userID)
	if !found {
		return commands.UserPreferences{}, false
	}
	return commands.UserPreferences{
		ShowThinking:   prefs.ShowThinking,
		ShowTools:      prefs.ShowTools,
		ShowDone:       prefs.ShowDone,
		ThinkingFormat: prefs.ThinkingFormat,
		ToolFormat:     prefs.ToolFormat,
		Verbose:        prefs.Verbose,
	}, true
}

func (s *sessionManagerAdapter) UpdatePreferences(userID string, preferences commands.UserPreferences) error {
	return s.sm.UpdatePreferences(userID, UserPreferences{
		ShowThinking:   preferences.ShowThinking,
		ShowTools:      preferences.ShowTools,
		ShowDone:       preferences.ShowDone,
		ThinkingFormat: preferences.ThinkingFormat,
		ToolFormat:     preferences.ToolFormat,
		Verbose:        preferences.Verbose,
	})
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

	_, _, err := mh.connection.GetClient().PostMessageContext(
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
		_, _, err = mh.connection.GetClient().PostMessageContext(
			ctx,
			slackCtx.ChannelID,
			slack.MsgOptionText(initErrMsg, true),
		)
		if err != nil {
			mh.securityLogger.LogError(slackCtx.UserID, "ToolInit", fmt.Sprintf("Failed to send error notification: %v", err))
		}
		return fmt.Errorf("tool initialization failed: %w", initErr)
	}

	_, _, err = mh.connection.GetClient().PostMessageContext(
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
		_, _, err := mh.connection.GetClient().PostMessageContext(
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
			SessionManager:   mh.sessionManager,
			Connection:       mh.connection,
			ApprovalWorkflow: mh.approvalWorkflow,
			TenantToolSet:    mh.tenantToolSet,
			SecurityLogger:   mh.securityLogger,
		}
		return handler(ctx, deps, command)
	}

	mh.securityLogger.LogError(ev.User, "Command", fmt.Sprintf("Unauthorized admin command: %s", cmdName))
	_, _, err := mh.connection.GetClient().PostMessageContext(
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

	_, _, err := mh.connection.GetClient().PostMessageContext(
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

	// First try to match against the command registry
	if cmd, handler, ok := mh.commandRegistry.Match(message); ok {
		span.SetAttributes(attribute.String("command.matched", cmd))
		deps := &CommandDeps{
			ChannelID:        slackCtx.ChannelID,
			UserID:           slackCtx.UserID,
			SlackClient:      mh.connection.client,
			Config:           mh.config,
			SessionManager:   mh.sessionManager,
			Connection:       mh.connection,
			ApprovalWorkflow: mh.approvalWorkflow,
			TenantToolSet:    mh.tenantToolSet,
			SecurityLogger:   mh.securityLogger,
		}
		return handler(ctx, deps, message)
	}

	// No command matched, treat as a regular query
	return mh.handleQuery(ctx, ev, slackCtx, session, message)
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

// LogSessionEvent logs a session event
func (mh *MessageHandler) LogSessionEvent(userID, channelID, event string) {
	mh.securityLogger.LogSessionEvent(userID, channelID, event)
}
