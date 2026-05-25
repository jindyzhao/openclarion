package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNewEvidenceSnapshot(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"metric":"cpu","value":99}`)
	provenance := json.RawMessage(`{"prom":{"status":"ok"}}`)

	t.Run("happy path: complete with no missing fields", func(t *testing.T) {
		t.Parallel()
		s, err := NewEvidenceSnapshot(7, "digest-1", payload, provenance, SnapshotStatusComplete, nil, "wf-abc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.AlertGroupID != 7 || s.Digest != "digest-1" {
			t.Fatalf("snapshot fields not preserved: %+v", s)
		}
		if string(s.Payload) != string(payload) {
			t.Fatalf("payload not preserved")
		}
	})

	t.Run("partial with missing_fields is allowed", func(t *testing.T) {
		t.Parallel()
		_, err := NewEvidenceSnapshot(7, "digest-2", payload, provenance, SnapshotStatusPartial, []string{"topology.dependents"}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	cases := []struct {
		name          string
		groupID       AlertGroupID
		digest        string
		payload       json.RawMessage
		status        SnapshotStatus
		missingFields []string
	}{
		{name: "zero group id", digest: "d", payload: payload, status: SnapshotStatusComplete},
		{name: "empty digest", groupID: 1, payload: payload, status: SnapshotStatusComplete},
		{name: "empty payload", groupID: 1, digest: "d", status: SnapshotStatusComplete},
		{name: "unknown status", groupID: 1, digest: "d", payload: payload, status: SnapshotStatus("weird")},
		{name: "missing fields with status complete", groupID: 1, digest: "d", payload: payload, status: SnapshotStatusComplete, missingFields: []string{"x"}},
		{name: "missing fields with status failed", groupID: 1, digest: "d", payload: payload, status: SnapshotStatusFailed, missingFields: []string{"x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewEvidenceSnapshot(tc.groupID, tc.digest, tc.payload, nil, tc.status, tc.missingFields, "")
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}
