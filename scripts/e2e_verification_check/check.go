// Package main validates END_TO_END_VERIFICATION.md verdict taxonomy drift.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultDocPath = "docs/design/END_TO_END_VERIFICATION.md"

func main() {
	path := defaultDocPath
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "[e2e-verification] usage: e2e_verification_check [markdown-file]")
		os.Exit(2)
	}
	if len(os.Args) == 2 {
		path = os.Args[1]
	}
	if err := Check(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Check verifies that every verdict used in the E2E verification tables is
// defined in the Verdict Scale table.
func Check(path string) error {
	cleaned := filepath.Clean(path)
	info, err := os.Lstat(cleaned)
	if err != nil {
		return fmt.Errorf("[e2e-verification] %s: %w", cleaned, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("[e2e-verification] %s must be a regular file, not a symlink", cleaned)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("[e2e-verification] %s must be a regular file", cleaned)
	}
	raw, err := os.ReadFile(cleaned)
	if err != nil {
		return fmt.Errorf("[e2e-verification] read %s: %w", cleaned, err)
	}

	defined, used, err := verdicts(string(raw))
	if err != nil {
		return fmt.Errorf("[e2e-verification] %s: %w", cleaned, err)
	}
	if len(defined) == 0 {
		return fmt.Errorf("[e2e-verification] %s: Verdict Scale table has no verdict definitions", cleaned)
	}
	if len(used) == 0 {
		return fmt.Errorf("[e2e-verification] %s: no verdict usages found outside Verdict Scale", cleaned)
	}

	var missing []string
	for verdict := range used {
		if !defined[verdict] {
			missing = append(missing, verdict)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("[e2e-verification] %s uses undefined verdict(s): %s", cleaned, strings.Join(missing, ", "))
	}
	fmt.Fprintf(os.Stdout, "[e2e-verification] OK (%d definitions, %d usages)\n", len(defined), len(used))
	return nil
}

func verdicts(markdown string) (map[string]bool, map[string]bool, error) {
	lines := strings.Split(markdown, "\n")
	defined := map[string]bool{}
	used := map[string]bool{}

	var section string
	inTable := false
	var tableHeader []string
	verdictColumn := -1
	isScaleTable := false
	sawScale := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			section = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			inTable = false
			tableHeader = nil
			verdictColumn = -1
			isScaleTable = false
			continue
		}
		if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
			inTable = false
			tableHeader = nil
			verdictColumn = -1
			isScaleTable = false
			continue
		}

		cells := splitMarkdownTableRow(trimmed)
		if len(cells) == 0 {
			continue
		}
		if !inTable {
			tableHeader = normalizeCells(cells)
			verdictColumn = indexOf(tableHeader, "verdict")
			isScaleTable = section == "Verdict Scale"
			if isScaleTable && verdictColumn < 0 {
				return nil, nil, errors.New("Verdict Scale table must include a Verdict column")
			}
			inTable = true
			continue
		}
		if isSeparatorRow(cells) {
			continue
		}
		if len(cells) < len(tableHeader) {
			continue
		}

		if isScaleTable {
			sawScale = true
			verdict := normalizeVerdict(cells[verdictColumn])
			if verdict != "" {
				defined[verdict] = true
			}
			continue
		}
		if verdictColumn >= 0 {
			verdict := normalizeVerdict(cells[verdictColumn])
			if verdict != "" {
				used[verdict] = true
			}
		}
	}
	if !sawScale {
		return nil, nil, errors.New("missing Verdict Scale table")
	}
	return defined, used, nil
}

func splitMarkdownTableRow(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	raw := strings.Split(row, "|")
	out := make([]string, 0, len(raw))
	for _, cell := range raw {
		out = append(out, strings.TrimSpace(cell))
	}
	return out
}

func normalizeCells(cells []string) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = normalizeText(cell)
	}
	return out
}

func normalizeVerdict(value string) string {
	return normalizeText(value)
}

func normalizeText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "*")
	value = strings.Trim(value, "`")
	value = strings.ToLower(strings.TrimSpace(value))
	return value
}

func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		trimmed := strings.TrimSpace(cell)
		trimmed = strings.Trim(trimmed, ":")
		if trimmed == "" || strings.Trim(trimmed, "-") != "" {
			return false
		}
	}
	return true
}

func indexOf(values []string, needle string) int {
	for i, value := range values {
		if value == needle {
			return i
		}
	}
	return -1
}
