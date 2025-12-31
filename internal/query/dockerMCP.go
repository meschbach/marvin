package query

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

// dockerMCPTool manages the lifecycle and invocation of a single configured docker MCP service using stdio transport.
type dockerMCPTool struct {
	cfg          *config.DockerMCPBlock
	mcpClient    *client.Client
	docker       *dockerclient.Client
	containerID  string
	transport    types.HijackedResponse
	stdoutWriter io.WriteCloser
	stdoutReader *io.PipeReader
	initResult   *mcp.InitializeResult
}

func (d *dockerMCPTool) Describe() string {
	return fmt.Sprintf("Docker MCP %s", d.cfg.Name)
}

func (d *dockerMCPTool) launch(ctx context.Context) error {
	verbose := d.cfg.ResolveVerbose()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return &operationalError{"failed to create docker client", err}
	}
	d.docker = cli

	// 1. Pull image if it does not exist
	_, _, err = cli.ImageInspectWithRaw(ctx, d.cfg.Image)
	if err != nil {
		if dockerclient.IsErrNotFound(err) {
			fmt.Printf("Pulling image %s...\n", d.cfg.Image)
			pullOut, err := cli.ImagePull(ctx, d.cfg.Image, image.PullOptions{})
			if err != nil {
				return &operationalError{"failed to pull docker image", err}
			}
			defer pullOut.Close()
			io.Copy(io.Discard, pullOut)
		} else {
			return &operationalError{"failed to inspect docker image", err}
		}
	}

	// 2. Prepare container config
	var envs []string
	for _, e := range d.cfg.Env {
		key, value, err := e.ResolveValue()
		if err != nil {
			return err
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
			return &operationalError{fmt.Sprintf("failed to resolve mount target %s:%s", m.Source, m.Target), err}
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

	if d.cfg.ResolveVerbose() {
		dockerArgs := strings.Join(containerArgs, " ")
		fmt.Printf("docker-%s > `docker run --rm -i %s %s`\n", d.cfg.Name, d.cfg.Image, dockerArgs)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
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
		return &operationalError{"failed to create docker container", err}
	}
	d.containerID = resp.ID

	// 3. Attach to container
	attach, err := cli.ContainerAttach(ctx, d.containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return &operationalError{"failed to attach to docker container", err}
	}

	// 4. Start container
	if err := cli.ContainerStart(ctx, d.containerID, container.StartOptions{}); err != nil {
		attach.Close()
		return &operationalError{"failed to start docker container", err}
	}

	// 5. Setup MCP client
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()

	go func() {
		stdcopy.StdCopy(stdoutWriter, stderrWriter, attach.Reader)
		stdoutWriter.CloseWithError(io.EOF)
		stderrWriter.CloseWithError(io.EOF)
	}()
	d.transport = attach
	d.stdoutWriter = stdoutWriter
	d.stdoutReader = stdoutReader

	startupTimer, startupDone := context.WithTimeout(ctx, 10*time.Second)
	defer startupDone()
	d.mcpClient = client.NewClient(transport.NewIO(stdoutReader, attach.Conn, stderrReader))
	d.mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
		fmt.Printf("docker-%s >{notification} %s\n", d.cfg.Name, notification.JSONRPC)
	})
	startedStderrPump := make(chan struct{})
	go func() {
		close(startedStderrPump) //todo: very bad practice
		if d.cfg.ResolveVerbose() {
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
	if err := d.mcpClient.Start(startupTimer); err != nil {
		return &operationalError{"failed to start docker mcp client", err}
	}
	if d.cfg.ResolveVerbose() {
		fmt.Printf("docker-%s > Container started, sending initialize message\n", d.cfg.Name)
	}

	d.initResult, err = d.mcpClient.Initialize(startupTimer, mcp.InitializeRequest{})
	if err != nil {
		return &operationalError{"failed to invoke Initialize", err}
	}
	if d.cfg.ResolveVerbose() {
		fmt.Printf("docker-%s > Initialize complete.\n", d.cfg.Name)
	}

	return nil
}

// Shutdown cleans up the docker container and MCP client.
func (d *dockerMCPTool) Shutdown(shutdownContext context.Context) (problem error) {
	//if err := d.stdoutWriter.Close(); err != nil {
	//	problem = errors.Join(problem, &operationalError{"failed to close stdout pipe", err})
	//}
	if d.mcpClient != nil {
		if err := d.mcpClient.Close(); err != nil {
			problem = errors.Join(problem, &operationalError{"failed to close MCP API client", err})
		}
	}
	fmt.Printf("docker-%s > Shutting down container...\n", d.cfg.Name)
	if d.docker != nil && d.containerID != "" {
		// Use a separate context for shutdown to ensure it has enough time even if shutdownContext is short
		stopCtx, cancel := context.WithTimeout(shutdownContext, 15*time.Second)
		defer cancel()

		// Stop and remove the container
		stopTimeout := 10
		if err := d.docker.ContainerStop(stopCtx, d.containerID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
			problem = errors.Join(problem, &operationalError{"failed to stop container", err})
		}
		containers, err := d.docker.ContainerList(stopCtx, container.ListOptions{})
		if err != nil {
			problem = errors.Join(problem, &operationalError{"failed to list containers", err})
		} else {
			for _, c := range containers {
				if c.ID == d.containerID {
					if err := d.docker.ContainerRemove(stopCtx, d.containerID, container.RemoveOptions{Force: true}); err != nil {
						if d.cfg.ResolveVerbose() {
							fmt.Printf("(normal) Docker reported container removal error which is normal after stop for a conflict.  Please confirm %s\n", err.Error())
						}
					}
				}
			}
		}
		if err := d.docker.Close(); err != nil {
			problem = errors.Join(problem, &operationalError{"filed to cleanly close docker client", err})
		}
	}
	return problem
}

func (d *dockerMCPTool) defineAPI(ctx context.Context) ([]api.Message, api.Tools, error) {
	var out []api.Message
	discovered, err := d.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, nil, &operationalError{"list tools", err}
	}

	if d.initResult.Instructions != "" {
		out = append(out, api.Message{
			Role:    roleSystem,
			Content: d.initResult.Instructions,
		})
	}

	var tools api.Tools
	for _, dtl := range discovered.Tools {
		var params api.ToolFunctionParameters
		bytes, err := json.Marshal(dtl.InputSchema)
		if err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal(bytes, &params); err != nil {
			return nil, nil, &operationalError{"translating tooling", err}
		}

		output := api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        d.namespaced(dtl.Name),
				Description: dtl.Description,
				Parameters:  params,
			},
		}
		tools = append(tools, output)
	}
	return out, tools, nil
}

func (d *dockerMCPTool) namespaced(op string) string {
	return d.cfg.Name + "." + op
}

func (d *dockerMCPTool) invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	verbose := d.cfg.ResolveVerbose()

	opName := call.Function.Name
	if idx := strings.IndexByte(opName, '.'); idx >= 0 {
		opName = opName[idx+1:]
	}
	if opName == "" {
		return nil, fmt.Errorf("invalid tool name: %q", call.Function.Name)
	}

	resp, err := d.mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      opName,
			Arguments: call.Function.Arguments,
		},
	})
	if err != nil {
		if verbose {
			fmt.Printf("docker-%s >{error} mapping to error result from %s\n", d.cfg.Name, err.Error())
		}
		return []api.Message{
			toolResponseMessage(call, fmt.Sprintf("{\"error\":%q}", err.Error())),
		}, nil
	}
	if verbose {
		fmt.Printf("docker-%s > received %d results.\n", d.cfg.Name, len(resp.Content))
	}
	for _, c := range resp.Content {
		if text, isText := c.(mcp.TextContent); isText {
			out = append(out, toolResponseMessage(call, text.Text))
		} else {
			if verbose {
				fmt.Printf("docker-%s >{warning} not mapping content type %T\n", d.cfg.Name, c)
			}
		}
	}
	return out, nil
}

func (ts *ToolSet) loadToolsFromDocker(ctx context.Context, cfg *config.File) (problem error) {
	for _, mcpCfg := range cfg.DockerMCPBlock {
		tool := &dockerMCPTool{cfg: mcpCfg}
		ts.container.Register(tool)
		if err := tool.launch(ctx); err != nil {
			problem = errors.Join(problem, &operationalError{
				description: fmt.Sprintf("launching docker MCP server %q", tool.Describe()),
				underlying:  err,
			})
			continue
		}
		if err := ts.registerTool(ctx, tool); err != nil {
			problem = errors.Join(problem, err)
		}
	}
	return problem
}
