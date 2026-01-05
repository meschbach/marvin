model = "qwen3:8b"

system_prompt {
  from_string = <<EOS
You are an assistant helping an operator deal with logs in different time zones.  Use time.get_current_time to infer the
local time zone when needed.
EOS
}

docker_mcp "time" "mcp/time" {
  verbose = false
  args {
    strings = ["--local-timezone", "America/Los_Angeles"]
  }
}
