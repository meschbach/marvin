package openrouter

import (
	"encoding/json"
	"fmt"

	"github.com/ollama/ollama/api"
)

func (o *LLM) buildRequest(req *api.ChatRequest) (*Request, error) {
	openRouterReq := Request{
		Model:    o.model,
		Messages: req.Messages,
		Stream:   true,
	}

	if len(req.Tools) > 0 {
		toolsJSON, err := json.Marshal(req.Tools)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tools: %w", err)
		}
		openRouterReq.Tools = toolsJSON
	}

	if req.Options != nil {
		openRouterReq.Temperature = o.extractFloat32(req.Options, "temperature")
		openRouterReq.TopP = o.extractFloat32(req.Options, "top_p")
		openRouterReq.TopK = o.extractInt(req.Options, "top_k")
		openRouterReq.MaxTokens = o.extractInt(req.Options, "num_predict")
		openRouterReq.Seed = o.extractInt(req.Options, "seed")
		openRouterReq.Stop = o.extractStrings(req.Options, "stop")
	}

	return &openRouterReq, nil
}

func (o *LLM) extractFloat32(opts map[string]any, key string) *float32 {
	if val, ok := opts[key]; ok {
		if f, ok := val.(float64); ok {
			f := float32(f)
			return &f
		}
	}
	return nil
}

func (o *LLM) extractInt(opts map[string]any, key string) *int {
	if val, ok := opts[key]; ok {
		if f, ok := val.(float64); ok {
			i := int(f)
			return &i
		}
	}
	return nil
}

func (o *LLM) extractStrings(opts map[string]any, key string) []string {
	if val, ok := opts[key]; ok {
		if slice, ok := val.([]any); ok {
			strs := make([]string, len(slice))
			for i, v := range slice {
				if s, ok := v.(string); ok {
					strs[i] = s
				}
			}
			return strs
		}
	}
	return nil
}
