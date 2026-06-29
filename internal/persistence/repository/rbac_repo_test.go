package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestRBACRepo_SaveFindUpdateAndListAssignment(t *testing.T) {
	resetDB(t)
	createdAt := time.Date(2026, 6, 29, 11, 0, 0, 0, time.UTC)
	var saved domain.RBACAssignment

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		saved, err = uow.RBAC().SaveAssignment(ctx, mustNewRBACAssignment(t, "iam-admin", domain.RBACRoleViewer, true, createdAt))
		if err != nil {
			t.Fatalf("SaveAssignment: %v", err)
		}
	})
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved assignment missing generated fields: %+v", saved)
	}

	updated := saved
	updated.Role = domain.RBACRoleAdmin
	updated.Enabled = false
	updated.UpdatedBy = "owner-2"
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.RBAC().UpdateAssignment(ctx, updated)
		if err != nil {
			t.Fatalf("UpdateAssignment: %v", err)
		}
		if got.ID != saved.ID || got.Role != domain.RBACRoleAdmin || got.Enabled || got.CreatedBy != "owner-1" || got.UpdatedBy != "owner-2" {
			t.Fatalf("updated assignment = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.RBAC().FindAssignmentByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindAssignmentByID: %v", err)
		}
		if got.Role != domain.RBACRoleAdmin || got.UpdatedBy != "owner-2" {
			t.Fatalf("found assignment = %+v", got)
		}
		listed, err := uow.RBAC().ListAssignments(ctx, 10)
		if err != nil {
			t.Fatalf("ListAssignments: %v", err)
		}
		if len(listed) != 1 || listed[0].ID != saved.ID {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestRBACRepo_DuplicateAssignmentReturnsAlreadyExists(t *testing.T) {
	resetDB(t)
	createdAt := time.Date(2026, 6, 29, 11, 30, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.RBAC().SaveAssignment(ctx, mustNewRBACAssignment(t, "iam-admin", domain.RBACRoleAdmin, true, createdAt)); err != nil {
			t.Fatalf("initial save: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.RBAC().SaveAssignment(ctx, mustNewRBACAssignment(t, "iam-admin", domain.RBACRoleAdmin, true, createdAt))
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate err = %v, want ErrAlreadyExists", err)
	}
}

func TestRBACRepo_ListEnabledAssignmentsForPrincipal(t *testing.T) {
	resetDB(t)
	createdAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		userAssignment := mustNewRBACAssignment(t, "iam-user-1", domain.RBACRoleViewer, true, createdAt)
		if _, err := uow.RBAC().SaveAssignment(ctx, userAssignment); err != nil {
			t.Fatalf("save user assignment: %v", err)
		}
		departmentAssignment, err := domain.NewRBACAssignment(
			domain.RBACSubjectKindDepartment,
			"dep-platform",
			domain.RBACRoleResponder,
			domain.RBACScopeKindDiagnosisRoom,
			"room-1",
			true,
		)
		if err != nil {
			t.Fatalf("NewRBACAssignment department: %v", err)
		}
		departmentAssignment.CreatedBy = "owner-1"
		departmentAssignment.UpdatedBy = "owner-1"
		if _, err := uow.RBAC().SaveAssignment(ctx, departmentAssignment); err != nil {
			t.Fatalf("save department assignment: %v", err)
		}
		disabled := mustNewRBACAssignment(t, "iam-user-1", domain.RBACRoleAdmin, false, createdAt)
		disabled.ScopeKind = domain.RBACScopeKindAlertSource
		disabled.ScopeKey = "alert-source-1"
		if _, err := uow.RBAC().SaveAssignment(ctx, disabled); err != nil {
			t.Fatalf("save disabled assignment: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		assignments, err := uow.RBAC().ListEnabledAssignmentsForPrincipal(ctx, domain.RBACPrincipal{
			Subject:        "iam-user-1",
			DepartmentKeys: []string{" dep-platform ", "dep-platform", ""},
		})
		if err != nil {
			t.Fatalf("ListEnabledAssignmentsForPrincipal: %v", err)
		}
		if len(assignments) != 2 {
			t.Fatalf("assignments = %+v, want 2 enabled matches", assignments)
		}
		allowed, err := domain.RBACAuthorize(
			domain.RBACPrincipal{Subject: "iam-user-1", DepartmentKeys: []string{"dep-platform"}},
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
			t.Fatal("RBACAuthorize denied department responder assignment")
		}
	})
}

func TestRBACRepo_NotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.RBAC().FindAssignmentByID(ctx, 404)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing err = %v, want ErrNotFound", err)
		}
		_, err = uow.RBAC().FindAssignmentByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.RBAC().ListAssignments(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.RBAC().ListEnabledAssignmentsForPrincipal(ctx, domain.RBACPrincipal{})
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("principal empty subject err = %v, want ErrInvariantViolation", err)
		}
		assignment := mustNewRBACAssignment(t, "iam-user-1", domain.RBACRoleViewer, true, time.Now().UTC())
		assignment.CreatedBy = ""
		_, err = uow.RBAC().SaveAssignment(ctx, assignment)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("save empty created_by err = %v, want ErrInvariantViolation", err)
		}
		assignment = mustNewRBACAssignment(t, "iam-user-1", domain.RBACRoleViewer, true, time.Now().UTC())
		assignment.ID = 404
		assignment.UpdatedBy = "owner-1"
		_, err = uow.RBAC().UpdateAssignment(ctx, assignment)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing err = %v, want ErrNotFound", err)
		}
	})
}

func mustNewRBACAssignment(t *testing.T, subject string, role domain.RBACRole, enabled bool, createdAt time.Time) domain.RBACAssignment {
	t.Helper()
	assignment, err := domain.NewRBACAssignment(
		domain.RBACSubjectKindUser,
		subject,
		role,
		domain.RBACScopeKindGlobal,
		"",
		enabled,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment: %v", err)
	}
	assignment.CreatedBy = "owner-1"
	assignment.UpdatedBy = "owner-1"
	assignment.CreatedAt = createdAt
	assignment.UpdatedAt = createdAt
	return assignment
}
