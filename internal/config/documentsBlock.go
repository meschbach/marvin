package config

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollama/ollama/api"
	"github.com/philippgille/chromem-go"
)

// DocumentsBlock is a configuration stanza to index a path with the given context (labeled as description)
type DocumentsBlock struct {
	Name         string `hcl:"name,label"`
	DocumentPath string `hcl:"document_path,label"`
	StoragePath  string `hcl:"storage_path,optional"`
	Description  string `hcl:"description,optional"`
	//Model is the embedding model to use
	Model string `hcl:"model,optional"`
}

func (d *DocumentsBlock) Query(ctx context.Context, query string) (string, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return "", fmt.Errorf("creating Ollama client for embeddings: %w", err)
	}
	embedder := &ollamaEncoder{client, d.EmbeddingModel()}

	db, err := chromem.NewPersistentDB(d.StoragePath, false)
	if err != nil {
		return "", fmt.Errorf("opening persistent DB: %w", err)
	}
	col := db.GetCollection(d.Name, embedder.Encode)
	documentCount := col.Count()
	if documentCount > 10 {
		documentCount = 10
	}
	result, err := col.Query(ctx, query, documentCount, nil, nil)
	if err != nil {
		return "", fmt.Errorf("querying collection: %w", err)
	}
	files := make([]string, len(result))
	for i, result := range result {
		files[i] = fmt.Sprintf("%s (%f)", result.Metadata["path"], result.Similarity)
	}
	return strings.Join(files, "\n"), nil
}

func (d *DocumentsBlock) EmbeddingModel() string {
	embedModel := d.Model
	if embedModel == "" {
		embedModel = DefaultEmbeddingModel
	}
	return embedModel
}

func (d *DocumentsBlock) Index(ctx context.Context) error {
	if d.DocumentPath == "" {
		return fmt.Errorf("documents.DocumentPath is empty")
	}
	if d.StoragePath == "" {
		return fmt.Errorf("documents.StoragePath is empty")
	}

	db, err := chromem.NewPersistentDB(d.StoragePath, false)
	if err != nil {
		panic(err)
	}
	meta := map[string]string{}
	if d.Description != "" {
		meta["description"] = d.Description
	}

	// Create a client once and capture in the closure
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return fmt.Errorf("creating Ollama client for embeddings: %w", err)
	}

	// Determine an embedding model from env or use a sensible default known to work with Ollama
	embeddingModel := d.EmbeddingModel()
	embedder := &ollamaEncoder{client, embeddingModel}
	fmt.Printf("Using embedding model %q\n", embeddingModel)

	col, err := db.GetOrCreateCollection(d.Name, meta, embedder.Encode)
	if err != nil {
		panic(err)
	}

	base, err := filepath.Abs(d.DocumentPath)
	if err != nil {
		return fmt.Errorf("resolving base path: %w", err)
	}

	var docs []chromem.Document
	err = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		// Compute relative id
		rel, err := filepath.Rel(base, path)
		if err != nil {
			rel = entry.Name()
		}

		// Read file content
		b, err := os.ReadFile(path)
		if err != nil {
			// Skip unreadable files but continue indexing others
			return nil
		}

		metadata := map[string]string{
			"path":     filepath.ToSlash(rel),
			"abs_path": path,
		}
		if d.Description != "" {
			metadata["description"] = d.Description
		}

		docs = append(docs, chromem.Document{
			ID:       filepath.ToSlash(rel),
			Metadata: metadata,
			Content:  string(b),
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking path %q: %w", base, err)
	}

	if len(docs) == 0 {
		return nil
	}

	// Add to collection; choose a moderate concurrency
	const concurrency = 4
	if err := col.AddDocuments(ctx, docs, concurrency); err != nil {
		return fmt.Errorf("adding documents: %w", err)
	}
	fmt.Printf("Added %d documents\n", len(docs))
	return nil
}
