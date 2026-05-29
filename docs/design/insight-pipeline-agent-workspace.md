# Insight Pipeline and Agent Workspace Boundary

OpenClarion remains an intelligent alert analysis product. The design should
separate the automated alert insight flow from the human interactive workspace,
while keeping them in one product and one repository for the current stage.

Decision:

- separate the two subsystems logically;
- keep a unified product surface and shared governance model;
- do not split repositories, deployments, or databases until scale, ownership,
  or runtime isolation evidence requires it.

## Subsystems

### Insight Pipeline

The Insight Pipeline is the automatic path from incoming alert state to a
schema-validated report.

Responsibilities:

- ingest alert or signal state through provider interfaces;
- normalize, deduplicate, and group alerts deterministically;
- freeze all downstream AI input into `EvidenceSnapshot`;
- run Temporal report workflows;
- call LLM or sandboxed analysis only through validated provider/runtime
  boundaries;
- persist `SubReport` and `FinalReport`;
- notify humans through `IMProvider`;
- retain replayable audit evidence.

Characteristics:

- automatically triggered;
- batch-oriented or workflow-oriented;
- deterministic, retryable, and auditable;
- outputs structured reports;
- does not depend on long conversation state;
- does not let an agent own workflow lifecycle.

Current implementation anchors:

- `AlertEvent`
- `AlertGroup`
- `EvidenceSnapshot`
- `ReportFanOutWorkflow`
- `ReportBatchWorkflow`
- `FinalReportWorkflow`
- `SubReport`
- `FinalReport`
- report notification delivery logs

### Agent Workspace

The Agent Workspace is the user-initiated path for asking follow-up questions,
explaining a report, drafting next actions, and handing off controlled actions
to governed systems.

Responsibilities:

- authenticate and authorize the user;
- open or reuse a bounded diagnosis room from a report, task, or evidence
  snapshot;
- preserve `ChatSession` and `ChatTurn` history;
- call a sandboxed agent per turn;
- explain the frozen evidence and persisted report;
- draft recommendations or action proposals;
- record all interaction and lifecycle audit events;
- hand off high-risk operations to an external approval or governance system.

Characteristics:

- user initiated;
- multi-turn but bounded in V1;
- UI/workspace oriented;
- requires identity, RBAC, session state, and transcript persistence;
- may suggest actions but must not bypass approval;
- is the natural place to evaluate an external workspace-capable runtime after
  the M4 gate accepts a baseline.

Current implementation anchors:

- `DiagnosisRoomWorkflow`
- `DiagnosisTask`
- `ChatSession`
- `ChatTurn`
- `DiagnosisTaskEvent`
- WebSocket relay
- single-use WebSocket tickets
- per-turn `ContainerProvider.Run`

## Shared Foundations

The two subsystems intentionally share these foundations:

- `EvidenceSnapshot`
- `FinalReport`
- provider interfaces
- `AuthProvider` and RBAC primitives
- audit event patterns
- OpenAPI
- PostgreSQL
- Temporal infrastructure
- observability and CI governance
- sandbox runtime boundary

Sharing these foundations keeps the product coherent: users can move from an
automatic report into a governed follow-up room without duplicating evidence or
inventing a second audit model.

## Non-Shared Boundaries

The two subsystems must not share these responsibilities:

| Boundary | Insight Pipeline | Agent Workspace |
|----------|------------------|-----------------|
| Workflow lifecycle | report workflows own fan-out, fan-in, persistence, and notification | room workflow owns turn submission, timers, close/cancel, and reconnect state |
| Output schema | `SubReport` and `FinalReport` schemas | diagnosis-turn output and future action-proposal schemas |
| State model | snapshots, report rows, delivery rows | sessions, turns, room audit events |
| Trigger model | scheduler, webhook, replay, or alert-source event | authenticated user action |
| Authority | reports what happened and what is recommended | explains, collaborates, and drafts next steps |
| Operation boundary | no production-changing action execution | no direct high-risk execution; hand off to approval/governance |

## Evidence Immutability

Long conversation state must not mutate the original evidence package.

- `EvidenceSnapshot` is frozen input evidence.
- `FinalReport` is persisted report output derived from snapshots.
- Workspace follow-up state lives in `ChatSession`, `ChatTurn`, and
  `DiagnosisTaskEvent`.
- If a later turn discovers new facts, it should create a new retained artifact
  or action proposal rather than editing the original snapshot.

This keeps report provenance stable and makes later audits clear: the pipeline
answer and the workspace conversation are related but distinct artifacts.

## Action Boundary

OpenClarion does not directly execute production-changing operations in V1.
The workspace may draft an action, explain risk, collect context, and send a
handoff request, but the final operation must go through the system that owns
that resource and approval policy.

Examples:

- infrastructure resource creation, renewal, or release belongs to the
  resource-governance system that owns those resources;
- incident escalation belongs to the paging or ITSM system that owns escalation
  policy;
- business-impacting insurance decisions belong to the responsible business
  workflow with human review and audit controls.

## Deployment Policy

Do not split deployments yet.

The current practical shape is:

```text
OpenClarion product
├── Insight Pipeline
│   ├── alert ingest
│   ├── grouping
│   ├── evidence snapshots
│   ├── report workflows
│   └── reports and notifications
└── Agent Workspace
    ├── diagnosis rooms
    ├── conversation sessions
    ├── tool and sandbox calls
    ├── action proposals
    └── audit and approval handoff
```

The first operational split, if needed, should be worker-level or task-queue
level: separate report workers from workspace workers while retaining the same
repository, database, OpenAPI, provider contracts, and audit model. A repository
or product split requires stronger evidence: independent ownership, independent
release cadence, incompatible runtime isolation needs, or unacceptable shared
blast radius.

## Design Consequences

- Do not merge report generation and diagnosis rooms into one workflow.
- Keep report schemas and diagnosis-turn schemas separate.
- Keep `ReportBatchWorkflow` / `FinalReportWorkflow` separate from
  `DiagnosisRoomWorkflow`.
- Keep agent framework dependencies inside sandbox images until the runtime
  selection gate accepts a baseline.
- Let any accepted external agent runtime fit behind the workspace/sandbox
  boundary; do not move its planning or memory model into the Go control plane.
- Keep the public MVP terminology alert-first. Signal-capable architecture is
  an extension boundary, not a rename.
