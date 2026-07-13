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

const diagnosisRunnerImageDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestBuildDiagnosisAssistantRunnerDefaultsToAMD64(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "commands.log")
	binDir := writeDiagnosisRunnerBuildFakeTools(t)
	outPath := filepath.Join(t.TempDir(), "runner.digest-ref")

	out, err := runDiagnosisRunnerBuild(t, binDir, logPath, outPath, "", "")
	if err != nil {
		t.Fatalf("runner build failed: %v\n%s", err, out)
	}
	assertDiagnosisRunnerBuildTarget(t, logPath, "amd64")
	if !strings.Contains(out, "image platform: linux/amd64") {
		t.Fatalf("build output = %q, want amd64 platform", out)
	}
	assertDiagnosisRunnerDigestRef(t, outPath)
}

func TestBuildDiagnosisAssistantRunnerAllowsARM64Override(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "commands.log")
	binDir := writeDiagnosisRunnerBuildFakeTools(t)
	outPath := filepath.Join(t.TempDir(), "runner.digest-ref")

	out, err := runDiagnosisRunnerBuild(t, binDir, logPath, outPath, "arm64", "")
	if err != nil {
		t.Fatalf("runner build failed: %v\n%s", err, out)
	}
	assertDiagnosisRunnerBuildTarget(t, logPath, "arm64")
	if !strings.Contains(out, "image platform: linux/arm64") {
		t.Fatalf("build output = %q, want arm64 platform", out)
	}
}

func TestBuildDiagnosisAssistantRunnerRejectsUnsupportedArchitecture(t *testing.T) {
	binDir := writeDiagnosisRunnerBuildFakeTools(t)
	outPath := filepath.Join(t.TempDir(), "runner.digest-ref")

	out, err := runDiagnosisRunnerBuild(t, binDir, filepath.Join(t.TempDir(), "commands.log"), outPath, "ppc64le", "")
	if err == nil {
		t.Fatalf("runner build passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "target architecture must be amd64 or arm64") {
		t.Fatalf("build output = %q, want target architecture rejection", out)
	}
}

func TestBuildDiagnosisAssistantRunnerRejectsImagePlatformMismatch(t *testing.T) {
	binDir := writeDiagnosisRunnerBuildFakeTools(t)
	outPath := filepath.Join(t.TempDir(), "runner.digest-ref")

	out, err := runDiagnosisRunnerBuild(t, binDir, filepath.Join(t.TempDir(), "commands.log"), outPath, "amd64", "arm64")
	if err == nil {
		t.Fatalf("runner build passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "local image platform linux/arm64 does not match linux/amd64") {
		t.Fatalf("build output = %q, want image platform mismatch", out)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatalf("digest output exists after platform mismatch: %v", statErr)
	}
}

func TestBuildDiagnosisAssistantRunnerWaitsForRegistryReadiness(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "commands.log")
	binDir := writeDiagnosisRunnerBuildFakeTools(t)
	outPath := filepath.Join(t.TempDir(), "runner.digest-ref")

	out, err := runDiagnosisRunnerBuild(
		t,
		binDir,
		logPath,
		outPath,
		"",
		"",
		"OPENCLARION_TEST_REGISTRY_READY_AFTER=2",
		"OPENCLARION_DIAGNOSIS_RUNNER_REGISTRY_READY_TIMEOUT_SECONDS=5",
	)
	if err != nil {
		t.Fatalf("runner build failed: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(logPath) // #nosec G304 -- test reads its controlled fixture log.
	if err != nil {
		t.Fatal(err)
	}
	logText := string(raw)
	if got := strings.Count(logText, "curl --proto"); got != 2 {
		t.Fatalf("registry readiness attempts = %d, want 2:\n%s", got, logText)
	}
	probe := strings.LastIndex(logText, "curl --proto")
	tag := strings.Index(logText, "docker tag")
	push := strings.Index(logText, "docker push")
	if probe < 0 || tag <= probe || push <= tag {
		t.Fatalf("registry probe/tag/push order is invalid:\n%s", logText)
	}
}

func TestBuildDiagnosisAssistantRunnerRejectsRegistryReadinessTimeout(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "commands.log")
	binDir := writeDiagnosisRunnerBuildFakeTools(t)
	outPath := filepath.Join(t.TempDir(), "runner.digest-ref")

	out, err := runDiagnosisRunnerBuild(
		t,
		binDir,
		logPath,
		outPath,
		"",
		"",
		"OPENCLARION_TEST_REGISTRY_READY_AFTER=99",
		"OPENCLARION_DIAGNOSIS_RUNNER_REGISTRY_READY_TIMEOUT_SECONDS=1",
	)
	if err == nil {
		t.Fatalf("runner build passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "temporary registry did not become ready within 1s") {
		t.Fatalf("build output = %q, want registry timeout", out)
	}
	raw, readErr := os.ReadFile(logPath) // #nosec G304 -- test reads its controlled fixture log.
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(raw), "docker tag") || strings.Contains(string(raw), "docker push") {
		t.Fatalf("build tagged or pushed before registry readiness:\n%s", raw)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatalf("digest output exists after registry timeout: %v", statErr)
	}
}

func TestBuildDiagnosisAssistantRunnerRejectsInvalidRegistryReadinessTimeout(t *testing.T) {
	for _, timeout := range []string{"0", "121", "01", "not-a-number"} {
		t.Run(timeout, func(t *testing.T) {
			binDir := writeDiagnosisRunnerBuildFakeTools(t)
			outPath := filepath.Join(t.TempDir(), "runner.digest-ref")
			out, err := runDiagnosisRunnerBuild(
				t,
				binDir,
				filepath.Join(t.TempDir(), "commands.log"),
				outPath,
				"",
				"",
				"OPENCLARION_DIAGNOSIS_RUNNER_REGISTRY_READY_TIMEOUT_SECONDS="+timeout,
			)
			if err == nil {
				t.Fatalf("runner build passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, "registry readiness timeout must be an integer from 1 to 120 seconds") {
				t.Fatalf("build output = %q, want timeout validation", out)
			}
		})
	}
}

func runDiagnosisRunnerBuild(
	t *testing.T,
	binDir string,
	logPath string,
	outPath string,
	targetArch string,
	imageArch string,
	extraEnv ...string,
) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/build_diagnosis_assistant_runner.sh") // #nosec G204 -- controlled repository script.
	cmd.Dir = ".."
	env := make([]string, 0, len(os.Environ())+9+len(extraEnv))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "PATH",
			"OPENCLARION_DIAGNOSIS_RUNNER_BUILD_ID",
			"OPENCLARION_DIAGNOSIS_RUNNER_DIGEST_REF_OUT",
			"OPENCLARION_DIAGNOSIS_RUNNER_REGISTRY_READY_TIMEOUT_SECONDS",
			"OPENCLARION_DIAGNOSIS_RUNNER_TARGET_ARCH",
			"OPENCLARION_TEST_COMMAND_LOG",
			"OPENCLARION_TEST_CURL_COUNT",
			"OPENCLARION_TEST_IMAGE_ARCH",
			"OPENCLARION_TEST_REGISTRY_READY_AFTER":
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCLARION_DIAGNOSIS_RUNNER_BUILD_ID=test-build",
		"OPENCLARION_DIAGNOSIS_RUNNER_DIGEST_REF_OUT="+outPath,
		"OPENCLARION_DIAGNOSIS_RUNNER_TARGET_ARCH="+targetArch,
		"OPENCLARION_TEST_COMMAND_LOG="+logPath,
		"OPENCLARION_TEST_CURL_COUNT="+filepath.Join(t.TempDir(), "curl-count"),
		"OPENCLARION_TEST_IMAGE_ARCH="+imageArch,
	)
	env = append(env, extraEnv...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func assertDiagnosisRunnerBuildTarget(t *testing.T, logPath, arch string) {
	t.Helper()
	raw, err := os.ReadFile(logPath) // #nosec G304 -- test reads its controlled fixture log.
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	logText := string(raw)
	for _, want := range []string{
		"go GOOS=linux GOARCH=" + arch,
		"docker build --pull=false --platform linux/" + arch,
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("command log missing %q:\n%s", want, logText)
		}
	}
}

func assertDiagnosisRunnerDigestRef(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads its controlled fixture output.
	if err != nil {
		t.Fatalf("read digest ref: %v", err)
	}
	want := "localhost:35000/openclarion/diagnosis-assistant-runner@sha256:" + diagnosisRunnerImageDigest + "\n"
	if string(raw) != want {
		t.Fatalf("digest ref = %q, want %q", raw, want)
	}
}

func writeDiagnosisRunnerBuildFakeTools(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	writeDiagnosisRunnerBuildFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
{
  printf 'go GOOS=%s GOARCH=%s' "${GOOS:-}" "${GOARCH:-}"
  printf ' %s' "$@"
  printf '\n'
} >>"$OPENCLARION_TEST_COMMAND_LOG"
out=""
license_dir=""
while (($# > 0)); do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    --save_path=*)
      license_dir="${1#--save_path=}"
      shift
      ;;
    *) shift ;;
  esac
done
if [[ -n "$out" ]]; then
  mkdir -p "$(dirname "$out")"
  printf '#!/bin/sh\nexit 0\n' >"$out"
  chmod 0755 "$out"
fi
if [[ -n "$license_dir" ]]; then
  mkdir -p "$license_dir/example"
  printf 'fixture license\n' >"$license_dir/example/LICENSE"
fi
`, 0o700)

	writeDiagnosisRunnerBuildFile(t, binDir, "docker", `#!/usr/bin/env bash
set -euo pipefail
{
  printf 'docker'
  printf ' %s' "$@"
  printf '\n'
} >>"$OPENCLARION_TEST_COMMAND_LOG"
target_arch="${OPENCLARION_DIAGNOSIS_RUNNER_TARGET_ARCH:-amd64}"
reported_arch="${OPENCLARION_TEST_IMAGE_ARCH:-$target_arch}"
if [[ -z "$reported_arch" ]]; then
  reported_arch="$target_arch"
fi
case "${1:-}" in
  build)
    context="${!#}"
    test -x "$context/diagnosis-assistant-runner"
    test -f "$context/Dockerfile"
    test -f "$context/ca-certificates.crt"
    exit 0
    ;;
  run)
    printf 'fake-registry-cid\n'
    exit 0
    ;;
  port)
    printf '127.0.0.1:35000\n'
    exit 0
    ;;
  tag | push | rm)
    exit 0
    ;;
  image)
    if [[ "${2:-}" == "rm" ]]; then
      exit 0
    fi
    if [[ "${2:-}" != "inspect" ]]; then
      exit 2
    fi
    ref="${!#}"
    for arg in "$@"; do
      if [[ "$arg" == *'.Os'*'.Architecture'* ]]; then
        printf 'linux/%s\n' "$reported_arch"
        exit 0
      fi
      if [[ "$arg" == *'RepoDigests'* ]]; then
        repository="${ref%:*}"
        printf '%s@sha256:%s\n' "$repository" "`+diagnosisRunnerImageDigest+`"
        exit 0
      fi
    done
    exit 0
    ;;
esac
exit 2
`, 0o700)

	writeDiagnosisRunnerBuildFile(t, binDir, "curl", `#!/usr/bin/env bash
set -euo pipefail
{
  printf 'curl'
  printf ' %s' "$@"
  printf '\n'
} >>"$OPENCLARION_TEST_COMMAND_LOG"
header_file=""
while (($# > 0)); do
  case "$1" in
    --dump-header)
      header_file="$2"
      shift 2
      ;;
    *) shift ;;
  esac
done
[[ -n "$header_file" ]]
count=0
if [[ -f "$OPENCLARION_TEST_CURL_COUNT" ]]; then
  read -r count <"$OPENCLARION_TEST_CURL_COUNT"
fi
count=$((count + 1))
printf '%s\n' "$count" >"$OPENCLARION_TEST_CURL_COUNT"
ready_after="${OPENCLARION_TEST_REGISTRY_READY_AFTER:-1}"
if ((count < ready_after)); then
  exit 7
fi
printf 'HTTP/1.1 200 OK\r\nDocker-Distribution-Api-Version: registry/2.0\r\n\r\n' >"$header_file"
`, 0o700)
	return binDir
}

func writeDiagnosisRunnerBuildFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
}
