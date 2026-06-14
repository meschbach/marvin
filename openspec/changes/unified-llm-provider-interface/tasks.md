## Phase 1: Core Interface + Ollama Provider

- [x] 1.1 Create `internal/llm/chat.go` with LLM interface, ChatRequest (common
      fields: Messages, Tools, Temperature, TopK, TopP), ChatResponse, Message,
      ToolCall, ToolDefinition
- [x] 1.2 Create `internal/llm/errors.go` with structured error types and
      `IsRetryable() bool` / `IsPermanent() bool` methods
- [x] 1.3 Create `internal/llm/embedding.go` with EmbeddingProvider interface and
      EmbeddingRequest (separate from LLM interface)
- [x] 1.4 Create `internal/llm/ollama/client.go` implementing both LLM and
      EmbeddingProvider for Ollama (move embedding from `internal/config/ollama.go`)
- [x] 1.5 Verify Ollama impl works with existing conversation engine (temp import path)

## Phase 2: Configuration Types

- [x] 2.1 Add `ProviderModelBlock` struct in `internal/config/file.go`
- [x] 2.2 Add `ProviderModels []ProviderModelBlock`, `Models []string`, and `LLMBlock`
      fields to `File`
- [x] 2.3 Add backward-compat detection: `len(cfg.ProviderModels) == 0` → legacy mode
- [x] 2.4 Add config parsing tests for:
      - Legacy `provider` + `model` only
      - Multiple `provider_model` blocks with `models` list
      - Both legacy AND `provider_model` blocks (error)
      - Duplicate `provider_model` labels (error)
      - Unknown provider in `provider_model` (error)
      - `models` list referencing non-existent label (error)
      - Empty `models` list (error)
      - `provider_model` blocks without `llm { models }` (uses declaration order)

## Phase 3: Conversation Package Migration

### 3a: Define Internal Types

- [x] 3.1.1 Ensure `llm.ToolCall` and `llm.Message` are structurally complete 1:1
      equivalents of `api.ToolCall` / `api.Message` (mechanical migration step)
- [x] 3.1.2 Ensure `llm.ChatRequest` and `llm.ChatResponse` are defined and stable

### 3b: Update StreamingUpdater

- [x] 3.2.1 Update `StreamingUpdater` interface to use `llm.ToolCall` and `llm.Message`
      instead of `api.*` types
- [x] 3.2.2 Update `CLIStreamingUpdater` for new types
- [x] 3.2.3 Update `SlackUpdater` for new types
- [x] 3.2.4 Update `RecordingUpdater` for new types
- [x] 3.2.5 Run conversation streaming tests

### 3c: Update Engine

- [x] 3.3.1 Change `Engine.messages` from `[]api.Message` to `[]llm.Message`
- [x] 3.3.2 Update `buildChatRequest` to construct `*llm.ChatRequest`
- [x] 3.3.3 Update `executeTurn` response handling to produce `llm.Message`
- [x] 3.3.4 Update `RunConversation` signature (model string stays, request type changes)
- [x] 3.3.5 Update `conversation.Runner` to use `llm` types

### 3d: Update Tool Interface

- [x] 3.4.1 Update `conversation.Tool` interface to operate on `llm.ToolCall` / `llm.Message`
- [x] 3.4.2 Update `conversation.ToolSet` to use `llm` types
- [x] 3.4.3 Update all tool implementations in `internal/query/` (chromemTool, list, mark3labs, etc.)
- [x] 3.4.4 Update `conversation.ToolResponseMessage` to return `llm.Message`

### 3e: Update Ollama Provider to Produce llm Types

- [x] 3.5.1 Update Ollama provider to accept `*llm.ChatRequest` and return `*llm.ChatResponse`
- [x] 3.5.2 Remove dependency on `github.com/ollama/ollama/api` from the provider
- [x] 3.5.3 Run full conversation test suite

## Phase 4: Refactor OpenRouter and Gemini Providers

- [ ] 4.1 Move `internal/openrouter/openrouter.go` → `internal/llm/openrouter/client.go`
- [ ] 4.2 Move `internal/openrouter/chat.go` → `internal/llm/openrouter/chat.go`
- [ ] 4.3 Move `internal/openrouter/retry.go` → `internal/llm/openrouter/retry.go`
- [ ] 4.4 Move `internal/openrouter/otel.go` → `internal/llm/openrouter/otel.go`
- [ ] 4.5 Move `internal/openrouter/tools.go` → `internal/llm/openrouter/`
- [ ] 4.6 Update OpenRouter to implement new LLM interface (decoupled from Ollama types)
- [ ] 4.7 Move `internal/gemini/gemini.go` → `internal/llm/gemini/client.go`
- [ ] 4.8 Move `internal/gemini/chat.go` → `internal/llm/gemini/chat.go`
- [ ] 4.9 Update Gemini to implement new LLM interface
- [ ] 4.10 Run tests for both providers

## Phase 5: Circuit Breaker + OrderedChain

- [ ] 5.1 Add `sony/gobreaker/v2` dependency to go.mod
- [ ] 5.2 Create `internal/llm/breaker.go` — per-model circuit breaker factory with
      ReadyToTrip=5, Timeout=60s, OnStateChange wiring (429s do NOT trip breaker)
- [ ] 5.3 Create `internal/llm/ramping_breaker.go` — `RampingBreaker` wrapping gobreaker
      with:
      - Token-bucket slow-start: tokens=1 on recovery, doubles per round capped at 50,
        returns ErrNoToken when no token available, resets on any failure during recovery
      - Rate-limit gate: `rateLimitUntil` atomic timestamp; returns ErrRateLimited when
        active; catches 429-with-timing inside gobreaker.Execute to prevent counting as
        a breaker failure; automatically clears when timestamp passes
      - Wire `OnStateChange` to seed recovery state when half-open → closed
- [ ] 5.4 Create `internal/llm/ordered_chain.go` — OrderedChain iterating model list,
      skipping open breakers, falling through on ErrNoToken, calling Execute on each,
      with per-model timeout allocation (min 30s or remainingTime/remainingModels)
- [ ] 5.5 `OrderedChain` returns `ErrAllModelsExhausted` when all models tried and failed
- [ ] 5.6 Add error type propagation — `ErrRateLimited` (active gate, immediately fall through),
      `ErrNoToken` (recovery ramp full), health errors (counted toward breaker threshold),
      permanent errors (401, 404 — fail fast, don't count toward breaker)
- [ ] 5.7 Add `AccessCheck func(label string) bool` field to OrderedChain
      (runtime model access filtering via closure)
- [ ] 5.8 Add unit tests:
      - First model succeeds (single attempt)
      - First fails, second succeeds (recovery)
      - All models fail (exhaustion)
      - First model breaker Open → skips to second
      - Breaker half-open → succeeds (closes)
      - Breaker half-open → fails (re-opens)
      - 429 does NOT trip breaker
      - Context cancellation propagates (not treated as model failure)
      - Concurrent Chat() calls maintain consistent breaker state
      - Single-entry chain with breaker trip returns model-unavailable error
      - Empty chain (no models) returns clear error
      - AccessCheck skips denied model at runtime
      - Recovery ramp: tokens start at 1, double per round, cap at maxTokens
      - Recovery ramp: failure during ramp resets tokens to 1, re-opens breaker
      - ErrNoToken on multi-entry chain: silently falls through to next model
      - ErrNoToken on single-entry chain: brief non-blocking wait serves request

## Phase 6: Wire Factory and Consumers

- [ ] 6.1 Create `internal/llm/factory.go` with `NewFromConfig(ctx, Config) (LLM, error)`;
      Config includes File and AccessCheck func
- [ ] 6.2 Add `NewEmbeddingProvider(ctx, *config.File) (EmbeddingProvider, error)` to factory.go
- [ ] 6.3 Update `internal/query/query.go` — use new factory
- [ ] 6.4 Update `internal/query/goal.go` — use new factory (remove hardcoded Ollama)
- [ ] 6.5 Update `internal/slacker/query_streaming.go` — pass chain, wire AccessCheck closure
      with model validation at request time
- [ ] 6.6 Update `internal/slacker/query_handler.go` — pass config to QueryStreamer
- [ ] 6.7 Update `internal/rag/` — use `NewEmbeddingProvider` instead of direct
      `ollamaEncoder` construction (no type casting)
- [ ] 6.8 Update `cmd/marvin/llm.go` — no change needed (delegates to query package)
- [ ] 6.9 Remove old `internal/openrouter/` package directory
- [ ] 6.10 Remove old `internal/gemini/` package directory
- [ ] 6.11 Remove `internal/config/ollama.go` (moved to `internal/llm/ollama/embeddings.go`)

## Phase 7: Observability

- [ ] 7.1 Create `internal/llm/telemetry.go` — OTel tracer + meter + chain metric instruments
- [ ] 7.3 Add `OrderedChain.Chat` span with chain result, models total/attempted, model names
- [ ] 7.4 Add `OrderedChain.tryModel` child span per attempt with model, provider, result
- [ ] 7.5 Wire `llm.chain.result` counter, `llm.chain.latency` histogram,
      `llm.chain.try_count` histogram
- [ ] 7.6 Wire `llm.chain.switch` counter on model switch
- [ ] 7.7 Wire `llm.chain.circuit_event` counter from gobreaker OnStateChange
- [ ] 7.8 Wire `llm.chain.per_model_timeout` histogram tracking per-model timeout durations
- [ ] 7.9 Create `docs/agentic-systems/chain-observability-dashboard.json`

## Phase 8: Cleanup and Verification

- [ ] 8.1 Remove old `internal/query/llm.go` (functionality moved to `internal/llm/`)
- [ ] 8.2 Update `openspec/specs/llm-providers/spec.md` with new config format and health model
- [ ] 8.3 Run full test suite, fix regressions
- [ ] 8.4 Run pre-commit hooks: go fmt, go vet, golangci-lint, go build

## Test Cases Reference

### Chain-Level Tests

| Scenario | Behavior |
|----------|----------|
| First model succeeds | Returns immediately, try_count=1, result=success |
| First fails, second succeeds | Recovery activated, try_count=2, result=recovered |
| All models fail | ErrAllModelsExhausted, result=exhausted |
| Breaker Open on first model | Skips to second without attempting |
| Breaker half-open → succeeds | Returns to Closed, request succeeds |
| Breaker half-open → fails | Returns to Open, tries next model |
| Context cancelled mid-attempt | Propagates cancellation, NOT model failure |
| Concurrent Chat() calls | Consistent breaker state across goroutines |
| 429 with timing info | Sets rateLimitUntil, falls through to next model, breaker not tripped |
| 429 without timing info | Backoff retries, falls through to next model if exhausted, breaker not tripped |
| Request during rate-limit window | ErrRateLimited immediately (no network call), falls through |
| Rate-limit window expires naturally | Next request proceeds normally, no half-open probe needed |
| Empty chain | Clear error, not panic |
| Single-entry, breaker trips | Returns model-unavailable error after 50ms wait |
| Per-model timeout triggers | Skips to next model, records timeout metric |
| Recovery ramp: tokens start at 1, double per round, cap at 50 | Gradual re-admission, full capacity in ~350ms |
| Recovery ramp: any failure during ramp | Resets tokens to 1, breaker re-opens |
| ErrNoToken on multi-entry chain | Falls through to next model immediately |
| ErrNoToken on single-entry chain | 50ms non-blocking wait, then model-unavailable error |
| Concurrent requests during recovery | Tokens serialize correctly under contention |

### Model Access × Chain Tests

| Scenario | Behavior |
|----------|----------|
| All models allowed | Chain iterates all entries normally |
| Some models denied | AccessCheck skips denied model, tries next |
| All models denied for user | ErrAllModelsExhausted per-request |
| Admin user | AccessCheck always returns true, full chain |
| Mixed access levels across requests | Single chain serves both with different skip patterns |

### Config Parsing Tests

| Scenario | Behavior |
|----------|----------|
| Legacy provider + model only | Single-entry chain, legacy single-provider mode |
| Multiple provider_model + models list | Ordered chain |
| Duplicate provider_model labels | Error |
| Unknown provider in provider_model | Error |
| models references non-existent label | Error |
| Empty models list | Error — no models configured |
| provider_model without llm { models } | Use all provider_model blocks in declaration order |

### Provider Migration Tests

| Scenario | Behavior |
|----------|----------|
| Ollama Chat streaming | Full response with content + tool calls |
| OpenRouter Chat | Same, through new interface |
| Gemini Chat | Same |
| Embeddings via Ollama | Works via separate EmbeddingProvider interface |
| Error from provider | Wrapped in new error types with IsRetryable/IsPermanent |
| Timeout | Returns structured timeout error |
