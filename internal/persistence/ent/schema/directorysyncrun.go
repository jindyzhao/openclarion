// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DirectorySyncRun is one admitted local directory projection sync run. It
// stores bounded request metadata, aggregate counters, and sanitized failure
// details only.
type DirectorySyncRun struct {
	ent.Schema
}

// Mixin of the DirectorySyncRun.
func (DirectorySyncRun) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the DirectorySyncRun.
func (DirectorySyncRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").
			MaxLen(64).
			NotEmpty().
			Comment("upstream directory provider identifier such as ops_iam"),
		field.Int("page_size").
			Positive().
			Comment("upstream directory page size used for the sync run"),
		field.Time("updated_after").
			Optional().
			Nillable().
			Comment("optional incremental sync lower bound requested by the operator"),
		field.String("status").
			MaxLen(32).
			Default("succeeded").
			Comment(`"succeeded" | "failed"`),
		field.String("failure_code").
			MaxLen(64).
			Default("").
			Comment("stable sanitized failure reason code when status is failed"),
		field.String("failure_message").
			MaxLen(240).
			Default("").
			Comment("operator-facing sanitized failure summary when status is failed"),
		field.Int("department_pages").
			NonNegative().
			Comment("number of department pages read from the upstream provider"),
		field.Int("user_pages").
			NonNegative().
			Comment("number of user pages read from the upstream provider"),
		field.Int("departments_upserted").
			NonNegative().
			Comment("number of local department projection rows upserted"),
		field.Int("users_upserted").
			NonNegative().
			Comment("number of local user projection rows upserted"),
		field.Time("synced_at").
			Comment("UTC, microsecond-truncated sync completion timestamp"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side row creation timestamp"),
	}
}

// Indexes of the DirectorySyncRun.
func (DirectorySyncRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "synced_at"),
		index.Fields("provider", "status", "synced_at"),
		index.Fields("synced_at"),
	}
}
