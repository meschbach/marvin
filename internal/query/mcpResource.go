package query

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
	"github.com/yosida95/uritemplate/v3"
)

type mcpResource interface {
	matches() []*uritemplate.Template
	describeMessages() []api.Message
	readResource(ctx context.Context, invocation api.ToolCall, uri string) ([]api.Message, error)
}

// mcpResourceGateway manages all the MCP integrations in regard to reading various resources.
type mcpResourceGateway struct {
	resourceServices []mcpResource
}

func newMCPResourceGateway() *mcpResourceGateway {
	return &mcpResourceGateway{}
}

func (m *mcpResourceGateway) register(gateway mcpResource) {
	m.resourceServices = append(m.resourceServices, gateway)
}

func (m *mcpResourceGateway) defineAPI(ctx context.Context) (definition *toolDefinition, problem error) {
	fmt.Printf("gateway\t\t> defining API with %d services\n", len(m.resourceServices))
	definition = &toolDefinition{}
	definition.tool = append(definition.tool, api.Tool{
		Type: ToolTypeFunction,
		Function: api.ToolFunction{
			Name:        "read_resource",
			Description: "read_resource is a gateway to other tools resources identified by a URI.  Pass the full URI as the `uri` parameter",
			Parameters: api.ToolFunctionParameters{
				Type:     "resource_resource",
				Required: []string{"uri"},
				Properties: map[string]api.ToolProperty{
					"uri": {
						Type:        ToolPropTypeString,
						Description: "URI of the resource to read",
					},
				},
			},
		},
	})
	definition.instructions = append(definition.instructions, api.Message{
		Role:    roleSystem,
		Content: "Use the tool read_resource to access resources identified by a URI.",
	})

	fmt.Printf("gateway\t\t> defining API with %d services\n", len(m.resourceServices))
	for _, rs := range m.resourceServices {
		msg := rs.describeMessages()
		fmt.Printf("gateway\t\t> adding %d instructions\n", len(msg))
		definition.instructions = append(definition.instructions, msg...)
	}
	fmt.Printf("gateway\t\t> done defining API with %d instructions\n\n", len(definition.instructions))
	return definition, nil
}

func (m *mcpResourceGateway) invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	args := call.Function.Arguments
	uriUnknownType, hasURI := args["uri"]
	if !hasURI {
		return []api.Message{toolResponseMessage(call, "required parameter uri is missing")}, nil
	}
	uri, stringURI := uriUnknownType.(string)
	if !stringURI {
		return []api.Message{toolResponseMessage(call, "required parameter uri can not be cast to a string")}, nil
	}

	for _, rs := range m.resourceServices {
		if matches := rs.matches(); len(matches) > 0 {
			return rs.readResource(ctx, call, uri)
		}
	}
	return []api.Message{toolResponseMessage(call, "no resource service found for uri")}, nil
}
