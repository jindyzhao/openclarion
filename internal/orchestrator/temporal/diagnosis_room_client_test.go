package temporal

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type recordingDiagnosisRoomTemporalClient struct {
	updateCalled  int
	updateOptions client.UpdateWorkflowOptions
	updateHandle  client.WorkflowUpdateHandle
	updateErr     error

	queryCalled     int
	queryWorkflowID string
	queryRunID      string
	queryType       string
	queryArgs       []interface{}
	queryValue      converter.EncodedValue
	queryErr        error
}

func (c *recordingDiagnosisRoomTemporalClient) UpdateWorkflow(_ context.Context, options client.UpdateWorkflowOptions) (client.WorkflowUpdateHandle, error) {
	c.updateCalled++
	c.updateOptions = options
	if c.updateErr != nil {
		return nil, c.updateErr
	}
	return c.updateHandle, nil
}

func (c *recordingDiagnosisRoomTemporalClient) QueryWorkflow(_ context.Context, workflowID string, runID string, queryType string, args ...interface{}) (converter.EncodedValue, error) {
	c.queryCalled++
	c.queryWorkflowID = workflowID
	c.queryRunID = runID
	c.queryType = queryType
	c.queryArgs = append([]interface{}(nil), args...)
	if c.queryErr != nil {
		return nil, c.queryErr
	}
	return c.queryValue, nil
}

type fakeWorkflowUpdateHandle struct {
	result SubmitDiagnosisTurnResult
	err    error
}

func (h fakeWorkflowUpdateHandle) WorkflowID() string { return "diagnosis-room-session-1" }
func (h fakeWorkflowUpdateHandle) RunID() string      { return "run-1" }
func (h fakeWorkflowUpdateHandle) UpdateID() string   { return "update-1" }

func (h fakeWorkflowUpdateHandle) Get(_ context.Context, valuePtr interface{}) error {
	if h.err != nil {
		return h.err
	}
	out, ok := valuePtr.(*SubmitDiagnosisTurnResult)
	if !ok {
		return errors.New("unexpected result pointer")
	}
	*out = h.result
	return nil
}

type fakeEncodedValue struct {
	value DiagnosisRoomWorkflowState
	err   error
}

func (v fakeEncodedValue) HasValue() bool { return true }

func (v fakeEncodedValue) Get(valuePtr interface{}) error {
	if v.err != nil {
		return v.err
	}
	out, ok := valuePtr.(*DiagnosisRoomWorkflowState)
	if !ok {
		return errors.New("unexpected query pointer")
	}
	*out = v.value
	return nil
}

func TestDiagnosisRoomClient_SubmitDiagnosisTurnUsesCompletedUpdate(t *testing.T) {
	temporalClient := &recordingDiagnosisRoomTemporalClient{
		updateHandle: fakeWorkflowUpdateHandle{
			result: SubmitDiagnosisTurnResult{
				SessionID:           "session-1",
				ChatSessionID:       21,
				MessageID:           "msg-1",
				AssistantMessageID:  "msg-1-assistant",
				UserTurnID:          31,
				AssistantTurnID:     32,
				UserSequence:        1,
				AssistantSequence:   2,
				TurnCount:           1,
				ContextBytes:        100,
				Status:              "open",
				AssistantMessage:    "CPU alert is still firing.",
				RequiresHumanReview: true,
				Confidence:          "medium",
				EvidenceRequests: []diagnosisroom.EvidenceRequest{{
					Tool:   domain.DiagnosisToolKindActiveAlerts,
					Reason: "Need current sibling alerts.",
					Limit:  5,
				}},
				CollectionResults: []diagnosisevidence.Item{{
					Tool:           domain.DiagnosisToolKindActiveAlerts,
					Status:         diagnosisevidence.StatusCollected,
					ReasonCode:     diagnosisevidence.ReasonOK,
					Message:        "Active alert collection succeeded.",
					ObservedAlerts: 1,
					ActiveAlerts: []ports.ActiveAlert{{
						Source:   "alertmanager",
						Labels:   map[string]string{"alertname": "CPUHigh"},
						StartsAt: time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC),
					}},
					CollectedAt: time.Date(2026, 6, 17, 10, 0, 1, 0, time.UTC),
				}},
				Insight: diagnosisroom.ConsultationInsight{
					ConfidenceRationale: "CPU evidence is present but restart evidence is missing.",
					MissingEvidenceRequests: []diagnosisroom.ConsultationEvidenceRequest{{
						Label:    "Restart cause",
						Detail:   "Inspect previous pod logs.",
						Priority: "high",
					}},
					ConclusionStatus: "needs_evidence",
				},
				FollowUpTurns: []DiagnosisRoomFollowUpTurnResult{{
					MessageID:           "msg-1/auto-evidence-1",
					UserMessage:         "OpenClarion automatic evidence follow-up.",
					AssistantMessageID:  "msg-1/auto-evidence-1/assistant",
					UserTurnID:          33,
					AssistantTurnID:     34,
					UserSequence:        3,
					AssistantSequence:   4,
					TurnCount:           2,
					ContextBytes:        256,
					AssistantMessage:    "Collected evidence confirms CPU saturation.",
					RequiresHumanReview: false,
					Confidence:          "high",
					Insight:             diagnosisroom.ConsultationInsight{ConclusionStatus: "final"},
					Trigger:             "collected_evidence",
				}},
			},
		},
	}
	roomClient := newDiagnosisRoomClient(temporalClient)

	got, err := roomClient.SubmitDiagnosisTurn(context.Background(), ports.DiagnosisRoomSubmitTurnRequest{
		SessionID:    "session-1",
		MessageID:    "msg-1",
		ActorSubject: "owner-1",
		Message:      "Please continue investigating this alert",
	})
	if err != nil {
		t.Fatalf("SubmitDiagnosisTurn: %v", err)
	}

	if temporalClient.updateCalled != 1 {
		t.Fatalf("UpdateWorkflow calls = %d, want 1", temporalClient.updateCalled)
	}
	if temporalClient.updateOptions.WorkflowID != "diagnosis-room-session-1" {
		t.Fatalf("WorkflowID = %q, want diagnosis-room-session-1", temporalClient.updateOptions.WorkflowID)
	}
	if temporalClient.updateOptions.UpdateName != DiagnosisRoomSubmitTurnUpdate {
		t.Fatalf("UpdateName = %q, want %q", temporalClient.updateOptions.UpdateName, DiagnosisRoomSubmitTurnUpdate)
	}
	if temporalClient.updateOptions.WaitForStage != client.WorkflowUpdateStageCompleted {
		t.Fatalf("WaitForStage = %v, want WorkflowUpdateStageCompleted", temporalClient.updateOptions.WaitForStage)
	}
	if len(temporalClient.updateOptions.Args) != 1 {
		t.Fatalf("Args len = %d, want 1", len(temporalClient.updateOptions.Args))
	}
	req, ok := temporalClient.updateOptions.Args[0].(SubmitDiagnosisTurnRequest)
	if !ok {
		t.Fatalf("Arg[0] = %T, want SubmitDiagnosisTurnRequest", temporalClient.updateOptions.Args[0])
	}
	if req.MessageID != "msg-1" || req.ActorSubject != "owner-1" || req.Message != "Please continue investigating this alert" {
		t.Fatalf("Update request = %+v", req)
	}

	want := ports.DiagnosisRoomSubmitTurnResult{
		SessionID:           "session-1",
		ChatSessionID:       domain.ChatSessionID(21),
		MessageID:           "msg-1",
		AssistantMessageID:  "msg-1-assistant",
		UserTurnID:          domain.ChatTurnID(31),
		AssistantTurnID:     domain.ChatTurnID(32),
		UserSequence:        1,
		AssistantSequence:   2,
		TurnCount:           1,
		ContextBytes:        100,
		Status:              "open",
		AssistantMessage:    "CPU alert is still firing.",
		RequiresHumanReview: true,
		Confidence:          "medium",
		EvidenceRequests: []ports.DiagnosisRoomEvidenceRequest{{
			Tool:   domain.DiagnosisToolKindActiveAlerts,
			Reason: "Need current sibling alerts.",
			Limit:  5,
		}},
		CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
			Tool:           domain.DiagnosisToolKindActiveAlerts,
			Status:         "collected",
			ReasonCode:     "ok",
			Message:        "Active alert collection succeeded.",
			ObservedAlerts: 1,
			ActiveAlerts: []ports.DiagnosisRoomActiveAlert{{
				Source:   "alertmanager",
				Labels:   map[string]string{"alertname": "CPUHigh"},
				StartsAt: time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC),
			}},
			CollectedAt: time.Date(2026, 6, 17, 10, 0, 1, 0, time.UTC),
		}},
		ConsultationInsight: ports.DiagnosisRoomConsultationInsight{
			ConfidenceRationale: "CPU evidence is present but restart evidence is missing.",
			MissingEvidenceRequests: []ports.DiagnosisRoomConsultationEvidenceRequest{{
				Label:    "Restart cause",
				Detail:   "Inspect previous pod logs.",
				Priority: "high",
			}},
			ConclusionStatus: "needs_evidence",
		},
		FollowUpTurns: []ports.DiagnosisRoomFollowUpTurnResult{{
			MessageID:           "msg-1/auto-evidence-1",
			UserMessage:         "OpenClarion automatic evidence follow-up.",
			AssistantMessageID:  "msg-1/auto-evidence-1/assistant",
			UserTurnID:          domain.ChatTurnID(33),
			AssistantTurnID:     domain.ChatTurnID(34),
			UserSequence:        3,
			AssistantSequence:   4,
			TurnCount:           2,
			ContextBytes:        256,
			AssistantMessage:    "Collected evidence confirms CPU saturation.",
			RequiresHumanReview: false,
			Confidence:          "high",
			ConsultationInsight: ports.DiagnosisRoomConsultationInsight{ConclusionStatus: "final"},
			Trigger:             "collected_evidence",
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
}

func TestDiagnosisRoomClient_QueryDiagnosisRoom(t *testing.T) {
	startedAt := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	closedAt := startedAt.Add(5 * time.Minute)
	requiresHumanReview := true
	latestRequiresHumanReview := false
	temporalClient := &recordingDiagnosisRoomTemporalClient{
		queryValue: fakeEncodedValue{value: DiagnosisRoomWorkflowState{
			SessionID:       "session-1",
			ChatSessionID:   21,
			DiagnosisTaskID: 11,
			OwnerSubject:    "owner-1",
			Status:          "closed",
			TurnCount:       1,
			StartedAt:       startedAt,
			LastActivityAt:  closedAt,
			ClosedAt:        &closedAt,
			CloseReason:     "user_requested",
			FinalConclusion: &DiagnosisRoomFinalConclusion{
				Status:              "available",
				Source:              "latest_assistant_turn",
				AssistantTurnID:     32,
				AssistantMessageID:  "msg-1/assistant",
				AssistantSequence:   2,
				AssistantOccurredAt: &closedAt,
				Content:             "The alert has recovered.",
				Confidence:          "high",
				RequiresHumanReview: &requiresHumanReview,
			},
			LatestInsight: &diagnosisroom.ConsultationInsight{
				ConfidenceRationale: "CPU evidence is present and restart evidence has recovered.",
				MissingEvidenceRequests: []diagnosisroom.ConsultationEvidenceRequest{{
					Label:    "Deployment event",
					Detail:   "Confirm whether a deployment overlapped with recovery.",
					Priority: "medium",
				}},
				ConclusionStatus: "ready_for_review",
			},
			LatestConfidence:          "medium",
			LatestRequiresHumanReview: &latestRequiresHumanReview,
			InFlight:                  false,
			SeenMessageIDs:            []string{"msg-1"},
			Conversation: []diagnosisroom.ConversationTurn{
				{Role: "user", Content: "Please continue investigating"},
				{Role: "assistant", Content: "The alert has recovered."},
			},
		}},
	}
	roomClient := newDiagnosisRoomClient(temporalClient)

	got, err := roomClient.QueryDiagnosisRoom(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("QueryDiagnosisRoom: %v", err)
	}
	if temporalClient.queryWorkflowID != "diagnosis-room-session-1" || temporalClient.queryRunID != "" || temporalClient.queryType != DiagnosisRoomStateQuery {
		t.Fatalf("query workflow=%q run=%q type=%q", temporalClient.queryWorkflowID, temporalClient.queryRunID, temporalClient.queryType)
	}
	if len(temporalClient.queryArgs) != 0 {
		t.Fatalf("query args len = %d, want 0", len(temporalClient.queryArgs))
	}
	if got.ChatSessionID != domain.ChatSessionID(21) || got.DiagnosisTaskID != domain.DiagnosisTaskID(11) || got.CloseReason != "user_requested" {
		t.Fatalf("state = %+v", got)
	}
	if len(got.Conversation) != 2 || got.Conversation[1].Content != "The alert has recovered." {
		t.Fatalf("conversation = %+v", got.Conversation)
	}
	if got.ClosedAt == nil || !got.ClosedAt.Equal(closedAt) {
		t.Fatalf("ClosedAt = %v, want %s", got.ClosedAt, closedAt)
	}
	if got.FinalConclusion == nil ||
		got.FinalConclusion.Status != "available" ||
		got.FinalConclusion.AssistantTurnID != domain.ChatTurnID(32) ||
		got.FinalConclusion.AssistantMessageID != "msg-1/assistant" ||
		got.FinalConclusion.AssistantSequence != 2 ||
		got.FinalConclusion.AssistantOccurredAt == nil ||
		!got.FinalConclusion.AssistantOccurredAt.Equal(closedAt) ||
		got.FinalConclusion.Content != "The alert has recovered." ||
		got.FinalConclusion.Confidence != "high" ||
		got.FinalConclusion.RequiresHumanReview == nil ||
		!*got.FinalConclusion.RequiresHumanReview {
		t.Fatalf("FinalConclusion = %+v", got.FinalConclusion)
	}
	if got.LatestConsultationInsight == nil ||
		got.LatestConsultationInsight.ConfidenceRationale != "CPU evidence is present and restart evidence has recovered." ||
		len(got.LatestConsultationInsight.MissingEvidenceRequests) != 1 ||
		got.LatestConsultationInsight.MissingEvidenceRequests[0].Label != "Deployment event" ||
		got.LatestConsultationInsight.ConclusionStatus != "ready_for_review" ||
		got.LatestConfidence != "medium" ||
		got.LatestRequiresHumanReview == nil ||
		*got.LatestRequiresHumanReview {
		t.Fatalf("latest consultation state = insight=%+v confidence=%q review=%v",
			got.LatestConsultationInsight, got.LatestConfidence, got.LatestRequiresHumanReview)
	}
}

func TestDiagnosisRoomClient_Validation(t *testing.T) {
	roomClient := newDiagnosisRoomClient(&recordingDiagnosisRoomTemporalClient{})
	cases := []ports.DiagnosisRoomSubmitTurnRequest{
		{SessionID: "", MessageID: "msg-1", ActorSubject: "owner-1", Message: "msg"},
		{SessionID: " session-1 ", MessageID: "msg-1", ActorSubject: "owner-1", Message: "msg"},
		{SessionID: "session-1", MessageID: " msg-1 ", ActorSubject: "owner-1", Message: "msg"},
		{SessionID: "session-1", MessageID: "msg-1", ActorSubject: " ", Message: "msg"},
		{SessionID: "session-1", MessageID: "msg-1", ActorSubject: "owner-1", Message: " "},
	}
	for i, req := range cases {
		if _, err := roomClient.SubmitDiagnosisTurn(context.Background(), req); !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("case %d error = %v, want ErrInvariantViolation", i, err)
		}
	}
}
