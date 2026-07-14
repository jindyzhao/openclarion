package temporal

import (
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
)

func TestDiagnosisRoomTurnActivityOptionsGateStreamingHeartbeat(t *testing.T) {
	policy := diagnosisroom.DefaultPolicy()
	policy.TurnTimeout = 20 * time.Second

	withoutStreaming := diagnosisRoomTurnActivityOptions(policy, false)
	if withoutStreaming.HeartbeatTimeout != 0 {
		t.Fatalf("heartbeat without streaming = %s", withoutStreaming.HeartbeatTimeout)
	}

	withStreaming := diagnosisRoomTurnActivityOptions(policy, true)
	if withStreaming.StartToCloseTimeout != 20*time.Second || withStreaming.HeartbeatTimeout != 10*time.Second {
		t.Fatalf("streaming options = %+v", withStreaming)
	}

	policy.TurnTimeout = 2 * time.Minute
	if got := diagnosisRoomTurnActivityOptions(policy, true).HeartbeatTimeout; got != 30*time.Second {
		t.Fatalf("capped heartbeat = %s, want 30s", got)
	}
}
