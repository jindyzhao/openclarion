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

func TestRunNotificationChannelLiveSmokeWrapperAcceptsExpandedKinds(t *testing.T) {
	for _, kind := range []string{"dingtalk", "feishu", "slack", "email"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			argsOut := filepath.Join(dir, "args.txt")
			binDir := writeNotificationChannelLiveSmokeFakeGo(t)
			envFile := writeNotificationChannelLiveSmokeEnvFile(t, dir, map[string]string{
				"OPENCLARION_LIVE_API_BASE_URL":                  "http://127.0.0.1:32102",
				"NOTIFICATION_CHANNEL_PROFILE_ID":                "7",
				"NOTIFICATION_CHANNEL_EXPECTED_KIND":             kind,
				"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND":     "transport_sample",
				"NOTIFICATION_CHANNEL_LIVE_SMOKE_OUTPUT":         filepath.Join(dir, "proof.json"),
				"OPENCLARION_TEST_NOTIFICATION_CHANNEL_ARGS_OUT": argsOut,
			})

			out, err := runNotificationChannelLiveSmokeWrapper(t, binDir, "--env-file", envFile)
			if err != nil {
				t.Fatalf("notification channel live smoke wrapper failed: %v\n%s", err, out)
			}
			args := readNotificationChannelLiveSmokeArgs(t, argsOut)
			assertNotificationChannelLiveSmokeArg(t, args, "--expected-kind", kind)
		})
	}
}

func TestRunNotificationChannelLiveSmokeWrapperRejectsUnsupportedKind(t *testing.T) {
	dir := t.TempDir()
	argsOut := filepath.Join(dir, "args.txt")
	binDir := writeNotificationChannelLiveSmokeFakeGo(t)
	envFile := writeNotificationChannelLiveSmokeEnvFile(t, dir, map[string]string{
		"OPENCLARION_LIVE_API_BASE_URL":                  "http://127.0.0.1:32102",
		"NOTIFICATION_CHANNEL_PROFILE_ID":                "7",
		"NOTIFICATION_CHANNEL_EXPECTED_KIND":             "pager",
		"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND":     "transport_sample",
		"NOTIFICATION_CHANNEL_LIVE_SMOKE_OUTPUT":         filepath.Join(dir, "proof.json"),
		"OPENCLARION_TEST_NOTIFICATION_CHANNEL_ARGS_OUT": argsOut,
	})

	out, err := runNotificationChannelLiveSmokeWrapper(t, binDir, "--env-file", envFile)
	if err == nil {
		t.Fatalf("notification channel live smoke wrapper succeeded; want rejection\n%s", out)
	}
	if !strings.Contains(out, "must be webhook, wecom, dingtalk, feishu, slack, or email") {
		t.Fatalf("output = %q, want expanded kind guidance", out)
	}
	if _, statErr := os.Stat(argsOut); !os.IsNotExist(statErr) {
		t.Fatalf("fake go args file stat err = %v, want not exist", statErr)
	}
}

func runNotificationChannelLiveSmokeWrapper(t *testing.T, binDir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{"scripts/run_notification_channel_live_smoke.sh"}, args...)...) // #nosec G204 -- test invokes a controlled repository script.
	cmd.Dir = openclarionRepoRoot(t)
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeNotificationChannelLiveSmokeFakeGo(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	writeNotificationChannelLiveSmokeFile(t, binDir, "go", `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "run" || "${2:-}" != "./scripts/notification_channel_live_smoke" ]]; then
  exit 2
fi
printf '%s\n' "$@" >"$OPENCLARION_TEST_NOTIFICATION_CHANNEL_ARGS_OUT"
`, 0o700)
	return binDir
}

func writeNotificationChannelLiveSmokeEnvFile(t *testing.T, dir string, values map[string]string) string {
	t.Helper()
	var body strings.Builder
	for key, value := range values {
		body.WriteString(key)
		body.WriteString("=")
		body.WriteString(notificationChannelLiveSmokeShellSingleQuote(value))
		body.WriteString("\n")
	}
	path := filepath.Join(dir, "notification-channel-live.env")
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil { // #nosec G306,G703 -- test helper writes a private fixture env file.
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func readNotificationChannelLiveSmokeArgs(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads the args path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	trimmed := strings.TrimSuffix(string(raw), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func assertNotificationChannelLiveSmokeArg(t *testing.T, args []string, name, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name && args[i+1] == value {
			return
		}
	}
	t.Fatalf("args %q missing %s %s", strings.Join(args, " "), name, value)
}

func writeNotificationChannelLiveSmokeFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G306,G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func notificationChannelLiveSmokeShellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
