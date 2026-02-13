package junk

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Component interface {
	Describe() string
	Shutdown(ctx context.Context) error
}

type Container struct {
	name       string
	state      sync.Mutex
	components []Component
}

func NewContainer(name string) *Container {
	return &Container{
		name:       name,
		state:      sync.Mutex{},
		components: nil,
	}
}

func (c *Container) Register(comp Component) {
	c.state.Lock()
	defer c.state.Unlock()
	c.components = append(c.components, comp)
}

func (c *Container) Describe() string {
	return c.name
}

func (c *Container) Shutdown(ctx context.Context) (problem error) {
	c.state.Lock()
	defer c.state.Unlock()
	for _, comp := range c.components {
		if err := comp.Shutdown(ctx); err != nil {
			problem = errors.Join(problem, &OperationalError{fmt.Sprintf("failed to shutdown %s", comp.Describe()), err})
		}
	}
	c.components = nil
	return problem
}
