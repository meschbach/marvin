package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
	"github.com/yosida95/uritemplate/v3"
)

// MCPLocalProgramTool manages the lifecycle and invocation of a single configured local program.
type MCPLocalProgramTool struct {
	Name                 string
	Program              string
	Args                 []string
	resourceInstructions []api.Message
	resourceTemplates    []*uritemplate.Template
}

// FromLocalProgram constructs a MCPLocalProgramTool from a LocalProgramBlock configuration block.
func FromLocalProgram(lp config.LocalProgramBlock) *MCPLocalProgramTool {
	return &MCPLocalProgramTool{
		Name:    lp.Name,
		Program: lp.Program,
		Args:    lp.Args,
	}
}

// defineAPI queries the MCP server for available operations and returns Ollama tool
// definitions using namespaced names: "<toolName>.<operationName>".
func (t *MCPLocalProgramTool) defineAPI(ctx context.Context) (definitions *toolDefinition, problem error) {
	definitions = &toolDefinition{}

	c := client.NewClient(transport.NewStdio(t.Program, []string{}, t.Args...))

	//fmt.Printf("Discovering tools for %q %#v...\n", t.Program, t.Args)
	discoveryContext, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	if err := c.Start(discoveryContext); err != nil {
		return definitions, &operationalError{"failed to start client", err}
	}
	defer func() {
		if err := c.Close(); err != nil {
			//problem = errors.Join(problem, &operationalError{"failed to close client", err})
		}
	}()

	init, err := c.Initialize(discoveryContext, mcp.InitializeRequest{})
	if err != nil {
		return definitions, &operationalError{"failed to initialize client", err}
	}

	if init.Instructions != "" {
		definitions.appendInstruction(init.Instructions)
	}

	resources, err := c.ListResources(discoveryContext, mcp.ListResourcesRequest{})
	if err != nil {
		return definitions, &operationalError{"list resources", err}
	}
	if len(resources.Resources) > 0 {
		for _, r := range resources.Resources {
			content := fmt.Sprintf("# %s\nUse URI %s to access this resources\n%s", r.Name, r.URI, r.Description)
			t.resourceInstructions = append(t.resourceInstructions, api.Message{
				Role:    roleSystem,
				Content: content,
			})
			template, err := uritemplate.New(r.URI)
			if err != nil {
				return definitions, &operationalError{"parsing resource URI", err}
			}
			t.resourceTemplates = append(t.resourceTemplates, template)
		}
		definitions.uriHandler = t
	}

	resourceTemplates, err := c.ListResourceTemplates(discoveryContext, mcp.ListResourceTemplatesRequest{})
	if err != nil {
		return definitions, &operationalError{"list resource templates", err}
	}
	//fmt.Printf("resource templates: %d\n", len(resourceTemplates.ResourceTemplates))
	for _, rt := range resourceTemplates.ResourceTemplates {
		content := fmt.Sprintf("# %s\nURI template: %s\n%s\n", rt.Name, rt.URITemplate.Template.Raw(), rt.Description)
		t.resourceInstructions = append(t.resourceInstructions, api.Message{
			Role:    roleSystem,
			Content: content,
		})
		t.resourceTemplates = append(t.resourceTemplates, rt.URITemplate.Template)
	}
	//fmt.Printf("resource instructions: %d\n", len(t.resourceInstructions))

	discovered, err := c.ListTools(discoveryContext, mcp.ListToolsRequest{})
	if err != nil {
		return definitions, &operationalError{"list tools", err}
	}

	var tools api.Tools
	for _, d := range discovered.Tools {
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
				Name:        t.namespaced(d.Name),
				Description: d.Description,
				Parameters:  params,
			},
		}
		tools = append(tools, output)
	}
	return definitions, nil
}

func (t *MCPLocalProgramTool) namespaced(op string) string { return t.Name + "." + op }

// invoke executes the MCP tool operation based on a ToolCall and returns the
// corresponding tool message. The call.Function.Describe is expected to be
// "<toolName>.<operationName>".
func (t *MCPLocalProgramTool) invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	// Extract the operation name from the namespaced function name
	opName := call.Function.Name
	if idx := strings.IndexByte(opName, '.'); idx >= 0 {
		opName = opName[idx+1:]
	}
	if opName == "" {
		return nil, fmt.Errorf("invalid tool name: %q", call.Function.Name)
	}

	//fmt.Printf("Invoking %q with arguments %#v\n", t.Program, t.Args)
	c := client.NewClient(transport.NewStdio(t.Program, []string{}, t.Args...))
	defer func() {
		closeErr := c.Close()
		var execErr *exec.ExitError
		if errors.As(closeErr, &execErr) {
			//ignore as the service is already dead
		} else {
			problem = errors.Join(problem, closeErr)
		}
	}()

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

func (t *MCPLocalProgramTool) matches() []*uritemplate.Template {
	return t.resourceTemplates
}

func (t *MCPLocalProgramTool) describeMessages() []api.Message {
	return t.resourceInstructions
}

func (t *MCPLocalProgramTool) readResource(ctx context.Context, invocation api.ToolCall, uri string) (output []api.Message, problem error) {
	c := client.NewClient(transport.NewStdio(t.Program, []string{}, t.Args...))
	if err := c.Start(ctx); err != nil {
		return nil, err
	}
	defer func() {
		closeErr := c.Close()
		var execErr *exec.ExitError
		if errors.As(closeErr, &execErr) {
			//ignore as the service is already dead
		} else {
			problem = errors.Join(problem, closeErr)
		}
	}()
	_, err := c.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		return nil, err
	}
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
			fmt.Printf("mcp-local-program:%s >\tignoring resource content type %T\n", t.Name, content)
		}
	}
	return output, nil
}
