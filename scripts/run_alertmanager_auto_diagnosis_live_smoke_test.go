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

func TestRunAlertmanagerAutoDiagnosisSmokeDefaultsToAssistantMessage(t *testing.T) {
	binDir := writeAlertmanagerAutoDiagnosisFakeGo(t)
	fixtureDir := t.TempDir()
	privateDir := alertmanagerAutoDiagnosisRepoPrivateDir(t)
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeAlertmanagerAutoDiagnosisEnvFile(t, fixtureDir, map[string]string{
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR": privateDir,
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID":         "42",
		"OPENCLARION_LIVE_API_BASE_URL":                  "http://127.0.0.1:32101",
	})

	out, err := runAlertmanagerAutoDiagnosisSmoke(t, binDir, argsOut, "--env-file", envFile)
	if err != nil {
		t.Fatalf("auto-diagnosis smoke wrapper failed: %v\n%s", err, out)
	}
	args := readAlertmanagerAutoDiagnosisArgs(t, argsOut)
	assertAlertmanagerAutoDiagnosisArg(t, args, "--expected-content-kind", "assistant_message")
	assertAlertmanagerAutoDiagnosisArg(t, args, "--required-content-kinds", "assistant_message")
}

func TestRunAlertmanagerAutoDiagnosisSmokeAllowsAssistantMessageOverride(t *testing.T) {
	binDir := writeAlertmanagerAutoDiagnosisFakeGo(t)
	fixtureDir := t.TempDir()
	privateDir := alertmanagerAutoDiagnosisRepoPrivateDir(t)
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeAlertmanagerAutoDiagnosisEnvFile(t, fixtureDir, map[string]string{
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND": " Assistant_Message ",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR":    privateDir,
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID":            "42",
		"OPENCLARION_LIVE_API_BASE_URL":                     "http://127.0.0.1:32101",
	})

	out, err := runAlertmanagerAutoDiagnosisSmoke(t, binDir, argsOut, "--env-file", envFile)
	if err != nil {
		t.Fatalf("auto-diagnosis smoke wrapper failed: %v\n%s", err, out)
	}
	args := readAlertmanagerAutoDiagnosisArgs(t, argsOut)
	assertAlertmanagerAutoDiagnosisArg(t, args, "--expected-content-kind", "assistant_message")
}

func TestRunAlertmanagerAutoDiagnosisSmokePassesRequiredContentKinds(t *testing.T) {
	binDir := writeAlertmanagerAutoDiagnosisFakeGo(t)
	fixtureDir := t.TempDir()
	privateDir := alertmanagerAutoDiagnosisRepoPrivateDir(t)
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeAlertmanagerAutoDiagnosisEnvFile(t, fixtureDir, map[string]string{
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR":     privateDir,
		"ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS": " final_conclusion, Assistant_Message ",
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID":             "42",
		"OPENCLARION_LIVE_API_BASE_URL":                      "http://127.0.0.1:32101",
	})

	out, err := runAlertmanagerAutoDiagnosisSmoke(t, binDir, argsOut, "--env-file", envFile)
	if err != nil {
		t.Fatalf("auto-diagnosis smoke wrapper failed: %v\n%s", err, out)
	}
	args := readAlertmanagerAutoDiagnosisArgs(t, argsOut)
	assertAlertmanagerAutoDiagnosisArg(t, args, "--required-content-kinds", "final_conclusion,assistant_message")
}

func TestRunAlertmanagerAutoDiagnosisSmokeRejectsBadRequiredContentKinds(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "unknown",
			value: "raw_alert",
			want:  "must be assistant_message or final_conclusion",
		},
		{
			name:  "duplicate",
			value: "assistant_message,assistant_message",
			want:  "must not contain duplicate values",
		},
		{
			name:  "empty",
			value: "assistant_message,,final_conclusion",
			want:  "must not contain empty values",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binDir := writeAlertmanagerAutoDiagnosisFakeGo(t)
			fixtureDir := t.TempDir()
			privateDir := alertmanagerAutoDiagnosisRepoPrivateDir(t)
			argsOut := filepath.Join(fixtureDir, "args.txt")
			envFile := writeAlertmanagerAutoDiagnosisEnvFile(t, fixtureDir, map[string]string{
				"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR":     privateDir,
				"ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS": tc.value,
				"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID":             "42",
				"OPENCLARION_LIVE_API_BASE_URL":                      "http://127.0.0.1:32101",
			})

			out, err := runAlertmanagerAutoDiagnosisSmoke(t, binDir, argsOut, "--env-file", envFile)
			if err == nil {
				t.Fatalf("auto-diagnosis smoke wrapper succeeded; want rejection\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestRunAlertmanagerAutoDiagnosisSmokeRejectsRepoLocalPublicOutput(t *testing.T) {
	binDir := writeAlertmanagerAutoDiagnosisFakeGo(t)
	fixtureDir := t.TempDir()
	repoRoot := openclarionRepoRoot(t)
	output := filepath.Join(repoRoot, "alertmanager-auto-diagnosis-public-output.json")
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeAlertmanagerAutoDiagnosisEnvFile(t, fixtureDir, map[string]string{
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT": output,
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID":        "42",
		"OPENCLARION_LIVE_API_BASE_URL":                 "http://127.0.0.1:32101",
	})
	t.Cleanup(func() {
		_ = os.Remove(output)
	})

	out, err := runAlertmanagerAutoDiagnosisSmoke(t, binDir, argsOut, "--env-file", envFile)
	if err == nil {
		t.Fatalf("auto-diagnosis smoke wrapper succeeded; want rejection\n%s", out)
	}
	if !strings.Contains(out, "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT must live under repo-local ignored .openclarion-private/") {
		t.Fatalf("output = %q, want repo-local output rejection", out)
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("public output was created; stat error = %v", statErr)
	}
	if _, statErr := os.Stat(argsOut); !os.IsNotExist(statErr) {
		t.Fatalf("fake go was invoked; args output stat error = %v", statErr)
	}
}

func TestRunAlertmanagerAutoDiagnosisSmokeRejectsRepoLocalPublicWorkdirBeforeMktemp(t *testing.T) {
	binDir := writeAlertmanagerAutoDiagnosisFakeGo(t)
	fixtureDir := t.TempDir()
	repoRoot := openclarionRepoRoot(t)
	workdir := filepath.Join(repoRoot, "alertmanager-auto-diagnosis-public-workdir")
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeAlertmanagerAutoDiagnosisEnvFile(t, fixtureDir, map[string]string{
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR": workdir,
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID":         "42",
		"OPENCLARION_LIVE_API_BASE_URL":                  "http://127.0.0.1:32101",
	})
	t.Cleanup(func() {
		_ = os.RemoveAll(workdir)
	})

	out, err := runAlertmanagerAutoDiagnosisSmoke(t, binDir, argsOut, "--env-file", envFile)
	if err == nil {
		t.Fatalf("auto-diagnosis smoke wrapper succeeded; want rejection\n%s", out)
	}
	if !strings.Contains(out, "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR must live under repo-local ignored .openclarion-private/") {
		t.Fatalf("output = %q, want repo-local workdir rejection", out)
	}
	if _, statErr := os.Stat(workdir); !os.IsNotExist(statErr) {
		t.Fatalf("public workdir was created before validation; stat error = %v", statErr)
	}
	if _, statErr := os.Stat(argsOut); !os.IsNotExist(statErr) {
		t.Fatalf("fake go was invoked; args output stat error = %v", statErr)
	}
}

func TestRunAlertmanagerAutoDiagnosisSmokeRejectsExternalOutput(t *testing.T) {
	binDir := writeAlertmanagerAutoDiagnosisFakeGo(t)
	fixtureDir := t.TempDir()
	output := filepath.Join(fixtureDir, "external-output.json")
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeAlertmanagerAutoDiagnosisEnvFile(t, fixtureDir, map[string]string{
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT": output,
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID":        "42",
		"OPENCLARION_LIVE_API_BASE_URL":                 "http://127.0.0.1:32101",
	})

	out, err := runAlertmanagerAutoDiagnosisSmoke(t, binDir, argsOut, "--env-file", envFile)
	if err == nil {
		t.Fatalf("auto-diagnosis smoke wrapper succeeded; want rejection\n%s", out)
	}
	if !strings.Contains(out, "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_OUTPUT must live under repo-local ignored .openclarion-private/") {
		t.Fatalf("output = %q, want external output rejection", out)
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("external output was created; stat error = %v", statErr)
	}
	if _, statErr := os.Stat(argsOut); !os.IsNotExist(statErr) {
		t.Fatalf("fake go was invoked; args output stat error = %v", statErr)
	}
}

func TestRunAlertmanagerAutoDiagnosisSmokeRejectsExternalWorkdirBeforeMktemp(t *testing.T) {
	binDir := writeAlertmanagerAutoDiagnosisFakeGo(t)
	fixtureDir := t.TempDir()
	workdir := filepath.Join(fixtureDir, "external-workdir")
	argsOut := filepath.Join(fixtureDir, "args.txt")
	envFile := writeAlertmanagerAutoDiagnosisEnvFile(t, fixtureDir, map[string]string{
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR": workdir,
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID":         "42",
		"OPENCLARION_LIVE_API_BASE_URL":                  "http://127.0.0.1:32101",
	})

	out, err := runAlertmanagerAutoDiagnosisSmoke(t, binDir, argsOut, "--env-file", envFile)
	if err == nil {
		t.Fatalf("auto-diagnosis smoke wrapper succeeded; want rejection\n%s", out)
	}
	if !strings.Contains(out, "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_WORKDIR must live under repo-local ignored .openclarion-private/") {
		t.Fatalf("output = %q, want external workdir rejection", out)
	}
	if _, statErr := os.Stat(workdir); !os.IsNotExist(statErr) {
		t.Fatalf("external workdir was created before validation; stat error = %v", statErr)
	}
	if _, statErr := os.Stat(argsOut); !os.IsNotExist(statErr) {
		t.Fatalf("fake go was invoked; args output stat error = %v", statErr)
	}
}

func runAlertmanagerAutoDiagnosisSmoke(t *testing.T, binDir, argsOut string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{"scripts/run_alertmanager_auto_diagnosis_live_smoke.sh"}, args...)...) // #nosec G204 -- test invokes a controlled repository script.
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCLARION_TEST_ALERTMANAGER_AUTO_DIAGNOSIS_ARGS_OUT="+argsOut,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeAlertmanagerAutoDiagnosisFakeGo(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	writeAlertmanagerAutoDiagnosisFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "run" || "${2:-}" != "./scripts/alertmanager_auto_diagnosis_live_smoke" ]]; then
  exit 2
fi
printf '%s\n' "$@" >"$OPENCLARION_TEST_ALERTMANAGER_AUTO_DIAGNOSIS_ARGS_OUT"
`, 0o700)
	return binDir
}

func writeAlertmanagerAutoDiagnosisEnvFile(t *testing.T, dir string, values map[string]string) string {
	t.Helper()
	var body strings.Builder
	for key, value := range values {
		body.WriteString(key)
		body.WriteString("=")
		body.WriteString(alertmanagerAutoDiagnosisShellSingleQuote(value))
		body.WriteString("\n")
	}
	path := filepath.Join(dir, "live.env")
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil { // #nosec G306,G703 -- test helper writes a private fixture env file.
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func readAlertmanagerAutoDiagnosisArgs(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads the args path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	return strings.Fields(string(raw))
}

func assertAlertmanagerAutoDiagnosisArg(t *testing.T, args []string, name, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name && args[i+1] == value {
			return
		}
	}
	t.Fatalf("args %q missing %s %s", strings.Join(args, " "), name, value)
}

func alertmanagerAutoDiagnosisRepoPrivateDir(t *testing.T) string {
	t.Helper()
	root := openclarionRepoRoot(t)
	parent := filepath.Join(root, ".openclarion-private")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatalf("mkdir repo private dir: %v", err)
	}
	dir, err := os.MkdirTemp(parent, "alertmanager-auto-diagnosis-test-")
	if err != nil {
		t.Fatalf("mktemp repo private dir: %v", err)
	}
	// #nosec G302 -- private test directory requires execute permission.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod repo private dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func writeAlertmanagerAutoDiagnosisFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G306,G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func alertmanagerAutoDiagnosisShellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
