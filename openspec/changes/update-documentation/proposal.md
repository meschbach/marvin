## Why

Documentation has fallen out of sync with implementation, causing confusion for users trying to configure Marvin. Key issues: RAG documentation still says "will" instead of describing actual behavior; MCP docs show config syntax that doesn't match the code; configuration reference uses outdated HCL block style.

## What Changes

- Rewrite `docs/rag.md` to document actual RAG implementation (Chromem + Ollama embeddings)
- Update `docs/mcp.md` Documents section to match actual `documents` block configuration
- Rewrite `docs/configuration/complete-reference.md` to use current HCL syntax
- Add missing config options where docs show features not in code (HTTP MCP timeouts, health checks)
- Verify other docs (slacker/, deployment/) are accurate

**No breaking changes** - this is documentation only.

## Capabilities

### Modified Capabilities
- `configuration`: Update configuration reference to match actual HCL syntax
- `rag`: Update RAG documentation from "will" to current behavior
- `mcp-tools`: Fix documents block config in MCP docs

## Impact

- `docs/rag.md` - Complete rewrite
- `docs/mcp.md` - Documents block section fix
- `docs/configuration/complete-reference.md` - Major rewrite
- `specs/rag/spec.md` - Already created in exploration phase, aligns with actual implementation