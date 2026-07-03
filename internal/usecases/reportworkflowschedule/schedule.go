// Package reportworkflowschedule owns report workflow schedule persistence and
// explicit enablement actions. Runtime adapters may synchronize saved state to
// Temporal outside this usecase.
package reportworkflowschedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// WriteRequest describes mutable report workflow schedule metadata. Enabled
// state is intentionally excluded; callers must use explicit actions.
type WriteRequest struct {
	Name                   string
	ReportWorkflowPolicyID domain.ReportWorkflowPolicyID
	TemporalScheduleID     string
	Interval               time.Duration
	Offset                 time.Duration
	ReplayWindow           time.Duration
	ReplayDelay            time.Duration
	ReplayLimit            int
	CatchupWindow          time.Duration
}

// ActionRequest identifies one report workflow schedule enablement action.
type ActionRequest struct {
	ScheduleID domain.ReportWorkflowScheduleID
}

// Service coordinates report workflow schedule configuration updates.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	now        func() time.Time
}

// Option customizes report workflow schedule service behavior.
type Option func(*Service)

// WithClock injects the clock used for explicit enablement timestamps.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// NewService constructs a report workflow schedule service.
func NewService(uowFactory ports.UnitOfWorkFactory, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("report workflow schedule: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	svc := &Service{uowFactory: uowFactory}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc, nil
}

// WithClock returns the service with a deterministic clock for tests.
func (s *Service) WithClock(now func() time.Time) *Service {
	if s != nil {
		WithClock(now)(s)
	}
	return s
}

// Create stores a disabled report workflow schedule draft.
func (s *Service) Create(ctx context.Context, req WriteRequest) (domain.ReportWorkflowSchedule, error) {
	if s == nil {
		return domain.ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	schedule, err := scheduleFromWriteRequest(req, false, nil, nil)
	if err != nil {
		return domain.ReportWorkflowSchedule{}, err
	}

	var saved domain.ReportWorkflowSchedule
	err = s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if err := requireReportWorkflowPolicyExists(ctx, uow, schedule.ReportWorkflowPolicyID); err != nil {
			return err
		}
		var serr error
		saved, serr = uow.Config().SaveReportWorkflowSchedule(ctx, schedule)
		return serr
	})
	return saved, err
}

// Replace updates schedule metadata while preserving explicit enablement state.
func (s *Service) Replace(ctx context.Context, scheduleID domain.ReportWorkflowScheduleID, req WriteRequest) (domain.ReportWorkflowSchedule, error) {
	if s == nil {
		return domain.ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if scheduleID == 0 {
		return domain.ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: id must be non-zero: %w", domain.ErrInvariantViolation)
	}

	var saved domain.ReportWorkflowSchedule
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Config().FindReportWorkflowScheduleByID(ctx, scheduleID)
		if err != nil {
			return err
		}
		schedule, err := scheduleFromWriteRequest(req, existing.Enabled, existing.EnabledAt, existing.DisabledAt)
		if err != nil {
			return err
		}
		schedule.ID = scheduleID
		if existing.Enabled {
			if err := requireReportWorkflowPolicyEnabled(ctx, uow, schedule.ReportWorkflowPolicyID); err != nil {
				return err
			}
		} else if err := requireReportWorkflowPolicyExists(ctx, uow, schedule.ReportWorkflowPolicyID); err != nil {
			return err
		}
		var uerr error
		saved, uerr = uow.Config().UpdateReportWorkflowSchedule(ctx, schedule)
		return uerr
	})
	return saved, err
}

// Enable explicitly enables a report workflow schedule after validating bound
// policy readiness. The usecase updates persisted state only; runtime adapters
// may synchronize the saved state to Temporal.
func (s *Service) Enable(ctx context.Context, req ActionRequest) (domain.ReportWorkflowSchedule, error) {
	return s.setEnabled(ctx, req.ScheduleID, true)
}

// Disable explicitly disables a report workflow schedule. The usecase updates
// persisted state only and does not cancel already-started report workflows.
func (s *Service) Disable(ctx context.Context, req ActionRequest) (domain.ReportWorkflowSchedule, error) {
	return s.setEnabled(ctx, req.ScheduleID, false)
}

func (s *Service) setEnabled(ctx context.Context, scheduleID domain.ReportWorkflowScheduleID, enabled bool) (domain.ReportWorkflowSchedule, error) {
	if s == nil {
		return domain.ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if scheduleID == 0 {
		return domain.ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if s.now == nil {
		return domain.ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: clock must be configured: %w", domain.ErrInvariantViolation)
	}

	var saved domain.ReportWorkflowSchedule
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		schedule, err := uow.Config().FindReportWorkflowScheduleByID(ctx, scheduleID)
		if err != nil {
			return err
		}
		if enabled {
			if err := requireReportWorkflowPolicyEnabled(ctx, uow, schedule.ReportWorkflowPolicyID); err != nil {
				return err
			}
		}
		updated, err := domain.WithReportWorkflowScheduleEnabled(schedule, enabled, s.now())
		if err != nil {
			return err
		}
		var uerr error
		saved, uerr = uow.Config().UpdateReportWorkflowSchedule(ctx, updated)
		return uerr
	})
	return saved, err
}

func scheduleFromWriteRequest(
	req WriteRequest,
	enabled bool,
	enabledAt *time.Time,
	disabledAt *time.Time,
) (domain.ReportWorkflowSchedule, error) {
	return domain.NewReportWorkflowSchedule(
		req.Name,
		req.ReportWorkflowPolicyID,
		req.TemporalScheduleID,
		req.Interval,
		req.Offset,
		req.ReplayWindow,
		req.ReplayDelay,
		req.ReplayLimit,
		req.CatchupWindow,
		enabled,
		enabledAt,
		disabledAt,
	)
}

func requireReportWorkflowPolicyExists(ctx context.Context, uow ports.UnitOfWork, policyID domain.ReportWorkflowPolicyID) error {
	if _, err := uow.Config().FindReportWorkflowPolicyByID(ctx, policyID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("report workflow schedule: report workflow policy not found: %w", domain.ErrNotFound)
		}
		return err
	}
	return nil
}

func requireReportWorkflowPolicyEnabled(ctx context.Context, uow ports.UnitOfWork, policyID domain.ReportWorkflowPolicyID) error {
	policy, err := uow.Config().FindReportWorkflowPolicyByID(ctx, policyID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("report workflow schedule: report workflow policy not found: %w", domain.ErrNotFound)
		}
		return err
	}
	if !policy.Enabled {
		return fmt.Errorf("report workflow schedule: report workflow policy must be enabled before schedule enablement: %w", domain.ErrInvariantViolation)
	}
	return nil
}
