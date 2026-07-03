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

const runtimeSmokeArtifactDigest = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

func TestSandboxM4RuntimeSmokeArtifactsRejectsRepoLocalPublicOutput(t *testing.T) {
	binDir := writeSandboxM4RuntimeSmokeArtifactsFakeTools(t)

	out, err := runSandboxM4RuntimeSmokeArtifacts(
		t,
		binDir,
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=docs/runtime-smokes",
		"OPENCLARION_AGENT_RUNTIME_IMAGE=registry.example.com/openclarion/runtime@sha256:"+runtimeSmokeArtifactDigest,
	)
	if err == nil {
		t.Fatalf("runtime smoke artifact collection passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "repo-local output must be ignored by git") {
		t.Fatalf("runtime smoke output = %q, want ignored-path rejection", out)
	}
}

func TestSandboxM4RuntimeSmokeArtifactsAllowsIgnoredRepoLocalOutput(t *testing.T) {
	binDir := writeSandboxM4RuntimeSmokeArtifactsFakeTools(t)
	outputDir := filepath.Join("artifacts", "runtime-smokes-"+strings.ReplaceAll(t.Name(), "/", "-"))
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join("..", outputDir))
	})

	out, err := runSandboxM4RuntimeSmokeArtifacts(
		t,
		binDir,
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR="+outputDir,
		"OPENCLARION_AGENT_RUNTIME_IMAGE=registry.example.com/openclarion/runtime@sha256:"+runtimeSmokeArtifactDigest,
	)
	if err != nil {
		t.Fatalf("runtime smoke artifact collection failed: %v\n%s", err, out)
	}
	for _, name := range []string{
		"agent-runtime-smoke.json",
		"container-provider-smoke.json",
		"container-provider-timeout-smoke.json",
		"container-provider-output-cap-smoke.json",
		"egress-allowdeny-smoke.json",
	} {
		if len(readSandboxM4RuntimeSmokeArtifactFile(t, filepath.Join("..", outputDir, name))) == 0 {
			t.Fatalf("artifact %s is empty", name)
		}
	}
}

func runSandboxM4RuntimeSmokeArtifacts(t *testing.T, binDir string, env ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/run_sandbox_m4_runtime_smoke_artifacts.sh") // #nosec G204 -- test invokes a controlled repository script.
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), append([]string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeSandboxM4RuntimeSmokeArtifactsFakeTools(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	writeSandboxM4RuntimeSmokeArtifactFile(t, binDir, "docker", `#!/usr/bin/env bash
set -euo pipefail
exit 0
`, 0o700)
	writeSandboxM4RuntimeSmokeArtifactFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "run" && "${2:-}" == "./scripts/sandbox_m4_runtime_smoke_artifacts" ]]; then
  exit 0
fi
exit 0
`, 0o700)
	writeSandboxM4RuntimeSmokeArtifactFile(t, binDir, "make", `#!/usr/bin/env bash
set -euo pipefail
proof_path=""
case "${1:-}" in
  agent-runtime-smoke)
    proof_path="${OPENCLARION_AGENT_RUNTIME_PROOF_PATH:-}"
    ;;
  container-provider-smoke|container-provider-timeout-smoke|container-provider-output-cap-smoke)
    proof_path="${OPENCLARION_CONTAINER_PROVIDER_SMOKE_PROOF_PATH:-}"
    ;;
  egress-allowdeny-smoke)
    proof_path="${OPENCLARION_EGRESS_SMOKE_PROOF_PATH:-}"
    ;;
  *)
    exit 2
    ;;
esac
if [[ -z "$proof_path" ]]; then
  echo "missing proof path" >&2
  exit 2
fi
mkdir -p "$(dirname "$proof_path")"
printf '{"status":"pass"}\n' >"$proof_path"
`, 0o700)
	return binDir
}

func writeSandboxM4RuntimeSmokeArtifactFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func readSandboxM4RuntimeSmokeArtifactFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads controlled fixture paths.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}
