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
