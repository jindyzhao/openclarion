package diagnosisstream

import (
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestHubKeepsNewestSnapshotWithoutBlockingPublisher(t *testing.T) {
	hub := NewHub()
	events, cancel := hub.SubscribeDiagnosisTurnStream("session-1", "message-1")
	defer cancel()

	for sequence := 1; sequence <= 100; sequence++ {
		hub.PublishDiagnosisTurnStream(ports.DiagnosisTurnStreamEvent{
			Phase:            ports.DiagnosisTurnStreamDelta,
			SessionID:        "session-1",
			MessageID:        "message-1",
			Sequence:         sequence,
			AssistantMessage: "latest",
		})
	}

	event := <-events
	if event.Sequence != 100 || event.AssistantMessage != "latest" {
		t.Fatalf("event = %#v, want newest snapshot", event)
	}
}

func TestHubScopesSubscriptionsAndCancelClosesChannel(t *testing.T) {
	hub := NewHub()
	events, cancel := hub.SubscribeDiagnosisTurnStream("session-1", "message-1")
	hub.PublishDiagnosisTurnStream(ports.DiagnosisTurnStreamEvent{
		SessionID: "session-1",
		MessageID: "other-message",
		Sequence:  1,
	})
	select {
	case event := <-events:
		t.Fatalf("received mismatched event: %#v", event)
	default:
	}

	cancel()
	if _, ok := <-events; ok {
		t.Fatal("events channel remained open after cancel")
	}
}

func TestHubRejectsEmptySubscriptionKeys(t *testing.T) {
	events, cancel := NewHub().SubscribeDiagnosisTurnStream(" ", "message-1")
	defer cancel()
	if _, ok := <-events; ok {
		t.Fatal("empty-key subscription remained open")
	}
}
