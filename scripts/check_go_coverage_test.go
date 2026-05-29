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

func TestGoCoverageCheckFiltersGeneratedPackagesAndPasses(t *testing.T) {
	root := newGoCoverageFixture(t, fakeGoCoverageTool(`
github.com/openclarion/openclarion/api
github.com/openclarion/openclarion/cmd/openclarion
github.com/openclarion/openclarion/internal/persistence/ent
github.com/openclarion/openclarion/internal/persistence/ent/alertevent
github.com/openclarion/openclarion/internal/usecases/reporttrigger
github.com/openclarion/openclarion/scripts
github.com/openclarion/openclarion/scripts/report_live_smoke_output
`, `
ok  	github.com/openclarion/openclarion/cmd/openclarion	0.01s	coverage: 42.8% of statements
ok  	github.com/openclarion/openclarion/internal/usecases/reporttrigger	0.01s	coverage: 70.5% of statements
ok  	github.com/openclarion/openclarion/scripts/report_live_smoke_output	0.01s	coverage: 81.8% of statements
`))

	out, err := runGoCoverageCheck(t, root, "40.0")
	if err != nil {
		t.Fatalf("go coverage check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[go-coverage] OK (3 packages, min 40.0%)") {
		t.Fatalf("output = %q, want OK with 3 checked packages", out)
	}

	calls := readGoCoverageCalls(t, root)
	for _, excluded := range []string{
		"github.com/openclarion/openclarion/api",
		"github.com/openclarion/openclarion/internal/persistence/ent",
		"github.com/openclarion/openclarion/internal/persistence/ent/alertevent",
		"github.com/openclarion/openclarion/scripts ",
	} {
		if strings.Contains(calls, excluded) {
			t.Fatalf("go test calls = %q, must not include excluded package %q", calls, excluded)
		}
	}
}

func TestGoCoverageCheckRejectsCoverageBelowFloor(t *testing.T) {
	root := newGoCoverageFixture(t, fakeGoCoverageTool(`
github.com/openclarion/openclarion/internal/usecases/reporttrigger
`, `
ok  	github.com/openclarion/openclarion/internal/usecases/reporttrigger	0.01s	coverage: 39.9% of statements
`))

	out, err := runGoCoverageCheck(t, root, "40.0")
	if err == nil {
		t.Fatalf("go coverage check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"package coverage below 40.0%",
		"github.com/openclarion/openclarion/internal/usecases/reporttrigger coverage 39.9% < 40.0%",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, want substring %q", out, want)
		}
	}
}

func TestGoCoverageCheckRejectsInvalidThreshold(t *testing.T) {
	root := newGoCoverageFixture(t, fakeGoCoverageTool(`
github.com/openclarion/openclarion/internal/usecases/reporttrigger
`, `
ok  	github.com/openclarion/openclarion/internal/usecases/reporttrigger	0.01s	coverage: 70.5% of statements
`))

	out, err := runGoCoverageCheck(t, root, "forty")
	if err == nil {
		t.Fatalf("go coverage check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "GO_COVERAGE_MIN must be a non-negative number") {
		t.Fatalf("output = %q, want invalid threshold error", out)
	}
}

func newGoCoverageFixture(t *testing.T, fakeGo string) string {
	t.Helper()
	root := t.TempDir()
	goCoverageWriteFile(t, root, "scripts/check_go_coverage.sh", goCoverageScript(t), 0o750)
	goCoverageWriteFile(t, root, "bin/go", fakeGo, 0o750)
	return root
}

func goCoverageScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_go_coverage.sh")
	if err != nil {
		t.Fatalf("read go coverage script: %v", err)
	}
	return string(raw)
}

func fakeGoCoverageTool(packages, coverageOutput string) string {
	return `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "list" ]]; then
  cat <<'EOF'
` + strings.TrimSpace(packages) + `
EOF
  exit 0
fi
if [[ "$1" == "test" ]]; then
  printf '%s\n' "$*" >>go-coverage-calls.txt
  cat <<'EOF'
` + strings.TrimSpace(coverageOutput) + `
EOF
  exit 0
fi
echo "unexpected go command: $*" >&2
exit 42
`
}

func goCoverageWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGoCoverageCheck(t *testing.T, root, threshold string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_go_coverage.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GO_COVERAGE_MIN="+threshold,
	)
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}

func readGoCoverageCalls(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "go-coverage-calls.txt")) // #nosec G304 -- test reads a temp file it created.
	if err != nil {
		t.Fatalf("read fake go calls: %v", err)
	}
	return string(raw)
}
