package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLocalProductHelpAndMissingEnv(t *testing.T) {
	help := exec.CommandContext(t.Context(), "bash", "run_local_product.sh", "--help")
	help.Dir = "."
	output, err := help.CombinedOutput()
	if err != nil {
		t.Fatalf("--help: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "complete local product without Kubernetes") {
		t.Fatalf("help output = %q", output)
	}

	missing := exec.CommandContext(t.Context(), "bash", "run_local_product.sh")
	missing.Dir = "."
	missing.Env = filteredLocalProductEnvironment()
	output, err = missing.CombinedOutput()
	if err == nil {
		t.Fatal("missing env err = nil, want failure")
	}
	if !strings.Contains(string(output), "set OPENCLARION_LOCAL_PRODUCT_ENV_FILE or pass --env-file") {
		t.Fatalf("missing env output = %q", output)
	}
}

func TestRunLocalProductCheckUsesIsolatedComposeDatabaseAndBoundedWaits(t *testing.T) {
	fixture := newLocalProductFixture(t)
	output := fixture.run(t, "--check-only")

	dockerArgs := fixture.readCapture(t, "docker.args")
	for _, want := range []string{
		"compose --project-name openclarion-local-product --profile sandbox-egress up -d --wait --wait-timeout 180",
		"compose --project-name openclarion-local-product exec -T postgres",
	} {
		if !strings.Contains(dockerArgs, want) {
			t.Fatalf("Docker args missing %q:\n%s", want, dockerArgs)
		}
	}
	atlas := fixture.readCapture(t, "atlas.env")
	for _, want := range []string{
		"url=postgres://openclarion:openclarion_dev@postgres:5432/openclarion_local_product?sslmode=disable",
		"network=openclarion-local-product_default",
	} {
		if !strings.Contains(atlas, want) {
			t.Fatalf("Atlas environment missing %q:\n%s", want, atlas)
		}
	}
	stage5 := fixture.readCapture(t, "stage5.env")
	for _, want := range []string{
		"database=postgres://openclarion:openclarion_dev@127.0.0.1:25432/openclarion_local_product?sslmode=disable",
		"temporal=127.0.0.1:27233",
		"listen=127.0.0.1:32101",
		"network=openclarion-local-product-sandbox-allowlist",
		"origins=http://existing.test,http://127.0.0.1:3000",
		"args=--env-file " + fixture.envFile + " --source --check-only",
	} {
		if !strings.Contains(stage5, want) {
			t.Fatalf("Stage 5 environment missing %q:\n%s", want, stage5)
		}
	}
	if strings.Contains(string(output), "stale-private-value") {
		t.Fatalf("launcher leaked stale private database URL: %s", output)
	}
	if _, err := os.Stat(filepath.Join(fixture.captureDir, "npm.env")); !os.IsNotExist(err) {
		t.Fatalf("check-only started npm unexpectedly: %v", err)
	}
}

func TestRunLocalProductStartsFrontendWithMatchingPublicURLs(t *testing.T) {
	fixture := newLocalProductFixture(t)
	output := fixture.runWithEnvironment(t, []string{"OPENCLARION_LOCAL_WEB_PORT=3300"})

	if !strings.Contains(string(output), "frontend: http://127.0.0.1:3300") {
		t.Fatalf("output = %q, want frontend URL", output)
	}
	npm := fixture.readCapture(t, "npm.env")
	for _, want := range []string{
		"api=http://127.0.0.1:32101",
		"ws=http://127.0.0.1:32101",
		"public=http://127.0.0.1:32101",
		"backend_secret_present=",
		"oidc_secret_present=x",
		"args=run dev -- --hostname 127.0.0.1 --port 3300",
	} {
		if !strings.Contains(npm, want) {
			t.Fatalf("npm environment missing %q:\n%s", want, npm)
		}
	}
	if strings.Contains(npm, "backend_secret_present=x") {
		t.Fatalf("frontend inherited a backend-only secret variable:\n%s", npm)
	}
	stage5 := fixture.readCapture(t, "stage5.env")
	if !strings.Contains(stage5, "origins=http://existing.test,http://127.0.0.1:3300") {
		t.Fatalf("Stage 5 origins do not include the frontend: %s", stage5)
	}
}

func TestRunLocalProductConfiguredDatabaseSkipsLocalApplicationDatabase(t *testing.T) {
	fixture := newLocalProductFixture(t)
	databaseURL := strings.Join([]string{
		"postgres://operator:", "configured-private-value", "@database.test/openclarion?sslmode=require",
	}, "")
	output := fixture.runWithEnvironment(t, []string{
		"OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE=1",
		"OPENCLARION_LOCAL_ATLAS_DOCKER_NETWORK=database_network",
		"DATABASE_URL=" + databaseURL,
	}, "--check-only")

	dockerArgs := fixture.readCapture(t, "docker.args")
	if strings.Contains(dockerArgs, "exec -T postgres") {
		t.Fatalf("configured database mode mutated the local application database:\n%s", dockerArgs)
	}
	atlas := fixture.readCapture(t, "atlas.env")
	if !strings.Contains(atlas, "url="+databaseURL) || !strings.Contains(atlas, "network=database_network") {
		t.Fatalf("configured Atlas environment = %q", atlas)
	}
	if strings.Contains(string(output), "configured-private-value") {
		t.Fatalf("configured database URL leaked through launcher output: %s", output)
	}
}

func TestRunLocalProductRejectsUnsupportedComposeAndInvalidPort(t *testing.T) {
	tests := []struct {
		name        string
		environment []string
		want        string
	}{
		{
			name:        "old Compose",
			environment: []string{"OPENCLARION_TEST_COMPOSE_VERSION=2.32.4"},
			want:        "Docker Compose 2.33.1 or newer is required",
		},
		{
			name:        "port out of range",
			environment: []string{"OPENCLARION_LOCAL_API_PORT=65536"},
			want:        "OPENCLARION_LOCAL_API_PORT must be between 1 and 65535",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newLocalProductFixture(t)
			output, err := fixture.command(t, tt.environment, "--check-only").CombinedOutput()
			if err == nil {
				t.Fatalf("launcher passed unexpectedly:\n%s", output)
			}
			if !strings.Contains(string(output), tt.want) {
				t.Fatalf("output = %q, want %q", output, tt.want)
			}
		})
	}
}

type localProductFixture struct {
	root       string
	envFile    string
	binDir     string
	captureDir string
}

func newLocalProductFixture(t *testing.T) localProductFixture {
	t.Helper()
	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "checkout")
	binDir := filepath.Join(tempDir, "bin")
	captureDir := filepath.Join(tempDir, "capture")
	for _, dir := range []string{
		filepath.Join(root, "scripts"),
		filepath.Join(root, "config", "agents", "diagnosis-assistant"),
		filepath.Join(root, "web", "node_modules", ".bin"),
		binDir,
		captureDir,
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	copyLocalProductFile(t, "run_local_product.sh", filepath.Join(root, "scripts", "run_local_product.sh"), 0o755)
	copyLocalProductFile(t, "lib_private_env.sh", filepath.Join(root, "scripts", "lib_private_env.sh"), 0o644)
	writeLocalProductFile(t, filepath.Join(root, "web", "package-lock.json"), "{}\n", 0o644)
	writeLocalProductFile(t, filepath.Join(root, "web", "node_modules", ".bin", "next"), "#!/bin/sh\nexit 0\n", 0o755)
	writeLocalProductFile(t, filepath.Join(root, "config", "agents", "diagnosis-assistant", "instructions.md"), "fixture\n", 0o644)

	writeLocalProductFile(t, filepath.Join(binDir, "docker"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$OPENCLARION_TEST_CAPTURE_DIR/docker.args"
if [ "$*" = "compose version --short" ]; then
  printf '%s\n' "${OPENCLARION_TEST_COMPOSE_VERSION:-2.33.1}"
  exit 0
fi
case "$*" in
  *"SELECT 1 FROM pg_database"*) printf '1\n' ;;
esac
`, 0o755)
	writeLocalProductFile(t, filepath.Join(binDir, "curl"), "#!/bin/sh\nexit 0\n", 0o755)
	writeLocalProductFile(t, filepath.Join(binDir, "go"), "#!/bin/sh\nexit 0\n", 0o755)
	writeLocalProductFile(t, filepath.Join(binDir, "npm"), `#!/bin/sh
set -eu
{
  printf 'api=%s\n' "${OPENCLARION_API_BASE_URL:-}"
  printf 'ws=%s\n' "${OPENCLARION_BROWSER_WS_BASE_URL:-}"
  printf 'public=%s\n' "${NEXT_PUBLIC_OPENCLARION_API_PUBLIC_BASE_URL:-}"
  printf 'backend_secret_present=%s\n' "${OPENCLARION_LLM_API_KEY+x}"
  printf 'oidc_secret_present=%s\n' "${OIDC_CLIENT_SECRET+x}"
  printf 'args=%s\n' "$*"
} >"$HOME/npm.env"
sleep 2
`, 0o755)

	writeLocalProductFile(t, filepath.Join(root, "scripts", "build_local_egress_proxy.sh"), "#!/bin/sh\nexit 0\n", 0o755)
	writeLocalProductFile(t, filepath.Join(root, "scripts", "build_diagnosis_assistant_runner.sh"), `#!/bin/sh
printf 'localhost:5000/openclarion/diagnosis-assistant-runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n'
`, 0o755)
	writeLocalProductFile(t, filepath.Join(root, "scripts", "apply_atlas_migrations.sh"), `#!/bin/sh
set -eu
{
  printf 'url=%s\n' "$OPENCLARION_ATLAS_DATABASE_URL"
  printf 'network=%s\n' "$OPENCLARION_ATLAS_DOCKER_NETWORK"
} >"$OPENCLARION_TEST_CAPTURE_DIR/atlas.env"
`, 0o755)
	writeLocalProductFile(t, filepath.Join(root, "scripts", "run_stage5_local_worker.sh"), `#!/bin/sh
set -eu
{
  printf 'database=%s\n' "${DATABASE_URL:-}"
  printf 'temporal=%s\n' "${TEMPORAL_HOST_PORT:-}"
  printf 'listen=%s\n' "${LISTEN_ADDR:-}"
  printf 'network=%s\n' "${OPENCLARION_SANDBOX_EGRESS_NETWORK:-}"
  printf 'origins=%s\n' "${OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS:-}"
  printf 'args=%s\n' "$*"
} >>"$OPENCLARION_TEST_CAPTURE_DIR/stage5.env"
case "$*" in
  *--check-only*) exit 0 ;;
esac
sleep 1
`, 0o755)

	envFile := filepath.Join(tempDir, "operator.env")
	writeLocalProductFile(t, envFile, strings.Join([]string{
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED='llm.example.test:443'",
		"OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS='http://existing.test'",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT='config/agents'",
		"OPENCLARION_LLM_API_KEY='backend-only-private-value'",
		"OIDC_CLIENT_SECRET='frontend-private-value'",
		"DATABASE_URL='postgres://operator:stale-private-value@stale.test/stale'",
		"TEMPORAL_HOST_PORT='stale.test:1'",
		"LISTEN_ADDR='0.0.0.0:1'",
		"",
	}, "\n"), 0o600)

	return localProductFixture{root: root, envFile: envFile, binDir: binDir, captureDir: captureDir}
}

func (f localProductFixture) run(t *testing.T, args ...string) []byte {
	t.Helper()
	return f.runWithEnvironment(t, nil, args...)
}

func (f localProductFixture) runWithEnvironment(t *testing.T, environment []string, args ...string) []byte {
	t.Helper()
	output, err := f.command(t, environment, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("run local product: %v\n%s", err, output)
	}
	return output
}

func (f localProductFixture) command(t *testing.T, environment []string, args ...string) *exec.Cmd {
	t.Helper()
	commandArgs := append([]string{"scripts/run_local_product.sh", "--env-file", f.envFile}, args...)
	cmd := exec.CommandContext(t.Context(), "bash", commandArgs...) // #nosec G204 -- args are test-owned fixture values.
	cmd.Dir = f.root
	cmd.Env = append(filteredLocalProductEnvironment(),
		"PATH="+f.binDir+":"+os.Getenv("PATH"),
		"HOME="+f.captureDir,
		"OPENCLARION_TEST_CAPTURE_DIR="+f.captureDir,
	)
	cmd.Env = append(cmd.Env, environment...)
	return cmd
}

func (f localProductFixture) readCapture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.captureDir, name)) // #nosec G304 -- fixture capture path is test-owned.
	if err != nil {
		t.Fatalf("read capture %s: %v", name, err)
	}
	return string(raw)
}

func copyLocalProductFile(t *testing.T, source, target string, mode os.FileMode) {
	t.Helper()
	raw, err := os.ReadFile(source) // #nosec G304 -- source is one of two repository-owned fixture scripts.
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}
	writeLocalProductFile(t, target, string(raw), mode)
}

func writeLocalProductFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil { // #nosec G306,G703 -- path stays under a test-owned temporary root.
		t.Fatalf("write %s: %v", path, err)
	}
}

func filteredLocalProductEnvironment() []string {
	blockedPrefixes := []string{
		"ATLAS_IMAGE=",
		"DATABASE_URL=",
		"LISTEN_ADDR=",
		"OPENCLARION_",
		"POSTGRES_PORT=",
		"TEMPORAL_HOST_PORT=",
		"TEMPORAL_PORT=",
		"TEMPORAL_UI_PORT=",
	}
	out := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		blocked := false
		for _, prefix := range blockedPrefixes {
			if strings.HasPrefix(entry, prefix) {
				blocked = true
				break
			}
		}
		if !blocked {
			out = append(out, entry)
		}
	}
	return out
}
