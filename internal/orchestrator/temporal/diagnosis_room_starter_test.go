package temporal

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type recordingDiagnosisRoomStarterClient struct {
	executeCalled int
	options       client.StartWorkflowOptions
	workflow      interface{}
	args          []interface{}
	run           client.WorkflowRun
	executeErr    error

	queryCalled     int
	queryWorkflowID string
	queryRunID      string
	queryType       string
	queryValue      converter.EncodedValue
	queryErr        error
}

func (c *recordingDiagnosisRoomStarterClient) ExecuteWorkflow(_ context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	c.executeCalled++
	c.options = options
	c.workflow = workflow
	c.args = append([]interface{}(nil), args...)
	if c.executeErr != nil {
		return nil, c.executeErr
	}
	return c.run, nil
}

func (c *recordingDiagnosisRoomStarterClient) QueryWorkflow(_ context.Context, workflowID string, runID string, queryType string, _ ...interface{}) (converter.EncodedValue, error) {
	c.queryCalled++
	c.queryWorkflowID = workflowID
	c.queryRunID = runID
	c.queryType = queryType
	if c.queryErr != nil {
		return nil, c.queryErr
	}
	return c.queryValue, nil
}

func TestDiagnosisRoomStarter_StartDiagnosisRoomStartsWorkflowAndWaitsReady(t *testing.T) {
	client := &recordingDiagnosisRoomStarterClient{
		run: staticWorkflowRun{workflowID: "diagnosis-room-session-1", runID: "run-1"},
		queryValue: fakeEncodedValue{value: DiagnosisRoomWorkflowState{
			SessionID:       "session-1",
			DiagnosisTaskID: 101,
			ChatSessionID:   202,
		}},
	}
	starter := newDiagnosisRoomStarter(
		client,
		WithDiagnosisRoomStarterTaskQueue("diagnosis"),
		WithDiagnosisRoomStarterReadyTimeout(time.Second),
	)

	got, err := starter.StartDiagnosisRoom(context.Background(), ports.DiagnosisRoomStartRequest{
		SessionID:                         "session-1",
		EvidenceSnapshotID:                42,
		OwnerSubject:                      "owner-1",
		Evidence:                          []byte(`{"alert":"cpu"}`),
		CloseNotificationChannelProfileID: 9,
		InitialTurn: &ports.DiagnosisRoomInitialTurnRequest{
			MessageID:    "initial-1",
			ActorSubject: "openclarion.alertmanager-webhook:7:policy:3",
			Message:      "Generate the initial diagnosis.",
		},
	})
	if err != nil {
		t.Fatalf("StartDiagnosisRoom: %v", err)
	}
	if got.SessionID != "session-1" ||
		got.EvidenceSnapshotID != 42 ||
		got.DiagnosisTaskID != 101 ||
		got.ChatSessionID != 202 ||
		got.Workflow.WorkflowID != "diagnosis-room-session-1" ||
		got.Workflow.RunID != "run-1" {
		t.Fatalf("result = %+v", got)
	}
	if client.executeCalled != 1 {
		t.Fatalf("ExecuteWorkflow calls = %d, want 1", client.executeCalled)
	}
	if client.options.ID != "diagnosis-room-session-1" || client.options.TaskQueue != "diagnosis" {
		t.Fatalf("options = %+v", client.options)
	}
	if reflect.ValueOf(client.workflow).Pointer() != reflect.ValueOf(DiagnosisRoomWorkflow).Pointer() {
		t.Fatalf("workflow = %T, want DiagnosisRoomWorkflow", client.workflow)
	}
	if len(client.args) != 1 {
		t.Fatalf("args len = %d, want 1", len(client.args))
	}
	input, ok := client.args[0].(DiagnosisRoomWorkflowInput)
	if !ok {
		t.Fatalf("arg[0] = %T, want DiagnosisRoomWorkflowInput", client.args[0])
	}
	if input.SessionID != "session-1" ||
		input.EvidenceSnapshotID != 42 ||
		input.OwnerSubject != "owner-1" ||
		input.CloseNotificationChannelProfileID != 9 {
		t.Fatalf("input = %+v", input)
	}
	if input.InitialTurn == nil ||
		input.InitialTurn.MessageID != "initial-1" ||
		input.InitialTurn.ActorSubject != "openclarion.alertmanager-webhook:7:policy:3" ||
		input.InitialTurn.Message != "Generate the initial diagnosis." {
		t.Fatalf("initial turn input = %+v", input.InitialTurn)
	}
	if client.queryCalled == 0 || client.queryWorkflowID != "diagnosis-room-session-1" || client.queryRunID != "run-1" || client.queryType != DiagnosisRoomStateQuery {
		t.Fatalf("query calls=%d workflow=%q run=%q type=%q", client.queryCalled, client.queryWorkflowID, client.queryRunID, client.queryType)
	}
}

func TestDiagnosisRoomStarter_StartDiagnosisRoomValidation(t *testing.T) {
	good := ports.DiagnosisRoomStartRequest{
		SessionID:          "session-1",
		EvidenceSnapshotID: 42,
		OwnerSubject:       "owner-1",
		Evidence:           []byte(`{"alert":"cpu"}`),
	}
	cases := []struct {
		name       string
		starter    *DiagnosisRoomStarter
		req        ports.DiagnosisRoomStartRequest
		wantSubstr string
	}{
		{name: "nil_starter", starter: nil, req: good},
		{name: "nil_client", starter: &DiagnosisRoomStarter{}, req: good},
		{name: "empty_session", starter: newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}), req: withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) { req.SessionID = "" })},
		{name: "trimmed_session", starter: newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}), req: withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) { req.SessionID = " session-1 " })},
		{name: "zero_snapshot", starter: newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}), req: withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) { req.EvidenceSnapshotID = 0 })},
		{name: "negative_close_channel", starter: newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}), req: withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) {
			req.CloseNotificationChannelProfileID = -1
		})},
		{name: "empty_owner", starter: newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}), req: withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) { req.OwnerSubject = " " })},
		{
			name:       "invalid_evidence",
			starter:    newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}),
			req:        withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) { req.Evidence = []byte(`not-json`) }),
			wantSubstr: "decode JSON token",
		},
		{
			name:       "duplicate_evidence_key",
			starter:    newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}),
			req:        withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) { req.Evidence = []byte(`{"alert":"cpu","alert":"memory"}`) }),
			wantSubstr: `duplicate object key "alert"`,
		},
		{
			name:    "trailing_evidence_value",
			starter: newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}),
			req: withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) {
				req.Evidence = []byte(`{"alert":"cpu"} {"alert":"memory"}`)
			}),
			wantSubstr: "trailing JSON values",
		},
		{
			name:       "non_object_evidence",
			starter:    newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}),
			req:        withStartRequest(good, func(req *ports.DiagnosisRoomStartRequest) { req.Evidence = []byte(`["cpu"]`) }),
			wantSubstr: "must be a JSON object",
		},
		{name: "empty_task_queue", starter: newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}, WithDiagnosisRoomStarterTaskQueue(" ")), req: good},
		{name: "bad_ready_timeout", starter: newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{}, WithDiagnosisRoomStarterReadyTimeout(0)), req: good},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.starter.StartDiagnosisRoom(context.Background(), tc.req)
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("StartDiagnosisRoom error = %v, want ErrInvariantViolation", err)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("StartDiagnosisRoom error = %v, want substring %q", err, tc.wantSubstr)
			}
		})
	}
}

func TestDiagnosisRoomStarter_PropagatesExecuteWorkflowError(t *testing.T) {
	wantErr := errors.New("temporal unavailable")
	starter := newDiagnosisRoomStarter(&recordingDiagnosisRoomStarterClient{executeErr: wantErr})

	_, err := starter.StartDiagnosisRoom(context.Background(), ports.DiagnosisRoomStartRequest{
		SessionID:          "session-1",
		EvidenceSnapshotID: 42,
		OwnerSubject:       "owner-1",
		Evidence:           []byte(`{"alert":"cpu"}`),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("StartDiagnosisRoom error = %v, want %v", err, wantErr)
	}
}

func TestDiagnosisRoomStarter_WaitReadyTimesOut(t *testing.T) {
	starter := newDiagnosisRoomStarter(
		&recordingDiagnosisRoomStarterClient{
			run:        staticWorkflowRun{workflowID: "diagnosis-room-session-1", runID: "run-1"},
			queryValue: fakeEncodedValue{value: DiagnosisRoomWorkflowState{SessionID: "session-1"}},
		},
		WithDiagnosisRoomStarterReadyTimeout(time.Millisecond),
	)
	starter.readyPollInterval = time.Millisecond

	_, err := starter.StartDiagnosisRoom(context.Background(), ports.DiagnosisRoomStartRequest{
		SessionID:          "session-1",
		EvidenceSnapshotID: 42,
		OwnerSubject:       "owner-1",
		Evidence:           []byte(`{"alert":"cpu"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "wait for room readiness") {
		t.Fatalf("StartDiagnosisRoom error = %v, want readiness timeout", err)
	}
}

func withStartRequest(req ports.DiagnosisRoomStartRequest, mutate func(*ports.DiagnosisRoomStartRequest)) ports.DiagnosisRoomStartRequest {
	mutate(&req)
	return req
}
