# Custom MCP Tools Development

## MCP Server Basics

Model Context Protocol (MCP) servers extend Marvin with custom capabilities. Each server provides specific tools that
the AI can call.

### Server Structure
```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net"
    "os"
)

type MCPServer struct {
    name    string
    version string
    tools   []Tool
}

type Tool struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    InputSchema struct {
        Type       string                 `json:"type"`
        Properties map[string]interface{} `json:"properties"`
        Required   []string               `json:"required"`
    } `json:"inputSchema"`
}
```

## Simple Tool Example

### Basic Calculator
```go
func main() {
    server := &MCPServer{
        name:    "calculator",
        version: "1.0.0",
        tools: []Tool{
            {
                Name:        "add",
                Description: "Add two numbers",
                InputSchema: json.RawMessage(`{
                    "type": "object",
                    "properties": {
                        "a": {"type": "number"},
                        "b": {"type": "number"}
                    },
                    "required": ["a", "b"]
                }`),
            },
        },
    }

    if err := server.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}

func (s *MCPServer) HandleToolCall(ctx context.Context, name string, args json.RawMessage) (interface{}, error) {
    switch name {
    case "add":
        var params struct {
            A float64 `json:"a"`
            B float64 `json:"b"`
        }
        if err := json.Unmarshal(args, &params); err != nil {
            return nil, err
        }
        return map[string]float64{"result": params.A + params.B}, nil
    default:
        return nil, fmt.Errorf("unknown tool: %s", name)
    }
}
```

## Advanced Tool Example

### Docker Container Manager
```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
)

type DockerMCP struct {
    socketPath string
}

func main() {
    socket := "/var/run/docker.sock"
    if len(os.Args) > 1 {
        socket = os.Args[1]
    }

    server := &DockerMCP{socketPath: socket}
    server.Start()
}

func (d *DockerMCP) GetTools() []Tool {
    return []Tool{
        {
            Name:        "list_containers",
            Description: "List running Docker containers",
            InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "all": {"type": "boolean", "description": "Show all containers"}
                }
            }`),
        },
        {
            Name:        "run_container",
            Description: "Start a new container",
            InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "image": {"type": "string"},
                    "command": {"type": "array", "items": {"type": "string"}},
                    "detach": {"type": "boolean"}
                },
                "required": ["image"]
            }`),
        },
    }
}

func (d *DockerMCP) HandleToolCall(ctx context.Context, name string, args json.RawMessage) (interface{}, error) {
    switch name {
    case "list_containers":
        var params struct {
            All bool `json:"all"`
        }
        json.Unmarshal(args, &params)

        dockerArgs := []string{"ps"}
        if params.All {
            dockerArgs = append(dockerArgs, "-a")
        }

        return d.runDockerCommand(ctx, dockerArgs...)

    case "run_container":
        var params struct {
            Image   string   `json:"image"`
            Command []string `json:"command"`
            Detach  bool     `json:"detach"`
        }
        json.Unmarshal(args, &params)

        dockerArgs := []string{"run"}
        if params.Detach {
            dockerArgs = append(dockerArgs, "-d")
        }
        dockerArgs = append(dockerArgs, params.Image)
        dockerArgs = append(dockerArgs, params.Command...)

        return d.runDockerCommand(ctx, dockerArgs...)

    default:
        return nil, fmt.Errorf("unknown tool: %s", name)
    }
}

func (d *DockerMCP) runDockerCommand(ctx context.Context, args ...string) (string, error) {
    cmd := exec.CommandContext(ctx, "docker", args...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return string(output), err
    }
    return string(output), nil
}
```

## MCP Protocol Implementation

### Server Communication
```go
type MCPRequest struct {
    Jsonrpc string      `json:"jsonrpc"`
    ID      interface{} `json:"id"`
    Method  string      `json:"method"`
    Params  interface{} `json:"params,omitempty"`
}

type MCPResponse struct {
    Jsonrpc string      `json:"jsonrpc"`
    ID      interface{} `json:"id"`
    Result  interface{} `json:"result,omitempty"`
    Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}

func (s *MCPServer) HandleRequest(req MCPRequest) MCPResponse {
    switch req.Method {
    case "initialize":
        return s.handleInitialize(req.ID)
    case "tools/list":
        return s.handleListTools(req.ID)
    case "tools/call":
        return s.handleToolCall(req.ID, req.Params)
    default:
        return MCPResponse{
            Jsonrpc: "2.0",
            ID:      req.ID,
            Error: &MCPError{
                Code:    -32601,
                Message: "Method not found",
            },
        }
    }
}

func (s *MCPServer) handleInitialize(id interface{}) MCPResponse {
    return MCPResponse{
        Jsonrpc: "2.0",
        ID:      id,
        Result: map[string]interface{}{
            "protocolVersion": "2024-11-05",
            "capabilities": map[string]interface{}{
                "tools": map[string]interface{}{},
            },
            "serverInfo": map[string]interface{}{
                "name":    s.name,
                "version": s.version,
            },
        },
    }
}
```

## Configuration Integration

### HCL Configuration
```hcl
mcp "custom-tool" {
  command = "/path/to/custom-mcp-server"
  args    = ["--config", "/etc/mcp-config.yaml"]

  config = {
    api_key = "${env.API_KEY}"
    timeout = "30s"
    debug   = true
  }
}
```

### Environment Variables
```go
func getConfig() map[string]interface{} {
    config := make(map[string]interface{})

    if apiKey := os.Getenv("API_KEY"); apiKey != "" {
        config["api_key"] = apiKey
    }

    if timeout := os.Getenv("TIMEOUT"); timeout != "" {
        config["timeout"] = timeout
    }

    if debug := os.Getenv("DEBUG"); debug == "true" {
        config["debug"] = true
    }

    return config
}
```

## Error Handling

### Structured Errors
```go
type ToolError struct {
    Code    string                 `json:"code"`
    Message string                 `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

func (e ToolError) Error() string {
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (s *MCPServer) handleToolCall(id interface{}, params interface{}) MCPResponse {
    toolParams, ok := params.(map[string]interface{})
    if !ok {
        return MCPResponse{
            Jsonrpc: "2.0",
            ID:      id,
            Error: &MCPError{
                Code:    -32602,
                Message: "Invalid params",
            },
        }
    }

    toolName, ok := toolParams["name"].(string)
    if !ok {
        return s.errorResponse(id, "MISSING_TOOL_NAME", "Tool name required")
    }

    args, _ := json.Marshal(toolParams["arguments"])
    result, err := s.HandleToolCall(context.Background(), toolName, args)
    if err != nil {
        if toolErr, ok := err.(ToolError); ok {
            return MCPResponse{
                Jsonrpc: "2.0",
                ID:      id,
                Error: &MCPError{
                    Code:    -32000,
                    Message: toolErr.Message,
                    Data:    toolErr.Details,
                },
            }
        }
        return s.errorResponse(id, "TOOL_ERROR", err.Error())
    }

    return MCPResponse{
        Jsonrpc: "2.0",
        ID:      id,
        Result: map[string]interface{}{
            "content": []interface{}{
                map[string]interface{}{
                    "type": "text",
                    "text": fmt.Sprintf("%v", result),
                },
            },
        },
    }
}
```

## Testing MCP Tools

### Unit Tests
```go
func TestCalculatorAdd(t *testing.T) {
    server := &MCPServer{}

    args, _ := json.Marshal(map[string]float64{
        "a": 2.0,
        "b": 3.0,
    })

    result, err := server.HandleToolCall(context.Background(), "add", args)
    assert.NoError(t, err)

    expected := map[string]float64{"result": 5.0}
    assert.Equal(t, expected, result)
}
```

### Integration Tests
```go
func TestMCPCommunication(t *testing.T) {
    cmd := exec.Command("./test-mcp-server")
    stdin, err := cmd.StdinPipe()
    require.NoError(t, err)

    stdout, err := cmd.StdoutPipe()
    require.NoError(t, err)

    err = cmd.Start()
    require.NoError(t, err)
    defer cmd.Process.Kill()

    // Test initialize
    initReq := MCPRequest{
        Jsonrpc: "2.0",
        ID:      1,
        Method:  "initialize",
        Params: map[string]interface{}{
            "protocolVersion": "2024-11-05",
        },
    }

    json.NewEncoder(stdin).Encode(initReq)

    var response MCPResponse
    json.NewDecoder(stdout).Decode(&response)

    assert.Equal(t, 1, response.ID)
    assert.NotNil(t, response.Result)
}
```

## Deployment

### Docker Deployment
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o custom-mcp-server ./cmd/custom-mcp-server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/custom-mcp-server .
EXPOSE 8080
CMD ["./custom-mcp-server", "--port", "8080"]
```

### Systemd Service
```ini
[Unit]
Description=Custom MCP Server
After=network.target

[Service]
Type=simple
User=marvin
ExecStart=/usr/local/bin/custom-mcp-server
Restart=always
RestartSec=5
Environment=API_KEY_FILE=/etc/marvin/api.key

[Install]
WantedBy=multi-user.target
```

## Best Practices

### Security
- Validate all input parameters
- Sanitize file paths
- Use least privilege access
- Never log sensitive data
- Implement timeouts for external calls

### Performance
- Use connection pooling for external APIs
- Cache expensive operations
- Implement request rate limiting
- Monitor memory usage

### Reliability
- Graceful error handling
- Health check endpoints
- Structured logging
- Circuit breakers for external services

### Maintainability
- Clear error messages
- Comprehensive tests
- Documentation for each tool
- Version compatibility matrix
