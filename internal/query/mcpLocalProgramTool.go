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
)

// MCPLocalProgramTool manages the lifecycle and invocation of a single configured local program.
type MCPLocalProgramTool struct {
	Name    string
	Program string
	Args    []string
}

// FromLocalProgram constructs a MCPLocalProgramTool from a LocalProgramBlock configuration block.
func FromLocalProgram(lp config.LocalProgramBlock) MCPLocalProgramTool {
	return MCPLocalProgramTool{
		Name:    lp.Name,
		Program: lp.Program,
		Args:    lp.Args,
	}
}

// defineAPI queries the MCP server for available operations and returns Ollama tool
// definitions using namespaced names: "<toolName>.<operationName>".
func (t MCPLocalProgramTool) defineAPI(ctx context.Context) (instructions []api.Message, tool api.Tools, problem error) {
	c := client.NewClient(transport.NewStdio(t.Program, []string{}, t.Args...))

	//fmt.Printf("Discovering tools for %q %#v...", t.Program, t.Args)
	discoveryContext, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	if err := c.Start(discoveryContext); err != nil {
		return instructions, nil, &operationalError{"failed to start client", err}
	}
	defer func() {
		problem = errors.Join(problem, c.Close())
	}()

	init, err := c.Initialize(discoveryContext, mcp.InitializeRequest{})
	if err != nil {
		return instructions, nil, &operationalError{"failed to initialize client", err}
	}
	discovered, err := c.ListTools(discoveryContext, mcp.ListToolsRequest{})
	if err != nil {
		return instructions, nil, &operationalError{"list tools", err}
	}

	var tools api.Tools
	for _, d := range discovered.Tools {
		//todo: likely drift here -- will cause problems in the future
		var params api.ToolFunctionParameters
		bytes, err := json.Marshal(d.InputSchema)
		if err != nil {
			return instructions, nil, err
		}
		if err := json.Unmarshal(bytes, &params); err != nil {
			return instructions, nil, &operationalError{"translating tooling", err}
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
	if init.Instructions == "" {
		instructions = append(instructions, api.Message{
			Role:    roleSystem,
			Content: init.Instructions,
		})
	}
	return instructions, tools, nil
}

func (t MCPLocalProgramTool) namespaced(op string) string { return t.Name + "." + op }

// invoke executes the MCP tool operation based on a ToolCall and returns the
// corresponding tool message. The call.Function.Describe is expected to be
// "<toolName>.<operationName>".
func (t MCPLocalProgramTool) invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	// Extract the operation name from the namespaced function name
	opName := call.Function.Name
	if idx := strings.IndexByte(opName, '.'); idx >= 0 {
		opName = opName[idx+1:]
	}
	if opName == "" {
		return nil, fmt.Errorf("invalid tool name: %q", call.Function.Name)
	}

	fmt.Printf("Invoking %q with arguments %#v\n", t.Program, t.Args)
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

	//fmt.Printf("invoking %q %#v...", t.Program, t.Args)
	invocationContext, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	if err := c.Start(invocationContext); err != nil {
		return nil, &operationalError{"failed to start client", err}
	}

	_, err := c.Initialize(invocationContext, mcp.InitializeRequest{})
	if err != nil {
		return nil, &operationalError{"failed to initialize client", err}
	}

	fmt.Printf("<\ttool\t%s\t%#v\n", opName, call.Function.Arguments)
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
	//fmt.Printf("Invocation result: %#v\n", resp)
	for _, c := range resp.Content {
		if text, isText := c.(mcp.TextContent); isText {
			out = append(out, toolResponseMessage(call, text.Text))
		}
	}
	return out, nil
}
