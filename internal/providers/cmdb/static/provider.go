// Package static provides a deterministic label-matched CMDBProvider for local
// deployments and tests.
package static

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"

	"github.com/openclarion/openclarion/internal/providers/cmdb/internal/cmdbnorm"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	maxRecords     = 1000
	maxMatchLabels = cmdbnorm.MaxMatchLabels
)

// Record maps a non-empty set of alert labels to one sanitized CMDB resource.
// MatchLabels are subset-matched against CMDBLookupRequest.Labels.
type Record struct {
	MatchLabels map[string]string
	Resource    ports.CMDBResource
}

// Provider is a deterministic static CMDBProvider.
type Provider struct {
	records []Record
}

var _ ports.CMDBProvider = (*Provider)(nil)

// NewProvider validates and copies records. Selectors that could match the
// same alert labels are rejected up front so lookup never has to choose between
// overlapping CMDB resources.
func NewProvider(records []Record) (*Provider, error) {
	if len(records) > maxRecords {
		return nil, fmt.Errorf("static cmdb: records exceed %d", maxRecords)
	}
	out := make([]Record, len(records))
	for i, record := range records {
		normalized, err := normalizeRecord(record)
		if err != nil {
			return nil, fmt.Errorf("static cmdb: record[%d]: %w", i, err)
		}
		out[i] = normalized
	}
	if err := rejectOverlappingSelectors(out); err != nil {
		return nil, err
	}
	return &Provider{records: out}, nil
}

// NewProviderFromYAML builds a Provider from a strict static CMDB YAML config.
func NewProviderFromYAML(raw []byte) (*Provider, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return NewProvider(nil)
	}
	cfg, err := parseConfigYAML(raw)
	if err != nil {
		return nil, err
	}
	records := make([]Record, len(cfg.Records))
	for i, record := range cfg.Records {
		records[i] = record.toRecord()
	}
	return NewProvider(records)
}

// LookupResource implements ports.CMDBProvider.
func (p *Provider) LookupResource(ctx context.Context, req ports.CMDBLookupRequest) (ports.CMDBLookupResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.CMDBLookupResult{}, err
	}
	if p == nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("static cmdb: provider is not configured")
	}
	labels := cmdbnorm.CloneStringMap(req.Labels)
	matched := -1
	for i := range p.records {
		if selectorMatches(p.records[i].MatchLabels, labels) {
			if matched != -1 {
				return ports.CMDBLookupResult{}, fmt.Errorf("static cmdb: multiple records matched lookup labels")
			}
			matched = i
		}
	}
	if matched == -1 {
		return ports.CMDBLookupResult{}, nil
	}
	return ports.CMDBLookupResult{
		Found:    true,
		Resource: cmdbnorm.CloneResource(p.records[matched].Resource),
	}, nil
}

func normalizeRecord(record Record) (Record, error) {
	labels, err := cmdbnorm.NormalizeLabelMap(record.MatchLabels, true)
	if err != nil {
		return Record{}, err
	}
	resource, err := cmdbnorm.NormalizeResource(record.Resource)
	if err != nil {
		return Record{}, err
	}
	return Record{MatchLabels: labels, Resource: resource}, nil
}

func rejectOverlappingSelectors(records []Record) error {
	for i := range records {
		for j := i + 1; j < len(records); j++ {
			if selectorsCanOverlap(records[i].MatchLabels, records[j].MatchLabels) {
				return fmt.Errorf("static cmdb: records %d and %d have overlapping match_labels", i, j)
			}
		}
	}
	return nil
}

func selectorsCanOverlap(a, b map[string]string) bool {
	for key, av := range a {
		if bv, ok := b[key]; ok && av != bv {
			return false
		}
	}
	return true
}

func selectorMatches(selector, labels map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

type yamlConfig struct {
	Records []yamlRecord `yaml:"records"`
}

type yamlRecord struct {
	MatchLabels map[string]string `yaml:"match_labels"`
	Resource    yamlResource      `yaml:"resource"`
}

type yamlResource struct {
	ID         string             `yaml:"id"`
	Kind       string             `yaml:"kind"`
	Name       string             `yaml:"name"`
	Owners     []yamlOwner        `yaml:"owners"`
	Topology   []yamlTopologyLink `yaml:"topology"`
	Attributes map[string]string  `yaml:"attributes"`
}

type yamlOwner struct {
	Subject string `yaml:"subject"`
	Team    string `yaml:"team"`
	Role    string `yaml:"role"`
}

type yamlTopologyLink struct {
	Relation   string `yaml:"relation"`
	TargetID   string `yaml:"target_id"`
	TargetKind string `yaml:"target_kind"`
	TargetName string `yaml:"target_name"`
}

func (r yamlRecord) toRecord() Record {
	return Record{
		MatchLabels: cmdbnorm.CloneStringMap(r.MatchLabels),
		Resource: ports.CMDBResource{
			ID:         r.Resource.ID,
			Kind:       r.Resource.Kind,
			Name:       r.Resource.Name,
			Owners:     yamlOwnersToPorts(r.Resource.Owners),
			Topology:   yamlTopologyToPorts(r.Resource.Topology),
			Attributes: cmdbnorm.CloneStringMap(r.Resource.Attributes),
		},
	}
}

func yamlOwnersToPorts(in []yamlOwner) []ports.CMDBOwner {
	out := make([]ports.CMDBOwner, len(in))
	for i, owner := range in {
		out[i] = ports.CMDBOwner{Subject: owner.Subject, Team: owner.Team, Role: owner.Role}
	}
	return out
}

func yamlTopologyToPorts(in []yamlTopologyLink) []ports.CMDBTopologyLink {
	out := make([]ports.CMDBTopologyLink, len(in))
	for i, link := range in {
		out[i] = ports.CMDBTopologyLink{
			Relation:   link.Relation,
			TargetID:   link.TargetID,
			TargetKind: link.TargetKind,
			TargetName: link.TargetName,
		}
	}
	return out
}

func parseConfigYAML(raw []byte) (yamlConfig, error) {
	var root yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&root); err != nil {
		return yamlConfig{}, fmt.Errorf("static cmdb: parse YAML: %w", err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return yamlConfig{}, fmt.Errorf("static cmdb: multiple YAML documents are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return yamlConfig{}, fmt.Errorf("static cmdb: parse trailing YAML: %w", err)
	}
	if err := rejectUnsafeYAMLNodes(&root, "$"); err != nil {
		return yamlConfig{}, fmt.Errorf("static cmdb: %w", err)
	}

	var cfg yamlConfig
	strict := yaml.NewDecoder(bytes.NewReader(raw))
	strict.KnownFields(true)
	if err := strict.Decode(&cfg); err != nil {
		return yamlConfig{}, fmt.Errorf("static cmdb: decode YAML: %w", err)
	}
	return cfg, nil
}

func rejectUnsafeYAMLNodes(node *yaml.Node, path string) error {
	if node == nil {
		return nil
	}
	if node.Anchor != "" {
		return fmt.Errorf("%s: YAML anchors are not allowed", path)
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return fmt.Errorf("%s: YAML document must have exactly one root node", path)
		}
		return rejectUnsafeYAMLNodes(node.Content[0], path)
	case yaml.MappingNode:
		seen := map[string]struct{}{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if err := rejectUnsafeYAMLNodes(key, fmt.Sprintf("%s.key[%d]", path, i/2)); err != nil {
				return err
			}
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("%s: YAML mapping keys must be scalar", path)
			}
			if key.ShortTag() == "!!merge" {
				return fmt.Errorf("%s: YAML merge keys are not allowed", path)
			}
			keyID := key.ShortTag() + "\x00" + key.Value
			if _, exists := seen[keyID]; exists {
				return fmt.Errorf("%s: duplicate YAML key %q", path, key.Value)
			}
			seen[keyID] = struct{}{}
			if err := rejectUnsafeYAMLNodes(value, pathForYAMLKey(path, key.Value)); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			if err := rejectUnsafeYAMLNodes(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		return fmt.Errorf("%s: YAML aliases are not allowed", path)
	}
	return nil
}

func pathForYAMLKey(path, key string) string {
	if key == "" {
		return path + "[\"\"]"
	}
	return path + "." + key
}
