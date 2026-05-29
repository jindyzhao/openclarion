package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewSubReport(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := NewSubReport(validSubReport())
		if err != nil {
			t.Fatalf("NewSubReport: %v", err)
		}
		if got.IdempotencyKey != "sub-key" || got.Title != "CPU saturation" {
			t.Fatalf("got = %+v", got)
		}
	})

	tests := []struct {
		name string
		edit func(*SubReport)
	}{
		{name: "zero snapshot", edit: func(r *SubReport) { r.EvidenceSnapshotID = 0 }},
		{name: "empty key", edit: func(r *SubReport) { r.IdempotencyKey = " " }},
		{name: "empty scenario", edit: func(r *SubReport) { r.Scenario = "" }},
		{name: "empty title", edit: func(r *SubReport) { r.Title = "" }},
		{name: "invalid severity", edit: func(r *SubReport) { r.Severity = ReportSeverity("page") }},
		{name: "invalid confidence", edit: func(r *SubReport) { r.Confidence = ReportConfidence("sure") }},
		{name: "invalid findings", edit: func(r *SubReport) { r.Findings = json.RawMessage(`{`) }},
		{name: "empty content", edit: func(r *SubReport) { r.Content = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validSubReport()
			tc.edit(&r)
			_, err := NewSubReport(r)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestNewFinalReport(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := NewFinalReport(validFinalReport())
		if err != nil {
			t.Fatalf("NewFinalReport: %v", err)
		}
		if got.CorrelationKey != "window-1" || got.NotificationText != "Scale payments." {
			t.Fatalf("got = %+v", got)
		}
	})

	tests := []struct {
		name string
		edit func(*FinalReport)
	}{
		{name: "empty correlation key", edit: func(r *FinalReport) { r.CorrelationKey = "" }},
		{name: "empty idempotency key", edit: func(r *FinalReport) { r.IdempotencyKey = "" }},
		{name: "empty title", edit: func(r *FinalReport) { r.Title = "" }},
		{name: "invalid severity", edit: func(r *FinalReport) { r.Severity = ReportSeverity("page") }},
		{name: "invalid confidence", edit: func(r *FinalReport) { r.Confidence = ReportConfidence("sure") }},
		{name: "invalid subreports", edit: func(r *FinalReport) { r.SubReports = json.RawMessage(`{`) }},
		{name: "empty notification", edit: func(r *FinalReport) { r.NotificationText = "" }},
		{name: "empty content", edit: func(r *FinalReport) { r.Content = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validFinalReport()
			tc.edit(&r)
			_, err := NewFinalReport(r)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestReportNotificationDeliveryLifecycle(t *testing.T) {
	pending, err := NewReportNotificationDelivery(42, " final_report:42/notification ")
	if err != nil {
		t.Fatalf("NewReportNotificationDelivery: %v", err)
	}
	if pending.Status != ReportNotificationDeliveryStatusPending {
		t.Fatalf("Status = %q, want pending", pending.Status)
	}
	if pending.IdempotencyKey != "final_report:42/notification" {
		t.Fatalf("IdempotencyKey = %q", pending.IdempotencyKey)
	}
	if string(pending.Raw) != "{}" {
		t.Fatalf("Raw = %s, want {}", pending.Raw)
	}

	deliveredAt := time.Date(2026, 5, 28, 11, 10, 9, 123456789, time.FixedZone("HKT", 8*60*60))
	delivered, err := pending.MarkDelivered(" msg-1 ", " accepted ", json.RawMessage(`{"message_id":"msg-1"}`), deliveredAt)
	if err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if delivered.Status != ReportNotificationDeliveryStatusDelivered || delivered.ProviderMessageID != "msg-1" || delivered.ProviderStatus != "accepted" {
		t.Fatalf("delivered = %+v", delivered)
	}
	wantDeliveredAt := NormalizeUTCMicro(deliveredAt)
	if delivered.DeliveredAt == nil || !delivered.DeliveredAt.Equal(wantDeliveredAt) {
		t.Fatalf("DeliveredAt = %v, want %s", delivered.DeliveredAt, wantDeliveredAt)
	}

	failed, err := delivered.MarkFailed(" webhook 500 ", nil)
	if err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if failed.Status != ReportNotificationDeliveryStatusFailed || failed.FailureReason != "webhook 500" || failed.DeliveredAt != nil {
		t.Fatalf("failed = %+v", failed)
	}
	if string(failed.Raw) != "{}" {
		t.Fatalf("failed.Raw = %s, want {}", failed.Raw)
	}
}

func TestReportNotificationDeliveryRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "zero final report id",
			call: func() error {
				_, err := NewReportNotificationDelivery(0, "key")
				return err
			},
		},
		{
			name: "empty key",
			call: func() error {
				_, err := NewReportNotificationDelivery(42, " ")
				return err
			},
		},
		{
			name: "delivered without time",
			call: func() error {
				d, err := NewReportNotificationDelivery(42, "key")
				if err != nil {
					return err
				}
				_, err = d.MarkDelivered("msg", "delivered", nil, time.Time{})
				return err
			},
		},
		{
			name: "delivered invalid raw",
			call: func() error {
				d, err := NewReportNotificationDelivery(42, "key")
				if err != nil {
					return err
				}
				_, err = d.MarkDelivered("msg", "delivered", json.RawMessage(`{`), time.Now())
				return err
			},
		},
		{
			name: "failed without reason",
			call: func() error {
				d, err := NewReportNotificationDelivery(42, "key")
				if err != nil {
					return err
				}
				_, err = d.MarkFailed(" ", nil)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func validSubReport() SubReport {
	return SubReport{
		EvidenceSnapshotID: 11,
		IdempotencyKey:     "sub-key",
		Scenario:           "single_alert",
		Title:              "CPU saturation",
		Summary:            "CPU is high.",
		Severity:           ReportSeverityWarning,
		Confidence:         ReportConfidenceHigh,
		Findings:           json.RawMessage(`[{"label":"CPU","detail":"high","evidence_id":"alert:1"}]`),
		RecommendedActions: json.RawMessage(`[{"label":"Scale","detail":"Add one replica","priority":"medium"}]`),
		EvidenceRefs:       []string{"alert:1"},
		Content:            json.RawMessage(`{"title":"CPU saturation"}`),
		Model:              "gpt-test",
		OutputMode:         "json_schema",
		CreatedByWorkflow:  "wf-1",
	}
}

func validFinalReport() FinalReport {
	return FinalReport{
		CorrelationKey:     "window-1",
		IdempotencyKey:     "final-key",
		Title:              "Payments degradation",
		ExecutiveSummary:   "Payments is degraded.",
		Severity:           ReportSeverityWarning,
		Confidence:         ReportConfidenceHigh,
		SubReports:         json.RawMessage(`[{"title":"CPU saturation","summary":"CPU is high.","severity":"warning"}]`),
		RecommendedActions: json.RawMessage(`[{"label":"Scale","detail":"Add one replica","priority":"medium"}]`),
		NotificationText:   "Scale payments.",
		Content:            json.RawMessage(`{"title":"Payments degradation"}`),
		Model:              "gpt-test",
		OutputMode:         "json_schema",
		CreatedByWorkflow:  "wf-1",
	}
}
