// Command comment_debt_check verifies that source comment debt remains
// explicitly tracked.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	debtMarkerRe  = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(TODO|FIXME|HACK|XXX)([^A-Za-z0-9_]|$)`)
	trackingRefRe = regexp.MustCompile(`(?i)(#[0-9]+|https://github\.com/[^[:space:]/]+/[^[:space:]/]+/(issues|pull)/[0-9]+|\bADR-[0-9]{4}\b|docs/adr/ADR-[0-9]{4}[-A-Za-z0-9]*\.md)`)
)

type scanResult struct {
	CheckedFiles int
	Violations   []violation
}

type violation struct {
	Path   string
	Line   int
	Marker string
	Text   string
}

type fileKind int

const (
	fileKindUnsupported fileKind = iota
	fileKindGo
	fileKindHashComment
	fileKindSlashComment
)

func main() {
	if err := run(".", os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[comment-debt] %v\n", err)
		os.Exit(1)
	}
}

func run(root string, stdout io.Writer) error {
	result, err := scanRoot(root)
	if err != nil {
		return err
	}
	if len(result.Violations) > 0 {
		sortViolations(result.Violations)
		fmt.Fprintln(stdout, "[comment-debt] untracked source comment debt:")
		for _, found := range result.Violations {
			fmt.Fprintf(stdout, "  %s:%d: %s comment must reference an issue or ADR: %s\n", found.Path, found.Line, found.Marker, found.Text)
		}
		return fmt.Errorf("found %d untracked comment debt marker(s)", len(result.Violations))
	}

	fmt.Fprintf(stdout, "[comment-debt] OK (%d source files checked)\n", result.CheckedFiles)
	return nil
}

func scanRoot(root string) (scanResult, error) {
	root = filepath.Clean(root)
	result := scanResult{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		kind := classifyFile(path)
		if kind == fileKindUnsupported {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			rel, relErr := relativePath(root, path)
			if relErr != nil {
				return relErr
			}
			return fmt.Errorf("%s: source path must be a regular file", rel)
		}
		violations, checked, err := scanFile(root, path, kind)
		if err != nil {
			return err
		}
		if checked {
			result.CheckedFiles++
		}
		result.Violations = append(result.Violations, violations...)
		return nil
	})
	if err != nil {
		return scanResult{}, err
	}
	sortViolations(result.Violations)
	return result, nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".next", "bin", "build", "coverage", "dist", "node_modules", "out", "testdata":
		return true
	default:
		return strings.HasPrefix(name, ".atlas-drift-tmp")
	}
}

func classifyFile(path string) fileKind {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch {
	case ext == ".go":
		return fileKindGo
	case base == "Makefile" || base == "Dockerfile" || strings.HasSuffix(base, ".mk") || strings.HasSuffix(base, ".Dockerfile"):
		return fileKindHashComment
	case ext == ".sh" || ext == ".bash" || ext == ".zsh" || ext == ".yaml" || ext == ".yml" || ext == ".toml":
		return fileKindHashComment
	case ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" || ext == ".mjs" || ext == ".cjs" || ext == ".css":
		return fileKindSlashComment
	default:
		return fileKindUnsupported
	}
}

func scanFile(root, path string, kind fileKind) ([]violation, bool, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- repository-owned checker input discovered under the requested root.
	if err != nil {
		return nil, false, err
	}
	if hasGeneratedFileMarker(raw) {
		return nil, false, nil
	}
	rel, err := relativePath(root, path)
	if err != nil {
		return nil, false, err
	}
	if kind == fileKindGo {
		violations, err := scanGoComments(rel, path, raw)
		return violations, true, err
	}
	violations, err := scanLineCommentFile(rel, raw, kind)
	return violations, true, err
}

func scanGoComments(relPath, path string, raw []byte) ([]violation, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, raw, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("%s: parse Go comments: %w", relPath, err)
	}

	var violations []violation
	for _, group := range parsed.Comments {
		groupText := rawCommentGroupText(group)
		groupHasRef := trackingRefRe.MatchString(groupText)
		for _, comment := range group.List {
			startLine := fset.Position(comment.Pos()).Line
			violations = append(violations, scanCommentText(relPath, startLine, comment.Text, groupHasRef)...)
		}
	}
	return violations, nil
}

func rawCommentGroupText(group *ast.CommentGroup) string {
	var builder strings.Builder
	for _, comment := range group.List {
		builder.WriteString(comment.Text)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func scanLineCommentFile(relPath string, raw []byte, kind fileKind) ([]violation, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var violations []violation
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !isCommentLine(line, kind) {
			continue
		}
		violations = append(violations, scanCommentText(relPath, lineNumber, line, trackingRefRe.MatchString(line))...)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: scan line comments: %w", relPath, err)
	}
	return violations, nil
}

func isCommentLine(line string, kind fileKind) bool {
	trimmed := strings.TrimSpace(line)
	switch kind {
	case fileKindHashComment:
		return strings.HasPrefix(trimmed, "#")
	case fileKindSlashComment:
		return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*")
	default:
		return false
	}
}

func scanCommentText(path string, startLine int, text string, hasTrackingRef bool) []violation {
	var violations []violation
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		marker, ok := debtMarker(line)
		if !ok {
			continue
		}
		if hasTrackingRef || trackingRefRe.MatchString(line) {
			continue
		}
		violations = append(violations, violation{
			Path:   path,
			Line:   startLine + i,
			Marker: marker,
			Text:   compactText(line),
		})
	}
	return violations
}

func debtMarker(line string) (string, bool) {
	match := debtMarkerRe.FindStringSubmatch(line)
	if len(match) < 3 {
		return "", false
	}
	return strings.ToUpper(match[2]), true
}

func compactText(line string) string {
	line = strings.Join(strings.Fields(line), " ")
	const maxLen = 160
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen] + "..."
}

func hasGeneratedFileMarker(raw []byte) bool {
	prefixLen := len(raw)
	if prefixLen > 4096 {
		prefixLen = 4096
	}
	prefix := raw[:prefixLen]
	return bytes.Contains(prefix, []byte("Code generated")) && bytes.Contains(prefix, []byte("DO NOT EDIT"))
}

func relativePath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func sortViolations(violations []violation) {
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Path != violations[j].Path {
			return violations[i].Path < violations[j].Path
		}
		if violations[i].Line != violations[j].Line {
			return violations[i].Line < violations[j].Line
		}
		return violations[i].Text < violations[j].Text
	})
}
