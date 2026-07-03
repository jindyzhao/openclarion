package temporal

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservicepb "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	temporalsdk "go.temporal.io/sdk/temporal"
	"google.golang.org/protobuf/types/known/timestamppb"

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

	signalCalled     int
	signalWorkflowID string
	signalRunID      string
	signalName       string
	signalArg        interface{}
	signalErr        error

	getCalled     int
	getWorkflowID string
	getRunID      string
	getRun        client.WorkflowRun

	describeCalled     int
	describeWorkflowID string
	describeRunID      string
	describeResp       *workflowservicepb.DescribeWorkflowExecutionResponse
	describeErr        error

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

func (c *recordingDiagnosisRoomTemporalClient) SignalWorkflow(_ context.Context, workflowID string, runID string, signalName string, arg interface{}) error {
	c.signalCalled++
	c.signalWorkflowID = workflowID
	c.signalRunID = runID
	c.signalName = signalName
	c.signalArg = arg
	return c.signalErr
}

func (c *recordingDiagnosisRoomTemporalClient) GetWorkflow(_ context.Context, workflowID string, runID string) client.WorkflowRun {
	c.getCalled++
	c.getWorkflowID = workflowID
	c.getRunID = runID
	return c.getRun
}

func (c *recordingDiagnosisRoomTemporalClient) DescribeWorkflowExecution(_ context.Context, workflowID, runID string) (*workflowservicepb.DescribeWorkflowExecutionResponse, error) {
	c.describeCalled++
	c.describeWorkflowID = workflowID
	c.describeRunID = runID
	if c.describeErr != nil {
		return nil, c.describeErr
	}
	return c.describeResp, nil
}

type fakeWorkflowUpdateHandle struct {
	result interface{}
	err    error
}

type fakeWorkflowRun struct {
	workflowID string
	runID      string
	result     DiagnosisRoomWorkflowResult
	err        error
}

func (r fakeWorkflowRun) GetID() string    { return r.workflowID }
func (r fakeWorkflowRun) GetRunID() string { return r.runID }

func (r fakeWorkflowRun) Get(ctx context.Context, valuePtr interface{}) error {
	return r.GetWithOptions(ctx, valuePtr, client.WorkflowRunGetOptions{})
}

func (r fakeWorkflowRun) GetWithOptions(_ context.Context, valuePtr interface{}, _ client.WorkflowRunGetOptions) error {
	if r.err != nil {
		return r.err
	}
	out, ok := valuePtr.(*DiagnosisRoomWorkflowResult)
	if !ok {
		return errors.New("unexpected workflow result pointer")
	}
	*out = r.result
	return nil
}

func (h fakeWorkflowUpdateHandle) WorkflowID() string { return "diagnosis-room-session-1" }
func (h fakeWorkflowUpdateHandle) RunID() string      { return "run-1" }
func (h fakeWorkflowUpdateHandle) UpdateID() string   { return "update-1" }

func (h fakeWorkflowUpdateHandle) Get(_ context.Context, valuePtr interface{}) error {
	if h.err != nil {
		return h.err
	}
	if h.result == nil {
		return errors.New("missing update result")
	}
	target := reflect.ValueOf(valuePtr)
	if target.Kind() != reflect.Ptr || target.IsNil() {
		return errors.New("unexpected update result pointer")
	}
	value := reflect.ValueOf(h.result)
	if !value.Type().AssignableTo(target.Elem().Type()) {
		return errors.New("unexpected update result pointer")
	}
	target.Elem().Set(value)
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
				EvidenceTimeline: []DiagnosisRoomEvidenceTimelineEntry{{
					TurnCount:          1,
					MessageID:          "msg-1",
					AssistantMessageID: "msg-1-assistant",
					ActorSubject:       "owner-1",
					Trigger:            "operator_turn",
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
						CollectedAt:    time.Date(2026, 6, 17, 10, 0, 1, 0, time.UTC),
					}},
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
				LatestError: &DiagnosisRoomLatestError{
					Code:       "notification_failed",
					Message:    "AI diagnosis was saved, but downstream diagnosis notification delivery failed; review notification channel configuration.",
					MessageID:  "msg-1-assistant",
					OccurredAt: time.Date(2026, 6, 17, 10, 0, 2, 0, time.UTC),
				},
			},
		},
	}
	roomClient := newDiagnosisRoomClient(temporalClient)

	got, err := roomClient.SubmitDiagnosisTurn(context.Background(), ports.DiagnosisRoomSubmitTurnRequest{
		SessionID:    "session-1",
		MessageID:    "msg-1",
		ActorSubject: "owner-1",
		Message:      "Please continue investigating this alert",
		SupplementalEvidence: &ports.DiagnosisRoomSupplementalEvidence{
			Label:    "Restart cause",
			Detail:   "Inspect previous pod logs.",
			Priority: "high",
			Evidence: "Previous pod logs show OOMKilled before restart.",
		},
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
	if req.SupplementalEvidence == nil ||
		req.SupplementalEvidence.Label != "Restart cause" ||
		req.SupplementalEvidence.Detail != "Inspect previous pod logs." ||
		req.SupplementalEvidence.Priority != "high" ||
		req.SupplementalEvidence.Evidence != "Previous pod logs show OOMKilled before restart." {
		t.Fatalf("Update supplemental evidence = %+v", req.SupplementalEvidence)
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
		EvidenceTimeline: []ports.DiagnosisRoomEvidenceTimelineEntry{{
			TurnCount:          1,
			MessageID:          "msg-1",
			AssistantMessageID: "msg-1-assistant",
			ActorSubject:       "owner-1",
			Trigger:            "operator_turn",
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
				CollectedAt:    time.Date(2026, 6, 17, 10, 0, 1, 0, time.UTC),
			}},
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
		LatestError: &ports.DiagnosisRoomLatestError{
			Code:       "notification_failed",
			Message:    "AI diagnosis was saved, but downstream diagnosis notification delivery failed; review notification channel configuration.",
			MessageID:  "msg-1-assistant",
			OccurredAt: time.Date(2026, 6, 17, 10, 0, 2, 0, time.UTC),
		},
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
				Status:                  "available",
				Source:                  "latest_assistant_turn",
				EvidenceSnapshotID:      9001,
				ConclusionVersion:       "diagnosis-room-close.v1",
				RecordedAt:              &closedAt,
				ConfirmedBy:             "owner-1",
				SupplementalContextRefs: []string{"chat_session:21/turn:31", "chat_session:21/turn:32"},
				AssistantTurnID:         32,
				AssistantMessageID:      "msg-1/assistant",
				AssistantSequence:       2,
				AssistantOccurredAt:     &closedAt,
				Content:                 "The alert has recovered.",
				Confidence:              "high",
				RequiresHumanReview:     &requiresHumanReview,
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
			LatestEvidenceRequests: []diagnosisroom.EvidenceRequest{{
				TemplateID:           55,
				AlertSourceProfileID: 3,
				Tool:                 "metric_query",
				Reason:               "Read current CPU saturation.",
				Query:                `sum(rate(container_cpu_usage_seconds_total{namespace="prod"}[5m]))`,
			}},
			LatestCollectionResults: []diagnosisevidence.Item{{
				Request: diagnosisroom.EvidenceRequest{
					TemplateID:           55,
					AlertSourceProfileID: 3,
					Tool:                 "metric_query",
					Reason:               "Read current CPU saturation.",
					Query:                `sum(rate(container_cpu_usage_seconds_total{namespace="prod"}[5m]))`,
				},
				TemplateID:           domain.DiagnosisToolTemplateID(55),
				AlertSourceProfileID: domain.AlertSourceProfileID(3),
				Tool:                 domain.DiagnosisToolKind("metric_query"),
				Status:               diagnosisevidence.StatusCollected,
				ReasonCode:           diagnosisevidence.ReasonOK,
				Message:              "Metric query collected.",
				Query:                `sum(rate(container_cpu_usage_seconds_total{namespace="prod"}[5m]))`,
				ObservedMetricSeries: 1,
				CollectedAt:          closedAt,
			}},
			EvidenceTimeline: []DiagnosisRoomEvidenceTimelineEntry{{
				TurnCount:          1,
				MessageID:          "msg-1",
				AssistantMessageID: "msg-1/assistant",
				ActorSubject:       "owner-1",
				Trigger:            "operator_turn",
				EvidenceRequests: []diagnosisroom.EvidenceRequest{{
					TemplateID:           55,
					AlertSourceProfileID: 3,
					Tool:                 "metric_query",
					Reason:               "Read current CPU saturation.",
					Query:                `sum(rate(container_cpu_usage_seconds_total{namespace="prod"}[5m]))`,
				}},
				CollectionResults: []diagnosisevidence.Item{{
					TemplateID:           domain.DiagnosisToolTemplateID(55),
					AlertSourceProfileID: domain.AlertSourceProfileID(3),
					Tool:                 domain.DiagnosisToolKind("metric_query"),
					Status:               diagnosisevidence.StatusCollected,
					ReasonCode:           diagnosisevidence.ReasonOK,
					Message:              "Metric query collected.",
					Query:                `sum(rate(container_cpu_usage_seconds_total{namespace="prod"}[5m]))`,
					ObservedMetricSeries: 1,
					CollectedAt:          closedAt,
				}},
			}},
			SupplementalEvidence: []DiagnosisRoomSupplementalEvidenceRecord{{
				Label:              "Restart cause",
				Detail:             "Collect previous container logs.",
				Priority:           "high",
				Evidence:           "Previous logs show the pod restarted after OOMKilled.",
				ActorSubject:       "reviewer-1",
				UserMessageID:      "msg-2",
				AssistantMessageID: "msg-2/assistant",
				UserTurnID:         33,
				AssistantTurnID:    34,
				UserSequence:       3,
				AssistantSequence:  4,
				ProvidedAt:         closedAt,
			}},
			LatestError: &DiagnosisRoomLatestError{
				Code:       "llm_timeout",
				Message:    "Diagnosis turn failed before an assistant response; upstream LLM request timed out.",
				MessageID:  "msg-timeout",
				OccurredAt: closedAt,
			},
			InFlight:       false,
			SeenMessageIDs: []string{"msg-1"},
			Conversation: []diagnosisroom.ConversationTurn{
				{Role: "user", ActorSubject: "reviewer-1", Content: "Please continue investigating"},
				{Role: "assistant", ActorSubject: "openclarion:auto-diagnosis", Content: "The alert has recovered."},
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
	if got.Conversation[0].ActorSubject != "reviewer-1" ||
		got.Conversation[1].ActorSubject != "openclarion:auto-diagnosis" {
		t.Fatalf("conversation actor subjects = %+v", got.Conversation)
	}
	if got.ClosedAt == nil || !got.ClosedAt.Equal(closedAt) {
		t.Fatalf("ClosedAt = %v, want %s", got.ClosedAt, closedAt)
	}
	if len(got.LatestEvidenceRequests) != 1 ||
		got.LatestEvidenceRequests[0].TemplateID != domain.DiagnosisToolTemplateID(55) ||
		got.LatestEvidenceRequests[0].AlertSourceProfileID != domain.AlertSourceProfileID(3) ||
		len(got.LatestCollectionResults) != 1 ||
		got.LatestCollectionResults[0].TemplateID != domain.DiagnosisToolTemplateID(55) ||
		got.LatestCollectionResults[0].AlertSourceProfileID != domain.AlertSourceProfileID(3) ||
		got.LatestCollectionResults[0].ObservedMetricSeries != 1 {
		t.Fatalf("latest evidence = requests=%+v results=%+v",
			got.LatestEvidenceRequests, got.LatestCollectionResults)
	}
	if len(got.EvidenceTimeline) != 1 ||
		got.EvidenceTimeline[0].TurnCount != 1 ||
		got.EvidenceTimeline[0].MessageID != "msg-1" ||
		got.EvidenceTimeline[0].AssistantMessageID != "msg-1/assistant" ||
		got.EvidenceTimeline[0].ActorSubject != "owner-1" ||
		got.EvidenceTimeline[0].Trigger != "operator_turn" ||
		got.EvidenceTimeline[0].EvidenceRequests[0].TemplateID != domain.DiagnosisToolTemplateID(55) ||
		got.EvidenceTimeline[0].CollectionResults[0].ObservedMetricSeries != 1 {
		t.Fatalf("evidence timeline = %+v", got.EvidenceTimeline)
	}
	if len(got.SupplementalEvidence) != 1 ||
		got.SupplementalEvidence[0].Label != "Restart cause" ||
		got.SupplementalEvidence[0].ActorSubject != "reviewer-1" ||
		got.SupplementalEvidence[0].UserTurnID != domain.ChatTurnID(33) ||
		got.SupplementalEvidence[0].AssistantTurnID != domain.ChatTurnID(34) ||
		!got.SupplementalEvidence[0].ProvidedAt.Equal(closedAt) {
		t.Fatalf("supplemental evidence = %+v", got.SupplementalEvidence)
	}
	if got.LatestError == nil ||
		got.LatestError.Code != "llm_timeout" ||
		got.LatestError.MessageID != "msg-timeout" ||
		!got.LatestError.OccurredAt.Equal(closedAt) {
		t.Fatalf("latest error = %+v", got.LatestError)
	}
	if got.FinalConclusion == nil ||
		got.FinalConclusion.Status != "available" ||
		got.FinalConclusion.AssistantTurnID != domain.ChatTurnID(32) ||
		got.FinalConclusion.AssistantMessageID != "msg-1/assistant" ||
		got.FinalConclusion.AssistantSequence != 2 ||
		got.FinalConclusion.AssistantOccurredAt == nil ||
		!got.FinalConclusion.AssistantOccurredAt.Equal(closedAt) ||
		got.FinalConclusion.EvidenceSnapshotID != domain.EvidenceSnapshotID(9001) ||
		got.FinalConclusion.ConclusionVersion != "diagnosis-room-close.v1" ||
		got.FinalConclusion.RecordedAt == nil ||
		!got.FinalConclusion.RecordedAt.Equal(closedAt) ||
		got.FinalConclusion.ConfirmedBy != "owner-1" ||
		!reflect.DeepEqual(got.FinalConclusion.SupplementalContextRefs, []string{"chat_session:21/turn:31", "chat_session:21/turn:32"}) ||
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

func TestDiagnosisRoomClient_ListDiagnosisRoomWorkflowVisibility(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 0, 4, 0, 0, time.UTC)
	executionAt := startedAt.Add(2 * time.Second)
	temporalClient := &recordingDiagnosisRoomTemporalClient{
		describeResp: &workflowservicepb.DescribeWorkflowExecutionResponse{
			WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{
				Execution: &commonpb.WorkflowExecution{
					WorkflowId: "diagnosis-room-session-1",
					RunId:      "run-1",
				},
				Status:           enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
				TaskQueue:        "openclarion",
				StartTime:        timestamppb.New(startedAt),
				ExecutionTime:    timestamppb.New(executionAt),
				HistoryLength:    42,
				HistorySizeBytes: 2048,
			},
		},
	}
	roomClient := newDiagnosisRoomClient(temporalClient)

	got, err := roomClient.ListDiagnosisRoomWorkflowVisibility(context.Background(), []ports.DiagnosisRoomWorkflowVisibilityRequest{
		{WorkflowID: " diagnosis-room-session-1 ", RunID: " run-1 "},
		{WorkflowID: "diagnosis-room-session-1", RunID: "run-1"},
	})
	if err != nil {
		t.Fatalf("ListDiagnosisRoomWorkflowVisibility: %v", err)
	}
	if temporalClient.describeCalled != 1 ||
		temporalClient.describeWorkflowID != "diagnosis-room-session-1" ||
		temporalClient.describeRunID != "run-1" {
		t.Fatalf("describe call = %d workflow=%q run=%q", temporalClient.describeCalled, temporalClient.describeWorkflowID, temporalClient.describeRunID)
	}
	key := ports.DiagnosisRoomWorkflowVisibilityRequest{WorkflowID: "diagnosis-room-session-1", RunID: "run-1"}
	value, ok := got[key]
	if !ok {
		t.Fatalf("visibility missing key %+v: %+v", key, got)
	}
	if value.Status != "running" ||
		value.TaskQueue != "openclarion" ||
		value.StartTime == nil ||
		!value.StartTime.Equal(startedAt) ||
		value.ExecutionTime == nil ||
		!value.ExecutionTime.Equal(executionAt) ||
		value.HistoryLength != 42 ||
		value.HistorySizeBytes != 2048 {
		t.Fatalf("visibility = %+v", value)
	}
}

func TestDiagnosisRoomClient_ListDiagnosisRoomWorkflowVisibilityMapsNotFound(t *testing.T) {
	temporalClient := &recordingDiagnosisRoomTemporalClient{
		describeErr: serviceerror.NewNotFound("missing workflow"),
	}
	roomClient := newDiagnosisRoomClient(temporalClient)

	got, err := roomClient.ListDiagnosisRoomWorkflowVisibility(context.Background(), []ports.DiagnosisRoomWorkflowVisibilityRequest{
		{WorkflowID: "diagnosis-room-missing", RunID: "run-missing"},
	})
	if err != nil {
		t.Fatalf("ListDiagnosisRoomWorkflowVisibility: %v", err)
	}
	key := ports.DiagnosisRoomWorkflowVisibilityRequest{WorkflowID: "diagnosis-room-missing", RunID: "run-missing"}
	value, ok := got[key]
	if !ok {
		t.Fatalf("visibility missing key %+v: %+v", key, got)
	}
	if value.Status != "not_found" {
		t.Fatalf("status = %q, want not_found", value.Status)
	}
}

func TestDiagnosisRoomClient_CollectDiagnosisEvidenceUsesCompletedUpdate(t *testing.T) {
	collectedAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	temporalClient := &recordingDiagnosisRoomTemporalClient{
		updateHandle: fakeWorkflowUpdateHandle{
			result: CollectDiagnosisEvidenceUpdateResult{
				State: DiagnosisRoomWorkflowState{
					SessionID:       "session-1",
					ChatSessionID:   21,
					DiagnosisTaskID: 11,
					OwnerSubject:    "owner-1",
					Status:          "open",
					TurnCount:       2,
					StartedAt:       collectedAt.Add(-time.Minute),
					LastActivityAt:  collectedAt,
					LatestCollectionResults: []diagnosisevidence.Item{{
						Request: diagnosisroom.EvidenceRequest{
							AlertSourceProfileID: 4,
							Tool:                 domain.DiagnosisToolKindMetricRangeQuery,
							Reason:               "CPU and memory saturation window",
							Query:                "up",
							WindowSeconds:        300,
							StepSeconds:          60,
							Limit:                10,
						},
						AlertSourceProfileID: domain.AlertSourceProfileID(4),
						Tool:                 domain.DiagnosisToolKindMetricRangeQuery,
						Status:               diagnosisevidence.StatusCollected,
						ReasonCode:           diagnosisevidence.ReasonOK,
						Message:              "Metric range collection succeeded.",
						Query:                "up",
						WindowSeconds:        300,
						StepSeconds:          60,
						ObservedMetricSeries: 2,
						CollectedAt:          collectedAt,
					}},
				},
				FollowUpTurns: []DiagnosisRoomFollowUpTurnResult{{
					MessageID:           "collect-1/auto-evidence-1",
					UserMessage:         "OpenClarion automatic evidence follow-up.",
					AssistantMessageID:  "collect-1/auto-evidence-1/assistant",
					UserTurnID:          31,
					AssistantTurnID:     32,
					UserSequence:        3,
					AssistantSequence:   4,
					TurnCount:           2,
					ContextBytes:        256,
					AssistantMessage:    "Collected evidence raises confidence.",
					RequiresHumanReview: false,
					Confidence:          "high",
					CollectionResults: []diagnosisevidence.Item{{
						Tool:        domain.DiagnosisToolKindMetricRangeQuery,
						Status:      diagnosisevidence.StatusCollected,
						ReasonCode:  diagnosisevidence.ReasonOK,
						Message:     "Metric range collection succeeded.",
						CollectedAt: collectedAt,
					}},
					Insight: diagnosisroom.ConsultationInsight{
						ConclusionStatus: "final",
					},
				}},
			},
		},
	}
	roomClient := newDiagnosisRoomClient(temporalClient)

	got, err := roomClient.CollectDiagnosisEvidence(context.Background(), ports.DiagnosisRoomCollectEvidenceRequest{
		SessionID:    "session-1",
		MessageID:    "collect-1",
		ActorSubject: "reviewer-1",
		Message:      "Run planned evidence collection.",
		Requests: []ports.DiagnosisRoomEvidenceRequest{{
			AlertSourceProfileID: domain.AlertSourceProfileID(4),
			Tool:                 domain.DiagnosisToolKindMetricRangeQuery,
			Reason:               "CPU and memory saturation window",
			Query:                "up",
			WindowSeconds:        300,
			StepSeconds:          60,
			Limit:                10,
		}},
	})
	if err != nil {
		t.Fatalf("CollectDiagnosisEvidence: %v", err)
	}
	if temporalClient.updateCalled != 1 {
		t.Fatalf("UpdateWorkflow calls = %d, want 1", temporalClient.updateCalled)
	}
	if temporalClient.updateOptions.WorkflowID != "diagnosis-room-session-1" ||
		temporalClient.updateOptions.UpdateName != DiagnosisRoomCollectEvidenceUpdate ||
		temporalClient.updateOptions.WaitForStage != client.WorkflowUpdateStageCompleted {
		t.Fatalf("update options = %+v", temporalClient.updateOptions)
	}
	if len(temporalClient.updateOptions.Args) != 1 {
		t.Fatalf("update args len = %d, want 1", len(temporalClient.updateOptions.Args))
	}
	updateReq, ok := temporalClient.updateOptions.Args[0].(CollectDiagnosisEvidenceRequest)
	if !ok ||
		updateReq.MessageID != "collect-1" ||
		updateReq.ActorSubject != "reviewer-1" ||
		len(updateReq.Requests) != 1 ||
		updateReq.Requests[0].Tool != domain.DiagnosisToolKindMetricRangeQuery ||
		updateReq.Requests[0].AlertSourceProfileID != 4 {
		t.Fatalf("update arg = %#v", temporalClient.updateOptions.Args[0])
	}
	if got.State.TurnCount != 2 ||
		len(got.State.LatestCollectionResults) != 1 ||
		got.State.LatestCollectionResults[0].ObservedMetricSeries != 2 ||
		len(got.FollowUpTurns) != 1 ||
		got.FollowUpTurns[0].MessageID != "collect-1/auto-evidence-1" ||
		got.FollowUpTurns[0].ConsultationInsight.ConclusionStatus != "final" {
		t.Fatalf("collect result = %+v", got)
	}
}

func TestDiagnosisRoomClient_ConfirmDiagnosisConclusionUsesCompletedUpdateAndWaits(t *testing.T) {
	startedAt := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
	closedAt := startedAt.Add(3 * time.Minute)
	requiresHumanReview := true
	temporalClient := &recordingDiagnosisRoomTemporalClient{
		updateHandle: fakeWorkflowUpdateHandle{
			result: DiagnosisRoomWorkflowState{
				SessionID:       "session-1",
				ChatSessionID:   21,
				DiagnosisTaskID: 11,
				OwnerSubject:    "owner-1",
				Status:          "closed",
				TurnCount:       1,
				StartedAt:       startedAt,
				LastActivityAt:  closedAt,
				ClosedAt:        &closedAt,
				CloseReason:     "human_confirmed",
			},
		},
		getRun: fakeWorkflowRun{
			workflowID: "diagnosis-room-session-1",
			runID:      "run-1",
			result: DiagnosisRoomWorkflowResult{
				SessionID:       "session-1",
				ChatSessionID:   21,
				DiagnosisTaskID: 11,
				OwnerSubject:    "owner-1",
				Status:          "closed",
				TurnCount:       1,
				StartedAt:       startedAt,
				LastActivityAt:  closedAt,
				ClosedAt:        &closedAt,
				CloseReason:     "human_confirmed",
				FinalConclusion: &DiagnosisRoomFinalConclusion{
					Status:                  "available",
					Source:                  "latest_assistant_turn",
					EvidenceSnapshotID:      9001,
					ConclusionVersion:       "diagnosis-room-close.v1",
					RecordedAt:              &closedAt,
					ConfirmedBy:             "reviewer-1",
					SupplementalContextRefs: []string{"chat_session:21/turn:31", "chat_session:21/turn:32"},
					AssistantTurnID:         32,
					AssistantMessageID:      "msg-1/assistant",
					AssistantSequence:       2,
					AssistantOccurredAt:     &closedAt,
					Content:                 "The alert has recovered.",
					Confidence:              "high",
					RequiresHumanReview:     &requiresHumanReview,
				},
			},
		},
	}
	roomClient := newDiagnosisRoomClient(temporalClient)

	got, err := roomClient.ConfirmDiagnosisConclusion(context.Background(), ports.DiagnosisRoomConfirmConclusionRequest{
		SessionID:    "session-1",
		ActorSubject: "reviewer-1",
	})
	if err != nil {
		t.Fatalf("ConfirmDiagnosisConclusion: %v", err)
	}
	if temporalClient.updateCalled != 1 {
		t.Fatalf("UpdateWorkflow calls = %d, want 1", temporalClient.updateCalled)
	}
	if temporalClient.updateOptions.WorkflowID != "diagnosis-room-session-1" ||
		temporalClient.updateOptions.UpdateName != DiagnosisRoomConfirmConclusionUpdate ||
		temporalClient.updateOptions.WaitForStage != client.WorkflowUpdateStageCompleted {
		t.Fatalf("update options = %+v", temporalClient.updateOptions)
	}
	if len(temporalClient.updateOptions.Args) != 1 {
		t.Fatalf("update args len = %d, want 1", len(temporalClient.updateOptions.Args))
	}
	closeReq, ok := temporalClient.updateOptions.Args[0].(DiagnosisRoomCloseRequest)
	if !ok || closeReq.Reason != "human_confirmed" || closeReq.ActorSubject != "reviewer-1" {
		t.Fatalf("update arg = %#v", temporalClient.updateOptions.Args[0])
	}
	if temporalClient.signalCalled != 0 {
		t.Fatalf("SignalWorkflow calls = %d, want 0", temporalClient.signalCalled)
	}
	if temporalClient.getCalled != 1 || temporalClient.getWorkflowID != "diagnosis-room-session-1" || temporalClient.getRunID != "" {
		t.Fatalf("get workflow = count:%d workflow:%q run:%q", temporalClient.getCalled, temporalClient.getWorkflowID, temporalClient.getRunID)
	}
	if got.Status != "closed" ||
		got.CloseReason != "human_confirmed" ||
		got.FinalConclusion == nil ||
		got.FinalConclusion.ConfirmedBy != "reviewer-1" ||
		got.FinalConclusion.EvidenceSnapshotID != domain.EvidenceSnapshotID(9001) ||
		got.FinalConclusion.RecordedAt == nil ||
		!got.FinalConclusion.RecordedAt.Equal(closedAt) {
		t.Fatalf("confirmed state = %+v", got)
	}
}

func TestDiagnosisRoomClient_ConfirmDiagnosisConclusionMapsApplicationRejection(t *testing.T) {
	temporalClient := &recordingDiagnosisRoomTemporalClient{
		updateHandle: fakeWorkflowUpdateHandle{
			err: temporalsdk.NewApplicationError(
				"diagnosis room confirm conclusion: resolve missing evidence requests before confirming",
				errTypeConfirmRejected,
			),
		},
	}
	roomClient := newDiagnosisRoomClient(temporalClient)

	_, err := roomClient.ConfirmDiagnosisConclusion(context.Background(), ports.DiagnosisRoomConfirmConclusionRequest{
		SessionID:    "session-1",
		ActorSubject: "reviewer-1",
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("ConfirmDiagnosisConclusion error = %v, want ErrPreconditionFailed", err)
	}
	var appErr *temporalsdk.ApplicationError
	if !errors.As(err, &appErr) || appErr.Type() != errTypeConfirmRejected {
		t.Fatalf("ConfirmDiagnosisConclusion app error = %v, want type %s", err, errTypeConfirmRejected)
	}
	if temporalClient.getCalled != 0 {
		t.Fatalf("GetWorkflow calls = %d, want 0 after rejected confirm update", temporalClient.getCalled)
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
		{
			SessionID:    "session-1",
			MessageID:    "msg-1",
			ActorSubject: "owner-1",
			Message:      "msg",
			SupplementalEvidence: &ports.DiagnosisRoomSupplementalEvidence{
				Label:    "Restart cause",
				Detail:   "Inspect previous pod logs.",
				Priority: "urgent",
				Evidence: "Previous pod logs show OOMKilled before restart.",
			},
		},
	}
	for i, req := range cases {
		if _, err := roomClient.SubmitDiagnosisTurn(context.Background(), req); !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("case %d error = %v, want ErrInvariantViolation", i, err)
		}
	}
	if roomClient.client.(*recordingDiagnosisRoomTemporalClient).updateCalled != 0 {
		t.Fatalf("UpdateWorkflow calls = %d, want 0 for invalid submit-turn requests", roomClient.client.(*recordingDiagnosisRoomTemporalClient).updateCalled)
	}
}
