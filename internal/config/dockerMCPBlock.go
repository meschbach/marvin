package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type DockerMCPBlock struct {
	Name    string              `hcl:"name,label"`
	Image   string              `hcl:"image,label"`
	Args    []DockerMCPBlockArg `hcl:"args,block"`
	Mount   []DockerMCPMount    `hcl:"mount,block"`
	Env     []DockerMCPBlockEnv `hcl:"env,block"`
	Verbose *bool               `hcl:"verbose,optional"`
	//WorkingDirectory is an optionally overridable path.  By default, the working directory is the directory containing
	//the enclosing configuration.
	WorkingDirectory string `hcl:"working_directory,optional"`
}

func (d *DockerMCPBlock) ResolveVerbose() bool {
	if d.Verbose == nil {
		return false
	}
	return *d.Verbose
}

func (d *DockerMCPBlock) EnsureWorkingDirectory(marvinWorkingDirectory string) string {
	if filepath.IsAbs(d.WorkingDirectory) {
		return d.WorkingDirectory
	}
	d.WorkingDirectory = filepath.Join(marvinWorkingDirectory, d.WorkingDirectory)
	if d.ResolveVerbose() {
		fmt.Printf("docker-%s >{cfg} Using working directory: %s\n", d.Name, d.WorkingDirectory)
	}
	return d.WorkingDirectory
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

func (d *DockerMCPMount) ResolveSourcePath(dockerContextWorkingPath string) (string, error) {
	if filepath.IsAbs(d.Source) {
		return d.Source, nil
	}
	joined := filepath.Join(dockerContextWorkingPath, d.Source)
	absolute, err := filepath.Abs(joined)
	return absolute, err
}
