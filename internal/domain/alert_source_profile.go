package domain

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const (
	maxAlertSourceNameLen   = 120
	maxAlertSourceURLLen    = 2048
	maxAlertSourceSecretLen = 256
	maxAlertSourceLabels    = 32
	maxAlertSourceLabelKey  = 64
	maxAlertSourceLabelVal  = 128
)

// AlertSourceKind identifies the upstream alert source adapter type.
type AlertSourceKind string

const (
	// AlertSourceKindPrometheus reads alert state from a Prometheus-compatible API.
	AlertSourceKindPrometheus AlertSourceKind = "prometheus"
	// AlertSourceKindAlertmanager reads alert state from an Alertmanager-compatible API.
	AlertSourceKindAlertmanager AlertSourceKind = "alertmanager"
)

// Valid reports whether k is a supported alert source kind.
func (k AlertSourceKind) Valid() bool {
	switch k {
	case AlertSourceKindPrometheus, AlertSourceKindAlertmanager:
		return true
	}
	return false
}

// AlertSourceAuthMode describes how the concrete provider obtains credentials.
type AlertSourceAuthMode string

const (
	// AlertSourceAuthModeNone uses no upstream credentials.
	AlertSourceAuthModeNone AlertSourceAuthMode = "none"
	// AlertSourceAuthModeBearer uses a deployment-managed bearer-token secret reference.
	AlertSourceAuthModeBearer AlertSourceAuthMode = "bearer"
)

// Valid reports whether m is a supported authentication mode.
func (m AlertSourceAuthMode) Valid() bool {
	switch m {
	case AlertSourceAuthModeNone, AlertSourceAuthModeBearer:
		return true
	}
	return false
}

// AlertSourceProfile is operator-managed connection metadata for an alert
// source. Secret material is intentionally excluded; SecretRef points at a
// deployment-managed secret boundary.
type AlertSourceProfile struct {
	ID        AlertSourceProfileID
	Name      string
	Kind      AlertSourceKind
	BaseURL   string
	AuthMode  AlertSourceAuthMode
	SecretRef string
	Enabled   bool
	Labels    map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewAlertSourceProfile constructs a validated alert source profile.
func NewAlertSourceProfile(
	name string,
	kind AlertSourceKind,
	baseURL string,
	authMode AlertSourceAuthMode,
	secretRef string,
	enabled bool,
	labels map[string]string,
) (AlertSourceProfile, error) {
	name = strings.TrimSpace(name)
	baseURL = strings.TrimSpace(baseURL)
	secretRef = strings.TrimSpace(secretRef)
	if name == "" {
		return AlertSourceProfile{}, fmt.Errorf("alert source profile: name must be non-empty: %w", ErrInvariantViolation)
	}
	if len(name) > maxAlertSourceNameLen {
		return AlertSourceProfile{}, fmt.Errorf("alert source profile: name exceeds %d bytes: %w", maxAlertSourceNameLen, ErrInvariantViolation)
	}
	if !kind.Valid() {
		return AlertSourceProfile{}, fmt.Errorf("alert source profile: kind %q is unsupported: %w", kind, ErrInvariantViolation)
	}
	if err := validateAlertSourceBaseURL(baseURL); err != nil {
		return AlertSourceProfile{}, err
	}
	if !authMode.Valid() {
		return AlertSourceProfile{}, fmt.Errorf("alert source profile: auth mode %q is unsupported: %w", authMode, ErrInvariantViolation)
	}
	if authMode == AlertSourceAuthModeNone && secretRef != "" {
		return AlertSourceProfile{}, fmt.Errorf("alert source profile: secret_ref requires bearer auth: %w", ErrInvariantViolation)
	}
	if authMode == AlertSourceAuthModeBearer && secretRef == "" {
		return AlertSourceProfile{}, fmt.Errorf("alert source profile: bearer auth requires secret_ref: %w", ErrInvariantViolation)
	}
	if err := validateSecretRef(secretRef); err != nil {
		return AlertSourceProfile{}, err
	}
	normalizedLabels, err := validateAlertSourceLabels(labels)
	if err != nil {
		return AlertSourceProfile{}, err
	}
	return AlertSourceProfile{
		Name:      name,
		Kind:      kind,
		BaseURL:   baseURL,
		AuthMode:  authMode,
		SecretRef: secretRef,
		Enabled:   enabled,
		Labels:    normalizedLabels,
	}, nil
}

func validateAlertSourceBaseURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("alert source profile: base_url must be non-empty: %w", ErrInvariantViolation)
	}
	if len(raw) > maxAlertSourceURLLen {
		return fmt.Errorf("alert source profile: base_url exceeds %d bytes: %w", maxAlertSourceURLLen, ErrInvariantViolation)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("alert source profile: base_url must be a valid URL: %w", ErrInvariantViolation)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("alert source profile: base_url scheme must be http or https: %w", ErrInvariantViolation)
	}
	if parsed.Host == "" {
		return fmt.Errorf("alert source profile: base_url host must be non-empty: %w", ErrInvariantViolation)
	}
	if parsed.User != nil {
		return fmt.Errorf("alert source profile: base_url must not include userinfo: %w", ErrInvariantViolation)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("alert source profile: base_url must not include query or fragment: %w", ErrInvariantViolation)
	}
	return nil
}

func validateSecretRef(secretRef string) error {
	if secretRef == "" {
		return nil
	}
	if len(secretRef) > maxAlertSourceSecretLen {
		return fmt.Errorf("alert source profile: secret_ref exceeds %d bytes: %w", maxAlertSourceSecretLen, ErrInvariantViolation)
	}
	if containsControlOrSpace(secretRef) {
		return fmt.Errorf("alert source profile: secret_ref must not contain whitespace or control characters: %w", ErrInvariantViolation)
	}
	return nil
}

func validateAlertSourceLabels(labels map[string]string) (map[string]string, error) {
	if labels == nil {
		return map[string]string{}, nil
	}
	if len(labels) > maxAlertSourceLabels {
		return nil, fmt.Errorf("alert source profile: labels exceed %d entries: %w", maxAlertSourceLabels, ErrInvariantViolation)
	}
	out := make(map[string]string, len(labels))
	for rawKey, rawVal := range labels {
		key := strings.TrimSpace(rawKey)
		val := strings.TrimSpace(rawVal)
		if key == "" {
			return nil, fmt.Errorf("alert source profile: label key must be non-empty: %w", ErrInvariantViolation)
		}
		if len(key) > maxAlertSourceLabelKey {
			return nil, fmt.Errorf("alert source profile: label key exceeds %d bytes: %w", maxAlertSourceLabelKey, ErrInvariantViolation)
		}
		if len(val) > maxAlertSourceLabelVal {
			return nil, fmt.Errorf("alert source profile: label value exceeds %d bytes: %w", maxAlertSourceLabelVal, ErrInvariantViolation)
		}
		if containsControl(key) || containsControl(val) {
			return nil, fmt.Errorf("alert source profile: labels must not contain control characters: %w", ErrInvariantViolation)
		}
		out[key] = val
	}
	return out, nil
}

func containsControlOrSpace(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func containsControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// AlertSourceLabelsJSON returns labels as a stable JSON object for API or
// evidence surfaces that need an explicit empty object rather than nil.
func AlertSourceLabelsJSON(labels map[string]string) json.RawMessage {
	if labels == nil {
		labels = map[string]string{}
	}
	raw, err := json.Marshal(labels)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}
