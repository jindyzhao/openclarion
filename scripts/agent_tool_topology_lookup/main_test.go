package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLooksUpServiceTopology(t *testing.T) {
	path := writeTopology(t, `{
		"services": [
			{"name":"postgres","owner":"platform","tier":"data"},
			{"name":"checkout","owner":"checkout-team","tier":"edge","dependencies":["payments"]},
			{"name":"payments","owner":"payments-team","tier":"backend","dependencies":["postgres","postgres"],"dependents":["checkout"],"runbooks":["https://runbooks/payments"],"metadata":{"env":"prod"}}
		]
	}`)

	var stdout bytes.Buffer
	err := run([]string{"--service", "payments"}, &stdout, mapGetenv(map[string]string{
		envTopologyFile: path,
	}))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var out topologyOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout JSON: %v\n%s", err, stdout.String())
	}
	if out.Tool != "topology_lookup" || out.Source != "static_json" {
		t.Fatalf("unexpected tool/source: %#v", out)
	}
	if out.Node.Name != "payments" || out.Node.Owner != "payments-team" {
		t.Fatalf("node = %#v", out.Node)
	}
	if len(out.Node.Dependencies) != 1 || out.Node.Dependencies[0] != "postgres" {
		t.Fatalf("node dependencies = %#v", out.Node.Dependencies)
	}
	if len(out.Dependencies) != 1 || out.Dependencies[0].Name != "postgres" {
		t.Fatalf("dependencies = %#v", out.Dependencies)
	}
	if len(out.Dependents) != 1 || out.Dependents[0].Name != "checkout" {
		t.Fatalf("dependents = %#v", out.Dependents)
	}
}

func TestParseConfigRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr string
	}{
		{name: "missing file", args: []string{"--service", "payments"}, wantErr: envTopologyFile},
		{name: "missing service", env: map[string]string{envTopologyFile: "/tmp/topology.json"}, wantErr: "--service"},
		{name: "path service", args: []string{"--service", "../payments"}, env: map[string]string{envTopologyFile: "/tmp/topology.json"}, wantErr: "plain service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig(tt.args, mapGetenv(tt.env))
			if err == nil {
				t.Fatal("parseConfig err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseConfig err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLookupTopologyRejectsBadTopology(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		service string
		wantErr string
	}{
		{name: "duplicate", body: `{"services":[{"name":"api"},{"name":"api"}]}`, service: "api", wantErr: "duplicate"},
		{name: "empty name", body: `{"services":[{"name":" "}]}`, service: "api", wantErr: "non-empty"},
		{name: "missing service", body: `{"services":[{"name":"api"}]}`, service: "worker", wantErr: "not found"},
		{name: "unknown field", body: `{"services":[{"name":"api","extra":true}]}`, service: "api", wantErr: "unknown field"},
		{name: "duplicate key", body: `{"services":[{"name":"stale","name":"api"}]}`, service: "api", wantErr: "duplicate object key"},
		{name: "trailing", body: `{"services":[{"name":"api"}]} {}`, service: "api", wantErr: "trailing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTopology(t, tt.body)
			_, err := lookupTopology(config{TopologyFile: path, Service: tt.service})
			if err == nil {
				t.Fatal("lookupTopology err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("lookupTopology err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLookupTopologyRejectsSymlinkTopologyFile(t *testing.T) {
	target := writeTopology(t, `{"services":[{"name":"api"}]}`)
	link := filepath.Join(t.TempDir(), "topology-link.json")
	createSymlinkOrSkip(t, target, link)

	_, err := lookupTopology(config{TopologyFile: link, Service: "api"})
	if err == nil {
		t.Fatal("lookupTopology err = nil, want symlink rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("lookupTopology err = %v, want symlink rejection", err)
	}
}

func writeTopology(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "topology.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write topology: %v", err)
	}
	return path
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func mapGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
