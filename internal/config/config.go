// Package config provides system configuration
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
	if err := validateProviderModels(cfg); err != nil {
		return nil, err
	}
	_, err := cfg.resolveWorkingDirectory(workingPath)
	return cfg, err
}

func validateProviderModels(cfg *File) error {
	hasLegacy := cfg.ProviderName != "" || cfg.Model != ""
	hasStructured := len(cfg.ProviderModels) > 0

	if hasLegacy && hasStructured {
		return fmt.Errorf("cannot use both legacy 'provider'/'model' and 'provider_model' blocks")
	}

	if !hasStructured {
		return nil
	}

	labelSet, err := validateProviderModelEntries(cfg.ProviderModels)
	if err != nil {
		return err
	}

	return validateLLMBlock(cfg.LLM, labelSet)
}

func validateProviderModelEntries(models []ProviderModelBlock) (map[string]bool, error) {
	validProviders := map[string]bool{
		"ollama":     true,
		"openrouter": true,
		"gemini":     true,
	}

	labelSet := make(map[string]bool, len(models))
	for _, pm := range models {
		if pm.Name == "" {
			return nil, fmt.Errorf("provider_model block requires a label")
		}
		if !validProviders[pm.Provider] {
			return nil, fmt.Errorf("provider_model %q: unknown provider %q (must be one of: ollama, openrouter, gemini)", pm.Name, pm.Provider)
		}
		if pm.Model == "" {
			return nil, fmt.Errorf("provider_model %q: model is required", pm.Name)
		}
		if labelSet[pm.Name] {
			return nil, fmt.Errorf("duplicate provider_model label %q", pm.Name)
		}
		labelSet[pm.Name] = true
	}

	return labelSet, nil
}

func validateLLMBlock(llmBlock *LLMBlock, labelSet map[string]bool) error {
	if llmBlock == nil {
		return nil
	}

	if len(llmBlock.Models) == 0 {
		return fmt.Errorf("llm block: 'models' list is empty; specify at least one model or remove the llm block")
	}
	for _, m := range llmBlock.Models {
		if !labelSet[m] {
			return fmt.Errorf("llm.models: %q does not match any provider_model label", m)
		}
	}

	return nil
}
