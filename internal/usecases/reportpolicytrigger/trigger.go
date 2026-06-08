// Package reportpolicytrigger starts report replay from enabled report workflow
// policies and their bound alert source/grouping profiles.
package reportpolicytrigger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

// CreatedByWorkflow stamps snapshots produced by policy-driven manual replay.
const CreatedByWorkflow = "ReportWorkflowPolicyManualReplay"

// Request identifies one explicit manual replay action for a persisted policy.
type Request struct {
	PolicyID       domain.ReportWorkflowPolicyID
	WindowStart    time.Time
	WindowEnd      time.Time
	Limit          int
	CorrelationKey string
	WorkflowID     string

	// CreatedByWorkflow optionally overrides the snapshot audit source.
	// Empty keeps the historical manual replay value.
	CreatedByWorkflow string
}

// ReplayAndStartFunc is the report trigger function used after policy
// resolution. Tests override it to assert the exact immutable request boundary
// without running alert replay persistence.
type ReplayAndStartFunc func(
	ctx context.Context,
	provider ports.MetricsProvider,
	factory ports.UnitOfWorkFactory,
	starter ports.ReportWorkflowStarter,
	req reporttrigger.Request,
) (reporttrigger.Result, error)

// Result returns the replay/start result plus the immutable policy metadata
// that was resolved for the request.
type Result struct {
	Trigger reporttrigger.Result
	Policy  domain.ReportWorkflowPolicy
}

// Service resolves enabled configuration and starts policy-driven report replay.
type Service struct {
	uowFactory     ports.UnitOfWorkFactory
	starter        ports.ReportWorkflowStarter
	providers      *alertsourceprovider.Builder
	replayAndStart ReplayAndStartFunc
}

// Option customizes Service construction.
type Option func(*Service)

// WithReplayAndStart overrides the replay/start function for tests.
func WithReplayAndStart(fn ReplayAndStartFunc) Option {
	return func(s *Service) {
		if fn != nil {
			s.replayAndStart = fn
		}
	}
}

// NewService constructs a policy-driven report replay service.
func NewService(
	uowFactory ports.UnitOfWorkFactory,
	starter ports.ReportWorkflowStarter,
	providers *alertsourceprovider.Builder,
	opts ...Option,
) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("report policy trigger: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if starter == nil {
		return nil, fmt.Errorf("report policy trigger: report workflow starter must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if providers == nil {
		return nil, fmt.Errorf("report policy trigger: alert source provider builder must be non-nil: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		uowFactory:     uowFactory,
		starter:        starter,
		providers:      providers,
		replayAndStart: reporttrigger.ReplayAndStart,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service, nil
}

// ReplayAndStart resolves policy/source/grouping state before starting replay.
// The Temporal workflow never reads mutable configuration directly.
func (s *Service) ReplayAndStart(ctx context.Context, req Request) (reporttrigger.Result, error) {
	result, err := s.ReplayAndStartDetailed(ctx, req)
	if err != nil {
		return reporttrigger.Result{}, err
	}
	return result.Trigger, nil
}

// ReplayAndStartDetailed resolves policy/source/grouping state before starting
// replay and returns the effective policy metadata used for the start.
func (s *Service) ReplayAndStartDetailed(ctx context.Context, req Request) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("report policy trigger: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if err := validateRequest(req); err != nil {
		return Result{}, err
	}

	binding, err := s.loadBinding(ctx, req.PolicyID)
	if err != nil {
		return Result{}, err
	}
	provider, err := s.providers.Build(ctx, binding.source)
	if err != nil {
		return Result{}, providerBuildError(err)
	}

	triggerReq := reporttrigger.Request{
		Replay: alertreplay.Request{
			WindowStart:       req.WindowStart,
			WindowEnd:         req.WindowEnd,
			Grouping:          groupingConfig(binding.grouping),
			SourceFilter:      append([]string(nil), binding.grouping.SourceFilter...),
			CreatedByWorkflow: createdByWorkflow(req),
			Limit:             req.Limit,
		},
		CorrelationKey:                     correlationKey(req),
		WorkflowID:                         strings.TrimSpace(req.WorkflowID),
		Scenario:                           reportprompt.Scenario(binding.policy.ReportScenario),
		ReportNotificationChannelProfileID: binding.policy.ReportNotificationChannelProfileID,
	}
	result, err := s.replayAndStart(ctx, provider, s.uowFactory, s.starter, triggerReq)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Trigger: result,
		Policy:  binding.policy,
	}, nil
}

type policyBinding struct {
	policy   domain.ReportWorkflowPolicy
	source   domain.AlertSourceProfile
	grouping domain.GroupingPolicy
}

func (s *Service) loadBinding(ctx context.Context, policyID domain.ReportWorkflowPolicyID) (policyBinding, error) {
	var binding policyBinding
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		binding.policy, err = uow.Config().FindReportWorkflowPolicyByID(ctx, policyID)
		if err != nil {
			return err
		}
		if !binding.policy.Enabled {
			return fmt.Errorf("report policy trigger: report workflow policy must be enabled before replay: %w", domain.ErrInvariantViolation)
		}
		if binding.policy.TriggerMode != domain.ReportWorkflowTriggerModeManualReplay {
			return fmt.Errorf("report policy trigger: trigger mode %q is not supported by manual replay: %w", binding.policy.TriggerMode, domain.ErrInvariantViolation)
		}
		binding.source, err = uow.Config().FindAlertSourceProfileByID(ctx, binding.policy.AlertSourceProfileID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("report policy trigger: alert source profile not found: %w", domain.ErrNotFound)
			}
			return err
		}
		if !binding.source.Enabled {
			return fmt.Errorf("report policy trigger: alert source profile must be enabled before replay: %w", domain.ErrInvariantViolation)
		}
		binding.grouping, err = uow.Config().FindGroupingPolicyByID(ctx, binding.policy.GroupingPolicyID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("report policy trigger: grouping policy not found: %w", domain.ErrNotFound)
			}
			return err
		}
		if !binding.grouping.Enabled {
			return fmt.Errorf("report policy trigger: grouping policy must be enabled before replay: %w", domain.ErrInvariantViolation)
		}
		return nil
	})
	if err != nil {
		return policyBinding{}, err
	}
	return binding, nil
}

func validateRequest(req Request) error {
	if req.PolicyID <= 0 {
		return fmt.Errorf("report policy trigger: policy_id must be positive: %w", domain.ErrInvariantViolation)
	}
	if req.WindowStart.IsZero() {
		return fmt.Errorf("report policy trigger: window_start must be set: %w", domain.ErrInvariantViolation)
	}
	if req.WindowEnd.IsZero() {
		return fmt.Errorf("report policy trigger: window_end must be set: %w", domain.ErrInvariantViolation)
	}
	start := domain.NormalizeUTCMicro(req.WindowStart)
	end := domain.NormalizeUTCMicro(req.WindowEnd)
	if !end.After(start) {
		return fmt.Errorf("report policy trigger: window_end must be after window_start: %w", domain.ErrInvariantViolation)
	}
	if req.Limit <= 0 {
		return fmt.Errorf("report policy trigger: limit must be > 0: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func groupingConfig(policy domain.GroupingPolicy) alertgrouping.Config {
	return alertgrouping.Config{
		DimensionKeys: append([]string(nil), policy.DimensionKeys...),
		SeverityKey:   policy.SeverityKey,
	}
}

func correlationKey(req Request) string {
	if value := strings.TrimSpace(req.CorrelationKey); value != "" {
		return value
	}
	start := domain.NormalizeUTCMicro(req.WindowStart)
	end := domain.NormalizeUTCMicro(req.WindowEnd)
	return fmt.Sprintf(
		"report-workflow-policy:%d:%s:%s",
		req.PolicyID,
		start.Format(time.RFC3339Nano),
		end.Format(time.RFC3339Nano),
	)
}

func createdByWorkflow(req Request) string {
	if value := strings.TrimSpace(req.CreatedByWorkflow); value != "" {
		return value
	}
	return CreatedByWorkflow
}

func providerBuildError(err error) error {
	switch {
	case errors.Is(err, alertsourceprovider.ErrUnsupportedKind):
		return fmt.Errorf("report policy trigger: alert source kind is not supported for replay: %w", domain.ErrInvariantViolation)
	case errors.Is(err, alertsourceprovider.ErrSecretResolverUnavailable):
		return fmt.Errorf("report policy trigger: alert source credentials require a server-side secret resolver: %w", domain.ErrInvariantViolation)
	case errors.Is(err, alertsourceprovider.ErrSecretNotFound):
		return fmt.Errorf("report policy trigger: alert source secret reference is not available to the server-side resolver: %w", domain.ErrInvariantViolation)
	case errors.Is(err, alertsourceprovider.ErrSecretResolveFailed):
		return fmt.Errorf("report policy trigger: alert source secret reference could not be resolved by the server-side resolver: %w", domain.ErrInvariantViolation)
	case errors.Is(err, alertsourceprovider.ErrCredentialUnusable):
		return fmt.Errorf("report policy trigger: alert source secret reference resolved to an unusable credential: %w", domain.ErrInvariantViolation)
	case errors.Is(err, domain.ErrInvariantViolation):
		return err
	default:
		return fmt.Errorf("report policy trigger: metrics provider could not be constructed from the stored alert source profile: %w", domain.ErrInvariantViolation)
	}
}
