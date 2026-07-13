// Command agent_runtime_policy_check keeps agent frameworks inside the accepted
// sandbox runtime and out of OpenClarion's control plane.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const defaultPolicyPath = "docs/design/ci/agent-runtime-forbidden.tsv"

const acceptedSandboxRuntimeRoot = "scripts/diagnosis_assistant_runner"

var dependencySections = []string{
	"dependencies",
	"devDependencies",
	"optionalDependencies",
	"peerDependencies",
}

var textSourceExtensions = map[string]struct{}{
	".bash": {},
	".cjs":  {},
	".js":   {},
	".jsx":  {},
	".mjs":  {},
	".sh":   {},
	".ts":   {},
	".tsx":  {},
}

type config struct {
	Root       string
	PolicyPath string
}

type policy struct {
	Manifest     []policyPattern
	Code         []policyPattern
	SandboxAllow []policyPattern
}

type policyPattern struct {
	Raw   string
	Match string
	Exact bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "[forbidden-agent-runtime] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	pol, err := readPolicy(cfg)
	if err != nil {
		return err
	}
	manifestFindings, err := scanManifests(cfg.Root, pol.Manifest, pol.SandboxAllow)
	if err != nil {
		return err
	}
	codeFindings, err := scanGoControlPlane(cfg.Root, pol.Code, pol.SandboxAllow)
	if err != nil {
		return err
	}
	textFindings, err := scanTextControlPlane(cfg.Root, pol.Code, pol.SandboxAllow)
	if err != nil {
		return err
	}
	for _, finding := range manifestFindings {
		fmt.Fprintf(stderr, "[forbidden-agent-runtime] %s dependency %q matched policy pattern %q\n", finding.Path, finding.Value, finding.Pattern.Raw)
		fmt.Fprintf(stderr, "[forbidden-agent-runtime] %s must not add agent runtime dependency '%s' outside the accepted sandbox runtime module.\n", finding.Path, finding.Pattern.Raw)
	}
	for _, finding := range codeFindings {
		fmt.Fprintf(stderr, "[forbidden-agent-runtime] %s %s %q matched policy pattern %q\n", finding.Position, finding.Kind, finding.Value, finding.Pattern.Raw)
		fmt.Fprintf(stderr, "[forbidden-agent-runtime] %s must not hard-code agent runtime family '%s' in first-party Go control-plane code.\n", finding.Path, finding.Pattern.Raw)
	}
	for _, finding := range textFindings {
		fmt.Fprintf(stderr, "[forbidden-agent-runtime] %s %s %q matched policy pattern %q\n", finding.Position, finding.Kind, finding.Value, finding.Pattern.Raw)
		fmt.Fprintf(stderr, "[forbidden-agent-runtime] %s must not hard-code agent runtime family '%s' in first-party control-plane source.\n", finding.Path, finding.Pattern.Raw)
	}
	if len(manifestFindings) > 0 || len(codeFindings) > 0 || len(textFindings) > 0 {
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Justification: docs/design/agent-runtime-selection.md keeps agent-runtime dependencies and runtime-specific logic inside the accepted sandbox module.")
		fmt.Fprintf(stderr, "Fix: remove the control-plane dependency or hard-coded runtime name, keep runtime-specific code under %s, or update the runtime selection gate and %s in the same change.\n", acceptedSandboxRuntimeRoot, cfg.PolicyPath)
		return errors.New("agent runtime policy violation")
	}
	fmt.Fprintln(stdout, "[forbidden-agent-runtime] OK")
	return nil
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	cfg := config{
		Root:       ".",
		PolicyPath: getenvDefault("OPENCLARION_AGENT_RUNTIME_POLICY_FILE", defaultPolicyPath),
	}
	fs := flag.NewFlagSet("agent_runtime_policy_check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.Root, "root", cfg.Root, "repository root to scan")
	fs.StringVar(&cfg.PolicyPath, "policy-file", cfg.PolicyPath, "agent runtime policy TSV path")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cleanRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return config{}, fmt.Errorf("resolve root: %w", err)
	}
	cfg.Root = cleanRoot
	if !filepath.IsAbs(cfg.PolicyPath) {
		cfg.PolicyPath = filepath.Join(cfg.Root, cfg.PolicyPath)
	}
	return cfg, nil
}

func getenvDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func readPolicy(cfg config) (policy, error) {
	raw, err := readRegularFile(cfg.PolicyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return policy{}, fmt.Errorf("missing policy file: %s", displayPath(cfg.Root, cfg.PolicyPath))
		}
		return policy{}, fmt.Errorf("read policy file: %w", err)
	}
	seen := map[string]struct{}{}
	var pol policy
	for i, line := range strings.Split(string(raw), "\n") {
		lineNo := i + 1
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return policy{}, fmt.Errorf("invalid policy row %s:%d; expected '<manifest|code|sandbox-allow><TAB><pattern>'", displayPath(cfg.Root, cfg.PolicyPath), lineNo)
		}
		scope, rawPattern := parts[0], parts[1]
		if strings.TrimSpace(scope) != scope || strings.TrimSpace(rawPattern) != rawPattern {
			return policy{}, fmt.Errorf("invalid policy row %s:%d; scope and pattern must not contain leading or trailing whitespace", displayPath(cfg.Root, cfg.PolicyPath), lineNo)
		}
		key := scope + "\t" + rawPattern
		if _, ok := seen[key]; ok {
			return policy{}, fmt.Errorf("duplicate policy row %s:%d: %s", displayPath(cfg.Root, cfg.PolicyPath), lineNo, key)
		}
		seen[key] = struct{}{}
		pattern, err := newPolicyPattern(rawPattern)
		if err != nil {
			return policy{}, fmt.Errorf("invalid policy row %s:%d: %w", displayPath(cfg.Root, cfg.PolicyPath), lineNo, err)
		}
		switch scope {
		case "manifest":
			pol.Manifest = append(pol.Manifest, pattern)
		case "code":
			pol.Code = append(pol.Code, pattern)
		case "sandbox-allow":
			pol.SandboxAllow = append(pol.SandboxAllow, pattern)
		default:
			return policy{}, fmt.Errorf("invalid policy scope %s:%d: %s", displayPath(cfg.Root, cfg.PolicyPath), lineNo, scope)
		}
	}
	if len(pol.Manifest) == 0 || len(pol.Code) == 0 {
		return policy{}, fmt.Errorf("policy file must define at least one manifest and one code pattern: %s", displayPath(cfg.Root, cfg.PolicyPath))
	}
	for _, allowed := range pol.SandboxAllow {
		if !allowed.Exact || !strings.Contains(allowed.Match, "/") {
			return policy{}, fmt.Errorf("sandbox-allow pattern %q must be an exact module path", allowed.Raw)
		}
		if !containsPolicyMatch(pol.Manifest, allowed.Match) || !containsPolicyMatch(pol.Code, allowed.Match) {
			return policy{}, fmt.Errorf("sandbox-allow pattern %q must also exist in manifest and code scopes", allowed.Raw)
		}
	}
	return pol, nil
}

func containsPolicyMatch(patterns []policyPattern, match string) bool {
	for _, pattern := range patterns {
		if pattern.Match == match {
			return true
		}
	}
	return false
}

func newPolicyPattern(raw string) (policyPattern, error) {
	p := policyPattern{Raw: raw}
	if len(raw) >= 2 {
		first, last := raw[0], raw[len(raw)-1]
		if (first == '"' && last == '"') || (first == '`' && last == '`') {
			unquoted, err := strconv.Unquote(raw)
			if err != nil {
				return policyPattern{}, fmt.Errorf("invalid exact-match pattern %q: %w", raw, err)
			}
			if unquoted == "" {
				return policyPattern{}, errors.New("exact-match pattern must not be empty")
			}
			p.Match = strings.ToLower(unquoted)
			p.Exact = true
			return p, nil
		}
	}
	if strings.HasPrefix(raw, `"`) || strings.HasPrefix(raw, "`") || strings.HasSuffix(raw, `"`) || strings.HasSuffix(raw, "`") {
		return policyPattern{}, fmt.Errorf("invalid exact-match pattern %q", raw)
	}
	p.Match = strings.ToLower(raw)
	if p.Match == "" {
		return policyPattern{}, errors.New("pattern must not be empty")
	}
	return p, nil
}

func (p policyPattern) Matches(value string) bool {
	value = strings.ToLower(value)
	if p.Exact {
		return value == p.Match
	}
	return strings.Contains(value, p.Match)
}

func (p policyPattern) MatchesText(value string) bool {
	value = strings.ToLower(value)
	if p.Exact {
		return containsExactToken(value, p.Match)
	}
	return strings.Contains(value, p.Match)
}

type finding struct {
	Path     string
	Position string
	Kind     string
	Value    string
	Pattern  policyPattern
}

func scanManifests(root string, patterns, sandboxAllow []policyPattern) ([]finding, error) {
	paths, err := collectFiles(root, func(_ string, d fs.DirEntry) bool {
		name := d.Name()
		return name == "go.mod" || name == "package.json"
	})
	if err != nil {
		return nil, err
	}
	var findings []finding
	for _, path := range paths {
		var values []string
		switch filepath.Base(path) {
		case "go.mod":
			values, err = goModulePaths(path)
		case "package.json":
			values, err = packageDependencyNames(path)
		}
		if err != nil {
			return nil, err
		}
		rel := displayPath(root, path)
		for _, value := range values {
			for _, pattern := range patterns {
				if pattern.Matches(value) {
					if isAcceptedSandboxRuntimeDependencyValue(root, path, value, pattern, sandboxAllow) {
						continue
					}
					findings = append(findings, finding{Path: rel, Value: value, Pattern: pattern})
				}
			}
		}
	}
	return findings, nil
}

func goModulePaths(path string) ([]string, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	parsed, err := modfile.Parse(path, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	seen := map[string]struct{}{}
	add := func(value string) {
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	for _, req := range parsed.Require {
		add(req.Mod.Path)
	}
	for _, rep := range parsed.Replace {
		add(rep.Old.Path)
		add(rep.New.Path)
	}
	return sortedKeys(seen), nil
}

func packageDependencyNames(path string) ([]string, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if root == nil {
		return nil, fmt.Errorf("parse %s: package.json must be an object", path)
	}
	names := map[string]struct{}{}
	for _, section := range dependencySections {
		rawSection, ok := root[section]
		if !ok {
			continue
		}
		var deps map[string]json.RawMessage
		if err := json.Unmarshal(rawSection, &deps); err != nil {
			return nil, fmt.Errorf("parse %s: %s must be an object: %w", path, section, err)
		}
		if deps == nil {
			return nil, fmt.Errorf("parse %s: %s must be an object", path, section)
		}
		for name := range deps {
			names[name] = struct{}{}
		}
	}
	return sortedKeys(names), nil
}

func scanGoControlPlane(root string, patterns, sandboxAllow []policyPattern) ([]finding, error) {
	paths, err := collectFiles(root, func(path string, d fs.DirEntry) bool {
		return strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") && inControlPlaneGoRoot(root, path)
	})
	if err != nil {
		return nil, err
	}
	var findings []finding
	for _, path := range paths {
		fileFindings, err := scanGoFile(root, path, patterns, sandboxAllow)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fileFindings...)
	}
	return findings, nil
}

func inControlPlaneGoRoot(root, path string) bool {
	rel := displayPath(root, path)
	if strings.HasPrefix(rel, "internal/persistence/ent/") {
		return false
	}
	return strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "scripts/")
}

func scanGoFile(root, path string, patterns, sandboxAllow []policyPattern) ([]finding, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", displayPath(root, path), err)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, raw, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", displayPath(root, path), err)
	}
	rel := displayPath(root, path)
	var findings []finding
	check := func(pos token.Pos, kind, value string) {
		for _, pattern := range patterns {
			if pattern.Matches(value) {
				if isAcceptedSandboxRuntimePattern(root, path, pattern, sandboxAllow) {
					continue
				}
				position := fset.Position(pos).String()
				findings = append(findings, finding{
					Path:     rel,
					Position: displayPath(root, position),
					Kind:     kind,
					Value:    value,
					Pattern:  pattern,
				})
			}
		}
	}
	for _, spec := range parsed.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("parse import %s: %w", fset.Position(spec.Path.Pos()), err)
		}
		check(spec.Path.Pos(), "import path", value)
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.ImportSpec:
			return false
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(n.Value)
			if err != nil {
				check(n.Pos(), "invalid string literal", n.Value)
				return true
			}
			check(n.Pos(), "string literal", value)
		case *ast.Ident:
			check(n.Pos(), "identifier", n.Name)
		}
		return true
	})
	return findings, nil
}

func scanTextControlPlane(root string, patterns, sandboxAllow []policyPattern) ([]finding, error) {
	paths, err := collectFiles(root, func(path string, d fs.DirEntry) bool {
		return isTextSourceFile(d.Name()) && inControlPlaneSourceRoot(root, path)
	})
	if err != nil {
		return nil, err
	}
	var findings []finding
	for _, path := range paths {
		fileFindings, err := scanTextFile(root, path, patterns, sandboxAllow)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fileFindings...)
	}
	return findings, nil
}

func isTextSourceFile(name string) bool {
	if strings.HasSuffix(name, "_test.go") || isJSLikeTestFile(name) {
		return false
	}
	_, ok := textSourceExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

func isJSLikeTestFile(name string) bool {
	for _, suffix := range []string{
		".test.cjs",
		".test.js",
		".test.jsx",
		".test.mjs",
		".test.ts",
		".test.tsx",
		".spec.cjs",
		".spec.js",
		".spec.jsx",
		".spec.mjs",
		".spec.ts",
		".spec.tsx",
	} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func inControlPlaneSourceRoot(root, path string) bool {
	rel := displayPath(root, path)
	if strings.HasPrefix(rel, "internal/persistence/ent/") {
		return false
	}
	return strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "scripts/") || strings.HasPrefix(rel, "web/src/")
}

func isAcceptedSandboxRuntimeRelativePath(path string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	return path == acceptedSandboxRuntimeRoot || strings.HasPrefix(path, acceptedSandboxRuntimeRoot+"/")
}

func isAcceptedSandboxRuntimePattern(root, path string, pattern policyPattern, sandboxAllow []policyPattern) bool {
	return isAcceptedSandboxRuntimeRelativePath(displayPath(root, path)) &&
		containsPolicyMatch(sandboxAllow, pattern.Match)
}

func isAcceptedSandboxRuntimeDependencyValue(root, path, value string, pattern policyPattern, sandboxAllow []policyPattern) bool {
	return isAcceptedSandboxRuntimePattern(root, path, pattern, sandboxAllow) &&
		strings.EqualFold(value, pattern.Match)
}

func scanTextFile(root, path string, patterns, sandboxAllow []policyPattern) ([]finding, error) {
	raw, err := readRegularFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", displayPath(root, path), err)
	}
	rel := displayPath(root, path)
	var findings []finding
	for i, line := range strings.Split(string(raw), "\n") {
		lineNo := i + 1
		value := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		for _, pattern := range patterns {
			if pattern.MatchesText(line) {
				if isAcceptedSandboxRuntimePattern(root, path, pattern, sandboxAllow) {
					continue
				}
				findings = append(findings, finding{
					Path:     rel,
					Position: fmt.Sprintf("%s:%d", rel, lineNo),
					Kind:     "source text",
					Value:    value,
					Pattern:  pattern,
				})
			}
		}
	}
	return findings, nil
}

func containsExactToken(value, token string) bool {
	for offset := 0; ; {
		index := strings.Index(value[offset:], token)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(token)
		if isTokenBoundary(value, start-1) && isTokenBoundary(value, end) {
			return true
		}
		offset = end
	}
}

func isTokenBoundary(value string, index int) bool {
	if index < 0 || index >= len(value) {
		return true
	}
	ch := value[index]
	return !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-')
}

func collectFiles(root string, keep func(path string, d fs.DirEntry) bool) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if keep(path, d) {
				return fmt.Errorf("%s must be a regular file, not a symlink", displayPath(root, path))
			}
			return nil
		}
		if keep(path, d) {
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

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".next", "coverage", "dist", "build":
		return true
	default:
		return false
	}
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must be a regular file, not a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", path)
	}
	// #nosec G304 -- callers pass files discovered under the repository root or
	// the operator-supplied policy path after rejecting symlinks/non-regular files.
	return os.ReadFile(path)
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func displayPath(root, path string) string {
	if strings.Contains(path, ":") {
		file, suffix, ok := strings.Cut(path, ":")
		if ok {
			return displayPath(root, file) + ":" + suffix
		}
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
