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
		schedule.Cadence != ReportWorkflowScheduleCadenceInterval ||
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

func TestNewReportWorkflowScheduleWithCadenceAcceptsCalendarCadences(t *testing.T) {
	tests := []struct {
		name               string
		cadence            ReportWorkflowScheduleCadence
		calendarHour       int
		calendarMinute     int
		calendarDayOfWeek  int
		calendarDayOfMonth int
		guardInterval      time.Duration
	}{
		{
			name:           "daily",
			cadence:        ReportWorkflowScheduleCadenceDaily,
			calendarHour:   2,
			calendarMinute: 30,
			guardInterval:  24 * time.Hour,
		},
		{
			name:              "weekly",
			cadence:           ReportWorkflowScheduleCadenceWeekly,
			calendarHour:      3,
			calendarMinute:    15,
			calendarDayOfWeek: 1,
			guardInterval:     7 * 24 * time.Hour,
		},
		{
			name:               "monthly",
			cadence:            ReportWorkflowScheduleCadenceMonthly,
			calendarHour:       4,
			calendarMinute:     45,
			calendarDayOfMonth: 1,
			guardInterval:      28 * 24 * time.Hour,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schedule, err := NewReportWorkflowScheduleWithCadence(
				"Calendar reports",
				ReportWorkflowPolicyID(7),
				"openclarion-report-policy-7-calendar",
				tc.cadence,
				tc.calendarHour,
				tc.calendarMinute,
				tc.calendarDayOfWeek,
				tc.calendarDayOfMonth,
				tc.guardInterval,
				0,
				time.Hour,
				2*time.Minute,
				1000,
				10*time.Minute,
				false,
				nil,
				nil,
			)
			if err != nil {
				t.Fatalf("NewReportWorkflowScheduleWithCadence: %v", err)
			}
			if schedule.Cadence != tc.cadence ||
				schedule.CalendarHour != tc.calendarHour ||
				schedule.CalendarMinute != tc.calendarMinute ||
				schedule.CalendarDayOfWeek != tc.calendarDayOfWeek ||
				schedule.CalendarDayOfMonth != tc.calendarDayOfMonth ||
				schedule.Offset != 0 {
				t.Fatalf("schedule = %+v", schedule)
			}
		})
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
		{name: "replay_window_exceeds_interval", edit: func(a *reportWorkflowScheduleArgs) { a.replayWindow = 2 * time.Hour }},
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

func TestNewReportWorkflowScheduleWithCadenceRejectsInvalidCalendarInputs(t *testing.T) {
	tests := []struct {
		name string
		edit func(*reportWorkflowScheduleArgs)
	}{
		{name: "unsupported_cadence", edit: func(a *reportWorkflowScheduleArgs) { a.cadence = ReportWorkflowScheduleCadence("yearly") }},
		{name: "bad_hour", edit: func(a *reportWorkflowScheduleArgs) { a.calendarHour = 24 }},
		{name: "bad_minute", edit: func(a *reportWorkflowScheduleArgs) { a.calendarMinute = 60 }},
		{name: "interval_with_calendar_hour", edit: func(a *reportWorkflowScheduleArgs) { a.calendarHour = 2 }},
		{name: "daily_with_day_of_week", edit: func(a *reportWorkflowScheduleArgs) {
			a.cadence = ReportWorkflowScheduleCadenceDaily
			a.calendarDayOfWeek = 1
			a.interval = 24 * time.Hour
		}},
		{name: "weekly_bad_day", edit: func(a *reportWorkflowScheduleArgs) {
			a.cadence = ReportWorkflowScheduleCadenceWeekly
			a.calendarDayOfWeek = 7
			a.interval = 7 * 24 * time.Hour
		}},
		{name: "weekly_with_day_of_month", edit: func(a *reportWorkflowScheduleArgs) {
			a.cadence = ReportWorkflowScheduleCadenceWeekly
			a.calendarDayOfWeek = 1
			a.calendarDayOfMonth = 1
			a.interval = 7 * 24 * time.Hour
		}},
		{name: "monthly_zero_day", edit: func(a *reportWorkflowScheduleArgs) {
			a.cadence = ReportWorkflowScheduleCadenceMonthly
			a.interval = 28 * 24 * time.Hour
		}},
		{name: "monthly_day_29", edit: func(a *reportWorkflowScheduleArgs) {
			a.cadence = ReportWorkflowScheduleCadenceMonthly
			a.calendarDayOfMonth = 29
			a.interval = 28 * 24 * time.Hour
		}},
		{name: "monthly_with_day_of_week", edit: func(a *reportWorkflowScheduleArgs) {
			a.cadence = ReportWorkflowScheduleCadenceMonthly
			a.calendarDayOfWeek = 1
			a.calendarDayOfMonth = 1
			a.interval = 28 * 24 * time.Hour
		}},
		{name: "calendar_offset", edit: func(a *reportWorkflowScheduleArgs) {
			a.cadence = ReportWorkflowScheduleCadenceDaily
			a.offset = time.Minute
			a.interval = 24 * time.Hour
		}},
		{name: "calendar_replay_exceeds_guard", edit: func(a *reportWorkflowScheduleArgs) {
			a.cadence = ReportWorkflowScheduleCadenceDaily
			a.interval = time.Hour
			a.replayWindow = 2 * time.Hour
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := defaultReportWorkflowScheduleArgs()
			tc.edit(&args)
			_, err := NewReportWorkflowScheduleWithCadence(
				args.name,
				args.policyID,
				args.temporalID,
				args.cadence,
				args.calendarHour,
				args.calendarMinute,
				args.calendarDayOfWeek,
				args.calendarDayOfMonth,
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

func TestWithReportWorkflowScheduleEnabledRejectsOverlappingReplayWindow(t *testing.T) {
	schedule := mustReportWorkflowSchedule(t)
	schedule.ID = 9
	schedule.Interval = time.Minute
	schedule.ReplayWindow = time.Hour

	_, err := WithReportWorkflowScheduleEnabled(schedule, true, time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrInvariantViolation) {
		t.Fatalf("enable err = %v, want ErrInvariantViolation", err)
	}

	disabled, err := WithReportWorkflowScheduleEnabled(schedule, false, time.Date(2026, 6, 5, 8, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("disable invalid historical schedule: %v", err)
	}
	if disabled.Enabled || disabled.DisabledAt == nil {
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
	name               string
	policyID           ReportWorkflowPolicyID
	temporalID         string
	cadence            ReportWorkflowScheduleCadence
	calendarHour       int
	calendarMinute     int
	calendarDayOfWeek  int
	calendarDayOfMonth int
	interval           time.Duration
	offset             time.Duration
	replayWindow       time.Duration
	replayDelay        time.Duration
	replayLimit        int
	catchupWindow      time.Duration
	enabled            bool
	enabledAt          *time.Time
	disabledAt         *time.Time
}

func defaultReportWorkflowScheduleArgs() reportWorkflowScheduleArgs {
	return reportWorkflowScheduleArgs{
		name:          "Hourly reports",
		policyID:      7,
		temporalID:    "openclarion-report-policy-7-hourly",
		cadence:       ReportWorkflowScheduleCadenceInterval,
		interval:      time.Hour,
		offset:        0,
		replayWindow:  30 * time.Minute,
		replayDelay:   2 * time.Minute,
		replayLimit:   1000,
		catchupWindow: 10 * time.Minute,
	}
}
