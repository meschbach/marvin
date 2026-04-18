## Context

OpenRouter returns rate limit information in the `metadata` field of 429 error responses. Specifically, `X-RateLimit-Reset` contains a Unix timestamp (in milliseconds) indicating when the rate limit will be reset. Currently, the retry logic in `internal/openrouter/retry.go` uses exponential backoff via `cenkalti/backoff/v5`, ignoring the server's explicit reset time.

## Goals / Non-Goals

**Goals:**
- Extract `X-RateLimit-Reset` from OpenRouter 429 error metadata
- Wait the server-stated duration when it's reasonable (≤2 minutes by default)
- Fail fast with clear error when reset time exceeds configured maximum
- Add observability for rate limit wait exceeds events

**Non-Goals:**
- This only applies to OpenRouter provider (not Ollama, not other providers)
- Not implementing `X-RateLimit-Remaining` logic at this time
- Not changing the underlying backoff behavior for non-rate-limit errors

## Decisions

### 1. Custom retry loop vs library
**Decision:** Replace `backoff.Retry()` with a custom retry loop

**Rationale:** The library doesn't provide a way to inject custom wait logic per-error. We need to inspect each error after it occurs and decide whether to use the server's reset time or fall back to exponential backoff. A custom loop gives us this flexibility.

### 2. Configuration field placement
**Decision:** Add `max_rate_limit_wait` as a field in the existing `RetryBlock` struct

**Rationale:** This keeps all retry-related configuration in one place. Users already have a retry block for max_attempts, initial_interval, max_interval. Adding max_rate_limit_wait fits naturally.

**Alternative considered:** Could have been a top-level OpenRouter config field, but grouping with retry makes sense since it's about retry behavior.

### 3. 2-minute default with 1-second minimum
**Decision:** Default `max_rate_limit_wait` to 2 minutes, minimum 1 second

**Rationale:** 2 minutes is reasonable for most rate limits while not being excessive. The 1-second minimum ensures there's at least some time for quota to tick up on retries.

### 4. Metric naming
**Decision:** Use `llm.rate_limit_wait_exceeded` counter

**Rationale:** Follows existing naming pattern (`llm.rate_limit_retries`). "Exceeds" clearly indicates the condition - reset time was greater than max.

## Risks / Trade-offs

- **Risk:** `X-RateLimit-Reset` format varies
  - **Mitigation:** Handle both float64 (JSON number) and string types in the metadata map

- **Risk:** Clock skew between client and server
  - **Mitigation:** The value from OpenRouter is a relative offset in milliseconds from now, not an absolute timestamp, so clock skew doesn't apply

- **Risk:** Users misconfigure very short max wait times
  - **Mitigation:** Enforce 1-second minimum in validation

- **Trade-off:** We lose some of backoff library's jitter and interval management for non-429 errors
  - **Mitigation:** Fall back to the same exponential backoff for non-rate-limit retryable errors

## Migration Plan

1. Deploy with new code, default config (2 minutes max wait)
2. Existing configurations continue to work (field is optional)
3. Users can opt-in to custom wait behavior by updating their HCL config

No migration needed - this is additive functionality.

## Open Questions

- ~~Should we expose the actual wait time in a metric?~~ **Resolved** - Will add `wait_source` attribute to `llm.rate_limit_wait_seconds` histogram with values `server_reset` or `exponential_backoff`
- ~~Is there value in also checking `X-RateLimit-Remaining`?~~ **Deferred** - Not implementing in this change

## Additional Risks Identified

- **Risk:** Context timeout during long waits
  - **Mitigation:** Check context deadline in retry loop; if context cancels during sleep, return immediately with context error