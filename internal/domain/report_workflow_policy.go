package domain

import (
	"fmt"
	"strings"
	"time"
)

const maxReportWorkflowPolicyNameLen = 120

// ReportWorkflowTriggerMode describes how a report workflow policy may be
// invoked. The first persisted mode binds the existing replay-window trigger;
// scheduled triggers remain a later extension.
type ReportWorkflowTriggerMode string

const (
	// ReportWorkflowTriggerModeManualReplay binds the policy to operator- or
	// API-initiated replay-window report starts.
	ReportWorkflowTriggerModeManualReplay ReportWorkflowTriggerMode = "manual_replay"
)

// Valid reports whether m is a supported report workflow trigger mode.
func (m ReportWorkflowTriggerMode) Valid() bool {
	switch m {
	case ReportWorkflowTriggerModeManualReplay:
		return true
	}
	return false
}

// ReportWorkflowScenario describes the prompt variant used by generated
// SubReports.
type ReportWorkflowScenario string

const (
	// ReportWorkflowScenarioSingleAlert is for one isolated alert group.
	ReportWorkflowScenarioSingleAlert ReportWorkflowScenario = "single_alert"
	// ReportWorkflowScenarioCascade is for causally related alerts across
	// services.
	ReportWorkflowScenarioCascade ReportWorkflowScenario = "cascade"
	// ReportWorkflowScenarioAlertStorm is for broad alert bursts with shared
	// context.
	ReportWorkflowScenarioAlertStorm ReportWorkflowScenario = "alert_storm"
)

// Valid reports whether s is a supported report prompt scenario.
func (s ReportWorkflowScenario) Valid() bool {
	switch s {
	case ReportWorkflowScenarioSingleAlert, ReportWorkflowScenarioCascade, ReportWorkflowScenarioAlertStorm:
		return true
	}
	return false
}

// DiagnosisFollowUpMode describes how report policies should prepare the
// diagnosis-room handoff after a report is created or when an alert intake
// trigger produces a matching evidence snapshot.
type DiagnosisFollowUpMode string

const (
	// DiagnosisFollowUpModeDisabled records no diagnosis follow-up request.
	DiagnosisFollowUpModeDisabled DiagnosisFollowUpMode = "disabled"
	// DiagnosisFollowUpModeSuggestRoom asks the UI or later workflow binding
	// to offer a diagnosis room handoff after report creation.
	DiagnosisFollowUpModeSuggestRoom DiagnosisFollowUpMode = "suggest_room"
	// DiagnosisFollowUpModeAutoRoom allows backend-owned alert intake paths to
	// start a diagnosis room automatically after producing a matching snapshot.
	DiagnosisFollowUpModeAutoRoom DiagnosisFollowUpMode = "auto_room"
)

// Valid reports whether m is a supported diagnosis follow-up mode.
func (m DiagnosisFollowUpMode) Valid() bool {
	switch m {
	case DiagnosisFollowUpModeDisabled, DiagnosisFollowUpModeSuggestRoom, DiagnosisFollowUpModeAutoRoom:
		return true
	}
	return false
}

// ReportWorkflowPolicy is operator-managed report workflow configuration.
// Saving or replacing this profile never starts Temporal workflows. Enabled
// state is changed only through explicit enable/disable actions.
type ReportWorkflowPolicy struct {
	ID                                 ReportWorkflowPolicyID
	Name                               string
	AlertSourceProfileID               AlertSourceProfileID
	GroupingPolicyID                   GroupingPolicyID
	ReportNotificationChannelProfileID NotificationChannelProfileID
	TriggerMode                        ReportWorkflowTriggerMode
	ReportScenario                     ReportWorkflowScenario
	DiagnosisFollowUp                  DiagnosisFollowUpMode
	Enabled                            bool
	EnabledAt                          *time.Time
	DisabledAt                         *time.Time
	CreatedAt                          time.Time
	UpdatedAt                          time.Time
}

// NewReportWorkflowPolicy constructs a validated report workflow policy.
func NewReportWorkflowPolicy(
	name string,
	alertSourceProfileID AlertSourceProfileID,
	groupingPolicyID GroupingPolicyID,
	reportNotificationChannelProfileID NotificationChannelProfileID,
	triggerMode ReportWorkflowTriggerMode,
	reportScenario ReportWorkflowScenario,
	diagnosisFollowUp DiagnosisFollowUpMode,
	enabled bool,
	enabledAt *time.Time,
	disabledAt *time.Time,
) (ReportWorkflowPolicy, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: name must be non-empty: %w", ErrInvariantViolation)
	}
	if len(name) > maxReportWorkflowPolicyNameLen {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: name exceeds %d bytes: %w", maxReportWorkflowPolicyNameLen, ErrInvariantViolation)
	}
	if alertSourceProfileID == 0 {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: alert_source_profile_id must be non-zero: %w", ErrInvariantViolation)
	}
	if groupingPolicyID == 0 {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: grouping_policy_id must be non-zero: %w", ErrInvariantViolation)
	}
	if reportNotificationChannelProfileID < 0 {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: report_notification_channel_profile_id must be non-negative: %w", ErrInvariantViolation)
	}
	if !triggerMode.Valid() {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: trigger_mode %q is unsupported: %w", triggerMode, ErrInvariantViolation)
	}
	if !reportScenario.Valid() {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: report_scenario %q is unsupported: %w", reportScenario, ErrInvariantViolation)
	}
	if !diagnosisFollowUp.Valid() {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: diagnosis_follow_up %q is unsupported: %w", diagnosisFollowUp, ErrInvariantViolation)
	}
	if !enabled && enabledAt != nil {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: enabled_at requires enabled=true: %w", ErrInvariantViolation)
	}
	if enabled && disabledAt != nil {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: disabled_at requires enabled=false: %w", ErrInvariantViolation)
	}
	return ReportWorkflowPolicy{
		Name:                               name,
		AlertSourceProfileID:               alertSourceProfileID,
		GroupingPolicyID:                   groupingPolicyID,
		ReportNotificationChannelProfileID: reportNotificationChannelProfileID,
		TriggerMode:                        triggerMode,
		ReportScenario:                     reportScenario,
		DiagnosisFollowUp:                  diagnosisFollowUp,
		Enabled:                            enabled,
		EnabledAt:                          cloneTimePtr(enabledAt),
		DisabledAt:                         cloneTimePtr(disabledAt),
	}, nil
}

// WithReportWorkflowPolicyEnabled returns a copy with explicit enablement
// state updated at the supplied time.
func WithReportWorkflowPolicyEnabled(policy ReportWorkflowPolicy, enabled bool, at time.Time) (ReportWorkflowPolicy, error) {
	if policy.ID == 0 {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: id must be non-zero: %w", ErrInvariantViolation)
	}
	at = NormalizeUTCMicro(at)
	if at.IsZero() {
		return ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy: enablement time must be non-zero: %w", ErrInvariantViolation)
	}
	policy.Enabled = enabled
	if enabled {
		policy.EnabledAt = &at
		policy.DisabledAt = nil
	} else {
		policy.EnabledAt = nil
		policy.DisabledAt = &at
	}
	return policy, nil
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
