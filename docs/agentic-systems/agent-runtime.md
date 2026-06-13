# Agent Runtime

Every agent executes inside an **Agent Runtime** — a deterministic execution environment that enforces the topic model,
capability constraints, and actor invariants. One runtime per agent. The runtime is the agent's home address: the
Exchange delivers messages to it, the Supervisor manages its lifecycle, and the LLM interacts with it through
runtime protocol tools.

## Runtime Lifecycle

Each runtime passes through a lifecycle managed by the Supervisor:

```
unprovisioned → running → terminating → terminated
```

- **unprovisioned**: Created by the Supervisor on spawn. The runtime has an assigned mailbox address but no
  delivery endpoint. Messages addressed to it are queued in the Exchange.
- **running**: The runtime has registered its delivery endpoint with the Exchange and is processing messages.
  This is the steady state. The Supervisor monitors health via heartbeat.
- **terminating**: A termination signal has been received (TTL expiry, parent request, supervisor action).
  The runtime completes processing the current message, journals the termination reason, flushes any pending
  journal entries, and deregisters its delivery endpoint. New messages are rejected at the Exchange level.
- **terminated**: The runtime has stopped. The Exchange routes subsequent messages to dead-letter. The
  Supervisor cleans up registry entries and notifies the parent.

The Supervisor advances the runtime through these states. A runtime that stalls or crashes is detected via
missed heartbeat and moved to `terminated` by the Supervisor.

## Inbox Processing Priority

The runtime processes messages from the agent's mailbox in strict priority order. Cancellations and guardrail
rejections use reserved system mailbox slots, ensuring they can always be delivered even if the topic message
queue is at capacity:

1. **Cancellation messages** — Always processed first, bypassing LLM discretion. The runtime journals
   "cancellation received from X for topic T1," transitions T1 to Cancelled, and fan-outs `cancel_topic` to all
   tracked continuations.
2. **GuardrailRejection system messages** — Processed before any LLM invocation. The runtime appends the
   rejection details to working memory so the LLM sees the feedback before deciding its next action.
3. **Continuation replies** — Messages carrying a `continue_ref` are matched to their active topic. The runtime
   transitions the topic back to Active and journals the arrival. If the topic is already Resolved or Cancelled,
   the message is journaled as `stale_continuation`.
4. **New work** — Messages with no topic assignment get a fresh topic ID. Messages with an existing topic ID are
   routed to that topic's activity stream.

## LLM Context Assembly

Before each LLM invocation, the Agent Runtime assembles the context:

- Current topic's working memory (raw journal entries as text, or compressed summary)
- Topic state: Active / WaitingForContinuations / Resolved / Cancelled
- Pending continuations set (read-only: "You are waiting on responses from B and C for topic T1")
- Pending guardrail rejections (system messages at priority 2, appended before LLM invocation)
- Runtime protocol tools: `runtime_send_message`, `runtime_resolve_topic`, `runtime_request_continuation`,
  `runtime_cancel_topic` — always bound and visible in every invocation's tool set
- Capability set: only ConnectorRefs the agent holds are bound as available tool targets
- Provenance of the incoming message

The assembled context is sent to the LLM Gateway via gRPC streaming rather than a
direct provider connection. The Agent Runtime embeds a Gateway client that handles
the streaming RPC.

## LLM Gateway Integration

Each agent's LLM inference is handled by the **LLM Gateway** — an external process
(not a mailbox actor) that manages connectivity to inference providers. The Agent
Runtime connects to the Gateway via gRPC streaming for `Complete` and `Embed` RPCs.

The Gateway is a pure proxy with operational infrastructure. It never sees agent
journals, messages, or internal state — only LLM request/response payloads. Model
selection, fallback, and conversation flow remain agent-local.

**Gateway responsibilities:** connection pooling, auth rotation, retry with backoff,
per-endpoint health tracking, slow-start for recovering endpoints, rate limit
management, token counting, and cost attribution.

See [LLM Gateway](llm-gateway.md) for the full specification of retry behavior,
status codes, and health tracking.

## Chain Engine Integration

The Agent Runtime does not perform model selection directly. Instead, it delegates to
a **Chain Engine** — a per-runtime component that holds a reference to the agent's
configured chains and tracks per-provider-model cooldown state.

**Delegation flow:**

1. Runtime assembles context (system prompt + working memory + tools)
2. Runtime calls `ChainEngine.Select()` to get the next provider model
3. Runtime sends inference to the Gateway referencing that provider model
4. On success: runtime calls `ChainEngine.ReportSuccess(selection)`
5. On failure: runtime calls `ChainEngine.Advance(outcome)` which returns:
   - `Use{selection}` — try this provider model now
   - `Wait{duration, selection}` — pause and retry this selection later
   - `exhausted` — all options in chain are unusable

**The runtime does not filter by context size.** If the selected provider model's
context window is smaller than the assembled context, the runtime compresses working
memory to fit (see Context Compression below) and retries the same selection.

**Chain updates:** The runtime holds a shared pointer to the chain definition. On
administrative update, the runtime is notified via callback. In-flight inferences
complete against the old state; the next `Select` evaluates against the updated
chain.

See [Model Registry](model-registry.md#chain-engine) for the Chain Engine interface
specification.

## Pause and Resume

When the Chain Engine returns a `Wait{duration, selection}` directive, the runtime
must delay before retrying. Instead of blocking a goroutine, the runtime pauses
itself:

1. Runtime calls `self.Pause()` — suspends inbox polling
2. Registers a timer for the specified duration
3. Returns the goroutine to the pool
4. On timer expiry: calls `self.Resume()` — resumes inbox polling
5. On next inbox poll, retries the inference with the Wait's selection

Pause is also available for external use — the Supervisor may pause an agent for
backpressure (inbox too full, memory pressure) using the same mechanism.

**Interface:**
```
Pause()    — suspend inbox processing. Active inference completes; next poll blocks.
Resume()   — restart inbox processing after a pause.
IsPaused() — report current state.
```

While paused, the mailbox continues accepting messages (up to its depth limit) but
the runtime does not dispatch them. Cancellation messages still flow through system
slots.

## Context Compression on Fallback

When a provider model has a smaller context window than the assembled working memory,
the runtime compresses rather than advancing the chain. Compression uses the existing
Journal compression mechanism:

1. Calculate overage: `assembled_size - provider_model.context_window`
2. Compress oldest working memory entries (summary) until the context fits
3. If even after full compression the context does not fit: advance chain
4. Journal: `context_compressed { provider_model: "gemma-4@ollama",
   original_size: 45000, compressed_size: 27000, window: 28000 }`

This ensures the chain engine's `Select` remains independent of context size — the
runtime always makes the selected model work if possible.

## Model Switch Notification

When the chain advances to a different provider model mid-conversation, the runtime
may optionally inform the LLM. Controlled by the `notify_model_switch` toggle on the
agent config (default: false).

When enabled, the runtime injects a system-origin note into working memory before
the next context assembly:

```
[infrastructure: model switched from "gemma-4@openrouter" to "ministral@ollama"
 because "gemma-4@openrouter" was unavailable (rate limit). Context unchanged.]
```

The toggle exists because the effect of signaling model switches is not fully
understood — it may reduce confusion on capability differences or cause the LLM to
second-guess itself. Default off for now, togglable for experimentation.

## Tool Binding

The runtime binds exactly the Connectors referenced by the agent's ConnectorRef capabilities. Each Connector exposes
its bound tool protocol functions as callable tools. The LLM cannot call tools the agent does not hold a ConnectorRef
to. All tool calls and results are journaled automatically.

## Runtime Protocol Tools

Every agent has a fixed set of runtime-bound tools that control topic lifecycle, outbound messaging, and
cancellation. These are always present regardless of the agent's ConnectorRefs:

| Tool | Parameters | Purpose |
|---|---|---|
| `runtime_send_message` | `destination`, `content`, `continue_ref?` | Send a message to another actor. If `continue_ref` is set, the topic enters WaitingForContinuations. |
| `runtime_resolve_topic` | `reason` | Signal the current topic is complete. The runtime verifies no continuations are pending. |
| `runtime_request_continuation` | `destination`, `content`, `deadline` | Send a message and enter WaitingForContinuations in one call. The runtime stamps `continue_ref` on the outbound. |
| `runtime_cancel_topic` | `topic_id`, `reason` | Request cancellation of a topic. The runtime validates the agent holds a `CancelRef` before acting. |

All runtime tool invocations and their results are journaled automatically (kind: `tool_invocation`,
`tool_result`). The runtime validates invariants on each call — it can reject or defer a tool call if the
agent's topic state is inconsistent with the requested action.

## Outbox & Protocol Enforcement

When the LLM produces an outbound action (via a runtime protocol tool or a tool call on a Connector), the
runtime validates it against the topic state machine:

- **`runtime_resolve_topic` arriving**: The runtime checks the topic's continuations set. If non-empty, the
  resolution is **deferred** — the topic stays Active, the runtime journals "Deferred resolution for T1 —
  outstanding continuations: B, C." On the next context assembly, the runtime includes a prompt note explaining
  why. If empty, the resolution is forwarded through guardrails → Exchange → destination, and the topic is
  marked Resolved.
- **`runtime_request_continuation` arriving**: The `continue_ref` is added to the topic's continuations set, the
  deadline timer begins, and the topic transitions to WaitingForContinuations.
- **`runtime_cancel_topic` arriving**: The runtime validates the agent holds a `CancelRef` for the target topic.
  If valid, it follows the Cancellation Protocol (fan-out, journal). If invalid, the call is rejected.
- **Topic mismatch detection**: If `runtime_resolve_topic(T1)` arrives but the LLM's current working memory
  indicates topic T2 is active, the runtime journals a warning and does not execute. On the next context
  assembly, it re-prompts: "You resolved T1 but your current conversation is on T2. Confirm or correct."
- **Cancellation in progress**: If a cancellation is actively fanning out for the topic, the runtime does not
  accept new `runtime_resolve_topic` or `runtime_request_continuation` calls for that topic. The LLM sees
  the cancellation state in its next context.

## Continuation Deadline Monitoring

On each inbox poll, the runtime checks all pending `continue_ref.deadline` values against the wall clock. The
clock skew tolerance window (5% of timeout duration) is applied before marking a deadline as expired. When a
deadline is exceeded within tolerance:

1. Runtime journals `continuation_deadline_expired` with the target agent identity.
2. Runtime removes the continuation from the set.
3. If the set is now empty and the LLM previously attempted resolution (deferred), the runtime surfaces this on
   the next context assembly: "Your pending continuation to B has expired — you may want to resolve or retry."

## Rate Limiting

Every mailbox enforces limits at two levels to provide backpressure:

**Per-mailbox limits:**

- **`max_depth`** (default: 100) — Maximum number of topic messages and continuation replies in the inbox
  queue. When exceeded, the Exchange rejects `SendMessage` with `ErrMailboxFull`.
- **`system_max_depth`** (default: 10) — Separate capacity for system messages (cancellations,
  GuardrailRejection, supervisor control). These slots are never consumed by topic or continuation messages,
  ensuring high-priority messages can always be delivered.
- **`max_processing_rate`** (default: 10/s) — Maximum messages processed per second from the inbox.

**Connector-level limits:** Each Connector enforces its own `max_depth` and `max_processing_rate`,
independent of the agents using it. A deep queue on one Connector does not affect other Connectors.

When a sender receives `ErrMailboxFull`, its Agent Runtime journals `mailbox_full` and may either retry or
surface the backpressure to the LLM for handling. The rate limits complement the existing inbox priority order
(cancellations > guardrail rejections > continuations > new work) — rate applies to dispatch, not to priority
ordering.
