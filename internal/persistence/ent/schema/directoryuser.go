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

// DirectoryUser is OpenClarion's local projection of an upstream directory
// user. Login still comes from OIDC; this table supports room attribution,
// local RBAC, and operator pickers without live directory calls.
type DirectoryUser struct {
	ent.Schema
}

// Mixin of the DirectoryUser.
func (DirectoryUser) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the DirectoryUser.
func (DirectoryUser) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").
			MaxLen(64).
			NotEmpty().
			Comment("upstream directory provider identifier such as ops_iam"),
		field.String("subject").
			MaxLen(256).
			NotEmpty().
			Comment("stable IAM subject used for OpenClarion ownership and audit"),
		field.String("external_id").
			MaxLen(256).
			NotEmpty().
			Comment("provider-stable upstream user identifier"),
		field.String("username").
			MaxLen(128).
			NotEmpty().
			Comment("normalized login or account name"),
		field.String("display_name").
			MaxLen(256).
			NotEmpty().
			Comment("operator-facing user display name"),
		field.String("email").
			MaxLen(320).
			Optional().
			Comment("corporate email when supplied by the upstream directory"),
		field.String("job_title").
			MaxLen(256).
			Optional().
			Comment("job title when supplied by the upstream directory"),
		field.String("department").
			MaxLen(256).
			Optional().
			Comment("first business department segment"),
		field.String("section").
			MaxLen(256).
			Optional().
			Comment("department path below the first department segment"),
		field.String("department_path").
			MaxLen(1024).
			Optional().
			Comment("primary readable department path"),
		field.JSON("department_paths", []string{}).
			Comment("all readable department paths from the upstream directory"),
		field.JSON("department_external_ids", []string{}).
			Comment("provider-stable department identifiers for local RBAC candidates"),
		field.Bool("active").
			Default(true).
			Comment("whether the upstream account is active in the directory projection"),
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

// Indexes of the DirectoryUser.
func (DirectoryUser) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "provider", "subject").
			Unique(),
		index.Fields("tenant_id", "provider", "external_id").
			Unique(),
		index.Fields("provider", "active"),
		index.Fields("display_name"),
	}
}
