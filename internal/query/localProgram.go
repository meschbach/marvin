package query

import (
	"context"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/meschbach/marvin/internal/config"
)

// FromLocalProgram constructs a tool capable of invoking a local program specified in the configuration
func FromLocalProgram(lp config.LocalProgramBlock) *Mark3labsTool {
	spec := &localProgramRuntimeSpec{
		Name:    lp.Name,
		Program: lp.Program,
		Args:    lp.Args,
	}
	return &Mark3labsTool{
		Name: lp.Name,
		spec: spec,
	}
}

type localProgramRuntimeSpec struct {
	Name    string
	Program string
	Args    []string
}

func (l *localProgramRuntimeSpec) start(ctx context.Context) (runningProgram, error) {
	stdioTransport := transport.NewStdio(l.Program, []string{}, l.Args...)
	return &localRunningProgram{stdioTransport}, nil
}

type localRunningProgram struct {
	mcpTransport transport.Interface
}

func (l *localRunningProgram) transport() transport.Interface {
	return l.mcpTransport
}
func (l *localRunningProgram) stop(ctx context.Context) error {
	return nil
}
