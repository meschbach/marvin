package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/meschbach/marvin/internal/slacker"
	sec "github.com/meschbach/marvin/internal/slacker/security"
)

const (
	defaultConfigFile      = "marvin.slacker.hcl"
	defaultSessionStore    = "./sessions"
	defaultCredentialStore = "./credentials"
)

// SlackerOptions holds command line options
type SlackerOptions struct {
	ConfigFile      string
	SessionStore    string
	CredentialStore string
	CredentialPass  string
	Verbose         bool
}

// logStartupDiagnostics logs comprehensive startup information
func logStartupDiagnostics(cfg *config.File, options *SlackerOptions) {
	secLogger := sec.NewSecurityLogger()

	secLogger.LogStartupEvent("Configuration", fmt.Sprintf("Config file: %s", options.ConfigFile))

	if cfg.MultiTenant != nil {
		secLogger.LogStartupEvent("Users", fmt.Sprintf("Admin Users: %v", cfg.MultiTenant.AdminUsers))
		if cfg.MultiTenant.AdminChannel != "" {
			secLogger.LogStartupEvent("Channel", fmt.Sprintf("Admin Channel: %s", cfg.MultiTenant.AdminChannel))
		}
	}

	secLogger.LogStartupEvent("Directories", fmt.Sprintf("Session Store: %s, Credential Store: %s", options.SessionStore, options.CredentialStore))
}

func main() {
	var (
		configFile      = flag.String("config", defaultConfigFile, "Path to configuration file")
		sessionStore    = flag.String("sessions", defaultSessionStore, "Directory to store sessions")
		credentialStore = flag.String("credentials", defaultCredentialStore, "Directory to store encrypted credentials")
		credentialPass  = flag.String("passphrase", "", "Passphrase for credential encryption (required)")
		verbose         = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()

	options := &SlackerOptions{
		ConfigFile:      *configFile,
		SessionStore:    *sessionStore,
		CredentialStore: *credentialStore,
		CredentialPass:  *credentialPass,
		Verbose:         *verbose,
	}

	if options.CredentialPass == "" {
		fmt.Fprintf(os.Stderr, "Error: --passphrase is required for credential encryption\n")
		flag.Usage()
		os.Exit(1)
	}

	if options.Verbose {
		fmt.Printf("Starting Slacker bot...\n")
		fmt.Printf("Config file: %s\n", options.ConfigFile)
		fmt.Printf("Session store: %s\n", options.SessionStore)
		fmt.Printf("Credential store: %s\n", options.CredentialStore)
	}

	// Load configuration
	cfg, err := loadConfig(options.ConfigFile)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Initialize security logger
	securityLogger := sec.NewSecurityLogger()

	// Initialize session manager
	sessionStorePath := options.SessionStore
	if cfg.MultiTenant != nil && cfg.MultiTenant.SessionStorePath != "" {
		sessionStorePath = cfg.MultiTenant.SessionStorePath
	}
	sessionManager, err := slacker.NewSessionManager(sessionStorePath)
	if err != nil {
		log.Fatalf("Error creating session manager: %v", err)
	}

	// Initialize credential store
	credentialStorePath := options.CredentialStore
	if cfg.MultiTenant != nil && cfg.MultiTenant.CredentialStore != "" {
		credentialStorePath = cfg.MultiTenant.CredentialStore
	}
	keyFile := credentialStorePath + "/.key"
	credentialManager := sec.NewUserCredentialStore(credentialStorePath, keyFile)
	if err := credentialManager.Initialize(options.CredentialPass); err != nil {
		log.Fatalf("Error initializing credential manager: %v", err)
	}

	// Log startup diagnostics
	logStartupDiagnostics(cfg, options)

	if options.Verbose {
		fmt.Printf("Starting Slacker bot...\n")
		fmt.Printf("Config file: %s\n", options.ConfigFile)
		fmt.Printf("Session store: %s\n", options.SessionStore)
		fmt.Printf("Credential store: %s\n", options.CredentialStore)
	}

	// Load configuration
	cfg, err = loadConfig(options.ConfigFile)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Create multi-tenant tool set
	ctx := context.Background()
	tenantToolSet, err := query.NewTenantToolSet(ctx, cfg)
	if err != nil {
		log.Fatalf("Error creating tenant tool set: %v", err)
	}

	// Initialize approval workflow
	var adminUsers []string
	if cfg.MultiTenant != nil {
		adminUsers = cfg.MultiTenant.AdminUsers
	}
	approvalWorkflow := slacker.NewApprovalWorkflow(adminUsers, securityLogger)

	// Get Slack tokens from environment
	slackToken := os.Getenv("SLACK_BOT_TOKEN")
	if slackToken == "" {
		log.Fatalf("SLACK_BOT_TOKEN environment variable is required")
	}

	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		log.Fatalf("SLACK_APP_TOKEN environment variable is required")
	}

	// Create Slack bot
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
		log.Fatalf("Error creating Slack bot: %v", err)
	}

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Validate Slack setup before starting
	fmt.Println("Validating Slack setup...")
	if err := bot.ValidateSlackSetup(); err != nil {
		log.Fatalf("Slack setup validation failed: %v", err)
	}
	fmt.Println("✓ Slack setup validation passed")

	// Start bot in goroutine
	botCtx, cancel := context.WithCancel(ctx)
	go func() {
		if err := bot.StartSocketMode(botCtx); err != nil {
			log.Printf("Bot error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutting down Slacker bot...")

	// Cancel context to stop bot
	cancel()

	// Give some time for graceful shutdown
	time.Sleep(2 * time.Second)

	// Shutdown components
	if err := tenantToolSet.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down tool set: %v", err)
	}

	fmt.Println("Slacker bot stopped")
}

// loadConfig loads the configuration file
func loadConfig(configFile string) (*config.File, error) {
	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Create a default configuration
		if err := createDefaultConfig(configFile); err != nil {
			return nil, fmt.Errorf("creating default config: %w", err)
		}
		fmt.Printf("Created default configuration file: %s\n", configFile)
		fmt.Printf("Please edit this file and restart the bot\n")
		os.Exit(0)
	}

	// Load existing configuration
	return loadConfigFile(configFile)
}

// createDefaultConfig creates a default configuration file
func createDefaultConfig(configFile string) error {
	defaultConfig := `# Slacker Bot Configuration
# This uses Socket Mode API (recommended for modern Slack apps)
#
# Socket Mode Requirements:
# 1. Create a Slack App at https://api.slack.com/apps
# 2. Enable Socket Mode in the app settings  
# 3. Add the Bot User scope (bot, chat:write, etc.)
# 4. Add Event Subscriptions: message.channels, app_mention
# 5. Generate App Token (xapp-...) with socket:write scope
# 6. Generate Bot Token (xoxb-...) with required scopes
#
# Environment Variables Required:
# - SLACK_BOT_TOKEN: Bot token (xoxb-...)  
# - SLACK_APP_TOKEN: App token for Socket Mode (xapp-...)
#
# Start the bot with: ./slacker --config marvin.slacker.hcl --passphrase your-secret

model = "ministral-3:3b"

# Advanced model options for fine-tuning responses
options {
  # Context window size (tokens) - default: 2048
  context_window_size = 4096
  
  # Sampling temperature (0.0-1.0) - higher = more creative - default: 0.8
  temperature = 0.7
  
  # Nucleus sampling (0.0-1.0) - default: 0.9
  top_p = 0.9
  
  # Top-k sampling (-1 = no limit) - default: 40
  top_k = 40
  
  # Maximum tokens in response (-1 = unlimited) - default: -1
  num_predict = 2048
  
  # Repetition penalty - default: 1.1
  repeat_penalty = 1.1
  
  # How far back to check for repetitions (0 = disabled) - default: 64
  repeat_last_n = 64
  
  # Random seed for reproducible results (optional)
  seed = 42
  
  # Stop sequences to end generation (optional)
  stop = ["###", "END"]
}

# Multi-tenant settings
multi_tenant {
  admin_users = ["YOUR_ADMIN_USER_ID_HERE"]
  admin_channel = "C_ADMIN_CHANNEL_ID_HERE"  # Optional admin notification channel
  session_store_path = "./sessions"
  credential_store = "./credentials"
}

# Global HTTP MCP tools (available to all users, no approval required)
mcp_over_http "weather-api" {
  name = "weather-api"
  url = "https://weather.example.com/mcp"
}

# Pre-approved shared tools (admin-configured)
local_program "company-git" {
  name = "company-git"
  program = "/usr/local/bin/git-mcp"
  args = ["--read-only"]
  
  sharing {
    allowed_users = ["USER_ID_HERE"]
    can_share = false
  }
}

docker_mcp "shared-docker" {
  name = "shared-docker"
  image = "your-docker-tool:latest"
  
  sharing {
    allowed_users = ["USER_ID_HERE"]
    can_share = false
  }
}
`

	return os.WriteFile(configFile, []byte(defaultConfig), 0644)
}

// loadConfigFile loads a configuration file using Marvin's config system
func loadConfigFile(filePath string) (*config.File, error) {
	opts := config.NewCommandLineOptions()
	return opts.LoadFromPath(filePath)
}
