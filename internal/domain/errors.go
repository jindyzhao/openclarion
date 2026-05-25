package domain

import "errors"

// ErrNotFound is returned by repository read paths when the requested
// entity does not exist. Wrap with fmt.Errorf("...: %w", ErrNotFound)
// so callers can branch with errors.Is(err, domain.ErrNotFound).
var ErrNotFound = errors.New("domain: entity not found")

// ErrAlreadyExists is returned by repository write paths when an
// insert would violate a documented natural unique key, for example:
//
//   - AlertEvent (source, canonical_fingerprint, starts_at)
//   - AlertGroup (group_key, first_seen_at)
//   - EvidenceSnapshot (alert_group_id, digest)
//   - DiagnosisTask (workflow_id, run_id)
//   - DiagnosisTaskEvent (task_id, dedupe_key) when dedupe_key is set
//
// Repositories MUST translate the underlying Postgres unique-violation
// (SQLSTATE 23505) into this sentinel so usecases can implement
// idempotent producers without inspecting database error codes.
var ErrAlreadyExists = errors.New("domain: entity already exists")

// ErrInvariantViolation is returned by domain constructors and
// mutators when the proposed state would break a documented
// invariant. It is NOT a database-level error: invariant checks run
// before any persistence call. Wrap with fmt.Errorf to attach the
// specific field or rule that failed.
var ErrInvariantViolation = errors.New("domain: invariant violation")
