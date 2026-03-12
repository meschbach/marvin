package query

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yosida95/uritemplate/v3"
)

// mockTransport implements transport.Interface for testing
type mockTransport struct {
	initializeResult            *mcp.InitializeResult
	listResourcesResult         *mcp.ListResourcesResult
	listResourceTemplatesResult *mcp.ListResourceTemplatesResult
}

func (mt *mockTransport) Start(ctx context.Context) error                                           { return nil }
func (mt *mockTransport) Close() error                                                              { return nil }
func (mt *mockTransport) GetSessionId() string                                                      { return "test-session" }
func (mt *mockTransport) SetNotificationHandler(handler func(notification mcp.JSONRPCNotification)) {}

func (mt *mockTransport) SendRequest(ctx context.Context, req transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	var result interface{}
	switch req.Method {
	case "initialize":
		result = mt.initializeResult
	case "resources/list":
		result = mt.listResourcesResult
	case "resources/templates/list":
		result = mt.listResourceTemplatesResult
	case "tools/list":
		result = &mcp.ListToolsResult{Tools: []mcp.Tool{}}
	default:
		return nil, fmt.Errorf("unexpected method: %s", req.Method)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resultJSON,
	}, nil
}

func (mt *mockTransport) SendNotification(ctx context.Context, notification mcp.JSONRPCNotification) error {
	return nil
}

// mockRunningProgram implements runningProgram
type mockRunningProgram struct {
	transportImpl transport.Interface
}

func (mrp *mockRunningProgram) transport() transport.Interface {
	return mrp.transportImpl
}
func (mrp *mockRunningProgram) stop(ctx context.Context) error { return nil }

// mockProgramRuntimeSpec implements programRuntimeSpec
type mockProgramRuntimeSpec struct{}

func (ms *mockProgramRuntimeSpec) start(ctx context.Context) (runningProgram, error) {
	return &mockRunningProgram{}, nil
}

// TestMark3labsTool_DefineAPI_Idempotent tests that calling DefineAPI multiple times
// does not accumulate resource instructions and templates
func TestMark3labsTool_DefineAPI_Idempotent(t *testing.T) {
	t.Parallel()

	// Prepare mock responses
	initResult := &mcp.InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: mcp.ServerCapabilities{
			Resources: &struct {
				Subscribe   bool `json:"subscribe,omitempty"`
				ListChanged bool `json:"listChanged,omitempty"`
			}{Subscribe: false, ListChanged: false},
		},
		ServerInfo: mcp.Implementation{
			Name:    "test",
			Version: "1.0",
		},
		Instructions: "test",
	}

	uriTemplate := &mcp.URITemplate{
		Template: uritemplate.MustNew("test://template/{id}"),
	}

	listResourceTemplatesResult := &mcp.ListResourceTemplatesResult{
		ResourceTemplates: []mcp.ResourceTemplate{
			{
				Name:        "test-template",
				URITemplate: uriTemplate,
				Description: "A test template",
			},
		},
	}

	listResourcesResult := &mcp.ListResourcesResult{
		Resources: []mcp.Resource{
			{
				Name:        "test-resource",
				URI:         "test://resource/1",
				Description: "A test resource",
			},
		},
	}

	mt := &mockTransport{
		initializeResult:            initResult,
		listResourcesResult:         listResourcesResult,
		listResourceTemplatesResult: listResourceTemplatesResult,
	}

	mcpClient := client.NewClient(mt)

	tool := &Mark3labsTool{
		Name:      "test-tool",
		spec:      &mockProgramRuntimeSpec{},
		mcpClient: mcpClient,
		active: &mockRunningProgram{
			transportImpl: mt,
		},
	}

	// First call to DefineAPI
	definitions1, err := tool.DefineAPI(t.Context())
	require.NoError(t, err)
	require.NotNil(t, definitions1)

	instructionsCount1 := len(tool.resourceInstructions)
	templatesCount1 := len(tool.resourceTemplates)
	assert.Greater(t, instructionsCount1, 0, "should have resource instructions after first call")
	assert.Greater(t, templatesCount1, 0, "should have resource templates after first call")

	// Second call to DefineAPI
	definitions2, err := tool.DefineAPI(t.Context())
	require.NoError(t, err)
	require.NotNil(t, definitions2)

	instructionsCount2 := len(tool.resourceInstructions)
	templatesCount2 := len(tool.resourceTemplates)

	// These should be equal (no accumulation)
	assert.Equal(t, instructionsCount1, instructionsCount2,
		"resource instructions should not accumulate")
	assert.Equal(t, templatesCount1, templatesCount2,
		"resource templates should not accumulate")
}

// TestMark3labsTool_DescribeMessages_NoDuplicates tests that DescribeMessages
// returns resource instructions without duplicates even after multiple DefineAPI calls
func TestMark3labsTool_DescribeMessages_NoDuplicates(t *testing.T) {
	t.Parallel()

	// Prepare same mock responses
	initResult := &mcp.InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: mcp.ServerCapabilities{
			Resources: &struct {
				Subscribe   bool `json:"subscribe,omitempty"`
				ListChanged bool `json:"listChanged,omitempty"`
			}{Subscribe: false, ListChanged: false},
		},
		ServerInfo: mcp.Implementation{
			Name:    "test",
			Version: "1.0",
		},
		Instructions: "test",
	}

	uriTemplate := &mcp.URITemplate{
		Template: uritemplate.MustNew("test://template/{id}"),
	}

	listResourceTemplatesResult := &mcp.ListResourceTemplatesResult{
		ResourceTemplates: []mcp.ResourceTemplate{
			{
				Name:        "test-template",
				URITemplate: uriTemplate,
				Description: "A test template",
			},
		},
	}

	listResourcesResult := &mcp.ListResourcesResult{
		Resources: []mcp.Resource{
			{
				Name:        "test-resource",
				URI:         "test://resource/1",
				Description: "A test resource",
			},
		},
	}

	mt := &mockTransport{
		initializeResult:            initResult,
		listResourcesResult:         listResourcesResult,
		listResourceTemplatesResult: listResourceTemplatesResult,
	}

	mcpClient := client.NewClient(mt)

	tool := &Mark3labsTool{
		Name:      "test-tool",
		spec:      &mockProgramRuntimeSpec{},
		mcpClient: mcpClient,
		active: &mockRunningProgram{
			transportImpl: mt,
		},
	}

	// Call DefineAPI multiple times (simulating multi-turn chat)
	for i := 0; i < 3; i++ {
		_, err := tool.DefineAPI(t.Context())
		require.NoError(t, err)
	}

	messages := tool.DescribeMessages()

	// Should have exactly 2 instruction messages (1 resource + 1 template)
	assert.Len(t, messages, 2, "should have exactly 2 instruction messages")

	// Check for duplicate content
	contents := make([]string, len(messages))
	for i, msg := range messages {
		contents[i] = msg.Content
	}

	duplicateCount := countDuplicates(contents)
	assert.Equal(t, 0, duplicateCount, "should have no duplicate resource instruction messages")
}

func countDuplicates(items []string) int {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item]++
	}
	duplicates := 0
	for _, count := range counts {
		if count > 1 {
			duplicates += count - 1
		}
	}
	return duplicates
}
