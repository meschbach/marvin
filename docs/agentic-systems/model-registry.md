# Model Registry

The Model Registry is the central configuration that defines available inference options across
the system. It lives at the scope level and is dynamically updatable via the administrative
interface. Agents reference chains from the registry — they do not define their own model
selection logic.

## Concepts

The registry has three levels of configuration:

| Level | Purpose | Example |
|---|---|---|
| **Provider** | A named instance of an inference service | `ollama-primary`, `openrouter-premium` |
| **Provider Model** | A specific model served through a specific provider | `gemma-4@openrouter`, `ministral@ollama` |
| **Chain** | An ordered composition of provider models with a selection strategy | `gemma-balanced`, `fast-ministral` |

### Provider

A named configuration for an inference service endpoint. Multiple instances of the same provider
type are supported (separate API keys, different hosts, different rate limit tiers).

```hcl
provider "ollama-primary" {
    type     = "ollama"
    endpoint = "http://192.168.1.100:11434"
}
provider "ollama-backup" {
    type     = "ollama"
    endpoint = "http://localhost:11434"
}
provider "openrouter-premium" {
    type    = "openrouter"
    api_key { from_env = "OPENROUTER_PREMIUM_KEY" }
}
provider "openrouter-budget" {
    type    = "openrouter"
    api_key { from_env = "OPENROUTER_BUDGET_KEY" }
}
provider "google" {
    type    = "google"
    api_key { from_env = "GOOGLE_API_KEY" }
}
```

### Provider Model

A pairing of a logical model with a specific provider. Each provider model declares its context
window and optional max_wait for rate-limit tolerance.

```hcl
provider_model "gemma-4@openrouter" {
    model          = "gemma-4"
    provider       = "openrouter-premium"
    provider_model = "google/gemma-4"
    context_window = 32000
    max_wait       = "60s"
}
provider_model "gemma-4@google" {
    model          = "gemma-4"
    provider       = "google"
    provider_model = "gemma-4"
    context_window = 32000
}
provider_model "gemma-4@ollama" {
    model          = "gemma-4"
    provider       = "ollama-primary"
    provider_model = "gemma-4"
    context_window = 28000
}
provider_model "ministral@ollama" {
    model          = "ministral-3:3b"
    provider       = "ollama-backup"
    provider_model = "ministral-3:3b"
    context_window = 32000
}
```

The `max_wait` field caps how long the system will wait on a rate-limit cooldown for this
specific provider model before advancing the chain. It is the provider-model-side limit; the
agent's `max_wait` is the other half (minimum of the two wins).

### Chain

A chain is a named, ordered composition of provider models (and optionally other chains) with a
selection strategy applied at each node. Chains are immutable from the agent's perspective —
agents select them by name without overrides.

#### Strategies

| Strategy | Behavior | When to Use |
|---|---|---|
| `ordered` | Try children left-to-right. Advance on exhaustion. Wait within max_wait before advancing. | General-purpose heterogeneous chains |
| `round-robin` | Rotate through children for load distribution. Advance only when all exhausted. | Same model across multiple providers |
| `quality-first` | Wait for preferred children to recover up to max_wait before advancing. | Prefer high-quality model, tolerate latency |
| `fast-fail` | Advance immediately on any failure, no waiting. | Latency-sensitive paths |

#### Flat Chain (Single Node)

```hcl
chain "gemma-reliable" {
    strategy = "ordered"
    models   = ["gemma-4@openrouter", "gemma-4@google", "gemma-4@ollama", "ministral@ollama"]
}
```

This is shorthand for a chain with a single implicit node containing all listed models, applying
the strategy to select among them.

#### Composed Chain (Multiple Nodes)

Chains can reference other chains, creating a tree. Each node has its own strategy.

```hcl
chain "cloud-gemma" {
    # Same model across multiple providers — rotate for utilization
    strategy = "round-robin"
    models   = ["gemma-4@openrouter", "gemma-4@google"]
}

chain "local-mini" {
    strategy = "ordered"
    models   = ["ministral@ollama"]
}

chain "gemma-balanced" {
    # Try cloud-gemma first (round-robin across providers),
    # then local-mini if all cloud options exhausted.
    # This node advances left-to-right.
    strategy = "ordered"
    chains   = ["cloud-gemma", "local-mini"]
}
```

#### Resolution Tree (Runtime View)

```
Chain: gemma-balanced (ordered)
├── Sub-chain: cloud-gemma (round-robin)
│   ├── ProviderModel: gemma-4@openrouter → Provider: openrouter-premium
│   │     model: "gemma-4", context_window: 32000, max_wait: 60s
│   └── ProviderModel: gemma-4@google → Provider: google
│         model: "gemma-4", context_window: 32000
└── Sub-chain: local-mini (ordered)
    └── ProviderModel: ministral@ollama → Provider: ollama-backup
          model: "ministral-3:3b", context_window: 32000
```

Inference 1: `cloud-gemma` selects `gemma-4@openrouter` (round-robin step)
Inference 2: `cloud-gemma` selects `gemma-4@google` (round-robin step)
Inference 3: if both `cloud-gemma` members exhausted → advance to `local-mini`
Next inference after cooldown: re-check `cloud-gemma` for recovery

## Agent Configuration

Agents are simple — they select chains and set their own `max_wait` cap.

```hcl
agent "researcher" {
    chains   = ["gemma-balanced"]
    max_wait = "30s"         # agent-level cap on any single wait
    notify_model_switch = false  # optional: inform LLM on fallback
}
```

- `chains`: ordered list — runtime flattens into a single list at load time, deduplicating.
  When multiple chains are specified, each is tried in declaration order.
- `max_wait`: agent-level cap. Effective wait for any provider model is
  `min(provider_model.max_wait, agent.max_wait)`. If neither is set, no wait limit applies.
- `notify_model_switch`: when true, the runtime injects a system note in the LLM context on
  fallback. Default false.

## Chain Engine

The Chain Engine is a per-agent-runtime component that holds a reference to the chain definition
and tracks per-provider-model cooldown and exhaustion state. It is the sole consumer of chain
definitions at runtime.

### Interface

```
Select() → Selection | exhausted

    Returns the best available provider model given current strategy + health signals.
    Does not consider context size — compression is the runtime's responsibility.

Advance(outcome) → Use{selection} | Wait{duration, selection} | exhausted

    Signals that the last Selection failed.
    - Short wait (<100ms): engine blocks inline, returns Use with same or next selection
    - Long wait: engine returns Wait directive — runtime pauses agent and retries later
    - Advance: engine returns next selection in chain traversal
    - exhausted: all options in chain are unusable

ReportSuccess(selection)

    Feeds back a successful inference. Resets health signal for that provider model,
    potentially bringing it back into rotation at the next Select.
```

### State Ownership

| State | Location | Persists Across Restarts |
|---|---|---|
| Provider endpoint health | LLM Gateway | Yes (independent process) |
| Per-provider-model cooldown | Chain Engine | No (rebuilt on restart) |
| Per-provider-model exhaustion | Chain Engine | No (rebuilt on restart) |
| Chain definition | Model Registry config | Yes |

### Cooldown Mechanics

On `Advance(RETRYABLE)` with a retry_after:

1. Provider model enters cooldown for `min(retry_after, effective_max_wait)`
2. Subsequent `Select` calls skip this provider model
3. After cooldown expires, provider model returns to `Available`
4. On repeated failure cycles: cooldown doubles per cycle (exponential: 30s → 60s → 120s)
   up to a configurable ceiling. Reset to base on success.

### Dynamic Chain Updates

Chains are mutable at the administrative level. The Chain Engine holds a shared pointer to the
chain definition. On update:

1. The runtime is notified via a callback
2. In-flight inferences complete against the old chain state
3. The next `Select` evaluates against the updated chain
4. Exhaustion flags for provider models still present in the new chain are reset
5. Journal entry: `config_change { chain: "gemma-balanced", source: "admin" }`

### Concurrent Topics

The Chain Engine is thread-safe. Cooldown and exhaustion state is shared across all topics
within an agent runtime. If topic T1 exhausts a provider model, topic T2's next `Select` also
skips it. This prevents cascading retry storms across topics.

### All Models Exhausted

When the engine returns `exhausted`, the runtime journals:
```
all_models_exhausted {
    chain: "gemma-balanced",
    failed: [
        { provider_model: "gemma-4@openrouter", reason: "rate_limit" },
        { provider_model: "gemma-4@google", reason: "unavailable" },
    ]
}
```

The next context assembly includes a system note:
> "All provider models in your model chain failed for topic T1. Retry possible in ~22s
> (gemma-4@openrouter cooldown). You may retry, simplify your request, or escalate."
