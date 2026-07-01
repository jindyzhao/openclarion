package static

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProviderAuthenticatesBearerAndReturnsClonedPrincipal(t *testing.T) {
	provider, err := NewProvider(Config{
		Token:   "token-1",
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner, ports.AuthRoleAdmin},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	principal, err := provider.AuthenticateAuthorization(context.Background(), "Bearer token-1")
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}
	if principal.Subject != "operator-1" || len(principal.Roles) != 2 {
		t.Fatalf("principal = %+v", principal)
	}
	var claims map[string]any
	if err := json.Unmarshal(principal.Claims, &claims); err != nil {
		t.Fatalf("claims JSON: %v", err)
	}
	if claims["auth_provider"] != "static" || claims["sub"] != "operator-1" {
		t.Fatalf("claims = %+v", claims)
	}
	if strings.Contains(string(principal.Claims), "token-1") {
		t.Fatalf("claims unexpectedly contain raw token")
	}

	principal.Roles[0] = ports.AuthRoleAdmin
	principal.Claims[0] = '{'
	again, err := provider.AuthenticateAuthorization(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("AuthenticateAuthorization again: %v", err)
	}
	if again.Roles[0] != ports.AuthRoleOwner {
		t.Fatalf("provider returned mutable roles: %+v", again.Roles)
	}
	if !json.Valid(again.Claims) {
		t.Fatalf("provider returned mutable claims: %s", string(again.Claims))
	}
}

func TestProviderRejectsInvalidBearerWithoutLeakingToken(t *testing.T) {
	provider, err := NewProvider(Config{
		Token:   "token-1",
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.AuthenticateAuthorization(context.Background(), "Bearer wrong-token")
	if err == nil {
		t.Fatal("AuthenticateAuthorization: want error")
	}
	if strings.Contains(err.Error(), "wrong-token") || strings.Contains(err.Error(), "token-1") {
		t.Fatalf("error leaked token material: %v", err)
	}
}

func TestNewProviderRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "empty token",
			cfg: Config{
				Subject: "operator-1",
				Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
			},
			want: "token",
		},
		{
			name: "token whitespace",
			cfg: Config{
				Token:   " token-1 ",
				Subject: "operator-1",
				Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
			},
			want: "whitespace",
		},
		{
			name: "empty subject",
			cfg: Config{
				Token: "token-1",
				Roles: []ports.AuthRole{ports.AuthRoleAdmin},
			},
			want: "subject",
		},
		{
			name: "missing roles",
			cfg: Config{
				Token:   "token-1",
				Subject: "operator-1",
			},
			want: "role",
		},
		{
			name: "unsupported role",
			cfg: Config{
				Token:   "token-1",
				Subject: "operator-1",
				Roles:   []ports.AuthRole{"viewer"},
			},
			want: "viewer",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(tc.cfg)
			if err == nil {
				t.Fatal("NewProvider error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewProvider error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}
