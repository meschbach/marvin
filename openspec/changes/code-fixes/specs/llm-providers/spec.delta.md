## MODIFIED Requirements

### Requirement: OpenRouter Provider Configuration
OpenRouter provides access to multiple LLM providers through a unified API.

#### Scenario: OpenRouter configuration with retry
- **GIVEN** HCL configuration with openrouter block containing retry sub-block
- **WHEN** the configuration is parsed
- **THEN** retry parameters (max_attempts, initial_interval, max_interval) are applied to the retry handler

#### Scenario: OpenRouter retry behavior
- **GIVEN** OpenRouter returns 429 (rate limit) or transient 5xx error
- **WHEN** the request is retried
- **THEN** exponential backoff is applied with initial_interval starting delay, max_interval capping the growth, and max_attempts limiting total tries; if Retry-After header present, that value is respected for the next retry delay

**Note**: The retry behavior is configured via the `retry` sub-block within the `openrouter` configuration block.