package alertgrouping

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

// makeEvent constructs a minimal AlertEvent struct literal. Using a
// struct literal (not domain.NewAlertEvent) so we can test invalid
// inputs (ID==0, StartsAt.IsZero()) with clear semantics.
func makeEvent(id int64, labels map[string]string, startsAt time.Time) domain.AlertEvent {
	return domain.AlertEvent{
		ID:       domain.AlertEventID(id),
		Labels:   labels,
		StartsAt: startsAt,
	}
}

// base time for tests.
var t0 = time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)

func TestGroupEvents_SameKeyAggregates(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "HighCPU", "severity": "warning"}, t0),
		makeEvent(2, map[string]string{"alertname": "HighCPU", "severity": "critical"}, t0.Add(1*time.Minute)),
		makeEvent(3, map[string]string{"alertname": "HighCPU", "severity": "info"}, t0.Add(2*time.Minute)),
	}

	groups, err := GroupEvents(events, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	g := groups[0]
	if g.Severity != domain.GroupSeverityCritical {
		t.Errorf("severity: got %q, want %q", g.Severity, domain.GroupSeverityCritical)
	}
	if g.EventCount != 3 {
		t.Errorf("event count: got %d, want 3", g.EventCount)
	}
	if !g.FirstSeenAt.Equal(t0) {
		t.Errorf("first seen: got %v, want %v", g.FirstSeenAt, t0)
	}
	if !g.LastSeenAt.Equal(t0.Add(2 * time.Minute)) {
		t.Errorf("last seen: got %v, want %v", g.LastSeenAt, t0.Add(2*time.Minute))
	}
	wantIDs := []domain.AlertEventID{1, 2, 3}
	if !reflect.DeepEqual(g.EventIDs, wantIDs) {
		t.Errorf("event IDs: got %v, want %v", g.EventIDs, wantIDs)
	}
}

func TestGroupEvents_DifferentKeySplits(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "HighCPU", "severity": "warning"}, t0),
		makeEvent(2, map[string]string{"alertname": "DiskFull", "severity": "critical"}, t0.Add(1*time.Minute)),
	}

	groups, err := GroupEvents(events, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// Verify each group has distinct alertname in dimensions.
	dims0 := parseDimensions(t, groups[0].Dimensions)
	dims1 := parseDimensions(t, groups[1].Dimensions)
	if dims0["alertname"] == dims1["alertname"] {
		t.Errorf("groups should have different alertnames, both have %q", dims0["alertname"])
	}
}

func TestGroupEvents_ShuffledInputSameOutput(t *testing.T) {
	base := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "A", "severity": "info"}, t0),
		makeEvent(2, map[string]string{"alertname": "B", "severity": "warning"}, t0.Add(1*time.Minute)),
		makeEvent(3, map[string]string{"alertname": "A", "severity": "critical"}, t0.Add(2*time.Minute)),
	}

	// Three orderings.
	orderings := [][]domain.AlertEvent{
		{base[0], base[1], base[2]},
		{base[2], base[0], base[1]},
		{base[1], base[2], base[0]},
	}

	var reference []domain.AlertGroup
	for i, input := range orderings {
		groups, err := GroupEvents(input, DefaultConfig())
		if err != nil {
			t.Fatalf("ordering %d: unexpected error: %v", i, err)
		}
		if i == 0 {
			reference = groups
		} else {
			if !reflect.DeepEqual(groups, reference) {
				t.Errorf("ordering %d produced different output:\ngot:  %+v\nwant: %+v", i, groups, reference)
			}
		}
	}
}

func TestGroupEvents_SeverityPromotion(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X", "severity": "info"}, t0),
		makeEvent(2, map[string]string{"alertname": "X", "severity": "warning"}, t0.Add(1*time.Minute)),
		makeEvent(3, map[string]string{"alertname": "X", "severity": "critical"}, t0.Add(2*time.Minute)),
	}

	groups, err := GroupEvents(events, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups[0].Severity != domain.GroupSeverityCritical {
		t.Errorf("severity: got %q, want %q", groups[0].Severity, domain.GroupSeverityCritical)
	}
}

func TestGroupEvents_MissingSeverityDefaultsToUnknown(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X"}, t0),
		makeEvent(2, map[string]string{"alertname": "X"}, t0.Add(1*time.Minute)),
	}

	groups, err := GroupEvents(events, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups[0].Severity != domain.GroupSeverityUnknown {
		t.Errorf("severity: got %q, want %q", groups[0].Severity, domain.GroupSeverityUnknown)
	}
}

func TestGroupEvents_UnrecognizedSeverityDefaultsToUnknown(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X", "severity": "high"}, t0),
		makeEvent(2, map[string]string{"alertname": "X", "severity": "WARN"}, t0.Add(1*time.Minute)),
	}

	groups, err := GroupEvents(events, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups[0].Severity != domain.GroupSeverityUnknown {
		t.Errorf("severity: got %q, want %q", groups[0].Severity, domain.GroupSeverityUnknown)
	}
}

func TestGroupEvents_SeverityTrimAndCaseNormalization(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X", "severity": " Critical "}, t0),
	}
	groups, err := GroupEvents(events, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups[0].Severity != domain.GroupSeverityCritical {
		t.Errorf("severity: got %q, want %q", groups[0].Severity, domain.GroupSeverityCritical)
	}

	events2 := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "Y", "severity": "WARNING"}, t0),
	}
	groups2, err := GroupEvents(events2, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups2[0].Severity != domain.GroupSeverityWarning {
		t.Errorf("severity: got %q, want %q", groups2[0].Severity, domain.GroupSeverityWarning)
	}

	events3 := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "Z", "severity": " info "}, t0),
	}
	groups3, err := GroupEvents(events3, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups3[0].Severity != domain.GroupSeverityInfo {
		t.Errorf("severity: got %q, want %q", groups3[0].Severity, domain.GroupSeverityInfo)
	}
}

func TestGroupEvents_MissingDimensionKeyPreservesEmptyString(t *testing.T) {
	cfg := Config{
		DimensionKeys: []string{"alertname", "service"},
		SeverityKey:   "severity",
	}
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X", "severity": "info"}, t0),
	}

	groups, err := GroupEvents(events, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dims := parseDimensions(t, groups[0].Dimensions)
	if dims["service"] != "" {
		t.Errorf("missing dimension key should be empty string, got %q", dims["service"])
	}
	if dims["alertname"] != "X" {
		t.Errorf("alertname: got %q, want %q", dims["alertname"], "X")
	}
}

func TestGroupEvents_OutputSortedByFirstSeenAtThenGroupKey(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "B", "severity": "info"}, t0.Add(2*time.Minute)),
		makeEvent(2, map[string]string{"alertname": "A", "severity": "info"}, t0.Add(2*time.Minute)),
		makeEvent(3, map[string]string{"alertname": "C", "severity": "info"}, t0),
	}

	groups, err := GroupEvents(events, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	// C has earliest FirstSeenAt (t0), A and B share t0+2m -> sorted by GroupKey.
	dims0 := parseDimensions(t, groups[0].Dimensions)
	if dims0["alertname"] != "C" {
		t.Errorf("first group should be C (earliest), got %q", dims0["alertname"])
	}
	// Groups 1 and 2 have same FirstSeenAt; verify GroupKey ordering.
	if groups[1].GroupKey >= groups[2].GroupKey {
		t.Errorf("groups with same FirstSeenAt should be sorted by GroupKey asc: %s >= %s",
			groups[1].GroupKey, groups[2].GroupKey)
	}
}

func TestGroupEvents_EventIDsSortedByStartsAtThenID(t *testing.T) {
	sameTime := t0
	events := []domain.AlertEvent{
		makeEvent(3, map[string]string{"alertname": "X", "severity": "info"}, sameTime),
		makeEvent(1, map[string]string{"alertname": "X", "severity": "info"}, sameTime),
		makeEvent(2, map[string]string{"alertname": "X", "severity": "info"}, sameTime.Add(-1*time.Minute)),
	}

	groups, err := GroupEvents(events, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Event 2 has earliest StartsAt; events 1 and 3 share sameTime -> sorted by ID.
	wantIDs := []domain.AlertEventID{2, 1, 3}
	if !reflect.DeepEqual(groups[0].EventIDs, wantIDs) {
		t.Errorf("event IDs: got %v, want %v", groups[0].EventIDs, wantIDs)
	}
}

func TestGroupEvents_EmptyInput(t *testing.T) {
	groups, err := GroupEvents(nil, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups != nil {
		t.Errorf("expected nil groups for nil input, got %v", groups)
	}

	groups2, err := GroupEvents([]domain.AlertEvent{}, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups2 != nil {
		t.Errorf("expected nil groups for empty input, got %v", groups2)
	}
}

func TestGroupEvents_EmptyInputWithInvalidConfigStillSucceeds(t *testing.T) {
	// Empty input should skip config validation entirely.
	badCfg := Config{DimensionKeys: nil, SeverityKey: ""}
	groups, err := GroupEvents(nil, badCfg)
	if err != nil {
		t.Fatalf("expected nil error for empty input with bad config, got: %v", err)
	}
	if groups != nil {
		t.Errorf("expected nil groups, got %v", groups)
	}
}

func TestGroupEvents_ZeroIDReturnsInvariantViolation(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(0, map[string]string{"alertname": "X"}, t0),
	}
	_, err := GroupEvents(events, DefaultConfig())
	if err == nil {
		t.Fatal("expected error for zero ID")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Errorf("error should wrap ErrInvariantViolation, got: %v", err)
	}
}

func TestGroupEvents_ZeroStartsAtReturnsInvariantViolation(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X"}, time.Time{}),
	}
	_, err := GroupEvents(events, DefaultConfig())
	if err == nil {
		t.Fatal("expected error for zero StartsAt")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Errorf("error should wrap ErrInvariantViolation, got: %v", err)
	}
}

func TestGroupEvents_EmptyDimensionKeysReturnsError(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X"}, t0),
	}
	cfg := Config{DimensionKeys: nil, SeverityKey: "severity"}
	_, err := GroupEvents(events, cfg)
	if err == nil {
		t.Fatal("expected error for nil DimensionKeys")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Errorf("error should wrap ErrInvariantViolation, got: %v", err)
	}

	cfg2 := Config{DimensionKeys: []string{}, SeverityKey: "severity"}
	_, err = GroupEvents(events, cfg2)
	if err == nil {
		t.Fatal("expected error for empty DimensionKeys")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Errorf("error should wrap ErrInvariantViolation, got: %v", err)
	}
}

func TestGroupEvents_BlankDimensionKeyReturnsError(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X"}, t0),
	}
	cfg := Config{DimensionKeys: []string{" ", "\t"}, SeverityKey: "severity"}
	_, err := GroupEvents(events, cfg)
	if err == nil {
		t.Fatal("expected error for blank DimensionKeys")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Errorf("error should wrap ErrInvariantViolation, got: %v", err)
	}
}

func TestGroupEvents_BlankSeverityKeyReturnsError(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X"}, t0),
	}
	cfg := Config{DimensionKeys: []string{"alertname"}, SeverityKey: " "}
	_, err := GroupEvents(events, cfg)
	if err == nil {
		t.Fatal("expected error for blank SeverityKey")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Errorf("error should wrap ErrInvariantViolation, got: %v", err)
	}
}

func TestGroupEvents_DuplicateDimensionKeysDeduped(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X", "severity": "info"}, t0),
	}
	cfg1 := Config{DimensionKeys: []string{"alertname", "alertname"}, SeverityKey: "severity"}
	cfg2 := Config{DimensionKeys: []string{"alertname"}, SeverityKey: "severity"}

	g1, err := GroupEvents(events, cfg1)
	if err != nil {
		t.Fatalf("cfg1: unexpected error: %v", err)
	}
	g2, err := GroupEvents(events, cfg2)
	if err != nil {
		t.Fatalf("cfg2: unexpected error: %v", err)
	}
	if g1[0].GroupKey != g2[0].GroupKey {
		t.Errorf("duplicate keys should produce same group key: %s vs %s", g1[0].GroupKey, g2[0].GroupKey)
	}
}

func TestGroupEvents_ConfigKeyOrderIrrelevant(t *testing.T) {
	events := []domain.AlertEvent{
		makeEvent(1, map[string]string{"alertname": "X", "service": "api", "severity": "info"}, t0),
	}
	cfgAB := Config{DimensionKeys: []string{"alertname", "service"}, SeverityKey: "severity"}
	cfgBA := Config{DimensionKeys: []string{"service", "alertname"}, SeverityKey: "severity"}

	g1, err := GroupEvents(events, cfgAB)
	if err != nil {
		t.Fatalf("cfgAB: unexpected error: %v", err)
	}
	g2, err := GroupEvents(events, cfgBA)
	if err != nil {
		t.Fatalf("cfgBA: unexpected error: %v", err)
	}
	if g1[0].GroupKey != g2[0].GroupKey {
		t.Errorf("key order should not affect group key: %s vs %s", g1[0].GroupKey, g2[0].GroupKey)
	}
	if string(g1[0].Dimensions) != string(g2[0].Dimensions) {
		t.Errorf("key order should not affect dimensions JSON: %s vs %s",
			string(g1[0].Dimensions), string(g2[0].Dimensions))
	}
}

// --- test helper ---

func parseDimensions(t *testing.T, raw json.RawMessage) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("failed to parse dimensions JSON: %v", err)
	}
	return m
}
