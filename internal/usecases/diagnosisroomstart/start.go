// Package diagnosisroomstart owns the usecase boundary for creating an M5
// short-conversation diagnosis room from a frozen EvidenceSnapshot.
package diagnosisroomstart

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiscontext"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	sessionIDPrefix                  = "diagnosis-session-"
	sessionIDBytes                   = 18
	reportWorkflowAutomationSubject  = "openclarion.report-workflow"
	reportWorkflowAutomationSubjectP = reportWorkflowAutomationSubject + ":"
	supportedEvidenceSnapshotSchema  = "m1.evidence_snapshot.v1"
)

// Request starts one room owned by the authenticated principal. Callers are
// responsible for authorizing the principal through OpenClarion-local RBAC
// before invoking this use case.
type Request struct {
	EvidenceSnapshotID                domain.EvidenceSnapshotID
	CloseNotificationChannelProfileID domain.NotificationChannelProfileID
	ApprovalMode                      domain.DiagnosisApprovalMode
	Principal                         ports.AuthPrincipal
}

// Result returns the external session key plus the underlying workflow and
// persistence identities needed by operators and live smoke harnesses.
type Result struct {
	SessionID          string
	EvidenceSnapshotID domain.EvidenceSnapshotID
	DiagnosisTaskID    domain.DiagnosisTaskID
	ChatSessionID      domain.ChatSessionID
	Workflow           ports.WorkflowHandle
	ApprovalMode       domain.DiagnosisApprovalMode
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

// Start verifies the caller identity, loads the immutable EvidenceSnapshot, and starts
// the room workflow. The returned room is ready for ticket issuance.
func (s *Service) Start(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.uowFactory == nil || s.starter == nil {
		return Result{}, fmt.Errorf("diagnosis room start: service is not configured: %w", domain.ErrInvariantViolation)
	}
	if req.EvidenceSnapshotID == 0 {
		return Result{}, fmt.Errorf("diagnosis room start: evidence_snapshot_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if req.CloseNotificationChannelProfileID < 0 {
		return Result{}, fmt.Errorf("diagnosis room start: close_notification_channel_profile_id must be non-negative: %w", domain.ErrInvariantViolation)
	}
	approvalMode := req.ApprovalMode
	if approvalMode == "" {
		approvalMode = domain.DiagnosisApprovalModeSingle
	}
	if !approvalMode.Valid() {
		return Result{}, fmt.Errorf("diagnosis room start: approval_mode %q is unsupported: %w", approvalMode, domain.ErrInvariantViolation)
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
	if err := s.validateCloseNotificationChannel(ctx, req.CloseNotificationChannelProfileID); err != nil {
		return Result{}, err
	}

	snapshot, err := s.loadSnapshot(ctx, req.EvidenceSnapshotID)
	if err != nil {
		return Result{}, err
	}
	if snapshot.Status == domain.SnapshotStatusFailed {
		return Result{}, fmt.Errorf("diagnosis room start: evidence snapshot %d has failed status: %w", snapshot.ID, domain.ErrInvariantViolation)
	}
	if err := validateSnapshotPayloadForDiagnosisRoom(snapshot); err != nil {
		return Result{}, err
	}
	evidence, err := diagnosiscontext.EvidenceWithAvailableDiagnosisTools(ctx, s.uowFactory, snapshot.Payload)
	if err != nil {
		return Result{}, err
	}

	started, err := s.starter.StartDiagnosisRoom(ctx, ports.DiagnosisRoomStartRequest{
		SessionID:                         sessionID,
		EvidenceSnapshotID:                snapshot.ID,
		OwnerSubject:                      subject,
		Evidence:                          evidence,
		CloseNotificationChannelProfileID: req.CloseNotificationChannelProfileID,
		ApprovalMode:                      approvalMode,
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
		ApprovalMode:       started.ApprovalMode,
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

func (s *Service) validateCloseNotificationChannel(ctx context.Context, id domain.NotificationChannelProfileID) error {
	if id == 0 {
		return nil
	}
	var channel domain.NotificationChannelProfile
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Config().FindNotificationChannelProfileByID(ctx, id)
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("diagnosis room start: close notification channel profile not found: %w", domain.ErrInvariantViolation)
		}
		if err != nil {
			return err
		}
		channel = got
		return nil
	})
	if err != nil {
		return err
	}
	if !channel.Enabled {
		return fmt.Errorf("diagnosis room start: close notification channel profile must be enabled: %w", domain.ErrInvariantViolation)
	}
	if channel.Kind != domain.NotificationChannelKindWeCom {
		return fmt.Errorf("diagnosis room start: close notification channel profile must be an Enterprise WeChat channel: %w", domain.ErrInvariantViolation)
	}
	if !notificationChannelSupportsScope(channel, domain.NotificationDeliveryScopeDiagnosisConsultation) {
		return fmt.Errorf("diagnosis room start: close notification channel profile must include diagnosis_consultation delivery scope: %w", domain.ErrInvariantViolation)
	}
	if !notificationChannelSupportsScope(channel, domain.NotificationDeliveryScopeDiagnosisClose) {
		return fmt.Errorf("diagnosis room start: close notification channel profile must include diagnosis_close delivery scope: %w", domain.ErrInvariantViolation)
	}
	if missingProofs := channel.MissingAIDiagnosisProofContentKinds(); len(missingProofs) > 0 {
		return fmt.Errorf("diagnosis room start: close notification channel profile must have current AI delivery test proof for %s: %w", notificationProofKindList(missingProofs), domain.ErrInvariantViolation)
	}
	return nil
}

type diagnosisRoomSnapshotPayload struct {
	SchemaVersion string                       `json:"schema_version"`
	Events        []diagnosisRoomSnapshotEvent `json:"events"`
}

type diagnosisRoomSnapshotEvent struct {
	AlertSourceProfileID int64 `json:"alert_source_profile_id"`
}

func validateSnapshotPayloadForDiagnosisRoom(snapshot domain.EvidenceSnapshot) error {
	var payload diagnosisRoomSnapshotPayload
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		return fmt.Errorf("diagnosis room start: evidence snapshot %d payload must be valid JSON: %w: %w", snapshot.ID, err, domain.ErrInvariantViolation)
	}
	if payload.SchemaVersion != supportedEvidenceSnapshotSchema {
		return fmt.Errorf("diagnosis room start: evidence snapshot %d schema_version must be %q: %w", snapshot.ID, supportedEvidenceSnapshotSchema, domain.ErrInvariantViolation)
	}
	if len(payload.Events) == 0 {
		return fmt.Errorf("diagnosis room start: evidence snapshot %d events must be non-empty: %w", snapshot.ID, domain.ErrInvariantViolation)
	}
	for i, event := range payload.Events {
		if event.AlertSourceProfileID <= 0 {
			return fmt.Errorf("diagnosis room start: evidence snapshot %d events[%d].alert_source_profile_id must be positive: %w", snapshot.ID, i, domain.ErrInvariantViolation)
		}
	}
	return nil
}

func isReportWorkflowAutomationSubject(subject string) bool {
	subject = strings.TrimSpace(subject)
	return subject == reportWorkflowAutomationSubject || strings.HasPrefix(subject, reportWorkflowAutomationSubjectP)
}

func notificationChannelSupportsScope(channel domain.NotificationChannelProfile, want domain.NotificationDeliveryScope) bool {
	for _, scope := range channel.DeliveryScopes {
		if scope == want {
			return true
		}
	}
	return false
}

func notificationProofKindList(kinds []domain.NotificationChannelTestContentKind) string {
	values := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		values = append(values, string(kind))
	}
	return strings.Join(values, " and ")
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
