## 1. Create LLM Interface Package

- [ ] 1.1 Create `internal/llm/interface.go` with LLM interface definition (Chat, Stream, Embeddings methods)
- [ ] 1.2 Create `internal/llm/options.go` with common options struct
- [ ] 1.3 Create `internal/llm/errors.go` with structured error types
- [ ] 1.4 Create `internal/llm/factory.go` with `NewFromConfig(*config.File) (LLM, error)` function

## 2. Implement Ollama Provider

- [ ] 2.1 Create `internal/llm/ollama/client.go` implementing LLM interface for Ollama
- [ ] 2.2 Create `internal/llm/ollama/embeddings.go` implementing Embeddings method
- [ ] 2.3 Create `internal/llm/ollama/stream.go` implementing Stream method
- [ ] 2.4 Verify Ollama implementation works with existing conversation engine

## 3. Refactor OpenRouter Provider

- [ ] 3.1 Move/rename `internal/openrouter/openrouter.go` to `internal/llm/openrouter/client.go`
- [ ] 3.2 Update OpenRouter to implement LLM interface
- [ ] 3.3 Add Stream and Embeddings methods if missing
- [ ] 3.4 Update imports in consumers (temp, then remove old package after migration)

## 4. Refactor Gemini Provider

- [ ] 4.1 Move/rename `internal/gemini/gemini.go` to `internal/llm/gemini/client.go`
- [ ] 4.2 Update Gemini to implement LLM interface
- [ ] 4.3 Add Stream and Embeddings methods if missing

## 5. Migrate Consumers

- [ ] 5.1 Update `internal/conversation/engine.go` to use `internal/llm` instead of direct provider imports
- [ ] 5.2 Update `internal/query/` consumers (query.go, config.go, llm.go)
- [ ] 5.3 Update `internal/slacker/query_streaming.go` to use llm interface
- [ ] 5.4 Update any remaining consumers

## 6. Cleanup

- [ ] 6.1 Remove or deprecate `internal/config/ollama.go` helpers (or keep for RAG embeddings)
- [ ] 6.2 Update specs to reflect new package structure
- [ ] 6.3 Run all tests to verify no regressions
- [ ] 6.4 Update imports documentation if needed