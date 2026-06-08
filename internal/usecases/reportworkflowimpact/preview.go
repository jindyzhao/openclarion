// Package reportworkflowimpact computes non-persistent report workflow policy
// impact previews for operator-managed configuration.
package reportworkflowimpact

import (
	"context"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/groupingpreview"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Status is the sanitized readiness category returned by an impact preview.
type Status string

const (
	// StatusReady means persisted bindings are ready and recent events match.
	StatusReady Status = "ready"
	// StatusReview means bindings are usable but operators should review impact.
	StatusReview Status = "review"
	// StatusBlocked means persisted bindings are not ready for report workflow use.
	StatusBlocked Status = "blocked"
)

// ReasonCode is a stable machine-readable reason for the preview status.
type ReasonCode string

const (
	// ReasonOK means no readiness issue was detected.
	ReasonOK ReasonCode = "ok"
	// ReasonAlertSourceDisabled means the bound alert source is disabled.
	ReasonAlertSourceDisabled ReasonCode = "alert_source_disabled"
	// ReasonGroupingPolicyDisabled means the bound grouping policy is disabled.
	ReasonGroupingPolicyDisabled ReasonCode = "grouping_policy_disabled"
	// ReasonNotificationChannelDisabled means the bound report notification channel is disabled.
	ReasonNotificationChannelDisabled ReasonCode = "notification_channel_disabled"
	// ReasonNotificationChannelMissingReportScope means the bound channel cannot deliver reports.
	ReasonNotificationChannelMissingReportScope ReasonCode = "notification_channel_missing_report_scope"
	// ReasonUnsupportedTriggerMode means the stored trigger mode is not supported by this preview.
	ReasonUnsupportedTriggerMode ReasonCode = "unsupported_trigger_mode"
	// ReasonNoMatchingEvents means the recent alert sample had no events matching the grouping policy.
	ReasonNoMatchingEvents ReasonCode = "no_matching_events"
)

// Request identifies the policy and bounded sample size for an impact preview.
type Request struct {
	PolicyID domain.ReportWorkflowPolicyID
	Limit    int
}

// Result is action output only. It summarizes persisted policy bindings and a
// bounded recent-alert grouping preview without calling providers or persisting
// workflow artifacts.
type Result struct {
	Policy                               domain.ReportWorkflowPolicy
	Status                               Status
	ReasonCodes                          []ReasonCode
	Message                              string
	CheckedAt                            time.Time
	AlertSourceID                        domain.AlertSourceProfileID
	AlertSourceKind                      domain.AlertSourceKind
	AlertSourceAuthMode                  domain.AlertSourceAuthMode
	AlertSourceEnabled                   bool
	GroupingPolicy                       domain.GroupingPolicy
	ReportNotificationChannelBound       bool
	ReportNotificationChannelEnabled     bool
	ReportNotificationChannelReportScope bool
	EventsScanned                        int
	EventsMatched                        int
	Groups                               []groupingpreview.Group
}

// Service owns report workflow impact preview orchestration.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	now        func() time.Time
}

// Option customizes impact preview behavior.
type Option func(*Service)

// WithClock injects a deterministic clock for preview timestamps.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// NewService constructs a report workflow impact preview service.
func NewService(uowFactory ports.UnitOfWorkFactory, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("report workflow impact preview: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	svc := &Service{uowFactory: uowFactory}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc, nil
}

// Preview loads the stored report workflow policy, resolves its persisted
// bindings, reads a bounded recent AlertEvent sample, and returns sanitized
// readiness plus estimated grouping impact. It does not resolve secrets, build
// providers, start workflows, send notifications, or persist groups/snapshots.
func (s *Service) Preview(ctx context.Context, req Request) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("report workflow impact preview: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if req.PolicyID == 0 {
		return Result{}, fmt.Errorf("report workflow impact preview: policy_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if req.Limit <= 0 {
		return Result{}, fmt.Errorf("report workflow impact preview: limit must be > 0: %w", domain.ErrInvariantViolation)
	}
	if s.now == nil {
		return Result{}, fmt.Errorf("report workflow impact preview: clock must be configured: %w", domain.ErrInvariantViolation)
	}

	var policy domain.ReportWorkflowPolicy
	var source domain.AlertSourceProfile
	var grouping domain.GroupingPolicy
	var channel *domain.NotificationChannelProfile
	var events []domain.AlertEvent
	if err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		policy, err = uow.Config().FindReportWorkflowPolicyByID(ctx, req.PolicyID)
		if err != nil {
			return err
		}
		source, err = uow.Config().FindAlertSourceProfileByID(ctx, policy.AlertSourceProfileID)
		if err != nil {
			return err
		}
		grouping, err = uow.Config().FindGroupingPolicyByID(ctx, policy.GroupingPolicyID)
		if err != nil {
			return err
		}
		if policy.ReportNotificationChannelProfileID != 0 {
			resolved, err := uow.Config().FindNotificationChannelProfileByID(ctx, policy.ReportNotificationChannelProfileID)
			if err != nil {
				return err
			}
			channel = &resolved
		}
		events, err = uow.Alerts().ListEvents(ctx, req.Limit)
		return err
	}); err != nil {
		return Result{}, err
	}

	eventsMatched, groups, err := groupingpreview.PreviewEvents(grouping, events)
	if err != nil {
		return Result{}, err
	}

	return buildResult(policy, source, grouping, channel, len(events), eventsMatched, groups, s.now().UTC()), nil
}

func buildResult(
	policy domain.ReportWorkflowPolicy,
	source domain.AlertSourceProfile,
	grouping domain.GroupingPolicy,
	channel *domain.NotificationChannelProfile,
	eventsScanned int,
	eventsMatched int,
	groups []groupingpreview.Group,
	checkedAt time.Time,
) Result {
	reasons := make([]ReasonCode, 0, 4)
	if !source.Enabled {
		reasons = append(reasons, ReasonAlertSourceDisabled)
	}
	if !grouping.Enabled {
		reasons = append(reasons, ReasonGroupingPolicyDisabled)
	}
	if policy.TriggerMode != domain.ReportWorkflowTriggerModeManualReplay {
		reasons = append(reasons, ReasonUnsupportedTriggerMode)
	}

	channelBound := channel != nil
	channelEnabled := false
	channelReportScope := false
	if channelBound {
		channelEnabled = channel.Enabled
		channelReportScope = supportsReportDelivery(*channel)
		if !channelEnabled {
			reasons = append(reasons, ReasonNotificationChannelDisabled)
		}
		if !channelReportScope {
			reasons = append(reasons, ReasonNotificationChannelMissingReportScope)
		}
	}

	status := StatusReady
	message := "Report workflow policy impact preview is ready."
	if len(reasons) != 0 {
		status = StatusBlocked
		message = "Report workflow policy impact preview is blocked by configuration readiness."
	} else if eventsMatched == 0 {
		status = StatusReview
		reasons = append(reasons, ReasonNoMatchingEvents)
		message = "Report workflow policy impact preview found no matching recent alerts."
	} else {
		reasons = append(reasons, ReasonOK)
	}

	return Result{
		Policy:                               policy,
		Status:                               status,
		ReasonCodes:                          reasons,
		Message:                              message,
		CheckedAt:                            checkedAt,
		AlertSourceID:                        source.ID,
		AlertSourceKind:                      source.Kind,
		AlertSourceAuthMode:                  source.AuthMode,
		AlertSourceEnabled:                   source.Enabled,
		GroupingPolicy:                       grouping,
		ReportNotificationChannelBound:       channelBound,
		ReportNotificationChannelEnabled:     channelEnabled,
		ReportNotificationChannelReportScope: channelReportScope,
		EventsScanned:                        eventsScanned,
		EventsMatched:                        eventsMatched,
		Groups:                               groups,
	}
}

func supportsReportDelivery(channel domain.NotificationChannelProfile) bool {
	for _, scope := range channel.DeliveryScopes {
		if scope == domain.NotificationDeliveryScopeReport {
			return true
		}
	}
	return false
}
