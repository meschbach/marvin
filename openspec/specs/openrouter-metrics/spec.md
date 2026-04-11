# OpenRouter Metrics Specification

## Purpose

TBD - Add observability metrics for OpenRouter API calls.
## Requirements
### Requirement: LLM Request Metrics
The system SHALL track request metrics using OpenTelemetry.

#### Scenario: Request started
- **WHEN** an LLM request is initiated
- **THEN** the `llm.requests.started` counter is incremented with provider and model attributes (no outcome)

#### Scenario: Request completed
- **WHEN** an LLM request completes (success or error)
- **THEN** the `llm.requests.total` counter is incremented with provider, model, and outcome attributes

#### Scenario: Request latency
- **WHEN** an LLM request completes (success or error)
- **THEN** the `llm.requests.latency` histogram records the total request time

### Requirement: Retry Metrics
The system SHALL track retry attempts and outcomes.

#### Scenario: Retry wait time
- **WHEN** waiting between retries
- **THEN** the `llm.rate_limit_wait_seconds` histogram records the wait duration

**Reason**: OpenRouter does not provide `Retry-After` headers, so only exponential backoff waits are possible.

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

