// Package rbac owns OpenClarion-local authorization checks.
package rbac

import (
	"context"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const defaultAssignmentLimit = 1000

// AuthorizeRequest describes one local authorization check.
type AuthorizeRequest struct {
	Principal  domain.RBACPrincipal
	Permission domain.RBACPermission
	ScopeKind  domain.RBACScopeKind
	ScopeKey   string
}

// AuthorizeDecision records the local RBAC result and the time it was checked.
type AuthorizeDecision struct {
	Allowed   bool
	CheckedAt time.Time
}

// Service coordinates local RBAC repository reads and domain authorization.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	now        func() time.Time
}

// Option customizes the RBAC service.
type Option func(*Service)

// WithClock injects the decision timestamp clock.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// NewService constructs an RBAC service.
func NewService(uowFactory ports.UnitOfWorkFactory, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("rbac: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	svc := &Service{
		uowFactory: uowFactory,
		now:        time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc, nil
}

// Authorize evaluates OpenClarion-local role assignments for one request.
func (s *Service) Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeDecision, error) {
	if s == nil {
		return AuthorizeDecision{}, fmt.Errorf("rbac: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	checkedAt := domain.NormalizeUTCMicro(s.now())
	if checkedAt.IsZero() {
		return AuthorizeDecision{}, fmt.Errorf("rbac: clock returned zero time: %w", domain.ErrInvariantViolation)
	}
	var assignments []domain.RBACAssignment
	if err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		assignments, err = uow.Config().ListRBACAssignmentsForPrincipal(ctx, req.Principal.Subject, req.Principal.DepartmentKeys, defaultAssignmentLimit)
		return err
	}); err != nil {
		return AuthorizeDecision{}, err
	}
	allowed, err := domain.RBACAuthorize(req.Principal, domain.RBACRequest{
		Permission: req.Permission,
		ScopeKind:  req.ScopeKind,
		ScopeKey:   req.ScopeKey,
	}, assignments)
	if err != nil {
		return AuthorizeDecision{}, err
	}
	return AuthorizeDecision{Allowed: allowed, CheckedAt: checkedAt}, nil
}
