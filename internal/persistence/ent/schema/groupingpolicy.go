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

// GroupingPolicy is operator-managed alert grouping configuration.
type GroupingPolicy struct {
	ent.Schema
}

// Mixin of the GroupingPolicy.
func (GroupingPolicy) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the GroupingPolicy.
func (GroupingPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(120).
			NotEmpty().
			Comment("operator-facing unique display name"),
		field.JSON("dimension_keys", []string{}).
			Comment("alert label keys used as deterministic grouping dimensions"),
		field.String("severity_key").
			MaxLen(64).
			NotEmpty().
			Comment("alert label key used to compute group severity"),
		field.JSON("source_filter", []string{}).
			Comment("optional alert source identifiers; empty means all sources"),
		field.Bool("enabled").
			Default(false).
			Comment("whether operators enabled this grouping policy for binding"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side policy creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Indexes of the GroupingPolicy.
func (GroupingPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "name").Unique(),
		index.Fields("enabled", "updated_at"),
		index.Fields("source_filter").
			Annotations(
				entsql.IndexTypes(map[string]string{
					dialect.Postgres: "GIN",
				}),
			),
	}
}
