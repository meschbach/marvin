## Context

**Current State:**
- OpenRouter client calls `CreateChatCompletionStream` directly without retry
- HTTP 429 errors bubble up as `*openrouter.APIError`
- No visibility into retry attempts or rate limiting
- `cenkalti/backoff/v5` available as indirect dependency

**Constraints:**
- Must work within existing OpenRouter client structure
- Cannot break non-streaming paths (if any)
- Metrics must use existing OTEL infrastructure
- Configuration via HCL config, not new flags

**Stakeholders:**
- Users hitting rate limits with OpenRouter free tier
- Ops teams needing visibility into rate limiting behavior

## Goals / Non-Goals

**Goals:**
- Retry 429 (rate limit) and 5xx errors with exponential backoff
- Honor `Retry-After` header from 429 responses
- Track retry attempts in metrics
- Make max retries configurable per-model in HCL
- Export metrics via existing Prometheus endpoint

**Non-Goals:**
- Retry logic for other providers (Ollama, Gemini) - separate change
- Client-level HTTP middleware retry - less visibility
- Changes to CLI interface
- Runtime retry configuration changes

## Decisions

### 1. Retry Placement

Decision: Wrap in `chat.go` around `CreateChatCompletionStream` call.

```go
// In openrouterLLM.Chat()
operation := func() error {
    stream, err = o.httpClient.CreateChatCompletionStream(ctx, *openRouterReq)
    return err
}
err := backoff.RetryNotify(operation, bo, onRetry)
```

**Alternative:** HTTP Transport middleware
→ Rejected: Harder to extract Retry-After, less visibility into operation

### 2. Backoff Strategy

Decision: Exponential backoff with jitter, 3 retries default.

- Initial: 1 second
- Max: 30 seconds  
- Jitter: ±25%

**Alternative:** Fixed intervals
→ Rejected: Doesn't adapt to server load

### 3. Metrics Implementation

Decision: OpenTelemetry counters and histograms in chat.go.

```go
var (
    requestCounter = otel.Meter.Int64Counter("llm.requests.total")
    retryCounter   = otel.Meter.Int64Counter("llm.rate_limit_retries")
    latencyHist    = otel.Meter.Float64Histogram("llm.requests.latency")
)
```

**Alternative:** Manual Prometheus client
→ Rejected: OTEL already configured, Prometheus exporter already exists

### 4. Configuration

Decision: Add `retry` block to openrouter HCL configuration.

```hcl
provider = "openrouter"
openrouter {
    api_key = "..."
    base_url = "..."
    retry {
        max_retries = 3
        initial_interval = "1s"
        max_interval = "30s"
    }
}
```

**Alternative:** Environment variables
→ Rejected: HCL is canonical config, no new flags

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Retries consume context deadline | Respect context timeout, abort if exhausted |
| Metrics cardinality explosion | Use model as attribute, not high-cardinality labels |
| Retry-After ignored on non-429 | Log warning, still backoff |
| Backoff causes long waits | Configurable max, default reasonable |

## Migration Plan

1. Add `cenkalti/backoff/v5` as direct dependency
2. Implement backoff in chat.go with metrics
3. Update HCL config parsing for retry block
4. Add unit tests for retry scenarios
5. Verify metrics in local Prometheus scrape

## Open Questions

- Should we also retry 5xx server errors? (Yes per proposal - but worth confirming)
- Handle "X-RateLimit-Reset" header in addition to Retry-After? Yes
- Configurable backoff per-model or global? Per-model