### Marvin

Marvin is an agentic workflow CLI that connects an AI reasoning loop to Model Context Protocol (MCP) tools. It’s an experimental workbench intended to:

- Let you ask natural language queries and stream answers from a local LLM (via Ollama)
- Orchestrate tool-augmented reasoning using MCP-compatible tools (planned)

What Marvin is not:
- A replacement for LLMs
- A replacement for MCP tools
- A chat system like Open WebUI or AnythingLLM

> Status: Early prototype. The `query` command works against Ollama. MCP tool driving is in active development.

---

### Features (current and planned)

- Query local LLMs through Ollama and stream responses
- MCP tool invocation scaffold (`marvin mcp ...`) with roadmap to:
    - List MCP tools and capabilities
    - Invoke tools through model-directed function/tool calls
    - Stream intermediate steps for transparency

---

### Prerequisites

- Go 1.25+ (to build from source)
- Ollama running locally
    - Install: https://ollama.com
    - Default host: `http://127.0.0.1:11434`
    - Environment override supported by Ollama SDK: set `OLLAMA_HOST` if needed

> Models: The prototype targets a small model by default (`ministral-3:3b`). Adjust locally if you prefer another model.

---

### Install

Build from source:

```bash
# Clone
git clone https://github.com/meschbach/marvin.git
cd marvin

# Build binary into repository root
go build -o marvin ./cmd

# Optionally place on PATH
mv marvin /usr/local/bin/
```

Run without installing:

```bash
go run ./cmd --help
```

---

### Usage

- Free-form query to your local LLM via Ollama:

```bash
marvin query "Summarize the main differences between BFS and DFS."
```

What happens:
- Your prompt is sent to Ollama using the SDK’s Chat API
- Responses are streamed to your terminal
- If the model emits tool calls, Marvin will display them (MCP integration WIP)

---

### MCP (Model Context Protocol) integration

Marvin is designed to drive MCP tools, allowing models to:
- Discover available tools
- Decide when to call a tool (via function/tool-call messages)
- Provide arguments, receive outputs, and incorporate results into reasoning

Current CLI surface:

```bash
marvin mcp list
```

- `list` is scaffolded and will enumerate MCP providers/tools in upcoming releases
- Tool execution and session orchestration are under active development

---

### Configuration

- Ollama host: set `OLLAMA_HOST` (e.g., `export OLLAMA_HOST=http://localhost:11434`)
- Default model: currently hard-coded to `ministral-3:3b` in `internal/query/query.go`
    - You can change this in code for now; a `--model` flag is planned

---

### Development

Project layout (key parts):
- `cmd/` – CLI entrypoint and command wiring
- `internal/query/` – `query` command, Ollama chat invocation, basic tool-call surfacing

Run from source:

```bash
go run ./cmd query "hello, world"
```

Code style:
- Go modules
- `cobra` for commands
- `github.com/ollama/ollama/api` for chat streaming and tool call hooks

---

### Roadmap

- MCP provider discovery and `marvin mcp list`
- Execute MCP tools based on model-emitted function calls
- Session transcripts and step-by-step visibility
- Configurable models and system prompts
- Safer tool schemas and argument validation

---

### Troubleshooting

- Ollama not reachable:
    - Ensure the daemon is running: `ollama serve`
    - Check `OLLAMA_HOST` or default `127.0.0.1:11434`
- Model missing:
    - Pull the model you configured, e.g.: `ollama pull mistral` (or your preferred one)
- Empty output:
    - Smaller models sometimes respond tersely; try another model or adjust prompts

---

### License

Apache-2.0

---

### Acknowledgements

- Ollama for local model serving and a simple streaming API
- MCP ecosystem for standardizing tool interfaces