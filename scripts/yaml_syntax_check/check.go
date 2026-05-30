// Command yaml_syntax_check validates tracked repository YAML files.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const maxYAMLBytes = 2 << 20

type config struct {
	Root string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.Root, "root", ".", "repository root")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[yaml-syntax] %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return err
	}
	files, err := discoverYAMLFiles(root)
	if err != nil {
		return err
	}
	var failures []string
	for _, file := range files {
		if err := validateYAMLFile(root, file); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", file, err))
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return fmt.Errorf("YAML validation failures:\n%s", strings.Join(failures, "\n"))
	}
	fmt.Fprintf(stdout, "[yaml-syntax] OK (%d files checked)\n", len(files))
	return nil
}

func discoverYAMLFiles(root string) ([]string, error) {
	files, err := trackedFiles(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, rel := range files {
		if !isYAMLPath(rel) {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s: YAML files must be regular files, not symlinks", rel)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: YAML files must be regular files", rel)
		}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

func trackedFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z") // #nosec G204 -- fixed git invocation for repository file discovery.
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git ls-files failed: %w\n%s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	parts := bytes.Split(out, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		rel := filepath.ToSlash(string(part))
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			return nil, fmt.Errorf("git returned unsafe path %q", rel)
		}
		files = append(files, rel)
	}
	sort.Strings(files)
	return files, nil
}

func isYAMLPath(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")
}

func validateYAMLFile(root, rel string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path) // #nosec G304 -- path comes from git ls-files under the requested root.
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxYAMLBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxYAMLBytes {
		return fmt.Errorf("file exceeds %d byte limit", maxYAMLBytes)
	}
	doc, err := parseSingleYAMLDocument(raw)
	if err != nil {
		return err
	}
	if doc == nil || len(doc.Content) == 0 && doc.Kind == 0 {
		return nil
	}
	return rejectUnsafeYAMLNodes(doc, "document 1")
}

func parseSingleYAMLDocument(raw []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("multiple YAML documents are not allowed")
	}
	return &doc, nil
}

func rejectUnsafeYAMLNodes(node *yaml.Node, path string) error {
	if node == nil {
		return nil
	}
	if node.Anchor != "" {
		return fmt.Errorf("%s: YAML anchors are not allowed at line %d column %d", path, node.Line, node.Column)
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := rejectUnsafeYAMLNodes(child, path); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := map[string]*yaml.Node{}
		for i := 0; i < len(node.Content); i += 2 {
			if i+1 >= len(node.Content) {
				return fmt.Errorf("%s: malformed mapping node at line %d column %d", path, node.Line, node.Column)
			}
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("%s: mapping key at line %d column %d must be scalar", path, key.Line, key.Column)
			}
			if key.ShortTag() == "!!merge" {
				return fmt.Errorf("%s: YAML merge keys are not allowed at line %d column %d", path, key.Line, key.Column)
			}
			keyID := key.ShortTag() + "\x00" + key.Value
			if first := seen[keyID]; first != nil {
				return fmt.Errorf("%s: duplicate key %q at line %d column %d; first defined at line %d column %d", path, key.Value, key.Line, key.Column, first.Line, first.Column)
			}
			seen[keyID] = key
			nextPath := fmt.Sprintf("%s.%s", path, key.Value)
			if err := rejectUnsafeYAMLNodes(value, nextPath); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			if err := rejectUnsafeYAMLNodes(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		return fmt.Errorf("%s: YAML aliases are not allowed at line %d column %d", path, node.Line, node.Column)
	default:
		for _, child := range node.Content {
			if err := rejectUnsafeYAMLNodes(child, path); err != nil {
				return err
			}
		}
	}
	return nil
}
