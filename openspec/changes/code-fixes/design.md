## Context

This change addresses 7 feedback items from CodeRabbit review across config validation, retry logic, error handling, and documentation specs.

**Current State:**
- RetryBlock in `internal/config/file.go` validates fields individually
- Metrics in OpenRouter use single counter with outcome at start
- CLI spec has incorrect local_program syntax and wrong signal name

**Constraints:**
- Backward compatible field naming for HCL (only internal rename)
- Metrics must maintain Prometheus compatibility
- Spec docs must match actual implementation

## Goals / Non-Goals

**Goals:**
- Add cross-field validation to RetryBlock
- Rename max_retries to max_attempts with correct semantics
- Fix retry.go to return actual terminal error
- Update all affected spec documentation

**Non-Goals:**
- No new features or capabilities
- No API changes to CLI commands
- No changes to other providers (Ollama, Gemini)

## Decisions

### 1. Cross-field validation approach

Decision: Validate in `MaxIntervalValue()` by calling `InitialIntervalValue()`

```go
func (r *RetryBlock) MaxIntervalValue() (time.Duration, error) {
    // ... existing validation ...
    if max >= initial {
        return max, nil
    }
    return 0, fmt.Errorf("max_interval (%s) must be >= initial_interval (%s)", max, initial)
}
```

Alternative: Add separate `Validate()` method
→ Rejected: Validation needed at assignment time anyway

### 2. max_retries → max_attempts rename

Decision: Rename all occurrences, change > 0 to >= 1

| Field/Method | Old | New |
|--------------|-----|-----|
| HCL field | `max_retries` | `max_attempts` |
| Struct field | `MaxRetries` | `MaxAttempts` |
| Method | `MaxRetriesValue()` | `MaxAttemptsValue()` |
| Validation | > 0 | >= 1 |

Alternative: Keep backward alias `max_retries`
→ Rejected: Adds complexity, user explicitly asked for rename

### 3. retry.go error path

Decision: Return `err` directly, derive metrics from error type

```go
// Terminal error is in 'err', not 'lastErr'
if err != nil {
    status := deriveStatus(err)  // timeout, canceled, api_error
    o.metrics.recordError(ctx, provider, model, status, statusCode)
    return nil, fmt.Errorf("retry exhausted after %d attempts: %w", attempt, err)
}
```

Alternative: Use errors.Join to combine lastErr and err
→ Rejected: err is the actual terminal error from Retry()

### 4. Metrics counter split

Decision: Keep `llm.requests.total` as completion counter, add `llm.requests.started`

- Increment `llm.requests.started` (no outcome tag) at request initiation
- Increment `llm.requests.total` with outcome tag at completion

Alternative: Rename to `llm.requests.completed`
→ Rejected: `llm.requests.total` is more standard naming

### 5. CLI spec fixes

Decision: Direct fixes in place
- local_program: Change to labeled form with `program` key
- SIGSTOP: Replace with SIGTERM and/or SIGINT with appropriate wording

## Risks / Trade-offs

- [Risk] Field rename breaks existing configs using `max_retries`
  → [Mitigation] Document in migration notes, users must update to `max_attempts`
  
- [Risk] Metrics backward compatibility
  → [Mitigation] Adding new counter doesn't break existing Prometheus queries