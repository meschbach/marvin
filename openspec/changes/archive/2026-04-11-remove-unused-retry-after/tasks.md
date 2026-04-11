## 1. Remove Dead Code

- [x] 1.1 Remove `parseRetryAfterHeader` function from `internal/openrouter/retry.go` (lines 28-46)
- [x] 1.2 Verify `go build` passes after removal
- [x] 1.3 Run existing tests to ensure no regressions

## 2. Update Specs

- [x] 2.1 Sync openrouter-retry delta spec to main spec (`openspec sync`)
- [x] 2.2 Sync openrouter-metrics delta spec to main spec (`openspec sync`)