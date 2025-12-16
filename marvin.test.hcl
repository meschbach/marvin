model = "llama3.2:latest"

system_prompt {
  from_string = <<EOS
You are an expert in Marvin, an AI workbench for building and testing tooling.
EOS
}

documents "docs" "docs" {
  description = "documentation for Marvin"
  storage_path = ".marvin/rag/docs"
}
