// Package alertdiagnosis turns already-persisted alert windows into automatic
// diagnosis-room starts for explicitly enabled auto-room policies.
package alertdiagnosis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiscontext"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// CreatedByWorkflow stamps snapshots produced by automatic alert-intake
	// diagnosis handoff.
	CreatedByWorkflow = "AlertmanagerWebhookAutoDiagnosis"

	defaultPolicyScanLimit = 1000

	// DefaultMaxRoomsPerTrigger bounds automatic diagnosis-room starts from one
	// alert intake or report-policy replay so a large alert window cannot fan
	// out into unbounded sandbox/LLM work.
	DefaultMaxRoomsPerTrigger = 3
	// MaxRoomsPerTriggerLimit is the largest accepted operator override.
	MaxRoomsPerTriggerLimit = 100

	diagnosisRoomClosedEventKind      = "diagnosis_room.closed"
	diagnosisRoomProgressEventKind    = "diagnosis_room.turn_persisted"
	diagnosisRoomHumanConfirmedReason = "human_confirmed"
	diagnosisRoomCloseVersion         = "diagnosis-room-close.v1"
	confirmedDiagnosisTaskScanLimit   = 100
	confirmedDiagnosisAvailableStatus = "available"
)

// Request identifies one already-ingested alert window eligible for automatic
// diagnosis-room starts.
type Request struct {
	AlertSourceProfileID domain.AlertSourceProfileID
	WindowStart          time.Time
	WindowEnd            time.Time
	AlertEventIDs        []domain.AlertEventID
	Limit                int
}

// StartRoomsRequest identifies already-built EvidenceSnapshots that should be
// handed off to automatic diagnosis rooms for one auto_room policy.
type StartRoomsRequest struct {
	AlertSourceProfileID domain.AlertSourceProfileID
	Policy               domain.ReportWorkflowPolicy
	Snapshots            []alertreplay.SnapshotRef
}

// Result summarizes the auto-room work performed for one alert intake.
type Result struct {
	PoliciesMatched int
	Snapshots       []alertreplay.SnapshotRef
	Rooms           []RoomStart
	// SkippedSnapshots contains snapshots left without an automatic room only
	// because the per-trigger runtime safety cap was reached.
	SkippedSnapshots []alertreplay.SnapshotRef
}

// RoomStart identifies one diagnosis room accepted by the workflow starter.
type RoomStart struct {
	PolicyID           domain.ReportWorkflowPolicyID
	EvidenceSnapshotID domain.EvidenceSnapshotID
	SessionID          string
	InitialMessageID   string
	Workflow           ports.WorkflowHandle
}

// PersistedWindowReplayer is the alertreplay boundary used after webhook
// ingestion has already persisted firing alerts.
type PersistedWindowReplayer func(context.Context, ports.UnitOfWorkFactory, alertreplay.Request) (alertreplay.Result, error)

// Service resolves enabled auto-room policies, builds evidence snapshots from
// persisted alert events, and starts idempotent diagnosis rooms.
type Service struct {
	uowFactory         ports.UnitOfWorkFactory
	starter            ports.DiagnosisRoomWorkflowStarter
	replay             PersistedWindowReplayer
	cmdbProvider       ports.CMDBProvider
	maxRoomsPerTrigger int
}

// Option customizes Service construction.
type Option func(*Service)

// WithPersistedWindowReplayer overrides the replay function for tests.
func WithPersistedWindowReplayer(replay PersistedWindowReplayer) Option {
	return func(s *Service) {
		if replay != nil {
			s.replay = replay
		}
	}
}

// WithCMDBProvider enables optional ownership and topology enrichment for
// EvidenceSnapshots produced from persisted alert intake.
func WithCMDBProvider(provider ports.CMDBProvider) Option {
	return func(s *Service) {
		if provider != nil {
			s.cmdbProvider = provider
		}
	}
}

// WithMaxRoomsPerTrigger bounds automatic diagnosis-room starts per trigger.
// The service still reports every produced snapshot; snapshots beyond the
// bound are counted as skipped for this invocation.
func WithMaxRoomsPerTrigger(limit int) Option {
	return func(s *Service) {
		s.maxRoomsPerTrigger = limit
	}
}

// NewService constructs an automatic alert-diagnosis trigger service.
func NewService(
	uowFactory ports.UnitOfWorkFactory,
	starter ports.DiagnosisRoomWorkflowStarter,
	opts ...Option,
) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("alert diagnosis: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if starter == nil {
		return nil, fmt.Errorf("alert diagnosis: diagnosis room starter must be non-nil: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		uowFactory:         uowFactory,
		starter:            starter,
		replay:             alertreplay.ReplayPersistedWindowForReport,
		maxRoomsPerTrigger: DefaultMaxRoomsPerTrigger,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	if err := validateMaxRoomsPerTrigger(service.maxRoomsPerTrigger); err != nil {
		return nil, err
	}
	return service, nil
}

// Trigger starts diagnosis rooms for policies whose diagnosis follow-up mode is
// explicitly auto_room. suggest_room remains a UI handoff mode and is not
// promoted to automatic workflow starts.
func (s *Service) Trigger(ctx context.Context, req Request) (Result, error) {
	var result Result
	if s == nil || s.uowFactory == nil || s.starter == nil || s.replay == nil {
		return result, fmt.Errorf("alert diagnosis: service is not configured: %w", domain.ErrInvariantViolation)
	}
	window, err := validateRequest(req)
	if err != nil {
		return result, err
	}

	bindings, err := s.loadBindings(ctx, req.AlertSourceProfileID)
	if err != nil {
		return result, err
	}
	result.PoliciesMatched = len(bindings)
	if len(bindings) == 0 {
		return result, nil
	}

	plannedRooms := make([]plannedRoomStart, 0, s.maxRoomsPerTrigger)
	for _, binding := range bindings {
		replay, err := s.replay(ctx, s.uowFactory, alertreplay.Request{
			WindowStart:              window.StartInclusive(),
			WindowEnd:                window.EndExclusive(),
			Grouping:                 groupingConfig(binding.grouping),
			AlertEventIDFilter:       append([]domain.AlertEventID(nil), req.AlertEventIDs...),
			SourceFilter:             append([]string(nil), binding.grouping.SourceFilter...),
			AlertSourceProfileFilter: []domain.AlertSourceProfileID{req.AlertSourceProfileID},
			CreatedByWorkflow:        CreatedByWorkflow,
			Limit:                    req.Limit,
			CMDBProvider:             s.cmdbProvider,
		})
		if err != nil {
			return result, err
		}
		result.Snapshots = append(result.Snapshots, replay.Snapshots...)

		plan, err := s.planRooms(
			ctx,
			binding.policy,
			req.AlertSourceProfileID,
			replay.Snapshots,
			s.maxRoomsPerTrigger-len(plannedRooms),
		)
		if err != nil {
			return result, err
		}
		plannedRooms = append(plannedRooms, plan.rooms...)
		result.SkippedSnapshots = append(result.SkippedSnapshots, plan.skippedSnapshots...)
	}

	rooms, err := s.startPlannedRooms(ctx, plannedRooms)
	result.Rooms = append(result.Rooms, rooms...)
	if err != nil {
		return result, err
	}
	return result, nil
}

// StartRooms starts diagnosis rooms for already-built snapshots under a single
// enabled auto_room policy. It is used by report-policy replay paths that have
// already performed alert replay and must not replay the alert window again.
func (s *Service) StartRooms(ctx context.Context, req StartRoomsRequest) (Result, error) {
	var result Result
	if s == nil || s.uowFactory == nil || s.starter == nil {
		return result, fmt.Errorf("alert diagnosis: service is not configured: %w", domain.ErrInvariantViolation)
	}
	if err := validateStartRoomsRequest(req); err != nil {
		return result, err
	}
	if err := s.validateAutoRoomNotificationBinding(ctx, req.Policy.ReportNotificationChannelProfileID); err != nil {
		return result, err
	}
	result.PoliciesMatched = 1
	result.Snapshots = cloneSnapshotRefs(req.Snapshots)

	plan, err := s.planRooms(
		ctx,
		req.Policy,
		req.AlertSourceProfileID,
		req.Snapshots,
		s.maxRoomsPerTrigger,
	)
	if err != nil {
		return result, err
	}
	result.SkippedSnapshots = plan.skippedSnapshots
	result.Rooms, err = s.startPlannedRooms(ctx, plan.rooms)
	if err != nil {
		return result, err
	}
	return result, nil
}

func validateMaxRoomsPerTrigger(limit int) error {
	if limit <= 0 || limit > MaxRoomsPerTriggerLimit {
		return fmt.Errorf("alert diagnosis: max_rooms_per_trigger must be between 1 and %d: %w", MaxRoomsPerTriggerLimit, domain.ErrInvariantViolation)
	}
	return nil
}

type roomStartPlan struct {
	rooms            []plannedRoomStart
	skippedSnapshots []alertreplay.SnapshotRef
}

type plannedRoomStart struct {
	policyID         domain.ReportWorkflowPolicyID
	snapshotID       domain.EvidenceSnapshotID
	initialMessageID string
	request          ports.DiagnosisRoomStartRequest
}

func (s *Service) planRooms(
	ctx context.Context,
	policy domain.ReportWorkflowPolicy,
	sourceID domain.AlertSourceProfileID,
	snapshots []alertreplay.SnapshotRef,
	capacity int,
) (roomStartPlan, error) {
	confirmationStates, err := s.confirmedDiagnosisStates(ctx, snapshots)
	if err != nil {
		return roomStartPlan{}, err
	}

	roomCapacity := max(capacity, 0)
	plan := roomStartPlan{
		rooms:            make([]plannedRoomStart, 0, min(len(snapshots), roomCapacity)),
		skippedSnapshots: make([]alertreplay.SnapshotRef, 0, max(len(snapshots)-roomCapacity, 0)),
	}
	for _, snapshotRef := range snapshots {
		state, ok := confirmationStates[snapshotRef.ID]
		if !ok {
			return roomStartPlan{}, fmt.Errorf(
				"alert diagnosis: confirmed diagnosis history missing for snapshot %d: %w",
				snapshotRef.ID,
				domain.ErrInvariantViolation,
			)
		}
		if state.err != nil {
			// Once capacity is exhausted, an unreadable history cannot cause an
			// extra room start. Keep it in the exact cap-skip set and continue
			// checking later snapshots so confirmed suffixes are not misreported.
			if len(plan.rooms) >= roomCapacity {
				plan.skippedSnapshots = append(plan.skippedSnapshots, snapshotRef)
				continue
			}
			return roomStartPlan{}, state.err
		}
		if state.confirmed {
			continue
		}
		if len(plan.rooms) >= roomCapacity {
			plan.skippedSnapshots = append(plan.skippedSnapshots, snapshotRef)
			continue
		}
		room, err := s.prepareRoomStart(ctx, policy, sourceID, snapshotRef.ID)
		if err != nil {
			return roomStartPlan{}, err
		}
		plan.rooms = append(plan.rooms, room)
	}
	return plan, nil
}

func (s *Service) startPlannedRooms(ctx context.Context, plans []plannedRoomStart) ([]RoomStart, error) {
	// Every local read and validation finishes during planning. Deterministic
	// session IDs let a retry converge if an external start fails mid-batch.
	rooms := make([]RoomStart, 0, len(plans))
	for _, plan := range plans {
		started, err := s.starter.StartDiagnosisRoom(ctx, plan.request)
		if err != nil {
			return rooms, fmt.Errorf("alert diagnosis: start diagnosis room: %w", err)
		}
		rooms = append(rooms, RoomStart{
			PolicyID:           plan.policyID,
			EvidenceSnapshotID: plan.snapshotID,
			SessionID:          started.SessionID,
			InitialMessageID:   plan.initialMessageID,
			Workflow:           started.Workflow,
		})
	}
	return rooms, nil
}

type confirmedDiagnosisClosedEventPayload struct {
	Kind               string `json:"kind"`
	DiagnosisTaskID    int64  `json:"diagnosis_task_id"`
	EvidenceSnapshotID int64  `json:"evidence_snapshot_id"`
	CloseReason        string `json:"close_reason"`
	ConclusionVersion  string `json:"conclusion_version"`
	FinalConclusion    *struct {
		Status             string `json:"status"`
		EvidenceSnapshotID int64  `json:"evidence_snapshot_id"`
		ConclusionVersion  string `json:"conclusion_version"`
		ConfirmedBy        string `json:"confirmed_by"`
		Content            string `json:"content"`
	} `json:"final_conclusion"`
}

type confirmedDiagnosisState struct {
	confirmed bool
	err       error
}

func (s *Service) confirmedDiagnosisStates(
	ctx context.Context,
	snapshots []alertreplay.SnapshotRef,
) (map[domain.EvidenceSnapshotID]confirmedDiagnosisState, error) {
	snapshotIDs := make([]domain.EvidenceSnapshotID, 0, len(snapshots))
	seen := make(map[domain.EvidenceSnapshotID]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		if _, ok := seen[snapshot.ID]; ok {
			continue
		}
		seen[snapshot.ID] = struct{}{}
		snapshotIDs = append(snapshotIDs, snapshot.ID)
	}
	if len(snapshotIDs) == 0 {
		return map[domain.EvidenceSnapshotID]confirmedDiagnosisState{}, nil
	}

	var histories []ports.DiagnosisSnapshotHistory
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		histories, err = uow.Diagnosis().ListSnapshotHistories(
			ctx,
			snapshotIDs,
			confirmedDiagnosisTaskScanLimit,
			[]string{diagnosisRoomClosedEventKind, diagnosisRoomProgressEventKind},
		)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("alert diagnosis: inspect confirmed diagnosis histories: %w", err)
	}
	if len(histories) != len(snapshotIDs) {
		return nil, fmt.Errorf(
			"alert diagnosis: confirmed diagnosis history count %d does not match snapshot count %d: %w",
			len(histories),
			len(snapshotIDs),
			domain.ErrInvariantViolation,
		)
	}

	states := make(map[domain.EvidenceSnapshotID]confirmedDiagnosisState, len(histories))
	for _, history := range histories {
		if _, expected := seen[history.EvidenceSnapshotID]; !expected {
			return nil, fmt.Errorf(
				"alert diagnosis: confirmed diagnosis history references unexpected snapshot %d: %w",
				history.EvidenceSnapshotID,
				domain.ErrInvariantViolation,
			)
		}
		if _, duplicate := states[history.EvidenceSnapshotID]; duplicate {
			return nil, fmt.Errorf(
				"alert diagnosis: duplicate confirmed diagnosis history for snapshot %d: %w",
				history.EvidenceSnapshotID,
				domain.ErrInvariantViolation,
			)
		}
		confirmed, historyErr := confirmedDiagnosisFromHistory(history)
		if historyErr != nil {
			historyErr = fmt.Errorf(
				"alert diagnosis: inspect confirmed diagnosis for snapshot %d: %w",
				history.EvidenceSnapshotID,
				historyErr,
			)
		}
		states[history.EvidenceSnapshotID] = confirmedDiagnosisState{confirmed: confirmed, err: historyErr}
	}
	return states, nil
}

func confirmedDiagnosisFromHistory(history ports.DiagnosisSnapshotHistory) (bool, error) {
	if history.EvidenceSnapshotID <= 0 {
		return false, fmt.Errorf("confirmed diagnosis history has an invalid snapshot id: %w", domain.ErrInvariantViolation)
	}
	if history.TasksTruncated {
		return false, fmt.Errorf(
			"confirmed diagnosis lookup for snapshot %d exceeded %d recent tasks: %w",
			history.EvidenceSnapshotID,
			confirmedDiagnosisTaskScanLimit,
			domain.ErrInvariantViolation,
		)
	}
	if len(history.Tasks) > confirmedDiagnosisTaskScanLimit {
		return false, fmt.Errorf(
			"confirmed diagnosis lookup for snapshot %d returned %d tasks above limit %d: %w",
			history.EvidenceSnapshotID,
			len(history.Tasks),
			confirmedDiagnosisTaskScanLimit,
			domain.ErrInvariantViolation,
		)
	}

	tasks := make(map[domain.DiagnosisTaskID]domain.DiagnosisTask, len(history.Tasks))
	for _, task := range history.Tasks {
		if task.ID <= 0 || task.EvidenceSnapshotID != history.EvidenceSnapshotID || !task.Status.Valid() {
			return false, fmt.Errorf(
				"diagnosis task %d is invalid for snapshot %d: %w",
				task.ID,
				history.EvidenceSnapshotID,
				domain.ErrInvariantViolation,
			)
		}
		if _, duplicate := tasks[task.ID]; duplicate {
			return false, fmt.Errorf("diagnosis task %d is duplicated in snapshot history: %w", task.ID, domain.ErrInvariantViolation)
		}
		tasks[task.ID] = task
	}

	events := make(map[domain.DiagnosisTaskID]map[string]domain.DiagnosisTaskEvent, len(tasks))
	for _, event := range history.LatestEvents {
		if event.ID <= 0 || (event.Kind != diagnosisRoomClosedEventKind && event.Kind != diagnosisRoomProgressEventKind) {
			return false, fmt.Errorf("diagnosis history event %d has an invalid identity: %w", event.ID, domain.ErrInvariantViolation)
		}
		if _, ok := tasks[event.TaskID]; !ok {
			return false, fmt.Errorf(
				"diagnosis history event %d references unexpected task %d: %w",
				event.ID,
				event.TaskID,
				domain.ErrInvariantViolation,
			)
		}
		if events[event.TaskID] == nil {
			events[event.TaskID] = make(map[string]domain.DiagnosisTaskEvent, 2)
		}
		if _, duplicate := events[event.TaskID][event.Kind]; duplicate {
			return false, fmt.Errorf(
				"diagnosis history has duplicate latest %q events for task %d: %w",
				event.Kind,
				event.TaskID,
				domain.ErrInvariantViolation,
			)
		}
		events[event.TaskID][event.Kind] = event
	}

	var latestConfirmation time.Time
	var latestProgress time.Time
	for _, task := range history.Tasks {
		taskEvents := events[task.ID]
		if progress, ok := taskEvents[diagnosisRoomProgressEventKind]; ok {
			recordedAt, err := diagnosisHistoryEventRecordedAt(progress)
			if err != nil {
				return false, err
			}
			if recordedAt.After(latestProgress) {
				latestProgress = recordedAt
			}
		}
		if task.Status != domain.DiagnosisStatusSucceeded {
			continue
		}
		closed, ok := taskEvents[diagnosisRoomClosedEventKind]
		if !ok {
			continue
		}
		confirmed, err := confirmedDiagnosisClosedEvent(closed, task)
		if err != nil {
			return false, err
		}
		if !confirmed {
			continue
		}
		recordedAt, err := diagnosisHistoryEventRecordedAt(closed)
		if err != nil {
			return false, err
		}
		if recordedAt.After(latestConfirmation) {
			latestConfirmation = recordedAt
		}
	}

	return !latestConfirmation.IsZero() && !latestProgress.After(latestConfirmation), nil
}

func diagnosisHistoryEventRecordedAt(event domain.DiagnosisTaskEvent) (time.Time, error) {
	recordedAt := event.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = event.OccurredAt
	}
	if recordedAt.IsZero() {
		return time.Time{}, fmt.Errorf("diagnosis history event %d has no recorded time: %w", event.ID, domain.ErrInvariantViolation)
	}
	return recordedAt, nil
}

func confirmedDiagnosisClosedEvent(event domain.DiagnosisTaskEvent, task domain.DiagnosisTask) (bool, error) {
	if event.ID <= 0 ||
		event.Kind != diagnosisRoomClosedEventKind ||
		event.TaskID != task.ID ||
		len(event.Payload) == 0 {
		return false, fmt.Errorf("confirmed diagnosis close event identity is incomplete: %w", domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return false, fmt.Errorf("confirmed diagnosis close event %d payload is ambiguous: %w: %w", event.ID, err, domain.ErrInvariantViolation)
	}
	var payload confirmedDiagnosisClosedEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return false, fmt.Errorf("confirmed diagnosis close event %d payload: %w: %w", event.ID, err, domain.ErrInvariantViolation)
	}
	if payload.Kind != diagnosisRoomClosedEventKind ||
		payload.DiagnosisTaskID != int64(task.ID) ||
		payload.EvidenceSnapshotID != int64(task.EvidenceSnapshotID) ||
		payload.ConclusionVersion != diagnosisRoomCloseVersion {
		return false, fmt.Errorf("confirmed diagnosis close event %d payload identity mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	closeReason := strings.TrimSpace(payload.CloseReason)
	if closeReason == "" || closeReason != payload.CloseReason {
		return false, fmt.Errorf("confirmed diagnosis close event %d has an invalid close_reason: %w", event.ID, domain.ErrInvariantViolation)
	}
	if closeReason != diagnosisRoomHumanConfirmedReason {
		return false, nil
	}
	if payload.FinalConclusion == nil || payload.FinalConclusion.Status != confirmedDiagnosisAvailableStatus {
		return false, fmt.Errorf("confirmed diagnosis close event %d has no available conclusion: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.FinalConclusion.ConclusionVersion != diagnosisRoomCloseVersion {
		return false, fmt.Errorf("confirmed diagnosis close event %d conclusion version mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.FinalConclusion.EvidenceSnapshotID != int64(task.EvidenceSnapshotID) {
		return false, fmt.Errorf("confirmed diagnosis close event %d conclusion snapshot mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	confirmedBy := strings.TrimSpace(payload.FinalConclusion.ConfirmedBy)
	if confirmedBy == "" || confirmedBy != payload.FinalConclusion.ConfirmedBy {
		return false, fmt.Errorf("confirmed diagnosis close event %d has an invalid confirmed_by: %w", event.ID, domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(payload.FinalConclusion.Content) == "" {
		return false, fmt.Errorf("confirmed diagnosis close event %d has empty conclusion content: %w", event.ID, domain.ErrInvariantViolation)
	}
	return true, nil
}

type binding struct {
	policy   domain.ReportWorkflowPolicy
	grouping domain.GroupingPolicy
}

func (s *Service) loadBindings(ctx context.Context, sourceID domain.AlertSourceProfileID) ([]binding, error) {
	var out []binding
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		policies, err := uow.Config().ListReportWorkflowPolicies(ctx, defaultPolicyScanLimit)
		if err != nil {
			return err
		}
		for _, policy := range policies {
			if !isAutoRoomPolicyForSource(policy, sourceID) {
				continue
			}
			if err := validateAutoRoomPolicyForSource(policy, sourceID); err != nil {
				return err
			}
			channel, err := uow.Config().FindNotificationChannelProfileByID(ctx, policy.ReportNotificationChannelProfileID)
			if err != nil {
				return err
			}
			if err := validateAutoRoomNotificationChannel(channel); err != nil {
				return err
			}
			grouping, err := uow.Config().FindGroupingPolicyByID(ctx, policy.GroupingPolicyID)
			if err != nil {
				return err
			}
			if !grouping.Enabled {
				continue
			}
			out = append(out, binding{policy: policy, grouping: grouping})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) validateAutoRoomNotificationBinding(ctx context.Context, id domain.NotificationChannelProfileID) error {
	return s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		channel, err := uow.Config().FindNotificationChannelProfileByID(ctx, id)
		if err != nil {
			return err
		}
		return validateAutoRoomNotificationChannel(channel)
	})
}

func (s *Service) prepareRoomStart(
	ctx context.Context,
	policy domain.ReportWorkflowPolicy,
	sourceID domain.AlertSourceProfileID,
	snapshotID domain.EvidenceSnapshotID,
) (plannedRoomStart, error) {
	snapshot, err := s.loadSnapshot(ctx, snapshotID)
	if err != nil {
		return plannedRoomStart{}, err
	}
	evidence, err := diagnosiscontext.EvidenceWithAvailableDiagnosisTools(ctx, s.uowFactory, snapshot.Payload)
	if err != nil {
		return plannedRoomStart{}, err
	}
	sessionID := AutoRoomSessionID(policy.ID, snapshotID)
	ownerSubject := AutoRoomOwnerSubject(sourceID, policy.ID)
	initialMessageID := AutoRoomInitialMessageID(policy.ID, snapshotID)
	return plannedRoomStart{
		policyID:         policy.ID,
		snapshotID:       snapshot.ID,
		initialMessageID: initialMessageID,
		request: ports.DiagnosisRoomStartRequest{
			SessionID:                         sessionID,
			EvidenceSnapshotID:                snapshot.ID,
			OwnerSubject:                      ownerSubject,
			Evidence:                          evidence,
			CloseNotificationChannelProfileID: policy.ReportNotificationChannelProfileID,
			InitialTurn: &ports.DiagnosisRoomInitialTurnRequest{
				MessageID:    initialMessageID,
				ActorSubject: ownerSubject,
				Message:      AutoRoomInitialMessage(policy.ID, sourceID, snapshotID),
			},
		},
	}, nil
}

func (s *Service) loadSnapshot(ctx context.Context, id domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error) {
	var snapshot domain.EvidenceSnapshot
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		snapshot, err = uow.Evidence().FindByID(ctx, id)
		return err
	})
	if err != nil {
		return domain.EvidenceSnapshot{}, err
	}
	if snapshot.Status == domain.SnapshotStatusFailed {
		return domain.EvidenceSnapshot{}, fmt.Errorf("alert diagnosis: evidence snapshot %d has failed status: %w", snapshot.ID, domain.ErrInvariantViolation)
	}
	return snapshot, nil
}

// AutoRoomSessionID returns the deterministic external session key for one
// policy/snapshot pair. The Temporal starter maps this to an idempotent
// workflow ID, so webhook retries converge on the same diagnosis room.
func AutoRoomSessionID(policyID domain.ReportWorkflowPolicyID, snapshotID domain.EvidenceSnapshotID) string {
	return fmt.Sprintf("diagnosis-session-auto-p%d-s%d", policyID, snapshotID)
}

// AutoRoomOwnerSubject returns the service principal that owns automatic rooms.
func AutoRoomOwnerSubject(sourceID domain.AlertSourceProfileID, policyID domain.ReportWorkflowPolicyID) string {
	return fmt.Sprintf("openclarion.alertmanager-webhook:%d:policy:%d", sourceID, policyID)
}

// AutoRoomInitialMessageID returns the deterministic first-turn id for an
// automatic diagnosis room.
func AutoRoomInitialMessageID(policyID domain.ReportWorkflowPolicyID, snapshotID domain.EvidenceSnapshotID) string {
	return fmt.Sprintf("diagnosis-auto-initial-p%d-s%d", policyID, snapshotID)
}

// AutoRoomInitialMessage asks the diagnosis workflow to produce the first AI
// assessment from frozen alert evidence and to identify executable and
// operator-supplied follow-up evidence before confidence is raised.
func AutoRoomInitialMessage(
	policyID domain.ReportWorkflowPolicyID,
	sourceID domain.AlertSourceProfileID,
	snapshotID domain.EvidenceSnapshotID,
) string {
	return fmt.Sprintf(
		"OpenClarion automatic alert intake for source %d, workflow policy %d, evidence snapshot %d: generate an initial diagnosis report from the frozen alert evidence, then explicitly list what executable evidence and operator-supplied evidence can raise confidence. Summarize operational impact, explain the current confidence, and recommend immediate next actions. The first operator notification must be the AI diagnosis report with evidence requests, not a raw alert forward or final conclusion. When openclarion_available_diagnosis_tools is present, copy relevant evidence_request_example objects into evidence_requests before asking for human-supplied evidence. Use missing_evidence_requests for operator-provided evidence that cannot be collected by the listed tools. Keep conclusion_status as needs_evidence and keep confidence low or medium until collected evidence or reviewed supplemental evidence supports ready_for_review. Do not mark the first automatic turn final. Do not invent evidence outside the snapshot.",
		sourceID,
		policyID,
		snapshotID,
	)
}

func validateRequest(req Request) (domain.AlertWindow, error) {
	if req.AlertSourceProfileID <= 0 {
		return domain.AlertWindow{}, fmt.Errorf("alert diagnosis: alert_source_profile_id must be positive: %w", domain.ErrInvariantViolation)
	}
	window, err := domain.NewAlertWindow(req.WindowStart, req.WindowEnd)
	if err != nil {
		return domain.AlertWindow{}, fmt.Errorf("alert diagnosis: replay window: %w", err)
	}
	if req.Limit <= 0 {
		return domain.AlertWindow{}, fmt.Errorf("alert diagnosis: limit must be > 0: %w", domain.ErrInvariantViolation)
	}
	for _, id := range req.AlertEventIDs {
		if id <= 0 {
			return domain.AlertWindow{}, fmt.Errorf("alert diagnosis: alert_event_ids must contain positive ids: %w", domain.ErrInvariantViolation)
		}
	}
	return window, nil
}

func validateStartRoomsRequest(req StartRoomsRequest) error {
	if req.AlertSourceProfileID <= 0 {
		return fmt.Errorf("alert diagnosis: alert_source_profile_id must be positive: %w", domain.ErrInvariantViolation)
	}
	if err := validateAutoRoomPolicyForSource(req.Policy, req.AlertSourceProfileID); err != nil {
		return err
	}
	for i, snapshot := range req.Snapshots {
		if snapshot.ID <= 0 {
			return fmt.Errorf("alert diagnosis: snapshots[%d].id must be positive: %w", i, domain.ErrInvariantViolation)
		}
		if snapshot.GroupIndex < 0 {
			return fmt.Errorf("alert diagnosis: snapshots[%d].group_index must be >= 0: %w", i, domain.ErrInvariantViolation)
		}
		if snapshot.EventCount <= 0 {
			return fmt.Errorf("alert diagnosis: snapshots[%d].event_count must be > 0: %w", i, domain.ErrInvariantViolation)
		}
	}
	return nil
}

func cloneSnapshotRefs(in []alertreplay.SnapshotRef) []alertreplay.SnapshotRef {
	if in == nil {
		return nil
	}
	return append([]alertreplay.SnapshotRef(nil), in...)
}

func isAutoRoomPolicyForSource(policy domain.ReportWorkflowPolicy, sourceID domain.AlertSourceProfileID) bool {
	return policy.Enabled &&
		policy.AlertSourceProfileID == sourceID &&
		policy.DiagnosisFollowUp == domain.DiagnosisFollowUpModeAutoRoom
}

func validateAutoRoomPolicyForSource(policy domain.ReportWorkflowPolicy, sourceID domain.AlertSourceProfileID) error {
	if !isAutoRoomPolicyForSource(policy, sourceID) {
		return fmt.Errorf("alert diagnosis: policy must be enabled auto_room for alert source %d: %w", sourceID, domain.ErrInvariantViolation)
	}
	if policy.ReportNotificationChannelProfileID == 0 {
		return fmt.Errorf("alert diagnosis: auto_room policy must bind a notification channel profile: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func validateAutoRoomNotificationChannel(channel domain.NotificationChannelProfile) error {
	if !channel.Enabled {
		return fmt.Errorf("alert diagnosis: auto_room notification channel profile must be enabled: %w", domain.ErrInvariantViolation)
	}
	if channel.Kind != domain.NotificationChannelKindWeCom {
		return fmt.Errorf("alert diagnosis: auto_room notification channel profile must be an Enterprise WeChat channel: %w", domain.ErrInvariantViolation)
	}
	if !notificationChannelSupportsScope(channel, domain.NotificationDeliveryScopeReport) {
		return fmt.Errorf("alert diagnosis: auto_room notification channel profile must include report delivery scope: %w", domain.ErrInvariantViolation)
	}
	if !notificationChannelSupportsScope(channel, domain.NotificationDeliveryScopeDiagnosisConsultation) {
		return fmt.Errorf("alert diagnosis: auto_room notification channel profile must include diagnosis_consultation delivery scope: %w", domain.ErrInvariantViolation)
	}
	if !notificationChannelSupportsScope(channel, domain.NotificationDeliveryScopeDiagnosisClose) {
		return fmt.Errorf("alert diagnosis: auto_room notification channel profile must include diagnosis_close delivery scope: %w", domain.ErrInvariantViolation)
	}
	if missingProofs := channel.MissingAIDiagnosisProofContentKinds(); len(missingProofs) > 0 {
		return fmt.Errorf("alert diagnosis: auto_room notification channel profile must have current AI delivery test proof for %s: %w", notificationProofKindList(missingProofs), domain.ErrInvariantViolation)
	}
	return nil
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
	values := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		values = append(values, string(kind))
	}
	return strings.Join(values, " and ")
}

func groupingConfig(policy domain.GroupingPolicy) alertgrouping.Config {
	return alertgrouping.Config{
		DimensionKeys: append([]string(nil), policy.DimensionKeys...),
		SeverityKey:   policy.SeverityKey,
	}
}
