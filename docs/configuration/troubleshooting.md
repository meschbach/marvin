# Troubleshooting Guide

## Quick Diagnosis

### Health Check Commands
```bash
# Check configuration
marvin config validate

# Test model connectivity
marvin model test

# Verify MCP tools
marvin mcp list

# Check Slack connection
marvin slack test
```

### Log Locations
- **Application**: `/var/log/marvin.log` or `marvin.log`
- **Slack Events**: `/var/log/marvin-slack.log`
- **MCP Tools**: Individual tool logs in `/var/log/marvin-tools/`

## Common Issues

### Connection Problems

#### Ollama Not Responding
```bash
# Error: "connection refused"
curl http://localhost:11434/api/tags

# Fix: Start Ollama
ollama serve
# or use Docker
docker run -p 11434:11434 ollama/ollama
```

#### Model Not Found
```bash
# Error: "model not found"
ollama list

# Fix: Pull required model
ollama pull ministral-3:3b
ollama pull llama3.1:8b
```

#### Slack Connection Failed
```bash
# Check tokens
echo $SLACK_BOT_TOKEN | cut -c1-10
echo $SLACK_APP_TOKEN | cut -c1-10

# Verify bot permissions
curl -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  https://slack.com/api/auth.test
```

### Performance Issues

#### Slow Response Times
```hcl
# In .marvin.hcl - reduce model size
model "ollama" "fast" {
  name = "ministral-3:3b"  # Use smaller model
  timeout = "30s"          # Reduce timeout
}
```

#### Memory Issues
```bash
# Check Ollama memory usage
docker stats $(docker ps | grep ollama)

# Fix: Limit model context
model "ollama" "light" {
  max_tokens = 1024  # Reduce from default 4096
}
```

### MCP Tool Problems

#### Docker MCP Not Working
```bash
# Check Docker daemon
docker ps

# Verify socket access
ls -la /var/run/docker.sock

# Fix permissions
sudo usermod -aG docker $USER
# or run with sudo
sudo marvin query "docker ps"
```

#### Filesystem Access Denied
```hcl
# Check config - paths must be absolute
filesystem {
  read  = ["/absolute/path/to/src"]
  write = ["/absolute/path/to/tmp"]
}
```

#### HTTP MCP Timeouts
```hcl
mcp "http" {
  config = {
    timeout = "60s"  # Increase timeout
    max_response_size = "5MB"  # Reduce size limit
  }
}
```

## Configuration Errors

### Invalid HCL Syntax
```bash
# Error: "failed to parse config"
# Fix: Check HCL syntax
marvin config validate --verbose

# Common issues:
# - Missing commas
# - Unmatched brackets
# - Invalid string quotes
```

### Missing Required Fields
```hcl
# Error: "missing required field 'model'"
program "marvin" "bad" {
  # Missing: model = model.ollama.main
  description = "Invalid program"
}

# Fix: Add required fields
program "marvin" "good" {
  model = model.ollama.main
  description = "Valid program"
}
```

### Circular Dependencies
```hcl
# Error: "circular dependency detected"
# This is invalid:
model "ollama" "a" {
  name = model.ollama.b.name  # Circular
}
```

## Slack Bot Issues

### Bot Not Responding
```bash
# Check bot status
curl -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  https://slack.com/api/apps.auth.info

# Restart bot
marvin slack restart
```

### Approval Workflow Not Working
```hcl
# Check admin configuration
slack {
  admin_channels = ["#admins"]  # Must exist
}

# Verify user permissions
marvin slack users list
```

### Rate Limiting
```hcl
# Increase limits if needed
slack {
  rate_limit {
    messages_per_minute = 120  # Increase from 60
  }
}
```

## Debugging Tools

### Verbose Logging
```bash
# Enable debug mode
marvin --log-level debug query "test"

# Check specific component
marvin --log-level debug mcp test docker
```

### Network Debugging
```bash
# Test Ollama connectivity
curl -v http://localhost:11434/api/version

# Test Slack API
curl -v -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  https://slack.com/api/auth.test
```

### Configuration Debugging
```bash
# Show effective configuration
marvin config show

# Test specific program
marvin program test my-program
```

## Recovery Procedures

### Corrupted Configuration
```bash
# Backup current config
cp .marvin.hcl .marvin.hcl.backup

# Reset to defaults
marvin config init --force

# Restore from backup selectively
marvin config restore --from .marvin.hcl.backup --only models
```

### Stuck MCP Server
```bash
# List running tools
marvin mcp list

# Restart specific tool
marvin mcp restart docker

# Restart all tools
marvin mcp restart --all
```

### Database Issues
```bash
# Check vector store integrity
marvin rag check

# Rebuild index
marvin rag rebuild --force

# Clear cache
marvin cache clear --all
```

## Performance Tuning

### Memory Optimization
```hcl
# Reduce concurrent requests
performance {
  concurrent_requests = 2  # Reduce from 5
}

# Lower model limits
model "ollama" "optimized" {
  max_tokens = 2048  # Reduce from 4096
}
```

### CPU Optimization
```bash
# Set process priority
nice -n 10 marvin query "task"

# Limit CPU usage
cpulimit -l 50 marvin query "task"
```

### Disk I/O Optimization
```hcl
# Configure cache settings
performance {
  cache_enabled = false  # Disable if disk I/O is bottleneck
}

# Or use memory cache
performance {
  cache_type = "memory"
  cache_size = "256MB"
}
```

## Security Issues

### Unauthorized Access
```bash
# Check file permissions
ls -la .marvin.hcl
chmod 600 .marvin.hcl  # Restrict to owner only
```

### Exposed Secrets
```bash
# Check for secrets in logs
grep -i "password\|token\|key" /var/log/marvin.log

# Use environment variables instead
export MARVIN_SLACK_TOKEN="xoxb-..."
```

### Network Security
```hcl
# Restrict allowed domains
mcp "http" {
  config = {
    allowed_domains = ["api.company.com"]  # Whitelist approach
  }
}
```

## Getting Help

### Collect Diagnostic Information
```bash
# Generate support bundle
marvin support bundle

# Include:
# - Configuration files
# - Recent logs
# - System information
# - Network connectivity tests
```

### Community Support
- Check GitHub Issues for known problems
- Review documentation at `/docs`
- Use `marvin --help` for command assistance

### Escalation
```bash
# Create detailed bug report
marvin bug report --include-logs --include-config
```