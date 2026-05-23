# Topic Model & Async Protocols

The system uses topic IDs to group messages into workflow units and a continuation protocol for asynchronous
agent-to-agent coordination.

## Communication Flow

Messages carry two independent identifiers:

- **Correlation ID** (transport level): Set by the original sender (CommSystem or parent agent). The Exchange
  passes it through and echoes it on all subscription stream events for that message tree. Agents never interpret it.
- **Topic ID** (application level): Assigned by the target agent's runtime when processing begins. All messages
  within a conversation unit share the same topic ID. The agent's LLM sees the topic ID in its working memory and
  uses it for workflow tracking.

Topic lifecycle and outbound messaging are handled entirely through **runtime protocol tools** — structured
tool calls the LLM makes, not flags embedded in prose:

- `runtime_request_continuation(destination, content, deadline)` — signals "I need a response to continue topic T1."
  The runtime stamps a `continue_ref` on the outbound message containing the sender's topic ID, the sender's mailbox
  address, and an absolute deadline. The receiver echoes this in its response.
- `runtime_cancel_topic(topic_id, reason)` — the LLM's request to cancel a topic. The runtime validates the
  agent holds the required `CancelRef` capability before acting.
- `runtime_resolve_topic(reason)` — signals topic completion. The runtime verifies no outstanding continuations
  before forwarding the resolution through guardrails.

Additionally, the transport layer carries:

- **continue_ref**: System-level header stamped by the runtime on outbound messages sent via
  `runtime_request_continuation`. Not visible to the LLM as a mutable field.
- **cancellation**: An inbound message with a `CancelRef` capability targeting a specific topic ID. The runtime
  processes this before any LLM invocation — always accepted, bypassing LLM discretion.

```
CommSystem ──SendMessage(correlation_id=X)──→ Exchange ──→ Agent Mailbox
                                                               │
                                                   Runtime assigns topic_id: T1
                                                   LLM processes, decides to delegate
                                                   LLM calls runtime_request_continuation(B, content, deadline)
                                                               │
                                                               ├── Runtime stamps continue_ref, sends to B
                                                               │   └──→ Agent B (continue_ref echoed in response)
                                                               │
                                                               ├── B responds, continue_ref matches T1,
                                                               │   LLM resumes
                                                               │
                                                               ├── LLM calls runtime_resolve_topic("completed")
                                                               │   Runtime checks continuations set (empty ✓)
                                                               │   Resolution forwarded → Guardrail(s) → Exchange
                                                               │       → CommSystem stream (correlation_id=X echoed)
                                                               │
                                                               └── LLM calls runtime_cancel_topic(T1) (if interrupted)
                                                                   Runtime validates CancelRef, journals,
                                                                   fan-outs cancel_topic(T1) to B
 ```

## Topic State Machine

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
                   ║  LLM calls          │ runtime fan-outs cancel
                   ║  runtime_resolve,   │
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

## Continuation Protocol

When an agent needs a response from another agent to continue a topic, the LLM calls
`runtime_request_continuation(destination, content, deadline)`. The runtime stamps a `continue_ref` on the outbound
message:

```
continue_ref {
    topic_id: string           // sender's local topic ID
    reply_to_mailbox: string   // sender's mailbox address
    deadline: timestamp        // absolute wall clock
}
```

The `deadline` is an absolute timestamp rather than a relative TTL. Both the sender and the receiver can
independently evaluate whether a continuation has expired without remembering when the clock started.

Deadline evaluation includes a **clock skew tolerance window** equal to 5% of the continuation's timeout
duration (e.g., a 60s timeout has a 3s tolerance; a 5min timeout has a 15s tolerance). All agents are expected
to run NTP or equivalent clock discipline. A `clock_discontinuity` warning is journaled if the skew exceeds the
tolerance. Deadline expiry is an approximate signal, not a definitive failure — the LLM may retry or escalate.

**Fan-out**: A topic may have multiple outstanding continuations concurrently. The runtime tracks them as a set
keyed by topic ID. Resolution is gated on all continuations being resolved or expired.

**Continuation deadline expiry**: On each inbox poll, the runtime checks all pending `continue_ref.deadline` values
against the wall clock (applying the skew tolerance window). Expired deadlines produce a
`continuation_deadline_expired` raw journal entry and may inject a notification into the agent's working memory.
The LLM decides how to respond.

**Stale continuations**: If a continuation reply arrives for a topic that has already been resolved or cancelled,
the runtime journals it as `stale_continuation` and surfaces it on the next idle inbox poll. The LLM may act on it
or ignore it.

## Cancellation Protocol

Cancellations arrive in an agent's inbox as system messages from two sources:
- **External cancellation**: Another actor sends `cancel_topic(topic_id=T1, reason, CancelRef)`. The Exchange
  validates the sender holds a `CancelRef` capability before routing.
- **Self-initiated cancellation**: The LLM calls `runtime_cancel_topic(topic_id, reason)`. The runtime validates
  the agent holds the required `CancelRef` for that topic. Self-cancellation follows the same processing path
  below.

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

## Resolution Protocol

When the LLM determines a topic is complete, it calls `runtime_resolve_topic(reason)`. The runtime intercepts
this tool call and checks the topic's continuations set:

- **If continuations set is empty**: The resolution is forwarded through guardrails → Exchange → destination. The
  topic is marked Resolved. On the next context assembly, the runtime injects a `GuardrailRejection` system message
  at priority level 2 (see Inbox Processing Priority).
- **If continuations set is non-empty**: The runtime defers the resolution, journals "Deferred resolution for T1 —
  outstanding continuations: B, C", and does not resolve the topic. On the next context assembly, the runtime
  includes a prompt note: "Your resolution of T1 was deferred because continuations B and C are still pending."
  The LLM may choose to wait for replies, cancel outstanding continuations, or revise its approach.

The runtime acts as a protocol invariants enforcer. If the LLM calls `runtime_resolve_topic` for topic T1 while
the working memory indicates T2 is the active topic, the runtime journals a warning and re-prompts ("You resolved
T1 but your current conversation is on T2. Confirm or correct.") rather than blindly executing.

For the synchronous bridge (CLI, Web), the CommSystem waits for a resolution event with matching `correlation_id`
on the subscription stream before returning control to the user.

## Agent-to-Agent Async

When agent A needs data from agent B to continue topic T1:

1. A's LLM calls `runtime_request_continuation(B, content, deadline)`.
2. A's runtime stamps a `continue_ref` on the outbound message, adds it to T1's continuations set, journals
   the outbound message, and transitions T1 to WaitingForContinuations.
3. A's runtime returns to processing its inbox (other topics, other messages).
4. B eventually responds; the response carries the echoed `continue_ref`.
5. A's runtime matches the incoming reply to T1, transitions T1 back to Active.
6. The LLM sees the pending journal entry (from step 1) plus the response on the next context assembly, and
   resumes T1.

No blocking, no special async runtime. The journal IS the continuation store — the LLM reads past context to
understand what it was waiting for and why.
