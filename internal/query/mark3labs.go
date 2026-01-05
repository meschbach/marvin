package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ollama/ollama/api"
	"github.com/yosida95/uritemplate/v3"
)

// programRuntimeSpec is a configured program for a runtime
type programRuntimeSpec interface {
	start(ctx context.Context) (runningProgram, error)
}

type runningProgram interface {
	//todo: decouple from mark3labs/go-sdk
	transport() transport.Interface
	stop(ctx context.Context) error
}

type Mark3labsTool struct {
	Name                 string
	spec                 programRuntimeSpec
	active               runningProgram
	mcpClient            *client.Client
	resourceInstructions []api.Message
	resourceTemplates    []*uritemplate.Template
}

func (m *Mark3labsTool) ensureRunning(ctx context.Context) (problem error) {
	if m.active != nil {
		return nil
	}
	m.active, problem = m.spec.start(ctx)
	m.mcpClient = client.NewClient(m.active.transport())
	if err := m.mcpClient.Start(ctx); err != nil {
		problem = errors.Join(problem, &operationalError{"failed to start MCP client", err})
		return problem
	}
	return nil
}

func (m *Mark3labsTool) Describe() string {
	return fmt.Sprintf("mcp via mark3labs for %s", m.Name)
}

func (m *Mark3labsTool) Shutdown(shutdownContext context.Context) (problem error) {
	problem = m.mcpClient.Close()
	if m.active != nil {
		if err := m.active.stop(shutdownContext); err != nil {
			problem = errors.Join(problem, &operationalError{"failed to stop MCP client", err})
		}
	}
	return problem
}

// defineAPI queries the MCP server for available operations and returns Ollama tool
// definitions using namespaced names: "<toolName>.<operationName>".
func (m *Mark3labsTool) defineAPI(ctx context.Context) (definitions *toolDefinition, problem error) {
	if err := m.ensureRunning(ctx); err != nil {
		return nil, err
	}
	definitions = &toolDefinition{}

	discoveryContext, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()

	init, err := m.mcpClient.Initialize(discoveryContext, mcp.InitializeRequest{})
	if err != nil {
		return definitions, &operationalError{"failed to initialize client", err}
	}
	if init.Instructions != "" {
		definitions.appendInstruction(init.Instructions)
	}

	if init.Capabilities.Resources != nil {
		resources, err := m.mcpClient.ListResources(discoveryContext, mcp.ListResourcesRequest{})
		if err != nil {
			return definitions, &operationalError{"list resources", err}
		}
		if len(resources.Resources) > 0 {
			for _, r := range resources.Resources {
				content := fmt.Sprintf("# %s\nUse URI %s to access this resources\n%s", r.Name, r.URI, r.Description)
				m.resourceInstructions = append(m.resourceInstructions, api.Message{
					Role:    roleSystem,
					Content: content,
				})
				template, err := uritemplate.New(r.URI)
				if err != nil {
					return definitions, &operationalError{"parsing resource URI", err}
				}
				m.resourceTemplates = append(m.resourceTemplates, template)
			}
			definitions.uriHandler = m
		}
		resourceTemplates, err := m.mcpClient.ListResourceTemplates(discoveryContext, mcp.ListResourceTemplatesRequest{})
		if err != nil {
			return definitions, &operationalError{"list resource templates", err}
		}
		for _, rt := range resourceTemplates.ResourceTemplates {
			content := fmt.Sprintf("# %s\nURI template: %s\n%s\n", rt.Name, rt.URITemplate.Template.Raw(), rt.Description)
			m.resourceInstructions = append(m.resourceInstructions, api.Message{
				Role:    roleSystem,
				Content: content,
			})
			m.resourceTemplates = append(m.resourceTemplates, rt.URITemplate.Template)
		}
	}

	discovered, err := m.mcpClient.ListTools(discoveryContext, mcp.ListToolsRequest{})
	if err != nil {
		return definitions, &operationalError{"list tools", err}
	}
	for _, d := range discovered.Tools {
		fmt.Printf("mcp-%s\t>\tDiscovered tool %s\n", m.Name, d.Name)
		//todo: likely drift here -- will cause problems in the future
		var params api.ToolFunctionParameters
		bytes, err := json.Marshal(d.InputSchema)
		if err != nil {
			return definitions, &operationalError{"unmarshalling tooling", err}
		}
		if err := json.Unmarshal(bytes, &params); err != nil {
			return definitions, &operationalError{"translating tooling", err}
		}

		output := api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        m.namespaced(d.Name),
				Description: d.Description,
				Parameters:  params,
			},
		}
		definitions.tool = append(definitions.tool, output)
	}
	return definitions, nil
}

func (m *Mark3labsTool) namespaced(op string) string { return m.Name + "." + op }

// invoke executes the MCP tool operation based on a ToolCall and returns the
// corresponding tool message. The call.Function.Describe is expected to be
// "<toolName>.<operationName>".
func (m *Mark3labsTool) invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	// Extract the operation name from the namespaced function name
	opName := call.Function.Name
	if idx := strings.IndexByte(opName, '.'); idx >= 0 {
		opName = opName[idx+1:]
	}
	if opName == "" {
		return nil, fmt.Errorf("invalid tool name: %q", call.Function.Name)
	}

	//fmt.Printf("Invoking %q with arguments %#v\n", t.Program, t.Args)
	if err := m.ensureRunning(ctx); err != nil {
		return nil, err
	}

	c := m.mcpClient
	invocationContext, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	if err := c.Start(invocationContext); err != nil {
		return nil, &operationalError{"failed to start client", err}
	}

	_, err := c.Initialize(invocationContext, mcp.InitializeRequest{})
	if err != nil {
		return nil, &operationalError{"failed to initialize client", err}
	}

	//fmt.Printf("<\ttool\t%s\t%#v\n", opName, call.Function.Arguments)
	resp, err := c.CallTool(invocationContext, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      opName,
			Arguments: call.Function.Arguments,
		},
	})
	if err != nil {
		return []api.Message{
			toolResponseMessage(call, fmt.Sprintf("{\"error\":%q}", err.Error())),
		}, nil
	}
	for _, c := range resp.Content {
		if text, isText := c.(mcp.TextContent); isText {
			out = append(out, toolResponseMessage(call, text.Text))
		}
	}
	return out, nil
}

func (m *Mark3labsTool) matches() []*uritemplate.Template {
	return m.resourceTemplates
}

func (m *Mark3labsTool) describeMessages() []api.Message {
	return m.resourceInstructions
}

func (m *Mark3labsTool) readResource(ctx context.Context, invocation api.ToolCall, uri string) (output []api.Message, problem error) {
	if err := m.ensureRunning(ctx); err != nil {
		return nil, err
	}
	c := m.mcpClient
	result, err := c.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: uri,
		},
	})
	if err != nil {
		return nil, err
	}
	for _, rawContent := range result.Contents {
		switch content := rawContent.(type) {
		case mcp.TextResourceContents:
			output = append(output, toolResponseMessage(invocation, fmt.Sprintf("URI: %s\nContent-type: %s\n\n%s", content.URI, content.MIMEType, content.Text)))
		case mcp.BlobResourceContents:
			output = append(output, toolResponseMessage(invocation, fmt.Sprintf("URI: %s\nContent-type: %s\n\n%s", content.URI, content.MIMEType, string(content.Blob))))
		default:
			output = append(output, toolResponseMessage(invocation, fmt.Sprintf("Error: agent system could not interpret result")))
		}
	}
	return output, nil
}
