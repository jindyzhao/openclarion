// Package evidencebuild provides a deterministic builder that packs an
// AlertGroup and its AlertEvents into a single EvidenceSnapshot. The
// builder is a pure function -- it does not call any provider, does not
// perform persistence, and does not embed the current wall-clock time.
// The resulting snapshot payload and digest are fully reproducible given
// the same semantic inputs regardless of field ordering or whitespace.
package evidencebuild

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
)

// Input holds the data required to build an EvidenceSnapshot.
type Input struct {
	Group             domain.AlertGroup
	Events            []domain.AlertEvent
	CreatedByWorkflow string // optional; empty allowed
}

// BuildSnapshot constructs a deterministic EvidenceSnapshot from the
// given Input. Validation failures return domain.ErrInvariantViolation.
// The payload JSON is canonical: nested json.RawMessage fields are
// re-serialised, maps are key-sorted by encoding/json, and all times
// are normalised via domain.NormalizeUTCMicro before formatting.
func BuildSnapshot(in Input) (domain.EvidenceSnapshot, error) {
	if err := validateInput(in); err != nil {
		return domain.EvidenceSnapshot{}, err
	}

	payload, err := buildPayload(in.Group, in.Events)
	if err != nil {
		return domain.EvidenceSnapshot{}, fmt.Errorf("evidence build: payload construction: %w", err)
	}

	digest := computeDigest(payload)
	provenance := buildProvenance()

	return domain.NewEvidenceSnapshot(
		in.Group.ID,
		digest,
		payload,
		provenance,
		domain.SnapshotStatusComplete,
		nil,
		in.CreatedByWorkflow,
	)
}

// --------------- validation ---------------

func validateInput(in Input) error {
	g := in.Group

	if g.ID == 0 {
		return fmt.Errorf("evidence build: group ID must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if g.GroupKey == "" {
		return fmt.Errorf("evidence build: group_key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if g.FirstSeenAt.IsZero() {
		return fmt.Errorf("evidence build: group first_seen_at must be set: %w", domain.ErrInvariantViolation)
	}
	if g.LastSeenAt.IsZero() {
		return fmt.Errorf("evidence build: group last_seen_at must be set: %w", domain.ErrInvariantViolation)
	}
	if !g.Severity.Valid() {
		return fmt.Errorf("evidence build: group severity %q is invalid: %w", g.Severity, domain.ErrInvariantViolation)
	}
	if err := validateDimensions(g.Dimensions); err != nil {
		return err
	}

	first := domain.NormalizeUTCMicro(g.FirstSeenAt)
	last := domain.NormalizeUTCMicro(g.LastSeenAt)
	if last.Before(first) {
		return fmt.Errorf("evidence build: group last_seen_at %s precedes first_seen_at %s: %w", last, first, domain.ErrInvariantViolation)
	}

	if g.EventCount != len(in.Events) {
		return fmt.Errorf("evidence build: group event_count %d != len(events) %d: %w", g.EventCount, len(in.Events), domain.ErrInvariantViolation)
	}
	if len(in.Events) == 0 {
		return fmt.Errorf("evidence build: events must be non-empty: %w", domain.ErrInvariantViolation)
	}

	eventIDSet := make(map[domain.AlertEventID]struct{}, len(in.Events))
	for i := range in.Events {
		e := &in.Events[i]
		if e.ID == 0 {
			return fmt.Errorf("evidence build: event[%d] ID must be non-zero: %w", i, domain.ErrInvariantViolation)
		}
		if e.StartsAt.IsZero() {
			return fmt.Errorf("evidence build: event[%d] starts_at must be set: %w", i, domain.ErrInvariantViolation)
		}
		if e.Source == "" {
			return fmt.Errorf("evidence build: event[%d] source must be non-empty: %w", i, domain.ErrInvariantViolation)
		}
		if e.CanonicalFingerprint == "" {
			return fmt.Errorf("evidence build: event[%d] canonical_fingerprint must be non-empty: %w", i, domain.ErrInvariantViolation)
		}
		if e.SourceFingerprint == "" {
			return fmt.Errorf("evidence build: event[%d] source_fingerprint must be non-empty: %w", i, domain.ErrInvariantViolation)
		}
		if !e.Status.Valid() {
			return fmt.Errorf("evidence build: event[%d] status %q is invalid: %w", i, e.Status, domain.ErrInvariantViolation)
		}
		if len(e.RawPayload) > 0 {
			if err := validateStrictJSONRawMessage(e.RawPayload); err != nil {
				return fmt.Errorf("evidence build: event[%d] raw_payload is not strict JSON: %w: %w", i, err, domain.ErrInvariantViolation)
			}
		}
		// Cross-invariant: Status and EndsAt must agree (see internal/domain/doc.go).
		// AlertEvent.EndsAt is non-nil iff Status == AlertStatusResolved.
		switch e.Status {
		case domain.AlertStatusFiring:
			if e.EndsAt != nil {
				return fmt.Errorf("evidence build: event[%d] status=firing but ends_at is set: %w", i, domain.ErrInvariantViolation)
			}
		case domain.AlertStatusResolved:
			if e.EndsAt == nil {
				return fmt.Errorf("evidence build: event[%d] status=resolved but ends_at is nil: %w", i, domain.ErrInvariantViolation)
			}
		}
		if e.EndsAt != nil {
			nEnd := domain.NormalizeUTCMicro(*e.EndsAt)
			if nEnd.IsZero() {
				return fmt.Errorf("evidence build: event[%d] ends_at must be non-zero when set: %w", i, domain.ErrInvariantViolation)
			}
			nStart := domain.NormalizeUTCMicro(e.StartsAt)
			if nEnd.Before(nStart) {
				return fmt.Errorf("evidence build: event[%d] ends_at %s precedes starts_at %s: %w", i, nEnd, nStart, domain.ErrInvariantViolation)
			}
		}

		starts := domain.NormalizeUTCMicro(e.StartsAt)
		if starts.Before(first) || starts.After(last) {
			return fmt.Errorf("evidence build: event[%d] starts_at %s outside group range [%s, %s]: %w", i, starts, first, last, domain.ErrInvariantViolation)
		}

		if _, dup := eventIDSet[e.ID]; dup {
			return fmt.Errorf("evidence build: duplicate event ID %d: %w", e.ID, domain.ErrInvariantViolation)
		}
		eventIDSet[e.ID] = struct{}{}
	}

	// Group.EventIDs cross-check (optional: only when non-nil and non-empty).
	if len(g.EventIDs) > 0 {
		groupIDSet := make(map[domain.AlertEventID]struct{}, len(g.EventIDs))
		for _, id := range g.EventIDs {
			if _, dup := groupIDSet[id]; dup {
				return fmt.Errorf("evidence build: duplicate ID %d in group.EventIDs: %w", id, domain.ErrInvariantViolation)
			}
			groupIDSet[id] = struct{}{}
		}
		if len(groupIDSet) != len(eventIDSet) {
			return fmt.Errorf("evidence build: group.EventIDs set size %d != events size %d: %w", len(groupIDSet), len(eventIDSet), domain.ErrInvariantViolation)
		}
		for id := range groupIDSet {
			if _, ok := eventIDSet[id]; !ok {
				return fmt.Errorf("evidence build: group.EventIDs contains %d not found in events: %w", id, domain.ErrInvariantViolation)
			}
		}
	}

	return nil
}

func validateDimensions(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("evidence build: group dimensions must be non-empty JSON object: %w", domain.ErrInvariantViolation)
	}
	var obj map[string]interface{}
	if err := decodeStrictJSONUseNumber(raw, &obj); err != nil {
		return fmt.Errorf("evidence build: group dimensions is not strict JSON: %w: %w", err, domain.ErrInvariantViolation)
	}
	if obj == nil {
		return fmt.Errorf("evidence build: group dimensions must be a JSON object: %w", domain.ErrInvariantViolation)
	}
	return nil
}

// --------------- payload construction ---------------

// snapshotPayload is the typed struct whose field declaration order
// determines the JSON key order in the marshalled payload.
type snapshotPayload struct {
	SchemaVersion string         `json:"schema_version"`
	Group         payloadGroup   `json:"group"`
	Events        []payloadEvent `json:"events"`
}

type payloadGroup struct {
	ID          int64           `json:"id"`
	GroupKey    string          `json:"group_key"`
	Dimensions  json.RawMessage `json:"dimensions"`
	Severity    string          `json:"severity"`
	EventCount  int             `json:"event_count"`
	FirstSeenAt string          `json:"first_seen_at"`
	LastSeenAt  string          `json:"last_seen_at"`
}

type payloadEvent struct {
	ID                   int64             `json:"id"`
	Source               string            `json:"source"`
	AlertSourceProfileID int64             `json:"alert_source_profile_id,omitempty"`
	SourceFingerprint    string            `json:"source_fingerprint"`
	CanonicalFingerprint string            `json:"canonical_fingerprint"`
	Labels               map[string]string `json:"labels"`
	Annotations          map[string]string `json:"annotations"`
	Status               string            `json:"status"`
	StartsAt             string            `json:"starts_at"`
	EndsAt               *string           `json:"ends_at"`
	RawPayload           *json.RawMessage  `json:"raw_payload"`
}

const timeFormat = "2006-01-02T15:04:05.999999Z07:00"

func buildPayload(group domain.AlertGroup, events []domain.AlertEvent) ([]byte, error) {
	// Sort events deterministically: (StartsAt asc, ID asc).
	sorted := make([]domain.AlertEvent, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		si := domain.NormalizeUTCMicro(sorted[i].StartsAt)
		sj := domain.NormalizeUTCMicro(sorted[j].StartsAt)
		if !si.Equal(sj) {
			return si.Before(sj)
		}
		return sorted[i].ID < sorted[j].ID
	})

	// Canonicalize group dimensions.
	dimCanon, err := canonicalizeJSON(group.Dimensions)
	if err != nil {
		return nil, fmt.Errorf("dimensions canonicalize: %w", err)
	}

	first := domain.NormalizeUTCMicro(group.FirstSeenAt)
	last := domain.NormalizeUTCMicro(group.LastSeenAt)

	pg := payloadGroup{
		ID:          int64(group.ID),
		GroupKey:    group.GroupKey,
		Dimensions:  dimCanon,
		Severity:    string(group.Severity),
		EventCount:  group.EventCount,
		FirstSeenAt: first.Format(timeFormat),
		LastSeenAt:  last.Format(timeFormat),
	}

	pe := make([]payloadEvent, 0, len(sorted))
	for i := range sorted {
		ev := &sorted[i]
		starts := domain.NormalizeUTCMicro(ev.StartsAt)

		var endsAt *string
		if ev.EndsAt != nil {
			s := domain.NormalizeUTCMicro(*ev.EndsAt).Format(timeFormat)
			endsAt = &s
		}

		var rawPtr *json.RawMessage
		if len(ev.RawPayload) > 0 {
			canon, cErr := canonicalizeJSON(ev.RawPayload)
			if cErr != nil {
				return nil, fmt.Errorf("event[%d] raw_payload canonicalize: %w", i, cErr)
			}
			rawPtr = &canon
		}
		// nil/empty RawPayload -> nil pointer -> JSON null

		pe = append(pe, payloadEvent{
			ID:                   int64(ev.ID),
			Source:               ev.Source,
			AlertSourceProfileID: int64(ev.AlertSourceProfileID),
			SourceFingerprint:    ev.SourceFingerprint,
			CanonicalFingerprint: ev.CanonicalFingerprint,
			Labels:               cloneStringMap(ev.Labels),
			Annotations:          cloneStringMap(ev.Annotations),
			Status:               string(ev.Status),
			StartsAt:             starts.Format(timeFormat),
			EndsAt:               endsAt,
			RawPayload:           rawPtr,
		})
	}

	p := snapshotPayload{
		SchemaVersion: "m1.evidence_snapshot.v1",
		Group:         pg,
		Events:        pe,
	}

	return json.Marshal(p)
}

// --------------- helpers ---------------

func computeDigest(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

// provenancePayload is typed so that key order is stable by declaration.
type provenancePayload struct {
	Core provenanceCore `json:"openclarion.core"`
}

type provenanceCore struct {
	Status string   `json:"status"`
	Inputs []string `json:"inputs"`
}

func buildProvenance() json.RawMessage {
	p := provenancePayload{
		Core: provenanceCore{
			Status: "ok",
			Inputs: []string{"alert_group", "alert_events"},
		},
	}
	b, _ := json.Marshal(p) // typed struct -- cannot fail
	return b
}

// canonicalizeJSON re-serialises arbitrary JSON so that object keys are
// sorted and whitespace is normalised. This makes semantically
// equivalent JSON documents produce identical byte sequences.
// It uses json.Decoder.UseNumber() to preserve integer precision
// (avoids float64 lossy conversion for large integers like 2^53+1).
func canonicalizeJSON(raw json.RawMessage) (json.RawMessage, error) {
	var v interface{}
	if err := decodeStrictJSONUseNumber(raw, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func validateStrictJSONRawMessage(raw json.RawMessage) error {
	var v any
	return decodeStrictJSONUseNumber(raw, &v)
}

func decodeStrictJSONUseNumber(raw json.RawMessage, dst any) error {
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(dst)
}

// cloneStringMap returns a shallow copy of in. If in is nil, an empty
// non-nil map is returned so that JSON marshalling produces {} instead
// of null.
func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
