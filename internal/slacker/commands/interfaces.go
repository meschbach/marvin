package commands

import (
	"context"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/slack-go/slack"
)

// SlackClientAPI abstracts Slack API operations for testing.
type SlackClientAPI interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
	GetUserInfo(userID string) (*slack.User, error)
	AuthTest() (*slack.AuthTestResponse, error)
	OpenConversation(context.Context, *slack.OpenConversationParameters) (*slack.Channel, bool, bool, error)
	UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
}

type UserPreferences struct {
	ShowThinking   bool
	ShowTools      bool
	ShowDone       bool
	ThinkingFormat string
	ToolFormat     string
	Verbose        bool
}

func DefaultUserPreferences() UserPreferences {
	return UserPreferences{
		ShowThinking:   false,
		ShowTools:      true,
		ShowDone:       true,
		ThinkingFormat: "plain",
		ToolFormat:     "detailed",
		Verbose:        false,
	}
}

type Context interface {
	UserID() string
	UserName() string
	ChannelID() string
	TeamID() string
}

type ApprovalWorkflow interface {
	RequestToolApproval(ctx context.Context, request *ToolApprovalRequest) (string, error)
	ApproveTool(ctx context.Context, approverID, requestID, reason string) error
	RejectTool(ctx context.Context, approverID, requestID, reason string) error
	IsAdmin(userID string) bool
}

type SecurityLogger interface {
	LogError(userID, operation, message string)
	LogSessionEvent(userID, channelID, event string)
	LogToolRequest(userID, toolType, config string)
	LogToolAdded(userID, toolID, toolType string)
	LogToolRemoved(userID, toolID string)
	LogToolShare(userID, toolID, targetWorkspace string)
	LogConfigChange(userID, configType, details string)
	LogAdminAction(adminID, action, target string)
}

type SessionManager interface {
	ClearSession(userID, channelID string) error
	GetOrCreateSession(ctx context.Context, userID, channelID string, userCtx *query.UserContext) (*UserSession, error)
	GetPreferences(userID string) (UserPreferences, bool)
	UpdatePreferences(userID string, preferences UserPreferences) error
}

type ToolSet interface {
	ToolsForUser(ctx context.Context, userID string) ([]string, error)
	AddTool(ctx context.Context, userID, toolID string, toolType string, config interface{}) error
	RemoveTool(ctx context.Context, userID, toolID string) error
	ShareTool(ctx context.Context, toolID, targetUserID string) error
}

type Connection interface {
	GetBotUserID() string
}

type ToolParser interface {
	ParseToolConfig(toolType, config string) (interface{}, error)
	GenerateToolID(userID, toolType, name string) string
}

type MessageSender interface {
	SendMessage(ctx context.Context, userID, message string) error
}

type CommandsDependencies struct {
	Context          Context
	ApprovalWorkflow ApprovalWorkflow
	TenantToolSet    *query.TenantToolSet
	ToolSet          ToolSet
	SecurityLogger   SecurityLogger
	SessionManager   SessionManager
	SlackClient      SlackClientAPI
	Config           *config.File
	Connection       Connection
	ToolParser       ToolParser
	MessageSender    MessageSender
}

type ToolApprovalRequest struct {
	ToolID        string
	RequesterID   string
	ToolType      string
	Config        interface{}
	RequesterName string
	Timestamp     interface{}
}

type UserSession struct {
	ID             string
	UserID         string
	Context        *query.UserContext
	AvailableTools []string
}
