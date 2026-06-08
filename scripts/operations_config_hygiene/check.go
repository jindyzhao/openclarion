// Command operations_config_hygiene validates alert-operations configuration
// surfaces for endpoint and browser-storage hygiene.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
)

const defaultRepoRoot = "."

var urlPattern = regexp.MustCompile(`https?://[^\s"'` + "`" + `<>]+`)

var browserStoragePatterns = []string{
	"localStorage",
	"sessionStorage",
	"indexedDB",
	"document.cookie",
	"navigator.storage",
}

type config struct {
	RepoRoot string
}

type fileIssue struct {
	Path    string
	Message string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.RepoRoot, "repo-root", defaultRepoRoot, "repository root")
	flag.Parse()

	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[operations-config-hygiene] %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	files, err := trackedFiles(ctx, cfg.RepoRoot)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("git index has no tracked files")
	}

	var checked int
	var issues []fileIssue
	for _, file := range files {
		if !isOperationsConfigSurface(file) && !isBrowserDurableStateSurface(file) {
			continue
		}
		checked++
		data, err := readTrackedFile(cfg.RepoRoot, file)
		if err != nil {
			issues = append(issues, fileIssue{Path: file, Message: err.Error()})
			continue
		}
		issues = append(issues, inspectFile(file, data)...)
	}
	if checked == 0 {
		return fmt.Errorf("no tracked operations configuration files found")
	}
	if len(issues) > 0 {
		sort.Slice(issues, func(i, j int) bool {
			if issues[i].Path == issues[j].Path {
				return issues[i].Message < issues[j].Message
			}
			return issues[i].Path < issues[j].Path
		})
		lines := make([]string, 0, len(issues))
		for _, issue := range issues {
			lines = append(lines, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
		}
		return fmt.Errorf("%s", strings.Join(lines, "\n"))
	}

	fmt.Fprintf(out, "[operations-config-hygiene] OK (%d tracked configuration files checked)\n", checked)
	return nil
}

func trackedFiles(ctx context.Context, repoRoot string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return nil, fmt.Errorf("git ls-files failed: %s", stderr)
			}
		}
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}
	return splitNULPaths(out), nil
}

func splitNULPaths(raw []byte) []string {
	records := bytes.Split(raw, []byte{0})
	paths := make([]string, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		paths = append(paths, string(record))
	}
	return paths
}

func readTrackedFile(repoRoot, file string) ([]byte, error) {
	if file == "" || strings.HasPrefix(file, "/") || strings.Contains(file, "\\") {
		return nil, fmt.Errorf("invalid tracked path")
	}
	clean := path.Clean(file)
	if clean == "." || clean != file || strings.HasPrefix(clean, "../") {
		return nil, fmt.Errorf("invalid tracked path")
	}
	fullPath := repoRoot + string(os.PathSeparator) + file
	data, err := os.ReadFile(fullPath) // #nosec G304 -- path comes from git ls-files in the selected repo.
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	return data, nil
}

func isOperationsConfigSurface(file string) bool {
	if file == "api/openapi.yaml" || file == "api/openapi.gen.go" || file == "web/src/lib/api/openapi.ts" {
		return true
	}
	if file == "docs/adr/ADR-0014-alert-operations-configuration.md" ||
		file == "docs/design/CURRENT_STATE.md" ||
		file == "docs/design/END_TO_END_VERIFICATION.md" ||
		file == "docs/design/database/schema-catalog.md" ||
		file == "docs/roadmap/tasks.md" {
		return true
	}
	if strings.HasPrefix(file, "docs/design/frontend/") ||
		strings.HasPrefix(file, "web/src/features/settings/") ||
		strings.HasPrefix(file, "web/src/app/api/config/") ||
		strings.HasPrefix(file, "web/tests/e2e/") ||
		strings.HasPrefix(file, "artifacts/") {
		return true
	}
	if strings.HasPrefix(file, "internal/usecases/alertsource") ||
		strings.HasPrefix(file, "internal/usecases/groupingpolicy") ||
		strings.HasPrefix(file, "internal/usecases/reportworkflowpolicy") ||
		strings.HasPrefix(file, "internal/usecases/reportpolicytrigger") ||
		strings.HasPrefix(file, "internal/usecases/notificationchannel") ||
		strings.HasPrefix(file, "internal/usecases/alertreplay") {
		return true
	}
	if strings.HasPrefix(file, "internal/transport/http/") {
		name := path.Base(file)
		return strings.Contains(name, "alert_source") ||
			strings.Contains(name, "grouping_policy") ||
			strings.Contains(name, "report_workflow_policy") ||
			strings.Contains(name, "notification_channel")
	}
	return strings.HasPrefix(file, "cmd/openclarion/")
}

func isBrowserDurableStateSurface(file string) bool {
	return strings.HasPrefix(file, "web/src/")
}

func inspectFile(file string, data []byte) []fileIssue {
	var issues []fileIssue
	text := string(data)
	if isBrowserDurableStateSurface(file) {
		issues = append(issues, inspectBrowserStorage(file, text, data)...)
	}
	if isOperationsConfigSurface(file) {
		issues = append(issues, inspectURLs(file, text, data)...)
	}
	return issues
}

func inspectBrowserStorage(file, text string, data []byte) []fileIssue {
	var issues []fileIssue
	for _, pattern := range browserStoragePatterns {
		offset := 0
		for {
			idx := strings.Index(text[offset:], pattern)
			if idx < 0 {
				break
			}
			pos := offset + idx
			line, column := lineColumn(data, pos)
			issues = append(issues, fileIssue{
				Path:    file,
				Message: fmt.Sprintf("must not use browser durable storage API %q at line %d, column %d", pattern, line, column),
			})
			offset = pos + len(pattern)
		}
	}
	return issues
}

func inspectURLs(file, text string, data []byte) []fileIssue {
	matches := urlPattern.FindAllStringIndex(text, -1)
	issues := make([]fileIssue, 0)
	for _, match := range matches {
		raw := trimURLLiteral(text[match[0]:match[1]])
		parsed, err := url.Parse(normalizeTemplateURL(raw))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			if isTestFixture(file) {
				continue
			}
			line, column := lineColumn(data, match[0])
			issues = append(issues, fileIssue{
				Path:    file,
				Message: fmt.Sprintf("contains unparsable URL literal at line %d, column %d", line, column),
			})
			continue
		}
		line, column := lineColumn(data, match[0])
		if !isAllowedExampleHost(parsed.Hostname()) {
			issues = append(issues, fileIssue{
				Path:    file,
				Message: fmt.Sprintf("contains non-placeholder URL host %q at line %d, column %d", parsed.Hostname(), line, column),
			})
		}
		if parsed.User != nil && !allowsCredentialedURLFixture(file) {
			issues = append(issues, fileIssue{
				Path:    file,
				Message: fmt.Sprintf("contains URL userinfo outside a test fixture at line %d, column %d", line, column),
			})
		}
		if (parsed.RawQuery != "" || parsed.Fragment != "") && !allowsCredentialedURLFixture(file) {
			issues = append(issues, fileIssue{
				Path:    file,
				Message: fmt.Sprintf("contains URL query or fragment outside a test fixture at line %d, column %d", line, column),
			})
		}
	}
	return issues
}

func trimURLLiteral(raw string) string {
	return strings.TrimRight(raw, ".,;:)]}")
}

func normalizeTemplateURL(raw string) string {
	return regexp.MustCompile(`\$\{[^}]*\}`).ReplaceAllString(raw, "1")
}

func isAllowedExampleHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "" {
		return false
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, suffix := range []string{".example", ".example.com", ".example.net", ".example.org", ".example.test", ".invalid"} {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func isTestFixture(file string) bool {
	base := path.Base(file)
	return strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.tsx") ||
		strings.HasSuffix(base, ".test.mjs") ||
		strings.Contains(file, "/testdata/") ||
		strings.HasPrefix(file, "web/tests/")
}

func allowsCredentialedURLFixture(file string) bool {
	return isTestFixture(file) || strings.HasPrefix(file, "docs/")
}

func lineColumn(data []byte, offset int) (int, int) {
	line := 1
	column := 1
	for i := 0; i < len(data) && i < offset; i++ {
		if data[i] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}
