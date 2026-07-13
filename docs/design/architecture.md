# Architecture Layering

This document defines the layering contract for OpenClarion. It complements the
high-level system view in [README.md](README.md) and the ADRs by specifying how
packages, interfaces, and dependencies are arranged.

## Layer Diagram

```text
+-------------------------------------------------------------+
| Frontend (Next.js 16, React 19)                             |
|   - consumes generated TS types from OpenAPI                |
+-------------------------- HTTP/WS --------------------------+
| Transport (std net/http + oapi-codegen-exp ServerInterface) |
|   - handlers implement generated interface                  |
|   - thin: parse, authorize, delegate to use cases           |
+-------------------------------------------------------------+
| Use Cases / Application Services                            |
|   - business workflows expressed as Go interfaces           |
|   - depend on orchestrator ports and provider interfaces    |
+-------------------------------------------------------------+
| Domain                                                      |
|   - entity types, value objects, invariants                 |
|   - no I/O, no framework imports                            |
+-------------------------------------------------------------+
| Orchestrator Ports (business-level)                         |
|   - ReportOrchestrator, DiagnosisRoomOrchestrator           |
|   - implementation lives in Temporal workflows + activities |
+-------------------------------------------------------------+
| Provider Interfaces                                         |
|   - MetricsProvider, AlertSource adapters, CMDBProvider,    |
|     IMProvider, AuthProvider, ContainerProvider, LLMProvider|
|   - one capability per interface                            |
+--------------- external systems (real or fake) -------------+
| Persistence: PostgreSQL via Ent + Atlas                     |
| Workflow runtime: Temporal Go SDK                           |
+-------------------------------------------------------------+
```

## Dependency Direction

* Outer layers depend on inner layers, never the reverse.
* Domain has zero external dependencies (no Ent, no Temporal, no HTTP).
* Use cases depend only on domain types, orchestrator ports, and provider
  interfaces.
* Orchestrator implementations and provider implementations live in adapter
  packages and are wired in `cmd/` only.

## Alert-First, Signal-Capable Boundary

The current product and public language stay focused on intelligent alert
analysis. `AlertEvent` and `AlertGroup` remain the implementation and contract
names for the MVP. Architecture discussions may use `SignalEvent`,
`SignalGroup`, or `CaseGroup` as aliases for future extension, but those names
are not code rename targets until OpenAPI compatibility, database migrations,
dashboard copy, and operator documentation are planned together.

New code should keep alert behavior first while avoiding hard binding to one
monitoring system. Provider interfaces should describe capabilities, not vendor
products. Evidence building should accept frozen inputs from providers and
produce `EvidenceSnapshot` records that downstream AI can analyze without
calling untracked external systems.

For the detailed extension boundary, see
[alert-first-signal-extension.md](alert-first-signal-extension.md).

## Insight Pipeline vs Agent Workspace

OpenClarion has two logical subsystems inside one product:

- **Insight Pipeline**: the automatic alert-to-evidence-to-report path. It owns
  ingestion, grouping, `EvidenceSnapshot` creation, report workflows,
  `SubReport` / `FinalReport` persistence, and report notification.
- **Agent Workspace**: the user-initiated follow-up path. It owns diagnosis
  rooms, authenticated WebSocket interaction, `ChatSession` / `ChatTurn`
  persistence, per-turn sandbox calls, action proposals, and audit handoff.

They share `EvidenceSnapshot`, `FinalReport`, provider interfaces, auth/audit
primitives, OpenAPI, PostgreSQL, Temporal infrastructure, and observability.
They must not share workflow lifecycle, output schemas, conversation state, or
production-changing operation authority.

This is a logical boundary, not a deployment split. Keep one repository and
one product surface until runtime scale, team ownership, or isolation evidence
requires a worker or service split. See
[insight-pipeline-agent-workspace.md](insight-pipeline-agent-workspace.md).

## Business-Level Orchestrator Ports

Use cases must not depend on a generic "WorkflowEngine" abstraction. Instead,
each use case defines a business-level port whose method names express the
business operation:

```go
// usecases/orchestrator.go (port owned by the application layer)
type ReportOrchestrator interface {
    GenerateFinalReport(ctx context.Context, req GenerateReportInput) (ReportHandle, error)
}

type DiagnosisRoomOrchestrator interface {
    StartSession(ctx context.Context, req StartDiagnosisInput) (SessionHandle, error)
    SubmitTurn(ctx context.Context, sessionID string, message string) error
    Query(ctx context.Context, sessionID string) (SessionState, error)
    Close(ctx context.Context, sessionID string, reason CloseReason) error
}
```

The Temporal-backed implementation lives in `internal/orchestrator/temporal/`
and is the only layer aware of workflow IDs, signals, and queries. Swapping
the implementation (or running a fake in tests) does not require changes
above this line.

## Provider Interface Discipline

* One capability per interface. Resist the temptation to add convenience
  methods that combine capabilities.
* Each provider interface ships with a Responsibilities and Non-
  responsibilities section in its godoc.
* Real and fake implementations live in sibling packages and are selected
  through composition in `cmd/`.
* Future non-monitoring signal providers, such as claims, policy, document,
  weather, GIS, fraud, customer-service, or vendor data providers, must follow
  the same one-capability rule and feed the evidence pipeline instead of
  bypassing it.

## Control Plane vs Agent Framework Boundary

The Go control plane and the Agent framework (inside sandbox containers) have
strictly separated responsibilities. Neither should attempt the other's job.

### Go + Temporal Owns (Control Plane)

* **when** to invoke an agent (trigger conditions, scheduling)
* **what data** to pass in (evidence, conversation history)
* **lifecycle** (timeout, cleanup, crash recovery)
* **output validation** (JSON Schema conformance, refusal detection)
* **persistence** (chat turns, reports, audit trail -> PostgreSQL)
* **human message relay** (WebSocket -> Temporal Update -> Activity; Signals for close/cancel/fallback only)
* **security perimeter** (non-root, resource limits, deny-list filter)

### Agent Framework Owns (Inside Container)

* **role definition** (system prompt, persona, behavioral constraints)
* **reasoning lifecycle** (Eino ChatModelAgent iteration and cancellation
  within one invocation)
* **invocation-local state** used by the framework while producing one response

CloudWeGo Eino `v0.9.12` is the selected M5 implementation in the
[agent runtime selection record](agent-runtime-selection.md). The dependency
belongs to the isolated sandbox module and is not linked into the Go
control-plane service. V1 does not register in-container tools, framework
memory, or sub-Agents. The file boundary remains governed by
[ADR-0013](../adr/ADR-0013-per-turn-container-invocation.md).

### Go Does NOT Own

* the Agent framework's internal iteration and message mechanics inside one
  invocation

### Agent Does NOT Own

* when it gets invoked
* where its input comes from
* where its output goes
* its own lifecycle (timeout, cleanup)
* direct production-system access or tool authorization
* persistence of conversation state across turns

### Data Flow Contract

The contract between control plane and agent sandbox is files:

```text
Input (mounted readonly by Go):
  /workspace/evidence.json         # structured evidence from EvidenceSnapshot
  /workspace/conversation.json     # conversation history (M5 only)
  /workspace/message.json          # latest user message (M5 only)
  /workspace/agent_config/         # reviewed agent instructions (V1)

Output (writable capped output mount, read by Go after container exits):
  /workspace/out/output.json       # structured response (schema-validated by Go)
```

Go never reads `agent_config/` contents. It mounts the directory opaquely.
The agent writes only to `/workspace/out/`. Go validates output.json against
the expected JSON Schema before accepting it.

### Agent Config Structure (Reference)

```text
config/agents/
  diagnosis-assistant/
    instructions.md               # reviewed V1 system instructions
```

V1 mounts reviewed instructions only. It does not load framework skills,
register direct data-source tools, or grant the sandbox access to production
systems. Any future tool configuration must preserve control-plane approval,
tenant scope, evidence provenance, and the ADR-0013 boundary.

## M5 Interactive Model: Per-Turn Container Invocation

M5 reuses M4's batch container model. Each turn is a separate container run:

```text
Turn 1:
  Go mounts: evidence.json + conversation.json (empty) + message.json (user msg 1)
  Container runs -> agent responds -> /workspace/out/output.json
  Go reads + validates output, persists turn, appends to conversation history

Turn 2:
  Go mounts: evidence.json + conversation.json ([turn1]) + message.json (user msg 2)
  Container runs -> agent sees full history -> responds -> /workspace/out/output.json
  Go reads + validates output, persists turn, appends to conversation history

... repeat until turn limit or timeout ...
```

This means:
* No long-running container process to manage
* No stdin/stdout streaming protocol to design
* Crash recovery is trivial (replay from last persisted turn)
* Conversation state lives in Temporal workflow (durable), not container memory
* Each turn pays ~1-3s container startup cost (acceptable for V1 short-conversation)

Post-V1 optimization: keep container alive with HTTP endpoint inside for lower
latency. This is a ContainerProvider implementation detail that doesn't change
the workflow contract.

## Forbidden Patterns

* HTTP handlers calling Ent or Temporal directly.
* Use cases importing `temporal.io/sdk/...` packages.
* Provider implementations importing each other.
* Domain types depending on generated code.
* `internal/temporal/` packages reaching into `internal/usecases/`.
* Go code reading or interpreting agent config files.
* First-party manifests or non-test control-plane source hard-coding
  agent-framework dependencies or runtime-family names before the M4 runtime
  selection gate accepts a baseline.
* Agent containers accessing network endpoints not in their allowlist.
* Agent containers writing outside `/workspace/`.

These rules are enforced by the architecture CI gate (see
[ci/README.md](ci/README.md), introduced at M2).

## Package Layout (Target)

```
cmd/openclarion/                  # composition root
internal/
  domain/                         # pure types, invariants
  usecases/                       # business operations (depend on ports)
  orchestrator/
    ports.go                      # ReportOrchestrator, DiagnosisRoomOrchestrator
    temporal/                     # Temporal-backed implementation
  providers/
    metrics/, cmdb/, im/, auth/, container/, llm/
    # future extension examples: claims/, policy/, documents/, fraud/
  transport/
    http/                         # generated server + handler adapters
    ws/                           # WebSocket proxy
  persistence/
    ent/                          # Ent schemas and generated client
    repository/                   # repository implementations
api/openapi.yaml                  # contract source
agents/                           # agent workspace configs (NOT read by Go)
  report-enhancer/                # M4 sandbox agent
  diagnosis-assistant/            # M5 interactive agent
web/                              # Next.js frontend
```

## See Also

- [adr/ADR-0002-ai-agent-black-box.md](../adr/ADR-0002-ai-agent-black-box.md)
- [adr/ADR-0003-provider-extension-interfaces.md](../adr/ADR-0003-provider-extension-interfaces.md)
- [adr/ADR-0004-temporal-workflow-engine.md](../adr/ADR-0004-temporal-workflow-engine.md)
- [adr/ADR-0005-ephemeral-container-security.md](../adr/ADR-0005-ephemeral-container-security.md)
- [adr/ADR-0007-openapi-31-native-toolchain.md](../adr/ADR-0007-openapi-31-native-toolchain.md)
- [adr/ADR-0009-go-control-plane-scheduling.md](../adr/ADR-0009-go-control-plane-scheduling.md)
- [adr/ADR-0013-per-turn-container-invocation.md](../adr/ADR-0013-per-turn-container-invocation.md)
- [alert-first-signal-extension.md](alert-first-signal-extension.md)
- [CODING_STYLE.md](CODING_STYLE.md)
