package conversation

import (
	"context"

	"github.com/ollama/ollama/api"
	"github.com/yosida95/uritemplate/v3"
)

type McpResource interface {
	Matches() []*uritemplate.Template
	DescribeMessages() []api.Message
	ReadResource(ctx context.Context, invocation api.ToolCall, uri string) ([]api.Message, error)
}

// McpResourceGateway manages all the MCP integrations in regard to reading various resources.
type McpResourceGateway struct {
	ResourceServices []McpResource
}

func NewMCPResourceGateway() *McpResourceGateway {
	return &McpResourceGateway{}
}

func (m *McpResourceGateway) Register(gateway McpResource) {
	m.ResourceServices = append(m.ResourceServices, gateway)
}

func (m *McpResourceGateway) DefineAPI(ctx context.Context) (definition *ToolDefinition, problem error) {
	props := api.NewToolPropertiesMap()
	props.Set("uri", api.ToolProperty{
		Type:        ToolPropTypeString,
		Description: "URI of the resource to read",
	})
	definition = &ToolDefinition{}
	definition.Tool = append(definition.Tool, api.Tool{
		Type: ToolTypeFunction,
		Function: api.ToolFunction{
			Name:        "read_resource",
			Description: "read_resource is a gateway to other tools resources identified by a URI.  Pass the full URI as the `uri` parameter",
			Parameters: api.ToolFunctionParameters{
				Type:       "resource_resource",
				Required:   []string{"uri"},
				Properties: props,
			},
		},
	})
	definition.Instructions = append(definition.Instructions, api.Message{
		Role:    RoleSystem,
		Content: "Use the Tool read_resource to access resources identified by a URI.",
	})

	for _, rs := range m.ResourceServices {
		msg := rs.DescribeMessages()
		definition.Instructions = append(definition.Instructions, msg...)
	}
	return definition, nil
}

func (m *McpResourceGateway) Invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	args := call.Function.Arguments
	uriUnknownType, hasURI := args.Get("uri")
	if !hasURI {
		return []api.Message{ToolResponseMessage(call, "required parameter uri is missing")}, nil
	}
	uri, stringURI := uriUnknownType.(string)
	if !stringURI {
		return []api.Message{ToolResponseMessage(call, "required parameter uri can not be cast to a string")}, nil
	}

	for _, rs := range m.ResourceServices {
		if matches := rs.Matches(); len(matches) > 0 {
			return rs.ReadResource(ctx, call, uri)
		}
	}
	return []api.Message{ToolResponseMessage(call, "no resource service found for uri")}, nil
}
