model = "qwen3:0.6b"
system_prompt {
  from_string = <<EOS
Think through how to write a story via `thinking.sequentialthinking`.  After you've thought through how to write the
story follow your plan to write it.
EOS
}

docker_mcp "thinking" "mcp/sequentialthinking" {
  verbose = false
}
