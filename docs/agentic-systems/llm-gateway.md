# LLM Gateway

The LLM Gateway is an external process (not a mailbox actor) that manages connectivity to inference
providers. Every agent connects to the Gateway via gRPC streaming for inference. The Gateway is a
pure proxy with operational infrastructure layered around it — it never sees agent journals,
messages, or internal state, only LLM request/response payloads.

## Boundary

The Gateway owns all connectivity concerns. It does not influence agent behavior, model selection,
conversation flow, or context assembly. Those remain entirely agent-local (see
[Chain Engine](model-registry.md#chain-engine)).

**What the Gateway owns:**
- Provider connections (HTTP pooling, keep-alive, TLS)
- Auth rotation (rotate API keys without restarting agents)
- Retry with backoff for transient provider errors
- Per-endpoint health tracking (sliding window failures)
- Slow-start for recovering endpoints
- Rate limit management per provider
- Token counting and cost attribution
- Telemetry (latency, error rates, throughput)

**What the Gateway never sees:**
- Agent journals or working memory
- Agent messages or topic state
- Tool definitions or tool call results
- System prompts or agent instructions

## Provider Model Status Codes

Every `Complete` RPC response carries a status code indicating the outcome of that specific
inference attempt. These codes are the primary signal the Gateway returns to the caller:

| Code | Meaning | Next Action |
|---|---|---|
| `OK` | Inference succeeded normally | Use result |
| `DEGRADED` | Succeeded but endpoint is recovering (slow-start) | Accept, but caller may skip depending on chain strategy |
| `RETRYABLE` | Transient failure — provider indicated retry timing | Wait `retry_after` and retry, or advance chain |
| `UNAVAILABLE` | Permanent failure — do not retry this endpoint | Advance chain immediately |

### Response Envelope

```protobuf
message CompleteResponse {
    // Streamed content chunks (on success)
    repeated ContentChunk content = 1;

    // Per-request inference metadata
    ProviderModelStatus status = 10;
    RetryInfo retry = 11;
}

message ProviderModelStatus {
    enum Code {
        OK = 0;
        DEGRADED = 1;
        RETRYABLE = 2;
        UNAVAILABLE = 3;
    }
    Code code = 1;
    optional Duration retry_after = 2;       // populated for RETRYABLE
    optional string reason = 3;              // ops visibility (e.g., "rate_limit", "model_overloaded")
}

message RetryInfo {
    int32 retry_count = 1;          // how many retries the Gateway attempted internally
    int64 total_wait_ms = 2;        // cumulative wait time from backoff
    repeated string reasons = 3;    // individual reasons (e.g., ["429", "503", "timeout"])
}
```

## Retry with Backoff

The Gateway retries transient failures internally before returning a terminal status. The agent
never sees intermediate retry attempts — only the final outcome.

### Retryable Errors

- HTTP 429 (rate limit) — respects `Retry-After` header if present
- HTTP 5xx (server errors)
- Connection errors (timeout, reset, DNS failure, TLS handshake failure)
- Context deadline errors within the Gateway's own deadline budget

### Non-Retryable Errors

Passed through immediately as `UNAVAILABLE`:
- HTTP 4xx except 429 (auth errors, bad request, model not found)
- Content policy violations
- Invalid request format

### Algorithm

Exponential backoff with full jitter:

1. Initial interval: `initial_interval`
2. Each attempt: `wait = random(0, min(max_interval, initial_interval * multiplier^attempt))`
3. Stop on: success, non-retryable error, or `max_duration` exceeded
4. If `max_duration` exceeded with retryable error: return `RETRYABLE` with remaining
   `retry_after` from provider (or default cooldown)

### Configuration

```hcl
gateway {
    retry {
        max_attempts      = 3
        initial_interval  = "1s"
        max_interval      = "30s"
        multiplier        = 2.0
        jitter            = 0.25
        max_duration      = "60s"
    }
}
```

### Provider-Specific Behavior

| Provider Type | Retry Behavior |
|---|---|
| **Cooperative** (e.g., OpenRouter) | Respects `Retry-After` and `X-RateLimit-Reset` headers. Gateway uses exact timing from provider. |
| **Hard reject** (e.g., Google, Ollama) | No rate-limit feedback in responses. Retry once with brief backoff (2s). Second failure → `UNAVAILABLE`. |
| **Transient** (any provider) | Network-level failures (timeouts, resets). Standard exponential backoff for up to 3 attempts. |

## Health Tracking

The Gateway maintains a sliding window of errors per provider model endpoint. When the error
threshold is exceeded within the window, the endpoint is degraded:

- **State machine per endpoint:** `healthy → degraded → cooling`
- On degradation: new requests are still accepted but tagged as `DEGRADED` in response
- On cooling: requests are rejected with `UNAVAILABLE` until the cooldown expires
- Successful requests during `degraded` reset the error counter

```hcl
gateway {
    provider "openrouter-premium" {
        endpoint = "https://openrouter.ai/api/v1"
        health {
            error_threshold = 5
            window          = "60s"
            cooldown        = "120s"
        }
    }
}
```

## Slow-Start

When a degraded or cooling endpoint recovers, the Gateway does not immediately resume full
throughput. Instead, it ramps up gradually:

1. First request after recovery: accepted, tagged `DEGRADED`
2. On success: rate limit increases (1 → 2 → 4 → 8 requests/second)
3. On any failure during ramp-up: rate limit resets to 1 request/second
4. After sustained success at full rate: status transitions to `OK`

This prevents a recovered endpoint from being immediately overwhelmed by all agents returning at
once.

## gRPC Interface

```protobuf
service LLMGateway {
    // Complete sends an inference request and streams the response.
    rpc Complete(CompleteRequest) returns (stream CompleteResponse);

    // Embed sends an embedding request (non-streaming).
    rpc Embed(EmbedRequest) returns (EmbedResponse);
}

message CompleteRequest {
    string provider_model_id = 1;    // "gemma-4@openrouter"
    string provider = 2;             // "openrouter-premium"
    string model = 3;                // provider-specific model ID: "google/gemma-4"

    // The assembled context — system prompt, working memory, tools.
    // The Gateway never inspects this content.
    bytes payload = 10;

    // Deadline from the agent's topic context.
    // Gateway must complete (including retries) within this window.
    Duration deadline = 11;

    // Model-level configuration overrides.
    map<string, string> options = 12;
}

message EmbedRequest {
    string provider = 1;
    string model = 2;
    repeated string texts = 3;
    Duration deadline = 4;
}
```

**Timeout contract:** The Gateway receives the agent's remaining topic deadline with each request.
If the retry budget would exceed this deadline, the Gateway skips additional retries and returns
`UNAVAILABLE` immediately. This prevents inference from delaying topic processing beyond the
agent's deadline.

## Relationship to Agent Runtime

The Gateway is stateless with respect to any particular agent. Agent-level model selection,
fallback strategies, and cooldown tracking are handled by the [Chain Engine](model-registry.md)
inside each Agent Runtime. The Gateway and Chain Engine communicate through the status codes
returned in each `CompleteResponse`:

```
Agent Runtime                    Chain Engine                    LLM Gateway
     │                               │                               │
     │  Select() ──────────────────► │                               │
     │◄──── Selection ────────────── │                               │
     │                               │                               │
     │  Complete(selection) ──────────────────────────────────────► │
     │◄── status=RETRYABLE ───────────────────────────────────────── │
     │                               │                               │
     │  Advance(RETRYABLE) ────────► │                               │
     │◄── Wait{duration, selection} │                               │
     │  [pause, schedule resume]     │                               │
     │  ...duration passes...        │                               │
     │                               │                               │
     │  Complete(selection) ──────────────────────────────────────► │
     │◄── status=OK ──────────────────────────────────────────────── │
     │                               │                               │
     │  ReportSuccess(selection) ──► │                               │
```
