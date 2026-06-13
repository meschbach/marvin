## ADDED Requirements

### Requirement: Unified LLM Interface
The system SHALL provide a unified interface for all LLM providers that abstracts chat and
streaming behind a common contract using internal types (not Ollama SDK types).

#### Scenario: Chat request with streaming
- **GIVEN** a provider implementing the LLM interface
- **WHEN** a consumer calls `LLM.Chat(ctx, req)` with a ChatRequest containing common
  fields (Messages, Tools, Temperature, TopK, TopP)
- **THEN** the provider streams the response incrementally via a callback parameter
- **AND** when complete, the callback receives a done signal with final statistics

#### Scenario: ChatRequest common fields
- **GIVEN** a ChatRequest with Temperature, TopK, and TopP set
- **WHEN** the request is sent to any provider (Ollama, OpenRouter, Gemini)
- **THEN** the provider applies the sampling parameters as supported
- **AND** unsupported fields are silently ignored (not an error)

### Requirement: Embedding Provider (Separate Interface)
The system SHALL define a separate EmbeddingProvider interface distinct from the LLM
interface, with its own factory and lifecycle.

#### Scenario: Embedding request via separate interface
- **GIVEN** a provider implementing the EmbeddingProvider interface
- **WHEN** a consumer calls `EmbeddingProvider.Embeddings(ctx, req)`
- **THEN** the provider returns a slice of float32 vectors

#### Scenario: Provider does not support embeddings
- **GIVEN** a provider that only implements LLM (not EmbeddingProvider)
- **WHEN** a consumer attempts to use it for embeddings
- **THEN** the consumer handles "not supported" at its boundary
- **AND** no type casting or runtime assertion is used — the consumer calls the
  separate embedding factory directly

### Requirement: Provider Factory
The system SHALL provide a factory function that creates the appropriate LLM implementation
based on configuration.

#### Scenario: Single-provider configuration
- **WHEN** config uses legacy `provider` + `model` fields
- **THEN** factory returns a single-entry chain wrapping the legacy provider

#### Scenario: Multi-model chain configuration
- **WHEN** config has `provider_model` blocks with `llm { models }`
- **THEN** factory returns an OrderedChain wrapping all models in declared order

#### Scenario: Unknown provider
- **WHEN** config has an unknown provider value
- **THEN** factory returns an error with clear message

### Requirement: Consistent Error Handling
All LLM implementations SHALL return structured errors that can be handled consistently
by consumers.

#### Scenario: Retryable API error
- **WHEN** an LLM provider returns a retryable error (429, 5xx)
- **THEN** the error reports `IsRetryable() == true`
- **AND** `IsPermanent() == false`

#### Scenario: Permanent API error
- **WHEN** an LLM provider returns a permanent error (401, 404, 400)
- **THEN** the error reports `IsPermanent() == true`
- **AND** `IsRetryable() == false`

#### Scenario: Timeout
- **WHEN** an LLM request exceeds its timeout
- **THEN** the error indicates timeout and reports `IsRetryable() == true`

#### Scenario: Context cancellation
- **WHEN** the context is cancelled during an LLM request
- **THEN** the request is cancelled and returns context.Canceled or context.DeadlineExceeded
  (not wrapped — chain treats this as termination, not model failure)

### Requirement: Internal Types
The system SHALL define its own request, response, message, and tool call types in
`internal/llm/` rather than using the Ollama SDK types directly. All providers convert
between these internal types and their native API types.

#### Scenario: Provider type isolation
- **WHEN** a new provider is added
- **THEN** it only needs to implement the LLM interface using internal types
- **AND** it does not need to understand Ollama's data model
