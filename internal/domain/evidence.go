package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SnapshotStatus is the row-level status of an EvidenceSnapshot. It
// is the WORST of all per-provider statuses recorded in Provenance:
//
//   - "complete": every provider returned ok
//   - "partial":  at least one provider returned partial; downstream
//     stages MAY proceed but report quality is degraded
//   - "failed":   at least one provider returned failed
//
// This vocabulary is text-typed (not a database enum) so adding a
// new state does not require a schema migration.
type SnapshotStatus string

// SnapshotStatus enumeration.
const (
	SnapshotStatusComplete SnapshotStatus = "complete"
	SnapshotStatusPartial  SnapshotStatus = "partial"
	SnapshotStatusFailed   SnapshotStatus = "failed"
)

// Valid reports whether s is a known SnapshotStatus value.
func (s SnapshotStatus) Valid() bool {
	switch s {
	case SnapshotStatusComplete, SnapshotStatusPartial, SnapshotStatusFailed:
		return true
	}
	return false
}

// IsTerminal reports whether s is a "no further evidence will be
// added" state. All three current values are terminal because
// EvidenceSnapshot is immutable once persisted; the helper exists so
// callers do not depend on that fact.
func (s SnapshotStatus) IsTerminal() bool {
	return s.Valid()
}

// EvidenceSnapshot is the enriched evidence package produced by
// Stage S2. It is the single input contract for downstream AI
// analysis; ReportFanOutWorkflow consumes the snapshot, never the
// live providers, so report generation is deterministic and
// replayable.
//
// Idempotency boundary at persistence: per-group, NOT cross-row
// global. (AlertGroupID, Digest) is UNIQUE, so Activity retries
// within the same group with the same canonical payload collapse to
// one row. Two different groups MAY produce identical canonical
// payloads and are legitimately distinct rows.
type EvidenceSnapshot struct {
	ID                EvidenceSnapshotID
	AlertGroupID      AlertGroupID
	Digest            string
	Payload           json.RawMessage
	Provenance        json.RawMessage
	Status            SnapshotStatus
	MissingFields     []string
	CreatedByWorkflow string
	CreatedAt         time.Time
}

// NewEvidenceSnapshot constructs an EvidenceSnapshot. Cross-field
// invariants enforced:
//
//   - AlertGroupID must be non-zero (snapshot must be linked to a
//     persisted group)
//   - Digest must be non-empty
//   - Payload must be non-empty (raw json.RawMessage{} or nil is
//     rejected; an empty snapshot has no idempotency value)
//   - Status must be a known SnapshotStatus
//   - MissingFields must contain unique, trimmed, non-empty paths exactly when
//     Status == "partial"
//
// CreatedByWorkflow is optional (empty string is allowed for
// manually-seeded test rows or backfill).
func NewEvidenceSnapshot(
	groupID AlertGroupID,
	digest string,
	payload, provenance json.RawMessage,
	status SnapshotStatus,
	missingFields []string,
	createdByWorkflow string,
) (EvidenceSnapshot, error) {
	if groupID == 0 {
		return EvidenceSnapshot{}, fmt.Errorf("evidence snapshot: alert_group_id must be non-zero: %w", ErrInvariantViolation)
	}
	if digest == "" {
		return EvidenceSnapshot{}, fmt.Errorf("evidence snapshot: digest must be non-empty: %w", ErrInvariantViolation)
	}
	if len(payload) == 0 {
		return EvidenceSnapshot{}, fmt.Errorf("evidence snapshot: payload must be non-empty: %w", ErrInvariantViolation)
	}
	if err := ValidateEvidenceSnapshotQuality(status, missingFields); err != nil {
		return EvidenceSnapshot{}, err
	}
	return EvidenceSnapshot{
		AlertGroupID:      groupID,
		Digest:            digest,
		Payload:           payload,
		Provenance:        provenance,
		Status:            status,
		MissingFields:     missingFields,
		CreatedByWorkflow: createdByWorkflow,
	}, nil
}

// ValidateEvidenceSnapshotQuality verifies the status/missing-field contract
// for snapshots constructed locally or loaded from persistence.
func ValidateEvidenceSnapshotQuality(status SnapshotStatus, missingFields []string) error {
	if !status.Valid() {
		return fmt.Errorf("evidence snapshot: status %q is not a known value: %w", status, ErrInvariantViolation)
	}
	if len(missingFields) > 0 && status != SnapshotStatusPartial {
		return fmt.Errorf("evidence snapshot: missing_fields requires partial status (got %q): %w", status, ErrInvariantViolation)
	}
	if status == SnapshotStatusPartial && len(missingFields) == 0 {
		return fmt.Errorf("evidence snapshot: partial status requires missing_fields: %w", ErrInvariantViolation)
	}

	seen := make(map[string]struct{}, len(missingFields))
	for i, field := range missingFields {
		if strings.TrimSpace(field) == "" || strings.TrimSpace(field) != field {
			return fmt.Errorf("evidence snapshot: missing_fields[%d] must be trimmed and non-empty: %w", i, ErrInvariantViolation)
		}
		if _, duplicate := seen[field]; duplicate {
			return fmt.Errorf("evidence snapshot: missing_fields[%d] duplicates %q: %w", i, field, ErrInvariantViolation)
		}
		seen[field] = struct{}{}
	}
	return nil
}

// ValidateEvidenceSnapshotReportability rejects failed or malformed snapshot
// quality metadata before evidence reaches an LLM or sandbox boundary.
func ValidateEvidenceSnapshotReportability(status SnapshotStatus, missingFields []string) error {
	if !status.Valid() || status == SnapshotStatusFailed {
		return fmt.Errorf("evidence snapshot: status %q cannot produce a report: %w", status, ErrInvariantViolation)
	}
	return ValidateEvidenceSnapshotQuality(status, missingFields)
}
