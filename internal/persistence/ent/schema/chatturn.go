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

// ChatTurn is an append-only transcript row for M5 interactive diagnosis.
//
// Idempotency discipline:
//
//	message_id is UNIQUE per session. Browser retries and Temporal Update
//	replays must collide here instead of producing duplicate turns.
//
// Ordering discipline:
//
//	sequence is UNIQUE per session and is the canonical transcript order used
//	when reconstructing /workspace/conversation.json for a per-turn sandbox
//	invocation.
//
// Mutability:
//
//	rows are immutable. Corrections or follow-ups are new turns.
type ChatTurn struct {
	ent.Schema
}

// Mixin of the ChatTurn.
func (ChatTurn) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the ChatTurn.
func (ChatTurn) Fields() []ent.Field {
	return []ent.Field{
		field.Int("chat_session_id").
			Immutable().
			Comment("FK to chat_sessions.id"),
		field.String("message_id").
			MaxLen(128).
			NotEmpty().
			Immutable().
			Comment("caller-supplied idempotency key; UNIQUE per chat session"),
		field.Int("sequence").
			Positive().
			Immutable().
			Comment("monotonic transcript sequence; UNIQUE per chat session"),
		field.String("role").
			MaxLen(16).
			NotEmpty().
			Immutable().
			Comment(`"user" | "assistant" | "system" | "tool"; text, not a db enum`),
		field.String("actor_subject").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("authenticated user subject, assistant id, system marker, or tool id responsible for the turn"),
		field.Text("content").
			NotEmpty().
			Immutable().
			Comment("full turn content retained for audit and replay"),
		field.JSON("metadata", json.RawMessage{}).
			Optional().
			Immutable().
			Comment("jsonb metadata such as model, container invocation id, policy decision, or tool data"),
		field.Time("occurred_at").
			Immutable().
			Comment("UTC, microsecond-truncated producer-supplied wall-clock"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side turn row creation timestamp"),
	}
}

// Edges of the ChatTurn.
func (ChatTurn) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("session", ChatSession.Type).
			Ref("turns").
			Field("chat_session_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the ChatTurn.
func (ChatTurn) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "chat_session_id", "message_id").
			Unique(),
		index.Fields("tenant_id", "chat_session_id", "sequence").
			Unique(),
		index.Fields("chat_session_id", "occurred_at"),
		index.Fields("chat_session_id", "created_at"),
	}
}
