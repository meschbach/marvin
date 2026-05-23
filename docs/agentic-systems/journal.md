# Journal & Persistence

Every actor operates a **two-level journal architecture**: a raw append-only journal owned by the runtime, and a
working memory that serves as the LLM's context window, assembled incrementally from journal events.

## Raw Journal (Runtime-Owned)

A full-fidelity, append-only record of every event the actor participates in. Never modified after append. The
raw journal is the source of truth — full actor state must be reproducible from it alone.

**Retention**: The raw journal grows without bound. This is by design — every event is preserved in full
fidelity as the authoritative audit trail. No compaction, archival, or deletion policy is applied. If storage
constraints later motivate tiering, a checkpoint-based compaction scheme can be added as a future enhancement
while preserving the "reproducible from journal alone" invariant.

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

## Working Memory (LLM Context)

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
│   llm_output, tool results,                  │
│   ltm_search results, etc.                   │
├─────────────────────────────────────────────┤
│ Current Inbox Message + Topic State          │
│   "New message from User on topic T1"       │
├─────────────────────────────────────────────┤
│ Available Runtime Tools                      │
│   runtime_send_message, runtime_resolve_topic│
│   runtime_request_continuation,              │
│   runtime_cancel_topic                       │
└─────────────────────────────────────────────┘
```

Fresh assembly and incremental append produce **functionally equivalent** contexts. Incremental append is a
runtime optimization — each new event is appended as text to the recent events block. The runtime monitors total
token count against the configured maximum.

## Compression

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

## Long-Term Memory Search

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

## Continuation Set (Runtime-Owned)

The set of pending `continue_ref` for each active topic is owned by the Agent Runtime, separate from both the raw
journal and working memory. Entries are created when the LLM calls `runtime_request_continuation`. It is an
in-memory data structure that:

- Gates resolution (does not fire if continuations non-empty)
- Drives cancellation fan-out
- Detects deadline expiry (checked on each inbox poll, with clock skew tolerance)
- Routes incoming continuation replies to the correct topic

The continuation set survives compression — compressing journal entries does not affect pending continuations.

## Persistence

All actors run ephemerally by default. When persistence is configured, the raw journal, searchable memory index,
and continuation set are persisted to durable storage. The storage mechanism is abstracted — the architecture
does not prescribe files, databases, or object stores.
