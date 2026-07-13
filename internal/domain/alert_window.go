package domain

import (
	"fmt"
	"time"
)

// AlertWindow is a normalized half-open alert interval
// [StartInclusive, EndExclusive).
type AlertWindow struct {
	startInclusive time.Time
	endExclusive   time.Time
}

// NewAlertWindow validates and normalizes a half-open alert interval to the
// PostgreSQL timestamp precision used by alert persistence.
func NewAlertWindow(startInclusive, endExclusive time.Time) (AlertWindow, error) {
	if startInclusive.IsZero() {
		return AlertWindow{}, fmt.Errorf("alert window: start must be set: %w", ErrInvariantViolation)
	}
	if endExclusive.IsZero() {
		return AlertWindow{}, fmt.Errorf("alert window: end must be set: %w", ErrInvariantViolation)
	}

	start := NormalizeUTCMicro(startInclusive)
	end := NormalizeUTCMicro(endExclusive)
	if !end.After(start) {
		return AlertWindow{}, fmt.Errorf(
			"alert window: end %s must be strictly after start %s after normalization: %w",
			end,
			start,
			ErrInvariantViolation,
		)
	}

	return AlertWindow{startInclusive: start, endExclusive: end}, nil
}

// StartInclusive returns the normalized inclusive lower bound.
func (w AlertWindow) StartInclusive() time.Time {
	return w.startInclusive
}

// EndExclusive returns the normalized exclusive upper bound.
func (w AlertWindow) EndExclusive() time.Time {
	return w.endExclusive
}
