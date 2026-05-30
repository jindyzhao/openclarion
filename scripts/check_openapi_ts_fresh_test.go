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

func TestOpenAPITSFreshScript(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, root string)
		wantOK bool
		want   []string
	}{
		{
			name:   "no web tree skips",
			setup:  func(_ *testing.T, _ string) {},
			wantOK: true,
			want:   []string{"no web/ tree; skipping"},
		},
		{
			name: "up to date generated file passes",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "already-generated\n", "keep")
			},
			wantOK: true,
			want:   []string{"generated TypeScript types are up-to-date"},
		},
		{
			name: "stale generated file is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "before\n", "write-after")
			},
			want: []string{
				"FAIL: web/src/lib/api/openapi.ts is stale",
				"after",
			},
		},
		{
			name: "missing package json is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "already-generated\n", "keep")
				if err := os.Remove(filepath.Join(root, "web", "package.json")); err != nil {
					t.Fatalf("remove package.json: %v", err)
				}
			},
			want: []string{"web/package.json must be a regular file"},
		},
		{
			name: "symlinked web tree is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPIYAML(t, root)
				if err := os.Mkdir(filepath.Join(root, "web-target"), 0o750); err != nil {
					t.Fatalf("mkdir web target: %v", err)
				}
				createOpenAPITSFreshSymlink(t, "web-target", filepath.Join(root, "web"))
			},
			want: []string{"web/ must be a real directory"},
		},
		{
			name: "regular file web tree is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFile(t, root, "web", "not a directory\n", 0o644)
			},
			want: []string{"web/ must be a real directory"},
		},
		{
			name: "dangling web tree symlink is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				createOpenAPITSFreshSymlink(t, "missing-web", filepath.Join(root, "web"))
			},
			want: []string{"web/ must be a real directory"},
		},
		{
			name: "symlinked openapi source is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "already-generated\n", "keep")
				if err := os.Remove(filepath.Join(root, "api", "openapi.yaml")); err != nil {
					t.Fatalf("remove openapi.yaml: %v", err)
				}
				writeOpenAPITSFreshFile(t, root, "api/openapi.target.yaml", "openapi: 3.1.0\n", 0o644)
				createOpenAPITSFreshSymlink(t, "openapi.target.yaml", filepath.Join(root, "api", "openapi.yaml"))
			},
			want: []string{"api/openapi.yaml must be a regular file"},
		},
		{
			name: "dangling openapi source symlink is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "already-generated\n", "keep")
				if err := os.Remove(filepath.Join(root, "api", "openapi.yaml")); err != nil {
					t.Fatalf("remove openapi.yaml: %v", err)
				}
				createOpenAPITSFreshSymlink(t, "missing-openapi.yaml", filepath.Join(root, "api", "openapi.yaml"))
			},
			want: []string{"api/openapi.yaml must be a regular file"},
		},
		{
			name: "symlinked package json is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "already-generated\n", "keep")
				if err := os.Remove(filepath.Join(root, "web", "package.json")); err != nil {
					t.Fatalf("remove package.json: %v", err)
				}
				writeOpenAPITSFreshFile(t, root, "web/package.target.json", packageJSON("keep"), 0o644)
				createOpenAPITSFreshSymlink(t, "package.target.json", filepath.Join(root, "web", "package.json"))
			},
			want: []string{"web/package.json must be a regular file"},
		},
		{
			name: "dangling package json symlink is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "already-generated\n", "keep")
				if err := os.Remove(filepath.Join(root, "web", "package.json")); err != nil {
					t.Fatalf("remove package.json: %v", err)
				}
				createOpenAPITSFreshSymlink(t, "missing-package.json", filepath.Join(root, "web", "package.json"))
			},
			want: []string{"web/package.json must be a regular file"},
		},
		{
			name: "symlinked generated file is rejected before generation",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "already-generated\n", "keep")
				generated := filepath.Join(root, "web", "src", "lib", "api", "openapi.ts")
				if err := os.Remove(generated); err != nil {
					t.Fatalf("remove generated file: %v", err)
				}
				writeOpenAPITSFreshFile(t, root, "web/src/lib/api/openapi.target.ts", "already-generated\n", 0o644)
				createOpenAPITSFreshSymlink(t, "openapi.target.ts", generated)
			},
			want: []string{"web/src/lib/api/openapi.ts must be a regular file"},
		},
		{
			name: "dangling generated file symlink is rejected before generation",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "already-generated\n", "keep")
				generated := filepath.Join(root, "web", "src", "lib", "api", "openapi.ts")
				if err := os.Remove(generated); err != nil {
					t.Fatalf("remove generated file: %v", err)
				}
				createOpenAPITSFreshSymlink(t, "missing-openapi.ts", generated)
			},
			want: []string{"web/src/lib/api/openapi.ts must be a regular file"},
		},
		{
			name: "generator must not replace output with symlink",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeOpenAPITSFreshFixture(t, root, "before\n", "write-symlink")
			},
			want: []string{"web/src/lib/api/openapi.ts must remain a regular file after generation"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newOpenAPITSFreshRepo(t)
			tc.setup(t, root)

			out, err := runOpenAPITSFresh(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("openapi-ts-fresh failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("openapi-ts-fresh passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("openapi-ts-fresh output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func newOpenAPITSFreshRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeOpenAPITSFreshFile(t, root, "scripts/check_openapi_ts_fresh.sh", openAPITSFreshScript(t), 0o750)
	return root
}

func writeOpenAPITSFreshFixture(t *testing.T, root, generated, mode string) {
	t.Helper()
	writeOpenAPIYAML(t, root)
	writeOpenAPITSFreshFile(t, root, "web/package.json", packageJSON(mode), 0o644)
	writeOpenAPITSFreshFile(t, root, "web/src/lib/api/openapi.ts", generated, 0o644)
}

func writeOpenAPIYAML(t *testing.T, root string) {
	t.Helper()
	writeOpenAPITSFreshFile(t, root, "api/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Fixture\n  version: 0.0.0\npaths: {}\n", 0o644)
}

func packageJSON(mode string) string {
	var script string
	switch mode {
	case "keep":
		script = `node -e ""`
	case "write-after":
		script = `node -e "require('fs').writeFileSync('src/lib/api/openapi.ts','after\\n')"`
	case "write-symlink":
		script = `node -e "const fs=require('fs');fs.rmSync('src/lib/api/openapi.ts',{force:true});fs.writeFileSync('src/lib/api/target.ts','after\\n');fs.symlinkSync('target.ts','src/lib/api/openapi.ts')"`
	default:
		panic("unknown package fixture mode: " + mode)
	}
	return `{"scripts":{"api:generate":` + strconv.Quote(script) + `}}`
}

func openAPITSFreshScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_openapi_ts_fresh.sh")
	if err != nil {
		t.Fatalf("read openapi-ts-fresh script: %v", err)
	}
	return string(raw)
}

func writeOpenAPITSFreshFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func createOpenAPITSFreshSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runOpenAPITSFresh(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_openapi_ts_fresh.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
