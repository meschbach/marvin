## Why

The RetryBlock accessors (MaxRetriesValue, InitialIntervalValue, MaxIntervalValue) currently return raw user-configured values without validating that they are positive. If a user writes `max_retries = 0` or `initial_interval = 0s` in their HCL config, the code silently accepts it and passes invalid values to the backoff library. This causes incorrect runtime behavior (e.g., 0 retries means no retries, not "use defaults") that is difficult to debug. Configuration errors should fail explicitly at startup rather than silently causing wrong behavior.

## What Changes

- Update RetryBlock accessors to validate input: return an error if the configured value exists but is ≤ 0
- Update call sites in `internal/openrouter/retry.go` and `internal/openrouter/openrouter.go` to handle the error
- Add tests in `internal/openrouter/retry_test.go` for invalid value handling

## Capabilities

### New Capabilities

- **retry-config-validation**: Validates retry block config values and fails explicitly when user sets non-positive values for MaxRetries, InitialInterval, or MaxInterval

### Modified Capabilities

- None (this is configuration-level validation, not spec behavior change)

## Impact

- **Code**: `internal/config/file.go` (accessor methods), `internal/openrouter/retry.go` (call site), `internal/openrouter/openrouter.go` (call site)
- **Config**: Retry block HCL syntax unchanged
- **User**: Invalid config now fails at startup with clear error message