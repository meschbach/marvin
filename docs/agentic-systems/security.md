# Security & Authentication

The Exchange runs an **internal CA** that issues x509 certificates. All external connections authenticate via
certificate or platform delegation.

**Connection auth matrix:**

| Connection           | Mechanism                                                       | Notes                                                                 |
|----------------------|-----------------------------------------------------------------|-----------------------------------------------------------------------|
| CLI (networked)      | x509 cert from CSR flow                                         | Admin approves cert request                                           |
| CLI (embedded)       | None (in-process)                                               | Trusted by process boundary                                           |
| Web UI backend       | x509 cert from CSR flow                                         | Or Unix socket with peer cred mapping                                 |
| Slack CommSystem     | Platform auth (OAuth/socket mode), mapped to security principal | No x509 needed; platform resolves user identity                       |
| Email Receiver       | x509 cert from CSR flow                                         | Event Source identity                                                 |
| Webhook Receiver     | x509 cert from CSR flow                                         | Event Source identity                                                 |
| Other CommSystems    | x509 cert from CSR flow                                         | Administrator issues on registration                                  |
| Other Event Sources  | x509 cert from CSR flow                                         | Administrator issues on registration                                  |
| Unix socket (local)  | `SO_PEERCRED` → UID → security principal mapping                | Configurable mapping table: "UID 1001 → admin, UID 1002 → restricted" |
| Loopback / localhost | Same as Unix socket if Unix socket used; otherwise cert         | Auto-approve CSR only when `ENV=dev`                                  |

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
