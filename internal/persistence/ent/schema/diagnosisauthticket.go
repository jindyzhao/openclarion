// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"fmt"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/openclarion/openclarion/internal/domain"
)

// DiagnosisAuthTicket stores short-lived diagnosis WebSocket ticket metadata.
//
// Security contract:
//
//	token_hash is SHA-256 over the raw ticket token. The raw ticket is returned
//	only at issuance time and is never persisted.
//
// Single-use contract:
//
//	consumed_at is set once by a conditional update that requires the row to be
//	unconsumed and unexpired.
type DiagnosisAuthTicket struct {
	ent.Schema
}

// Fields of the DiagnosisAuthTicket.
func (DiagnosisAuthTicket) Fields() []ent.Field {
	return []ent.Field{
		field.Int("tenant_id").
			Positive().
			Immutable().
			Comment("tenant bound into the authenticated ticket"),
		field.String("tenant_key").
			MaxLen(domain.MaxTenantKeyLength).
			NotEmpty().
			Immutable().
			Validate(func(value string) error {
				normalized, err := domain.NormalizeTenantKey(value)
				if err != nil {
					return err
				}
				if normalized != value {
					return fmt.Errorf("tenant key must already be normalized")
				}
				return nil
			}).
			Comment("stable tenant key bound into the authenticated ticket"),
		field.String("token_hash").
			MaxLen(64).
			NotEmpty().
			Unique().
			Immutable().
			Comment("hex SHA-256 digest of the raw WebSocket ticket token"),
		field.String("subject").
			MaxLen(256).
			NotEmpty().
			Immutable().
			Comment("authenticated principal subject"),
		field.JSON("roles", []string{}).
			Immutable().
			Comment("provider-neutral auth roles captured at ticket issuance"),
		field.String("session_id").
			MaxLen(128).
			NotEmpty().
			Immutable().
			Comment("diagnosis room session id this ticket authorizes"),
		field.String("scope").
			MaxLen(64).
			NotEmpty().
			Immutable().
			Comment("ticket purpose marker, e.g. diagnosis_ws"),
		field.Time("issued_at").
			Immutable().
			Comment("UTC, microsecond-truncated ticket issue time"),
		field.Time("expires_at").
			Immutable().
			Comment("UTC, microsecond-truncated ticket expiry time"),
		field.Time("consumed_at").
			Optional().
			Nillable().
			Comment("UTC, microsecond-truncated first successful consume time"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side ticket row creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Edges of the DiagnosisAuthTicket.
func (DiagnosisAuthTicket) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tenant", Tenant.Type).
			Field("tenant_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the DiagnosisAuthTicket.
func (DiagnosisAuthTicket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "session_id", "expires_at"),
		index.Fields("session_id", "expires_at"),
		index.Fields("expires_at"),
		index.Fields("consumed_at", "expires_at"),
	}
}
