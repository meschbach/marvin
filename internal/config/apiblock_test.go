package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/meschbach/marvin/internal/junk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nolint:paralleltest
func TestResolve(t *testing.T) {
	withNilButKey := strings.ToUpper(faker.Word()) + "_" + strings.ToUpper(faker.Word())
	withNilButKeyValue := faker.Password()
	tests := []struct {
		name          string
		block         *APIKeyBlock
		keyName       string
		envSetup      func(t *testing.T)
		skipParallel  bool
		expectedValue string
		expectedHas   bool
		expectedErr   bool
		errContains   string
	}{
		{
			name:          "NilReceiver",
			keyName:       "TEST_KEY",
			expectedValue: "",
		},
		{
			name:    "NilWithKey",
			block:   nil,
			keyName: withNilButKey,
			envSetup: func(t *testing.T) {
				t.Setenv(withNilButKey, withNilButKeyValue)
			},
			expectedValue: withNilButKeyValue,
			expectedHas:   true,
			expectedErr:   false,
			skipParallel:  true,
		},
		{
			name: "DirectAPIKey",
			block: &APIKeyBlock{
				APIKey: "my-secret-key",
			},
			keyName:       "TEST_KEY",
			expectedValue: "my-secret-key",
			expectedHas:   true,
			expectedErr:   false,
		},
		{
			name: "APIKeyFile",
			block: &APIKeyBlock{
				APIKeyFile: filepath.Join("testdata", "api_key.txt"),
			},
			keyName:       "TEST_KEY",
			expectedValue: "file-key-123",
			expectedHas:   true,
			expectedErr:   false,
		},
		{
			name: "APIKeyFile_WithWhitespace",
			block: &APIKeyBlock{
				APIKeyFile: filepath.Join("testdata", "api_key_whitespace.txt"),
			},
			keyName:       "TEST_KEY",
			expectedValue: "trimmed-key",
			expectedHas:   true,
			expectedErr:   false,
		},
		{
			name: "APIKeyFile_NotFound",
			block: &APIKeyBlock{
				APIKeyFile: "/nonexistent/path/to/key.txt",
			},
			keyName:     "TEST_KEY",
			expectedHas: false,
			expectedErr: true,
			errContains: "failed to read key file",
		},
		{
			name:    "EnvVarFallback",
			block:   &APIKeyBlock{},
			keyName: "TEST_RESOLVE_ENV_KEY",
			envSetup: func(t *testing.T) {
				t.Setenv("TEST_RESOLVE_ENV_KEY", "env-key-value")
			},
			skipParallel:  true,
			expectedValue: "env-key-value",
			expectedHas:   true,
			expectedErr:   false,
		},
		{
			name: "APIKeyOverFile",
			block: &APIKeyBlock{
				APIKey:     "direct-key",
				APIKeyFile: filepath.Join("testdata", "api_key.txt"),
			},
			keyName:       "TEST_KEY",
			expectedValue: "direct-key",
			expectedHas:   true,
			expectedErr:   false,
		},
		{
			name: "APIKeyOverEnv",
			block: &APIKeyBlock{
				APIKey: "direct-key",
			},
			keyName: "TEST_RESOLVE_API_OVER_ENV",
			envSetup: func(t *testing.T) {
				t.Setenv("TEST_RESOLVE_API_OVER_ENV", "env-key")
			},
			skipParallel:  true,
			expectedValue: "direct-key",
			expectedHas:   true,
			expectedErr:   false,
		},
		{
			name: "FileOverEnv",
			block: &APIKeyBlock{
				APIKeyFile: filepath.Join("testdata", "api_key.txt"),
			},
			keyName: "TEST_RESOLVE_FILE_OVER_ENV",
			envSetup: func(t *testing.T) {
				t.Setenv("TEST_RESOLVE_FILE_OVER_ENV", "env-key")
			},
			skipParallel:  true,
			expectedValue: "file-key-123",
			expectedHas:   true,
			expectedErr:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if !tt.skipParallel {
				t.Parallel()
			}
			if tt.envSetup != nil {
				tt.envSetup(t)
			}

			value, has, err := tt.block.Resolve(tt.keyName)

			if tt.expectedErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				var opErr *junk.OperationalError
				require.ErrorAs(t, err, &opErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedValue, value)
			assert.Equal(t, tt.expectedHas, has)
		})
	}
}
