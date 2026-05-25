package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AlertStatus is the lifecycle status of an AlertEvent. Values are
// validated by AlertStatus.Valid; the underlying string mirrors the
// `alert_events.status` column so adding a new state does NOT require
// a database migration.
type AlertStatus string

// AlertStatus enumeration. New values may be appended without a
// schema migration; consumers must update the Valid switch below.
const (
	AlertStatusFiring   AlertStatus = "firing"
	AlertStatusResolved AlertStatus = "resolved"
)

// Valid reports whether s is a known AlertStatus value.
func (s AlertStatus) Valid() bool {
	switch s {
	case AlertStatusFiring, AlertStatusResolved:
		return true
	}
	return false
}

// AlertEvent is a single alert event ingested from an upstream
// metrics provider. The natural unique key at the persistence
// boundary is (Source, CanonicalFingerprint, StartsAt). Re-ingestion
// of an identical event collapses to the same row; a status
// transition to "resolved" is an UPDATE in place.
//
// All time fields are normalised to UTC microsecond resolution by
// NewAlertEvent / Resolve before being returned.
type AlertEvent struct {
	ID                   AlertEventID
	Source               string
	SourceFingerprint    string
	CanonicalFingerprint string
	Labels               map[string]string
	Annotations          map[string]string
	RawPayload           json.RawMessage
	Status               AlertStatus
	StartsAt             time.Time
	EndsAt               *time.Time
	CreatedAt            time.Time
}

// NewAlertEvent constructs an AlertEvent in the "firing" state. ID
// is zero (filled in on persist). Cross-field invariants enforced:
//
//   - Source / SourceFingerprint / CanonicalFingerprint must be
//     non-empty after trimming
//   - StartsAt must be non-zero
//   - Labels / Annotations may be nil; the constructor normalises
//     them to empty (non-nil) maps so JSON round-trips are stable
//
// Per-field length limits documented at the Ent schema level (e.g.
// MaxLen 64 / 128) are NOT re-checked here: that is double
// enforcement and the persistence call will reject violations.
// Returned errors wrap ErrInvariantViolation so callers can branch
// with errors.Is.
func NewAlertEvent(
	source, sourceFingerprint, canonicalFingerprint string,
	labels, annotations map[string]string,
	rawPayload json.RawMessage,
	startsAt time.Time,
) (AlertEvent, error) {
	source = strings.TrimSpace(source)
	sourceFingerprint = strings.TrimSpace(sourceFingerprint)
	canonicalFingerprint = strings.TrimSpace(canonicalFingerprint)
	if source == "" {
		return AlertEvent{}, fmt.Errorf("alert event: source must be non-empty: %w", ErrInvariantViolation)
	}
	if sourceFingerprint == "" {
		return AlertEvent{}, fmt.Errorf("alert event: source fingerprint must be non-empty: %w", ErrInvariantViolation)
	}
	if canonicalFingerprint == "" {
		return AlertEvent{}, fmt.Errorf("alert event: canonical fingerprint must be non-empty: %w", ErrInvariantViolation)
	}
	if startsAt.IsZero() {
		return AlertEvent{}, fmt.Errorf("alert event: starts_at must be set: %w", ErrInvariantViolation)
	}
	if labels == nil {
		labels = map[string]string{}
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	return AlertEvent{
		Source:               source,
		SourceFingerprint:    sourceFingerprint,
		CanonicalFingerprint: canonicalFingerprint,
		Labels:               labels,
		Annotations:          annotations,
		RawPayload:           rawPayload,
		Status:               AlertStatusFiring,
		StartsAt:             NormalizeUTCMicro(startsAt),
	}, nil
}

// Resolve transitions the event to "resolved" with EndsAt = endsAt
// (normalised to UTC microsecond). It returns ErrInvariantViolation
// if endsAt is zero or precedes StartsAt. Calling Resolve on an
// already-resolved event with the same endsAt is a no-op (returns
// the receiver unchanged); a different endsAt is an invariant
// violation because resolution timestamps are immutable once set.
func (a AlertEvent) Resolve(endsAt time.Time) (AlertEvent, error) {
	if endsAt.IsZero() {
		return AlertEvent{}, fmt.Errorf("alert event: ends_at must be set on resolve: %w", ErrInvariantViolation)
	}
	normalised := NormalizeUTCMicro(endsAt)
	if normalised.Before(a.StartsAt) {
		return AlertEvent{}, fmt.Errorf("alert event: ends_at %s precedes starts_at %s: %w", normalised, a.StartsAt, ErrInvariantViolation)
	}
	if a.Status == AlertStatusResolved && a.EndsAt != nil {
		if a.EndsAt.Equal(normalised) {
			return a, nil
		}
		return AlertEvent{}, fmt.Errorf("alert event: ends_at is immutable once set (%s -> %s): %w", *a.EndsAt, normalised, ErrInvariantViolation)
	}
	a.Status = AlertStatusResolved
	a.EndsAt = &normalised
	return a, nil
}

// AlertGroupStatus enumerates the lifecycle states of an AlertGroup.
// "active" means the group is still accreting events; "closed" means
// downstream EvidenceSnapshot has been produced and the group is
// sealed.
type AlertGroupStatus string

// AlertGroupStatus enumeration.
const (
	AlertGroupStatusActive AlertGroupStatus = "active"
	AlertGroupStatusClosed AlertGroupStatus = "closed"
)

// Valid reports whether s is a known AlertGroupStatus value.
func (s AlertGroupStatus) Valid() bool {
	switch s {
	case AlertGroupStatusActive, AlertGroupStatusClosed:
		return true
	}
	return false
}

// GroupSeverity is the maximum severity observed across the group's
// events. Producers MUST emit one of the listed values; "unknown" is
// the default for groups whose events do not carry a severity label.
type GroupSeverity string

// GroupSeverity enumeration.
const (
	GroupSeverityCritical GroupSeverity = "critical"
	GroupSeverityWarning  GroupSeverity = "warning"
	GroupSeverityInfo     GroupSeverity = "info"
	GroupSeverityUnknown  GroupSeverity = "unknown"
)

// Valid reports whether s is a known GroupSeverity value.
func (s GroupSeverity) Valid() bool {
	switch s {
	case GroupSeverityCritical, GroupSeverityWarning, GroupSeverityInfo, GroupSeverityUnknown:
		return true
	}
	return false
}

// AlertGroup is the deterministic grouping result produced by
// Stage S1. Identity at the persistence boundary is the natural
// unique key (GroupKey, FirstSeenAt). The same group recurring in a
// later window is a NEW row, not an update, because each instance
// fans out to an independent EvidenceSnapshot.
//
// EventIDs holds the AlertEvent identifiers covered by the group;
// the M2N mapping is materialised by the repository via the
// alert_event_groups join table. Producers may leave EventIDs nil
// when only writing the group header (events linked separately).
type AlertGroup struct {
	ID          AlertGroupID
	GroupKey    string
	Dimensions  json.RawMessage
	Severity    GroupSeverity
	EventCount  int
	Status      AlertGroupStatus
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	EventIDs    []AlertEventID
}

// NewAlertGroup constructs an active AlertGroup with normalised
// timestamps. Cross-field invariants enforced:
//
//   - GroupKey must be non-empty
//   - FirstSeenAt / LastSeenAt must be non-zero
//   - LastSeenAt must not precede FirstSeenAt
//   - Severity must be a known GroupSeverity (caller defaults to
//     GroupSeverityUnknown if no events carry severity)
//   - EventCount must be >= 0
//
// Dimensions may be nil; the constructor leaves it as-is. Caller is
// expected to supply a canonicalised JSON document derived from the
// grouping configuration. EventIDs may be nil and is populated
// separately by the repository.
func NewAlertGroup(
	groupKey string,
	dimensions json.RawMessage,
	severity GroupSeverity,
	eventCount int,
	firstSeenAt, lastSeenAt time.Time,
	eventIDs []AlertEventID,
) (AlertGroup, error) {
	if groupKey == "" {
		return AlertGroup{}, fmt.Errorf("alert group: group_key must be non-empty: %w", ErrInvariantViolation)
	}
	if firstSeenAt.IsZero() {
		return AlertGroup{}, fmt.Errorf("alert group: first_seen_at must be set: %w", ErrInvariantViolation)
	}
	if lastSeenAt.IsZero() {
		return AlertGroup{}, fmt.Errorf("alert group: last_seen_at must be set: %w", ErrInvariantViolation)
	}
	first := NormalizeUTCMicro(firstSeenAt)
	last := NormalizeUTCMicro(lastSeenAt)
	if last.Before(first) {
		return AlertGroup{}, fmt.Errorf("alert group: last_seen_at %s precedes first_seen_at %s: %w", last, first, ErrInvariantViolation)
	}
	if !severity.Valid() {
		return AlertGroup{}, fmt.Errorf("alert group: severity %q is not a known value: %w", severity, ErrInvariantViolation)
	}
	if eventCount < 0 {
		return AlertGroup{}, fmt.Errorf("alert group: event_count %d must be >= 0: %w", eventCount, ErrInvariantViolation)
	}
	return AlertGroup{
		GroupKey:    groupKey,
		Dimensions:  dimensions,
		Severity:    severity,
		EventCount:  eventCount,
		Status:      AlertGroupStatusActive,
		FirstSeenAt: first,
		LastSeenAt:  last,
		EventIDs:    eventIDs,
	}, nil
}

// Close marks the group sealed (status -> closed). Closing an
// already-closed group is a no-op.
func (g AlertGroup) Close() AlertGroup {
	if g.Status == AlertGroupStatusClosed {
		return g
	}
	g.Status = AlertGroupStatusClosed
	return g
}
