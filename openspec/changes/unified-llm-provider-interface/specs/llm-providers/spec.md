## MODIFIED Requirements

### Requirement: Provider Interface (Changed from: Provider-Specific Implementation)

The system SHALL provide a unified LLM interface that all providers implement. Currently, each provider (Ollama, OpenRouter, Gemini) has its own interface structure. After this change, all providers implement a common interface.

**Previous behavior**: Each provider had separate implementations in different packages without a common interface.

**New behavior**: All providers implement `LLM` interface with Chat, Stream, and Embeddings methods.

#### Scenario: Unified provider access
- **WHEN** a consumer imports `internal/llm` and calls `llm.NewFromConfig(cfg)`
- **THEN** the factory returns an LLM implementation matching the configured provider

### Requirement: Provider Configuration (Changed from: Provider-Specific Config Blocks)

The system SHALL support provider selection via the `provider` field in configuration. Currently documented in `internal/config/file.go`. After this change, this behavior remains but is routed through the llm package factory.

**Previous behavior**: Provider selection via HCL `provider` field, config blocks for each provider.

**New behavior**: Same HCL interface, but internal routing handled by `llm.NewFromConfig()`.

#### Scenario: Configuration-driven provider selection
- **WHEN** HCL config has `provider = "openrouter"` with openrouter block
- **THEN** factory returns OpenRouter implementation with config from openrouter block