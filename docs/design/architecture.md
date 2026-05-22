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

## Forbidden Patterns

* HTTP handlers calling Ent or Temporal directly.
* Use cases importing `temporal.io/sdk/...` packages.
* Provider implementations importing each other.
* Domain types depending on generated code.
* `internal/temporal/` packages reaching into `internal/usecases/`.

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
web/                              # Next.js frontend
```

## See Also

- [adr/ADR-0009-go-control-plane-scheduling.md](../adr/ADR-0009-go-control-plane-scheduling.md)
- [adr/ADR-0003-provider-extension-interfaces.md](../adr/ADR-0003-provider-extension-interfaces.md)
- [adr/ADR-0004-temporal-workflow-engine.md](../adr/ADR-0004-temporal-workflow-engine.md)
- [adr/ADR-0007-openapi-31-native-toolchain.md](../adr/ADR-0007-openapi-31-native-toolchain.md)
- [CODING_STYLE.md](CODING_STYLE.md)
