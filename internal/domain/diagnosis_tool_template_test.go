package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewDiagnosisToolTemplateValidatesAndTrims(t *testing.T) {
	template, err := NewDiagnosisToolTemplate(
		" CPU saturation ",
		AlertSourceProfileID(1),
		DiagnosisToolKindMetricRangeQuery,
		` rate(container_cpu_usage_seconds_total[5m]) `,
		5,
		time.Hour,
		2*time.Hour,
		time.Minute,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	if template.Name != "CPU saturation" ||
		template.AlertSourceProfileID != 1 ||
		template.Tool != DiagnosisToolKindMetricRangeQuery ||
		template.QueryTemplate != `rate(container_cpu_usage_seconds_total[5m])` ||
		template.DefaultLimit != 5 ||
		template.DefaultWindow != time.Hour ||
		template.MaxWindow != 2*time.Hour ||
		template.DefaultStep != time.Minute ||
		template.Enabled {
		t.Fatalf("template = %+v", template)
	}
}

func TestNewDiagnosisToolTemplateRejectsInvalidInputs(t *testing.T) {
	now := time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		edit func(*diagnosisToolTemplateArgs)
	}{
		{name: "empty_name", edit: func(a *diagnosisToolTemplateArgs) { a.name = " " }},
		{name: "missing_source", edit: func(a *diagnosisToolTemplateArgs) { a.sourceID = 0 }},
		{name: "bad_tool", edit: func(a *diagnosisToolTemplateArgs) { a.tool = "kubectl" }},
		{name: "active_with_query", edit: func(a *diagnosisToolTemplateArgs) {
			a.tool = DiagnosisToolKindActiveAlerts
			a.query = "up"
			a.defaultWindow = 0
			a.maxWindow = 0
			a.defaultStep = 0
		}},
		{name: "active_bad_limit", edit: func(a *diagnosisToolTemplateArgs) {
			a.tool = DiagnosisToolKindActiveAlerts
			a.query = ""
			a.limit = 11
			a.defaultWindow = 0
			a.maxWindow = 0
			a.defaultStep = 0
		}},
		{name: "metric_without_query", edit: func(a *diagnosisToolTemplateArgs) {
			a.tool = DiagnosisToolKindMetricQuery
			a.query = ""
			a.defaultWindow = 0
			a.maxWindow = 0
			a.defaultStep = 0
		}},
		{name: "metric_multiline_query", edit: func(a *diagnosisToolTemplateArgs) {
			a.tool = DiagnosisToolKindMetricQuery
			a.query = "up\nrate(http_requests_total[5m])"
			a.defaultWindow = 0
			a.maxWindow = 0
			a.defaultStep = 0
		}},
		{name: "metric_bad_placeholder", edit: func(a *diagnosisToolTemplateArgs) {
			a.tool = DiagnosisToolKindMetricQuery
			a.query = `up{job={{label.job}}}`
			a.defaultWindow = 0
			a.maxWindow = 0
			a.defaultStep = 0
		}},
		{name: "range_window_too_large", edit: func(a *diagnosisToolTemplateArgs) { a.maxWindow = 7 * time.Hour }},
		{name: "range_max_before_default", edit: func(a *diagnosisToolTemplateArgs) { a.maxWindow = 30 * time.Minute }},
		{name: "range_step_too_small", edit: func(a *diagnosisToolTemplateArgs) { a.defaultStep = time.Second }},
		{name: "disabled_with_enabled_at", edit: func(a *diagnosisToolTemplateArgs) { a.enabledAt = &now }},
		{name: "enabled_with_disabled_at", edit: func(a *diagnosisToolTemplateArgs) {
			a.enabled = true
			a.disabledAt = &now
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := defaultDiagnosisToolTemplateArgs()
			tc.edit(&args)
			_, err := NewDiagnosisToolTemplate(
				args.name,
				args.sourceID,
				args.tool,
				args.query,
				args.limit,
				args.defaultWindow,
				args.maxWindow,
				args.defaultStep,
				args.enabled,
				args.enabledAt,
				args.disabledAt,
			)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestNewDiagnosisToolTemplateAcceptsSafePlaceholders(t *testing.T) {
	template, err := NewDiagnosisToolTemplate(
		"Parameterized metric",
		AlertSourceProfileID(1),
		DiagnosisToolKindMetricQuery,
		`up{job="{{label.job}}"}`,
		5,
		0,
		0,
		0,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	if template.QueryTemplate != `up{job="{{label.job}}"}` {
		t.Fatalf("query template = %q", template.QueryTemplate)
	}
}

func TestWithDiagnosisToolTemplateEnabledTogglesExplicitState(t *testing.T) {
	template, err := NewDiagnosisToolTemplate(
		"CPU saturation",
		AlertSourceProfileID(1),
		DiagnosisToolKindMetricRangeQuery,
		`rate(container_cpu_usage_seconds_total[5m])`,
		5,
		time.Hour,
		2*time.Hour,
		time.Minute,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	template.ID = 9

	enabledAt := time.Date(2026, 6, 8, 9, 0, 0, 987654321, time.FixedZone("offset", 3600))
	enabled, err := WithDiagnosisToolTemplateEnabled(template, true, enabledAt)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !enabled.Enabled || enabled.EnabledAt == nil || !enabled.EnabledAt.Equal(time.Date(2026, 6, 8, 8, 0, 0, 987654000, time.UTC)) || enabled.DisabledAt != nil {
		t.Fatalf("enabled = %+v", enabled)
	}

	disabled, err := WithDiagnosisToolTemplateEnabled(enabled, false, enabledAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if disabled.Enabled || disabled.EnabledAt != nil || disabled.DisabledAt == nil {
		t.Fatalf("disabled = %+v", disabled)
	}
}

type diagnosisToolTemplateArgs struct {
	name          string
	sourceID      AlertSourceProfileID
	tool          DiagnosisToolKind
	query         string
	limit         int
	defaultWindow time.Duration
	maxWindow     time.Duration
	defaultStep   time.Duration
	enabled       bool
	enabledAt     *time.Time
	disabledAt    *time.Time
}

func defaultDiagnosisToolTemplateArgs() diagnosisToolTemplateArgs {
	return diagnosisToolTemplateArgs{
		name:          "CPU saturation",
		sourceID:      AlertSourceProfileID(1),
		tool:          DiagnosisToolKindMetricRangeQuery,
		query:         `rate(container_cpu_usage_seconds_total[5m])`,
		limit:         5,
		defaultWindow: time.Hour,
		maxWindow:     2 * time.Hour,
		defaultStep:   time.Minute,
	}
}
