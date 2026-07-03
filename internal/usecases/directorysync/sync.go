// Package directorysync refreshes OpenClarion's local IAM directory
// projections through provider-neutral ports.
package directorysync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// DefaultPageSize is used when callers do not choose a provider page size.
	DefaultPageSize = 200
	// MaxPageSize matches the domain sync-run admission boundary.
	MaxPageSize = 500
	// DefaultMaxPages bounds a sync run against provider cursor loops.
	DefaultMaxPages = 100

	failureCodeProviderError     = "provider_error"
	failureCodeProjectionInvalid = "projection_invalid"
	failureCodePageLimitExceeded = "page_limit_exceeded"
	failureCodePersistenceError  = "persistence_error"
)

var (
	errProjectionInvalid = errors.New("directory sync: projection invalid")
	errPageLimitExceeded = errors.New("directory sync: page limit exceeded")
)

// SyncRequest configures one admitted directory projection refresh.
type SyncRequest struct {
	Provider     string
	PageSize     int
	UpdatedAfter *time.Time
	MaxPages     int
}

// Result summarizes one sync attempt and the persisted audit run when it could
// be recorded.
type Result struct {
	Run                 domain.DirectorySyncRun
	DepartmentPages     int
	UserPages           int
	DepartmentsUpserted int
	UsersUpserted       int
	UsersDeactivated    int
}

// Service coordinates provider pagination, domain validation, repository
// writes, and sync-run audit records.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	providers  map[string]ports.DirectoryProvider
	now        func() time.Time
}

// Option customizes directory sync service behavior.
type Option func(*Service)

// WithClock injects the clock used for sync-run timestamps.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// NewService constructs a directory sync service.
func NewService(uowFactory ports.UnitOfWorkFactory, providers map[string]ports.DirectoryProvider, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("directory sync: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("directory sync: at least one provider must be configured: %w", domain.ErrInvariantViolation)
	}
	svc := &Service{
		uowFactory: uowFactory,
		providers:  make(map[string]ports.DirectoryProvider, len(providers)),
		now:        time.Now,
	}
	for name, provider := range providers {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("directory sync: provider name must be non-empty: %w", domain.ErrInvariantViolation)
		}
		if err := validateProviderName(name); err != nil {
			return nil, err
		}
		if provider == nil {
			return nil, fmt.Errorf("directory sync: provider %q must be non-nil: %w", name, domain.ErrInvariantViolation)
		}
		if _, exists := svc.providers[name]; exists {
			return nil, fmt.Errorf("directory sync: provider %q configured more than once: %w", name, domain.ErrInvariantViolation)
		}
		svc.providers[name] = provider
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	if svc.now == nil {
		return nil, fmt.Errorf("directory sync: clock must be configured: %w", domain.ErrInvariantViolation)
	}
	return svc, nil
}

type admittedRequest struct {
	providerName string
	provider     ports.DirectoryProvider
	pageSize     int
	updatedAfter *time.Time
	maxPages     int
	syncedAt     time.Time
}

// Sync refreshes departments first, users second, then writes the successful
// projection and audit run in one transaction. Provider/projection failures are
// audited as failed runs without persisting partial projection pages.
func (s *Service) Sync(ctx context.Context, req SyncRequest) (Result, error) {
	admitted, err := s.admit(req)
	if err != nil {
		return Result{}, err
	}

	departments, departmentPages, err := collectDepartments(ctx, admitted)
	if err != nil {
		return s.recordFailedRun(ctx, admitted, failureCodeFor(err), failureMessageFor(err), departmentPages, 0, 0, 0, err)
	}
	users, userPages, err := collectUsers(ctx, admitted)
	if err != nil {
		return s.recordFailedRun(ctx, admitted, failureCodeFor(err), failureMessageFor(err), departmentPages, userPages, 0, 0, err)
	}

	result, err := s.persistSuccess(ctx, admitted, departments, users, departmentPages, userPages)
	if err == nil {
		return result, nil
	}
	failed, failureErr := s.recordFailedRun(ctx, admitted, failureCodePersistenceError, "directory sync persistence failed", departmentPages, userPages, 0, 0, nil)
	if failureErr != nil {
		failed.DepartmentPages = departmentPages
		failed.UserPages = userPages
		return failed, errors.Join(err, failureErr)
	}
	return failed, err
}

func (s *Service) admit(req SyncRequest) (admittedRequest, error) {
	if s == nil {
		return admittedRequest{}, fmt.Errorf("directory sync: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if s.uowFactory == nil {
		return admittedRequest{}, fmt.Errorf("directory sync: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if s.now == nil {
		return admittedRequest{}, fmt.Errorf("directory sync: clock must be configured: %w", domain.ErrInvariantViolation)
	}
	providerName := strings.TrimSpace(req.Provider)
	if providerName == "" {
		return admittedRequest{}, fmt.Errorf("directory sync: provider is required: %w", domain.ErrInvariantViolation)
	}
	if err := validateProviderName(providerName); err != nil {
		return admittedRequest{}, err
	}
	provider, ok := s.providers[providerName]
	if !ok {
		return admittedRequest{}, fmt.Errorf("directory sync: provider %q is not configured: %w", providerName, domain.ErrInvariantViolation)
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = DefaultPageSize
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		return admittedRequest{}, fmt.Errorf("directory sync: page_size must be between 1 and %d: %w", MaxPageSize, domain.ErrInvariantViolation)
	}
	maxPages := req.MaxPages
	if maxPages == 0 {
		maxPages = DefaultMaxPages
	}
	if maxPages < 1 {
		return admittedRequest{}, fmt.Errorf("directory sync: max_pages must be positive: %w", domain.ErrInvariantViolation)
	}
	syncedAt := domain.NormalizeUTCMicro(s.now())
	if syncedAt.IsZero() {
		return admittedRequest{}, fmt.Errorf("directory sync: clock returned zero time: %w", domain.ErrInvariantViolation)
	}
	return admittedRequest{
		providerName: providerName,
		provider:     provider,
		pageSize:     pageSize,
		updatedAfter: normalizeOptionalTime(req.UpdatedAfter),
		maxPages:     maxPages,
		syncedAt:     syncedAt,
	}, nil
}

func validateProviderName(provider string) error {
	if len(provider) > domain.MaxDirectoryProviderBytes {
		return fmt.Errorf("directory sync: provider exceeds %d bytes: %w", domain.MaxDirectoryProviderBytes, domain.ErrInvariantViolation)
	}
	return nil
}

func collectDepartments(ctx context.Context, req admittedRequest) ([]domain.DirectoryDepartment, int, error) {
	var out []domain.DirectoryDepartment
	cursor := ""
	pages := 0
	for {
		if pages >= req.maxPages {
			return out, pages, fmt.Errorf("%w: departments exceeded %d pages", errPageLimitExceeded, req.maxPages)
		}
		page, err := req.provider.ListDepartments(ctx, ports.DirectoryListRequest{
			Cursor:       cursor,
			PageSize:     req.pageSize,
			UpdatedAfter: cloneOptionalTime(req.updatedAfter),
		})
		if err != nil {
			return out, pages, fmt.Errorf("directory sync: list departments: %w", err)
		}
		pages++
		for _, projection := range page.Departments {
			department, err := departmentFromProjection(req.providerName, req.syncedAt, projection)
			if err != nil {
				return out, pages, err
			}
			out = append(out, department)
		}
		cursor = strings.TrimSpace(page.NextCursor)
		if cursor == "" {
			return out, pages, nil
		}
	}
}

func collectUsers(ctx context.Context, req admittedRequest) ([]domain.DirectoryUser, int, error) {
	var out []domain.DirectoryUser
	cursor := ""
	pages := 0
	for {
		if pages >= req.maxPages {
			return out, pages, fmt.Errorf("%w: users exceeded %d pages", errPageLimitExceeded, req.maxPages)
		}
		page, err := req.provider.ListUsers(ctx, ports.DirectoryListRequest{
			Cursor:       cursor,
			PageSize:     req.pageSize,
			UpdatedAfter: cloneOptionalTime(req.updatedAfter),
		})
		if err != nil {
			return out, pages, fmt.Errorf("directory sync: list users: %w", err)
		}
		pages++
		for _, projection := range page.Users {
			user, err := userFromProjection(req.providerName, req.syncedAt, projection)
			if err != nil {
				return out, pages, err
			}
			out = append(out, user)
		}
		cursor = strings.TrimSpace(page.NextCursor)
		if cursor == "" {
			return out, pages, nil
		}
	}
}

func departmentFromProjection(provider string, syncedAt time.Time, projection ports.DirectoryDepartmentProjection) (domain.DirectoryDepartment, error) {
	department, err := domain.NewDirectoryDepartment(
		provider,
		projection.ExternalID,
		projection.ParentExternalID,
		projection.Name,
		projection.DisplayName,
		projection.Path,
		projection.ParentPath,
		projection.Level,
		projection.Source,
		projection.MemberCount,
		projection.SourceUpdatedAt,
		syncedAt,
	)
	if err != nil {
		return domain.DirectoryDepartment{}, fmt.Errorf("%w: department: %w", errProjectionInvalid, err)
	}
	return department, nil
}

func userFromProjection(provider string, syncedAt time.Time, projection ports.DirectoryUserProjection) (domain.DirectoryUser, error) {
	user, err := domain.NewDirectoryUser(
		provider,
		projection.Subject,
		projection.ExternalID,
		projection.Username,
		projection.DisplayName,
		projection.Email,
		projection.JobTitle,
		projection.Department,
		projection.Section,
		projection.DepartmentPath,
		projection.DepartmentPaths,
		projection.DepartmentExternalIDs,
		projection.Active,
		projection.SourceUpdatedAt,
		syncedAt,
	)
	if err != nil {
		return domain.DirectoryUser{}, fmt.Errorf("%w: user: %w", errProjectionInvalid, err)
	}
	return user, nil
}

func (s *Service) persistSuccess(
	ctx context.Context,
	req admittedRequest,
	departments []domain.DirectoryDepartment,
	users []domain.DirectoryUser,
	departmentPages int,
	userPages int,
) (Result, error) {
	result := Result{DepartmentPages: departmentPages, UserPages: userPages}
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		for _, department := range departments {
			if _, err := uow.Directory().UpsertDepartment(ctx, department); err != nil {
				return fmt.Errorf("directory sync: upsert department: %w", err)
			}
			result.DepartmentsUpserted++
		}
		for _, user := range users {
			if _, err := uow.Directory().UpsertUser(ctx, user); err != nil {
				return fmt.Errorf("directory sync: upsert user: %w", err)
			}
			result.UsersUpserted++
		}
		if req.updatedAfter == nil {
			deactivated, err := uow.Directory().DeactivateStaleUsers(ctx, req.providerName, req.syncedAt)
			if err != nil {
				return fmt.Errorf("directory sync: deactivate stale users: %w", err)
			}
			result.UsersDeactivated = deactivated
		}
		run, err := domain.NewDirectorySyncSucceededRun(
			req.providerName,
			req.pageSize,
			req.updatedAfter,
			departmentPages,
			userPages,
			result.DepartmentsUpserted,
			result.UsersUpserted,
			req.syncedAt,
		)
		if err != nil {
			return err
		}
		saved, err := uow.Directory().SaveSyncRun(ctx, run)
		if err != nil {
			return fmt.Errorf("directory sync: save succeeded run: %w", err)
		}
		result.Run = saved
		return nil
	})
	return result, err
}

func (s *Service) recordFailedRun(
	ctx context.Context,
	req admittedRequest,
	code string,
	message string,
	departmentPages int,
	userPages int,
	departmentsUpserted int,
	usersUpserted int,
	cause error,
) (Result, error) {
	result := Result{
		DepartmentPages:     departmentPages,
		UserPages:           userPages,
		DepartmentsUpserted: departmentsUpserted,
		UsersUpserted:       usersUpserted,
	}
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		run, err := domain.NewDirectorySyncFailedRun(
			req.providerName,
			req.pageSize,
			req.updatedAfter,
			code,
			message,
			departmentPages,
			userPages,
			departmentsUpserted,
			usersUpserted,
			req.syncedAt,
		)
		if err != nil {
			return err
		}
		saved, err := uow.Directory().SaveSyncRun(ctx, run)
		if err != nil {
			return fmt.Errorf("directory sync: save failed run: %w", err)
		}
		result.Run = saved
		return nil
	})
	if err != nil {
		if cause != nil {
			return result, errors.Join(cause, err)
		}
		return result, err
	}
	return result, cause
}

func failureCodeFor(err error) string {
	switch {
	case errors.Is(err, errPageLimitExceeded):
		return failureCodePageLimitExceeded
	case errors.Is(err, errProjectionInvalid):
		return failureCodeProjectionInvalid
	default:
		return failureCodeProviderError
	}
}

func failureMessageFor(err error) string {
	switch {
	case errors.Is(err, errPageLimitExceeded):
		return "directory sync exceeded the configured page limit"
	case errors.Is(err, errProjectionInvalid):
		return "upstream directory projection was invalid"
	default:
		return "upstream directory provider returned an error"
	}
}

func normalizeOptionalTime(in *time.Time) *time.Time {
	if in == nil || in.IsZero() {
		return nil
	}
	out := domain.NormalizeUTCMicro(*in)
	return &out
}

func cloneOptionalTime(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
