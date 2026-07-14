package domain

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
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
		{name: "duplicate findings key", edit: func(r *SubReport) { r.Findings = json.RawMessage(`[{"label":"CPU","label":"Memory"}]`) }},
		{name: "duplicate recommended action key", edit: func(r *SubReport) { r.RecommendedActions = json.RawMessage(`[{"label":"Scale","label":"Restart"}]`) }},
		{name: "duplicate content key", edit: func(r *SubReport) { r.Content = json.RawMessage(`{"title":"old","title":"new"}`) }},
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

func TestRetrievalSourceRefAndChunkValidation(t *testing.T) {
	kind, id, err := ParseRetrievalSourceRef("sub_report:42")
	if err != nil || kind != RetrievalSourceSubReport || id != 42 {
		t.Fatalf("ParseRetrievalSourceRef = %q/%d/%v", kind, id, err)
	}
	for _, ref := range []string{"", " sub_report:42", "sub_report:0", "sub_report:042", "unknown:1", "final_report:not-a-number"} {
		if _, _, err := ParseRetrievalSourceRef(ref); !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("ParseRetrievalSourceRef(%q) error = %v, want ErrInvariantViolation", ref, err)
		}
	}

	embedding := make([]float32, RetrievalEmbeddingDimensions)
	embedding[0] = 1
	metadata := json.RawMessage(`{"scenario":"single_alert"}`)
	chunk, err := NewRetrievalChunk(RetrievalChunk{
		SourceKind:          RetrievalSourceSubReport,
		SourceID:            42,
		SourceRef:           "sub_report:42",
		Content:             `{"title":"CPU"}`,
		EmbeddingModel:      "embed-model",
		EmbeddingDimensions: RetrievalEmbeddingDimensions,
		Embedding:           embedding,
		Metadata:            metadata,
	})
	if err != nil {
		t.Fatalf("NewRetrievalChunk: %v", err)
	}
	embedding[0] = 9
	metadata[2] = 'X'
	if chunk.Embedding[0] != 1 || string(chunk.Metadata) != `{"scenario":"single_alert"}` || len(chunk.ContentDigest) != 64 {
		t.Fatalf("chunk was polluted or incomplete: %+v", chunk)
	}

	bad := chunk
	bad.ID = 0
	bad.SourceRef = "final_report:42"
	if _, err := NewRetrievalChunk(bad); !errors.Is(err, ErrInvariantViolation) {
		t.Fatalf("mismatched source ref error = %v", err)
	}

	for name, edit := range map[string]func(*RetrievalChunk){
		"oversized content": func(c *RetrievalChunk) { c.Content = strings.Repeat("x", RetrievalChunkMaxBytes+1) },
		"oversized metadata": func(c *RetrievalChunk) {
			c.Metadata = json.RawMessage(`{"value":"` + strings.Repeat("x", RetrievalMetadataMaxBytes) + `"}`)
		},
		"duplicate metadata key": func(c *RetrievalChunk) { c.Metadata = json.RawMessage(`{"scenario":"one","scenario":"two"}`) },
		"zero embedding":         func(c *RetrievalChunk) { c.Embedding = make([]float32, RetrievalEmbeddingDimensions) },
		"non-finite embedding":   func(c *RetrievalChunk) { c.Embedding[0] = float32(math.NaN()) },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := chunk
			invalid.Embedding = append([]float32(nil), chunk.Embedding...)
			edit(&invalid)
			if _, err := NewRetrievalChunk(invalid); !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("NewRetrievalChunk error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestNewSubReportRetrievalRefs(t *testing.T) {
	report := validSubReport()
	refs := []string{" sub_report:7 ", "final_report:9"}
	report.RetrievalRefs = refs
	got, err := NewSubReport(report)
	if err != nil {
		t.Fatalf("NewSubReport: %v", err)
	}
	refs[0] = "final_report:99"
	if got.RetrievalRefs[0] != "sub_report:7" {
		t.Fatalf("retrieval refs = %v, want normalized defensive copy", got.RetrievalRefs)
	}

	for _, refs := range [][]string{
		{"sub_report:1", "sub_report:1"},
		{"snapshot:1"},
		make([]string, RetrievalReferenceLimit+1),
	} {
		report := validSubReport()
		report.RetrievalRefs = refs
		if _, err := NewSubReport(report); !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("retrieval refs %v error = %v, want ErrInvariantViolation", refs, err)
		}
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
		{name: "duplicate subreports key", edit: func(r *FinalReport) { r.SubReports = json.RawMessage(`[{"title":"old","title":"new"}]`) }},
		{name: "duplicate recommended action key", edit: func(r *FinalReport) { r.RecommendedActions = json.RawMessage(`[{"label":"Scale","label":"Restart"}]`) }},
		{name: "duplicate content key", edit: func(r *FinalReport) { r.Content = json.RawMessage(`{"title":"old","title":"new"}`) }},
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
	pending.ReportNotificationChannelProfileID = 7

	deliveredAt := time.Date(2026, 5, 28, 11, 10, 9, 123456789, time.FixedZone("HKT", 8*60*60))
	delivered, err := pending.MarkDelivered(" msg-1 ", " accepted ", json.RawMessage(`{"message_id":"msg-1"}`), deliveredAt)
	if err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if delivered.Status != ReportNotificationDeliveryStatusDelivered || delivered.ProviderMessageID != "msg-1" || delivered.ProviderStatus != "accepted" {
		t.Fatalf("delivered = %+v", delivered)
	}
	if delivered.ReportNotificationChannelProfileID != 7 {
		t.Fatalf("delivered channel profile id = %d, want 7", delivered.ReportNotificationChannelProfileID)
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
			name: "negative notification channel profile id",
			call: func() error {
				d, err := NewReportNotificationDelivery(42, "key")
				if err != nil {
					return err
				}
				d.ReportNotificationChannelProfileID = -1
				_, err = d.MarkDelivered("msg", "delivered", nil, time.Now())
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
			name: "delivered duplicate raw key",
			call: func() error {
				d, err := NewReportNotificationDelivery(42, "key")
				if err != nil {
					return err
				}
				_, err = d.MarkDelivered("msg", "delivered", json.RawMessage(`{"message_id":"old","message_id":"new"}`), time.Now())
				return err
			},
		},
		{
			name: "failed duplicate raw key",
			call: func() error {
				d, err := NewReportNotificationDelivery(42, "key")
				if err != nil {
					return err
				}
				_, err = d.MarkFailed("webhook 500", json.RawMessage(`{"status":"old","status":"new"}`))
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

func TestReportRetainedJSONRejectsTrailingValues(t *testing.T) {
	report := validSubReport()
	report.Content = json.RawMessage(`{"title":"CPU saturation"} {"title":"again"}`)

	_, err := NewSubReport(report)
	if !errors.Is(err, ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
	if !strings.Contains(err.Error(), "trailing JSON values") {
		t.Fatalf("err = %q, want trailing JSON values", err.Error())
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
