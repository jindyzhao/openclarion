// Package diagnosistooltemplate owns operator-managed diagnosis tool template
// configuration and explicit enablement actions.
package diagnosistooltemplate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultActiveAlertsLimit     = 5
	defaultMetricQueryLimit      = 5
	defaultMetricRangeQueryLimit = 5
	defaultMetricRangeWindow     = time.Hour
	defaultMetricRangeMaxWindow  = 6 * time.Hour
	defaultMetricRangeStep       = time.Minute
)

// WriteRequest describes mutable diagnosis tool template metadata. Enabled
// state is intentionally excluded; callers must use explicit actions.
type WriteRequest struct {
	Name                 string
	AlertSourceProfileID domain.AlertSourceProfileID
	Tool                 domain.DiagnosisToolKind
	QueryTemplate        string
	DefaultLimit         int
	DefaultWindow        time.Duration
	MaxWindow            time.Duration
	DefaultStep          time.Duration
}

// ActionRequest identifies one diagnosis tool template enablement action.
type ActionRequest struct {
	TemplateID domain.DiagnosisToolTemplateID
}

// Service coordinates diagnosis tool template configuration updates.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	now        func() time.Time
}

// Option customizes diagnosis tool template service behavior.
type Option func(*Service)

// WithClock injects the clock used for explicit enablement timestamps.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// NewService constructs a diagnosis tool template service.
func NewService(uowFactory ports.UnitOfWorkFactory, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("diagnosis tool template: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
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

// Create stores a disabled diagnosis tool template draft.
func (s *Service) Create(ctx context.Context, req WriteRequest) (domain.DiagnosisToolTemplate, error) {
	if s == nil {
		return domain.DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	template, err := templateFromWriteRequest(req, false, nil, nil)
	if err != nil {
		return domain.DiagnosisToolTemplate{}, err
	}

	var saved domain.DiagnosisToolTemplate
	err = s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if err := requireBoundAlertSourceCompatible(ctx, uow, template, false); err != nil {
			return err
		}
		var serr error
		saved, serr = uow.Config().SaveDiagnosisToolTemplate(ctx, template)
		return serr
	})
	return saved, err
}

// Replace updates template metadata while preserving explicit enablement state.
func (s *Service) Replace(ctx context.Context, templateID domain.DiagnosisToolTemplateID, req WriteRequest) (domain.DiagnosisToolTemplate, error) {
	if s == nil {
		return domain.DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if templateID == 0 {
		return domain.DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: id must be non-zero: %w", domain.ErrInvariantViolation)
	}

	var saved domain.DiagnosisToolTemplate
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Config().FindDiagnosisToolTemplateByID(ctx, templateID)
		if err != nil {
			return err
		}
		template, err := templateFromWriteRequest(req, existing.Enabled, existing.EnabledAt, existing.DisabledAt)
		if err != nil {
			return err
		}
		template.ID = templateID
		if err := requireBoundAlertSourceCompatible(ctx, uow, template, existing.Enabled); err != nil {
			return err
		}
		var uerr error
		saved, uerr = uow.Config().UpdateDiagnosisToolTemplate(ctx, template)
		return uerr
	})
	return saved, err
}

// Enable explicitly enables a diagnosis tool template after validating source
// readiness. It does not start a diagnosis workflow or collect evidence.
func (s *Service) Enable(ctx context.Context, req ActionRequest) (domain.DiagnosisToolTemplate, error) {
	return s.setEnabled(ctx, req.TemplateID, true)
}

// Disable explicitly disables a diagnosis tool template.
func (s *Service) Disable(ctx context.Context, req ActionRequest) (domain.DiagnosisToolTemplate, error) {
	return s.setEnabled(ctx, req.TemplateID, false)
}

func (s *Service) setEnabled(ctx context.Context, templateID domain.DiagnosisToolTemplateID, enabled bool) (domain.DiagnosisToolTemplate, error) {
	if s == nil {
		return domain.DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if templateID == 0 {
		return domain.DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if s.now == nil {
		return domain.DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: clock must be configured: %w", domain.ErrInvariantViolation)
	}

	var saved domain.DiagnosisToolTemplate
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		template, err := uow.Config().FindDiagnosisToolTemplateByID(ctx, templateID)
		if err != nil {
			return err
		}
		if enabled {
			if err := requireBoundAlertSourceCompatible(ctx, uow, template, true); err != nil {
				return err
			}
		}
		updated, err := domain.WithDiagnosisToolTemplateEnabled(template, enabled, s.now())
		if err != nil {
			return err
		}
		var uerr error
		saved, uerr = uow.Config().UpdateDiagnosisToolTemplate(ctx, updated)
		return uerr
	})
	return saved, err
}

func templateFromWriteRequest(
	req WriteRequest,
	enabled bool,
	enabledAt *time.Time,
	disabledAt *time.Time,
) (domain.DiagnosisToolTemplate, error) {
	tool := req.Tool
	if tool == "" {
		tool = domain.DiagnosisToolKindActiveAlerts
	}
	limit := req.DefaultLimit
	if limit == 0 {
		limit = defaultLimitForTool(tool)
	}
	defaultWindow := req.DefaultWindow
	maxWindow := req.MaxWindow
	defaultStep := req.DefaultStep
	if tool == domain.DiagnosisToolKindMetricRangeQuery {
		if defaultWindow == 0 {
			defaultWindow = defaultMetricRangeWindow
		}
		if maxWindow == 0 {
			maxWindow = defaultMetricRangeMaxWindow
		}
		if defaultStep == 0 {
			defaultStep = defaultMetricRangeStep
		}
	}
	return domain.NewDiagnosisToolTemplate(
		req.Name,
		req.AlertSourceProfileID,
		tool,
		req.QueryTemplate,
		limit,
		defaultWindow,
		maxWindow,
		defaultStep,
		enabled,
		enabledAt,
		disabledAt,
	)
}

func defaultLimitForTool(tool domain.DiagnosisToolKind) int {
	switch tool {
	case domain.DiagnosisToolKindActiveAlerts:
		return defaultActiveAlertsLimit
	case domain.DiagnosisToolKindMetricQuery:
		return defaultMetricQueryLimit
	case domain.DiagnosisToolKindMetricRangeQuery:
		return defaultMetricRangeQueryLimit
	default:
		return 0
	}
}

func requireBoundAlertSourceCompatible(
	ctx context.Context,
	uow ports.UnitOfWork,
	template domain.DiagnosisToolTemplate,
	requireEnabled bool,
) error {
	source, err := uow.Config().FindAlertSourceProfileByID(ctx, template.AlertSourceProfileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("diagnosis tool template: alert source profile not found: %w", domain.ErrNotFound)
		}
		return err
	}
	if requireEnabled && !source.Enabled {
		return fmt.Errorf("diagnosis tool template: alert source profile must be enabled before template enablement: %w", domain.ErrInvariantViolation)
	}
	if !toolSupportsAlertSourceKind(template.Tool, source.Kind) {
		return fmt.Errorf("diagnosis tool template: alert source profile kind %q does not support tool %q: %w", source.Kind, template.Tool, domain.ErrInvariantViolation)
	}
	return nil
}

func toolSupportsAlertSourceKind(tool domain.DiagnosisToolKind, kind domain.AlertSourceKind) bool {
	switch tool {
	case domain.DiagnosisToolKindActiveAlerts:
		return kind == domain.AlertSourceKindPrometheus || kind == domain.AlertSourceKindAlertmanager
	case domain.DiagnosisToolKindMetricQuery, domain.DiagnosisToolKindMetricRangeQuery:
		return kind == domain.AlertSourceKindPrometheus
	default:
		return false
	}
}
