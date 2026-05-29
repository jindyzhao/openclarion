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
