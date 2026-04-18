## 1. Configuration

- [x] 1.1 Add `MaxRateLimitWait` field to `RetryBlock` struct in `internal/config/file.go`
- [x] 1.2 Add `MaxRateLimitWaitValue()` method with default 2 minutes, min 1 second validation
- [x] 1.3 Add constant `DefaultMaxRateLimitWait = 2 * time.Minute`
- [x] 1.4 Add tests for `MaxRateLimitWaitValue()` in `internal/config/retryblock_test.go`

## 2. Metrics

- [x] 2.1 Add `rateLimitExceededCounter metric.Int64Counter` to `metricsRecorder` struct
- [x] 2.2 Initialize counter in `newMetricsRecorder()` as `llm.rate_limit_wait_exceeded`
- [x] 2.3 Add `recordRateLimitExceeded()` method to `metricsRecorder`
- [x] 2.4 Add `waitSource` attribute to `recordWaitTime()` to distinguish `server_reset` vs `exponential_backoff`

## 3. Reset Time Extraction

- [x] 3.1 Implement `extractRateLimitReset()` function in `internal/openrouter/retry.go`
- [x] 3.2 Handle both float64 and string types from metadata map
- [x] 3.3 Convert milliseconds to time.Duration
- [x] 3.4 Add unit tests for `extractRateLimitReset()` in `internal/openrouter/retry_test.go`

## 4. Custom Retry Loop

- [x] 4.1 Replace `backoff.Retry()` with custom for loop in `executeWithRetry()`
- [x] 4.2 On 429, call `extractRateLimitReset()` to get server-provided wait time
- [x] 4.3 If reset time ≤ max wait: sleep that duration, continue retry
- [x] 4.4 If reset time > max wait: record exceeded metric, return error
- [x] 4.5 Fall back to exponential backoff when no reset time in response
- [x] 4.6 Check context deadline during sleeps to handle context cancellation

## 5. Integration & Testing

- [x] 5.1 Update `getBackoff()` to expose backoff manager for fallback waits
- [x] 5.2 Add integration tests for retry loop behavior in `internal/openrouter/retry_test.go`
- [x] 5.3 Verify all existing tests still pass
- [x] 5.4 Run `go vet` and `golangci-lint` to check for issues