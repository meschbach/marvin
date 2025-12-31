package config

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseHCLString(t *testing.T, input, fileName string) *hcl.File {
	t.Helper()
	p := hclparse.NewParser()
	parseFileContent, diags := p.ParseHCL([]byte(input), fileName)
	if diags.HasErrors() {
		require.NoError(t, diags)
	}
	require.NotNil(t, parseFileContent)
	return parseFileContent
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	cfg, err := interpretConfigFile(parseHCLString(t, "", "empty.hcl"), "/test/"+t.Name())
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.LocalPrograms, 0)
}

func TestLoadConfig_SingleLocalProgram(t *testing.T) {
	hcl := `
local_program "echo" {
  program = "/bin/echo"
}
`
	parsedContent := parseHCLString(t, hcl, t.Name()+".hcl")
	cfg, err := interpretConfigFile(parsedContent, "/test/"+t.Name())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	if assert.Len(t, cfg.LocalPrograms, 1) {
		lp := cfg.LocalPrograms[0]
		assert.Equal(t, "echo", lp.Name)
		assert.Equal(t, "/bin/echo", lp.Program)
		assert.Empty(t, lp.Args)
	}
}

func TestLoadConfig_AllOptionsMultipleBlocks(t *testing.T) {
	hcl := `
local_program "one" {
  program = "/usr/bin/one"
  args    = ["-a", "--flag", "value"]
}

local_program "two" {
  program = "/usr/bin/two"
  args    = ["/p", "q"]
}

local_program "three" {
  program = "/usr/bin/three"
  args    = ["--x", "1", "--y", "2"]
}
`
	parsedContent := parseHCLString(t, hcl, t.Name()+".hcl")
	cfg, err := interpretConfigFile(parsedContent, "/test/"+t.Name())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Validate each block
	cases := []struct {
		name    string
		program string
		args    []string
	}{
		{"one", "/usr/bin/one", []string{"-a", "--flag", "value"}},
		{"two", "/usr/bin/two", []string{"/p", "q"}},
		{"three", "/usr/bin/three", []string{"--x", "1", "--y", "2"}},
	}

	for i, c := range cases {
		require.Less(t, i, len(cfg.LocalPrograms), "missing local program %q at index %d", c.name, i)
		lp := cfg.LocalPrograms[i]
		assert.Equal(t, c.name, lp.Name, "program %d: expected name %q, got %q", i, c.name, lp.Name)
		assert.Equal(t, c.program, lp.Program, "program %d: expected program %q, got %q", i, c.program, lp.Program)
		assert.Len(t, lp.Args, len(c.args), "program %d: expected %d args, got %d", i, len(c.args), len(lp.Args))
		for j := range c.args {
			assert.Equal(t, c.args[j], lp.Args[j], "program %d arg %d: expected %q, got %q", i, j, c.args[j], lp.Args[j])
		}
	}
}
