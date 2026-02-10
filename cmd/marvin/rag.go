package main

import (
	"fmt"
	"os"
	"os/signal"

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

			fmt.Printf("Indexing %d repositories\n", len(file.Documents))
			for _, group := range file.Documents {
				if err := group.Index(procContext); err != nil {
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

			result, err := file.QueryRAGDocuments(procContext, args[0], args[1])
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
