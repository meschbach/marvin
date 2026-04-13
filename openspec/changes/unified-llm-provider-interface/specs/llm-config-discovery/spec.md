## ADDED Requirements

### Requirement: Config-Based Provider Selection
The system SHALL select the appropriate LLM implementation based on the `provider` field in the configuration file.

#### Scenario: Default provider
- **WHEN** configuration has no `provider` field specified
- **THEN** the system defaults to Ollama provider

#### Scenario: Explicit provider selection
- **WHEN** configuration specifies `provider = "openrouter"`
- **THEN** the system uses the OpenRouter implementation

#### Scenario: Model passthrough
- **WHEN** configuration specifies a model name
- **THEN** the model name is passed to the underlying provider unchanged