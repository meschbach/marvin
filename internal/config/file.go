package config

import (
	"context"
	"fmt"
)

const DefaultEmbeddingModel = "mxbai-embed-large:latest"

// File represents a parsed configuration file
type File struct {
	// Model is the large language model to use
	Model         string              `hcl:"model,optional"`
	LocalPrograms []LocalProgramBlock `hcl:"local_program,block"`
	SystemPrompt  *SystemPromptBlock  `hcl:"system_prompt,block"`
	// Documents represents blocks fo contextual documents to manage
	Documents []*DocumentsBlock `hcl:"documents,block"`
}

func (f *File) QueryRAGDocuments(ctx context.Context, storeName, query string) (string, error) {
	var documentBlock *DocumentsBlock
	for _, doc := range f.Documents {
		if doc.Name == storeName {
			documentBlock = doc
		}
	}
	if documentBlock == nil {
		return "", fmt.Errorf("no documents block with name %q", storeName)
	}
	result, err := documentBlock.Query(ctx, query)
	return result, err
}

type LocalProgramBlock struct {
	Name    string   `hcl:"name,label"`
	Program string   `hcl:"program"`
	Args    []string `hcl:"args,optional"`
}

type SystemPromptBlock struct {
	FromString string `hcl:"from_string,optional"`
	FromFile   string `hcl:"from_file,optional"`
}
