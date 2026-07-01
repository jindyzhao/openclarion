package temporal

import (
	"testing"
)

func TestReportActivityOptionsAllowSlowReportLLM(t *testing.T) {
	options := reportActivityOptions()
	if options.StartToCloseTimeout != reportActivityStartToCloseTimeout {
		t.Fatalf("StartToCloseTimeout = %s, want %s", options.StartToCloseTimeout, reportActivityStartToCloseTimeout)
	}
	if options.ScheduleToCloseTimeout != reportActivityScheduleToCloseTimeout {
		t.Fatalf("ScheduleToCloseTimeout = %s, want %s", options.ScheduleToCloseTimeout, reportActivityScheduleToCloseTimeout)
	}
	if options.RetryPolicy == nil || options.RetryPolicy.MaximumAttempts != 3 {
		t.Fatalf("RetryPolicy = %+v, want maximum attempts 3", options.RetryPolicy)
	}
	if reportChildWorkflowOptions("reports").WorkflowExecutionTimeout != reportChildWorkflowExecutionTimeout {
		t.Fatalf(
			"child WorkflowExecutionTimeout = %s, want %s",
			reportChildWorkflowOptions("reports").WorkflowExecutionTimeout,
			reportChildWorkflowExecutionTimeout,
		)
	}
}
