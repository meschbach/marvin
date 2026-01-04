#!/bin/bash

set -e
go test -count 1 ./internal/...
go build -o marvin ./cmd

echo "===="
echo "RAG index"
echo "===="
./marvin -c marvin.test.hcl rag index
echo "===="
echo "RAG query"
echo "===="
./marvin -c marvin.test.hcl rag query docs marvin
echo "===="
echo "LLM Query"
echo "===="
./marvin -c marvin.test.hcl query --show-thinking --show-done --show-tools "does Marvin support retrieval augmented generation?"
