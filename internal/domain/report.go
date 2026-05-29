package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ReportSeverity is the report-facing severity vocabulary shared by
// SubReport and FinalReport.
type ReportSeverity string

// ReportSeverity enumeration.
const (
	ReportSeverityInfo     ReportSeverity = "info"
	ReportSeverityWarning  ReportSeverity = "warning"
	ReportSeverityCritical ReportSeverity = "critical"
)

// Valid reports whether s is a known ReportSeverity value.
func (s ReportSeverity) Valid() bool {
	switch s {
	case ReportSeverityInfo, ReportSeverityWarning, ReportSeverityCritical:
		return true
	default:
		return false
	}
}

// ReportConfidence captures how strongly a report is supported by
// evidence.
type ReportConfidence string

// ReportConfidence enumeration.
const (
	ReportConfidenceLow    ReportConfidence = "low"
	ReportConfidenceMedium ReportConfidence = "medium"
	ReportConfidenceHigh   ReportConfidence = "high"
)

// Valid reports whether c is a known ReportConfidence value.
func (c ReportConfidence) Valid() bool {
	switch c {
	case ReportConfidenceLow, ReportConfidenceMedium, ReportConfidenceHigh:
		return true
	default:
		return false
	}
}

// SubReport is one schema-validated AI report for a single
// EvidenceSnapshot. The accepted JSON is retained in Content for
// audit/replay; selected fields are duplicated into typed columns for
// list/detail read paths.
type SubReport struct {
	ID                 SubReportID
	EvidenceSnapshotID EvidenceSnapshotID
	IdempotencyKey     string
	Scenario           string
	Title              string
	Summary            string
	Severity           ReportSeverity
	Confidence         ReportConfidence
	Findings           json.RawMessage
	RecommendedActions json.RawMessage
	EvidenceRefs       []string
	Content            json.RawMessage
	Model              string
	OutputMode         string
	CreatedByWorkflow  string
	CreatedAt          time.Time
}

// NewSubReport constructs a SubReport draft ready for persistence.
// Repository insert paths fill ID / CreatedAt.
func NewSubReport(in SubReport) (SubReport, error) {
	if in.EvidenceSnapshotID == 0 {
		return SubReport{}, fmt.Errorf("sub report: evidence_snapshot_id must be non-zero: %w", ErrInvariantViolation)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return SubReport{}, fmt.Errorf("sub report: idempotency_key must be non-empty: %w", ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Scenario) == "" {
		return SubReport{}, fmt.Errorf("sub report: scenario must be non-empty: %w", ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Title) == "" {
		return SubReport{}, fmt.Errorf("sub report: title must be non-empty: %w", ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Summary) == "" {
		return SubReport{}, fmt.Errorf("sub report: summary must be non-empty: %w", ErrInvariantViolation)
	}
	if !in.Severity.Valid() {
		return SubReport{}, fmt.Errorf("sub report: severity %q is invalid: %w", in.Severity, ErrInvariantViolation)
	}
	if !in.Confidence.Valid() {
		return SubReport{}, fmt.Errorf("sub report: confidence %q is invalid: %w", in.Confidence, ErrInvariantViolation)
	}
	if err := requireValidJSON("sub report: findings", in.Findings); err != nil {
		return SubReport{}, err
	}
	if err := requireValidJSON("sub report: recommended_actions", in.RecommendedActions); err != nil {
		return SubReport{}, err
	}
	if err := requireValidJSON("sub report: content", in.Content); err != nil {
		return SubReport{}, err
	}
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	in.Scenario = strings.TrimSpace(in.Scenario)
	in.Title = strings.TrimSpace(in.Title)
	in.Summary = strings.TrimSpace(in.Summary)
	in.Model = strings.TrimSpace(in.Model)
	in.OutputMode = strings.TrimSpace(in.OutputMode)
	in.CreatedByWorkflow = strings.TrimSpace(in.CreatedByWorkflow)
	return in, nil
}

// FinalReport is the persisted incident-level reduction of validated
// SubReports. It is immutable; notification delivery is tracked by
// ReportNotificationDelivery so report queryability is independent of
// webhook success.
type FinalReport struct {
	ID                 FinalReportID
	CorrelationKey     string
	IdempotencyKey     string
	Title              string
	ExecutiveSummary   string
	Severity           ReportSeverity
	Confidence         ReportConfidence
	SubReports         json.RawMessage
	RecommendedActions json.RawMessage
	NotificationText   string
	Content            json.RawMessage
	Model              string
	OutputMode         string
	CreatedByWorkflow  string
	CreatedAt          time.Time
}

// NewFinalReport constructs a FinalReport draft ready for
// persistence. Repository insert paths fill ID / CreatedAt and attach
// SubReport edges.
func NewFinalReport(in FinalReport) (FinalReport, error) {
	if strings.TrimSpace(in.CorrelationKey) == "" {
		return FinalReport{}, fmt.Errorf("final report: correlation_key must be non-empty: %w", ErrInvariantViolation)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return FinalReport{}, fmt.Errorf("final report: idempotency_key must be non-empty: %w", ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Title) == "" {
		return FinalReport{}, fmt.Errorf("final report: title must be non-empty: %w", ErrInvariantViolation)
	}
	if strings.TrimSpace(in.ExecutiveSummary) == "" {
		return FinalReport{}, fmt.Errorf("final report: executive_summary must be non-empty: %w", ErrInvariantViolation)
	}
	if !in.Severity.Valid() {
		return FinalReport{}, fmt.Errorf("final report: severity %q is invalid: %w", in.Severity, ErrInvariantViolation)
	}
	if !in.Confidence.Valid() {
		return FinalReport{}, fmt.Errorf("final report: confidence %q is invalid: %w", in.Confidence, ErrInvariantViolation)
	}
	if err := requireValidJSON("final report: sub_reports", in.SubReports); err != nil {
		return FinalReport{}, err
	}
	if err := requireValidJSON("final report: recommended_actions", in.RecommendedActions); err != nil {
		return FinalReport{}, err
	}
	if strings.TrimSpace(in.NotificationText) == "" {
		return FinalReport{}, fmt.Errorf("final report: notification_text must be non-empty: %w", ErrInvariantViolation)
	}
	if err := requireValidJSON("final report: content", in.Content); err != nil {
		return FinalReport{}, err
	}
	in.CorrelationKey = strings.TrimSpace(in.CorrelationKey)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	in.Title = strings.TrimSpace(in.Title)
	in.ExecutiveSummary = strings.TrimSpace(in.ExecutiveSummary)
	in.NotificationText = strings.TrimSpace(in.NotificationText)
	in.Model = strings.TrimSpace(in.Model)
	in.OutputMode = strings.TrimSpace(in.OutputMode)
	in.CreatedByWorkflow = strings.TrimSpace(in.CreatedByWorkflow)
	return in, nil
}

// ReportNotificationDeliveryStatus is the notification delivery
// lifecycle state persisted alongside a FinalReport.
type ReportNotificationDeliveryStatus string

// ReportNotificationDeliveryStatus enumeration.
const (
	ReportNotificationDeliveryStatusPending   ReportNotificationDeliveryStatus = "pending"
	ReportNotificationDeliveryStatusDelivered ReportNotificationDeliveryStatus = "delivered"
	ReportNotificationDeliveryStatusFailed    ReportNotificationDeliveryStatus = "failed"
)

// Valid reports whether s is a known delivery lifecycle state.
func (s ReportNotificationDeliveryStatus) Valid() bool {
	switch s {
	case ReportNotificationDeliveryStatusPending,
		ReportNotificationDeliveryStatusDelivered,
		ReportNotificationDeliveryStatusFailed:
		return true
	default:
		return false
	}
}

// ReportNotificationDelivery is the durable audit record for one
// outbound FinalReport notification idempotency key. The same row moves
// pending -> delivered/failed across Activity retries.
type ReportNotificationDelivery struct {
	ID                ReportNotificationDeliveryID
	FinalReportID     FinalReportID
	IdempotencyKey    string
	ProviderMessageID string
	ProviderStatus    string
	Status            ReportNotificationDeliveryStatus
	Raw               json.RawMessage
	FailureReason     string
	DeliveredAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NewReportNotificationDelivery constructs a pending delivery row for a
// persisted FinalReport. Repository insert paths fill ID / CreatedAt /
// UpdatedAt.
func NewReportNotificationDelivery(finalReportID FinalReportID, idempotencyKey string) (ReportNotificationDelivery, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if finalReportID == 0 {
		return ReportNotificationDelivery{}, fmt.Errorf("report notification delivery: final_report_id must be non-zero: %w", ErrInvariantViolation)
	}
	if idempotencyKey == "" {
		return ReportNotificationDelivery{}, fmt.Errorf("report notification delivery: idempotency_key must be non-empty: %w", ErrInvariantViolation)
	}
	return ReportNotificationDelivery{
		FinalReportID:  finalReportID,
		IdempotencyKey: idempotencyKey,
		Status:         ReportNotificationDeliveryStatusPending,
		Raw:            json.RawMessage(`{}`),
	}, nil
}

// MarkDelivered records the provider's accepted delivery metadata.
func (d ReportNotificationDelivery) MarkDelivered(providerMessageID, providerStatus string, raw json.RawMessage, deliveredAt time.Time) (ReportNotificationDelivery, error) {
	if deliveredAt.IsZero() {
		return ReportNotificationDelivery{}, fmt.Errorf("report notification delivery: delivered_at must be set: %w", ErrInvariantViolation)
	}
	normalised := NormalizeUTCMicro(deliveredAt)
	providerStatus = strings.TrimSpace(providerStatus)
	if providerStatus == "" {
		providerStatus = string(ReportNotificationDeliveryStatusDelivered)
	}
	d.ProviderMessageID = strings.TrimSpace(providerMessageID)
	d.ProviderStatus = providerStatus
	d.Status = ReportNotificationDeliveryStatusDelivered
	d.Raw = defaultJSONObject(raw)
	d.FailureReason = ""
	d.DeliveredAt = &normalised
	if err := d.validate(); err != nil {
		return ReportNotificationDelivery{}, err
	}
	return d, nil
}

// MarkFailed records the provider failure without dropping the
// idempotency row. A later retry may move the same row to delivered.
func (d ReportNotificationDelivery) MarkFailed(reason string, raw json.RawMessage) (ReportNotificationDelivery, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ReportNotificationDelivery{}, fmt.Errorf("report notification delivery: failure_reason must be non-empty when status==failed: %w", ErrInvariantViolation)
	}
	d.ProviderMessageID = ""
	d.ProviderStatus = ""
	d.Status = ReportNotificationDeliveryStatusFailed
	d.Raw = defaultJSONObject(raw)
	d.FailureReason = reason
	d.DeliveredAt = nil
	if err := d.validate(); err != nil {
		return ReportNotificationDelivery{}, err
	}
	return d, nil
}

func (d ReportNotificationDelivery) validate() error {
	if d.FinalReportID == 0 {
		return fmt.Errorf("report notification delivery: final_report_id must be non-zero: %w", ErrInvariantViolation)
	}
	if strings.TrimSpace(d.IdempotencyKey) == "" {
		return fmt.Errorf("report notification delivery: idempotency_key must be non-empty: %w", ErrInvariantViolation)
	}
	if !d.Status.Valid() {
		return fmt.Errorf("report notification delivery: status %q is invalid: %w", d.Status, ErrInvariantViolation)
	}
	if err := requireValidJSON("report notification delivery: raw", defaultJSONObject(d.Raw)); err != nil {
		return err
	}
	if d.Status == ReportNotificationDeliveryStatusDelivered && d.DeliveredAt == nil {
		return fmt.Errorf("report notification delivery: delivered_at must be set when status==delivered: %w", ErrInvariantViolation)
	}
	if d.Status != ReportNotificationDeliveryStatusDelivered && d.DeliveredAt != nil {
		return fmt.Errorf("report notification delivery: delivered_at must be empty when status==%q: %w", d.Status, ErrInvariantViolation)
	}
	if d.Status == ReportNotificationDeliveryStatusFailed && strings.TrimSpace(d.FailureReason) == "" {
		return fmt.Errorf("report notification delivery: failure_reason must be non-empty when status==failed: %w", ErrInvariantViolation)
	}
	if d.Status != ReportNotificationDeliveryStatusFailed && strings.TrimSpace(d.FailureReason) != "" {
		return fmt.Errorf("report notification delivery: failure_reason must be empty when status==%q: %w", d.Status, ErrInvariantViolation)
	}
	return nil
}

func requireValidJSON(label string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s must be non-empty JSON: %w", label, ErrInvariantViolation)
	}
	if !json.Valid(raw) {
		return fmt.Errorf("%s must be valid JSON: %w", label, ErrInvariantViolation)
	}
	return nil
}

func defaultJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}
