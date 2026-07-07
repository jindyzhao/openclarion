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

// ReportWorkflowSchedule is operator-managed schedule configuration for one
// report workflow policy.
type ReportWorkflowSchedule struct {
	ent.Schema
}

// Fields of the ReportWorkflowSchedule.
func (ReportWorkflowSchedule) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(120).
			NotEmpty().
			Unique().
			Comment("operator-facing unique display name"),
		field.Int("report_workflow_policy_id").
			Positive().
			Comment("bound ReportWorkflowPolicy identifier"),
		field.String("temporal_schedule_id").
			MaxLen(200).
			NotEmpty().
			Unique().
			Comment("server-owned Temporal Schedule identifier"),
		field.String("cadence").
			MaxLen(32).
			Default("interval").
			Comment(`"interval" | "daily" | "weekly" | "monthly"; text, not a db enum`),
		field.Int("calendar_hour").
			NonNegative().
			Default(0).
			Comment("UTC hour for calendar cadences; 0-23 at application layer"),
		field.Int("calendar_minute").
			NonNegative().
			Default(0).
			Comment("UTC minute for calendar cadences; 0-59 at application layer"),
		field.Int("calendar_day_of_week").
			NonNegative().
			Default(0).
			Comment("UTC day of week for weekly cadence; 0=Sunday through 6=Saturday"),
		field.Int("calendar_day_of_month").
			NonNegative().
			Default(0).
			Comment("UTC day of month for monthly cadence; 1-28 when cadence=monthly"),
		field.Int64("interval_ns").
			Positive().
			Comment("Temporal Schedule interval duration in nanoseconds, or replay guard interval for calendar cadences"),
		field.Int64("offset_ns").
			NonNegative().
			Comment("Temporal Schedule interval offset duration in nanoseconds"),
		field.Int64("replay_window_ns").
			Positive().
			Comment("alert replay lookback window duration in nanoseconds"),
		field.Int64("replay_delay_ns").
			NonNegative().
			Comment("delay subtracted from fire time before replay window end"),
		field.Int("replay_limit").
			Positive().
			Comment("maximum alert events loaded for each scheduled replay"),
		field.Int64("catchup_window_ns").
			Positive().
			Comment("Temporal Schedule catch-up window duration in nanoseconds"),
		field.Bool("enabled").
			Default(false).
			Comment("whether operators explicitly enabled this schedule"),
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
			Comment("server-side schedule creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Indexes of the ReportWorkflowSchedule.
func (ReportWorkflowSchedule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "updated_at"),
		index.Fields("report_workflow_policy_id", "enabled"),
	}
}
