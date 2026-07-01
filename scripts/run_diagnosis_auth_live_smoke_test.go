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

func TestRunDiagnosisAuthLiveSmokeRejectsLegacyWeComSessionMode(t *testing.T) {
	binDir := writeDiagnosisAuthLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	envFile := writeDiagnosisAuthLiveSmokeEnvFile(t, fixtureDir, map[string]string{
		"DIAGNOSIS_AUTH_LIVE_SMOKE_OUTPUT":     filepath.Join(fixtureDir, "proof.json"),
		"OPENCLARION_LIVE_API_BASE_URL":        "http://127.0.0.1:32102",
		"OPENCLARION_LIVE_AUTH_MODE":           "wecom_session",
		"OPENCLARION_LIVE_WECOM_SESSION_TOKEN": "wecom-session-token-1",
	})

	out, err := runDiagnosisAuthLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err == nil {
		t.Fatalf("diagnosis auth live smoke wrapper succeeded; want legacy WeCom rejection\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_LIVE_AUTH_MODE must be ldap or bearer") {
		t.Fatalf("output = %q, want legacy auth mode rejection", out)
	}
	if strings.Contains(out, "wecom-session-token-1") {
		t.Fatalf("wrapper output leaked WeCom session token: %q", out)
	}
}

func TestRunDiagnosisAuthLiveSmokePassesLDAPSessionIssueFlag(t *testing.T) {
	binDir := writeDiagnosisAuthLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeDiagnosisAuthLiveSmokeEnvFile(t, fixtureDir, map[string]string{
		"DIAGNOSIS_AUTH_LIVE_SMOKE_OUTPUT":              filepath.Join(fixtureDir, "proof.json"),
		"OPENCLARION_LIVE_API_BASE_URL":                 "http://127.0.0.1:32102",
		"OPENCLARION_LIVE_AUTH_MODE":                    "ldap",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_ISSUE_SESSION": "true",
		"OPENCLARION_LIVE_LDAP_PASSWORD":                "ldap-password",
		"OPENCLARION_LIVE_LDAP_USERNAME":                "operator-1",
		"OPENCLARION_TEST_DIAGNOSIS_AUTH_ARGS_OUT":      argsOut,
	})

	out, err := runDiagnosisAuthLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err != nil {
		t.Fatalf("diagnosis auth live smoke wrapper failed: %v\n%s", err, out)
	}
	args := readDiagnosisAuthLiveSmokeArgs(t, argsOut)
	assertDiagnosisAuthLiveSmokeArg(t, args, "--auth-mode", "ldap")
	assertDiagnosisAuthLiveSmokeFlag(t, args, "--issue-session")
	if strings.Contains(out, "ldap-password") {
		t.Fatalf("wrapper output leaked LDAP password: %q", out)
	}
}

func TestRunDiagnosisAuthLiveSmokeRejectsLegacyWeComSessionWithIssueSession(t *testing.T) {
	binDir := writeDiagnosisAuthLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	envFile := writeDiagnosisAuthLiveSmokeEnvFile(t, fixtureDir, map[string]string{
		"DIAGNOSIS_AUTH_LIVE_SMOKE_OUTPUT":              filepath.Join(fixtureDir, "proof.json"),
		"OPENCLARION_LIVE_API_BASE_URL":                 "http://127.0.0.1:32102",
		"OPENCLARION_LIVE_AUTH_MODE":                    "wecom_session",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_ISSUE_SESSION": "true",
		"OPENCLARION_LIVE_WECOM_SESSION_TOKEN":          "wecom-session-token-1",
	})

	out, err := runDiagnosisAuthLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err == nil {
		t.Fatalf("diagnosis auth live smoke wrapper succeeded; want rejection\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_LIVE_AUTH_MODE must be ldap or bearer") {
		t.Fatalf("output = %q, want legacy auth mode rejection", out)
	}
	if strings.Contains(out, "wecom-session-token-1") {
		t.Fatalf("wrapper output leaked WeCom session token: %q", out)
	}
}

func TestRunDiagnosisAuthLiveSmokeRejectsBadSessionIssueFlag(t *testing.T) {
	binDir := writeDiagnosisAuthLiveSmokeFakeGo(t)
	fixtureDir := t.TempDir()
	envFile := writeDiagnosisAuthLiveSmokeEnvFile(t, fixtureDir, map[string]string{
		"DIAGNOSIS_AUTH_LIVE_SMOKE_OUTPUT":              filepath.Join(fixtureDir, "proof.json"),
		"OPENCLARION_LIVE_API_BASE_URL":                 "http://127.0.0.1:32102",
		"OPENCLARION_LIVE_AUTH_MODE":                    "ldap",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_ISSUE_SESSION": "sometimes",
		"OPENCLARION_LIVE_LDAP_PASSWORD":                "ldap-password",
		"OPENCLARION_LIVE_LDAP_USERNAME":                "operator-1",
	})

	out, err := runDiagnosisAuthLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err == nil {
		t.Fatalf("diagnosis auth live smoke wrapper succeeded; want rejection\n%s", out)
	}
	if !strings.Contains(out, "must be true or false") {
		t.Fatalf("output = %q, want boolean guidance", out)
	}
	if strings.Contains(out, "ldap-password") {
		t.Fatalf("wrapper output leaked LDAP password: %q", out)
	}
}

func runDiagnosisAuthLiveSmokeWrapper(t *testing.T, binDir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{"scripts/run_diagnosis_auth_live_smoke.sh"}, args...)...) // #nosec G204 -- test invokes a controlled repository script.
	cmd.Dir = openclarionRepoRoot(t)
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeDiagnosisAuthLiveSmokeFakeGo(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	writeDiagnosisAuthLiveSmokeFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "run" || "${2:-}" != "./scripts/diagnosis_auth_live_smoke" ]]; then
  exit 2
fi
printf '%s\n' "$@" >"$OPENCLARION_TEST_DIAGNOSIS_AUTH_ARGS_OUT"
`, 0o700)
	return binDir
}

func writeDiagnosisAuthLiveSmokeEnvFile(t *testing.T, dir string, values map[string]string) string {
	t.Helper()
	var body strings.Builder
	for key, value := range values {
		body.WriteString(key)
		body.WriteString("=")
		body.WriteString(diagnosisAuthLiveSmokeShellSingleQuote(value))
		body.WriteString("\n")
	}
	path := filepath.Join(dir, "diagnosis-auth-live.env")
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil { // #nosec G306,G703 -- test helper writes a private fixture env file.
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func readDiagnosisAuthLiveSmokeArgs(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads the args path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	return strings.Fields(string(raw))
}

func assertDiagnosisAuthLiveSmokeArg(t *testing.T, args []string, name, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name && args[i+1] == value {
			return
		}
	}
	t.Fatalf("args %q missing %s %s", strings.Join(args, " "), name, value)
}

func assertDiagnosisAuthLiveSmokeFlag(t *testing.T, args []string, name string) {
	t.Helper()
	for _, arg := range args {
		if arg == name {
			return
		}
	}
	t.Fatalf("args %q missing %s", strings.Join(args, " "), name)
}

func writeDiagnosisAuthLiveSmokeFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G306,G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func diagnosisAuthLiveSmokeShellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
