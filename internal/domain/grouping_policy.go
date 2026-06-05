package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	maxGroupingPolicyNameLen       = 120
	maxGroupingPolicyDimensionKeys = 16
	maxGroupingPolicyKeyLen        = 64
	maxGroupingPolicySourceFilters = 16
	maxGroupingPolicySourceLen     = 64
)

// GroupingPolicy is operator-managed grouping configuration. It is stored as
// business configuration and can be dry-run previewed against persisted alert
// samples before any workflow policy binds to it.
type GroupingPolicy struct {
	ID            GroupingPolicyID
	Name          string
	DimensionKeys []string
	SeverityKey   string
	SourceFilter  []string
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewGroupingPolicy constructs a validated grouping policy. Dimension keys and
// source filters are trimmed, deduplicated, and sorted for stable persistence.
// An empty source filter means all alert sources.
func NewGroupingPolicy(
	name string,
	dimensionKeys []string,
	severityKey string,
	sourceFilter []string,
	enabled bool,
) (GroupingPolicy, error) {
	name = strings.TrimSpace(name)
	severityKey = strings.TrimSpace(severityKey)
	if name == "" {
		return GroupingPolicy{}, fmt.Errorf("grouping policy: name must be non-empty: %w", ErrInvariantViolation)
	}
	if len(name) > maxGroupingPolicyNameLen {
		return GroupingPolicy{}, fmt.Errorf("grouping policy: name exceeds %d bytes: %w", maxGroupingPolicyNameLen, ErrInvariantViolation)
	}
	normalizedDimensions, err := normalizePolicyKeyList("dimension key", dimensionKeys, maxGroupingPolicyDimensionKeys)
	if err != nil {
		return GroupingPolicy{}, fmt.Errorf("grouping policy: %w", err)
	}
	if severityKey == "" {
		return GroupingPolicy{}, fmt.Errorf("grouping policy: severity_key must be non-empty: %w", ErrInvariantViolation)
	}
	if err := validatePolicyKey("severity_key", severityKey); err != nil {
		return GroupingPolicy{}, fmt.Errorf("grouping policy: %w", err)
	}
	normalizedSources, err := normalizeSourceFilter(sourceFilter)
	if err != nil {
		return GroupingPolicy{}, fmt.Errorf("grouping policy: %w", err)
	}
	return GroupingPolicy{
		Name:          name,
		DimensionKeys: normalizedDimensions,
		SeverityKey:   severityKey,
		SourceFilter:  normalizedSources,
		Enabled:       enabled,
	}, nil
}

func normalizePolicyKeyList(label string, values []string, maxEntries int) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%s list must be non-empty: %w", label, ErrInvariantViolation)
	}
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, fmt.Errorf("%s must be non-empty: %w", label, ErrInvariantViolation)
		}
		if err := validatePolicyKey(label, value); err != nil {
			return nil, err
		}
		seen[value] = struct{}{}
	}
	if len(seen) > maxEntries {
		return nil, fmt.Errorf("%s list exceeds %d entries: %w", label, maxEntries, ErrInvariantViolation)
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	slices.Sort(out)
	return out, nil
}

func validatePolicyKey(label, value string) error {
	if len(value) > maxGroupingPolicyKeyLen {
		return fmt.Errorf("%s exceeds %d bytes: %w", label, maxGroupingPolicyKeyLen, ErrInvariantViolation)
	}
	if containsControlOrSpace(value) {
		return fmt.Errorf("%s must not contain whitespace or control characters: %w", label, ErrInvariantViolation)
	}
	return nil
}

func normalizeSourceFilter(values []string) ([]string, error) {
	if len(values) == 0 {
		return []string{}, nil
	}
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, fmt.Errorf("source_filter value must be non-empty: %w", ErrInvariantViolation)
		}
		if len(value) > maxGroupingPolicySourceLen {
			return nil, fmt.Errorf("source_filter value exceeds %d bytes: %w", maxGroupingPolicySourceLen, ErrInvariantViolation)
		}
		if containsControlOrSpace(value) {
			return nil, fmt.Errorf("source_filter value must not contain whitespace or control characters: %w", ErrInvariantViolation)
		}
		seen[value] = struct{}{}
	}
	if len(seen) > maxGroupingPolicySourceFilters {
		return nil, fmt.Errorf("source_filter exceeds %d entries: %w", maxGroupingPolicySourceFilters, ErrInvariantViolation)
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	slices.Sort(out)
	return out, nil
}
