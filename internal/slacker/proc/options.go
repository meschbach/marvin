package proc

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	DefaultConfigFilename  = "marvin.slacker.hcl"
	DefaultSessionStore    = "./sessions"
	DefaultCredentialStore = "./credentials"
)

type Options struct {
	ConfigFile         string
	SessionStore       string
	CredentialStore    string
	CredentialPass     string
	CredentialPassFile string
	Verbose            bool
}

func DefaultOptions() *Options {
	return &Options{
		ConfigFile:      DefaultConfigFilename,
		SessionStore:    DefaultSessionStore,
		CredentialStore: DefaultCredentialStore,
		Verbose:         false,
	}
}

func (o *Options) Validate() error {
	if o.CredentialPass == "" && o.CredentialPassFile == "" {
		return errors.New("either --passphrase or --passphrase-file is required for credential encryption")
	}

	if o.CredentialPass != "" && o.CredentialPassFile != "" {
		return errors.New("cannot specify both --passphrase and --passphrase-file")
	}

	return nil
}

func (o *Options) ResolvePassphrase() (string, error) {
	if o.CredentialPassFile == "" {
		return o.CredentialPass, nil
	}

	data, err := os.ReadFile(o.CredentialPassFile)
	if err != nil {
		return "", fmt.Errorf("reading passphrase file: %w", err)
	}

	if info, err := os.Stat(o.CredentialPassFile); err == nil {
		mode := info.Mode()
		if mode.Perm()&0077 != 0 {
			fmt.Fprintf(os.Stderr, "Warning: Passphrase file %s has permissive permissions (%v). Consider using 0600 or more restrictive.\n", o.CredentialPassFile, mode.Perm())
		}
	}

	return strings.TrimSpace(string(data)), nil
}
