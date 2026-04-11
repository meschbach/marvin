## Why

CodeRabbit feedback identified several issues requiring fixes:
1. Cross-field validation gap in retry config (max_interval >= initial_interval)
2. Semantic confusion between max_retries (attempts) vs retries (count after first attempt)
3. Metrics spec has timing issue with outcome tagging
4. Missing documentation for retry configuration
5. CLI spec has incorrect example and wrong signal name

## What Changes

**Config fixes** (`internal/config/file.go`):
- Add cross-field validation: `max_interval` must be >= `initial_interval`
- Rename `max_retries` to `max_attempts` with value >= 1 (semantics change from "retry count" to "total attempts")

**Retry error handling** (`internal/openrouter/retry.go`):
- Return actual terminal error instead of stale `lastErr`
- Derive metrics status from actual error type, not hardcoded "retry_exhausted"

**Spec documentation fixes**:
- `openspec/specs/openrouter-metrics/spec.md`: Separate started/completed request counters
- `openspec/specs/llm-providers/spec.md`: Document OpenRouter retry block
- `openspec/specs/cli/spec.md`: Fix local_program example to labeled form
- `openspec/specs/cli/spec.md`: Replace SIGSTOP with SIGTERM/SIGINT

## Capabilities

### Modified Capabilities
- `retry-config`: Update validation rules and field names
- `openrouter-metrics`: Fix counter timing semantics
- `llm-providers`: Add retry block documentation
- `cli-spec`: Fix example and signal references

## Impact

- `internal/config/file.go`: Validation logic changes, field rename
- `internal/openrouter/retry.go`: Error return and metrics logic
- `openspec/specs/openrouter-metrics/spec.md`: Metric semantics
- `openspec/specs/llm-providers/spec.md`: Documentation
- `openspec/specs/cli/spec.md`: Documentation