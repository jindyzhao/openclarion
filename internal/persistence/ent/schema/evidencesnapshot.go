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

// EvidenceSnapshot is the enriched evidence package produced by Stage S2
// (see docs/design/interaction-flows/master-flow.md). It is the single
// source for all downstream AI analysis: ReportFanOutWorkflow consumes
// the snapshot, never the live providers, so report generation is
// deterministic and replayable.
//
// Idempotency contract (per-group, NOT cross-row global):
//
//	digest = sha256 hex of `canonical(payload)`. The natural unique key
//	is (alert_group_id, digest) -- two different AlertGroups MAY produce
//	snapshots with identical payload bytes (e.g. same labels, same
//	provider responses), and they are legitimately distinct rows. Within
//	a single group, Activity retries that re-enrich the same group with
//	the same provider responses MUST collapse to a single row at the
//	persistence boundary (Postgres unique-violation -> "already known").
//	Persistence is the idempotency boundary, not the workflow.
//
// Provenance discipline:
//
//	provenance is a jsonb map from provider name to its response
//	envelope (status, fetched_at, error). status MUST be one of "ok",
//	"partial", "failed"; the row-level status is the worst of all
//	provider statuses. missing_fields lists the dotted-path keys that
//	were requested but not produced; downstream stages may still
//	proceed but mark report quality.
type EvidenceSnapshot struct {
	ent.Schema
}

// Fields of the EvidenceSnapshot.
func (EvidenceSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int("alert_group_id").
			Immutable().
			Comment("FK to alert_groups.id; the grouping result this snapshot was materialised from"),
		field.String("digest").
			MaxLen(64).
			NotEmpty().
			Immutable().
			Comment("sha256 hex of canonical(payload); per-group idempotency key (UNIQUE per alert_group_id, see Indexes)"),
		field.JSON("payload", json.RawMessage{}).
			Immutable().
			Comment("full evidence jsonb (labels, metrics, topology, runbook excerpts); immutable once persisted"),
		field.JSON("provenance", json.RawMessage{}).
			Comment(`jsonb map provider->{status,fetched_at,error}; status in "ok"|"partial"|"failed"`),
		field.String("status").
			MaxLen(32).
			Default("complete").
			Comment(`row-level status: "complete" | "partial" | "failed"; worst of all provider statuses`),
		field.JSON("missing_fields", []string{}).
			Optional().
			Comment("dotted-path keys requested but not produced; populated when status=partial"),
		field.String("created_by_workflow").
			MaxLen(128).
			Optional().
			Comment("temporal workflow id that materialised this snapshot; nullable for manually-seeded test rows"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side materialisation timestamp"),
	}
}

// Edges of the EvidenceSnapshot.
func (EvidenceSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		// Required FK: every snapshot is materialised from exactly one
		// AlertGroup. The grouping result is the input contract.
		edge.From("group", AlertGroup.Type).
			Ref("snapshots").
			Field("alert_group_id").
			Unique().
			Required().
			Immutable(),
		// One snapshot may drive zero-or-more DiagnosisTasks (one per
		// workflow attempt; retries with a fresh workflow id are
		// additional rows, not updates).
		edge.To("tasks", DiagnosisTask.Type),
	}
}

// Indexes of the EvidenceSnapshot.
func (EvidenceSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		// Per-group idempotency boundary: Activity retries within the
		// same group with the same canonical payload collapse to one
		// row. Two DIFFERENT groups MAY produce identical payload bytes
		// and are legitimately distinct rows -- digest is NOT globally
		// unique.
		index.Fields("alert_group_id", "digest").
			Unique(),
		// Read path: latest snapshot for a given group. FK column
		// first so the index prefix serves "snapshots WHERE
		// alert_group_id = ? ORDER BY created_at".
		index.Fields("alert_group_id", "created_at"),
	}
}
