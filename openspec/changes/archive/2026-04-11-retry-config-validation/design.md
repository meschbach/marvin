## Context

The `RetryBlock` in `internal/config/file.go` provides three accessor methods: `MaxRetriesValue()`, `InitialIntervalValue()`, and `MaxIntervalValue()`. These are called by `internal/openrouter/openrouter.go` and `internal/openrouter/retry.go` to configure the backoff library.

Currently, these accessors return any non-nil value the user configures, even if invalid (≤ 0). The backoff library accepts invalid values and either ignores them or exhibits undefined behavior.

## Goals / Non-Goals

**Goals:**
- Fail explicitly at startup or first use if user configures non-positive retry values
- Provide clear error messages indicating which config value is invalid

**Non-Goals:**
- Sanitize values silently (user's intent should be respected - if they set 0, they meant 0)
- Validate ModelOptionsBlock or other config blocks (out of scope for this change)
- Change HCL syntax or schema

## Decisions

### Decision 1: Return Error vs. Sentinel Value

**Chosen:** Return error from accessor methods

**Rationale:** The call site already handles errors (from backoff.Retry). A validation error is distinct from runtime errors and should fail fast.

**Alternative considered:** Return default + warning - rejected because:
1. Silent behavior change is dangerous (user thinks 0 retries works, gets 3)
2. Warning might be missed in logs

### Decision 2: Error Type

**Chosen:** Custom error type wrapping `fmt.Errorf` with sentinel check

**Rationale:** Keep it simple - no need for special error handling elsewhere. Callers can wrap with additional context.

### Decision 3: Fail-Fast Location

**Chosen:** Fail at accessor call time, not config parsing time

**Rationale:** Validates both HCL config and programmatically-created config. Simpler implementation.

## Risks / Trade-offs

**[Minor]:** Call sites need error handling
  
  **Mitigation:** Add 2-line error handling at each call site (4 total locations). Simple propagate-up pattern.

## Open Questions

None - design is straightforward.