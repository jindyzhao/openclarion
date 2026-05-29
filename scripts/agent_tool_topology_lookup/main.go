// Command agent_tool_topology_lookup is a read-only static topology helper for
// sandbox runtime images. It loads a bounded JSON topology file and emits one
// service-centered JSON object to stdout.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	envTopologyFile      = "OPENCLARION_TOOL_TOPOLOGY_FILE"
	maxTopologyFileBytes = 4 * 1024 * 1024
)

type config struct {
	TopologyFile string
	Service      string
}

type topologyFile struct {
	Services []serviceNode `json:"services"`
}

type serviceNode struct {
	Name         string            `json:"name"`
	Owner        string            `json:"owner,omitempty"`
	Tier         string            `json:"tier,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Dependents   []string          `json:"dependents,omitempty"`
	Runbooks     []string          `json:"runbooks,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type topologyOutput struct {
	Tool         string        `json:"tool"`
	Source       string        `json:"source"`
	Service      string        `json:"service"`
	Node         serviceNode   `json:"node"`
	Dependencies []serviceNode `json:"dependencies,omitempty"`
	Dependents   []serviceNode `json:"dependents,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "[agent-tool-topology-lookup] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, getenv func(string) string) error {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}
	out, err := lookupTopology(cfg)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	fs := flag.NewFlagSet("agent_tool_topology_lookup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	topologyFile := fs.String("topology-file", strings.TrimSpace(getenv(envTopologyFile)), "static topology JSON file")
	service := fs.String("service", "", "service name to look up")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg := config{
		TopologyFile: strings.TrimSpace(*topologyFile),
		Service:      strings.TrimSpace(*service),
	}
	if cfg.TopologyFile == "" {
		return config{}, fmt.Errorf("%s or --topology-file is required", envTopologyFile)
	}
	if cfg.Service == "" {
		return config{}, errors.New("--service is required")
	}
	if strings.ContainsAny(cfg.Service, "/\\\x00") {
		return config{}, errors.New("--service must be a plain service name")
	}
	return cfg, nil
}

func lookupTopology(cfg config) (topologyOutput, error) {
	topology, err := loadTopology(cfg.TopologyFile)
	if err != nil {
		return topologyOutput{}, err
	}
	byName := make(map[string]serviceNode, len(topology.Services))
	for _, node := range topology.Services {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			return topologyOutput{}, errors.New("topology service name must be non-empty")
		}
		if _, exists := byName[name]; exists {
			return topologyOutput{}, fmt.Errorf("topology contains duplicate service %q", name)
		}
		node.Name = name
		node.Dependencies = sortedUnique(node.Dependencies)
		node.Dependents = sortedUnique(node.Dependents)
		node.Runbooks = sortedUnique(node.Runbooks)
		byName[name] = node
	}
	node, ok := byName[cfg.Service]
	if !ok {
		return topologyOutput{}, fmt.Errorf("service %q not found in topology", cfg.Service)
	}
	return topologyOutput{
		Tool:         "topology_lookup",
		Source:       "static_json",
		Service:      cfg.Service,
		Node:         node,
		Dependencies: relatedNodes(node.Dependencies, byName),
		Dependents:   relatedNodes(node.Dependents, byName),
	}, nil
}

func loadTopology(path string) (topologyFile, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return topologyFile{}, err
	}
	// #nosec G304 -- this helper intentionally reads the operator-supplied
	// static topology file mounted into the sandbox image.
	f, err := os.Open(clean)
	if err != nil {
		return topologyFile{}, fmt.Errorf("open topology file: %w", err)
	}
	defer f.Close()
	raw, err := readCapped(f, maxTopologyFileBytes)
	if err != nil {
		return topologyFile{}, err
	}
	var topology topologyFile
	if err := strictjson.Unmarshal(raw, &topology); err != nil {
		return topologyFile{}, fmt.Errorf("decode topology JSON: %w", err)
	}
	if len(topology.Services) == 0 {
		return topologyFile{}, errors.New("topology must contain at least one service")
	}
	return topology, nil
}

func requireRegularFile(path string) error {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("stat topology file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", clean)
	}
	return nil
}

func readCapped(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read topology file: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("topology file exceeds maximum %d bytes", maxBytes)
	}
	return raw, nil
}

func relatedNodes(names []string, byName map[string]serviceNode) []serviceNode {
	out := make([]serviceNode, 0, len(names))
	for _, name := range names {
		if node, ok := byName[name]; ok {
			out = append(out, node)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sortedUnique(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
