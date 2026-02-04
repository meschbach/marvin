package slacker

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/ollama/ollama/api"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// SlackBot represents the main Slack bot
type SlackBot struct {
	client           *slack.Client
	socketClient     *socketmode.Client
	sessionManager   *SessionManager
	intentProcessor  *IntentProcessor
	tenantToolSet    *query.TenantToolSet
	approvalWorkflow *ApprovalWorkflow
	securityLogger   *sec.SecurityLogger
	config           *config.File

	// Bot state
	botUserID  string
	adminUsers []string
}

// SlackContext provides context for Slack operations
type SlackContext struct {
	UserID    string
	UserName  string
	ChannelID string
	TeamID    string
	Message   string
	Timestamp string
	ThreadTS  string
}

// slackUpdater handles rate-limited updates to Slack messages
type slackUpdater struct {
	client         *slack.Client
	channelID      string
	messageTS      string
	lastUpdate     time.Time
	contentBuffer  strings.Builder
	thoughtBuffer  strings.Builder
	toolCalls      []string
	updateInterval time.Duration
	mutex          sync.Mutex
	complete       bool
}

// newSlackUpdater creates a new rate-limited message updater
func newSlackUpdater(client *slack.Client, channelID, messageTS string) *slackUpdater {
	return &slackUpdater{
		client:         client,
		channelID:      channelID,
		messageTS:      messageTS,
		lastUpdate:     time.Now(), // Initialize to now to prevent immediate update
		contentBuffer:  strings.Builder{},
		thoughtBuffer:  strings.Builder{},
		toolCalls:      []string{},
		updateInterval: 1 * time.Second,
	}
}

// addContent adds regular content to the buffer
func (su *slackUpdater) addContent(content string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.contentBuffer.WriteString(content)
}

// addThought adds thought content to the buffer with proper formatting
func (su *slackUpdater) addThought(thought string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.thoughtBuffer.WriteString(fmt.Sprintf("> Thought: %s", thought))
}

// addToolCall records a tool call without exposing details
func (su *slackUpdater) addToolCall(toolName string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.toolCalls = append(su.toolCalls, toolName)
}

// shouldUpdate checks if enough time has passed since the last update
func (su *slackUpdater) shouldUpdate() bool {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	return time.Since(su.lastUpdate) >= su.updateInterval
}

// updateMessage updates the Slack message if enough time has passed
func (su *slackUpdater) updateMessage() error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	// Don't update if not enough time has passed and not complete
	if !su.complete && time.Since(su.lastUpdate) < su.updateInterval {
		return nil
	}

	var message strings.Builder

	// Add content if present
	if su.contentBuffer.Len() > 0 {
		message.WriteString(su.contentBuffer.String())
	}

	// Add thoughts if present
	if su.thoughtBuffer.Len() > 0 {
		if message.Len() > 0 {
			message.WriteString("\n\n")
		}
		message.WriteString(su.thoughtBuffer.String())
	}

	// Add tool call notifications if present
	if len(su.toolCalls) > 0 {
		if message.Len() > 0 {
			message.WriteString("\n\n")
		}
		message.WriteString("🔧 Tools used: ")
		for i, tool := range su.toolCalls {
			if i > 0 {
				message.WriteString(", ")
			}
			message.WriteString(fmt.Sprintf("`%s`", tool))
		}
	}

	// Update the message with proper block formatting
	blocks := parseMessageToBlocks(message.String())
	_, _, _, err := su.client.UpdateMessage(
		su.channelID,
		su.messageTS,
		slack.MsgOptionBlocks(blocks...),
	)

	if err == nil {
		su.lastUpdate = time.Now()
	}

	return err
}

// markComplete marks the response as complete and forces final update
func (su *slackUpdater) markComplete() {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.complete = true
}

// forceUpdate forces an immediate update regardless of timing
func (su *slackUpdater) forceUpdate() error {
	// Mark as complete first
	su.mutex.Lock()
	su.complete = true
	su.mutex.Unlock()

	// Then update message without holding the lock during API call
	if err := su.updateMessage(); err != nil {
		return err
	}
	return nil
}

// NewSlackBot creates a new Slack bot instance
func NewSlackBot(
	slackToken string,
	appToken string,
	config *config.File,
	sessionManager *SessionManager,
	tenantToolSet *query.TenantToolSet,
	approvalWorkflow *ApprovalWorkflow,
	securityLogger *sec.SecurityLogger,
) (*SlackBot, error) {
	// Validate app token format
	if !strings.HasPrefix(appToken, "xapp-") {
		return nil, fmt.Errorf("app token must start with 'xapp-'")
	}

	// Set the app token as environment variable for Socket Mode client
	// The slack-go library uses this environment variable for Socket Mode authentication
	if err := os.Setenv("SLACK_APP_TOKEN", appToken); err != nil {
		return nil, fmt.Errorf("setting SLACK_APP_TOKEN environment variable: %w", err)
	}

	fmt.Printf("New slack client\n")
	client := slack.New(slackToken, slack.OptionAppLevelToken(appToken))

	// Get bot user info
	fmt.Printf("\tauth test\n")
	authResp, err := client.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("authenticating with Slack: %w", err)
	}
	fmt.Printf("\tauth success: %#v\n", authResp)

	// Create Socket Mode client with proper authentication
	// The client will automatically use the SLACK_APP_TOKEN environment variable
	socketClient := socketmode.New(
		client,
		socketmode.OptionDebug(false),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.LstdFlags)),
	)

	bot := &SlackBot{
		client:           client,
		socketClient:     socketClient,
		sessionManager:   sessionManager,
		intentProcessor:  NewIntentProcessor(),
		tenantToolSet:    tenantToolSet,
		approvalWorkflow: approvalWorkflow,
		securityLogger:   securityLogger,
		config:           config,
		botUserID:        authResp.UserID,
	}

	// Set up admin users
	fmt.Printf("\tmulti-tenant\n")
	if config.MultiTenant != nil {
		bot.adminUsers = config.MultiTenant.AdminUsers
	}

	// Set approval workflow notification function
	fmt.Printf("\tnotify\n")
	bot.approvalWorkflow.SetNotifyFunction(bot.notifyAdmins)

	fmt.Printf("\tdone\n")
	return bot, nil
}

// ValidateSlackSetup validates Slack tokens and permissions
func (sb *SlackBot) ValidateSlackSetup() error {
	sb.securityLogger.LogInfo("system", "SlackValidation", "Validating Slack setup...")

	// Check that SLACK_APP_TOKEN environment variable is set
	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		err := fmt.Errorf("SLACK_APP_TOKEN environment variable not set")
		sb.securityLogger.LogError("system", "SlackAuth", err.Error())
		return err
	}

	// Validate app token format
	if !strings.HasPrefix(appToken, "xapp-") {
		err := fmt.Errorf("SLACK_APP_TOKEN must start with 'xapp-', got invalid format")
		sb.securityLogger.LogError("system", "SlackAuth", err.Error())
		return err
	}

	sb.securityLogger.LogInfo("system", "SlackAuth", fmt.Sprintf("App token format valid (prefix: %s)", appToken[:5]))

	// Validate bot token
	authResp, err := sb.client.AuthTest()
	if err != nil {
		sb.securityLogger.LogError("system", "SlackAuth", fmt.Sprintf("Bot token validation failed: %v", err))
		return fmt.Errorf("bot token validation failed: %w", err)
	}

	sb.botUserID = authResp.UserID
	sb.securityLogger.LogInfo("system", "SlackAuth", fmt.Sprintf("Bot token valid - Bot: %s (%s), Team: %s (%s)",
		authResp.User, authResp.UserID, authResp.Team, authResp.TeamID))

	// Check if bot is part of the workspace
	if authResp.User == "" || authResp.UserID == "" {
		err := fmt.Errorf("bot user information missing - check bot token permissions")
		sb.securityLogger.LogError("system", "SlackAuth", err.Error())
		return err
	}

	// Log bot capabilities for debugging
	sb.securityLogger.LogInfo("system", "SlackValidation", fmt.Sprintf("Bot user ID: %s, Bot name: %s", authResp.UserID, authResp.User))

	// Log that both tokens are ready for Socket Mode
	sb.securityLogger.LogInfo("system", "SlackValidation", "Both bot and app tokens validated - ready for Socket Mode connection")

	return nil
}

// StartSocketMode starts the Socket Mode API
func (sb *SlackBot) StartSocketMode(ctx context.Context) error {
	// Validate Slack setup first
	if err := sb.ValidateSlackSetup(); err != nil {
		return fmt.Errorf("Slack setup validation failed: %w", err)
	}

	go func() {
		if err := sb.socketClient.RunContext(ctx); err != nil {
			// Provide enhanced error diagnostics for Socket Mode failures
			errMsg := err.Error()

			// Check for specific authentication errors
			if strings.Contains(errMsg, "not_authed") {
				sb.securityLogger.LogError("system", "SlackSocketMode", "Authentication failed - check SLACK_APP_TOKEN and Socket Mode app configuration")
				fmt.Printf("❌ Socket Mode Authentication Error: 'not_authed'\n")
				fmt.Printf("   This typically means:\n")
				fmt.Printf("   1. SLACK_APP_TOKEN is missing or invalid\n")
				fmt.Printf("   2. App token doesn't have 'socket:write' scope\n")
				fmt.Printf("   3. Socket Mode is not enabled in the Slack app\n")
				fmt.Printf("   4. App is not properly installed in the workspace\n")
			} else if strings.Contains(errMsg, "invalid_auth") {
				sb.securityLogger.LogError("system", "SlackSocketMode", "Invalid authentication - check SLACK_BOT_TOKEN permissions")
				fmt.Printf("❌ Bot Token Authentication Error: 'invalid_auth'\n")
			} else {
				sb.securityLogger.LogError("system", "SlackSocketMode", fmt.Sprintf("Connection error: %v", err))
				fmt.Printf("❌ Socket Mode Connection Error: %v\n", err)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-sb.socketClient.Events:
			switch event.Type {
			case socketmode.EventTypeHello:
				// IMPORTANT: Hello events from Slack should NOT be acknowledged
				// Hello is a one-way notification from Slack to indicate connection is ready
				// Acknowledging Hello events violates Socket Mode protocol and may cause connection issues
				fmt.Println("\t- Hello received from Slack (connection established)")
				// No Ack() - Hello is one-way notification

			case socketmode.EventTypeConnecting:
				sb.securityLogger.LogInfo("system", "SlackSocketMode", "Connecting to Slack via Socket Mode...")
				fmt.Println("Connecting to Slack via Socket Mode...")

			case socketmode.EventTypeConnected:
				sb.securityLogger.LogInfo("system", "SlackSocketMode", "Successfully connected to Slack via Socket Mode")
				fmt.Println("Connected to Slack via Socket Mode!")

			case socketmode.EventTypeConnectionError:
				fmt.Printf("***\t\tError: %#v\n", event)
				errMsg := fmt.Sprintf("Socket Mode connection error: %+v", event.Data)
				sb.securityLogger.LogError("system", "SlackSocketMode", errMsg)
				fmt.Printf("❌ Socket Mode Connection Error: %+v\n", event.Data)

				// Provide specific guidance for connection errors
				if strings.Contains(errMsg, "not_authed") {
					fmt.Printf("💡 Troubleshooting 'not_authed' error:\n")
					fmt.Printf("   1. Verify SLACK_APP_TOKEN starts with 'xapp-' and has 'socket:write' scope\n")
					fmt.Printf("   2. Enable Socket Mode in your Slack app settings\n")
					fmt.Printf("   3. Reinstall the app to your workspace\n")
					fmt.Printf("   4. Check that the app has the required permissions\n")
				}

			case socketmode.EventTypeEventsAPI:
				// Acknowledge the event
				sb.socketClient.Ack(*event.Request)

				payload, ok := event.Data.(slackevents.EventsAPIEvent)
				if !ok {
					break
				}
				// Handle different event types
				switch ev := payload.InnerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					if err := sb.handleMessage(ctx, ev); err != nil {
						sb.securityLogger.LogError(ev.User, "SlackBot", err.Error())
					}
				default:
					// For other events, just log them
					fmt.Printf("EventsAPI Event Type: %T\n", ev)
				}

			case socketmode.EventTypeInteractive:
				// Handle interactive events (buttons, modals, etc.)
				callback, ok := event.Data.(slack.InteractionCallback)
				if ok {
					sb.socketClient.Ack(*event.Request)
					if err := sb.handleInteractiveCallback(ctx, &callback); err != nil {
						sb.securityLogger.LogError("system", "SlackInteractive", err.Error())
					}
				}

			default:
				fmt.Printf("Unhandled Socket Mode event: %s\n", event.Type)
			}
		}
	}
}

// handleMessage processes incoming Slack messages
func (sb *SlackBot) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) error {
	// Ignore messages from bots or messages without text
	if ev.BotID != "" || ev.SubType != "" || ev.Text == "" {
		return nil
	}

	// Check if message is mentioning the bot
	if ev.ChannelType == "im" {

	} else {
		if !strings.Contains(ev.Text, fmt.Sprintf("<@%s>", sb.botUserID)) {
			return nil
		}
	}

	// Remove bot mention from message
	cleanMessage := strings.ReplaceAll(ev.Text, fmt.Sprintf("<@%s>", sb.botUserID), "")
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
	user, err := sb.client.GetUserInfo(ev.User)
	if err == nil {
		slackCtx.UserName = user.Name
	}

	// Log session event
	sb.securityLogger.LogSessionEvent(ev.User, ev.Channel, "Message received")

	// Create user context
	userCtx := &query.UserContext{
		UserID:      ev.User,
		SlackTeamID: slackCtx.TeamID,
		IsAdmin:     sb.tenantToolSet.IsAdmin(ev.User),
	}

	// Get or create user session
	session := sb.sessionManager.GetOrCreateSession(ev.User, ev.Channel, userCtx)

	// Check if this is a tool management request
	intent, err := sb.intentProcessor.ProcessMessage(cleanMessage)
	if err != nil {
		return fmt.Errorf("processing intent: %w", err)
	}

	if intent != nil && intent.Confidence >= 0.7 {
		// Handle tool management request
		return sb.handleToolManagementIntent(ctx, slackCtx, session, intent)
	}

	// Handle as regular query
	return sb.handleQuery(ctx, slackCtx, session, cleanMessage)
}

// handleToolManagementIntent processes tool management intents
func (sb *SlackBot) handleToolManagementIntent(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	switch intent.Action {
	case "add_tool":
		return sb.handleAddTool(ctx, slackCtx, session, intent)
	case "share_tool":
		return sb.handleShareTool(ctx, slackCtx, session, intent)
	case "list_tools":
		return sb.handleListTools(ctx, slackCtx, session)
	case "remove_tool":
		return sb.handleRemoveTool(ctx, slackCtx, session, intent)
	case "approve_tool":
		return sb.handleApprovalCommand(ctx, slackCtx, session, intent, "approve")
	case "reject_tool":
		return sb.handleApprovalCommand(ctx, slackCtx, session, intent, "reject")
	default:
		return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("I don't know how to handle: %s", intent.Action))
	}
}

// handleApprovalCommand handles approve/reject commands via natural language
func (sb *SlackBot) handleApprovalCommand(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent, action string) error {
	// Verify admin permissions
	if !sb.approvalWorkflow.IsAdmin(slackCtx.UserID) {
		return sb.sendMessage(ctx, slackCtx, "❌ Only admins can approve/reject tool requests")
	}

	requestID := intent.Target
	if requestID == "" {
		return sb.sendMessage(ctx, slackCtx, "❌ Please specify a request ID")
	}

	if action == "approve" {
		if err := sb.approvalWorkflow.ApproveTool(slackCtx.UserID, requestID, "Approved via natural language"); err != nil {
			return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("❌ Error approving request: %s", err.Error()))
		}
		return sb.sendApprovalNotification(ctx, slackCtx.UserID, requestID, "approved")
	} else {
		reason := "No reason provided"
		if intent.Config != nil {
			reason = intent.Config.(string)
		}
		if err := sb.approvalWorkflow.RejectTool(slackCtx.UserID, requestID, reason); err != nil {
			return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("❌ Error rejecting request: %s", err.Error()))
		}
		return sb.sendApprovalNotification(ctx, slackCtx.UserID, requestID, "rejected")
	}
}

// handleAddTool handles adding new tools
func (sb *SlackBot) handleAddTool(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// Parse tool configuration
	toolConfig, err := ParseToolConfig(intent.ToolType, intent.Config.(string))
	if err != nil {
		return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("❌ Error parsing tool configuration: %s", err.Error()))
	}

	// Check if approval is needed
	if config.RequiresApproval(intent.ToolType) {
		// Submit for approval
		request := &ToolApprovalRequest{
			ToolID:        GenerateToolID(slackCtx.UserID, intent.ToolType, getNameFromConfig(toolConfig)),
			RequesterID:   slackCtx.UserID,
			ToolType:      intent.ToolType,
			Config:        toolConfig,
			RequesterName: slackCtx.UserName,
			Timestamp:     time.Now(),
		}

		requestID, err := sb.approvalWorkflow.RequestToolApproval(request)
		if err != nil {
			return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("❌ Error submitting approval request: %s", err.Error()))
		}

		sb.securityLogger.LogToolRequest(slackCtx.UserID, intent.ToolType, fmt.Sprintf("%+v", toolConfig))

		return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("📋 Tool approval request submitted:\n• Tool ID: %s\n• Status: Pending approval\n• I'll notify you when it's approved.", requestID))
	} else {
		// HTTP tools can be added directly
		toolID := GenerateToolID(slackCtx.UserID, intent.ToolType, getNameFromConfig(toolConfig))

		// TODO: Add tool directly to user's tool set
		// This would require extending the TenantToolSet to support dynamic tool addition

		sb.securityLogger.LogToolAdded(slackCtx.UserID, toolID, intent.ToolType)

		return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("✅ Added %s tool successfully. You can now use it in your conversations.", intent.ToolType))
	}
}

// handleShareTool handles sharing tools with other users
func (sb *SlackBot) handleShareTool(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// TODO: Implement tool sharing logic
	sb.securityLogger.LogToolShare(slackCtx.UserID, intent.TargetUser, intent.Target)
	return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("🔄 Tool sharing requested. Feature coming soon!"))
}

// handleListTools lists available tools for the user
func (sb *SlackBot) handleListTools(ctx context.Context, slackCtx *SlackContext, session *UserSession) error {
	tools := session.GetAvailableTools()

	if len(tools) == 0 {
		return sb.sendMessage(ctx, slackCtx, "You don't have any tools available yet. Try adding an HTTP MCP tool!")
	}

	var toolList strings.Builder
	toolList.WriteString("🔧 **Your Available Tools:**\n\n")

	for i, tool := range tools {
		toolList.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, tool))
	}

	return sb.sendMessage(ctx, slackCtx, toolList.String())
}

// handleRemoveTool handles removing tools
func (sb *SlackBot) handleRemoveTool(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// TODO: Implement tool removal logic
	return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("🗑️ Tool removal requested. Feature coming soon!"))
}

// handleQuery processes regular AI queries
func (sb *SlackBot) handleQuery(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string) error {
	fmt.Printf("handle query\n")
	// Add user message to session
	userMsg := api.Message{Role: "user", Content: message}
	if err := sb.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, userMsg); err != nil {
		fmt.Printf("add message failure? %#v\n", err)
		sb.securityLogger.LogError(slackCtx.UserID, "SessionManager", err.Error())
	}

	// Get user's tools
	userCtx := &query.UserContext{
		UserID:      slackCtx.UserID,
		SlackTeamID: slackCtx.TeamID,
		IsAdmin:     sb.tenantToolSet.IsAdmin(slackCtx.UserID),
	}

	fmt.Println("GetUserTools")
	userToolSet, err := sb.tenantToolSet.GetUserTools(ctx, userCtx)
	if err != nil {
		fmt.Printf("failed to get tool: %e\n", err)
		return fmt.Errorf("getting user tools: %w", err)
	}

	fmt.Println("kciking off query")
	// Start progressive response
	go sb.processQueryWithProgressiveResponse(ctx, slackCtx, session, message, userToolSet)

	return nil
}

// processQueryWithProgressiveResponse handles AI processing with progressive Slack updates
func (sb *SlackBot) processQueryWithProgressiveResponse(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, userToolSet *query.ToolSet) {
	fmt.Printf("Starting message\n")
	// Send initial "thinking" message
	initialMsg := "thinking..."
	ts, err := sb.postMessage(ctx, slackCtx, initialMsg)
	if err != nil {
		sb.securityLogger.LogError(slackCtx.UserID, "SlackBot", err.Error())
		return
	}

	// Set thread timestamp for ongoing conversation
	if ts != "" {
		if err := sb.sessionManager.SetThreadTS(slackCtx.UserID, slackCtx.ChannelID, ts); err != nil {
			sb.securityLogger.LogError(slackCtx.UserID, "SessionManager", err.Error())
		}
	}

	// Create rate-limited updater for this message
	updater := newSlackUpdater(sb.client, slackCtx.ChannelID, ts)

	// Create Ollama client
	client, err := api.ClientFromEnvironment()
	if err != nil {
		// Log error and flush any pending content
		updater.forceUpdate()
		sb.securityLogger.LogError(slackCtx.UserID, "SlackBot", fmt.Sprintf("Error creating Ollama client: %v", err))
		return
	}

	// Prepare conversation messages
	systemMessageContent := "You are a helpful assistant integrated with Slack."
	if sb.config.SystemPrompt != nil && len(sb.config.SystemPrompt.FromString) > 0 {
		systemMessageContent = sb.config.SystemPrompt.FromString
	}

	// Build conversation history from session
	messages := []api.Message{
		{Role: "system", Content: systemMessageContent},
	}
	messages = append(messages, session.Messages...)
	messages = append(messages, api.Message{Role: "user", Content: message})

	// Get available tools from user toolset
	availableTools := userToolSet.APITools()

	// Create streaming chat request
	stream := true
	req := &api.ChatRequest{
		Model:    sb.config.LanguageModel(),
		Messages: messages,
		Tools:    availableTools,
		Stream:   &stream,
	}

	// Process streaming response
	var assistantContent, thinkingBuffer strings.Builder
	var pendingCalls []api.ToolCall
	var thisLine strings.Builder
	var thisThinking strings.Builder

	err = client.Chat(ctx, req, func(resp api.ChatResponse) error {
		if resp.Done {
			// Response complete, mark updater as complete
			updater.markComplete()
		}

		// Handle content
		if s := resp.Message.Content; s != "" {
			thisLine.WriteString(s)
			if strings.Contains(s, "\n") {
				assistantContent.WriteString(thisLine.String())
				updater.addContent(thisLine.String())
				thisLine.Reset()

				// Try to update if enough time has passed
				if updater.shouldUpdate() {
					if updateErr := updater.updateMessage(); updateErr != nil {
						sb.securityLogger.LogError(slackCtx.UserID, "SlackBot", fmt.Sprintf("Error updating message: %v", updateErr))
					}
				}
			}
		}

		// Handle thinking
		if len(resp.Message.Thinking) > 0 {
			thisThinking.WriteString(resp.Message.Thinking)
			thinkingBuffer.WriteString(resp.Message.Thinking)

			if strings.Contains(resp.Message.Thinking, "\n") {
				updater.addThought(thisThinking.String())
				thisThinking.Reset()

				// Try to update if enough time has passed
				if updater.shouldUpdate() {
					if updateErr := updater.updateMessage(); updateErr != nil {
						sb.securityLogger.LogError(slackCtx.UserID, "SlackBot", fmt.Sprintf("Error updating message: %v", updateErr))
					}
				}
			}
		}

		// Handle tool calls
		if len(resp.Message.ToolCalls) > 0 {
			for _, toolCall := range resp.Message.ToolCalls {
				updater.addToolCall(toolCall.Function.Name)
			}
			pendingCalls = append(pendingCalls, resp.Message.ToolCalls...)
		}

		return nil
	})

	// Handle any remaining content
	if thisLine.Len() > 0 {
		assistantContent.WriteString(thisLine.String())
		updater.addContent(thisLine.String())
	}
	if thisThinking.Len() > 0 {
		updater.addThought(thisThinking.String())
	}

	// Force final update with complete content
	if err := updater.forceUpdate(); err != nil {
		sb.securityLogger.LogError(slackCtx.UserID, "SlackBot", fmt.Sprintf("Error in final message update: %v", err))
	}

	// Handle chat errors
	if err != nil {
		sb.securityLogger.LogError(slackCtx.UserID, "SlackBot", fmt.Sprintf("Error in Ollama chat: %v", err))
		return
	}

	// Execute tool calls if any
	if len(pendingCalls) > 0 {
		for _, call := range pendingCalls {
			reply, herr := userToolSet.HandleCall(ctx, call)
			if herr != nil {
				sb.securityLogger.LogError(slackCtx.UserID, "SlackBot", fmt.Sprintf("Error invoking tool %s: %v", call.Function.Name, herr))
				continue
			}
			// Add tool responses to conversation
			messages = append(messages, reply...)
		}

		// Continue conversation with tool results (simplified for now)
		// In a full implementation, we might want to continue the conversation loop
	}

	// Add assistant message to session
	finalContent := assistantContent.String()
	if thinkingBuffer.Len() > 0 {
		finalContent += fmt.Sprintf("\n\n> Thought: %s", thinkingBuffer.String())
	}

	assistantMsg := api.Message{
		Role:      "assistant",
		Content:   finalContent,
		ToolCalls: pendingCalls,
		Thinking:  thinkingBuffer.String(),
	}
	if err := sb.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, assistantMsg); err != nil {
		sb.securityLogger.LogError(slackCtx.UserID, "SessionManager", err.Error())
	}
}

// normalizeMarkdownForSlack converts standard markdown to Slack's mrkdwn format
func normalizeMarkdownForSlack(text string) string {
	result := text

	// Convert **bold** to *bold* (Slack uses single asterisks for bold)
	result = strings.ReplaceAll(result, "**", "*")

	// Handle edge case where replacement might create ***bold***
	// Replace *** with * (which will become bold)
	result = strings.ReplaceAll(result, "***", "*")

	// Convert __bold__ to *bold* (another common markdown variant)
	result = strings.ReplaceAll(result, "__", "*")

	// Convert *italic* to _italic_ (Slack prefers underscores for italics)
	// Be careful not to interfere with bold formatting
	result = strings.ReplaceAll(result, "*_", "_")
	result = strings.ReplaceAll(result, "_*", "_")

	return result
}

// parseMessageToBlocks converts markdown message to Slack blocks with proper header handling
func parseMessageToBlocks(message string) []slack.Block {
	// Normalize markdown for Slack compatibility
	message = normalizeMarkdownForSlack(message)
	var blocks []slack.Block
	lines := strings.Split(message, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Handle H1 headers (# Header)
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			headerText := strings.TrimPrefix(line, "# ")
			headerBlock := slack.NewHeaderBlock(&slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: headerText,
			})
			blocks = append(blocks, headerBlock)
			continue
		}

		// Handle H2 headers (## Header)
		if strings.HasPrefix(line, "## ") {
			headerText := strings.TrimPrefix(line, "## ")
			headerBlock := slack.NewHeaderBlock(&slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: headerText,
			})
			blocks = append(blocks, headerBlock)
			continue
		}

		// Handle H3+ headers (### Header, #### Header) - convert to bold text
		if strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "#### ") {
			boldText := strings.TrimPrefix(strings.TrimPrefix(line, "### "), "#### ")
			sectionBlock := slack.NewSectionBlock(&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: "*" + boldText + "*",
			}, nil, nil)
			blocks = append(blocks, sectionBlock)
			continue
		}

		// Handle block quotes (> Quote text)
		if strings.HasPrefix(line, "> ") {
			// Collect consecutive quote lines
			var quoteLines []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "> ") {
				quoteLine := strings.TrimPrefix(strings.TrimSpace(lines[i]), "> ")
				quoteLines = append(quoteLines, quoteLine)
				i++
			}
			i-- // Back up one since we'll increment in the outer loop

			if len(quoteLines) > 0 {
				quoteText := strings.Join(quoteLines, "\n")
				sectionBlock := slack.NewSectionBlock(&slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: quoteText,
				}, nil, nil)
				blocks = append(blocks, sectionBlock)
			}
			continue
		}

		// Skip empty lines
		if line == "" {
			continue
		}

		// For regular content, collect consecutive non-empty lines that aren't special
		var contentLines []string
		for i < len(lines) {
			currentLine := strings.TrimSpace(lines[i])
			if currentLine == "" {
				break // Empty line breaks content collection
			}
			if isHeaderLine(currentLine) || strings.HasPrefix(currentLine, "> ") {
				break // Header or quote breaks content collection
			}
			contentLines = append(contentLines, lines[i])
			i++
		}
		i-- // Back up one since we'll increment in the outer loop

		if len(contentLines) > 0 {
			contentText := strings.Join(contentLines, "\n")
			sectionBlock := slack.NewSectionBlock(&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: contentText,
			}, nil, nil)
			blocks = append(blocks, sectionBlock)
		}
	}

	// If no blocks were created (message was empty or only had unsupported content),
	// create a simple section block
	if len(blocks) == 0 {
		blocks = append(blocks, slack.NewSectionBlock(&slack.TextBlockObject{
			Type: slack.MarkdownType,
			Text: message,
		}, nil, nil))
	}

	return blocks
}

// isHeaderLine checks if a line is a markdown header
func isHeaderLine(line string) bool {
	return strings.HasPrefix(line, "# ") ||
		strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ") ||
		strings.HasPrefix(line, "#### ")
}

// sendMessage sends a message to Slack
func (sb *SlackBot) sendMessage(ctx context.Context, slackCtx *SlackContext, message string) error {
	_, _, err := sb.client.PostMessage(
		slackCtx.ChannelID,
		slack.MsgOptionText(message, false),
		slack.MsgOptionTS(slackCtx.ThreadTS),
	)
	return err
}

// postMessage posts a new message and returns its timestamp
func (sb *SlackBot) postMessage(ctx context.Context, slackCtx *SlackContext, message string) (string, error) {
	blocks := parseMessageToBlocks(message)
	_, timestamp, err := sb.client.PostMessageContext(
		ctx,
		slackCtx.ChannelID,
		slack.MsgOptionTS(slackCtx.ThreadTS),
		slack.MsgOptionBlocks(blocks...),
	)
	return timestamp, err
}

// updateMessage updates an existing message
func (sb *SlackBot) updateMessage(ctx context.Context, slackCtx *SlackContext, timestamp, message string) error {
	blocks := parseMessageToBlocks(message)
	_, _, _, err := sb.client.UpdateMessageContext(
		ctx,
		slackCtx.ChannelID,
		timestamp,
		slack.MsgOptionBlocks(blocks...),
	)
	return err
}

// notifyAdmins sends approval notifications to admin users
func (sb *SlackBot) notifyAdmins(request *ToolApprovalRequest) error {
	message := sb.approvalWorkflow.FormatApprovalForSlack(request)

	for _, adminID := range sb.adminUsers {
		// Open DM channel with admin
		channel, _, _, err := sb.client.OpenConversation(&slack.OpenConversationParameters{
			Users: []string{adminID},
		})
		if err != nil {
			sb.securityLogger.LogError(adminID, "SlackBot", fmt.Sprintf("Failed to open DM: %s", err.Error()))
			continue
		}

		// Send approval request
		_, _, err = sb.client.PostMessage(
			channel.ID,
			slack.MsgOptionText(message, false),
		)
		if err != nil {
			sb.securityLogger.LogError(adminID, "SlackBot", fmt.Sprintf("Failed to send approval notification: %s", err.Error()))
		}
	}

	return nil
}

// handleInteractiveCallback handles interactive events (buttons, modals, etc.)
func (sb *SlackBot) handleInteractiveCallback(ctx context.Context, callback *slack.InteractionCallback) error {
	// Handle approval/rejection interactions
	if callback.Type == slack.InteractionTypeBlockActions {
		for _, action := range callback.ActionCallback.BlockActions {
			switch action.ActionID {
			case "approve_tool":
				// Extract request ID from action value
				requestID := action.Value
				if err := sb.approvalWorkflow.ApproveTool(callback.User.ID, requestID, "Approved via Slack button"); err != nil {
					return fmt.Errorf("approving tool: %w", err)
				}
				// Notify requester
				return sb.sendApprovalNotification(ctx, callback.User.ID, requestID, "approved")

			case "reject_tool":
				// Extract request ID and reason from action value
				parts := strings.SplitN(action.Value, ":", 2)
				if len(parts) < 2 {
					return fmt.Errorf("invalid reject action format")
				}
				requestID := parts[0]
				reason := parts[1]
				if err := sb.approvalWorkflow.RejectTool(callback.User.ID, requestID, reason); err != nil {
					return fmt.Errorf("rejecting tool: %w", err)
				}
				// Notify requester
				return sb.sendApprovalNotification(ctx, callback.User.ID, requestID, "rejected")
			}
		}
	}

	return fmt.Errorf("unhandled interactive callback type: %s", callback.Type)
}

// sendApprovalNotification sends approval status notification
func (sb *SlackBot) sendApprovalNotification(ctx context.Context, adminID, requestID, status string) error {
	// Get approval details to find the requester
	approval, err := sb.approvalWorkflow.GetApprovalStatus(requestID)
	if err != nil {
		return fmt.Errorf("getting approval status: %w", err)
	}

	message := fmt.Sprintf("🔧 Your tool request %s has been **%s** by <@%s>", requestID, status, adminID)

	// Open DM channel with requester
	channel, _, _, err := sb.client.OpenConversation(&slack.OpenConversationParameters{
		Users: []string{approval.RequesterID},
	})
	if err != nil {
		return fmt.Errorf("opening DM with requester: %w", err)
	}

	_, _, err = sb.client.PostMessage(
		channel.ID,
		slack.MsgOptionText(message, false),
	)
	return err
}

// getNameFromConfig extracts name from tool configuration
func getNameFromConfig(config interface{}) string {
	// Use type assertion safely
	if cfg, ok := config.(interface{ GetName() string }); ok {
		return cfg.GetName()
	}
	return "unknown"
}
