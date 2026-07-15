package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultTenantID is reserved for the compatibility tenant seeded by migrations.
	DefaultTenantID TenantID = 1
	// DefaultTenantKey preserves the existing single-tenant deployment mode.
	DefaultTenantKey = "default"
	// DefaultTenantName is the operator-facing name of the bootstrap tenant.
	DefaultTenantName = "Default"
	// MaxTenantKeyLength bounds tenant keys used in tokens and Temporal headers.
	MaxTenantKeyLength = 63
	// MaxTenantNameLength bounds tenant display names.
	MaxTenantNameLength = 120
)

// TenantStatus controls whether new sessions can be issued for a tenant.
type TenantStatus string

const (
	// TenantStatusActive permits authenticated product operations.
	TenantStatusActive TenantStatus = "active"
	// TenantStatusDisabled blocks new tenant-scoped sessions and schedules.
	TenantStatusDisabled TenantStatus = "disabled"
)

// TenantMembershipRole controls tenant-registry administration, independently
// of product RBAC roles inside the tenant workspace.
type TenantMembershipRole string

const (
	// TenantMembershipRoleOwner permits tenant registry administration.
	TenantMembershipRoleOwner TenantMembershipRole = "owner"
	// TenantMembershipRoleMember permits access without registry administration.
	TenantMembershipRoleMember TenantMembershipRole = "member"
)

// Tenant is the global registry entry for one isolated product workspace.
type Tenant struct {
	ID        TenantID
	Key       string
	Name      string
	Status    TenantStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TenantMembership grants one authenticated subject access to a tenant.
type TenantMembership struct {
	ID        TenantMembershipID
	TenantID  TenantID
	Subject   string
	Role      TenantMembershipRole
	Enabled   bool
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NormalizeTenantKey validates and normalizes an ASCII lowercase tenant key.
func NormalizeTenantKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" || len(key) > MaxTenantKeyLength {
		return "", fmt.Errorf("tenant key must contain 1 to %d characters: %w", MaxTenantKeyLength, ErrInvariantViolation)
	}
	for i, r := range key {
		valid := r >= 'a' && r <= 'z'
		if i > 0 {
			valid = valid || r >= '0' && r <= '9' || r == '-'
		}
		if !valid {
			return "", fmt.Errorf("tenant key %q must start with a lowercase letter and contain only lowercase letters, digits, or hyphens: %w", key, ErrInvariantViolation)
		}
	}
	if strings.HasSuffix(key, "-") || strings.Contains(key, "--") {
		return "", fmt.Errorf("tenant key %q must not end with or repeat hyphens: %w", key, ErrInvariantViolation)
	}
	return key, nil
}

// NormalizeTenantName validates an operator-facing tenant name.
func NormalizeTenantName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxTenantNameLength || strings.ContainsAny(name, "\x00\r\n") {
		return "", fmt.Errorf("tenant name must be valid UTF-8, non-empty, single-line, and at most %d characters: %w", MaxTenantNameLength, ErrInvariantViolation)
	}
	return name, nil
}

// ValidateTenantStatus rejects unknown lifecycle values.
func ValidateTenantStatus(status TenantStatus) error {
	switch status {
	case TenantStatusActive, TenantStatusDisabled:
		return nil
	default:
		return fmt.Errorf("tenant status %q is unsupported: %w", status, ErrInvariantViolation)
	}
}

// ValidateTenantMembershipRole rejects unknown registry roles.
func ValidateTenantMembershipRole(role TenantMembershipRole) error {
	switch role {
	case TenantMembershipRoleOwner, TenantMembershipRoleMember:
		return nil
	default:
		return fmt.Errorf("tenant membership role %q is unsupported: %w", role, ErrInvariantViolation)
	}
}
