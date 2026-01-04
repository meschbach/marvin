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

const chromemToolDescriptionFormat = `The %s tool enhances your agent’s ability to retrieve accurate, contextually relevant information for decision-making. Here's a concise overview:

** %s Tool Description**:  
The tool combines facts, reasoning, and context to provide efficient, accurate information. It enables your agent to search for terms of interest (via the ` + "`search` function) and retrieve document content (via `read_document`) to support analysis or decision-making." + `

**Usage**:
1. **` + "`%s.search`" + `**: Input keywords or topics to find relevant information.
2. **` + "`%s.read_document`" + `**: Access pre-prepared documents to generate insights.

**Example**:
- Use ` + "`%s.search`" + ` to find technical specifications for a project.
- Use ` + "`%s.read_document`" + ` to access a research paper to support a hypothesis.

This integration streamlines information retrieval for real-time decision-making. Let me know if further details are needed!`

func (c *chromemTool) defineAPI(ctx context.Context) (definition *toolDefinition, problem error) {
	definitions := &toolDefinition{}
	definitions.instructions = append(definitions.instructions, api.Message{
		Role:    roleSystem,
		Content: fmt.Sprintf(chromemToolDescriptionFormat, c.config.Name, c.config.Name, c.config.Name, c.config.Name, c.config.Name, c.config.Name),
	})
	definitions.tool = append(definitions.tool, api.Tool{
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
	definitions.tool = append(definitions.tool, api.Tool{
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
	definitions.instructions = append(definitions.instructions, api.Message{
		Role:    roleSystem,
		Content: fmt.Sprintf("Use the tools `search` and `read_document` to search and read documents from the repository %q", c.config.Name),
	})
	return definitions, nil
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
			toolResponseMessage(call, "required parameter query is missing"),
		}, nil
	}
	unwrappedQuery, ok := query.(string)
	if !ok {
		return []api.Message{
			toolResponseMessage(call, "required parameter query must be a string"),
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
		if os.IsNotExist(err) {
			return []api.Message{
				toolResponseMessage(call, fmt.Sprintf("file %q does not exist", unwrappedPath)),
			}, nil
		}
		return nil, err
	}
	fileContents := string(wholeFile)

	return []api.Message{
		toolResponseMessage(call, fileContents),
	}, nil
}
