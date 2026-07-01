package diagnosisroomclose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceCloseUnavailableClosesOpenRoom(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
	closedAt := startedAt.Add(5 * time.Minute)
	repo := newCloseTestRepo(t, startedAt)
	service := mustCloseService(t, repo, &closeVisibility{
		items: map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility{
			{WorkflowID: "diagnosis-room-session-1", RunID: "run-1"}: {
				WorkflowID:    "diagnosis-room-session-1",
				RunID:         "run-1",
				Status:        "not_found",
				HistoryLength: 12,
			},
		},
	})

	got, err := service.CloseUnavailable(context.Background(), Request{
		SessionID: "session-1",
		Principal: ports.AuthPrincipal{
			Subject: "room-admin-1",
		},
		Now: closedAt,
	})
	if err != nil {
		t.Fatalf("CloseUnavailable: %v", err)
	}
	if got.Session.Status != domain.ChatSessionStatusClosed ||
		got.Session.CloseReason != DefaultUnavailableCloseReason ||
		got.Session.ClosedAt == nil ||
		!got.Session.ClosedAt.Equal(domain.NormalizeUTCMicro(closedAt)) {
		t.Fatalf("closed session = %+v", got.Session)
	}
	if got.Task.Status != domain.DiagnosisStatusCancelled ||
		got.Task.FinishedAt == nil ||
		!got.Task.FinishedAt.Equal(domain.NormalizeUTCMicro(closedAt)) {
		t.Fatalf("finished task = %+v", got.Task)
	}
	if len(repo.events) != 1 {
		t.Fatalf("events len = %d, want 1", len(repo.events))
	}
	event := repo.events[0]
	if event.Kind != diagnosisRoomClosedEventKind || event.DedupeKey == nil || *event.DedupeKey == "" {
		t.Fatalf("event = %+v", event)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("event payload: %v", err)
	}
	if payload["source"] != "DiagnosisRoomCloseUnavailable" ||
		payload["workflow_status"] != "not_found" ||
		payload["actor_subject"] != "room-admin-1" ||
		payload["closed_by"] != "room-admin-1" ||
		payload["close_reason"] != DefaultUnavailableCloseReason {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestServiceCloseUnavailableRejectsRunningWorkflow(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
	repo := newCloseTestRepo(t, startedAt)
	service := mustCloseService(t, repo, &closeVisibility{
		items: map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility{
			{WorkflowID: "diagnosis-room-session-1", RunID: "run-1"}: {
				Status: "running",
			},
		},
	})

	_, err := service.CloseUnavailable(context.Background(), Request{
		SessionID: "session-1",
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
		Now: startedAt.Add(time.Minute),
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("CloseUnavailable error = %v, want ErrPreconditionFailed", err)
	}
	if repo.session.Status != domain.ChatSessionStatusOpen || len(repo.events) != 0 {
		t.Fatalf("repo mutated on rejected close: session=%+v events=%d", repo.session, len(repo.events))
	}
}

func TestServiceCloseUnavailableRejectsUnauthenticatedPrincipal(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
	repo := newCloseTestRepo(t, startedAt)
	service := mustCloseService(t, repo, &closeVisibility{
		items: map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility{},
	})

	_, err := service.CloseUnavailable(context.Background(), Request{
		SessionID: "session-1",
		Principal: ports.AuthPrincipal{
			Subject: " ",
		},
		Now: startedAt.Add(time.Minute),
	})
	if !errors.Is(err, diagnosisauth.ErrUnauthenticated) {
		t.Fatalf("CloseUnavailable error = %v, want ErrUnauthenticated", err)
	}
}

func TestServiceCloseUnavailableRejectsMissingNow(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
	repo := newCloseTestRepo(t, startedAt)
	service := mustCloseService(t, repo, &closeVisibility{})

	_, err := service.CloseUnavailable(context.Background(), Request{
		SessionID: "session-1",
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("CloseUnavailable error = %v, want ErrInvariantViolation", err)
	}
}

func TestServiceCloseUnavailableReturnsClosedRoomIdempotently(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
	closedAt := startedAt.Add(time.Minute)
	repo := newCloseTestRepo(t, startedAt)
	closed, err := repo.session.Close(closedAt, DefaultUnavailableCloseReason)
	if err != nil {
		t.Fatalf("seed close: %v", err)
	}
	repo.session = closed
	repo.task, err = repo.task.Finish(domain.DiagnosisStatusCancelled, closedAt, "")
	if err != nil {
		t.Fatalf("seed finish: %v", err)
	}
	service := mustCloseService(t, repo, &closeVisibility{})

	got, err := service.CloseUnavailable(context.Background(), Request{
		SessionID: "session-1",
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
		Now: closedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("CloseUnavailable: %v", err)
	}
	if got.Session.CloseReason != DefaultUnavailableCloseReason || len(repo.events) != 0 {
		t.Fatalf("idempotent result = %+v events=%d", got.Session, len(repo.events))
	}
}

func mustCloseService(t *testing.T, repo *closeTestRepo, visibility ports.DiagnosisRoomWorkflowVisibilityLookup) *Service {
	t.Helper()
	service, err := NewService(closeFactory{repo: repo}, visibility)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func newCloseTestRepo(t *testing.T, startedAt time.Time) *closeTestRepo {
	t.Helper()
	startedAt = domain.NormalizeUTCMicro(startedAt)
	return &closeTestRepo{
		t: t,
		session: domain.ChatSession{
			ID:              11,
			DiagnosisTaskID: 21,
			SessionKey:      "session-1",
			OwnerSubject:    "owner-1",
			Status:          domain.ChatSessionStatusOpen,
			TurnCount:       1,
			StartedAt:       startedAt,
			LastActivityAt:  startedAt.Add(time.Minute),
			CreatedAt:       startedAt,
			UpdatedAt:       startedAt,
		},
		task: domain.DiagnosisTask{
			ID:                 21,
			EvidenceSnapshotID: 31,
			WorkflowID:         "diagnosis-room-session-1",
			RunID:              "run-1",
			Status:             domain.DiagnosisStatusRunning,
			StartedAt:          &startedAt,
			CreatedAt:          startedAt,
			UpdatedAt:          startedAt,
		},
	}
}

type closeVisibility struct {
	items map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility
	err   error
}

func (v *closeVisibility) ListDiagnosisRoomWorkflowVisibility(
	context.Context,
	[]ports.DiagnosisRoomWorkflowVisibilityRequest,
) (map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility, error) {
	if v.err != nil {
		return nil, v.err
	}
	return v.items, nil
}

type closeFactory struct {
	repo *closeTestRepo
}

func (f closeFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, errors.New("unexpected Begin call")
}

func (f closeFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, closeUOW{repo: f.repo})
}

type closeUOW struct {
	repo *closeTestRepo
}

func (u closeUOW) Alerts() ports.AlertRepository {
	u.repo.failNow("unexpected Alerts call")
	return nil
}
func (u closeUOW) Evidence() ports.EvidenceRepository {
	u.repo.failNow("unexpected Evidence call")
	return nil
}
func (u closeUOW) Diagnosis() ports.DiagnosisRepository { return u.repo }
func (u closeUOW) Reports() ports.ReportRepository {
	u.repo.failNow("unexpected Reports call")
	return nil
}
func (u closeUOW) Config() ports.ConfigurationRepository {
	u.repo.failNow("unexpected Config call")
	return nil
}
func (u closeUOW) Directory() ports.DirectoryRepository {
	u.repo.failNow("unexpected Directory call")
	return nil
}
func (u closeUOW) RBAC() ports.RBACRepository {
	u.repo.failNow("unexpected RBAC call")
	return nil
}
func (u closeUOW) Commit(context.Context) error   { return errors.New("unexpected Commit call") }
func (u closeUOW) Rollback(context.Context) error { return errors.New("unexpected Rollback call") }

type closeTestRepo struct {
	t       *testing.T
	session domain.ChatSession
	task    domain.DiagnosisTask
	events  []domain.DiagnosisTaskEvent
}

func (r *closeTestRepo) SaveTask(context.Context, domain.DiagnosisTask) (domain.DiagnosisTask, error) {
	return domain.DiagnosisTask{}, errors.New("unexpected SaveTask call")
}

func (r *closeTestRepo) UpdateTask(_ context.Context, task domain.DiagnosisTask) (domain.DiagnosisTask, error) {
	r.task = task
	return r.task, nil
}

func (r *closeTestRepo) FindTaskByID(_ context.Context, id domain.DiagnosisTaskID) (domain.DiagnosisTask, error) {
	if r.task.ID != id {
		return domain.DiagnosisTask{}, domain.ErrNotFound
	}
	return r.task, nil
}

func (r *closeTestRepo) FindTaskByExecution(context.Context, string, string) (domain.DiagnosisTask, error) {
	return domain.DiagnosisTask{}, errors.New("unexpected FindTaskByExecution call")
}

func (r *closeTestRepo) ListTasksByEvidenceSnapshot(context.Context, domain.EvidenceSnapshotID, int) ([]domain.DiagnosisTask, error) {
	return nil, errors.New("unexpected ListTasksByEvidenceSnapshot call")
}

func (r *closeTestRepo) AppendEvent(_ context.Context, event domain.DiagnosisTaskEvent) (domain.DiagnosisTaskEvent, error) {
	event.ID = domain.DiagnosisTaskEventID(len(r.events) + 1)
	r.events = append(r.events, event)
	return event, nil
}

func (r *closeTestRepo) FindEventByTaskAndDedupeKey(context.Context, domain.DiagnosisTaskID, string) (domain.DiagnosisTaskEvent, error) {
	return domain.DiagnosisTaskEvent{}, errors.New("unexpected FindEventByTaskAndDedupeKey call")
}

func (r *closeTestRepo) ListEvents(context.Context, domain.DiagnosisTaskID, int) ([]domain.DiagnosisTaskEvent, error) {
	return nil, errors.New("unexpected ListEvents call")
}

func (r *closeTestRepo) ListEventsByTaskAndKind(context.Context, domain.DiagnosisTaskID, string, int) ([]domain.DiagnosisTaskEvent, error) {
	return nil, errors.New("unexpected ListEventsByTaskAndKind call")
}

func (r *closeTestRepo) SaveChatSession(context.Context, domain.ChatSession) (domain.ChatSession, error) {
	return domain.ChatSession{}, errors.New("unexpected SaveChatSession call")
}

func (r *closeTestRepo) UpdateChatSession(_ context.Context, session domain.ChatSession) (domain.ChatSession, error) {
	r.session = session
	return r.session, nil
}

func (r *closeTestRepo) FindChatSessionByID(context.Context, domain.ChatSessionID) (domain.ChatSession, error) {
	return domain.ChatSession{}, errors.New("unexpected FindChatSessionByID call")
}

func (r *closeTestRepo) FindChatSessionByKey(_ context.Context, sessionKey string) (domain.ChatSession, error) {
	if r.session.SessionKey != sessionKey {
		return domain.ChatSession{}, domain.ErrNotFound
	}
	return r.session, nil
}

func (r *closeTestRepo) ListChatSessions(context.Context, int) ([]domain.ChatSessionWithTask, error) {
	return nil, errors.New("unexpected ListChatSessions call")
}

func (r *closeTestRepo) SaveChatTurn(context.Context, domain.ChatTurn) (domain.ChatTurn, error) {
	return domain.ChatTurn{}, errors.New("unexpected SaveChatTurn call")
}

func (r *closeTestRepo) FindChatTurnBySessionAndMessageID(context.Context, domain.ChatSessionID, string) (domain.ChatTurn, error) {
	return domain.ChatTurn{}, errors.New("unexpected FindChatTurnBySessionAndMessageID call")
}

func (r *closeTestRepo) ListChatTurnsBySession(context.Context, domain.ChatSessionID, int) ([]domain.ChatTurn, error) {
	return nil, errors.New("unexpected ListChatTurnsBySession call")
}

func (r *closeTestRepo) failNow(message string) {
	r.t.Helper()
	r.t.Fatal(message)
}
