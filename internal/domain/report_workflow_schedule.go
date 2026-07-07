package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	maxReportWorkflowScheduleNameLen       = 120
	maxReportWorkflowScheduleTemporalIDLen = 200

	reportWorkflowScheduleCalendarDayUnset = 0
)

// ReportWorkflowScheduleCadence describes how Temporal should interpret the
// schedule fire times. Interval preserves the original fixed-period behavior;
// calendar cadences use Temporal calendar specs for natural UTC day/week/month
// boundaries.
type ReportWorkflowScheduleCadence string

const (
	// ReportWorkflowScheduleCadenceInterval schedules every Interval with Offset.
	ReportWorkflowScheduleCadenceInterval ReportWorkflowScheduleCadence = "interval"
	// ReportWorkflowScheduleCadenceDaily schedules once per UTC day at the
	// configured calendar hour/minute.
	ReportWorkflowScheduleCadenceDaily ReportWorkflowScheduleCadence = "daily"
	// ReportWorkflowScheduleCadenceWeekly schedules once per UTC week on the
	// configured day/hour/minute.
	ReportWorkflowScheduleCadenceWeekly ReportWorkflowScheduleCadence = "weekly"
	// ReportWorkflowScheduleCadenceMonthly schedules once per UTC month on the
	// configured day/hour/minute. Days are intentionally limited to 1-28 so every
	// month has a matching fire time.
	ReportWorkflowScheduleCadenceMonthly ReportWorkflowScheduleCadence = "monthly"
)

// Valid reports whether c is a supported schedule cadence.
func (c ReportWorkflowScheduleCadence) Valid() bool {
	switch c {
	case ReportWorkflowScheduleCadenceInterval,
		ReportWorkflowScheduleCadenceDaily,
		ReportWorkflowScheduleCadenceWeekly,
		ReportWorkflowScheduleCadenceMonthly:
		return true
	}
	return false
}

// ReportWorkflowSchedule is persisted schedule configuration for a report
// workflow policy. Domain construction and repository saves do not talk to
// Temporal; runtime synchronization happens at transport or startup boundaries
// when configured.
type ReportWorkflowSchedule struct {
	ID                     ReportWorkflowScheduleID
	Name                   string
	ReportWorkflowPolicyID ReportWorkflowPolicyID
	TemporalScheduleID     string
	Cadence                ReportWorkflowScheduleCadence
	CalendarHour           int
	CalendarMinute         int
	CalendarDayOfWeek      int
	CalendarDayOfMonth     int
	Interval               time.Duration
	Offset                 time.Duration
	ReplayWindow           time.Duration
	ReplayDelay            time.Duration
	ReplayLimit            int
	CatchupWindow          time.Duration
	Enabled                bool
	EnabledAt              *time.Time
	DisabledAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// NewReportWorkflowSchedule constructs a validated report workflow schedule.
func NewReportWorkflowSchedule(
	name string,
	reportWorkflowPolicyID ReportWorkflowPolicyID,
	temporalScheduleID string,
	interval time.Duration,
	offset time.Duration,
	replayWindow time.Duration,
	replayDelay time.Duration,
	replayLimit int,
	catchupWindow time.Duration,
	enabled bool,
	enabledAt *time.Time,
	disabledAt *time.Time,
) (ReportWorkflowSchedule, error) {
	return NewReportWorkflowScheduleWithCadence(
		name,
		reportWorkflowPolicyID,
		temporalScheduleID,
		ReportWorkflowScheduleCadenceInterval,
		0,
		0,
		reportWorkflowScheduleCalendarDayUnset,
		reportWorkflowScheduleCalendarDayUnset,
		interval,
		offset,
		replayWindow,
		replayDelay,
		replayLimit,
		catchupWindow,
		enabled,
		enabledAt,
		disabledAt,
	)
}

// NewReportWorkflowScheduleWithCadence constructs a validated report workflow
// schedule with either fixed-interval or UTC calendar cadence metadata.
func NewReportWorkflowScheduleWithCadence(
	name string,
	reportWorkflowPolicyID ReportWorkflowPolicyID,
	temporalScheduleID string,
	cadence ReportWorkflowScheduleCadence,
	calendarHour int,
	calendarMinute int,
	calendarDayOfWeek int,
	calendarDayOfMonth int,
	interval time.Duration,
	offset time.Duration,
	replayWindow time.Duration,
	replayDelay time.Duration,
	replayLimit int,
	catchupWindow time.Duration,
	enabled bool,
	enabledAt *time.Time,
	disabledAt *time.Time,
) (ReportWorkflowSchedule, error) {
	name = strings.TrimSpace(name)
	temporalScheduleID = strings.TrimSpace(temporalScheduleID)
	if cadence == "" {
		cadence = ReportWorkflowScheduleCadenceInterval
	}
	if name == "" {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: name must be non-empty: %w", ErrInvariantViolation)
	}
	if len(name) > maxReportWorkflowScheduleNameLen {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: name exceeds %d bytes: %w", maxReportWorkflowScheduleNameLen, ErrInvariantViolation)
	}
	if reportWorkflowPolicyID == 0 {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: report_workflow_policy_id must be non-zero: %w", ErrInvariantViolation)
	}
	if err := validateTemporalScheduleID(temporalScheduleID); err != nil {
		return ReportWorkflowSchedule{}, err
	}
	if err := validateReportWorkflowScheduleCadenceShape(
		cadence,
		calendarHour,
		calendarMinute,
		calendarDayOfWeek,
		calendarDayOfMonth,
		interval,
		offset,
		replayWindow,
	); err != nil {
		return ReportWorkflowSchedule{}, err
	}
	if interval <= 0 {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: interval must be positive: %w", ErrInvariantViolation)
	}
	if offset < 0 {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: offset must be non-negative: %w", ErrInvariantViolation)
	}
	if offset >= interval {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: offset must be less than interval: %w", ErrInvariantViolation)
	}
	if replayWindow <= 0 {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: replay_window must be positive: %w", ErrInvariantViolation)
	}
	if err := validateReportWorkflowScheduleReplayCadence(interval, replayWindow); err != nil {
		return ReportWorkflowSchedule{}, err
	}
	if replayDelay < 0 {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: replay_delay must be non-negative: %w", ErrInvariantViolation)
	}
	if replayLimit <= 0 {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: replay_limit must be positive: %w", ErrInvariantViolation)
	}
	if catchupWindow <= 0 {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: catchup_window must be positive: %w", ErrInvariantViolation)
	}
	if !enabled && enabledAt != nil {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: enabled_at requires enabled=true: %w", ErrInvariantViolation)
	}
	if enabled && disabledAt != nil {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: disabled_at requires enabled=false: %w", ErrInvariantViolation)
	}
	return ReportWorkflowSchedule{
		Name:                   name,
		ReportWorkflowPolicyID: reportWorkflowPolicyID,
		TemporalScheduleID:     temporalScheduleID,
		Cadence:                cadence,
		CalendarHour:           calendarHour,
		CalendarMinute:         calendarMinute,
		CalendarDayOfWeek:      calendarDayOfWeek,
		CalendarDayOfMonth:     calendarDayOfMonth,
		Interval:               interval,
		Offset:                 offset,
		ReplayWindow:           replayWindow,
		ReplayDelay:            replayDelay,
		ReplayLimit:            replayLimit,
		CatchupWindow:          catchupWindow,
		Enabled:                enabled,
		EnabledAt:              cloneTimePtr(enabledAt),
		DisabledAt:             cloneTimePtr(disabledAt),
	}, nil
}

// WithReportWorkflowScheduleEnabled returns a copy with explicit schedule
// enablement state updated at the supplied time.
func WithReportWorkflowScheduleEnabled(schedule ReportWorkflowSchedule, enabled bool, at time.Time) (ReportWorkflowSchedule, error) {
	if schedule.ID == 0 {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: id must be non-zero: %w", ErrInvariantViolation)
	}
	at = NormalizeUTCMicro(at)
	if at.IsZero() {
		return ReportWorkflowSchedule{}, fmt.Errorf("report workflow schedule: enablement time must be non-zero: %w", ErrInvariantViolation)
	}
	if enabled {
		if err := validateReportWorkflowScheduleReplayCadence(schedule.Interval, schedule.ReplayWindow); err != nil {
			return ReportWorkflowSchedule{}, err
		}
	}
	schedule.Enabled = enabled
	if enabled {
		schedule.EnabledAt = &at
		schedule.DisabledAt = nil
	} else {
		schedule.EnabledAt = nil
		schedule.DisabledAt = &at
	}
	return schedule, nil
}

func validateTemporalScheduleID(id string) error {
	if id == "" {
		return fmt.Errorf("report workflow schedule: temporal_schedule_id must be non-empty: %w", ErrInvariantViolation)
	}
	if len(id) > maxReportWorkflowScheduleTemporalIDLen {
		return fmt.Errorf("report workflow schedule: temporal_schedule_id exceeds %d bytes: %w", maxReportWorkflowScheduleTemporalIDLen, ErrInvariantViolation)
	}
	for _, r := range id {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("report workflow schedule: temporal_schedule_id must not contain whitespace or control characters: %w", ErrInvariantViolation)
		}
	}
	return nil
}

func validateReportWorkflowScheduleReplayCadence(interval, replayWindow time.Duration) error {
	if replayWindow > interval {
		return fmt.Errorf("report workflow schedule: replay_window must not exceed interval: %w", ErrInvariantViolation)
	}
	return nil
}

func validateReportWorkflowScheduleCadenceShape(
	cadence ReportWorkflowScheduleCadence,
	calendarHour int,
	calendarMinute int,
	calendarDayOfWeek int,
	calendarDayOfMonth int,
	interval time.Duration,
	offset time.Duration,
	replayWindow time.Duration,
) error {
	if !cadence.Valid() {
		return fmt.Errorf("report workflow schedule: cadence %q is unsupported: %w", cadence, ErrInvariantViolation)
	}
	if calendarHour < 0 || calendarHour > 23 {
		return fmt.Errorf("report workflow schedule: calendar_hour must be between 0 and 23: %w", ErrInvariantViolation)
	}
	if calendarMinute < 0 || calendarMinute > 59 {
		return fmt.Errorf("report workflow schedule: calendar_minute must be between 0 and 59: %w", ErrInvariantViolation)
	}
	switch cadence {
	case ReportWorkflowScheduleCadenceInterval:
		if calendarHour != 0 ||
			calendarMinute != 0 ||
			calendarDayOfWeek != reportWorkflowScheduleCalendarDayUnset ||
			calendarDayOfMonth != reportWorkflowScheduleCalendarDayUnset {
			return fmt.Errorf("report workflow schedule: interval cadence must not include calendar fields: %w", ErrInvariantViolation)
		}
	case ReportWorkflowScheduleCadenceDaily:
		if calendarDayOfWeek != reportWorkflowScheduleCalendarDayUnset ||
			calendarDayOfMonth != reportWorkflowScheduleCalendarDayUnset {
			return fmt.Errorf("report workflow schedule: daily cadence must not include calendar day fields: %w", ErrInvariantViolation)
		}
	case ReportWorkflowScheduleCadenceWeekly:
		if calendarDayOfWeek < 0 || calendarDayOfWeek > 6 {
			return fmt.Errorf("report workflow schedule: calendar_day_of_week must be between 0 and 6: %w", ErrInvariantViolation)
		}
		if calendarDayOfMonth != reportWorkflowScheduleCalendarDayUnset {
			return fmt.Errorf("report workflow schedule: weekly cadence must not include calendar_day_of_month: %w", ErrInvariantViolation)
		}
	case ReportWorkflowScheduleCadenceMonthly:
		if calendarDayOfMonth < 1 || calendarDayOfMonth > 28 {
			return fmt.Errorf("report workflow schedule: calendar_day_of_month must be between 1 and 28: %w", ErrInvariantViolation)
		}
		if calendarDayOfWeek != reportWorkflowScheduleCalendarDayUnset {
			return fmt.Errorf("report workflow schedule: monthly cadence must not include calendar_day_of_week: %w", ErrInvariantViolation)
		}
	}
	if cadence != ReportWorkflowScheduleCadenceInterval {
		if offset != 0 {
			return fmt.Errorf("report workflow schedule: offset must be zero for calendar cadence: %w", ErrInvariantViolation)
		}
		if interval <= 0 || offset < 0 || offset >= interval {
			return nil
		}
		if replayWindow > interval {
			return fmt.Errorf("report workflow schedule: replay_window must not exceed cadence guard interval: %w", ErrInvariantViolation)
		}
	}
	return nil
}
