package fake

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const testNotificationKey = "final_report:42/notification"

func notificationFor(key string) ports.IMNotification {
	return ports.IMNotification{
		IdempotencyKey: key,
		FinalReportID:  42,
		CorrelationKey: "window-1",
		Title:          "Payments degradation",
		Body:           "Scale payments.",
		Severity:       "warning",
	}
}

func deliveryFor(raw string) ports.IMDelivery {
	return ports.IMDelivery{
		ProviderMessageID: "msg-1",
		Status:            "delivered",
		Raw:               json.RawMessage(raw),
	}
}

func TestNew_DeepCopiesScript(t *testing.T) {
	scripts := map[string][]Result{
		testNotificationKey: {{Delivery: deliveryFor(`{"ok":true}`)}},
	}
	p := New(scripts)
	scripts[testNotificationKey][0].Delivery.Raw[6] = 'X'
	scripts[testNotificationKey][0].Delivery.ProviderMessageID = "mutated"

	got, err := p.SendNotification(context.Background(), notificationFor(testNotificationKey))
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if string(got.Raw) != `{"ok":true}` {
		t.Fatalf("Raw = %s, want original", got.Raw)
	}
	if got.ProviderMessageID != "msg-1" {
		t.Fatalf("ProviderMessageID = %q, want msg-1", got.ProviderMessageID)
	}
}

func TestSendNotification_DeepCopiesReturnAndRecordsRequests(t *testing.T) {
	p := New(map[string][]Result{
		testNotificationKey: {{Delivery: deliveryFor(`{"stable":true}`)}},
	})

	first, err := p.SendNotification(context.Background(), notificationFor(testNotificationKey))
	if err != nil {
		t.Fatalf("first SendNotification: %v", err)
	}
	first.Raw[11] = 'X'

	second, err := p.SendNotification(context.Background(), notificationFor(testNotificationKey))
	if err != nil {
		t.Fatalf("second SendNotification: %v", err)
	}
	if string(second.Raw) != `{"stable":true}` {
		t.Fatalf("second Raw = %s, want stable original", second.Raw)
	}
	if p.Calls(testNotificationKey) != 2 {
		t.Fatalf("Calls = %d, want 2", p.Calls(testNotificationKey))
	}
	requests := p.Requests(testNotificationKey)
	if len(requests) != 2 {
		t.Fatalf("Requests len = %d, want 2", len(requests))
	}
	requests[0].Title = "mutated"
	if p.Requests(testNotificationKey)[0].Title != "Payments degradation" {
		t.Fatalf("Requests return was not isolated")
	}
}

func TestSendNotification_RepeatsLastScriptResult(t *testing.T) {
	p := New(map[string][]Result{
		testNotificationKey: {
			{Delivery: ports.IMDelivery{ProviderMessageID: "msg-1", Status: "accepted"}},
			{Delivery: ports.IMDelivery{ProviderMessageID: "msg-2", Status: "delivered"}},
		},
	})

	for i, want := range []string{"msg-1", "msg-2", "msg-2"} {
		got, err := p.SendNotification(context.Background(), notificationFor(testNotificationKey))
		if err != nil {
			t.Fatalf("SendNotification %d: %v", i+1, err)
		}
		if got.ProviderMessageID != want {
			t.Fatalf("SendNotification %d ProviderMessageID = %q, want %q", i+1, got.ProviderMessageID, want)
		}
	}
}

func TestSendNotification_Errors(t *testing.T) {
	sentinel := errors.New("boom")
	p := New(map[string][]Result{
		testNotificationKey: {{Err: sentinel}},
	})
	if _, err := p.SendNotification(context.Background(), ports.IMNotification{}); err == nil {
		t.Fatalf("empty idempotency key: want error, got nil")
	}
	if _, err := p.SendNotification(context.Background(), notificationFor("missing")); err == nil {
		t.Fatalf("missing script: want error, got nil")
	}
	_, err := p.SendNotification(context.Background(), notificationFor(testNotificationKey))
	if !errors.Is(err, sentinel) {
		t.Fatalf("scripted error = %v, want sentinel", err)
	}
}
