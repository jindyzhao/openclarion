package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPRDescriptionCheckAcceptsRiskAndRollbackSections(t *testing.T) {
	root := newPRDescriptionFixture(t)
	body := `Summary text.

## Risk

Low. This only changes CI policy.

## Rollback

Revert this PR.
`

	out, err := runPRDescriptionCheck(t, root, map[string]string{"PR_BODY": body})
	if err != nil {
		t.Fatalf("pr description check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[pr-description-check] OK") {
		t.Fatalf("pr description output = %q, want OK", out)
	}
}

func TestPRDescriptionCheckReadsGitHubEventPath(t *testing.T) {
	root := newPRDescriptionFixture(t)
	body := "## Rollback\n\nRevert this PR.\n\n## Risk\n\nNone; documentation only.\n"
	event := map[string]any{"pull_request": map[string]any{"body": body}}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	eventPath := filepath.Join(root, "event.json")
	if err := os.WriteFile(eventPath, raw, 0o600); err != nil {
		t.Fatalf("write event: %v", err)
	}

	out, err := runPRDescriptionCheck(t, root, map[string]string{"GITHUB_EVENT_PATH": eventPath})
	if err != nil {
		t.Fatalf("pr description check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[pr-description-check] OK") {
		t.Fatalf("pr description output = %q, want OK", out)
	}
}

func TestPRDescriptionCheckRejectsMissingOrEmptySections(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing risk",
			body: "## Rollback\n\nRevert this PR.\n",
			want: "missing ## Risk",
		},
		{
			name: "missing rollback",
			body: "## Risk\n\nLow.\n",
			want: "missing ## Rollback",
		},
		{
			name: "empty risk",
			body: "## Risk\n\n<!-- fill this in -->\n\n## Rollback\n\nRevert this PR.\n",
			want: "empty ## Risk",
		},
		{
			name: "empty body",
			body: "",
			want: "PR description is empty",
		},
		{
			name: "heading hidden in code fence",
			body: "```\n## Risk\nfake\n```\n\n## Rollback\n\nRevert this PR.\n",
			want: "missing ## Risk",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newPRDescriptionFixture(t)
			out, err := runPRDescriptionCheck(t, root, map[string]string{"PR_BODY": tc.body})
			if err == nil {
				t.Fatalf("pr description check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("pr description output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestPRDescriptionCheckRejectsUnsafeGitHubEventPath(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string) string
		want  string
	}{
		{
			name: "symlink event path",
			setup: func(t *testing.T, root string) string {
				target := filepath.Join(root, "target-event.json")
				if err := os.WriteFile(target, []byte(validPRDescriptionEvent()), 0o600); err != nil {
					t.Fatalf("write target event: %v", err)
				}
				eventPath := filepath.Join(root, "event.json")
				if err := os.Symlink(target, eventPath); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
				return eventPath
			},
			want: "GITHUB_EVENT_PATH must be a regular file, not a symlink",
		},
		{
			name: "directory event path",
			setup: func(t *testing.T, root string) string {
				eventPath := filepath.Join(root, "event.json")
				if err := os.Mkdir(eventPath, 0o750); err != nil {
					t.Fatalf("mkdir event path: %v", err)
				}
				return eventPath
			},
			want: "GITHUB_EVENT_PATH must be a regular file",
		},
		{
			name: "duplicate event keys",
			setup: func(t *testing.T, root string) string {
				eventPath := filepath.Join(root, "event.json")
				body := `{"pull_request":{"body":"## Risk\n\nLow.\n\n## Rollback\n\nRevert.\n","body":"## Risk\n\nShadow.\n\n## Rollback\n\nRevert.\n"}}`
				if err := os.WriteFile(eventPath, []byte(body), 0o600); err != nil {
					t.Fatalf("write event path: %v", err)
				}
				return eventPath
			},
			want: "duplicate JSON key: body",
		},
		{
			name: "non-object pull request",
			setup: func(t *testing.T, root string) string {
				eventPath := filepath.Join(root, "event.json")
				if err := os.WriteFile(eventPath, []byte(`{"pull_request":"bad"}`), 0o600); err != nil {
					t.Fatalf("write event path: %v", err)
				}
				return eventPath
			},
			want: "pull_request must be an object",
		},
		{
			name: "non-string body",
			setup: func(t *testing.T, root string) string {
				eventPath := filepath.Join(root, "event.json")
				if err := os.WriteFile(eventPath, []byte(`{"pull_request":{"body":123}}`), 0o600); err != nil {
					t.Fatalf("write event path: %v", err)
				}
				return eventPath
			},
			want: "pull_request.body must be a string or null",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newPRDescriptionFixture(t)
			eventPath := tc.setup(t, root)

			out, err := runPRDescriptionCheck(t, root, map[string]string{"GITHUB_EVENT_PATH": eventPath})
			if err == nil {
				t.Fatalf("pr description check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("pr description output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestPRDescriptionCheckRequiresInput(t *testing.T) {
	root := newPRDescriptionFixture(t)
	t.Setenv("PR_BODY", "## Risk\n\nLow.\n\n## Rollback\n\nRevert this PR.\n")
	t.Setenv("GITHUB_EVENT_PATH", filepath.Join(root, "event.json"))

	out, err := runPRDescriptionCheck(t, root, nil)
	if err == nil {
		t.Fatalf("pr description check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"PR_BODY or GITHUB_EVENT_PATH is required",
		"make pr-description-check",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pr description output = %q, want substring %q", out, want)
		}
	}
}

func validPRDescriptionEvent() string {
	return `{"pull_request":{"body":"## Risk\n\nLow.\n\n## Rollback\n\nRevert.\n"}}`
}

func newPRDescriptionFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	prDescriptionWriteFile(t, root, "scripts/check_pr_description.sh", prDescriptionScript(t), 0o750)
	return root
}

func prDescriptionScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_pr_description.sh")
	if err != nil {
		t.Fatalf("read PR description script: %v", err)
	}
	return string(raw)
}

func prDescriptionWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runPRDescriptionCheck(t *testing.T, root string, env map[string]string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_pr_description.sh")
	cmd.Dir = root
	cmd.Env = minimalPRDescriptionCheckEnv()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}

func minimalPRDescriptionCheckEnv() []string {
	var env []string
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	return env
}
