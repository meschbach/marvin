# RAG (Retrieval Augmented Generation)

## Overview

RAG provides semantic document search capabilities for Marvin, enabling AI agents to query indexed document repositories using natural language. The system combines vector storage with embedding-based similarity search to retrieve relevant documents.

### Technologies

| Component | Technology | Purpose |
|-----------|------------|---------|
| Vector Database | [chromem-go](https://github.com/philippgille/chromem-go) | Persistent vector storage and similarity search |
| Embeddings | Ollama | Generate embeddings via `mxbai-embed-large:latest` |

### CLI Integration

```bash
marvin rag index              # Index all documents from config
marvin rag query <store> <query>  # Query a RAG store
```

---

## Configuration

The `documents` block configures a document repository for indexing and search.

**File:** `internal/config/documentsBlock.go:14-22`

```go
type DocumentsBlock struct {
    Name         string `hcl:"name,label"`
    DocumentPath string `hcl:"document_path,label"`
    StoragePath  string `hcl:"storage_path,optional"`
    Description  string `hcl:"description,optional"`
    Model        string `hcl:"model,optional"`
}
```

### Configuration Options

| Field | Type | Required | Description |
|-------|------|----------|--------------|
| `name` | string | Yes | Label identifying this document repository |
| `document_path` | string | Yes | Path to documents to index |
| `storage_path` | string | Yes | Path for vector database storage |
| `description` | string | No | Description of the document repository |
| `model` | string | No | Embedding model (defaults to `mxbai-embed-large:latest`) |

### Default Embedding Model

**File:** `internal/config/file.go:16`

```go
const DefaultEmbeddingModel = "mxbai-embed-large:latest"
```

---

## CLI Commands

### `rag index`

Indexes all documents from the configuration file. Walks the document path and adds all files to the vector database.

**File:** `cmd/marvin/rag.go:13-33`

```go
index := &cobra.Command{
    Use:   "index",
    Short: "Indexes all documents from the configuration file",
    Run: func(cmd *cobra.Command, args []string) {
        // Loads config and indexes each DocumentsBlock
    },
}
```

### `rag query <store> <query>`

Queries a specific RAG store with a search query.

**File:** `cmd/marvin/rag.go:35-56`

```go
query := &cobra.Command{
    Use:   "query <store> <query>",
    Short: "Queries the RAG store",
    Args:  cobra.ExactArgs(2),
}
```

---

## Implementation

### ChromemTool

The ChromemTool provides RAG functionality as an MCP tool for the AI agent.

**File:** `internal/query/chromemTool.go:14-173`

```go
type ChromemTool struct {
    config          *config.DocumentsBlock
    showInvocations bool
}
```

#### Methods

| Method | Description |
|--------|-------------|
| `DefineAPI` | Defines the tool's API with `search` and `read_document` functions |
| `Invoke` | Dispatches tool calls to appropriate handlers |
| `search` | Performs semantic search against the vector store |
| `readDocument` | Reads a specific document by path |

#### Tool Functions

**File:** `internal/query/chromemTool.go:46-91`

1. **`search`** - Searches the document repository for documents matching a query
   - Parameter: `query` (string, required)

2. **`read_document`** - Retrieves the content of a specific document
   - Parameter: `filename` (string, required)

### Ollama Encoder

The ollamaEncoder wraps Ollama's embeddings API for chromem-go.

**File:** `internal/config/ollama.go:11-45`

```go
type ollamaEncoder struct {
    client    *api.Client
    modelName string
}

func (o *ollamaEncoder) Encode(ctx context.Context, text string) ([]float32, error)
```

The encoder:
1. Calls Ollama's `/embeddings` endpoint
2. Converts `[]float64` to `[]float32`
3. Normalizes the vector to unit length (required by chromem-go)

---

## Key Types

### QueryResult

**File:** `internal/config/documentsBlock.go:24-27`

```go
type QueryResult struct {
    Path       string  `json:"path"`
    Similarity float32 `json:"similarity"`
}
```

### DocumentsBlock Methods

| Method | Description |
|--------|-------------|
| `Query` | Queries the vector store for similar documents |
| `Index` | Indexes all documents from `DocumentPath` into the vector store |
| `EmbeddingModel` | Returns the embedding model (with default fallback) |

**Index Details:** `internal/config/documentsBlock.go:71-159`

- Walks `DocumentPath` recursively
- Reads file contents and creates chromem.Document entries
- Adds documents to the collection with concurrency of 4
- Stores file path as document ID and in metadata

---

## Integration Points

RAG tools are loaded from configuration in two locations:

### Query Initialization

**File:** `internal/query/query.go:71`

For CLI queries, ChromemTool is created and added to the conversation tool set:

```go
tool := &ChromemTool{config: rag, showInvocations: false}
```

### Config Initialization

**File:** `internal/query/config.go:66`

For config-based tool loading:

```go
tool := NewChromemTool(rag, false)
```

---

## HCL Example

```hcl
documents "my_docs" "/path/to/documents" {
  storage_path = "./data/my_docs_db"
  description = "Project documentation"
  model = "mxbai-embed-large:latest"
}
```

### Full Configuration Example

```hcl
# Configuration file: .marvin.hcl

# LLM settings
model = "ministral-3:3b"

# Document repositories
documents "project_docs" "./docs" {
  storage_path = "./data/project_docs"
  description = "Project documentation and specs"
}

documents "api_docs" "/usr/local/share/doc/api" {
  storage_path = "./data/api_docs"
  description = "API reference documentation"
  model = "mxbai-embed-large:latest"
}
```

---

## Usage

### Indexing Documents

```bash
marvin rag index
```

Output:
```
Indexing 2 repositories
```

### Querying from CLI

```bash
marvin rag query project_docs "authentication"
```

Output shows matching documents with similarity scores.

### Querying via AI Agent

When using `marvin query`, the agent can use the `search` and `read_document` functions to access indexed documents:

```
rag> invoked chromem tool search
rag> invoked chromem tool read_document
```

---

## Dependencies

- **chromem-go** - Vector database (`github.com/philippgille/chromem-go`)
- **ollama** - Embedding generation via Ollama API
- **Ollama model** - `mxbai-embed-large:latest` (default)