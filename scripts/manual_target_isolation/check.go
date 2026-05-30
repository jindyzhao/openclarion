// Command manual_target_isolation validates that manual smoke and evidence
// targets stay out of automated CI entry points.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	defaultMakefilePath = "Makefile"
	defaultPolicyPath   = "docs/design/ci/manual-targets.tsv"
	defaultCIReadmePath = "docs/design/ci/README.md"
	defaultWorkflowsDir = ".github/workflows"
)

var targetNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
var workflowRunMakeRE = regexp.MustCompile(`(?m)^[ \t]*(?:-[ \t]+)?run:[ \t]+['"]?make[ \t]+([A-Za-z][A-Za-z0-9_.-]*)['"]?[ \t]*(?:#.*)?$`)

type config struct {
	MakefilePath string
	PolicyPath   string
	CIReadmePath string
	WorkflowsDir string
}

type targetInfo struct {
	Name        string
	Deps        []string
	Description string
}

type policyEntry struct {
	Target string
	Reason string
}

type workflowRun struct {
	Path   string
	Target string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.MakefilePath, "makefile", defaultMakefilePath, "Makefile path")
	flag.StringVar(&cfg.PolicyPath, "policy", defaultPolicyPath, "manual target policy TSV path")
	flag.StringVar(&cfg.CIReadmePath, "ci-readme", defaultCIReadmePath, "CI governance README path")
	flag.StringVar(&cfg.WorkflowsDir, "workflows", defaultWorkflowsDir, "GitHub Actions workflows directory")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[manual-target-isolation] %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	targets, err := readMakefileTargets(cfg.MakefilePath)
	if err != nil {
		return err
	}
	entries, err := readPolicy(cfg.PolicyPath)
	if err != nil {
		return err
	}
	ciReadme, err := os.ReadFile(cfg.CIReadmePath) // #nosec G304 -- repository-owned checker input.
	if err != nil {
		return err
	}
	workflowRuns, err := readWorkflowRuns(cfg.WorkflowsDir)
	if err != nil {
		return err
	}

	manual := map[string]policyEntry{}
	var problems []string
	for _, entry := range entries {
		if _, exists := manual[entry.Target]; exists {
			problems = append(problems, fmt.Sprintf("%s: duplicate policy row for %q", cfg.PolicyPath, entry.Target))
			continue
		}
		manual[entry.Target] = entry
		info, ok := targets[entry.Target]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: manual target %q is not declared in %s", cfg.PolicyPath, entry.Target, cfg.MakefilePath))
			continue
		}
		if !manualDescription(info.Description) {
			problems = append(problems, fmt.Sprintf("%s: target %q help text must start with Manual", cfg.MakefilePath, entry.Target))
		}
		if !documentedMakeTarget(string(ciReadme), entry.Target) {
			problems = append(problems, fmt.Sprintf("%s: manual target %q must be documented as `make %s`", cfg.CIReadmePath, entry.Target, entry.Target))
		}
	}
	for _, info := range targets {
		if manualDescription(info.Description) {
			if _, ok := manual[info.Name]; !ok {
				problems = append(problems, fmt.Sprintf("%s: target %q is marked Manual but is missing from %s", cfg.MakefilePath, info.Name, cfg.PolicyPath))
			}
		}
	}

	for _, manualTarget := range reachableManualTargets("ci", targets, manual) {
		problems = append(problems, fmt.Sprintf("%s: make ci must not reach manual target %q", cfg.MakefilePath, manualTarget))
	}
	for _, run := range workflowRuns {
		for _, manualTarget := range reachableManualTargets(run.Target, targets, manual) {
			problems = append(problems, fmt.Sprintf("%s: workflow run `make %s` must not reach manual target %q", run.Path, run.Target, manualTarget))
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "\n"))
	}
	fmt.Fprintf(stdout, "[manual-target-isolation] OK (%d manual targets isolated; %d workflow make runs checked)\n", len(manual), len(workflowRuns))
	return nil
}

func readMakefileTargets(path string) (map[string]targetInfo, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- repository-owned checker input.
	if err != nil {
		return nil, err
	}
	targets := map[string]targetInfo{}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, ".") || strings.HasPrefix(line, "\t") || strings.TrimSpace(line) == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if !targetNameRE.MatchString(name) {
			continue
		}
		rest := line[colon+1:]
		depText := rest
		description := ""
		if marker := strings.Index(rest, "##"); marker >= 0 {
			depText = rest[:marker]
			description = strings.TrimSpace(rest[marker+2:])
		} else if marker := strings.Index(rest, "#"); marker >= 0 {
			depText = rest[:marker]
		}
		targets[name] = targetInfo{Name: name, Deps: parseDeps(depText), Description: description}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%s: no make targets found", path)
	}
	return targets, nil
}

func parseDeps(depText string) []string {
	var deps []string
	seen := map[string]struct{}{}
	for _, field := range strings.Fields(depText) {
		if !targetNameRE.MatchString(field) {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		deps = append(deps, field)
	}
	return deps
}

func readPolicy(path string) ([]policyEntry, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- repository-owned checker input.
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "target\treason" {
		return nil, fmt.Errorf("%s: first row must be exactly %q", path, "target\treason")
	}
	var entries []policyEntry
	for i, line := range lines[1:] {
		lineNo := i + 2
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s:%d: policy row must have exactly two tab-separated fields", path, lineNo)
		}
		target := strings.TrimSpace(parts[0])
		reason := strings.TrimSpace(parts[1])
		if target != parts[0] || reason != parts[1] {
			return nil, fmt.Errorf("%s:%d: policy fields must not have leading or trailing whitespace", path, lineNo)
		}
		if !targetNameRE.MatchString(target) {
			return nil, fmt.Errorf("%s:%d: invalid target name %q", path, lineNo, target)
		}
		if reason == "" {
			return nil, fmt.Errorf("%s:%d: reason must not be empty", path, lineNo)
		}
		entries = append(entries, policyEntry{Target: target, Reason: reason})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s: no manual targets registered", path)
	}
	return entries, nil
}

func readWorkflowRuns(workflowsDir string) ([]workflowRun, error) {
	var runs []workflowRun
	entries, err := os.ReadDir(workflowsDir)
	if errors.Is(err, os.ErrNotExist) {
		return runs, nil
	} else if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		path := filepath.Join(workflowsDir, name)
		raw, err := os.ReadFile(path) // #nosec G304 -- file found under repository workflow directory.
		if err != nil {
			return nil, err
		}
		rel := filepath.ToSlash(path)
		for _, match := range workflowRunMakeRE.FindAllStringSubmatch(string(raw), -1) {
			runs = append(runs, workflowRun{Path: rel, Target: match[1]})
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Path == runs[j].Path {
			return runs[i].Target < runs[j].Target
		}
		return runs[i].Path < runs[j].Path
	})
	return runs, nil
}

func reachableManualTargets(start string, targets map[string]targetInfo, manual map[string]policyEntry) []string {
	found := map[string]struct{}{}
	visited := map[string]struct{}{}
	var visit func(string)
	visit = func(name string) {
		if _, ok := visited[name]; ok {
			return
		}
		visited[name] = struct{}{}
		if _, ok := manual[name]; ok {
			found[name] = struct{}{}
		}
		info, ok := targets[name]
		if !ok {
			return
		}
		for _, dep := range info.Deps {
			visit(dep)
		}
	}
	visit(start)

	out := make([]string, 0, len(found))
	for name := range found {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func manualDescription(description string) bool {
	lower := strings.ToLower(strings.TrimSpace(description))
	return strings.HasPrefix(lower, "manual ") || strings.HasPrefix(lower, "manual:")
}

func documentedMakeTarget(markdown, target string) bool {
	pattern := `(?:^|[^A-Za-z0-9_.-])make[ \t]+` + regexp.QuoteMeta(target) + `(?:[^A-Za-z0-9_.-]|$)`
	return regexp.MustCompile(pattern).FindStringIndex(markdown) != nil
}
