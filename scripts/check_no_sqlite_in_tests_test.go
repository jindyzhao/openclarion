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

func TestForbiddenSQLiteAllowsNoGoTestsYet(t *testing.T) {
	root := writeForbiddenSQLiteRepo(t, nil)

	out, err := runForbiddenSQLite(t, root)
	if err != nil {
		t.Fatalf("forbidden-sqlite failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no _test.go files yet; skipping") {
		t.Fatalf("forbidden-sqlite output = %q, want no-test skip", out)
	}
}

func TestForbiddenSQLiteAllowsNonSQLiteGoTests(t *testing.T) {
	root := writeForbiddenSQLiteRepo(t, map[string]string{
		"internal/repository/postgres_test.go": `package repository

import "testing"

func TestPostgresHarness(t *testing.T) {}
`,
	})

	out, err := runForbiddenSQLite(t, root)
	if err != nil {
		t.Fatalf("forbidden-sqlite failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[forbidden-sqlite] OK") {
		t.Fatalf("forbidden-sqlite output = %q, want OK", out)
	}
}

func TestForbiddenSQLiteRejectsSQLiteImportsInGoTests(t *testing.T) {
	root := writeForbiddenSQLiteRepo(t, map[string]string{
		"internal/repository/sqlite_driver_test.go": `package repository

import _ "` + sqliteImportFixture("github.com/mattn/", "go-sqlite3") + `"

func TestSQLiteHarness() {}
`,
	})

	out, err := runForbiddenSQLite(t, root)
	if err == nil {
		t.Fatalf("forbidden-sqlite passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"SQLite imports in test files",
		"internal/repository/sqlite_driver_test.go",
		sqliteImportFixture("github.com/mattn/", "go-sqlite3"),
		"ADR-0001 forbids SQLite for tests",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("forbidden-sqlite output = %q, want substring %q", out, want)
		}
	}
}

func TestForbiddenSQLiteRejectsSQLiteDSNsInGoTests(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{
			name: "memory file",
			dsn:  sqliteImportFixture("file::", "memory:"),
		},
		{
			name: "sqlite scheme",
			dsn:  sqliteImportFixture("sqlite", "://fixture.db"),
		},
		{
			name: "sqlite3 scheme",
			dsn:  sqliteImportFixture("sqlite3", "://fixture.db"),
		},
		{
			name: "memory marker",
			dsn:  sqliteImportFixture(":memory", ":"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeForbiddenSQLiteRepo(t, map[string]string{
				"internal/repository/dsn_test.go": `package repository

const dsn = "` + tc.dsn + `"
`,
			})

			out, err := runForbiddenSQLite(t, root)
			if err == nil {
				t.Fatalf("forbidden-sqlite passed unexpectedly:\n%s", out)
			}
			for _, want := range []string{
				"SQLite DSN patterns in test files",
				"internal/repository/dsn_test.go",
				tc.dsn,
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("forbidden-sqlite output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestForbiddenSQLiteIgnoresNonTestGoFiles(t *testing.T) {
	root := writeForbiddenSQLiteRepo(t, map[string]string{
		"internal/repository/postgres_test.go": "package repository\n\nfunc TestPostgresHarness() {}\n",
		"internal/repository/sqlite_driver.go": `package repository

import _ "` + sqliteImportFixture("modernc.org/", "sqlite") + `"
`,
	})

	out, err := runForbiddenSQLite(t, root)
	if err != nil {
		t.Fatalf("forbidden-sqlite failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[forbidden-sqlite] OK") {
		t.Fatalf("forbidden-sqlite output = %q, want OK", out)
	}
}

func TestForbiddenSQLiteRejectsIndirectGoTestPaths(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, root string) {
				t.Helper()
				dir := filepath.Join(root, "internal", "repository")
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
				target := filepath.Join(dir, "target.go")
				if err := os.WriteFile(target, []byte("package repository\n"), 0o600); err != nil {
					t.Fatalf("write symlink target: %v", err)
				}
				if err := os.Symlink("target.go", filepath.Join(dir, "indirect_test.go")); err != nil {
					t.Fatalf("symlink fixture: %v", err)
				}
			},
			want: "internal/repository/indirect_test.go",
		},
		{
			name: "dangling symlink",
			setup: func(t *testing.T, root string) {
				t.Helper()
				dir := filepath.Join(root, "internal", "repository")
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
				if err := os.Symlink("missing.go", filepath.Join(dir, "dangling_test.go")); err != nil {
					t.Fatalf("symlink fixture: %v", err)
				}
			},
			want: "internal/repository/dangling_test.go",
		},
		{
			name: "directory",
			setup: func(t *testing.T, root string) {
				t.Helper()
				path := filepath.Join(root, "internal", "repository", "directory_test.go")
				if err := os.MkdirAll(path, 0o750); err != nil {
					t.Fatalf("mkdir test directory: %v", err)
				}
			},
			want: "internal/repository/directory_test.go",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeForbiddenSQLiteRepo(t, nil)
			tc.setup(t, root)

			out, err := runForbiddenSQLite(t, root)
			if err == nil {
				t.Fatalf("forbidden-sqlite passed unexpectedly:\n%s", out)
			}
			for _, want := range []string{
				"Go test file paths must be regular files",
				tc.want,
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("forbidden-sqlite output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func writeForbiddenSQLiteRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeForbiddenSQLiteFile(t, root, "scripts/check_no_sqlite_in_tests.sh", forbiddenSQLiteScript(t), 0o750)
	for name, body := range files {
		writeForbiddenSQLiteFile(t, root, name, body, 0o644)
	}
	return root
}

func forbiddenSQLiteScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_no_sqlite_in_tests.sh")
	if err != nil {
		t.Fatalf("read forbidden-sqlite script: %v", err)
	}
	return string(raw)
}

func writeForbiddenSQLiteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runForbiddenSQLite(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_no_sqlite_in_tests.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}

func sqliteImportFixture(parts ...string) string {
	return strings.Join(parts, "")
}
