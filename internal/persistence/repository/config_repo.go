package repository

import (
	"context"
	"fmt"
	"sync/atomic"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertsourceprofile"
	"github.com/openclarion/openclarion/internal/persistence/ent/groupingpolicy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// configRepo is the Ent-backed implementation of
// ports.ConfigurationRepository.
type configRepo struct {
	tx     *ent.Tx
	closed *atomic.Int32
}

var _ ports.ConfigurationRepository = (*configRepo)(nil)

// SaveAlertSourceProfile inserts one operator-managed alert source profile.
func (r *configRepo) SaveAlertSourceProfile(ctx context.Context, p domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertSourceProfile{}, err
	}
	builder := r.tx.AlertSourceProfile.Create().
		SetName(p.Name).
		SetKind(string(p.Kind)).
		SetBaseURL(p.BaseURL).
		SetAuthMode(string(p.AuthMode)).
		SetEnabled(p.Enabled).
		SetLabels(p.Labels)
	if p.SecretRef != "" {
		builder = builder.SetSecretRef(p.SecretRef)
	}
	if !p.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(p.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.AlertSourceProfile{}, asAlreadyExists(err)
	}
	return alertSourceProfileToDomain(saved), nil
}

// UpdateAlertSourceProfile persists mutable profile fields.
func (r *configRepo) UpdateAlertSourceProfile(ctx context.Context, p domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertSourceProfile{}, err
	}
	if p.ID == 0 {
		return domain.AlertSourceProfile{}, fmt.Errorf("update alert source profile: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.AlertSourceProfile.UpdateOneID(int(p.ID)).
		SetName(p.Name).
		SetKind(string(p.Kind)).
		SetBaseURL(p.BaseURL).
		SetAuthMode(string(p.AuthMode)).
		SetEnabled(p.Enabled).
		SetLabels(p.Labels)
	if p.SecretRef == "" {
		builder = builder.ClearSecretRef()
	} else {
		builder = builder.SetSecretRef(p.SecretRef)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.AlertSourceProfile{}, asAlreadyExists(asNotFound(err))
	}
	return alertSourceProfileToDomain(saved), nil
}

// FindAlertSourceProfileByID returns one alert source profile.
func (r *configRepo) FindAlertSourceProfileByID(ctx context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertSourceProfile{}, err
	}
	if id == 0 {
		return domain.AlertSourceProfile{}, fmt.Errorf("find alert source profile: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.AlertSourceProfile.Get(ctx, int(id))
	if err != nil {
		return domain.AlertSourceProfile{}, asNotFound(err)
	}
	return alertSourceProfileToDomain(row), nil
}

// ListAlertSourceProfiles returns the most recently updated profiles.
func (r *configRepo) ListAlertSourceProfiles(ctx context.Context, limit int) ([]domain.AlertSourceProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list alert source profiles: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.AlertSourceProfile.Query().
		Order(alertsourceprofile.ByUpdatedAt(entsql.OrderDesc()), alertsourceprofile.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list alert source profiles: %w", err)
	}
	out := make([]domain.AlertSourceProfile, len(rows))
	for i, row := range rows {
		out[i] = alertSourceProfileToDomain(row)
	}
	return out, nil
}

// SaveGroupingPolicy inserts one operator-managed grouping policy.
func (r *configRepo) SaveGroupingPolicy(ctx context.Context, p domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.GroupingPolicy{}, err
	}
	builder := r.tx.GroupingPolicy.Create().
		SetName(p.Name).
		SetDimensionKeys(p.DimensionKeys).
		SetSeverityKey(p.SeverityKey).
		SetSourceFilter(p.SourceFilter).
		SetEnabled(p.Enabled)
	if !p.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(p.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.GroupingPolicy{}, asAlreadyExists(err)
	}
	return groupingPolicyToDomain(saved), nil
}

// UpdateGroupingPolicy persists mutable grouping policy fields.
func (r *configRepo) UpdateGroupingPolicy(ctx context.Context, p domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.GroupingPolicy{}, err
	}
	if p.ID == 0 {
		return domain.GroupingPolicy{}, fmt.Errorf("update grouping policy: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	saved, err := r.tx.GroupingPolicy.UpdateOneID(int(p.ID)).
		SetName(p.Name).
		SetDimensionKeys(p.DimensionKeys).
		SetSeverityKey(p.SeverityKey).
		SetSourceFilter(p.SourceFilter).
		SetEnabled(p.Enabled).
		Save(ctx)
	if err != nil {
		return domain.GroupingPolicy{}, asAlreadyExists(asNotFound(err))
	}
	return groupingPolicyToDomain(saved), nil
}

// FindGroupingPolicyByID returns one grouping policy.
func (r *configRepo) FindGroupingPolicyByID(ctx context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.GroupingPolicy{}, err
	}
	if id == 0 {
		return domain.GroupingPolicy{}, fmt.Errorf("find grouping policy: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.GroupingPolicy.Get(ctx, int(id))
	if err != nil {
		return domain.GroupingPolicy{}, asNotFound(err)
	}
	return groupingPolicyToDomain(row), nil
}

// ListGroupingPolicies returns the most recently updated grouping policies.
func (r *configRepo) ListGroupingPolicies(ctx context.Context, limit int) ([]domain.GroupingPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list grouping policies: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.GroupingPolicy.Query().
		Order(groupingpolicy.ByUpdatedAt(entsql.OrderDesc()), groupingpolicy.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list grouping policies: %w", err)
	}
	out := make([]domain.GroupingPolicy, len(rows))
	for i, row := range rows {
		out[i] = groupingPolicyToDomain(row)
	}
	return out, nil
}
