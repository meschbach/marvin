package query

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ollama/ollama/api"
)

type ollamaConversation struct {
	client         *api.Client
	messages       []api.Message
	tools          *ToolSet
	showThinking   bool
	showDone       bool
	showTools      bool
	responseTokens int
	promptTokens   int
}

// runAIToConclusion executes the AI chat loop with tool-call handling until
// the assistant produces a final answer (no further tool calls) or an error occurs.
func (o *ollamaConversation) runAIToConclusion(ctx context.Context, model string, availableTools api.Tools) error {
	for {
		req := &api.ChatRequest{
			Model:    model,
			Messages: o.messages,
			Tools:    availableTools,
		}

		// Accumulate the assistant response and capture any tool calls
		var assistantOut, thinkingBuffer strings.Builder
		var thisLine strings.Builder
		var thisThinking strings.Builder
		var pendingCalls []api.ToolCall

		err := o.client.Chat(ctx, req, func(resp api.ChatResponse) error {
			if resp.Done {
				if o.showDone {
					fmt.Printf("<Done> (%d) %s\n", resp.EvalCount, resp.DoneReason)
				}
				o.responseTokens = o.responseTokens + resp.EvalCount
				o.promptTokens = o.promptTokens + resp.PromptEvalCount
			}
			if s := resp.Message.Content; s != "" {
				thisLine.WriteString(s)
				if strings.Contains(s, "\n") {
					fmt.Print(thisLine.String())
					thisLine.Reset()
				}

				assistantOut.WriteString(s)
			}
			if len(resp.Message.Thinking) > 0 {
				if o.showThinking {
					thisThinking.WriteString(resp.Message.Thinking)
					if strings.Contains(thisThinking.String(), "\n") {
						fmt.Printf("Thinking: %s", thisThinking.String())
						thisThinking.Reset()
					}
				}
				thinkingBuffer.WriteString(resp.Message.Thinking)
			}

			if len(resp.Message.ToolCalls) > 0 {
				if o.showTools {
					fmt.Printf("tool call {%s} > %s\n\t%#v\n", resp.Message.ToolCalls[0].ID, resp.Message.ToolCalls[0].Function.Name, resp.Message.ToolCalls[0].Function.Arguments)
				}
				// Capture tool calls signaled by the model
				pendingCalls = append(pendingCalls, resp.Message.ToolCalls...)
			}
			return nil
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError querying Ollama: %v\nAssitant buffer:%q\nPending calls: %#v\nTools:\n", err, assistantOut.String(), pendingCalls)
			for _, tool := range availableTools {
				fmt.Fprintf(os.Stderr, "\t%s: %s\n", tool.Function.Name, tool.Function.Description)
			}
			return err
		}

		// Record the assistant turn (including tool calls, if any)
		assistantMsg := api.Message{
			Role:      roleAssistant,
			Content:   assistantOut.String(),
			ToolCalls: pendingCalls,
			Thinking:  thinkingBuffer.String(),
		}
		o.messages = append(o.messages, assistantMsg)

		if o.showTools {
			fmt.Println()
			fmt.Printf("tools > %d pending invocations\n", len(pendingCalls))
		}
		// If there are no tool calls, we are done for this turn
		if len(pendingCalls) == 0 {
			fmt.Println()
			fmt.Printf("Total tokens: %d = (prompt tokens: %d) + (response tokens: %d)\n", o.responseTokens+o.promptTokens, o.promptTokens, o.responseTokens)
			return nil
		}

		var pendingCallsErrors error
		// For each tool call, invoke via the toolset and append tool results
		for _, call := range pendingCalls {
			if o.showTools {
				fmt.Printf("call %s> Function %s with argument %#v\n", call.ID, call.Function.Name, call.Function.Arguments)
			}
			reply, herr := o.tools.HandleCall(ctx, call)
			pendingCallsErrors = errors.Join(herr, pendingCallsErrors)
			if o.showTools {
				for _, reply := range reply {
					fmt.Printf("call %s>\t%s\t%s: %s\n", reply.ToolCallID, reply.Role, reply.ToolName, reply.Content)
				}
				if len(reply) == 0 {
					fmt.Printf("call %s> no response\n", call.ID)
				}
			}
			o.messages = append(o.messages, reply...)
		}
		if pendingCallsErrors != nil {
			fmt.Printf("\nError invoking tools: %v\n", pendingCallsErrors)
			return pendingCallsErrors
		}

		// Loop continues: the next iteration sends messages including tool outputs
	}
}
