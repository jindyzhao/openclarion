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

const customThinRunnerDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestCustomThinRunnerSmokeRejectsRepoLocalPublicDigestRefOutput(t *testing.T) {
	binDir := writeCustomThinRunnerSmokeFakeTools(t)

	out, err := runCustomThinRunnerSmoke(
		t,
		binDir,
		"OPENCLARION_CUSTOM_THIN_RUNNER_DIGEST_REF_OUT=docs/custom-thin-runner.digest-ref",
	)
	if err == nil {
		t.Fatalf("custom thin runner smoke passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "repo-local output must be ignored by git") {
		t.Fatalf("custom thin runner output = %q, want ignored-path rejection", out)
	}
}

func TestCustomThinRunnerSmokeAllowsIgnoredRuntimeArtifactOutput(t *testing.T) {
	binDir := writeCustomThinRunnerSmokeFakeTools(t)
	outputDir := filepath.Join("artifacts", "custom-thin-runner-smoke-"+strings.ReplaceAll(t.Name(), "/", "-"))
	digestRefOut := filepath.Join(outputDir, "digest-ref.txt")
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join("..", outputDir))
	})

	out, err := runCustomThinRunnerSmoke(
		t,
		binDir,
		"OPENCLARION_CUSTOM_THIN_RUNNER_SMOKE_RUN_ID=test-output",
		"OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR="+outputDir,
		"OPENCLARION_CUSTOM_THIN_RUNNER_DIGEST_REF_OUT="+digestRefOut,
	)
	if err != nil {
		t.Fatalf("custom thin runner smoke failed: %v\n%s", err, out)
	}
	want := "localhost:5001/openclarion/custom-thin-runner@sha256:" + customThinRunnerDigest + "\n"
	if got := string(readCustomThinRunnerSmokeFile(t, filepath.Join("..", digestRefOut))); got != want {
		t.Fatalf("digest ref file = %q, want %q", got, want)
	}
}

func runCustomThinRunnerSmoke(t *testing.T, binDir string, env ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/run_custom_thin_runner_smoke.sh") // #nosec G204 -- test invokes a controlled repository script.
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), append([]string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeCustomThinRunnerSmokeFakeTools(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	writeCustomThinRunnerSmokeFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "env" && "${2:-}" == "GOARCH" ]]; then
  printf 'amd64\n'
  exit 0
fi
if [[ "${1:-}" == "run" ]]; then
  exit 0
fi
out=""
while (($# > 0)); do
  if [[ "$1" == "-o" ]]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
if [[ -z "$out" ]]; then
  echo "missing -o" >&2
  exit 2
fi
mkdir -p "$(dirname "$out")"
printf '#!/bin/sh\necho custom-thin-runner\n' >"$out"
chmod 0755 "$out"
`, 0o700)
	writeCustomThinRunnerSmokeFile(t, binDir, "docker", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "build" ]]; then
  exit 0
fi
if [[ "${1:-}" == "run" ]]; then
  for arg in "$@"; do
    if [[ "$arg" == "-d" ]]; then
      printf 'registry-container-id\n'
      exit 0
    fi
  done
  printf '{"service":"payments"}\n'
  exit 0
fi
if [[ "${1:-}" == "port" ]]; then
  printf '127.0.0.1:5001\n'
  exit 0
fi
if [[ "${1:-}" == "tag" || "${1:-}" == "push" || "${1:-}" == "rm" ]]; then
  exit 0
fi
if [[ "${1:-}" == "image" && "${2:-}" == "rm" ]]; then
  exit 0
fi
if [[ "${1:-}" == "image" && "${2:-}" == "inspect" ]]; then
  ref="${!#}"
  repository="${ref%:*}"
  printf '%s@sha256:`+customThinRunnerDigest+`\n' "$repository"
  exit 0
fi
exit 2
`, 0o700)
	writeCustomThinRunnerSmokeFile(t, binDir, "make", `#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  agent-runtime-smoke|container-provider-smoke|sandbox-m4-runtime-smoke-artifacts)
    exit 0
    ;;
esac
exit 2
`, 0o700)
	return binDir
}

func writeCustomThinRunnerSmokeFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func readCustomThinRunnerSmokeFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads controlled fixture paths.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}
