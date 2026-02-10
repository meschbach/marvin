# Testing Marvin + Slacker

## Quick Start

### Run All Tests
```bash
# Unit tests
go test -count 1 ./internal/...

# Integration tests
./test.sh

# With coverage
go test -cover ./internal/...
```

### Test Single Component
```bash
# Configuration tests
go test ./internal/config -v

# MCP server tests  
go test ./internal/mcp -v

# Slack integration tests
go test ./internal/slack -v
```

## Unit Testing

### Configuration Tests
```go
package config_test

import (
    "testing"
    "github.com/stretchr/testify/require"
    "github.com/meschbach/marvin/internal/config"
)

func TestLoadConfig_ValidHCL(t *testing.T) {
    hcl := `
model "ollama" "main" {
  name = "ministral-3:3b"
}

program "marvin" "default" {
  model = model.ollama.main
  description = "Test program"
}
`
    
    cfg, err := config.LoadFromString(hcl)
    require.NoError(t, err)
    assert.Len(t, cfg.Models, 1)
    assert.Len(t, cfg.Programs, 1)
}

func TestLoadConfig_MissingRequired(t *testing.T) {
    hcl := `
program "marvin" "invalid" {
  description = "Missing model reference"
}
`
    
    _, err := config.LoadFromString(hcl)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "missing required field")
}
```

### MCP Tool Tests
```go
package mcp_test

import (
    "context"
    "testing"
    "github.com/stretchr/testify/mock"
)

type MockTool struct {
    mock.Mock
}

func (m *MockTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    callArgs := m.Called(ctx, args)
    return callArgs.Get(0), callArgs.Error(1)
}

func TestToolManager_CallTool(t *testing.T) {
    manager := NewToolManager()
    mockTool := &MockTool{}
    
    manager.RegisterTool("test", mockTool)
    
    mockTool.On("Execute", mock.Anything, map[string]interface{}{
        "input": "test",
    }).Return("result", nil)
    
    result, err := manager.CallTool(context.Background(), "test", map[string]interface{}{
        "input": "test",
    })
    
    assert.NoError(t, err)
    assert.Equal(t, "result", result)
    mockTool.AssertExpectations(t)
}
```

## Integration Testing

### Slack Bot Tests
```go
package integration

import (
    "testing"
    "time"
    "github.com/slack-go/slack"
)

func TestSlackBot_MessageHandling(t *testing.T) {
    if testing.Short() {
        t.Skip("Integration test")
    }
    
    bot := setupTestBot(t)
    defer bot.Shutdown()
    
    // Send test message
    resp, err := bot.PostMessage("test-channel", slack.MsgOptionText("hello", false))
    require.NoError(t, err)
    assert.NotEmpty(t, resp.Timestamp)
    
    // Wait for response
    time.Sleep(2 * time.Second)
    
    // Verify response
    history, err := bot.GetConversationHistory(&slack.GetConversationHistoryParameters{
        ChannelID: "test-channel",
    })
    assert.NoError(t, err)
    
    messages := history.Messages
    assert.True(t, len(messages) >= 2) // Original + response
}

func setupTestBot(t *testing.T) *SlackBot {
    config := &SlackConfig{
        BotToken: os.Getenv("SLACK_BOT_TOKEN"),
        AppToken: os.Getenv("SLACK_APP_TOKEN"),
    }
    
    bot, err := NewSlackBot(config)
    require.NoError(t, err)
    
    return bot
}
```

### Ollama Integration Tests
```go
package integration

import (
    "context"
    "testing"
    "github.com/ollama/ollama/api"
)

func TestOllama_ClientConnection(t *testing.T) {
    if testing.Short() {
        t.Skip("Requires Ollama server")
    }
    
    client, err := api.ClientFromEnvironment()
    require.NoError(t, err)
    
    ctx := context.Background()
    
    // Test connection
    err = client.Heartbeat(ctx)
    assert.NoError(t, err)
    
    // Test model availability
    models, err := client.ListModels(ctx)
    assert.NoError(t, err)
    assert.NotEmpty(t, models.Models)
    
    // Test generation
    resp, err := client.Generate(ctx, &api.GenerateRequest{
        Model:  "ministral-3:3b",
        Prompt: "Say hello",
        Stream: new(mockStream),
    })
    assert.NoError(t, err)
    assert.NotEmpty(t, resp.Response)
}
```

## Test Configuration

### Test Config Files
Create `marvin.test.hcl`:
```hcl
model "ollama" "test" {
  name    = "ministral-3:3b"
  timeout = "30s"
}

program "marvin" "test" {
  model = model.ollama.test
  description = "Test configuration"
  
  tool "echo" {
    executable = "echo"
    args = ["test"]
  }
}
```

### Environment Setup
```bash
# .env.test
SLACK_BOT_TOKEN=xoxb-test-token
SLACK_APP_TOKEN=xapp-test-token
OLLAMA_HOST=http://localhost:11434
MARVIN_CONFIG=marvin.test.hcl
MARVIN_LOG_LEVEL=debug
```

### Test Docker Compose
```yaml
version: '3.8'
services:
  ollama:
    image: ollama/ollama
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama
    
  marvin-test:
    build: .
    environment:
      - OLLAMA_HOST=http://ollama:11434
      - MARVIN_CONFIG=/test/marvin.test.hcl
    volumes:
      - ./marvin.test.hcl:/test/marvin.test.hcl
    depends_on:
      - ollama

volumes:
  ollama_data:
```

## End-to-End Testing

### Full Conversation Flow
```go
func TestCompleteConversation(t *testing.T) {
    if testing.Short() {
        t.Skip("E2E test")
    }
    
    // Setup test environment
    testEnv := setupTestEnvironment(t)
    defer testEnv.Cleanup()
    
    // Start conversation
    engine := NewConversationEngine(testEnv.Config)
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    var responses []string
    updater := func(response string) {
        responses = append(responses, response)
    }
    
    // Run conversation
    err := engine.RunConversation(ctx, "ministral-3:3b", updater)
    require.NoError(t, err)
    
    // Verify responses
    assert.NotEmpty(t, responses)
    assert.Contains(t, responses[len(responses)-1], "goodbye") // Final response
}
```

### Slack Workflow Tests
```go
func TestSlackApprovalWorkflow(t *testing.T) {
    if testing.Short() {
        t.Skip("Slack integration")
    }
    
    slackBot := setupSlackBot(t)
    
    // User requests tool access
    msg, err := slackBot.HandleMessage(&slack.MessageEvent{
        ChannelID: "test-channel",
        User:      "test-user",
        Text:      "/marvin-tool docker ps",
    })
    require.NoError(t, err)
    
    // Should create approval request
    assert.Contains(t, msg.Text, "approval request")
    
    // Admin approves
    approveMsg, err := slackBot.HandleMessage(&slack.MessageEvent{
        ChannelID: "admins",
        User:      "admin-user",
        Text:      "/approve 1",
    })
    require.NoError(t, err)
    
    assert.Contains(t, approveMsg.Text, "approved")
    
    // Original command should execute
    time.Sleep(1 * time.Second)
    history := getChannelHistory(t, "test-channel")
    assert.Contains(t, getLastMessage(history).Text, "CONTAINER ID")
}
```

## Performance Testing

### Load Testing
```go
func BenchmarkConversationEngine(b *testing.B) {
    engine := setupBenchmarkEngine()
    ctx := context.Background()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        updater := func(string) {} // Discard responses
        
        err := engine.RunConversation(ctx, "ministral-3:3b", updater)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkConcurrentRequests(b *testing.B) {
    engine := setupBenchmarkEngine()
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            ctx := context.Background()
            updater := func(string) {}
            
            err := engine.RunConversation(ctx, "ministral-3:3b", updater)
            if err != nil {
                b.Error(err)
            }
        }
    })
}
```

### Memory Profiling
```go
func TestMemoryUsage(t *testing.T) {
    var m1, m2 runtime.MemStats
    runtime.GC()
    runtime.ReadMemStats(&m1)
    
    engine := NewConversationEngine(loadTestConfig())
    ctx := context.Background()
    updater := func(string) {}
    
    // Run 100 conversations
    for i := 0; i < 100; i++ {
        engine.RunConversation(ctx, "ministral-3:3b", updater)
    }
    
    runtime.GC()
    runtime.ReadMemStats(&m2)
    
    // Memory usage should be reasonable
    memUsed := m2.Alloc - m1.Alloc
    assert.Less(t, memUsed, uint64(100*1024*1024)) // Less than 100MB
}
```

## Test Utilities

### Mock Server
```go
func setupMockOllama(t *testing.T) *httptest.Server {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/api/tags":
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]interface{}{
                "models": []map[string]interface{}{
                    {"name": "ministral-3:3b"},
                },
            })
        case "/api/generate":
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]interface{}{
                "response": "Hello!",
                "done":     true,
            })
        default:
            http.NotFound(w, r)
        }
    }))
    
    t.Cleanup(server.Close)
    return server
}
```

### Test Helpers
```go
func createTestConfig(t *testing.T) *config.Config {
    hcl := `
model "ollama" "test" {
  name = "ministral-3:3b"
}

program "marvin" "test" {
  model = model.ollama.test
  description = "Test"
}
`
    
    cfg, err := config.LoadFromString(hcl)
    require.NoError(t, err)
    return cfg
}

func assertNoTestLeaks(t *testing.T) {
    // Check for goroutine leaks
    runtime.GC()
    time.Sleep(100 * time.Millisecond)
    
    // Add specific leak detection logic
}
```

## CI/CD Testing

### GitHub Actions
```yaml
name: Test
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      ollama:
        image: ollama/ollama
        ports:
          - 11434:11434
        options: --health-cmd="curl http://localhost:11434/api/tags" --health-interval=30s
    
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      
      - name: Wait for Ollama
        run: |
          timeout 60 bash -c 'until curl -f http://localhost:11434/api/tags; do sleep 2; done'
      
      - name: Pull model
        run: |
          docker exec ${{ job.services.ollama.id }} ollama pull ministral-3:3b
      
      - name: Run tests
        run: go test -v ./...
        env:
          OLLAMA_HOST: http://localhost:11434
      
      - name: Run integration tests
        run: ./test.sh
        if: github.event_name == 'push'
```

### Test Data Management
```bash
#!/bin/bash
# test-data.sh - Setup test data

setup_test_data() {
    # Create test configuration
    cat > marvin.test.hcl << 'EOF'
model "ollama" "test" {
  name = "ministral-3:3b"
}
EOF
    
    # Download test models
    ollama pull ministral-3:3b
    
    # Create test documents
    mkdir -p test-data/docs
    echo "Test document content" > test-data/docs/test.md
}

cleanup_test_data() {
    rm -f marvin.test.hcl
    rm -rf test-data
}
```

## Troubleshooting Tests

### Common Test Failures
1. **Race Conditions**: Use `-race` flag
2. **Time Sensitivity**: Add proper waits/timeouts
3. **Resource Leaks**: Check goroutine and file handle leaks
4. **Network Issues**: Mock external dependencies
5. **Environment Variables**: Set up test environment properly

### Debug Tools
```bash
# Verbose test output
go test -v ./...

# Run specific test with coverage
go test -run TestLoadConfig -cover ./internal/config

# Race detection
go test -race ./...

# Memory profiling
go test -memprofile=mem.prof -bench=. ./...

# CPU profiling
go test -cpuprofile=cpu.prof -bench=. ./...
```