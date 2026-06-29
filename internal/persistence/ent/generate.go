// Package ent contains the Ent ORM-generated client and the hand-written
// schema files (under ./schema/) that drive that generation.
//
// Code generation is the canonical path: never edit the generated files
// in this directory directly. Instead, edit a schema under ./schema/ and
// re-run `make ent-generate` (which is a thin wrapper over the
// `go:generate` directive below). The CI freshness gate `make ent-fresh`
// enforces that the working tree has no diff after regeneration.
//
// The generator binary is pinned via the Go 1.24+ `tool` directive in
// go.mod (see `entgo.io/ent/cmd/ent`) so that the tool version is
// reproducible without a separate install step. This mirrors the
// pattern already used by `vacuum` and `oapi-codegen-exp`.
package ent

//go:generate go tool entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
