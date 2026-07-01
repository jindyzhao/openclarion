// Package reportnotification owns final-report notification delivery and
// retry semantics.
package reportnotification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	maxProviderMessageIDLength = 256
	maxProviderStatusLength    = 64
	maxFailureReasonLength     = 2000

	linkedSubReportLookupLimit = 500
	diagnosisTaskLookupLimit   = 5
	diagnosisEventLookupLimit  = 1

	diagnosisEventFinalReady = "diagnosis_room.final_conclusion_ready"
	diagnosisEventClosed     = "diagnosis_room.closed"
	diagnosisEventProgress   = "diagnosis_room.turn_persisted"
)

// NotificationPurpose controls whether a report notification is phrased as a
// diagnosis handoff or an operator-confirmed final report.
type NotificationPurpose string

// NotificationPurpose values describe the operator-visible report delivery
// scenario for a notification request.
const (
	NotificationPurposeHandoff NotificationPurpose = "handoff"
	NotificationPurposeFinal   NotificationPurpose = "final"
)

// RetryState reports whether Send triggered a provider request or reused an
// existing delivery proof row.
type RetryState string

// RetryState values returned by final-report notification delivery.
const (
	RetryStateSent             RetryState = "sent"
	RetryStateAlreadyPending   RetryState = "already_pending"
	RetryStateAlreadyDelivered RetryState = "already_delivered"
)

// Clock supplies the delivery timestamp. It is injected for focused tests.
type Clock func() time.Time

// Request identifies the persisted final report notification to send.
type Request struct {
	FinalReportID                      domain.FinalReportID
	ReportNotificationChannelProfileID domain.NotificationChannelProfileID
	NotificationPurpose                NotificationPurpose
}

// Result returns provider delivery metadata for callers and workflows.
type Result struct {
	Delivery                   domain.ReportNotificationDelivery
	FinalReportID              domain.FinalReportID
	NotificationIdempotencyKey string
	ProviderMessageID          string
	RetryState                 RetryState
	Status                     string
}

// Service sends final report notifications and updates durable delivery proof.
type Service struct {
	uowFactory                   ports.UnitOfWorkFactory
	imProvider                   ports.IMProvider
	notificationProviderResolver ports.NotificationChannelProviderResolver
	clock                        Clock
}

// Option customizes Service construction.
type Option func(*Service)

// WithIMProvider injects the legacy fallback provider used when no channel
// profile id is bound to the report workflow.
func WithIMProvider(provider ports.IMProvider) Option {
	return func(s *Service) {
		s.imProvider = provider
	}
}

// WithNotificationChannelProviderResolver injects profile-backed provider
// resolution.
func WithNotificationChannelProviderResolver(resolver ports.NotificationChannelProviderResolver) Option {
	return func(s *Service) {
		s.notificationProviderResolver = resolver
	}
}

// WithClock overrides the delivery timestamp source.
func WithClock(clock Clock) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// NewService constructs a final-report notification service.
func NewService(uowFactory ports.UnitOfWorkFactory, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("report notification: unit of work factory is required: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		uowFactory: uowFactory,
		clock:      time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service, nil
}

// Send sends or retries the final-report notification. The same delivery proof
// row is reused by idempotency key so failed attempts can later move to
// delivered.
func (s *Service) Send(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.uowFactory == nil || s.clock == nil {
		return Result{}, fmt.Errorf("report notification: service is not configured: %w", domain.ErrInvariantViolation)
	}
	if req.FinalReportID == 0 {
		return Result{}, fmt.Errorf("report notification: final_report_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if req.ReportNotificationChannelProfileID < 0 {
		return Result{}, fmt.Errorf("report notification: report_notification_channel_profile_id must be non-negative: %w", domain.ErrInvariantViolation)
	}
	notificationPurpose, err := normalizeNotificationPurpose(req.NotificationPurpose)
	if err != nil {
		return Result{}, err
	}

	report, err := s.loadFinalReport(ctx, req.FinalReportID)
	if err != nil {
		return Result{}, err
	}
	if notificationPurpose == NotificationPurposeFinal {
		if err := s.ensureFinalNotificationReady(ctx, report.ID); err != nil {
			return Result{}, err
		}
	}
	idempotencyKey := reportNotificationIdempotencyKey(report.ID, notificationPurpose)
	notification := ports.IMNotification{
		IdempotencyKey: idempotencyKey,
		FinalReportID:  int64(report.ID),
		CorrelationKey: report.CorrelationKey,
		Title:          report.Title,
		Body:           reportNotificationBody(report, notificationPurpose),
		Severity:       string(report.Severity),
	}
	deliveryLog, existingDelivery, err := s.ensureNotificationDelivery(ctx, report.ID, notification.IdempotencyKey)
	if err != nil {
		return Result{}, err
	}
	if deliveryLog.Status == domain.ReportNotificationDeliveryStatusDelivered {
		return resultFromDelivery(deliveryLog, RetryStateAlreadyDelivered), nil
	}
	if existingDelivery && deliveryLog.Status == domain.ReportNotificationDeliveryStatusPending {
		return resultFromDelivery(deliveryLog, RetryStateAlreadyPending), nil
	}
	deliveryLog.ReportNotificationChannelProfileID = req.ReportNotificationChannelProfileID

	imProvider, err := s.reportNotificationProvider(ctx, req.ReportNotificationChannelProfileID)
	if err != nil {
		if persistErr := s.markNotificationDeliveryFailed(ctx, deliveryLog, err); persistErr != nil {
			return Result{}, fmt.Errorf("report notification: persist failure after provider resolution error: %w", persistErr)
		}
		return Result{}, err
	}

	delivery, err := imProvider.SendNotification(ctx, notification)
	if err != nil {
		if persistErr := s.markNotificationDeliveryFailed(ctx, deliveryLog, err); persistErr != nil {
			return Result{}, fmt.Errorf("report notification: persist failure after provider error: %w", persistErr)
		}
		return Result{}, err
	}
	delivered, err := deliveryLog.MarkDelivered(
		truncateString(delivery.ProviderMessageID, maxProviderMessageIDLength),
		truncateString(delivery.Status, maxProviderStatusLength),
		defaultJSONObject(delivery.Raw),
		s.clock(),
	)
	if err != nil {
		return Result{}, err
	}
	saved, err := s.updateNotificationDelivery(ctx, delivered)
	if err != nil {
		return Result{}, err
	}
	return resultFromDelivery(saved, RetryStateSent), nil
}

func (s *Service) reportNotificationProvider(ctx context.Context, channelProfileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	if channelProfileID == 0 {
		if s.imProvider == nil {
			return nil, fmt.Errorf("report notification: im provider is not configured: %w", domain.ErrInvariantViolation)
		}
		return s.imProvider, nil
	}
	if s.notificationProviderResolver == nil {
		return nil, fmt.Errorf("report notification: notification channel provider resolver is not configured: %w", domain.ErrInvariantViolation)
	}
	provider, err := s.notificationProviderResolver.ResolveReportNotificationProvider(ctx, channelProfileID)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("report notification: notification channel provider resolver returned nil provider: %w", domain.ErrInvariantViolation)
	}
	return provider, nil
}

func (s *Service) loadFinalReport(ctx context.Context, id domain.FinalReportID) (domain.FinalReport, error) {
	var report domain.FinalReport
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().FindFinalReportByID(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("report notification: final report %d not found: %w", id, domain.ErrInvariantViolation)
			}
			return err
		}
		report = got
		return nil
	})
	return report, err
}

func (s *Service) ensureFinalNotificationReady(ctx context.Context, reportID domain.FinalReportID) error {
	return s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		subReports, err := uow.Reports().ListSubReportsForFinalReport(ctx, reportID, linkedSubReportLookupLimit)
		if err != nil {
			return err
		}
		if len(subReports) == 0 {
			return fmt.Errorf("report notification: final report %d has no linked subreports: %w", reportID, domain.ErrPreconditionFailed)
		}
		diagnosisRepo := uow.Diagnosis()
		if diagnosisRepo == nil {
			return fmt.Errorf("report notification: diagnosis repository is required for final notification readiness: %w", domain.ErrInvariantViolation)
		}
		for _, subReport := range subReports {
			if err := ensureSubReportFinalNotificationReady(ctx, diagnosisRepo, reportID, subReport); err != nil {
				return err
			}
		}
		return nil
	})
}

func ensureSubReportFinalNotificationReady(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	reportID domain.FinalReportID,
	subReport domain.SubReport,
) error {
	snapshotID := subReport.EvidenceSnapshotID
	if snapshotID == 0 {
		return fmt.Errorf("report notification: final report %d subreport %d has no evidence snapshot: %w", reportID, subReport.ID, domain.ErrPreconditionFailed)
	}
	tasks, err := repo.ListTasksByEvidenceSnapshot(ctx, snapshotID, diagnosisTaskLookupLimit)
	if err != nil {
		return err
	}
	conclusion, ok, err := latestConfirmedDiagnosisConclusion(ctx, repo, tasks)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf(
			"report notification: final report %d evidence snapshot %d has no operator-confirmed diagnosis conclusion: %w",
			reportID,
			snapshotID,
			domain.ErrPreconditionFailed,
		)
	}
	progressRecordedAt, ok, err := latestDiagnosisProgressRecordedAt(ctx, repo, tasks)
	if err != nil {
		return err
	}
	if ok && progressRecordedAt.After(conclusion.RecordedAt) {
		return fmt.Errorf(
			"report notification: final report %d evidence snapshot %d has newer diagnosis progress after the confirmed conclusion: %w",
			reportID,
			snapshotID,
			domain.ErrPreconditionFailed,
		)
	}
	return nil
}

type diagnosisConclusion struct {
	RecordedAt time.Time
}

func latestConfirmedDiagnosisConclusion(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	tasks []domain.DiagnosisTask,
) (diagnosisConclusion, bool, error) {
	var best diagnosisConclusion
	bestSet := false
	for _, task := range tasks {
		conclusion, ok, err := latestConfirmedDiagnosisConclusionForTask(ctx, repo, task.ID)
		if err != nil {
			return diagnosisConclusion{}, false, err
		}
		if !ok {
			continue
		}
		if !bestSet || conclusion.RecordedAt.After(best.RecordedAt) {
			best = conclusion
			bestSet = true
		}
	}
	return best, bestSet, nil
}

func latestConfirmedDiagnosisConclusionForTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) (diagnosisConclusion, bool, error) {
	var best diagnosisConclusion
	bestSet := false
	for _, kind := range []string{diagnosisEventFinalReady, diagnosisEventClosed} {
		events, err := repo.ListEventsByTaskAndKind(ctx, taskID, kind, diagnosisEventLookupLimit)
		if err != nil {
			return diagnosisConclusion{}, false, err
		}
		if len(events) == 0 {
			continue
		}
		conclusion, ok, err := confirmedDiagnosisConclusionFromEvent(events[0])
		if err != nil {
			return diagnosisConclusion{}, false, err
		}
		if !ok {
			continue
		}
		if !bestSet || conclusion.RecordedAt.After(best.RecordedAt) {
			best = conclusion
			bestSet = true
		}
	}
	return best, bestSet, nil
}

type diagnosisConclusionEventPayload struct {
	Kind            string                     `json:"kind"`
	DiagnosisTaskID int64                      `json:"diagnosis_task_id,omitempty"`
	FinalConclusion diagnosisConclusionPayload `json:"final_conclusion"`
}

type diagnosisConclusionPayload struct {
	Status      string `json:"status"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
}

func confirmedDiagnosisConclusionFromEvent(event domain.DiagnosisTaskEvent) (diagnosisConclusion, bool, error) {
	if len(event.Payload) == 0 {
		return diagnosisConclusion{}, false, fmt.Errorf("report notification: diagnosis conclusion event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return diagnosisConclusion{}, false, fmt.Errorf("report notification: diagnosis conclusion event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisConclusionEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return diagnosisConclusion{}, false, fmt.Errorf("report notification: diagnosis conclusion event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return diagnosisConclusion{}, false, fmt.Errorf("report notification: diagnosis conclusion event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return diagnosisConclusion{}, false, fmt.Errorf("report notification: diagnosis conclusion event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.FinalConclusion.Status != "available" || strings.TrimSpace(payload.FinalConclusion.ConfirmedBy) == "" {
		return diagnosisConclusion{}, false, nil
	}
	recordedAt := event.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = event.OccurredAt
	}
	if recordedAt.IsZero() {
		return diagnosisConclusion{}, false, fmt.Errorf("report notification: diagnosis conclusion event %d has no recorded time: %w", event.ID, domain.ErrInvariantViolation)
	}
	return diagnosisConclusion{RecordedAt: recordedAt}, true, nil
}

func latestDiagnosisProgressRecordedAt(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	tasks []domain.DiagnosisTask,
) (time.Time, bool, error) {
	var best time.Time
	bestSet := false
	for _, task := range tasks {
		events, err := repo.ListEventsByTaskAndKind(ctx, task.ID, diagnosisEventProgress, diagnosisEventLookupLimit)
		if err != nil {
			return time.Time{}, false, err
		}
		if len(events) == 0 {
			continue
		}
		recordedAt := events[0].RecordedAt
		if recordedAt.IsZero() {
			recordedAt = events[0].OccurredAt
		}
		if recordedAt.IsZero() {
			return time.Time{}, false, fmt.Errorf("report notification: diagnosis progress event %d has no recorded time: %w", events[0].ID, domain.ErrInvariantViolation)
		}
		if !bestSet || recordedAt.After(best) {
			best = recordedAt
			bestSet = true
		}
	}
	return best, bestSet, nil
}

func reportNotificationBody(report domain.FinalReport, purpose NotificationPurpose) string {
	notificationText := strings.TrimSpace(report.NotificationText)
	if purpose == NotificationPurposeFinal {
		return notificationText
	}
	parts := []string{
		"Report handoff: review Diagnosis Readiness and resolve evidence follow-up before treating this report as the operator-confirmed final conclusion.",
		notificationText,
	}
	return strings.Join(nonEmptyStrings(parts), "\n\n")
}

func normalizeNotificationPurpose(purpose NotificationPurpose) (NotificationPurpose, error) {
	switch purpose {
	case "":
		return NotificationPurposeHandoff, nil
	case NotificationPurposeHandoff, NotificationPurposeFinal:
		return purpose, nil
	default:
		return "", fmt.Errorf("report notification: notification_purpose %q is unsupported: %w", purpose, domain.ErrInvariantViolation)
	}
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func (s *Service) ensureNotificationDelivery(ctx context.Context, finalReportID domain.FinalReportID, idempotencyKey string) (domain.ReportNotificationDelivery, bool, error) {
	existing, found, err := s.lookupNotificationDelivery(ctx, idempotencyKey)
	if err != nil {
		return domain.ReportNotificationDelivery{}, false, err
	}
	if found {
		return existing, true, nil
	}
	pending, err := domain.NewReportNotificationDelivery(finalReportID, idempotencyKey)
	if err != nil {
		return domain.ReportNotificationDelivery{}, false, err
	}
	saved, err := s.saveNotificationDelivery(ctx, pending)
	if err == nil {
		return saved, false, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.ReportNotificationDelivery{}, false, err
	}
	existing, found, err = s.lookupNotificationDelivery(ctx, idempotencyKey)
	if err != nil {
		return domain.ReportNotificationDelivery{}, false, err
	}
	if !found {
		return domain.ReportNotificationDelivery{}, false, fmt.Errorf(
			"report notification: delivery duplicate re-fetch missing for idempotency_key %q",
			idempotencyKey)
	}
	return existing, true, nil
}

func (s *Service) markNotificationDeliveryFailed(ctx context.Context, delivery domain.ReportNotificationDelivery, providerErr error) error {
	failed, err := delivery.MarkFailed(
		truncateString(providerErr.Error(), maxFailureReasonLength),
		notificationFailureRaw(providerErr),
	)
	if err != nil {
		return err
	}
	_, err = s.updateNotificationDelivery(ctx, failed)
	return err
}

func (s *Service) lookupNotificationDelivery(ctx context.Context, idempotencyKey string) (domain.ReportNotificationDelivery, bool, error) {
	var (
		delivery domain.ReportNotificationDelivery
		found    bool
	)
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Reports().FindNotificationDeliveryByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		delivery = existing
		found = true
		return nil
	})
	return delivery, found, err
}

func (s *Service) saveNotificationDelivery(ctx context.Context, delivery domain.ReportNotificationDelivery) (domain.ReportNotificationDelivery, error) {
	var saved domain.ReportNotificationDelivery
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().SaveNotificationDelivery(ctx, delivery)
		if err != nil {
			return err
		}
		saved = got
		return nil
	})
	return saved, err
}

func (s *Service) updateNotificationDelivery(ctx context.Context, delivery domain.ReportNotificationDelivery) (domain.ReportNotificationDelivery, error) {
	var saved domain.ReportNotificationDelivery
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().UpdateNotificationDelivery(ctx, delivery)
		if err != nil {
			return err
		}
		saved = got
		return nil
	})
	return saved, err
}

func reportNotificationIdempotencyKey(id domain.FinalReportID, purpose NotificationPurpose) string {
	switch purpose {
	case NotificationPurposeFinal:
		return fmt.Sprintf("final_report:%d/notification/final", id)
	default:
		return fmt.Sprintf("final_report:%d/notification/handoff", id)
	}
}

func resultFromDelivery(delivery domain.ReportNotificationDelivery, retryState RetryState) Result {
	status := delivery.ProviderStatus
	if status == "" {
		status = string(delivery.Status)
	}
	return Result{
		Delivery:                   delivery,
		FinalReportID:              delivery.FinalReportID,
		NotificationIdempotencyKey: delivery.IdempotencyKey,
		ProviderMessageID:          delivery.ProviderMessageID,
		RetryState:                 retryState,
		Status:                     status,
	}
}

func notificationFailureRaw(err error) json.RawMessage {
	payload := struct {
		Error      string `json:"error"`
		Retryable  *bool  `json:"retryable,omitempty"`
		StatusCode int    `json:"status_code,omitempty"`
	}{
		Error: err.Error(),
	}
	var imErr *ports.IMError
	if errors.As(err, &imErr) {
		retryable := imErr.Retryable
		payload.Retryable = &retryable
		payload.StatusCode = imErr.StatusCode
	}
	raw, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func defaultJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 0 {
		return ""
	}
	return string(runes[:maxLen])
}
