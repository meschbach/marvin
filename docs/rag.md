# Retrieval Augmented Retrieval

Retrieval Augmented Retrieval (RAG) is a technique that enhances language model responses by retrieving relevant
information from external knowledge sources before generating an answer.

## Implementation
Marvin will index and query a [Chromem](github.com/philippgille/chromem-go) database stored in the specified locations.
Core user visible component exists within the configuration file defined in `github.com/meschbach/marvin/internal/config.DocumentsBlock`.
Tooling surrounding supporting LLM interactions is located in `chromemTool` in `github.com/meschbach/marvin/internal/query`.
