# Agentic Systems

An architecture for multi-user, multi-agent coordination built on an actor model.

## Glossary

* **Actor** — A unit of computation with a mailbox that processes messages sequentially, can spawn child actors, and
carries a security context. Every agent is an actor, but not all actors are agents.
* **Agent** — An actor that uses an LLM for reasoning. Agents wrap MCP tools with contextual instructions, maintain a
private journal, communicate via mailboxes, and carry a security context. An agent may have a guardrail assigned at
spawn time for outbound message interposition.
* **Agent Runtime** — The deterministic execution environment that wraps every agent process. Enforces inbox processing
priority (cancellations first, then continuation replies, then new work), binds tools from the capability set, gates
resolution against pending continuations, manages continuation deadlines, handles cancellation fan-out to sub-agents, and
connects to an LLM Gateway for inference.
* **Cancellation** — A message signaling an agent to stop processing a specific topic. Must carry a `CancelRef`
capability validated by the Exchange against an active topic. Always accepted by the runtime, bypassing LLM
discretion.
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
* **ContinueRef** — A reference stamped on outbound messages when an agent needs a response to resume a topic. Contains
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
trace context, hosts system actors (Supervisor, Registry, Dead Letter Agent) per scope, and enforces mailbox-level rate
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
Manages provider connections (Ollama, OpenAI-compatible, etc.), auth rotation, failover, load balancing, and operational
telemetry (token counts, latency, cost attribution). Never sees agent journals or messages — only LLM request/response
payloads.
* **Mailbox / Inbox** — A message queue belonging to an actor. Messages are processed sequentially by the owning actor.
The mailbox address IS the capability to send messages to that actor.
* **MCP (Model Context Protocol)** — A protocol for connecting agents to external tools and data sources.
* **Memory Mechanism** — An LLM-facing view over the raw journal. A compressed reconstruction of conversation and
decisions produced on-demand for context window injection. Retains narrative while eliding tool internals.
* **Mutating Guard** — A guardrail agent that evaluates outbound messages and responds with `{decision: "approve",
modified_content: "..."}`. The Exchange forwards the modified version instead of the original. Used for redaction,
sanitization, or formatting.
* **Pass/Reject Guard** — A guardrail agent that evaluates outbound messages and responds with either `{decision:
"approve"}` or `{decision: "reject", reason: "..."}`. The Exchange forwards or blocks the message accordingly.
* **Provenance** — A trust-level tag stamped on every message by the Exchange indicating its origin. Examples:
`event_source:inbound-email`, `comm_system:slack/user:U1234`, `internal_agent:research-agent`.
* **Raw Journal** — The runtime-owned, append-only record of every event: received and sent messages, tool
invocations and results, LLM invocations, state transitions, cancellations, and resolutions. The full-fidelity
source of truth. Never modified.
* **Registry** — A system actor at mailbox://registry.{scope} providing directory, discovery, and naming services within
a scope. Agents register on spawn with UUID, aliases, type, capabilities, and role memberships. Supports lookup by UUID,
alias, capability, role membership, and presence watching via TTL-based re-registration. One per scope (user, org).
* **Resolution** — A terminal message with `is_resolution: true` and `resolution_reason` that an agent emits when it
considers a topic complete. Gated by the runtime — only fires when the topic's outstanding continuations set is
empty.
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

## Architecture

### System Diagram

```mermaid
flowchart TB
    subgraph External["External Platforms"]
        Slack
        Browser["Browser (WebSocket)"]
        Webhooks["Webhooks / Events"]
    end

    subgraph CommSys["Communication Systems (gRPC)"]
        SlackAdapter["Slack CommSystem"]
        WebUI["Web UI Backend"]
        CLI["CLI (networked mode)"]
    end

    subgraph Events["Event Sources (gRPC, inbound only)"]
        EmailReceiver["Email Receiver\n(IMAP → gRPC)"]
        WebhookReceiver["Webhook Receiver\n(HTTP → gRPC)"]
    end

    subgraph LLMGateway["LLM Gateway (external process)"]
        LLMGW["LLM Gateway\n(provider mgmt, failover,\nmonitoring, auth rotation)"]
        Provider1["Ollama"]
        Provider2["OpenAI-compatible"]
        ProviderN["..."]
    end

    subgraph Exchange["Exchange (library / server)"]
        direction TB
        Gateway["External Gateway\ngRPC endpoint"]
        AgentRegistry["Agent Registry"]
        Router["Message Router"]
        CA["Internal CA\nx509"]
        CapEnforce["Capability Enforcer"]
        GuardRoute["Guardrail Router"]
        GuardPolicy["Guardrail Policy Engine\n(provenance match)"]
    end

    subgraph ScopeInfra["Scope System Actors (per user / org)"]
        Super["Supervisor\n(lifecycle management)"]
        Reg["Registry\n(directory + discovery)"]
        DL["Dead Letter Agent\n(undeliverable messages)"]
    end

    subgraph Infrastructure["Infrastructure"]
        RAG[("Vector Store\n(RAG / semantic search)")]
    end

    subgraph Actors["Actor Layer"]
        direction TB
        subgraph AgentFrontend["User-Facing Agent"]
            Journal1["Journal"]
            AR1["Agent Runtime"]
        end
        subgraph AgentVikunja["Project Agent\n(Vikunja context)"]
            Journal2["Journal"]
            AR2["Agent Runtime"]
        end
        subgraph AgentEmail["Email Agent\n(routing rules)"]
            Journal3["Journal"]
            AR3["Agent Runtime"]
        end
        subgraph AgentResearch["Research Agent\n(TTL: 15m)"]
            Journal4["Journal"]
            AR4["Agent Runtime"]
        end
        subgraph AgentChild["Sandbox Agent\n(remote, sandboxed)"]
            Journal5["Journal"]
            AR5["Agent Runtime"]
        end
    end

    subgraph Roles["Role Actors"]
        PersonalRole["Personal RoleActor\n(brokers grants)"]
        OrgsRole["Family RoleActor\n(brokers grants)"]
    end

    subgraph Connectors["Connectors (per-grant actors)"]
        CV["Vikunja Connector\n(protocol ref + context)"]
        CE["Email Connector\n(protocol ref + context)"]
        CSearch["Search Connector\n(protocol ref + context)"]
        CShell["Shell Connector\n(protocol ref + context)"]
    end

    subgraph Guardrails["Guardrail Agents (user-defined)"]
        GR1["PII Redactor\n(mutating guard)"]
        GR2["Policy Check\n(pass/reject guard)"]
    end

    subgraph ProtocolServers["Tool Protocol Servers"]
        P1["Vikunja API\n(MCP)"]
        P2["Email Server\n(MCP)"]
        P3["Web Search\n(MCP)"]
        P4["Shell/Sandbox\n(Local Program)"]
    end

    Slack -->|socket mode| SlackAdapter
    Browser -->|WebSocket| WebUI

    SlackAdapter -->|gRPC| Gateway
    WebUI -->|gRPC| Gateway
    CLI -->|gRPC| Gateway

    EmailReceiver -->|InjectMessage| Gateway
    WebhookReceiver -->|InjectMessage| Gateway

    Gateway -->|route to inbox| AgentFrontend
    Gateway -->|route to inbox| AgentVikunja
    Gateway -->|route to inbox| AgentEmail
    Gateway -->|route to inbox| AgentResearch

    Gateway <-->|subscribe / stream| SlackAdapter
    Gateway <-->|subscribe / stream| WebUI
    Gateway <-->|subscribe / stream| CLI

    AgentFrontend -->|spawn with caps| AgentVikunja
    AgentFrontend -->|spawn with caps| AgentEmail
    AgentFrontend -->|spawn with caps + TTL| AgentResearch
    AgentResearch -->|spawn sandboxed| AgentChild

    AgentFrontend -->|request grant| PersonalRole
    AgentFrontend -->|org role membership| OrgsRole
    PersonalRole -->|spawns with context| CV
    PersonalRole -->|spawns with context| CE
    OrgsRole -->|spawns with context| CSearch
    OrgsRole -->|spawns with context| CShell

    AgentVikunja -->|has ConnectorRef| CV
    AgentEmail -->|has ConnectorRef| CE
    AgentResearch -->|has ConnectorRef| CSearch
    AgentChild -->|has ConnectorRef| CShell

    CV -->|has protocol ref| P1
    CE -->|has protocol ref| P2
    CSearch -->|has protocol ref| P3
    CShell -->|has protocol ref| P4

    CV -.->|compress journal| RAG
    CE -.->|compress journal| RAG
    CSearch -.->|compress journal| RAG
    CShell -.->|compress journal| RAG

    AR1 -->|gRPC streaming| LLMGW
    AR2 -->|gRPC streaming| LLMGW
    AR3 -->|gRPC streaming| LLMGW
    AR4 -->|gRPC streaming| LLMGW
    AR5 -->|gRPC streaming| LLMGW

    LLMGW -->|provider routing| Provider1
    LLMGW -->|provider routing| Provider2
    LLMGW -->|provider routing| ProviderN

    CV -.->|outbound → guardrail| GR1
    GR1 -.->|forward| GR2

    PersonalRole -.->|policy guardrail| GR2

    GuardPolicy -.->|injects| GuardRoute

    Super -.->|manages| AgentFrontend
    Super -.->|manages| AgentVikunja
    Super -.->|manages| AgentEmail
    Super -.->|manages| AgentResearch
    Reg -.->|maintains| AgentMap[("Mailbox→Actor Map")]
    CA -.->|signs| Certs["x509 Certs\nfor CommSystems & Event Sources"]
    CapEnforce -.->|validates| Caps["Capabilities\nat routing time"]
    DL -.->|handles| DeadQ[("Dead Letter Queue")]
```

### Naming & Addressing

Actor mailbox addresses use a structured URI scheme with scope-encoded namespacing:

```
mailbox://{name}.{scope}[:{parent-scope}...]/{path?}
```

**Address format:**
- **Scheme**: `mailbox://` — indicates a mailbox address (possession IS capability to send)
- **Name**: The entity's identifier (e.g., `registry`, `supervisor`, `research-agent`, `alice`)
- **Scope**: Encodes what kind of namespace the entity belongs to, using TLD-style suffixes

**Scope types:**

| Type | Purpose | Examples |
|------|---------|---------|
| `.local` | Resolved relative to sender's scope chain | `registry.local`, `supervisor.local` |
| `.user` | Individual identity | `alice.user`, `bob.user` |
| `.org` | Organizational unit | `accounting.org`, `engineering.org`, `family.org` |

**.local resolution:**

When an agent references `mailbox://registry.local`, the Exchange resolves it by
walking the sender's scope chain from innermost to outermost, using the first match:

```
Sender: agent within alice.accounting.org
  → check mailbox://registry.alice.user       (personal scope)
  → check mailbox://registry.accounting.org   (org scope)
  → check mailbox://registry.company.org      (parent org scope)
  → use mailbox://registry.system             (Exchange root fallback)
```

This gives every actor a default view of "their own world" while supporting explicit
cross-scope references (`mailbox://registry.engineering.org`).

**Scope nesting:**

Scopes form a two-level hierarchy: `.user` within `.org`. A user may belong to an
org, giving them a scope chain of `{user}.{user} → {org}.org → {parent-org}.org`.

The path component (`/{path?}`) is reserved for future extension.

### Actor Model

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

### Agent Structure

Each agent contains:

1. **Mailbox** — Incoming message queue (address = capability to send)
2. **Journal** — Append-only record of all activity: received messages (with origin mailbox), sent messages, tool
invocations and results, internal state transitions. Short-term entries are in the LLM context window; long-term
entries are compressed and indexed for retrieval.
3. **Inference Platform** — LLM integration for reasoning
4. **Capability Set** — Held references: mailbox addresses of known actors, RoleActor mailbox, Connector mailbox
addresses, tool protocol handles, spawn tokens
5. **Target-side ACL** — Optional allow/deny list for incoming senders
6. **Guardrail Mailbox** — Optional mailbox address of a guardrail agent through which all outbound messages are routed
7. **Child Registry** — References to spawned child actors
8. **Continuation Tracker** — A set of outstanding `ContinueRef` requests keyed by topic ID. Runtime-owned (not
   LLM-visible for mutation). The runtime checks this set before allowing resolution to fire and before dispatching
   cancellation fan-out notifications.
9. **Topic Router** — Maps incoming messages to existing topics or spawns new topics based on envelope headers
   (`topic_id` for new messages, `continue_ref` for replies, `cancellation` for cancels). A message that lacks any
   topic header gets a fresh topic ID assigned.

### Communication Flow

Messages carry two independent identifiers:

- **Correlation ID** (transport level): Set by the original sender (CommSystem or parent agent). The Exchange
  passes it through and echoes it on all subscription stream events for that message tree. Agents never interpret it.
- **Topic ID** (application level): Assigned by the target agent's runtime when processing begins. All messages
  within a conversation unit share the same topic ID. The agent's LLM sees the topic ID in its working memory and
  uses it for workflow tracking.

Additionally, messages may carry:

- **continue_ref**: An outbound message indicating "I need a response to continue topic T1." Contains the sender's
  topic ID, the sender's mailbox address, and an absolute deadline. The receiver echoes this in its response.
- **cancellation**: An inbound message with a `CancelRef` capability targeting a specific topic ID. The runtime
  processes this before any LLM invocation — always accepted, bypassing LLM discretion.
- **is_resolution**: A flag on an outbound message marking a topic as complete. The runtime verifies no outstanding
  continuations before forwarding.

```
CommSystem ──SendMessage(correlation_id=X)──→ Exchange ──→ Agent Mailbox
                                                               │
                                                   Runtime assigns topic_id: T1
                                                   LLM processes, decides to delegate
                                                               │
                                                               ├── SendMessage(continue_ref={topic_id: T1,
                                                               │   reply_to: A, deadline: <absolute>})
                                                               │   └──→ Agent B
                                                               │
                                                               ├── Agent B responds
                                                               │   ←── continue_ref echoed back
                                                               │
                                                               ├── Runtime routes reply to T1,
                                                               │   LLM resumes, produces resolution
                                                               │
                                                               ├── is_resolution: true, reason: "completed"
                                                               │   └──→ Guardrail(s) → Exchange
                                                               │       → CommSystem stream (correlation_id=X echoed)
                                                               │
                                                               └── CancelTopic(topic_id=T1) (if interrupted)
                                                                   └──→ Runtime accepts (privileged), journals,
                                                                        notifies B via cancel_topic(T1)
 ```

### Topic Model & Async Protocols

The system uses topic IDs to group messages into workflow units and a continuation protocol for asynchronous
agent-to-agent coordination.

#### Topic State Machine

Every topic transitions through a lifecycle owned by the agent runtime, not the LLM:

```
                        ┌──────────────────┐
                        │  New (unassigned) │
                        └────────┬─────────┘
                                 │ agent pulls message, assigns topic_id
                                 ▼
                        ┌──────────────────┐
                        │     Active       │
                        └────────┬─────────┘
                                 │ agent sends message with continue_ref
                                 ▼
               ┌──────────────────────────────────┐
               │     WaitingForContinuations       │
               │  (tracking: set of outstanding)   │
               └──────┬───────────────┬────────────┘
                  ▲   │               │
                  ║   │ reply arrives │ cancellation arrives
                  ║   ▼               ▼
                  ║  ┌──────────┐  ┌──────────┐
                  ║  │  Active  │  │ Cancelled│←── runtime always acts
                  ║  └──────────┘  └────┬─────┘
                  ║                     │
                  ║  LLM says done,     │ runtime fan-outs cancel
                  ║  continuations      │ to sub-agents, journals
                  ║  set is empty       │ both events
                  ║                     ▼
                  ║              ┌──────────────┐
                  ║              │  Resolved     │
                  ║              │  (resolution  │
                  ║              │   message     │
                  ║              │   sent)       │
                  ║              └──────────────┘
                  ╚══════════════════════════════╝
                                      (on re-engagement)
```

#### Continuation Protocol

When an agent needs a response from another agent to continue a topic, it stamps a `continue_ref` on the outbound
message:

```
continue_ref {
    topic_id: string           // sender's local topic ID
    reply_to_mailbox: string   // sender's mailbox address
    deadline: timestamp        // absolute wall clock
}
```

The `deadline` is an absolute timestamp rather than a relative TTL. Both the sender and the receiver can
independently evaluate whether a continuation has expired without remembering when the clock started. This assumes
agents' clocks are synchronized within a few seconds.

**Fan-out**: A topic may have multiple outstanding continuations concurrently. The runtime tracks them as a set
keyed by topic ID. Resolution is gated on all continuations being resolved or expired.

**Continuation deadline expiry**: On each inbox poll, the runtime checks all pending `continue_ref.deadline` values
against the wall clock. Expired deadlines produce a `continuation_deadline_expired` raw journal entry and may
inject a notification into the agent's working memory. The LLM decides how to respond.

**Stale continuations**: If a continuation reply arrives for a topic that has already been resolved or cancelled,
the runtime journals it as `stale_continuation` and surfaces it on the next idle inbox poll. The LLM may act on it
or ignore it.

#### Cancellation Protocol

Cancellation requires a `CancelRef` capability targeting an active topic. The Exchange validates the sender holds
this capability before routing the cancellation message.

The runtime always accepts cancellation from the inbox, bypassing LLM discretion. Processing order:

1. Runtime receives `cancel_topic(topic_id=T1, reason, CancelRef)` in the inbox.
2. Runtime journals `cancellation_received` with provenance (who sent, reason).
3. Runtime transitions T1 to Cancelled state.
4. Runtime fan-outs `cancel_topic(T1)` to all outstanding continuations' target mailboxes, notifying sub-agents
   so they can suppress work.
5. On each fan-out notification, the runtime journals `cancellation_fanout`.
6. When the LLM next processes, it sees the cancellation in working memory and may act accordingly.

If a topic is cancelled while awaiting a response, the runtime journals both the cancellation receipt and the
outstanding continuation as pending. When the delayed response eventually arrives, it is journaled as
`stale_continuation` for the topic.

#### Resolution Protocol

When the LLM determines a topic is complete, it emits a message with `is_resolution: true` and a
`resolution_reason`. The runtime intercepts this and checks the topic's continuations set:

- **If continuations set is empty**: The resolution message is forwarded through guardrails → Exchange →
  destination. The topic is marked Resolved.
- **If continuations set is non-empty**: The runtime defers the resolution, journals "Deferred resolution for T1 —
  outstanding continuations: B, C", and does not forward the message. The LLM continues processing (or handles
  the pending state).

For the synchronous bridge (CLI, Web), the CommSystem waits for an `is_resolution` message with matching
`correlation_id` on the subscription stream before returning control to the user.

#### Agent-to-Agent Async

When agent A needs data from agent B to continue topic T1:

1. A sends B a message containing `continue_ref = {topic_id: T1, reply_to: A, deadline: <time>}`.
2. A's runtime adds this to T1's continuations set, journals the outbound message, and transitions T1 to
   WaitingForContinuations.
3. A returns to processing its inbox (other topics, other messages).
4. B eventually responds; the response carries the echoed `continue_ref`.
5. A's topic router matches the incoming reply to T1, transitions T1 back to Active.
6. The LLM sees the pending journal entry (from step 2) plus the response, and resumes T1.

No blocking, no special async runtime. The journal IS the continuation store — the LLM reads past context to
understand what it was waiting for and why.

### Exchange

The Exchange is both a **library** (importable, embeddable) and a **standalone server**. Its responsibilities:

- **Registry**: Maps actor identities to mailbox locations
- **Routing**: Delivers messages to the correct destination mailbox
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

An Exchange may run embedded (in-process, no persistence) or as a server (with optional persistence for routing table
and agent state).

### Exchange External Gateway

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

### System Actors

The Exchange provisions a set of built-in system actors for every scope (user and
org). These are ordinary actors — same mailbox model, same message patterns — with
reserved addresses and well-defined responsibilities.

**Every scope gets its own triad:**

```
alice.user scope:
  mailbox://supervisor.alice.user
  mailbox://registry.alice.user
  mailbox://dead-letter.alice.user

accounting.org scope:
  mailbox://supervisor.accounting.org
  mailbox://registry.accounting.org
  mailbox://dead-letter.accounting.org
```

**Supervisor** (`mailbox://supervisor.{scope}`) — Agent lifecycle management:
- Spawns agent actors within its scope on behalf of authorized parents
- Monitors agent health; applies supervision strategy on death (notifies parent)
- Enforces TTL expiry: graceful shutdown, then dead-letter routing
- Failure is scoped — a supervisor failure does not escalate to parent scopes

**Registry** (`mailbox://registry.{scope}`) — Directory and discovery:
- Agents register on spawn with UUID, aliases, type, capabilities, role memberships
- Lookup by UUID, alias, capability prefix, or role membership
- Presence watching via TTL-based re-registration (missed check-in = stale entry)
- Supports RoleActor discovery: `LookupRoleActors(InternalUser="alice")` returns
  `[MailboxRef(personal-role), MailboxRef(family-role)]`

**Dead Letter Agent** (`mailbox://dead-letter.{scope}`) — Unroutable message
handling:
- Receives messages when the target agent's TTL has expired or the mailbox does
  not exist
- Configurable handling per scope: `log` (discard with log), `notify_sender`,
  `notify_parent`, `forward_to_handler`
- Messages carry provenance and original sender for policy-based routing

All three are provisioned lazily — created on first use rather than at scope
creation. This is an optimization; the behavior is equivalent to eager creation.

### Event Sources & Inbound Adapters

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

### Guardrails

Guardrails are user-defined agents placed in the outbound message path of another agent. They implement
**interposition** — every response an agent sends passes through its guardrail before reaching the destination. This is
a general mechanism: guardrails can sit between any two actors, including agent → another agent and agent → CommSystem.

**Configuration:**

When spawning an agent, the parent may designate a `guardrail_mailbox`. All outbound messages from that agent are
routed through the guardrail before delivery. The Exchange performs the routing automatically.

**Routing flow:**

```
Agent → Outbound Response → Exchange → Guardrail Agent → Exchange → Destination
                                             │
                                    approve ─┤
                                             │
                                    reject ──┴──→ Dead letter / notification
```

**Two types:**

- **Pass/Reject Guard**: Evaluates the message and responds with `{decision: "approve"}` or `{decision: "reject",
reason: "..."}`. The Exchange forwards or blocks the message accordingly. If rejected, the Exchange may notify the
origin agent or discard silently based on configuration.

- **Mutating Guard**: Evaluates the message and responds with `{decision: "approve", modified_content: "..."}`. The
Exchange forwards the modified version. Used for redaction (PII stripping), sanitization (removing markup), or
formatting (rewriting for a specific output channel).

**Composability:**

Guardrails chain naturally. The spawner specifies a sequence:

```
Agent → PII Redactor (mutate) → Policy Check (pass/reject) → Destination
```

The Exchange routes through each guardrail in order. If any guardrail rejects, the chain stops.

**Design constraints:**

- Guardrail agents hold **zero tool protocol references** by design. They cannot call external tools, cannot cause side
effects, cannot exfiltrate. Their only function is to inspect and respond to messages. An exception exists for
guardrails attached to a RoleActor, which may hold limited references for policy lookup operations only (e.g., querying
a directory or policy database). These are never for production tool calls on behalf of users.
- Guardrails are **not built into the Exchange**. The Exchange provides the routing pattern; users define guardrails as
ordinary agents with whatever logic their use case requires (keyword filtering, PII detection, content policy, manual
approval workflows).
- Guardrails themselves may have guardrails (recursive composition), though this adds latency.

**Exchange-Level Guardrail Policy:**

In addition to spawn-time guardrails set by the parent, the Exchange supports
**provenance-based guardrail policies** configured at the Exchange level. These
policies match outbound messages by provenance pattern and inject mandatory
guardrails that are **prepended** to the agent's own guardrail chain.

```hcl
guardrail_policy "event-source-safety" {
  match {
    provenance = "event_source:*"      // glob-style prefix match
  }
  guardrails = ["mailbox://pii-redactor.system"]
  on_reject  = "notify_parent"         // | "notify_sender" | "log" | "drop"
}
```

**Guardrail chain merge order** (outermost → innermost):

```
Exchange-level policies (prepended, by provenance match)
  → RoleActor spawn-time policy
    → Parent spawn-time guardrails
```

Policies are dynamically definable at runtime via admin commands and persisted by
the Exchange. The `match` block supports `provenance` and `target` fields (both
optional — omitting means "match all").

**Guardrail rejection feedback:**

When a guardrail rejects a message, the Exchange generates a `GuardrailRejection`
system message containing `{guardrail_id, topic_id, original_message_ref, reason}`
and injects it into the originating agent's inbox — the same mechanism used for
cancellation messages. The Agent Runtime processes it before the next LLM
invocation (same priority as cancellations) and journals it into working memory so
the LLM can handle the rejection.

This means guardrail rejections are surfaced to the sending agent's LLM as an
actionable notification, not silently discarded.

**Capabilities Model**

The system uses an **object-capability (ocap) model**. A capability is a reference that confers authority. Possession
of the reference IS authorization — there is no ambient authority or global ACL.

**Primitive capabilities:**

| Capability | What it grants | Granted by | Typical constraints |
|------------|---------------|------------|-------------------|
| `MailboxRef(id)` | Send messages to the target actor's mailbox | Parent on spawn; Exchange on registration | `delegation_policy`, `expires_at` |
| `RoleActorRef(role_mailbox)` | Request tool protocol grants from a RoleActor | Exchange on bootstrap; org admin on assignment | `expires_at` for temporary admin assignments |
| `ConnectorRef(connector_mailbox)` | Send tool requests to a Connector actor | RoleActor on grant approval | `expires_at`, `delegation_policy` (same-org by default) |
| `Spawn` | Create child actors with a subset of own capabilities | Parent on spawn; system bootstrap | `max_children`, `max_ttl`, `allowed_protocols` |
| `ToolProtocolRef(server_handle, tool_filter)` | Connect to and use a tool protocol server (e.g., MCP, optionally filtered to a subset of tools) | Admin registration; Connector holds these, agents never hold them directly | `tool_filter` |
| `CancelRef(topic_id)` | Send a cancellation message for a specific topic | Parent on spawn; Exchange on registration for system actors | Scoped to single `topic_id` |

**How capabilities flow:**

1. **Bootstrap**: The Exchange creates an InternalUser with an associated personal RoleActor and a root agent actor.
   The root actor receives a `RoleActorRef` to its personal RoleActor (and optionally org RoleActors). The RoleActor
   receives a `Spawn` token and a `ToolProtocolRef` to each tool protocol server the user's role may request.
   The RoleActor holds **zero** direct tool references itself — it spawns Connector actors on demand.

2. **Grant request**: When the root actor needs tool access, it sends a message to its RoleActor requesting a specific
   tool protocol grant (e.g., "Vikunja MCP with read_tasks scope"). The request is async — the agent continues
   processing its inbox while awaiting the RoleActor's response.

3. **Policy evaluation**: The RoleActor evaluates the request using its internal policy, which may involve guardrail
   agents for escalation, human-in-the-loop approval, or automated rule matching. Policy evaluation may take seconds
   or minutes depending on complexity.

4. **Grant**: On approval, the RoleActor spawns a Connector actor as its child. The Connector holds the
   `ToolProtocolRef` to the underlying server, carries context about who it serves (user identity, scope,
   credentials), and may have a guardrail chain on its outbound. The RoleActor returns the Connector's `MailboxRef`
   to the requesting agent. The agent now holds a `ConnectorRef`.

5. **Usage**: The agent sends tool call messages directly to the Connector. The Connector proxies to the tool protocol
   server and returns results through its guardrail chain. The RoleActor is **not** in the hot path.

6. **Delegation**: An actor may forward a `MailboxRef` or `ConnectorRef` to another actor (e.g., parent passes a
   ConnectorRef to a child agent). The Exchange logs delegation for audit.

7. **Delegation constraints**: When forwarding a reference, the granter may attach a delegation policy at grant time
   (see Constrained References below). The Exchange validates constraints at routing time — if a message arrives via
   a reference whose policy forbids the forwarding chain, it is rejected.

8. **Revocation**: Three-tier revocation model:

   a. **Reference expiry (primary mechanism)** — Every capability descriptor carries an optional `expires_at` field.
      The Exchange checks it at routing time and rejects expired references. Agents may auto-renew before expiry via
      `RenewRef(reference_id, duration)` to the granter. Denial of renewal IS revocation. This is the standard path:
      deterministic, no registry lookup, consistent with the continuation deadline system (§ Continuation Protocol).

   b. **Target-side ACL mutation (emergency override)** — For immediate revocation that cannot wait for expiry, the
      granter sends `UpdateACL(actor_id, deny=[sender_mailbox])` to the target actor. The target adds the sender to
      its existing target-side ACL (§ Agent Structure, item 5). The Exchange already checks ACLs at routing time.
      This is the kill switch: no new mechanism, uses existing ACL infrastructure.

   c. **Intermediary termination (heavy option)** — Already exists for Connectors: terminate the intermediary actor
      and all references to it go stale. For critical MailboxRefs requiring instant full revocation, route through
      a lightweight proxy actor whose lifecycle the granter controls. This option incurs a per-message hop cost and
      is applied case-by-case, not as the default.

   The three tiers compose: expiry handles routine revocation (overdue grants, rotation), ACL mutation handles
   emergencies (compromised agent, policy violation), and intermediary termination handles structural revocation
   (role change, user offboarding).

**Capability flow diagram:**

```
Bootstrap:
Exchange → spawns InternalUser
  ├── Personal RoleActor (holds Spawn, delegated ToolProtocolRefs)
  │     (holds no tool refs itself)
  └── Root Agent (holds RoleActorRef)

Grant request:
RootAgent ──"grant mcp:vikunja:read_tasks"──→ RoleActor
  RoleActor evaluates policy (auto | guardrail | human)
  RoleActor spawns Connector (child)
    Connector: ToolProtocolRef(mcp, vikunja, read_tasks), context: "Alice"
  RoleActor ←── MailboxRef(Connector) ──→ RootAgent

Usage (RoleActor out of path):
RootAgent ──"list_projects()"──→ Connector
  Connector proxies to Vikunja MCP
  Guardrail interposes result
Connector ←── result ──→ RootAgent
```

**Explicit capabilities for system actions:**

Address-based capabilities (you have the mailbox address = you can message) are sufficient for agent-to-agent
communication. The following actions additionally require an **explicit capability descriptor** validated by the
Exchange at routing time:

- `Spawn` — validates the caller has spawn authority with matching constraints (e.g., max TTL, allowed tool protocols)
- `RegisterAgent` — registering a new actor with the Exchange's registry
- `ModifyRouting` — changing the routing table or federation links

These system capabilities carry constraints encoded in the descriptor:

```go
type CapabilityDescriptor struct {
    // Common fields
    ExpiresAt      *time.Time         // nil = no expiry; Exchange rejects expired descriptors at routing time
    GrantedBy      ActorID            // identity of granter (for audit trail)

    // Type-specific constraints
    MaxChildren    int                // Spawn only
    MaxTTL         time.Duration      // Spawn only
    AllowedProtocols []ProtocolPrefix // Spawn only: e.g., "mcp://vikunja/*", "local://*"
    ToolFilter     []string           // ToolProtocolRef only: e.g., ["read_tasks", "write_tasks"]
    TopicID        string             // CancelRef only: which topic this cancel capability targets
}
```

**Constrained References (Delegation Policy):**

Every mailbox reference may optionally carry delegation constraints encoded as a `ConstrainedRef` wrapping the
base `MailboxRef`. The Exchange validates these constraints at routing time — possession is still authorization,
but possession within the bounds encoded in the reference:

```go
type DelegationPolicy int

const (
    DelegationUnrestricted DelegationPolicy = iota // may be forwarded to any actor
    DelegationSameOrg                              // may only be forwarded within the same org tenant
    DelegationSameUser                             // may only be held by the grantee
    DelegationNoForward                            // may not be forwarded; holder may use but not delegate
)

type ConstrainedRef struct {
    Target         MailboxID         // the underlying mailbox address
    GrantedBy      ActorID           // identity of the granter (for audit)
    ExpiresAt      *time.Time        // nil = no expiry
    Delegation     DelegationPolicy  // how this ref may be forwarded
    GrantedAt      time.Time         // for audit trail
    GrantedTo      ActorID           // original grantee (for DelegationSameUser enforcement)
}
```

**How constraints are enforced at routing time:**

When the Exchange routes a message, it unwraps the reference chain. For each hop in the chain, it checks:

1. **Expiry**: If any reference in the chain's `ExpiresAt` is in the past, the message is rejected with
   `ErrExpiredReference`. No registry lookup — purely time-based.

2. **Delegation policy**: If a message arrives via a forwarded reference, the Exchange walks the delegation
   chain. A `DelegationNoForward` reference that was forwarded (i.e., the sender is not the `GrantedTo` actor)
   is rejected. A `DelegationSameOrg` reference forwarded to an actor in a different org tenant is rejected.

3. **Audit logging**: Every delegation hop is logged by the Exchange with `{granted_by, granted_to, policy,
   forwarding_actor}` for the audit trail. This resolves the "should the Exchange log or restrict this?" open
   question by answering: both — log always, restrict per policy.

**Who sets constraints:**

| Granter | Typical policy | Rationale |
|---------|---------------|-----------|
| Parent spawning a child | `DelegationSameUser` | Child should use the reference, not hand it to third parties |
| RoleActor granting a Connector | `DelegationSameOrg` | Connector may be forwarded within the user's org but not across org boundaries |
| Agent forwarding a reference to a child | Inherited or tightened | Forwarding cannot loosen the original policy (monotonic constraining) |
| Exchange bootstrap | `DelegationUnrestricted` | Root actors are trusted; policy is their responsibility |

**Monotonic constraining:** When actor A holds a `ConstrainedRef` with policy P, and A forwards it to B, the
forwarded reference's policy must be P' where P' >= P in restrictiveness (i.e., constraining is monotonic —
you can only tighten, never loosen). The Exchange enforces this at forwarding time. This prevents a
restricted reference from being "loosened" by an intermediary.

### Agent Spawning

An agent spawns a child by providing:

- Context and behavioral instructions
- A **granted subset** of the parent's capabilities (MailboxRefs, ConnectorRefs, Spawn token)
- Optional time-to-live (TTL)
- Optional guardrail mailbox (or chain of guardrail mailboxes) for all outbound responses
- Security context (inherited or reduced from parent)
- Optional **continuation_deadline** — overrides the default continuation deadline for `ContinueRef` messages
  directed at this child. If unset, inherits the spawning agent's default.

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

### Tools & Tool Protocols

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

**Supported tool protocols:**

| Protocol | Description |
|----------|-------------|
| **MCP** (Model Context Protocol) | Standard tool protocol for connecting to external tools and data sources |
| **Local Program** | Subprocess execution on the host |
| **Docker MCP** | Containerized tool execution |
| **HTTP MCP** | REST-based tool access |
| (Extensible) | New protocols register a Connector type |

### Journal & Persistence

Every actor operates a **two-level journal architecture**: a raw append-only journal owned by the runtime, and a
working memory that serves as the LLM's context window, assembled incrementally from journal events.

#### Raw Journal (Runtime-Owned)

A full-fidelity, append-only record of every event the actor participates in. Never modified after append. The
raw journal is the source of truth — full actor state must be reproducible from it alone.

Entry structure:

```
Entry {
    seq: uint64                    // monotonic counter
    wall_clock: timestamp          // when the event occurred
    kind: enum {
        inbox_message,             // includes sender, provenance, topic_id
        outbox_message,            // includes destination, correlation_id
        tool_invocation,           // tool name + arguments
        tool_result,               // result payload
        llm_invocation,            // prompt + response tokens, model ID, token count
        llm_output,                // generated text from the LLM
        state_transition,          // topic_id: Active → WaitingForContinuations
        cancellation_received,     // who sent, reason, CancelRef
        cancellation_fanout,       // runtime notified sub-agent
        resolution,                // topic_id resolved/cancelled, with reason
        continuation_deadline_expired,
        stale_continuation,
        compression,               // LLM compressed working memory
        ltm_search,                // LLM searched its history
    }
    data: bytes                    // type-specific payload
    topic_id: string               // may be nil for system entries
    correlation_id: string         // passed through from original sender
    token_count: uint64            // tokens consumed (for context accounting)
}
```

#### Working Memory (LLM Context)

The agent's working memory is the context sent to the inference provider on each LLM invocation. It is assembled
incrementally:

```
┌─────────────────────────────────────────────┐
│ System Prompt (static, set at launch)        │
├─────────────────────────────────────────────┤
│ Compressed History (produced by LLM,         │
│   may be absent if no compression needed)    │
├─────────────────────────────────────────────┤
│ Recent Events (appended incrementally)       │
│   llm_output, tool results,                 │
│   ltm_search results, etc.                  │
├─────────────────────────────────────────────┤
│ Current Inbox Message + Topic State          │
│   "New message from User on topic T1"       │
└─────────────────────────────────────────────┘
```

Fresh assembly and incremental append produce **functionally equivalent** contexts. Incremental append is a
runtime optimization — each new event is appended as text to the recent events block. The runtime monitors total
token count against the configured maximum.

#### Compression

When the working memory exceeds the maximum context size, the runtime triggers compression. Compression may also
be triggered by the inference provider signaling that it requires compression. In either case the effective
maximum becomes:

```
new_max = min(configurable_max, inference_provider_signal_max)
```

The compression process:

1. Runtime freezes the current working memory.
2. Runtime constructs a compression prompt: "Compress the following agent history into a concise summary
   preserving key facts, decisions, and unresolved state: <oldest portion of working memory>"
3. The LLM produces a compressed summary.
4. Runtime replaces the oldest portion of working memory with the summary.
5. The compression event is journaled as kind `compression`, including the compression prompt and resulting
   summary, the seq ranges of raw journal entries compressed, and token counts before and after.
6. The summary is indexed into a searchable memory store with pointers to the raw journal seq range it covers.

The compressed summary is plain text in the context — no special data structure. The LLM reads it as part of its
history. Compression does not modify the raw journal.

#### Long-Term Memory Search

Every agent holds a built-in ConnectorRef for `agent_ltm_search`. It exposes:

```
agent_ltm_search(query: string) → ToolResult {
    results: [{seq_range, summary, relevance}]
}
```

The LLM can explicitly search its own history by calling this tool. Search results are injected into working
memory as tool results. The search invocation and result are journaled as kind `ltm_search`.

The searchable memory store indexes compressed summaries. The storage mechanism is abstracted — the architecture
describes the capability to recall memories, not the implementation.

#### Continuation Set (Runtime-Owned)

The set of pending `continue_ref` for each active topic is owned by the Agent Runtime, separate from both the raw
journal and working memory. It is an in-memory data structure that:

- Gates resolution (does not fire if continuations non-empty)
- Drives cancellation fan-out
- Detects deadline expiry (checked on each inbox poll)
- Routes incoming continuation replies to the correct topic

The continuation set survives compression — compressing journal entries does not affect pending continuations.

#### Persistence

All actors run ephemerally by default. When persistence is configured, the raw journal, searchable memory index,
and continuation set are persisted to durable storage. The storage mechanism is abstracted — the architecture
does not prescribe files, databases, or object stores.

### Agent Runtime

Every agent process executes inside an **Agent Runtime** — a deterministic execution environment for the agent process, wrapping the LLM and Connector tool
infrastructure that enforces the topic model, capability constraints, and actor invariants.

#### Inbox Processing Priority

The runtime processes messages from the agent's mailbox in strict priority order:

1. **Cancellation messages** — Always processed first, bypassing LLM discretion. The runtime journals
   "cancellation received from X for topic T1," transitions T1 to Cancelled, and fan-outs `cancel_topic` to all
   tracked continuations.
2. **Continuation replies** — Messages carrying a `continue_ref` are matched to their active topic. The runtime
   transitions the topic back to Active and journals the arrival. If the topic is already Resolved or Cancelled,
   the message is journaled as `stale_continuation`.
3. **New work** — Messages with no topic assignment get a fresh topic ID. Messages with an existing topic ID are
   routed to that topic's activity stream.

#### LLM Context Assembly

Before each LLM invocation, the Agent Runtime assembles the context:

- Current topic's working memory (raw journal entries as text, or compressed summary)
- Topic state: Active / WaitingForContinuations / Resolved / Cancelled
- Pending continuations set (read-only: "You are waiting on responses from B and C for topic T1")
- Capability set: only ConnectorRefs the agent holds are bound as available tool targets
- Provenance of the incoming message

The assembled context is sent to the LLM Gateway via gRPC streaming rather than a
direct provider connection. The Agent Runtime embeds a Gateway client that handles
the streaming RPC.

#### LLM Gateway Integration

Each agent's LLM inference is handled by the **LLM Gateway** — an external process
(not a mailbox actor) that manages connectivity to inference providers. The Agent
Runtime connects to the Gateway via gRPC streaming for `Complete` and `Embed` RPCs.

**Gateway responsibilities:**
- Connection pooling (reuse HTTP connections to providers)
- Auth rotation (rotate API keys without restarting agents)
- Health monitoring (probe providers, failover on errors)
- Unified token counting and cost attribution
- Rate limit management per provider

**Key constraint:** The Gateway never sees agent journals, messages, or internal
state. It only receives LLM request/response payloads. The agent's context, system
prompt, and tool results are assembled by the Agent Runtime locally — the Gateway
is a pure proxy with operational infrastructure layered around it.

**Model selection and turn management** remain entirely agent-local. Each agent
controls which model to use, temperature, and conversation flow — the Gateway does
not influence agent behavior.

#### Tool Binding

The runtime binds exactly the Connectors referenced by the agent's ConnectorRef capabilities. Each Connector exposes
its bound tool protocol functions as callable tools. The LLM cannot call tools the agent does not hold a ConnectorRef
to. All tool calls and results are journaled automatically.

#### Outbox Enforcement

When the LLM produces an outbound message, the runtime checks:

- If the message has `is_resolution: true`: the runtime checks the topic's continuations set. If non-empty, the
  resolution is **deferred** — the message is not forwarded, and the runtime journals "Deferred resolution for
  T1 — outstanding continuations: B, C." The LLM may continue processing or address the pending state. If empty,
  the resolution is forwarded through guardrails → Exchange → destination.
- If the message has `continue_ref`: the reference is added to the topic's continuations set, and the deadline
  timer begins.
- If a cancellation is in progress for the topic: the runtime notifies sub-agents by sending `cancel_topic` to
  each tracked continuation target, journaling each as `cancellation_fanout`.

#### Continuation Deadline Monitoring

On each inbox poll, the runtime checks all pending `continue_ref.deadline` values against the wall clock. When a
deadline is exceeded:

1. Runtime journals `continuation_deadline_expired` with the target agent identity.
2. Runtime removes the continuation from the set.
3. If the set is now empty and the LLM previously attempted resolution (deferred), the runtime surfaces this on
   the next context assembly: "Your pending continuation to B has expired — you may want to resolve or retry."

#### Rate Limiting

Every mailbox enforces two configurable limits to provide backpressure:

- **`max_depth`** (default: 100) — Maximum number of messages in the inbox queue.
  When exceeded, the Exchange rejects `SendMessage` with `ErrMailboxFull`.
- **`max_processing_rate`** (default: 10/s) — Maximum messages processed per
  second from the inbox.

When a sender receives `ErrMailboxFull`, its Agent Runtime journals
`mailbox_full` and may either retry or surface the backpressure to the LLM for
handling. The rate limit complements the existing inbox priority order
(cancellations > continuations > new work) — rate applies to dispatch, not to
priority ordering.

### Security & Authentication

The Exchange runs an **internal CA** that issues x509 certificates. All external connections authenticate via
certificate or platform delegation.

**Connection auth matrix:**

| Connection | Mechanism | Notes |
|------------|-----------|-------|
| CLI (networked) | x509 cert from CSR flow | Admin approves cert request |
| CLI (embedded) | None (in-process) | Trusted by process boundary |
| Web UI backend | x509 cert from CSR flow | Or Unix socket with peer cred mapping |
| Slack CommSystem | Platform auth (OAuth/socket mode), mapped to security principal | No x509 needed; platform resolves user identity |
| Email Receiver | x509 cert from CSR flow | Event Source identity |
| Webhook Receiver | x509 cert from CSR flow | Event Source identity |
| Other CommSystems | x509 cert from CSR flow | Administrator issues on registration |
| Other Event Sources | x509 cert from CSR flow | Administrator issues on registration |
| Unix socket (local) | `SO_PEERCRED` → UID → security principal mapping | Configurable mapping table: "UID 1001 → admin, UID 1002 → restricted" |
| Loopback / localhost | Same as Unix socket if Unix socket used; otherwise cert | Auto-approve CSR only when `ENV=dev` |

**CSR flow:**

1. CLI/CommSystem/Event Source generates keypair + CSR
2. Sends CSR to Exchange admin endpoint
3. Admin approves (CLI `marvin exchange approve`, or auto-approve if `ENV=dev` and localhost)
4. Exchange CA signs and returns certificate
5. Certificate presented on subsequent connections

**Platform auth delegation:**

When a user interacts via Slack, the Slack CommSystem resolves the Slack user ID to a security principal using a
configured mapping. The CommSystem includes this principal in the gRPC `SendMessage` envelope. The Exchange trusts the
CommSystem's mapping (trust is established by the CommSystem's own x509 cert or socket peer cred).

**Internal user mapping:**

Multiple CommSystem identities for the same person resolve to a single InternalUser:

```
Slack:U12345 ──┐
Web:alice@... ──┼── InternalUser("alice") ── RoleActor(Personal)
SSH:alice ──────┘                              RoleActor(Family)
```

The Exchange maintains a mapping table from CommSystem principal to InternalUser. On connect, the CommSystem's
platform identity is resolved through this table. The InternalUser then provides:
- The list of RoleActors the user may request grants from (personal, org memberships)
- The default RoleActor for unqualified requests
- The security context applied to the user's root actor

This mapping is configured at deployment time (HCL) and may be updated at runtime via admin commands. RoleActor
assignment is independent of which CommSystem the user is connected through — a user gets the same RoleActors
whether they connect via Slack, Web UI, or CLI.

### Observability & Tracing

- **Span naming**: `StructName.MethodName` convention
- **Trace propagation**: Context flows with every message through the Exchange
- **Key trace points**: Message reception, agent processing, LLM inference, tool execution, journal compression,
guardrail evaluation
- **Span attributes**: Agent ID, message type, capability references, provenance, security context, processing latency
- **Correlation**: Every message carries a trace ID for end-to-end tracking

### Deployment Topology

```
┌──────────────┐     ┌──────────────┐
│  Machine 1   │     │  Machine 2   │
│              │     │              │
│  Exchange ◄──┼─────┼──► Exchange  │
│    │         │     │    │         │
│  Agent A     │     │  Agent B    │
│  Agent C     │     │  Agent D    │
└──────────────┘     └──────────────┘
```

Federated Exchanges route messages across machines, enabling sandboxed execution on isolated hardware, horizontal
scaling of agent capacity, and platform-specific deployment (e.g., Docker on Linux hosts).

### Open Questions

- **Rate limiting**: How to handle mailbox overload?
  (Resolved: mailbox-level `max_depth` + `max_processing_rate`. See Agent Runtime section.)
- **Dead letters**: What happens to undeliverable messages or agents that have expired TTLs?
  (Resolved: `dead-letter.{scope}` system actor. See System Actors section.)
- **Agent discovery**: How does an agent discover another agent's address to send a message?
  (Resolved: `registry.{scope}` system actor. See System Actors section.)
- **CancelRef propagation**: Is CancelRef for child topics implicit on spawn, or must it be explicitly granted?
  (Designed: CancelRef is a capability like any other — see Constrained References.)
- **Stale continuation surfacing**: Should the runtime inject stale continuations as conversational prompts,
  passive notifications, or only on LLM query?
  (Existing design — journaled, surfaced on next idle inbox poll. No change needed.)
- **Continuation set on restart**: Should the persisted continuation set survive agent restart, or should the
  parent re-establish it on restart?
  (Resolved: parent-managed. Agent Runtime does not persist continuation sets. See Supervisor in System Actors.)
- **Capability delegation policy**: Can agent A freely forward agent B's mailbox address to C? Should the Exchange log
  or restrict this? (Designed: `ConstrainedRef` with `DelegationPolicy` — see Constrained References.)
- **Agent naming / aliases**: How are human-readable aliases registered and resolved to UUID mailbox addresses?
  (Resolved: `registry.{scope}` alias registration at spawn. See System Actors section.)
- **Agent lifecycle**: Start, stop, restart, and health check protocols?
  (Resolved: `supervisor.{scope}` manages lifecycle, notifies parent on death. See System Actors section.)
- **LLM placement**: When configured as an internal Exchange service, what does the LLM protocol look like versus
  agent-local?
  (Resolved: LLM Gateway as external process, gRPC streaming, agent-local model control. See LLM Gateway
  Integration section.)
- **Guardrail rejection feedback**: How does a guardrail communicate a rejection reason back to the originating agent?
  (Resolved: `GuardrailRejection` system message to origin agent's inbox. See Guardrails section.)
- **Mandatory vs. opt-in guardrails**: Should the Exchange enforce guardrail routing for certain provenances?
  (Resolved: Exchange-level provenance-based guardrail policies prepended to spawn-time chain. See Guardrails
  section.)
- **RoleActor discovery**: How does a root actor find its RoleActor mailbox?
  (Resolved: `registry.{scope}` role membership lookup. See System Actors section.)
- **Multi-role priority**: A user belongs to a personal role and one or more org roles. Which RoleActor handles a
  tool request?
  (Resolved: explicit routing by the agent via registry lookup for the relevant RoleActor.)
- **Connector TTL and re-request**: Should Connectors have a built-in TTL to force periodic re-authorization?
  (Designed: `expires_at` on ConnectorRef; `RenewRef` to RoleActor. See Constrained References.)
- **Tool protocol credential management**: Where does a Connector obtain credentials? (Designed: stored at RoleActor
  level, injected at spawn. See RoleActor section.)
- **Connector-level guardrail composition**: Can the requestor add guardrails on top of the Connector's chain?
  (Resolved: guardrail chain merge — Exchange policies prepended, spawn-time chains appended. See Guardrails
  section.)
- **Cross-org Connector sharing**: Can a Connector be forwarded across org boundaries? (Designed:
  `DelegationSameOrg` by default. See Constrained References.)

## Deployment Scenarios

### 1. Local CLI-only

**Purpose**: Development, MCP tool debugging, system verification, testing. A single developer running queries from the
terminal.

**Two modes:**

**Embedded mode** (default, `marvin query "hello"`):
- The Exchange runs **in-process as a library** with the full actor model, capabilities, guardrail routing, and
journals — all in-memory, no persistence.
- CLI operates as a synchronous stdin/stdout Communication System connected to the embedded Exchange.
- All state is ephemeral: actors, mailboxes, journals vanish on exit.
- Ideal for: iterating on agent behavior, testing MCP tool integration, testing guardrail configurations, system
verification in CI.

**Networked mode** (`marvin --exchange localhost:9090 query "hello"`):
- CLI connects to an external Exchange server via gRPC, presenting its x509 certificate.
- Operates as a standard Communication System with a subscription stream for output.
- Gains access to persistent state, remote agents, event sources, and multi-user coordination.
- Ideal for: interacting with a deployed system, remote debugging, REPL sessions (`marvin repl` with persistent gRPC
stream).

**Config** (`~/.marvin/config.hcl`):
```hcl
mode = "embedded"  # or "networked"
exchange = "localhost:9090"  # only in networked mode
cert_path = "~/.marvin/cert.pem"
key_path = "~/.marvin/key.pem"
```

**Architecture mapping**:
- Embedded: Exchange library imported, agent structure identical to server mode, capabilities and guardrails enforced
in-process
- Networked: CLI is a gRPC CommSystem, same as any other Communication System
- LLM provider: local Ollama or remote API
- MCP tools and other tool protocols: the tools under development or test (embedded) or remote protocol servers (networked)
- Security: none in embedded (process boundary); x509 in networked
- Event sources: webhook receivers and email receivers run as MCP tools or local scripts in embedded mode

**What fits**: Full architecture — same actor model, Exchange, capabilities, guardrails, journal model. Embedded mode
proves the library design works.

**What doesn't**: Nothing — both modes are first-class citizens in the architecture.

**Gaps**: None identified beyond the open questions above.

**New components**: In embedded mode, system actors (supervisor, registry,
dead-letter) run in-memory inside the same process. The LLM Gateway may run as a
local subprocess or connect to an external Gateway if configured.

### 2. Family Slack Coordination

**Users**: Parents and children of varying ages (5 to 20). Trust is graduated: parents have full access to all agents
and data; children have graduated access based on age and maturity.

**Trust model**:
- **Parents**: Full access to all agents, override and supervision capability via guardrails.
- **Older teens (16-20)**: Access to most agents, personal calendars, task lists. Limited visibility into sibling and
parent-private data.
- **Younger children (5-12)**: Restricted agent set, supervised actions on shared agents, no access to sensitive agents
(e.g., finance, scheduling).

**Architecture mapping**:
- Single Exchange node.
- Slack as a Communication System (gRPC bridge).
- Domain agents: family calendar, shopping list, notes, school coordinator, research helper. Each agent manages its own
domain and journal.
- Inbound email: email receiver event source → summarization agent (zero ConnectorRefs) → family members can ask about new
offerings via Slack.
- No shared memory — agents that need information from another domain message the relevant agent and receive a
response. The shopping list is an agent; other agents message it to add items or query state.
- Security contexts: graduated access enforced by target-side ACLs on each agent and capability grants at spawn time.
- Guardrails: children's agents have pass/reject guardrails on outbound messages to CommSystems (prevent sharing
private data) and on agent-to-agent messages (prevent accessing sensitive agents). Parents' agents have no guardrails
or mutating guards for oversight.

**What fits**: Exchange routing, mailbox isolation, per-agent capability scoping, guardrail interposition, journal
model, event sources with zero-tool pattern.

**What doesn't**:
- **Graduated capability grants**: Parent needs to spawn child-facing agents with restricted capabilities. The spawn
capability model supports this (parent grants a subset), but there's no policy language yet to say "agents spawned for
children cannot hold ConnectorRefs to finance tools."
- **RoleActor provisioning**: RoleActors for each role (parent, teen, child) must be created and assigned to
InternalUsers. The RoleActor pattern resolves graduated access: each role has its own policy logic for what Connectors
it spawns. RoleActor creation and assignment is a new operation.
- **Simple onboarding**: No provisioning flow to add a new family member, create their InternalUser, assign RoleActors,
and configure initial Connector grants and guardrail policies.

**Gaps**: RoleActor provisioning, onboarding workflow, Connector lifecycle management.

**New components**: Each family member's user scope gets its own triad
(supervisor, registry, dead-letter). The family org scope gets a separate triad
that governs org-wide agents. The naming scheme (`alice.family.org`,
`registry.family.org`) enables natural scope isolation between members.

### 3. Personal Assistant via Web UI

**Users**: A single person managing a set of agentic staff to accomplish goals.

**Architecture mapping**:
- Browser connects via WebSocket to the Web UI backend, which is a CommSystem speaking gRPC to the Exchange.
- A user-facing "concierge" agent spawns specialist agents (calendar, research, email drafting, project tracking) with
specific context, capabilities (ConnectorRefs, MailboxRefs), and TTL. Tool protocol grants come from the user's
RoleActor.
- Agent spawning hierarchy maps directly to the actor model and capabilities model. Tool access is separate: parent
delegates ConnectorRefs to children, and children may also request their own grants from the RoleActor.
- Guardrails on Connectors (email, finance) ensure all outbound responses from specialist agents are reviewed
before reaching the user or other agents.
- Webhook event sources for calendar sync, incoming document processing, and service integrations.

**What fits**: Actor model, capabilities-based spawning, guardrail interposition, journal model, gRPC CommSystem
pattern, event sources, tool scoping.

**What doesn't**:
- **Synchronous bridge**: Web requests expect a response in seconds, but mailbox processing is asynchronous. The
topic model resolves this: the Web UI CommSystem stamps a `correlation_id`, the Exchange echoes it on subscription
stream events, and the Web UI backend waits for `is_resolution` with matching `correlation_id`. Detail of mapping
HTTP request → gRPC message → stream response → HTTP response is Web UI backend implementation.
- **Browser ↔ Web UI protocol**: WebSocket for real-time updates, REST for one-shot. Not yet defined.
- **Session continuity**: Web sessions span multiple requests; need to correlate messages to the same user session
across agent interactions.

**Gaps**: Browser protocol definition, session correlation.

**New components**: The personal user scope provisions the triad for the single
user. The LLM Gateway runs as a shared infrastructure service.

### 4. Boutique Services / Dark Factory

**Users**: Customers of a small business running automated services (e.g., document processing, data analysis, custom
manufacturing quoting).

**Architecture mapping**:
- Multi-tenant by design — each customer is a tenant with isolated data.
- Exchange federation for sandboxed execution on isolated hardware.
- Agent spawning for per-customer service instances, each with dedicated capabilities and guardrails.
- Security contexts with tenant dimension isolate customer data and actions.
- Journal per agent per customer; no cross-customer data access.
- Event sources as canonical input: webhook receivers for customer orders, CI/CD triggers, document uploads. Each event
source is stamped with provenance and routed to the appropriate customer's order-processing agent.
- Guardrails between the order-receiving agent and billing/financial agents ensure only approved transactions proceed.

**What fits**: Nearly everything — Exchange federation, capabilities model, guardrail interposition, journal model,
event sources, gRPC CommSystems, target-side ACLs.

**What doesn't**:
- **Tenant dimension**: Security contexts lack a tenant identifier. Each agent's journal and capability grants need
tenant scoping.
- **Billing / metering**: No mechanism to track LLM calls, tool executions, storage, or processing time per tenant.
- **Customer onboarding**: No provisioning flow for new customers, their agent instances, and initial capability and
guardrail configuration.
- **SLA guarantees**: No reliable delivery, retry policies, dead letter recovery, or uptime commitments defined.
- **Audit logging**: The journal provides per-actor history, but no tamper-evident, tenant-scoped audit view across
multiple actors and guardrails.
- **Resource quotas**: No per-tenant limits on memory, compute, concurrent agents, or tool executions.

**Gaps**: Tenant dimension, billing/metering, customer onboarding, SLA/reliability patterns, audit logging, resource
quotas.

**New components**: The naming scheme's `.org` scoping provides a natural tenant
boundary — each customer is an org scope with its own triad. Per-scope system
actors give each tenant isolated lifecycle management, discovery, and dead letter
handling. The LLM Gateway provides centralized cost tracking and rate limiting per
tenant.

## Cross-cutting Gaps

| Gap                          | Description                                                                                                                                       | Affects | Status      |
|------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|---------|-------------|
| **Synchronous/async bridge** | Resolved by topic model: CommSystem stamps `correlation_id` on `SendMessage`, Exchange echoes on stream events. CLI/Web waits for `is_resolution`. | 1, 3    | Closed      |
| **Role / access model**      | RoleActors broker tool protocol grants on policy evaluation. Replaces group principals with actor-based capability brokering. Each role is a RoleActor with its own policy logic and guardrails. See RoleActor section. | 2, 4    | Design     |
| **Onboarding workflow**      | Provisioning flow for new users and their initial capability and guardrail configuration.                                                         | 2, 4    | Open        |
| **Tenant dimension**         | Tenant IDs in security contexts and journal isolation for multi-tenant deployments.                                                               | 4       | Open        |
| **Billing / metering**       | Usage tracking per tenant for LLM calls, tool executions, storage, processing time.                                                               | 4       | Open        |
| **SLA / reliable delivery**  | Retry policies, dead letter handling, delivery guarantees, health checks. (Dead letter handling resolved; retry policies and delivery guarantees remain.) | 4       | Partial     |
| **Audit logging**            | Tamper-evident, tenant-scoped audit records across multiple actor journals and guardrail decisions.                                               | 4       | Open        |
| **Resource quotas**          | Per-tenant limits on memory, compute, concurrent agents, tool executions.                                                                         | 4       | Open        |
| **Browser protocol**         | REST + WebSocket contract for browser ↔ Web UI backend communication.                                                                             | 3       | Open        |
