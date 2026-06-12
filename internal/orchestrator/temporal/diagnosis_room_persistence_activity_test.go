package temporal_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
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
		first.EvidenceRequests[0].Tool != domain.DiagnosisToolKindActiveAlerts {
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
			InvocationID        string                          `json:"invocation_id"`
			Confidence          string                          `json:"confidence"`
			RequiresHumanReview bool                            `json:"requires_human_review"`
			EvidenceRequests    []diagnosisroom.EvidenceRequest `json:"evidence_requests"`
		}
		if err := json.Unmarshal(turns[1].Metadata, &assistantMeta); err != nil {
			t.Fatalf("assistant metadata: %v", err)
		}
		if assistantMeta.InvocationID != req.InvocationID ||
			assistantMeta.Confidence != "high" ||
			!assistantMeta.RequiresHumanReview ||
			len(assistantMeta.EvidenceRequests) != 1 ||
			assistantMeta.EvidenceRequests[0].Reason != "Need current active sibling alerts." {
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
			UserMessageID      string                          `json:"user_message_id"`
			AssistantMessageID string                          `json:"assistant_message_id"`
			TurnCount          int                             `json:"turn_count"`
			EvidenceRequests   []diagnosisroom.EvidenceRequest `json:"evidence_requests"`
		}
		if err := json.Unmarshal(events[1].Payload, &turnPayload); err != nil {
			t.Fatalf("turn event payload: %v", err)
		}
		if turnPayload.UserMessageID != req.UserMessageID ||
			turnPayload.AssistantMessageID != req.AssistantMessageID ||
			turnPayload.TurnCount != 1 ||
			len(turnPayload.EvidenceRequests) != 1 ||
			turnPayload.EvidenceRequests[0].Limit != 5 {
			t.Fatalf("turn event payload = %+v raw=%s", turnPayload, events[1].Payload)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify persisted turns: %v", err)
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
		second.FinalConclusion != first.FinalConclusion {
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
		SessionID:       "session-room-close-conclusion",
		DiagnosisTaskID: int64(seed.TaskID),
		OwnerSubject:    "owner-1",
		TurnCount:       1,
		ClosedAt:        startedAt.Add(5 * time.Minute),
		Reason:          "user_done",
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
	if first.FinalConclusion.Status != "available" ||
		first.FinalConclusion.Source != "latest_assistant_turn" ||
		first.FinalConclusion.AssistantTurnID != persisted.AssistantTurnID ||
		first.FinalConclusion.AssistantMessageID != turnReq.AssistantMessageID ||
		first.FinalConclusion.AssistantSequence != turnReq.AssistantSequence ||
		first.FinalConclusion.AssistantOccurredAt == nil ||
		!first.FinalConclusion.AssistantOccurredAt.Equal(domain.NormalizeUTCMicro(turnReq.AssistantOccurredAt)) ||
		first.FinalConclusion.Content != turnReq.AssistantMessage ||
		first.FinalConclusion.Confidence != "high" ||
		first.FinalConclusion.RequiresHumanReview == nil ||
		!*first.FinalConclusion.RequiresHumanReview {
		t.Fatalf("close result final conclusion = %+v", first.FinalConclusion)
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
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
				AssistantTurnID     int64      `json:"assistant_turn_id"`
				AssistantMessageID  string     `json:"assistant_message_id"`
				AssistantSequence   int        `json:"assistant_sequence"`
				AssistantOccurredAt *time.Time `json:"assistant_occurred_at"`
				Content             string     `json:"content"`
				Confidence          string     `json:"confidence"`
				RequiresHumanReview *bool      `json:"requires_human_review"`
			} `json:"final_conclusion"`
		}
		if err := json.Unmarshal(closeEvent.Payload, &payload); err != nil {
			t.Fatalf("close event payload: %v", err)
		}
		if payload.CloseReason != "user_done" ||
			payload.TurnCount != 1 ||
			payload.ConclusionVersion != "diagnosis-room-close.v1" ||
			payload.Conclusion.Status != "available" ||
			payload.Conclusion.Source != "latest_assistant_turn" ||
			payload.Conclusion.AssistantTurnID != persisted.AssistantTurnID ||
			payload.Conclusion.AssistantMessageID != turnReq.AssistantMessageID ||
			payload.Conclusion.AssistantSequence != turnReq.AssistantSequence ||
			payload.Conclusion.AssistantOccurredAt == nil ||
			!payload.Conclusion.AssistantOccurredAt.Equal(domain.NormalizeUTCMicro(turnReq.AssistantOccurredAt)) ||
			payload.Conclusion.Content != turnReq.AssistantMessage ||
			payload.Conclusion.Confidence != "high" ||
			payload.Conclusion.RequiresHumanReview == nil ||
			!*payload.Conclusion.RequiresHumanReview {
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
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithIMProvider(im))
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
		requests[0].Body == "" {
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
				ProviderMessageID string `json:"provider_message_id"`
				ProviderStatus    string `json:"provider_status"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("notification event payload: %v", err)
			}
			if payload.IdempotencyKey != first.IdempotencyKey ||
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

func validPersistDiagnosisTurnInput(taskID domain.DiagnosisTaskID, sessionID string, startedAt time.Time) temporalpkg.PersistDiagnosisTurnInput {
	rawOutput := json.RawMessage(`{
		"schema_version":"diagnosis_turn.v1",
		"message":"CPU saturation is concentrated on api-1.",
		"findings":["api-1 CPU exceeded threshold"],
		"recommended_actions":["Inspect recent deployment"],
		"evidence_requests":[{"tool":"active_alerts","reason":"Need current active sibling alerts.","limit":5}],
		"confidence":"high",
		"requires_human_review":true
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
