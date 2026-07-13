package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

type fakeStore struct {
	snapshot domain.EvidenceSnapshot
	reports  map[string]domain.SubReport
	saved    []domain.SubReport
	saveErr  error
}

type fakeProvider struct {
	output json.RawMessage
	reqs   []ports.ContainerRunRequest
	err    error
}

func TestGenerateRunsSandboxAndPersistsSubReport(t *testing.T) {
	store := &fakeStore{snapshot: validSnapshot(), reports: map[string]domain.SubReport{}}
	provider := &fakeProvider{output: validSandboxSubReport("snapshot:11")}
	cfg := validConfig()

	got, err := generate(context.Background(), cfg, store, provider)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got.Status != "created" || !got.Created {
		t.Fatalf("summary status = %q created=%v", got.Status, got.Created)
	}
	if got.SubReportID == 0 {
		t.Fatal("summary SubReportID = 0")
	}
	wantKey := "snapshot:11/group:2/sandbox:custom-thin-runner/sub_report"
	if got.IdempotencyKey != wantKey {
		t.Fatalf("IdempotencyKey = %q, want %q", got.IdempotencyKey, wantKey)
	}
	if len(provider.reqs) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.reqs))
	}
	req := provider.reqs[0]
	if req.AgentName != defaultAgentName {
		t.Fatalf("AgentName = %q", req.AgentName)
	}
	if req.Network.EffectiveMode() != ports.ContainerNetworkNone {
		t.Fatalf("Network = %#v, want network-none", req.Network)
	}
	var envelope sandboxEvidenceEnvelope
	if err := json.Unmarshal(req.Evidence, &envelope); err != nil {
		t.Fatalf("unmarshal sandbox evidence: %v", err)
	}
	if envelope.EvidenceSnapshotRef != "snapshot:11" ||
		envelope.EvidenceDigest != "digest-abc" ||
		envelope.Scenario != string(reportprompt.ScenarioCascade) ||
		envelope.GroupIndex != 2 ||
		!bytes.Contains(envelope.Payload, []byte(`"alert:cpu"`)) {
		t.Fatalf("unexpected envelope: %+v payload=%s", envelope, envelope.Payload)
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved count = %d, want 1", len(store.saved))
	}
	saved := store.saved[0]
	if saved.IdempotencyKey != wantKey ||
		saved.Model != "sandbox:custom-thin-runner" ||
		saved.OutputMode != string(ports.LLMOutputModeJSONSchema) ||
		saved.CreatedByWorkflow != toolName ||
		!containsString(saved.EvidenceRefs, "snapshot:11") {
		t.Fatalf("unexpected saved report: %+v", saved)
	}
}

func TestGenerateReturnsExistingSubReportWithoutSandboxRun(t *testing.T) {
	cfg := validConfig()
	key := sandboxSubReportIdempotencyKey(11, cfg.GroupIndex, cfg.CandidateID)
	existing := validDomainSubReport(t, key)
	existing.ID = 55
	store := &fakeStore{
		snapshot: validSnapshot(),
		reports:  map[string]domain.SubReport{key: existing},
	}
	provider := &fakeProvider{err: errors.New("must not run")}

	got, err := generate(context.Background(), cfg, store, provider)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got.Status != "already_exists" || got.Created || got.SubReportID != 55 {
		t.Fatalf("summary = %+v", got)
	}
	if len(provider.reqs) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(provider.reqs))
	}
}

func TestGenerateRejectsSandboxOutputWithoutSnapshotRef(t *testing.T) {
	store := &fakeStore{snapshot: validSnapshot(), reports: map[string]domain.SubReport{}}
	provider := &fakeProvider{output: validSandboxSubReport("alert:cpu")}

	_, err := generate(context.Background(), validConfig(), store, provider)
	if err == nil {
		t.Fatal("generate err = nil, want snapshot ref error")
	}
	if !strings.Contains(err.Error(), `evidence_refs must include "snapshot:11"`) {
		t.Fatalf("err = %v, want snapshot ref error", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved count = %d, want 0", len(store.saved))
	}
}

func TestParseConfigReadsManualSandboxEnv(t *testing.T) {
	env := []string{
		"DATABASE_URL=postgres://openclarion@localhost:5432/openclarion?sslmode=disable",
		"OPENCLARION_M4_SANDBOX_IMAGE_REF=registry.example.com/openclarion/runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"OPENCLARION_M4_SANDBOX_AGENT_CONFIG_ROOT=/tmp/agents",
		`OPENCLARION_M4_SANDBOX_COMMAND_JSON=["/bin/runner","--mode","report"]`,
		"OPENCLARION_M4_SANDBOX_EGRESS_ALLOWED=prometheus.internal:9090, topology.internal:8080",
		"OPENCLARION_M4_SANDBOX_EGRESS_NETWORK=openclarion-sandbox-egress-prod",
		"OPENCLARION_M4_SANDBOX_EGRESS_PROXY_URL=http://openclarion-egress-proxy:18080",
	}
	cfg, err := parseConfig([]string{
		"--snapshot-id", "11",
		"--scenario", "single_alert",
		"--group-index", "1",
		"--candidate-id", "candidate-a",
		"--out", "artifacts/m4/sandbox-subreport.json",
	}, env)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.SnapshotID != 11 ||
		cfg.Scenario != "single_alert" ||
		cfg.GroupIndex != 1 ||
		cfg.CandidateID != "candidate-a" ||
		strings.Join(cfg.Command, " ") != "/bin/runner --mode report" ||
		len(cfg.AllowedEgress) != 2 ||
		cfg.EgressNetwork != "openclarion-sandbox-egress-prod" ||
		cfg.EgressProxyURL != "http://openclarion-egress-proxy:18080" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestParseConfigRequiresProxyForAllowedEgress(t *testing.T) {
	env := []string{
		"DATABASE_URL=postgres://openclarion@localhost:5432/openclarion?sslmode=disable",
		"OPENCLARION_M4_SANDBOX_IMAGE_REF=registry.example.com/openclarion/runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"OPENCLARION_M4_SANDBOX_AGENT_CONFIG_ROOT=/tmp/agents",
		"OPENCLARION_M4_SANDBOX_EGRESS_ALLOWED=prometheus.internal:9090",
	}
	_, err := parseConfig([]string{
		"--snapshot-id", "11",
		"--scenario", "single_alert",
		"--candidate-id", "candidate-a",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "OPENCLARION_M4_SANDBOX_EGRESS_PROXY_URL") {
		t.Fatalf("parseConfig err = %v, want missing proxy URL", err)
	}
}

func TestParseConfigRejectsInvalidCandidateID(t *testing.T) {
	env := []string{
		"DATABASE_URL=postgres://openclarion@localhost:5432/openclarion?sslmode=disable",
		"OPENCLARION_M4_SANDBOX_IMAGE_REF=registry.example.com/openclarion/runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"OPENCLARION_M4_SANDBOX_AGENT_CONFIG_ROOT=/tmp/agents",
	}
	_, err := parseConfig([]string{
		"--snapshot-id", "11",
		"--scenario", "single_alert",
		"--candidate-id", "bad candidate",
	}, env)
	if err == nil {
		t.Fatal("parseConfig err = nil, want invalid candidate id")
	}
	if !strings.Contains(err.Error(), "--candidate-id") {
		t.Fatalf("err = %v, want candidate id error", err)
	}
}

func (s *fakeStore) FindSnapshotByID(_ context.Context, id domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error) {
	if s.snapshot.ID != id {
		return domain.EvidenceSnapshot{}, domain.ErrNotFound
	}
	return s.snapshot, nil
}

func (s *fakeStore) FindSubReportByKey(_ context.Context, _ domain.EvidenceSnapshotID, key string) (domain.SubReport, bool, error) {
	if s.reports == nil {
		return domain.SubReport{}, false, nil
	}
	report, ok := s.reports[key]
	return report, ok, nil
}

func (s *fakeStore) SaveSubReport(_ context.Context, report domain.SubReport) (domain.SubReport, error) {
	if s.saveErr != nil {
		return domain.SubReport{}, s.saveErr
	}
	if s.reports == nil {
		s.reports = map[string]domain.SubReport{}
	}
	if _, exists := s.reports[report.IdempotencyKey]; exists {
		return domain.SubReport{}, domain.ErrAlreadyExists
	}
	report.ID = domain.SubReportID(len(s.saved) + 100)
	report.CreatedAt = time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	s.reports[report.IdempotencyKey] = report
	s.saved = append(s.saved, report)
	return report, nil
}

func (p *fakeProvider) Run(ctx context.Context, req ports.ContainerRunRequest) (ports.ContainerRunResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.ContainerRunResult{}, err
	}
	p.reqs = append(p.reqs, req)
	if p.err != nil {
		return ports.ContainerRunResult{}, p.err
	}
	return ports.ContainerRunResult{
		InvocationID: req.InvocationID,
		AgentName:    req.AgentName,
		Output:       cloneRawMessage(p.output),
		ExitCode:     0,
		StartedAt:    time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC),
		FinishedAt:   time.Date(2026, 6, 4, 9, 0, 1, 0, time.UTC),
		RuntimeID:    "container-1",
	}, nil
}

func validConfig() config {
	return config{
		SnapshotID:      11,
		Scenario:        string(reportprompt.ScenarioCascade),
		GroupIndex:      2,
		CandidateID:     "custom-thin-runner",
		AgentName:       defaultAgentName,
		DatabaseURL:     "postgres://openclarion@localhost:5432/openclarion?sslmode=disable",
		ImageRef:        "registry.example.com/openclarion/runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentConfigRoot: "/tmp/agents",
		Timeout:         ports.DefaultContainerRunTimeout,
		OutputMax:       ports.DefaultContainerOutputBytes,
	}
}

func validSnapshot() domain.EvidenceSnapshot {
	return domain.EvidenceSnapshot{
		ID:           11,
		AlertGroupID: 7,
		Digest:       "digest-abc",
		Payload:      json.RawMessage(`{"schema_version":"evidence.snapshot.v1","alerts":[{"id":"alert:cpu"}]}`),
		Status:       domain.SnapshotStatusComplete,
	}
}

func validSandboxSubReport(firstRef string) json.RawMessage {
	refs := quotedRefs(firstRef, "alert:cpu")
	if firstRef == "alert:cpu" {
		refs = quotedRefs("alert:cpu")
	}
	return json.RawMessage(`{
		"title":"CPU saturation",
		"summary":"CPU is saturated in the payments service.",
		"severity":"warning",
		"confidence":"high",
		"findings":[{"label":"CPU","detail":"CPU is above threshold.","evidence_id":"alert:cpu"}],
		"recommended_actions":[{"label":"Scale","detail":"Add one replica.","priority":"medium"}],
		"evidence_refs":[` + refs + `]
	}`)
}

func quotedRefs(values ...string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			panic(err)
		}
		quoted[i] = string(raw)
	}
	return strings.Join(quoted, ",")
}

func validDomainSubReport(t *testing.T, key string) domain.SubReport {
	t.Helper()
	report, err := subReportDomainFromDraft(
		11,
		key,
		reportprompt.ScenarioCascade,
		"custom-thin-runner",
		mustParseSubReport(t, validSandboxSubReport("snapshot:11")),
		validSandboxSubReport("snapshot:11"),
	)
	if err != nil {
		t.Fatalf("subReportDomainFromDraft: %v", err)
	}
	return report
}

func mustParseSubReport(t *testing.T, raw json.RawMessage) reportdraft.SubReport {
	t.Helper()
	draft, err := parseSandboxSubReport(raw, "custom-thin-runner")
	if err != nil {
		t.Fatalf("parseSandboxSubReport: %v", err)
	}
	return draft
}
