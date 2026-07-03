package repository

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/directorydepartment"
	"github.com/openclarion/openclarion/internal/persistence/ent/directorysyncrun"
	"github.com/openclarion/openclarion/internal/persistence/ent/directoryuser"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// directoryRepo is the Ent-backed implementation of ports.DirectoryRepository.
type directoryRepo struct {
	tx     *ent.Tx
	closed *atomic.Int32
}

var _ ports.DirectoryRepository = (*directoryRepo)(nil)

// UpsertDepartment inserts or updates one provider department projection.
func (r *directoryRepo) UpsertDepartment(ctx context.Context, d domain.DirectoryDepartment) (domain.DirectoryDepartment, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryDepartment{}, err
	}
	d, err := normalizeDirectoryDepartmentForPersistence(d)
	if err != nil {
		return domain.DirectoryDepartment{}, err
	}
	if err := validateDirectoryNaturalKey("upsert directory department", d.Provider, d.ExternalID); err != nil {
		return domain.DirectoryDepartment{}, err
	}
	id, err := buildDirectoryDepartmentCreate(r.tx.DirectoryDepartment.Create(), d).
		OnConflictColumns(directorydepartment.FieldProvider, directorydepartment.FieldExternalID).
		UpdateNewValues().
		Update(func(u *ent.DirectoryDepartmentUpsert) {
			setDirectoryDepartmentUpsertClears(u, d)
		}).
		ID(ctx)
	if err != nil {
		return domain.DirectoryDepartment{}, asAlreadyExists(err)
	}
	saved, err := r.tx.DirectoryDepartment.Get(ctx, id)
	if err != nil {
		return domain.DirectoryDepartment{}, asNotFound(err)
	}
	return directoryDepartmentToDomain(saved), nil
}

// FindDepartmentByExternalID returns one provider department projection.
func (r *directoryRepo) FindDepartmentByExternalID(ctx context.Context, provider, externalID string) (domain.DirectoryDepartment, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryDepartment{}, err
	}
	if err := validateDirectoryNaturalKey("find directory department", provider, externalID); err != nil {
		return domain.DirectoryDepartment{}, err
	}
	row, err := r.tx.DirectoryDepartment.Query().
		Where(directorydepartment.Provider(strings.TrimSpace(provider)), directorydepartment.ExternalID(strings.TrimSpace(externalID))).
		Only(ctx)
	if err != nil {
		return domain.DirectoryDepartment{}, asNotFound(err)
	}
	return directoryDepartmentToDomain(row), nil
}

// ListDepartments returns provider departments in picker-friendly order.
func (r *directoryRepo) ListDepartments(ctx context.Context, provider string, limit int) ([]domain.DirectoryDepartment, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, fmt.Errorf("list directory departments: provider must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list directory departments: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.DirectoryDepartment.Query().
		Where(directorydepartment.Provider(provider)).
		Order(directorydepartment.ByPath(entsql.OrderAsc()), directorydepartment.ByID(entsql.OrderAsc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list directory departments: %w", err)
	}
	out := make([]domain.DirectoryDepartment, len(rows))
	for i, row := range rows {
		out[i] = directoryDepartmentToDomain(row)
	}
	return out, nil
}

// UpsertUser inserts or updates one provider user projection.
func (r *directoryRepo) UpsertUser(ctx context.Context, u domain.DirectoryUser) (domain.DirectoryUser, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryUser{}, err
	}
	u, err := normalizeDirectoryUserForPersistence(u)
	if err != nil {
		return domain.DirectoryUser{}, err
	}
	if err := validateDirectoryNaturalKey("upsert directory user", u.Provider, u.Subject); err != nil {
		return domain.DirectoryUser{}, err
	}
	id, err := buildDirectoryUserCreate(r.tx.DirectoryUser.Create(), u).
		OnConflictColumns(directoryuser.FieldProvider, directoryuser.FieldSubject).
		UpdateNewValues().
		Update(func(upsert *ent.DirectoryUserUpsert) {
			setDirectoryUserUpsertClears(upsert, u)
		}).
		ID(ctx)
	if err != nil {
		return domain.DirectoryUser{}, asAlreadyExists(err)
	}
	saved, err := r.tx.DirectoryUser.Get(ctx, id)
	if err != nil {
		return domain.DirectoryUser{}, asNotFound(err)
	}
	return directoryUserToDomain(saved), nil
}

// FindUserBySubject returns one provider user projection by login subject.
func (r *directoryRepo) FindUserBySubject(ctx context.Context, provider, subject string) (domain.DirectoryUser, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryUser{}, err
	}
	if err := validateDirectoryNaturalKey("find directory user", provider, subject); err != nil {
		return domain.DirectoryUser{}, err
	}
	row, err := r.tx.DirectoryUser.Query().
		Where(directoryuser.Provider(strings.TrimSpace(provider)), directoryuser.Subject(strings.TrimSpace(subject))).
		Only(ctx)
	if err != nil {
		return domain.DirectoryUser{}, asNotFound(err)
	}
	return directoryUserToDomain(row), nil
}

// FindUserByExternalID returns one provider user projection by upstream ID.
func (r *directoryRepo) FindUserByExternalID(ctx context.Context, provider, externalID string) (domain.DirectoryUser, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryUser{}, err
	}
	if err := validateDirectoryNaturalKey("find directory user", provider, externalID); err != nil {
		return domain.DirectoryUser{}, err
	}
	row, err := r.tx.DirectoryUser.Query().
		Where(directoryuser.Provider(strings.TrimSpace(provider)), directoryuser.ExternalID(strings.TrimSpace(externalID))).
		Only(ctx)
	if err != nil {
		return domain.DirectoryUser{}, asNotFound(err)
	}
	return directoryUserToDomain(row), nil
}

// ListUsers returns provider users in picker-friendly order.
func (r *directoryRepo) ListUsers(ctx context.Context, provider string, activeOnly bool, limit int) ([]domain.DirectoryUser, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, fmt.Errorf("list directory users: provider must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list directory users: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	query := r.tx.DirectoryUser.Query().Where(directoryuser.Provider(provider))
	if activeOnly {
		query = query.Where(directoryuser.Active(true))
	}
	rows, err := query.
		Order(directoryuser.ByDisplayName(entsql.OrderAsc()), directoryuser.ByID(entsql.OrderAsc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list directory users: %w", err)
	}
	out := make([]domain.DirectoryUser, len(rows))
	for i, row := range rows {
		out[i] = directoryUserToDomain(row)
	}
	return out, nil
}

// DeactivateStaleUsers marks active users inactive when a full sync did not
// refresh them at the provided timestamp.
func (r *directoryRepo) DeactivateStaleUsers(ctx context.Context, provider string, syncedAt time.Time) (int, error) {
	if err := checkOpen(r.closed); err != nil {
		return 0, err
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return 0, fmt.Errorf("deactivate stale directory users: provider must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if syncedAt.IsZero() {
		return 0, fmt.Errorf("deactivate stale directory users: synced_at must be non-zero: %w", domain.ErrInvariantViolation)
	}
	updated, err := r.tx.DirectoryUser.Update().
		Where(
			directoryuser.ProviderEQ(provider),
			directoryuser.ActiveEQ(true),
			directoryuser.SyncedAtLT(syncedAt),
		).
		SetActive(false).
		SetSyncedAt(syncedAt).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("deactivate stale directory users: %w", err)
	}
	return updated, nil
}

// SaveSyncRun records one admitted directory sync result.
func (r *directoryRepo) SaveSyncRun(ctx context.Context, run domain.DirectorySyncRun) (domain.DirectorySyncRun, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectorySyncRun{}, err
	}
	run, err := normalizeDirectorySyncRunForPersistence(run)
	if err != nil {
		return domain.DirectorySyncRun{}, err
	}
	if strings.TrimSpace(run.Provider) == "" {
		return domain.DirectorySyncRun{}, fmt.Errorf("save directory sync run: provider must be non-empty: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.DirectorySyncRun.Create().
		SetProvider(run.Provider).
		SetPageSize(run.PageSize).
		SetStatus(string(run.Status)).
		SetFailureCode(run.FailureCode).
		SetFailureMessage(run.FailureMessage).
		SetDepartmentPages(run.DepartmentPages).
		SetUserPages(run.UserPages).
		SetDepartmentsUpserted(run.DepartmentsUpserted).
		SetUsersUpserted(run.UsersUpserted).
		SetSyncedAt(run.SyncedAt)
	if run.UpdatedAfter != nil {
		builder = builder.SetUpdatedAfter(*run.UpdatedAfter)
	}
	if !run.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(run.CreatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DirectorySyncRun{}, err
	}
	return directorySyncRunToDomain(saved), nil
}

// ListSyncRuns returns provider sync runs from newest to oldest.
func (r *directoryRepo) ListSyncRuns(ctx context.Context, provider string, limit int) ([]domain.DirectorySyncRun, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, fmt.Errorf("list directory sync runs: provider must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list directory sync runs: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.DirectorySyncRun.Query().
		Where(directorysyncrun.Provider(provider)).
		Order(directorysyncrun.BySyncedAt(entsql.OrderDesc()), directorysyncrun.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list directory sync runs: %w", err)
	}
	out := make([]domain.DirectorySyncRun, len(rows))
	for i, row := range rows {
		out[i] = directorySyncRunToDomain(row)
	}
	return out, nil
}

func buildDirectoryDepartmentCreate(builder *ent.DirectoryDepartmentCreate, d domain.DirectoryDepartment) *ent.DirectoryDepartmentCreate {
	builder = builder.
		SetProvider(d.Provider).
		SetExternalID(d.ExternalID).
		SetName(d.Name).
		SetDisplayName(d.DisplayName).
		SetPath(d.Path).
		SetLevel(d.Level).
		SetMemberCount(d.MemberCount).
		SetSyncedAt(d.SyncedAt)
	builder = setDirectoryDepartmentCreateOptional(builder, d)
	if !d.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(d.CreatedAt)
	}
	if !d.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(d.UpdatedAt)
	}
	return builder
}

func buildDirectoryUserCreate(builder *ent.DirectoryUserCreate, u domain.DirectoryUser) *ent.DirectoryUserCreate {
	builder = builder.
		SetProvider(u.Provider).
		SetSubject(u.Subject).
		SetExternalID(u.ExternalID).
		SetUsername(u.Username).
		SetDisplayName(u.DisplayName).
		SetDepartmentPaths(u.DepartmentPaths).
		SetDepartmentExternalIds(u.DepartmentExternalIDs).
		SetActive(u.Active).
		SetSyncedAt(u.SyncedAt)
	builder = setDirectoryUserCreateOptional(builder, u)
	if !u.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(u.CreatedAt)
	}
	if !u.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(u.UpdatedAt)
	}
	return builder
}

func validateDirectoryNaturalKey(action, provider, key string) error {
	if strings.TrimSpace(provider) == "" {
		return fmt.Errorf("%s: provider must be non-empty: %w", action, domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%s: key must be non-empty: %w", action, domain.ErrInvariantViolation)
	}
	return nil
}

func normalizeDirectoryDepartmentForPersistence(d domain.DirectoryDepartment) (domain.DirectoryDepartment, error) {
	out, err := domain.NewDirectoryDepartment(
		d.Provider,
		d.ExternalID,
		d.ParentExternalID,
		d.Name,
		d.DisplayName,
		d.Path,
		d.ParentPath,
		d.Level,
		d.Source,
		d.MemberCount,
		d.SourceUpdatedAt,
		d.SyncedAt,
	)
	if err != nil {
		return domain.DirectoryDepartment{}, err
	}
	out.ID = d.ID
	out.CreatedAt = d.CreatedAt
	out.UpdatedAt = d.UpdatedAt
	return out, nil
}

func normalizeDirectoryUserForPersistence(u domain.DirectoryUser) (domain.DirectoryUser, error) {
	out, err := domain.NewDirectoryUser(
		u.Provider,
		u.Subject,
		u.ExternalID,
		u.Username,
		u.DisplayName,
		u.Email,
		u.JobTitle,
		u.Department,
		u.Section,
		u.DepartmentPath,
		u.DepartmentPaths,
		u.DepartmentExternalIDs,
		u.Active,
		u.SourceUpdatedAt,
		u.SyncedAt,
	)
	if err != nil {
		return domain.DirectoryUser{}, err
	}
	out.ID = u.ID
	out.CreatedAt = u.CreatedAt
	out.UpdatedAt = u.UpdatedAt
	return out, nil
}

func normalizeDirectorySyncRunForPersistence(run domain.DirectorySyncRun) (domain.DirectorySyncRun, error) {
	var (
		out domain.DirectorySyncRun
		err error
	)
	switch run.Status {
	case domain.DirectorySyncRunStatusSucceeded:
		out, err = domain.NewDirectorySyncSucceededRun(
			run.Provider,
			run.PageSize,
			run.UpdatedAfter,
			run.DepartmentPages,
			run.UserPages,
			run.DepartmentsUpserted,
			run.UsersUpserted,
			run.SyncedAt,
		)
	case domain.DirectorySyncRunStatusFailed:
		out, err = domain.NewDirectorySyncFailedRun(
			run.Provider,
			run.PageSize,
			run.UpdatedAfter,
			run.FailureCode,
			run.FailureMessage,
			run.DepartmentPages,
			run.UserPages,
			run.DepartmentsUpserted,
			run.UsersUpserted,
			run.SyncedAt,
		)
	default:
		return domain.DirectorySyncRun{}, fmt.Errorf("directory sync run: unsupported status %q: %w", run.Status, domain.ErrInvariantViolation)
	}
	if err != nil {
		return domain.DirectorySyncRun{}, err
	}
	out.ID = run.ID
	out.CreatedAt = run.CreatedAt
	return out, nil
}

func setDirectoryDepartmentCreateOptional(builder *ent.DirectoryDepartmentCreate, d domain.DirectoryDepartment) *ent.DirectoryDepartmentCreate {
	if d.ParentExternalID != "" {
		builder = builder.SetParentExternalID(d.ParentExternalID)
	}
	if d.ParentPath != "" {
		builder = builder.SetParentPath(d.ParentPath)
	}
	if d.Source != "" {
		builder = builder.SetSource(d.Source)
	}
	if d.SourceUpdatedAt != nil {
		builder = builder.SetSourceUpdatedAt(*d.SourceUpdatedAt)
	}
	return builder
}

func setDirectoryDepartmentUpsertClears(builder *ent.DirectoryDepartmentUpsert, d domain.DirectoryDepartment) {
	if d.ParentExternalID == "" {
		builder.ClearParentExternalID()
	}
	if d.ParentPath == "" {
		builder.ClearParentPath()
	}
	if d.Source == "" {
		builder.ClearSource()
	}
	if d.SourceUpdatedAt == nil {
		builder.ClearSourceUpdatedAt()
	}
}

func setDirectoryUserCreateOptional(builder *ent.DirectoryUserCreate, u domain.DirectoryUser) *ent.DirectoryUserCreate {
	if u.Email != "" {
		builder = builder.SetEmail(u.Email)
	}
	if u.JobTitle != "" {
		builder = builder.SetJobTitle(u.JobTitle)
	}
	if u.Department != "" {
		builder = builder.SetDepartment(u.Department)
	}
	if u.Section != "" {
		builder = builder.SetSection(u.Section)
	}
	if u.DepartmentPath != "" {
		builder = builder.SetDepartmentPath(u.DepartmentPath)
	}
	if u.SourceUpdatedAt != nil {
		builder = builder.SetSourceUpdatedAt(*u.SourceUpdatedAt)
	}
	return builder
}

func setDirectoryUserUpsertClears(builder *ent.DirectoryUserUpsert, u domain.DirectoryUser) {
	if u.Email == "" {
		builder.ClearEmail()
	}
	if u.JobTitle == "" {
		builder.ClearJobTitle()
	}
	if u.Department == "" {
		builder.ClearDepartment()
	}
	if u.Section == "" {
		builder.ClearSection()
	}
	if u.DepartmentPath == "" {
		builder.ClearDepartmentPath()
	}
	if u.SourceUpdatedAt == nil {
		builder.ClearSourceUpdatedAt()
	}
}
