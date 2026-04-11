## ADDED Requirements

### Requirement: Retry on Rate Limit
The system SHALL automatically retry OpenRouter requests when receiving HTTP 429 (rate limit) responses.

#### Scenario: Rate limit response with Retry-After
- **WHEN** OpenRouter returns HTTP 429 with `Retry-After` header
- **THEN** the system waits the specified duration and retries the request

#### Scenario: Rate limit response without Retry-After
- **WHEN** OpenRouter returns HTTP 429 without `Retry-After` header
- **THEN** the system waits using exponential backoff and retries the request

### Requirement: Retry Configuration
The system SHALL support configurable retry parameters per OpenRouter configuration.

#### Scenario: Custom retry settings
- **WHEN** HCL config specifies `retry { max_retries = N, ... }`
- **THEN** the system uses the specified values for retry behavior

#### Scenario: Default retry settings
- **WHEN** HCL config has no retry block
- **THEN** the system uses defaults: 3 max retries, 1s initial, 30s max interval

### Requirement: Retry Exhaustion
The system SHALL return an error when all retry attempts are exhausted.

#### Scenario: All retries exhausted
- **WHEN** all retry attempts return errors
- **THEN** the final error is returned to the caller with context about retries attempted

### Requirement: Context Timeout Respected
The system SHALL abort retry loop if context timeout is exceeded.

#### Scenario: Context deadline approached
- **WHEN** context deadline would be exceeded by next retry wait
- **THEN** the system returns context.DeadlineExceeded immediately

## MODIFIED Requirements

### Requirement: OpenRouter Error Handling
The OpenRouter client SHALL handle API errors consistently for retry logic.

#### Scenario: Transient server error
- **WHEN** OpenRouter returns HTTP 5xx (500, 502, 503, 504)
- **THEN** the system retries the request (same as 429)