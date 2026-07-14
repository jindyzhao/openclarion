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

// NotificationChannelTestProof stores sanitized notification-channel test
// proof. It never stores endpoint URLs, secret references, raw provider
// responses, or raw provider errors.
type NotificationChannelTestProof struct {
	ent.Schema
}

// Mixin of the NotificationChannelTestProof.
func (NotificationChannelTestProof) Mixin() []ent.Mixin { return tenantMixins() }

// Fields of the NotificationChannelTestProof.
func (NotificationChannelTestProof) Fields() []ent.Field {
	return []ent.Field{
		field.Int("notification_channel_profile_id").
			Immutable().
			Comment("FK to notification_channel_profiles.id; channel tested"),
		field.String("kind").
			MaxLen(32).
			Immutable().
			Comment(`notification channel kind at test time, such as "wecom"`),
		field.String("status").
			MaxLen(32).
			Immutable().
			Comment(`sanitized test status: "success" | "failed" | "unsupported" | "blocked"`),
		field.String("reason_code").
			MaxLen(64).
			Immutable().
			Comment("stable sanitized reason code for the test result"),
		field.String("message").
			MaxLen(240).
			Immutable().
			Comment("operator-facing sanitized message"),
		field.String("content_kind").
			MaxLen(64).
			Optional().
			Immutable().
			Comment("sanitized content sample kind, when provider delivery was attempted"),
		field.String("content_sha256").
			MaxLen(64).
			Optional().
			Immutable().
			Comment("SHA-256 digest of the sanitized test notification body"),
		field.Time("checked_at").
			Immutable().
			Comment("UTC, microsecond-truncated; test completion timestamp"),
		field.String("provider_message_id").
			MaxLen(128).
			Optional().
			Immutable().
			Comment("sanitized provider message id, when supplied"),
		field.String("provider_status").
			MaxLen(64).
			Optional().
			Immutable().
			Comment("sanitized provider status, when supplied"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side proof row creation timestamp"),
	}
}

// Edges of the NotificationChannelTestProof.
func (NotificationChannelTestProof) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("notification_channel_profile", NotificationChannelProfile.Type).
			Ref("test_proofs").
			Field("notification_channel_profile_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the NotificationChannelTestProof.
func (NotificationChannelTestProof) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("notification_channel_profile_id", "checked_at"),
		index.Fields("notification_channel_profile_id", "content_kind", "checked_at"),
	}
}
