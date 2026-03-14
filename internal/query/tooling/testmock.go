package tooling

import (
	"context"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
)

type testMockTool struct {
	name        string
	description string
	toolCount   int
}

func (m *testMockTool) DefineAPI(_ context.Context) (*conversation.ToolDefinition, error) {
	def := &conversation.ToolDefinition{}
	for i := 0; i < m.toolCount; i++ {
		toolName := m.name
		if m.toolCount > 1 {
			toolName = m.name + "_" + string(rune('a'+i))
		}
		def.Tool = append(def.Tool, api.Tool{
			Type: conversation.ToolTypeFunction,
			Function: api.ToolFunction{
				Name:        toolName,
				Description: m.description,
			},
		})
	}
	return def, nil
}

func (m *testMockTool) Invoke(_ context.Context, _ api.ToolCall) ([]api.Message, error) {
	return nil, nil
}
