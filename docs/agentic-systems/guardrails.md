# Guardrails

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
  guardrails attached to a RoleActor, which may hold limited references for policy lookup operations only (e.g.,
querying
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
  on_reject = "notify_parent"         // | "notify_sender" | "log" | "drop"
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
cancellation messages. GuardrailRejection messages use reserved system mailbox
slots and are processed at **priority level 2** in the inbox (after cancellations,
before continuation replies — see Inbox Processing Priority). The runtime journals
it into working memory so the LLM can handle the rejection.

This means guardrail rejections are surfaced to the sending agent's LLM as an
actionable notification, not silently discarded.
