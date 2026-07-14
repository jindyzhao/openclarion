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

// ChatSessionApproval is an immutable stakeholder approval bound to one exact
// conclusion digest. A later diagnosis conclusion writes new rows and leaves
// prior approvals available for audit.
type ChatSessionApproval struct {
	ent.Schema
}

// Fields of the ChatSessionApproval.
func (ChatSessionApproval) Fields() []ent.Field {
	return []ent.Field{
		field.Int("chat_session_id").
			Immutable().
			Comment("FK to chat_sessions.id"),
		field.String("conclusion_digest").
			MinLen(64).
			MaxLen(64).
			Immutable().
			Comment("lowercase SHA-256 of assistant message id, sequence, and content"),
		field.String("actor_subject").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("authenticated subject that approved the conclusion"),
		field.String("authority").
			MaxLen(32).
			NotEmpty().
			Immutable().
			Comment(`stakeholder capacity: "owner" or "leader"`),
		field.String("reason").
			MaxLen(512).
			NotEmpty().
			Immutable().
			Comment("single-line operator-supplied approval reason"),
		field.Time("approved_at").
			Immutable().
			Comment("UTC, microsecond-truncated workflow approval time"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side approval row creation timestamp"),
	}
}

// Edges of the ChatSessionApproval.
func (ChatSessionApproval) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("session", ChatSession.Type).
			Ref("approvals").
			Field("chat_session_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the ChatSessionApproval.
func (ChatSessionApproval) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("chat_session_id", "conclusion_digest", "actor_subject").
			Unique(),
		index.Fields("chat_session_id", "conclusion_digest", "authority").
			Unique(),
		index.Fields("chat_session_id", "conclusion_digest", "approved_at"),
		index.Fields("actor_subject", "approved_at"),
	}
}
