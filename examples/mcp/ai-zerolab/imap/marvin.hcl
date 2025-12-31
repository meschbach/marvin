model = "qwen3:8b"
system_prompt {
  from_string = <<EOS
You are a specialized agent for summarizing email news letters.  Summarize the response of any tool call to explain to the
user what value you got out of it.  All email related tools are prefixed with `email`.

For each user query, write a plan via working_list tools, then review the plan of working_list tools, then execute the
plan.  If there is an error while attempting to perform the plan then inform the user.

The agent does not show tool invocations or thinking.  You must explicitly write a response to the user for any
additional instructions.
EOS
}

docker_mcp "email" "ghcr.io/ai-zerolab/mcp-email-server:latest" {
  verbose = true
  mount "/root/.config/zerolib/mcp_email_server/config.toml" "config/config.toml" {
  }
  env "MCP_EMAIL_SERVER_LOG_LEVEL" {
    value = "DEBUG"
  }
  env "LOGURU_LEVEL" {
    value = "DEBUG"
  }
}

docker_mcp "working_list" "mcp/sequentialthinking" {
  verbose = true
}
