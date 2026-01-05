package query

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/meschbach/marvin/internal/config"
)

func FromDockerSpec(cfg *config.DockerMCPBlock) *Mark3labsTool {
	spec := &dockerRuntimeSpec{cfg: cfg}
	return &Mark3labsTool{
		Name: cfg.Name,
		spec: spec,
	}
}

type dockerRuntimeSpec struct {
	cfg *config.DockerMCPBlock
}

func (d *dockerRuntimeSpec) start(ctx context.Context) (program runningProgram, problem error) {
	verbose := d.cfg.ResolveVerbose()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, &operationalError{"failed to create docker client", err}
	}
	defer func() {
		if problem != nil && cli != nil {
			if err := cli.Close(); err != nil {
				problem = errors.Join(problem, &operationalError{"failed to close docker client", err})
			}
		}
	}()

	// 1. Pull image if it does not exist
	_, _, err = cli.ImageInspectWithRaw(ctx, d.cfg.Image)
	if err != nil {
		if dockerclient.IsErrNotFound(err) {
			fmt.Printf("Pulling image %s...\n", d.cfg.Image)
			pullOut, err := cli.ImagePull(ctx, d.cfg.Image, image.PullOptions{})
			if err != nil {
				return nil, &operationalError{"failed to pull docker image", err}
			}
			defer func() {
				if err := pullOut.Close(); err != nil {
					problem = errors.Join(problem, &operationalError{"failed to close docker pull output", err})
				}
			}()
			if _, err := io.Copy(io.Discard, pullOut); err != nil {
				problem = errors.Join(problem, &operationalError{"failed to discard docker pull output", err})
			}
		} else {
			return nil, &operationalError{"failed to inspect docker image", err}
		}
	}

	// 2. Prepare container config
	var envs []string
	for _, e := range d.cfg.Env {
		key, value, err := e.ResolveValue()
		if err != nil {
			return nil, &operationalError{
				description: fmt.Sprintf("failed to resolve %s", key),
				underlying:  err,
			}
		}

		spec := fmt.Sprintf("%s=%s", key, value)
		if verbose {
			fmt.Printf("docker-%s >{env} %s\n", d.cfg.Name, spec)
		}
		envs = append(envs, spec)
	}

	var binds []string
	for _, m := range d.cfg.Mount {
		source, err := m.ResolveSourcePath(d.cfg.WorkingDirectory)
		if err != nil {
			return nil, &operationalError{fmt.Sprintf("failed to resolve mount target %s:%s", m.Source, m.Target), err}
		}
		mountStr := fmt.Sprintf("%s:%s", source, m.Target)
		if m.Options != "" {
			mountStr += ":" + m.Options
		}
		binds = append(binds, mountStr)
	}

	var containerArgs []string
	for _, a := range d.cfg.Args {
		containerArgs = append(containerArgs, a.Strings...)
	}

	if verbose {
		dockerArgs := strings.Join(containerArgs, " ")
		fmt.Printf("docker-%s > `docker run --rm -i %s %s`\n", d.cfg.Name, d.cfg.Image, dockerArgs)
	}

	createContainerReply, err := cli.ContainerCreate(ctx, &container.Config{
		Image:     d.cfg.Image,
		Cmd:       containerArgs,
		Env:       envs,
		OpenStdin: true,
		StdinOnce: true,
		Tty:       false,
	}, &container.HostConfig{
		Binds:      binds,
		AutoRemove: true,
	}, nil, nil, "")
	if err != nil {
		return nil, &operationalError{"failed to create docker container", err}
	}

	// 3. Attach to container
	attach, err := cli.ContainerAttach(ctx, createContainerReply.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, &operationalError{"failed to attach to docker container", err}
	}

	// 4. Start container
	if err := cli.ContainerStart(ctx, createContainerReply.ID, container.StartOptions{}); err != nil {
		attach.Close()
		return nil, &operationalError{"failed to start docker container", err}
	}

	// 5. Setup MCP client
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()

	go func() {
		stdcopy.StdCopy(stdoutWriter, stderrWriter, attach.Reader)
		stdoutWriter.CloseWithError(io.EOF)
		stderrWriter.CloseWithError(io.EOF)
	}()
	startedStderrPump := make(chan struct{})

	go func() {
		close(startedStderrPump) //todo: very bad practice
		if verbose {
			scanner := bufio.NewScanner(stderrReader)
			for scanner.Scan() {
				fmt.Printf("docker-%s >{stderr} %s\n", d.cfg.Name, scanner.Text())
			}
			//todo: handle errors.
		} else {
			io.Copy(io.Discard, stderrReader)
		}
	}()
	<-startedStderrPump

	bridge := transport.NewIO(stdoutReader, attach.Conn, stderrReader)
	return &dockerContainer{
		name:         d.cfg.Name,
		verbose:      verbose,
		bridge:       bridge,
		dockerClient: cli,
		containerID:  createContainerReply.ID,
	}, nil
}

type dockerContainer struct {
	verbose      bool
	name         string
	bridge       transport.Interface
	dockerClient *dockerclient.Client
	containerID  string
}

func (d dockerContainer) transport() transport.Interface {
	return d.bridge
}

func (d dockerContainer) stop(ctx context.Context) (problem error) {
	if d.verbose {
		fmt.Printf("docker-%s > Shutting down container...\n", d.name)
	}
	if d.verbose {
		defer func() {
			if problem != nil {
				fmt.Printf("docker-%s > Shut down with errors %s.\n", d.name, problem.Error())
			} else {
				fmt.Printf("docker-%s > Shut down complete with no errors.\n", d.name)
			}
		}()
	}
	verbose := d.verbose
	stopCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Stop and remove the container
	stopTimeout := 10
	if d.verbose {
		fmt.Printf("docker-%s > Stopping container...\n", d.name)
	}
	if err := d.dockerClient.ContainerStop(stopCtx, d.containerID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		problem = errors.Join(problem, &operationalError{"failed to stop container", err})
	}

	//Ensure the container is removed.
	if d.verbose {
		fmt.Printf("docker-%s > Stopped.  Ensuring container is removed...\n", d.name)
	}
	containers, err := d.dockerClient.ContainerList(stopCtx, container.ListOptions{})
	if err != nil {
		problem = errors.Join(problem, &operationalError{"failed to list containers", err})
	} else {
		for _, c := range containers {
			if c.ID == d.containerID {
				if err := d.dockerClient.ContainerRemove(stopCtx, d.containerID, container.RemoveOptions{Force: true}); err != nil {
					if verbose {
						fmt.Printf("(normal) Docker reported container removal error which is normal after stop for a conflict.  Please confirm %s\n", err.Error())
					}
				}
			}
		}
	}
	if err := d.dockerClient.Close(); err != nil {
		problem = errors.Join(problem, &operationalError{"filed to cleanly close docker client", err})
	}
	return nil
}

func (ts *ToolSet) loadToolsFromDocker(ctx context.Context, cfg *config.File) (problem error) {
	for _, mcpCfg := range cfg.DockerMCPBlock {
		tool := FromDockerSpec(mcpCfg)
		ts.container.Register(tool)
		if err := ts.registerTool(ctx, tool); err != nil {
			return &operationalError{
				description: fmt.Sprintf("failed to register %s", mcpCfg.Name),
				underlying:  err,
			}
		}
	}
	return problem
}
