package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/domain"
	embeddingfake "github.com/openclarion/openclarion/internal/providers/embedding/fake"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestIndexSubReportIsIdempotent(t *testing.T) {
	repo := &memoryReportRepo{chunks: make(map[string]domain.RetrievalChunk)}
	factory := memoryFactory{uow: memoryUOW{reports: repo}}
	provider := embeddingfake.NewDeterministic("embed-model")
	report := domain.SubReport{
		ID:                 7,
		EvidenceSnapshotID: 11,
		Scenario:           "single_alert",
		Title:              "CPU saturation",
		Summary:            "CPU is high",
		Severity:           domain.ReportSeverityWarning,
		Confidence:         domain.ReportConfidenceHigh,
		Findings:           json.RawMessage(`[]`),
		RecommendedActions: json.RawMessage(`[]`),
		EvidenceRefs:       []string{"snapshot:11"},
	}

	first, err := IndexSubReport(context.Background(), factory, provider, report)
	if err != nil {
		t.Fatalf("IndexSubReport first: %v", err)
	}
	second, err := IndexSubReport(context.Background(), factory, provider, report)
	if err != nil {
		t.Fatalf("IndexSubReport second: %v", err)
	}
	if first.ID == 0 || second.ID != first.ID || first.SourceRef != "sub_report:7" {
		t.Fatalf("indexed chunks = %+v / %+v", first, second)
	}
	if calls := provider.Calls("retrieval-index:sub_report:7"); calls != 1 {
		t.Fatalf("embedding calls = %d, want 1", calls)
	}
}

func TestReindexReportsIndexesLinkedSourcesIdempotently(t *testing.T) {
	subReport := domain.SubReport{
		ID: 7, EvidenceSnapshotID: 11, Scenario: "single_alert", Title: "CPU", Summary: "CPU high",
		Severity: domain.ReportSeverityWarning, Confidence: domain.ReportConfidenceHigh,
		Findings: json.RawMessage(`[]`), RecommendedActions: json.RawMessage(`[]`),
	}
	finalReports := []domain.FinalReport{
		{ID: 9, CorrelationKey: "window-9", Title: "Incident 9", ExecutiveSummary: "CPU incident", Severity: domain.ReportSeverityWarning, Confidence: domain.ReportConfidenceHigh, RecommendedActions: json.RawMessage(`[]`)},
		{ID: 8, CorrelationKey: "window-8", Title: "Incident 8", ExecutiveSummary: "CPU incident", Severity: domain.ReportSeverityWarning, Confidence: domain.ReportConfidenceHigh, RecommendedActions: json.RawMessage(`[]`)},
	}
	repo := &memoryReportRepo{
		chunks:       make(map[string]domain.RetrievalChunk),
		finalReports: finalReports,
		linkedSubReports: map[domain.FinalReportID][]domain.SubReport{
			9: {subReport},
			8: {subReport},
		},
	}
	factory := memoryFactory{uow: memoryUOW{reports: repo}}
	provider := embeddingfake.NewDeterministic("embed-model")

	first, err := ReindexReports(context.Background(), factory, provider, 10)
	if err != nil {
		t.Fatalf("ReindexReports first: %v", err)
	}
	second, err := ReindexReports(context.Background(), factory, provider, 10)
	if err != nil {
		t.Fatalf("ReindexReports second: %v", err)
	}
	if first.FinalReportsLoaded != 2 || first.FinalReportsProcessed != 2 || first.SubReportsProcessed != 1 || first.Failed != 0 || second != first {
		t.Fatalf("stats first/second = %+v / %+v", first, second)
	}
	if provider.Calls("retrieval-index:sub_report:7") != 1 || provider.Calls("retrieval-index:final_report:9") != 1 || provider.Calls("retrieval-index:final_report:8") != 1 {
		t.Fatalf("unexpected provider call counts")
	}
}

func TestRetrieveExcludesBeforeLimitAndAppliesByteBudget(t *testing.T) {
	repo := &memoryReportRepo{searchRows: []domain.RetrievedChunk{
		retrieved("sub_report:1", "excluded", 0.01),
		retrieved("sub_report:2", "alpha", 0.02),
		retrieved("final_report:3", "bravo", 0.03),
	}}
	factory := memoryFactory{uow: memoryUOW{reports: repo}}
	provider := embeddingfake.NewDeterministic("embed-model")
	firstEncoded, err := json.Marshal(ContextItem{
		SourceRef: "sub_report:2", SourceKind: domain.RetrievalSourceSubReport, Content: "alpha", CosineDistance: 0.02,
	})
	if err != nil {
		t.Fatalf("marshal first context item: %v", err)
	}
	secondEmpty, err := json.Marshal(ContextItem{
		SourceRef: "final_report:3", SourceKind: domain.RetrievalSourceFinalReport, Content: "", CosineDistance: 0.03,
	})
	if err != nil {
		t.Fatalf("marshal second context item: %v", err)
	}
	contextBytes := 2 + len(firstEncoded) + 1 + len(secondEmpty) + len("bra")

	got, err := Retrieve(context.Background(), factory, provider, Query{
		Text:              "current CPU saturation",
		IdempotencyKey:    "retrieval-query:test",
		Limit:             2,
		MaxCosineDistance: 0.2,
		ContextBytes:      contextBytes,
		ExcludeSourceRefs: []string{"sub_report:1"},
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if repo.searchLimit != 3 {
		t.Fatalf("search limit = %d, want 3", repo.searchLimit)
	}
	if len(got) != 2 || got[0].SourceRef != "sub_report:2" || got[0].Content != "alpha" || got[1].Content != "bra" {
		t.Fatalf("contexts = %+v", got)
	}
	serialized, err := json.Marshal(got)
	if err != nil || len(serialized) > contextBytes {
		t.Fatalf("serialized contexts = %d bytes, budget %d, error %v", len(serialized), contextBytes, err)
	}
}

func TestRetrieveRejectsInvalidEmbeddingAndExclusion(t *testing.T) {
	repo := &memoryReportRepo{}
	factory := memoryFactory{uow: memoryUOW{reports: repo}}
	provider := embeddingfake.New("embed-model", map[string][]embeddingfake.Result{
		"retrieval-query:bad": {{Response: ports.EmbeddingResponse{Model: "embed-model", Vectors: [][]float32{{1}}}}},
	})
	_, err := Retrieve(context.Background(), factory, provider, Query{
		Text: "query", IdempotencyKey: "retrieval-query:bad", ExcludeSourceRefs: []string{"snapshot:1"},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("invalid exclusion error = %v, want ErrInvariantViolation", err)
	}

	_, err = Retrieve(context.Background(), factory, provider, Query{
		Text: "query", IdempotencyKey: "retrieval-query:bad",
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("invalid vector error = %v, want ErrInvariantViolation", err)
	}
}

func TestRetrieveRejectsNonFiniteDistance(t *testing.T) {
	provider := embeddingfake.NewDeterministic("embed-model")
	for _, distance := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := Retrieve(context.Background(), memoryFactory{uow: memoryUOW{reports: &memoryReportRepo{}}}, provider, Query{
			Text: "query", IdempotencyKey: "retrieval-query:distance", MaxCosineDistance: distance,
		})
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("distance %v error = %v, want ErrInvariantViolation", distance, err)
		}
	}
}

func TestReportDocumentsAreBoundedAndValidUTF8(t *testing.T) {
	large := strings.Repeat("诊断\\\"", domain.RetrievalChunkMaxBytes)
	for name, build := range map[string]func() (string, json.RawMessage, error){
		"subreport": func() (string, json.RawMessage, error) {
			return subReportDocument(domain.SubReport{
				ID: 1, EvidenceSnapshotID: 2, Scenario: "single_alert", Title: large, Summary: large,
				Severity: domain.ReportSeverityWarning, Confidence: domain.ReportConfidenceHigh,
			})
		},
		"final report": func() (string, json.RawMessage, error) {
			return finalReportDocument(domain.FinalReport{
				ID: 1, CorrelationKey: "window", Title: large, ExecutiveSummary: large, NotificationText: large,
				Severity: domain.ReportSeverityCritical, Confidence: domain.ReportConfidenceMedium,
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			content, metadata, err := build()
			if err != nil {
				t.Fatalf("build document: %v", err)
			}
			if !json.Valid([]byte(content)) || !json.Valid(metadata) || !utf8.ValidString(content) || len([]byte(content)) > domain.RetrievalChunkMaxBytes {
				t.Fatalf("document validity/size = %t/%t/%t/%d", json.Valid([]byte(content)), json.Valid(metadata), utf8.ValidString(content), len([]byte(content)))
			}
		})
	}
}

func TestEvidenceSnapshotQueryIsDeterministicAndBounded(t *testing.T) {
	snapshot := domain.EvidenceSnapshot{ID: 9, Digest: "digest-9", Payload: json.RawMessage(`{ "cpu": 95 }`)}
	first, err := EvidenceSnapshotQuery(snapshot)
	if err != nil {
		t.Fatalf("EvidenceSnapshotQuery: %v", err)
	}
	second, err := EvidenceSnapshotQuery(snapshot)
	if err != nil || first != second || len([]byte(first)) > domain.RetrievalChunkMaxBytes {
		t.Fatalf("query = %q / %q / %v", first, second, err)
	}
	if QueryDigest(first) != QueryDigest(second) {
		t.Fatal("query digest is not deterministic")
	}
}

func retrieved(ref, content string, distance float64) domain.RetrievedChunk {
	kind, id, err := domain.ParseRetrievalSourceRef(ref)
	if err != nil {
		panic(err)
	}
	return domain.RetrievedChunk{
		Chunk:          domain.RetrievalChunk{SourceKind: kind, SourceID: id, SourceRef: ref, Content: content},
		CosineDistance: distance,
	}
}

type memoryFactory struct {
	uow memoryUOW
}

func (f memoryFactory) Begin(context.Context) (ports.UnitOfWork, error) { return f.uow, nil }
func (f memoryFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, f.uow)
}

type memoryUOW struct {
	ports.UnitOfWork
	reports ports.ReportRepository
}

func (u memoryUOW) Reports() ports.ReportRepository { return u.reports }
func (memoryUOW) Commit(context.Context) error      { return nil }
func (memoryUOW) Rollback(context.Context) error    { return nil }

type memoryReportRepo struct {
	ports.ReportRepository
	chunks           map[string]domain.RetrievalChunk
	searchRows       []domain.RetrievedChunk
	searchLimit      int
	finalReports     []domain.FinalReport
	linkedSubReports map[domain.FinalReportID][]domain.SubReport
}

func (r *memoryReportRepo) ListFinalReports(_ context.Context, limit int) ([]domain.FinalReport, error) {
	if limit > len(r.finalReports) {
		limit = len(r.finalReports)
	}
	return append([]domain.FinalReport(nil), r.finalReports[:limit]...), nil
}

func (r *memoryReportRepo) ListSubReportsForFinalReport(_ context.Context, id domain.FinalReportID, limit int) ([]domain.SubReport, error) {
	items := r.linkedSubReports[id]
	if limit > len(items) {
		limit = len(items)
	}
	return append([]domain.SubReport(nil), items[:limit]...), nil
}

func (r *memoryReportRepo) SaveRetrievalChunk(_ context.Context, chunk domain.RetrievalChunk) (domain.RetrievalChunk, error) {
	key := chunk.SourceRef + "/" + chunk.EmbeddingModel
	if _, exists := r.chunks[key]; exists {
		return domain.RetrievalChunk{}, domain.ErrAlreadyExists
	}
	chunk.ID = domain.RetrievalChunkID(len(r.chunks) + 1)
	r.chunks[key] = chunk
	return chunk, nil
}

func (r *memoryReportRepo) FindRetrievalChunkBySource(_ context.Context, kind domain.RetrievalSourceKind, id int64, model string) (domain.RetrievalChunk, error) {
	ref := string(kind) + ":" + strconv.FormatInt(id, 10)
	chunk, found := r.chunks[ref+"/"+model]
	if !found {
		return domain.RetrievalChunk{}, domain.ErrNotFound
	}
	return chunk, nil
}

func (r *memoryReportRepo) SearchRetrievalChunks(_ context.Context, _ string, _ []float32, _ float64, limit int) ([]domain.RetrievedChunk, error) {
	r.searchLimit = limit
	if limit > len(r.searchRows) {
		limit = len(r.searchRows)
	}
	return append([]domain.RetrievedChunk(nil), r.searchRows[:limit]...), nil
}
