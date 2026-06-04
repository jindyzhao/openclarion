package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dockerprovider "github.com/openclarion/openclarion/internal/providers/container/docker"
)

func TestRunEmitsPassingBaselineAudit(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(&stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out auditOutput
	if err := json.NewDecoder(&stdout).Decode(&out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if out.Tool != "sandbox_baseline_audit" {
		t.Fatalf("Tool = %q", out.Tool)
	}
	if out.Status != "pass" {
		t.Fatalf("Status = %q", out.Status)
	}
	want := []string{
		"fixed_file_contract",
		"batch_network_none_spec",
		"m5_turn_input_mounts",
		"docker_security_posture",
		"allowlist_enforcer_subset",
		"allowlist_enforcer_drift_rejection",
		"raw_result_validation",
	}
	if len(out.Checks) != len(want) {
		t.Fatalf("Checks = %v, want %d checks", out.Checks, len(want))
	}
	for i, name := range want {
		if out.Checks[i].Name != name || out.Checks[i].Status != "pass" {
			t.Fatalf("Checks[%d] = %+v, want %s pass", i, out.Checks[i], name)
		}
	}
}

func TestRunWritesBaselineAuditOutputFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "baseline-audit.json")

	var stdout bytes.Buffer
	if err := runWithArgs([]string{"--out", outPath}, &stdout); err != nil {
		t.Fatalf("runWithArgs: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when --out is used", stdout.String())
	}
	// #nosec G304 -- test reads the temp output path produced by this run.
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var out auditOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output %q: %v", raw, err)
	}
	if out.Tool != "sandbox_baseline_audit" || out.Status != "pass" || len(out.Checks) == 0 {
		t.Fatalf("output = %+v, want passing baseline audit", out)
	}
}

func TestRunRejectsExistingOutputFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "baseline-audit.json")
	if err := os.WriteFile(outPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	var stdout bytes.Buffer
	err := runWithArgs([]string{"--out", outPath}, &stdout)
	if err == nil {
		t.Fatal("runWithArgs err = nil, want existing output rejection")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("runWithArgs err = %v, want already exists", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on output rejection", stdout.String())
	}
}

func TestRunRejectsUnexpectedArguments(t *testing.T) {
	var stdout bytes.Buffer
	err := runWithArgs([]string{"unexpected"}, &stdout)
	if err == nil {
		t.Fatal("runWithArgs err = nil, want unexpected argument rejection")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("runWithArgs err = %v, want unexpected arguments", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on argument rejection", stdout.String())
	}
}

func TestIndividualChecksStayExecutable(t *testing.T) {
	checks := []auditProbe{
		{name: "fixed_file_contract", run: checkFixedFileContract},
		{name: "batch_network_none_spec", run: checkBatchNetworkNoneSpec},
		{name: "m5_turn_input_mounts", run: checkM5TurnInputMounts},
		{name: "docker_security_posture", run: checkDockerSecurityPosture},
		{name: "allowlist_enforcer_subset", run: checkAllowlistEnforcerSubset},
		{name: "allowlist_enforcer_drift_rejection", run: checkAllowlistEnforcerDriftRejection},
		{name: "raw_result_validation", run: checkRawResultValidation},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err != nil {
				t.Fatalf("%s: %v", check.name, err)
			}
		})
	}
}

func TestRequireReadonlyMountRejectsMissingTarget(t *testing.T) {
	err := requireReadonlyMount(mustBaselineSpec(t), "/workspace/missing.json")
	if err == nil {
		t.Fatal("requireReadonlyMount err = nil, want missing target")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("requireReadonlyMount err = %v, want not found", err)
	}
}

func mustBaselineSpec(t *testing.T) dockerprovider.RunSpec {
	t.Helper()
	spec, err := dockerprovider.BuildRunSpec(baselineConfig(), baselineRequest(), baselineWorkspace())
	if err != nil {
		t.Fatalf("BuildRunSpec: %v", err)
	}
	return spec
}
