// Package alertgrouping implements the deterministic grouping algorithm
// for Stage S1 of the OpenClarion diagnosis pipeline.
//
// GroupEvents is a pure function: given a slice of persisted AlertEvents
// and a grouping configuration, it produces a deterministic set of
// AlertGroup drafts suitable for persistence. The algorithm is
// replay-deterministic: the same input window always produces the same
// output regardless of event ordering within the slice.
//
// This package has zero external dependencies beyond the Go standard
// library and the internal domain package.
package alertgrouping

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
)

// Config holds the grouping configuration. DimensionKeys selects
// which alert labels participate in the group key computation;
// SeverityKey identifies the label whose value is mapped to
// domain.GroupSeverity for max-severity aggregation.
type Config struct {
	DimensionKeys []string // label keys for grouping; MUST be non-empty after trim
	SeverityKey   string   // label key whose value maps to GroupSeverity; MUST be non-empty after trim
}

// DefaultConfig returns the recommended default grouping configuration.
// It groups alerts by "alertname" and reads severity from the
// "severity" label.
func DefaultConfig() Config {
	return Config{
		DimensionKeys: []string{"alertname"},
		SeverityKey:   "severity",
	}
}

// normalizedConfig is the validated, canonical form of Config used
// internally by GroupEvents. It is not exported because callers
// interact only with Config.
type normalizedConfig struct {
	dimensionKeys []string // trimmed, deduped, sorted
	severityKey   string   // trimmed
}

// validateConfig trims, deduplicates, and sorts the dimension keys,
// and validates that both DimensionKeys and SeverityKey are non-empty
// after trimming. Returns a normalizedConfig on success.
func validateConfig(cfg Config) (normalizedConfig, error) {
	// Trim and collect non-empty keys.
	seen := make(map[string]struct{}, len(cfg.DimensionKeys))
	var keys []string
	for _, k := range cfg.DimensionKeys {
		trimmed := strings.TrimSpace(k)
		if trimmed == "" {
			return normalizedConfig{}, fmt.Errorf(
				"alertgrouping: dimension key %q is blank after trim: %w",
				k, domain.ErrInvariantViolation,
			)
		}
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		keys = append(keys, trimmed)
	}
	if len(keys) == 0 {
		return normalizedConfig{}, fmt.Errorf(
			"alertgrouping: DimensionKeys must be non-empty: %w",
			domain.ErrInvariantViolation,
		)
	}
	slices.Sort(keys)

	sevKey := strings.TrimSpace(cfg.SeverityKey)
	if sevKey == "" {
		return normalizedConfig{}, fmt.Errorf(
			"alertgrouping: SeverityKey %q is blank after trim: %w",
			cfg.SeverityKey, domain.ErrInvariantViolation,
		)
	}

	return normalizedConfig{
		dimensionKeys: keys,
		severityKey:   sevKey,
	}, nil
}

// GroupEvents performs deterministic grouping on a slice of persisted
// AlertEvents. The algorithm:
//
//  1. Returns (nil, nil) for empty input (config is NOT validated).
//  2. Validates config via validateConfig.
//  3. Validates each event: ID must be non-zero, StartsAt must be
//     non-zero.
//  4. Buckets events by canonical dimensions JSON (NOT sha256).
//  5. Builds an AlertGroup draft per bucket.
//  6. Sorts output by (FirstSeenAt asc, GroupKey asc).
//
// The output is fully deterministic regardless of input order.
func GroupEvents(events []domain.AlertEvent, cfg Config) ([]domain.AlertGroup, error) {
	// Empty input fast path -- intentionally skips config validation.
	if len(events) == 0 {
		return nil, nil
	}

	ncfg, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Validate input events.
	for i := range events {
		if events[i].ID == 0 {
			return nil, fmt.Errorf(
				"alertgrouping: event at index %d has zero ID (input must be persisted): %w",
				i, domain.ErrInvariantViolation,
			)
		}
		if events[i].StartsAt.IsZero() {
			return nil, fmt.Errorf(
				"alertgrouping: event at index %d has zero StartsAt: %w",
				i, domain.ErrInvariantViolation,
			)
		}
	}

	// Bucket events by canonical dimensions JSON.
	// Key = canonical JSON string (not sha256) to avoid hash collision.
	type bucket struct {
		dimsJSON []byte
		events   []domain.AlertEvent
	}
	buckets := make(map[string]*bucket)
	// Preserve insertion order for deterministic iteration
	var bucketOrder []string

	for i := range events {
		dims := dimensionSubset(events[i].Labels, ncfg.dimensionKeys)
		dj := canonicalDimensionsJSON(dims)
		key := string(dj)
		if b, ok := buckets[key]; ok {
			b.events = append(b.events, events[i])
		} else {
			buckets[key] = &bucket{
				dimsJSON: dj,
				events:   []domain.AlertEvent{events[i]},
			}
			bucketOrder = append(bucketOrder, key)
		}
	}

	// Build group drafts.
	groups := make([]domain.AlertGroup, 0, len(buckets))
	for _, key := range bucketOrder {
		b := buckets[key]

		gk := sha256Hex(b.dimsJSON)
		dimensions := json.RawMessage(append([]byte(nil), b.dimsJSON...))

		// Compute max severity via binary helper in loop.
		groupSeverity := domain.GroupSeverityUnknown
		for _, e := range b.events {
			groupSeverity = maxSeverity(groupSeverity, extractSeverity(e.Labels, ncfg.severityKey))
		}

		// Compute FirstSeenAt / LastSeenAt.
		firstSeen := b.events[0].StartsAt
		lastSeen := b.events[0].StartsAt
		for _, e := range b.events[1:] {
			if e.StartsAt.Before(firstSeen) {
				firstSeen = e.StartsAt
			}
			if e.StartsAt.After(lastSeen) {
				lastSeen = e.StartsAt
			}
		}

		// Build sorted EventIDs: (StartsAt asc, ID asc).
		sort.Slice(b.events, func(i, j int) bool {
			if b.events[i].StartsAt.Equal(b.events[j].StartsAt) {
				return b.events[i].ID < b.events[j].ID
			}
			return b.events[i].StartsAt.Before(b.events[j].StartsAt)
		})
		eventIDs := make([]domain.AlertEventID, len(b.events))
		for i, e := range b.events {
			eventIDs[i] = e.ID
		}

		g, err := domain.NewAlertGroup(
			gk,
			dimensions,
			groupSeverity,
			len(b.events),
			firstSeen,
			lastSeen,
			eventIDs,
		)
		if err != nil {
			return nil, fmt.Errorf("alertgrouping: building group for key %s: %w", gk, err)
		}
		groups = append(groups, g)
	}

	// Sort output: (FirstSeenAt asc, GroupKey asc).
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].FirstSeenAt.Equal(groups[j].FirstSeenAt) {
			return groups[i].GroupKey < groups[j].GroupKey
		}
		return groups[i].FirstSeenAt.Before(groups[j].FirstSeenAt)
	})

	return groups, nil
}

// --- private helpers ---

// dimensionSubset extracts the values for each configured dimension
// key from the event labels. Missing keys are preserved as empty
// strings to guarantee a stable JSON structure across all groups.
func dimensionSubset(labels map[string]string, keys []string) map[string]string {
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		m[k] = labels[k] // missing key -> zero value ""
	}
	return m
}

// canonicalDimensionsJSON serialises dims as the byte-stable input to
// the group key hash. The nil -> {} guard mirrors alertingest's
// canonicalLabelsJSON for consistency, although dimensionSubset always
// returns a non-nil map.
func canonicalDimensionsJSON(dims map[string]string) []byte {
	if dims == nil {
		dims = map[string]string{}
	}
	b, err := json.Marshal(dims)
	if err != nil {
		// map[string]string cannot fail to marshal; defensive panic.
		panic(fmt.Sprintf("alertgrouping: canonical dimensions marshal: %v", err))
	}
	return b
}

// sha256Hex returns the lowercase hex-encoded SHA-256 digest of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// severityRank maps GroupSeverity values to ordinal ranks for
// comparison. Higher rank wins in maxSeverity.
var severityRank = map[domain.GroupSeverity]int{
	domain.GroupSeverityUnknown:  0,
	domain.GroupSeverityInfo:     1,
	domain.GroupSeverityWarning:  2,
	domain.GroupSeverityCritical: 3,
}

// extractSeverity reads the severity label from the event, trims and
// lower-cases it, and maps it to a domain.GroupSeverity. Unrecognised
// or missing values fall back to GroupSeverityUnknown.
func extractSeverity(labels map[string]string, key string) domain.GroupSeverity {
	raw, ok := labels[key]
	if !ok {
		return domain.GroupSeverityUnknown
	}
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "critical":
		return domain.GroupSeverityCritical
	case "warning":
		return domain.GroupSeverityWarning
	case "info":
		return domain.GroupSeverityInfo
	case "unknown":
		return domain.GroupSeverityUnknown
	default:
		return domain.GroupSeverityUnknown
	}
}

// maxSeverity returns the higher of two GroupSeverity values based on
// severityRank ordering.
func maxSeverity(a, b domain.GroupSeverity) domain.GroupSeverity {
	if severityRank[b] > severityRank[a] {
		return b
	}
	return a
}
