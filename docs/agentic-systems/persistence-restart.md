# Persistence & Restart

This document defines what state survives process boundaries and how the system
recovers from restarts — cold (full system shutdown) and warm (single actor
crash).

## Persistence Boundaries

The system splits persistence responsibility between the Exchange and Agent
Runtimes. This boundary is by design: the Exchange is the durable authority;
runtimes are ephemeral workers that reconstruct state from their journal.

| Component | Persists | Does not persist |
|---|---|---|
| Exchange | Routing table, capability descriptors (MailboxRefs, ConnectorRefs, Spawn tokens, CancelRefs), mailbox queues, guardrail policy config, CA state, agent registry entries | Delivery endpoint registrations (runtimes re-register on start) |
| Agent Runtime | Raw journal, searchable memory index | Continuation sets, in-memory working memory, delivery endpoint binding |
| Connector | Tool protocol refs, credentials, context | In-flight tool call state |

The storage mechanism is abstracted — the architecture does not prescribe files,
databases, or object stores for any of these. Single-user mode uses in-memory
by default but may opt into persistence; server mode requires it for all
Exchange-level state.

## Cold Restart

A cold restart means every process in the system has stopped and restarted.

1. **Exchange restores**: routing table, capability descriptors, mailbox
   contents, guardrail policies, and CA state are loaded from persistent
   storage. System actors (Supervisor, Registry, Dead Letter per scope) are
   provisioned lazily on first use, same as initial boot.

2. **Agents replay**: on start, each Agent Runtime loads its raw journal from
   persistent storage. All pending continuations from the previous session
   have deadlines in the past — the runtime journals
   `continuation_deadline_expired` for each, and the continuation sets start
   empty.

3. **LLM discovers**: on the first context assembly, the LLM sees the expired
   deadlines in working memory and may retry, escalate, or resolve the topic
   as it sees fit.

4. **Mailboxes preserved**: messages that arrived during downtime are queued at
   the Exchange. On agent re-registration (delivery endpoint established), the
   Exchange pushes queued messages to the new runtime.

5. **Delegations survive**: ConnectorRefs, MailboxRefs, and all other capability
   descriptors are Exchange-persisted. Runtimes re-establish their delivery
   endpoints on registration. Connector endpoints are independent of any
   single agent runtime's liveness.

## Warm Restart

A warm restart means a single actor has crashed but the Exchange and other
actors continue running.

1. **Mailbox persists at the Exchange** — messages destined for the crashed
   actor continue to queue in its mailbox. This includes continuation replies
   from other agents and new work.

2. **Supervisor detects** the crash (missed heartbeat or delivery endpoint
   failure), journals the event, and notifies the parent per the supervision
   strategy (existing design).

3. **Parent may respawn** a replacement agent via the Supervisor. The new Agent
   Runtime is created in `unprovisioned` state (existing lifecycle).

4. **New runtime replays** its persisted raw journal. Deadlines have expired.
   Continuation sets start empty.

5. **Re-registration**: once the new runtime registers its delivery endpoint
   with the Exchange, the Exchange pushes all queued messages. Continuation
   replies that arrived during downtime are matched to topics via
   `continue_ref`:

   - If the topic has been implicitly resolved (all deadlines expired), the
     reply is journaled as `stale_continuation`.
   - If the topic is still Active (the LLM hasn't resolved it yet), the reply
     resumes the topic normally.

6. **Connector liveness**: if the crashed actor held ConnectorRefs, the
   Connectors remain live (they are independent actors). The replacement
   runtime re-establishes the same ConnectorRefs from its capabilty set
   (persisted by the Exchange), and tool access resumes.

## Continuation Set Rule

> Continuation sets are **runtime-owned, in-memory, and do not survive
> restart**. On restart (cold or warm), all pending continuations are
> treated as expired.

This is the central contract. It keeps the persistence boundary clean: the
Exchange owns durable routing and capabilities; the runtime owns transient
conversation state. Deadlines are the self-healing mechanism — the LLM
retries if appropriate, escalates if not, and the system makes progress
without needing to serialize an active continuation graph.

## Implications

- **No serialization of conversation state** beyond the raw journal. Recovery
  is journal replay + deadline expiry, not state reconstruction.
- **Retry is the default recovery pattern**. If a continuation reply is
  critical, the LLM re-sends the request after seeing the expired deadline in
  its next context assembly.
- **Crash-tolerant by design**. Since continuations are always treated as
  expired on restart, there is no window where in-flight continuations are
  silently lost — the LLM always learns about them on the next context
  assembly.
