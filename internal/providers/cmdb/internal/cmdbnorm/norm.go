// Package cmdbnorm contains shared validation and defensive-copy helpers for
// CMDB provider implementations.
package cmdbnorm

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// MaxMatchLabels caps static CMDB selector label counts.
	MaxMatchLabels = 32

	// MaxStringBytes caps every provider-neutral CMDB string field.
	MaxStringBytes = 256

	// MaxAttributePairs caps sanitized CMDB resource attributes.
	MaxAttributePairs = 64
)

// NormalizeResource validates and copies one provider-neutral CMDB resource.
func NormalizeResource(resource ports.CMDBResource) (ports.CMDBResource, error) {
	id, err := NormalizeRequiredString("resource id", resource.ID)
	if err != nil {
		return ports.CMDBResource{}, err
	}
	kind, err := NormalizeRequiredString("resource kind", resource.Kind)
	if err != nil {
		return ports.CMDBResource{}, err
	}
	name, err := NormalizeRequiredString("resource name", resource.Name)
	if err != nil {
		return ports.CMDBResource{}, err
	}
	owners, err := normalizeOwners(resource.Owners)
	if err != nil {
		return ports.CMDBResource{}, err
	}
	topology, err := normalizeTopology(resource.Topology)
	if err != nil {
		return ports.CMDBResource{}, err
	}
	attributes, err := normalizeAttributes(resource.Attributes)
	if err != nil {
		return ports.CMDBResource{}, err
	}
	if len(owners) == 0 && len(topology) == 0 && len(attributes) == 0 {
		return ports.CMDBResource{}, fmt.Errorf("resource must include owners, topology, or attributes")
	}
	return ports.CMDBResource{
		ID:         id,
		Kind:       kind,
		Name:       name,
		Owners:     owners,
		Topology:   topology,
		Attributes: attributes,
	}, nil
}

// NormalizeLabelMap validates and copies alert-label selector maps.
func NormalizeLabelMap(in map[string]string, requireNonEmpty bool) (map[string]string, error) {
	if len(in) == 0 {
		if requireNonEmpty {
			return nil, fmt.Errorf("match_labels must be non-empty")
		}
		return nil, nil
	}
	if len(in) > MaxMatchLabels {
		return nil, fmt.Errorf("label map exceeds %d entries", MaxMatchLabels)
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		k, err := NormalizeRequiredString("label key", key)
		if err != nil {
			return nil, err
		}
		v, err := NormalizeRequiredString("label value", value)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// NormalizeRequiredString validates a bounded non-empty string field.
func NormalizeRequiredString(field, value string) (string, error) {
	value, err := NormalizeOptionalString(field, value)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s must be non-empty", field)
	}
	return value, nil
}

// NormalizeOptionalString validates a bounded string field while allowing empty
// values.
func NormalizeOptionalString(field, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed != value {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if len(value) > MaxStringBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", field, MaxStringBytes)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%s must not contain control characters", field)
		}
	}
	return value, nil
}

// CloneResource returns a deep-enough copy for provider DTOs.
func CloneResource(in ports.CMDBResource) ports.CMDBResource {
	return ports.CMDBResource{
		ID:         in.ID,
		Kind:       in.Kind,
		Name:       in.Name,
		Owners:     append([]ports.CMDBOwner(nil), in.Owners...),
		Topology:   append([]ports.CMDBTopologyLink(nil), in.Topology...),
		Attributes: CloneStringMap(in.Attributes),
	}
}

// CloneStringMap returns a defensive copy of in.
func CloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeOwners(in []ports.CMDBOwner) ([]ports.CMDBOwner, error) {
	out := make([]ports.CMDBOwner, len(in))
	for i, owner := range in {
		subject, err := NormalizeOptionalString("owner subject", owner.Subject)
		if err != nil {
			return nil, fmt.Errorf("owner[%d]: %w", i, err)
		}
		team, err := NormalizeOptionalString("owner team", owner.Team)
		if err != nil {
			return nil, fmt.Errorf("owner[%d]: %w", i, err)
		}
		role, err := NormalizeOptionalString("owner role", owner.Role)
		if err != nil {
			return nil, fmt.Errorf("owner[%d]: %w", i, err)
		}
		if subject == "" && team == "" {
			return nil, fmt.Errorf("owner[%d]: subject or team must be non-empty", i)
		}
		out[i] = ports.CMDBOwner{Subject: subject, Team: team, Role: role}
	}
	return out, nil
}

func normalizeTopology(in []ports.CMDBTopologyLink) ([]ports.CMDBTopologyLink, error) {
	out := make([]ports.CMDBTopologyLink, len(in))
	for i, link := range in {
		relation, err := NormalizeRequiredString("topology relation", link.Relation)
		if err != nil {
			return nil, fmt.Errorf("topology[%d]: %w", i, err)
		}
		targetID, err := NormalizeRequiredString("topology target id", link.TargetID)
		if err != nil {
			return nil, fmt.Errorf("topology[%d]: %w", i, err)
		}
		targetKind, err := NormalizeRequiredString("topology target kind", link.TargetKind)
		if err != nil {
			return nil, fmt.Errorf("topology[%d]: %w", i, err)
		}
		targetName, err := NormalizeOptionalString("topology target name", link.TargetName)
		if err != nil {
			return nil, fmt.Errorf("topology[%d]: %w", i, err)
		}
		out[i] = ports.CMDBTopologyLink{
			Relation:   relation,
			TargetID:   targetID,
			TargetKind: targetKind,
			TargetName: targetName,
		}
	}
	return out, nil
}

func normalizeAttributes(in map[string]string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > MaxAttributePairs {
		return nil, fmt.Errorf("attributes exceed %d entries", MaxAttributePairs)
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		k, err := NormalizeRequiredString("attribute key", key)
		if err != nil {
			return nil, err
		}
		v, err := NormalizeRequiredString("attribute value", value)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}
