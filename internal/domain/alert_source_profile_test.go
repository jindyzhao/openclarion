package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewAlertSourceProfileValid(t *testing.T) {
	profile, err := NewAlertSourceProfile(
		" Primary Prometheus ",
		AlertSourceKindPrometheus,
		"https://prometheus.example.test/prometheus",
		AlertSourceAuthModeBearer,
		"secret/prometheus-token",
		true,
		map[string]string{" env ": " prod "},
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	if profile.Name != "Primary Prometheus" || profile.BaseURL != "https://prometheus.example.test/prometheus" {
		t.Fatalf("profile normalization failed: %+v", profile)
	}
	if profile.Labels["env"] != "prod" {
		t.Fatalf("labels = %+v", profile.Labels)
	}
}

func TestNewAlertSourceProfileRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		edit func(*alertSourceProfileArgs)
	}{
		{name: "empty_name", edit: func(a *alertSourceProfileArgs) { a.name = " " }},
		{name: "unsupported_kind", edit: func(a *alertSourceProfileArgs) { a.kind = AlertSourceKind("datadog") }},
		{name: "invalid_url", edit: func(a *alertSourceProfileArgs) { a.baseURL = "not a url" }},
		{name: "url_userinfo", edit: func(a *alertSourceProfileArgs) { a.baseURL = "https://user:pass@example.test" }},
		{name: "url_query", edit: func(a *alertSourceProfileArgs) { a.baseURL = "https://example.test?token=secret" }},
		{name: "unsupported_auth", edit: func(a *alertSourceProfileArgs) { a.authMode = AlertSourceAuthMode("basic") }},
		{name: "none_with_secret", edit: func(a *alertSourceProfileArgs) {
			a.authMode = AlertSourceAuthModeNone
			a.secretRef = "secret/prom"
		}},
		{name: "bearer_without_secret", edit: func(a *alertSourceProfileArgs) {
			a.authMode = AlertSourceAuthModeBearer
			a.secretRef = ""
		}},
		{name: "secret_with_space", edit: func(a *alertSourceProfileArgs) { a.secretRef = "secret value" }},
		{name: "empty_label_key", edit: func(a *alertSourceProfileArgs) { a.labels = map[string]string{" ": "prod"} }},
		{name: "control_label_value", edit: func(a *alertSourceProfileArgs) { a.labels = map[string]string{"env": "pro\nd"} }},
		{name: "too_many_labels", edit: func(a *alertSourceProfileArgs) {
			a.labels = map[string]string{}
			for i := 0; i < maxAlertSourceLabels+1; i++ {
				a.labels["label"+strings.Repeat("x", i)] = "value"
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := validAlertSourceProfileArgs()
			tc.edit(&args)
			_, err := NewAlertSourceProfile(args.name, args.kind, args.baseURL, args.authMode, args.secretRef, args.enabled, args.labels)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

type alertSourceProfileArgs struct {
	name      string
	kind      AlertSourceKind
	baseURL   string
	authMode  AlertSourceAuthMode
	secretRef string
	enabled   bool
	labels    map[string]string
}

func validAlertSourceProfileArgs() alertSourceProfileArgs {
	return alertSourceProfileArgs{
		name:      "Prometheus",
		kind:      AlertSourceKindPrometheus,
		baseURL:   "https://prometheus.example.test",
		authMode:  AlertSourceAuthModeBearer,
		secretRef: "secret/prometheus",
		enabled:   true,
		labels:    map[string]string{"env": "test"},
	}
}
