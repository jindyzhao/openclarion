package domain

// Typed identifiers for persisted entities. Using distinct named
// types prevents accidental cross-entity ID confusion at compile time
// (e.g. passing an AlertEventID where a DiagnosisTaskID is expected)
// and documents the FK target right at the field type.
//
// The underlying type is int64 to match the Ent default `bigserial`
// primary key (see schema-catalog.md). A zero value (0) means
// "not yet persisted"; repository Insert paths fill it in.

// AlertEventID is the surrogate identifier of an AlertEvent row.
type AlertEventID int64

// AlertGroupID is the surrogate identifier of an AlertGroup row.
type AlertGroupID int64

// EvidenceSnapshotID is the surrogate identifier of an
// EvidenceSnapshot row.
type EvidenceSnapshotID int64

// DiagnosisTaskID is the surrogate identifier of a DiagnosisTask row.
type DiagnosisTaskID int64

// DiagnosisTaskEventID is the surrogate identifier of a
// DiagnosisTaskEvent row.
type DiagnosisTaskEventID int64

// SubReportID is the surrogate identifier of a SubReport row.
type SubReportID int64

// FinalReportID is the surrogate identifier of a FinalReport row.
type FinalReportID int64

// ReportNotificationDeliveryID is the surrogate identifier of a
// report notification delivery row.
type ReportNotificationDeliveryID int64

// ChatSessionID is the surrogate identifier of an M5 diagnosis-room
// chat session row.
type ChatSessionID int64

// ChatTurnID is the surrogate identifier of an append-only
// diagnosis-room chat turn row.
type ChatTurnID int64

// ChatSessionSummaryID is the surrogate identifier of an immutable,
// versioned diagnosis-room conversation summary row.
type ChatSessionSummaryID int64

// ChatSessionApprovalID is the surrogate identifier of an immutable diagnosis
// conclusion approval row.
type ChatSessionApprovalID int64

// DiagnosisToolTemplateID is the surrogate identifier of an operator-managed
// diagnosis tool template.
type DiagnosisToolTemplateID int64

// AlertSourceProfileID is the surrogate identifier of an operator-managed
// alert source configuration profile.
type AlertSourceProfileID int64

// GroupingPolicyID is the surrogate identifier of an operator-managed
// grouping policy profile.
type GroupingPolicyID int64

// ReportWorkflowPolicyID is the surrogate identifier of an operator-managed
// report workflow policy profile.
type ReportWorkflowPolicyID int64

// ReportWorkflowScheduleID is the surrogate identifier of an operator-managed
// report workflow schedule profile.
type ReportWorkflowScheduleID int64

// NotificationChannelProfileID is the surrogate identifier of an
// operator-managed notification channel profile.
type NotificationChannelProfileID int64

// NotificationChannelTestProofID is the surrogate identifier of a sanitized
// notification channel test proof row.
type NotificationChannelTestProofID int64

// DirectoryDepartmentID is the surrogate identifier of one locally projected
// upstream directory department row.
type DirectoryDepartmentID int64

// DirectoryUserID is the surrogate identifier of one locally projected
// upstream directory user row.
type DirectoryUserID int64

// DirectorySyncRunID is the surrogate identifier of one successful local
// directory sync run.
type DirectorySyncRunID int64

// RBACAssignmentID is the surrogate identifier of one local OpenClarion role
// assignment row.
type RBACAssignmentID int64
