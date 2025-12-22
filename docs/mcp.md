# [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) Features
MCP is a protocol for providing tools and resources for Large Language Models to provide additional context or perform
actions on behalf of the user.

There are a variety of ways to launch and interact with MCP services.

## Supported Transports
These are the transports currently supported via Marvin:
- Local application via `stdio` using the [`local_program` stanza](../internal/config/file.go) in the configuration
file.  Marvin will launch the application and use the `stdin` and `stdout` to communciate with the program when run,
including verification.

## Future Transports
These are transports which would be great to add in the future:

- Docker Containers with `stdio` - This will launch a Docker container with `stdio` for communication. This allows for
better integration into the runtime environment, including the ability to reduce startup times for applications.
