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

func TestTestcontainersContractAllowsDatabasePackageWithTestcontainersHarness(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "internal/repository/setup_test.go", `package repository

import (
	"database/sql"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestSetup(t *testing.T) {
	_, _ = sql.Open("pgx", "")
	_ = postgres.BasicWaitStrategies
}
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err != nil {
		t.Fatalf("testcontainers contract failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[testcontainers-contract] OK") {
		t.Fatalf("contract output = %q, want OK", out)
	}
	if !strings.Contains(out, "(1 test files scanned)") {
		t.Fatalf("contract output = %q, want one scanned test file", out)
	}
}

func TestTestcontainersContractAllowsSeparateSetupFileInSamePackage(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "internal/repository/repo_test.go", `package repository

import "database/sql"

func openDB() (*sql.DB, error) {
	return sql.Open("pgx", "")
}
`, 0o644)
	testcontainersContractWriteFile(t, root, "internal/repository/setup_test.go", `package repository

import "github.com/testcontainers/testcontainers-go/modules/postgres"

var _ = postgres.BasicWaitStrategies
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err != nil {
		t.Fatalf("testcontainers contract failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[testcontainers-contract] OK") {
		t.Fatalf("contract output = %q, want OK", out)
	}
	if !strings.Contains(out, "(2 test files scanned)") {
		t.Fatalf("contract output = %q, want two scanned test files", out)
	}
}

func TestTestcontainersContractRejectsDatabasePackageWithoutTestcontainers(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "internal/repository/repo_test.go", `package repository

import "database/sql"

func openDB() (*sql.DB, error) {
	return sql.Open("pgx", "")
}
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err == nil {
		t.Fatalf("testcontainers contract passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"database integration tests must use testcontainers-go",
		"internal/repository/",
		"internal/repository/repo_test.go",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("contract output = %q, want substring %q", out, want)
		}
	}
}

func TestTestcontainersContractIgnoresAnalyzerFixturesUnderTestdata(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "tools/analyzer/testdata/src/pkg/repo_test.go", `package pkg

import "database/sql"

func openDB() (*sql.DB, error) {
	return sql.Open("pgx", "")
}
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err != nil {
		t.Fatalf("testcontainers contract failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[testcontainers-contract] OK") {
		t.Fatalf("contract output = %q, want OK", out)
	}
	if !strings.Contains(out, "(0 test files scanned)") {
		t.Fatalf("contract output = %q, want testdata fixture excluded from scan count", out)
	}
}

func TestTestcontainersContractIgnoresImportTextOutsideGoImportDeclarations(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "internal/repository/repo_test.go", `package repository

const commentedImport = "import \"database/sql\""

// import "database/sql"
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err != nil {
		t.Fatalf("testcontainers contract failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[testcontainers-contract] OK") {
		t.Fatalf("contract output = %q, want OK", out)
	}
	if !strings.Contains(out, "(1 test files scanned)") {
		t.Fatalf("contract output = %q, want one scanned test file", out)
	}
}

func TestTestcontainersContractAllowsHttptestAndInjectedClient(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "internal/provider/http_test.go", `package provider

import (
	"net/http"
	"net/http/httptest"
)

func exercise() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		return err
	}
	resp, err := srv.Client().Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	return err
}
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err != nil {
		t.Fatalf("testcontainers contract failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[testcontainers-contract] OK") {
		t.Fatalf("contract output = %q, want OK", out)
	}
}

func TestTestcontainersContractRejectsDirectHTTPPackageCalls(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "internal/provider/http_test.go", `package provider

import stdhttp "net/http"

func exercise() error {
	resp, err := stdhttp.Get("https://example.com")
	if resp != nil {
		_ = resp.Body.Close()
	}
	return err
}
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err == nil {
		t.Fatalf("testcontainers contract passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"Go tests must not use direct host/public network entry points",
		"internal/provider/http_test.go",
		"uses net/http.Get",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("contract output = %q, want substring %q", out, want)
		}
	}
}

func TestTestcontainersContractRejectsDirectNetDial(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "internal/provider/net_test.go", `package provider

import "net"

func exercise() error {
	conn, err := net.Dial("tcp", "example.com:443")
	if conn != nil {
		_ = conn.Close()
	}
	return err
}
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err == nil {
		t.Fatalf("testcontainers contract passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"internal/provider/net_test.go",
		"uses net.Dial",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("contract output = %q, want substring %q", out, want)
		}
	}
}

func TestTestcontainersContractRejectsHTTPDefaultClient(t *testing.T) {
	root := newTestcontainersContractFixture(t)
	testcontainersContractWriteFile(t, root, "internal/provider/http_test.go", `package provider

import "net/http"

var client = http.DefaultClient
`, 0o644)

	out, err := runTestcontainersContract(t, root)
	if err == nil {
		t.Fatalf("testcontainers contract passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"internal/provider/http_test.go",
		"uses net/http.DefaultClient",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("contract output = %q, want substring %q", out, want)
		}
	}
}

func newTestcontainersContractFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testcontainersContractWriteFile(t, root, "scripts/check_testcontainers_integration.sh", testcontainersContractScript(t), 0o750)
	return root
}

func testcontainersContractScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_testcontainers_integration.sh")
	if err != nil {
		t.Fatalf("read testcontainers contract script: %v", err)
	}
	return string(raw)
}

func testcontainersContractWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runTestcontainersContract(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_testcontainers_integration.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
