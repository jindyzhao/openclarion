package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	maxNotificationChannelNameLen   = 120
	maxNotificationChannelSecretLen = 256
	maxNotificationChannelLabels    = 32
	maxNotificationChannelLabelKey  = 64
	maxNotificationChannelLabelVal  = 128
)

// NotificationChannelKind identifies the concrete operator-notification
// adapter shape. The first persisted channel type maps to the existing
// Webhook IM provider boundary without resolving the secret in this slice.
type NotificationChannelKind string

const (
	// NotificationChannelKindWebhook represents an HTTP webhook target whose
	// endpoint is resolved from a deployment-managed secret reference.
	NotificationChannelKindWebhook NotificationChannelKind = "webhook"
)

// Valid reports whether k is a supported notification channel kind.
func (k NotificationChannelKind) Valid() bool {
	switch k {
	case NotificationChannelKindWebhook:
		return true
	}
	return false
}

// NotificationDeliveryScope records which notification flows may use a
// channel once later workflow binding is implemented.
type NotificationDeliveryScope string

const (
	// NotificationDeliveryScopeReport covers final report notifications.
	NotificationDeliveryScopeReport NotificationDeliveryScope = "report"
	// NotificationDeliveryScopeDiagnosisClose covers diagnosis-room close
	// notifications.
	NotificationDeliveryScopeDiagnosisClose NotificationDeliveryScope = "diagnosis_close"
)

// Valid reports whether s is a supported notification delivery scope.
func (s NotificationDeliveryScope) Valid() bool {
	switch s {
	case NotificationDeliveryScopeReport, NotificationDeliveryScopeDiagnosisClose:
		return true
	}
	return false
}

// NotificationChannelProfile is operator-managed notification target metadata.
// Secret material is intentionally excluded; SecretRef points at a
// deployment-managed secret boundary that can resolve endpoint credentials
// later, outside OpenAPI responses and browser state.
type NotificationChannelProfile struct {
	ID             NotificationChannelProfileID
	Name           string
	Kind           NotificationChannelKind
	SecretRef      string
	DeliveryScopes []NotificationDeliveryScope
	Enabled        bool
	Labels         map[string]string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewNotificationChannelProfile constructs a validated notification channel
// profile. Delivery scopes are trimmed, deduplicated, and sorted for stable
// persistence. It does not resolve SecretRef or construct an IM provider.
func NewNotificationChannelProfile(
	name string,
	kind NotificationChannelKind,
	secretRef string,
	deliveryScopes []NotificationDeliveryScope,
	enabled bool,
	labels map[string]string,
) (NotificationChannelProfile, error) {
	name = strings.TrimSpace(name)
	secretRef = strings.TrimSpace(secretRef)
	if name == "" {
		return NotificationChannelProfile{}, fmt.Errorf("notification channel profile: name must be non-empty: %w", ErrInvariantViolation)
	}
	if len(name) > maxNotificationChannelNameLen {
		return NotificationChannelProfile{}, fmt.Errorf("notification channel profile: name exceeds %d bytes: %w", maxNotificationChannelNameLen, ErrInvariantViolation)
	}
	if !kind.Valid() {
		return NotificationChannelProfile{}, fmt.Errorf("notification channel profile: kind %q is unsupported: %w", kind, ErrInvariantViolation)
	}
	if err := validateNotificationChannelSecretRef(secretRef); err != nil {
		return NotificationChannelProfile{}, err
	}
	normalizedScopes, err := normalizeNotificationDeliveryScopes(deliveryScopes)
	if err != nil {
		return NotificationChannelProfile{}, err
	}
	normalizedLabels, err := validateNotificationChannelLabels(labels)
	if err != nil {
		return NotificationChannelProfile{}, err
	}
	return NotificationChannelProfile{
		Name:           name,
		Kind:           kind,
		SecretRef:      secretRef,
		DeliveryScopes: normalizedScopes,
		Enabled:        enabled,
		Labels:         normalizedLabels,
	}, nil
}

func validateNotificationChannelSecretRef(secretRef string) error {
	if secretRef == "" {
		return fmt.Errorf("notification channel profile: secret_ref must be non-empty: %w", ErrInvariantViolation)
	}
	if len(secretRef) > maxNotificationChannelSecretLen {
		return fmt.Errorf("notification channel profile: secret_ref exceeds %d bytes: %w", maxNotificationChannelSecretLen, ErrInvariantViolation)
	}
	if containsControlOrSpace(secretRef) {
		return fmt.Errorf("notification channel profile: secret_ref must not contain whitespace or control characters: %w", ErrInvariantViolation)
	}
	return nil
}

func normalizeNotificationDeliveryScopes(scopes []NotificationDeliveryScope) ([]NotificationDeliveryScope, error) {
	if len(scopes) == 0 {
		return nil, fmt.Errorf("notification channel profile: delivery_scopes must be non-empty: %w", ErrInvariantViolation)
	}
	seen := map[NotificationDeliveryScope]struct{}{}
	for _, raw := range scopes {
		scope := NotificationDeliveryScope(strings.TrimSpace(string(raw)))
		if scope == "" {
			return nil, fmt.Errorf("notification channel profile: delivery scope must be non-empty: %w", ErrInvariantViolation)
		}
		if !scope.Valid() {
			return nil, fmt.Errorf("notification channel profile: delivery scope %q is unsupported: %w", scope, ErrInvariantViolation)
		}
		seen[scope] = struct{}{}
	}
	out := make([]NotificationDeliveryScope, 0, len(seen))
	for scope := range seen {
		out = append(out, scope)
	}
	slices.Sort(out)
	return out, nil
}

func validateNotificationChannelLabels(labels map[string]string) (map[string]string, error) {
	if labels == nil {
		return map[string]string{}, nil
	}
	if len(labels) > maxNotificationChannelLabels {
		return nil, fmt.Errorf("notification channel profile: labels exceed %d entries: %w", maxNotificationChannelLabels, ErrInvariantViolation)
	}
	out := make(map[string]string, len(labels))
	for rawKey, rawVal := range labels {
		key := strings.TrimSpace(rawKey)
		val := strings.TrimSpace(rawVal)
		if key == "" {
			return nil, fmt.Errorf("notification channel profile: label key must be non-empty: %w", ErrInvariantViolation)
		}
		if len(key) > maxNotificationChannelLabelKey {
			return nil, fmt.Errorf("notification channel profile: label key exceeds %d bytes: %w", maxNotificationChannelLabelKey, ErrInvariantViolation)
		}
		if len(val) > maxNotificationChannelLabelVal {
			return nil, fmt.Errorf("notification channel profile: label value exceeds %d bytes: %w", maxNotificationChannelLabelVal, ErrInvariantViolation)
		}
		if containsControl(key) || containsControl(val) {
			return nil, fmt.Errorf("notification channel profile: labels must not contain control characters: %w", ErrInvariantViolation)
		}
		out[key] = val
	}
	return out, nil
}
