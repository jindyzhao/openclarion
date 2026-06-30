package directorysync

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceSyncUpsertsPagesAndRecordsSuccess(t *testing.T) {
	now := time.Date(2026, 6, 29, 11, 1, 2, 123456789, time.FixedZone("CST", 8*60*60))
	updatedAfter := time.Date(2026, 6, 29, 10, 1, 2, 987654321, time.FixedZone("CST", 8*60*60))
	sourceUpdatedAt := time.Date(2026, 6, 29, 9, 1, 2, 777777777, time.FixedZone("CST", 8*60*60))
	provider := &fakeDirectoryProvider{
		departmentPages: []ports.DirectoryDepartmentPage{
			{
				Departments: []ports.DirectoryDepartmentProjection{
					{
						ExternalID:      "dep-1",
						Name:            "Platform",
						DisplayName:     "Platform",
						Path:            "IT/Platform",
						Level:           1,
						Source:          "iam",
						MemberCount:     12,
						SourceUpdatedAt: &sourceUpdatedAt,
					},
				},
				NextCursor: "departments-2",
			},
			{
				Departments: []ports.DirectoryDepartmentProjection{
					{
						ExternalID:       "dep-2",
						ParentExternalID: "dep-1",
						Name:             "SRE",
						Path:             "IT/Platform/SRE",
						ParentPath:       "IT/Platform",
						Level:            2,
						Source:           "iam",
						MemberCount:      5,
					},
				},
			},
		},
		userPages: []ports.DirectoryUserPage{
			{
				Users: []ports.DirectoryUserProjection{
					{
						Subject:               "iam-user-1",
						ExternalID:            "wecom-user-1",
						Username:              "user1",
						DisplayName:           "User One",
						Email:                 "USER1@EXAMPLE.COM",
						JobTitle:              "SRE",
						Department:            "Platform",
						Section:               "SRE",
						DepartmentPath:        "IT/Platform/SRE",
						DepartmentPaths:       []string{"IT/Platform", "IT/Platform/SRE"},
						DepartmentExternalIDs: []string{"dep-2", "dep-1"},
						Active:                true,
						SourceUpdatedAt:       &sourceUpdatedAt,
					},
				},
			},
		},
	}
	repo := &fakeDirectoryRepo{}
	svc := mustService(t, repo, provider, now)

	result, err := svc.Sync(context.Background(), SyncRequest{
		Provider:     "ops_iam",
		PageSize:     50,
		UpdatedAfter: &updatedAfter,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.DepartmentPages != 2 || result.UserPages != 1 || result.DepartmentsUpserted != 2 || result.UsersUpserted != 1 {
		t.Fatalf("result counters = %+v", result)
	}
	if len(repo.departments) != 2 || len(repo.users) != 1 || len(repo.runs) != 1 {
		t.Fatalf("repo state departments=%d users=%d runs=%d", len(repo.departments), len(repo.users), len(repo.runs))
	}
	if got := provider.departmentRequests; len(got) != 2 || got[0].Cursor != "" || got[1].Cursor != "departments-2" {
		t.Fatalf("department requests = %+v", got)
	}
	if got := provider.userRequests; len(got) != 1 || got[0].PageSize != 50 {
		t.Fatalf("user requests = %+v", got)
	}
	wantUpdatedAfter := domain.NormalizeUTCMicro(updatedAfter)
	if provider.departmentRequests[0].UpdatedAfter == nil || !provider.departmentRequests[0].UpdatedAfter.Equal(wantUpdatedAfter) {
		t.Fatalf("UpdatedAfter = %v, want %s", provider.departmentRequests[0].UpdatedAfter, wantUpdatedAfter)
	}
	wantSyncedAt := domain.NormalizeUTCMicro(now)
	if repo.departments[0].Provider != "ops_iam" || !repo.departments[0].SyncedAt.Equal(wantSyncedAt) {
		t.Fatalf("department identity/timestamp = %+v", repo.departments[0])
	}
	if repo.users[0].Email != "user1@example.com" || !slices.Equal(repo.users[0].DepartmentExternalIDs, []string{"dep-1", "dep-2"}) {
		t.Fatalf("user normalization = %+v", repo.users[0])
	}
	run := repo.runs[0]
	if run.Status != domain.DirectorySyncRunStatusSucceeded || run.FailureCode != "" || run.DepartmentsUpserted != 2 || run.UsersUpserted != 1 {
		t.Fatalf("run = %+v", run)
	}
	if result.Run.ID == 0 || result.Run.ID != run.ID {
		t.Fatalf("result run = %+v, saved run = %+v", result.Run, run)
	}
}

func TestServiceSyncRecordsProviderFailureWithoutUpserts(t *testing.T) {
	providerErr := errors.New("temporary upstream failure")
	provider := &fakeDirectoryProvider{departmentErr: providerErr}
	repo := &fakeDirectoryRepo{}
	svc := mustService(t, repo, provider, fixedNow())

	result, err := svc.Sync(context.Background(), SyncRequest{Provider: "ops_iam"})
	if err == nil || !strings.Contains(err.Error(), "list departments") {
		t.Fatalf("Sync err = %v, want provider failure", err)
	}
	if len(repo.departments) != 0 || len(repo.users) != 0 {
		t.Fatalf("unexpected upserts departments=%d users=%d", len(repo.departments), len(repo.users))
	}
	assertFailedRun(t, result.Run, repo.runs, failureCodeProviderError)
	if strings.Contains(repo.runs[0].FailureMessage, "temporary upstream failure") {
		t.Fatalf("failure message leaked provider error: %q", repo.runs[0].FailureMessage)
	}
}

func TestServiceSyncRecordsInvalidProjectionFailure(t *testing.T) {
	provider := &fakeDirectoryProvider{
		departmentPages: []ports.DirectoryDepartmentPage{
			{
				Departments: []ports.DirectoryDepartmentProjection{
					{Name: "Missing external id"},
				},
			},
		},
	}
	repo := &fakeDirectoryRepo{}
	svc := mustService(t, repo, provider, fixedNow())

	result, err := svc.Sync(context.Background(), SyncRequest{Provider: "ops_iam"})
	if !errors.Is(err, domain.ErrInvariantViolation) || !errors.Is(err, errProjectionInvalid) {
		t.Fatalf("Sync err = %v, want projection invariant", err)
	}
	if len(repo.departments) != 0 || len(repo.users) != 0 {
		t.Fatalf("unexpected upserts departments=%d users=%d", len(repo.departments), len(repo.users))
	}
	assertFailedRun(t, result.Run, repo.runs, failureCodeProjectionInvalid)
	if result.DepartmentPages != 1 || result.UserPages != 0 {
		t.Fatalf("result counters = %+v", result)
	}
}

func TestServiceSyncRecordsPageLimitFailure(t *testing.T) {
	provider := &fakeDirectoryProvider{
		departmentPages: []ports.DirectoryDepartmentPage{
			{
				Departments: []ports.DirectoryDepartmentProjection{
					{ExternalID: "dep-1", Name: "Platform", Level: 1},
				},
				NextCursor: "more",
			},
		},
	}
	repo := &fakeDirectoryRepo{}
	svc := mustService(t, repo, provider, fixedNow())

	result, err := svc.Sync(context.Background(), SyncRequest{Provider: "ops_iam", MaxPages: 1})
	if !errors.Is(err, errPageLimitExceeded) {
		t.Fatalf("Sync err = %v, want page limit", err)
	}
	if len(provider.departmentRequests) != 1 {
		t.Fatalf("department requests = %d, want 1", len(provider.departmentRequests))
	}
	if len(repo.departments) != 0 || len(repo.users) != 0 {
		t.Fatalf("unexpected upserts departments=%d users=%d", len(repo.departments), len(repo.users))
	}
	assertFailedRun(t, result.Run, repo.runs, failureCodePageLimitExceeded)
	if result.DepartmentPages != 1 {
		t.Fatalf("DepartmentPages = %d, want 1", result.DepartmentPages)
	}
}

func TestServiceSyncRecordsPersistenceFailure(t *testing.T) {
	provider := &fakeDirectoryProvider{
		departmentPages: []ports.DirectoryDepartmentPage{
			{Departments: []ports.DirectoryDepartmentProjection{{ExternalID: "dep-1", Name: "Platform", Level: 1}}},
		},
		userPages: []ports.DirectoryUserPage{
			{Users: []ports.DirectoryUserProjection{{Subject: "iam-user-1", Active: true}}},
		},
	}
	repo := &fakeDirectoryRepo{failUpsertUser: errors.New("database unavailable")}
	svc := mustService(t, repo, provider, fixedNow())

	result, err := svc.Sync(context.Background(), SyncRequest{Provider: "ops_iam"})
	if err == nil || !strings.Contains(err.Error(), "upsert user") {
		t.Fatalf("Sync err = %v, want persistence failure", err)
	}
	if len(repo.departments) != 0 || len(repo.users) != 0 {
		t.Fatalf("transaction should roll back projections, got departments=%d users=%d", len(repo.departments), len(repo.users))
	}
	assertFailedRun(t, result.Run, repo.runs, failureCodePersistenceError)
	if repo.runs[0].DepartmentsUpserted != 0 || repo.runs[0].UsersUpserted != 0 {
		t.Fatalf("failed persistence run counters = %+v", repo.runs[0])
	}
}

func TestNewServiceRejectsOverlongProviderName(t *testing.T) {
	repo := &fakeDirectoryRepo{}
	providerName := strings.Repeat("x", domain.MaxDirectoryProviderBytes+1)

	_, err := NewService(&fakeUOWFactory{repo: repo}, map[string]ports.DirectoryProvider{
		providerName: &fakeDirectoryProvider{},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("NewService err = %v, want ErrInvariantViolation", err)
	}
}

func TestServiceRejectsInvalidRequestBeforeProviderAndAudit(t *testing.T) {
	tests := []struct {
		name string
		req  SyncRequest
	}{
		{name: "missing provider", req: SyncRequest{}},
		{name: "unknown provider", req: SyncRequest{Provider: "missing"}},
		{name: "overlong provider", req: SyncRequest{Provider: strings.Repeat("x", domain.MaxDirectoryProviderBytes+1)}},
		{name: "oversized page", req: SyncRequest{Provider: "ops_iam", PageSize: MaxPageSize + 1}},
		{name: "negative max pages", req: SyncRequest{Provider: "ops_iam", MaxPages: -1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeDirectoryProvider{}
			repo := &fakeDirectoryRepo{}
			svc := mustService(t, repo, provider, fixedNow())

			_, err := svc.Sync(context.Background(), tc.req)
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("Sync err = %v, want ErrInvariantViolation", err)
			}
			if len(provider.departmentRequests) != 0 || len(provider.userRequests) != 0 || len(repo.runs) != 0 {
				t.Fatalf("unexpected side effects provider=%+v repo runs=%d", provider, len(repo.runs))
			}
		})
	}
}

func mustService(t *testing.T, repo *fakeDirectoryRepo, provider ports.DirectoryProvider, now time.Time) *Service {
	t.Helper()
	svc, err := NewService(&fakeUOWFactory{repo: repo}, map[string]ports.DirectoryProvider{"ops_iam": provider}, WithClock(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 29, 11, 1, 2, 123456000, time.UTC)
}

func assertFailedRun(t *testing.T, resultRun domain.DirectorySyncRun, runs []domain.DirectorySyncRun, code string) {
	t.Helper()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	run := runs[0]
	if run.Status != domain.DirectorySyncRunStatusFailed || run.FailureCode != code || run.FailureMessage == "" {
		t.Fatalf("run = %+v, want failed %q", run, code)
	}
	if resultRun.ID == 0 || resultRun.ID != run.ID {
		t.Fatalf("result run = %+v, saved run = %+v", resultRun, run)
	}
}

type fakeDirectoryProvider struct {
	departmentPages    []ports.DirectoryDepartmentPage
	userPages          []ports.DirectoryUserPage
	departmentErr      error
	userErr            error
	departmentRequests []ports.DirectoryListRequest
	userRequests       []ports.DirectoryListRequest
}

func (p *fakeDirectoryProvider) ListDepartments(_ context.Context, req ports.DirectoryListRequest) (ports.DirectoryDepartmentPage, error) {
	p.departmentRequests = append(p.departmentRequests, cloneListRequest(req))
	if p.departmentErr != nil {
		return ports.DirectoryDepartmentPage{}, p.departmentErr
	}
	idx := len(p.departmentRequests) - 1
	if idx >= len(p.departmentPages) {
		return ports.DirectoryDepartmentPage{}, nil
	}
	return p.departmentPages[idx], nil
}

func (p *fakeDirectoryProvider) ListUsers(_ context.Context, req ports.DirectoryListRequest) (ports.DirectoryUserPage, error) {
	p.userRequests = append(p.userRequests, cloneListRequest(req))
	if p.userErr != nil {
		return ports.DirectoryUserPage{}, p.userErr
	}
	idx := len(p.userRequests) - 1
	if idx >= len(p.userPages) {
		return ports.DirectoryUserPage{}, nil
	}
	return p.userPages[idx], nil
}

func cloneListRequest(req ports.DirectoryListRequest) ports.DirectoryListRequest {
	out := req
	if req.UpdatedAfter != nil {
		updatedAfter := *req.UpdatedAfter
		out.UpdatedAfter = &updatedAfter
	}
	return out
}

type fakeUOWFactory struct {
	repo *fakeDirectoryRepo
}

func (f *fakeUOWFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return fakeUOW{repo: f.repo}, nil
}

func (f *fakeUOWFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	txRepo := f.repo.clone()
	if err := fn(ctx, fakeUOW{repo: txRepo}); err != nil {
		return err
	}
	f.repo.replace(txRepo)
	return nil
}

type fakeUOW struct {
	ports.UnitOfWork
	repo *fakeDirectoryRepo
}

func (u fakeUOW) Directory() ports.DirectoryRepository {
	return u.repo
}

type fakeDirectoryRepo struct {
	ports.DirectoryRepository
	departments    []domain.DirectoryDepartment
	users          []domain.DirectoryUser
	runs           []domain.DirectorySyncRun
	failUpsertUser error
}

func (r *fakeDirectoryRepo) UpsertDepartment(_ context.Context, department domain.DirectoryDepartment) (domain.DirectoryDepartment, error) {
	department.ID = domain.DirectoryDepartmentID(len(r.departments) + 1)
	r.departments = append(r.departments, department)
	return department, nil
}

func (r *fakeDirectoryRepo) UpsertUser(_ context.Context, user domain.DirectoryUser) (domain.DirectoryUser, error) {
	if r.failUpsertUser != nil {
		return domain.DirectoryUser{}, r.failUpsertUser
	}
	user.ID = domain.DirectoryUserID(len(r.users) + 1)
	r.users = append(r.users, user)
	return user, nil
}

func (r *fakeDirectoryRepo) SaveSyncRun(_ context.Context, run domain.DirectorySyncRun) (domain.DirectorySyncRun, error) {
	run.ID = domain.DirectorySyncRunID(len(r.runs) + 1)
	r.runs = append(r.runs, run)
	return run, nil
}

func (r *fakeDirectoryRepo) clone() *fakeDirectoryRepo {
	return &fakeDirectoryRepo{
		departments:    append([]domain.DirectoryDepartment(nil), r.departments...),
		users:          append([]domain.DirectoryUser(nil), r.users...),
		runs:           append([]domain.DirectorySyncRun(nil), r.runs...),
		failUpsertUser: r.failUpsertUser,
	}
}

func (r *fakeDirectoryRepo) replace(committed *fakeDirectoryRepo) {
	r.departments = append([]domain.DirectoryDepartment(nil), committed.departments...)
	r.users = append([]domain.DirectoryUser(nil), committed.users...)
	r.runs = append([]domain.DirectorySyncRun(nil), committed.runs...)
}
