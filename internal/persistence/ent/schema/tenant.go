// Package schema holds the hand-written Ent schema definitions.
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

// Tenant is one globally addressable, independently isolated workspace.
type Tenant struct {
	ent.Schema
}

// Fields of the Tenant.
func (Tenant) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
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
			Comment("stable lowercase tenant key used in authenticated selection"),
		field.String("name").
			NotEmpty().
			Validate(func(value string) error {
				normalized, err := domain.NormalizeTenantName(value)
				if err != nil {
					return err
				}
				if normalized != value {
					return fmt.Errorf("tenant name must already be normalized")
				}
				return nil
			}).
			Comment("operator-facing tenant name"),
		field.String("status").
			MaxLen(16).
			Default(string(domain.TenantStatusActive)).
			Validate(func(value string) error {
				return domain.ValidateTenantStatus(domain.TenantStatus(value))
			}).
			Comment("active or disabled; disabled tenants cannot issue new sessions"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Tenant.
func (Tenant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("memberships", TenantMembership.Type).Ref("tenant"),
	}
}

// Indexes of the Tenant.
func (Tenant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("key").Unique(),
		index.Fields("status", "name"),
	}
}
