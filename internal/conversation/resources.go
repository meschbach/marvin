package conversation

import (
	"context"

	"github.com/meschbach/marvin/internal/llm"
	"github.com/yosida95/uritemplate/v3"
)

// McpResource defines an interface for an MCP resource.
type McpResource interface {
	Matches() []*uritemplate.Template
	DescribeMessages() []llm.Message
	ReadResource(ctx context.Context, invocation llm.ToolCall, uri string) ([]llm.Message, error)
}

// McpResourceGateway manages all the MCP integrations in regard to reading various resources.
type McpResourceGateway struct {
	ResourceServices []McpResource
}

// NewMCPResourceGateway creates a new MCP resource gateway.
func NewMCPResourceGateway() *McpResourceGateway {
	return &McpResourceGateway{}
}

// Register adds a new resource gateway to the gateway.
func (m *McpResourceGateway) Register(gateway McpResource) {
	m.ResourceServices = append(m.ResourceServices, gateway)
}

// DefineAPI defines the API for the MCP resource gateway.
func (m *McpResourceGateway) DefineAPI(_ context.Context) (definition *ToolDefinition, problem error) {
	definition = &ToolDefinition{}
	definition.Tool = append(definition.Tool, llm.ToolDefinition{
		Type: ToolTypeFunction,
		Function: llm.ToolFunction{
			Name:        "read_resource",
			Description: "read_resource is a gateway to other tools resources identified by a URI.  Pass the full URI as the `uri` parameter",
			Parameters: &llm.ToolFunctionParameters{
				Type:     "object",
				Required: []string{"uri"},
				Properties: map[string]llm.ToolProperty{
					"uri": {
						Type:        []string{"string"},
						Description: "URI of the resource to read",
					},
				},
			},
		},
	})
	definition.Instructions = append(definition.Instructions, llm.Message{
		Role:    RoleSystem,
		Content: "Use the Tool read_resource to access resources identified by a URI.",
	})

	for _, rs := range m.ResourceServices {
		msg := rs.DescribeMessages()
		definition.Instructions = append(definition.Instructions, msg...)
	}
	return definition, nil
}

// Invoke executes a tool call through the gateway.
func (m *McpResourceGateway) Invoke(ctx context.Context, call llm.ToolCall) (out []llm.Message, problem error) {
	args, ok := call.Function.Arguments.(map[string]any)
	if !ok {
		return []llm.Message{ToolResponseMessage(call, "required parameter uri is missing")}, nil
	}
	uriUnknownType, hasURI := args["uri"]
	if !hasURI {
		return []llm.Message{ToolResponseMessage(call, "required parameter uri is missing")}, nil
	}
	uri, stringURI := uriUnknownType.(string)
	if !stringURI {
		return []llm.Message{ToolResponseMessage(call, "required parameter uri can not be cast to a string")}, nil
	}

	for _, rs := range m.ResourceServices {
		if matches := rs.Matches(); len(matches) > 0 {
			return rs.ReadResource(ctx, call, uri)
		}
	}
	return []llm.Message{ToolResponseMessage(call, "no resource service found for uri")}, nil
}
