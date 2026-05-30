// Command branch_protection_check validates the required-check policy for main.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/openclarion/openclarion/internal/strictjson"
	"go.yaml.in/yaml/v3"
)

const (
	defaultPolicyPath  = "docs/design/ci/branch-protection.required-checks.json"
	defaultWorkflowDir = ".github/workflows"
	policySchema       = "openclarion.branch_protection_required_checks.v1"
)

type config struct {
	PolicyPath  string
	WorkflowDir string
}

type policy struct {
	Schema    string   `json:"schema"`
	Branch    string   `json:"branch"`
	Strict    bool     `json:"strict"`
	SourceApp string   `json:"source_app"`
	Contexts  []string `json:"contexts"`
}

type finding struct {
	Path string
	Msg  string
}

type namedContext struct {
	Path string
	Name string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.PolicyPath, "policy", defaultPolicyPath, "branch protection required checks policy path")
	flag.StringVar(&cfg.WorkflowDir, "workflows", defaultWorkflowDir, "GitHub Actions workflow directory")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[branch-protection] %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	if cfg.PolicyPath == "" {
		cfg.PolicyPath = defaultPolicyPath
	}
	if cfg.WorkflowDir == "" {
		cfg.WorkflowDir = defaultWorkflowDir
	}

	pol, err := readPolicy(cfg.PolicyPath)
	if err != nil {
		return err
	}
	contexts, findings := requiredPRCheckContexts(cfg.WorkflowDir)
	findings = append(findings, validatePolicy(cfg.PolicyPath, pol, contexts)...)
	if len(findings) > 0 {
		sortFindings(findings)
		var lines []string
		for _, finding := range findings {
			lines = append(lines, fmt.Sprintf("%s: %s", finding.Path, finding.Msg))
		}
		return fmt.Errorf("branch protection policy drift:\n%s", strings.Join(lines, "\n"))
	}

	fmt.Fprintf(stdout, "[branch-protection] OK (%d required checks for %s)\n", len(pol.Contexts), pol.Branch)
	return nil
}

func readPolicy(path string) (policy, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return policy{}, err
	}
	var pol policy
	if err := strictjson.Unmarshal(raw, &pol); err != nil {
		return policy{}, fmt.Errorf("%s: %w", path, err)
	}
	return pol, nil
}

func validatePolicy(path string, pol policy, expected []string) []finding {
	var findings []finding
	if pol.Schema != policySchema {
		findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("schema = %q, want %q", pol.Schema, policySchema)})
	}
	if pol.Branch != "main" {
		findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("branch = %q, want main", pol.Branch)})
	}
	if !pol.Strict {
		findings = append(findings, finding{Path: path, Msg: "strict must be true so required checks stay up to date with the protected branch"})
	}
	if pol.SourceApp != "github-actions" {
		findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("source_app = %q, want github-actions", pol.SourceApp)})
	}
	findings = append(findings, validateContexts(path, pol.Contexts, expected)...)
	return findings
}

func validateContexts(path string, got, expected []string) []finding {
	var findings []finding
	if len(got) == 0 {
		findings = append(findings, finding{Path: path, Msg: "contexts must not be empty"})
		return findings
	}
	seen := map[string]struct{}{}
	for i, context := range got {
		if strings.TrimSpace(context) != context || context == "" {
			findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("contexts[%d] must be non-empty and unpadded", i)})
			continue
		}
		if _, ok := seen[context]; ok {
			findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("duplicate required check context %q", context)})
			continue
		}
		seen[context] = struct{}{}
	}
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	if !slices.Equal(got, sorted) {
		findings = append(findings, finding{Path: path, Msg: "contexts must be sorted lexicographically for stable branch-protection review"})
	}

	gotSet := stringSet(got)
	expectedSet := stringSet(expected)
	for _, context := range expected {
		if _, ok := gotSet[context]; !ok {
			findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("missing required check context %q", context)})
		}
	}
	for _, context := range got {
		if _, ok := expectedSet[context]; !ok {
			findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("stale required check context %q", context)})
		}
	}
	return findings
}

func requiredPRCheckContexts(workflowDir string) ([]string, []finding) {
	paths, err := workflowFiles(workflowDir)
	if err != nil {
		return nil, []finding{{Path: workflowDir, Msg: err.Error()}}
	}
	var findings []finding
	var contexts []namedContext
	for _, path := range paths {
		names, workflowFindings := prWorkflowJobNames(path)
		findings = append(findings, workflowFindings...)
		for _, name := range names {
			contexts = append(contexts, namedContext{Path: path, Name: name})
		}
	}
	sort.Slice(contexts, func(i, j int) bool {
		if contexts[i].Name == contexts[j].Name {
			return contexts[i].Path < contexts[j].Path
		}
		return contexts[i].Name < contexts[j].Name
	})

	seen := map[string]string{}
	for _, context := range contexts {
		if previous, ok := seen[context.Name]; ok {
			findings = append(findings, finding{
				Path: context.Path,
				Msg:  fmt.Sprintf("duplicate PR workflow job name %q also used by %s; GitHub required checks need unique job names", context.Name, previous),
			})
			continue
		}
		seen[context.Name] = context.Path
	}
	names := make([]string, 0, len(contexts))
	for _, context := range contexts {
		names = append(names, context.Name)
	}
	return names, findings
}

func workflowFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workflow directory is required")
		}
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			path := filepath.Join(dir, name)
			if _, err := readRegularFile(path); err != nil {
				return nil, err
			}
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no workflow files found")
	}
	sort.Strings(paths)
	return paths, nil
}

func prWorkflowJobNames(path string) ([]string, []finding) {
	raw, err := readRegularFile(path)
	if err != nil {
		return nil, []finding{{Path: path, Msg: err.Error()}}
	}
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return nil, []finding{{Path: path, Msg: fmt.Sprintf("invalid YAML: %v", err)}}
	}
	if err := rejectDuplicateYAMLKeys(&node, path); err != nil {
		return nil, []finding{{Path: path, Msg: err.Error()}}
	}
	root := documentRoot(&node)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, []finding{{Path: path, Msg: "workflow must be a YAML mapping"}}
	}
	if !hasPRTrigger(mappingValue(root, "on")) {
		return nil, nil
	}
	jobs := mappingValue(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return nil, []finding{{Path: path, Msg: "pull_request workflow must define jobs"}}
	}

	var findings []finding
	var names []string
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		jobID := jobs.Content[i].Value
		job := jobs.Content[i+1]
		displayName, err := jobDisplayName(job)
		if err != nil {
			findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("jobs.%s: %v", jobID, err)})
			continue
		}
		names = append(names, displayName)
	}
	return names, findings
}

func jobDisplayName(job *yaml.Node) (string, error) {
	if job == nil || job.Kind != yaml.MappingNode {
		return "", fmt.Errorf("job must be a mapping")
	}
	name := mappingValue(job, "name")
	if name == nil {
		return "", fmt.Errorf("missing name")
	}
	if name.Kind != yaml.ScalarNode || name.Tag != "!!str" {
		return "", fmt.Errorf("name must be a string")
	}
	value := strings.TrimSpace(name.Value)
	if value == "" || value != name.Value {
		return "", fmt.Errorf("name must be non-empty and unpadded")
	}
	return value, nil
}

func hasPRTrigger(on *yaml.Node) bool {
	if on == nil {
		return false
	}
	switch on.Kind {
	case yaml.ScalarNode:
		return isPRTrigger(on.Value)
	case yaml.SequenceNode:
		for _, item := range on.Content {
			if item.Kind == yaml.ScalarNode && isPRTrigger(item.Value) {
				return true
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(on.Content); i += 2 {
			if isPRTrigger(on.Content[i].Value) {
				return true
			}
		}
	}
	return false
}

func isPRTrigger(trigger string) bool {
	return trigger == "pull_request" || trigger == "pull_request_target"
}

func documentRoot(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		return node.Content[0]
	}
	return node
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func rejectDuplicateYAMLKeys(node *yaml.Node, path string) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			if err := rejectDuplicateYAMLKeys(child, path); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]struct{}{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if _, ok := seen[key.Value]; ok {
				return fmt.Errorf("duplicate YAML key %q at line %d", key.Value, key.Line)
			}
			seen[key.Value] = struct{}{}
			if err := rejectDuplicateYAMLKeys(value, path+"."+key.Value); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		for i, child := range node.Content {
			if err := rejectDuplicateYAMLKeys(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func readRegularFile(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", clean)
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("%s must not be empty", clean)
	}
	return os.ReadFile(clean) // #nosec G304 -- repository-owned governance inputs.
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func sortFindings(findings []finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			return findings[i].Msg < findings[j].Msg
		}
		return findings[i].Path < findings[j].Path
	})
}
