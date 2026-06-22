package config

// DefaultEmbeddingModel is the fallback model for embeddings when not configured.
const DefaultEmbeddingModel = "mxbai-embed-large:latest"

// DocumentsBlock is a configuration stanza to index a path with the given context (labeled as description)
type DocumentsBlock struct {
	Name         string `hcl:"name,label"`
	DocumentPath string `hcl:"document_path,label"`
	StoragePath  string `hcl:"storage_path,optional"`
	Description  string `hcl:"description,optional"`
	Model        string `hcl:"model,optional"`
}

// QueryResult represents a single result from a document query.
type QueryResult struct {
	Path       string  `json:"path"`
	Similarity float32 `json:"similarity"`
}

// EmbeddingModel returns the configured embedding model or the default.
func (d *DocumentsBlock) EmbeddingModel() string {
	if d.Model != "" {
		return d.Model
	}
	return DefaultEmbeddingModel
}
