package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewAlertWindowNormalizesBounds(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	start := time.Date(2026, 7, 13, 9, 30, 0, 123456789, location)
	end := start.Add(90*time.Minute + 400*time.Nanosecond)

	window, err := NewAlertWindow(start, end)
	if err != nil {
		t.Fatalf("NewAlertWindow: %v", err)
	}

	wantStart := start.UTC().Truncate(time.Microsecond)
	wantEnd := end.UTC().Truncate(time.Microsecond)
	if got := window.StartInclusive(); !got.Equal(wantStart) || got.Location() != time.UTC {
		t.Fatalf("StartInclusive = %s (%s), want %s (UTC)", got, got.Location(), wantStart)
	}
	if got := window.EndExclusive(); !got.Equal(wantEnd) || got.Location() != time.UTC {
		t.Fatalf("EndExclusive = %s (%s), want %s (UTC)", got, got.Location(), wantEnd)
	}
}

func TestNewAlertWindowRejectsInvalidBounds(t *testing.T) {
	start := time.Date(2026, 7, 13, 1, 30, 0, 0, time.UTC)
	tests := []struct {
		name  string
		start time.Time
		end   time.Time
	}{
		{name: "missing start", end: start.Add(time.Minute)},
		{name: "missing end", start: start},
		{name: "equal bounds", start: start, end: start},
		{name: "reversed bounds", start: start.Add(time.Minute), end: start},
		{
			name:  "sub-microsecond interval collapses",
			start: start.Add(500 * time.Nanosecond),
			end:   start.Add(800 * time.Nanosecond),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAlertWindow(tt.start, tt.end)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("NewAlertWindow error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}
