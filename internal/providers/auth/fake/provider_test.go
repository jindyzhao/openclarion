package fake

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProviderReturnsScriptedPrincipalAndRecordsRequests(t *testing.T) {
	provider := New(map[string][]Result{
		"token-1": {
			{Principal: ports.AuthPrincipal{
				Subject: "owner-1",
				Roles:   []ports.AuthRole{ports.AuthRoleOwner},
				Claims:  []byte(`{"roles":["owner"]}`),
			}},
		},
	})
	principal, err := provider.AuthenticateAuthorization(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}
	if principal.Subject != "owner-1" || !slices.Contains(principal.Roles, ports.AuthRoleOwner) {
		t.Fatalf("principal = %+v", principal)
	}
	principal.Roles[0] = ports.AuthRoleAdmin
	principal.Claims[0] = '{'

	again, err := provider.AuthenticateAuthorization(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("AuthenticateAuthorization again: %v", err)
	}
	if again.Roles[0] != ports.AuthRoleOwner {
		t.Fatalf("Roles[0] = %q, want owner", again.Roles[0])
	}
	if provider.Calls("token-1") != 2 {
		t.Fatalf("Calls = %d, want 2", provider.Calls("token-1"))
	}
	requests := provider.Requests()
	if len(requests) != 2 || requests[0] != "token-1" || requests[1] != "token-1" {
		t.Fatalf("Requests = %#v", requests)
	}
}

func TestProviderReturnsScriptedErrorAndMissingScript(t *testing.T) {
	wantErr := errors.New("denied")
	provider := New(map[string][]Result{
		"token-1": {{Err: wantErr}},
	})
	if _, err := provider.AuthenticateAuthorization(context.Background(), "token-1"); !errors.Is(err, wantErr) {
		t.Fatalf("AuthenticateAuthorization scripted err = %v, want %v", err, wantErr)
	}
	if _, err := provider.AuthenticateAuthorization(context.Background(), "unknown"); err == nil {
		t.Fatalf("AuthenticateAuthorization missing script: want error")
	}
}
