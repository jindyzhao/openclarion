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

func TestStage5LocalWorkerCheckOnlyRejectsUnsafeRuntimeNetwork(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{name: "external", state: "openclarion-sandbox-allowlist|false|false|false"},
		{name: "ingress", state: "openclarion-sandbox-allowlist|true|true|false"},
		{name: "config only", state: "openclarion-sandbox-allowlist|true|false|true"},
		{name: "different name", state: "other-network|true|false|false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newStage5LocalWorkerFixture(t)
			privateDir := t.TempDir()
			envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
			binDir := writeStage5LocalWorkerFakeDockerScript(t, `#!/usr/bin/env bash
if [[ "$1" == "network" && "$2" == "inspect" ]]; then
  printf '%s\n' '`+tt.state+`'
  exit 0
fi
exit 2
`)

			out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
			if err == nil {
				t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, "must name the exact internal, non-ingress, non-config-only Docker network") {
				t.Fatalf("stage5-local-worker output = %q, want unsafe network error", out)
			}
			assertStage5LocalWorkerNoSecretLeak(t, out)
		})
	}
}

func TestStage5LocalWorkerCheckOnlyRejectsFailedSandboxReadinessProbe(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED": "other.example.invalid:443",
	})
	binDir := writeStage5LocalWorkerFakeDockerScript(t, `#!/usr/bin/env bash
if [[ "$1" == "network" && "$2" == "inspect" ]]; then
  printf '%s\n' 'openclarion-sandbox-allowlist|true|false|false'
  exit 0
fi
if [[ "$1" == "image" && "$2" == "inspect" ]]; then
  exit 0
fi
if [[ "$1" == "run" ]]; then
  exit 1
fi
exit 2
`)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "diagnosis sandbox readiness probe failed") {
		t.Fatalf("stage5-local-worker output = %q, want readiness probe error", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRequiresEgressProxyURL(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_SANDBOX_EGRESS_PROXY_URL": "",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_SANDBOX_EGRESS_PROXY_URL") {
		t.Fatalf("stage5-local-worker output = %q, want missing proxy URL", out)
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

func TestStage5LocalWorkerCheckOnlySupportsStaticDiagnosisAuth(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":           "static",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL":     "",
		"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN": "stage5-static-token-fixture",
		"OPENCLARION_DIAGNOSIS_STATIC_SUBJECT":      "operator-1",
		"OPENCLARION_DIAGNOSIS_STATIC_ROLES":        "admin",
	})
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

func TestStage5LocalWorkerCheckOnlySupportsStandardOIDCDiagnosisAuth(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":       "",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL": "",
		"OIDC_ISSUER":                           "https://iam.example.invalid",
		"OIDC_CLIENT_ID":                        "openclarion-web",
	})
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

func TestStage5LocalWorkerCheckOnlyRejectsLegacyWeComDiagnosisAuth(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE": "wecom",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_DIAGNOSIS_AUTH_MODE must be ldap, oidc, or static") {
		t.Fatalf("stage5-local-worker output = %q, want legacy WeCom auth mode rejection", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyIgnoresLDAPOptionalEnvWhenDetectingMode(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":                     "",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL":               "",
		"OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER":              "(uid={username})",
		"OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE":        "mail",
		"OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE":           "memberOf",
		"OPENCLARION_DIAGNOSIS_LDAP_START_TLS":                "sometimes",
		"OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT": "sometimes",
		"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN":           "stage5-static-token-fixture",
		"OPENCLARION_DIAGNOSIS_STATIC_SUBJECT":                "operator-1",
		"OPENCLARION_DIAGNOSIS_STATIC_ROLES":                  "admin",
	})
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

func TestStage5LocalWorkerCheckOnlyDefaultsToLDAPWhenNoAuthConfigured(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":       "",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL": "",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_DIAGNOSIS_LDAP_URL") ||
		!strings.Contains(out, "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN") {
		t.Fatalf("stage5-local-worker output = %q, want LDAP default prerequisites", out)
	}
	if strings.Contains(out, "OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL") {
		t.Fatalf("stage5-local-worker output = %q, still defaulted to OIDC", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlySupportsLDAPDiagnosisAuth(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":       "ldap",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL": "",
		"OPENCLARION_DIAGNOSIS_LDAP_URL":        "ldaps://ldap.example.invalid:636",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN":    "dc=example,dc=invalid",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_DN":    "cn=openclarion,dc=example,dc=invalid",
		// #nosec G101 -- test-only LDAP fixture value, not a production credential.
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD":  "stage5-ldap-password-fixture",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES":  "owner",
		"OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER":    "(uid={username})",
		"OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE": "memberOf",
	})
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

func TestStage5LocalWorkerCheckOnlyRunsDiagnosisAuthEnvCheckForCurrentCheckout(t *testing.T) {
	root := openclarionRepoRoot(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":          "ldap",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL":    "",
		"OPENCLARION_DIAGNOSIS_LDAP_URL":           "ldaps://ldap.example.invalid:636",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN":       "dc=example,dc=invalid",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES": "owner",
		"OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER":   "(uid=*)",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorkerAtRoot(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "diagnosis auth provider environment validation failed") ||
		!strings.Contains(out, "user filter") {
		t.Fatalf("stage5-local-worker output = %q, want provider env validation failure", out)
	}
	for _, leaked := range []string{
		"ldap.example.invalid",
		"dc=example,dc=invalid",
	} {
		if strings.Contains(out, leaked) {
			t.Fatalf("stage5-local-worker leaked %q in output: %q", leaked, out)
		}
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyKeepsWeComCallbackSeparateFromDiagnosisAuth(t *testing.T) {
	root := openclarionRepoRoot(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":             "static",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL":       "",
		"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN":   "stage5-static-token-fixture",
		"OPENCLARION_DIAGNOSIS_STATIC_SUBJECT":        "operator-1",
		"OPENCLARION_DIAGNOSIS_STATIC_ROLES":          "admin",
		"OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY":   "unit-test-state-signing-key-32-bytes",
		"OPENCLARION_WECOM_CORP_ID":                   "ww-openclarion",
		"OPENCLARION_WECOM_CALLBACK_TOKEN":            "callback-token-1",
		"OPENCLARION_WECOM_CALLBACK_ENCODING_AES_KEY": "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG",
		"OPENCLARION_WECOM_CALLBACK_RECEIVE_ID":       "ww-openclarion",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorkerAtRoot(t, root, envFile, binDir, "--check-only")
	if err != nil {
		t.Fatalf("stage5-local-worker failed: %v\n%s", err, out)
	}
	for _, leaked := range []string{
		"ww-openclarion",
		"callback-token-1",
		"0123456789abcdefghijklmnopqrstuvwxyzABCDEFG",
	} {
		if strings.Contains(out, leaked) {
			t.Fatalf("stage5-local-worker leaked %q in output: %q", leaked, out)
		}
	}
	if !strings.Contains(out, "runtime prerequisites are ready") {
		t.Fatalf("stage5-local-worker output = %q, want runtime readiness success", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlySupportsLDAPStartTLS(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":          "ldap",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL":    "",
		"OPENCLARION_DIAGNOSIS_LDAP_URL":           "ldap://ldap.example.invalid:389",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN":       "dc=example,dc=invalid",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES": "owner",
		"OPENCLARION_DIAGNOSIS_LDAP_START_TLS":     "true",
	})
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

func TestStage5LocalWorkerCheckOnlyRejectsPlaintextLDAPByDefault(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":          "ldap",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL":    "",
		"OPENCLARION_DIAGNOSIS_LDAP_URL":           "ldap://ldap.example.invalid:389",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN":       "dc=example,dc=invalid",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES": "owner",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_DIAGNOSIS_LDAP_START_TLS") {
		t.Fatalf("stage5-local-worker output = %q, want LDAP transport error", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRejectsIncompleteStaticDiagnosisAuth(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":           "static",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL":     "",
		"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN": "",
		"OPENCLARION_DIAGNOSIS_STATIC_SUBJECT":      "operator-1",
		"OPENCLARION_DIAGNOSIS_STATIC_ROLES":        "admin",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN") {
		t.Fatalf("stage5-local-worker output = %q, want missing static bearer token", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRejectsIncompleteLDAPDiagnosisAuth(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_DIAGNOSIS_AUTH_MODE":       "ldap",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL": "",
		"OPENCLARION_DIAGNOSIS_LDAP_URL":        "ldaps://ldap.example.invalid:636",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN") {
		t.Fatalf("stage5-local-worker output = %q, want missing LDAP base DN", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRejectsMissingDiagnosisInstructions(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
	if err := os.Remove(stage5LocalWorkerInstructionsPath(privateDir)); err != nil {
		t.Fatalf("remove diagnosis instructions: %v", err)
	}
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "diagnosis-assistant instructions.md must be a direct regular file") {
		t.Fatalf("stage5-local-worker output = %q, want missing instructions error", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRejectsSandboxUnreadableDiagnosisInstructions(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
	if err := os.Chmod(stage5LocalWorkerInstructionsPath(privateDir), 0o600); err != nil {
		t.Fatalf("chmod diagnosis instructions: %v", err)
	}
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "diagnosis-assistant instructions.md must be accessible by the sandbox user") {
		t.Fatalf("stage5-local-worker output = %q, want sandbox-readable instructions error", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRejectsOversizedDiagnosisInstructions(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
	// #nosec G306 -- this fixture must be sandbox-readable to reach the size validation branch.
	if err := os.WriteFile(stage5LocalWorkerInstructionsPath(privateDir), []byte(strings.Repeat("x", 65*1024)), 0o644); err != nil {
		t.Fatalf("write oversized diagnosis instructions: %v", err)
	}
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "diagnosis-assistant instructions.md must be 64 KiB or smaller") {
		t.Fatalf("stage5-local-worker output = %q, want oversized instructions error", out)
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

func TestStage5LocalWorkerSourceModeIgnoresBinaryOverride(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_STAGE5_WORKER_BINARY": filepath.Join(privateDir, "missing-openclarion"),
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--check-only", "--source")
	if err != nil {
		t.Fatalf("stage5-local-worker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "runtime prerequisites are ready") {
		t.Fatalf("stage5-local-worker output = %q, want runtime readiness success", out)
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
echo "diagnosis-timeout=${OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS:-}"
echo "diagnosis-output-mode=${OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE:-}"
`, 0o700)
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_STAGE5_WORKER_BINARY":               workerBin,
		"OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS": "260",
		"OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE":          "json_schema",
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorker(t, root, envFile, binDir)
	if err != nil {
		t.Fatalf("stage5-local-worker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "fake-openclarion-serve") {
		t.Fatalf("stage5-local-worker output = %q, want binary execution", out)
	}
	if !strings.Contains(out, "diagnosis-timeout=260") ||
		!strings.Contains(out, "diagnosis-output-mode=json_schema") {
		t.Fatalf("stage5-local-worker output = %q, want optional diagnosis runner env exported", out)
	}
	if !strings.Contains(out, "starting OpenClarion from configured binary") ||
		!strings.Contains(out, "pass --source to run current checkout") {
		t.Fatalf("stage5-local-worker output = %q, want configured binary startup notice", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerSourceModeExecsGoRun(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	workerBin := filepath.Join(privateDir, "openclarion")
	writeStage5LocalWorkerFile(t, privateDir, "openclarion", `#!/usr/bin/env bash
echo "unexpected binary execution" >&2
exit 7
`, 0o700)
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_STAGE5_WORKER_BINARY": workerBin,
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)
	writeStage5LocalWorkerFile(t, binDir, "go", `#!/usr/bin/env bash
if [[ "$1" != "run" || "$2" != "./cmd/openclarion" || "$3" != "serve" ]]; then
  echo "unexpected go args: $*" >&2
  exit 8
fi
echo "fake-go-run-openclarion-serve"
`, 0o755)

	out, err := runStage5LocalWorker(t, root, envFile, binDir, "--source")
	if err != nil {
		t.Fatalf("stage5-local-worker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "fake-go-run-openclarion-serve") {
		t.Fatalf("stage5-local-worker output = %q, want go run execution", out)
	}
	if !strings.Contains(out, "starting OpenClarion from current checkout") {
		t.Fatalf("stage5-local-worker output = %q, want current checkout startup notice", out)
	}
	if strings.Contains(out, "pass --source to run current checkout") {
		t.Fatalf("stage5-local-worker printed binary-mode notice in source mode: %q", out)
	}
	if strings.Contains(out, "unexpected binary execution") {
		t.Fatalf("stage5-local-worker used binary despite source mode: %q", out)
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyPullsMissingSandboxImage(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{})
	binDir := writeStage5LocalWorkerFakeDockerScript(t, `#!/usr/bin/env bash
if [[ "$1" == "network" && "$2" == "inspect" && "$3" == "openclarion-sandbox-allowlist" ]]; then
	printf '%s\n' 'openclarion-sandbox-allowlist|true|false|false'
	exit 0
fi
if [[ "$1" == "image" && "$2" == "inspect" ]]; then
  exit 1
fi
if [[ "$1" == "pull" ]]; then
	exit 0
fi
if [[ "$1" == "run" ]]; then
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

func TestStage5LocalWorkerCheckOnlySupportsProfileOnlyNotificationConfig(t *testing.T) {
	root := newStage5LocalWorkerFixture(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_IM_WEBHOOK_URL":                        "",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON": `{"secret/openclarion/report-webhook":"https://hooks.example.invalid/openclarion"}`,
	})
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

func TestStage5LocalWorkerCheckOnlyRunsNotificationSecretRefsEnvCheckForCurrentCheckout(t *testing.T) {
	root := openclarionRepoRoot(t)
	privateDir := t.TempDir()
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_IM_WEBHOOK_URL":                        "",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON": `{"secret/openclarion/ops-wecom":"https://hooks.example.invalid/openclarion?key=secret-value"}`,
	})
	binDir := writeStage5LocalWorkerFakeDocker(t, 0)

	out, err := runStage5LocalWorkerAtRoot(t, root, envFile, binDir, "--check-only")
	if err == nil {
		t.Fatalf("stage5-local-worker passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "notification channel secret reference environment validation failed") {
		t.Fatalf("stage5-local-worker output = %q, want notification secret refs validation failure", out)
	}
	for _, leaked := range []string{
		"hooks.example.invalid",
		"secret-value",
		"secret/openclarion/ops-wecom",
	} {
		if strings.Contains(out, leaked) {
			t.Fatalf("stage5-local-worker leaked %q in output: %q", leaked, out)
		}
	}
	assertStage5LocalWorkerNoSecretLeak(t, out)
}

func TestStage5LocalWorkerCheckOnlyRequiresNotificationDeliveryConfig(t *testing.T) {
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
	if !strings.Contains(out, "OPENCLARION_IM_WEBHOOK_URL or OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON") {
		t.Fatalf("stage5-local-worker output = %q, want notification delivery requirement", out)
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
	weComWebhookURL := "https://" + "qyapi.weixin.qq.com" + "/cgi-bin/webhook/send"
	envFile := writeStage5LocalWorkerEnv(t, privateDir, map[string]string{
		"OPENCLARION_IM_WEBHOOK_URL":    weComWebhookURL,
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
	diagnosisAgentDir := filepath.Join(agentDir, "diagnosis-assistant")
	// #nosec G301 -- the stage5 sandbox mounts agent instructions read-only as a non-owner user.
	if err := os.MkdirAll(diagnosisAgentDir, 0o755); err != nil {
		t.Fatalf("mkdir diagnosis agent config: %v", err)
	}
	// #nosec G302 -- the stage5 sandbox needs execute permission on this fixture directory.
	if err := os.Chmod(diagnosisAgentDir, 0o755); err != nil {
		t.Fatalf("chmod diagnosis agent config: %v", err)
	}
	// #nosec G306 -- the stage5 sandbox needs read permission on this fixture file.
	if err := os.WriteFile(filepath.Join(diagnosisAgentDir, "instructions.md"), []byte("Return diagnosis_turn.v1 JSON.\n"), 0o644); err != nil {
		t.Fatalf("write diagnosis instructions: %v", err)
	}
	// #nosec G302 -- the stage5 sandbox needs read permission on this fixture file.
	if err := os.Chmod(filepath.Join(diagnosisAgentDir, "instructions.md"), 0o644); err != nil {
		t.Fatalf("chmod diagnosis instructions: %v", err)
	}
	values := map[string]string{
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL": "https://issuer.example.invalid",
		"OPENCLARION_SANDBOX_IMAGE_REF":         "registry.example/openclarion/diagnosis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT": agentDir,
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED":    "llm.example.invalid:443",
		"OPENCLARION_SANDBOX_EGRESS_PROXY_URL":  "http://openclarion-egress-proxy:18080",
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
		"OPENCLARION_DIAGNOSIS_AUTH_MODE",
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL",
		"OIDC_ISSUER",
		"OIDC_CLIENT_ID",
		"OPENCLARION_DIAGNOSIS_LDAP_URL",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_DN",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD",
		"OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER",
		"OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE",
		"OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE",
		"OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES",
		"OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES",
		"OPENCLARION_DIAGNOSIS_LDAP_START_TLS",
		"OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT",
		"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN",
		"OPENCLARION_DIAGNOSIS_STATIC_SUBJECT",
		"OPENCLARION_DIAGNOSIS_STATIC_ROLES",
		"OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY",
		"OPENCLARION_WECOM_CORP_ID",
		"OPENCLARION_WECOM_CALLBACK_TOKEN",
		"OPENCLARION_WECOM_CALLBACK_ENCODING_AES_KEY",
		"OPENCLARION_WECOM_CALLBACK_RECEIVE_ID",
		"OPENCLARION_SANDBOX_IMAGE_REF",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT",
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED",
		"OPENCLARION_SANDBOX_EGRESS_PROXY_URL",
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL",
		"OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS",
		"OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE",
		"OPENCLARION_IM_WEBHOOK_URL",
		"OPENCLARION_IM_WEBHOOK_FORMAT",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
		"OPENCLARION_NOTIFICATION_CHANNEL_WECOM_SECRET_REFS",
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

func stage5LocalWorkerInstructionsPath(privateDir string) string {
	return filepath.Join(privateDir, "agent-config", "diagnosis-assistant", "instructions.md")
}

func writeStage5LocalWorkerFakeDocker(t *testing.T, exitCode int) string {
	t.Helper()
	body := `#!/usr/bin/env bash
if [[ "$1" == "network" && "$2" == "inspect" && "$3" == "openclarion-sandbox-allowlist" ]]; then
	if (( ` + strconv.Itoa(exitCode) + ` != 0 )); then
		exit ` + strconv.Itoa(exitCode) + `
	fi
	printf '%s\n' 'openclarion-sandbox-allowlist|true|false|false'
	exit 0
fi
if [[ "$1" == "image" && "$2" == "inspect" ]]; then
	exit 0
fi
if [[ "$1" == "pull" ]]; then
	exit 0
fi
if [[ "$1" == "run" ]]; then
	args=" $* "
	for required in \
		' --network openclarion-sandbox-allowlist ' \
		' -e OPENCLARION_DIAGNOSIS_LLM_BASE_URL ' \
		' -e OPENCLARION_SANDBOX_EGRESS_ALLOWED ' \
		' -e OPENCLARION_SANDBOX_EGRESS_PROXY_URL ' \
		' --entrypoint /diagnosis-assistant-runner '
	do
		if [[ "$args" != *"$required"* ]]; then
			echo "missing readiness argument" >&2
			exit 9
		fi
	done
	if [[ "${!#}" != "readiness" ]]; then
		exit 9
	fi
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
	return runStage5LocalWorkerAtRoot(t, root, envFile, binDir, args...)
}

func runStage5LocalWorkerAtRoot(t *testing.T, root, envFile, binDir string, args ...string) (string, error) {
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

func openclarionRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if filepath.Base(wd) == "scripts" {
		return filepath.Dir(wd)
	}
	return wd
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
		"stage5-static-token-fixture",
		"stage5-ldap-password-fixture",
		"https://llm.example.invalid/v1",
		"https://hooks.example.invalid/openclarion",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("stage5-local-worker leaked %q in output: %q", forbidden, out)
		}
	}
}
