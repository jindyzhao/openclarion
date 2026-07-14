package diagnosiscontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
)

const (
	// HistoricalReportContextKey is server-owned advisory context. Runners may
	// use it to form hypotheses but must corroborate against current evidence.
	HistoricalReportContextKey = "openclarion_historical_report_context"

	historicalReportContextUsage = "These are persisted historical reports and may be stale. Use them only to form hypotheses. " +
		"Never treat them as current evidence, never cite source_ref as an evidence identifier, and corroborate every claim with other current evidence in this document."
)

// ErrHistoricalReportContextTooLarge identifies a valid advisory catalog that
// must be retried with a smaller content budget.
var ErrHistoricalReportContextTooLarge = errors.New("diagnosis context: historical report context is too large")

// HistoricalReportContextItem is one bounded report projection mounted for a
// diagnosis turn.
type HistoricalReportContextItem struct {
	SourceRef      string                     `json:"source_ref"`
	SourceKind     domain.RetrievalSourceKind `json:"source_kind"`
	Content        string                     `json:"content"`
	CosineDistance float64                    `json:"cosine_distance"`
}

type historicalReportContext struct {
	Usage string                        `json:"usage"`
	Items []HistoricalReportContextItem `json:"items"`
}

// ValidateHistoricalReportContextAbsent rejects caller-supplied data in the
// server-owned historical context slot.
func ValidateHistoricalReportContextAbsent(base json.RawMessage) error {
	top, err := decodeEvidenceObject(base)
	if err != nil {
		return err
	}
	return rejectHistoricalReportContext(top)
}

// AppendHistoricalReportContext adds validated server-owned advisory context.
// Existing reserved data is rejected instead of overwritten.
func AppendHistoricalReportContext(base json.RawMessage, items []HistoricalReportContextItem) (json.RawMessage, error) {
	if len(items) == 0 {
		return cloneRawMessage(base), nil
	}
	if len(items) > domain.RetrievalReferenceLimit {
		return nil, fmt.Errorf("diagnosis context: historical reports exceed %d items: %w", domain.RetrievalReferenceLimit, domain.ErrInvariantViolation)
	}
	top, err := decodeEvidenceObject(base)
	if err != nil {
		return nil, err
	}
	if err := rejectHistoricalReportContext(top); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(items))
	validated := make([]HistoricalReportContextItem, len(items))
	totalContentBytes := 0
	for i, item := range items {
		item.SourceRef = strings.TrimSpace(item.SourceRef)
		kind, _, err := domain.ParseRetrievalSourceRef(item.SourceRef)
		if err != nil || kind != item.SourceKind {
			return nil, fmt.Errorf("diagnosis context: historical report[%d] source is invalid: %w", i, domain.ErrInvariantViolation)
		}
		if _, duplicate := seen[item.SourceRef]; duplicate {
			return nil, fmt.Errorf("diagnosis context: historical report[%d] duplicates %q: %w", i, item.SourceRef, domain.ErrInvariantViolation)
		}
		seen[item.SourceRef] = struct{}{}
		item.Content = strings.TrimSpace(item.Content)
		contentBytes := len([]byte(item.Content))
		totalContentBytes += contentBytes
		if item.Content == "" || contentBytes > domain.RetrievalChunkMaxBytes {
			return nil, fmt.Errorf("diagnosis context: historical report content must be non-empty and bounded: %w", domain.ErrInvariantViolation)
		}
		if totalContentBytes > domain.RetrievalContextMaxBytes {
			return nil, fmt.Errorf("diagnosis context: historical report content exceeds %d bytes: %w: %w", domain.RetrievalContextMaxBytes, ErrHistoricalReportContextTooLarge, domain.ErrInvariantViolation)
		}
		if math.IsNaN(item.CosineDistance) || math.IsInf(item.CosineDistance, 0) || item.CosineDistance < 0 || item.CosineDistance > 2 {
			return nil, fmt.Errorf("diagnosis context: historical report[%d] distance is invalid: %w", i, domain.ErrInvariantViolation)
		}
		validated[i] = item
	}
	catalog, err := json.Marshal(historicalReportContext{Usage: historicalReportContextUsage, Items: validated})
	if err != nil {
		return nil, fmt.Errorf("diagnosis context: marshal historical reports: %w", err)
	}
	if len(catalog) > domain.RetrievalContextMaxBytes {
		return nil, fmt.Errorf("diagnosis context: encoded historical reports exceed %d bytes: %w: %w", domain.RetrievalContextMaxBytes, ErrHistoricalReportContextTooLarge, domain.ErrInvariantViolation)
	}
	top[HistoricalReportContextKey] = catalog
	out, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("diagnosis context: marshal evidence: %w", err)
	}
	if _, err := decodeEvidenceObject(out); err != nil {
		return nil, err
	}
	return out, nil
}

func rejectHistoricalReportContext(top map[string]json.RawMessage) error {
	if _, exists := top[HistoricalReportContextKey]; exists {
		return fmt.Errorf("diagnosis context: evidence already contains reserved key %q: %w", HistoricalReportContextKey, domain.ErrInvariantViolation)
	}
	return nil
}

// EvidenceForHistoricalRetrieval removes runtime instruction catalogs before
// embedding current evidence, keeping similarity focused on incident facts.
func EvidenceForHistoricalRetrieval(base json.RawMessage) (json.RawMessage, error) {
	top, err := decodeEvidenceObject(base)
	if err != nil {
		return nil, err
	}
	delete(top, AvailableDiagnosisToolsKey)
	delete(top, HistoricalReportContextKey)
	out, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("diagnosis context: marshal retrieval evidence: %w", err)
	}
	return out, nil
}
