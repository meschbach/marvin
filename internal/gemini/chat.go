package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"

	"google.golang.org/genai"
)

func (g *LLM) chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
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
		if err := fn(*ollamaResp); err != nil {
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

	//nolint:gocritic
	for i, msg := range messages {
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

//nolint:gocritic
func convertSingleMessage(msg api.Message, index int) (*genai.Content, error) {
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

//nolint:gocritic
func convertAssistantMessage(msg api.Message, index int) (*genai.Content, error) {
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

//nolint:gocritic
func convertUserMessage(msg api.Message, role string, index int) (*genai.Content, error) {
	if msg.Content == "" {
		fmt.Printf("warn\tSkipping empty %s message @ %d\n", role, index)
		return nil, nil
	}
	return genai.NewContentFromText(msg.Content, genai.Role(role)), nil
}

//nolint:gocritic
func convertToolResult(msg api.Message) *genai.Content {
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
	convertIntToFloatOption(opts, "top_k", func(v float32) {
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

func convertIntToFloatOption(opts map[string]any, key string, setter func(float32)) {
	if val, ok := opts[key].(int); ok {
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

//nolint:gocyclo
func handleFinishReason(reason genai.FinishReason) error {
	switch reason {
	case genai.FinishReasonStop:
		return nil
	case genai.FinishReasonMaxTokens:
		return nil
	case genai.FinishReasonUnexpectedToolCall:
		return errors.New("model attempted to call a tool but none were provided")
	case genai.FinishReasonSafety:
		return errors.New("response blocked due to safety policy")
	case genai.FinishReasonRecitation:
		return errors.New("response blocked due to recitation policy")
	case genai.FinishReasonOther:
		return errors.New("response stopped for unknown reason")
	case genai.FinishReasonUnspecified:
		return errors.New("response finish reason unspecified")
	case genai.FinishReasonLanguage:
		return errors.New("response blocked due to language policy")
	case genai.FinishReasonBlocklist:
		return errors.New("response blocked due to blocklist policy")
	case genai.FinishReasonProhibitedContent:
		return errors.New("response blocked due to prohibited content")
	case genai.FinishReasonSPII:
		return errors.New("response blocked due to sensitive personal information")
	case genai.FinishReasonMalformedFunctionCall:
		return errors.New("response has malformed function call")
	case genai.FinishReasonImageSafety:
		return errors.New("response blocked due to image safety policy")
	case genai.FinishReasonImageProhibitedContent:
		return errors.New("response blocked due to prohibited image content")
	case genai.FinishReasonNoImage:
		return errors.New("response blocked: no image provided")
	case genai.FinishReasonImageRecitation:
		return errors.New("response blocked due to image recitation policy")
	case genai.FinishReasonImageOther:
		return errors.New("response blocked due to image policy")
	}
	return nil
}

func convertTools(tools api.Tools) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]*genai.Tool, len(tools))
	//nolint:gocritic
	for i, t := range tools {
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

//nolint:gocritic
func convertSchema(params api.ToolFunctionParameters) *genai.Schema {
	schema := &genai.Schema{
		Type: genai.TypeObject,
	}

	if params.Properties != nil {
		properties := make(map[string]*genai.Schema)
		for k, v := range params.Properties.ToMap() {
			properties[k] = convertToolProperty(v)
		}
		schema.Properties = properties
	}

	if len(params.Required) > 0 {
		schema.Required = params.Required
	}

	return schema
}

//nolint:gocritic,gocyclo
func convertToolProperty(prop api.ToolProperty) *genai.Schema {
	schema := &genai.Schema{}

	for _, t := range prop.Type {
		switch t {
		case "string":
			schema.Type = genai.TypeString
		case "integer":
			schema.Type = genai.TypeInteger
		case "number":
			schema.Type = genai.TypeNumber
		case "boolean":
			schema.Type = genai.TypeBoolean
		case "array":
			schema.Type = genai.TypeArray
		case "object":
			schema.Type = genai.TypeObject
		}
	}

	schema.Description = prop.Description

	if len(prop.Enum) > 0 {
		enumStrs := make([]string, len(prop.Enum))
		for i, e := range prop.Enum {
			if s, ok := e.(string); ok {
				enumStrs[i] = s
			}
		}
		schema.Enum = enumStrs
	}

	return schema
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
			_ = args.UnmarshalJSON(argsJSON) //nolint:errcheck
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

//nolint:gocyclo
func convertToOllamaResponse(resp *genai.GenerateContentResponse) (*api.ChatResponse, error) {
	ollamaResp := &api.ChatResponse{
		Model:     resp.ModelVersion,
		CreatedAt: time.Now(),
		Done:      false,
	}

	if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != "" {
		if resp.Candidates[0].FinishReason != genai.FinishReasonStop {
			finishErr := handleFinishReason(resp.Candidates[0].FinishReason)
			if finishErr != nil {
				return nil, finishErr
			}
		}
		ollamaResp.Done = true
	}

	if len(resp.Candidates) > 0 &&
		resp.Candidates[0].Content != nil &&
		len(resp.Candidates[0].Content.Parts) > 0 {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part == nil {
				continue
			}
			if part.Text != "" {
				ollamaResp.Message = api.Message{
					Role:    "assistant",
					Content: part.Text,
				}
			}
			if part.FunctionCall != nil {
				toolCall := convertFunctionCall(part.FunctionCall)
				ollamaResp.Message.ToolCalls = append(ollamaResp.Message.ToolCalls, toolCall)
			}
		}
	}

	if resp.UsageMetadata != nil {
		ollamaResp.PromptEvalCount = int(resp.UsageMetadata.PromptTokenCount)
		ollamaResp.EvalCount = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return ollamaResp, nil
}
