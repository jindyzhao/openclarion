package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestNewGroupingPolicyNormalizesKeysAndSources(t *testing.T) {
	policy, err := NewGroupingPolicy(
		" Default grouping ",
		[]string{"service", "alertname", "service"},
		" severity ",
		[]string{"prometheus", "alertmanager", "prometheus"},
		true,
	)
	if err != nil {
		t.Fatalf("NewGroupingPolicy: %v", err)
	}
	if policy.Name != "Default grouping" || policy.SeverityKey != "severity" || !policy.Enabled {
		t.Fatalf("policy = %+v", policy)
	}
	if !reflect.DeepEqual(policy.DimensionKeys, []string{"alertname", "service"}) {
		t.Fatalf("dimension keys = %v", policy.DimensionKeys)
	}
	if !reflect.DeepEqual(policy.SourceFilter, []string{"alertmanager", "prometheus"}) {
		t.Fatalf("source filter = %v", policy.SourceFilter)
	}
}

func TestNewGroupingPolicyRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name          string
		displayName   string
		dimensionKeys []string
		severityKey   string
		sourceFilter  []string
	}{
		{name: "blank name", displayName: " ", dimensionKeys: []string{"alertname"}, severityKey: "severity"},
		{name: "long name", displayName: strings.Repeat("x", 121), dimensionKeys: []string{"alertname"}, severityKey: "severity"},
		{name: "no dimensions", displayName: "Policy", dimensionKeys: nil, severityKey: "severity"},
		{name: "blank dimension", displayName: "Policy", dimensionKeys: []string{" "}, severityKey: "severity"},
		{name: "dimension whitespace", displayName: "Policy", dimensionKeys: []string{"alert name"}, severityKey: "severity"},
		{name: "blank severity", displayName: "Policy", dimensionKeys: []string{"alertname"}, severityKey: " "},
		{name: "severity whitespace", displayName: "Policy", dimensionKeys: []string{"alertname"}, severityKey: "severity level"},
		{name: "blank source", displayName: "Policy", dimensionKeys: []string{"alertname"}, severityKey: "severity", sourceFilter: []string{"prometheus", " "}},
		{name: "source whitespace", displayName: "Policy", dimensionKeys: []string{"alertname"}, severityKey: "severity", sourceFilter: []string{"prometheus east"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewGroupingPolicy(tc.displayName, tc.dimensionKeys, tc.severityKey, tc.sourceFilter, false)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}
