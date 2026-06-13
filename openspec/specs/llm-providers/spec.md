# LLM Providers Specification

## Overview

Marvin supports multiple LLM (Large Language Model) providers, allowing users to choose the backend that best fits their needs. Providers are configured in HCL configuration files using `provider_model` blocks or legacy `provider` + `model` fields.

### Supported Providers

| Provider | Default Model | Description |
|----------|---------------|-------------|
| Ollama | `ministral-3:3b` | Local LLM server (default) |
| OpenRouter | (user-specified) | Multi-provider API aggregation |
| Google Gemini | (user-specified) | Google's Gemini models |

### Default Models

- **DefaultLanguageModel**: `ministral-3:3b` (defined at `internal/config/file.go:15`)
- **DefaultEmbeddingModel**: `mxbai-embed-large:latest` (defined at `internal/config/file.go:16`)

---

## Configuration Modes

Marvin supports two configuration modes. Legacy mode is fully backward compatible.
Structured mode enables fallback, health tracking, and multi-model chains.

### Legacy Mode (Backward Compatible)

Single provider selected via `provider` field:

```hcl
provider = "ollama"
model    = "ministral-3:3b"
```

When `provider` is not specified, Marvin defaults to **Ollama**.

### Structured Mode

Uses `provider_model` blocks to define `(model, provider)` pairs, with an optional
`fallback` list for ordered chain behavior:

```hcl
provider_model "primary" {
    provider = "openrouter"
    model    = "anthropic/claude-3.5-sonnet"
}

provider_model "backup" {
    provider = "ollama"
    model    = "ministral-3:3b"
}

llm {
    fallback           = ["primary", "backup"]
}
```

**Detection:** The factory checks `len(cfg.ProviderModels) > 0`. If present, structured
mode is used and `fallback` controls the ordered chain. If absent, legacy mode applies
with a single implicit entry.

*Note: `notify_model_switch` was considered but deferred — this change does not include
LLM-aware model switch notifications. The chain silently falls back to the next model.*

---

## Ordered Chain (Fallback)

When multiple `provider_model` entries are configured with a `fallback` list, Marvin
uses an **OrderedChain** to try models in declaration order, skipping unhealthy ones.

### Selection Logic

```
Request → OrderedChain.Select()
           ├── Apply per-model timeout (min 30s or remainingTime/remainingModels)
           ├── Iterate models in order
           │    ├── Check model access (runtime AccessCheck function)
           │    │    └── Denied → skip silently to next
           │    ├── Check RampingBreaker gates (fast path, no network call)
           │    │    ├── Rate-limited (now < rateLimitUntil) → ErrRateLimited → skip to next
           │    │    ├── OPEN → skip to next
           │    │    ├── RECOVERING → try acquire token
           │    │    │    ├── Token acquired → proceed
           │    │    │    └── ErrNoToken → fall through to next model
           │    │    └── CLOSED → proceed
            │    ├── Call provider LLM
            │    │    ├── 429 with timing → set rateLimitUntil → next model
            │    │    │    (caught before backoff — model needs to clear backlog)
            │    │    ├── 429 without timing → backoff retry → next model if exhausted
            │    │    │    (breaker NOT tripped — load signal, not health signal)
            │    │    ├── 5xx/timeout → backoff retry
            │    │    │    └── exhausted → report failure to breaker
            │    │    │         ├── consecutive > 5? → OPEN
            │    │    │         └── else → continue to next model
            │    │    │         (if RECOVERING, reset tokens to 1)
            │    │    └── Success → ramp if RECOVERING (tokens *= 2, cap at 50), return
            └── All models exhausted → return ErrAllModelsExhausted
```

### Circuit Breaker Health Model

Each `provider_model` entry has an associated circuit breaker (`sony/gobreaker/v2`)
tracking its health state, wrapped in a `RampingBreaker` for controlled readmission:

| State | Behavior | Recovery |
|-------|----------|----------|
| **CLOSED** | Normal operation. Requests pass through. | — |
| **OPEN** | Fail-fast. Requests are skipped without attempting. | 60s timeout → HALF-OPEN |
| **HALF-OPEN** | One probe request allowed. | Success → RECOVERING. Failure → OPEN (60s again). |
| **RECOVERING** | Token-bucket slow-start. 1 token initially, doubles per round, capped at 50. | Success at 50 tokens → CLOSED. Any failure → OPEN (tokens reset to 1). |
| **RATE-LIMITED** | Not a breaker state — a timestamp gate on RampingBreaker. Active when a 429 with timing info was received. | Automatic when `rateLimitUntil` timestamp passes. No half-open probe needed. |

- **Trip threshold:** 5 consecutive failures (configurable via `ReadyToTrip`)
- **Cooldown:** 60 seconds before automatic recovery probe
- **Readmission ramp:** After half-open probe succeeds, traffic is gradually re-admitted
  at a doubling rate (1, 2, 4, 8, ... 50). Full capacity reached in ~350ms.
- **ErrNoToken:** When no token is available during recovery, `RampingBreaker` returns
  `ErrNoToken`. Multi-entry chains fall through to the next model immediately (zero
  latency). Single-entry chains attempt a brief 50ms non-blocking wait.
- **Failure during ramp:** Any failure resets tokens to 1 and re-opens the breaker.
- **Scope:** In-memory only. Restart resets all breaker state.

### Per-Request Retry (Provider-Level)

Before the circuit breaker records a failure, the provider's internal retry logic
(`cenkalti/backoff/v5`) handles transient errors:

- **429 without timing info:** Exponential backoff up to `max_attempts` (default: 3).
  The provider may retry since the model is busy but may accept a brief gap.
- **429 with timing info (`Retry-After` / `X-RateLimit-Reset`):** No backoff retry.
  The provider returns `ErrRateLimitWithTiming` immediately — the `RampingBreaker`
  sets `rateLimitUntil` and the chain falls through to the next model.
- **5xx (Server Error):** Exponential backoff up to `max_attempts` (default: 3)
- **Only on exhaustion:** The circuit breaker (for health errors) records the failure
  and advances the chain. 429s are never counted toward the breaker.

The two mechanisms are complementary:
- `backoff` handles short spikes (429 without timing, brief 5xx)
- `gobreaker` handles sustained failure (model is down, skip it for a while)

---

## Ollama Provider

Ollama is the default and most commonly used provider. It runs locally and connects to a local Ollama server.

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `host` | string | No | Ollama server URL (default: `http://localhost:11434`) |
| `model` | string | No | Model name (default: `ministral-3:3b`) |

### HCL Configuration Example

```hcl
# Using default local Ollama
llm {
  model = "llama3.2:latest"
  host = "http://localhost:11434"
}

# With model options
llm {
  model = "llama3.2:latest"
  host = "http://localhost:11434"

  options {
    temperature = 0.7
    top_p = 0.9
    num_predict = 512
  }
}
```

### Implementation

**Package**: `internal/llm/ollama/`

The Ollama provider implements the unified `LLM` interface:

- `client.go` — Chat method and client setup
- `embeddings.go` — Embeddings method (moved from `internal/config/ollama.go`)

---

## OpenRouter Provider

OpenRouter provides access to multiple LLM providers through a unified API.

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `api_key` | string | Yes | OpenRouter API key |
| `base_url` | string | No | Custom endpoint (default: `https://openrouter.ai/api/v1`) |
| `retry` | block | No | Retry configuration (see Retry Configuration) |

### Retry Configuration

The `retry` block configures retry behavior for rate limits and server errors.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `max_attempts` | int | 3 | Maximum retry attempts |
| `initial_interval` | duration | 1s | Initial backoff interval |
| `max_interval` | duration | 30s | Maximum backoff interval |

### HCL Configuration Example

```hcl
provider = "openrouter"

openrouter {
  api_key = "sk-or-..."
  base_url = "https://openrouter.ai/api/v1"  # optional

  retry {
    max_attempts = 5
    initial_interval = "1s"
    max_interval = "30s"
  }
}

llm {
  model = "anthropic/claude-3.5-sonnet"
}
```

### Retry Behavior

The OpenRouter provider implements automatic retry with exponential backoff:

- **429 (Rate Limited)**: Automatically retried with exponential backoff
- **5xx (Server Error)**: Automatically retried with exponential backoff
- **Retry-After**: OpenRouter does not provide `Retry-After` headers, so exponential backoff is used
- **Max Attempts**: Configurable via `max_attempts` (default: 3)
- **Backoff**: Exponential starting at `initial_interval`, capped at `max_interval`

### Implementation

**Package**: `internal/llm/openrouter/`

Key files:

| File | Description |
|------|-------------|
| `client.go` | Main LLM wrapper implementing unified interface |
| `chat.go` | Chat method with streaming response handling |
| `retry.go` | Per-request retry with backoff and rate-limit-reset handling |
| `tools.go` | Tool call conversion (OpenRouter function calling format) |
| `otel.go` | Provider-level OTel metrics (latency, errors, rate limit waits) |

The implementation:
- Uses `github.com/revrost/go-openrouter` library
- Configures OpenTelemetry instrumentation for observability
- Sets custom HTTP headers (`HttpReferer`, `XTitle`) for API tracking

### Environment Variable

- `OPENROUTER_API_KEY` - API key source (see `internal/config/file.go:47-52`)

---

## Google Gemini Provider

Google Gemini provides access to Google's Gemini models.

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `api_key` | string | Yes | Google AI API key |

### HCL Configuration Example

```hcl
provider = "gemini"

gemini {
  api_key = "AIza..."
}

llm {
  model = "gemini-2.0-flash"
}
```

### Implementation

**Package**: `internal/llm/gemini/`

Key files:

| File | Description |
|------|-------------|
| `client.go` | Main LLM wrapper implementing unified interface |
| `chat.go` | Chat method with Google GenAI streaming integration |

The implementation:
- Uses `google.golang.org/genai` library
- Provides streaming response support via iterators
- Adapts Google GenAI client to Marvin's conversation interface

### Environment Variable

- `GEMINI_API_KEY` - API key source (see `internal/config/file.go:32-38`)

---

## Model Options

The `options` block in HCL configuration provides fine-grained control over model behavior.

### Configuration Options

**File**: `internal/config/file.go:55-75`

| Option | Type | Range | Description |
|--------|------|-------|-------------|
| `context_window_size` | int | >0 | Context window size in tokens (maps to `num_ctx`) |
| `temperature` | float32 | 0.0-1.0 | Sampling temperature (higher = more creative) |
| `top_p` | float32 | 0.0-1.0 | Nucleus sampling parameter |
| `top_k` | int | ≥-1 | Top-k sampling (-1 = no limit) |
| `num_predict` | int | ≥-1 | Maximum tokens to predict (-1 = unlimited) |
| `repeat_penalty` | float32 | any | Repetition penalty |
| `repeat_last_n` | int | ≥-1 | Lookback for repetitions (-1 = context_size) |
| `seed` | int | any | Random seed for reproducibility |
| `stop` | []string | N/A | Stop sequences |

### Example with Options

```hcl
llm {
  model = "ministral-3:3b"

  options {
    temperature = 0.8
    top_p = 0.95
    top_k = 40
    num_predict = 1024
    repeat_penalty = 1.1
    stop = ["END", "STOP"]
  }
}
```

---

## Package Structure

All provider implementations live under `internal/llm/` to share a common interface
and enable chain orchestration:

```
internal/llm/
├── interface.go         # LLM interface, ChatRequest, ChatResponse
├── types.go             # Shared types: Message, ToolCall, ToolDefinition
├── options.go           # Common options struct for ChatRequest
├── embedding.go         # EmbeddingProvider interface + EmbeddingRequest (separate from LLM)
├── embedding_factory.go # NewEmbeddingProvider — separate factory for embeddings
├── ordered_chain.go     # OrderedChain — ordered fallback with circuit breaker health
├── breaker.go           # Circuit breaker setup per model (sony/gobreaker)
├── factory.go           # NewFromConfig — builds LLM from config
├── errors.go            # Structured error types
├── tracer.go            # OTel tracer for chain-level spans
├── metrics.go           # OTel meter for chain-level metrics
├── ollama/
│   ├── client.go        # Ollama provider implementing LLM interface
│   └── embeddings.go    # Embeddings (implements EmbeddingProvider)
├── openrouter/
│   ├── client.go
│   ├── chat.go
│   ├── retry.go
│   ├── tools.go
│   └── otel.go
└── gemini/
    ├── client.go
    └── chat.go
```

## Observability

### Tracing

The OrderedChain emits OTel spans for chain-level orchestration:

```
OrderedChain.Chat  ← root span
  chain.strategy="ordered"
  chain.outcome="success" | "exhausted"
  models.total=3
  models.attempted=2
  └─ OrderedChain.tryModel  ← per actual attempt
       model.label="claude-sonnet"
       model.provider="openrouter"
       outcome="success" | "error"
```

### Metrics

| Metric | Type | Attributes | Purpose |
|--------|------|------------|---------|
| `llm.chain.outcome` | counter | `outcome` | Chain success rate |
| `llm.chain.latency` | histogram | `outcome` | Total time with fallbacks |
| `llm.chain.models_attempted` | histogram | — | Fallback depth |
| `llm.chain.model_switch` | counter | `from_model`, `to_model` | Fallback trigger frequency |
| `llm.chain.circuit_event` | counter | `model`, `from`, `to` | Breaker trip/recover events |

Provider-level metrics (per-request latency, errors, rate limit waits) live in each
provider package. Chain metrics are additive — they track orchestration only.

## Configuration File Reference

### Key Configuration Types

| Type | Location | Description |
|------|----------|-------------|
| `ProviderType` | `internal/config/file.go:18-25` | Provider type constants |
| `GeminiBlock` | `internal/config/file.go:27-38` | Gemini configuration |
| `OpenRouterBlock` | `internal/config/file.go:40-53` | OpenRouter configuration |
| `RetryBlock` | `internal/config/file.go:62-67` | Retry parameters |
| `ProviderModelBlock` | `internal/config/file.go` | Model+provider pair definition |
| `ModelOptionsBlock` | `internal/config/file.go:136-155` | Model behavior options |

### Provider Resolution (Legacy Mode)

```go
func (f *File) Provider() ProviderType {
    if f.ProviderName == "" {
        return ProviderOllama
    }
    return ProviderType(f.ProviderName)
}
```

### Structured Mode Detection

```go
func (f *File) IsStructuredMode() bool {
    return len(f.ProviderModels) > 0
}
```

---

## Usage Notes

1. **Ollama is the default** - No provider configuration needed for local Ollama
2. **API keys required** - OpenRouter and Gemini require API keys via `api_key` block or environment variables
3. **Model selection** - Each provider accepts different model names; refer to provider documentation
4. **Options are provider-specific** - Not all options work with all providers; Ollama-specific options may be ignored by others
5. **Embeddings are a separate concern** - The `EmbeddingProvider` interface is distinct from `LLM` with its own factory. Existing RAG pipeline is not migrated in this change.
6. **Fallback requires structured mode** - Use `provider_model` blocks + `fallback` list to enable automatic fallback between models/providers
7. **Health state is in-memory** - Circuit breaker cooldowns reset on process restart; no persistence
8. **Single model in chain** - A `fallback` list with one entry is equivalent to legacy mode, still gains circuit breaker protection
9. **Per-model timeouts** - The chain allocates `min(30s, remainingTime/remainingModels)` per attempt to prevent a hanging model from starving fallbacks
10. **Model access is runtime-checked** - The chain carries an `AccessCheck` function that filters models per-request; denied models are skipped silently