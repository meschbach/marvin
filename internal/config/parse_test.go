package config

import (
	"os"
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
	t.Parallel()
	cfg, err := interpretConfigFile(parseHCLString(t, "", "empty.hcl"), t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Empty(t, cfg.LocalPrograms)
}

func TestLoadConfig_SingleLocalProgram(t *testing.T) {
	t.Parallel()
	exampleHCLFile := `
local_program "echo" {
  program = "/bin/echo"
}
`
	parsedContent := parseHCLString(t, exampleHCLFile, t.Name()+".exampleHCLFile")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	if assert.Len(t, cfg.LocalPrograms, 1) {
		lp := cfg.LocalPrograms[0]
		assert.Equal(t, "echo", lp.Name)
		assert.Equal(t, "/bin/echo", lp.Program)
		assert.Empty(t, lp.Args)
	}
}

// todo: fix duplication
// nolint
func TestLoadConfig_LocalProgramWithAssistantPrompt(t *testing.T) {
	t.Parallel()
	exampleHCLFile := `
local_program "git" {
  program = "/usr/bin/git"
  
  assistant_prompt {
    from_string = "Use this git tool for version control operations."
  }
}
`
	parsedContent := parseHCLString(t, exampleHCLFile, t.Name()+".exampleHCLFile")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	if assert.Len(t, cfg.LocalPrograms, 1) {
		lp := cfg.LocalPrograms[0]
		assert.Equal(t, "git", lp.Name)
		assert.Equal(t, "/usr/bin/git", lp.Program)
		assert.NotNil(t, lp.AssistantPrompt)
		assert.Equal(t, "Use this git tool for version control operations.", lp.AssistantPrompt.FromString)
		assert.Empty(t, lp.AssistantPrompt.FromFile)
	}
}

// todo: fix duplication
// nolint
func TestLoadConfig_LocalProgramWithAssistantPromptFromFile(t *testing.T) {
	t.Parallel()
	exmapleFileContent := `
local_program "git" {
  program = "/usr/bin/git"
  
  assistant_prompt {
    from_file = "prompts/git.txt"
  }
}
`
	parsedContent := parseHCLString(t, exmapleFileContent, t.Name()+".exmapleFileContent")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	if assert.Len(t, cfg.LocalPrograms, 1) {
		lp := cfg.LocalPrograms[0]
		assert.Equal(t, "git", lp.Name)
		assert.Equal(t, "/usr/bin/git", lp.Program)
		assert.NotNil(t, lp.AssistantPrompt)
		assert.Equal(t, "prompts/git.txt", lp.AssistantPrompt.FromFile)
		assert.Empty(t, lp.AssistantPrompt.FromString)
	}
}

// todo: fix duplication
// nolint
func TestLoadConfig_AllOptionsMultipleBlocks(t *testing.T) {
	t.Parallel()
	exampleFileContent := `
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
	parsedContent := parseHCLString(t, exampleFileContent, t.Name()+".exampleFileContent")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
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

func TestBuildAPIOptions_NoOptions(t *testing.T) {
	t.Parallel()
	cfg := &File{}
	result := cfg.BuildAPIOptions()
	assert.Nil(t, result, "should return nil when no options block")
}

func TestBuildAPIOptions_NilOptions(t *testing.T) {
	t.Parallel()
	cfg := &File{Options: nil}
	result := cfg.BuildAPIOptions()
	assert.Nil(t, result, "should return nil when options block is nil")
}

func TestBuildAPIOptions_EmptyOptions(t *testing.T) {
	t.Parallel()
	cfg := &File{Options: &ModelOptionsBlock{}}
	result := cfg.BuildAPIOptions()
	assert.Nil(t, result, "should return nil when all option fields are nil/empty")
}

func TestBuildAPIOptions_SingleOption(t *testing.T) {
	t.Parallel()
	temp := float32(0.7)
	cfg := &File{Options: &ModelOptionsBlock{
		Temperature: &temp,
	}}
	result := cfg.BuildAPIOptions()
	require.NotNil(t, result)
	assert.InEpsilon(t, temp, result["temperature"], 0.001)
	assert.Len(t, result, 1, "should only contain the specified option")
}

func TestBuildAPIOptions_MultipleOptions(t *testing.T) {
	t.Parallel()
	ctxSize := 4096
	temp := float32(0.7)
	topP := float32(0.9)
	topK := 50
	stop := []string{"###", "END"}

	cfg := &File{Options: &ModelOptionsBlock{
		ContextWindowSize: &ctxSize,
		Temperature:       &temp,
		TopP:              &topP,
		TopK:              &topK,
		Stop:              stop,
	}}

	result := cfg.BuildAPIOptions()
	require.NotNil(t, result)
	assert.Equal(t, ctxSize, result["num_ctx"])
	assert.InEpsilon(t, temp, result["temperature"], 0.001)
	assert.InEpsilon(t, topP, result["top_p"], 0.001)
	assert.Equal(t, topK, result["top_k"])
	assert.Equal(t, stop, result["stop"])
	assert.Len(t, result, 5, "should contain all specified options")
}

func TestLoadConfig_DockerMCPWithAssistantPrompt(t *testing.T) {
	t.Parallel()
	exampleHCLContent := `
docker_mcp "postgres" "postgres:15" {
  
  assistant_prompt {
    from_string = <<EOS
You have access to a PostgreSQL database running in Docker. Use this for:
- Database queries and operations
- Schema management  
- Data manipulation
Best practices: Use transactions for multi-step operations.
EOS
  }
}
`
	parsedContent := parseHCLString(t, exampleHCLContent, t.Name()+".exampleHCLContent")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	if assert.Len(t, cfg.DockerMCPBlock, 1) {
		dm := cfg.DockerMCPBlock[0]
		assert.Equal(t, "postgres", dm.Name)
		assert.Equal(t, "postgres:15", dm.Image)
		assert.NotNil(t, dm.AssistantPrompt)
		assert.Contains(t, dm.AssistantPrompt.FromString, "PostgreSQL database running in Docker")
		assert.Empty(t, dm.AssistantPrompt.FromFile)
	}
}

// todo: fix duplication
// nolint
func TestLoadConfig_HttpMCPWithAssistantPrompt(t *testing.T) {
	t.Parallel()
	hclFilecontent := `
mcp_over_http "weather_api" "http://weather-api:8080" {
  
  assistant_prompt {
    from_string = "Use this weather API for current conditions, forecasts, and historical data."
  }
}
`
	parsedContent := parseHCLString(t, hclFilecontent, t.Name()+".hcl")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	if assert.Len(t, cfg.HttpMCPBlock, 1) {
		hm := cfg.HttpMCPBlock[0]
		assert.Equal(t, "weather_api", hm.Name)
		assert.Equal(t, "http://weather-api:8080", hm.URL)
		assert.NotNil(t, hm.AssistantPrompt)
		assert.Equal(t, "Use this weather API for current conditions, forecasts, and historical data.", hm.AssistantPrompt.FromString)
		assert.Empty(t, hm.AssistantPrompt.FromFile)
	}
}

func TestCommandLineOptions_NewWithEnvVar(t *testing.T) {
	// Test without env var
	t.Setenv("MARVIN_CONFIG", "")
	opts := NewCommandLineOptions()
	assert.Equal(t, ".marvin.hcl", opts.ConfigFile)

	// Test with env var set
	t.Setenv("MARVIN_CONFIG", "/custom/config.hcl")
	opts = NewCommandLineOptions()
	assert.Equal(t, "/custom/config.hcl", opts.ConfigFile)
}

func TestCommandLineOptions_LoadPriority(t *testing.T) {
	hclFileContents := `
local_program "test" {
  program = "/bin/test"
}
`

	// Create a temporary config file
	tmpFile := t.TempDir() + "/test.hcls"
	err := os.WriteFile(tmpFile, []byte(hclFileContents), 0600)
	require.NoError(t, err)

	// Test 1: Environment variable is used when creating options
	t.Setenv("MARVIN_CONFIG", tmpFile)
	opts := NewCommandLineOptions()
	cfg, err := opts.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.LocalPrograms, 1)

	// Test 2: CLI flag override happens when user manually sets ConfigFile
	// (Cobra would do this when -c flag is used)
	t.Setenv("MARVIN_CONFIG", "/nonexistent.hcls")
	opts = NewCommandLineOptions()
	opts.ConfigFile = tmpFile // Simulate CLI flag being set by Cobra
	cfg, err = opts.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Len(t, cfg.LocalPrograms, 1)
}

func TestCommandLineOptions_LoadWithEnvVarError(t *testing.T) {
	// Test error when env var points to non-existent file
	t.Setenv("MARVIN_CONFIG", "/nonexistent/config.hcl")
	opts := NewCommandLineOptions()
	_, err := opts.Load()
	assert.Error(t, err)
}

// todo: fix duplication
// nolint
func TestLoadConfig_DisplayBlockDefaults(t *testing.T) {
	t.Parallel()
	hclFileContent := `
display {}
`
	parsedContent := parseHCLString(t, hclFileContent, t.Name()+".hcl")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.NotNil(t, cfg.Display)
	// Test default values when display block is empty
	assert.False(t, cfg.ShowThinking())            // default: false
	assert.True(t, cfg.ShowTools())                // default: true
	assert.True(t, cfg.ShowDone())                 // default: true
	assert.False(t, cfg.Verbose())                 // default: false
	assert.Equal(t, "plain", cfg.ThinkingFormat()) // default: plain
	assert.Equal(t, "detailed", cfg.ToolFormat())  // default: detailed
}

// todo: fix duplication
// nolint
func TestLoadConfig_DisplayBlockCustomValues(t *testing.T) {
	t.Parallel()
	hclFileContents := `
display {
  show_thinking = true
  show_tools = false
  show_done = false
  verbose = true
  thinking_format = "markdown"
  tool_format = "simple"
}
`
	parsedContent := parseHCLString(t, hclFileContents, t.Name()+".hcl")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.NotNil(t, cfg.Display)
	assert.True(t, cfg.ShowThinking())
	assert.False(t, cfg.ShowTools())
	assert.False(t, cfg.ShowDone())
	assert.True(t, cfg.Verbose())
	assert.Equal(t, "markdown", cfg.ThinkingFormat())
	assert.Equal(t, "simple", cfg.ToolFormat())
}

// todo: fix duplication
// nolint
func TestLoadConfig_NoDisplayBlock(t *testing.T) {
	t.Parallel()
	hclFileContent := `
local_program "test" {
  program = "/bin/test"
}
`
	parsedContent := parseHCLString(t, hclFileContent, t.Name()+".hcl")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Nil(t, cfg.Display)
	// Should still return defaults when no display block
	assert.False(t, cfg.ShowThinking())
	assert.True(t, cfg.ShowTools())
	assert.True(t, cfg.ShowDone())
	assert.False(t, cfg.Verbose())
	assert.Equal(t, "plain", cfg.ThinkingFormat())
	assert.Equal(t, "detailed", cfg.ToolFormat())
}

// todo: fix duplication
// nolint
func TestLoadConfig_DisplayBlockPartialValues(t *testing.T) {
	t.Parallel()
	hclFileContents := `
display {
  show_thinking = true
  thinking_format = "collapsed"
}
`
	parsedContent := parseHCLString(t, hclFileContents, t.Name()+".hcl")
	cfg, err := interpretConfigFile(parsedContent, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.NotNil(t, cfg.Display)
	assert.True(t, cfg.ShowThinking())                 // explicitly set
	assert.True(t, cfg.ShowTools())                    // default
	assert.True(t, cfg.ShowDone())                     // default
	assert.False(t, cfg.Verbose())                     // default
	assert.Equal(t, "collapsed", cfg.ThinkingFormat()) // explicitly set
	assert.Equal(t, "detailed", cfg.ToolFormat())      // default
}
