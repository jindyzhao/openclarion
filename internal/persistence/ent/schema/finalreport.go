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

// FinalReport is the incident-level reduction of validated SubReports.
// It is persisted before notification delivery starts so report
// queryability is decoupled from IM/Webhook success.
//
// Idempotency contract:
//
//	idempotency_key is unique within one tenant for a final-report generation
//	attempt. A Temporal retry that re-runs the same reduce step must
//	collapse to the already-persisted FinalReport row.
type FinalReport struct {
	ent.Schema
}

// Mixin of the FinalReport.
func (FinalReport) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the FinalReport.
func (FinalReport) Fields() []ent.Field {
	return []ent.Field{
		field.String("correlation_key").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("business correlation key for the reduced incident/window"),
		field.String("idempotency_key").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("activity idempotency key for final report generation"),
		field.String("title").
			MaxLen(160).
			NotEmpty().
			Immutable().
			Comment("operator-facing title extracted from validated JSON"),
		field.String("executive_summary").
			MaxLen(4000).
			NotEmpty().
			Immutable().
			Comment("operator-facing executive summary extracted from validated JSON"),
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
		field.String("generation_status").
			MaxLen(16).
			Default("complete").
			Immutable().
			Comment(`fan-in coverage: "complete" | "partial"`),
		field.Int("expected_sub_report_count").
			Positive().
			Default(1).
			Immutable().
			Comment("number of SubReports expected before applying the failure threshold"),
		field.Int("successful_sub_report_count").
			Positive().
			Default(1).
			Immutable().
			Comment("number of generated SubReports included in fan-in"),
		field.Int("failed_sub_report_count").
			NonNegative().
			Default(0).
			Immutable().
			Comment("number of failed SubReports omitted under the configured threshold"),
		field.JSON("subreport_summaries", json.RawMessage{}).
			Immutable().
			Comment("validated sub_reports projection from the FinalReport JSON"),
		field.JSON("recommended_actions", json.RawMessage{}).
			Immutable().
			Comment("validated recommended_actions array from the FinalReport JSON"),
		field.String("notification_text").
			MaxLen(2000).
			NotEmpty().
			Immutable().
			Comment("operator-facing notification body to send after persistence succeeds"),
		field.JSON("content", json.RawMessage{}).
			Immutable().
			Comment("full accepted FinalReport JSON payload"),
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
			Comment("temporal workflow id that produced this final report"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side report persistence timestamp"),
	}
}

// Edges of the FinalReport.
func (FinalReport) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("sub_reports", SubReport.Type),
		edge.To("notification_deliveries", ReportNotificationDelivery.Type),
	}
}

// Indexes of the FinalReport.
func (FinalReport) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "idempotency_key").Unique(),
		index.Fields("correlation_key", "created_at"),
		index.Fields("severity", "created_at"),
	}
}
