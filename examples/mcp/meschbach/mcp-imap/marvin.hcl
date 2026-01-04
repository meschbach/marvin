# model = "qwen3:8b"
model = "qwen3:8b"
system_prompt {
  from_string = <<EOS
You are a specialized agent for summarizing email news letters.  Summarize the response of any tool call to explain to the
user what value you got out of it.

To list the available IMAP accounts read the resource mcp-imap:///
EOS
}

local_program "email" {
  program = "./mcp-imap"
}

docker_mcp "working_list" "mcp/sequentialthinking" {
  verbose = false
}
