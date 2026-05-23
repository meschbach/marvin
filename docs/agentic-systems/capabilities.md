# Capabilities Model

The system uses an **object-capability (ocap) model**. A capability is a reference that confers authority. Possession
of the reference IS authorization — there is no ambient authority or global ACL.

## Primitive capabilities

| Capability                                    | What it grants                                                                                  | Granted by                                                                 | Typical constraints                                     |
|-----------------------------------------------|-------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------|---------------------------------------------------------|
| `MailboxRef(id)`                              | Send messages to the target actor's mailbox                                                     | Parent on spawn; Exchange on registration                                  | `delegation_policy`, `expires_at`                       |
| `RoleActorRef(role_mailbox)`                  | Request tool protocol grants from a RoleActor                                                   | Exchange on bootstrap; org admin on assignment                             | `expires_at` for temporary admin assignments            |
| `ConnectorRef(connector_mailbox)`             | Send tool requests to a Connector actor. Connectors use per-sender fair queuing to prevent one agent from starving others. | RoleActor on grant approval | `expires_at`, `delegation_policy` (same-org by default) |
| `Spawn`                                       | Create child actors with a subset of own capabilities                                           | Parent on spawn; system bootstrap                                          | `max_children`, `max_ttl`, `allowed_protocols`          |
| `ToolProtocolRef(server_handle, tool_filter)` | Connect to and use a tool protocol server (e.g., MCP, optionally filtered to a subset of tools) | Admin registration; Connector holds these, agents never hold them directly | `tool_filter`                                           |
| `CancelRef(topic_id)`                         | Send a cancellation message for a specific topic                                                | Parent on spawn; Exchange on registration for system actors                | Scoped to single `topic_id`                             |

## Capability Flow

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

## Explicit Capabilities for System Actions

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
ExpiresAt      *time.Time // nil = no expiry; Exchange rejects expired descriptors at routing time
GrantedBy      ActorID    // identity of granter (for audit trail)

// Type-specific constraints
MaxChildren    int           // Spawn only
MaxTTL         time.Duration // Spawn only
AllowedProtocols []ProtocolPrefix // Spawn only: e.g., "mcp://vikunja/*", "local://*"
ToolFilter     []string           // ToolProtocolRef only: e.g., ["read_tasks", "write_tasks"]
TopicID        string             // CancelRef only: which topic this cancel capability targets
}
```

## Constrained References (Delegation Policy)

Every mailbox reference may optionally carry delegation constraints encoded as a `ConstrainedRef` wrapping the
base `MailboxRef`. The Exchange validates these constraints at routing time — possession is still authorization,
but possession within the bounds encoded in the reference:

```go
type DelegationPolicy int

const (
DelegationUnrestricted DelegationPolicy = iota // may be forwarded to any actor
DelegationSameOrg   // may only be forwarded within the same org tenant
DelegationSameUser  // may only be held by the grantee
DelegationNoForward // may not be forwarded; holder may use but not delegate
)

type ConstrainedRef struct {
Target         MailboxID // the underlying mailbox address
GrantedBy      ActorID   // identity of the granter (for audit)
ExpiresAt      *time.Time       // nil = no expiry
Delegation     DelegationPolicy // how this ref may be forwarded
GrantedAt      time.Time        // for audit trail
GrantedTo      ActorID // original grantee (for DelegationSameUser enforcement)
}
```

## How Constraints Are Enforced at Routing Time

When the Exchange routes a message, it unwraps the reference chain. For each hop in the chain, it checks:

1. **Expiry**: If any reference in the chain's `ExpiresAt` is in the past, the message is rejected with
   `ErrExpiredReference`. No registry lookup — purely time-based.

2. **Delegation policy**: If a message arrives via a forwarded reference, the Exchange walks the delegation
   chain. A `DelegationNoForward` reference that was forwarded (i.e., the sender is not the `GrantedTo` actor)
   is rejected. A `DelegationSameOrg` reference forwarded to an actor in a different org tenant is rejected.

3. **Audit logging**: Every delegation hop is logged by the Exchange with `{granted_by, granted_to, policy,
   forwarding_actor}` for the audit trail. This resolves the "should the Exchange log or restrict this?" open
   question by answering: both — log always, restrict per policy.

## Who Sets Constraints

| Granter                                 | Typical policy           | Rationale                                                                      |
|-----------------------------------------|--------------------------|--------------------------------------------------------------------------------|
| Parent spawning a child                 | `DelegationSameUser`     | Child should use the reference, not hand it to third parties                   |
| RoleActor granting a Connector          | `DelegationSameOrg`      | Connector may be forwarded within the user's org but not across org boundaries |
| Agent forwarding a reference to a child | Inherited or tightened   | Forwarding cannot loosen the original policy (monotonic constraining)          |
| Exchange bootstrap                      | `DelegationUnrestricted` | Root actors are trusted; policy is their responsibility                        |

**Monotonic constraining:** When actor A holds a `ConstrainedRef` with policy P, and A forwards it to B, the
forwarded reference's policy must be P' where P' >= P in restrictiveness (i.e., constraining is monotonic —
you can only tighten, never loosen). The Exchange enforces this at forwarding time. This prevents a
restricted reference from being "loosened" by an intermediary.
