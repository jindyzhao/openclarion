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
	childTimeout := reportChildWorkflowOptions("reports").WorkflowExecutionTimeout
	if childTimeout != reportChildWorkflowExecutionTimeout {
		t.Fatalf(
			"child WorkflowExecutionTimeout = %s, want %s",
			childTimeout,
			reportChildWorkflowExecutionTimeout,
		)
	}
	activityBudget := reportActivityScheduleToCloseTimeout + 2*reportTaskActivityScheduleToCloseTimeout
	if childTimeout <= activityBudget || childTimeout-activityBudget != reportChildWorkflowCleanupHeadroom {
		t.Fatalf(
			"child WorkflowExecutionTimeout = %s, want %s activity budget plus %s cleanup headroom",
			childTimeout,
			activityBudget,
			reportChildWorkflowCleanupHeadroom,
		)
	}
}
