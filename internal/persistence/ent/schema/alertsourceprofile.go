// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AlertSourceProfile is operator-managed connection metadata for alert source
// adapters. It stores secret references only; credential values live outside
// PostgreSQL and are never returned by OpenAPI.
type AlertSourceProfile struct {
	ent.Schema
}

// Mixin of the AlertSourceProfile.
func (AlertSourceProfile) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the AlertSourceProfile.
func (AlertSourceProfile) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(120).
			NotEmpty().
			Comment("operator-facing unique display name"),
		field.String("kind").
			MaxLen(32).
			NotEmpty().
			Comment(`"prometheus" | "alertmanager"`),
		field.String("base_url").
			MaxLen(2048).
			NotEmpty().
			Comment("http(s) base URL without userinfo, query, or fragment"),
		field.String("auth_mode").
			MaxLen(32).
			Default("none").
			Comment(`"none" | "bearer"; secret material is represented only by secret_ref`),
		field.String("secret_ref").
			MaxLen(256).
			Optional().
			Comment("deployment-managed secret reference, never the secret value"),
		field.Bool("enabled").
			Default(false).
			Comment("whether operators have enabled this source for policy binding"),
		field.JSON("labels", map[string]string{}).
			Comment("operator labels for ownership, environment, or routing metadata"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side profile creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Indexes of the AlertSourceProfile.
func (AlertSourceProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "name").Unique(),
		index.Fields("kind", "enabled"),
		index.Fields("labels").
			Annotations(
				entsql.IndexTypes(map[string]string{
					dialect.Postgres: "GIN",
				}),
			),
	}
}
