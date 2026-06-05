package diagnosisroomstart

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceStartLoadsEvidenceAndStartsWorkflow(t *testing.T) {
	snapshot := domain.EvidenceSnapshot{
		ID:            42,
		Payload:       json.RawMessage(`{"alert":"cpu"}`),
		Status:        domain.SnapshotStatusComplete,
		AlertGroupID:  7,
		Digest:        "digest",
		Provenance:    json.RawMessage(`{}`),
		MissingFields: nil,
	}
	starter := &recordingStarter{
		result: ports.DiagnosisRoomStartResult{
			SessionID:          "diagnosis-session-QUFBQUFBQUFBQUFBQUFBQUFB",
			EvidenceSnapshotID: 42,
			DiagnosisTaskID:    101,
			ChatSessionID:      202,
			Workflow:           ports.WorkflowHandle{WorkflowID: "diagnosis-room-1", RunID: "run-1"},
		},
	}
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{42: snapshot}}},
		starter,
		WithRandomReader(strings.NewReader(strings.Repeat("A", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := service.Start(context.Background(), Request{
		EvidenceSnapshotID: 42,
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if starter.req.SessionID == "" || !strings.HasPrefix(starter.req.SessionID, sessionIDPrefix) {
		t.Fatalf("starter session id = %q", starter.req.SessionID)
	}
	if starter.req.EvidenceSnapshotID != 42 ||
		starter.req.OwnerSubject != "owner-1" ||
		string(starter.req.Evidence) != `{"alert":"cpu"}` {
		t.Fatalf("starter request = %+v", starter.req)
	}
	if got.DiagnosisTaskID != 101 || got.ChatSessionID != 202 || got.Workflow.RunID != "run-1" {
		t.Fatalf("result = %+v", got)
	}
}

func TestServiceStartRejectsUnauthorizedPrincipal(t *testing.T) {
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{}},
		&recordingStarter{},
		WithRandomReader(strings.NewReader(strings.Repeat("B", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.Start(context.Background(), Request{
		EvidenceSnapshotID: 42,
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
		},
	})
	if !errors.Is(err, diagnosisauth.ErrUnauthorized) {
		t.Fatalf("Start error = %v, want ErrUnauthorized", err)
	}
}

func TestServiceStartRejectsFailedSnapshot(t *testing.T) {
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{
			42: {ID: 42, Status: domain.SnapshotStatusFailed, Payload: json.RawMessage(`{"alert":"cpu"}`)},
		}}},
		&recordingStarter{},
		WithRandomReader(strings.NewReader(strings.Repeat("C", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.Start(context.Background(), Request{
		EvidenceSnapshotID: 42,
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Start error = %v, want ErrInvariantViolation", err)
	}
}

type recordingStarter struct {
	req    ports.DiagnosisRoomStartRequest
	result ports.DiagnosisRoomStartResult
	err    error
}

func (s *recordingStarter) StartDiagnosisRoom(_ context.Context, req ports.DiagnosisRoomStartRequest) (ports.DiagnosisRoomStartResult, error) {
	s.req = req
	if s.err != nil {
		return ports.DiagnosisRoomStartResult{}, s.err
	}
	if s.result.SessionID == "" {
		s.result.SessionID = req.SessionID
	}
	return s.result, nil
}

type fakeFactory struct {
	evidence fakeEvidenceRepo
}

func (f fakeFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, errors.New("Begin is not implemented in fakeFactory")
}

func (f fakeFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, fakeUOW{evidence: f.evidence})
}

type fakeUOW struct {
	evidence fakeEvidenceRepo
}

func (u fakeUOW) Alerts() ports.AlertRepository         { panic("Alerts not implemented") }
func (u fakeUOW) Evidence() ports.EvidenceRepository    { return u.evidence }
func (u fakeUOW) Diagnosis() ports.DiagnosisRepository  { panic("Diagnosis not implemented") }
func (u fakeUOW) Reports() ports.ReportRepository       { panic("Reports not implemented") }
func (u fakeUOW) Config() ports.ConfigurationRepository { panic("Config not implemented") }
func (u fakeUOW) Commit(context.Context) error          { return nil }
func (u fakeUOW) Rollback(context.Context) error        { return nil }

type fakeEvidenceRepo struct {
	snapshots map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot
}

func (r fakeEvidenceRepo) Save(context.Context, domain.EvidenceSnapshot) (domain.EvidenceSnapshot, error) {
	panic("Save not implemented")
}

func (r fakeEvidenceRepo) FindByID(_ context.Context, id domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error) {
	snapshot, ok := r.snapshots[id]
	if !ok {
		return domain.EvidenceSnapshot{}, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r fakeEvidenceRepo) FindByGroupAndDigest(context.Context, domain.AlertGroupID, string) (domain.EvidenceSnapshot, error) {
	panic("FindByGroupAndDigest not implemented")
}

func (r fakeEvidenceRepo) ListByGroup(context.Context, domain.AlertGroupID, int) ([]domain.EvidenceSnapshot, error) {
	panic("ListByGroup not implemented")
}

func (r fakeEvidenceRepo) List(context.Context, int) ([]domain.EvidenceSnapshot, error) {
	panic("List not implemented")
}

var _ ports.DiagnosisRoomWorkflowStarter = (*recordingStarter)(nil)
var _ ports.UnitOfWorkFactory = fakeFactory{}
var _ ports.UnitOfWork = fakeUOW{}
var _ ports.EvidenceRepository = fakeEvidenceRepo{}

func TestNewSessionIDRejectsShortRandomReader(t *testing.T) {
	_, err := newSessionID(io.LimitReader(strings.NewReader("short"), 1))
	if err == nil {
		t.Fatal("newSessionID error = nil, want short reader error")
	}
}

func TestNewSessionIDShape(t *testing.T) {
	got, err := newSessionID(strings.NewReader(strings.Repeat("D", sessionIDBytes)))
	if err != nil {
		t.Fatalf("newSessionID: %v", err)
	}
	if !strings.HasPrefix(got, sessionIDPrefix) || strings.ContainsAny(got, "+/=") {
		t.Fatalf("session id = %q", got)
	}
}

func TestServiceStartRejectsZeroSnapshotID(t *testing.T) {
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{}},
		&recordingStarter{},
		WithRandomReader(strings.NewReader(strings.Repeat("E", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Start(context.Background(), Request{
		Principal: ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Start error = %v, want ErrInvariantViolation", err)
	}
}

func TestServiceStartPropagatesSnapshotNotFound(t *testing.T) {
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{}},
		&recordingStarter{},
		WithRandomReader(strings.NewReader(strings.Repeat("F", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Start(context.Background(), Request{
		EvidenceSnapshotID: 99,
		Principal:          ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Start error = %v, want ErrNotFound", err)
	}
}

func TestServiceStartPropagatesStarterError(t *testing.T) {
	wantErr := errors.New("temporal unavailable")
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{
			42: {ID: 42, Status: domain.SnapshotStatusComplete, Payload: json.RawMessage(`{"alert":"cpu"}`)},
		}}},
		&recordingStarter{err: wantErr},
		WithRandomReader(strings.NewReader(strings.Repeat("G", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Start(context.Background(), Request{
		EvidenceSnapshotID: 42,
		Principal:          ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Start error = %v, want %v", err, wantErr)
	}
}

func TestServiceStartRejectsMissingSubject(t *testing.T) {
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{}},
		&recordingStarter{},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Start(context.Background(), Request{EvidenceSnapshotID: 42})
	if !errors.Is(err, diagnosisauth.ErrUnauthenticated) {
		t.Fatalf("Start error = %v, want ErrUnauthenticated", err)
	}
}

func TestNewServiceValidation(t *testing.T) {
	_, err := NewService(nil, &recordingStarter{})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("nil factory error = %v, want ErrInvariantViolation", err)
	}
	_, err = NewService(fakeFactory{}, nil)
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("nil starter error = %v, want ErrInvariantViolation", err)
	}
}
