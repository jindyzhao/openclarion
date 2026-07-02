package reportnotification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceSendCreatesDeliveryAndSendsNotification(t *testing.T) {
	report := validFinalReport(101)
	repo := newFakeReportRepo(report)
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "wecom-final-report-101",
		Status:            "accepted",
		Raw:               json.RawMessage(`{"message_id":"wecom-final-report-101"}`),
	}}
	svc, err := NewService(
		fakeUOWFactory{reports: repo},
		WithIMProvider(provider),
		WithClock(func() time.Time { return time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC) }),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Send(context.Background(), Request{FinalReportID: report.ID})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	wantKey := "final_report:101/notification/handoff"
	if result.FinalReportID != report.ID ||
		result.NotificationIdempotencyKey != wantKey ||
		result.ProviderMessageID != "wecom-final-report-101" ||
		result.RetryState != RetryStateSent ||
		result.Status != "accepted" {
		t.Fatalf("result = %+v", result)
	}
	requests := provider.Requests()
	if len(requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(requests))
	}
	if requests[0].IdempotencyKey != wantKey || requests[0].FinalReportID != int64(report.ID) {
		t.Fatalf("provider request = %+v", requests[0])
	}
	if !strings.Contains(requests[0].Body, "Report handoff: review Diagnosis Readiness") ||
		!strings.Contains(requests[0].Body, report.NotificationText) {
		t.Fatalf("provider body = %q, want handoff context and original notification text", requests[0].Body)
	}
	delivery := repo.deliveryByKey[wantKey]
	if delivery.Status != domain.ReportNotificationDeliveryStatusDelivered ||
		delivery.ProviderMessageID != "wecom-final-report-101" ||
		delivery.ProviderStatus != "accepted" ||
		delivery.DeliveredAt == nil {
		t.Fatalf("delivery = %+v", delivery)
	}
}

func TestServiceSendPersistsProviderResolutionFailure(t *testing.T) {
	report := validFinalReport(102)
	repo := newFakeReportRepo(report)
	svc, err := NewService(fakeUOWFactory{reports: repo})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Send(context.Background(), Request{FinalReportID: report.ID})
	if err == nil || !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Send err = %v, want ErrInvariantViolation", err)
	}

	delivery := repo.deliveryByKey["final_report:102/notification/handoff"]
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(delivery.Raw, &payload); err != nil {
		t.Fatalf("decode delivery raw: %v", err)
	}
	if delivery.Status != domain.ReportNotificationDeliveryStatusFailed ||
		!strings.Contains(delivery.FailureReason, "im provider is not configured") ||
		!strings.Contains(payload.Error, "im provider is not configured") {
		t.Fatalf("delivery = %+v raw=%s", delivery, delivery.Raw)
	}
}

func TestServiceSendSkipsAlreadyDeliveredNotification(t *testing.T) {
	report := validFinalReport(103)
	repo := newFakeReportRepo(report)
	deliveredAt := time.Date(2026, 6, 18, 8, 1, 0, 0, time.UTC)
	delivery, err := domain.NewReportNotificationDelivery(report.ID, "final_report:103/notification/handoff")
	if err != nil {
		t.Fatalf("NewReportNotificationDelivery: %v", err)
	}
	delivery.ID = 1
	delivery, err = delivery.MarkDelivered("existing-message", "accepted", json.RawMessage(`{}`), deliveredAt)
	if err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	repo.deliveryByKey[delivery.IdempotencyKey] = delivery
	provider := &recordingIMProvider{}
	svc, err := NewService(fakeUOWFactory{reports: repo}, WithIMProvider(provider))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Send(context.Background(), Request{FinalReportID: report.ID})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if result.ProviderMessageID != "existing-message" ||
		result.RetryState != RetryStateAlreadyDelivered ||
		result.Status != "accepted" {
		t.Fatalf("result = %+v", result)
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("provider requests = %d, want 0", got)
	}
}

func TestServiceSendSkipsFreshPendingNotification(t *testing.T) {
	report := validFinalReport(111)
	repo := newFakeReportRepo(report)
	delivery, err := domain.NewReportNotificationDelivery(report.ID, "final_report:111/notification/handoff")
	if err != nil {
		t.Fatalf("NewReportNotificationDelivery: %v", err)
	}
	delivery.ID = 1
	delivery.CreatedAt = time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	delivery.UpdatedAt = delivery.CreatedAt
	repo.deliveryByKey[delivery.IdempotencyKey] = delivery
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "duplicate-message",
		Status:            "accepted",
		Raw:               json.RawMessage(`{}`),
	}}
	svc, err := NewService(
		fakeUOWFactory{reports: repo},
		WithIMProvider(provider),
		WithClock(func() time.Time { return delivery.CreatedAt.Add(time.Minute) }),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Send(context.Background(), Request{FinalReportID: report.ID})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if result.Status != string(domain.ReportNotificationDeliveryStatusPending) ||
		result.ProviderMessageID != "" ||
		result.RetryState != RetryStateAlreadyPending {
		t.Fatalf("result = %+v, want existing pending delivery", result)
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("provider requests = %d, want 0", got)
	}
	if repo.deliveryByKey[delivery.IdempotencyKey].Status != domain.ReportNotificationDeliveryStatusPending {
		t.Fatalf("delivery = %+v, want still pending", repo.deliveryByKey[delivery.IdempotencyKey])
	}
}

func TestServiceSendRetriesStalePendingNotification(t *testing.T) {
	report := validFinalReport(112)
	repo := newFakeReportRepo(report)
	delivery, err := domain.NewReportNotificationDelivery(report.ID, "final_report:112/notification/handoff")
	if err != nil {
		t.Fatalf("NewReportNotificationDelivery: %v", err)
	}
	delivery.ID = 1
	delivery.CreatedAt = time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	delivery.UpdatedAt = delivery.CreatedAt
	repo.deliveryByKey[delivery.IdempotencyKey] = delivery
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "retried-message",
		Status:            "accepted",
		Raw:               json.RawMessage(`{"message_id":"retried-message"}`),
	}}
	svc, err := NewService(
		fakeUOWFactory{reports: repo},
		WithIMProvider(provider),
		WithClock(func() time.Time { return delivery.CreatedAt.Add(pendingDeliveryInFlightTTL + time.Second) }),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Send(context.Background(), Request{FinalReportID: report.ID})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if result.ProviderMessageID != "retried-message" ||
		result.RetryState != RetryStateSent ||
		result.Status != "accepted" {
		t.Fatalf("result = %+v, want retried delivery", result)
	}
	if got := len(provider.Requests()); got != 1 {
		t.Fatalf("provider requests = %d, want 1", got)
	}
	if repo.deliveryByKey[delivery.IdempotencyKey].Status != domain.ReportNotificationDeliveryStatusDelivered {
		t.Fatalf("delivery = %+v, want delivered", repo.deliveryByKey[delivery.IdempotencyKey])
	}
}

func TestServiceSendUsesNotificationChannelResolver(t *testing.T) {
	report := validFinalReport(104)
	repo := newFakeReportRepo(report)
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "profile-message",
		Status:            "accepted",
		Raw:               json.RawMessage(`{}`),
	}}
	resolver := &recordingResolver{provider: provider}
	svc, err := NewService(
		fakeUOWFactory{reports: repo},
		WithNotificationChannelProviderResolver(resolver),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Send(context.Background(), Request{
		FinalReportID:                      report.ID,
		ReportNotificationChannelProfileID: 7,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if resolver.profileID != 7 {
		t.Fatalf("resolver profile id = %d, want 7", resolver.profileID)
	}
	if result.ProviderMessageID != "profile-message" || result.RetryState != RetryStateSent {
		t.Fatalf("result = %+v", result)
	}
	delivery := repo.deliveryByKey["final_report:104/notification/handoff"]
	if delivery.ReportNotificationChannelProfileID != 7 {
		t.Fatalf("delivery channel profile id = %d, want 7", delivery.ReportNotificationChannelProfileID)
	}
}

func TestServiceSendFinalPurposeDoesNotReuseDeliveredHandoff(t *testing.T) {
	report := validFinalReport(108)
	repo := newFakeReportRepo(report)
	repo.linkedSubReports[report.ID] = []domain.SubReport{
		{ID: 21, EvidenceSnapshotID: 7},
	}
	diagnosis := newReadyFinalNotificationDiagnosisRepo("owner-1", time.Date(2026, 6, 18, 8, 2, 0, 0, time.UTC))
	deliveredAt := time.Date(2026, 6, 18, 8, 1, 0, 0, time.UTC)
	handoff, err := domain.NewReportNotificationDelivery(report.ID, "final_report:108/notification/handoff")
	if err != nil {
		t.Fatalf("NewReportNotificationDelivery: %v", err)
	}
	handoff.ID = 1
	handoff, err = handoff.MarkDelivered("handoff-message", "accepted", json.RawMessage(`{}`), deliveredAt)
	if err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	repo.deliveryByKey[handoff.IdempotencyKey] = handoff
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "final-message",
		Status:            "accepted",
		Raw:               json.RawMessage(`{}`),
	}}
	svc, err := NewService(fakeUOWFactory{reports: repo, diagnosis: diagnosis}, WithIMProvider(provider))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Send(context.Background(), Request{
		FinalReportID:       report.ID,
		NotificationPurpose: NotificationPurposeFinal,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	wantKey := "final_report:108/notification/final"
	if result.NotificationIdempotencyKey != wantKey ||
		result.ProviderMessageID != "final-message" ||
		result.RetryState != RetryStateSent {
		t.Fatalf("result = %+v", result)
	}
	requests := provider.Requests()
	if len(requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(requests))
	}
	if requests[0].IdempotencyKey != wantKey || requests[0].Body != report.NotificationText {
		t.Fatalf("provider request = %+v", requests[0])
	}
	if repo.deliveryByKey["final_report:108/notification/handoff"].ProviderMessageID != "handoff-message" {
		t.Fatalf("handoff delivery was not preserved: %+v", repo.deliveryByKey["final_report:108/notification/handoff"])
	}
	if repo.deliveryByKey[wantKey].ProviderMessageID != "final-message" {
		t.Fatalf("final delivery = %+v", repo.deliveryByKey[wantKey])
	}
}

func TestServiceSendUsesFinalNotificationPurpose(t *testing.T) {
	report := validFinalReport(106)
	repo := newFakeReportRepo(report)
	repo.linkedSubReports[report.ID] = []domain.SubReport{
		{ID: 21, EvidenceSnapshotID: 7},
	}
	diagnosis := newReadyFinalNotificationDiagnosisRepo("owner-1", time.Date(2026, 6, 18, 8, 2, 0, 0, time.UTC))
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "final-message",
		Status:            "accepted",
		Raw:               json.RawMessage(`{}`),
	}}
	svc, err := NewService(fakeUOWFactory{reports: repo, diagnosis: diagnosis}, WithIMProvider(provider))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Send(context.Background(), Request{
		FinalReportID:       report.ID,
		NotificationPurpose: NotificationPurposeFinal,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	requests := provider.Requests()
	if len(requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(requests))
	}
	if requests[0].Body != report.NotificationText {
		t.Fatalf("provider body = %q, want final notification text %q", requests[0].Body, report.NotificationText)
	}
}

func TestServiceSendRejectsFinalPurposeWithoutConfirmedDiagnosis(t *testing.T) {
	report := validFinalReport(109)
	repo := newFakeReportRepo(report)
	repo.linkedSubReports[report.ID] = []domain.SubReport{
		{ID: 21, EvidenceSnapshotID: 7},
	}
	diagnosis := newReadyFinalNotificationDiagnosisRepo("", time.Date(2026, 6, 18, 8, 2, 0, 0, time.UTC))
	provider := &recordingIMProvider{}
	svc, err := NewService(fakeUOWFactory{reports: repo, diagnosis: diagnosis}, WithIMProvider(provider))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Send(context.Background(), Request{
		FinalReportID:       report.ID,
		NotificationPurpose: NotificationPurposeFinal,
	})
	if err == nil || !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("Send err = %v, want ErrPreconditionFailed", err)
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("provider requests = %d, want 0", got)
	}
	if len(repo.deliveryByKey) != 0 {
		t.Fatalf("delivery rows = %+v, want none", repo.deliveryByKey)
	}
}

func TestServiceSendRejectsFinalPurposeWhenProgressIsNewer(t *testing.T) {
	report := validFinalReport(110)
	repo := newFakeReportRepo(report)
	repo.linkedSubReports[report.ID] = []domain.SubReport{
		{ID: 21, EvidenceSnapshotID: 7},
	}
	readyAt := time.Date(2026, 6, 18, 8, 2, 0, 0, time.UTC)
	diagnosis := newReadyFinalNotificationDiagnosisRepo("owner-1", readyAt)
	diagnosis.eventsByTaskAndKind[31][diagnosisEventProgress] = []domain.DiagnosisTaskEvent{
		diagnosisProgressEvent(31, readyAt.Add(time.Minute), readyAt.Add(time.Minute+time.Second)),
	}
	provider := &recordingIMProvider{}
	svc, err := NewService(fakeUOWFactory{reports: repo, diagnosis: diagnosis}, WithIMProvider(provider))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Send(context.Background(), Request{
		FinalReportID:       report.ID,
		NotificationPurpose: NotificationPurposeFinal,
	})
	if err == nil || !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("Send err = %v, want ErrPreconditionFailed", err)
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("provider requests = %d, want 0", got)
	}
	if len(repo.deliveryByKey) != 0 {
		t.Fatalf("delivery rows = %+v, want none", repo.deliveryByKey)
	}
}

func TestServiceSendRejectsInvalidNotificationPurpose(t *testing.T) {
	report := validFinalReport(107)
	repo := newFakeReportRepo(report)
	provider := &recordingIMProvider{}
	svc, err := NewService(fakeUOWFactory{reports: repo}, WithIMProvider(provider))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Send(context.Background(), Request{
		FinalReportID:       report.ID,
		NotificationPurpose: "closed",
	})
	if err == nil || !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Send err = %v, want ErrInvariantViolation", err)
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("provider requests = %d, want 0", got)
	}
}

func TestServiceSendPersistsProfileOnResolverFailure(t *testing.T) {
	report := validFinalReport(105)
	repo := newFakeReportRepo(report)
	svc, err := NewService(fakeUOWFactory{reports: repo})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Send(context.Background(), Request{
		FinalReportID:                      report.ID,
		ReportNotificationChannelProfileID: 9,
	})
	if err == nil || !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Send err = %v, want ErrInvariantViolation", err)
	}

	delivery := repo.deliveryByKey["final_report:105/notification/handoff"]
	if delivery.ReportNotificationChannelProfileID != 9 ||
		delivery.Status != domain.ReportNotificationDeliveryStatusFailed {
		t.Fatalf("delivery = %+v", delivery)
	}
}

func TestReportNotificationBodyKeepsHandoffContext(t *testing.T) {
	report := validFinalReport(108)
	report.NotificationText = "  Checkout latency incident requires owner review.  "

	body := reportNotificationBody(report, NotificationPurposeHandoff)
	if !strings.HasPrefix(body, "Report handoff: review Diagnosis Readiness") {
		t.Fatalf("body = %q, want handoff prefix", body)
	}
	if !strings.Contains(body, "\n\nCheckout latency incident requires owner review.") {
		t.Fatalf("body = %q, want trimmed original notification after separator", body)
	}
}

func validFinalReport(id domain.FinalReportID) domain.FinalReport {
	return domain.FinalReport{
		ID:                id,
		CorrelationKey:    "window:checkout-latency",
		IdempotencyKey:    "final_report:window:checkout-latency",
		Title:             "Checkout latency incident",
		ExecutiveSummary:  "Checkout latency increased after deployment.",
		Severity:          domain.ReportSeverityWarning,
		Confidence:        domain.ReportConfidenceHigh,
		NotificationText:  "Checkout latency incident requires review.",
		CreatedByWorkflow: "FinalReportWorkflow",
		CreatedAt:         time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC),
	}
}

type fakeUOWFactory struct {
	reports   *fakeReportRepo
	diagnosis *fakeDiagnosisRepo
}

func (f fakeUOWFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return fakeUOW{reports: f.reports, diagnosis: f.diagnosis}, nil
}

func (f fakeUOWFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, fakeUOW{reports: f.reports, diagnosis: f.diagnosis})
}

type fakeUOW struct {
	ports.UnitOfWork
	reports   *fakeReportRepo
	diagnosis *fakeDiagnosisRepo
}

func (u fakeUOW) Reports() ports.ReportRepository {
	return u.reports
}

func (u fakeUOW) Diagnosis() ports.DiagnosisRepository {
	return u.diagnosis
}

func (u fakeUOW) Commit(context.Context) error {
	return nil
}

func (u fakeUOW) Rollback(context.Context) error {
	return nil
}

type fakeReportRepo struct {
	ports.ReportRepository
	report           domain.FinalReport
	linkedSubReports map[domain.FinalReportID][]domain.SubReport
	deliveryID       domain.ReportNotificationDeliveryID
	deliveryByKey    map[string]domain.ReportNotificationDelivery
}

func newFakeReportRepo(report domain.FinalReport) *fakeReportRepo {
	return &fakeReportRepo{
		report:           report,
		linkedSubReports: map[domain.FinalReportID][]domain.SubReport{},
		deliveryID:       1,
		deliveryByKey:    map[string]domain.ReportNotificationDelivery{},
	}
}

func (r *fakeReportRepo) FindFinalReportByID(_ context.Context, id domain.FinalReportID) (domain.FinalReport, error) {
	if r.report.ID != id {
		return domain.FinalReport{}, domain.ErrNotFound
	}
	return r.report, nil
}

func (r *fakeReportRepo) ListSubReportsForFinalReport(_ context.Context, finalReportID domain.FinalReportID, limit int) ([]domain.SubReport, error) {
	if r.report.ID != finalReportID {
		return nil, domain.ErrNotFound
	}
	reports := r.linkedSubReports[finalReportID]
	if limit > len(reports) {
		limit = len(reports)
	}
	return reports[:limit], nil
}

func (r *fakeReportRepo) FindNotificationDeliveryByIdempotencyKey(_ context.Context, idempotencyKey string) (domain.ReportNotificationDelivery, error) {
	delivery, ok := r.deliveryByKey[idempotencyKey]
	if !ok {
		return domain.ReportNotificationDelivery{}, domain.ErrNotFound
	}
	return delivery, nil
}

func (r *fakeReportRepo) SaveNotificationDelivery(_ context.Context, delivery domain.ReportNotificationDelivery) (domain.ReportNotificationDelivery, error) {
	if _, ok := r.deliveryByKey[delivery.IdempotencyKey]; ok {
		return domain.ReportNotificationDelivery{}, domain.ErrAlreadyExists
	}
	delivery.ID = r.deliveryID
	r.deliveryID++
	now := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	delivery.CreatedAt = now
	delivery.UpdatedAt = now
	r.deliveryByKey[delivery.IdempotencyKey] = delivery
	return delivery, nil
}

func (r *fakeReportRepo) UpdateNotificationDelivery(_ context.Context, delivery domain.ReportNotificationDelivery) (domain.ReportNotificationDelivery, error) {
	existing, ok := r.deliveryByKey[delivery.IdempotencyKey]
	if !ok {
		return domain.ReportNotificationDelivery{}, domain.ErrNotFound
	}
	if delivery.ID == 0 {
		delivery.ID = existing.ID
	}
	delivery.CreatedAt = existing.CreatedAt
	delivery.UpdatedAt = time.Date(2026, 6, 18, 8, 0, 1, 0, time.UTC)
	r.deliveryByKey[delivery.IdempotencyKey] = delivery
	return delivery, nil
}

type fakeDiagnosisRepo struct {
	ports.DiagnosisRepository
	tasksBySnapshot     map[domain.EvidenceSnapshotID][]domain.DiagnosisTask
	eventsByTaskAndKind map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent
}

func newReadyFinalNotificationDiagnosisRepo(
	confirmedBy string,
	readyAt time.Time,
) *fakeDiagnosisRepo {
	snapshotID := domain.EvidenceSnapshotID(7)
	taskID := domain.DiagnosisTaskID(31)
	return &fakeDiagnosisRepo{
		tasksBySnapshot: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
			snapshotID: {
				{ID: taskID, EvidenceSnapshotID: snapshotID},
			},
		},
		eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
			taskID: {
				diagnosisEventFinalReady: {
					diagnosisConclusionEvent(taskID, snapshotID, confirmedBy, readyAt, readyAt.Add(time.Second)),
				},
			},
		},
	}
}

func (r *fakeDiagnosisRepo) ListTasksByEvidenceSnapshot(_ context.Context, snapshotID domain.EvidenceSnapshotID, limit int) ([]domain.DiagnosisTask, error) {
	tasks := r.tasksBySnapshot[snapshotID]
	if limit > len(tasks) {
		limit = len(tasks)
	}
	return tasks[:limit], nil
}

func (r *fakeDiagnosisRepo) ListEventsByTaskAndKind(_ context.Context, taskID domain.DiagnosisTaskID, kind string, limit int) ([]domain.DiagnosisTaskEvent, error) {
	if r.eventsByTaskAndKind == nil {
		return nil, nil
	}
	events := r.eventsByTaskAndKind[taskID][kind]
	if limit > len(events) {
		limit = len(events)
	}
	return events[:limit], nil
}

func diagnosisConclusionEvent(
	taskID domain.DiagnosisTaskID,
	snapshotID domain.EvidenceSnapshotID,
	confirmedBy string,
	occurredAt time.Time,
	recordedAt time.Time,
) domain.DiagnosisTaskEvent {
	return domain.DiagnosisTaskEvent{
		ID:         41,
		TaskID:     taskID,
		Kind:       diagnosisEventFinalReady,
		OccurredAt: occurredAt,
		RecordedAt: recordedAt,
		Payload: json.RawMessage(fmt.Sprintf(`{
			"kind":"diagnosis_room.final_conclusion_ready",
			"diagnosis_task_id":%d,
			"final_conclusion":{
				"status":"available",
				"evidence_snapshot_id":%d,
				"confirmed_by":%q
			}
		}`, taskID, snapshotID, confirmedBy)),
	}
}

func diagnosisProgressEvent(
	taskID domain.DiagnosisTaskID,
	occurredAt time.Time,
	recordedAt time.Time,
) domain.DiagnosisTaskEvent {
	return domain.DiagnosisTaskEvent{
		ID:         42,
		TaskID:     taskID,
		Kind:       diagnosisEventProgress,
		OccurredAt: occurredAt,
		RecordedAt: recordedAt,
		Payload:    json.RawMessage(`{"kind":"diagnosis_room.turn_persisted"}`),
	}
}

type recordingIMProvider struct {
	mu       sync.Mutex
	requests []ports.IMNotification
	delivery ports.IMDelivery
	err      error
}

func (p *recordingIMProvider) SendNotification(ctx context.Context, req ports.IMNotification) (ports.IMDelivery, error) {
	if err := ctx.Err(); err != nil {
		return ports.IMDelivery{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	if p.err != nil {
		return ports.IMDelivery{}, p.err
	}
	return p.delivery, nil
}

func (p *recordingIMProvider) Requests() []ports.IMNotification {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ports.IMNotification, len(p.requests))
	copy(out, p.requests)
	return out
}

type recordingResolver struct {
	profileID domain.NotificationChannelProfileID
	provider  ports.IMProvider
	err       error
}

func (r *recordingResolver) ResolveReportNotificationProvider(_ context.Context, profileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	r.profileID = profileID
	return r.provider, r.err
}

func (r *recordingResolver) ResolveDiagnosisConsultationNotificationProvider(context.Context, domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	panic("ResolveDiagnosisConsultationNotificationProvider not implemented")
}

func (r *recordingResolver) ResolveDiagnosisCloseNotificationProvider(context.Context, domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	panic("ResolveDiagnosisCloseNotificationProvider not implemented")
}
