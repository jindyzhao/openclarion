package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtlasStartDevPostgresWaitsForTargetDatabase(t *testing.T) {
	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "docker.args")
	readinessPath := filepath.Join(tempDir, "readiness.count")
	dockerPath := filepath.Join(tempDir, "docker")
	fakeDocker := `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$OPENCLARION_TEST_DOCKER_CAPTURE"
if [ "${1:-}" != exec ]; then
  exit 0
fi
case " $* " in
  *" SELECT 1 "*)
    count=0
    if [ -f "$OPENCLARION_TEST_READINESS_COUNT" ]; then
      count="$(cat "$OPENCLARION_TEST_READINESS_COUNT")"
    fi
    count=$((count + 1))
    printf '%s\n' "$count" >"$OPENCLARION_TEST_READINESS_COUNT"
    [ "$count" -ge 2 ]
    ;;
  *" CREATE EXTENSION IF NOT EXISTS vector "*)
    [ "$(cat "$OPENCLARION_TEST_READINESS_COUNT")" -ge 2 ]
    ;;
esac
`
	if err := os.WriteFile(dockerPath, []byte(fakeDocker), 0o700); err != nil { // #nosec G306 -- executable test shim.
		t.Fatalf("write fake docker: %v", err)
	}
	sleepPath := filepath.Join(tempDir, "sleep")
	if err := os.WriteFile(sleepPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil { // #nosec G306 -- executable test shim.
		t.Fatalf("write fake sleep: %v", err)
	}

	cmd := exec.CommandContext(t.Context(), "bash", "-c", "source ./lib_atlas.sh; atlas::start_dev_pg")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"PATH="+tempDir+":"+os.Getenv("PATH"),
		"ATLAS_RUN_ID=readiness-test",
		"OPENCLARION_TEST_DOCKER_CAPTURE="+capturePath,
		"OPENCLARION_TEST_READINESS_COUNT="+readinessPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("start dev postgres: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(capturePath) // #nosec G304 -- test-owned temporary path.
	if err != nil {
		t.Fatalf("read docker capture: %v", err)
	}
	calls := string(raw)
	if !strings.Contains(calls, "-e POSTGRES_DB=dev") {
		t.Fatalf("docker run did not initialize dev database:\n%s", calls)
	}
	if strings.Contains(calls, "pg_isready") {
		t.Fatalf("readiness used pg_isready instead of querying the target database:\n%s", calls)
	}
	if strings.Count(calls, "SELECT 1") != 2 {
		t.Fatalf("target database readiness attempts = %d, want 2:\n%s", strings.Count(calls, "SELECT 1"), calls)
	}
	if strings.Count(calls, "CREATE EXTENSION IF NOT EXISTS vector") != 1 {
		t.Fatalf("extension setup calls = %d, want 1:\n%s", strings.Count(calls, "CREATE EXTENSION IF NOT EXISTS vector"), calls)
	}
}
