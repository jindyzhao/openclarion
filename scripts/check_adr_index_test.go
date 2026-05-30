package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestADRCheckRequiresConsequencesSection(t *testing.T) {
	tests := []struct {
		name    string
		adrBody string
		wantOK  bool
		want    []string
	}{
		{
			name: "non empty consequences section",
			adrBody: adrFixture(`### Consequences

* Good, because the decision tradeoffs are explicit.
`),
			wantOK: true,
			want:   []string{"[adr-check] OK"},
		},
		{
			name:    "missing consequences section",
			adrBody: adrFixture("### Confirmation\n\n* checked locally\n"),
			want: []string{
				"missing a non-empty 'Consequences' section",
				"ADR-0001-test.md",
			},
		},
		{
			name: "empty consequences section",
			adrBody: adrFixture(`### Consequences

### Confirmation

* checked locally
`),
			want: []string{
				"missing a non-empty 'Consequences' section",
				"ADR-0001-test.md",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newADRCheckRepo(t, tc.adrBody)

			out, err := runADRCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("adr-check failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("adr-check passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("adr-check output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestADRCheckValidatesFrontMatterSchema(t *testing.T) {
	tests := []struct {
		name    string
		adrBody string
		want    []string
	}{
		{
			name: "missing id",
			adrBody: strings.Replace(
				adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"),
				"id: ADR-0001\n",
				"",
				1,
			),
			want: []string{"missing front-matter 'id'"},
		},
		{
			name: "unknown key",
			adrBody: strings.Replace(
				adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"),
				"informed: []\n",
				"informed: []\nowner: test\n",
				1,
			),
			want: []string{"unknown front-matter key 'owner'"},
		},
		{
			name: "title mismatch",
			adrBody: strings.Replace(
				adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"),
				`title: "Test"`,
				`title: "Wrong"`,
				1,
			),
			want: []string{"front-matter title 'Wrong' does not match H1 title 'Test'"},
		},
		{
			name: "invalid date",
			adrBody: strings.Replace(
				adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"),
				"date: 2026-05-29",
				"date: 2026/05/29",
				1,
			),
			want: []string{"front-matter date '2026/05/29' must be YYYY-MM-DD"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newADRCheckRepo(t, tc.adrBody)

			out, err := runADRCheck(t, root)
			if err == nil {
				t.Fatalf("adr-check passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("adr-check output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestADRCheckRejectsIndirectADRInputs(t *testing.T) {
	t.Run("adr directory symlink", func(t *testing.T) {
		root := newADRCheckRepo(t, adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"))
		if err := os.Rename(filepath.Join(root, "docs", "adr"), filepath.Join(root, "docs", "adr-real")); err != nil {
			t.Fatalf("rename docs/adr: %v", err)
		}
		if err := os.Symlink("adr-real", filepath.Join(root, "docs", "adr")); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}

		out, err := runADRCheck(t, root)
		if err == nil {
			t.Fatalf("adr-check passed unexpectedly:\n%s", out)
		}
		if !strings.Contains(out, "docs/adr must be a directory, not a symlink") {
			t.Fatalf("adr-check output = %q, want ADR directory symlink rejection", out)
		}
	})

	t.Run("readme symlink", func(t *testing.T) {
		root := newADRCheckRepo(t, adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"))
		adrCheckWriteFile(t, root, "docs/adr/README-target.md", "# linked index\n", 0o644)
		replaceWithSymlink(t, root, "docs/adr/README-target.md", "docs/adr/README.md")

		out, err := runADRCheck(t, root)
		if err == nil {
			t.Fatalf("adr-check passed unexpectedly:\n%s", out)
		}
		if !strings.Contains(out, "docs/adr/README.md must be a regular file, not a symlink") {
			t.Fatalf("adr-check output = %q, want README symlink rejection", out)
		}
	})

	t.Run("readme directory", func(t *testing.T) {
		root := newADRCheckRepo(t, adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"))
		readmePath := filepath.Join(root, "docs", "adr", "README.md")
		if err := os.Remove(readmePath); err != nil {
			t.Fatalf("remove README.md: %v", err)
		}
		if err := os.Mkdir(readmePath, 0o750); err != nil {
			t.Fatalf("mkdir README.md: %v", err)
		}

		out, err := runADRCheck(t, root)
		if err == nil {
			t.Fatalf("adr-check passed unexpectedly:\n%s", out)
		}
		if !strings.Contains(out, "docs/adr/README.md must be a regular file") {
			t.Fatalf("adr-check output = %q, want README non-regular rejection", out)
		}
	})

	t.Run("adr symlink", func(t *testing.T) {
		root := newADRCheckRepo(t, adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"))
		adrCheckWriteFile(t, root, "docs/adr/target.md", adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"), 0o644)
		replaceWithSymlink(t, root, "docs/adr/target.md", "docs/adr/ADR-0001-test.md")

		out, err := runADRCheck(t, root)
		if err == nil {
			t.Fatalf("adr-check passed unexpectedly:\n%s", out)
		}
		if !strings.Contains(out, "docs/adr/ADR-0001-test.md must be a regular file, not a symlink") {
			t.Fatalf("adr-check output = %q, want ADR symlink rejection", out)
		}
	})

	t.Run("adr directory", func(t *testing.T) {
		root := newADRCheckRepo(t, adrFixture("### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n"))
		adrPath := filepath.Join(root, "docs", "adr", "ADR-0001-test.md")
		if err := os.Remove(adrPath); err != nil {
			t.Fatalf("remove ADR file: %v", err)
		}
		if err := os.Mkdir(adrPath, 0o750); err != nil {
			t.Fatalf("mkdir ADR path: %v", err)
		}

		out, err := runADRCheck(t, root)
		if err == nil {
			t.Fatalf("adr-check passed unexpectedly:\n%s", out)
		}
		if !strings.Contains(out, "docs/adr/ADR-0001-test.md must be a regular file") {
			t.Fatalf("adr-check output = %q, want ADR non-regular rejection", out)
		}
	})
}

func TestADRCheckValidatesSupersedesClosure(t *testing.T) {
	tests := []struct {
		name   string
		adrs   []adrCheckFixture
		wantOK bool
		want   []string
	}{
		{
			name: "closed supersedes reference",
			adrs: []adrCheckFixture{
				{
					id:     "ADR-0001",
					file:   "ADR-0001-old.md",
					title:  "Old",
					status: "superseded",
					extra:  "superseded_by: ADR-0002\n",
				},
				{
					id:     "ADR-0002",
					file:   "ADR-0002-new.md",
					title:  "New",
					status: "proposed",
					extra:  "supersedes: ADR-0001\n",
				},
			},
			wantOK: true,
			want:   []string{"[adr-check] OK"},
		},
		{
			name: "missing superseded target",
			adrs: []adrCheckFixture{
				{
					id:     "ADR-0001",
					file:   "ADR-0001-new.md",
					title:  "New",
					status: "proposed",
					extra:  "supersedes: ADR-0002\n",
				},
			},
			want: []string{"supersedes 'ADR-0002', but that ADR does not exist"},
		},
		{
			name: "target not superseded",
			adrs: []adrCheckFixture{
				{id: "ADR-0001", file: "ADR-0001-old.md", title: "Old", status: "proposed"},
				{id: "ADR-0002", file: "ADR-0002-new.md", title: "New", status: "proposed", extra: "supersedes: ADR-0001\n"},
			},
			want: []string{"status is 'proposed', expected 'superseded'"},
		},
		{
			name: "missing back pointer",
			adrs: []adrCheckFixture{
				{id: "ADR-0001", file: "ADR-0001-old.md", title: "Old", status: "superseded"},
				{id: "ADR-0002", file: "ADR-0002-new.md", title: "New", status: "proposed", extra: "supersedes: ADR-0001\n"},
			},
			want: []string{
				"has status 'superseded' but no superseded_by back-pointer",
				"does not declare superseded_by: ADR-0002",
			},
		},
		{
			name: "superseded by on non superseded adr",
			adrs: []adrCheckFixture{
				{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "proposed", extra: "superseded_by: ADR-0002\n"},
			},
			want: []string{"declares superseded_by but status is 'proposed', expected 'superseded'"},
		},
		{
			name: "invalid reference format",
			adrs: []adrCheckFixture{
				{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "proposed", extra: "supersedes: 1\n"},
			},
			want: []string{"front-matter 'supersedes' reference '1' must be ADR-NNNN"},
		},
		{
			name: "array reference format",
			adrs: []adrCheckFixture{
				{id: "ADR-0001", file: "ADR-0001-old.md", title: "Old", status: "superseded", extra: "superseded_by: [ADR-0003]\n"},
				{id: "ADR-0002", file: "ADR-0002-other.md", title: "Other", status: "superseded", extra: "superseded_by: [ADR-0003]\n"},
				{id: "ADR-0003", file: "ADR-0003-new.md", title: "New", status: "proposed", extra: "supersedes: [ADR-0001, ADR-0002]\n"},
			},
			wantOK: true,
			want:   []string{"[adr-check] OK"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newADRCheckRepoWithFixtures(t, tc.adrs)

			out, err := runADRCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("adr-check failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("adr-check passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("adr-check output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestADRCheckRejectsAcceptedADRBodyChanges(t *testing.T) {
	tests := []struct {
		name       string
		baseADR    adrCheckFixture
		currentADR adrCheckFixture
		bodyEdit   func(string) string
		wantOK     bool
		want       []string
	}{
		{
			name:       "accepted body unchanged",
			baseADR:    adrCheckFixture{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "accepted"},
			currentADR: adrCheckFixture{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "accepted", extra: "supersedes: []\n"},
			wantOK:     true,
			want:       []string{"[adr-check] OK"},
		},
		{
			name:       "accepted body changed",
			baseADR:    adrCheckFixture{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "accepted"},
			currentADR: adrCheckFixture{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "accepted"},
			bodyEdit: func(body string) string {
				return strings.Replace(body, "Test context.", "Changed context.", 1)
			},
			want: []string{"accepted ADR body changed: docs/adr/ADR-0001-test.md"},
		},
		{
			name:       "proposed body changed",
			baseADR:    adrCheckFixture{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "proposed"},
			currentADR: adrCheckFixture{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "proposed"},
			bodyEdit: func(body string) string {
				return strings.Replace(body, "Test context.", "Changed context.", 1)
			},
			wantOK: true,
			want:   []string{"[adr-check] OK"},
		},
		{
			name:       "unresolved base ref",
			baseADR:    adrCheckFixture{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "accepted"},
			currentADR: adrCheckFixture{id: "ADR-0001", file: "ADR-0001-test.md", title: "Test", status: "accepted"},
			want:       []string{"ADR_BASE_REF 'missing-ref' could not be resolved"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newADRCheckRepoWithFixtures(t, []adrCheckFixture{tc.baseADR})
			gitCommitAll(t, root, "base")

			current := adrFixtureWithMeta(tc.currentADR, "### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n")
			if tc.bodyEdit != nil {
				current = tc.bodyEdit(current)
			}
			adrCheckWriteFile(t, root, "docs/adr/"+tc.currentADR.file, current, 0o644)

			baseRef := "HEAD"
			if tc.name == "unresolved base ref" {
				baseRef = "missing-ref"
			}
			out, err := runADRCheckWithEnv(t, root, map[string]string{"ADR_BASE_REF": baseRef})
			if tc.wantOK && err != nil {
				t.Fatalf("adr-check failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("adr-check passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("adr-check output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func adrFixture(section string) string {
	return `---
id: ADR-0001
title: "Test"
status: "proposed"
date: 2026-05-29
deciders: ["test"]
consulted: []
informed: []
---

# ADR-0001: Test

## Context and Problem Statement

Test context.

## Decision Outcome

**Chosen option**: Test.

` + section + `

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-29 | test | Initial proposal |
`
}

type adrCheckFixture struct {
	id     string
	file   string
	title  string
	status string
	extra  string
}

func adrFixtureWithMeta(fixture adrCheckFixture, section string) string {
	return `---
id: ` + fixture.id + `
title: "` + fixture.title + `"
status: "` + fixture.status + `"
date: 2026-05-29
deciders: ["test"]
consulted: []
informed: []
` + fixture.extra + `---

# ` + fixture.id + `: ` + fixture.title + `

## Context and Problem Statement

Test context.

## Decision Outcome

**Chosen option**: Test.

` + section + `

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-29 | test | Initial proposal |
`
}

func newADRCheckRepo(t *testing.T, adrBody string) string {
	t.Helper()

	root := t.TempDir()
	adrCheckWriteFile(t, root, "scripts/check_adr_index.sh", adrCheckScript(t), 0o750)
	adrCheckWriteFile(t, root, "docs/adr/README.md", `# Architecture Decision Records

## ADR Index

| ID | Title | Status |
|----|-------|--------|
| [ADR-0001](ADR-0001-test.md) | Test | Proposed |
`, 0o644)
	adrCheckWriteFile(t, root, "docs/adr/ADR-0001-test.md", adrBody, 0o644)
	return root
}

func newADRCheckRepoWithFixtures(t *testing.T, adrs []adrCheckFixture) string {
	t.Helper()

	root := t.TempDir()
	adrCheckWriteFile(t, root, "scripts/check_adr_index.sh", adrCheckScript(t), 0o750)

	readme := `# Architecture Decision Records

## ADR Index

| ID | Title | Status |
|----|-------|--------|
`
	for _, adr := range adrs {
		readme += "| [" + adr.id + "](" + adr.file + ") | " + adr.title + " | " + adr.status + " |\n"
		body := adrFixtureWithMeta(adr, "### Consequences\n\n* Good, because the decision tradeoffs are explicit.\n")
		adrCheckWriteFile(t, root, "docs/adr/"+adr.file, body, 0o644)
	}
	adrCheckWriteFile(t, root, "docs/adr/README.md", readme, 0o644)
	return root
}

func adrCheckScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_adr_index.sh")
	if err != nil {
		t.Fatalf("read adr-check script: %v", err)
	}
	return string(raw)
}

func adrCheckWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func replaceWithSymlink(t *testing.T, root, targetName, linkName string) {
	t.Helper()
	linkPath := filepath.Join(root, filepath.FromSlash(linkName))
	targetPath := filepath.Join(root, filepath.FromSlash(targetName))
	if err := os.Remove(linkPath); err != nil {
		t.Fatalf("remove %s: %v", linkName, err)
	}
	relTarget, err := filepath.Rel(filepath.Dir(linkPath), targetPath)
	if err != nil {
		t.Fatalf("relative symlink target: %v", err)
	}
	if err := os.Symlink(relTarget, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runADRCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	return runADRCheckWithEnv(t, root, nil)
}

func runADRCheckWithEnv(t *testing.T, root string, env map[string]string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_adr_index.sh")
	cmd.Dir = root
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}

func gitCommitAll(t *testing.T, root string, message string) {
	t.Helper()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "OpenClarion Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", message)
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// #nosec G204 -- tests invoke git with helper-owned arguments against
	// temporary repositories only.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, raw)
	}
}
