package conversation

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/junk"
	"github.com/ollama/ollama/api"
)

// ToolTypeFunction represents a function-based tool.
const ToolTypeFunction = "function"

// ToolPropTypeString represents a string property type.
var ToolPropTypeString = []string{"string"}

// ToolDefinition defines the structure and instructions for a tool.
type ToolDefinition struct {
	Instructions []api.Message
	//todo: rename to `tools`
	Tool api.Tools
	// URIHandler handles MCP resource reading.
	URIHandler McpResource
}

// NewToolDefinition creates a new tool definition.
func NewToolDefinition() *ToolDefinition {
	return &ToolDefinition{}
}

// AppendInstruction adds a new system instruction to the tool definition.
func (t *ToolDefinition) AppendInstruction(message string) {
	t.Instructions = append(t.Instructions, api.Message{Role: RoleAssistant, Content: message})
}

// Tool defines the interface for an executable tool.
type Tool interface {
	DefineAPI(ctx context.Context) (definition *ToolDefinition, problem error)
	Invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error)
}

// ToolSet manages a collection of tools and provides helpers for chat integration.
// todo: revisit design so we are not exporting every field.
type ToolSet struct {
	Instructions []api.Message
	ByName       map[string]Tool // maps namespaced op name -> base Tool
	Defs         api.Tools
	Container    *junk.Container
	gateway      *McpResourceGateway
}

// NewToolSet builds a ToolSet from the parsed configuration. Nil cfg or empty
// content yields an empty ToolSet.
func NewToolSet() *ToolSet {
	ts := &ToolSet{
		ByName:    map[string]Tool{},
		gateway:   NewMCPResourceGateway(),
		Container: junk.NewContainer("Tool Container"),
	}
	return ts
}

// RegisterGatewayService registers the resource gateway service.
func (ts *ToolSet) RegisterGatewayService(ctx context.Context) error {
	if len(ts.gateway.ResourceServices) == 0 {
		return nil
	}
	return ts.RegisterTool(ctx, ts.gateway)
}

// RegisterTool registers a new tool in the tool set.
func (ts *ToolSet) RegisterTool(ctx context.Context, t Tool) error {
	definition, err := t.DefineAPI(ctx)
	if err != nil {
		return err
	}
	for i := range definition.Tool {
		d := &definition.Tool[i]
		if _, exists := ts.ByName[d.Function.Name]; !exists {
			ts.ByName[d.Function.Name] = t
			ts.Defs = append(ts.Defs, *d)
		}
	}
	if definition.URIHandler != nil {
		ts.gateway.Register(definition.URIHandler)
	}
	ts.Instructions = append(ts.Instructions, definition.Instructions...)
	return nil
}

// APITools returns the list of api.Tool definitions to send with chat requests.
func (ts *ToolSet) APITools() api.Tools { return ts.Defs }

// Shutdown stops all tools in the tool set.
func (ts *ToolSet) Shutdown(ctx context.Context) error {
	return ts.Container.Shutdown(ctx)
}

// HandleCall invokes the named Tool if available, otherwise returns an error Tool message.
func (ts *ToolSet) HandleCall(ctx context.Context, call api.ToolCall) ([]api.Message, error) {
	t, ok := ts.ByName[call.Function.Name]
	if !ok {
		// Return an error message so the model can recover gracefully
		errMsg := fmt.Sprintf("Tool not found {name: %q}", call.Function.Name)
		return []api.Message{ToolResponseMessage(call, fmt.Sprintf("{\"error\":%q}", errMsg))}, nil
	}
	msgs, err := t.Invoke(ctx, call)
	if err != nil {
		err = &junk.OperationalError{Description: fmt.Sprintf("Tool invocation %q (id: %s)", call.Function.Name, call.ID), Underlying: err}
	}
	return msgs, err
}

// ToolResponseMessage is a utility to respond to a Tool invocation with some content
func ToolResponseMessage(call api.ToolCall, content string) api.Message {
	return api.Message{
		Role:       RoleToolResult,
		ToolName:   call.Function.Name,
		ToolCallID: call.ID,
		Content:    content,
	}
}
