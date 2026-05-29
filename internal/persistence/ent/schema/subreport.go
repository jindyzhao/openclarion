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

// SubReport is one schema-validated AI report for a single
// EvidenceSnapshot. It keeps the accepted JSON content for audit/replay
// and duplicates selected fields into typed columns for report list and
// detail read paths.
//
// Idempotency contract:
//
//	(evidence_snapshot_id, idempotency_key) is unique. Temporal Activity
//	retries that regenerate the same subreport for the same snapshot must
//	collapse at the persistence boundary.
type SubReport struct {
	ent.Schema
}

// Fields of the SubReport.
func (SubReport) Fields() []ent.Field {
	return []ent.Field{
		field.Int("evidence_snapshot_id").
			Immutable().
			Comment("FK to evidence_snapshots.id; the snapshot this subreport explains"),
		field.String("idempotency_key").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("activity idempotency key; UNIQUE per evidence_snapshot_id"),
		field.String("scenario").
			MaxLen(64).
			NotEmpty().
			Immutable().
			Comment(`prompt scenario: "single_alert" | "cascade" | "alert_storm"`),
		field.String("title").
			MaxLen(160).
			NotEmpty().
			Immutable().
			Comment("operator-facing title extracted from validated JSON"),
		field.String("summary").
			MaxLen(4000).
			NotEmpty().
			Immutable().
			Comment("operator-facing summary extracted from validated JSON"),
		field.String("severity").
			MaxLen(32).
			NotEmpty().
			Immutable().
			Comment(`"info" | "warning" | "critical"`),
		field.String("confidence").
			MaxLen(32).
			NotEmpty().
			Immutable().
			Comment(`"low" | "medium" | "high"`),
		field.JSON("findings", json.RawMessage{}).
			Immutable().
			Comment("validated findings array from the SubReport JSON"),
		field.JSON("recommended_actions", json.RawMessage{}).
			Immutable().
			Comment("validated recommended_actions array from the SubReport JSON"),
		field.JSON("evidence_refs", []string{}).
			Immutable().
			Comment("evidence identifiers referenced by the SubReport"),
		field.JSON("content", json.RawMessage{}).
			Immutable().
			Comment("full accepted SubReport JSON payload"),
		field.String("model").
			MaxLen(128).
			Optional().
			Immutable().
			Comment("provider model that produced the accepted output"),
		field.String("output_mode").
			MaxLen(32).
			Optional().
			Immutable().
			Comment(`provider output mode: "json_schema" | "json_object"`),
		field.String("created_by_workflow").
			MaxLen(128).
			Optional().
			Immutable().
			Comment("temporal workflow id that produced this subreport"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side report persistence timestamp"),
	}
}

// Edges of the SubReport.
func (SubReport) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("snapshot", EvidenceSnapshot.Type).
			Ref("sub_reports").
			Field("evidence_snapshot_id").
			Unique().
			Required().
			Immutable(),
		edge.From("final_reports", FinalReport.Type).
			Ref("sub_reports"),
	}
}

// Indexes of the SubReport.
func (SubReport) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("evidence_snapshot_id", "idempotency_key").
			Unique(),
		index.Fields("evidence_snapshot_id", "created_at"),
		index.Fields("severity", "created_at"),
	}
}
