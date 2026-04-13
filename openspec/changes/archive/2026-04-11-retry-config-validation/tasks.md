## 1. Update RetryBlock Accessors

- [x] 1.1 Modify `MaxRetriesValue()` in `internal/config/file.go:67-72` to return `(int, error)` - return error if configured value ≤ 0
- [x] 1.2 Modify `InitialIntervalValue()` in `internal/config/file.go:74-79` to return `(time.Duration, error)` - return error if configured value ≤ 0
- [x] 1.3 Modify `MaxIntervalValue()` in `internal/config/file.go:81-86` to return `(time.Duration, error)` - return error if configured value ≤ 0

## 2. Update Call Sites

- [x] 2.1 Update `internal/openrouter/openrouter.go:61-74` - handle errors from `InitialIntervalValue()` and `MaxIntervalValue()`
- [x] 2.2 Update `internal/openrouter/retry.go:151-154` - handle error from `MaxRetriesValue()`

## 3. Update Tests

- [x] 3.1 Add test for `MaxRetriesValue()` with zero value in `internal/openrouter/retry_test.go`
- [x] 3.2 Add test for `MaxRetriesValue()` with negative value in `internal/openrouter/retry_test.go`
- [x] 3.3 Add test for `InitialIntervalValue()` with zero value in `internal/openrouter/retry_test.go`
- [x] 3.4 Add test for `InitialIntervalValue()` with negative value in `internal/openrouter/retry_test.go`
- [x] 3.5 Add test for `MaxIntervalValue()` with zero value in `internal/openrouter/retry_test.go`
- [x] 3.6 Add test for `MaxIntervalValue()` with negative value in `internal/openrouter/retry_test.go`
- [x] 3.7 Update existing tests that call accessor methods to handle new return signatures