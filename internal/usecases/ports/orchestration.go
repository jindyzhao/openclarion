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
	SessionID    string
	MessageID    string
	ActorSubject string
	Message      string
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
}

// DiagnosisRoomConversationTurn is the workflow-visible reconnect transcript
// shape returned by DiagnosisRoomWorkflow queries.
type DiagnosisRoomConversationTurn struct {
	Role    string
	Content string
}

// DiagnosisRoomFinalConclusion is the close-time conclusion snapshot returned
// for read/reconnect flows after the room has closed.
type DiagnosisRoomFinalConclusion struct {
	Status              string
	Source              string
	Reason              string
	AssistantTurnID     domain.ChatTurnID
	AssistantMessageID  string
	AssistantSequence   int
	AssistantOccurredAt *time.Time
	Content             string
	Confidence          string
	RequiresHumanReview *bool
}

// DiagnosisRoomState is the provider-neutral room state returned for
// WebSocket reconnect/read flows.
type DiagnosisRoomState struct {
	SessionID       string
	ChatSessionID   domain.ChatSessionID
	DiagnosisTaskID domain.DiagnosisTaskID
	OwnerSubject    string
	Status          string
	TurnCount       int
	StartedAt       time.Time
	LastActivityAt  time.Time
	ClosedAt        *time.Time
	CloseReason     string
	FinalConclusion *DiagnosisRoomFinalConclusion
	InFlight        bool
	SeenMessageIDs  []string
	Conversation    []DiagnosisRoomConversationTurn
}

// DiagnosisRoomWorkflowClient submits user turns and queries room state
// without exposing the concrete workflow engine to the transport layer.
type DiagnosisRoomWorkflowClient interface {
	SubmitDiagnosisTurn(ctx context.Context, req DiagnosisRoomSubmitTurnRequest) (DiagnosisRoomSubmitTurnResult, error)
	QueryDiagnosisRoom(ctx context.Context, sessionID string) (DiagnosisRoomState, error)
}
