// Command sandbox_m4_subreport_generate runs one persisted EvidenceSnapshot
// through a sandbox runtime candidate and stores the accepted SubReport output.
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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	dockerclient "github.com/moby/moby/client"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/repository"
	containerdocker "github.com/openclarion/openclarion/internal/providers/container/docker"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/llmretry"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

const (
	toolName               = "sandbox_m4_subreport_generate"
	summarySchemaID        = "openclarion_sandbox_m4_subreport_generate_v1"
	defaultAgentName       = "report-enhancer"
	defaultTimeoutSeconds  = 300
	defaultLLMHTTPTimeout  = 90 * time.Second
	runnerShutdownGrace    = 5 * time.Second
	maxCandidateIDBytes    = 80
	maxCommandEnvBytes     = 4096
	maxEvidenceEnvelopeLen = ports.DefaultContainerOutputBytes
)

var candidateIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

type config struct {
	SnapshotID      int64
	Scenario        string
	GroupIndex      int
	CandidateID     string
	AgentName       string
	OutPath         string
	DatabaseURL     string
	ImageRef        string
	Command         []string
	AgentConfigRoot string
	WorkspaceRoot   string
	Timeout         time.Duration
	OutputMax       int64
	AllowedEgress   []string
	EgressNetwork   string
	EgressProxyURL  string
	ReportLLM       reportLLMConfig
}

type reportLLMConfig struct {
	Configured  bool
	BaseURL     string
	APIKey      string
	Model       string
	OutputMode  ports.LLMOutputMode
	HTTPTimeout time.Duration
}

type evidenceStore interface {
	FindSnapshotByID(context.Context, domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error)
	FindSubReportByKey(context.Context, domain.EvidenceSnapshotID, string) (domain.SubReport, bool, error)
	SaveSubReport(context.Context, domain.SubReport) (domain.SubReport, error)
}

type uowEvidenceStore struct {
	factory ports.UnitOfWorkFactory
}

type sandboxEvidenceEnvelope struct {
	Schema              string          `json:"schema"`
	EvidenceSnapshotID  int64           `json:"evidence_snapshot_id"`
	EvidenceSnapshotRef string          `json:"evidence_snapshot_ref"`
	EvidenceDigest      string          `json:"evidence_digest"` // Persisted snapshot identity, created before the JSONB round trip.
	PayloadSHA256       string          `json:"payload_sha256"`  // Checksum of the canonical bytes mounted for this invocation.
	EvidenceStatus      string          `json:"evidence_status"`
	MissingFields       []string        `json:"missing_fields"`
	Scenario            string          `json:"scenario"`
	GroupIndex          int             `json:"group_index"`
	Payload             json.RawMessage `json:"payload"`
}

type generationSummary struct {
	Tool                string   `json:"tool"`
	SchemaID            string   `json:"schema_id"`
	Status              string   `json:"status"`
	EvidenceSnapshotID  int64    `json:"evidence_snapshot_id"`
	EvidenceSnapshotRef string   `json:"evidence_snapshot_ref"`
	EvidenceDigest      string   `json:"evidence_digest"`
	Scenario            string   `json:"scenario"`
	GroupIndex          int      `json:"group_index"`
	CandidateID         string   `json:"candidate_id"`
	AgentName           string   `json:"agent_name"`
	SubReportID         int64    `json:"sub_report_id"`
	IdempotencyKey      string   `json:"idempotency_key"`
	InvocationID        string   `json:"invocation_id"`
	RuntimeID           string   `json:"runtime_id,omitempty"`
	OutputSHA256        string   `json:"output_sha256"`
	EvidenceRefs        []string `json:"evidence_refs"`
	Created             bool     `json:"created"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Environ(), os.Stdout, nil, nil); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", toolName, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, environ []string, stdout io.Writer, store evidenceStore, provider ports.ContainerProvider) error {
	cfg, err := parseConfig(args, environ)
	if err != nil {
		return err
	}
	if store == nil {
		client, err := repository.OpenPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("open PostgreSQL: %w", err)
		}
		defer client.Close()
		store = uowEvidenceStore{factory: repository.NewFactory(client)}
	}
	if provider == nil {
		built, cleanup, err := dockerProviderFromConfig(cfg)
		if err != nil {
			return err
		}
		defer cleanup()
		provider = built
	}
	summary, err := generate(ctx, cfg, store, provider)
	if err != nil {
		return err
	}
	return writeJSONOutput(stdout, cfg.OutPath, summary)
}

func parseConfig(args []string, environ []string) (config, error) {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	snapshotID := fs.Int64("snapshot-id", 0, "persisted EvidenceSnapshot ID to run through the sandbox candidate")
	scenario := fs.String("scenario", "", "report prompt scenario: single_alert, cascade, or alert_storm")
	groupIndex := fs.Int("group-index", 0, "non-negative report group index")
	candidateID := fs.String("candidate-id", "", "stable candidate identifier used in the sandbox SubReport idempotency key")
	agentName := fs.String("agent-name", defaultAgentName, "ADR-0013 sandbox agent name")
	outPath := fs.String("out", "", "optional output summary JSON path; stdout is used when omitted")
	timeoutSeconds := fs.Int("timeout-seconds", defaultTimeoutSeconds, "sandbox run timeout in seconds")
	outputMax := fs.Int64("output-max-bytes", ports.DefaultContainerOutputBytes, "sandbox output.json byte cap")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	reportLLMBaseURL := firstEnv(environ, "OPENCLARION_M4_REPORT_LLM_BASE_URL")
	reportLLMAPIKey := firstEnv(environ, "OPENCLARION_M4_REPORT_LLM_API_KEY")
	reportLLMModel := firstEnv(environ, "OPENCLARION_M4_REPORT_LLM_MODEL")
	reportLLMOutputMode := firstEnv(environ, "OPENCLARION_M4_REPORT_LLM_OUTPUT_MODE")
	reportLLMTimeout := firstEnv(environ, "OPENCLARION_M4_REPORT_LLM_HTTP_TIMEOUT_SECONDS")
	cfg := config{
		SnapshotID:      *snapshotID,
		Scenario:        strings.TrimSpace(*scenario),
		GroupIndex:      *groupIndex,
		CandidateID:     strings.TrimSpace(*candidateID),
		AgentName:       strings.TrimSpace(*agentName),
		OutPath:         strings.TrimSpace(*outPath),
		DatabaseURL:     firstEnv(environ, "DATABASE_URL"),
		ImageRef:        firstEnv(environ, "OPENCLARION_M4_SANDBOX_IMAGE_REF", "OPENCLARION_SANDBOX_IMAGE_REF"),
		AgentConfigRoot: firstEnv(environ, "OPENCLARION_M4_SANDBOX_AGENT_CONFIG_ROOT", "OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT"),
		WorkspaceRoot:   firstEnv(environ, "OPENCLARION_M4_SANDBOX_WORKSPACE_ROOT", "OPENCLARION_SANDBOX_WORKSPACE_ROOT"),
		Timeout:         time.Duration(*timeoutSeconds) * time.Second,
		OutputMax:       *outputMax,
		AllowedEgress:   csvValues(firstEnv(environ, "OPENCLARION_M4_SANDBOX_EGRESS_ALLOWED", "OPENCLARION_SANDBOX_EGRESS_ALLOWED")),
		EgressNetwork:   firstEnv(environ, "OPENCLARION_M4_SANDBOX_EGRESS_NETWORK", "OPENCLARION_SANDBOX_EGRESS_NETWORK"),
		EgressProxyURL:  firstEnv(environ, "OPENCLARION_M4_SANDBOX_EGRESS_PROXY_URL", "OPENCLARION_SANDBOX_EGRESS_PROXY_URL"),
		ReportLLM: reportLLMConfig{
			Configured: reportLLMBaseURL != "" || reportLLMAPIKey != "" || reportLLMModel != "" || reportLLMOutputMode != "" || reportLLMTimeout != "",
			BaseURL:    reportLLMBaseURL,
			APIKey:     reportLLMAPIKey,
			Model:      reportLLMModel,
			OutputMode: ports.LLMOutputMode(reportLLMOutputMode),
		},
	}
	if cfg.ReportLLM.OutputMode == "" {
		cfg.ReportLLM.OutputMode = ports.LLMOutputModeJSONSchema
	}
	if reportLLMTimeout == "" {
		cfg.ReportLLM.HTTPTimeout = defaultLLMHTTPTimeout
	} else {
		seconds, err := strconv.Atoi(reportLLMTimeout)
		maxSeconds := int(ports.MaxContainerRunTimeout / time.Second)
		if err != nil || seconds <= 0 || seconds > maxSeconds {
			return config{}, fmt.Errorf("OPENCLARION_M4_REPORT_LLM_HTTP_TIMEOUT_SECONDS must be an integer from 1 to %d", maxSeconds)
		}
		cfg.ReportLLM.HTTPTimeout = time.Duration(seconds) * time.Second
	}
	commandEnvName := "OPENCLARION_M4_SANDBOX_COMMAND_JSON"
	commandRaw := firstEnv(environ, commandEnvName)
	if commandRaw == "" && !cfg.ReportLLM.enabled() {
		commandEnvName = "OPENCLARION_SANDBOX_COMMAND_JSON"
		commandRaw = firstEnv(environ, commandEnvName)
	}
	command, err := parseOptionalJSONStringArray(commandRaw, commandEnvName)
	if err != nil {
		return config{}, err
	}
	cfg.Command = command
	if err := cfg.validate(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (c config) validate() error {
	if c.SnapshotID <= 0 {
		return errors.New("--snapshot-id must be positive")
	}
	if scenario := reportprompt.Scenario(c.Scenario); !scenario.Valid() {
		return fmt.Errorf("--scenario %q is unsupported", c.Scenario)
	}
	if c.GroupIndex < 0 {
		return errors.New("--group-index must be >= 0")
	}
	if err := validateCandidateID(c.CandidateID); err != nil {
		return fmt.Errorf("--candidate-id: %w", err)
	}
	if strings.TrimSpace(c.AgentName) == "" {
		return errors.New("--agent-name must be non-empty")
	}
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.ImageRef == "" {
		return errors.New("OPENCLARION_M4_SANDBOX_IMAGE_REF is required")
	}
	if c.AgentConfigRoot == "" {
		return errors.New("OPENCLARION_M4_SANDBOX_AGENT_CONFIG_ROOT is required")
	}
	if c.Timeout <= 0 {
		return errors.New("--timeout-seconds must be positive")
	}
	if c.Timeout > ports.MaxContainerRunTimeout {
		return fmt.Errorf("--timeout-seconds exceeds maximum %s", ports.MaxContainerRunTimeout)
	}
	if c.OutputMax <= 0 {
		return errors.New("--output-max-bytes must be positive")
	}
	if c.OutputMax > ports.MaxContainerOutputBytes {
		return fmt.Errorf("--output-max-bytes exceeds maximum %d", ports.MaxContainerOutputBytes)
	}
	if len(c.AllowedEgress) > 0 && c.EgressProxyURL == "" {
		return errors.New("OPENCLARION_M4_SANDBOX_EGRESS_PROXY_URL is required when allowlisted egress is configured")
	}
	if len(c.AllowedEgress) == 0 && (c.EgressNetwork != "" || c.EgressProxyURL != "") {
		return errors.New("OPENCLARION_M4_SANDBOX_EGRESS_ALLOWED is required when an egress network or proxy is configured")
	}
	if len(c.Command) > 0 && c.ReportLLM.enabled() {
		return errors.New("OPENCLARION_M4_SANDBOX_COMMAND_JSON is not supported when report LLM credentials are configured; use the image ENTRYPOINT")
	}
	if err := c.ReportLLM.validate(c.Timeout, c.AllowedEgress); err != nil {
		return err
	}
	return nil
}

func (c reportLLMConfig) enabled() bool {
	return c.Configured
}

func (c reportLLMConfig) validate(containerTimeout time.Duration, allowedEgress []string) error {
	if !c.enabled() {
		return nil
	}
	if c.BaseURL == "" || c.APIKey == "" || c.Model == "" {
		return errors.New("OPENCLARION_M4_REPORT_LLM_BASE_URL, OPENCLARION_M4_REPORT_LLM_API_KEY, and OPENCLARION_M4_REPORT_LLM_MODEL must be configured together")
	}
	if c.OutputMode != ports.LLMOutputModeJSONSchema && c.OutputMode != ports.LLMOutputModeJSONObject {
		return fmt.Errorf("OPENCLARION_M4_REPORT_LLM_OUTPUT_MODE must be %q or %q", ports.LLMOutputModeJSONSchema, ports.LLMOutputModeJSONObject)
	}
	if c.HTTPTimeout <= 0 {
		return errors.New("OPENCLARION_M4_REPORT_LLM_HTTP_TIMEOUT_SECONDS must be positive")
	}
	maxLLMRuntime := time.Duration(llmretry.DefaultMaxAttempts)*c.HTTPTimeout + runnerShutdownGrace
	if maxLLMRuntime > containerTimeout {
		return fmt.Errorf("OPENCLARION_M4_REPORT_LLM_HTTP_TIMEOUT_SECONDS allows %s across validation attempts, exceeding container timeout %s", maxLLMRuntime, containerTimeout)
	}
	if len(allowedEgress) == 0 {
		return errors.New("OPENCLARION_M4_SANDBOX_EGRESS_ALLOWED is required when report LLM runtime credentials are configured")
	}
	if err := ports.ValidateContainerEgressURL(c.BaseURL, allowedEgress); err != nil {
		return fmt.Errorf("validate OPENCLARION_M4_REPORT_LLM_BASE_URL against OPENCLARION_M4_SANDBOX_EGRESS_ALLOWED: %w", err)
	}
	return nil
}

func (c reportLLMConfig) credentials(expiresAt time.Time) []ports.ContainerCredential {
	if !c.enabled() {
		return nil
	}
	values := []struct {
		name  string
		value string
	}{
		{name: "OPENCLARION_REPORT_LLM_BASE_URL", value: c.BaseURL},
		{name: "OPENCLARION_REPORT_LLM_API_KEY", value: c.APIKey},
		{name: "OPENCLARION_REPORT_LLM_MODEL", value: c.Model},
		{name: "OPENCLARION_REPORT_LLM_OUTPUT_MODE", value: string(c.OutputMode)},
		{name: "OPENCLARION_REPORT_LLM_HTTP_TIMEOUT_SECONDS", value: strconv.Itoa(int(c.HTTPTimeout / time.Second))},
	}
	credentials := make([]ports.ContainerCredential, len(values))
	for i, value := range values {
		credentials[i] = ports.ContainerCredential{
			Name:      value.name,
			Value:     value.value,
			ExpiresAt: expiresAt,
		}
	}
	return credentials
}

func generate(ctx context.Context, cfg config, store evidenceStore, provider ports.ContainerProvider) (generationSummary, error) {
	snapshotID := domain.EvidenceSnapshotID(cfg.SnapshotID)
	snapshot, err := store.FindSnapshotByID(ctx, snapshotID)
	if err != nil {
		return generationSummary{}, fmt.Errorf("load evidence snapshot %d: %w", cfg.SnapshotID, err)
	}
	if snapshot.ID != snapshotID {
		return generationSummary{}, fmt.Errorf("store returned snapshot %d for requested snapshot %d", snapshot.ID, snapshotID)
	}
	idempotencyKey := sandboxSubReportIdempotencyKey(snapshot.ID, cfg.GroupIndex, reportprompt.Scenario(cfg.Scenario), cfg.CandidateID)
	if existing, found, err := store.FindSubReportByKey(ctx, snapshot.ID, idempotencyKey); err != nil {
		return generationSummary{}, fmt.Errorf("lookup sandbox SubReport: %w", err)
	} else if found {
		if existing.Scenario != cfg.Scenario {
			return generationSummary{}, fmt.Errorf("existing sandbox SubReport scenario %q does not match requested %q", existing.Scenario, cfg.Scenario)
		}
		return summaryFromExisting(cfg, snapshot, existing, idempotencyKey), nil
	}

	evidence, err := buildSandboxEvidence(snapshot, cfg)
	if err != nil {
		return generationSummary{}, err
	}
	req := ports.ContainerRunRequest{
		InvocationID: sandboxInvocationID(snapshot.ID, cfg.GroupIndex, reportprompt.Scenario(cfg.Scenario), cfg.CandidateID),
		AgentName:    cfg.AgentName,
		Evidence:     evidence,
		Timeout:      cfg.Timeout,
		OutputMax:    cfg.OutputMax,
		Network:      networkPolicy(cfg),
		Credentials:  cfg.ReportLLM.credentials(time.Now().UTC().Add(cfg.Timeout)),
		Metadata: map[string]string{
			"tool":                  toolName,
			"evidence_snapshot_id":  strconv.FormatInt(cfg.SnapshotID, 10),
			"evidence_snapshot_ref": snapshotRef(snapshot.ID),
			"scenario":              cfg.Scenario,
			"group_index":           strconv.Itoa(cfg.GroupIndex),
			"candidate_id":          cfg.CandidateID,
			"schema_id":             reportdraft.SubReportSchemaID,
		},
	}
	if err := req.Validate(); err != nil {
		return generationSummary{}, fmt.Errorf("container request: %w", err)
	}
	result, err := provider.Run(ctx, req)
	if err != nil {
		return generationSummary{}, fmt.Errorf("sandbox run: %w", err)
	}
	if err := ports.ValidateContainerRunResult(req, result); err != nil {
		return generationSummary{}, fmt.Errorf("sandbox result: %w", err)
	}
	draft, err := parseSandboxSubReport(result.Output, cfg.CandidateID)
	if err != nil {
		return generationSummary{}, err
	}
	if !containsString(draft.EvidenceRefs, snapshotRef(snapshot.ID)) {
		return generationSummary{}, fmt.Errorf("sandbox SubReport evidence_refs must include %q", snapshotRef(snapshot.ID))
	}
	report, err := subReportDomainFromDraft(snapshot.ID, idempotencyKey, reportprompt.Scenario(cfg.Scenario), cfg.CandidateID, draft, result.Output)
	if err != nil {
		return generationSummary{}, err
	}
	saved, err := store.SaveSubReport(ctx, report)
	if err == nil {
		return summaryFromSaved(cfg, snapshot, saved, req.InvocationID, result.RuntimeID, true), nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return generationSummary{}, fmt.Errorf("persist sandbox SubReport: %w", err)
	}
	existing, found, lookupErr := store.FindSubReportByKey(ctx, snapshot.ID, idempotencyKey)
	if lookupErr != nil {
		return generationSummary{}, fmt.Errorf("lookup sandbox SubReport after duplicate: %w", lookupErr)
	}
	if !found {
		return generationSummary{}, fmt.Errorf("duplicate sandbox SubReport missing after idempotency conflict for %q", idempotencyKey)
	}
	if existing.Scenario != cfg.Scenario {
		return generationSummary{}, fmt.Errorf("existing sandbox SubReport scenario %q does not match requested %q after idempotency conflict", existing.Scenario, cfg.Scenario)
	}
	return summaryFromSaved(cfg, snapshot, existing, req.InvocationID, result.RuntimeID, false), nil
}

func dockerProviderFromConfig(cfg config) (ports.ContainerProvider, func(), error) {
	engine, err := dockerclient.New(dockerclient.FromEnv, dockerclient.WithUserAgent("openclarion-sandbox-m4-subreport-generate"))
	if err != nil {
		return nil, nil, fmt.Errorf("docker client: %w", err)
	}
	cleanup := func() {
		_ = engine.Close()
	}
	opts := []containerdocker.ProviderOption{}
	if cfg.WorkspaceRoot != "" {
		opts = append(opts, containerdocker.WithWorkspaceRoot(cfg.WorkspaceRoot))
	}
	networkMode := ""
	if len(cfg.AllowedEgress) > 0 {
		networkMode = strings.TrimSpace(cfg.EgressNetwork)
		if networkMode == "" {
			networkMode = containerdocker.DefaultAllowlistNetworkMode
		}
		enforcer, err := containerdocker.NewStaticAllowlistEnforcer(networkMode, cfg.AllowedEgress)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("configure sandbox egress enforcer: %w", err)
		}
		opts = append(opts, containerdocker.WithEgressEnforcer(enforcer))
	}
	provider, err := containerdocker.NewProvider(engine, containerdocker.Config{
		ImageRef:             cfg.ImageRef,
		ReadonlyRootFS:       true,
		NoNewPrivileges:      true,
		CapDrop:              []string{containerdocker.DropAllCapabilities},
		Command:              cloneStrings(cfg.Command),
		AllowlistNetworkMode: networkMode,
		EgressProxyURL:       cfg.EgressProxyURL,
	}, cfg.AgentConfigRoot, opts...)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("docker provider: %w", err)
	}
	return provider, cleanup, nil
}

func networkPolicy(cfg config) ports.ContainerNetworkPolicy {
	if len(cfg.AllowedEgress) == 0 {
		return ports.ContainerNetworkPolicy{Mode: ports.ContainerNetworkNone}
	}
	return ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: cloneStrings(cfg.AllowedEgress),
	}
}

func parseSandboxSubReport(raw json.RawMessage, candidateID string) (reportdraft.SubReport, error) {
	draft, err := reportdraft.ParseSubReport(ports.LLMResponse{
		Content:      cloneRawMessage(raw),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        sandboxModelName(candidateID),
	})
	if err != nil {
		return reportdraft.SubReport{}, fmt.Errorf("sandbox SubReport failed production validation: %w", err)
	}
	return draft, nil
}

func buildSandboxEvidence(snapshot domain.EvidenceSnapshot, cfg config) (json.RawMessage, error) {
	if len(snapshot.Digest) != sha256.Size*2 || strings.ToLower(snapshot.Digest) != snapshot.Digest {
		return nil, fmt.Errorf("evidence snapshot %d digest must be a lowercase SHA-256 digest", snapshot.ID)
	}
	if _, err := hex.DecodeString(snapshot.Digest); err != nil {
		return nil, fmt.Errorf("evidence snapshot %d digest must be a lowercase SHA-256 digest", snapshot.ID)
	}
	if err := domain.ValidateEvidenceSnapshotReportability(snapshot.Status, snapshot.MissingFields); err != nil {
		return nil, fmt.Errorf("evidence snapshot %d quality: %w", snapshot.ID, err)
	}
	payload, err := canonicalizeSnapshotPayload(snapshot.Payload)
	if err != nil {
		return nil, fmt.Errorf("evidence snapshot %d payload: %w", snapshot.ID, err)
	}
	missingFields := append([]string(nil), snapshot.MissingFields...)
	if missingFields == nil {
		missingFields = make([]string, 0)
	}
	envelope := sandboxEvidenceEnvelope{
		Schema:              "openclarion.sandbox_m4.evidence.v2",
		EvidenceSnapshotID:  int64(snapshot.ID),
		EvidenceSnapshotRef: snapshotRef(snapshot.ID),
		EvidenceDigest:      snapshot.Digest,
		PayloadSHA256:       sha256Hex(payload),
		EvidenceStatus:      string(snapshot.Status),
		MissingFields:       missingFields,
		Scenario:            cfg.Scenario,
		GroupIndex:          cfg.GroupIndex,
		Payload:             payload,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal sandbox evidence: %w", err)
	}
	if len(raw) > maxEvidenceEnvelopeLen {
		return nil, fmt.Errorf("sandbox evidence envelope is %d bytes, max %d", len(raw), maxEvidenceEnvelopeLen)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return nil, fmt.Errorf("sandbox evidence envelope: %w", err)
	}
	return raw, nil
}

func subReportDomainFromDraft(snapshotID domain.EvidenceSnapshotID, idempotencyKey string, scenario reportprompt.Scenario, candidateID string, draft reportdraft.SubReport, content json.RawMessage) (domain.SubReport, error) {
	findings, err := marshalRaw("subreport findings", draft.Findings)
	if err != nil {
		return domain.SubReport{}, err
	}
	actions, err := marshalRaw("subreport recommended_actions", draft.RecommendedActions)
	if err != nil {
		return domain.SubReport{}, err
	}
	return domain.NewSubReport(domain.SubReport{
		EvidenceSnapshotID: snapshotID,
		IdempotencyKey:     idempotencyKey,
		Scenario:           string(scenario),
		Title:              draft.Title,
		Summary:            draft.Summary,
		Severity:           domain.ReportSeverity(draft.Severity),
		Confidence:         domain.ReportConfidence(draft.Confidence),
		Findings:           findings,
		RecommendedActions: actions,
		EvidenceRefs:       append([]string(nil), draft.EvidenceRefs...),
		Content:            cloneRawMessage(content),
		Model:              sandboxModelName(candidateID),
		OutputMode:         string(ports.LLMOutputModeJSONSchema),
		CreatedByWorkflow:  toolName,
	})
}

func (s uowEvidenceStore) FindSnapshotByID(ctx context.Context, id domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error) {
	var snapshot domain.EvidenceSnapshot
	err := s.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Evidence().FindByID(ctx, id)
		if err != nil {
			return err
		}
		snapshot = got
		return nil
	})
	return snapshot, err
}

func (s uowEvidenceStore) FindSubReportByKey(ctx context.Context, snapshotID domain.EvidenceSnapshotID, idempotencyKey string) (domain.SubReport, bool, error) {
	var report domain.SubReport
	found := false
	err := s.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().FindSubReportBySnapshotAndIdempotencyKey(ctx, snapshotID, idempotencyKey)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		report = got
		found = true
		return nil
	})
	return report, found, err
}

func (s uowEvidenceStore) SaveSubReport(ctx context.Context, report domain.SubReport) (domain.SubReport, error) {
	var saved domain.SubReport
	err := s.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().SaveSubReport(ctx, report)
		if err != nil {
			return err
		}
		saved = got
		return nil
	})
	return saved, err
}

func summaryFromExisting(cfg config, snapshot domain.EvidenceSnapshot, report domain.SubReport, idempotencyKey string) generationSummary {
	return generationSummary{
		Tool:                toolName,
		SchemaID:            summarySchemaID,
		Status:              "already_exists",
		EvidenceSnapshotID:  int64(snapshot.ID),
		EvidenceSnapshotRef: snapshotRef(snapshot.ID),
		EvidenceDigest:      snapshot.Digest,
		Scenario:            cfg.Scenario,
		GroupIndex:          cfg.GroupIndex,
		CandidateID:         cfg.CandidateID,
		AgentName:           cfg.AgentName,
		SubReportID:         int64(report.ID),
		IdempotencyKey:      idempotencyKey,
		InvocationID:        "",
		OutputSHA256:        sha256Hex(report.Content),
		EvidenceRefs:        append([]string(nil), report.EvidenceRefs...),
		Created:             false,
	}
}

func summaryFromSaved(cfg config, snapshot domain.EvidenceSnapshot, report domain.SubReport, invocationID, runtimeID string, created bool) generationSummary {
	status := "created"
	if !created {
		status = "already_exists"
	}
	return generationSummary{
		Tool:                toolName,
		SchemaID:            summarySchemaID,
		Status:              status,
		EvidenceSnapshotID:  int64(snapshot.ID),
		EvidenceSnapshotRef: snapshotRef(snapshot.ID),
		EvidenceDigest:      snapshot.Digest,
		Scenario:            cfg.Scenario,
		GroupIndex:          cfg.GroupIndex,
		CandidateID:         cfg.CandidateID,
		AgentName:           cfg.AgentName,
		SubReportID:         int64(report.ID),
		IdempotencyKey:      report.IdempotencyKey,
		InvocationID:        invocationID,
		RuntimeID:           runtimeID,
		OutputSHA256:        sha256Hex(report.Content),
		EvidenceRefs:        append([]string(nil), report.EvidenceRefs...),
		Created:             created,
	}
}

func sandboxSubReportIdempotencyKey(snapshotID domain.EvidenceSnapshotID, groupIndex int, scenario reportprompt.Scenario, candidateID string) string {
	return fmt.Sprintf("snapshot:%d/group:%d/scenario:%s/sandbox:%s/sub_report", snapshotID, groupIndex, scenario, candidateID)
}

func sandboxInvocationID(snapshotID domain.EvidenceSnapshotID, groupIndex int, scenario reportprompt.Scenario, candidateID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s\x00%s", snapshotID, groupIndex, scenario, candidateID)))
	return fmt.Sprintf("m4-report/snapshot-%d/group-%d/scenario-%s/%s", snapshotID, groupIndex, scenario, hex.EncodeToString(sum[:])[:24])
}

func snapshotRef(snapshotID domain.EvidenceSnapshotID) string {
	return "snapshot:" + strconv.FormatInt(int64(snapshotID), 10)
}

func sandboxModelName(candidateID string) string {
	return "sandbox:" + candidateID
}

func validateCandidateID(candidateID string) error {
	if candidateID == "" {
		return errors.New("must be non-empty")
	}
	if strings.TrimSpace(candidateID) != candidateID {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if len([]byte(candidateID)) > maxCandidateIDBytes {
		return fmt.Errorf("must be no more than %d bytes", maxCandidateIDBytes)
	}
	if !candidateIDPattern.MatchString(candidateID) {
		return errors.New("must match [A-Za-z0-9][A-Za-z0-9._-]{0,79}")
	}
	return nil
}

func parseOptionalJSONStringArray(raw, label string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if len([]byte(raw)) > maxCommandEnvBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, maxCommandEnvBytes)
	}
	var values []string
	if err := strictjson.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("%s must be a JSON string array: %w", label, err)
	}
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s[%d] must be non-empty", label, i)
		}
	}
	return values, nil
}

func firstEnv(environ []string, keys ...string) string {
	for _, key := range keys {
		prefix := key + "="
		for _, entry := range environ {
			if strings.HasPrefix(entry, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(entry, prefix))
			}
		}
	}
	return ""
}

func csvValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func marshalRaw(label string, value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return raw, nil
}

func canonicalizeSnapshotPayload(raw json.RawMessage) (json.RawMessage, error) {
	var payload any
	if err := strictjson.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("must be strict JSON: %w", err)
	}
	if _, ok := payload.(map[string]any); !ok {
		return nil, errors.New("must be a JSON object")
	}
	return marshalRaw("canonicalize payload", payload)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func writeJSONOutput(stdout io.Writer, outPath string, value any) error {
	if outPath == "" || outPath == "-" {
		return encodeJSON(stdout, value)
	}
	var buf bytes.Buffer
	if err := encodeJSON(&buf, value); err != nil {
		return err
	}
	return writeNewOutputFile(outPath, buf.Bytes())
}

func encodeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func writeNewOutputFile(path string, raw []byte) error {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." || clean == string(filepath.Separator) {
		return errors.New("output file must not be empty, current directory, or filesystem root")
	}
	parent := filepath.Dir(clean)
	info, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("stat output parent %s: %w", parent, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("output parent %s must be a direct directory", parent)
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return fmt.Errorf("open output parent %s: %w", parent, err)
	}
	defer root.Close()
	name := filepath.Base(clean)
	if _, err := root.Lstat(name); err == nil {
		return fmt.Errorf("output file %s already exists", clean)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat output file %s: %w", clean, err)
	}
	tmp := name + ".tmp"
	file, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary output file %s: %w", clean, err)
	}
	published := false
	defer func() {
		if !published {
			_ = root.Remove(tmp)
		}
	}()
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return fmt.Errorf("write output file %s: %w", clean, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync output file %s: %w", clean, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close output file %s: %w", clean, err)
	}
	if err := root.Rename(tmp, name); err != nil {
		return fmt.Errorf("publish output file %s: %w", clean, err)
	}
	published = true
	return nil
}
