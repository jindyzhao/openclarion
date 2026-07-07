package repository

import (
	"encoding/json"
	"time"

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
		AlertSourceProfileID: domain.AlertSourceProfileID(e.AlertSourceProfileID),
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
	var reportNotificationChannelProfileID domain.NotificationChannelProfileID
	if r.ReportNotificationChannelProfileID != nil {
		reportNotificationChannelProfileID = domain.NotificationChannelProfileID(*r.ReportNotificationChannelProfileID)
	}
	return domain.ReportNotificationDelivery{
		ID:                                 domain.ReportNotificationDeliveryID(r.ID),
		FinalReportID:                      domain.FinalReportID(r.FinalReportID),
		ReportNotificationChannelProfileID: reportNotificationChannelProfileID,
		IdempotencyKey:                     r.IdempotencyKey,
		ProviderMessageID:                  r.ProviderMessageID,
		ProviderStatus:                     r.ProviderStatus,
		Status:                             domain.ReportNotificationDeliveryStatus(r.Status),
		Raw:                                r.Raw,
		FailureReason:                      r.FailureReason,
		DeliveredAt:                        r.DeliveredAt,
		CreatedAt:                          r.CreatedAt,
		UpdatedAt:                          r.UpdatedAt,
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

// groupingPolicyToDomain converts an Ent GroupingPolicy row to a domain entity.
func groupingPolicyToDomain(p *ent.GroupingPolicy) domain.GroupingPolicy {
	dimensionKeys := p.DimensionKeys
	if dimensionKeys == nil {
		dimensionKeys = []string{}
	}
	sourceFilter := p.SourceFilter
	if sourceFilter == nil {
		sourceFilter = []string{}
	}
	return domain.GroupingPolicy{
		ID:            domain.GroupingPolicyID(p.ID),
		Name:          p.Name,
		DimensionKeys: dimensionKeys,
		SeverityKey:   p.SeverityKey,
		SourceFilter:  sourceFilter,
		Enabled:       p.Enabled,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

// reportWorkflowPolicyToDomain converts an Ent ReportWorkflowPolicy row to a
// domain entity.
func reportWorkflowPolicyToDomain(p *ent.ReportWorkflowPolicy) domain.ReportWorkflowPolicy {
	reportNotificationChannelProfileID := domain.NotificationChannelProfileID(0)
	if p.ReportNotificationChannelProfileID != nil {
		reportNotificationChannelProfileID = domain.NotificationChannelProfileID(*p.ReportNotificationChannelProfileID)
	}
	return domain.ReportWorkflowPolicy{
		ID:                                 domain.ReportWorkflowPolicyID(p.ID),
		Name:                               p.Name,
		AlertSourceProfileID:               domain.AlertSourceProfileID(p.AlertSourceProfileID),
		GroupingPolicyID:                   domain.GroupingPolicyID(p.GroupingPolicyID),
		ReportNotificationChannelProfileID: reportNotificationChannelProfileID,
		TriggerMode:                        domain.ReportWorkflowTriggerMode(p.TriggerMode),
		ReportScenario:                     domain.ReportWorkflowScenario(p.ReportScenario),
		DiagnosisFollowUp:                  domain.DiagnosisFollowUpMode(p.DiagnosisFollowUp),
		Enabled:                            p.Enabled,
		EnabledAt:                          p.EnabledAt,
		DisabledAt:                         p.DisabledAt,
		CreatedAt:                          p.CreatedAt,
		UpdatedAt:                          p.UpdatedAt,
	}
}

// reportWorkflowScheduleToDomain converts an Ent ReportWorkflowSchedule row to
// a domain entity.
func reportWorkflowScheduleToDomain(s *ent.ReportWorkflowSchedule) domain.ReportWorkflowSchedule {
	return domain.ReportWorkflowSchedule{
		ID:                     domain.ReportWorkflowScheduleID(s.ID),
		Name:                   s.Name,
		ReportWorkflowPolicyID: domain.ReportWorkflowPolicyID(s.ReportWorkflowPolicyID),
		TemporalScheduleID:     s.TemporalScheduleID,
		Cadence:                domain.ReportWorkflowScheduleCadence(s.Cadence),
		CalendarHour:           s.CalendarHour,
		CalendarMinute:         s.CalendarMinute,
		CalendarDayOfWeek:      s.CalendarDayOfWeek,
		CalendarDayOfMonth:     s.CalendarDayOfMonth,
		Interval:               time.Duration(s.IntervalNs),
		Offset:                 time.Duration(s.OffsetNs),
		ReplayWindow:           time.Duration(s.ReplayWindowNs),
		ReplayDelay:            time.Duration(s.ReplayDelayNs),
		ReplayLimit:            s.ReplayLimit,
		CatchupWindow:          time.Duration(s.CatchupWindowNs),
		Enabled:                s.Enabled,
		EnabledAt:              s.EnabledAt,
		DisabledAt:             s.DisabledAt,
		CreatedAt:              s.CreatedAt,
		UpdatedAt:              s.UpdatedAt,
	}
}

// diagnosisToolTemplateToDomain converts an Ent DiagnosisToolTemplate row to a
// domain entity.
func diagnosisToolTemplateToDomain(t *ent.DiagnosisToolTemplate) domain.DiagnosisToolTemplate {
	return domain.DiagnosisToolTemplate{
		ID:                   domain.DiagnosisToolTemplateID(t.ID),
		Name:                 t.Name,
		AlertSourceProfileID: domain.AlertSourceProfileID(t.AlertSourceProfileID),
		Tool:                 domain.DiagnosisToolKind(t.Tool),
		QueryTemplate:        t.QueryTemplate,
		DefaultLimit:         t.DefaultLimit,
		DefaultWindow:        time.Duration(t.DefaultWindowNs),
		MaxWindow:            time.Duration(t.MaxWindowNs),
		DefaultStep:          time.Duration(t.DefaultStepNs),
		Enabled:              t.Enabled,
		EnabledAt:            t.EnabledAt,
		DisabledAt:           t.DisabledAt,
		CreatedAt:            t.CreatedAt,
		UpdatedAt:            t.UpdatedAt,
	}
}

// notificationChannelProfileToDomain converts an Ent
// NotificationChannelProfile row to a domain entity.
func notificationChannelProfileToDomain(p *ent.NotificationChannelProfile) domain.NotificationChannelProfile {
	labels := p.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	testProofs := make([]domain.NotificationChannelTestProof, 0, len(p.Edges.TestProofs))
	for _, proof := range p.Edges.TestProofs {
		testProofs = append(testProofs, notificationChannelTestProofToDomain(proof))
	}
	return domain.NotificationChannelProfile{
		ID:               domain.NotificationChannelProfileID(p.ID),
		Name:             p.Name,
		Kind:             domain.NotificationChannelKind(p.Kind),
		SecretRef:        p.SecretRef,
		DeliveryScopes:   notificationDeliveryScopesToDomain(p.DeliveryScopes),
		Enabled:          p.Enabled,
		Labels:           labels,
		LatestTestProofs: testProofs,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

// notificationChannelTestProofToDomain converts an Ent
// NotificationChannelTestProof row to a domain entity.
func notificationChannelTestProofToDomain(p *ent.NotificationChannelTestProof) domain.NotificationChannelTestProof {
	return domain.NotificationChannelTestProof{
		ID:                           domain.NotificationChannelTestProofID(p.ID),
		NotificationChannelProfileID: domain.NotificationChannelProfileID(p.NotificationChannelProfileID),
		Kind:                         domain.NotificationChannelKind(p.Kind),
		Status:                       domain.NotificationChannelTestStatus(p.Status),
		ReasonCode:                   domain.NotificationChannelTestReasonCode(p.ReasonCode),
		Message:                      p.Message,
		ContentKind:                  domain.NotificationChannelTestContentKind(p.ContentKind),
		ContentSHA256:                p.ContentSha256,
		CheckedAt:                    p.CheckedAt,
		ProviderMessageID:            p.ProviderMessageID,
		ProviderStatus:               p.ProviderStatus,
		CreatedAt:                    p.CreatedAt,
	}
}

// directoryDepartmentToDomain converts an Ent DirectoryDepartment row to a
// domain entity.
func directoryDepartmentToDomain(d *ent.DirectoryDepartment) domain.DirectoryDepartment {
	return domain.DirectoryDepartment{
		ID:               domain.DirectoryDepartmentID(d.ID),
		Provider:         d.Provider,
		ExternalID:       d.ExternalID,
		ParentExternalID: d.ParentExternalID,
		Name:             d.Name,
		DisplayName:      d.DisplayName,
		Path:             d.Path,
		ParentPath:       d.ParentPath,
		Level:            d.Level,
		Source:           d.Source,
		MemberCount:      d.MemberCount,
		SourceUpdatedAt:  d.SourceUpdatedAt,
		SyncedAt:         d.SyncedAt,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
	}
}

// directoryUserToDomain converts an Ent DirectoryUser row to a domain entity.
func directoryUserToDomain(u *ent.DirectoryUser) domain.DirectoryUser {
	departmentPaths := u.DepartmentPaths
	if departmentPaths == nil {
		departmentPaths = []string{}
	}
	departmentExternalIDs := u.DepartmentExternalIds
	if departmentExternalIDs == nil {
		departmentExternalIDs = []string{}
	}
	return domain.DirectoryUser{
		ID:                    domain.DirectoryUserID(u.ID),
		Provider:              u.Provider,
		Subject:               u.Subject,
		ExternalID:            u.ExternalID,
		Username:              u.Username,
		DisplayName:           u.DisplayName,
		Email:                 u.Email,
		JobTitle:              u.JobTitle,
		Department:            u.Department,
		Section:               u.Section,
		DepartmentPath:        u.DepartmentPath,
		DepartmentPaths:       departmentPaths,
		DepartmentExternalIDs: departmentExternalIDs,
		Active:                u.Active,
		SourceUpdatedAt:       u.SourceUpdatedAt,
		SyncedAt:              u.SyncedAt,
		CreatedAt:             u.CreatedAt,
		UpdatedAt:             u.UpdatedAt,
	}
}

// directorySyncRunToDomain converts an Ent DirectorySyncRun row to a domain
// entity.
func directorySyncRunToDomain(run *ent.DirectorySyncRun) domain.DirectorySyncRun {
	return domain.DirectorySyncRun{
		ID:                  domain.DirectorySyncRunID(run.ID),
		Provider:            run.Provider,
		PageSize:            run.PageSize,
		UpdatedAfter:        run.UpdatedAfter,
		Status:              domain.DirectorySyncRunStatus(run.Status),
		FailureCode:         run.FailureCode,
		FailureMessage:      run.FailureMessage,
		DepartmentPages:     run.DepartmentPages,
		UserPages:           run.UserPages,
		DepartmentsUpserted: run.DepartmentsUpserted,
		UsersUpserted:       run.UsersUpserted,
		SyncedAt:            run.SyncedAt,
		CreatedAt:           run.CreatedAt,
	}
}

// rbacAssignmentToDomain converts an Ent RBACAssignment row to a domain entity.
func rbacAssignmentToDomain(a *ent.RBACAssignment) domain.RBACAssignment {
	return domain.RBACAssignment{
		ID:          domain.RBACAssignmentID(a.ID),
		SubjectKind: domain.RBACSubjectKind(a.SubjectKind),
		SubjectKey:  a.SubjectKey,
		Role:        domain.RBACRole(a.Role),
		ScopeKind:   domain.RBACScopeKind(a.ScopeKind),
		ScopeKey:    a.ScopeKey,
		Enabled:     a.Enabled,
		CreatedBy:   a.CreatedBy,
		UpdatedBy:   a.UpdatedBy,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

func notificationDeliveryScopesToDomain(scopes []string) []domain.NotificationDeliveryScope {
	if len(scopes) == 0 {
		return []domain.NotificationDeliveryScope{}
	}
	out := make([]domain.NotificationDeliveryScope, len(scopes))
	for i, scope := range scopes {
		out[i] = domain.NotificationDeliveryScope(scope)
	}
	return out
}

func notificationDeliveryScopesToStrings(scopes []domain.NotificationDeliveryScope) []string {
	if len(scopes) == 0 {
		return []string{}
	}
	out := make([]string, len(scopes))
	for i, scope := range scopes {
		out[i] = string(scope)
	}
	return out
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
