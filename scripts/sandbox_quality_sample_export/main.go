// Command sandbox_quality_sample_export exports operator-selected persisted
// SubReport rows into the retained sample layout consumed by the M4 sandbox
// quality manifest helper.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/repository"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

const (
	toolName              = "sandbox_quality_sample_export"
	summarySchemaID       = "openclarion_sandbox_quality_sample_export_v1"
	maxInputBytes   int64 = 1024 * 1024
	maxCases              = 100
	maxCaseIDBytes        = 128
	directRole            = "direct"
	sandboxRole           = "sandbox"
)

type config struct {
	SelectionPath string
	OutRoot       string
	DatabaseURL   string
}

type selectionFile struct {
	Cases []selectionCase `json:"cases"`
}

type selectionCase struct {
	ID                 string `json:"id"`
	Scenario           string `json:"scenario"`
	DirectSubReportID  int64  `json:"direct_sub_report_id"`
	SandboxSubReportID int64  `json:"sandbox_sub_report_id"`
}

type caseKey struct {
	Scenario string
	ID       string
}

type storedSubReport struct {
	ID                 int64
	EvidenceSnapshotID int64
	Scenario           string
	Content            json.RawMessage
	Model              string
	OutputMode         string
}

type subReportStore interface {
	FindSubReportByID(context.Context, int64) (storedSubReport, error)
}

type entSubReportStore struct {
	client *ent.Client
}

type preparedCase struct {
	ID                 string
	Scenario           string
	EvidenceSnapshotID int64
	DirectSubReportID  int64
	SandboxSubReportID int64
	DirectPath         string
	SandboxPath        string
	DirectContent      json.RawMessage
	SandboxContent     json.RawMessage
}

type exportSummary struct {
	Tool      string              `json:"tool"`
	SchemaID  string              `json:"schema_id"`
	OutRoot   string              `json:"out_root"`
	CaseCount int                 `json:"case_count"`
	Cases     []exportSummaryCase `json:"cases"`
}

type exportSummaryCase struct {
	ID                 string `json:"id"`
	Scenario           string `json:"scenario"`
	EvidenceSnapshotID int64  `json:"evidence_snapshot_id"`
	DirectSubReportID  int64  `json:"direct_sub_report_id"`
	SandboxSubReportID int64  `json:"sandbox_sub_report_id"`
	DirectSubReport    string `json:"direct_sub_report"`
	SandboxSubReport   string `json:"sandbox_sub_report"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Environ(), os.Stdout, nil); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", toolName, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, environ []string, stdout io.Writer, store subReportStore) error {
	ctx = tenancy.EnsureDefault(ctx)
	cfg, err := parseConfig(args, environ)
	if err != nil {
		return err
	}
	selection, err := readSelectionFile(cfg.SelectionPath)
	if err != nil {
		return err
	}
	if store == nil {
		if cfg.DatabaseURL == "" {
			return errors.New("DATABASE_URL is required when no test store is injected")
		}
		client, err := repository.OpenPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("open PostgreSQL: %w", err)
		}
		defer client.Close()
		store = entSubReportStore{client: client}
	}
	summary, err := exportSelection(ctx, store, cfg.OutRoot, selection)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		return fmt.Errorf("write export summary: %w", err)
	}
	return nil
}

func parseConfig(args []string, environ []string) (config, error) {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	selection := fs.String("selection", "", "JSON file mapping case IDs to persisted direct/sandbox SubReport IDs")
	outRoot := fs.String("out-root", "", "empty output directory for direct/<scenario>/<case>.json and sandbox/<scenario>/<case>.json")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	cfg := config{
		SelectionPath: strings.TrimSpace(*selection),
		OutRoot:       strings.TrimSpace(*outRoot),
		DatabaseURL:   strings.TrimSpace(environValue(environ, "DATABASE_URL")),
	}
	if cfg.SelectionPath == "" {
		return config{}, errors.New("--selection is required")
	}
	if cfg.OutRoot == "" {
		return config{}, errors.New("--out-root is required")
	}
	return cfg, nil
}

func environValue(environ []string, key string) string {
	prefix := key + "="
	for _, entry := range environ {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func readSelectionFile(path string) (selectionFile, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return selectionFile{}, fmt.Errorf("read selection file: %w", err)
	}
	var selection selectionFile
	if err := strictjson.Unmarshal(raw, &selection); err != nil {
		return selectionFile{}, fmt.Errorf("parse selection file: %w", err)
	}
	return selection, nil
}

func exportSelection(ctx context.Context, store subReportStore, outRoot string, selection selectionFile) (exportSummary, error) {
	cases, err := normalizeSelection(selection)
	if err != nil {
		return exportSummary{}, err
	}
	prepared := make([]preparedCase, 0, len(cases))
	for _, selected := range cases {
		direct, err := loadValidatedSubReport(ctx, store, directRole, selected.DirectSubReportID, selected.Scenario)
		if err != nil {
			return exportSummary{}, fmt.Errorf("case %q: %w", selected.ID, err)
		}
		sandbox, err := loadValidatedSubReport(ctx, store, sandboxRole, selected.SandboxSubReportID, selected.Scenario)
		if err != nil {
			return exportSummary{}, fmt.Errorf("case %q: %w", selected.ID, err)
		}
		if direct.EvidenceSnapshotID != sandbox.EvidenceSnapshotID {
			return exportSummary{}, fmt.Errorf("case %q direct evidence snapshot %d differs from sandbox evidence snapshot %d", selected.ID, direct.EvidenceSnapshotID, sandbox.EvidenceSnapshotID)
		}
		prepared = append(prepared, preparedCase{
			ID:                 selected.ID,
			Scenario:           selected.Scenario,
			EvidenceSnapshotID: direct.EvidenceSnapshotID,
			DirectSubReportID:  direct.ID,
			SandboxSubReportID: sandbox.ID,
			DirectPath:         sampleRelativePath(directRole, selected.Scenario, selected.ID),
			SandboxPath:        sampleRelativePath(sandboxRole, selected.Scenario, selected.ID),
			DirectContent:      direct.Content,
			SandboxContent:     sandbox.Content,
		})
	}
	root, err := prepareOutputRoot(outRoot)
	if err != nil {
		return exportSummary{}, err
	}
	for _, item := range prepared {
		if err := writeSampleFile(root, item.DirectPath, item.DirectContent); err != nil {
			return exportSummary{}, err
		}
		if err := writeSampleFile(root, item.SandboxPath, item.SandboxContent); err != nil {
			return exportSummary{}, err
		}
	}
	summary := exportSummary{
		Tool:      toolName,
		SchemaID:  summarySchemaID,
		OutRoot:   ".",
		CaseCount: len(prepared),
		Cases:     make([]exportSummaryCase, 0, len(prepared)),
	}
	for _, item := range prepared {
		summary.Cases = append(summary.Cases, exportSummaryCase{
			ID:                 item.ID,
			Scenario:           item.Scenario,
			EvidenceSnapshotID: item.EvidenceSnapshotID,
			DirectSubReportID:  item.DirectSubReportID,
			SandboxSubReportID: item.SandboxSubReportID,
			DirectSubReport:    item.DirectPath,
			SandboxSubReport:   item.SandboxPath,
		})
	}
	return summary, nil
}

func normalizeSelection(selection selectionFile) ([]selectionCase, error) {
	if len(selection.Cases) == 0 {
		return nil, errors.New("selection must contain at least one case")
	}
	if len(selection.Cases) > maxCases {
		return nil, fmt.Errorf("selection contains %d cases, max %d", len(selection.Cases), maxCases)
	}
	cases := make([]selectionCase, len(selection.Cases))
	seenCases := map[caseKey]struct{}{}
	seenReportIDs := map[int64]string{}
	for i, item := range selection.Cases {
		if err := validateCaseID(item.ID); err != nil {
			return nil, fmt.Errorf("cases[%d].id: %w", i, err)
		}
		scenario := reportprompt.Scenario(item.Scenario)
		if !scenario.Valid() {
			return nil, fmt.Errorf("cases[%d].scenario %q is unsupported", i, item.Scenario)
		}
		if item.DirectSubReportID <= 0 {
			return nil, fmt.Errorf("cases[%d].direct_sub_report_id must be positive", i)
		}
		if item.SandboxSubReportID <= 0 {
			return nil, fmt.Errorf("cases[%d].sandbox_sub_report_id must be positive", i)
		}
		if item.DirectSubReportID == item.SandboxSubReportID {
			return nil, fmt.Errorf("cases[%d] direct_sub_report_id and sandbox_sub_report_id must be distinct", i)
		}
		key := caseKey{Scenario: item.Scenario, ID: item.ID}
		if _, exists := seenCases[key]; exists {
			return nil, fmt.Errorf("cases[%d] duplicates case %q scenario %q", i, item.ID, item.Scenario)
		}
		seenCases[key] = struct{}{}
		if err := recordSelectedReportID(seenReportIDs, item.DirectSubReportID, item.ID, directRole); err != nil {
			return nil, fmt.Errorf("cases[%d]: %w", i, err)
		}
		if err := recordSelectedReportID(seenReportIDs, item.SandboxSubReportID, item.ID, sandboxRole); err != nil {
			return nil, fmt.Errorf("cases[%d]: %w", i, err)
		}
		cases[i] = item
	}
	sort.Slice(cases, func(i, j int) bool {
		leftRank := scenarioRank(cases[i].Scenario)
		rightRank := scenarioRank(cases[j].Scenario)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return cases[i].ID < cases[j].ID
	})
	return cases, nil
}

func recordSelectedReportID(seen map[int64]string, id int64, caseID, role string) error {
	label := caseID + "/" + role
	if previous, exists := seen[id]; exists {
		return fmt.Errorf("subreport id %d is reused by %s and %s", id, previous, label)
	}
	seen[id] = label
	return nil
}

func validateCaseID(id string) error {
	if id == "" {
		return errors.New("must be non-empty")
	}
	if strings.TrimSpace(id) != id {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if strings.ContainsAny(id, "\r\n\t") {
		return errors.New("must be single-line without tabs")
	}
	if strings.ContainsAny(id, `/\:`) {
		return errors.New("must not contain path separators or drive syntax")
	}
	if len([]byte(id)) > maxCaseIDBytes {
		return fmt.Errorf("must be no more than %d bytes", maxCaseIDBytes)
	}
	return nil
}

func scenarioRank(scenario string) int {
	switch reportprompt.Scenario(scenario) {
	case reportprompt.ScenarioSingleAlert:
		return 0
	case reportprompt.ScenarioCascade:
		return 1
	case reportprompt.ScenarioAlertStorm:
		return 2
	default:
		return 99
	}
}

func loadValidatedSubReport(ctx context.Context, store subReportStore, role string, id int64, scenario string) (storedSubReport, error) {
	row, err := store.FindSubReportByID(ctx, id)
	if err != nil {
		return storedSubReport{}, fmt.Errorf("%s subreport id %d: %w", role, id, err)
	}
	if row.ID != id {
		return storedSubReport{}, fmt.Errorf("%s subreport id %d returned row id %d", role, id, row.ID)
	}
	if row.EvidenceSnapshotID <= 0 {
		return storedSubReport{}, fmt.Errorf("%s subreport id %d has non-positive evidence_snapshot_id", role, id)
	}
	if row.Scenario != scenario {
		return storedSubReport{}, fmt.Errorf("%s subreport id %d scenario %q does not match selection scenario %q", role, id, row.Scenario, scenario)
	}
	mode, err := parseOutputMode(row.OutputMode)
	if err != nil {
		return storedSubReport{}, fmt.Errorf("%s subreport id %d: %w", role, id, err)
	}
	model := strings.TrimSpace(row.Model)
	if model == "" {
		model = toolName
	}
	content := cloneRawMessage(row.Content)
	if _, err := reportdraft.ParseSubReport(ports.LLMResponse{
		Content:      content,
		FinishReason: "stop",
		OutputMode:   mode,
		Model:        model,
	}); err != nil {
		return storedSubReport{}, fmt.Errorf("%s subreport id %d content failed production SubReport validation: %w", role, id, err)
	}
	row.Content = content
	return row, nil
}

func parseOutputMode(raw string) (ports.LLMOutputMode, error) {
	if raw == "" {
		return ports.LLMOutputModeJSONSchema, nil
	}
	if strings.TrimSpace(raw) != raw {
		return "", fmt.Errorf("output_mode %q must not contain leading or trailing whitespace", raw)
	}
	switch mode := ports.LLMOutputMode(raw); mode {
	case ports.LLMOutputModeJSONSchema, ports.LLMOutputModeJSONObject:
		return mode, nil
	default:
		return "", fmt.Errorf("output_mode %q is unsupported", raw)
	}
}

func (store entSubReportStore) FindSubReportByID(ctx context.Context, id int64) (storedSubReport, error) {
	if id > maxIntValue() {
		return storedSubReport{}, fmt.Errorf("id %d exceeds platform int max", id)
	}
	row, err := store.client.SubReport.Get(ctx, int(id))
	if err != nil {
		return storedSubReport{}, err
	}
	return storedSubReport{
		ID:                 int64(row.ID),
		EvidenceSnapshotID: int64(row.EvidenceSnapshotID),
		Scenario:           row.Scenario,
		Content:            cloneRawMessage(row.Content),
		Model:              row.Model,
		OutputMode:         row.OutputMode,
	}, nil
}

func maxIntValue() int64 {
	return int64(^uint(0) >> 1)
}

func prepareOutputRoot(path string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || isFilesystemRoot(clean) {
		return "", errors.New("--out-root must not be the current directory or a filesystem root")
	}
	info, err := os.Lstat(clean)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("output root %q must not be a symlink", clean)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("output root %q must be a directory", clean)
		}
		entries, err := os.ReadDir(clean)
		if err != nil {
			return "", fmt.Errorf("read output root %q: %w", clean, err)
		}
		if len(entries) != 0 {
			return "", fmt.Errorf("output root %q must be empty", clean)
		}
	case errors.Is(err, os.ErrNotExist):
		parent := filepath.Dir(clean)
		if err := requireDirectDirectory(parent); err != nil {
			return "", fmt.Errorf("output root parent: %w", err)
		}
		if err := os.Mkdir(clean, 0o700); err != nil {
			return "", fmt.Errorf("create output root %q: %w", clean, err)
		}
	default:
		return "", fmt.Errorf("stat output root %q: %w", clean, err)
	}
	return clean, nil
}

func isFilesystemRoot(path string) bool {
	return path == filepath.Dir(path)
}

func writeSampleFile(root, relPath string, raw json.RawMessage) error {
	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create sample directory for %q: %w", relPath, err)
	}
	// #nosec G304 -- relPath is generated from validated role/scenario/case IDs and joined under the empty operator-selected root.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create sample file %q: %w", relPath, err)
	}
	if _, err := file.Write(raw); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("write sample file %q: %w", relPath, errors.Join(err, closeErr))
		}
		return fmt.Errorf("write sample file %q: %w", relPath, err)
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		if _, err := file.Write([]byte("\n")); err != nil {
			if closeErr := file.Close(); closeErr != nil {
				return fmt.Errorf("write sample file newline %q: %w", relPath, errors.Join(err, closeErr))
			}
			return fmt.Errorf("write sample file newline %q: %w", relPath, err)
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close sample file %q: %w", relPath, err)
	}
	return nil
}

func sampleRelativePath(role, scenario, id string) string {
	return filepath.ToSlash(filepath.Join(role, scenario, id+".json"))
}

func readRegularFile(path string) ([]byte, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." || isFilesystemRoot(clean) {
		return nil, fmt.Errorf("path %q must name a regular file", path)
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%q must not be a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q must be a regular file", clean)
	}
	if info.Size() > maxInputBytes {
		return nil, fmt.Errorf("%q is %d bytes, max %d", clean, info.Size(), maxInputBytes)
	}
	file, err := os.Open(clean)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxInputBytes {
		return nil, fmt.Errorf("%q exceeds %d bytes", clean, maxInputBytes)
	}
	return raw, nil
}

func requireDirectDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q must not be a symlink", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q must be a directory", path)
	}
	return nil
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}
