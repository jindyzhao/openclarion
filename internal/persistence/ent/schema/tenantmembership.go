// Package schema holds the hand-written Ent schema definitions.
package schema

import (
	"fmt"
	"strings"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/openclarion/openclarion/internal/domain"
)

// TenantMembership grants one stable authentication subject access to a
// tenant. It is global registry data and therefore intentionally does not use
// TenantMixin; access is limited by the tenant administration usecase.
type TenantMembership struct {
	ent.Schema
}

// Fields of the TenantMembership.
func (TenantMembership) Fields() []ent.Field {
	validSubject := func(value string) error {
		if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("must be non-empty, trimmed, and single-line")
		}
		return nil
	}
	return []ent.Field{
		field.Int("tenant_id").Positive().Immutable(),
		field.String("subject").MaxLen(256).NotEmpty().Immutable().Validate(validSubject),
		field.String("role").
			MaxLen(16).
			Default(string(domain.TenantMembershipRoleMember)).
			Validate(func(value string) error {
				return domain.ValidateTenantMembershipRole(domain.TenantMembershipRole(value))
			}),
		field.Bool("enabled").Default(true),
		field.String("created_by").MaxLen(256).NotEmpty().Immutable().Validate(validSubject),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the TenantMembership.
func (TenantMembership) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tenant", Tenant.Type).
			Field("tenant_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the TenantMembership.
func (TenantMembership) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "subject").Unique(),
		index.Fields("subject", "enabled"),
		index.Fields("tenant_id", "enabled"),
	}
}
