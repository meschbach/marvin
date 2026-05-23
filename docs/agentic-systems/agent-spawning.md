# Agent Spawning

An agent spawns a child by providing:

- Context and behavioral instructions
- A **granted subset** of the parent's capabilities (MailboxRefs, ConnectorRefs, Spawn token)
- Optional time-to-live (TTL)
- Optional guardrail mailbox (or chain of guardrail mailboxes) for all outbound responses
- Security context (inherited or reduced from parent)
- Optional **continuation_deadline** — overrides the default continuation deadline supplied to
  `runtime_request_continuation` for messages directed at this child. If unset, inherits the spawning agent's
  default.

Spawning creates a new Agent Runtime in `unprovisioned` state. The Supervisor transitions it to `running` once
the delivery endpoint is registered with the Exchange. The parent receives the child's mailbox address and can
begin messaging it immediately; messages are queued in the mailbox until the runtime is ready to process them.

Child agents communicate with their parent through the same mailbox pattern. This enables hierarchies with guardrail
interposition. Tool protocol access, however, flows through RoleActors rather than through the parent-child hierarchy.

**RoleActor-based grant flow:**

Tool access is not inherited from a parent during spawning. Instead, agents request grants from a RoleActor:

```
Root Agent
  │
  ├── request "grant mcp:vikunja:read_tasks" → Personal RoleActor
  ├── request "grant mcp:family-calendar:*" → Family RoleActor
  │
  └── receives MailboxRef(Connector) from each RoleActor
        └── holds ConnectorRefs, not direct protocol refs
```

The agent hierarchy (who spawned whom) and the tool access model (who granted what) are separate concerns:

```
Actor hierarchy:
User-Facing Agent (Slack/Web Frontend)
  ├── Project Agent (spawned child, has MailboxRef: parent)
  │     └── Guardrail: PII Redactor → Policy Check (outbound → CommSystem)
  ├── Email Agent (spawned child, has MailboxRef: parent)
  └── Research Agent (spawned child, TTL=15m, has MailboxRef: parent)

Tool protocol grants (via RoleActor):
User-Facing Agent
  ├── ConnectorRef: Vikunja Connector → has protocol ref: mcp://vikunja (read_tasks scope)
  ├── ConnectorRef: Email Connector → has protocol ref: mcp://email
  └── ConnectorRef: Search Connector → has protocol ref: mcp://web-search
```

Parent agents may delegate their ConnectorRefs to child agents by forwarding the MailboxRef. The child's use of the
Connector is governed by the Connector's own context (scope narrowing, guardrails) and any target-side ACL the
Connector maintains.
