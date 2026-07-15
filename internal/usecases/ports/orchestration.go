package ports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

// DiagnosisTurnStreamPhase identifies a transient, non-durable preview event.
type DiagnosisTurnStreamPhase string

const (
	// DiagnosisTurnStreamStarted resets transient preview state for one Activity attempt.
	DiagnosisTurnStreamStarted DiagnosisTurnStreamPhase = "started"
	// DiagnosisTurnStreamReset clears a stale draft when model validation retries.
	DiagnosisTurnStreamReset DiagnosisTurnStreamPhase = "reset"
	// DiagnosisTurnStreamDelta replaces the preview with a validated text snapshot.
	DiagnosisTurnStreamDelta DiagnosisTurnStreamPhase = "delta"
)

// DiagnosisTurnStreamEvent is an in-process preview snapshot. It intentionally
// stays outside Workflow state and persistence; the validated turn_result is
// the only authoritative assistant response.
type DiagnosisTurnStreamEvent struct {
	Phase              DiagnosisTurnStreamPhase
	SessionID          string
	MessageID          string
	AssistantMessageID string
	ActivityAttempt    int
	GenerationAttempt  int
	Sequence           int
	AssistantMessage   string
}

// DiagnosisTurnStreamSink accepts transient preview snapshots from Activities.
type DiagnosisTurnStreamSink interface {
	PublishDiagnosisTurnStream(DiagnosisTurnStreamEvent)
}

// DiagnosisTurnStreamSource subscribes a WebSocket relay before it starts the
// corresponding Workflow Update. cancel must always be called by the relay.
type DiagnosisTurnStreamSource interface {
	SubscribeDiagnosisTurnStream(sessionID, messageID string) (<-chan DiagnosisTurnStreamEvent, func())
}

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

// DiagnosisRoomInitialTurnRequest describes an automatic first turn that the
// room workflow should run after creating its durable session boundary.
type DiagnosisRoomInitialTurnRequest struct {
	MessageID    string
	ActorSubject string
	Message      string
}

// ReportWorkflowStarter starts the report batch orchestration without
// exposing a concrete workflow engine to usecases.
type ReportWorkflowStarter interface {
	StartReportBatch(ctx context.Context, req ReportBatchStartRequest) (WorkflowHandle, error)
}

// DiagnosisRoomStartRequest describes an idempotent request to start one
// short-conversation diagnosis room from a frozen EvidenceSnapshot.
type DiagnosisRoomStartRequest struct {
	SessionID                         string
	EvidenceSnapshotID                domain.EvidenceSnapshotID
	OwnerSubject                      string
	Evidence                          json.RawMessage
	CloseNotificationChannelProfileID domain.NotificationChannelProfileID
	ApprovalMode                      domain.DiagnosisApprovalMode
	InitialTurn                       *DiagnosisRoomInitialTurnRequest
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
	ApprovalMode       domain.DiagnosisApprovalMode
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

// DiagnosisRoomCollectEvidenceRequest asks the room workflow to execute one
// operator-selected evidence plan and reassess the diagnosis with the results.
type DiagnosisRoomCollectEvidenceRequest struct {
	SessionID    string
	MessageID    string
	ActorSubject string
	Message      string
	Requests     []DiagnosisRoomEvidenceRequest
}

// DiagnosisRoomCollectEvidenceResult returns the updated room state plus any
// automatic AI reassessment turns triggered by the just-collected evidence.
type DiagnosisRoomCollectEvidenceResult struct {
	State         DiagnosisRoomState
	FollowUpTurns []DiagnosisRoomFollowUpTurnResult
}

// DiagnosisRoomSupplementalEvidence captures operator-provided evidence that
// answers a specific missing-evidence request or collection suggestion.
type DiagnosisRoomSupplementalEvidence struct {
	Label    string
	Detail   string
	Priority string
	Evidence string
}

// DiagnosisRoomSupplementalEvidenceRecord captures one accepted supplemental
// evidence update plus the persisted conversation turn metadata that caused it.
type DiagnosisRoomSupplementalEvidenceRecord struct {
	Label              string
	Detail             string
	Priority           string
	Evidence           string
	ActorSubject       string
	UserMessageID      string
	AssistantMessageID string
	UserTurnID         domain.ChatTurnID
	AssistantTurnID    domain.ChatTurnID
	UserSequence       int
	AssistantSequence  int
	ProvidedAt         time.Time
}

// DiagnosisRoomConfirmConclusionRequest describes one explicit operator
// confirmation that should close the room and retain the final conclusion.
type DiagnosisRoomConfirmConclusionRequest struct {
	SessionID    string
	ActorSubject string
	Reason       string
}

// DiagnosisRoomCloseRequest describes one explicit operator request to close
// a room without confirming its latest conclusion.
type DiagnosisRoomCloseRequest struct {
	SessionID    string
	ActorSubject string
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
	RetrievalRefs       []string
	EvidenceRequests    []DiagnosisRoomEvidenceRequest
	CollectionResults   []DiagnosisRoomEvidenceCollectionResult
	EvidenceTimeline    []DiagnosisRoomEvidenceTimelineEntry
	ConfidenceTimeline  []DiagnosisRoomConfidenceTimelineEntry
	ConsultationInsight DiagnosisRoomConsultationInsight
	FollowUpTurns       []DiagnosisRoomFollowUpTurnResult
	LatestError         *DiagnosisRoomLatestError
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
	RetrievalRefs       []string
	EvidenceRequests    []DiagnosisRoomEvidenceRequest
	CollectionResults   []DiagnosisRoomEvidenceCollectionResult
	ConsultationInsight DiagnosisRoomConsultationInsight
	Trigger             string
}

// DiagnosisRoomEvidenceTimelineEntry records one concrete evidence collection
// cycle so reconnect/read flows do not have to infer evidence history from the
// latest assistant turn.
type DiagnosisRoomEvidenceTimelineEntry struct {
	TurnCount          int
	MessageID          string
	AssistantMessageID string
	ActorSubject       string
	Trigger            string
	EvidenceRequests   []DiagnosisRoomEvidenceRequest
	CollectionResults  []DiagnosisRoomEvidenceCollectionResult
}

// DiagnosisRoomConfidenceTimelineEntry records one assistant confidence
// checkpoint for reconnect/read flows.
type DiagnosisRoomConfidenceTimelineEntry struct {
	TurnCount                     int
	MessageID                     string
	AssistantMessageID            string
	AssistantTurnID               domain.ChatTurnID
	AssistantSequence             int
	OccurredAt                    time.Time
	Trigger                       string
	Confidence                    string
	RequiresHumanReview           bool
	ConclusionStatus              string
	ConfidenceRationale           string
	ContextBytes                  int
	RetrievalRefs                 []string
	EvidenceRequests              []DiagnosisRoomEvidenceRequest
	CollectionResults             []DiagnosisRoomEvidenceCollectionResult
	MissingEvidenceRequests       []DiagnosisRoomConsultationEvidenceRequest
	EvidenceCollectionSuggestions []DiagnosisRoomConsultationEvidenceRequest
}

// DiagnosisRoomLatestError is the last operator-visible diagnosis-room
// failure retained for reconnect/read flows.
type DiagnosisRoomLatestError struct {
	Code       string
	Message    string
	MessageID  string
	OccurredAt time.Time
}

// DiagnosisRoomConversationTurn is the workflow-visible reconnect transcript
// shape returned by DiagnosisRoomWorkflow queries.
type DiagnosisRoomConversationTurn struct {
	Role         string
	ActorSubject string
	Content      string
}

// DiagnosisRoomEvidenceRequest captures one bounded, executable evidence
// collection plan returned by a diagnosis-room turn.
type DiagnosisRoomEvidenceRequest struct {
	TemplateID           domain.DiagnosisToolTemplateID
	AlertSourceProfileID domain.AlertSourceProfileID
	Tool                 domain.DiagnosisToolKind
	Reason               string
	Query                string
	WindowSeconds        int
	StepSeconds          int
	Limit                int
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
	Source               string
	AlertSourceProfileID domain.AlertSourceProfileID
	Labels               map[string]string
	Annotations          map[string]string
	StartsAt             time.Time
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
	Status                        string
	Source                        string
	Reason                        string
	EvidenceSnapshotID            domain.EvidenceSnapshotID
	ConclusionVersion             string
	RecordedAt                    *time.Time
	ConfirmedBy                   string
	SupplementalContextRefs       []string
	AssistantTurnID               domain.ChatTurnID
	AssistantMessageID            string
	AssistantSequence             int
	AssistantOccurredAt           *time.Time
	Content                       string
	Confidence                    string
	RequiresHumanReview           *bool
	ConfidenceRationale           string
	Findings                      []string
	RecommendedActions            []string
	EvidenceRequests              []DiagnosisRoomEvidenceRequest
	MissingEvidenceRequests       []DiagnosisRoomConsultationEvidenceRequest
	EvidenceCollectionSuggestions []DiagnosisRoomConsultationEvidenceRequest
}

// DiagnosisRoomConclusionApproval is the provider-neutral read model for one
// immutable approval bound to the currently retained conclusion digest.
type DiagnosisRoomConclusionApproval struct {
	ID               domain.ChatSessionApprovalID
	ConclusionDigest string
	ActorSubject     string
	Authority        domain.DiagnosisApprovalAuthority
	Reason           string
	ApprovedAt       time.Time
}

// DiagnosisRoomState is the provider-neutral room state returned for
// WebSocket reconnect/read flows.
type DiagnosisRoomState struct {
	SessionID                  string
	ChatSessionID              domain.ChatSessionID
	DiagnosisTaskID            domain.DiagnosisTaskID
	OwnerSubject               string
	Status                     string
	TurnCount                  int
	StartedAt                  time.Time
	LastActivityAt             time.Time
	ClosedAt                   *time.Time
	CloseReason                string
	FinalConclusion            *DiagnosisRoomFinalConclusion
	ConversationSummary        *DiagnosisRoomConversationSummary
	ApprovalMode               domain.DiagnosisApprovalMode
	ConclusionDigest           string
	Approvals                  []DiagnosisRoomConclusionApproval
	PendingApprovalAuthorities []domain.DiagnosisApprovalAuthority
	ApprovalInFlight           bool
	LatestConsultationInsight  *DiagnosisRoomConsultationInsight
	LatestConfidence           string
	LatestRequiresHumanReview  *bool
	LatestEvidenceRequests     []DiagnosisRoomEvidenceRequest
	LatestCollectionResults    []DiagnosisRoomEvidenceCollectionResult
	EvidenceTimeline           []DiagnosisRoomEvidenceTimelineEntry
	ConfidenceTimeline         []DiagnosisRoomConfidenceTimelineEntry
	SupplementalEvidence       []DiagnosisRoomSupplementalEvidenceRecord
	LatestError                *DiagnosisRoomLatestError
	InFlight                   bool
	SeenMessageIDs             []string
	Conversation               []DiagnosisRoomConversationTurn
}

// DiagnosisRoomConversationSummary is the provider-neutral read model for an
// immutable lifecycle-end transcript compression checkpoint.
type DiagnosisRoomConversationSummary struct {
	ID                  domain.ChatSessionSummaryID
	Version             int
	SchemaVersion       string
	SourceFirstSequence int
	SourceLastSequence  int
	SourceTurnCount     int
	SourceDigest        string
	Content             json.RawMessage
	GeneratedAt         time.Time
}

// DiagnosisRoomWorkflowClient submits user turns and queries room state
// without exposing the concrete workflow engine to the transport layer.
type DiagnosisRoomWorkflowClient interface {
	SubmitDiagnosisTurn(ctx context.Context, req DiagnosisRoomSubmitTurnRequest) (DiagnosisRoomSubmitTurnResult, error)
	CollectDiagnosisEvidence(ctx context.Context, req DiagnosisRoomCollectEvidenceRequest) (DiagnosisRoomCollectEvidenceResult, error)
	ConfirmDiagnosisConclusion(ctx context.Context, req DiagnosisRoomConfirmConclusionRequest) (DiagnosisRoomState, error)
	QueryDiagnosisRoom(ctx context.Context, sessionID string) (DiagnosisRoomState, error)
}

// DiagnosisRoomWorkflowCloser closes an existing room without expanding the
// submit/query client contract for consumers that do not expose lifecycle
// administration.
type DiagnosisRoomWorkflowCloser interface {
	CloseDiagnosisRoom(ctx context.Context, req DiagnosisRoomCloseRequest) (DiagnosisRoomState, error)
}

// DiagnosisRoomWorkflowVisibilityRequest identifies one room workflow execution
// whose execution metadata should be surfaced to operators.
type DiagnosisRoomWorkflowVisibilityRequest struct {
	WorkflowID string
	RunID      string
}

// DiagnosisRoomWorkflowVisibility is a sanitized execution-metadata snapshot.
// It intentionally excludes workflow input, memo, search attributes, and
// payload-bearing history data.
type DiagnosisRoomWorkflowVisibility struct {
	WorkflowID       string
	RunID            string
	Status           string
	TaskQueue        string
	StartTime        *time.Time
	ExecutionTime    *time.Time
	CloseTime        *time.Time
	HistoryLength    int64
	HistorySizeBytes int64
}

// DiagnosisRoomWorkflowVisibilityLookup reads sanitized workflow execution
// metadata for diagnosis-room list surfaces.
type DiagnosisRoomWorkflowVisibilityLookup interface {
	ListDiagnosisRoomWorkflowVisibility(
		ctx context.Context,
		requests []DiagnosisRoomWorkflowVisibilityRequest,
	) (map[DiagnosisRoomWorkflowVisibilityRequest]DiagnosisRoomWorkflowVisibility, error)
}
