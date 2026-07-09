package alertreplay_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertevent"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertgroup"
	"github.com/openclarion/openclarion/internal/persistence/ent/evidencesnapshot"
	cmdbfake "github.com/openclarion/openclarion/internal/providers/cmdb/fake"
	"github.com/openclarion/openclarion/internal/providers/metrics/fake"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertingest"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// seedTime is the deterministic anchor used by every test in this
// file. Pinning it keeps assertion failures (`want X, got Y`) stable
// across test runs and across CI hosts in different time zones.
var seedTime = time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

// seedAlerts builds a deterministic batch of ActiveAlerts for a
// single alertname. Each instance gets a unique label set so the
// (source, canonical_fingerprint, starts_at) natural key is unique
// per alert. starts_at = base + offset + i*interval so callers can
// place batches relative to a window.
func seedAlerts(alertname string, instances int, base time.Time, offset, interval time.Duration, severity string) []ports.ActiveAlert {
	out := make([]ports.ActiveAlert, instances)
	for i := 0; i < instances; i++ {
		out[i] = ports.ActiveAlert{
			Source: "prometheus",
			Labels: map[string]string{
				"alertname": alertname,
				"instance":  alertname + "-" + strconv.Itoa(i),
				"severity":  severity,
			},
			Annotations: map[string]string{"summary": alertname + " firing"},
			StartsAt:    base.Add(offset + time.Duration(i)*interval),
			RawPayload:  json.RawMessage(`{"raw":1}`),
		}
	}
	return out
}

// defaultRequest builds a Request with the package's default
// grouping config (alertname / severity) and a generous limit so
// the safety valve is out of the picture for the "happy path"
// tests. Tests that exercise the safety valve construct their own
// Request literal.
func defaultRequest(start, end time.Time) alertreplay.Request {
	return alertreplay.Request{
		WindowStart:       start,
		WindowEnd:         end,
		Grouping:          alertgrouping.DefaultConfig(),
		CreatedByWorkflow: "test-workflow",
		Limit:             10000,
	}
}

// countAlertEvents reads the alert_event row count via the Ent
// client directly so the assertion targets ground truth rather than
// the production code path under test.
func countAlertEvents(ctx context.Context, t *testing.T) int {
	t.Helper()
	n, err := integration.client.AlertEvent.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count alert_event: %v", err)
	}
	return n
}

func countAlertGroups(ctx context.Context, t *testing.T) int {
	t.Helper()
	n, err := integration.client.AlertGroup.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count alert_group: %v", err)
	}
	return n
}

func countEvidenceSnapshots(ctx context.Context, t *testing.T) int {
	t.Helper()
	n, err := integration.client.EvidenceSnapshot.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count evidence_snapshot: %v", err)
	}
	return n
}

// countEventGroupLinks counts the rows in the alert_event_groups
// M2N join table by summing each group's eager-loaded events edge.
// We do not have a generated Count() helper for the join table, so
// the proxy is exact rather than approximate.
func countEventGroupLinks(ctx context.Context, t *testing.T) int {
	t.Helper()
	groups, err := integration.client.AlertGroup.Query().WithEvents().All(ctx)
	if err != nil {
		t.Fatalf("list alert_group with events: %v", err)
	}
	total := 0
	for _, g := range groups {
		total += len(g.Edges.Events)
	}
	return total
}

// countClosedGroups isolates the "all groups must be closed by the
// time replay returns" assertion that several tests share.
func countClosedGroups(ctx context.Context, t *testing.T) int {
	t.Helper()
	n, err := integration.client.AlertGroup.Query().Where(alertgroup.StatusEQ(string(domain.AlertGroupStatusClosed))).Count(ctx)
	if err != nil {
		t.Fatalf("count closed alert_group: %v", err)
	}
	return n
}

// TestReplayWindow_TwentyAlertsHappyPath: 4 alertnames * 5 instances
// over a 1h window. Asserts every Stats counter so a future change
// that silently flips one branch (saved -> existing, etc.) fails
// here rather than slipping past as "the test still passes".
func TestReplayWindow_TwentyAlertsHappyPath(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	var batch []ports.ActiveAlert
	// Spread instances across the hour so StartsAt is unique per
	// alert (the natural key includes starts_at). Severity is
	// fixed per alertname so GroupSeverity is deterministic.
	for ai, name := range []string{"AlertA", "AlertB", "AlertC", "AlertD"} {
		offset := time.Duration(ai) * 10 * time.Minute
		batch = append(batch, seedAlerts(name, 5, windowStart, offset, time.Minute, "warning")...)
	}

	provider := fake.New(batch)
	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("ReplayWindow: %v", err)
	}

	want := alertreplay.Stats{
		Ingested:           alertingest.Stats{Total: 20, Saved: 20},
		EventsLoaded:       20,
		GroupsBuilt:        4,
		GroupsSaved:        4,
		GroupsRefreshed:    0,
		GroupsExisting:     0,
		SnapshotsSaved:     4,
		SnapshotsDuplicate: 0,
		GroupsClosed:       4,
		Failed:             0,
	}
	if stats != want {
		t.Errorf("stats = %+v\nwant   %+v", stats, want)
	}

	if got := countAlertEvents(ctx, t); got != 20 {
		t.Errorf("alert_event count = %d, want 20", got)
	}
	if got := countAlertGroups(ctx, t); got != 4 {
		t.Errorf("alert_group count = %d, want 4", got)
	}
	if got := countClosedGroups(ctx, t); got != 4 {
		t.Errorf("closed alert_group count = %d, want 4", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 4 {
		t.Errorf("evidence_snapshot count = %d, want 4", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 20 {
		t.Errorf("alert_event_groups (M2N) count = %d, want 20", got)
	}
}

// TestReplayWindow_RerunIsIdempotent: replay over the same provider
// state must not produce side effects on the second run. The
// expected Stats reshape (Saved -> Duplicate / Existing) is the
// regression guard for the refresh-vs-existing diff.
func TestReplayWindow_RerunIsIdempotent(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	var batch []ports.ActiveAlert
	for ai, name := range []string{"AlertA", "AlertB", "AlertC", "AlertD"} {
		offset := time.Duration(ai) * 10 * time.Minute
		batch = append(batch, seedAlerts(name, 5, windowStart, offset, time.Minute, "warning")...)
	}
	provider := fake.New(batch)

	if _, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, defaultRequest(windowStart, windowEnd)); err != nil {
		t.Fatalf("first ReplayWindow: %v", err)
	}

	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("second ReplayWindow: %v", err)
	}

	want := alertreplay.Stats{
		Ingested:           alertingest.Stats{Total: 20, Duplicate: 20},
		EventsLoaded:       20,
		GroupsBuilt:        4,
		GroupsSaved:        0,
		GroupsRefreshed:    0,
		GroupsExisting:     4,
		SnapshotsSaved:     0,
		SnapshotsDuplicate: 4,
		GroupsClosed:       0, // already closed after first run
		Failed:             0,
	}
	if stats != want {
		t.Errorf("second stats = %+v\nwant         %+v", stats, want)
	}

	if got := countAlertEvents(ctx, t); got != 20 {
		t.Errorf("alert_event count after rerun = %d, want 20", got)
	}
	if got := countAlertGroups(ctx, t); got != 4 {
		t.Errorf("alert_group count after rerun = %d, want 4", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 4 {
		t.Errorf("evidence_snapshot count after rerun = %d, want 4", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 20 {
		t.Errorf("alert_event_groups count after rerun = %d, want 20", got)
	}
}

// TestReplayWindowForReport_ReturnsSnapshotRefsForSavedAndDuplicateSnapshots
// proves the report-trigger API exposes the persisted snapshot IDs
// for both first-run saves and idempotent duplicate reruns. ReplayWindow
// remains the counter-only compatibility API.
func TestReplayWindowForReport_ReturnsSnapshotRefsForSavedAndDuplicateSnapshots(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	var batch []ports.ActiveAlert
	batch = append(batch, seedAlerts("AlertA", 2, windowStart, 0, time.Minute, "warning")...)
	batch = append(batch, seedAlerts("AlertB", 1, windowStart, 30*time.Minute, time.Minute, "critical")...)
	provider := fake.New(batch)
	req := defaultRequest(windowStart, windowEnd)

	first, err := alertreplay.ReplayWindowForReport(ctx, provider, integration.factory, req)
	if err != nil {
		t.Fatalf("first ReplayWindowForReport: %v", err)
	}
	if first.Stats.SnapshotsSaved != 2 || first.Stats.SnapshotsDuplicate != 0 {
		t.Fatalf("first snapshot stats = %+v, want saved=2 duplicate=0", first.Stats)
	}
	if len(first.Snapshots) != 2 {
		t.Fatalf("first snapshots len = %d, want 2", len(first.Snapshots))
	}
	firstIDs := make([]domain.EvidenceSnapshotID, len(first.Snapshots))
	for i, ref := range first.Snapshots {
		if ref.ID == 0 {
			t.Fatalf("first snapshots[%d].ID is zero", i)
		}
		if ref.GroupIndex != i {
			t.Fatalf("first snapshots[%d].GroupIndex = %d, want %d", i, ref.GroupIndex, i)
		}
		if ref.EventCount <= 0 {
			t.Fatalf("first snapshots[%d].EventCount = %d, want > 0", i, ref.EventCount)
		}
		firstIDs[i] = ref.ID
	}

	second, err := alertreplay.ReplayWindowForReport(ctx, provider, integration.factory, req)
	if err != nil {
		t.Fatalf("second ReplayWindowForReport: %v", err)
	}
	if second.Stats.SnapshotsSaved != 0 || second.Stats.SnapshotsDuplicate != 2 {
		t.Fatalf("second snapshot stats = %+v, want saved=0 duplicate=2", second.Stats)
	}
	if len(second.Snapshots) != len(firstIDs) {
		t.Fatalf("second snapshots len = %d, want %d", len(second.Snapshots), len(firstIDs))
	}
	for i, ref := range second.Snapshots {
		if ref.ID != firstIDs[i] {
			t.Fatalf("second snapshots[%d].ID = %d, want existing ID %d", i, ref.ID, firstIDs[i])
		}
		if ref.GroupIndex != i {
			t.Fatalf("second snapshots[%d].GroupIndex = %d, want %d", i, ref.GroupIndex, i)
		}
	}
}

func TestReplayPersistedWindowForReport_BuildsSnapshotsWithoutProviderIngest(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("AlertA", 2, windowStart, 0, time.Minute, "warning")
	ingested, err := alertingest.IngestAlerts(ctx, batch, integration.factory)
	if err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	if ingested != (alertingest.Stats{Total: 2, Saved: 2}) {
		t.Fatalf("ingested = %+v, want total=2 saved=2", ingested)
	}

	result, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("ReplayPersistedWindowForReport: %v", err)
	}

	if result.Stats.Ingested != (alertingest.Stats{}) {
		t.Fatalf("persisted replay Ingested = %+v, want zero", result.Stats.Ingested)
	}
	if result.Stats.EventsLoaded != 2 || result.Stats.GroupsBuilt != 1 || result.Stats.SnapshotsSaved != 1 {
		t.Fatalf("persisted replay stats = %+v, want loaded=2 groups=1 snapshots=1", result.Stats)
	}
	if len(result.Snapshots) != 1 {
		t.Fatalf("snapshots len = %d, want 1", len(result.Snapshots))
	}
	if result.Snapshots[0].ID == 0 || result.Snapshots[0].GroupIndex != 0 || result.Snapshots[0].EventCount != 2 {
		t.Fatalf("snapshot ref = %+v, want id>0 group_index=0 event_count=2", result.Snapshots[0])
	}
	if got := countAlertEvents(ctx, t); got != 2 {
		t.Fatalf("alert_event count = %d, want 2", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 1 {
		t.Fatalf("evidence_snapshot count = %d, want 1", got)
	}
}

func TestReplayWindow_IncludesCMDBEnrichmentInSnapshot(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("AlertA", 1, windowStart, 0, time.Minute, "warning")
	provider := fake.New(batch)
	cmdbProvider := cmdbfake.New(ports.CMDBLookupResult{
		Found: true,
		Resource: ports.CMDBResource{
			ID:   "service/checkout",
			Kind: "service",
			Name: "Checkout",
			Owners: []ports.CMDBOwner{
				{Subject: "team-checkout", Team: "Checkout", Role: "primary"},
			},
			Topology: []ports.CMDBTopologyLink{
				{Relation: "depends_on", TargetID: "database/postgres", TargetKind: "database", TargetName: "PostgreSQL"},
			},
			Attributes: map[string]string{"tier": "checkout"},
		},
	})
	req := defaultRequest(windowStart, windowEnd)
	req.CMDBProvider = cmdbProvider

	result, err := alertreplay.ReplayWindowForReport(ctx, provider, integration.factory, req)
	if err != nil {
		t.Fatalf("ReplayWindowForReport: %v", err)
	}
	if result.Stats.GroupsBuilt != 1 || result.Stats.SnapshotsSaved != 1 || result.Stats.Failed != 0 {
		t.Fatalf("stats = %+v, want one successful enriched snapshot", result.Stats)
	}

	requests := cmdbProvider.Requests()
	if len(requests) != 1 {
		t.Fatalf("cmdb requests len = %d, want 1", len(requests))
	}
	if requests[0].Labels["alertname"] != "AlertA" || requests[0].Labels["instance"] != "AlertA-0" {
		t.Fatalf("cmdb lookup labels = %+v", requests[0].Labels)
	}

	snapshot, err := integration.client.EvidenceSnapshot.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load evidence snapshot: %v", err)
	}
	var payload struct {
		CMDB *struct {
			Matches []struct {
				EventID  int64 `json:"event_id"`
				Resource struct {
					ID     string `json:"id"`
					Kind   string `json:"kind"`
					Name   string `json:"name"`
					Owners []struct {
						Subject string `json:"subject"`
					} `json:"owners"`
					Topology []struct {
						TargetID string `json:"target_id"`
					} `json:"topology"`
					Attributes map[string]string `json:"attributes"`
				} `json:"resource"`
			} `json:"matches"`
		} `json:"cmdb"`
	}
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		t.Fatalf("unmarshal snapshot payload: %v", err)
	}
	if payload.CMDB == nil || len(payload.CMDB.Matches) != 1 {
		t.Fatalf("payload cmdb = %+v, want one match", payload.CMDB)
	}
	match := payload.CMDB.Matches[0]
	if match.EventID == 0 ||
		match.Resource.ID != "service/checkout" ||
		match.Resource.Kind != "service" ||
		match.Resource.Name != "Checkout" ||
		match.Resource.Owners[0].Subject != "team-checkout" ||
		match.Resource.Topology[0].TargetID != "database/postgres" ||
		match.Resource.Attributes["tier"] != "checkout" {
		t.Fatalf("cmdb match = %+v", match)
	}
}

func TestReplayWindow_CMDBErrorFailsGroupWithoutSnapshot(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("AlertA", 1, windowStart, 0, time.Minute, "warning")
	provider := fake.New(batch)
	req := defaultRequest(windowStart, windowEnd)
	req.CMDBProvider = cmdbfake.NewError(errors.New("cmdb unavailable"))

	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, req)
	if err == nil {
		t.Fatal("ReplayWindow err = nil, want cmdb error")
	}
	if !strings.Contains(err.Error(), "cmdb lookup for event") {
		t.Fatalf("ReplayWindow err = %v, want cmdb lookup context", err)
	}
	if stats.EventsLoaded != 1 || stats.GroupsBuilt != 1 || stats.Failed != 1 || stats.SnapshotsSaved != 0 {
		t.Fatalf("stats = %+v, want one failed group and no snapshots", stats)
	}
	if got := countAlertEvents(ctx, t); got != 1 {
		t.Fatalf("alert_event count = %d, want ingested event retained", got)
	}
	if got := countAlertGroups(ctx, t); got != 0 {
		t.Fatalf("alert_group count = %d, want no group transaction after cmdb failure", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 0 {
		t.Fatalf("evidence_snapshot count = %d, want 0", got)
	}
}

// TestReplayWindow_OutOfWindowEventsExcluded: IngestOnce persists
// every alert the provider returns, including those before the
// replay window. ReplayWindow's Step 2 must filter them out so
// grouping / snapshotting only sees in-window events. The DB
// AlertEvent count == 30 is the proof that Step 1 did not also
// filter (which would silently break the "ingest writes raw
// firing state" contract).
func TestReplayWindow_OutOfWindowEventsExcluded(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	// 20 in-window: 4 alertnames * 5 instances, spread inside the hour.
	var batch []ports.ActiveAlert
	for ai, name := range []string{"AlertA", "AlertB", "AlertC", "AlertD"} {
		offset := time.Duration(ai) * 10 * time.Minute
		batch = append(batch, seedAlerts(name, 5, windowStart, offset, time.Minute, "warning")...)
	}
	// 10 before-window: shifted to seedTime - 1h so StartsAt < windowStart.
	beforeBase := seedTime.Add(-time.Hour)
	batch = append(batch, seedAlerts("AlertHistorical", 10, beforeBase, 0, time.Minute, "info")...)

	provider := fake.New(batch)
	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("ReplayWindow: %v", err)
	}

	if stats.Ingested.Total != 30 || stats.Ingested.Saved != 30 {
		t.Errorf("Ingested = %+v, want Total=30 Saved=30 (IngestOnce must NOT filter by window)", stats.Ingested)
	}
	if stats.EventsLoaded != 20 {
		t.Errorf("EventsLoaded = %d, want 20 (Step 2 must filter)", stats.EventsLoaded)
	}
	if stats.GroupsBuilt != 4 || stats.GroupsSaved != 4 || stats.SnapshotsSaved != 4 || stats.GroupsClosed != 4 {
		t.Errorf("group/snapshot stats = %+v, want GroupsBuilt=GroupsSaved=SnapshotsSaved=GroupsClosed=4", stats)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0", stats.Failed)
	}

	if got := countAlertEvents(ctx, t); got != 30 {
		t.Errorf("alert_event count = %d, want 30 (every provider alert persists)", got)
	}
	if got := countAlertGroups(ctx, t); got != 4 {
		t.Errorf("alert_group count = %d, want 4 (only in-window groups)", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 4 {
		t.Errorf("evidence_snapshot count = %d, want 4", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 20 {
		t.Errorf("alert_event_groups count = %d, want 20 (only in-window events linked)", got)
	}
}

func TestReplayPersistedWindowForReport_AlertEventIDFilterAppliedBeforeLimit(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("Selected", 1, windowStart, 0, time.Minute, "warning")
	batch = append(batch, seedAlerts("Other", 4, windowStart, time.Minute, time.Minute, "critical")...)
	if _, err := alertingest.IngestAlerts(ctx, batch, integration.factory); err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	events, err := integration.client.AlertEvent.Query().
		Order(alertevent.ByID()).
		All(ctx)
	if err != nil {
		t.Fatalf("list alert events: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("events = %d, want 5", len(events))
	}

	req := defaultRequest(windowStart, windowEnd)
	req.AlertEventIDFilter = []domain.AlertEventID{domain.AlertEventID(events[0].ID)}
	req.Limit = 1
	result, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, req)
	if err != nil {
		t.Fatalf("ReplayPersistedWindowForReport: %v", err)
	}
	if result.Stats.EventsLoaded != 1 ||
		result.Stats.GroupsBuilt != 1 ||
		result.Stats.SnapshotsSaved != 1 ||
		len(result.Snapshots) != 1 ||
		result.Snapshots[0].EventCount != 1 {
		t.Fatalf("result = %+v, want one selected alert replayed before limit enforcement", result)
	}
	if got := countEventGroupLinks(ctx, t); got != 1 {
		t.Fatalf("alert_event_groups count = %d, want only selected event linked", got)
	}
}

func TestReplayPersistedWindowForReport_IDFilterDoesNotShrinkExistingGroup(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("Selected", 2, windowStart, 0, time.Minute, "warning")
	if _, err := alertingest.IngestAlerts(ctx, batch, integration.factory); err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	first, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("first ReplayPersistedWindowForReport: %v", err)
	}
	if first.Stats.GroupsSaved != 1 || first.Stats.SnapshotsSaved != 1 || len(first.Snapshots) != 1 || first.Snapshots[0].EventCount != 2 {
		t.Fatalf("first result = %+v, want one two-event snapshot", first)
	}
	events, err := integration.client.AlertEvent.Query().
		Order(alertevent.ByID()).
		All(ctx)
	if err != nil {
		t.Fatalf("list alert events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	req := defaultRequest(windowStart, windowEnd)
	req.AlertEventIDFilter = []domain.AlertEventID{domain.AlertEventID(events[0].ID)}
	second, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, req)
	if err != nil {
		t.Fatalf("second ReplayPersistedWindowForReport: %v", err)
	}
	if second.Stats.EventsLoaded != 1 ||
		second.Stats.GroupsBuilt != 1 ||
		second.Stats.GroupsExisting != 1 ||
		second.Stats.SnapshotsDuplicate != 1 ||
		len(second.Snapshots) != 1 ||
		second.Snapshots[0].EventCount != 2 {
		t.Fatalf("second result = %+v, want duplicate snapshot over existing two-event group", second)
	}
	groups, err := integration.client.AlertGroup.Query().All(ctx)
	if err != nil {
		t.Fatalf("list alert groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("alert groups = %d, want 1", len(groups))
	}
	wantLastSeen := domain.NormalizeUTCMicro(batch[1].StartsAt)
	if groups[0].EventCount != 2 || !groups[0].LastSeenAt.Equal(wantLastSeen) {
		t.Fatalf("group = event_count:%d last_seen:%s, want event_count=2 last_seen=%s", groups[0].EventCount, groups[0].LastSeenAt, wantLastSeen)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 1 {
		t.Fatalf("evidence_snapshot count = %d, want duplicate replay to reuse existing snapshot", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 2 {
		t.Fatalf("alert_event_groups count = %d, want both original events still linked", got)
	}
}

func TestReplayPersistedWindowForReport_CMDBLookupCoversExpandedExistingGroup(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("Selected", 2, windowStart, 0, time.Minute, "warning")
	if _, err := alertingest.IngestAlerts(ctx, batch, integration.factory); err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	first, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("first ReplayPersistedWindowForReport: %v", err)
	}
	if first.Stats.SnapshotsSaved != 1 || len(first.Snapshots) != 1 || first.Snapshots[0].EventCount != 2 {
		t.Fatalf("first result = %+v, want one two-event snapshot", first)
	}

	events, err := integration.client.AlertEvent.Query().
		Order(alertevent.ByID()).
		All(ctx)
	if err != nil {
		t.Fatalf("list alert events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	cmdbProvider := cmdbfake.New(ports.CMDBLookupResult{
		Found: true,
		Resource: ports.CMDBResource{
			ID:         "service/selected",
			Kind:       "service",
			Name:       "Selected",
			Attributes: map[string]string{"tier": "critical"},
		},
	})
	req := defaultRequest(windowStart, windowEnd)
	req.AlertEventIDFilter = []domain.AlertEventID{domain.AlertEventID(events[0].ID)}
	req.CMDBProvider = cmdbProvider
	second, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, req)
	if err != nil {
		t.Fatalf("second ReplayPersistedWindowForReport: %v", err)
	}
	if second.Stats.EventsLoaded != 1 ||
		second.Stats.GroupsBuilt != 1 ||
		second.Stats.GroupsExisting != 1 ||
		second.Stats.SnapshotsSaved != 1 ||
		len(second.Snapshots) != 1 ||
		second.Snapshots[0].EventCount != 2 {
		t.Fatalf("second result = %+v, want enriched snapshot over expanded two-event group", second)
	}

	requests := cmdbProvider.Requests()
	if len(requests) != 2 {
		t.Fatalf("cmdb requests = %d, want one for each event in expanded group", len(requests))
	}
	if requests[0].Labels["instance"] != "Selected-0" || requests[1].Labels["instance"] != "Selected-1" {
		t.Fatalf("cmdb request labels = %+v, want both existing group events", requests)
	}

	snapshot, err := integration.client.EvidenceSnapshot.Get(ctx, int(second.Snapshots[0].ID))
	if err != nil {
		t.Fatalf("get enriched evidence snapshot: %v", err)
	}
	var payload struct {
		CMDB struct {
			Matches []struct {
				EventID int64 `json:"event_id"`
			} `json:"matches"`
		} `json:"cmdb"`
	}
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		t.Fatalf("unmarshal enriched snapshot payload: %v", err)
	}
	if len(payload.CMDB.Matches) != 2 ||
		payload.CMDB.Matches[0].EventID != int64(events[0].ID) ||
		payload.CMDB.Matches[1].EventID != int64(events[1].ID) {
		t.Fatalf("cmdb matches = %+v, want both expanded event IDs", payload.CMDB.Matches)
	}
}

func TestReplayPersistedWindowForReport_IDFilterLaterEventReusesExistingGroup(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("Selected", 2, windowStart, 0, time.Minute, "warning")
	if _, err := alertingest.IngestAlerts(ctx, batch, integration.factory); err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	first, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("first ReplayPersistedWindowForReport: %v", err)
	}
	if first.Stats.GroupsSaved != 1 || first.Stats.SnapshotsSaved != 1 || len(first.Snapshots) != 1 || first.Snapshots[0].EventCount != 2 {
		t.Fatalf("first result = %+v, want one two-event snapshot", first)
	}

	events, err := integration.client.AlertEvent.Query().
		Order(alertevent.ByID()).
		All(ctx)
	if err != nil {
		t.Fatalf("list alert events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	req := defaultRequest(windowStart, windowEnd)
	req.AlertEventIDFilter = []domain.AlertEventID{domain.AlertEventID(events[1].ID)}
	second, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, req)
	if err != nil {
		t.Fatalf("second ReplayPersistedWindowForReport: %v", err)
	}
	if second.Stats.EventsLoaded != 1 ||
		second.Stats.GroupsBuilt != 1 ||
		second.Stats.GroupsExisting != 1 ||
		second.Stats.SnapshotsDuplicate != 1 ||
		len(second.Snapshots) != 1 ||
		second.Snapshots[0].EventCount != 2 {
		t.Fatalf("second result = %+v, want duplicate snapshot over linked existing group", second)
	}
	if got := countAlertGroups(ctx, t); got != 1 {
		t.Fatalf("alert_group count = %d, want 1", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 1 {
		t.Fatalf("evidence_snapshot count = %d, want duplicate replay to reuse existing snapshot", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 2 {
		t.Fatalf("alert_event_groups count = %d, want both original events still linked", got)
	}
}

func TestReplayPersistedWindowForReport_NonIDScopedLaterWindowCreatesNewGroup(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("Selected", 2, windowStart, 0, time.Minute, "warning")
	if _, err := alertingest.IngestAlerts(ctx, batch, integration.factory); err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	first, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("first ReplayPersistedWindowForReport: %v", err)
	}
	if first.Stats.GroupsSaved != 1 || first.Stats.SnapshotsSaved != 1 || len(first.Snapshots) != 1 || first.Snapshots[0].EventCount != 2 {
		t.Fatalf("first result = %+v, want one two-event snapshot", first)
	}

	narrowStart := domain.NormalizeUTCMicro(batch[1].StartsAt)
	req := defaultRequest(narrowStart, narrowStart.Add(time.Minute))
	second, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, req)
	if err != nil {
		t.Fatalf("second ReplayPersistedWindowForReport: %v", err)
	}
	if second.Stats.EventsLoaded != 1 ||
		second.Stats.GroupsBuilt != 1 ||
		second.Stats.GroupsSaved != 1 ||
		second.Stats.SnapshotsSaved != 1 ||
		len(second.Snapshots) != 1 ||
		second.Snapshots[0].EventCount != 1 {
		t.Fatalf("second result = %+v, want new one-event group for non-ID-scoped replay", second)
	}

	groups, err := integration.client.AlertGroup.Query().
		Order(alertgroup.ByFirstSeenAt()).
		All(ctx)
	if err != nil {
		t.Fatalf("list alert groups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("alert groups = %d, want 2", len(groups))
	}
	firstSeen := domain.NormalizeUTCMicro(batch[0].StartsAt)
	if groups[0].EventCount != 2 ||
		!groups[0].FirstSeenAt.Equal(firstSeen) ||
		!groups[0].LastSeenAt.Equal(narrowStart) {
		t.Fatalf("original group = event_count:%d first_seen:%s last_seen:%s, want unchanged two-event group",
			groups[0].EventCount, groups[0].FirstSeenAt, groups[0].LastSeenAt)
	}
	if groups[1].EventCount != 1 ||
		!groups[1].FirstSeenAt.Equal(narrowStart) ||
		!groups[1].LastSeenAt.Equal(narrowStart) {
		t.Fatalf("narrow group = event_count:%d first_seen:%s last_seen:%s, want one-event narrow-window group",
			groups[1].EventCount, groups[1].FirstSeenAt, groups[1].LastSeenAt)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 2 {
		t.Fatalf("evidence_snapshot count = %d, want one snapshot per group", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 3 {
		t.Fatalf("alert_event_groups count = %d, want later event linked to both groups", got)
	}
}

func TestReplayPersistedWindowForReport_ExistingGroupExpansionHonorsLimit(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	batch := seedAlerts("Selected", 2, windowStart, 0, time.Minute, "warning")
	if _, err := alertingest.IngestAlerts(ctx, batch, integration.factory); err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	first, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("first ReplayPersistedWindowForReport: %v", err)
	}
	if first.Stats.SnapshotsSaved != 1 || len(first.Snapshots) != 1 || first.Snapshots[0].EventCount != 2 {
		t.Fatalf("first result = %+v, want one two-event snapshot", first)
	}

	events, err := integration.client.AlertEvent.Query().
		Order(alertevent.ByID()).
		All(ctx)
	if err != nil {
		t.Fatalf("list alert events: %v", err)
	}
	req := defaultRequest(windowStart, windowEnd)
	req.AlertEventIDFilter = []domain.AlertEventID{domain.AlertEventID(events[0].ID)}
	req.Limit = 1
	second, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, req)
	if err == nil {
		t.Fatal("second ReplayPersistedWindowForReport: want limit error, got nil")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) ||
		!strings.Contains(err.Error(), "existing alert group") ||
		!strings.Contains(err.Error(), "more than limit (1)") {
		t.Fatalf("second error = %v, want existing group expansion limit violation", err)
	}
	if second.Stats.EventsLoaded != 1 ||
		second.Stats.GroupsBuilt != 1 ||
		second.Stats.Failed != 1 ||
		len(second.Snapshots) != 0 {
		t.Fatalf("second result = %+v, want per-group failure before snapshot", second)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 1 {
		t.Fatalf("evidence_snapshot count = %d, want only original snapshot", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 2 {
		t.Fatalf("alert_event_groups count = %d, want original links unchanged", got)
	}
}

func TestReplayPersistedWindowForReport_ExistingGroupExpansionHonorsReplayScope(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	prometheusAlert := seedAlerts("Scoped", 1, windowStart, 0, time.Minute, "warning")[0]
	alertmanagerAlert := prometheusAlert
	alertmanagerAlert.Source = "alertmanager"
	alertmanagerAlert.Labels = map[string]string{
		"alertname": "Scoped",
		"instance":  "Scoped-0",
		"severity":  "critical",
	}
	alertmanagerAlert.StartsAt = windowStart.Add(time.Minute)
	if _, err := alertingest.IngestAlerts(ctx, []ports.ActiveAlert{prometheusAlert, alertmanagerAlert}, integration.factory); err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	first, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("first ReplayPersistedWindowForReport: %v", err)
	}
	if first.Stats.GroupsSaved != 1 || first.Stats.SnapshotsSaved != 1 || len(first.Snapshots) != 1 || first.Snapshots[0].EventCount != 2 {
		t.Fatalf("first result = %+v, want one two-source snapshot", first)
	}

	req := defaultRequest(windowStart, windowEnd)
	req.SourceFilter = []string{"prometheus"}
	second, err := alertreplay.ReplayPersistedWindowForReport(ctx, integration.factory, req)
	if err != nil {
		t.Fatalf("second ReplayPersistedWindowForReport: %v", err)
	}
	if second.Stats.EventsLoaded != 1 ||
		second.Stats.GroupsBuilt != 1 ||
		second.Stats.GroupsRefreshed != 1 ||
		second.Stats.SnapshotsSaved != 1 ||
		len(second.Snapshots) != 1 ||
		second.Snapshots[0].EventCount != 1 {
		t.Fatalf("second result = %+v, want scoped one-event snapshot refresh", second)
	}

	groups, err := integration.client.AlertGroup.Query().All(ctx)
	if err != nil {
		t.Fatalf("list alert groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("alert groups = %d, want 1", len(groups))
	}
	wantLastSeen := domain.NormalizeUTCMicro(prometheusAlert.StartsAt)
	if groups[0].EventCount != 1 ||
		groups[0].Severity != string(domain.GroupSeverityWarning) ||
		!groups[0].LastSeenAt.Equal(wantLastSeen) {
		t.Fatalf("group = event_count:%d severity:%s last_seen:%s, want scoped prometheus event", groups[0].EventCount, groups[0].Severity, groups[0].LastSeenAt)
	}

	snapshot, err := integration.client.EvidenceSnapshot.Get(ctx, int(second.Snapshots[0].ID))
	if err != nil {
		t.Fatalf("get scoped evidence snapshot: %v", err)
	}
	var payload struct {
		Events []struct {
			Source string `json:"source"`
		} `json:"events"`
	}
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		t.Fatalf("unmarshal scoped snapshot payload: %v", err)
	}
	if len(payload.Events) != 1 || payload.Events[0].Source != "prometheus" {
		t.Fatalf("snapshot events = %+v, want only prometheus event", payload.Events)
	}
	if got := countEventGroupLinks(ctx, t); got != 2 {
		t.Fatalf("alert_event_groups count = %d, want existing historical links retained", got)
	}
}

func TestReplayWindow_SourceFilterExcludesNonMatchingEventsFromGrouping(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	prometheusAlert := seedAlerts("AlertA", 1, windowStart, 0, time.Minute, "warning")[0]
	alertmanagerAlert := prometheusAlert
	alertmanagerAlert.Source = "alertmanager"
	alertmanagerAlert.Labels = map[string]string{
		"alertname": "AlertB",
		"instance":  "AlertB-0",
		"severity":  "critical",
	}
	provider := fake.New([]ports.ActiveAlert{prometheusAlert, alertmanagerAlert})
	req := defaultRequest(windowStart, windowEnd)
	req.SourceFilter = []string{"prometheus"}

	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, req)
	if err != nil {
		t.Fatalf("ReplayWindow: %v", err)
	}

	if stats.Ingested.Total != 2 || stats.Ingested.Saved != 2 {
		t.Fatalf("Ingested = %+v, want Total=2 Saved=2", stats.Ingested)
	}
	if stats.EventsLoaded != 1 {
		t.Fatalf("EventsLoaded = %d, want 1 filtered in-window event", stats.EventsLoaded)
	}
	if stats.GroupsBuilt != 1 || stats.GroupsSaved != 1 || stats.SnapshotsSaved != 1 {
		t.Fatalf("group stats = %+v, want one matching source group", stats)
	}
	if got := countAlertEvents(ctx, t); got != 2 {
		t.Fatalf("alert_event count = %d, want both provider events persisted", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 1 {
		t.Fatalf("alert_event_groups count = %d, want only matching source linked", got)
	}
}

func TestReplayWindow_SourceProfileFilterExcludesNonMatchingEventsFromGrouping(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	matching := seedAlerts("AlertA", 1, windowStart, 0, time.Minute, "warning")[0]
	matching.AlertSourceProfileID = 7
	other := seedAlerts("AlertB", 1, windowStart, 0, time.Minute, "critical")[0]
	other.AlertSourceProfileID = 9
	provider := fake.New([]ports.ActiveAlert{matching, other})
	req := defaultRequest(windowStart, windowEnd)
	req.AlertSourceProfileFilter = []domain.AlertSourceProfileID{7}

	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, req)
	if err != nil {
		t.Fatalf("ReplayWindow: %v", err)
	}

	if stats.Ingested.Total != 2 || stats.Ingested.Saved != 2 {
		t.Fatalf("Ingested = %+v, want Total=2 Saved=2", stats.Ingested)
	}
	if stats.EventsLoaded != 1 {
		t.Fatalf("EventsLoaded = %d, want 1 filtered in-window event", stats.EventsLoaded)
	}
	if stats.GroupsBuilt != 1 || stats.GroupsSaved != 1 || stats.SnapshotsSaved != 1 {
		t.Fatalf("group stats = %+v, want one matching source profile group", stats)
	}
	if got := countEventGroupLinks(ctx, t); got != 1 {
		t.Fatalf("alert_event_groups count = %d, want only matching source profile linked", got)
	}
}

func TestReplayWindow_SourceProfileFilterAppliedBeforeLimit(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)
	matching := seedAlerts("AlertA", 1, windowStart, 0, time.Minute, "warning")[0]
	matching.AlertSourceProfileID = 7
	alerts := []ports.ActiveAlert{matching}
	for i := 0; i < 4; i++ {
		other := seedAlerts(fmt.Sprintf("Other%d", i), 1, windowStart, time.Duration(i+1)*time.Minute, time.Minute, "critical")[0]
		other.AlertSourceProfileID = 9
		alerts = append(alerts, other)
	}
	provider := fake.New(alerts)
	req := defaultRequest(windowStart, windowEnd)
	req.AlertSourceProfileFilter = []domain.AlertSourceProfileID{7}
	req.Limit = 1

	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, req)
	if err != nil {
		t.Fatalf("ReplayWindow: %v", err)
	}
	if stats.EventsLoaded != 1 || stats.GroupsBuilt != 1 || stats.GroupsSaved != 1 {
		t.Fatalf("stats = %+v, want one filtered event without unrelated-profile limit failure", stats)
	}
	if got := countAlertEvents(ctx, t); got != 5 {
		t.Fatalf("alert_event count = %d, want all provider events persisted", got)
	}
}

// TestReplayWindow_MultipleAlertnamesYieldMultipleGroups: covers
// the simplest non-trivial grouping case (2 buckets, asymmetric
// sizes) so a regression that collapses everything into one bucket
// surfaces as a count mismatch.
func TestReplayWindow_MultipleAlertnamesYieldMultipleGroups(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	var batch []ports.ActiveAlert
	batch = append(batch, seedAlerts("AlertA", 3, windowStart, 0, time.Minute, "warning")...)
	batch = append(batch, seedAlerts("AlertB", 2, windowStart, 30*time.Minute, time.Minute, "critical")...)

	provider := fake.New(batch)
	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("ReplayWindow: %v", err)
	}

	if stats.GroupsBuilt != 2 {
		t.Errorf("GroupsBuilt = %d, want 2", stats.GroupsBuilt)
	}
	if stats.GroupsSaved != 2 {
		t.Errorf("GroupsSaved = %d, want 2", stats.GroupsSaved)
	}
	if stats.SnapshotsSaved != 2 {
		t.Errorf("SnapshotsSaved = %d, want 2", stats.SnapshotsSaved)
	}

	// The two snapshots MUST have different digests because the
	// underlying group payloads differ (alertname, severity,
	// event count). Eyes on this: a bug that reuses one digest
	// would still pass count assertions because (group_id,
	// digest) is the per-group idempotency key.
	digests, err := integration.client.EvidenceSnapshot.Query().Select(evidencesnapshot.FieldDigest).Strings(ctx)
	if err != nil {
		t.Fatalf("query digests: %v", err)
	}
	if len(digests) != 2 {
		t.Fatalf("digests = %v, want 2 entries", digests)
	}
	if digests[0] == digests[1] {
		t.Errorf("snapshot digests collapsed to %q for two distinct groups", digests[0])
	}
}

// TestReplayWindow_GroupClosedAfterSnapshot asserts the close
// transition runs after Save (not before): the persisted group has
// status=closed and its UpdatedAt has advanced past CreatedAt.
// We assert "not Before" rather than strictly ">" because Postgres'
// timestamptz column resolves to microseconds and the close
// transition can land in the same microsecond as the insert on
// fast hosts -- a strict ">" check is a documented flake source.
func TestReplayWindow_GroupClosedAfterSnapshot(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	provider := fake.New(seedAlerts("AlertA", 3, windowStart, 0, time.Minute, "warning"))
	if _, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, defaultRequest(windowStart, windowEnd)); err != nil {
		t.Fatalf("ReplayWindow: %v", err)
	}

	row, err := integration.client.AlertGroup.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query single alert_group: %v", err)
	}
	if row.Status != string(domain.AlertGroupStatusClosed) {
		t.Errorf("group.Status = %q, want %q", row.Status, domain.AlertGroupStatusClosed)
	}
	if row.UpdatedAt.IsZero() {
		t.Errorf("group.UpdatedAt is zero, want non-zero after close")
	}
	if row.UpdatedAt.Before(row.CreatedAt) {
		t.Errorf("group.UpdatedAt %s is Before CreatedAt %s", row.UpdatedAt, row.CreatedAt)
	}

	if got := countEvidenceSnapshots(ctx, t); got != 1 {
		t.Errorf("evidence_snapshot count = %d, want 1", got)
	}
}

// TestReplayWindow_RefreshOnNewEventInExistingGroup exercises D7
// directly: after run 1 the group is closed, and run 2 brings a
// new in-window event for the same (group_key, first_seen_at).
// The replay harness allows the closed group to refresh its mutable
// fields and produces a new snapshot row (because the digest
// changes). Status MUST stay closed -- refresh does not reopen.
func TestReplayWindow_RefreshOnNewEventInExistingGroup(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	run1 := seedAlerts("AlertA", 3, windowStart, 0, 10*time.Minute, "warning")
	provider1 := fake.New(run1)
	if _, err := alertreplay.ReplayWindow(ctx, provider1, integration.factory, defaultRequest(windowStart, windowEnd)); err != nil {
		t.Fatalf("first ReplayWindow: %v", err)
	}

	// Sanity: group is closed after run 1.
	if got := countClosedGroups(ctx, t); got != 1 {
		t.Fatalf("after run 1: closed groups = %d, want 1", got)
	}

	// Run 2: same 3 alerts + 1 additional alertname=A instance at
	// a later StartsAt (still in-window, after the existing
	// LastSeenAt). The new event has a unique instance label so
	// its canonical fingerprint differs from the run 1 set.
	run2 := append([]ports.ActiveAlert(nil), run1...)
	run2 = append(run2, ports.ActiveAlert{
		Source: "prometheus",
		Labels: map[string]string{
			"alertname": "AlertA",
			"instance":  "AlertA-extra",
			"severity":  "warning",
		},
		Annotations: map[string]string{"summary": "AlertA firing"},
		StartsAt:    windowStart.Add(45 * time.Minute),
		RawPayload:  json.RawMessage(`{"raw":2}`),
	})
	provider2 := fake.New(run2)

	stats, err := alertreplay.ReplayWindow(ctx, provider2, integration.factory, defaultRequest(windowStart, windowEnd))
	if err != nil {
		t.Fatalf("second ReplayWindow: %v", err)
	}

	if stats.Ingested.Total != 4 || stats.Ingested.Saved != 1 || stats.Ingested.Duplicate != 3 {
		t.Errorf("Ingested = %+v, want Total=4 Saved=1 Duplicate=3", stats.Ingested)
	}
	if stats.EventsLoaded != 4 {
		t.Errorf("EventsLoaded = %d, want 4", stats.EventsLoaded)
	}
	if stats.GroupsBuilt != 1 {
		t.Errorf("GroupsBuilt = %d, want 1", stats.GroupsBuilt)
	}
	if stats.GroupsRefreshed != 1 {
		t.Errorf("GroupsRefreshed = %d, want 1 (event_count and last_seen_at changed)", stats.GroupsRefreshed)
	}
	if stats.GroupsSaved != 0 || stats.GroupsExisting != 0 {
		t.Errorf("GroupsSaved=%d GroupsExisting=%d, both want 0 when refresh fires", stats.GroupsSaved, stats.GroupsExisting)
	}
	if stats.SnapshotsSaved != 1 || stats.SnapshotsDuplicate != 0 {
		t.Errorf("snapshots: saved=%d duplicate=%d, want saved=1 duplicate=0 (digest changed with new event)", stats.SnapshotsSaved, stats.SnapshotsDuplicate)
	}
	if stats.GroupsClosed != 0 {
		t.Errorf("GroupsClosed = %d, want 0 (already closed after run 1; refresh does NOT reopen)", stats.GroupsClosed)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0", stats.Failed)
	}

	// DB checks: 4 events, still 1 group, 2 snapshot rows
	// (run1 + run2 with different digests), 4 M2N links.
	if got := countAlertEvents(ctx, t); got != 4 {
		t.Errorf("alert_event count = %d, want 4", got)
	}
	if got := countAlertGroups(ctx, t); got != 1 {
		t.Errorf("alert_group count = %d, want 1", got)
	}
	if got := countClosedGroups(ctx, t); got != 1 {
		t.Errorf("closed group count = %d, want 1 (status preserved across refresh)", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 2 {
		t.Errorf("evidence_snapshot count = %d, want 2 (run1 + run2 keep both rows)", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 4 {
		t.Errorf("alert_event_groups count = %d, want 4", got)
	}

	// The persisted group must reflect the refreshed mutable
	// fields (event_count=4, last_seen_at moved to the new event)
	// while keeping status=closed.
	row, err := integration.client.AlertGroup.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query single group: %v", err)
	}
	if row.EventCount != 4 {
		t.Errorf("group.EventCount = %d, want 4", row.EventCount)
	}
	wantLast := domain.NormalizeUTCMicro(windowStart.Add(45 * time.Minute))
	if !row.LastSeenAt.Equal(wantLast) {
		t.Errorf("group.LastSeenAt = %s, want %s", row.LastSeenAt, wantLast)
	}
	if row.Status != string(domain.AlertGroupStatusClosed) {
		t.Errorf("group.Status = %q, want %q (refresh preserves closed)", row.Status, domain.AlertGroupStatusClosed)
	}
}

// TestReplayWindow_LimitSafetyValve: 5 in-window events with
// Limit=4. The replay must abort the run with
// ErrInvariantViolation, leave Stats.EventsLoaded == Limit+1 so a
// caller can diagnose, and not write any group / snapshot / link
// rows. AlertEvents from Step 1 are intentionally NOT rolled back:
// IngestOnce wrote them in their own transactions before Step 2
// even ran.
func TestReplayWindow_LimitSafetyValve(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	provider := fake.New(seedAlerts("AlertA", 5, windowStart, 0, time.Minute, "warning"))

	req := defaultRequest(windowStart, windowEnd)
	req.Limit = 4

	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, req)
	if err == nil {
		t.Fatalf("ReplayWindow: want non-nil error, got nil")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Errorf("err: want errors.Is ErrInvariantViolation, got %v", err)
	}
	if !strings.Contains(err.Error(), "more than limit") {
		t.Errorf("err message = %q, want it to mention 'more than limit'", err.Error())
	}

	if stats.EventsLoaded != req.Limit+1 {
		t.Errorf("EventsLoaded = %d, want %d (Limit+1 must reach Stats so callers can diagnose)", stats.EventsLoaded, req.Limit+1)
	}
	if stats.GroupsBuilt != 0 {
		t.Errorf("GroupsBuilt = %d, want 0 (Step 2 fail short-circuits Step 3)", stats.GroupsBuilt)
	}
	if stats.GroupsSaved != 0 || stats.GroupsRefreshed != 0 || stats.GroupsExisting != 0 {
		t.Errorf("group writes leaked: saved=%d refreshed=%d existing=%d, want all 0", stats.GroupsSaved, stats.GroupsRefreshed, stats.GroupsExisting)
	}
	if stats.SnapshotsSaved != 0 || stats.SnapshotsDuplicate != 0 {
		t.Errorf("snapshot writes leaked: saved=%d duplicate=%d, want both 0", stats.SnapshotsSaved, stats.SnapshotsDuplicate)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0 (safety valve is NOT a per-group failure)", stats.Failed)
	}

	// AlertEvents from Step 1 persist (their per-event tx already
	// committed); the safety valve only protects Step 3+.
	if got := countAlertEvents(ctx, t); got != 5 {
		t.Errorf("alert_event count = %d, want 5", got)
	}
	if got := countAlertGroups(ctx, t); got != 0 {
		t.Errorf("alert_group count = %d, want 0", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 0 {
		t.Errorf("evidence_snapshot count = %d, want 0", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 0 {
		t.Errorf("alert_event_groups count = %d, want 0", got)
	}
}

// TestReplayWindow_RequestValidationRejected covers the orchestrator's
// boundary self-defence. Each sub-case asserts that an
// ErrInvariantViolation is returned BEFORE any DB write happens.
// Note: an "empty Grouping" sub-case is intentionally absent here
// because GroupEvents short-circuits on empty events without
// consuming the config; the empty-grouping path needs at least one
// in-window event and is covered separately by
// TestReplayWindow_EmptyGroupingConfigRejectedWhenEventsExist.
func TestReplayWindow_RequestValidationRejected(t *testing.T) {
	ctx := context.Background()
	provider := fake.New(nil)
	good := defaultRequest(seedTime, seedTime.Add(time.Hour))

	// A sub-microsecond start/end pair that NormalizeUTCMicro
	// collapses to identical microseconds: 12:00:00.000000500 and
	// 12:00:00.000000800 both truncate to 12:00:00.000000.
	microCollapseStart := time.Date(2026, 5, 26, 12, 0, 0, 500, time.UTC)
	microCollapseEnd := time.Date(2026, 5, 26, 12, 0, 0, 800, time.UTC)

	cases := []struct {
		name     string
		provider ports.MetricsProvider
		factory  ports.UnitOfWorkFactory
		req      alertreplay.Request
	}{
		{
			name:     "nil_provider",
			provider: nil,
			factory:  integration.factory,
			req:      good,
		},
		{
			name:     "nil_factory",
			provider: provider,
			factory:  nil,
			req:      good,
		},
		{
			name:     "zero_window_start",
			provider: provider,
			factory:  integration.factory,
			req:      withWindow(good, time.Time{}, good.WindowEnd),
		},
		{
			name:     "zero_window_end",
			provider: provider,
			factory:  integration.factory,
			req:      withWindow(good, good.WindowStart, time.Time{}),
		},
		{
			name:     "end_equals_start",
			provider: provider,
			factory:  integration.factory,
			req:      withWindow(good, seedTime, seedTime),
		},
		{
			name:     "end_before_start",
			provider: provider,
			factory:  integration.factory,
			req:      withWindow(good, seedTime.Add(time.Hour), seedTime),
		},
		{
			name:     "end_collapses_to_start_after_normalisation",
			provider: provider,
			factory:  integration.factory,
			req:      withWindow(good, microCollapseStart, microCollapseEnd),
		},
		{
			name:     "limit_zero",
			provider: provider,
			factory:  integration.factory,
			req:      withLimit(good, 0),
		},
		{
			name:     "limit_negative",
			provider: provider,
			factory:  integration.factory,
			req:      withLimit(good, -1),
		},
		{
			name:     "limit_max_int_overflow_guard",
			provider: provider,
			factory:  integration.factory,
			req:      withLimit(good, math.MaxInt),
		},
		{
			name:     "zero_alert_event_id_filter",
			provider: provider,
			factory:  integration.factory,
			req:      withAlertEventIDFilter(good, []domain.AlertEventID{0}),
		},
		{
			name:     "negative_alert_event_id_filter",
			provider: provider,
			factory:  integration.factory,
			req:      withAlertEventIDFilter(good, []domain.AlertEventID{-1}),
		},
		{
			name:     "negative_source_profile_filter",
			provider: provider,
			factory:  integration.factory,
			req:      withSourceProfileFilter(good, []domain.AlertSourceProfileID{-1}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetDB(t)
			stats, err := alertreplay.ReplayWindow(ctx, tc.provider, tc.factory, tc.req)
			if err == nil {
				t.Fatalf("ReplayWindow: want non-nil error, got nil (stats=%+v)", stats)
			}
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Errorf("err: want errors.Is ErrInvariantViolation, got %v", err)
			}

			// No side effects allowed for any validation
			// failure: the function must reject before
			// reaching IngestOnce.
			if got := countAlertEvents(ctx, t); got != 0 {
				t.Errorf("alert_event count = %d, want 0 (validation must short-circuit before ingest)", got)
			}
			if got := countAlertGroups(ctx, t); got != 0 {
				t.Errorf("alert_group count = %d, want 0", got)
			}
			if got := countEvidenceSnapshots(ctx, t); got != 0 {
				t.Errorf("evidence_snapshot count = %d, want 0", got)
			}
		})
	}
}

// withWindow returns r with WindowStart/WindowEnd overridden. The
// helpers exist so the case table reads as "this is the field we
// are mutating" instead of inline struct copies.
func withWindow(r alertreplay.Request, start, end time.Time) alertreplay.Request {
	r.WindowStart = start
	r.WindowEnd = end
	return r
}

func withLimit(r alertreplay.Request, limit int) alertreplay.Request {
	r.Limit = limit
	return r
}

func withAlertEventIDFilter(
	r alertreplay.Request,
	filter []domain.AlertEventID,
) alertreplay.Request {
	r.AlertEventIDFilter = filter
	return r
}

func withSourceProfileFilter(
	r alertreplay.Request,
	filter []domain.AlertSourceProfileID,
) alertreplay.Request {
	r.AlertSourceProfileFilter = filter
	return r
}

// TestReplayWindow_EmptyGroupingConfigRejectedWhenEventsExist
// complements RequestValidationRejected. With at least one
// in-window event, GroupEvents proceeds past its empty-input fast
// path and runs validateConfig, which rejects an empty
// DimensionKeys with ErrInvariantViolation. The replay must surface
// that error verbatim and leave the per-group stages untouched.
func TestReplayWindow_EmptyGroupingConfigRejectedWhenEventsExist(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	provider := fake.New(seedAlerts("AlertA", 1, windowStart, 0, time.Minute, "warning"))
	req := alertreplay.Request{
		WindowStart:       windowStart,
		WindowEnd:         windowEnd,
		Grouping:          alertgrouping.Config{}, // both fields empty -> rejected once consumed
		CreatedByWorkflow: "test-workflow",
		Limit:             100,
	}

	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, req)
	if err == nil {
		t.Fatalf("ReplayWindow: want non-nil error, got nil")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Errorf("err: want errors.Is ErrInvariantViolation, got %v", err)
	}

	// Step 1 ran (Ingested.Saved=1) and Step 2 saw the event,
	// so EventsLoaded=1, but Step 3 failed before any per-group
	// work. Failed must remain 0: this is NOT a per-group tx
	// failure.
	if stats.Ingested.Total != 1 || stats.Ingested.Saved != 1 {
		t.Errorf("Ingested = %+v, want Total=1 Saved=1", stats.Ingested)
	}
	if stats.EventsLoaded != 1 {
		t.Errorf("EventsLoaded = %d, want 1", stats.EventsLoaded)
	}
	if stats.GroupsBuilt != 0 {
		t.Errorf("GroupsBuilt = %d, want 0 (GroupEvents rejected before producing drafts)", stats.GroupsBuilt)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0 (Step 3 failure is not a per-group tx fail)", stats.Failed)
	}

	// Ingest persisted 1 event in its own tx; nothing past Step 3
	// must touch the database.
	if got := countAlertEvents(ctx, t); got != 1 {
		t.Errorf("alert_event count = %d, want 1", got)
	}
	if got := countAlertGroups(ctx, t); got != 0 {
		t.Errorf("alert_group count = %d, want 0", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 0 {
		t.Errorf("evidence_snapshot count = %d, want 0", got)
	}
	if got := countEventGroupLinks(ctx, t); got != 0 {
		t.Errorf("alert_event_groups count = %d, want 0", got)
	}
}

// TestReplayWindow_EmptyWindowReturnsZeroStats: with an empty
// provider seed (no alerts at all) ReplayWindow must touch nothing
// and return a zero-valued Stats. The empty-events path also must
// NOT consume the grouping config -- pass an empty Grouping to make
// that explicit.
func TestReplayWindow_EmptyWindowReturnsZeroStats(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	windowStart := seedTime
	windowEnd := seedTime.Add(time.Hour)

	provider := fake.New(nil)
	req := alertreplay.Request{
		WindowStart:       windowStart,
		WindowEnd:         windowEnd,
		Grouping:          alertgrouping.Config{}, // empty config; must NOT be consumed
		CreatedByWorkflow: "test-workflow",
		Limit:             100,
	}

	stats, err := alertreplay.ReplayWindow(ctx, provider, integration.factory, req)
	if err != nil {
		t.Fatalf("ReplayWindow: %v", err)
	}

	if stats.Ingested != (alertingest.Stats{}) {
		t.Errorf("Ingested = %+v, want zero-valued Stats", stats.Ingested)
	}
	if stats.EventsLoaded != 0 {
		t.Errorf("EventsLoaded = %d, want 0", stats.EventsLoaded)
	}
	if stats.GroupsBuilt != 0 {
		t.Errorf("GroupsBuilt = %d, want 0", stats.GroupsBuilt)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0", stats.Failed)
	}

	if got := countAlertEvents(ctx, t); got != 0 {
		t.Errorf("alert_event count = %d, want 0", got)
	}
	if got := countAlertGroups(ctx, t); got != 0 {
		t.Errorf("alert_group count = %d, want 0", got)
	}
	if got := countEvidenceSnapshots(ctx, t); got != 0 {
		t.Errorf("evidence_snapshot count = %d, want 0", got)
	}
}
