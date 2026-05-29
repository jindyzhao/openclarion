package fake

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const testInvocationID = "snapshot-11/group-0"

func TestNewDeepCopiesScripts(t *testing.T) {
	req := requestFor(testInvocationID)
	run := resultFor(req, `{"summary":"original"}`)
	scripts := map[string][]Result{
		testInvocationID: {{Run: run}},
	}
	p := New(scripts)

	scripts[testInvocationID][0].Run.Output[12] = 'X'
	scripts[testInvocationID] = append(scripts[testInvocationID], Result{Run: resultFor(req, `{"summary":"extra"}`)})

	got, err := p.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(got.Output) != `{"summary":"original"}` {
		t.Fatalf("Output = %s, want original", got.Output)
	}
}

func TestRunDeepCopiesReturnAndRecordedRequests(t *testing.T) {
	req := requestFor(testInvocationID)
	p := New(map[string][]Result{
		testInvocationID: {{Run: resultFor(req, `{"summary":"stable"}`)}},
	})

	first, err := p.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	first.Output[12] = 'X'

	recorded := p.Requests(testInvocationID)
	if len(recorded) != 1 {
		t.Fatalf("Requests len = %d, want 1", len(recorded))
	}
	recorded[0].Evidence[1] = 'X'
	secondRecorded := p.Requests(testInvocationID)
	if string(secondRecorded[0].Evidence) != string(req.Evidence) {
		t.Fatalf("recorded evidence mutated to %s", secondRecorded[0].Evidence)
	}

	second, err := p.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if string(second.Output) != `{"summary":"stable"}` {
		t.Fatalf("second Output = %s, want stable", second.Output)
	}
}

func TestRunScriptedByInvocationIDRepeatsLastResult(t *testing.T) {
	req := requestFor(testInvocationID)
	otherReq := requestFor("snapshot-11/group-1")
	p := New(map[string][]Result{
		testInvocationID: {
			{Run: resultFor(req, `{"summary":"first"}`)},
			{Run: resultFor(req, `{"summary":"second"}`)},
		},
		otherReq.InvocationID: {
			{Run: resultFor(otherReq, `{"summary":"other"}`)},
		},
	})

	ctx := context.Background()
	first, err := p.Run(ctx, req)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	second, err := p.Run(ctx, req)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	third, err := p.Run(ctx, req)
	if err != nil {
		t.Fatalf("third Run: %v", err)
	}
	other, err := p.Run(ctx, otherReq)
	if err != nil {
		t.Fatalf("other Run: %v", err)
	}

	if string(first.Output) != `{"summary":"first"}` {
		t.Fatalf("first Output = %s", first.Output)
	}
	if string(second.Output) != `{"summary":"second"}` {
		t.Fatalf("second Output = %s", second.Output)
	}
	if string(third.Output) != `{"summary":"second"}` {
		t.Fatalf("third Output = %s, want repeated last result", third.Output)
	}
	if string(other.Output) != `{"summary":"other"}` {
		t.Fatalf("other Output = %s", other.Output)
	}
	if p.Calls(testInvocationID) != 3 {
		t.Fatalf("Calls(%q) = %d, want 3", testInvocationID, p.Calls(testInvocationID))
	}
}

func TestRunReturnsScriptedError(t *testing.T) {
	req := requestFor(testInvocationID)
	wantErr := errors.New("sandbox unavailable")
	p := New(map[string][]Result{
		testInvocationID: {{Err: wantErr}},
	})

	_, err := p.Run(context.Background(), req)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run err = %v, want %v", err, wantErr)
	}
}

func TestRunRejectsMissingOrUnknownInvocationID(t *testing.T) {
	req := requestFor(testInvocationID)
	p := New(map[string][]Result{
		testInvocationID: {{Run: resultFor(req, `{"summary":"ok"}`)}},
	})

	missingID := req
	missingID.InvocationID = ""
	if _, err := p.Run(context.Background(), missingID); err == nil {
		t.Fatal("Run missing invocation id err = nil, want error")
	}

	unknown := requestFor("snapshot-11/group-99")
	if _, err := p.Run(context.Background(), unknown); err == nil {
		t.Fatal("Run unknown invocation id err = nil, want error")
	}
}

func TestRunRejectsInvalidScriptedResult(t *testing.T) {
	req := requestFor(testInvocationID)
	invalid := resultFor(req, `{"summary":"ok"}`)
	invalid.ExitCode = 1
	p := New(map[string][]Result{
		testInvocationID: {{Run: invalid}},
	})

	if _, err := p.Run(context.Background(), req); err == nil {
		t.Fatal("Run err = nil, want invalid scripted result error")
	}
}

func TestRunHonoursCancelledContext(t *testing.T) {
	req := requestFor(testInvocationID)
	p := New(map[string][]Result{
		testInvocationID: {{Run: resultFor(req, `{"summary":"ok"}`)}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Run(ctx, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err = %v, want context.Canceled", err)
	}
}

func requestFor(invocationID string) ports.ContainerRunRequest {
	return ports.ContainerRunRequest{
		InvocationID: invocationID,
		AgentName:    "report-enhancer",
		Evidence:     json.RawMessage(`{"snapshot_id":11,"alerts":[]}`),
		Timeout:      time.Minute,
		OutputMax:    1024,
		Metadata:     map[string]string{"scenario": "single_alert"},
	}
}

func resultFor(req ports.ContainerRunRequest, output string) ports.ContainerRunResult {
	startedAt := time.Date(2026, 5, 28, 6, 0, 0, 0, time.UTC)
	return ports.ContainerRunResult{
		InvocationID: req.InvocationID,
		AgentName:    req.AgentName,
		Output:       json.RawMessage(output),
		ExitCode:     0,
		StartedAt:    startedAt,
		FinishedAt:   startedAt.Add(time.Second),
		RuntimeID:    "fake-container",
	}
}
