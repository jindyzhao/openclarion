// Package domain contains the core OpenClarion domain types: the
// in-memory representation of alerts, alert groups, evidence
// snapshots and diagnosis tasks shared by usecases and adapters.
//
// The package has zero non-stdlib dependencies. It MUST NOT import:
//
//   - the generated Ent client (`internal/persistence/ent`)
//   - the Temporal Go SDK (`go.temporal.io/sdk/...`)
//   - HTTP / WebSocket transport types
//   - any third-party library
//
// This isolation is required by the architecture rule "domain types
// must not depend on generated code or transport details" (see
// docs/design/architecture.md, the Forbidden Patterns section).
//
// Repositories own the translation between domain and Ent types.
// Domain constructors (e.g. NewAlertEvent) enforce cross-field
// invariants that Ent column constraints alone cannot express, such
// as "AlertEvent.EndsAt is non-nil iff Status == AlertStatusResolved"
// or "DiagnosisTask.FinishedAt is non-nil iff Status is terminal".
//
// Sentinel errors (ErrNotFound, ErrAlreadyExists,
// ErrInvariantViolation) live in errors.go and are intended to be
// wrapped with fmt.Errorf("...: %w", err) so callers can use
// errors.Is for branching.
package domain
