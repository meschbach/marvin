# Deployment Scenarios

## 1. Local CLI-only

**Purpose**: Development, MCP tool debugging, system verification, testing. A single developer running queries from the
terminal.

**Two modes:**

**Embedded mode** (default, `marvin query "hello"`):

- The Exchange runs **in-process as a library** with the full actor model, capabilities, guardrail routing, and
  journals — all in-memory, no persistence.
- Message delivery uses the same asynchronous queue and priority ordering as server mode. There is no synchronous
  shortcut — messages are enqueued, acknowledged, and dispatched through the same path, exercising identical
  delivery semantics.
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
key_path  = "~/.marvin/key.pem"
```

**Architecture mapping**:

- Embedded: Exchange library imported, agent structure identical to server mode, capabilities and guardrails enforced
  in-process
- Networked: CLI is a gRPC CommSystem, same as any other Communication System
- LLM provider: local Ollama or remote API
- MCP tools and other tool protocols: the tools under development or test (embedded) or remote protocol servers (
  networked)
- Security: none in embedded (process boundary); x509 in networked
- Event sources: webhook receivers and email receivers run as MCP tools or local scripts in embedded mode

**What fits**: Full architecture — same actor model, Exchange, capabilities, guardrails, journal model. Embedded mode
proves the library design works.

**What doesn't**: Nothing — both modes are first-class citizens in the architecture.

**Gaps**: None identified beyond the open questions above.

**New components**: In embedded mode, system actors (supervisor, registry,
dead-letter) run in-memory inside the same process. The LLM Gateway may run as a
local subprocess or connect to an external Gateway if configured.

## 2. Family Slack Coordination

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
- Inbound email: email receiver event source → summarization agent (zero ConnectorRefs) → family members can ask about
  new offerings via Slack.
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

## 3. Personal Assistant via Web UI

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
   stream events, and the Web UI backend waits for a resolution event with matching `correlation_id`. Detail of mapping
  HTTP request → gRPC message → stream response → HTTP response is Web UI backend implementation.
- **Browser ↔ Web UI protocol**: WebSocket for real-time updates, REST for one-shot. Not yet defined.
- **Session continuity**: Web sessions span multiple requests; need to correlate messages to the same user session
  across agent interactions.

**Gaps**: Browser protocol definition, session correlation.

**New components**: The personal user scope provisions the triad for the single
user. The LLM Gateway runs as a shared infrastructure service.

## 4. Boutique Services / Dark Factory

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
