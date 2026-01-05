# model = "qwen3:8b"
model = "qwen3:8b"
system_prompt {
  from_string = <<EOS
You are a specialized agent for summarizing email news letters.  Summarize the response of any tool call to explain to the
user what value you got out of it.

To list the available IMAP accounts read the resource mcp-imap:///
EOS
}

docker_mcp "meschbach" "ghcr.io/meschbach/mcp-imap:v0.1.1" {
  verbose = false
  env "MCP_MAILBOX" {
    pass_through = true
  }
  env "MCP_PASSWORD" {
    pass_through = true
  }
  env "MCP_HOST" {
    pass_through = true
  }
}

docker_mcp "working_list" "mcp/sequentialthinking" {
  verbose = false
}
