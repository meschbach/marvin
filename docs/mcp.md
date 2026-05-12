# [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) Features

MCP is a protocol for providing tools and resources for Large Language Models to provide additional context or perform
actions on behalf of the user. Marvin supports multiple MCP transport types to accommodate different use cases and
security requirements.

## 🚀 **Supported Transports**

### **1. Local Programs** (`local_program`)
Execute local MCP servers via `stdio` communication.

**Use Cases:**
- Development and testing tools
- System administration utilities
- Custom internal tools

**Configuration:**
```hcl
local_program "my_tool" {
  program = "/usr/local/bin/my-mcp-server"
  args = ["--read-only", "--config", "/etc/my-tool/config.yaml"]

  env "API_KEY" {
    value = "your-api-key-here"
  }

  sharing {
    allowed_users = ["U123456789"]
    can_share = false
  }

  assistant_prompt {
    from_string = <<EOS
You have access to my custom tool. Use it for...
EOS
  }
}
```

**Security:** Requires admin approval in Slacker (high risk).

### **2. Docker Containers** (`docker_mcp`)
Run MCP servers in isolated Docker containers.

**Use Cases:**
- Third-party MCP servers
- Isolated execution environments
- Complex tool dependencies
- Microservices architecture

**Configuration:**
```hcl
docker_mcp "postgres_tools" "postgres:15" {
  env "DATABASE_URL" {
    value = "postgresql://user:pass@localhost:5432/db"
  }

  mount "/data" "/host/data" {
    description = "Database data directory"
  }

  resources {
    memory = "512Mi"
    cpu = "500m"
  }

  sharing {
    allowed_users = ["U123456789"]
    can_share = true
  }
}
```

**Security:** Requires admin approval in Slacker (medium risk).

### **3. HTTP MCP Servers** (`mcp_over_http`)
Connect to remote MCP servers via HTTP/HTTPS.

**Use Cases:**
- Cloud-based MCP services
- Public API integrations
- Multi-tenant shared services
- Rapid tool deployment

**Configuration:**
```hcl
mcp_over_http "weather_api" "https://weather.example.com/mcp" {
  env "API_KEY" {
    pass_through = true
  }

  assistant_prompt {
    from_string = <<EOS
You have access to weather data. Use for current conditions and forecasts.
EOS
  }
}
```

**Security:** Auto-approved in Slacker (low risk).

### **4. Documents/RAG** (`documents`)
Vector storage and retrieval for document augmentation.

**Use Cases:**
- Knowledge base integration
- Document analysis
- RAG (Retrieval Augmented Generation)
- Code documentation

**Configuration:**
```hcl
documents "knowledge_base" {
  source = "./docs"
  database = "./knowledge.db"

  embedding_model = "nomic-embed-text:latest"

  assistant_prompt {
    from_string = <<EOS
You have access to a knowledge base. Use it to answer questions about...
EOS
  }
}
```

## 🔒 **Security Considerations by Transport**

| Transport | Risk Level | Approval Required | Best For |
|------------|------------|-------------------|-----------|
| `local_program` | High | Admin Approval | Internal tools, system access |
| `docker_mcp` | Medium | Admin Approval | Third-party tools, isolation needed |
| `mcp_over_http` | Low | Auto-approved | Public APIs, cloud services |
| `documents` | Low | Auto-approved | Knowledge bases, documentation |

## 🏗️ **Multi-Tenant Considerations**

### **Slacker Integration**
In multi-tenant Slack environments:

- **Tool Isolation**: Each user has isolated access to approved tools
- **Admin Approval**: Security-sensitive tools require admin review
- **Sharing Controls**: Tools can be shared between authorized users
- **Audit Trail**: All tool usage is logged for security analysis

### **Configuration Patterns**

**Global Tools (Available to All):**
```hcl
mcp_over_http "public_api" "https://api.example.com/mcp"
# No sharing block = available to all users
```

**Restricted Tools (Admin Controlled):**
```hcl
local_program "sensitive_tool" {
  sharing {
    allowed_users = ["U123456789"]  # Specific users only
    can_share = false                # Cannot be shared further
    expires_at = "2025-12-31"        # Time-limited access
  }
}
```

## 📚 **Configuration Examples**

### **Development Environment**
```hcl
local_program "dev_tools" {
  program = "./dev-mcp-server"
  args = ["--workspace", "/home/user/projects"]

  # Auto-approved for development
  sharing {
    allowed_users = ["U123456789", "U098765432"]
    can_share = true
  }
}
```

### **Production Services**
```hcl
docker_mcp "prod_database" "postgres:15" {
  env "DATABASE_URL" {
    value = "postgresql://prod-host:5432/proddb"
  }

  # Strict access control
  sharing {
    allowed_users = ["U123456789"]  # Admin only
    can_share = false
  }

  security_context {
    read_only_root_filesystem = true
    drop_all_capabilities = true
  }
}
```

### **Cloud Integration**
```hcl
mcp_over_http "cloud_api" "https://cloud-provider.com/mcp" {
  env "CLOUD_TOKEN" {
    pass_through = true  # Use environment variable
  }

  # Available to everyone (public API)
  assistant_prompt {
    from_string = <<EOS
You have access to cloud services. Use for infrastructure management...
EOS
  }
}
```

## 🔧 **Advanced Features**

### **Resource Management**
```hcl
docker_mcp "resource_heavy" "tool:latest" {
  resources {
    memory = "2Gi"
    cpu = "2000m"
    gpu = "1"  # If GPU acceleration needed
  }

  limits {
    timeout = "30m"  # Maximum execution time
  }
}
```

### **Health Checks**
```hcl
mcp_over_http "critical_api" "https://api.example.com/mcp" {
  health_check {
    path = "/health"
    interval = "30s"
    timeout = "5s"
    retries = 3
  }
}
```

### **Monitoring & Logging**
```hcl
local_program "monitored_tool" {
  program = "/usr/bin/monitored-server"

  env "LOG_LEVEL" {
    value = "info"
  }

  env "METRICS_ENDPOINT" {
    value = "http://prometheus:9090"
  }

  # Enhanced logging for security
  security_context {
    enable_audit_log = true
    log_file_path = "/var/log/mcp-audit.log"
  }
}
```

## 🚀 **Getting Started**

1. **Choose your transport** based on security requirements
2. **Create configuration** using appropriate syntax
3. **Test locally** before deploying to production
4. **Set up monitoring** for tool usage and performance
5. **Review security settings** for multi-tenant environments

For more examples, see:
- [`marvin.example.hcl`](../marvin.example.hcl) - CLI examples
- [`marvin.slacker.example.hcl`](../marvin.slacker.example.hcl) - Slack bot examples
- [`examples/`](../examples/) - Specific integration examples
