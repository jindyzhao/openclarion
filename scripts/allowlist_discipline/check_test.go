package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCheckAllowlistFileRequiresAdjacentGovernanceMetadata(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	valid := `# Owner: openclarion CI maintainers.
# Expires: 2026-08-31.
# Removal trigger: delete this fixture exception after the value changes.
[[rules.allowlists]]
description = "fixture"
`
	findings, entries := checkAllowlistFile(gitleaksSpec, valid, now)
	if entries != 1 {
		t.Fatalf("entries = %d, want 1", entries)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestCheckAllowlistFileRejectsMissingMetadata(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{
			name: "missing owner",
			contents: `# Expires: 2026-08-31.
# Removal trigger: delete after fixture changes.
[[rules.allowlists]]
`,
			want: "Owner",
		},
		{
			name: "missing expiry",
			contents: `# Owner: openclarion CI maintainers.
# Removal trigger: delete after fixture changes.
[[rules.allowlists]]
`,
			want: "Expires",
		},
		{
			name: "missing removal trigger",
			contents: `# Owner: openclarion CI maintainers.
# Expires: 2026-08-31.
[[rules.allowlists]]
`,
			want: "Removal trigger",
		},
		{
			name: "detached metadata",
			contents: `# Owner: openclarion CI maintainers.
# Expires: 2026-08-31.
# Removal trigger: delete after fixture changes.

[[rules.allowlists]]
`,
			want: "Owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings, _ := checkAllowlistFile(gitleaksSpec, tt.contents, now)
			if len(findings) == 0 {
				t.Fatal("findings = none, want violation")
			}
			if !strings.Contains(joinFindings(findings), tt.want) {
				t.Fatalf("findings = %#v, want substring %q", findings, tt.want)
			}
		})
	}
}

func TestCheckAllowlistFileRejectsExpiredMetadata(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	contents := `# Owner: openclarion CI maintainers.
# Expires: 2026-05-28.
# Removal trigger: delete after fixture changes.
[[rules.allowlists]]
`

	findings, _ := checkAllowlistFile(gitleaksSpec, contents, now)
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want one expired finding", findings)
	}
	if !strings.Contains(findings[0].Msg, "expired") {
		t.Fatalf("finding = %#v, want expired message", findings[0])
	}
}

func TestCheckAllowlistFileRejectsExpiryBeyondReviewHorizon(t *testing.T) {
	now := time.Date(2026, 5, 29, 14, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	contents := `# Owner: openclarion CI maintainers.
# Expires: 2026-09-27.
# Removal trigger: delete after fixture changes.
[[rules.allowlists]]
`

	findings, _ := checkAllowlistFile(gitleaksSpec, contents, now)
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want one horizon finding", findings)
	}
	if !strings.Contains(findings[0].Msg, "more than 120 days") {
		t.Fatalf("finding = %#v, want horizon message", findings[0])
	}
}

func TestCheckAllowlistFileAcceptsExpiryAtReviewHorizon(t *testing.T) {
	now := time.Date(2026, 5, 29, 23, 59, 0, 0, time.UTC)
	contents := `# Owner: openclarion CI maintainers.
# Expires: 2026-09-26.
# Removal trigger: delete after fixture changes.
[[rules.allowlists]]
`

	findings, entries := checkAllowlistFile(gitleaksSpec, contents, now)
	if entries != 1 {
		t.Fatalf("entries = %d, want 1", entries)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestCheckAllowlistFileAcceptsExpiryToday(t *testing.T) {
	now := time.Date(2026, 5, 29, 23, 59, 0, 0, time.UTC)
	contents := `# Owner: openclarion CI maintainers.
# Expires: 2026-05-29.
# Removal trigger: delete after fixture changes.
[[rules.allowlists]]
`

	findings, entries := checkAllowlistFile(gitleaksSpec, contents, now)
	if entries != 1 {
		t.Fatalf("entries = %d, want 1", entries)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestRunPropagatesReadErrors(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]allowlistSpec{gitleaksSpec}, time.Now(), func(string) ([]byte, error) {
		return nil, errors.New("permission denied")
	}, &stderr)
	if code != 2 {
		t.Fatalf("run code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "permission denied") {
		t.Fatalf("stderr = %q, want read error", stderr.String())
	}
}

func TestRunSkipsMissingFiles(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]allowlistSpec{gitleaksSpec}, time.Now(), func(string) ([]byte, error) {
		return nil, os.ErrNotExist
	}, &stderr)
	if code != 0 {
		t.Fatalf("run code = %d, want 0", code)
	}
}

func joinFindings(findings []finding) string {
	var b strings.Builder
	for _, finding := range findings {
		b.WriteString(finding.Msg)
		b.WriteByte('\n')
	}
	return b.String()
}
