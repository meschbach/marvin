package slacker

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// ParseToolConfig parses a tool configuration string and returns a config map
func ParseToolConfig(toolType, config string) (interface{}, error) {
	configMap := make(map[string]interface{})

	switch strings.ToLower(toolType) {
	case "http":
		if config == "" {
			return nil, fmt.Errorf("HTTP tool requires a URL configuration")
		}
		configMap["url"] = strings.TrimSpace(config)
		return configMap, nil
	case "docker":
		if config == "" {
			return nil, fmt.Errorf("Docker tool requires a container name configuration")
		}
		configMap["container"] = strings.TrimSpace(config)
		return configMap, nil
	case "local":
		if config == "" {
			return nil, fmt.Errorf("local tool requires a command path configuration")
		}
		configMap["command"] = strings.TrimSpace(config)
		return configMap, nil
	default:
		return nil, fmt.Errorf("unsupported tool type: %s", toolType)
	}
}

// GenerateToolID generates a unique ID for a tool based on user, type, and name
func GenerateToolID(userID, toolType, name string) string {
	hash := sha256.New()
	hash.Write([]byte(userID))
	hash.Write([]byte(toolType))
	hash.Write([]byte(name))
	return fmt.Sprintf("%s-%s-%x", toolType, name, hash.Sum(nil)[:4])
}

// GenerateApprovalRequestID generates a unique ID for an approval request
func GenerateApprovalRequestID(toolType, requesterID string) string {
	hash := sha256.New()
	hash.Write([]byte(toolType))
	hash.Write([]byte(requesterID))
	hash.Write([]byte(fmt.Sprintf("%d", 0))) // Use a counter or timestamp in production
	return fmt.Sprintf("approval-%s-%x", toolType, hash.Sum(nil)[:6])
}
