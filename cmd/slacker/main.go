package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/meschbach/marvin/internal/slacker"
	sec "github.com/meschbach/marvin/internal/slacker/security"
)

//go:embed default_config.hcl
var defaultConfigFile embed.FS

const (
	defaultConfigFilename  = "marvin.slacker.hcl"
	defaultSessionStore    = "./sessions"
	defaultCredentialStore = "./credentials"
)

// SlackerOptions holds command line options
type SlackerOptions struct {
	ConfigFile         string
	SessionStore       string
	CredentialStore    string
	CredentialPass     string
	CredentialPassFile string
	Verbose            bool
}

// readPassphraseFromFile securely reads passphrase from file
func readPassphraseFromFile(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading passphrase file: %w", err)
	}

	// Check file permissions and warn if too permissive
	if info, err := os.Stat(filePath); err == nil {
		mode := info.Mode()
		if mode.Perm()&0077 != 0 {
			fmt.Fprintf(os.Stderr, "Warning: Passphrase file %s has permissive permissions (%v). Consider using 0600 or more restrictive.\n", filePath, mode.Perm())
		}
	}

	return strings.TrimSpace(string(data)), nil
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

//nolint:funlen,gocyclo
func main() {
	var (
		configFile         = flag.String("config", defaultConfigFilename, "Path to configuration file")
		sessionStore       = flag.String("sessions", defaultSessionStore, "Directory to store sessions")
		credentialStore    = flag.String("credentials", defaultCredentialStore, "Directory to store encrypted credentials")
		credentialPass     = flag.String("passphrase", "", "Passphrase for credential encryption (required)")
		credentialPassFile = flag.String("passphrase-file", "", "File containing passphrase for credential encryption")
		verbose            = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()

	options := &SlackerOptions{
		ConfigFile:         *configFile,
		SessionStore:       *sessionStore,
		CredentialStore:    *credentialStore,
		CredentialPass:     *credentialPass,
		CredentialPassFile: *credentialPassFile,
		Verbose:            *verbose,
	}

	if options.CredentialPass == "" && options.CredentialPassFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --passphrase or --passphrase-file is required for credential encryption\n")
		flag.Usage()
		os.Exit(1)
	}

	if options.CredentialPass != "" && options.CredentialPassFile != "" {
		fmt.Fprintf(os.Stderr, "Error: Cannot specify both --passphrase and --passphrase-file\n")
		os.Exit(1)
	}

	// Read from file if file flag is provided
	if options.CredentialPassFile != "" {
		pass, err := readPassphraseFromFile(options.CredentialPassFile)
		if err != nil {
			log.Fatalf("Error reading passphrase file: %v", err)
		}
		options.CredentialPass = pass
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
	content, err := defaultConfigFile.ReadFile("default_config.hcl")
	if err != nil {
		return fmt.Errorf("reading embedded config: %w", err)
	}

	return os.WriteFile(configFile, content, 0644)
}

// loadConfigFile loads a configuration file using Marvin's config system
func loadConfigFile(filePath string) (*config.File, error) {
	opts := config.NewCommandLineOptions()
	return opts.LoadFromPath(filePath)
}
