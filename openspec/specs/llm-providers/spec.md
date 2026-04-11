# LLM Providers Specification

## Overview

Marvin supports multiple LLM (Large Language Model) providers, allowing users to choose the backend that best fits their needs. The provider is configured in HCL configuration files using the `provider` block.

### Supported Providers

| Provider | Default Model | Description |
|----------|---------------|-------------|
| Ollama | `ministral-3:3b` | Local LLM server (default) |
| OpenRouter | (user-specified) | Multi-provider API aggregation |
| Google Gemini | (user-specified) | Google's Gemini models |

### Default Models

- **DefaultLanguageModel**: `ministral-3:3b` (defined at `internal/config/file.go:15`)
- **DefaultEmbeddingModel**: `mxbai-embed-large:latest` (defined at `internal/config/file.go:16`)

---

## Provider Configuration

### Provider Selection

The `provider` option in the HCL configuration file selects which LLM backend to use:

```hcl
# Select provider (ollama, openrouter, or gemini)
provider = "ollama"
```

When not specified, Marvin defaults to **Ollama** (`internal/config/file.go:140-145`).

---

## Ollama Provider

Ollama is the default and most commonly used provider. It runs locally and connects to a local Ollama server.

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `host` | string | No | Ollama server URL (default: `http://localhost:11434`) |
| `model` | string | No | Model name (default: `ministral-3:3b`) |

### HCL Configuration Example

```hcl
# Using default local Ollama
llm {
  model = "llama3.2:latest"
  host = "http://localhost:11434"
}

# With model options
llm {
  model = "llama3.2:latest"
  host = "http://localhost:11434"

  options {
    temperature = 0.7
    top_p = 0.9
    num_predict = 512
  }
}
```

### Implementation

**File**: `internal/config/ollama.go:1-45`

The Ollama encoder provides embedding functionality:

- `ollamaEncoder` struct (line 11-14) - wraps the Ollama API client
- `Encode()` method (line 16-45) - generates embeddings from text using the Ollama embeddings endpoint

---

## OpenRouter Provider

OpenRouter provides access to multiple LLM providers through a unified API.

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `api_key` | string | Yes | OpenRouter API key |
| `base_url` | string | No | Custom endpoint (default: `https://openrouter.ai/api/v1`) |

### HCL Configuration Example

```hcl
provider = "openrouter"

openrouter {
  api_key = "sk-or-..."
  base_url = "https://openrouter.ai/api/v1"  # optional
}

llm {
  model = "anthropic/claude-3.5-sonnet"
}
```

### Implementation

**File**: `internal/openrouter/openrouter.go:1-46`

Key structs and functions:

| Element | Line | Description |
|---------|------|-------------|
| `defaultOpenRouterBaseURL` | 12 | Default API endpoint |
| `LLM` struct | 14-19 | Main client wrapper |
| `NewLLM()` | 21-46 | Constructor with HTTP client setup |

The implementation:
- Uses `github.com/revrost/go-openrouter` library
- Configures OpenTelemetry instrumentation for observability
- Sets custom HTTP headers (`HttpReferer`, `XTitle`) for API tracking

### Environment Variable

- `OPENROUTER_API_KEY` - API key source (see `internal/config/file.go:47-52`)

---

## Google Gemini Provider

Google Gemini provides access to Google's Gemini models.

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `api_key` | string | Yes | Google AI API key |

### HCL Configuration Example

```hcl
provider = "gemini"

gemini {
  api_key = "AIza..."
}

llm {
  model = "gemini-2.0-flash"
}
```

### Implementation

**File**: `internal/gemini/gemini.go:1-50`

Key structs and functions:

| Element | Line | Description |
|---------|------|-------------|
| `Streamer` interface | 14-16 | Streaming content generation interface |
| `LLM` struct | 18-22 | Main client wrapper |
| `genaiClient` | 24-30 | Google genai client adapter |
| `NewLLM()` | 32-45 | Constructor |

The implementation:
- Uses `google.golang.org/genai` library
- Provides streaming response support via iterators
- Adapts Google GenAI client to Marvin's conversation interface

### Environment Variable

- `GEMINI_API_KEY` - API key source (see `internal/config/file.go:32-38`)

---

## Model Options

The `options` block in HCL configuration provides fine-grained control over model behavior.

### Configuration Options

**File**: `internal/config/file.go:55-75`

| Option | Type | Range | Description |
|--------|------|-------|-------------|
| `context_window_size` | int | >0 | Context window size in tokens (maps to `num_ctx`) |
| `temperature` | float32 | 0.0-1.0 | Sampling temperature (higher = more creative) |
| `top_p` | float32 | 0.0-1.0 | Nucleus sampling parameter |
| `top_k` | int | ≥-1 | Top-k sampling (-1 = no limit) |
| `num_predict` | int | ≥-1 | Maximum tokens to predict (-1 = unlimited) |
| `repeat_penalty` | float32 | any | Repetition penalty |
| `repeat_last_n` | int | ≥-1 | Lookback for repetitions (-1 = context_size) |
| `seed` | int | any | Random seed for reproducibility |
| `stop` | []string | N/A | Stop sequences |

### Example with Options

```hcl
llm {
  model = "ministral-3:3b"

  options {
    temperature = 0.8
    top_p = 0.95
    top_k = 40
    num_predict = 1024
    repeat_penalty = 1.1
    stop = ["END", "STOP"]
  }
}
```

---

## Configuration File Reference

### Key Configuration Types

| Type | Location | Description |
|------|----------|-------------|
| `ProviderType` | `internal/config/file.go:18-25` | Provider type constants |
| `GeminiBlock` | `internal/config/file.go:27-38` | Gemini configuration |
| `OpenRouterBlock` | `internal/config/file.go:40-53` | OpenRouter configuration |
| `ModelOptionsBlock` | `internal/config/file.go:55-75` | Model behavior options |

### Provider Resolution

```go
// internal/config/file.go:140-145
func (f *File) Provider() ProviderType {
    if f.ProviderName == "" {
        return ProviderOllama
    }
    return ProviderType(f.ProviderName)
}
```

---

## Usage Notes

1. **Ollama is the default** - No provider configuration needed for local Ollama
2. **API keys required** - OpenRouter and Gemini require API keys via `api_key` block or environment variables
3. **Model selection** - Each provider accepts different model names; refer to provider documentation
4. **Options are provider-specific** - Not all options work with all providers; Ollama-specific options may be ignored by others
5. **Embedding models** - Use `embedding_model` in configuration to specify embedding models (default: `mxbai-embed-large:latest`)