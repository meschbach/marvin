## MODIFIED Requirements

### Requirement: LLM Request Metrics
The system SHALL track request metrics using OpenTelemetry.

#### Scenario: Request started
- **WHEN** an LLM request is initiated
- **THEN** the `llm.requests.started` counter is incremented with provider and model attributes (no outcome)

#### Scenario: Request completed successfully
- **WHEN** an LLM request completes with success
- **THEN** the `llm.requests.total` counter is incremented with provider, model, and outcome="success" attributes

#### Scenario: Request completed with error
- **WHEN** an LLM request completes with error
- **THEN** the `llm.requests.total` counter is incremented with provider, model, and outcome attributes

#### Scenario: Request latency
- **WHEN** an LLM request completes (success or error)
- **THEN** the `llm.requests.latency` histogram records the total request time