# OpenRouter Metrics Specification Delta

## MODIFIED Requirements

### Requirement: Retry Metrics
The system SHALL track retry attempts and outcomes.

#### Scenario: Retry wait time
- **WHEN** waiting between retries
- **THEN** the `llm.rate_limit_wait_seconds` histogram records the wait duration

**Reason**: OpenRouter does not provide `Retry-After` headers, so only exponential backoff waits are possible.