# Naming & Addressing

Actor mailbox addresses use a structured URI scheme with scope-encoded namespacing:

```
mailbox://{name}.{scope}[:{parent-scope}...]/{path?}
```

**Address format:**

- **Scheme**: `mailbox://` — indicates a mailbox address (possession IS capability to send)
- **Name**: The entity's identifier (e.g., `registry`, `supervisor`, `research-agent`, `alice`)
- **Scope**: Encodes what kind of namespace the entity belongs to, using TLD-style suffixes

**Scope types:**

| Type     | Purpose                                   | Examples                                          |
|----------|-------------------------------------------|---------------------------------------------------|
| `.local` | Resolved relative to sender's scope chain | `registry.local`, `supervisor.local`              |
| `.user`  | Individual identity                       | `alice.user`, `bob.user`                          |
| `.org`   | Organizational unit                       | `accounting.org`, `engineering.org`, `family.org` |

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
