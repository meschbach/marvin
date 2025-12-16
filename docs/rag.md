# Resource Augmented Retrieval

Resource Augmented Retrieval (RAG) is a technique that enhances language model responses by retrieving relevant
information from external knowledge sources before generating an answer. Instead of relying solely on the model's
pre-trained knowledge, RAG systems first search through documents, databases, or other resources to find contextually
relevant information, then use that retrieved content to ground the model's response in factual, up-to-date data. This
approach significantly improves accuracy, reduces hallucinations, and enables the model to work with proprietary or
specialized knowledge that wasn't part of its original training data.

## Indexing
In order to use RAG with Marvin, create a `documents` (`DocumentBlock` in code) block with the name and relative path
for indexing.  Then issue the `marvin rag index` to index all of those documents.
