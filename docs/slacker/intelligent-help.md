# Intelligent Help System

Marvin's intelligent help system provides proactive assistance when you encounter issues with commands, tool access, or
model permissions. Instead of generic error messages, you get contextual, personalized help powered by Marvin's AI.

## 🎯 What It Does

The help system automatically assists you in these situations:

- **Command Recognition Failures** - When your command isn't understood
- **Model Access Issues** - When you can't use specific AI models
- **Tool Configuration Errors** - When tool setup fails
- **Tool Permission Denials** - When you don't have access to tools

## 🔧 How It Works

### 1. Smart Command Correction

When you type a command that Marvin doesn't recognize (confidence < 70%), the help system:

```
You: add http tol
🤖 Did you mean "add http tool at <url>"?

You: show my pref
🤖 Did you mean "show preferences"?

You: list my tool
🔧 Command incomplete. Try: "list my tools"
```

### 2. Model Access Help

When you request a model you don't have access to:

```
You: Use llama3.2:70b
🚫 Model 'llama3.2:70b' unavailable for your account
✅ Using 'ministral-3:3b' instead
💡 Available models: llama3.2:latest, qwen2.5:7b
⭐ Admins can grant access: @marvin model allow llama3.2:70b
```

### 3. Tool Setup Guidance

When tool configuration fails:

```
You: add ftp tool at ftp://example.com
❌ 'ftp' is not a supported tool type
✅ Supported types: http, local, docker
💡 Examples:
   • HTTP: add http tool at https://api.example.com/mcp
   • Local: add local program /usr/bin/git
   • Docker: add docker tool nginx:latest
```

### 4. Permission Assistance

When you try to access restricted tools:

```
🔒 Tool 'kubectl' restricted by security policy
📋 Reason: Admin-only tool for cluster operations
🙋 Request access from @admin
💡 Alternative: docker:kubectl (sandboxed version)
```

## 🎛️ Configuration

You can configure the help system in your Marvin configuration file:

```hcl
help_system {
    enabled = true
    confidence_threshold = 0.7      # Trigger help below this confidence
    model = "ministral-3:3b"      # Model for help analysis
    max_context_messages = 5        # Messages to consider for context
    analysis_timeout = 5            # Timeout in seconds

    # Enable help for specific scenarios
    help_on_intent_failure = true           # Command recognition help
    help_on_model_access_denied = true      # Model access help
    help_on_tool_configuration_error = true # Tool setup help
    help_on_tool_permission_denied = true   # Permission help
}
```

## 📊 Available Help Types

### Intent Failure Help
- **Trigger**: Command confidence < 70%
- **Features**:
  - Fuzzy command matching
  - Typo correction
  - Similar command suggestions
  - Context-aware examples

### Model Access Help
- **Trigger**: Model access denied
- **Features**:
  - Transparent denial explanations
  - Available alternatives
  - Admin escalation options
  - Policy information

### Tool Configuration Help
- **Trigger**: Tool setup errors
- **Features**:
  - Supported tool types
  - Correct syntax examples
  - Interactive configuration builders
  - Validation feedback

### Tool Permission Help
- **Trigger**: Access denied to tools
- **Features**:
  - Clear permission explanations
  - Access request workflows
  - Alternative tool suggestions
  - Admin notification

## 🎨 Help Message Formats

### Quick Suggestions
One-line help with emoji indicators:
- `🤖 Did you mean 'command'?`
- `🔧 Try: 'correct format'`
- `💡 Alternative: 'other option'`

### Detailed Examples
Multi-line help with code blocks and explanations:
```
❌ Issue with 'add docker'
💡 **Correct format:** `add docker tool <image>`
📋 **Examples:**
```bash
add docker tool nginx:latest
add docker tool postgres:15
```
```

### Interactive Guidance
Step-by-step assistance for complex scenarios:
1. Analysis of your input
2. Identification of the issue
3. Multiple solution options
4. Interactive choice selection

## 🛠️ Advanced Features

### Context-Aware Analysis
The help system considers:
- Your recent commands (last 5)
- Your available tools and models
- Your user role (admin vs regular user)
- Your personal preferences
- Current session context

### Personalized Suggestions
Help is tailored based on:
- **User Role**: Admins get additional options
- **History**: Frequently used commands prioritized
- **Preferences**: Your preferred help verbosity
- **Context**: Current conversation topic

### Admin Features
Admin users get enhanced help:
- Model access management commands
- User permission modification options
- Bulk tool approval workflows
- System-wide configuration help

## 🚀 Getting Started

The help system is enabled by default. To use it:

1. **Try any command** - If it fails, help appears automatically
2. **Ask for help explicitly** - Use `@marvin help` or `help` command
3. **Configure preferences** - Adjust help verbosity and types
4. **Provide feedback** - Help system learns from successful corrections

## 📈 Tips for Best Results

### For Users
- **Be specific** with your commands
- **Check the suggestions** - they often fix the issue
- **Use examples** as templates for your commands
- **Ask an admin** if you need access to restricted tools

### For Admins
- **Monitor help effectiveness** through metrics
- **Adjust confidence thresholds** based on user feedback
- **Extend help examples** for your specific tools
- **Use permission requests** to manage access grants

## 🔍 Troubleshooting

### Help Not Appearing
- Check `help_system.enabled = true` in config
- Verify confidence threshold isn't too low
- Ensure help type is enabled for your scenario

### Generic Help Responses
- Check LLM model is available and working
- Verify network connectivity to Ollama
- Review help system logs for errors

### Too Many/Too Few Suggestions
- Adjust `confidence_threshold` (0.6-0.8 recommended)
- Modify `max_context_messages` for more/less context
- Tune `analysis_timeout` for complex scenarios

## 📚 Related Documentation

- [Slacker Setup Guide](setup.md) - Basic bot configuration
- [Tools Management](tools-management.md) - Working with tools
- [Security & Permissions](security.md) - Access control
- [Admin Guide](admin-guide.md) - Advanced administration

The intelligent help system makes Marvin more accessible and reduces frustration by turning errors into learning opportunities. If you have suggestions for improving the help system, please share them with your Marvin administrator!