// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// NotificationChannelProfile is operator-managed notification target metadata.
// It stores secret references only; endpoint URLs and credential values live
// outside PostgreSQL and are never returned by OpenAPI.
type NotificationChannelProfile struct {
	ent.Schema
}

// Fields of the NotificationChannelProfile.
func (NotificationChannelProfile) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(120).
			NotEmpty().
			Unique().
			Comment("operator-facing unique display name"),
		field.String("kind").
			MaxLen(32).
			Default("webhook").
			Comment(`"webhook", "wecom", "dingtalk", or "feishu"`),
		field.String("secret_ref").
			MaxLen(256).
			NotEmpty().
			Comment("deployment-managed endpoint secret reference, never the endpoint or credential value"),
		field.JSON("delivery_scopes", []string{}).
			Comment(`enabled delivery scopes such as "report" and "diagnosis_close"`),
		field.Bool("enabled").
			Default(false).
			Comment("whether operators enabled this channel for future notification binding"),
		field.JSON("labels", map[string]string{}).
			Comment("operator labels for ownership, environment, or routing metadata"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side channel creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Edges of the NotificationChannelProfile.
func (NotificationChannelProfile) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("test_proofs", NotificationChannelTestProof.Type),
	}
}

// Indexes of the NotificationChannelProfile.
func (NotificationChannelProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kind", "enabled"),
		index.Fields("delivery_scopes").
			Annotations(
				entsql.IndexTypes(map[string]string{
					dialect.Postgres: "GIN",
				}),
			),
		index.Fields("labels").
			Annotations(
				entsql.IndexTypes(map[string]string{
					dialect.Postgres: "GIN",
				}),
			),
	}
}
