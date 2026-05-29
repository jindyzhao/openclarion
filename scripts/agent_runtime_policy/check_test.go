package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAllowsRuntimeNamesOutsideStructuredDependencies(t *testing.T) {
	root := newRepo(t, map[string]string{
		"web/package.json": `{
			"description": "documents an acme-agent candidate",
			"scripts": {"notes": "echo acme-agent evidence"},
			"overrides": {"acme-agent": "1.0.0"}
		}`,
		"internal/usecases/runtime/selector.go": "package runtime\n\nconst selectedRuntime = \"evidence-supplied\"\n",
	})

	if err := run(root); err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

func TestRunRejectsStructuredManifestDependenciesAndCodeHardcoding(t *testing.T) {
	root := newRepo(t, map[string]string{
		"go.mod":                              "module example.test/openclarion\n\ngo 1.25.10\n\nrequire example.com/acme-agent v1.0.0\n",
		"web/package.json":                    `{"devDependencies":{"acme-agent":"1.0.0"}}`,
		"internal/usecases/runtime/select.go": "package runtime\n\nconst selectedRuntime = \"acme-agent\"\n",
	})

	err := run(root)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{
		`devDependencies dependency "acme-agent"`,
		`require path "example.com/acme-agent"`,
		"must not hard-code agent runtime family 'acme-agent'",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestRunRejectsGoToolDirective(t *testing.T) {
	root := newRepo(t, map[string]string{
		"go.mod": "module example.test/openclarion\n\ngo 1.25.10\n\ntool example.com/acme-agent/cmd/acme\n",
	})

	err := run(root)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), `tool path "example.com/acme-agent/cmd/acme"`) {
		t.Fatalf("run error = %q, want tool directive rejection", err.Error())
	}
}

func TestRunAllowsGoModReplaceWithoutRequire(t *testing.T) {
	root := newRepo(t, map[string]string{
		"go.mod": "module example.test/openclarion\n\ngo 1.25.10\n\nreplace example.com/acme-agent => ../agent\n",
	})

	if err := run(root); err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

func TestRunRejectsMalformedInputs(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name: "duplicate policy",
			files: map[string]string{
				"docs/design/ci/agent-runtime-forbidden.tsv": "manifest\tacme-agent\nmanifest\tacme-agent\ncode\tacme-agent\n",
			},
			want: "duplicate policy row",
		},
		{
			name: "bad dependency section",
			files: map[string]string{
				"web/package.json": `{"dependencies":["acme-agent"]}`,
			},
			want: "dependencies must be a JSON object",
		},
		{
			name: "unterminated go mod block",
			files: map[string]string{
				"go.mod": "module example.test/openclarion\n\ngo 1.25.10\n\nrequire (\nexample.com/acme-agent v1.0.0\n",
			},
			want: "unterminated block",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newRepo(t, tc.files)
			err := run(root)
			if err == nil {
				t.Fatal("run passed unexpectedly")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("run error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("OPENCLARION_AGENT_RUNTIME_POLICY_FILE", filepath.Join(root, "docs/design/ci/agent-runtime-forbidden.tsv"))
	if _, ok := files["docs/design/ci/agent-runtime-forbidden.tsv"]; !ok {
		writeFile(t, root, "docs/design/ci/agent-runtime-forbidden.tsv", "manifest\tacme-agent\ncode\tacme-agent\n")
	}
	if _, ok := files["go.mod"]; !ok {
		writeFile(t, root, "go.mod", "module example.test/openclarion\n\ngo 1.25.10\n")
	}
	for path, body := range files {
		writeFile(t, root, path, body)
	}
	return root
}

func writeFile(t *testing.T, root, path, body string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", fullPath, err)
	}
}
