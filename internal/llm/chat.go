package llm

import "context"

// Role constants for messages
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCallFunction represents the function details of a tool call.
type ToolCallFunction struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

// ToolCall represents an invocation of a tool by the model.
// Structurally equivalent to api.ToolCall for mechanical migration.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Function ToolCallFunction `json:"function"`
}

// Message represents a single message in a conversation.
// Structurally equivalent to api.Message for mechanical migration.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Thinking   string     `json:"thinking,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
}

// SchemaTranscriber converts the internal parameter hierarchy into a
// provider-specific schema. Type parameter T is the native schema type for the
// target provider.
type SchemaTranscriber[T any] interface {
	// Scalar builds a primitive or enum schema.
	Scalar(types []string, description string, enum []string) T
	// Object builds an object schema from its properties.
	Object(properties map[string]T, required []string) T
	// Array builds an array schema from its item schema.
	Array(items T) T
}

// ToolProperty represents a single property in a JSON Schema object definition.
// The recursive fields (Properties, Items, Required) are present for future
// nesting support but are not populated today.
type ToolProperty struct {
	Type        []string                `json:"type,omitempty"`
	Description string                  `json:"description,omitempty"`
	Enum        []string                `json:"enum,omitempty"`
	Properties  map[string]ToolProperty `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
	Items       *ToolProperty           `json:"items,omitempty"`
}

// TranscribeToolProperty recursively walks a property and its children via the
// provided transcriber.
func TranscribeToolProperty[T any](p *ToolProperty, t SchemaTranscriber[T]) T {
	if p == nil {
		var zero T
		return zero
	}
	if len(p.Properties) > 0 {
		props := make(map[string]T, len(p.Properties))
		for name, child := range p.Properties {
			props[name] = TranscribeToolProperty(&child, t)
		}
		return t.Object(props, p.Required)
	}
	if p.Items != nil {
		return t.Array(TranscribeToolProperty(p.Items, t))
	}
	return t.Scalar(p.Type, p.Description, p.Enum)
}

// ToolFunctionParameters represents JSON Schema parameters for a tool function.
type ToolFunctionParameters struct {
	Type       string                  `json:"type,omitempty"`
	Required   []string                `json:"required,omitempty"`
	Properties map[string]ToolProperty `json:"properties,omitempty"`
}

// TranscribeParameters walks the parameter hierarchy and invokes the transcriber.
func TranscribeParameters[T any](p *ToolFunctionParameters, t SchemaTranscriber[T]) T {
	if p == nil {
		return t.Object(nil, nil)
	}
	props := make(map[string]T, len(p.Properties))
	for name, prop := range p.Properties {
		props[name] = TranscribeToolProperty(&prop, t)
	}
	return t.Object(props, p.Required)
}

// ToolFunction describes the schema of a callable tool.
type ToolFunction struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Parameters  *ToolFunctionParameters `json:"parameters"`
}

// ToolDefinition defines a tool that the model can call.
type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ChatRequest contains common fields for LLM chat requests.
type ChatRequest struct {
	Messages    []Message
	Tools       []ToolDefinition
	Model       string
	Temperature *float32
	TopK        *int
	TopP        *float32
}

// Stats contains token usage and completion statistics.
type Stats struct {
	PromptTokens   int
	ResponseTokens int
	TotalTokens    int
	DoneReason     string
}

// ChatResponse represents a streaming chunk or final response from a chat call.
type ChatResponse struct {
	Content   string
	Thinking  string
	ToolCalls []ToolCall
	Done      bool
	Stats     Stats
}

// LLM is the unified interface for chat-based language models.
// Providers implement this interface and handle conversion between
// internal llm types and their native API types.
type LLM interface {
	Chat(ctx context.Context, req *ChatRequest, onResponse func(ctx context.Context, resp *ChatResponse) error) error
}
