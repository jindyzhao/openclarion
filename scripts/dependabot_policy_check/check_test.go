package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependabotPolicyAcceptsExpectedPolicy(t *testing.T) {
	root := t.TempDir()
	writeDependabotPolicy(t, root, validDependabotPolicy())

	var stdout bytes.Buffer
	if err := run(config{Path: filepath.Join(root, ".github", "dependabot.yml")}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[dependabot-policy] OK (4 update rules checked)") {
		t.Fatalf("stdout = %q, want OK", stdout.String())
	}
}

func TestDependabotPolicyRejectsInvalidPolicies(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		wantErr string
	}{
		{
			name:    "missing web update",
			policy:  strings.Replace(validDependabotPolicy(), webUpdateBlock(), "", 1),
			wantErr: "updates count = 3, want 4",
		},
		{
			name:    "non weekly schedule",
			policy:  strings.Replace(validDependabotPolicy(), `interval: "weekly"`, `interval: "daily"`, 1),
			wantErr: `schedule.interval = "daily", want weekly`,
		},
		{
			name:    "wrong schedule time",
			policy:  strings.Replace(validDependabotPolicy(), `time: "09:00"`, `time: "09:15"`, 1),
			wantErr: `schedule.time = "09:15", want "09:00"`,
		},
		{
			name: "missing labels",
			policy: strings.Replace(validDependabotPolicy(), `    labels:
      - "dependencies"
      - "github-actions"
`, "", 1),
			wantErr: "labels = [], want [dependencies,github-actions]",
		},
		{
			name: "blanket ignore outside linter",
			policy: strings.Replace(validDependabotPolicy(), `    groups:
      go-patch:`, `    ignore:
      - dependency-name: "*"
    groups:
      go-patch:`, 1),
			wantErr: "ignore entries are forbidden outside the linter tooling exception",
		},
		{
			name:    "security group restricted",
			policy:  strings.Replace(validDependabotPolicy(), `applies-to: "security-updates"`, "update-types:\n          - \"patch\"\n        applies-to: \"security-updates\"", 1),
			wantErr: `security group "github-actions-security" must not restrict update-types`,
		},
		{
			name:    "patch group permits minor",
			policy:  strings.Replace(validDependabotPolicy(), `          - "patch"`, "          - \"patch\"\n          - \"minor\"", 1),
			wantErr: `patch group "github-actions-patch" update-types = [patch,minor], want [patch]`,
		},
		{
			name:    "linter ignore includes patch updates",
			policy:  strings.Replace(validDependabotPolicy(), `          - "version-update:semver-major"`, "          - \"version-update:semver-major\"\n          - \"version-update:semver-patch\"", 1),
			wantErr: "linter tooling ignore update-types",
		},
		{
			name:    "duplicate yaml key",
			policy:  strings.Replace(validDependabotPolicy(), "version: 2\n", "version: 2\nversion: 2\n", 1),
			wantErr: "duplicate YAML key",
		},
		{
			name:    "multiple yaml documents",
			policy:  validDependabotPolicy() + "---\nversion: 2\nupdates: []\n",
			wantErr: "dependabot policy must contain exactly one YAML document",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeDependabotPolicy(t, root, tc.policy)

			var stdout bytes.Buffer
			err := run(config{Path: filepath.Join(root, ".github", "dependabot.yml")}, &stdout)
			if err == nil {
				t.Fatalf("run passed unexpectedly:\n%s", stdout.String())
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("run error = %q, want substring %q", err.Error(), tc.wantErr)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty on failure", stdout.String())
			}
		})
	}
}

func TestDependabotPolicyRejectsNonRegularPolicyFile(t *testing.T) {
	root := t.TempDir()
	writeDependabotPolicy(t, root, validDependabotPolicy())
	if err := os.Rename(
		filepath.Join(root, ".github", "dependabot.yml"),
		filepath.Join(root, ".github", "dependabot-real.yml"),
	); err != nil {
		t.Fatalf("rename dependabot policy: %v", err)
	}
	if err := os.Symlink("dependabot-real.yml", filepath.Join(root, ".github", "dependabot.yml")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	var stdout bytes.Buffer
	err := run(config{Path: filepath.Join(root, ".github", "dependabot.yml")}, &stdout)
	if err == nil {
		t.Fatalf("run passed unexpectedly:\n%s", stdout.String())
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run error = %q, want symlink rejection", err.Error())
	}
}

func writeDependabotPolicy(t *testing.T, root, policy string) {
	t.Helper()
	path := filepath.Join(root, ".github", "dependabot.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(policy), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func validDependabotPolicy() string {
	return `version: 2

updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
      day: "monday"
      time: "09:00"
      timezone: "Asia/Hong_Kong"
    open-pull-requests-limit: 10
    labels:
      - "dependencies"
      - "github-actions"
    groups:
      github-actions-patch:
        patterns:
          - "*"
        update-types:
          - "patch"
      github-actions-security:
        applies-to: "security-updates"
        patterns:
          - "*"

  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
      day: "monday"
      time: "09:30"
      timezone: "Asia/Hong_Kong"
    open-pull-requests-limit: 10
    labels:
      - "dependencies"
      - "go"
    groups:
      go-patch:
        patterns:
          - "*"
        update-types:
          - "patch"
      go-security:
        applies-to: "security-updates"
        patterns:
          - "*"

  - package-ecosystem: "gomod"
    directory: "/tools/openclarion-linter"
    schedule:
      interval: "weekly"
      day: "monday"
      time: "09:45"
      timezone: "Asia/Hong_Kong"
    open-pull-requests-limit: 10
    labels:
      - "dependencies"
      - "go"
      - "tooling"
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
      day: "monday"
      time: "10:00"
      timezone: "Asia/Hong_Kong"
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
`
}

func webUpdateBlock() string {
	return `  - package-ecosystem: "npm"
    directory: "/web"
    schedule:
      interval: "weekly"
      day: "monday"
      time: "10:00"
      timezone: "Asia/Hong_Kong"
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
`
}
