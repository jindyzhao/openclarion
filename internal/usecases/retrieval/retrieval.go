// Package retrieval owns semantic indexing and bounded historical report
// context retrieval. Accepted reports remain the audit source of truth.
package retrieval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// DefaultLimit is the number of historical chunks returned by default.
	DefaultLimit = 4
	// MaxLimit bounds one historical nearest-neighbor result set.
	MaxLimit       = 10
	maxSearchLimit = MaxLimit * 2
	// DefaultMaxCosineDistance rejects weak semantic matches by default.
	DefaultMaxCosineDistance = 0.35
	// DefaultContextBytes bounds historical prompt context by default.
	DefaultContextBytes = 16 * 1024
	// MaxContextBytes is the hard historical prompt-context byte limit.
	MaxContextBytes = domain.RetrievalContextMaxBytes
	// MaxReindexReports bounds one corpus reindex operation.
	MaxReindexReports    = 10_000
	maxReindexSubReports = 10_000
)

// Query configures one bounded nearest-neighbor lookup.
type Query struct {
	Text              string
	IdempotencyKey    string
	Limit             int
	MaxCosineDistance float64
	ContextBytes      int
	ExcludeSourceRefs []string
}

// ContextItem is one sanitized historical report projection supplied to a
// prompt or sandbox as advisory context, never as primary incident evidence.
type ContextItem struct {
	SourceRef      string                     `json:"source_ref"`
	SourceKind     domain.RetrievalSourceKind `json:"source_kind"`
	Content        string                     `json:"content"`
	CosineDistance float64                    `json:"cosine_distance"`
}

// ReindexStats summarizes one bounded historical corpus backfill.
type ReindexStats struct {
	FinalReportsLoaded    int `json:"final_reports_loaded"`
	FinalReportsProcessed int `json:"final_reports_processed"`
	SubReportsProcessed   int `json:"sub_reports_processed"`
	Failed                int `json:"failed"`
}

// ReindexReports idempotently indexes recent final reports and each linked
// SubReport. Provider calls always happen outside database transactions.
func ReindexReports(ctx context.Context, factory ports.UnitOfWorkFactory, provider ports.EmbeddingProvider, limit int) (ReindexStats, error) {
	if factory == nil || provider == nil || limit <= 0 || limit > MaxReindexReports {
		return ReindexStats{}, fmt.Errorf("retrieval reindex: persistence, provider, and limit in [1,%d] are required: %w", MaxReindexReports, domain.ErrInvariantViolation)
	}
	var finalReports []domain.FinalReport
	if err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		finalReports, err = uow.Reports().ListFinalReports(ctx, limit)
		return err
	}); err != nil {
		return ReindexStats{}, fmt.Errorf("retrieval reindex: list final reports: %w", err)
	}
	stats := ReindexStats{FinalReportsLoaded: len(finalReports)}
	seenSubReports := make(map[domain.SubReportID]struct{})
	failures := make([]error, 0)
	for _, finalReport := range finalReports {
		var subReports []domain.SubReport
		err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
			var err error
			subReports, err = uow.Reports().ListSubReportsForFinalReport(ctx, finalReport.ID, maxReindexSubReports)
			return err
		})
		if err != nil {
			stats.Failed++
			failures = append(failures, fmt.Errorf("final_report:%d list subreports: %w", finalReport.ID, err))
		} else {
			for _, subReport := range subReports {
				if _, seen := seenSubReports[subReport.ID]; seen {
					continue
				}
				seenSubReports[subReport.ID] = struct{}{}
				if _, err := IndexSubReport(ctx, factory, provider, subReport); err != nil {
					stats.Failed++
					failures = append(failures, fmt.Errorf("sub_report:%d: %w", subReport.ID, err))
					continue
				}
				stats.SubReportsProcessed++
			}
		}
		if _, err := IndexFinalReport(ctx, factory, provider, finalReport); err != nil {
			stats.Failed++
			failures = append(failures, fmt.Errorf("final_report:%d: %w", finalReport.ID, err))
			continue
		}
		stats.FinalReportsProcessed++
	}
	return stats, errors.Join(failures...)
}

// IndexSubReport idempotently indexes one accepted immutable SubReport.
func IndexSubReport(ctx context.Context, factory ports.UnitOfWorkFactory, provider ports.EmbeddingProvider, report domain.SubReport) (domain.RetrievalChunk, error) {
	content, metadata, err := subReportDocument(report)
	if err != nil {
		return domain.RetrievalChunk{}, err
	}
	return indexSource(ctx, factory, provider, domain.RetrievalSourceSubReport, int64(report.ID), fmt.Sprintf("sub_report:%d", report.ID), content, metadata)
}

// IndexFinalReport idempotently indexes one accepted immutable FinalReport.
func IndexFinalReport(ctx context.Context, factory ports.UnitOfWorkFactory, provider ports.EmbeddingProvider, report domain.FinalReport) (domain.RetrievalChunk, error) {
	content, metadata, err := finalReportDocument(report)
	if err != nil {
		return domain.RetrievalChunk{}, err
	}
	return indexSource(ctx, factory, provider, domain.RetrievalSourceFinalReport, int64(report.ID), fmt.Sprintf("final_report:%d", report.ID), content, metadata)
}

// Retrieve embeds a query once, runs model-scoped cosine search, and applies a
// deterministic byte budget to the ordered results.
func Retrieve(ctx context.Context, factory ports.UnitOfWorkFactory, provider ports.EmbeddingProvider, query Query) ([]ContextItem, error) {
	if factory == nil || provider == nil {
		return nil, fmt.Errorf("retrieval: persistence and embedding provider must be configured: %w", domain.ErrInvariantViolation)
	}
	query.Text = strings.TrimSpace(query.Text)
	query.IdempotencyKey = strings.TrimSpace(query.IdempotencyKey)
	if query.Text == "" || len([]byte(query.Text)) > domain.RetrievalChunkMaxBytes || query.IdempotencyKey == "" {
		return nil, fmt.Errorf("retrieval: query text and idempotency key must be bounded and non-empty: %w", domain.ErrInvariantViolation)
	}
	if query.Limit == 0 {
		query.Limit = DefaultLimit
	}
	if query.MaxCosineDistance == 0 {
		query.MaxCosineDistance = DefaultMaxCosineDistance
	}
	if query.ContextBytes == 0 {
		query.ContextBytes = DefaultContextBytes
	}
	if query.Limit < 1 || query.Limit > MaxLimit || math.IsNaN(query.MaxCosineDistance) || math.IsInf(query.MaxCosineDistance, 0) || query.MaxCosineDistance <= 0 || query.MaxCosineDistance > 2 || query.ContextBytes < 1 || query.ContextBytes > MaxContextBytes {
		return nil, fmt.Errorf("retrieval: query bounds are invalid: %w", domain.ErrInvariantViolation)
	}
	if len(query.ExcludeSourceRefs) > domain.RetrievalReferenceLimit {
		return nil, fmt.Errorf("retrieval: excluded source refs exceed %d values: %w", domain.RetrievalReferenceLimit, domain.ErrInvariantViolation)
	}
	model := strings.TrimSpace(provider.Model())
	if model == "" || len(model) > 128 {
		return nil, fmt.Errorf("retrieval: embedding model must contain 1-128 bytes: %w", domain.ErrInvariantViolation)
	}
	embedded, err := provider.Embed(ctx, ports.EmbeddingRequest{
		Inputs:         []string{query.Text},
		IdempotencyKey: query.IdempotencyKey,
	})
	if err != nil {
		return nil, fmt.Errorf("retrieval: embed query: %w", err)
	}
	if err := validateEmbeddingResponse(model, 1, embedded); err != nil {
		return nil, err
	}
	excluded := make(map[string]struct{}, len(query.ExcludeSourceRefs))
	for _, sourceRef := range query.ExcludeSourceRefs {
		sourceRef = strings.TrimSpace(sourceRef)
		if _, _, err := domain.ParseRetrievalSourceRef(sourceRef); err != nil {
			return nil, fmt.Errorf("retrieval: excluded source ref %q is invalid: %w", sourceRef, err)
		}
		excluded[sourceRef] = struct{}{}
	}
	searchLimit := query.Limit + len(excluded)
	if searchLimit > maxSearchLimit {
		searchLimit = maxSearchLimit
	}
	var rows []domain.RetrievedChunk
	err = factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		rows, err = uow.Reports().SearchRetrievalChunks(ctx, model, embedded.Vectors[0], query.MaxCosineDistance, searchLimit)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("retrieval: search corpus: %w", err)
	}
	if query.ContextBytes <= 2 {
		return nil, nil
	}
	remaining := query.ContextBytes - 2 // JSON array brackets.
	out := make([]ContextItem, 0, len(rows))
	for _, row := range rows {
		if _, skip := excluded[row.Chunk.SourceRef]; skip {
			continue
		}
		item := ContextItem{
			SourceRef:      row.Chunk.SourceRef,
			SourceKind:     row.Chunk.SourceKind,
			Content:        row.Chunk.Content,
			CosineDistance: row.CosineDistance,
		}
		separatorBytes := 0
		if len(out) > 0 {
			separatorBytes = 1
		}
		fitted, encodedBytes, ok := fitContextItem(item, remaining-separatorBytes)
		if !ok {
			break
		}
		out = append(out, fitted)
		remaining -= separatorBytes + encodedBytes
		if remaining <= 0 || len(out) == query.Limit {
			break
		}
	}
	return out, nil
}

func fitContextItem(item ContextItem, budget int) (ContextItem, int, bool) {
	if budget <= 0 {
		return ContextItem{}, 0, false
	}
	item.Content = strings.TrimSpace(item.Content)
	encoded, err := json.Marshal(item)
	if err != nil {
		return ContextItem{}, 0, false
	}
	if item.Content != "" && len(encoded) <= budget {
		return item, len(encoded), true
	}
	runes := []rune(item.Content)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		candidate := item
		candidate.Content = strings.TrimSpace(string(runes[:mid]))
		encoded, err = json.Marshal(candidate)
		if err == nil && candidate.Content != "" && len(encoded) <= budget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low == 0 {
		return ContextItem{}, 0, false
	}
	item.Content = strings.TrimSpace(string(runes[:low]))
	encoded, err = json.Marshal(item)
	if err != nil || item.Content == "" || len(encoded) > budget {
		return ContextItem{}, 0, false
	}
	return item, len(encoded), true
}

func indexSource(
	ctx context.Context,
	factory ports.UnitOfWorkFactory,
	provider ports.EmbeddingProvider,
	sourceKind domain.RetrievalSourceKind,
	sourceID int64,
	sourceRef string,
	content string,
	metadata json.RawMessage,
) (domain.RetrievalChunk, error) {
	if factory == nil || provider == nil || sourceID <= 0 {
		return domain.RetrievalChunk{}, fmt.Errorf("retrieval: source, persistence, and embedding provider must be configured: %w", domain.ErrInvariantViolation)
	}
	model := strings.TrimSpace(provider.Model())
	if model == "" || len(model) > 128 {
		return domain.RetrievalChunk{}, fmt.Errorf("retrieval: embedding model must contain 1-128 bytes: %w", domain.ErrInvariantViolation)
	}
	existing, found, err := findSource(ctx, factory, sourceKind, sourceID, model)
	if err != nil {
		return domain.RetrievalChunk{}, err
	}
	if found {
		if existing.Content != content || existing.SourceRef != sourceRef {
			return domain.RetrievalChunk{}, fmt.Errorf("retrieval: immutable source %s changed after indexing: %w", sourceRef, domain.ErrInvariantViolation)
		}
		return existing, nil
	}
	embedded, err := provider.Embed(ctx, ports.EmbeddingRequest{
		Inputs:         []string{content},
		IdempotencyKey: "retrieval-index:" + sourceRef,
	})
	if err != nil {
		return domain.RetrievalChunk{}, fmt.Errorf("retrieval: embed %s: %w", sourceRef, err)
	}
	if err := validateEmbeddingResponse(model, 1, embedded); err != nil {
		return domain.RetrievalChunk{}, err
	}
	candidate, err := domain.NewRetrievalChunk(domain.RetrievalChunk{
		SourceKind:          sourceKind,
		SourceID:            sourceID,
		SourceRef:           sourceRef,
		Content:             content,
		EmbeddingModel:      model,
		EmbeddingDimensions: domain.RetrievalEmbeddingDimensions,
		Embedding:           embedded.Vectors[0],
		Metadata:            metadata,
	})
	if err != nil {
		return domain.RetrievalChunk{}, err
	}
	var saved domain.RetrievalChunk
	err = factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Reports().SaveRetrievalChunk(ctx, candidate)
		return err
	})
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.RetrievalChunk{}, err
	}
	existing, found, findErr := findSource(ctx, factory, sourceKind, sourceID, model)
	if findErr != nil {
		return domain.RetrievalChunk{}, findErr
	}
	if !found || existing.ContentDigest != candidate.ContentDigest {
		return domain.RetrievalChunk{}, fmt.Errorf("retrieval: duplicate source %s does not match indexed content: %w", sourceRef, domain.ErrInvariantViolation)
	}
	return existing, nil
}

func findSource(ctx context.Context, factory ports.UnitOfWorkFactory, sourceKind domain.RetrievalSourceKind, sourceID int64, model string) (domain.RetrievalChunk, bool, error) {
	var chunk domain.RetrievalChunk
	found := false
	err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().FindRetrievalChunkBySource(ctx, sourceKind, sourceID, model)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		chunk = got
		found = true
		return nil
	})
	return chunk, found, err
}

func validateEmbeddingResponse(model string, inputCount int, response ports.EmbeddingResponse) error {
	if strings.TrimSpace(response.Model) != model || len(response.Vectors) != inputCount {
		return fmt.Errorf("retrieval: embedding response model or vector count mismatch: %w", domain.ErrInvariantViolation)
	}
	for i, vector := range response.Vectors {
		if len(vector) != domain.RetrievalEmbeddingDimensions {
			return fmt.Errorf("retrieval: embedding vector[%d] has %d dimensions, want %d: %w", i, len(vector), domain.RetrievalEmbeddingDimensions, domain.ErrInvariantViolation)
		}
		var normSquared float64
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return fmt.Errorf("retrieval: embedding vector[%d] contains a non-finite value: %w", i, domain.ErrInvariantViolation)
			}
			normSquared += float64(value) * float64(value)
		}
		if normSquared == 0 {
			return fmt.Errorf("retrieval: embedding vector[%d] must be non-zero: %w", i, domain.ErrInvariantViolation)
		}
	}
	return nil
}

func subReportDocument(report domain.SubReport) (string, json.RawMessage, error) {
	if report.ID == 0 {
		return "", nil, fmt.Errorf("retrieval: subreport must be persisted: %w", domain.ErrInvariantViolation)
	}
	document := struct {
		SourceRef  string `json:"source_ref"`
		Scenario   string `json:"scenario"`
		Title      string `json:"title"`
		Summary    string `json:"summary"`
		Severity   string `json:"severity"`
		Confidence string `json:"confidence"`
	}{
		SourceRef:  fmt.Sprintf("sub_report:%d", report.ID),
		Scenario:   truncateUTF8Bytes(report.Scenario, 128),
		Title:      truncateUTF8Bytes(report.Title, 512),
		Summary:    truncateUTF8Bytes(report.Summary, 4*1024),
		Severity:   string(report.Severity),
		Confidence: string(report.Confidence),
	}
	raw, err := marshalBoundedReportDocument(
		"subreport",
		func() ([]byte, error) { return json.Marshal(document) },
		boundedDocumentField{value: document.Title, set: func(value string) { document.Title = value }},
		boundedDocumentField{value: document.Summary, set: func(value string) { document.Summary = value }},
	)
	if err != nil {
		return "", nil, err
	}
	metadata, err := json.Marshal(map[string]any{
		"evidence_snapshot_id": int64(report.EvidenceSnapshotID),
		"scenario":             report.Scenario,
	})
	if err != nil {
		return "", nil, fmt.Errorf("retrieval: marshal subreport metadata: %w", err)
	}
	return string(raw), metadata, nil
}

func finalReportDocument(report domain.FinalReport) (string, json.RawMessage, error) {
	if report.ID == 0 {
		return "", nil, fmt.Errorf("retrieval: final report must be persisted: %w", domain.ErrInvariantViolation)
	}
	document := struct {
		SourceRef        string `json:"source_ref"`
		Title            string `json:"title"`
		ExecutiveSummary string `json:"executive_summary"`
		Severity         string `json:"severity"`
		Confidence       string `json:"confidence"`
		NotificationText string `json:"notification_text"`
	}{
		SourceRef:        fmt.Sprintf("final_report:%d", report.ID),
		Title:            truncateUTF8Bytes(report.Title, 512),
		ExecutiveSummary: truncateUTF8Bytes(report.ExecutiveSummary, 4*1024),
		Severity:         string(report.Severity),
		Confidence:       string(report.Confidence),
		NotificationText: truncateUTF8Bytes(report.NotificationText, 2*1024),
	}
	raw, err := marshalBoundedReportDocument(
		"final report",
		func() ([]byte, error) { return json.Marshal(document) },
		boundedDocumentField{value: document.NotificationText, set: func(value string) { document.NotificationText = value }},
		boundedDocumentField{value: document.Title, set: func(value string) { document.Title = value }},
		boundedDocumentField{value: document.ExecutiveSummary, set: func(value string) { document.ExecutiveSummary = value }},
	)
	if err != nil {
		return "", nil, err
	}
	metadata, err := json.Marshal(map[string]any{"correlation_key": report.CorrelationKey})
	if err != nil {
		return "", nil, fmt.Errorf("retrieval: marshal final report metadata: %w", err)
	}
	return string(raw), metadata, nil
}

type boundedDocumentField struct {
	value string
	set   func(string)
}

func marshalBoundedReportDocument(label string, marshal func() ([]byte, error), shrinkers ...boundedDocumentField) ([]byte, error) {
	raw, err := marshal()
	if err != nil {
		return nil, fmt.Errorf("retrieval: marshal %s document: %w", label, err)
	}
	if len(raw) <= domain.RetrievalChunkMaxBytes {
		return raw, nil
	}
	for _, field := range shrinkers {
		runes := []rune(field.value)
		field.set("")
		raw, err = marshal()
		if err != nil {
			return nil, fmt.Errorf("retrieval: marshal %s document: %w", label, err)
		}
		if len(raw) > domain.RetrievalChunkMaxBytes {
			continue
		}
		low, high := 0, len(runes)
		for low < high {
			mid := (low + high + 1) / 2
			field.set(strings.TrimSpace(string(runes[:mid])))
			candidate, marshalErr := marshal()
			if marshalErr == nil && len(candidate) <= domain.RetrievalChunkMaxBytes {
				low = mid
			} else {
				high = mid - 1
			}
		}
		field.set(strings.TrimSpace(string(runes[:low])))
		raw, err = marshal()
		if err != nil {
			return nil, fmt.Errorf("retrieval: marshal %s document: %w", label, err)
		}
		return raw, nil
	}
	return nil, fmt.Errorf("retrieval: fixed %s document fields exceed %d bytes: %w", label, domain.RetrievalChunkMaxBytes, domain.ErrInvariantViolation)
}

func truncateUTF8Bytes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return ""
	}
	if len([]byte(value)) <= limit {
		return value
	}
	runes := []rune(value)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		if len([]byte(string(runes[:mid]))) <= limit {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return strings.TrimSpace(string(runes[:low]))
}

// EvidenceSnapshotQuery returns a deterministic bounded embedding input for
// current incident evidence. It is intentionally separate from prompt text so
// retrieval similarity cannot be influenced by historical context.
func EvidenceSnapshotQuery(snapshot domain.EvidenceSnapshot) (string, error) {
	if snapshot.ID == 0 || strings.TrimSpace(snapshot.Digest) == "" {
		return "", fmt.Errorf("retrieval: evidence snapshot identity is incomplete: %w", domain.ErrInvariantViolation)
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, snapshot.Payload); err != nil {
		return "", fmt.Errorf("retrieval: evidence snapshot payload must be valid JSON: %w", domain.ErrInvariantViolation)
	}
	query := fmt.Sprintf(
		"current evidence snapshot:%d digest:%s payload:%s",
		snapshot.ID,
		strings.TrimSpace(snapshot.Digest),
		compacted.String(),
	)
	query = truncateUTF8Bytes(query, domain.RetrievalChunkMaxBytes)
	if query == "" {
		return "", fmt.Errorf("retrieval: evidence snapshot query is empty: %w", domain.ErrInvariantViolation)
	}
	return query, nil
}

// QueryDigest provides a bounded deterministic idempotency component without
// putting raw evidence or operator text into provider headers.
func QueryDigest(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return fmt.Sprintf("%x", sum[:])
}
