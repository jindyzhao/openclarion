package diagnosisroomstart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiscontext"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceStartLoadsEvidenceAndStartsWorkflow(t *testing.T) {
	snapshot := domain.EvidenceSnapshot{
		ID:            42,
		Payload:       validStartSnapshotPayload(3),
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
			ApprovalMode:       domain.DiagnosisApprovalModeOwnerAndLeader,
		},
	}
	service, err := NewService(
		fakeFactory{
			evidence: fakeEvidenceRepo{snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{42: snapshot}},
			config: fakeStartConfigRepo{
				channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
					9: mustStartNotificationChannel(t, true, domain.NotificationDeliveryScopeDiagnosisConsultation, domain.NotificationDeliveryScopeDiagnosisClose),
				},
			},
		},
		starter,
		WithRandomReader(strings.NewReader(strings.Repeat("A", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := service.Start(context.Background(), Request{
		EvidenceSnapshotID:                42,
		CloseNotificationChannelProfileID: 9,
		ApprovalMode:                      domain.DiagnosisApprovalModeOwnerAndLeader,
		Principal: ports.AuthPrincipal{
			Subject: "responder-1",
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if starter.req.SessionID == "" || !strings.HasPrefix(starter.req.SessionID, sessionIDPrefix) {
		t.Fatalf("starter session id = %q", starter.req.SessionID)
	}
	if starter.req.EvidenceSnapshotID != 42 ||
		starter.req.OwnerSubject != "responder-1" ||
		starter.req.ApprovalMode != domain.DiagnosisApprovalModeOwnerAndLeader ||
		string(starter.req.Evidence) != string(snapshot.Payload) ||
		starter.req.CloseNotificationChannelProfileID != 9 {
		t.Fatalf("starter request = %+v", starter.req)
	}
	if got.DiagnosisTaskID != 101 || got.ChatSessionID != 202 || got.Workflow.RunID != "run-1" ||
		got.ApprovalMode != domain.DiagnosisApprovalModeOwnerAndLeader {
		t.Fatalf("result = %+v", got)
	}
}

func TestServiceStartRejectsInvalidCloseNotificationChannel(t *testing.T) {
	snapshot := domain.EvidenceSnapshot{
		ID:         42,
		Payload:    validStartSnapshotPayload(3),
		Status:     domain.SnapshotStatusComplete,
		Digest:     "digest",
		Provenance: json.RawMessage(`{}`),
	}
	tests := []struct {
		name     string
		channels map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile
		want     string
	}{
		{
			name:     "missing",
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{},
			want:     "not found",
		},
		{
			name: "disabled",
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				9: mustStartNotificationChannel(t, false, domain.NotificationDeliveryScopeDiagnosisConsultation, domain.NotificationDeliveryScopeDiagnosisClose),
			},
			want: "must be enabled",
		},
		{
			name: "generic webhook",
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				// #nosec G101 -- test-only secret_ref identifier, not a credential value.
				9: {
					ID:        9,
					Name:      "Operations webhook",
					Kind:      domain.NotificationChannelKindWebhook,
					SecretRef: "secret/ops-webhook",
					DeliveryScopes: []domain.NotificationDeliveryScope{
						domain.NotificationDeliveryScopeDiagnosisConsultation,
						domain.NotificationDeliveryScopeDiagnosisClose,
					},
					Enabled: true,
					Labels:  map[string]string{"provider": "webhook"},
				},
			},
			want: "Enterprise WeChat",
		},
		{
			name: "missing consultation scope",
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				9: mustStartNotificationChannel(t, true, domain.NotificationDeliveryScopeDiagnosisClose),
			},
			want: "diagnosis_consultation",
		},
		{
			name: "missing close scope",
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				9: mustStartNotificationChannel(t, true, domain.NotificationDeliveryScopeDiagnosisConsultation),
			},
			want: "diagnosis_close",
		},
		{
			name: "missing ai proof",
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				9: func() domain.NotificationChannelProfile {
					channel := mustStartNotificationChannel(t, true, domain.NotificationDeliveryScopeDiagnosisConsultation, domain.NotificationDeliveryScopeDiagnosisClose)
					channel.LatestTestProofs = nil
					return channel
				}(),
			},
			want: "ai_diagnosis_sample",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			starter := &recordingStarter{}
			service, err := NewService(
				fakeFactory{
					evidence: fakeEvidenceRepo{snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{42: snapshot}},
					config:   fakeStartConfigRepo{channels: tc.channels},
				},
				starter,
				WithRandomReader(strings.NewReader(strings.Repeat("Q", sessionIDBytes))),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			_, err = service.Start(context.Background(), Request{
				EvidenceSnapshotID:                42,
				CloseNotificationChannelProfileID: 9,
				Principal: ports.AuthPrincipal{
					Subject: "owner-1",
					Roles:   []ports.AuthRole{ports.AuthRoleOwner},
				},
			})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("Start error = %v, want ErrInvariantViolation", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Start error = %q, want fragment %q", err, tc.want)
			}
			if starter.req.SessionID != "" {
				t.Fatalf("starter was called: %+v", starter.req)
			}
		})
	}
}

func TestServiceStartMountsAvailableDiagnosisTools(t *testing.T) {
	source := mustStartAlertSource(t, 11)
	template := mustStartToolTemplate(t, 22, source.ID)
	snapshot := domain.EvidenceSnapshot{
		ID:         42,
		Payload:    validStartSnapshotPayload(source.ID),
		Status:     domain.SnapshotStatusComplete,
		Digest:     "digest",
		Provenance: json.RawMessage(`{}`),
	}
	starter := &recordingStarter{}
	service, err := NewService(
		fakeFactory{
			evidence: fakeEvidenceRepo{snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{42: snapshot}},
			config: fakeStartConfigRepo{
				templates: []domain.DiagnosisToolTemplate{template},
				sources:   map[domain.AlertSourceProfileID]domain.AlertSourceProfile{source.ID: source},
			},
		},
		starter,
		WithRandomReader(strings.NewReader(strings.Repeat("H", sessionIDBytes))),
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
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if strings.Contains(string(starter.req.Evidence), "prometheus.example.invalid") ||
		strings.Contains(string(starter.req.Evidence), "secret/prometheus") {
		t.Fatalf("evidence leaked provider configuration: %s", starter.req.Evidence)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(starter.req.Evidence, &top); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	rawCatalog, ok := top[diagnosiscontext.AvailableDiagnosisToolsKey]
	if !ok {
		t.Fatalf("evidence missing %q: %s", diagnosiscontext.AvailableDiagnosisToolsKey, starter.req.Evidence)
	}
	var catalog struct {
		Items []diagnosiscontext.AvailableDiagnosisTool `json:"items"`
	}
	if err := json.Unmarshal(rawCatalog, &catalog); err != nil {
		t.Fatalf("unmarshal tools catalog: %v", err)
	}
	if len(catalog.Items) != 1 {
		t.Fatalf("tools len = %d, want 1: %+v", len(catalog.Items), catalog.Items)
	}
	got := catalog.Items[0]
	if got.TemplateID != int64(template.ID) ||
		got.AlertSourceProfileID != int64(source.ID) ||
		got.Tool != string(domain.DiagnosisToolKindMetricQuery) ||
		got.QueryTemplate != "up" {
		t.Fatalf("tool = %+v", got)
	}
}

func TestServiceStartRejectsUnauthenticatedPrincipal(t *testing.T) {
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
			Subject: " ",
		},
	})
	if !errors.Is(err, diagnosisauth.ErrUnauthenticated) {
		t.Fatalf("Start error = %v, want ErrUnauthenticated", err)
	}
}

func TestServiceStartRejectsReportWorkflowAutomationPrincipal(t *testing.T) {
	for _, subject := range []string{
		"openclarion.report-workflow",
		"openclarion.report-workflow:policy:3",
	} {
		t.Run(subject, func(t *testing.T) {
			starter := &recordingStarter{}
			service, err := NewService(
				fakeFactory{evidence: fakeEvidenceRepo{}},
				starter,
				WithRandomReader(strings.NewReader(strings.Repeat("R", sessionIDBytes))),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			_, err = service.Start(context.Background(), Request{
				EvidenceSnapshotID: 42,
				Principal: ports.AuthPrincipal{
					Subject: subject,
					Roles:   []ports.AuthRole{ports.AuthRoleOwner},
				},
			})
			if !errors.Is(err, diagnosisauth.ErrUnauthorized) {
				t.Fatalf("Start error = %v, want ErrUnauthorized", err)
			}
			if starter.req.SessionID != "" {
				t.Fatalf("starter was called: %+v", starter.req)
			}
		})
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

func TestServiceStartRejectsSnapshotWithoutSourceProfileAwareEvents(t *testing.T) {
	tests := []struct {
		name    string
		payload json.RawMessage
		want    string
	}{
		{
			name:    "legacy ad hoc object",
			payload: json.RawMessage(`{"alert":"cpu"}`),
			want:    "schema_version",
		},
		{
			name: "missing event source profile",
			payload: json.RawMessage(`{
				"schema_version":"m1.evidence_snapshot.v1",
				"events":[{"source":"alertmanager","labels":{"alertname":"HighCPU"}}]
			}`),
			want: "alert_source_profile_id",
		},
		{
			name: "zero event source profile",
			payload: json.RawMessage(`{
				"schema_version":"m1.evidence_snapshot.v1",
				"events":[{"source":"alertmanager","alert_source_profile_id":0}]
			}`),
			want: "alert_source_profile_id",
		},
		{
			name: "empty events",
			payload: json.RawMessage(`{
				"schema_version":"m1.evidence_snapshot.v1",
				"events":[]
			}`),
			want: "events must be non-empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			starter := &recordingStarter{}
			service, err := NewService(
				fakeFactory{evidence: fakeEvidenceRepo{snapshots: map[domain.EvidenceSnapshotID]domain.EvidenceSnapshot{
					42: {ID: 42, Status: domain.SnapshotStatusComplete, Payload: tc.payload},
				}}},
				starter,
				WithRandomReader(strings.NewReader(strings.Repeat("P", sessionIDBytes))),
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
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Start error = %q, want fragment %q", err, tc.want)
			}
			if starter.req.SessionID != "" {
				t.Fatalf("starter was called: %+v", starter.req)
			}
		})
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
	config   fakeStartConfigRepo
}

func (f fakeFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, errors.New("Begin is not implemented in fakeFactory")
}

func (f fakeFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, fakeUOW{evidence: f.evidence, config: f.config})
}

type fakeUOW struct {
	evidence fakeEvidenceRepo
	config   fakeStartConfigRepo
}

func (u fakeUOW) Alerts() ports.AlertRepository         { panic("Alerts not implemented") }
func (u fakeUOW) Evidence() ports.EvidenceRepository    { return u.evidence }
func (u fakeUOW) Diagnosis() ports.DiagnosisRepository  { panic("Diagnosis not implemented") }
func (u fakeUOW) Reports() ports.ReportRepository       { panic("Reports not implemented") }
func (u fakeUOW) Config() ports.ConfigurationRepository { return u.config }
func (u fakeUOW) Directory() ports.DirectoryRepository  { panic("Directory not implemented") }
func (u fakeUOW) RBAC() ports.RBACRepository            { panic("RBAC not implemented") }
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

type fakeStartConfigRepo struct {
	ports.ConfigurationRepository
	channels  map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile
	templates []domain.DiagnosisToolTemplate
	sources   map[domain.AlertSourceProfileID]domain.AlertSourceProfile
}

func (r fakeStartConfigRepo) ListDiagnosisToolTemplates(
	context.Context,
	int,
) ([]domain.DiagnosisToolTemplate, error) {
	return append([]domain.DiagnosisToolTemplate(nil), r.templates...), nil
}

func (r fakeStartConfigRepo) FindAlertSourceProfileByID(
	_ context.Context,
	id domain.AlertSourceProfileID,
) (domain.AlertSourceProfile, error) {
	source, ok := r.sources[id]
	if !ok {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return source, nil
}

func (r fakeStartConfigRepo) FindNotificationChannelProfileByID(
	_ context.Context,
	id domain.NotificationChannelProfileID,
) (domain.NotificationChannelProfile, error) {
	channel, ok := r.channels[id]
	if !ok {
		return domain.NotificationChannelProfile{}, domain.ErrNotFound
	}
	return channel, nil
}

func mustStartToolTemplate(
	t *testing.T,
	id domain.DiagnosisToolTemplateID,
	sourceID domain.AlertSourceProfileID,
) domain.DiagnosisToolTemplate {
	t.Helper()
	enabledAt := time.Date(2026, 6, 18, 1, 2, 3, 0, time.UTC)
	template, err := domain.NewDiagnosisToolTemplate(
		"CPU instant",
		sourceID,
		domain.DiagnosisToolKindMetricQuery,
		"up",
		5,
		0,
		0,
		0,
		true,
		&enabledAt,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	template.ID = id
	return template
}

func mustStartAlertSource(t *testing.T, id domain.AlertSourceProfileID) domain.AlertSourceProfile {
	t.Helper()
	source, err := domain.NewAlertSourceProfile(
		"Prometheus",
		domain.AlertSourceKindPrometheus,
		"https://prometheus.example.invalid",
		domain.AlertSourceAuthModeBearer,
		"secret/prometheus",
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	source.ID = id
	return source
}

func mustStartNotificationChannel(
	t *testing.T,
	enabled bool,
	scopes ...domain.NotificationDeliveryScope,
) domain.NotificationChannelProfile {
	t.Helper()
	return mustStartNotificationChannelWithKind(t, 9, domain.NotificationChannelKindWeCom, enabled, scopes...)
}

func mustStartNotificationChannelWithKind(
	t *testing.T,
	id domain.NotificationChannelProfileID,
	kind domain.NotificationChannelKind,
	enabled bool,
	scopes ...domain.NotificationDeliveryScope,
) domain.NotificationChannelProfile {
	t.Helper()
	provider := "wecom"
	secretRef := "secret/ops-wecom"
	if kind == domain.NotificationChannelKindWebhook {
		provider = "webhook"
		// #nosec G101 -- test-only secret_ref identifier, not a credential value.
		secretRef = "secret/ops-webhook"
	}
	channel, err := domain.NewNotificationChannelProfile(
		"Operations WeCom",
		kind,
		secretRef,
		scopes,
		enabled,
		map[string]string{"provider": provider},
	)
	if err != nil {
		t.Fatalf("NewNotificationChannelProfile: %v", err)
	}
	channel.ID = id
	channel.UpdatedAt = time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	if kind == domain.NotificationChannelKindWeCom {
		channel.LatestTestProofs = []domain.NotificationChannelTestProof{
			mustStartNotificationChannelTestProof(t, channel, domain.NotificationChannelTestContentAIDiagnosisSample, channel.UpdatedAt.Add(time.Minute)),
			mustStartNotificationChannelTestProof(t, channel, domain.NotificationChannelTestContentDiagnosisCloseSample, channel.UpdatedAt.Add(time.Minute)),
		}
	}
	return channel
}

func mustStartNotificationChannelTestProof(
	t *testing.T,
	channel domain.NotificationChannelProfile,
	contentKind domain.NotificationChannelTestContentKind,
	checkedAt time.Time,
) domain.NotificationChannelTestProof {
	t.Helper()
	proof, err := domain.NewNotificationChannelTestProof(
		channel.ID,
		channel.Kind,
		domain.NotificationChannelTestStatusSuccess,
		domain.NotificationChannelTestReasonOK,
		"Notification channel test delivery succeeded.",
		contentKind,
		strings.Repeat("a", 64),
		checkedAt,
		"provider-message-1",
		"delivered",
	)
	if err != nil {
		t.Fatalf("NewNotificationChannelTestProof: %v", err)
	}
	return proof
}

func validStartSnapshotPayload(profileID domain.AlertSourceProfileID) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"schema_version":"m1.evidence_snapshot.v1","group":{"id":7,"group_key":"group-1","dimensions":{},"severity":"warning","event_count":1,"first_seen_at":"2026-06-19T01:02:03Z","last_seen_at":"2026-06-19T01:02:03Z"},"events":[{"id":101,"source":"alertmanager","alert_source_profile_id":%d,"source_fingerprint":"source-fp","canonical_fingerprint":"canon-fp","labels":{"alertname":"HighCPU"},"annotations":{},"status":"firing","starts_at":"2026-06-19T01:02:03Z","ends_at":null,"raw_payload":null}]}`, profileID))
}

var _ ports.DiagnosisRoomWorkflowStarter = (*recordingStarter)(nil)
var _ ports.UnitOfWorkFactory = fakeFactory{}
var _ ports.UnitOfWork = fakeUOW{}
var _ ports.EvidenceRepository = fakeEvidenceRepo{}
var _ ports.ConfigurationRepository = fakeStartConfigRepo{}

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

func TestServiceStartRejectsNegativeNotificationChannelProfileID(t *testing.T) {
	starter := &recordingStarter{}
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{}},
		starter,
		WithRandomReader(strings.NewReader(strings.Repeat("N", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Start(context.Background(), Request{
		EvidenceSnapshotID:                42,
		CloseNotificationChannelProfileID: -1,
		Principal:                         ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Start error = %v, want ErrInvariantViolation", err)
	}
	if starter.req.SessionID != "" {
		t.Fatalf("starter was called: %+v", starter.req)
	}
}

func TestServiceStartRejectsUnsupportedApprovalMode(t *testing.T) {
	starter := &recordingStarter{}
	service, err := NewService(
		fakeFactory{evidence: fakeEvidenceRepo{}},
		starter,
		WithRandomReader(strings.NewReader(strings.Repeat("P", sessionIDBytes))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Start(context.Background(), Request{
		EvidenceSnapshotID: 42,
		ApprovalMode:       domain.DiagnosisApprovalMode("committee"),
		Principal:          ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Start error = %v, want ErrInvariantViolation", err)
	}
	if starter.req.SessionID != "" {
		t.Fatalf("starter was called: %+v", starter.req)
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
			42: {ID: 42, Status: domain.SnapshotStatusComplete, Payload: validStartSnapshotPayload(3)},
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
