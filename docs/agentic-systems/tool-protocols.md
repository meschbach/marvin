# Tools & Tool Protocols

Tools are **not global**. Each agent interacts with tools through a Connector that wraps a specific tool protocol
instance with contextual instructions:

- **Vikunja Connector**: Knows the user's Vikunja host, credentials, and authorized project scope (e.g., "this user
  can only access the 'work' project")
- **Email Connector**: Knows contact groups, organizational routing, inbox triage rules authorization
- **Database Connector**: Knows schema, query patterns, access restrictions
- **Shell Connector**: Shell/script execution on isolated hardware for sandboxing, scoped to a specific directory

The same protocol server connected through different Connectors produces different behavior because each Connector
carries its own context (user identity, scope, credentials). Access is governed by whether the agent holds a
`ConnectorRef` for that Connector.

## Supported Tool Protocols

| Protocol                         | Description                                                              |
|----------------------------------|--------------------------------------------------------------------------|
| **MCP** (Model Context Protocol) | Standard tool protocol for connecting to external tools and data sources |
| **Local Program**                | Subprocess execution on the host                                         |
| **Docker MCP**                   | Containerized tool execution                                             |
| **HTTP MCP**                     | REST-based tool access                                                   |
| (Extensible)                     | New protocols register a Connector type                                  |

## Connector Queue Behavior

Connectors are actors with a single mailbox, but they may receive tool requests from multiple agents
concurrently. To prevent one agent from starving others:

- **Per-sender fair queuing**: The Connector's mailbox processes requests round-robin by sender identity.
  A single agent cannot flood the queue and block other agents' tool calls.
- **Independent rate limits**: Each Connector enforces its own `max_depth` and `max_processing_rate`,
  independent of the agents using it. A deep queue on one Connector does not affect other Connectors.
- **Idempotency contract**: Where the underlying protocol permits, tool calls should be designed as
  idempotent. This is a contract between the Connector and its protocol server, not enforced by the
  runtime, but documented as the expected pattern for safe retry.
