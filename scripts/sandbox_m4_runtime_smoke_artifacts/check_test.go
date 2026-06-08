package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const imageRef = "registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRunAcceptsValidRuntimeSmokeArtifactBundle(t *testing.T) {
	root := validRuntimeSmokeBundle(t)

	var stdout bytes.Buffer
	if err := run([]string{"--root", root}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "OK (5 artifacts)") {
		t.Fatalf("stdout = %q, want OK", stdout.String())
	}
}

func TestRunRejectsMissingArtifact(t *testing.T) {
	root := validRuntimeSmokeBundle(t)
	if err := os.Remove(filepath.Join(root, "egress-allowdeny-smoke.json")); err != nil {
		t.Fatalf("remove artifact: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--root", root}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want missing artifact error")
	}
	if !strings.Contains(err.Error(), `missing required artifact "egress-allowdeny-smoke.json"`) {
		t.Fatalf("run err = %q, want missing artifact", err.Error())
	}
}

func TestRunRejectsUnexpectedArtifact(t *testing.T) {
	root := validRuntimeSmokeBundle(t)
	writeRuntimeArtifact(t, root, "notes.txt", "operator notes")

	var stdout bytes.Buffer
	err := run([]string{"--root", root}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want unexpected artifact error")
	}
	if !strings.Contains(err.Error(), `unexpected artifact "notes.txt"`) {
		t.Fatalf("run err = %q, want unexpected artifact", err.Error())
	}
}

func TestRunRejectsDuplicateJSONKeys(t *testing.T) {
	root := validRuntimeSmokeBundle(t)
	writeRuntimeArtifact(t, root, "agent-runtime-smoke.json", `{"tool":"agent-runtime-smoke","tool":"agent-runtime-smoke"}`)

	var stdout bytes.Buffer
	err := run([]string{"--root", root}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want duplicate key error")
	}
	if !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("run err = %q, want duplicate key", err.Error())
	}
}

func TestRunRejectsWrongContainerProviderMode(t *testing.T) {
	root := validRuntimeSmokeBundle(t)
	writeRuntimeArtifact(t, root, "container-provider-timeout-smoke.json", containerProviderSuccessArtifact("make container-provider-timeout-smoke"))

	var stdout bytes.Buffer
	err := run([]string{"--root", root}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want mode error")
	}
	if !strings.Contains(err.Error(), `mode = "success", want expected_error`) {
		t.Fatalf("run err = %q, want expected_error mode rejection", err.Error())
	}
}

func TestRunRejectsMutableImageReference(t *testing.T) {
	root := validRuntimeSmokeBundle(t)
	writeRuntimeArtifact(t, root, "egress-allowdeny-smoke.json", strings.Replace(egressAllowDenyArtifactJSON(), imageRef, "registry.example.com/openclarion/runtime-candidate-a:latest", 1))

	var stdout bytes.Buffer
	err := run([]string{"--root", root}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want image ref error")
	}
	if !strings.Contains(err.Error(), "image_ref must be name@sha256") {
		t.Fatalf("run err = %q, want image ref rejection", err.Error())
	}
}

func TestRunRejectsSymlinkArtifact(t *testing.T) {
	root := validRuntimeSmokeBundle(t)
	if err := os.Remove(filepath.Join(root, "agent-runtime-smoke.json")); err != nil {
		t.Fatalf("remove artifact: %v", err)
	}
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "target.json")
	if err := os.WriteFile(target, []byte(agentRuntimeArtifactJSON()), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(root, "agent-runtime-smoke.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--root", root}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink error")
	}
	if !strings.Contains(err.Error(), "artifact must be a regular file, not a symlink") {
		t.Fatalf("run err = %q, want symlink rejection", err.Error())
	}
}

func validRuntimeSmokeBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRuntimeArtifact(t, root, "agent-runtime-smoke.json", agentRuntimeArtifactJSON())
	writeRuntimeArtifact(t, root, "container-provider-smoke.json", containerProviderSuccessArtifact("make container-provider-smoke"))
	writeRuntimeArtifact(t, root, "container-provider-timeout-smoke.json", containerProviderExpectedErrorArtifact("make container-provider-timeout-smoke"))
	writeRuntimeArtifact(t, root, "container-provider-output-cap-smoke.json", containerProviderExpectedErrorArtifact("make container-provider-output-cap-smoke"))
	writeRuntimeArtifact(t, root, "egress-allowdeny-smoke.json", egressAllowDenyArtifactJSON())
	return root
}

func writeRuntimeArtifact(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func agentRuntimeArtifactJSON() string {
	return `{
  "tool": "agent-runtime-smoke",
  "status": "pass",
  "source": "make agent-runtime-smoke",
  "runtime_candidate": "` + imageRef + `",
  "output": {
    "path": "/workspace/out/output.json",
    "bytes": 128,
    "max_bytes": 10485760,
    "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },
  "checks": [
    {"name": "regular_output_file", "status": "pass"},
    {"name": "bounded_output_size", "status": "pass"},
    {"name": "valid_json_object", "status": "pass"},
    {"name": "duplicate_key_free", "status": "pass"},
    {"name": "non_empty_object", "status": "pass"}
  ]
}`
}

func containerProviderSuccessArtifact(source string) string {
	return `{
  "tool": "container-provider-smoke",
  "status": "pass",
  "source": "` + source + `",
  "image_ref": "` + imageRef + `",
  "invocation_id": "container-provider-smoke-1",
  "mode": "success",
  "timeout_seconds": 60,
  "output": {
    "bytes": 64,
    "max_bytes": 10485760,
    "sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  },
  "checks": [
    {"name": "digest_pinned_image", "status": "pass"},
    {"name": "request_validated", "status": "pass"},
    {"name": "network_none", "status": "pass"},
    {"name": "readonly_rootfs", "status": "pass"},
    {"name": "no_new_privileges", "status": "pass"},
    {"name": "cap_drop_all", "status": "pass"},
    {"name": "provider_run_succeeded", "status": "pass"},
    {"name": "valid_json_object_output", "status": "pass"},
    {"name": "duplicate_key_free_output", "status": "pass"}
  ]
}`
}

func containerProviderExpectedErrorArtifact(source string) string {
	return `{
  "tool": "container-provider-smoke",
  "status": "pass",
  "source": "` + source + `",
  "image_ref": "` + imageRef + `",
  "invocation_id": "container-provider-smoke-1",
  "mode": "expected_error",
  "timeout_seconds": 2,
  "expected_error": {
    "pattern": "context deadline exceeded",
    "matched": "context deadline exceeded"
  },
  "checks": [
    {"name": "digest_pinned_image", "status": "pass"},
    {"name": "request_validated", "status": "pass"},
    {"name": "network_none", "status": "pass"},
    {"name": "readonly_rootfs", "status": "pass"},
    {"name": "no_new_privileges", "status": "pass"},
    {"name": "cap_drop_all", "status": "pass"},
    {"name": "expected_provider_error_observed", "status": "pass"}
  ]
}`
}

func egressAllowDenyArtifactJSON() string {
	return `{
  "tool": "egress-allowdeny-smoke",
  "status": "pass",
  "source": "make egress-allowdeny-smoke",
  "image_ref": "` + imageRef + `",
  "run_id": "manual-run-1",
  "timeout_seconds": 8,
  "topology": {
    "sandbox_network": "internal",
    "upstream_network": "separate",
    "proxy_target": "egress-proxy:18080",
    "allowed_target": "allowed.internal:8080",
    "denied_target": "denied.internal:8080",
    "direct_bypass_target": "allowed.internal:8080"
  },
  "checks": [
    {"name": "digest_pinned_image", "status": "pass"},
    {"name": "sandbox_network_internal", "status": "pass"},
    {"name": "upstream_network_separate", "status": "pass"},
    {"name": "proxy_dual_network", "status": "pass"},
    {"name": "allowed_target_via_proxy", "status": "pass"},
    {"name": "denied_target_blocked_by_proxy", "status": "pass"},
    {"name": "direct_bypass_failed", "status": "pass"},
    {"name": "non_root_readonly_no_new_privileges_cap_drop", "status": "pass"}
  ]
}`
}
