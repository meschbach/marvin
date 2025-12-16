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

const ToolTypeFunction = "function"

var ToolPropTypeString = []string{"string"}

type operationalError struct {
	description string
	underlying  error
}

func (o *operationalError) Error() string {
	return fmt.Sprintf("%s: %s", o.description, o.underlying.Error())
}

func (o *operationalError) Unwrap() error { return o.underlying }

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

// APIDefs queries the MCP server for available operations and returns Ollama tool
// definitions using namespaced names: "<toolName>.<operationName>".
func (t MCPLocalProgramTool) defineAPI(ctx context.Context) (tool api.Tools, problem error) {
	c := client.NewClient(transport.NewStdio(t.Program, []string{}, t.Args...))

	//fmt.Printf("Discovering tools for %q %#v...", t.Program, t.Args)
	discoveryContext, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	if err := c.Start(discoveryContext); err != nil {
		return nil, &operationalError{"failed to start client", err}
	}
	defer func() {
		problem = errors.Join(problem, c.Close())
	}()

	//fmt.Print("init...")
	_, err := c.Initialize(discoveryContext, mcp.InitializeRequest{})
	if err != nil {
		return nil, &operationalError{"failed to initialize client", err}
	}
	discovered, err := c.ListTools(discoveryContext, mcp.ListToolsRequest{})
	if err != nil {
		return nil, &operationalError{"list tools", err}
	}

	var tools api.Tools
	for _, d := range discovered.Tools {
		//todo: likely drift here -- will cause problems in the future
		var params api.ToolFunctionParameters
		bytes, err := json.Marshal(d.InputSchema)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(bytes, &params); err != nil {
			return nil, &operationalError{"translating tooling", err}
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
	//fmt.Printf("Done.\n")
	return tools, nil
}

func (t MCPLocalProgramTool) namespaced(op string) string { return t.Name + "." + op }

// Invoke executes the MCP tool operation based on a ToolCall and returns the
// corresponding tool message. The call.Function.Name is expected to be
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
			{
				Role:       "tool",
				ToolName:   call.Function.Name,
				ToolCallID: call.ID,
				Content:    fmt.Sprintf("{\"error\":%q}", err.Error()),
			},
		}, nil
	}
	//fmt.Printf("Invocation result: %#v\n", resp)
	for _, c := range resp.Content {
		if text, isText := c.(mcp.TextContent); isText {
			out = append(out, api.Message{
				Role:       "tool",
				Content:    text.Text,
				ToolName:   call.Function.Name,
				ToolCallID: call.ID,
			})
		}
	}
	return out, nil
}

type Tool interface {
	defineAPI(ctx context.Context) (tool api.Tools, problem error)
	invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error)
}

// ToolSet manages a collection of tools and provides helpers for chat integration.
type ToolSet struct {
	byName map[string]Tool // maps namespaced op name -> base Tool
	defs   api.Tools
}

type localProgramDiscoveryError struct {
	name       string
	underlying error
}

func (l *localProgramDiscoveryError) Unwrap() error {
	return l.underlying
}

func (l *localProgramDiscoveryError) Error() string {
	return fmt.Sprintf("failed to discover local program %q: %s", l.name, l.underlying.Error())
}

// NewToolSet builds a ToolSet from the parsed configuration. Nil cfg or empty
// content yields an empty ToolSet.
func NewToolSet(ctx context.Context, cfg *config.File) (*ToolSet, error) {
	ts := &ToolSet{byName: map[string]Tool{}}
	if cfg == nil {
		return ts, nil
	}
	for _, lp := range cfg.LocalPrograms {
		t := FromLocalProgram(lp)
		defs, err := t.defineAPI(ctx)
		if err != nil {
			return nil, &localProgramDiscoveryError{
				name:       t.Name,
				underlying: err,
			} // fail hard per requirements
		}
		for _, d := range defs {
			ts.byName[d.Function.Name] = t
		}
		ts.defs = append(ts.defs, defs...)
	}
	return ts, nil
}

func (ts *ToolSet) registerTool(ctx context.Context, t Tool) error {
	defs, err := t.defineAPI(ctx)
	if err != nil {
		return err
	}
	for _, d := range defs {
		ts.byName[d.Function.Name] = t
	}
	ts.defs = append(ts.defs, defs...)
	return nil
}

// APITools returns the list of api.Tool definitions to send with chat requests.
func (ts *ToolSet) APITools() api.Tools { return ts.defs }

// HandleCall invokes the named tool if available, otherwise returns an error tool message.
func (ts *ToolSet) HandleCall(ctx context.Context, call api.ToolCall) ([]api.Message, error) {
	t, ok := ts.byName[call.Function.Name]
	if !ok {
		// Return an error message so the model can recover gracefully
		errMsg := fmt.Sprintf("tool not found {name: %q}", call.Function.Name)
		return []api.Message{toolResponseMessage(call, fmt.Sprintf("{\"error\":%q}", errMsg))}, nil
	}
	msgs, err := t.invoke(ctx, call)
	if err != nil {
		err = &operationalError{fmt.Sprintf("tool invocation %q (id: %s)", call.Function.Name, call.ID), err}
	}
	return msgs, err
}

func toolResponseMessage(call api.ToolCall, content string) api.Message {
	return api.Message{
		Role:       "tool",
		ToolName:   call.Function.Name,
		ToolCallID: call.ID,
		Content:    content,
	}
}
