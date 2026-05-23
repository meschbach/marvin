# System Actors

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

- Spawns agent actors within its scope on behalf of authorized parents. Creates the Agent Runtime instance and
  advances it through its lifecycle: `unprovisioned → running → terminating → terminated`.
- Monitors runtime health via heartbeat; detects stalled or crashed runtimes and advances their state.
- Applies supervision strategy on death (notifies parent).
- Enforces TTL expiry: signals runtime to terminate gracefully, then dead-letter routing.
- Failure is scoped — a supervisor failure does not escalate to parent scopes.

**Registry** (`mailbox://registry.{scope}`) — Directory and discovery:

- Agents register on spawn with UUID, aliases, type, capabilities, role memberships
- Lookup by UUID, alias, capability prefix, or role membership
- Presence watching via TTL-based re-registration (missed check-in = stale entry)
- Supports RoleActor discovery: `LookupRoleActors(InternalUser="alice")` returns
  `[MailboxRef(personal-role), MailboxRef(family-role)]`

**Dead Letter Agent** (`mailbox://dead-letter.{scope}`) — Unroutable message handling:

- Receives messages when the target agent's TTL has expired or the mailbox does not exist
- Configurable handling per scope: `log` (discard with log), `notify_sender`,
  `notify_parent`, `forward_to_handler`
- Messages carry provenance and original sender for policy-based routing

All three are provisioned lazily — created on first use rather than at scope
creation. This is an optimization; the behavior is equivalent to eager creation.
