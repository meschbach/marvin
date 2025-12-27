system_prompt {
  from_string = <<EOS
You are an assistant helping an operator deal with logs in different time zones.
EOS
}

docker_mcp "time" "mcp/time" {
  # env "LOCAL_TIMEZONE" {
  #   value = "America/Los_Angeles"
  # }
}
