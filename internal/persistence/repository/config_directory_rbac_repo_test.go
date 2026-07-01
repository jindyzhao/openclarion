package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestConfigRepo_UpsertAndListDirectoryProjection(t *testing.T) {
	resetDB(t)
	syncedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	sourceUpdatedAt := time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC)
	department, err := domain.NewDirectoryDepartment(
		"ops_iam",
		"dep-2",
		"dep-1",
		"SRE",
		"SRE",
		"IT/Platform/SRE",
		"IT/Platform",
		3,
		"iam",
		4,
		&sourceUpdatedAt,
		syncedAt,
	)
	if err != nil {
		t.Fatalf("NewDirectoryDepartment: %v", err)
	}
	user, err := domain.NewDirectoryUser(
		"ops_iam",
		"iam-user-1",
		"wecom-user-1",
		"alice",
		"Alice",
		"alice@example.test",
		"SRE",
		"IT",
		"Platform/SRE",
		"IT/Platform/SRE",
		[]string{"IT/Platform/SRE", "IT/Shared"},
		[]string{"dep-2"},
		true,
		&sourceUpdatedAt,
		syncedAt,
	)
	if err != nil {
		t.Fatalf("NewDirectoryUser: %v", err)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Config().UpsertDirectoryDepartment(ctx, department); err != nil {
			t.Fatalf("UpsertDirectoryDepartment: %v", err)
		}
		if _, err := uow.Config().UpsertDirectoryUser(ctx, user); err != nil {
			t.Fatalf("UpsertDirectoryUser: %v", err)
		}
	})

	updatedUser := user
	updatedUser.DisplayName = "Alice Updated"
	updatedUser.Active = false
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().UpsertDirectoryUser(ctx, updatedUser)
		if err != nil {
			t.Fatalf("second UpsertDirectoryUser: %v", err)
		}
		if got.ID == 0 || got.DisplayName != "Alice Updated" || got.Active {
			t.Fatalf("updated user = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		gotDepartment, err := uow.Config().FindDirectoryDepartmentByProviderExternalID(ctx, "ops_iam", "dep-2")
		if err != nil {
			t.Fatalf("FindDirectoryDepartmentByProviderExternalID: %v", err)
		}
		if gotDepartment.Path != "IT/Platform/SRE" || gotDepartment.ParentExternalID != "dep-1" {
			t.Fatalf("department = %+v", gotDepartment)
		}
		gotUser, err := uow.Config().FindDirectoryUserByProviderSubject(ctx, "ops_iam", "iam-user-1")
		if err != nil {
			t.Fatalf("FindDirectoryUserByProviderSubject: %v", err)
		}
		if gotUser.DisplayName != "Alice Updated" || gotUser.Active ||
			len(gotUser.DepartmentPaths) != 2 ||
			gotUser.DepartmentPaths[1] != "IT/Shared" {
			t.Fatalf("user = %+v", gotUser)
		}
		subjectUsers, err := uow.Config().ListDirectoryUsersBySubject(ctx, "iam-user-1", 10)
		if err != nil {
			t.Fatalf("ListDirectoryUsersBySubject: %v", err)
		}
		if len(subjectUsers) != 1 ||
			subjectUsers[0].Subject != "iam-user-1" ||
			subjectUsers[0].DisplayName != "Alice Updated" {
			t.Fatalf("subject users = %+v", subjectUsers)
		}
		users, err := uow.Config().ListDirectoryUsers(ctx, "ops_iam", 10)
		if err != nil {
			t.Fatalf("ListDirectoryUsers: %v", err)
		}
		if len(users) != 1 || users[0].Subject != "iam-user-1" {
			t.Fatalf("users = %+v", users)
		}
	})

	run, err := domain.NewDirectorySyncSucceededRun(
		"ops_iam",
		100,
		&sourceUpdatedAt,
		1,
		2,
		3,
		4,
		syncedAt,
	)
	if err != nil {
		t.Fatalf("NewDirectorySyncSucceededRun: %v", err)
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		saved, err := uow.Config().SaveDirectorySyncRun(ctx, run)
		if err != nil {
			t.Fatalf("SaveDirectorySyncRun: %v", err)
		}
		if saved.ID == 0 || saved.UpdatedAfter == nil || !saved.SyncedAt.Equal(syncedAt) {
			t.Fatalf("saved sync run = %+v", saved)
		}
	})
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		runs, err := uow.Config().ListDirectorySyncRuns(ctx, "ops_iam", 10)
		if err != nil {
			t.Fatalf("ListDirectorySyncRuns: %v", err)
		}
		if len(runs) != 1 || runs[0].UsersUpserted != 4 || runs[0].PageSize != 100 || runs[0].Status != domain.DirectorySyncRunStatusSucceeded {
			t.Fatalf("sync runs = %+v", runs)
		}
	})
}

func TestConfigRepo_DeactivateStaleDirectoryUsers(t *testing.T) {
	resetDB(t)
	oldSync := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	currentSync := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	staleUser := mustDirectoryUser(t, "ops_iam", "stale-user", "stale-ext", oldSync, true)
	currentUser := mustDirectoryUser(t, "ops_iam", "current-user", "current-ext", currentSync, true)
	otherProviderUser := mustDirectoryUser(t, "other_iam", "other-user", "other-ext", oldSync, true)
	inactiveUser := mustDirectoryUser(t, "ops_iam", "inactive-user", "inactive-ext", oldSync, false)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for _, user := range []domain.DirectoryUser{staleUser, currentUser, otherProviderUser, inactiveUser} {
			if _, err := uow.Config().UpsertDirectoryUser(ctx, user); err != nil {
				t.Fatalf("UpsertDirectoryUser(%s): %v", user.Subject, err)
			}
		}
		updated, err := uow.Config().DeactivateStaleDirectoryUsers(ctx, "ops_iam", currentSync)
		if err != nil {
			t.Fatalf("DeactivateStaleDirectoryUsers: %v", err)
		}
		if updated != 1 {
			t.Fatalf("updated = %d, want 1", updated)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		stale, err := uow.Config().FindDirectoryUserByProviderSubject(ctx, "ops_iam", "stale-user")
		if err != nil {
			t.Fatalf("Find stale user: %v", err)
		}
		if stale.Active || !stale.SyncedAt.Equal(currentSync) {
			t.Fatalf("stale user = %+v", stale)
		}
		current, err := uow.Config().FindDirectoryUserByProviderSubject(ctx, "ops_iam", "current-user")
		if err != nil {
			t.Fatalf("Find current user: %v", err)
		}
		if !current.Active {
			t.Fatalf("current user should stay active: %+v", current)
		}
		other, err := uow.Config().FindDirectoryUserByProviderSubject(ctx, "other_iam", "other-user")
		if err != nil {
			t.Fatalf("Find other provider user: %v", err)
		}
		if !other.Active {
			t.Fatalf("other provider user should stay active: %+v", other)
		}
	})
}

func TestConfigRepo_UpsertRBACAssignmentRequiresUpdatedBy(t *testing.T) {
	resetDB(t)
	assignment, err := domain.NewRBACAssignment(
		domain.RBACSubjectKindUser,
		"iam-user-1",
		domain.RBACRoleViewer,
		domain.RBACScopeKindGlobal,
		"",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment: %v", err)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Config().UpsertRBACAssignment(ctx, assignment)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("UpsertRBACAssignment error = %v, want ErrInvariantViolation", err)
		}
	})
}

func mustDirectoryUser(t *testing.T, provider, subject, externalID string, syncedAt time.Time, active bool) domain.DirectoryUser {
	t.Helper()
	user, err := domain.NewDirectoryUser(
		provider,
		subject,
		externalID,
		subject,
		subject,
		"",
		"",
		"IT",
		"Platform",
		"IT/Platform",
		[]string{"IT/Platform"},
		[]string{"dep-1"},
		active,
		nil,
		syncedAt,
	)
	if err != nil {
		t.Fatalf("NewDirectoryUser(%s): %v", subject, err)
	}
	return user
}

func TestConfigRepo_UpsertAndListRBACAssignmentsForPrincipal(t *testing.T) {
	resetDB(t)
	userAssignment, err := domain.NewRBACAssignment(
		domain.RBACSubjectKindUser,
		"iam-user-1",
		domain.RBACRoleViewer,
		domain.RBACScopeKindGlobal,
		"",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment user: %v", err)
	}
	departmentAssignment, err := domain.NewRBACAssignment(
		domain.RBACSubjectKindDepartment,
		"dep-2",
		domain.RBACRoleResponder,
		domain.RBACScopeKindDiagnosisRoom,
		"room-1",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment department: %v", err)
	}
	disabledAssignment, err := domain.NewRBACAssignment(
		domain.RBACSubjectKindUser,
		"iam-user-1",
		domain.RBACRoleAdmin,
		domain.RBACScopeKindGlobal,
		"",
		false,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment disabled: %v", err)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for _, assignment := range []domain.RBACAssignment{userAssignment, departmentAssignment, disabledAssignment} {
			assignment.UpdatedBy = "iam-admin-1"
			if _, err := uow.Config().UpsertRBACAssignment(ctx, assignment); err != nil {
				t.Fatalf("UpsertRBACAssignment: %v", err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		allAssignments, err := uow.Config().ListRBACAssignments(ctx, 10)
		if err != nil {
			t.Fatalf("ListRBACAssignments: %v", err)
		}
		if len(allAssignments) != 3 {
			t.Fatalf("all assignments = %+v", allAssignments)
		}
		hasDisabledAdmin := false
		for _, assignment := range allAssignments {
			if assignment.Role == domain.RBACRoleAdmin && !assignment.Enabled {
				hasDisabledAdmin = true
			}
			if assignment.CreatedBy != "iam-admin-1" || assignment.UpdatedBy != "iam-admin-1" {
				t.Fatalf("assignment audit = %+v", assignment)
			}
		}
		if !hasDisabledAdmin {
			t.Fatalf("all assignments should include disabled admin assignment: %+v", allAssignments)
		}

		assignments, err := uow.Config().ListRBACAssignmentsForPrincipal(ctx, "iam-user-1", []string{"dep-2"}, 10)
		if err != nil {
			t.Fatalf("ListRBACAssignmentsForPrincipal: %v", err)
		}
		if len(assignments) != 2 {
			t.Fatalf("assignments = %+v", assignments)
		}
		allowed, err := domain.RBACAuthorize(
			domain.RBACPrincipal{Subject: "iam-user-1", DepartmentKeys: []string{"dep-2"}},
			domain.RBACRequest{
				Permission: domain.RBACPermissionDiagnosisRoomParticipate,
				ScopeKind:  domain.RBACScopeKindDiagnosisRoom,
				ScopeKey:   "room-1",
			},
			assignments,
		)
		if err != nil {
			t.Fatalf("RBACAuthorize: %v", err)
		}
		if !allowed {
			t.Fatalf("department assignment should allow room participation")
		}
	})
}
