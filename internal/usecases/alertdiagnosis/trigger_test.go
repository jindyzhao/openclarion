package alertdiagnosis

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiscontext"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestTriggerStartsRoomsOnlyForAutoRoomPolicies(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	autoPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	autoPolicy.ReportNotificationChannelProfileID = 33
	suggestPolicy := mustReportWorkflowPolicy(t, 14, sourceID, domain.DiagnosisFollowUpModeSuggestRoom)
	otherSourcePolicy := mustReportWorkflowPolicy(t, 15, domain.AlertSourceProfileID(99), domain.DiagnosisFollowUpModeAutoRoom)
	grouping := mustGroupingPolicy(t)
	snapshot := domain.EvidenceSnapshot{
		ID:                77,
		AlertGroupID:      31,
		Digest:            "digest-77",
		Payload:           json.RawMessage(`{"schema_version":"test"}`),
		Provenance:        json.RawMessage(`{}`),
		Status:            domain.SnapshotStatusComplete,
		CreatedByWorkflow: CreatedByWorkflow,
	}
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{
			policies: []domain.ReportWorkflowPolicy{suggestPolicy, otherSourcePolicy, autoPolicy},
			groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{
				grouping.ID: grouping,
			},
		},
		evidence: &fakeTriggerEvidenceRepo{
			snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{snapshot.ID: snapshot},
		},
	}
	starter := &recordingRoomStarter{}
	var replayRequests []alertreplay.Request
	service, err := NewService(
		factory,
		starter,
		WithPersistedWindowReplayer(func(_ context.Context, _ ports.UnitOfWorkFactory, req alertreplay.Request) (alertreplay.Result, error) {
			replayRequests = append(replayRequests, req)
			return alertreplay.Result{
				Stats:     alertreplay.Stats{EventsLoaded: 2, GroupsBuilt: 1, SnapshotsSaved: 1},
				Snapshots: []alertreplay.SnapshotRef{{ID: snapshot.ID, GroupIndex: 0, EventCount: 2}},
			}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	windowStart := time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	result, err := service.Trigger(ctx, Request{
		AlertSourceProfileID: sourceID,
		WindowStart:          windowStart,
		WindowEnd:            windowStart.Add(time.Minute),
		AlertEventIDs:        []domain.AlertEventID{101, 102},
		Limit:                100,
	})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	if result.PoliciesMatched != 1 || len(result.Snapshots) != 1 || len(result.Rooms) != 1 {
		t.Fatalf("result = %+v, want one matched policy, snapshot, and room", result)
	}
	if len(replayRequests) != 1 {
		t.Fatalf("replay requests = %d, want 1", len(replayRequests))
	}
	gotReplay := replayRequests[0]
	if gotReplay.CreatedByWorkflow != CreatedByWorkflow ||
		gotReplay.Limit != 100 ||
		gotReplay.Grouping.SeverityKey != "severity" ||
		len(gotReplay.Grouping.DimensionKeys) != 1 ||
		gotReplay.Grouping.DimensionKeys[0] != "alertname" ||
		len(gotReplay.SourceFilter) != 1 ||
		gotReplay.SourceFilter[0] != "alertmanager" ||
		len(gotReplay.AlertSourceProfileFilter) != 1 ||
		gotReplay.AlertSourceProfileFilter[0] != sourceID ||
		len(gotReplay.AlertEventIDFilter) != 2 ||
		gotReplay.AlertEventIDFilter[0] != 101 ||
		gotReplay.AlertEventIDFilter[1] != 102 {
		t.Fatalf("replay request = %+v", gotReplay)
	}
	if len(starter.requests) != 1 {
		t.Fatalf("starter requests = %d, want 1", len(starter.requests))
	}
	gotStart := starter.requests[0]
	wantSessionID := AutoRoomSessionID(autoPolicy.ID, snapshot.ID)
	if gotStart.SessionID != wantSessionID ||
		gotStart.EvidenceSnapshotID != snapshot.ID ||
		gotStart.OwnerSubject != AutoRoomOwnerSubject(sourceID, autoPolicy.ID) ||
		string(gotStart.Evidence) != string(snapshot.Payload) ||
		gotStart.CloseNotificationChannelProfileID != autoPolicy.ReportNotificationChannelProfileID {
		t.Fatalf("start request = %+v", gotStart)
	}
	if gotStart.InitialTurn == nil {
		t.Fatal("start request initial turn = nil, want automatic first diagnosis turn")
	}
	wantMessageID := AutoRoomInitialMessageID(autoPolicy.ID, snapshot.ID)
	if gotStart.InitialTurn.MessageID != wantMessageID ||
		gotStart.InitialTurn.ActorSubject != AutoRoomOwnerSubject(sourceID, autoPolicy.ID) ||
		gotStart.InitialTurn.Message == "" {
		t.Fatalf("initial turn = %+v", gotStart.InitialTurn)
	}
	if result.Rooms[0].SessionID != wantSessionID ||
		result.Rooms[0].PolicyID != autoPolicy.ID ||
		result.Rooms[0].EvidenceSnapshotID != snapshot.ID ||
		result.Rooms[0].InitialMessageID != wantMessageID ||
		result.Rooms[0].Workflow.WorkflowID == "" {
		t.Fatalf("room result = %+v", result.Rooms[0])
	}
}

func TestTriggerMountsAvailableDiagnosisTools(t *testing.T) {
	ctx := context.Background()
	source := mustTriggerAlertSource(t, 7)
	template := mustTriggerToolTemplate(t, 17, source.ID)
	autoPolicy := mustReportWorkflowPolicy(t, 13, source.ID, domain.DiagnosisFollowUpModeAutoRoom)
	grouping := mustGroupingPolicy(t)
	snapshot := domain.EvidenceSnapshot{
		ID:                77,
		AlertGroupID:      31,
		Digest:            "digest-77",
		Payload:           json.RawMessage(`{"schema_version":"test"}`),
		Provenance:        json.RawMessage(`{}`),
		Status:            domain.SnapshotStatusComplete,
		CreatedByWorkflow: CreatedByWorkflow,
	}
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{
			policies:  []domain.ReportWorkflowPolicy{autoPolicy},
			templates: []domain.DiagnosisToolTemplate{template},
			sources:   map[domain.AlertSourceProfileID]domain.AlertSourceProfile{source.ID: source},
			groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{
				grouping.ID: grouping,
			},
		},
		evidence: &fakeTriggerEvidenceRepo{
			snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{snapshot.ID: snapshot},
		},
	}
	starter := &recordingRoomStarter{}
	service, err := NewService(
		factory,
		starter,
		WithPersistedWindowReplayer(func(context.Context, ports.UnitOfWorkFactory, alertreplay.Request) (alertreplay.Result, error) {
			return alertreplay.Result{
				Snapshots: []alertreplay.SnapshotRef{{ID: snapshot.ID, GroupIndex: 0, EventCount: 2}},
			}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	windowStart := time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	_, err = service.Trigger(ctx, Request{
		AlertSourceProfileID: source.ID,
		WindowStart:          windowStart,
		WindowEnd:            windowStart.Add(time.Minute),
		Limit:                100,
	})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if len(starter.requests) != 1 {
		t.Fatalf("starter requests = %d, want 1", len(starter.requests))
	}
	evidence := starter.requests[0].Evidence
	if strings.Contains(string(evidence), "alertmanager.example.invalid") ||
		strings.Contains(string(evidence), "secret/alertmanager") {
		t.Fatalf("evidence leaked provider configuration: %s", evidence)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(evidence, &top); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	rawCatalog, ok := top[diagnosiscontext.AvailableDiagnosisToolsKey]
	if !ok {
		t.Fatalf("evidence missing %q: %s", diagnosiscontext.AvailableDiagnosisToolsKey, evidence)
	}
	var catalog struct {
		Items []diagnosiscontext.AvailableDiagnosisTool `json:"items"`
	}
	if err := json.Unmarshal(rawCatalog, &catalog); err != nil {
		t.Fatalf("unmarshal tools catalog: %v", err)
	}
	if len(catalog.Items) != 1 {
		t.Fatalf("tools len = %d, want 1: %+v", len(catalog.Items), catalog.Items)
	}
	got := catalog.Items[0]
	if got.TemplateID != int64(template.ID) ||
		got.AlertSourceProfileID != int64(source.ID) ||
		got.Tool != string(domain.DiagnosisToolKindActiveAlerts) ||
		got.QueryTemplate != "" {
		t.Fatalf("tool = %+v", got)
	}
}

func TestTriggerRejectsAutoRoomPolicyWithoutNotificationChannelBeforeReplay(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	autoPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	autoPolicy.ReportNotificationChannelProfileID = 0
	grouping := mustGroupingPolicy(t)
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{
			policies: []domain.ReportWorkflowPolicy{autoPolicy},
			groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{
				grouping.ID: grouping,
			},
		},
		evidence: &fakeTriggerEvidenceRepo{},
	}
	starter := &recordingRoomStarter{}
	service, err := NewService(
		factory,
		starter,
		WithPersistedWindowReplayer(func(context.Context, ports.UnitOfWorkFactory, alertreplay.Request) (alertreplay.Result, error) {
			t.Fatal("replay should not run for an invalid auto_room policy")
			return alertreplay.Result{}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	windowStart := time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	_, err = service.Trigger(ctx, Request{
		AlertSourceProfileID: sourceID,
		WindowStart:          windowStart,
		WindowEnd:            windowStart.Add(time.Minute),
		Limit:                100,
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Trigger error = %v, want ErrInvariantViolation", err)
	}
	if len(starter.requests) != 0 {
		t.Fatalf("starter requests = %d, want 0", len(starter.requests))
	}
}

func TestTriggerRejectsAutoRoomPolicyWithUnreadyNotificationChannelBeforeReplay(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	grouping := mustGroupingPolicy(t)
	tests := []struct {
		name    string
		channel domain.NotificationChannelProfile
		want    string
	}{
		{
			name: "disabled_channel",
			channel: domain.NotificationChannelProfile{
				ID:             33,
				Enabled:        false,
				DeliveryScopes: autoRoomNotificationScopes(),
			},
			want: "must be enabled",
		},
		{
			name: "missing_consultation_scope",
			channel: domain.NotificationChannelProfile{
				ID:      33,
				Kind:    domain.NotificationChannelKindWeCom,
				Enabled: true,
				DeliveryScopes: []domain.NotificationDeliveryScope{
					domain.NotificationDeliveryScopeDiagnosisClose,
					domain.NotificationDeliveryScopeReport,
				},
			},
			want: "diagnosis_consultation",
		},
		{
			name: "missing_close_scope",
			channel: domain.NotificationChannelProfile{
				ID:      33,
				Kind:    domain.NotificationChannelKindWeCom,
				Enabled: true,
				DeliveryScopes: []domain.NotificationDeliveryScope{
					domain.NotificationDeliveryScopeDiagnosisConsultation,
					domain.NotificationDeliveryScopeReport,
				},
			},
			want: "diagnosis_close",
		},
		{
			name: "missing_report_scope",
			channel: domain.NotificationChannelProfile{
				ID:      33,
				Kind:    domain.NotificationChannelKindWeCom,
				Enabled: true,
				DeliveryScopes: []domain.NotificationDeliveryScope{
					domain.NotificationDeliveryScopeDiagnosisClose,
					domain.NotificationDeliveryScopeDiagnosisConsultation,
				},
			},
			want: "report delivery scope",
		},
		{
			name: "generic_webhook",
			channel: domain.NotificationChannelProfile{
				ID:             33,
				Kind:           domain.NotificationChannelKindWebhook,
				Enabled:        true,
				DeliveryScopes: autoRoomNotificationScopes(),
			},
			want: "Enterprise WeChat",
		},
		{
			name:    "missing_ai_proof",
			channel: readyAutoRoomNotificationChannel(33, []domain.NotificationChannelTestProof{}),
			want:    "ai_diagnosis_sample",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			autoPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
			factory := &fakeTriggerFactory{
				config: &fakeTriggerConfigRepo{
					channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
						tc.channel.ID: tc.channel,
					},
					policies: []domain.ReportWorkflowPolicy{autoPolicy},
					groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{
						grouping.ID: grouping,
					},
				},
				evidence: &fakeTriggerEvidenceRepo{},
			}
			starter := &recordingRoomStarter{}
			service, err := NewService(
				factory,
				starter,
				WithPersistedWindowReplayer(func(context.Context, ports.UnitOfWorkFactory, alertreplay.Request) (alertreplay.Result, error) {
					t.Fatal("replay should not run for an invalid notification channel")
					return alertreplay.Result{}, nil
				}),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			windowStart := time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
			_, err = service.Trigger(ctx, Request{
				AlertSourceProfileID: sourceID,
				WindowStart:          windowStart,
				WindowEnd:            windowStart.Add(time.Minute),
				Limit:                100,
			})
			if !errors.Is(err, domain.ErrInvariantViolation) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Trigger error = %v, want ErrInvariantViolation containing %q", err, tc.want)
			}
			if len(starter.requests) != 0 {
				t.Fatalf("starter requests = %d, want 0", len(starter.requests))
			}
		})
	}
}

func TestStartRoomsUsesExistingSnapshotsWithoutReplay(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	autoPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	autoPolicy.ReportNotificationChannelProfileID = 33
	snapshot := domain.EvidenceSnapshot{
		ID:                77,
		AlertGroupID:      31,
		Digest:            "digest-77",
		Payload:           json.RawMessage(`{"schema_version":"test"}`),
		Provenance:        json.RawMessage(`{}`),
		Status:            domain.SnapshotStatusComplete,
		CreatedByWorkflow: CreatedByWorkflow,
	}
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{},
		evidence: &fakeTriggerEvidenceRepo{
			snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{snapshot.ID: snapshot},
		},
	}
	starter := &recordingRoomStarter{}
	service, err := NewService(
		factory,
		starter,
		WithPersistedWindowReplayer(func(context.Context, ports.UnitOfWorkFactory, alertreplay.Request) (alertreplay.Result, error) {
			t.Fatal("replay should not run when starting rooms for existing snapshots")
			return alertreplay.Result{}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.StartRooms(ctx, StartRoomsRequest{
		AlertSourceProfileID: sourceID,
		Policy:               autoPolicy,
		Snapshots: []alertreplay.SnapshotRef{{
			ID:         snapshot.ID,
			GroupIndex: 0,
			EventCount: 2,
		}},
	})
	if err != nil {
		t.Fatalf("StartRooms: %v", err)
	}
	if result.PoliciesMatched != 1 || len(result.Snapshots) != 1 || len(result.Rooms) != 1 {
		t.Fatalf("result = %+v, want one policy, snapshot, and room", result)
	}
	if len(starter.requests) != 1 {
		t.Fatalf("starter requests = %d, want 1", len(starter.requests))
	}
	gotStart := starter.requests[0]
	if gotStart.SessionID != AutoRoomSessionID(autoPolicy.ID, snapshot.ID) ||
		gotStart.EvidenceSnapshotID != snapshot.ID ||
		gotStart.OwnerSubject != AutoRoomOwnerSubject(sourceID, autoPolicy.ID) ||
		gotStart.CloseNotificationChannelProfileID != autoPolicy.ReportNotificationChannelProfileID ||
		gotStart.InitialTurn == nil {
		t.Fatalf("start request = %+v", gotStart)
	}
}

func TestStartRoomsRejectsAutoRoomPolicyWithoutNotificationChannel(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	autoPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	autoPolicy.ReportNotificationChannelProfileID = 0
	factory := &fakeTriggerFactory{
		config:   &fakeTriggerConfigRepo{},
		evidence: &fakeTriggerEvidenceRepo{},
	}
	starter := &recordingRoomStarter{}
	service, err := NewService(factory, starter)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.StartRooms(ctx, StartRoomsRequest{
		AlertSourceProfileID: sourceID,
		Policy:               autoPolicy,
		Snapshots: []alertreplay.SnapshotRef{{
			ID:         77,
			GroupIndex: 0,
			EventCount: 2,
		}},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("StartRooms error = %v, want ErrInvariantViolation", err)
	}
	if len(starter.requests) != 0 {
		t.Fatalf("starter requests = %d, want 0", len(starter.requests))
	}
}

func TestStartRoomsRejectsAutoRoomPolicyWithUnreadyNotificationChannel(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	autoPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				33: {
					ID:      33,
					Kind:    domain.NotificationChannelKindWeCom,
					Enabled: true,
					DeliveryScopes: []domain.NotificationDeliveryScope{
						domain.NotificationDeliveryScopeReport,
					},
				},
			},
		},
		evidence: &fakeTriggerEvidenceRepo{},
	}
	starter := &recordingRoomStarter{}
	service, err := NewService(factory, starter)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.StartRooms(ctx, StartRoomsRequest{
		AlertSourceProfileID: sourceID,
		Policy:               autoPolicy,
		Snapshots: []alertreplay.SnapshotRef{{
			ID:         77,
			GroupIndex: 0,
			EventCount: 2,
		}},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) || !strings.Contains(err.Error(), "diagnosis_consultation") {
		t.Fatalf("StartRooms error = %v, want diagnosis_consultation invariant violation", err)
	}
	if len(starter.requests) != 0 {
		t.Fatalf("starter requests = %d, want 0", len(starter.requests))
	}
}

func TestStartRoomsCapsRoomStartsPerTrigger(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	autoPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	snapshots := make(map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot)
	refs := make([]alertreplay.SnapshotRef, 0, 5)
	for i := 0; i < 5; i++ {
		id := domain.EvidenceSnapshotID(77 + i)
		snapshots[id] = domain.EvidenceSnapshot{
			ID:                id,
			AlertGroupID:      domain.AlertGroupID(31 + i),
			Digest:            "digest",
			Payload:           json.RawMessage(`{"schema_version":"test"}`),
			Provenance:        json.RawMessage(`{}`),
			Status:            domain.SnapshotStatusComplete,
			CreatedByWorkflow: CreatedByWorkflow,
		}
		refs = append(refs, alertreplay.SnapshotRef{ID: id, GroupIndex: i, EventCount: 2})
	}
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{},
		evidence: &fakeTriggerEvidenceRepo{
			snapshots: snapshots,
		},
	}
	starter := &recordingRoomStarter{}
	service, err := NewService(factory, starter, WithMaxRoomsPerTrigger(2))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.StartRooms(ctx, StartRoomsRequest{
		AlertSourceProfileID: sourceID,
		Policy:               autoPolicy,
		Snapshots:            refs,
	})
	if err != nil {
		t.Fatalf("StartRooms: %v", err)
	}
	if result.PoliciesMatched != 1 ||
		len(result.Snapshots) != len(refs) ||
		len(result.Rooms) != 2 ||
		result.RoomsSkipped != 3 {
		t.Fatalf("result = %+v, want 5 snapshots, 2 rooms, 3 skipped", result)
	}
	if len(starter.requests) != 2 {
		t.Fatalf("starter requests = %d, want 2", len(starter.requests))
	}
	for i, req := range starter.requests {
		wantSnapshotID := refs[i].ID
		if req.EvidenceSnapshotID != wantSnapshotID ||
			req.SessionID != AutoRoomSessionID(autoPolicy.ID, wantSnapshotID) {
			t.Fatalf("starter request %d = %+v, want snapshot %d", i, req, wantSnapshotID)
		}
	}
}

func TestTriggerSharesRoomStartCapAcrossMatchedPolicies(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	firstPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	secondPolicy := mustReportWorkflowPolicy(t, 14, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	grouping := mustGroupingPolicy(t)
	snapshots := make(map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot)
	refs := make([]alertreplay.SnapshotRef, 0, 3)
	for i := 0; i < 3; i++ {
		id := domain.EvidenceSnapshotID(77 + i)
		snapshots[id] = domain.EvidenceSnapshot{
			ID:                id,
			AlertGroupID:      domain.AlertGroupID(31 + i),
			Digest:            "digest",
			Payload:           json.RawMessage(`{"schema_version":"test"}`),
			Provenance:        json.RawMessage(`{}`),
			Status:            domain.SnapshotStatusComplete,
			CreatedByWorkflow: CreatedByWorkflow,
		}
		refs = append(refs, alertreplay.SnapshotRef{ID: id, GroupIndex: i, EventCount: 2})
	}
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{
			policies: []domain.ReportWorkflowPolicy{firstPolicy, secondPolicy},
			groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{
				grouping.ID: grouping,
			},
		},
		evidence: &fakeTriggerEvidenceRepo{
			snapshots: snapshots,
		},
	}
	starter := &recordingRoomStarter{}
	replayCalls := 0
	service, err := NewService(
		factory,
		starter,
		WithMaxRoomsPerTrigger(4),
		WithPersistedWindowReplayer(func(context.Context, ports.UnitOfWorkFactory, alertreplay.Request) (alertreplay.Result, error) {
			replayCalls++
			return alertreplay.Result{Snapshots: cloneSnapshotRefs(refs)}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	windowStart := time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	result, err := service.Trigger(ctx, Request{
		AlertSourceProfileID: sourceID,
		WindowStart:          windowStart,
		WindowEnd:            windowStart.Add(time.Minute),
		Limit:                100,
	})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if replayCalls != 2 ||
		result.PoliciesMatched != 2 ||
		len(result.Snapshots) != 6 ||
		len(result.Rooms) != 4 ||
		result.RoomsSkipped != 2 {
		t.Fatalf("result = %+v replayCalls=%d, want 6 snapshots, 4 rooms, 2 skipped across two policies", result, replayCalls)
	}
	if len(starter.requests) != 4 {
		t.Fatalf("starter requests = %d, want 4", len(starter.requests))
	}
	wantLastSnapshotID := refs[0].ID
	last := starter.requests[3]
	if last.SessionID != AutoRoomSessionID(secondPolicy.ID, wantLastSnapshotID) ||
		last.EvidenceSnapshotID != wantLastSnapshotID {
		t.Fatalf("last starter request = %+v, want first snapshot for second policy", last)
	}
}

func TestStartRoomsRejectsFailedSnapshotBeforeStartingRoom(t *testing.T) {
	ctx := context.Background()
	sourceID := domain.AlertSourceProfileID(7)
	autoPolicy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeAutoRoom)
	snapshot := domain.EvidenceSnapshot{
		ID:                77,
		AlertGroupID:      31,
		Digest:            "digest-77",
		Payload:           json.RawMessage(`{"schema_version":"test"}`),
		Provenance:        json.RawMessage(`{}`),
		Status:            domain.SnapshotStatusFailed,
		CreatedByWorkflow: CreatedByWorkflow,
	}
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{},
		evidence: &fakeTriggerEvidenceRepo{
			snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{snapshot.ID: snapshot},
		},
	}
	starter := &recordingRoomStarter{}
	service, err := NewService(factory, starter)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.StartRooms(ctx, StartRoomsRequest{
		AlertSourceProfileID: sourceID,
		Policy:               autoPolicy,
		Snapshots: []alertreplay.SnapshotRef{{
			ID:         snapshot.ID,
			GroupIndex: 0,
			EventCount: 2,
		}},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("StartRooms error = %v, want ErrInvariantViolation", err)
	}
	if len(starter.requests) != 0 {
		t.Fatalf("starter requests = %d, want 0", len(starter.requests))
	}
}

func TestNewServiceRejectsInvalidMaxRoomsPerTrigger(t *testing.T) {
	tests := []struct {
		name  string
		limit int
	}{
		{name: "zero", limit: 0},
		{name: "negative", limit: -1},
		{name: "too_large", limit: MaxRoomsPerTriggerLimit + 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(&fakeTriggerFactory{}, &recordingRoomStarter{}, WithMaxRoomsPerTrigger(tc.limit))
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("NewService error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestAutoRoomInitialMessageAsksForExecutableAndHumanEvidence(t *testing.T) {
	message := AutoRoomInitialMessage(13, 7, 77)
	for _, fragment := range []string{
		"evidence snapshot 77",
		"initial diagnosis report",
		"what executable evidence and operator-supplied evidence can raise confidence",
		"first operator notification must be the AI diagnosis report with evidence requests",
		"not a raw alert forward or final conclusion",
		"openclarion_available_diagnosis_tools",
		"evidence_request_example",
		"evidence_requests",
		"missing_evidence_requests",
		"operator-provided evidence",
		"collected evidence or reviewed supplemental evidence supports ready_for_review",
		"Do not mark the first automatic turn final",
		"keep confidence low or medium",
		"Do not invent evidence outside the snapshot",
	} {
		t.Run(fragment, func(t *testing.T) {
			if !strings.Contains(message, fragment) {
				t.Fatalf("AutoRoomInitialMessage() = %q, want fragment %q", message, fragment)
			}
		})
	}
}

func TestTriggerSkipsWhenNoAutoRoomPolicyMatches(t *testing.T) {
	sourceID := domain.AlertSourceProfileID(7)
	policy := mustReportWorkflowPolicy(t, 13, sourceID, domain.DiagnosisFollowUpModeSuggestRoom)
	factory := &fakeTriggerFactory{
		config: &fakeTriggerConfigRepo{policies: []domain.ReportWorkflowPolicy{policy}},
	}
	starter := &recordingRoomStarter{}
	service, err := NewService(
		factory,
		starter,
		WithPersistedWindowReplayer(func(context.Context, ports.UnitOfWorkFactory, alertreplay.Request) (alertreplay.Result, error) {
			t.Fatal("replay should not run without a matching auto_room policy")
			return alertreplay.Result{}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	windowStart := time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	result, err := service.Trigger(context.Background(), Request{
		AlertSourceProfileID: sourceID,
		WindowStart:          windowStart,
		WindowEnd:            windowStart.Add(time.Minute),
		Limit:                100,
	})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if result.PoliciesMatched != 0 || len(result.Snapshots) != 0 || len(result.Rooms) != 0 || len(starter.requests) != 0 {
		t.Fatalf("result/starter = %+v/%d, want no work", result, len(starter.requests))
	}
}

func TestTriggerRejectsInvalidRequest(t *testing.T) {
	service, err := NewService(&fakeTriggerFactory{}, &recordingRoomStarter{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Trigger(context.Background(), Request{Limit: 100})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Trigger err = %v, want ErrInvariantViolation", err)
	}
}

func mustReportWorkflowPolicy(
	t *testing.T,
	id domain.ReportWorkflowPolicyID,
	sourceID domain.AlertSourceProfileID,
	followUp domain.DiagnosisFollowUpMode,
) domain.ReportWorkflowPolicy {
	t.Helper()
	policy, err := domain.NewReportWorkflowPolicy(
		"policy",
		sourceID,
		domain.GroupingPolicyID(21),
		0,
		domain.ReportWorkflowTriggerModeManualReplay,
		domain.ReportWorkflowScenarioSingleAlert,
		followUp,
		true,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowPolicy: %v", err)
	}
	policy.ID = id
	if followUp == domain.DiagnosisFollowUpModeAutoRoom {
		policy.ReportNotificationChannelProfileID = 33
	}
	return policy
}

func mustGroupingPolicy(t *testing.T) domain.GroupingPolicy {
	t.Helper()
	policy, err := domain.NewGroupingPolicy(
		"grouping",
		[]string{"alertname"},
		"severity",
		[]string{"alertmanager"},
		true,
	)
	if err != nil {
		t.Fatalf("NewGroupingPolicy: %v", err)
	}
	policy.ID = 21
	return policy
}

func mustTriggerToolTemplate(
	t *testing.T,
	id domain.DiagnosisToolTemplateID,
	sourceID domain.AlertSourceProfileID,
) domain.DiagnosisToolTemplate {
	t.Helper()
	enabledAt := time.Date(2026, 6, 18, 1, 2, 3, 0, time.UTC)
	template, err := domain.NewDiagnosisToolTemplate(
		"Active alerts",
		sourceID,
		domain.DiagnosisToolKindActiveAlerts,
		"",
		10,
		0,
		0,
		0,
		true,
		&enabledAt,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	template.ID = id
	return template
}

func mustTriggerAlertSource(t *testing.T, id domain.AlertSourceProfileID) domain.AlertSourceProfile {
	t.Helper()
	source, err := domain.NewAlertSourceProfile(
		"Alertmanager",
		domain.AlertSourceKindAlertmanager,
		"https://alertmanager.example.invalid",
		domain.AlertSourceAuthModeBearer,
		"secret/alertmanager",
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	source.ID = id
	return source
}

func autoRoomNotificationScopes() []domain.NotificationDeliveryScope {
	return []domain.NotificationDeliveryScope{
		domain.NotificationDeliveryScopeDiagnosisClose,
		domain.NotificationDeliveryScopeDiagnosisConsultation,
		domain.NotificationDeliveryScopeReport,
	}
}

func readyAutoRoomNotificationChannel(
	id domain.NotificationChannelProfileID,
	proofs []domain.NotificationChannelTestProof,
) domain.NotificationChannelProfile {
	updatedAt := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	channel := domain.NotificationChannelProfile{
		ID:             id,
		Kind:           domain.NotificationChannelKindWeCom,
		Enabled:        true,
		DeliveryScopes: autoRoomNotificationScopes(),
		UpdatedAt:      updatedAt,
	}
	if proofs == nil {
		channel.LatestTestProofs = []domain.NotificationChannelTestProof{
			autoRoomNotificationProof(id, domain.NotificationChannelTestContentAIDiagnosisSample, updatedAt.Add(time.Minute)),
			autoRoomNotificationProof(id, domain.NotificationChannelTestContentDiagnosisCloseSample, updatedAt.Add(time.Minute)),
		}
		return channel
	}
	channel.LatestTestProofs = proofs
	return channel
}

func autoRoomNotificationProof(
	id domain.NotificationChannelProfileID,
	contentKind domain.NotificationChannelTestContentKind,
	checkedAt time.Time,
) domain.NotificationChannelTestProof {
	return domain.NotificationChannelTestProof{
		NotificationChannelProfileID: id,
		Kind:                         domain.NotificationChannelKindWeCom,
		Status:                       domain.NotificationChannelTestStatusSuccess,
		ReasonCode:                   domain.NotificationChannelTestReasonOK,
		Message:                      "Notification channel test delivery succeeded.",
		ContentKind:                  contentKind,
		ContentSHA256:                strings.Repeat("a", 64),
		CheckedAt:                    checkedAt,
		ProviderMessageID:            "provider-message-1",
		ProviderStatus:               "delivered",
	}
}

type recordingRoomStarter struct {
	requests []ports.DiagnosisRoomStartRequest
}

func (s *recordingRoomStarter) StartDiagnosisRoom(_ context.Context, req ports.DiagnosisRoomStartRequest) (ports.DiagnosisRoomStartResult, error) {
	s.requests = append(s.requests, req)
	return ports.DiagnosisRoomStartResult{
		SessionID:          req.SessionID,
		EvidenceSnapshotID: req.EvidenceSnapshotID,
		DiagnosisTaskID:    1001,
		ChatSessionID:      2002,
		Workflow:           ports.WorkflowHandle{WorkflowID: "diagnosis-room/" + req.SessionID, RunID: "run-1"},
	}, nil
}

type fakeTriggerFactory struct {
	config   *fakeTriggerConfigRepo
	evidence *fakeTriggerEvidenceRepo
}

func (f *fakeTriggerFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return &fakeTriggerUOW{config: f.config, evidence: f.evidence}, nil
}

func (f *fakeTriggerFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, &fakeTriggerUOW{config: f.config, evidence: f.evidence})
}

type fakeTriggerUOW struct {
	config   *fakeTriggerConfigRepo
	evidence *fakeTriggerEvidenceRepo
}

func (u *fakeTriggerUOW) Alerts() ports.AlertRepository         { return nil }
func (u *fakeTriggerUOW) Evidence() ports.EvidenceRepository    { return u.evidence }
func (u *fakeTriggerUOW) Diagnosis() ports.DiagnosisRepository  { return nil }
func (u *fakeTriggerUOW) Reports() ports.ReportRepository       { return nil }
func (u *fakeTriggerUOW) Config() ports.ConfigurationRepository { return u.config }
func (u *fakeTriggerUOW) Directory() ports.DirectoryRepository  { return nil }
func (u *fakeTriggerUOW) RBAC() ports.RBACRepository            { return nil }
func (u *fakeTriggerUOW) Commit(context.Context) error          { return nil }
func (u *fakeTriggerUOW) Rollback(context.Context) error        { return nil }

type fakeTriggerConfigRepo struct {
	ports.ConfigurationRepository
	policies  []domain.ReportWorkflowPolicy
	groupings map[domain.GroupingPolicyID]domain.GroupingPolicy
	templates []domain.DiagnosisToolTemplate
	sources   map[domain.AlertSourceProfileID]domain.AlertSourceProfile
	channels  map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile
}

func (r *fakeTriggerConfigRepo) SaveAlertSourceProfile(context.Context, domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	return domain.AlertSourceProfile{}, nil
}
func (r *fakeTriggerConfigRepo) UpdateAlertSourceProfile(context.Context, domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	return domain.AlertSourceProfile{}, nil
}
func (r *fakeTriggerConfigRepo) FindAlertSourceProfileByID(
	_ context.Context,
	id domain.AlertSourceProfileID,
) (domain.AlertSourceProfile, error) {
	source, ok := r.sources[id]
	if !ok {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return source, nil
}
func (r *fakeTriggerConfigRepo) ListAlertSourceProfiles(context.Context, int) ([]domain.AlertSourceProfile, error) {
	return nil, nil
}
func (r *fakeTriggerConfigRepo) SaveGroupingPolicy(context.Context, domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	return domain.GroupingPolicy{}, nil
}
func (r *fakeTriggerConfigRepo) UpdateGroupingPolicy(context.Context, domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	return domain.GroupingPolicy{}, nil
}
func (r *fakeTriggerConfigRepo) FindGroupingPolicyByID(_ context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	grouping, ok := r.groupings[id]
	if !ok {
		return domain.GroupingPolicy{}, domain.ErrNotFound
	}
	return grouping, nil
}
func (r *fakeTriggerConfigRepo) ListGroupingPolicies(context.Context, int) ([]domain.GroupingPolicy, error) {
	return nil, nil
}
func (r *fakeTriggerConfigRepo) SaveReportWorkflowPolicy(context.Context, domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	return domain.ReportWorkflowPolicy{}, nil
}
func (r *fakeTriggerConfigRepo) UpdateReportWorkflowPolicy(context.Context, domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	return domain.ReportWorkflowPolicy{}, nil
}
func (r *fakeTriggerConfigRepo) FindReportWorkflowPolicyByID(context.Context, domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
}
func (r *fakeTriggerConfigRepo) ListReportWorkflowPolicies(context.Context, int) ([]domain.ReportWorkflowPolicy, error) {
	return append([]domain.ReportWorkflowPolicy(nil), r.policies...), nil
}
func (r *fakeTriggerConfigRepo) SaveReportWorkflowSchedule(context.Context, domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	return domain.ReportWorkflowSchedule{}, nil
}
func (r *fakeTriggerConfigRepo) UpdateReportWorkflowSchedule(context.Context, domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	return domain.ReportWorkflowSchedule{}, nil
}
func (r *fakeTriggerConfigRepo) FindReportWorkflowScheduleByID(context.Context, domain.ReportWorkflowScheduleID) (domain.ReportWorkflowSchedule, error) {
	return domain.ReportWorkflowSchedule{}, domain.ErrNotFound
}
func (r *fakeTriggerConfigRepo) ListReportWorkflowSchedules(context.Context, int) ([]domain.ReportWorkflowSchedule, error) {
	return nil, nil
}
func (r *fakeTriggerConfigRepo) SaveDiagnosisToolTemplate(context.Context, domain.DiagnosisToolTemplate) (domain.DiagnosisToolTemplate, error) {
	return domain.DiagnosisToolTemplate{}, nil
}
func (r *fakeTriggerConfigRepo) UpdateDiagnosisToolTemplate(context.Context, domain.DiagnosisToolTemplate) (domain.DiagnosisToolTemplate, error) {
	return domain.DiagnosisToolTemplate{}, nil
}
func (r *fakeTriggerConfigRepo) FindDiagnosisToolTemplateByID(context.Context, domain.DiagnosisToolTemplateID) (domain.DiagnosisToolTemplate, error) {
	return domain.DiagnosisToolTemplate{}, domain.ErrNotFound
}
func (r *fakeTriggerConfigRepo) ListDiagnosisToolTemplates(context.Context, int) ([]domain.DiagnosisToolTemplate, error) {
	return append([]domain.DiagnosisToolTemplate(nil), r.templates...), nil
}
func (r *fakeTriggerConfigRepo) SaveNotificationChannelProfile(context.Context, domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	return domain.NotificationChannelProfile{}, nil
}
func (r *fakeTriggerConfigRepo) UpdateNotificationChannelProfile(context.Context, domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	return domain.NotificationChannelProfile{}, nil
}
func (r *fakeTriggerConfigRepo) FindNotificationChannelProfileByID(
	_ context.Context,
	id domain.NotificationChannelProfileID,
) (domain.NotificationChannelProfile, error) {
	if r.channels != nil {
		if channel, ok := r.channels[id]; ok {
			return channel, nil
		}
		return domain.NotificationChannelProfile{}, domain.ErrNotFound
	}
	if id == 33 {
		return readyAutoRoomNotificationChannel(33, nil), nil
	}
	return domain.NotificationChannelProfile{}, domain.ErrNotFound
}
func (r *fakeTriggerConfigRepo) ListNotificationChannelProfiles(context.Context, int) ([]domain.NotificationChannelProfile, error) {
	return nil, nil
}
func (r *fakeTriggerConfigRepo) SaveNotificationChannelTestProof(context.Context, domain.NotificationChannelTestProof) (domain.NotificationChannelTestProof, error) {
	return domain.NotificationChannelTestProof{}, nil
}
func (r *fakeTriggerConfigRepo) ListLatestNotificationChannelTestProofs(context.Context, domain.NotificationChannelProfileID, int) ([]domain.NotificationChannelTestProof, error) {
	return nil, nil
}

type fakeTriggerEvidenceRepo struct {
	snapshots map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot
}

func (r *fakeTriggerEvidenceRepo) Save(context.Context, domain.EvidenceSnapshot) (domain.EvidenceSnapshot, error) {
	return domain.EvidenceSnapshot{}, nil
}
func (r *fakeTriggerEvidenceRepo) FindByID(_ context.Context, id domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error) {
	snapshot, ok := r.snapshots[id]
	if !ok {
		return domain.EvidenceSnapshot{}, domain.ErrNotFound
	}
	return snapshot, nil
}
func (r *fakeTriggerEvidenceRepo) FindByGroupAndDigest(context.Context, domain.AlertGroupID, string) (domain.EvidenceSnapshot, error) {
	return domain.EvidenceSnapshot{}, domain.ErrNotFound
}
func (r *fakeTriggerEvidenceRepo) ListByGroup(context.Context, domain.AlertGroupID, int) ([]domain.EvidenceSnapshot, error) {
	return nil, nil
}
func (r *fakeTriggerEvidenceRepo) List(context.Context, int) ([]domain.EvidenceSnapshot, error) {
	return nil, nil
}
