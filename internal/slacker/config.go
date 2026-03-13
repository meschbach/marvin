package slacker

// getNameFromConfig extracts name from tool configuration
func getNameFromConfig(config interface{}) string {
	// Use type assertion safely
	if cfg, ok := config.(interface{ GetName() string }); ok {
		return cfg.GetName()
	}
	return "unknown"
}
