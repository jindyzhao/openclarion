package repository

import (
	"context"
	"fmt"
	"sync/atomic"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/evidencesnapshot"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// evidenceRepo is the Ent-backed implementation of
// ports.EvidenceRepository.
type evidenceRepo struct {
	tx     *ent.Tx
	closed *atomic.Int32
}

// Compile-time assertion that the implementation satisfies the port.
var _ ports.EvidenceRepository = (*evidenceRepo)(nil)

// Save inserts a new EvidenceSnapshot. The (alert_group_id, digest)
// natural unique key surfaces SQLSTATE 23505 as
// domain.ErrAlreadyExists.
func (r *evidenceRepo) Save(ctx context.Context, s domain.EvidenceSnapshot) (domain.EvidenceSnapshot, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.EvidenceSnapshot{}, err
	}
	builder := r.tx.EvidenceSnapshot.Create().
		SetAlertGroupID(int(s.AlertGroupID)).
		SetDigest(s.Digest).
		SetPayload(s.Payload).
		SetProvenance(s.Provenance)
	if s.Status != "" {
		builder = builder.SetStatus(string(s.Status))
	}
	if len(s.MissingFields) > 0 {
		builder = builder.SetMissingFields(s.MissingFields)
	}
	if s.CreatedByWorkflow != "" {
		builder = builder.SetCreatedByWorkflow(s.CreatedByWorkflow)
	}
	if !s.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(s.CreatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.EvidenceSnapshot{}, asAlreadyExists(err)
	}
	return evidenceSnapshotToDomain(saved), nil
}

// FindByID returns the EvidenceSnapshot or domain.ErrNotFound.
func (r *evidenceRepo) FindByID(ctx context.Context, id domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.EvidenceSnapshot{}, err
	}
	row, err := r.tx.EvidenceSnapshot.Get(ctx, int(id))
	if err != nil {
		return domain.EvidenceSnapshot{}, asNotFound(err)
	}
	return evidenceSnapshotToDomain(row), nil
}

// FindByGroupAndDigest returns the snapshot matching the per-group
// idempotency key. Producers use this to retrieve a row after a
// duplicate-key insert without re-deriving the surrogate ID.
func (r *evidenceRepo) FindByGroupAndDigest(ctx context.Context, groupID domain.AlertGroupID, digest string) (domain.EvidenceSnapshot, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.EvidenceSnapshot{}, err
	}
	if groupID == 0 {
		return domain.EvidenceSnapshot{}, fmt.Errorf("find evidence by group/digest: group id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if digest == "" {
		return domain.EvidenceSnapshot{}, fmt.Errorf("find evidence by group/digest: digest must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.EvidenceSnapshot.Query().
		Where(
			evidencesnapshot.AlertGroupIDEQ(int(groupID)),
			evidencesnapshot.DigestEQ(digest),
		).
		Only(ctx)
	if err != nil {
		return domain.EvidenceSnapshot{}, asNotFound(err)
	}
	return evidenceSnapshotToDomain(row), nil
}

// ListByGroup returns snapshots for a group ordered by created_at
// descending, capped by limit.
func (r *evidenceRepo) ListByGroup(ctx context.Context, groupID domain.AlertGroupID, limit int) ([]domain.EvidenceSnapshot, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if groupID == 0 {
		return nil, fmt.Errorf("list evidence by group: group id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list evidence by group: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.EvidenceSnapshot.Query().
		Where(evidencesnapshot.AlertGroupIDEQ(int(groupID))).
		Order(evidencesnapshot.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list evidence by group: %w", err)
	}
	out := make([]domain.EvidenceSnapshot, len(rows))
	for i, row := range rows {
		out[i] = evidenceSnapshotToDomain(row)
	}
	return out, nil
}

// List returns the most recent snapshots across groups, ordered by
// created_at descending with id as the deterministic tie-breaker.
func (r *evidenceRepo) List(ctx context.Context, limit int) ([]domain.EvidenceSnapshot, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list evidence: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.EvidenceSnapshot.Query().
		Order(evidencesnapshot.ByCreatedAt(entsql.OrderDesc()), evidencesnapshot.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list evidence: %w", err)
	}
	out := make([]domain.EvidenceSnapshot, len(rows))
	for i, row := range rows {
		out[i] = evidenceSnapshotToDomain(row)
	}
	return out, nil
}
