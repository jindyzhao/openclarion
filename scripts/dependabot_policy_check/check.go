// Command dependabot_policy_check validates the repository Dependabot update policy.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const defaultPolicyPath = ".github/dependabot.yml"

type config struct {
	Path string
}

type dependabotConfig struct {
	Version int                `yaml:"version"`
	Updates []dependabotUpdate `yaml:"updates"`
}

type dependabotUpdate struct {
	PackageEcosystem      string                     `yaml:"package-ecosystem"`
	Directory             string                     `yaml:"directory"`
	Schedule              dependabotSchedule         `yaml:"schedule"`
	OpenPullRequestsLimit int                        `yaml:"open-pull-requests-limit"`
	Labels                []string                   `yaml:"labels"`
	Groups                map[string]dependabotGroup `yaml:"groups"`
	Ignore                []dependabotIgnore         `yaml:"ignore"`
}

type dependabotSchedule struct {
	Interval string `yaml:"interval"`
	Day      string `yaml:"day"`
	Time     string `yaml:"time"`
	Timezone string `yaml:"timezone"`
}

type dependabotGroup struct {
	AppliesTo   string   `yaml:"applies-to"`
	Patterns    []string `yaml:"patterns"`
	UpdateTypes []string `yaml:"update-types"`
}

type dependabotIgnore struct {
	DependencyName string   `yaml:"dependency-name"`
	UpdateTypes    []string `yaml:"update-types"`
}

type expectedUpdate struct {
	Ecosystem        string
	Directory        string
	ScheduleDay      string
	ScheduleTime     string
	ScheduleTimezone string
	Labels           []string
	PatchGroup       string
	SecurityGroup    string
	AllowIgnore      bool
}

var expectedUpdates = []expectedUpdate{
	{
		Ecosystem:        "github-actions",
		Directory:        "/",
		ScheduleDay:      "monday",
		ScheduleTime:     "09:00",
		ScheduleTimezone: "Asia/Hong_Kong",
		Labels:           []string{"dependencies", "github-actions"},
		PatchGroup:       "github-actions-patch",
		SecurityGroup:    "github-actions-security",
	},
	{
		Ecosystem:        "gomod",
		Directory:        "/",
		ScheduleDay:      "monday",
		ScheduleTime:     "09:30",
		ScheduleTimezone: "Asia/Hong_Kong",
		Labels:           []string{"dependencies", "go"},
		PatchGroup:       "go-patch",
		SecurityGroup:    "go-security",
	},
	{
		Ecosystem:        "gomod",
		Directory:        "/tools/openclarion-linter",
		ScheduleDay:      "monday",
		ScheduleTime:     "09:45",
		ScheduleTimezone: "Asia/Hong_Kong",
		Labels:           []string{"dependencies", "go", "tooling"},
		PatchGroup:       "openclarion-linter-patch",
		SecurityGroup:    "openclarion-linter-security",
		AllowIgnore:      true,
	},
	{
		Ecosystem:        "npm",
		Directory:        "/web",
		ScheduleDay:      "monday",
		ScheduleTime:     "10:00",
		ScheduleTimezone: "Asia/Hong_Kong",
		Labels:           []string{"dependencies", "frontend"},
		PatchGroup:       "web-patch",
		SecurityGroup:    "web-security",
	},
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.Path, "path", defaultPolicyPath, "Dependabot policy path")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[dependabot-policy] %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	if cfg.Path == "" {
		cfg.Path = defaultPolicyPath
	}
	raw, err := readRegularFile(cfg.Path)
	if err != nil {
		return err
	}
	parsed, err := parseDependabotConfig(raw)
	if err != nil {
		return err
	}
	if err := validateDependabotConfig(parsed); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "[dependabot-policy] OK (%d update rules checked)\n", len(parsed.Updates))
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
	raw, err := os.ReadFile(clean) // #nosec G304 -- repository-owned governance file path.
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%s must not be empty", clean)
	}
	return raw, nil
}

func parseDependabotConfig(raw []byte) (dependabotConfig, error) {
	var node yaml.Node
	nodeDecoder := yaml.NewDecoder(bytes.NewReader(raw))
	if err := nodeDecoder.Decode(&node); err != nil {
		return dependabotConfig{}, fmt.Errorf("parse dependabot policy: %w", err)
	}
	var extra yaml.Node
	if err := nodeDecoder.Decode(&extra); err == nil {
		return dependabotConfig{}, errors.New("dependabot policy must contain exactly one YAML document")
	} else if !errors.Is(err, io.EOF) {
		return dependabotConfig{}, fmt.Errorf("parse dependabot policy: %w", err)
	}
	if err := rejectDuplicateKeys(&node, "dependabot"); err != nil {
		return dependabotConfig{}, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var parsed dependabotConfig
	if err := decoder.Decode(&parsed); err != nil {
		return dependabotConfig{}, fmt.Errorf("decode dependabot policy: %w", err)
	}
	return parsed, nil
}

func rejectDuplicateKeys(node *yaml.Node, path string) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			if err := rejectDuplicateKeys(child, path); err != nil {
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
				return fmt.Errorf("duplicate YAML key at %s.%s (line %d)", path, key.Value, key.Line)
			}
			seen[key.Value] = struct{}{}
			childPath := path + "." + key.Value
			if err := rejectDuplicateKeys(value, childPath); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		for i, child := range node.Content {
			if err := rejectDuplicateKeys(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDependabotConfig(cfg dependabotConfig) error {
	if cfg.Version != 2 {
		return fmt.Errorf("dependabot version = %d, want 2", cfg.Version)
	}
	if len(cfg.Updates) != len(expectedUpdates) {
		return fmt.Errorf("dependabot updates count = %d, want %d", len(cfg.Updates), len(expectedUpdates))
	}

	expectedByKey := map[string]expectedUpdate{}
	for _, expected := range expectedUpdates {
		expectedByKey[updateKey(expected.Ecosystem, expected.Directory)] = expected
	}

	seen := map[string]struct{}{}
	for _, update := range cfg.Updates {
		key := updateKey(update.PackageEcosystem, update.Directory)
		expected, ok := expectedByKey[key]
		if !ok {
			return fmt.Errorf("unexpected dependabot update rule %s", key)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate dependabot update rule %s", key)
		}
		seen[key] = struct{}{}
		if err := validateUpdate(expected, update); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	for key := range expectedByKey {
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("missing dependabot update rule %s", key)
		}
	}
	return nil
}

func updateKey(ecosystem, directory string) string {
	return ecosystem + "@" + directory
}

func validateUpdate(expected expectedUpdate, update dependabotUpdate) error {
	if update.Schedule.Interval != "weekly" {
		return fmt.Errorf("schedule.interval = %q, want weekly", update.Schedule.Interval)
	}
	if update.Schedule.Day != expected.ScheduleDay {
		return fmt.Errorf("schedule.day = %q, want %q", update.Schedule.Day, expected.ScheduleDay)
	}
	if update.Schedule.Time != expected.ScheduleTime {
		return fmt.Errorf("schedule.time = %q, want %q", update.Schedule.Time, expected.ScheduleTime)
	}
	if update.Schedule.Timezone != expected.ScheduleTimezone {
		return fmt.Errorf("schedule.timezone = %q, want %q", update.Schedule.Timezone, expected.ScheduleTimezone)
	}
	if update.OpenPullRequestsLimit != 10 {
		return fmt.Errorf("open-pull-requests-limit = %d, want 10", update.OpenPullRequestsLimit)
	}
	if !equalStringSets(update.Labels, expected.Labels) {
		return fmt.Errorf("labels = %s, want %s", formatStrings(update.Labels), formatStrings(expected.Labels))
	}
	if len(update.Groups) != 2 {
		return fmt.Errorf("groups count = %d, want exactly patch and security groups", len(update.Groups))
	}
	if err := validatePatchGroup(expected.PatchGroup, update.Groups[expected.PatchGroup]); err != nil {
		return err
	}
	if err := validateSecurityGroup(expected.SecurityGroup, update.Groups[expected.SecurityGroup]); err != nil {
		return err
	}
	if err := validateIgnorePolicy(expected, update.Ignore); err != nil {
		return err
	}
	return nil
}

func validatePatchGroup(name string, group dependabotGroup) error {
	if isZeroGroup(group) {
		return fmt.Errorf("missing patch group %q", name)
	}
	if group.AppliesTo != "" {
		return fmt.Errorf("patch group %q applies-to = %q, want empty version-update default", name, group.AppliesTo)
	}
	if !equalStrings(group.Patterns, []string{"*"}) {
		return fmt.Errorf("patch group %q patterns = %s, want [*]", name, formatStrings(group.Patterns))
	}
	if !equalStrings(group.UpdateTypes, []string{"patch"}) {
		return fmt.Errorf("patch group %q update-types = %s, want [patch]", name, formatStrings(group.UpdateTypes))
	}
	return nil
}

func validateSecurityGroup(name string, group dependabotGroup) error {
	if isZeroGroup(group) {
		return fmt.Errorf("missing security group %q", name)
	}
	if group.AppliesTo != "security-updates" {
		return fmt.Errorf("security group %q applies-to = %q, want security-updates", name, group.AppliesTo)
	}
	if !equalStrings(group.Patterns, []string{"*"}) {
		return fmt.Errorf("security group %q patterns = %s, want [*]", name, formatStrings(group.Patterns))
	}
	if len(group.UpdateTypes) != 0 {
		return fmt.Errorf("security group %q must not restrict update-types", name)
	}
	return nil
}

func validateIgnorePolicy(expected expectedUpdate, ignores []dependabotIgnore) error {
	if !expected.AllowIgnore {
		if len(ignores) != 0 {
			return errors.New("ignore entries are forbidden outside the linter tooling exception")
		}
		return nil
	}
	if len(ignores) != 1 {
		return fmt.Errorf("linter tooling ignore entries = %d, want exactly 1", len(ignores))
	}
	ignore := ignores[0]
	if ignore.DependencyName != "golang.org/x/tools" {
		return fmt.Errorf("linter tooling ignore dependency-name = %q, want golang.org/x/tools", ignore.DependencyName)
	}
	want := []string{"version-update:semver-major", "version-update:semver-minor"}
	if !equalStringSets(ignore.UpdateTypes, want) {
		return fmt.Errorf("linter tooling ignore update-types = %s, want %s", formatStrings(ignore.UpdateTypes), formatStrings(want))
	}
	return nil
}

func isZeroGroup(group dependabotGroup) bool {
	return group.AppliesTo == "" && len(group.Patterns) == 0 && len(group.UpdateTypes) == 0
}

func equalStrings(got, want []string) bool {
	return slices.Equal(got, want)
}

func equalStringSets(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)
	sort.Strings(gotCopy)
	sort.Strings(wantCopy)
	return equalStrings(gotCopy, wantCopy)
}

func formatStrings(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	return "[" + strings.Join(values, ",") + "]"
}
