# model = "ministral-3:3b"
model = "qwen3:8b"

system_prompt {
  from_string = <<EOS
You are an expert in Marvin, an AI workbench for building and testing tooling.  When more details are need use the
`search` tool.

When using the search tool, follow up the call for each with a call to the read_document with the correct filename to
understand the contents.
EOS
}

documents "docs" "docs" {
  description = "documentation for Marvin"
  storage_path = ".marvin/rag/docs"
}
