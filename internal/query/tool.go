package query

import (
	"context"
	"fmt"
	"sync"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

const ToolTypeFunction = "function"

var ToolPropTypeString = []string{"string"}

type Tool interface {
	defineAPI(ctx context.Context) (instructions []api.Message, tool api.Tools, problem error)
	invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error)
}

// ToolSet manages a collection of tools and provides helpers for chat integration.
type ToolSet struct {
	instructions []api.Message
	byName       map[string]Tool // maps namespaced op name -> base Tool
	defs         api.Tools
	container    *Container
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
	ts := &ToolSet{
		byName: map[string]Tool{},
		container: &Container{
			name:  "tool container",
			state: sync.Mutex{},
		},
	}

	if cfg == nil {
		return ts, nil
	}
	for _, lp := range cfg.LocalPrograms {
		t := FromLocalProgram(lp)
		if err := ts.registerTool(ctx, t); err != nil {
			return nil, &localProgramDiscoveryError{
				name:       t.Name,
				underlying: err,
			} // fail hard per requirements
		}
	}
	if err := ts.loadToolsFromDocker(ctx, cfg); err != nil {
		return nil, err
	}
	return ts, nil
}

func (ts *ToolSet) registerTool(ctx context.Context, t Tool) error {
	thisInstructions, defs, err := t.defineAPI(ctx)
	if err != nil {
		return err
	}
	for _, d := range defs {
		ts.byName[d.Function.Name] = t
	}
	ts.defs = append(ts.defs, defs...)
	ts.instructions = append(ts.instructions, thisInstructions...)
	return nil
}

// APITools returns the list of api.Tool definitions to send with chat requests.
func (ts *ToolSet) APITools() api.Tools { return ts.defs }

func (ts *ToolSet) Shutdown(ctx context.Context) error {
	return ts.container.Shutdown(ctx)
}

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

// toolResponseMessage is a utility to respond to a tool invocation with some content
func toolResponseMessage(call api.ToolCall, content string) api.Message {
	return api.Message{
		Role:       "tool",
		ToolName:   call.Function.Name,
		ToolCallID: call.ID,
		Content:    content,
	}
}
