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
- prompt templates (single alert, cascade, alert storm)
- JSON output parser with schema validation
- retry mechanism: 3 attempts with validation error fed back to LLM
- golden prompt tests (validate structure, not content)

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
- stdout JSON extraction
- cleanup on success, failure, and timeout
- quality comparison: sandbox-augmented report vs direct LLM report (M2 baseline)

### Sandbox Architecture

The Go control plane owns the full lifecycle:

```text
Go control plane
  -> docker create (non-root, resource limits, network allowlist)
  -> mount /workspace/evidence.json (readonly)
  -> mount /workspace/tools/ (readonly helper scripts)
  -> docker start
  -> wait with timeout (context.WithTimeout)
  -> read /workspace/output.json
  -> docker stop + docker rm
```

The container interior can be:
- a Python script calling LLM APIs with tool access
- a Go binary with predefined analysis steps
- any agent framework (LangChain, CrewAI, custom) as long as it writes
  structured JSON to `/workspace/output.json`

### Decision Gate

After M4 delivery, evaluate:
- Does sandbox-augmented analysis measurably improve report quality?
- Is the operational overhead justified?
- Should the report-enhancement track proceed, iterate, or stay deferred?

M5 does not depend on the quality-comparison outcome. It depends on the minimum
sandbox baseline: non-root execution, resource limits, authenticated control-
plane access, timeout handling, cleanup, and auditable input/output capture.

## 04.3 Short-Conversation Interactive Diagnosis Room (M5)

M5 is V1 required at minimum-viable scope. Long-session features remain deferred
to post-V1 work.

M5 deliverables:

- Next.js diagnosis room
- AuthProvider integration (OIDC)
- RBAC for owner and admin roles (leader is deferred)
- WebSocket proxy to sandbox stdin/stdout
- chat turn persistence
- unsafe instruction filter
- final group notification after session expiry

## Acceptance

- M2: headless LLM reports pass golden tests using direct LLM API
- M4: sandbox agent produces enhanced reports with quality delta vs M2
- M5: short-conversation interactive diagnosis ships at minimum-viable scope (V1 required); long-session features remain deferred
