// Command dependabot_policy_check validates the repository Dependabot policy.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const defaultDependabotPath = ".github/dependabot.yml"

type dependabotConfig struct {
	Version int                `yaml:"version"`
	Updates []dependabotUpdate `yaml:"updates"`
}

type dependabotUpdate struct {
	PackageEcosystem      string                     `yaml:"package-ecosystem"`
	Directory             string                     `yaml:"directory"`
	Schedule              map[string]any             `yaml:"schedule"`
	OpenPullRequestsLimit int                        `yaml:"open-pull-requests-limit"`
	Labels                []string                   `yaml:"labels"`
	Groups                map[string]dependabotGroup `yaml:"groups"`
	Ignore                []dependabotIgnore         `yaml:"ignore"`
}

type dependabotGroup struct {
	AppliesTo   string   `yaml:"applies-to"`
	Patterns    []string `yaml:"patterns"`
	UpdateTypes []string `yaml:"update-types"`
}

type dependabotIgnore struct {
	DependencyName string   `yaml:"dependency-name"`
	UpdateTypes    []string `yaml:"update-types"`
	Versions       []string `yaml:"versions"`
}

type finding struct {
	Path string
	Msg  string
}

func main() {
	if err := run(defaultDependabotPath, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[dependabot-policy] %v\n", err)
		os.Exit(1)
	}
}

func run(path string, out io.Writer) error {
	cfg, err := readConfig(path)
	if err != nil {
		return err
	}
	findings := validateConfig(cfg)
	if len(findings) > 0 {
		sortFindings(findings)
		fmt.Fprintln(out, "[dependabot-policy] policy drift:")
		for _, finding := range findings {
			fmt.Fprintf(out, "  %s: %s\n", finding.Path, finding.Msg)
		}
		return fmt.Errorf("found %d policy issue(s)", len(findings))
	}
	fmt.Fprintln(out, "[dependabot-policy] OK")
	return nil
}

func readConfig(path string) (dependabotConfig, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- repository-owned checker input.
	if err != nil {
		return dependabotConfig{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var cfg dependabotConfig
	if err := decoder.Decode(&cfg); err != nil {
		return dependabotConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return dependabotConfig{}, fmt.Errorf("%s: multiple YAML documents are not allowed", path)
		}
		return dependabotConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func validateConfig(cfg dependabotConfig) []finding {
	var findings []finding
	if cfg.Version != 2 {
		findings = append(findings, finding{Path: defaultDependabotPath, Msg: "version must be 2"})
	}
	findings = append(findings, validateUpdate(cfg, "npm", "/web", validateWebUpdate)...)
	findings = append(findings, validateUpdate(cfg, "gomod", "/tools/openclarion-linter", validateLinterUpdate)...)
	return findings
}

func validateUpdate(cfg dependabotConfig, ecosystem, directory string, validate func(dependabotUpdate) []finding) []finding {
	var matches []dependabotUpdate
	for _, update := range cfg.Updates {
		if update.PackageEcosystem == ecosystem && update.Directory == directory {
			matches = append(matches, update)
		}
	}
	path := fmt.Sprintf("%s update %s", ecosystem, directory)
	if len(matches) == 0 {
		return []finding{{Path: path, Msg: "missing update block"}}
	}
	if len(matches) > 1 {
		return []finding{{Path: path, Msg: "duplicate update blocks are not allowed"}}
	}
	return validate(matches[0])
}

func validateWebUpdate(update dependabotUpdate) []finding {
	var findings []finding
	findings = append(findings, requirePatchGroup(update, "web-patch")...)
	findings = append(findings, requireSecurityGroup(update, "web-security")...)
	for _, dependency := range []string{"typescript", "eslint"} {
		findings = append(findings, requireExactIgnore(update, dependency, []string{"version-update:semver-major"})...)
	}
	return findings
}

func validateLinterUpdate(update dependabotUpdate) []finding {
	var findings []finding
	findings = append(findings, requirePatchGroup(update, "openclarion-linter-patch")...)
	findings = append(findings, requireSecurityGroup(update, "openclarion-linter-security")...)
	findings = append(findings, requireExactIgnore(update, "golang.org/x/tools", []string{
		"version-update:semver-minor",
		"version-update:semver-major",
	})...)
	return findings
}

func requirePatchGroup(update dependabotUpdate, groupName string) []finding {
	group, ok := update.Groups[groupName]
	if !ok {
		return []finding{{Path: groupPath(update, groupName), Msg: "missing patch group"}}
	}
	var findings []finding
	if !sameStringSet(group.Patterns, []string{"*"}) {
		findings = append(findings, finding{Path: groupPath(update, groupName), Msg: "patterns must be exactly [\"*\"]"})
	}
	if !sameStringSet(group.UpdateTypes, []string{"patch"}) {
		findings = append(findings, finding{Path: groupPath(update, groupName), Msg: "update-types must be exactly [\"patch\"]"})
	}
	if group.AppliesTo != "" {
		findings = append(findings, finding{Path: groupPath(update, groupName), Msg: "patch group must not set applies-to"})
	}
	return findings
}

func requireSecurityGroup(update dependabotUpdate, groupName string) []finding {
	group, ok := update.Groups[groupName]
	if !ok {
		return []finding{{Path: groupPath(update, groupName), Msg: "missing security-update group"}}
	}
	var findings []finding
	if group.AppliesTo != "security-updates" {
		findings = append(findings, finding{Path: groupPath(update, groupName), Msg: `applies-to must be "security-updates"`})
	}
	if !sameStringSet(group.Patterns, []string{"*"}) {
		findings = append(findings, finding{Path: groupPath(update, groupName), Msg: "patterns must be exactly [\"*\"]"})
	}
	if len(group.UpdateTypes) != 0 {
		findings = append(findings, finding{Path: groupPath(update, groupName), Msg: "security group must not restrict update-types"})
	}
	return findings
}

func requireExactIgnore(update dependabotUpdate, dependency string, updateTypes []string) []finding {
	var matches []dependabotIgnore
	for _, ignore := range update.Ignore {
		if ignore.DependencyName == dependency {
			matches = append(matches, ignore)
		}
	}
	path := fmt.Sprintf("%s %s ignore %s", update.PackageEcosystem, update.Directory, dependency)
	if len(matches) == 0 {
		return []finding{{Path: path, Msg: "missing ignore entry"}}
	}
	if len(matches) > 1 {
		return []finding{{Path: path, Msg: "duplicate ignore entries are not allowed"}}
	}
	ignore := matches[0]
	var findings []finding
	if !sameStringSet(ignore.UpdateTypes, updateTypes) {
		findings = append(findings, finding{Path: path, Msg: fmt.Sprintf("update-types must be exactly %q", updateTypes)})
	}
	if len(ignore.Versions) != 0 {
		findings = append(findings, finding{Path: path, Msg: "versions must stay empty so security updates are not version-blocked"})
	}
	return findings
}

func groupPath(update dependabotUpdate, groupName string) string {
	return fmt.Sprintf("%s %s group %s", update.PackageEcosystem, update.Directory, groupName)
}

func sameStringSet(got, want []string) bool {
	got = normalizeStrings(got)
	want = normalizeStrings(want)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortFindings(findings []finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			return findings[i].Msg < findings[j].Msg
		}
		return findings[i].Path < findings[j].Path
	})
}
