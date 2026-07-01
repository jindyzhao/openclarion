package rbac

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceAuthorizeUsesPrincipalAssignments(t *testing.T) {
	assignment, err := domain.NewRBACAssignment(
		domain.RBACSubjectKindDepartment,
		"dep-2",
		domain.RBACRoleResponder,
		domain.RBACScopeKindDiagnosisRoom,
		"room-1",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment: %v", err)
	}
	config := &fakeConfigRepository{assignments: []domain.RBACAssignment{assignment}}
	checkedAt := time.Date(2026, 6, 26, 12, 30, 0, 123456789, time.UTC)
	svc, err := NewService(fakeFactory{config: config}, WithClock(func() time.Time { return checkedAt }))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	decision, err := svc.Authorize(context.Background(), AuthorizeRequest{
		Principal: domain.RBACPrincipal{
			Subject:        "iam-user-1",
			DepartmentKeys: []string{"dep-2"},
		},
		Permission: domain.RBACPermissionDiagnosisRoomParticipate,
		ScopeKind:  domain.RBACScopeKindDiagnosisRoom,
		ScopeKey:   "room-1",
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("decision = %+v, want allowed", decision)
	}
	if decision.CheckedAt.Nanosecond() != 123456000 {
		t.Fatalf("checked_at = %s, want microsecond normalized", decision.CheckedAt)
	}
	if config.gotSubject != "iam-user-1" || len(config.gotDepartments) != 1 || config.gotDepartments[0] != "dep-2" {
		t.Fatalf("principal query subject=%q departments=%#v", config.gotSubject, config.gotDepartments)
	}
}

type fakeFactory struct {
	config *fakeConfigRepository
}

func (f fakeFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, errors.New("not implemented")
}

func (f fakeFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, fakeUOW{config: f.config})
}

type fakeUOW struct {
	ports.UnitOfWork
	config *fakeConfigRepository
}

func (u fakeUOW) Config() ports.ConfigurationRepository {
	return u.config
}

type fakeConfigRepository struct {
	ports.ConfigurationRepository
	assignments    []domain.RBACAssignment
	gotSubject     string
	gotDepartments []string
}

func (r *fakeConfigRepository) ListRBACAssignmentsForPrincipal(_ context.Context, subject string, departmentKeys []string, _ int) ([]domain.RBACAssignment, error) {
	r.gotSubject = subject
	r.gotDepartments = append([]string(nil), departmentKeys...)
	return append([]domain.RBACAssignment(nil), r.assignments...), nil
}
