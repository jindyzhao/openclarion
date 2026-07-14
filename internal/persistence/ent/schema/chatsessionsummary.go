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

// ChatSessionSummary is an append-only conversation compression checkpoint.
// Source turn rows remain immutable and complete; this table stores a bounded
// read model plus a digest binding it to the exact ordered source transcript.
// A per-session version supports future periodic checkpoints without mutating
// lifecycle-end summaries.
type ChatSessionSummary struct {
	ent.Schema
}

// Fields of the ChatSessionSummary.
func (ChatSessionSummary) Fields() []ent.Field {
	return []ent.Field{
		field.Int("chat_session_id").
			Immutable().
			Comment("FK to chat_sessions.id"),
		field.Int("version").
			Positive().
			Immutable().
			Comment("monotonic summary revision; UNIQUE per chat session"),
		field.String("schema_version").
			MaxLen(64).
			NotEmpty().
			Immutable().
			Comment("versioned contract for the JSON summary content"),
		field.Int("source_first_sequence").
			NonNegative().
			Immutable().
			Comment("first included chat_turn sequence, or zero for an empty transcript"),
		field.Int("source_last_sequence").
			NonNegative().
			Immutable().
			Comment("last included chat_turn sequence, or zero for an empty transcript"),
		field.Int("source_turn_count").
			NonNegative().
			Immutable().
			Comment("number of immutable chat_turn rows bound by this summary"),
		field.String("source_digest").
			MinLen(64).
			MaxLen(64).
			Immutable().
			Comment("lowercase SHA-256 digest of canonical ordered source turns"),
		field.JSON("content", json.RawMessage{}).
			Immutable().
			Comment("bounded provider-neutral structured conversation summary"),
		field.Time("generated_at").
			Immutable().
			Comment("UTC, microsecond-truncated producer-supplied summary time"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side summary row creation timestamp"),
	}
}

// Edges of the ChatSessionSummary.
func (ChatSessionSummary) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("session", ChatSession.Type).
			Ref("summaries").
			Field("chat_session_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the ChatSessionSummary.
func (ChatSessionSummary) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("chat_session_id", "version").
			Unique(),
		index.Fields("chat_session_id", "source_digest").
			Unique(),
		index.Fields("chat_session_id", "generated_at"),
	}
}
