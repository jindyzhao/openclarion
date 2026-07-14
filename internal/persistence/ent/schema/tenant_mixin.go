package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// tenantMixins attaches the tenant ownership contract to a business entity.
// Each entity declares Mixin explicitly so Ent's schema loader does not treat
// a shared embedded schema as another generated entity.
func tenantMixins() []ent.Mixin {
	return []ent.Mixin{TenantMixin{}}
}

// TenantMixin adds immutable tenant ownership. Runtime interceptors and hooks
// enforce query and mutation isolation for every schema embedding it.
type TenantMixin struct {
	mixin.Schema
}

// Fields of the TenantMixin.
func (TenantMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Int("tenant_id").
			Positive().
			Immutable().
			Comment("tenant owning this row; assigned from authenticated operation context"),
	}
}

// Edges of the TenantMixin.
func (TenantMixin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tenant", Tenant.Type).
			Field("tenant_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the TenantMixin keep tenant-prefixed scans bounded.
func (TenantMixin) Indexes() []ent.Index {
	return []ent.Index{index.Fields("tenant_id")}
}
