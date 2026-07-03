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

// ReportWorkflowPolicy is operator-managed report workflow configuration.
type ReportWorkflowPolicy struct {
	ent.Schema
}

// Fields of the ReportWorkflowPolicy.
func (ReportWorkflowPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(120).
			NotEmpty().
			Unique().
			Comment("operator-facing unique display name"),
		field.Int("alert_source_profile_id").
			Positive().
			Comment("bound AlertSourceProfile identifier"),
		field.Int("grouping_policy_id").
			Positive().
			Comment("bound GroupingPolicy identifier"),
		field.Int("report_notification_channel_profile_id").
			Positive().
			Optional().
			Nillable().
			Comment("optional bound NotificationChannelProfile identifier for final report delivery"),
		field.String("trigger_mode").
			MaxLen(32).
			Default("manual_replay").
			Comment(`"manual_replay"`),
		field.String("report_scenario").
			MaxLen(32).
			Default("single_alert").
			Comment(`"single_alert" | "cascade" | "alert_storm"`),
		field.String("diagnosis_follow_up").
			MaxLen(32).
			Default("disabled").
			Comment(`"disabled" | "suggest_room" | "auto_room"`),
		field.Bool("enabled").
			Default(false).
			Comment("whether operators explicitly enabled this policy for report workflow binding"),
		field.Time("enabled_at").
			Optional().
			Nillable().
			Comment("time of the latest explicit enable action"),
		field.Time("disabled_at").
			Optional().
			Nillable().
			Comment("time of the latest explicit disable action"),
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

// Indexes of the ReportWorkflowPolicy.
func (ReportWorkflowPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "updated_at"),
		index.Fields("alert_source_profile_id", "grouping_policy_id"),
		index.Fields("report_notification_channel_profile_id"),
	}
}
