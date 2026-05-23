# Exchange

The Exchange is both a **library** (importable, embeddable) and a **standalone server**. It uses a single message
delivery contract with two hosting modes. The delivery semantics are identical in both modes — asynchronous,
non-blocking, priority-ordered. Only persistence and fault isolation differ between modes.

Its responsibilities:

- **Registry**: Maps actor identities to mailbox locations
- **Routing**: Delivers messages to the correct destination mailbox through the push delivery contract
- **Capability Enforcement**: Validates sender has a reference to the target mailbox before delivery; forwards to
  target-side ACL for secondary check
- **Cancellation Validation**: Validates sender holds a `CancelRef` matching the target topic ID before routing a
  cancellation message. Rejects with "no authority" if mismatched.
- **Guardrail Routing**: Routes outbound messages through designated guardrail agents in sequence before delivery
- **External Gateway**: Accepts gRPC connections from Communication Systems and Event Sources; manages subscription
  streams for outbound delivery to CommSystems
- **Internal CA**: Issues x509 certificates to authenticated CommSystems, Event Sources, and agents
- **Federation**: Routes across machines when exchanges are networked
- **Observability**: Propagates OpenTelemetry trace context with every message
- **Guardrail policy evaluation**: Matches outbound message provenance against
  configured policies and injects mandatory guardrails before delivery
- **System actor hosting**: Provisions `supervisor.{scope}`, `registry.{scope}`, and
  `dead-letter.{scope}` for each user and org scope
- **Rate limit enforcement**: Rejects messages targeting a mailbox that has
  exceeded its `max_depth`, returning `ErrMailboxFull`

## Message Delivery

Message delivery follows a **push contract** between the Exchange and agent runtimes:

1. On spawn, the runtime registers a **delivery endpoint** with the Exchange. The endpoint is an address the
   Exchange can push messages to (in-process callback in embedded mode, network address in server mode).
2. When routing a message, the Exchange resolves the mailbox address to the runtime's delivery endpoint and pushes
   the message. The runtime acknowledges receipt. The Exchange may retry against a configured policy on failure.
3. Runtimes never poll the Exchange for messages. The Exchange is the active party in delivery.
4. Mailbox depth, rate limits, and priority ordering are enforced at the Exchange side before push.

This separates the routing question (Exchange resolves `mailbox://` to a delivery endpoint) from the transport
question (how the push is implemented). The architecture works identically whether the runtime is in-process on
the same machine or on a remote machine — the Exchange resolves to a different endpoint type but the contract
is the same.

## Hosting Modes

| Property | Embedded | Server |
|---|---|---|
| Delivery | Asynchronous (in-memory queue) | Asynchronous (persisted queue) |
| Persistence | None (ephemeral) | Configurable (routing table + agent state) |
| Runtime endpoints | In-process callbacks | Network-addressable endpoints |
| Fault isolation | Process boundary only | Per-node failure isolation |
| Federation | N/A | Cross-machine routing |

Embedded mode uses the same message queue and routing path as server mode. Messages are enqueued, the sender
receives acknowledgment, and the target processes the message asynchronously in priority order. There is no
synchronous shortcut — this ensures the embedded path exercises the same delivery contract as the networked path.

## Exchange External Gateway

The Exchange exposes a single external protocol: **gRPC**. All Communication Systems and Event Sources connect via gRPC
to the external gateway.

**Communication Systems (bidirectional):**

- **Inbound**: CommSystem calls `SendMessage(agent_id, envelope)` → Exchange validates the CommSystem has a capability
  reference to the target agent's mailbox → routes to agent's inbox.
- **Outbound**: On connect, CommSystem establishes a `Subscribe(agent_ids[])` streaming RPC. Exchange pushes agent
  responses, status changes, and events into the stream. CommSystem translates stream events into platform-specific
  output (Slack thread reply, CLI stdout, browser push).

**Event Sources (inbound only):**

- **Inbound**: Event source calls `InjectMessage(agent_id, envelope)` → Exchange validates the Event Source's x509
  identity, stamps provenance on the message, and routes it to the target agent's inbox.
- **No outbound**: Event sources have no subscription stream. They inject events and receive no response back through
  the same connection.

**Auth on connect**: CommSystems and Event Sources present x509 certificates (or Unix peer cred for local sockets).
Exchange validates against the CA. Platform-specific CommSystems (Slack) additionally map the platform user identity to
a security principal.

**Protocol choice**: gRPC only for Exchange-facing communication. WebSocket is used exclusively between browser and Web
UI backend (which itself is a CommSystem speaking gRPC to the Exchange). This keeps the Exchange's external contract
typed (protobuf) and minimal.
