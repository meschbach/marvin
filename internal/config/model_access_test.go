package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelAccessConfig_Validation tests the model access validation logic
func TestModelAccessConfig_Validation(t *testing.T) {
	tests := []struct {
		name           string
		config         *File
		model          string
		userID         string
		expectedAllow  bool
		expectedReason string
		description    string
	}{
		{
			name: "NoRestrictions_AllowAll",
			config: &File{
				MultiTenant: &MultiTenantBlock{},
			},
			model:          "any-model",
			userID:         "user123",
			expectedAllow:  true,
			expectedReason: "",
			description:    "When no restrictions exist, all models should be allowed",
		},
		{
			name: "DefaultModel_AlwaysAllowed",
			config: &File{
				MultiTenant: &MultiTenantBlock{},
			},
			model:          DefaultLanguageModel,
			userID:         "user123",
			expectedAllow:  true,
			expectedReason: "",
			description:    "Default model should always be allowed",
		},
		{
			name: "Admin_BypassAllRestrictions",
			config: &File{
				MultiTenant: &MultiTenantBlock{
					AdminUsers: []string{"admin123"},
				},
				ModelAccess: &ModelAccessBlock{
					DeniedModels: []string{"restricted-model"},
				},
			},
			model:          "restricted-model",
			userID:         "admin123",
			expectedAllow:  true,
			expectedReason: "",
			description:    "Admin users should bypass all restrictions",
		},
		{
			name: "DeniedList_BlockModel",
			config: &File{
				MultiTenant: &MultiTenantBlock{},
				ModelAccess: &ModelAccessBlock{
					DeniedModels: []string{"restricted-model"},
				},
			},
			model:          "restricted-model",
			userID:         "user123",
			expectedAllow:  false,
			expectedReason: "Model 'restricted-model' is denied by access policy",
			description:    "Models in deny list should be blocked",
		},
		{
			name: "AllowList_OnlyAllowedModels",
			config: &File{
				MultiTenant: &MultiTenantBlock{},
				ModelAccess: &ModelAccessBlock{
					AllowedModels: []string{"allowed-model", "another-allowed"},
				},
			},
			model:          "allowed-model",
			userID:         "user123",
			expectedAllow:  true,
			expectedReason: "",
			description:    "Models in allow list should be permitted",
		},
		{
			name: "AllowList_BlockNotInList",
			config: &File{
				MultiTenant: &MultiTenantBlock{},
				ModelAccess: &ModelAccessBlock{
					AllowedModels: []string{"allowed-model"},
				},
			},
			model:          "not-allowed-model",
			userID:         "user123",
			expectedAllow:  false,
			expectedReason: "Model 'not-allowed-model' is not in allowed list",
			description:    "Models not in allow list should be blocked",
		},
		{
			name: "BothLists_DenyTakesPriority",
			config: &File{
				MultiTenant: &MultiTenantBlock{},
				ModelAccess: &ModelAccessBlock{
					AllowedModels: []string{"conflict-model"},
					DeniedModels:  []string{"conflict-model"},
				},
			},
			model:          "conflict-model",
			userID:         "user123",
			expectedAllow:  false,
			expectedReason: "Model 'conflict-model' is denied by access policy",
			description:    "Deny list should take priority over allow list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, reason := tt.config.ValidateModelAccess(tt.model, tt.userID)

			assert.Equal(t, tt.expectedAllow, allowed, "Allow/deny mismatch")
			assert.Equal(t, tt.expectedReason, reason, "Reason mismatch")
		})
	}
}

// TestModelAccessState_JSONSerialization tests JSON serialization of model access state
func TestModelAccessState_JSONSerialization(t *testing.T) {
	state := &ModelAccessState{
		AllowedModels: []string{"model1", "model2"},
		DeniedModels:  []string{"model3"},
		DefaultModel:  DefaultLanguageModel,
		LastUpdated:   "2025-02-11T10:30:00Z",
		UpdatedBy:     "admin123",
	}

	// Serialize to JSON
	data, err := json.Marshal(state)
	require.NoError(t, err)

	// Deserialize from JSON
	var deserialized ModelAccessState
	err = json.Unmarshal(data, &deserialized)
	require.NoError(t, err)

	// Verify equality
	assert.Equal(t, state.AllowedModels, deserialized.AllowedModels)
	assert.Equal(t, state.DeniedModels, deserialized.DeniedModels)
	assert.Equal(t, state.DefaultModel, deserialized.DefaultModel)
	assert.Equal(t, state.LastUpdated, deserialized.LastUpdated)
	assert.Equal(t, state.UpdatedBy, deserialized.UpdatedBy)
}

// TestModelAccessState_SaveLoad tests saving and loading model access state
func TestModelAccessState_SaveLoad(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create config with slacker state path
	config := &File{
		MultiTenant: &MultiTenantBlock{
			SlackerStatePath: tempDir,
		},
	}

	// Create test state
	originalState := &ModelAccessState{
		AllowedModels: []string{"allowed1", "allowed2"},
		DeniedModels:  []string{"denied1"},
		DefaultModel:  DefaultLanguageModel,
		LastUpdated:   "",
		UpdatedBy:     "",
	}

	// Save state
	err = config.SaveModelAccessState(originalState, "admin123")
	require.NoError(t, err)

	// Load state
	loadedState, err := config.LoadModelAccessState()
	require.NoError(t, err)
	require.NotNil(t, loadedState)

	// Verify loaded state (except for timestamps)
	assert.Equal(t, originalState.AllowedModels, loadedState.AllowedModels)
	assert.Equal(t, originalState.DeniedModels, loadedState.DeniedModels)
	assert.Equal(t, originalState.DefaultModel, loadedState.DefaultModel)
	assert.Equal(t, "admin123", loadedState.UpdatedBy)
	assert.NotEmpty(t, loadedState.LastUpdated)

	// Verify timestamp format
	_, err = time.Parse(time.RFC3339, loadedState.LastUpdated)
	assert.NoError(t, err)
}

// TestModelAccessState_NoStateFile tests loading when no state file exists
func TestModelAccessState_NoStateFile(t *testing.T) {
	// Create config with non-existent slacker state path
	config := &File{
		MultiTenant: &MultiTenantBlock{
			SlackerStatePath: "/non/existent/path",
		},
	}

	// Load state should return nil, nil when file doesn't exist
	state, err := config.LoadModelAccessState()
	assert.NoError(t, err)
	assert.Nil(t, state)
}

// TestModelAccessState_NoSlackerStatePath tests loading when no slacker state path is configured
func TestModelAccessState_NoSlackerStatePath(t *testing.T) {
	// Create config without slacker state path
	config := &File{}

	// Load state should return nil, nil when no path is configured
	state, err := config.LoadModelAccessState()
	assert.NoError(t, err)
	assert.Nil(t, state)
}

// TestGetEffectiveModelAccess tests priority order of model access configuration
func TestGetEffectiveModelAccess(t *testing.T) {
	// Create temporary directory for state
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("StateOverridesHCL", func(t *testing.T) {
		config := &File{
			MultiTenant: &MultiTenantBlock{
				SlackerStatePath: tempDir,
			},
			ModelAccess: &ModelAccessBlock{
				AllowedModels: []string{"hcl-allowed"},
				DeniedModels:  []string{"hcl-denied"},
			},
		}

		// Create different state configuration
		stateConfig := &ModelAccessState{
			AllowedModels: []string{"state-allowed"},
			DeniedModels:  []string{"state-denied"},
			DefaultModel:  DefaultLanguageModel,
		}

		// Save state
		err = config.SaveModelAccessState(stateConfig, "admin123")
		require.NoError(t, err)

		// Get effective configuration
		effective, err := config.GetEffectiveModelAccess()
		require.NoError(t, err)

		// State should override HCL
		assert.Equal(t, []string{"state-allowed"}, effective.AllowedModels)
		assert.Equal(t, []string{"state-denied"}, effective.DeniedModels)
	})

	t.Run("HCLFallback", func(t *testing.T) {
		config := &File{
			MultiTenant: &MultiTenantBlock{
				SlackerStatePath: "/non/existent/path", // No state file
			},
			ModelAccess: &ModelAccessBlock{
				AllowedModels: []string{"hcl-allowed"},
				DeniedModels:  []string{"hcl-denied"},
			},
		}

		// Get effective configuration
		effective, err := config.GetEffectiveModelAccess()
		require.NoError(t, err)

		// Should fall back to HCL
		assert.Equal(t, []string{"hcl-allowed"}, effective.AllowedModels)
		assert.Equal(t, []string{"hcl-denied"}, effective.DeniedModels)
	})

	t.Run("NoConfiguration", func(t *testing.T) {
		config := &File{
			MultiTenant: &MultiTenantBlock{
				SlackerStatePath: "/non/existent/path", // No state file
			},
			// No ModelAccess block
		}

		// Get effective configuration
		effective, err := config.GetEffectiveModelAccess()
		require.NoError(t, err)

		// Should return empty configuration (allow all)
		assert.Empty(t, effective.AllowedModels)
		assert.Empty(t, effective.DeniedModels)
		assert.Equal(t, DefaultLanguageModel, effective.DefaultModel)
	})
}

// TestModelAccessConfig_AtomicSave tests that state files are written atomically
func TestModelAccessConfig_AtomicSave(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := &File{
		MultiTenant: &MultiTenantBlock{
			SlackerStatePath: tempDir,
		},
	}

	state := &ModelAccessState{
		AllowedModels: []string{"test-model"},
		DeniedModels:  []string{},
		DefaultModel:  DefaultLanguageModel,
	}

	// Save state
	err = config.SaveModelAccessState(state, "admin123")
	require.NoError(t, err)

	// Verify file exists and is properly named
	stateFile := filepath.Join(tempDir, "model-access.json")
	_, err = os.Stat(stateFile)
	assert.NoError(t, err)

	// Verify temp file doesn't exist
	tempFile := stateFile + ".tmp"
	_, err = os.Stat(tempFile)
	assert.True(t, os.IsNotExist(err))

	// Verify content is valid JSON
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)

	var loadedState ModelAccessState
	err = json.Unmarshal(data, &loadedState)
	require.NoError(t, err)

	assert.Equal(t, state.AllowedModels, loadedState.AllowedModels)
}
