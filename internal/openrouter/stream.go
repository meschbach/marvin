package openrouter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ollama/ollama/api"
)

type usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (o *LLM) processStream(body io.Reader, fn api.ChatResponseFunc) error {
	reader := bufio.NewReader(body)
	finalUsage := usage{}

	for {
		line, err := o.readLine(reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		data, err := o.extractData(line)
		if err != nil {
			continue
		}

		streamResp, err := o.parseChunk(data)
		if err != nil {
			continue
		}

		if err := o.processChunk(streamResp, &finalUsage, fn); err != nil {
			return err
		}

		if o.shouldStop(streamResp, finalUsage) {
			break
		}
	}

	return nil
}

func (o *LLM) readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return "", io.EOF
		}
		return "", fmt.Errorf("error reading stream: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func (o *LLM) extractData(line string) (string, error) {
	if line == "" || line == "data: [DONE]" {
		return "", fmt.Errorf("skip")
	}

	if !strings.HasPrefix(line, "data: ") {
		return "", fmt.Errorf("skip")
	}

	return strings.TrimPrefix(line, "data: "), nil
}

func (o *LLM) parseChunk(data string) (StreamResponse, error) {
	var streamResp StreamResponse
	if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
		return streamResp, fmt.Errorf("skip")
	}
	return streamResp, nil
}

func (o *LLM) processChunk(streamResp StreamResponse, finalUsage *usage, fn api.ChatResponseFunc) error {
	o.updateUsage(streamResp, finalUsage)

	if o.isUsageOnlyChunk(streamResp) {
		return o.sendUsageOnlyChunk(streamResp, finalUsage, fn)
	}

	return o.sendContentChunk(streamResp, finalUsage, fn)
}

func (o *LLM) updateUsage(streamResp StreamResponse, finalUsage *usage) {
	if streamResp.Usage.PromptTokens > 0 || streamResp.Usage.CompletionTokens > 0 {
		*finalUsage = usage(streamResp.Usage)
	}
}

func (o *LLM) isUsageOnlyChunk(streamResp StreamResponse) bool {
	return len(streamResp.Choices) == 0 ||
		(len(streamResp.Choices) == 1 &&
			streamResp.Choices[0].Delta.Content == "" &&
			streamResp.Choices[0].FinishReason == "")
}

func (o *LLM) sendUsageOnlyChunk(streamResp StreamResponse, finalUsage *usage, fn api.ChatResponseFunc) error {
	if streamResp.Usage.PromptTokens > 0 || streamResp.Usage.CompletionTokens > 0 {
		*finalUsage = usage(streamResp.Usage)
		apiResp := api.ChatResponse{
			Model: streamResp.Model,
			Done:  true,
			Metrics: api.Metrics{
				EvalCount:       finalUsage.CompletionTokens,
				PromptEvalCount: finalUsage.PromptTokens,
			},
		}
		return fn(apiResp)
	}
	return nil
}

func (o *LLM) sendContentChunk(streamResp StreamResponse, finalUsage *usage, fn api.ChatResponseFunc) error {
	choice := streamResp.Choices[0]

	if streamResp.Usage.CompletionTokens > 0 || streamResp.Usage.PromptTokens > 0 {
		*finalUsage = usage(streamResp.Usage)
	}

	isDone := o.isDone(choice.Index, choice.Delta.Content, choice.FinishReason, *finalUsage)

	apiResp := api.ChatResponse{
		Model: streamResp.Model,
		Message: api.Message{
			Role:    choice.Delta.Role,
			Content: choice.Delta.Content,
		},
		Done: isDone,
		Metrics: api.Metrics{
			EvalCount:       finalUsage.CompletionTokens,
			PromptEvalCount: finalUsage.PromptTokens,
		},
	}

	if choice.Delta.ToolCalls != nil {
		apiResp.Message.ToolCalls = choice.Delta.ToolCalls
	}

	return fn(apiResp)
}

func (o *LLM) isDone(choiceIndex int, choiceDelta string, choiceFinishReason string, finalUsage usage) bool {
	return choiceFinishReason != "" && (finalUsage.CompletionTokens > 0 || finalUsage.PromptTokens > 0)
}

func (o *LLM) shouldStop(streamResp StreamResponse, finalUsage usage) bool {
	if len(streamResp.Choices) == 0 {
		return false
	}

	if streamResp.Choices[0].FinishReason == "" {
		return false
	}

	return finalUsage.CompletionTokens > 0 || finalUsage.PromptTokens > 0
}
