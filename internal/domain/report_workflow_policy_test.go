package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewReportWorkflowPolicyValidatesAndTrims(t *testing.T) {
	policy, err := NewReportWorkflowPolicy(
		" Default report policy ",
		AlertSourceProfileID(1),
		GroupingPolicyID(2),
		NotificationChannelProfileID(3),
		ReportWorkflowTriggerModeManualReplay,
		ReportWorkflowScenarioCascade,
		DiagnosisFollowUpModeAutoRoom,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowPolicy: %v", err)
	}
	if policy.Name != "Default report policy" ||
		policy.AlertSourceProfileID != 1 ||
		policy.GroupingPolicyID != 2 ||
		policy.ReportNotificationChannelProfileID != 3 ||
		policy.ReportScenario != ReportWorkflowScenarioCascade ||
		policy.DiagnosisFollowUp != DiagnosisFollowUpModeAutoRoom ||
		policy.Enabled {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestNewReportWorkflowPolicyRejectsInvalidInputs(t *testing.T) {
	now := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		edit func(*reportWorkflowPolicyArgs)
	}{
		{name: "empty_name", edit: func(a *reportWorkflowPolicyArgs) { a.name = " " }},
		{name: "missing_source", edit: func(a *reportWorkflowPolicyArgs) { a.sourceID = 0 }},
		{name: "missing_grouping", edit: func(a *reportWorkflowPolicyArgs) { a.groupingID = 0 }},
		{name: "negative_report_channel", edit: func(a *reportWorkflowPolicyArgs) { a.reportChannelID = -1 }},
		{name: "bad_trigger", edit: func(a *reportWorkflowPolicyArgs) { a.triggerMode = "scheduled" }},
		{name: "bad_scenario", edit: func(a *reportWorkflowPolicyArgs) { a.scenario = "unknown" }},
		{name: "bad_followup", edit: func(a *reportWorkflowPolicyArgs) { a.followUp = "auto_start" }},
		{name: "disabled_with_enabled_time", edit: func(a *reportWorkflowPolicyArgs) { a.enabledAt = &now }},
		{name: "enabled_with_disabled_time", edit: func(a *reportWorkflowPolicyArgs) {
			a.enabled = true
			a.disabledAt = &now
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := defaultReportWorkflowPolicyArgs()
			tc.edit(&args)
			_, err := NewReportWorkflowPolicy(
				args.name,
				args.sourceID,
				args.groupingID,
				args.reportChannelID,
				args.triggerMode,
				args.scenario,
				args.followUp,
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

func TestWithReportWorkflowPolicyEnabledTogglesExplicitState(t *testing.T) {
	policy, err := NewReportWorkflowPolicy(
		"Default report policy",
		AlertSourceProfileID(1),
		GroupingPolicyID(2),
		0,
		ReportWorkflowTriggerModeManualReplay,
		ReportWorkflowScenarioSingleAlert,
		DiagnosisFollowUpModeDisabled,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowPolicy: %v", err)
	}
	policy.ID = 7

	enabledAt := time.Date(2026, 6, 5, 8, 0, 0, 123456789, time.FixedZone("offset", 3600))
	enabled, err := WithReportWorkflowPolicyEnabled(policy, true, enabledAt)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !enabled.Enabled || enabled.EnabledAt == nil || !enabled.EnabledAt.Equal(time.Date(2026, 6, 5, 7, 0, 0, 123456000, time.UTC)) || enabled.DisabledAt != nil {
		t.Fatalf("enabled = %+v", enabled)
	}

	disabledAt := enabledAt.Add(time.Minute)
	disabled, err := WithReportWorkflowPolicyEnabled(enabled, false, disabledAt)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if disabled.Enabled || disabled.EnabledAt != nil || disabled.DisabledAt == nil || !disabled.DisabledAt.Equal(time.Date(2026, 6, 5, 7, 1, 0, 123456000, time.UTC)) {
		t.Fatalf("disabled = %+v", disabled)
	}
}

type reportWorkflowPolicyArgs struct {
	name            string
	sourceID        AlertSourceProfileID
	groupingID      GroupingPolicyID
	reportChannelID NotificationChannelProfileID
	triggerMode     ReportWorkflowTriggerMode
	scenario        ReportWorkflowScenario
	followUp        DiagnosisFollowUpMode
	enabled         bool
	enabledAt       *time.Time
	disabledAt      *time.Time
}

func defaultReportWorkflowPolicyArgs() reportWorkflowPolicyArgs {
	return reportWorkflowPolicyArgs{
		name:            "Default report policy",
		sourceID:        AlertSourceProfileID(1),
		groupingID:      GroupingPolicyID(2),
		reportChannelID: 0,
		triggerMode:     ReportWorkflowTriggerModeManualReplay,
		scenario:        ReportWorkflowScenarioSingleAlert,
		followUp:        DiagnosisFollowUpModeDisabled,
	}
}
