// Command docs_metadata_check validates freshness metadata in governed docs.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	lastUpdatedRe = regexp.MustCompile(`^> Last updated: ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
	dateHeaderRe  = regexp.MustCompile(`^\|\s*Date\s*\|`)
	tableDateRe   = regexp.MustCompile(`^\|\s*([0-9]{4}-[0-9]{2}-[0-9]{2})\s*\|`)
)

var defaultRoots = []string{
	"README.md",
	"DEVELOPMENT_WORKFLOW.md",
	"CONTRIBUTING.md",
	"GOVERNANCE.md",
	"SECURITY.md",
	"CODE_OF_CONDUCT.md",
	"DCO.md",
	"MAINTAINERS.md",
	"docs",
}

func main() {
	roots := defaultRoots
	if len(os.Args) > 1 {
		roots = os.Args[1:]
	}
	result, err := Check(roots)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[docs-metadata] %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "[docs-metadata] OK (%d files checked, %d files with Last updated)\n", result.Files, result.MetadataFiles)
}

// Result records the checked Markdown file counts.
type Result struct {
	Files         int
	MetadataFiles int
}

// Check validates Last updated metadata for Markdown files under roots.
func Check(roots []string) (Result, error) {
	files, err := markdownFiles(roots)
	if err != nil {
		return Result{}, err
	}

	var findings []string
	metadataFiles := 0
	for _, file := range files {
		hasMetadata, err := checkFile(file)
		if err != nil {
			findings = append(findings, err.Error())
			continue
		}
		if hasMetadata {
			metadataFiles++
		}
	}
	if len(findings) > 0 {
		return Result{}, errors.New(strings.Join(findings, "\n"))
	}
	return Result{Files: len(files), MetadataFiles: metadataFiles}, nil
}

func markdownFiles(roots []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, root := range roots {
		clean := filepath.Clean(root)
		info, err := os.Lstat(clean)
		if err != nil {
			if os.IsNotExist(err) && isOptionalRoot(clean) {
				continue
			}
			return nil, fmt.Errorf("%s: %w", clean, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s: markdown metadata root must not be a symlink", clean)
		}
		if info.IsDir() {
			if err := filepath.WalkDir(clean, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("%s: markdown metadata input must not be a symlink", path)
				}
				if entry.IsDir() || filepath.Ext(path) != ".md" {
					return nil
				}
				entryInfo, err := entry.Info()
				if err != nil {
					return err
				}
				if !entryInfo.Mode().IsRegular() {
					return fmt.Errorf("%s: markdown metadata input must be a regular file", path)
				}
				addFile(&files, seen, path)
				return nil
			}); err != nil {
				return nil, err
			}
			continue
		}
		if filepath.Ext(clean) != ".md" {
			continue
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: markdown metadata input must be a regular file", clean)
		}
		addFile(&files, seen, clean)
	}
	sort.Strings(files)
	return files, nil
}

func addFile(files *[]string, seen map[string]bool, path string) {
	slashPath := filepath.ToSlash(filepath.Clean(path))
	if seen[slashPath] {
		return
	}
	seen[slashPath] = true
	*files = append(*files, slashPath)
}

func isOptionalRoot(path string) bool {
	for _, root := range defaultRoots {
		if filepath.Clean(root) == path && root != "docs" {
			return true
		}
	}
	return false
}

func checkFile(path string) (bool, error) {
	raw, err := os.ReadFile(path) // #nosec G304,G703 -- path comes from governed markdown roots after symlink/regular-file checks.
	if err != nil {
		return false, fmt.Errorf("%s: read markdown metadata input: %w", path, err)
	}

	var lastUpdated *time.Time
	var latestTableDate *time.Time
	inDateTable := false
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if match := lastUpdatedRe.FindStringSubmatch(line); match != nil {
			if lastUpdated != nil {
				return false, fmt.Errorf("%s:%d: duplicate Last updated metadata", path, lineNo)
			}
			parsed, err := parseDate(match[1])
			if err != nil {
				return false, fmt.Errorf("%s:%d: invalid Last updated date %q", path, lineNo, match[1])
			}
			lastUpdated = &parsed
			continue
		}
		if !strings.HasPrefix(line, "|") {
			inDateTable = false
			continue
		}
		if dateHeaderRe.MatchString(line) {
			inDateTable = true
			continue
		}
		if !inDateTable {
			continue
		}
		match := tableDateRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		parsed, err := parseDate(match[1])
		if err != nil {
			return false, fmt.Errorf("%s:%d: invalid dated table row %q", path, lineNo, match[1])
		}
		if latestTableDate == nil || parsed.After(*latestTableDate) {
			latestTableDate = &parsed
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("%s: scan markdown metadata input: %w", path, err)
	}
	if lastUpdated == nil {
		return false, nil
	}
	if latestTableDate != nil && lastUpdated.Before(*latestTableDate) {
		return true, fmt.Errorf("%s: Last updated %s is older than latest dated table row %s", path, formatDate(*lastUpdated), formatDate(*latestTableDate))
	}
	return true, nil
}

func parseDate(value string) (time.Time, error) {
	return time.Parse("2006-01-02", value)
}

func formatDate(value time.Time) string {
	return value.Format("2006-01-02")
}
