# Marvin

Marvin is a comprehensive AI agent platform that provides two ways to interact with Model Context Protocol (MCP) tools:

## 🎯 **Two Ways to Use Marvin**

### **1. Marvin CLI** - Direct Terminal Access
- Natural language queries with streaming responses from local LLM (via Ollama)
- Direct access to MCP-compatible tools
- Perfect for developers and power users

### **2. Slacker** - Multi-Tenant Slack Bot  
- Enterprise Slack integration with admin approval workflows
- Multi-user AI assistance with per-user session isolation
- Natural language tool management and sharing
- Ideal for team collaboration and controlled environments

## 🚀 **Platform Capabilities**

- **AI Reasoning Loop**: Connect to local LLMs with intelligent tool augmentation
- **MCP Tool Integration**: Extensive support for Model Context Protocol tools
- **Multi-Tenant Security**: Admin approval workflows and user isolation
- **Flexible Deployment**: CLI for development, Slack for collaboration

## What Marvin is not:
- A replacement for LLMs (it uses your existing LLMs)
- A replacement for MCP tools (it orchestrates them)
- A traditional chat system like Open WebUI or AnythingLLM
- A SaaS platform (it's self-hosted for privacy and control)

---

## 🛠 **Usage**

### **Marvin CLI - Quick Start**

Free-form queries with your local LLM via Ollama:

```bash
marvin query "Summarize the main differences between BFS and DFS."
```

**What happens:**
- Queries the default model (`ministral-3:3b`) on `ollama`
- Responses are streamed to your terminal
- Tool calls are displayed and handled automatically

**Example output:**
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

### **Slacker - Quick Start**

Deploy the multi-tenant Slack bot with admin approval workflows:

```bash
# Build and configure
go build -o slacker ./cmd/slacker
cp marvin.slacker.example.hcl marvin.slacker.hcl

# Set your Slack bot token
export SLACK_BOT_TOKEN=xoxb-your-bot-token-here

# Run the bot
./slacker --config marvin.slacker.hcl --passphrase "your-secure-passphrase"
```

**Slacker Features:**
- Natural language tool management in Slack
- Admin approval for security-sensitive tools
- Multi-user session isolation
- Encrypted credential storage
- Comprehensive audit logging

### **Use Case Comparison**

| Use Case | Marvin CLI | Slacker | When to Choose |
|----------|------------|---------|----------------|
| **Development & Testing** | ✅ Ideal | ⚠️ Possible | Local development, API testing |
| **Team Collaboration** | ❌ Limited | ✅ Ideal | Shared workspace, multiple users |
| **Automation & CI/CD** | ✅ Ideal | ⚠️ Possible | Scripted workflows, build pipelines |
| **Enterprise Deployment** | ⚠️ Individual | ✅ Ideal | Managed access, compliance needs |
| **Quick Queries** | ✅ Instant | ⚠️ Context switch | Personal use, fast iteration |

### **Configuration**

Both Marvin CLI and Slacker use flexible HCL configuration files to specify:
- LLM model and parameters
- MCP servers and tools  
- System prompts
- Multi-tenant settings (Slacker)

**Example Configurations:**
- [`marvin.example.hcl`](marvin.example.hcl) - CLI tool examples
- [`marvin.slacker.example.hcl`](marvin.slacker.example.hcl) - Slack bot setup

**Configuration Highlights:**
- **Multiple MCP transports**: Local programs, Docker containers, HTTP servers
- **Tool sharing**: Grant access to specific users or teams
- **Security controls**: Admin approval workflows for sensitive tools
- **Session persistence**: Maintain conversation history across restarts

---

## 🎨 **Display Preferences**

Both Marvin CLI and Slacker support configurable display preferences to control how responses are formatted.

### **CLI Display Flags**

Control output formatting directly from the command line:

```bash
# Show AI thinking process
marvin query "Explain quantum computing" --show-thinking

# Set thinking format (plain, markdown, collapsed)
marvin query "Help me debug this" --thinking-format markdown

# Show/hide tool invocation details
marvin query "Analyze this data" --show-tools
marvin query "Quick summary" --hide-tools

# Enable/disable completion messages
marvin query "Generate report" --show-done
marvin query "Simple answer" --hide-done

# Enable verbose debugging
marvin query "Debug this issue" --verbose
```

### **Slack Commands**

Manage your preferences in Slack using natural language:

```bash
# Control thinking display
@marvin thinking on           # Enable AI thinking process
@marvin thinking off          # Disable AI thinking process
@marvin thinking format markdown  # Set thinking format
@marvin thinking format plain     # Set thinking format
@marvin thinking format collapsed # Set thinking format

# Control tool display
@marvin tools on             # Show tool invocation details
@marvin tools off            # Hide tool invocation details

# Control completion messages
@marvin done on              # Show completion messages
@marvin done off             # Hide completion messages

# Verbose mode
@marvin verbose on           # Enable verbose debugging
@marvin verbose off          # Disable verbose debugging

# View current preferences
@marvin show preferences
```

### **Configuration File Display Settings**

Set default preferences in your HCL configuration:

```hcl
# Display preferences for output formatting
display {
  # Whether to show AI thinking process (default: false)
  show_thinking = true
  
  # Format for displaying thinking content: "plain", "markdown", or "collapsed" (default: "plain")
  thinking_format = "markdown"
  
  # Whether to show tool invocation details (default: true)
  show_tools = true
  
  # Format for displaying tool details: "simple" or "detailed" (default: "detailed")
  tool_format = "detailed"
  
  # Whether to show completion messages (default: true)
  show_done = true
  
  # Whether to enable verbose debugging output (default: false)
  verbose = false
}
```

### **Preference Resolution Priority**

Display preferences are resolved in this priority order:

1. **CLI flags** (highest priority - Marvin CLI only)
2. **User session preferences** (Slack - persistent per user)
3. **HCL display configuration** (file defaults)
4. **Hard-coded fallbacks** (lowest priority)

---

## Prerequisites

- Go 1.25+ (to build from source)
- Ollama running locally
    - Install: recommended via `brew install ollama`
    - Default host: `http://127.0.0.1:11434`
    - Environment override supported by Ollama SDK: set `OLLAMA_HOST` if needed

> Models: The prototype targets a small model by default (`ministral-3:3b`). Adjust locally if you prefer another model.

## 📦 **Install**

### **Option 1: Pre-built Binaries**
```bash
# Download and extract
curl -L https://github.com/meschbach/marvin/releases/latest/download/marvin_linux_amd64.tgz | tar xz
curl -L https://github.com/meschbach/marvin/releases/latest/download/slacker_linux_amd64.tgz | tar xz

# Make executable
chmod +x marvin slacker
```

### **Option 2: Build from Source**
```bash
# Clone repository
git clone https://github.com/meschbach/marvin.git
cd marvin

# Build both components
go build -o marvin ./cmd/marvin
go build -o slacker ./cmd/slacker
```

### **Option 3: Docker**
```bash
# Marvin CLI
docker run --rm -it ghcr.io/meschbach/marvin:latest

# Slacker (with volume for persistence)
docker run --rm -v $(pwd)/config:/config \
  -e SLACK_BOT_TOKEN=xoxb-your-token \
  ghcr.io/meschbach/marvin/slacker:latest
```

### **Prerequisites**
- **Ollama** running locally (for CLI) or accessible (for Slacker)
- **Slack App** with Bot User OAuth Token (for Slacker)
- **Go 1.25+** (when building from source)

### **Project Structure**
```
marvin/
├── cmd/                    # CLI entrypoints
│   ├── marvin/            # Main CLI application
│   └── slacker/           # Slack bot application
├── internal/              # Core functionality
│   ├── config/            # HCL configuration parsing
│   ├── query/             # LLM interaction and MCP tools
│   └── slacker/           # Multi-tenant Slack integration
├── docs/                  # Comprehensive documentation
├── examples/              # Configuration examples
└── deploy/                # Kubernetes deployments
```

**Run from source:**
```bash
# CLI
go run ./cmd/marvin query "hello, world"

# Slacker  
go run ./cmd/slacker --config marvin.slacker.example.hcl --passphrase "test"
```

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

## Development

### Code Organization

This project follows specific code organization standards to maintain quality and readability:

**File Size Guidelines**:
- Target: <200 lines per file (ideal)
- Maximum: <400 lines per file
- Structs kept with their methods (Go conventions)

**Functional Grouping**:
- Related functionality grouped in focused files
- Interface boundaries where logical separation exists
- Clear single responsibility for each file

For detailed guidelines, see:
- [`AGENTS.md`](AGENTS.md) - Agent development guidelines
- [`docs/REFACTORING.md`](docs/REFACTORING.md) - Code quality refactoring decisions

### Contributing

When contributing:
1. Follow the established code organization patterns
2. Keep files under 400 lines (ideally under 200)
3. Maintain functional cohesion within files
4. Test thoroughly after refactoring

### Acknowledgements

- Ollama for local model serving and a simple streaming API
- MCP ecosystem for standardizing tool interfaces
