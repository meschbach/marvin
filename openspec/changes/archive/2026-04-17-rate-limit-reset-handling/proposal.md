## Why

OpenRouter returns rate limit information in the 429 response metadata, specifically `X-RateLimit-Reset` indicating when the quota will replenish. Currently, the retry logic ignores this and uses fixed exponential backoff, causing unnecessary waits when the server already knows exactly when the limit will clear. This leads to longer retry delays than needed and poor user experience during rate limiting.

## What Changes

- Add `max_rate_limit_wait` configuration option to retry block (default: 2 minutes, min: 1 second)
- Implement extraction of `X-RateLimit-Reset` from OpenRouter 429 error responses
- Replace backoff.Retry() with custom retry loop that respects server-provided reset times
- Add new metric `llm.rate_limit_wait_exceeded` to track when reset time exceeds configured max

## Capabilities

### New Capabilities
- **rate-limit-reset-handling**: Parse server-provided rate limit reset times from OpenRouter 429 responses and wait accordingly instead of using blind exponential backoff

### Modified Capabilities
- (none - this is a new capability, no existing spec requirements change)

## Impact

- **Files modified**: 
  - `internal/config/file.go` - Add MaxRateLimitWait field to RetryBlock
  - `internal/openrouter/retry.go` - Implement custom retry loop with reset time handling
  - `internal/openrouter/retry_test.go` - Add tests for new behavior
- **Config**: New optional `max_rate_limit_wait` in retry block (HCL)
- **Metrics**: New counter `llm.rate_limit_wait_exceeded` for observability
- **Dependencies**: No new external dependencies