package main

import (
	"fmt"
	"os"
	"os/signal"

	llmfactory "github.com/meschbach/marvin/internal/llm/factory"
	"github.com/meschbach/marvin/internal/rag"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

func ragCommand(global *globalOptions) *cobra.Command {
	index := &cobra.Command{
		Use:   "index",
		Short: "Indexes all documents from the configuration file",
		Run: func(cmd *cobra.Command, args []string) {
			procContext, done := signal.NotifyContext(cmd.Context(), unix.SIGSTOP)
			defer done()

			file, problem := global.config.Load()
			if problem != nil {
				fmt.Fprintf(os.Stderr, "%s\n", problem.Error())
				return
			}

			embedder, err := llmfactory.NewEmbeddingProvider(procContext, file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create embedding provider: %s\n", err.Error())
				return
			}

			fmt.Printf("Indexing %d repositories\n", len(file.Documents))
			for _, docCfg := range file.Documents {
				collection := rag.NewCollection(docCfg, embedder)
				if err := collection.Index(procContext); err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				}
			}
		},
	}

	query := &cobra.Command{
		Use:   "query <store> <query>",
		Short: "Queries the RAG store",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			procContext, done := signal.NotifyContext(cmd.Context(), unix.SIGSTOP)
			defer done()

			file, problem := global.config.Load()
			if problem != nil {
				fmt.Fprintf(os.Stderr, "%s\n", problem.Error())
				return
			}

			embedder, err := llmfactory.NewEmbeddingProvider(procContext, file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create embedding provider: %s\n", err.Error())
				return
			}

			var docCfg = file.FindDocumentsBlock(args[0])
			if docCfg == nil {
				fmt.Fprintf(os.Stderr, "no documents block with name %q\n", args[0])
				return
			}

			collection := rag.NewCollection(docCfg, embedder)
			result, err := collection.Query(procContext, args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				return
			}
			fmt.Println(result)
		},
	}

	rag := &cobra.Command{
		Use:   "rag",
		Short: "Operations against the RAG store",
	}
	rag.AddCommand(index)
	rag.AddCommand(query)
	return rag
}
