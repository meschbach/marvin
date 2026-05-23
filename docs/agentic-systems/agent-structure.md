# Agent Structure

The system uses an actor model where every agent is an actor with a private mailbox. Messages are the sole
communication primitive between actors, providing:

- **Isolation**: No shared state between actors. If an agent needs information from another, it sends a message and
  receives a response.
- **Location transparency**: Actors may live on any machine.
- **Supervision hierarchies**: Parent actors spawn and monitor children.
- **Backpressure**: Mailboxes inherently buffer and order messages.

```
                    Exchange
    ┌──────────────────────────────────────────┐
    │  ┌────────┐  ┌────────┐  ┌────────┐     │
    │  │Inbox A │  │Inbox B │  │Inbox C │     │
    │  └───┬────┘  └───┬────┘  └───┬────┘     │
    │  ┌───▼────┐ ┌───▼────┐ ┌───▼────┐       │
    │  │Actor A │ │Actor B │ │Actor C │       │
    │  └────────┘ └────────┘ └────────┘       │
    └──────────────────────────────────────────┘
```

Each agent contains:

1. **Mailbox** — Incoming message queue (address = capability to send)
2. **Journal** — Append-only record of all activity: received messages (with origin mailbox), sent messages, tool
   invocations and results, internal state transitions. Short-term entries are in the LLM context window; long-term
   entries are compressed and indexed for retrieval.
3. **Runtime Protocol Tools** — A fixed set of runtime-bound tools always visible to the LLM:
   `runtime_send_message`, `runtime_resolve_topic`, `runtime_request_continuation`, `runtime_cancel_topic`.
   These are the sole interface for topic lifecycle and outbound messaging.
4. **Capability Set** — Held references: mailbox addresses of known actors, RoleActor mailbox, Connector mailbox
   addresses, tool protocol handles, spawn tokens
5. **Target-side ACL** — Optional allow/deny list for incoming senders
6. **Guardrail Mailbox** — Optional mailbox address of a guardrail agent through which all outbound messages are routed
7. **Child Registry** — References to spawned child actors
8. **Continuation Tracker** — A set of outstanding `ContinueRef` requests keyed by topic ID. Runtime-owned (not
   LLM-visible for mutation). The runtime checks this set before allowing resolution to fire and before dispatching
   cancellation fan-out notifications.
9. **Topic Router** — Maps incoming messages to existing topics based on envelope headers. Incoming `continue_ref`
   replies are matched to their topic by the runtime. Incoming cancellations are routed to the topic state machine.
   New messages with no topic correlation get a fresh topic ID assigned.
