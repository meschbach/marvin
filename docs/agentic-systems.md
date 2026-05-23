# Agentic Systems

Architecture for multi-user, multi-agent coordination built on an actor model with capabilities-based security,
asynchronous topic workflows, and LLM-powered agents.

## Design Rationale

Every design choice in this architecture follows from a few core principles:

- **Actor model** — Agents are isolated actors communicating exclusively through messages. No shared state, no
  locks, no races. This gives natural backpressure (mailbox depth), location transparency (actors on any machine),
  and supervision hierarchies (parents monitor children).
- **Object capabilities** — Possession of a reference IS authorization. No ambient authority, no global ACL, no
  privilege escalation through confused-deputy attacks. Every delegation is auditable. The question is never "who
  are you?" but "what reference do you hold?"
- **Async-first** — Agents never block waiting for responses. The journal IS the continuation store: the LLM reads
  past context to understand what it was waiting for and why. Topic lifecycle is owned by the runtime, not the LLM,
  preventing state corruption from model hallucination.
- **Push delivery** — The Exchange pushes messages to agent runtimes (never poll). This enables priority ordering
  (cancellations before continuation replies before new work) and backpressure at the Exchange level.
- **Two-level journal** — Raw append-only journal for full-fidelity audit; working memory (compiled from raw
  entries) for the LLM context window. Separation of concerns between audit trail and inference context.
- **Guardrails as agents** — Interposition uses the same actor model, not a special primitive. Guardrail agents
  are ordinary agents that hold zero tool protocol references by design — they can inspect and block, but never
  exfiltrate or cause side effects.
- **LLM Gateway as pure proxy** — The gateway manages provider connections, auth rotation, and failover. It never
  sees agent journals, messages, or internal state — only LLM request/response payloads.

## Core Mental Model

Two ideas underpin the entire architecture:

**1. Everything is an actor.** Every agent, every system component (Supervisor, Registry, Connector, Guardrail), is
an actor with a private mailbox. Messages are the sole communication primitive. Actors process one message at a time,
may spawn child actors, and carry a security context. This gives you isolation, sequential processing within an actor,
and a unified model for all concurrent computation.

**2. Possession is authorization.** You hold a mailbox address = you can send to that actor. You hold a ConnectorRef =
you can call tools through that Connector. You hold a Spawn token = you can create children. There is no separate
permissions database. The Exchange validates at routing time that every message arrives via a valid reference chain,
checking expiry and delegation policy on each hop.

## Quick Tour: A Message's Journey

Alice sends a Slack message: "What's on my calendar today?"

```
┌────────────────────────────────────────────────────────────┐
│ 1. Slack → CommSystem                                      │
│    The Slack Communication System translates the message    │
│    into a gRPC SendMessage call to the Exchange.            │
│    Tags it with correlation_id (for response matching)      │
│    and Alice's platform identity.                           │
│    [More: Exchange →](agentic-systems/exchange.md)          │
├────────────────────────────────────────────────────────────┤
│ 2. Exchange → Route                                        │
│    The Exchange validates the CommSystem holds a            │
│    MailboxRef for Alice's agent. Routes the message to      │
│    the agent's mailbox in the Exchange.                     │
│    [More: Capabilities →](agentic-systems/capabilities.md)  │
├────────────────────────────────────────────────────────────┤
│ 3. Agent Runtime → Process                                  │
│    The runtime pulls the message from its inbox.            │
│    Assigns Topic ID "T1". Assembles LLM context:            │
│    system prompt + recent journal + current message.        │
│    Priority: cancellations (none) → guardrail rejections    │
│    (none) → continuation replies (none) → this new work.    │
│    [More: Agent Runtime →](agentic-systems/agent-runtime.md)│
├────────────────────────────────────────────────────────────┤
│ 4. LLM → Decide                                            │
│    LLM reads "What's on my calendar?" and decides it needs  │
│    calendar data. It has a ConnectorRef for the Calendar    │
│    Connector (granted by Alice's RoleActor).                │
│    [More: Tool Protocols →](agentic-systems/tool-protocols.md)│
├────────────────────────────────────────────────────────────┤
│ 5. Runtime → Continuation                                   │
│    LLM calls runtime_request_continuation(CalendarAgent,    │
│    "get today's events", deadline=30s). Runtime stamps      │
│    a continue_ref {topic_id: T1, reply_to: AliceAgent,     │
│    deadline: ...} on the outbound. Transitions T1 to        │
│    WaitingForContinuations.                                 │
│    [More: Topic Model →](agentic-systems/topic-model.md)    │
├────────────────────────────────────────────────────────────┤
│ 6. CalendarAgent → Response                                 │
│    CalendarAgent receives the request, processes it         │
│    through its own Connector (Vikunja MCP), and responds.   │
│    The response echoes the continue_ref.                    │
├────────────────────────────────────────────────────────────┤
│ 7. Runtime → Match & Resume                                 │
│    Alice's runtime matches the incoming reply to T1 via     │
│    the continue_ref. Transitions T1 back to Active.         │
│    LLM resumes — sees the calendar events in context.       │
├────────────────────────────────────────────────────────────┤
│ 8. LLM → Resolve                                            │
│    LLM calls runtime_resolve_topic("done"). Runtime         │
│    checks: continuations set for T1 is empty ✓.             │
│    Forwards resolution through guardrail chain (PII         │
│    redactor → policy check).                                │
│    [More: Guardrails →](agentic-systems/guardrails.md)      │
├────────────────────────────────────────────────────────────┤
│ 9. Exchange → Deliver                                       │
│    Exchange routes the resolution event to the Slack        │
│    CommSystem's subscription stream, tagged with the        │
│    original correlation_id.                                 │
├────────────────────────────────────────────────────────────┤
│ 10. CommSystem → Slack                                      │
│     Slack CommSystem translates the stream event into a     │
│     thread reply: "You have 3 events today: standup at     │
│     10am, lunch at 12pm, review at 3pm."                    │
└────────────────────────────────────────────────────────────┘
```

## Key Concepts

### Core Primitives

The foundational building blocks. Every entity in the system is one of these.

- **Actor** — Unit of computation with a mailbox that processes messages sequentially. Can spawn child actors and
  carries a security context. Every agent is an actor, but not all actors are agents. [Details →](agentic-systems/agent-structure.md)
- **Agent** — An actor that uses an LLM for reasoning. Wraps tools with contextual instructions, maintains a
  private journal, communicates via mailboxes. [Details →](agentic-systems/agent-structure.md)
- **Agent Runtime** — The deterministic environment wrapping every agent. Manages lifecycle, inbox priority (cancellations
  → guardrail rejections → continuation replies → new work), topic state machine, tool binding, LLM context assembly,
  and deadline monitoring. One runtime per agent, managed by the Supervisor.
  [Details →](agentic-systems/agent-runtime.md)
- **Mailbox / Inbox** — Message queue belonging to an actor. Processes sequentially. The mailbox address IS the
  capability to send to that actor. Reserved system slots ensure cancellations and guardrail rejections are never
  blocked by a full topic queue. [Details →](agentic-systems/agent-runtime.md)
- **Topic ID** — Application-level workflow identifier assigned when an agent pulls a message from its inbox. Groups
  related messages (a conversation, a task) into a unit of work. Agent-scoped unique. Topic lifecycle is owned by the
  runtime, not the LLM. [Details →](agentic-systems/topic-model.md)

### Communication Fabric

How messages move between actors, external platforms, and tools.

- **Exchange** — The core message router. A library (embeddable) and standalone server. Maintains the agent registry,
  enforces capabilities at delivery time, routes outbound messages through guardrails, federates across machines, and
  hosts system actors per scope. Uses a push delivery contract — runtimes register endpoints, Exchange pushes.
  Single external protocol: gRPC. [Details →](agentic-systems/exchange.md)
- **Communication System** — Bidirectional gRPC bridge between an external platform (Slack, Web UI, CLI) and the
  Exchange. Translates platform-specific messages to/from the actor model. Handles auth and subscription streams for
  agent output. [Details →](agentic-systems/exchange.md)
- **Event Source** — Inbound-only adapter that receives external events (webhooks, email, sensors) and injects them
  into an agent's mailbox via gRPC `InjectMessage`. Authenticated via x509. No subscription stream — one-directional.
  [Details →](agentic-systems/event-sources.md)
- **Connector** — An actor spawned by a RoleActor that holds a reference to a tool protocol server. All tool calls are
  messages sent to a Connector, which proxies to the protocol and returns results through its guardrail chain.
  Per-sender fair queuing prevents one agent from starving others.
  [Details →](agentic-systems/tool-protocols.md)

### Security & Identity

Who users are and what they can do.

- **Capability** — A reference that confers authority: a mailbox address (can message), a RoleActor mailbox (can request
  grants), a Connector mailbox (can call tools), a tool protocol handle (can use a protocol), or a spawn token (can
  create children). Possession IS authorization. Constrained references carry expiry and delegation policies.
  [Details →](agentic-systems/capabilities.md)
- **InternalUser** — A single principal identity mapped from one or more CommSystem accounts (Slack user, Web session,
  SSH key). All connections from the same person resolve to the same InternalUser, regardless of platform.
  [Details →](agentic-systems/security.md)
- **RoleActor** — An actor that brokers capability grants. Holds no direct tool references. On policy-approved request,
  spawns a Connector actor (with optional guardrails) and returns its MailboxRef. Uses guardrails for its own decision
  logic — automated policies, escalation, human-in-the-loop.
  [Details →](agentic-systems/capabilities.md)
- **Scope** — Namespace boundary for mailbox addresses (`mailbox://{name}.{scope}`). Hierarchy: `.user` within `.org`.
  The `.local` pseudo-scope resolves relative to the sender's scope chain.
  [Details →](agentic-systems/naming-addressing.md)
- **Provenance** — Trust-level tag stamped on every message by the Exchange indicating its origin. Enables
  provenance-based guardrail policies. Examples: `event_source:inbound-email`, `comm_system:slack/user:U1234`.
  [Details →](agentic-systems/guardrails.md)

### Control & Lifecycle

How agents are created, monitored, intercepted, and garbage-collected.

- **Guardrail Agent** — A user-defined agent placed in another agent's outbound message path for interposition. Two
  flavors: Pass/Reject (block or allow) and Mutating (modify content). Chainable. Hold zero tool protocol references
  by design — cannot exfiltrate or cause side effects. [Details →](agentic-systems/guardrails.md)
- **Supervisor** — System actor at mailbox://supervisor.{scope} managing agent lifecycle: spawns, monitors health via
  heartbeat, enforces TTL expiry, notifies parents on death. One per scope, created lazily.
  [Details →](agentic-systems/system-actors.md)
- **Registry** — System actor at mailbox://registry.{scope} providing directory and discovery: lookup by UUID, alias,
  capability, or role membership. Presence watching via TTL-based re-registration.
  [Details →](agentic-systems/system-actors.md)
- **Dead Letter Agent** — System actor at mailbox://dead-letter.{scope} receiving undeliverable messages (expired TTL,
  non-existent actor, exceeded mailbox depth). Configurable handling: log, notify sender, forward.
  [Details →](agentic-systems/system-actors.md)
- **Cancellation** — System message signaling an agent to stop processing a topic. Must carry a CancelRef validated
  by the Exchange. Always accepted by the runtime, bypassing LLM discretion. Fan-outs to sub-agents on active
  continuations. [Details →](agentic-systems/topic-model.md)
- **ContinueRef** — Reference stamped on outbound messages when the LLM calls `runtime_request_continuation`. Contains
  topic_id, reply_to mailbox, and absolute deadline. The runtime matches incoming replies by ContinueRef to resume the
  correct topic. [Details →](agentic-systems/topic-model.md)
- **Agent Spawning** — Parents spawn children with context, capability subset, optional TTL, optional guardrail chain.
  Tool access is separate — agents request grants from RoleActors, not inherited from parents.
  [Details →](agentic-systems/agent-spawning.md)

### Memory & State

How agents remember what happened and what they were doing.

- **Journal (Raw)** — Runtime-owned, append-only record of every event: received/sent messages, tool invocations and
  results, LLM invocations, state transitions, cancellations. Full fidelity, never modified. Source of truth — actor
  state must be reproducible from it alone. [Details →](agentic-systems/journal.md)
- **Working Memory** — The LLM-facing context window, assembled incrementally from raw journal entries. System prompt +
  compressed history + recent events + current inbox message + runtime tools. Compression replaces oldest entries with
  LLM-generated summaries when context exceeds max size.
  [Details →](agentic-systems/journal.md)
- **Long-Term Memory Search** — Built-in tool `agent_ltm_search(query)` that searches compressed summaries. Results
  injected as tool results. Enables the LLM to recall past conversations and decisions.
  [Details →](agentic-systems/journal.md)
- **Continuation Set** — Runtime-owned data structure tracking pending ContinueRefs per topic. Gates resolution (won't
  fire if continuations pending), drives cancellation fan-out, detects deadline expiry. Survives journal compression.
  [Details →](agentic-systems/journal.md)

### Infrastructure

Background services that support agent execution.

- **LLM Gateway** — External process (not a mailbox actor) that agents connect to via gRPC streaming for inference.
  Manages provider connections (Ollama, OpenAI-compatible), auth rotation, failover, load balancing, and telemetry.
  Never sees agent journals or messages — only LLM request/response payloads. Model selection and turn management
  remain agent-local. [Details →](agentic-systems/agent-runtime.md)

## Where to Go Next

| If you want to... | Start here |
|---|---|
| Understand the actor model and agent internals | [Agent Structure](agentic-systems/agent-structure.md) |
| See how messages flow through the system | [Topic Model](agentic-systems/topic-model.md) |
| Learn the capability and security model | [Capabilities Model](agentic-systems/capabilities.md) |
| Understand the Exchange and message routing | [Exchange](agentic-systems/exchange.md) |
| Deep-dive into the Agent Runtime | [Agent Runtime](agentic-systems/agent-runtime.md) |
| Set up guardrails and policy | [Guardrails](agentic-systems/guardrails.md) |
| Explore system actors (supervisor, registry) | [System Actors](agentic-systems/system-actors.md) |
| Understand persistence and memory | [Journal & Persistence](agentic-systems/journal.md) |
| Review deployment scenarios | [Deployment Scenarios](agentic-systems/deployment-scenarios.md) |
| View the system architecture diagram | [System Diagram](agentic-systems/system-diagram.md) |
| Browse all terms | [Glossary](agentic-systems/glossary.md) |
