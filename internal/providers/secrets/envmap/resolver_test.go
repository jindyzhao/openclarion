package envmap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestResolverResolveSecret(t *testing.T) {
	resolver, err := NewResolver(map[string]string{
		"secret/openclarion/prometheus-bearer": "token-123",
	})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	secret, err := resolver.ResolveSecret(context.Background(), "secret/openclarion/prometheus-bearer")
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if secret.Value != "token-123" {
		t.Fatalf("secret value = %q, want token-123", secret.Value)
	}
}

func TestResolverReturnsNotFound(t *testing.T) {
	resolver, err := NewResolver(nil)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	_, err = resolver.ResolveSecret(context.Background(), "secret/missing")
	if !errors.Is(err, ports.ErrSecretNotFound) {
		t.Fatalf("ResolveSecret err = %v, want ErrSecretNotFound", err)
	}
}

func TestResolverHonorsContextCancellation(t *testing.T) {
	resolver, err := NewResolver(map[string]string{"secret/ref": "token"})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = resolver.ResolveSecret(ctx, "secret/ref")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveSecret err = %v, want context.Canceled", err)
	}
}

func TestNewResolverFromJSONRejectsAmbiguousInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "duplicate ref", raw: `{"secret/ref":"one","secret/ref":"two"}`, want: "parse secret map JSON"},
		{name: "trailing value", raw: `{"secret/ref":"one"} {"secret/ref":"two"}`, want: "parse secret map JSON"},
		{name: "array", raw: `["secret/ref"]`, want: "parse secret map JSON"},
		{name: "empty ref", raw: `{"":"token"}`, want: "secret ref must be non-empty"},
		{name: "space ref", raw: `{"secret/ref value":"token"}`, want: "secret ref must not contain"},
		{name: "empty value", raw: `{"secret/ref":""}`, want: "secret value must be non-empty"},
		{name: "space value", raw: `{"secret/ref":"token value"}`, want: "secret value must not contain"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewResolverFromJSON(tc.raw)
			if err == nil {
				t.Fatal("NewResolverFromJSON err = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewResolverFromJSON err = %q, want substring %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "token value") {
				t.Fatalf("NewResolverFromJSON error leaked secret value: %v", err)
			}
		})
	}
}

func TestNewResolverFromJSONAcceptsEmptyInput(t *testing.T) {
	resolver, err := NewResolverFromJSON("  ")
	if err != nil {
		t.Fatalf("NewResolverFromJSON: %v", err)
	}
	_, err = resolver.ResolveSecret(context.Background(), "secret/ref")
	if !errors.Is(err, ports.ErrSecretNotFound) {
		t.Fatalf("ResolveSecret err = %v, want ErrSecretNotFound", err)
	}
}
