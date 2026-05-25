// Package repository contains the Ent-backed implementations of the
// persistence ports defined in `internal/usecases/ports`.
//
// Layering rules (per docs/design/architecture.md):
//
//   - This package MAY import `internal/domain`, `internal/usecases/ports`,
//     `internal/persistence/ent` (and its sub-packages), and the
//     standard library + Ent's runtime + the Postgres driver.
//   - This package MUST NOT import any transport package, Temporal
//     SDK, or any provider integration. Repository code is pure
//     Postgres I/O wrapped behind the usecase-owned ports.
//   - Domain entities cross the boundary in BOTH directions through
//     the helpers in mapper.go. The Ent client and Tx types are
//     never returned beyond this package.
//
// Transaction model:
//
//   - The three repository implementations (alertRepo, evidenceRepo,
//     diagnosisRepo) all run inside a single `*ent.Tx`. They are
//     therefore created exclusively by the UnitOfWork; there is no
//     "non-transactional" repository constructor. This matches the
//     usecase-layer guarantee that grouping / evidence / diagnosis
//     writes are atomic.
//   - The UnitOfWork is created by Factory.Begin or Factory.WithinTx.
//     WithinTx is the recommended default (panic-safe rollback);
//     Begin is the escape hatch for orchestration whose transaction
//     lifetime spans control boundaries.
//
// Error translation:
//
//   - Postgres SQLSTATE 23505 (unique-violation) is translated to a
//     wrapped domain.ErrAlreadyExists so usecases can implement
//     idempotent producers without inspecting Postgres error codes.
//   - ent's NotFoundError is translated to a wrapped
//     domain.ErrNotFound for the same reason.
//   - All other database errors propagate as-is. Callers SHOULD use
//     errors.Is to branch on the sentinel values.
//
// Concurrency:
//
//   - A UnitOfWork instance is bound to one transaction and is NOT
//     safe for concurrent use. Repositories obtained from the same
//     UoW share the underlying *ent.Tx and therefore inherit that
//     constraint.
package repository
