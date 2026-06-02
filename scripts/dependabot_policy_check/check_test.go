package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsCurrentPolicy(t *testing.T) {
	path := writePolicy(t, validPolicy())

	var out bytes.Buffer
	if err := run(path, &out); err != nil {
		t.Fatalf("run() error = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "[dependabot-policy] OK") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestRunRejectsMissingFrontendMajorIgnore(t *testing.T) {
	policy := strings.Replace(validPolicy(), `      - dependency-name: "typescript"
        update-types:
          - "version-update:semver-major"
`, "", 1)
	path := writePolicy(t, policy)

	var out bytes.Buffer
	err := run(path, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), "npm /web ignore typescript: missing ignore entry")
}

func TestRunRejectsFrontendMajorIgnoreThatBlocksSecurityVersions(t *testing.T) {
	policy := strings.Replace(validPolicy(), `      - dependency-name: "eslint"
        update-types:
          - "version-update:semver-major"
`, `      - dependency-name: "eslint"
        update-types:
          - "version-update:semver-major"
        versions: ["10.x"]
`, 1)
	path := writePolicy(t, policy)

	var out bytes.Buffer
	err := run(path, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), "npm /web ignore eslint: versions must stay empty")
}

func TestRunRejectsMissingSecurityGroup(t *testing.T) {
	policy := strings.Replace(validPolicy(), `      web-security:
        applies-to: "security-updates"
        patterns:
          - "*"
`, "", 1)
	path := writePolicy(t, policy)

	var out bytes.Buffer
	err := run(path, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), "npm /web group web-security: missing security-update group")
}

func TestRunRejectsLinterToolsMajorMinorDrift(t *testing.T) {
	policy := strings.Replace(validPolicy(), `      - dependency-name: "golang.org/x/tools"
        update-types:
          - "version-update:semver-minor"
          - "version-update:semver-major"
`, `      - dependency-name: "golang.org/x/tools"
        update-types:
          - "version-update:semver-major"
`, 1)
	path := writePolicy(t, policy)

	var out bytes.Buffer
	err := run(path, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), "gomod /tools/openclarion-linter ignore golang.org/x/tools: update-types must be exactly")
}

func TestRunRejectsUnknownYAMLField(t *testing.T) {
	path := writePolicy(t, strings.Replace(validPolicy(), `version: 2`, "version: 2\nunexpected: true", 1))

	var out bytes.Buffer
	err := run(path, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "field unexpected not found") {
		t.Fatalf("run() error = %v, want known-fields failure", err)
	}
}

func TestRunRejectsMultipleYAMLDocuments(t *testing.T) {
	path := writePolicy(t, validPolicy()+"---\nversion: 2\nupdates: []\n")

	var out bytes.Buffer
	err := run(path, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "multiple YAML documents are not allowed") {
		t.Fatalf("run() error = %v, want multiple document rejection", err)
	}
}

func TestRunRejectsNonRegularPolicyInput(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string) string
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				target := filepath.Join(root, "target.yml")
				if err := os.WriteFile(target, []byte(validPolicy()), 0o600); err != nil {
					t.Fatalf("WriteFile(%s): %v", target, err)
				}
				link := filepath.Join(root, "dependabot.yml")
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
				return link
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T, root string) string {
				t.Helper()
				dir := filepath.Join(root, "dependabot.yml")
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatalf("Mkdir(%s): %v", dir, err)
				}
				return dir
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.setup(t, t.TempDir())

			var out bytes.Buffer
			err := run(path, &out)
			if err == nil {
				t.Fatalf("run() error = nil\noutput:\n%s", out.String())
			}
			if !strings.Contains(err.Error(), "must be a regular file") {
				t.Fatalf("run() error = %v, want regular file rejection", err)
			}
		})
	}
}

func writePolicy(t *testing.T, contents string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "dependabot.yml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func validPolicy() string {
	return `version: 2

updates:
  - package-ecosystem: "gomod"
    directory: "/tools/openclarion-linter"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 10
    labels:
      - "dependencies"
      - "go"
    groups:
      openclarion-linter-patch:
        patterns:
          - "*"
        update-types:
          - "patch"
      openclarion-linter-security:
        applies-to: "security-updates"
        patterns:
          - "*"
    ignore:
      - dependency-name: "golang.org/x/tools"
        update-types:
          - "version-update:semver-minor"
          - "version-update:semver-major"

  - package-ecosystem: "npm"
    directory: "/web"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 10
    labels:
      - "dependencies"
      - "frontend"
    groups:
      web-patch:
        patterns:
          - "*"
        update-types:
          - "patch"
      web-security:
        applies-to: "security-updates"
        patterns:
          - "*"
    ignore:
      - dependency-name: "typescript"
        update-types:
          - "version-update:semver-major"
      - dependency-name: "eslint"
        update-types:
          - "version-update:semver-major"
`
}

func assertOutputContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Fatalf("output missing %q:\n%s", want, output)
	}
}
