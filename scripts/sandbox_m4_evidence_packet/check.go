// Command sandbox_m4_evidence_packet assembles a repeatable M4 sandbox
// decision evidence packet by running the existing baseline, quality, and
// decision helpers. It does not run Docker, an LLM, or an agent runtime.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

const (
	maxInputBytes                     int64 = 1024 * 1024
	defaultMinCases                         = 3
	defaultCommandTimeout                   = 10 * time.Minute
	decisionProceed                         = "proceed"
	decisionIterate                         = "iterate"
	decisionDefer                           = "defer"
	canonicalProceedReason                  = "all required M4 evidence is present and no regression was detected"
	loopbackRuntimeCandidateReason          = "review evidence runtime_candidate uses a loopback registry reference; publishable runtime baselines must use a non-loopback registry reference"
	requiredSnapshotRefPrefix               = "snapshot:"
	maxReviewEvidenceTextBytes              = 2048
	maxEvidenceCaseIDBytes                  = 128
	maxEvidenceRefRunes                     = 120
	maxRuntimeSmokeEvidenceRefBytes         = 256
	maxQualityManifestCases                 = 100
	maxQualityManifestRequiredRefs          = 20
	maxQualityManifestReportPathBytes       = 512
)

var artifactNames = map[string]string{
	"baseline": "baseline-audit.json",
	"quality":  "quality-comparison.json",
	"review":   "review-evidence.json",
	"decision": "decision.json",
	"packet":   "packet.json",
}

const (
	runtimeSmokeArtifactsDir = "runtime-smoke-artifacts"
	qualityInputsDir         = "quality-inputs"
	qualityManifestRef       = qualityInputsDir + "/quality-manifest.json"
	qualityReportsDir        = qualityInputsDir + "/reports"
)

var requiredBaselineCheckNames = []string{
	"fixed_file_contract",
	"batch_network_none_spec",
	"m5_turn_input_mounts",
	"docker_security_posture",
	"allowlist_enforcer_subset",
	"allowlist_enforcer_drift_rejection",
	"raw_result_validation",
}

var requiredQualityScenarioNames = []string{
	"single_alert",
	"cascade",
	"alert_storm",
}

var requiredRuntimeSmokeNames = []string{
	"candidate_runtime_file_contract",
	"container_provider_lifecycle",
	"container_provider_timeout_cleanup",
	"container_provider_output_cap",
	"egress_allowdeny",
}

var requiredRuntimeSmokeSources = map[string]string{
	"candidate_runtime_file_contract":    "make agent-runtime-smoke",
	"container_provider_lifecycle":       "make container-provider-smoke",
	"container_provider_timeout_cleanup": "make container-provider-timeout-smoke",
	"container_provider_output_cap":      "make container-provider-output-cap-smoke",
	"egress_allowdeny":                   "make egress-allowdeny-smoke",
}

var imageDigestHexRe = regexp.MustCompile(`^[a-f0-9]{64}$`)

var todayUTC = func() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

type config struct {
	VerifyPacketPath          string
	QualityManifestPath       string
	ReviewEvidencePath        string
	RuntimeSmokeArtifactsRoot string
	OutDir                    string
	MinCases                  int
	FailUnless                string
	CommandTimeout            time.Duration
}

type commandRunner interface {
	Run(ctx context.Context, spec commandSpec) ([]byte, []byte, error)
}

type commandSpec struct {
	Name string
	Args []string
}

type execRunner struct{}

type packetOutput struct {
	Tool                  string                       `json:"tool"`
	OutDir                string                       `json:"out_dir"`
	Artifacts             packetArtifacts              `json:"artifacts"`
	ArtifactSHA256        packetArtifactDigests        `json:"artifact_sha256"`
	QualityInputs         packetQualityInputs          `json:"quality_inputs"`
	RuntimeSmokeArtifacts []packetRuntimeSmokeArtifact `json:"runtime_smoke_artifacts"`
	Decision              string                       `json:"decision"`
	ReviewRequired        bool                         `json:"review_required"`
	Commands              []packetCommand              `json:"commands"`
}

type packetArtifacts struct {
	BaselineAudit     string `json:"baseline_audit"`
	QualityComparison string `json:"quality_comparison"`
	ReviewEvidence    string `json:"review_evidence"`
	Decision          string `json:"decision"`
	Packet            string `json:"packet"`
}

type packetArtifactDigests struct {
	BaselineAudit     string `json:"baseline_audit"`
	QualityComparison string `json:"quality_comparison"`
	ReviewEvidence    string `json:"review_evidence"`
	Decision          string `json:"decision"`
}

type packetRuntimeSmokeArtifact struct {
	Name           string `json:"name"`
	EvidenceRef    string `json:"evidence_ref"`
	Path           string `json:"path"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type packetQualityInputs struct {
	Manifest packetInputArtifact           `json:"manifest"`
	Reports  []packetQualityReportArtifact `json:"reports"`
}

type packetInputArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type packetQualityReportArtifact struct {
	CaseID      string `json:"case_id"`
	Role        string `json:"role"`
	ManifestRef string `json:"manifest_ref"`
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
}

type packetCommand struct {
	Name       string   `json:"name"`
	Args       []string `json:"args"`
	OutputPath string   `json:"output_path"`
	Status     string   `json:"status"`
}

type decisionFile struct {
	Tool           string                 `json:"tool"`
	Decision       string                 `json:"decision"`
	ReviewRequired bool                   `json:"review_required"`
	Evidence       helperDecisionEvidence `json:"evidence"`
	Reasons        []string               `json:"reasons"`
}

type helperBaselineAudit struct {
	Tool   string              `json:"tool"`
	Status string              `json:"status"`
	Checks []helperStatusCheck `json:"checks"`
}

type helperStatusCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type helperQualityComparison struct {
	Tool             string               `json:"tool"`
	Schema           string               `json:"schema"`
	Mode             string               `json:"mode"`
	CaseCount        int                  `json:"case_count"`
	SampleBasis      string               `json:"sample_basis"`
	ScenarioCoverage []string             `json:"scenario_coverage"`
	Summary          helperQualitySummary `json:"summary"`
	Recommendation   string               `json:"recommendation"`
	ReviewRequired   bool                 `json:"review_required"`
	Cases            []helperQualityCase  `json:"cases"`
}

type helperQualitySummary struct {
	ImprovedCount         int `json:"improved_count"`
	EquivalentCount       int `json:"equivalent_count"`
	RegressedCount        int `json:"regressed_count"`
	NeedsHumanReviewCount int `json:"needs_human_review_count"`
}

type helperReportMetrics struct {
	FindingCount            int `json:"finding_count"`
	RecommendedActionCount  int `json:"recommended_action_count"`
	HighPriorityActionCount int `json:"high_priority_action_count"`
	UniqueEvidenceRefCount  int `json:"unique_evidence_ref_count"`
	ConfidenceRank          int `json:"confidence_rank"`
	SeverityRank            int `json:"severity_rank"`
}

type helperReportDeltas struct {
	FindingCount            int `json:"finding_count"`
	RecommendedActionCount  int `json:"recommended_action_count"`
	HighPriorityActionCount int `json:"high_priority_action_count"`
	UniqueEvidenceRefCount  int `json:"unique_evidence_ref_count"`
	ConfidenceRank          int `json:"confidence_rank"`
	SeverityRank            int `json:"severity_rank"`
}

type helperQualityCase struct {
	ID                   string              `json:"id"`
	Scenario             string              `json:"scenario"`
	RequiredEvidenceRefs []string            `json:"required_evidence_refs"`
	Direct               helperReportMetrics `json:"direct"`
	Sandbox              helperReportMetrics `json:"sandbox"`
	Delta                helperReportDeltas  `json:"delta"`
	Recommendation       string              `json:"recommendation"`
	ReviewRequired       bool                `json:"review_required"`
	Notes                []string            `json:"notes,omitempty"`
}

type helperDecisionOutput struct {
	Tool           string                 `json:"tool"`
	Decision       string                 `json:"decision"`
	ReviewRequired bool                   `json:"review_required"`
	Evidence       helperDecisionEvidence `json:"evidence"`
	Reasons        []string               `json:"reasons"`
}

type helperDecisionEvidence struct {
	BaselineAuditStatus      string   `json:"baseline_audit_status"`
	QualityRecommendation    string   `json:"quality_recommendation"`
	CaseCount                int      `json:"case_count"`
	MinimumCaseCount         int      `json:"minimum_case_count"`
	SampleBasis              string   `json:"sample_basis"`
	ScenarioCoverage         []string `json:"scenario_coverage"`
	SelectedCandidate        string   `json:"selected_candidate"`
	RuntimeCandidate         string   `json:"runtime_candidate"`
	CandidateEvaluationCount int      `json:"candidate_evaluation_count"`
	RuntimeSmokePassedCount  int      `json:"runtime_smoke_passed_count"`
	ReviewedCaseCount        int      `json:"reviewed_case_count"`
	RepresentativeSample     bool     `json:"representative_sample"`
	HumanReviewStatus        string   `json:"human_review_status"`
}

type packetReviewEvidence struct {
	Tool                 string                      `json:"tool"`
	EvidenceDate         string                      `json:"evidence_date"`
	SelectedCandidate    string                      `json:"selected_candidate"`
	RuntimeCandidate     string                      `json:"runtime_candidate"`
	RepresentativeSample bool                        `json:"representative_sample"`
	SampleBasis          string                      `json:"sample_basis"`
	CandidateEvaluations []packetCandidateEvaluation `json:"candidate_evaluations"`
	RuntimeSmokes        []packetRuntimeSmoke        `json:"runtime_smokes"`
	ReviewedCases        []packetReviewedCase        `json:"reviewed_cases"`
	HumanReview          packetHumanReview           `json:"human_review"`
}

type packetCandidateEvaluation struct {
	Candidate        string   `json:"candidate"`
	Status           string   `json:"status"`
	RuntimeCandidate string   `json:"runtime_candidate,omitempty"`
	RuntimeSmokeRefs []string `json:"runtime_smoke_refs,omitempty"`
	Source           string   `json:"source"`
	Notes            string   `json:"notes"`
}

type packetReviewedCase struct {
	ID                   string   `json:"id"`
	Scenario             string   `json:"scenario"`
	RequiredEvidenceRefs []string `json:"required_evidence_refs"`
	Status               string   `json:"status"`
	Notes                string   `json:"notes"`
}

type packetRuntimeSmoke struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Source         string `json:"source"`
	EvidenceRef    string `json:"evidence_ref"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type packetQualityManifest struct {
	SampleBasis string                      `json:"sample_basis"`
	Cases       []packetQualityManifestCase `json:"cases"`
}

type packetQualityManifestCase struct {
	ID                   string   `json:"id"`
	Scenario             string   `json:"scenario"`
	RequiredEvidenceRefs []string `json:"required_evidence_refs"`
	DirectSubReport      string   `json:"direct_sub_report"`
	SandboxSubReport     string   `json:"sandbox_sub_report"`
}

type packetHumanReview struct {
	Status   string `json:"status"`
	Reviewer string `json:"reviewer"`
	Notes    string `json:"notes,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sandbox-m4-evidence-packet] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	return runWithRunner(context.Background(), args, stdout, execRunner{})
}

func runWithRunner(ctx context.Context, args []string, stdout io.Writer, runner commandRunner) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	var out packetOutput
	if cfg.VerifyPacketPath != "" {
		out, err = verifyPacket(cfg.VerifyPacketPath)
	} else {
		out, err = assemblePacket(ctx, cfg, runner)
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("sandbox_m4_evidence_packet", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	verifyPacketPath := fs.String("verify-packet", "", "existing packet directory or packet.json path to verify without re-running helpers")
	qualityManifest := fs.String("quality-manifest", "", "sandbox_quality_compare manifest JSON path")
	reviewEvidence := fs.String("review-evidence", "", "sandbox_m4_review_evidence JSON path")
	runtimeSmokeArtifactsRoot := fs.String("runtime-smoke-artifacts-root", "", "directory containing runtime smoke artifacts referenced by review evidence")
	outDir := fs.String("out-dir", "", "empty output directory for the evidence packet")
	minCases := fs.Int("min-cases", defaultMinCases, "minimum representative quality cases required")
	failUnless := fs.String("fail-unless", "", "pass through to sandbox_m4_decision")
	commandTimeout := fs.Duration("command-timeout", defaultCommandTimeout, "timeout for each helper command")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg := config{
		VerifyPacketPath:          strings.TrimSpace(*verifyPacketPath),
		QualityManifestPath:       strings.TrimSpace(*qualityManifest),
		ReviewEvidencePath:        strings.TrimSpace(*reviewEvidence),
		RuntimeSmokeArtifactsRoot: strings.TrimSpace(*runtimeSmokeArtifactsRoot),
		OutDir:                    strings.TrimSpace(*outDir),
		MinCases:                  *minCases,
		FailUnless:                strings.TrimSpace(*failUnless),
		CommandTimeout:            *commandTimeout,
	}
	if cfg.VerifyPacketPath != "" {
		if cfg.QualityManifestPath != "" || cfg.ReviewEvidencePath != "" || cfg.OutDir != "" || cfg.RuntimeSmokeArtifactsRoot != "" || cfg.FailUnless != "" {
			return config{}, errors.New("--verify-packet cannot be combined with assembly inputs")
		}
		return cfg, nil
	}
	if cfg.QualityManifestPath == "" {
		return config{}, errors.New("--quality-manifest is required")
	}
	if cfg.ReviewEvidencePath == "" {
		return config{}, errors.New("--review-evidence is required")
	}
	if cfg.OutDir == "" {
		return config{}, errors.New("--out-dir is required")
	}
	if cfg.RuntimeSmokeArtifactsRoot == "" {
		cfg.RuntimeSmokeArtifactsRoot = filepath.Dir(cfg.ReviewEvidencePath)
	}
	if cfg.MinCases <= 0 {
		return config{}, errors.New("--min-cases must be greater than zero")
	}
	if cfg.CommandTimeout <= 0 {
		return config{}, errors.New("--command-timeout must be greater than zero")
	}
	return cfg, nil
}

func assemblePacket(ctx context.Context, cfg config, runner commandRunner) (packetOutput, error) {
	outDir, err := prepareOutDir(cfg.OutDir)
	if err != nil {
		return packetOutput{}, err
	}
	artifacts := packetArtifacts{
		BaselineAudit:     filepath.Join(outDir, artifactNames["baseline"]),
		QualityComparison: filepath.Join(outDir, artifactNames["quality"]),
		ReviewEvidence:    filepath.Join(outDir, artifactNames["review"]),
		Decision:          filepath.Join(outDir, artifactNames["decision"]),
		Packet:            filepath.Join(outDir, artifactNames["packet"]),
	}

	var commands []packetCommand
	baselineSpec := commandSpec{Name: "go", Args: []string{"run", "./scripts/sandbox_baseline_audit"}}
	if err := runCommandToFile(ctx, runner, baselineSpec, artifacts.BaselineAudit, cfg.CommandTimeout); err != nil {
		return packetOutput{}, err
	}
	commands = append(commands, packetCommand{Name: baselineSpec.Name, Args: baselineSpec.Args, OutputPath: artifacts.BaselineAudit, Status: "pass"})

	qualitySpec := commandSpec{Name: "go", Args: []string{"run", "./scripts/sandbox_quality_compare", "--manifest", cfg.QualityManifestPath, "--fail-on-regression"}}
	if err := runCommandToFile(ctx, runner, qualitySpec, artifacts.QualityComparison, cfg.CommandTimeout); err != nil {
		return packetOutput{}, err
	}
	commands = append(commands, packetCommand{Name: qualitySpec.Name, Args: qualitySpec.Args, OutputPath: artifacts.QualityComparison, Status: "pass"})
	var quality helperQualityComparison
	if err := readStrictJSONArtifact(artifacts.QualityComparison, &quality); err != nil {
		return packetOutput{}, fmt.Errorf("quality comparison: %w", err)
	}
	qualityInputs, err := copyQualityInputs(cfg.QualityManifestPath, outDir, quality)
	if err != nil {
		return packetOutput{}, err
	}

	if err := copyEvidenceFile(cfg.ReviewEvidencePath, artifacts.ReviewEvidence, quality.SampleBasis); err != nil {
		return packetOutput{}, err
	}
	var review packetReviewEvidence
	if err := readStrictJSONArtifact(artifacts.ReviewEvidence, &review); err != nil {
		return packetOutput{}, fmt.Errorf("review evidence: %w", err)
	}
	runtimeSmokeArtifacts, err := copyRuntimeSmokeArtifacts(cfg.RuntimeSmokeArtifactsRoot, outDir, review.RuntimeSmokes)
	if err != nil {
		return packetOutput{}, err
	}

	decisionArgs := []string{
		"run", "./scripts/sandbox_m4_decision",
		"--baseline-audit", artifacts.BaselineAudit,
		"--quality-comparison", artifacts.QualityComparison,
		"--review-evidence", artifacts.ReviewEvidence,
		"--min-cases", fmt.Sprintf("%d", cfg.MinCases),
	}
	if cfg.FailUnless != "" {
		decisionArgs = append(decisionArgs, "--fail-unless", cfg.FailUnless)
	}
	decisionSpec := commandSpec{Name: "go", Args: decisionArgs}
	if err := runCommandToFile(ctx, runner, decisionSpec, artifacts.Decision, cfg.CommandTimeout); err != nil {
		return packetOutput{}, err
	}
	commands = append(commands, packetCommand{Name: decisionSpec.Name, Args: decisionSpec.Args, OutputPath: artifacts.Decision, Status: "pass"})

	decision, err := parseDecisionFile(artifacts.Decision)
	if err != nil {
		return packetOutput{}, err
	}
	if err := validateDecisionEvidenceMatchesArtifacts(decision, artifacts, cfg.MinCases); err != nil {
		return packetOutput{}, err
	}
	digests, err := computeArtifactDigests(artifacts)
	if err != nil {
		return packetOutput{}, err
	}
	localArtifacts := packetLocalArtifacts()
	localPaths := artifactPathMap(artifacts, localArtifacts)
	localPaths[cfg.QualityManifestPath] = qualityInputs.Manifest.Path
	localPaths[filepath.Clean(cfg.QualityManifestPath)] = qualityInputs.Manifest.Path
	packet := packetOutput{
		Tool:                  "sandbox_m4_evidence_packet",
		OutDir:                ".",
		Artifacts:             localArtifacts,
		ArtifactSHA256:        digests,
		QualityInputs:         qualityInputs,
		RuntimeSmokeArtifacts: runtimeSmokeArtifacts,
		Decision:              decision.Decision,
		ReviewRequired:        decision.ReviewRequired,
		Commands:              localizePacketCommands(commands, localPaths),
	}
	if err := writeJSONFile(artifacts.Packet, packet); err != nil {
		return packetOutput{}, err
	}
	return packet, nil
}

func packetLocalArtifacts() packetArtifacts {
	return packetArtifacts{
		BaselineAudit:     artifactNames["baseline"],
		QualityComparison: artifactNames["quality"],
		ReviewEvidence:    artifactNames["review"],
		Decision:          artifactNames["decision"],
		Packet:            artifactNames["packet"],
	}
}

func artifactPathMap(actual, local packetArtifacts) map[string]string {
	return map[string]string{
		actual.BaselineAudit:     local.BaselineAudit,
		actual.QualityComparison: local.QualityComparison,
		actual.ReviewEvidence:    local.ReviewEvidence,
		actual.Decision:          local.Decision,
		actual.Packet:            local.Packet,
	}
}

func localizePacketCommands(commands []packetCommand, paths map[string]string) []packetCommand {
	out := make([]packetCommand, 0, len(commands))
	for _, command := range commands {
		local := command
		local.Args = append([]string(nil), command.Args...)
		if replacement, ok := paths[command.OutputPath]; ok {
			local.OutputPath = replacement
		}
		for i, arg := range local.Args {
			if replacement, ok := paths[arg]; ok {
				local.Args[i] = replacement
			}
		}
		out = append(out, local)
	}
	return out
}

func computeArtifactDigests(artifacts packetArtifacts) (packetArtifactDigests, error) {
	baseline, err := fileSHA256Hex(artifacts.BaselineAudit)
	if err != nil {
		return packetArtifactDigests{}, fmt.Errorf("hash baseline audit: %w", err)
	}
	quality, err := fileSHA256Hex(artifacts.QualityComparison)
	if err != nil {
		return packetArtifactDigests{}, fmt.Errorf("hash quality comparison: %w", err)
	}
	review, err := fileSHA256Hex(artifacts.ReviewEvidence)
	if err != nil {
		return packetArtifactDigests{}, fmt.Errorf("hash review evidence: %w", err)
	}
	decision, err := fileSHA256Hex(artifacts.Decision)
	if err != nil {
		return packetArtifactDigests{}, fmt.Errorf("hash decision: %w", err)
	}
	return packetArtifactDigests{
		BaselineAudit:     baseline,
		QualityComparison: quality,
		ReviewEvidence:    review,
		Decision:          decision,
	}, nil
}

func verifyPacket(inputPath string) (packetOutput, error) {
	packetPath, root, err := resolvePacketInput(inputPath)
	if err != nil {
		return packetOutput{}, err
	}
	var packet packetOutput
	if err := readStrictJSONArtifact(packetPath, &packet); err != nil {
		return packetOutput{}, fmt.Errorf("packet summary: %w", err)
	}
	if err := verifyPacketSummary(root, packetPath, packet); err != nil {
		return packetOutput{}, err
	}
	return packet, nil
}

func resolvePacketInput(inputPath string) (string, string, error) {
	clean := filepath.Clean(inputPath)
	info, err := os.Stat(clean)
	if err != nil {
		return "", "", fmt.Errorf("stat packet input %s: %w", clean, err)
	}
	var packetPath, root string
	if info.IsDir() {
		root = clean
		packetPath = filepath.Join(root, artifactNames["packet"])
	} else {
		packetPath = clean
		root = filepath.Dir(clean)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve packet root: %w", err)
	}
	absPacket, err := filepath.Abs(packetPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve packet path: %w", err)
	}
	return absPacket, absRoot, nil
}

func verifyPacketSummary(root, packetPath string, packet packetOutput) error {
	if packet.Tool != "sandbox_m4_evidence_packet" {
		return fmt.Errorf("packet tool = %q, want sandbox_m4_evidence_packet", packet.Tool)
	}
	if packet.OutDir != "." {
		return fmt.Errorf("packet out_dir = %q, want packet-local root", packet.OutDir)
	}
	if !validDecision(packet.Decision) {
		return fmt.Errorf("packet decision = %q, want proceed, iterate, or defer", packet.Decision)
	}
	if !packet.ReviewRequired {
		return errors.New("packet review_required must be true")
	}
	if packet.Artifacts != packetLocalArtifacts() {
		return fmt.Errorf("packet artifacts = %+v, want %+v", packet.Artifacts, packetLocalArtifacts())
	}
	actualArtifacts, err := resolvePacketArtifactPaths(root, packet.Artifacts)
	if err != nil {
		return err
	}
	if filepath.Clean(packetPath) != filepath.Clean(actualArtifacts.Packet) {
		return fmt.Errorf("verify packet input %s must match packet artifact path %s", filepath.Clean(packetPath), filepath.Clean(actualArtifacts.Packet))
	}
	minCases, err := verifyPacketCommands(packet)
	if err != nil {
		return err
	}
	if err := verifyPacketInventory(root, packet); err != nil {
		return err
	}
	if err := verifyPacketCoreArtifacts(actualArtifacts, packet.ArtifactSHA256); err != nil {
		return err
	}

	var quality helperQualityComparison
	if err := readStrictJSONArtifact(actualArtifacts.QualityComparison, &quality); err != nil {
		return fmt.Errorf("quality comparison: %w", err)
	}
	var review packetReviewEvidence
	if err := readStrictJSONArtifact(actualArtifacts.ReviewEvidence, &review); err != nil {
		return fmt.Errorf("review evidence: %w", err)
	}
	if review.SampleBasis != quality.SampleBasis {
		return fmt.Errorf("review evidence sample_basis %q must match quality comparison sample_basis %q", review.SampleBasis, quality.SampleBasis)
	}
	decision, err := parseDecisionFile(actualArtifacts.Decision)
	if err != nil {
		return fmt.Errorf("decision: %w", err)
	}
	if packet.Decision != decision.Decision {
		return fmt.Errorf("packet decision = %q, want decision artifact %q", packet.Decision, decision.Decision)
	}
	if packet.ReviewRequired != decision.ReviewRequired {
		return fmt.Errorf("packet review_required = %v, want decision artifact %v", packet.ReviewRequired, decision.ReviewRequired)
	}
	if err := validateDecisionEvidenceMatchesArtifacts(decision, actualArtifacts, minCases); err != nil {
		return err
	}
	if err := verifyPacketQualityInputs(root, packet.QualityInputs, quality); err != nil {
		return err
	}
	if err := verifyPacketRuntimeSmokeArtifacts(root, packet.RuntimeSmokeArtifacts, review.RuntimeSmokes); err != nil {
		return err
	}
	return nil
}

func verifyPacketInventory(root string, packet packetOutput) error {
	expectedFiles, expectedDirs, err := expectedPacketInventory(packet)
	if err != nil {
		return err
	}
	seenFiles := map[string]bool{}
	err = filepath.WalkDir(root, func(currentPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, currentPath)
		if err != nil {
			return err
		}
		ref := filepath.ToSlash(rel)
		if ref == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("packet contains symlink %q", ref)
		}
		if entry.IsDir() {
			if !expectedDirs[ref] {
				return fmt.Errorf("packet contains unexpected directory %q", ref)
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat packet entry %q: %w", ref, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("packet contains non-regular file %q", ref)
		}
		if !expectedFiles[ref] {
			return fmt.Errorf("packet contains unexpected file %q", ref)
		}
		seenFiles[ref] = true
		return nil
	})
	if err != nil {
		return err
	}
	for ref := range expectedFiles {
		if !seenFiles[ref] {
			return fmt.Errorf("packet expected file %q is missing", ref)
		}
	}
	return nil
}

func expectedPacketInventory(packet packetOutput) (map[string]bool, map[string]bool, error) {
	expectedFiles := map[string]bool{}
	for field, ref := range map[string]string{
		"artifacts.baseline_audit":     packet.Artifacts.BaselineAudit,
		"artifacts.quality_comparison": packet.Artifacts.QualityComparison,
		"artifacts.review_evidence":    packet.Artifacts.ReviewEvidence,
		"artifacts.decision":           packet.Artifacts.Decision,
		"artifacts.packet":             packet.Artifacts.Packet,
		"quality_inputs.manifest.path": packet.QualityInputs.Manifest.Path,
	} {
		if err := rememberExpectedPacketFile(expectedFiles, field, ref); err != nil {
			return nil, nil, err
		}
	}
	for i, artifact := range packet.QualityInputs.Reports {
		if err := rememberExpectedPacketFile(expectedFiles, fmt.Sprintf("quality_inputs.reports[%d].path", i), artifact.Path); err != nil {
			return nil, nil, err
		}
	}
	for i, artifact := range packet.RuntimeSmokeArtifacts {
		if err := rememberExpectedPacketFile(expectedFiles, fmt.Sprintf("runtime_smoke_artifacts[%d].path", i), artifact.Path); err != nil {
			return nil, nil, err
		}
	}
	expectedDirs := map[string]bool{}
	for ref := range expectedFiles {
		addExpectedPacketDirs(expectedDirs, ref)
	}
	return expectedFiles, expectedDirs, nil
}

func rememberExpectedPacketFile(expected map[string]bool, field, ref string) error {
	if err := validatePacketPathRef(field, ref); err != nil {
		return err
	}
	if expected[ref] {
		return fmt.Errorf("packet file ref %q is referenced more than once", ref)
	}
	expected[ref] = true
	return nil
}

func addExpectedPacketDirs(expected map[string]bool, fileRef string) {
	dir := path.Dir(fileRef)
	for dir != "." {
		expected[dir] = true
		next := path.Dir(dir)
		if next == dir {
			return
		}
		dir = next
	}
}

func resolvePacketArtifactPaths(root string, artifacts packetArtifacts) (packetArtifacts, error) {
	baseline, err := resolvePacketPath(root, "artifacts.baseline_audit", artifacts.BaselineAudit)
	if err != nil {
		return packetArtifacts{}, err
	}
	quality, err := resolvePacketPath(root, "artifacts.quality_comparison", artifacts.QualityComparison)
	if err != nil {
		return packetArtifacts{}, err
	}
	review, err := resolvePacketPath(root, "artifacts.review_evidence", artifacts.ReviewEvidence)
	if err != nil {
		return packetArtifacts{}, err
	}
	decision, err := resolvePacketPath(root, "artifacts.decision", artifacts.Decision)
	if err != nil {
		return packetArtifacts{}, err
	}
	packet, err := resolvePacketPath(root, "artifacts.packet", artifacts.Packet)
	if err != nil {
		return packetArtifacts{}, err
	}
	return packetArtifacts{
		BaselineAudit:     baseline,
		QualityComparison: quality,
		ReviewEvidence:    review,
		Decision:          decision,
		Packet:            packet,
	}, nil
}

func verifyPacketCommands(packet packetOutput) (int, error) {
	if len(packet.Commands) != 3 {
		return 0, fmt.Errorf("packet commands contains %d items, want 3", len(packet.Commands))
	}
	var minCases int
	seen := map[string]bool{}
	for i, command := range packet.Commands {
		if command.Status != "pass" {
			return 0, fmt.Errorf("packet commands[%d].status = %q, want pass", i, command.Status)
		}
		if err := validateCommandSpec(commandSpec{Name: command.Name, Args: command.Args}); err != nil {
			return 0, fmt.Errorf("packet commands[%d]: %w", i, err)
		}
		if err := validatePacketPathRef(fmt.Sprintf("packet commands[%d].output_path", i), command.OutputPath); err != nil {
			return 0, err
		}
		helper := command.Args[1]
		if seen[helper] {
			return 0, fmt.Errorf("packet commands contains duplicate helper %q", helper)
		}
		seen[helper] = true
		switch helper {
		case "./scripts/sandbox_baseline_audit":
			if command.OutputPath != packet.Artifacts.BaselineAudit {
				return 0, fmt.Errorf("baseline command output_path = %q, want %q", command.OutputPath, packet.Artifacts.BaselineAudit)
			}
		case "./scripts/sandbox_quality_compare":
			if command.OutputPath != packet.Artifacts.QualityComparison {
				return 0, fmt.Errorf("quality command output_path = %q, want %q", command.OutputPath, packet.Artifacts.QualityComparison)
			}
			manifest, err := commandFlagValue(command.Args, "--manifest")
			if err != nil {
				return 0, fmt.Errorf("quality command: %w", err)
			}
			if manifest != packet.QualityInputs.Manifest.Path {
				return 0, fmt.Errorf("quality command --manifest = %q, want %q", manifest, packet.QualityInputs.Manifest.Path)
			}
			if !containsString(command.Args, "--fail-on-regression") {
				return 0, errors.New("quality command must include --fail-on-regression")
			}
		case "./scripts/sandbox_m4_decision":
			if command.OutputPath != packet.Artifacts.Decision {
				return 0, fmt.Errorf("decision command output_path = %q, want %q", command.OutputPath, packet.Artifacts.Decision)
			}
			if err := verifyDecisionCommandInputs(command.Args, packet.Artifacts); err != nil {
				return 0, err
			}
			rawMinCases, err := commandFlagValue(command.Args, "--min-cases")
			if err != nil {
				return 0, fmt.Errorf("decision command: %w", err)
			}
			value, err := strconv.Atoi(rawMinCases)
			if err != nil || value <= 0 {
				return 0, fmt.Errorf("decision command --min-cases = %q, want positive integer", rawMinCases)
			}
			minCases = value
		}
	}
	for _, helper := range []string{"./scripts/sandbox_baseline_audit", "./scripts/sandbox_quality_compare", "./scripts/sandbox_m4_decision"} {
		if !seen[helper] {
			return 0, fmt.Errorf("packet commands missing helper %q", helper)
		}
	}
	if minCases <= 0 {
		return 0, errors.New("packet commands missing decision --min-cases")
	}
	return minCases, nil
}

func verifyDecisionCommandInputs(args []string, artifacts packetArtifacts) error {
	wants := map[string]string{
		"--baseline-audit":     artifacts.BaselineAudit,
		"--quality-comparison": artifacts.QualityComparison,
		"--review-evidence":    artifacts.ReviewEvidence,
	}
	for flag, want := range wants {
		got, err := commandFlagValue(args, flag)
		if err != nil {
			return fmt.Errorf("decision command: %w", err)
		}
		if got != want {
			return fmt.Errorf("decision command %s = %q, want %q", flag, got, want)
		}
	}
	return nil
}

func commandFlagValue(args []string, flag string) (string, error) {
	for i, arg := range args {
		if arg == flag {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", fmt.Errorf("%s requires a value", flag)
			}
			return args[i+1], nil
		}
	}
	return "", fmt.Errorf("%s is required", flag)
}

func verifyPacketCoreArtifacts(artifacts packetArtifacts, want packetArtifactDigests) error {
	if err := validateArtifactDigest("baseline audit", artifacts.BaselineAudit, want.BaselineAudit); err != nil {
		return err
	}
	if err := validateArtifactDigest("quality comparison", artifacts.QualityComparison, want.QualityComparison); err != nil {
		return err
	}
	if err := validateArtifactDigest("review evidence", artifacts.ReviewEvidence, want.ReviewEvidence); err != nil {
		return err
	}
	if err := validateArtifactDigest("decision", artifacts.Decision, want.Decision); err != nil {
		return err
	}
	baselineRaw, err := readFileCapped(artifacts.BaselineAudit)
	if err != nil {
		return fmt.Errorf("read baseline audit: %w", err)
	}
	if err := validateBaselineOutput(baselineRaw); err != nil {
		return err
	}
	qualityRaw, err := readFileCapped(artifacts.QualityComparison)
	if err != nil {
		return fmt.Errorf("read quality comparison: %w", err)
	}
	if err := validateQualityOutput(qualityRaw); err != nil {
		return err
	}
	reviewRaw, err := readFileCapped(artifacts.ReviewEvidence)
	if err != nil {
		return fmt.Errorf("read review evidence: %w", err)
	}
	if err := validateReviewEvidence(reviewRaw); err != nil {
		return err
	}
	decisionRaw, err := readFileCapped(artifacts.Decision)
	if err != nil {
		return fmt.Errorf("read decision: %w", err)
	}
	if err := validateDecisionOutput(decisionRaw); err != nil {
		return err
	}
	return nil
}

func validateArtifactDigest(label, filePath, want string) error {
	if !imageDigestHexRe.MatchString(want) {
		return fmt.Errorf("%s sha256 %q must be 64 lowercase hex characters", label, want)
	}
	got, err := fileSHA256Hex(filePath)
	if err != nil {
		return fmt.Errorf("hash %s: %w", label, err)
	}
	if got != want {
		return fmt.Errorf("%s sha256 = %q, want %q", label, got, want)
	}
	return nil
}

func verifyPacketQualityInputs(root string, inputs packetQualityInputs, quality helperQualityComparison) error {
	if inputs.Manifest.Path != qualityManifestRef {
		return fmt.Errorf("quality input manifest path = %q, want %q", inputs.Manifest.Path, qualityManifestRef)
	}
	manifestPath, err := resolvePacketPath(root, "quality_inputs.manifest.path", inputs.Manifest.Path)
	if err != nil {
		return err
	}
	if err := validateArtifactDigest("quality input manifest", manifestPath, inputs.Manifest.SHA256); err != nil {
		return err
	}
	rawManifest, err := readFileCapped(manifestPath)
	if err != nil {
		return fmt.Errorf("read quality input manifest: %w", err)
	}
	manifest, err := parseQualityManifestRaw(rawManifest, manifestPath)
	if err != nil {
		return err
	}
	if err := validateQualityManifestMatchesComparison(manifest, quality); err != nil {
		return err
	}
	expectedReports := expectedQualityReportArtifacts(manifest)
	if len(inputs.Reports) != len(expectedReports) {
		return fmt.Errorf("quality input reports contains %d items, want %d", len(inputs.Reports), len(expectedReports))
	}
	seen := map[string]bool{}
	for _, artifact := range inputs.Reports {
		key := artifact.CaseID + "\x00" + artifact.Role
		expected, ok := expectedReports[key]
		if !ok {
			return fmt.Errorf("quality input report case=%q role=%q is not declared by quality manifest", artifact.CaseID, artifact.Role)
		}
		if seen[key] {
			return fmt.Errorf("duplicate quality input report case=%q role=%q", artifact.CaseID, artifact.Role)
		}
		seen[key] = true
		if artifact.ManifestRef != expected.ManifestRef {
			return fmt.Errorf("quality input report case=%q role=%q manifest_ref = %q, want %q", artifact.CaseID, artifact.Role, artifact.ManifestRef, expected.ManifestRef)
		}
		if artifact.Path != expected.Path {
			return fmt.Errorf("quality input report case=%q role=%q path = %q, want %q", artifact.CaseID, artifact.Role, artifact.Path, expected.Path)
		}
		reportPath, err := resolvePacketPath(root, fmt.Sprintf("quality input report %q %s path", artifact.CaseID, artifact.Role), artifact.Path)
		if err != nil {
			return err
		}
		if err := validateArtifactDigest(fmt.Sprintf("quality input report %q %s", artifact.CaseID, artifact.Role), reportPath, artifact.SHA256); err != nil {
			return err
		}
		report, _, err := parseQualityInputSubReportFile(reportPath)
		if err != nil {
			return fmt.Errorf("quality input report %q %s: %w", artifact.CaseID, artifact.Role, err)
		}
		if err := validateQualityInputReportEvidenceRefs(artifact.CaseID, artifact.Role, expected.RequiredEvidenceRefs, report); err != nil {
			return err
		}
	}
	for key, expected := range expectedReports {
		if !seen[key] {
			return fmt.Errorf("quality input report case=%q role=%q is missing", expected.CaseID, expected.Role)
		}
	}
	return nil
}

func parseQualityManifestRaw(raw []byte, label string) (packetQualityManifest, error) {
	var manifest packetQualityManifest
	if err := strictjson.Unmarshal(raw, &manifest); err != nil {
		return packetQualityManifest{}, fmt.Errorf("parse quality manifest %s: %w", filepath.Clean(label), err)
	}
	return normalizeQualityManifest(manifest)
}

type expectedQualityReport struct {
	CaseID               string
	Role                 string
	ManifestRef          string
	Path                 string
	RequiredEvidenceRefs []string
}

func expectedQualityReportArtifacts(manifest packetQualityManifest) map[string]expectedQualityReport {
	expected := make(map[string]expectedQualityReport, len(manifest.Cases)*2)
	for _, item := range manifest.Cases {
		expected[item.ID+"\x00direct"] = expectedQualityReport{
			CaseID:               item.ID,
			Role:                 "direct",
			ManifestRef:          item.DirectSubReport,
			Path:                 path.Join(qualityReportsDir, item.DirectSubReport),
			RequiredEvidenceRefs: append([]string(nil), item.RequiredEvidenceRefs...),
		}
		expected[item.ID+"\x00sandbox"] = expectedQualityReport{
			CaseID:               item.ID,
			Role:                 "sandbox",
			ManifestRef:          item.SandboxSubReport,
			Path:                 path.Join(qualityReportsDir, item.SandboxSubReport),
			RequiredEvidenceRefs: append([]string(nil), item.RequiredEvidenceRefs...),
		}
	}
	return expected
}

func verifyPacketRuntimeSmokeArtifacts(root string, artifacts []packetRuntimeSmokeArtifact, smokes []packetRuntimeSmoke) error {
	if len(artifacts) != len(smokes) {
		return fmt.Errorf("runtime smoke artifacts contains %d items, want %d", len(artifacts), len(smokes))
	}
	expected := make(map[string]packetRuntimeSmoke, len(smokes))
	for _, smoke := range smokes {
		expected[smoke.Name] = smoke
	}
	seen := map[string]bool{}
	for _, artifact := range artifacts {
		smoke, ok := expected[artifact.Name]
		if !ok {
			return fmt.Errorf("runtime smoke artifact %q is not declared by review evidence", artifact.Name)
		}
		if seen[artifact.Name] {
			return fmt.Errorf("duplicate runtime smoke artifact %q", artifact.Name)
		}
		seen[artifact.Name] = true
		if artifact.EvidenceRef != smoke.EvidenceRef {
			return fmt.Errorf("runtime smoke artifact %q evidence_ref = %q, want %q", artifact.Name, artifact.EvidenceRef, smoke.EvidenceRef)
		}
		if artifact.EvidenceSHA256 != smoke.EvidenceSHA256 {
			return fmt.Errorf("runtime smoke artifact %q evidence_sha256 = %q, want review evidence %q", artifact.Name, artifact.EvidenceSHA256, smoke.EvidenceSHA256)
		}
		wantPath := path.Join(runtimeSmokeArtifactsDir, smoke.EvidenceRef)
		if artifact.Path != wantPath {
			return fmt.Errorf("runtime smoke artifact %q path = %q, want %q", artifact.Name, artifact.Path, wantPath)
		}
		artifactPath, err := resolvePacketPath(root, fmt.Sprintf("runtime smoke artifact %q path", artifact.Name), artifact.Path)
		if err != nil {
			return err
		}
		if err := validateArtifactDigest(fmt.Sprintf("runtime smoke artifact %q", artifact.Name), artifactPath, artifact.EvidenceSHA256); err != nil {
			return err
		}
	}
	for _, smoke := range smokes {
		if !seen[smoke.Name] {
			return fmt.Errorf("runtime smoke artifact %q is missing", smoke.Name)
		}
	}
	return nil
}

func resolvePacketPath(root, field, ref string) (string, error) {
	if err := validatePacketPathRef(field, ref); err != nil {
		return "", err
	}
	return joinUnderRoot(root, ref)
}

func validatePacketPathRef(field, ref string) error {
	value := strings.TrimSpace(ref)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if value != ref {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(ref, "\r\n\t") {
		return fmt.Errorf("%s must be a single-line path", field)
	}
	if strings.Contains(ref, "\\") || path.IsAbs(ref) {
		return fmt.Errorf("%s must be a slash-separated relative path", field)
	}
	clean := path.Clean(ref)
	if clean != ref || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s must be a normalized relative packet path", field)
	}
	return nil
}

func copyQualityInputs(manifestPath, outDir string, quality helperQualityComparison) (packetQualityInputs, error) {
	rawManifest, manifest, manifestBaseDir, err := parseQualityManifestFile(manifestPath)
	if err != nil {
		return packetQualityInputs{}, err
	}
	if err := validateQualityManifestMatchesComparison(manifest, quality); err != nil {
		return packetQualityInputs{}, err
	}
	cleanOutDir, err := filepath.Abs(filepath.Clean(outDir))
	if err != nil {
		return packetQualityInputs{}, fmt.Errorf("resolve output directory: %w", err)
	}
	manifestDestination, err := joinUnderRoot(cleanOutDir, qualityManifestRef)
	if err != nil {
		return packetQualityInputs{}, fmt.Errorf("quality manifest packet path %q: %w", qualityManifestRef, err)
	}
	if err := writePacketBytes(manifestDestination, rawManifest); err != nil {
		return packetQualityInputs{}, fmt.Errorf("copy quality manifest: %w", err)
	}
	manifestDigest, err := fileSHA256Hex(manifestDestination)
	if err != nil {
		return packetQualityInputs{}, fmt.Errorf("hash copied quality manifest: %w", err)
	}
	inputs := packetQualityInputs{
		Manifest: packetInputArtifact{
			Path:   qualityManifestRef,
			SHA256: manifestDigest,
		},
		Reports: make([]packetQualityReportArtifact, 0, len(manifest.Cases)*2),
	}
	for _, item := range manifest.Cases {
		direct, err := copyQualityReportInput(manifestBaseDir, cleanOutDir, item.ID, "direct", item.DirectSubReport, item.RequiredEvidenceRefs)
		if err != nil {
			return packetQualityInputs{}, err
		}
		inputs.Reports = append(inputs.Reports, direct)
		sandbox, err := copyQualityReportInput(manifestBaseDir, cleanOutDir, item.ID, "sandbox", item.SandboxSubReport, item.RequiredEvidenceRefs)
		if err != nil {
			return packetQualityInputs{}, err
		}
		inputs.Reports = append(inputs.Reports, sandbox)
	}
	return inputs, nil
}

func parseQualityManifestFile(manifestPath string) ([]byte, packetQualityManifest, string, error) {
	raw, err := readFileCapped(manifestPath)
	if err != nil {
		return nil, packetQualityManifest{}, "", fmt.Errorf("read quality manifest: %w", err)
	}
	var manifest packetQualityManifest
	if err := strictjson.Unmarshal(raw, &manifest); err != nil {
		return nil, packetQualityManifest{}, "", fmt.Errorf("parse quality manifest %s: %w", filepath.Clean(manifestPath), err)
	}
	normalized, err := normalizeQualityManifest(manifest)
	if err != nil {
		return nil, packetQualityManifest{}, "", err
	}
	baseDir, err := filepath.Abs(filepath.Dir(filepath.Clean(manifestPath)))
	if err != nil {
		return nil, packetQualityManifest{}, "", fmt.Errorf("resolve quality manifest directory: %w", err)
	}
	return raw, normalized, baseDir, nil
}

func normalizeQualityManifest(manifest packetQualityManifest) (packetQualityManifest, error) {
	if err := validateQualityManifestSampleBasis(manifest.SampleBasis); err != nil {
		return packetQualityManifest{}, err
	}
	if len(manifest.Cases) == 0 {
		return packetQualityManifest{}, errors.New("quality manifest cases must contain at least one item")
	}
	if len(manifest.Cases) > maxQualityManifestCases {
		return packetQualityManifest{}, fmt.Errorf("quality manifest cases contains %d items, max %d", len(manifest.Cases), maxQualityManifestCases)
	}
	seenIDs := map[string]bool{}
	seenReportRefs := map[string]string{}
	for i := range manifest.Cases {
		item := &manifest.Cases[i]
		id, err := validateEvidenceCaseID(fmt.Sprintf("quality manifest cases[%d].id", i), item.ID)
		if err != nil {
			return packetQualityManifest{}, err
		}
		if seenIDs[id] {
			return packetQualityManifest{}, fmt.Errorf("duplicate quality manifest case id %q", id)
		}
		seenIDs[id] = true
		scenario := strings.TrimSpace(item.Scenario)
		if scenario == "" {
			return packetQualityManifest{}, fmt.Errorf("quality manifest case %q scenario is required", id)
		}
		if scenario != item.Scenario {
			return packetQualityManifest{}, fmt.Errorf("quality manifest case %q scenario must not contain leading or trailing whitespace", id)
		}
		if !reportprompt.Scenario(scenario).Valid() {
			return packetQualityManifest{}, fmt.Errorf("quality manifest case %q scenario %q is unsupported", id, scenario)
		}
		if err := validateQualityManifestRequiredEvidenceRefs(id, item.RequiredEvidenceRefs); err != nil {
			return packetQualityManifest{}, err
		}
		directRef, err := validateQualityManifestReportRef(id, "direct_sub_report", item.DirectSubReport)
		if err != nil {
			return packetQualityManifest{}, err
		}
		sandboxRef, err := validateQualityManifestReportRef(id, "sandbox_sub_report", item.SandboxSubReport)
		if err != nil {
			return packetQualityManifest{}, err
		}
		if directRef == sandboxRef {
			return packetQualityManifest{}, fmt.Errorf("quality manifest case %q direct_sub_report and sandbox_sub_report must be distinct files", id)
		}
		if err := rememberQualityReportRef(seenReportRefs, id, "direct_sub_report", directRef); err != nil {
			return packetQualityManifest{}, err
		}
		if err := rememberQualityReportRef(seenReportRefs, id, "sandbox_sub_report", sandboxRef); err != nil {
			return packetQualityManifest{}, err
		}
		item.DirectSubReport = directRef
		item.SandboxSubReport = sandboxRef
	}
	return manifest, nil
}

func validateQualityManifestSampleBasis(raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return errors.New("quality manifest sample_basis must not contain leading or trailing whitespace")
	}
	if value == "" {
		return errors.New("quality manifest sample_basis is required")
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return errors.New("quality manifest sample_basis must be a single-line value")
	}
	if len(raw) > maxReviewEvidenceTextBytes {
		return fmt.Errorf("quality manifest sample_basis exceeds %d bytes", maxReviewEvidenceTextBytes)
	}
	return nil
}

func validateQualityManifestRequiredEvidenceRefs(caseID string, refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("quality manifest case %q required_evidence_refs must contain at least one item", caseID)
	}
	if len(refs) > maxQualityManifestRequiredRefs {
		return fmt.Errorf("quality manifest case %q required_evidence_refs contains %d items, max %d", caseID, len(refs), maxQualityManifestRequiredRefs)
	}
	if err := validateQualityRequiredEvidenceRefs(caseID, refs); err != nil {
		return stringsReplaceError(err, "quality comparison", "quality manifest")
	}
	return nil
}

func validateQualityManifestReportRef(caseID, field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("quality manifest case %q %s is required", caseID, field)
	}
	if value != raw {
		return "", fmt.Errorf("quality manifest case %q %s must not contain leading or trailing whitespace", caseID, field)
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return "", fmt.Errorf("quality manifest case %q %s must be a single-line slash-separated relative path", caseID, field)
	}
	if len(value) > maxQualityManifestReportPathBytes {
		return "", fmt.Errorf("quality manifest case %q %s exceeds %d bytes", caseID, field, maxQualityManifestReportPathBytes)
	}
	if strings.ContainsAny(value, "\\:") {
		return "", fmt.Errorf("quality manifest case %q %s must be a slash-separated relative path", caseID, field)
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("quality manifest case %q %s must be relative", caseID, field)
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", fmt.Errorf("quality manifest case %q %s must not contain parent directory traversal", caseID, field)
		}
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("quality manifest case %q %s must point to a file under the manifest directory", caseID, field)
	}
	if clean != value {
		return "", fmt.Errorf("quality manifest case %q %s must be a normalized slash-separated relative path", caseID, field)
	}
	return clean, nil
}

func rememberQualityReportRef(seen map[string]string, caseID, field, ref string) error {
	owner := caseID + "." + field
	if previous, ok := seen[ref]; ok {
		return fmt.Errorf("quality manifest case %q %s repeats report path %q already used by %s", caseID, field, ref, previous)
	}
	seen[ref] = owner
	return nil
}

func validateQualityManifestMatchesComparison(manifest packetQualityManifest, quality helperQualityComparison) error {
	if manifest.SampleBasis != quality.SampleBasis {
		return fmt.Errorf("quality manifest sample_basis %q must match quality comparison sample_basis %q", manifest.SampleBasis, quality.SampleBasis)
	}
	if len(manifest.Cases) != quality.CaseCount {
		return fmt.Errorf("quality manifest case count = %d, want quality comparison case_count %d", len(manifest.Cases), quality.CaseCount)
	}
	if len(quality.Cases) != len(manifest.Cases) {
		return fmt.Errorf("quality comparison cases = %d, want quality manifest case count %d", len(quality.Cases), len(manifest.Cases))
	}
	for i, manifestCase := range manifest.Cases {
		qualityCase := quality.Cases[i]
		if manifestCase.ID != qualityCase.ID {
			return fmt.Errorf("quality manifest cases[%d].id = %q, want quality comparison case id %q", i, manifestCase.ID, qualityCase.ID)
		}
		if manifestCase.Scenario != qualityCase.Scenario {
			return fmt.Errorf("quality manifest case %q scenario = %q, want quality comparison scenario %q", manifestCase.ID, manifestCase.Scenario, qualityCase.Scenario)
		}
		if !equalStrings(manifestCase.RequiredEvidenceRefs, qualityCase.RequiredEvidenceRefs) {
			return fmt.Errorf("quality manifest case %q required_evidence_refs = %v, want quality comparison refs %v", manifestCase.ID, manifestCase.RequiredEvidenceRefs, qualityCase.RequiredEvidenceRefs)
		}
	}
	return nil
}

func copyQualityReportInput(manifestBaseDir, outDir, caseID, role, manifestRef string, requiredRefs []string) (packetQualityReportArtifact, error) {
	sourcePath, err := joinUnderRoot(manifestBaseDir, manifestRef)
	if err != nil {
		return packetQualityReportArtifact{}, fmt.Errorf("quality input case %q %s_sub_report %q: %w", caseID, role, manifestRef, err)
	}
	report, raw, err := parseQualityInputSubReportFile(sourcePath)
	if err != nil {
		return packetQualityReportArtifact{}, fmt.Errorf("quality input case %q %s_sub_report %q: %w", caseID, role, manifestRef, err)
	}
	if err := validateQualityInputReportEvidenceRefs(caseID, role, requiredRefs, report); err != nil {
		return packetQualityReportArtifact{}, err
	}
	destinationRef := path.Join(qualityReportsDir, manifestRef)
	destinationPath, err := joinUnderRoot(outDir, destinationRef)
	if err != nil {
		return packetQualityReportArtifact{}, fmt.Errorf("quality input case %q %s packet path %q: %w", caseID, role, destinationRef, err)
	}
	if err := writePacketBytes(destinationPath, raw); err != nil {
		return packetQualityReportArtifact{}, fmt.Errorf("copy quality input case %q %s_sub_report %q: %w", caseID, role, manifestRef, err)
	}
	digest, err := fileSHA256Hex(destinationPath)
	if err != nil {
		return packetQualityReportArtifact{}, fmt.Errorf("hash copied quality input case %q %s_sub_report %q: %w", caseID, role, manifestRef, err)
	}
	return packetQualityReportArtifact{
		CaseID:      caseID,
		Role:        role,
		ManifestRef: manifestRef,
		Path:        destinationRef,
		SHA256:      digest,
	}, nil
}

func parseQualityInputSubReportFile(path string) (reportdraft.SubReport, []byte, error) {
	raw, err := readFileCapped(path)
	if err != nil {
		return reportdraft.SubReport{}, nil, err
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return reportdraft.SubReport{}, nil, fmt.Errorf("parse: %w", err)
	}
	report, err := reportdraft.ParseSubReport(ports.LLMResponse{
		Content:      json.RawMessage(raw),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "offline-quality-input-freeze",
	})
	if err != nil {
		return reportdraft.SubReport{}, nil, err
	}
	return report, raw, nil
}

func validateQualityInputReportEvidenceRefs(caseID, role string, requiredRefs []string, report reportdraft.SubReport) error {
	refs := qualityInputEvidenceRefSet(report)
	for _, ref := range requiredRefs {
		if _, ok := refs[ref]; !ok {
			return fmt.Errorf("quality input case %q %s subreport missing required evidence ref %q", caseID, role, ref)
		}
	}
	return nil
}

func qualityInputEvidenceRefSet(report reportdraft.SubReport) map[string]struct{} {
	refs := make(map[string]struct{}, len(report.EvidenceRefs)+len(report.Findings))
	for _, ref := range report.EvidenceRefs {
		refs[ref] = struct{}{}
	}
	for _, finding := range report.Findings {
		refs[finding.EvidenceID] = struct{}{}
	}
	return refs
}

func writePacketBytes(dst string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	// #nosec G304 -- destination is constrained to the empty packet output
	// directory by joinUnderRoot and opened with O_EXCL.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	if _, err := out.Write(raw); err != nil {
		_ = out.Close()
		return fmt.Errorf("write bytes: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination: %w", err)
	}
	return nil
}

func stringsReplaceError(err error, oldValue, newValue string) error {
	if err == nil {
		return nil
	}
	return errors.New(strings.ReplaceAll(err.Error(), oldValue, newValue))
}

func copyRuntimeSmokeArtifacts(root, outDir string, smokes []packetRuntimeSmoke) ([]packetRuntimeSmokeArtifact, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("resolve runtime smoke artifacts root: %w", err)
	}
	cleanOutDir, err := filepath.Abs(filepath.Clean(outDir))
	if err != nil {
		return nil, fmt.Errorf("resolve output directory: %w", err)
	}
	copied := make([]packetRuntimeSmokeArtifact, 0, len(smokes))
	for _, smoke := range smokes {
		sourcePath, err := joinUnderRoot(cleanRoot, smoke.EvidenceRef)
		if err != nil {
			return nil, fmt.Errorf("runtime smoke %q evidence_ref %q: %w", smoke.Name, smoke.EvidenceRef, err)
		}
		gotDigest, err := fileSHA256Hex(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("hash runtime smoke %q artifact %q: %w", smoke.Name, smoke.EvidenceRef, err)
		}
		if gotDigest != smoke.EvidenceSHA256 {
			return nil, fmt.Errorf("runtime smoke %q artifact %q sha256 = %q, want %q", smoke.Name, smoke.EvidenceRef, gotDigest, smoke.EvidenceSHA256)
		}
		destinationRef := path.Join(runtimeSmokeArtifactsDir, smoke.EvidenceRef)
		destinationPath, err := joinUnderRoot(cleanOutDir, destinationRef)
		if err != nil {
			return nil, fmt.Errorf("runtime smoke %q packet path %q: %w", smoke.Name, destinationRef, err)
		}
		if err := copyFile(sourcePath, destinationPath); err != nil {
			return nil, fmt.Errorf("copy runtime smoke %q artifact %q: %w", smoke.Name, smoke.EvidenceRef, err)
		}
		copiedDigest, err := fileSHA256Hex(destinationPath)
		if err != nil {
			return nil, fmt.Errorf("hash copied runtime smoke %q artifact %q: %w", smoke.Name, destinationRef, err)
		}
		if copiedDigest != smoke.EvidenceSHA256 {
			return nil, fmt.Errorf("copied runtime smoke %q artifact %q sha256 = %q, want %q", smoke.Name, destinationRef, copiedDigest, smoke.EvidenceSHA256)
		}
		copied = append(copied, packetRuntimeSmokeArtifact{
			Name:           smoke.Name,
			EvidenceRef:    smoke.EvidenceRef,
			Path:           destinationRef,
			EvidenceSHA256: smoke.EvidenceSHA256,
		})
	}
	return copied, nil
}

func joinUnderRoot(root, slashRef string) (string, error) {
	pathValue := filepath.Clean(filepath.Join(root, filepath.FromSlash(slashRef)))
	rel, err := filepath.Rel(root, pathValue)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", errors.New("path escapes root")
	}
	return pathValue, nil
}

func copyFile(src, dst string) error {
	if err := requireRegularFile(src); err != nil {
		return err
	}
	// #nosec G304 -- source and destination paths are derived from already
	// validated relative evidence refs and guarded by joinUnderRoot.
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	// #nosec G304 -- destination is constrained to the empty packet output
	// directory by joinUnderRoot and opened with O_EXCL.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	n, copyErr := io.Copy(out, io.LimitReader(in, maxInputBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("copy bytes: %w", copyErr)
	}
	if n > maxInputBytes {
		_ = os.Remove(dst)
		return fmt.Errorf("%s exceeds %d bytes", src, maxInputBytes)
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("close destination: %w", closeErr)
	}
	return nil
}

func fileSHA256Hex(path string) (string, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return "", err
	}
	// #nosec G304 -- this packet helper intentionally hashes
	// repository-owned local evidence artifacts it just created or copied.
	f, err := os.Open(clean)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", clean, err)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxInputBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", clean, err)
	}
	if n > maxInputBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", clean, maxInputBytes)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (execRunner) Run(ctx context.Context, spec commandSpec) ([]byte, []byte, error) {
	if err := validateCommandSpec(spec); err != nil {
		return nil, nil, err
	}
	// #nosec G204 -- command specs are constructed internally and restricted
	// to the repository-owned Go helper commands by validateCommandSpec.
	cmd := exec.CommandContext(ctx, "go", spec.Args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func validateCommandSpec(spec commandSpec) error {
	if spec.Name != "go" {
		return fmt.Errorf("unsupported command %q", spec.Name)
	}
	if len(spec.Args) < 2 || spec.Args[0] != "run" {
		return fmt.Errorf("unsupported go args %v", spec.Args)
	}
	switch spec.Args[1] {
	case "./scripts/sandbox_baseline_audit",
		"./scripts/sandbox_quality_compare",
		"./scripts/sandbox_m4_decision":
		return nil
	default:
		return fmt.Errorf("unsupported helper package %q", spec.Args[1])
	}
}

func prepareOutDir(path string) (string, error) {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return "", fmt.Errorf("refusing output directory %q", path)
	}
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return "", fmt.Errorf("read output directory: %w", err)
	}
	if len(entries) > 0 {
		return "", fmt.Errorf("output directory %q must be empty", clean)
	}
	return clean, nil
}

func runCommandToFile(ctx context.Context, runner commandRunner, spec commandSpec, outputPath string, timeout time.Duration) error {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, stderr, err := runner.Run(commandCtx, spec)
	if err != nil {
		if strings.TrimSpace(string(stderr)) != "" {
			return fmt.Errorf("%s %s: %w: %s", spec.Name, strings.Join(spec.Args, " "), err, strings.TrimSpace(string(stderr)))
		}
		return fmt.Errorf("%s %s: %w", spec.Name, strings.Join(spec.Args, " "), err)
	}
	if len(stdout) == 0 {
		return fmt.Errorf("%s %s produced empty stdout", spec.Name, strings.Join(spec.Args, " "))
	}
	if err := validateHelperOutput(spec, stdout); err != nil {
		return fmt.Errorf("%s %s produced invalid artifact: %w", spec.Name, strings.Join(spec.Args, " "), err)
	}
	if err := os.WriteFile(outputPath, stdout, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}
	return nil
}

func validateHelperOutput(spec commandSpec, raw []byte) error {
	if err := validateCommandSpec(spec); err != nil {
		return err
	}
	switch spec.Args[1] {
	case "./scripts/sandbox_baseline_audit":
		return validateBaselineOutput(raw)
	case "./scripts/sandbox_quality_compare":
		return validateQualityOutput(raw)
	case "./scripts/sandbox_m4_decision":
		return validateDecisionOutput(raw)
	default:
		return fmt.Errorf("unsupported helper package %q", spec.Args[1])
	}
}

func validateBaselineOutput(raw []byte) error {
	var out helperBaselineAudit
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("parse baseline audit output: %w", err)
	}
	if out.Tool != "sandbox_baseline_audit" {
		return fmt.Errorf("baseline audit tool = %q, want sandbox_baseline_audit", out.Tool)
	}
	if out.Status != "pass" {
		return fmt.Errorf("baseline audit status = %q, want pass", out.Status)
	}
	if len(out.Checks) == 0 {
		return errors.New("baseline audit checks must not be empty")
	}
	checks := map[string]string{}
	for i, check := range out.Checks {
		name := strings.TrimSpace(check.Name)
		if name == "" {
			return fmt.Errorf("baseline audit checks[%d].name is required", i)
		}
		if name != check.Name {
			return fmt.Errorf("baseline audit check name %q must not contain leading or trailing whitespace", check.Name)
		}
		if _, ok := checks[name]; ok {
			return fmt.Errorf("duplicate baseline audit check %q", name)
		}
		if check.Status != "pass" {
			return fmt.Errorf("baseline audit check %q status = %q, want pass", check.Name, check.Status)
		}
		checks[name] = check.Status
	}
	for _, name := range requiredBaselineCheckNames {
		if checks[name] != "pass" {
			return fmt.Errorf("baseline audit missing required pass check %q", name)
		}
	}
	return nil
}

func validateQualityOutput(raw []byte) error {
	var out helperQualityComparison
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("parse quality comparison output: %w", err)
	}
	if out.Tool != "sandbox_quality_compare" {
		return fmt.Errorf("quality comparison tool = %q, want sandbox_quality_compare", out.Tool)
	}
	if out.Schema != reportdraft.SubReportSchemaID {
		return fmt.Errorf("quality comparison schema = %q, want %q", out.Schema, reportdraft.SubReportSchemaID)
	}
	if out.Mode != "manifest" {
		return fmt.Errorf("quality comparison mode = %q, want manifest", out.Mode)
	}
	if !out.ReviewRequired {
		return errors.New("quality comparison review_required must be true")
	}
	if err := validateReviewEvidenceText("quality comparison sample_basis", out.SampleBasis); err != nil {
		return err
	}
	if out.CaseCount <= 0 {
		return errors.New("quality comparison case_count must be greater than zero")
	}
	if len(out.Cases) != out.CaseCount {
		return fmt.Errorf("quality comparison has %d cases, want case_count %d", len(out.Cases), out.CaseCount)
	}
	if len(out.ScenarioCoverage) == 0 {
		return errors.New("quality comparison scenario_coverage must not be empty")
	}
	for _, scenario := range requiredQualityScenarioNames {
		if !containsString(out.ScenarioCoverage, scenario) {
			return fmt.Errorf("quality comparison missing required scenario coverage %q", scenario)
		}
	}
	if strings.TrimSpace(out.Recommendation) == "" {
		return errors.New("quality comparison recommendation is required")
	}
	sum := out.Summary.ImprovedCount + out.Summary.EquivalentCount + out.Summary.RegressedCount + out.Summary.NeedsHumanReviewCount
	if sum != out.CaseCount {
		return fmt.Errorf("quality comparison summary count = %d, want case_count %d", sum, out.CaseCount)
	}
	caseScenarios := make([]string, 0, len(out.Cases))
	var caseSummary helperQualitySummary
	seenCaseIDs := map[string]bool{}
	for i, item := range out.Cases {
		id, err := validateEvidenceCaseID(fmt.Sprintf("quality comparison cases[%d].id", i), item.ID)
		if err != nil {
			return err
		}
		if seenCaseIDs[id] {
			return fmt.Errorf("duplicate quality comparison case id %q", id)
		}
		seenCaseIDs[id] = true
		scenario := strings.TrimSpace(item.Scenario)
		if scenario == "" {
			return fmt.Errorf("quality comparison case %q scenario is required", id)
		}
		if scenario != item.Scenario {
			return fmt.Errorf("quality comparison case %q scenario must not contain leading or trailing whitespace", id)
		}
		if !reportprompt.Scenario(scenario).Valid() {
			return fmt.Errorf("quality comparison case %q scenario %q is unsupported", id, scenario)
		}
		if err := validateQualityRequiredEvidenceRefs(id, item.RequiredEvidenceRefs); err != nil {
			return err
		}
		if strings.TrimSpace(item.Recommendation) == "" {
			return fmt.Errorf("quality comparison case %q recommendation is required", id)
		}
		if strings.TrimSpace(item.Recommendation) != item.Recommendation {
			return fmt.Errorf("quality comparison case %q recommendation must not contain leading or trailing whitespace", id)
		}
		if !validCaseRecommendation(item.Recommendation) {
			return fmt.Errorf("quality comparison case %q has unsupported recommendation %q", id, item.Recommendation)
		}
		if !item.ReviewRequired {
			return fmt.Errorf("quality comparison case %q review_required must be true", id)
		}
		addCaseRecommendation(&caseSummary, item.Recommendation)
		caseScenarios = append(caseScenarios, scenario)
	}
	if out.Summary != caseSummary {
		return fmt.Errorf("quality comparison summary = %+v, want case-derived summary %+v", out.Summary, caseSummary)
	}
	if expected := batchRecommendation(out.Summary); out.Recommendation != expected {
		return fmt.Errorf("quality comparison recommendation = %q, want %q", out.Recommendation, expected)
	}
	derivedCoverage := scenarioCoverage(caseScenarios)
	if len(out.ScenarioCoverage) != len(derivedCoverage) {
		return fmt.Errorf("quality comparison scenario_coverage = %v, want %v", out.ScenarioCoverage, derivedCoverage)
	}
	for i := range derivedCoverage {
		if out.ScenarioCoverage[i] != derivedCoverage[i] {
			return fmt.Errorf("quality comparison scenario_coverage = %v, want %v", out.ScenarioCoverage, derivedCoverage)
		}
	}
	return nil
}

func validateQualityRequiredEvidenceRefs(caseID string, refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("quality comparison case %q required_evidence_refs must contain at least one item", caseID)
	}
	seen := map[string]bool{}
	hasSnapshotRef := false
	for i, ref := range refs {
		value, err := validateEvidenceRef(fmt.Sprintf("quality comparison case %q required_evidence_refs[%d]", caseID, i), ref)
		if err != nil {
			return err
		}
		if seen[value] {
			return fmt.Errorf("quality comparison case %q duplicate required_evidence_refs value %q", caseID, value)
		}
		if strings.HasPrefix(value, requiredSnapshotRefPrefix) {
			if !validQualitySnapshotRef(value) {
				return fmt.Errorf("quality comparison case %q required_evidence_refs[%d] snapshot evidence ref %q must use snapshot:<positive-id>", caseID, i, value)
			}
			hasSnapshotRef = true
		}
		seen[value] = true
	}
	if !hasSnapshotRef {
		return fmt.Errorf("quality comparison case %q required_evidence_refs must include one snapshot:<positive-id> reference", caseID)
	}
	return nil
}

func validQualitySnapshotRef(ref string) bool {
	rawID := strings.TrimPrefix(ref, requiredSnapshotRefPrefix)
	id, err := strconv.ParseInt(rawID, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == rawID
}

func addCaseRecommendation(summary *helperQualitySummary, recommendation string) {
	switch recommendation {
	case "sandbox_candidate_improved":
		summary.ImprovedCount++
	case "equivalent_metrics":
		summary.EquivalentCount++
	case "sandbox_candidate_regressed":
		summary.RegressedCount++
	case "needs_human_review":
		summary.NeedsHumanReviewCount++
	}
}

func validateDecisionOutput(raw []byte) error {
	var out helperDecisionOutput
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("parse decision output: %w", err)
	}
	if out.Tool != "sandbox_m4_decision" {
		return fmt.Errorf("decision tool = %q, want sandbox_m4_decision", out.Tool)
	}
	if !validDecision(out.Decision) {
		return fmt.Errorf("decision = %q, want proceed, iterate, or defer", out.Decision)
	}
	if !out.ReviewRequired {
		return errors.New("decision review_required must be true")
	}
	if err := validateDecisionReasons(out.Decision, out.Reasons); err != nil {
		return err
	}
	if strings.TrimSpace(out.Evidence.BaselineAuditStatus) == "" {
		return errors.New("decision evidence baseline_audit_status is required")
	}
	if strings.TrimSpace(out.Evidence.QualityRecommendation) == "" {
		return errors.New("decision evidence quality_recommendation is required")
	}
	if out.Evidence.CaseCount <= 0 {
		return errors.New("decision evidence case_count must be greater than zero")
	}
	if out.Evidence.MinimumCaseCount <= 0 {
		return errors.New("decision evidence minimum_case_count must be greater than zero")
	}
	if strings.TrimSpace(out.Evidence.SampleBasis) == "" {
		return errors.New("decision evidence sample_basis is required")
	}
	if len(out.Evidence.ScenarioCoverage) == 0 {
		return errors.New("decision evidence scenario_coverage must not be empty")
	}
	if strings.TrimSpace(out.Evidence.RuntimeCandidate) == "" {
		return errors.New("decision evidence runtime_candidate is required")
	}
	if !immutableImageReference(out.Evidence.RuntimeCandidate) {
		return errors.New("decision evidence runtime_candidate must be an immutable image reference `name@sha256:<64-hex-digest>`")
	}
	if out.Decision == decisionProceed && loopbackImageReference(out.Evidence.RuntimeCandidate) {
		return errors.New(loopbackRuntimeCandidateReason)
	}
	if out.Evidence.RuntimeSmokePassedCount < 0 {
		return errors.New("decision evidence runtime_smoke_passed_count must not be negative")
	}
	if out.Evidence.ReviewedCaseCount <= 0 {
		return errors.New("decision evidence reviewed_case_count must be greater than zero")
	}
	if !validReviewStatus(out.Evidence.HumanReviewStatus) {
		return fmt.Errorf("decision evidence human_review_status = %q, want pass or fail", out.Evidence.HumanReviewStatus)
	}
	return nil
}

func validateDecisionReasons(decision string, reasons []string) error {
	if len(reasons) == 0 {
		return errors.New("decision reasons must not be empty")
	}
	seen := map[string]bool{}
	for i, reason := range reasons {
		trimmed := strings.TrimSpace(reason)
		if trimmed == "" {
			return fmt.Errorf("decision reasons[%d] is required", i)
		}
		if trimmed != reason {
			return fmt.Errorf("decision reason %q must not contain leading or trailing whitespace", reason)
		}
		if seen[reason] {
			return fmt.Errorf("duplicate decision reason %q", reason)
		}
		seen[reason] = true
	}
	switch decision {
	case decisionProceed:
		if len(reasons) != 1 || reasons[0] != canonicalProceedReason {
			return fmt.Errorf("proceed decision reasons must be exactly [%q]", canonicalProceedReason)
		}
	default:
		for _, reason := range reasons {
			if reason == canonicalProceedReason {
				return fmt.Errorf("%s decision must not use the proceed success reason", decision)
			}
		}
	}
	return nil
}

func validateDecisionEvidenceMatchesArtifacts(decision decisionFile, artifacts packetArtifacts, minCases int) error {
	var baseline helperBaselineAudit
	if err := readStrictJSONArtifact(artifacts.BaselineAudit, &baseline); err != nil {
		return fmt.Errorf("baseline audit: %w", err)
	}
	var quality helperQualityComparison
	if err := readStrictJSONArtifact(artifacts.QualityComparison, &quality); err != nil {
		return fmt.Errorf("quality comparison: %w", err)
	}
	var review packetReviewEvidence
	if err := readStrictJSONArtifact(artifacts.ReviewEvidence, &review); err != nil {
		return fmt.Errorf("review evidence: %w", err)
	}
	evidence := decision.Evidence
	if evidence.BaselineAuditStatus != baseline.Status {
		return fmt.Errorf("decision evidence baseline_audit_status = %q, want baseline audit status %q", evidence.BaselineAuditStatus, baseline.Status)
	}
	if evidence.QualityRecommendation != quality.Recommendation {
		return fmt.Errorf("decision evidence quality_recommendation = %q, want quality comparison recommendation %q", evidence.QualityRecommendation, quality.Recommendation)
	}
	if evidence.CaseCount != quality.CaseCount {
		return fmt.Errorf("decision evidence case_count = %d, want quality comparison case_count %d", evidence.CaseCount, quality.CaseCount)
	}
	if evidence.MinimumCaseCount != minCases {
		return fmt.Errorf("decision evidence minimum_case_count = %d, want min-cases %d", evidence.MinimumCaseCount, minCases)
	}
	if evidence.SampleBasis != quality.SampleBasis {
		return fmt.Errorf("decision evidence sample_basis = %q, want quality comparison sample_basis %q", evidence.SampleBasis, quality.SampleBasis)
	}
	if !equalStrings(evidence.ScenarioCoverage, quality.ScenarioCoverage) {
		return fmt.Errorf("decision evidence scenario_coverage = %v, want quality comparison scenario_coverage %v", evidence.ScenarioCoverage, quality.ScenarioCoverage)
	}
	if evidence.SelectedCandidate != review.SelectedCandidate {
		return fmt.Errorf("decision evidence selected_candidate = %q, want review evidence selected_candidate %q", evidence.SelectedCandidate, review.SelectedCandidate)
	}
	if evidence.RuntimeCandidate != review.RuntimeCandidate {
		return fmt.Errorf("decision evidence runtime_candidate = %q, want review evidence runtime_candidate %q", evidence.RuntimeCandidate, review.RuntimeCandidate)
	}
	if evidence.CandidateEvaluationCount != len(review.CandidateEvaluations) {
		return fmt.Errorf("decision evidence candidate_evaluation_count = %d, want review evidence candidate_evaluations count %d", evidence.CandidateEvaluationCount, len(review.CandidateEvaluations))
	}
	if expected := countPassedReviewSmokes(review.RuntimeSmokes); evidence.RuntimeSmokePassedCount != expected {
		return fmt.Errorf("decision evidence runtime_smoke_passed_count = %d, want review evidence passed smoke count %d", evidence.RuntimeSmokePassedCount, expected)
	}
	if evidence.ReviewedCaseCount != len(review.ReviewedCases) {
		return fmt.Errorf("decision evidence reviewed_case_count = %d, want review evidence reviewed case count %d", evidence.ReviewedCaseCount, len(review.ReviewedCases))
	}
	if evidence.ReviewedCaseCount != quality.CaseCount {
		return fmt.Errorf("decision evidence reviewed_case_count = %d, want quality comparison case_count %d", evidence.ReviewedCaseCount, quality.CaseCount)
	}
	if err := validateReviewedCaseIDsMatchQuality(quality.Cases, review.ReviewedCases); err != nil {
		return err
	}
	if evidence.RepresentativeSample != review.RepresentativeSample {
		return fmt.Errorf("decision evidence representative_sample = %v, want review evidence representative_sample %v", evidence.RepresentativeSample, review.RepresentativeSample)
	}
	if evidence.HumanReviewStatus != review.HumanReview.Status {
		return fmt.Errorf("decision evidence human_review_status = %q, want review evidence human_review.status %q", evidence.HumanReviewStatus, review.HumanReview.Status)
	}
	return nil
}

func countPassedReviewSmokes(smokes []packetRuntimeSmoke) int {
	count := 0
	for _, smoke := range smokes {
		if smoke.Status == "pass" {
			count++
		}
	}
	return count
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validDecision(decision string) bool {
	switch decision {
	case decisionProceed, decisionIterate, decisionDefer:
		return true
	default:
		return false
	}
}

func validCaseRecommendation(recommendation string) bool {
	switch recommendation {
	case "sandbox_candidate_improved", "equivalent_metrics", "sandbox_candidate_regressed", "needs_human_review":
		return true
	default:
		return false
	}
}

func batchRecommendation(summary helperQualitySummary) string {
	switch {
	case summary.RegressedCount > 0:
		return "sandbox_batch_has_regressions"
	case summary.NeedsHumanReviewCount > 0:
		return "needs_human_review"
	case summary.ImprovedCount > 0:
		return "sandbox_batch_candidate_improved"
	default:
		return "equivalent_metrics"
	}
}

func scenarioCoverage(caseScenarios []string) []string {
	seen := map[string]bool{}
	for _, scenario := range caseScenarios {
		seen[scenario] = true
	}
	out := make([]string, 0, len(requiredQualityScenarioNames))
	for _, scenario := range requiredQualityScenarioNames {
		if seen[scenario] {
			out = append(out, scenario)
		}
	}
	return out
}

func validReviewStatus(status string) bool {
	switch status {
	case "pass", "fail":
		return true
	default:
		return false
	}
}

func validCandidateID(candidate string) bool {
	return candidate != "" && len(candidate) <= maxEvidenceCaseIDBytes && !strings.ContainsAny(candidate, " \t\r\n")
}

func validCandidateEvaluationStatus(status string) bool {
	switch status {
	case "pass", "fail", "not_fit":
		return true
	default:
		return false
	}
}

func requiredRuntimeSmokeName(name string) bool {
	for _, required := range requiredRuntimeSmokeNames {
		if name == required {
			return true
		}
	}
	return false
}

func immutableImageReference(value string) bool {
	name, digest, ok := strings.Cut(value, "@sha256:")
	if !ok || name == "" {
		return false
	}
	if strings.ContainsAny(name, " \t\r\n@") {
		return false
	}
	return imageDigestHexRe.MatchString(digest)
}

func loopbackImageReference(value string) bool {
	name, _, ok := strings.Cut(value, "@sha256:")
	if !ok {
		return false
	}
	registry := imageReferenceRegistry(name)
	if registry == "" {
		return false
	}
	host := registry
	if h, _, err := net.SplitHostPort(registry); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func imageReferenceRegistry(name string) string {
	first, _, _ := strings.Cut(name, "/")
	if first == "" {
		return ""
	}
	if strings.EqualFold(first, "localhost") || strings.ContainsAny(first, ".:") {
		return first
	}
	return ""
}

func copyEvidenceFile(src, dst, qualitySampleBasis string) error {
	raw, err := readFileCapped(src)
	if err != nil {
		return err
	}
	if err := validateReviewEvidence(raw); err != nil {
		return fmt.Errorf("%s is invalid review evidence: %w", filepath.Clean(src), err)
	}
	reviewSampleBasis, err := reviewSampleBasis(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid review evidence: %w", filepath.Clean(src), err)
	}
	if reviewSampleBasis != qualitySampleBasis {
		return fmt.Errorf("%s is invalid review evidence: review evidence sample_basis %q must match quality comparison sample_basis %q", filepath.Clean(src), reviewSampleBasis, qualitySampleBasis)
	}
	if err := os.WriteFile(dst, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

func reviewSampleBasis(raw []byte) (string, error) {
	var review packetReviewEvidence
	if err := strictjson.Unmarshal(raw, &review); err != nil {
		return "", fmt.Errorf("parse review evidence: %w", err)
	}
	return review.SampleBasis, nil
}

func validateReviewEvidence(raw []byte) error {
	var review packetReviewEvidence
	if err := strictjson.Unmarshal(raw, &review); err != nil {
		return fmt.Errorf("parse review evidence: %w", err)
	}
	if review.Tool != "sandbox_m4_review_evidence" {
		return fmt.Errorf("review evidence tool = %q, want sandbox_m4_review_evidence", review.Tool)
	}
	if evidenceDate, err := time.Parse("2006-01-02", review.EvidenceDate); err != nil {
		return fmt.Errorf("review evidence date %q must be YYYY-MM-DD", review.EvidenceDate)
	} else if evidenceDate.After(todayUTC()) {
		return fmt.Errorf("review evidence date %q must not be in the future", review.EvidenceDate)
	}
	if strings.TrimSpace(review.RuntimeCandidate) == "" {
		return errors.New("review evidence runtime_candidate is required")
	}
	if strings.TrimSpace(review.RuntimeCandidate) != review.RuntimeCandidate {
		return errors.New("review evidence runtime_candidate must not contain leading or trailing whitespace")
	}
	if !immutableImageReference(review.RuntimeCandidate) {
		return errors.New("review evidence runtime_candidate must be an immutable image reference `name@sha256:<64-hex-digest>`")
	}
	if err := validateCandidateEvaluations(review.SelectedCandidate, review.RuntimeCandidate, review.CandidateEvaluations); err != nil {
		return err
	}
	if strings.TrimSpace(review.SampleBasis) == "" {
		return errors.New("review evidence sample_basis is required")
	}
	if err := validateReviewEvidenceText("review evidence sample_basis", review.SampleBasis); err != nil {
		return err
	}
	if err := validateReviewRuntimeSmokes(review.RuntimeSmokes); err != nil {
		return err
	}
	if err := validateReviewCaseList(review.ReviewedCases); err != nil {
		return err
	}
	if err := validateReviewEvidenceText("review evidence human_review.reviewer", review.HumanReview.Reviewer); err != nil {
		return err
	}
	if err := validateReviewEvidenceText("review evidence human_review.notes", review.HumanReview.Notes); err != nil {
		return err
	}
	if strings.TrimSpace(review.HumanReview.Status) == "" {
		return errors.New("review evidence human_review.status is required")
	}
	if strings.TrimSpace(review.HumanReview.Status) != review.HumanReview.Status {
		return errors.New("review evidence human_review.status must not contain leading or trailing whitespace")
	}
	if !validReviewStatus(review.HumanReview.Status) {
		return fmt.Errorf("review evidence human_review.status = %q, want pass or fail", review.HumanReview.Status)
	}
	return nil
}

func validateCandidateEvaluations(selected, runtimeCandidate string, evaluations []packetCandidateEvaluation) error {
	selectedValue := strings.TrimSpace(selected)
	if selectedValue == "" {
		return errors.New("review evidence selected_candidate is required")
	}
	if selectedValue != selected {
		return errors.New("review evidence selected_candidate must not contain leading or trailing whitespace")
	}
	if !validCandidateID(selected) {
		return fmt.Errorf("review evidence selected_candidate %q must be a non-whitespace candidate id up to %d bytes", selected, maxEvidenceCaseIDBytes)
	}
	selected = selectedValue
	if len(evaluations) == 0 {
		return errors.New("review evidence candidate_evaluations must contain at least one candidate")
	}
	byCandidate := map[string]packetCandidateEvaluation{}
	for i, item := range evaluations {
		candidate := strings.TrimSpace(item.Candidate)
		if candidate == "" {
			return fmt.Errorf("review evidence candidate_evaluations[%d].candidate is required", i)
		}
		if candidate != item.Candidate {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q must not contain leading or trailing whitespace", item.Candidate)
		}
		if !validCandidateID(candidate) {
			return fmt.Errorf("review evidence candidate_evaluations[%d].candidate %q must be a non-whitespace candidate id up to %d bytes", i, candidate, maxEvidenceCaseIDBytes)
		}
		if _, ok := byCandidate[candidate]; ok {
			return fmt.Errorf("duplicate review evidence candidate_evaluations candidate %q", candidate)
		}
		status := strings.TrimSpace(item.Status)
		if status == "" {
			return fmt.Errorf("review evidence candidate_evaluations[%d].status is required", i)
		}
		if status != item.Status {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q status must not contain leading or trailing whitespace", candidate)
		}
		if !validCandidateEvaluationStatus(status) {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q status = %q, want pass, fail, or not_fit", candidate, item.Status)
		}
		itemRuntimeCandidate := strings.TrimSpace(item.RuntimeCandidate)
		if status == "pass" && itemRuntimeCandidate == "" {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_candidate is required when status is pass", candidate)
		}
		if itemRuntimeCandidate != item.RuntimeCandidate {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_candidate must not contain leading or trailing whitespace", candidate)
		}
		if itemRuntimeCandidate != "" && !immutableImageReference(itemRuntimeCandidate) {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_candidate must be an immutable image reference `name@sha256:<64-hex-digest>`", candidate)
		}
		if err := validateCandidateRuntimeSmokeRefs(candidate, status, item.RuntimeSmokeRefs); err != nil {
			return err
		}
		if err := validateReviewEvidenceText(fmt.Sprintf("review evidence candidate_evaluations candidate %q source", candidate), item.Source); err != nil {
			return err
		}
		if err := validateReviewEvidenceText(fmt.Sprintf("review evidence candidate_evaluations candidate %q notes", candidate), item.Notes); err != nil {
			return err
		}
		byCandidate[candidate] = item
	}
	selectedEval, ok := byCandidate[selected]
	if !ok {
		return fmt.Errorf("review evidence selected_candidate %q must have a matching candidate_evaluations entry", selected)
	}
	if selectedEval.Status == "pass" && selectedEval.RuntimeCandidate != runtimeCandidate {
		return fmt.Errorf("selected candidate %q runtime_candidate %q must match review evidence runtime_candidate %q", selected, selectedEval.RuntimeCandidate, runtimeCandidate)
	}
	return nil
}

func validateCandidateRuntimeSmokeRefs(candidate, status string, refs []string) error {
	seen := map[string]bool{}
	for i, ref := range refs {
		value := strings.TrimSpace(ref)
		if value == "" {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_smoke_refs[%d] is required", candidate, i)
		}
		if value != ref {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_smoke_refs[%d] must not contain leading or trailing whitespace", candidate, i)
		}
		if !requiredRuntimeSmokeName(value) {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_smoke_refs[%d] = %q is not a required runtime smoke", candidate, i, value)
		}
		if seen[value] {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q duplicate runtime_smoke_refs value %q", candidate, value)
		}
		seen[value] = true
	}
	if status != "pass" {
		return nil
	}
	for _, name := range requiredRuntimeSmokeNames {
		if !seen[name] {
			return fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_smoke_refs must include %q when status is pass", candidate, name)
		}
	}
	return nil
}

func validateReviewCaseList(cases []packetReviewedCase) error {
	if len(cases) == 0 {
		return errors.New("review evidence reviewed_cases must contain at least one item")
	}
	seen := map[string]bool{}
	for i, item := range cases {
		id, err := validateEvidenceCaseID(fmt.Sprintf("review evidence reviewed_cases[%d].id", i), item.ID)
		if err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("duplicate review evidence reviewed case %q", id)
		}
		seen[id] = true
		scenario := strings.TrimSpace(item.Scenario)
		if scenario == "" {
			return fmt.Errorf("review evidence reviewed case %q scenario is required", id)
		}
		if scenario != item.Scenario {
			return fmt.Errorf("review evidence reviewed case %q scenario must not contain leading or trailing whitespace", id)
		}
		if !reportprompt.Scenario(scenario).Valid() {
			return fmt.Errorf("review evidence reviewed case %q scenario %q is unsupported", id, scenario)
		}
		if err := validateReviewedCaseRequiredEvidenceRefs(id, item.RequiredEvidenceRefs); err != nil {
			return err
		}
		if strings.TrimSpace(item.Status) == "" {
			return fmt.Errorf("review evidence reviewed case %q status is required", id)
		}
		if strings.TrimSpace(item.Status) != item.Status {
			return fmt.Errorf("review evidence reviewed case %q status must not contain leading or trailing whitespace", id)
		}
		if !validReviewStatus(item.Status) {
			return fmt.Errorf("review evidence reviewed case %q status = %q, want pass or fail", id, item.Status)
		}
		if err := validateReviewEvidenceText(fmt.Sprintf("review evidence reviewed case %q notes", id), item.Notes); err != nil {
			return err
		}
	}
	return nil
}

func validateReviewedCaseRequiredEvidenceRefs(caseID string, refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("review evidence reviewed case %q required_evidence_refs must contain at least one item", caseID)
	}
	seen := map[string]bool{}
	hasSnapshotRef := false
	for i, ref := range refs {
		value, err := validateEvidenceRef(fmt.Sprintf("review evidence reviewed case %q required_evidence_refs[%d]", caseID, i), ref)
		if err != nil {
			return err
		}
		if seen[value] {
			return fmt.Errorf("review evidence reviewed case %q duplicate required_evidence_refs value %q", caseID, value)
		}
		if strings.HasPrefix(value, requiredSnapshotRefPrefix) {
			if !validQualitySnapshotRef(value) {
				return fmt.Errorf("review evidence reviewed case %q required_evidence_refs[%d] snapshot evidence ref %q must use snapshot:<positive-id>", caseID, i, value)
			}
			hasSnapshotRef = true
		}
		seen[value] = true
	}
	if !hasSnapshotRef {
		return fmt.Errorf("review evidence reviewed case %q required_evidence_refs must include one snapshot:<positive-id> reference", caseID)
	}
	return nil
}

func validateEvidenceCaseID(field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if value != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return "", fmt.Errorf("%s must be a single-line value", field)
	}
	if len(raw) > maxEvidenceCaseIDBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", field, maxEvidenceCaseIDBytes)
	}
	return raw, nil
}

func validateEvidenceRef(field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if value != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return "", fmt.Errorf("%s must be a single-line value", field)
	}
	if len([]rune(raw)) > maxEvidenceRefRunes {
		return "", fmt.Errorf("%s exceeds %d runes", field, maxEvidenceRefRunes)
	}
	return raw, nil
}

func validateReviewEvidenceText(field, raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return fmt.Errorf("%s must be a single-line value", field)
	}
	if len(raw) > maxReviewEvidenceTextBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, maxReviewEvidenceTextBytes)
	}
	return nil
}

func validateReviewedCaseIDsMatchQuality(qualityCases []helperQualityCase, reviewedCases []packetReviewedCase) error {
	reviewedByID := map[string]packetReviewedCase{}
	for _, item := range reviewedCases {
		reviewedByID[item.ID] = item
	}
	qualityIDs := map[string]bool{}
	for _, item := range qualityCases {
		qualityIDs[item.ID] = true
		review, ok := reviewedByID[item.ID]
		if !ok {
			return fmt.Errorf("review evidence missing reviewed case %q", item.ID)
		}
		if review.Scenario != item.Scenario {
			return fmt.Errorf("review evidence reviewed case %q scenario = %q, want quality comparison scenario %q", item.ID, review.Scenario, item.Scenario)
		}
		if !equalStrings(review.RequiredEvidenceRefs, item.RequiredEvidenceRefs) {
			return fmt.Errorf("review evidence reviewed case %q required_evidence_refs = %v, want quality comparison refs %v", item.ID, review.RequiredEvidenceRefs, item.RequiredEvidenceRefs)
		}
	}
	for _, item := range reviewedCases {
		if !qualityIDs[item.ID] {
			return fmt.Errorf("review evidence reviewed case %q does not match a quality comparison case", item.ID)
		}
	}
	return nil
}

func validateReviewRuntimeSmokes(smokes []packetRuntimeSmoke) error {
	seen := map[string]packetRuntimeSmoke{}
	seenEvidenceRefs := map[string]bool{}
	for i, smoke := range smokes {
		name := strings.TrimSpace(smoke.Name)
		if name == "" {
			return fmt.Errorf("runtime_smokes[%d].name is required", i)
		}
		if name != smoke.Name {
			return fmt.Errorf("runtime smoke name %q must not contain leading or trailing whitespace", smoke.Name)
		}
		if !requiredRuntimeSmokeName(name) {
			return fmt.Errorf("runtime_smokes[%d].name = %q is not a required runtime smoke", i, name)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate runtime smoke %q", name)
		}
		evidenceRef, err := validateRuntimeSmokeEvidence(name, smoke.EvidenceRef, smoke.EvidenceSHA256)
		if err != nil {
			return err
		}
		if seenEvidenceRefs[evidenceRef] {
			return fmt.Errorf("runtime smoke %q evidence_ref %q duplicates another runtime smoke", name, evidenceRef)
		}
		seenEvidenceRefs[evidenceRef] = true
		seen[name] = smoke
	}
	for _, name := range requiredRuntimeSmokeNames {
		smoke, ok := seen[name]
		if !ok {
			return fmt.Errorf("runtime smoke %q is missing", name)
		}
		if strings.TrimSpace(smoke.Status) == "" {
			return fmt.Errorf("runtime smoke %q status is required", name)
		}
		if strings.TrimSpace(smoke.Status) != smoke.Status {
			return fmt.Errorf("runtime smoke %q status must not contain leading or trailing whitespace", name)
		}
		if !validReviewStatus(smoke.Status) {
			return fmt.Errorf("runtime smoke %q status = %q, want pass or fail", name, smoke.Status)
		}
		source := strings.TrimSpace(smoke.Source)
		if source == "" {
			return fmt.Errorf("runtime smoke %q source is required", name)
		}
		if source != smoke.Source {
			return fmt.Errorf("runtime smoke %q source must not contain leading or trailing whitespace", name)
		}
		if want := requiredRuntimeSmokeSources[name]; source != want {
			return fmt.Errorf("runtime smoke %q source = %q, want %q", name, source, want)
		}
	}
	return nil
}

func validateRuntimeSmokeEvidence(name, ref, digest string) (string, error) {
	value := strings.TrimSpace(ref)
	if value == "" {
		return "", fmt.Errorf("runtime smoke %q evidence_ref is required", name)
	}
	if value != ref {
		return "", fmt.Errorf("runtime smoke %q evidence_ref must not contain leading or trailing whitespace", name)
	}
	if strings.ContainsAny(ref, "\r\n\t") {
		return "", fmt.Errorf("runtime smoke %q evidence_ref must be a single-line value", name)
	}
	if len(ref) > maxRuntimeSmokeEvidenceRefBytes {
		return "", fmt.Errorf("runtime smoke %q evidence_ref exceeds %d bytes", name, maxRuntimeSmokeEvidenceRefBytes)
	}
	if err := validatePortableEvidenceRef(name, ref); err != nil {
		return "", err
	}
	if !imageDigestHexRe.MatchString(digest) {
		return "", fmt.Errorf("runtime smoke %q evidence_sha256 must be 64 lowercase hex characters", name)
	}
	return ref, nil
}

func validatePortableEvidenceRef(name, ref string) error {
	if strings.HasPrefix(ref, "/") {
		return fmt.Errorf("runtime smoke %q evidence_ref must be a relative artifact path", name)
	}
	if strings.ContainsAny(ref, "\\: ") {
		return fmt.Errorf("runtime smoke %q evidence_ref must be a slash-separated relative artifact path without spaces or URI syntax", name)
	}
	clean := path.Clean(ref)
	if clean != ref || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("runtime smoke %q evidence_ref must be a normalized relative artifact path", name)
	}
	return nil
}

func parseDecisionFile(path string) (decisionFile, error) {
	var decision decisionFile
	if err := readStrictJSONArtifact(path, &decision); err != nil {
		return decisionFile{}, err
	}
	if decision.Decision == "" {
		return decisionFile{}, fmt.Errorf("decision file %s missing decision", filepath.Clean(path))
	}
	return decision, nil
}

func readStrictJSONArtifact(path string, dst any) error {
	raw, err := readFileCapped(path)
	if err != nil {
		return err
	}
	if err := strictjson.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Clean(path), err)
	}
	return nil
}

func writeJSONFile(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func readFileCapped(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return nil, err
	}
	// #nosec G304 -- this packet helper intentionally opens
	// operator-supplied local evidence files.
	f, err := os.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", clean, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", clean, err)
	}
	if int64(len(raw)) > maxInputBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", clean, maxInputBytes)
	}
	return raw, nil
}

func requireRegularFile(path string) error {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("stat %s: %w", clean, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", clean)
	}
	return nil
}
