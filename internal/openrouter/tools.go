package openrouter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/ollama/ollama/api"
	openrouter2 "github.com/revrost/go-openrouter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (o *LLM) convertToolCallsFromOpenRouter(ctx context.Context, toolCalls []openrouter2.ToolCall) []api.ToolCall {
	result := make([]api.ToolCall, 0, len(toolCalls))
	for i, tc := range toolCalls {
		if tc.Function.Name == "" {
			fmt.Printf("[openrouter/internalize] WARNING: Empty tool name from model: %#v\n", tc)
			span := trace.SpanFromContext(ctx)
			span.AddEvent("empty-tool", trace.WithAttributes(attribute.Int("index", i)))
			continue
		}

		args := api.NewToolCallFunctionArguments()
		if err := args.UnmarshalJSON([]byte(tc.Function.Arguments)); err != nil {
			args = api.NewToolCallFunctionArguments()
		}

		result = append(result, api.ToolCall{
			ID: tc.ID,
			Function: api.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}
	return result
}

func (o *LLM) convertToolCallsFromOllama(ctx context.Context, toolCalls []api.ToolCall) []openrouter2.ToolCall {
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

		// Convert arguments using ToMap() for proper serialization
		argsMap := tc.Function.Arguments.ToMap()
		argsBytes, err := json.Marshal(argsMap)
		if err != nil {
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
