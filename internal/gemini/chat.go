// Package gemini provides LLM integration with Google's Gemini API via Model Context Protocol.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"

	"google.golang.org/genai"
)

func (g *LLM) chat(ctx context.Context, req *api.ChatRequest, onEvent conversation.ChatResponseListener) error {
	contents, config, err := convertToGenAIRequest(req)
	if err != nil {
		return err
	}

	stream := g.client.GenerateContentStream(ctx, g.model, contents, config)

	for resp, err := range stream {
		if err != nil {
			return err
		}
		ollamaResp, finishErr := convertToOllamaResponse(resp)
		if finishErr != nil {
			return finishErr
		}
		if err := onEvent.OnChatResponse(ctx, ollamaResp); err != nil {
			return err
		}
	}

	return nil
}

func convertToGenAIRequest(req *api.ChatRequest) ([]*genai.Content, *genai.GenerateContentConfig, error) {
	systemInstruction, userContents, err := convertMessages(req.Messages)
	if err != nil {
		return nil, nil, err
	}
	generationConfig, err := convertOptions(req.Options)
	if err != nil {
		return nil, nil, err
	}
	generationConfig.SystemInstruction = systemInstruction
	generationConfig.Tools = convertTools(req.Tools)

	return userContents, generationConfig, nil
}

func convertMessages(messages []api.Message) (systemInstruction *genai.Content, userContents []*genai.Content, err error) {
	userContents = make([]*genai.Content, 0, len(messages))

	for i := range messages {
		msg := &messages[i]
		if msg.Role == "system" {
			systemInstruction = genai.NewContentFromText(msg.Content, "system")
			continue
		}

		converted, err := convertSingleMessage(msg, i)
		if err != nil {
			return nil, nil, err
		}
		if converted != nil {
			userContents = append(userContents, converted)
		}
	}

	return systemInstruction, userContents, nil
}

func convertSingleMessage(msg *api.Message, index int) (*genai.Content, error) {
	role := msg.Role
	switch role {
	case "tool":
		return convertToolResult(msg), nil
	case conversation.RoleAssistant:
		return convertAssistantMessage(msg, index)
	default:
		return convertUserMessage(msg, role, index)
	}
}

func convertAssistantMessage(msg *api.Message, index int) (*genai.Content, error) {
	if msg.Content == "" && len(msg.ToolCalls) > 0 {
		parts, err := convertToolCalls(msg.ToolCalls)
		if err != nil {
			return nil, err
		}
		return genai.NewContentFromParts(parts, genai.RoleModel), nil
	}
	if msg.Content == "" {
		fmt.Printf("warn\tSkipping empty assistant message @ %d (no tool calls)\n", index)
		return nil, nil
	}
	return genai.NewContentFromText(msg.Content, genai.RoleModel), nil
}

func convertToolCalls(toolCalls []api.ToolCall) ([]*genai.Part, error) {
	parts := make([]*genai.Part, 0, len(toolCalls))
	for _, tc := range toolCalls {
		fc, err := convertFunctionCallFromAPI(tc)
		if err != nil {
			return nil, err
		}
		part := genai.NewPartFromFunctionResponse(tc.Function.Name, fc.Args)
		parts = append(parts, part)
	}
	return parts, nil
}

func convertUserMessage(msg *api.Message, role string, index int) (*genai.Content, error) {
	if msg.Content == "" {
		fmt.Printf("warn\tSkipping empty %s message @ %d\n", role, index)
		return nil, nil
	}
	return genai.NewContentFromText(msg.Content, genai.Role(role)), nil
}

func convertToolResult(msg *api.Message) *genai.Content {
	part := genai.NewPartFromFunctionResponse(msg.ToolName, map[string]any{"result": msg.Content})
	return genai.NewContentFromParts([]*genai.Part{part}, "user")
}

func convertOptions(opts map[string]any) (*genai.GenerateContentConfig, error) {
	generationConfig := &genai.GenerateContentConfig{}

	if opts == nil {
		return generationConfig, nil
	}

	convertFloatOption(opts, "temperature", func(v float32) {
		generationConfig.Temperature = &v
	})
	convertFloatOption(opts, "top_p", func(v float32) {
		generationConfig.TopP = &v
	})
	convertTopKOption(opts, func(v float32) {
		generationConfig.TopK = &v
	})
	if err := convertIntOption(opts, "num_predict", func(v int32) {
		generationConfig.MaxOutputTokens = v
	}); err != nil {
		return nil, err
	}
	convertStopSequences(opts, generationConfig)

	return generationConfig, nil
}

func convertFloatOption(opts map[string]any, key string, setter func(float32)) {
	if temp, ok := opts[key].(float64); ok {
		setter(float32(temp))
	}
}

func convertTopKOption(opts map[string]any, setter func(float32)) {
	if val, ok := opts["top_k"].(int); ok {
		setter(float32(val))
	}
}

func convertIntOption(opts map[string]any, key string, setter func(int32)) error {
	if val, ok := opts[key].(int); ok {
		if val > math.MaxInt32 || val < math.MinInt32 {
			return fmt.Errorf("value for %s exceeds int32 bounds", key)
		}
		setter(int32(val))
	}
	return nil
}

func convertStopSequences(opts map[string]any, config *genai.GenerateContentConfig) {
	if stop, ok := opts["stop"].([]any); ok {
		stopSeqs := make([]string, 0, len(stop))
		for _, s := range stop {
			if str, ok := s.(string); ok {
				stopSeqs = append(stopSeqs, str)
			}
		}
		config.StopSequences = stopSeqs
	}
}

var finishReasonErrors = map[genai.FinishReason]string{
	genai.FinishReasonStop:                   "", // no error
	genai.FinishReasonMaxTokens:              "", // no error
	genai.FinishReasonUnexpectedToolCall:     "model attempted to call a tool but none were provided",
	genai.FinishReasonSafety:                 "response blocked due to safety policy",
	genai.FinishReasonRecitation:             "response blocked due to recitation policy",
	genai.FinishReasonOther:                  "response stopped for unknown reason",
	genai.FinishReasonUnspecified:            "response finish reason unspecified",
	genai.FinishReasonLanguage:               "response blocked due to language policy",
	genai.FinishReasonBlocklist:              "response blocked due to blocklist policy",
	genai.FinishReasonProhibitedContent:      "response blocked due to prohibited content",
	genai.FinishReasonSPII:                   "response blocked due to sensitive personal information",
	genai.FinishReasonMalformedFunctionCall:  "response has malformed function call",
	genai.FinishReasonImageSafety:            "response blocked due to image safety policy",
	genai.FinishReasonImageProhibitedContent: "response blocked due to prohibited image content",
	genai.FinishReasonNoImage:                "response blocked: no image provided",
	genai.FinishReasonImageRecitation:        "response blocked due to image recitation policy",
	genai.FinishReasonImageOther:             "response blocked due to image policy",
}

func handleFinishReason(reason genai.FinishReason) error {
	if errMsg, ok := finishReasonErrors[reason]; ok {
		if errMsg != "" {
			return errors.New(errMsg)
		}
		return nil
	}
	return nil
}

func convertTools(tools api.Tools) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]*genai.Tool, len(tools))
	for i := range tools {
		t := &tools[i]
		funcDecl := &genai.FunctionDeclaration{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  convertSchema(&t.Function.Parameters),
		}
		result[i] = &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{funcDecl},
		}
	}
	return result
}

func convertSchema(params *api.ToolFunctionParameters) *genai.Schema {
	schema := &genai.Schema{
		Type: genai.TypeObject,
	}

	if params.Properties != nil {
		properties := make(map[string]*genai.Schema)
		for k, v := range params.Properties.ToMap() {
			properties[k] = convertToolProperty(&v)
		}
		schema.Properties = properties
	}

	if len(params.Required) > 0 {
		schema.Required = params.Required
	}

	return schema
}

func convertToolProperty(prop *api.ToolProperty) *genai.Schema {
	schema := &genai.Schema{}
	schema.Type = convertType(prop.Type)
	schema.Description = prop.Description

	if len(prop.Enum) > 0 {
		schema.Enum = convertEnum(prop.Enum)
	}

	return schema
}

// convertType maps API property types to GenAI schema types
func convertType(types []string) genai.Type {
	for _, t := range types {
		switch t {
		case "string":
			return genai.TypeString
		case "integer":
			return genai.TypeInteger
		case "number":
			return genai.TypeNumber
		case "boolean":
			return genai.TypeBoolean
		case "array":
			return genai.TypeArray
		case "object":
			return genai.TypeObject
		}
	}
	return genai.TypeString // default
}

// convertEnum extracts string values from enum array
func convertEnum(enum []any) []string {
	result := make([]string, 0, len(enum))
	for _, e := range enum {
		if s, ok := e.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func convertFunctionCallFromAPI(tc api.ToolCall) (*genai.FunctionCall, error) {
	argsJSON, err := json.Marshal(tc.Function.Arguments)
	if err != nil {
		return &genai.FunctionCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
		}, fmt.Errorf("failed to marshal tool call arguments: %w", err)
	}
	var args map[string]any
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool call arguments: %w", err)
	}
	return &genai.FunctionCall{
		ID:   tc.ID,
		Name: tc.Function.Name,
		Args: args,
	}, nil
}

func convertFunctionCall(fc *genai.FunctionCall) api.ToolCall {
	args := api.NewToolCallFunctionArguments()
	if fc.Args != nil {
		argsJSON, err := json.Marshal(fc.Args)
		if err == nil {
			if err := args.UnmarshalJSON(argsJSON); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to unmarshal tool call arguments: %v\n", err)
			}
		}
	}

	return api.ToolCall{
		ID: fc.ID,
		Function: api.ToolCallFunction{
			Name:      fc.Name,
			Arguments: args,
		},
	}
}

func convertToOllamaResponse(resp *genai.GenerateContentResponse) (*api.ChatResponse, error) {
	ollamaResp := &api.ChatResponse{
		Model:     resp.ModelVersion,
		CreatedAt: time.Now(),
		Done:      false,
	}

	done, err := handleFinishReasonIfPresent(resp)
	if err != nil {
		return nil, err
	}
	ollamaResp.Done = done

	ollamaResp.Message = extractMessageFromCandidates(resp)

	if resp.UsageMetadata != nil {
		ollamaResp.PromptEvalCount = int(resp.UsageMetadata.PromptTokenCount)
		ollamaResp.EvalCount = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return ollamaResp, nil
}

func handleFinishReasonIfPresent(resp *genai.GenerateContentResponse) (bool, error) {
	if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != "" {
		if resp.Candidates[0].FinishReason != genai.FinishReasonStop {
			finishErr := handleFinishReason(resp.Candidates[0].FinishReason)
			if finishErr != nil {
				return false, finishErr
			}
		}
		return true, nil
	}
	return false, nil
}

func extractMessageFromCandidates(resp *genai.GenerateContentResponse) api.Message {
	if len(resp.Candidates) == 0 ||
		resp.Candidates[0].Content == nil ||
		len(resp.Candidates[0].Content.Parts) == 0 {
		return api.Message{}
	}

	var msg api.Message
	for _, part := range resp.Candidates[0].Content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" {
			msg = api.Message{
				Role:    "assistant",
				Content: part.Text,
			}
		}
		if part.FunctionCall != nil {
			toolCall := convertFunctionCall(part.FunctionCall)
			msg.ToolCalls = append(msg.ToolCalls, toolCall)
		}
	}
	return msg
}
