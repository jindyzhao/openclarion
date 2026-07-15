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

// DiagnosisToolTemplate is operator-managed diagnosis evidence collection
// configuration.
type DiagnosisToolTemplate struct {
	ent.Schema
}

// Mixin of the DiagnosisToolTemplate.
func (DiagnosisToolTemplate) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the DiagnosisToolTemplate.
func (DiagnosisToolTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(120).
			NotEmpty().
			Comment("operator-facing unique display name"),
		field.Int("alert_source_profile_id").
			Positive().
			Comment("bound AlertSourceProfile identifier"),
		field.String("tool").
			MaxLen(32).
			NotEmpty().
			Comment(`"active_alerts" | "metric_query" | "metric_range_query"`),
		field.Text("query_template").
			Optional().
			Comment("operator-reviewed PromQL template for metric tools"),
		field.Int("default_limit").
			Positive().
			Comment("default result limit for one template-backed collection"),
		field.Int64("default_window_ns").
			NonNegative().
			Comment("default range-query window in nanoseconds; zero for non-range tools"),
		field.Int64("max_window_ns").
			NonNegative().
			Comment("maximum range-query window in nanoseconds; zero for non-range tools"),
		field.Int64("default_step_ns").
			NonNegative().
			Comment("default range-query step in nanoseconds; zero for non-range tools"),
		field.Bool("enabled").
			Default(false).
			Comment("whether operators have enabled this template for diagnosis tool execution"),
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
			Comment("server-side template creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Indexes of the DiagnosisToolTemplate.
func (DiagnosisToolTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "name").Unique(),
		index.Fields("enabled", "updated_at"),
		index.Fields("alert_source_profile_id", "tool", "enabled"),
	}
}
