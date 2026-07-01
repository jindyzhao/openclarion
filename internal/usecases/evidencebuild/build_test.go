package evidencebuild

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

// --------------- test helpers ---------------

func validGroup() domain.AlertGroup {
	return domain.AlertGroup{
		ID:          1,
		GroupKey:    "abc123def456",
		Dimensions:  json.RawMessage(`{"alertname":"HighCPU","cluster":"prod"}`),
		Severity:    domain.GroupSeverityCritical,
		EventCount:  2,
		Status:      domain.AlertGroupStatusActive,
		FirstSeenAt: time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC),
		LastSeenAt:  time.Date(2026, 5, 26, 10, 5, 0, 0, time.UTC),
		EventIDs:    nil, // optional cross-check disabled
	}
}

func validEvents() []domain.AlertEvent {
	return []domain.AlertEvent{
		{
			ID:                   10,
			Source:               "prometheus",
			AlertSourceProfileID: 7,
			SourceFingerprint:    "sfp1",
			CanonicalFingerprint: "cfp1",
			Labels:               map[string]string{"alertname": "HighCPU", "severity": "critical"},
			Annotations:          map[string]string{"summary": "CPU high"},
			RawPayload:           json.RawMessage(`{"metric":"cpu_usage","value":95.2}`),
			Status:               domain.AlertStatusFiring,
			StartsAt:             time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:                   20,
			Source:               "prometheus",
			SourceFingerprint:    "sfp2",
			CanonicalFingerprint: "cfp2",
			Labels:               map[string]string{"alertname": "HighCPU", "severity": "critical"},
			Annotations:          map[string]string{"summary": "CPU high node-2"},
			RawPayload:           json.RawMessage(`{"metric":"cpu_usage","value":97.1}`),
			Status:               domain.AlertStatusFiring,
			StartsAt:             time.Date(2026, 5, 26, 10, 3, 0, 0, time.UTC),
		},
	}
}

func validInput() Input {
	return Input{
		Group:             validGroup(),
		Events:            validEvents(),
		CreatedByWorkflow: "diagnosis-workflow-1",
	}
}

func mustBeInvariantViolation(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("expected ErrInvariantViolation, got: %v", err)
	}
}

// --------------- deterministic / happy path tests ---------------

func TestBuildSnapshot_HappyPath(t *testing.T) {
	in := validInput()
	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.AlertGroupID != in.Group.ID {
		t.Errorf("AlertGroupID = %d, want %d", snap.AlertGroupID, in.Group.ID)
	}
	if snap.Digest == "" {
		t.Error("Digest is empty")
	}
	if len(snap.Digest) != 64 {
		t.Errorf("Digest length = %d, want 64 hex chars", len(snap.Digest))
	}
	if snap.Status != domain.SnapshotStatusComplete {
		t.Errorf("Status = %q, want complete", snap.Status)
	}
	if snap.MissingFields != nil {
		t.Errorf("MissingFields = %v, want nil", snap.MissingFields)
	}
	if snap.CreatedByWorkflow != "diagnosis-workflow-1" {
		t.Errorf("CreatedByWorkflow = %q, want diagnosis-workflow-1", snap.CreatedByWorkflow)
	}
	if len(snap.Payload) == 0 {
		t.Error("Payload is empty")
	}
}

func TestBuildSnapshot_PayloadDeterministic(t *testing.T) {
	in := validInput()
	snap1, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}

	// Shuffle events: reverse order.
	in2 := validInput()
	in2.Events[0], in2.Events[1] = in2.Events[1], in2.Events[0]
	snap2, err := BuildSnapshot(in2)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}

	// Shuffle again: original order (redundant but tests 3 orderings).
	in3 := validInput()
	snap3, err := BuildSnapshot(in3)
	if err != nil {
		t.Fatalf("run 3: %v", err)
	}

	if snap1.Digest != snap2.Digest {
		t.Errorf("digest mismatch between run 1 and 2:\n  run1=%s\n  run2=%s", snap1.Digest, snap2.Digest)
	}
	if snap1.Digest != snap3.Digest {
		t.Errorf("digest mismatch between run 1 and 3")
	}
	if string(snap1.Payload) != string(snap2.Payload) {
		t.Error("payload bytes differ between run 1 and 2")
	}
}

func TestBuildSnapshot_DigestChangesOnLabelChange(t *testing.T) {
	in1 := validInput()
	snap1, _ := BuildSnapshot(in1)

	in2 := validInput()
	in2.Events[0].Labels["extra"] = "value"
	snap2, _ := BuildSnapshot(in2)

	if snap1.Digest == snap2.Digest {
		t.Error("digest should differ when labels change")
	}
}

func TestBuildSnapshot_DigestChangesOnAnnotationChange(t *testing.T) {
	in1 := validInput()
	snap1, _ := BuildSnapshot(in1)

	in2 := validInput()
	in2.Events[0].Annotations["runbook"] = "https://example.com"
	snap2, _ := BuildSnapshot(in2)

	if snap1.Digest == snap2.Digest {
		t.Error("digest should differ when annotations change")
	}
}

func TestBuildSnapshot_DigestChangesOnRawPayloadChange(t *testing.T) {
	in1 := validInput()
	snap1, _ := BuildSnapshot(in1)

	in2 := validInput()
	in2.Events[0].RawPayload = json.RawMessage(`{"metric":"cpu_usage","value":50.0}`)
	snap2, _ := BuildSnapshot(in2)

	if snap1.Digest == snap2.Digest {
		t.Error("digest should differ when raw_payload changes")
	}
}

func TestBuildSnapshot_EventOrderInPayload(t *testing.T) {
	in := validInput()
	// Reverse so event with later StartsAt comes first in input.
	in.Events[0], in.Events[1] = in.Events[1], in.Events[0]

	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var p snapshotPayload
	if err := json.Unmarshal(snap.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(p.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(p.Events))
	}
	// Event with StartsAt 10:00 (ID=10) should come before 10:03 (ID=20).
	if p.Events[0].ID != 10 {
		t.Errorf("first event ID = %d, want 10 (earlier StartsAt)", p.Events[0].ID)
	}
	if p.Events[1].ID != 20 {
		t.Errorf("second event ID = %d, want 20", p.Events[1].ID)
	}
}

func TestBuildSnapshot_IncludesAlertSourceProfileIDWhenPresent(t *testing.T) {
	in := validInput()
	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}

	var p snapshotPayload
	if err := json.Unmarshal(snap.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(p.Events) != 2 {
		t.Fatalf("events len = %d, want 2", len(p.Events))
	}
	if p.Events[0].AlertSourceProfileID != 7 {
		t.Fatalf("events[0].alert_source_profile_id = %d, want 7", p.Events[0].AlertSourceProfileID)
	}
	if p.Events[1].AlertSourceProfileID != 0 {
		t.Fatalf("events[1].alert_source_profile_id = %d, want omitted/zero", p.Events[1].AlertSourceProfileID)
	}
}

func TestBuildSnapshot_RawPayloadKeyOrderIrrelevant(t *testing.T) {
	in1 := validInput()
	in1.Events[0].RawPayload = json.RawMessage(`{"a":1,"b":2}`)

	in2 := validInput()
	in2.Events[0].RawPayload = json.RawMessage(`{"b":2,"a":1}`)

	snap1, err := BuildSnapshot(in1)
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	snap2, err := BuildSnapshot(in2)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}

	if snap1.Digest != snap2.Digest {
		t.Errorf("digest should be identical for semantically equivalent raw_payload:\n  d1=%s\n  d2=%s", snap1.Digest, snap2.Digest)
	}
}

func TestBuildSnapshot_RawPayloadDuplicateKeysRejected(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "top-level duplicate",
			raw:  json.RawMessage(`{"metric":"old","metric":"new"}`),
			want: `duplicate object key "metric"`,
		},
		{
			name: "nested duplicate",
			raw:  json.RawMessage(`{"metric":"cpu_usage","labels":{"node":"a","node":"b"}}`),
			want: `duplicate object key "node"`,
		},
		{
			name: "trailing value",
			raw:  json.RawMessage(`{"metric":"cpu_usage"}[]`),
			want: "trailing JSON values",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			in.Events[0].RawPayload = tt.raw

			_, err := BuildSnapshot(in)
			mustBeInvariantViolation(t, err)
			if !contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestBuildSnapshot_DimensionsKeyOrderIrrelevant(t *testing.T) {
	in1 := validInput()
	in1.Group.Dimensions = json.RawMessage(`{"alertname":"HighCPU","cluster":"prod"}`)

	in2 := validInput()
	in2.Group.Dimensions = json.RawMessage(`{ "cluster" : "prod" , "alertname" : "HighCPU" }`)

	snap1, err := BuildSnapshot(in1)
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	snap2, err := BuildSnapshot(in2)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}

	if snap1.Digest != snap2.Digest {
		t.Errorf("digest should be identical for semantically equivalent dimensions:\n  d1=%s\n  d2=%s", snap1.Digest, snap2.Digest)
	}
}

func TestBuildSnapshot_DimensionsDuplicateKeysRejected(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "top-level duplicate",
			raw:  json.RawMessage(`{"alertname":"HighCPU","alertname":"HighMemory"}`),
			want: `duplicate object key "alertname"`,
		},
		{
			name: "nested duplicate",
			raw:  json.RawMessage(`{"alertname":"HighCPU","scope":{"cluster":"prod","cluster":"stage"}}`),
			want: `duplicate object key "cluster"`,
		},
		{
			name: "trailing value",
			raw:  json.RawMessage(`{"alertname":"HighCPU"}{}`),
			want: "trailing JSON values",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			in.Group.Dimensions = tt.raw

			_, err := BuildSnapshot(in)
			mustBeInvariantViolation(t, err)
			if !contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestBuildSnapshot_NilRawPayloadRendersAsNull(t *testing.T) {
	in := validInput()
	in.Events[0].RawPayload = nil
	in.Events[1].RawPayload = nil

	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var p snapshotPayload
	if err := json.Unmarshal(snap.Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for i, ev := range p.Events {
		if ev.RawPayload != nil {
			t.Errorf("event[%d] raw_payload should be nil pointer (JSON null), got %s", i, string(*ev.RawPayload))
		}
	}
}

func TestBuildSnapshot_NilLabelsAnnotationsRenderAsEmptyObject(t *testing.T) {
	in := validInput()
	in.Events[0].Labels = nil
	in.Events[0].Annotations = nil

	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var p snapshotPayload
	if err := json.Unmarshal(snap.Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Events[0].Labels == nil {
		t.Error("labels should be empty map, not nil")
	}
	if len(p.Events[0].Labels) != 0 {
		t.Errorf("labels should be empty, got %v", p.Events[0].Labels)
	}
	if p.Events[0].Annotations == nil {
		t.Error("annotations should be empty map, not nil")
	}
}

// --------------- validation error tests ---------------

func TestBuildSnapshot_GroupEventCountMismatch(t *testing.T) {
	in := validInput()
	in.Group.EventCount = 99
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_ZeroGroupID(t *testing.T) {
	in := validInput()
	in.Group.ID = 0
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_EmptyGroupKey(t *testing.T) {
	in := validInput()
	in.Group.GroupKey = ""
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_ZeroGroupFirstSeenAt(t *testing.T) {
	in := validInput()
	in.Group.FirstSeenAt = time.Time{}
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_ZeroGroupLastSeenAt(t *testing.T) {
	in := validInput()
	in.Group.LastSeenAt = time.Time{}
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_GroupLastSeenBeforeFirstSeen(t *testing.T) {
	in := validInput()
	in.Group.LastSeenAt = in.Group.FirstSeenAt.Add(-1 * time.Minute)
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_InvalidGroupDimensions(t *testing.T) {
	cases := []struct {
		name string
		dims json.RawMessage
	}{
		{"nil", nil},
		{"empty", json.RawMessage{}},
		{"invalid json", json.RawMessage(`{invalid}`)},
		{"json array", json.RawMessage(`[1,2,3]`)},
		{"json string", json.RawMessage(`"hello"`)},
		{"json null", json.RawMessage(`null`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			in.Group.Dimensions = tc.dims
			_, err := BuildSnapshot(in)
			mustBeInvariantViolation(t, err)
		})
	}
}

func TestBuildSnapshot_EmptyEvents(t *testing.T) {
	in := validInput()
	in.Events = nil
	in.Group.EventCount = 0
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_ZeroEventID(t *testing.T) {
	in := validInput()
	in.Events[0].ID = 0
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_ZeroEventStartsAt(t *testing.T) {
	in := validInput()
	in.Events[0].StartsAt = time.Time{}
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_EmptyEventSource(t *testing.T) {
	in := validInput()
	in.Events[0].Source = ""
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_EmptyEventFingerprints(t *testing.T) {
	t.Run("canonical empty", func(t *testing.T) {
		in := validInput()
		in.Events[0].CanonicalFingerprint = ""
		_, err := BuildSnapshot(in)
		mustBeInvariantViolation(t, err)
	})
	t.Run("source empty", func(t *testing.T) {
		in := validInput()
		in.Events[0].SourceFingerprint = ""
		_, err := BuildSnapshot(in)
		mustBeInvariantViolation(t, err)
	})
}

func TestBuildSnapshot_InvalidNonEmptyRawPayload(t *testing.T) {
	in := validInput()
	in.Events[0].RawPayload = json.RawMessage(`{not valid json}`)
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_EventIDSetMismatch(t *testing.T) {
	in := validInput()
	in.Group.EventIDs = []domain.AlertEventID{10, 999} // 999 not in events
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_EventIDSetNilSkipsCheck(t *testing.T) {
	in := validInput()
	in.Group.EventIDs = nil
	_, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildSnapshot_EventOutsideTimeRange(t *testing.T) {
	in := validInput()
	// Event starts before group FirstSeenAt.
	in.Events[0].StartsAt = in.Group.FirstSeenAt.Add(-10 * time.Minute)
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_InvalidGroupSeverity(t *testing.T) {
	in := validInput()
	in.Group.Severity = "bogus"
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_DuplicateEventIDs(t *testing.T) {
	t.Run("duplicate in events", func(t *testing.T) {
		in := validInput()
		in.Events[1].ID = in.Events[0].ID // same ID
		_, err := BuildSnapshot(in)
		mustBeInvariantViolation(t, err)
	})
	t.Run("duplicate in group.EventIDs", func(t *testing.T) {
		in := validInput()
		in.Group.EventIDs = []domain.AlertEventID{10, 10}
		_, err := BuildSnapshot(in)
		mustBeInvariantViolation(t, err)
	})
}

// --------------- output field tests ---------------

func TestBuildSnapshot_ProvenanceAndStatus(t *testing.T) {
	in := validInput()
	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap.Status != domain.SnapshotStatusComplete {
		t.Errorf("Status = %q, want complete", snap.Status)
	}
	if snap.MissingFields != nil {
		t.Errorf("MissingFields = %v, want nil", snap.MissingFields)
	}

	var prov provenancePayload
	if err := json.Unmarshal(snap.Provenance, &prov); err != nil {
		t.Fatalf("unmarshal provenance: %v", err)
	}
	if prov.Core.Status != "ok" {
		t.Errorf("provenance status = %q, want ok", prov.Core.Status)
	}
	if len(prov.Core.Inputs) != 2 || prov.Core.Inputs[0] != "alert_group" || prov.Core.Inputs[1] != "alert_events" {
		t.Errorf("provenance inputs = %v, want [alert_group, alert_events]", prov.Core.Inputs)
	}
}

func TestBuildSnapshot_PayloadJSONDeserializable(t *testing.T) {
	in := validInput()
	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var p snapshotPayload
	if err := json.Unmarshal(snap.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.SchemaVersion != "m1.evidence_snapshot.v1" {
		t.Errorf("schema_version = %q", p.SchemaVersion)
	}
	if p.Group.ID != int64(in.Group.ID) {
		t.Errorf("group.id = %d, want %d", p.Group.ID, in.Group.ID)
	}
	if p.Group.GroupKey != in.Group.GroupKey {
		t.Errorf("group.group_key = %q", p.Group.GroupKey)
	}
	if p.Group.Severity != string(in.Group.Severity) {
		t.Errorf("group.severity = %q", p.Group.Severity)
	}
	if p.Group.EventCount != in.Group.EventCount {
		t.Errorf("group.event_count = %d", p.Group.EventCount)
	}
	if len(p.Events) != 2 {
		t.Fatalf("events count = %d, want 2", len(p.Events))
	}
	if p.Events[0].Source != "prometheus" {
		t.Errorf("events[0].source = %q", p.Events[0].Source)
	}
}

func TestBuildSnapshot_CreatedByWorkflowEmpty(t *testing.T) {
	in := validInput()
	in.CreatedByWorkflow = ""
	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.CreatedByWorkflow != "" {
		t.Errorf("CreatedByWorkflow = %q, want empty", snap.CreatedByWorkflow)
	}
}

func TestBuildSnapshot_CreatedByWorkflowNotInDigest(t *testing.T) {
	in1 := validInput()
	in1.CreatedByWorkflow = "workflow-A"
	snap1, err := BuildSnapshot(in1)
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}

	in2 := validInput()
	in2.CreatedByWorkflow = "workflow-B"
	snap2, err := BuildSnapshot(in2)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}

	if snap1.Digest != snap2.Digest {
		t.Errorf("digest should be identical regardless of CreatedByWorkflow:\n  d1=%s\n  d2=%s", snap1.Digest, snap2.Digest)
	}
	if string(snap1.Payload) != string(snap2.Payload) {
		t.Error("payload should be identical regardless of CreatedByWorkflow")
	}
	if snap1.CreatedByWorkflow == snap2.CreatedByWorkflow {
		t.Error("snapshot CreatedByWorkflow field should differ")
	}
}

func TestBuildSnapshot_RawPayloadLiteralNullSameAsNil(t *testing.T) {
	in1 := validInput()
	in1.Events[0].RawPayload = nil
	in1.Events[1].RawPayload = nil

	in2 := validInput()
	in2.Events[0].RawPayload = json.RawMessage(`null`)
	in2.Events[1].RawPayload = json.RawMessage(`null`)

	snap1, err := BuildSnapshot(in1)
	if err != nil {
		t.Fatalf("nil run: %v", err)
	}
	snap2, err := BuildSnapshot(in2)
	if err != nil {
		t.Fatalf("literal null run: %v", err)
	}

	if snap1.Digest != snap2.Digest {
		t.Errorf("digest should match for nil vs literal null raw_payload:\n  nil=%s\n  null=%s", snap1.Digest, snap2.Digest)
	}
}

func TestBuildSnapshot_LargeIntegerPreservedInRawPayload(t *testing.T) {
	// 9007199254740993 = 2^53 + 1, exceeds float64 exact range.
	largeInt := `{"id":9007199254740993,"nested":{"big":9007199254740994}}`

	in := validInput()
	in.Events[0].RawPayload = json.RawMessage(largeInt)

	snap, err := BuildSnapshot(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the large integers are preserved in the payload.
	var p snapshotPayload
	if err := json.Unmarshal(snap.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	raw := string(*p.Events[0].RawPayload)
	// The exact integer should be preserved, not converted to float.
	if !contains(raw, "9007199254740993") {
		t.Errorf("large integer lost precision in raw_payload: %s", raw)
	}
	if !contains(raw, "9007199254740994") {
		t.Errorf("nested large integer lost precision in raw_payload: %s", raw)
	}

	// Same payload with identical semantics should produce the same digest.
	in2 := validInput()
	in2.Events[0].RawPayload = json.RawMessage(`{ "nested" : {"big":9007199254740994} , "id" : 9007199254740993 }`)
	snap2, err := BuildSnapshot(in2)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if snap.Digest != snap2.Digest {
		t.Errorf("large integer payload with different key order should produce same digest:\n  d1=%s\n  d2=%s", snap.Digest, snap2.Digest)
	}
}

func TestBuildSnapshot_InvalidEventStatus(t *testing.T) {
	in := validInput()
	in.Events[0].Status = "bogus"
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_EndsAtBeforeStartsAt(t *testing.T) {
	in := validInput()
	// Status must be resolved so EndsAt is allowed by the cross-invariant; we want to
	// exercise the EndsAt-before-StartsAt branch specifically.
	in.Events[0].Status = domain.AlertStatusResolved
	badEnd := in.Events[0].StartsAt.Add(-1 * time.Minute)
	in.Events[0].EndsAt = &badEnd
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_EndsAtZero(t *testing.T) {
	in := validInput()
	// Same rationale as above: keep Status consistent with EndsAt set so we hit the
	// zero-EndsAt branch instead of the firing/EndsAt cross-invariant.
	in.Events[0].Status = domain.AlertStatusResolved
	zero := time.Time{}
	in.Events[0].EndsAt = &zero
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
}

func TestBuildSnapshot_FiringEventWithEndsAtReturnsInvariantViolation(t *testing.T) {
	in := validInput()
	// Status=firing must be paired with EndsAt=nil per internal/domain/doc.go.
	in.Events[0].Status = domain.AlertStatusFiring
	end := in.Events[0].StartsAt.Add(1 * time.Minute)
	in.Events[0].EndsAt = &end
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
	if !contains(err.Error(), "status=firing but ends_at is set") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBuildSnapshot_ResolvedEventWithoutEndsAtReturnsInvariantViolation(t *testing.T) {
	in := validInput()
	// Status=resolved must be paired with EndsAt!=nil per internal/domain/doc.go.
	in.Events[0].Status = domain.AlertStatusResolved
	in.Events[0].EndsAt = nil
	_, err := BuildSnapshot(in)
	mustBeInvariantViolation(t, err)
	if !contains(err.Error(), "status=resolved but ends_at is nil") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// contains is a simple substring check helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
