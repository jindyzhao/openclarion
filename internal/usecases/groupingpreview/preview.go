// Package groupingpreview computes bounded, non-persistent grouping previews
// for operator-managed grouping policies.
package groupingpreview

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Request identifies the policy and bounded sample size for a preview.
type Request struct {
	PolicyID domain.GroupingPolicyID
	Limit    int
}

// Result is the complete grouping preview. It is action output only and is not
// persisted as configuration.
type Result struct {
	Policy        domain.GroupingPolicy
	EventsScanned int
	EventsMatched int
	Groups        []Group
}

// Group is one preview grouping result converted from an AlertGroup draft.
type Group struct {
	GroupKey    string
	Dimensions  map[string]string
	Severity    domain.GroupSeverity
	EventCount  int
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	EventIDs    []domain.AlertEventID
}

// Service owns grouping preview orchestration.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
}

// NewService constructs a grouping preview service.
func NewService(uowFactory ports.UnitOfWorkFactory) *Service {
	return &Service{uowFactory: uowFactory}
}

// Preview loads the persisted policy, reads a bounded recent alert sample, and
// applies the deterministic grouping algorithm without persisting AlertGroup
// rows or calling external providers.
func (s *Service) Preview(ctx context.Context, req Request) (Result, error) {
	if req.PolicyID == 0 {
		return Result{}, fmt.Errorf("grouping preview: policy_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if req.Limit <= 0 {
		return Result{}, fmt.Errorf("grouping preview: limit must be > 0: %w", domain.ErrInvariantViolation)
	}
	var policy domain.GroupingPolicy
	var events []domain.AlertEvent
	if err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		policy, err = uow.Config().FindGroupingPolicyByID(ctx, req.PolicyID)
		if err != nil {
			return err
		}
		events, err = uow.Alerts().ListEvents(ctx, req.Limit)
		return err
	}); err != nil {
		return Result{}, err
	}

	matched := filterEventsBySource(events, policy.SourceFilter)
	drafts, err := alertgrouping.GroupEvents(matched, alertgrouping.Config{
		DimensionKeys: policy.DimensionKeys,
		SeverityKey:   policy.SeverityKey,
	})
	if err != nil {
		return Result{}, err
	}
	groups, err := previewGroups(drafts)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Policy:        policy,
		EventsScanned: len(events),
		EventsMatched: len(matched),
		Groups:        groups,
	}, nil
}

func filterEventsBySource(events []domain.AlertEvent, sourceFilter []string) []domain.AlertEvent {
	if len(sourceFilter) == 0 {
		return events
	}
	allowed := make(map[string]struct{}, len(sourceFilter))
	for _, source := range sourceFilter {
		allowed[source] = struct{}{}
	}
	out := make([]domain.AlertEvent, 0, len(events))
	for _, event := range events {
		if _, ok := allowed[event.Source]; ok {
			out = append(out, event)
		}
	}
	return out
}

func previewGroups(drafts []domain.AlertGroup) ([]Group, error) {
	out := make([]Group, len(drafts))
	for i, draft := range drafts {
		dimensions := map[string]string{}
		if len(draft.Dimensions) != 0 {
			if err := json.Unmarshal(draft.Dimensions, &dimensions); err != nil {
				return nil, fmt.Errorf("grouping preview: decode dimensions for group %s: %w", draft.GroupKey, err)
			}
		}
		eventIDs := make([]domain.AlertEventID, len(draft.EventIDs))
		copy(eventIDs, draft.EventIDs)
		out[i] = Group{
			GroupKey:    draft.GroupKey,
			Dimensions:  dimensions,
			Severity:    draft.Severity,
			EventCount:  draft.EventCount,
			FirstSeenAt: draft.FirstSeenAt,
			LastSeenAt:  draft.LastSeenAt,
			EventIDs:    eventIDs,
		}
	}
	return out, nil
}
