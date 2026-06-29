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

// RBACAssignment stores one local OpenClarion role binding. IAM authenticates
// users; OpenClarion owns these product-specific authorization rules.
type RBACAssignment struct {
	ent.Schema
}

// Fields of the RBACAssignment.
func (RBACAssignment) Fields() []ent.Field {
	return []ent.Field{
		field.String("subject_kind").
			MaxLen(32).
			NotEmpty().
			Comment(`"user" or "department"`),
		field.String("subject_key").
			MaxLen(256).
			NotEmpty().
			Comment("IAM subject or local directory department key"),
		field.String("role").
			MaxLen(32).
			NotEmpty().
			Comment(`local OpenClarion role such as "admin", "operator", "responder", or "viewer"`),
		field.String("scope_kind").
			MaxLen(64).
			NotEmpty().
			Comment("OpenClarion resource family covered by the role assignment"),
		field.String("scope_key").
			MaxLen(256).
			Default("").
			Comment("resource identifier within scope_kind; empty only for global scope"),
		field.Bool("enabled").
			Default(true).
			Comment("whether this role assignment is active"),
		field.String("created_by").
			MaxLen(256).
			NotEmpty().
			Comment("authenticated subject that created the assignment"),
		field.String("updated_by").
			MaxLen(256).
			NotEmpty().
			Comment("authenticated subject that last mutated the assignment"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("server-side assignment creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("server-side last-mutation timestamp"),
	}
}

// Indexes of the RBACAssignment.
func (RBACAssignment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subject_kind", "subject_key", "role", "scope_kind", "scope_key").
			Unique(),
		index.Fields("subject_kind", "subject_key", "enabled"),
		index.Fields("scope_kind", "scope_key", "enabled"),
	}
}
