## Why

Users currently have no visibility into Marvin's processing state during long-running LLM queries. When the model takes time to generate a response, users may wonder if the bot is stuck or dead. Adding a progress indicator similar to The Sims/SimCity loading messages provides engaging feedback while maintaining the modern Slack look and feel.

## What Changes

- Add a visual progress indicator that shows when Marvin is actively processing a query
- Cycle through emoji indicators and Marvin-specific quips every 3 seconds (or on each streaming event)
- Track Slack API rate limit (429) occurrences via metrics for observability
- Post progress message immediately when query processing begins

## Capabilities

### New Capabilities

- `slack-progress-indicator`: Real-time visual feedback during Slack query processing with animated indicators and Marvin-themed messages

### Modified Capabilities

- None - this is a new feature for Slack integration only

## Impact

- **Code affected**: `internal/slacker/slack_updater.go`, `internal/slacker/slack_client.go`
- **Metrics**: New counter for Slack API rate limit retries
- **User experience**: Immediate visual feedback on query start, updates every 3s or on streaming events