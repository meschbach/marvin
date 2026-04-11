## ADDED Requirements

### Requirement: LLM Request Metrics
The system SHALL track request metrics using OpenTelemetry.

#### Scenario: Request counter
- **WHEN** an LLM request is initiated
- **THEN** the `llm.requests.total` counter is incremented with provider, model, and outcome attributes

#### Scenario: Request latency
- **WHEN** an LLM request completes (success or error)
- **THEN** the `llm.requests.latency` histogram records the total request time

### Requirement: Retry Metrics
The system SHALL track retry attempts and outcomes.

#### Scenario: Retry attempt
- **WHEN** a request is retried due to 429 or 5xx
- **THEN** the `llm.rate_limit_retries` counter is incremented

#### Scenario: Retry wait time
- **WHEN** waiting between retries (or Retry-After)
- **THEN** the `llm.rate_limit_wait_seconds` histogram records the wait duration

#### Scenario: Success after retry
- **WHEN** a request succeeds after one or more retries
- **THEN** the counter attributes include `retryed: "true"`

### Requirement: Error Metrics
The system SHALL track errors by type for observability.

#### Scenario: Error tracking
- **WHEN** a request fails with API error
- **THEN** the `llm.errors.total` counter is incremented with error type and HTTP status

### Requirement: Prometheus Export
The system SHALL expose metrics via Prometheus scrape endpoint.

#### Scenario: Metrics scrape
- **WHEN** Prometheus scrapes the /metrics endpoint
- **THEN** the OTEL metrics are available in Prometheus format