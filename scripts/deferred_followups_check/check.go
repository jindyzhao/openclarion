// Command deferred_followups_check validates the deferred-decision ledger.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultDeferredFollowupsPath = "docs/design/DEFERRED_FOLLOWUPS.md"

var (
	deferralHeadingRe = regexp.MustCompile(`^### (D[0-9]+):\s+(.+)$`)
	deferralIDRe      = regexp.MustCompile(`^D([0-9]+)$`)
	dateRe            = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	dateInTextRe      = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}`)
)

type section string

const (
	sectionNone   section = ""
	sectionActive section = "active"
	sectionClosed section = "closed"
)

type config struct {
	Path string
	Now  time.Time
}

type deferral struct {
	ID      string
	Title   string
	Section section
	Line    int
	Fields  map[string]string
}

func main() {
	cfg := config{Now: time.Now().UTC()}
	flag.StringVar(&cfg.Path, "path", defaultDeferredFollowupsPath, "deferred follow-ups markdown path")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[deferred-followups] %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	if cfg.Now.IsZero() {
		return fmt.Errorf("current time is required")
	}
	entries, err := readDeferrals(cfg.Path)
	if err != nil {
		return err
	}
	problems := validateDeferrals(entries, cfg.Now.UTC())
	if len(problems) > 0 {
		sort.Strings(problems)
		fmt.Fprintln(stdout, "[deferred-followups] ledger violations:")
		for _, problem := range problems {
			fmt.Fprintf(stdout, "  %s\n", problem)
		}
		return fmt.Errorf("found %d deferred follow-up violation(s)", len(problems))
	}

	open, closed := countStatuses(entries)
	fmt.Fprintf(stdout, "[deferred-followups] OK (%d deferrals: %d open, %d closed)\n", len(entries), open, closed)
	return nil
}

func readDeferrals(path string) ([]deferral, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: deferred follow-up ledger must be a regular file", path)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- repository-owned checker input.
	if err != nil {
		return nil, err
	}
	return parseDeferrals(string(raw)), nil
}

func parseDeferrals(contents string) []deferral {
	lines := strings.Split(contents, "\n")
	currentSection := sectionNone
	var entries []deferral
	var current *deferral

	flush := func() {
		if current != nil {
			entries = append(entries, *current)
			current = nil
		}
	}

	for i, line := range lines {
		lineNumber := i + 1
		switch strings.TrimSpace(line) {
		case "## Active Deferrals":
			flush()
			currentSection = sectionActive
			continue
		case "## Closed Deferrals":
			flush()
			currentSection = sectionClosed
			continue
		case "## Changelog":
			flush()
			currentSection = sectionNone
			continue
		}

		if currentSection == sectionNone {
			continue
		}
		if match := deferralHeadingRe.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 3 {
			flush()
			current = &deferral{
				ID:      match[1],
				Title:   strings.TrimSpace(match[2]),
				Section: currentSection,
				Line:    lineNumber,
				Fields:  map[string]string{},
			}
			continue
		}
		if current == nil {
			continue
		}
		cells, ok := splitMarkdownRow(line)
		if !ok || len(cells) < 2 || isSeparator(cells) || strings.EqualFold(cells[0], "Field") {
			continue
		}
		current.Fields[normalizeField(cells[0])] = strings.TrimSpace(cells[1])
	}
	flush()
	return entries
}

func validateDeferrals(entries []deferral, now time.Time) []string {
	var problems []string
	if len(entries) == 0 {
		return []string{"no deferral entries found"}
	}

	seen := map[string]deferral{}
	maxID := 0
	for _, entry := range entries {
		idNumber, ok := parseDeferralNumber(entry.ID)
		if !ok {
			problems = append(problems, problem(entry, "deferral id must be shaped as D<number>"))
		} else if idNumber > maxID {
			maxID = idNumber
		}
		if previous, exists := seen[entry.ID]; exists {
			problems = append(problems, problem(entry, fmt.Sprintf("duplicate id also declared at line %d", previous.Line)))
		}
		seen[entry.ID] = entry
		problems = append(problems, validateEntry(entry, now)...)
	}
	for id := 1; id <= maxID; id++ {
		key := fmt.Sprintf("D%d", id)
		if _, ok := seen[key]; !ok {
			problems = append(problems, fmt.Sprintf("%s: missing deferral id", key))
		}
	}
	return problems
}

func validateEntry(entry deferral, now time.Time) []string {
	var problems []string
	if entry.Title == "" || weakValue(entry.Title) {
		problems = append(problems, problem(entry, "title must be concrete"))
	}

	allowedFields := map[string]struct{}{
		"Decided": {},
		"Reason":  {},
		"Status":  {},
		"Target":  {},
		"Trigger": {},
		"Updated": {},
	}
	for field := range entry.Fields {
		if _, ok := allowedFields[field]; !ok {
			problems = append(problems, problem(entry, fmt.Sprintf("unknown field %q", field)))
		}
	}
	for _, field := range []string{"Status", "Decided", "Reason", "Trigger", "Target"} {
		value := strings.TrimSpace(entry.Fields[field])
		if value == "" {
			problems = append(problems, problem(entry, fmt.Sprintf("missing %s field", field)))
			continue
		}
		if weakValue(value) {
			problems = append(problems, problem(entry, fmt.Sprintf("%s field must be concrete", field)))
		}
	}

	status := canonicalStatus(entry.Fields["Status"])
	switch status {
	case "open":
		if entry.Section != sectionActive {
			problems = append(problems, problem(entry, "open deferral must live under Active Deferrals"))
		}
	case "closed":
		if entry.Section != sectionClosed {
			problems = append(problems, problem(entry, "closed deferral must live under Closed Deferrals"))
		}
	default:
		problems = append(problems, problem(entry, `Status must be exactly "open" or "closed"`))
	}

	decided, ok := parseDate(entry.Fields["Decided"])
	if !ok {
		problems = append(problems, problem(entry, "Decided field must be YYYY-MM-DD"))
	} else if decided.After(startOfDay(now)) {
		problems = append(problems, problem(entry, "Decided field must not be in the future"))
	}

	if updated := strings.TrimSpace(entry.Fields["Updated"]); updated != "" {
		updateDates := dateInTextRe.FindAllString(updated, -1)
		if len(updateDates) == 0 {
			problems = append(problems, problem(entry, "Updated field must contain at least one YYYY-MM-DD date"))
		}
		for _, value := range updateDates {
			parsed, dateOK := parseDate(value)
			if !dateOK {
				problems = append(problems, problem(entry, fmt.Sprintf("Updated date %q must be YYYY-MM-DD", value)))
				continue
			}
			if parsed.After(startOfDay(now)) {
				problems = append(problems, problem(entry, fmt.Sprintf("Updated date %q must not be in the future", value)))
			}
			if ok && parsed.Before(decided) {
				problems = append(problems, problem(entry, fmt.Sprintf("Updated date %q must not be before Decided", value)))
			}
		}
	}

	return problems
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
		if strings.Trim(cell, " :-") != "" {
			return false
		}
	}
	return true
}

func normalizeField(field string) string {
	field = strings.ReplaceAll(field, "`", "")
	field = strings.Trim(field, "* ")
	return strings.Join(strings.Fields(field), " ")
}

func canonicalStatus(status string) string {
	return strings.TrimSpace(status)
}

func parseDeferralNumber(id string) (int, bool) {
	match := deferralIDRe.FindStringSubmatch(id)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	return value, err == nil && value > 0
}

func parseDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if !dateRe.MatchString(value) {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return parsed, err == nil
}

func startOfDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func weakValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.Trim(normalized, ".")
	switch normalized {
	case "", "-", "n/a", "none", "placeholder", "tbd", "todo":
		return true
	default:
		return false
	}
}

func problem(entry deferral, message string) string {
	return fmt.Sprintf("%s line %d: %s", entry.ID, entry.Line, message)
}

func countStatuses(entries []deferral) (open, closed int) {
	for _, entry := range entries {
		switch canonicalStatus(entry.Fields["Status"]) {
		case "open":
			open++
		case "closed":
			closed++
		}
	}
	return open, closed
}
