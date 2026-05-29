// Command gate_hardening_check validates that every activated CI schedule row
// has a gate-maturity record.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const (
	defaultSchedulePath  = "docs/design/ci/README.md"
	defaultChecklistPath = "docs/design/ci/GATE_HARDENING_CHECKLIST.md"
)

var allowedMaturity = map[string]struct{}{
	"baseline": {},
	"hardened": {},
	"manual":   {},
	"mature":   {},
	"partial":  {},
	"replaced": {},
}

type config struct {
	SchedulePath  string
	ChecklistPath string
}

type scheduleGate struct {
	Name   string
	Status string
}

type checklistRecord struct {
	Gate          string
	Maturity      string
	Evidence      string
	NextHardening string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.SchedulePath, "schedule", defaultSchedulePath, "CI schedule markdown path")
	flag.StringVar(&cfg.ChecklistPath, "checklist", defaultChecklistPath, "gate hardening checklist markdown path")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[gate-hardening] %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config, out io.Writer) error {
	gates, err := readSchedule(cfg.SchedulePath)
	if err != nil {
		return err
	}
	records, err := readChecklist(cfg.ChecklistPath)
	if err != nil {
		return err
	}

	want := map[string]scheduleGate{}
	for _, gate := range gates {
		if !activatedStatus(gate.Status) {
			continue
		}
		want[gate.Name] = gate
	}

	var problems []string
	for name := range want {
		record, ok := records[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("missing maturity record for schedule gate %q", name))
			continue
		}
		if err := validateRecord(record); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
		}
	}
	for name := range records {
		if _, ok := want[name]; !ok {
			problems = append(problems, fmt.Sprintf("stale maturity record without activated schedule row %q", name))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}

	fmt.Fprintf(out, "[gate-hardening] OK (%d activated gates audited)\n", len(want))
	return nil
}

func readSchedule(path string) ([]scheduleGate, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- repository-owned checker input.
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	rows := tableRowsInSection(lines, "## Progressive Gate Schedule", "## Current Private-Incubation Gate")
	gates := make([]scheduleGate, 0, len(rows))
	for _, cells := range rows {
		if len(cells) < 4 || strings.EqualFold(cells[0], "Gate") || isSeparator(cells) {
			continue
		}
		gates = append(gates, scheduleGate{
			Name:   normalizeGate(cells[0]),
			Status: strings.ToLower(strings.TrimSpace(cells[2])),
		})
	}
	if len(gates) == 0 {
		return nil, fmt.Errorf("%s: no schedule gates found", path)
	}
	return gates, nil
}

func readChecklist(path string) (map[string]checklistRecord, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- repository-owned checker input.
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	rows := tableRowsInSection(lines, "## Gate Maturity Records", "")
	records := map[string]checklistRecord{}
	for _, cells := range rows {
		if len(cells) < 4 || strings.EqualFold(cells[0], "Gate") || isSeparator(cells) {
			continue
		}
		record := checklistRecord{
			Gate:          normalizeGate(cells[0]),
			Maturity:      strings.ToLower(strings.TrimSpace(cells[1])),
			Evidence:      strings.TrimSpace(cells[2]),
			NextHardening: strings.TrimSpace(cells[3]),
		}
		if _, exists := records[record.Gate]; exists {
			return nil, fmt.Errorf("%s: duplicate maturity record for %q", path, record.Gate)
		}
		records[record.Gate] = record
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%s: no gate maturity records found", path)
	}
	return records, nil
}

func tableRowsInSection(lines []string, startHeading, endHeading string) [][]string {
	var rows [][]string
	inSection := false
	for _, line := range lines {
		switch {
		case strings.TrimSpace(line) == startHeading:
			inSection = true
			continue
		case inSection && endHeading != "" && strings.TrimSpace(line) == endHeading:
			return rows
		case inSection && endHeading == "" && strings.HasPrefix(line, "## ") && strings.TrimSpace(line) != startHeading:
			return rows
		}
		if !inSection {
			continue
		}
		if cells, ok := splitMarkdownRow(line); ok {
			rows = append(rows, cells)
		}
	}
	return rows
}

func splitMarkdownRow(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil, false
	}
	line = strings.TrimPrefix(strings.TrimSuffix(line, "|"), "|")
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells, true
}

func isSeparator(cells []string) bool {
	for _, cell := range cells {
		trimmed := strings.Trim(cell, " :-")
		if trimmed != "" {
			return false
		}
	}
	return true
}

func normalizeGate(gate string) string {
	gate = strings.ReplaceAll(gate, "`", "")
	return strings.Join(strings.Fields(gate), " ")
}

func activatedStatus(status string) bool {
	for _, marker := range []string{"landed", "manual", "partial", "replaced"} {
		if strings.Contains(status, marker) {
			return true
		}
	}
	return false
}

func validateRecord(record checklistRecord) error {
	var problems []string
	if _, ok := allowedMaturity[record.Maturity]; !ok {
		problems = append(problems, fmt.Sprintf("unsupported maturity %q", record.Maturity))
	}
	for field, value := range map[string]string{
		"evidence":       record.Evidence,
		"next hardening": record.NextHardening,
	} {
		if placeholder(value) {
			problems = append(problems, fmt.Sprintf("%s must be concrete, got %q", field, value))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

func placeholder(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return true
	}
	for _, marker := range []string{"todo", "tbd"} {
		if strings.Contains(trimmed, marker) {
			return true
		}
	}
	if trimmed == "n/a" || trimmed == "none" {
		return true
	}
	return false
}
