// Package diagnosisroomclose owns operator-driven local closure of diagnosis
// rooms whose workflow execution can no longer be used.
package diagnosisroomclose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// DefaultUnavailableCloseReason is the audit reason used when an operator
	// locally closes a room whose workflow execution is no longer usable.
	DefaultUnavailableCloseReason = "workflow_unavailable"

	diagnosisRoomClosedEventKind = "diagnosis_room.closed"
	maxCloseReasonBytes          = 120
)

var closeReasonPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

// Request identifies one diagnosis room to close after backend workflow
// visibility confirms that its workflow execution is unavailable. Callers are
// responsible for authorizing the principal through OpenClarion-local RBAC
// before invoking this use case.
type Request struct {
	SessionID string
	Principal ports.AuthPrincipal
	Reason    string
	Now       time.Time
}

// Result returns the updated local room lifecycle and backing task.
type Result struct {
	Session domain.ChatSession
	Task    domain.DiagnosisTask
}

// Service closes local diagnosis-room lifecycle rows without depending on
// concrete persistence or workflow SDK implementations.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	visibility ports.DiagnosisRoomWorkflowVisibilityLookup
}

// NewService constructs a diagnosis-room close service.
func NewService(
	uowFactory ports.UnitOfWorkFactory,
	visibility ports.DiagnosisRoomWorkflowVisibilityLookup,
) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("diagnosis room close: unit of work factory is required: %w", domain.ErrInvariantViolation)
	}
	if visibility == nil {
		return nil, fmt.Errorf("diagnosis room close: workflow visibility lookup is required: %w", domain.ErrInvariantViolation)
	}
	return &Service{uowFactory: uowFactory, visibility: visibility}, nil
}

// CloseUnavailable verifies the caller identity and workflow visibility before
// closing an open local room row. It refuses to close rooms whose workflow is
// still usable.
func (s *Service) CloseUnavailable(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.uowFactory == nil || s.visibility == nil {
		return Result{}, fmt.Errorf("diagnosis room close: service is not configured: %w", domain.ErrInvariantViolation)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return Result{}, fmt.Errorf("diagnosis room close: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	reason, err := closeReasonOrDefault(req.Reason)
	if err != nil {
		return Result{}, err
	}
	closedAt := req.Now.UTC()
	if closedAt.IsZero() {
		return Result{}, fmt.Errorf("diagnosis room close: now must be non-zero: %w", domain.ErrInvariantViolation)
	}
	principalSubject := strings.TrimSpace(req.Principal.Subject)
	if principalSubject == "" {
		return Result{}, fmt.Errorf("diagnosis room close: principal subject is required: %w", diagnosisauth.ErrUnauthenticated)
	}

	session, task, err := s.loadRoom(ctx, sessionID)
	if err != nil {
		return Result{}, err
	}
	if session.Status == domain.ChatSessionStatusClosed {
		return Result{Session: session, Task: task}, nil
	}

	visibility, err := s.workflowVisibility(ctx, task)
	if err != nil {
		return Result{}, err
	}
	if !workflowVisibilityUnavailable(visibility.Status) {
		return Result{}, fmt.Errorf("diagnosis room close: workflow is still %q: %w", visibility.Status, domain.ErrPreconditionFailed)
	}

	var result Result
	err = s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		latestSession, err := uow.Diagnosis().FindChatSessionByKey(ctx, sessionID)
		if err != nil {
			return err
		}
		latestTask, err := uow.Diagnosis().FindTaskByID(ctx, task.ID)
		if err != nil {
			return err
		}
		if latestSession.Status == domain.ChatSessionStatusClosed {
			result = Result{Session: latestSession, Task: latestTask}
			return nil
		}
		finishedTask, err := latestTask.Finish(domain.DiagnosisStatusCancelled, closedAt, "")
		if err != nil {
			return err
		}
		savedTask, err := uow.Diagnosis().UpdateTask(ctx, finishedTask)
		if err != nil {
			return err
		}
		closedSession, err := latestSession.Close(closedAt, reason)
		if err != nil {
			return err
		}
		savedSession, err := uow.Diagnosis().UpdateChatSession(ctx, closedSession)
		if err != nil {
			return err
		}
		if err := appendClosedEvent(ctx, uow.Diagnosis(), savedSession, savedTask, visibility, principalSubject); err != nil {
			return err
		}
		result = Result{Session: savedSession, Task: savedTask}
		return nil
	})
	return result, err
}

func (s *Service) loadRoom(ctx context.Context, sessionID string) (domain.ChatSession, domain.DiagnosisTask, error) {
	var session domain.ChatSession
	var task domain.DiagnosisTask
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		session, err = uow.Diagnosis().FindChatSessionByKey(ctx, sessionID)
		if err != nil {
			return err
		}
		task, err = uow.Diagnosis().FindTaskByID(ctx, session.DiagnosisTaskID)
		return err
	})
	return session, task, err
}

func (s *Service) workflowVisibility(ctx context.Context, task domain.DiagnosisTask) (ports.DiagnosisRoomWorkflowVisibility, error) {
	req := ports.DiagnosisRoomWorkflowVisibilityRequest{
		WorkflowID: task.WorkflowID,
		RunID:      task.RunID,
	}
	items, err := s.visibility.ListDiagnosisRoomWorkflowVisibility(ctx, []ports.DiagnosisRoomWorkflowVisibilityRequest{req})
	if err != nil {
		return ports.DiagnosisRoomWorkflowVisibility{}, err
	}
	visibility, ok := items[req]
	if !ok {
		return ports.DiagnosisRoomWorkflowVisibility{}, fmt.Errorf("diagnosis room close: workflow visibility missing for %s/%s: %w", task.WorkflowID, task.RunID, domain.ErrPreconditionFailed)
	}
	return visibility, nil
}

func closeReasonOrDefault(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = DefaultUnavailableCloseReason
	}
	if len([]byte(reason)) > maxCloseReasonBytes || !closeReasonPattern.MatchString(reason) {
		return "", fmt.Errorf("diagnosis room close: reason must match %s and be at most %d bytes: %w", closeReasonPattern.String(), maxCloseReasonBytes, domain.ErrInvariantViolation)
	}
	return reason, nil
}

func workflowVisibilityUnavailable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "not_found", "completed", "failed", "canceled", "cancelled", "terminated", "timed_out", "continued_as_new":
		return true
	default:
		return false
	}
}

func appendClosedEvent(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	visibility ports.DiagnosisRoomWorkflowVisibility,
	actorSubject string,
) error {
	payload, err := json.Marshal(map[string]any{
		"source":                  "DiagnosisRoomCloseUnavailable",
		"kind":                    diagnosisRoomClosedEventKind,
		"session_id":              session.SessionKey,
		"chat_session_id":         int64(session.ID),
		"diagnosis_task_id":       int64(task.ID),
		"evidence_snapshot_id":    int64(task.EvidenceSnapshotID),
		"owner_subject":           session.OwnerSubject,
		"actor_subject":           actorSubject,
		"closed_by":               actorSubject,
		"status":                  string(session.Status),
		"turn_count":              session.TurnCount,
		"close_reason":            session.CloseReason,
		"closed_at":               session.ClosedAt,
		"workflow_id":             task.WorkflowID,
		"run_id":                  task.RunID,
		"workflow_status":         visibility.Status,
		"diagnosis_task_status":   string(task.Status),
		"diagnosis_task_closed":   task.FinishedAt,
		"workflow_history_length": visibility.HistoryLength,
	})
	if err != nil {
		return fmt.Errorf("diagnosis room close: marshal lifecycle event: %w", err)
	}
	dedupeKey := closedEventDedupeKey(session.SessionKey)
	event, err := domain.NewDiagnosisTaskEvent(
		task.ID,
		diagnosisRoomClosedEventKind,
		payload,
		&dedupeKey,
		*session.ClosedAt,
	)
	if err != nil {
		return err
	}
	if _, err := repo.AppendEvent(ctx, event); err != nil {
		if !errors.Is(err, domain.ErrAlreadyExists) {
			return err
		}
	}
	return nil
}

func closedEventDedupeKey(sessionID string) string {
	sum := sha256.Sum256([]byte(diagnosisRoomClosedEventKind + "\x00" + sessionID + "\x00unavailable"))
	return "dr:close:" + hex.EncodeToString(sum[:])[:24]
}
