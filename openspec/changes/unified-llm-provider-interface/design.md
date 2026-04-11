## Context

**Current State:**
- Ollama: Just helper functions in `internal/config/ollama.go` (45 lines), used directly
- OpenRouter: Dedicated `internal/openrouter/` with multiple files (chat, tools, etc.)
- Gemini: Dedicated `internal/gemini/` with multiple files (chat, gemini, etc.)
- Each provider has inconsistent interfaces for chat, streaming, embeddings
- Consumers (conversation, query, slacker) import providers directly, making it hard to swap providers

**Constraints:**
- Must maintain backward compatibility for `provider = "ollama|openrouter|gemini"` config
- Keep existing model selection behavior (default: `ministral-3:3b`)
- Streaming, embeddings, tool calling must work for all providers post-migration
- No breaking changes to CLI API

**Stakeholders:**
- CLI users (query, goal commands)
- Slacker users (multi-tenant Slack bot)
- Future provider implementers

## Goals / Non-Goals

**Goals:**
- Unified interface for all LLM providers
- Consistent behavior for chat, streaming, embeddings
- Easy to add new providers (just implement interface)
- Simplified consumer code (import one package)
- Provider-specific configuration discovery from HCL

**Non-Goals:**
- Add new providers (design enables, implementation is separate)
- Change CLI commands or flags
- Migrate Slacker in same change (separate task)
- Runtime provider switching (config loads once at startup)

## Decisions

### 1. Interface Design

Decision: Single `LLM` interface with provider-specific options
```go
type LLM interface {
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    Stream(ctx context.Context, req *ChatRequest) (<-chan Chunk, error)
    Embeddings(ctx context.Context, req *EmbeddingRequest) ([]float32, error)
}
```

Alternative considered: Separate interfaces per capability (ChatLLM, StreamingLLM, EmbeddingLLM)
→ Rejected: Most providers implement all three, separate interfaces adds complexity

### 2. Package Structure

Decision: Flat `internal/llm/` with sub-packages per provider
```
internal/llm/
├── interface.go     # LLM interface definition
├── factory.go       # Provider factory from config
├── options.go       # Common options
├── ollama/
│   ├── client.go
│   └── embeddings.go
├── openrouter/
│   └── ...
└── gemini/
    └── ...
```

Alternative considered: Single file with all implementations
→ Rejected: Already have 3 providers, will grow - sub-packages more maintainable

### 3. Configuration Discovery

Decision: Factory takes `*config.File` and returns appropriate LLM
```go
func NewFromConfig(cfg *config.File) (LLM, error)
```

Alternative considered: Each provider has own config block
→ Rejected: Config is already in `config.File`, just need to route to right implementation

### 4. Migration Strategy

Decision: Create new `internal/llm/` package alongside existing, migrate consumers one at a time
- Step 1: Create llm package with interface and Ollama impl
- Step 2: Update conversation engine to use llm interface
- Step 3: Add OpenRouter, Gemini implementations  
- Step 4: Remove old packages after all consumers migrated

Alternative considered: Big bang migration
→ Rejected: Riskier, harder to test, more chance of breaking changes

## Risks / Trade-offs

- [Risk] Consumer migration complexity → [Mitigation] One consumer at a time, test each
- [Risk] Feature parity between providers → [Mitigation] Interface defines minimum viable, providers optional extended features
- [Risk] Performance regression → [Mitigation] Benchmark existing vs new, optimize if needed
- [Risk] Config breaking changes → [Mitigation] Keep provider string values same, just internal routing changes

## Open Questions

- Should we expose provider-specific options (Ollama host, OpenRouter base_url) in unified interface or via options?
- How to handle providers that don't support all features (e.g., some might not have embeddings)?
- Keep or remove `internal/config/ollama.go` helper after migration?