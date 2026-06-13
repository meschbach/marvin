# Open Questions

- **Rate limiting**: How to handle mailbox overload?
  (Resolved: mailbox-level `max_depth` + `max_processing_rate`. See Agent Runtime section.)
- **Dead letters**: What happens to undeliverable messages or agents that have expired TTLs?
  (Resolved: `dead-letter.{scope}` system actor. See System Actors section.)
- **Agent discovery**: How does an agent discover another agent's address to send a message?
  (Resolved: `registry.{scope}` system actor. See System Actors section.)
- **CancelRef propagation**: Is CancelRef for child topics implicit on spawn, or must it be explicitly granted?
  (Designed: CancelRef is a capability like any other — see Constrained References.)
- **Stale continuation surfacing**: Should the runtime inject stale continuations as conversational prompts,
  passive notifications, or only on LLM query?
  (Existing design — journaled, surfaced on next idle inbox poll. No change needed.)
- **Continuation set on restart**: Should the persisted continuation set survive agent restart, or should the
  parent re-establish it on restart?
  (Resolved: continuation sets are runtime-owned, in-memory, and do not survive restart — cold or warm. All
  pending continuations are treated as expired on restart. Deadlines are the self-healing mechanism: the LLM
  retries or escalates on the next context assembly. See [Persistence & Restart](persistence-restart.md).)
- **Capability delegation policy**: Can agent A freely forward agent B's mailbox address to C? Should the Exchange log
  or restrict this? (Designed: `ConstrainedRef` with `DelegationPolicy` — see Constrained References.)
- **Agent naming / aliases**: How are human-readable aliases registered and resolved to UUID mailbox addresses?
  (Resolved: `registry.{scope}` alias registration at spawn. See System Actors section.)
- **Agent lifecycle**: Start, stop, restart, and health check protocols?
  (Resolved: `supervisor.{scope}` manages lifecycle, notifies parent on death. See System Actors section.)
- **LLM placement**: When configured as an internal Exchange service, what does the LLM protocol look like versus
  agent-local?
  (Resolved: LLM Gateway as external process, gRPC streaming, agent-local model control. See LLM Gateway
  Integration section.)
- **Guardrail rejection feedback**: How does a guardrail communicate a rejection reason back to the originating agent?
  (Resolved: `GuardrailRejection` system message to origin agent's inbox. See Guardrails section.)
- **Mandatory vs. opt-in guardrails**: Should the Exchange enforce guardrail routing for certain provenances?
  (Resolved: Exchange-level provenance-based guardrail policies prepended to spawn-time chain. See Guardrails
  section.)
- **RoleActor discovery**: How does a root actor find its RoleActor mailbox?
  (Resolved: `registry.{scope}` role membership lookup. See System Actors section.)
- **Multi-role priority**: A user belongs to a personal role and one or more org roles. Which RoleActor handles a
  tool request?
  (Resolved: explicit routing by the agent via registry lookup for the relevant RoleActor.)
- **Connector TTL and re-request**: Should Connectors have a built-in TTL to force periodic re-authorization?
  (Designed: `expires_at` on ConnectorRef; `RenewRef` to RoleActor. See Constrained References.)
- **Tool protocol credential management**: Where does a Connector obtain credentials? (Designed: stored at RoleActor
  level, injected at spawn. See RoleActor section.)
- **Connector-level guardrail composition**: Can the requestor add guardrails on top of the Connector's chain?
  (Resolved: guardrail chain merge — Exchange policies prepended, spawn-time chains appended. See Guardrails
  section.)
- **Cross-org Connector sharing**: Can a Connector be forwarded across org boundaries? (Designed:
  `DelegationSameOrg` by default. See Constrained References.)
- **Inference retry**: Should the LLM Gateway or Agent Runtime handle retries for transient inference errors?
  (Resolved: Gateway owns retry with backoff, returning status codes per call. See [LLM Gateway](llm-gateway.md).)
- **Model fallback**: When should an agent switch models on inference failure?
  (Resolved: Chain Engine in Agent Runtime handles fallback with configurable strategies. Gateway provides status
  codes; Engine decides to wait, advance, or exhaust. See [Model Registry](model-registry.md).)
- **Context window on fallback**: Should the runtime reject a model with a smaller context window?
  (Resolved: Runtime compresses working memory to fit. Only advances chain if compression cannot fit the context.
  See [Agent Runtime](agentic-runtime.md#context-compression-on-fallback).)
- **Model switch transparency**: Should the agent know when it falls back to a different model?
  (Designed: opt-in `notify_model_switch` toggle on agent config, default false. See [Agent
  Runtime](agentic-runtime.md#model-switch-notification).)
- **Pause/resume for wait-and-retry**: How should the runtime handle waiting for a rate-limit cooldown?
  (Resolved: Runtime pauses itself, schedules resume timer, returns goroutine to pool. See [Agent
  Runtime](agentic-runtime.md#pause-and-resume).)
