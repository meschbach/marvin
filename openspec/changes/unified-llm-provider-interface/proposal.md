## Why

The current codebase has inconsistent provider abstraction - Ollama is treated as a special case (just helper functions in `internal/config/`) while OpenRouter and Gemini each have dedicated packages. This makes it harder to add new providers, maintain consistent interfaces, and share common functionality like streaming, embeddings, and tool calling.

## What Changes

- Create a unified `internal/llm/` package with a common interface
- Move Ollama implementation from config helpers to proper package (`internal/llm/ollama/`)
- Restructure OpenRouter (`internal/llm/openrouter/`) and Gemini (`internal/llm/gemini/`) under unified interface
- Standardize provider behaviors: chat, streaming, embeddings
- Add provider configuration discovery (vs hardcoded in each package)
- Update all consumers (conversation, query, slacker) to use unified interface

**Breaking Changes:**
- Replace direct imports of `internal/openrouter/`, `internal/gemini/` with `internal/llm/`
- Config field `provider` values remain compatible but internal structure changes
- Some internal types moved/reorganized

## Capabilities

### New Capabilities
- `llm-interface`: Unified LLM interface defining chat, stream, embeddings contract
- `llm-ollama`: Ollama provider implementation with full feature parity
- `llm-openrouter`: Refactored OpenRouter provider under unified interface
- `llm-gemini`: Refactored Gemini provider under unified interface
- `llm-config-discovery`: Dynamic provider configuration from HCL

### Modified Capabilities
- `llm-providers`: Requirements change - all providers now implement unified interface

## Impact

- `internal/conversation/` - Uses LLM interface, updated imports
- `internal/query/` - Tool loading uses unified provider
- `internal/slacker/query_streaming.go` - Uses unified interface
- `cmd/marvin/` - No change (uses query package)
- Configuration parsing (`internal/config/`) - Simplified, delegates to llm package