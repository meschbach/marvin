package slacker

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

// SlackConnection handles Slack API and Socket Mode connections
type SlackConnection struct {
	client         *slack.Client
	socketClient   *socketmode.Client
	botUserID      string
	adminUsers     []string
	securityLogger *sec.SecurityLogger
}

// NewSlackConnection creates a new Slack connection
func NewSlackConnection(
	slackToken string,
	appToken string,
	config *config.File,
	securityLogger *sec.SecurityLogger,
) (*SlackConnection, error) {
	// Validate app token format
	if !strings.HasPrefix(appToken, "xapp-") {
		return nil, fmt.Errorf("app token must start with 'xapp-'")
	}

	// Set the app token as environment variable for Socket Mode client
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

	// Create Socket Mode client
	socketClient := socketmode.New(
		client,
		socketmode.OptionDebug(false),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.LstdFlags)),
	)

	// Set up admin users
	adminUsers := []string{}
	if config.MultiTenant != nil {
		adminUsers = config.MultiTenant.AdminUsers
	}

	return &SlackConnection{
		client:         client,
		socketClient:   socketClient,
		botUserID:      authResp.UserID,
		adminUsers:     adminUsers,
		securityLogger: securityLogger,
	}, nil
}

// ValidateSetup validates Slack tokens and permissions
func (sc *SlackConnection) ValidateSetup() error {
	sc.securityLogger.LogInfo("system", "SlackValidation", "Validating Slack setup...")

	// Check that SLACK_APP_TOKEN environment variable is set
	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		err := fmt.Errorf("SLACK_APP_TOKEN environment variable not set")
		sc.securityLogger.LogError("system", "SlackAuth", err.Error())
		return err
	}

	// Validate app token format
	if !strings.HasPrefix(appToken, "xapp-") {
		err := fmt.Errorf("SLACK_APP_TOKEN must start with 'xapp-', got invalid format")
		sc.securityLogger.LogError("system", "SlackAuth", err.Error())
		return err
	}

	sc.securityLogger.LogInfo("system", "SlackAuth", fmt.Sprintf("App token format valid (prefix: %s)", appToken[:5]))

	// Validate bot token
	authResp, err := sc.client.AuthTest()
	if err != nil {
		sc.securityLogger.LogError("system", "SlackAuth", fmt.Sprintf("Bot token validation failed: %v", err))
		return fmt.Errorf("bot token validation failed: %w", err)
	}

	sc.botUserID = authResp.UserID
	sc.securityLogger.LogInfo("system", "SlackAuth", fmt.Sprintf("Bot token valid - Bot: %s (%s), Team: %s (%s)",
		authResp.User, authResp.UserID, authResp.Team, authResp.TeamID))

	// Check if bot is part of the workspace
	if authResp.User == "" || authResp.UserID == "" {
		err := fmt.Errorf("bot user information missing - check bot token permissions")
		sc.securityLogger.LogError("system", "SlackAuth", err.Error())
		return err
	}

	// Log bot capabilities for debugging
	sc.securityLogger.LogInfo("system", "SlackValidation", fmt.Sprintf("Bot user ID: %s, Bot name: %s", authResp.UserID, authResp.User))

	// Log that both tokens are ready for Socket Mode
	sc.securityLogger.LogInfo("system", "SlackValidation", "Both bot and app tokens validated - ready for Socket Mode connection")

	return nil
}

// StartSocketMode starts the Socket Mode API
func (sc *SlackConnection) StartSocketMode(ctx context.Context) error {
	// Validate Slack setup first
	if err := sc.ValidateSetup(); err != nil {
		return fmt.Errorf("slack setup validation failed: %w", err)
	}

	return sc.socketClient.RunContext(ctx)
}

// GetBotUserID returns the bot user ID
func (sc *SlackConnection) GetBotUserID() string {
	return sc.botUserID
}

// GetClient returns the Slack client
func (sc *SlackConnection) GetClient() *slack.Client {
	return sc.client
}

// GetAdminUsers returns the list of admin users
func (sc *SlackConnection) GetAdminUsers() []string {
	return sc.adminUsers
}

// LogSecurityEvent logs a security event
func (sc *SlackConnection) LogSecurityEvent(userID, component, message string) {
	sc.securityLogger.LogInfo(userID, component, message)
}

// LogSecurityError logs a security error
func (sc *SlackConnection) LogSecurityError(userID, component, error string) {
	sc.securityLogger.LogError(userID, component, error)
}
