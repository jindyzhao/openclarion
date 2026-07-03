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

func TestRunDiagnosisAuthBFFLiveSmokePassesBearerEnvBackedCredentials(t *testing.T) {
	binDir := writeDiagnosisAuthBFFLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	argsOut := filepath.Join(fixtureDir, "args.txt")
	output := filepath.Join(fixtureDir, "proof.json")
	envFile := writeDiagnosisAuthBFFEnvFile(t, fixtureDir, map[string]string{
		"DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT":         output,
		"OPENCLARION_LIVE_BEARER_TOKEN":                "Bearer fixture-token",
		"OPENCLARION_LIVE_WEB_BASE_URL":                "http://127.0.0.1:32102",
		"OPENCLARION_TEST_DIAGNOSIS_AUTH_BFF_ARGS_OUT": argsOut,
	})

	out, err := runDiagnosisAuthBFFLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err != nil {
		t.Fatalf("diagnosis auth BFF live smoke wrapper failed: %v\n%s", err, out)
	}
	args := readDiagnosisAuthBFFLiveSmokeArgs(t, argsOut)
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--web-base-url", "http://127.0.0.1:32102")
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--auth-mode", "bearer")
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--bearer-token-env", "OPENCLARION_LIVE_DIAGNOSIS_AUTH_BFF_EFFECTIVE_BEARER_TOKEN")
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--output", output)
	if strings.Contains(out, "fixture-token") || strings.Contains(strings.Join(args, " "), "fixture-token") {
		t.Fatalf("wrapper leaked bearer token: out=%q args=%q", out, strings.Join(args, " "))
	}
}

func TestRunDiagnosisAuthBFFLiveSmokePassesExplicitLDAPEnvBackedCredentials(t *testing.T) {
	binDir := writeDiagnosisAuthBFFLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	argsOut := filepath.Join(fixtureDir, "args.txt")
	output := filepath.Join(fixtureDir, "proof.json")
	envFile := writeDiagnosisAuthBFFEnvFile(t, fixtureDir, map[string]string{
		"DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT":         output,
		"OPENCLARION_LIVE_AUTH_MODE":                   "ldap",
		"OPENCLARION_LIVE_LDAP_PASSWORD":               "fixture-secret",
		"OPENCLARION_LIVE_LDAP_USERNAME":               "operator-1",
		"OPENCLARION_LIVE_WEB_BASE_URL":                "http://127.0.0.1:32102",
		"OPENCLARION_TEST_DIAGNOSIS_AUTH_BFF_ARGS_OUT": argsOut,
	})

	out, err := runDiagnosisAuthBFFLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err != nil {
		t.Fatalf("diagnosis auth BFF live smoke wrapper failed: %v\n%s", err, out)
	}
	args := readDiagnosisAuthBFFLiveSmokeArgs(t, argsOut)
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--web-base-url", "http://127.0.0.1:32102")
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--auth-mode", "ldap")
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--ldap-username-env", "OPENCLARION_LIVE_LDAP_USERNAME")
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--ldap-password-env", "OPENCLARION_LIVE_LDAP_PASSWORD")
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--output", output)
	if strings.Contains(out, "fixture-secret") || strings.Contains(strings.Join(args, " "), "fixture-secret") {
		t.Fatalf("wrapper leaked LDAP password: out=%q args=%q", out, strings.Join(args, " "))
	}
}

func TestRunDiagnosisAuthBFFLiveSmokeDerivesLocalWebBaseURLFromPort(t *testing.T) {
	binDir := writeDiagnosisAuthBFFLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeDiagnosisAuthBFFEnvFile(t, fixtureDir, map[string]string{
		"DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_OUTPUT":         filepath.Join(fixtureDir, "proof.json"),
		"OPENCLARION_LIVE_BEARER_TOKEN":                "fixture-token",
		"OPENCLARION_LIVE_WEB_PORT":                    "32102",
		"OPENCLARION_TEST_DIAGNOSIS_AUTH_BFF_ARGS_OUT": argsOut,
	})

	out, err := runDiagnosisAuthBFFLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err != nil {
		t.Fatalf("diagnosis auth BFF live smoke wrapper failed: %v\n%s", err, out)
	}
	args := readDiagnosisAuthBFFLiveSmokeArgs(t, argsOut)
	assertDiagnosisAuthBFFLiveSmokeArg(t, args, "--web-base-url", "http://127.0.0.1:32102")
}

func TestRunDiagnosisAuthBFFLiveSmokeRejectsInvalidLocalWebPort(t *testing.T) {
	binDir := writeDiagnosisAuthBFFLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	envFile := writeDiagnosisAuthBFFEnvFile(t, fixtureDir, map[string]string{
		"OPENCLARION_LIVE_BEARER_TOKEN": "fixture-token",
		"OPENCLARION_LIVE_WEB_PORT":     "70000",
	})

	out, err := runDiagnosisAuthBFFLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err == nil {
		t.Fatalf("diagnosis auth BFF live smoke wrapper passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_LIVE_WEB_PORT must be an integer from 1 to 65535") {
		t.Fatalf("output = %q, want port validation error", out)
	}
}

func TestRunDiagnosisAuthBFFLiveSmokeRejectsRepoLocalPublicWorkdirBeforeMktemp(t *testing.T) {
	binDir := writeDiagnosisAuthBFFLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	repoRoot := openclarionRepoRoot(t)
	workdir := filepath.Join(repoRoot, "auth-bff-public-workdir")
	envFile := writeDiagnosisAuthBFFEnvFile(t, fixtureDir, map[string]string{
		"DIAGNOSIS_AUTH_BFF_LIVE_SMOKE_WORKDIR": workdir,
		"OPENCLARION_LIVE_BEARER_TOKEN":         "fixture-token",
		"OPENCLARION_LIVE_WEB_BASE_URL":         "http://127.0.0.1:32102",
	})
	t.Cleanup(func() {
		_ = os.RemoveAll(workdir)
	})

	out, err := runDiagnosisAuthBFFLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err == nil {
		t.Fatalf("diagnosis auth BFF live smoke wrapper passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "repo-local output must live under .openclarion-private/") {
		t.Fatalf("output = %q, want repo-local workdir rejection", out)
	}
	if _, statErr := os.Stat(workdir); !os.IsNotExist(statErr) {
		t.Fatalf("public workdir was created before validation; stat error = %v", statErr)
	}
}

func runDiagnosisAuthBFFLiveSmokeWrapper(t *testing.T, binDir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{"scripts/run_diagnosis_auth_bff_live_smoke.sh"}, args...)...) // #nosec G204 -- test invokes a controlled repository script.
	cmd.Dir = openclarionRepoRoot(t)
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeDiagnosisAuthBFFLiveSmokeFakeGo(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	writeDiagnosisAuthBFFLiveSmokeFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "run" || "${2:-}" != "./scripts/diagnosis_auth_bff_live_smoke" ]]; then
  exit 2
fi
printf '%s\n' "$@" >"$OPENCLARION_TEST_DIAGNOSIS_AUTH_BFF_ARGS_OUT"
`, 0o700)
	return binDir
}

func writeDiagnosisAuthBFFEnvFile(t *testing.T, dir string, values map[string]string) string {
	t.Helper()
	var body strings.Builder
	for key, value := range values {
		body.WriteString(key)
		body.WriteString("=")
		body.WriteString(diagnosisAuthBFFShellSingleQuote(value))
		body.WriteString("\n")
	}
	path := filepath.Join(dir, "diagnosis-auth-bff-live.env")
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil { // #nosec G306,G703 -- test helper writes a private fixture env file.
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func readDiagnosisAuthBFFLiveSmokeArgs(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads the args path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	return strings.Fields(string(raw))
}

func assertDiagnosisAuthBFFLiveSmokeArg(t *testing.T, args []string, name, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name && args[i+1] == value {
			return
		}
	}
	t.Fatalf("args %q missing %s %s", strings.Join(args, " "), name, value)
}

func writeDiagnosisAuthBFFLiveSmokeFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G306,G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func diagnosisAuthBFFShellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
