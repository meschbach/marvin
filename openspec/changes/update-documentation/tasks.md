## 1. Update RAG Documentation

- [ ] 1.1 Rewrite `docs/rag.md` to document current RAG implementation (Chromem + Ollama embeddings)
- [ ] 1.2 Remove "will" language, document actual behavior
- [ ] 1.3 Add working HCL example with correct field names

## 2. Fix MCP Documents Section

- [ ] 2.1 Update `docs/mcp.md` Documents section to use correct field names
- [ ] 2.2 Change `source` → `document_path`, `database` → `storage_path`
- [ ] 2.3 Verify all other MCP transport docs are accurate

## 3. Rewrite Configuration Reference

- [ ] 3.1 Rewrite `docs/configuration/complete-reference.md` 
- [ ] 3.2 Use current HCL syntax (`model = "..."` not block style)
- [ ] 3.3 Reference actual config structs for accuracy

## 4. Review Other Documentation

- [ ] 4.1 Review `docs/slacker/` for accuracy
- [ ] 4.2 Review `docs/deployment/` for accuracy
- [ ] 4.3 Fix any issues found during review