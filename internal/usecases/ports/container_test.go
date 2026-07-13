package ports

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestContainerRunRequestValidateAcceptsBatchDefaults(t *testing.T) {
	req := validContainerRequest()
	req.Timeout = 0
	req.OutputMax = 0
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if req.EffectiveTimeout() != DefaultContainerRunTimeout {
		t.Fatalf("EffectiveTimeout = %s, want %s", req.EffectiveTimeout(), DefaultContainerRunTimeout)
	}
	if req.EffectiveOutputMax() != DefaultContainerOutputBytes {
		t.Fatalf("EffectiveOutputMax = %d, want %d", req.EffectiveOutputMax(), DefaultContainerOutputBytes)
	}
	if req.Network.EffectiveMode() != ContainerNetworkNone {
		t.Fatalf("EffectiveMode = %q, want %q", req.Network.EffectiveMode(), ContainerNetworkNone)
	}
}

func TestContainerRunRequestValidateAcceptsM5TurnFilesAndAllowlist(t *testing.T) {
	req := validContainerRequest()
	req.Conversation = json.RawMessage(`[{"role":"assistant","content":"previous"}]`)
	req.Message = json.RawMessage(`{"role":"user","content":"what changed?"}`)
	req.Network = ContainerNetworkPolicy{
		Mode:          ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090", "api.openai.com"},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestNormalizeContainerEgressTargets(t *testing.T) {
	got, err := NormalizeContainerEgressTargets([]string{"Prometheus.Internal:9090", "api.openai.com"})
	if err != nil {
		t.Fatalf("NormalizeContainerEgressTargets: %v", err)
	}
	want := []string{"prometheus.internal:9090", "api.openai.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("NormalizeContainerEgressTargets = %v, want %v", got, want)
	}
}

func TestValidateContainerEgressURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		allowed []string
		wantErr string
	}{
		{
			name:    "default https port by bare host",
			rawURL:  "https://llm.example.invalid/v1",
			allowed: []string{"LLM.EXAMPLE.INVALID"},
		},
		{
			name:    "explicit non-default port",
			rawURL:  "http://llm.example.invalid:8080/v1",
			allowed: []string{"llm.example.invalid:8080"},
		},
		{
			name:    "wrong port",
			rawURL:  "https://llm.example.invalid:8443/v1",
			allowed: []string{"llm.example.invalid:443"},
			wantErr: "host must be listed",
		},
		// #nosec G101 -- test-only credential-bearing URL verifies rejection.
		{
			name:    "userinfo",
			rawURL:  "https://user:secret@llm.example.invalid/v1",
			allowed: []string{"llm.example.invalid"},
			wantErr: "without userinfo",
		},
		{
			name:    "surrounding whitespace",
			rawURL:  " https://llm.example.invalid/v1",
			allowed: []string{"llm.example.invalid"},
			wantErr: "surrounding whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerEgressURL(tt.rawURL, tt.allowed)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateContainerEgressURL: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateContainerEgressURL err = %v, want %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked URL credential: %v", err)
			}
		})
	}
}

func TestContainerRunRequestValidateAcceptsShortLivedCredentials(t *testing.T) {
	now := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
	req := validContainerRequest()
	req.Credentials = []ContainerCredential{{
		Name:      "OPENCLARION_PROVIDER_TOKEN",
		Value:     "value-for-test",
		ExpiresAt: now.Add(req.EffectiveTimeout()),
	}}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := req.ValidateCredentialExpirations(now); err != nil {
		t.Fatalf("ValidateCredentialExpirations: %v", err)
	}
}

func TestContainerRunRequestValidateRejectsInvalidInputs(t *testing.T) {
	now := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		mutate  func(*ContainerRunRequest)
		wantErr string
	}{
		{
			name:    "missing invocation id",
			mutate:  func(req *ContainerRunRequest) { req.InvocationID = "" },
			wantErr: "invocation id is required",
		},
		{
			name:    "unsafe invocation id",
			mutate:  func(req *ContainerRunRequest) { req.InvocationID = "../escape" },
			wantErr: "invocation id",
		},
		{
			name:    "unsafe agent name",
			mutate:  func(req *ContainerRunRequest) { req.AgentName = "Report Enhancer" },
			wantErr: "agent name",
		},
		{
			name:    "evidence must be object",
			mutate:  func(req *ContainerRunRequest) { req.Evidence = json.RawMessage(`[]`) },
			wantErr: "evidence JSON must be an object",
		},
		{
			name:    "evidence duplicate object key",
			mutate:  func(req *ContainerRunRequest) { req.Evidence = json.RawMessage(`{"snapshot_id":11,"snapshot_id":12}`) },
			wantErr: `duplicate object key "snapshot_id"`,
		},
		{
			name: "conversation nested duplicate object key",
			mutate: func(req *ContainerRunRequest) {
				req.Conversation = json.RawMessage(`[{"role":"assistant","role":"user"}]`)
			},
			wantErr: `duplicate object key "role"`,
		},
		{
			name:    "invalid message json",
			mutate:  func(req *ContainerRunRequest) { req.Message = json.RawMessage(`{`) },
			wantErr: "message JSON is invalid",
		},
		{
			name:    "message trailing json value",
			mutate:  func(req *ContainerRunRequest) { req.Message = json.RawMessage(`{"role":"user"} {"role":"assistant"}`) },
			wantErr: "trailing JSON values",
		},
		{
			name:    "timeout too long",
			mutate:  func(req *ContainerRunRequest) { req.Timeout = MaxContainerRunTimeout + time.Second },
			wantErr: "exceeds maximum",
		},
		{
			name:    "output max too large",
			mutate:  func(req *ContainerRunRequest) { req.OutputMax = MaxContainerOutputBytes + 1 },
			wantErr: "output max",
		},
		{
			name:    "egress with network none",
			mutate:  func(req *ContainerRunRequest) { req.Network.AllowedEgress = []string{"prometheus.internal:9090"} },
			wantErr: "allowed egress requires network mode",
		},
		{
			name: "allowlist without targets",
			mutate: func(req *ContainerRunRequest) {
				req.Network = ContainerNetworkPolicy{Mode: ContainerNetworkAllowlist}
			},
			wantErr: "requires at least one egress target",
		},
		{
			name: "url egress target",
			mutate: func(req *ContainerRunRequest) {
				req.Network = ContainerNetworkPolicy{Mode: ContainerNetworkAllowlist, AllowedEgress: []string{"https://prometheus.internal"}}
			},
			wantErr: "not a URL",
		},
		{
			name: "egress target with whitespace",
			mutate: func(req *ContainerRunRequest) {
				req.Network = ContainerNetworkPolicy{Mode: ContainerNetworkAllowlist, AllowedEgress: []string{" prometheus.internal:9090"}}
			},
			wantErr: "whitespace",
		},
		{
			name: "egress target with path",
			mutate: func(req *ContainerRunRequest) {
				req.Network = ContainerNetworkPolicy{Mode: ContainerNetworkAllowlist, AllowedEgress: []string{"prometheus.internal:9090/api/v1/query"}}
			},
			wantErr: "host[:port]",
		},
		{
			name: "egress target invalid port",
			mutate: func(req *ContainerRunRequest) {
				req.Network = ContainerNetworkPolicy{Mode: ContainerNetworkAllowlist, AllowedEgress: []string{"prometheus.internal:port"}}
			},
			wantErr: "invalid port",
		},
		{
			name: "egress target wildcard",
			mutate: func(req *ContainerRunRequest) {
				req.Network = ContainerNetworkPolicy{Mode: ContainerNetworkAllowlist, AllowedEgress: []string{"*.example.com"}}
			},
			wantErr: "wildcards",
		},
		{
			name: "egress target duplicate after normalization",
			mutate: func(req *ContainerRunRequest) {
				req.Network = ContainerNetworkPolicy{Mode: ContainerNetworkAllowlist, AllowedEgress: []string{"Prometheus.Internal:9090", "prometheus.internal:9090"}}
			},
			wantErr: "duplicate",
		},
		{
			name: "unsafe credential name",
			mutate: func(req *ContainerRunRequest) {
				req.Credentials = []ContainerCredential{{Name: "provider-token", Value: "value-for-test", ExpiresAt: now.Add(time.Minute)}}
			},
			wantErr: "credential name",
		},
		{
			name: "credential name with whitespace",
			mutate: func(req *ContainerRunRequest) {
				req.Credentials = []ContainerCredential{{Name: " OPENCLARION_PROVIDER_TOKEN", Value: "value-for-test", ExpiresAt: now.Add(time.Minute)}}
			},
			wantErr: "credential name",
		},
		{
			name: "credential value missing",
			mutate: func(req *ContainerRunRequest) {
				req.Credentials = []ContainerCredential{{Name: "OPENCLARION_PROVIDER_TOKEN", ExpiresAt: now.Add(time.Minute)}}
			},
			wantErr: "value is required",
		},
		{
			name: "credential expiry missing",
			mutate: func(req *ContainerRunRequest) {
				req.Credentials = []ContainerCredential{{Name: "OPENCLARION_PROVIDER_TOKEN", Value: "value-for-test"}}
			},
			wantErr: "expiry is required",
		},
		{
			name: "duplicate credential name",
			mutate: func(req *ContainerRunRequest) {
				req.Credentials = []ContainerCredential{
					{Name: "OPENCLARION_PROVIDER_TOKEN", Value: "value-one", ExpiresAt: now.Add(time.Minute)},
					{Name: "OPENCLARION_PROVIDER_TOKEN", Value: "value-two", ExpiresAt: now.Add(time.Minute)},
				}
			},
			wantErr: "duplicate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validContainerRequest()
			tt.mutate(&req)
			err := req.Validate()
			if err == nil {
				t.Fatal("Validate err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestContainerRunRequestValidateCredentialExpirationsRejectsLongLivedOrExpiredCredentials(t *testing.T) {
	now := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		expires time.Time
		wantErr string
	}{
		{
			name:    "expired",
			expires: now.Add(-time.Second),
			wantErr: "expired",
		},
		{
			name:    "exceeds container timeout",
			expires: now.Add(validContainerRequest().EffectiveTimeout() + time.Second),
			wantErr: "exceeds container timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validContainerRequest()
			req.Credentials = []ContainerCredential{{
				Name:      "OPENCLARION_PROVIDER_TOKEN",
				Value:     "value-for-test",
				ExpiresAt: tt.expires,
			}}
			if err := req.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			err := req.ValidateCredentialExpirations(now)
			if err == nil {
				t.Fatal("ValidateCredentialExpirations err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateCredentialExpirations err = %v, want containing %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "value-for-test") {
				t.Fatalf("ValidateCredentialExpirations leaked credential value: %v", err)
			}
		})
	}
}

func TestValidateContainerRunResult(t *testing.T) {
	req := validContainerRequest()
	result := validContainerResult(req)
	if err := ValidateContainerRunResult(req, result); err != nil {
		t.Fatalf("ValidateContainerRunResult: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*ContainerRunResult)
		wantErr string
	}{
		{
			name:    "wrong invocation id",
			mutate:  func(result *ContainerRunResult) { result.InvocationID = "other" },
			wantErr: "result invocation id",
		},
		{
			name:    "nonzero exit",
			mutate:  func(result *ContainerRunResult) { result.ExitCode = 1 },
			wantErr: "exit code",
		},
		{
			name:    "missing output",
			mutate:  func(result *ContainerRunResult) { result.Output = nil },
			wantErr: "output is required",
		},
		{
			name:    "invalid output",
			mutate:  func(result *ContainerRunResult) { result.Output = json.RawMessage(`{`) },
			wantErr: "not valid JSON",
		},
		{
			name:    "duplicate output key",
			mutate:  func(result *ContainerRunResult) { result.Output = json.RawMessage(`{"summary":"old","summary":"new"}`) },
			wantErr: `duplicate object key "summary"`,
		},
		{
			name: "trailing output value",
			mutate: func(result *ContainerRunResult) {
				result.Output = json.RawMessage(`{"summary":"ok"} {"summary":"again"}`)
			},
			wantErr: "trailing JSON values",
		},
		{
			name: "output too large",
			mutate: func(result *ContainerRunResult) {
				result.Output = json.RawMessage(`{"result":"too-large"}`)
			},
			wantErr: "exceeds maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validContainerRequest()
			if tt.name == "output too large" {
				req.OutputMax = 2
			}
			result := validContainerResult(req)
			tt.mutate(&result)
			err := ValidateContainerRunResult(req, result)
			if err == nil {
				t.Fatal("ValidateContainerRunResult err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateContainerRunResult err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func validContainerRequest() ContainerRunRequest {
	return ContainerRunRequest{
		InvocationID: "snapshot-11/group-0",
		AgentName:    "report-enhancer",
		Evidence:     json.RawMessage(`{"snapshot_id":11,"alerts":[]}`),
		Timeout:      time.Minute,
		OutputMax:    1024,
	}
}

func validContainerResult(req ContainerRunRequest) ContainerRunResult {
	startedAt := time.Date(2026, 5, 28, 6, 0, 0, 0, time.UTC)
	return ContainerRunResult{
		InvocationID: req.InvocationID,
		AgentName:    req.AgentName,
		Output:       json.RawMessage(`{"summary":"ok"}`),
		ExitCode:     0,
		StartedAt:    startedAt,
		FinishedAt:   startedAt.Add(time.Second),
		RuntimeID:    "container-1",
	}
}
