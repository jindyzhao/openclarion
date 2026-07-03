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

func TestDiagnosisLiveConvergenceSmokeLoadsPrivateEnvFile(t *testing.T) {
	binDir := writeDiagnosisConvergenceFakeTools(t)
	privateDir := t.TempDir()
	output := filepath.Join(privateDir, "proof.json")
	envFile := writeDiagnosisConvergenceEnvFile(t, privateDir, map[string]string{
		"DIAGNOSIS_LIVE_CONVERGENCE_SMOKE_OUTPUT": output,
		"OPENCLARION_LIVE_API_BASE_URL":           "http://127.0.0.1:32101",
		"OPENCLARION_LIVE_BEARER_TOKEN":           "convergence-secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID":   "42",
		"OPENCLARION_LIVE_SMOKE_WORKDIR":          privateDir,
	})

	out, err := runDiagnosisConvergenceSmoke(t, binDir, "--env-file", envFile)
	if err != nil {
		t.Fatalf("diagnosis convergence smoke failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "OK - live convergence output") {
		t.Fatalf("diagnosis convergence output = %q, want success", out)
	}
	if strings.Contains(out, "convergence-secret-token") {
		t.Fatalf("diagnosis convergence output leaked secret: %s", out)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	if !strings.Contains(string(raw), `"passed":true`) ||
		!strings.Contains(string(raw), `"api_base_url":"http://127.0.0.1:32101"`) {
		t.Fatalf("proof = %s, want env-backed proof", raw)
	}
	if strings.Contains(string(raw), "convergence-secret-token") {
		t.Fatalf("proof leaked secret: %s", raw)
	}
}

func TestDiagnosisLiveConvergenceSmokeUsesStaticBearerFallback(t *testing.T) {
	binDir := writeDiagnosisConvergenceFakeTools(t)
	privateDir := t.TempDir()
	output := filepath.Join(privateDir, "proof.json")
	envFile := writeDiagnosisConvergenceEnvFile(t, privateDir, map[string]string{
		"DIAGNOSIS_LIVE_CONVERGENCE_SMOKE_OUTPUT":   output,
		"OPENCLARION_LIVE_API_BASE_URL":             "http://127.0.0.1:32101",
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":           "static",
		"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN": "static-convergence-secret",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID":     "42",
		"OPENCLARION_LIVE_SMOKE_WORKDIR":            privateDir,
	})

	out, err := runDiagnosisConvergenceSmoke(t, binDir, "--env-file", envFile)
	if err != nil {
		t.Fatalf("diagnosis convergence smoke failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "OK - live convergence output") {
		t.Fatalf("diagnosis convergence output = %q, want success", out)
	}
	if strings.Contains(out, "static-convergence-secret") {
		t.Fatalf("diagnosis convergence output leaked static token: %s", out)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	if strings.Contains(string(raw), "static-convergence-secret") {
		t.Fatalf("proof leaked static token: %s", raw)
	}
}

func TestDiagnosisLiveConvergenceSmokeRejectsUnknownArgument(t *testing.T) {
	binDir := writeDiagnosisConvergenceFakeTools(t)
	out, err := runDiagnosisConvergenceSmoke(t, binDir, "--unknown")
	if err == nil {
		t.Fatalf("diagnosis convergence smoke passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "usage: bash scripts/run_diagnosis_live_convergence_smoke.sh") {
		t.Fatalf("diagnosis convergence output = %q, want usage", out)
	}
}

func runDiagnosisConvergenceSmoke(t *testing.T, binDir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{"scripts/run_diagnosis_live_convergence_smoke.sh"}, args...)...) // #nosec G204 -- test invokes a controlled repository script.
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeDiagnosisConvergenceFakeTools(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	writeDiagnosisConvergenceFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "run" && "${2:-}" == "./scripts/manual_evidence_readiness" ]]; then
  printf '{"targets":[{"name":"diagnosis-live-convergence-smoke","status":"ready"}]}\n'
  exit 0
fi
if [[ "${1:-}" == "run" && "${2:-}" == "./scripts/diagnosis_live_convergence_smoke_output" ]]; then
  test -s "${3:-}"
  exit 0
fi
exit 2
`, 0o700)
	writeDiagnosisConvergenceFile(t, binDir, "node", `#!/usr/bin/env bash
set -euo pipefail
output="${2:-}"
if [[ -z "$output" ]]; then
  echo "missing output path" >&2
  exit 2
fi
mkdir -p "$(dirname "$output")"
printf '{"passed":true,"api_base_url":"%s","snapshot":"%s"}\n' \
  "${OPENCLARION_LIVE_API_BASE_URL:-}" \
  "${OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID:-}" >"$output"
`, 0o700)
	return binDir
}

func writeDiagnosisConvergenceEnvFile(t *testing.T, dir string, values map[string]string) string {
	t.Helper()
	var body strings.Builder
	for key, value := range values {
		body.WriteString(key)
		body.WriteString("=")
		body.WriteString(shellSingleQuote(value))
		body.WriteString("\n")
	}
	path := filepath.Join(dir, "live.env")
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil { // #nosec G306,G703 -- test helper writes a private fixture env file.
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func writeDiagnosisConvergenceFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G306,G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
