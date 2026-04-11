## ADDED Requirements

### Requirement: Unified LLM Interface
The system SHALL provide a unified interface for all LLM providers that abstracts chat, streaming, and embeddings operations behind a common contract.

#### Scenario: Chat request
- **WHEN** a consumer calls `LLM.Chat(ctx, req)` with a ChatRequest
- **THEN** the provider executes the request and returns a ChatResponse with content and tool calls

#### Scenario: Streaming request
- **WHEN** a consumer calls `LLM.Stream(ctx, req)` with a ChatRequest
- **THEN** the provider returns a channel of Chunks that can be read incrementally

#### Scenario: Embedding request
- **WHEN** a consumer calls `LLM.Embeddings(ctx, req)` with an EmbeddingRequest
- **THEN** the provider returns a slice of float32 vectors

### Requirement: Provider Factory
The system SHALL provide a factory function that creates the appropriate LLM implementation based on configuration.

#### Scenario: Ollama configuration
- **WHEN** config has `provider = "ollama"` (or unset, defaulting to Ollama)
- **THEN** factory returns an Ollama LLM implementation

#### Scenario: OpenRouter configuration
- **WHEN** config has `provider = "openrouter"`
- **THEN** factory returns an OpenRouter LLM implementation

#### Scenario: Gemini configuration
- **WHEN** config has `provider = "gemini"`
- **THEN** factory returns a Gemini LLM implementation

#### Scenario: Invalid provider
- **WHEN** config has an unknown provider value
- **THEN** factory returns an error with clear message

### Requirement: Consistent Error Handling
All LLM implementations SHALL return structured errors that can be handled consistently by consumers.

#### Scenario: API error
- **WHEN** an LLM provider returns an API error
- **THEN** the error includes a clear message and potentially retryable status

#### Scenario: Timeout
- **WHEN** an LLM request exceeds its timeout
- **THEN** the error indicates timeout and is retryable

#### Scenario: Context cancellation
- **WHEN** the context is cancelled during an LLM request
- **THEN** the request is cancelled and returns context.Canceled or context.DeadlineExceeded