package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunAcceptsCompleteLedger(t *testing.T) {
	path := writeDeferredFollowupsFile(t, t.TempDir(), validLedger())

	var stdout bytes.Buffer
	err := run(config{Path: path, Now: fixedDeferredNow()}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[deferred-followups] OK (2 deferrals: 1 open, 1 closed)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsMissingRequiredFields(t *testing.T) {
	path := writeDeferredFollowupsFile(t, t.TempDir(), strings.Replace(validLedger(), "| Trigger | revisit when deployment requires it |\n", "", 1))

	err := run(config{Path: path, Now: fixedDeferredNow()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "found 1 deferred follow-up violation") {
		t.Fatalf("run error = %v, want one violation", err)
	}
}

func TestValidateRejectsStatusSectionMismatch(t *testing.T) {
	path := writeDeferredFollowupsFile(t, t.TempDir(), strings.Replace(validLedger(), "| Status | open |", "| Status | closed |", 1))

	var stdout bytes.Buffer
	err := run(config{Path: path, Now: fixedDeferredNow()}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(stdout.String(), "closed deferral must live under Closed Deferrals") {
		t.Fatalf("stdout = %q, want section mismatch", stdout.String())
	}
}

func TestValidateRejectsNonCanonicalStatusCasing(t *testing.T) {
	path := writeDeferredFollowupsFile(t, t.TempDir(), strings.Replace(validLedger(), "| Status | open |", "| Status | Open |", 1))

	var stdout bytes.Buffer
	err := run(config{Path: path, Now: fixedDeferredNow()}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(stdout.String(), `Status must be exactly "open" or "closed"`) {
		t.Fatalf("stdout = %q, want exact status casing failure", stdout.String())
	}
}

func TestValidateRejectsDuplicateAndMissingIDs(t *testing.T) {
	body := strings.Replace(validLedger(), "### D2: Closed Item", "### D3: Closed Item", 1)
	body = strings.Replace(body, "## Changelog", `
### D3: Duplicate Item

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-23 |
| Reason | Completed by a later implementation. |
| Trigger | closed by implementation evidence |
| Target | M2 |

## Changelog`, 1)
	path := writeDeferredFollowupsFile(t, t.TempDir(), body)

	var stdout bytes.Buffer
	err := run(config{Path: path, Now: fixedDeferredNow()}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{"D2: missing deferral id", "duplicate id also declared"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestValidateRejectsWeakFieldsUnknownFieldsAndBadDates(t *testing.T) {
	body := strings.Replace(validLedger(), "| Reason | The work is real but intentionally deferred. |", "| Reason | TBD |", 1)
	body = strings.Replace(body, "| Decided | 2026-05-20 |", "| Decided | 2026-99-99 |", 1)
	body = strings.Replace(body, "| Target | post-V1 |", "| Target | post-V1 |\n| Owner | nobody |", 1)
	path := writeDeferredFollowupsFile(t, t.TempDir(), body)

	var stdout bytes.Buffer
	err := run(config{Path: path, Now: fixedDeferredNow()}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{"Reason field must be concrete", "Decided field must be YYYY-MM-DD", `unknown field "Owner"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestValidateRejectsFutureAndPreDecisionUpdates(t *testing.T) {
	body := strings.Replace(validLedger(), "| Updated | 2026-05-22 (closed) |", "| Updated | 2026-05-18 (before), 2026-06-01 (future) |", 1)
	path := writeDeferredFollowupsFile(t, t.TempDir(), body)

	var stdout bytes.Buffer
	err := run(config{Path: path, Now: fixedDeferredNow()}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{`Updated date "2026-05-18" must not be before Decided`, `Updated date "2026-06-01" must not be in the future`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunRejectsSymlinkLedger(t *testing.T) {
	root := t.TempDir()
	target := writeDeferredFollowupsFile(t, root, validLedger())
	link := filepath.Join(root, "linked.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := run(config{Path: link, Now: fixedDeferredNow()}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "deferred follow-up ledger must be a regular file") {
		t.Fatalf("run error = %v, want regular file rejection", err)
	}
}

func fixedDeferredNow() time.Time {
	return time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
}

func validLedger() string {
	return `# Deferred Follow-ups

## Active Deferrals

### D1: Open Item

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-20 |
| Reason | The work is real but intentionally deferred. |
| Trigger | revisit when deployment requires it |
| Target | post-V1 |

## Closed Deferrals

### D2: Closed Item

| Field | Value |
|-------|-------|
| Status | closed |
| Decided | 2026-05-21 |
| Updated | 2026-05-22 (closed) |
| Reason | Completed by a later implementation. |
| Trigger | closed by implementation evidence |
| Target | M2 |

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-22 | jindyzhao | Closed D2 |
`
}

func writeDeferredFollowupsFile(t *testing.T, root, body string) string {
	t.Helper()
	path := filepath.Join(root, "DEFERRED_FOLLOWUPS.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
