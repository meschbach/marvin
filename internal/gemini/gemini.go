package gemini

import (
	"context"
	"iter"

	"github.com/ollama/ollama/api"

	"google.golang.org/genai"
)

type Streamer interface {
	GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error]
}

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

func (g *LLM) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	return g.chat(ctx, req, fn)
}
