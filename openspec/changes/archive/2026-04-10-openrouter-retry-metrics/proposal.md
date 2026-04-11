## Why

OpenRouter returns HTTP 429 rate limit errors intermittently, causing requests to fail without retry. Users see intermittent failures with no visibility into retry behavior. We need retry logic with exponential backoff and metrics to track rate limiting and retry success.

## What Changes

- Add retry logic with exponential backoff around OpenRouter streaming API calls
- Extract and honor `Retry-After` header from 429 responses
- Add OpenTelemetry metrics for Prometheus export
- Configure max retries per-model or global default (3 retries, exponential backoff)
- Track retry attempts, success after retry, and wait times

### New Capabilities

- `openrouter-retry`: Retry logic with exponential backoff for rate-limited and transient errors
- `openrouter-metrics`: Prometheus metrics exposed via OpenTelemetry for rate limiting observations

## Impact

- **Modified Packages**: `internal/openrouter/`
- **Dependencies**: Add `cenkalti/backoff/v5` as direct dependency
- **Metrics**: OTEL metrics with Prometheus exporter (scraping endpoint already exists)