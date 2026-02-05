package slacker

import (
	"context"
	"fmt"
	"strings"

	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// EventRouter routes Socket Mode events to appropriate handlers
type EventRouter struct {
	connection     *SlackConnection
	messageHandler MessageProcessor
	securityLogger *sec.SecurityLogger
}

// NewEventRouter creates a new event router
func NewEventRouter(
	connection *SlackConnection,
	messageHandler MessageProcessor,
	securityLogger *sec.SecurityLogger,
) *EventRouter {
	return &EventRouter{
		connection:     connection,
		messageHandler: messageHandler,
		securityLogger: securityLogger,
	}
}

// RouteEvent processes Socket Mode events and routes them to appropriate handlers
func (er *EventRouter) RouteEvent(ctx context.Context, event socketmode.Event) error {
	switch event.Type {
	case socketmode.EventTypeHello:
		// Hello events from Slack should NOT be acknowledged
		fmt.Println("\t- Hello received from Slack (connection established)")
		// No Ack() - Hello is one-way notification

	case socketmode.EventTypeConnecting:
		er.securityLogger.LogInfo("system", "SlackSocketMode", "Connecting to Slack via Socket Mode...")
		fmt.Println("Connecting to Slack via Socket Mode...")

	case socketmode.EventTypeConnected:
		er.securityLogger.LogInfo("system", "SlackSocketMode", "Successfully connected to Slack via Socket Mode")
		fmt.Println("Connected to Slack via Socket Mode!")

	case socketmode.EventTypeConnectionError:
		fmt.Printf("***\t\tError: %#v\n", event)
		errMsg := fmt.Sprintf("Socket Mode connection error: %+v", event.Data)
		er.securityLogger.LogError("system", "SlackSocketMode", errMsg)
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
		er.connection.socketClient.Ack(*event.Request)

		payload, ok := event.Data.(slackevents.EventsAPIEvent)
		if !ok {
			break
		}
		// Handle different event types
		switch ev := payload.InnerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			return er.messageHandler.ProcessMessage(ctx, ev)
		default:
			// For other events, just log them
			fmt.Printf("EventsAPI Event Type: %T\n", ev)
		}

	case socketmode.EventTypeInteractive:
		// Handle interactive events (buttons, modals, etc.)
		callback, ok := event.Data.(slack.InteractionCallback)
		if ok {
			er.connection.socketClient.Ack(*event.Request)
			return er.handleInteractiveCallback(ctx, &callback)
		}

	default:
		fmt.Printf("Unhandled Socket Mode event: %s\n", event.Type)
	}

	return nil
}

// handleInteractiveCallback handles interactive events (buttons, modals, etc.)
func (er *EventRouter) handleInteractiveCallback(ctx context.Context, callback *slack.InteractionCallback) error {
	// Handle approval/rejection interactions
	if callback.Type == slack.InteractionTypeBlockActions {
		for _, action := range callback.ActionCallback.BlockActions {
			switch action.ActionID {
			case "approve_tool":
				// Extract request ID from action value
				requestID := action.Value
				// This would need to be handled by the tool manager
				fmt.Printf("Approve tool request: %s\n", requestID)

			case "reject_tool":
				// Extract request ID and reason from action value
				parts := strings.SplitN(action.Value, ":", 2)
				if len(parts) < 2 {
					return fmt.Errorf("invalid reject action format")
				}
				requestID := parts[0]
				reason := parts[1]
				// This would need to be handled by the tool manager
				fmt.Printf("Reject tool request: %s, reason: %s\n", requestID, reason)
			}
		}
	}

	return fmt.Errorf("unhandled interactive callback type: %s", callback.Type)
}
