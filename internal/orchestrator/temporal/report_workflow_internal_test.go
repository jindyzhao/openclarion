package temporal

import (
	"testing"
	"time"
)

func TestReportActivityOptionsAllowSlowReportLLM(t *testing.T) {
	options := reportActivityOptions()
	if options.StartToCloseTimeout != 5*time.Minute {
		t.Fatalf("StartToCloseTimeout = %s, want 5m", options.StartToCloseTimeout)
	}
}
