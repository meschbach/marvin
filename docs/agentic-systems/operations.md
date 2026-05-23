# Observability & Deployment

## Observability & Tracing

- **Span naming**: `StructName.MethodName` convention
- **Trace propagation**: Context flows with every message through the Exchange
- **Key trace points**: Message reception, agent processing, LLM inference, tool execution, journal compression,
  guardrail evaluation
- **Span attributes**: Agent ID, message type, capability references, provenance, security context, processing latency
- **Correlation**: Every message carries a trace ID for end-to-end tracking

## Deployment Topology

```
┌──────────────┐     ┌──────────────┐
│  Machine 1   │     │  Machine 2   │
│              │     │              │
│  Exchange ◄──┼─────┼──► Exchange  │
│    │         │     │    │         │
│  Agent A     │     │  Agent B    │
│  Agent C     │     │  Agent D    │
└──────────────┘     └──────────────┘
```

Federated Exchanges route messages across machines, enabling sandboxed execution on isolated hardware, horizontal
scaling of agent capacity, and platform-specific deployment (e.g., Docker on Linux hosts).
