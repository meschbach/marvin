package slacker

import (
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/config"
)

// ParseToolConfig parses tool configuration based on type
func ParseToolConfig(toolType, config string) (interface{}, error) {
	switch toolType {
	case "http":
		return parseHTTPConfig(config)
	case "local":
		return parseLocalProgramConfig(config)
	case "docker":
		return parseDockerConfig(config)
	default:
		return nil, fmt.Errorf("unsupported tool type: %s", toolType)
	}
}

// parseHTTPConfig parses HTTP tool configuration
func parseHTTPConfig(configStr string) (*config.HttpMCPBlock, error) {
	// Simple parsing - look for URLs
	// Format could be: "https://api.example.com/mcp" or "name at https://api.example.com/mcp"

	var name, url string

	// Check if there's an "at" separator
	if strings.Contains(strings.ToLower(configStr), " at ") {
		parts := strings.SplitN(configStr, " at ", 2)
		if len(parts) == 2 {
			name = strings.TrimSpace(parts[0])
			url = strings.TrimSpace(parts[1])
		}
	} else {
		// Just a URL - generate name from URL
		url = strings.TrimSpace(configStr)
		name = generateNameFromURL(url)
	}

	if url == "" {
		return nil, fmt.Errorf("no URL found in HTTP tool configuration")
	}

	return &config.HttpMCPBlock{
		Name: name,
		URL:  url,
	}, nil
}

// parseLocalProgramConfig parses local program configuration
func parseLocalProgramConfig(configStr string) (*config.LocalProgramBlock, error) {
	// Format could be: "/path/to/program" or "my-tool at /path/to/program" or "program /path/to/program with args"

	var name, program string
	var args []string

	configStr = strings.TrimSpace(configStr)

	// Check if there's an "at" separator
	if strings.Contains(strings.ToLower(configStr), " at ") {
		parts := strings.SplitN(configStr, " at ", 2)
		if len(parts) == 2 {
			name = strings.TrimSpace(parts[0])
			rest := strings.TrimSpace(parts[1])

			// Split rest into program and args
			restParts := strings.Fields(rest)
			if len(restParts) > 0 {
				program = restParts[0]
				args = restParts[1:]
			}
		}
	} else {
		// Just a program path - generate name
		parts := strings.Fields(configStr)
		if len(parts) > 0 {
			program = parts[0]
			args = parts[1:]
			name = generateNameFromPath(program)
		}
	}

	if program == "" {
		return nil, fmt.Errorf("no program path found in local tool configuration")
	}

	return &config.LocalProgramBlock{
		Name:    name,
		Program: program,
		Args:    args,
	}, nil
}

// parseDockerConfig parses Docker tool configuration
func parseDockerConfig(configStr string) (*config.DockerMCPBlock, error) {
	// Format could be: "image:tag" or "name using image:tag" or "image image:tag with args"

	var name, image string
	var args []config.DockerMCPBlockArg

	configStr = strings.TrimSpace(configStr)

	// Check for different separators
	if strings.Contains(strings.ToLower(configStr), " using ") {
		parts := strings.SplitN(configStr, " using ", 2)
		if len(parts) == 2 {
			name = strings.TrimSpace(parts[0])
			rest := strings.TrimSpace(parts[1])

			restParts := strings.Fields(rest)
			if len(restParts) > 0 {
				image = restParts[0]
				for i, arg := range restParts[1:] {
					args = append(args, config.DockerMCPBlockArg{
						Strings: []string{arg},
					})
					if i == 0 { // For now, just take first extra arg as one strings array
						break
					}
				}
			}
		}
	} else if strings.Contains(strings.ToLower(configStr), " image ") {
		parts := strings.SplitN(configStr, " image ", 2)
		if len(parts) == 2 {
			name = strings.TrimSpace(parts[0])
			image = strings.TrimSpace(parts[1])
		}
	} else {
		// Just an image - generate name
		image = configStr
		name = generateNameFromImage(image)
	}

	if image == "" {
		return nil, fmt.Errorf("no Docker image found in Docker tool configuration")
	}

	return &config.DockerMCPBlock{
		Name:  name,
		Image: image,
		Args:  args,
	}, nil
}

// generateNameFromURL generates a tool name from a URL
func generateNameFromURL(url string) string {
	// Remove protocol and extract domain
	parts := strings.Split(url, "://")
	if len(parts) > 1 {
		url = parts[1]
	}

	// Get domain part
	domainParts := strings.Split(url, "/")
	if len(domainParts) > 0 {
		domain := domainParts[0]
		// Remove common prefixes and suffixes
		domain = strings.TrimPrefix(domain, "api.")
		domain = strings.TrimPrefix(domain, "mcp.")
		domain = strings.TrimSuffix(domain, ".com")
		domain = strings.TrimSuffix(domain, ".org")
		domain = strings.TrimSuffix(domain, ".net")
		return domain + "-api"
	}

	return "http-api"
}

// generateNameFromPath generates a tool name from a file path
func generateNameFromPath(path string) string {
	// Get just filename
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		// Remove extension
		if idx := strings.LastIndex(filename, "."); idx > 0 {
			filename = filename[:idx]
		}
		return filename
	}
	return "local-tool"
}

// generateNameFromImage generates a tool name from a Docker image
func generateNameFromImage(image string) string {
	// Remove tag if present
	if idx := strings.LastIndex(image, ":"); idx > 0 {
		image = image[:idx]
	}

	// Get just image name (remove registry)
	parts := strings.Split(image, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		return name
	}

	return "docker-tool"
}

// HandlePreferenceIntent processes preference management commands
func HandlePreferenceIntent(intent *ToolManagementIntent, sessionManager *SessionManager, userID string) (string, error) {
	// Get current preferences
	currentPrefs, hasPrefs := sessionManager.GetPreferences(userID)
	if !hasPrefs {
		currentPrefs = DefaultUserPreferences()
	}

	// Create updated preferences based on intent
	updatedPrefs := currentPrefs
	var responseMessage string

	switch intent.Action {
	case "toggle_thinking":
		configStr, ok := intent.Config.(string)
		if !ok {
			return "🤖 Invalid option specified.", nil
		}
		switch configStr {
		case "on":
			updatedPrefs.ShowThinking = true
			responseMessage = "🤖 Thinking display enabled. Use `/marvin thinking off` to disable."
		case "off":
			updatedPrefs.ShowThinking = false
			responseMessage = "🤖 Thinking display disabled. Use `/marvin thinking on` to re-enable."
		default:
			return "🤖 Please specify `on` or `off` for thinking display.", nil
		}

	case "set_thinking_format":
		configStr, ok := intent.Config.(string)
		if !ok {
			return "🤖 Invalid format specified.", nil
		}
		if configStr == "plain" || configStr == "markdown" || configStr == "collapsed" {
			updatedPrefs.ThinkingFormat = configStr
			responseMessage = fmt.Sprintf("🤖 Thinking format set to %s.", configStr)
		} else {
			return "🤖 Please specify a valid format: `plain`, `markdown`, or `collapsed`.", nil
		}

	case "toggle_tools":
		configStr, ok := intent.Config.(string)
		if !ok {
			return "🔧 Invalid option specified.", nil
		}
		switch configStr {
		case "on":
			updatedPrefs.ShowTools = true
			responseMessage = "🔧 Tool display enabled. Use `/marvin tools off` to disable."
		case "off":
			updatedPrefs.ShowTools = false
			responseMessage = "🔧 Tool display disabled. Use `/marvin tools on` to re-enable."
		default:
			return "🔧 Please specify `on` or `off` for tool display.", nil
		}

	case "toggle_done":
		configStr, ok := intent.Config.(string)
		if !ok {
			return "✅ Invalid option specified.", nil
		}
		switch configStr {
		case "on":
			updatedPrefs.ShowDone = true
			responseMessage = "✅ Completion messages enabled. Use `/marvin done off` to disable."
		case "off":
			updatedPrefs.ShowDone = false
			responseMessage = "✅ Completion messages disabled. Use `/marvin done on` to re-enable."
		default:
			return "✅ Please specify `on` or `off` for completion messages.", nil
		}

	case "toggle_verbose":
		configStr, ok := intent.Config.(string)
		if !ok {
			return "🔍 Invalid option specified.", nil
		}
		switch configStr {
		case "on":
			updatedPrefs.Verbose = true
			responseMessage = "🔍 Verbose mode enabled. Use `/marvin verbose off` to disable."
		case "off":
			updatedPrefs.Verbose = false
			responseMessage = "🔍 Verbose mode disabled. Use `/marvin verbose on` to re-enable."
		default:
			return "🔍 Please specify `on` or `off` for verbose mode.", nil
		}

	case "show_preferences":
		return fmt.Sprintf("🤖 Current preferences:\n• Thinking: %t\n• Tools: %t\n• Done messages: %t\n• Thinking format: %s\n• Tool format: %s\n• Verbose: %t\n\n• Use `/marvin thinking on/off` to toggle thinking\n• Use `/marvin thinking format [plain|markdown|collapsed]` to set format\n• Use `/marvin tools on/off` to toggle tool display\n• Use `/marvin done on/off` to toggle completion messages\n• Use `/marvin verbose on/off` to toggle verbose mode",
			currentPrefs.ShowThinking,
			currentPrefs.ShowTools,
			currentPrefs.ShowDone,
			currentPrefs.ThinkingFormat,
			currentPrefs.ToolFormat,
			currentPrefs.Verbose), nil

	default:
		return "", fmt.Errorf("unknown preference action: %s", intent.Action)
	}

	// Actually persist the changes
	if err := sessionManager.UpdatePreferences(userID, updatedPrefs); err != nil {
		return "", fmt.Errorf("failed to update preferences: %w", err)
	}

	return responseMessage, nil
}
