package slacker

import (
	"context"
	"testing"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/query"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"
)

// TestQueryStreamer_FixedBehavior verifies that tool calls now properly complete
// with LLM consuming tool results and providing a final response
func TestQueryStreamer_FixedBehavior(t *testing.T) {
	env := NewTestEnvironment(t)
	qs := env.QueryStreamer
	mockLLM := env.MockLLM
	sessionManager := env.SessionManager

	// Configure the mock LLM to call a tool first, then consume results
	mockLLM.responses = [][]api.ChatResponse{
		{
			// First response: LLM calls a tool
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "I'll help you with that. Let me call a tool.",
					ToolCalls: []api.ToolCall{
						{
							ID: "call_1",
							Function: api.ToolCallFunction{
								Name: "test_tool",
							},
						},
					},
				},
			},
			{Done: true},
		},
		{
			// Second response: LLM consumes tool results and provides final answer
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "Based on the tool results, here's your final answer.",
				},
			},
			{Done: true},
		},
	}

	// Create a basic ToolSet
	toolSet := conversation.NewToolSet()

	// Create test context and session
	slackCtx := &SlackContext{UserID: "U123", ChannelID: "C456"}
	updater := env.Updater
	userCtx := &query.UserContext{UserID: "U123"}
	userSession := sessionManager.GetOrCreateSession("U123", "C456", userCtx)

	// Process the query
	ctx := context.Background()
	err := qs.ProcessQueryWithUpdater(ctx, slackCtx, userSession, "Please use a tool to help me", toolSet, updater)
	require.NoError(t, err)

	// Verify the fix: LLM is now called twice for complete conversation
	require.Len(t, mockLLM.calls, 2, "Fixed behavior: LLM should be called twice for complete conversation")

	// Verify the first call contained the user message
	firstCall := mockLLM.calls[0]
	require.Len(t, firstCall.Messages, 2) // system + user
	require.Equal(t, "system", firstCall.Messages[0].Role)
	require.Equal(t, "user", firstCall.Messages[1].Role)
	require.Equal(t, "Please use a tool to help me", firstCall.Messages[1].Content)

	// Verify the second call included the assistant message with tool calls
	secondCall := mockLLM.calls[1]
	require.GreaterOrEqual(t, len(secondCall.Messages), 3) // system + user + assistant + tool responses

	// Verify session shows complete conversation
	finalSession, exists := sessionManager.GetSession("U123", "C456")
	require.True(t, exists)
	require.GreaterOrEqual(t, len(finalSession.Messages), 2, "Should have assistant messages")
}

// TestQueryStreamer_DesiredBehavior shows what the behavior should be after fixing
// This test currently fails, demonstrating what needs to be implemented
func TestQueryStreamer_DesiredBehavior(t *testing.T) {
	env := NewTestEnvironment(t)
	qs := env.QueryStreamer
	mockLLM := env.MockLLM
	sessionManager := env.SessionManager

	// Configure the mock LLM to call a tool first, then consume results and respond
	mockLLM.responses = [][]api.ChatResponse{
		{
			// First response: LLM calls a tool
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "I'll help you with that. Let me call a tool.",
					ToolCalls: []api.ToolCall{
						{
							ID: "call_1",
							Function: api.ToolCallFunction{
								Name: "test_tool",
							},
						},
					},
				},
			},
			{Done: true},
		},
		{
			// Second response: LLM consumes tool results and provides final answer
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "Based on the tool results, here's your final answer.",
				},
			},
			{Done: true},
		},
	}

	// Create a basic ToolSet that returns a response
	toolSet := conversation.NewToolSet()

	// Create test context and session
	slackCtx := &SlackContext{UserID: "U123", ChannelID: "C456"}
	updater := env.Updater
	userCtx := &query.UserContext{UserID: "U123"}
	userSession := sessionManager.GetOrCreateSession("U123", "C456", userCtx)

	// Process the query
	ctx := context.Background()
	err := qs.ProcessQueryWithUpdater(ctx, slackCtx, userSession, "Please use a tool to help me", toolSet, updater)
	require.NoError(t, err)

	// This should be the desired behavior after fixing:
	// LLM should be called twice - once for initial, once for final response
	require.Len(t, mockLLM.calls, 2, "LLM should be called twice for complete conversation")

	// Verify first call had the user message
	firstCall := mockLLM.calls[0]
	require.Len(t, firstCall.Messages, 2) // system + user
	require.Equal(t, "Please use a tool to help me", firstCall.Messages[1].Content)

	// Verify second call included tool results and the previous assistant message
	secondCall := mockLLM.calls[1]
	require.GreaterOrEqual(t, len(secondCall.Messages), 3) // system + user + assistant (with tool calls) + tool responses

	// Note: Tool responses should be included in the second LLM call messages

	// Verify session contains complete conversation with final assistant message
	finalSession, exists := sessionManager.GetSession("U123", "C456")
	require.True(t, exists)

	// Should have: user message, assistant with tool calls, final assistant message
	require.GreaterOrEqual(t, len(finalSession.Messages), 2, "Session should contain at least user + assistant messages")

	// Verify there's a final assistant message without tool calls
	hasFinalAssistant := false
	for _, msg := range finalSession.Messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) == 0 && msg.Content != "" {
			hasFinalAssistant = true
			break
		}
	}
	require.True(t, hasFinalAssistant, "Should have final assistant message without tool calls")
}

// TestQueryStreamer_MultiTurnConversation tests that the conversation loop properly handles
// multiple rounds of tool calls until the LLM is truly finished
func TestQueryStreamer_MultiTurnConversation(t *testing.T) {
	env := NewTestEnvironment(t)
	qs := env.QueryStreamer
	mockLLM := env.MockLLM
	sessionManager := env.SessionManager

	// Configure the mock LLM for a 3-turn conversation:
	// 1. Call tool A
	// 2. Call tool B after seeing result from A
	// 3. Provide final response after seeing result from B
	mockLLM.responses = [][]api.ChatResponse{
		{
			// First response: LLM calls first tool
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "I'll help by calling tool A.",
					ToolCalls: []api.ToolCall{
						{
							ID:       "call_1",
							Function: api.ToolCallFunction{Name: "tool_a"},
						},
					},
				},
			},
			{Done: true},
		},
		{
			// Second response: LLM calls second tool after seeing first result
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "Now I'll call tool B based on that result.",
					ToolCalls: []api.ToolCall{
						{
							ID:       "call_2",
							Function: api.ToolCallFunction{Name: "tool_b"},
						},
					},
				},
			},
			{Done: true},
		},
		{
			// Third response: LLM provides final answer after both tools
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "Based on both tool results, here's your comprehensive answer.",
				},
			},
			{Done: true},
		},
	}

	// Create a basic ToolSet that returns different responses for different tools
	toolSet := conversation.NewToolSet()

	// Create test context and session
	slackCtx := &SlackContext{UserID: "U123", ChannelID: "C456"}
	updater := env.Updater
	userCtx := &query.UserContext{UserID: "U123"}
	userSession := sessionManager.GetOrCreateSession("U123", "C456", userCtx)

	// Process the query
	ctx := context.Background()
	err := qs.ProcessQueryWithUpdater(ctx, slackCtx, userSession, "Please use multiple tools to help me", toolSet, updater)
	require.NoError(t, err)

	// Verify the LLM was called 3 times for the complete conversation
	require.Len(t, mockLLM.calls, 3, "LLM should be called 3 times for multi-turn conversation")

	// Verify the final session shows the complete conversation flow
	finalSession, exists := sessionManager.GetSession("U123", "C456")
	require.True(t, exists)

	// Should have at least 3 assistant messages in the conversation
	assistantCount := 0
	for _, msg := range finalSession.Messages {
		if msg.Role == "assistant" {
			assistantCount++
		}
	}
	require.GreaterOrEqual(t, assistantCount, 3, "Should have multiple assistant messages for multi-turn conversation")

	// Verify the final message has no tool calls (indicating completion)
	var finalMsg *api.Message
	for i := len(finalSession.Messages) - 1; i >= 0; i-- {
		if finalSession.Messages[i].Role == "assistant" {
			finalMsg = &finalSession.Messages[i]
			break
		}
	}
	require.NotNil(t, finalMsg, "Should have a final assistant message")
	require.Empty(t, finalMsg.ToolCalls, "Final message should have no tool calls")
	require.Contains(t, finalMsg.Content, "comprehensive answer", "Final message should provide the answer")
}

// TestQueryStreamer_NoToolCalls verifies that conversations without tools work normally
func TestQueryStreamer_NoToolCalls(t *testing.T) {
	env := NewTestEnvironment(t)
	qs := env.QueryStreamer
	mockLLM := env.MockLLM
	sessionManager := env.SessionManager

	// Configure the mock LLM for a simple 1-turn conversation without tools
	mockLLM.responses = [][]api.ChatResponse{
		{
			// Single response with no tool calls
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "Here's your direct answer without needing any tools.",
				},
			},
			{Done: true},
		},
	}

	// Create a basic ToolSet
	toolSet := conversation.NewToolSet()

	// Create test context and session
	slackCtx := &SlackContext{UserID: "U123", ChannelID: "C456"}
	updater := env.Updater
	userCtx := &query.UserContext{UserID: "U123"}
	userSession := sessionManager.GetOrCreateSession("U123", "C456", userCtx)

	// Process the query
	ctx := context.Background()
	err := qs.ProcessQueryWithUpdater(ctx, slackCtx, userSession, "Give me a simple answer", toolSet, updater)
	require.NoError(t, err)

	// Verify the LLM was called exactly once (no follow-up calls needed)
	require.Len(t, mockLLM.calls, 1, "LLM should be called only once for conversations without tools")

	// Verify the session contains the expected conversation
	finalSession, exists := sessionManager.GetSession("U123", "C456")
	require.True(t, exists)

	// Should have exactly 1 assistant message in the conversation
	assistantCount := 0
	for _, msg := range finalSession.Messages {
		if msg.Role == "assistant" {
			assistantCount++
		}
	}
	require.Equal(t, 1, assistantCount, "Should have exactly 1 assistant message")

	// Verify the message has no tool calls
	var assistantMsg *api.Message
	for _, msg := range finalSession.Messages {
		if msg.Role == "assistant" {
			assistantMsg = &msg
			break
		}
	}
	require.NotNil(t, assistantMsg, "Should have an assistant message")
	require.Empty(t, assistantMsg.ToolCalls, "Message should have no tool calls")
	require.Contains(t, assistantMsg.Content, "direct answer", "Message should contain the expected content")
}

// TestQueryStreamer_LLMWaitingForMoreTools tests the specific scenario mentioned:
// LLM takes a turn but is still waiting for more tool calls, requiring continued looping
func TestQueryStreamer_LLMWaitingForMoreTools(t *testing.T) {
	env := NewTestEnvironment(t)
	qs := env.QueryStreamer
	mockLLM := env.MockLLM
	sessionManager := env.SessionManager

	// Configure the mock LLM to show that it's still thinking and needs more tools
	// This tests that the loop properly continues until LLM is truly done
	mockLLM.responses = [][]api.ChatResponse{
		{
			// First response: LLM calls first tool but indicates it needs more
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "Let me start by gathering some information.",
					ToolCalls: []api.ToolCall{
						{
							ID:       "call_1",
							Function: api.ToolCallFunction{Name: "info_tool"},
						},
					},
				},
			},
			{Done: true},
		},
		{
			// Second response: LLM calls another tool, still not done
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "Now I need to process this information further.",
					ToolCalls: []api.ToolCall{
						{
							ID:       "call_2",
							Function: api.ToolCallFunction{Name: "process_tool"},
						},
					},
				},
			},
			{Done: true},
		},
		{
			// Third response: LLM finally provides the complete answer
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "After processing all the information, here's your complete answer.",
				},
			},
			{Done: true},
		},
	}

	// Create a basic ToolSet
	toolSet := conversation.NewToolSet()

	// Create test context and session
	slackCtx := &SlackContext{UserID: "U123", ChannelID: "C456"}
	updater := env.Updater
	userCtx := &query.UserContext{UserID: "U123"}
	userSession := sessionManager.GetOrCreateSession("U123", "C456", userCtx)

	// Process the query
	ctx := context.Background()
	err := qs.ProcessQueryWithUpdater(ctx, slackCtx, userSession, "Help me with a complex task", toolSet, updater)
	require.NoError(t, err)

	// Verify the LLM was called 3 times (each time it had more tool calls)
	require.Len(t, mockLLM.calls, 3, "LLM should be called 3 times until completion")

	// Verify the final session shows the progressive conversation
	finalSession, exists := sessionManager.GetSession("U123", "C456")
	require.True(t, exists)

	// Count assistant messages to verify proper progression
	assistantMessages := []api.Message{}
	for _, msg := range finalSession.Messages {
		if msg.Role == "assistant" {
			assistantMessages = append(assistantMessages, msg)
		}
	}
	require.Len(t, assistantMessages, 3, "Should have 3 assistant messages showing progression")

	// Verify first message has tool calls (first tool)
	require.NotEmpty(t, assistantMessages[0].ToolCalls, "First message should have tool calls")
	require.Equal(t, "info_tool", assistantMessages[0].ToolCalls[0].Function.Name)

	// Verify second message has tool calls (second tool)
	require.NotEmpty(t, assistantMessages[1].ToolCalls, "Second message should have tool calls")
	require.Equal(t, "process_tool", assistantMessages[1].ToolCalls[0].Function.Name)

	// Verify final message has no tool calls (completion)
	require.Empty(t, assistantMessages[2].ToolCalls, "Final message should have no tool calls")
	require.Contains(t, assistantMessages[2].Content, "complete answer", "Final message should indicate completion")
}
