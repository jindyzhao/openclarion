# Phase 04: AI Integration

## Goal

Implement the headless LLM report loop (M2) using a lightweight OpenAI-compatible
provider. Agent sandbox exploration (M4) is a separate later milestone that does not
block report loop delivery.

## 04.1 Headless LLMProvider (M2)

Deliverables:

- `LLMProvider` interface
- fake provider (deterministic output for tests)
- OpenAI-compatible provider (direct HTTP, no agent framework)
- runtime provider injection from env into Temporal Activities
- provider-capability detection at initialization (strict schema vs json_object)
- prompt templates (single alert, cascade, alert storm)
- JSON output parser with schema validation
- retry mechanism: 3 attempts with validation error fed back to LLM
- `finish_reason` / refusal / truncation checked before accepting output
- SubReport and FinalReport persistence schemas + repository
- ReportFanOutWorkflow, ReportBatchWorkflow, and FinalReportWorkflow
- IMProvider interface and Webhook implementation
- report notification flow after FinalReport persistence
- report read APIs: list FinalReports and get detail with linked SubReports
- golden prompt tests (validate structure, not content)

### Idempotency and Ordering

- LLM Activity must carry an idempotency key (e.g. `snapshotID + groupIndex`)
  to prevent duplicate SubReports on Temporal retry.
- Webhook Activity must carry an idempotency key to prevent duplicate
  notifications.
- FinalReport persistence Activity must succeed **before** IMProvider
  notification Activity starts. Notification without persisted report is a bug.

### LLM Integration Pattern

```go
type LLMProvider interface {
    GenerateJSON(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

type LLMRequest struct {
    Messages       []LLMMessage
    OutputSchema   json.RawMessage
    OutputSchemaID string
    IdempotencyKey string
}
```

The implementation is a thin HTTP client calling an OpenAI-compatible
`/chat/completions` endpoint. No agent framework, no tool calling, no multi-turn
conversation at this stage. Prompt construction is owned by
`internal/usecases/reportprompt/`; provider implementations only execute the
request and return raw JSON plus completion metadata.

### Output Structuring Strategy

Use a two-tier approach depending on provider capability:

1. **Preferred**: if the provider supports OpenAI Structured Outputs
   (`response_format: { type: "json_schema", json_schema: { strict: true } }`),
   use it. This significantly reduces format errors but does not eliminate all
   failure modes. The caller must still check `finish_reason` for truncation,
   inspect for refusal responses, and parse/validate before persistence.
2. **Fallback**: if the provider does not support `strict: true` (e.g. older
   models, non-OpenAI providers), request `response_format: { type: "json_object" }`
   and validate output against JSON Schema client-side. Retry up to 3 times with
   the validation error message fed back as user context.

Both paths must validate and parse the response before persistence. The
difference is retry frequency: strict mode rarely needs retry for schema
violations, but may still fail on refusal or incomplete output.

### Golden Prompt Test Criteria

Tests validate **structure correctness**, not content:

- output parses as valid JSON
- output conforms to SubReport/FinalReport JSON Schema
- required fields are non-empty
- severity and category values are valid enum members
- output length is within reasonable bounds

## 04.2 Agent Sandbox Baseline and Exploration (M4)

This section does not block M2 or M3. It has two parts:

- a minimum sandbox baseline required before M5 short-conversation diagnosis
- an exploratory quality track that compares sandbox-augmented reports with the
  M2 direct LLM baseline

M4 sandbox augmentation belongs to the Insight Pipeline only when it produces a
schema-validated report artifact for the report workflow. It must not absorb
M5 diagnosis-room conversation state or workspace action authority. The
boundary between the automated pipeline and the human Agent Workspace is
defined in
[insight-pipeline-agent-workspace.md](../insight-pipeline-agent-workspace.md).

Deliverables:

- `ContainerProvider` interface
- agent runtime selection gate for operator-supplied runtime candidates
- runtime-agnostic Docker sandbox (non-root, readonly fs, network allowlist,
  CPU/memory/PID limits, fixed timeout)
- evidence injection via mounted workspace files
- tool scripts: metric query helper, topology lookup helper
- output extraction via /workspace/out/output.json (file-based contract)
- cleanup on success, failure, and timeout
- live Docker Provider smoke (`make container-provider-smoke`)
- quality comparison: sandbox-augmented report vs direct LLM report (M2 baseline)

### Sandbox Architecture

The Go control plane owns the full lifecycle. The agent framework owns
reasoning. See [architecture.md](../architecture.md) for the complete boundary.
The concrete runtime is chosen by
[agent-runtime-selection.md](../agent-runtime-selection.md); M4 implementation
must not grow a custom agent framework before runtime candidates are evaluated
against the same sandbox smoke.

```text
Go control plane (Activity in Temporal workflow)
  -> docker create (non-root, resource limits, network allowlist)
  -> mount /workspace/evidence.json     (readonly, from EvidenceSnapshot)
  -> mount /workspace/agent_config/     (readonly, from agents/report-enhancer/)
  -> docker start
  -> wait with timeout (context.WithTimeout)
  -> read /workspace/out/output.json
  -> validate against SubReport JSON Schema
  -> docker stop + docker rm
```

### Agent Config Boundary

Go mounts `agents/report-enhancer/` into the container as `/workspace/agent_config/`
but never reads its contents. The agent runtime inside the container uses this
config to determine:
- its role and system prompt
- which skills to load (tool definitions, helper scripts)
- which data sources to connect to (V1: direct HTTP; post-V1: MCP-over-Streamable-HTTP)
- its reasoning strategy

Changing agent behavior (skills, prompts, tool endpoints) does not require
Go code changes or redeployment. See [architecture.md](../architecture.md)
Agent Config Structure.

### Concrete M4 Sandbox Call Chain

```text
1. Temporal ReportFanOutWorkflow determines: this group needs sandbox analysis
2. Workflow starts Activity: ContainerProvider.Run(agentName, evidence)
3. ContainerProvider implementation:
   a. docker create --user=nonroot --memory=512m --cpus=1 \
      -v /data/evidence-{id}.json:/workspace/evidence.json:ro \
      -v /repo/agents/report-enhancer:/workspace/agent_config:ro \
      openclarion-agent@sha256:<pinned-digest>
   b. docker start {container_id}
   c. docker wait {container_id} --timeout=5m
   d. docker cp {container_id}:/workspace/out/output.json /tmp/output-{id}.json
   e. docker rm {container_id}
4. Activity reads output.json, validates against SubReport schema
5. Activity returns validated SubReport to workflow
6. Workflow merges into FinalReport
```

Each step uses existing Docker Engine API (no custom protocol). Container image
is pre-built with the selected runtime adapter for an operator-supplied
candidate and referenced by digest. The local `make
container-provider-smoke` gate proves the Go `ContainerProvider.Run` path
against a real Docker daemon before a candidate runtime is selected.

### Runtime Selection Gate

Before implementing tool orchestration inside the sandbox, M4 must prove at
least one candidate runtime against the ADR-0013 file contract:

Current named candidates are evaluation examples, not platform enums:

- OpenClaw candidate: embedded run or SDK/Gateway path can run in a short-lived
  container with channel/gateway tools disabled and output normalized to
  `/workspace/out/output.json`
- Hermes Agent candidate: one-shot CLI or equivalent path can run with memory
  scoped to tmpfs, dangerous tools denied, and strict JSON emitted to the file
  contract
- custom thin runner candidate: limited to file reading, provider/tool calls,
  and JSON normalization; no custom planning, persistent memory, approval
  system, or multi-agent orchestration without a new decision

The first candidate that passes security/lifecycle smoke and shows acceptable
quality delta becomes the M4/M5 baseline.

Current contract proof: `make custom-thin-runner-smoke` builds the local
scratch-based custom runner, pushes it to an ephemeral localhost registry to
obtain a real `repo@sha256` image reference, and runs it through both
`make agent-runtime-smoke` and `make container-provider-smoke`. This proves the
file contract and lifecycle path; it does not replace external framework
candidate evaluation or the quality-delta decision. The same smoke packages the
metric/topology helper binaries and proves the topology helper inside the
digest-pinned image with an alternate entrypoint.

Tool helper proof: `make agent-tool-scripts-test` covers the first read-only
metric and topology helper contracts. `scripts/agent_tool_metric_query` calls
Prometheus `/api/v1/query` with bounded response parsing and JSON-object output.
`scripts/agent_tool_topology_lookup` reads a bounded static JSON topology file
and returns a service-centered JSON object. Packaging those helpers into a
candidate runtime image is proven by the custom thin runner smoke.

Minimum baseline audit: `make sandbox-baseline-audit` covers the code-level
M4/M5 sandbox baseline. It emits a JSON proof for ADR-0013 file paths,
network-none batch defaults, M5 read-only turn input mounts, Docker runtime
security posture, resource limits, allowlist subset enforcement, and bounded
strict request/raw-output JSON validation. It is not a live Docker smoke;
manual smoke targets remain the evidence for daemon cleanup behavior.

M5 minimum sandbox baseline: accepted locally on 2026-05-28 and revalidated on
2026-05-29 after `make sandbox-baseline-audit`,
`make custom-thin-runner-smoke`, `make container-provider-smoke`,
`make container-provider-timeout-smoke`,
`make container-provider-output-cap-smoke`, and
`make egress-allowdeny-smoke` passed against the current tree. This closes the
file-I/O, helper-packaging, cleanup, output-cap, and egress allow/deny
foundation for M5 short-conversation implementation. It also closes the
runtime-agnostic Docker sandbox baseline. It does not close the M4
report-quality comparison or runtime framework decision.

Quality comparison harness: `make sandbox-quality-compare-test` covers the
offline direct-vs-sandbox SubReport comparator in
`scripts/sandbox_quality_compare`. The comparator validates both candidate
outputs with the production `reportdraft.ParseSubReport` path, emits
machine-readable metric deltas, supports manifest-mode batches of
representative direct/sandbox sample pairs, records `sample_basis` plus
per-case alert scenario labels, emits `scenario_coverage`, and can fail on
conservative structural regression. Manual
`make sandbox-m4-quality-compare` runs that manifest-mode comparison with
fail-on-regression and writes a retained `quality-comparison.json` output
without overwriting an existing file. This does not close the M4 quality-delta
decision; real direct and sandbox outputs from representative alert evidence
are still required.

M4 decision gate: `make sandbox-m4-decision-test` covers the offline
proceed/iterate/defer decision helper in `scripts/sandbox_m4_decision`. The
manual `make sandbox-m4-decision` target requires three evidence files: the
baseline audit JSON, the manifest-mode quality comparison JSON, and a human
review evidence JSON that records representative sample basis plus runtime
smoke results. The quality comparison must cover `single_alert`, `cascade`, and
`alert_storm` before the decision can proceed, and the review evidence
`sample_basis` must match the quality comparison `sample_basis` so an operator
cannot attach stale review evidence to a different sample. Candidate runtime
IDs remain evidence-supplied values; any selected pass candidate must bind its
digest-pinned runtime ref and cite every required runtime smoke name through
`runtime_smoke_refs`, keeping the control plane generic. See
[../sandbox-m4-decision.md](../sandbox-m4-decision.md).

Evidence packet assembly: `make sandbox-m4-evidence-packet-test` covers the
manual packet helper in `scripts/sandbox_m4_evidence_packet`. The manual
`make sandbox-m4-evidence-packet` target runs the baseline audit and quality
comparison, copies the review evidence, runs the decision gate, and writes all
outputs into one empty directory for audit. It rejects weak generated helper
artifacts before writing them, including missing baseline pass checks, missing
quality sample/scenario evidence, review evidence whose `sample_basis` does not
match the generated quality comparison, weak review evidence, and invalid
decision outputs. See
[../sandbox-m4-evidence-packet.md](../sandbox-m4-evidence-packet.md).

### Sandbox Security Constraints

- **Image digest pinning**: container image must be referenced by digest
  (`openclarion-agent@sha256:...`), not mutable tag, in all non-dev configs.
- **Short-lived credentials**: if the agent needs API tokens (LLM, Prometheus),
  Go issues short-lived tokens (TTL <= container timeout) via environment
  variable injection. No long-lived secrets inside container.
- **Docker daemon access**: V1 uses host Docker socket. Post-V1 considers
  rootless Docker or dedicated sandbox host with mTLS-protected API. The
  boundary is documented in
  [../docker-daemon-boundary.md](../docker-daemon-boundary.md).
- **Egress control prerequisite**: egress allowlist design (iptables or egress
  proxy) must be tested before M4 acceptance, not just documented. SaaS LLM
  targets with rotating IPs require domain-based egress proxy even in V1.
  Allowlist requests use exact `host[:port]` targets and must pass the Docker
  provider's subset enforcer before container creation. `make
  egress-allowdeny-smoke` proves the Docker internal-network + proxy topology;
  production wiring into the accepted candidate runtime remains pending.
- **Writable mount**: only `/workspace/out/` (private writable output mount
  capped by `fsize` ulimit and Go read limit); all other mounts are read-only.

### Decision Gate

After M4 delivery, evaluate:
- Does sandbox-augmented analysis measurably improve report quality?
- Is the operational overhead justified?
- Should the report-enhancement track proceed, iterate, or stay deferred?

The decision must be recorded through `scripts/sandbox_m4_decision` once real
candidate runtime evidence exists. Until then, the decision remains pending.

M5 does not depend on the quality-comparison outcome. It depends on the minimum
sandbox baseline: non-root execution, resource limits, authenticated control-
plane access, timeout handling, cleanup, and auditable input/output capture.

## 04.3 Short-Conversation Interactive Diagnosis Room (M5)

> Implements the per-turn container invocation contract from
> [ADR-0013](../../adr/ADR-0013-per-turn-container-invocation.md).

M5 is V1 required at minimum-viable scope. Long-session features remain deferred
to post-V1 work.

### M5 Per-Turn Call Chain

M5 reuses the M4 batch container model. Each turn is a separate container run
(see [architecture.md](../architecture.md) M5 Interactive Model for rationale):

```text
1. User sends message via WebSocket
2. Go WS handler -> deny-list filter -> passes to use case
3. Use case -> DiagnosisRoomOrchestrator.SubmitTurn(sessionID, message)
4. Temporal implementation sends Update to DiagnosisRoomWorkflow
5. Workflow Update handler starts Activity:
   ContainerProvider.Run("diagnosis-assistant", {
     evidence: original_evidence.json,
     conversation: [all previous turns],
     message: filtered user message
   })
6. ContainerProvider:
   a. docker create with agent_config from agents/diagnosis-assistant/
   b. mount evidence.json + conversation.json + message.json (readonly)
   c. docker start -> agent loads full context -> reasons -> /workspace/out/output.json
   d. docker wait (turn-level timeout, e.g. 2 minutes)
   e. read /workspace/out/output.json -> validate -> docker rm
7. Activity returns agent response to Update handler
8. Update handler: persist ChatTurn (via persistence Activity)
9. Update handler: append to workflow conversation history, return response
10. SubmitTurn returns response to use case (Update completes synchronously)
11. Go WS handler sends response to browser
12. Repeat 1-11 until turn limit or session timeout
13. On close: workflow runs final notification Activity (IMProvider)
```

Key design decisions:
- Per-turn container invocation (no long-running process to manage)
- Conversation state durably stored in Temporal workflow, not container memory
- Crash recovery: replay from last persisted ChatTurn
- Container startup cost (~1-3s) acceptable for V1 short-conversation

M5 deliverables:

- Next.js diagnosis room
- AuthProvider integration (OIDC)
- RBAC for owner and admin roles (leader is deferred)
- WebSocket connection: browser <-> Go control plane (per-turn file contract)
- chat turn persistence
- unsafe instruction filter
- final group notification after session expiry

## Acceptance

- M2: headless LLM reports pass golden tests using direct LLM API
- M4: sandbox agent produces enhanced reports with quality delta vs M2
- M5: short-conversation interactive diagnosis ships at minimum-viable scope (V1 required); long-session features remain deferred
