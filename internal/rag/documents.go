package rag

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/llm"
	"github.com/philippgille/chromem-go"
)

// Collection wraps a config.DocumentsBlock with embedding provider functionality.
type Collection struct {
	cfg      *config.DocumentsBlock
	embedder llm.EmbeddingProvider
}

// NewCollection creates a new Collection from a config.DocumentsBlock and embedding provider.
func NewCollection(cfg *config.DocumentsBlock, embedder llm.EmbeddingProvider) *Collection {
	return &Collection{cfg: cfg, embedder: embedder}
}

// QueryResult represents a single result from a document query.
type QueryResult struct {
	Path       string  `json:"path"`
	Similarity float32 `json:"similarity"`
}

// Query searches the indexed documents for the given query.
func (c *Collection) Query(ctx context.Context, query string) ([]QueryResult, error) {
	if c.embedder == nil {
		return nil, fmt.Errorf("embedding provider not set for documents block %q", c.cfg.Name)
	}

	db, err := chromem.NewPersistentDB(c.cfg.StoragePath, false)
	if err != nil {
		return nil, fmt.Errorf("opening persistent DB: %w", err)
	}

	encodeFunc := func(ctx context.Context, text string) ([]float32, error) {
		return c.embedder.Embeddings(ctx, &llm.EmbeddingRequest{
			Model: c.cfg.EmbeddingModel(),
			Input: text,
		})
	}

	col := db.GetCollection(c.cfg.Name, encodeFunc)
	if col == nil {
		return nil, nil
	}
	documentCount := col.Count()
	if documentCount > 10 {
		documentCount = 10
	}
	result, err := col.Query(ctx, query, documentCount, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("querying collection: %w", err)
	}
	files := make([]QueryResult, len(result))
	for i, result := range result {
		files[i] = QueryResult{
			Path:       result.Metadata["path"],
			Similarity: result.Similarity,
		}
	}
	return files, nil
}

// Index walks the document path and indexes all files into the vector database.
//
//nolint:gocyclo
func (c *Collection) Index(ctx context.Context) error {
	if c.embedder == nil {
		return fmt.Errorf("embedding provider not set for documents block %q", c.cfg.Name)
	}
	if c.cfg.DocumentPath == "" {
		return fmt.Errorf("documents.DocumentPath is empty")
	}
	if c.cfg.StoragePath == "" {
		return fmt.Errorf("documents.StoragePath is empty")
	}

	db, err := chromem.NewPersistentDB(c.cfg.StoragePath, false)
	if err != nil {
		panic(err)
	}
	meta := map[string]string{}
	if c.cfg.Description != "" {
		meta["description"] = c.cfg.Description
	}

	encodeFunc := func(ctx context.Context, text string) ([]float32, error) {
		return c.embedder.Embeddings(ctx, &llm.EmbeddingRequest{
			Model: c.cfg.EmbeddingModel(),
			Input: text,
		})
	}

	col, err := db.GetOrCreateCollection(c.cfg.Name, meta, encodeFunc)
	if err != nil {
		panic(err)
	}

	base, err := filepath.Abs(c.cfg.DocumentPath)
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
		rel, err := filepath.Rel(base, path)
		if err != nil {
			rel = entry.Name()
		}

		//nolint:gosec
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		metadata := map[string]string{
			"path":     filepath.ToSlash(rel),
			"abs_path": path,
		}
		if c.cfg.Description != "" {
			metadata["description"] = c.cfg.Description
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

	const concurrency = 4
	if err := col.AddDocuments(ctx, docs, concurrency); err != nil {
		return fmt.Errorf("adding documents: %w", err)
	}
	return nil
}
