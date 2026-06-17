// Package diagnosisroomstart owns the usecase boundary for creating an M5
// short-conversation diagnosis room from a frozen EvidenceSnapshot.
package diagnosisroomstart

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	sessionIDPrefix                  = "diagnosis-session-"
	sessionIDBytes                   = 18
	reportWorkflowAutomationSubject  = "openclarion.report-workflow"
	reportWorkflowAutomationSubjectP = reportWorkflowAutomationSubject + ":"
)

// Request starts one room owned by the authenticated principal.
type Request struct {
	EvidenceSnapshotID domain.EvidenceSnapshotID
	Principal          ports.AuthPrincipal
}

// Result returns the external session key plus the underlying workflow and
// persistence identities needed by operators and live smoke harnesses.
type Result struct {
	SessionID          string
	EvidenceSnapshotID domain.EvidenceSnapshotID
	DiagnosisTaskID    domain.DiagnosisTaskID
	ChatSessionID      domain.ChatSessionID
	Workflow           ports.WorkflowHandle
}

// Service creates diagnosis rooms without depending on concrete Temporal or
// repository implementations.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	starter    ports.DiagnosisRoomWorkflowStarter
	random     io.Reader
}

// Option customizes Service construction.
type Option func(*Service)

// WithRandomReader overrides session-id entropy for deterministic tests.
func WithRandomReader(random io.Reader) Option {
	return func(s *Service) {
		if random != nil {
			s.random = random
		}
	}
}

// NewService constructs a diagnosis-room starter service.
func NewService(uowFactory ports.UnitOfWorkFactory, starter ports.DiagnosisRoomWorkflowStarter, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("diagnosis room start: unit of work factory is required: %w", domain.ErrInvariantViolation)
	}
	if starter == nil {
		return nil, fmt.Errorf("diagnosis room start: workflow starter is required: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		uowFactory: uowFactory,
		starter:    starter,
		random:     rand.Reader,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service, nil
}

// Start verifies the caller, loads the immutable EvidenceSnapshot, and starts
// the room workflow. The returned room is ready for ticket issuance.
func (s *Service) Start(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.uowFactory == nil || s.starter == nil {
		return Result{}, fmt.Errorf("diagnosis room start: service is not configured: %w", domain.ErrInvariantViolation)
	}
	if req.EvidenceSnapshotID == 0 {
		return Result{}, fmt.Errorf("diagnosis room start: evidence_snapshot_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	subject := strings.TrimSpace(req.Principal.Subject)
	if subject == "" {
		return Result{}, fmt.Errorf("diagnosis room start: principal subject is required: %w", diagnosisauth.ErrUnauthenticated)
	}
	if isReportWorkflowAutomationSubject(subject) {
		return Result{}, fmt.Errorf("diagnosis room start: report workflow automation cannot create diagnosis rooms directly: %w", diagnosisauth.ErrUnauthorized)
	}
	sessionID, err := newSessionID(s.random)
	if err != nil {
		return Result{}, err
	}
	if err := diagnosisauth.AuthorizeSessionAccess(req.Principal, diagnosisauth.SessionRef{
		SessionID:    sessionID,
		OwnerSubject: subject,
	}); err != nil {
		return Result{}, err
	}

	snapshot, err := s.loadSnapshot(ctx, req.EvidenceSnapshotID)
	if err != nil {
		return Result{}, err
	}
	if snapshot.Status == domain.SnapshotStatusFailed {
		return Result{}, fmt.Errorf("diagnosis room start: evidence snapshot %d has failed status: %w", snapshot.ID, domain.ErrInvariantViolation)
	}

	started, err := s.starter.StartDiagnosisRoom(ctx, ports.DiagnosisRoomStartRequest{
		SessionID:          sessionID,
		EvidenceSnapshotID: snapshot.ID,
		OwnerSubject:       subject,
		Evidence:           snapshot.Payload,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		SessionID:          started.SessionID,
		EvidenceSnapshotID: started.EvidenceSnapshotID,
		DiagnosisTaskID:    started.DiagnosisTaskID,
		ChatSessionID:      started.ChatSessionID,
		Workflow:           started.Workflow,
	}, nil
}

func (s *Service) loadSnapshot(ctx context.Context, id domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error) {
	var snapshot domain.EvidenceSnapshot
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		snapshot, err = uow.Evidence().FindByID(ctx, id)
		return err
	})
	return snapshot, err
}

func isReportWorkflowAutomationSubject(subject string) bool {
	subject = strings.TrimSpace(subject)
	return subject == reportWorkflowAutomationSubject || strings.HasPrefix(subject, reportWorkflowAutomationSubjectP)
}

func newSessionID(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	buf := make([]byte, sessionIDBytes)
	if _, err := io.ReadFull(random, buf); err != nil {
		return "", fmt.Errorf("diagnosis room start: generate session id: %w", err)
	}
	return sessionIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}
