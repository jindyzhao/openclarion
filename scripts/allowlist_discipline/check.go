// Command allowlist_discipline verifies that repository allowlist entries stay
// owned, expiring, and tied to a removal trigger.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type allowlistSpec struct {
	Path        string
	EntryMarker string
	Name        string
}

type finding struct {
	Path string
	Line int
	Msg  string
}

var gitleaksSpec = allowlistSpec{
	Path:        ".gitleaks.toml",
	EntryMarker: "[[rules.allowlists]]",
	Name:        "gitleaks allowlist",
}

const allowlistExpiryHorizonDays = 120

var (
	ownerRe          = regexp.MustCompile(`(?i)^#\s*Owner:\s*\S`)
	expiresRe        = regexp.MustCompile(`(?i)^#\s*Expires:\s*([0-9]{4}-[0-9]{2}-[0-9]{2})\.?\s*$`)
	removalTriggerRe = regexp.MustCompile(`(?i)^#\s*Removal trigger:\s*\S`)
)

func main() {
	code := run([]allowlistSpec{gitleaksSpec}, time.Now().UTC(), readRegularFile, os.Stderr)
	os.Exit(code)
}

func run(specs []allowlistSpec, now time.Time, readFile func(string) ([]byte, error), stderr io.Writer) int {
	var findings []finding
	checkedEntries := 0
	for _, spec := range specs {
		raw, err := readFile(spec.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			fmt.Fprintf(stderr, "[allowlist-discipline] cannot read %s: %v\n", spec.Path, err)
			return 2
		}
		result, entries := checkAllowlistFile(spec, string(raw), now)
		checkedEntries += entries
		findings = append(findings, result...)
	}
	if len(findings) > 0 {
		fmt.Fprintln(stderr, "[allowlist-discipline] allowlist metadata violations:")
		for _, f := range findings {
			fmt.Fprintf(stderr, "  %s:%d: %s\n", f.Path, f.Line, f.Msg)
		}
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Expected each allowlist entry to have adjacent comments:")
		fmt.Fprintln(stderr, "  # Owner: <team or person>.")
		fmt.Fprintln(stderr, "  # Expires: YYYY-MM-DD.")
		fmt.Fprintln(stderr, "  # Removal trigger: <specific condition for deletion>.")
		fmt.Fprintf(stderr, "Expires must be no more than %d days in the future.\n", allowlistExpiryHorizonDays)
		return 1
	}
	fmt.Fprintf(stderr, "[allowlist-discipline] OK (%d allowlist entries checked)\n", checkedEntries)
	return 0
}

func readRegularFile(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	if err := validateNoSymlinkAncestors(clean); err != nil {
		return nil, err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", clean, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", clean)
	}
	raw, err := os.ReadFile(clean) // #nosec G304 -- allowlist paths are repository-owned gate inputs.
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", clean, err)
	}
	return raw, nil
}

func validateNoSymlinkAncestors(cleanPath string) error {
	for dir := filepath.Dir(cleanPath); dir != "."; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s parent directory %s must not be a symlink", cleanPath, dir)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s parent path %s must be a directory", cleanPath, dir)
		}
		if parent := filepath.Dir(dir); parent == dir {
			return nil
		}
	}
	return nil
}

func checkAllowlistFile(spec allowlistSpec, contents string, now time.Time) ([]finding, int) {
	lines := strings.Split(contents, "\n")
	var findings []finding
	entries := 0
	for i, line := range lines {
		if strings.TrimSpace(line) != spec.EntryMarker {
			continue
		}
		entries++
		lineNumber := i + 1
		comments := adjacentCommentBlock(lines, i)
		findings = append(findings, checkMetadata(spec, lineNumber, comments, now)...)
	}
	return findings, entries
}

func adjacentCommentBlock(lines []string, markerIndex int) []string {
	var comments []string
	for i := markerIndex - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "#") {
			break
		}
		comments = append([]string{line}, comments...)
	}
	return comments
}

func checkMetadata(spec allowlistSpec, lineNumber int, comments []string, now time.Time) []finding {
	block := strings.Join(comments, "\n")
	var findings []finding
	if !matchesAnyLine(block, ownerRe) {
		findings = append(findings, finding{Path: spec.Path, Line: lineNumber, Msg: spec.Name + " entry must include adjacent `# Owner:` metadata"})
	}
	expiry := expiryDate(block)
	if expiry == "" {
		findings = append(findings, finding{Path: spec.Path, Line: lineNumber, Msg: spec.Name + " entry must include adjacent `# Expires: YYYY-MM-DD` metadata"})
	} else if err := validateExpiry(expiry, now); err != nil {
		findings = append(findings, finding{Path: spec.Path, Line: lineNumber, Msg: spec.Name + " expiry is invalid: " + err.Error()})
	}
	if !matchesAnyLine(block, removalTriggerRe) {
		findings = append(findings, finding{Path: spec.Path, Line: lineNumber, Msg: spec.Name + " entry must include adjacent `# Removal trigger:` metadata"})
	}
	return findings
}

func matchesAnyLine(block string, re *regexp.Regexp) bool {
	for _, line := range strings.Split(block, "\n") {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func expiryDate(block string) string {
	for _, line := range strings.Split(block, "\n") {
		match := expiresRe.FindStringSubmatch(line)
		if len(match) == 2 {
			return match[1]
		}
	}
	return ""
}

func validateExpiry(value string, now time.Time) error {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return fmt.Errorf("%q must be a valid YYYY-MM-DD date", value)
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if parsed.Before(today) {
		return fmt.Errorf("%s is expired", value)
	}
	latest := today.AddDate(0, 0, allowlistExpiryHorizonDays)
	if parsed.After(latest) {
		return fmt.Errorf("%s is more than %d days in the future", value, allowlistExpiryHorizonDays)
	}
	return nil
}
