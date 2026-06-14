package llm

import "context"

// EmbeddingRequest contains the model and input text for an embedding query.
type EmbeddingRequest struct {
	Model string
	Input string
}

// EmbeddingProvider is the interface for generating text embeddings.
// This is separate from the LLM interface — providers may implement
// one or both interfaces independently.
type EmbeddingProvider interface {
	Embeddings(ctx context.Context, req *EmbeddingRequest) ([]float32, error)
}
