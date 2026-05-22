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
- provider-capability detection at initialization (strict schema vs json_object)
- prompt templates (single alert, cascade, alert storm)
- JSON output parser with schema validation
- retry mechanism: 3 attempts with validation error fed back to LLM
- `finish_reason` / refusal / truncation checked before accepting output
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
    GenerateReport(ctx context.Context, req ReportRequest) (*SubReport, error)
}

type ReportRequest struct {
    Evidence       EvidenceSnapshot
    PromptTemplate string
    OutputSchema   json.RawMessage // JSON Schema for validation
}
```

The implementation is a thin HTTP client calling an OpenAI-compatible
`/chat/completions` endpoint. No agent framework, no tool calling, no multi-turn
conversation at this stage.

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

Deliverables:

- `ContainerProvider` interface
- self-built Docker sandbox (non-root, readonly fs, network allowlist, CPU/memory
  limits, fixed timeout)
- evidence injection via mounted workspace files
- tool scripts: metric query helper, topology lookup helper
- output extraction via /workspace/out/output.json (file-based contract)
- cleanup on success, failure, and timeout
- quality comparison: sandbox-augmented report vs direct LLM report (M2 baseline)

### Sandbox Architecture

The Go control plane owns the full lifecycle. The agent framework owns
reasoning. See [architecture.md](../architecture.md) for the complete boundary.

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
is pre-built with the agent runtime (Python/Go/OpenClaw/any framework).

### Sandbox Security Constraints

- **Image digest pinning**: container image must be referenced by digest
  (`openclarion-agent@sha256:...`), not mutable tag, in all non-dev configs.
- **Short-lived credentials**: if the agent needs API tokens (LLM, Prometheus),
  Go issues short-lived tokens (TTL <= container timeout) via environment
  variable injection. No long-lived secrets inside container.
- **Docker daemon access**: V1 uses host Docker socket. Post-V1 considers
  rootless Docker or dedicated sandbox host with mTLS-protected API.
- **Egress control prerequisite**: egress allowlist design (iptables or egress
  proxy) must be tested before M4 acceptance, not just documented. SaaS LLM
  targets with rotating IPs require domain-based egress proxy even in V1.
- **Writable mount**: only `/workspace/out/` (writable tmpfs, capped 10MB);
  all other mounts are read-only.

### Decision Gate

After M4 delivery, evaluate:
- Does sandbox-augmented analysis measurably improve report quality?
- Is the operational overhead justified?
- Should the report-enhancement track proceed, iterate, or stay deferred?

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
