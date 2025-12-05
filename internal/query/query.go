package query

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

func perform(cmd *cobra.Command, args []string) {
	actualQuery := strings.Join(args, " ")
	if actualQuery == "" {
		fmt.Fprintln(os.Stderr, "No query provided")
		_ = cmd.Help()
		return
	}
	fmt.Println(actualQuery)

	// Query Ollama for a response
	client, err := api.ClientFromEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
		return
	}

	req := &api.ChatRequest{
		Model: "ministral-3:3b",
		Messages: []api.Message{
			{
				Role:    "user",
				Content: actualQuery,
			},
		},
	}

	ctx := context.Background()
	err = client.Chat(ctx, req, func(resp api.ChatResponse) error {
		fmt.Print(resp.Message.Content)
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError querying Ollama: %v\n", err)
		return
	}
	fmt.Println()
}

// New creates the `query` command that sends a prompt to Ollama and streams the response.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query <query...>",
		Short: "Send a free-form query to Ollama and print the response",
		Run:   perform,
	}

	return cmd
}
