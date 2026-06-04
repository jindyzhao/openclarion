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

func TestOSVScanScansCommittedPackageLocks(t *testing.T) {
	root := newOSVScanFixture(t)
	osvWriteFile(t, root, "web/package.json", `{"name":"web"}`+"\n", 0o644)
	osvWriteFile(t, root, "web/package-lock.json", `{"lockfileVersion":3}`+"\n", 0o644)
	osvWriteFile(t, root, "web/node_modules/ignored/package-lock.json", `{"lockfileVersion":3}`+"\n", 0o644)
	callsPath := filepath.Join(root, "calls.txt")

	out, err := runOSVScan(t, root, callsPath, "")
	if err != nil {
		t.Fatalf("osv scan failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[osv-scan] OK (1 lockfiles scanned)") {
		t.Fatalf("output = %q, want one scanned lockfile", out)
	}
	callsRaw, err := os.ReadFile(callsPath) // #nosec G304 -- test reads a temp file it created.
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	want := "run github.com/google/osv-scanner/cmd/osv-scanner@v1.9.2 scan --lockfile=web/package-lock.json --format=json --verbosity=error"
	if !strings.Contains(string(callsRaw), want) {
		t.Fatalf("calls = %q, want %q", string(callsRaw), want)
	}
	if strings.Contains(string(callsRaw), "node_modules") {
		t.Fatalf("calls = %q, node_modules lockfile must be ignored", string(callsRaw))
	}
}

func TestOSVScanRejectsPackageJSONWithoutLockfile(t *testing.T) {
	root := newOSVScanFixture(t)
	osvWriteFile(t, root, "web/package.json", `{"name":"web"}`+"\n", 0o644)

	out, err := runOSVScan(t, root, filepath.Join(root, "calls.txt"), "")
	if err == nil {
		t.Fatalf("osv scan passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "package.json exists but no package-lock.json was found") {
		t.Fatalf("output = %q, want missing lockfile failure", out)
	}
}

func TestOSVScanPropagatesToolFailure(t *testing.T) {
	root := newOSVScanFixture(t)
	osvWriteFile(t, root, "web/package-lock.json", `{"lockfileVersion":3}`+"\n", 0o644)

	out, err := runOSVScan(t, root, filepath.Join(root, "calls.txt"), "1")
	if err == nil {
		t.Fatalf("osv scan passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "simulated osv failure") {
		t.Fatalf("output = %q, want fake tool failure", out)
	}
}

func TestOSVScanRejectsNonRegularNPMInputs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "lockfile symlink",
			setup: func(t *testing.T, root string) {
				osvWriteFile(t, root, "web/package-lock.target.json", `{"lockfileVersion":3}`+"\n", 0o644)
				osvSymlink(t, "package-lock.target.json", filepath.Join(root, "web", "package-lock.json"))
			},
			want: "lockfile must be a regular file, not a symlink",
		},
		{
			name: "lockfile directory",
			setup: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, "web", "package-lock.json"), 0o750); err != nil {
					t.Fatalf("mkdir lockfile replacement: %v", err)
				}
			},
			want: "lockfile not found or not a regular file",
		},
		{
			name: "package manifest symlink",
			setup: func(t *testing.T, root string) {
				osvWriteFile(t, root, "web/package-lock.json", `{"lockfileVersion":3}`+"\n", 0o644)
				osvWriteFile(t, root, "web/package.target.json", `{"name":"web"}`+"\n", 0o644)
				osvSymlink(t, "package.target.json", filepath.Join(root, "web", "package.json"))
			},
			want: "package manifest must be a regular file, not a symlink",
		},
		{
			name: "package manifest directory",
			setup: func(t *testing.T, root string) {
				osvWriteFile(t, root, "web/package-lock.json", `{"lockfileVersion":3}`+"\n", 0o644)
				if err := os.MkdirAll(filepath.Join(root, "web", "package.json"), 0o750); err != nil {
					t.Fatalf("mkdir package manifest replacement: %v", err)
				}
			},
			want: "package manifest not found or not a regular file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newOSVScanFixture(t)
			tc.setup(t, root)

			out, err := runOSVScan(t, root, filepath.Join(root, "calls.txt"), "")
			if err == nil {
				t.Fatalf("osv scan passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("osv output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func newOSVScanFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	osvWriteFile(t, root, "scripts/run_osv_scan.sh", osvScanScript(t), 0o750)
	osvWriteFile(t, root, "bin/go", fakeOSVGo(), 0o750)
	return root
}

func osvScanScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("run_osv_scan.sh")
	if err != nil {
		t.Fatalf("read osv scan script: %v", err)
	}
	return string(raw)
}

func fakeOSVGo() string {
	return `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${OSV_CALLS:?}"
if [[ "${OSV_FAKE_FAIL:-}" == "1" ]]; then
  echo "simulated osv failure" >&2
  exit 43
fi
`
}

func osvWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func osvSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runOSVScan(t *testing.T, root, callsPath, fakeFail string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/run_osv_scan.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OSV_CALLS="+callsPath,
		"OSV_FAKE_FAIL="+fakeFail,
	)
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
