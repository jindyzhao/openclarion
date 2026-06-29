package repository

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/predicate"
	"github.com/openclarion/openclarion/internal/persistence/ent/rbacassignment"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// rbacRepo is the Ent-backed implementation of ports.RBACRepository.
type rbacRepo struct {
	tx     *ent.Tx
	closed *atomic.Int32
}

var _ ports.RBACRepository = (*rbacRepo)(nil)

// SaveAssignment inserts one local role assignment.
func (r *rbacRepo) SaveAssignment(ctx context.Context, a domain.RBACAssignment) (domain.RBACAssignment, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.RBACAssignment{}, err
	}
	a, err := normalizeRBACAssignmentForPersistence(a)
	if err != nil {
		return domain.RBACAssignment{}, err
	}
	if err := validateRBACAuditActors("save rbac assignment", a.CreatedBy, a.UpdatedBy); err != nil {
		return domain.RBACAssignment{}, err
	}
	builder := r.tx.RBACAssignment.Create().
		SetSubjectKind(string(a.SubjectKind)).
		SetSubjectKey(a.SubjectKey).
		SetRole(string(a.Role)).
		SetScopeKind(string(a.ScopeKind)).
		SetScopeKey(a.ScopeKey).
		SetEnabled(a.Enabled).
		SetCreatedBy(a.CreatedBy).
		SetUpdatedBy(a.UpdatedBy)
	if !a.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(a.CreatedAt)
	}
	if !a.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(a.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.RBACAssignment{}, asAlreadyExists(err)
	}
	return rbacAssignmentToDomain(saved), nil
}

// UpdateAssignment persists mutable local role assignment fields.
func (r *rbacRepo) UpdateAssignment(ctx context.Context, a domain.RBACAssignment) (domain.RBACAssignment, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.RBACAssignment{}, err
	}
	if a.ID == 0 {
		return domain.RBACAssignment{}, fmt.Errorf("update rbac assignment: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	a, err := normalizeRBACAssignmentForPersistence(a)
	if err != nil {
		return domain.RBACAssignment{}, err
	}
	if strings.TrimSpace(a.UpdatedBy) == "" {
		return domain.RBACAssignment{}, fmt.Errorf("update rbac assignment: updated_by must be non-empty: %w", domain.ErrInvariantViolation)
	}
	saved, err := r.tx.RBACAssignment.UpdateOneID(int(a.ID)).
		SetSubjectKind(string(a.SubjectKind)).
		SetSubjectKey(a.SubjectKey).
		SetRole(string(a.Role)).
		SetScopeKind(string(a.ScopeKind)).
		SetScopeKey(a.ScopeKey).
		SetEnabled(a.Enabled).
		SetUpdatedBy(a.UpdatedBy).
		Save(ctx)
	if err != nil {
		return domain.RBACAssignment{}, asAlreadyExists(asNotFound(err))
	}
	return rbacAssignmentToDomain(saved), nil
}

// FindAssignmentByID returns one local role assignment.
func (r *rbacRepo) FindAssignmentByID(ctx context.Context, id domain.RBACAssignmentID) (domain.RBACAssignment, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.RBACAssignment{}, err
	}
	if id == 0 {
		return domain.RBACAssignment{}, fmt.Errorf("find rbac assignment: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.RBACAssignment.Get(ctx, int(id))
	if err != nil {
		return domain.RBACAssignment{}, asNotFound(err)
	}
	return rbacAssignmentToDomain(row), nil
}

// ListAssignments returns local role assignments from newest mutation to oldest.
func (r *rbacRepo) ListAssignments(ctx context.Context, limit int) ([]domain.RBACAssignment, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list rbac assignments: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.RBACAssignment.Query().
		Order(rbacassignment.ByUpdatedAt(entsql.OrderDesc()), rbacassignment.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rbac assignments: %w", err)
	}
	out := make([]domain.RBACAssignment, len(rows))
	for i, row := range rows {
		out[i] = rbacAssignmentToDomain(row)
	}
	return out, nil
}

// ListEnabledAssignmentsForPrincipal returns enabled assignments that may
// apply to the principal's user subject or department memberships.
func (r *rbacRepo) ListEnabledAssignmentsForPrincipal(ctx context.Context, principal domain.RBACPrincipal) ([]domain.RBACAssignment, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	subject := strings.TrimSpace(principal.Subject)
	if subject == "" {
		return nil, fmt.Errorf("list rbac assignments for principal: subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	userPredicate := rbacassignment.And(
		rbacassignment.SubjectKind(string(domain.RBACSubjectKindUser)),
		rbacassignment.SubjectKey(subject),
	)
	subjectPredicates := []predicate.RBACAssignment{userPredicate}
	departmentKeys := normalizeRepositoryDepartmentKeys(principal.DepartmentKeys)
	if len(departmentKeys) > 0 {
		subjectPredicates = append(subjectPredicates, rbacassignment.And(
			rbacassignment.SubjectKind(string(domain.RBACSubjectKindDepartment)),
			rbacassignment.SubjectKeyIn(departmentKeys...),
		))
	}
	rows, err := r.tx.RBACAssignment.Query().
		Where(
			rbacassignment.Enabled(true),
			rbacassignment.Or(subjectPredicates...),
		).
		Order(
			rbacassignment.ByScopeKind(entsql.OrderAsc()),
			rbacassignment.ByScopeKey(entsql.OrderAsc()),
			rbacassignment.ByRole(entsql.OrderAsc()),
			rbacassignment.ByID(entsql.OrderAsc()),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rbac assignments for principal: %w", err)
	}
	out := make([]domain.RBACAssignment, len(rows))
	for i, row := range rows {
		out[i] = rbacAssignmentToDomain(row)
	}
	return out, nil
}

func validateRBACAuditActors(action, createdBy, updatedBy string) error {
	if strings.TrimSpace(createdBy) == "" {
		return fmt.Errorf("%s: created_by must be non-empty: %w", action, domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(updatedBy) == "" {
		return fmt.Errorf("%s: updated_by must be non-empty: %w", action, domain.ErrInvariantViolation)
	}
	return nil
}

func normalizeRBACAssignmentForPersistence(a domain.RBACAssignment) (domain.RBACAssignment, error) {
	out, err := domain.NewRBACAssignment(
		a.SubjectKind,
		a.SubjectKey,
		a.Role,
		a.ScopeKind,
		a.ScopeKey,
		a.Enabled,
	)
	if err != nil {
		return domain.RBACAssignment{}, err
	}
	out.ID = a.ID
	out.CreatedBy = strings.TrimSpace(a.CreatedBy)
	out.UpdatedBy = strings.TrimSpace(a.UpdatedBy)
	out.CreatedAt = a.CreatedAt
	out.UpdatedAt = a.UpdatedAt
	return out, nil
}

func normalizeRepositoryDepartmentKeys(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, key := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}
