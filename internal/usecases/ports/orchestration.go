package ports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

// ReportBatchStartItem identifies one EvidenceSnapshot to include in
// a report batch workflow.
type ReportBatchStartItem struct {
	EvidenceSnapshotID domain.EvidenceSnapshotID
	Scenario           string
	GroupIndex         int
}

// ReportBatchStartRequest describes an idempotent request to start
// the report batch workflow for a replay window.
type ReportBatchStartRequest struct {
	WorkflowID                         string
	CorrelationKey                     string
	ReportNotificationChannelProfileID domain.NotificationChannelProfileID
	Items                              []ReportBatchStartItem
}

// WorkflowHandle is the provider-neutral handle returned after a
// workflow start request is accepted.
type WorkflowHandle struct {
	WorkflowID string
	RunID      string
}

// ReportWorkflowStarter starts the report batch orchestration without
// exposing a concrete workflow engine to usecases.
type ReportWorkflowStarter interface {
	StartReportBatch(ctx context.Context, req ReportBatchStartRequest) (WorkflowHandle, error)
}

// DiagnosisRoomStartRequest describes an idempotent request to start one
// short-conversation diagnosis room from a frozen EvidenceSnapshot.
type DiagnosisRoomStartRequest struct {
	SessionID          string
	EvidenceSnapshotID domain.EvidenceSnapshotID
	OwnerSubject       string
	Evidence           json.RawMessage
}

// DiagnosisRoomStartResult is returned once the room workflow has created its
// persistent task/session boundary and can accept authenticated WebSocket
// tickets.
type DiagnosisRoomStartResult struct {
	SessionID          string
	EvidenceSnapshotID domain.EvidenceSnapshotID
	DiagnosisTaskID    domain.DiagnosisTaskID
	ChatSessionID      domain.ChatSessionID
	Workflow           WorkflowHandle
}

// DiagnosisRoomWorkflowStarter starts a DiagnosisRoomWorkflow without exposing
// Temporal SDK types to HTTP or usecase packages.
type DiagnosisRoomWorkflowStarter interface {
	StartDiagnosisRoom(ctx context.Context, req DiagnosisRoomStartRequest) (DiagnosisRoomStartResult, error)
}

// DiagnosisRoomSubmitTurnRequest describes one user turn submitted from the
// authenticated diagnosis-room transport boundary.
type DiagnosisRoomSubmitTurnRequest struct {
	SessionID            string
	MessageID            string
	ActorSubject         string
	Message              string
	SupplementalEvidence *DiagnosisRoomSupplementalEvidence
}

// DiagnosisRoomSupplementalEvidence captures operator-provided evidence that
// answers a specific missing-evidence request or collection suggestion.
type DiagnosisRoomSupplementalEvidence struct {
	Label    string
	Detail   string
	Priority string
	Evidence string
}

// DiagnosisRoomConfirmConclusionRequest describes one explicit operator
// confirmation that should close the room and retain the final conclusion.
type DiagnosisRoomConfirmConclusionRequest struct {
	SessionID    string
	ActorSubject string
	Reason       string
}

// DiagnosisRoomSubmitTurnResult is the provider-neutral response returned
// after DiagnosisRoomWorkflow persists the user+assistant turn pair.
type DiagnosisRoomSubmitTurnResult struct {
	SessionID           string
	ChatSessionID       domain.ChatSessionID
	MessageID           string
	AssistantMessageID  string
	UserTurnID          domain.ChatTurnID
	AssistantTurnID     domain.ChatTurnID
	UserSequence        int
	AssistantSequence   int
	TurnCount           int
	ContextBytes        int
	Status              string
	AssistantMessage    string
	RequiresHumanReview bool
	Confidence          string
	EvidenceRequests    []DiagnosisRoomEvidenceRequest
	CollectionResults   []DiagnosisRoomEvidenceCollectionResult
	ConsultationInsight DiagnosisRoomConsultationInsight
	FollowUpTurns       []DiagnosisRoomFollowUpTurnResult
}

// DiagnosisRoomFollowUpTurnResult describes one workflow-triggered turn that
// ran after collecting evidence for the submitted operator turn.
type DiagnosisRoomFollowUpTurnResult struct {
	MessageID           string
	UserMessage         string
	AssistantMessageID  string
	UserTurnID          domain.ChatTurnID
	AssistantTurnID     domain.ChatTurnID
	UserSequence        int
	AssistantSequence   int
	TurnCount           int
	ContextBytes        int
	AssistantMessage    string
	RequiresHumanReview bool
	Confidence          string
	EvidenceRequests    []DiagnosisRoomEvidenceRequest
	CollectionResults   []DiagnosisRoomEvidenceCollectionResult
	ConsultationInsight DiagnosisRoomConsultationInsight
	Trigger             string
}

// DiagnosisRoomConversationTurn is the workflow-visible reconnect transcript
// shape returned by DiagnosisRoomWorkflow queries.
type DiagnosisRoomConversationTurn struct {
	Role    string
	Content string
}

// DiagnosisRoomEvidenceRequest captures one bounded, executable evidence
// collection plan returned by a diagnosis-room turn.
type DiagnosisRoomEvidenceRequest struct {
	TemplateID    domain.DiagnosisToolTemplateID
	Tool          domain.DiagnosisToolKind
	Reason        string
	Query         string
	WindowSeconds int
	StepSeconds   int
	Limit         int
}

// DiagnosisRoomEvidenceCollectionResult captures the provider-backed outcome
// for one executable diagnosis evidence request.
type DiagnosisRoomEvidenceCollectionResult struct {
	Request              DiagnosisRoomEvidenceRequest
	TemplateID           domain.DiagnosisToolTemplateID
	AlertSourceProfileID domain.AlertSourceProfileID
	AlertSourceKind      domain.AlertSourceKind
	Tool                 domain.DiagnosisToolKind
	Status               string
	ReasonCode           string
	Message              string
	Limit                int
	ObservedAlerts       int
	ActiveAlerts         []DiagnosisRoomActiveAlert
	Query                string
	WindowSeconds        int
	StepSeconds          int
	ObservedMetricSeries int
	MetricResult         DiagnosisRoomMetricQueryResult
	CollectedAt          time.Time
}

// DiagnosisRoomActiveAlert is the operator-facing active alert projection
// included in diagnosis evidence collection results.
type DiagnosisRoomActiveAlert struct {
	Source      string
	Labels      map[string]string
	Annotations map[string]string
	StartsAt    time.Time
}

// DiagnosisRoomMetricQueryResult is the operator-facing metric evidence
// summary included in diagnosis collection results.
type DiagnosisRoomMetricQueryResult struct {
	ResultType string
	Series     []DiagnosisRoomMetricSeries
	Scalar     *DiagnosisRoomMetricPoint
	String     *DiagnosisRoomMetricPoint
	Warnings   []string
}

// DiagnosisRoomMetricSeries is one Prometheus time series summary.
type DiagnosisRoomMetricSeries struct {
	Metric map[string]string
	Points []DiagnosisRoomMetricPoint
}

// DiagnosisRoomMetricPoint is one timestamped metric sample value.
type DiagnosisRoomMetricPoint struct {
	Timestamp time.Time
	Value     string
}

// DiagnosisRoomConsultationEvidenceRequest captures one human-readable
// evidence gap or collection hint returned by a diagnosis-room turn.
type DiagnosisRoomConsultationEvidenceRequest struct {
	Label    string
	Detail   string
	Priority string
}

// DiagnosisRoomConsultationInsight is the latest structured assistant
// confidence-lift state returned by a diagnosis-room turn.
type DiagnosisRoomConsultationInsight struct {
	ConfidenceRationale           string
	MissingEvidenceRequests       []DiagnosisRoomConsultationEvidenceRequest
	EvidenceCollectionSuggestions []DiagnosisRoomConsultationEvidenceRequest
	ConclusionStatus              string
}

// DiagnosisRoomFinalConclusion is the close-time conclusion snapshot returned
// for read/reconnect flows after the room has closed.
type DiagnosisRoomFinalConclusion struct {
	Status                  string
	Source                  string
	Reason                  string
	EvidenceSnapshotID      domain.EvidenceSnapshotID
	ConclusionVersion       string
	RecordedAt              *time.Time
	ConfirmedBy             string
	SupplementalContextRefs []string
	AssistantTurnID         domain.ChatTurnID
	AssistantMessageID      string
	AssistantSequence       int
	AssistantOccurredAt     *time.Time
	Content                 string
	Confidence              string
	RequiresHumanReview     *bool
}

// DiagnosisRoomState is the provider-neutral room state returned for
// WebSocket reconnect/read flows.
type DiagnosisRoomState struct {
	SessionID                 string
	ChatSessionID             domain.ChatSessionID
	DiagnosisTaskID           domain.DiagnosisTaskID
	OwnerSubject              string
	Status                    string
	TurnCount                 int
	StartedAt                 time.Time
	LastActivityAt            time.Time
	ClosedAt                  *time.Time
	CloseReason               string
	FinalConclusion           *DiagnosisRoomFinalConclusion
	LatestConsultationInsight *DiagnosisRoomConsultationInsight
	LatestConfidence          string
	LatestRequiresHumanReview *bool
	InFlight                  bool
	SeenMessageIDs            []string
	Conversation              []DiagnosisRoomConversationTurn
}

// DiagnosisRoomWorkflowClient submits user turns and queries room state
// without exposing the concrete workflow engine to the transport layer.
type DiagnosisRoomWorkflowClient interface {
	SubmitDiagnosisTurn(ctx context.Context, req DiagnosisRoomSubmitTurnRequest) (DiagnosisRoomSubmitTurnResult, error)
	ConfirmDiagnosisConclusion(ctx context.Context, req DiagnosisRoomConfirmConclusionRequest) (DiagnosisRoomState, error)
	QueryDiagnosisRoom(ctx context.Context, sessionID string) (DiagnosisRoomState, error)
}
