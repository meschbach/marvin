model = "llama3.2:latest"

# Advanced model options for fine-tuning responses
options {
  # Context window size (tokens) - default: 2048
  context_window_size = 4096
  
  # Sampling temperature (0.0-1.0) - higher = more creative - default: 0.8
  temperature = 0.7
  
  # Nucleus sampling (0.0-1.0) - default: 0.9
  top_p = 0.9
  
  # Top-k sampling (-1 = no limit) - default: 40
  top_k = 40
  
  # Maximum tokens in response (-1 = unlimited) - default: -1
  num_predict = 2048
  
  # Repetition penalty - default: 1.1
  repeat_penalty = 1.1
  
  # How far back to check for repetitions (0 = disabled) - default: 64
  repeat_last_n = 64
  
  # Random seed for reproducible results (optional)
  seed = 42
  
  # Stop sequences to end generation (optional)
  stop = ["###", "END"]
}

system_prompt {
  from_string = <<EOS
You are an example AI tool demonstrating Marvin, an AI tooling assistant allowing for plugging in various assistants
within knowledge bases.
EOS
}

local_program "gitea" {
  program = "/opt/homebrew/bin/gitea-mcp-server"
  args = ["-host", "https://gitea.example.com", "-read-only","-token", "<some token>"]
}

local_program "docs" {
  # provides access to specific files
  program = "file-mcp"
}
