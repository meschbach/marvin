// Package junk provides miscellaneous utilities and common types.
package junk

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Component defines an interface for components that can be described and shut down.
type Component interface {
	Describe() string
	Shutdown(ctx context.Context) error
}

// Container manages a collection of components.
type Container struct {
	name       string
	state      sync.Mutex
	components []Component
}

// NewContainer creates a new component container.
func NewContainer(name string) *Container {
	return &Container{
		name:       name,
		state:      sync.Mutex{},
		components: nil,
	}
}

// Register adds a new component to the container.
func (c *Container) Register(comp Component) {
	c.state.Lock()
	defer c.state.Unlock()
	c.components = append(c.components, comp)
}

// Describe returns a string representation of the container's components.
func (c *Container) Describe() string {
	return c.name
}

// Shutdown shuts down all components in the container.
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
