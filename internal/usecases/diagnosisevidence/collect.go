// Package diagnosisevidence executes bounded diagnosis evidence collection
// plans produced by the sandboxed diagnosis assistant.
package diagnosisevidence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultTimeout       = 10 * time.Second
	defaultTemplateLimit = 100
)

// Status is the coarse outcome for one evidence collection request.
type Status string

// Status values describe the coarse outcome for a diagnosis evidence request.
const (
	StatusCollected   Status = "collected"
	StatusSkipped     Status = "skipped"
	StatusFailed      Status = "failed"
	StatusUnsupported Status = "unsupported"
)

// ReasonCode is the stable machine-readable outcome for one collection item.
type ReasonCode string

// ReasonCode values are stable machine-readable evidence collection outcomes.
const (
	ReasonOK                   ReasonCode = "ok"
	ReasonUnsupportedTool      ReasonCode = "unsupported_tool"
	ReasonTemplateUnavailable  ReasonCode = "template_unavailable"
	ReasonTemplateAmbiguous    ReasonCode = "template_ambiguous"
	ReasonTemplateDisabled     ReasonCode = "template_disabled"
	ReasonTemplateToolMismatch ReasonCode = "template_tool_mismatch"
	ReasonSourceUnavailable    ReasonCode = "source_unavailable"
	ReasonSourceDisabled       ReasonCode = "source_disabled"
	ReasonSourceKindMismatch   ReasonCode = "source_kind_mismatch"
	ReasonProviderUnavailable  ReasonCode = "provider_unavailable"
	ReasonProviderFailed       ReasonCode = "provider_failed"
	ReasonCollectionTimedOut   ReasonCode = "collection_timed_out"
	ReasonInvalidRequest       ReasonCode = "invalid_request"
)

// Request asks the service to execute one batch of diagnosis evidence plans.
type Request struct {
	Requests []diagnosisroom.EvidenceRequest
}

// Result is the batch collection result.
type Result struct {
	Items []Item
}

// Item is the sanitized result for one evidence request.
type Item struct {
	Request              diagnosisroom.EvidenceRequest
	TemplateID           domain.DiagnosisToolTemplateID
	AlertSourceProfileID domain.AlertSourceProfileID
	AlertSourceKind      domain.AlertSourceKind
	Tool                 domain.DiagnosisToolKind
	Status               Status
	ReasonCode           ReasonCode
	Message              string
	Limit                int
	ObservedAlerts       int
	ActiveAlerts         []ports.ActiveAlert
	CollectedAt          time.Time
}

// Service coordinates configured templates, alert source providers, and bounded
// read-only collection calls.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	providers  *alertsourceprovider.Builder
	timeout    time.Duration
	clock      func() time.Time
}

// Option customizes Service construction.
type Option func(*Service)

// WithTimeout overrides the per-request provider call timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(s *Service) {
		if timeout > 0 {
			s.timeout = timeout
		}
	}
}

// WithClock injects a deterministic result timestamp clock.
func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// NewService constructs a diagnosis evidence collection service.
func NewService(
	uowFactory ports.UnitOfWorkFactory,
	providers *alertsourceprovider.Builder,
	opts ...Option,
) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("diagnosis evidence: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if providers == nil {
		return nil, fmt.Errorf("diagnosis evidence: alert source provider builder must be non-nil: %w", domain.ErrInvariantViolation)
	}
	svc := &Service{
		uowFactory: uowFactory,
		providers:  providers,
		timeout:    defaultTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	if svc.clock == nil {
		return nil, fmt.Errorf("diagnosis evidence: clock must be configured: %w", domain.ErrInvariantViolation)
	}
	return svc, nil
}

// Collect executes supported evidence requests and returns per-request results.
// Unsupported tools and configuration gaps are reported as result items rather
// than hard errors, so an operator can still see why confidence could not be
// raised automatically.
func (s *Service) Collect(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.uowFactory == nil || s.providers == nil || s.clock == nil {
		return Result{}, fmt.Errorf("diagnosis evidence: service is not configured: %w", domain.ErrInvariantViolation)
	}
	items := make([]Item, 0, len(req.Requests))
	for _, evidenceReq := range req.Requests {
		item, err := s.collectOne(ctx, evidenceReq)
		if err != nil {
			return Result{}, err
		}
		items = append(items, item)
	}
	return Result{Items: items}, nil
}

func (s *Service) collectOne(ctx context.Context, req diagnosisroom.EvidenceRequest) (Item, error) {
	item := Item{
		Request:     req,
		Tool:        req.Tool,
		Status:      StatusSkipped,
		ReasonCode:  ReasonInvalidRequest,
		Message:     "Evidence request is invalid.",
		CollectedAt: s.clock().UTC(),
	}
	if !req.Tool.Valid() {
		item.Status = StatusUnsupported
		item.ReasonCode = ReasonUnsupportedTool
		item.Message = "Evidence request tool is unsupported."
		return item, nil
	}
	if req.Tool != domain.DiagnosisToolKindActiveAlerts {
		item.Status = StatusUnsupported
		item.ReasonCode = ReasonUnsupportedTool
		item.Message = "Evidence collection currently supports active_alerts only."
		return item, nil
	}

	plan, blocked, err := s.resolvePlan(ctx, req, item)
	if err != nil {
		return Item{}, err
	}
	if blocked != nil {
		return *blocked, nil
	}

	provider, err := s.providers.Build(ctx, plan.profile)
	if err != nil {
		item = plan.apply(item)
		item.Status = StatusFailed
		item.ReasonCode = ReasonProviderUnavailable
		item.Message = "Alert source provider could not be constructed."
		return item, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	alerts, err := provider.ListActiveAlerts(callCtx)
	if err != nil {
		item = plan.apply(item)
		item.Status = StatusFailed
		if callCtx.Err() != nil {
			item.ReasonCode = ReasonCollectionTimedOut
			item.Message = "Active alert collection timed out."
			return item, nil
		}
		item.ReasonCode = ReasonProviderFailed
		item.Message = "Active alert collection failed."
		return item, nil
	}

	item = plan.apply(item)
	item.Status = StatusCollected
	item.ReasonCode = ReasonOK
	item.Message = "Active alert collection succeeded."
	item.ObservedAlerts = len(alerts)
	item.ActiveAlerts = cloneActiveAlerts(limitActiveAlerts(alerts, plan.limit))
	return item, nil
}

type resolvedPlan struct {
	template domain.DiagnosisToolTemplate
	profile  domain.AlertSourceProfile
	limit    int
}

func (p resolvedPlan) apply(item Item) Item {
	item.TemplateID = p.template.ID
	item.AlertSourceProfileID = p.profile.ID
	item.AlertSourceKind = p.profile.Kind
	item.Tool = p.template.Tool
	item.Limit = p.limit
	return item
}

func (s *Service) resolvePlan(
	ctx context.Context,
	req diagnosisroom.EvidenceRequest,
	item Item,
) (resolvedPlan, *Item, error) {
	var template domain.DiagnosisToolTemplate
	var profile domain.AlertSourceProfile
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		template, err = resolveTemplate(ctx, uow.Config(), req)
		if err != nil {
			return err
		}
		profile, err = uow.Config().FindAlertSourceProfileByID(ctx, template.AlertSourceProfileID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("%w: %w", errSourceUnavailable, err)
			}
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errTemplateUnavailable) {
			item.Status = StatusSkipped
			item.ReasonCode = ReasonTemplateUnavailable
			item.Message = "No enabled diagnosis tool template is available for this evidence request."
			return resolvedPlan{}, &item, nil
		}
		if errors.Is(err, errTemplateAmbiguous) {
			item.Status = StatusSkipped
			item.ReasonCode = ReasonTemplateAmbiguous
			item.Message = "Multiple enabled diagnosis tool templates match this evidence request; template_id is required."
			return resolvedPlan{}, &item, nil
		}
		if errors.Is(err, errSourceUnavailable) {
			item.Status = StatusSkipped
			item.ReasonCode = ReasonSourceUnavailable
			item.Message = "Configured alert source profile is unavailable."
			return resolvedPlan{}, &item, nil
		}
		return resolvedPlan{}, nil, err
	}

	item.TemplateID = template.ID
	item.AlertSourceProfileID = template.AlertSourceProfileID
	item.Tool = template.Tool
	if !template.Enabled {
		item.Status = StatusSkipped
		item.ReasonCode = ReasonTemplateDisabled
		item.Message = "Diagnosis tool template is disabled."
		return resolvedPlan{}, &item, nil
	}
	if template.Tool != req.Tool {
		item.Status = StatusSkipped
		item.ReasonCode = ReasonTemplateToolMismatch
		item.Message = "Diagnosis tool template does not match the requested tool."
		return resolvedPlan{}, &item, nil
	}
	if !profile.Enabled {
		item.AlertSourceKind = profile.Kind
		item.Status = StatusSkipped
		item.ReasonCode = ReasonSourceDisabled
		item.Message = "Alert source profile is disabled."
		return resolvedPlan{}, &item, nil
	}
	if !toolSupportsSourceKind(req.Tool, profile.Kind) {
		item.AlertSourceKind = profile.Kind
		item.Status = StatusSkipped
		item.ReasonCode = ReasonSourceKindMismatch
		item.Message = "Alert source profile kind does not support the requested tool."
		return resolvedPlan{}, &item, nil
	}
	limit := req.Limit
	if limit == 0 {
		limit = template.DefaultLimit
	}
	if limit <= 0 {
		item.AlertSourceKind = profile.Kind
		item.Status = StatusSkipped
		item.ReasonCode = ReasonInvalidRequest
		item.Message = "Evidence request limit is invalid."
		return resolvedPlan{}, &item, nil
	}
	return resolvedPlan{template: template, profile: profile, limit: limit}, nil, nil
}

var (
	errTemplateUnavailable = errors.New("diagnosis evidence template unavailable")
	errTemplateAmbiguous   = errors.New("diagnosis evidence template ambiguous")
	errSourceUnavailable   = errors.New("diagnosis evidence source unavailable")
)

func resolveTemplate(
	ctx context.Context,
	repo ports.ConfigurationRepository,
	req diagnosisroom.EvidenceRequest,
) (domain.DiagnosisToolTemplate, error) {
	if req.TemplateID > 0 {
		template, err := repo.FindDiagnosisToolTemplateByID(ctx, domain.DiagnosisToolTemplateID(req.TemplateID))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.DiagnosisToolTemplate{}, fmt.Errorf("%w: %w", errTemplateUnavailable, err)
			}
			return domain.DiagnosisToolTemplate{}, err
		}
		return template, nil
	}
	templates, err := repo.ListDiagnosisToolTemplates(ctx, defaultTemplateLimit)
	if err != nil {
		return domain.DiagnosisToolTemplate{}, err
	}
	var matches []domain.DiagnosisToolTemplate
	for _, template := range templates {
		if template.Enabled && template.Tool == req.Tool {
			matches = append(matches, template)
		}
	}
	switch len(matches) {
	case 0:
		return domain.DiagnosisToolTemplate{}, errTemplateUnavailable
	case 1:
		return matches[0], nil
	default:
		return domain.DiagnosisToolTemplate{}, errTemplateAmbiguous
	}
}

func toolSupportsSourceKind(tool domain.DiagnosisToolKind, kind domain.AlertSourceKind) bool {
	switch tool {
	case domain.DiagnosisToolKindActiveAlerts:
		return kind == domain.AlertSourceKindPrometheus || kind == domain.AlertSourceKindAlertmanager
	case domain.DiagnosisToolKindMetricQuery, domain.DiagnosisToolKindMetricRangeQuery:
		return kind == domain.AlertSourceKindPrometheus
	default:
		return false
	}
}

func limitActiveAlerts(in []ports.ActiveAlert, limit int) []ports.ActiveAlert {
	if limit <= 0 || len(in) <= limit {
		return in
	}
	return in[:limit]
}

func cloneActiveAlerts(in []ports.ActiveAlert) []ports.ActiveAlert {
	if in == nil {
		return nil
	}
	out := make([]ports.ActiveAlert, len(in))
	for i, alert := range in {
		out[i] = ports.ActiveAlert{
			Source:      alert.Source,
			Labels:      cloneStringMap(alert.Labels),
			Annotations: cloneStringMap(alert.Annotations),
			StartsAt:    alert.StartsAt,
		}
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// CloneItems returns a defensive copy of collection result items.
func CloneItems(in []Item) []Item {
	if in == nil {
		return nil
	}
	out := make([]Item, len(in))
	for i, item := range in {
		out[i] = item
		out[i].ActiveAlerts = cloneActiveAlerts(item.ActiveAlerts)
	}
	return out
}
