package config

import (
	"context"
	"fmt"
	"math"

	"github.com/ollama/ollama/api"
)

type ollamaEncoder struct {
	client    *api.Client
	modelName string
}

func (o *ollamaEncoder) Encode(ctx context.Context, text string) ([]float32, error) {
	// Call Ollama embeddings endpoint
	resp, err := o.client.Embeddings(ctx, &api.EmbeddingRequest{
		Model:  o.modelName,
		Prompt: text,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embeddings: %w", err)
	}
	if len(resp.Embedding) == 0 {
		return nil, fmt.Errorf("ollama embeddings: empty vector")
	}

	// Convert to []float32 and normalize to unit length as required by chromem-go
	v32 := make([]float32, len(resp.Embedding))
	var sumSquares float64
	for i, f64 := range resp.Embedding {
		v32[i] = float32(f64)
		sumSquares += f64 * f64
	}
	norm := math.Sqrt(sumSquares)
	if norm == 0 {
		return v32, nil
	}
	inv := float32(1.0 / norm)
	for i := range v32 {
		v32[i] *= inv
	}
	return v32, nil
}
