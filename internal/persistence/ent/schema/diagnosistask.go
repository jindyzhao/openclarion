// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DiagnosisTask is the workflow-bound lifecycle record for one
// DiagnosisWorkflow execution against one EvidenceSnapshot
// (see docs/design/phases/03-workflows.md).
//
// Identity discipline (matches Temporal's own identity model):
//
//	The natural unique key is (workflow_id, run_id). workflow_id is the
//	business key shared across the chain of executions for a logical
//	workflow; run_id is the per-execution identity assigned by Temporal
//	when an execution starts. Temporal retries that spawn a new run_id
//	(continue-as-new, reset, or scheduled retry) are persisted as NEW
//	DiagnosisTask rows, NOT as updates to an existing row. This
//	preserves the per-execution audit trail. workflow_id alone has a
//	non-unique index so queries can list the full chain of executions.
//
// Status discipline:
//
//	status is text (NOT a database enum) so adding new lifecycle states
//	does not require a schema migration. Allowed values at M1:
//	"pending" | "running" | "succeeded" | "failed" | "cancelled".
//	Validation lives at the application layer.
//
// Time discipline:
//
//	started_at / finished_at MUST be UTC().Truncate(time.Microsecond);
//	finished_at is non-nil iff status is a terminal state.
type DiagnosisTask struct {
	ent.Schema
}

// Fields of the DiagnosisTask.
func (DiagnosisTask) Fields() []ent.Field {
	return []ent.Field{
		field.Int("evidence_snapshot_id").
			Immutable().
			Comment("FK to evidence_snapshots.id; the evidence this task processes"),
		field.String("workflow_id").
			MaxLen(128).
			NotEmpty().
			Immutable().
			Comment("temporal workflow id (business key); chain of executions shares this id; identity is (workflow_id, run_id)"),
		field.String("run_id").
			MaxLen(128).
			NotEmpty().
			Immutable().
			Comment("temporal run id; immutable. A new run_id (retry, continue-as-new, reset) yields a NEW DiagnosisTask row"),
		field.String("status").
			MaxLen(32).
			Default("pending").
			Comment(`"pending" | "running" | "succeeded" | "failed" | "cancelled"; text, not a db enum`),
		field.String("failure_reason").
			MaxLen(1024).
			Optional().
			Comment("short human-readable reason; populated when status=failed"),
		field.Time("started_at").
			Optional().
			Nillable().
			Comment("UTC, microsecond-truncated; set on first transition to running"),
		field.Time("finished_at").
			Optional().
			Nillable().
			Comment("UTC, microsecond-truncated; non-nil iff status is terminal"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side task creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Edges of the DiagnosisTask.
func (DiagnosisTask) Edges() []ent.Edge {
	return []ent.Edge{
		// Required FK: every task drives one EvidenceSnapshot.
		edge.From("snapshot", EvidenceSnapshot.Type).
			Ref("tasks").
			Field("evidence_snapshot_id").
			Unique().
			Required().
			Immutable(),
		// Append-only lifecycle log. Tasks are the durability anchor;
		// events are derivative.
		edge.To("events", DiagnosisTaskEvent.Type),
	}
}

// Indexes of the DiagnosisTask.
func (DiagnosisTask) Indexes() []ent.Index {
	return []ent.Index{
		// Natural execution identity: (workflow_id, run_id). Temporal
		// retries that produce a new run_id are NEW rows.
		index.Fields("workflow_id", "run_id").
			Unique(),
		// Business view: list every execution in a workflow chain.
		index.Fields("workflow_id"),
		// Hot read path: list non-terminal tasks, oldest-first, for
		// orphan recovery and operator dashboards.
		index.Fields("status", "created_at"),
	}
}
