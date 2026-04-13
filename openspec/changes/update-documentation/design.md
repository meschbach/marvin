## Context

**Current State:**
- `docs/rag.md` - 9 lines, says "Marvin **will** index" (future tense, not current)
- `docs/mcp.md` - Shows `documents` block with `source`, `database` fields but code uses `document_path`, `storage_path`
- `docs/configuration/complete-reference.md` - Uses old block style `model "ollama" "main" {}` but actual config uses flat `model = "..."`

**Correct Behavior (from code):**
- RAG: `documents "name" "document_path"` with `storage_path`, `model` fields
- Configuration: `model = "llama3.2:latest"` (flat key-value, not blocks)
- MCP transports: local_program, docker_mcp, mcp_over_http with specific fields

**Constraints:**
- Must match actual implementation in code
- Keep existing HCL syntax working
- Don't change behavior, just document it accurately

## Goals / Non-Goals

**Goals:**
- Accurate documentation matching code
- Fix syntax errors in config examples
- Remove "will" language, document current behavior

**Non-Goals:**
- Add new features
- Change configuration format
- Update implementation (separate change)

## Decisions

- Use actual config fields in examples vs. invented ones
- Reference `specs/rag/spec.md` for RAG deep-dive
- Keep MCP transport docs but fix the Documents section

## Risks / Trade-offs

- [Risk] Making docs too verbose → [Mitigation] Keep high-level in docs, link to specs for detail
- [Risk] Missing edge cases → [Mitigation] Review all config structs before writing