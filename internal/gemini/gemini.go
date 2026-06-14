package gemini

import (
	"context"
	"iter"

	"github.com/meschbach/marvin/internal/llm"

	"google.golang.org/genai"
)

// Streamer defines the interface for generating content streams.
type Streamer interface {
	GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error]
}

// LLM represents a Gemini-powered language model.
type LLM struct {
	client Streamer
	model  string
}

type genaiClient struct {
	client *genai.Client
}

func (g *genaiClient) GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error] {
	return g.client.Models.GenerateContentStream(ctx, model, contents, config)
}

// NewLLM creates a new LLM instance with the provided API key and model.
func NewLLM(ctx context.Context, apiKey, model string) (*LLM, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, err
	}

	return &LLM{
		client: &genaiClient{client: client},
		model:  model,
	}, nil
}

// Chat executes a chat request and calls the provided function with responses.
func (g *LLM) Chat(ctx context.Context, req *llm.ChatRequest, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) error {
	return g.chat(ctx, req, onResponse)
}
