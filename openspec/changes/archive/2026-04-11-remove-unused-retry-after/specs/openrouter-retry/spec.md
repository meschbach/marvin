# OpenRouter Retry Specification Delta

## MODIFIED Requirements

### Requirement: Retry on Rate Limit
The system SHALL automatically retry OpenRouter requests when receiving HTTP 429 (rate limit) responses.

#### Scenario: Rate limit response without Retry-After
- **WHEN** OpenRouter returns HTTP 429 (rate limit response includes no timing headers)
- **THEN** the system waits using exponential backoff and retries the request

**Reason**: OpenRouter does not emit `Retry-After` or `x-ratelimit-reset` headers on 429 responses. The system cannot honor timing information that isn't provided.