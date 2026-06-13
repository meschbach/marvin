## MODIFIED Requirements

### Requirement: Provider Lifecycle (Changed from: Provider-Specific Implementation)

The system SHALL manage provider lifecycle through a common pattern. Currently, each provider (Ollama, OpenRouter,
Gemini) has its own structure and initialization. After this change, all providers implement the unified `LLM` interface
and are managed through the `llm` package's factory.

**Previous behavior**: Each provider had separate initialization in `internal/query/llm.go` via a switch statement.
Providers were wired directly with Ollama SDK types.

**New behavior**: All providers live under `internal/llm/<provider>/` and implement the `LLM` interface using internal
types. The factory in `internal/llm/factory.go` creates the appropriate implementation(s) from configuration.

#### Scenario: Provider as interface implementation

- **WHEN** a provider package is created under `internal/llm/<provider>/`
- **THEN** it implements the LLM interface (Chat + Embeddings)
- **AND** converts between internal `llm` types and its native API types

#### Scenario: Provider-level configuration

- **WHEN** a provider needs API keys, base URLs, or other config
- **THEN** its configuration block (e.g., `openrouter {}`, `gemini {}`) remains in the HCL config
- **AND** the factory reads these blocks and passes relevant config to the provider constructor

### Requirement: Circuit Breaker Health Tracking

The system SHALL track per-model health using circuit breakers for all entries in a chain.

**Previous behavior**: No health tracking. Every request went to the configured provider regardless of prior failures.

**New behavior**: Each model entry in a chain has an associated circuit breaker. Failures trip breakers; opened breakers
are skipped with a wait state; breakers auto-recover via half-open probes after a cooldown period.

#### Scenario: Breaker trips on health failure

- **WHEN** a model returns 5xx, timeout, or connection error
- **THEN** the breaker records a health failure
- **AND** if the failure threshold (5 consecutive) is met, the breaker transitions to Open state

#### Scenario: Rate limited with timing info sets gate

- **WHEN** a model returns 429 with `Retry-After` or `X-RateLimit-Reset`
- **THEN** the `RampingBreaker` sets `rateLimitUntil` to the specified reset timestamp
- **AND** the provider does NOT retry within the current request (the model needs to clear its backlog)
- **AND** the chain falls through to the next model for this request
- **AND** the circuit breaker is NOT tripped (rate limits are load signals, not health signals)

#### Scenario: Subsequent requests within rate-limit window

- **WHEN** a request arrives for a model whose `rateLimitUntil` timestamp is in the future
- **THEN** `RampingBreaker` returns `ErrRateLimited` immediately (no network call)
- **AND** the chain falls through to the next model
- **AND** the circuit breaker is NOT involved

#### Scenario: Rate limit window expires naturally

- **WHEN** the current time passes the model's `rateLimitUntil` timestamp
- **THEN** the next request proceeds normally (no half-open probe needed — the model was never unhealthy)
- **AND** the request succeeds or fails on its own merits

#### Scenario: Rate limited without timing info

- **WHEN** a model returns 429 with no rate-limit reset information
- **THEN** the provider may retry with exponential backoff
- **AND** if all retries are exhausted, the chain moves to the next model
- **AND** the circuit breaker is NOT tripped

#### Scenario: Open breaker skips to next model

- **WHEN** a breaker is Open/RECOVERING and a new request arrives
- **THEN** the chain receives `ErrNoToken` from the RampingBreaker
- **AND** falls through to the next model in the chain (for multi-entry)
- **OR** returns a model-unavailable error (for single-entry with expired wait)
