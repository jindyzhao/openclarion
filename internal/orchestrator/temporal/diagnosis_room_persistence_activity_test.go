package temporal_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisapproval"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestDiagnosisRoomPersistenceActivities_EnsureSessionIsIdempotent(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-ensure-session")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	req := temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-ensure",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       time.Date(2026, 5, 28, 15, 0, 0, 0, time.UTC),
	}

	first, err := activities.EnsureDiagnosisChatSession(ctx, req)
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession first: %v", err)
	}
	second, err := activities.EnsureDiagnosisChatSession(ctx, req)
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession second: %v", err)
	}
	if first.ChatSessionID == 0 || second.ChatSessionID != first.ChatSessionID {
		t.Fatalf("session IDs first=%d second=%d", first.ChatSessionID, second.ChatSessionID)
	}
	if first.LifecycleEventID == 0 || second.LifecycleEventID != first.LifecycleEventID {
		t.Fatalf("lifecycle event IDs first=%d second=%d", first.LifecycleEventID, second.LifecycleEventID)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if session.ID != domain.ChatSessionID(first.ChatSessionID) ||
			session.DiagnosisTaskID != seed.TaskID ||
			session.OwnerSubject != "owner-1" ||
			session.TurnCount != 0 ||
			session.Status != domain.ChatSessionStatusOpen {
			t.Fatalf("stored session = %+v", session)
		}
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0].Kind != "diagnosis_room.opened" || events[0].ID != domain.DiagnosisTaskEventID(first.LifecycleEventID) {
			t.Fatalf("events = %+v, want one opened event", events)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify session: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_EnsureRoomSessionCreatesTaskAndSession(t *testing.T) {
	snapshotID := seedEvidenceSnapshot(t, "room-start-session")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	req := temporalpkg.EnsureDiagnosisRoomSessionInput{
		SessionID:          "session-room-start",
		EvidenceSnapshotID: int64(snapshotID),
		WorkflowID:         "diagnosis-room-session-room-start",
		RunID:              "run-room-start",
		OwnerSubject:       "owner-1",
		StartedAt:          time.Date(2026, 5, 28, 15, 10, 0, 0, time.UTC),
	}

	first, err := activities.EnsureDiagnosisRoomSession(ctx, req)
	if err != nil {
		t.Fatalf("EnsureDiagnosisRoomSession first: %v", err)
	}
	second, err := activities.EnsureDiagnosisRoomSession(ctx, req)
	if err != nil {
		t.Fatalf("EnsureDiagnosisRoomSession second: %v", err)
	}
	if first.DiagnosisTaskID == 0 || second.DiagnosisTaskID != first.DiagnosisTaskID {
		t.Fatalf("task IDs first=%d second=%d", first.DiagnosisTaskID, second.DiagnosisTaskID)
	}
	if first.ChatSessionID == 0 || second.ChatSessionID != first.ChatSessionID {
		t.Fatalf("session IDs first=%d second=%d", first.ChatSessionID, second.ChatSessionID)
	}
	if first.LifecycleEventID == 0 || second.LifecycleEventID != first.LifecycleEventID {
		t.Fatalf("lifecycle event IDs first=%d second=%d", first.LifecycleEventID, second.LifecycleEventID)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		task, err := uow.Diagnosis().FindTaskByID(ctx, domain.DiagnosisTaskID(first.DiagnosisTaskID))
		if err != nil {
			return err
		}
		if task.EvidenceSnapshotID != snapshotID ||
			task.WorkflowID != req.WorkflowID ||
			task.RunID != req.RunID ||
			task.Status != domain.DiagnosisStatusRunning ||
			task.StartedAt == nil ||
			!task.StartedAt.Equal(domain.NormalizeUTCMicro(req.StartedAt)) {
			t.Fatalf("stored task = %+v", task)
		}
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if session.ID != domain.ChatSessionID(first.ChatSessionID) ||
			session.DiagnosisTaskID != task.ID ||
			session.OwnerSubject != "owner-1" ||
			session.Status != domain.ChatSessionStatusOpen {
			t.Fatalf("stored session = %+v", session)
		}
		events, err := uow.Diagnosis().ListEvents(ctx, task.ID, 10)
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0].Kind != "diagnosis_room.opened" || events[0].ID != domain.DiagnosisTaskEventID(first.LifecycleEventID) {
			t.Fatalf("events = %+v, want one opened event", events)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify room session: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_PersistTurnIsIdempotent(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-persist-turn")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 15, 30, 0, 0, time.UTC)
	ensure, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-persist",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}

	req := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-persist", startedAt)
	first, err := activities.PersistDiagnosisTurn(ctx, req)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn first: %v", err)
	}
	second, err := activities.PersistDiagnosisTurn(ctx, req)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn second: %v", err)
	}
	if first.ChatSessionID != ensure.ChatSessionID ||
		first.LifecycleEventID == 0 ||
		second.LifecycleEventID != first.LifecycleEventID ||
		first.UserTurnID == 0 ||
		first.AssistantTurnID == 0 ||
		second.UserTurnID != first.UserTurnID ||
		second.AssistantTurnID != first.AssistantTurnID ||
		first.TurnCount != 1 ||
		second.TurnCount != 1 ||
		len(first.EvidenceRequests) != 1 ||
		first.EvidenceRequests[0].Tool != domain.DiagnosisToolKindActiveAlerts ||
		first.Insight.ConfidenceRationale != "Confidence depends on sibling alert and restart evidence." ||
		len(first.Insight.MissingEvidenceRequests) != 1 ||
		first.Insight.MissingEvidenceRequests[0].Label != "Restart cause" {
		t.Fatalf("persist results first=%+v second=%+v ensure=%+v", first, second, ensure)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if session.TurnCount != 1 || !session.LastActivityAt.Equal(domain.NormalizeUTCMicro(req.AssistantOccurredAt)) {
			t.Fatalf("session after persist = %+v", session)
		}
		turns, err := uow.Diagnosis().ListChatTurnsBySession(ctx, session.ID, 10)
		if err != nil {
			return err
		}
		if len(turns) != 2 {
			t.Fatalf("turns len = %d, want 2: %+v", len(turns), turns)
		}
		if turns[0].MessageID != req.UserMessageID ||
			turns[0].Role != domain.ChatRoleUser ||
			turns[0].Sequence != 1 ||
			turns[1].MessageID != req.AssistantMessageID ||
			turns[1].Role != domain.ChatRoleAssistant ||
			turns[1].Sequence != 2 {
			t.Fatalf("turns = %+v", turns)
		}
		var assistantMeta struct {
			InvocationID        string                            `json:"invocation_id"`
			Confidence          string                            `json:"confidence"`
			RequiresHumanReview bool                              `json:"requires_human_review"`
			EvidenceRequests    []diagnosisroom.EvidenceRequest   `json:"evidence_requests"`
			ConsultationInsight diagnosisroom.ConsultationInsight `json:"consultation_insight"`
		}
		if err := json.Unmarshal(turns[1].Metadata, &assistantMeta); err != nil {
			t.Fatalf("assistant metadata: %v", err)
		}
		if assistantMeta.InvocationID != req.InvocationID ||
			assistantMeta.Confidence != "high" ||
			!assistantMeta.RequiresHumanReview ||
			len(assistantMeta.EvidenceRequests) != 1 ||
			assistantMeta.EvidenceRequests[0].Reason != "Need current active sibling alerts." ||
			assistantMeta.ConsultationInsight.ConfidenceRationale != "Confidence depends on sibling alert and restart evidence." ||
			len(assistantMeta.ConsultationInsight.EvidenceCollectionSuggestions) != 1 ||
			assistantMeta.ConsultationInsight.EvidenceCollectionSuggestions[0].Label != "CPU trend" {
			t.Fatalf("assistant metadata = %+v raw=%s", assistantMeta, turns[1].Metadata)
		}
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 2 || events[0].Kind != "diagnosis_room.opened" || events[1].Kind != "diagnosis_room.turn_persisted" {
			t.Fatalf("events = %+v, want opened + turn_persisted", events)
		}
		var turnPayload struct {
			UserMessageID       string                            `json:"user_message_id"`
			AssistantMessageID  string                            `json:"assistant_message_id"`
			TurnCount           int                               `json:"turn_count"`
			EvidenceRequests    []diagnosisroom.EvidenceRequest   `json:"evidence_requests"`
			ConsultationInsight diagnosisroom.ConsultationInsight `json:"consultation_insight"`
		}
		if err := json.Unmarshal(events[1].Payload, &turnPayload); err != nil {
			t.Fatalf("turn event payload: %v", err)
		}
		if turnPayload.UserMessageID != req.UserMessageID ||
			turnPayload.AssistantMessageID != req.AssistantMessageID ||
			turnPayload.TurnCount != 1 ||
			len(turnPayload.EvidenceRequests) != 1 ||
			turnPayload.EvidenceRequests[0].Limit != 5 ||
			turnPayload.ConsultationInsight.ConclusionStatus != "needs_evidence" {
			t.Fatalf("turn event payload = %+v raw=%s", turnPayload, events[1].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify persisted turns: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_RecordEvidenceCollectedIsIdempotent(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-evidence-collected")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 15, 40, 0, 0, time.UTC)
	if _, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-evidence-collected",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	}); err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	turnReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-evidence-collected", startedAt)
	persisted, err := activities.PersistDiagnosisTurn(ctx, turnReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn: %v", err)
	}
	collectedAt := startedAt.Add(90 * time.Second)
	recordReq := temporalpkg.RecordDiagnosisEvidenceCollectedInput{
		SessionID:          turnReq.SessionID,
		DiagnosisTaskID:    int64(seed.TaskID),
		ChatSessionID:      persisted.ChatSessionID,
		OwnerSubject:       turnReq.OwnerSubject,
		ActorSubject:       turnReq.ActorSubject,
		UserMessageID:      turnReq.UserMessageID,
		AssistantMessageID: turnReq.AssistantMessageID,
		UserTurnID:         persisted.UserTurnID,
		AssistantTurnID:    persisted.AssistantTurnID,
		UserSequence:       turnReq.UserSequence,
		AssistantSequence:  turnReq.AssistantSequence,
		TurnCount:          persisted.TurnCount,
		Items: []diagnosisevidence.Item{{
			Request: diagnosisroom.EvidenceRequest{
				Tool:   domain.DiagnosisToolKindActiveAlerts,
				Reason: "Need current active sibling alerts.",
				Limit:  5,
			},
			TemplateID:           domain.DiagnosisToolTemplateID(12),
			AlertSourceProfileID: domain.AlertSourceProfileID(13),
			AlertSourceKind:      domain.AlertSourceKindPrometheus,
			Tool:                 domain.DiagnosisToolKindActiveAlerts,
			Status:               diagnosisevidence.StatusCollected,
			ReasonCode:           diagnosisevidence.ReasonOK,
			Message:              "Active alert collection succeeded.",
			Limit:                5,
			ObservedAlerts:       2,
			CollectedAt:          collectedAt,
		}},
		OccurredAt: collectedAt,
	}
	first, err := activities.RecordDiagnosisEvidenceCollected(ctx, recordReq)
	if err != nil {
		t.Fatalf("RecordDiagnosisEvidenceCollected first: %v", err)
	}
	second, err := activities.RecordDiagnosisEvidenceCollected(ctx, recordReq)
	if err != nil {
		t.Fatalf("RecordDiagnosisEvidenceCollected second: %v", err)
	}
	if first.LifecycleEventID == 0 || second.LifecycleEventID != first.LifecycleEventID {
		t.Fatalf("record results first=%+v second=%+v", first, second)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 3 ||
			events[0].Kind != "diagnosis_room.opened" ||
			events[1].Kind != "diagnosis_room.turn_persisted" ||
			events[2].Kind != "diagnosis_room.evidence_collected" {
			t.Fatalf("events = %+v, want opened + turn_persisted + evidence_collected", events)
		}
		var payload struct {
			AssistantMessageID        string `json:"assistant_message_id"`
			AssistantTurnID           int64  `json:"assistant_turn_id"`
			TurnCount                 int    `json:"turn_count"`
			EvidenceCollectionResults []struct {
				TemplateID           int64      `json:"template_id"`
				AlertSourceProfileID int64      `json:"alert_source_profile_id"`
				AlertSourceKind      string     `json:"alert_source_kind"`
				Tool                 string     `json:"tool"`
				Status               string     `json:"status"`
				ReasonCode           string     `json:"reason_code"`
				RequestReason        string     `json:"request_reason"`
				Limit                int        `json:"limit"`
				ObservedAlerts       *int       `json:"observed_alerts"`
				CollectedAt          *time.Time `json:"collected_at"`
			} `json:"evidence_collection_results"`
		}
		if err := json.Unmarshal(events[2].Payload, &payload); err != nil {
			t.Fatalf("evidence collected payload: %v", err)
		}
		if payload.AssistantMessageID != turnReq.AssistantMessageID ||
			payload.AssistantTurnID != persisted.AssistantTurnID ||
			payload.TurnCount != 1 ||
			len(payload.EvidenceCollectionResults) != 1 ||
			payload.EvidenceCollectionResults[0].TemplateID != 12 ||
			payload.EvidenceCollectionResults[0].AlertSourceProfileID != 13 ||
			payload.EvidenceCollectionResults[0].AlertSourceKind != string(domain.AlertSourceKindPrometheus) ||
			payload.EvidenceCollectionResults[0].Tool != string(domain.DiagnosisToolKindActiveAlerts) ||
			payload.EvidenceCollectionResults[0].Status != string(diagnosisevidence.StatusCollected) ||
			payload.EvidenceCollectionResults[0].ReasonCode != string(diagnosisevidence.ReasonOK) ||
			payload.EvidenceCollectionResults[0].RequestReason != "Need current active sibling alerts." ||
			payload.EvidenceCollectionResults[0].Limit != 5 ||
			payload.EvidenceCollectionResults[0].ObservedAlerts == nil ||
			*payload.EvidenceCollectionResults[0].ObservedAlerts != 2 ||
			payload.EvidenceCollectionResults[0].CollectedAt == nil ||
			!payload.EvidenceCollectionResults[0].CollectedAt.Equal(domain.NormalizeUTCMicro(collectedAt)) {
			t.Fatalf("evidence collected payload = %+v raw=%s", payload, events[2].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify evidence collected event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_RecordManualEvidenceCollectedIsIdempotent(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-manual-evidence-collected")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 15, 43, 0, 0, time.UTC)
	if _, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-manual-evidence-collected",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	}); err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	turnReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-manual-evidence-collected", startedAt)
	persisted, err := activities.PersistDiagnosisTurn(ctx, turnReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn: %v", err)
	}
	collectedAt := startedAt.Add(2 * time.Minute)
	recordReq := temporalpkg.RecordDiagnosisEvidenceCollectedInput{
		SessionID:       turnReq.SessionID,
		DiagnosisTaskID: int64(seed.TaskID),
		ChatSessionID:   persisted.ChatSessionID,
		OwnerSubject:    turnReq.OwnerSubject,
		ActorSubject:    "owner-1",
		UserMessageID:   "collect-manual-1",
		TurnCount:       persisted.TurnCount,
		Items: []diagnosisevidence.Item{{
			Request: diagnosisroom.EvidenceRequest{
				Tool:          domain.DiagnosisToolKindMetricQuery,
				Reason:        "Need current API availability.",
				Query:         `up{job="api"}`,
				TemplateID:    15,
				WindowSeconds: 0,
				Limit:         3,
			},
			TemplateID:           domain.DiagnosisToolTemplateID(15),
			AlertSourceProfileID: domain.AlertSourceProfileID(13),
			AlertSourceKind:      domain.AlertSourceKindPrometheus,
			Tool:                 domain.DiagnosisToolKindMetricQuery,
			Status:               diagnosisevidence.StatusCollected,
			ReasonCode:           diagnosisevidence.ReasonOK,
			Message:              "Metric query collection succeeded.",
			Query:                `up{job="api"}`,
			Limit:                3,
			ObservedMetricSeries: 1,
			CollectedAt:          collectedAt,
		}},
		OccurredAt: collectedAt,
	}
	first, err := activities.RecordDiagnosisEvidenceCollected(ctx, recordReq)
	if err != nil {
		t.Fatalf("RecordDiagnosisEvidenceCollected first: %v", err)
	}
	second, err := activities.RecordDiagnosisEvidenceCollected(ctx, recordReq)
	if err != nil {
		t.Fatalf("RecordDiagnosisEvidenceCollected second: %v", err)
	}
	if first.LifecycleEventID == 0 || second.LifecycleEventID != first.LifecycleEventID {
		t.Fatalf("record results first=%+v second=%+v", first, second)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 3 ||
			events[0].Kind != "diagnosis_room.opened" ||
			events[1].Kind != "diagnosis_room.turn_persisted" ||
			events[2].Kind != "diagnosis_room.evidence_collected" {
			t.Fatalf("events = %+v, want opened + turn_persisted + manual evidence_collected", events)
		}
		if strings.Contains(string(events[2].Payload), "assistant_message_id") ||
			strings.Contains(string(events[2].Payload), "context_refs") {
			t.Fatalf("manual evidence payload should not include assistant turn binding: %s", events[2].Payload)
		}
		var payload struct {
			ActorSubject              string `json:"actor_subject"`
			UserMessageID             string `json:"user_message_id"`
			TurnCount                 int    `json:"turn_count"`
			EvidenceCollectionResults []struct {
				TemplateID           int64      `json:"template_id"`
				AlertSourceProfileID int64      `json:"alert_source_profile_id"`
				AlertSourceKind      string     `json:"alert_source_kind"`
				Tool                 string     `json:"tool"`
				Status               string     `json:"status"`
				ReasonCode           string     `json:"reason_code"`
				RequestReason        string     `json:"request_reason"`
				Query                string     `json:"query"`
				Limit                int        `json:"limit"`
				ObservedMetricSeries *int       `json:"observed_metric_series"`
				CollectedAt          *time.Time `json:"collected_at"`
			} `json:"evidence_collection_results"`
		}
		if err := json.Unmarshal(events[2].Payload, &payload); err != nil {
			t.Fatalf("manual evidence collected payload: %v", err)
		}
		if payload.ActorSubject != "owner-1" ||
			payload.UserMessageID != "collect-manual-1" ||
			payload.TurnCount != persisted.TurnCount ||
			len(payload.EvidenceCollectionResults) != 1 ||
			payload.EvidenceCollectionResults[0].TemplateID != 15 ||
			payload.EvidenceCollectionResults[0].AlertSourceProfileID != 13 ||
			payload.EvidenceCollectionResults[0].AlertSourceKind != string(domain.AlertSourceKindPrometheus) ||
			payload.EvidenceCollectionResults[0].Tool != string(domain.DiagnosisToolKindMetricQuery) ||
			payload.EvidenceCollectionResults[0].Status != string(diagnosisevidence.StatusCollected) ||
			payload.EvidenceCollectionResults[0].ReasonCode != string(diagnosisevidence.ReasonOK) ||
			payload.EvidenceCollectionResults[0].RequestReason != "Need current API availability." ||
			payload.EvidenceCollectionResults[0].Query != `up{job="api"}` ||
			payload.EvidenceCollectionResults[0].Limit != 3 ||
			payload.EvidenceCollectionResults[0].ObservedMetricSeries == nil ||
			*payload.EvidenceCollectionResults[0].ObservedMetricSeries != 1 ||
			payload.EvidenceCollectionResults[0].CollectedAt == nil ||
			!payload.EvidenceCollectionResults[0].CollectedAt.Equal(domain.NormalizeUTCMicro(collectedAt)) {
			t.Fatalf("manual evidence collected payload = %+v raw=%s", payload, events[2].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify manual evidence collected event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_PersistTurnAuditsSupplementalEvidence(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-supplemental-evidence")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 15, 45, 0, 0, time.UTC)
	if _, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-supplemental",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	}); err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}

	req := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-supplemental", startedAt)
	req.UserMessageID = "msg-supplemental-1"
	req.AssistantMessageID = "msg-supplemental-1/assistant"
	req.UserMessage = "Supplemental evidence update\n\nEvidence provided:\n- previous pod logs show OOMKilled"
	req.SupplementalEvidence = &temporalpkg.DiagnosisRoomSupplementalEvidence{
		Label:    "Restart cause",
		Detail:   "Inspect previous container logs.",
		Priority: "high",
		Evidence: "previous pod logs show OOMKilled",
	}
	first, err := activities.PersistDiagnosisTurn(ctx, req)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn first: %v", err)
	}
	second, err := activities.PersistDiagnosisTurn(ctx, req)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn second: %v", err)
	}
	if second.UserTurnID != first.UserTurnID || second.AssistantTurnID != first.AssistantTurnID {
		t.Fatalf("persist results first=%+v second=%+v", first, second)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 3 ||
			events[0].Kind != "diagnosis_room.opened" ||
			events[1].Kind != "diagnosis_room.supplemental_evidence_provided" ||
			events[2].Kind != "diagnosis_room.turn_persisted" {
			t.Fatalf("events = %+v, want opened + supplemental + turn_persisted", events)
		}
		var payload struct {
			UserMessageID        string                                        `json:"user_message_id"`
			AssistantMessageID   string                                        `json:"assistant_message_id"`
			ContextRefs          []string                                      `json:"context_refs"`
			SupplementalEvidence temporalpkg.DiagnosisRoomSupplementalEvidence `json:"supplemental_evidence"`
			Confidence           string                                        `json:"confidence"`
			RequiresHumanReview  bool                                          `json:"requires_human_review"`
		}
		if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
			t.Fatalf("supplemental event payload: %v", err)
		}
		if payload.UserMessageID != req.UserMessageID ||
			payload.AssistantMessageID != req.AssistantMessageID ||
			len(payload.ContextRefs) != 2 ||
			payload.ContextRefs[0] == "" ||
			payload.SupplementalEvidence.Label != "Restart cause" ||
			payload.SupplementalEvidence.Detail != "Inspect previous container logs." ||
			payload.SupplementalEvidence.Priority != "high" ||
			payload.SupplementalEvidence.Evidence != "previous pod logs show OOMKilled" ||
			payload.Confidence != "high" ||
			!payload.RequiresHumanReview {
			t.Fatalf("supplemental event payload = %+v raw=%s", payload, events[1].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify supplemental event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_PersistTurnRejectsUnsupportedSupplementalEvidencePriority(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-supplemental-evidence-bad-priority")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 15, 45, 0, 0, time.UTC)
	if _, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-supplemental-bad-priority",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	}); err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}

	req := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-supplemental-bad-priority", startedAt)
	req.SupplementalEvidence = &temporalpkg.DiagnosisRoomSupplementalEvidence{
		Label:    "Restart cause",
		Detail:   "Inspect previous container logs.",
		Priority: "urgent",
		Evidence: "previous pod logs show OOMKilled",
	}
	if _, err := activities.PersistDiagnosisTurn(ctx, req); !errors.Is(err, domain.ErrInvariantViolation) ||
		!strings.Contains(err.Error(), "supplemental evidence priority is unsupported") {
		t.Fatalf("PersistDiagnosisTurn error = %v, want unsupported supplemental priority invariant", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_FinalConclusionReadyIsAudited(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-final-ready")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 15, 45, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-final-ready",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}

	supplementalReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-final-ready", startedAt)
	supplementalReq.UserMessageID = "msg-supplemental"
	supplementalReq.AssistantMessageID = "msg-supplemental/assistant"
	supplementalReq.SupplementalEvidence = &temporalpkg.DiagnosisRoomSupplementalEvidence{
		Label:    "Restart cause",
		Detail:   "Inspect previous container logs.",
		Priority: "high",
		Evidence: "previous pod logs show OOMKilled",
	}
	supplemental, err := activities.PersistDiagnosisTurn(ctx, supplementalReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn supplemental: %v", err)
	}

	req := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-final-ready", startedAt)
	req.UserMessageID = "msg-final"
	req.AssistantMessageID = "msg-final/assistant"
	req.UserSequence = 3
	req.AssistantSequence = 4
	req.TurnCount = 2
	req.UserOccurredAt = startedAt.Add(2 * time.Minute)
	req.AssistantOccurredAt = startedAt.Add(2*time.Minute + 2*time.Second)
	req.UserMessage = "Use the restart logs and finalize the diagnosis."
	req.AssistantMessage = "CPU saturation has a final bounded diagnosis."
	req.RawOutput = json.RawMessage(`{
		"schema_version":"diagnosis_turn.v1",
		"message":"CPU saturation has a final bounded diagnosis.",
		"findings":["CPU saturation matches the deployment window"],
		"recommended_actions":["Scale the API deployment before peak traffic"],
		"evidence_requests":[{"tool":"active_alerts","reason":"Confirm sibling alerts are not firing.","limit":5}],
		"confidence":"high",
		"requires_human_review":true,
		"confidence_rationale":"The diagnosis is bounded but still needs owner confirmation.",
		"missing_evidence_requests":[{"label":"Owner sign-off","detail":"Confirm the rollout owner accepts the remediation.","priority":"medium"}],
		"evidence_collection_suggestions":[{"label":"Post-scale CPU trend","detail":"Collect a short CPU trend after scaling.","priority":"low"}],
		"conclusion_status":"final"
	}`)
	first, err := activities.PersistDiagnosisTurn(ctx, req)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn first: %v", err)
	}
	second, err := activities.PersistDiagnosisTurn(ctx, req)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn second: %v", err)
	}
	if first.FinalConclusion == nil ||
		second.FinalConclusion == nil ||
		first.FinalConclusion.Status != "available" ||
		first.FinalConclusion.Source != "latest_assistant_turn" ||
		first.FinalConclusion.Reason != "assistant_marked_final" ||
		first.FinalConclusion.AssistantTurnID != first.AssistantTurnID ||
		first.FinalConclusion.AssistantMessageID != req.AssistantMessageID ||
		first.FinalConclusion.AssistantSequence != req.AssistantSequence ||
		first.FinalConclusion.AssistantOccurredAt == nil ||
		!first.FinalConclusion.AssistantOccurredAt.Equal(domain.NormalizeUTCMicro(req.AssistantOccurredAt)) ||
		first.FinalConclusion.EvidenceSnapshotID != int64(seed.SnapshotID) ||
		first.FinalConclusion.ConclusionVersion != "diagnosis-room-final-ready.v1" ||
		first.FinalConclusion.RecordedAt == nil ||
		!first.FinalConclusion.RecordedAt.Equal(domain.NormalizeUTCMicro(req.AssistantOccurredAt)) ||
		first.FinalConclusion.ConfirmedBy != "" ||
		!reflect.DeepEqual(first.FinalConclusion.SupplementalContextRefs, []string{
			fmt.Sprintf("chat_session:%d/turn:%d", first.ChatSessionID, supplemental.UserTurnID),
			fmt.Sprintf("chat_session:%d/turn:%d", first.ChatSessionID, supplemental.AssistantTurnID),
			fmt.Sprintf("chat_session:%d/turn:%d", first.ChatSessionID, first.UserTurnID),
			fmt.Sprintf("chat_session:%d/turn:%d", first.ChatSessionID, first.AssistantTurnID),
		}) ||
		first.FinalConclusion.Content != req.AssistantMessage ||
		first.FinalConclusion.Confidence != "high" ||
		first.FinalConclusion.ConfidenceRationale != "The diagnosis is bounded but still needs owner confirmation." ||
		!reflect.DeepEqual(first.FinalConclusion.Findings, []string{"CPU saturation matches the deployment window"}) ||
		!reflect.DeepEqual(first.FinalConclusion.RecommendedActions, []string{"Scale the API deployment before peak traffic"}) ||
		len(first.FinalConclusion.EvidenceRequests) != 1 ||
		first.FinalConclusion.EvidenceRequests[0].Tool != "active_alerts" ||
		len(first.FinalConclusion.MissingEvidenceRequests) != 1 ||
		first.FinalConclusion.MissingEvidenceRequests[0].Label != "Owner sign-off" ||
		len(first.FinalConclusion.EvidenceCollectionSuggestions) != 1 ||
		first.FinalConclusion.EvidenceCollectionSuggestions[0].Label != "Post-scale CPU trend" ||
		first.FinalConclusion.RequiresHumanReview == nil ||
		!*first.FinalConclusion.RequiresHumanReview ||
		second.FinalConclusion.AssistantTurnID != first.FinalConclusion.AssistantTurnID {
		t.Fatalf("final conclusions first=%+v second=%+v", first.FinalConclusion, second.FinalConclusion)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 5 ||
			events[0].Kind != "diagnosis_room.opened" ||
			events[1].Kind != "diagnosis_room.supplemental_evidence_provided" ||
			events[2].Kind != "diagnosis_room.turn_persisted" ||
			events[3].Kind != "diagnosis_room.turn_persisted" ||
			events[4].Kind != "diagnosis_room.final_conclusion_ready" {
			t.Fatalf("events = %+v, want opened + supplemental + turn_persisted + turn_persisted + final_conclusion_ready", events)
		}
		var payload struct {
			TurnCount         int    `json:"turn_count"`
			ConclusionVersion string `json:"conclusion_version"`
			Conclusion        struct {
				Status              string     `json:"status"`
				Source              string     `json:"source"`
				Reason              string     `json:"reason"`
				EvidenceSnapshotID  int64      `json:"evidence_snapshot_id"`
				ConclusionVersion   string     `json:"conclusion_version"`
				RecordedAt          *time.Time `json:"recorded_at"`
				ConfirmedBy         string     `json:"confirmed_by"`
				SupplementalRefs    []string   `json:"supplemental_context_refs"`
				AssistantTurnID     int64      `json:"assistant_turn_id"`
				AssistantMessageID  string     `json:"assistant_message_id"`
				AssistantSequence   int        `json:"assistant_sequence"`
				AssistantOccurredAt *time.Time `json:"assistant_occurred_at"`
				Content             string     `json:"content"`
				Confidence          string     `json:"confidence"`
				ConfidenceRationale string     `json:"confidence_rationale"`
				Findings            []string   `json:"findings"`
				RecommendedActions  []string   `json:"recommended_actions"`
				EvidenceRequests    []struct {
					Tool string `json:"tool"`
				} `json:"evidence_requests"`
				MissingEvidenceRequests []struct {
					Label string `json:"label"`
				} `json:"missing_evidence_requests"`
				EvidenceCollectionSuggestions []struct {
					Label string `json:"label"`
				} `json:"evidence_collection_suggestions"`
				RequiresHumanReview *bool `json:"requires_human_review"`
			} `json:"final_conclusion"`
		}
		if err := json.Unmarshal(events[4].Payload, &payload); err != nil {
			t.Fatalf("final event payload: %v", err)
		}
		if payload.TurnCount != 2 ||
			payload.ConclusionVersion != "diagnosis-room-final-ready.v1" ||
			payload.Conclusion.Status != "available" ||
			payload.Conclusion.Source != "latest_assistant_turn" ||
			payload.Conclusion.Reason != "assistant_marked_final" ||
			payload.Conclusion.EvidenceSnapshotID != int64(seed.SnapshotID) ||
			payload.Conclusion.ConclusionVersion != "diagnosis-room-final-ready.v1" ||
			payload.Conclusion.RecordedAt == nil ||
			!payload.Conclusion.RecordedAt.Equal(domain.NormalizeUTCMicro(req.AssistantOccurredAt)) ||
			payload.Conclusion.ConfirmedBy != "" ||
			!reflect.DeepEqual(payload.Conclusion.SupplementalRefs, first.FinalConclusion.SupplementalContextRefs) ||
			payload.Conclusion.AssistantTurnID != first.AssistantTurnID ||
			payload.Conclusion.AssistantMessageID != req.AssistantMessageID ||
			payload.Conclusion.AssistantSequence != req.AssistantSequence ||
			payload.Conclusion.AssistantOccurredAt == nil ||
			!payload.Conclusion.AssistantOccurredAt.Equal(domain.NormalizeUTCMicro(req.AssistantOccurredAt)) ||
			payload.Conclusion.Content != req.AssistantMessage ||
			payload.Conclusion.Confidence != "high" ||
			payload.Conclusion.ConfidenceRationale != "The diagnosis is bounded but still needs owner confirmation." ||
			!reflect.DeepEqual(payload.Conclusion.Findings, []string{"CPU saturation matches the deployment window"}) ||
			!reflect.DeepEqual(payload.Conclusion.RecommendedActions, []string{"Scale the API deployment before peak traffic"}) ||
			len(payload.Conclusion.EvidenceRequests) != 1 ||
			payload.Conclusion.EvidenceRequests[0].Tool != "active_alerts" ||
			len(payload.Conclusion.MissingEvidenceRequests) != 1 ||
			payload.Conclusion.MissingEvidenceRequests[0].Label != "Owner sign-off" ||
			len(payload.Conclusion.EvidenceCollectionSuggestions) != 1 ||
			payload.Conclusion.EvidenceCollectionSuggestions[0].Label != "Post-scale CPU trend" ||
			payload.Conclusion.RequiresHumanReview == nil ||
			!*payload.Conclusion.RequiresHumanReview {
			t.Fatalf("final event payload = %+v raw=%s", payload, events[2].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify final-ready event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_ReadyForReviewConclusionIsAudited(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-ready-for-review")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 15, 50, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-ready-for-review",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}

	req := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-ready-for-review", startedAt)
	req.UserMessageID = "msg-ready"
	req.AssistantMessageID = "msg-ready/assistant"
	req.UserMessage = "Use the supplemental evidence and prepare the bounded conclusion."
	req.AssistantMessage = "The bounded diagnosis is ready for operator review."
	req.RawOutput = json.RawMessage(`{
		"schema_version":"diagnosis_turn.v1",
		"message":"The bounded diagnosis is ready for operator review.",
		"findings":["The latest restart evidence explains the alert window"],
		"recommended_actions":["Have the service owner confirm the remediation record"],
		"confidence":"medium",
		"requires_human_review":true,
		"confidence_rationale":"The causal chain is bounded, but the owner still needs to confirm the closeout.",
		"conclusion_status":"ready_for_review"
	}`)
	first, err := activities.PersistDiagnosisTurn(ctx, req)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn first: %v", err)
	}
	second, err := activities.PersistDiagnosisTurn(ctx, req)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn second: %v", err)
	}
	if first.FinalConclusion == nil ||
		second.FinalConclusion == nil ||
		first.FinalConclusion.Status != "available" ||
		first.FinalConclusion.Source != "latest_assistant_turn" ||
		first.FinalConclusion.Reason != "assistant_marked_ready_for_review" ||
		first.FinalConclusion.AssistantTurnID != first.AssistantTurnID ||
		first.FinalConclusion.AssistantMessageID != req.AssistantMessageID ||
		first.FinalConclusion.Content != req.AssistantMessage ||
		first.FinalConclusion.Confidence != "medium" ||
		first.FinalConclusion.ConfidenceRationale != "The causal chain is bounded, but the owner still needs to confirm the closeout." ||
		first.FinalConclusion.RequiresHumanReview == nil ||
		!*first.FinalConclusion.RequiresHumanReview ||
		second.FinalConclusion.AssistantTurnID != first.FinalConclusion.AssistantTurnID {
		t.Fatalf("ready final conclusions first=%+v second=%+v", first.FinalConclusion, second.FinalConclusion)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 3 ||
			events[0].Kind != "diagnosis_room.opened" ||
			events[1].Kind != "diagnosis_room.turn_persisted" ||
			events[2].Kind != "diagnosis_room.final_conclusion_ready" {
			t.Fatalf("events = %+v, want opened + turn_persisted + final_conclusion_ready", events)
		}
		var payload struct {
			TurnCount  int `json:"turn_count"`
			Conclusion struct {
				Status             string `json:"status"`
				Source             string `json:"source"`
				Reason             string `json:"reason"`
				AssistantMessageID string `json:"assistant_message_id"`
				Content            string `json:"content"`
				Confidence         string `json:"confidence"`
			} `json:"final_conclusion"`
		}
		if err := json.Unmarshal(events[2].Payload, &payload); err != nil {
			t.Fatalf("final event payload: %v", err)
		}
		if payload.TurnCount != 1 ||
			payload.Conclusion.Status != "available" ||
			payload.Conclusion.Source != "latest_assistant_turn" ||
			payload.Conclusion.Reason != "assistant_marked_ready_for_review" ||
			payload.Conclusion.AssistantMessageID != req.AssistantMessageID ||
			payload.Conclusion.Content != req.AssistantMessage ||
			payload.Conclusion.Confidence != "medium" {
			t.Fatalf("ready final event payload = %+v raw=%s", payload, events[2].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify ready final event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_CloseSessionIsIdempotentAndAudited(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-close-session")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 16, 0, 0, 0, time.UTC)
	ensure, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-close",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}

	req := temporalpkg.CloseDiagnosisChatSessionInput{
		SessionID:       "session-room-close",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		TurnCount:       0,
		ClosedAt:        startedAt.Add(5 * time.Minute),
		Reason:          "idle_timeout",
	}
	first, err := activities.CloseDiagnosisChatSession(ctx, req)
	if err != nil {
		t.Fatalf("CloseDiagnosisChatSession first: %v", err)
	}
	second, err := activities.CloseDiagnosisChatSession(ctx, req)
	if err != nil {
		t.Fatalf("CloseDiagnosisChatSession second: %v", err)
	}
	if first.ChatSessionID != ensure.ChatSessionID ||
		first.LifecycleEventID == 0 ||
		second.LifecycleEventID != first.LifecycleEventID ||
		first.Status != string(domain.ChatSessionStatusClosed) ||
		first.CloseReason != "idle_timeout" ||
		!first.ClosedAt.Equal(domain.NormalizeUTCMicro(req.ClosedAt)) ||
		first.FinalConclusion.Status != "not_available" ||
		first.FinalConclusion.Source != "none" ||
		first.FinalConclusion.Reason != "room_closed_without_assistant_turn" ||
		first.FinalConclusion.EvidenceSnapshotID != int64(seed.SnapshotID) ||
		first.FinalConclusion.ConclusionVersion != "diagnosis-room-close.v1" ||
		first.FinalConclusion.RecordedAt == nil ||
		!first.FinalConclusion.RecordedAt.Equal(domain.NormalizeUTCMicro(req.ClosedAt)) ||
		first.FinalConclusion.ConfirmedBy != "" ||
		!reflect.DeepEqual(second.FinalConclusion, first.FinalConclusion) {
		t.Fatalf("close results first=%+v second=%+v ensure=%+v", first, second, ensure)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if session.Status != domain.ChatSessionStatusClosed ||
			session.ClosedAt == nil ||
			!session.ClosedAt.Equal(domain.NormalizeUTCMicro(req.ClosedAt)) ||
			session.CloseReason != "idle_timeout" {
			t.Fatalf("closed session = %+v", session)
		}
		task, err := uow.Diagnosis().FindTaskByID(ctx, seed.TaskID)
		if err != nil {
			return err
		}
		if task.Status != domain.DiagnosisStatusCancelled ||
			task.FinishedAt == nil ||
			!task.FinishedAt.Equal(domain.NormalizeUTCMicro(req.ClosedAt)) ||
			task.FailureReason != "" {
			t.Fatalf("closed task = %+v", task)
		}
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 2 || events[0].Kind != "diagnosis_room.opened" || events[1].Kind != "diagnosis_room.closed" {
			t.Fatalf("events = %+v, want opened + closed", events)
		}
		var closePayload struct {
			CloseReason string `json:"close_reason"`
			TurnCount   int    `json:"turn_count"`
			Conclusion  struct {
				Status string `json:"status"`
				Source string `json:"source"`
				Reason string `json:"reason"`
			} `json:"final_conclusion"`
		}
		if err := json.Unmarshal(events[1].Payload, &closePayload); err != nil {
			t.Fatalf("close event payload: %v", err)
		}
		if closePayload.CloseReason != "idle_timeout" ||
			closePayload.TurnCount != 0 ||
			closePayload.Conclusion.Status != "not_available" ||
			closePayload.Conclusion.Source != "none" ||
			closePayload.Conclusion.Reason != "room_closed_without_assistant_turn" {
			t.Fatalf("close event payload = %+v raw=%s", closePayload, events[1].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify close: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_MultiStakeholderApprovalGatesClose(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-approval-quorum")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 7, 11, 9, 30, 0, 0, time.UTC)
	ensure, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-approval-quorum",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
		ApprovalMode:    domain.DiagnosisApprovalModeOwnerAndLeader,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	persistReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-approval-quorum", startedAt)
	persisted, err := activities.PersistDiagnosisTurn(ctx, persistReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn: %v", err)
	}
	digest, err := diagnosisapproval.ConclusionDigest(
		persistReq.AssistantMessageID,
		persistReq.AssistantSequence,
		persistReq.AssistantMessage,
	)
	if err != nil {
		t.Fatalf("ConclusionDigest: %v", err)
	}
	ownerReq := temporalpkg.RecordDiagnosisConclusionApprovalInput{
		SessionID:          "session-room-approval-quorum",
		DiagnosisTaskID:    int64(seed.TaskID),
		OwnerSubject:       "owner-1",
		ActorSubject:       "owner-1",
		Authority:          domain.DiagnosisApprovalAuthorityOwner,
		ApprovalMode:       domain.DiagnosisApprovalModeOwnerAndLeader,
		Reason:             "human_confirmed",
		AssistantMessageID: persistReq.AssistantMessageID,
		AssistantSequence:  persistReq.AssistantSequence,
		ConclusionContent:  persistReq.AssistantMessage,
		ConclusionDigest:   digest,
		ApprovedAt:         persisted.LastActivityAt.Add(time.Second),
	}
	ownerApproval, err := activities.RecordDiagnosisConclusionApproval(ctx, ownerReq)
	if err != nil {
		t.Fatalf("RecordDiagnosisConclusionApproval owner: %v", err)
	}
	closeReq := temporalpkg.CloseDiagnosisChatSessionInput{
		SessionID:                 "session-room-approval-quorum",
		DiagnosisTaskID:           int64(seed.TaskID),
		OwnerSubject:              "owner-1",
		ConfirmedBy:               "owner-1",
		TurnCount:                 1,
		ClosedAt:                  ownerReq.ApprovedAt.Add(time.Minute),
		Reason:                    "human_confirmed",
		RequireConclusionApproval: true,
		ApprovalMode:              domain.DiagnosisApprovalModeOwnerAndLeader,
		ConclusionDigest:          digest,
	}
	if _, err := activities.CloseDiagnosisChatSession(ctx, closeReq); err == nil || !strings.Contains(err.Error(), "quorum is incomplete") {
		t.Fatalf("CloseDiagnosisChatSession incomplete quorum error = %v", err)
	}

	leaderReq := ownerReq
	leaderReq.ActorSubject = "leader-1"
	leaderReq.Authority = domain.DiagnosisApprovalAuthorityLeader
	leaderReq.ApprovedAt = ownerReq.ApprovedAt.Add(time.Second)
	leaderApproval, err := activities.RecordDiagnosisConclusionApproval(ctx, leaderReq)
	if err != nil {
		t.Fatalf("RecordDiagnosisConclusionApproval leader: %v", err)
	}
	leaderApprovalRetry, err := activities.RecordDiagnosisConclusionApproval(ctx, leaderReq)
	if err != nil {
		t.Fatalf("RecordDiagnosisConclusionApproval leader retry: %v", err)
	}
	if ownerApproval.Approval.ID == 0 ||
		leaderApproval.Approval.ID == 0 ||
		leaderApprovalRetry.Approval.ID != leaderApproval.Approval.ID ||
		leaderApprovalRetry.LifecycleEventID != leaderApproval.LifecycleEventID {
		t.Fatalf("approval results owner=%+v leader=%+v retry=%+v", ownerApproval, leaderApproval, leaderApprovalRetry)
	}
	duplicateLeaderReq := leaderReq
	duplicateLeaderReq.ActorSubject = "leader-2"
	duplicateLeaderReq.ApprovedAt = leaderReq.ApprovedAt.Add(time.Second)
	if _, err := activities.RecordDiagnosisConclusionApproval(ctx, duplicateLeaderReq); err == nil ||
		!strings.Contains(err.Error(), "one authority cannot be satisfied by multiple subjects") {
		t.Fatalf("duplicate leader authority error = %v", err)
	}
	closeReq.ConfirmedBy = "leader-1"
	closeReq.ClosedAt = leaderReq.ApprovedAt.Add(time.Minute)
	closed, err := activities.CloseDiagnosisChatSession(ctx, closeReq)
	if err != nil {
		t.Fatalf("CloseDiagnosisChatSession complete quorum: %v", err)
	}
	if closed.ChatSessionID != ensure.ChatSessionID ||
		closed.ApprovalMode != domain.DiagnosisApprovalModeOwnerAndLeader ||
		closed.ConclusionDigest != digest ||
		len(closed.Approvals) != 2 {
		t.Fatalf("closed approval state = %+v", closed)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, closeReq.SessionID)
		if err != nil {
			return err
		}
		if session.Status != domain.ChatSessionStatusClosed || session.ApprovalMode != domain.DiagnosisApprovalModeOwnerAndLeader {
			t.Fatalf("stored session = %+v", session)
		}
		rows, err := uow.Diagnosis().ListChatSessionApprovals(ctx, session.ID, digest, 3)
		if err != nil {
			return err
		}
		if len(rows) != 2 || rows[0].ActorSubject != "owner-1" || rows[1].ActorSubject != "leader-1" {
			t.Fatalf("stored approvals = %+v", rows)
		}
		events, err := uow.Diagnosis().ListEventsByTaskAndKind(ctx, seed.TaskID, "diagnosis_room.conclusion_approved", 10)
		if err != nil {
			return err
		}
		if len(events) != 2 {
			t.Fatalf("approval events = %+v, want two idempotent events", events)
		}
		closedEvents, err := uow.Diagnosis().ListEventsByTaskAndKind(ctx, seed.TaskID, "diagnosis_room.closed", 1)
		if err != nil {
			return err
		}
		if len(closedEvents) != 1 {
			t.Fatalf("closed events = %+v", closedEvents)
		}
		var payload struct {
			ApprovalMode     domain.DiagnosisApprovalMode `json:"approval_mode"`
			ConclusionDigest string                       `json:"conclusion_digest"`
			Approvals        []json.RawMessage            `json:"approvals"`
		}
		if err := json.Unmarshal(closedEvents[0].Payload, &payload); err != nil {
			return err
		}
		if payload.ApprovalMode != domain.DiagnosisApprovalModeOwnerAndLeader ||
			payload.ConclusionDigest != digest ||
			len(payload.Approvals) != 2 {
			t.Fatalf("closed approval payload = %+v raw=%s", payload, closedEvents[0].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify approval quorum: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_CloseRejectsSupersededConclusionApprovals(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-superseded-approval")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 7, 11, 10, 30, 0, 0, time.UTC)
	if _, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-superseded-approval",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
		ApprovalMode:    domain.DiagnosisApprovalModeOwnerAndLeader,
	}); err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}

	firstTurn := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-superseded-approval", startedAt)
	if _, err := activities.PersistDiagnosisTurn(ctx, firstTurn); err != nil {
		t.Fatalf("PersistDiagnosisTurn first: %v", err)
	}
	oldDigest, err := diagnosisapproval.ConclusionDigest(
		firstTurn.AssistantMessageID,
		firstTurn.AssistantSequence,
		firstTurn.AssistantMessage,
	)
	if err != nil {
		t.Fatalf("ConclusionDigest first: %v", err)
	}
	approval := temporalpkg.RecordDiagnosisConclusionApprovalInput{
		SessionID:          firstTurn.SessionID,
		DiagnosisTaskID:    int64(seed.TaskID),
		OwnerSubject:       "owner-1",
		ActorSubject:       "owner-1",
		Authority:          domain.DiagnosisApprovalAuthorityOwner,
		ApprovalMode:       domain.DiagnosisApprovalModeOwnerAndLeader,
		Reason:             "human_confirmed",
		AssistantMessageID: firstTurn.AssistantMessageID,
		AssistantSequence:  firstTurn.AssistantSequence,
		ConclusionContent:  firstTurn.AssistantMessage,
		ConclusionDigest:   oldDigest,
		ApprovedAt:         firstTurn.AssistantOccurredAt.Add(time.Second),
	}
	if _, err := activities.RecordDiagnosisConclusionApproval(ctx, approval); err != nil {
		t.Fatalf("RecordDiagnosisConclusionApproval owner: %v", err)
	}
	approval.ActorSubject = "leader-1"
	approval.Authority = domain.DiagnosisApprovalAuthorityLeader
	approval.ApprovedAt = approval.ApprovedAt.Add(time.Second)
	if _, err := activities.RecordDiagnosisConclusionApproval(ctx, approval); err != nil {
		t.Fatalf("RecordDiagnosisConclusionApproval leader: %v", err)
	}

	secondTurn := validPersistDiagnosisTurnInput(seed.TaskID, firstTurn.SessionID, startedAt)
	secondTurn.UserMessageID = "msg-2"
	secondTurn.AssistantMessageID = "msg-2/assistant"
	secondTurn.UserSequence = 3
	secondTurn.AssistantSequence = 4
	secondTurn.TurnCount = 2
	secondTurn.UserMessage = "Reassess with the latest deployment evidence."
	secondTurn.AssistantMessage = "The revised conclusion is deployment saturation."
	secondTurn.UserOccurredAt = startedAt.Add(3 * time.Minute)
	secondTurn.AssistantOccurredAt = startedAt.Add(3*time.Minute + 2*time.Second)
	secondTurn.ContainerStartedAt = secondTurn.UserOccurredAt
	secondTurn.ContainerFinishedAt = secondTurn.UserOccurredAt.Add(time.Second)
	secondTurn.InvocationID = "diagnosis-room/task-1/msg-revised"
	secondTurn.RawOutput = json.RawMessage(strings.Replace(
		string(secondTurn.RawOutput),
		"CPU saturation is concentrated on api-1.",
		secondTurn.AssistantMessage,
		1,
	))
	if _, err := activities.PersistDiagnosisTurn(ctx, secondTurn); err != nil {
		t.Fatalf("PersistDiagnosisTurn second: %v", err)
	}

	_, err = activities.CloseDiagnosisChatSession(ctx, temporalpkg.CloseDiagnosisChatSessionInput{
		SessionID:                 firstTurn.SessionID,
		DiagnosisTaskID:           int64(seed.TaskID),
		OwnerSubject:              "owner-1",
		ConfirmedBy:               "leader-1",
		TurnCount:                 2,
		ClosedAt:                  secondTurn.AssistantOccurredAt.Add(time.Minute),
		Reason:                    "human_confirmed",
		RequireConclusionApproval: true,
		ApprovalMode:              domain.DiagnosisApprovalModeOwnerAndLeader,
		ConclusionDigest:          oldDigest,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match the latest assistant conclusion") {
		t.Fatalf("CloseDiagnosisChatSession superseded approval error = %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_CloseSessionCanFailTask(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-close-failed-task")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 16, 15, 0, 0, time.UTC)
	if _, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-close-failed",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	}); err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}

	req := temporalpkg.CloseDiagnosisChatSessionInput{
		SessionID:                  "session-room-close-failed",
		DiagnosisTaskID:            int64(seed.TaskID),
		OwnerSubject:               "owner-1",
		TurnCount:                  0,
		ClosedAt:                   startedAt.Add(2 * time.Minute),
		Reason:                     "initial_turn_failed",
		DiagnosisTaskStatus:        "failed",
		DiagnosisTaskFailureReason: "Diagnosis turn failed before an assistant response; upstream LLM request timed out.",
	}
	first, err := activities.CloseDiagnosisChatSession(ctx, req)
	if err != nil {
		t.Fatalf("CloseDiagnosisChatSession first: %v", err)
	}
	second, err := activities.CloseDiagnosisChatSession(ctx, req)
	if err != nil {
		t.Fatalf("CloseDiagnosisChatSession second: %v", err)
	}
	if first.LifecycleEventID == 0 || second.LifecycleEventID != first.LifecycleEventID {
		t.Fatalf("close lifecycle ids first=%d second=%d", first.LifecycleEventID, second.LifecycleEventID)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		task, err := uow.Diagnosis().FindTaskByID(ctx, seed.TaskID)
		if err != nil {
			return err
		}
		if task.Status != domain.DiagnosisStatusFailed ||
			task.FailureReason != req.DiagnosisTaskFailureReason ||
			task.FinishedAt == nil ||
			!task.FinishedAt.Equal(domain.NormalizeUTCMicro(req.ClosedAt)) {
			t.Fatalf("failed task = %+v", task)
		}
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		if len(events) != 3 ||
			events[0].Kind != "diagnosis_room.opened" ||
			events[1].Kind != "diagnosis_room.failed" ||
			events[2].Kind != "diagnosis_room.closed" {
			t.Fatalf("events = %+v, want opened + failed + closed", events)
		}
		var failedPayload struct {
			Status        string `json:"status"`
			FailureReason string `json:"failure_reason"`
			CloseReason   string `json:"close_reason"`
		}
		if err := json.Unmarshal(events[1].Payload, &failedPayload); err != nil {
			t.Fatalf("failed event payload: %v", err)
		}
		if failedPayload.Status != "failed" ||
			failedPayload.FailureReason != req.DiagnosisTaskFailureReason ||
			failedPayload.CloseReason != "initial_turn_failed" {
			t.Fatalf("failed event payload = %+v raw=%s", failedPayload, events[1].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify failed close: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_CloseEventCapturesFinalConclusion(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-close-conclusion")
	activities := temporalpkg.NewActivities(env.factory)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 16, 30, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-close-conclusion",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	turnReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-close-conclusion", startedAt)
	persisted, err := activities.PersistDiagnosisTurn(ctx, turnReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn: %v", err)
	}

	closeReq := temporalpkg.CloseDiagnosisChatSessionInput{
		SessionID:                   "session-room-close-conclusion",
		DiagnosisTaskID:             int64(seed.TaskID),
		OwnerSubject:                "owner-1",
		ConfirmedBy:                 "reviewer-1",
		TurnCount:                   1,
		ClosedAt:                    startedAt.Add(5 * time.Minute),
		Reason:                      "human_confirmed",
		GenerateConversationSummary: true,
	}
	first, err := activities.CloseDiagnosisChatSession(ctx, closeReq)
	if err != nil {
		t.Fatalf("CloseDiagnosisChatSession first: %v", err)
	}
	second, err := activities.CloseDiagnosisChatSession(ctx, closeReq)
	if err != nil {
		t.Fatalf("CloseDiagnosisChatSession second: %v", err)
	}
	if second.LifecycleEventID != first.LifecycleEventID {
		t.Fatalf("close event ID second=%d, want %d", second.LifecycleEventID, first.LifecycleEventID)
	}
	if first.ConversationSummary == nil ||
		first.ConversationSummary.ID == 0 ||
		first.ConversationSummary.Version != 1 ||
		first.ConversationSummary.SchemaVersion != "diagnosis-conversation-summary.v1" ||
		first.ConversationSummary.SourceFirstSequence != 1 ||
		first.ConversationSummary.SourceLastSequence != 2 ||
		first.ConversationSummary.SourceTurnCount != 2 ||
		len(first.ConversationSummary.SourceDigest) != 64 ||
		second.ConversationSummary == nil ||
		second.ConversationSummary.ID != first.ConversationSummary.ID ||
		second.ConversationSummary.SourceDigest != first.ConversationSummary.SourceDigest {
		t.Fatalf("conversation summaries first=%+v second=%+v", first.ConversationSummary, second.ConversationSummary)
	}
	var summaryContent struct {
		SchemaVersion           string `json:"schema_version"`
		CompressionMethod       string `json:"compression_method"`
		SourceTurnCount         int    `json:"source_turn_count"`
		OpeningRequest          string `json:"opening_request"`
		LatestRequest           string `json:"latest_request"`
		LatestAssistantResponse string `json:"latest_assistant_response"`
	}
	if err := json.Unmarshal(first.ConversationSummary.Content, &summaryContent); err != nil {
		t.Fatalf("conversation summary content: %v", err)
	}
	if summaryContent.SchemaVersion != "diagnosis-conversation-summary.v1" ||
		summaryContent.CompressionMethod != "deterministic-extractive" ||
		summaryContent.SourceTurnCount != 2 ||
		summaryContent.OpeningRequest != turnReq.UserMessage ||
		summaryContent.LatestRequest != turnReq.UserMessage ||
		summaryContent.LatestAssistantResponse != turnReq.AssistantMessage {
		t.Fatalf("conversation summary content = %+v", summaryContent)
	}
	if first.FinalConclusion.Status != "available" ||
		first.FinalConclusion.Source != "latest_assistant_turn" ||
		first.FinalConclusion.AssistantTurnID != persisted.AssistantTurnID ||
		first.FinalConclusion.AssistantMessageID != turnReq.AssistantMessageID ||
		first.FinalConclusion.AssistantSequence != turnReq.AssistantSequence ||
		first.FinalConclusion.AssistantOccurredAt == nil ||
		!first.FinalConclusion.AssistantOccurredAt.Equal(domain.NormalizeUTCMicro(turnReq.AssistantOccurredAt)) ||
		first.FinalConclusion.EvidenceSnapshotID != int64(seed.SnapshotID) ||
		first.FinalConclusion.ConclusionVersion != "diagnosis-room-close.v1" ||
		first.FinalConclusion.RecordedAt == nil ||
		!first.FinalConclusion.RecordedAt.Equal(domain.NormalizeUTCMicro(closeReq.ClosedAt)) ||
		first.FinalConclusion.ConfirmedBy != closeReq.ConfirmedBy ||
		!reflect.DeepEqual(first.FinalConclusion.SupplementalContextRefs, []string{
			fmt.Sprintf("chat_session:%d/turn:%d", first.ChatSessionID, persisted.UserTurnID),
			fmt.Sprintf("chat_session:%d/turn:%d", first.ChatSessionID, persisted.AssistantTurnID),
		}) ||
		first.FinalConclusion.Content != turnReq.AssistantMessage ||
		first.FinalConclusion.Confidence != "high" ||
		first.FinalConclusion.ConfidenceRationale != "Confidence depends on sibling alert and restart evidence." ||
		!reflect.DeepEqual(first.FinalConclusion.Findings, []string{"api-1 CPU exceeded threshold"}) ||
		!reflect.DeepEqual(first.FinalConclusion.RecommendedActions, []string{"Inspect recent deployment"}) ||
		len(first.FinalConclusion.EvidenceRequests) != 1 ||
		first.FinalConclusion.EvidenceRequests[0].Reason != "Need current active sibling alerts." ||
		len(first.FinalConclusion.MissingEvidenceRequests) != 1 ||
		first.FinalConclusion.MissingEvidenceRequests[0].Label != "Restart cause" ||
		len(first.FinalConclusion.EvidenceCollectionSuggestions) != 1 ||
		first.FinalConclusion.EvidenceCollectionSuggestions[0].Label != "CPU trend" ||
		first.FinalConclusion.RequiresHumanReview == nil ||
		!*first.FinalConclusion.RequiresHumanReview {
		t.Fatalf("close result final conclusion = %+v", first.FinalConclusion)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		task, err := uow.Diagnosis().FindTaskByID(ctx, seed.TaskID)
		if err != nil {
			return err
		}
		if task.Status != domain.DiagnosisStatusSucceeded ||
			task.FinishedAt == nil ||
			!task.FinishedAt.Equal(domain.NormalizeUTCMicro(closeReq.ClosedAt)) ||
			task.FailureReason != "" {
			t.Fatalf("closed task = %+v", task)
		}
		persistedSummary, err := uow.Diagnosis().FindLatestChatSessionSummary(ctx, domain.ChatSessionID(first.ChatSessionID))
		if err != nil {
			return err
		}
		if int64(persistedSummary.ID) != first.ConversationSummary.ID ||
			persistedSummary.SourceDigest != first.ConversationSummary.SourceDigest {
			t.Fatalf("persisted summary = %+v, result = %+v", persistedSummary, first.ConversationSummary)
		}
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		var closeEvent domain.DiagnosisTaskEvent
		for _, event := range events {
			if event.Kind == "diagnosis_room.closed" {
				closeEvent = event
				break
			}
		}
		if closeEvent.ID == 0 {
			t.Fatalf("events = %+v, want diagnosis_room.closed", events)
		}
		var payload struct {
			CloseReason       string `json:"close_reason"`
			TurnCount         int    `json:"turn_count"`
			ConclusionVersion string `json:"conclusion_version"`
			Conclusion        struct {
				Status              string     `json:"status"`
				Source              string     `json:"source"`
				EvidenceSnapshotID  int64      `json:"evidence_snapshot_id"`
				ConclusionVersion   string     `json:"conclusion_version"`
				RecordedAt          *time.Time `json:"recorded_at"`
				ConfirmedBy         string     `json:"confirmed_by"`
				SupplementalRefs    []string   `json:"supplemental_context_refs"`
				AssistantTurnID     int64      `json:"assistant_turn_id"`
				AssistantMessageID  string     `json:"assistant_message_id"`
				AssistantSequence   int        `json:"assistant_sequence"`
				AssistantOccurredAt *time.Time `json:"assistant_occurred_at"`
				Content             string     `json:"content"`
				Confidence          string     `json:"confidence"`
				ConfidenceRationale string     `json:"confidence_rationale"`
				Findings            []string   `json:"findings"`
				RecommendedActions  []string   `json:"recommended_actions"`
				EvidenceRequests    []struct {
					Reason string `json:"reason"`
				} `json:"evidence_requests"`
				MissingEvidenceRequests []struct {
					Label string `json:"label"`
				} `json:"missing_evidence_requests"`
				EvidenceCollectionSuggestions []struct {
					Label string `json:"label"`
				} `json:"evidence_collection_suggestions"`
				RequiresHumanReview *bool `json:"requires_human_review"`
			} `json:"final_conclusion"`
			ConversationSummary struct {
				ID              int64  `json:"id"`
				Version         int    `json:"version"`
				SchemaVersion   string `json:"schema_version"`
				SourceTurnCount int    `json:"source_turn_count"`
				SourceDigest    string `json:"source_digest"`
			} `json:"conversation_summary"`
		}
		if err := json.Unmarshal(closeEvent.Payload, &payload); err != nil {
			t.Fatalf("close event payload: %v", err)
		}
		if payload.CloseReason != "human_confirmed" ||
			payload.TurnCount != 1 ||
			payload.ConclusionVersion != "diagnosis-room-close.v1" ||
			payload.Conclusion.Status != "available" ||
			payload.Conclusion.Source != "latest_assistant_turn" ||
			payload.Conclusion.EvidenceSnapshotID != int64(seed.SnapshotID) ||
			payload.Conclusion.ConclusionVersion != "diagnosis-room-close.v1" ||
			payload.Conclusion.RecordedAt == nil ||
			!payload.Conclusion.RecordedAt.Equal(domain.NormalizeUTCMicro(closeReq.ClosedAt)) ||
			payload.Conclusion.ConfirmedBy != closeReq.ConfirmedBy ||
			!reflect.DeepEqual(payload.Conclusion.SupplementalRefs, first.FinalConclusion.SupplementalContextRefs) ||
			payload.Conclusion.AssistantTurnID != persisted.AssistantTurnID ||
			payload.Conclusion.AssistantMessageID != turnReq.AssistantMessageID ||
			payload.Conclusion.AssistantSequence != turnReq.AssistantSequence ||
			payload.Conclusion.AssistantOccurredAt == nil ||
			!payload.Conclusion.AssistantOccurredAt.Equal(domain.NormalizeUTCMicro(turnReq.AssistantOccurredAt)) ||
			payload.Conclusion.Content != turnReq.AssistantMessage ||
			payload.Conclusion.Confidence != "high" ||
			payload.Conclusion.ConfidenceRationale != "Confidence depends on sibling alert and restart evidence." ||
			!reflect.DeepEqual(payload.Conclusion.Findings, []string{"api-1 CPU exceeded threshold"}) ||
			!reflect.DeepEqual(payload.Conclusion.RecommendedActions, []string{"Inspect recent deployment"}) ||
			len(payload.Conclusion.EvidenceRequests) != 1 ||
			payload.Conclusion.EvidenceRequests[0].Reason != "Need current active sibling alerts." ||
			len(payload.Conclusion.MissingEvidenceRequests) != 1 ||
			payload.Conclusion.MissingEvidenceRequests[0].Label != "Restart cause" ||
			len(payload.Conclusion.EvidenceCollectionSuggestions) != 1 ||
			payload.Conclusion.EvidenceCollectionSuggestions[0].Label != "CPU trend" ||
			payload.Conclusion.RequiresHumanReview == nil ||
			!*payload.Conclusion.RequiresHumanReview ||
			payload.ConversationSummary.ID != first.ConversationSummary.ID ||
			payload.ConversationSummary.Version != 1 ||
			payload.ConversationSummary.SchemaVersion != "diagnosis-conversation-summary.v1" ||
			payload.ConversationSummary.SourceTurnCount != 2 ||
			payload.ConversationSummary.SourceDigest != first.ConversationSummary.SourceDigest {
			t.Fatalf("close event payload = %+v raw=%s", payload, closeEvent.Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify close conclusion: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_SendCloseNotificationIsIdempotentAndAudited(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-close-notification")
	im := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "msg-close",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"accepted":true}`),
	}}
	activities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithIMProvider(im),
		temporalpkg.WithPublicBaseURL(mustPublicBaseURL(t)),
	)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 17, 0, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-close-notify",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	req := temporalpkg.CloseDiagnosisChatSessionInput{
		SessionID:       "session-room-close-notify",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		TurnCount:       0,
		ClosedAt:        startedAt.Add(5 * time.Minute),
		Reason:          "user_done",
	}
	if _, err := activities.CloseDiagnosisChatSession(ctx, req); err != nil {
		t.Fatalf("CloseDiagnosisChatSession: %v", err)
	}

	first, err := activities.SendDiagnosisRoomCloseNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomCloseNotification first: %v", err)
	}
	second, err := activities.SendDiagnosisRoomCloseNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomCloseNotification second: %v", err)
	}
	if first.LifecycleEventID == 0 ||
		second.LifecycleEventID != first.LifecycleEventID ||
		first.ProviderMessageID != "msg-close" ||
		first.NotificationStatus != "delivered" ||
		first.IdempotencyKey == "" {
		t.Fatalf("notification results first=%+v second=%+v", first, second)
	}
	requests := im.Requests()
	if len(requests) != 1 {
		t.Fatalf("notification requests len = %d, want 1", len(requests))
	}
	if requests[0].FinalReportID != 0 ||
		requests[0].DiagnosisTaskID != int64(seed.TaskID) ||
		requests[0].CorrelationKey == "" ||
		requests[0].IdempotencyKey != first.IdempotencyKey ||
		requests[0].Title == "" ||
		!strings.Contains(requests[0].Body, "Review room: https://openclarion.example.test/ops/diagnosis-room?auth_mode=session&session_id=session-room-close-notify&wecom_auto_login=1&wecom_launch_context=app_conversation") ||
		!strings.Contains(requests[0].Body, "AI conclusion: unavailable") {
		t.Fatalf("notification request = %+v", requests[0])
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 10)
		if err != nil {
			return err
		}
		var found bool
		for _, event := range events {
			if event.Kind != "diagnosis_room.close_notification_sent" {
				continue
			}
			found = true
			if event.ID != domain.DiagnosisTaskEventID(first.LifecycleEventID) {
				t.Fatalf("notification event ID = %d, want %d", event.ID, first.LifecycleEventID)
			}
			var payload struct {
				IdempotencyKey    string `json:"idempotency_key"`
				RoomURL           string `json:"room_url"`
				ProviderMessageID string `json:"provider_message_id"`
				ProviderStatus    string `json:"provider_status"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("notification event payload: %v", err)
			}
			if payload.IdempotencyKey != first.IdempotencyKey ||
				payload.RoomURL != "https://openclarion.example.test/ops/diagnosis-room?auth_mode=session&session_id=session-room-close-notify&wecom_auto_login=1&wecom_launch_context=app_conversation" ||
				payload.ProviderMessageID != "msg-close" ||
				payload.ProviderStatus != "delivered" {
				t.Fatalf("notification event payload = %+v raw=%s", payload, event.Payload)
			}
		}
		if !found {
			t.Fatalf("events = %+v, want close notification event", events)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify notification event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_SendCloseNotificationUsesProfileResolver(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-close-notification-profile")
	im := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "msg-profile-close",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"accepted":true}`),
	}}
	resolver := &recordingNotificationProviderResolver{provider: im}
	activities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithNotificationChannelProviderResolver(resolver),
		temporalpkg.WithPublicBaseURL(mustPublicBaseURL(t)),
	)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 17, 30, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-close-notify-profile",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	req := temporalpkg.CloseDiagnosisChatSessionInput{
		SessionID:                         "session-room-close-notify-profile",
		DiagnosisTaskID:                   int64(seed.TaskID),
		OwnerSubject:                      "owner-1",
		TurnCount:                         0,
		ClosedAt:                          startedAt.Add(5 * time.Minute),
		Reason:                            "user_done",
		CloseNotificationChannelProfileID: 5,
	}
	if _, err := activities.CloseDiagnosisChatSession(ctx, req); err != nil {
		t.Fatalf("CloseDiagnosisChatSession: %v", err)
	}

	result, err := activities.SendDiagnosisRoomCloseNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomCloseNotification: %v", err)
	}
	calls, profileID := resolver.LastCall()
	if calls != 1 || profileID != 5 {
		t.Fatalf("resolver calls/profile = %d/%d, want 1/5", calls, profileID)
	}
	if scope := resolver.LastScope(); scope != domain.NotificationDeliveryScopeDiagnosisClose {
		t.Fatalf("resolver scope = %s, want %s", scope, domain.NotificationDeliveryScopeDiagnosisClose)
	}
	requests := im.Requests()
	if len(requests) != 1 ||
		requests[0].NotificationChannelID != 5 ||
		requests[0].DiagnosisTaskID != int64(seed.TaskID) ||
		requests[0].IdempotencyKey != result.IdempotencyKey {
		t.Fatalf("notification requests = %+v result=%+v", requests, result)
	}
}

func TestDiagnosisRoomPersistenceActivities_SendFinalReadyNotificationIsIdempotentAndAudited(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-final-ready-notification")
	im := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "msg-final-ready",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"accepted":true}`),
	}}
	resolver := &recordingNotificationProviderResolver{provider: im}
	activities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithNotificationChannelProviderResolver(resolver),
		temporalpkg.WithPublicBaseURL(mustPublicBaseURL(t)),
	)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 17, 45, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-final-ready-notify",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	turnReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-final-ready-notify", startedAt)
	turnReq.AssistantMessage = "CPU saturation is explained by the latest rollout and requires scaling before traffic peak."
	turnReq.RawOutput = json.RawMessage(`{
		"schema_version":"diagnosis_turn.v1",
		"message":"CPU saturation is explained by the latest rollout and requires scaling before traffic peak.",
		"findings":["api-1 CPU exceeded threshold"],
		"recommended_actions":["Scale the affected deployment before traffic peak"],
		"evidence_requests":[{"tool":"active_alerts","reason":"Confirm sibling alerts remain firing.","limit":5}],
		"confidence":"high",
		"requires_human_review":true,
		"confidence_rationale":"Deployment timing and CPU evidence are aligned.",
		"missing_evidence_requests":[{"label":"Restart cause","detail":"Inspect previous container logs before final remediation.","priority":"high"}],
		"evidence_collection_suggestions":[{"label":"CPU trend","detail":"Collect a bounded CPU range query for the affected deployment.","priority":"medium"}],
		"conclusion_status":"final"
	}`)
	persisted, err := activities.PersistDiagnosisTurn(ctx, turnReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn: %v", err)
	}
	if persisted.FinalConclusion == nil {
		t.Fatal("PersistDiagnosisTurn final conclusion = nil, want available conclusion")
	}
	req := temporalpkg.SendDiagnosisRoomFinalReadyNotificationInput{
		SessionID:                         "session-room-final-ready-notify",
		DiagnosisTaskID:                   int64(seed.TaskID),
		OwnerSubject:                      "owner-1",
		AssistantTurnID:                   persisted.AssistantTurnID,
		AssistantMessageID:                persisted.AssistantMessageID,
		AssistantSequence:                 persisted.AssistantSequence,
		TurnCount:                         persisted.TurnCount,
		OccurredAt:                        persisted.AssistantOccurredAt,
		CloseNotificationChannelProfileID: 5,
		FinalConclusion:                   *persisted.FinalConclusion,
	}

	first, err := activities.SendDiagnosisRoomFinalReadyNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomFinalReadyNotification first: %v", err)
	}
	second, err := activities.SendDiagnosisRoomFinalReadyNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomFinalReadyNotification second: %v", err)
	}
	if first.LifecycleEventID == 0 ||
		second.LifecycleEventID != first.LifecycleEventID ||
		first.ProviderMessageID != "msg-final-ready" ||
		first.NotificationStatus != "delivered" ||
		first.IdempotencyKey == "" {
		t.Fatalf("notification results first=%+v second=%+v", first, second)
	}
	calls, profileID := resolver.LastCall()
	if calls != 1 || profileID != 5 {
		t.Fatalf("resolver calls/profile = %d/%d, want 1/5", calls, profileID)
	}
	if scope := resolver.LastScope(); scope != domain.NotificationDeliveryScopeDiagnosisConsultation {
		t.Fatalf("resolver scope = %s, want %s", scope, domain.NotificationDeliveryScopeDiagnosisConsultation)
	}
	requests := im.Requests()
	if len(requests) != 1 {
		t.Fatalf("notification requests len = %d, want 1", len(requests))
	}
	body := requests[0].Body
	if requests[0].NotificationChannelID != 5 ||
		requests[0].DiagnosisTaskID != int64(seed.TaskID) ||
		requests[0].IdempotencyKey != first.IdempotencyKey ||
		requests[0].Title != "AI diagnosis ready: session-room-final-ready-notify" ||
		requests[0].Severity != "warning" ||
		!strings.Contains(body, "AI diagnosis is ready for room session-room-final-ready-notify") ||
		!strings.Contains(body, "Review room: https://openclarion.example.test/ops/diagnosis-room?auth_mode=session&session_id=session-room-final-ready-notify&wecom_auto_login=1&wecom_launch_context=app_conversation") ||
		!strings.Contains(body, "Review the conclusion, provide missing evidence if needed") ||
		!strings.Contains(body, "Confidence: high") ||
		!strings.Contains(body, "Human review: required") ||
		!strings.Contains(body, "AI conclusion: CPU saturation is explained by the latest rollout") ||
		!strings.Contains(body, "Missing evidence:") ||
		!strings.Contains(body, "[high] Restart cause - Inspect previous container logs before final remediation.") ||
		!strings.Contains(body, "Evidence collection suggestions:") ||
		!strings.Contains(body, "Next action: collect 1 executable evidence request(s) and provide 1 operator-supplied evidence item(s).") ||
		!strings.Contains(body, "Executable evidence requests: 1") ||
		!strings.Contains(body, "1. active_alerts - Confirm sibling alerts remain firing. (limit=5)") {
		t.Fatalf("notification request = %+v", requests[0])
	}
	assertSubstringsInOrder(t, body, "Next action:", "AI conclusion:")
	assertSubstringsInOrder(t, body, "Missing evidence:", "AI conclusion:")
	assertSubstringsInOrder(t, body, "Evidence collection suggestions:", "AI conclusion:")
	assertSubstringsInOrder(t, body, "Executable evidence requests: 1", "AI conclusion:")

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 20)
		if err != nil {
			return err
		}
		var event domain.DiagnosisTaskEvent
		for _, candidate := range events {
			if candidate.ID == domain.DiagnosisTaskEventID(first.LifecycleEventID) {
				event = candidate
				break
			}
		}
		if event.ID == 0 || event.Kind != "diagnosis_room.final_ready_notification_sent" {
			t.Fatalf("events = %+v, want final-ready notification event %d", events, first.LifecycleEventID)
		}
		var payload struct {
			IdempotencyKey string                                   `json:"idempotency_key"`
			ChannelProfile int64                                    `json:"notification_channel_profile_id"`
			RoomURL        string                                   `json:"room_url"`
			Conclusion     temporalpkg.DiagnosisRoomFinalConclusion `json:"final_conclusion"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("notification event payload: %v", err)
		}
		if payload.IdempotencyKey != first.IdempotencyKey ||
			payload.ChannelProfile != 5 ||
			payload.RoomURL != "https://openclarion.example.test/ops/diagnosis-room?auth_mode=session&session_id=session-room-final-ready-notify&wecom_auto_login=1&wecom_launch_context=app_conversation" ||
			payload.Conclusion.Status != "available" ||
			payload.Conclusion.Content != turnReq.AssistantMessage ||
			payload.Conclusion.Confidence != "high" {
			t.Fatalf("notification event payload = %+v raw=%s", payload, event.Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify notification event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_SendAssistantTurnNotificationIsIdempotentAndAudited(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-assistant-turn-notification")
	im := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "msg-assistant-turn",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"accepted":true}`),
	}}
	resolver := &recordingNotificationProviderResolver{provider: im}
	activities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithNotificationChannelProviderResolver(resolver),
		temporalpkg.WithPublicBaseURL(mustPublicBaseURL(t)),
	)
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 17, 50, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-assistant-turn-notify",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	turnReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-assistant-turn-notify", startedAt)
	persisted, err := activities.PersistDiagnosisTurn(ctx, turnReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn: %v", err)
	}
	if persisted.FinalConclusion != nil {
		t.Fatalf("PersistDiagnosisTurn final conclusion = %+v, want nil for needs-evidence turn", persisted.FinalConclusion)
	}
	req := temporalpkg.SendDiagnosisRoomAssistantTurnNotificationInput{
		SessionID:                         "session-room-assistant-turn-notify",
		DiagnosisTaskID:                   int64(seed.TaskID),
		OwnerSubject:                      "owner-1",
		AssistantTurnID:                   persisted.AssistantTurnID,
		AssistantMessageID:                persisted.AssistantMessageID,
		AssistantSequence:                 persisted.AssistantSequence,
		TurnCount:                         persisted.TurnCount,
		OccurredAt:                        persisted.AssistantOccurredAt,
		CloseNotificationChannelProfileID: 5,
		AssistantMessage:                  persisted.AssistantMessage,
		Confidence:                        persisted.Confidence,
		RequiresHumanReview:               persisted.RequiresHumanReview,
		Findings:                          persisted.Findings,
		RecommendedActions:                persisted.RecommendedActions,
		EvidenceRequests:                  persisted.EvidenceRequests,
		Insight:                           persisted.Insight,
	}

	first, err := activities.SendDiagnosisRoomAssistantTurnNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomAssistantTurnNotification first: %v", err)
	}
	second, err := activities.SendDiagnosisRoomAssistantTurnNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomAssistantTurnNotification second: %v", err)
	}
	if first.LifecycleEventID == 0 ||
		second.LifecycleEventID != first.LifecycleEventID ||
		first.ProviderMessageID != "msg-assistant-turn" ||
		first.NotificationStatus != "delivered" ||
		first.IdempotencyKey == "" {
		t.Fatalf("notification results first=%+v second=%+v", first, second)
	}
	calls, profileID := resolver.LastCall()
	if calls != 1 || profileID != 5 {
		t.Fatalf("resolver calls/profile = %d/%d, want 1/5", calls, profileID)
	}
	if scope := resolver.LastScope(); scope != domain.NotificationDeliveryScopeDiagnosisConsultation {
		t.Fatalf("resolver scope = %s, want %s", scope, domain.NotificationDeliveryScopeDiagnosisConsultation)
	}
	requests := im.Requests()
	if len(requests) != 1 {
		t.Fatalf("notification requests len = %d, want 1", len(requests))
	}
	body := requests[0].Body
	if requests[0].NotificationChannelID != 5 ||
		requests[0].DiagnosisTaskID != int64(seed.TaskID) ||
		requests[0].IdempotencyKey != first.IdempotencyKey ||
		requests[0].Title != "Initial AI diagnosis report: session-room-assistant-turn-notify" ||
		requests[0].Severity != "warning" ||
		!strings.Contains(body, "Initial AI diagnosis report is ready for room session-room-assistant-turn-notify") ||
		!strings.Contains(body, "Review room: https://openclarion.example.test/ops/diagnosis-room?auth_mode=session&session_id=session-room-assistant-turn-notify&wecom_auto_login=1&wecom_launch_context=app_conversation") ||
		!strings.Contains(body, "collect missing evidence") ||
		!strings.Contains(body, "Confidence: high") ||
		!strings.Contains(body, "Human review: required") ||
		!strings.Contains(body, "Conclusion status: needs_evidence") ||
		!strings.Contains(body, "AI diagnosis: CPU saturation is concentrated on api-1.") ||
		!strings.Contains(body, "Confidence rationale: Confidence depends on sibling alert and restart evidence.") ||
		!strings.Contains(body, "Findings:") ||
		!strings.Contains(body, "api-1 CPU exceeded threshold") ||
		!strings.Contains(body, "Recommended actions:") ||
		!strings.Contains(body, "Inspect recent deployment") ||
		!strings.Contains(body, "Missing evidence:") ||
		!strings.Contains(body, "[high] Restart cause - Inspect previous container logs.") ||
		!strings.Contains(body, "Evidence collection suggestions:") ||
		!strings.Contains(body, "Next action: collect 1 executable evidence request(s) and provide 1 operator-supplied evidence item(s).") ||
		!strings.Contains(body, "Executable evidence requests: 1") ||
		!strings.Contains(body, "1. active_alerts - Need current active sibling alerts. (limit=5)") {
		t.Fatalf("notification request = %+v", requests[0])
	}
	assertSubstringsInOrder(t, body, "Next action:", "AI diagnosis:")
	assertSubstringsInOrder(t, body, "Missing evidence:", "AI diagnosis:")
	assertSubstringsInOrder(t, body, "Evidence collection suggestions:", "AI diagnosis:")
	assertSubstringsInOrder(t, body, "Executable evidence requests: 1", "AI diagnosis:")

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 20)
		if err != nil {
			return err
		}
		var event domain.DiagnosisTaskEvent
		for _, candidate := range events {
			if candidate.ID == domain.DiagnosisTaskEventID(first.LifecycleEventID) {
				event = candidate
				break
			}
		}
		if event.ID == 0 || event.Kind != "diagnosis_room.assistant_turn_notification_sent" {
			t.Fatalf("events = %+v, want assistant-turn notification event %d", events, first.LifecycleEventID)
		}
		var payload struct {
			IdempotencyKey    string                            `json:"idempotency_key"`
			ChannelProfile    int64                             `json:"notification_channel_profile_id"`
			RoomURL           string                            `json:"room_url"`
			AssistantMessage  string                            `json:"assistant_message"`
			Confidence        string                            `json:"confidence"`
			RequiresReview    bool                              `json:"requires_human_review"`
			Findings          []string                          `json:"findings"`
			Recommended       []string                          `json:"recommended_actions"`
			EvidenceRequests  []diagnosisroom.EvidenceRequest   `json:"evidence_requests"`
			ConsultationState diagnosisroom.ConsultationInsight `json:"consultation_insight"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("notification event payload: %v", err)
		}
		if payload.IdempotencyKey != first.IdempotencyKey ||
			payload.ChannelProfile != 5 ||
			payload.RoomURL != "https://openclarion.example.test/ops/diagnosis-room?auth_mode=session&session_id=session-room-assistant-turn-notify&wecom_auto_login=1&wecom_launch_context=app_conversation" ||
			payload.AssistantMessage != turnReq.AssistantMessage ||
			payload.Confidence != "high" ||
			!payload.RequiresReview ||
			!reflect.DeepEqual(payload.Findings, []string{"api-1 CPU exceeded threshold"}) ||
			!reflect.DeepEqual(payload.Recommended, []string{"Inspect recent deployment"}) ||
			len(payload.EvidenceRequests) != 1 ||
			payload.ConsultationState.ConclusionStatus != "needs_evidence" ||
			payload.ConsultationState.ConfidenceRationale != "Confidence depends on sibling alert and restart evidence." {
			t.Fatalf("notification event payload = %+v raw=%s", payload, event.Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify notification event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_SendAssistantTurnNotificationRetriesAfterFailure(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-assistant-turn-notification-failure")
	im := &recordingIMProvider{err: &ports.IMError{Message: "webhook unavailable", StatusCode: 503, Retryable: true}}
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithIMProvider(im))
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 17, 55, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-assistant-turn-notify-failure",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	turnReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-assistant-turn-notify-failure", startedAt)
	persisted, err := activities.PersistDiagnosisTurn(ctx, turnReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn: %v", err)
	}
	req := temporalpkg.SendDiagnosisRoomAssistantTurnNotificationInput{
		SessionID:           "session-room-assistant-turn-notify-failure",
		DiagnosisTaskID:     int64(seed.TaskID),
		OwnerSubject:        "owner-1",
		AssistantTurnID:     persisted.AssistantTurnID,
		AssistantMessageID:  persisted.AssistantMessageID,
		AssistantSequence:   persisted.AssistantSequence,
		TurnCount:           persisted.TurnCount,
		OccurredAt:          persisted.AssistantOccurredAt,
		AssistantMessage:    persisted.AssistantMessage,
		Confidence:          persisted.Confidence,
		RequiresHumanReview: persisted.RequiresHumanReview,
		Findings:            persisted.Findings,
		RecommendedActions:  persisted.RecommendedActions,
		EvidenceRequests:    persisted.EvidenceRequests,
		Insight:             persisted.Insight,
	}

	_, err = activities.SendDiagnosisRoomAssistantTurnNotification(ctx, req)
	if err == nil {
		t.Fatal("SendDiagnosisRoomAssistantTurnNotification first error = nil, want retryable notification error")
	}
	var failedEvent domain.DiagnosisTaskEvent
	var failedPayload struct {
		IdempotencyKey string `json:"idempotency_key"`
		ProviderStatus string `json:"provider_status"`
		ProviderRaw    struct {
			Retryable  bool `json:"retryable"`
			StatusCode int  `json:"status_code"`
		} `json:"provider_raw"`
	}
	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 20)
		if err != nil {
			return err
		}
		for _, candidate := range events {
			if candidate.Kind != "diagnosis_room.assistant_turn_notification_sent" {
				continue
			}
			if err := json.Unmarshal(candidate.Payload, &failedPayload); err != nil {
				return err
			}
			if failedPayload.ProviderStatus == "failed" {
				failedEvent = candidate
				return nil
			}
		}
		return fmt.Errorf("failed assistant-turn notification event not found")
	})
	if err != nil {
		t.Fatalf("verify first failed notification event: %v", err)
	}
	if failedEvent.ID == 0 ||
		failedPayload.IdempotencyKey == "" ||
		!failedPayload.ProviderRaw.Retryable ||
		failedPayload.ProviderRaw.StatusCode != 503 {
		t.Fatalf("failed notification event = %+v payload=%+v", failedEvent, failedPayload)
	}
	im.mu.Lock()
	im.err = nil
	im.delivery = ports.IMDelivery{
		ProviderMessageID: "msg-assistant-turn-retry",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"accepted":true}`),
	}
	im.mu.Unlock()
	second, err := activities.SendDiagnosisRoomAssistantTurnNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomAssistantTurnNotification second: %v", err)
	}
	third, err := activities.SendDiagnosisRoomAssistantTurnNotification(ctx, req)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomAssistantTurnNotification third: %v", err)
	}
	if second.LifecycleEventID == 0 ||
		second.LifecycleEventID == int64(failedEvent.ID) ||
		second.ProviderMessageID != "msg-assistant-turn-retry" ||
		second.NotificationStatus != "delivered" ||
		second.IdempotencyKey != failedPayload.IdempotencyKey {
		t.Fatalf("second notification result = %+v after failed payload=%+v", second, failedPayload)
	}
	if third.LifecycleEventID != second.LifecycleEventID ||
		third.ProviderMessageID != second.ProviderMessageID ||
		third.NotificationStatus != second.NotificationStatus ||
		third.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("third notification result = %+v after second=%+v", third, second)
	}
	if requests := im.Requests(); len(requests) != 2 ||
		requests[0].IdempotencyKey != failedPayload.IdempotencyKey ||
		requests[1].IdempotencyKey != failedPayload.IdempotencyKey {
		t.Fatalf("notification requests = %+v, want failed attempt plus delivered retry with same idempotency key", requests)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 20)
		if err != nil {
			return err
		}
		var event domain.DiagnosisTaskEvent
		for _, candidate := range events {
			if candidate.ID == failedEvent.ID {
				event = candidate
				break
			}
		}
		if event.ID == 0 || event.Kind != "diagnosis_room.assistant_turn_notification_sent" {
			t.Fatalf("events = %+v, want failed assistant-turn notification event %d", events, failedEvent.ID)
		}
		var delivered domain.DiagnosisTaskEvent
		for _, candidate := range events {
			if candidate.ID == domain.DiagnosisTaskEventID(second.LifecycleEventID) {
				delivered = candidate
				break
			}
		}
		if delivered.ID == 0 || delivered.Kind != "diagnosis_room.assistant_turn_notification_sent" {
			t.Fatalf("events = %+v, want delivered retry event %d", events, second.LifecycleEventID)
		}
		var deliveredPayload struct {
			IdempotencyKey    string `json:"idempotency_key"`
			ProviderMessageID string `json:"provider_message_id"`
			ProviderStatus    string `json:"provider_status"`
		}
		if err := json.Unmarshal(delivered.Payload, &deliveredPayload); err != nil {
			t.Fatalf("delivered notification event payload: %v", err)
		}
		if deliveredPayload.IdempotencyKey != failedPayload.IdempotencyKey ||
			deliveredPayload.ProviderMessageID != "msg-assistant-turn-retry" ||
			deliveredPayload.ProviderStatus != "delivered" {
			t.Fatalf("delivered notification event payload = %+v raw=%s", deliveredPayload, delivered.Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify notification event: %v", err)
	}
}

func TestDiagnosisRoomPersistenceActivities_CloseNotificationIncludesFinalConclusion(t *testing.T) {
	seed := seedDiagnosisTask(t, "room-close-notification-final")
	im := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "msg-final-close",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"accepted":true}`),
	}}
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithIMProvider(im))
	ctx := context.Background()
	startedAt := time.Date(2026, 5, 28, 18, 0, 0, 0, time.UTC)
	_, err := activities.EnsureDiagnosisChatSession(ctx, temporalpkg.EnsureDiagnosisChatSessionInput{
		SessionID:       "session-room-close-notify-final",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureDiagnosisChatSession: %v", err)
	}
	turnReq := validPersistDiagnosisTurnInput(seed.TaskID, "session-room-close-notify-final", startedAt)
	turnReq.AssistantMessage = "CPU saturation is explained by the latest rollout and requires scaling before traffic peak."
	turnReq.RawOutput = json.RawMessage(`{
		"schema_version":"diagnosis_turn.v1",
		"message":"CPU saturation is explained by the latest rollout and requires scaling before traffic peak.",
		"findings":["api-1 CPU exceeded threshold"],
		"recommended_actions":["Scale the affected deployment before traffic peak"],
		"evidence_requests":[{"tool":"active_alerts","reason":"Confirm sibling alerts remain firing.","limit":5}],
		"confidence":"high",
		"requires_human_review":true,
		"confidence_rationale":"Deployment timing and CPU evidence are aligned.",
		"missing_evidence_requests":[{"label":"Restart cause","detail":"Inspect previous container logs before final remediation.","priority":"high"}],
		"evidence_collection_suggestions":[{"label":"CPU trend","detail":"Collect a bounded CPU range query for the affected deployment.","priority":"medium"}],
		"conclusion_status":"final"
	}`)
	persisted, err := activities.PersistDiagnosisTurn(ctx, turnReq)
	if err != nil {
		t.Fatalf("PersistDiagnosisTurn: %v", err)
	}
	closeReq := temporalpkg.CloseDiagnosisChatSessionInput{
		SessionID:       "session-room-close-notify-final",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		ConfirmedBy:     "owner-1",
		TurnCount:       persisted.TurnCount,
		ClosedAt:        startedAt.Add(5 * time.Minute),
		Reason:          "human_confirmed",
	}
	if _, err := activities.CloseDiagnosisChatSession(ctx, closeReq); err != nil {
		t.Fatalf("CloseDiagnosisChatSession: %v", err)
	}

	result, err := activities.SendDiagnosisRoomCloseNotification(ctx, closeReq)
	if err != nil {
		t.Fatalf("SendDiagnosisRoomCloseNotification: %v", err)
	}
	requests := im.Requests()
	if len(requests) != 1 {
		t.Fatalf("notification requests len = %d, want 1", len(requests))
	}
	body := requests[0].Body
	if requests[0].Severity != "warning" ||
		!strings.Contains(body, "Confidence: high") ||
		!strings.Contains(body, "Human review: required") ||
		!strings.Contains(body, "AI conclusion: CPU saturation is explained by the latest rollout") ||
		!strings.Contains(body, "Confidence rationale: Deployment timing and CPU evidence are aligned.") ||
		!strings.Contains(body, "Findings:") ||
		!strings.Contains(body, "api-1 CPU exceeded threshold") ||
		!strings.Contains(body, "Recommended actions:") ||
		!strings.Contains(body, "Scale the affected deployment before traffic peak") ||
		!strings.Contains(body, "Missing evidence:") ||
		!strings.Contains(body, "[high] Restart cause - Inspect previous container logs before final remediation.") ||
		!strings.Contains(body, "Evidence collection suggestions:") ||
		!strings.Contains(body, "[medium] CPU trend - Collect a bounded CPU range query for the affected deployment.") ||
		!strings.Contains(body, "Next action: collect 1 executable evidence request(s) and provide 1 operator-supplied evidence item(s).") ||
		!strings.Contains(body, "Executable evidence requests: 1") ||
		!strings.Contains(body, "1. active_alerts - Confirm sibling alerts remain firing. (limit=5)") ||
		!strings.Contains(body, "Evidence context refs:") {
		t.Fatalf("notification request = %+v", requests[0])
	}
	assertSubstringsInOrder(t, body, "Next action:", "AI conclusion:")
	assertSubstringsInOrder(t, body, "Missing evidence:", "AI conclusion:")
	assertSubstringsInOrder(t, body, "Evidence collection suggestions:", "AI conclusion:")
	assertSubstringsInOrder(t, body, "Executable evidence requests: 1", "AI conclusion:")

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 20)
		if err != nil {
			return err
		}
		var event domain.DiagnosisTaskEvent
		for _, candidate := range events {
			if candidate.ID == domain.DiagnosisTaskEventID(result.LifecycleEventID) {
				event = candidate
				break
			}
		}
		if event.ID == 0 {
			t.Fatalf("events = %+v, want notification event %d", events, result.LifecycleEventID)
		}
		var payload struct {
			FinalConclusion temporalpkg.DiagnosisRoomFinalConclusion `json:"final_conclusion"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("notification event payload: %v", err)
		}
		if payload.FinalConclusion.Status != "available" ||
			payload.FinalConclusion.Content != turnReq.AssistantMessage ||
			payload.FinalConclusion.Confidence != "high" ||
			payload.FinalConclusion.RequiresHumanReview == nil ||
			!*payload.FinalConclusion.RequiresHumanReview {
			t.Fatalf("notification final conclusion = %+v raw=%s", payload.FinalConclusion, event.Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify notification event: %v", err)
	}
}

func validPersistDiagnosisTurnInput(taskID domain.DiagnosisTaskID, sessionID string, startedAt time.Time) temporalpkg.PersistDiagnosisTurnInput {
	rawOutput := json.RawMessage(`{
		"schema_version":"diagnosis_turn.v1",
		"message":"CPU saturation is concentrated on api-1.",
		"findings":["api-1 CPU exceeded threshold"],
		"recommended_actions":["Inspect recent deployment"],
		"evidence_requests":[{"tool":"active_alerts","reason":"Need current active sibling alerts.","limit":5}],
		"confidence":"high",
		"requires_human_review":true,
		"confidence_rationale":"Confidence depends on sibling alert and restart evidence.",
		"missing_evidence_requests":[{"label":"Restart cause","detail":"Inspect previous container logs.","priority":"high"}],
		"evidence_collection_suggestions":[{"label":"CPU trend","detail":"Collect a bounded CPU range query.","priority":"medium"}],
		"conclusion_status":"needs_evidence"
	}`)
	return temporalpkg.PersistDiagnosisTurnInput{
		SessionID:           sessionID,
		DiagnosisTaskID:     int64(taskID),
		OwnerSubject:        "owner-1",
		UserMessageID:       "msg-1",
		AssistantMessageID:  "msg-1/assistant",
		UserSequence:        1,
		AssistantSequence:   2,
		TurnCount:           1,
		ActorSubject:        "owner-1",
		UserMessage:         "Why is CPU saturated?",
		AssistantMessage:    "CPU saturation is concentrated on api-1.",
		UserOccurredAt:      startedAt.Add(time.Minute),
		AssistantOccurredAt: startedAt.Add(time.Minute + 2*time.Second),
		ContextBytes:        512,
		InvocationID:        "diagnosis-room/task-1/msg-abcdef",
		RuntimeID:           "container-1",
		ContainerStartedAt:  startedAt.Add(time.Minute),
		ContainerFinishedAt: startedAt.Add(time.Minute + time.Second),
		RawOutput:           rawOutput,
	}
}

func mustPublicBaseURL(t *testing.T) *url.URL {
	t.Helper()
	parsed, err := url.Parse("https://openclarion.example.test/ops")
	if err != nil {
		t.Fatalf("parse public base URL: %v", err)
	}
	return parsed
}

func assertSubstringsInOrder(t *testing.T, value, first, second string) {
	t.Helper()
	firstIndex := strings.Index(value, first)
	secondIndex := strings.Index(value, second)
	if firstIndex < 0 || secondIndex < 0 {
		t.Fatalf("value is missing ordered substrings %q then %q: %s", first, second, value)
	}
	if firstIndex >= secondIndex {
		t.Fatalf("substring %q index %d must be before %q index %d: %s", first, firstIndex, second, secondIndex, value)
	}
}

func seedEvidenceSnapshot(t *testing.T, label string) domain.EvidenceSnapshotID {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	var snapshotID domain.EvidenceSnapshotID
	err := env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		group, err := domain.NewAlertGroup(
			"room-grp-"+label,
			json.RawMessage(`{"region":"test"}`),
			domain.GroupSeverityWarning,
			1,
			now,
			now,
			nil,
		)
		if err != nil {
			return err
		}
		savedGroup, err := uow.Alerts().SaveGroup(ctx, group)
		if err != nil {
			return err
		}
		snapshot, err := domain.NewEvidenceSnapshot(
			savedGroup.ID,
			"room-digest-"+label,
			json.RawMessage(`{"metric":"cpu"}`),
			json.RawMessage(`{"providers":{"prom":"ok"}}`),
			domain.SnapshotStatusComplete,
			nil,
			"DiagnosisRoomStarter",
		)
		if err != nil {
			return err
		}
		savedSnapshot, err := uow.Evidence().Save(ctx, snapshot)
		if err != nil {
			return err
		}
		snapshotID = savedSnapshot.ID
		return nil
	})
	if err != nil {
		t.Fatalf("seedEvidenceSnapshot(%s): %v", label, err)
	}
	return snapshotID
}
