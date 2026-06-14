package query

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/gemini"
	"github.com/meschbach/marvin/internal/llm"
	"github.com/meschbach/marvin/internal/openrouter"
	"github.com/ollama/ollama/api"
)

type OllamaLLM struct {
	client *api.Client
}

func NewOllamaLLM(client *api.Client) *OllamaLLM {
	return &OllamaLLM{client: client}
}

func (o *OllamaLLM) Chat(ctx context.Context, req *llm.ChatRequest, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) error {
	apiReq, err := toAPIRequest(req)
	if err != nil {
		return fmt.Errorf("ollama: build request: %w", err)
	}

	return o.client.Chat(ctx, apiReq, func(cr api.ChatResponse) error {
		return onResponse(ctx, toChatResponse(&cr))
	})
}

func toAPIRequest(req *llm.ChatRequest) (*api.ChatRequest, error) {
	stream := true
	apiReq := &api.ChatRequest{
		Model:    req.Model,
		Messages: make([]api.Message, len(req.Messages)),
		Stream:   &stream,
		Options:  map[string]any{},
	}

	for i, msg := range req.Messages {
		apiReq.Messages[i] = api.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			Thinking:   msg.Thinking,
			ToolCallID: msg.ToolCallID,
		}
		if len(msg.ToolCalls) > 0 {
			apiReq.Messages[i].ToolCalls = make([]api.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				apiReq.Messages[i].ToolCalls[j] = api.ToolCall{
					ID: tc.ID,
					Function: api.ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: toAPIToolCallArgs(tc.Function.Arguments),
					},
				}
			}
		}
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

func NewLLM(ctx context.Context, cfg *config.File) (conversation.LLM, error) {
	return NewLLMForModel(ctx, cfg, cfg.LanguageModel())
}

// NewLLMForModel creates a new LLM using the specified model override.
func NewLLMForModel(ctx context.Context, cfg *config.File, model string) (conversation.LLM, error) {
	switch cfg.Provider() {
	case config.ProviderGemini:
		return newGeminiLLMForModel(ctx, cfg, model)
	case config.ProviderOpenRouter:
		return newOpenRouterLLMForModel(cfg, model)
	case config.ProviderOllama:
		fallthrough
	default:
		return newOllamaLLMFromEnv()
	}
}

func newGeminiLLM(ctx context.Context, cfg *config.File) (conversation.LLM, error) {
	return newGeminiLLMForModel(ctx, cfg, cfg.LanguageModel())
}

func newGeminiLLMForModel(ctx context.Context, cfg *config.File, model string) (conversation.LLM, error) {
	apiKey, has, err := cfg.Gemini.ResolveKey()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Gemini API key: %w", err)
	}
	if !has {
		return nil, fmt.Errorf("gemini API key is required. Set it via config, file, or GEMINI_API_KEY env var")
	}

	return gemini.NewLLM(ctx, apiKey, model)
}

func newOpenRouterLLM(cfg *config.File) (conversation.LLM, error) {
	return newOpenRouterLLMForModel(cfg, cfg.LanguageModel())
}

func newOpenRouterLLMForModel(cfg *config.File, model string) (conversation.LLM, error) {
	apiKey, has, err := cfg.OpenRouter.ResolveKey()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve OpenRouter API key: %w", err)
	}
	if !has {
		return nil, fmt.Errorf("openrouter API key is required. Set it via config, file (--openrouter-key-file), or OPENROUTER_API_KEY env var")
	}

	baseURL := ""
	if cfg.OpenRouter != nil && cfg.OpenRouter.BaseURL != "" {
		baseURL = cfg.OpenRouter.BaseURL
	}

	var retryConfig *config.RetryBlock
	if cfg.OpenRouter != nil {
		retryConfig = cfg.OpenRouter.Retry
	}

	return openrouter.NewLLM(apiKey, baseURL, model, retryConfig), nil
}

func newOllamaLLMFromEnv() (conversation.LLM, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama client: %w", err)
	}
	return NewOllamaLLM(client), nil
}
