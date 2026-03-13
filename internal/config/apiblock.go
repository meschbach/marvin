package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/meschbach/marvin/internal/junk"
)

// APIKeyBlock provides a standard way to access
type APIKeyBlock struct {
	// APIKey is the key for accessing the API
	APIKey string `hcl:"api_key,optional"`
	// APIKeyFile is a path to a file containing the Gemini API key
	APIKeyFile string `hcl:"api_key_file,optional"`
}

func (a *APIKeyBlock) Resolve(keyName string) (value string, has bool, problem error) {
	if a != nil {
		if a.APIKey != "" {
			return a.APIKey, true, nil
		}
		if a.APIKeyFile != "" {
			data, err := os.ReadFile(a.APIKeyFile)
			if err != nil {
				return "", false, &junk.OperationalError{
					Description: fmt.Sprintf("failed to read key file %s", a.APIKeyFile),
					Underlying:  err,
				}
			}
			return strings.TrimSpace(string(data)), true, nil
		}
	}
	value, has = os.LookupEnv(keyName)
	return value, has, nil
}
