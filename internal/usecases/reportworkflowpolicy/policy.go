// Package reportworkflowpolicy owns report workflow policy persistence and
// explicit enablement actions.
package reportworkflowpolicy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// WriteRequest describes mutable report workflow policy metadata. Enabled
// state is intentionally excluded; callers must use explicit actions.
type WriteRequest struct {
	Name                               string
	AlertSourceProfileID               domain.AlertSourceProfileID
	GroupingPolicyID                   domain.GroupingPolicyID
	ReportNotificationChannelProfileID domain.NotificationChannelProfileID
	MaxFailedSubReports                int
	TriggerMode                        domain.ReportWorkflowTriggerMode
	ReportScenario                     domain.ReportWorkflowScenario
	DiagnosisFollowUp                  domain.DiagnosisFollowUpMode
}

// ActionRequest identifies one report workflow policy enablement action.
type ActionRequest struct {
	PolicyID domain.ReportWorkflowPolicyID
}

// Service coordinates report workflow policy configuration updates.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	now        func() time.Time
}

// Option customizes report workflow policy service behavior.
type Option func(*Service)

// WithClock injects the clock used for explicit enablement timestamps.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// NewService constructs a report workflow policy service.
func NewService(uowFactory ports.UnitOfWorkFactory, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("report workflow policy: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
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

// Create stores a disabled report workflow policy draft.
func (s *Service) Create(ctx context.Context, req WriteRequest) (domain.ReportWorkflowPolicy, error) {
	if s == nil {
		return domain.ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	policy, err := policyFromWriteRequest(req, false, nil, nil)
	if err != nil {
		return domain.ReportWorkflowPolicy{}, err
	}

	var saved domain.ReportWorkflowPolicy
	err = s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if err := requireBoundProfilesExist(ctx, uow, policy); err != nil {
			return err
		}
		var serr error
		saved, serr = uow.Config().SaveReportWorkflowPolicy(ctx, policy)
		return serr
	})
	return saved, err
}

// Replace updates policy metadata while preserving explicit enablement state.
func (s *Service) Replace(ctx context.Context, policyID domain.ReportWorkflowPolicyID, req WriteRequest) (domain.ReportWorkflowPolicy, error) {
	if s == nil {
		return domain.ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if policyID == 0 {
		return domain.ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: id must be non-zero: %w", domain.ErrInvariantViolation)
	}

	var saved domain.ReportWorkflowPolicy
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Config().FindReportWorkflowPolicyByID(ctx, policyID)
		if err != nil {
			return err
		}
		policy, err := policyFromWriteRequest(req, existing.Enabled, existing.EnabledAt, existing.DisabledAt)
		if err != nil {
			return err
		}
		policy.ID = policyID
		if existing.Enabled {
			if err := requireBoundProfilesEnabled(ctx, uow, policy); err != nil {
				return err
			}
		} else if err := requireBoundProfilesExist(ctx, uow, policy); err != nil {
			return err
		}
		var uerr error
		saved, uerr = uow.Config().UpdateReportWorkflowPolicy(ctx, policy)
		return uerr
	})
	return saved, err
}

// Enable explicitly enables a report workflow policy after validating bound
// profile readiness. It does not start a report workflow.
func (s *Service) Enable(ctx context.Context, req ActionRequest) (domain.ReportWorkflowPolicy, error) {
	return s.setEnabled(ctx, req.PolicyID, true)
}

// Disable explicitly disables a report workflow policy. It does not cancel or
// start report workflows.
func (s *Service) Disable(ctx context.Context, req ActionRequest) (domain.ReportWorkflowPolicy, error) {
	return s.setEnabled(ctx, req.PolicyID, false)
}

func (s *Service) setEnabled(ctx context.Context, policyID domain.ReportWorkflowPolicyID, enabled bool) (domain.ReportWorkflowPolicy, error) {
	if s == nil {
		return domain.ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if policyID == 0 {
		return domain.ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if s.now == nil {
		return domain.ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: clock must be configured: %w", domain.ErrInvariantViolation)
	}

	var saved domain.ReportWorkflowPolicy
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		policy, err := uow.Config().FindReportWorkflowPolicyByID(ctx, policyID)
		if err != nil {
			return err
		}
		if enabled {
			if err := requireBoundProfilesEnabled(ctx, uow, policy); err != nil {
				return err
			}
		}
		updated, err := domain.WithReportWorkflowPolicyEnabled(policy, enabled, s.now())
		if err != nil {
			return err
		}
		var uerr error
		saved, uerr = uow.Config().UpdateReportWorkflowPolicy(ctx, updated)
		return uerr
	})
	return saved, err
}

func policyFromWriteRequest(
	req WriteRequest,
	enabled bool,
	enabledAt *time.Time,
	disabledAt *time.Time,
) (domain.ReportWorkflowPolicy, error) {
	triggerMode := req.TriggerMode
	if triggerMode == "" {
		triggerMode = domain.ReportWorkflowTriggerModeManualReplay
	}
	reportScenario := req.ReportScenario
	if reportScenario == "" {
		reportScenario = domain.ReportWorkflowScenarioSingleAlert
	}
	diagnosisFollowUp := req.DiagnosisFollowUp
	if diagnosisFollowUp == "" {
		diagnosisFollowUp = domain.DiagnosisFollowUpModeDisabled
	}
	return domain.NewReportWorkflowPolicy(
		req.Name,
		req.AlertSourceProfileID,
		req.GroupingPolicyID,
		req.ReportNotificationChannelProfileID,
		req.MaxFailedSubReports,
		triggerMode,
		reportScenario,
		diagnosisFollowUp,
		enabled,
		enabledAt,
		disabledAt,
	)
}

func requireBoundProfilesExist(ctx context.Context, uow ports.UnitOfWork, policy domain.ReportWorkflowPolicy) error {
	if _, err := uow.Config().FindAlertSourceProfileByID(ctx, policy.AlertSourceProfileID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("report workflow policy: alert source profile not found: %w", domain.ErrNotFound)
		}
		return err
	}
	if _, err := uow.Config().FindGroupingPolicyByID(ctx, policy.GroupingPolicyID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("report workflow policy: grouping policy not found: %w", domain.ErrNotFound)
		}
		return err
	}
	if policy.ReportNotificationChannelProfileID != 0 {
		if _, err := uow.Config().FindNotificationChannelProfileByID(ctx, policy.ReportNotificationChannelProfileID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("report workflow policy: notification channel profile not found: %w", domain.ErrNotFound)
			}
			return err
		}
	}
	return nil
}

func requireBoundProfilesEnabled(ctx context.Context, uow ports.UnitOfWork, policy domain.ReportWorkflowPolicy) error {
	source, err := uow.Config().FindAlertSourceProfileByID(ctx, policy.AlertSourceProfileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("report workflow policy: alert source profile not found: %w", domain.ErrNotFound)
		}
		return err
	}
	if !source.Enabled {
		return fmt.Errorf("report workflow policy: alert source profile must be enabled before workflow policy enablement: %w", domain.ErrInvariantViolation)
	}
	if policy.DiagnosisFollowUp == domain.DiagnosisFollowUpModeAutoRoom &&
		source.Kind != domain.AlertSourceKindAlertmanager {
		return fmt.Errorf("report workflow policy: auto-room workflow policy requires an alertmanager alert source before workflow policy enablement: %w", domain.ErrInvariantViolation)
	}
	if policy.DiagnosisFollowUp == domain.DiagnosisFollowUpModeAutoRoom &&
		policy.ReportNotificationChannelProfileID == 0 {
		return fmt.Errorf("report workflow policy: auto-room workflow policy requires a notification channel profile before workflow policy enablement: %w", domain.ErrInvariantViolation)
	}
	grouping, err := uow.Config().FindGroupingPolicyByID(ctx, policy.GroupingPolicyID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("report workflow policy: grouping policy not found: %w", domain.ErrNotFound)
		}
		return err
	}
	if !grouping.Enabled {
		return fmt.Errorf("report workflow policy: grouping policy must be enabled before workflow policy enablement: %w", domain.ErrInvariantViolation)
	}
	if policy.ReportNotificationChannelProfileID != 0 {
		channel, err := uow.Config().FindNotificationChannelProfileByID(ctx, policy.ReportNotificationChannelProfileID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("report workflow policy: notification channel profile not found: %w", domain.ErrNotFound)
			}
			return err
		}
		if !channel.Enabled {
			return fmt.Errorf("report workflow policy: notification channel profile must be enabled before workflow policy enablement: %w", domain.ErrInvariantViolation)
		}
		if !notificationChannelSupportsReport(channel) {
			return fmt.Errorf("report workflow policy: notification channel profile must include report delivery scope before workflow policy enablement: %w", domain.ErrInvariantViolation)
		}
		if policy.DiagnosisFollowUp == domain.DiagnosisFollowUpModeAutoRoom &&
			channel.Kind != domain.NotificationChannelKindWeCom {
			return fmt.Errorf("report workflow policy: notification channel profile must be an Enterprise WeChat channel before auto-room workflow policy enablement: %w", domain.ErrInvariantViolation)
		}
		if policy.DiagnosisFollowUp == domain.DiagnosisFollowUpModeAutoRoom &&
			!notificationChannelSupportsScope(channel, domain.NotificationDeliveryScopeDiagnosisConsultation) {
			return fmt.Errorf("report workflow policy: notification channel profile must include diagnosis_consultation delivery scope before auto-room workflow policy enablement: %w", domain.ErrInvariantViolation)
		}
		if policy.DiagnosisFollowUp == domain.DiagnosisFollowUpModeAutoRoom &&
			!notificationChannelSupportsScope(channel, domain.NotificationDeliveryScopeDiagnosisClose) {
			return fmt.Errorf("report workflow policy: notification channel profile must include diagnosis_close delivery scope before auto-room workflow policy enablement: %w", domain.ErrInvariantViolation)
		}
		if policy.DiagnosisFollowUp == domain.DiagnosisFollowUpModeAutoRoom {
			if missingProofs := channel.MissingAIDiagnosisProofContentKinds(); len(missingProofs) > 0 {
				return fmt.Errorf("report workflow policy: notification channel profile must have current AI delivery test proof for %s before auto-room workflow policy enablement: %w", notificationProofKindList(missingProofs), domain.ErrInvariantViolation)
			}
		}
	}
	return nil
}

func notificationChannelSupportsReport(channel domain.NotificationChannelProfile) bool {
	return notificationChannelSupportsScope(channel, domain.NotificationDeliveryScopeReport)
}

func notificationChannelSupportsScope(channel domain.NotificationChannelProfile, want domain.NotificationDeliveryScope) bool {
	for _, scope := range channel.DeliveryScopes {
		if scope == want {
			return true
		}
	}
	return false
}

func notificationProofKindList(kinds []domain.NotificationChannelTestContentKind) string {
	if len(kinds) == 0 {
		return ""
	}
	out := make([]string, len(kinds))
	for i, kind := range kinds {
		out[i] = string(kind)
	}
	return strings.Join(out, ",")
}
