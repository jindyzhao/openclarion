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

// DirectoryDepartment is OpenClarion's local projection of an upstream
// directory department. The upstream directory remains the source of truth.
type DirectoryDepartment struct {
	ent.Schema
}

// Mixin of the DirectoryDepartment.
func (DirectoryDepartment) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the DirectoryDepartment.
func (DirectoryDepartment) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").
			MaxLen(64).
			NotEmpty().
			Comment("upstream directory provider identifier such as ops_iam"),
		field.String("external_id").
			MaxLen(256).
			NotEmpty().
			Comment("provider-stable department identifier"),
		field.String("parent_external_id").
			MaxLen(256).
			Optional().
			Comment("provider-stable parent department identifier when available"),
		field.String("name").
			MaxLen(256).
			NotEmpty().
			Comment("normalized department name"),
		field.String("display_name").
			MaxLen(256).
			NotEmpty().
			Comment("operator-facing department display name"),
		field.String("path").
			MaxLen(1024).
			NotEmpty().
			Comment("normalized readable department path"),
		field.String("parent_path").
			MaxLen(1024).
			Optional().
			Comment("normalized readable parent department path when available"),
		field.Int("level").
			NonNegative().
			Default(0).
			Comment("department path depth from the upstream projection"),
		field.String("source").
			MaxLen(64).
			Optional().
			Comment("upstream source family, for example wecom"),
		field.Int("member_count").
			NonNegative().
			Default(0).
			Comment("upstream reported member count for this department projection"),
		field.Time("source_updated_at").
			Optional().
			Nillable().
			Comment("upstream record update timestamp when supplied"),
		field.Time("synced_at").
			Comment("UTC, microsecond-truncated local sync timestamp"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side row creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Indexes of the DirectoryDepartment.
func (DirectoryDepartment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "provider", "external_id").
			Unique(),
		index.Fields("provider", "path"),
		index.Fields("path"),
	}
}
