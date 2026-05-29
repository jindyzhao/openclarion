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

func TestForbiddenAgentRuntimeRejectsCommonFrameworks(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		body     string
		want     []string
	}{
		{
			name:     "langgraph dependency",
			manifest: "web/package.json",
			body:     `{"dependencies":{"@langchain/langgraph":"1.0.0"}}`,
			want:     []string{"langgraph", "must not add agent runtime dependency"},
		},
		{
			name:     "llamaindex dependency",
			manifest: "web/package.json",
			body:     `{"dependencies":{"llamaindex":"1.0.0"}}`,
			want:     []string{"llamaindex", "must not add agent runtime dependency"},
		},
		{
			name:     "llama index hyphen dependency",
			manifest: "web/package.json",
			body:     `{"dependencies":{"llama-index":"1.0.0"}}`,
			want:     []string{"llama-index", "must not add agent runtime dependency"},
		},
		{
			name:     "semantic kernel dependency",
			manifest: "web/package.json",
			body:     `{"dependencies":{"semantic-kernel":"1.0.0"}}`,
			want:     []string{"semantic-kernel", "must not add agent runtime dependency"},
		},
		{
			name:     "pydantic ai dependency",
			manifest: "web/package.json",
			body:     `{"dependencies":{"pydantic-ai":"1.0.0"}}`,
			want:     []string{"pydantic-ai", "must not add agent runtime dependency"},
		},
		{
			name:     "agno npm dependency",
			manifest: "web/package.json",
			body:     `{"dependencies":{"agno":"1.0.0"}}`,
			want:     []string{`"agno"`, "must not add agent runtime dependency"},
		},
		{
			name:     "agno go module dependency",
			manifest: "go.mod",
			body:     "module example.test/openclarion\n\ngo 1.25.10\n\nrequire github.com/agno-agi/agno v0.1.0\n",
			want:     []string{"agno-agi/agno", "must not add agent runtime dependency"},
		},
		{
			name:     "mastra dependency",
			manifest: "web/package.json",
			body:     `{"dependencies":{"@mastra/core":"1.0.0"}}`,
			want:     []string{"@mastra/", "must not add agent runtime dependency"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeAgentRuntimeRepo(t, map[string]string{
				tc.manifest: tc.body,
			})

			out, err := runForbiddenAgentRuntime(t, root)
			if err == nil {
				t.Fatalf("forbidden-agent-runtime passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("forbidden-agent-runtime output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestForbiddenAgentRuntimeRejectsControlPlaneHardcodedRuntimeNames(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
		want string
	}{
		{
			name: "openclaw string",
			file: "internal/usecases/runtime/selector.go",
			body: "package runtime\n\nconst selectedRuntime = \"openclaw\"\n",
			want: "must not hard-code agent runtime family 'openclaw'",
		},
		{
			name: "hermes string",
			file: "scripts/sandbox_m4_decision/check.go",
			body: "package main\n\nconst selectedRuntime = \"hermes-agent\"\n",
			want: "must not hard-code agent runtime family 'hermes'",
		},
		{
			name: "langchain comment",
			file: "cmd/openclarion/main.go",
			body: "package main\n\n// TODO: call langchain here.\nfunc main() {}\n",
			want: "must not hard-code agent runtime family 'langchain'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeAgentRuntimeRepo(t, map[string]string{
				tc.file: tc.body,
			})

			out, err := runForbiddenAgentRuntime(t, root)
			if err == nil {
				t.Fatalf("forbidden-agent-runtime passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("forbidden-agent-runtime output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestForbiddenAgentRuntimePolicyIsConfigDriven(t *testing.T) {
	root := writeAgentRuntimeRepo(t, map[string]string{
		"docs/design/ci/agent-runtime-forbidden.tsv": "manifest\tacme-agent\ncode\tacme-agent\n",
		"web/package.json":                           `{"dependencies":{"acme-agent":"1.0.0"}}`,
		"internal/usecases/runtime/selector.go":      "package runtime\n\nconst selectedRuntime = \"acme-agent\"\n",
	})

	out, err := runForbiddenAgentRuntime(t, root)
	if err == nil {
		t.Fatalf("forbidden-agent-runtime passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"acme-agent",
		"must not add agent runtime dependency",
		"must not hard-code agent runtime family",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("forbidden-agent-runtime output = %q, want substring %q", out, want)
		}
	}
}

func TestForbiddenAgentRuntimeRejectsDuplicatePolicyRows(t *testing.T) {
	root := writeAgentRuntimeRepo(t, map[string]string{
		"docs/design/ci/agent-runtime-forbidden.tsv": "manifest\tacme-agent\nmanifest\tacme-agent\ncode\tacme-agent\n",
	})

	out, err := runForbiddenAgentRuntime(t, root)
	if err == nil {
		t.Fatalf("forbidden-agent-runtime passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "duplicate policy row") {
		t.Fatalf("forbidden-agent-runtime output = %q, want duplicate policy row error", out)
	}
}

func TestForbiddenAgentRuntimeRejectsWhitespacePaddedPolicyRows(t *testing.T) {
	tests := []struct {
		name   string
		policy string
	}{
		{
			name:   "scope",
			policy: " manifest\tacme-agent\ncode\tacme-agent\n",
		},
		{
			name:   "pattern",
			policy: "manifest\t acme-agent\ncode\tacme-agent\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeAgentRuntimeRepo(t, map[string]string{
				"docs/design/ci/agent-runtime-forbidden.tsv": tc.policy,
			})

			out, err := runForbiddenAgentRuntime(t, root)
			if err == nil {
				t.Fatalf("forbidden-agent-runtime passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, "scope and pattern must not contain leading or trailing whitespace") {
				t.Fatalf("forbidden-agent-runtime output = %q, want whitespace policy error", out)
			}
		})
	}
}

func TestForbiddenAgentRuntimeRejectsIncompletePolicy(t *testing.T) {
	root := writeAgentRuntimeRepo(t, map[string]string{
		"docs/design/ci/agent-runtime-forbidden.tsv": "manifest\tacme-agent\n",
	})

	out, err := runForbiddenAgentRuntime(t, root)
	if err == nil {
		t.Fatalf("forbidden-agent-runtime passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "policy file must define at least one manifest and one code pattern") {
		t.Fatalf("forbidden-agent-runtime output = %q, want incomplete policy error", out)
	}
}

func TestForbiddenAgentRuntimeAllowsCandidateNamesInDocs(t *testing.T) {
	root := writeAgentRuntimeRepo(t, map[string]string{
		"docs/design/agent-runtime-selection.md": "OpenClaw and Hermes Agent are candidate runtime evidence values.\n",
		"internal/usecases/runtime/selector.go":  "package runtime\n\nconst selectedRuntime = \"evidence-supplied\"\n",
	})

	out, err := runForbiddenAgentRuntime(t, root)
	if err != nil {
		t.Fatalf("forbidden-agent-runtime failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[forbidden-agent-runtime] OK") {
		t.Fatalf("forbidden-agent-runtime output = %q, want OK", out)
	}
}

func TestForbiddenAgentRuntimeAvoidsAgnoSubstringFalsePositive(t *testing.T) {
	root := writeAgentRuntimeRepo(t, map[string]string{
		"web/package.json":                        `{"dependencies":{"@openclarion/diagnosis-ui":"1.0.0"}}`,
		"internal/usecases/diagnosis/selector.go": "package diagnosis\n\nconst domain = \"diagnosis\"\n",
	})

	out, err := runForbiddenAgentRuntime(t, root)
	if err != nil {
		t.Fatalf("forbidden-agent-runtime failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[forbidden-agent-runtime] OK") {
		t.Fatalf("forbidden-agent-runtime output = %q, want OK", out)
	}
}

func writeAgentRuntimeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeAgentRuntimeFile(t, root, "scripts/check_no_control_plane_agent_runtime_deps.sh", forbiddenAgentRuntimeScript(t), 0o750)
	writeAgentRuntimeFile(t, root, "docs/design/ci/agent-runtime-forbidden.tsv", forbiddenAgentRuntimePolicy(t), 0o644)
	if _, ok := files["go.mod"]; !ok {
		writeAgentRuntimeFile(t, root, "go.mod", "module example.test/openclarion\n\ngo 1.25.10\n", 0o644)
	}
	for name, body := range files {
		writeAgentRuntimeFile(t, root, name, body, 0o644)
	}
	return root
}

func forbiddenAgentRuntimeScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_no_control_plane_agent_runtime_deps.sh")
	if err != nil {
		t.Fatalf("read forbidden-agent-runtime script: %v", err)
	}
	return string(raw)
}

func forbiddenAgentRuntimePolicy(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "docs", "design", "ci", "agent-runtime-forbidden.tsv"))
	if err != nil {
		t.Fatalf("read forbidden-agent-runtime policy: %v", err)
	}
	return string(raw)
}

func writeAgentRuntimeFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runForbiddenAgentRuntime(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_no_control_plane_agent_runtime_deps.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
