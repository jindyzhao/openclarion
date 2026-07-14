package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeTenantKey(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"default", "incident-response", "team2"} {
		got, err := NormalizeTenantKey(key)
		if err != nil || got != key {
			t.Fatalf("NormalizeTenantKey(%q) = %q, %v", key, got, err)
		}
	}
	for _, key := range []string{"", " Default", "2team", "team_1", "team--one", "team-"} {
		if _, err := NormalizeTenantKey(key); !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("NormalizeTenantKey(%q) err = %v, want invariant violation", key, err)
		}
	}
}

func TestNormalizeTenantNameCountsCharacters(t *testing.T) {
	t.Parallel()

	name := strings.Repeat("中", MaxTenantNameLength)
	if got, err := NormalizeTenantName(name); err != nil || got != name {
		t.Fatalf("NormalizeTenantName(valid UTF-8) = %q, %v", got, err)
	}
	if _, err := NormalizeTenantName(name + "文"); !errors.Is(err, ErrInvariantViolation) {
		t.Fatalf("NormalizeTenantName(over limit) err = %v, want invariant violation", err)
	}
}
