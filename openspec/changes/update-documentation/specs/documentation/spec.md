## ADDED Requirements

### Requirement: Accurate RAG Documentation
The documentation SHALL accurately describe RAG implementation as it currently exists.

#### Scenario: Document current RAG behavior
- **WHEN** a user reads `docs/rag.md`
- **THEN** they see accurate description of Chromem + Ollama embeddings, not "will" future tense

### Requirement: Correct MCP Documents Block Syntax
The documentation SHALL show correct HCL syntax for documents block.

#### Scenario: Correct field names in docs
- **WHEN** user looks at documents config in `docs/mcp.md`
- **THEN** they see `document_path`, `storage_path` fields (not `source`, `database`)

### Requirement: Current Configuration Syntax
The configuration reference SHALL use current HCL syntax that matches implementation.

#### Scenario: Accurate config examples
- **WHEN** user reads `docs/configuration/complete-reference.md`
- **THEN** they see `model = "..."` (flat) not `model "ollama" "main" {}` (block style)