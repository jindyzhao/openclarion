// Package changedfiles parses git changed-file output for PR-only gates.
package changedfiles

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SplitNameOnlyOutput parses newline-delimited git diff --name-only output.
func SplitNameOnlyOutput(out string) ([]string, error) {
	var files []string
	lines := strings.Split(out, "\n")
	for lineNo, line := range lines {
		if line == "" {
			if lineNo == len(lines)-1 {
				continue
			}
			return nil, fmt.Errorf("line %d: changed file path must not be empty", lineNo+1)
		}
		file, err := Normalize(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
		}
		files = append(files, file)
	}
	return files, nil
}

// Normalize validates and returns one repository-relative slash-separated path.
func Normalize(file string) (string, error) {
	if strings.TrimSpace(file) != file {
		return "", fmt.Errorf("changed file path %q must not be whitespace padded", file)
	}
	if file == "" {
		return "", fmt.Errorf("changed file path must not be empty")
	}
	if !utf8.ValidString(file) {
		return "", fmt.Errorf("changed file path must be valid UTF-8")
	}
	for _, r := range file {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("changed file path %q contains control characters", file)
		}
	}
	if strings.Contains(file, "\\") {
		return "", fmt.Errorf("changed file path %q must use slash separators", file)
	}
	if strings.HasPrefix(file, `"`) || strings.HasSuffix(file, `"`) {
		return "", fmt.Errorf("changed file path %q must not be git-quoted", file)
	}
	if strings.Contains(file, "://") || looksLikeWindowsDrivePath(file) {
		return "", fmt.Errorf("changed file path %q must be repository-relative", file)
	}
	if path.IsAbs(file) {
		return "", fmt.Errorf("changed file path %q must be repository-relative", file)
	}
	if file == "." || !fs.ValidPath(file) {
		return "", fmt.Errorf("changed file path %q must be a normalized repository-relative slash path", file)
	}
	return file, nil
}

func looksLikeWindowsDrivePath(file string) bool {
	if len(file) < 3 || file[1] != ':' || file[2] != '/' {
		return false
	}
	drive := file[0]
	return ('a' <= drive && drive <= 'z') || ('A' <= drive && drive <= 'Z')
}
