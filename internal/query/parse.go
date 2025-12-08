package query

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

func LoadConfig(filePath string) (*Config, error) {
	fmt.Printf("Loading config from %s\n", filePath)
	p := hclparse.NewParser()
	parsedContent, diags := p.ParseHCLFile(filePath)
	if diags != nil {
		return nil, diags
	}
	if parsedContent == nil {
		return nil, errors.New("parsed file is nil")
	}
	return interpretConfigFile(parsedContent)
}

func interpretConfigFile(parsedContent *hcl.File) (*Config, error) {
	var cfg Config
	diags := gohcl.DecodeBody(parsedContent.Body, nil, &cfg)
	if diags.HasErrors() {
		return nil, fmt.Errorf("decode HCL: %w", diags)
	}
	return &cfg, nil
}
