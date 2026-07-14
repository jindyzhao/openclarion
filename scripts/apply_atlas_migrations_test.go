package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyAtlasMigrationsRunsValidatedHardenedSequence(t *testing.T) {
	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "docker.args")
	writeApplyAtlasExecutable(t, filepath.Join(tempDir, "docker"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$OPENCLARION_TEST_DOCKER_CAPTURE"
if [ "$1" = "network" ]; then
  [ "$2" = "inspect" ]
  [ "$3" = "product_default" ]
  exit 0
fi
[ "$1" = "run" ]
[ -n "${ATLAS_DATABASE_URL:-}" ]
`)

	databaseURL := strings.Join([]string{
		"postgres://operator:", "private-value", "@database.test/openclarion?sslmode=disable",
	}, "")
	cmd := exec.CommandContext(t.Context(), "bash", "apply_atlas_migrations.sh")
	cmd.Dir = "."
	cmd.Env = append(filteredApplyAtlasEnvironment(),
		"PATH="+tempDir+":"+os.Getenv("PATH"),
		"DATABASE_URL="+databaseURL,
		"OPENCLARION_ATLAS_DOCKER_NETWORK=product_default",
		"OPENCLARION_ATLAS_TIMEOUT_SECONDS=45",
		"OPENCLARION_TEST_DOCKER_CAPTURE="+capturePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply migrations: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(capturePath) // #nosec G304 -- capturePath is derived from t.TempDir.
	if err != nil {
		t.Fatalf("read Docker capture: %v", err)
	}
	args := string(raw)
	for _, command := range []string{
		"migrate validate --env runtime",
		"migrate apply --env runtime",
		"migrate status --env runtime",
	} {
		if !strings.Contains(args, command) {
			t.Fatalf("Docker args missing %q:\n%s", command, args)
		}
	}
	for _, flag := range []string{
		"--network product_default",
		"--read-only",
		"--cap-drop ALL",
		"--security-opt no-new-privileges",
		"--tmpfs /tmp:rw,noexec,nosuid,nodev,size=64m,mode=1777",
		"--env HOME=/tmp",
		"--env ATLAS_DATABASE_URL",
	} {
		if !strings.Contains(args, flag) {
			t.Fatalf("Docker args missing hardening flag %q:\n%s", flag, args)
		}
	}
	if strings.Count(args, "migrate ") != 3 {
		t.Fatalf("Docker args = %q, want exactly three Atlas invocations", args)
	}
	if strings.Contains(args, databaseURL) || strings.Contains(string(output), databaseURL) ||
		strings.Contains(args, "private-value") || strings.Contains(string(output), "private-value") {
		t.Fatal("database URL leaked through command arguments or output")
	}
}

func TestApplyAtlasMigrationsRejectsInvalidConfiguration(t *testing.T) {
	tempDir := t.TempDir()
	writeApplyAtlasExecutable(t, filepath.Join(tempDir, "docker"), "#!/bin/sh\nexit 99\n")

	tests := []struct {
		name        string
		environment []string
		want        string
	}{
		{
			name: "missing database URL",
			want: "set DATABASE_URL or OPENCLARION_ATLAS_DATABASE_URL",
		},
		{
			name:        "URL whitespace",
			environment: []string{"DATABASE_URL=postgres://database.test/open clarion"},
			want:        "database URL must not contain whitespace",
		},
		{
			name:        "URL scheme",
			environment: []string{"DATABASE_URL=mysql://database.test/openclarion"},
			want:        "database URL must use the postgres or postgresql scheme",
		},
		{
			name: "network name",
			environment: []string{
				"DATABASE_URL=postgres://database.test/openclarion",
				"OPENCLARION_ATLAS_DOCKER_NETWORK=--invalid",
			},
			want: "Docker network name is invalid",
		},
		{
			name: "network none",
			environment: []string{
				"DATABASE_URL=postgres://database.test/openclarion",
				"OPENCLARION_ATLAS_DOCKER_NETWORK=none",
			},
			want: "Docker network must permit database access",
		},
		{
			name: "mutable image",
			environment: []string{
				"DATABASE_URL=postgres://database.test/openclarion",
				"ATLAS_IMAGE=arigaio/atlas:latest",
			},
			want: "ATLAS_IMAGE must use a lowercase repository and concrete semantic-version tag",
		},
		{
			name: "timeout bound",
			environment: []string{
				"DATABASE_URL=postgres://database.test/openclarion",
				"OPENCLARION_ATLAS_TIMEOUT_SECONDS=0",
			},
			want: "OPENCLARION_ATLAS_TIMEOUT_SECONDS must be between 1 and 1800",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), "bash", "apply_atlas_migrations.sh")
			cmd.Dir = "."
			cmd.Env = append(filteredApplyAtlasEnvironment(), "PATH="+tempDir+":"+os.Getenv("PATH"))
			cmd.Env = append(cmd.Env, tt.environment...)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("apply migrations passed unexpectedly:\n%s", output)
			}
			if !strings.Contains(string(output), tt.want) {
				t.Fatalf("output = %q, want %q", output, tt.want)
			}
		})
	}
}

func TestApplyAtlasMigrationsFailsClosedWhenNetworkIsAbsent(t *testing.T) {
	tempDir := t.TempDir()
	writeApplyAtlasExecutable(t, filepath.Join(tempDir, "docker"), `#!/bin/sh
set -eu
if [ "$1" = "network" ] && [ "$2" = "inspect" ]; then
  exit 1
fi
exit 90
`)
	cmd := exec.CommandContext(t.Context(), "bash", "apply_atlas_migrations.sh")
	cmd.Dir = "."
	cmd.Env = append(filteredApplyAtlasEnvironment(),
		"PATH="+tempDir+":"+os.Getenv("PATH"),
		"DATABASE_URL=postgres://database.test/openclarion",
		"OPENCLARION_ATLAS_DOCKER_NETWORK=missing_network",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("apply migrations passed unexpectedly:\n%s", output)
	}
	if !strings.Contains(string(output), "Docker network does not exist") {
		t.Fatalf("output = %q, want missing network failure", output)
	}
}

func TestAtlasRuntimeConfigUsesEnvironmentURLAndLockTimeout(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "atlas.hcl"))
	if err != nil {
		t.Fatalf("read atlas.hcl: %v", err)
	}
	config := string(raw)
	for _, want := range []string{
		`url = getenv("ATLAS_DATABASE_URL")`,
		`dir          = "file://internal/persistence/migrations"`,
		`lock_timeout = "10s"`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("atlas.hcl missing %q:\n%s", want, config)
		}
	}
}

func writeApplyAtlasExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil { // #nosec G306 -- test shims must be executable.
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func filteredApplyAtlasEnvironment() []string {
	blocked := map[string]bool{
		"ATLAS_IMAGE":                       true,
		"DATABASE_URL":                      true,
		"OPENCLARION_ATLAS_DATABASE_URL":    true,
		"OPENCLARION_ATLAS_DOCKER_NETWORK":  true,
		"OPENCLARION_ATLAS_TIMEOUT_SECONDS": true,
	}
	out := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			out = append(out, entry)
		}
	}
	return out
}
