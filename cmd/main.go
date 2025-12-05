package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

func main() {
	mcpList := &cobra.Command{
		Use: "list",
		Run: func(cmd *cobra.Command, args []string) {

		},
	}

	mcp := &cobra.Command{
		Use: "mcp",
	}
	mcp.AddCommand(mcpList)

	query := &cobra.Command{
		Use: "query <query...>",
		Run: func(cmd *cobra.Command, args []string) {
			actualQuery := strings.Join(args, " ")
			if actualQuery == "" {
				fmt.Fprintln(os.Stderr, "No query provided")
				cmd.Help()
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
		},
	}

	root := &cobra.Command{
		Use:   "marvin",
		Short: "An AI workbench experiment backed by ollama",
	}
	root.AddCommand(mcp)
	root.AddCommand(query)

	if err := root.Execute(); err != nil {
		panic(err)
	}
}
