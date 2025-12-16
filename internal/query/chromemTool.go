package query

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

type chromemTool struct {
	config *config.DocumentsBlock
}

const ChromemSearchQueryParameter = "query"
const ChromemDocumentPathParameter = "path"

func (c *chromemTool) defineAPI(ctx context.Context) (tool api.Tools, problem error) {
	tools := make(api.Tools, 0)
	tools = append(tools, api.Tool{
		Type: ToolTypeFunction,
		Function: api.ToolFunction{
			Name:        "search",
			Description: fmt.Sprintf("searches the document repository containing %s for a similarities to the given description.  returns the path name and similar to the query.  use read_document tool to retrieve the contents of the document.", c.config.Description),
			Parameters: api.ToolFunctionParameters{
				Type:     ToolTypeFunction,
				Required: []string{"query"},
				Properties: map[string]api.ToolProperty{
					ChromemSearchQueryParameter: {
						Type:        ToolPropTypeString,
						Description: "query terms of interest to search for",
					},
				},
			},
		},
	})
	tools = append(tools, api.Tool{
		Type: ToolTypeFunction,
		Function: api.ToolFunction{
			Name:        "read_document",
			Description: "Reads a document from the repository",
			Parameters: api.ToolFunctionParameters{
				Type:     ToolTypeFunction,
				Required: []string{"path"},
				Properties: map[string]api.ToolProperty{
					ChromemDocumentPathParameter: {
						Type:        ToolPropTypeString,
						Description: "path name to read",
					},
				},
			},
		},
	})
	return tools, nil
}

func (c *chromemTool) invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	fmt.Printf("rag> invoked chromem tool %s\n", call.Function.Name)
	functionName := call.Function.Name

	switch functionName {
	case "search":
		return c.search(ctx, call)
	case "read_document":
		return c.readDocument(ctx, call)
	default:
		return []api.Message{
			toolResponseMessage(call, "no such function"+functionName),
		}, nil
	}
}

func (c *chromemTool) search(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	query, has := call.Function.Arguments[ChromemSearchQueryParameter]
	if !has {
		return []api.Message{
			{
				Role:       roleToolResult,
				Content:    "required parameter query is missing",
				ToolCallID: call.ID,
			},
		}, nil
	}
	unwrappedQuery, ok := query.(string)
	if !ok {
		return []api.Message{
			{
				Role:       roleToolResult,
				Content:    "required parameter query must be a string",
				ToolCallID: call.ID,
			},
		}, nil
	}

	matches, err := c.config.Query(ctx, unwrappedQuery)
	if err != nil {
		return nil, err
	}

	output, err := json.Marshal(matches)
	if err != nil {
		return nil, err
	}
	return []api.Message{
		toolResponseMessage(call, string(output)),
	}, nil
}

func (c *chromemTool) readDocument(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	path, has := call.Function.Arguments[ChromemDocumentPathParameter]
	if !has {
		return nil, fmt.Errorf("required parameter path is missing")
	}
	unwrappedPath, ok := path.(string)
	if !ok {
		return nil, fmt.Errorf("required parameter path must be a string")
	}
	return []api.Message{
		{
			Role:       roleToolResult,
			ToolCallID: call.ID,
			Content:    unwrappedPath,
		},
	}, nil
}
