package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStage5LocalWorkerCheckOnlyRequiresRuntimeNetwork(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
	binDir := writeStage5LocalWorkerFakeDocker(t, 1)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_SANDBOX_EGRESS_NETWORK must name an existing Docker network") {
		t.Fatalf("stage5-local-worker output = %q, want network readiness error", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyPassesAfterRuntimeNetworkCheck(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err != nil {
		t.Fatalf("stage5-local-worker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "runtime prerequisites are ready") {
		t.Fatalf("stage5-local-worker output = %q, want runtime readiness success", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRejectsInvalidBinaryOverride(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_STAGE5_WORKER_BINARY": filepath.Join(privateDir, "missing-openclarion"),
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_STAGE5_WORKER_BINARY must be a direct executable file") {
		t.Fatalf("stage5-local-worker output = %q, want binary readiness error", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerExecsBinaryOverride(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	workerBin := filepath.Join(privateDir, "openclarion")
	writeStage5LocalWorkerFile(t, privateDir, "openclarion", `#!/usr/bin/env bash
if [[ "$1" != "serve" ]]; then
  echo "unexpected args: $*" >&2
  exit 7
fi
echo "fake-openclarion-serve"
`, 0o700)
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_STAGE5_WORKER_BINARY": workerBin,
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir)
	if err != nil {
		t.Fatalf("stage5-local-worker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "fake-openclarion-serve") {
		t.Fatalf("stage5-local-worker output = %q, want binary execution", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyPullsMissingSandboxImage(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
	binDir := writeStage5LocalWorkerFakeDockerScript(t, `#!/usr/bin/env bash
if [[ "$1" == "network" && "$2" == "inspect" && "$3" == "openclarion-sandbox-allowlist" ]]; then
  exit 0
fi
if [[ "$1" == "image" && "$2" == "inspect" ]]; then
  exit 1
fi
if [[ "$1" == "pull" ]]; then
  exit 0
fi
exit 2
`)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err != nil {
		t.Fatalf("stage5-local-worker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "pulling digest-pinned sandbox image") {
		t.Fatalf("stage5-local-worker output = %q, want image pull notice", out)
	}
	if !strings.Contains(out, "runtime prerequisites are ready") {
		t.Fatalf("stage5-local-worker output = %q, want runtime readiness success", out)
	}
	if strings.Contains(out, "registry.example/openclarion/diagnosis") {
		t.Fatalf("stage5-local-worker leaked image ref in output: %q", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRejectsProfileOnlyNotificationConfig(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_IM_WEBHOOK_URL":                        "",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON": `{"secret/openclarion/report-webhook":"https://hooks.example.invalid/openclarion"}`,
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_IM_WEBHOOK_URL") {
		t.Fatalf("stage5-local-worker output = %q, want direct notification webhook requirement", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRequiresDirectNotificationWebhook(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_IM_WEBHOOK_URL":                        "",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON": "",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_IM_WEBHOOK_URL") {
		t.Fatalf("stage5-local-worker output = %q, want direct notification webhook requirement", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerRejectsRepoLocalEnvFile(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	envFile := writeStage5LocalWorkerEnv(t, root, map[string]string{})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "env file must live outside the repository or under .openclarion-private/") {
		t.Fatalf("stage5-local-worker output = %q, want repo-local env rejection", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerAllowsIgnoredRepoLocalPrivateEnvFile(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	requireStage5LocalWorkerGitFixture(t, root)
	writeStage5LocalWorkerFile(t, root, ".gitignore", "/.openclarion-private/\n", 0o644)
	privateDir := filepath.Join(root, ".openclarion-private")
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err != nil {
		t.Fatalf("stage5-local-worker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "runtime prerequisites are ready") {
		t.Fatalf("stage5-local-worker output = %q, want runtime readiness success", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerRejectsWeComWebhookWithoutFormat(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_IM_WEBHOOK_URL":    "https://qyapi.weixin.qq.com/cgi-bin/webhook/send",
		"OPENCLARION_IM_WEBHOOK_FORMAT": "",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "WeCom webhook endpoints require OPENCLARION_IM_WEBHOOK_FORMAT=wecom") {
		t.Fatalf("stage5-local-worker output = %q, want WeCom format rejection", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func newStage5LocalWorkerFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	raw, err := os.ReadFile(filepath.Join("run_stage5_local_worker.sh"))
	if err != nil {
		t.Fatalf("read run_stage5_local_worker.sh: %v", err)
	}
	writeStage5LocalWorkerFile(t, root, "scripts/run_stage5_local_worker.sh", string(raw), 0o755)
	lib, err := os.ReadFile(filepath.Join("lib_private_env.sh"))
	if err != nil {
		t.Fatalf("read lib_private_env.sh: %v", err)
	}
	writeStage5LocalWorkerFile(t, root, "scripts/lib_private_env.sh", string(lib), 0o755)
	return root
}

func writeStage5LocalWorkerEnv(t *testing.T, dir string, overrides map[string]string) string {
	t.Helper()
	agentDir := filepath.Join(dir, "agent-config")
	if err := os.MkdirAll(agentDir, 0o750); err != nil {
		t.Fatalf("mkdir agent config: %v", err)
	}
	values := map[string]string{
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL": "https://issuer.example.invalid",
		"OPENCLARION_SANDBOX_IMAGE_REF":         "registry.example/openclarion/diagnosis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT": agentDir,
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED":    "llm.example.invalid:443",
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":    "https://llm.example.invalid/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":     "not-a-secret-fixture",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":       "test-model",
		"OPENCLARION_IM_WEBHOOK_URL":            "https://hooks.example.invalid/openclarion",
	}
	for key, value := range overrides {
		values[key] = value
	}
	var body strings.Builder
	keys := []string{
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL",
		"OPENCLARION_SANDBOX_IMAGE_REF",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT",
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED",
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL",
		"OPENCLARION_IM_WEBHOOK_URL",
		"OPENCLARION_IM_WEBHOOK_FORMAT",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
		"OPENCLARION_STAGE5_WORKER_BINARY",
	}
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		body.WriteString(key)
		body.WriteString("='")
		body.WriteString(value)
		body.WriteString("'\n")
	}
	path := filepath.Join(dir, "stage5.env")
	if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func writeStage5LocalWorkerFakeDocker(t *testing.T, exitCode int) string {
	t.Helper()
	body := `#!/usr/bin/env bash
if [[ "$1" == "network" && "$2" == "inspect" && "$3" == "openclarion-sandbox-allowlist" ]]; then
  exit ` + strconv.Itoa(exitCode) + `
fi
if [[ "$1" == "image" && "$2" == "inspect" ]]; then
  exit 0
fi
if [[ "$1" == "pull" ]]; then
  exit 0
fi
exit 2
`
	return writeStage5LocalWorkerFakeDockerScript(t, body)
}

func writeStage5LocalWorkerFakeDockerScript(t *testing.T, body string) string {
	t.Helper()
	binDir := t.TempDir()
	writeStage5LocalWorkerFile(t, binDir, "docker", body, 0o755)
	return binDir
}

func runStage5LocalWorker(t *testing.T, root, envFile, binDir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmdArgs := append([]string{"scripts/run_stage5_local_worker.sh"}, args...)
	cmd := exec.CommandContext(ctx, "bash", cmdArgs...) // #nosec G204 -- test invokes a controlled fixture script.
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCLARION_STAGE5_WORKER_ENV_FILE="+envFile,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeStage5LocalWorkerFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil { // #nosec G703 -- test helper writes controlled fixture paths.
		t.Fatalf("write %s: %v", path, err)
	}
}

func requireStage5LocalWorkerGitFixture(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for repo-local private env checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "init", "-q") // #nosec G204 -- test initializes a controlled fixture repository.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(out))
	}
}

func assertStage5LocalWorkerNoSecretLeak(t *testing.T, out string) {
	t.Helper()
	for _, forbidden := range []string{
		"not-a-secret-fixture",
		"https://llm.example.invalid/v1",
		"https://hooks.example.invalid/openclarion",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("stage5-local-worker leaked %q in output: %q", forbidden, out)
		}
	}
}
