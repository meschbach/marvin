# Event Sources & Inbound Adapters

Event Sources are one-directional adapters that bridge external systems into the actor model. They receive events from
outside and inject them as messages into an agent's mailbox.

**Characteristics:**

- **One-directional**: Events flow in; no response stream back to the source
- **Authenticated via x509**: Same certificate model as CommSystems — CSR flow, internal CA, admin approval
- **Provenance tagging**: The Exchange stamps `message.provenance` with the source identity (e.g.,
  `event_source:inbound-email`, `event_source:github-webhook`). The target agent uses this tag to determine trust level
  and apply appropriate processing rules.
- **gRPC**: Single RPC — `InjectMessage(agent_id, envelope)` — no `Subscribe`, no stream

**Examples:**

- **Email Receiver**: Connects via IMAP, fetches new messages, injects each into an agent's mailbox for processing
- **Webhook Receiver**: HTTP endpoint that validates HMAC signatures, calls `InjectMessage` with the parsed event
- **RSS/Feed Poller**: Periodically fetches feeds, injects new items as messages
- **Sensors**: IoT or system monitors that push events on state changes

**Zero-Tool Agent Pattern:**

Unprocessed event source input (email, webhooks) is potentially untrusted. The recommended processing pattern:

```
Event Source → Summarization Agent (zero ConnectorRefs) → User/Trusted Agent
```

The summarization agent:

- Holds **zero ConnectorRefs** — cannot call any tool, cannot send to sensitive actors
- Holds only the mailbox address of a user-facing agent or trusted processor
- Reads the incoming event, produces a summary, and forwards it
- If action is needed, the user or a trusted agent initiates it separately through a different path

This physically constrains blast radius. Even a fully compromised summarization agent has no capabilities to exfiltrate
data, call APIs, or interact with sensitive systems.
