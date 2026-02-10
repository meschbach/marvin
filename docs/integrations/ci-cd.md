# CI/CD Integration with Marvin + Slacker

## Quick Start

### GitHub Actions
```yaml
name: Marvin Testing
on: [push, pull_request]
jobs:
  marvin-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: ollama/ollama@main
      - run: ollama pull ministral-3:3b
      - run: go run ./cmd/marvin query "Review PR for security issues"
```

### GitLab CI
```yaml
marvin_test:
  stage: test
  image: golang:1.21
  services:
    - name: ollama/ollama
      alias: ollama
  script:
    - go run ./cmd/marvin query "Analyze code quality"
```

## Pipeline Patterns

### Automated Testing
- **PR Reviews**: AI-powered code analysis for every pull request
- **Security Scanning**: Automated vulnerability detection
- **Performance Testing**: Code efficiency analysis
- **Documentation**: Auto-generate API docs

### Quality Gates
```bash
# Fail build on critical issues
go run ./cmd/marvin query "Check for breaking changes" --strict

# Generate report
go run ./cmd/marvin rag --files src/ --query "Generate changelog" > CHANGELOG.md
```

### Deployment Integration
```yaml
# Deploy with approval workflow
deploy:
  needs: marvin-approval
  script:
    - ./deploy.sh
```

## Best Practices

1. **Tool Selection**: Use Marvin for analysis, traditional tools for execution
2. **Model Choice**: Ministral-3 for speed, larger models for complex analysis
3. **Context Limits**: Chunk large codebases for thorough analysis
4. **Security**: Never pass sensitive data to AI models
5. **Cost Control**: Use conditional AI calls based on file changes

## Configuration

### Pipeline-Specific Config
Create `.marvin.ci.hcl`:
```hcl
model "ollama" "ci" {
  name = "ministral-3:3b"
  timeout = "30s"
}

program "marvin" "ci" {
  model = model.ollama.ci
  description = "CI/CD pipeline automation"
}
```

### Slack Integration
Connect pipeline status to Slack:
```bash
# Notify on failure
go run ./cmd/slacker approve "Deploy failed: ${BUILD_URL}" --channel "#devops"
```

## Enterprise Features

- **Parallel Testing**: Run multiple Marvin queries simultaneously
- **Caching**: Cache model responses for repeated analyses
- **Metrics**: Track AI usage and costs across pipelines
- **Compliance**: Log all AI interactions for audit trails

## Troubleshooting

- **Timeouts**: Increase timeout in CI configuration
- **Resource Limits**: Ensure sufficient memory for Ollama
- **Network Issues**: Use self-hosted runners for private repos
- **Model Availability**: Pre-cache models in pipeline images