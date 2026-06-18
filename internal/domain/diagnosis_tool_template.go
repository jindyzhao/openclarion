package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	maxDiagnosisToolTemplateNameLen  = 120
	maxDiagnosisToolQueryTemplateLen = 500

	minDiagnosisToolRangeWindow = 15 * time.Second
	maxDiagnosisToolRangeWindow = 6 * time.Hour
	minDiagnosisToolRangeStep   = 15 * time.Second
)

// DiagnosisToolKind identifies a backend-approved diagnosis evidence collector.
type DiagnosisToolKind string

const (
	// DiagnosisToolKindActiveAlerts lists active alerts from a configured source.
	DiagnosisToolKindActiveAlerts DiagnosisToolKind = "active_alerts"
	// DiagnosisToolKindMetricQuery runs a bounded instant Prometheus query.
	DiagnosisToolKindMetricQuery DiagnosisToolKind = "metric_query"
	// DiagnosisToolKindMetricRangeQuery runs a bounded Prometheus range query.
	DiagnosisToolKindMetricRangeQuery DiagnosisToolKind = "metric_range_query"
)

// Valid reports whether k is a supported diagnosis tool kind.
func (k DiagnosisToolKind) Valid() bool {
	switch k {
	case DiagnosisToolKindActiveAlerts, DiagnosisToolKindMetricQuery, DiagnosisToolKindMetricRangeQuery:
		return true
	}
	return false
}

// DiagnosisToolTemplate is operator-managed configuration for routine
// diagnosis evidence collection. It is configuration only; saving or enabling
// a template never starts a workflow or calls an upstream provider.
type DiagnosisToolTemplate struct {
	ID                   DiagnosisToolTemplateID
	Name                 string
	AlertSourceProfileID AlertSourceProfileID
	Tool                 DiagnosisToolKind
	QueryTemplate        string
	DefaultLimit         int
	DefaultWindow        time.Duration
	MaxWindow            time.Duration
	DefaultStep          time.Duration
	Enabled              bool
	EnabledAt            *time.Time
	DisabledAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// NewDiagnosisToolTemplate constructs a validated diagnosis tool template.
func NewDiagnosisToolTemplate(
	name string,
	alertSourceProfileID AlertSourceProfileID,
	tool DiagnosisToolKind,
	queryTemplate string,
	defaultLimit int,
	defaultWindow time.Duration,
	maxWindow time.Duration,
	defaultStep time.Duration,
	enabled bool,
	enabledAt *time.Time,
	disabledAt *time.Time,
) (DiagnosisToolTemplate, error) {
	name = strings.TrimSpace(name)
	queryTemplate = strings.TrimSpace(queryTemplate)
	if name == "" {
		return DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: name must be non-empty: %w", ErrInvariantViolation)
	}
	if len(name) > maxDiagnosisToolTemplateNameLen {
		return DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: name exceeds %d bytes: %w", maxDiagnosisToolTemplateNameLen, ErrInvariantViolation)
	}
	if alertSourceProfileID == 0 {
		return DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: alert_source_profile_id must be non-zero: %w", ErrInvariantViolation)
	}
	if !tool.Valid() {
		return DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: tool %q is unsupported: %w", tool, ErrInvariantViolation)
	}
	if err := validateDiagnosisToolTemplateShape(tool, queryTemplate, defaultLimit, defaultWindow, maxWindow, defaultStep); err != nil {
		return DiagnosisToolTemplate{}, err
	}
	if !enabled && enabledAt != nil {
		return DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: enabled_at requires enabled=true: %w", ErrInvariantViolation)
	}
	if enabled && disabledAt != nil {
		return DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: disabled_at requires enabled=false: %w", ErrInvariantViolation)
	}
	return DiagnosisToolTemplate{
		Name:                 name,
		AlertSourceProfileID: alertSourceProfileID,
		Tool:                 tool,
		QueryTemplate:        queryTemplate,
		DefaultLimit:         defaultLimit,
		DefaultWindow:        defaultWindow,
		MaxWindow:            maxWindow,
		DefaultStep:          defaultStep,
		Enabled:              enabled,
		EnabledAt:            cloneTimePtr(enabledAt),
		DisabledAt:           cloneTimePtr(disabledAt),
	}, nil
}

// WithDiagnosisToolTemplateEnabled returns a copy with explicit enablement
// state updated at the supplied time.
func WithDiagnosisToolTemplateEnabled(template DiagnosisToolTemplate, enabled bool, at time.Time) (DiagnosisToolTemplate, error) {
	if template.ID == 0 {
		return DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: id must be non-zero: %w", ErrInvariantViolation)
	}
	at = NormalizeUTCMicro(at)
	if at.IsZero() {
		return DiagnosisToolTemplate{}, fmt.Errorf("diagnosis tool template: enablement time must be non-zero: %w", ErrInvariantViolation)
	}
	template.Enabled = enabled
	if enabled {
		template.EnabledAt = &at
		template.DisabledAt = nil
	} else {
		template.EnabledAt = nil
		template.DisabledAt = &at
	}
	return template, nil
}

func validateDiagnosisToolTemplateShape(
	tool DiagnosisToolKind,
	queryTemplate string,
	defaultLimit int,
	defaultWindow time.Duration,
	maxWindow time.Duration,
	defaultStep time.Duration,
) error {
	switch tool {
	case DiagnosisToolKindActiveAlerts:
		if queryTemplate != "" || defaultWindow != 0 || maxWindow != 0 || defaultStep != 0 {
			return fmt.Errorf("diagnosis tool template: active_alerts must not include query_template, windows, or step: %w", ErrInvariantViolation)
		}
		return validateDiagnosisToolTemplateLimit(defaultLimit, 10)
	case DiagnosisToolKindMetricQuery:
		if err := validateDiagnosisToolQueryTemplate(queryTemplate); err != nil {
			return err
		}
		if defaultWindow != 0 || maxWindow != 0 || defaultStep != 0 {
			return fmt.Errorf("diagnosis tool template: metric_query must not include windows or step: %w", ErrInvariantViolation)
		}
		return validateDiagnosisToolTemplateLimit(defaultLimit, 20)
	case DiagnosisToolKindMetricRangeQuery:
		if err := validateDiagnosisToolQueryTemplate(queryTemplate); err != nil {
			return err
		}
		if err := validateDiagnosisToolTemplateLimit(defaultLimit, 20); err != nil {
			return err
		}
		return validateDiagnosisToolTemplateRange(defaultWindow, maxWindow, defaultStep)
	default:
		return fmt.Errorf("diagnosis tool template: tool %q is unsupported: %w", tool, ErrInvariantViolation)
	}
}

func validateDiagnosisToolTemplateLimit(limit int, maximum int) error {
	if limit < 1 || limit > maximum {
		return fmt.Errorf("diagnosis tool template: default_limit must be between 1 and %d: %w", maximum, ErrInvariantViolation)
	}
	return nil
}

func validateDiagnosisToolQueryTemplate(queryTemplate string) error {
	if queryTemplate == "" {
		return fmt.Errorf("diagnosis tool template: query_template must be non-empty: %w", ErrInvariantViolation)
	}
	if len([]byte(queryTemplate)) > maxDiagnosisToolQueryTemplateLen {
		return fmt.Errorf("diagnosis tool template: query_template exceeds %d bytes: %w", maxDiagnosisToolQueryTemplateLen, ErrInvariantViolation)
	}
	if strings.Contains(queryTemplate, "{{") || strings.Contains(queryTemplate, "}}") {
		return fmt.Errorf("diagnosis tool template: query_template must not include unresolved template delimiters: %w", ErrInvariantViolation)
	}
	for _, r := range queryTemplate {
		if unicode.IsControl(r) {
			return fmt.Errorf("diagnosis tool template: query_template must be single-line: %w", ErrInvariantViolation)
		}
	}
	return nil
}

func validateDiagnosisToolTemplateRange(defaultWindow time.Duration, maxWindow time.Duration, defaultStep time.Duration) error {
	if defaultWindow < minDiagnosisToolRangeWindow || defaultWindow > maxDiagnosisToolRangeWindow {
		return fmt.Errorf("diagnosis tool template: default_window must be between %s and %s: %w", minDiagnosisToolRangeWindow, maxDiagnosisToolRangeWindow, ErrInvariantViolation)
	}
	if maxWindow < minDiagnosisToolRangeWindow || maxWindow > maxDiagnosisToolRangeWindow {
		return fmt.Errorf("diagnosis tool template: max_window must be between %s and %s: %w", minDiagnosisToolRangeWindow, maxDiagnosisToolRangeWindow, ErrInvariantViolation)
	}
	if maxWindow < defaultWindow {
		return fmt.Errorf("diagnosis tool template: max_window must be greater than or equal to default_window: %w", ErrInvariantViolation)
	}
	if defaultStep < minDiagnosisToolRangeStep {
		return fmt.Errorf("diagnosis tool template: default_step must be at least %s: %w", minDiagnosisToolRangeStep, ErrInvariantViolation)
	}
	if defaultStep > defaultWindow {
		return fmt.Errorf("diagnosis tool template: default_step must not exceed default_window: %w", ErrInvariantViolation)
	}
	return nil
}
