package proc

import (
	"context"
	"fmt"
	"os"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/meschbach/marvin/internal/slacker"
	sec "github.com/meschbach/marvin/internal/slacker/security"

	"github.com/meschbach/go-junk-bucket/pkg/observability"
)

type Components struct {
	Config           *config.File
	SecurityLogger   *sec.SecurityLogger
	SessionManager   *slacker.SessionManager
	CredentialStore  *sec.UserCredentialStore
	Observability    *observability.Component
	TenantToolSet    *query.TenantToolSet
	ApprovalWorkflow *slacker.ApprovalWorkflow
	SlackBot         *slacker.SlackBot
}

func loadConfig(opts *Options) (*config.File, error) {
	if _, err := os.Stat(opts.ConfigFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", opts.ConfigFile)
	}

	cmdOpts := config.NewCommandLineOptions()
	return cmdOpts.LoadFromPath(opts.ConfigFile)
}

func logStartupDiagnostics(cfg *config.File, opts *Options) {
	secLogger := sec.NewSecurityLogger()

	secLogger.LogStartupEvent("Configuration", fmt.Sprintf("Config file: %s", opts.ConfigFile))

	if cfg.MultiTenant != nil {
		secLogger.LogStartupEvent("Users", fmt.Sprintf("Admin Users: %v", cfg.MultiTenant.AdminUsers))
		if cfg.MultiTenant.AdminChannel != "" {
			secLogger.LogStartupEvent("Channel", fmt.Sprintf("Admin Channel: %s", cfg.MultiTenant.AdminChannel))
		}
	}

	secLogger.LogStartupEvent("Directories", fmt.Sprintf("Session Store: %s, Credential Store: %s", opts.SessionStore, opts.CredentialStore))
}

func initializeComponents(ctx context.Context, opts *Options, cfg *config.File, passphrase string) (*Components, error) {
	securityLogger := sec.NewSecurityLogger()

	sessionStorePath := opts.SessionStore
	if cfg.MultiTenant != nil && cfg.MultiTenant.SessionStorePath != "" {
		sessionStorePath = cfg.MultiTenant.SessionStorePath
	}
	sessionManager, err := slacker.NewSessionManager(sessionStorePath)
	if err != nil {
		return nil, fmt.Errorf("creating session manager: %w", err)
	}

	credentialStorePath := opts.CredentialStore
	if cfg.MultiTenant != nil && cfg.MultiTenant.CredentialStore != "" {
		credentialStorePath = cfg.MultiTenant.CredentialStore
	}
	keyFile := credentialStorePath + "/.key"
	credentialManager := sec.NewUserCredentialStore(credentialStorePath, keyFile)
	if err := credentialManager.Initialize(passphrase); err != nil {
		return nil, fmt.Errorf("initializing credential manager: %w", err)
	}

	var obsComponent *observability.Component
	if cfg.Observability != nil || cfg.HasObservabilityEnvVars() {
		var obsConfig observability.Config
		if cfg.Observability != nil {
			obsConfig = cfg.Observability.ToObservabilityConfig()
		} else {
			obsConfig = observability.DefaultConfig("marvin")
		}
		obsComponent, err = obsConfig.Start(ctx)
		if err != nil {
			return nil, fmt.Errorf("initializing observability: %w", err)
		}
	}

	tenantToolSet, err := query.NewTenantToolSet(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating tenant tool set: %w", err)
	}

	var adminUsers []string
	if cfg.MultiTenant != nil {
		adminUsers = cfg.MultiTenant.AdminUsers
	}
	approvalWorkflow := slacker.NewApprovalWorkflow(adminUsers, securityLogger)

	slackToken := os.Getenv("SLACK_BOT_TOKEN")
	if slackToken == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN environment variable is required")
	}

	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		return nil, fmt.Errorf("SLACK_APP_TOKEN environment variable is required")
	}

	bot, err := slacker.NewSlackBot(
		slackToken,
		appToken,
		cfg,
		sessionManager,
		tenantToolSet,
		approvalWorkflow,
		securityLogger,
	)
	if err != nil {
		return nil, fmt.Errorf("creating Slack bot: %w", err)
	}

	return &Components{
		Config:           cfg,
		SecurityLogger:   securityLogger,
		SessionManager:   sessionManager,
		CredentialStore:  credentialManager,
		Observability:    obsComponent,
		TenantToolSet:    tenantToolSet,
		ApprovalWorkflow: approvalWorkflow,
		SlackBot:         bot,
	}, nil
}
