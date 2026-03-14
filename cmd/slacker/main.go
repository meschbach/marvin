package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/meschbach/marvin/internal/slacker/proc"
)

func main() {
	opts := proc.DefaultOptions()

	flag.StringVar(&opts.ConfigFile, "config", opts.ConfigFile, "Path to configuration file")
	flag.StringVar(&opts.SessionStore, "sessions", opts.SessionStore, "Directory to store sessions")
	flag.StringVar(&opts.CredentialStore, "credentials", opts.CredentialStore, "Directory to store encrypted credentials")
	flag.StringVar(&opts.CredentialPass, "passphrase", "", "Passphrase for credential encryption (required)")
	flag.StringVar(&opts.CredentialPassFile, "credential-pass-file", "", "File containing passphrase for credential encryption (alias: --passphrase-file)")
	flag.StringVar(&opts.CredentialPassFile, "passphrase-file", "", "File containing passphrase for credential encryption (alias: --credential-pass-file)")
	flag.BoolVar(&opts.Verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	if err := opts.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := proc.Run(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
