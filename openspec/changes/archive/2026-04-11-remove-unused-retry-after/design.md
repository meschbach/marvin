## Context

Code review identified dead code in `internal/openrouter/retry.go`: the `parseRetryAfterHeader` function was added assuming OpenRouter sends `Retry-After` headers on 429 responses. Investigation confirmed OpenRouter does not provide this header or any rate limit timing information (like `x-ratelimit-reset`). The function can never be called in the current architecture.

## Goals / Non-Goals

**Goals:**
- Remove dead code to reduce confusion
- Update specs to reflect actual OpenRouter behavior

**Non-Goals:**
- Change retry behavior (exponential backoff remains)
- Add new functionality

## Decisions

| Decision | Rationale |
|----------|-----------|
| Remove entire function | Function cannot work given current `streamCreator` signature returns only error, not response. Restructuring to pass response would be new work beyond cleanup scope. |
| Update specs not delete | Spec files provide valuable documentation of retry behavior; removing inaccurate scenarios improves accuracy |

## Risks / Trade-offs

- **Risk**: Future API change - OpenRouter could add Retry-After headers later
- **Mitigation**: Git preserves history; easy to restore if needed

## Open Questions

None - this is straightforward cleanup.