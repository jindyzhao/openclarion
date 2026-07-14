package http

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisstream"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestDiagnosisWebSocketRelayDoesNotWaitForUpdateAfterPreviewWriteFailure(t *testing.T) {
	hub := diagnosisstream.NewHub()
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		submitStarted: make(chan struct{}),
		releaseSubmit: make(chan struct{}),
		submitDone:    make(chan struct{}),
	}
	relay := newDiagnosisWebSocketRelay(
		workflowClient,
		WithDiagnosisTurnStreamSource(hub),
		WithDiagnosisWebSocketUpdateTimeout(5*time.Second),
	)
	wantErr := errors.New("preview write failed")
	relay.writeTurnPreview = func(*websocket.Conn, diagnosisWSTurnStreamFrame) error {
		return wantErr
	}

	done := make(chan error, 1)
	go func() {
		done <- relay.handleSubmitTurn(
			context.Background(),
			nil,
			diagnosisauth.Ticket{SessionID: "session-1", Subject: "owner-1"},
			diagnosisWSClientFrame{MessageID: "message-1", Message: "Investigate."},
		)
	}()
	select {
	case <-workflowClient.submitStarted:
	case <-time.After(time.Second):
		t.Fatal("SubmitDiagnosisTurn did not start")
	}
	hub.PublishDiagnosisTurnStream(ports.DiagnosisTurnStreamEvent{
		Phase:              ports.DiagnosisTurnStreamDelta,
		SessionID:          "session-1",
		MessageID:          "message-1",
		AssistantMessageID: "message-1/assistant",
		ActivityAttempt:    1,
		GenerationAttempt:  1,
		Sequence:           1,
		AssistantMessage:   "Draft",
	})
	select {
	case err := <-done:
		if !errors.Is(err, wantErr) {
			t.Fatalf("handleSubmitTurn error = %v, want %v", err, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("preview write failure waited for the detached Update")
	}
	select {
	case <-workflowClient.submitDone:
		t.Fatal("detached Update completed before release")
	default:
	}

	close(workflowClient.releaseSubmit)
	select {
	case <-workflowClient.submitDone:
	case <-time.After(time.Second):
		t.Fatal("detached Update did not finish after release")
	}
	if err := workflowClient.submitContextErrSnapshot(); err != nil {
		t.Fatalf("submit context err = %v, want nil", err)
	}
}

func TestDiagnosisWSTurnStreamFrameFromEventAcceptsReset(t *testing.T) {
	event := ports.DiagnosisTurnStreamEvent{
		Phase:              ports.DiagnosisTurnStreamReset,
		SessionID:          "session-1",
		MessageID:          "message-1",
		AssistantMessageID: "message-1/assistant",
		ActivityAttempt:    1,
		GenerationAttempt:  2,
	}
	frame, ok := diagnosisWSTurnStreamFrameFromEvent("session-1", "message-1", event)
	if !ok || frame.Phase != ports.DiagnosisTurnStreamReset || frame.GenerationAttempt != 2 || frame.Sequence != 0 || frame.AssistantMessage != "" {
		t.Fatalf("reset frame = %+v accepted=%t", frame, ok)
	}

	event.Sequence = 1
	if _, ok := diagnosisWSTurnStreamFrameFromEvent("session-1", "message-1", event); ok {
		t.Fatal("reset frame with non-zero sequence was accepted")
	}
}
