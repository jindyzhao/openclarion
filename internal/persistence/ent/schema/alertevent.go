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

// AlertEvent is a single alert event ingested from an upstream metrics
// provider (Alertmanager, Datadog, etc.).
//
// Fingerprint discipline (per M1 design decision, two-track):
//   - source_fingerprint     : the fingerprint reported by the upstream
//     provider, retained verbatim for traceability.
//   - canonical_fingerprint  : sha256 hex of `canonical(sorted(labels))`,
//     computed in-process so that re-ingestion of the same logical alert
//     from the same source always collapses to the same row.
//
// Time discipline:
//
//	starts_at MUST be normalised to UTC().Truncate(time.Microsecond)
//	BEFORE the canonical fingerprint is computed and BEFORE the row is
//	persisted. This is enforced by the ingestion path, not by the
//	database, because PostgreSQL `timestamptz` accepts microsecond
//	resolution but the comparison rules at the application layer must
//	be deterministic regardless of the upstream clock format.
//
// Natural unique key:
//
//	(source, canonical_fingerprint, starts_at)
//
// Re-ingestion of an identical event is a no-op (Postgres unique-violation
// is converted to "already-known" at the persistence layer); status
// transitions to "resolved" are an UPDATE, not a new row.
type AlertEvent struct {
	ent.Schema
}

// Fields of the AlertEvent.
func (AlertEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("source").
			MaxLen(64).
			NotEmpty().
			Immutable().
			Comment(`upstream provider identifier, e.g. "alertmanager", "datadog"`),
		field.String("source_fingerprint").
			MaxLen(128).
			NotEmpty().
			Immutable().
			Comment("fingerprint reported by the upstream provider, retained verbatim"),
		field.String("canonical_fingerprint").
			MaxLen(64).
			NotEmpty().
			Immutable().
			Comment("sha256 hex of canonical(sorted(labels)); computed in-process"),
		field.JSON("labels", map[string]string{}).
			Comment("alert labels, canonicalised before fingerprinting"),
		field.JSON("annotations", map[string]string{}).
			Comment("alert annotations (free-form descriptive metadata)"),
		field.JSON("raw_payload", json.RawMessage{}).
			Optional().
			Comment("raw upstream payload as JSONB; retained for forensic debugging only (per JSONB Usage rule in schema-catalog.md)"),
		field.String("status").
			MaxLen(32).
			Default("firing").
			Comment(`"firing" | "resolved"`),
		field.Time("starts_at").
			Immutable().
			Comment("UTC, microsecond-truncated; part of the natural unique key"),
		field.Time("ends_at").
			Optional().
			Nillable().
			Comment("set when status transitions to resolved"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side ingestion timestamp"),
	}
}

// Edges of the AlertEvent.
func (AlertEvent) Edges() []ent.Edge {
	return []ent.Edge{
		// Many-to-many: an AlertEvent MAY appear in multiple AlertGroups
		// when several grouping configurations run concurrently. The
		// owning side is declared on AlertGroup.
		edge.To("groups", AlertGroup.Type),
	}
}

// Indexes of the AlertEvent.
func (AlertEvent) Indexes() []ent.Index {
	return []ent.Index{
		// Natural unique key: dedupe identical events on re-ingestion.
		index.Fields("source", "canonical_fingerprint", "starts_at").
			Unique(),
		// Hot read path: list firing alerts for a given source.
		index.Fields("source", "status"),
		// Label-based queries (e.g. "all firing alerts where service=foo")
		// are served by a GIN index over the labels jsonb column.
		index.Fields("labels").
			Annotations(
				entsql.IndexTypes(map[string]string{
					dialect.Postgres: "GIN",
				}),
			),
	}
}
