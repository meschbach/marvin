package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/llm"
)

// ChromemTool provides RAG functionality for searching and reading documents.
type ChromemTool struct {
	config          *config.DocumentsBlock
	showInvocations bool
}

// NewChromemTool creates a new ChromemTool for the given documents config.
func NewChromemTool(cfg *config.DocumentsBlock, showInvocations bool) *ChromemTool {
	return &ChromemTool{
		config:          cfg,
		showInvocations: showInvocations,
	}
}

const ChromemSearchQueryParameter = "query"
const ChromemDocumentPathParameter = "filename"

const ChromemToolDescriptionFormat = `The %s tool enhances your agent's ability to retrieve accurate, contextually relevant information for decision-making. Here's a concise overview:

** %s Tool Description**:  
The tool combines facts, reasoning, and context to provide efficient, accurate information. It enables your agent to search for terms of interest (via the ` + "`search` function) and retrieve document content (via `read_document`) to support analysis or decision-making." + `

**Usage**:
1. **` + "`%s.search`" + `**: Input keywords or topics to find relevant information.
2. **` + "`%s.read_document`" + `**: Access pre-prepared documents to generate insights.

**Example**:
- Use ` + "`%s.search`" + ` to find technical specifications for a project.
- Use ` + "`%s.read_document`" + ` to access a research paper to support a hypothesis.

This integration streamlines information retrieval for real-time decision-making. Let me know if further details are needed!`

func (c *ChromemTool) DefineAPI(ctx context.Context) (definition *conversation.ToolDefinition, problem error) {
	definitions := &conversation.ToolDefinition{}
	definitions.Instructions = append(definitions.Instructions, llm.Message{
		Role:    conversation.RoleSystem,
		Content: fmt.Sprintf(ChromemToolDescriptionFormat, c.config.Name, c.config.Name, c.config.Name, c.config.Name, c.config.Name, c.config.Name),
	})
	searchProps := map[string]llm.ToolProperty{
		ChromemSearchQueryParameter: {
			Type:        conversation.ToolPropTypeString,
			Description: "query terms of interest to search for",
		},
	}
	definitions.Tool = append(definitions.Tool, llm.ToolDefinition{
		Type: conversation.ToolTypeFunction,
		Function: llm.ToolFunction{
			Name:        "search",
			Description: fmt.Sprintf("Searches the document repository %q for a document matching the given query", c.config.Description),
			Parameters: &llm.ToolFunctionParameters{
				Type:       "object",
				Required:   []string{"query"},
				Properties: searchProps,
			},
		},
	})
	readProps := map[string]llm.ToolProperty{
		ChromemDocumentPathParameter: {
			Type:        conversation.ToolPropTypeString,
			Description: "path name to read",
		},
	}
	definitions.Tool = append(definitions.Tool, llm.ToolDefinition{
		Type: conversation.ToolTypeFunction,
		Function: llm.ToolFunction{
			Name:        "read_document",
			Description: fmt.Sprintf("Retrieves a specific document from the repository %q", c.config.Description),
			Parameters: &llm.ToolFunctionParameters{
				Type:       "object",
				Required:   []string{ChromemDocumentPathParameter},
				Properties: readProps,
			},
		},
	})
	definitions.Instructions = append(definitions.Instructions, llm.Message{
		Role:    conversation.RoleSystem,
		Content: fmt.Sprintf("Use the tools `search` and `read_document` to search and read documents from the repository %q", c.config.Name),
	})
	return definitions, nil
}

func (c *ChromemTool) Invoke(ctx context.Context, call llm.ToolCall) (out []llm.Message, problem error) {
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
		return []llm.Message{
			conversation.ToolResponseMessage(call, "no such function"+functionName),
		}, nil
	}
}

func (c *ChromemTool) search(ctx context.Context, call llm.ToolCall) (out []llm.Message, problem error) {
	args, ok := call.Function.Arguments.(map[string]any)
	if !ok {
		return []llm.Message{
			conversation.ToolResponseMessage(call, "required parameter query is missing"),
		}, nil
	}
	query, has := args[ChromemSearchQueryParameter]
	if !has {
		return []llm.Message{
			conversation.ToolResponseMessage(call, "required parameter query is missing"),
		}, nil
	}
	unwrappedQuery, ok := query.(string)
	if !ok {
		return []llm.Message{
			conversation.ToolResponseMessage(call, "required parameter query must be a string"),
		}, nil
	}

	matches, err := c.config.Query(ctx, unwrappedQuery)
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		return []llm.Message{
			conversation.ToolResponseMessage(call, fmt.Sprintf("no Matches for %q found", unwrappedQuery)),
		}, nil
	}
	var output []llm.Message
	for _, match := range matches {
		output = append(output, conversation.ToolResponseMessage(call, fmt.Sprintf("the file %q matched the query %q", match.Path, unwrappedQuery)))
	}
	return output, nil
}

func (c *ChromemTool) readDocument(ctx context.Context, call llm.ToolCall) (out []llm.Message, problem error) {
	args, ok := call.Function.Arguments.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("required parameter path is missing")
	}
	path, has := args[ChromemDocumentPathParameter]
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
			return []llm.Message{
				conversation.ToolResponseMessage(call, fmt.Sprintf("file %q does not exist", unwrappedPath)),
			}, nil
		}
		return nil, err
	}
	fileContents := string(wholeFile)

	return []llm.Message{
		conversation.ToolResponseMessage(call, fileContents),
	}, nil
}
