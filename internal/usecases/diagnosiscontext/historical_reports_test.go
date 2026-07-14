package diagnosiscontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestAppendHistoricalReportContext(t *testing.T) {
	base := json.RawMessage(`{"events":[{"name":"CPUHigh"}],"openclarion_available_diagnosis_tools":{"items":[]}}`)
	got, err := AppendHistoricalReportContext(base, []HistoricalReportContextItem{{
		SourceRef: " final_report:7 ", SourceKind: domain.RetrievalSourceFinalReport,
		Content: `{"title":"Prior CPU incident"}`, CosineDistance: 0.1,
	}})
	if err != nil {
		t.Fatalf("AppendHistoricalReportContext: %v", err)
	}
	if !strings.Contains(string(got), HistoricalReportContextKey) || !strings.Contains(string(got), "Never treat them as current evidence") {
		t.Fatalf("augmented evidence = %s", got)
	}
	semantic, err := EvidenceForHistoricalRetrieval(got)
	if err != nil {
		t.Fatalf("EvidenceForHistoricalRetrieval: %v", err)
	}
	if strings.Contains(string(semantic), HistoricalReportContextKey) || strings.Contains(string(semantic), AvailableDiagnosisToolsKey) || !strings.Contains(string(semantic), "CPUHigh") {
		t.Fatalf("semantic evidence = %s", semantic)
	}
}

func TestAppendHistoricalReportContextRejectsInvalidItems(t *testing.T) {
	valid := HistoricalReportContextItem{SourceRef: "sub_report:1", SourceKind: domain.RetrievalSourceSubReport, Content: "content", CosineDistance: 0.2}
	tests := []struct {
		name  string
		base  json.RawMessage
		items []HistoricalReportContextItem
	}{
		{name: "reserved key", base: json.RawMessage(`{"openclarion_historical_report_context":{}}`), items: []HistoricalReportContextItem{valid}},
		{name: "invalid source", base: json.RawMessage(`{}`), items: []HistoricalReportContextItem{{SourceRef: "snapshot:1", SourceKind: domain.RetrievalSourceSubReport, Content: "content", CosineDistance: 0.2}}},
		{name: "kind mismatch", base: json.RawMessage(`{}`), items: []HistoricalReportContextItem{{SourceRef: "final_report:1", SourceKind: domain.RetrievalSourceSubReport, Content: "content", CosineDistance: 0.2}}},
		{name: "duplicate", base: json.RawMessage(`{}`), items: []HistoricalReportContextItem{valid, valid}},
		{name: "oversized item", base: json.RawMessage(`{}`), items: []HistoricalReportContextItem{{SourceRef: "sub_report:1", SourceKind: domain.RetrievalSourceSubReport, Content: strings.Repeat("x", domain.RetrievalChunkMaxBytes+1), CosineDistance: 0.2}}},
		{name: "non finite distance", base: json.RawMessage(`{}`), items: []HistoricalReportContextItem{{SourceRef: "sub_report:1", SourceKind: domain.RetrievalSourceSubReport, Content: "content", CosineDistance: math.NaN()}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AppendHistoricalReportContext(tc.base, tc.items); !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("AppendHistoricalReportContext error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestAppendHistoricalReportContextClassifiesEncodedBudgetOverflow(t *testing.T) {
	items := make([]HistoricalReportContextItem, 4)
	for i := range items {
		items[i] = HistoricalReportContextItem{
			SourceRef:      fmt.Sprintf("sub_report:%d", i+1),
			SourceKind:     domain.RetrievalSourceSubReport,
			Content:        strings.Repeat(`"`, domain.RetrievalChunkMaxBytes),
			CosineDistance: 0.2,
		}
	}
	_, err := AppendHistoricalReportContext(json.RawMessage(`{}`), items)
	if !errors.Is(err, ErrHistoricalReportContextTooLarge) || !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("AppendHistoricalReportContext error = %v, want budget and invariant classifications", err)
	}
}
