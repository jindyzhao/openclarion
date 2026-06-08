package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewReportWorkflowScheduleValidatesAndTrims(t *testing.T) {
	schedule, err := NewReportWorkflowSchedule(
		" Hourly reports ",
		ReportWorkflowPolicyID(7),
		" openclarion-report-policy-7-hourly ",
		time.Hour,
		5*time.Minute,
		30*time.Minute,
		2*time.Minute,
		1000,
		10*time.Minute,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowSchedule: %v", err)
	}
	if schedule.Name != "Hourly reports" ||
		schedule.ReportWorkflowPolicyID != 7 ||
		schedule.TemporalScheduleID != "openclarion-report-policy-7-hourly" ||
		schedule.Interval != time.Hour ||
		schedule.Offset != 5*time.Minute ||
		schedule.ReplayWindow != 30*time.Minute ||
		schedule.ReplayDelay != 2*time.Minute ||
		schedule.ReplayLimit != 1000 ||
		schedule.CatchupWindow != 10*time.Minute ||
		schedule.Enabled {
		t.Fatalf("schedule = %+v", schedule)
	}
}

func TestNewReportWorkflowScheduleRejectsInvalidInputs(t *testing.T) {
	now := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		edit func(*reportWorkflowScheduleArgs)
	}{
		{name: "empty_name", edit: func(a *reportWorkflowScheduleArgs) { a.name = " " }},
		{name: "missing_policy", edit: func(a *reportWorkflowScheduleArgs) { a.policyID = 0 }},
		{name: "empty_temporal_id", edit: func(a *reportWorkflowScheduleArgs) { a.temporalID = " " }},
		{name: "space_temporal_id", edit: func(a *reportWorkflowScheduleArgs) { a.temporalID = "bad id" }},
		{name: "zero_interval", edit: func(a *reportWorkflowScheduleArgs) { a.interval = 0 }},
		{name: "negative_offset", edit: func(a *reportWorkflowScheduleArgs) { a.offset = -time.Second }},
		{name: "offset_equals_interval", edit: func(a *reportWorkflowScheduleArgs) { a.offset = time.Hour }},
		{name: "zero_replay_window", edit: func(a *reportWorkflowScheduleArgs) { a.replayWindow = 0 }},
		{name: "negative_replay_delay", edit: func(a *reportWorkflowScheduleArgs) { a.replayDelay = -time.Second }},
		{name: "zero_replay_limit", edit: func(a *reportWorkflowScheduleArgs) { a.replayLimit = 0 }},
		{name: "zero_catchup", edit: func(a *reportWorkflowScheduleArgs) { a.catchupWindow = 0 }},
		{name: "disabled_with_enabled_time", edit: func(a *reportWorkflowScheduleArgs) { a.enabledAt = &now }},
		{name: "enabled_with_disabled_time", edit: func(a *reportWorkflowScheduleArgs) {
			a.enabled = true
			a.disabledAt = &now
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := defaultReportWorkflowScheduleArgs()
			tc.edit(&args)
			_, err := NewReportWorkflowSchedule(
				args.name,
				args.policyID,
				args.temporalID,
				args.interval,
				args.offset,
				args.replayWindow,
				args.replayDelay,
				args.replayLimit,
				args.catchupWindow,
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

func TestWithReportWorkflowScheduleEnabledTogglesExplicitState(t *testing.T) {
	schedule := mustReportWorkflowSchedule(t)
	schedule.ID = 9

	enabledAt := time.Date(2026, 6, 5, 8, 0, 0, 123456789, time.FixedZone("offset", 3600))
	enabled, err := WithReportWorkflowScheduleEnabled(schedule, true, enabledAt)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !enabled.Enabled || enabled.EnabledAt == nil || !enabled.EnabledAt.Equal(time.Date(2026, 6, 5, 7, 0, 0, 123456000, time.UTC)) || enabled.DisabledAt != nil {
		t.Fatalf("enabled = %+v", enabled)
	}

	disabledAt := enabledAt.Add(time.Minute)
	disabled, err := WithReportWorkflowScheduleEnabled(enabled, false, disabledAt)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if disabled.Enabled || disabled.EnabledAt != nil || disabled.DisabledAt == nil || !disabled.DisabledAt.Equal(time.Date(2026, 6, 5, 7, 1, 0, 123456000, time.UTC)) {
		t.Fatalf("disabled = %+v", disabled)
	}
}

func mustReportWorkflowSchedule(t *testing.T) ReportWorkflowSchedule {
	t.Helper()
	schedule, err := NewReportWorkflowSchedule(
		"Hourly reports",
		ReportWorkflowPolicyID(7),
		"openclarion-report-policy-7-hourly",
		time.Hour,
		0,
		30*time.Minute,
		2*time.Minute,
		1000,
		10*time.Minute,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowSchedule: %v", err)
	}
	return schedule
}

type reportWorkflowScheduleArgs struct {
	name          string
	policyID      ReportWorkflowPolicyID
	temporalID    string
	interval      time.Duration
	offset        time.Duration
	replayWindow  time.Duration
	replayDelay   time.Duration
	replayLimit   int
	catchupWindow time.Duration
	enabled       bool
	enabledAt     *time.Time
	disabledAt    *time.Time
}

func defaultReportWorkflowScheduleArgs() reportWorkflowScheduleArgs {
	return reportWorkflowScheduleArgs{
		name:          "Hourly reports",
		policyID:      7,
		temporalID:    "openclarion-report-policy-7-hourly",
		interval:      time.Hour,
		offset:        0,
		replayWindow:  30 * time.Minute,
		replayDelay:   2 * time.Minute,
		replayLimit:   1000,
		catchupWindow: 10 * time.Minute,
	}
}
