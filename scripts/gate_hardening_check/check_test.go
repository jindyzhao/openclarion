package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsCompleteActivatedGateRecords(t *testing.T) {
	dir := t.TempDir()
	schedule := writeGateHardeningFile(t, dir, "ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |
| Manual smoke | M4 | landed / manual | make smoke |
| Future gate | M9 | planned | not active yet |

## Current Private-Incubation Gate
`)
	checklist := writeGateHardeningFile(t, dir, "checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
| Manual smoke | manual | make smoke retained proof | Promote only with real evidence |
`)

	var stdout bytes.Buffer
	if err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[gate-hardening] OK (2 activated gates audited)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsMissingActivatedGateRecord(t *testing.T) {
	dir := t.TempDir()
	schedule := writeGateHardeningFile(t, dir, "ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |
| Missing gate | M1 | replaced | analyzer |

## Current Private-Incubation Gate
`)
	checklist := writeGateHardeningFile(t, dir, "checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
`)

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil || !strings.Contains(err.Error(), `missing maturity record for schedule gate "Missing gate"`) {
		t.Fatalf("run error = %v, want missing gate", err)
	}
}

func TestRunRejectsStaleRecord(t *testing.T) {
	dir := t.TempDir()
	schedule := writeGateHardeningFile(t, dir, "ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |

## Current Private-Incubation Gate
`)
	checklist := writeGateHardeningFile(t, dir, "checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
| Old gate | baseline | old evidence | Remove stale rows |
`)

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil || !strings.Contains(err.Error(), `stale maturity record without activated schedule row "Old gate"`) {
		t.Fatalf("run error = %v, want stale row", err)
	}
}

func TestRunRejectsWeakRecordFields(t *testing.T) {
	dir := t.TempDir()
	schedule := writeGateHardeningFile(t, dir, "ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |

## Current Private-Incubation Gate
`)
	checklist := writeGateHardeningFile(t, dir, "checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | unknown | TBD | TODO |
`)

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{`unsupported maturity "unknown"`, `evidence must be concrete`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %v, want %q", err, want)
		}
	}
}

func TestRunRejectsSymlinkScheduleInput(t *testing.T) {
	dir := t.TempDir()
	target := writeGateHardeningFile(t, dir, "target-ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |

## Current Private-Incubation Gate
`)
	schedule := filepath.Join(dir, "ci.md")
	if err := os.Symlink(target, schedule); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	checklist := writeGateHardeningFile(t, dir, "checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
`)

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "ci.md must be a regular file, not a symlink") {
		t.Fatalf("run error = %v, want symlink schedule rejection", err)
	}
}

func TestRunRejectsSymlinkScheduleParentInput(t *testing.T) {
	dir := t.TempDir()
	target := writeGateHardeningFile(t, dir, "target/ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |

## Current Private-Incubation Gate
`)
	scheduleDir := filepath.Join(dir, "schedule-dir")
	if err := os.Symlink(filepath.Dir(target), scheduleDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	schedule := filepath.Join(scheduleDir, "ci.md")
	checklist := writeGateHardeningFile(t, dir, "checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
`)

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{"parent directory", "must not be a symlink"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %v, want %q", err, want)
		}
	}
}

func TestRunRejectsNonDirectoryScheduleParentInput(t *testing.T) {
	dir := t.TempDir()
	scheduleParent := writeGateHardeningFile(t, dir, "schedule-parent", "not a directory")
	schedule := filepath.Join(scheduleParent, "ci.md")
	checklist := writeGateHardeningFile(t, dir, "checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
`)

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{"parent path", "must be a directory"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %v, want %q", err, want)
		}
	}
}

func TestRunRejectsSymlinkChecklistInput(t *testing.T) {
	dir := t.TempDir()
	schedule := writeGateHardeningFile(t, dir, "ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |

## Current Private-Incubation Gate
`)
	target := writeGateHardeningFile(t, dir, "target-checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
`)
	checklist := filepath.Join(dir, "checklist.md")
	if err := os.Symlink(target, checklist); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "checklist.md must be a regular file, not a symlink") {
		t.Fatalf("run error = %v, want symlink checklist rejection", err)
	}
}

func TestRunRejectsSymlinkChecklistParentInput(t *testing.T) {
	dir := t.TempDir()
	schedule := writeGateHardeningFile(t, dir, "ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |

## Current Private-Incubation Gate
`)
	target := writeGateHardeningFile(t, dir, "target/checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
`)
	checklistDir := filepath.Join(dir, "checklist-dir")
	if err := os.Symlink(filepath.Dir(target), checklistDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	checklist := filepath.Join(checklistDir, "checklist.md")

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{"parent directory", "must not be a symlink"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %v, want %q", err, want)
		}
	}
}

func TestRunRejectsNonDirectoryChecklistParentInput(t *testing.T) {
	dir := t.TempDir()
	schedule := writeGateHardeningFile(t, dir, "ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |

## Current Private-Incubation Gate
`)
	checklistParent := writeGateHardeningFile(t, dir, "checklist-parent", "not a directory")
	checklist := filepath.Join(checklistParent, "checklist.md")

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{"parent path", "must be a directory"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %v, want %q", err, want)
		}
	}
}

func TestRunRejectsNonRegularScheduleInput(t *testing.T) {
	dir := t.TempDir()
	schedule := filepath.Join(dir, "ci.md")
	if err := os.MkdirAll(schedule, 0o750); err != nil {
		t.Fatalf("mkdir schedule path: %v", err)
	}
	checklist := writeGateHardeningFile(t, dir, "checklist.md", `# Checklist

## Gate Maturity Records

| Gate | Maturity | Evidence | Next hardening |
|------|----------|----------|----------------|
| Docs gate | hardened | make docs plus fixture tests | Add negative fixture when scope expands |
`)

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "ci.md must be a regular file") {
		t.Fatalf("run error = %v, want non-regular schedule rejection", err)
	}
}

func TestRunRejectsNonRegularChecklistInput(t *testing.T) {
	dir := t.TempDir()
	schedule := writeGateHardeningFile(t, dir, "ci.md", `# CI

## Progressive Gate Schedule

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| Docs gate | M0 | landed | make docs |

## Current Private-Incubation Gate
`)
	checklist := filepath.Join(dir, "checklist.md")
	if err := os.MkdirAll(checklist, 0o750); err != nil {
		t.Fatalf("mkdir checklist path: %v", err)
	}

	var stdout bytes.Buffer
	err := run(config{SchedulePath: schedule, ChecklistPath: checklist}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "checklist.md must be a regular file") {
		t.Fatalf("run error = %v, want non-regular checklist rejection", err)
	}
}

func writeGateHardeningFile(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
