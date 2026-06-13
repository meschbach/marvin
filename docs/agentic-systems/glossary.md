# Glossary

An architecture for multi-user, multi-agent coordination built on an actor model.

* **Actor** — A unit of computation with a mailbox that processes messages sequentially, can spawn child actors, and
  carries a security context. Every agent is an actor, but not all actors are agents.
* **Agent** — An actor that uses an LLM for reasoning. Agents wrap MCP tools with contextual instructions, maintain a
  private journal, communicate via mailboxes, and carry a security context. An agent may have a guardrail assigned at
  spawn time for outbound message interposition.
* **Agent Runtime** — The deterministic execution environment that wraps every agent process. One runtime per agent.
  Exists in a lifecycle (`unprovisioned → running → terminating → terminated`) managed by the Supervisor. Enforces inbox
  processing priority (cancellations first, then guardrail rejections, then continuation replies, then new work), binds
  tools from the capability set including runtime protocol tools, gates resolution against pending continuations,
manages
  continuation deadlines, handles cancellation fan-out to sub-agents, validates LLM protocol actions against topic
state,
  and connects to an LLM Gateway for inference.
* **Cancellation** — A message signaling an agent to stop processing a specific topic. Must carry a `CancelRef`
  capability validated by the Exchange against an active topic. Always accepted by the runtime, bypassing LLM
  discretion.
* **Chain** — A named, ordered composition of [provider models](#provider-model) (and optionally other chains)
  with a selection strategy. Chains are defined in the Model Registry and selected by agents by name. Chains are
  immutable from the agent's perspective — agents never override chain behavior.
  [Details →](model-registry.md#chain)
* **Chain Engine** — A per-agent-runtime component that holds a reference to the agent's chains and tracks
  per-provider-model cooldown and exhaustion state. The runtime delegates model selection to the Chain Engine via
  `Select`, `Advance`, and `ReportSuccess`. Thread-safe — cooldown is shared across all topics in the runtime.
  [Details →](model-registry.md#chain-engine)
* **Capability** — A reference that confers authority: a mailbox address (can message that actor), a RoleActor mailbox
  (can request tool grants), a Connector mailbox (can send tool requests), a tool protocol handle (can call a protocol
  server), or a spawn token (can create children). Possession IS authorization in the actor model.
* **Child Agent** — An agent spawned by another agent with specific context, capability set, optional TTL, optional
  guardrail mailbox, and a (possibly reduced) security context.
* **CLI** — A synchronous, stdin/stdout Communication System supporting both embedded (in-process Exchange) and
  networked (gRPC-connected) modes.
* **Communication Channel** — A mechanism for transporting messages between agents and users on a specific
  communication system.
* **Communication System** — A bidirectional gRPC bridge between an external platform (Slack, Web UI, CLI, etc.) and
  the Exchange. Handles protocol translation, authentication, and subscription streams for agent output.
* **Connector** — An actor spawned by a RoleActor that holds a reference to a tool protocol server. Carries context
  about who it serves (user identity, scope, credentials) and what operations are permitted. All tool calls are messages
  sent to the Connector, which proxies to the underlying protocol and returns results through its optional guardrail
  chain.
* **ContinueRef** — A reference attached to outbound messages when an agent needs a response to resume a topic. Set by
  the runtime when the LLM calls `runtime_request_continuation`. Contains
  `{topic_id, reply_to_mailbox, deadline}`. The receiver echoes this in its response, allowing the runtime to route
  the reply back to the correct topic.
* **Correlation ID** — Transport-level identifier set by the original sender (CommSystem or parent agent). Opaque to
  agents. The Exchange echoes it on subscription stream events so the sender can correlate responses to requests.
* **Dead Letter Agent** — A system actor at mailbox://dead-letter.{scope} that receives undeliverable messages —
  targets whose TTL has expired, actors that no longer exist, or messages that exceeded mailbox depth. Configurable
  handling per scope: log, notify_sender, notify_parent, forward_to_handler. One provisioned per scope (user, org).
* **Event Source** — An inbound-only adapter that receives external events (webhooks, email, sensors) and injects them
  into an agent's mailbox via a gRPC `InjectMessage` call. Authenticated via x509. One-directional — no subscription
  stream back to the source. The Exchange stamps provenance information on each message so the target agent can apply
  appropriate trust processing.
* **Exchange** — A library (importable, embeddable) and standalone server providing message routing between actor
  mailboxes. Maintains an agent registry and routing table, enforces capabilities at delivery, routes outbound messages
  through guardrails, evaluates provenance-based guardrail policies, federates across machines, propagates OpenTelemetry
  trace context, hosts system actors (Supervisor, Registry, Dead Letter Agent) per scope, and enforces mailbox-level
  rate
  limits.
* **Guardrail Agent** — A user-managed agent placed in another agent's outbound message path for interposition.
  Receives outbound responses before delivery and decides whether to pass, reject, or mutate. May be placed between any
  two actors (agent → guardrail → agent, agent → guardrail → CommSystem, etc.). Exists in two flavors: Pass/Reject Guard
  and Mutating Guard. Guardrail agents hold zero tool protocol references by design. An exception exists for guardrails
  attached to a RoleActor, which may hold limited references for policy lookups only.
* **InternalUser** — A single principal identity mapped from one or more CommSystem accounts (Slack user, Web session,
  SSH key). All connections from the same person resolve to the same InternalUser, regardless of which platform they
  are using. The InternalUser links to one or more RoleActors (personal, org) that govern tool protocol grants.
* **Journal** — Every actor's append-only record of all activity. Two sub-levels: the **Raw Journal** (runtime-owned,
  append-only, full fidelity) and the **Memory Mechanism** (LLM-facing view compiled from raw entries). Both are
  described in the Journal & Persistence section.
* **LLM Gateway** — An external process (not a mailbox actor) that agents connect to via gRPC streaming for inference.
  Manages provider connections (Ollama, OpenAI-compatible, etc.), auth rotation, retry with backoff, per-endpoint health
  tracking, slow-start for recovering endpoints, load balancing, and operational telemetry (token counts, latency, cost
  attribution). Never sees agent journals or messages — only LLM request/response payloads.
  [Details →](llm-gateway.md)
* **Mailbox / Inbox** — A message queue belonging to an actor. Messages are processed sequentially by the owning actor.
  The mailbox address IS the capability to send messages to that actor. Mailboxes reserve dedicated capacity for system
  messages (cancellations, guardrail rejections, supervisor control) so high-priority messages are never blocked by a
  full topic queue.
* **MCP (Model Context Protocol)** — A protocol for connecting agents to external tools and data sources.
* **Memory Mechanism** — An LLM-facing view over the raw journal. A compressed reconstruction of conversation and
  decisions produced on-demand for context window injection. Retains narrative while eliding tool internals.
* **Model Registry** — The central configuration scope for inference options. Defines named providers, provider models,
  and chains. Dynamically updatable via administrative interface. Agents reference chains from the registry; they do not
  define their own model selection logic. [Details →](model-registry.md)
* **Mutating Guard** — A guardrail agent that evaluates outbound messages and responds with `{decision: "approve",
  modified_content: "..."}`. The Exchange forwards the modified version instead of the original. Used for redaction,
  sanitization, or formatting.
* **Pass/Reject Guard** — A guardrail agent that evaluates outbound messages and responds with either `{decision:
  "approve"}` or `{decision: "reject", reason: "..."}`. The Exchange forwards or blocks the message accordingly.
* **Provider** — A named configuration for an inference service endpoint. Multiple instances of the same provider type
  are supported (e.g., `ollama-primary`, `ollama-backup` with different hosts or API keys).
  [Details →](model-registry.md#provider)
* **Provider Model** — A pairing of a logical model with a specific provider (e.g., `gemma-4@openrouter`). Declares
  context window and optional max_wait for rate-limit tolerance.
  [Details →](model-registry.md#provider-model)
* **Provenance** — A trust-level tag stamped on every message by the Exchange indicating its origin. Examples:
  `event_source:inbound-email`, `comm_system:slack/user:U1234`, `internal_agent:research-agent`.
* **Raw Journal** — The runtime-owned, append-only record of every event: received and sent messages, tool
  invocations and results, LLM invocations, state transitions, cancellations, and resolutions. The full-fidelity
  source of truth. Never modified.
* **Registry** — A system actor at mailbox://registry.{scope} providing directory, discovery, and naming services within
  a scope. Agents register on spawn with UUID, aliases, type, capabilities, and role memberships. Supports lookup by
  UUID,
  alias, capability, role membership, and presence watching via TTL-based re-registration. One per scope (user, org).
* **Resolution** — A terminal signal emitted when the LLM calls `runtime_resolve_topic(reason)`. The runtime gates it:
  the topic is only marked Resolved when the outstanding continuations set is empty. If continuations are pending, the
  runtime defers the resolution and re-surfaces the pending state to the LLM.
* **RoleActor** — An actor that brokers capability grants. Holds no tool protocol references directly. On policy-
  approved request, spawns a Connector actor (with optional guardrails) and returns its MailboxRef as the grant. Uses
  guardrails for its own decision logic to support automated policy evaluation, escalation, and human-in-the-loop
  approval workflows.
* **Scope** — A namespace boundary for mailbox addresses, encoded as the TLD portion of an address
  (mailbox://{name}.{scope}). Scopes form a hierarchy: .user (individual identity), .org (organizational unit). The
  .local pseudo-scope resolves relative to the sender's security context by walking the scope chain inward → outward.
* **Supervisor** — A system actor at mailbox://supervisor.{scope} responsible for agent lifecycle management within its
  scope. Spawns agent actors, monitors their health, enforces TTL expiry, handles death notification to parents, and
  manages supervision strategies. One per scope (user, org). Created lazily on first use.
* **Security Context** — Identity, tenant, and capability set attached to an actor. Controls which actors may address
  it (target-side ACL) and what system-level actions it may perform.
* **Tool** — An capability exposed through a tool protocol. Tools are scoped to individual agents, which add contextual
  instructions specific to their domain (e.g., "this Vikunja bucket is for work projects"). MCP is one implementation of
  a tool protocol.
* **ToolAccessChain** — The grant produced by a RoleActor: a Connector actor that holds the tool protocol reference and
  context, with an optional GuardAgent interposing its outbound. Access is possession of the Connector's MailboxRef. The
  chain is a child of the RoleActor, giving the RoleActor lifecycle control (terminate on role change, expiry, etc.).
* **ToolProtocol** — The abstraction for external tool systems. Each protocol defines how a Connector communicates with
  the external server. MCP, local programs, Docker MCP, and HTTP MCP are all tool protocols.
* **Topic ID** — An application-level workflow identifier assigned by an agent when it pulls a message from its inbox.
  Groups related messages (a conversation, a task, a request) into a unit of work. Agent-scoped unique.
* **User** — A real person interacting with the system.
