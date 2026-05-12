# Slacker Tool Management Guide

This guide covers advanced tool management, configuration patterns, and operational procedures for managing MCP tools
in Slacker's multi-tenant environment.

## 🎯 **Tool Management Overview**

Slacker supports three types of MCP tools, each with different security characteristics and use cases:

| Tool Type | Approval Required | Risk Level | Typical Use Cases |
|------------|-------------------|------------|-------------------|
| **HTTP MCP** | No approval | Low | Public APIs, cloud services, web-based tools |
| **Docker MCP** | Admin approval | Medium | Third-party services, isolated execution |
| **Local Program** | Admin approval | High | System utilities, internal tools, file access |

---

## 🌐 **HTTP MCP Tools**

### **Characteristics**
- **Auto-approved**: No admin approval required
- **Remote execution**: Tools run on external servers
- **Low risk**: Limited access to local resources
- **Network dependent**: Requires internet connectivity

### **Configuration Examples**

#### **Basic HTTP Tool**
```hcl
mcp_over_http "weather_api" "https://weather.example.com/mcp" {
  assistant_prompt {
    from_string = <<EOS
You have access to weather data via HTTP API. Use this for:
- Current weather conditions
- Weather forecasts
- Historical weather data
Always specify location when asking for weather information.
EOS
  }
}
```

#### **HTTP Tool with Authentication**
```hcl
mcp_over_http "github_api" "https://api.github.com/mcp" {
  env "API_TOKEN" {
    pass_through = true  # Use system environment variable
  }

  env "RATE_LIMIT" {
    value = "1000/hour"
  }

  assistant_prompt {
    from_string = <<EOS
You have access to GitHub API. Use this for:
- Repository information and statistics
- Issue and pull request tracking
- Code search and analysis
Respect rate limits and privacy settings.
EOS
  }
}
```

#### **Secured HTTP Tool**
```hcl
mcp_over_http "secure_api" "https://secure-api.company.com/mcp" {
  env "API_KEY" {
    pass_through = true
  }

  env "CERT_PATH" {
    value = "/etc/ssl/certs/company-ca.pem"
  }

  security_context {
    verify_ssl = true
    ssl_version = "TLSv1.3"
    timeout = "30s"
  }
}
```

### **HTTP Tool Best Practices**

#### **Security Considerations**
- ✅ Use HTTPS endpoints only
- ✅ Validate SSL certificates
- ✅ Implement rate limiting
- ✅ Use secure authentication methods
- ❌ Avoid HTTP endpoints without encryption

#### **Performance Optimization**
```hcl
mcp_over_http "optimized_api" "https://api.example.com/mcp" {
  env "CONNECTION_POOL" {
    value = "10"
  }

  env "TIMEOUT" {
    value = "15s"
  }

  env "RETRY_ATTEMPTS" {
    value = "3"
  }

  health_check {
    path = "/health"
    interval = "60s"
    timeout = "5s"
  }
}
```

---

## 🐳 **Docker MCP Tools**

### **Characteristics**
- **Admin approval required**: Security review needed
- **Containerized execution**: Isolated environment
- **Resource controlled**: CPU/memory limits available
- **Medium risk**: More controlled than local programs

### **Configuration Examples**

#### **Basic Docker Tool**
```hcl
docker_mcp "postgres_tools" "postgres:15" {
  env "DATABASE_URL" {
    value = "postgresql://user:password@localhost:5432/db"
  }

  env "PGPASSWORD" {
    pass_through = true  # Use system environment
  }

  assistant_prompt {
    from_string = <<EOS
You have access to a PostgreSQL database. Use for:
- SQL queries and data analysis
- Database administration tasks
- Schema inspection and management
Always use transactions for multi-step operations.
EOS
  }
}
```

#### **Docker Tool with Resource Limits**
```hcl
docker_mcp "resource_heavy" "company/tool:latest" {
  resources {
    memory = "2Gi"
    cpu = "2000m"
    gpu = "1"  # If GPU acceleration needed
  }

  limits {
    timeout = "30m"
    restart_policy = "on-failure"
    max_restarts = 3
  }

  env "LOG_LEVEL" {
    value = "info"
  }
}
```

#### **Docker Tool with Volume Mounts**
```hcl
docker_mcp "file_processor" "company/file-processor:latest" {
  mount "/app/data" "/host/data" {
    description = "Data processing directory"
    read_only = false
  }

  mount "/app/config" "/etc/tool-config" {
    description = "Configuration files"
    read_only = true
  }

  security_context {
    read_only_root_filesystem = true
    drop_all_capabilities = true
    no_new_privileges = true
  }
}
```

### **Docker Tool Security**

#### **Security Context Configuration**
```hcl
docker_mcp "secure_container" "company/secure-tool:latest" {
  security_context {
    # Filesystem security
    read_only_root_filesystem = true

    # Capability dropping
    drop_all_capabilities = true
    add_capabilities = ["NET_BIND_SERVICE"]

    # User management
    run_as_user = "1000"
    run_as_group = "1000"

    # Network isolation
    network_mode = "bridge"

    # Seccomp profile
    seccomp_profile = "default"

    # AppArmor profile (if supported)
    apparmor_profile = "docker-default"
  }

  env "SECURITY_LEVEL" {
    value = "high"
  }
}
```

#### **Resource Isolation**
```hcl
docker_mcp "isolated_tool" "company/isolated-tool:latest" {
  resources {
    memory = "512Mi"
    cpu = "500m"

    # Disk limits
    disk_quota = "1Gi"
    disk_inodes = "10000"
  }

  limits {
    timeout = "10m"
    max_file_size = "100Mi"
  }

  network {
    disabled = false
    allowed_ports = ["443", "80"]
    blocked_ips = ["169.254.0.0/16"]  # Block metadata service
  }
}
```

---

## 💻 **Local Program Tools**

### **Characteristics**
- **Admin approval required**: Highest security review needed
- **Direct system access**: Full access to host system
- **High risk**: Potential for system impact
- **Best performance**: No container overhead

### **Configuration Examples**

#### **Basic Local Tool**
```hcl
local_program "file_manager" {
  program = "/usr/local/bin/file-mcp"
  args = ["--read-only", "--secure"]

  env "HOME_DIR" {
    value = "/home/user"
  }

  assistant_prompt {
    from_string = <<EOS
You have access to file system operations. Use for:
- Reading and viewing files
- Directory listings and search
- File analysis and processing
Be cautious with any destructive operations.
EOS
  }
}
```

#### **Local Tool with Security Context**
```hcl
local_program "secure_utility" {
  program = "/usr/local/bin/secure-tool"
  args = ["--sandbox", "--no-network"]

  security_context {
    # User isolation
    run_as_user = "nobody"
    run_as_group = "nogroup"

    # Filesystem restrictions
    chroot_directory = "/var/lib/tool-sandbox"
    read_only_filesystem = true

    # Capability dropping
    drop_capabilities = ["CAP_SYS_ADMIN", "CAP_NET_ADMIN"]

    # Resource limits
    max_memory = "256Mi"
    max_cpu_time = "30s"
    max_processes = 10
  }

  env "SECURITY_MODE" {
    value = "strict"
  }
}
```

#### **Local Tool with Shared Access**
```hcl
local_program "company_git" {
  program = "/usr/local/bin/git-mcp"
  args = ["--read-only", "--company-repo"]

  sharing {
    allowed_users = ["U1234567890", "U0987654321"]
    can_share = false
    expires_at = "2025-12-31T23:59:59Z"
  }

  assistant_prompt {
    from_string = <<EOS
You have access to company Git repositories. Use for:
- Code review and analysis
- Repository history and statistics
- Issue and pull request tracking
Always respect company policies and data sensitivity.
EOS
  }
}
```

---

## 🔧 **Tool Sharing and Permissions**

### **Sharing Configuration**

#### **Tool Sharing Basics**
```hcl
local_program "shared_tool" {
  program = "/usr/local/bin/shared-utility"

  sharing {
    # Who can use this tool
    allowed_users = ["U1234567890", "U0987654321"]

    # Can users share it further?
    can_share = false

    # When does access expire?
    expires_at = "2025-12-31T23:59:59Z"

    # Sharing restrictions
    max_recipients = 5
    require_approval = false
  }
}
```

#### **Hierarchical Access Control**
```hcl
# Admin-level tool
local_program "admin_tool" {
  program = "/usr/local/bin/admin-utility"

  sharing {
    allowed_users = ["U1234567890"]  # Only primary admin
    can_share = false
    expires_at = "2025-12-31T23:59:59Z"
  }
}

# Team-level tool
local_program "team_tool" {
  program = "/usr/local/bin/team-utility"

  sharing {
    allowed_users = ["U1234567890", "U0987654321", "U1111111111"]  # Team members
    can_share = true
    expires_at = "2025-12-31T23:59:59Z"
  }
}

# Public tool (available to all)
mcp_over_http "public_api" "https://api.example.com/mcp"
# No sharing block = available to all users
```

### **Natural Language Tool Management**

#### **Adding Tools (User Commands)**

```bash
# Add HTTP tool (auto-approved)
@marvin-bot Add HTTP MCP server at https://weather.example.com/mcp

# Add Docker tool (requires approval)
@marvin-bot Add docker tool nginx

# Add local tool (requires approval)
@marvin-bot Add local tool at /usr/local/bin/my-tool with args "--read-only"

# Add tool with specific configuration
@marvin-bot Add docker tool company/api:latest with env API_KEY=prod-key
```

#### **Tool Management Commands**

```bash
# List available tools
@marvin-bot List my tools

# List all tools (admin only)
@marvin-bot List all tools

# Get tool details
@marvin-bot Show weather-api tool details

# Share tool with another user
@marvin-bot Share weather-api with @john.doe

# Remove tool from session
@marvin-bot Remove weather-api

# Tool usage help
@marvin-bot How do I use the github-api tool?
```

#### **Admin Tool Management**

```bash
# Approve pending request
Approve user-123-local-20260210-150405

# Reject with reason
Reject user-123-docker-20260210-150410 because insufficient documentation

# Batch approve
Approve user-123-local-20260210-150405 user-123-docker-20260210-150410

# List pending approvals
@marvin-bot Show pending approvals

# Revoke tool access
@marvin-bot Revoke access to weather-api for user U123456

# Update tool permissions
@marvin-bot Allow sharing of company-tool for user U123456
```

---

## 📊 **Tool Lifecycle Management**

### **Tool Approval Workflow**

#### **Request Phase**
1. **User Request**: Natural language command
2. **Intent Parsing**: Extract tool configuration
3. **Validation**: Check syntax and safety
4. **Risk Assessment**: Determine approval requirement
5. **Request Storage**: Save with unique ID
6. **Admin Notification**: Send approval request

#### **Review Phase**
1. **Admin Notification**: Detailed request information
2. **Security Review**: Evaluate risks and benefits
3. **Configuration Check**: Validate tool settings
4. **Business Justification**: Confirm legitimate need
5. **Decision**: Approve or reject with reasoning

#### **Implementation Phase**
1. **Approval Processing**: Update tool status
2. **User Notification**: Inform requester of decision
3. **Tool Activation**: Add to user's session
4. **Access Control**: Implement sharing restrictions
5. **Audit Logging**: Record all actions

### **Tool Maintenance**

#### **Regular Reviews**
```bash
# Generate tool usage report
@marvin-bot Generate tool usage report for last 30 days

# Review active tools
@marvin-bot Show tools approaching expiration

# Audit tool permissions
@marvin-bot Audit user tool permissions

# Cleanup inactive tools
@marvin-bot Remove tools unused for 90 days
```

#### **Tool Updates**
```hcl
# Version-controlled tool updates
docker_mcp "api_client" "company/api-client:v2.1.0" {
  version = "2.1.0"
  update_policy = "manual"  # manual, auto, rollback

  env "API_VERSION" {
    value = "v2.1.0"
  }

  # Rollback configuration
  rollback_version = "2.0.5"
  rollback_policy = "on-failure"
}
```

---

## 🎛️ **Advanced Configuration**

### **Tool Dependencies**

#### **Inter-tool Dependencies**
```hcl
# Database tool that depends on authentication
local_program "database_client" {
  program = "/usr/local/bin/db-client"

  # Dependencies
  depends_on = ["auth_service"]

  env "AUTH_SERVICE_URL" {
    value = "http://localhost:8080"
  }
}

# Authentication service
mcp_over_http "auth_service" "http://auth.company.com/mcp"
```

#### **Conditional Tool Loading**
```hcl
# Environment-based tool selection
local_program "dev_tool" {
  program = "/usr/local/bin/dev-utility"

  condition {
    environment = "development"
    user_groups = ["developers"]
    time_window = "09:00-17:00"
  }
}

local_program "prod_tool" {
  program = "/usr/local/bin/prod-utility"

  condition {
    environment = "production"
    user_groups = ["admins", "ops"]
    require_approval = true
  }
}
```

### **Tool Health Monitoring**

#### **Health Check Configuration**
```hcl
mcp_over_http "monitored_api" "https://api.example.com/mcp" {
  health_check {
    path = "/health"
    method = "GET"
    interval = "60s"
    timeout = "5s"
    retries = 3

    success_criteria {
      status_code = 200
      response_time = "< 2s"
      content_check = "status: ok"
    }
  }

  failure_action {
    on_failure = "disable_tool"
    notification_channel = "CADMIN"
    retry_after = "5m"
  }
}
```

#### **Performance Monitoring**
```hcl
docker_mcp "performance_tool" "company/performance:latest" {
  monitoring {
    enable_metrics = true
    metrics_endpoint = "/metrics"
    collection_interval = "30s"

    alerts {
      response_time_threshold = "5s"
      error_rate_threshold = "5%"
      memory_usage_threshold = "80%"
    }
  }

  resources {
    memory = "1Gi"
    cpu = "1000m"

    limits {
      max_memory = "1.5Gi"
      max_cpu = "2000m"
    }
  }
}
```

---

## 🔍 **Tool Usage Analytics**

### **Usage Tracking**

#### **User Analytics**
```bash
# Tool usage by user
grep "Tool Execution" slacker.log | \
  awk '{print $4, $6}' | sort | uniq -c

# Popular tools ranking
grep "Tool Execution" slacker.log | \
  awk '{print $6}' | sort | uniq -c | sort -nr

# Time-based usage patterns
grep "Tool Execution" slacker.log | \
  awk '{print $2}' | cut -d: -f1 | sort | uniq -c
```

#### **Performance Metrics**
```bash
# Response time analysis
grep "Tool Execution.*Duration" slacker.log | \
  awk '{print $8}' | sort -n

# Error rate analysis
grep "Tool Execution.*ERROR" slacker.log | \
  awk '{print $6}' | sort | uniq -c | sort -nr

# Resource usage monitoring
docker stats --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}"
```

### **Optimization Recommendations**

#### **Tool Performance Tuning**
```hcl
# Optimized HTTP tool
mcp_over_http "fast_api" "https://api.example.com/mcp" {
  # Connection pooling
  env "MAX_CONNECTIONS" {
    value = "20"
  }

  # Caching
  env "CACHE_TTL" {
    value = "300"  # 5 minutes
  }

  # Compression
  env "ENABLE_COMPRESSION" {
    value = "true"
  }

  # Timeout optimization
  env "CONNECT_TIMEOUT" {
    value = "5s"
  }

  env "READ_TIMEOUT" {
    value = "30s"
  }
}
```

#### **Resource Optimization**
```hcl
docker_mcp "optimized_container" "company/tool:latest" {
  # Resource limits based on usage data
  resources {
    memory = "512Mi"    # Reduced from 1Gi based on actual usage
    cpu = "500m"        # Reduced from 1000m
  }

  # Restart policy
  restart_policy = "unless-stopped"
  max_restarts = 3

  # Cleanup
  cleanup_policy {
    remove_containers = true
    remove_images = false
    cleanup_interval = "24h"
  }
}
```

---

## 🚨 **Troubleshooting**

### **Common Tool Issues**

#### **Tool Connection Problems**
```bash
# Symptoms: Tool timeout, connection refused
# Diagnosis:
@marvin-bot Test connection to weather-api

# Solutions:
1. Check network connectivity
2. Verify API endpoint is accessible
3. Confirm authentication credentials
4. Check rate limiting
```

#### **Tool Permission Issues**
```bash
# Symptoms: Access denied, permission errors
# Diagnosis:
@marvin-bot Show my permissions for company-tool

# Solutions:
1. Verify user is in allowed_users list
2. Check if tool sharing is permitted
3. Confirm tool hasn't expired
4. Contact admin for permission update
```

#### **Docker Tool Issues**
```bash
# Symptoms: Container startup failure, resource limits
# Diagnosis:
docker logs <container_id>
docker inspect <container_id>

# Solutions:
1. Check Docker image availability
2. Verify resource limits
3. Confirm volume mount paths
4. Review security context settings
```

### **Debug Mode**

#### **Enable Debug Logging**
```bash
# Enable verbose logging for troubleshooting
./slacker --config marvin.slacker.hcl --verbose --debug-tools

# Test tool configuration
@marvin-bot Test configuration for weather-api

# Validate tool setup
@marvin-bot Validate all my tools
```

#### **Tool Diagnostics**
```bash
# Run comprehensive tool diagnostics
@marvin-bot Run tool diagnostics

# Check specific tool health
@marvin-bot Check health of github-api

# Get detailed tool information
@marvin-bot Show detailed info for postgres-tools
```

---

## 📚 **Best Practices**

### **Security Best Practices**
1. **Principle of Least Privilege**: Grant minimum necessary permissions
2. **Regular Audits**: Review tool access and usage regularly
3. **Secure Defaults**: Use secure configurations by default
4. **Monitoring**: Implement comprehensive logging and monitoring
5. **Training**: Educate users on security best practices

### **Performance Best Practices**
1. **Resource Monitoring**: Track resource usage and optimize
2. **Connection Pooling**: Reuse connections for HTTP tools
3. **Caching**: Implement caching where appropriate
4. **Timeout Management**: Set appropriate timeouts
5. **Load Testing**: Test tools under load conditions

### **Operational Best Practices**
1. **Documentation**: Document all tool configurations
2. **Version Control**: Track tool configuration changes
3. **Backup Policies**: Backup tool configurations and data
4. **Disaster Recovery**: Plan for tool outages
5. **User Support**: Provide clear user guidance and support

---

## 🔄 **Continuous Improvement**

### **Feedback Loop**
1. **User Feedback**: Collect user experience and suggestions
2. **Usage Analysis**: Analyze tool usage patterns
3. **Performance Review**: Regular performance assessments
4. **Security Review**: Ongoing security evaluations
5. **Tool Optimization**: Continuous tool improvement

### **Evolution Strategy**
1. **Tool Lifecycle Management**: Plan tool updates and retirement
2. **Technology Assessment**: Evaluate new tool technologies
3. **Process Improvement**: Streamline management processes
4. **Scalability Planning**: Plan for growth and expansion
5. **Innovation**: Explore new tool capabilities and integrations

---

## 📖 **Additional Resources**

- [Setup Guide](setup.md) - Initial bot configuration
- [Admin Guide](admin-guide.md) - User management and workflows
- [Security Guide](security.md) - Security best practices
- [MCP Documentation](../mcp.md) - Transport options and configuration
- [Troubleshooting Guide](../configuration/troubleshooting.md) - Common issues and solutions

Remember: Effective tool management is key to Slacker's success. Focus on security, performance, and user experience to create a productive and secure environment.