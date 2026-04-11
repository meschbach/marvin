## 1. Dependency Setup

- [x] 1.1 Add `cenkalti/backoff/v5` as direct dependency in go.mod
- [x] 1.2 Verify dependency resolves correctly with `go mod tidy`

## 2. Retry Configuration

- [x] 2.1 Add `RetryBlock` struct to `internal/config/file.go`
- [x] 2.2 Add `retry` block parsing to HCL config
- [x] 2.3 Wire retry config into OpenRouter initialization

## 3. Retry Implementation

- [x] 3.1 Create `internal/openrouter/retry.go` with backoff logic
- [x] 3.2 Extract `Retry-After` header handling from API errors
- [x] 3.3 Add 5xx retry support
- [x] 3.4 Integrate retry into `chat.go` Chat() method

## 4. Metrics Implementation

- [x] 4.1 Create OTEL meter and metric instruments in openrouter package
- [x] 4.2 Add request counter with provider/model/outcome attributes
- [x] 4.3 Add latency histogram
- [x] 4.4 Add retry counter and wait time histogram
- [x] 4.5 Add error counter with error type/HTTP status attributes

## 5. Testing

- [x] 5.1 Write unit tests for retry logic with mocked 429 responses
- [x] 5.2 Write tests for Retry-After header parsing
- [x] 5.3 Write tests for successful retry after rate limit
- [x] 5.4 Verify metrics are recorded correctly at each step
- [x] 5.5 Run existing tests to ensure no regressions

## 6. Integration

- [x] 6.1 Test end-to-end with actual OpenRouter (or mock server)
- [x] 6.2 Verify Prometheus metrics are exported correctly
- [x] 6.3 Run pre-commit hooks and verify clean build