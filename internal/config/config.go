package config

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/spf13/cobra"
)

type CommandLineOptions struct {
	ConfigFile string
}

func NewCommandLineOptions() *CommandLineOptions {
	return &CommandLineOptions{
		ConfigFile: ".marvin.hcl",
	}
}

func (c *CommandLineOptions) PersistentFlags(forCommand *cobra.Command) {
	pflags := forCommand.PersistentFlags()
	pflags.StringVarP(&c.ConfigFile, "config", "c", c.ConfigFile, "path to the configuration file")
}

func (c *CommandLineOptions) Load() (*File, error) {
	file, err := loadConfig(c.ConfigFile)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func loadConfig(filePath string) (*File, error) {
	fmt.Printf("Loading config from %s\n", filePath)
	p := hclparse.NewParser()
	parsedContent, diags := p.ParseHCLFile(filePath)
	if diags != nil {
		return nil, diags
	}
	if parsedContent == nil {
		return nil, errors.New("parsed file is nil")
	}
	return interpretConfigFile(parsedContent, filePath)
}

func interpretConfigFile(parsedContent *hcl.File, workingPath string) (*File, error) {
	cfg := &File{}
	diags := gohcl.DecodeBody(parsedContent.Body, nil, cfg)
	if diags.HasErrors() {
		return nil, fmt.Errorf("decode HCL: %w", diags)
	}
	_, err := cfg.resolveWorkingDirectory(workingPath)
	return cfg, err
}
