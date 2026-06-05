package temporal

import (
	"strings"
	"testing"
)

func TestNewWorkerWithTaskQueueRejectsEmptyQueue(t *testing.T) {
	_, err := NewWorkerWithTaskQueue(nil, nil, " ")
	if err == nil {
		t.Fatal("expected empty task queue error, got nil")
	}
	if !strings.Contains(err.Error(), "task queue") {
		t.Fatalf("error = %q, want task queue", err.Error())
	}
}

func TestReportChildWorkflowOptionsUseProvidedTaskQueue(t *testing.T) {
	opts := reportChildWorkflowOptions(" openclarion-local-rehearsal ")
	if opts.TaskQueue != "openclarion-local-rehearsal" {
		t.Fatalf("TaskQueue = %q, want custom queue", opts.TaskQueue)
	}
}
