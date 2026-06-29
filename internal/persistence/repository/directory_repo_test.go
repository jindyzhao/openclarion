package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestDirectoryRepo_UpsertFindAndListDepartment(t *testing.T) {
	resetDB(t)
	syncedAt := time.Date(2026, 6, 29, 8, 0, 0, 123456789, time.UTC)
	sourceUpdatedAt := syncedAt.Add(-time.Hour)
	var saved domain.DirectoryDepartment

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		saved, err = uow.Directory().UpsertDepartment(ctx, mustNewDirectoryDepartment(t, "dep-sre", "SRE", "Technology/SRE", &sourceUpdatedAt, syncedAt))
		if err != nil {
			t.Fatalf("UpsertDepartment create: %v", err)
		}
	})
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved department missing generated fields: %+v", saved)
	}
	if !saved.SyncedAt.Equal(domain.NormalizeUTCMicro(syncedAt)) {
		t.Fatalf("SyncedAt = %s, want microsecond normalized", saved.SyncedAt)
	}

	updated := mustNewDirectoryDepartment(t, "dep-sre", "Platform SRE", "Technology/Platform SRE", nil, syncedAt.Add(time.Minute))
	updated.ParentExternalID = "dep-tech"
	updated.ParentPath = "Technology"
	updated.MemberCount = 12
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Directory().UpsertDepartment(ctx, updated)
		if err != nil {
			t.Fatalf("UpsertDepartment update: %v", err)
		}
		if got.ID != saved.ID || got.DisplayName != "Platform SRE" || got.MemberCount != 12 || got.SourceUpdatedAt != nil {
			t.Fatalf("updated department = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Directory().FindDepartmentByExternalID(ctx, "ops_iam", "dep-sre")
		if err != nil {
			t.Fatalf("FindDepartmentByExternalID: %v", err)
		}
		if got.ID != saved.ID || got.ParentExternalID != "dep-tech" {
			t.Fatalf("got department = %+v", got)
		}
		listed, err := uow.Directory().ListDepartments(ctx, "ops_iam", 10)
		if err != nil {
			t.Fatalf("ListDepartments: %v", err)
		}
		if len(listed) != 1 || listed[0].ExternalID != "dep-sre" {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestDirectoryRepo_UpsertFindAndListUser(t *testing.T) {
	resetDB(t)
	syncedAt := time.Date(2026, 6, 29, 8, 30, 0, 0, time.UTC)
	var saved domain.DirectoryUser

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		saved, err = uow.Directory().UpsertUser(ctx, mustNewDirectoryUser(t, "iam-user-1", "User One", true, syncedAt))
		if err != nil {
			t.Fatalf("UpsertUser create: %v", err)
		}
	})
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved user missing generated fields: %+v", saved)
	}
	if len(saved.DepartmentExternalIDs) != 2 || saved.DepartmentExternalIDs[0] != "dep-platform" {
		t.Fatalf("DepartmentExternalIDs = %+v", saved.DepartmentExternalIDs)
	}

	updated := mustNewDirectoryUser(t, "iam-user-1", "User One Disabled", false, syncedAt.Add(time.Minute))
	updated.Email = ""
	updated.DepartmentExternalIDs = []string{"dep-platform"}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Directory().UpsertUser(ctx, updated)
		if err != nil {
			t.Fatalf("UpsertUser update: %v", err)
		}
		if got.ID != saved.ID || got.Active || got.Email != "" || got.DisplayName != "User One Disabled" {
			t.Fatalf("updated user = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		bySubject, err := uow.Directory().FindUserBySubject(ctx, "ops_iam", "iam-user-1")
		if err != nil {
			t.Fatalf("FindUserBySubject: %v", err)
		}
		byExternalID, err := uow.Directory().FindUserByExternalID(ctx, "ops_iam", "ext-iam-user-1")
		if err != nil {
			t.Fatalf("FindUserByExternalID: %v", err)
		}
		if bySubject.ID != saved.ID || byExternalID.ID != saved.ID {
			t.Fatalf("lookup mismatch: subject=%+v external=%+v", bySubject, byExternalID)
		}
		activeOnly, err := uow.Directory().ListUsers(ctx, "ops_iam", true, 10)
		if err != nil {
			t.Fatalf("ListUsers active: %v", err)
		}
		if len(activeOnly) != 0 {
			t.Fatalf("activeOnly = %+v, want none", activeOnly)
		}
		allUsers, err := uow.Directory().ListUsers(ctx, "ops_iam", false, 10)
		if err != nil {
			t.Fatalf("ListUsers all: %v", err)
		}
		if len(allUsers) != 1 || allUsers[0].ID != saved.ID {
			t.Fatalf("allUsers = %+v", allUsers)
		}
	})
}

func TestDirectoryRepo_UserExternalIDConflictReturnsAlreadyExists(t *testing.T) {
	resetDB(t)
	syncedAt := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Directory().UpsertUser(ctx, mustNewDirectoryUser(t, "iam-user-1", "User One", true, syncedAt)); err != nil {
			t.Fatalf("initial upsert: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		conflicting := mustNewDirectoryUser(t, "iam-user-2", "User Two", true, syncedAt)
		conflicting.ExternalID = "ext-iam-user-1"
		_, serr := uow.Directory().UpsertUser(ctx, conflicting)
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("conflict err = %v, want ErrAlreadyExists", err)
	}
}

func TestDirectoryRepo_ConcurrentUpsertsAreIdempotent(t *testing.T) {
	resetDB(t)
	syncedAt := time.Date(2026, 6, 29, 9, 30, 0, 0, time.UTC)
	department := mustNewDirectoryDepartment(t, "dep-concurrent", "Concurrent", "Technology/Concurrent", nil, syncedAt)
	user := mustNewDirectoryUser(t, "iam-concurrent", "Concurrent User", true, syncedAt)
	const workers = 12
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
				if _, err := uow.Directory().UpsertDepartment(ctx, department); err != nil {
					return fmt.Errorf("upsert department worker %d: %w", worker, err)
				}
				if _, err := uow.Directory().UpsertUser(ctx, user); err != nil {
					return fmt.Errorf("upsert user worker %d: %w", worker, err)
				}
				return nil
			})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent upsert returned error: %v", err)
		}
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		departments, err := uow.Directory().ListDepartments(ctx, "ops_iam", 10)
		if err != nil {
			t.Fatalf("ListDepartments: %v", err)
		}
		if len(departments) != 1 || departments[0].ExternalID != "dep-concurrent" {
			t.Fatalf("departments = %+v, want exactly one concurrent department", departments)
		}
		users, err := uow.Directory().ListUsers(ctx, "ops_iam", false, 10)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if len(users) != 1 || users[0].Subject != "iam-concurrent" {
			t.Fatalf("users = %+v, want exactly one concurrent user", users)
		}
	})
}

func TestDirectoryRepo_SaveAndListSyncRuns(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	updatedAfter := base.Add(-24 * time.Hour)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		older, err := domain.NewDirectorySyncSucceededRun("ops_iam", 200, &updatedAfter, 1, 2, 3, 4, base)
		if err != nil {
			t.Fatalf("NewDirectorySyncSucceededRun older: %v", err)
		}
		if _, err := uow.Directory().SaveSyncRun(ctx, older); err != nil {
			t.Fatalf("SaveSyncRun older: %v", err)
		}
		newer, err := domain.NewDirectorySyncFailedRun("ops_iam", 200, &updatedAfter, "provider_timeout", "provider request timed out", 1, 1, 2, 2, base.Add(time.Minute))
		if err != nil {
			t.Fatalf("NewDirectorySyncFailedRun newer: %v", err)
		}
		if _, err := uow.Directory().SaveSyncRun(ctx, newer); err != nil {
			t.Fatalf("SaveSyncRun newer: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Directory().ListSyncRuns(ctx, "ops_iam", 1)
		if err != nil {
			t.Fatalf("ListSyncRuns: %v", err)
		}
		if len(listed) != 1 || listed[0].Status != domain.DirectorySyncRunStatusFailed || listed[0].UpdatedAfter == nil {
			t.Fatalf("listed sync runs = %+v", listed)
		}
	})
}

func TestDirectoryRepo_NotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Directory().FindDepartmentByExternalID(ctx, "ops_iam", "missing")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing department err = %v, want ErrNotFound", err)
		}
		_, err = uow.Directory().FindUserBySubject(ctx, "", "iam-user-1")
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find user empty provider err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Directory().ListUsers(ctx, "ops_iam", false, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list users zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Directory().ListSyncRuns(ctx, "", 10)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list sync runs empty provider err = %v, want ErrInvariantViolation", err)
		}
	})
}

func mustNewDirectoryDepartment(t *testing.T, externalID, displayName, path string, sourceUpdatedAt *time.Time, syncedAt time.Time) domain.DirectoryDepartment {
	t.Helper()
	department, err := domain.NewDirectoryDepartment(
		"ops_iam",
		externalID,
		"",
		displayName,
		displayName,
		path,
		"",
		2,
		"iam",
		7,
		sourceUpdatedAt,
		syncedAt,
	)
	if err != nil {
		t.Fatalf("NewDirectoryDepartment: %v", err)
	}
	return department
}

func mustNewDirectoryUser(t *testing.T, subject, displayName string, active bool, syncedAt time.Time) domain.DirectoryUser {
	t.Helper()
	user, err := domain.NewDirectoryUser(
		"ops_iam",
		subject,
		"ext-"+subject,
		subject,
		displayName,
		subject+"@example.com",
		"SRE",
		"Technology",
		"Platform",
		"Technology/Platform",
		[]string{"Technology/Platform", "Technology"},
		[]string{"dep-platform", "dep-tech"},
		active,
		nil,
		syncedAt,
	)
	if err != nil {
		t.Fatalf("NewDirectoryUser: %v", err)
	}
	return user
}
