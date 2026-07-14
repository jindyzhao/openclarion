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

// ReportNotificationDelivery is the durable delivery log for a
// FinalReport notification idempotency key.
//
// Idempotency contract:
//
//	idempotency_key is unique within one tenant. Activity retries must update the
//	same row instead of appending duplicate delivery records.
//
// Status discipline:
//
//	status is text, not a database enum. Allowed values at M2 are
//	"pending" | "delivered" | "failed"; validation lives in the domain.
type ReportNotificationDelivery struct {
	ent.Schema
}

// Mixin of the ReportNotificationDelivery.
func (ReportNotificationDelivery) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the ReportNotificationDelivery.
func (ReportNotificationDelivery) Fields() []ent.Field {
	return []ent.Field{
		field.Int("final_report_id").
			Immutable().
			Comment("FK to final_reports.id; the report this delivery belongs to"),
		field.Int("report_notification_channel_profile_id").
			Optional().
			Nillable().
			Comment("optional notification channel profile used for this report notification attempt"),
		field.String("idempotency_key").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("tenant-scoped notification idempotency key"),
		field.String("provider_message_id").
			MaxLen(256).
			Optional().
			Comment("stable provider message identifier, when supplied"),
		field.String("provider_status").
			MaxLen(64).
			Optional().
			Comment("provider-reported success status, when supplied"),
		field.String("status").
			MaxLen(32).
			Default("pending").
			Comment(`"pending" | "delivered" | "failed"; text, not a db enum`),
		field.JSON("raw", json.RawMessage{}).
			Comment("provider success payload or failure detail JSON"),
		field.String("failure_reason").
			MaxLen(2000).
			Optional().
			Comment("short failure reason populated when status=failed"),
		field.Time("delivered_at").
			Optional().
			Nillable().
			Comment("UTC, microsecond-truncated; set when status=delivered"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side delivery row creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Edges of the ReportNotificationDelivery.
func (ReportNotificationDelivery) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("final_report", FinalReport.Type).
			Ref("notification_deliveries").
			Field("final_report_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the ReportNotificationDelivery.
func (ReportNotificationDelivery) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "idempotency_key").Unique(),
		index.Fields("final_report_id", "created_at"),
		index.Fields("status", "updated_at"),
	}
}
