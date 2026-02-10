# Slacker Security Guide

This guide covers security best practices, threat models, and compliance considerations for deploying and operating Slacker in enterprise environments.

## 🛡️ **Security Overview**

Slacker implements a multi-layered security architecture designed for enterprise multi-tenant environments:

### **Core Security Features**
- **Multi-tenant Isolation**: User sessions and tools are completely isolated
- **Admin Approval Workflows**: Security-sensitive tools require admin review
- **Encrypted Credential Storage**: AES-GCM encryption with Argon2 key derivation
- **Comprehensive Audit Logging**: Complete audit trail of all security events
- **Role-based Access Control**: Granular permissions for users and admins
- **Secure Communication**: Socket Mode API with proper token management

---

## 🔐 **Threat Model**

### **Attack Vectors and Mitigations**

| Threat Vector | Risk Level | Mitigation Strategy |
|---------------|------------|-------------------|
| **Credential Theft** | High | Encrypted storage, secure key management |
| **Unauthorized Tool Access** | Medium | Approval workflows, user isolation |
| **Data Exfiltration** | Medium | Tool restrictions, audit logging |
| **Privilege Escalation** | Medium | Role-based access, admin controls |
| **Session Hijacking** | Low | Session isolation, timeout policies |
| **Malicious Tool Injection** | High | Configuration validation, sandboxing |

### **Security Boundaries**

```
┌─────────────────────────────────────────────────────────┐
│                    Slack Workspace                    │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │ User A      │  │ User B      │  │ Admin       │ │
│  │ Session     │  │ Session     │  │ Session     │ │
│  │ Isolation   │  │ Isolation   │  │ Full Access │ │
│  └─────────────┘  └─────────────┘  └─────────────┘ │
├─────────────────────────────────────────────────────────┤
│                 Tool Approval Gateway                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │ HTTP Tools  │  │ Docker Tools│  │ Local Tools │ │
│  │ Auto-approve│  │ Admin Review│  │ Admin Review│ │
│  └─────────────┘  └─────────────┘  └─────────────┘ │
├─────────────────────────────────────────────────────────┤
│                Encrypted Storage                     │
│  ┌─────────────┐  ┌─────────────┐                 │
│  │ Sessions    │  │ Credentials │                 │
│  │ (JSON)      │  │ (AES-GCM)   │                 │
│  └─────────────┘  └─────────────┘                 │
└─────────────────────────────────────────────────────────┘
```

---

## 🔑 **Credential Management**

### **Slack Token Security**

#### **Bot User OAuth Token (xoxb-...)**
- **Purpose**: Bot authentication and API access
- **Risk Level**: High - Provides bot capabilities
- **Protection**: Environment variables, rotation schedule

#### **App Token (xapp-...)**
- **Purpose**: Socket Mode connection
- **Risk Level**: High - Enables real-time communication
- **Protection**: Environment variables, restricted scope

#### **Secure Token Management**

```bash
# ✅ GOOD: Environment variables
export SLACK_BOT_TOKEN=xoxb-your-bot-token-here
export SLACK_APP_TOKEN=xapp-your-app-token-here

# ❌ BAD: Hardcoded in files
# SLACK_BOT_TOKEN=xoxb-token-in-config-file

# ✅ GOOD: Use secret management
export SLACK_BOT_TOKEN=$(aws secretsmanager get-secret-value --secret-id slack-bot-token --query SecretString --output text)
```

#### **Token Rotation Policy**
- **Frequency**: Every 90 days (or per company policy)
- **Process**: Generate new tokens, update environment, revoke old tokens
- **Monitoring**: Track token usage and expiration
- **Emergency**: Immediate rotation if compromise suspected

### **Credential Encryption**

#### **Encryption Architecture**
- **Algorithm**: AES-256-GCM (authenticated encryption)
- **Key Derivation**: Argon2id (memory-hard, resistant to GPU attacks)
- **Salt Management**: Unique salt per installation, securely stored
- **Passphrase Protection**: User-provided passphrase for key encryption

#### **Secure Passphrase Management**

```bash
# ✅ GOOD: Strong passphrase with entropy
export SLACKER_PASSPHRASE=$(openssl rand -base64 32)

# ✅ GOOD: From secure source
export SLACKER_PASSPHRASE=$(vault kv get -field=passphrase secret/slacker)

# ❌ BAD: Weak or predictable passphrase
# export SLACKER_PASSPHRASE="slacker123"
```

#### **Credential Storage Security**

```bash
# Set proper file permissions
chmod 700 credentials/
chmod 600 credentials/*
chmod 600 credentials/.key.salt

# Verify ownership
ls -la credentials/
# Expected: drwx------ (owner only access)
```

---

## 👥 **Access Control**

### **Role-Based Permissions**

#### **Admin Users**
- **Tool Management**: Approve/reject tool requests
- **User Management**: Modify user permissions
- **System Configuration**: Update bot settings
- **Audit Access**: Review all logs and events
- **Emergency Controls**: Shutdown, emergency lockdown

#### **Regular Users**
- **Tool Usage**: Use approved tools within sessions
- **Tool Requests**: Request new tools (subject to approval)
- **Session Management**: Personal conversation history
- **Tool Sharing**: Share permitted tools to other users

#### **Permission Matrix**

| Action | Admin | Regular User |
|--------|-------|--------------|
| **Add HTTP Tools** | ✅ | ✅ |
| **Add Docker Tools** | ✅ | 📋 Request |
| **Add Local Tools** | ✅ | 📋 Request |
| **Approve Tools** | ✅ | ❌ |
| **Share Tools** | ✅ | ⚠️ If permitted |
| **View All Tools** | ✅ | 📋 Own tools only |
| **System Config** | ✅ | ❌ |
| **User Management** | ✅ | ❌ |

### **User Access Control Implementation**

#### **Configuration-Based Access Control**
```hcl
multi_tenant {
  # Admin users with full access
  admin_users = ["U1234567890", "U0987654321"]
  
  # Optional: Admin notification channel
  admin_channel = "CADMIN123"
}

# Tool-specific access control
local_program "sensitive_tool" {
  sharing {
    allowed_users = ["U1234567890"]  # Specific users only
    can_share = false                  # Cannot share further
    expires_at = "2025-12-31"         # Time-limited access
  }
}
```

#### **Dynamic Access Control**
```bash
# Add temporary admin access
./slacker --config marvin.slacker.hcl --add-temp-admin "U1234567890" --duration 24h

# Revoke user access
./slacker --config marvin.slacker.hcl --revoke-user "U1234567890"

# Grant temporary tool access
./slacker --config marvin.slacker.hcl --grant-tool "company-api" --user "U1234567890" --duration 7d
```

---

## 🔍 **Audit and Compliance**

### **Security Event Logging**

#### **Event Categories**
```bash
[SECURITY] Tool Request - User: U123456, Type: local, Command: /usr/bin/ls
[SECURITY] Tool Added - User: U123456, ToolID: user_U123456_github-api, Type: http
[SECURITY] Approval Required - ToolID: local-req-12345, Requester: U123456, Type: local
[SECURITY] Tool Approval - Admin: U789012, ToolID: local-req-12345, Decision: approved
[SECURITY] Tool Rejection - Admin: U789012, ToolID: docker-req-12346, Decision: rejected
[SECURITY] User Access - User: U123456, Action: Login, IP: 192.168.1.100
[SECURITY] Config Change - Admin: U789012, Change: Added admin user U0987654321
[SECURITY] Emergency Action - Admin: U789012, Action: Bot shutdown, Reason: Security incident
```

#### **Log Analysis and Monitoring**

```bash
# Security event summary
grep "\[SECURITY\]" slacker.log | awk '{print $1}' | sort | uniq -c | sort -nr

# Failed access attempts
grep "FAILURE\|REJECTED" slacker.log | tail -20

# Suspicious activity patterns
grep -E "(root|sudo|chmod|chown)" slacker.log | tail -10

# Admin actions review
grep "Admin:" slacker.log | grep "$(date +%Y-%m-%d)"
```

### **Compliance Frameworks**

#### **SOC 2 Type II Considerations**
- **Security**: Tool approval workflows, encryption standards
- **Availability**: Monitoring, backup procedures, disaster recovery
- **Processing Integrity**: Audit logging, change management
- **Confidentiality**: Data encryption, access controls
- **Privacy**: User data handling, data minimization

#### **GDPR Compliance**
- **Data Minimization**: Only collect necessary user data
- **User Rights**: Ability to request data deletion
- **Consent Management**: Clear privacy notices
- **Data Protection**: Encryption at rest and in transit
- **Breach Notification**: Incident response procedures

#### **Industry-Specific Compliance**

```hcl
# Healthcare (HIPAA)
multi_tenant {
  hipaa_mode = true
  audit_retention_days = 2555  # 7 years
  encryption_level = "fips-140-2"
}

# Financial (PCI DSS)
multi_tenant {
  pci_mode = true
  tokenization = true
  audit_retention_days = 365  # 1 year
  access_review_frequency = "quarterly"
}
```

---

## 🛡️ **Security Best Practices**

### **Configuration Security**

#### **Secure Configuration Template**
```hcl
# Security-focused configuration
model = "ministral-3:3b"

multi_tenant {
  admin_users = ["U1234567890"]
  
  # Secure storage paths
  session_store_path = "./sessions"
  credential_store = "./credentials"
  
  # Security settings
  session_timeout = "24h"
  max_session_duration = "72h"
  audit_retention_days = 90
}

# Only allow safe HTTP tools
mcp_over_http "safe_api" "https://trusted-api.example.com/mcp" {
  # Validate URL patterns
  allowed_hosts = ["trusted-api.example.com"]
  
  # Rate limiting
  rate_limit = "10/min"
  
  # Security headers
  security_headers = true
}
```

#### **Configuration Validation**
```bash
# Validate configuration security
./slacker --config marvin.slacker.hcl --security-scan

# Check for common misconfigurations
./slacker --config marvin.slacker.hcl --security-checklist
```

### **Tool Security Guidelines**

#### **Safe Tool Patterns**

```hcl
# ✅ GOOD: Read-only tool with specific purpose
local_program "log_reader" {
  program = "/usr/bin/tail"
  args = ["-n", "100", "/var/log/app.log"]
  
  security_context {
    read_only_filesystem = true
    drop_all_capabilities = true
    user_namespace = true
  }
}

# ✅ GOOD: Containerized tool with resource limits
docker_mcp "api_client" "company/api-client:latest" {
  resources {
    memory = "256Mi"
    cpu = "200m"
    network = "restricted"
  }
  
  security_context {
    read_only_root_filesystem = true
    no_new_privileges = true
    seccomp_profile = "default"
  }
}
```

#### **Dangerous Tool Patterns to Avoid**

```hcl
# ❌ BAD: Unrestricted shell access
local_program "shell" {
  program = "/bin/bash"
  args = ["-i"]  # Interactive shell
}

# ❌ BAD: Network access without restrictions
local_program "network_tool" {
  program = "/usr/bin/ncat"
  args = ["-l", "8080", "-e", "/bin/bash"]
}

# ❌ BAD: System modification tools
local_program "system_admin" {
  program = "/usr/bin/sudo"
  args = ["bash"]
}
```

### **Network Security**

#### **Firewall Configuration**
```bash
# Allow only necessary ports
ufw allow 22/tcp    # SSH
ufw allow 443/tcp   # HTTPS (Slack API)
ufw deny 11434/tcp # Ollama (if local only)
ufw enable

# Monitor network connections
ss -tulpn | grep slacker
```

#### **SSL/TLS Configuration**
```bash
# Enforce HTTPS for external MCP servers
curl https://api.example.com/mcp --tlsv1.2 --ciphers ECDHE-RSA-AES256-GCM-SHA384

# Verify SSL certificates
openssl s_client -connect api.example.com:443 -verify_return_error
```

---

## 🚨 **Incident Response**

### **Security Incident Classification**

#### **High Severity Incidents**
- **Credential Compromise**: Slack tokens exposed or stolen
- **Unauthorized Access**: Non-admin users accessing admin functions
- **Data Breach**: Sensitive data exfiltration through tools
- **System Compromise**: Bot code or configuration tampered

#### **Medium Severity Incidents**
- **Policy Violations**: Users accessing unauthorized tools
- **Configuration Errors**: Security misconfigurations detected
- **Suspicious Activity**: Unusual usage patterns detected

#### **Low Severity Incidents**
- **Failed Access Attempts**: Repeated failed authentication
- **Performance Issues**: Security controls impacting functionality
- **Documentation Gaps**: Security procedures need updates

### **Incident Response Procedures**

#### **Immediate Response (0-1 hour)**

1. **Assessment**
   ```bash
   # Check system status
   ./slacker --status
   
   # Review recent security events
   grep "\[SECURITY\]" slacker.log | tail -50
   
   # Check active sessions
   find ./sessions -name "session_*.json" -mtime -1
   ```

2. **Containment**
   ```bash
   # Emergency bot shutdown
   pkill -INT slacker
   
   # Revoke Slack tokens
   # Go to Slack API console → revoke tokens
   
   # Change admin passwords/passphrases
   export SLACKER_PASSPHRASE=$(openssl rand -base64 32)
   ```

3. **Preservation**
   ```bash
   # Create forensic backup
   tar -czf incident-backup-$(date +%Y%m%d-%H%M%S).tar.gz \
     ./sessions/ ./credentials/ slacker.log marvin.slacker.hcl
   
   # Preserve system state
   ps aux > process-list.txt
   netstat -an > network-connections.txt
   ```

#### **Investigation (1-24 hours)**

1. **Log Analysis**
   ```bash
   # Extract timeline of events
   grep "\[SECURITY\]" slacker.log | \
     awk '{print $1, $2, $6, $7, $8, $9}' | \
     sort -n > incident-timeline.txt
   
   # Identify affected users
   grep -E "(COMPROMISE|UNAUTHORIZED)" slacker.log | \
     awk '{print $4}' | sort | uniq > affected-users.txt
   ```

2. **Impact Assessment**
   - Identify compromised data or systems
   - Assess duration of unauthorized access
   - Determine affected users and data scope
   - Evaluate business impact

3. **Root Cause Analysis**
   - Identify security control failures
   - Analyze attacker methods and objectives
   - Document lessons learned
   - Recommend improvements

#### **Recovery (24-72 hours)**

1. **System Restoration**
   ```bash
   # Restore from clean backup
   tar -xzf clean-backup-YYYYMMDD.tar.gz
   
   # Update Slack tokens
   export SLACK_BOT_TOKEN=xoxb-new-token
   export SLACK_APP_TOKEN=xapp-new-token
   
   # Restart with enhanced monitoring
   ./slacker --config marvin.slacker.hcl --verbose --security-monitor
   ```

2. **Security Hardening**
   - Review and update security configurations
   - Implement additional monitoring
   - Update incident response procedures
   - Conduct security training

3. **Communication**
   - Notify affected users
   - Report to stakeholders
   - Document incident findings
   - Update security policies

---

## 📊 **Security Monitoring**

### **Real-time Monitoring**

#### **Security Metrics Dashboard**
```bash
# Active monitoring script
#!/bin/bash

while true; do
  # Check for security events
  SECURITY_EVENTS=$(grep "\[SECURITY\]" slacker.log | grep "$(date +%Y-%m-%d)" | wc -l)
  
  # Check bot status
  BOT_STATUS=$(pgrep slacker > /dev/null && echo "Running" || echo "Down")
  
  # Check failed logins
  FAILED_ATTEMPTS=$(grep "FAILURE" slacker.log | grep "$(date +%Y-%m-%d)" | wc -l)
  
  echo "$(date): Events=$SECURITY_EVENTS, Status=$BOT_STATUS, Failed=$FAILED_ATTEMPTS"
  sleep 60
done
```

#### **Alert Thresholds**
```bash
# Security alert script
#!/bin/bash

SECURITY_THRESHOLD=50
FAILED_LOGIN_THRESHOLD=10

CURRENT_SECURITY=$(grep "\[SECURITY\]" slacker.log | grep "$(date +%Y-%m-%d)" | wc -l)
CURRENT_FAILED=$(grep "FAILURE" slacker.log | grep "$(date +%Y-%m-%d)" | wc -l)

if [ $CURRENT_SECURITY -gt $SECURITY_THRESHOLD ]; then
  echo "ALERT: High security activity detected: $CURRENT_SECURITY events"
  # Send alert to admin
fi

if [ $CURRENT_FAILED -gt $FAILED_LOGIN_THRESHOLD ]; then
  echo "ALERT: High failed login attempts: $CURRENT_FAILED attempts"
  # Send alert to admin
fi
```

### **Log Analysis and Intelligence**

#### **Security Analytics**
```bash
# Pattern analysis
grep "Tool Request" slacker.log | \
  awk '{print $8}' | sort | uniq -c | sort -nr | head -10

# Risk assessment
grep -E "(root|sudo|chmod|chown)" slacker.log | \
  awk '{print $4, $6, $8}' | sort | uniq

# Time-based analysis
grep "\[SECURITY\]" slacker.log | \
  awk '{print $2}' | cut -d: -f1 | sort | uniq -c
```

---

## 🔒 **Compliance and Auditing**

### **Audit Trail Requirements**

#### **Mandatory Audit Events**
1. **User Authentication**: Login attempts, session creation
2. **Tool Access**: Tool requests, approvals, executions
3. **Configuration Changes**: Admin actions, policy updates
4. **Security Events**: Failed attempts, suspicious activity
5. **Data Access**: File reads, data exports
6. **System Events**: Startup, shutdown, errors

#### **Audit Data Retention**
```hcl
# Retention policy configuration
multi_tenant {
  audit_retention_days = 2555  # 7 years (HIPAA)
  session_retention_days = 90   # 3 months
  credential_retention_days = 365 # 1 year
  
  # Archival policy
  archive_old_logs = true
  archive_format = "gzip"
  archive_location = "/var/log/slacker/archive"
}
```

### **Regulatory Compliance**

#### **SOC 2 Controls**
- **Control Activity-1**: User access review and certification
- **Control Activity-2**: System configuration change management
- **Control Activity-3**: Security incident response procedures
- **Control Activity-4**: Data backup and recovery testing

#### **GDPR Data Protection**
```hcl
# GDPR compliance settings
multi_tenant {
  gdpr_mode = true
  data_minimization = true
  
  # User rights implementation
  allow_data_export = true
  allow_data_deletion = true
  
  # Consent management
  consent_required = true
  consent_storage = "./consent-records"
}
```

---

## 🛠️ **Security Testing**

### **Vulnerability Assessment**

#### **Static Security Analysis**
```bash
# Configuration security scan
./slacker --config marvin.slacker.hcl --security-scan

# Check for common vulnerabilities
./slacker --security-checklist
```

#### **Penetration Testing Scenarios**
1. **Credential Theft Attempts**
2. **Unauthorized Tool Access**
3. **Session Hijacking**
4. **Configuration Tampering**
5. **Data Exfiltration**

### **Security Validation**

#### **Security Test Suite**
```bash
# Run comprehensive security tests
./slacker --security-test --verbose

# Test encryption integrity
./slacker --test-encryption --passphrase "test-passphrase"

# Validate access controls
./slacker --test-access-control
```

---

## 📚 **Additional Resources**

- [Setup Guide](setup.md) - Initial bot configuration
- [Admin Guide](admin-guide.md) - User management and workflows
- [Tool Management Guide](tools-management.md) - Advanced tool configuration
- [Incident Response](../configuration/troubleshooting.md) - Common issues and solutions

## 🆘 **Security Support**

For security issues:
- **Immediate**: Follow incident response procedures
- **Report**: security@company.com
- **Emergency**: Security hotline: +1-XXX-XXX-XXXX

Remember: Security is everyone's responsibility. Stay vigilant, report suspicious activity, and follow established security procedures at all times.