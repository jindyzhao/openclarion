package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPRCIRunsPreflightBeforeConcurrentLanes(t *testing.T) {
	root := prParallelFixture(t, false)
	out, err := runPRParallelFixture(t, root)
	if err != nil {
		t.Fatalf("ci-pr failed: %v\n%s", err, out)
	}
	if strings.Contains(strings.ToLower(out), "warning:") {
		t.Fatalf("ci-pr emitted a warning:\n%s", out)
	}
	for _, name := range []string{"preflight.done", "backend.done", "frontend.done"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s not produced: %v", name, err)
		}
	}
	for _, name := range []string{"backend.makeflags", "frontend.makeflags"} {
		raw, err := os.ReadFile(filepath.Join(root, name)) // #nosec G304 -- test reads its own fixture output.
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !makeflagsEnableLineOutputSync(string(raw)) {
			t.Fatalf("%s = %q, want line-level output synchronization", name, raw)
		}
	}
}

func TestPRCIPropagatesLaneFailure(t *testing.T) {
	root := prParallelFixture(t, true)
	out, err := runPRParallelFixture(t, root)
	if err == nil {
		t.Fatalf("ci-pr passed despite backend failure:\n%s", out)
	}
	if !strings.Contains(out, "backend fixture failed") {
		t.Fatalf("ci-pr output = %q, want backend failure", out)
	}
}

func TestPRCILanesMatchSequentialCITargets(t *testing.T) {
	makefile := prParallelRepoMakefile(t)
	sequential := explicitMakeRulePrerequisites(t, makefile, "ci")
	root := t.TempDir()
	body := fmt.Sprintf(`include %s

.PHONY: fixture-print-ci-targets

fixture-print-ci-targets:
	@printf '%%s\n' "$(CI_TARGETS)"
`, makeIncludePath(makefile))
	if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture Makefile: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "make", "-s", "fixture-print-ci-targets")
	cmd.Env = isolatedMakeEnvironment()
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("print CI target partition: %v\n%s", err, raw)
	}
	partitioned := strings.Fields(string(raw))
	if !slices.Equal(partitioned, sequential) {
		t.Fatalf("CI target partition = %q, want sequential prerequisites %q", partitioned, sequential)
	}
}

func prParallelFixture(t *testing.T, failBackend bool) string {
	t.Helper()
	makefile := prParallelRepoMakefile(t)
	root := t.TempDir()
	backendResult := "touch backend.done"
	if failBackend {
		backendResult = "echo 'backend fixture failed' >&2; exit 9"
	}
	body := fmt.Sprintf(`include %s

.PHONY: fixture-preflight fixture-backend fixture-frontend

fixture-preflight:
	@touch preflight.done

fixture-backend:
	@test -f preflight.done
	@printf '%%s\n' "$(MAKEFLAGS)" > backend.makeflags
	@touch backend.started
	@for ((attempt = 0; attempt < 50; attempt++)); do test -f frontend.started && break; sleep 0.1; done
	@test -f frontend.started
	@%s

fixture-frontend:
	@test -f preflight.done
	@printf '%%s\n' "$(MAKEFLAGS)" > frontend.makeflags
	@touch frontend.started
	@for ((attempt = 0; attempt < 50; attempt++)); do test -f backend.started && break; sleep 0.1; done
	@test -f backend.started
	@touch frontend.done
`, makeIncludePath(makefile), backendResult)
	if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture Makefile: %v", err)
	}
	return root
}

func makeflagsEnableLineOutputSync(raw string) bool {
	for _, flag := range strings.Fields(raw) {
		if flag == "-Oline" || flag == "--output-sync=line" {
			return true
		}
	}
	return false
}

func runPRParallelFixture(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		"make",
		"ci-pr",
		"CI_PREFLIGHT_TARGETS=fixture-preflight",
		"CI_BACKEND_TARGETS=fixture-backend",
		"CI_FRONTEND_TARGETS=fixture-frontend",
	)
	cmd.Env = isolatedMakeEnvironment()
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}

func isolatedMakeEnvironment() []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GNUMAKEFLAGS", "MAKEFLAGS", "MFLAGS", "MAKELEVEL":
			// Fixture makes are independent top-level invocations and cannot use
			// jobserver descriptors owned by a parent of the Go test process.
			continue
		default:
			env = append(env, entry)
		}
	}
	return env
}

func prParallelRepoMakefile(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return filepath.Join(repoRoot, "Makefile")
}

func makeIncludePath(path string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		` `, `\ `,
		`#`, `\#`,
		`$`, `$$`,
	).Replace(filepath.ToSlash(path))
}

func explicitMakeRulePrerequisites(t *testing.T, makefile, target string) []string {
	t.Helper()
	raw, err := os.ReadFile(makefile) // #nosec G304 -- test reads the repository-owned Makefile resolved from its package root.
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	prefix := target + ":"
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		prerequisites := strings.TrimPrefix(line, prefix)
		if comment := strings.Index(prerequisites, "##"); comment >= 0 {
			prerequisites = prerequisites[:comment]
		}
		fields := strings.Fields(prerequisites)
		if len(fields) == 0 {
			t.Fatalf("Makefile target %q has no explicit prerequisites", target)
		}
		return fields
	}
	t.Fatalf("Makefile target %q not found", target)
	return nil
}
