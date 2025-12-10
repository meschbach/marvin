package query

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

var truthful = true
var truePtr = &truthful

// performWithConfig executes the query using the optional parsed configuration.
func performWithConfig(cfg *Config, cmd *cobra.Command, args []string) {
	actualQuery := strings.Join(args, " ")
	if actualQuery == "" {
		fmt.Fprintln(os.Stderr, "No query provided")
		_ = cmd.Help()
		return
	}
	fmt.Printf("Query:\t%s\n", actualQuery)

	// Query Ollama for a response
	client, err := api.ClientFromEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
		return
	}

	// Build tools from configuration (if provided)
	ctx := context.Background()
	toolset, tsErr := NewToolSet(ctx, cfg)
	if tsErr != nil {
		fmt.Fprintf(os.Stderr, "Error initializing tools: %v\n", tsErr)
		return
	}

	systemMessageContent := "You are a helpful assistant."
	if cfg != nil && cfg.SystemPrompt != nil {
		if len(cfg.SystemPrompt.FromString) > 0 {
			systemMessageContent = cfg.SystemPrompt.FromString
		}
		if len(cfg.SystemPrompt.FromFile) > 0 {
			contents, err := os.ReadFile(cfg.SystemPrompt.FromFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading system prompt file %q: %v\n", cfg.SystemPrompt.FromFile, err)
				return
			}
			systemMessageContent = string(contents)
		}
	}

	systemMessage := api.Message{
		Role: "system", Content: systemMessageContent,
	}

	// Maintain the rolling chat messages to support tool-call loops
	messages := []api.Message{
		systemMessage,
		{Role: "user", Content: actualQuery},
	}
	availableTools := toolset.APITools()
	//fmt.Println("Available tools:")
	//for _, tool := range availableTools {
	//	fmt.Printf("%s: %s\n", tool.Function.Name, tool.Function.Description)
	//}

	// ctx already defined above
	conversation := &ollamaConversation{
		client:   client,
		messages: messages,
		tools:    toolset,
	}
	model := "ministral-3:3b"
	if cfg != nil && cfg.Model != "" {
		model = cfg.Model
	}

	if err := conversation.runAIToConclusion(ctx, model, availableTools); err != nil {
		return
	}
}

// perform is the cobra Run function that handles parsing the --config flag and
// invoking the query with the parsed configuration.
func perform(cmd *cobra.Command, args []string) {
	var cfg *Config
	configPath, _ := cmd.Flags().GetString("config")
	if configPath != "" {
		parsed, err := LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config %q: %v\n", configPath, err)
			return
		}
		cfg = parsed
	}

	performWithConfig(cfg, cmd, args)
}

// New creates the `query` command that sends a prompt to Ollama and streams the response.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query <query...>",
		Short: "Send a free-form query to Ollama and print the response",
		Run:   perform,
	}

	// Add a --config (alias -c) flag to specify a configuration file to parse.
	cmd.Flags().StringP("config", "c", "", "Path to configuration file (HCL)")

	return cmd
}
