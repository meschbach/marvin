## ADDED Requirements

### Requirement: Respect server-provided rate limit reset times
When OpenRouter returns a 429 rate limit response with `X-RateLimit-Reset` in the metadata, the retry logic SHALL extract this value and wait exactly that duration instead of using exponential backoff.

#### Scenario: Reset time within configured maximum
- **GIVEN** OpenRouter returns HTTP 429 with `X-RateLimit-Reset` metadata
- **AND** the reset time is less than or equal to `max_rate_limit_wait` configuration
- **WHEN** the retry logic processes the 429 response
- **THEN** it SHALL wait exactly the server-specified reset duration before retrying

#### Scenario: Reset time exceeds configured maximum
- **GIVEN** OpenRouter returns HTTP 429 with `X-RateLimit-Reset` metadata
- **AND** the reset time is greater than `max_rate_limit_wait` configuration
- **WHEN** the retry logic processes the 429 response
- **THEN** it SHALL NOT retry
- **AND** it SHALL record the `llm.rate_limit_wait_exceeded` metric
- **AND** it SHALL return an error indicating the rate limit reset time exceeded the maximum wait

#### Scenario: No reset time in response
- **GIVEN** OpenRouter returns HTTP 429 without `X-RateLimit-Reset` metadata
- **WHEN** the retry logic processes the 429 response
- **THEN** it SHALL fall back to exponential backoff (existing behavior)

### Requirement: Configurable maximum rate limit wait
The system SHALL allow configuration of the maximum time to wait for a rate limit to reset via the `max_rate_limit_wait` option in the retry block.

#### Scenario: Default configuration
- **GIVEN** no `max_rate_limit_wait` is configured
- **WHEN** the system initializes the retry configuration
- **THEN** it SHALL use a default value of 2 minutes

#### Scenario: Custom configuration
- **GIVEN** `max_rate_limit_wait` is configured to a valid duration >= 1 second
- **WHEN** the system initializes the retry configuration
- **THEN** it SHALL use the configured value

#### Scenario: Invalid configuration below minimum
- **GIVEN** `max_rate_limit_wait` is configured to a duration less than 1 second
- **WHEN** the system initializes the retry configuration
- **THEN** it SHALL return an error indicating the value must be at least 1 second

### Requirement: Metric for exceeded rate limit waits
The system SHALL record a metric when a rate limit reset time exceeds the configured maximum.

#### Scenario: Rate limit wait exceeded recorded
- **GIVEN** a 429 response with reset time exceeding `max_rate_limit_wait`
- **WHEN** the retry logic processes the response
- **THEN** it SHALL increment the `llm.rate_limit_wait_exceeded` counter
- **AND** the metric SHALL include provider and model attributes

### Requirement: Distinguish wait time source in metrics
The system SHALL record wait time metrics with a source attribute indicating whether the wait came from server-provided reset time or exponential backoff.

#### Scenario: Server-provided reset wait
- **GIVEN** a 429 response with `X-RateLimit-Reset` within the configured maximum
- **WHEN** the retry logic waits for the reset time
- **THEN** it SHALL record the wait time to `llm.rate_limit_wait_seconds` histogram
- **AND** the metric SHALL include a `wait_source` attribute with value `server_reset`

#### Scenario: Exponential backoff wait
- **GIVEN** a 429 response without `X-RateLimit-Reset` metadata
- **WHEN** the retry logic waits using exponential backoff
- **THEN** it SHALL record the wait time to `llm.rate_limit_wait_seconds` histogram
- **AND** the metric SHALL include a `wait_source` attribute with value `exponential_backoff`