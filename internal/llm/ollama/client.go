package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/meschbach/marvin/internal/llm"
	"github.com/ollama/ollama/api"
)

// Provider implements both llm.LLM and llm.EmbeddingProvider for Ollama.
type Provider struct {
	client *api.Client
}

// NewProvider creates a new Ollama provider from an existing api.Client.
func NewProvider(client *api.Client) *Provider {
	return &Provider{client: client}
}

// NewProviderFromEnv creates a new Ollama provider using environment
// configuration (OLLAMA_HOST, etc.).
func NewProviderFromEnv() (*Provider, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to create client: %w", err)
	}
	return NewProvider(client), nil
}

// Chat streams a chat response from Ollama. It converts the internal llm types
// to Ollama SDK types, calls the API, and converts responses back.
func (o *Provider) Chat(ctx context.Context, req *llm.ChatRequest, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) error {
	apiReq, err := toAPIRequest(req)
	if err != nil {
		return fmt.Errorf("ollama: build request: %w", err)
	}

	return o.client.Chat(ctx, apiReq, func(cr api.ChatResponse) error {
		return onResponse(ctx, toChatResponse(&cr))
	})
}

// Embeddings generates embeddings for the given input text.
func (o *Provider) Embeddings(ctx context.Context, req *llm.EmbeddingRequest) ([]float32, error) {
	resp, err := o.client.Embeddings(ctx, &api.EmbeddingRequest{
		Model:  req.Model,
		Prompt: req.Input,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embeddings: %w", err)
	}
	if len(resp.Embedding) == 0 {
		return nil, fmt.Errorf("ollama embeddings: empty vector")
	}

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

// toAPIRequest converts an internal ChatRequest to Ollama's api.ChatRequest.
func toAPIRequest(req *llm.ChatRequest) (*api.ChatRequest, error) {
	stream := true
	apiReq := &api.ChatRequest{
		Model:    req.Model,
		Messages: make([]api.Message, len(req.Messages)),
		Stream:   &stream,
		Options:  map[string]any{},
	}

	for i := range req.Messages {
		apiReq.Messages[i] = toAPIMessage(&req.Messages[i])
	}

	if req.Temperature != nil {
		apiReq.Options["temperature"] = *req.Temperature
	}
	if req.TopK != nil {
		apiReq.Options["top_k"] = *req.TopK
	}
	if req.TopP != nil {
		apiReq.Options["top_p"] = *req.TopP
	}

	if len(req.Tools) > 0 {
		apiReq.Tools = make(api.Tools, len(req.Tools))
		for i, t := range req.Tools {
			apiReq.Tools[i] = toAPITool(t)
		}
	}

	return apiReq, nil
}

// toAPIMessage converts an internal Message to Ollama's api.Message.
func toAPIMessage(msg *llm.Message) api.Message {
	out := api.Message{
		Role:       msg.Role,
		Content:    msg.Content,
		Thinking:   msg.Thinking,
		ToolCallID: msg.ToolCallID,
	}
	if len(msg.ToolCalls) > 0 {
		out.ToolCalls = make([]api.ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			out.ToolCalls[i] = api.ToolCall{
				ID: tc.ID,
				Function: api.ToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: toAPIToolCallArgs(tc.Function.Arguments),
				},
			}
		}
	}
	return out
}

// toAPIToolCallArgs converts any (typically map[string]any) to
// api.ToolCallFunctionArguments via JSON round-trip.
func toAPIToolCallArgs(args any) api.ToolCallFunctionArguments {
	apiArgs := api.NewToolCallFunctionArguments()
	if args == nil {
		return apiArgs
	}
	b, err := json.Marshal(args)
	if err != nil {
		return apiArgs
	}
	if err := apiArgs.UnmarshalJSON(b); err != nil {
		return api.NewToolCallFunctionArguments()
	}
	return apiArgs
}

// toAPITool converts an internal ToolDefinition to Ollama's api.Tool.
func toAPITool(t llm.ToolDefinition) api.Tool {
	var params api.ToolFunctionParameters
	if t.Function.Parameters != nil {
		b, err := json.Marshal(t.Function.Parameters)
		if err == nil {
			_ = json.Unmarshal(b, &params)
		}
	}
	return api.Tool{
		Type: t.Type,
		Function: api.ToolFunction{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  params,
		},
	}
}

// toChatResponse converts an Ollama api.ChatResponse to internal types.
func toChatResponse(cr *api.ChatResponse) *llm.ChatResponse {
	resp := llm.ChatResponse{
		Done: cr.Done,
	}
	if cr.Done {
		resp.Stats = llm.Stats{
			PromptTokens:   cr.PromptEvalCount,
			ResponseTokens: cr.EvalCount,
			TotalTokens:    cr.PromptEvalCount + cr.EvalCount,
			DoneReason:     cr.DoneReason,
		}
	}
	resp.Content = cr.Message.Content
	resp.Thinking = cr.Message.Thinking
	if len(cr.Message.ToolCalls) > 0 {
		resp.ToolCalls = make([]llm.ToolCall, len(cr.Message.ToolCalls))
		for i, tc := range cr.Message.ToolCalls {
			resp.ToolCalls[i] = llm.ToolCall{
				ID: tc.ID,
				Function: llm.ToolCallFunction{
					Index:     tc.Function.Index,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments.ToMap(),
				},
			}
		}
	}
	return &resp
}
