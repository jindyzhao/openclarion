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
)

// ReportWorkflowSchedule is persisted schedule configuration for a report
// workflow policy. Domain construction and repository saves do not talk to
// Temporal; runtime synchronization happens at transport or startup boundaries
// when configured.
type ReportWorkflowSchedule struct {
	ID                     ReportWorkflowScheduleID
	Name                   string
	ReportWorkflowPolicyID ReportWorkflowPolicyID
	TemporalScheduleID     string
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
	name = strings.TrimSpace(name)
	temporalScheduleID = strings.TrimSpace(temporalScheduleID)
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
