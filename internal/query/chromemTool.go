package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

type chromemTool struct {
	config          *config.DocumentsBlock
	showInvocations bool
}

const ChromemSearchQueryParameter = "query"
const ChromemDocumentPathParameter = "filename"

func (c *chromemTool) defineAPI(ctx context.Context) (tool api.Tools, problem error) {
	tools := make(api.Tools, 0)
	tools = append(tools, api.Tool{
		Type: ToolTypeFunction,
		Function: api.ToolFunction{
			Name:        "search",
			Description: fmt.Sprintf("Searches the document repository %q for a document matching the given query", c.config.Description),
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
			Description: fmt.Sprintf("Retrieves a specific document from the repository %q", c.config.Description),
			Parameters: api.ToolFunctionParameters{
				Type:     ToolTypeFunction,
				Required: []string{ChromemDocumentPathParameter},
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
	if c.showInvocations {
		fmt.Printf("rag> invoked chromem tool %s\n", call.Function.Name)
	}
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

	if len(matches) == 0 {
		return []api.Message{
			toolResponseMessage(call, fmt.Sprintf("no matches for %q found", unwrappedQuery)),
		}, nil
	}
	var output []api.Message
	for _, match := range matches {
		output = append(output, toolResponseMessage(call, fmt.Sprintf("the file %q matched the query %q", match.Path, unwrappedQuery)))
	}
	return output, nil
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

	//resolve relative
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	documentBase := filepath.Join(wd, c.config.DocumentPath)
	unwrappedPath = filepath.Join(documentBase, unwrappedPath)
	wholeFile, err := os.ReadFile(unwrappedPath)
	if err != nil {
		return nil, err
	}
	fileContents := string(wholeFile)

	return []api.Message{
		{
			Role:       roleToolResult,
			ToolCallID: call.ID,
			Content:    fileContents,
		},
	}, nil
}
