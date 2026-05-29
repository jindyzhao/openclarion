package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectsCommonFrameworkDependencies(t *testing.T) {
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
			body:     `{"optionalDependencies":{"llama-index":"1.0.0"}}`,
			want:     []string{"llama-index", "must not add agent runtime dependency"},
		},
		{
			name:     "semantic kernel dev dependency",
			manifest: "web/package.json",
			body:     `{"devDependencies":{"semantic-kernel":"1.0.0"}}`,
			want:     []string{"semantic-kernel", "must not add agent runtime dependency"},
		},
		{
			name:     "pydantic ai peer dependency",
			manifest: "web/package.json",
			body:     `{"peerDependencies":{"pydantic-ai":"1.0.0"}}`,
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
			root := writeRepo(t, map[string]string{tc.manifest: tc.body})
			out, err := runCheck(t, root)
			if err == nil {
				t.Fatalf("policy check passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("policy check output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestRejectsControlPlaneRuntimeNames(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
		want string
	}{
		{
			name: "openclaw string literal",
			file: "internal/usecases/runtime/selector.go",
			body: "package runtime\n\nconst selectedRuntime = \"openclaw\"\n",
			want: "must not hard-code agent runtime family 'openclaw'",
		},
		{
			name: "hermes import path",
			file: "cmd/openclarion/main.go",
			body: "package main\n\nimport _ \"example.com/hermes-agent/runtime\"\n\nfunc main() {}\n",
			want: "must not hard-code agent runtime family 'hermes'",
		},
		{
			name: "langchain identifier",
			file: "scripts/runtime_selector/main.go",
			body: "package main\n\nvar LangchainRuntime = true\n",
			want: "must not hard-code agent runtime family 'langchain'",
		},
		{
			name: "shell script runtime branch",
			file: "scripts/runtime_selector.sh",
			body: "#!/usr/bin/env bash\nruntime=langgraph\n",
			want: "must not hard-code agent runtime family 'langgraph'",
		},
		{
			name: "frontend runtime branch",
			file: "web/src/features/diagnosis/runtime.ts",
			body: "export const runtimeFamily = \"hermes-agent\";\n",
			want: "must not hard-code agent runtime family 'hermes'",
		},
		{
			name: "frontend exact token",
			file: "web/src/features/diagnosis/runtime.ts",
			body: "export const runtimeFamily = \"agno\";\n",
			want: "must not hard-code agent runtime family '\"agno\"'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{tc.file: tc.body})
			out, err := runCheck(t, root)
			if err == nil {
				t.Fatalf("policy check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("policy check output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestAllowsCommentsAndPackageScripts(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"web/package.json": `{
			"scripts": {
				"agent-note": "echo langchain is documentation only"
			},
			"dependencies": {
				"@openclarion/diagnosis-ui": "1.0.0"
			}
		}`,
		"cmd/openclarion/main.go": "package main\n\n// TODO: evaluate langchain in a sandbox image only.\nfunc main() {}\n",
	})

	out, err := runCheck(t, root)
	if err != nil {
		t.Fatalf("policy check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[forbidden-agent-runtime] OK") {
		t.Fatalf("policy check output = %q, want OK", out)
	}
}

func TestAllowsCandidateNamesInTests(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"internal/usecases/runtime/selector_test.go": "package runtime\n\nconst fixtureRuntime = \"openclaw\"\n",
		"web/src/features/diagnosis/runtime.test.ts": "export const fixtureRuntime = \"hermes-agent\";\n",
	})

	out, err := runCheck(t, root)
	if err != nil {
		t.Fatalf("policy check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[forbidden-agent-runtime] OK") {
		t.Fatalf("policy check output = %q, want OK", out)
	}
}

func TestPolicyIsConfigDriven(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"docs/design/ci/agent-runtime-forbidden.tsv": "manifest\tacme-agent\ncode\tacme-agent\n",
		"web/package.json":                           `{"dependencies":{"acme-agent":"1.0.0"}}`,
		"internal/usecases/runtime/selector.go":      "package runtime\n\nconst selectedRuntime = \"acme-agent\"\n",
	})

	out, err := runCheck(t, root)
	if err == nil {
		t.Fatalf("policy check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"acme-agent",
		"must not add agent runtime dependency",
		"must not hard-code agent runtime family",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("policy check output = %q, want substring %q", out, want)
		}
	}
}

func TestRejectsWeakPolicyRows(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		want   string
	}{
		{
			name:   "duplicate",
			policy: "manifest\tacme-agent\nmanifest\tacme-agent\ncode\tacme-agent\n",
			want:   "duplicate policy row",
		},
		{
			name:   "whitespace padded scope",
			policy: " manifest\tacme-agent\ncode\tacme-agent\n",
			want:   "scope and pattern must not contain leading or trailing whitespace",
		},
		{
			name:   "whitespace padded pattern",
			policy: "manifest\t acme-agent\ncode\tacme-agent\n",
			want:   "scope and pattern must not contain leading or trailing whitespace",
		},
		{
			name:   "incomplete",
			policy: "manifest\tacme-agent\n",
			want:   "policy file must define at least one manifest and one code pattern",
		},
		{
			name:   "invalid exact pattern",
			policy: "manifest\t\"agno\ncode\t`agno`\n",
			want:   "invalid exact-match pattern",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{
				"docs/design/ci/agent-runtime-forbidden.tsv": tc.policy,
			})
			out, err := runCheck(t, root)
			if err == nil {
				t.Fatalf("policy check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("policy check output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestRejectsWeakStructuredInputs(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
		want string
	}{
		{
			name: "duplicate package json key",
			file: "web/package.json",
			body: `{"dependencies":{},"dependencies":{}}`,
			want: "duplicate object key",
		},
		{
			name: "non object dependency section",
			file: "web/package.json",
			body: `{"dependencies":["langchain"]}`,
			want: "dependencies must be an object",
		},
		{
			name: "null package json root",
			file: "web/package.json",
			body: `null`,
			want: "package.json must be an object",
		},
		{
			name: "null dependency section",
			file: "web/package.json",
			body: `{"dependencies":null}`,
			want: "dependencies must be an object",
		},
		{
			name: "invalid go file",
			file: "internal/usecases/runtime/selector.go",
			body: "package runtime\n\nfunc broken( {",
			want: "parse internal/usecases/runtime/selector.go",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{tc.file: tc.body})
			out, err := runCheck(t, root)
			if err == nil {
				t.Fatalf("policy check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("policy check output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestRejectsSymlinkPolicyAndManifest(t *testing.T) {
	t.Run("policy", func(t *testing.T) {
		root := writeRepo(t, nil)
		policyPath := filepath.Join(root, "docs", "design", "ci", "agent-runtime-forbidden.tsv")
		targetPath := filepath.Join(root, "policy-target.tsv")
		if err := os.Rename(policyPath, targetPath); err != nil {
			t.Fatalf("rename policy: %v", err)
		}
		if err := os.Symlink(targetPath, policyPath); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		out, err := runCheck(t, root)
		if err == nil {
			t.Fatalf("policy check passed unexpectedly:\n%s", out)
		}
		if !strings.Contains(out, "must be a regular file, not a symlink") {
			t.Fatalf("policy check output = %q, want symlink rejection", out)
		}
	})

	t.Run("manifest", func(t *testing.T) {
		root := writeRepo(t, map[string]string{
			"web/package-target.json": `{"dependencies":{}}`,
		})
		linkPath := filepath.Join(root, "web", "package.json")
		targetPath := filepath.Join(root, "web", "package-target.json")
		if err := os.Symlink(targetPath, linkPath); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		out, err := runCheck(t, root)
		if err == nil {
			t.Fatalf("policy check passed unexpectedly:\n%s", out)
		}
		if !strings.Contains(out, "web/package.json must be a regular file, not a symlink") {
			t.Fatalf("policy check output = %q, want manifest symlink rejection", out)
		}
	})

	t.Run("source", func(t *testing.T) {
		root := writeRepo(t, map[string]string{
			"scripts/runtime-target.sh": "runtime=langgraph\n",
		})
		linkPath := filepath.Join(root, "scripts", "runtime_selector.sh")
		targetPath := filepath.Join(root, "scripts", "runtime-target.sh")
		if err := os.Symlink(targetPath, linkPath); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		out, err := runCheck(t, root)
		if err == nil {
			t.Fatalf("policy check passed unexpectedly:\n%s", out)
		}
		if !strings.Contains(out, "scripts/runtime_selector.sh must be a regular file, not a symlink") {
			t.Fatalf("policy check output = %q, want source symlink rejection", out)
		}
	})
}

func TestAllowsCandidateNamesInDocsAndAgnoSubstrings(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"docs/design/agent-runtime-selection.md":  "OpenClaw and Hermes Agent are candidate runtime evidence values.\n",
		"web/package.json":                        `{"dependencies":{"@openclarion/diagnosis-ui":"1.0.0"}}`,
		"internal/usecases/diagnosis/selector.go": "package diagnosis\n\nconst domain = \"diagnosis\"\n",
	})

	out, err := runCheck(t, root)
	if err != nil {
		t.Fatalf("policy check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[forbidden-agent-runtime] OK") {
		t.Fatalf("policy check output = %q, want OK", out)
	}
}

func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "docs/design/ci/agent-runtime-forbidden.tsv", currentPolicy(t), 0o644)
	if _, ok := files["go.mod"]; !ok {
		writeFile(t, root, "go.mod", "module example.test/openclarion\n\ngo 1.25.10\n", 0o644)
	}
	for name, body := range files {
		writeFile(t, root, name, body, 0o644)
	}
	return root
}

func currentPolicy(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "design", "ci", "agent-runtime-forbidden.tsv"))
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	return string(raw)
}

func writeFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run([]string{"--root", root}, &stdout, &stderr)
	out := stdout.String() + stderr.String()
	if err != nil {
		out += err.Error()
	}
	return out, err
}
