package repository

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/finalreport"
	"github.com/openclarion/openclarion/internal/persistence/ent/reportnotificationdelivery"
	"github.com/openclarion/openclarion/internal/persistence/ent/retrievalchunk"
	"github.com/openclarion/openclarion/internal/persistence/ent/subreport"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/pgvector/pgvector-go"
)

// reportRepo is the Ent-backed implementation of ports.ReportRepository.
type reportRepo struct {
	tx     *ent.Tx
	closed *atomic.Int32
}

const maxRetrievalSearchLimit = 20

const (
	hnswIterativeScanVariable = "LOCAL hnsw.iterative_scan"
	hnswIterativeScanMode     = "strict_order"
)

// Compile-time assertion that the implementation satisfies the port.
var _ ports.ReportRepository = (*reportRepo)(nil)

// SaveRetrievalChunk appends one immutable report-corpus projection.
func (r *reportRepo) SaveRetrievalChunk(ctx context.Context, chunk domain.RetrievalChunk) (domain.RetrievalChunk, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.RetrievalChunk{}, err
	}
	validated, err := domain.NewRetrievalChunk(chunk)
	if err != nil {
		return domain.RetrievalChunk{}, err
	}
	row, err := r.tx.RetrievalChunk.Create().
		SetSourceKind(string(validated.SourceKind)).
		SetSourceID(validated.SourceID).
		SetSourceRef(validated.SourceRef).
		SetContent(validated.Content).
		SetContentDigest(validated.ContentDigest).
		SetEmbeddingModel(validated.EmbeddingModel).
		SetEmbeddingDimensions(validated.EmbeddingDimensions).
		SetEmbedding(pgvector.NewVector(validated.Embedding)).
		SetMetadata(validated.Metadata).
		Save(ctx)
	if err != nil {
		return domain.RetrievalChunk{}, asAlreadyExists(err)
	}
	return retrievalChunkToDomain(row), nil
}

// FindRetrievalChunkBySource returns one exact source/model projection.
func (r *reportRepo) FindRetrievalChunkBySource(ctx context.Context, sourceKind domain.RetrievalSourceKind, sourceID int64, embeddingModel string) (domain.RetrievalChunk, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.RetrievalChunk{}, err
	}
	embeddingModel = strings.TrimSpace(embeddingModel)
	if !sourceKind.Valid() || sourceID <= 0 || embeddingModel == "" || len(embeddingModel) > 128 {
		return domain.RetrievalChunk{}, fmt.Errorf("find retrieval chunk: source identity and embedding model must be valid: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.RetrievalChunk.Query().Where(
		retrievalchunk.SourceKindEQ(string(sourceKind)),
		retrievalchunk.SourceIDEQ(sourceID),
		retrievalchunk.EmbeddingModelEQ(embeddingModel),
	).Only(ctx)
	if err != nil {
		return domain.RetrievalChunk{}, asNotFound(err)
	}
	return retrievalChunkToDomain(row), nil
}

// SearchRetrievalChunks performs model-scoped cosine nearest-neighbor search.
func (r *reportRepo) SearchRetrievalChunks(ctx context.Context, embeddingModel string, query []float32, maxCosineDistance float64, limit int) ([]domain.RetrievedChunk, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	embeddingModel = strings.TrimSpace(embeddingModel)
	if embeddingModel == "" || len(embeddingModel) > 128 || len(query) != domain.RetrievalEmbeddingDimensions ||
		limit <= 0 || limit > maxRetrievalSearchLimit ||
		math.IsNaN(maxCosineDistance) || math.IsInf(maxCosineDistance, 0) || maxCosineDistance < 0 || maxCosineDistance > 2 {
		return nil, fmt.Errorf("search retrieval chunks: model, %d-dimension query, distance, and limit must be valid: %w", domain.RetrievalEmbeddingDimensions, domain.ErrInvariantViolation)
	}
	if !finiteNonZeroVector(query) {
		return nil, fmt.Errorf("search retrieval chunks: query vector must be finite and non-zero: %w", domain.ErrInvariantViolation)
	}
	queryVector := pgvector.NewVector(query)
	queryCtx := hnswFilteredSearchContext(ctx)
	rows, err := r.tx.RetrievalChunk.Query().
		Where(
			retrievalchunk.EmbeddingModelEQ(embeddingModel),
			retrievalchunk.EmbeddingDimensionsEQ(domain.RetrievalEmbeddingDimensions),
			func(selector *entsql.Selector) {
				selector.Where(entsql.P(func(builder *entsql.Builder) {
					builder.Join(pgvectorCosineDistance(retrievalchunk.FieldEmbedding, queryVector)).
						WriteOp(entsql.OpLTE).
						Arg(maxCosineDistance)
				}))
			},
		).
		Order(func(selector *entsql.Selector) {
			selector.OrderExpr(pgvectorCosineDistance(retrievalchunk.FieldEmbedding, queryVector))
		}, retrievalchunk.ByID()).
		Limit(limit).
		All(queryCtx)
	if err != nil {
		return nil, fmt.Errorf("search retrieval chunks: %w", err)
	}
	out := make([]domain.RetrievedChunk, 0, len(rows))
	for _, row := range rows {
		distance := cosineDistance(query, row.Embedding.Slice())
		if math.IsNaN(distance) || math.IsInf(distance, 0) || distance < 0 || distance > maxCosineDistance {
			continue
		}
		out = append(out, domain.RetrievedChunk{
			Chunk:          retrievalChunkToDomain(row),
			CosineDistance: distance,
		})
	}
	return out, nil
}

func hnswFilteredSearchContext(ctx context.Context) context.Context {
	// reportRepo always runs inside a transaction. Ent expands this fixed
	// variable into SET LOCAL before the query, so model filtering can continue
	// the HNSW scan without leaking the setting to a pooled connection.
	return entsql.WithVar(ctx, hnswIterativeScanVariable, hnswIterativeScanMode)
}

func pgvectorCosineDistance(column string, value any) entsql.Querier {
	return entsql.ExprFunc(func(builder *entsql.Builder) {
		builder.Ident(column).WriteString(" <=> ").Arg(value)
	})
}

func vectorNorm(vector []float32) float64 {
	var squared float64
	for _, value := range vector {
		squared += float64(value) * float64(value)
	}
	return math.Sqrt(squared)
}

func finiteNonZeroVector(vector []float32) bool {
	var squared float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
		squared += float64(value) * float64(value)
	}
	return squared > 0 && !math.IsInf(squared, 0)
}

func cosineDistance(left, right []float32) float64 {
	if len(left) != len(right) || len(left) == 0 {
		return math.Inf(1)
	}
	var dot float64
	for i := range left {
		dot += float64(left[i]) * float64(right[i])
	}
	denominator := vectorNorm(left) * vectorNorm(right)
	if denominator == 0 {
		return math.Inf(1)
	}
	distance := 1 - dot/denominator
	if distance < 0 && distance > -1e-12 {
		return 0
	}
	return distance
}

// ListReportSourceRefsByEvidenceSnapshot returns every report source that
// could describe the current immutable evidence snapshot. A partial exclusion
// list would allow circular retrieval, so overflow is rejected.
func (r *reportRepo) ListReportSourceRefsByEvidenceSnapshot(ctx context.Context, snapshotID domain.EvidenceSnapshotID, limit int) ([]string, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if snapshotID == 0 {
		return nil, fmt.Errorf("list report source refs by evidence snapshot: snapshot id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 || limit > domain.RetrievalReferenceLimit {
		return nil, fmt.Errorf("list report source refs by evidence snapshot: limit must be in [1,%d] (got %d): %w", domain.RetrievalReferenceLimit, limit, domain.ErrInvariantViolation)
	}

	subReportIDs, err := r.tx.SubReport.Query().
		Where(subreport.EvidenceSnapshotIDEQ(int(snapshotID))).
		Order(subreport.ByID()).
		Limit(limit + 1).
		IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list report source refs by evidence snapshot: list subreports: %w", err)
	}
	if len(subReportIDs) > limit {
		return nil, fmt.Errorf("list report source refs by evidence snapshot: more than %d associated reports: %w", limit, domain.ErrInvariantViolation)
	}

	remaining := limit - len(subReportIDs)
	finalReportIDs, err := r.tx.FinalReport.Query().
		Where(finalreport.HasSubReportsWith(subreport.EvidenceSnapshotIDEQ(int(snapshotID)))).
		Order(finalreport.ByID()).
		Limit(remaining + 1).
		IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list report source refs by evidence snapshot: list final reports: %w", err)
	}
	if len(finalReportIDs) > remaining {
		return nil, fmt.Errorf("list report source refs by evidence snapshot: more than %d associated reports: %w", limit, domain.ErrInvariantViolation)
	}

	refs := make([]string, 0, len(subReportIDs)+len(finalReportIDs))
	for _, id := range subReportIDs {
		refs = append(refs, fmt.Sprintf("%s:%d", domain.RetrievalSourceSubReport, id))
	}
	for _, id := range finalReportIDs {
		refs = append(refs, fmt.Sprintf("%s:%d", domain.RetrievalSourceFinalReport, id))
	}
	return refs, nil
}

// SaveSubReport inserts one immutable SubReport. The
// (evidence_snapshot_id, idempotency_key) unique key is the retry
// boundary for per-snapshot report generation.
func (r *reportRepo) SaveSubReport(ctx context.Context, sr domain.SubReport) (domain.SubReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.SubReport{}, err
	}
	builder := r.tx.SubReport.Create().
		SetEvidenceSnapshotID(int(sr.EvidenceSnapshotID)).
		SetIdempotencyKey(sr.IdempotencyKey).
		SetScenario(sr.Scenario).
		SetTitle(sr.Title).
		SetSummary(sr.Summary).
		SetSeverity(string(sr.Severity)).
		SetConfidence(string(sr.Confidence)).
		SetFindings(sr.Findings).
		SetRecommendedActions(sr.RecommendedActions).
		SetEvidenceRefs(sr.EvidenceRefs).
		SetRetrievalRefs(sr.RetrievalRefs).
		SetContent(sr.Content)
	if sr.Model != "" {
		builder = builder.SetModel(sr.Model)
	}
	if sr.OutputMode != "" {
		builder = builder.SetOutputMode(sr.OutputMode)
	}
	if sr.CreatedByWorkflow != "" {
		builder = builder.SetCreatedByWorkflow(sr.CreatedByWorkflow)
	}
	if !sr.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(sr.CreatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.SubReport{}, asAlreadyExists(err)
	}
	return subReportToDomain(saved), nil
}

// FindSubReportByID returns the SubReport or domain.ErrNotFound.
func (r *reportRepo) FindSubReportByID(ctx context.Context, id domain.SubReportID) (domain.SubReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.SubReport{}, err
	}
	row, err := r.tx.SubReport.Get(ctx, int(id))
	if err != nil {
		return domain.SubReport{}, asNotFound(err)
	}
	return subReportToDomain(row), nil
}

// FindSubReportBySnapshotAndIdempotencyKey returns the report matching
// the per-snapshot idempotency key.
func (r *reportRepo) FindSubReportBySnapshotAndIdempotencyKey(ctx context.Context, snapshotID domain.EvidenceSnapshotID, idempotencyKey string) (domain.SubReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.SubReport{}, err
	}
	if snapshotID == 0 {
		return domain.SubReport{}, fmt.Errorf("find subreport by snapshot/idempotency: snapshot id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if idempotencyKey == "" {
		return domain.SubReport{}, fmt.Errorf("find subreport by snapshot/idempotency: idempotency_key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.SubReport.Query().
		Where(
			subreport.EvidenceSnapshotIDEQ(int(snapshotID)),
			subreport.IdempotencyKeyEQ(idempotencyKey),
		).
		Only(ctx)
	if err != nil {
		return domain.SubReport{}, asNotFound(err)
	}
	return subReportToDomain(row), nil
}

// ListSubReportsBySnapshot returns SubReports for a snapshot ordered by
// recency with ID as a deterministic tie-breaker.
func (r *reportRepo) ListSubReportsBySnapshot(ctx context.Context, snapshotID domain.EvidenceSnapshotID, limit int) ([]domain.SubReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if snapshotID == 0 {
		return nil, fmt.Errorf("list subreports by snapshot: snapshot id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list subreports by snapshot: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.SubReport.Query().
		Where(subreport.EvidenceSnapshotIDEQ(int(snapshotID))).
		Order(subreport.ByCreatedAt(entsql.OrderDesc()), subreport.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subreports by snapshot: %w", err)
	}
	out := make([]domain.SubReport, len(rows))
	for i, row := range rows {
		out[i] = subReportToDomain(row)
	}
	return out, nil
}

// SaveFinalReport inserts one immutable FinalReport and materialises the
// FinalReport <-> SubReport fan-in edge in the same transaction.
func (r *reportRepo) SaveFinalReport(ctx context.Context, fr domain.FinalReport, subReportIDs []domain.SubReportID) (domain.FinalReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.FinalReport{}, err
	}
	validated, err := domain.NewFinalReport(fr)
	if err != nil {
		return domain.FinalReport{}, err
	}
	fr = validated
	if len(subReportIDs) == 0 {
		return domain.FinalReport{}, fmt.Errorf("save final report: subreport ids must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if fr.SuccessfulSubReportCount != len(subReportIDs) {
		return domain.FinalReport{}, fmt.Errorf(
			"save final report: successful_sub_report_count %d does not match %d linked subreports: %w",
			fr.SuccessfulSubReportCount, len(subReportIDs), domain.ErrInvariantViolation,
		)
	}
	seenSubReportIDs := make(map[domain.SubReportID]struct{}, len(subReportIDs))
	for _, id := range subReportIDs {
		if id == 0 {
			return domain.FinalReport{}, fmt.Errorf("save final report: subreport ids must be non-zero: %w", domain.ErrInvariantViolation)
		}
		if _, exists := seenSubReportIDs[id]; exists {
			return domain.FinalReport{}, fmt.Errorf("save final report: duplicate subreport id %d: %w", id, domain.ErrInvariantViolation)
		}
		seenSubReportIDs[id] = struct{}{}
	}
	builder := r.tx.FinalReport.Create().
		SetCorrelationKey(fr.CorrelationKey).
		SetIdempotencyKey(fr.IdempotencyKey).
		SetTitle(fr.Title).
		SetExecutiveSummary(fr.ExecutiveSummary).
		SetSeverity(string(fr.Severity)).
		SetConfidence(string(fr.Confidence)).
		SetGenerationStatus(string(fr.GenerationStatus)).
		SetExpectedSubReportCount(fr.ExpectedSubReportCount).
		SetSuccessfulSubReportCount(fr.SuccessfulSubReportCount).
		SetFailedSubReportCount(fr.FailedSubReportCount).
		SetSubreportSummaries(fr.SubReports).
		SetRecommendedActions(fr.RecommendedActions).
		SetNotificationText(fr.NotificationText).
		SetContent(fr.Content).
		AddSubReportIDs(subReportIDsToEnt(subReportIDs)...)
	if fr.Model != "" {
		builder = builder.SetModel(fr.Model)
	}
	if fr.OutputMode != "" {
		builder = builder.SetOutputMode(fr.OutputMode)
	}
	if fr.CreatedByWorkflow != "" {
		builder = builder.SetCreatedByWorkflow(fr.CreatedByWorkflow)
	}
	if !fr.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(fr.CreatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.FinalReport{}, asAlreadyExists(err)
	}
	return finalReportToDomain(saved), nil
}

// FindFinalReportByID returns the FinalReport or domain.ErrNotFound.
func (r *reportRepo) FindFinalReportByID(ctx context.Context, id domain.FinalReportID) (domain.FinalReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.FinalReport{}, err
	}
	row, err := r.tx.FinalReport.Get(ctx, int(id))
	if err != nil {
		return domain.FinalReport{}, asNotFound(err)
	}
	return finalReportToDomain(row), nil
}

// FindFinalReportByIdempotencyKey returns the report matching the
// global final-report idempotency key.
func (r *reportRepo) FindFinalReportByIdempotencyKey(ctx context.Context, idempotencyKey string) (domain.FinalReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.FinalReport{}, err
	}
	if idempotencyKey == "" {
		return domain.FinalReport{}, fmt.Errorf("find final report by idempotency: idempotency_key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.FinalReport.Query().
		Where(finalreport.IdempotencyKeyEQ(idempotencyKey)).
		Only(ctx)
	if err != nil {
		return domain.FinalReport{}, asNotFound(err)
	}
	return finalReportToDomain(row), nil
}

// ListFinalReports returns the most recent final reports.
func (r *reportRepo) ListFinalReports(ctx context.Context, limit int) ([]domain.FinalReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list final reports: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.FinalReport.Query().
		Order(finalreport.ByCreatedAt(entsql.OrderDesc()), finalreport.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list final reports: %w", err)
	}
	out := make([]domain.FinalReport, len(rows))
	for i, row := range rows {
		out[i] = finalReportToDomain(row)
	}
	return out, nil
}

// ListSubReportsForFinalReport returns the linked SubReports ordered by
// their original creation order.
func (r *reportRepo) ListSubReportsForFinalReport(ctx context.Context, finalReportID domain.FinalReportID, limit int) ([]domain.SubReport, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if finalReportID == 0 {
		return nil, fmt.Errorf("list subreports for final report: final report id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list subreports for final report: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	if _, err := r.tx.FinalReport.Get(ctx, int(finalReportID)); err != nil {
		return nil, asNotFound(err)
	}
	rows, err := r.tx.FinalReport.Query().
		Where(finalreport.IDEQ(int(finalReportID))).
		QuerySubReports().
		Order(subreport.ByCreatedAt(), subreport.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subreports for final report %d: %w", finalReportID, err)
	}
	out := make([]domain.SubReport, len(rows))
	for i, row := range rows {
		out[i] = subReportToDomain(row)
	}
	return out, nil
}

// SaveNotificationDelivery inserts one pending notification delivery row.
func (r *reportRepo) SaveNotificationDelivery(ctx context.Context, d domain.ReportNotificationDelivery) (domain.ReportNotificationDelivery, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportNotificationDelivery{}, err
	}
	builder := r.tx.ReportNotificationDelivery.Create().
		SetFinalReportID(int(d.FinalReportID)).
		SetIdempotencyKey(d.IdempotencyKey).
		SetStatus(string(d.Status)).
		SetRaw(d.Raw)
	if d.ReportNotificationChannelProfileID > 0 {
		builder = builder.SetReportNotificationChannelProfileID(int(d.ReportNotificationChannelProfileID))
	}
	if d.ProviderMessageID != "" {
		builder = builder.SetProviderMessageID(d.ProviderMessageID)
	}
	if d.ProviderStatus != "" {
		builder = builder.SetProviderStatus(d.ProviderStatus)
	}
	if d.FailureReason != "" {
		builder = builder.SetFailureReason(d.FailureReason)
	}
	if d.DeliveredAt != nil {
		builder = builder.SetDeliveredAt(*d.DeliveredAt)
	}
	if !d.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(d.CreatedAt)
	}
	if !d.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(d.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ReportNotificationDelivery{}, asAlreadyExists(err)
	}
	return reportNotificationDeliveryToDomain(saved), nil
}

// UpdateNotificationDelivery writes mutable delivery fields. Immutable
// identity fields (final_report_id, idempotency_key, created_at) are
// ignored. updated_at is stamped automatically by Ent UpdateDefault.
func (r *reportRepo) UpdateNotificationDelivery(ctx context.Context, d domain.ReportNotificationDelivery) (domain.ReportNotificationDelivery, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportNotificationDelivery{}, err
	}
	if d.ID == 0 {
		return domain.ReportNotificationDelivery{}, fmt.Errorf("update notification delivery: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.ReportNotificationDelivery.UpdateOneID(int(d.ID)).
		SetProviderMessageID(d.ProviderMessageID).
		SetProviderStatus(d.ProviderStatus).
		SetStatus(string(d.Status)).
		SetRaw(d.Raw).
		SetFailureReason(d.FailureReason)
	if d.ReportNotificationChannelProfileID > 0 {
		builder = builder.SetReportNotificationChannelProfileID(int(d.ReportNotificationChannelProfileID))
	} else {
		builder = builder.ClearReportNotificationChannelProfileID()
	}
	if d.DeliveredAt != nil {
		builder = builder.SetDeliveredAt(*d.DeliveredAt)
	} else {
		builder = builder.ClearDeliveredAt()
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ReportNotificationDelivery{}, asNotFound(err)
	}
	return reportNotificationDeliveryToDomain(saved), nil
}

// FindNotificationDeliveryByIdempotencyKey returns a delivery row by its
// global idempotency key.
func (r *reportRepo) FindNotificationDeliveryByIdempotencyKey(ctx context.Context, idempotencyKey string) (domain.ReportNotificationDelivery, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportNotificationDelivery{}, err
	}
	if idempotencyKey == "" {
		return domain.ReportNotificationDelivery{}, fmt.Errorf("find notification delivery by idempotency: idempotency_key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.ReportNotificationDelivery.Query().
		Where(reportnotificationdelivery.IdempotencyKeyEQ(idempotencyKey)).
		Only(ctx)
	if err != nil {
		return domain.ReportNotificationDelivery{}, asNotFound(err)
	}
	return reportNotificationDeliveryToDomain(row), nil
}

// ListNotificationDeliveriesByFinalReport returns delivery rows for a
// report ordered by recency with ID as a deterministic tie-breaker.
func (r *reportRepo) ListNotificationDeliveriesByFinalReport(ctx context.Context, finalReportID domain.FinalReportID, limit int) ([]domain.ReportNotificationDelivery, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if finalReportID == 0 {
		return nil, fmt.Errorf("list notification deliveries by final report: final report id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list notification deliveries by final report: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.ReportNotificationDelivery.Query().
		Where(reportnotificationdelivery.FinalReportIDEQ(int(finalReportID))).
		Order(reportnotificationdelivery.ByCreatedAt(entsql.OrderDesc()), reportnotificationdelivery.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list notification deliveries by final report %d: %w", finalReportID, err)
	}
	out := make([]domain.ReportNotificationDelivery, len(rows))
	for i, row := range rows {
		out[i] = reportNotificationDeliveryToDomain(row)
	}
	return out, nil
}
