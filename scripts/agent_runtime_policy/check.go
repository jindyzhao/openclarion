// Command agent_runtime_policy rejects control-plane agent runtime bindings
// before the M4 runtime selection gate accepts a baseline.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

const (
	defaultPolicyFile = "docs/design/ci/agent-runtime-forbidden.tsv"
)

var dependencyFields = []string{
	"dependencies",
	"devDependencies",
	"optionalDependencies",
	"peerDependencies",
}

type policy struct {
	manifestPatterns []string
	codePatterns     []string
}

type finding struct {
	Path string
	Msg  string
}

type packageDependency struct {
	Field string
	Name  string
}

func main() {
	if err := run("."); err != nil {
		fmt.Fprintf(os.Stderr, "[forbidden-agent-runtime] %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[forbidden-agent-runtime] OK")
}

func run(root string) error {
	policyPath := os.Getenv("OPENCLARION_AGENT_RUNTIME_POLICY_FILE")
	if policyPath == "" {
		policyPath = defaultPolicyFile
	}
	if !filepath.IsAbs(policyPath) {
		policyPath = filepath.Join(root, policyPath)
	}
	pol, err := readPolicy(policyPath)
	if err != nil {
		return err
	}

	var findings []finding
	manifestPaths, err := findFiles(root, func(_ string, entry fs.DirEntry) bool {
		return entry.Name() == "go.mod" || entry.Name() == "package.json"
	})
	if err != nil {
		return err
	}
	for _, path := range manifestPaths {
		rel := cleanRel(root, path)
		manifestFindings, err := checkManifest(path, rel, pol.manifestPatterns)
		if err != nil {
			findings = append(findings, finding{Path: rel, Msg: err.Error()})
			continue
		}
		findings = append(findings, manifestFindings...)
	}

	codeRoots := existingRoots(root, "cmd", "internal", "scripts")
	if len(codeRoots) > 0 {
		codePaths, err := findGoControlPlaneFiles(root, codeRoots)
		if err != nil {
			return err
		}
		for _, path := range codePaths {
			rel := cleanRel(root, path)
			codeFindings, err := checkCodeFile(path, rel, pol.codePatterns)
			if err != nil {
				findings = append(findings, finding{Path: rel, Msg: err.Error()})
				continue
			}
			findings = append(findings, codeFindings...)
		}
	}

	if len(findings) > 0 {
		sort.Slice(findings, func(i, j int) bool {
			if findings[i].Path == findings[j].Path {
				return findings[i].Msg < findings[j].Msg
			}
			return findings[i].Path < findings[j].Path
		})
		var b strings.Builder
		b.WriteString("violations:\n")
		for _, f := range findings {
			fmt.Fprintf(&b, "  %s: %s\n", f.Path, f.Msg)
		}
		b.WriteString("\nJustification: docs/design/agent-runtime-selection.md keeps agent-runtime dependencies and runtime-specific logic inside candidate sandbox images until M4 proves the runtime baseline.\n")
		b.WriteString("Fix: remove the control-plane dependency or hard-coded runtime name, keep candidate names in evidence/docs/sandbox images, or update the runtime selection gate and policy file in the same change.")
		return errors.New(strings.TrimRight(b.String(), "\n"))
	}
	return nil
}

func readPolicy(path string) (policy, error) {
	// #nosec G304,G703 -- CI policy file path is repository-owned or explicitly supplied by the caller for gate tests.
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return policy{}, fmt.Errorf("missing policy file: %s", cleanPath(path))
		}
		return policy{}, fmt.Errorf("read policy file %s: %w", cleanPath(path), err)
	}

	var pol policy
	seen := make(map[string]struct{})
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 || parts[1] == "" {
			return policy{}, fmt.Errorf("invalid policy row %s:%d; expected '<manifest|code><TAB><pattern>'", cleanPath(path), i+1)
		}
		scope, pattern := parts[0], parts[1]
		if hasEdgeSpace(scope) || hasEdgeSpace(pattern) {
			return policy{}, fmt.Errorf("invalid policy row %s:%d; scope and pattern must not contain leading or trailing whitespace", cleanPath(path), i+1)
		}
		key := scope + "\t" + pattern
		if _, ok := seen[key]; ok {
			return policy{}, fmt.Errorf("duplicate policy row %s:%d: %s", cleanPath(path), i+1, key)
		}
		seen[key] = struct{}{}
		switch scope {
		case "manifest":
			pol.manifestPatterns = append(pol.manifestPatterns, pattern)
		case "code":
			pol.codePatterns = append(pol.codePatterns, pattern)
		default:
			return policy{}, fmt.Errorf("invalid policy scope %s:%d: %s", cleanPath(path), i+1, scope)
		}
	}
	if len(pol.manifestPatterns) == 0 || len(pol.codePatterns) == 0 {
		return policy{}, fmt.Errorf("policy file must define at least one manifest and one code pattern: %s", cleanPath(path))
	}
	return pol, nil
}

func checkManifest(path, rel string, patterns []string) ([]finding, error) {
	switch filepath.Base(path) {
	case "package.json":
		deps, err := parsePackageJSONDependencies(path)
		if err != nil {
			return nil, err
		}
		return checkPackageDependencies(rel, deps, patterns), nil
	case "go.mod":
		refs, err := parseGoModRefs(path)
		if err != nil {
			return nil, err
		}
		return checkGoModRefs(rel, refs, patterns), nil
	default:
		return nil, nil
	}
}

func parsePackageJSONDependencies(path string) ([]packageDependency, error) {
	// #nosec G304 -- this gate reads first-party package.json manifests found inside the repository tree.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read package.json: %w", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode package.json: %w", err)
	}
	var deps []packageDependency
	for _, field := range dependencyFields {
		rawField, ok := doc[field]
		if !ok || string(rawField) == "null" {
			continue
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal(rawField, &values); err != nil {
			return nil, fmt.Errorf("%s must be a JSON object", field)
		}
		for name := range values {
			deps = append(deps, packageDependency{Field: field, Name: name})
		}
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Field == deps[j].Field {
			return deps[i].Name < deps[j].Name
		}
		return deps[i].Field < deps[j].Field
	})
	return deps, nil
}

func parseGoModRefs(path string) ([]packageDependency, error) {
	// #nosec G304 -- this gate reads first-party go.mod manifests found inside the repository tree.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read go.mod: %w", err)
	}
	file, err := modfile.Parse(cleanPath(path), raw, nil)
	if err != nil {
		return nil, fmt.Errorf("parse go.mod: %w", err)
	}
	refs := make([]packageDependency, 0, len(file.Require)+len(file.Tool))
	for _, req := range file.Require {
		refs = append(refs, packageDependency{Field: "require", Name: req.Mod.Path})
	}
	for _, tool := range file.Tool {
		refs = append(refs, packageDependency{Field: "tool", Name: tool.Path})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Field == refs[j].Field {
			return refs[i].Name < refs[j].Name
		}
		return refs[i].Field < refs[j].Field
	})
	return refs, nil
}

func checkPackageDependencies(path string, deps []packageDependency, patterns []string) []finding {
	var findings []finding
	for _, dep := range deps {
		for _, pattern := range patterns {
			if manifestPatternMatches(dep.Name, pattern) {
				findings = append(findings, finding{
					Path: path,
					Msg:  fmt.Sprintf("%s dependency %q must not add agent runtime dependency '%s' before the runtime selection gate accepts a baseline", dep.Field, dep.Name, pattern),
				})
			}
		}
	}
	return findings
}

func checkGoModRefs(path string, refs []packageDependency, patterns []string) []finding {
	var findings []finding
	for _, ref := range refs {
		for _, pattern := range patterns {
			if manifestPatternMatches(ref.Name, pattern) {
				findings = append(findings, finding{
					Path: path,
					Msg:  fmt.Sprintf("%s path %q must not add agent runtime dependency '%s' before the runtime selection gate accepts a baseline", ref.Field, ref.Name, pattern),
				})
			}
		}
	}
	return findings
}

func manifestPatternMatches(name, pattern string) bool {
	if strings.HasPrefix(pattern, `"`) && strings.HasSuffix(pattern, `"`) && len(pattern) >= 2 {
		return name == strings.Trim(pattern, `"`)
	}
	if strings.HasPrefix(pattern, "`") && strings.HasSuffix(pattern, "`") && len(pattern) >= 2 {
		return name == strings.Trim(pattern, "`")
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(pattern))
}

func checkCodeFile(path, rel string, patterns []string) ([]finding, error) {
	// #nosec G304 -- this gate reads first-party Go source files found inside the repository tree.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Go source: %w", err)
	}
	lines := strings.Split(string(raw), "\n")
	var findings []finding
	for i, line := range lines {
		for _, pattern := range patterns {
			if strings.Contains(strings.ToLower(line), strings.ToLower(pattern)) {
				findings = append(findings, finding{
					Path: rel,
					Msg:  fmt.Sprintf("line %d must not hard-code agent runtime family '%s' in first-party Go control-plane code before the runtime selection gate accepts a baseline", i+1, pattern),
				})
			}
		}
	}
	return findings, nil
}

func findFiles(root string, match func(path string, entry fs.DirEntry) bool) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if match(path, entry) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func findGoControlPlaneFiles(root string, roots []string) ([]string, error) {
	var paths []string
	for _, scanRoot := range roots {
		err := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if shouldSkipDir(entry.Name()) && path != scanRoot {
					return filepath.SkipDir
				}
				rel := cleanRel(root, path)
				if rel == "internal/persistence/ent" {
					return filepath.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func existingRoots(root string, names ...string) []string {
	var roots []string
	for _, name := range names {
		path := filepath.Join(root, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			roots = append(roots, path)
		}
	}
	return roots
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func cleanRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(rel)
}

func cleanPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func hasEdgeSpace(value string) bool {
	return value != strings.TrimSpace(value)
}
