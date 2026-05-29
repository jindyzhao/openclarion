// Command check_openapi_fingerprint validates OpenAPI critical-node locks.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openclarion/openclarion/internal/strictjson"
	"go.yaml.in/yaml/v3"
)

const (
	specPath = "api/openapi.yaml"
	lockPath = "docs/design/ci/locks/openapi-critical.lock"
)

var criticalPaths = []string{
	"paths./api/v1/alerts.get",
	"paths./api/v1/dashboard.get",
	"paths./api/v1/diagnosis/rooms.post",
	"paths./api/v1/diagnosis/ws-ticket.post",
	"paths./api/v1/evidence-snapshots.get",
	"paths./api/v1/report-triggers/replay-window.post",
	"paths./api/v1/reports.get",
	"paths./api/v1/reports/{report_id}.get",
	"components.parameters.ListLimit",
	"components.parameters.ReportID",
	"components.schemas.AlertListResponse",
	"components.schemas.AlertEventSummary",
	"components.schemas.DashboardSummary",
	"components.schemas.DashboardAlertStats",
	"components.schemas.DashboardReportStats",
	"components.schemas.DashboardReportSeverityStats",
	"components.schemas.DiagnosisRoomCreateRequest",
	"components.schemas.DiagnosisRoomCreateResponse",
	"components.schemas.DiagnosisWSTicketRequest",
	"components.schemas.DiagnosisWSTicketResponse",
	"components.schemas.EvidenceSnapshotListResponse",
	"components.schemas.EvidenceSnapshotSummary",
	"components.schemas.ReportListResponse",
	"components.schemas.ReportReplayTriggerRequest",
	"components.schemas.ReportReplayTriggerResponse",
	"components.schemas.ReportReplayStats",
	"components.schemas.ReportReplayIngestStats",
	"components.schemas.ReportReplaySnapshotRef",
	"components.schemas.FinalReportSummary",
	"components.schemas.FinalReportDetail",
	"components.schemas.SubReportDetail",
	"components.schemas.FinalReportSubReportSummary",
	"components.schemas.ReportFinding",
	"components.schemas.ReportAction",
	"components.schemas.ReportSeverity",
	"components.schemas.ReportConfidence",
}

func main() {
	update := flag.Bool("update", false, "rewrite the OpenAPI fingerprint lock")
	flag.Parse()

	root, err := readYAML(specPath)
	if err != nil {
		fail(err)
	}
	actual, err := fingerprints(root)
	if err != nil {
		fail(err)
	}
	if *update {
		if err := writeLock(actual); err != nil {
			fail(err)
		}
		fmt.Fprintf(os.Stdout, "[openapi-fingerprint] wrote %s (%d nodes)\n", lockPath, len(actual))
		return
	}
	expected, err := readLock(lockPath)
	if err != nil {
		fail(err)
	}
	if err := compare(expected, actual); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stdout, "[openapi-fingerprint] OK (%d nodes)\n", len(actual))
}

func readYAML(path string) (any, error) {
	// #nosec G304 -- path is one of this repository-owned checker's fixed inputs.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out any
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return normalize(out), nil
}

func fingerprints(root any) (map[string]string, error) {
	out := make(map[string]string, len(criticalPaths))
	for _, path := range criticalPaths {
		node, err := lookup(root, path)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(node)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", path, err)
		}
		sum := sha256.Sum256(raw)
		out[path] = fmt.Sprintf("%x", sum[:])
	}
	return out, nil
}

func lookup(root any, path string) (any, error) {
	cur := root
	for _, segment := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q: segment %q is not an object", path, segment)
		}
		next, ok := m[segment]
		if !ok {
			return nil, fmt.Errorf("path %q: missing segment %q", path, segment)
		}
		cur = next
	}
	return cur, nil
}

func normalize(in any) any {
	switch v := in.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[key] = normalize(val)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = normalize(val)
		}
		return out
	default:
		return v
	}
}

func readLock(path string) (map[string]string, error) {
	// #nosec G304 -- path is one of this repository-owned checker's fixed inputs.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]string
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

func writeLock(values map[string]string) error {
	// #nosec G301 -- this repository metadata directory is meant to be readable.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	// #nosec G306 -- lock files are committed project metadata.
	return os.WriteFile(lockPath, raw, 0o644)
}

func compare(expected, actual map[string]string) error {
	var problems []string
	for path, want := range expected {
		got, ok := actual[path]
		if !ok {
			problems = append(problems, fmt.Sprintf("stale lock entry %s", path))
			continue
		}
		if got != want {
			problems = append(problems, fmt.Sprintf("fingerprint changed for %s", path))
		}
	}
	for path := range actual {
		if _, ok := expected[path]; !ok {
			problems = append(problems, fmt.Sprintf("missing lock entry %s", path))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("%s\nRun: go run scripts/check_openapi_fingerprint.go -update", strings.Join(problems, "\n"))
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "[openapi-fingerprint] %v\n", err)
	os.Exit(1)
}
