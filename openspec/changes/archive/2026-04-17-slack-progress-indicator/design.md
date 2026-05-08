## Context

Currently, when users send a query to Marvin in Slack and the LLM takes time to respond, there is no feedback to the user that processing is happening. The current implementation posts a message only when content arrives from the LLM or via the existing "thinking" display (which users can toggle off). This leaves users wondering if their message was received or if Marvin is stuck.

## Goals / Non-Goals

**Goals:**
- Show immediate visual feedback when a query starts processing
- Update progress indicator every 3 seconds or on each streaming event
- Display engaging, Marvin-specific quips that cycle through (SimCity/The Sims style)
- Enable emoji cycling (🕐 → 💭 → ⏳ → etc.)
- Track Slack API rate limit (429) occurrences via metrics

**Non-Goals:**
- Per-user configuration/toggle (keeping it simple, always on)
- CLI implementation (this is Slack-only for now)
- Animated GIF support (stick with text + emoji approach)

## Decisions

### D1: Timer Cadence
**Decision:** 3 seconds  
**Rationale:** Provides responsive feedback without overwhelming Slack with updates. 12 seconds felt too slow for real-time UX.
**Alternatives:** 5s, 12s were considered.

### D2: Visual Style
**Decision:** Emoji cycling + message text  
**Rationale:** SimCity-style messages provide personality while emoji advances give clear visual progress indication.
**Examples:**
```
🕐 *Consulting the oracles*
🕑 *Decoding latent space*  
🕒 *Reticulating splines*
```

### D3: When to Show
**Decision:** Show progress immediately when query processing begins  
**Rationale:** Immediate feedback prevents user uncertainty.

### D4: Message Source
**Decision:** Marvin-specific phrases  
**Rationale:** Aligns with Marvin's personality and differentiates from generic loading messages.
**Message themes:** LLM/processing humor (tokens, attention heads, hallucinations, etc.)

### D5: Rate Limit Metrics
**Decision:** Wrap Slack client to track 429 occurrences  
**Rationale:** The slack-go library already handles retries with backoff. We should track when this happens for observability.
**Implementation:** Add counter in slack_client.go wrapper.

### D6: Timer Threshold in Updater
**Decision:** Change from 1 second to 3 seconds in slack_updater.go line 264  
**Rationale:** Current threshold is too aggressive for our desired 3s cadence.

## Risks / Trade-offs

- **Risk:** Slack message edit limits → **Mitigation:** Track edits per conversation, unlikely to hit limit in normal use
- **Risk:** Multiple concurrent queries from one user → **Mitigation:** Each SlackUpdater is independent, OK
- **Risk:** 429 errors on post → **Mitigation:** slack-go handles retries automatically; we just track metrics

## Implementation Points

1. **slack_updater.go**: Modify timer threshold from 1s → 3s (line 264)
2. **slack_updater.go**: Add emoji cycle + message rotation in `addContentInternal()` 
3. **slack_client.go**: Add metrics for 429 occurrences
4. **New file**: Define progress messages list (Marvin-specific quips)

## Resolved Questions

All questions resolved:

1. **Quips**: 15 Marvin-specific quips defined in spec (Section 10.1)
2. **Reset behavior**: Per conversation (resets emoji/message position per session)
3. **Thread usage**: Not using threads - main channel only
4. **User replies while processing**: Queued for future ingestion
5. **Fallback on edit failure**: No fallback - leave message as-is

---

## Deferred Concerns

### C1: Long-Running Operations Beyond Slack Chat

The progress indicator design addresses immediate user feedback in Slack. However, similar UX concerns exist for long-running operations that may occur outside the chat context:

- **Rate limit waits**: When waiting on `X-RateLimit-Reset` (e.g., 60s+ wait), users have no visibility into progress
- **LLM streaming delays**: Extended silence during token generation
- **Non-interactive contexts**: CLI queries where there's no visual feedback at all

**Note:** These are out of scope for the current change but should be considered in future iterations of the progress indicator system.