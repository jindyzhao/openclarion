# End-to-End Chain Verification

> This document traces every link in the three primary chains (M2 headless
> report, M4 sandbox analysis, M5 interactive diagnosis) and verifies that each
> node has a concrete technical implementation path. Verdicts are classified
> honestly — not everything is "proven".

> Last updated: 2026-05-19

## Verdict Scale

| Verdict | Meaning |
|---------|---------|
| **proven** | Standard API/library/protocol, no design ambiguity, directly implementable |
| **feasible-with-constraint** | Technically possible, but requires specific design decisions, configuration, or operational constraints documented below |
| **needs-design** | Implementation path exists but requires non-trivial design work not yet completed |
| **risky** | No proven path exists, or path depends on unproven/unstable technology |

Current assessment: **no risky nodes exist**. All chains are implementable.
"No risky" means "no technically impossible path"; engineering, operational,
and product risks (LLM cost, agent output quality, Docker daemon security
surface) still apply but are not feasibility blockers.

---

## Chain A: M2 Headless Report (No Agent Framework)

```text
Alertmanager/Prometheus
  -> [A1] Go HTTP handler (poll or webhook)
  -> [A2] Deterministic grouping (pure Go logic)
  -> [A3] EvidenceSnapshot builder (MetricsProvider + CMDBProvider)
  -> [A4] Temporal workflow dispatch
  -> [A5] Fan-out N parallel Activities (one per group)
  -> [A6] Each Activity: HTTP POST to /chat/completions
  -> [A7] JSON Schema validation of LLM output
  -> [A8] Reduce SubReports into FinalReport
  -> [A10] Ent persist to PostgreSQL (must be Activity)
  -> [A9] IMProvider Activity: Webhook POST (only after A10 succeeds)
```

### Per-Node Verification

| Node | Operation | Implementation | Verdict |
|------|-----------|----------------|---------|
| A1 | Receive alerts | Go `net/http` handler accepts Alertmanager webhook POST; or timer-driven poll via Prometheus `/api/v1/alerts` HTTP GET | proven |
| A2 | Deduplicate and group | Pure Go function, deterministic, no external dependency | proven |
| A3 | Build evidence | Go calls MetricsProvider (Prometheus HTTP API for PromQL), CMDBProvider (fake in M1) | proven |
| A4 | Start Temporal workflow | `temporalClient.ExecuteWorkflow(ctx, opts, ReportFanOutWorkflow, snapshotID)` | proven |
| A5 | Parallel SubReports | `workflow.Go()` spawns goroutines within Temporal, or parallel `ExecuteActivity()` calls | proven |
| A6 | Call LLM | HTTP POST to `/chat/completions`; structured output mode is provider-capability dependent | **feasible-with-constraint** |
| A7 | Validate output | `santhosh-tekuri/jsonschema` Go library; check `finish_reason`, detect refusal/truncation | proven |
| A8 | Reduce | Pure Go merge logic inside Temporal Activity | proven |
| A9 | Send notification | HTTP POST to Webhook URL; **must execute after A10 persistence succeeds** | proven |
| A10 | Persist | Ent ORM → PostgreSQL; **must run as Temporal Activity** (not in workflow code) | proven |

### Constraint: A6 Structured Output Provider Dependency

OpenAI supports `response_format: { type: "json_schema", json_schema: { strict: true } }`
but with restrictions:
- only a subset of JSON Schema is supported (no `$ref` cycles, limited `oneOf`)
- refusal, truncation, and unsupported-schema responses must be handled
- non-OpenAI providers (Anthropic, local models) may not support strict mode

**V1 constraint**: provider-capability detection at LLMProvider initialization.
Strict schema preferred; fallback to `json_object` mode + client-side validation
+ retry (up to 3 attempts). Both paths validated before persistence.

### Chain A Additional Constraints

- **Persistence is an Activity**: Ent writes to PostgreSQL must run inside
  `workflow.ExecuteActivity()`, never directly in workflow code.
- **Notification after persistence**: IMProvider Activity starts only after
  FinalReport persistence Activity succeeds (prevents phantom notifications).
- **Idempotency keys**: LLM Activity and Webhook Activity must carry
  idempotency keys (e.g. `snapshotID + groupIndex`) to prevent duplicate
  reports/notifications on Temporal retry.

**Chain A conclusion**: proven at every node except A6, which is
feasible-with-constraint (provider-dependent structured output capability).

---

## Chain B: M4 Sandbox Agent Report Enhancement

```text
Temporal workflow (from Chain A)
  -> [B1] Workflow decides: invoke sandbox analysis
  -> [B2] Activity: ContainerProvider.Run()
  -> [B3] docker create with volume mounts
  -> [B4] Security: non-root, resource limits, no-new-privileges
  -> [B5] Network: egress control
  -> [B6] Container interior: agent reads agent_config/
  -> [B7] Agent connects to allowed data sources (direct HTTP for V1)
  -> [B8] Agent writes /workspace/out/output.json
  -> [B9] Go reads + validates output
  -> [B10] Timeout/cleanup
```

### Per-Node Verification

| Node | Operation | Implementation | Verdict |
|------|-----------|----------------|---------|
| B1 | Conditional branch | Temporal workflow `if` condition based on group criteria | proven |
| B2 | Start container | `github.com/docker/docker/client` → `ContainerCreate()` + `ContainerStart()` | proven |
| B3 | Mount evidence + config | `HostConfig.Binds` with `:ro` for inputs; writable tmpfs for output | proven |
| B4 | Non-root + limits | `User = "nonroot"`, `Resources.Memory`, `NanoCPUs`, `SecurityOpt: ["no-new-privileges"]` | proven |
| B5 | Egress control | Docker network isolation + per-endpoint allowlist | **feasible-with-constraint** |
| B6 | Agent loads config | Agent runtime reads `/workspace/agent_config/agent.yaml` at startup | proven |
| B7 | Agent queries data | V1: direct HTTP to allowed endpoints (Prometheus, K8s); post-V1: MCP-over-Streamable-HTTP | proven (V1 scope) |
| B8 | Agent writes output | Writes to `/workspace/out/output.json` on writable tmpfs | proven |
| B9 | Go reads + validates | Read output file; JSON Schema validate against SubReport schema | proven |
| B10 | Timeout + cleanup | `context.WithTimeout` → `ContainerStop()` → `ContainerRemove(force=true)` | proven |

### Constraint: B5 Egress Control Design

Docker can isolate containers (`--network=none`, internal networks), but
**precise per-endpoint egress allowlist** requires additional infrastructure:

| Approach | How | Tradeoff |
|----------|-----|----------|
| Egress proxy (Squid/Envoy) | Container routes all HTTP through proxy; proxy allowlists URLs | most precise; adds a sidecar |
| Host iptables | Rules on host restrict container subnet to specific IPs/ports | IP-level only; fragile if targets are dynamic |
| K8s NetworkPolicy | If deployed on K8s, CiliumNetworkPolicy can do L7 filtering | K8s-only; not applicable to bare Docker |
| DNS-based + firewall | Resolve allowed domains, block everything else | domain-level; needs DNS sidecar |

**V1 decision**: start with Docker internal network (`--network=sandbox-internal`)
+ host iptables allowing only Prometheus and LLM API IPs. Note: if LLM API is
public SaaS (e.g. api.openai.com), IP-based allowlist is fragile because SaaS
IPs rotate. For SaaS LLM targets, egress proxy (Envoy/Squid with domain
allowlist) is recommended even in V1. Graduate to full egress proxy for all
endpoints as sandbox matures.

**Concrete M0/M1 task**: design and test the egress control configuration before
M4 implementation starts.

### Chain B Additional Constraints

- **Image digest pinning**: sandbox container image referenced by digest
  (`openclarion-agent@sha256:...`), not mutable tag, in production config.
- **Short-lived credentials**: if agent needs API tokens (LLM, Prometheus),
  Go issues short-lived tokens (TTL ≤ container timeout) via environment
  variable injection. No long-lived secrets inside container.
- **Docker daemon privilege**: ContainerProvider requires access to Docker
  socket. V1 runs on same host; post-V1 considers rootless Docker or
  dedicated sandbox host with mTLS-protected API.
- **Writable mount scope**: output directory is `/workspace/out/` (writable
  tmpfs); agent writes to `/workspace/out/output.json`. All other paths
  (`/workspace/evidence.json`, `/workspace/agent_config/`, etc.) are read-only
  bind mounts. Agent cannot write outside `/workspace/out/`.

**Chain B conclusion**: all nodes proven except B5 (egress control), which is
feasible-with-constraint. Requires concrete infrastructure design before M4.

---

## Chain C: M5 Short-Conversation Interactive Diagnosis

```text
Browser
  -> [C1] WebSocket connection to Go
  -> [C2] WS ticket-based authentication
  -> [C3] Create Temporal DiagnosisRoomWorkflow
  -> [C4] User sends message via WS
  -> [C5] Go sends Temporal Update (or Signal as fallback)
  -> [C6] Workflow receives Update/Signal
  -> [C7] Workflow starts per-turn Activity (ContainerProvider.Run)
  -> [C8] Mount evidence + conversation + message + agent_config
  -> [C9] Agent loads full conversation context (budget-enforced)
  -> [C10] Agent reasons + writes /workspace/out/output.json
  -> [C11] Go reads + validates response
  -> [C12] Persist ChatTurn via Ent
  -> [C13] Return response to WS handler (via Update result)
  -> [C14] Check turn/time limits
  -> [C15] On close: notify via IMProvider
```

### Per-Node Verification

| Node | Operation | Implementation | Verdict |
|------|-----------|----------------|---------|
| C1 | WS connection | Go `nhooyr.io/websocket` or `gorilla/websocket`; Browser WebSocket API in Next.js Client Component | proven |
| C2 | Auth handshake | WS ticket model (see constraint below) | **feasible-with-constraint** |
| C3 | Start workflow | `temporalClient.ExecuteWorkflow(ctx, opts, DiagnosisRoomWorkflow, sessionConfig)` | proven |
| C4 | User message in | WS frame → Go handler → deny-list filter → use case | proven |
| C5 | Send to workflow | Temporal Update (preferred) or Signal (fallback) | proven (Temporal 1.21+) |
| C6 | Workflow receives | Update handler or Signal channel | proven |
| C7 | Per-turn Activity | `workflow.ExecuteActivity(ctx, RunDiagnosisTurn, turnInput)` | proven |
| C8 | Mount files | Same as B3, plus `conversation.json` and `message.json` | proven |
| C9 | Agent context size | Evidence + conversation + tools output; must fit token budget | **feasible-with-constraint** |
| C10 | Agent writes output | Same as B8 | proven |
| C11 | Validate response | Same as B9 | proven |
| C12 | Persist turn | `client.ChatTurn.Create().SetSessionID(...).SetContent(...).Save(ctx)` | proven |
| C13 | Push response to WS | Temporal Update returns result synchronously to caller | **feasible-with-constraint** |
| C14 | Limit check | Workflow counter + `workflow.NewTimer(ctx, sessionLifetime)` | proven |
| C15 | Close notification | Activity calls IMProvider.SendNotification() | proven |

### Constraint: C2 WebSocket Authentication

Browser `new WebSocket(url)` cannot set custom HTTP headers. Standard
approaches:

**V1 design: ticket-based auth**

```text
1. Browser calls POST /api/ws-ticket (with OIDC Bearer token in header)
2. Server validates OIDC token, issues short-lived ticket (UUID, TTL=30s, single-use)
3. Browser opens WebSocket: new WebSocket("wss://host/ws/diagnosis?ticket=xxx")
4. Go WS handler validates ticket (exists, not expired, not used), consumes it
5. If invalid: reject upgrade with 401
```

Constraints:
- Ticket must be single-use (delete after consumption)
- Ticket TTL must be short (30s) to minimize replay window
- Do NOT put long-lived JWT in query string (appears in logs/referrer)

### Constraint: C9 Context Token Budget

20 turns of text ≈ 96KB, but total context includes:
- evidence.json (potentially 50-200KB depending on alert batch size)
- conversation history (grows with turns)
- tool call results (if agent uses tools within a turn)
- system prompt + agent instructions

**V1 constraint**: enforce a byte/token budget at the workflow level before
mounting files into the container. If total context exceeds budget:
- truncate oldest conversation turns (keep first + last N)
- or reject the turn with an explicit "context limit reached" response
- never silently pass oversized context to LLM (causes truncation or failure)

### Constraint: C13 WS Push via Temporal Update

**Problem**: WS handler needs to send user message to workflow AND synchronously
wait for the per-turn Activity result to push back to the browser.

**V1 primary: Temporal Workflow Update (1.21+)**

```go
// WS handler sends Update and waits for result
updateHandle, err := temporalClient.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
    WorkflowID:   workflowID,
    RunID:        runID,
    UpdateName:   "submit-turn",
    Args:         []any{userMessage},
    WaitForStage: client.WorkflowUpdateStageCompleted,
})
var response domain.ChatTurn
err = updateHandle.Get(ctx, &response)
// response is now available to push via WS
```

```go
// Inside DiagnosisRoomWorkflow: register Update handler
err := workflow.SetUpdateHandlerWithOptions(ctx, "submit-turn",
    func(ctx workflow.Context, msg string) (domain.ChatTurn, error) {
        // Run per-turn Activity
        var turn domain.ChatTurn
        err := workflow.ExecuteActivity(ctx, RunDiagnosisTurn, turnInput).Get(ctx, &turn)
        if err != nil {
            return domain.ChatTurn{}, err
        }
        // Persist
        _ = workflow.ExecuteActivity(ctx, PersistChatTurn, turn).Get(ctx, nil)
        return turn, nil
    },
    workflow.UpdateHandlerOptions{
        Validator: func(ctx workflow.Context, msg string) error {
            // Reject if turn already in flight or turn limit reached
            if s.turnInFlight {
                return fmt.Errorf("turn in progress")
            }
            if s.turnCount >= s.maxTurns {
                return fmt.Errorf("turn limit reached")
            }
            return nil
        },
    },
)
```

Temporal Update semantics:
- Caller (WS handler) blocks until Update handler completes
- If Activity fails, error propagates to caller
- If WS disconnects mid-wait, workflow continues executing (durable)
- User can reconnect and Query for missed turns

**Constraints for Update-based approach**:
- **Idempotency**: each turn carries a `turn_id` / `message_id`; Update handler
  rejects duplicate IDs
- **Update timeout**: WS handler sets a context timeout (e.g. 3 minutes) on
  the UpdateWorkflow call; if exceeded, inform user "still processing"
- **WS disconnect during wait**: workflow completes the turn regardless; on
  reconnect, WS handler calls Query to retrieve missed turns
- **Concurrent turn rejection**: Update handler checks if a turn Activity is
  already in flight; if so, rejects with "turn in progress" error

**Conservative fallback: Signal + Query polling**

If Temporal Update is not available (older SDK, stability concerns):
1. Signal sends user message to workflow
2. WS handler polls Temporal Query (or reads DB) every 500ms until new turn
   appears
3. Higher latency (~500ms-1s) but no dependency on Update API

**In-memory channel bridge: optional acceleration only**

An in-memory `sync.Map[sessionID]chan` can reduce push latency to sub-100ms,
but it is NOT a correctness mechanism:
- Temporal Activities may retry on different workers
- Process restart loses all channels
- It is an optimization layer on top of Update or Query, not a substitute

**Chain C conclusion**: all nodes are proven or feasible-with-constraint. No
needs-design nodes remain once Temporal Update is selected as the primary path.

---

## Cross-Cutting Concerns

### Container Image Build

All sandbox chains (B, C) depend on a pre-built container image containing the
agent runtime. This image must:
- include the chosen agent framework (OpenClaw / LangChain / custom)
- include Python/Go runtime for tool scripts
- read `/workspace/agent_config/` at startup
- write to `/workspace/out/output.json` on completion
- run as non-root user
- exit cleanly on SIGTERM (for timeout cleanup)
- be referenced by digest in production configuration

This is a standard Dockerfile + CI build pipeline concern.

### Temporal Worker Deployment

For V1 (single-binary monolith):
- Go binary hosts HTTP server, WS server, and Temporal worker in one process
- Temporal dev-server (temporalite) runs via Docker Compose

This colocation is the simplest deployment model. Post-V1 can separate workers
if scaling demands it. The Update-based C13 design works regardless of whether
the WS handler and Temporal worker are colocated or separate.

### Volume Mount Security

- Evidence and agent_config are mounted `:ro` (read-only bind mounts)
- Output directory: `--tmpfs /workspace/out:size=10m` (writable, capped)
- Agent writes only to `/workspace/out/output.json`
- Container cannot access host filesystem beyond mounted paths
- Container runs with `--security-opt=no-new-privileges`

---

## Verdict Summary

| Node | Verdict | Key Constraint |
|------|---------|----------------|
| A6 | feasible-with-constraint | provider-capability dependent; strict preferred, JSON mode + validation fallback |
| B5 | feasible-with-constraint | requires egress proxy or iptables design; Docker isolation alone is insufficient |
| C2 | feasible-with-constraint | WS ticket model (short-lived, single-use); no JWT in query string |
| C9 | feasible-with-constraint | byte/token budget enforced at workflow level; truncate or reject on exceed |
| C13 | feasible-with-constraint | Temporal Update primary; Signal+Poll fallback; in-memory channel optional |
| All others | proven | standard APIs, no ambiguity |

---

## Risk Summary

| Risk | Category | Chain | V1 Mitigation |
|------|----------|-------|---------------|
| LLM structured output compatibility | engineering | A6 | two-tier strategy (strict + fallback); provider capability detection |
| Egress allowlist enforcement | engineering | B5 | Docker internal net + host iptables; egress proxy design before M4 |
| Per-turn container startup latency | UX | C7-C10 | ~1-3s acceptable for V1; post-V1: persistent container with HTTP endpoint |
| Conversation context exceeds budget | engineering | C9 | enforce budget before container mount; truncate oldest turns |
| WS auth token exposure | security | C2 | ticket model (30s TTL, single-use, not in logs) |
| Docker daemon privilege surface | security | B2 | V1: host Docker socket; post-V1: rootless Docker or dedicated host |
| Agent output quality | product | B8, C10 | golden tests validate structure; quality delta measurement in M4 |
| LLM cost per turn | product | A6, C7 | token budget caps; model selection per use case |

---

## Conclusion

All three chains are technically feasible:
- **Chain A** (M2): proven at all nodes except A6 (provider-dependent structured
  output). Standard HTTP + Temporal fan-out + Ent persistence.
- **Chain B** (M4): proven except B5 (egress control needs concrete design).
  Docker Engine API + file-based contract.
- **Chain C** (M5): proven except C2/C9/C13 (each feasible-with-constraint).
  Temporal Update provides synchronous request-response semantics that close
  the WS push gap cleanly.

No link requires inventing a new protocol or depending on unproven technology.
Constraints are documented and resolvable within existing tool ecosystems.

---

## Implementation Priority Timeline

| Phase | Must-Resolve Before | Items |
|-------|--------------------|---------|
| **M0/M1** | First code lands | Verify Temporal Update SDK/server version compatibility; pin `oapi-codegen-exp` commit; `docker-compose.yml` (PostgreSQL 18 + temporalite); validate Update round-trip in integration test |
| **M2** | LLM Activity ships | LLMProvider capability detection (strict vs json_object); JSON Schema subset validation; idempotency key design for LLM + Webhook Activities; confirm `finish_reason` / refusal handling |
| **M4** | Sandbox ships | Egress allowlist scheme tested (iptables or egress proxy); image digest pin in CI; short-lived credential injection; Docker daemon privilege boundary; `/workspace/out/` tmpfs mount validated |
| **M5** | Diagnosis room ships | WS ticket storage/consumption; Temporal Update timeout + reconnect + Query fallback; concurrent-turn rejection (Validator); context byte/token budget enforcement + truncation strategy |

Items not completed by their deadline are blockers for that milestone's
acceptance. Items may be started earlier but must be proven (not just designed)
before milestone delivery.
