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
| `SubReport` | shipped at M2 local | per-snapshot AI report; `(evidence_snapshot_id, idempotency_key)` is the retry-safe producer key |
| `FinalReport` | shipped at M2 local | incident/window reduction of validated SubReports; persisted before notification |
| `ReportNotificationDelivery` | shipped at M2 local | one delivery audit row per notification idempotency key; tracks pending/delivered/failed state and provider metadata |
| `DiagnosisAuthTicket` | shipped at M5 local | short-lived WebSocket ticket metadata; stores `sha256(token)`, never the raw token |
| `ChatSession` | shipped at M5 local | interactive diagnosis-room lifecycle anchored to `DiagnosisTask` |
| `ChatTurn` | shipped at M5 local | append-only human, assistant, system, and tool messages |
| `AlertSourceProfile` | shipped at M3.1 local | operator-managed alert source connection metadata; stores `secret_ref`, never secret values |
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
maps to PostgreSQL `bigserial`. Security-sensitive single-use credentials
use random opaque token material outside the primary key; persistence stores
only a cryptographic digest of that token (see the WS ticket described in
[../SECURITY_CODING.md](../SECURITY_CODING.md)).

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

## DiagnosisAuthTicket

`DiagnosisAuthTicket` is the persistence backing for the M5 WebSocket ticket
handshake. The raw ticket token is returned only at issuance time and is never
stored. The table stores `token_hash = sha256(raw_token)` with a UNIQUE index.

The row is append-mostly:

* identity and authorization metadata (`subject`, `roles`, `session_id`,
  `scope`, `issued_at`, `expires_at`) are immutable
* `consumed_at` is nullable and is set once by a conditional update requiring
  `consumed_at IS NULL` and `expires_at > now`
* concurrent consumers racing for the same ticket produce exactly one
  successful update; the rest observe `ErrTicketConsumed`
* expired tickets are not consumed, so replay attempts remain auditable

## ChatSession and ChatTurn

`ChatSession` and `ChatTurn` are the M5 short-conversation persistence
boundary. They remain tied to the intelligent alert diagnosis path: every
`ChatSession` belongs to exactly one `DiagnosisTask`, while `session_key` is
the external room id used by WebSocket tickets and reconnect flows.

The V1 model is intentionally small:

* `chat_sessions.session_key` is globally UNIQUE and immutable
* `chat_sessions.diagnosis_task_id` is UNIQUE, enforcing one diagnosis-room
  session per workflow execution in V1
* `owner_subject` is immutable and backs owner/admin RBAC resolution
* `status` is text (`open` / `closed`), not a database enum
* close metadata is explicit (`closed_at`, `close_reason`) so lifecycle
  ending is queryable

`ChatTurn` rows are append-only. The two persistence idempotency boundaries are:

* `UNIQUE (chat_session_id, message_id)` rejects browser retry / Temporal
  replay duplicates
* `UNIQUE (chat_session_id, sequence)` preserves one canonical transcript
  order for `/workspace/conversation.json`

Each turn records `role`, `actor_subject`, `content`, `metadata`, and
`occurred_at`. Workflow and WebSocket relay code still need to call this
repository boundary before the full M5 room is accepted.

## AlertSourceProfile

`AlertSourceProfile` is the first M3.1 operations-configuration table. It
records operator-managed alert source metadata for Prometheus and future
Alertmanager adapters:

* `name` is globally UNIQUE so operators have one stable display handle per
  profile
* `kind` is text (`prometheus` / `alertmanager`), not a database enum
* `base_url` stores only an HTTP(S) base URL; domain validation rejects
  userinfo, query strings, and fragments before persistence
* `auth_mode` is text (`none` / `bearer`)
* `secret_ref` stores only a deployment-managed secret reference, never a
  bearer token or credential value
* `enabled` is explicit so creating and testing a profile remains separate
  from allowing policy binding
* `labels` is JSONB for bounded operator metadata and has a GIN index

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

The relations covered by the current schema are:

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
* `DiagnosisTask` -one-to-one-> `ChatSession` (FK
  `chat_sessions.diagnosis_task_id`, UNIQUE in V1)
* `ChatSession` -one-to-many-> `ChatTurn` (FK
  `chat_turns.chat_session_id`; per-session unique keys on
  `message_id` and `sequence`)

## JSONB Usage

Use JSONB for raw alert payloads, provider-specific evidence, tool
results, and model metadata. Extract commonly queried fields into typed
columns. Index JSONB columns with GIN where label-style query patterns
exist (see `AlertEvent.labels` and `AlertSourceProfile.labels` for current
examples).

## Retention

Raw evidence retention, report retention, and chat retention require
explicit operator configuration before public release.
