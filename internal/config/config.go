package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/spf13/cobra"
)

type CommandLineOptions struct {
	ConfigFile        string
	OpenRouterKeyFile string
}

func NewCommandLineOptions() *CommandLineOptions {
	configFile := ".marvin.hcl"
	if envConfig := os.Getenv("MARVIN_CONFIG"); envConfig != "" {
		configFile = envConfig
	}
	return &CommandLineOptions{
		ConfigFile: configFile,
	}
}

func (c *CommandLineOptions) PersistentFlags(forCommand *cobra.Command) {
	pflags := forCommand.PersistentFlags()
	pflags.StringVarP(&c.ConfigFile, "config", "c", c.ConfigFile, "path to the configuration file")
	pflags.StringVar(&c.OpenRouterKeyFile, "openrouter-key-file", "", "path to a file containing the OpenRouter API key")
}

func (c *CommandLineOptions) Load() (*File, error) {
	file, err := loadConfig(c.ConfigFile)
	if err != nil {
		return nil, err
	}
	// Apply CLI flag for OpenRouter key file (takes priority over config file)
	if c.OpenRouterKeyFile != "" {
		if file.OpenRouter == nil {
			file.OpenRouter = &OpenRouterBlock{}
		}
		file.OpenRouter.APIKeyFile = c.OpenRouterKeyFile
	}
	return file, nil
}

// LoadFromPath loads configuration from a specific path
func (c *CommandLineOptions) LoadFromPath(filePath string) (*File, error) {
	c.ConfigFile = filePath
	return c.Load()
}

func loadConfig(filePath string) (*File, error) {
	//fmt.Printf("Loading config from %s\n", filePath)
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
