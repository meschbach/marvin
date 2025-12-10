# Marvin

Marvin is an agentic workflow CLI that connects an AI reasoning loop to Model Context Protocol (MCP) tools. It’s an
experimental workbench intended to:

- Let you ask natural language queries and stream answers from a local LLM (via Ollama)
- Use MCP-compatible tools to augment reasoning
- Explore the capabilities of MCP-compatible tools

## What Marvin is not:
- A replacement for LLMs
- A replacement for MCP tools
- A chat system like Open WebUI or AnythingLLM

## Future
These features are nice to have in the future:
- Orchestrate tool-augmented reasoning using MCP-compatible tools (planned)

---

## Usage

- Free-form query to your local LLM via Ollama:

```bash
marvin query "Summarize the main differences between BFS and DFS."
```

What happens:
- Queries the default model (`ministral-3:3b`) on `ollama`
- Responses are streamed to your terminal
- If the model emits tool calls, Marvin will display them

Example output:
>Query:  Summarize the main differences between BFS and DFS.
>Here are the **key differences** between **Breadth-First Search (BFS)** and **Depth-First Search (DFS)**:
>
>| Feature               | **Breadth-First Search (BFS)**                          | **Depth-First Search (DFS)**                          |
>|-----------------------|-------------------------------------------------------|-------------------------------------------------------|
>| **Approach**          | Explores **level by level** (visits all nodes at depth *d* before moving to depth *d+1*). | Explores **as far as possible** along a branch before backtracking. |
>| **Data Structure**    | Uses a **queue** (FIFO) to track nodes.               | Uses a **stack** (LIFO) or recursion to track nodes.   |
>| **Time Complexity**   | **O(B + D)** (where *B* = branching factor, *D* = depth). | **O(B + D)** (worst case, but often less due to backtracking). |
>- **BFS** is better for **shortest path** in unweighted graphs.
>- **DFS** is better for **deep exploration** (e.g., finding a path in a maze) and **cycle detection**.
>- Both have **O(B + D)** time complexity, but DFS may use less space if the graph is **sparse** (many branches but shallow depth).

### Configuration
Optionally, by passing `-c <file>` or `--config <file>` you can load a configuration file.  You can specify:
- MCP servers
- System Prompt

For an example see [`marvin.example.yaml`](marvin.example.hcl).

---

## Prerequisites

- Go 1.25+ (to build from source)
- Ollama running locally
    - Install: recommended via `brew install ollama`
    - Default host: `http://127.0.0.1:11434`
    - Environment override supported by Ollama SDK: set `OLLAMA_HOST` if needed

> Models: The prototype targets a small model by default (`ministral-3:3b`). Adjust locally if you prefer another model.

## Install
Marvin is a single binary and only requires access to Ollama.  Using MCP servers only require the exectuable to be 
accessible in the Marvin runtime.

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
