package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/tenantops"
)

func TestTenantAPIListsAccessibleAndManagesMemberships(t *testing.T) {
	t.Parallel()

	registry := newHTTPTestTenantRegistry()
	service, err := tenantops.NewService(registry)
	if err != nil {
		t.Fatalf("tenantops.NewService: %v", err)
	}
	authorizer := &fakeRBACAuthorizer{}
	opts := testLocalRBACOptions(t, "owner-1", authorizer)
	opts = append(opts, WithTenantOperations(service))
	handler := testHandler(&fakeUOWFactory{}, opts...)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/tenants", nil)
	addTestLocalRBACAuthorization(request)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var list api.TenantListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 2 || list.Items[0].Key != domain.DefaultTenantKey || list.Items[1].Key != "platform" {
		t.Fatalf("tenant list = %+v", list.Items)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/tenants/2/memberships", strings.NewReader(`{"subject":"operator-2","role":"member","enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	addTestLocalRBACAuthorization(request)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("membership status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var membership api.TenantMembership
	if err := json.NewDecoder(recorder.Body).Decode(&membership); err != nil {
		t.Fatalf("decode membership: %v", err)
	}
	if membership.Subject != "operator-2" || membership.Role != string(domain.TenantMembershipRoleMember) || !membership.Enabled {
		t.Fatalf("membership = %+v", membership)
	}
}

func TestTenantAPICreateRequiresBootstrapAdmin(t *testing.T) {
	t.Parallel()

	registry := newHTTPTestTenantRegistry()
	service, _ := tenantops.NewService(registry)
	authorizer := &fakeRBACAuthorizer{}
	opts := testLocalRBACOptions(t, "bootstrap-1", authorizer)
	opts = append(opts,
		WithTenantOperations(service),
		WithLocalRBACBootstrapAdminSubjects([]string{"bootstrap-1"}),
	)
	handler := testHandler(&fakeUOWFactory{}, opts...)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/tenants", strings.NewReader(`{"key":"security","name":"Security"}`))
	request.Header.Set("Content-Type", "application/json")
	addTestLocalRBACAuthorization(request)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var created api.Tenant
	if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil {
		t.Fatalf("decode tenant: %v", err)
	}
	if created.Key != "security" || created.Status != string(domain.TenantStatusActive) {
		t.Fatalf("created tenant = %+v", created)
	}
}

func TestDiagnosisSessionBindsSelectedTenant(t *testing.T) {
	t.Parallel()

	registry := newHTTPTestTenantRegistry()
	service, _ := tenantops.NewService(registry)
	authorizer := &fakeRBACAuthorizer{}
	opts := testLocalRBACOptions(t, "owner-1", authorizer)
	issuer, err := diagnosisauth.NewSessionTokenService(
		diagnosisauth.DefaultSessionTokenPolicy(strings.Repeat("s", diagnosisauth.MinSessionSigningKeyBytes)),
		func() time.Time { return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewSessionTokenService: %v", err)
	}
	opts = append(opts, WithTenantOperations(service), WithDiagnosisAuthSessionIssuer(issuer))
	handler := testHandler(&fakeUOWFactory{}, opts...)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/diagnosis/auth/session", nil)
	addTestLocalRBACAuthorization(request)
	request.Header.Set(tenantSelectionHeader, "platform")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("session status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response api.DiagnosisAuthSessionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if response.TenantID != 2 || response.TenantKey != "platform" {
		t.Fatalf("response tenant = %d/%q", response.TenantID, response.TenantKey)
	}
	principal, err := issuer.AuthenticateBearer(context.Background(), response.Token)
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
	}
	if principal.TenantID != 2 || principal.TenantKey != "platform" {
		t.Fatalf("signed tenant = %d/%q", principal.TenantID, principal.TenantKey)
	}
}

func TestDiagnosisSessionSwitchesSignedSessionWithoutAllowingRequestOverride(t *testing.T) {
	t.Parallel()

	registry := newHTTPTestTenantRegistry()
	service, _ := tenantops.NewService(registry)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	issuer, err := diagnosisauth.NewSessionTokenService(
		diagnosisauth.DefaultSessionTokenPolicy(strings.Repeat("s", diagnosisauth.MinSessionSigningKeyBytes)),
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("NewSessionTokenService: %v", err)
	}
	defaultSession, err := issuer.IssueToken(context.Background(), ports.AuthPrincipal{
		Subject:   "owner-1",
		Roles:     []ports.AuthRole{ports.AuthRoleOwner},
		TenantID:  tenancy.DefaultIdentity().ID,
		TenantKey: tenancy.DefaultIdentity().Key,
	}, "oidc")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	handler := testHandler(
		&fakeUOWFactory{},
		WithTenantOperations(service),
		WithDiagnosisAuthSessionIssuer(issuer),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/diagnosis/auth/session", nil)
	request.Header.Set("Authorization", "Bearer "+defaultSession.Token)
	request.Header.Set(tenantSelectionHeader, "platform")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("switch status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var switched api.DiagnosisAuthSessionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&switched); err != nil {
		t.Fatalf("decode switched session: %v", err)
	}
	if switched.TenantID != 2 || switched.TenantKey != "platform" {
		t.Fatalf("switched tenant = %d/%q", switched.TenantID, switched.TenantKey)
	}
	if !switched.CheckedAt.Equal(defaultSession.IssuedAt) || !switched.ExpiresAt.Equal(defaultSession.ExpiresAt) {
		t.Fatalf("switched lifetime = %s..%s, want %s..%s", switched.CheckedAt, switched.ExpiresAt, defaultSession.IssuedAt, defaultSession.ExpiresAt)
	}
	principal, err := issuer.AuthenticateBearer(context.Background(), switched.Token)
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
	}
	if principal.TenantID != 2 || principal.TenantKey != "platform" {
		t.Fatalf("signed switched tenant = %d/%q", principal.TenantID, principal.TenantKey)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/diagnosis/auth/check", nil)
	request.Header.Set("Authorization", "Bearer "+defaultSession.Token)
	request.Header.Set(tenantSelectionHeader, "platform")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("ordinary override status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAlertmanagerIngressBindsSelectedTenant(t *testing.T) {
	t.Parallel()

	registry := newHTTPTestTenantRegistry()
	service, _ := tenantops.NewService(registry)
	ingestor := &fakeAlertmanagerWebhookIngestor{}
	handler := testHandler(
		&fakeUOWFactory{},
		WithTenantOperations(service),
		WithAlertmanagerWebhookIngestor(ingestor),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/v1/alert-sources/7/webhooks/alertmanager",
		strings.NewReader(`{"version":"4","status":"firing","alerts":[]}`),
	)
	request.Header.Set(tenantSelectionHeader, "platform")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("ingress status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !ingestor.tenantBound || ingestor.tenant.ID != 2 || ingestor.tenant.Key != "platform" {
		t.Fatalf("ingress tenant = %+v bound=%t", ingestor.tenant, ingestor.tenantBound)
	}
}

type httpTestTenantRegistry struct {
	tenants     []domain.Tenant
	memberships []domain.TenantMembership
}

func newHTTPTestTenantRegistry() *httpTestTenantRegistry {
	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	return &httpTestTenantRegistry{
		tenants: []domain.Tenant{
			{ID: 1, Key: domain.DefaultTenantKey, Name: domain.DefaultTenantName, Status: domain.TenantStatusActive, CreatedAt: now, UpdatedAt: now},
			{ID: 2, Key: "platform", Name: "Platform", Status: domain.TenantStatusActive, CreatedAt: now, UpdatedAt: now},
		},
		memberships: []domain.TenantMembership{
			{ID: 1, TenantID: 2, Subject: "owner-1", Role: domain.TenantMembershipRoleOwner, Enabled: true, CreatedBy: "bootstrap-1", CreatedAt: now, UpdatedAt: now},
		},
	}
}

func (r *httpTestTenantRegistry) FindTenantByKey(_ context.Context, key string) (domain.Tenant, error) {
	for _, tenant := range r.tenants {
		if tenant.Key == key {
			return tenant, nil
		}
	}
	return domain.Tenant{}, domain.ErrNotFound
}

func (r *httpTestTenantRegistry) ListTenants(context.Context, int) ([]domain.Tenant, error) {
	return append([]domain.Tenant(nil), r.tenants...), nil
}

func (r *httpTestTenantRegistry) ListTenantsForSubject(_ context.Context, subject string, _ int) ([]domain.Tenant, error) {
	allowed := map[domain.TenantID]bool{}
	for _, membership := range r.memberships {
		if membership.Subject == subject && membership.Enabled {
			allowed[membership.TenantID] = true
		}
	}
	var out []domain.Tenant
	for _, tenant := range r.tenants {
		if allowed[tenant.ID] {
			out = append(out, tenant)
		}
	}
	return out, nil
}

func (r *httpTestTenantRegistry) FindTenantMembership(_ context.Context, tenantID domain.TenantID, subject string) (domain.TenantMembership, error) {
	for _, membership := range r.memberships {
		if membership.TenantID == tenantID && membership.Subject == subject {
			return membership, nil
		}
	}
	return domain.TenantMembership{}, domain.ErrNotFound
}

func (r *httpTestTenantRegistry) CreateTenantWithOwner(_ context.Context, tenant domain.Tenant, owner string) (domain.Tenant, domain.TenantMembership, error) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	tenant.ID = domain.TenantID(len(r.tenants) + 1)
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	r.tenants = append(r.tenants, tenant)
	membership := domain.TenantMembership{ID: domain.TenantMembershipID(len(r.memberships) + 1), TenantID: tenant.ID, Subject: owner, Role: domain.TenantMembershipRoleOwner, Enabled: true, CreatedBy: owner, CreatedAt: now, UpdatedAt: now}
	r.memberships = append(r.memberships, membership)
	return tenant, membership, nil
}

func (r *httpTestTenantRegistry) UpdateTenantStatus(_ context.Context, id domain.TenantID, status domain.TenantStatus) (domain.Tenant, error) {
	for i := range r.tenants {
		if r.tenants[i].ID == id {
			r.tenants[i].Status = status
			return r.tenants[i], nil
		}
	}
	return domain.Tenant{}, domain.ErrNotFound
}

func (r *httpTestTenantRegistry) ListTenantMemberships(_ context.Context, tenantID domain.TenantID, _ int) ([]domain.TenantMembership, error) {
	var out []domain.TenantMembership
	for _, membership := range r.memberships {
		if membership.TenantID == tenantID {
			out = append(out, membership)
		}
	}
	return out, nil
}

func (r *httpTestTenantRegistry) SetTenantMembership(_ context.Context, input domain.TenantMembership) (domain.TenantMembership, error) {
	for i := range r.memberships {
		if r.memberships[i].TenantID == input.TenantID && r.memberships[i].Subject == input.Subject {
			if r.memberships[i].Enabled && r.memberships[i].Role == domain.TenantMembershipRoleOwner &&
				(!input.Enabled || input.Role != domain.TenantMembershipRoleOwner) {
				owners := 0
				for _, current := range r.memberships {
					if current.TenantID == input.TenantID && current.Enabled && current.Role == domain.TenantMembershipRoleOwner {
						owners++
					}
				}
				if owners <= 1 {
					return domain.TenantMembership{}, domain.ErrPreconditionFailed
				}
			}
			r.memberships[i].Role = input.Role
			r.memberships[i].Enabled = input.Enabled
			return r.memberships[i], nil
		}
	}
	input.ID = domain.TenantMembershipID(len(r.memberships) + 1)
	input.CreatedAt = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	input.UpdatedAt = input.CreatedAt
	r.memberships = append(r.memberships, input)
	return input, nil
}
