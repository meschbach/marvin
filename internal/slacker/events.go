package slacker

import (
	"context"
	"fmt"
	"strings"

	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"go.opentelemetry.io/otel/trace"
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
		return er.handleHello(event)
	case socketmode.EventTypeConnecting:
		return er.handleConnecting(event)
	case socketmode.EventTypeConnected:
		return er.handleConnected(event)
	case socketmode.EventTypeConnectionError:
		return er.handleConnectionError(event)
	case socketmode.EventTypeEventsAPI:
		return er.handleEventsAPI(ctx, event)
	case socketmode.EventTypeInteractive:
		return er.handleInteractive(ctx, event)
	default:
		fmt.Printf("Unhandled Socket Mode event: %s\n", event.Type)
	}
	return nil
}

func (er *EventRouter) handleHello(event socketmode.Event) error {
	fmt.Println("\t- Hello received from Slack (connection established)")
	return nil
}

func (er *EventRouter) handleConnecting(event socketmode.Event) error {
	er.securityLogger.LogInfo("system", "SlackSocketMode", "Connecting to Slack via Socket Mode...")
	fmt.Println("Connecting to Slack via Socket Mode...")
	return nil
}

func (er *EventRouter) handleConnected(event socketmode.Event) error {
	er.securityLogger.LogInfo("system", "SlackSocketMode", "Successfully connected to Slack via Socket Mode")
	fmt.Println("Connected to Slack via Socket Mode!")
	return nil
}

func (er *EventRouter) handleConnectionError(event socketmode.Event) error {
	fmt.Printf("***\t\tError: %#v\n", event)
	errMsg := fmt.Sprintf("Socket Mode connection error: %+v", event.Data)
	er.securityLogger.LogError("system", "SlackSocketMode", errMsg)
	fmt.Printf("❌ Socket Mode Connection Error: %+v\n", event.Data)

	if strings.Contains(errMsg, "not_authed") {
		fmt.Printf("💡 Troubleshooting 'not_authed' error:\n")
		fmt.Printf("   1. Verify SLACK_APP_TOKEN starts with 'xapp-' and has 'socket:write' scope\n")
		fmt.Printf("   2. Enable Socket Mode in your Slack app settings\n")
		fmt.Printf("   3. Reinstall the app to your workspace\n")
		fmt.Printf("   4. Check that the app has the required permissions\n")
	}
	return nil
}

func (er *EventRouter) handleEventsAPI(ctx context.Context, event socketmode.Event) error {
	er.connection.socketClient.Ack(*event.Request)

	payload, ok := event.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return nil
	}

	span := trace.SpanFromContext(ctx)
	span.SetName(fmt.Sprintf("slacker.event.events_api:%s", payload.InnerEvent.Type))

	switch ev := payload.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		return er.messageHandler.ProcessMessage(ctx, ev)
	default:
		fmt.Printf("EventsAPI Event Type: %T\n", ev)
	}
	return nil
}

func (er *EventRouter) handleInteractive(ctx context.Context, event socketmode.Event) error {
	callback, ok := event.Data.(slack.InteractionCallback)
	if !ok {
		return nil
	}

	er.connection.socketClient.Ack(*event.Request)
	return er.handleInteractiveCallback(ctx, &callback)
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
