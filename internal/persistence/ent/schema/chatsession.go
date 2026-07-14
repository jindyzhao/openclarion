// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChatSession is the M5 short-conversation diagnosis-room lifecycle record.
//
// It is deliberately anchored to DiagnosisTask rather than a generic signal
// table: the product direction remains intelligent alert diagnosis, while the
// external session_key gives the WebSocket/auth layer a stable reconnect id.
//
// Lifecycle discipline:
//
//	status is text ("open" | "closed"), not a database enum. Closing a room
//	stamps closed_at and close_reason; application code treats closed rows as
//	terminal.
//
// Identity discipline:
//
//	session_key is the external room id used by browser WebSocket tickets.
//	diagnosis_task_id is UNIQUE in V1 because one DiagnosisRoomWorkflow
//	execution owns one chat session.
type ChatSession struct {
	ent.Schema
}

// Fields of the ChatSession.
func (ChatSession) Fields() []ent.Field {
	return []ent.Field{
		field.Int("diagnosis_task_id").
			Immutable().
			Comment("FK to diagnosis_tasks.id; one diagnosis workflow execution owns one chat session"),
		field.String("session_key").
			MaxLen(128).
			NotEmpty().
			Unique().
			Immutable().
			Comment("external diagnosis-room session id used by WebSocket auth and reconnect flows"),
		field.String("owner_subject").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("stable authenticated subject that owns this diagnosis room; admins may access across owners"),
		field.String("status").
			MaxLen(32).
			Default("open").
			Comment(`"open" | "closed"; text, not a db enum`),
		field.Int("turn_count").
			NonNegative().
			Default(0).
			Comment("accepted user-turn count tracked by the workflow for policy checks"),
		field.Time("started_at").
			Immutable().
			Comment("UTC, microsecond-truncated session start time"),
		field.Time("last_activity_at").
			Comment("UTC, microsecond-truncated last accepted turn or close time"),
		field.Time("closed_at").
			Optional().
			Nillable().
			Comment("UTC, microsecond-truncated terminal close time"),
		field.String("close_reason").
			MaxLen(128).
			Optional().
			Comment("workflow/user/system reason that closed the room"),
		field.String("approval_mode").
			MaxLen(32).
			Default("single").
			Immutable().
			Comment(`human conclusion quorum: "single" or "owner_and_leader"`),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side session row creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Edges of the ChatSession.
func (ChatSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("task", DiagnosisTask.Type).
			Ref("chat_sessions").
			Field("diagnosis_task_id").
			Unique().
			Required().
			Immutable(),
		edge.To("turns", ChatTurn.Type),
		edge.To("summaries", ChatSessionSummary.Type),
		edge.To("approvals", ChatSessionApproval.Type),
	}
}

// Indexes of the ChatSession.
func (ChatSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("diagnosis_task_id").
			Unique(),
		index.Fields("owner_subject", "status"),
		index.Fields("status", "last_activity_at"),
		index.Fields("started_at"),
	}
}
