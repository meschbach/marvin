## Context

**Current State:**

- Ollama: helper functions in `internal/config/ollama.go` (45 lines), used directly in query package
- OpenRouter: `internal/openrouter/` with client, chat, retry, tools, metrics, tracing files
- Gemini: `internal/gemini/` with client, chat files
- LLM interface (`conversation.LLM`) coupled to Ollama types (`api.ChatRequest`, `api.ChatResponse`)
- Factory in `internal/query/llm.go` returns a single provider via switch statement
- No model ordering, no health tracking, no cross-provider awareness
- OpenRouter has per-request retry (backoff) and OTel metrics

**Constraints:**

- Must maintain backward compatibility for legacy `provider` + `model` config
- Keep existing default model behavior (`ministral-3:3b` for Ollama)
- No breaking changes to CLI API or flags
- Circuit breaker state is in-memory only — restarts reset all health state

**Stakeholders:**

- CLI users (query, goal commands)
- Slacker users (multi-tenant Slack bot with model access controls)
- Future provider implementers

## Goals / Non-Goals

**Goals:**

- Unified LLM interface decoupled from provider-specific types
- Ordered model chain with per-model circuit breaker health tracking
- Structured `provider_model` HCL blocks matching vision's ProviderModel concept
- Backward-compatible config (legacy `provider` + `model` still works)
- OTel spans and metrics for chain-level observability

**Non-Goals:**

- Full Chain Engine with multiple strategies (this step is ordered-only)
- LLM Gateway as external process
- Dynamic config/runtime chain updates
- Restart persistence of circuit breaker cooldowns
- Context compression on model switch
- Adding new providers (design enables, implementation is separate)
- Per-model provider config overrides (e.g., different `base_url` per OpenRouter model) —
  `provider_model` blocks use the provider's top-level config block
- Streaming partial output on model failure — the chain may emit partial content from a
  failed provider before falling back. Requires a buffered streaming proxy to be fully
  addressed; deferred to a future Slack UX rework.
- LLM-aware model switch notification — `notify_model_switch` and system message injection
  deferred to a future agentic runtimes feature.
- RAG/embedding path changes — embeddings are a separate concern with their own interface
  and factory, but the existing RAG pipeline and `internal/config/ollama.go` encoder move
  are out of scope. The new `internal/llm/` package defines the `EmbeddingProvider`
  interface for future provider implementations.

## Decisions

### 1. Interface Design

Decision: Provider-agnostic LLM interface decoupled from Ollama types

```go
type LLM interface {
Chat(ctx context.Context, req *ChatRequest, onResponse func (ctx context.Context, resp ChatResponse) error) error
}
```

The current interface uses `api.ChatRequest` / `api.ChatResponse` from Ollama's SDK. We
define our own request/response types in `internal/llm/` to break this coupling.

`ChatRequest` uses a common-fields approach — fields that all major providers
(Ollama, OpenRouter, Gemini) support:

```go
type ChatRequest struct {
Messages  []Message
Tools     []ToolDefinition
Model     string // set by the chain or factory

// Common sampling parameters — supported by all current providers
Temperature *float32
TopK        *int
TopP        *float32
}
```

`ChatResponse` carries streaming chunks and the final done signal:

```go
type ChatResponse struct {
Content   string // streamed content chunk
Thinking  string // streamed reasoning (if supported)
ToolCalls []ToolCall  // tool invocation (on final chunk)
Done      bool        // true = final chunk with stats
Stats     Stats       // token usage (only on Done)
}
```

Streaming is folded into `Chat` via the `onResponse` callback parameter (matching the
existing pattern), not a separate method. The callback is called zero or more times
with incremental content chunks, then exactly once with `Done=true` for final stats.
This avoids needing a separate streaming interface for a single consumer pattern.

Alternative considered: Keep Ollama types as the common format (all providers translate)
→ Rejected: Leaky abstraction, forces every provider to understand Ollama's data model.

#### Stop Sequences as Model-Level Configuration

Stop sequences are a **model-level behavior**, not a per-request parameter. The tokenizer and
inference engine know the stop tokens for each specific model (e.g., `<|eot_id|>` for
Llama-3-Instruct, `<|end|>` for Gemma). These are baked into the model's weights and its
Modelfile / configuration, not determined at conversation time.

The unified `ChatRequest` intentionally omits `StopSequences` from its common fields. The
orphaned `convertStopSequences` function in `internal/gemini/chat.go` is a migration artifact
from the transition away from Ollama's `Options map[string]any` (which conflated model config
with request parameters). It will be removed as part of the cleanup phase.

Future model configuration work (e.g., `provider_model` blocks) may address model-level stop
sequence overrides, but per-request stop control is not a current capability. The interface
contract is: **conversation-scoped parameters only** (Messages, Tools, Temperature, TopK,
TopP).

#### Embedding as a Separate Concern

Embeddings are a different type of interaction — different lifecycle, different provider
capabilities. The `LLM` interface does NOT include embedding methods. Instead:

```go
type EmbeddingProvider interface {
Embeddings(ctx context.Context, req *EmbeddingRequest) ([]float32, error)
}
```

This is a separate interface with its own factory. Providers that support embeddings
(E.g., Ollama) implement both `LLM` and `EmbeddingProvider`. Providers that don't (E.g.,
OpenRouter, Gemini in this step) implement only `LLM`. Consumers (E.g., RAG system) call
the embedding factory directly — no type casting.

```go
// Separate factory, separate config, separate lifecycle
func NewEmbeddingProvider(ctx context.Context, cfg *config.File) (EmbeddingProvider, error)
```

### 2. Package Structure

```
internal/llm/
├── chat.go              # LLM interface, ChatRequest, ChatResponse, Message, ToolCall, ToolDefinition
├── ordered_chain.go     # OrderedChain — tries models in order, skips unhealthy
├── ramping_breaker.go   # Token-bucket readmission wrapping gobreaker[*ChatResponse]
├── factory.go           # NewFromConfig + NewEmbeddingProvider
├── embedding.go         # EmbeddingProvider interface + EmbeddingRequest
├── errors.go            # Structured error types with their methods
├── telemetry.go         # OTel tracer + metrics for llm package
├── ollama/
│   └── client.go        # Ollama provider (LLM + EmbeddingProvider)
├── openrouter/
│   ├── client.go        # OpenRouter provider (LLM)
│   ├── retry.go         # Per-request retry with backoff (existing, moved)
│   └── otel.go          # Provider-level metrics (existing, moved)
└── gemini/
    └── client.go        # Gemini provider (LLM)
```

### 3. Circuit Breaker Selection

Decision: Use `sony/gobreaker/v2` for per-model health tracking

- One `CircuitBreaker[*ChatResponse]` instance per `provider_model` entry
- `ReadyToTrip`: trip after 5 consecutive failures
- `Timeout`: 60s cooldown before half-open probe
- `OnStateChange`: emit OTel counter for circuit events
- On half-open success: model returns to rotation
- On half-open failure: re-opens with same timeout

Alternative considered: Hand-rolled HealthTracker with failure counts and cooldown timestamps
→ Rejected: gobreaker gives us the state machine (closed/open/half-open), thread safety,
and automatic recovery probing for ~40 lines of wiring.

Both `gobreaker` and `backoff` are used — they operate at different levels:

- `backoff`: per-request retry (short 429/5xx spikes within a single attempt)
- `gobreaker`: per-model health (skip failing models entirely, let them cool down)

#### Error Type Differentiation

Not all errors are equal for breaker purposes:

| Error Type                                   | Breaker Action         | Rate Limit Gate                | Backoff Action                          |
|----------------------------------------------|------------------------|--------------------------------|-----------------------------------------|
| 429 with `Retry-After` / `X-RateLimit-Reset` | Do NOT trip            | Set `rateLimitUntil` timestamp | Do not retry — wait until window passes |
| 429 without timing info                      | Do NOT trip            | —                              | Retry with exponential backoff          |
| 5xx / connection refused                     | Trip breaker after 5   | —                              | Retry with backoff                      |
| Timeout / deadline exceeded                  | Trip breaker after 5   | —                              | Retry with backoff                      |
| Context cancellation                         | Propagate (don't trip) | —                              | Do not retry                            |

Two independent throttles control access to each model:

- **Circuit breaker** (gobreaker): trips on *health* failures (5xx, timeout). Uses fixed 60s
  cooldown with half-open probe. Purpose: stop wasting requests on a genuinely broken endpoint.
- **Rate-limit gate** (RampingBreaker `rateLimitUntil`): activated on *load* failures (429 with
  timing info). Uses a dynamic timestamp set by the provider's response. No half-open probe
  needed — when the timestamp passes, the next request proceeds normally.
  Purpose: don't send traffic during a known congestion window.

429s tell us the model is functional but congested. Backing off solves it — there's nothing
wrong with the endpoint. The caller just needs to wait. Breaker trips are reserved for
errors that suggest the model endpoint is genuinely unhealthy (connection failures, 5xx,
persistent timeouts).

The provider must expose whether an error has rate-limit timing info so the gate can be set.

#### Behavior Consistency

Circuit breakers apply consistently regardless of chain length. For a single-entry
chain (legacy mode):

1. Error occurs → breaker records failure → if `ReadyToTrip` threshold met, breaker Opens
2. Next request → breaker is Open → `breaker.Execute` returns model-unavailable error
   → request fails with clear error message → after `Timeout`, half-open probe fires
3. Probe succeeds → breaker Closes, request proceeds normally
4. Probe fails → breaker re-Opens, model remains unavailable

Without a model-agnostic streaming notification mechanism, single-entry chains surface
breaker state through error responses rather than in-stream notifications. Multi-entry
chains silently skip open-breaker models and try the next entry.

#### Readmission Ramp (Slow-Start after Recovery)

**Problem:** When a breaker transitions half-open → closed, gobreaker immediately admits
all traffic to the freshly-recovered model. If the model's rate-limit or load capacity
is marginal, this thundering herd can overwhelm it and trigger another outage cycle:

```
t: Model A healthy
t+1s: Model A starts failing (5xx / timeouts)
t+61s: Breaker half-open → probe succeeds → closed
t+61s: ALL traffic shifts back to Model A simultaneously  ← herd
t+62s: Model A immediately overwhelmed → fails again      ← flap
t+122s: Same cycle repeats
```

**Decision:** Wrap gobreaker in a `RampingBreaker` that uses a **token-bucket slow-start**
inspired by TCP congestion control. After recovery, traffic is gradually re-admitted
doubling each round until reaching capacity.

```
States:
  CLOSED   (steady) — tokens = maxTokens (50), no ramp
  OPEN     — gobreaker handles this
  HALF-OPEN — probe → succeeds → RECOVERING

  RECOVERING:
    tokens = 1
    for each Execute():
      if acquireToken() → run request
      else → ErrNoToken (chain falls through to next model)
    on success, after request completes:
      tokens = min(tokens*2, maxTokens)
      if tokens == maxTokens → steady (CLOSED)
    on failure:
      tokens = 1 (reset)
      gobreaker → OPEN
```

Ramp-up timeline (assuming ~50ms per request round-trip):

| Round | Tokens | Concurrent served | Cumulative time to full capacity |
|-------|--------|-------------------|----------------------------------|
| 0     | 1      | 1                 | ~50ms                            |
| 1     | 2      | 2                 | ~100ms                           |
| 2     | 4      | 4                 | ~150ms                           |
| 3     | 8      | 8                 | ~200ms                           |
| 4     | 16     | 16                | ~250ms                           |
| 5     | 32     | 32                | ~300ms                           |
| 6     | 64     | cap at 50         | ~350ms                           |

~350ms to reach full capacity at doubling ramp. Tokens are replenished as requests
complete (one token per completed request), so the rate self-regulates: a slow model
re-admits slowly, a fast model ramps quickly.

**Zero-latency fallback:** When `ErrNoToken` is returned by `RampingBreaker`, the
OrderedChain immediately falls through to the next model — no blocking, no artificial
delay. Users only see a potential brief fallback during the ramp-up window.

**Single-entry chain mitigation:** For chains with a single model (no fallback),
`ErrNoToken` would fail the request. To avoid this during the ~350ms ramp, add a
brief non-blocking wait:

```go
func (r *RampingBreaker) acquireToken(ctx context.Context) bool {
if tryAcquireToken() {
return true
}
// Non-blocking wait for a token to free up
select {
case <-r.tokenAvailable:
return tryAcquireToken()
case <-time.After(50 * time.Millisecond):
return false // ErrNoToken
case <-ctx.Done():
return false
}
}
```

For multi-entry chains, the `tokenAvailable` channel isn't needed — the chain falls
through to the fallback model immediately. A future version can always enable the
brief wait for both cases to improve single-entry availability.

**Integration with gobreaker:**

```go
type RampingBreaker struct {
*gobreaker.CircuitBreaker[*ChatResponse]
maxTokens       int64
tokens          int64
state           int32 // 0=steady, 1=recovering
rateLimitUntil  atomic.Int64 // UnixNano; 0 = not rate-limited
tokenAvail      chan struct{} // signals token release (buffered)
}

func (r *RampingBreaker) Execute(ctx context.Context, req func (context.Context) (*ChatResponse, error)) (*ChatResponse, error) {
// Rate-limit gate: skip if within a known congestion window
if until := r.rateLimitUntil.Load(); until > 0 && until > time.Now().UnixNano() {
return nil, ErrRateLimited
}

// Recovery ramp: acquire token or fall through
if atomic.LoadInt32(&r.state) == recovering {
if !r.acquireToken(ctx) {
return nil, ErrNoToken
}
}

// Execute through gobreaker, but catch 429s before they count as failures
var capturedRateLimit bool
result, err := r.CircuitBreaker.Execute(func () (*ChatResponse, error) {
result, err := req(ctx)
if errors.Is(err, ErrRateLimitWithTiming) {
// 429 with timing — set the rate-limit gate, don't count as breaker failure
r.setRateLimitWindow(err)
capturedRateLimit = true
return nil, nil // gobreaker sees success
}
return result, err
})

if capturedRateLimit {
return nil, ErrRateLimited
}
r.admit(err)
return result, err
}

// setRateLimitWindow extracts timing from the error and sets rateLimitUntil.
func (r *RampingBreaker) setRateLimitWindow(err error) {
var rateLimitErr *RateLimitTimingError
if errors.As(err, &rateLimitErr) {
r.rateLimitUntil.Store(time.Now().Add(rateLimitErr.RetryAfter).UnixNano())
}
}

// OnStateChange from gobreaker seeds the recovery ramp:
OnStateChange: func (name string, from, to gobreaker.State) {
if from == StateHalfOpen && to == StateClosed {
atomic.StoreInt64(&r.tokens, 1)
atomic.StoreInt32(&r.state, recovering)
}
}
```

**Alternatives considered:**

| Approach                                         | Latency                      | Complexity | Flapping resilience | Why rejected                                                                                                                          |
|--------------------------------------------------|------------------------------|------------|---------------------|---------------------------------------------------------------------------------------------------------------------------------------|
| **Token bucket ramping (chosen)**                | None (fallback)              | ~80 lines  | Excellent           | —                                                                                                                                     |
| Fibonacci admission delay                        | High (blocks caller N*100ms) | ~40 lines  | Good                | Adds artificial latency to every request during ramp; 3s wait at Fib(8) is bad UX                                                     |
| Exponential backoff via existing `backoff` lib   | High (wasted delay)          | ~20 lines  | Good                | Reuses existing dep, but delays the caller rather than falling through; forces every request to wait even if fallback would be faster |
| Fixed leaky bucket (constant rate)               | Low                          | ~50 lines  | Fair                | Doesn't self-tune; must know the model's rate limit in advance                                                                        |
| Weighted probabilistic admission (N% to primary) | None                         | ~100 lines | Excellent           | More complex; requires random sampling per request; overkill for a recovery window measured in milliseconds                           |
| No ramp (gobreaker only — current behavior)      | None                         | 0 lines    | Poor                | Flapping cycle confirmed in production patterns                                                                                       |

Key differentiator of the chosen approach: **zero added latency**. The chain never waits
— it tries the next model immediately. Only single-entry chains use the brief 50ms
wait, and only during the ~350ms ramp window.

```go
type OrderedChain struct {
models      []modelEntry // ordered list of (label, LLM, breaker)
tracer      trace.Tracer
metrics     *chainMetrics
accessCheck func (label string) bool // nil = no filtering; userID captured at construction
}

type modelEntry struct {
label   string  // "claude-sonnet"
provider string // "openrouter"
llm     LLM     // the provider implementation
breaker *RampingBreaker // wraps gobreaker with token-bucket slow-start
}
```

**Lifecycle:**

1. `OrderedChain.Chat()` iterates `models` in order
2. For each entry: check `accessCheck` — if denied, skip silently to next
3. Apply per-model timeout: allocate `min(30s, remainingTime/remainingModels)` from parent
   context deadline, or use a default 30s timeout if no deadline is set
4. Call `entry.breaker.Execute(ctx, modelReq)` where `modelReq` wraps `entry.llm.Chat`:
    - **Rate-limited** (within known window): returns `ErrRateLimited` immediately
    - **Breaker OPEN**: returns model-unavailable error immediately
    - **RECOVERING** (token ramp): either acquires a token (proceed) or `ErrNoToken`
    - **CLOSED**: proceeds normally, request is sent to the provider
5. On success: return result. If RECOVERING, ramp doubles tokens.
6. On error from provider:
    - 429 with timing: `RampingBreaker` sets `rateLimitUntil`, returns `ErrRateLimited`.
      NOT counted as breaker failure. Chain falls through to next model.
    - 429 without timing: available for backoff retry at the provider level.
    - Health error (5xx, timeout): counted as breaker failure (5 → OPEN).
      If RECOVERING, ramp resets to token=1 and re-opens.
7. If all models exhausted: return `ErrAllModelsExhausted` with details

The per-model timeout prevents a hanging model from consuming the entire context deadline,
leaving insufficient time for fallback models. Timeout duration is recorded in the
`llm.chain.timeout` metric for operational visibility.

**Single-entry behavior:** A single-entry chain uses the same breaker machinery. On trip,
`breaker.Execute` returns a model-unavailable error. The request fails with a clear
error message. On half-open success, normal operation resumes. This is consistent with
multi-entry chains — difference is only that there's no alternative model to try.

#### Model Access Integration

The OrderedChain integrates model access control as a runtime check during model
iteration, not at build time. The chain carries an optional access-check function
with userID captured by the closure at chain-construction time:

```go
type OrderedChain struct {
models      []modelEntry
accessCheck func (label string) bool // nil = no filtering
...
}
```

```
Chain iteration:
  for each modelEntry:
    if accessCheck != nil && !accessCheck(entry.label):
      continue  // skip denied model silently
    if breaker is Open → model-unavailable error, skip to next
    try model → succeed or fail
```

This means:

- Each Slacker user gets their own chain instance with their access policy baked in
- Denied models are skipped at request time, not removed from the chain
- Admin users pass an accessCheck that always returns true
- If all models are denied for a given user, that request returns
  `ErrAllModelsExhausted` (per-request, not at startup)
- Access changes take effect immediately (no restart required if config reloaded)

### 5. Configuration Format

**Legacy mode** (backward compatible, no change):

```hcl
provider = "ollama"
model    = "ministral-3:3b"
```

**Structured mode** (new):

```hcl
provider_model "claude-sonnet" {
    provider = "openrouter"
    model    = "anthropic/claude-3.5-sonnet"
}

provider_model "gemma-4" {
    provider = "openrouter"
    model    = "google/gemma-4"
}

provider_model "local" {
    provider = "ollama"
    model    = "ministral-3:3b"
}

llm {
    models = ["claude-sonnet", "gemma-4", "local"]
}
```

**Detection:** `len(cfg.ProviderModels) > 0` → structured mode. The factory builds an
OrderedChain from the `provider_model` blocks. If `llm { models }` is present, that
defines the preference order. If absent, blocks are used in declaration order. An
empty `llm { models }` list is an error. In legacy mode (no `provider_model` blocks),
the factory builds a single-entry OrderedChain wrapping the legacy provider.

**Provider-level config:** `openrouter {}` and `gemini {}` blocks remain as-is for
provider-level settings (API keys, base URL, retry config). `provider_model` blocks
reference these by the `provider` field value.

### 6. Observability

**Tracing spans:**

```
OrderedChain.Chat  ← root span
  chain.strategy="ordered"
  chain.result="success" | "recovered" | "exhausted"
  models.total=3
  models.attempted=2
  chain.first_model="claude-sonnet"
  chain.final_model="local"
  └─ OrderedChain.tryModel  ← per actual attempt (not skipped)
       model.label="claude-sonnet"
       model.provider="openrouter"
       result="success" | "error"
       error.type="retry_exhausted"  (only on error)
       attempt.index=0
```

`chain.result` values:

- `success` — the first (preferred) model in the chain succeeded
- `recovered` — the first model failed, but an alternative model recovered (chain healed itself)
- `exhausted` — all models failed, no alternative succeeded

`tryModel.result` values:

- `success` — this model returned a valid response
- `error` — this model failed (will be recorded in breaker)

**Metrics:**

| Metric                    | Type      | Attributes                             | Purpose                                         |
|---------------------------|-----------|----------------------------------------|-------------------------------------------------|
| `llm.chain.result`        | counter   | `result` (success/recovered/exhausted) | Chain success and self-healing rate             |
| `llm.chain.latency`       | histogram | `result`                               | Total time including model switches             |
| `llm.chain.try_count`     | histogram | —                                      | Number of models actually attempted per request |
| `llm.chain.switch`        | counter   | `from_model`, `to_model`               | Which model switch paths trigger most           |
| `llm.chain.circuit_event` | counter   | `model`, `from` (state), `to` (state)  | Breaker trip/recover frequency                  |

Meter name: `"github.com/meschbach/marvin/llm/chain"`
Tracer name: `"github.com/meschbach/marvin/llm/chain"`

Provider-level metrics (per-request latency, errors, rate limit waits) remain in the
provider packages. Chain metrics are additive — they track orchestration, not individual
requests.

### 7. Factory User Threading

The factory creates the `OrderedChain` and optionally wires an access-check function
for runtime model filtering. The factory does NOT pre-filter at build time — it
produces a chain that can serve multiple users with different access levels.

```go
type Config struct {
File           *config.File
AccessCheck    func (label string) bool // nil = no filtering (CLI path)
}

func NewFromConfig(ctx context.Context, cfg Config) (LLM, error)
```

Using a config struct rather than positional params keeps the signature stable as
options evolve. Callers:

| Caller                       | AccessCheck                   | Behavior                                                                 |
|------------------------------|-------------------------------|--------------------------------------------------------------------------|
| CLI (`query.go`, `goal.go`)  | `nil`                         | No access filtering — all models usable                                  |
| Slacker (`query_handler.go`) | `ValidateModelAccess` closure | Captures userID at chain construction time, skips denied models per user |

The `AccessCheck` function is called once per model entry per request during chain
iteration. It takes only the model label — userID is captured by the closure at
chain-construction time:

```go
// Slacker per-user chain creation:
chain := llm.NewFromConfig(ctx, llm.Config{
File: cfg,
AccessCheck: func (label string) bool {
return cfg.ValidateModelAccess(label, currentUserID)
},
})
```

This means each Slacker user gets their own chain instance with their model access
baked in. Chain construction is cheap (no network calls), so this doesn't introduce
meaningful overhead. The underlying provider instances and breaker state are shared
through the `llm` and `breaker` fields of each `modelEntry`, which are created once
in the factory.

### 9. StreamingUpdater Migration

The `StreamingUpdater` interface currently uses Ollama SDK types directly:

```go
AddToolCall(ctx context.Context, toolCall api.ToolCall) error
AddToolResult(ctx context.Context, toolCall api.ToolCall, result []api.Message, err error) error
```

These MUST also migrate to the new internal types in `internal/llm/`. Strategy:

1. Define internal `ToolCall` and `Message` types in `internal/llm/chat.go`
   (structural 1:1 copies of the Ollama types for a mechanical first step)
2. Update `StreamingUpdater` to use `llm.ToolCall` and `llm.Message`
3. Update all updater implementations: `CLIStreamingUpdater`, `SlackUpdater`, `RecordingUpdater`
4. Update `conversation.Engine` to store `[]llm.Message` internally instead of `[]api.Message`
5. Update `conversation.Tool` interface to operate on `llm.ToolCall` / `llm.Message`
6. The `conversation` package converts between its internal types and `llm` types at its boundary

This prevents the class of defects seen in the OpenRouter implementation where
provider-specific types leaked into the shared streaming layer.

### 10. goal Command Decision

Decision: Keep and migrate `goal` through the unified path

`internal/query/goal.go` currently hardcodes Ollama (`newOllamaLLMFromEnv()`) and the
model name (`"ministral-3:3b"`). The value it provides is a two-phase reasoning pattern:

1. Produce a step-by-step plan using a reasoning-only toolset
2. Execute the plan with the full toolset

This pattern is architecturally distinct from the standard query path but does not
justify bypassing the LLM provider abstraction. `goal.go` will:

- Use the same `llm.NewFromConfig(cfg)` factory as everything else
- Pass through the chain (or single provider) normally
- Continue using its two-toolset pattern for the reasoning loop

The separate toolset pattern is orthogonal to provider selection and doesn't conflict
with the unified interface.

### 11. Migration Strategy

Decision: Build `internal/llm/` alongside existing packages, switch consumers one at a time

- Phase 1: Core interface + types + Ollama (no external impact)
- Phase 2: Config types (`provider_model`, `models`, `llm {}` block, backward compat)
- Phase 3: Conversation package migration (StreamingUpdater, Engine, Tool interfaces)
    - 3a: Define `llm.ToolCall` and `llm.Message` types
    - 3b: Update `StreamingUpdater` + all implementations
    - 3c: Update `Engine` internal storage to `[]llm.Message`
    - 3d: Update `Tool` interface + implementations
    - 3e: Update Ollama impl to produce `llm` types natively
- Phase 4: Refactor OpenRouter + Gemini under `internal/llm/`
- Phase 5: Circuit breaker + OrderedChain
- Phase 6: Wire factory (`NewFromConfig`, `NewEmbeddingProvider`), update consumers
- Phase 7: Observability spans and metrics
- Phase 8: Cleanup and verification

Each phase produces a working state. No long-lived feature branches.

## Risks / Trade-offs

- [Risk] Circuit breaker trip threshold too sensitive → [Mitigation] `ReadyToTrip` at 5 consecutive failures; tune if
  needed. 429s don't trip breakers.
- [Risk] gobreaker introduces new dependency → [Mitigation] Well-established (3.6k stars, Sony-maintained), same class
  as existing backoff lib
- [Risk] Config complexity for users with a single model → [Mitigation] Legacy mode requires zero config changes;
  breakers are invisible to users until they trigger
- [Risk] Single-model users experience model-unavailable errors on breaker trip → [Mitigation] Transient — 60s cooldown,
  then auto-recovery probe; brief 50ms non-blocking wait for in-flight requests during recovery ramp
- [Risk] Consumer migration coordination → [Mitigation] One consumer at a time, test each before moving on
- [Risk] StreamingUpdater type migration touches 21 files → [Mitigation] Split Phase 3 into 5 sub-phases (3a-3e), each
  independently testable
- [Risk] Partial streaming output on fallback → [Mitigation] Accepted as non-goal; deferred to Slack UX rework

## Open Questions

- How should provider-specific config (e.g., OpenRouter `base_url`) flow through `provider_model` blocks? If someone
  configures two OpenRouter models with different base URLs, the current design doesn't support it. (Non-goal for this
  step.)
- Should we expose circuit breaker `Timeout` and `ReadyToTrip` as config knobs or start with sensible defaults?
