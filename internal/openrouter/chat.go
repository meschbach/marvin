package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/ollama/ollama/api"
)

func (o *LLM) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	openRouterReq, err := o.buildRequest(req)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(openRouterReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/meschbach/marvin")
	httpReq.Header.Set("X-Title", "Marvin")

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "Error closing response body: %v\n", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			return fmt.Errorf("OpenRouter API error: %s (failed to read body: %v)", resp.Status, rerr)
		}
		return fmt.Errorf("OpenRouter API error: %s %s", resp.Status, body)
	}

	return o.processStream(resp.Body, fn)
}
