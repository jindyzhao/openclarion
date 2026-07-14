// Package schema holds the hand-written Ent schema definitions. Each file
// in this package describes one entity; the generated client lives in the
// parent `ent` package and is produced by `make ent-generate`.
package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/pgvector/pgvector-go"
)

// RetrievalChunk is an immutable semantic projection of one accepted report.
// The report remains the source of truth; this table owns only bounded text,
// provenance, and its fixed-dimension vector index.
type RetrievalChunk struct {
	ent.Schema
}

// Fields of the RetrievalChunk.
func (RetrievalChunk) Fields() []ent.Field {
	return []ent.Field{
		field.String("source_kind").
			MaxLen(32).
			NotEmpty().
			Immutable(),
		field.Int64("source_id").
			Positive().
			Immutable(),
		field.String("source_ref").
			MaxLen(256).
			NotEmpty().
			Immutable(),
		field.Text("content").
			NotEmpty().
			Immutable(),
		field.String("content_digest").
			MinLen(64).
			MaxLen(64).
			Immutable(),
		field.String("embedding_model").
			MaxLen(128).
			NotEmpty().
			Immutable(),
		field.Int("embedding_dimensions").
			Positive().
			Immutable(),
		field.Other("embedding", pgvector.Vector{}).
			SchemaType(map[string]string{dialect.Postgres: "vector(1536)"}).
			Immutable(),
		field.JSON("metadata", json.RawMessage{}).
			Immutable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Indexes of the RetrievalChunk.
func (RetrievalChunk) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_kind", "source_id", "embedding_model").Unique(),
		index.Fields("embedding_model", "created_at"),
		index.Fields("embedding").Annotations(
			entsql.IndexType("hnsw"),
			entsql.OpClass("vector_cosine_ops"),
		),
	}
}
