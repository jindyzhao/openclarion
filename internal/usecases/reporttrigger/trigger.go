// Package reporttrigger connects alert replay output to the report
// batch workflow start port.
package reporttrigger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

// Request configures one replay-and-report trigger invocation.
type Request struct {
	Replay                             alertreplay.Request
	CorrelationKey                     string
	WorkflowID                         string
	Scenario                           reportprompt.Scenario
	ReportNotificationChannelProfileID domain.NotificationChannelProfileID
}

// Result records both replay output and the optional workflow handle.
type Result struct {
	CorrelationKey string
	Replay         alertreplay.Result
	Workflow       ports.WorkflowHandle
	Started        bool
}

// Service owns the dependencies needed to replay alerts and start
// report workflows.
type Service struct {
	provider     ports.ActiveAlertProvider
	cmdbProvider ports.CMDBProvider
	factory      ports.UnitOfWorkFactory
	starter      ports.ReportWorkflowStarter
}

// Option customizes Service construction.
type Option func(*Service)

// WithCMDBProvider enables optional ownership and topology enrichment for
// EvidenceSnapshots produced by replay.
func WithCMDBProvider(provider ports.CMDBProvider) Option {
	return func(s *Service) {
		if provider != nil {
			s.cmdbProvider = provider
		}
	}
}

// NewService builds a report trigger service with explicit port
// dependencies.
func NewService(
	provider ports.ActiveAlertProvider,
	factory ports.UnitOfWorkFactory,
	starter ports.ReportWorkflowStarter,
	opts ...Option,
) (*Service, error) {
	if provider == nil {
		return nil, fmt.Errorf("report trigger: provider must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if factory == nil {
		return nil, fmt.Errorf("report trigger: factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if starter == nil {
		return nil, fmt.Errorf("report trigger: starter must be non-nil: %w", domain.ErrInvariantViolation)
	}
	service := &Service{provider: provider, factory: factory, starter: starter}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service, nil
}

// ReplayAndStart replays one alert window through the service's
// configured ports, then starts ReportBatchWorkflow when needed.
func (s *Service) ReplayAndStart(ctx context.Context, req Request) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("report trigger: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if s.cmdbProvider != nil {
		req.Replay.CMDBProvider = s.cmdbProvider
	}
	return ReplayAndStart(ctx, s.provider, s.factory, s.starter, req)
}

// ReplayAndStart replays one alert window, then starts ReportBatchWorkflow
// when replay produced at least one EvidenceSnapshot.
func ReplayAndStart(
	ctx context.Context,
	provider ports.ActiveAlertProvider,
	factory ports.UnitOfWorkFactory,
	starter ports.ReportWorkflowStarter,
	req Request,
) (Result, error) {
	var result Result
	if starter == nil {
		return result, fmt.Errorf("report trigger: starter must be non-nil: %w", domain.ErrInvariantViolation)
	}

	replay, err := alertreplay.ReplayWindowForReport(ctx, provider, factory, req.Replay)
	result.Replay = replay
	if err != nil {
		return result, err
	}
	correlationKey, err := correlationKeyForRequest(req)
	if err != nil {
		return result, err
	}
	result.CorrelationKey = correlationKey

	startReq, ok, err := BuildStartRequest(replay, req)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, nil
	}

	handle, err := starter.StartReportBatch(ctx, startReq)
	if err != nil {
		return result, fmt.Errorf("report trigger: start report batch: %w", err)
	}
	result.Workflow = handle
	result.Started = true
	return result, nil
}

// BuildStartRequest maps replay snapshot refs into a report workflow
// start request. The boolean return is false when there is nothing to
// start because replay produced no snapshots.
func BuildStartRequest(replay alertreplay.Result, req Request) (ports.ReportBatchStartRequest, bool, error) {
	scenario := req.Scenario
	if scenario == "" {
		scenario = reportprompt.ScenarioSingleAlert
	}
	if !scenario.Valid() {
		return ports.ReportBatchStartRequest{}, false, fmt.Errorf("report trigger: scenario %q is unsupported: %w", scenario, domain.ErrInvariantViolation)
	}
	if len(replay.Snapshots) == 0 {
		return ports.ReportBatchStartRequest{}, false, nil
	}

	correlationKey, err := correlationKeyForRequest(req)
	if err != nil {
		return ports.ReportBatchStartRequest{}, false, err
	}
	workflowID := strings.TrimSpace(req.WorkflowID)
	if workflowID == "" {
		workflowID = workflowIDForCorrelationKey(correlationKey)
	}

	items := make([]ports.ReportBatchStartItem, len(replay.Snapshots))
	for i, ref := range replay.Snapshots {
		if ref.ID == 0 {
			return ports.ReportBatchStartRequest{}, false, fmt.Errorf("report trigger: snapshots[%d].id must be non-zero: %w", i, domain.ErrInvariantViolation)
		}
		if ref.GroupIndex < 0 {
			return ports.ReportBatchStartRequest{}, false, fmt.Errorf("report trigger: snapshots[%d].group_index must be >= 0: %w", i, domain.ErrInvariantViolation)
		}
		if ref.EventCount <= 0 {
			return ports.ReportBatchStartRequest{}, false, fmt.Errorf("report trigger: snapshots[%d].event_count must be > 0: %w", i, domain.ErrInvariantViolation)
		}
		items[i] = ports.ReportBatchStartItem{
			EvidenceSnapshotID: ref.ID,
			Scenario:           string(scenario),
			GroupIndex:         ref.GroupIndex,
		}
	}

	return ports.ReportBatchStartRequest{
		WorkflowID:                         workflowID,
		CorrelationKey:                     correlationKey,
		ReportNotificationChannelProfileID: req.ReportNotificationChannelProfileID,
		Items:                              items,
	}, true, nil
}

func correlationKeyForRequest(req Request) (string, error) {
	correlationKey := strings.TrimSpace(req.CorrelationKey)
	if correlationKey != "" {
		return correlationKey, nil
	}
	start := domain.NormalizeUTCMicro(req.Replay.WindowStart)
	end := domain.NormalizeUTCMicro(req.Replay.WindowEnd)
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return "", fmt.Errorf("report trigger: replay window must be valid when correlation key is omitted: %w", domain.ErrInvariantViolation)
	}
	components := []string{"alert-replay", start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano)}
	if len(req.Replay.AlertEventIDFilter) > 0 {
		ids := append([]domain.AlertEventID(nil), req.Replay.AlertEventIDFilter...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		idParts := make([]string, 0, len(ids))
		for _, id := range ids {
			if id <= 0 {
				return "", fmt.Errorf("report trigger: alert event id filter must contain positive ids: %w", domain.ErrInvariantViolation)
			}
			idParts = append(idParts, fmt.Sprintf("%d", id))
		}
		components = append(components, "alert-events", strings.Join(idParts, ","))
	}
	return strings.Join(components, ":"), nil
}

func workflowIDForCorrelationKey(correlationKey string) string {
	sum := sha256.Sum256([]byte(correlationKey))
	return "report-batch-" + hex.EncodeToString(sum[:16])
}
