// Package gemini provides LLM integration with Google's Gemini API via Model Context Protocol.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/llm"

	"google.golang.org/genai"
)

func (g *LLM) chat(ctx context.Context, req *llm.ChatRequest, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) error {
	contents, config, err := convertToGenAIRequest(req)
	if err != nil {
		return err
	}

	stream := g.client.GenerateContentStream(ctx, g.model, contents, config)

	for resp, err := range stream {
		if err != nil {
			return err
		}
		llmResp, finishErr := convertToLLMResponse(resp)
		if finishErr != nil {
			return finishErr
		}
		if err := onResponse(ctx, llmResp); err != nil {
			return err
		}
	}

	return nil
}

func convertToGenAIRequest(req *llm.ChatRequest) ([]*genai.Content, *genai.GenerateContentConfig, error) {
	systemInstruction, userContents, err := convertMessages(req.Messages)
	if err != nil {
		return nil, nil, err
	}
	generationConfig, err := convertOptions(req)
	if err != nil {
		return nil, nil, err
	}
	generationConfig.SystemInstruction = systemInstruction
	generationConfig.Tools = convertTools(req.Tools)

	return userContents, generationConfig, nil
}

func convertMessages(messages []llm.Message) (systemInstruction *genai.Content, userContents []*genai.Content, err error) {
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

func convertSingleMessage(msg *llm.Message, index int) (*genai.Content, error) {
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

func convertAssistantMessage(msg *llm.Message, index int) (*genai.Content, error) {
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

func convertToolCalls(toolCalls []llm.ToolCall) ([]*genai.Part, error) {
	parts := make([]*genai.Part, 0, len(toolCalls))
	for _, tc := range toolCalls {
		fc, err := convertFunctionCallFromLLM(tc)
		if err != nil {
			return nil, err
		}
		part := genai.NewPartFromFunctionResponse(tc.Function.Name, fc.Args)
		parts = append(parts, part)
	}
	return parts, nil
}

func convertUserMessage(msg *llm.Message, role string, index int) (*genai.Content, error) {
	if msg.Content == "" {
		fmt.Printf("warn\tSkipping empty %s message @ %d\n", role, index)
		return nil, nil
	}
	return genai.NewContentFromText(msg.Content, genai.Role(role)), nil
}

func convertToolResult(msg *llm.Message) *genai.Content {
	part := genai.NewPartFromFunctionResponse(msg.ToolName, map[string]any{"result": msg.Content})
	return genai.NewContentFromParts([]*genai.Part{part}, "user")
}

func convertOptions(req *llm.ChatRequest) (*genai.GenerateContentConfig, error) {
	generationConfig := &genai.GenerateContentConfig{}

	if req == nil {
		return generationConfig, nil
	}

	if req.Temperature != nil {
		generationConfig.Temperature = req.Temperature
	}
	if req.TopP != nil {
		generationConfig.TopP = req.TopP
	}
	if req.TopK != nil {
		v := float32(*req.TopK)
		generationConfig.TopK = &v
	}

	return generationConfig, nil
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

func convertTools(tools []llm.ToolDefinition) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]*genai.Tool, len(tools))
	for i := range tools {
		t := &tools[i]
		funcDecl := &genai.FunctionDeclaration{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  convertSchema(t.Function.Parameters),
		}
		result[i] = &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{funcDecl},
		}
	}
	return result
}

func convertSchema(params *llm.ToolFunctionParameters) *genai.Schema {
	return llm.TranscribeParameters(params, geminiSchemaTranscriber{})
}

// geminiSchemaTranscriber converts the internal parameter hierarchy to Gemini's
// genai.Schema types.
type geminiSchemaTranscriber struct{}

func (g geminiSchemaTranscriber) Scalar(types []string, description string, enum []string) *genai.Schema {
	return &genai.Schema{
		Type:        convertType(types),
		Description: description,
		Enum:        enum,
	}
}

func (g geminiSchemaTranscriber) Object(properties map[string]*genai.Schema, required []string) *genai.Schema {
	s := &genai.Schema{Type: genai.TypeObject}
	if len(properties) > 0 {
		s.Properties = properties
	}
	if len(required) > 0 {
		s.Required = required
	}
	return s
}

func (g geminiSchemaTranscriber) Array(items *genai.Schema) *genai.Schema {
	return &genai.Schema{Type: genai.TypeArray, Items: items}
}

// convertType maps JSON Schema type strings to GenAI schema types.
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

func convertFunctionCallFromLLM(tc llm.ToolCall) (*genai.FunctionCall, error) {
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

func convertFunctionCall(fc *genai.FunctionCall) llm.ToolCall {
	return llm.ToolCall{
		ID: fc.ID,
		Function: llm.ToolCallFunction{
			Name:      fc.Name,
			Arguments: fc.Args,
		},
	}
}

func convertToLLMResponse(resp *genai.GenerateContentResponse) (*llm.ChatResponse, error) {
	result := extractResponseFromCandidates(resp)
	result.Done = false

	done, err := handleFinishReasonIfPresent(resp)
	if err != nil {
		return nil, err
	}
	result.Done = done

	if resp.UsageMetadata != nil {
		result.Stats.PromptTokens = int(resp.UsageMetadata.PromptTokenCount)
		result.Stats.ResponseTokens = int(resp.UsageMetadata.CandidatesTokenCount)
		result.Stats.TotalTokens = int(resp.UsageMetadata.TotalTokenCount)
	}

	return result, nil
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

func extractResponseFromCandidates(resp *genai.GenerateContentResponse) *llm.ChatResponse {
	result := &llm.ChatResponse{}
	if len(resp.Candidates) == 0 ||
		resp.Candidates[0].Content == nil ||
		len(resp.Candidates[0].Content.Parts) == 0 {
		return result
	}

	for _, part := range resp.Candidates[0].Content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" {
			result.Content = part.Text
		}
		if part.FunctionCall != nil {
			result.ToolCalls = append(result.ToolCalls, convertFunctionCall(part.FunctionCall))
		}
	}
	return result
}
