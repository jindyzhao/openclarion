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

const serviceImageDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestOpenClarionServiceImageRejectsLatestTag(t *testing.T) {
	binDir := writeOpenClarionServiceImageFakeTools(t, "")

	out, err := runOpenClarionServiceImageBuild(t, binDir, "--image-ref", "registry.example/openclarion/openclarion:latest")
	if err == nil {
		t.Fatalf("service image build passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "must not be latest") {
		t.Fatalf("service image build output = %q, want latest rejection", out)
	}
	assertOpenClarionServiceImageNoSecretLeak(t, out)
}

func TestOpenClarionServiceImagePushRequiresRegistryHost(t *testing.T) {
	binDir := writeOpenClarionServiceImageFakeTools(t, "")

	out, err := runOpenClarionServiceImageBuild(t, binDir, "--push", "--image-ref", "openclarion/openclarion:test")
	if err == nil {
		t.Fatalf("service image build passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "must include an explicit registry host") {
		t.Fatalf("service image build output = %q, want registry-host rejection", out)
	}
	assertOpenClarionServiceImageNoSecretLeak(t, out)
}

func TestOpenClarionServiceImageBuildUsesScratchContext(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "docker.log")
	binDir := writeOpenClarionServiceImageFakeTools(t, logPath)

	out, err := runOpenClarionServiceImageBuild(t, binDir, "--image-ref", "registry.example/openclarion/openclarion:test")
	if err != nil {
		t.Fatalf("service image build failed: %v\n%s", err, out)
	}
	assertOpenClarionServiceImageNoSecretLeak(t, out)

	logRaw, err := os.ReadFile(logPath) // #nosec G304 -- test reads a controlled fixture log path.
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	logText := string(logRaw)
	for _, want := range []string{
		"FROM scratch",
		"COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt",
		"COPY openclarion /openclarion",
		"USER 65532:65532",
		"ENTRYPOINT [\"/openclarion\"]",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("docker build context log missing %q:\n%s", want, logText)
		}
	}
}

func TestOpenClarionServiceImagePushWritesDigestRef(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "docker.log")
	binDir := writeOpenClarionServiceImageFakeTools(t, logPath)
	outPath := filepath.Join(t.TempDir(), "openclarion-service.digest")

	out, err := runOpenClarionServiceImageBuild(
		t,
		binDir,
		"--push",
		"--image-ref", "localhost:5000/openclarion/openclarion:test",
		"--digest-ref-out", outPath,
	)
	if err != nil {
		t.Fatalf("service image push failed: %v\n%s", err, out)
	}
	assertOpenClarionServiceImageNoSecretLeak(t, out)
	want := "localhost:5000/openclarion/openclarion@sha256:" + serviceImageDigest + "\n"
	if got := string(readOpenClarionServiceImageFile(t, outPath)); got != want {
		t.Fatalf("digest ref file = %q, want %q", got, want)
	}
	if !strings.Contains(out, strings.TrimSpace(want)) {
		t.Fatalf("service image output = %q, want digest ref", out)
	}
}

func runOpenClarionServiceImageBuild(t *testing.T, binDir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmdArgs := append([]string{"scripts/run_openclarion_service_image_build.sh"}, args...)
	cmd := exec.CommandContext(ctx, "bash", cmdArgs...) // #nosec G204 -- test invokes a controlled repository script.
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCLARION_LLM_API_KEY=not-a-secret-fixture",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeOpenClarionServiceImageFakeTools(t *testing.T, logPath string) string {
	t.Helper()
	binDir := t.TempDir()
	writeOpenClarionServiceImageFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "env" && "${2:-}" == "GOARCH" ]]; then
  printf 'amd64\n'
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
printf '#!/bin/sh\necho openclarion\n' >"$out"
chmod 0755 "$out"
`, 0o700)

	dockerScript := `#!/usr/bin/env bash
set -euo pipefail
log_path="` + logPath + `"
if [[ "$#" -ge 1 && "$1" == "build" ]]; then
  context="${!#}"
  if [[ -n "$log_path" ]]; then
    {
      printf 'docker build args:'
      printf ' %q' "$@"
      printf '\n'
      cat "$context/Dockerfile"
    } >>"$log_path"
  fi
  test -f "$context/openclarion"
  test -f "$context/ca-certificates.crt"
  exit 0
fi
if [[ "$#" -ge 1 && "$1" == "push" ]]; then
  exit 0
fi
if [[ "$#" -ge 3 && "$1" == "image" && "$2" == "inspect" ]]; then
  ref="${!#}"
  repository="${ref%:*}"
  printf '%s@sha256:%s\n' "$repository" "` + serviceImageDigest + `"
  exit 0
fi
if [[ "$#" -ge 3 && "$1" == "buildx" && "$2" == "imagetools" && "$3" == "inspect" ]]; then
  printf 'sha256:%s\n' "` + serviceImageDigest + `"
  exit 0
fi
exit 2
`
	writeOpenClarionServiceImageFile(t, binDir, "docker", dockerScript, 0o700)
	return binDir
}

func writeOpenClarionServiceImageFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func readOpenClarionServiceImageFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads controlled fixture paths.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func assertOpenClarionServiceImageNoSecretLeak(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "not-a-secret-fixture") {
		t.Fatalf("service image build leaked secret fixture: %q", out)
	}
}
