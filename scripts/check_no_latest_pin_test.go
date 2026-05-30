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

func TestForbiddenLatestDockerfilePins(t *testing.T) {
	digest := strings.Repeat("a", 64)
	tests := []struct {
		name   string
		files  map[string]string
		wantOK bool
		want   []string
	}{
		{
			name: "scratch base image is allowed",
			files: map[string]string{
				"scripts/custom/Dockerfile": "FROM scratch\nCOPY app /app\n",
			},
			wantOK: true,
			want:   []string{"[forbidden-latest] OK"},
		},
		{
			name: "digest pinned external image and previous stage are allowed",
			files: map[string]string{
				"Dockerfile": "FROM --platform=linux/amd64 docker.io/library/alpine:3.21@sha256:" + digest + " AS builder\nRUN true\nFROM builder AS final\n",
			},
			wantOK: true,
			want:   []string{"[forbidden-latest] OK"},
		},
		{
			name: "tag only external image is rejected",
			files: map[string]string{
				"Dockerfile": "FROM alpine:3.21 AS builder\n",
			},
			want: []string{
				"external Docker base image must be pinned",
				"alpine:3.21",
			},
		},
		{
			name: "latest external image is rejected",
			files: map[string]string{
				"Dockerfile": "FROM busybox:latest\n",
			},
			want: []string{
				"external Docker base image must be pinned",
				"busybox:latest",
			},
		},
		{
			name: "dynamic external image without literal digest is rejected",
			files: map[string]string{
				"Dockerfile": "ARG BASE_IMAGE=alpine:3.21\nFROM ${BASE_IMAGE}\n",
			},
			want: []string{
				"external Docker base image must be pinned",
				"${BASE_IMAGE}",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newNoLatestRepo(t, tc.files)

			out, err := runNoLatestCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("forbidden-latest failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("forbidden-latest passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("forbidden-latest output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestForbiddenLatestNPMRangeStillFails(t *testing.T) {
	root := newNoLatestRepo(t, map[string]string{
		"package.json": `{"dependencies":{"next":"^16.2.6"}}`,
	})

	out, err := runNoLatestCheck(t, root)
	if err == nil {
		t.Fatalf("forbidden-latest passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "must pin dependencies to exact versions") {
		t.Fatalf("forbidden-latest output = %q, want npm range rejection", out)
	}
}

func TestForbiddenLatestNodeTypesMajorMatchesCINodeMajor(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		wantOK bool
		want   []string
	}{
		{
			name: "node types match setup node major",
			files: map[string]string{
				".github/workflows/ci.yml": "jobs:\n  test:\n    steps:\n      - uses: actions/setup-node@v6\n        with:\n          node-version: \"24\"\n",
				"web/package.json":         `{"devDependencies":{"@types/node":"24.12.4"}}`,
			},
			wantOK: true,
			want:   []string{"[forbidden-latest] OK"},
		},
		{
			name: "multiple setup node jobs on same major are allowed",
			files: map[string]string{
				".github/workflows/ci.yml":       "jobs:\n  test:\n    steps:\n      - uses: actions/setup-node@v6\n        with:\n          node-version: \"24\"\n",
				".github/workflows/frontend.yml": "jobs:\n  lint:\n    steps:\n      - uses: actions/setup-node@v6\n        with:\n          node-version: 24.12.0\n",
				"web/package.json":               `{"devDependencies":{"@types/node":"24.12.4"}}`,
			},
			wantOK: true,
			want:   []string{"[forbidden-latest] OK"},
		},
		{
			name: "multiple setup node majors are rejected",
			files: map[string]string{
				".github/workflows/ci.yml":       "jobs:\n  test:\n    steps:\n      - uses: actions/setup-node@v6\n        with:\n          node-version: \"24\"\n",
				".github/workflows/frontend.yml": "jobs:\n  lint:\n    steps:\n      - uses: actions/setup-node@v6\n        with:\n          node-version: \"25\"\n",
				"web/package.json":               `{"devDependencies":{"@types/node":"24.12.4"}}`,
			},
			want: []string{"requires exactly one numeric actions/setup-node node-version major"},
		},
		{
			name: "node types ahead of setup node major",
			files: map[string]string{
				".github/workflows/ci.yml": "jobs:\n  test:\n    steps:\n      - uses: actions/setup-node@v6\n        with:\n          node-version: \"24\"\n",
				"web/package.json":         `{"devDependencies":{"@types/node":"25.9.1"}}`,
			},
			want: []string{"@types/node major 25 must match CI Node.js major 24"},
		},
		{
			name: "node types require workflow node major",
			files: map[string]string{
				"web/package.json": `{"devDependencies":{"@types/node":"24.12.4"}}`,
			},
			want: []string{"requires exactly one numeric actions/setup-node node-version major"},
		},
		{
			name: "node types require concrete semantic version",
			files: map[string]string{
				".github/workflows/ci.yml": "jobs:\n  test:\n    steps:\n      - uses: actions/setup-node@v6\n        with:\n          node-version: \"24\"\n",
				"web/package.json":         `{"devDependencies":{"@types/node":"24"}}`,
			},
			want: []string{"@types/node must use a concrete semantic version"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newNoLatestRepo(t, tc.files)

			out, err := runNoLatestCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("forbidden-latest failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("forbidden-latest passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("forbidden-latest output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestForbiddenLatestCriticalGoModulePins(t *testing.T) {
	tests := []struct {
		name   string
		goMod  string
		wantOK bool
		want   []string
	}{
		{
			name:   "critical modules are direct concrete pins",
			goMod:  criticalGoMod(""),
			wantOK: true,
			want:   []string{"[forbidden-latest] OK"},
		},
		{
			name: "missing critical module",
			goMod: strings.Replace(
				criticalGoMod(""),
				"\tgo.temporal.io/sdk v1.44.0\n",
				"",
				1,
			),
			want: []string{"must directly require critical first-import module go.temporal.io/sdk"},
		},
		{
			name: "indirect critical module",
			goMod: strings.Replace(
				criticalGoMod(""),
				"\tentgo.io/ent v0.14.6",
				"\tentgo.io/ent v0.14.6 // indirect",
				1,
			),
			want: []string{"entgo.io/ent must be a direct require, not // indirect"},
		},
		{
			name: "non concrete critical version",
			goMod: strings.Replace(
				criticalGoMod(""),
				"\tgo.temporal.io/sdk v1.44.0",
				"\tgo.temporal.io/sdk main",
				1,
			),
			want: []string{"go.temporal.io/sdk must use a concrete semantic or pseudo-version pin"},
		},
		{
			name: "critical module replace",
			goMod: criticalGoMod(`
replace go.temporal.io/sdk => ../sdk
`),
			want: []string{"must not replace critical first-import module go.temporal.io/sdk"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newNoLatestRepo(t, map[string]string{"go.mod": tc.goMod})

			out, err := runNoLatestCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("forbidden-latest failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("forbidden-latest passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("forbidden-latest output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestForbiddenLatestGoToolDirectivePins(t *testing.T) {
	tests := []struct {
		name   string
		goMod  string
		wantOK bool
		want   []string
	}{
		{
			name: "external tool paths are backed by concrete require pins",
			goMod: criticalGoMod(`
tool (
	example.com/test/cmd/local
	example.com/tools/cmd/alpha
	github.com/daveshanley/vacuum
)

require (
	example.com/tools v1.2.3 // indirect
	github.com/daveshanley/vacuum v0.26.6 // indirect
)
`),
			wantOK: true,
			want:   []string{"[forbidden-latest] OK"},
		},
		{
			name: "external tool path without backing require is rejected",
			goMod: criticalGoMod(`
tool example.com/tools/cmd/alpha
`),
			want: []string{"tool directive example.com/tools/cmd/alpha must be backed by a concrete require pin"},
		},
		{
			name: "external tool backing require must use concrete version",
			goMod: criticalGoMod(`
tool example.com/tools/cmd/alpha

require example.com/tools main
`),
			want: []string{
				"tool directive example.com/tools/cmd/alpha resolves to example.com/tools",
				"must use a concrete semantic or pseudo-version pin",
			},
		},
		{
			name: "tool path uses longest matching module prefix",
			goMod: criticalGoMod(`
tool example.com/tools/v2/cmd/alpha

require (
	example.com/tools v1.9.0 // indirect
	example.com/tools/v2 v2.0.1 // indirect
)
`),
			wantOK: true,
			want:   []string{"[forbidden-latest] OK"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newNoLatestRepo(t, map[string]string{"go.mod": tc.goMod})

			out, err := runNoLatestCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("forbidden-latest failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("forbidden-latest passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("forbidden-latest output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestForbiddenLatestGoReplaceAllowlist(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		wantOK bool
		want   []string
	}{
		{
			name: "unallowlisted root replace is rejected",
			files: map[string]string{
				"go.mod": criticalGoMod(`
replace example.com/forked => ../forked
`),
			},
			want: []string{
				"replace directive must be documented",
				"replace-allow: example.com/forked => ../forked",
			},
		},
		{
			name: "allowlisted block replace is accepted",
			files: map[string]string{
				"go.mod": criticalGoMod(`
replace (
	example.com/forked v1.2.3 => example.com/openclarion/forked v1.2.4
)
`),
				"docs/design/DEPENDENCIES.md": "replace-allow: example.com/forked => example.com/openclarion/forked; owner: ci-maintainers; expires: 2099-12-31; reason: temporary upstream fork\n",
			},
			wantOK: true,
			want:   []string{"[forbidden-latest] OK"},
		},
		{
			name: "allowlisted replace without owner expiry is rejected",
			files: map[string]string{
				"go.mod": criticalGoMod(`
replace example.com/forked => example.com/openclarion/forked
`),
				"docs/design/DEPENDENCIES.md": "replace-allow: example.com/forked => example.com/openclarion/forked\n",
			},
			want: []string{
				"must include owner",
				"must include expires",
			},
		},
		{
			name: "expired replace allowlist is rejected",
			files: map[string]string{
				"go.mod": criticalGoMod(`
replace example.com/forked => example.com/openclarion/forked
`),
				"docs/design/DEPENDENCIES.md": "replace-allow: example.com/forked => example.com/openclarion/forked; owner: ci-maintainers; expires: 2000-01-01; reason: old fork\n",
			},
			want: []string{"expired on 2000-01-01"},
		},
		{
			name: "unallowlisted nested module replace is rejected",
			files: map[string]string{
				"go.mod": criticalGoMod(""),
				"tools/custom/go.mod": `module example.com/tool

go 1.25.10

replace example.com/tooldep => ../tooldep
`,
			},
			want: []string{
				"./tools/custom/go.mod",
				"replace-allow: example.com/tooldep => ../tooldep",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newNoLatestRepo(t, tc.files)

			out, err := runNoLatestCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("forbidden-latest failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("forbidden-latest passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("forbidden-latest output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestForbiddenLatestRejectsSymlinkDependencyPolicyForReplaceAllowlist(t *testing.T) {
	root := newNoLatestRepo(t, map[string]string{
		"go.mod": criticalGoMod(`
replace example.com/forked => example.com/openclarion/forked
`),
		"docs/design/DEPENDENCIES.md": "replace-allow: example.com/forked => example.com/openclarion/forked; owner: ci-maintainers; expires: 2099-12-31; reason: temporary upstream fork\n",
	})
	policy := filepath.Join(root, "docs", "design", "DEPENDENCIES.md")
	target := filepath.Join(root, "docs", "design", "DEPENDENCIES-target.md")
	if err := os.Rename(policy, target); err != nil {
		t.Fatalf("rename dependency policy: %v", err)
	}
	if err := os.Symlink(target, policy); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	out, err := runNoLatestCheck(t, root)
	if err == nil {
		t.Fatalf("forbidden-latest passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/DEPENDENCIES.md must be a regular file, not a symlink") {
		t.Fatalf("forbidden-latest output = %q, want symlink policy rejection", out)
	}
}

func TestForbiddenLatestRejectsNonRegularDependencyPolicyForReplaceAllowlist(t *testing.T) {
	root := newNoLatestRepo(t, map[string]string{
		"go.mod": criticalGoMod(`
replace example.com/forked => example.com/openclarion/forked
`),
		"docs/design/DEPENDENCIES.md": "replace-allow: example.com/forked => example.com/openclarion/forked; owner: ci-maintainers; expires: 2099-12-31; reason: temporary upstream fork\n",
	})
	policy := filepath.Join(root, "docs", "design", "DEPENDENCIES.md")
	if err := os.Remove(policy); err != nil {
		t.Fatalf("remove dependency policy: %v", err)
	}
	if err := os.Mkdir(policy, 0o750); err != nil {
		t.Fatalf("mkdir dependency policy path: %v", err)
	}

	out, err := runNoLatestCheck(t, root)
	if err == nil {
		t.Fatalf("forbidden-latest passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/DEPENDENCIES.md must be a regular file") {
		t.Fatalf("forbidden-latest output = %q, want regular-file policy rejection", out)
	}
}

func criticalGoMod(extra string) string {
	return `module example.com/test

go 1.25.10

require (
	entgo.io/ent v0.14.6
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.68.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.temporal.io/sdk v1.44.0
)
` + extra
}

func newNoLatestRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	noLatestWriteFile(t, root, "scripts/check_no_latest_pin.sh", noLatestScript(t), 0o750)
	for name, body := range files {
		noLatestWriteFile(t, root, name, body, 0o644)
	}
	return root
}

func noLatestScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_no_latest_pin.sh")
	if err != nil {
		t.Fatalf("read forbidden-latest script: %v", err)
	}
	return string(raw)
}

func noLatestWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runNoLatestCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_no_latest_pin.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
