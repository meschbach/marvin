package query

import (
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadToolsFromConfig_ZeroConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.File{}
	ts, warnings := loadToolsFromConfig(t.Context(), cfg)
	require.NotNil(t, ts)
	assert.Empty(t, warnings)
	assert.Empty(t, ts.Defs)
	assert.Empty(t, ts.ByName)
}

func TestLoadToolsFromConfig_AllInvalidLocalPrograms(t *testing.T) {
	t.Parallel()
	cfg := &config.File{
		LocalPrograms: []config.LocalProgramBlock{
			{
				Name:    "non_existent_program",
				Program: "/this/program/definitely/does/not/exist",
			},
			{
				Name:    "another_invalid",
				Program: "/also/does/not/exist",
			},
		},
	}
	ts, warnings := loadToolsFromConfig(t.Context(), cfg)
	require.NotNil(t, ts)
	assert.Len(t, warnings, 2, "expected warnings for both failed tools")
	assert.Empty(t, ts.Defs, "expected no tools to be registered")
	assert.Empty(t, ts.ByName)
}

func TestLoadToolsFromConfig_PartialFailure(t *testing.T) {
	t.Parallel()
	// This test would need a valid tool to verify success case.
	// Skipping for now as it requires a working MCP server or more complex mocking.
	t.Skip("requires a valid tool fixture")
}
