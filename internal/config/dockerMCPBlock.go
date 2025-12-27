package config

import (
	"errors"
	"os"
)

type DockerMCPBlock struct {
	Name    string              `hcl:"name,label"`
	Image   string              `hcl:"image,label"`
	Args    []DockerMCPBlockArg `hcl:"args_string,block"`
	Mount   []DockerMCPMount    `hcl:"mount,block"`
	Value   []DockerMCPBlockEnv `hcl:"env,block"`
	Verbose *bool               `hcl:"verbose,optional"`
}

func (d *DockerMCPBlock) ResolveVerbose() bool {
	if d.Verbose == nil {
		return false
	}
	return *d.Verbose
}

type DockerMCPBlockEnv struct {
	Key         string `hcl:"key,label"`
	Value       string `hcl:"value,optional"`
	Passthrough *bool  `hcl:"pass_through,optional"`
}

func (d *DockerMCPBlockEnv) ResolveValue() (string, string, error) {
	if d.Value != "" {
		if d.Passthrough != nil && *d.Passthrough {
			return "", "", errors.New("only value or pass_through can be set, not both")
		}
		return d.Key, d.Value, nil
	}
	return d.Key, os.Getenv(d.Key), nil
}

type DockerMCPBlockArg struct {
	Strings []string `hcl:"strings"`
}

type DockerMCPMount struct {
	Target  string `hcl:"target,label"`
	Source  string `hcl:"source,label"`
	Options string `hcl:"options,optional"`
}
