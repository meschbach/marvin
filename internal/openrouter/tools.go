package openrouter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/meschbach/marvin/internal/llm"
	openrouter2 "github.com/revrost/go-openrouter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (o *LLM) convertToolCallsFromOpenRouter(ctx context.Context, toolCalls []openrouter2.ToolCall) []llm.ToolCall {
	result := make([]llm.ToolCall, 0, len(toolCalls))
	for i, tc := range toolCalls {
		if tc.Function.Name == "" {
			fmt.Printf("[openrouter/internalize] WARNING: Empty tool name from model: %#v\n", tc)
			span := trace.SpanFromContext(ctx)
			span.AddEvent("empty-tool", trace.WithAttributes(attribute.Int("index", i)))
			continue
		}

		var args any
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]any{}
			}
		} else {
			args = map[string]any{}
		}

		result = append(result, llm.ToolCall{
			ID: tc.ID,
			Function: llm.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}
	return result
}

func (o *LLM) convertToolCallsFromOllama(ctx context.Context, toolCalls []llm.ToolCall) []openrouter2.ToolCall {
	result := make([]openrouter2.ToolCall, 0, len(toolCalls))
	for i, tc := range toolCalls {
		if tc.Function.Name == "" {
			fmt.Printf("[openrouter/externalize] WARNING: Empty tool name: %#v\n", tc)
			span := trace.SpanFromContext(ctx)
			span.AddEvent("empty-tool", trace.WithAttributes(attribute.Int("index", i)))
			continue
		}
		// Ensure ID is set
		id := tc.ID
		if id == "" {
			id = "call_" + uuid.New().String()[:8]
		}

		var argsBytes []byte
		if tc.Function.Arguments != nil {
			var err error
			argsBytes, err = json.Marshal(tc.Function.Arguments)
			if err != nil {
				argsBytes = []byte("{}")
			}
		} else {
			argsBytes = []byte("{}")
		}

		index := tc.Function.Index
		result = append(result, openrouter2.ToolCall{
			ID:   id,
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: string(argsBytes),
			},
			Index: &index,
		})
	}
	return result
}
