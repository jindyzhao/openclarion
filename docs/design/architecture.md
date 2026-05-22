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
|   - MetricsProvider, CMDBProvider, IMProvider,              |
|     AuthProvider, ContainerProvider, LLMProvider            |
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
* **skills/tools** loading and registration
* **data source connections** (V1: direct HTTP to Prometheus, K8s, etc.;
  post-V1: MCP-over-Streamable-HTTP if tool standardization adds value)
* **reasoning strategy** (ReAct, plan-and-execute, multi-Agent, etc.)
* **sub-Agent composition** (analysis Agent, verification Agent, etc.)
* **internal state** between tool calls within a single invocation

### Go Does NOT Own

* which skills the agent loads
* what data sources the agent connects to (Go only controls the network allowlist)
* how the agent reasons internally
* sub-Agent delegation strategies

### Agent Does NOT Own

* when it gets invoked
* where its input comes from
* where its output goes
* its own lifecycle (timeout, cleanup)
* persistence of conversation state across turns

### Data Flow Contract

The contract between control plane and agent sandbox is files:

```text
Input (mounted readonly by Go):
  /workspace/evidence.json         # structured evidence from EvidenceSnapshot
  /workspace/conversation.json     # conversation history (M5 only)
  /workspace/message.json          # latest user message (M5 only)
  /workspace/agent_config/         # agent role, skills, tool endpoints

Output (writable tmpfs, read by Go after container exits):
  /workspace/out/output.json       # structured response (schema-validated by Go)
```

Go never reads `agent_config/` contents. It mounts the directory opaquely.
The agent writes only to `/workspace/out/`. Go validates output.json against
the expected JSON Schema before accepting it.

### Agent Config Structure (Reference)

```
agents/                            # version-controlled agent workspaces
  report-enhancer/                 # M4 sandbox agent
    agent.yaml                     # role, model, skills list, tool endpoints
    skills/                        # skill definitions
    prompts/                       # prompt templates
  diagnosis-assistant/             # M5 interactive agent
    agent.yaml                     # role, model, conversation skills, tool endpoints
    skills/
    prompts/
```

These configs are iterated by agent developers independently of Go code changes.
Changing an agent's skills or tool endpoints does not require modifying Go,
Temporal workflows, or redeploying the control plane.

V1: agent.yaml references direct HTTP tool endpoints (Prometheus, K8s API).
Post-V1: may migrate to MCP-over-Streamable-HTTP for tool standardization.

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
- [CODING_STYLE.md](CODING_STYLE.md)
