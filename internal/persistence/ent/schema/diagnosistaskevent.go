// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DiagnosisTaskEvent is the append-only lifecycle log for a
// DiagnosisTask. Per the M1 persistence and CI-gate modeling decision
// (see docs/design/database/schema-catalog.md DiagnosisTaskEvent block),
// events live in their own small table rather than a jsonb array on
// DiagnosisTask: this preserves index-friendly queries, lets the
// kind vocabulary grow without schema migrations, and supports
// idempotent producers via dedupe_key.
//
// Idempotency contract:
//
//	dedupe_key is text NULL with UNIQUE(task_id, dedupe_key). Postgres
//	allows multiple NULL values in a UNIQUE index, so producers that
//	don't need idempotency can omit the key and still record events;
//	producers that DO (e.g. Temporal Activity retries) supply a stable
//	key and the second insert is rejected at the database boundary.
//
// Time discipline:
//
//	occurred_at is the wall-clock when the event happened (UTC,
//	microsecond-truncated). recorded_at is the server-side ingestion
//	timestamp; the two MAY differ when events are buffered.
//
// Mutability:
//
//	rows are append-only. Updating an event is a bug; producers that
//	need to express "X superseded Y" should write a new event with the
//	relation in payload.
type DiagnosisTaskEvent struct {
	ent.Schema
}

// Mixin of the DiagnosisTaskEvent.
func (DiagnosisTaskEvent) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the DiagnosisTaskEvent.
func (DiagnosisTaskEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int("task_id").
			Immutable().
			Comment("FK to diagnosis_tasks.id"),
		field.String("kind").
			MaxLen(64).
			NotEmpty().
			Immutable().
			Comment(`free-text event kind, e.g. "task.started", "subreport.failed"; NOT a db enum`),
		field.JSON("payload", json.RawMessage{}).
			Optional().
			Comment("jsonb event detail; immutable once persisted (append-only)"),
		field.String("dedupe_key").
			MaxLen(128).
			Optional().
			Nillable().
			Comment("optional idempotency key; UNIQUE per task. Multiple NULLs are allowed (Postgres semantics)"),
		field.Time("occurred_at").
			Immutable().
			Comment("UTC, microsecond-truncated; producer-supplied wall-clock"),
		field.Time("recorded_at").
			Default(time.Now).
			Immutable().
			Comment("server-side ingestion timestamp"),
	}
}

// Edges of the DiagnosisTaskEvent.
func (DiagnosisTaskEvent) Edges() []ent.Edge {
	return []ent.Edge{
		// Required FK: every event belongs to exactly one task.
		edge.From("task", DiagnosisTask.Type).
			Ref("events").
			Field("task_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the DiagnosisTaskEvent.
func (DiagnosisTaskEvent) Indexes() []ent.Index {
	return []ent.Index{
		// Idempotency boundary: one row per (task, dedupe_key) when
		// dedupe_key is supplied. NULLs are allowed to repeat, which is
		// the standard Postgres UNIQUE NULL semantics and exactly the
		// behaviour we want for non-idempotent producers.
		index.Fields("tenant_id", "task_id", "dedupe_key").
			Unique(),
		// Hot read path: list events for a task, oldest-first. FK
		// column first so the index prefix serves "events WHERE
		// task_id = ? ORDER BY occurred_at".
		index.Fields("task_id", "occurred_at"),
	}
}
