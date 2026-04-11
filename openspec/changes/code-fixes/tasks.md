## 1. RetryBlock Config Changes

- [x] 1.1 Rename MaxRetries field to MaxAttempts in RetryBlock struct (file.go:62)
- [x] 1.2 Update HCL tag from `max_retries` to `max_attempts`
- [x] 1.3 Rename MaxRetriesValue() method to MaxAttemptsValue()
- [x] 1.4 Change validation from > 0 to >= 1 in MaxAttemptsValue()
- [x] 1.5 Add cross-field validation: MaxIntervalValue() checks >= InitialIntervalValue()

## 2. Retry Error Handling Fix

- [x] 2.1 Update retry.go to return actual `err` instead of `lastErr` (line 185)
- [x] 2.2 Derive metrics status from actual error type (timeout/canceled vs API error)
- [x] 2.3 Update recordError call to use actual error status and HTTP status code

## 3. OpenRouter Metrics Spec Update

- [x] 3.1 Update openrouter-metrics/spec.md: split request counter to started/total
- [x] 3.2 Add llm.requests.started counter (no outcome at initiation)
- [x] 3.3 Update llm.requests.total to only have outcome at completion

## 4. LLM Providers Documentation Update

- [x] 4.1 Update llm-providers/spec.md: add retry block to OpenRouter config table
- [x] 4.2 Add HCL example showing retry block under openrouter
- [x] 4.3 Document retry behavior (429 handling, 5xx, exponential backoff, Retry-After)

## 5. CLI Spec - local_program Fix

- [x] 5.1 Update cli/spec.md example (line 246-249) to labeled form: local_program "git" { program = "..." }

## 6. CLI Spec - Signal Fix

- [x] 6.1 Replace SIGSTOP with SIGTERM/SIGINT in cli/spec.md (line 77)
- [x] 6.2 Replace SIGSTOP with SIGTERM/SIGINT in cli/spec.md (line 271)
- [x] 6.3 Add note that SIGSTOP is not trap-able

## 7. Verify & Test

- [x] 7.1 Run existing tests for config package
- [x] 7.2 Run pre-commit hooks (go fmt, go vet, golangci-lint)
- [x] 7.3 Verify HCL configs using max_retries still work or document migration