# Database Schema Catalog

PostgreSQL is the business source of truth. Ent schemas are the canonical
application schema definitions; Atlas migrations are the canonical migration
artifacts. See [migrations.md](./migrations.md) for the toolchain and
workflow.

## Core Entities

| Entity | Status | Purpose |
|--------|--------|---------|
| `AlertEvent` | shipped at M1-PR1 | raw alert event, fingerprint, labels, status, timing, raw payload |
| `AlertGroup` | shipped at M1-PR1 | deterministic grouping result for report fan-out |
| `EvidenceSnapshot` | shipped at M1-PR1 | enriched evidence package sent to AI providers |
| `DiagnosisTask` | shipped at M1-PR1 | workflow-bound lifecycle record |
| `DiagnosisTaskEvent` | shipped at M1-PR1 | append-only lifecycle event log for `DiagnosisTask`; `dedupe_key` UNIQUE per task allows idempotent producers |
| `AlertWindow` | M1-PR3 | replayable polling window and active alert snapshot |
| `SubReport` | M2 | per-group AI report |
| `ResolutionReport` | M2 | final report and closure outcome |
| `ChatSession` | M5 | interactive session lifecycle |
| `ChatTurn` | M5 | append-only human, assistant, system, and tool messages |
| `AuditLog` | M2+ | security and lifecycle audit trail |

## Fingerprint Discipline (M1)

Two fingerprint columns coexist on `AlertEvent`:

* `source_fingerprint` — fingerprint reported by the upstream provider
  (Alertmanager, Datadog, etc.); retained verbatim for traceability.
* `canonical_fingerprint` — `sha256(canonical(sorted(labels)))`,
  computed in-process so re-ingestion of the same logical alert always
  collapses to the same row regardless of upstream fingerprint quirks.

The natural unique key is `(source, canonical_fingerprint, starts_at)`.
`starts_at` MUST be normalised to `UTC().Truncate(time.Microsecond)`
before the canonical fingerprint is computed and before the row is
persisted; this rule is enforced by the ingestion path, not by the
database.

## Primary Keys

All business entities use the Ent default `int` primary key, which Ent
maps to PostgreSQL `bigserial`. UUID is reserved for security-sensitive
single-use tokens (see the WS ticket described in
[../SECURITY_CODING.md](../SECURITY_CODING.md)), where unguessability is
the dominant property and the row is short-lived.

Switching entity primary keys to UUID or ULID is deferred and is gated
on a concrete need (sharding, multi-region writes without a coordinator,
client-side ID generation). Per the M1-PR1 decision this choice is
locked in **before** the first migration is cut so it does not become
harder to reverse later.

## EvidenceSnapshot Idempotency (per-group, NOT cross-row global)

`EvidenceSnapshot.digest` is `sha256(canonical(payload))` and is unique
**only within a single `alert_group_id`** via the composite unique
index `UNIQUE (alert_group_id, digest)`. It is intentionally NOT a
table-wide unique constraint:

* The model is `AlertGroup` -1:N-> `EvidenceSnapshot`. A snapshot is
  always anchored to exactly one group.
* Two distinct `AlertGroup`s MAY produce snapshots whose canonical
  payload bytes happen to be identical (same labels, same provider
  responses for an overlapping incident). They are legitimately
  separate rows; a global `UNIQUE(digest)` would silently reject the
  second one and break the per-group enrichment pipeline.
* Within a single group, Activity retries that re-enrich the same
  group with the same provider responses MUST collapse to one row at
  the persistence boundary (Postgres unique-violation surfaces as
  "already known" to the workflow). Persistence is the idempotency
  boundary, not the workflow.

## DiagnosisTask Identity ((workflow_id, run_id) is the natural key)

`DiagnosisTask` represents one Temporal **workflow execution**, not
one workflow chain. The natural unique key is `(workflow_id, run_id)`,
which mirrors Temporal's own identity model:

* `workflow_id` is the business key. A chain of executions for the
  same logical workflow shares this id.
* `run_id` is the per-execution identity assigned by Temporal when an
  execution starts; it is stored as **NOT NULL** and **immutable**.
* Temporal retries that produce a new `run_id` (continue-as-new,
  reset, or scheduled retry policy) are persisted as **NEW rows**,
  not as updates to an existing `run_id` field. This preserves the
  per-execution audit trail and makes `started_at` / `finished_at`
  accurate per execution.
* `workflow_id` alone has a non-unique index for the chain-view query
  ("show me all executions of this logical workflow").

The earlier "one workflow id is one task row" model was rejected
during M1-PR1 review because it forced overwriting `run_id` on retry,
which conflicted with both the audit goal and Temporal's `(workflow_id,
run_id)` event-history boundary.

## DiagnosisTaskEvent

`DiagnosisTaskEvent` is a separate append-only table (NOT a JSONB column
on `DiagnosisTask`). Per the M1 design decision (option 2A):

* one row per lifecycle event
* `kind` is `text` (not a database enum, so adding event types does not
  require a schema migration)
* `dedupe_key` is `text NULL`, with a `UNIQUE (task_id, dedupe_key)`
  constraint. PostgreSQL allows multiple `NULL` values in a UNIQUE
  index, so producers that don't need idempotency can simply omit the
  key; producers that do (e.g. Temporal Activity retries) supply a
  stable key and the second insert is rejected.

## Foreign Keys and Composite Indexes

All inter-entity foreign keys are surfaced as explicit `field.Int`
columns in the Ent schema (rather than relying on the implicit FK
column Ent synthesises from `edge.From(...).Ref(...)`). This is
deliberate so that:

* the FK column has a stable, conventional name
  (`alert_group_id` / `evidence_snapshot_id` / `task_id`) instead of
  Ent's default `<parent>_<child_edge>` form;
* composite indexes can be defined as `index.Fields("task_id",
  "occurred_at")` with the parent FK as the leading column, so the
  index prefix actually serves the dominant read pattern (`WHERE
  parent_id = ? ORDER BY secondary`). Using `index.Edges(...).Fields(...)`
  produces the columns in the reverse order, which silently degrades
  these queries to a full index scan.

The relations at M1-PR1 are:

* `AlertEvent` <-many-to-many-> `AlertGroup` (join table
  `alert_event_groups`, cascade-delete on both sides)
* `AlertGroup` -one-to-many-> `EvidenceSnapshot` (FK
  `evidence_snapshots.alert_group_id`; **per-group** unique on
  `(alert_group_id, digest)`, NOT cross-row global)
* `EvidenceSnapshot` -one-to-many-> `DiagnosisTask` (FK
  `diagnosis_tasks.evidence_snapshot_id`; identity is
  `(workflow_id, run_id)`, NOT `workflow_id` alone)
* `DiagnosisTask` -one-to-many-> `DiagnosisTaskEvent` (FK
  `diagnosis_task_events.task_id`)

## JSONB Usage

Use JSONB for raw alert payloads, provider-specific evidence, tool
results, and model metadata. Extract commonly queried fields into typed
columns. Index JSONB columns with GIN where label-style query patterns
exist (see `AlertEvent.labels` for the canonical example).

## Retention

Raw evidence retention, report retention, and chat retention require
explicit operator configuration before public release.
