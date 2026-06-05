package repository

import (
	"encoding/json"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
)

// mapper.go: bidirectional translation between Ent generated entities
// (storage representation, IDs as `int`) and domain types (behavioural
// representation, IDs as named int64 types). Mappers are the only
// place int <-> int64 conversion happens.
//
// The mappers are deliberately straight assignments: there is NO
// validation here. Domain invariants are enforced by the constructors
// in `internal/domain` BEFORE persistence; database invariants are
// enforced by the schema. A mapper that found mid-flight invalid data
// would be hiding a corrupted row, not "fixing" one.

// alertEventToDomain converts an Ent AlertEvent row to a domain
// entity. The map fields default to non-nil empty maps so callers can
// treat an absent label as "key missing", not "row corrupt".
func alertEventToDomain(e *ent.AlertEvent) domain.AlertEvent {
	labels := e.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	annotations := e.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	return domain.AlertEvent{
		ID:                   domain.AlertEventID(e.ID),
		Source:               e.Source,
		SourceFingerprint:    e.SourceFingerprint,
		CanonicalFingerprint: e.CanonicalFingerprint,
		Labels:               labels,
		Annotations:          annotations,
		RawPayload:           e.RawPayload,
		Status:               domain.AlertStatus(e.Status),
		StartsAt:             e.StartsAt,
		EndsAt:               e.EndsAt,
		CreatedAt:            e.CreatedAt,
	}
}

// alertGroupToDomain converts an Ent AlertGroup row to a domain
// entity. The M2N event link is NOT materialised here; callers that
// need it call AlertRepository.ListEventIDsForGroup separately.
func alertGroupToDomain(g *ent.AlertGroup) domain.AlertGroup {
	return domain.AlertGroup{
		ID:          domain.AlertGroupID(g.ID),
		GroupKey:    g.GroupKey,
		Dimensions:  g.Dimensions,
		Severity:    domain.GroupSeverity(g.Severity),
		EventCount:  g.EventCount,
		Status:      domain.AlertGroupStatus(g.Status),
		FirstSeenAt: g.FirstSeenAt,
		LastSeenAt:  g.LastSeenAt,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

// evidenceSnapshotToDomain converts an Ent EvidenceSnapshot row to a
// domain entity.
func evidenceSnapshotToDomain(s *ent.EvidenceSnapshot) domain.EvidenceSnapshot {
	return domain.EvidenceSnapshot{
		ID:                domain.EvidenceSnapshotID(s.ID),
		AlertGroupID:      domain.AlertGroupID(s.AlertGroupID),
		Digest:            s.Digest,
		Payload:           s.Payload,
		Provenance:        s.Provenance,
		Status:            domain.SnapshotStatus(s.Status),
		MissingFields:     s.MissingFields,
		CreatedByWorkflow: s.CreatedByWorkflow,
		CreatedAt:         s.CreatedAt,
	}
}

// diagnosisTaskToDomain converts an Ent DiagnosisTask row to a domain
// entity.
func diagnosisTaskToDomain(t *ent.DiagnosisTask) domain.DiagnosisTask {
	return domain.DiagnosisTask{
		ID:                 domain.DiagnosisTaskID(t.ID),
		EvidenceSnapshotID: domain.EvidenceSnapshotID(t.EvidenceSnapshotID),
		WorkflowID:         t.WorkflowID,
		RunID:              t.RunID,
		Status:             domain.DiagnosisStatus(t.Status),
		FailureReason:      t.FailureReason,
		StartedAt:          t.StartedAt,
		FinishedAt:         t.FinishedAt,
		CreatedAt:          t.CreatedAt,
		UpdatedAt:          t.UpdatedAt,
	}
}

// diagnosisTaskEventToDomain converts an Ent DiagnosisTaskEvent row
// to a domain entity. DedupeKey preserves nil-vs-pointer-to-string
// semantics so usecases can distinguish "no idempotency key" from "the
// empty string" (the latter is a domain invariant violation rejected
// by the constructor).
func diagnosisTaskEventToDomain(e *ent.DiagnosisTaskEvent) domain.DiagnosisTaskEvent {
	return domain.DiagnosisTaskEvent{
		ID:         domain.DiagnosisTaskEventID(e.ID),
		TaskID:     domain.DiagnosisTaskID(e.TaskID),
		Kind:       e.Kind,
		Payload:    e.Payload,
		DedupeKey:  e.DedupeKey,
		OccurredAt: e.OccurredAt,
		RecordedAt: e.RecordedAt,
	}
}

// chatSessionToDomain converts an Ent ChatSession row to a domain
// entity.
func chatSessionToDomain(s *ent.ChatSession) domain.ChatSession {
	return domain.ChatSession{
		ID:              domain.ChatSessionID(s.ID),
		DiagnosisTaskID: domain.DiagnosisTaskID(s.DiagnosisTaskID),
		SessionKey:      s.SessionKey,
		OwnerSubject:    s.OwnerSubject,
		Status:          domain.ChatSessionStatus(s.Status),
		TurnCount:       s.TurnCount,
		StartedAt:       s.StartedAt,
		LastActivityAt:  s.LastActivityAt,
		ClosedAt:        s.ClosedAt,
		CloseReason:     s.CloseReason,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
}

// chatTurnToDomain converts an Ent ChatTurn row to a domain entity.
func chatTurnToDomain(t *ent.ChatTurn) domain.ChatTurn {
	metadata := t.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	return domain.ChatTurn{
		ID:           domain.ChatTurnID(t.ID),
		SessionID:    domain.ChatSessionID(t.ChatSessionID),
		MessageID:    t.MessageID,
		Sequence:     t.Sequence,
		Role:         domain.ChatRole(t.Role),
		ActorSubject: t.ActorSubject,
		Content:      t.Content,
		Metadata:     metadata,
		OccurredAt:   t.OccurredAt,
		CreatedAt:    t.CreatedAt,
	}
}

// subReportToDomain converts an Ent SubReport row to a domain entity.
func subReportToDomain(r *ent.SubReport) domain.SubReport {
	return domain.SubReport{
		ID:                 domain.SubReportID(r.ID),
		EvidenceSnapshotID: domain.EvidenceSnapshotID(r.EvidenceSnapshotID),
		IdempotencyKey:     r.IdempotencyKey,
		Scenario:           r.Scenario,
		Title:              r.Title,
		Summary:            r.Summary,
		Severity:           domain.ReportSeverity(r.Severity),
		Confidence:         domain.ReportConfidence(r.Confidence),
		Findings:           r.Findings,
		RecommendedActions: r.RecommendedActions,
		EvidenceRefs:       r.EvidenceRefs,
		Content:            r.Content,
		Model:              r.Model,
		OutputMode:         r.OutputMode,
		CreatedByWorkflow:  r.CreatedByWorkflow,
		CreatedAt:          r.CreatedAt,
	}
}

// finalReportToDomain converts an Ent FinalReport row to a domain entity.
func finalReportToDomain(r *ent.FinalReport) domain.FinalReport {
	return domain.FinalReport{
		ID:                 domain.FinalReportID(r.ID),
		CorrelationKey:     r.CorrelationKey,
		IdempotencyKey:     r.IdempotencyKey,
		Title:              r.Title,
		ExecutiveSummary:   r.ExecutiveSummary,
		Severity:           domain.ReportSeverity(r.Severity),
		Confidence:         domain.ReportConfidence(r.Confidence),
		SubReports:         r.SubreportSummaries,
		RecommendedActions: r.RecommendedActions,
		NotificationText:   r.NotificationText,
		Content:            r.Content,
		Model:              r.Model,
		OutputMode:         r.OutputMode,
		CreatedByWorkflow:  r.CreatedByWorkflow,
		CreatedAt:          r.CreatedAt,
	}
}

// reportNotificationDeliveryToDomain converts an Ent
// ReportNotificationDelivery row to a domain entity.
func reportNotificationDeliveryToDomain(r *ent.ReportNotificationDelivery) domain.ReportNotificationDelivery {
	return domain.ReportNotificationDelivery{
		ID:                domain.ReportNotificationDeliveryID(r.ID),
		FinalReportID:     domain.FinalReportID(r.FinalReportID),
		IdempotencyKey:    r.IdempotencyKey,
		ProviderMessageID: r.ProviderMessageID,
		ProviderStatus:    r.ProviderStatus,
		Status:            domain.ReportNotificationDeliveryStatus(r.Status),
		Raw:               r.Raw,
		FailureReason:     r.FailureReason,
		DeliveredAt:       r.DeliveredAt,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}
}

// alertSourceProfileToDomain converts an Ent AlertSourceProfile row to a
// domain entity.
func alertSourceProfileToDomain(p *ent.AlertSourceProfile) domain.AlertSourceProfile {
	labels := p.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return domain.AlertSourceProfile{
		ID:        domain.AlertSourceProfileID(p.ID),
		Name:      p.Name,
		Kind:      domain.AlertSourceKind(p.Kind),
		BaseURL:   p.BaseURL,
		AuthMode:  domain.AlertSourceAuthMode(p.AuthMode),
		SecretRef: p.SecretRef,
		Enabled:   p.Enabled,
		Labels:    labels,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

// alertEventIDsToEnt converts a slice of domain.AlertEventID
// (int64) to a slice of Ent IDs (int). Used by LinkEventsToGroup
// to feed AddEventIDs. The conversion is unconditional because
// Ent's ID column is bigserial and Go's int is at least 32 bits;
// on 64-bit platforms the conversion is lossless.
func alertEventIDsToEnt(ids []domain.AlertEventID) []int {
	if len(ids) == 0 {
		return []int{}
	}
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = int(id)
	}
	return out
}

func subReportIDsToEnt(ids []domain.SubReportID) []int {
	if len(ids) == 0 {
		return []int{}
	}
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = int(id)
	}
	return out
}
