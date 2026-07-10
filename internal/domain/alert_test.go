package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewAlertEvent(t *testing.T) {
	t.Parallel()

	startsAt := time.Date(2026, 5, 22, 10, 30, 0, 123456789, time.UTC)

	t.Run("happy path normalises time and defaults maps", func(t *testing.T) {
		t.Parallel()
		e, err := NewAlertEvent("alertmanager", "fp-1", "canonical-1", nil, nil, nil, startsAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.Status != AlertStatusFiring {
			t.Fatalf("status = %q, want %q", e.Status, AlertStatusFiring)
		}
		want := startsAt.Truncate(time.Microsecond)
		if !e.StartsAt.Equal(want) {
			t.Fatalf("StartsAt = %s, want %s", e.StartsAt, want)
		}
		if e.Labels == nil || e.Annotations == nil {
			t.Fatalf("Labels/Annotations must be non-nil maps after normalisation")
		}
	})

	cases := []struct {
		name                 string
		source               string
		sourceFingerprint    string
		canonicalFingerprint string
		startsAt             time.Time
	}{
		{name: "empty source", sourceFingerprint: "fp", canonicalFingerprint: "c", startsAt: startsAt},
		{name: "empty source fingerprint", source: "alertmanager", canonicalFingerprint: "c", startsAt: startsAt},
		{name: "empty canonical fingerprint", source: "alertmanager", sourceFingerprint: "fp", startsAt: startsAt},
		{name: "zero starts_at", source: "alertmanager", sourceFingerprint: "fp", canonicalFingerprint: "c"},
		{name: "whitespace-only source", source: "   ", sourceFingerprint: "fp", canonicalFingerprint: "c", startsAt: startsAt},
		{name: "whitespace-only source fingerprint", source: "alertmanager", sourceFingerprint: "  \t ", canonicalFingerprint: "c", startsAt: startsAt},
		{name: "whitespace-only canonical fingerprint", source: "alertmanager", sourceFingerprint: "fp", canonicalFingerprint: "  \n ", startsAt: startsAt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewAlertEvent(tc.source, tc.sourceFingerprint, tc.canonicalFingerprint, nil, nil, nil, tc.startsAt)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}

	t.Run("surrounding whitespace is trimmed", func(t *testing.T) {
		t.Parallel()
		e, err := NewAlertEvent("  alertmanager  ", " fp-1 ", " canonical-1 ", nil, nil, nil, startsAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.Source != "alertmanager" {
			t.Errorf("Source = %q, want %q", e.Source, "alertmanager")
		}
		if e.SourceFingerprint != "fp-1" {
			t.Errorf("SourceFingerprint = %q, want %q", e.SourceFingerprint, "fp-1")
		}
		if e.CanonicalFingerprint != "canonical-1" {
			t.Errorf("CanonicalFingerprint = %q, want %q", e.CanonicalFingerprint, "canonical-1")
		}
	})

	t.Run("source profile is optional but cannot be negative", func(t *testing.T) {
		t.Parallel()
		e, err := NewAlertEvent("alertmanager", "fp-1", "canonical-1", nil, nil, nil, startsAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tagged, err := e.WithAlertSourceProfile(7)
		if err != nil {
			t.Fatalf("WithAlertSourceProfile: %v", err)
		}
		if tagged.AlertSourceProfileID != 7 {
			t.Fatalf("AlertSourceProfileID = %d, want 7", tagged.AlertSourceProfileID)
		}
		_, err = e.WithAlertSourceProfile(-1)
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})
}

func TestAlertEvent_Resolve(t *testing.T) {
	t.Parallel()

	startsAt := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	endsAt := time.Date(2026, 5, 22, 10, 5, 0, 0, time.UTC)

	base, err := NewAlertEvent("alertmanager", "fp", "c", nil, nil, nil, startsAt)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("happy path sets resolved + ends_at", func(t *testing.T) {
		t.Parallel()
		got, err := base.Resolve(endsAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != AlertStatusResolved {
			t.Fatalf("status = %q, want %q", got.Status, AlertStatusResolved)
		}
		if got.EndsAt == nil || !got.EndsAt.Equal(endsAt) {
			t.Fatalf("EndsAt = %v, want %s", got.EndsAt, endsAt)
		}
	})

	t.Run("zero ends_at is invariant violation", func(t *testing.T) {
		t.Parallel()
		_, err := base.Resolve(time.Time{})
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("ends_at before starts_at is invariant violation", func(t *testing.T) {
		t.Parallel()
		_, err := base.Resolve(startsAt.Add(-time.Second))
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("re-resolve same ends_at is idempotent", func(t *testing.T) {
		t.Parallel()
		first, err := base.Resolve(endsAt)
		if err != nil {
			t.Fatalf("first resolve: %v", err)
		}
		second, err := first.Resolve(endsAt)
		if err != nil {
			t.Fatalf("second resolve: %v", err)
		}
		if second.EndsAt == nil || !second.EndsAt.Equal(endsAt) {
			t.Fatalf("idempotent re-resolve drifted ends_at")
		}
	})

	t.Run("re-resolve different ends_at is invariant violation", func(t *testing.T) {
		t.Parallel()
		first, err := base.Resolve(endsAt)
		if err != nil {
			t.Fatalf("first resolve: %v", err)
		}
		_, err = first.Resolve(endsAt.Add(time.Second))
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("resolved status without ends_at is invariant violation", func(t *testing.T) {
		t.Parallel()
		invalid := base
		invalid.Status = AlertStatusResolved
		_, err := invalid.Resolve(endsAt)
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("unknown source status is invariant violation", func(t *testing.T) {
		t.Parallel()
		invalid := base
		invalid.Status = AlertStatus("unknown")
		_, err := invalid.Resolve(endsAt)
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})
}

func TestNewAlertGroup(t *testing.T) {
	t.Parallel()

	first := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	last := first.Add(5 * time.Minute)

	t.Run("happy path defaults to active", func(t *testing.T) {
		t.Parallel()
		g, err := NewAlertGroup("k", []byte(`{"service":"foo"}`), GroupSeverityCritical, 3, first, last, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if g.Status != AlertGroupStatusActive {
			t.Fatalf("status = %q, want %q", g.Status, AlertGroupStatusActive)
		}
	})

	cases := []struct {
		name        string
		groupKey    string
		severity    GroupSeverity
		eventCount  int
		firstSeenAt time.Time
		lastSeenAt  time.Time
	}{
		{name: "empty group_key", severity: GroupSeverityWarning, firstSeenAt: first, lastSeenAt: last},
		{name: "zero first_seen_at", groupKey: "k", severity: GroupSeverityWarning, lastSeenAt: last},
		{name: "zero last_seen_at", groupKey: "k", severity: GroupSeverityWarning, firstSeenAt: first},
		{name: "last before first", groupKey: "k", severity: GroupSeverityWarning, firstSeenAt: last, lastSeenAt: first},
		{name: "unknown severity", groupKey: "k", severity: GroupSeverity("oops"), firstSeenAt: first, lastSeenAt: last},
		{name: "negative event_count", groupKey: "k", severity: GroupSeverityWarning, eventCount: -1, firstSeenAt: first, lastSeenAt: last},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewAlertGroup(tc.groupKey, nil, tc.severity, tc.eventCount, tc.firstSeenAt, tc.lastSeenAt, nil)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestAlertGroup_Close(t *testing.T) {
	t.Parallel()

	first := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	last := first.Add(time.Minute)
	g, err := NewAlertGroup("k", nil, GroupSeverityWarning, 1, first, last, nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	closed := g.Close()
	if closed.Status != AlertGroupStatusClosed {
		t.Fatalf("status = %q, want %q", closed.Status, AlertGroupStatusClosed)
	}
	again := closed.Close()
	if again.Status != AlertGroupStatusClosed {
		t.Fatalf("re-close drifted status to %q", again.Status)
	}
}
