package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/junk"
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
	assistantPrompt      *config.AssistantPromptBlock
}

func (m *Mark3labsTool) ensureRunning(ctx context.Context) (problem error) {
	if m.active != nil {
		return nil
	}
	m.active, problem = m.spec.start(ctx)
	m.mcpClient = client.NewClient(m.active.transport())
	if err := m.mcpClient.Start(ctx); err != nil {
		problem = errors.Join(problem, &junk.OperationalError{Description: "failed to start MCP client", Underlying: err})
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
			problem = errors.Join(problem, &junk.OperationalError{Description: "failed to stop MCP client", Underlying: err})
		}
	}
	return problem
}

// DefineAPI queries the MCP server for available operations and returns Ollama tool
// definitions using namespaced names: "<toolName>_<operationName>".
func (m *Mark3labsTool) DefineAPI(ctx context.Context) (definitions *conversation.ToolDefinition, problem error) {
	if err := m.ensureRunning(ctx); err != nil {
		return nil, err
	}
	definitions = &conversation.ToolDefinition{}

	discoveryContext, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()

	init, err := m.initializeMCPClient(discoveryContext)
	if err != nil {
		return definitions, err
	}

	if err := m.processInitializationResult(definitions, init); err != nil {
		return definitions, err
	}

	if init.Capabilities.Resources != nil {
		if err := m.processResources(discoveryContext, definitions); err != nil {
			return definitions, err
		}
		if err := m.processResourceTemplates(discoveryContext, definitions); err != nil {
			return definitions, err
		}
	}

	discovered, err := m.mcpClient.ListTools(discoveryContext, mcp.ListToolsRequest{})
	if err != nil {
		return definitions, &junk.OperationalError{Description: "list tools", Underlying: err}
	}
	for _, d := range discovered.Tools {
		tool, err := m.convertToolDefinition(d)
		if err != nil {
			return definitions, err
		}
		definitions.Tool = append(definitions.Tool, tool)
	}
	return definitions, nil
}

func (m *Mark3labsTool) initializeMCPClient(ctx context.Context) (*mcp.InitializeResult, error) {
	init, err := m.mcpClient.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		return nil, &junk.OperationalError{Description: "failed to initialize client", Underlying: err}
	}
	return init, nil
}

func (m *Mark3labsTool) processInitializationResult(definitions *conversation.ToolDefinition, init *mcp.InitializeResult) error {
	if init.Instructions != "" {
		definitions.AppendInstruction(init.Instructions)
	}

	if assistantPromptContent, err := m.resolveAssistantPrompt(); err != nil {
		return &junk.OperationalError{Description: "resolving assistant prompt", Underlying: err}
	} else if assistantPromptContent != "" {
		definitions.AppendInstruction(assistantPromptContent)
	}

	return nil
}

func (m *Mark3labsTool) processResources(ctx context.Context, definitions *conversation.ToolDefinition) error {
	resources, err := m.mcpClient.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return &junk.OperationalError{Description: "list resources", Underlying: err}
	}

	for _, r := range resources.Resources {
		content := fmt.Sprintf("# %s\nUse URI %s to access this resources\n%s", r.Name, r.URI, r.Description)
		m.resourceInstructions = append(m.resourceInstructions, api.Message{
			Role:    conversation.RoleSystem,
			Content: content,
		})
		template, err := uritemplate.New(r.URI)
		if err != nil {
			return &junk.OperationalError{Description: "parsing resource URI", Underlying: err}
		}
		m.resourceTemplates = append(m.resourceTemplates, template)
	}
	definitions.UriHandler = m
	return nil
}

func (m *Mark3labsTool) processResourceTemplates(ctx context.Context, definitions *conversation.ToolDefinition) error {
	resourceTemplates, err := m.mcpClient.ListResourceTemplates(ctx, mcp.ListResourceTemplatesRequest{})
	if err != nil {
		return &junk.OperationalError{Description: "list resource templates", Underlying: err}
	}
	for _, rt := range resourceTemplates.ResourceTemplates {
		content := fmt.Sprintf("# %s\nURI template: %s\n%s\n", rt.Name, rt.URITemplate.Raw(), rt.Description)
		m.resourceInstructions = append(m.resourceInstructions, api.Message{
			Role:    conversation.RoleSystem,
			Content: content,
		})
		m.resourceTemplates = append(m.resourceTemplates, rt.URITemplate.Template)
	}
	return nil
}

func (m *Mark3labsTool) convertToolDefinition(d mcp.Tool) (api.Tool, error) {
	var params api.ToolFunctionParameters
	bytes, err := json.Marshal(d.InputSchema)
	if err != nil {
		return api.Tool{}, &junk.OperationalError{Description: "unmarshalling tooling", Underlying: err}
	}
	if err := json.Unmarshal(bytes, &params); err != nil {
		return api.Tool{}, &junk.OperationalError{Description: "translating tooling", Underlying: err}
	}

	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        m.namespaced(d.Name),
			Description: d.Description,
			Parameters:  params,
		},
	}, nil
}

func (m *Mark3labsTool) namespaced(op string) string { return m.Name + "_" + op }

// Invoke executes the MCP tool operation based on a ToolCall and returns the
// corresponding tool message. The call.Function.Describe is expected to be
// "<toolName>_<operationName>".
func (m *Mark3labsTool) Invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	c := m.mcpClient
	invocationContext, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()

	if err := m.ensureRunning(ctx); err != nil {
		return nil, err
	}
	// Extract the operation name from the namespaced function name
	opName := call.Function.Name
	if idx := strings.IndexByte(opName, '_'); idx >= 0 {
		opName = opName[idx+1:]
	}
	if opName == "" {
		return nil, fmt.Errorf("invalid tool name: %q", call.Function.Name)
	}

	//fmt.Printf("Invoking %q with arguments %#v\n", t.Program, t.Args)
	if err := m.ensureRunning(ctx); err != nil {
		return nil, err
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
			conversation.ToolResponseMessage(call, fmt.Sprintf("{\"error\":%q}", err.Error())),
		}, nil
	}
	for _, c := range resp.Content {
		if text, isText := c.(mcp.TextContent); isText {
			out = append(out, conversation.ToolResponseMessage(call, text.Text))
		}
	}
	return out, nil
}

func (m *Mark3labsTool) Matches() []*uritemplate.Template {
	return m.resourceTemplates
}

// resolveAssistantPrompt resolves the assistant prompt content from the configuration
func (m *Mark3labsTool) resolveAssistantPrompt() (string, error) {
	if m.assistantPrompt == nil {
		return "", nil
	}

	if len(m.assistantPrompt.FromString) > 0 {
		return m.assistantPrompt.FromString, nil
	}

	if len(m.assistantPrompt.FromFile) > 0 {
		contents, err := os.ReadFile(m.assistantPrompt.FromFile)
		if err != nil {
			return "", fmt.Errorf("reading assistant prompt file %q: %w", m.assistantPrompt.FromFile, err)
		}
		return string(contents), nil
	}

	return "", nil
}

func (m *Mark3labsTool) DescribeMessages() []api.Message {
	return m.resourceInstructions
}

func (m *Mark3labsTool) ReadResource(ctx context.Context, invocation api.ToolCall, uri string) (output []api.Message, problem error) {
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
			output = append(output, conversation.ToolResponseMessage(invocation, fmt.Sprintf("URI: %s\nContent-type: %s\n\n%s", content.URI, content.MIMEType, content.Text)))
		case mcp.BlobResourceContents:
			output = append(output, conversation.ToolResponseMessage(invocation, fmt.Sprintf("URI: %s\nContent-type: %s\n\n%s", content.URI, content.MIMEType, string(content.Blob))))
		default:
			output = append(output, conversation.ToolResponseMessage(invocation, "Error: agent system could not interpret result"))
		}
	}
	return output, nil
}
