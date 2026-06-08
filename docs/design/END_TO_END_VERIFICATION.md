# End-to-End Chain Verification

> This document traces every link in the three primary chains (M2 headless
> report, M4 sandbox analysis, M5 interactive diagnosis) and verifies that each
> node has a concrete technical implementation path. Verdicts are classified
> honestly — not everything is "proven".

> Last updated: 2026-06-06

## Verdict Scale

| Verdict | Meaning |
|---------|---------|
| **proven** | Standard API/library/protocol, no design ambiguity, directly implementable |
| **proven-local** | Implemented and exercised locally, but still missing retained live or production-environment evidence |
| **partial** | Some implementation evidence exists, but at least one required real-environment or end-to-end proof is still pending |
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
| A1 | Receive alerts | Prometheus provider + ingestion library exist; local E2E drives `POST /api/v1/report-triggers/replay-window` against a Prometheus-compatible API and loads one firing alert; policy-driven replay can resolve an enabled alert source profile before replay; `openclarion report-replay` provides the legacy one-shot replay path and `openclarion report-policy-replay` provides the persisted-policy replay path for operator-triggered live runs; profile-bound Alertmanager webhook intake persists firing webhook alerts into `AlertEvent` rows without starting report workflows; scheduled trigger metadata, generated API, frontend settings, launcher workflow, ScheduleOptions registration mapping, persisted-schedule reconciliation, and scheduled-trigger proof harness are implemented locally. Live external verification remains pending in the required order: M2 headless Prometheus proof, persisted policy replay proof, then scheduled-trigger proof | partial |
| A2 | Deduplicate and group | Pure Go function, deterministic, no external dependency | proven |
| A3 | Build evidence | `evidencebuild.BuildSnapshot` constructs deterministic EvidenceSnapshot payloads from persisted alert groups/events | proven |
| A4 | Start Temporal workflow | `ReportWorkflowStarter` and Temporal `ReportStarter` can start `ReportBatchWorkflow` from replay snapshot refs with stable workflow IDs and duplicate-start policies; local E2E verifies HTTP trigger dispatch into a real Temporal dev server and worker, policy-driven replay resolves enabled workflow policy bindings before start, and CLI tests verify legacy and persisted-policy one-shot request/JSON/wait-result mapping. `ReportPolicyScheduleLauncherWorkflow` now computes scheduled replay windows, delegates to a worker-injected policy replayer Activity, and `ReportWorkflowScheduleRegistrar` maps persisted schedules to Temporal `ScheduleOptions` with skip overlap and catch-up windows, creates missing Temporal Schedules, updates existing schedule specs/actions/policies, and synchronizes paused state during startup and after successful schedule mutations. `make report-schedule-live-smoke` waits for a real Temporal Schedule action, launcher workflow, downstream `ReportBatchWorkflow`, and validator-checked delivery JSON when run against real services; live external scheduled E2E verification remains pending until that retained proof is captured | partial |
| A5 | Parallel SubReports | `ReportBatchWorkflow` starts one `ReportFanOutWorkflow` child per EvidenceSnapshot and fans in persisted SubReport IDs in input order | proven-local |
| A6 | Call LLM | OpenAI-compatible provider exists; report Activities use injected `LLMProvider`; `cmd/openclarion` wires it from env with startup capability detection | proven-local |
| A7 | Validate output | `santhosh-tekuri/jsonschema` Go library; check `finish_reason`, detect refusal/truncation | proven |
| A8 | Reduce | `GenerateFinalReport` Activity reduces persisted SubReports through `reportprompt` + `llmretry` + `reportdraft` before persistence | proven |
| A9 | Send notification | `FinalReportWorkflow` schedules `SendReportNotification` only after `GenerateFinalReport` returns a persisted ID; the Activity can select either the legacy env-injected IMProvider or a profile-backed Webhook provider for a bound report notification channel, persists a pending delivery row before calling the selected provider, marks it delivered/failed afterwards, skips duplicate provider calls after delivered, and the Webhook provider posts JSON with idempotency headers | proven-local |
| A10 | Persist | Ent ORM → PostgreSQL; **must run as Temporal Activity** (not in workflow code). Covers SubReport, FinalReport, fan-in links, and ReportNotificationDelivery rows | proven |

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
- **Report artifact is not closure**: `FinalReport` is the final artifact of
  the automated report workflow, not a final accountable incident conclusion.
  Human-confirmed closure is recorded through the diagnosis-room close path or
  a future explicit confirmation artifact, as defined in
  [report-lifecycle.md](report-lifecycle.md).

**Chain A conclusion**: local LLM validation, persistence, report
Temporal workflow units, batch-level fan-out/fan-in, runtime provider
injection, notification sequencing, report read APIs, the HTTP
replay-to-workflow trigger, the CLI one-shot replay trigger, and a manual
`make report-live-smoke` proof harness, the policy-driven
`make report-policy-live-smoke` proof harness, and the scheduled-trigger proof
harness are proven locally. The acceptance order follows
[ADR-0014](../adr/ADR-0014-alert-operations-configuration.md): first retain
the M2 headless `make report-live-smoke` artifact against the legacy
environment-configured Prometheus adapter, then retain profile-driven policy
replay proof, then retain scheduled-trigger proof. Replay proof is validated by
`scripts/report_live_smoke_output`, including canonical UTC timestamps for
`checked_at` and the replay window, replay request metadata with optional
`request.policy_id`, replay stats, and snapshot/SubReport consistency. The
replay validator requires the replay window to be valid and no later than
`checked_at`, the scenario to be one of the supported alert-analysis
scenarios, the live proof to have explicit wait intent, and the workflow
result notification status to be `accepted` or `delivered` with a matching
`notification_idempotency_key` shaped as `final_report:<id>/notification`; a
failed or pending notification cannot support live E2E acceptance. Scheduled
proof is validated separately by `scripts/report_schedule_live_smoke_output`,
which requires `request.schedule_id`, `request.policy_id`, enabled persisted
schedule metadata, a real Temporal Schedule action at or after the operator's
observation time, launcher/report-batch workflow binding, FinalReport output,
and accepted or delivered notification status. The validators also keep
retained workflow and run IDs whitespace-free and bounded, validate bounded
single-line provider message ID formatting when the upstream supplies one, and
bound the retained notification idempotency key; Webhook 2xx responses without
a stable upstream message ID remain acceptable because the IM provider contract
permits that path. Live
Prometheus/Alertmanager->Temporal->notification execution evidence remains
pending. The current readiness preflights expect real database and Temporal
addresses, a real alert-source profile or Prometheus endpoint, canonical UTC
replay or observation windows, explicit retained output paths when retaining
M2, policy, or scheduled proof, and either worker-side LLM/notification
configuration or an operator assertion that an externally managed worker is
ready. The policy and scheduled proof scripts invoke readiness in require-ready
mode before writing retained JSON. The operator configuration sequence and
required external inputs are documented in
[alert-operations-live-proof-runbook.md](alert-operations-live-proof-runbook.md).
Report live proof demonstrates automated report delivery; it does not by
itself prove human-confirmed incident closure.

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
| B3 | Mount evidence + config | readonly bind mounts for inputs; private writable output bind mount capped by `fsize` and Go read limit | proven |
| B4 | Non-root + limits | `User = "nonroot"`, `Resources.Memory`, `NanoCPUs`, `SecurityOpt: ["no-new-privileges"]` | proven |
| B5 | Egress control | Docker network isolation + per-endpoint allowlist | **feasible-with-constraint** |
| B6 | Agent loads config | Agent runtime reads `/workspace/agent_config/agent.yaml` at startup | proven |
| B7 | Agent queries data | V1-proven direct HTTP to allowed endpoints (Prometheus, K8s); post-V1: MCP-over-Streamable-HTTP | proven |
| B8 | Agent writes output | Writes to `/workspace/out/output.json` on the only writable output mount | proven |
| B9 | Go reads + validates | Read output file; enforce output cap with `fsize`/Go read limit and JSON Schema validate against SubReport schema; `make container-provider-output-cap-smoke` proves cap failure cleanup | proven |
| B10 | Timeout + cleanup | `context.WithTimeout` -> `ContainerStop()` -> `ContainerRemove(force=true)`, plus `make container-provider-timeout-smoke` leak check | proven |

### Constraint: B5 Egress Control Design

Docker can isolate containers (`--network=none`, internal networks), but
**precise per-endpoint egress allowlist** requires additional infrastructure:

| Approach | How | Tradeoff |
|----------|-----|----------|
| Egress proxy (Squid/Envoy) | Container routes all HTTP through proxy; proxy allowlists URLs | most precise; adds a sidecar |
| Host iptables | Rules on host restrict container subnet to specific IPs/ports | IP-level only; fragile if targets are dynamic |
| K8s NetworkPolicy | If deployed on K8s, CiliumNetworkPolicy can do L7 filtering | K8s-only; not applicable to bare Docker |
| DNS-based + firewall | Resolve allowed domains, block everything else | domain-level; needs DNS sidecar |

**V1 direction**: default remains network-none. Allowlist mode uses a dedicated
Docker network plus an external egress proxy or firewall boundary. The
provider-neutral contract now accepts only exact `host[:port]` targets, and the
Docker `StaticAllowlistEnforcer` can reject requests that are not a subset of
an externally provisioned allowlist. Note: if LLM API is public SaaS
(e.g. api.openai.com), IP-based allowlists are fragile because SaaS IPs rotate.
For SaaS LLM targets, egress proxy (Envoy/Squid with domain allowlist) is
recommended even in V1.

`make egress-allowdeny-smoke` proves this topology locally with Docker: a
sandbox client on an internal network reaches `allowed.internal:8080` through a
dual-network proxy, receives 403 for `denied.internal:8080`, and cannot bypass
the proxy to reach the upstream network directly.

**Remaining M4 task**: wire the chosen proxy/firewall boundary into the
candidate runtime path before accepting allowlist networking as production
enforced.

### Chain B Additional Constraints

- **Image digest pinning**: sandbox container image referenced by digest
  (`openclarion-agent@sha256:...`), not mutable tag, in production config.
- **Short-lived credentials**: if an agent needs API tokens (LLM,
  Prometheus), the control plane passes one-invocation credentials with an
  expiry timestamp. The provider-neutral request validates credential names,
  values, and required expiry; the Docker provider rejects expired credentials
  and credentials whose expiry exceeds the effective container timeout
  immediately before `ContainerCreate()`, then injects accepted credentials via
  environment variables. Errors name the credential but never include the
  credential value. Credential issuance/rotation remains runtime wiring.
- **Docker daemon privilege**: ContainerProvider requires access to Docker
  socket. V1 permits a local host socket only as a control-plane boundary;
  agent containers must never receive the socket as a mount. Remote Docker
  access must not use plaintext TCP; if remote mode is required, it must use
  TLS verification with client authentication. Post-V1 considers rootless
  Docker, a dedicated sandbox host with mTLS-protected API, or a Kubernetes
  Job provider behind the same file contract. See
  [docker-daemon-boundary.md](docker-daemon-boundary.md).
- **Writable mount scope**: output directory is `/workspace/out/` (private
  per-invocation bind mount capped by `fsize` ulimit and Go read limit);
  agent writes to `/workspace/out/output.json`. All other paths
  (`/workspace/evidence.json`, `/workspace/agent_config/`, etc.) are read-only
  bind mounts. Agent cannot write outside `/workspace/out/`.

**Chain B conclusion**: all nodes proven except production wiring for B5
(egress control), which remains feasible-with-constraint. The Docker Engine
provider now fails closed for allowlist-mode requests unless an injected egress
enforcer validates the provider-specific network boundary before container
creation, rejects allowlist targets that are not exact `host[:port]` values,
rejects long-lived or expired runtime credentials before create, and has a
manual live Docker smoke through `make container-provider-smoke`, timeout
cleanup proof through `make container-provider-timeout-smoke`, and output cap
proof through `make container-provider-output-cap-smoke`. A concrete Docker
internal-network + proxy allow/deny smoke passes locally, but candidate runtime
egress wiring and credential issuer wiring remain separate M4 work.
`scripts/sandbox_m4_decision` now provides the auditable proceed/iterate/defer
decision path once real baseline-audit, manifest-mode quality, runtime-smoke,
and human-review evidence files exist. Runtime-smoke sources are bound to their
canonical `make` targets, and each retained smoke claim must include a bounded
normalized relative `evidence_ref` plus a lowercase SHA-256 digest for the
retained smoke artifact/log. Absolute paths, traversal, URI-style refs,
backslashes, and spaces are rejected so retained packets stay portable. Review
evidence must name the same `sample_basis` as the quality comparison.
Pass-status candidate evaluations must also cite all required runtime smoke
names through `runtime_smoke_refs`, so review evidence remains reproducible,
tied to the compared sample, and not hard-bound to a particular agent runtime
family.
`scripts/sandbox_m4_evidence_packet` assembles those files and generated
outputs into a single empty artifact directory for review handoff, verifies and
copies the referenced runtime-smoke artifacts into `runtime-smoke-artifacts/`,
and validates minimum artifact shape and sample-basis consistency before
writing baseline, quality, review, runtime-smoke artifact, and decision
outputs.

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
  -> [C15] On close: persist conclusion snapshot + notify via IMProvider
```

### Per-Node Verification

| Node | Operation | Implementation | Verdict |
|------|-----------|----------------|---------|
| C1 | WS connection | Go `nhooyr.io/websocket` or `gorilla/websocket`; Browser WebSocket API in Next.js Client Component | proven |
| C2 | Auth handshake | `POST /api/v1/diagnosis/ws-ticket` + ticket-required `GET /ws/diagnosis` upgrade; runtime wiring now injects OIDC auth, PostgreSQL ticket storage, exact-origin CORS, and WebSocket origin checks from env | proven-local |
| C3 | Start workflow | `POST /api/v1/diagnosis/rooms` authenticates the bearer principal, loads a frozen EvidenceSnapshot, starts `DiagnosisRoomWorkflow` through `DiagnosisRoomStarter`, and waits for the workflow-created `DiagnosisTask` / `ChatSession` before returning the session handle | proven-local |
| C4 | User message in | WS `submit_turn` frame -> Go relay -> provider-neutral workflow client; deny-list enforcement remains authoritative in the workflow Update Validator | proven-local |
| C5 | Send to workflow | `DiagnosisRoomClient` calls Temporal `UpdateWorkflow` with `WorkflowUpdateStageCompleted` for `submit-turn` | proven-local |
| C6 | Workflow receives | `submit-turn` Update, `state` Query, `close`/`cancel` Signals | proven-local |
| C7 | Per-turn Activity | `workflow.ExecuteActivity(ctx, RunDiagnosisTurn, turnInput)` calls `ContainerProvider.Run` | proven-local |
| C8 | Mount files | Same as B3, plus `conversation.json` and `message.json`; M5 request construction is covered by `RunDiagnosisTurn` tests | proven-local |
| C9 | Agent context size | Evidence + conversation + tools output; must fit token budget | **feasible-with-constraint** |
| C10 | Agent writes output | Same as B8 | proven |
| C11 | Validate response | V1 diagnosis-turn `output.json` JSON Schema parser plus raw Container result validation | proven-local |
| C12 | Persist turn | `PersistDiagnosisTurn` writes the user+assistant ChatTurn pair and advances ChatSession turn count idempotently through workflow coverage | proven-local |
| C13 | Push response to WS | WebSocket relay returns the synchronous Temporal Update result as a `turn_result` frame and supports reconnect `query_state` frames | proven-local |
| C14 | Limit check | workflow Update Validator + durable idle/session timers | proven-local |
| C15 | Close conclusion + notification | Close path persists ChatSession terminal metadata, records a bounded `final_conclusion` snapshot in the idempotent `diagnosis_room.closed` event, sends the diagnosis-task-scoped IMProvider notification, and records an idempotent `diagnosis_room.close_notification_sent` audit event | proven-local |

### Constraint: C2 WebSocket Authentication

Browser `new WebSocket(url)` cannot set custom HTTP headers. Standard
approaches:

**V1 design: ticket-based auth**

```text
1. Browser calls POST /api/v1/diagnosis/ws-ticket (with OIDC Bearer token in header)
2. Server validates OIDC token, issues short-lived ticket (UUID, TTL=30s, single-use)
3. Browser opens WebSocket: new WebSocket("wss://host/ws/diagnosis?session_id=<id>&ticket=xxx")
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
agent runtime adapter. This image must:
- include the selected adapter for the candidate accepted by
  [agent-runtime-selection.md](agent-runtime-selection.md); candidate identity
  is operator-supplied evidence, not a control-plane enum
- include Python/Go runtime for tool scripts
- read `/workspace/agent_config/` at startup
- write to `/workspace/out/output.json` on completion
- run as non-root user
- exit cleanly on SIGTERM (for timeout cleanup)
- be referenced by digest in production configuration

The first executable check for a candidate image is
`make agent-runtime-smoke`, which validates the ADR-0013 file contract and
Docker security posture before the image can be considered for the M4/M5
baseline. `make custom-thin-runner-smoke` now provides the first concrete
candidate proof by building a scratch custom runner, resolving it through an
ephemeral localhost registry as `repo@sha256`, and running it through both the
runtime harness and the Docker Provider harness. The same smoke packages the
metric/topology helper binaries into the candidate image and proves the topology
helper via an alternate entrypoint under non-root, readonly, network-none
settings. This proves contract and lifecycle behavior only; quality comparison
remains a separate M4 decision gate. The image build itself remains a standard
Dockerfile + CI build pipeline concern.

`make agent-tool-scripts-test` proves the first read-only tool helper contracts
that a runtime image can later package: bounded Prometheus instant queries and
bounded static topology lookups, both returning JSON objects for downstream
agent/report validation.

`make sandbox-baseline-audit` emits a code-level JSON proof for the M4/M5
sandbox baseline without requiring a Docker daemon. It builds the same
provider-neutral requests and Docker runtime specs used by the production
boundary, then verifies fixed ADR-0013 file paths, network-none batch defaults,
M5 read-only turn inputs, non-root/readonly/no-new-privileges/capability-drop
security posture, resource limits, allowlist subset enforcement, and strict
request/raw-output JSON validation. Manual Docker smokes still provide the live
daemon evidence for create/start/wait/copy/remove behavior.

The M5 minimum sandbox baseline is accepted locally as of 2026-05-28:
`make sandbox-baseline-audit`, `make custom-thin-runner-smoke`,
`make container-provider-timeout-smoke`, `make container-provider-output-cap-smoke`,
and `make egress-allowdeny-smoke` pass against the current tree. That evidence
is deliberately narrower than full M4 report enhancement acceptance: it proves
the sandbox foundation for M5, not representative report quality.

`make sandbox-quality-compare-test` proves the offline comparison helper for
M4 direct-vs-sandbox SubReport outputs. The helper reuses the production
SubReport schema parser before calculating conservative deltas, so invalid
candidate report JSON cannot be compared. It also supports manifest-mode
batches so a future representative sample run can produce per-case and
aggregate evidence without changing the tool. Manifest-mode evidence includes
`sample_basis`, per-case alert scenario labels, `scenario_coverage`, and
canonical `snapshot:<positive-id>` EvidenceSnapshot refs so the M4 decision
gate can require `single_alert`, `cascade`, and `alert_storm` coverage while
proving each case remains bound to frozen evidence before proceeding. This
remains a harness only; the actual M4 quality decision still needs
representative direct and sandbox report outputs from candidate runtime runs.

`make sandbox-m4-subreport-generate` provides the manual persisted-sample
bridge for the sandbox side of that comparison. It loads a real
`EvidenceSnapshot` from PostgreSQL, mounts a snapshot-bound evidence envelope
through the existing Docker `ContainerProvider`, validates the candidate
`output.json` as a production SubReport, requires the canonical `snapshot:<id>`
ref in `evidence_refs`, and persists an idempotent sandbox SubReport row for
later sample export. This closes the candidate-output persistence gap, not the
representative quality review or M4 proceed/iterate/defer decision.

### Temporal Worker Deployment

For V1 (single-binary monolith):
- Go binary hosts HTTP server, WS server, and Temporal worker in one process
- Temporal dev server (`temporalio/auto-setup`) runs via Docker Compose

This colocation is the simplest deployment model. Post-V1 can separate workers
if scaling demands it. The Update-based C13 design works regardless of whether
the WS handler and Temporal worker are colocated or separate.

### Volume Mount Security

- Evidence and agent_config are mounted `:ro` (read-only bind mounts)
- Output directory: private writable bind mount at `/workspace/out` plus
  `fsize` ulimit and Go read cap
- Agent writes only to `/workspace/out/output.json`
- Container cannot access host filesystem beyond mounted paths
- Container runs with `--security-opt=no-new-privileges`

---

## Verdict Summary

| Node | Verdict | Key Constraint |
|------|---------|----------------|
| A6 | feasible-with-constraint | provider-capability dependent; strict preferred, JSON mode + validation fallback |
| B5 | feasible-with-constraint | requires egress proxy or iptables design; Docker isolation alone is insufficient |
| C2 | proven-local | WS ticket model (short-lived, single-use); no JWT in query string; authenticated submit/query relay proven locally |
| C9 | feasible-with-constraint | byte/token budget enforced at workflow level; truncate or reject on exceed |
| C13 | proven-local | Temporal Update primary path wired to WebSocket relay; Query handles reconnect state |
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
| Docker daemon privilege surface | security | B2 | V1: documented host-socket boundary; no socket mount into agents; no plaintext remote daemon; post-V1 rootless Docker or dedicated host |
| Agent output quality | product | B8, C10 | golden tests validate structure; quality delta measurement in M4 |
| LLM cost per turn | product | A6, C7 | token budget caps; model selection per use case |

---

## Conclusion

All three chains are technically feasible:
- **Chain A** (M2): local report contracts, OpenAI-compatible provider,
  report persistence, Temporal report workflow units, notification delivery
  logging, notification sequencing, and the HTTP replay-to-workflow trigger
  are proven locally, including a protocol-level E2E test that reaches a real
  Temporal worker and Webhook provider. `make report-live-smoke` is the first
  retained real-service proof target; `make report-policy-live-smoke` extends
  that proof to persisted policy replay, and `make report-schedule-live-smoke`
  captures the scheduled-trigger proof after a real Temporal Schedule action.
  Live external Prometheus/Alertmanager->Temporal->notification execution
  evidence remains pending until those validator-checked artifacts are retained.
  The required operator configuration order is documented in
  [alert-operations-live-proof-runbook.md](alert-operations-live-proof-runbook.md).
- **Chain B** (M4): proven except production B5 egress wiring. The Docker
  Engine API, file-based contract, Provider live/timeout/output-cap smokes, and
  local proxy allow/deny topology are proven.
- **Chain C** (M5): locally proven through the control-plane path. C1-C8 and
  C11-C15, lifecycle audit persistence, final close notification, room
  creation, and the mocked browser diagnosis-room route are proven locally at
  their current boundary. `make diagnosis-live-browser-smoke` now captures the
  required live browser proof shape against a real backend/worker stack and can
  create a room from `OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID` before connecting.
  The harness records canonical UTC `checked_at`, the exercised request mode,
  session id, evidence snapshot id, submitted-message length, and
  submitted-message SHA-256 digest without retaining message plaintext.
  Retained session, workflow, and run IDs must be single-line, whitespace-free,
  and bounded before they can support M5 acceptance. The retained `evidence`
  summary must also be a bounded single-line statement that mentions the
  `turn_result` round trip rather than copied logs. It also
  records structured browser observations for state load, `turn_result`,
  submitted-message visibility,
  browser-submitted message length and digest, connected status after the turn,
  assistant-turn count increment, user+assistant transcript pair increment,
  completed-turn log consistency, and transcript count consistency with the pair
  model.
  It validates the retained proof with `scripts/diagnosis_live_smoke_output`
  before reporting success, so M5 live acceptance cannot rely on a malformed,
  log-polluted, request-mismatched, or free-text-only JSON artifact.
  `cmd/openclarion` wires the OIDC auth, room starter, ticket store, WebSocket
  relay, diagnosis Temporal client, browser origin policy, and Docker-backed
  per-turn sandbox from env. `make diagnosis-dev-oidc-issuer` supplies a
  loopback-only local OIDC discovery/JWKS/token helper for manual live-smoke
  setup while preserving the same runtime verifier path. It is not acceptance
  evidence by itself. C9 remains a documented byte-budget constraint. Full M5
  acceptance still requires running the live gate and retaining its evidence.

No link requires inventing a new protocol or depending on unproven technology.
Constraints are documented and resolvable within existing tool ecosystems.

---

## Implementation Priority Timeline

| Phase | Must-Resolve Before | Items |
|-------|--------------------|---------|
| **M0/M1** | First code lands | `oapi-codegen-exp v0.1.0` pinned (M0); `docker-compose.yml` (PostgreSQL 18 + Temporal `auto-setup` 1.25.2) shipped (M0); Ent `v0.14.6` and Atlas `arigaio/atlas:1.2.0` pinned, 5 Ent schemas + first migration shipped (M1-PR1); Temporal Go SDK `>= 1.21` first-import pin and Update round-trip integration test planned for **M1-PR3** when the `DiagnosisWorkflow` shell lands (per ADR-0012 amendment and first-import rule) |
| **M2** | LLM Activity ships | LLMProvider capability detection (strict vs json_object); JSON Schema subset validation; idempotency key design for LLM + Webhook Activities; confirm `finish_reason` / refusal handling |
| **M4** | Sandbox ships | Egress allowlist scheme tested (iptables or egress proxy); image digest pin in CI; short-lived credential injection; Docker daemon privilege boundary documented; `/workspace/out/` output mount validated |
| **M5** | Diagnosis room ships | Run `make diagnosis-live-browser-smoke` against a real backend/worker stack and retain the validator-checked JSON proof |

Items not completed by their deadline are blockers for that milestone's
acceptance. Items may be started earlier but must be proven (not just designed)
before milestone delivery.
