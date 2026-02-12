package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

func listModelsCommand(global *globalOptions) *cobra.Command {
	var showAccess bool

	cmd := &cobra.Command{
		Use:   "list-models",
		Short: "List available Ollama models and their access status",
		Long: `List all available models from Ollama with their access status.
This command shows which models are available and whether they are allowed
for Slacker operations based on model access control configuration.`,
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := global.config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				return
			}

			// Create Ollama client
			client, err := api.ClientFromEnvironment()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
				return
			}

			// List available models
			ctx := context.Background()
			models, err := client.List(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
				return
			}

			if len(models.Models) == 0 {
				fmt.Println("No models found in Ollama.")
				return
			}

			// Create tabwriter for aligned output
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

			if showAccess && cfg.MultiTenant != nil {
				// Show access information for Slacker operations
				if _, err := fmt.Fprintln(w, "MODEL\tSIZE\tSTATUS\tACCESS"); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing header: %v\n", err)
					return
				}
				if _, err := fmt.Fprintln(w, "-----\t----\t------\t------"); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing separator: %v\n", err)
					return
				}

				for _, model := range models.Models {
					// Check access for a regular user (empty userID)
					allowed, reason := cfg.ValidateModelAccess(model.Name, "")

					status := "Available"
					access := "✅ Allowed"
					if !allowed {
						status = "Denied"
						access = fmt.Sprintf("❌ %s", reason)
					}

					if model.Name == config.DefaultLanguageModel {
						access += " (default)"
					}

					if _, err := fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", model.Name, model.Size, status, access); err != nil {
						fmt.Fprintf(os.Stderr, "Error writing model info: %v\n", err)
						return
					}
				}
			} else {
				// Simple model list
				if _, err := fmt.Fprintln(w, "MODEL\tSIZE"); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing header: %v\n", err)
					return
				}
				if _, err := fmt.Fprintln(w, "-----\t----"); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing separator: %v\n", err)
					return
				}

				for _, model := range models.Models {
					if _, err := fmt.Fprintf(w, "%s\t%d\n", model.Name, model.Size); err != nil {
						fmt.Fprintf(os.Stderr, "Error writing model info: %v\n", err)
						return
					}
				}
			}

			if err := w.Flush(); err != nil {
				fmt.Fprintf(os.Stderr, "Error flushing output: %v\n", err)
			}

			// Show current model configuration
			currentModel := cfg.LanguageModel()
			fmt.Printf("\nCurrent model: %s\n", currentModel)

			if showAccess && cfg.MultiTenant != nil {
				// Show access configuration summary
				if accessState, err := cfg.GetEffectiveModelAccess(); err == nil {
					fmt.Printf("\nModel Access Configuration:\n")
					if len(accessState.AllowedModels) == 0 && len(accessState.DeniedModels) == 0 {
						fmt.Printf("  No restrictions in place\n")
					} else {
						if len(accessState.AllowedModels) > 0 {
							fmt.Printf("  Allowed models: %d\n", len(accessState.AllowedModels))
						}
						if len(accessState.DeniedModels) > 0 {
							fmt.Printf("  Denied models: %d\n", len(accessState.DeniedModels))
						}
					}
				}
			}
		},
	}

	cmd.Flags().BoolVar(&showAccess, "access", false, "Show model access status for Slacker operations")

	return cmd
}
