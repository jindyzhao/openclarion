// Command go_toolchain_check keeps first-party Go version declarations aligned.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go/version"

	"go.yaml.in/yaml/v3"
)

const (
	rootGoModPath      = "go.mod"
	golangCIConfigPath = ".golangci.yml"
	workflowDir        = ".github/workflows"
)

type finding struct {
	Path string
	Msg  string
}

type moduleVersion struct {
	Path    string
	Version string
}

type golangCIConfig struct {
	Run struct {
		Go string `yaml:"go"`
	} `yaml:"run"`
}

type workflowFile struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Steps []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Uses string         `yaml:"uses"`
	With map[string]any `yaml:"with"`
}

func main() {
	if err := run(".", os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[go-toolchain-check] %v\n", err)
		os.Exit(1)
	}
}

func run(root string, out io.Writer) error {
	modules, findings := checkGoModules(root)
	if len(modules) == 0 {
		return fmt.Errorf("no go.mod files found")
	}
	rootVersion := modules[0].Version
	languageVersion, err := languageVersion(rootVersion)
	if err != nil {
		findings = append(findings, finding{Path: rootGoModPath, Msg: err.Error()})
	}
	findings = append(findings, checkGolangCI(root, languageVersion)...)
	setupGoSteps, workflowFindings := checkWorkflows(root)
	findings = append(findings, workflowFindings...)

	if len(findings) > 0 {
		sortFindings(findings)
		fmt.Fprintln(out, "[go-toolchain-check] Go toolchain version drift:")
		for _, f := range findings {
			fmt.Fprintf(out, "  %s: %s\n", f.Path, f.Msg)
		}
		return fmt.Errorf("found %d drift issue(s)", len(findings))
	}

	fmt.Fprintf(out, "[go-toolchain-check] OK (%d go.mod files, %d setup-go steps)\n", len(modules), setupGoSteps)
	return nil
}

func checkGoModules(root string) ([]moduleVersion, []finding) {
	paths, err := findGoModFiles(root)
	if err != nil {
		return nil, []finding{{Path: root, Msg: err.Error()}}
	}
	if len(paths) == 0 {
		return nil, nil
	}

	modules := make([]moduleVersion, 0, len(paths))
	var findings []finding
	var rootVersion string
	for _, path := range paths {
		versionValue, err := readGoDirective(filepath.Join(root, path))
		if err != nil {
			findings = append(findings, finding{Path: path, Msg: err.Error()})
			continue
		}
		toolchainVersion := "go" + versionValue
		if !version.IsValid(toolchainVersion) {
			findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("go directive %q is not a valid Go version", versionValue)})
			continue
		}
		if !strings.Contains(versionValue, ".") || strings.Count(versionValue, ".") < 2 {
			findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("go directive %q must include an explicit patch version for reproducible setup-go installs", versionValue)})
		}
		modules = append(modules, moduleVersion{Path: path, Version: versionValue})
		if path == rootGoModPath {
			rootVersion = versionValue
		}
	}
	if rootVersion == "" {
		findings = append(findings, finding{Path: rootGoModPath, Msg: "root go.mod is required as the Go toolchain source of truth"})
		return modules, findings
	}
	for _, module := range modules {
		if module.Path == rootGoModPath {
			continue
		}
		if module.Version != rootVersion {
			findings = append(findings, finding{
				Path: module.Path,
				Msg:  fmt.Sprintf("go directive %q must match root go.mod %q", module.Version, rootVersion),
			})
		}
	}
	return modules, findings
}

func findGoModFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() && shouldSkipDir(name) && path != root {
			return filepath.SkipDir
		}
		if name == "go.mod" {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(paths, func(i, j int) bool {
		if paths[i] == rootGoModPath {
			return true
		}
		if paths[j] == rootGoModPath {
			return false
		}
		return paths[i] < paths[j]
	})
	return paths, nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "bin", "node_modules":
		return true
	default:
		return strings.HasPrefix(name, ".atlas-drift-tmp")
	}
}

func readGoDirective(path string) (string, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(stripLineComment(line))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("missing go directive")
}

func stripLineComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func languageVersion(goDirective string) (string, error) {
	if goDirective == "" {
		return "", fmt.Errorf("cannot derive language version from empty go directive")
	}
	toolchainVersion := "go" + goDirective
	if !version.IsValid(toolchainVersion) {
		return "", fmt.Errorf("go directive %q is not a valid Go version", goDirective)
	}
	return strings.TrimPrefix(version.Lang(toolchainVersion), "go"), nil
}

func checkGolangCI(root, expectedLanguageVersion string) []finding {
	path := filepath.Join(root, golangCIConfigPath)
	raw, err := readRegularFile(path)
	if err != nil {
		return []finding{{Path: golangCIConfigPath, Msg: err.Error()}}
	}
	var cfg golangCIConfig
	if err := decodeCheckedYAML(raw, &cfg); err != nil {
		return []finding{{Path: golangCIConfigPath, Msg: fmt.Sprintf("invalid YAML: %v", err)}}
	}
	actual := strings.TrimSpace(cfg.Run.Go)
	if actual == "" {
		return []finding{{Path: golangCIConfigPath, Msg: "run.go must be set to the root Go language version"}}
	}
	if expectedLanguageVersion != "" && actual != expectedLanguageVersion {
		return []finding{{Path: golangCIConfigPath, Msg: fmt.Sprintf("run.go %q must match root Go language version %q", actual, expectedLanguageVersion)}}
	}
	if strings.Count(actual, ".") != 1 {
		return []finding{{Path: golangCIConfigPath, Msg: fmt.Sprintf("run.go %q must be a major.minor language version, not a patch/toolchain version", actual)}}
	}
	return nil
}

func checkWorkflows(root string) (int, []finding) {
	paths, err := workflowFiles(root)
	if err != nil {
		return 0, []finding{{Path: workflowDir, Msg: err.Error()}}
	}
	var findings []finding
	setupGoSteps := 0
	for _, path := range paths {
		count, pathFindings := checkWorkflowFile(filepath.Join(root, path), path)
		setupGoSteps += count
		findings = append(findings, pathFindings...)
	}
	return setupGoSteps, findings
}

func workflowFiles(root string) ([]string, error) {
	dir := filepath.Join(root, workflowDir)
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must be a directory, not a symlink", workflowDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s must be a directory", workflowDir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			path := filepath.ToSlash(filepath.Join(workflowDir, name))
			if entry.Type()&fs.ModeSymlink != 0 {
				return nil, fmt.Errorf("%s must be a regular file, not a symlink", path)
			}
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("%s must be a regular file", path)
			}
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func checkWorkflowFile(absPath, displayPath string) (int, []finding) {
	raw, err := readRegularFile(absPath)
	if err != nil {
		return 0, []finding{{Path: displayPath, Msg: err.Error()}}
	}
	var wf workflowFile
	if err := decodeCheckedYAML(raw, &wf); err != nil {
		return 0, []finding{{Path: displayPath, Msg: fmt.Sprintf("invalid YAML: %v", err)}}
	}
	var findings []finding
	setupGoSteps := 0
	for jobName, job := range wf.Jobs {
		for stepIndex, step := range job.Steps {
			if !isSetupGoStep(step.Uses) {
				continue
			}
			setupGoSteps++
			stepPath := fmt.Sprintf("%s jobs.%s.steps[%d]", displayPath, jobName, stepIndex)
			if _, exists := step.With["go-version"]; exists {
				findings = append(findings, finding{Path: stepPath, Msg: "actions/setup-go must not use hard-coded go-version; use go-version-file: go.mod"})
			}
			value, ok := step.With["go-version-file"]
			if !ok {
				findings = append(findings, finding{Path: stepPath, Msg: "actions/setup-go must set go-version-file: go.mod"})
				continue
			}
			versionFile, ok := value.(string)
			if !ok || strings.TrimSpace(versionFile) != rootGoModPath {
				findings = append(findings, finding{Path: stepPath, Msg: fmt.Sprintf("actions/setup-go go-version-file must be %q", rootGoModPath)})
			}
		}
	}
	return setupGoSteps, findings
}

func decodeCheckedYAML(raw []byte, out any) error {
	doc, err := parseSingleYAMLDocument(raw)
	if err != nil {
		return err
	}
	if err := rejectDuplicateYAMLKeys(doc); err != nil {
		return err
	}

	return doc.Decode(out)
}

func parseSingleYAMLDocument(raw []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("multiple YAML documents are not allowed")
	}
	return &doc, nil
}

func rejectDuplicateYAMLKeys(node *yaml.Node) error {
	if node.Anchor != "" {
		return fmt.Errorf("YAML anchors are not allowed at line %d", node.Line)
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := rejectDuplicateYAMLKeys(child); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := rejectDuplicateYAMLKeys(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := map[string]*yaml.Node{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("mapping key at line %d must be scalar", key.Line)
			}
			if key.ShortTag() == "!!merge" {
				return fmt.Errorf("YAML merge keys are not allowed at line %d", key.Line)
			}
			keyID := key.ShortTag() + "\x00" + key.Value
			if previous, exists := seen[keyID]; exists {
				return fmt.Errorf("duplicate YAML key %q at line %d; first declared at line %d", key.Value, key.Line, previous.Line)
			}
			seen[keyID] = key
			if err := rejectDuplicateYAMLKeys(value); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		return fmt.Errorf("YAML aliases are not allowed at line %d", node.Line)
	}
	return nil
}

func isSetupGoStep(uses string) bool {
	return strings.HasPrefix(strings.TrimSpace(uses), "actions/setup-go@")
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path) // #nosec G304 -- repository-owned checker input.
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("must be a regular file, not a symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("must be a regular file")
	}
	return os.ReadFile(path) // #nosec G304 -- repository-owned checker input.
}

func sortFindings(findings []finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			return findings[i].Msg < findings[j].Msg
		}
		return findings[i].Path < findings[j].Path
	})
}
