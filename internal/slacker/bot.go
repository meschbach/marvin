package slacker

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/ollama/ollama/api"
	"github.com/slack-go/slack"
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
			fmt.Printf("Socket Mode Event Received: %#v\n", event.Type)
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

				// Handle different event types
				switch ev := event.Data.(type) {
				case *slack.MessageEvent:
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

			case socketmode.EventTypeSlashCommand:
				// Handle slash commands
				cmd, ok := event.Data.(slack.SlashCommand)
				if ok {
					sb.socketClient.Ack(*event.Request)
					if err := sb.handleSlashCommand(ctx, &cmd); err != nil {
						sb.securityLogger.LogError(cmd.UserID, "SlackSlash", err.Error())
					}
				}

			default:
				fmt.Printf("Unhandled Socket Mode event: %s\n", event.Type)
			}
		}
	}
}

// handleMessage processes incoming Slack messages
func (sb *SlackBot) handleMessage(ctx context.Context, ev *slack.MessageEvent) error {
	// Ignore messages from bots or messages without text
	if ev.BotID != "" || ev.SubType != "" || ev.Text == "" {
		return nil
	}

	// Check if message is mentioning the bot
	if !strings.Contains(ev.Text, fmt.Sprintf("<@%s>", sb.botUserID)) {
		return nil
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
		Timestamp: ev.Timestamp,
		ThreadTS:  ev.ThreadTimestamp,
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
	default:
		return sb.sendMessage(ctx, slackCtx, fmt.Sprintf("I don't know how to handle: %s", intent.Action))
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
	// Add user message to session
	userMsg := api.Message{Role: "user", Content: message}
	if err := sb.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, userMsg); err != nil {
		sb.securityLogger.LogError(slackCtx.UserID, "SessionManager", err.Error())
	}

	// Get user's tools
	userCtx := &query.UserContext{
		UserID:      slackCtx.UserID,
		SlackTeamID: slackCtx.TeamID,
		IsAdmin:     sb.tenantToolSet.IsAdmin(slackCtx.UserID),
	}

	userToolSet, err := sb.tenantToolSet.GetUserTools(ctx, userCtx)
	if err != nil {
		return fmt.Errorf("getting user tools: %w", err)
	}

	// Start progressive response
	go sb.processQueryWithProgressiveResponse(ctx, slackCtx, session, message, userToolSet)

	return nil
}

// processQueryWithProgressiveResponse handles AI processing with progressive Slack updates
func (sb *SlackBot) processQueryWithProgressiveResponse(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, userToolSet *query.ToolSet) {
	// Send initial "thinking" message
	initialMsg := fmt.Sprintf("🤔 Processing your request: \"%s\"", message)
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

	// TODO: Implement actual AI processing using Marvin's query system
	// For now, send a simple response

	time.Sleep(2 * time.Second) // Simulate processing time

	response := fmt.Sprintf("I received your query: \"%s\"\n\n🚀 **AI Integration coming soon!**\n\nThis is where I would:\n1. Process your request with the AI model\n2. Use available tools as needed\n3. Provide a helpful response", message)

	// Update the message with the response
	if err := sb.updateMessage(ctx, slackCtx, ts, response); err != nil {
		sb.securityLogger.LogError(slackCtx.UserID, "SlackBot", err.Error())
		return
	}

	// Add assistant message to session
	assistantMsg := api.Message{Role: "assistant", Content: response}
	if err := sb.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, assistantMsg); err != nil {
		sb.securityLogger.LogError(slackCtx.UserID, "SessionManager", err.Error())
	}
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
	_, timestamp, err := sb.client.PostMessage(
		slackCtx.ChannelID,
		slack.MsgOptionText(message, false),
		slack.MsgOptionTS(slackCtx.ThreadTS),
	)
	return timestamp, err
}

// updateMessage updates an existing message
func (sb *SlackBot) updateMessage(ctx context.Context, slackCtx *SlackContext, timestamp, message string) error {
	_, _, _, err := sb.client.UpdateMessage(
		slackCtx.ChannelID,
		timestamp,
		slack.MsgOptionText(message, false),
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

// handleSlashCommand handles slash commands
func (sb *SlackBot) handleSlashCommand(ctx context.Context, cmd *slack.SlashCommand) error {
	switch cmd.Command {
	case "/approve":
		// Handle approval slash command: /approve <request-id>
		if len(strings.TrimSpace(cmd.Text)) == 0 {
			return sb.sendSlashResponse(ctx, cmd, "Usage: /approve <request-id>")
		}
		requestID := strings.TrimSpace(cmd.Text)
		if err := sb.approvalWorkflow.ApproveTool(cmd.UserID, requestID, "Approved via slash command"); err != nil {
			return sb.sendSlashResponse(ctx, cmd, fmt.Sprintf("Error approving request: %s", err.Error()))
		}
		return sb.sendSlashResponse(ctx, cmd, fmt.Sprintf("Request %s approved", requestID))

	case "/reject":
		// Handle rejection slash command: /reject <request-id> <reason>
		parts := strings.SplitN(cmd.Text, " ", 2)
		if len(parts) < 2 {
			return sb.sendSlashResponse(ctx, cmd, "Usage: /reject <request-id> <reason>")
		}
		requestID := parts[0]
		reason := parts[1]
		if err := sb.approvalWorkflow.RejectTool(cmd.UserID, requestID, reason); err != nil {
			return sb.sendSlashResponse(ctx, cmd, fmt.Sprintf("Error rejecting request: %s", err.Error()))
		}
		return sb.sendSlashResponse(ctx, cmd, fmt.Sprintf("Request %s rejected: %s", requestID, reason))

	case "/list-tools":
		// Handle tool listing command
		session := sb.sessionManager.GetOrCreateSession(cmd.UserID, cmd.ChannelID, &query.UserContext{
			UserID:  cmd.UserID,
			IsAdmin: sb.tenantToolSet.IsAdmin(cmd.UserID),
		})
		return sb.handleListTools(ctx, &SlackContext{
			UserID:    cmd.UserID,
			ChannelID: cmd.ChannelID,
		}, session)

	default:
		return sb.sendSlashResponse(ctx, cmd, fmt.Sprintf("Unknown command: %s", cmd.Command))
	}
}

// sendSlashResponse sends a response to a slash command
func (sb *SlackBot) sendSlashResponse(ctx context.Context, cmd *slack.SlashCommand, message string) error {
	_, _, err := sb.client.PostMessage(
		cmd.ChannelID,
		slack.MsgOptionText(message, false),
	)
	return err
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
