package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func makeSnapshotForReport(t *testing.T, key, digest string) domain.EvidenceSnapshotID {
	t.Helper()
	groupID := makeGroupForEvidence(t, key)
	var id domain.EvidenceSnapshotID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		saved, err := uow.Evidence().Save(ctx, mustNewSnapshot(t, groupID, digest))
		if err != nil {
			t.Fatalf("Save evidence snapshot: %v", err)
		}
		id = saved.ID
	})
	return id
}

func mustNewReportSubReport(t *testing.T, snapshotID domain.EvidenceSnapshotID, key string) domain.SubReport {
	t.Helper()
	r, err := domain.NewSubReport(domain.SubReport{
		EvidenceSnapshotID: snapshotID,
		IdempotencyKey:     key,
		Scenario:           "single_alert",
		Title:              "CPU saturation",
		Summary:            "CPU usage is above threshold.",
		Severity:           domain.ReportSeverityWarning,
		Confidence:         domain.ReportConfidenceHigh,
		Findings:           json.RawMessage(`[{"label":"CPU","detail":"high","evidence_id":"alert:1"}]`),
		RecommendedActions: json.RawMessage(`[{"label":"Scale","detail":"Add one replica","priority":"medium"}]`),
		EvidenceRefs:       []string{"alert:1"},
		RetrievalRefs:      []string{"final_report:9"},
		Content:            json.RawMessage(`{"title":"CPU saturation","summary":"CPU usage is above threshold."}`),
		Model:              "gpt-test",
		OutputMode:         "json_schema",
		CreatedByWorkflow:  "ReportFanOutWorkflow",
	})
	if err != nil {
		t.Fatalf("NewSubReport: %v", err)
	}
	return r
}

func mustNewReportFinalReport(t *testing.T, key string) domain.FinalReport {
	t.Helper()
	r, err := domain.NewFinalReport(domain.FinalReport{
		CorrelationKey:     "incident-window-1",
		IdempotencyKey:     key,
		Title:              "Payments degradation",
		ExecutiveSummary:   "Payments is degraded by CPU saturation.",
		Severity:           domain.ReportSeverityWarning,
		Confidence:         domain.ReportConfidenceHigh,
		SubReports:         json.RawMessage(`[{"title":"CPU saturation","summary":"CPU usage is above threshold.","severity":"warning"}]`),
		RecommendedActions: json.RawMessage(`[{"label":"Scale","detail":"Add one replica","priority":"medium"}]`),
		NotificationText:   "Payments is degraded. Scale the payments deployment.",
		Content:            json.RawMessage(`{"title":"Payments degradation","notification_text":"Payments is degraded. Scale the payments deployment."}`),
		Model:              "gpt-test",
		OutputMode:         "json_schema",
		CreatedByWorkflow:  "FinalReportWorkflow",
	})
	if err != nil {
		t.Fatalf("NewFinalReport: %v", err)
	}
	return r
}

func TestReportRepository_SaveSubReportAndQuery(t *testing.T) {
	resetDB(t)
	snapshotID := makeSnapshotForReport(t, "report-sub-save", "digest-report-sub-save")

	var saved domain.SubReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotID, "sub-key-1"))
		if err != nil {
			t.Fatalf("SaveSubReport: %v", err)
		}
		saved = got
	})
	if saved.ID == 0 {
		t.Errorf("saved.ID = 0, want non-zero")
	}
	if saved.CreatedAt.IsZero() {
		t.Errorf("saved.CreatedAt is zero")
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byID, err := uow.Reports().FindSubReportByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindSubReportByID: %v", err)
		}
		if byID.IdempotencyKey != "sub-key-1" {
			t.Errorf("FindSubReportByID.IdempotencyKey = %q, want sub-key-1", byID.IdempotencyKey)
		}
		if len(byID.RetrievalRefs) != 1 || byID.RetrievalRefs[0] != "final_report:9" {
			t.Errorf("FindSubReportByID.RetrievalRefs = %v", byID.RetrievalRefs)
		}
		byKey, err := uow.Reports().FindSubReportBySnapshotAndIdempotencyKey(ctx, snapshotID, "sub-key-1")
		if err != nil {
			t.Fatalf("FindSubReportBySnapshotAndIdempotencyKey: %v", err)
		}
		if byKey.ID != saved.ID {
			t.Errorf("FindSubReportBySnapshotAndIdempotencyKey.ID = %d, want %d", byKey.ID, saved.ID)
		}
	})
}

func TestReportRepository_RetrievalChunkCosineSearch(t *testing.T) {
	resetDB(t)
	subReportIDs, finalReportIDs := makeRetrievalReportSources(t)
	chunks := []domain.RetrievalChunk{
		mustRetrievalChunk(t, domain.RetrievalSourceSubReport, int64(subReportIDs[0]), "embed-model", vectorWith(1, 0)),
		mustRetrievalChunk(t, domain.RetrievalSourceSubReport, int64(subReportIDs[1]), "embed-model", vectorWith(1, 1)),
		mustRetrievalChunk(t, domain.RetrievalSourceFinalReport, int64(finalReportIDs[0]), "embed-model", vectorWith(0, 1)),
		mustRetrievalChunk(t, domain.RetrievalSourceFinalReport, int64(finalReportIDs[1]), "other-model", vectorWith(1, 0)),
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for _, chunk := range chunks {
			if _, err := uow.Reports().SaveRetrievalChunk(ctx, chunk); err != nil {
				t.Fatalf("SaveRetrievalChunk(%s): %v", chunk.SourceRef, err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		rows, err := uow.Reports().SearchRetrievalChunks(ctx, "embed-model", vectorWith(1, 0), 0.4, 2)
		if err != nil {
			t.Fatalf("SearchRetrievalChunks: %v", err)
		}
		if len(rows) != 2 || rows[0].Chunk.SourceRef != chunks[0].SourceRef || rows[1].Chunk.SourceRef != chunks[1].SourceRef {
			t.Fatalf("rows = %+v", rows)
		}
		if rows[0].CosineDistance != 0 || rows[1].CosineDistance <= 0 || rows[1].CosineDistance >= 0.4 {
			t.Fatalf("distances = %v/%v", rows[0].CosineDistance, rows[1].CosineDistance)
		}
		found, err := uow.Reports().FindRetrievalChunkBySource(ctx, domain.RetrievalSourceSubReport, int64(subReportIDs[0]), "embed-model")
		if err != nil || found.SourceRef != chunks[0].SourceRef {
			t.Fatalf("FindRetrievalChunkBySource = %+v, %v", found, err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Reports().SaveRetrievalChunk(ctx, chunks[0])
		return err
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate SaveRetrievalChunk error = %v, want ErrAlreadyExists", err)
	}
}

func TestHNSWFilteredSearchContext_UsesTransactionLocalStrictIteration(t *testing.T) {
	ctx := hnswFilteredSearchContext(context.Background())
	got, ok := entsql.VarFromContext(ctx, hnswIterativeScanVariable)
	if !ok || got != hnswIterativeScanMode {
		t.Fatalf("hnsw iterative scan = %q, %v; want %q, true", got, ok, hnswIterativeScanMode)
	}
}

func makeRetrievalReportSources(t *testing.T) ([]domain.SubReportID, []domain.FinalReportID) {
	t.Helper()
	snapshotIDs := []domain.EvidenceSnapshotID{
		makeSnapshotForReport(t, "retrieval-source-a", "digest-retrieval-source-a"),
		makeSnapshotForReport(t, "retrieval-source-b", "digest-retrieval-source-b"),
	}
	subReportIDs := make([]domain.SubReportID, 0, len(snapshotIDs))
	finalReportIDs := make([]domain.FinalReportID, 0, len(snapshotIDs))
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for i, snapshotID := range snapshotIDs {
			subReport, err := uow.Reports().SaveSubReport(
				ctx,
				mustNewReportSubReport(t, snapshotID, fmt.Sprintf("retrieval-sub-%d", i+1)),
			)
			if err != nil {
				t.Fatalf("SaveSubReport retrieval source[%d]: %v", i, err)
			}
			subReportIDs = append(subReportIDs, subReport.ID)
			finalReport, err := uow.Reports().SaveFinalReport(
				ctx,
				mustNewReportFinalReport(t, fmt.Sprintf("retrieval-final-%d", i+1)),
				[]domain.SubReportID{subReport.ID},
			)
			if err != nil {
				t.Fatalf("SaveFinalReport retrieval source[%d]: %v", i, err)
			}
			finalReportIDs = append(finalReportIDs, finalReport.ID)
		}
	})
	return subReportIDs, finalReportIDs
}

func mustRetrievalChunk(t *testing.T, kind domain.RetrievalSourceKind, id int64, model string, embedding []float32) domain.RetrievalChunk {
	t.Helper()
	chunk, err := domain.NewRetrievalChunk(domain.RetrievalChunk{
		SourceKind:          kind,
		SourceID:            id,
		SourceRef:           string(kind) + ":" + fmt.Sprint(id),
		Content:             fmt.Sprintf(`{"source_ref":%q}`, string(kind)+":"+fmt.Sprint(id)),
		EmbeddingModel:      model,
		EmbeddingDimensions: domain.RetrievalEmbeddingDimensions,
		Embedding:           embedding,
		Metadata:            json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("NewRetrievalChunk: %v", err)
	}
	return chunk
}

func vectorWith(first, second float32) []float32 {
	vector := make([]float32, domain.RetrievalEmbeddingDimensions)
	vector[0] = first
	vector[1] = second
	return vector
}

func TestReportRepository_SubReportIdempotencyPerSnapshot(t *testing.T) {
	resetDB(t)
	snapshotA := makeSnapshotForReport(t, "report-sub-idem-A", "digest-report-sub-idem-A")
	snapshotB := makeSnapshotForReport(t, "report-sub-idem-B", "digest-report-sub-idem-B")

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotA, "shared-sub-key")); err != nil {
			t.Fatalf("SaveSubReport first: %v", err)
		}
	})

	ctx := context.Background()
	err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotA, "shared-sub-key"))
		return serr
	})
	if err == nil {
		t.Fatalf("duplicate SaveSubReport: want error, got nil")
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate SaveSubReport: want ErrAlreadyExists, got %v", err)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotB, "shared-sub-key")); err != nil {
			t.Fatalf("SaveSubReport same key on different snapshot: %v", err)
		}
	})
}

func TestReportRepository_ListSubReportsBySnapshot(t *testing.T) {
	resetDB(t)
	snapshotID := makeSnapshotForReport(t, "report-sub-list", "digest-report-sub-list")
	t0 := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)

	var newest, middle domain.SubReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewReportSubReport(t, snapshotID, "sub-list-1")
		oldest.CreatedAt = t0.Add(10 * time.Minute)
		if _, err := uow.Reports().SaveSubReport(ctx, oldest); err != nil {
			t.Fatalf("SaveSubReport oldest: %v", err)
		}
		middle = mustNewReportSubReport(t, snapshotID, "sub-list-2")
		middle.CreatedAt = t0.Add(20 * time.Minute)
		var err error
		middle, err = uow.Reports().SaveSubReport(ctx, middle)
		if err != nil {
			t.Fatalf("SaveSubReport middle: %v", err)
		}
		newest = mustNewReportSubReport(t, snapshotID, "sub-list-3")
		newest.CreatedAt = t0.Add(30 * time.Minute)
		newest, err = uow.Reports().SaveSubReport(ctx, newest)
		if err != nil {
			t.Fatalf("SaveSubReport newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Reports().ListSubReportsBySnapshot(ctx, snapshotID, 2)
		if err != nil {
			t.Fatalf("ListSubReportsBySnapshot: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("ListSubReportsBySnapshot len = %d, want 2", len(out))
		}
		if out[0].ID != newest.ID || out[1].ID != middle.ID {
			t.Errorf("ListSubReportsBySnapshot order = [%d,%d], want [%d,%d]", out[0].ID, out[1].ID, newest.ID, middle.ID)
		}
	})
}

func TestReportRepository_ListReportSourceRefsByEvidenceSnapshot(t *testing.T) {
	resetDB(t)
	snapshotA := makeSnapshotForReport(t, "report-source-refs-a", "digest-report-source-refs-a")
	snapshotB := makeSnapshotForReport(t, "report-source-refs-b", "digest-report-source-refs-b")

	var subReportA, subReportB domain.SubReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		subReportA, err = uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotA, "source-refs-sub-a"))
		if err != nil {
			t.Fatalf("SaveSubReport snapshot A: %v", err)
		}
		subReportB, err = uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotB, "source-refs-sub-b"))
		if err != nil {
			t.Fatalf("SaveSubReport snapshot B: %v", err)
		}
	})

	var finalReportA domain.FinalReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		finalReportA, err = uow.Reports().SaveFinalReport(ctx, mustNewReportFinalReport(t, "source-refs-final-a"), []domain.SubReportID{subReportA.ID})
		if err != nil {
			t.Fatalf("SaveFinalReport snapshot A: %v", err)
		}
		if _, err := uow.Reports().SaveFinalReport(ctx, mustNewReportFinalReport(t, "source-refs-final-b"), []domain.SubReportID{subReportB.ID}); err != nil {
			t.Fatalf("SaveFinalReport snapshot B: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		refs, err := uow.Reports().ListReportSourceRefsByEvidenceSnapshot(ctx, snapshotA, domain.RetrievalReferenceLimit)
		if err != nil {
			t.Fatalf("ListReportSourceRefsByEvidenceSnapshot: %v", err)
		}
		want := []string{
			fmt.Sprintf("sub_report:%d", subReportA.ID),
			fmt.Sprintf("final_report:%d", finalReportA.ID),
		}
		if !slices.Equal(refs, want) {
			t.Fatalf("ListReportSourceRefsByEvidenceSnapshot = %v, want %v", refs, want)
		}

		_, err = uow.Reports().ListReportSourceRefsByEvidenceSnapshot(ctx, snapshotA, 1)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("overflow error = %v, want ErrInvariantViolation", err)
		}
	})
}

func TestReportRepository_SaveFinalReportAndQuery(t *testing.T) {
	resetDB(t)
	snapshotID := makeSnapshotForReport(t, "report-final-save", "digest-report-final-save")
	t0 := time.Date(2026, 5, 27, 11, 0, 0, 0, time.UTC)

	var older, newer domain.SubReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		older = mustNewReportSubReport(t, snapshotID, "final-sub-older")
		older.CreatedAt = t0
		var err error
		older, err = uow.Reports().SaveSubReport(ctx, older)
		if err != nil {
			t.Fatalf("SaveSubReport older: %v", err)
		}
		newer = mustNewReportSubReport(t, snapshotID, "final-sub-newer")
		newer.CreatedAt = t0.Add(time.Minute)
		newer, err = uow.Reports().SaveSubReport(ctx, newer)
		if err != nil {
			t.Fatalf("SaveSubReport newer: %v", err)
		}
	})

	var saved domain.FinalReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Reports().SaveFinalReport(ctx, mustNewReportFinalReport(t, "final-key-1"), []domain.SubReportID{newer.ID, older.ID})
		if err != nil {
			t.Fatalf("SaveFinalReport: %v", err)
		}
		saved = got
	})
	if saved.ID == 0 {
		t.Errorf("saved.ID = 0, want non-zero")
	}
	if saved.CreatedAt.IsZero() {
		t.Errorf("saved.CreatedAt is zero")
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byID, err := uow.Reports().FindFinalReportByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindFinalReportByID: %v", err)
		}
		if byID.IdempotencyKey != "final-key-1" {
			t.Errorf("FindFinalReportByID.IdempotencyKey = %q, want final-key-1", byID.IdempotencyKey)
		}
		byKey, err := uow.Reports().FindFinalReportByIdempotencyKey(ctx, "final-key-1")
		if err != nil {
			t.Fatalf("FindFinalReportByIdempotencyKey: %v", err)
		}
		if byKey.ID != saved.ID {
			t.Errorf("FindFinalReportByIdempotencyKey.ID = %d, want %d", byKey.ID, saved.ID)
		}
		linked, err := uow.Reports().ListSubReportsForFinalReport(ctx, saved.ID, 10)
		if err != nil {
			t.Fatalf("ListSubReportsForFinalReport: %v", err)
		}
		if len(linked) != 2 {
			t.Fatalf("ListSubReportsForFinalReport len = %d, want 2", len(linked))
		}
		if linked[0].ID != older.ID || linked[1].ID != newer.ID {
			t.Errorf("ListSubReportsForFinalReport order = [%d,%d], want [%d,%d]", linked[0].ID, linked[1].ID, older.ID, newer.ID)
		}
	})
}

func TestReportRepository_FinalReportIdempotencyKey(t *testing.T) {
	resetDB(t)
	snapshotID := makeSnapshotForReport(t, "report-final-idem", "digest-report-final-idem")
	var sub domain.SubReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		sub, err = uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotID, "final-idem-sub"))
		if err != nil {
			t.Fatalf("SaveSubReport: %v", err)
		}
		if _, err := uow.Reports().SaveFinalReport(ctx, mustNewReportFinalReport(t, "shared-final-key"), []domain.SubReportID{sub.ID}); err != nil {
			t.Fatalf("SaveFinalReport first: %v", err)
		}
	})

	ctx := context.Background()
	err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, ferr := uow.Reports().SaveFinalReport(ctx, mustNewReportFinalReport(t, "shared-final-key"), []domain.SubReportID{sub.ID})
		return ferr
	})
	if err == nil {
		t.Fatalf("duplicate SaveFinalReport: want error, got nil")
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate SaveFinalReport: want ErrAlreadyExists, got %v", err)
	}
}

func TestReportRepository_NotificationDeliveryLifecycle(t *testing.T) {
	resetDB(t)
	snapshotID := makeSnapshotForReport(t, "report-delivery-lifecycle", "digest-report-delivery-lifecycle")
	var final domain.FinalReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		sub, err := uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotID, "delivery-sub"))
		if err != nil {
			t.Fatalf("SaveSubReport: %v", err)
		}
		final, err = uow.Reports().SaveFinalReport(ctx, mustNewReportFinalReport(t, "delivery-final"), []domain.SubReportID{sub.ID})
		if err != nil {
			t.Fatalf("SaveFinalReport: %v", err)
		}
	})

	key := "final_report:delivery/notification"
	var saved domain.ReportNotificationDelivery
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		pending, err := domain.NewReportNotificationDelivery(final.ID, key)
		if err != nil {
			t.Fatalf("NewReportNotificationDelivery: %v", err)
		}
		pending.ReportNotificationChannelProfileID = 3
		saved, err = uow.Reports().SaveNotificationDelivery(ctx, pending)
		if err != nil {
			t.Fatalf("SaveNotificationDelivery: %v", err)
		}
	})
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved timestamps/id not populated: %+v", saved)
	}
	if saved.Status != domain.ReportNotificationDeliveryStatusPending {
		t.Fatalf("saved.Status = %q, want pending", saved.Status)
	}
	if saved.ReportNotificationChannelProfileID != 3 {
		t.Fatalf("saved channel profile id = %d, want 3", saved.ReportNotificationChannelProfileID)
	}

	ctx := context.Background()
	err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		pending, nerr := domain.NewReportNotificationDelivery(final.ID, key)
		if nerr != nil {
			return nerr
		}
		_, serr := uow.Reports().SaveNotificationDelivery(ctx, pending)
		return serr
	})
	if err == nil || !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate SaveNotificationDelivery err = %v, want ErrAlreadyExists", err)
	}

	deliveredAt := time.Date(2026, 5, 28, 12, 30, 0, 0, time.UTC)
	delivered, err := saved.MarkDelivered("msg-1", "accepted", json.RawMessage(`{"message_id":"msg-1","status":"accepted"}`), deliveredAt)
	if err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		updated, err := uow.Reports().UpdateNotificationDelivery(ctx, delivered)
		if err != nil {
			t.Fatalf("UpdateNotificationDelivery: %v", err)
		}
		if updated.ProviderMessageID != "msg-1" || updated.ProviderStatus != "accepted" {
			t.Fatalf("updated provider fields = %+v", updated)
		}
		if updated.DeliveredAt == nil || !updated.DeliveredAt.Equal(deliveredAt) {
			t.Fatalf("updated.DeliveredAt = %v, want %s", updated.DeliveredAt, deliveredAt)
		}

		byKey, err := uow.Reports().FindNotificationDeliveryByIdempotencyKey(ctx, key)
		if err != nil {
			t.Fatalf("FindNotificationDeliveryByIdempotencyKey: %v", err)
		}
		if byKey.ID != saved.ID ||
			byKey.Status != domain.ReportNotificationDeliveryStatusDelivered ||
			byKey.ReportNotificationChannelProfileID != 3 {
			t.Fatalf("byKey = %+v", byKey)
		}

		deliveries, err := uow.Reports().ListNotificationDeliveriesByFinalReport(ctx, final.ID, 10)
		if err != nil {
			t.Fatalf("ListNotificationDeliveriesByFinalReport: %v", err)
		}
		if len(deliveries) != 1 || deliveries[0].ID != saved.ID {
			t.Fatalf("deliveries = %+v, want saved only", deliveries)
		}
	})
}

func TestReportRepository_ListFinalReports(t *testing.T) {
	resetDB(t)
	snapshotID := makeSnapshotForReport(t, "report-final-list", "digest-report-final-list")
	t0 := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	var sub domain.SubReport
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		sub, err = uow.Reports().SaveSubReport(ctx, mustNewReportSubReport(t, snapshotID, "final-list-sub"))
		if err != nil {
			t.Fatalf("SaveSubReport: %v", err)
		}
		oldest := mustNewReportFinalReport(t, "final-list-1")
		oldest.CreatedAt = t0.Add(10 * time.Minute)
		if _, err := uow.Reports().SaveFinalReport(ctx, oldest, []domain.SubReportID{sub.ID}); err != nil {
			t.Fatalf("SaveFinalReport oldest: %v", err)
		}
		middle := mustNewReportFinalReport(t, "final-list-2")
		middle.CreatedAt = t0.Add(20 * time.Minute)
		if _, err := uow.Reports().SaveFinalReport(ctx, middle, []domain.SubReportID{sub.ID}); err != nil {
			t.Fatalf("SaveFinalReport middle: %v", err)
		}
		newest := mustNewReportFinalReport(t, "final-list-3")
		newest.CreatedAt = t0.Add(30 * time.Minute)
		if _, err := uow.Reports().SaveFinalReport(ctx, newest, []domain.SubReportID{sub.ID}); err != nil {
			t.Fatalf("SaveFinalReport newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Reports().ListFinalReports(ctx, 2)
		if err != nil {
			t.Fatalf("ListFinalReports: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("ListFinalReports len = %d, want 2", len(out))
		}
		if out[0].IdempotencyKey != "final-list-3" || out[1].IdempotencyKey != "final-list-2" {
			t.Errorf("ListFinalReports order = [%s,%s], want [final-list-3,final-list-2]", out[0].IdempotencyKey, out[1].IdempotencyKey)
		}
	})
}

func TestReportRepository_RejectsBadInputs(t *testing.T) {
	resetDB(t)
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		tests := []struct {
			name string
			call func() error
		}{
			{
				name: "find subreport zero snapshot",
				call: func() error {
					_, err := uow.Reports().FindSubReportBySnapshotAndIdempotencyKey(ctx, 0, "key")
					return err
				},
			},
			{
				name: "find subreport empty key",
				call: func() error {
					_, err := uow.Reports().FindSubReportBySnapshotAndIdempotencyKey(ctx, 1, "")
					return err
				},
			},
			{
				name: "list subreports bad limit",
				call: func() error {
					_, err := uow.Reports().ListSubReportsBySnapshot(ctx, 1, 0)
					return err
				},
			},
			{
				name: "save final report empty subreports",
				call: func() error {
					_, err := uow.Reports().SaveFinalReport(ctx, mustNewReportFinalReport(t, "bad-input-final"), nil)
					return err
				},
			},
			{
				name: "save final report zero subreport id",
				call: func() error {
					_, err := uow.Reports().SaveFinalReport(ctx, mustNewReportFinalReport(t, "bad-input-final-zero"), []domain.SubReportID{0})
					return err
				},
			},
			{
				name: "find final report empty key",
				call: func() error {
					_, err := uow.Reports().FindFinalReportByIdempotencyKey(ctx, "")
					return err
				},
			},
			{
				name: "list final reports bad limit",
				call: func() error {
					_, err := uow.Reports().ListFinalReports(ctx, 0)
					return err
				},
			},
			{
				name: "search retrieval chunks non-finite query",
				call: func() error {
					query := vectorWith(1, 0)
					query[0] = float32(math.NaN())
					_, err := uow.Reports().SearchRetrievalChunks(ctx, "embed-model", query, 0.4, 1)
					return err
				},
			},
			{
				name: "search retrieval chunks excessive limit",
				call: func() error {
					_, err := uow.Reports().SearchRetrievalChunks(ctx, "embed-model", vectorWith(1, 0), 0.4, maxRetrievalSearchLimit+1)
					return err
				},
			},
			{
				name: "list linked subreports zero final id",
				call: func() error {
					_, err := uow.Reports().ListSubReportsForFinalReport(ctx, 0, 10)
					return err
				},
			},
			{
				name: "list linked subreports bad limit",
				call: func() error {
					_, err := uow.Reports().ListSubReportsForFinalReport(ctx, 1, 0)
					return err
				},
			},
			{
				name: "update notification delivery zero id",
				call: func() error {
					_, err := uow.Reports().UpdateNotificationDelivery(ctx, domain.ReportNotificationDelivery{})
					return err
				},
			},
			{
				name: "find notification delivery empty key",
				call: func() error {
					_, err := uow.Reports().FindNotificationDeliveryByIdempotencyKey(ctx, "")
					return err
				},
			},
			{
				name: "list notification deliveries zero final id",
				call: func() error {
					_, err := uow.Reports().ListNotificationDeliveriesByFinalReport(ctx, 0, 10)
					return err
				},
			},
			{
				name: "list notification deliveries bad limit",
				call: func() error {
					_, err := uow.Reports().ListNotificationDeliveriesByFinalReport(ctx, 1, 0)
					return err
				},
			},
		}
		for _, tc := range tests {
			err := tc.call()
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("%s: want ErrInvariantViolation, got %v", tc.name, err)
			}
		}
	})
}
