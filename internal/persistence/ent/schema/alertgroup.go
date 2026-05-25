// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AlertGroup is the deterministic grouping result produced by Stage S1
// (see docs/design/interaction-flows/master-flow.md). One row represents
// the output of one grouping pass over a window of AlertEvents along a
// fixed set of dimensions (e.g. {service, severity}).
//
// Determinism contract:
//
//	group_key MUST be derived from `canonical(dimensions)` so re-running
//	grouping over the same window with the same configuration produces
//	the same key. The natural unique key is (group_key, first_seen_at):
//	the same group recurring in a later window is a NEW row, not an
//	update, because each group instance feeds an independent
//	EvidenceSnapshot fan-out.
//
// Time discipline:
//
//	first_seen_at / last_seen_at MUST be UTC().Truncate(time.Microsecond)
//	(same rule as AlertEvent.starts_at), to keep cross-entity timestamp
//	comparisons exact.
//
// AlertGroup is the source of truth for "which AlertEvents this group
// covers" via a many-to-many edge; one AlertEvent MAY belong to multiple
// groups when several grouping dimensions are configured concurrently.
type AlertGroup struct {
	ent.Schema
}

// Fields of the AlertGroup.
func (AlertGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("group_key").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("sha256 hex (or equivalent) of canonical(dimensions); deterministic per (configuration, window)"),
		field.JSON("dimensions", json.RawMessage{}).
			Comment("jsonb snapshot of the label subset used to group events (e.g. {service:foo, severity:critical})"),
		field.String("severity").
			MaxLen(32).
			Default("unknown").
			Comment(`max severity observed within the group; "critical" | "warning" | "info" | "unknown"`),
		field.Int("event_count").
			NonNegative().
			Default(0).
			Comment("denormalised count of AlertEvents in this group at materialisation time"),
		field.String("status").
			MaxLen(32).
			Default("active").
			Comment(`"active" | "closed"; closed means downstream EvidenceSnapshot has been produced and the group is sealed`),
		field.Time("first_seen_at").
			Immutable().
			Comment("UTC, microsecond-truncated; earliest AlertEvent.starts_at in this group"),
		field.Time("last_seen_at").
			Comment("UTC, microsecond-truncated; latest AlertEvent.starts_at observed; updated as the group accretes"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side materialisation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Edges of the AlertGroup.
func (AlertGroup) Edges() []ent.Edge {
	return []ent.Edge{
		// Many-to-many: a group covers many events; an event MAY appear
		// in multiple groups when several grouping configurations run
		// concurrently. The reverse side is declared on AlertEvent.
		edge.From("events", AlertEvent.Type).
			Ref("groups"),
		// One group materialises into zero-or-more EvidenceSnapshots
		// (re-snapshotting on retry is an additional row, not an
		// update; provenance is preserved).
		edge.To("snapshots", EvidenceSnapshot.Type),
	}
}

// Indexes of the AlertGroup.
func (AlertGroup) Indexes() []ent.Index {
	return []ent.Index{
		// Natural unique key: same group_key recurring in a later
		// window is a new row, not an upsert.
		index.Fields("group_key", "first_seen_at").
			Unique(),
		// Hot read path: list active groups, recent-first.
		index.Fields("status", "last_seen_at"),
		// Dimension-based queries (e.g. "all groups where service=foo")
		// are served by a GIN index over the dimensions jsonb column.
		index.Fields("dimensions").
			Annotations(
				entsql.IndexTypes(map[string]string{
					dialect.Postgres: "GIN",
				}),
			),
	}
}
