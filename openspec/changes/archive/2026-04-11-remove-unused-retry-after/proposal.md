## Why

OpenRouter does not emit `Retry-After` or `x-ratelimit-reset` headers on 429 responses. The `parseRetryAfterHeader` function in `internal/openrouter/retry.go` is therefore dead code - it can never be called given the current architecture, and the retry spec incorrectly assumes this header exists. Removing it clarifies the codebase and aligns specs with reality.

## What Changes

- Remove `parseRetryAfterHeader` function from `internal/openrouter/retry.go` (lines 28-46)
- Update `openspec/specs/openrouter-retry/spec.md` to remove the "Rate limit response with Retry-After" scenario
- Update `openspec/specs/openrouter-metrics/spec.md` to remove reference to Retry-After in wait time scenario

## Capabilities

### Modified Capabilities

- `openrouter-retry`: Remove the Retry-After scenario from the spec, leaving only exponential backoff behavior
- `openrouter-metrics`: Remove parenthetical reference to Retry-After in wait time metric scenario

## Impact

- **Code**: `internal/openrouter/retry.go` - removal of ~18 lines of dead code
- **Specs**: Two spec files updated to reflect actual OpenRouter behavior
- **None breaking**: No runtime behavior changes; this is purely cleanup