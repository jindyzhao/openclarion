package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	authfake "github.com/openclarion/openclarion/internal/providers/auth/fake"
	"github.com/openclarion/openclarion/internal/providers/im/wecomcallback"
	"github.com/openclarion/openclarion/internal/usecases/alertdiagnosis"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertingest"
	"github.com/openclarion/openclarion/internal/usecases/alertmanagerwebhook"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/alertsourcecheck"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisapproval"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisnotification"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomclose"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisstream"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiswecomcallback"
	"github.com/openclarion/openclarion/internal/usecases/directorysync"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelcheck"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	rbacusecase "github.com/openclarion/openclarion/internal/usecases/rbac"
	"github.com/openclarion/openclarion/internal/usecases/reportnotification"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

func TestDiagnosisWebSocketRelayDefaultUpdateTimeoutCoversAutoEvidenceBudget(t *testing.T) {
	relay := newDiagnosisWebSocketRelay(&fakeDiagnosisRoomWorkflowClient{})
	want := diagnosisroom.HardMaxTurnTimeout * (diagnosisroom.HardMaxAutoEvidenceFollowUps + 1)
	if relay.updateTimeout != want {
		t.Fatalf("update timeout = %s, want %s", relay.updateTimeout, want)
	}
}

func TestDiagnosisWSFramePermissionMapsActions(t *testing.T) {
	tests := []struct {
		frameType string
		want      domain.RBACPermission
		wantOK    bool
	}{
		{frameType: diagnosisWSClientQueryState, want: domain.RBACPermissionDiagnosisRoomRead, wantOK: true},
		{frameType: diagnosisWSClientSubmitTurn, want: domain.RBACPermissionDiagnosisRoomParticipate, wantOK: true},
		{frameType: diagnosisWSClientSubmitSupplementalEvidence, want: domain.RBACPermissionDiagnosisRoomParticipate, wantOK: true},
		{frameType: diagnosisWSClientCollectEvidence, want: domain.RBACPermissionDiagnosisRoomParticipate, wantOK: true},
		{frameType: diagnosisWSClientConfirm, want: domain.RBACPermissionDiagnosisRoomApprove, wantOK: true},
		{frameType: "unsupported", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.frameType, func(t *testing.T) {
			got, ok := diagnosisWSFramePermission(tc.frameType)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("permission = %q ok=%t, want %q ok=%t", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestDiagnosisWSStateFrameProjectsApprovalQuorum(t *testing.T) {
	approvedAt := time.Date(2026, 7, 14, 9, 30, 0, 0, time.UTC)
	digest := strings.Repeat("b", 64)
	frame := diagnosisWSStateFrameFromState(ports.DiagnosisRoomState{
		SessionID:        "session-1",
		ChatSessionID:    42,
		DiagnosisTaskID:  101,
		Status:           "open",
		ApprovalMode:     domain.DiagnosisApprovalModeOwnerAndLeader,
		ConclusionDigest: digest,
		Approvals: []ports.DiagnosisRoomConclusionApproval{{
			ID:               8,
			ConclusionDigest: digest,
			ActorSubject:     "owner-1",
			Authority:        domain.DiagnosisApprovalAuthorityOwner,
			Reason:           "human_confirmed",
			ApprovedAt:       approvedAt,
		}},
		PendingApprovalAuthorities: []domain.DiagnosisApprovalAuthority{
			domain.DiagnosisApprovalAuthorityLeader,
		},
	})

	if frame.ApprovalMode != domain.DiagnosisApprovalModeOwnerAndLeader ||
		frame.ConclusionDigest != digest ||
		len(frame.Approvals) != 1 ||
		frame.Approvals[0].ActorSubject != "owner-1" ||
		len(frame.PendingApprovalAuthorities) != 1 ||
		frame.PendingApprovalAuthorities[0] != domain.DiagnosisApprovalAuthorityLeader {
		t.Fatalf("approval frame = %+v", frame)
	}
}

func TestDiagnosisRoomApprovalStateIgnoresQuorumSupersededByNewTurn(t *testing.T) {
	digest, err := diagnosisapproval.ConclusionDigest("msg-1/assistant", 2, "Initial conclusion.")
	if err != nil {
		t.Fatalf("ConclusionDigest initial: %v", err)
	}
	approvedAt := time.Date(2026, 7, 14, 9, 45, 0, 0, time.UTC)
	repo := &fakeDiagnosisRepo{
		chatTurnsBySession: map[domain.ChatSessionID][]domain.ChatTurn{
			7: {
				{SessionID: 7, MessageID: "msg-1", Sequence: 1, Role: domain.ChatRoleUser, Content: "Investigate."},
				{SessionID: 7, MessageID: "msg-1/assistant", Sequence: 2, Role: domain.ChatRoleAssistant, Content: "Initial conclusion."},
				{SessionID: 7, MessageID: "msg-2", Sequence: 3, Role: domain.ChatRoleUser, Content: "Use the new evidence."},
				{SessionID: 7, MessageID: "msg-2/assistant", Sequence: 4, Role: domain.ChatRoleAssistant, Content: "Revised conclusion."},
			},
		},
		chatApprovalsByKey: map[string][]domain.ChatSessionApproval{
			chatApprovalTestKey(7, digest): {{
				ID:               1,
				SessionID:        7,
				ConclusionDigest: digest,
				ActorSubject:     "owner-1",
				Authority:        domain.DiagnosisApprovalAuthorityOwner,
				Reason:           "human_confirmed",
				ApprovedAt:       approvedAt,
				CreatedAt:        approvedAt,
			}},
		},
	}

	gotDigest, approvals, err := diagnosisRoomApprovalState(context.Background(), repo, domain.ChatSession{
		ID:           7,
		SessionKey:   "session-approval",
		TurnCount:    2,
		ApprovalMode: domain.DiagnosisApprovalModeOwnerAndLeader,
	}, repo.chatTurnsBySession[7])
	if err != nil {
		t.Fatalf("diagnosisRoomApprovalState: %v", err)
	}
	if gotDigest != "" || len(approvals) != 0 || repo.listChatApprovalsCalls != 1 {
		t.Fatalf("stale approval state digest=%q approvals=%+v row_calls=%d", gotDigest, approvals, repo.listChatApprovalsCalls)
	}
}

func TestDiagnosisRoomApprovalStateProjectsCurrentQuorum(t *testing.T) {
	digest, err := diagnosisapproval.ConclusionDigest("msg-1/assistant", 2, "Current conclusion.")
	if err != nil {
		t.Fatalf("ConclusionDigest current: %v", err)
	}
	approvedAt := time.Date(2026, 7, 14, 9, 45, 0, 0, time.UTC)
	repo := &fakeDiagnosisRepo{
		chatTurnsBySession: map[domain.ChatSessionID][]domain.ChatTurn{
			7: {
				{SessionID: 7, MessageID: "msg-1", Sequence: 1, Role: domain.ChatRoleUser, Content: "Investigate."},
				{SessionID: 7, MessageID: "msg-1/assistant", Sequence: 2, Role: domain.ChatRoleAssistant, Content: "Current conclusion."},
			},
		},
		chatApprovalsByKey: map[string][]domain.ChatSessionApproval{
			chatApprovalTestKey(7, digest): {{
				ID:               1,
				SessionID:        7,
				ConclusionDigest: digest,
				ActorSubject:     "owner-1",
				Authority:        domain.DiagnosisApprovalAuthorityOwner,
				Reason:           "human_confirmed",
				ApprovedAt:       approvedAt,
				CreatedAt:        approvedAt,
			}},
		},
	}

	gotDigest, approvals, err := diagnosisRoomApprovalState(context.Background(), repo, domain.ChatSession{
		ID:           7,
		SessionKey:   "session-approval",
		TurnCount:    1,
		ApprovalMode: domain.DiagnosisApprovalModeOwnerAndLeader,
	}, repo.chatTurnsBySession[7])
	if err != nil {
		t.Fatalf("diagnosisRoomApprovalState: %v", err)
	}
	if gotDigest != digest || len(approvals) != 1 ||
		approvals[0].ActorSubject != "owner-1" ||
		approvals[0].Authority != api.DiagnosisRoomApprovalAuthorityOwner ||
		repo.listChatApprovalsCalls != 1 {
		t.Fatalf("current approval state digest=%q approvals=%+v row_calls=%d", gotDigest, approvals, repo.listChatApprovalsCalls)
	}
}

func TestDiagnosisRoomSystemSubjectClassifiesOpenClarionServices(t *testing.T) {
	tests := []struct {
		subject string
		want    bool
	}{
		{subject: "openclarion:auto-diagnosis", want: true},
		{subject: "openclarion:alertmanager-webhook:1", want: true},
		{subject: "openclarion.notification-worker", want: true},
		{subject: "operator:alice", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.subject, func(t *testing.T) {
			if got := diagnosisRoomSystemSubject(tc.subject); got != tc.want {
				t.Fatalf("diagnosisRoomSystemSubject(%q) = %v, want %v", tc.subject, got, tc.want)
			}
		})
	}
}

func testDirectoryUser(id int64, subject, displayName string) domain.DirectoryUser {
	now := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	return domain.DirectoryUser{
		ID:                    domain.DirectoryUserID(id),
		Provider:              "ops_iam",
		Subject:               subject,
		ExternalID:            fmt.Sprintf("external-%d", id),
		Username:              fmt.Sprintf("user-%d", id),
		DisplayName:           displayName,
		Email:                 fmt.Sprintf("user-%d@example.test", id),
		JobTitle:              "SRE",
		Department:            "Platform",
		Section:               "SRE",
		DepartmentPath:        "IT/Platform/SRE",
		DepartmentExternalIDs: []string{"dep-platform"},
		Active:                true,
		SyncedAt:              now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

func TestSyncDirectoryUsesConfiguredSyncer(t *testing.T) {
	syncedAt := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	updatedAfter := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{configRepo: &fakeConfigRepo{}}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: syncedAt},
	}
	syncer := &fakeDirectorySyncer{
		result: directorysync.Result{
			Run: domain.DirectorySyncRun{
				SyncedAt: syncedAt,
			},
			DepartmentPages:     1,
			UserPages:           2,
			DepartmentsUpserted: 3,
			UsersUpserted:       4,
			UsersDeactivated:    5,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/config/directory/sync",
		strings.NewReader(`{"page_size":2,"updated_after":"2026-06-26T08:00:00Z"}`),
	)
	addTestLocalRBACAuthorization(req)
	opts := testLocalRBACOptions(t, "iam-admin", authorizer)
	opts = append(opts, WithDirectorySyncer(syncer, "ops_iam"))
	testHandler(factory, opts...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 || authorizer.req.Permission != domain.RBACPermissionDirectoryManage {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if syncer.called != 1 ||
		syncer.req.Provider != "ops_iam" ||
		syncer.req.PageSize != 2 ||
		syncer.req.UpdatedAfter == nil ||
		!syncer.req.UpdatedAfter.Equal(updatedAfter) {
		t.Fatalf("sync request = %+v called=%d", syncer.req, syncer.called)
	}
	var body api.DirectorySyncResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.DepartmentPages != 1 || body.UserPages != 2 || body.DepartmentsUpserted != 3 || body.UsersUpserted != 4 || body.UsersDeactivated != 5 || !body.SyncedAt.Equal(syncedAt) {
		t.Fatalf("body = %+v", body)
	}
}

func TestListDirectoryProjectionEndpointsReturnLocalRows(t *testing.T) {
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	config := &fakeConfigRepo{
		directoryDepartments: []domain.DirectoryDepartment{{
			ID:               1,
			Provider:         "ops_iam",
			ExternalID:       "dep-1",
			ParentExternalID: "",
			Name:             "Platform",
			DisplayName:      "Platform",
			Path:             "IT/Platform",
			ParentPath:       "IT",
			Level:            2,
			Source:           "iam",
			MemberCount:      7,
			SyncedAt:         now,
			CreatedAt:        now,
			UpdatedAt:        now,
		}},
		directoryUsers: []domain.DirectoryUser{{
			ID:                    2,
			Provider:              "ops_iam",
			Subject:               "iam-user-1",
			ExternalID:            "wecom-user-1",
			Username:              "alice",
			DisplayName:           "Alice",
			Email:                 "alice@example.test",
			DepartmentPath:        "IT/Platform",
			DepartmentExternalIDs: []string{"dep-1"},
			Active:                true,
			SyncedAt:              now,
			CreatedAt:             now,
			UpdatedAt:             now,
		}},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed:   req.Permission == domain.RBACPermissionDirectoryRead,
				CheckedAt: now,
			}, nil
		},
	}
	handler := testHandler(&fakeUOWFactory{configRepo: config}, testLocalRBACOptions(t, "iam-user-1", authorizer)...)

	departmentRec := httptest.NewRecorder()
	departmentReq := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/directory/departments?provider=ops_iam&limit=1", nil)
	addTestLocalRBACAuthorization(departmentReq)
	handler.ServeHTTP(departmentRec, departmentReq)
	if departmentRec.Code != stdhttp.StatusOK {
		t.Fatalf("departments status = %d, want 200; body=%s", departmentRec.Code, departmentRec.Body.String())
	}
	var departments api.DirectoryDepartmentListResponse
	if err := json.NewDecoder(departmentRec.Body).Decode(&departments); err != nil {
		t.Fatalf("decode departments: %v", err)
	}
	if len(departments.Items) != 1 || departments.Items[0].ExternalID != "dep-1" || config.lastDirectoryProvider != "ops_iam" || config.lastDirectoryLimit != 1 {
		t.Fatalf("departments = %+v provider=%q limit=%d", departments.Items, config.lastDirectoryProvider, config.lastDirectoryLimit)
	}

	userRec := httptest.NewRecorder()
	userReq := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/directory/users?provider=ops_iam&limit=1", nil)
	addTestLocalRBACAuthorization(userReq)
	handler.ServeHTTP(userRec, userReq)
	if userRec.Code != stdhttp.StatusOK {
		t.Fatalf("users status = %d, want 200; body=%s", userRec.Code, userRec.Body.String())
	}
	var users api.DirectoryUserListResponse
	if err := json.NewDecoder(userRec.Body).Decode(&users); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(users.Items) != 1 || users.Items[0].Subject != "iam-user-1" || !slices.Equal(users.Items[0].DepartmentExternalIds, []string{"dep-1"}) {
		t.Fatalf("users = %+v", users.Items)
	}
	if authorizer.called != 2 ||
		authorizer.requests[0].Permission != domain.RBACPermissionDirectoryRead ||
		!slices.Equal(authorizer.requests[0].Principal.DepartmentKeys, []string{"dep-1"}) {
		t.Fatalf("authorizer requests = %+v called=%d", authorizer.requests, authorizer.called)
	}
}

func TestListDirectorySyncRunsReturnsLocalHistory(t *testing.T) {
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	updatedAfter := now.Add(-time.Hour)
	config := &fakeConfigRepo{
		directorySyncRuns: []domain.DirectorySyncRun{{
			ID:                  1,
			Provider:            "ops_iam",
			PageSize:            100,
			UpdatedAfter:        &updatedAfter,
			Status:              domain.DirectorySyncRunStatusSucceeded,
			DepartmentPages:     1,
			UserPages:           2,
			DepartmentsUpserted: 3,
			UsersUpserted:       4,
			SyncedAt:            now,
			CreatedAt:           now,
		}},
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: now},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/directory/sync-runs?provider=ops_iam&limit=1", nil)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: config}, testLocalRBACOptions(t, "iam-user-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.DirectorySyncRunListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 ||
		body.Items[0].Status != string(domain.DirectorySyncRunStatusSucceeded) ||
		body.Items[0].UsersUpserted != 4 ||
		body.Items[0].UpdatedAfter == nil {
		t.Fatalf("body = %+v", body)
	}
	if authorizer.called != 1 || authorizer.req.Permission != domain.RBACPermissionDirectoryRead {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if config.lastDirectoryProvider != "ops_iam" || config.lastDirectoryLimit != 1 {
		t.Fatalf("provider=%q limit=%d", config.lastDirectoryProvider, config.lastDirectoryLimit)
	}
}

func TestUpsertRBACAssignmentStoresLocalAssignment(t *testing.T) {
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	config := &fakeConfigRepo{
		upsertRBACAssignmentResult: domain.RBACAssignment{
			ID:          9,
			SubjectKind: domain.RBACSubjectKindUser,
			SubjectKey:  "iam-user-1",
			Role:        domain.RBACRoleResponder,
			ScopeKind:   domain.RBACScopeKindDiagnosisRoom,
			ScopeKey:    "diagnosis-session-1",
			Enabled:     true,
			CreatedBy:   "iam-admin",
			UpdatedBy:   "iam-admin",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: now},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/config/rbac/assignments",
		strings.NewReader(`{"subject_kind":"user","subject_key":"iam-user-1","role":"responder","scope_kind":"diagnosis_room","scope_key":"diagnosis-session-1"}`),
	)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: config}, testLocalRBACOptions(t, "iam-admin", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 || authorizer.req.Permission != domain.RBACPermissionRBACManage {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if config.savedRBACAssignment.SubjectKey != "iam-user-1" ||
		config.savedRBACAssignment.Role != domain.RBACRoleResponder ||
		config.savedRBACAssignment.ScopeKey != "diagnosis-session-1" ||
		config.savedRBACAssignment.CreatedBy != "iam-admin" ||
		config.savedRBACAssignment.UpdatedBy != "iam-admin" ||
		!config.savedRBACAssignment.Enabled {
		t.Fatalf("saved assignment = %+v", config.savedRBACAssignment)
	}
	var body api.RBACAssignment
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ID != 9 || body.ScopeKey != "diagnosis-session-1" || body.CreatedBy != "iam-admin" || body.UpdatedBy != "iam-admin" {
		t.Fatalf("body = %+v", body)
	}
}

func TestListRBACAssignmentsReturnsLocalAssignments(t *testing.T) {
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	config := &fakeConfigRepo{
		rbacAssignments: []domain.RBACAssignment{{
			ID:          9,
			SubjectKind: domain.RBACSubjectKindDepartment,
			SubjectKey:  "dep-1",
			Role:        domain.RBACRoleResponder,
			ScopeKind:   domain.RBACScopeKindDiagnosisRoom,
			ScopeKey:    "diagnosis-session-1",
			Enabled:     true,
			CreatedBy:   "iam-admin",
			UpdatedBy:   "iam-admin",
			CreatedAt:   now,
			UpdatedAt:   now,
		}},
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: now},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/rbac/assignments?limit=1", nil)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: config}, testLocalRBACOptions(t, "iam-admin", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 || authorizer.req.Permission != domain.RBACPermissionRBACManage {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if config.lastRBACAssignmentLimit != 1 {
		t.Fatalf("last rbac assignment limit = %d, want 1", config.lastRBACAssignmentLimit)
	}
	var body api.RBACAssignmentListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].SubjectKey != "dep-1" || body.Items[0].Role != api.RBACRoleResponder {
		t.Fatalf("body = %+v", body)
	}
}

func TestAuthorizeRBACUsesConfiguredAuthorizer(t *testing.T) {
	checkedAt := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   true,
			CheckedAt: checkedAt,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/config/rbac/authorize",
		strings.NewReader(`{"subject":"iam-user-1","department_keys":["dep-1"],"permission":"diagnosis_room.participate","scope_kind":"diagnosis_room","scope_key":"diagnosis-session-1"}`),
	)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, testLocalRBACOptions(t, "iam-admin", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 2 ||
		authorizer.requests[0].Permission != domain.RBACPermissionRBACManage ||
		authorizer.req.Principal.Subject != "iam-user-1" ||
		!slices.Equal(authorizer.req.Principal.DepartmentKeys, []string{"dep-1"}) ||
		authorizer.req.Permission != domain.RBACPermissionDiagnosisRoomParticipate ||
		authorizer.req.ScopeKey != "diagnosis-session-1" {
		t.Fatalf("authorize request = %+v called=%d", authorizer.req, authorizer.called)
	}
	var body api.RBACAuthorizeResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.Allowed || !body.CheckedAt.Equal(checkedAt) {
		t.Fatalf("body = %+v", body)
	}
}

func TestAuthorizeCurrentRBACUsesAuthenticatedPrincipal(t *testing.T) {
	checkedAt := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   true,
			CheckedAt: checkedAt,
		},
	}
	config := &fakeConfigRepo{
		directoryUsers: []domain.DirectoryUser{
			{
				Subject:               "iam-user-1",
				DisplayName:           "Alice",
				DepartmentExternalIDs: []string{"dep-2", "dep-1", "dep-2", " "},
				Active:                true,
			},
			{
				Subject:               "other-user",
				DepartmentExternalIDs: []string{"dep-other"},
				Active:                true,
			},
			{
				Subject:               "iam-user-1",
				DisplayName:           "Alice Archived",
				DepartmentExternalIDs: []string{"dep-inactive"},
				Active:                false,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/config/rbac/current-authorizations",
		strings.NewReader(`{"requests":[{"permission":"directory.read","scope_kind":"global"},{"permission":"alert_source.manage","scope_kind":"alert_source","scope_key":"7"}]}`),
	)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: config}, testLocalRBACOptions(t, "iam-user-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 2 {
		t.Fatalf("authorizer called = %d, want 2", authorizer.called)
	}
	if authorizer.requests[0].Permission == domain.RBACPermissionRBACManage ||
		authorizer.requests[1].Permission == domain.RBACPermissionRBACManage {
		t.Fatalf("current authorization unexpectedly required rbac.manage: %+v", authorizer.requests)
	}
	if authorizer.requests[0].Principal.Subject != "iam-user-1" ||
		!slices.Equal(authorizer.requests[0].Principal.DepartmentKeys, []string{"dep-1", "dep-2"}) ||
		authorizer.requests[0].Permission != domain.RBACPermissionDirectoryRead ||
		authorizer.requests[0].ScopeKind != domain.RBACScopeKindGlobal ||
		authorizer.requests[0].ScopeKey != "" ||
		authorizer.requests[1].Permission != domain.RBACPermissionAlertSourceManage ||
		authorizer.requests[1].ScopeKind != domain.RBACScopeKindAlertSource ||
		authorizer.requests[1].ScopeKey != "7" {
		t.Fatalf("authorizer requests = %+v", authorizer.requests)
	}
	var body api.RBACCurrentAuthorizationResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Subject != "iam-user-1" ||
		!slices.Equal(body.DepartmentKeys, []string{"dep-1", "dep-2"}) ||
		len(body.DirectoryUsers) != 2 ||
		body.DirectoryUsers[0].DisplayName != "Alice" ||
		body.DirectoryUsers[1].DisplayName != "Alice Archived" ||
		len(body.Decisions) != 2 ||
		!body.Decisions[0].Allowed ||
		body.Decisions[0].ScopeKey != "" ||
		body.Decisions[1].ScopeKey != "7" ||
		!body.Decisions[1].CheckedAt.Equal(checkedAt) {
		t.Fatalf("body = %+v", body)
	}
}

func TestAuthorizeCurrentRBACAllowsDiagnosisRoomOwner(t *testing.T) {
	now := time.Date(2026, 6, 29, 5, 45, 0, 0, time.UTC)
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   false,
			CheckedAt: now,
		},
	}
	repo := &fakeDiagnosisRepo{
		chatSessions: []domain.ChatSessionWithTask{{
			Session: domain.ChatSession{
				ID:              911,
				DiagnosisTaskID: 912,
				SessionKey:      "diagnosis-session-owned",
				OwnerSubject:    "iam-user-1",
				Status:          domain.ChatSessionStatusOpen,
				StartedAt:       now,
				LastActivityAt:  now,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			Task: domain.DiagnosisTask{
				ID:                 912,
				EvidenceSnapshotID: 913,
				WorkflowID:         "diagnosis-room-diagnosis-session-owned",
				RunID:              "run-owned",
				Status:             domain.DiagnosisStatusRunning,
			},
		}},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/config/rbac/current-authorizations",
		strings.NewReader(`{"requests":[{"permission":"diagnosis_room.read","scope_kind":"diagnosis_room","scope_key":"diagnosis-session-owned"},{"permission":"diagnosis_room.participate","scope_kind":"diagnosis_room","scope_key":"diagnosis-session-owned"},{"permission":"diagnosis_room.administer","scope_kind":"diagnosis_room","scope_key":"diagnosis-session-owned"},{"permission":"alert_source.read","scope_kind":"alert_source","scope_key":"7"}]}`),
	)
	addTestLocalRBACAuthorization(req)
	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}, diagnosisRepo: repo},
		testLocalRBACOptions(t, "iam-user-1", authorizer)...,
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 ||
		authorizer.req.Permission != domain.RBACPermissionAlertSourceRead {
		t.Fatalf("authorizer requests = %+v called=%d, want only non-owner fallback", authorizer.requests, authorizer.called)
	}
	var body api.RBACCurrentAuthorizationResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Decisions) != 4 {
		t.Fatalf("decisions = %+v, want 4", body.Decisions)
	}
	for i := 0; i < 3; i++ {
		if !body.Decisions[i].Allowed ||
			body.Decisions[i].ScopeKey != "diagnosis-session-owned" ||
			body.Decisions[i].CheckedAt.IsZero() {
			t.Fatalf("owner decision %d = %+v, want allowed room decision", i, body.Decisions[i])
		}
	}
	if body.Decisions[3].Allowed {
		t.Fatalf("non-owner decision = %+v, want denied fallback", body.Decisions[3])
	}
}

func TestAuthorizeCurrentRBACRejectsEmptyBatch(t *testing.T) {
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/config/rbac/current-authorizations",
		strings.NewReader(`{"requests":[]}`),
	)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, testLocalRBACOptions(t, "iam-user-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer called = %d, want 0", authorizer.called)
	}
}

func TestConfigLocalRBACGuardDeniesUnauthorizedPrincipal(t *testing.T) {
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: false, CheckedAt: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/directory/users", nil)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, testLocalRBACOptions(t, "iam-user-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 || authorizer.req.Permission != domain.RBACPermissionDirectoryRead {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
}

func TestConfigLocalRBACGuardAllowsBootstrapAdminSubject(t *testing.T) {
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: false, CheckedAt: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/directory/users", nil)
	addTestLocalRBACAuthorization(req)
	opts := testLocalRBACOptions(t, "iam-bootstrap-admin", authorizer)
	opts = append(opts, WithLocalRBACBootstrapAdminSubjects([]string{"iam-bootstrap-admin"}))
	testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, opts...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer called = %d, want 0", authorizer.called)
	}
}

func TestConfigLocalRBACGuardAcceptsBrowserSessionToken(t *testing.T) {
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	sessionIssuer := newHTTPTestDiagnosisSessionIssuer(t, now)
	session, err := sessionIssuer.IssueToken(context.Background(), ports.AuthPrincipal{
		Subject: "iam-user-1",
		Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
		Claims:  json.RawMessage(`{"auth_provider":"oidc"}`),
	}, "oidc")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: now},
	}
	config := &fakeConfigRepo{
		directoryUsers: []domain.DirectoryUser{{
			Subject:               "iam-user-1",
			DepartmentExternalIDs: []string{"dep-2"},
			Active:                true,
		}},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/directory/users", nil)
	req.Header.Set("Authorization", "Bearer "+session.Token)
	testHandler(
		&fakeUOWFactory{configRepo: config},
		WithDiagnosisAuth(&neverAuthProvider{}, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("B", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}, "oidc"),
		WithDiagnosisAuthSessionIssuer(sessionIssuer),
		WithRBACAuthorizer(authorizer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 ||
		authorizer.req.Principal.Subject != "iam-user-1" ||
		!slices.Equal(authorizer.req.Principal.DepartmentKeys, []string{"dep-2"}) {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
}

func TestAuthorizeCurrentRBACAllowsBootstrapAdminSubject(t *testing.T) {
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: false, CheckedAt: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/config/rbac/current-authorizations",
		strings.NewReader(`{"requests":[{"permission":"directory.manage","scope_kind":"global"},{"permission":"alert_source.manage","scope_kind":"alert_source","scope_key":"7"}]}`),
	)
	addTestLocalRBACAuthorization(req)
	opts := testLocalRBACOptions(t, "iam-bootstrap-admin", authorizer)
	opts = append(opts, WithLocalRBACBootstrapAdminSubjects([]string{"iam-bootstrap-admin"}))
	testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, opts...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer called = %d, want 0", authorizer.called)
	}
	var body api.RBACCurrentAuthorizationResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Subject != "iam-bootstrap-admin" || len(body.Decisions) != 2 || !body.Decisions[0].Allowed || !body.Decisions[1].Allowed {
		t.Fatalf("body = %+v", body)
	}
}

func TestAuthorizeCurrentRBACBootstrapAdminStillValidatesRequests(t *testing.T) {
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: false, CheckedAt: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/config/rbac/current-authorizations",
		strings.NewReader(`{"requests":[{"permission":"unknown.permission","scope_kind":"global"}]}`),
	)
	addTestLocalRBACAuthorization(req)
	opts := testLocalRBACOptions(t, "iam-bootstrap-admin", authorizer)
	opts = append(opts, WithLocalRBACBootstrapAdminSubjects([]string{"iam-bootstrap-admin"}))
	testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, opts...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer called = %d, want 0", authorizer.called)
	}
}

func TestConfigLocalRBACGuardUsesResourceScope(t *testing.T) {
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: now},
	}
	config := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{{
			ID:        7,
			Name:      "Primary Prometheus",
			Kind:      domain.AlertSourceKindPrometheus,
			BaseURL:   "https://prometheus.example.test",
			AuthMode:  domain.AlertSourceAuthModeNone,
			Enabled:   true,
			Labels:    map[string]string{},
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/alert-sources/7", nil)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: config}, testLocalRBACOptions(t, "iam-admin", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 ||
		authorizer.req.Permission != domain.RBACPermissionAlertSourceRead ||
		authorizer.req.ScopeKind != domain.RBACScopeKindAlertSource ||
		authorizer.req.ScopeKey != "7" {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
}

func TestConfigLocalRBACGuardUsesReportWorkflowScheduleScope(t *testing.T) {
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: now},
	}
	config := &fakeConfigRepo{
		reportWorkflowSchedules: []domain.ReportWorkflowSchedule{{
			ID:                     9,
			Name:                   "Daily report window",
			ReportWorkflowPolicyID: 7,
			TemporalScheduleID:     "openclarion-report-policy-7-daily",
			Interval:               24 * time.Hour,
			Offset:                 6 * time.Hour,
			ReplayWindow:           time.Hour,
			ReplayDelay:            5 * time.Minute,
			ReplayLimit:            10000,
			CatchupWindow:          time.Hour,
			CreatedAt:              now,
			UpdatedAt:              now,
		}},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/report-workflow-schedules/9", nil)
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: config}, testLocalRBACOptions(t, "iam-admin", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 ||
		authorizer.req.Permission != domain.RBACPermissionReportWorkflowRead ||
		authorizer.req.ScopeKind != domain.RBACScopeKindReportWorkflowSchedule ||
		authorizer.req.ScopeKey != "9" {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
}

func TestListAlerts_ReturnsSummaries(t *testing.T) {
	startsAt := time.Date(2026, 5, 27, 8, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		alertRepo: &fakeAlertRepo{
			events: []domain.AlertEvent{
				{
					ID:                   42,
					Source:               "prometheus",
					SourceFingerprint:    "source-fp",
					CanonicalFingerprint: "canon-fp",
					Labels:               map[string]string{"alertname": "HighCPU"},
					Annotations:          map[string]string{"summary": "CPU high"},
					Status:               domain.AlertStatusFiring,
					StartsAt:             startsAt,
					CreatedAt:            startsAt.Add(time.Second),
				},
			},
		},
		evidenceRepo:  &fakeEvidenceRepo{},
		diagnosisRepo: &fakeDiagnosisRepo{},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/alerts?limit=1", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.alertRepo.lastLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.alertRepo.lastLimit)
	}

	var body api.AlertListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.ID != 42 || got.Source != "prometheus" || got.Status != string(domain.AlertStatusFiring) {
		t.Fatalf("unexpected alert summary: %+v", got)
	}
	if !got.EndsAt.IsNull() {
		t.Fatalf("ends_at should be explicit null for firing alerts")
	}
	if got.LinkedEvidenceSnapshots == nil || len(got.LinkedEvidenceSnapshots) != 0 {
		t.Fatalf("linked_evidence_snapshots = %+v, want empty array", got.LinkedEvidenceSnapshots)
	}
	if factory.evidenceRepo.lastLimit != maxListLimit || factory.diagnosisRepo.lastChatSessionLimit != maxListLimit {
		t.Fatalf("link lookup limits evidence=%d rooms=%d, want %d", factory.evidenceRepo.lastLimit, factory.diagnosisRepo.lastChatSessionLimit, maxListLimit)
	}
}

func TestListAlerts_ReturnsEvidenceAndDiagnosisLinks(t *testing.T) {
	startsAt := time.Date(2026, 6, 18, 2, 20, 28, 0, time.UTC)
	updatedAt := startsAt.Add(3 * time.Second)
	finalAt := startsAt.Add(4 * time.Second)
	requiresReview := false
	finalPayload, err := json.Marshal(diagnosisRoomConclusionEventPayload{
		Kind:            diagnosisConclusionEventFinalReady,
		SessionID:       "diagnosis-session-auto-p3-s247",
		ChatSessionID:   353,
		DiagnosisTaskID: 353,
		FinalConclusion: diagnosisRoomConclusionPayload{
			Status:              "available",
			Source:              "assistant",
			EvidenceSnapshotID:  247,
			ConclusionVersion:   "diagnosis-session-auto-p3-s247:1",
			Content:             "Checkout latency is correlated with downstream saturation.",
			Confidence:          "high",
			RequiresHumanReview: &requiresReview,
		},
	})
	if err != nil {
		t.Fatalf("marshal final payload: %v", err)
	}
	factory := &fakeUOWFactory{
		alertRepo: &fakeAlertRepo{
			events: []domain.AlertEvent{
				{
					ID:                   42,
					Source:               "alertmanager",
					SourceFingerprint:    "source-fp",
					CanonicalFingerprint: "canon-fp",
					Labels:               map[string]string{"alertname": "CheckoutLatencyHigh"},
					Annotations:          map[string]string{"summary": "Checkout p95 latency is high"},
					Status:               domain.AlertStatusFiring,
					StartsAt:             startsAt,
					CreatedAt:            startsAt.Add(time.Second),
				},
			},
		},
		evidenceRepo: &fakeEvidenceRepo{
			snapshots: []domain.EvidenceSnapshot{
				{
					ID:           247,
					AlertGroupID: 31,
					Digest:       "sha256:linked",
					Payload: json.RawMessage(`{
						"schema_version":"m1.evidence_snapshot.v1",
						"events":[{"source_fingerprint":"source-fp","canonical_fingerprint":"canon-fp"}]
					}`),
					Provenance:        json.RawMessage(`{}`),
					Status:            domain.SnapshotStatusComplete,
					CreatedByWorkflow: "AlertmanagerWebhookAutoDiagnosis",
					CreatedAt:         startsAt.Add(2 * time.Second),
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				353: {
					diagnosisConclusionEventFinalReady: {{
						ID:         91,
						TaskID:     353,
						Kind:       diagnosisConclusionEventFinalReady,
						Payload:    finalPayload,
						OccurredAt: finalAt,
						RecordedAt: finalAt.Add(time.Second),
					}},
				},
			},
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              353,
						DiagnosisTaskID: 353,
						SessionKey:      "diagnosis-session-auto-p3-s247",
						Status:          domain.ChatSessionStatusOpen,
						TurnCount:       1,
						StartedAt:       startsAt,
						LastActivityAt:  updatedAt,
						CreatedAt:       startsAt,
						UpdatedAt:       updatedAt,
					},
					Task: domain.DiagnosisTask{
						ID:                 353,
						EvidenceSnapshotID: 247,
						WorkflowID:         "diagnosis-room-diagnosis-session-auto-p3-s247",
						RunID:              "run-247",
						Status:             domain.DiagnosisStatusRunning,
						CreatedAt:          startsAt,
						UpdatedAt:          updatedAt,
					},
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/alerts?limit=1", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.AlertListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	links := body.Items[0].LinkedEvidenceSnapshots
	if len(links) != 1 {
		t.Fatalf("linked_evidence_snapshots = %+v, want 1 link", links)
	}
	link := links[0]
	if link.ID != 247 ||
		link.AlertGroupID != 31 ||
		link.Digest != "sha256:linked" ||
		link.Status != string(domain.SnapshotStatusComplete) ||
		link.CreatedByWorkflow != "AlertmanagerWebhookAutoDiagnosis" {
		t.Fatalf("unexpected link: %+v", link)
	}
	if len(link.DiagnosisRooms) != 1 {
		t.Fatalf("diagnosis rooms = %+v, want 1", link.DiagnosisRooms)
	}
	room := link.DiagnosisRooms[0]
	if room.SessionID != "diagnosis-session-auto-p3-s247" ||
		room.EvidenceSnapshotID != 247 ||
		room.RoomStatus != api.Open ||
		room.TaskStatus != api.DiagnosisTaskStatusRunning {
		t.Fatalf("unexpected room link: %+v", room)
	}
	if room.LatestConclusion == nil ||
		room.LatestConclusion.Content != "Checkout latency is correlated with downstream saturation." ||
		room.LatestConclusion.Confidence == nil ||
		*room.LatestConclusion.Confidence != api.ReportConfidenceHigh ||
		room.LatestConclusion.RequiresHumanReview == nil ||
		*room.LatestConclusion.RequiresHumanReview {
		t.Fatalf("unexpected room latest conclusion: %+v", room.LatestConclusion)
	}
}

func TestAlertEvidenceLinksPrefersSnapshotEventIdentity(t *testing.T) {
	startsAt := time.Date(2026, 6, 18, 2, 20, 28, 0, time.UTC)
	events := []domain.AlertEvent{
		{
			ID:                   42,
			AlertSourceProfileID: 7,
			Source:               "alertmanager",
			SourceFingerprint:    "source-fp",
			CanonicalFingerprint: "canon-fp",
			StartsAt:             startsAt,
		},
		{
			ID:                   43,
			AlertSourceProfileID: 9,
			Source:               "alertmanager",
			SourceFingerprint:    "source-fp",
			CanonicalFingerprint: "canon-fp",
			StartsAt:             startsAt.Add(time.Hour),
		},
	}
	snapshots := []domain.EvidenceSnapshot{
		{
			ID:           247,
			AlertGroupID: 31,
			Digest:       "sha256:precise",
			Payload: json.RawMessage(`{
				"events":[{
					"id":42,
					"source":"alertmanager",
					"alert_source_profile_id":7,
					"source_fingerprint":"source-fp",
					"canonical_fingerprint":"canon-fp",
					"starts_at":"2026-06-18T02:20:28Z"
				}]
			}`),
			Status:    domain.SnapshotStatusComplete,
			CreatedAt: startsAt.Add(time.Minute),
		},
		{
			ID:           248,
			AlertGroupID: 32,
			Digest:       "sha256:legacy",
			Payload: json.RawMessage(`{
				"events":[{"source_fingerprint":"source-fp","canonical_fingerprint":"canon-fp"}]
			}`),
			Status:    domain.SnapshotStatusComplete,
			CreatedAt: startsAt.Add(2 * time.Minute),
		},
		{
			ID:           249,
			AlertGroupID: 33,
			Digest:       "sha256:unknown-event-id",
			Payload: json.RawMessage(`{
				"events":[{
					"id":999,
					"source":"alertmanager",
					"alert_source_profile_id":7,
					"source_fingerprint":"source-fp",
					"canonical_fingerprint":"canon-fp",
					"starts_at":"2026-06-18T02:20:28Z"
				}]
			}`),
			Status:    domain.SnapshotStatusComplete,
			CreatedAt: startsAt.Add(3 * time.Minute),
		},
		{
			ID:           250,
			AlertGroupID: 34,
			Digest:       "sha256:incomplete-modern-identity",
			Payload: json.RawMessage(`{
				"events":[{
					"source":"alertmanager",
					"alert_source_profile_id":7,
					"source_fingerprint":"source-fp",
					"canonical_fingerprint":"canon-fp"
				}]
			}`),
			Status:    domain.SnapshotStatusComplete,
			CreatedAt: startsAt.Add(4 * time.Minute),
		},
	}

	links, err := alertEvidenceLinks(events, snapshots, nil)
	if err != nil {
		t.Fatalf("alertEvidenceLinks: %v", err)
	}
	if got := snapshotLinkIDs(links[42]); !slices.Equal(got, []int64{247, 248}) {
		t.Fatalf("alert 42 links = %v, want [247 248]", got)
	}
	if got := snapshotLinkIDs(links[43]); !slices.Equal(got, []int64{248}) {
		t.Fatalf("alert 43 links = %v, want legacy fallback only", got)
	}
}

func snapshotLinkIDs(links []api.AlertEvidenceSnapshotLink) []int64 {
	out := make([]int64, len(links))
	for i, link := range links {
		out[i] = link.ID
	}
	return out
}

func TestListAlerts_FiltersDiagnosisLinksWithoutRoomRead(t *testing.T) {
	startsAt := time.Date(2026, 6, 18, 2, 20, 28, 0, time.UTC)
	factory := &fakeUOWFactory{
		alertRepo: &fakeAlertRepo{
			events: []domain.AlertEvent{
				{
					ID:                   42,
					Source:               "alertmanager",
					SourceFingerprint:    "source-fp",
					CanonicalFingerprint: "canon-fp",
					Status:               domain.AlertStatusFiring,
					StartsAt:             startsAt,
					CreatedAt:            startsAt.Add(time.Second),
				},
			},
		},
		evidenceRepo: &fakeEvidenceRepo{
			snapshots: []domain.EvidenceSnapshot{
				{
					ID:           247,
					AlertGroupID: 31,
					Digest:       "sha256:linked",
					Payload: json.RawMessage(`{
						"schema_version":"m1.evidence_snapshot.v1",
						"events":[{"source_fingerprint":"source-fp","canonical_fingerprint":"canon-fp"}]
					}`),
					Provenance:        json.RawMessage(`{}`),
					Status:            domain.SnapshotStatusComplete,
					CreatedByWorkflow: "AlertmanagerWebhookAutoDiagnosis",
					CreatedAt:         startsAt.Add(2 * time.Second),
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              353,
						DiagnosisTaskID: 353,
						SessionKey:      "diagnosis-session-auto-p3-s247",
						Status:          domain.ChatSessionStatusOpen,
						StartedAt:       startsAt,
						LastActivityAt:  startsAt.Add(3 * time.Second),
						CreatedAt:       startsAt,
						UpdatedAt:       startsAt.Add(3 * time.Second),
					},
					Task: domain.DiagnosisTask{
						ID:                 353,
						EvidenceSnapshotID: 247,
						WorkflowID:         "diagnosis-room-diagnosis-session-auto-p3-s247",
						RunID:              "run-247",
						Status:             domain.DiagnosisStatusRunning,
						CreatedAt:          startsAt,
						UpdatedAt:          startsAt.Add(3 * time.Second),
					},
				},
			},
		},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed:   req.Permission == domain.RBACPermissionOperationsRead,
				CheckedAt: startsAt,
			}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/alerts?limit=1", nil)
	testOperationsReadHandlerWithAuthorizer(t, factory, authorizer).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.AlertListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || len(body.Items[0].LinkedEvidenceSnapshots) != 1 {
		t.Fatalf("linked evidence = %+v, want one alert with one evidence link", body.Items)
	}
	link := body.Items[0].LinkedEvidenceSnapshots[0]
	if link.ID != 247 || link.Digest != "sha256:linked" {
		t.Fatalf("unexpected evidence link: %+v", link)
	}
	if len(link.DiagnosisRooms) != 0 {
		t.Fatalf("diagnosis rooms = %+v, want filtered empty array", link.DiagnosisRooms)
	}
	if len(authorizer.requests) != 3 {
		t.Fatalf("authorizer requests = %+v, want operations, global room read, scoped room read", authorizer.requests)
	}
	if authorizer.requests[0].Permission != domain.RBACPermissionOperationsRead ||
		authorizer.requests[0].ScopeKind != domain.RBACScopeKindGlobal {
		t.Fatalf("operations read request = %+v", authorizer.requests[0])
	}
	if authorizer.requests[1].Permission != domain.RBACPermissionDiagnosisRoomRead ||
		authorizer.requests[1].ScopeKind != domain.RBACScopeKindGlobal {
		t.Fatalf("global room read request = %+v", authorizer.requests[1])
	}
	if authorizer.requests[2].Permission != domain.RBACPermissionDiagnosisRoomRead ||
		authorizer.requests[2].ScopeKind != domain.RBACScopeKindDiagnosisRoom ||
		authorizer.requests[2].ScopeKey != "diagnosis-session-auto-p3-s247" {
		t.Fatalf("scoped room read request = %+v", authorizer.requests[2])
	}
}

func TestListDiagnosisHandoffs_ReturnsPendingSnapshots(t *testing.T) {
	startsAt := time.Date(2026, 6, 18, 2, 20, 0, 0, time.UTC)
	createdAt := startsAt.Add(time.Minute)
	factory := &fakeUOWFactory{
		alertRepo: &fakeAlertRepo{
			events: []domain.AlertEvent{
				{
					ID:                   42,
					Source:               "alertmanager",
					SourceFingerprint:    "source-a",
					CanonicalFingerprint: "canon-a",
					Labels:               map[string]string{"alertname": "CheckoutLatencyHigh", "severity": "warning"},
					Annotations:          map[string]string{"summary": "Checkout p95 latency is high"},
					Status:               domain.AlertStatusFiring,
					StartsAt:             startsAt,
					CreatedAt:            startsAt.Add(time.Second),
				},
				{
					ID:                   43,
					Source:               "alertmanager",
					SourceFingerprint:    "source-b",
					CanonicalFingerprint: "canon-b",
					Labels:               map[string]string{"alertname": "PaymentErrorRateHigh", "severity": "warning"},
					Annotations:          map[string]string{"summary": "Payment error rate is high"},
					Status:               domain.AlertStatusFiring,
					StartsAt:             startsAt.Add(2 * time.Second),
					CreatedAt:            startsAt.Add(3 * time.Second),
				},
			},
		},
		evidenceRepo: &fakeEvidenceRepo{
			snapshots: []domain.EvidenceSnapshot{
				{
					ID:           247,
					AlertGroupID: 31,
					Digest:       "sha256:has-room",
					Payload: json.RawMessage(
						`{"events":[{"source_fingerprint":"source-a","canonical_fingerprint":"canon-a"}]}`,
					),
					Status:            domain.SnapshotStatusComplete,
					CreatedByWorkflow: "AlertmanagerWebhookAutoDiagnosis",
					CreatedAt:         createdAt,
				},
				{
					ID:           248,
					AlertGroupID: 32,
					Digest:       "sha256:needs-room-two-alerts",
					Payload: json.RawMessage(
						`{"events":[{"source_fingerprint":"source-a","canonical_fingerprint":"canon-a"},{"source_fingerprint":"source-b","canonical_fingerprint":"canon-b"}]}`,
					),
					Status:            domain.SnapshotStatusComplete,
					CreatedByWorkflow: "AlertmanagerWebhookAutoDiagnosis",
					CreatedAt:         createdAt.Add(time.Minute),
				},
				{
					ID:           249,
					AlertGroupID: 33,
					Digest:       "sha256:needs-room-latest",
					Payload: json.RawMessage(
						`{"events":[{"source_fingerprint":"source-b","canonical_fingerprint":"canon-b"}]}`,
					),
					Status:            domain.SnapshotStatusComplete,
					CreatedByWorkflow: "AlertmanagerWebhookAutoDiagnosis",
					CreatedAt:         createdAt.Add(2 * time.Minute),
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              353,
						DiagnosisTaskID: 353,
						SessionKey:      "diagnosis-session-auto-p3-s247",
						Status:          domain.ChatSessionStatusOpen,
						StartedAt:       createdAt,
						LastActivityAt:  createdAt.Add(time.Second),
						CreatedAt:       createdAt,
						UpdatedAt:       createdAt.Add(time.Second),
					},
					Task: domain.DiagnosisTask{
						ID:                 353,
						EvidenceSnapshotID: 247,
						WorkflowID:         "diagnosis-room-diagnosis-session-auto-p3-s247",
						RunID:              "run-247",
						Status:             domain.DiagnosisStatusRunning,
						CreatedAt:          createdAt,
						UpdatedAt:          createdAt.Add(time.Second),
					},
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/handoffs?limit=2", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.alertRepo.lastLimit != maxListLimit ||
		factory.evidenceRepo.lastLimit != maxListLimit ||
		factory.diagnosisRepo.lastChatSessionLimit != maxListLimit {
		t.Fatalf(
			"limits alert=%d evidence=%d rooms=%d, want %d",
			factory.alertRepo.lastLimit,
			factory.evidenceRepo.lastLimit,
			factory.diagnosisRepo.lastChatSessionLimit,
			maxListLimit,
		)
	}

	var body api.DiagnosisHandoffListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("len(items) = %d, want 2: %+v", len(body.Items), body.Items)
	}
	if body.Items[0].EvidenceSnapshot.ID != 249 || body.Items[1].EvidenceSnapshot.ID != 248 {
		t.Fatalf("snapshot order = [%d %d], want [249 248]", body.Items[0].EvidenceSnapshot.ID, body.Items[1].EvidenceSnapshot.ID)
	}
	if body.Items[0].Reason != api.MissingDiagnosisRoom || body.Items[1].Reason != api.MissingDiagnosisRoom {
		t.Fatalf("reasons = [%s %s]", body.Items[0].Reason, body.Items[1].Reason)
	}
	if len(body.Items[0].EvidenceSnapshot.DiagnosisRooms) != 0 || len(body.Items[1].EvidenceSnapshot.DiagnosisRooms) != 0 {
		t.Fatalf("pending handoffs should not include diagnosis rooms: %+v", body.Items)
	}
	if len(body.Items[0].Alerts) != 1 || body.Items[0].Alerts[0].ID != 43 {
		t.Fatalf("latest snapshot alerts = %+v, want alert 43", body.Items[0].Alerts)
	}
	if len(body.Items[1].Alerts) != 2 || body.Items[1].Alerts[0].ID != 42 || body.Items[1].Alerts[1].ID != 43 {
		t.Fatalf("grouped snapshot alerts = %+v, want alerts 42 and 43", body.Items[1].Alerts)
	}
}

func TestListDiagnosisHandoffsRequiresRoomParticipation(t *testing.T) {
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed:   req.Permission == domain.RBACPermissionOperationsRead,
				CheckedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	factory := &fakeUOWFactory{configRepo: &fakeConfigRepo{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/handoffs?limit=2", nil)
	addTestLocalRBACAuthorization(req)

	testHandler(factory, testLocalRBACOptions(t, "operations-viewer-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 2 ||
		authorizer.requests[0].Permission != domain.RBACPermissionOperationsRead ||
		authorizer.requests[1].Permission != domain.RBACPermissionDiagnosisRoomParticipate {
		t.Fatalf("authorizer requests = %+v called=%d", authorizer.requests, authorizer.called)
	}
}

func TestListEvidenceSnapshots_ReturnsSummaries(t *testing.T) {
	createdAt := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		evidenceRepo: &fakeEvidenceRepo{
			snapshots: []domain.EvidenceSnapshot{
				{
					ID:                7,
					AlertGroupID:      5,
					Digest:            "digest-1",
					Payload:           json.RawMessage(`{"metric":"cpu"}`),
					Provenance:        json.RawMessage(`{"prometheus":"ok"}`),
					Status:            domain.SnapshotStatusComplete,
					CreatedByWorkflow: "DiagnosisWorkflow",
					CreatedAt:         createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/evidence-snapshots?limit=1", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.evidenceRepo.lastLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.evidenceRepo.lastLimit)
	}

	var body api.EvidenceSnapshotListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.ID != 7 || got.AlertGroupID != 5 || got.Payload["metric"] != "cpu" || got.Provenance["prometheus"] != "ok" {
		t.Fatalf("unexpected evidence summary: %+v", got)
	}
	if got.MissingFields == nil {
		t.Fatalf("missing_fields should be an empty array, not null")
	}
}

func TestListReports_ReturnsSummaries(t *testing.T) {
	createdAt := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{
					ID:                11,
					CorrelationKey:    "window:checkout",
					Title:             "Checkout latency incident",
					ExecutiveSummary:  "Checkout latency increased after a deployment.",
					Severity:          domain.ReportSeverityWarning,
					Confidence:        domain.ReportConfidenceHigh,
					NotificationText:  "Checkout latency incident requires review.",
					CreatedByWorkflow: "FinalReportWorkflow",
					CreatedAt:         createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports?limit=1", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.reportRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.reportRepo.lastListLimit)
	}

	var body api.ReportListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.ID != 11 || got.CorrelationKey != "window:checkout" || string(got.Severity) != string(domain.ReportSeverityWarning) {
		t.Fatalf("unexpected report summary: %+v", got)
	}
}

func TestOperationsReadRejectsUnauthenticatedPrincipal(t *testing.T) {
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{},
		reportRepo: &fakeReportRepo{},
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports?limit=1", nil)
	testHandler(factory, testLocalRBACOptions(t, "operations-viewer-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer calls = %d, want 0", authorizer.called)
	}
	if factory.reportRepo.lastListLimit != 0 {
		t.Fatalf("report repo last limit = %d, want 0", factory.reportRepo.lastListLimit)
	}
}

func TestListDiagnosisRooms_ReturnsSummaries(t *testing.T) {
	startedAt := time.Date(2026, 6, 18, 2, 20, 28, 0, time.UTC)
	closedAt := startedAt.Add(5 * time.Minute)
	notificationAt := startedAt.Add(90 * time.Second)
	progressAt := startedAt.Add(2 * time.Minute)
	finalAt := startedAt.Add(3 * time.Minute)
	requiresReview := true
	finalPayload, err := json.Marshal(diagnosisRoomConclusionEventPayload{
		Kind:            diagnosisConclusionEventFinalReady,
		SessionID:       "diagnosis-session-auto-p3-s247",
		ChatSessionID:   353,
		DiagnosisTaskID: 353,
		FinalConclusion: diagnosisRoomConclusionPayload{
			Status:              "available",
			Source:              "assistant",
			EvidenceSnapshotID:  247,
			ConclusionVersion:   "diagnosis-session-auto-p3-s247:2",
			ConfirmedBy:         "operator:alice",
			Content:             "Database storage saturation is the primary outage risk.",
			Confidence:          "high",
			RequiresHumanReview: &requiresReview,
		},
	})
	if err != nil {
		t.Fatalf("marshal final payload: %v", err)
	}
	notificationPayload := json.RawMessage(`{
		"kind":"diagnosis_room.assistant_turn_notification_sent",
		"session_id":"diagnosis-session-auto-p3-s247",
		"chat_session_id":353,
		"diagnosis_task_id":353,
		"owner_subject":"operator:alice",
		"assistant_message_id":"msg-1/assistant",
		"assistant_turn_id":354,
		"assistant_sequence":2,
		"turn_count":1,
		"idempotency_key":"diagnosis_room:353:abc/assistant_turn_notification",
		"notification_channel_profile_id":2,
		"provider_message_id":"wecom-msg-1",
		"provider_status":"delivered",
		"provider_raw":{"errcode":0},
		"assistant_message":"Internal diagnosis body should not be copied into list responses.",
		"confidence":"low",
		"requires_human_review":true
	}`)
	progressPayload := json.RawMessage(`{
		"kind":"diagnosis_room.turn_persisted",
		"session_id":"diagnosis-session-auto-p3-s247",
		"chat_session_id":353,
		"diagnosis_task_id":353,
		"assistant_message_id":"msg-1/assistant",
		"assistant_turn_id":354,
		"assistant_sequence":2,
		"turn_count":1,
		"confidence":"low",
		"requires_human_review":true,
		"evidence_requests":[{"tool":"active_alerts","reason":"Check related active alerts.","limit":5}],
		"consultation_insight":{
			"conclusion_status":"needs_evidence",
			"confidence_rationale":"Initial evidence needs bounded active-alert confirmation.",
			"missing_evidence_requests":[{"label":"Owner action","detail":"Attach the current storage expansion status.","priority":"high"}],
			"evidence_collection_suggestions":[{"label":"Recent active alerts","detail":"Collect active alerts for the same service before final confirmation.","priority":"medium"}]
		}
	}`)
	evidenceCollectedPayload := json.RawMessage(`{
		"kind":"diagnosis_room.evidence_collected",
		"session_id":"diagnosis-session-auto-p3-s247",
		"chat_session_id":353,
		"diagnosis_task_id":353,
		"actor_subject":"operator:carol",
		"user_message_id":"msg-3",
		"assistant_message_id":"msg-1/assistant",
		"user_turn_id":357,
		"assistant_turn_id":354,
		"user_sequence":5,
		"assistant_sequence":2,
		"turn_count":1,
		"evidence_collection_results":[{
			"tool":"active_alerts",
			"status":"collected",
			"reason_code":"ok",
			"message":"Collected active alerts.",
			"observed_alerts":2,
			"collected_at":"2026-06-18T02:22:45Z"
		}]
	}`)
	supplementalPayload := json.RawMessage(`{
		"kind":"diagnosis_room.supplemental_evidence_provided",
		"session_id":"diagnosis-session-auto-p3-s247",
		"chat_session_id":353,
		"diagnosis_task_id":353,
		"actor_subject":"operator:bob",
		"user_message_id":"msg-2",
		"assistant_message_id":"msg-2/assistant",
		"user_turn_id":355,
		"assistant_turn_id":356,
		"user_sequence":3,
		"assistant_sequence":4,
		"supplemental_evidence":{
			"label":"Storage owner update",
			"detail":"Attach current storage expansion status.",
			"priority":"high",
			"evidence":"Expansion ticket is approved and scheduled."
		}
	}`)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			directoryUsers: []domain.DirectoryUser{
				testDirectoryUser(1, "operator:alice", "Alice Chen"),
				testDirectoryUser(2, "operator:bob", "Bob Li"),
				testDirectoryUser(3, "operator:carol", "Carol Wu"),
				testDirectoryUser(4, "openclarion:auto-diagnosis", "Automation"),
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			chatSessionSummaries: map[domain.ChatSessionID]domain.ChatSessionSummary{
				353: {
					ID:                  701,
					SessionID:           353,
					Version:             1,
					SchemaVersion:       "diagnosis-conversation-summary.v1",
					SourceFirstSequence: 1,
					SourceLastSequence:  2,
					SourceTurnCount:     2,
					SourceDigest:        "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					Content:             json.RawMessage(`{"schema_version":"diagnosis-conversation-summary.v1","compression_method":"deterministic-extractive","source_turn_count":2,"opening_request":"Please investigate","latest_request":"Please investigate","latest_assistant_response":"Storage evidence is needed."}`),
					GeneratedAt:         closedAt,
				},
			},
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				353: {
					diagnosisConclusionEventTurnPersisted: {{
						ID:         89,
						TaskID:     353,
						Kind:       diagnosisConclusionEventTurnPersisted,
						Payload:    progressPayload,
						OccurredAt: progressAt,
						RecordedAt: progressAt.Add(time.Second),
					}},
					diagnosisConclusionEventEvidenceCollected: {{
						ID:         88,
						TaskID:     353,
						Kind:       diagnosisConclusionEventEvidenceCollected,
						Payload:    evidenceCollectedPayload,
						OccurredAt: progressAt.Add(45 * time.Second),
						RecordedAt: progressAt.Add(46 * time.Second),
					}},
					diagnosisConclusionEventAssistantTurnNotification: {{
						ID:         90,
						TaskID:     353,
						Kind:       diagnosisConclusionEventAssistantTurnNotification,
						Payload:    notificationPayload,
						OccurredAt: notificationAt,
						RecordedAt: notificationAt.Add(time.Second),
					}},
					diagnosisConclusionEventFinalReady: {{
						ID:         91,
						TaskID:     353,
						Kind:       diagnosisConclusionEventFinalReady,
						Payload:    finalPayload,
						OccurredAt: finalAt,
						RecordedAt: finalAt.Add(time.Second),
					}},
					diagnosisConclusionEventSupplementalEvidence: {{
						ID:         92,
						TaskID:     353,
						Kind:       diagnosisConclusionEventSupplementalEvidence,
						Payload:    supplementalPayload,
						OccurredAt: progressAt.Add(30 * time.Second),
						RecordedAt: progressAt.Add(31 * time.Second),
					}},
				},
			},
			chatTurnsBySession: map[domain.ChatSessionID][]domain.ChatTurn{
				353: {
					{SessionID: 353, MessageID: "msg-1", Sequence: 1, Role: domain.ChatRoleUser, ActorSubject: "operator:alice", Content: "Please investigate", OccurredAt: startedAt.Add(time.Minute)},
					{SessionID: 353, MessageID: "msg-1/assistant", Sequence: 2, Role: domain.ChatRoleAssistant, ActorSubject: "openclarion:auto-diagnosis", Content: "Storage evidence is needed.", OccurredAt: progressAt},
				},
			},
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              353,
						DiagnosisTaskID: 353,
						SessionKey:      "diagnosis-session-auto-p3-s247",
						OwnerSubject:    "operator:alice",
						Status:          domain.ChatSessionStatusClosed,
						TurnCount:       2,
						StartedAt:       startedAt,
						LastActivityAt:  closedAt,
						ClosedAt:        &closedAt,
						CloseReason:     "human_confirmed",
						CreatedAt:       startedAt,
						UpdatedAt:       closedAt,
					},
					Task: domain.DiagnosisTask{
						ID:                 353,
						EvidenceSnapshotID: 247,
						WorkflowID:         "diagnosis-room-diagnosis-session-auto-p3-s247",
						RunID:              "run-247",
						Status:             domain.DiagnosisStatusRunning,
					},
				},
			},
		},
	}
	visibility := &fakeDiagnosisRoomWorkflowVisibilityLookup{
		results: map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility{
			{WorkflowID: "diagnosis-room-diagnosis-session-auto-p3-s247", RunID: "run-247"}: {
				WorkflowID:       "diagnosis-room-diagnosis-session-auto-p3-s247",
				RunID:            "run-247",
				Status:           "running",
				TaskQueue:        "openclarion",
				StartTime:        &startedAt,
				HistoryLength:    51,
				HistorySizeBytes: 4096,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/rooms?limit=1", nil)
	testConfigHandler(t, factory, WithDiagnosisRoomWorkflowVisibilityLookup(visibility)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	rawBody := rec.Body.String()
	if factory.diagnosisRepo.lastChatSessionLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.diagnosisRepo.lastChatSessionLimit)
	}

	var body api.DiagnosisRoomListResponse
	if err := json.NewDecoder(strings.NewReader(rawBody)).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.SessionID != "diagnosis-session-auto-p3-s247" ||
		got.ChatSessionID != 353 ||
		got.DiagnosisTaskID != 353 ||
		got.EvidenceSnapshotID != 247 ||
		got.WorkflowID != "diagnosis-room-diagnosis-session-auto-p3-s247" ||
		got.RunID != "run-247" ||
		got.TaskStatus != api.DiagnosisTaskStatusRunning ||
		got.RoomStatus != api.Closed ||
		got.TurnCount != 2 ||
		got.CloseReason != "human_confirmed" {
		t.Fatalf("unexpected diagnosis room summary: %+v", got)
	}
	if got.ClosedAt.IsNull() {
		t.Fatalf("closed_at should be populated for closed rooms")
	}
	if got.ConversationSummary == nil ||
		got.ConversationSummary.ID != 701 ||
		got.ConversationSummary.SourceTurnCount != 2 ||
		got.ConversationSummary.Content.LatestAssistantResponse == nil ||
		*got.ConversationSummary.Content.LatestAssistantResponse != "Storage evidence is needed." {
		t.Fatalf("conversation summary = %+v", got.ConversationSummary)
	}
	if visibility.called != 1 ||
		len(visibility.requests) != 1 ||
		visibility.requests[0].WorkflowID != "diagnosis-room-diagnosis-session-auto-p3-s247" ||
		visibility.requests[0].RunID != "run-247" {
		t.Fatalf("visibility requests = %+v called=%d", visibility.requests, visibility.called)
	}
	if got.WorkflowVisibility == nil ||
		got.WorkflowVisibility.Status != "running" ||
		got.WorkflowVisibility.TaskQueue == nil ||
		*got.WorkflowVisibility.TaskQueue != "openclarion" ||
		got.WorkflowVisibility.StartTime == nil ||
		!got.WorkflowVisibility.StartTime.Equal(startedAt) ||
		got.WorkflowVisibility.HistoryLength == nil ||
		*got.WorkflowVisibility.HistoryLength != 51 ||
		got.WorkflowVisibility.HistorySizeBytes == nil ||
		*got.WorkflowVisibility.HistorySizeBytes != 4096 {
		t.Fatalf("workflow_visibility = %+v", got.WorkflowVisibility)
	}
	if got.LatestConclusion == nil {
		t.Fatalf("latest_conclusion = nil, want populated")
	}
	if got.LatestConclusion.EventKind != diagnosisConclusionEventFinalReady ||
		got.LatestConclusion.Content != "Database storage saturation is the primary outage risk." ||
		got.LatestConclusion.ConfirmedBy == nil ||
		*got.LatestConclusion.ConfirmedBy != "operator:alice" ||
		got.LatestConclusion.Confidence == nil ||
		*got.LatestConclusion.Confidence != api.ReportConfidenceHigh ||
		got.LatestConclusion.RequiresHumanReview == nil ||
		!*got.LatestConclusion.RequiresHumanReview {
		t.Fatalf("unexpected latest conclusion: %+v", got.LatestConclusion)
	}
	if len(got.NotificationTimeline) != 1 {
		t.Fatalf("len(notification_timeline) = %d, want 1", len(got.NotificationTimeline))
	}
	if len(got.Participants) != 4 {
		t.Fatalf("participants = %+v, want 4", got.Participants)
	}
	if got.Participants[0].Subject != "operator:alice" ||
		got.Participants[0].MessageCount != 1 ||
		!got.Participants[0].ConfirmedConclusion ||
		!slices.Equal(got.Participants[0].Roles, []api.DiagnosisRoomParticipantSummaryRolesItem{
			api.DiagnosisRoomParticipantSummaryRolesItemOwner,
			api.DiagnosisRoomParticipantSummaryRolesItemMessage,
			api.DiagnosisRoomParticipantSummaryRolesItemConfirmation,
		}) {
		t.Fatalf("owner participant = %+v", got.Participants[0])
	}
	if got.Participants[1].Subject != "operator:bob" ||
		got.Participants[1].SupplementalEvidenceCount != 1 ||
		!slices.Equal(got.Participants[1].Roles, []api.DiagnosisRoomParticipantSummaryRolesItem{
			api.DiagnosisRoomParticipantSummaryRolesItemSupplementalEvidence,
		}) {
		t.Fatalf("supplemental participant = %+v", got.Participants[1])
	}
	if got.Participants[2].Subject != "operator:carol" ||
		got.Participants[2].EvidenceCollectionCount != 1 ||
		!slices.Equal(got.Participants[2].Roles, []api.DiagnosisRoomParticipantSummaryRolesItem{
			api.DiagnosisRoomParticipantSummaryRolesItemEvidence,
		}) {
		t.Fatalf("evidence participant = %+v", got.Participants[2])
	}
	if got.Participants[3].Subject != "openclarion:auto-diagnosis" ||
		!got.Participants[3].IsSystem ||
		got.Participants[3].MessageCount != 1 ||
		!slices.Equal(got.Participants[3].Roles, []api.DiagnosisRoomParticipantSummaryRolesItem{
			api.DiagnosisRoomParticipantSummaryRolesItemAssistant,
		}) {
		t.Fatalf("assistant participant = %+v", got.Participants[3])
	}
	participantDirectorySubjects := make([]string, 0, len(got.ParticipantDirectoryUsers))
	for _, user := range got.ParticipantDirectoryUsers {
		participantDirectorySubjects = append(participantDirectorySubjects, user.Subject)
	}
	if !slices.Equal(participantDirectorySubjects, []string{"operator:alice", "operator:bob", "operator:carol"}) {
		t.Fatalf("participant_directory_users subjects = %+v", participantDirectorySubjects)
	}
	if got.ParticipantDirectoryUsers[0].DisplayName != "Alice Chen" ||
		got.ParticipantDirectoryUsers[0].DepartmentPath != "IT/Platform/SRE" {
		t.Fatalf("participant directory user = %+v", got.ParticipantDirectoryUsers[0])
	}
	notification := got.NotificationTimeline[0]
	if notification.EventKind != diagnosisConclusionEventAssistantTurnNotification ||
		notification.NotificationChannelProfileID == nil ||
		*notification.NotificationChannelProfileID != 2 ||
		notification.ProviderStatus != "delivered" ||
		notification.ProviderMessageID == nil ||
		*notification.ProviderMessageID != "wecom-msg-1" ||
		notification.AssistantMessageID == nil ||
		*notification.AssistantMessageID != "msg-1/assistant" ||
		notification.AssistantTurnID == nil ||
		*notification.AssistantTurnID != 354 ||
		notification.AssistantSequence == nil ||
		*notification.AssistantSequence != 2 ||
		notification.TurnCount == nil ||
		*notification.TurnCount != 1 ||
		notification.Confidence == nil ||
		*notification.Confidence != api.ReportConfidenceLow ||
		notification.RequiresHumanReview == nil ||
		!*notification.RequiresHumanReview ||
		notification.ContentKind == nil ||
		*notification.ContentKind != "assistant_message" ||
		notification.ContentSha256 == nil ||
		*notification.ContentSha256 != testSHA256("Internal diagnosis body should not be copied into list responses.") ||
		!notification.OccurredAt.Equal(notificationAt) {
		t.Fatalf("unexpected notification timeline entry: %+v", notification)
	}
	for _, forbidden := range []string{"provider_raw", "idempotency_key", `"assistant_message":`, "memo", "search_attributes"} {
		if strings.Contains(rawBody, forbidden) {
			t.Fatalf("response leaked internal notification field %q: %s", forbidden, rawBody)
		}
	}
}

func TestListDiagnosisRooms_FiltersByScopedRBACWhenGlobalReadIsDenied(t *testing.T) {
	startedAt := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			directoryUsers: []domain.DirectoryUser{
				testDirectoryUser(11, "operator:visible", "Visible Operator"),
				testDirectoryUser(12, "operator:hidden", "Hidden Operator"),
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              501,
						DiagnosisTaskID: 601,
						SessionKey:      "room-visible",
						OwnerSubject:    "operator:visible",
						Status:          domain.ChatSessionStatusOpen,
						StartedAt:       startedAt,
						LastActivityAt:  startedAt.Add(time.Minute),
						CreatedAt:       startedAt,
						UpdatedAt:       startedAt.Add(time.Minute),
					},
					Task: domain.DiagnosisTask{
						ID:                 601,
						EvidenceSnapshotID: 701,
						WorkflowID:         "diagnosis-room-room-visible",
						RunID:              "run-visible",
						Status:             domain.DiagnosisStatusRunning,
					},
				},
				{
					Session: domain.ChatSession{
						ID:              502,
						DiagnosisTaskID: 602,
						SessionKey:      "room-hidden",
						OwnerSubject:    "operator:hidden",
						Status:          domain.ChatSessionStatusOpen,
						StartedAt:       startedAt,
						LastActivityAt:  startedAt.Add(2 * time.Minute),
						CreatedAt:       startedAt,
						UpdatedAt:       startedAt.Add(2 * time.Minute),
					},
					Task: domain.DiagnosisTask{
						ID:                 602,
						EvidenceSnapshotID: 702,
						WorkflowID:         "diagnosis-room-room-hidden",
						RunID:              "run-hidden",
						Status:             domain.DiagnosisStatusRunning,
					},
				},
			},
		},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed: req.ScopeKind == domain.RBACScopeKindDiagnosisRoom &&
					req.ScopeKey == "room-visible",
				CheckedAt: startedAt,
			}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/rooms?limit=10", nil)
	addTestLocalRBACAuthorization(req)
	testHandler(factory, testLocalRBACOptions(t, "responder-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.DiagnosisRoomListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].SessionID != "room-visible" {
		t.Fatalf("items = %+v, want only room-visible", body.Items)
	}
	if factory.configRepo.lastDirectorySubject != "operator:visible" {
		t.Fatalf("directory subject = %q, want operator:visible", factory.configRepo.lastDirectorySubject)
	}
	if len(body.Items[0].ParticipantDirectoryUsers) != 1 ||
		body.Items[0].ParticipantDirectoryUsers[0].Subject != "operator:visible" {
		t.Fatalf("participant_directory_users = %+v, want only visible room user", body.Items[0].ParticipantDirectoryUsers)
	}
	if authorizer.called != 3 {
		t.Fatalf("authorizer calls = %d, want global plus two scoped checks", authorizer.called)
	}
	if authorizer.requests[0].ScopeKind != domain.RBACScopeKindGlobal ||
		authorizer.requests[0].Permission != domain.RBACPermissionDiagnosisRoomRead {
		t.Fatalf("global auth request = %+v", authorizer.requests[0])
	}
	if authorizer.requests[1].ScopeKey != "room-visible" ||
		authorizer.requests[2].ScopeKey != "room-hidden" {
		t.Fatalf("scoped auth requests = %+v", authorizer.requests)
	}
}

func TestListDiagnosisRooms_FiltersBeforeLimitForScopedRBAC(t *testing.T) {
	startedAt := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			directoryUsers: []domain.DirectoryUser{
				testDirectoryUser(11, "operator:visible", "Visible Operator"),
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              502,
						DiagnosisTaskID: 602,
						SessionKey:      "room-hidden-newer",
						OwnerSubject:    "operator:hidden",
						Status:          domain.ChatSessionStatusOpen,
						StartedAt:       startedAt.Add(2 * time.Minute),
						LastActivityAt:  startedAt.Add(2 * time.Minute),
						CreatedAt:       startedAt.Add(2 * time.Minute),
						UpdatedAt:       startedAt.Add(2 * time.Minute),
					},
					Task: domain.DiagnosisTask{
						ID:                 602,
						EvidenceSnapshotID: 702,
						WorkflowID:         "diagnosis-room-room-hidden-newer",
						RunID:              "run-hidden",
						Status:             domain.DiagnosisStatusRunning,
					},
				},
				{
					Session: domain.ChatSession{
						ID:              501,
						DiagnosisTaskID: 601,
						SessionKey:      "room-visible-older",
						OwnerSubject:    "operator:visible",
						Status:          domain.ChatSessionStatusOpen,
						StartedAt:       startedAt,
						LastActivityAt:  startedAt.Add(time.Minute),
						CreatedAt:       startedAt,
						UpdatedAt:       startedAt.Add(time.Minute),
					},
					Task: domain.DiagnosisTask{
						ID:                 601,
						EvidenceSnapshotID: 701,
						WorkflowID:         "diagnosis-room-room-visible-older",
						RunID:              "run-visible",
						Status:             domain.DiagnosisStatusRunning,
					},
				},
			},
		},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed: req.ScopeKind == domain.RBACScopeKindDiagnosisRoom &&
					req.ScopeKey == "room-visible-older",
				CheckedAt: startedAt,
			}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/rooms?limit=1", nil)
	addTestLocalRBACAuthorization(req)
	testHandler(factory, testLocalRBACOptions(t, "responder-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.diagnosisRepo.lastChatSessionLimit != maxListLimit {
		t.Fatalf("repo limit = %d, want maxListLimit for scoped RBAC", factory.diagnosisRepo.lastChatSessionLimit)
	}
	var body api.DiagnosisRoomListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].SessionID != "room-visible-older" {
		t.Fatalf("items = %+v, want visible older room after RBAC filtering", body.Items)
	}
}

func TestListDiagnosisRooms_PagesScopedRBACPastFirstGlobalPage(t *testing.T) {
	startedAt := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	sessions := make([]domain.ChatSessionWithTask, 0, maxListLimit+1)
	for i := 0; i < maxListLimit; i++ {
		taskID := domain.DiagnosisTaskID(2000 + i)
		sessionKey := fmt.Sprintf("room-hidden-%03d", i)
		updatedAt := startedAt.Add(time.Duration(maxListLimit-i) * time.Minute)
		sessions = append(sessions, domain.ChatSessionWithTask{
			Session: domain.ChatSession{
				ID:              domain.ChatSessionID(1000 + i),
				DiagnosisTaskID: taskID,
				SessionKey:      sessionKey,
				OwnerSubject:    "operator:hidden",
				Status:          domain.ChatSessionStatusOpen,
				StartedAt:       updatedAt,
				LastActivityAt:  updatedAt,
				CreatedAt:       updatedAt,
				UpdatedAt:       updatedAt,
			},
			Task: domain.DiagnosisTask{
				ID:                 taskID,
				EvidenceSnapshotID: domain.EvidenceSnapshotID(3000 + i),
				WorkflowID:         "diagnosis-room-" + sessionKey,
				RunID:              "run-" + sessionKey,
				Status:             domain.DiagnosisStatusRunning,
			},
		})
	}
	visibleTaskID := domain.DiagnosisTaskID(9001)
	visibleAt := startedAt.Add(-time.Minute)
	sessions = append(sessions, domain.ChatSessionWithTask{
		Session: domain.ChatSession{
			ID:              9001,
			DiagnosisTaskID: visibleTaskID,
			SessionKey:      "room-visible-after-first-page",
			OwnerSubject:    "operator:visible",
			Status:          domain.ChatSessionStatusOpen,
			StartedAt:       visibleAt,
			LastActivityAt:  visibleAt,
			CreatedAt:       visibleAt,
			UpdatedAt:       visibleAt,
		},
		Task: domain.DiagnosisTask{
			ID:                 visibleTaskID,
			EvidenceSnapshotID: 9901,
			WorkflowID:         "diagnosis-room-room-visible-after-first-page",
			RunID:              "run-visible-after-first-page",
			Status:             domain.DiagnosisStatusRunning,
		},
	})
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			directoryUsers: []domain.DirectoryUser{
				testDirectoryUser(11, "operator:visible", "Visible Operator"),
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{chatSessions: sessions},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed: req.ScopeKind == domain.RBACScopeKindDiagnosisRoom &&
					req.ScopeKey == "room-visible-after-first-page",
				CheckedAt: startedAt,
			}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/rooms?limit=1", nil)
	addTestLocalRBACAuthorization(req)
	testHandler(factory, testLocalRBACOptions(t, "responder-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.DiagnosisRoomListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].SessionID != "room-visible-after-first-page" {
		t.Fatalf("items = %+v, want visible room from second page", body.Items)
	}
	if len(factory.diagnosisRepo.chatSessionPageCalls) != 2 ||
		factory.diagnosisRepo.chatSessionPageCalls[0] != (chatSessionPageCall{limit: maxListLimit, offset: 0}) ||
		factory.diagnosisRepo.chatSessionPageCalls[1] != (chatSessionPageCall{limit: maxListLimit, offset: maxListLimit}) {
		t.Fatalf("chat session page calls = %+v, want first two pages", factory.diagnosisRepo.chatSessionPageCalls)
	}
	if authorizer.called != maxListLimit+2 {
		t.Fatalf("authorizer calls = %d, want global plus scoped rooms through the second page", authorizer.called)
	}
}

func TestGetDiagnosisRoom_ReturnsExactSummary(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 3, 14, 25, 0, time.UTC)
	notificationAt := startedAt.Add(95 * time.Second)
	finalAt := startedAt.Add(2 * time.Minute)
	requiresReview := true
	finalPayload, err := json.Marshal(diagnosisRoomConclusionEventPayload{
		Kind:            diagnosisConclusionEventFinalReady,
		SessionID:       "diagnosis-session-exact",
		ChatSessionID:   703,
		DiagnosisTaskID: 703,
		FinalConclusion: diagnosisRoomConclusionPayload{
			Status:              "available",
			Source:              "assistant",
			EvidenceSnapshotID:  907,
			ConclusionVersion:   "diagnosis-session-exact:2",
			Content:             "Checkout latency is ready for operator confirmation.",
			Confidence:          "high",
			RequiresHumanReview: &requiresReview,
		},
	})
	if err != nil {
		t.Fatalf("marshal final payload: %v", err)
	}
	notificationPayload := json.RawMessage(`{
		"kind":"diagnosis_room.final_ready_notification_sent",
		"session_id":"diagnosis-session-exact",
		"chat_session_id":703,
		"diagnosis_task_id":703,
		"assistant_message_id":"msg-exact/assistant",
		"assistant_turn_id":704,
		"assistant_sequence":2,
		"turn_count":2,
		"idempotency_key":"diagnosis_room:703:exact/final_ready",
		"notification_channel_profile_id":2,
		"provider_message_id":"wecom-msg-exact-final-ready",
		"provider_status":"delivered",
		"provider_raw":{"errcode":0},
		"confidence":"high",
		"requires_human_review":true,
		"final_conclusion":{
			"content":"Checkout latency is ready for operator confirmation.",
			"confidence":"high",
			"requires_human_review":true,
			"recommended_actions":["Confirm the checkout deployment status."],
			"evidence_requests":[{"tool":"active_alerts","reason":"Confirm current alert state."}]
		}
	}`)
	repo := &fakeDiagnosisRepo{
		eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
			703: {
				diagnosisConclusionEventFinalReady: {{
					ID:         171,
					TaskID:     703,
					Kind:       diagnosisConclusionEventFinalReady,
					Payload:    finalPayload,
					OccurredAt: finalAt,
					RecordedAt: finalAt.Add(time.Second),
				}},
				diagnosisConclusionEventFinalReadyNotification: {{
					ID:         172,
					TaskID:     703,
					Kind:       diagnosisConclusionEventFinalReadyNotification,
					Payload:    notificationPayload,
					OccurredAt: notificationAt,
					RecordedAt: notificationAt.Add(time.Second),
				}},
			},
		},
		chatSessions: []domain.ChatSessionWithTask{
			{
				Session: domain.ChatSession{
					ID:              702,
					DiagnosisTaskID: 702,
					SessionKey:      "diagnosis-session-other",
					Status:          domain.ChatSessionStatusOpen,
					TurnCount:       1,
					StartedAt:       startedAt.Add(-time.Hour),
					LastActivityAt:  startedAt.Add(-time.Hour),
					CreatedAt:       startedAt.Add(-time.Hour),
					UpdatedAt:       startedAt.Add(-time.Hour),
				},
				Task: domain.DiagnosisTask{
					ID:                 702,
					EvidenceSnapshotID: 906,
					WorkflowID:         "diagnosis-room-diagnosis-session-other",
					RunID:              "run-other",
					Status:             domain.DiagnosisStatusRunning,
				},
			},
			{
				Session: domain.ChatSession{
					ID:              703,
					DiagnosisTaskID: 703,
					SessionKey:      "diagnosis-session-exact",
					OwnerSubject:    "operator:exact",
					Status:          domain.ChatSessionStatusOpen,
					TurnCount:       2,
					StartedAt:       startedAt,
					LastActivityAt:  finalAt,
					CreatedAt:       startedAt,
					UpdatedAt:       finalAt,
				},
				Task: domain.DiagnosisTask{
					ID:                 703,
					EvidenceSnapshotID: 907,
					WorkflowID:         "diagnosis-room-diagnosis-session-exact",
					RunID:              "run-exact",
					Status:             domain.DiagnosisStatusRunning,
				},
			},
		},
	}
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			directoryUsers: []domain.DirectoryUser{
				testDirectoryUser(21, "operator:exact", "Exact Operator"),
				testDirectoryUser(22, "operator:other", "Other Operator"),
			},
		},
		diagnosisRepo: repo,
	}
	visibility := &fakeDiagnosisRoomWorkflowVisibilityLookup{
		results: map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility{
			{WorkflowID: "diagnosis-room-diagnosis-session-exact", RunID: "run-exact"}: {
				WorkflowID:    "diagnosis-room-diagnosis-session-exact",
				RunID:         "run-exact",
				Status:        "running",
				HistoryLength: 44,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/rooms/diagnosis-session-exact", nil)
	testConfigHandler(t, factory, WithDiagnosisRoomWorkflowVisibilityLookup(visibility)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	rawBody := rec.Body.String()
	if repo.lastChatSessionKey != "diagnosis-session-exact" {
		t.Fatalf("session key = %q, want diagnosis-session-exact", repo.lastChatSessionKey)
	}
	if repo.lastFindTaskID != 703 {
		t.Fatalf("find task id = %d, want 703", repo.lastFindTaskID)
	}
	if visibility.called != 1 || len(visibility.requests) != 1 {
		t.Fatalf("visibility requests = %+v called=%d", visibility.requests, visibility.called)
	}

	var body api.DiagnosisRoomSummary
	if err := json.NewDecoder(strings.NewReader(rawBody)).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.SessionID != "diagnosis-session-exact" ||
		body.ChatSessionID != 703 ||
		body.DiagnosisTaskID != 703 ||
		body.EvidenceSnapshotID != 907 ||
		body.WorkflowID != "diagnosis-room-diagnosis-session-exact" ||
		body.RunID != "run-exact" ||
		body.TurnCount != 2 {
		t.Fatalf("unexpected diagnosis room summary: %+v", body)
	}
	if body.LatestConclusion == nil ||
		body.LatestConclusion.ConclusionVersion == nil ||
		*body.LatestConclusion.ConclusionVersion != "diagnosis-session-exact:2" ||
		body.LatestConclusion.Content != "Checkout latency is ready for operator confirmation." {
		t.Fatalf("latest_conclusion = %+v", body.LatestConclusion)
	}
	if len(body.ParticipantDirectoryUsers) != 1 ||
		body.ParticipantDirectoryUsers[0].Subject != "operator:exact" ||
		body.ParticipantDirectoryUsers[0].DisplayName != "Exact Operator" {
		t.Fatalf("participant_directory_users = %+v, want exact room owner projection", body.ParticipantDirectoryUsers)
	}
	if len(body.NotificationTimeline) != 1 ||
		body.NotificationTimeline[0].ProviderMessageID == nil ||
		*body.NotificationTimeline[0].ProviderMessageID != "wecom-msg-exact-final-ready" ||
		body.NotificationTimeline[0].EventKind != diagnosisConclusionEventFinalReadyNotification ||
		body.NotificationTimeline[0].ContentKind == nil ||
		*body.NotificationTimeline[0].ContentKind != "final_conclusion" ||
		body.NotificationTimeline[0].ContentSha256 == nil ||
		*body.NotificationTimeline[0].ContentSha256 != testSHA256("Checkout latency is ready for operator confirmation.") ||
		body.NotificationTimeline[0].RecommendedActionCount == nil ||
		*body.NotificationTimeline[0].RecommendedActionCount != 1 ||
		body.NotificationTimeline[0].EvidenceRequestCount == nil ||
		*body.NotificationTimeline[0].EvidenceRequestCount != 1 {
		t.Fatalf("notification_timeline = %+v", body.NotificationTimeline)
	}
	if body.WorkflowVisibility == nil ||
		body.WorkflowVisibility.HistoryLength == nil ||
		*body.WorkflowVisibility.HistoryLength != 44 {
		t.Fatalf("workflow_visibility = %+v", body.WorkflowVisibility)
	}
	for _, forbidden := range []string{"provider_raw", "idempotency_key", `"assistant_message":`, "memo", "search_attributes"} {
		if strings.Contains(rawBody, forbidden) {
			t.Fatalf("response leaked internal notification field %q: %s", forbidden, rawBody)
		}
	}
}

func TestGetDiagnosisRoomAllowsOwnerWithoutScopedAssignment(t *testing.T) {
	startedAt := time.Date(2026, 6, 29, 5, 30, 0, 0, time.UTC)
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer owner-token": {
			{Principal: ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}},
		},
	})
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: false, CheckedAt: startedAt},
	}
	repo := &fakeDiagnosisRepo{
		chatSessions: []domain.ChatSessionWithTask{{
			Session: domain.ChatSession{
				ID:              901,
				DiagnosisTaskID: 902,
				SessionKey:      "diagnosis-session-owned",
				OwnerSubject:    "owner-1",
				Status:          domain.ChatSessionStatusOpen,
				TurnCount:       0,
				StartedAt:       startedAt,
				LastActivityAt:  startedAt,
				CreatedAt:       startedAt,
				UpdatedAt:       startedAt,
			},
			Task: domain.DiagnosisTask{
				ID:                 902,
				EvidenceSnapshotID: 903,
				WorkflowID:         "diagnosis-room-diagnosis-session-owned",
				RunID:              "run-owned",
				Status:             domain.DiagnosisStatusRunning,
			},
		}},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/rooms/diagnosis-session-owned", nil)
	req.Header.Set("Authorization", "Bearer owner-token")
	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}, diagnosisRepo: repo},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("O", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithRBACAuthorizer(authorizer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer calls = %d, want owner authorization before assignment lookup", authorizer.called)
	}
	var body api.DiagnosisRoomSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.SessionID != "diagnosis-session-owned" || body.ChatSessionID != 901 {
		t.Fatalf("body = %+v, want owned room summary", body)
	}
}

func TestGetDiagnosisRoom_ReturnsNotificationTimelineInOccurrenceOrder(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 4, 0, 0, 0, time.UTC)
	notificationPayload := func(kind, providerMessageID, providerStatus string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{
			"kind":%q,
			"session_id":"diagnosis-session-timeline",
			"chat_session_id":804,
			"diagnosis_task_id":804,
			"assistant_message_id":"msg-timeline/assistant",
			"assistant_turn_id":805,
			"assistant_sequence":2,
			"turn_count":2,
			"notification_channel_profile_id":2,
			"provider_message_id":%q,
			"provider_status":%q,
			"provider_raw":{"errcode":0}
		}`, kind, providerMessageID, providerStatus))
	}
	repo := &fakeDiagnosisRepo{
		eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
			804: {
				diagnosisConclusionEventAssistantTurnNotification: {{
					ID:         204,
					TaskID:     804,
					Kind:       diagnosisConclusionEventAssistantTurnNotification,
					Payload:    notificationPayload(diagnosisConclusionEventAssistantTurnNotification, "wecom-assistant", "delivered"),
					OccurredAt: startedAt.Add(2 * time.Minute),
					RecordedAt: startedAt.Add(2*time.Minute + time.Second),
				}},
				diagnosisConclusionEventFinalReadyNotification: {
					{
						ID:         205,
						TaskID:     804,
						Kind:       diagnosisConclusionEventFinalReadyNotification,
						Payload:    notificationPayload(diagnosisConclusionEventFinalReadyNotification, "wecom-final-new", "delivered"),
						OccurredAt: startedAt.Add(4 * time.Minute),
						RecordedAt: startedAt.Add(4*time.Minute + time.Second),
					},
					{
						ID:         202,
						TaskID:     804,
						Kind:       diagnosisConclusionEventFinalReadyNotification,
						Payload:    notificationPayload(diagnosisConclusionEventFinalReadyNotification, "wecom-final-old", "failed"),
						OccurredAt: startedAt.Add(time.Minute),
						RecordedAt: startedAt.Add(time.Minute + time.Second),
					},
				},
				diagnosisConclusionEventCloseNotification: {{
					ID:         203,
					TaskID:     804,
					Kind:       diagnosisConclusionEventCloseNotification,
					Payload:    notificationPayload(diagnosisConclusionEventCloseNotification, "wecom-close", "delivered"),
					OccurredAt: startedAt.Add(3 * time.Minute),
					RecordedAt: startedAt.Add(3*time.Minute + time.Second),
				}},
			},
		},
		chatSessions: []domain.ChatSessionWithTask{{
			Session: domain.ChatSession{
				ID:              804,
				DiagnosisTaskID: 804,
				SessionKey:      "diagnosis-session-timeline",
				Status:          domain.ChatSessionStatusOpen,
				TurnCount:       2,
				StartedAt:       startedAt,
				LastActivityAt:  startedAt.Add(5 * time.Minute),
				CreatedAt:       startedAt,
				UpdatedAt:       startedAt.Add(5 * time.Minute),
			},
			Task: domain.DiagnosisTask{
				ID:                 804,
				EvidenceSnapshotID: 908,
				WorkflowID:         "diagnosis-room-diagnosis-session-timeline",
				RunID:              "run-timeline",
				Status:             domain.DiagnosisStatusRunning,
			},
		}},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/rooms/diagnosis-session-timeline", nil)
	testConfigHandler(t, &fakeUOWFactory{diagnosisRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	rawBody := rec.Body.String()
	var body api.DiagnosisRoomSummary
	if err := json.NewDecoder(strings.NewReader(rawBody)).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.NotificationTimeline) != 4 {
		t.Fatalf("notification_timeline len = %d, want 4: %+v", len(body.NotificationTimeline), body.NotificationTimeline)
	}
	gotMessages := make([]string, 0, len(body.NotificationTimeline))
	gotStatuses := make([]string, 0, len(body.NotificationTimeline))
	for index, item := range body.NotificationTimeline {
		if item.ProviderMessageID == nil {
			t.Fatalf("notification_timeline[%d] provider message id = nil: %+v", index, item)
		}
		if index > 0 && item.OccurredAt.Before(body.NotificationTimeline[index-1].OccurredAt) {
			t.Fatalf("notification_timeline not ordered by occurred_at: %+v", body.NotificationTimeline)
		}
		gotMessages = append(gotMessages, *item.ProviderMessageID)
		gotStatuses = append(gotStatuses, item.ProviderStatus)
	}
	if !slices.Equal(gotMessages, []string{
		"wecom-final-old",
		"wecom-assistant",
		"wecom-close",
		"wecom-final-new",
	}) {
		t.Fatalf("provider message order = %+v", gotMessages)
	}
	if !slices.Equal(gotStatuses, []string{"failed", "delivered", "delivered", "delivered"}) {
		t.Fatalf("provider statuses = %+v", gotStatuses)
	}
	for _, forbidden := range []string{"provider_raw", "idempotency_key"} {
		if strings.Contains(rawBody, forbidden) {
			t.Fatalf("response leaked internal notification field %q: %s", forbidden, rawBody)
		}
	}
}

func TestListDiagnosisRooms_ReturnsLatestProgressForOpenRoom(t *testing.T) {
	startedAt := time.Date(2026, 6, 20, 2, 41, 24, 0, time.UTC)
	progressAt := startedAt.Add(2 * time.Minute)
	progressPayload := json.RawMessage(`{
		"kind":"diagnosis_room.turn_persisted",
		"session_id":"diagnosis-session-auto-p3-s320",
		"chat_session_id":528,
		"diagnosis_task_id":528,
		"assistant_message_id":"diagnosis-auto-initial-p3-s320/assistant",
		"assistant_turn_id":910,
		"assistant_sequence":2,
		"turn_count":1,
		"confidence":"low",
		"requires_human_review":true,
		"evidence_requests":[{"tool":"active_alerts","reason":"Check related active alerts.","limit":5}],
		"consultation_insight":{
			"conclusion_status":"needs_evidence",
			"confidence_rationale":"Initial evidence needs bounded active-alert confirmation.",
			"missing_evidence_requests":[{"label":"Owner action","detail":"Attach the current remediation status.","priority":"high"}],
			"evidence_collection_suggestions":[{"label":"Recent active alerts","detail":"Collect active alerts for this service.","priority":"medium"}]
		}
	}`)
	factory := &fakeUOWFactory{
		diagnosisRepo: &fakeDiagnosisRepo{
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				528: {
					diagnosisConclusionEventTurnPersisted: {{
						ID:         900,
						TaskID:     528,
						Kind:       diagnosisConclusionEventTurnPersisted,
						Payload:    progressPayload,
						OccurredAt: progressAt,
						RecordedAt: progressAt.Add(time.Second),
					}},
				},
			},
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              528,
						DiagnosisTaskID: 528,
						SessionKey:      "diagnosis-session-auto-p3-s320",
						Status:          domain.ChatSessionStatusOpen,
						TurnCount:       1,
						StartedAt:       startedAt,
						LastActivityAt:  progressAt,
						CreatedAt:       startedAt,
						UpdatedAt:       progressAt,
					},
					Task: domain.DiagnosisTask{
						ID:                 528,
						EvidenceSnapshotID: 320,
						WorkflowID:         "diagnosis-room-diagnosis-session-auto-p3-s320",
						RunID:              "run-320",
						Status:             domain.DiagnosisStatusRunning,
					},
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/rooms?limit=1", nil)
	testConfigHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.DiagnosisRoomListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.LatestConclusion != nil {
		t.Fatalf("latest_conclusion = %+v, want nil", got.LatestConclusion)
	}
	if got.LatestProgress == nil {
		t.Fatalf("latest_progress = nil, want populated")
	}
	if got.LatestProgress.EventKind != diagnosisConclusionEventTurnPersisted ||
		got.LatestProgress.Status != string(api.DiagnosisRoomProgressSummaryStatusInProgress) ||
		got.LatestProgress.ConclusionStatus == nil ||
		*got.LatestProgress.ConclusionStatus != "needs_evidence" ||
		got.LatestProgress.Confidence != api.ReportConfidenceLow ||
		!got.LatestProgress.RequiresHumanReview ||
		got.LatestProgress.EvidenceRequestCount != 1 ||
		len(got.LatestProgress.MissingEvidenceRequests) != 1 ||
		got.LatestProgress.MissingEvidenceRequests[0].Label != "Owner action" ||
		len(got.LatestProgress.EvidenceCollectionSuggestions) != 1 ||
		!got.LatestProgress.OccurredAt.Equal(progressAt) {
		t.Fatalf("unexpected latest progress: %+v", got.LatestProgress)
	}
}

func TestGetDashboard_ReturnsRecentCounters(t *testing.T) {
	createdAt := time.Date(2026, 5, 28, 5, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		alertRepo: &fakeAlertRepo{
			events: []domain.AlertEvent{
				{
					ID:                   1,
					SourceFingerprint:    "source:checkout",
					CanonicalFingerprint: "canonical:checkout",
					Status:               domain.AlertStatusFiring,
				},
				{ID: 2, Status: domain.AlertStatusResolved},
				{
					ID:                   3,
					SourceFingerprint:    "source:payment",
					CanonicalFingerprint: "canonical:payment",
					Status:               domain.AlertStatusFiring,
				},
			},
		},
		evidenceRepo: &fakeEvidenceRepo{
			snapshots: []domain.EvidenceSnapshot{
				{
					ID:           101,
					AlertGroupID: 201,
					Digest:       "sha256:checkout",
					Payload: json.RawMessage(
						`{"events":[{"source_fingerprint":"source:checkout","canonical_fingerprint":"canonical:checkout"}]}`,
					),
					Status:    domain.SnapshotStatusComplete,
					CreatedAt: createdAt,
				},
				{
					ID:           102,
					AlertGroupID: 202,
					Digest:       "sha256:payment",
					Payload: json.RawMessage(
						`{"events":[{"source_fingerprint":"source:payment","canonical_fingerprint":"canonical:payment"}]}`,
					),
					Status:    domain.SnapshotStatusComplete,
					CreatedAt: createdAt,
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              301,
						DiagnosisTaskID: 401,
						SessionKey:      "diagnosis-session-102",
						Status:          domain.ChatSessionStatusOpen,
						StartedAt:       createdAt,
						LastActivityAt:  createdAt,
						CreatedAt:       createdAt,
						UpdatedAt:       createdAt,
					},
					Task: domain.DiagnosisTask{
						ID:                 401,
						EvidenceSnapshotID: 102,
						WorkflowID:         "diagnosis-room-102",
						RunID:              "run-102",
						Status:             domain.DiagnosisStatusRunning,
						CreatedAt:          createdAt,
						UpdatedAt:          createdAt,
					},
				},
			},
		},
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{ID: 11, Severity: domain.ReportSeverityWarning, CreatedAt: createdAt},
				{ID: 12, Severity: domain.ReportSeverityCritical, CreatedAt: createdAt},
				{ID: 13, Severity: domain.ReportSeverityInfo, CreatedAt: createdAt},
				{ID: 14, Severity: domain.ReportSeverityWarning, CreatedAt: createdAt},
			},
			deliveriesByReport: map[domain.FinalReportID][]domain.ReportNotificationDelivery{
				11: {{Status: domain.ReportNotificationDeliveryStatusDelivered}},
				12: {{Status: domain.ReportNotificationDeliveryStatusFailed}},
				13: {{Status: domain.ReportNotificationDeliveryStatusPending}},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/dashboard", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.alertRepo.lastLimit != defaultListLimit || factory.reportRepo.lastListLimit != defaultListLimit {
		t.Fatalf("limits alert=%d report=%d, want %d", factory.alertRepo.lastLimit, factory.reportRepo.lastListLimit, defaultListLimit)
	}
	if factory.evidenceRepo.lastLimit != maxListLimit || factory.diagnosisRepo.lastChatSessionLimit != maxListLimit {
		t.Fatalf(
			"diagnosis limits evidence=%d rooms=%d, want %d",
			factory.evidenceRepo.lastLimit,
			factory.diagnosisRepo.lastChatSessionLimit,
			maxListLimit,
		)
	}
	if factory.reportRepo.lastDeliveryLimit != 1 {
		t.Fatalf("delivery limit = %d, want 1", factory.reportRepo.lastDeliveryLimit)
	}

	var body api.DashboardSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.GeneratedAt.IsZero() {
		t.Fatalf("generated_at should be set")
	}
	if body.Alerts.TotalRecent != 3 || body.Alerts.Firing != 2 || body.Alerts.Resolved != 1 {
		t.Fatalf("alert stats = %+v", body.Alerts)
	}
	if body.Diagnosis.LinkedSnapshots != 2 ||
		body.Diagnosis.RoomsStarted != 1 ||
		body.Diagnosis.SnapshotsNeedingRoom != 1 ||
		body.Diagnosis.AffectedAlertsNeedingRoom != 1 {
		t.Fatalf("diagnosis stats = %+v", body.Diagnosis)
	}
	if body.Reports.TotalRecent != 4 ||
		body.Reports.Delivered != 1 ||
		body.Reports.Failed != 1 ||
		body.Reports.Pending != 1 ||
		body.Reports.MissingDelivery != 1 {
		t.Fatalf("report delivery stats = %+v", body.Reports)
	}
	if body.Reports.Severity.Info != 1 || body.Reports.Severity.Warning != 2 || body.Reports.Severity.Critical != 1 {
		t.Fatalf("severity stats = %+v", body.Reports.Severity)
	}
	rate, err := body.Reports.SuccessRate.Get()
	if err != nil {
		t.Fatalf("success_rate should be set: %v", err)
	}
	if rate != 0.25 {
		t.Fatalf("success_rate = %v, want 0.25", rate)
	}
}

func TestGetDashboardFiltersDiagnosisStatsWithoutRoomRead(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 7, 30, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		alertRepo: &fakeAlertRepo{
			events: []domain.AlertEvent{
				{
					ID:                   11,
					SourceFingerprint:    "source:checkout",
					CanonicalFingerprint: "canonical:checkout",
					Status:               domain.AlertStatusFiring,
				},
			},
		},
		evidenceRepo: &fakeEvidenceRepo{
			snapshots: []domain.EvidenceSnapshot{
				{
					ID:           501,
					AlertGroupID: 601,
					Digest:       "sha256:checkout",
					Payload: json.RawMessage(
						`{"events":[{"source_fingerprint":"source:checkout","canonical_fingerprint":"canonical:checkout"}]}`,
					),
					Status:    domain.SnapshotStatusComplete,
					CreatedAt: createdAt,
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              701,
						DiagnosisTaskID: 801,
						SessionKey:      "diagnosis-session-501",
						Status:          domain.ChatSessionStatusOpen,
						StartedAt:       createdAt,
						LastActivityAt:  createdAt,
						CreatedAt:       createdAt,
						UpdatedAt:       createdAt,
					},
					Task: domain.DiagnosisTask{
						ID:                 801,
						EvidenceSnapshotID: 501,
						WorkflowID:         "diagnosis-room-501",
						RunID:              "run-501",
						Status:             domain.DiagnosisStatusRunning,
						CreatedAt:          createdAt,
						UpdatedAt:          createdAt,
					},
				},
			},
		},
		reportRepo: &fakeReportRepo{},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed:   req.Permission == domain.RBACPermissionOperationsRead,
				CheckedAt: createdAt,
			}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/dashboard", nil)
	testOperationsReadHandlerWithAuthorizer(t, factory, authorizer).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.DashboardSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Diagnosis.LinkedSnapshots != 1 ||
		body.Diagnosis.RoomsStarted != 0 ||
		body.Diagnosis.SnapshotsNeedingRoom != 1 ||
		body.Diagnosis.AffectedAlertsNeedingRoom != 1 {
		t.Fatalf("diagnosis stats = %+v, want hidden room filtered from started count", body.Diagnosis)
	}
	if len(authorizer.requests) != 3 {
		t.Fatalf("authorizer requests = %+v, want operations, global room read, scoped room read", authorizer.requests)
	}
	if authorizer.requests[0].Permission != domain.RBACPermissionOperationsRead ||
		authorizer.requests[0].ScopeKind != domain.RBACScopeKindGlobal {
		t.Fatalf("operations read request = %+v", authorizer.requests[0])
	}
	if authorizer.requests[1].Permission != domain.RBACPermissionDiagnosisRoomRead ||
		authorizer.requests[1].ScopeKind != domain.RBACScopeKindGlobal {
		t.Fatalf("global room read request = %+v", authorizer.requests[1])
	}
	if authorizer.requests[2].Permission != domain.RBACPermissionDiagnosisRoomRead ||
		authorizer.requests[2].ScopeKind != domain.RBACScopeKindDiagnosisRoom ||
		authorizer.requests[2].ScopeKey != "diagnosis-session-501" {
		t.Fatalf("scoped room read request = %+v", authorizer.requests[2])
	}
}

func TestListAlertSourceProfilesReturnsProfiles(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			alertSourceProfiles: []domain.AlertSourceProfile{
				{
					ID:        1,
					Name:      "Primary Prometheus",
					Kind:      domain.AlertSourceKindPrometheus,
					BaseURL:   "https://prometheus.example.test",
					AuthMode:  domain.AlertSourceAuthModeBearer,
					SecretRef: "secret/openclarion/prometheus-bearer",
					Enabled:   false,
					Labels:    map[string]string{"env": "staging"},
					CreatedAt: createdAt,
					UpdatedAt: createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/alert-sources?limit=1", nil)
	testConfigHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.AlertSourceProfileListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.Name != "Primary Prometheus" || got.Kind != api.Prometheus || got.SecretRef != "secret/openclarion/prometheus-bearer" {
		t.Fatalf("unexpected profile: %+v", got)
	}
}

func TestCreateAlertSourceProfileSavesSanitizedProfile(t *testing.T) {
	repo := &fakeConfigRepo{
		saveResult: domain.AlertSourceProfile{
			ID:        1,
			Name:      "Primary Prometheus",
			Kind:      domain.AlertSourceKindPrometheus,
			BaseURL:   "https://prometheus.example.test",
			AuthMode:  domain.AlertSourceAuthModeBearer,
			SecretRef: "secret/openclarion/prometheus-bearer",
			Enabled:   true,
			Labels:    map[string]string{"env": "staging"},
			CreatedAt: time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
		},
	}
	body := `{
		"name":" Primary Prometheus ",
		"kind":"prometheus",
		"base_url":"https://prometheus.example.test",
		"auth_mode":"bearer",
		"secret_ref":"secret/openclarion/prometheus-bearer",
		"enabled":true,
		"labels":{" env ":" staging "}
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/alert-sources", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.saved.Name != "Primary Prometheus" || repo.saved.Labels["env"] != "staging" {
		t.Fatalf("saved profile was not normalized: %+v", repo.saved)
	}
	var resp api.AlertSourceProfile
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.ID != 1 || resp.SecretRef != "secret/openclarion/prometheus-bearer" || !resp.Enabled {
		t.Fatalf("response = %+v", resp)
	}
}

func TestReplaceAlertSourceProfileDisablesSource(t *testing.T) {
	repo := &fakeConfigRepo{
		updateResult: domain.AlertSourceProfile{
			ID:        7,
			Name:      "Primary Alertmanager",
			Kind:      domain.AlertSourceKindAlertmanager,
			BaseURL:   "https://alertmanager.example.test",
			AuthMode:  domain.AlertSourceAuthModeNone,
			Enabled:   false,
			Labels:    map[string]string{},
			CreatedAt: time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 6, 5, 3, 5, 0, 0, time.UTC),
		},
	}
	body := `{
		"name":"Primary Alertmanager",
		"kind":"alertmanager",
		"base_url":"https://alertmanager.example.test",
		"auth_mode":"none",
		"enabled":false
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPut, "/api/v1/config/alert-sources/7", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updated.ID != 7 || repo.updated.Enabled {
		t.Fatalf("updated = %+v, want id 7 disabled", repo.updated)
	}
}

func TestAlertSourceProfileConnectionTestReturnsSanitizedResult(t *testing.T) {
	checkedAt := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{
				ID:        1,
				Name:      "Primary Prometheus",
				Kind:      domain.AlertSourceKindPrometheus,
				BaseURL:   "https://prometheus.example.test",
				AuthMode:  domain.AlertSourceAuthModeBearer,
				SecretRef: "secret/openclarion/prometheus-bearer",
				Enabled:   true,
				Labels:    map[string]string{"env": "prod"},
				CreatedAt: checkedAt,
				UpdatedAt: checkedAt,
			},
		},
	}
	tester := &fakeAlertSourceConnectionTester{
		result: alertsourcecheck.Result{
			SourceID:       1,
			Kind:           domain.AlertSourceKindPrometheus,
			AuthMode:       domain.AlertSourceAuthModeBearer,
			Status:         alertsourcecheck.StatusBlocked,
			ReasonCode:     alertsourcecheck.ReasonCredentialsUnavailable,
			Message:        "Secret-backed connection tests require a server-side secret resolver.",
			CheckedAt:      checkedAt,
			ObservedAlerts: 0,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/alert-sources/1/test", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}, WithAlertSourceConnectionTester(tester)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if tester.called != 1 || tester.profile.ID != 1 || tester.profile.BaseURL == "" {
		t.Fatalf("tester called=%d profile=%+v", tester.called, tester.profile)
	}
	if body := rec.Body.String(); strings.Contains(body, "https://prometheus.example.test") || strings.Contains(body, "secret/openclarion") {
		t.Fatalf("response leaked endpoint or secret reference: %s", body)
	}
	var resp api.AlertSourceConnectionTestResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Status != api.AlertSourceConnectionTestStatusBlocked ||
		resp.ReasonCode != api.AlertSourceConnectionTestReasonCodeCredentialsUnavailable {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAlertSourceConnectionTestResponsePreservesCapabilityReason(t *testing.T) {
	result := alertsourcecheck.Result{
		ReasonCode: alertsourcecheck.ReasonCapabilityUnavailable,
	}

	response := alertSourceConnectionTestResponse(result)
	if response.ReasonCode != api.AlertSourceConnectionTestReasonCodeCapabilityUnavailable {
		t.Fatalf("reason code = %q, want capability_unavailable", response.ReasonCode)
	}
}

func TestAlertSourceProfileConnectionTestRejectsUnconfiguredAndMissingProfiles(t *testing.T) {
	tests := []struct {
		name       string
		repo       *fakeConfigRepo
		opts       []ServerOption
		wantStatus int
	}{
		{
			name:       "unconfigured",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusServiceUnavailable,
		},
		{
			name:       "not_found",
			repo:       &fakeConfigRepo{},
			opts:       []ServerOption{WithAlertSourceConnectionTester(&fakeAlertSourceConnectionTester{})},
			wantStatus: stdhttp.StatusNotFound,
		},
		{
			name:       "invalid_id",
			repo:       &fakeConfigRepo{},
			opts:       []ServerOption{WithAlertSourceConnectionTester(&fakeAlertSourceConnectionTester{})},
			wantStatus: stdhttp.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/config/alert-sources/99/test"
			if tc.name == "invalid_id" {
				path = "/api/v1/config/alert-sources/0/test"
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, path, nil)
			testConfigHandler(t, &fakeUOWFactory{configRepo: tc.repo}, tc.opts...).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestAlertSourceProfileWriteRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		repoErr    error
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/alert-sources",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://prometheus.example.test","auth_mode":"none","extra":true}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "userinfo_url",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/alert-sources",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://user@prometheus.example.test","auth_mode":"none"}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "bearer_missing_secret_ref",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/alert-sources",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://prometheus.example.test","auth_mode":"bearer"}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "duplicate_name",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/alert-sources",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://prometheus.example.test","auth_mode":"none"}`,
			wantStatus: stdhttp.StatusConflict,
			repoErr:    domain.ErrAlreadyExists,
		},
		{
			name:       "replace_not_found",
			method:     stdhttp.MethodPut,
			path:       "/api/v1/config/alert-sources/99",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://prometheus.example.test","auth_mode":"none"}`,
			wantStatus: stdhttp.StatusNotFound,
			repoErr:    domain.ErrNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeConfigRepo{saveErr: tc.repoErr, updateErr: tc.repoErr}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestListGroupingPoliciesReturnsPolicies(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			groupingPolicies: []domain.GroupingPolicy{
				{
					ID:            1,
					Name:          "Default alert grouping",
					DimensionKeys: []string{"alertname", "service"},
					SeverityKey:   "severity",
					SourceFilter:  []string{"prometheus"},
					Enabled:       true,
					CreatedAt:     createdAt,
					UpdatedAt:     createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/grouping-policies?limit=1", nil)
	testConfigHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.GroupingPolicyListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Default alert grouping" {
		t.Fatalf("body = %+v", body)
	}
}

func TestCreateGroupingPolicySavesNormalizedPolicy(t *testing.T) {
	repo := &fakeConfigRepo{
		saveGroupingPolicyResult: domain.GroupingPolicy{
			ID:            1,
			Name:          "Default alert grouping",
			DimensionKeys: []string{"alertname", "service"},
			SeverityKey:   "severity",
			SourceFilter:  []string{"prometheus"},
			Enabled:       true,
			CreatedAt:     time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC),
			UpdatedAt:     time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC),
		},
	}
	body := `{
		"name":" Default alert grouping ",
		"dimension_keys":["service","alertname","service"],
		"severity_key":" severity ",
		"source_filter":["prometheus","prometheus"],
		"enabled":true
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/grouping-policies", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedGroupingPolicy.Name != "Default alert grouping" ||
		len(repo.savedGroupingPolicy.DimensionKeys) != 2 ||
		repo.savedGroupingPolicy.DimensionKeys[0] != "alertname" ||
		repo.savedGroupingPolicy.SeverityKey != "severity" {
		t.Fatalf("saved policy was not normalized: %+v", repo.savedGroupingPolicy)
	}
	var resp api.GroupingPolicy
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.ID != 1 || !resp.Enabled || len(resp.SourceFilter) != 1 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestReplaceGroupingPolicyDisablesPolicy(t *testing.T) {
	repo := &fakeConfigRepo{
		updateGroupingPolicyResult: domain.GroupingPolicy{
			ID:            7,
			Name:          "Service grouping",
			DimensionKeys: []string{"service"},
			SeverityKey:   "severity",
			SourceFilter:  []string{},
			Enabled:       false,
			CreatedAt:     time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC),
			UpdatedAt:     time.Date(2026, 6, 5, 4, 5, 0, 0, time.UTC),
		},
	}
	body := `{
		"name":"Service grouping",
		"dimension_keys":["service"],
		"severity_key":"severity",
		"enabled":false
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPut, "/api/v1/config/grouping-policies/7", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updatedGroupingPolicy.ID != 7 || repo.updatedGroupingPolicy.Enabled {
		t.Fatalf("updated = %+v, want id 7 disabled", repo.updatedGroupingPolicy)
	}
}

func TestPreviewGroupingPolicyReturnsBoundedGroups(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		groupingPolicies: []domain.GroupingPolicy{
			{
				ID:            1,
				Name:          "Default alert grouping",
				DimensionKeys: []string{"alertname"},
				SeverityKey:   "severity",
				SourceFilter:  []string{"prometheus"},
				Enabled:       true,
				CreatedAt:     createdAt,
				UpdatedAt:     createdAt,
			},
		},
	}
	alerts := &fakeAlertRepo{
		events: []domain.AlertEvent{
			{
				ID:       101,
				Source:   "prometheus",
				Labels:   map[string]string{"alertname": "HighCPU", "severity": "warning"},
				StartsAt: createdAt,
			},
			{
				ID:       102,
				Source:   "prometheus",
				Labels:   map[string]string{"alertname": "HighCPU", "severity": "critical"},
				StartsAt: createdAt.Add(time.Minute),
			},
			{
				ID:       103,
				Source:   "alertmanager",
				Labels:   map[string]string{"alertname": "DiskFull", "severity": "info"},
				StartsAt: createdAt.Add(2 * time.Minute),
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/grouping-policies/1/preview?limit=3", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo, alertRepo: alerts}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if alerts.lastLimit != 3 {
		t.Fatalf("alert list limit = %d, want 3", alerts.lastLimit)
	}
	var resp api.GroupingPolicyPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.PolicyID != 1 || resp.EventsScanned != 3 || resp.EventsMatched != 2 {
		t.Fatalf("response counts = %+v", resp)
	}
	if len(resp.Groups) != 1 || resp.Groups[0].Severity != api.GroupingPolicyPreviewSeverityCritical {
		t.Fatalf("groups = %+v", resp.Groups)
	}
	if resp.Groups[0].Dimensions["alertname"] != "HighCPU" || len(resp.Groups[0].EventIds) != 2 {
		t.Fatalf("group detail = %+v", resp.Groups[0])
	}
}

func TestGroupingPolicyWriteAndPreviewRejectInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		repoErr    error
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies",
			body:       `{"name":"Policy","dimension_keys":["alertname"],"severity_key":"severity","extra":true}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "dimension_whitespace",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies",
			body:       `{"name":"Policy","dimension_keys":["alert name"],"severity_key":"severity"}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "duplicate_name",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies",
			body:       `{"name":"Policy","dimension_keys":["alertname"],"severity_key":"severity"}`,
			wantStatus: stdhttp.StatusConflict,
			repoErr:    domain.ErrAlreadyExists,
		},
		{
			name:       "replace_not_found",
			method:     stdhttp.MethodPut,
			path:       "/api/v1/config/grouping-policies/99",
			body:       `{"name":"Policy","dimension_keys":["alertname"],"severity_key":"severity"}`,
			wantStatus: stdhttp.StatusNotFound,
			repoErr:    domain.ErrNotFound,
		},
		{
			name:       "preview_invalid_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies/0/preview",
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "preview_not_found",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies/99/preview",
			wantStatus: stdhttp.StatusNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeConfigRepo{
				saveErr:   tc.repoErr,
				updateErr: tc.repoErr,
			}
			rec := httptest.NewRecorder()
			var body *strings.Reader
			if tc.body == "" {
				body = strings.NewReader("")
			} else {
				body = strings.NewReader(tc.body)
			}
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, body)
			testConfigHandler(t, &fakeUOWFactory{configRepo: repo, alertRepo: &fakeAlertRepo{}}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGetReport_ReturnsDetailWithLinkedSubReports(t *testing.T) {
	finalCreatedAt := time.Date(2026, 5, 27, 10, 5, 0, 0, time.UTC)
	subCreatedAt := finalCreatedAt.Add(-time.Minute)
	deliveredAt := finalCreatedAt.Add(10 * time.Second)
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{
					ID:                 11,
					CorrelationKey:     "window:checkout",
					Title:              "Checkout latency incident",
					ExecutiveSummary:   "Checkout latency increased after a deployment.",
					Severity:           domain.ReportSeverityWarning,
					Confidence:         domain.ReportConfidenceHigh,
					SubReports:         json.RawMessage(`[{"title":"Checkout API latency","severity":"warning","summary":"p95 latency exceeded the warning threshold."}]`),
					RecommendedActions: json.RawMessage(`[{"label":"Inspect deployment","detail":"Compare deployment timestamps.","priority":"high"}]`),
					NotificationText:   "Checkout latency incident requires review.",
					Content:            json.RawMessage(`{"title":"Checkout latency incident","executive_summary":"Checkout latency increased after a deployment."}`),
					Model:              "gpt-4.1-mini",
					OutputMode:         "json_schema",
					CreatedByWorkflow:  "FinalReportWorkflow",
					CreatedAt:          finalCreatedAt,
				},
			},
			linkedSubReports: map[domain.FinalReportID][]domain.SubReport{
				11: {
					{
						ID:                 21,
						EvidenceSnapshotID: 7,
						Scenario:           "single_alert",
						Title:              "Checkout API latency",
						Summary:            "p95 latency exceeded the warning threshold.",
						Severity:           domain.ReportSeverityWarning,
						Confidence:         domain.ReportConfidenceHigh,
						Findings:           json.RawMessage(`[{"label":"High p95 latency","detail":"p95 latency stayed above threshold.","evidence_id":"alert:checkout-latency"}]`),
						RecommendedActions: json.RawMessage(`[{"label":"Inspect deployment","detail":"Compare deployment timestamps.","priority":"high"}]`),
						EvidenceRefs:       []string{"alert:checkout-latency"},
						Content:            json.RawMessage(`{"title":"Checkout API latency","summary":"p95 latency exceeded the warning threshold."}`),
						Model:              "gpt-4.1-mini",
						OutputMode:         "json_schema",
						CreatedByWorkflow:  "ReportFanOutWorkflow",
						CreatedAt:          subCreatedAt,
					},
				},
			},
			deliveriesByReport: map[domain.FinalReportID][]domain.ReportNotificationDelivery{
				11: {
					{
						ID:                31,
						FinalReportID:     11,
						IdempotencyKey:    "final_report:11/notification/handoff",
						ProviderMessageID: "msg-31",
						ProviderStatus:    "accepted",
						Status:            domain.ReportNotificationDeliveryStatusDelivered,
						Raw:               json.RawMessage(`{"provider":"webhook","secret":"redacted"}`),
						DeliveredAt:       &deliveredAt,
						CreatedAt:         finalCreatedAt,
						UpdatedAt:         deliveredAt,
					},
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			tasksBySnapshot: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				7: {
					{
						ID:                 31,
						EvidenceSnapshotID: 7,
						WorkflowID:         "diagnosis-room-31",
						RunID:              "run-31",
						Status:             domain.DiagnosisStatusSucceeded,
						CreatedAt:          finalCreatedAt.Add(2 * time.Minute),
					},
				},
			},
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              51,
						DiagnosisTaskID: 31,
						SessionKey:      "diagnosis-session-31",
						OwnerSubject:    "owner-1",
						Status:          domain.ChatSessionStatusOpen,
						TurnCount:       2,
						StartedAt:       finalCreatedAt.Add(time.Minute),
						LastActivityAt:  finalCreatedAt.Add(3*time.Minute + time.Second),
						CreatedAt:       finalCreatedAt.Add(time.Minute),
						UpdatedAt:       finalCreatedAt.Add(3*time.Minute + time.Second),
					},
					Task: domain.DiagnosisTask{
						ID:                 31,
						EvidenceSnapshotID: 7,
						WorkflowID:         "diagnosis-room-31",
						RunID:              "run-31",
						Status:             domain.DiagnosisStatusSucceeded,
						CreatedAt:          finalCreatedAt.Add(2 * time.Minute),
					},
				},
			},
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				31: {
					diagnosisConclusionEventTurnPersisted: {
						{
							ID:     44,
							TaskID: 31,
							Kind:   diagnosisConclusionEventTurnPersisted,
							Payload: json.RawMessage(`{
								"kind":"diagnosis_room.turn_persisted",
								"session_id":"diagnosis-session-31",
								"chat_session_id":51,
								"diagnosis_task_id":31,
								"user_message_id":"msg-2/user",
								"assistant_message_id":"msg-2/assistant",
								"user_turn_id":62,
								"assistant_turn_id":63,
								"user_sequence":3,
								"assistant_sequence":4,
								"turn_count":2,
								"confidence":"high",
								"requires_human_review":false,
								"context_bytes":2048,
								"historical_report_refs":["sub_report:18","final_report:9"],
								"evidence_requests":[],
								"consultation_insight":{
									"confidence_rationale":"Deployment evidence explains the latency onset.",
									"conclusion_status":"ready_for_review"
								}
							}`),
							OccurredAt: finalCreatedAt.Add(2*time.Minute + 45*time.Second),
							RecordedAt: finalCreatedAt.Add(2*time.Minute + 46*time.Second),
						},
						{
							ID:     43,
							TaskID: 31,
							Kind:   diagnosisConclusionEventTurnPersisted,
							Payload: json.RawMessage(`{
								"kind":"diagnosis_room.turn_persisted",
								"session_id":"diagnosis-session-31",
								"chat_session_id":51,
								"diagnosis_task_id":31,
								"user_message_id":"msg-1/user",
								"assistant_message_id":"msg-1/assistant",
								"user_turn_id":60,
								"assistant_turn_id":61,
								"user_sequence":1,
								"assistant_sequence":2,
								"turn_count":1,
								"confidence":"low",
								"requires_human_review":true,
								"context_bytes":1536,
								"historical_report_refs":["final_report:8"],
								"evidence_requests":[{
									"tool":"metric_range_query",
									"reason":"Need checkout deployment timing.",
									"query":"histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
									"alert_source_profile_id":7,
									"window_seconds":1800,
									"step_seconds":60,
									"limit":5
								}],
								"consultation_insight":{
									"confidence_rationale":"Latency evidence is present but deployment timing is missing.",
									"missing_evidence_requests":[{
										"label":"Deployment window",
										"detail":"Provide checkout deployment timing before raising confidence.",
										"priority":"high"
									}],
									"evidence_collection_suggestions":[{
										"label":"Latency trend",
										"detail":"Collect a bounded checkout p95 range query for the incident window.",
										"priority":"medium"
									}],
									"conclusion_status":"needs_evidence"
								}
							}`),
							OccurredAt: finalCreatedAt.Add(2 * time.Minute),
							RecordedAt: finalCreatedAt.Add(2*time.Minute + time.Second),
						},
					},
					diagnosisConclusionEventEvidenceCollected: {
						{
							ID:     45,
							TaskID: 31,
							Kind:   diagnosisConclusionEventEvidenceCollected,
							Payload: json.RawMessage(`{
									"kind":"diagnosis_room.evidence_collected",
									"session_id":"diagnosis-session-31",
									"chat_session_id":51,
									"diagnosis_task_id":31,
									"user_message_id":"msg-1/user",
									"assistant_message_id":"msg-1/assistant",
									"user_turn_id":60,
									"assistant_turn_id":61,
									"user_sequence":1,
									"assistant_sequence":2,
									"turn_count":1,
									"context_refs":["chat_session:51/turn:60","chat_session:51/turn:61"],
									"evidence_collection_results":[{
										"tool":"metric_range_query",
										"status":"collected",
										"reason_code":"ok",
										"message":"Metric range collection succeeded.",
										"request_reason":"Need checkout deployment timing.",
										"query":"histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
										"template_id":88,
										"alert_source_profile_id":7,
										"alert_source_kind":"prometheus",
										"window_seconds":1800,
										"step_seconds":60,
										"limit":5,
										"observed_metric_series":2,
										"collected_at":"2026-05-27T10:08:30Z"
									}]
								}`),
							OccurredAt: finalCreatedAt.Add(2*time.Minute + 30*time.Second),
							RecordedAt: finalCreatedAt.Add(2*time.Minute + 31*time.Second),
						},
					},
					diagnosisConclusionEventFinalReady: {
						{
							ID:     41,
							TaskID: 31,
							Kind:   diagnosisConclusionEventFinalReady,
							Payload: json.RawMessage(`{
									"kind":"diagnosis_room.final_conclusion_ready",
									"session_id":"diagnosis-session-31",
									"chat_session_id":51,
									"diagnosis_task_id":31,
									"owner_subject":"owner-1",
									"turn_count":1,
									"final_conclusion":{
										"status":"available",
										"source":"latest_assistant_turn",
										"reason":"assistant_marked_final",
										"evidence_snapshot_id":7,
										"conclusion_version":"diagnosis-room-final-ready.v1",
										"supplemental_context_refs":["chat_session:51/turn:60","chat_session:51/turn:61"],
										"assistant_turn_id":61,
										"assistant_message_id":"msg-1/assistant",
										"assistant_sequence":2,
										"assistant_occurred_at":"2026-05-27T10:08:00Z",
										"content":"Checkout latency remains correlated with the deployment.",
										"confidence":"high",
										"requires_human_review":true
									},
									"conclusion_version":"diagnosis-room-final-ready.v1"
								}`),
							OccurredAt: finalCreatedAt.Add(3 * time.Minute),
							RecordedAt: finalCreatedAt.Add(3*time.Minute + time.Second),
						},
					},
					diagnosisConclusionEventFinalReadyNotification: {
						{
							ID:     46,
							TaskID: 31,
							Kind:   diagnosisConclusionEventFinalReadyNotification,
							Payload: json.RawMessage(`{
								"kind":"diagnosis_room.final_ready_notification_sent",
								"session_id":"diagnosis-session-31",
								"chat_session_id":51,
								"diagnosis_task_id":31,
								"assistant_message_id":"msg-1/assistant",
								"assistant_turn_id":61,
								"assistant_sequence":2,
								"turn_count":2,
								"notification_channel_profile_id":2,
								"provider_message_id":"wecom-final-ready-31",
								"provider_status":"delivered",
								"confidence":"high",
								"requires_human_review":true
							}`),
							OccurredAt: finalCreatedAt.Add(3*time.Minute + 2*time.Second),
							RecordedAt: finalCreatedAt.Add(3*time.Minute + 3*time.Second),
						},
					},
					diagnosisConclusionEventSupplementalEvidence: {
						{
							ID:     42,
							TaskID: 31,
							Kind:   diagnosisConclusionEventSupplementalEvidence,
							Payload: json.RawMessage(`{
								"kind":"diagnosis_room.supplemental_evidence_provided",
								"session_id":"diagnosis-session-31",
								"chat_session_id":51,
								"diagnosis_task_id":31,
								"user_message_id":"msg-1/user",
								"assistant_message_id":"msg-1/assistant",
								"user_turn_id":60,
								"assistant_turn_id":61,
								"user_sequence":1,
								"assistant_sequence":2,
								"context_refs":["chat_session:51/turn:60","chat_session:51/turn:61"],
								"supplemental_evidence":{
									"label":"Deployment window",
									"detail":"Compare checkout deployment time with the latency onset.",
									"priority":"high",
									"evidence":"The payment deployment started two minutes before checkout p95 crossed the warning threshold."
								},
								"confidence":"high",
								"requires_human_review":true
							}`),
							OccurredAt: finalCreatedAt.Add(2*time.Minute + 30*time.Second),
							RecordedAt: finalCreatedAt.Add(2*time.Minute + 31*time.Second),
						},
					},
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports/11", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.reportRepo.lastSubReportsLimit != maxListLimit {
		t.Fatalf("linked subreport limit = %d, want %d", factory.reportRepo.lastSubReportsLimit, maxListLimit)
	}
	if factory.reportRepo.lastDeliveryLimit != reportNotificationDeliveryLimit {
		t.Fatalf("notification delivery limit = %d, want %d", factory.reportRepo.lastDeliveryLimit, reportNotificationDeliveryLimit)
	}

	var body api.FinalReportDetail
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ID != 11 || body.Content["title"] != "Checkout latency incident" {
		t.Fatalf("unexpected report detail: %+v", body)
	}
	if len(body.SubReports) != 1 || body.SubReports[0].Title != "Checkout API latency" {
		t.Fatalf("unexpected embedded sub_reports: %+v", body.SubReports)
	}
	if len(body.NotificationDeliveries) != 1 {
		t.Fatalf("len(notification_deliveries) = %d, want 1", len(body.NotificationDeliveries))
	}
	if body.FinalNotificationReadiness.Ready ||
		body.FinalNotificationReadiness.NotificationPurpose != api.Handoff ||
		body.FinalNotificationReadiness.Status != string(api.ReportFinalNotificationReadinessStatusBlocked) ||
		!strings.Contains(body.FinalNotificationReadiness.Detail, "Checkout API latency has no operator-confirmed AI conclusion yet") {
		t.Fatalf("final notification readiness = %+v, want blocked handoff readiness", body.FinalNotificationReadiness)
	}
	delivery := body.NotificationDeliveries[0]
	if delivery.ID != 31 ||
		delivery.IdempotencyKey != "final_report:11/notification/handoff" ||
		delivery.NotificationPurpose != api.Handoff ||
		delivery.Status != api.ReportNotificationDeliveryStatusDelivered ||
		delivery.ProviderMessageID == nil ||
		*delivery.ProviderMessageID != "msg-31" ||
		delivery.ProviderStatus == nil ||
		*delivery.ProviderStatus != "accepted" ||
		delivery.DeliveredAt == nil ||
		!delivery.DeliveredAt.Equal(deliveredAt) ||
		!delivery.CreatedAt.Equal(finalCreatedAt) ||
		!delivery.UpdatedAt.Equal(deliveredAt) {
		t.Fatalf("unexpected notification delivery proof: %+v", delivery)
	}
	if len(body.LinkedSubReports) != 1 {
		t.Fatalf("len(linked_sub_reports) = %d, want 1", len(body.LinkedSubReports))
	}
	linked := body.LinkedSubReports[0]
	if linked.ID != 21 || linked.EvidenceSnapshotID != 7 || len(linked.Findings) != 1 || linked.Findings[0].EvidenceID != "alert:checkout-latency" {
		t.Fatalf("unexpected linked subreport: %+v", linked)
	}
	conclusion := linked.DiagnosisConclusion
	if conclusion == nil {
		t.Fatalf("diagnosis_conclusion is nil, want latest conclusion")
	}
	if conclusion.DiagnosisTaskID != 31 ||
		conclusion.SessionID != "diagnosis-session-31" ||
		conclusion.ChatSessionID != 51 ||
		conclusion.EvidenceSnapshotID == nil ||
		*conclusion.EvidenceSnapshotID != 7 ||
		conclusion.ConclusionVersion == nil ||
		*conclusion.ConclusionVersion != "diagnosis-room-final-ready.v1" ||
		len(conclusion.SupplementalContextRefs) != 2 ||
		conclusion.SupplementalContextRefs[1] != "chat_session:51/turn:61" ||
		conclusion.Content != "Checkout latency remains correlated with the deployment." ||
		conclusion.Confidence == nil ||
		*conclusion.Confidence != api.ReportConfidenceHigh ||
		conclusion.RequiresHumanReview == nil ||
		!*conclusion.RequiresHumanReview {
		t.Fatalf("unexpected diagnosis conclusion: %+v", conclusion)
	}
	if len(conclusion.SupplementalEvidence) != 1 ||
		conclusion.SupplementalEvidence[0].Label != "Deployment window" ||
		conclusion.SupplementalEvidence[0].Priority != "high" ||
		conclusion.SupplementalEvidence[0].Evidence != "The payment deployment started two minutes before checkout p95 crossed the warning threshold." ||
		len(conclusion.SupplementalEvidence[0].ContextRefs) != 2 ||
		conclusion.SupplementalEvidence[0].ContextRefs[1] != "chat_session:51/turn:61" ||
		conclusion.SupplementalEvidence[0].UserTurnID == nil ||
		*conclusion.SupplementalEvidence[0].UserTurnID != 60 ||
		conclusion.SupplementalEvidence[0].UserSequence == nil ||
		*conclusion.SupplementalEvidence[0].UserSequence != 1 ||
		conclusion.SupplementalEvidence[0].AssistantSequence == nil ||
		*conclusion.SupplementalEvidence[0].AssistantSequence != 2 {
		t.Fatalf("unexpected supplemental evidence: %+v", conclusion.SupplementalEvidence)
	}
	if len(conclusion.ConfidenceTimeline) != 2 ||
		conclusion.ConfidenceTimeline[0].Confidence != api.ReportConfidenceLow ||
		conclusion.ConfidenceTimeline[0].ConclusionStatus == nil ||
		*conclusion.ConfidenceTimeline[0].ConclusionStatus != "needs_evidence" ||
		conclusion.ConfidenceTimeline[0].ContextBytes == nil ||
		*conclusion.ConfidenceTimeline[0].ContextBytes != 1536 ||
		!slices.Equal(conclusion.ConfidenceTimeline[0].RetrievalRefs, []string{"final_report:8"}) ||
		conclusion.ConfidenceTimeline[0].EvidenceRequestCount != 1 ||
		len(conclusion.ConfidenceTimeline[0].EvidenceRequests) != 1 ||
		conclusion.ConfidenceTimeline[0].EvidenceRequests[0].Tool != "metric_range_query" ||
		conclusion.ConfidenceTimeline[0].EvidenceRequests[0].Query == nil ||
		*conclusion.ConfidenceTimeline[0].EvidenceRequests[0].Query != "histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))" ||
		conclusion.ConfidenceTimeline[0].EvidenceRequests[0].AlertSourceProfileID == nil ||
		*conclusion.ConfidenceTimeline[0].EvidenceRequests[0].AlertSourceProfileID != 7 ||
		conclusion.ConfidenceTimeline[0].EvidenceRequests[0].WindowSeconds == nil ||
		*conclusion.ConfidenceTimeline[0].EvidenceRequests[0].WindowSeconds != 1800 ||
		conclusion.ConfidenceTimeline[0].EvidenceRequests[0].StepSeconds == nil ||
		*conclusion.ConfidenceTimeline[0].EvidenceRequests[0].StepSeconds != 60 ||
		len(conclusion.ConfidenceTimeline[0].MissingEvidenceRequests) != 1 ||
		conclusion.ConfidenceTimeline[0].MissingEvidenceRequests[0].Label != "Deployment window" ||
		len(conclusion.ConfidenceTimeline[0].EvidenceCollectionSuggestions) != 1 ||
		conclusion.ConfidenceTimeline[0].EvidenceCollectionSuggestions[0].Label != "Latency trend" ||
		conclusion.ConfidenceTimeline[1].Confidence != api.ReportConfidenceHigh ||
		conclusion.ConfidenceTimeline[1].ConclusionStatus == nil ||
		*conclusion.ConfidenceTimeline[1].ConclusionStatus != "ready_for_review" ||
		conclusion.ConfidenceTimeline[1].ContextBytes == nil ||
		*conclusion.ConfidenceTimeline[1].ContextBytes != 2048 ||
		!slices.Equal(conclusion.ConfidenceTimeline[1].RetrievalRefs, []string{"sub_report:18", "final_report:9"}) ||
		conclusion.ConfidenceTimeline[1].EvidenceRequestCount != 0 {
		t.Fatalf("unexpected confidence timeline: %+v", conclusion.ConfidenceTimeline)
	}
	collectionResults := conclusion.ConfidenceTimeline[0].EvidenceCollectionResults
	if len(collectionResults) != 1 ||
		collectionResults[0].Tool != "metric_range_query" ||
		collectionResults[0].Status != "collected" ||
		collectionResults[0].ReasonCode == nil ||
		*collectionResults[0].ReasonCode != "ok" ||
		collectionResults[0].RequestReason == nil ||
		*collectionResults[0].RequestReason != "Need checkout deployment timing." ||
		collectionResults[0].ObservedMetricSeries == nil ||
		*collectionResults[0].ObservedMetricSeries != 2 ||
		collectionResults[0].AlertSourceKind == nil ||
		*collectionResults[0].AlertSourceKind != "prometheus" {
		t.Fatalf("unexpected evidence collection results: %+v", collectionResults)
	}
	progress := linked.DiagnosisProgress
	if progress == nil {
		t.Fatalf("diagnosis_progress is nil, want latest AI progress")
	}
	if progress.DiagnosisTaskID != 31 ||
		progress.SessionID == nil ||
		*progress.SessionID != "diagnosis-session-31" ||
		progress.ChatSessionID == nil ||
		*progress.ChatSessionID != 51 ||
		progress.EventKind != diagnosisConclusionEventTurnPersisted ||
		progress.Status != string(api.DiagnosisRoomProgressSummaryStatusInProgress) ||
		progress.EvidenceSnapshotID != 7 ||
		progress.Confidence != api.ReportConfidenceHigh ||
		progress.RequiresHumanReview ||
		progress.ConclusionStatus == nil ||
		*progress.ConclusionStatus != "ready_for_review" ||
		progress.ConfidenceRationale == nil ||
		*progress.ConfidenceRationale != "Deployment evidence explains the latency onset." ||
		progress.ContextBytes == nil ||
		*progress.ContextBytes != 2048 ||
		!slices.Equal(progress.RetrievalRefs, []string{"sub_report:18", "final_report:9"}) ||
		progress.EvidenceRequestCount != 0 ||
		len(progress.ConfidenceTimeline) != 2 ||
		len(progress.SupplementalEvidence) != 1 {
		t.Fatalf("unexpected diagnosis progress: %+v", progress)
	}
	if progress.SupplementalEvidence[0].AssistantSequence == nil ||
		*progress.SupplementalEvidence[0].AssistantSequence != 2 {
		t.Fatalf("progress supplemental evidence sequence = %+v, want assistant sequence 2", progress.SupplementalEvidence)
	}
	if len(progress.ConfidenceTimeline[0].EvidenceCollectionResults) != 1 ||
		progress.ConfidenceTimeline[0].EvidenceCollectionResults[0].Tool != "metric_range_query" {
		t.Fatalf("unexpected progress confidence timeline: %+v", progress.ConfidenceTimeline)
	}
	if len(progress.ConfidenceTimeline[0].EvidenceRequests) != 1 ||
		progress.ConfidenceTimeline[0].EvidenceRequests[0].AlertSourceProfileID == nil ||
		*progress.ConfidenceTimeline[0].EvidenceRequests[0].AlertSourceProfileID != 7 {
		t.Fatalf("unexpected progress evidence requests: %+v", progress.ConfidenceTimeline[0].EvidenceRequests)
	}
	room := linked.DiagnosisRoom
	if room == nil {
		t.Fatalf("diagnosis_room is nil, want exact room proof")
	}
	if room.SessionID != "diagnosis-session-31" ||
		room.ChatSessionID != 51 ||
		room.DiagnosisTaskID != 31 ||
		room.EvidenceSnapshotID != 7 ||
		room.WorkflowID != "diagnosis-room-31" ||
		room.RunID != "run-31" ||
		room.TurnCount != 2 {
		t.Fatalf("unexpected diagnosis room proof: %+v", room)
	}
	if len(room.NotificationTimeline) != 1 ||
		room.NotificationTimeline[0].EventKind != diagnosisConclusionEventFinalReadyNotification ||
		room.NotificationTimeline[0].ProviderMessageID == nil ||
		*room.NotificationTimeline[0].ProviderMessageID != "wecom-final-ready-31" ||
		room.NotificationTimeline[0].NotificationChannelProfileID == nil ||
		*room.NotificationTimeline[0].NotificationChannelProfileID != 2 {
		t.Fatalf("unexpected diagnosis room notification timeline: %+v", room.NotificationTimeline)
	}
	if factory.diagnosisRepo.lastChatSessionKey != "diagnosis-session-31" {
		t.Fatalf("last chat session key = %q, want diagnosis-session-31", factory.diagnosisRepo.lastChatSessionKey)
	}
}

func TestConfidenceTimelineEntryFromDiagnosisEventValidatesHistoricalReportRefs(t *testing.T) {
	eventWithRefs := func(rawRefs string) domain.DiagnosisTaskEvent {
		return domain.DiagnosisTaskEvent{
			ID:     77,
			TaskID: 31,
			Kind:   diagnosisConclusionEventTurnPersisted,
			Payload: json.RawMessage(fmt.Sprintf(`{
				"kind":"diagnosis_room.turn_persisted",
				"diagnosis_task_id":31,
				"confidence":"medium",
				"requires_human_review":true,
				"context_bytes":1024,
				"historical_report_refs":%s
			}`, rawRefs)),
			OccurredAt: time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC),
		}
	}

	got, ok, err := confidenceTimelineEntryFromDiagnosisEvent(
		eventWithRefs(`["sub_report:44","final_report:91"]`),
	)
	if err != nil || !ok {
		t.Fatalf("valid historical refs: ok=%t err=%v", ok, err)
	}
	if got.ContextBytes == nil ||
		*got.ContextBytes != 1024 ||
		!slices.Equal(got.RetrievalRefs, []string{"sub_report:44", "final_report:91"}) {
		t.Fatalf("confidence timeline historical retrieval = %+v", got)
	}

	for _, tc := range []struct {
		name string
		refs string
	}{
		{name: "duplicate", refs: `["final_report:91","final_report:91"]`},
		{name: "unnormalized", refs: `[" final_report:91"]`},
		{name: "unsupported kind", refs: `["incident:91"]`},
		{name: "invalid id", refs: `["sub_report:0"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := confidenceTimelineEntryFromDiagnosisEvent(eventWithRefs(tc.refs)); err == nil {
				t.Fatalf("historical refs %s: want invariant error", tc.refs)
			}
		})
	}
	for _, contextBytes := range []int{-1, diagnosisroom.HardMaxContextBytes + 1} {
		t.Run(fmt.Sprintf("context_bytes_%d", contextBytes), func(t *testing.T) {
			event := eventWithRefs(`["final_report:91"]`)
			event.Payload = json.RawMessage(strings.Replace(
				string(event.Payload),
				`"context_bytes":1024`,
				fmt.Sprintf(`"context_bytes":%d`, contextBytes),
				1,
			))
			if _, _, err := confidenceTimelineEntryFromDiagnosisEvent(event); err == nil {
				t.Fatalf("context_bytes %d: want invariant error", contextBytes)
			}
		})
	}
}

func TestGetReport_LinksDiagnosisRoomForNewestDiagnosisState(t *testing.T) {
	createdAt := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	oldRoomStartedAt := createdAt.Add(time.Minute)
	newRoomStartedAt := createdAt.Add(6 * time.Minute)
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{
					ID:                 13,
					CorrelationKey:     "window:checkout-followup",
					Title:              "Checkout follow-up",
					ExecutiveSummary:   "Checkout diagnosis needs additional evidence.",
					Severity:           domain.ReportSeverityWarning,
					Confidence:         domain.ReportConfidenceMedium,
					SubReports:         json.RawMessage(`[{"title":"Checkout follow-up","severity":"warning","summary":"The incident still needs evidence."}]`),
					RecommendedActions: json.RawMessage(`[{"label":"Collect evidence","detail":"Collect the missing deployment data.","priority":"high"}]`),
					NotificationText:   "Checkout follow-up needs evidence.",
					Content:            json.RawMessage(`{"title":"Checkout follow-up"}`),
					Model:              "gpt-4.1-mini",
					OutputMode:         "json_schema",
					CreatedByWorkflow:  "FinalReportWorkflow",
					CreatedAt:          createdAt,
				},
			},
			linkedSubReports: map[domain.FinalReportID][]domain.SubReport{
				13: {
					{
						ID:                 23,
						EvidenceSnapshotID: 9,
						Scenario:           "single_alert",
						Title:              "Checkout follow-up",
						Summary:            "The incident still needs evidence.",
						Severity:           domain.ReportSeverityWarning,
						Confidence:         domain.ReportConfidenceMedium,
						Findings:           json.RawMessage(`[{"label":"Latency warning","detail":"Latency remained above threshold.","evidence_id":"alert:checkout-followup"}]`),
						RecommendedActions: json.RawMessage(`[{"label":"Collect evidence","detail":"Collect the missing deployment data.","priority":"high"}]`),
						EvidenceRefs:       []string{"alert:checkout-followup"},
						Content:            json.RawMessage(`{"title":"Checkout follow-up"}`),
						Model:              "gpt-4.1-mini",
						OutputMode:         "json_schema",
						CreatedByWorkflow:  "ReportFanOutWorkflow",
						CreatedAt:          createdAt.Add(-time.Minute),
					},
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			tasksBySnapshot: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				9: {
					{
						ID:                 41,
						EvidenceSnapshotID: 9,
						WorkflowID:         "diagnosis-room-old",
						RunID:              "run-old",
						Status:             domain.DiagnosisStatusSucceeded,
						StartedAt:          &oldRoomStartedAt,
						CreatedAt:          oldRoomStartedAt,
						UpdatedAt:          createdAt.Add(3*time.Minute + time.Second),
					},
					{
						ID:                 42,
						EvidenceSnapshotID: 9,
						WorkflowID:         "diagnosis-room-new",
						RunID:              "run-new",
						Status:             domain.DiagnosisStatusRunning,
						StartedAt:          &newRoomStartedAt,
						CreatedAt:          newRoomStartedAt,
						UpdatedAt:          createdAt.Add(8*time.Minute + time.Second),
					},
				},
			},
			chatSessions: []domain.ChatSessionWithTask{
				{
					Session: domain.ChatSession{
						ID:              61,
						DiagnosisTaskID: 41,
						SessionKey:      "diagnosis-session-old",
						OwnerSubject:    "owner-1",
						Status:          domain.ChatSessionStatusClosed,
						TurnCount:       1,
						StartedAt:       oldRoomStartedAt,
						LastActivityAt:  createdAt.Add(3*time.Minute + time.Second),
						CreatedAt:       oldRoomStartedAt,
						UpdatedAt:       createdAt.Add(3*time.Minute + time.Second),
					},
					Task: domain.DiagnosisTask{
						ID:                 41,
						EvidenceSnapshotID: 9,
						WorkflowID:         "diagnosis-room-old",
						RunID:              "run-old",
						Status:             domain.DiagnosisStatusSucceeded,
						StartedAt:          &oldRoomStartedAt,
						CreatedAt:          oldRoomStartedAt,
						UpdatedAt:          createdAt.Add(3*time.Minute + time.Second),
					},
				},
				{
					Session: domain.ChatSession{
						ID:              62,
						DiagnosisTaskID: 42,
						SessionKey:      "diagnosis-session-new",
						OwnerSubject:    "owner-1",
						Status:          domain.ChatSessionStatusOpen,
						TurnCount:       2,
						StartedAt:       newRoomStartedAt,
						LastActivityAt:  createdAt.Add(8*time.Minute + time.Second),
						CreatedAt:       newRoomStartedAt,
						UpdatedAt:       createdAt.Add(8*time.Minute + time.Second),
					},
					Task: domain.DiagnosisTask{
						ID:                 42,
						EvidenceSnapshotID: 9,
						WorkflowID:         "diagnosis-room-new",
						RunID:              "run-new",
						Status:             domain.DiagnosisStatusRunning,
						StartedAt:          &newRoomStartedAt,
						CreatedAt:          newRoomStartedAt,
						UpdatedAt:          createdAt.Add(8*time.Minute + time.Second),
					},
				},
			},
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				41: {
					diagnosisConclusionEventFinalReady: {
						{
							ID:     51,
							TaskID: 41,
							Kind:   diagnosisConclusionEventFinalReady,
							Payload: json.RawMessage(`{
								"kind":"diagnosis_room.final_conclusion_ready",
								"session_id":"diagnosis-session-old",
								"chat_session_id":61,
								"diagnosis_task_id":41,
								"owner_subject":"owner-1",
								"turn_count":1,
								"final_conclusion":{
									"status":"available",
									"source":"latest_assistant_turn",
									"reason":"assistant_marked_final",
									"evidence_snapshot_id":9,
									"conclusion_version":"diagnosis-room-old.v1",
									"assistant_turn_id":71,
									"assistant_message_id":"old-assistant-message",
									"assistant_sequence":2,
									"assistant_occurred_at":"2026-06-21T09:03:00Z",
									"content":"Initial checkout diagnosis was ready before follow-up evidence was requested.",
									"confidence":"medium",
									"requires_human_review":false
								},
								"conclusion_version":"diagnosis-room-old.v1"
							}`),
							OccurredAt: createdAt.Add(3 * time.Minute),
							RecordedAt: createdAt.Add(3*time.Minute + time.Second),
						},
					},
				},
				42: {
					diagnosisConclusionEventTurnPersisted: {
						{
							ID:     52,
							TaskID: 42,
							Kind:   diagnosisConclusionEventTurnPersisted,
							Payload: json.RawMessage(`{
								"kind":"diagnosis_room.turn_persisted",
								"session_id":"diagnosis-session-new",
								"chat_session_id":62,
								"diagnosis_task_id":42,
								"assistant_message_id":"new-assistant-message",
								"assistant_turn_id":73,
								"assistant_sequence":4,
								"turn_count":2,
								"confidence":"high",
								"requires_human_review":true,
								"evidence_requests":[{
									"tool":"metric_range_query",
									"reason":"Need the deployment window for the follow-up diagnosis.",
									"query":"sum(rate(checkout_requests_total[5m]))",
									"alert_source_profile_id":7,
									"window_seconds":1800,
									"step_seconds":60,
									"limit":5
								}],
								"consultation_insight":{
									"confidence_rationale":"The follow-up turn supersedes the earlier final-ready event.",
									"conclusion_status":"needs_evidence"
								}
							}`),
							OccurredAt: createdAt.Add(8 * time.Minute),
							RecordedAt: createdAt.Add(8*time.Minute + time.Second),
						},
					},
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports/13", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var body api.FinalReportDetail
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.LinkedSubReports) != 1 {
		t.Fatalf("len(linked_sub_reports) = %d, want 1", len(body.LinkedSubReports))
	}
	linked := body.LinkedSubReports[0]
	if linked.DiagnosisConclusion == nil || linked.DiagnosisConclusion.SessionID != "diagnosis-session-old" {
		t.Fatalf("diagnosis_conclusion = %+v, want old conclusion", linked.DiagnosisConclusion)
	}
	progress := linked.DiagnosisProgress
	if progress == nil {
		t.Fatal("diagnosis_progress is nil, want newest progress")
	}
	if progress.DiagnosisTaskID != 42 ||
		progress.SessionID == nil ||
		*progress.SessionID != "diagnosis-session-new" ||
		progress.ChatSessionID == nil ||
		*progress.ChatSessionID != 62 ||
		progress.EventKind != diagnosisConclusionEventTurnPersisted ||
		progress.Status != string(api.DiagnosisRoomProgressSummaryStatusInProgress) ||
		progress.Confidence != api.ReportConfidenceHigh ||
		!progress.RequiresHumanReview ||
		len(progress.EvidenceRequests) != 1 ||
		progress.EvidenceRequests[0].AlertSourceProfileID == nil ||
		*progress.EvidenceRequests[0].AlertSourceProfileID != 7 {
		t.Fatalf("unexpected newest diagnosis progress: %+v", progress)
	}
	room := linked.DiagnosisRoom
	if room == nil {
		t.Fatal("diagnosis_room is nil, want newest state room")
	}
	if room.SessionID != "diagnosis-session-new" ||
		room.ChatSessionID != 62 ||
		room.DiagnosisTaskID != 42 ||
		room.EvidenceSnapshotID != 9 ||
		room.WorkflowID != "diagnosis-room-new" ||
		room.RunID != "run-new" ||
		room.TurnCount != 2 {
		t.Fatalf("unexpected diagnosis room proof: %+v", room)
	}
	if room.LatestProgress == nil ||
		room.LatestProgress.SessionID == nil ||
		*room.LatestProgress.SessionID != "diagnosis-session-new" {
		t.Fatalf("latest room progress = %+v, want new room progress", room.LatestProgress)
	}
	if room.LatestConclusion != nil {
		t.Fatalf("latest room conclusion = %+v, want nil for new running room", room.LatestConclusion)
	}
	if factory.diagnosisRepo.lastChatSessionKey != "diagnosis-session-new" {
		t.Fatalf("last chat session key = %q, want diagnosis-session-new", factory.diagnosisRepo.lastChatSessionKey)
	}
}

func TestDiagnosisConclusionFromEventProjectsProvenance(t *testing.T) {
	recordedAt := time.Date(2026, 5, 27, 10, 9, 0, 0, time.UTC)
	occurredAt := recordedAt.Add(-time.Minute)
	event := domain.DiagnosisTaskEvent{
		ID:         domain.DiagnosisTaskEventID(41),
		TaskID:     domain.DiagnosisTaskID(31),
		Kind:       diagnosisConclusionEventClosed,
		OccurredAt: recordedAt,
		RecordedAt: recordedAt,
		Payload: json.RawMessage(`{
			"kind":"diagnosis_room.closed",
			"session_id":"diagnosis-session-31",
			"chat_session_id":51,
			"diagnosis_task_id":31,
			"owner_subject":"owner-1",
			"turn_count":1,
			"final_conclusion":{
				"status":"available",
				"source":"latest_assistant_turn",
				"evidence_snapshot_id":7,
				"conclusion_version":"diagnosis-room-close.v1",
				"recorded_at":"2026-05-27T10:09:00Z",
				"confirmed_by":"owner-1",
				"supplemental_context_refs":["chat_session:51/turn:60","chat_session:51/turn:61"],
				"assistant_turn_id":61,
				"assistant_message_id":"msg-1/assistant",
				"assistant_sequence":2,
				"assistant_occurred_at":"2026-05-27T10:08:00Z",
				"content":"Checkout latency remains correlated with the deployment.",
				"confidence":"high",
				"requires_human_review":true,
				"confidence_rationale":"Deployment timing and active alerts point to checkout.",
				"findings":["Deployment overlaps the alert onset."],
				"recommended_actions":["Roll back the checkout deployment."],
				"evidence_requests":[{
					"tool":"metric_range_query",
					"reason":"Confirm checkout p95 remained elevated.",
					"query":"histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
					"alert_source_profile_id":7,
					"window_seconds":1800,
					"step_seconds":60,
					"limit":5
				}],
				"missing_evidence_requests":[{
					"label":"Owner sign-off",
					"detail":"Confirm the rollback window with the service owner.",
					"priority":"medium"
				}],
				"evidence_collection_suggestions":[{
					"label":"Post-rollback latency",
					"detail":"Collect p95 latency after rollback starts.",
					"priority":"medium"
				}]
			},
			"conclusion_version":"diagnosis-room-close.v1"
		}`),
	}

	conclusion, ok, err := diagnosisConclusionFromEvent(event)
	if err != nil {
		t.Fatalf("diagnosisConclusionFromEvent: %v", err)
	}
	if !ok {
		t.Fatal("diagnosisConclusionFromEvent ok = false, want true")
	}
	if conclusion.EvidenceSnapshotID == nil ||
		*conclusion.EvidenceSnapshotID != 7 ||
		conclusion.ConclusionVersion == nil ||
		*conclusion.ConclusionVersion != "diagnosis-room-close.v1" ||
		conclusion.ConfirmedBy == nil ||
		*conclusion.ConfirmedBy != "owner-1" ||
		len(conclusion.SupplementalContextRefs) != 2 ||
		conclusion.SupplementalContextRefs[1] != "chat_session:51/turn:61" ||
		conclusion.AssistantOccurredAt == nil ||
		!conclusion.AssistantOccurredAt.Equal(occurredAt) ||
		conclusion.ConfidenceRationale == nil ||
		*conclusion.ConfidenceRationale != "Deployment timing and active alerts point to checkout." ||
		len(conclusion.Findings) != 1 ||
		conclusion.Findings[0] != "Deployment overlaps the alert onset." ||
		len(conclusion.RecommendedActions) != 1 ||
		conclusion.RecommendedActions[0] != "Roll back the checkout deployment." ||
		len(conclusion.EvidenceRequests) != 1 ||
		conclusion.EvidenceRequests[0].Tool != "metric_range_query" ||
		conclusion.EvidenceRequests[0].AlertSourceProfileID == nil ||
		*conclusion.EvidenceRequests[0].AlertSourceProfileID != 7 ||
		len(conclusion.MissingEvidenceRequests) != 1 ||
		conclusion.MissingEvidenceRequests[0].Label != "Owner sign-off" ||
		len(conclusion.EvidenceCollectionSuggestions) != 1 ||
		conclusion.EvidenceCollectionSuggestions[0].Label != "Post-rollback latency" {
		t.Fatalf("conclusion provenance = %+v", conclusion)
	}
}

func TestEvidenceCollectionResultsFromDiagnosisEventIndexesManualCollectionByTurnCount(t *testing.T) {
	occurredAt := time.Date(2026, 6, 21, 9, 12, 0, 0, time.UTC)
	event := domain.DiagnosisTaskEvent{
		ID:         domain.DiagnosisTaskEventID(71),
		TaskID:     domain.DiagnosisTaskID(41),
		Kind:       diagnosisConclusionEventEvidenceCollected,
		OccurredAt: occurredAt,
		RecordedAt: occurredAt.Add(time.Second),
		Payload: json.RawMessage(`{
			"kind":"diagnosis_room.evidence_collected",
			"session_id":"diagnosis-session-41",
			"chat_session_id":61,
			"diagnosis_task_id":41,
			"owner_subject":"owner-1",
			"actor_subject":"owner-1",
			"user_message_id":"collect-manual-1",
			"turn_count":2,
			"evidence_collection_results":[{
				"tool":"metric_query",
				"status":"collected",
				"reason_code":"ok",
				"message":"Metric query collection succeeded.",
				"request_reason":"Need current API availability.",
				"query":"up{job=\"api\"}",
				"template_id":15,
				"alert_source_profile_id":13,
				"alert_source_kind":"prometheus",
				"limit":3,
				"observed_metric_series":1,
				"collected_at":"2026-06-21T09:12:00Z"
			}]
		}`),
	}

	keys, results, ok, err := evidenceCollectionResultsFromDiagnosisEvent(event)
	if err != nil {
		t.Fatalf("evidenceCollectionResultsFromDiagnosisEvent: %v", err)
	}
	if !ok {
		t.Fatal("evidenceCollectionResultsFromDiagnosisEvent ok = false, want true")
	}
	if len(keys) != 1 || keys[0] != "turn:2" {
		t.Fatalf("keys = %+v, want turn:2 only", keys)
	}
	if len(results) != 1 ||
		results[0].Tool != "metric_query" ||
		results[0].Status != "collected" ||
		results[0].ReasonCode == nil ||
		*results[0].ReasonCode != "ok" ||
		results[0].RequestReason == nil ||
		*results[0].RequestReason != "Need current API availability." ||
		results[0].Query == nil ||
		*results[0].Query != `up{job="api"}` ||
		results[0].TemplateID == nil ||
		*results[0].TemplateID != 15 ||
		results[0].AlertSourceProfileID == nil ||
		*results[0].AlertSourceProfileID != 13 ||
		results[0].AlertSourceKind == nil ||
		*results[0].AlertSourceKind != "prometheus" ||
		results[0].ObservedMetricSeries == nil ||
		*results[0].ObservedMetricSeries != 1 ||
		!results[0].CollectedAt.Equal(occurredAt) {
		t.Fatalf("results = %+v", results)
	}
}

func TestGetReport_ProjectsFailedDiagnosisProgress(t *testing.T) {
	createdAt := time.Date(2026, 6, 19, 20, 2, 0, 0, time.UTC)
	startedAt := createdAt.Add(time.Minute)
	finishedAt := startedAt.Add(15 * time.Second)
	failureReason := "initial turn failed: llm request timed out"
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{
					ID:                 12,
					CorrelationKey:     "window:database-capacity",
					Title:              "Database capacity warning",
					ExecutiveSummary:   "Database capacity requires review.",
					Severity:           domain.ReportSeverityWarning,
					Confidence:         domain.ReportConfidenceMedium,
					SubReports:         json.RawMessage(`[{"title":"Database capacity","severity":"warning","summary":"Capacity crossed warning threshold."}]`),
					RecommendedActions: json.RawMessage(`[{"label":"Inspect capacity","detail":"Check current tablespace utilization.","priority":"high"}]`),
					NotificationText:   "Database capacity warning requires review.",
					Content:            json.RawMessage(`{"title":"Database capacity warning"}`),
					Model:              "gpt-4.1-mini",
					OutputMode:         "json_schema",
					CreatedByWorkflow:  "FinalReportWorkflow",
					CreatedAt:          createdAt,
				},
			},
			linkedSubReports: map[domain.FinalReportID][]domain.SubReport{
				12: {
					{
						ID:                 22,
						EvidenceSnapshotID: 8,
						Scenario:           "single_alert",
						Title:              "Database capacity",
						Summary:            "Capacity crossed warning threshold.",
						Severity:           domain.ReportSeverityWarning,
						Confidence:         domain.ReportConfidenceMedium,
						Findings:           json.RawMessage(`[{"label":"High capacity","detail":"Capacity crossed warning threshold.","evidence_id":"alert:database-capacity"}]`),
						RecommendedActions: json.RawMessage(`[{"label":"Inspect capacity","detail":"Check current tablespace utilization.","priority":"high"}]`),
						EvidenceRefs:       []string{"alert:database-capacity"},
						Content:            json.RawMessage(`{"title":"Database capacity"}`),
						Model:              "gpt-4.1-mini",
						OutputMode:         "json_schema",
						CreatedByWorkflow:  "ReportFanOutWorkflow",
						CreatedAt:          createdAt.Add(-time.Minute),
					},
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			tasksBySnapshot: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				8: {
					{
						ID:                 32,
						EvidenceSnapshotID: 8,
						WorkflowID:         "diagnosis-room-32",
						RunID:              "run-32",
						Status:             domain.DiagnosisStatusFailed,
						FailureReason:      failureReason,
						StartedAt:          &startedAt,
						FinishedAt:         &finishedAt,
						CreatedAt:          startedAt,
						UpdatedAt:          finishedAt,
					},
				},
			},
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				32: {
					diagnosisConclusionEventFailed: {
						{
							ID:     45,
							TaskID: 32,
							Kind:   diagnosisConclusionEventFailed,
							Payload: json.RawMessage(`{
								"kind":"diagnosis_room.failed",
								"session_id":"diagnosis-session-32",
								"chat_session_id":52,
								"diagnosis_task_id":32,
								"evidence_snapshot_id":8,
								"status":"failed",
								"failure_reason":"initial turn failed: llm request timed out",
								"close_reason":"initial_turn_failed",
								"closed_at":"2026-06-19T20:03:15Z"
							}`),
							OccurredAt: finishedAt,
							RecordedAt: finishedAt.Add(time.Second),
						},
					},
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports/12", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var body api.FinalReportDetail
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.LinkedSubReports) != 1 {
		t.Fatalf("len(linked_sub_reports) = %d, want 1", len(body.LinkedSubReports))
	}
	progress := body.LinkedSubReports[0].DiagnosisProgress
	if progress == nil {
		t.Fatal("diagnosis_progress is nil, want failed progress")
	}
	if progress.DiagnosisTaskID != 32 ||
		progress.SessionID == nil ||
		*progress.SessionID != "diagnosis-session-32" ||
		progress.ChatSessionID == nil ||
		*progress.ChatSessionID != 52 ||
		progress.EventKind != diagnosisConclusionEventFailed ||
		progress.Status != string(domain.DiagnosisStatusFailed) ||
		progress.EvidenceSnapshotID != 8 ||
		progress.Confidence != api.ReportConfidenceLow ||
		!progress.RequiresHumanReview ||
		progress.FailureReason == nil ||
		*progress.FailureReason != failureReason ||
		progress.EvidenceRequestCount != 0 ||
		!progress.OccurredAt.Equal(finishedAt) ||
		!progress.RecordedAt.Equal(finishedAt.Add(time.Second)) {
		t.Fatalf("unexpected failed diagnosis progress: %+v", progress)
	}
}

func TestGetReport_FiltersDiagnosisStateWithoutRoomRead(t *testing.T) {
	createdAt := time.Date(2026, 6, 19, 20, 2, 0, 0, time.UTC)
	startedAt := createdAt.Add(time.Minute)
	finishedAt := startedAt.Add(15 * time.Second)
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{
					ID:                 14,
					CorrelationKey:     "window:database-capacity",
					Title:              "Database capacity warning",
					ExecutiveSummary:   "Database capacity requires review.",
					Severity:           domain.ReportSeverityWarning,
					Confidence:         domain.ReportConfidenceMedium,
					SubReports:         json.RawMessage(`[{"title":"Database capacity","severity":"warning","summary":"Capacity crossed warning threshold."}]`),
					RecommendedActions: json.RawMessage(`[{"label":"Inspect capacity","detail":"Check current tablespace utilization.","priority":"high"}]`),
					NotificationText:   "Database capacity warning requires review.",
					Content:            json.RawMessage(`{"title":"Database capacity warning"}`),
					Model:              "gpt-4.1-mini",
					OutputMode:         "json_schema",
					CreatedByWorkflow:  "FinalReportWorkflow",
					CreatedAt:          createdAt,
				},
			},
			linkedSubReports: map[domain.FinalReportID][]domain.SubReport{
				14: {
					{
						ID:                 24,
						EvidenceSnapshotID: 8,
						Scenario:           "single_alert",
						Title:              "Database capacity",
						Summary:            "Capacity crossed warning threshold.",
						Severity:           domain.ReportSeverityWarning,
						Confidence:         domain.ReportConfidenceMedium,
						Findings:           json.RawMessage(`[{"label":"High capacity","detail":"Capacity crossed warning threshold.","evidence_id":"alert:database-capacity"}]`),
						RecommendedActions: json.RawMessage(`[{"label":"Inspect capacity","detail":"Check current tablespace utilization.","priority":"high"}]`),
						EvidenceRefs:       []string{"alert:database-capacity"},
						Content:            json.RawMessage(`{"title":"Database capacity"}`),
						Model:              "gpt-4.1-mini",
						OutputMode:         "json_schema",
						CreatedByWorkflow:  "ReportFanOutWorkflow",
						CreatedAt:          createdAt.Add(-time.Minute),
					},
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			tasksBySnapshot: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				8: {
					{
						ID:                 32,
						EvidenceSnapshotID: 8,
						WorkflowID:         "diagnosis-room-32",
						RunID:              "run-32",
						Status:             domain.DiagnosisStatusFailed,
						StartedAt:          &startedAt,
						FinishedAt:         &finishedAt,
						CreatedAt:          startedAt,
						UpdatedAt:          finishedAt,
					},
				},
			},
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				32: {
					diagnosisConclusionEventFailed: {
						{
							ID:     45,
							TaskID: 32,
							Kind:   diagnosisConclusionEventFailed,
							Payload: json.RawMessage(`{
								"kind":"diagnosis_room.failed",
								"session_id":"diagnosis-session-32",
								"chat_session_id":52,
								"diagnosis_task_id":32,
								"evidence_snapshot_id":8,
								"status":"failed",
								"failure_reason":"initial turn failed: llm request timed out",
								"close_reason":"initial_turn_failed",
								"closed_at":"2026-06-19T20:03:15Z"
							}`),
							OccurredAt: finishedAt,
							RecordedAt: finishedAt.Add(time.Second),
						},
					},
				},
			},
		},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed:   req.Permission == domain.RBACPermissionOperationsRead,
				CheckedAt: createdAt,
			}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports/14", nil)
	testOperationsReadHandlerWithAuthorizer(t, factory, authorizer).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.FinalReportDetail
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ID != 14 || len(body.LinkedSubReports) != 1 {
		t.Fatalf("report detail = %+v, want report with one linked subreport", body)
	}
	linked := body.LinkedSubReports[0]
	if linked.DiagnosisConclusion != nil || linked.DiagnosisProgress != nil || linked.DiagnosisRoom != nil {
		t.Fatalf("diagnosis state = conclusion=%+v progress=%+v room=%+v, want all filtered", linked.DiagnosisConclusion, linked.DiagnosisProgress, linked.DiagnosisRoom)
	}
	if len(authorizer.requests) != 3 {
		t.Fatalf("authorizer requests = %+v, want operations, global room read, scoped room read", authorizer.requests)
	}
	if authorizer.requests[0].Permission != domain.RBACPermissionOperationsRead ||
		authorizer.requests[0].ScopeKind != domain.RBACScopeKindGlobal {
		t.Fatalf("operations read request = %+v", authorizer.requests[0])
	}
	if authorizer.requests[1].Permission != domain.RBACPermissionDiagnosisRoomRead ||
		authorizer.requests[1].ScopeKind != domain.RBACScopeKindGlobal {
		t.Fatalf("global room read request = %+v", authorizer.requests[1])
	}
	if authorizer.requests[2].Permission != domain.RBACPermissionDiagnosisRoomRead ||
		authorizer.requests[2].ScopeKind != domain.RBACScopeKindDiagnosisRoom ||
		authorizer.requests[2].ScopeKey != "diagnosis-session-32" {
		t.Fatalf("scoped room read request = %+v", authorizer.requests[2])
	}
}

func TestGetReport_ReturnsNotFound(t *testing.T) {
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports/999", nil)
	testOperationsReadHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestListReportWorkflowPoliciesReturnsPolicies(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
				{
					ID:                                 1,
					Name:                               "Default report workflow",
					AlertSourceProfileID:               1,
					GroupingPolicyID:                   2,
					ReportNotificationChannelProfileID: 3,
					TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
					ReportScenario:                     domain.ReportWorkflowScenarioSingleAlert,
					DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeSuggestRoom,
					Enabled:                            false,
					CreatedAt:                          createdAt,
					UpdatedAt:                          createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/report-workflow-policies?limit=1", nil)
	testConfigHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.ReportWorkflowPolicyListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Default report workflow" || body.Items[0].Enabled {
		t.Fatalf("items = %+v", body.Items)
	}
	if !body.Items[0].EnabledAt.IsNull() || !body.Items[0].DisabledAt.IsNull() {
		t.Fatalf("enablement timestamps should be explicit null: %+v", body.Items[0])
	}
	reportChannelID, err := body.Items[0].ReportNotificationChannelProfileID.Get()
	if err != nil || reportChannelID != 3 {
		t.Fatalf("report notification channel ID = %v, %v; want 3", reportChannelID, err)
	}
}

func TestCreateReportWorkflowPolicyStoresDisabledDraft(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles:         []domain.AlertSourceProfile{{ID: 1, Enabled: false}},
		groupingPolicies:            []domain.GroupingPolicy{{ID: 2, Enabled: false}},
		notificationChannelProfiles: []domain.NotificationChannelProfile{{ID: 3, Enabled: false}},
		saveReportWorkflowPolicyResult: domain.ReportWorkflowPolicy{
			ID:                                 7,
			Name:                               "Default report workflow",
			AlertSourceProfileID:               1,
			GroupingPolicyID:                   2,
			ReportNotificationChannelProfileID: 3,
			TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
			ReportScenario:                     domain.ReportWorkflowScenarioSingleAlert,
			DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeSuggestRoom,
			Enabled:                            false,
			CreatedAt:                          time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC),
			UpdatedAt:                          time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC),
		},
	}

	body := `{
		"name":"Default report workflow",
		"alert_source_profile_id":1,
		"grouping_policy_id":2,
		"report_notification_channel_profile_id":3,
		"trigger_mode":"manual_replay",
		"report_scenario":"single_alert",
		"diagnosis_follow_up":"suggest_room"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedReportWorkflowPolicy.Enabled {
		t.Fatalf("saved policy should be disabled: %+v", repo.savedReportWorkflowPolicy)
	}
	if repo.savedReportWorkflowPolicy.ReportNotificationChannelProfileID != 3 {
		t.Fatalf("saved report notification channel ID = %d, want 3", repo.savedReportWorkflowPolicy.ReportNotificationChannelProfileID)
	}
	var resp api.ReportWorkflowPolicy
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 7 || resp.ReportScenario != api.ReportWorkflowScenarioSingleAlert || resp.Enabled {
		t.Fatalf("response = %+v", resp)
	}
	reportChannelID, err := resp.ReportNotificationChannelProfileID.Get()
	if err != nil || reportChannelID != 3 {
		t.Fatalf("response report notification channel ID = %v, %v; want 3", reportChannelID, err)
	}
}

func TestReplaceReportWorkflowPolicyAuthorizesBindingsBeforeSaving(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles:         []domain.AlertSourceProfile{{ID: 1, Enabled: false}},
		groupingPolicies:            []domain.GroupingPolicy{{ID: 2, Enabled: false}},
		notificationChannelProfiles: []domain.NotificationChannelProfile{{ID: 3, Enabled: false}},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{
			ID:                   7,
			Name:                 "Existing report workflow",
			AlertSourceProfileID: 1,
			GroupingPolicyID:     2,
			TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
			ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
			DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
		}},
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: time.Date(2026, 6, 5, 8, 30, 0, 0, time.UTC)},
	}
	body := `{
		"name":"Updated report workflow",
		"alert_source_profile_id":1,
		"grouping_policy_id":2,
		"report_notification_channel_profile_id":3,
		"trigger_mode":"manual_replay",
		"report_scenario":"cascade",
		"diagnosis_follow_up":"suggest_room"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPut, "/api/v1/config/report-workflow-policies/7", strings.NewReader(body))
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: repo}, testLocalRBACOptions(t, "policy-manager-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updatedReportWorkflowPolicy.ID != 7 ||
		repo.updatedReportWorkflowPolicy.Name != "Updated report workflow" ||
		repo.updatedReportWorkflowPolicy.ReportNotificationChannelProfileID != 3 {
		t.Fatalf("updated policy = %+v", repo.updatedReportWorkflowPolicy)
	}
	if len(authorizer.requests) != 4 {
		t.Fatalf("authorizer requests = %+v, want policy manage plus three binding checks", authorizer.requests)
	}
	if authorizer.requests[0].Permission != domain.RBACPermissionReportWorkflowManage ||
		authorizer.requests[0].ScopeKind != domain.RBACScopeKindReportWorkflow ||
		authorizer.requests[0].ScopeKey != "7" {
		t.Fatalf("policy manage request = %+v", authorizer.requests[0])
	}
	if authorizer.requests[1].Permission != domain.RBACPermissionAlertSourceRead ||
		authorizer.requests[1].ScopeKind != domain.RBACScopeKindAlertSource ||
		authorizer.requests[1].ScopeKey != "1" {
		t.Fatalf("alert source request = %+v", authorizer.requests[1])
	}
	if authorizer.requests[2].Permission != domain.RBACPermissionGroupingPolicyRead ||
		authorizer.requests[2].ScopeKind != domain.RBACScopeKindGroupingPolicy ||
		authorizer.requests[2].ScopeKey != "2" {
		t.Fatalf("grouping policy request = %+v", authorizer.requests[2])
	}
	if authorizer.requests[3].Permission != domain.RBACPermissionNotificationChannelTest ||
		authorizer.requests[3].ScopeKind != domain.RBACScopeKindNotificationChannel ||
		authorizer.requests[3].ScopeKey != "3" {
		t.Fatalf("notification channel request = %+v", authorizer.requests[3])
	}
}

func TestReplaceReportWorkflowPolicyRejectsUnauthorizedBindings(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles:         []domain.AlertSourceProfile{{ID: 1, Enabled: false}},
		groupingPolicies:            []domain.GroupingPolicy{{ID: 2, Enabled: false}},
		notificationChannelProfiles: []domain.NotificationChannelProfile{{ID: 3, Enabled: false}},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{
			ID:                   7,
			Name:                 "Existing report workflow",
			AlertSourceProfileID: 1,
			GroupingPolicyID:     2,
			TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
			ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
			DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
		}},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			allowed := req.Permission != domain.RBACPermissionNotificationChannelTest
			return rbacusecase.AuthorizeDecision{Allowed: allowed}, nil
		},
	}
	body := `{
		"name":"Updated report workflow",
		"alert_source_profile_id":1,
		"grouping_policy_id":2,
		"report_notification_channel_profile_id":3,
		"trigger_mode":"manual_replay",
		"report_scenario":"cascade",
		"diagnosis_follow_up":"suggest_room"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPut, "/api/v1/config/report-workflow-policies/7", strings.NewReader(body))
	addTestLocalRBACAuthorization(req)
	testHandler(&fakeUOWFactory{configRepo: repo}, testLocalRBACOptions(t, "policy-manager-1", authorizer)...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updatedReportWorkflowPolicy.ID != 0 {
		t.Fatalf("policy should not be updated: %+v", repo.updatedReportWorkflowPolicy)
	}
	if len(authorizer.requests) != 4 ||
		authorizer.requests[3].Permission != domain.RBACPermissionNotificationChannelTest ||
		authorizer.requests[3].ScopeKey != "3" {
		t.Fatalf("authorizer requests = %+v", authorizer.requests)
	}
}

func TestEnableReportWorkflowPolicyRequiresReadyBindings(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{{ID: 1, Enabled: false}},
		groupingPolicies:    []domain.GroupingPolicy{{ID: 2, Enabled: true}},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                   7,
				Name:                 "Default report workflow",
				AlertSourceProfileID: 1,
				GroupingPolicyID:     2,
				TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/enable", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updatedReportWorkflowPolicy.ID != 0 {
		t.Fatalf("policy should not be updated on failed enablement: %+v", repo.updatedReportWorkflowPolicy)
	}
}

func TestEnableAndDisableReportWorkflowPolicyToggleState(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{{ID: 1, Enabled: true}},
		groupingPolicies:    []domain.GroupingPolicy{{ID: 2, Enabled: true}},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                   7,
				Name:                 "Default report workflow",
				AlertSourceProfileID: 1,
				GroupingPolicyID:     2,
				TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
			},
		},
	}

	enableRec := httptest.NewRecorder()
	enableReq := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/enable", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(enableRec, enableReq)
	if enableRec.Code != stdhttp.StatusOK {
		t.Fatalf("enable status = %d, want 200; body=%s", enableRec.Code, enableRec.Body.String())
	}
	var enabled api.ReportWorkflowPolicy
	if err := json.NewDecoder(enableRec.Body).Decode(&enabled); err != nil {
		t.Fatalf("decode enable: %v", err)
	}
	if !enabled.Enabled || enabled.EnabledAt.IsNull() || !enabled.DisabledAt.IsNull() {
		t.Fatalf("enabled response = %+v", enabled)
	}

	disableRec := httptest.NewRecorder()
	disableReq := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/disable", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(disableRec, disableReq)
	if disableRec.Code != stdhttp.StatusOK {
		t.Fatalf("disable status = %d, want 200; body=%s", disableRec.Code, disableRec.Body.String())
	}
	var disabled api.ReportWorkflowPolicy
	if err := json.NewDecoder(disableRec.Body).Decode(&disabled); err != nil {
		t.Fatalf("decode disable: %v", err)
	}
	if disabled.Enabled || !disabled.EnabledAt.IsNull() || disabled.DisabledAt.IsNull() {
		t.Fatalf("disabled response = %+v", disabled)
	}
}

func TestPreviewReportWorkflowPolicyImpactReturnsReadinessAndGroupingImpact(t *testing.T) {
	base := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{
				ID:       1,
				Kind:     domain.AlertSourceKindPrometheus,
				AuthMode: domain.AlertSourceAuthModeBearer,
				Enabled:  true,
			},
		},
		groupingPolicies: []domain.GroupingPolicy{
			{
				ID:            2,
				DimensionKeys: []string{"alertname"},
				SeverityKey:   "severity",
				SourceFilter:  []string{"prometheus"},
				Enabled:       true,
			},
		},
		notificationChannelProfiles: []domain.NotificationChannelProfile{
			{
				ID:             3,
				Enabled:        true,
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
			},
		},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                                 7,
				Name:                               "Default report workflow",
				AlertSourceProfileID:               1,
				GroupingPolicyID:                   2,
				ReportNotificationChannelProfileID: 3,
				TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:                     domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeSuggestRoom,
			},
		},
	}
	alerts := &fakeAlertRepo{
		events: []domain.AlertEvent{
			{
				ID:                   101,
				Source:               "prometheus",
				AlertSourceProfileID: 1,
				Labels:               map[string]string{"alertname": "checkout", "severity": "critical"},
				StartsAt:             base,
			},
			{
				ID:                   102,
				Source:               "alertmanager",
				AlertSourceProfileID: 1,
				Labels:               map[string]string{"alertname": "payments", "severity": "warning"},
				StartsAt:             base.Add(time.Minute),
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/impact-preview?limit=2", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo, alertRepo: alerts}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if alerts.lastLimit != 2 {
		t.Fatalf("alert limit = %d, want 2", alerts.lastLimit)
	}
	var body api.ReportWorkflowPolicyImpactPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.PolicyID != 7 ||
		body.Status != api.ReportWorkflowPolicyImpactPreviewStatusReady ||
		len(body.ReasonCodes) != 1 ||
		body.ReasonCodes[0] != api.ReportWorkflowPolicyImpactPreviewReasonCodeOk {
		t.Fatalf("readiness = %+v", body)
	}
	channelID, err := body.ReportNotificationChannelProfileID.Get()
	if err != nil || channelID != 3 {
		t.Fatalf("channel ID = %v, %v; want 3", channelID, err)
	}
	if !body.ReportNotificationChannelBound ||
		!body.ReportNotificationChannelEnabled ||
		!body.ReportNotificationChannelHasReportScope ||
		body.ReportNotificationChannelHasDiagnosisConsultationScope ||
		body.ReportNotificationChannelHasDiagnosisCloseScope {
		t.Fatalf("channel readiness = %+v", body)
	}
	if body.EventsScanned != 2 || body.EventsMatched != 1 || body.GroupsEstimated != 1 || len(body.Groups) != 1 {
		t.Fatalf("impact counts = %+v", body)
	}
	if body.Groups[0].EventCount != 1 || body.Groups[0].Dimensions["alertname"] != "checkout" {
		t.Fatalf("group = %+v", body.Groups[0])
	}
}

func TestPreviewReportWorkflowPolicyDraftImpactReturnsReadinessWithoutPersisting(t *testing.T) {
	base := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{
				ID:       1,
				Kind:     domain.AlertSourceKindAlertmanager,
				AuthMode: domain.AlertSourceAuthModeNone,
				Enabled:  true,
			},
		},
		groupingPolicies: []domain.GroupingPolicy{
			{
				ID:            2,
				DimensionKeys: []string{"alertname"},
				SeverityKey:   "severity",
				SourceFilter:  []string{"alertmanager"},
				Enabled:       true,
			},
		},
		notificationChannelProfiles: []domain.NotificationChannelProfile{
			{
				ID:      3,
				Kind:    domain.NotificationChannelKindWeCom,
				Enabled: true,
				DeliveryScopes: []domain.NotificationDeliveryScope{
					domain.NotificationDeliveryScopeDiagnosisClose,
					domain.NotificationDeliveryScopeDiagnosisConsultation,
					domain.NotificationDeliveryScopeReport,
				},
				LatestTestProofs: impactPreviewNotificationProofs(3, domain.NotificationChannelKindWeCom, base),
			},
		},
	}
	alerts := &fakeAlertRepo{
		events: []domain.AlertEvent{
			{
				ID:                   101,
				Source:               "alertmanager",
				AlertSourceProfileID: 1,
				Labels:               map[string]string{"alertname": "checkout", "severity": "critical"},
				StartsAt:             base,
			},
			{
				ID:                   102,
				Source:               "alertmanager",
				AlertSourceProfileID: 1,
				Labels:               map[string]string{"alertname": "checkout", "severity": "warning"},
				StartsAt:             base.Add(time.Minute),
			},
		},
	}
	body := strings.NewReader(`{
		"name": "Unsaved automatic diagnosis workflow",
		"alert_source_profile_id": 1,
		"grouping_policy_id": 2,
		"report_notification_channel_profile_id": 3,
		"trigger_mode": "manual_replay",
		"report_scenario": "cascade",
		"diagnosis_follow_up": "auto_room"
	}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/impact-preview?limit=2", body)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo, alertRepo: alerts}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if alerts.lastLimit != 2 {
		t.Fatalf("alert limit = %d, want 2", alerts.lastLimit)
	}
	if len(repo.reportWorkflowPolicies) != 0 || repo.savedReportWorkflowPolicy.Name != "" {
		t.Fatalf("draft preview should not persist policy: policies=%d saved=%+v", len(repo.reportWorkflowPolicies), repo.savedReportWorkflowPolicy)
	}
	var got api.ReportWorkflowPolicyImpactPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.PolicyID != 0 ||
		got.Status != api.ReportWorkflowPolicyImpactPreviewStatusReady ||
		len(got.ReasonCodes) != 1 ||
		got.ReasonCodes[0] != api.ReportWorkflowPolicyImpactPreviewReasonCodeOk {
		t.Fatalf("readiness = %+v", got)
	}
	if got.ReportScenario != api.ReportWorkflowScenarioCascade ||
		got.DiagnosisFollowUp != api.DiagnosisFollowUpModeAutoRoom {
		t.Fatalf("scenario/followup = %s/%s", got.ReportScenario, got.DiagnosisFollowUp)
	}
	if got.EventsScanned != 2 || got.EventsMatched != 2 || got.GroupsEstimated != 1 || len(got.Groups) != 1 {
		t.Fatalf("impact counts = %+v", got)
	}
	if !got.ReportNotificationChannelHasReportScope ||
		!got.ReportNotificationChannelHasDiagnosisConsultationScope ||
		!got.ReportNotificationChannelHasDiagnosisCloseScope {
		t.Fatalf("notification scopes = %+v", got)
	}
}

func TestPreviewReportWorkflowPolicyImpactRequiresDiagnosisCloseScopeForAutoRoom(t *testing.T) {
	base := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{ID: 1, Kind: domain.AlertSourceKindAlertmanager, AuthMode: domain.AlertSourceAuthModeBearer, Enabled: true},
		},
		groupingPolicies: []domain.GroupingPolicy{
			{ID: 2, DimensionKeys: []string{"alertname"}, SeverityKey: "severity", SourceFilter: []string{"prometheus"}, Enabled: true},
		},
		notificationChannelProfiles: []domain.NotificationChannelProfile{
			{
				ID:             3,
				Kind:           domain.NotificationChannelKindWeCom,
				Enabled:        true,
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
			},
		},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                                 7,
				Name:                               "Auto-room report workflow",
				AlertSourceProfileID:               1,
				GroupingPolicyID:                   2,
				ReportNotificationChannelProfileID: 3,
				TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:                     domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeAutoRoom,
			},
		},
	}
	alerts := &fakeAlertRepo{events: []domain.AlertEvent{
		{
			ID:       101,
			Source:   "prometheus",
			Labels:   map[string]string{"alertname": "checkout", "severity": "critical"},
			StartsAt: base,
		},
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/impact-preview", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo, alertRepo: alerts}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.ReportWorkflowPolicyImpactPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != api.ReportWorkflowPolicyImpactPreviewStatusBlocked ||
		len(body.ReasonCodes) != 2 ||
		body.ReasonCodes[0] != api.ReportWorkflowPolicyImpactPreviewReasonCodeNotificationChannelMissingDiagnosisConsultationScope ||
		body.ReasonCodes[1] != api.ReportWorkflowPolicyImpactPreviewReasonCodeNotificationChannelMissingDiagnosisCloseScope {
		t.Fatalf("readiness = %+v", body)
	}
	if !body.ReportNotificationChannelHasReportScope ||
		body.ReportNotificationChannelHasDiagnosisConsultationScope ||
		body.ReportNotificationChannelHasDiagnosisCloseScope {
		t.Fatalf("channel scopes = %+v", body)
	}
}

func TestPreviewReportWorkflowPolicyImpactRequiresNotificationChannelForAutoRoom(t *testing.T) {
	base := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{ID: 1, Kind: domain.AlertSourceKindAlertmanager, AuthMode: domain.AlertSourceAuthModeBearer, Enabled: true},
		},
		groupingPolicies: []domain.GroupingPolicy{
			{ID: 2, DimensionKeys: []string{"alertname"}, SeverityKey: "severity", SourceFilter: []string{"prometheus"}, Enabled: true},
		},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                   7,
				Name:                 "Auto-room report workflow",
				AlertSourceProfileID: 1,
				GroupingPolicyID:     2,
				TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:    domain.DiagnosisFollowUpModeAutoRoom,
			},
		},
	}
	alerts := &fakeAlertRepo{events: []domain.AlertEvent{
		{
			ID:       101,
			Source:   "prometheus",
			Labels:   map[string]string{"alertname": "checkout", "severity": "critical"},
			StartsAt: base,
		},
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/impact-preview", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo, alertRepo: alerts}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.ReportWorkflowPolicyImpactPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != api.ReportWorkflowPolicyImpactPreviewStatusBlocked ||
		len(body.ReasonCodes) != 1 ||
		body.ReasonCodes[0] != api.ReportWorkflowPolicyImpactPreviewReasonCodeNotificationChannelMissing {
		t.Fatalf("readiness = %+v", body)
	}
	if body.ReportNotificationChannelBound {
		t.Fatalf("channel bound = true, want false")
	}
}

func TestPreviewReportWorkflowPolicyImpactRequiresAlertmanagerSourceForAutoRoom(t *testing.T) {
	base := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{ID: 1, Kind: domain.AlertSourceKindPrometheus, AuthMode: domain.AlertSourceAuthModeBearer, Enabled: true},
		},
		groupingPolicies: []domain.GroupingPolicy{
			{ID: 2, DimensionKeys: []string{"alertname"}, SeverityKey: "severity", SourceFilter: []string{"prometheus"}, Enabled: true},
		},
		notificationChannelProfiles: []domain.NotificationChannelProfile{
			{
				ID:      3,
				Kind:    domain.NotificationChannelKindWeCom,
				Enabled: true,
				DeliveryScopes: []domain.NotificationDeliveryScope{
					domain.NotificationDeliveryScopeDiagnosisClose,
					domain.NotificationDeliveryScopeDiagnosisConsultation,
					domain.NotificationDeliveryScopeReport,
				},
				LatestTestProofs: impactPreviewNotificationProofs(3, domain.NotificationChannelKindWeCom, base),
			},
		},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                                 7,
				Name:                               "Auto-room report workflow",
				AlertSourceProfileID:               1,
				GroupingPolicyID:                   2,
				ReportNotificationChannelProfileID: 3,
				TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:                     domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeAutoRoom,
			},
		},
	}
	alerts := &fakeAlertRepo{events: []domain.AlertEvent{
		{
			ID:       101,
			Source:   "prometheus",
			Labels:   map[string]string{"alertname": "checkout", "severity": "critical"},
			StartsAt: base,
		},
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/impact-preview", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo, alertRepo: alerts}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.ReportWorkflowPolicyImpactPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != api.ReportWorkflowPolicyImpactPreviewStatusBlocked ||
		len(body.ReasonCodes) != 1 ||
		body.ReasonCodes[0] != api.ReportWorkflowPolicyImpactPreviewReasonCodeAutoRoomRequiresAlertmanager {
		t.Fatalf("readiness = %+v", body)
	}
}

func TestPreviewReportWorkflowPolicyImpactReturnsBlockedReadiness(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{ID: 1, Kind: domain.AlertSourceKindPrometheus, AuthMode: domain.AlertSourceAuthModeNone, Enabled: false},
		},
		groupingPolicies: []domain.GroupingPolicy{
			{ID: 2, DimensionKeys: []string{"alertname"}, SeverityKey: "severity", SourceFilter: []string{"prometheus"}, Enabled: false},
		},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                   7,
				Name:                 "Default report workflow",
				AlertSourceProfileID: 1,
				GroupingPolicyID:     2,
				TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/impact-preview", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo, alertRepo: &fakeAlertRepo{}}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.ReportWorkflowPolicyImpactPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != api.ReportWorkflowPolicyImpactPreviewStatusBlocked ||
		len(body.ReasonCodes) != 2 ||
		body.ReasonCodes[0] != api.ReportWorkflowPolicyImpactPreviewReasonCodeAlertSourceDisabled ||
		body.ReasonCodes[1] != api.ReportWorkflowPolicyImpactPreviewReasonCodeGroupingPolicyDisabled {
		t.Fatalf("readiness = %+v", body)
	}
	if !body.ReportNotificationChannelProfileID.IsNull() || body.ReportNotificationChannelBound {
		t.Fatalf("unbound channel fields = %+v", body)
	}
}

func impactPreviewNotificationProofs(
	profileID domain.NotificationChannelProfileID,
	kind domain.NotificationChannelKind,
	checkedAt time.Time,
) []domain.NotificationChannelTestProof {
	return []domain.NotificationChannelTestProof{
		impactPreviewNotificationProof(profileID, kind, domain.NotificationChannelTestContentAIDiagnosisSample, checkedAt),
		impactPreviewNotificationProof(profileID, kind, domain.NotificationChannelTestContentDiagnosisCloseSample, checkedAt),
	}
}

func impactPreviewNotificationProof(
	profileID domain.NotificationChannelProfileID,
	kind domain.NotificationChannelKind,
	contentKind domain.NotificationChannelTestContentKind,
	checkedAt time.Time,
) domain.NotificationChannelTestProof {
	return domain.NotificationChannelTestProof{
		NotificationChannelProfileID: profileID,
		Kind:                         kind,
		Status:                       domain.NotificationChannelTestStatusSuccess,
		ReasonCode:                   domain.NotificationChannelTestReasonOK,
		Message:                      "Notification channel test delivery succeeded.",
		ContentKind:                  contentKind,
		ContentSHA256:                strings.Repeat("c", 64),
		CheckedAt:                    checkedAt,
		ProviderMessageID:            "provider-message-1",
		ProviderStatus:               "delivered",
	}
}

func TestReportWorkflowPolicyWriteRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		repo       *fakeConfigRepo
		wantStatus int
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies",
			body:       `{"name":"Default","alert_source_profile_id":1,"grouping_policy_id":2,"extra":true}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "invalid_scenario",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies",
			body:       `{"name":"Default","alert_source_profile_id":1,"grouping_policy_id":2,"report_scenario":"unknown"}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "invalid_notification_channel_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies",
			body:       `{"name":"Default","alert_source_profile_id":1,"grouping_policy_id":2,"report_notification_channel_profile_id":0}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:   "missing_binding",
			method: stdhttp.MethodPost,
			path:   "/api/v1/config/report-workflow-policies",
			body:   `{"name":"Default","alert_source_profile_id":1,"grouping_policy_id":2}`,
			repo: &fakeConfigRepo{
				alertSourceProfiles: []domain.AlertSourceProfile{{ID: 1}},
			},
			wantStatus: stdhttp.StatusNotFound,
		},
		{
			name:       "invalid_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies/0/enable",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_policy",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies/99/disable",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			testConfigHandler(t, &fakeUOWFactory{configRepo: tc.repo}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestListReportWorkflowSchedulesReturnsSchedules(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 2, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			reportWorkflowSchedules: []domain.ReportWorkflowSchedule{
				{
					ID:                     1,
					Name:                   "Daily report window",
					ReportWorkflowPolicyID: 7,
					TemporalScheduleID:     "openclarion-report-policy-7-daily",
					Interval:               24 * time.Hour,
					Offset:                 6 * time.Hour,
					ReplayWindow:           time.Hour,
					ReplayDelay:            5 * time.Minute,
					ReplayLimit:            10000,
					CatchupWindow:          time.Hour,
					Enabled:                false,
					CreatedAt:              createdAt,
					UpdatedAt:              createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/report-workflow-schedules?limit=1", nil)
	testConfigHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.ReportWorkflowScheduleListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Daily report window" {
		t.Fatalf("items = %+v", body.Items)
	}
	if body.Items[0].IntervalSeconds != 86400 ||
		body.Items[0].OffsetSeconds != 21600 ||
		body.Items[0].Cadence != "interval" ||
		body.Items[0].CalendarHour != 0 ||
		body.Items[0].CalendarMinute != 0 ||
		body.Items[0].CalendarDayOfWeek != 0 ||
		body.Items[0].CalendarDayOfMonth != 0 ||
		body.Items[0].ReplayWindowSeconds != 3600 ||
		body.Items[0].ReplayDelaySeconds != 300 ||
		body.Items[0].CatchupWindowSeconds != 3600 {
		t.Fatalf("duration fields = %+v", body.Items[0])
	}
}

func TestCreateReportWorkflowScheduleStoresSecondsAsDurations(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
	}
	body := `{
		"name":"Daily report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-daily",
		"interval_seconds":86400,
		"offset_seconds":21600,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedReportWorkflowSchedule.ReportWorkflowPolicyID != 7 ||
		repo.savedReportWorkflowSchedule.Cadence != domain.ReportWorkflowScheduleCadenceInterval ||
		repo.savedReportWorkflowSchedule.Interval != 24*time.Hour ||
		repo.savedReportWorkflowSchedule.Offset != 6*time.Hour ||
		repo.savedReportWorkflowSchedule.ReplayWindow != time.Hour ||
		repo.savedReportWorkflowSchedule.ReplayDelay != 5*time.Minute ||
		repo.savedReportWorkflowSchedule.ReplayLimit != 10000 ||
		repo.savedReportWorkflowSchedule.CatchupWindow != time.Hour {
		t.Fatalf("saved schedule = %+v", repo.savedReportWorkflowSchedule)
	}
	var response api.ReportWorkflowSchedule
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if response.TemporalScheduleID != "openclarion-report-policy-7-daily" ||
		response.Cadence != "interval" ||
		response.Enabled {
		t.Fatalf("response = %+v", response)
	}
}

func TestCreateReportWorkflowScheduleStoresCalendarCadence(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
	}
	body := `{
		"name":"Weekly report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-weekly",
		"cadence":"weekly",
		"calendar_hour":2,
		"calendar_minute":30,
		"calendar_day_of_week":1,
		"calendar_day_of_month":0,
		"interval_seconds":604800,
		"offset_seconds":0,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedReportWorkflowSchedule.Cadence != domain.ReportWorkflowScheduleCadenceWeekly ||
		repo.savedReportWorkflowSchedule.CalendarHour != 2 ||
		repo.savedReportWorkflowSchedule.CalendarMinute != 30 ||
		repo.savedReportWorkflowSchedule.CalendarDayOfWeek != 1 ||
		repo.savedReportWorkflowSchedule.CalendarDayOfMonth != 0 ||
		repo.savedReportWorkflowSchedule.Interval != 7*24*time.Hour ||
		repo.savedReportWorkflowSchedule.Offset != 0 {
		t.Fatalf("saved schedule = %+v", repo.savedReportWorkflowSchedule)
	}
	var response api.ReportWorkflowSchedule
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if response.Cadence != "weekly" ||
		response.CalendarHour != 2 ||
		response.CalendarMinute != 30 ||
		response.CalendarDayOfWeek != 1 ||
		response.CalendarDayOfMonth != 0 {
		t.Fatalf("response = %+v", response)
	}
}

func TestReplaceReportWorkflowScheduleAuthorizesBoundPolicyBeforeSaving(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
		reportWorkflowSchedules: []domain.ReportWorkflowSchedule{{
			ID:                     9,
			Name:                   "Existing daily report window",
			ReportWorkflowPolicyID: 7,
			TemporalScheduleID:     "openclarion-report-policy-7-existing",
			Interval:               24 * time.Hour,
			Offset:                 0,
			ReplayWindow:           time.Hour,
			ReplayDelay:            5 * time.Minute,
			ReplayLimit:            10000,
			CatchupWindow:          time.Hour,
		}},
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: time.Date(2026, 6, 6, 3, 0, 0, 0, time.UTC)},
	}
	syncer := &recordingReportWorkflowScheduleSyncer{}
	body := `{
		"name":"Updated daily report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-daily",
		"interval_seconds":86400,
		"offset_seconds":21600,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPut, "/api/v1/config/report-workflow-schedules/9", strings.NewReader(body))
	addTestLocalRBACAuthorization(req)
	testHandler(
		&fakeUOWFactory{configRepo: repo},
		append(testLocalRBACOptions(t, "schedule-manager-1", authorizer), WithReportWorkflowScheduleSynchronizer(syncer))...,
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updatedReportWorkflowSchedule.ID != 9 ||
		repo.updatedReportWorkflowSchedule.Name != "Updated daily report window" ||
		repo.updatedReportWorkflowSchedule.ReportWorkflowPolicyID != 7 {
		t.Fatalf("updated schedule = %+v", repo.updatedReportWorkflowSchedule)
	}
	if syncer.calls != 1 || syncer.schedule.ID != 9 {
		t.Fatalf("syncer = %+v", syncer)
	}
	if len(authorizer.requests) != 2 {
		t.Fatalf("authorizer requests = %+v, want schedule manage and policy manage", authorizer.requests)
	}
	if authorizer.requests[0].Permission != domain.RBACPermissionReportWorkflowManage ||
		authorizer.requests[0].ScopeKind != domain.RBACScopeKindReportWorkflowSchedule ||
		authorizer.requests[0].ScopeKey != "9" {
		t.Fatalf("schedule manage request = %+v", authorizer.requests[0])
	}
	if authorizer.requests[1].Permission != domain.RBACPermissionReportWorkflowManage ||
		authorizer.requests[1].ScopeKind != domain.RBACScopeKindReportWorkflow ||
		authorizer.requests[1].ScopeKey != "7" {
		t.Fatalf("policy manage request = %+v", authorizer.requests[1])
	}
}

func TestReplaceReportWorkflowScheduleRejectsUnauthorizedPolicyBinding(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
		reportWorkflowSchedules: []domain.ReportWorkflowSchedule{{
			ID:                     9,
			Name:                   "Existing daily report window",
			ReportWorkflowPolicyID: 7,
			TemporalScheduleID:     "openclarion-report-policy-7-existing",
			Interval:               24 * time.Hour,
			Offset:                 0,
			ReplayWindow:           time.Hour,
			ReplayDelay:            5 * time.Minute,
			ReplayLimit:            10000,
			CatchupWindow:          time.Hour,
		}},
	}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			allowed := req.ScopeKind == domain.RBACScopeKindReportWorkflowSchedule && req.ScopeKey == "9"
			return rbacusecase.AuthorizeDecision{Allowed: allowed}, nil
		},
	}
	syncer := &recordingReportWorkflowScheduleSyncer{}
	body := `{
		"name":"Updated daily report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-daily",
		"interval_seconds":86400,
		"offset_seconds":21600,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPut, "/api/v1/config/report-workflow-schedules/9", strings.NewReader(body))
	addTestLocalRBACAuthorization(req)
	testHandler(
		&fakeUOWFactory{configRepo: repo},
		append(testLocalRBACOptions(t, "schedule-manager-1", authorizer), WithReportWorkflowScheduleSynchronizer(syncer))...,
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updatedReportWorkflowSchedule.ID != 0 {
		t.Fatalf("schedule should not be updated: %+v", repo.updatedReportWorkflowSchedule)
	}
	if syncer.calls != 0 {
		t.Fatalf("sync calls = %d, want 0", syncer.calls)
	}
	if len(authorizer.requests) != 2 ||
		authorizer.requests[1].Permission != domain.RBACPermissionReportWorkflowManage ||
		authorizer.requests[1].ScopeKind != domain.RBACScopeKindReportWorkflow ||
		authorizer.requests[1].ScopeKey != "7" {
		t.Fatalf("authorizer requests = %+v", authorizer.requests)
	}
}

func TestReportWorkflowScheduleMutationSynchronizesSavedSchedule(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
	}
	syncer := &recordingReportWorkflowScheduleSyncer{}
	body := `{
		"name":"Daily report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-daily",
		"interval_seconds":86400,
		"offset_seconds":21600,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules", strings.NewReader(body))
	testConfigHandler(t,
		&fakeUOWFactory{configRepo: repo},
		WithReportWorkflowScheduleSynchronizer(syncer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if syncer.calls != 1 ||
		syncer.schedule.ID == 0 ||
		syncer.schedule.TemporalScheduleID != "openclarion-report-policy-7-daily" ||
		syncer.schedule.Enabled {
		t.Fatalf("syncer = %+v", syncer)
	}
}

func TestReportWorkflowScheduleSyncFailureReturnsServerError(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
	}
	syncer := &recordingReportWorkflowScheduleSyncer{err: errors.New("temporal unavailable")}
	body := `{
		"name":"Daily report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-daily",
		"interval_seconds":86400,
		"offset_seconds":21600,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules", strings.NewReader(body))
	testConfigHandler(t,
		&fakeUOWFactory{configRepo: repo},
		WithReportWorkflowScheduleSynchronizer(syncer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "temporal unavailable") {
		t.Fatalf("response leaked synchronizer error: %s", rec.Body.String())
	}
	if syncer.calls != 1 || repo.savedReportWorkflowSchedule.Name != "Daily report window" {
		t.Fatalf("sync calls = %d saved = %+v", syncer.calls, repo.savedReportWorkflowSchedule)
	}
}

func TestEnableReportWorkflowScheduleRequiresEnabledPolicy(t *testing.T) {
	schedule := domain.ReportWorkflowSchedule{
		ID:                     9,
		Name:                   "Daily report window",
		ReportWorkflowPolicyID: 7,
		TemporalScheduleID:     "openclarion-report-policy-7-daily",
		Interval:               24 * time.Hour,
		Offset:                 6 * time.Hour,
		ReplayWindow:           time.Hour,
		ReplayDelay:            5 * time.Minute,
		ReplayLimit:            10000,
		CatchupWindow:          time.Hour,
	}
	repo := &fakeConfigRepo{
		reportWorkflowPolicies:  []domain.ReportWorkflowPolicy{{ID: 7, Enabled: true}},
		reportWorkflowSchedules: []domain.ReportWorkflowSchedule{schedule},
	}
	syncer := &recordingReportWorkflowScheduleSyncer{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules/9/enable", nil)
	testConfigHandler(t,
		&fakeUOWFactory{configRepo: repo},
		WithReportWorkflowScheduleSynchronizer(syncer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !repo.updatedReportWorkflowSchedule.Enabled || repo.updatedReportWorkflowSchedule.EnabledAt == nil {
		t.Fatalf("updated schedule = %+v", repo.updatedReportWorkflowSchedule)
	}
	if syncer.calls != 1 || !syncer.schedule.Enabled || syncer.schedule.EnabledAt == nil {
		t.Fatalf("syncer = %+v", syncer)
	}

	repo = &fakeConfigRepo{
		reportWorkflowPolicies:  []domain.ReportWorkflowPolicy{{ID: 7, Enabled: false}},
		reportWorkflowSchedules: []domain.ReportWorkflowSchedule{schedule},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules/9/enable", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("disabled policy status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestReportWorkflowScheduleWriteRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		repo       *fakeConfigRepo
		wantStatus int
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":86400,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600,"extra":true}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "invalid_offset",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":3600,"offset_seconds":3600,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}}},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "negative_policy_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":-7,"temporal_schedule_id":"schedule-1","interval_seconds":86400,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "oversized_duration",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":31536001,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "oversized_replay_limit",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":86400,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":100001,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "replay_window_exceeds_interval",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Every minute","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":60,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":0,"replay_limit":10000,"catchup_window_seconds":300}`,
			repo:       &fakeConfigRepo{reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}}},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_policy",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":86400,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusNotFound,
		},
		{
			name:       "invalid_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules/0/disable",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_schedule",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules/99/disable",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			testConfigHandler(t, &fakeUOWFactory{configRepo: tc.repo}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestListNotificationChannelProfilesReturnsProfiles(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			notificationChannelProfiles: []domain.NotificationChannelProfile{
				{
					ID:             1,
					Name:           "Operations webhook",
					Kind:           domain.NotificationChannelKindWebhook,
					SecretRef:      "secret/openclarion/ops-webhook",
					DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
					Enabled:        true,
					Labels:         map[string]string{"owner": "sre"},
					CreatedAt:      createdAt,
					UpdatedAt:      createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/notification-channels?limit=1", nil)
	testConfigHandler(t, factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.NotificationChannelProfileListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Operations webhook" || !body.Items[0].Enabled {
		t.Fatalf("items = %+v", body.Items)
	}
	if body.Items[0].SecretRef != "secret/openclarion/ops-webhook" || body.Items[0].DeliveryScopes[0] != api.Report {
		t.Fatalf("profile = %+v", body.Items[0])
	}
}

func TestCreateNotificationChannelProfileStoresSecretRefOnly(t *testing.T) {
	repo := &fakeConfigRepo{
		saveNotificationChannelResult: domain.NotificationChannelProfile{
			ID:             7,
			Name:           "Operations WeCom",
			Kind:           domain.NotificationChannelKindWeCom,
			SecretRef:      "secret/openclarion/ops-wecom",
			DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeDiagnosisClose, domain.NotificationDeliveryScopeReport},
			Enabled:        false,
			Labels:         map[string]string{"owner": "sre"},
			CreatedAt:      time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC),
		},
	}

	body := `{
		"name":"Operations WeCom",
		"kind":"wecom",
		"secret_ref":"secret/openclarion/ops-wecom",
		"delivery_scopes":["diagnosis_close","report"],
		"enabled":false,
		"labels":{"owner":"sre"}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/notification-channels", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedNotificationChannel.SecretRef != "secret/openclarion/ops-wecom" {
		t.Fatalf("saved notification channel = %+v", repo.savedNotificationChannel)
	}
	if len(repo.savedNotificationChannel.DeliveryScopes) != 2 ||
		repo.savedNotificationChannel.DeliveryScopes[0] != domain.NotificationDeliveryScopeDiagnosisClose ||
		repo.savedNotificationChannel.DeliveryScopes[1] != domain.NotificationDeliveryScopeReport {
		t.Fatalf("saved scopes = %+v", repo.savedNotificationChannel.DeliveryScopes)
	}
	var resp api.NotificationChannelProfile
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 7 || resp.Kind != api.Wecom || resp.Enabled {
		t.Fatalf("response = %+v", resp)
	}
}

func TestNotificationChannelProfileWriteRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		repo       *fakeConfigRepo
		wantStatus int
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/notification-channels",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":["report"],"extra":true}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_secret_ref",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/notification-channels",
			body:       `{"name":"Operations webhook","kind":"webhook","delivery_scopes":["report"]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "empty_delivery_scopes",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/notification-channels",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":[]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "webhook_diagnosis_scope",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/notification-channels",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":["report","diagnosis_close"]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "invalid_id",
			method:     stdhttp.MethodPut,
			path:       "/api/v1/config/notification-channels/0",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":["report"]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_channel",
			method:     stdhttp.MethodPut,
			path:       "/api/v1/config/notification-channels/99",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":["report"]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			testConfigHandler(t, &fakeUOWFactory{configRepo: tc.repo}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestNotificationChannelProfileTestReturnsSanitizedResult(t *testing.T) {
	checkedAt := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		notificationChannelProfiles: []domain.NotificationChannelProfile{
			{
				ID:             1,
				Name:           "Operations webhook",
				Kind:           domain.NotificationChannelKindWebhook,
				SecretRef:      "secret/openclarion/ops-webhook",
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
				Enabled:        false,
				Labels:         map[string]string{"owner": "sre"},
				CreatedAt:      checkedAt,
				UpdatedAt:      checkedAt,
			},
		},
	}
	tester := &fakeNotificationChannelTester{
		result: notificationchannelcheck.Result{
			ChannelID:         1,
			Kind:              domain.NotificationChannelKindWebhook,
			Status:            notificationchannelcheck.StatusBlocked,
			ReasonCode:        notificationchannelcheck.ReasonCredentialsUnavailable,
			Message:           "Secret-backed notification channel tests require a server-side secret resolver.",
			ContentKind:       "ai_diagnosis_sample",
			ContentSHA256:     strings.Repeat("a", 64),
			CheckedAt:         checkedAt,
			ProviderMessageID: "",
			ProviderStatus:    "",
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/notification-channels/1/test?content_kind=diagnosis_close_sample", nil)
	testConfigHandler(t, &fakeUOWFactory{configRepo: repo}, WithNotificationChannelTester(tester)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if tester.called != 1 ||
		tester.profile.ID != 1 ||
		tester.profile.SecretRef == "" ||
		tester.request.ContentKind != "diagnosis_close_sample" {
		t.Fatalf("tester called=%d profile=%+v request=%+v", tester.called, tester.profile, tester.request)
	}
	if body := rec.Body.String(); strings.Contains(body, "secret/openclarion") || strings.Contains(body, "ops-webhook") {
		t.Fatalf("response leaked secret reference: %s", body)
	}
	var resp api.NotificationChannelTestResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Status != api.NotificationChannelTestStatusBlocked ||
		resp.ReasonCode != api.NotificationChannelTestReasonCodeCredentialsUnavailable ||
		resp.ContentKind == nil ||
		*resp.ContentKind != "ai_diagnosis_sample" ||
		resp.ContentSha256 == nil ||
		*resp.ContentSha256 != strings.Repeat("a", 64) {
		t.Fatalf("response = %+v", resp)
	}
	if len(repo.savedNotificationChannelTestProofs) != 1 {
		t.Fatalf("saved test proofs = %+v, want 1", repo.savedNotificationChannelTestProofs)
	}
	savedProof := repo.savedNotificationChannelTestProofs[0]
	if savedProof.NotificationChannelProfileID != 1 ||
		savedProof.Status != domain.NotificationChannelTestStatusBlocked ||
		savedProof.ReasonCode != domain.NotificationChannelTestReasonCredentialsUnavailable ||
		savedProof.ContentKind != domain.NotificationChannelTestContentAIDiagnosisSample ||
		savedProof.ContentSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("saved proof = %+v", savedProof)
	}
}

func TestNotificationChannelProfileTestRejectsUnconfiguredAndMissingProfiles(t *testing.T) {
	tests := []struct {
		name       string
		repo       *fakeConfigRepo
		opts       []ServerOption
		wantStatus int
	}{
		{
			name:       "unconfigured",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusServiceUnavailable,
		},
		{
			name:       "not_found",
			repo:       &fakeConfigRepo{},
			opts:       []ServerOption{WithNotificationChannelTester(&fakeNotificationChannelTester{})},
			wantStatus: stdhttp.StatusNotFound,
		},
		{
			name:       "invalid_id",
			repo:       &fakeConfigRepo{},
			opts:       []ServerOption{WithNotificationChannelTester(&fakeNotificationChannelTester{})},
			wantStatus: stdhttp.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/config/notification-channels/99/test"
			if tc.name == "invalid_id" {
				path = "/api/v1/config/notification-channels/0/test"
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, path, nil)
			testConfigHandler(t, &fakeUOWFactory{configRepo: tc.repo}, tc.opts...).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestTriggerReportReplay_StartsReportWorkflow(t *testing.T) {
	windowStart := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	trigger := &fakeReportReplayTrigger{
		result: reporttrigger.Result{
			CorrelationKey: "incident-42",
			Replay: alertreplay.Result{
				Stats: alertreplay.Stats{
					Ingested:           alertingest.Stats{Total: 2, Saved: 2},
					EventsLoaded:       2,
					GroupsBuilt:        1,
					GroupsSaved:        1,
					SnapshotsSaved:     1,
					SnapshotsDuplicate: 0,
					GroupsClosed:       1,
				},
				Snapshots: []alertreplay.SnapshotRef{
					{ID: 7, GroupIndex: 0, EventCount: 2},
				},
			},
			Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-1", RunID: "run-1"},
			Started:  true,
		},
	}

	body := `{
		"window_start":"2026-05-27T09:00:00Z",
		"window_end":"2026-05-27T10:00:00Z",
		"limit":2,
		"alert_event_id":42,
		"correlation_key":"incident-42",
		"workflow_id":"report-batch-1",
		"scenario":"cascade"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/report-triggers/replay-window", strings.NewReader(body))
	addTestLocalRBACAuthorization(req)
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   true,
			CheckedAt: time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC),
		},
	}
	opts := testLocalRBACOptions(t, "report-manager-1", authorizer)
	opts = append(opts, WithReportReplayTrigger(trigger))
	testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, opts...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 ||
		authorizer.req.Permission != domain.RBACPermissionReportWorkflowManage ||
		authorizer.req.ScopeKind != domain.RBACScopeKindGlobal {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if trigger.called != 1 {
		t.Fatalf("trigger calls = %d, want 1", trigger.called)
	}
	if !trigger.req.Replay.WindowStart.Equal(windowStart) || !trigger.req.Replay.WindowEnd.Equal(windowEnd) {
		t.Fatalf("trigger window = %s..%s", trigger.req.Replay.WindowStart, trigger.req.Replay.WindowEnd)
	}
	if trigger.req.Replay.Limit != 2 {
		t.Fatalf("trigger limit = %d, want 2", trigger.req.Replay.Limit)
	}
	if len(trigger.req.Replay.AlertEventIDFilter) != 1 || trigger.req.Replay.AlertEventIDFilter[0] != 42 {
		t.Fatalf("trigger alert event filter = %+v, want [42]", trigger.req.Replay.AlertEventIDFilter)
	}
	if trigger.req.Replay.Grouping.SeverityKey != alertgrouping.DefaultConfig().SeverityKey {
		t.Fatalf("trigger grouping = %+v", trigger.req.Replay.Grouping)
	}
	if trigger.req.CorrelationKey != "incident-42" || trigger.req.WorkflowID != "report-batch-1" || trigger.req.Scenario != reportprompt.ScenarioCascade {
		t.Fatalf("trigger identity = %+v", trigger.req)
	}

	var resp api.ReportReplayTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Started || resp.CorrelationKey != "incident-42" || resp.WorkflowID != "report-batch-1" || resp.RunID != "run-1" {
		t.Fatalf("response workflow = %+v", resp)
	}
	if resp.Stats.Ingested.Total != 2 || resp.Stats.SnapshotsSaved != 1 {
		t.Fatalf("response stats = %+v", resp.Stats)
	}
	if len(resp.Snapshots) != 1 || resp.Snapshots[0].ID != 7 || resp.Snapshots[0].EventCount != 2 {
		t.Fatalf("response snapshots = %+v", resp.Snapshots)
	}
}

func TestRetryReportNotification_ReturnsDeliveryProof(t *testing.T) {
	deliveredAt := time.Date(2026, 6, 21, 9, 30, 0, 0, time.UTC)
	createdAt := deliveredAt.Add(-time.Minute)
	updatedAt := deliveredAt
	factory := reportNotificationRetryReadyFactory(
		"owner-1",
		deliveredAt.Add(-2*time.Minute),
	)
	sender := &fakeReportNotificationSender{
		result: reportnotification.Result{
			Delivery: domain.ReportNotificationDelivery{
				ID:                                 31,
				FinalReportID:                      11,
				ReportNotificationChannelProfileID: 2,
				IdempotencyKey:                     "final_report:11/notification/final",
				ProviderMessageID:                  "msg-31",
				ProviderStatus:                     "accepted",
				Status:                             domain.ReportNotificationDeliveryStatusDelivered,
				DeliveredAt:                        &deliveredAt,
				CreatedAt:                          createdAt,
				UpdatedAt:                          updatedAt,
			},
		},
	}
	authorizer := allowedReportNotificationRBACAuthorizer()
	handler := testReportNotificationRetryHandler(t, factory, sender, authorizer)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/reports/11/notification/retry",
		strings.NewReader(`{"report_notification_channel_profile_id":2,"notification_purpose":"final"}`),
	)
	addTestLocalRBACAuthorization(req)
	handler.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 ||
		authorizer.req.Permission != domain.RBACPermissionReportWorkflowManage ||
		authorizer.req.ScopeKind != domain.RBACScopeKindGlobal {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if sender.called != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.called)
	}
	if sender.req.FinalReportID != 11 ||
		sender.req.ReportNotificationChannelProfileID != 2 ||
		sender.req.NotificationPurpose != reportnotification.NotificationPurposeFinal {
		t.Fatalf("sender request = %+v", sender.req)
	}

	var resp api.ReportNotificationRetryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Delivery.ID != 31 ||
		resp.RetryState != api.ReportNotificationRetryStateSent ||
		resp.Delivery.IdempotencyKey != "final_report:11/notification/final" ||
		resp.Delivery.NotificationPurpose != api.Final ||
		resp.Delivery.Status != api.ReportNotificationDeliveryStatusDelivered ||
		resp.Delivery.ReportNotificationChannelProfileID.IsNull() ||
		resp.Delivery.ProviderMessageID == nil ||
		*resp.Delivery.ProviderMessageID != "msg-31" ||
		resp.Delivery.ProviderStatus == nil ||
		*resp.Delivery.ProviderStatus != "accepted" ||
		resp.Delivery.DeliveredAt == nil ||
		!resp.Delivery.DeliveredAt.Equal(deliveredAt) {
		t.Fatalf("delivery proof = %+v", resp.Delivery)
	}
	profileID, err := resp.Delivery.ReportNotificationChannelProfileID.Get()
	if err != nil || profileID != 2 {
		t.Fatalf("delivery profile id = %d err=%v, want 2", profileID, err)
	}
}

func TestRetryReportNotification_ReturnsPendingDeliveryProof(t *testing.T) {
	createdAt := time.Date(2026, 6, 21, 9, 45, 0, 0, time.UTC)
	factory := reportNotificationRetryReadyFactory(
		"owner-1",
		createdAt.Add(-2*time.Minute),
	)
	sender := &fakeReportNotificationSender{
		result: reportnotification.Result{
			RetryState: reportnotification.RetryStateAlreadyPending,
			Delivery: domain.ReportNotificationDelivery{
				ID:                                 32,
				FinalReportID:                      11,
				ReportNotificationChannelProfileID: 2,
				IdempotencyKey:                     "final_report:11/notification/final",
				Status:                             domain.ReportNotificationDeliveryStatusPending,
				CreatedAt:                          createdAt,
				UpdatedAt:                          createdAt,
			},
		},
	}
	handler := testReportNotificationRetryHandler(t, factory, sender, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/reports/11/notification/retry",
		strings.NewReader(`{"report_notification_channel_profile_id":2,"notification_purpose":"final"}`),
	)
	addTestLocalRBACAuthorization(req)
	handler.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if sender.called != 1 ||
		sender.req.NotificationPurpose != reportnotification.NotificationPurposeFinal {
		t.Fatalf("sender called=%d request=%+v, want one final retry lookup", sender.called, sender.req)
	}
	var resp api.ReportNotificationRetryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Delivery.ID != 32 ||
		resp.RetryState != api.ReportNotificationRetryStateAlreadyPending ||
		resp.Delivery.NotificationPurpose != api.Final ||
		resp.Delivery.Status != api.ReportNotificationDeliveryStatusPending ||
		resp.Delivery.ProviderMessageID != nil ||
		resp.Delivery.ProviderStatus != nil ||
		resp.Delivery.DeliveredAt != nil {
		t.Fatalf("delivery proof = %+v, want pending final proof without provider result", resp.Delivery)
	}
}

func TestReportDetailReadyStateAllowsFinalNotificationRetry(t *testing.T) {
	deliveredAt := time.Date(2026, 6, 21, 10, 45, 0, 0, time.UTC)
	reportID := domain.FinalReportID(11)
	factory := reportNotificationRetryReadyFactory(
		"owner-1",
		deliveredAt.Add(-2*time.Minute),
	)
	sender := &fakeReportNotificationSender{
		result: reportnotification.Result{
			Delivery: domain.ReportNotificationDelivery{
				ID:                                 41,
				FinalReportID:                      reportID,
				ReportNotificationChannelProfileID: 2,
				IdempotencyKey:                     "final_report:11/notification/final",
				ProviderMessageID:                  "msg-final-41",
				ProviderStatus:                     "accepted",
				Status:                             domain.ReportNotificationDeliveryStatusDelivered,
				DeliveredAt:                        &deliveredAt,
				CreatedAt:                          deliveredAt.Add(-time.Minute),
				UpdatedAt:                          deliveredAt,
			},
		},
	}
	handler := testReportNotificationRetryHandler(t, factory, sender, nil)

	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports/11", nil)
	addTestLocalRBACAuthorization(getReq)
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != stdhttp.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", getRec.Code, getRec.Body.String())
	}
	var detail api.FinalReportDetail
	if err := json.NewDecoder(getRec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode report detail: %v", err)
	}
	if !detail.FinalNotificationReadiness.Ready ||
		detail.FinalNotificationReadiness.NotificationPurpose != api.Final ||
		detail.FinalNotificationReadiness.Status != string(api.ReportFinalNotificationReadinessStatusReady) {
		t.Fatalf("final notification readiness = %+v, want ready final state", detail.FinalNotificationReadiness)
	}

	postRec := httptest.NewRecorder()
	postReq := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/reports/11/notification/retry",
		strings.NewReader(`{"report_notification_channel_profile_id":2,"notification_purpose":"final"}`),
	)
	addTestLocalRBACAuthorization(postReq)
	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != stdhttp.StatusOK {
		t.Fatalf("POST status = %d, want 200; body=%s", postRec.Code, postRec.Body.String())
	}
	if sender.called != 1 ||
		sender.req.FinalReportID != reportID ||
		sender.req.NotificationPurpose != reportnotification.NotificationPurposeFinal {
		t.Fatalf("sender called=%d request=%+v, want one final retry", sender.called, sender.req)
	}
	var retry api.ReportNotificationRetryResponse
	if err := json.NewDecoder(postRec.Body).Decode(&retry); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if retry.Delivery.NotificationPurpose != api.Final ||
		retry.RetryState != api.ReportNotificationRetryStateSent ||
		retry.Delivery.ProviderMessageID == nil ||
		*retry.Delivery.ProviderMessageID != "msg-final-41" {
		t.Fatalf("retry delivery = %+v, want final provider proof", retry.Delivery)
	}
}

func TestRetryReportNotification_RejectsFinalPurposeWithoutConfirmedConclusion(t *testing.T) {
	readyAt := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	factory := reportNotificationRetryReadyFactory(
		"",
		readyAt,
	)
	sender := &fakeReportNotificationSender{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/reports/11/notification/retry",
		strings.NewReader(`{"notification_purpose":"final"}`),
	)
	addTestLocalRBACAuthorization(req)
	testReportNotificationRetryHandler(t, factory, sender, nil).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if sender.called != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.called)
	}
}

func TestRetryReportNotification_RejectsFinalPurposeWhenProgressIsNewer(t *testing.T) {
	readyAt := time.Date(2026, 6, 21, 10, 15, 0, 0, time.UTC)
	taskID := domain.DiagnosisTaskID(31)
	factory := reportNotificationRetryReadyFactory("owner-1", readyAt)
	factory.diagnosisRepo.eventsByTaskAndKind[taskID][diagnosisConclusionEventTurnPersisted] = []domain.DiagnosisTaskEvent{
		reportNotificationRetryProgressEvent(taskID, readyAt.Add(time.Minute), readyAt.Add(time.Minute+time.Second)),
	}
	sender := &fakeReportNotificationSender{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/reports/11/notification/retry",
		strings.NewReader(`{"notification_purpose":"final"}`),
	)
	addTestLocalRBACAuthorization(req)
	testReportNotificationRetryHandler(t, factory, sender, nil).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if sender.called != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.called)
	}
}

func TestReportFinalNotificationReadiness(t *testing.T) {
	confirmedBy := "owner-1"
	confirmedAt := time.Date(2026, 6, 21, 10, 30, 0, 0, time.UTC)
	subReport := domain.SubReport{
		ID:                 21,
		EvidenceSnapshotID: 7,
		Title:              "Checkout API latency",
	}
	confirmedConclusion := api.DiagnosisRoomConclusionSummary{
		ConfirmedBy: &confirmedBy,
		RecordedAt:  confirmedAt,
	}

	tests := []struct {
		name                string
		subReports          []domain.SubReport
		conclusions         diagnosisConclusionBySnapshot
		progress            diagnosisProgressBySnapshot
		wantReady           bool
		wantPurpose         api.ReportNotificationPurpose
		wantStatus          string
		wantDetailSubstring string
	}{
		{
			name:                "no linked subreports",
			wantPurpose:         api.Handoff,
			wantStatus:          string(api.ReportFinalNotificationReadinessStatusBlocked),
			wantDetailSubstring: "no linked subreports",
		},
		{
			name:       "unconfirmed conclusion",
			subReports: []domain.SubReport{subReport},
			conclusions: diagnosisConclusionBySnapshot{
				7: {RecordedAt: confirmedAt},
			},
			wantPurpose:         api.Handoff,
			wantStatus:          string(api.ReportFinalNotificationReadinessStatusBlocked),
			wantDetailSubstring: "Checkout API latency has no operator-confirmed AI conclusion yet",
		},
		{
			name:       "newer progress after confirmation",
			subReports: []domain.SubReport{subReport},
			conclusions: diagnosisConclusionBySnapshot{
				7: confirmedConclusion,
			},
			progress: diagnosisProgressBySnapshot{
				7: {
					OccurredAt: confirmedAt.Add(time.Minute),
					RecordedAt: confirmedAt.Add(time.Minute + time.Second),
				},
			},
			wantPurpose:         api.Handoff,
			wantStatus:          string(api.ReportFinalNotificationReadinessStatusBlocked),
			wantDetailSubstring: "newer diagnosis progress after the confirmed conclusion",
		},
		{
			name:       "ready",
			subReports: []domain.SubReport{subReport},
			conclusions: diagnosisConclusionBySnapshot{
				7: confirmedConclusion,
			},
			wantReady:           true,
			wantPurpose:         api.Final,
			wantStatus:          string(api.ReportFinalNotificationReadinessStatusReady),
			wantDetailSubstring: "final notification can be sent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reportFinalNotificationReadiness(
				domain.FinalReportID(11),
				tc.subReports,
				tc.conclusions,
				tc.progress,
			)
			if got.Ready != tc.wantReady ||
				got.NotificationPurpose != tc.wantPurpose ||
				got.Status != tc.wantStatus ||
				!strings.Contains(got.Detail, tc.wantDetailSubstring) {
				t.Fatalf("readiness = %+v", got)
			}
		})
	}
}

func TestRetryReportNotification_AllowsEmptyBody(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 45, 0, 0, time.UTC)
	sender := &fakeReportNotificationSender{
		result: reportnotification.Result{
			Delivery: domain.ReportNotificationDelivery{
				ID:             32,
				FinalReportID:  12,
				IdempotencyKey: "final_report:12/notification/handoff",
				Status:         domain.ReportNotificationDeliveryStatusPending,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/reports/12/notification/retry", nil)
	addTestLocalRBACAuthorization(req)
	testReportNotificationRetryHandler(t, &fakeUOWFactory{}, sender, nil).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if sender.req.FinalReportID != 12 ||
		sender.req.ReportNotificationChannelProfileID != 0 ||
		sender.req.NotificationPurpose != "" {
		t.Fatalf("sender request = %+v", sender.req)
	}
}

func TestRetryReportNotification_ReturnsServiceUnavailableWhenUnconfigured(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/reports/11/notification/retry", nil)
	testHandler(&fakeUOWFactory{}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestRetryReportNotification_RejectsInvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"unexpected":true}`},
		{name: "invalid channel id", body: `{"report_notification_channel_profile_id":0}`},
		{name: "invalid notification purpose", body: `{"notification_purpose":"closed"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(
				context.Background(),
				stdhttp.MethodPost,
				"/api/v1/reports/11/notification/retry",
				strings.NewReader(tc.body),
			)
			addTestLocalRBACAuthorization(req)
			testReportNotificationRetryHandler(t, &fakeUOWFactory{}, &fakeReportNotificationSender{}, nil).ServeHTTP(rec, req)

			if rec.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRetryReportNotification_MapsSenderInvariantToBadRequest(t *testing.T) {
	sender := &fakeReportNotificationSender{
		err: fmt.Errorf("report notification: im provider is not configured: %w", domain.ErrInvariantViolation),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/reports/11/notification/retry", nil)
	addTestLocalRBACAuthorization(req)
	testReportNotificationRetryHandler(t, &fakeUOWFactory{}, sender, nil).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestRetryReportNotification_MapsSenderNotFoundToNotFound(t *testing.T) {
	sender := &fakeReportNotificationSender{
		err: fmt.Errorf("report notification: final report 11 not found: %w", domain.ErrNotFound),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/reports/11/notification/retry", nil)
	addTestLocalRBACAuthorization(req)
	testReportNotificationRetryHandler(t, &fakeUOWFactory{}, sender, nil).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestRetryReportNotification_RejectsUnauthenticatedPrincipal(t *testing.T) {
	sender := &fakeReportNotificationSender{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/reports/11/notification/retry", nil)
	testReportNotificationRetryHandler(t, &fakeUOWFactory{}, sender, nil).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if sender.called != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.called)
	}
}

func testReportNotificationRetryHandler(
	t *testing.T,
	factory *fakeUOWFactory,
	sender *fakeReportNotificationSender,
	authorizer *fakeRBACAuthorizer,
) stdhttp.Handler {
	t.Helper()
	if factory.configRepo == nil {
		factory.configRepo = &fakeConfigRepo{}
	}
	if authorizer == nil {
		authorizer = allowedReportNotificationRBACAuthorizer()
	}
	opts := testLocalRBACOptions(t, "report-manager-1", authorizer)
	opts = append(opts, WithReportNotificationSender(sender))
	return testHandler(factory, opts...)
}

func allowedReportNotificationRBACAuthorizer() *fakeRBACAuthorizer {
	return &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   true,
			CheckedAt: time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC),
		},
	}
}

func TestRetryDiagnosisRoomNotification_AuthenticatesAndReturnsNotificationProof(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}},
		},
	})
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: time.Date(2026, 6, 21, 11, 29, 0, 0, time.UTC)},
	}
	now := time.Date(2026, 6, 21, 11, 30, 0, 0, time.UTC)
	retrier := &fakeDiagnosisNotificationRetrier{
		result: diagnosisnotification.Result{
			RetryState: diagnosisnotification.RetryStateSent,
			Event: domain.DiagnosisTaskEvent{
				ID:     51,
				TaskID: 31,
				Kind:   diagnosisnotification.EventFinalReadyNotification,
				Payload: json.RawMessage(`{
					"kind":"diagnosis_room.final_ready_notification_sent",
					"session_id":"session-1",
					"diagnosis_task_id":31,
					"notification_channel_profile_id":2,
					"provider_status":"delivered",
					"provider_message_id":"wecom-retry-1",
					"assistant_message_id":"msg-1/assistant",
					"assistant_turn_id":32,
					"assistant_sequence":2,
					"turn_count":1,
					"final_conclusion":{
						"content":"Hydrated final conclusion returned by retry.",
						"confidence":"high",
						"requires_human_review":true
					}
				}`),
				OccurredAt: now,
				RecordedAt: now.Add(time.Second),
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/rooms/session-1/notifications/retry",
		strings.NewReader(`{"event_kind":"diagnosis_room.final_ready_notification_sent"}`),
	)
	req.Header.Set("Authorization", "Bearer oidc-token")
	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("R", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithRBACAuthorizer(authorizer),
		WithDiagnosisRoomNotificationRetrier(retrier),
		withDiagnosisClock(func() time.Time { return now }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.Calls("Bearer oidc-token") != 1 {
		t.Fatalf("auth calls = %d, want 1", authProvider.Calls("Bearer oidc-token"))
	}
	if authorizer.called != 1 ||
		authorizer.req.Permission != domain.RBACPermissionDiagnosisRoomAdminister ||
		authorizer.req.ScopeKind != domain.RBACScopeKindDiagnosisRoom ||
		authorizer.req.ScopeKey != "session-1" {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if retrier.called != 1 ||
		retrier.req.SessionID != "session-1" ||
		retrier.req.EventKind != diagnosisnotification.EventFinalReadyNotification ||
		retrier.req.Principal.Subject != "owner-1" ||
		!retrier.req.OccurredAt.Equal(now) {
		t.Fatalf("retrier called=%d req=%+v", retrier.called, retrier.req)
	}
	var resp api.DiagnosisNotificationRetryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RetryState != api.DiagnosisNotificationRetryStateSent ||
		resp.Notification.EventKind != diagnosisnotification.EventFinalReadyNotification ||
		resp.Notification.NotificationChannelProfileID == nil ||
		*resp.Notification.NotificationChannelProfileID != 2 ||
		resp.Notification.ProviderStatus != "delivered" ||
		resp.Notification.ProviderMessageID == nil ||
		*resp.Notification.ProviderMessageID != "wecom-retry-1" ||
		resp.Notification.Confidence == nil ||
		*resp.Notification.Confidence != api.ReportConfidenceHigh ||
		resp.Notification.RequiresHumanReview == nil ||
		!*resp.Notification.RequiresHumanReview ||
		resp.Notification.ContentKind == nil ||
		*resp.Notification.ContentKind != "final_conclusion" ||
		resp.Notification.ContentSha256 == nil ||
		*resp.Notification.ContentSha256 != testSHA256("Hydrated final conclusion returned by retry.") ||
		!resp.Notification.OccurredAt.Equal(now) {
		t.Fatalf("retry response = %+v", resp)
	}
}

func TestRetryDiagnosisRoomNotification_ReturnsServiceUnavailableWhenUnconfigured(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/rooms/session-1/notifications/retry",
		strings.NewReader(`{"event_kind":"diagnosis_room.final_ready_notification_sent"}`),
	)
	testHandler(&fakeUOWFactory{}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestRetryDiagnosisRoomNotification_RejectsMissingAuthorization(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/rooms/session-1/notifications/retry",
		strings.NewReader(`{"event_kind":"diagnosis_room.final_ready_notification_sent"}`),
	)
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("S", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithDiagnosisRoomNotificationRetrier(&fakeDiagnosisNotificationRetrier{}),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func reportNotificationRetryReadyFactory(
	confirmedBy string,
	readyAt time.Time,
) *fakeUOWFactory {
	reportID := domain.FinalReportID(11)
	snapshotID := domain.EvidenceSnapshotID(7)
	taskID := domain.DiagnosisTaskID(31)
	return &fakeUOWFactory{
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{ID: reportID},
			},
			linkedSubReports: map[domain.FinalReportID][]domain.SubReport{
				reportID: {
					{ID: 21, EvidenceSnapshotID: snapshotID},
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			tasksBySnapshot: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				snapshotID: {
					{ID: taskID, EvidenceSnapshotID: snapshotID},
				},
			},
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				taskID: {
					diagnosisConclusionEventFinalReady: {
						reportNotificationRetryConclusionEvent(taskID, snapshotID, confirmedBy, readyAt, readyAt.Add(time.Second)),
					},
				},
			},
		},
	}
}

func reportNotificationRetryConclusionEvent(
	taskID domain.DiagnosisTaskID,
	snapshotID domain.EvidenceSnapshotID,
	confirmedBy string,
	occurredAt time.Time,
	recordedAt time.Time,
) domain.DiagnosisTaskEvent {
	return domain.DiagnosisTaskEvent{
		ID:         41,
		TaskID:     taskID,
		Kind:       diagnosisConclusionEventFinalReady,
		OccurredAt: occurredAt,
		RecordedAt: recordedAt,
		Payload: json.RawMessage(fmt.Sprintf(`{
			"kind":"diagnosis_room.final_conclusion_ready",
			"session_id":"diagnosis-session-%d",
			"chat_session_id":51,
			"diagnosis_task_id":%d,
			"final_conclusion":{
				"status":"available",
				"source":"latest_assistant_turn",
				"evidence_snapshot_id":%d,
				"confirmed_by":%q,
				"content":"The incident conclusion has been confirmed by the operator."
			}
		}`, taskID, taskID, snapshotID, confirmedBy)),
	}
}

func reportNotificationRetryProgressEvent(
	taskID domain.DiagnosisTaskID,
	occurredAt time.Time,
	recordedAt time.Time,
) domain.DiagnosisTaskEvent {
	return domain.DiagnosisTaskEvent{
		ID:         42,
		TaskID:     taskID,
		Kind:       diagnosisConclusionEventTurnPersisted,
		OccurredAt: occurredAt,
		RecordedAt: recordedAt,
		Payload: json.RawMessage(fmt.Sprintf(`{
			"kind":"diagnosis_room.turn_persisted",
			"session_id":"diagnosis-session-%d",
			"chat_session_id":51,
			"diagnosis_task_id":%d,
			"confidence":"medium"
		}`, taskID, taskID)),
	}
}

func TestIngestAlertmanagerWebhook_AcceptsPayload(t *testing.T) {
	ingestor := &fakeAlertmanagerWebhookIngestor{
		result: alertmanagerwebhook.Result{
			ProfileID:         7,
			Received:          5,
			Resolved:          1,
			SkippedResolved:   1,
			SkippedSuppressed: 2,
			TruncatedAlerts:   0,
			Ingested:          alertingest.Stats{Total: 1, Saved: 1},
			AutoDiagnosis: &alertdiagnosis.Result{
				PoliciesMatched: 1,
				Snapshots: []alertreplay.SnapshotRef{
					{ID: 17, GroupIndex: 0, EventCount: 1},
					{ID: 18, GroupIndex: 1, EventCount: 1},
				},
				SkippedSnapshots: []alertreplay.SnapshotRef{{ID: 18, GroupIndex: 1, EventCount: 1}},
				Rooms: []alertdiagnosis.RoomStart{{
					PolicyID:           3,
					EvidenceSnapshotID: 17,
					SessionID:          "diagnosis-session-auto-p3-s17",
					InitialMessageID:   "diagnosis-auto-initial-p3-s17",
					Workflow:           ports.WorkflowHandle{WorkflowID: "diagnosis-room-diagnosis-session-auto-p3-s17", RunID: "run-diagnosis-17"},
				}},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/alert-sources/7/webhooks/alertmanager",
		strings.NewReader(`{"version":"4","status":"firing","alerts":[]}`),
	)
	req.Header.Set("Authorization", "Bearer webhook-token")
	testHandler(&fakeUOWFactory{}, WithAlertmanagerWebhookIngestor(ingestor)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if ingestor.called != 1 {
		t.Fatalf("ingestor calls = %d, want 1", ingestor.called)
	}
	if ingestor.req.ProfileID != 7 || ingestor.req.Authorization != "Bearer webhook-token" {
		t.Fatalf("ingestor request = %+v", ingestor.req)
	}
	if string(ingestor.req.Body) != `{"version":"4","status":"firing","alerts":[]}` {
		t.Fatalf("ingestor body = %s", ingestor.req.Body)
	}

	var body api.AlertmanagerWebhookIngestResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.SourceID != 7 || body.Received != 5 || body.Resolved != 1 || body.SkippedResolved != 1 || body.SkippedSuppressed != 2 || body.Ingested.Saved != 1 {
		t.Fatalf("response = %+v", body)
	}
	if body.AutoDiagnosis == nil ||
		body.AutoDiagnosis.PoliciesMatched != 1 ||
		body.AutoDiagnosis.Snapshots != 2 ||
		body.AutoDiagnosis.RoomsStarted != 1 ||
		body.AutoDiagnosis.RoomsSkipped != 1 ||
		len(body.AutoDiagnosis.SkippedSnapshotIds) != 1 ||
		body.AutoDiagnosis.SkippedSnapshotIds[0] != 18 {
		t.Fatalf("auto diagnosis response = %+v", body.AutoDiagnosis)
	}
	if len(body.AutoDiagnosis.Rooms) != 1 ||
		body.AutoDiagnosis.Rooms[0].PolicyID != 3 ||
		body.AutoDiagnosis.Rooms[0].EvidenceSnapshotID != 17 ||
		body.AutoDiagnosis.Rooms[0].SessionID != "diagnosis-session-auto-p3-s17" ||
		body.AutoDiagnosis.Rooms[0].InitialMessageID != "diagnosis-auto-initial-p3-s17" ||
		body.AutoDiagnosis.Rooms[0].WorkflowID != "diagnosis-room-diagnosis-session-auto-p3-s17" ||
		body.AutoDiagnosis.Rooms[0].RunID != "run-diagnosis-17" {
		t.Fatalf("auto diagnosis rooms = %+v", body.AutoDiagnosis.Rooms)
	}
}

func TestIngestAlertmanagerWebhookRejectsUnconfiguredIngestor(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/alert-sources/7/webhooks/alertmanager",
		strings.NewReader(`{"version":"4","status":"firing","alerts":[]}`),
	)
	testHandler(&fakeUOWFactory{}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestIngestAlertmanagerWebhookMapsUsecaseErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "unauthorized", err: alertmanagerwebhook.ErrUnauthorized, wantStatus: stdhttp.StatusUnauthorized},
		{name: "not_found", err: domain.ErrNotFound, wantStatus: stdhttp.StatusNotFound},
		{name: "invariant", err: domain.ErrInvariantViolation, wantStatus: stdhttp.StatusBadRequest},
		{name: "secret_unavailable", err: alertmanagerwebhook.ErrSecretResolverUnavailable, wantStatus: stdhttp.StatusServiceUnavailable},
		{name: "unexpected", err: errors.New("database unavailable"), wantStatus: stdhttp.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ingestor := &fakeAlertmanagerWebhookIngestor{err: tc.err}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(
				context.Background(),
				stdhttp.MethodPost,
				"/api/v1/alert-sources/7/webhooks/alertmanager",
				strings.NewReader(`{"version":"4","status":"firing","alerts":[]}`),
			)
			testHandler(&fakeUOWFactory{}, WithAlertmanagerWebhookIngestor(ingestor)).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestIngestAlertmanagerWebhookRejectsInvalidTransportRequest(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "invalid_source_id",
			path: "/api/v1/alert-sources/0/webhooks/alertmanager",
			body: `{"version":"4","status":"firing","alerts":[]}`,
		},
		{
			name: "overlarge_body",
			path: "/api/v1/alert-sources/7/webhooks/alertmanager",
			body: strings.Repeat("{", maxJSONRequestBodyBytes+1),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ingestor := &fakeAlertmanagerWebhookIngestor{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, tc.path, strings.NewReader(tc.body))
			testHandler(&fakeUOWFactory{}, WithAlertmanagerWebhookIngestor(ingestor)).ServeHTTP(rec, req)

			if rec.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if ingestor.called != 0 {
				t.Fatalf("ingestor calls = %d, want 0", ingestor.called)
			}
		})
	}
}

func TestTriggerReportReplayRejectsUnconfiguredTrigger(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/report-triggers/replay-window", strings.NewReader(`{}`))
	testHandler(&fakeUOWFactory{}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestTriggerReportReplayRejectsUnauthenticatedPrincipal(t *testing.T) {
	trigger := &fakeReportReplayTrigger{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/report-triggers/replay-window",
		strings.NewReader(`{"window_start":"2026-05-27T09:00:00Z","window_end":"2026-05-27T10:00:00Z","limit":1}`),
	)
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true},
	}
	opts := testLocalRBACOptions(t, "report-manager-1", authorizer)
	opts = append(opts, WithReportReplayTrigger(trigger))
	testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, opts...).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer calls = %d, want 0", authorizer.called)
	}
	if trigger.called != 0 {
		t.Fatalf("trigger calls = %d, want 0", trigger.called)
	}
}

func TestTriggerReportReplayRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "limit_out_of_range",
			body: `{"window_start":"2026-05-27T09:00:00Z","window_end":"2026-05-27T10:00:00Z","limit":0}`,
		},
		{
			name: "alert_event_id_out_of_range",
			body: `{"window_start":"2026-05-27T09:00:00Z","window_end":"2026-05-27T10:00:00Z","alert_event_id":0}`,
		},
		{
			name: "unknown_field",
			body: `{"window_start":"2026-05-27T09:00:00Z","window_end":"2026-05-27T10:00:00Z","extra":true}`,
		},
		{
			name: "duplicate_key",
			body: `{"window_start":"2026-05-27T09:00:00Z","window_end":"2026-05-27T10:00:00Z","limit":1,"limit":2}`,
		},
		{
			name: "overlarge_body",
			body: strings.Repeat("{", maxJSONRequestBodyBytes+1),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trigger := &fakeReportReplayTrigger{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/report-triggers/replay-window", strings.NewReader(tc.body))
			addTestLocalRBACAuthorization(req)
			authorizer := &fakeRBACAuthorizer{
				result: rbacusecase.AuthorizeDecision{Allowed: true},
			}
			opts := testLocalRBACOptions(t, "report-manager-1", authorizer)
			opts = append(opts, WithReportReplayTrigger(trigger))
			testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, opts...).ServeHTTP(rec, req)

			if rec.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if trigger.called != 0 {
				t.Fatalf("trigger calls = %d, want 0", trigger.called)
			}
		})
	}
}

func TestTriggerReportWorkflowPolicyReplay_StartsReportWorkflow(t *testing.T) {
	windowStart := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	trigger := &fakeReportWorkflowPolicyReplayTrigger{
		result: reporttrigger.Result{
			CorrelationKey: "policy-window-1",
			Replay: alertreplay.Result{
				Stats: alertreplay.Stats{
					Ingested:       alertingest.Stats{Total: 1, Saved: 1},
					EventsLoaded:   1,
					GroupsBuilt:    1,
					GroupsSaved:    1,
					SnapshotsSaved: 1,
					GroupsClosed:   1,
				},
				Snapshots: []alertreplay.SnapshotRef{
					{ID: 9, GroupIndex: 0, EventCount: 1},
				},
			},
			Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-policy-1", RunID: "run-policy-1"},
			Started:  true,
		},
	}

	body := `{
		"window_start":"2026-06-05T08:00:00Z",
		"window_end":"2026-06-05T09:00:00Z",
		"limit":5,
		"correlation_key":"policy-window-1",
		"workflow_id":"report-batch-policy-1"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/replay-window", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{}, WithReportWorkflowPolicyReplayTrigger(trigger)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if trigger.called != 1 {
		t.Fatalf("trigger calls = %d, want 1", trigger.called)
	}
	if trigger.req.PolicyID != 7 ||
		!trigger.req.WindowStart.Equal(windowStart) ||
		!trigger.req.WindowEnd.Equal(windowEnd) ||
		trigger.req.Limit != 5 ||
		trigger.req.CorrelationKey != "policy-window-1" ||
		trigger.req.WorkflowID != "report-batch-policy-1" {
		t.Fatalf("trigger request = %+v", trigger.req)
	}

	var resp api.ReportReplayTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Started || resp.CorrelationKey != "policy-window-1" || resp.WorkflowID != "report-batch-policy-1" || resp.RunID != "run-policy-1" {
		t.Fatalf("response workflow = %+v", resp)
	}
	if len(resp.Snapshots) != 1 || resp.Snapshots[0].ID != 9 {
		t.Fatalf("response snapshots = %+v", resp.Snapshots)
	}
}

func TestTriggerReportWorkflowPolicyReplayIncludesAutoDiagnosisSummary(t *testing.T) {
	trigger := &fakeDetailedReportWorkflowPolicyReplayTrigger{
		result: reportpolicytrigger.Result{
			Trigger: reporttrigger.Result{
				CorrelationKey: "report-workflow-policy:7:manual-replay",
				Replay: alertreplay.Result{
					Stats: alertreplay.Stats{
						Ingested:       alertingest.Stats{Total: 1, Saved: 1},
						EventsLoaded:   1,
						GroupsBuilt:    1,
						GroupsSaved:    1,
						SnapshotsSaved: 1,
						GroupsClosed:   1,
					},
					Snapshots: []alertreplay.SnapshotRef{
						{ID: 17, GroupIndex: 0, EventCount: 1},
					},
				},
				Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-policy-auto", RunID: "run-policy-auto"},
				Started:  true,
			},
			AutoDiagnosis: &alertdiagnosis.Result{
				PoliciesMatched: 1,
				Snapshots:       []alertreplay.SnapshotRef{{ID: 17, GroupIndex: 0, EventCount: 1}},
				Rooms: []alertdiagnosis.RoomStart{{
					PolicyID:           7,
					EvidenceSnapshotID: 17,
					SessionID:          "diagnosis-session-auto-p7-s17",
					InitialMessageID:   "diagnosis-auto-initial-p7-s17",
					Workflow:           ports.WorkflowHandle{WorkflowID: "diagnosis-room-diagnosis-session-auto-p7-s17", RunID: "run-diagnosis-17"},
				}},
			},
		},
	}

	body := `{
		"window_start":"2026-06-05T08:00:00Z",
		"window_end":"2026-06-05T09:00:00Z",
		"limit":5
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/replay-window", strings.NewReader(body))
	testConfigHandler(t, &fakeUOWFactory{}, WithReportWorkflowPolicyReplayTrigger(trigger)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if trigger.detailedCalled != 1 || trigger.legacyCalled != 0 {
		t.Fatalf("trigger calls detailed=%d legacy=%d, want detailed only", trigger.detailedCalled, trigger.legacyCalled)
	}
	var resp api.ReportReplayTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AutoDiagnosis == nil ||
		resp.AutoDiagnosis.PoliciesMatched != 1 ||
		resp.AutoDiagnosis.Snapshots != 1 ||
		resp.AutoDiagnosis.RoomsStarted != 1 ||
		resp.AutoDiagnosis.RoomsSkipped != 0 ||
		len(resp.AutoDiagnosis.SkippedSnapshotIds) != 0 {
		t.Fatalf("auto diagnosis response = %+v", resp.AutoDiagnosis)
	}
	if len(resp.AutoDiagnosis.Rooms) != 1 ||
		resp.AutoDiagnosis.Rooms[0].PolicyID != 7 ||
		resp.AutoDiagnosis.Rooms[0].EvidenceSnapshotID != 17 ||
		resp.AutoDiagnosis.Rooms[0].SessionID != "diagnosis-session-auto-p7-s17" ||
		resp.AutoDiagnosis.Rooms[0].InitialMessageID != "diagnosis-auto-initial-p7-s17" ||
		resp.AutoDiagnosis.Rooms[0].WorkflowID != "diagnosis-room-diagnosis-session-auto-p7-s17" ||
		resp.AutoDiagnosis.Rooms[0].RunID != "run-diagnosis-17" {
		t.Fatalf("auto diagnosis rooms = %+v", resp.AutoDiagnosis.Rooms)
	}
}

func TestTriggerReportWorkflowPolicyReplayRejectsUnconfiguredTrigger(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/replay-window", strings.NewReader(`{}`))
	testConfigHandler(t, &fakeUOWFactory{}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestTriggerReportWorkflowPolicyReplayRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "limit_out_of_range",
			path: "/api/v1/config/report-workflow-policies/7/replay-window",
			body: `{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z","limit":0}`,
		},
		{
			name: "unknown_field",
			path: "/api/v1/config/report-workflow-policies/7/replay-window",
			body: `{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z","extra":true}`,
		},
		{
			name: "duplicate_key",
			path: "/api/v1/config/report-workflow-policies/7/replay-window",
			body: `{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z","limit":1,"limit":2}`,
		},
		{
			name: "invalid_policy_id",
			path: "/api/v1/config/report-workflow-policies/0/replay-window",
			body: `{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trigger := &fakeReportWorkflowPolicyReplayTrigger{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, tc.path, strings.NewReader(tc.body))
			testConfigHandler(t, &fakeUOWFactory{}, WithReportWorkflowPolicyReplayTrigger(trigger)).ServeHTTP(rec, req)

			if rec.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if trigger.called != 0 {
				t.Fatalf("trigger calls = %d, want 0", trigger.called)
			}
		})
	}
}

func TestTriggerReportWorkflowPolicyReplayMapsUsecaseErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "not_found", err: domain.ErrNotFound, wantStatus: stdhttp.StatusNotFound},
		{name: "binding_not_found", err: errors.Join(errors.New("binding missing"), domain.ErrNotFound), wantStatus: stdhttp.StatusNotFound},
		{name: "invariant", err: domain.ErrInvariantViolation, wantStatus: stdhttp.StatusBadRequest},
		{name: "unexpected", err: errors.New("temporal unavailable"), wantStatus: stdhttp.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trigger := &fakeReportWorkflowPolicyReplayTrigger{err: tc.err}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(
				context.Background(),
				stdhttp.MethodPost,
				"/api/v1/config/report-workflow-policies/7/replay-window",
				strings.NewReader(`{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z"}`),
			)
			testConfigHandler(t, &fakeUOWFactory{}, WithReportWorkflowPolicyReplayTrigger(trigger)).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestCheckDiagnosisAuthAuthenticatesAndReturnsSanitizedPrincipal(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk": {
			{Principal: ports.AuthPrincipal{
				Subject: "operator-1",
				Roles: []ports.AuthRole{
					ports.AuthRoleOwner,
					ports.AuthRoleAdmin,
					ports.AuthRole("internal-provider-role"),
				},
				Claims: json.RawMessage(`{"email":"operator@example.com","token":"claim-secret"}`),
			}},
		},
	})
	now := time.Date(2026, 6, 21, 4, 0, 0, 0, time.UTC)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/auth/check", nil)
	req.Header.Set("Authorization", "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk")
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("A", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}, "ldap"),
		withDiagnosisClock(func() time.Time { return now }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if calls := authProvider.Calls("Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk"); calls != 1 {
		t.Fatalf("auth calls = %d, want 1", calls)
	}
	if strings.Contains(rec.Body.String(), "claim-secret") || strings.Contains(rec.Body.String(), "b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk") {
		t.Fatalf("response leaked credentials or claims: %s", rec.Body.String())
	}

	var body api.DiagnosisAuthCheckResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Subject != "operator-1" {
		t.Fatalf("subject = %q, want operator-1", body.Subject)
	}
	if !slices.Equal(body.Roles, []string{"owner", "admin"}) {
		t.Fatalf("roles = %#v, want owner/admin", body.Roles)
	}
	if body.Mode != string(api.DiagnosisAuthCheckResponseModeLdap) {
		t.Fatalf("mode = %q, want ldap", body.Mode)
	}
	if !body.CheckedAt.Equal(now) {
		t.Fatalf("checked_at = %s, want %s", body.CheckedAt, now)
	}
	if !body.RoleAuthorized {
		t.Fatalf("role_authorized = false, want true")
	}
}

func TestCheckDiagnosisAuthIgnoresUnsupportedModeFromProviderClaims(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer session-token": {
			{Principal: ports.AuthPrincipal{
				Subject: "operator-1",
				Roles:   []ports.AuthRole{ports.AuthRoleOwner},
				Claims:  json.RawMessage(`{"auth_provider":"wecom","userid":"operator-1"}`),
			}},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/auth/check", nil)
	req.Header.Set("Authorization", "Bearer session-token")
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("A", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}, "ldap"),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.DiagnosisAuthCheckResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Mode != string(api.DiagnosisAuthCheckResponseModeLdap) {
		t.Fatalf("mode = %q, want ldap fallback", body.Mode)
	}
	if strings.Contains(rec.Body.String(), "session-token") || strings.Contains(rec.Body.String(), "userid") {
		t.Fatalf("response leaked credentials or claims: %s", rec.Body.String())
	}
}

func TestCheckDiagnosisAuthAcceptsBrowserSessionToken(t *testing.T) {
	now := time.Date(2026, 6, 27, 9, 30, 0, 0, time.UTC)
	sessionIssuer := newHTTPTestDiagnosisSessionIssuer(t, now)
	sessionToken, err := sessionIssuer.IssueToken(context.Background(), ports.AuthPrincipal{
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
		Claims:  json.RawMessage(`{"auth_provider":"oidc"}`),
	}, "oidc")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/auth/check", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken.Token)
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuthSessionIssuer(sessionIssuer),
		withDiagnosisClock(func() time.Time { return now }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), sessionToken.Token) ||
		strings.Contains(rec.Body.String(), "auth_provider") {
		t.Fatalf("response leaked session token or claims: %s", rec.Body.String())
	}
	var body api.DiagnosisAuthCheckResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Subject != "operator-1" ||
		body.Mode != string(api.DiagnosisAuthCheckResponseModeOidc) ||
		!body.RoleAuthorized ||
		!slices.Equal(body.Roles, []string{"admin"}) ||
		!body.CheckedAt.Equal(now) {
		t.Fatalf("check response = %+v", body)
	}
}

func TestIssueDiagnosisAuthSessionExchangesLDAPCredentialsForSessionToken(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk": {
			{Principal: ports.AuthPrincipal{
				Subject: "operator-1",
				Roles: []ports.AuthRole{
					ports.AuthRoleOwner,
					ports.AuthRoleAdmin,
					ports.AuthRole("internal-provider-role"),
				},
				Claims: json.RawMessage(`{"email":"operator@example.com","token":"claim-secret"}`),
			}},
		},
	})
	now := time.Date(2026, 6, 21, 5, 0, 0, 0, time.UTC)
	sessionIssuer := newHTTPTestDiagnosisSessionIssuer(t, now)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/auth/session", nil)
	req.Header.Set("Authorization", "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk")
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("A", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}, "ldap"),
		WithDiagnosisAuthSessionIssuer(sessionIssuer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if calls := authProvider.Calls("Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk"); calls != 1 {
		t.Fatalf("auth calls = %d, want 1", calls)
	}
	if strings.Contains(rec.Body.String(), "claim-secret") ||
		strings.Contains(rec.Body.String(), "b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk") ||
		strings.Contains(rec.Body.String(), "ldap-password") {
		t.Fatalf("response leaked credentials or claims: %s", rec.Body.String())
	}

	var body api.DiagnosisAuthSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Token == "" ||
		body.Subject != "operator-1" ||
		body.Mode != string(api.DiagnosisAuthSessionResponseModeLdap) ||
		!body.CheckedAt.Equal(now) ||
		!body.ExpiresAt.Equal(now.Add(diagnosisauth.DefaultSessionTTL)) ||
		!body.RoleAuthorized ||
		!slices.Equal(body.Roles, []string{"owner", "admin"}) {
		t.Fatalf("session response = %+v", body)
	}
	principal, err := sessionIssuer.AuthenticateAuthorization(context.Background(), "Bearer "+body.Token)
	if err != nil {
		t.Fatalf("session token authenticate: %v", err)
	}
	if principal.Subject != "operator-1" ||
		!slices.Equal(principal.Roles, []ports.AuthRole{ports.AuthRoleOwner, ports.AuthRoleAdmin}) ||
		!strings.Contains(string(principal.Claims), `"auth_provider":"ldap"`) ||
		strings.Contains(string(principal.Claims), "claim-secret") {
		t.Fatalf("session principal = %+v claims=%s", principal, string(principal.Claims))
	}
}

func TestIssueDiagnosisAuthSessionPassesOIDCAccessTokenAuxiliaryCredential(t *testing.T) {
	authProvider := &auxiliaryCredentialAuthProvider{
		principal: ports.AuthPrincipal{
			Subject: "operator-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
	}
	now := time.Date(2026, 6, 21, 5, 0, 0, 0, time.UTC)
	sessionIssuer := newHTTPTestDiagnosisSessionIssuer(t, now)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/auth/session", nil)
	req.Header.Set("Authorization", "Bearer id-token-1")
	req.Header.Set("X-OpenClarion-OIDC-Access-Token", "access-token-1")
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("A", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}, "oidc"),
		WithDiagnosisAuthSessionIssuer(sessionIssuer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.calls != 1 || authProvider.authorization != "Bearer id-token-1" ||
		authProvider.credentials.OIDCAccessToken != "access-token-1" {
		t.Fatalf("auth provider calls=%d authorization=%q credentials=%#v", authProvider.calls, authProvider.authorization, authProvider.credentials)
	}
	if strings.Contains(rec.Body.String(), "id-token-1") ||
		strings.Contains(rec.Body.String(), "access-token-1") {
		t.Fatalf("response leaked OIDC tokens: %s", rec.Body.String())
	}
}

func TestIssueDiagnosisAuthSessionAllowsIdentityWithoutProviderRoleMapping(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk": {
			{Principal: ports.AuthPrincipal{
				Subject: "operator-1",
				Roles:   []ports.AuthRole{ports.AuthRole("viewer")},
			}},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/auth/session", nil)
	req.Header.Set("Authorization", "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk")
	sessionIssuer := newHTTPTestDiagnosisSessionIssuer(t, time.Date(2026, 6, 21, 5, 0, 0, 0, time.UTC))
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("A", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}, "ldap"),
		WithDiagnosisAuthSessionIssuer(sessionIssuer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk") ||
		strings.Contains(rec.Body.String(), "ldap-password") {
		t.Fatalf("response leaked credentials: %s", rec.Body.String())
	}
	var body api.DiagnosisAuthSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Token == "" || body.Subject != "operator-1" || body.RoleAuthorized || len(body.Roles) != 0 {
		t.Fatalf("session response = %+v, want identity session without provider roles", body)
	}
	principal, err := sessionIssuer.AuthenticateAuthorization(context.Background(), "Bearer "+body.Token)
	if err != nil {
		t.Fatalf("session token authenticate: %v", err)
	}
	if principal.Subject != "operator-1" || len(principal.Roles) != 0 ||
		!strings.Contains(string(principal.Claims), `"auth_provider":"ldap"`) {
		t.Fatalf("session principal = %+v claims=%s", principal, string(principal.Claims))
	}
}

func TestIssueDiagnosisAuthSessionRejectsUnconfiguredSessionIssuer(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk": {
			{Principal: ports.AuthPrincipal{
				Subject: "operator-1",
				Roles:   []ports.AuthRole{ports.AuthRoleOwner},
			}},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/auth/session", nil)
	req.Header.Set("Authorization", "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk")
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("A", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}, "ldap"),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.Calls("Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk") != 0 {
		t.Fatalf("auth provider was called before session issuer readiness check")
	}
}

func TestGetDiagnosisAuthStatusReturnsNonSensitiveProviderMode(t *testing.T) {
	tests := []struct {
		name          string
		providerName  string
		configure     bool
		wantMode      string
		wantReady     bool
		wantSupported []api.DiagnosisAuthStatusResponseSupportedModesItem
	}{
		{
			name:          "not configured",
			wantMode:      string(api.DiagnosisAuthStatusResponseModeNone),
			wantReady:     false,
			wantSupported: nil,
		},
		{
			name:          "ldap",
			providerName:  "ldap",
			configure:     true,
			wantMode:      string(api.DiagnosisAuthStatusResponseModeLdap),
			wantReady:     true,
			wantSupported: []api.DiagnosisAuthStatusResponseSupportedModesItem{api.DiagnosisAuthStatusResponseSupportedModesItemLdap},
		},
		{
			name:          "static",
			providerName:  "static",
			configure:     true,
			wantMode:      string(api.DiagnosisAuthStatusResponseModeStatic),
			wantReady:     true,
			wantSupported: []api.DiagnosisAuthStatusResponseSupportedModesItem{api.DiagnosisAuthStatusResponseSupportedModesItemStatic},
		},
		{
			name:          "unknown",
			providerName:  "custom-provider",
			configure:     true,
			wantMode:      string(api.DiagnosisAuthStatusResponseModeUnknown),
			wantReady:     true,
			wantSupported: []api.DiagnosisAuthStatusResponseSupportedModesItem{api.DiagnosisAuthStatusResponseSupportedModesItemUnknown},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authProvider := authfake.New(map[string][]authfake.Result{
				"Bearer secret-token": {
					{Principal: ports.AuthPrincipal{Subject: "operator-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}},
				},
			})
			opts := []ServerOption{}
			if tc.configure {
				opts = append(opts, WithDiagnosisAuth(
					authProvider,
					newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("A", diagnosisauth.DefaultTokenBytes))),
					&fakeDiagnosisSessionResolver{},
					tc.providerName,
				))
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/auth/status", nil)

			testHandler(&fakeUOWFactory{}, opts...).ServeHTTP(rec, req)

			if rec.Code != stdhttp.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			var body api.DiagnosisAuthStatusResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Configured != tc.wantReady {
				t.Fatalf("configured = %v, want %v", body.Configured, tc.wantReady)
			}
			if body.Mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", body.Mode, tc.wantMode)
			}
			if !slices.Equal(body.SupportedModes, tc.wantSupported) {
				t.Fatalf("supported_modes = %#v, want %#v", body.SupportedModes, tc.wantSupported)
			}
			if strings.Contains(rec.Body.String(), "secret-token") || strings.Contains(rec.Body.String(), "operator-1") {
				t.Fatalf("response leaked auth material: %s", rec.Body.String())
			}
		})
	}
}

func TestGetDiagnosisAuthStatusIncludesRoleMappingAndTransportPolicy(t *testing.T) {
	authProvider := roleMappingStatusAuthProvider{
		status: ports.AuthRoleMappingStatus{
			OwnerMappingCount: 1,
			DefaultRoles:      []ports.AuthRole{ports.AuthRoleAdmin},
		},
		transportStatus: ports.AuthTransportPolicyStatus{
			Security: ports.AuthTransportSecurityStartTLS,
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/auth/status", nil)

	testHandler(&fakeUOWFactory{},
		WithDiagnosisAuth(
			authProvider,
			newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("N", diagnosisauth.DefaultTokenBytes))),
			&fakeDiagnosisSessionResolver{},
			"ldap",
		),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.DiagnosisAuthStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Mode != string(api.DiagnosisAuthStatusResponseModeLdap) {
		t.Fatalf("mode = %q, want ldap", body.Mode)
	}
	if !slices.Equal(body.SupportedModes, []api.DiagnosisAuthStatusResponseSupportedModesItem{
		api.DiagnosisAuthStatusResponseSupportedModesItemLdap,
	}) {
		t.Fatalf("supported_modes = %#v", body.SupportedModes)
	}
	if body.RoleMapping == nil || !body.RoleMapping.Configured {
		t.Fatalf("role_mapping = %+v, want configured summary", body.RoleMapping)
	}
	if body.RoleMapping.OwnerMappingCount != 1 || body.RoleMapping.AdminMappingCount != 0 {
		t.Fatalf("role_mapping counts = %+v", body.RoleMapping)
	}
	if !slices.Equal(body.RoleMapping.DefaultRoles, []api.DiagnosisAuthRoleMappingStatusDefaultRolesItem{
		api.DiagnosisAuthRoleMappingStatusDefaultRolesItemAdmin,
	}) {
		t.Fatalf("default_roles = %#v, want admin", body.RoleMapping.DefaultRoles)
	}
	if body.TransportPolicy == nil ||
		body.TransportPolicy.Security != string(api.StartTLS) {
		t.Fatalf("transport_policy = %+v, want start_tls", body.TransportPolicy)
	}
	for _, leaked := range []string{
		"ldap.example.com",
		"cn=openclarion",
		"service-password",
	} {
		if strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("response leaked transport material %q: %s", leaked, rec.Body.String())
		}
	}
}

func TestGetDiagnosisAuthStatusIncludesNonSensitiveRoleMapping(t *testing.T) {
	authProvider := roleMappingStatusAuthProvider{
		status: ports.AuthRoleMappingStatus{
			OwnerMappingCount: 2,
			AdminMappingCount: 1,
			DefaultRoles:      []ports.AuthRole{ports.AuthRoleOwner},
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/diagnosis/auth/status", nil)

	testHandler(&fakeUOWFactory{}, WithDiagnosisAuth(
		authProvider,
		newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("M", diagnosisauth.DefaultTokenBytes))),
		&fakeDiagnosisSessionResolver{},
		"wecom",
	)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.DiagnosisAuthStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.RoleMapping == nil {
		t.Fatalf("role_mapping = nil, want summary")
	}
	if !body.RoleMapping.Configured {
		t.Fatalf("role_mapping.configured = false, want true")
	}
	if body.RoleMapping.OwnerMappingCount != 2 {
		t.Fatalf("owner_mapping_count = %d, want 2", body.RoleMapping.OwnerMappingCount)
	}
	if body.RoleMapping.AdminMappingCount != 1 {
		t.Fatalf("admin_mapping_count = %d, want 1", body.RoleMapping.AdminMappingCount)
	}
	if !slices.Equal(body.RoleMapping.DefaultRoles, []api.DiagnosisAuthRoleMappingStatusDefaultRolesItem{
		api.DiagnosisAuthRoleMappingStatusDefaultRolesItemOwner,
	}) {
		t.Fatalf("default_roles = %#v, want owner", body.RoleMapping.DefaultRoles)
	}
	for _, leaked := range []string{
		"cn=openclarion-owner",
		"wecom-user-1",
		"secret-token",
	} {
		if strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("response leaked sensitive role mapping material %q: %s", leaked, rec.Body.String())
		}
	}
}

func TestVerifyDiagnosisWeComAppCallbackReturnsEcho(t *testing.T) {
	verifier := &fakeDiagnosisWeComAppCallback{
		echo: "callback-echo-token",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodGet,
		"/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-1&timestamp=1700000000&nonce=nonce-1&echostr=encrypted-echo-1",
		nil,
	)

	testHandler(&fakeUOWFactory{}, WithDiagnosisWeComAppCallback(verifier)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Header().Get("Content-Type")); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	if rec.Body.String() != "callback-echo-token" {
		t.Fatalf("body = %q, want callback echo", rec.Body.String())
	}
	if verifier.echoSignature != "sig-1" ||
		verifier.echoTimestamp != "1700000000" ||
		verifier.echoNonce != "nonce-1" ||
		verifier.echoEncrypted != "encrypted-echo-1" {
		t.Fatalf("verifier echo inputs = %+v", verifier)
	}
}

func TestAcceptDiagnosisWeComAppCallbackAcknowledgesMessage(t *testing.T) {
	verifier := &fakeDiagnosisWeComAppCallback{
		message: wecomcallback.Message{
			FromUserName: "operator-1",
			MsgType:      "text",
			Content:      "sensitive message body",
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-2&timestamp=1700000001&nonce=nonce-2",
		strings.NewReader("<xml><Encrypt>encrypted-body-1</Encrypt></xml>"),
	)

	testHandler(&fakeUOWFactory{}, WithDiagnosisWeComAppCallback(verifier)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "success" {
		t.Fatalf("body = %q, want provider acknowledgement", rec.Body.String())
	}
	if verifier.messageSignature != "sig-2" ||
		verifier.messageTimestamp != "1700000001" ||
		verifier.messageNonce != "nonce-2" ||
		string(verifier.messageRawXML) != "<xml><Encrypt>encrypted-body-1</Encrypt></xml>" {
		t.Fatalf("verifier message inputs = %+v", verifier)
	}
	if strings.Contains(rec.Body.String(), "operator-1") ||
		strings.Contains(rec.Body.String(), "sensitive message body") ||
		strings.Contains(rec.Body.String(), "encrypted-body-1") {
		t.Fatalf("response leaked callback material: %s", rec.Body.String())
	}
}

func TestAcceptDiagnosisWeComAppCallbackRoutesMessage(t *testing.T) {
	messageHandler := &fakeDiagnosisWeComAppCallbackMessageHandler{
		result: diagnosiswecomcallback.Result{
			Status:    diagnosiswecomcallback.StatusSubmitted,
			SessionID: "diagnosis-session-1",
			MessageID: "wecom-app:msg-1",
		},
	}
	verifier := &fakeDiagnosisWeComAppCallback{
		message: wecomcallback.Message{
			FromUserName: "operator-1",
			MsgID:        "wecom-msg-1",
			MsgType:      "text",
			Content:      "diagnosis-session-1 please continue",
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-2&timestamp=1700000001&nonce=nonce-2",
		strings.NewReader("<xml><Encrypt>encrypted-body-1</Encrypt></xml>"),
	)

	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisWeComAppCallback(verifier),
		WithDiagnosisWeComAppCallbackMessageHandler(messageHandler),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "success" {
		t.Fatalf("body = %q, want provider acknowledgement", rec.Body.String())
	}
	if messageHandler.called != 1 ||
		messageHandler.req.Message.FromUserName != "operator-1" ||
		messageHandler.req.Message.Content != "diagnosis-session-1 please continue" {
		t.Fatalf("message handler request = %+v called=%d", messageHandler.req, messageHandler.called)
	}
}

func TestAcceptDiagnosisWeComAppCallbackWorkflowRouterAuthorizesSender(t *testing.T) {
	workflow := &fakeDiagnosisRoomWorkflowClient{}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   true,
			CheckedAt: time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC),
		},
	}
	configRepo := &fakeConfigRepo{
		directoryUsers: []domain.DirectoryUser{{
			Provider:              "ops_iam",
			Subject:               "operator-1",
			ExternalID:            "wecom-operator-1",
			DisplayName:           "Operator One",
			Active:                true,
			DepartmentExternalIDs: []string{"dept-1"},
		}},
	}
	diagnosisRepo := &fakeDiagnosisRepo{
		chatSessions: []domain.ChatSessionWithTask{{
			Session: domain.ChatSession{
				SessionKey:   "diagnosis-session-1",
				OwnerSubject: "owner-1",
			},
		}},
	}
	verifier := &fakeDiagnosisWeComAppCallback{
		message: wecomcallback.Message{
			FromUserName: "wecom-operator-1",
			CreateTime:   1700000001,
			MsgID:        "wecom-msg-1",
			MsgType:      "text",
			Content:      "diagnosis-session-1 please continue",
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-2&timestamp=1700000001&nonce=nonce-2",
		strings.NewReader("<xml><Encrypt>encrypted-body-1</Encrypt></xml>"),
	)

	testHandler(
		&fakeUOWFactory{configRepo: configRepo, diagnosisRepo: diagnosisRepo},
		WithRBACAuthorizer(authorizer),
		WithDiagnosisWeComAppCallback(verifier),
		WithDiagnosisWeComAppCallbackWorkflowRouter(workflow),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authorizer.called != 1 ||
		authorizer.req.Principal.Subject != "operator-1" ||
		!slices.Contains(authorizer.req.Principal.DepartmentKeys, "dept-1") ||
		authorizer.req.Permission != domain.RBACPermissionDiagnosisRoomParticipate ||
		authorizer.req.ScopeKind != domain.RBACScopeKindDiagnosisRoom ||
		authorizer.req.ScopeKey != "diagnosis-session-1" {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	workflow.mu.Lock()
	submitCalled := workflow.submitCalled
	submitReq := workflow.submitReq
	workflow.mu.Unlock()
	if submitCalled != 1 ||
		submitReq.SessionID != "diagnosis-session-1" ||
		submitReq.ActorSubject != "operator-1" ||
		submitReq.Message != "diagnosis-session-1 please continue" {
		t.Fatalf("submit request = %+v called=%d", submitReq, submitCalled)
	}
}

func TestAcceptDiagnosisWeComAppCallbackWorkflowRouterSkipsUnauthorizedSender(t *testing.T) {
	workflow := &fakeDiagnosisRoomWorkflowClient{}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   false,
			CheckedAt: time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC),
		},
	}
	verifier := &fakeDiagnosisWeComAppCallback{
		message: wecomcallback.Message{
			FromUserName: "operator-1",
			CreateTime:   1700000001,
			MsgID:        "wecom-msg-1",
			MsgType:      "text",
			Content:      "diagnosis-session-1 please continue",
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-2&timestamp=1700000001&nonce=nonce-2",
		strings.NewReader("<xml><Encrypt>encrypted-body-1</Encrypt></xml>"),
	)

	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}, diagnosisRepo: &fakeDiagnosisRepo{}},
		WithRBACAuthorizer(authorizer),
		WithDiagnosisWeComAppCallback(verifier),
		WithDiagnosisWeComAppCallbackWorkflowRouter(workflow),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	workflow.mu.Lock()
	submitCalled := workflow.submitCalled
	workflow.mu.Unlock()
	if submitCalled != 0 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 0", submitCalled)
	}
}

func TestDiagnosisWeComAppCallbackRejectsUnconfiguredAndInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		path       string
		body       string
		verifier   *fakeDiagnosisWeComAppCallback
		wantStatus int
		wantBody   string
	}{
		{
			name:       "verify unconfigured",
			method:     stdhttp.MethodGet,
			path:       "/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-1&timestamp=1700000000&nonce=nonce-1&echostr=encrypted-echo-1",
			wantStatus: stdhttp.StatusServiceUnavailable,
			wantBody:   "Enterprise WeChat app callback is not configured",
		},
		{
			name:       "accept unconfigured",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-2&timestamp=1700000001&nonce=nonce-2",
			body:       "<xml><Encrypt>encrypted-body-1</Encrypt></xml>",
			wantStatus: stdhttp.StatusServiceUnavailable,
			wantBody:   "Enterprise WeChat app callback is not configured",
		},
		{
			name:   "verify rejected",
			method: stdhttp.MethodGet,
			path:   "/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-1&timestamp=1700000000&nonce=nonce-1&echostr=encrypted-echo-1",
			verifier: &fakeDiagnosisWeComAppCallback{
				echoErr: fmt.Errorf("secret encrypted-echo-1 failed"),
			},
			wantStatus: stdhttp.StatusBadRequest,
			wantBody:   "Enterprise WeChat app callback verification failed",
		},
		{
			name:   "message rejected",
			method: stdhttp.MethodPost,
			path:   "/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-2&timestamp=1700000001&nonce=nonce-2",
			body:   "<xml><Encrypt>encrypted-body-1</Encrypt></xml>",
			verifier: &fakeDiagnosisWeComAppCallback{
				messageErr: fmt.Errorf("secret encrypted-body-1 failed"),
			},
			wantStatus: stdhttp.StatusBadRequest,
			wantBody:   "Enterprise WeChat app callback message rejected",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			opts := []ServerOption{}
			if tc.verifier != nil {
				opts = append(opts, WithDiagnosisWeComAppCallback(tc.verifier))
			}
			testHandler(&fakeUOWFactory{}, opts...).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tc.wantBody)
			}
			for _, leaked := range []string{"encrypted-echo-1", "encrypted-body-1", "secret"} {
				if strings.Contains(rec.Body.String(), leaked) {
					t.Fatalf("response leaked callback material %q: %s", leaked, rec.Body.String())
				}
			}
		})
	}
}

func TestAcceptDiagnosisWeComAppCallbackPropagatesMessageHandlingFailure(t *testing.T) {
	messageHandler := &fakeDiagnosisWeComAppCallbackMessageHandler{
		err: fmt.Errorf("workflow failed for sensitive message body"),
	}
	verifier := &fakeDiagnosisWeComAppCallback{
		message: wecomcallback.Message{
			FromUserName: "operator-1",
			MsgType:      "text",
			Content:      "sensitive message body",
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/wecom/app-callback?msg_signature=sig-2&timestamp=1700000001&nonce=nonce-2",
		strings.NewReader("<xml><Encrypt>encrypted-body-1</Encrypt></xml>"),
	)

	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisWeComAppCallback(verifier),
		WithDiagnosisWeComAppCallbackMessageHandler(messageHandler),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Enterprise WeChat app callback message handling failed") {
		t.Fatalf("body = %q, want handling failure", rec.Body.String())
	}
	for _, leaked := range []string{"operator-1", "sensitive message body", "encrypted-body-1"} {
		if strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("response leaked callback material %q: %s", leaked, rec.Body.String())
		}
	}
}

func TestCheckDiagnosisAuthRejectsBadInputs(t *testing.T) {
	tests := []struct {
		name          string
		authHeader    string
		principal     ports.AuthPrincipal
		authErr       error
		configureAuth bool
		wantStatus    int
		wantAuthCalls int
	}{
		{
			name:       "auth not configured",
			authHeader: "Bearer token-1",
			wantStatus: stdhttp.StatusServiceUnavailable,
		},
		{
			name:          "missing authorization",
			configureAuth: true,
			wantStatus:    stdhttp.StatusUnauthorized,
		},
		{
			name:          "unsupported authorization scheme",
			authHeader:    "Digest token-1",
			configureAuth: true,
			wantStatus:    stdhttp.StatusUnauthorized,
		},
		{
			name:          "provider rejects credentials",
			authHeader:    "Bearer token-1",
			authErr:       diagnosisauth.ErrUnauthenticated,
			configureAuth: true,
			wantStatus:    stdhttp.StatusUnauthorized,
			wantAuthCalls: 1,
		},
		{
			name:          "provider returns blank subject",
			authHeader:    "Bearer token-1",
			principal:     ports.AuthPrincipal{Subject: " ", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			configureAuth: true,
			wantStatus:    stdhttp.StatusUnauthorized,
			wantAuthCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authProvider := authfake.New(map[string][]authfake.Result{
				"Bearer token-1": {
					{Principal: tc.principal, Err: tc.authErr},
				},
			})
			opts := []ServerOption{}
			if tc.configureAuth {
				opts = append(opts, WithDiagnosisAuth(
					authProvider,
					newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("A", diagnosisauth.DefaultTokenBytes))),
					&fakeDiagnosisSessionResolver{},
				))
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/auth/check", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			testHandler(&fakeUOWFactory{}, opts...).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "token-1") {
				t.Fatalf("response leaked credentials: %s", rec.Body.String())
			}
			if calls := authProvider.Calls("Bearer token-1"); calls != tc.wantAuthCalls {
				t.Fatalf("auth calls = %d, want %d", calls, tc.wantAuthCalls)
			}
		})
	}
}

func TestIssueDiagnosisWSTicketAuthenticatesAndIssuesTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("B", diagnosisauth.DefaultTokenBytes)))
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "responder-1"}},
		},
	})
	configRepo := &fakeConfigRepo{}
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed:   req.Permission == domain.RBACPermissionDiagnosisRoomRead,
				CheckedAt: now,
			}, nil
		},
	}
	resolver := &fakeDiagnosisSessionResolver{
		sessions: map[string]diagnosisauth.SessionRef{
			"session-1": {SessionID: "session-1", OwnerSubject: "owner-1"},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/ws-ticket", strings.NewReader(`{"session_id":"session-1"}`))
	req.Header.Set("Authorization", "Bearer oidc-token")
	testHandler(
		&fakeUOWFactory{configRepo: configRepo},
		WithDiagnosisAuth(authProvider, service, resolver),
		WithRBACAuthorizer(authorizer),
		withDiagnosisClock(func() time.Time { return now }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.Calls("Bearer oidc-token") != 1 {
		t.Fatalf("auth calls = %d, want 1", authProvider.Calls("Bearer oidc-token"))
	}
	if resolver.called != 1 || resolver.sessionID != "session-1" {
		t.Fatalf("resolver called=%d session=%q", resolver.called, resolver.sessionID)
	}
	if authorizer.called != 1 ||
		authorizer.req.Permission != domain.RBACPermissionDiagnosisRoomRead ||
		authorizer.req.ScopeKind != domain.RBACScopeKindDiagnosisRoom ||
		authorizer.req.ScopeKey != "session-1" ||
		authorizer.req.Principal.Subject != "responder-1" {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if configRepo.lastDirectorySubject != "responder-1" {
		t.Fatalf("directory subject = %q, want responder-1", configRepo.lastDirectorySubject)
	}

	var body api.DiagnosisWSTicketResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Ticket == "" || strings.ContainsAny(body.Ticket, "+/=") {
		t.Fatalf("ticket = %q, want URL-safe opaque token", body.Ticket)
	}
	if body.SessionID != "session-1" || !body.ExpiresAt.Equal(now.Add(diagnosisauth.DefaultTicketTTL)) {
		t.Fatalf("response = %+v", body)
	}

	consumed, err := service.ConsumeAuthorizedTicket(context.Background(), body.Ticket, "session-1", now.Add(time.Second))
	if err != nil {
		t.Fatalf("issued ticket should be consumable: %v", err)
	}
	if consumed.Token != "" || consumed.Subject != "responder-1" {
		t.Fatalf("consumed ticket = %+v, want redacted responder ticket", consumed)
	}
}

func TestIssueDiagnosisWSTicketAcceptsBrowserSessionToken(t *testing.T) {
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("Q", diagnosisauth.DefaultTokenBytes)))
	sessionIssuer := newHTTPTestDiagnosisSessionIssuer(t, now)
	sessionToken, err := sessionIssuer.IssueToken(context.Background(), ports.AuthPrincipal{
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, "oidc")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "fallback-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}},
		},
	})
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: now},
	}
	resolver := &fakeDiagnosisSessionResolver{
		sessions: map[string]diagnosisauth.SessionRef{
			"session-1": {SessionID: "session-1", OwnerSubject: "owner-1"},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/ws-ticket", strings.NewReader(`{"session_id":"session-1"}`))
	req.Header.Set("Authorization", "Bearer "+sessionToken.Token)
	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}},
		WithDiagnosisAuth(authProvider, service, resolver, "oidc"),
		WithDiagnosisAuthSessionIssuer(sessionIssuer),
		WithRBACAuthorizer(authorizer),
		withDiagnosisClock(func() time.Time { return now }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.Calls("Bearer oidc-token") != 0 || authProvider.Calls("Bearer "+sessionToken.Token) != 0 {
		t.Fatalf("auth provider should not be called; requests=%v", authProvider.Requests())
	}
	if authorizer.called != 1 || authorizer.req.Principal.Subject != "operator-1" {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}

	var body api.DiagnosisWSTicketResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	consumed, err := service.ConsumeAuthorizedTicket(context.Background(), body.Ticket, "session-1", now.Add(time.Second))
	if err != nil {
		t.Fatalf("issued ticket should be consumable: %v", err)
	}
	if consumed.Subject != "operator-1" {
		t.Fatalf("consumed subject = %q, want operator-1", consumed.Subject)
	}
}

func TestIssueDiagnosisWSTicketRejectsBadInputs(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		authHeader    string
		body          string
		principal     ports.AuthPrincipal
		session       diagnosisauth.SessionRef
		resolverErr   error
		denyRBAC      bool
		wantStatus    int
		wantAuthCalls int
	}{
		{
			name:       "missing bearer",
			body:       `{"session_id":"session-1"}`,
			session:    diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus: stdhttp.StatusUnauthorized,
		},
		{
			name:          "unknown field",
			authHeader:    "Bearer oidc-token",
			body:          `{"session_id":"session-1","ticket":"nope"}`,
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			session:       diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 0,
		},
		{
			name:          "duplicate session id",
			authHeader:    "Bearer oidc-token",
			body:          `{"session_id":"session-1","session_id":"session-2"}`,
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			session:       diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 0,
		},
		{
			name:          "overlarge body",
			authHeader:    "Bearer oidc-token",
			body:          strings.Repeat("{", maxJSONRequestBodyBytes+1),
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			session:       diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 0,
		},
		{
			name:          "local rbac denied",
			authHeader:    "Bearer oidc-token",
			body:          `{"session_id":"session-1"}`,
			principal:     ports.AuthPrincipal{Subject: "responder-1"},
			session:       diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			denyRBAC:      true,
			wantStatus:    stdhttp.StatusForbidden,
			wantAuthCalls: 1,
		},
		{
			name:          "session not found",
			authHeader:    "Bearer oidc-token",
			body:          `{"session_id":"missing"}`,
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			resolverErr:   domain.ErrNotFound,
			wantStatus:    stdhttp.StatusNotFound,
			wantAuthCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("C", diagnosisauth.DefaultTokenBytes)))
			authProvider := authfake.New(map[string][]authfake.Result{
				"Bearer oidc-token": {
					{Principal: tc.principal},
				},
			})
			resolver := &fakeDiagnosisSessionResolver{
				sessions: map[string]diagnosisauth.SessionRef{
					"session-1": tc.session,
				},
				err: tc.resolverErr,
			}
			authorizer := &fakeRBACAuthorizer{
				result: rbacusecase.AuthorizeDecision{
					Allowed:   !tc.denyRBAC,
					CheckedAt: now,
				},
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/ws-ticket", strings.NewReader(tc.body))
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			testHandler(
				&fakeUOWFactory{configRepo: &fakeConfigRepo{}},
				WithDiagnosisAuth(authProvider, service, resolver),
				WithRBACAuthorizer(authorizer),
				withDiagnosisClock(func() time.Time { return now }),
			).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if calls := authProvider.Calls("Bearer oidc-token"); calls != tc.wantAuthCalls {
				t.Fatalf("auth calls = %d, want %d", calls, tc.wantAuthCalls)
			}
		})
	}
}

func TestCreateDiagnosisRoomAuthenticatesAndStartsRoom(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}},
		},
	})
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)},
	}
	starter := &fakeDiagnosisRoomStarter{
		result: diagnosisroomstart.Result{
			SessionID:          "diagnosis-session-1",
			EvidenceSnapshotID: 42,
			DiagnosisTaskID:    101,
			ChatSessionID:      202,
			Workflow:           ports.WorkflowHandle{WorkflowID: "diagnosis-room-diagnosis-session-1", RunID: "run-1"},
			ApprovalMode:       domain.DiagnosisApprovalModeOwnerAndLeader,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/rooms", strings.NewReader(`{"evidence_snapshot_id":42,"close_notification_channel_profile_id":5,"approval_mode":"owner_and_leader"}`))
	req.Header.Set("Authorization", "Bearer oidc-token")
	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("D", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithRBACAuthorizer(authorizer),
		WithDiagnosisRoomStarter(starter),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.Calls("Bearer oidc-token") != 1 {
		t.Fatalf("auth calls = %d, want 1", authProvider.Calls("Bearer oidc-token"))
	}
	if len(authorizer.requests) != 3 {
		t.Fatalf("authorizer requests = %+v, want participate, operations read, channel test", authorizer.requests)
	}
	if authorizer.requests[0].Permission != domain.RBACPermissionDiagnosisRoomParticipate ||
		authorizer.requests[0].ScopeKind != domain.RBACScopeKindGlobal ||
		authorizer.requests[0].ScopeKey != "" {
		t.Fatalf("participate request = %+v", authorizer.requests[0])
	}
	if authorizer.requests[1].Permission != domain.RBACPermissionOperationsRead ||
		authorizer.requests[1].ScopeKind != domain.RBACScopeKindGlobal ||
		authorizer.requests[1].ScopeKey != "" {
		t.Fatalf("operations read request = %+v", authorizer.requests[1])
	}
	if authorizer.requests[2].Permission != domain.RBACPermissionNotificationChannelTest ||
		authorizer.requests[2].ScopeKind != domain.RBACScopeKindNotificationChannel ||
		authorizer.requests[2].ScopeKey != "5" {
		t.Fatalf("notification channel request = %+v", authorizer.requests[2])
	}
	if starter.called != 1 ||
		starter.req.EvidenceSnapshotID != 42 ||
		starter.req.CloseNotificationChannelProfileID != 5 ||
		starter.req.ApprovalMode != domain.DiagnosisApprovalModeOwnerAndLeader ||
		starter.req.Principal.Subject != "owner-1" {
		t.Fatalf("starter called=%d req=%+v", starter.called, starter.req)
	}
	var body api.DiagnosisRoomCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.SessionID != "diagnosis-session-1" ||
		body.EvidenceSnapshotID != 42 ||
		body.DiagnosisTaskID != 101 ||
		body.ChatSessionID != 202 ||
		body.WorkflowID != "diagnosis-room-diagnosis-session-1" ||
		body.RunID != "run-1" ||
		body.ApprovalMode != api.OwnerAndLeader {
		t.Fatalf("response = %+v", body)
	}
}

func TestCreateDiagnosisRoomRequiresEvidenceReadAccess(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "responder-1"}},
		},
	})
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			return rbacusecase.AuthorizeDecision{
				Allowed: req.Permission == domain.RBACPermissionDiagnosisRoomParticipate,
			}, nil
		},
	}
	starter := &fakeDiagnosisRoomStarter{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/rooms", strings.NewReader(`{"evidence_snapshot_id":42}`))
	req.Header.Set("Authorization", "Bearer oidc-token")
	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("D", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithRBACAuthorizer(authorizer),
		WithDiagnosisRoomStarter(starter),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if starter.called != 0 {
		t.Fatalf("starter called=%d, want 0", starter.called)
	}
	if len(authorizer.requests) != 2 ||
		authorizer.requests[0].Permission != domain.RBACPermissionDiagnosisRoomParticipate ||
		authorizer.requests[1].Permission != domain.RBACPermissionOperationsRead {
		t.Fatalf("authorizer requests = %+v", authorizer.requests)
	}
}

func TestCreateDiagnosisRoomRequiresNotificationChannelAccess(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "operator-1"}},
		},
	})
	authorizer := &fakeRBACAuthorizer{
		authorize: func(req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
			allowed := req.Permission == domain.RBACPermissionDiagnosisRoomParticipate ||
				req.Permission == domain.RBACPermissionOperationsRead
			return rbacusecase.AuthorizeDecision{Allowed: allowed}, nil
		},
	}
	starter := &fakeDiagnosisRoomStarter{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/rooms", strings.NewReader(`{"evidence_snapshot_id":42,"close_notification_channel_profile_id":5}`))
	req.Header.Set("Authorization", "Bearer oidc-token")
	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("D", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithRBACAuthorizer(authorizer),
		WithDiagnosisRoomStarter(starter),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if starter.called != 0 {
		t.Fatalf("starter called=%d, want 0", starter.called)
	}
	if len(authorizer.requests) != 3 ||
		authorizer.requests[2].Permission != domain.RBACPermissionNotificationChannelTest ||
		authorizer.requests[2].ScopeKind != domain.RBACScopeKindNotificationChannel ||
		authorizer.requests[2].ScopeKey != "5" {
		t.Fatalf("authorizer requests = %+v", authorizer.requests)
	}
}

func TestCloseUnavailableDiagnosisRoomAuthenticatesAndClosesRoom(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}},
		},
	})
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: time.Date(2026, 6, 20, 1, 4, 0, 0, time.UTC)},
	}
	closedAt := time.Date(2026, 6, 20, 1, 5, 0, 0, time.UTC)
	startedAt := closedAt.Add(-5 * time.Minute)
	closer := &fakeDiagnosisRoomCloser{
		result: diagnosisroomclose.Result{
			Session: domain.ChatSession{
				ID:              409,
				DiagnosisTaskID: 309,
				SessionKey:      "diagnosis-session-orphaned-workflow",
				OwnerSubject:    "owner-1",
				Status:          domain.ChatSessionStatusClosed,
				TurnCount:       1,
				StartedAt:       startedAt,
				LastActivityAt:  closedAt,
				ClosedAt:        &closedAt,
				CloseReason:     diagnosisroomclose.DefaultUnavailableCloseReason,
				CreatedAt:       startedAt,
				UpdatedAt:       closedAt,
			},
			Task: domain.DiagnosisTask{
				ID:                 309,
				EvidenceSnapshotID: 9001,
				WorkflowID:         "diagnosis-room-diagnosis-session-orphaned-workflow",
				RunID:              "run-orphaned-9001",
				Status:             domain.DiagnosisStatusCancelled,
				StartedAt:          &startedAt,
				FinishedAt:         &closedAt,
				CreatedAt:          startedAt,
				UpdatedAt:          closedAt,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/diagnosis/rooms/diagnosis-session-orphaned-workflow/close-unavailable",
		strings.NewReader(`{"reason":"workflow_unavailable"}`),
	)
	req.Header.Set("Authorization", "Bearer oidc-token")
	testHandler(
		&fakeUOWFactory{configRepo: &fakeConfigRepo{}},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("C", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithRBACAuthorizer(authorizer),
		WithDiagnosisRoomCloser(closer),
		withDiagnosisClock(func() time.Time { return closedAt }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.Calls("Bearer oidc-token") != 1 {
		t.Fatalf("auth calls = %d, want 1", authProvider.Calls("Bearer oidc-token"))
	}
	if authorizer.called != 1 ||
		authorizer.req.Permission != domain.RBACPermissionDiagnosisRoomAdminister ||
		authorizer.req.ScopeKind != domain.RBACScopeKindDiagnosisRoom ||
		authorizer.req.ScopeKey != "diagnosis-session-orphaned-workflow" {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if closer.called != 1 ||
		closer.req.SessionID != "diagnosis-session-orphaned-workflow" ||
		closer.req.Principal.Subject != "owner-1" ||
		closer.req.Reason != diagnosisroomclose.DefaultUnavailableCloseReason ||
		!closer.req.Now.Equal(closedAt) {
		t.Fatalf("closer called=%d req=%+v", closer.called, closer.req)
	}
	var body api.DiagnosisRoomSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.SessionID != "diagnosis-session-orphaned-workflow" ||
		body.RoomStatus != api.Closed ||
		body.TaskStatus != api.DiagnosisTaskStatusCancelled ||
		body.CloseReason != diagnosisroomclose.DefaultUnavailableCloseReason {
		t.Fatalf("response = %+v", body)
	}
}

func TestCreateDiagnosisRoomRejectsBadInputs(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		authHeader    string
		starter       *fakeDiagnosisRoomStarter
		principal     ports.AuthPrincipal
		withAuth      bool
		wantStatus    int
		wantAuthCalls int
	}{
		{
			name:       "unconfigured starter",
			body:       `{"evidence_snapshot_id":42}`,
			authHeader: "Bearer oidc-token",
			principal:  ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:   true,
			wantStatus: stdhttp.StatusServiceUnavailable,
		},
		{
			name:          "unknown field",
			body:          `{"evidence_snapshot_id":42,"session_id":"manual"}`,
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 1,
		},
		{
			name:          "duplicate evidence snapshot id",
			body:          `{"evidence_snapshot_id":42,"evidence_snapshot_id":43}`,
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 1,
		},
		{
			name:          "overlarge body",
			body:          strings.Repeat("{", maxJSONRequestBodyBytes+1),
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 1,
		},
		{
			name:          "invalid notification channel profile",
			body:          `{"evidence_snapshot_id":42,"close_notification_channel_profile_id":0}`,
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 1,
		},
		{
			name:          "invalid approval mode",
			body:          `{"evidence_snapshot_id":42,"approval_mode":"committee"}`,
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 1,
		},
		{
			name:       "missing bearer",
			body:       `{"evidence_snapshot_id":42}`,
			starter:    &fakeDiagnosisRoomStarter{},
			withAuth:   true,
			wantStatus: stdhttp.StatusUnauthorized,
		},
		{
			name:          "snapshot not found",
			body:          `{"evidence_snapshot_id":42}`,
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{err: domain.ErrNotFound},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusNotFound,
			wantAuthCalls: 1,
		},
		{
			name:          "unauthorized",
			body:          `{"evidence_snapshot_id":42}`,
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{err: diagnosisauth.ErrUnauthorized},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusForbidden,
			wantAuthCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authProvider := authfake.New(map[string][]authfake.Result{
				"Bearer oidc-token": {
					{Principal: tc.principal},
				},
			})
			var opts []ServerOption
			if tc.withAuth {
				opts = append(opts, WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("E", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}))
				opts = append(opts, WithRBACAuthorizer(&fakeRBACAuthorizer{
					result: rbacusecase.AuthorizeDecision{Allowed: true, CheckedAt: time.Date(2026, 6, 20, 1, 10, 0, 0, time.UTC)},
				}))
			}
			if tc.starter != nil {
				opts = append(opts, WithDiagnosisRoomStarter(tc.starter))
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/rooms", strings.NewReader(tc.body))
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			testHandler(&fakeUOWFactory{configRepo: &fakeConfigRepo{}}, opts...).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if calls := authProvider.Calls("Bearer oidc-token"); calls != tc.wantAuthCalls {
				t.Fatalf("auth calls = %d, want %d", calls, tc.wantAuthCalls)
			}
		})
	}
}

func TestHandleDiagnosisWebSocketConsumesTicketAndHandsOffConnection(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("D", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	wsHandler := &fakeDiagnosisWebSocketHandler{done: make(chan struct{})}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisWebSocketHandler(wsHandler),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready map[string]string
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	select {
	case <-wsHandler.done:
	case <-time.After(time.Second):
		t.Fatal("websocket handler did not finish")
	}
	if ready["type"] != "ready" || ready["subject"] != "owner-1" {
		t.Fatalf("ready = %+v", ready)
	}
	if wsHandler.called != 1 || wsHandler.ticket.Token != "" || wsHandler.ticket.SessionID != "session-1" {
		t.Fatalf("handler called=%d ticket=%+v", wsHandler.called, wsHandler.ticket)
	}
	_, err = service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(2*time.Second))
	if !errors.Is(err, diagnosisauth.ErrTicketConsumed) {
		t.Fatalf("ConsumeTicket after websocket err = %v, want consumed", err)
	}
}

func TestHandleDiagnosisWebSocketAcceptsSameHostOrigin(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("O", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	wsHandler := &fakeDiagnosisWebSocketHandler{done: make(chan struct{})}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisWebSocketHandler(wsHandler),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	headers := stdhttp.Header{"Origin": []string{"http://" + host}}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready map[string]string
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	select {
	case <-wsHandler.done:
	case <-time.After(time.Second):
		t.Fatal("websocket handler did not finish")
	}
	if ready["type"] != "ready" || wsHandler.called != 1 {
		t.Fatalf("ready=%+v handler called=%d", ready, wsHandler.called)
	}
}

func TestHandleDiagnosisWebSocketRejectsBadOriginBeforeConsumingTicket(t *testing.T) {
	tests := []struct {
		name   string
		origin func(host string) string
	}{
		{
			name:   "different host",
			origin: func(string) string { return "https://evil.example" },
		},
		{
			name:   "userinfo same host",
			origin: func(host string) string { return "https://operator@" + host },
		},
		{
			name:   "escaped userinfo same host",
			origin: func(host string) string { return "https://operator%40team@" + host },
		},
		{
			name:   "malformed",
			origin: func(string) string { return "http://[::1" },
		},
		{
			name:   "missing host",
			origin: func(host string) string { return "https:" + host },
		},
		{
			name:   "path same host",
			origin: func(host string) string { return "https://" + host + "/diagnosis" },
		},
		{
			name:   "root path same host",
			origin: func(host string) string { return "https://" + host + "/" },
		},
		{
			name:   "query same host",
			origin: func(host string) string { return "https://" + host + "?ticket=redacted" },
		},
		{
			name:   "fragment same host",
			origin: func(host string) string { return "https://" + host + "#diagnosis" },
		},
		{
			name:   "scheme same host",
			origin: func(host string) string { return "ftp://" + host },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
			service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("E", diagnosisauth.DefaultTokenBytes)))
			session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
			ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
				Subject: "owner-1",
				Roles:   []ports.AuthRole{ports.AuthRoleOwner},
			}, session, now)
			if err != nil {
				t.Fatalf("IssueTicket: %v", err)
			}
			handler := testHandlerWithDiagnosisWS(
				&fakeUOWFactory{},
				WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
					sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
				}),
				WithDiagnosisWebSocketHandler(&fakeDiagnosisWebSocketHandler{}),
				withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
			)
			srv := httptest.NewServer(handler)
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			headers := stdhttp.Header{"Origin": []string{tc.origin(host)}}
			wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
			conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
			defer closeWebSocketDialResponse(resp)
			if err == nil {
				_ = conn.Close()
				t.Fatal("Dial with bad origin: want error")
			}
			if resp == nil || resp.StatusCode != stdhttp.StatusForbidden {
				t.Fatalf("resp status = %v, want 403", resp)
			}

			if _, err := service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(2*time.Second)); err != nil {
				t.Fatalf("ticket should not be consumed by rejected origin: %v", err)
			}
		})
	}
}

func TestHandleDiagnosisWebSocketRejectsNonUpgradeBeforeConsumingTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("G", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/ws/diagnosis?session_id=session-1&ticket="+ticket.Token, nil)
	testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisWebSocketHandler(&fakeDiagnosisWebSocketHandler{}),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if _, err := service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(2*time.Second)); err != nil {
		t.Fatalf("ticket should not be consumed by non-upgrade request: %v", err)
	}
}

func TestHandleDiagnosisWebSocketRejectsMissingTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("F", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisWebSocketHandler(&fakeDiagnosisWebSocketHandler{}),
		withDiagnosisClock(func() time.Time { return now }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err == nil {
		_ = conn.Close()
		t.Fatal("Dial without ticket: want error")
	}
	if resp == nil || resp.StatusCode != stdhttp.StatusUnauthorized {
		t.Fatalf("resp status = %v, want 401", resp)
	}
}

func TestDiagnosisWSEvidenceTimelineFromTurnResultFallbackPreservesActorSubject(t *testing.T) {
	result := ports.DiagnosisRoomSubmitTurnResult{
		MessageID:          "msg-1",
		AssistantMessageID: "msg-1/assistant",
		TurnCount:          1,
		EvidenceRequests: []ports.DiagnosisRoomEvidenceRequest{{
			Tool:   domain.DiagnosisToolKindActiveAlerts,
			Reason: "Collect current alerts.",
			Limit:  5,
		}},
		CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
			Tool:   domain.DiagnosisToolKindActiveAlerts,
			Status: "collected",
		}},
		FollowUpTurns: []ports.DiagnosisRoomFollowUpTurnResult{{
			MessageID:          "msg-1/auto-evidence-1",
			AssistantMessageID: "msg-1/auto-evidence-1/assistant",
			TurnCount:          2,
			CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
				Tool:   domain.DiagnosisToolKindMetricQuery,
				Status: "collected",
			}},
			Trigger: "collected_evidence",
		}},
	}

	timeline := diagnosisWSEvidenceTimelineFromTurnResult(result, "owner-1")
	if len(timeline) != 2 {
		t.Fatalf("timeline len = %d, want 2: %+v", len(timeline), timeline)
	}
	if timeline[0].ActorSubject != "owner-1" ||
		timeline[0].Trigger != "operator_turn" ||
		timeline[0].MessageID != "msg-1" {
		t.Fatalf("operator timeline entry = %+v", timeline[0])
	}
	if timeline[1].ActorSubject != "owner-1" ||
		timeline[1].Trigger != "collected_evidence" ||
		timeline[1].MessageID != "msg-1/auto-evidence-1" {
		t.Fatalf("follow-up timeline entry = %+v", timeline[1])
	}
}

func TestDiagnosisWSHistoricalRetrievalProjection(t *testing.T) {
	followUpRefs := []string{"final_report:91"}
	timelineRefs := []string{"sub_report:44", "final_report:91"}
	followUps := diagnosisWSFollowUpTurns([]ports.DiagnosisRoomFollowUpTurnResult{{
		ContextBytes:  1536,
		RetrievalRefs: followUpRefs,
	}})
	timeline := diagnosisWSConfidenceTimeline([]ports.DiagnosisRoomConfidenceTimelineEntry{{
		ContextBytes:  1536,
		RetrievalRefs: timelineRefs,
	}})

	if len(followUps) != 1 ||
		followUps[0].ContextBytes != 1536 ||
		!slices.Equal(followUps[0].RetrievalRefs, followUpRefs) {
		t.Fatalf("follow-up historical retrieval frame = %+v", followUps)
	}
	if len(timeline) != 1 ||
		timeline[0].ContextBytes != 1536 ||
		!slices.Equal(timeline[0].RetrievalRefs, timelineRefs) {
		t.Fatalf("confidence timeline historical retrieval frame = %+v", timeline)
	}

	followUpRefs[0] = "final_report:999"
	timelineRefs[0] = "sub_report:998"
	if followUps[0].RetrievalRefs[0] != "final_report:91" ||
		timeline[0].RetrievalRefs[0] != "sub_report:44" {
		t.Fatalf("websocket retrieval refs alias use-case results: follow_ups=%+v timeline=%+v", followUps, timeline)
	}
}

func TestDiagnosisWebSocketRelaySubmitsTurnAndQueriesState(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("H", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	requiresHumanReview := true
	latestRequiresHumanReview := false
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	startedAt := now.Add(time.Minute)
	closedAt := startedAt.Add(2 * time.Second)
	readyState := ports.DiagnosisRoomState{
		SessionID:       "session-1",
		ChatSessionID:   domain.ChatSessionID(21),
		DiagnosisTaskID: domain.DiagnosisTaskID(11),
		OwnerSubject:    "owner-1",
		Status:          "open",
		TurnCount:       1,
		StartedAt:       startedAt,
		LastActivityAt:  startedAt,
		LatestConsultationInsight: &ports.DiagnosisRoomConsultationInsight{
			ConfidenceRationale: "Collected evidence now supports final review.",
			EvidenceCollectionSuggestions: []ports.DiagnosisRoomConsultationEvidenceRequest{{
				Label:    "Owner confirmation",
				Detail:   "Confirm the proposed conclusion with the service owner.",
				Priority: "medium",
			}},
			ConclusionStatus: "ready_for_review",
		},
		LatestConfidence:          "high",
		LatestRequiresHumanReview: &latestRequiresHumanReview,
		LatestEvidenceRequests: []ports.DiagnosisRoomEvidenceRequest{{
			TemplateID:           domain.DiagnosisToolTemplateID(77),
			AlertSourceProfileID: domain.AlertSourceProfileID(3),
			Tool:                 domain.DiagnosisToolKindMetricQuery,
			Reason:               "Read current CPU saturation.",
			Query:                "up",
		}},
		LatestCollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
			Request: ports.DiagnosisRoomEvidenceRequest{
				TemplateID:           domain.DiagnosisToolTemplateID(77),
				AlertSourceProfileID: domain.AlertSourceProfileID(3),
				Tool:                 domain.DiagnosisToolKindMetricQuery,
				Reason:               "Read current CPU saturation.",
				Query:                "up",
			},
			TemplateID:           domain.DiagnosisToolTemplateID(77),
			AlertSourceProfileID: domain.AlertSourceProfileID(3),
			Tool:                 domain.DiagnosisToolKindMetricQuery,
			Status:               "collected",
			ReasonCode:           "ok",
			Message:              "Metric query collection succeeded.",
			Query:                "up",
			ObservedMetricSeries: 1,
			CollectedAt:          startedAt.Add(time.Second),
		}},
		EvidenceTimeline: []ports.DiagnosisRoomEvidenceTimelineEntry{{
			TurnCount:          1,
			MessageID:          "msg-1",
			AssistantMessageID: "msg-1/assistant",
			ActorSubject:       "owner-1",
			Trigger:            "operator_turn",
			EvidenceRequests: []ports.DiagnosisRoomEvidenceRequest{{
				TemplateID:           domain.DiagnosisToolTemplateID(77),
				AlertSourceProfileID: domain.AlertSourceProfileID(3),
				Tool:                 domain.DiagnosisToolKindMetricQuery,
				Reason:               "Read current CPU saturation.",
				Query:                "up",
			}},
			CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
				TemplateID:           domain.DiagnosisToolTemplateID(77),
				AlertSourceProfileID: domain.AlertSourceProfileID(3),
				Tool:                 domain.DiagnosisToolKindMetricQuery,
				Status:               "collected",
				ReasonCode:           "ok",
				Message:              "Metric query collection succeeded.",
				Query:                "up",
				ObservedMetricSeries: 1,
				CollectedAt:          startedAt.Add(time.Second),
			}},
		}},
		SupplementalEvidence: []ports.DiagnosisRoomSupplementalEvidenceRecord{{
			Label:              "Restart cause",
			Detail:             "Collect previous container logs.",
			Priority:           "high",
			Evidence:           "Previous logs show the pod restarted after OOMKilled.",
			ActorSubject:       "reviewer-1",
			UserMessageID:      "msg-2",
			AssistantMessageID: "msg-2/assistant",
			UserTurnID:         domain.ChatTurnID(33),
			AssistantTurnID:    domain.ChatTurnID(34),
			UserSequence:       3,
			AssistantSequence:  4,
			ProvidedAt:         startedAt.Add(time.Second),
		}},
		InFlight:       false,
		SeenMessageIDs: []string{"msg-1"},
		Conversation: []ports.DiagnosisRoomConversationTurn{
			{Role: "user", ActorSubject: "reviewer-1", Content: "Please investigate"},
			{Role: "assistant", ActorSubject: "openclarion:auto-diagnosis", Content: "CPU alert is still firing."},
		},
	}
	confirmedState := readyState
	confirmedState.Status = "closed"
	confirmedState.LastActivityAt = closedAt
	confirmedState.ClosedAt = &closedAt
	confirmedState.CloseReason = "human_confirmed"
	confirmedState.FinalConclusion = &ports.DiagnosisRoomFinalConclusion{
		Status:                  "available",
		Source:                  "latest_assistant_turn",
		EvidenceSnapshotID:      domain.EvidenceSnapshotID(9001),
		ConclusionVersion:       "diagnosis-room-close.v1",
		RecordedAt:              &closedAt,
		ConfirmedBy:             "owner-1",
		SupplementalContextRefs: []string{"chat_session:21/turn:31", "chat_session:21/turn:32"},
		AssistantTurnID:         domain.ChatTurnID(32),
		AssistantMessageID:      "msg-1/assistant",
		AssistantSequence:       2,
		AssistantOccurredAt:     &startedAt,
		Content:                 "CPU alert is still firing.",
		Confidence:              "medium",
		RequiresHumanReview:     &requiresHumanReview,
		ConfidenceRationale:     "CPU and restart evidence are aligned.",
		Findings:                []string{"api-1 CPU exceeded threshold"},
		RecommendedActions:      []string{"Scale api-1"},
		EvidenceRequests: []ports.DiagnosisRoomEvidenceRequest{{
			Tool:   domain.DiagnosisToolKindActiveAlerts,
			Reason: "Confirm sibling alerts.",
			Limit:  5,
		}},
		MissingEvidenceRequests: []ports.DiagnosisRoomConsultationEvidenceRequest{{
			Label:    "Restart cause",
			Detail:   "Inspect previous container logs.",
			Priority: "high",
		}},
		EvidenceCollectionSuggestions: []ports.DiagnosisRoomConsultationEvidenceRequest{{
			Label:    "CPU trend",
			Detail:   "Collect a bounded CPU range query.",
			Priority: "medium",
		}},
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		submitResult: ports.DiagnosisRoomSubmitTurnResult{
			SessionID:           "session-1",
			ChatSessionID:       domain.ChatSessionID(21),
			MessageID:           "msg-1",
			AssistantMessageID:  "msg-1-assistant",
			UserTurnID:          domain.ChatTurnID(31),
			AssistantTurnID:     domain.ChatTurnID(32),
			UserSequence:        1,
			AssistantSequence:   2,
			TurnCount:           1,
			ContextBytes:        100,
			Status:              "open",
			AssistantMessage:    "CPU alert is still firing.",
			RequiresHumanReview: true,
			Confidence:          "medium",
			RetrievalRefs:       []string{"sub_report:44", "final_report:91"},
			EvidenceRequests: []ports.DiagnosisRoomEvidenceRequest{{
				AlertSourceProfileID: domain.AlertSourceProfileID(3),
				Tool:                 domain.DiagnosisToolKindActiveAlerts,
				Reason:               "Need current sibling alerts.",
				Limit:                5,
			}},
			CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
				Tool:           domain.DiagnosisToolKindActiveAlerts,
				Status:         "collected",
				ReasonCode:     "ok",
				Message:        "Active alert collection succeeded.",
				ObservedAlerts: 1,
				ActiveAlerts: []ports.DiagnosisRoomActiveAlert{{
					Source:   "alertmanager",
					Labels:   map[string]string{"alertname": "CPUHigh"},
					StartsAt: now,
				}},
				CollectedAt: now.Add(time.Second),
			}, {
				Tool:                 domain.DiagnosisToolKindMetricQuery,
				Status:               "collected",
				ReasonCode:           "ok",
				Message:              "Metric query collection succeeded.",
				Query:                "up",
				ObservedMetricSeries: 1,
				MetricResult: ports.DiagnosisRoomMetricQueryResult{
					ResultType: "vector",
					Series: []ports.DiagnosisRoomMetricSeries{{
						Metric: map[string]string{"__name__": "up", "job": "prometheus"},
						Points: []ports.DiagnosisRoomMetricPoint{{
							Timestamp: now,
							Value:     "1",
						}},
					}},
					Warnings: []string{"partial response"},
				},
				CollectedAt: now.Add(time.Second),
			}},
			EvidenceTimeline: []ports.DiagnosisRoomEvidenceTimelineEntry{{
				TurnCount:          1,
				MessageID:          "msg-1",
				AssistantMessageID: "msg-1-assistant",
				ActorSubject:       "owner-1",
				Trigger:            "operator_turn",
				EvidenceRequests: []ports.DiagnosisRoomEvidenceRequest{{
					AlertSourceProfileID: domain.AlertSourceProfileID(3),
					Tool:                 domain.DiagnosisToolKindActiveAlerts,
					Reason:               "Need current sibling alerts.",
					Limit:                5,
				}},
				CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
					Tool:           domain.DiagnosisToolKindActiveAlerts,
					Status:         "collected",
					ReasonCode:     "ok",
					Message:        "Active alert collection succeeded.",
					ObservedAlerts: 1,
					CollectedAt:    now.Add(time.Second),
				}},
			}, {
				TurnCount:          2,
				MessageID:          "msg-1/auto-evidence-1",
				AssistantMessageID: "msg-1/auto-evidence-1/assistant",
				Trigger:            "collected_evidence",
				CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
					Tool:           domain.DiagnosisToolKindActiveAlerts,
					Status:         "collected",
					ReasonCode:     "ok",
					Message:        "Active alert collection succeeded.",
					ObservedAlerts: 1,
					CollectedAt:    now.Add(2 * time.Second),
				}},
			}},
			ConsultationInsight: ports.DiagnosisRoomConsultationInsight{
				ConfidenceRationale: "CPU evidence is present but restart evidence is missing.",
				MissingEvidenceRequests: []ports.DiagnosisRoomConsultationEvidenceRequest{{
					Label:    "Restart cause",
					Detail:   "Inspect previous pod logs.",
					Priority: "high",
				}},
				ConclusionStatus: "needs_evidence",
			},
			FollowUpTurns: []ports.DiagnosisRoomFollowUpTurnResult{{
				MessageID:           "msg-1/auto-evidence-1",
				UserMessage:         "OpenClarion automatic evidence follow-up.",
				AssistantMessageID:  "msg-1/auto-evidence-1/assistant",
				UserTurnID:          domain.ChatTurnID(33),
				AssistantTurnID:     domain.ChatTurnID(34),
				UserSequence:        3,
				AssistantSequence:   4,
				TurnCount:           2,
				ContextBytes:        512,
				AssistantMessage:    "Collected evidence confirms CPU saturation.",
				RequiresHumanReview: false,
				Confidence:          "high",
				CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
					Tool:           domain.DiagnosisToolKindActiveAlerts,
					Status:         "collected",
					ReasonCode:     "ok",
					Message:        "Active alert collection succeeded.",
					ObservedAlerts: 1,
					CollectedAt:    now.Add(2 * time.Second),
				}},
				ConsultationInsight: ports.DiagnosisRoomConsultationInsight{
					ConclusionStatus: "final",
				},
				Trigger: "collected_evidence",
			}},
			LatestError: &ports.DiagnosisRoomLatestError{
				Code:       "notification_failed",
				Message:    "AI diagnosis was saved, but downstream diagnosis notification delivery failed; review notification channel configuration.",
				MessageID:  "msg-1/assistant",
				OccurredAt: now.Add(2 * time.Second),
			},
		},
		queryState:   readyState,
		confirmState: confirmedState,
	}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if ready.Type != diagnosisWSServerReady || ready.SessionID != "session-1" || ready.Subject != "owner-1" {
		t.Fatalf("ready = %+v", ready)
	}

	if err := conn.WriteJSON(map[string]string{
		"type":       diagnosisWSClientSubmitTurn,
		"message_id": "msg-1",
		"message":    "Please investigate",
	}); err != nil {
		t.Fatalf("WriteJSON submit: %v", err)
	}
	var turn diagnosisWSTurnResultFrame
	if err := conn.ReadJSON(&turn); err != nil {
		t.Fatalf("ReadJSON turn: %v", err)
	}
	if turn.Type != diagnosisWSServerTurnResult ||
		turn.MessageID != "msg-1" ||
		turn.AssistantMessage != "CPU alert is still firing." ||
		!slices.Equal(turn.RetrievalRefs, []string{"sub_report:44", "final_report:91"}) {
		t.Fatalf("turn = %+v", turn)
	}
	if turn.ConsultationInsight.ConfidenceRationale != "CPU evidence is present but restart evidence is missing." ||
		len(turn.ConsultationInsight.MissingEvidenceRequests) != 1 ||
		turn.ConsultationInsight.MissingEvidenceRequests[0].Label != "Restart cause" ||
		turn.ConsultationInsight.ConclusionStatus != "needs_evidence" {
		t.Fatalf("turn consultation insight = %+v", turn.ConsultationInsight)
	}
	if len(turn.EvidenceRequests) != 1 ||
		turn.EvidenceRequests[0].AlertSourceProfileID != 3 ||
		turn.EvidenceRequests[0].Tool != "active_alerts" ||
		turn.EvidenceRequests[0].Reason != "Need current sibling alerts." ||
		turn.EvidenceRequests[0].Limit != 5 {
		t.Fatalf("turn evidence requests = %+v", turn.EvidenceRequests)
	}
	if len(turn.CollectionResults) != 2 ||
		turn.CollectionResults[0].Status != "collected" ||
		turn.CollectionResults[0].ReasonCode != "ok" ||
		turn.CollectionResults[0].ObservedAlerts != 1 ||
		len(turn.CollectionResults[0].ActiveAlerts) != 1 ||
		turn.CollectionResults[0].ActiveAlerts[0].Labels["alertname"] != "CPUHigh" {
		t.Fatalf("turn collection results = %+v", turn.CollectionResults)
	}
	metricResult := turn.CollectionResults[1]
	if metricResult.Query != "up" ||
		metricResult.ObservedMetricSeries != 1 ||
		metricResult.MetricResult.ResultType != "vector" ||
		metricResult.MetricResult.Series[0].Metric["job"] != "prometheus" ||
		metricResult.MetricResult.Series[0].Points[0].Value != "1" ||
		metricResult.MetricResult.Warnings[0] != "partial response" {
		t.Fatalf("turn metric collection result = %+v", metricResult)
	}
	if len(turn.EvidenceTimeline) != 2 ||
		turn.EvidenceTimeline[0].TurnCount != 1 ||
		turn.EvidenceTimeline[0].ActorSubject != "owner-1" ||
		turn.EvidenceTimeline[0].Trigger != "operator_turn" ||
		len(turn.EvidenceTimeline[0].CollectionResults) != 1 ||
		turn.EvidenceTimeline[1].TurnCount != 2 ||
		turn.EvidenceTimeline[1].Trigger != "collected_evidence" ||
		turn.EvidenceTimeline[1].CollectionResults[0].Status != "collected" {
		t.Fatalf("turn evidence timeline = %+v", turn.EvidenceTimeline)
	}
	submitReq, submitCalled := workflowClient.submitSnapshot()
	if submitCalled != 1 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 1", submitCalled)
	}
	if submitReq.SessionID != "session-1" || submitReq.MessageID != "msg-1" || submitReq.ActorSubject != "owner-1" || submitReq.Message != "Please investigate" {
		t.Fatalf("submit request = %+v", submitReq)
	}
	if len(turn.FollowUpTurns) != 1 ||
		turn.FollowUpTurns[0].MessageID != "msg-1/auto-evidence-1" ||
		turn.FollowUpTurns[0].UserMessage != "OpenClarion automatic evidence follow-up." ||
		turn.FollowUpTurns[0].AssistantMessage != "Collected evidence confirms CPU saturation." ||
		turn.FollowUpTurns[0].ConsultationInsight.ConclusionStatus != "final" ||
		turn.FollowUpTurns[0].CollectionResults[0].Status != "collected" ||
		turn.FollowUpTurns[0].Trigger != "collected_evidence" {
		t.Fatalf("turn follow-up results = %+v", turn.FollowUpTurns)
	}
	if turn.LatestError == nil ||
		turn.LatestError.Code != "notification_failed" ||
		turn.LatestError.MessageID != "msg-1/assistant" {
		t.Fatalf("turn latest error = %+v", turn.LatestError)
	}

	if err := conn.WriteJSON(map[string]string{"type": diagnosisWSClientQueryState}); err != nil {
		t.Fatalf("WriteJSON query: %v", err)
	}
	var state diagnosisWSStateFrame
	if err := conn.ReadJSON(&state); err != nil {
		t.Fatalf("ReadJSON state: %v", err)
	}
	if state.Type != diagnosisWSServerState || state.DiagnosisTaskID != 11 || len(state.Conversation) != 2 {
		t.Fatalf("state = %+v", state)
	}
	if state.Conversation[0].ActorSubject != "reviewer-1" ||
		state.Conversation[1].ActorSubject != "openclarion:auto-diagnosis" {
		t.Fatalf("state conversation actor subjects = %+v", state.Conversation)
	}
	if state.Status != "open" || state.FinalConclusion != nil {
		t.Fatalf("state status=%q final conclusion=%+v, want open/nil", state.Status, state.FinalConclusion)
	}
	if state.ConsultationInsight == nil ||
		state.ConsultationInsight.ConfidenceRationale != "Collected evidence now supports final review." ||
		len(state.ConsultationInsight.EvidenceCollectionSuggestions) != 1 ||
		state.ConsultationInsight.EvidenceCollectionSuggestions[0].Label != "Owner confirmation" ||
		state.ConsultationInsight.ConclusionStatus != "ready_for_review" ||
		state.Confidence != "high" ||
		state.RequiresHumanReview == nil ||
		*state.RequiresHumanReview {
		t.Fatalf("state latest consultation = insight=%+v confidence=%q review=%v",
			state.ConsultationInsight, state.Confidence, state.RequiresHumanReview)
	}
	if len(state.EvidenceRequests) != 1 ||
		state.EvidenceRequests[0].TemplateID != 77 ||
		state.EvidenceRequests[0].AlertSourceProfileID != 3 ||
		len(state.CollectionResults) != 1 ||
		state.CollectionResults[0].TemplateID != 77 ||
		state.CollectionResults[0].AlertSourceProfileID != 3 ||
		state.CollectionResults[0].ObservedMetricSeries != 1 ||
		state.CollectionResults[0].Query != "up" {
		t.Fatalf("state latest evidence = requests=%+v results=%+v",
			state.EvidenceRequests, state.CollectionResults)
	}
	if len(state.EvidenceTimeline) != 1 ||
		state.EvidenceTimeline[0].TurnCount != 1 ||
		state.EvidenceTimeline[0].ActorSubject != "owner-1" ||
		state.EvidenceTimeline[0].Trigger != "operator_turn" ||
		state.EvidenceTimeline[0].EvidenceRequests[0].Tool != "metric_query" ||
		state.EvidenceTimeline[0].CollectionResults[0].Query != "up" {
		t.Fatalf("state evidence timeline = %+v", state.EvidenceTimeline)
	}
	if len(state.SupplementalEvidence) != 1 ||
		state.SupplementalEvidence[0].Label != "Restart cause" ||
		state.SupplementalEvidence[0].ActorSubject != "reviewer-1" ||
		state.SupplementalEvidence[0].UserTurnID != 33 ||
		state.SupplementalEvidence[0].AssistantTurnID != 34 ||
		!state.SupplementalEvidence[0].ProvidedAt.Equal(startedAt.Add(time.Second)) {
		t.Fatalf("state supplemental evidence = %+v", state.SupplementalEvidence)
	}
	if querySession, queryCalled := workflowClient.querySnapshot(); queryCalled != 1 || querySession != "session-1" {
		t.Fatalf("QueryDiagnosisRoom calls=%d session=%q, want 1/session-1", queryCalled, querySession)
	}

	if err := conn.WriteJSON(map[string]string{"type": diagnosisWSClientConfirm}); err != nil {
		t.Fatalf("WriteJSON confirm: %v", err)
	}
	var confirmed diagnosisWSStateFrame
	if err := conn.ReadJSON(&confirmed); err != nil {
		t.Fatalf("ReadJSON confirmed state: %v", err)
	}
	if confirmed.Type != diagnosisWSServerState || confirmed.Status != "closed" || confirmed.CloseReason != "human_confirmed" {
		t.Fatalf("confirmed state = %+v", confirmed)
	}
	if confirmed.FinalConclusion == nil ||
		confirmed.FinalConclusion.Status != "available" ||
		confirmed.FinalConclusion.AssistantTurnID != 32 ||
		confirmed.FinalConclusion.AssistantMessageID != "msg-1/assistant" ||
		confirmed.FinalConclusion.EvidenceSnapshotID != 9001 ||
		confirmed.FinalConclusion.ConclusionVersion != "diagnosis-room-close.v1" ||
		confirmed.FinalConclusion.RecordedAt == nil ||
		!confirmed.FinalConclusion.RecordedAt.Equal(closedAt) ||
		confirmed.FinalConclusion.ConfirmedBy != "owner-1" ||
		len(confirmed.FinalConclusion.SupplementalContextRefs) != 2 ||
		confirmed.FinalConclusion.SupplementalContextRefs[1] != "chat_session:21/turn:32" ||
		confirmed.FinalConclusion.Content != "CPU alert is still firing." ||
		confirmed.FinalConclusion.Confidence != "medium" ||
		confirmed.FinalConclusion.ConfidenceRationale != "CPU and restart evidence are aligned." ||
		len(confirmed.FinalConclusion.Findings) != 1 ||
		confirmed.FinalConclusion.Findings[0] != "api-1 CPU exceeded threshold" ||
		len(confirmed.FinalConclusion.RecommendedActions) != 1 ||
		confirmed.FinalConclusion.RecommendedActions[0] != "Scale api-1" ||
		len(confirmed.FinalConclusion.EvidenceRequests) != 1 ||
		confirmed.FinalConclusion.EvidenceRequests[0].Tool != "active_alerts" ||
		len(confirmed.FinalConclusion.MissingEvidenceRequests) != 1 ||
		confirmed.FinalConclusion.MissingEvidenceRequests[0].Label != "Restart cause" ||
		len(confirmed.FinalConclusion.EvidenceCollectionSuggestions) != 1 ||
		confirmed.FinalConclusion.EvidenceCollectionSuggestions[0].Label != "CPU trend" ||
		confirmed.FinalConclusion.RequiresHumanReview == nil ||
		!*confirmed.FinalConclusion.RequiresHumanReview {
		t.Fatalf("confirmed final conclusion = %+v", confirmed.FinalConclusion)
	}
	confirmReq, confirmCalled := workflowClient.confirmSnapshot()
	if confirmCalled != 1 {
		t.Fatalf("ConfirmDiagnosisConclusion calls = %d, want 1", confirmCalled)
	}
	if confirmReq.SessionID != "session-1" || confirmReq.ActorSubject != "owner-1" || confirmReq.Reason != "human_confirmed" {
		t.Fatalf("confirm request = %+v", confirmReq)
	}
}

func TestDiagnosisWSStateFrameIncludesLatestErrorAndSummary(t *testing.T) {
	now := time.Date(2026, 6, 19, 11, 30, 0, 0, time.UTC)
	frame := diagnosisWSStateFrameFromState(ports.DiagnosisRoomState{
		SessionID:       "session-1",
		ChatSessionID:   domain.ChatSessionID(21),
		DiagnosisTaskID: domain.DiagnosisTaskID(11),
		OwnerSubject:    "owner-1",
		Status:          "open",
		StartedAt:       now,
		LastActivityAt:  now,
		LatestError: &ports.DiagnosisRoomLatestError{
			Code:       "llm_timeout",
			Message:    "Diagnosis turn failed before an assistant response; upstream LLM request timed out.",
			MessageID:  "msg-1",
			OccurredAt: now,
		},
		ConversationSummary: &ports.DiagnosisRoomConversationSummary{
			ID:                  44,
			Version:             1,
			SchemaVersion:       "diagnosis-conversation-summary.v1",
			SourceFirstSequence: 1,
			SourceLastSequence:  2,
			SourceTurnCount:     2,
			SourceDigest:        "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Content:             json.RawMessage(`{"schema_version":"diagnosis-conversation-summary.v1","compression_method":"deterministic-extractive","source_turn_count":2}`),
			GeneratedAt:         now,
		},
		SeenMessageIDs: []string{"msg-1"},
	})

	if frame.LatestError == nil {
		t.Fatalf("LatestError = nil, want frame error")
	}
	if frame.LatestError.Code != "llm_timeout" ||
		frame.LatestError.MessageID != "msg-1" ||
		!frame.LatestError.OccurredAt.Equal(now) {
		t.Fatalf("LatestError = %+v", frame.LatestError)
	}
	if frame.ConversationSummary == nil ||
		frame.ConversationSummary.ID != 44 ||
		frame.ConversationSummary.SourceTurnCount != 2 ||
		string(frame.ConversationSummary.Content) != `{"schema_version":"diagnosis-conversation-summary.v1","compression_method":"deterministic-extractive","source_turn_count":2}` {
		t.Fatalf("ConversationSummary = %+v", frame.ConversationSummary)
	}
}

func TestDiagnosisWebSocketRelayRejectsAmbiguousFrame(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("K", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"submit_turn","message_id":"msg-1","message_id":"msg-2","message":"Please investigate"}`)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	var frame diagnosisWSErrorFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("ReadJSON error: %v", err)
	}
	if frame.Type != diagnosisWSServerError || frame.Code != "bad_frame" || !strings.Contains(frame.Message, "duplicate object key") {
		t.Fatalf("error frame = %+v", frame)
	}
	if _, submitCalled := workflowClient.submitSnapshot(); submitCalled != 0 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 0", submitCalled)
	}
}

func TestDiagnosisWebSocketRelayReportsConfirmRejected(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("L", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		confirmErr: fmt.Errorf(
			"diagnosis-room client: get confirm conclusion result: diagnosis room confirm conclusion: resolve missing evidence requests before confirming: %w",
			domain.ErrPreconditionFailed,
		),
	}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{"type": diagnosisWSClientConfirm}); err != nil {
		t.Fatalf("WriteJSON confirm: %v", err)
	}
	var frame diagnosisWSErrorFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("ReadJSON error: %v", err)
	}
	if frame.Type != diagnosisWSServerError ||
		frame.Code != "confirm_rejected" ||
		!strings.Contains(frame.Message, "resolve missing evidence requests before confirming") {
		t.Fatalf("error frame = %+v", frame)
	}
	confirmReq, confirmCalled := workflowClient.confirmSnapshot()
	if confirmCalled != 1 || confirmReq.SessionID != "session-1" || confirmReq.ActorSubject != "owner-1" {
		t.Fatalf("ConfirmDiagnosisConclusion calls=%d request=%+v", confirmCalled, confirmReq)
	}
}

func TestDiagnosisWebSocketRelayRejectsUnauthorizedConfirmFrame(t *testing.T) {
	now := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("U", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		confirmState: ports.DiagnosisRoomState{SessionID: "session-1", Status: "closed"},
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: false, CheckedAt: now},
	}
	configRepo := &fakeConfigRepo{}
	factory := &fakeUOWFactory{configRepo: configRepo}
	handler := testHandlerWithDiagnosisWS(
		factory,
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithRBACAuthorizer(authorizer),
		WithDiagnosisRoomWorkflowClient(workflowClient),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{"type": diagnosisWSClientConfirm}); err != nil {
		t.Fatalf("WriteJSON confirm: %v", err)
	}
	var frame diagnosisWSErrorFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("ReadJSON error: %v", err)
	}
	if frame.Type != diagnosisWSServerError || frame.Code != "unauthorized" || frame.Message != "unauthorized" {
		t.Fatalf("error frame = %+v", frame)
	}
	if authorizer.called != 1 ||
		authorizer.req.Principal.Subject != "owner-1" ||
		authorizer.req.Permission != domain.RBACPermissionDiagnosisRoomApprove ||
		authorizer.req.ScopeKind != domain.RBACScopeKindDiagnosisRoom ||
		authorizer.req.ScopeKey != "session-1" {
		t.Fatalf("authorizer request = %+v called=%d", authorizer.req, authorizer.called)
	}
	if configRepo.lastDirectorySubject != "owner-1" {
		t.Fatalf("directory subject = %q, want owner-1", configRepo.lastDirectorySubject)
	}
	if confirmReq, confirmCalled := workflowClient.confirmSnapshot(); confirmCalled != 0 {
		t.Fatalf("ConfirmDiagnosisConclusion calls=%d request=%+v, want 0", confirmCalled, confirmReq)
	}
}

func TestDiagnosisWebSocketRelayAllowsOwnerConfirmWithoutScopedAssignment(t *testing.T) {
	now := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("O", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		confirmState: ports.DiagnosisRoomState{SessionID: "session-1", Status: "closed"},
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{Allowed: false, CheckedAt: now},
	}
	configRepo := &fakeConfigRepo{}
	diagnosisRepo := &fakeDiagnosisRepo{
		chatSessions: []domain.ChatSessionWithTask{{
			Session: domain.ChatSession{
				SessionKey:     "session-1",
				OwnerSubject:   "owner-1",
				Status:         domain.ChatSessionStatusOpen,
				StartedAt:      now,
				LastActivityAt: now,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		}},
	}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{configRepo: configRepo, diagnosisRepo: diagnosisRepo},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithRBACAuthorizer(authorizer),
		WithDiagnosisRoomWorkflowClient(workflowClient),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{"type": diagnosisWSClientConfirm}); err != nil {
		t.Fatalf("WriteJSON confirm: %v", err)
	}
	var frame diagnosisWSStateFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("ReadJSON state: %v", err)
	}
	if frame.Type != diagnosisWSServerState || frame.SessionID != "session-1" || frame.Status != "closed" {
		t.Fatalf("state frame = %+v", frame)
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer called=%d request=%+v, want owner shortcut", authorizer.called, authorizer.req)
	}
	if diagnosisRepo.lastChatSessionKey != "session-1" {
		t.Fatalf("owner lookup session key = %q, want session-1", diagnosisRepo.lastChatSessionKey)
	}
	if configRepo.lastDirectorySubject != "owner-1" {
		t.Fatalf("directory subject = %q, want owner-1", configRepo.lastDirectorySubject)
	}
	confirmReq, confirmCalled := workflowClient.confirmSnapshot()
	if confirmCalled != 1 || confirmReq.SessionID != "session-1" || confirmReq.ActorSubject != "owner-1" {
		t.Fatalf("ConfirmDiagnosisConclusion calls=%d request=%+v", confirmCalled, confirmReq)
	}
}

func TestDiagnosisWebSocketRelayDecodesSupplementalEvidenceFrame(t *testing.T) {
	frame, err := decodeDiagnosisWSClientFrame([]byte(`{
		"type":"submit_supplemental_evidence",
		"message_id":"msg-2",
		"message":"Supplemental evidence update\n\nEvidence provided:\n- previous pod logs show OOMKilled",
		"supplemental_evidence":{
			"label":"Restart cause",
			"detail":"Collect previous container logs.",
			"priority":"high",
			"evidence":"previous pod logs show OOMKilled"
		}
	}`))
	if err != nil {
		t.Fatalf("decode supplemental frame: %v", err)
	}
	if frame.Type != diagnosisWSClientSubmitSupplementalEvidence ||
		frame.MessageID != "msg-2" ||
		frame.Message == "" ||
		frame.SupplementalEvidence == nil {
		t.Fatalf("frame = %+v", frame)
	}
	got := diagnosisWSSupplementalEvidencePort(frame.SupplementalEvidence)
	if got == nil ||
		got.Label != "Restart cause" ||
		got.Detail != "Collect previous container logs." ||
		got.Priority != "high" ||
		got.Evidence != "previous pod logs show OOMKilled" {
		t.Fatalf("supplemental evidence port = %+v", got)
	}
}

func TestDiagnosisWebSocketRelayRejectsInvalidSupplementalEvidenceFrames(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "label leading whitespace",
			body: `{
				"type":"submit_supplemental_evidence",
				"message_id":"msg-2",
				"message":"Supplemental evidence update",
				"supplemental_evidence":{
					"label":" Restart cause",
					"detail":"Collect previous container logs.",
					"priority":"high",
					"evidence":"previous pod logs show OOMKilled"
				}
			}`,
			want: "supplemental_evidence.label must not contain leading or trailing whitespace",
		},
		{
			name: "detail multiline",
			body: `{
				"type":"submit_supplemental_evidence",
				"message_id":"msg-2",
				"message":"Supplemental evidence update",
				"supplemental_evidence":{
					"label":"Restart cause",
					"detail":"Collect previous container logs.\nAttach restart timeline.",
					"priority":"high",
					"evidence":"previous pod logs show OOMKilled"
				}
			}`,
			want: "supplemental_evidence.detail must be single-line",
		},
		{
			name: "unsupported priority",
			body: `{
				"type":"submit_supplemental_evidence",
				"message_id":"msg-2",
				"message":"Supplemental evidence update",
				"supplemental_evidence":{
					"label":"Restart cause",
					"detail":"Collect previous container logs.",
					"priority":"urgent",
					"evidence":"previous pod logs show OOMKilled"
				}
			}`,
			want: "supplemental_evidence.priority is unsupported",
		},
		{
			name: "evidence too large",
			body: fmt.Sprintf(`{
				"type":"submit_supplemental_evidence",
				"message_id":"msg-2",
				"message":"Supplemental evidence update",
				"supplemental_evidence":{
					"label":"Restart cause",
					"detail":"Collect previous container logs.",
					"priority":"high",
					"evidence":%q
				}
			}`, strings.Repeat("x", diagnosisroom.HardMaxMessageBytes+1)),
			want: fmt.Sprintf("supplemental_evidence.evidence exceeds %d bytes", diagnosisroom.HardMaxMessageBytes),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeDiagnosisWSClientFrame([]byte(tc.body))
			if err == nil {
				t.Fatal("decodeDiagnosisWSClientFrame returned nil error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestDiagnosisWebSocketRelayCollectsEvidencePlan(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("M", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	requiresReview := true
	evidenceRequest := ports.DiagnosisRoomEvidenceRequest{
		AlertSourceProfileID: domain.AlertSourceProfileID(4),
		Tool:                 domain.DiagnosisToolKindMetricRangeQuery,
		Reason:               "CPU and memory saturation window",
		Query:                "up",
		WindowSeconds:        300,
		StepSeconds:          60,
		Limit:                10,
	}
	collectionResult := ports.DiagnosisRoomEvidenceCollectionResult{
		Request:              evidenceRequest,
		AlertSourceProfileID: domain.AlertSourceProfileID(4),
		Tool:                 domain.DiagnosisToolKindMetricRangeQuery,
		Status:               "skipped",
		ReasonCode:           "template_query_mismatch",
		Message:              "Diagnosis tool template query does not match the requested query.",
		Query:                "up",
		WindowSeconds:        300,
		StepSeconds:          60,
		CollectedAt:          now.Add(time.Second),
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		collectState: ports.DiagnosisRoomState{
			SessionID:                 "session-1",
			ChatSessionID:             21,
			DiagnosisTaskID:           11,
			OwnerSubject:              "owner-1",
			Status:                    "open",
			TurnCount:                 2,
			StartedAt:                 now,
			LastActivityAt:            now.Add(time.Second),
			LatestConfidence:          "medium",
			LatestRequiresHumanReview: &requiresReview,
			LatestEvidenceRequests:    []ports.DiagnosisRoomEvidenceRequest{evidenceRequest},
			LatestCollectionResults:   []ports.DiagnosisRoomEvidenceCollectionResult{collectionResult},
			EvidenceTimeline: []ports.DiagnosisRoomEvidenceTimelineEntry{{
				TurnCount:         2,
				MessageID:         "collect-1",
				ActorSubject:      "owner-1",
				Trigger:           "manual_evidence_collection",
				EvidenceRequests:  []ports.DiagnosisRoomEvidenceRequest{evidenceRequest},
				CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{collectionResult},
			}},
			ConfidenceTimeline: []ports.DiagnosisRoomConfidenceTimelineEntry{{
				TurnCount:           2,
				MessageID:           "collect-1",
				OccurredAt:          now.Add(time.Second),
				Trigger:             "manual_evidence_collection",
				Confidence:          "medium",
				RequiresHumanReview: true,
				ConclusionStatus:    "needs_evidence",
				ConfidenceRationale: "Metric evidence could not be collected as requested.",
				EvidenceRequests:    []ports.DiagnosisRoomEvidenceRequest{evidenceRequest},
				CollectionResults:   []ports.DiagnosisRoomEvidenceCollectionResult{collectionResult},
				MissingEvidenceRequests: []ports.DiagnosisRoomConsultationEvidenceRequest{{
					Label:    "Metric evidence recovery",
					Detail:   "Provide verified alternative CPU and memory evidence.",
					Priority: "high",
				}},
			}},
			LatestConsultationInsight: &ports.DiagnosisRoomConsultationInsight{
				ConfidenceRationale: "Metric evidence could not be collected as requested.",
				MissingEvidenceRequests: []ports.DiagnosisRoomConsultationEvidenceRequest{{
					Label:    "Metric evidence recovery",
					Detail:   "Provide verified alternative CPU and memory evidence.",
					Priority: "high",
				}},
				ConclusionStatus: "needs_evidence",
			},
			SeenMessageIDs: []string{"collect-1"},
			Conversation: []ports.DiagnosisRoomConversationTurn{
				{Role: "user", ActorSubject: "owner-1", Content: "Run planned evidence collection."},
				{Role: "assistant", ActorSubject: "openclarion:auto-diagnosis", Content: "Metric evidence could not be collected as requested."},
			},
		},
		collectFollowUpTurns: []ports.DiagnosisRoomFollowUpTurnResult{{
			MessageID:           "collect-1/auto-evidence-1",
			UserMessage:         "OpenClarion automatic evidence follow-up.",
			AssistantMessageID:  "collect-1/auto-evidence-1/assistant",
			UserTurnID:          domain.ChatTurnID(31),
			AssistantTurnID:     domain.ChatTurnID(32),
			UserSequence:        3,
			AssistantSequence:   4,
			TurnCount:           2,
			ContextBytes:        256,
			AssistantMessage:    "Collected operator-selected evidence has been reassessed.",
			RequiresHumanReview: true,
			Confidence:          "medium",
			CollectionResults:   []ports.DiagnosisRoomEvidenceCollectionResult{collectionResult},
			ConsultationInsight: ports.DiagnosisRoomConsultationInsight{
				ConclusionStatus:    "needs_evidence",
				ConfidenceRationale: "Metric evidence still needs operator review.",
			},
			Trigger: "collected_evidence",
		}},
	}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{
		"type":       diagnosisWSClientCollectEvidence,
		"message_id": "collect-1",
		"message":    "Run planned evidence collection.",
		"evidence_requests": []map[string]any{{
			"alert_source_profile_id": 4,
			"tool":                    "metric_range_query",
			"reason":                  "CPU and memory saturation window",
			"query":                   "up",
			"window_seconds":          300,
			"step_seconds":            60,
			"limit":                   10,
		}},
	}); err != nil {
		t.Fatalf("WriteJSON collect: %v", err)
	}
	var state diagnosisWSStateFrame
	if err := conn.ReadJSON(&state); err != nil {
		t.Fatalf("ReadJSON state: %v", err)
	}
	if state.Type != diagnosisWSServerState ||
		state.ChatSessionID != 21 ||
		state.DiagnosisTaskID != 11 ||
		state.TurnCount != 2 ||
		state.Confidence != "medium" ||
		state.RequiresHumanReview == nil ||
		!*state.RequiresHumanReview ||
		len(state.EvidenceRequests) != 1 ||
		len(state.CollectionResults) != 1 ||
		state.CollectionResults[0].Status != "skipped" ||
		state.CollectionResults[0].ReasonCode != "template_query_mismatch" ||
		state.CollectionResults[0].Message != "Diagnosis tool template query does not match the requested query." ||
		state.CollectionResults[0].Request.Reason != "CPU and memory saturation window" {
		t.Fatalf("state = %+v", state)
	}
	if state.ConsultationInsight == nil ||
		state.ConsultationInsight.ConclusionStatus != "needs_evidence" ||
		state.ConsultationInsight.MissingEvidenceRequests[0].Label != "Metric evidence recovery" {
		t.Fatalf("state consultation insight = %+v", state.ConsultationInsight)
	}
	if len(state.EvidenceTimeline) != 1 ||
		state.EvidenceTimeline[0].MessageID != "collect-1" ||
		state.EvidenceTimeline[0].ActorSubject != "owner-1" ||
		state.EvidenceTimeline[0].Trigger != "manual_evidence_collection" ||
		state.EvidenceTimeline[0].CollectionResults[0].Status != "skipped" {
		t.Fatalf("state evidence timeline = %+v", state.EvidenceTimeline)
	}
	if len(state.ConfidenceTimeline) != 1 ||
		state.ConfidenceTimeline[0].MessageID != "collect-1" ||
		state.ConfidenceTimeline[0].CollectionResults[0].ReasonCode != "template_query_mismatch" ||
		state.ConfidenceTimeline[0].MissingEvidenceRequests[0].Priority != "high" {
		t.Fatalf("state confidence timeline = %+v", state.ConfidenceTimeline)
	}
	if len(state.FollowUpTurns) != 1 ||
		state.FollowUpTurns[0].MessageID != "collect-1/auto-evidence-1" ||
		state.FollowUpTurns[0].ConsultationInsight.ConclusionStatus != "needs_evidence" ||
		state.FollowUpTurns[0].Trigger != "collected_evidence" {
		t.Fatalf("state follow-up turns = %+v", state.FollowUpTurns)
	}
	if len(state.SeenMessageIDs) != 1 || state.SeenMessageIDs[0] != "collect-1" {
		t.Fatalf("state seen message ids = %+v", state.SeenMessageIDs)
	}
	collectReq, collectCalled := workflowClient.collectSnapshot()
	if collectCalled != 1 ||
		collectReq.SessionID != "session-1" ||
		collectReq.ActorSubject != "owner-1" ||
		collectReq.MessageID != "collect-1" ||
		collectReq.Message != "Run planned evidence collection." ||
		len(collectReq.Requests) != 1 ||
		collectReq.Requests[0].Tool != domain.DiagnosisToolKindMetricRangeQuery ||
		collectReq.Requests[0].AlertSourceProfileID != domain.AlertSourceProfileID(4) {
		t.Fatalf("CollectDiagnosisEvidence calls=%d request=%+v", collectCalled, collectReq)
	}
}

func TestDiagnosisWebSocketRelayReportsStillProcessingOnUpdateTimeout(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("I", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{blockSubmitUntilContextDone: true}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient, WithDiagnosisWebSocketUpdateTimeout(10*time.Millisecond)),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{
		"type":       diagnosisWSClientSubmitTurn,
		"message_id": "msg-1",
		"message":    "Please investigate",
	}); err != nil {
		t.Fatalf("WriteJSON submit: %v", err)
	}
	var frame diagnosisWSErrorFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("ReadJSON error: %v", err)
	}
	if frame.Type != diagnosisWSServerError || frame.Code != "turn_still_processing" || frame.Message != "turn is still processing" {
		t.Fatalf("error frame = %+v", frame)
	}
	submitReq, submitCalled := workflowClient.submitSnapshot()
	if submitCalled != 1 || submitReq.ActorSubject != "owner-1" {
		t.Fatalf("submit calls=%d request=%+v", submitCalled, submitReq)
	}
}

func TestDiagnosisWebSocketRelayStreamsPreviewThenReturnsAuthoritativeTurn(t *testing.T) {
	now := time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("S", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	hub := diagnosisstream.NewHub()
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		submitResult: ports.DiagnosisRoomSubmitTurnResult{
			SessionID:          "session-1",
			MessageID:          "msg-1",
			AssistantMessageID: "msg-1/assistant",
			Status:             "open",
			AssistantMessage:   "Validated final diagnosis.",
			Confidence:         "high",
		},
		submitStarted: make(chan struct{}),
		releaseSubmit: make(chan struct{}),
	}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(
			workflowClient,
			WithDiagnosisTurnStreamSource(hub),
			WithDiagnosisWebSocketUpdateTimeout(time.Second),
		),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()
	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{
		"type":       diagnosisWSClientSubmitTurn,
		"message_id": "msg-1",
		"message":    "Please investigate",
	}); err != nil {
		t.Fatalf("WriteJSON submit: %v", err)
	}
	select {
	case <-workflowClient.submitStarted:
	case <-time.After(time.Second):
		t.Fatal("SubmitDiagnosisTurn did not start")
	}
	hub.PublishDiagnosisTurnStream(ports.DiagnosisTurnStreamEvent{
		Phase:              ports.DiagnosisTurnStreamStarted,
		SessionID:          "session-1",
		MessageID:          "msg-1",
		AssistantMessageID: "msg-1/assistant",
		ActivityAttempt:    1,
	})
	hub.PublishDiagnosisTurnStream(ports.DiagnosisTurnStreamEvent{
		Phase:              ports.DiagnosisTurnStreamDelta,
		SessionID:          "session-1",
		MessageID:          "msg-1",
		AssistantMessageID: "msg-1/assistant",
		ActivityAttempt:    1,
		GenerationAttempt:  1,
		Sequence:           2,
		AssistantMessage:   "Transient draft.",
	})

	var preview diagnosisWSTurnStreamFrame
	for reads := 0; reads < 2; reads++ {
		if err := conn.ReadJSON(&preview); err != nil {
			t.Fatalf("ReadJSON preview: %v", err)
		}
		if preview.Phase == ports.DiagnosisTurnStreamDelta {
			break
		}
	}
	if preview.Type != diagnosisWSServerTurnStream || preview.Phase != ports.DiagnosisTurnStreamDelta || preview.AssistantMessage != "Transient draft." {
		t.Fatalf("preview = %+v", preview)
	}
	close(workflowClient.releaseSubmit)
	var result diagnosisWSTurnResultFrame
	if err := conn.ReadJSON(&result); err != nil {
		t.Fatalf("ReadJSON result: %v", err)
	}
	if result.Type != diagnosisWSServerTurnResult || result.AssistantMessage != "Validated final diagnosis." {
		t.Fatalf("result = %+v", result)
	}
}

func TestDiagnosisWebSocketRelayDoesNotCancelUpdateOnDisconnect(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("J", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		submitResult: ports.DiagnosisRoomSubmitTurnResult{
			SessionID:        "session-1",
			MessageID:        "msg-1",
			Status:           "open",
			AssistantMessage: "CPU alert is still firing.",
			Confidence:       "medium",
		},
		submitStarted: make(chan struct{}),
		releaseSubmit: make(chan struct{}),
		submitDone:    make(chan struct{}),
	}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient, WithDiagnosisWebSocketUpdateTimeout(time.Second)),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{
		"type":       diagnosisWSClientSubmitTurn,
		"message_id": "msg-1",
		"message":    "Please investigate",
	}); err != nil {
		t.Fatalf("WriteJSON submit: %v", err)
	}
	select {
	case <-workflowClient.submitStarted:
	case <-time.After(time.Second):
		t.Fatal("SubmitDiagnosisTurn did not start")
	}
	_ = conn.Close()
	select {
	case <-workflowClient.submitDone:
		t.Fatal("SubmitDiagnosisTurn completed before release; request context likely cancelled the update")
	case <-time.After(20 * time.Millisecond):
	}
	close(workflowClient.releaseSubmit)
	select {
	case <-workflowClient.submitDone:
	case <-time.After(time.Second):
		t.Fatal("SubmitDiagnosisTurn did not finish after release")
	}
	if err := workflowClient.submitContextErrSnapshot(); err != nil {
		t.Fatalf("submit context err = %v, want nil", err)
	}
}

func TestListEndpointRejectsOutOfRangeLimit(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/alerts?limit=0", nil)
	testOperationsReadHandler(t, &fakeUOWFactory{alertRepo: &fakeAlertRepo{}}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	var body api.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error == "" {
		t.Fatalf("error response should include a message")
	}
}

func testHandler(factory *fakeUOWFactory, opts ...ServerOption) stdhttp.Handler {
	logger := slog.New(slog.NewTextHandler(testingWriter{}, nil))
	server := NewServer(logger, factory, opts...)
	return api.HandlerWithOptions(server, api.StdHTTPServerOptions{
		ErrorHandlerFunc: OpenAPIErrorHandler(logger),
	})
}

func testConfigHandler(t *testing.T, factory *fakeUOWFactory, opts ...ServerOption) stdhttp.Handler {
	t.Helper()
	if factory.configRepo == nil {
		factory.configRepo = &fakeConfigRepo{}
	}
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   true,
			CheckedAt: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC),
		},
	}
	configOpts := testLocalRBACOptions(t, "iam-admin", authorizer)
	configOpts = append(configOpts, opts...)
	handler := testHandler(factory, configOpts...)
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Header.Get("Authorization") == "" {
			addTestLocalRBACAuthorization(r)
		}
		handler.ServeHTTP(w, r)
	})
}

func testOperationsReadHandler(t *testing.T, factory *fakeUOWFactory) stdhttp.Handler {
	t.Helper()
	authorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   true,
			CheckedAt: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC),
		},
	}
	return testOperationsReadHandlerWithAuthorizer(t, factory, authorizer)
}

func testOperationsReadHandlerWithAuthorizer(
	t *testing.T,
	factory *fakeUOWFactory,
	authorizer *fakeRBACAuthorizer,
) stdhttp.Handler {
	t.Helper()
	if factory.configRepo == nil {
		factory.configRepo = &fakeConfigRepo{}
	}
	localOpts := testLocalRBACOptions(t, "operations-viewer-1", authorizer)
	handler := testHandler(factory, localOpts...)
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Header.Get("Authorization") == "" {
			addTestLocalRBACAuthorization(r)
		}
		handler.ServeHTTP(w, r)
	})
}

const testLocalRBACAuthorization = "Bearer local-rbac-test-token"

func addTestLocalRBACAuthorization(req *stdhttp.Request) {
	req.Header.Set("Authorization", testLocalRBACAuthorization)
}

func testLocalRBACOptions(t *testing.T, subject string, authorizer RBACAuthorizer) []ServerOption {
	t.Helper()
	authProvider := authfake.New(map[string][]authfake.Result{
		testLocalRBACAuthorization: {{
			Principal: ports.AuthPrincipal{
				Subject: subject,
				Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
			},
		}},
	})
	return []ServerOption{
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("L", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithRBACAuthorizer(authorizer),
	}
}

func testSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func testHandlerWithDiagnosisWS(factory *fakeUOWFactory, opts ...ServerOption) stdhttp.Handler {
	logger := slog.New(slog.NewTextHandler(testingWriter{}, nil))
	if factory.configRepo == nil {
		factory.configRepo = &fakeConfigRepo{}
	}
	defaultAuthorizer := &fakeRBACAuthorizer{
		result: rbacusecase.AuthorizeDecision{
			Allowed:   true,
			CheckedAt: time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC),
		},
	}
	opts = append([]ServerOption{WithRBACAuthorizer(defaultAuthorizer)}, opts...)
	server := NewServer(logger, factory, opts...)
	mux := stdhttp.NewServeMux()
	server.RegisterDiagnosisWebSocketRoutes(mux)
	return api.HandlerWithOptions(server, api.StdHTTPServerOptions{
		BaseRouter:       mux,
		ErrorHandlerFunc: OpenAPIErrorHandler(logger),
	})
}

func newHTTPTestDiagnosisAuthService(t *testing.T, random *strings.Reader) diagnosisauth.Service {
	t.Helper()
	service, err := diagnosisauth.NewService(diagnosisauth.NewMemoryStore(), diagnosisauth.DefaultTicketPolicy(), random)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func newHTTPTestDiagnosisSessionIssuer(t *testing.T, now time.Time) *diagnosisauth.SessionTokenService {
	t.Helper()
	service, err := diagnosisauth.NewSessionTokenService(
		diagnosisauth.DefaultSessionTokenPolicy(strings.Repeat("S", diagnosisauth.MinSessionSigningKeyBytes)),
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewSessionTokenService: %v", err)
	}
	return service
}

func closeWebSocketDialResponse(resp *stdhttp.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

type fakeDiagnosisWeComAppCallback struct {
	echoSignature    string
	echoTimestamp    string
	echoNonce        string
	echoEncrypted    string
	echo             string
	echoErr          error
	messageSignature string
	messageTimestamp string
	messageNonce     string
	messageRawXML    []byte
	message          wecomcallback.Message
	messageErr       error
}

func (v *fakeDiagnosisWeComAppCallback) VerifyEcho(msgSignature, timestamp, nonce, echo string) (string, error) {
	v.echoSignature = msgSignature
	v.echoTimestamp = timestamp
	v.echoNonce = nonce
	v.echoEncrypted = echo
	if v.echoErr != nil {
		return "", v.echoErr
	}
	return v.echo, nil
}

func (v *fakeDiagnosisWeComAppCallback) DecryptMessage(msgSignature, timestamp, nonce string, rawXML []byte) (wecomcallback.Message, error) {
	v.messageSignature = msgSignature
	v.messageTimestamp = timestamp
	v.messageNonce = nonce
	v.messageRawXML = append([]byte(nil), rawXML...)
	if v.messageErr != nil {
		return wecomcallback.Message{}, v.messageErr
	}
	return v.message, nil
}

type fakeDiagnosisWeComAppCallbackMessageHandler struct {
	called int
	req    diagnosiswecomcallback.Request
	result diagnosiswecomcallback.Result
	err    error
}

func (h *fakeDiagnosisWeComAppCallbackMessageHandler) HandleMessage(_ context.Context, req diagnosiswecomcallback.Request) (diagnosiswecomcallback.Result, error) {
	h.called++
	h.req = req
	if h.err != nil {
		return diagnosiswecomcallback.Result{}, h.err
	}
	return h.result, nil
}

type testingWriter struct{}

func (testingWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

type fakeDiagnosisSessionResolver struct {
	sessions  map[string]diagnosisauth.SessionRef
	err       error
	called    int
	sessionID string
}

func (r *fakeDiagnosisSessionResolver) ResolveDiagnosisSession(_ context.Context, sessionID string) (diagnosisauth.SessionRef, error) {
	r.called++
	r.sessionID = sessionID
	if r.err != nil {
		return diagnosisauth.SessionRef{}, r.err
	}
	session, ok := r.sessions[sessionID]
	if !ok {
		return diagnosisauth.SessionRef{}, domain.ErrNotFound
	}
	return session, nil
}

type fakeDiagnosisWebSocketHandler struct {
	called int
	ticket diagnosisauth.Ticket
	done   chan struct{}
}

func (h *fakeDiagnosisWebSocketHandler) ServeDiagnosisWebSocket(_ context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket) {
	h.called++
	h.ticket = ticket
	_ = conn.WriteJSON(map[string]string{
		"type":    "ready",
		"subject": ticket.Subject,
	})
	if h.done != nil {
		close(h.done)
	}
}

type fakeDiagnosisRoomWorkflowClient struct {
	mu                          sync.Mutex
	submitCalled                int
	submitReq                   ports.DiagnosisRoomSubmitTurnRequest
	submitResult                ports.DiagnosisRoomSubmitTurnResult
	submitErr                   error
	blockSubmitUntilContextDone bool
	submitStarted               chan struct{}
	releaseSubmit               chan struct{}
	submitDone                  chan struct{}
	submitContextErr            error
	collectCalled               int
	collectReq                  ports.DiagnosisRoomCollectEvidenceRequest
	collectState                ports.DiagnosisRoomState
	collectFollowUpTurns        []ports.DiagnosisRoomFollowUpTurnResult
	collectErr                  error
	queryCalled                 int
	querySessionID              string
	queryState                  ports.DiagnosisRoomState
	queryErr                    error
	confirmCalled               int
	confirmReq                  ports.DiagnosisRoomConfirmConclusionRequest
	confirmState                ports.DiagnosisRoomState
	confirmErr                  error
}

type fakeDiagnosisRoomWorkflowVisibilityLookup struct {
	called   int
	requests []ports.DiagnosisRoomWorkflowVisibilityRequest
	results  map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility
	err      error
}

func (l *fakeDiagnosisRoomWorkflowVisibilityLookup) ListDiagnosisRoomWorkflowVisibility(
	_ context.Context,
	requests []ports.DiagnosisRoomWorkflowVisibilityRequest,
) (map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility, error) {
	l.called++
	l.requests = append([]ports.DiagnosisRoomWorkflowVisibilityRequest(nil), requests...)
	if l.err != nil {
		return nil, l.err
	}
	out := make(map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility, len(l.results))
	for key, value := range l.results {
		out[key] = value
	}
	return out, nil
}

type fakeDiagnosisRoomStarter struct {
	called int
	req    diagnosisroomstart.Request
	result diagnosisroomstart.Result
	err    error
}

func (s *fakeDiagnosisRoomStarter) Start(_ context.Context, req diagnosisroomstart.Request) (diagnosisroomstart.Result, error) {
	s.called++
	s.req = req
	if s.err != nil {
		return diagnosisroomstart.Result{}, s.err
	}
	return s.result, nil
}

type fakeDiagnosisRoomCloser struct {
	called int
	req    diagnosisroomclose.Request
	result diagnosisroomclose.Result
	err    error
}

func (c *fakeDiagnosisRoomCloser) CloseUnavailable(_ context.Context, req diagnosisroomclose.Request) (diagnosisroomclose.Result, error) {
	c.called++
	c.req = req
	if c.err != nil {
		return diagnosisroomclose.Result{}, c.err
	}
	return c.result, nil
}

func (c *fakeDiagnosisRoomWorkflowClient) SubmitDiagnosisTurn(ctx context.Context, req ports.DiagnosisRoomSubmitTurnRequest) (ports.DiagnosisRoomSubmitTurnResult, error) {
	c.mu.Lock()
	c.submitCalled++
	c.submitReq = req
	result := c.submitResult
	err := c.submitErr
	block := c.blockSubmitUntilContextDone
	started := c.submitStarted
	release := c.releaseSubmit
	done := c.submitDone
	c.mu.Unlock()
	if started != nil {
		close(started)
	}
	defer func() {
		if done != nil {
			close(done)
		}
	}()

	if block {
		<-ctx.Done()
		c.recordSubmitContextErr(ctx.Err())
		return ports.DiagnosisRoomSubmitTurnResult{}, ctx.Err()
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			c.recordSubmitContextErr(ctx.Err())
			return ports.DiagnosisRoomSubmitTurnResult{}, ctx.Err()
		}
	}
	if err != nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, err
	}
	return result, nil
}

func (c *fakeDiagnosisRoomWorkflowClient) QueryDiagnosisRoom(_ context.Context, sessionID string) (ports.DiagnosisRoomState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queryCalled++
	c.querySessionID = sessionID
	if c.queryErr != nil {
		return ports.DiagnosisRoomState{}, c.queryErr
	}
	return c.queryState, nil
}

func (c *fakeDiagnosisRoomWorkflowClient) CollectDiagnosisEvidence(_ context.Context, req ports.DiagnosisRoomCollectEvidenceRequest) (ports.DiagnosisRoomCollectEvidenceResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.collectCalled++
	c.collectReq = req
	if c.collectErr != nil {
		return ports.DiagnosisRoomCollectEvidenceResult{}, c.collectErr
	}
	return ports.DiagnosisRoomCollectEvidenceResult{
		State:         c.collectState,
		FollowUpTurns: append([]ports.DiagnosisRoomFollowUpTurnResult(nil), c.collectFollowUpTurns...),
	}, nil
}

func (c *fakeDiagnosisRoomWorkflowClient) ConfirmDiagnosisConclusion(_ context.Context, req ports.DiagnosisRoomConfirmConclusionRequest) (ports.DiagnosisRoomState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.confirmCalled++
	c.confirmReq = req
	if c.confirmErr != nil {
		return ports.DiagnosisRoomState{}, c.confirmErr
	}
	return c.confirmState, nil
}

func (c *fakeDiagnosisRoomWorkflowClient) submitSnapshot() (ports.DiagnosisRoomSubmitTurnRequest, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.submitReq, c.submitCalled
}

func (c *fakeDiagnosisRoomWorkflowClient) querySnapshot() (string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.querySessionID, c.queryCalled
}

func (c *fakeDiagnosisRoomWorkflowClient) collectSnapshot() (ports.DiagnosisRoomCollectEvidenceRequest, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.collectReq, c.collectCalled
}

func (c *fakeDiagnosisRoomWorkflowClient) confirmSnapshot() (ports.DiagnosisRoomConfirmConclusionRequest, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.confirmReq, c.confirmCalled
}

func (c *fakeDiagnosisRoomWorkflowClient) recordSubmitContextErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.submitContextErr = err
}

func (c *fakeDiagnosisRoomWorkflowClient) submitContextErrSnapshot() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.submitContextErr
}

type neverAuthProvider struct{}

func (*neverAuthProvider) AuthenticateAuthorization(context.Context, string) (ports.AuthPrincipal, error) {
	return ports.AuthPrincipal{}, errors.New("unexpected auth provider call")
}

type auxiliaryCredentialAuthProvider struct {
	calls         int
	authorization string
	credentials   ports.AuthAuxiliaryCredentials
	principal     ports.AuthPrincipal
	err           error
}

func (p *auxiliaryCredentialAuthProvider) AuthenticateAuthorization(context.Context, string) (ports.AuthPrincipal, error) {
	return ports.AuthPrincipal{}, errors.New("unexpected plain auth provider call")
}

func (p *auxiliaryCredentialAuthProvider) AuthenticateAuthorizationWithAuxiliaryCredentials(_ context.Context, authorization string, credentials ports.AuthAuxiliaryCredentials) (ports.AuthPrincipal, error) {
	p.calls++
	p.authorization = authorization
	p.credentials = credentials
	if p.err != nil {
		return ports.AuthPrincipal{}, p.err
	}
	return p.principal, nil
}

type roleMappingStatusAuthProvider struct {
	status          ports.AuthRoleMappingStatus
	transportStatus ports.AuthTransportPolicyStatus
}

func (p roleMappingStatusAuthProvider) AuthenticateAuthorization(context.Context, string) (ports.AuthPrincipal, error) {
	return ports.AuthPrincipal{}, errors.New("unexpected auth provider call")
}

func (p roleMappingStatusAuthProvider) RoleMappingStatus() ports.AuthRoleMappingStatus {
	return ports.AuthRoleMappingStatus{
		OwnerMappingCount: p.status.OwnerMappingCount,
		AdminMappingCount: p.status.AdminMappingCount,
		DefaultRoles:      append([]ports.AuthRole(nil), p.status.DefaultRoles...),
	}
}

func (p roleMappingStatusAuthProvider) TransportPolicyStatus() ports.AuthTransportPolicyStatus {
	return p.transportStatus
}

type fakeUOWFactory struct {
	alertRepo     *fakeAlertRepo
	evidenceRepo  *fakeEvidenceRepo
	diagnosisRepo *fakeDiagnosisRepo
	reportRepo    *fakeReportRepo
	configRepo    *fakeConfigRepo
	err           error
}

type fakeReportReplayTrigger struct {
	called int
	req    reporttrigger.Request
	result reporttrigger.Result
	err    error
}

func (t *fakeReportReplayTrigger) ReplayAndStart(_ context.Context, req reporttrigger.Request) (reporttrigger.Result, error) {
	t.called++
	t.req = req
	if t.err != nil {
		return reporttrigger.Result{}, t.err
	}
	return t.result, nil
}

type fakeReportNotificationSender struct {
	called int
	req    reportnotification.Request
	result reportnotification.Result
	err    error
}

func (s *fakeReportNotificationSender) Send(_ context.Context, req reportnotification.Request) (reportnotification.Result, error) {
	s.called++
	s.req = req
	if s.err != nil {
		return reportnotification.Result{}, s.err
	}
	return s.result, nil
}

type fakeDiagnosisNotificationRetrier struct {
	called int
	req    diagnosisnotification.Request
	result diagnosisnotification.Result
	err    error
}

func (r *fakeDiagnosisNotificationRetrier) Retry(_ context.Context, req diagnosisnotification.Request) (diagnosisnotification.Result, error) {
	r.called++
	r.req = req
	if r.err != nil {
		return diagnosisnotification.Result{}, r.err
	}
	return r.result, nil
}

type fakeReportWorkflowPolicyReplayTrigger struct {
	called int
	req    reportpolicytrigger.Request
	result reporttrigger.Result
	err    error
}

func (t *fakeReportWorkflowPolicyReplayTrigger) ReplayAndStart(_ context.Context, req reportpolicytrigger.Request) (reporttrigger.Result, error) {
	t.called++
	t.req = req
	if t.err != nil {
		return reporttrigger.Result{}, t.err
	}
	return t.result, nil
}

type fakeDetailedReportWorkflowPolicyReplayTrigger struct {
	legacyCalled   int
	detailedCalled int
	req            reportpolicytrigger.Request
	result         reportpolicytrigger.Result
	err            error
}

func (t *fakeDetailedReportWorkflowPolicyReplayTrigger) ReplayAndStart(_ context.Context, req reportpolicytrigger.Request) (reporttrigger.Result, error) {
	t.legacyCalled++
	t.req = req
	return reporttrigger.Result{}, errors.New("legacy policy replay should not be called")
}

func (t *fakeDetailedReportWorkflowPolicyReplayTrigger) ReplayAndStartDetailed(_ context.Context, req reportpolicytrigger.Request) (reportpolicytrigger.Result, error) {
	t.detailedCalled++
	t.req = req
	if t.err != nil {
		return reportpolicytrigger.Result{}, t.err
	}
	return t.result, nil
}

type fakeAlertmanagerWebhookIngestor struct {
	called int
	req    alertmanagerwebhook.Request
	result alertmanagerwebhook.Result
	err    error
}

func (i *fakeAlertmanagerWebhookIngestor) Ingest(_ context.Context, req alertmanagerwebhook.Request) (alertmanagerwebhook.Result, error) {
	i.called++
	i.req = req
	if i.err != nil {
		return alertmanagerwebhook.Result{}, i.err
	}
	return i.result, nil
}

type fakeAlertSourceConnectionTester struct {
	called  int
	profile domain.AlertSourceProfile
	result  alertsourcecheck.Result
	err     error
}

func (t *fakeAlertSourceConnectionTester) TestAlertSourceConnection(_ context.Context, profile domain.AlertSourceProfile) (alertsourcecheck.Result, error) {
	t.called++
	t.profile = profile
	if t.err != nil {
		return alertsourcecheck.Result{}, t.err
	}
	return t.result, nil
}

type fakeNotificationChannelTester struct {
	called  int
	profile domain.NotificationChannelProfile
	request notificationchannelcheck.Request
	result  notificationchannelcheck.Result
	err     error
}

func (t *fakeNotificationChannelTester) TestNotificationChannel(_ context.Context, profile domain.NotificationChannelProfile, reqs ...notificationchannelcheck.Request) (notificationchannelcheck.Result, error) {
	t.called++
	t.profile = profile
	if len(reqs) > 0 {
		t.request = reqs[0]
	}
	if t.err != nil {
		return notificationchannelcheck.Result{}, t.err
	}
	return t.result, nil
}

type fakeDirectorySyncer struct {
	called int
	req    directorysync.SyncRequest
	result directorysync.Result
	err    error
}

func (s *fakeDirectorySyncer) Sync(_ context.Context, req directorysync.SyncRequest) (directorysync.Result, error) {
	s.called++
	s.req = req
	if s.err != nil {
		return directorysync.Result{}, s.err
	}
	return s.result, nil
}

type fakeRBACAuthorizer struct {
	called    int
	req       rbacusecase.AuthorizeRequest
	requests  []rbacusecase.AuthorizeRequest
	result    rbacusecase.AuthorizeDecision
	err       error
	authorize func(rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error)
}

func (a *fakeRBACAuthorizer) Authorize(_ context.Context, req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error) {
	a.called++
	a.req = req
	a.requests = append(a.requests, req)
	if a.authorize != nil {
		return a.authorize(req)
	}
	if a.err != nil {
		return rbacusecase.AuthorizeDecision{}, a.err
	}
	return a.result, nil
}

func (f *fakeUOWFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return f.fakeUOW(), f.err
}

func (f *fakeUOWFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(ctx, f.fakeUOW())
}

func (f *fakeUOWFactory) fakeUOW() *fakeUOW {
	var diagnosisRepo ports.DiagnosisRepository
	if f.diagnosisRepo != nil {
		diagnosisRepo = f.diagnosisRepo
	}
	return &fakeUOW{alertRepo: f.alertRepo, evidenceRepo: f.evidenceRepo, diagnosisRepo: diagnosisRepo, reportRepo: f.reportRepo, configRepo: f.configRepo}
}

type fakeUOW struct {
	ports.UnitOfWork
	alertRepo     ports.AlertRepository
	evidenceRepo  ports.EvidenceRepository
	diagnosisRepo ports.DiagnosisRepository
	reportRepo    ports.ReportRepository
	configRepo    ports.ConfigurationRepository
}

func (u *fakeUOW) Alerts() ports.AlertRepository {
	return u.alertRepo
}

func (u *fakeUOW) Evidence() ports.EvidenceRepository {
	return u.evidenceRepo
}

func (u *fakeUOW) Diagnosis() ports.DiagnosisRepository {
	return u.diagnosisRepo
}

func (u *fakeUOW) Reports() ports.ReportRepository {
	return u.reportRepo
}

func (u *fakeUOW) Config() ports.ConfigurationRepository {
	return u.configRepo
}

func (u *fakeUOW) Commit(context.Context) error {
	return nil
}

func (u *fakeUOW) Rollback(context.Context) error {
	return nil
}

type fakeAlertRepo struct {
	ports.AlertRepository
	events    []domain.AlertEvent
	lastLimit int
}

func (r *fakeAlertRepo) ListEvents(_ context.Context, limit int) ([]domain.AlertEvent, error) {
	r.lastLimit = limit
	if limit > len(r.events) {
		limit = len(r.events)
	}
	return r.events[:limit], nil
}

func (r *fakeAlertRepo) ListEventsFiltered(_ context.Context, filter ports.AlertEventFilter, limit int) ([]domain.AlertEvent, error) {
	r.lastLimit = limit
	allowedProfiles := map[domain.AlertSourceProfileID]struct{}{}
	for _, id := range filter.AlertSourceProfileIDs {
		if id > 0 {
			allowedProfiles[id] = struct{}{}
		}
	}
	out := make([]domain.AlertEvent, 0, len(r.events))
	for _, event := range r.events {
		if len(allowedProfiles) > 0 {
			if _, ok := allowedProfiles[event.AlertSourceProfileID]; !ok {
				continue
			}
		}
		out = append(out, event)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

type fakeEvidenceRepo struct {
	ports.EvidenceRepository
	snapshots []domain.EvidenceSnapshot
	lastLimit int
}

func (r *fakeEvidenceRepo) List(_ context.Context, limit int) ([]domain.EvidenceSnapshot, error) {
	r.lastLimit = limit
	if limit > len(r.snapshots) {
		limit = len(r.snapshots)
	}
	return r.snapshots[:limit], nil
}

type fakeDiagnosisRepo struct {
	ports.DiagnosisRepository
	tasksBySnapshot        map[domain.EvidenceSnapshotID][]domain.DiagnosisTask
	eventsByTaskAndKind    map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent
	chatTurnsBySession     map[domain.ChatSessionID][]domain.ChatTurn
	chatSessionSummaries   map[domain.ChatSessionID]domain.ChatSessionSummary
	chatApprovalsByKey     map[string][]domain.ChatSessionApproval
	chatSessions           []domain.ChatSessionWithTask
	lastSnapshotID         domain.EvidenceSnapshotID
	lastTaskLimit          int
	lastTaskID             domain.DiagnosisTaskID
	lastFindTaskID         domain.DiagnosisTaskID
	lastEventKind          string
	lastEventLimit         int
	lastChatSessionLimit   int
	lastChatSessionOffset  int
	chatSessionPageCalls   []chatSessionPageCall
	lastChatSessionKey     string
	lastChatTurnSession    domain.ChatSessionID
	lastChatTurnLimit      int
	listChatApprovalsCalls int
}

type chatSessionPageCall struct {
	limit  int
	offset int
}

func (r *fakeDiagnosisRepo) ListTasksByEvidenceSnapshot(_ context.Context, snapshotID domain.EvidenceSnapshotID, limit int) ([]domain.DiagnosisTask, error) {
	r.lastSnapshotID = snapshotID
	r.lastTaskLimit = limit
	tasks := r.tasksBySnapshot[snapshotID]
	if limit > len(tasks) {
		limit = len(tasks)
	}
	return tasks[:limit], nil
}

func (r *fakeDiagnosisRepo) FindTaskByID(_ context.Context, id domain.DiagnosisTaskID) (domain.DiagnosisTask, error) {
	r.lastFindTaskID = id
	for _, item := range r.chatSessions {
		if item.Task.ID == id {
			return item.Task, nil
		}
	}
	for _, tasks := range r.tasksBySnapshot {
		for _, task := range tasks {
			if task.ID == id {
				return task, nil
			}
		}
	}
	return domain.DiagnosisTask{}, fmt.Errorf("diagnosis task %d: %w", id, domain.ErrNotFound)
}

func (r *fakeDiagnosisRepo) ListEventsByTaskAndKind(_ context.Context, taskID domain.DiagnosisTaskID, kind string, limit int) ([]domain.DiagnosisTaskEvent, error) {
	r.lastTaskID = taskID
	r.lastEventKind = kind
	r.lastEventLimit = limit
	if r.eventsByTaskAndKind == nil {
		return nil, nil
	}
	events := r.eventsByTaskAndKind[taskID][kind]
	if limit > len(events) {
		limit = len(events)
	}
	return events[:limit], nil
}

func (r *fakeDiagnosisRepo) ListChatSessions(ctx context.Context, limit int) ([]domain.ChatSessionWithTask, error) {
	return r.ListChatSessionsPage(ctx, limit, 0)
}

func (r *fakeDiagnosisRepo) ListChatSessionsPage(_ context.Context, limit int, offset int) ([]domain.ChatSessionWithTask, error) {
	r.lastChatSessionLimit = limit
	r.lastChatSessionOffset = offset
	r.chatSessionPageCalls = append(r.chatSessionPageCalls, chatSessionPageCall{limit: limit, offset: offset})
	if offset >= len(r.chatSessions) {
		return nil, nil
	}
	sessions := r.chatSessions[offset:]
	if limit > len(sessions) {
		limit = len(sessions)
	}
	return sessions[:limit], nil
}

func (r *fakeDiagnosisRepo) FindChatSessionByKey(_ context.Context, sessionKey string) (domain.ChatSession, error) {
	r.lastChatSessionKey = sessionKey
	for _, item := range r.chatSessions {
		if item.Session.SessionKey == sessionKey {
			return item.Session, nil
		}
	}
	return domain.ChatSession{}, fmt.Errorf("chat session %q: %w", sessionKey, domain.ErrNotFound)
}

func (r *fakeDiagnosisRepo) ListChatTurnsBySession(_ context.Context, sessionID domain.ChatSessionID, limit int) ([]domain.ChatTurn, error) {
	r.lastChatTurnSession = sessionID
	r.lastChatTurnLimit = limit
	if r.chatTurnsBySession == nil {
		return nil, nil
	}
	turns := r.chatTurnsBySession[sessionID]
	if limit > len(turns) {
		limit = len(turns)
	}
	return turns[:limit], nil
}

func (r *fakeDiagnosisRepo) FindLatestChatSessionSummary(_ context.Context, sessionID domain.ChatSessionID) (domain.ChatSessionSummary, error) {
	if summary, ok := r.chatSessionSummaries[sessionID]; ok {
		return summary, nil
	}
	return domain.ChatSessionSummary{}, domain.ErrNotFound
}

func chatApprovalTestKey(sessionID domain.ChatSessionID, digest string) string {
	return fmt.Sprintf("%d/%s", sessionID, digest)
}

func (r *fakeDiagnosisRepo) HasChatSessionApprovals(_ context.Context, sessionID domain.ChatSessionID) (bool, error) {
	prefix := fmt.Sprintf("%d/", sessionID)
	for key, items := range r.chatApprovalsByKey {
		if strings.HasPrefix(key, prefix) && len(items) > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeDiagnosisRepo) ListChatSessionApprovals(_ context.Context, sessionID domain.ChatSessionID, digest string, limit int) ([]domain.ChatSessionApproval, error) {
	r.listChatApprovalsCalls++
	items := r.chatApprovalsByKey[chatApprovalTestKey(sessionID, digest)]
	if limit > len(items) {
		limit = len(items)
	}
	return append([]domain.ChatSessionApproval(nil), items[:limit]...), nil
}

type fakeReportRepo struct {
	ports.ReportRepository
	finalReports        []domain.FinalReport
	linkedSubReports    map[domain.FinalReportID][]domain.SubReport
	deliveriesByReport  map[domain.FinalReportID][]domain.ReportNotificationDelivery
	lastListLimit       int
	lastSubReportsLimit int
	lastDeliveryLimit   int
}

type fakeConfigRepo struct {
	ports.ConfigurationRepository
	alertSourceProfiles                []domain.AlertSourceProfile
	saveResult                         domain.AlertSourceProfile
	updateResult                       domain.AlertSourceProfile
	saved                              domain.AlertSourceProfile
	updated                            domain.AlertSourceProfile
	groupingPolicies                   []domain.GroupingPolicy
	saveGroupingPolicyResult           domain.GroupingPolicy
	updateGroupingPolicyResult         domain.GroupingPolicy
	savedGroupingPolicy                domain.GroupingPolicy
	updatedGroupingPolicy              domain.GroupingPolicy
	reportWorkflowPolicies             []domain.ReportWorkflowPolicy
	saveReportWorkflowPolicyResult     domain.ReportWorkflowPolicy
	updateReportWorkflowPolicyResult   domain.ReportWorkflowPolicy
	savedReportWorkflowPolicy          domain.ReportWorkflowPolicy
	updatedReportWorkflowPolicy        domain.ReportWorkflowPolicy
	reportWorkflowSchedules            []domain.ReportWorkflowSchedule
	saveReportWorkflowScheduleResult   domain.ReportWorkflowSchedule
	updateReportWorkflowScheduleResult domain.ReportWorkflowSchedule
	savedReportWorkflowSchedule        domain.ReportWorkflowSchedule
	updatedReportWorkflowSchedule      domain.ReportWorkflowSchedule
	notificationChannelProfiles        []domain.NotificationChannelProfile
	saveNotificationChannelResult      domain.NotificationChannelProfile
	updateNotificationChannelResult    domain.NotificationChannelProfile
	savedNotificationChannel           domain.NotificationChannelProfile
	updatedNotificationChannel         domain.NotificationChannelProfile
	savedNotificationChannelTestProofs []domain.NotificationChannelTestProof
	directoryDepartments               []domain.DirectoryDepartment
	directorySyncRuns                  []domain.DirectorySyncRun
	directoryUsers                     []domain.DirectoryUser
	rbacAssignments                    []domain.RBACAssignment
	upsertRBACAssignmentResult         domain.RBACAssignment
	savedRBACAssignment                domain.RBACAssignment
	saveErr                            error
	updateErr                          error
	lastListLimit                      int
	lastDirectoryProvider              string
	lastDirectorySubject               string
	lastDirectoryLimit                 int
	lastRBACAssignmentLimit            int
}

type recordingReportWorkflowScheduleSyncer struct {
	calls    int
	schedule domain.ReportWorkflowSchedule
	err      error
}

func (r *recordingReportWorkflowScheduleSyncer) SyncReportWorkflowSchedule(_ context.Context, schedule domain.ReportWorkflowSchedule) error {
	r.calls++
	r.schedule = schedule
	return r.err
}

func (r *fakeConfigRepo) ListAlertSourceProfiles(_ context.Context, limit int) ([]domain.AlertSourceProfile, error) {
	r.lastListLimit = limit
	if limit > len(r.alertSourceProfiles) {
		limit = len(r.alertSourceProfiles)
	}
	return r.alertSourceProfiles[:limit], nil
}

func (r *fakeConfigRepo) SaveAlertSourceProfile(_ context.Context, profile domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	r.saved = profile
	if r.saveErr != nil {
		return domain.AlertSourceProfile{}, r.saveErr
	}
	return r.saveResult, nil
}

func (r *fakeConfigRepo) UpdateAlertSourceProfile(_ context.Context, profile domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	r.updated = profile
	if r.updateErr != nil {
		return domain.AlertSourceProfile{}, r.updateErr
	}
	return r.updateResult, nil
}

func (r *fakeConfigRepo) FindAlertSourceProfileByID(_ context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	for _, profile := range r.alertSourceProfiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return domain.AlertSourceProfile{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) ListGroupingPolicies(_ context.Context, limit int) ([]domain.GroupingPolicy, error) {
	r.lastListLimit = limit
	if limit > len(r.groupingPolicies) {
		limit = len(r.groupingPolicies)
	}
	return r.groupingPolicies[:limit], nil
}

func (r *fakeConfigRepo) SaveGroupingPolicy(_ context.Context, policy domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	r.savedGroupingPolicy = policy
	if r.saveErr != nil {
		return domain.GroupingPolicy{}, r.saveErr
	}
	return r.saveGroupingPolicyResult, nil
}

func (r *fakeConfigRepo) UpdateGroupingPolicy(_ context.Context, policy domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	r.updatedGroupingPolicy = policy
	if r.updateErr != nil {
		return domain.GroupingPolicy{}, r.updateErr
	}
	return r.updateGroupingPolicyResult, nil
}

func (r *fakeConfigRepo) FindGroupingPolicyByID(_ context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	for _, policy := range r.groupingPolicies {
		if policy.ID == id {
			return policy, nil
		}
	}
	return domain.GroupingPolicy{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) ListReportWorkflowPolicies(_ context.Context, limit int) ([]domain.ReportWorkflowPolicy, error) {
	r.lastListLimit = limit
	if limit > len(r.reportWorkflowPolicies) {
		limit = len(r.reportWorkflowPolicies)
	}
	return r.reportWorkflowPolicies[:limit], nil
}

func (r *fakeConfigRepo) SaveReportWorkflowPolicy(_ context.Context, policy domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	r.savedReportWorkflowPolicy = policy
	if r.saveErr != nil {
		return domain.ReportWorkflowPolicy{}, r.saveErr
	}
	if r.saveReportWorkflowPolicyResult.ID != 0 {
		r.reportWorkflowPolicies = append(r.reportWorkflowPolicies, r.saveReportWorkflowPolicyResult)
		return r.saveReportWorkflowPolicyResult, nil
	}
	policy.ID = domain.ReportWorkflowPolicyID(len(r.reportWorkflowPolicies) + 1)
	r.reportWorkflowPolicies = append(r.reportWorkflowPolicies, policy)
	return policy, nil
}

func (r *fakeConfigRepo) UpdateReportWorkflowPolicy(_ context.Context, policy domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	r.updatedReportWorkflowPolicy = policy
	if r.updateErr != nil {
		return domain.ReportWorkflowPolicy{}, r.updateErr
	}
	for i, existing := range r.reportWorkflowPolicies {
		if existing.ID == policy.ID {
			if r.updateReportWorkflowPolicyResult.ID != 0 {
				r.reportWorkflowPolicies[i] = r.updateReportWorkflowPolicyResult
				return r.updateReportWorkflowPolicyResult, nil
			}
			r.reportWorkflowPolicies[i] = policy
			return policy, nil
		}
	}
	return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) FindReportWorkflowPolicyByID(_ context.Context, id domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	for _, policy := range r.reportWorkflowPolicies {
		if policy.ID == id {
			return policy, nil
		}
	}
	return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) ListReportWorkflowSchedules(_ context.Context, limit int) ([]domain.ReportWorkflowSchedule, error) {
	r.lastListLimit = limit
	if limit > len(r.reportWorkflowSchedules) {
		limit = len(r.reportWorkflowSchedules)
	}
	return r.reportWorkflowSchedules[:limit], nil
}

func (r *fakeConfigRepo) SaveReportWorkflowSchedule(_ context.Context, schedule domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	r.savedReportWorkflowSchedule = schedule
	if r.saveErr != nil {
		return domain.ReportWorkflowSchedule{}, r.saveErr
	}
	if r.saveReportWorkflowScheduleResult.ID != 0 {
		r.reportWorkflowSchedules = append(r.reportWorkflowSchedules, r.saveReportWorkflowScheduleResult)
		return r.saveReportWorkflowScheduleResult, nil
	}
	schedule.ID = domain.ReportWorkflowScheduleID(len(r.reportWorkflowSchedules) + 1)
	r.reportWorkflowSchedules = append(r.reportWorkflowSchedules, schedule)
	return schedule, nil
}

func (r *fakeConfigRepo) UpdateReportWorkflowSchedule(_ context.Context, schedule domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	r.updatedReportWorkflowSchedule = schedule
	if r.updateErr != nil {
		return domain.ReportWorkflowSchedule{}, r.updateErr
	}
	for i, existing := range r.reportWorkflowSchedules {
		if existing.ID == schedule.ID {
			if r.updateReportWorkflowScheduleResult.ID != 0 {
				r.reportWorkflowSchedules[i] = r.updateReportWorkflowScheduleResult
				return r.updateReportWorkflowScheduleResult, nil
			}
			r.reportWorkflowSchedules[i] = schedule
			return schedule, nil
		}
	}
	return domain.ReportWorkflowSchedule{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) FindReportWorkflowScheduleByID(_ context.Context, id domain.ReportWorkflowScheduleID) (domain.ReportWorkflowSchedule, error) {
	for _, schedule := range r.reportWorkflowSchedules {
		if schedule.ID == id {
			return schedule, nil
		}
	}
	return domain.ReportWorkflowSchedule{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) ListNotificationChannelProfiles(_ context.Context, limit int) ([]domain.NotificationChannelProfile, error) {
	r.lastListLimit = limit
	if limit > len(r.notificationChannelProfiles) {
		limit = len(r.notificationChannelProfiles)
	}
	return r.notificationChannelProfiles[:limit], nil
}

func (r *fakeConfigRepo) SaveNotificationChannelProfile(_ context.Context, profile domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	r.savedNotificationChannel = profile
	if r.saveErr != nil {
		return domain.NotificationChannelProfile{}, r.saveErr
	}
	if r.saveNotificationChannelResult.ID != 0 {
		r.notificationChannelProfiles = append(r.notificationChannelProfiles, r.saveNotificationChannelResult)
		return r.saveNotificationChannelResult, nil
	}
	profile.ID = domain.NotificationChannelProfileID(len(r.notificationChannelProfiles) + 1)
	r.notificationChannelProfiles = append(r.notificationChannelProfiles, profile)
	return profile, nil
}

func (r *fakeConfigRepo) UpdateNotificationChannelProfile(_ context.Context, profile domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	r.updatedNotificationChannel = profile
	if r.updateErr != nil {
		return domain.NotificationChannelProfile{}, r.updateErr
	}
	for i, existing := range r.notificationChannelProfiles {
		if existing.ID == profile.ID {
			if r.updateNotificationChannelResult.ID != 0 {
				r.notificationChannelProfiles[i] = r.updateNotificationChannelResult
				return r.updateNotificationChannelResult, nil
			}
			r.notificationChannelProfiles[i] = profile
			return profile, nil
		}
	}
	return domain.NotificationChannelProfile{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) FindNotificationChannelProfileByID(_ context.Context, id domain.NotificationChannelProfileID) (domain.NotificationChannelProfile, error) {
	for _, profile := range r.notificationChannelProfiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return domain.NotificationChannelProfile{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) SaveNotificationChannelTestProof(_ context.Context, proof domain.NotificationChannelTestProof) (domain.NotificationChannelTestProof, error) {
	if r.saveErr != nil {
		return domain.NotificationChannelTestProof{}, r.saveErr
	}
	proof.ID = domain.NotificationChannelTestProofID(len(r.savedNotificationChannelTestProofs) + 1)
	r.savedNotificationChannelTestProofs = append(r.savedNotificationChannelTestProofs, proof)
	return proof, nil
}

func (r *fakeConfigRepo) ListLatestNotificationChannelTestProofs(_ context.Context, profileID domain.NotificationChannelProfileID, limit int) ([]domain.NotificationChannelTestProof, error) {
	out := []domain.NotificationChannelTestProof{}
	for _, proof := range r.savedNotificationChannelTestProofs {
		if proof.NotificationChannelProfileID == profileID {
			out = append(out, proof)
		}
	}
	if limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (r *fakeConfigRepo) ListDirectoryDepartments(_ context.Context, provider string, limit int) ([]domain.DirectoryDepartment, error) {
	r.lastDirectoryProvider = provider
	r.lastDirectoryLimit = limit
	out := make([]domain.DirectoryDepartment, 0, len(r.directoryDepartments))
	for _, department := range r.directoryDepartments {
		if provider == "" || department.Provider == provider {
			out = append(out, department)
		}
	}
	if limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (r *fakeConfigRepo) ListDirectoryUsers(_ context.Context, provider string, limit int) ([]domain.DirectoryUser, error) {
	r.lastDirectoryProvider = provider
	r.lastDirectoryLimit = limit
	out := make([]domain.DirectoryUser, 0, len(r.directoryUsers))
	for _, user := range r.directoryUsers {
		if provider == "" || user.Provider == provider {
			out = append(out, user)
		}
	}
	if limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (r *fakeConfigRepo) SaveDirectorySyncRun(_ context.Context, run domain.DirectorySyncRun) (domain.DirectorySyncRun, error) {
	run.ID = domain.DirectorySyncRunID(len(r.directorySyncRuns) + 1)
	r.directorySyncRuns = append(r.directorySyncRuns, run)
	return run, nil
}

func (r *fakeConfigRepo) ListDirectorySyncRuns(_ context.Context, provider string, limit int) ([]domain.DirectorySyncRun, error) {
	r.lastDirectoryProvider = provider
	r.lastDirectoryLimit = limit
	out := make([]domain.DirectorySyncRun, 0, len(r.directorySyncRuns))
	for _, run := range r.directorySyncRuns {
		if provider == "" || run.Provider == provider {
			out = append(out, run)
		}
	}
	if limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (r *fakeConfigRepo) ListDirectoryUsersBySubject(_ context.Context, subject string, limit int) ([]domain.DirectoryUser, error) {
	r.lastDirectorySubject = subject
	r.lastDirectoryLimit = limit
	out := make([]domain.DirectoryUser, 0, len(r.directoryUsers))
	for _, user := range r.directoryUsers {
		if user.Subject == subject {
			out = append(out, user)
		}
	}
	if limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (r *fakeConfigRepo) ListDirectoryUsersByExternalID(_ context.Context, externalID string, limit int) ([]domain.DirectoryUser, error) {
	r.lastDirectorySubject = externalID
	r.lastDirectoryLimit = limit
	out := make([]domain.DirectoryUser, 0, len(r.directoryUsers))
	for _, user := range r.directoryUsers {
		if user.ExternalID == externalID {
			out = append(out, user)
		}
	}
	if limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (r *fakeConfigRepo) FindDirectoryDepartmentByProviderExternalID(_ context.Context, provider, externalID string) (domain.DirectoryDepartment, error) {
	for _, department := range r.directoryDepartments {
		if department.Provider == provider && department.ExternalID == externalID {
			return department, nil
		}
	}
	return domain.DirectoryDepartment{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) FindDirectoryUserByProviderSubject(_ context.Context, provider, subject string) (domain.DirectoryUser, error) {
	for _, user := range r.directoryUsers {
		if user.Provider == provider && user.Subject == subject {
			return user, nil
		}
	}
	return domain.DirectoryUser{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) UpsertRBACAssignment(_ context.Context, assignment domain.RBACAssignment) (domain.RBACAssignment, error) {
	r.savedRBACAssignment = assignment
	if r.saveErr != nil {
		return domain.RBACAssignment{}, r.saveErr
	}
	if r.upsertRBACAssignmentResult.ID == 0 {
		assignment.ID = domain.RBACAssignmentID(len(r.rbacAssignments) + 1)
		now := time.Now().UTC()
		assignment.CreatedAt = now
		assignment.UpdatedAt = now
		r.rbacAssignments = append(r.rbacAssignments, assignment)
		return assignment, nil
	}
	r.rbacAssignments = append(r.rbacAssignments, r.upsertRBACAssignmentResult)
	return r.upsertRBACAssignmentResult, nil
}

func (r *fakeConfigRepo) ListRBACAssignments(_ context.Context, limit int) ([]domain.RBACAssignment, error) {
	r.lastRBACAssignmentLimit = limit
	if limit > len(r.rbacAssignments) {
		limit = len(r.rbacAssignments)
	}
	return r.rbacAssignments[:limit], nil
}

func (r *fakeConfigRepo) ListRBACAssignmentsForPrincipal(_ context.Context, subject string, departmentKeys []string, limit int) ([]domain.RBACAssignment, error) {
	out := make([]domain.RBACAssignment, 0, len(r.rbacAssignments))
	for _, assignment := range r.rbacAssignments {
		if !assignment.Enabled {
			continue
		}
		switch assignment.SubjectKind {
		case domain.RBACSubjectKindUser:
			if assignment.SubjectKey == subject {
				out = append(out, assignment)
			}
		case domain.RBACSubjectKindDepartment:
			if slices.Contains(departmentKeys, assignment.SubjectKey) {
				out = append(out, assignment)
			}
		}
	}
	if limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (r *fakeReportRepo) ListFinalReports(_ context.Context, limit int) ([]domain.FinalReport, error) {
	r.lastListLimit = limit
	if limit > len(r.finalReports) {
		limit = len(r.finalReports)
	}
	return r.finalReports[:limit], nil
}

func (r *fakeReportRepo) FindFinalReportByID(_ context.Context, id domain.FinalReportID) (domain.FinalReport, error) {
	for _, report := range r.finalReports {
		if report.ID == id {
			return report, nil
		}
	}
	return domain.FinalReport{}, domain.ErrNotFound
}

func (r *fakeReportRepo) ListSubReportsForFinalReport(_ context.Context, finalReportID domain.FinalReportID, limit int) ([]domain.SubReport, error) {
	r.lastSubReportsLimit = limit
	if _, err := r.FindFinalReportByID(context.Background(), finalReportID); err != nil {
		return nil, err
	}
	reports := r.linkedSubReports[finalReportID]
	if limit > len(reports) {
		limit = len(reports)
	}
	return reports[:limit], nil
}

func (r *fakeReportRepo) ListNotificationDeliveriesByFinalReport(_ context.Context, finalReportID domain.FinalReportID, limit int) ([]domain.ReportNotificationDelivery, error) {
	r.lastDeliveryLimit = limit
	deliveries := r.deliveriesByReport[finalReportID]
	if limit > len(deliveries) {
		limit = len(deliveries)
	}
	return deliveries[:limit], nil
}
