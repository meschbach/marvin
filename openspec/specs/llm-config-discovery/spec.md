# LLM Configuration and Model Discovery Specification

## Purpose

Defines how models are configured, ordered, and discovered in the system, supporting both
legacy single-model configuration and structured multi-model chains.

## Requirements

### Requirement: Structured Model Configuration

The system SHALL support configuring multiple named `(provider, model)` pairs via
`provider_model` HCL blocks.

#### Scenario: Single provider_model block

- **WHEN** config has a `provider_model` block with a label, provider, and model
- **THEN** the system registers that `(provider, model)` pair under the given label

#### Scenario: Multiple provider_model blocks

- **WHEN** config has multiple `provider_model` blocks with unique labels
- **THEN** all are registered and available for preference ordering

#### Scenario: Duplicate labels

- **WHEN** two `provider_model` blocks have the same label
- **THEN** config parsing returns an error

#### Scenario: Unknown provider in block

- **WHEN** a `provider_model` block specifies a provider that isn't configured
- **THEN** config parsing returns an error

### Requirement: Model Preference Ordering

The system SHALL support declaring an ordered list of model preferences via a
`llm { models }` attribute.

#### Scenario: Preference list references valid labels

- **WHEN** `llm { models }` references labels that exist as `provider_model` blocks
- **THEN** the chain uses models in that order

#### Scenario: Preference references non-existent label

- **WHEN** `llm { models }` references a label not defined by any `provider_model` block
- **THEN** config parsing returns an error

#### Scenario: No models list provided

- **WHEN** `provider_model` blocks exist but no `llm { models }` is specified
- **THEN** the chain uses all `provider_model` blocks in declaration order

### Requirement: Backward Compatibility

The system SHALL support legacy `provider` + `model` configuration without changes.

#### Scenario: Legacy config

- **WHEN** config has `provider = "ollama"` and `model = "ministral-3:3b"` (no
  `provider_model` blocks)
- **THEN** the system builds a single-entry chain using the legacy provider and model

#### Scenario: Mixed legacy and structured

- **WHEN** config has both legacy `provider`/`model` fields AND `provider_model` blocks
- **THEN** config parsing returns an error (mutually exclusive)
