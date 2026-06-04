package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

type fakeSubReportStore struct {
	rows map[int64]storedSubReport
}

func (store fakeSubReportStore) FindSubReportByID(_ context.Context, id int64) (storedSubReport, error) {
	row, ok := store.rows[id]
	if !ok {
		return storedSubReport{}, fmt.Errorf("subreport %d not found", id)
	}
	return row, nil
}

func TestRunExportsSelectedSubReportsToQualitySampleLayout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selection := writeSelection(t, dir, `{
		"cases": [
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"direct_sub_report_id": 101,
				"sandbox_sub_report_id": 201
			}
		]
	}`)
	outRoot := filepath.Join(dir, "quality-sample")
	store := fakeSubReportStore{rows: map[int64]storedSubReport{
		101: validStoredSubReport(101, 11, string(reportprompt.ScenarioSingleAlert), "direct baseline"),
		201: validStoredSubReport(201, 11, string(reportprompt.ScenarioSingleAlert), "sandbox candidate"),
	}}

	var stdout bytes.Buffer
	err := run(context.Background(), []string{"--selection", selection, "--out-root", outRoot}, nil, &stdout, store)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	assertValidSubReportFile(t, filepath.Join(outRoot, "direct", "single_alert", "payments-cpu.json"))
	assertValidSubReportFile(t, filepath.Join(outRoot, "sandbox", "single_alert", "payments-cpu.json"))

	var summary exportSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("parse stdout summary: %v\n%s", err, stdout.String())
	}
	if summary.Tool != toolName {
		t.Fatalf("summary tool = %q, want %q", summary.Tool, toolName)
	}
	if summary.SchemaID != summarySchemaID {
		t.Fatalf("summary schema = %q, want %q", summary.SchemaID, summarySchemaID)
	}
	if summary.OutRoot != "." {
		t.Fatalf("summary out_root = %q, want .", summary.OutRoot)
	}
	if summary.CaseCount != 1 || len(summary.Cases) != 1 {
		t.Fatalf("summary cases = count %d len %d, want 1", summary.CaseCount, len(summary.Cases))
	}
	got := summary.Cases[0]
	if got.DirectSubReport != "direct/single_alert/payments-cpu.json" {
		t.Fatalf("direct path = %q", got.DirectSubReport)
	}
	if got.SandboxSubReport != "sandbox/single_alert/payments-cpu.json" {
		t.Fatalf("sandbox path = %q", got.SandboxSubReport)
	}
	if got.EvidenceSnapshotID != 11 {
		t.Fatalf("evidence_snapshot_id = %d, want 11", got.EvidenceSnapshotID)
	}
}

func TestRunRejectsExistingNonEmptyOutputRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selection := writeSelection(t, dir, `{
		"cases": [
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"direct_sub_report_id": 101,
				"sandbox_sub_report_id": 201
			}
		]
	}`)
	outRoot := filepath.Join(dir, "quality-sample")
	if err := os.Mkdir(outRoot, 0o700); err != nil {
		t.Fatalf("create output root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outRoot, "old.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}
	store := fakeSubReportStore{rows: map[int64]storedSubReport{
		101: validStoredSubReport(101, 11, string(reportprompt.ScenarioSingleAlert), "direct baseline"),
		201: validStoredSubReport(201, 11, string(reportprompt.ScenarioSingleAlert), "sandbox candidate"),
	}}

	err := run(context.Background(), []string{"--selection", selection, "--out-root", outRoot}, nil, ioDiscard{}, store)
	if err == nil || !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("run error = %v, want non-empty output root rejection", err)
	}
}

func TestRunRejectsSelectionScenarioMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selection := writeSelection(t, dir, `{
		"cases": [
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"direct_sub_report_id": 101,
				"sandbox_sub_report_id": 201
			}
		]
	}`)
	store := fakeSubReportStore{rows: map[int64]storedSubReport{
		101: validStoredSubReport(101, 11, string(reportprompt.ScenarioCascade), "direct baseline"),
		201: validStoredSubReport(201, 11, string(reportprompt.ScenarioSingleAlert), "sandbox candidate"),
	}}

	err := run(context.Background(), []string{"--selection", selection, "--out-root", filepath.Join(dir, "out")}, nil, ioDiscard{}, store)
	if err == nil || !strings.Contains(err.Error(), "does not match selection scenario") {
		t.Fatalf("run error = %v, want scenario mismatch rejection", err)
	}
}

func TestRunRejectsInvalidPersistedContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selection := writeSelection(t, dir, `{
		"cases": [
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"direct_sub_report_id": 101,
				"sandbox_sub_report_id": 201
			}
		]
	}`)
	store := fakeSubReportStore{rows: map[int64]storedSubReport{
		101: {
			ID:                 101,
			EvidenceSnapshotID: 11,
			Scenario:           string(reportprompt.ScenarioSingleAlert),
			Content:            json.RawMessage(`{"title":"first","title":"second"}`),
			Model:              "test-model",
			OutputMode:         string(ports.LLMOutputModeJSONSchema),
		},
		201: validStoredSubReport(201, 11, string(reportprompt.ScenarioSingleAlert), "sandbox candidate"),
	}}

	err := run(context.Background(), []string{"--selection", selection, "--out-root", filepath.Join(dir, "out")}, nil, ioDiscard{}, store)
	if err == nil || !strings.Contains(err.Error(), "failed production SubReport validation") {
		t.Fatalf("run error = %v, want persisted content rejection", err)
	}
}

func TestRunRejectsMixedEvidenceSnapshots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selection := writeSelection(t, dir, `{
		"cases": [
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"direct_sub_report_id": 101,
				"sandbox_sub_report_id": 201
			}
		]
	}`)
	store := fakeSubReportStore{rows: map[int64]storedSubReport{
		101: validStoredSubReport(101, 11, string(reportprompt.ScenarioSingleAlert), "direct baseline"),
		201: validStoredSubReport(201, 12, string(reportprompt.ScenarioSingleAlert), "sandbox candidate"),
	}}

	err := run(context.Background(), []string{"--selection", selection, "--out-root", filepath.Join(dir, "out")}, nil, ioDiscard{}, store)
	if err == nil || !strings.Contains(err.Error(), "differs from sandbox evidence snapshot") {
		t.Fatalf("run error = %v, want mixed snapshot rejection", err)
	}
}

func TestRunRejectsDuplicateCaseID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selection := writeSelection(t, dir, `{
		"cases": [
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"direct_sub_report_id": 101,
				"sandbox_sub_report_id": 201
			},
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"direct_sub_report_id": 102,
				"sandbox_sub_report_id": 202
			}
		]
	}`)

	err := run(context.Background(), []string{"--selection", selection, "--out-root", filepath.Join(dir, "out")}, nil, ioDiscard{}, fakeSubReportStore{})
	if err == nil || !strings.Contains(err.Error(), "duplicates case") {
		t.Fatalf("run error = %v, want duplicate case rejection", err)
	}
}

func TestRunRequiresDatabaseURLWhenNoInjectedStore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selection := writeSelection(t, dir, `{
		"cases": [
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"direct_sub_report_id": 101,
				"sandbox_sub_report_id": 201
			}
		]
	}`)

	err := run(context.Background(), []string{"--selection", selection, "--out-root", filepath.Join(dir, "out")}, nil, ioDiscard{}, nil)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("run error = %v, want DATABASE_URL requirement", err)
	}
}

func writeSelection(t *testing.T, dir, raw string) string {
	t.Helper()
	path := filepath.Join(dir, "selection.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write selection: %v", err)
	}
	return path
}

func validStoredSubReport(id, snapshotID int64, scenario, titleSuffix string) storedSubReport {
	return storedSubReport{
		ID:                 id,
		EvidenceSnapshotID: snapshotID,
		Scenario:           scenario,
		Content:            validSubReportJSON(titleSuffix, snapshotID),
		Model:              "test-model",
		OutputMode:         string(ports.LLMOutputModeJSONSchema),
	}
}

func validSubReportJSON(titleSuffix string, snapshotID int64) json.RawMessage {
	ref := fmt.Sprintf("snapshot:%d", snapshotID)
	raw, err := json.Marshal(reportdraft.SubReport{
		Title:      "CPU saturation " + titleSuffix,
		Summary:    "CPU saturation is visible in the selected alert evidence.",
		Severity:   reportdraft.SeverityWarning,
		Confidence: reportdraft.ConfidenceMedium,
		Findings: []reportdraft.Finding{
			{
				Label:      "CPU saturation",
				Detail:     "The alert group shows sustained CPU pressure.",
				EvidenceID: ref,
			},
		},
		RecommendedActions: []reportdraft.Action{
			{
				Label:    "Inspect workload",
				Detail:   "Inspect recent workload changes for the affected service.",
				Priority: reportdraft.PriorityHigh,
			},
		},
		EvidenceRefs: []string{ref, "alert:cpu"},
	})
	if err != nil {
		panic(err)
	}
	return raw
}

func assertValidSubReportFile(t *testing.T, path string) {
	t.Helper()
	// #nosec G304 -- tests read the sample path created under t.TempDir by the helper.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	_, err = reportdraft.ParseSubReport(ports.LLMResponse{
		Content:      raw,
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "test-model",
	})
	if err != nil {
		t.Fatalf("%s is not a valid SubReport: %v", path, err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
