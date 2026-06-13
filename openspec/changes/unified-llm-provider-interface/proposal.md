## Why

When the configured LLM model is unavailable (OpenRouter rate limit, model overloaded, provider
outage), the entire agent request fails. There is no alternative model. This is the most frequent
operational pain point — agents that should be autonomous become fragile dependencies on a
single API endpoint.

To fix this, we need two things:
1. A way to configure multiple models/providers in order of preference
2. Health awareness so the system stops trying known-bad models

The provider code also needs structural unification as a prerequisite — today each provider is
wired independently, making it impossible to compose or swap them without modifying the
provider switch statement.

## What Changes

**For config authors:**
- New `provider_model` blocks define `(model, provider)` pairs with a readable label
- Optional `models` list on the `llm` block declares model preference order
- Existing `provider` + `model` config continues working unchanged

**For agent reliability:**
- Agents try models in declared order, skipping any that have been failing recently
- A model that fails repeatedly is automatically quarantined (~60s cooldown) before being
  retried — prevents hammering a downed endpoint
- When all models are exhausted, the agent surfaces a clear error instead of hanging

**For operators:**
- New OTel metrics: chain success rate, try count, model switch events, circuit breaker
  transitions
- New OTel spans: per-attempt trace showing which model was tried and what happened
- `llm.chain.result` tells you how often your agents survive provider outages vs exhausting
  all options

**Deliverables:**
- Grafana dashboard JSON (`docs/agentic-systems/chain-observability-dashboard.json`) with 8
  panels: chain success rate, latency (P50/P90/P99), try count heatmap, model switch
  events, circuit breaker transitions, provider errors, request rate, rate limit wait times

**Breaking changes:**
- No config-breaking changes for single-provider users
- Provider packages physically move — only internal consumers (not config files) are affected
- The `internal/config/ollama.go` embedding helper relocates

## Who's Affected

| Audience                  | Impact                                                                                                |
|---------------------------|-------------------------------------------------------------------------------------------------------|
| **CLI users**             | No interface change; agents become more reliable automatically when alternative models are configured |
| **Slacker users**         | No interface change; same reliability benefit                                                         |
| **Config authors**        | New `provider_model` and `models` fields available; existing configs work as-is                       |
| **Provider implementers** | Must implement `internal/llm` interface instead of being wired manually in a switch statement         |
| **Operators**             | New chain-level OTel metrics and spans for monitoring model health and chain behavior                 |

## Capabilities

### New Capabilities
- **Ordered model chains** — Agents can be configured with an ordered list of models; if the
  primary is unavailable or unhealthy, the next model is tried automatically
- **Health-aware model selection** — Models that fail repeatedly are quarantined temporarily,
  preventing wasted requests to known-bad endpoints
- **Structured model configuration** — `provider_model` HCL blocks with labeled `(provider,
  model)` pairs, matching the target architecture's ProviderModel concept
- **Chain observability** — OTel spans and metrics track try count, model switches, and
  circuit breaker events

### Modified Capabilities
- **Provider configuration** — Extended to support structured `provider_model` blocks
- **Model access control** — Pre-filters models at chain build time via the factory; denied
  models are excluded from the chain

## Impact

- `internal/conversation/` — Uses unified interface, no behavioral change
- `internal/query/` — Factory returns chain when multiple models configured, single provider otherwise
- `internal/slacker/` — Transparently benefits from chain; model access pre-filtered at build time
- `internal/config/` — Adds provider_model + models parsing, backward-compat detection
- `go.mod` — New `sony/gobreaker/v2` dependency
- `cmd/marvin/` — No change

## Non-Goals

- Full chain engine with multiple strategies (ordered-only in this step)
- LLM Gateway as an external process
- Runtime/dynamic config updates
- Restart persistence of health state (circuit breakers reset on restart)
- Context window compression on model switch
- Changes to Slacker's model access control system (beyond adding AccessCheck to chain)
- Automatic migration of legacy configs to structured format
- RAG/embedding path changes (separate EmbeddingProvider interface defined but no existing pipeline migration)
- Per-model provider config overrides (e.g., different `base_url` per OpenRouter model)
- Buffered streaming proxy — partial output from failed provider may be visible before fallback
- LLM-aware model switch notification
