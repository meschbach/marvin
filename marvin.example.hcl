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