package query

import (
	"context"
	"errors"
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
	if cfg.SystemPrompt != nil {
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

	for {
		req := &api.ChatRequest{
			Model:    "ministral-3:3b",
			Messages: messages,
			Tools:    availableTools,
			//Think:    &api.ThinkValue{Value: truePtr},
		}

		// Accumulate the assistant response and capture any tool calls
		var assistantOut strings.Builder
		var pendingCalls []api.ToolCall

		err = client.Chat(ctx, req, func(resp api.ChatResponse) error {
			if s := resp.Message.Content; s != "" {
				fmt.Print(s)
				assistantOut.WriteString(s)
			}
			if len(resp.Message.Thinking) > 0 {
				fmt.Printf("Thinking: %s\n", resp.Message.Thinking)
			}
			if len(resp.Message.ToolCalls) > 0 {
				fmt.Printf("Tool call: %s\n", resp.Message.ToolCalls[0].Function.Name)
				// Capture tool calls signaled by the model
				pendingCalls = resp.Message.ToolCalls
			}
			return nil
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError querying Ollama: %v\nAssitant buffer:%s\nPending calls: %#v\nTools:\n", err, assistantOut.String(), pendingCalls)
			for _, tool := range availableTools {
				fmt.Fprintf(os.Stderr, "\t%s: %s\n", tool.Function.Name, tool.Function.Description)
			}
			return
		}

		// Record the assistant turn (including tool calls, if any)
		assistantMsg := api.Message{
			Role:      "assistant",
			Content:   assistantOut.String(),
			ToolCalls: pendingCalls,
		}
		messages = append(messages, assistantMsg)

		// If there are no tool calls, we are done for this turn
		if len(pendingCalls) == 0 {
			fmt.Println()
			return
		}

		var pendingCallsErrors error
		// For each tool call, invoke via the toolset and append tool results
		for _, call := range pendingCalls {
			reply, err := toolset.HandleCall(ctx, call)
			pendingCallsErrors = errors.Join(err, pendingCallsErrors)
			for _, reply := range reply {
				fmt.Printf(">\t%s\t%s: %s\n", reply.Role, reply.Content, reply.ToolCallID)
			}
			messages = append(messages, reply...)
		}
		if pendingCallsErrors != nil {
			fmt.Printf("\nError invoking tools: %v\n", pendingCallsErrors)
			return
		}

		// Loop continues: the next iteration sends messages including tool outputs
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
