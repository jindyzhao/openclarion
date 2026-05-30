// Package main wires the OpenClarion HTTP server, Temporal worker,
// and persistence dependencies into the runtime binary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	temporalclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/interceptor"
	temporallog "go.temporal.io/sdk/log"
	"golang.org/x/sync/errgroup"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/observability/accesslog"
	"github.com/openclarion/openclarion/internal/observability/correlation"
	observabilitymetrics "github.com/openclarion/openclarion/internal/observability/metrics"
	observabilitytracing "github.com/openclarion/openclarion/internal/observability/tracing"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/persistence/repository"
	authoidc "github.com/openclarion/openclarion/internal/providers/auth/oidc"
	containerdocker "github.com/openclarion/openclarion/internal/providers/container/docker"
	imwebhook "github.com/openclarion/openclarion/internal/providers/im/webhook"
	openaillm "github.com/openclarion/openclarion/internal/providers/llm/openai"
	metricsprometheus "github.com/openclarion/openclarion/internal/providers/metrics/prometheus"
	transporthttp "github.com/openclarion/openclarion/internal/transport/http"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

type getenvFunc func(string) string

const (
	reportReplayCLICommand           = "report-replay"
	reportReplayCLICreatedByWorkflow = "ReportReplayCLI"
	defaultReportReplayCLILimit      = 10000
	defaultReportReplayCLIWait       = 20 * time.Minute
)

var reportReplayCLINowUTC = func() time.Time {
	return time.Now().UTC()
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if err := dispatch(context.Background(), logger, os.Args[1:], os.Stdout); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func dispatch(ctx context.Context, logger *slog.Logger, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return run(ctx, logger)
	}
	switch args[0] {
	case "serve":
		return run(ctx, logger)
	case reportReplayCLICommand:
		return runReportReplayCLI(ctx, logger, os.Getenv, args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q (expected: serve or %s)", args[0], reportReplayCLICommand)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Database wiring is mandatory: the binary refuses to start
	// without DATABASE_URL so misconfiguration fails fast at boot
	// rather than at the first persistence call. OpenPostgres pings
	// the server with a 5s timeout to make that promise true (a bad
	// DSN, unreachable host, or wrong credentials all surface here
	// rather than on the first request).
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	client, err := repository.OpenPostgres(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() {
		if cerr := client.Close(); cerr != nil {
			logger.Warn("close ent client", "error", cerr)
		}
	}()

	uowFactory := repository.NewFactory(client)

	traceConfig, err := observabilitytracing.ConfigFromEnv(os.Getenv)
	if err != nil {
		return err
	}
	httpTracing, err := observabilitytracing.NewHTTPTracing(ctx, traceConfig)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpTracing.Shutdown(shutdownCtx); err != nil {
			logger.Warn("shutdown OpenTelemetry tracing", "error", err)
		}
	}()
	if httpTracing.Enabled() {
		logger.Info("configured OpenTelemetry tracing", "service", traceConfig.ServiceName)
	} else {
		logger.Info("OpenTelemetry tracing disabled; set OTEL_EXPORTER_OTLP_ENDPOINT or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT to enable")
	}

	temporalAddr := envOrDefault("TEMPORAL_HOST_PORT", "localhost:7233")
	temporalInterceptors, err := temporalClientInterceptors(httpTracing)
	if err != nil {
		return err
	}
	tc, err := temporalclient.Dial(temporalclient.Options{
		HostPort:     temporalAddr,
		Logger:       temporallog.NewStructuredLogger(logger),
		Interceptors: temporalInterceptors,
	})
	if err != nil {
		return fmt.Errorf("dial temporal: %w", err)
	}
	defer tc.Close()

	activityOptions, err := reportActivityOptionsFromEnv(ctx, logger, os.Getenv, httpTracing)
	if err != nil {
		return err
	}
	diagnosisActivityOptions, err := diagnosisActivityOptionsFromEnv(logger, os.Getenv)
	if err != nil {
		return err
	}
	activityOptions = append(activityOptions, diagnosisActivityOptions...)
	w := temporalpkg.NewWorker(tc, uowFactory, activityOptions...)
	if err := w.Start(); err != nil {
		return fmt.Errorf("start temporal worker: %w", err)
	}
	defer w.Stop()

	reportStarter, err := temporalpkg.NewReportStarter(tc)
	if err != nil {
		return err
	}
	diagnosisRoomClient, err := temporalpkg.NewDiagnosisRoomClient(tc)
	if err != nil {
		return err
	}
	diagnosisRoomStarter, err := temporalpkg.NewDiagnosisRoomStarter(tc)
	if err != nil {
		return err
	}
	ticketStore, err := repository.NewDiagnosisAuthTicketStore(client)
	if err != nil {
		return fmt.Errorf("configure diagnosis WebSocket ticket store: %w", err)
	}
	serverOptions, originPolicy, err := httpServerOptionsFromEnv(logger, os.Getenv, uowFactory, reportStarter, diagnosisRoomClient, diagnosisRoomStarter, ticketStore, httpTracing)
	if err != nil {
		return err
	}

	addr := envOrDefault("LISTEN_ADDR", ":8080")

	mux := http.NewServeMux()
	httpMetrics := observabilitymetrics.NewHTTPMetrics()
	mux.Handle("GET /metrics", httpMetrics.Handler())

	// Wire the generated ServerInterface handler.
	server := transporthttp.NewServer(logger, uowFactory, serverOptions...)
	apiMiddlewares := []api.MiddlewareFunc{
		httpMetrics.Middleware("api"),
		accesslog.Middleware(logger),
		correlation.Middleware(),
		httpTracing.Middleware("api"),
	}
	server.RegisterDiagnosisWebSocketRoutes(mux, apiMiddlewares...)
	var handler http.Handler = api.HandlerWithOptions(server, api.StdHTTPServerOptions{
		BaseRouter:       mux,
		Middlewares:      apiMiddlewares,
		ErrorHandlerFunc: transporthttp.OpenAPIErrorHandler(logger),
	})
	if originPolicy != nil {
		handler = originPolicy.Middleware(handler)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		logger.Info("starting HTTP server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		<-groupCtx.Done()
		if ctx.Err() == nil {
			return nil
		}
		logger.Info("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})
	return group.Wait()
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func reportActivityOptionsFromEnv(
	ctx context.Context,
	logger *slog.Logger,
	getenv getenvFunc,
	httpTracing *observabilitytracing.HTTPTracing,
) ([]temporalpkg.ActivityOption, error) {
	var opts []temporalpkg.ActivityOption

	llmConfigured := anyEnv(getenv,
		"OPENCLARION_LLM_MODEL",
		"OPENCLARION_LLM_BASE_URL",
		"OPENCLARION_LLM_API_KEY",
	)
	if llmConfigured {
		model := strings.TrimSpace(getenv("OPENCLARION_LLM_MODEL"))
		if model == "" {
			return nil, fmt.Errorf("OPENCLARION_LLM_MODEL is required when configuring the report LLM provider")
		}
		provider, err := openaillm.NewProviderWithCapabilityDetection(ctx, openaillm.Config{
			BaseURL:    strings.TrimSpace(getenv("OPENCLARION_LLM_BASE_URL")),
			APIKey:     strings.TrimSpace(getenv("OPENCLARION_LLM_API_KEY")),
			Model:      model,
			HTTPClient: outboundHTTPClient(httpTracing, 30*time.Second),
		})
		if err != nil {
			return nil, fmt.Errorf("configure report LLM provider: %w", err)
		}
		opts = append(opts, temporalpkg.WithLLMProvider(provider))
		logger.Info("configured report LLM provider", "provider", "openai-compatible", "output_mode", provider.OutputMode())
	}

	imConfigured := anyEnv(getenv,
		"OPENCLARION_IM_WEBHOOK_URL",
		"OPENCLARION_IM_WEBHOOK_BEARER_TOKEN",
	)
	if imConfigured {
		url := strings.TrimSpace(getenv("OPENCLARION_IM_WEBHOOK_URL"))
		if url == "" {
			return nil, fmt.Errorf("OPENCLARION_IM_WEBHOOK_URL is required when configuring the report IM provider")
		}
		provider, err := imwebhook.NewProvider(imwebhook.Config{
			URL:         url,
			BearerToken: strings.TrimSpace(getenv("OPENCLARION_IM_WEBHOOK_BEARER_TOKEN")),
			HTTPClient:  outboundHTTPClient(httpTracing, 10*time.Second),
		})
		if err != nil {
			return nil, fmt.Errorf("configure report IM provider: %w", err)
		}
		opts = append(opts, temporalpkg.WithIMProvider(provider))
		logger.Info("configured report IM provider", "provider", "webhook")
	}

	if !llmConfigured || !imConfigured {
		logger.Warn("report provider wiring is incomplete; report workflows require OPENCLARION_LLM_* and OPENCLARION_IM_WEBHOOK_* configuration before production use")
	}
	return opts, nil
}

func diagnosisActivityOptionsFromEnv(
	logger *slog.Logger,
	getenv getenvFunc,
) ([]temporalpkg.ActivityOption, error) {
	sandboxConfigured := anyEnv(getenv,
		"OPENCLARION_SANDBOX_IMAGE_REF",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT",
		"OPENCLARION_SANDBOX_COMMAND_JSON",
		"OPENCLARION_SANDBOX_WORKSPACE_ROOT",
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED",
		"OPENCLARION_SANDBOX_EGRESS_NETWORK",
	)
	if !sandboxConfigured {
		logger.Warn("diagnosis sandbox provider is not configured; diagnosis-room turns require OPENCLARION_SANDBOX_IMAGE_REF and OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT before live use")
		return nil, nil
	}

	imageRef := strings.TrimSpace(getenv("OPENCLARION_SANDBOX_IMAGE_REF"))
	if imageRef == "" {
		return nil, fmt.Errorf("OPENCLARION_SANDBOX_IMAGE_REF is required when configuring the diagnosis sandbox provider")
	}
	agentConfigRoot := strings.TrimSpace(getenv("OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT"))
	if agentConfigRoot == "" {
		return nil, fmt.Errorf("OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT is required when configuring the diagnosis sandbox provider")
	}
	command, err := parseOptionalJSONStringArray(getenv("OPENCLARION_SANDBOX_COMMAND_JSON"), "OPENCLARION_SANDBOX_COMMAND_JSON")
	if err != nil {
		return nil, err
	}

	var providerOpts []containerdocker.ProviderOption
	if workspaceRoot := strings.TrimSpace(getenv("OPENCLARION_SANDBOX_WORKSPACE_ROOT")); workspaceRoot != "" {
		providerOpts = append(providerOpts, containerdocker.WithWorkspaceRoot(workspaceRoot))
	}
	allowedEgress := csvValues(getenv("OPENCLARION_SANDBOX_EGRESS_ALLOWED"))
	if len(allowedEgress) > 0 {
		networkMode := strings.TrimSpace(getenv("OPENCLARION_SANDBOX_EGRESS_NETWORK"))
		if networkMode == "" {
			networkMode = containerdocker.DefaultAllowlistNetworkMode
		}
		enforcer, err := containerdocker.NewStaticAllowlistEnforcer(networkMode, allowedEgress)
		if err != nil {
			return nil, fmt.Errorf("configure sandbox egress enforcer: %w", err)
		}
		providerOpts = append(providerOpts, containerdocker.WithEgressEnforcer(enforcer))
	}

	provider, err := containerdocker.NewProviderFromEnv(containerdocker.Config{
		ImageRef:        imageRef,
		ReadonlyRootFS:  true,
		NoNewPrivileges: true,
		Command:         command,
	}, agentConfigRoot, providerOpts...)
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis sandbox provider: %w", err)
	}
	logger.Info("configured diagnosis sandbox provider", "provider", "docker")
	return []temporalpkg.ActivityOption{temporalpkg.WithContainerProvider(provider)}, nil
}

func httpServerOptionsFromEnv(
	logger *slog.Logger,
	getenv getenvFunc,
	uowFactory ports.UnitOfWorkFactory,
	starter ports.ReportWorkflowStarter,
	diagnosisWorkflows ports.DiagnosisRoomWorkflowClient,
	diagnosisStarter ports.DiagnosisRoomWorkflowStarter,
	diagnosisTickets diagnosisauth.Store,
	httpTracing *observabilitytracing.HTTPTracing,
) ([]transporthttp.ServerOption, *browserOriginPolicy, error) {
	var opts []transporthttp.ServerOption
	originPolicy, err := browserOriginPolicyFromEnv(getenv)
	if err != nil {
		return nil, nil, err
	}

	if !anyEnv(getenv, "OPENCLARION_PROMETHEUS_URL", "OPENCLARION_PROMETHEUS_BEARER_TOKEN") {
		logger.Warn("report HTTP trigger is disabled; set OPENCLARION_PROMETHEUS_URL to enable replay-window report triggers")
	} else {
		prometheusURL := strings.TrimSpace(getenv("OPENCLARION_PROMETHEUS_URL"))
		if prometheusURL == "" {
			return nil, nil, fmt.Errorf("OPENCLARION_PROMETHEUS_URL is required when configuring the report HTTP trigger")
		}
		provider, err := metricsprometheus.NewProvider(
			prometheusURL,
			metricsprometheus.WithBearer(strings.TrimSpace(getenv("OPENCLARION_PROMETHEUS_BEARER_TOKEN"))),
			metricsprometheus.WithRoundTripperDecorator(outboundTransportDecorator(httpTracing)),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("configure report HTTP trigger metrics provider: %w", err)
		}
		service, err := reporttrigger.NewService(provider, uowFactory, starter)
		if err != nil {
			return nil, nil, fmt.Errorf("configure report HTTP trigger service: %w", err)
		}
		logger.Info("configured report HTTP trigger", "provider", "prometheus")
		opts = append(opts, transporthttp.WithReportReplayTrigger(service))
	}

	diagnosisOpts, err := diagnosisServerOptionsFromEnv(
		logger,
		getenv,
		uowFactory,
		diagnosisWorkflows,
		diagnosisStarter,
		diagnosisTickets,
		originPolicy,
		httpTracing,
	)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, diagnosisOpts...)
	return opts, originPolicy, nil
}

func diagnosisServerOptionsFromEnv(
	logger *slog.Logger,
	getenv getenvFunc,
	uowFactory ports.UnitOfWorkFactory,
	workflows ports.DiagnosisRoomWorkflowClient,
	starter ports.DiagnosisRoomWorkflowStarter,
	tickets diagnosisauth.Store,
	originPolicy *browserOriginPolicy,
	httpTracing *observabilitytracing.HTTPTracing,
) ([]transporthttp.ServerOption, error) {
	diagnosisConfigured := anyEnv(getenv,
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL",
		"OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID",
		"OPENCLARION_DIAGNOSIS_OIDC_ROLE_CLAIM",
		"OPENCLARION_DIAGNOSIS_OIDC_OWNER_ROLES",
		"OPENCLARION_DIAGNOSIS_OIDC_ADMIN_ROLES",
		"OPENCLARION_DIAGNOSIS_OIDC_SIGNING_ALGS",
	)
	if !diagnosisConfigured {
		logger.Warn("diagnosis WebSocket auth is disabled; set OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL and OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID to enable live diagnosis rooms")
		return nil, nil
	}
	if uowFactory == nil {
		return nil, fmt.Errorf("diagnosis WebSocket auth requires a unit of work factory")
	}
	if workflows == nil {
		return nil, fmt.Errorf("diagnosis WebSocket relay requires a DiagnosisRoomWorkflowClient")
	}
	if starter == nil {
		return nil, fmt.Errorf("diagnosis room creation requires a DiagnosisRoomWorkflowStarter")
	}
	if tickets == nil {
		return nil, fmt.Errorf("diagnosis WebSocket auth requires a ticket store")
	}

	oidcCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	authProvider, err := authoidc.NewProvider(oidcCtx, authoidc.Config{
		IssuerURL:            strings.TrimSpace(getenv("OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL")),
		ClientID:             strings.TrimSpace(getenv("OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID")),
		RoleClaim:            strings.TrimSpace(getenv("OPENCLARION_DIAGNOSIS_OIDC_ROLE_CLAIM")),
		OwnerRoleValues:      optionalCSVValues(getenv("OPENCLARION_DIAGNOSIS_OIDC_OWNER_ROLES")),
		AdminRoleValues:      optionalCSVValues(getenv("OPENCLARION_DIAGNOSIS_OIDC_ADMIN_ROLES")),
		SupportedSigningAlgs: optionalCSVValues(getenv("OPENCLARION_DIAGNOSIS_OIDC_SIGNING_ALGS")),
		HTTPClient:           outboundHTTPClient(httpTracing, 10*time.Second),
	})
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis OIDC auth provider: %w", err)
	}
	ticketService, err := diagnosisauth.NewService(tickets, diagnosisauth.DefaultTicketPolicy(), nil)
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis WebSocket ticket service: %w", err)
	}
	roomStartService, err := diagnosisroomstart.NewService(uowFactory, starter)
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis room starter service: %w", err)
	}
	opts := []transporthttp.ServerOption{
		transporthttp.WithDiagnosisAuth(authProvider, ticketService, diagnosisChatSessionResolver{uowFactory: uowFactory}),
		transporthttp.WithDiagnosisRoomStarter(roomStartService),
		transporthttp.WithDiagnosisRoomWorkflowClient(workflows),
	}
	if originPolicy != nil {
		opts = append(opts, transporthttp.WithDiagnosisWebSocketOriginCheck(originPolicy.CheckWebSocketOrigin))
	}
	logger.Info("configured diagnosis WebSocket auth and relay", "provider", "oidc")
	return opts, nil
}

func outboundHTTPClient(httpTracing *observabilitytracing.HTTPTracing, timeout time.Duration) *http.Client {
	if httpTracing == nil {
		return nil
	}
	return httpTracing.HTTPClient(timeout)
}

func outboundTransportDecorator(httpTracing *observabilitytracing.HTTPTracing) func(http.RoundTripper) http.RoundTripper {
	if httpTracing == nil {
		return nil
	}
	return httpTracing.Transport
}

func temporalClientInterceptors(httpTracing *observabilitytracing.HTTPTracing) ([]interceptor.ClientInterceptor, error) {
	if httpTracing == nil {
		return nil, nil
	}
	tracingInterceptor, err := httpTracing.TemporalInterceptor()
	if err != nil {
		return nil, fmt.Errorf("configure Temporal tracing interceptor: %w", err)
	}
	if tracingInterceptor == nil {
		return nil, nil
	}
	return []interceptor.ClientInterceptor{tracingInterceptor}, nil
}

func anyEnv(getenv getenvFunc, keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(getenv(key)) != "" {
			return true
		}
	}
	return false
}

type diagnosisChatSessionResolver struct {
	uowFactory ports.UnitOfWorkFactory
}

func (r diagnosisChatSessionResolver) ResolveDiagnosisSession(ctx context.Context, sessionID string) (diagnosisauth.SessionRef, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return diagnosisauth.SessionRef{}, fmt.Errorf("diagnosis session resolver: session id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if r.uowFactory == nil {
		return diagnosisauth.SessionRef{}, fmt.Errorf("diagnosis session resolver: unit of work factory is required: %w", domain.ErrInvariantViolation)
	}
	var session domain.ChatSession
	err := r.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if uow == nil {
			return fmt.Errorf("diagnosis session resolver: unit of work is nil: %w", domain.ErrInvariantViolation)
		}
		got, err := uow.Diagnosis().FindChatSessionByKey(ctx, sessionID)
		if err != nil {
			return err
		}
		session = got
		return nil
	})
	if err != nil {
		return diagnosisauth.SessionRef{}, err
	}
	return diagnosisauth.SessionRef{
		SessionID:    session.SessionKey,
		OwnerSubject: session.OwnerSubject,
	}, nil
}

type browserOriginPolicy struct {
	allowed map[string]bool
}

func browserOriginPolicyFromEnv(getenv getenvFunc) (*browserOriginPolicy, error) {
	values := csvValues(getenv("OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS"))
	if len(values) == 0 {
		return nil, nil
	}
	allowed := make(map[string]bool, len(values))
	for _, value := range values {
		origin, err := normalizeBrowserOrigin(value)
		if err != nil {
			return nil, fmt.Errorf("OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS: %w", err)
		}
		allowed[origin] = true
	}
	return &browserOriginPolicy{allowed: allowed}, nil
}

func (p *browserOriginPolicy) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p == nil {
			next.ServeHTTP(w, r)
			return
		}
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		allowed, normalized := p.allowOrigin(origin, r.Host)
		if !allowed {
			if r.Method == http.MethodOptions {
				http.Error(w, "CORS origin is not allowed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Origin", normalized)
		w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type, accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (p *browserOriginPolicy) CheckWebSocketOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	allowed, _ := p.allowOrigin(origin, r.Host)
	return allowed
}

func (p *browserOriginPolicy) allowOrigin(origin, requestHost string) (bool, string) {
	normalized, err := normalizeBrowserOrigin(origin)
	if err != nil {
		return false, ""
	}
	if originHostMatchesRequestHost(normalized, requestHost) {
		return true, normalized
	}
	return p.allowed[normalized], normalized
}

func originHostMatchesRequestHost(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return parsed.Host != "" && strings.EqualFold(parsed.Host, strings.TrimSpace(requestHost))
}

func normalizeBrowserOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("origin must be non-empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse origin %q: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("origin %q must use http or https", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("origin %q must be absolute", raw)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("origin %q must not include userinfo", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("origin %q must not include path, query, or fragment", raw)
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func parseOptionalJSONStringArray(raw, label string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("%s must be a JSON string array: %w", label, err)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s must not contain empty command arguments", label)
		}
	}
	return values, nil
}

func optionalCSVValues(raw string) []string {
	values := csvValues(raw)
	if len(values) == 0 {
		return nil
	}
	return values
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

type reportReplayCLIConfig struct {
	WindowStart    time.Time
	WindowEnd      time.Time
	Limit          int
	CorrelationKey string
	WorkflowID     string
	Scenario       reportprompt.Scenario
	Wait           bool
	WaitTimeout    time.Duration
}

type reportReplayCLITrigger interface {
	ReplayAndStart(ctx context.Context, req reporttrigger.Request) (reporttrigger.Result, error)
}

type reportReplayCLIWorkflowWaiter interface {
	WaitReportBatch(ctx context.Context, handle ports.WorkflowHandle) (reportReplayCLIWorkflowResult, error)
}

type reportReplayCLIOutput struct {
	CheckedAt      string                         `json:"checked_at"`
	Request        reportReplayCLIProofRequest    `json:"request"`
	Started        bool                           `json:"started"`
	WorkflowID     string                         `json:"workflow_id"`
	RunID          string                         `json:"run_id"`
	Waited         bool                           `json:"waited"`
	WorkflowResult *reportReplayCLIWorkflowResult `json:"workflow_result,omitempty"`
	Stats          reportReplayCLIStats           `json:"stats"`
	Snapshots      []reportReplayCLISnapshot      `json:"snapshots"`
}

type reportReplayCLIProofRequest struct {
	WindowStart    string `json:"window_start"`
	WindowEnd      string `json:"window_end"`
	Limit          int    `json:"limit"`
	Scenario       string `json:"scenario"`
	CorrelationKey string `json:"correlation_key,omitempty"`
	WorkflowID     string `json:"workflow_id,omitempty"`
	Wait           bool   `json:"wait"`
	WaitTimeout    string `json:"wait_timeout,omitempty"`
}

type reportReplayCLIStats struct {
	Ingested           reportReplayCLIIngestStats `json:"ingested"`
	EventsLoaded       int                        `json:"events_loaded"`
	GroupsBuilt        int                        `json:"groups_built"`
	GroupsSaved        int                        `json:"groups_saved"`
	GroupsRefreshed    int                        `json:"groups_refreshed"`
	GroupsExisting     int                        `json:"groups_existing"`
	SnapshotsSaved     int                        `json:"snapshots_saved"`
	SnapshotsDuplicate int                        `json:"snapshots_duplicate"`
	GroupsClosed       int                        `json:"groups_closed"`
	Failed             int                        `json:"failed"`
}

type reportReplayCLIIngestStats struct {
	Total     int `json:"total"`
	Saved     int `json:"saved"`
	Duplicate int `json:"duplicate"`
	Failed    int `json:"failed"`
}

type reportReplayCLISnapshot struct {
	ID         int64 `json:"id"`
	GroupIndex int   `json:"group_index"`
	EventCount int   `json:"event_count"`
}

type reportReplayCLIWorkflowResult struct {
	SubReportIDs               []int64 `json:"sub_report_ids"`
	FinalReportID              int64   `json:"final_report_id"`
	NotificationIdempotencyKey string  `json:"notification_idempotency_key"`
	ProviderMessageID          string  `json:"provider_message_id"`
	NotificationStatus         string  `json:"notification_status"`
}

type temporalReportReplayCLIWaiter struct {
	client temporalclient.Client
}

func runReportReplayCLI(
	ctx context.Context,
	logger *slog.Logger,
	getenv getenvFunc,
	args []string,
	stdout io.Writer,
) error {
	cfg, err := parseReportReplayCLIArgs(args)
	if err != nil {
		return err
	}
	dsn := strings.TrimSpace(getenv("DATABASE_URL"))
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	entClient, err := repository.OpenPostgres(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() {
		if cerr := entClient.Close(); cerr != nil {
			logger.Warn("close ent client", "error", cerr)
		}
	}()

	temporalAddr := envOrDefaultFrom(getenv, "TEMPORAL_HOST_PORT", "localhost:7233")
	traceConfig, err := observabilitytracing.ConfigFromEnv(getenv)
	if err != nil {
		return err
	}
	httpTracing, err := observabilitytracing.NewHTTPTracing(ctx, traceConfig)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpTracing.Shutdown(shutdownCtx); err != nil {
			logger.Warn("shutdown OpenTelemetry tracing", "error", err)
		}
	}()
	temporalInterceptors, err := temporalClientInterceptors(httpTracing)
	if err != nil {
		return err
	}
	tc, err := temporalclient.Dial(temporalclient.Options{
		HostPort:     temporalAddr,
		Logger:       temporallog.NewStructuredLogger(logger),
		Interceptors: temporalInterceptors,
	})
	if err != nil {
		return fmt.Errorf("dial temporal: %w", err)
	}
	defer tc.Close()

	starter, err := temporalpkg.NewReportStarter(tc)
	if err != nil {
		return err
	}
	prometheusURL := strings.TrimSpace(getenv("OPENCLARION_PROMETHEUS_URL"))
	if prometheusURL == "" {
		return fmt.Errorf("OPENCLARION_PROMETHEUS_URL is required")
	}
	provider, err := metricsprometheus.NewProvider(
		prometheusURL,
		metricsprometheus.WithBearer(strings.TrimSpace(getenv("OPENCLARION_PROMETHEUS_BEARER_TOKEN"))),
		metricsprometheus.WithRoundTripperDecorator(outboundTransportDecorator(httpTracing)),
	)
	if err != nil {
		return fmt.Errorf("configure report CLI metrics provider: %w", err)
	}
	service, err := reporttrigger.NewService(provider, repository.NewFactory(entClient), starter)
	if err != nil {
		return fmt.Errorf("configure report CLI trigger service: %w", err)
	}
	return runReportReplayCLITrigger(ctx, service, temporalReportReplayCLIWaiter{client: tc}, cfg, stdout)
}

func parseReportReplayCLIArgs(args []string) (reportReplayCLIConfig, error) {
	var rawStart, rawEnd, rawScenario string
	cfg := reportReplayCLIConfig{Limit: defaultReportReplayCLILimit}
	fs := flag.NewFlagSet(reportReplayCLICommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&rawStart, "window-start", "", "inclusive replay window start (RFC3339)")
	fs.StringVar(&rawEnd, "window-end", "", "exclusive replay window end (RFC3339)")
	fs.IntVar(&cfg.Limit, "limit", defaultReportReplayCLILimit, "maximum alert events to load")
	fs.StringVar(&cfg.CorrelationKey, "correlation-key", "", "optional final-report correlation key")
	fs.StringVar(&cfg.WorkflowID, "workflow-id", "", "optional Temporal workflow ID")
	fs.StringVar(&rawScenario, "scenario", string(reportprompt.ScenarioSingleAlert), "report prompt scenario")
	fs.BoolVar(&cfg.Wait, "wait", false, "wait for the report workflow to complete")
	fs.DurationVar(&cfg.WaitTimeout, "wait-timeout", defaultReportReplayCLIWait, "maximum duration to wait when --wait is set")
	if err := fs.Parse(args); err != nil {
		return reportReplayCLIConfig{}, fmt.Errorf("%w\n%s", err, reportReplayCLIUsage())
	}
	if fs.NArg() != 0 {
		return reportReplayCLIConfig{}, fmt.Errorf("unexpected positional arguments: %s\n%s", strings.Join(fs.Args(), " "), reportReplayCLIUsage())
	}
	if strings.TrimSpace(rawStart) == "" {
		return reportReplayCLIConfig{}, fmt.Errorf("--window-start is required\n%s", reportReplayCLIUsage())
	}
	windowStart, err := time.Parse(time.RFC3339, strings.TrimSpace(rawStart))
	if err != nil {
		return reportReplayCLIConfig{}, fmt.Errorf("parse --window-start: %w\n%s", err, reportReplayCLIUsage())
	}
	if strings.TrimSpace(rawEnd) == "" {
		return reportReplayCLIConfig{}, fmt.Errorf("--window-end is required\n%s", reportReplayCLIUsage())
	}
	windowEnd, err := time.Parse(time.RFC3339, strings.TrimSpace(rawEnd))
	if err != nil {
		return reportReplayCLIConfig{}, fmt.Errorf("parse --window-end: %w\n%s", err, reportReplayCLIUsage())
	}
	if cfg.Limit <= 0 {
		return reportReplayCLIConfig{}, fmt.Errorf("--limit must be > 0 (got %d)\n%s", cfg.Limit, reportReplayCLIUsage())
	}
	if cfg.Wait && cfg.WaitTimeout <= 0 {
		return reportReplayCLIConfig{}, fmt.Errorf("--wait-timeout must be > 0 when --wait is set (got %s)\n%s", cfg.WaitTimeout, reportReplayCLIUsage())
	}
	scenario := reportprompt.Scenario(strings.TrimSpace(rawScenario))
	if !scenario.Valid() {
		return reportReplayCLIConfig{}, fmt.Errorf("--scenario %q is unsupported\n%s", rawScenario, reportReplayCLIUsage())
	}
	cfg.WindowStart = windowStart
	cfg.WindowEnd = windowEnd
	cfg.CorrelationKey = strings.TrimSpace(cfg.CorrelationKey)
	cfg.WorkflowID = strings.TrimSpace(cfg.WorkflowID)
	cfg.Scenario = scenario
	return cfg, nil
}

func runReportReplayCLITrigger(
	ctx context.Context,
	trigger reportReplayCLITrigger,
	waiter reportReplayCLIWorkflowWaiter,
	cfg reportReplayCLIConfig,
	stdout io.Writer,
) error {
	if trigger == nil {
		return fmt.Errorf("report replay trigger must be configured")
	}
	result, err := trigger.ReplayAndStart(ctx, reporttrigger.Request{
		Replay: alertreplay.Request{
			WindowStart:       cfg.WindowStart,
			WindowEnd:         cfg.WindowEnd,
			Grouping:          alertgrouping.DefaultConfig(),
			CreatedByWorkflow: reportReplayCLICreatedByWorkflow,
			Limit:             cfg.Limit,
		},
		CorrelationKey: cfg.CorrelationKey,
		WorkflowID:     cfg.WorkflowID,
		Scenario:       cfg.Scenario,
	})
	if err != nil {
		return err
	}
	out := reportReplayCLIOutputFromResult(result, cfg)
	if cfg.Wait && result.Started {
		if waiter == nil {
			return fmt.Errorf("report replay workflow waiter must be configured when --wait is set")
		}
		waitCtx, cancel := context.WithTimeout(ctx, cfg.WaitTimeout)
		defer cancel()
		workflowResult, err := waiter.WaitReportBatch(waitCtx, result.Workflow)
		if err != nil {
			return err
		}
		out.Waited = true
		out.WorkflowResult = &workflowResult
	}
	out.CheckedAt = reportReplayCLINowUTC().Format(time.RFC3339Nano)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("write report replay output: %w", err)
	}
	return nil
}

func reportReplayCLIOutputFromResult(result reporttrigger.Result, cfg reportReplayCLIConfig) reportReplayCLIOutput {
	out := reportReplayCLIOutput{
		Request: reportReplayCLIProofRequest{
			WindowStart:    cfg.WindowStart.UTC().Format(time.RFC3339Nano),
			WindowEnd:      cfg.WindowEnd.UTC().Format(time.RFC3339Nano),
			Limit:          cfg.Limit,
			Scenario:       string(cfg.Scenario),
			CorrelationKey: cfg.CorrelationKey,
			WorkflowID:     cfg.WorkflowID,
			Wait:           cfg.Wait,
			WaitTimeout:    cfg.WaitTimeout.String(),
		},
		Started:    result.Started,
		WorkflowID: result.Workflow.WorkflowID,
		RunID:      result.Workflow.RunID,
		Stats: reportReplayCLIStats{
			Ingested: reportReplayCLIIngestStats{
				Total:     result.Replay.Stats.Ingested.Total,
				Saved:     result.Replay.Stats.Ingested.Saved,
				Duplicate: result.Replay.Stats.Ingested.Duplicate,
				Failed:    result.Replay.Stats.Ingested.Failed,
			},
			EventsLoaded:       result.Replay.Stats.EventsLoaded,
			GroupsBuilt:        result.Replay.Stats.GroupsBuilt,
			GroupsSaved:        result.Replay.Stats.GroupsSaved,
			GroupsRefreshed:    result.Replay.Stats.GroupsRefreshed,
			GroupsExisting:     result.Replay.Stats.GroupsExisting,
			SnapshotsSaved:     result.Replay.Stats.SnapshotsSaved,
			SnapshotsDuplicate: result.Replay.Stats.SnapshotsDuplicate,
			GroupsClosed:       result.Replay.Stats.GroupsClosed,
			Failed:             result.Replay.Stats.Failed,
		},
		Snapshots: make([]reportReplayCLISnapshot, len(result.Replay.Snapshots)),
	}
	for i, ref := range result.Replay.Snapshots {
		out.Snapshots[i] = reportReplayCLISnapshot{
			ID:         int64(ref.ID),
			GroupIndex: ref.GroupIndex,
			EventCount: ref.EventCount,
		}
	}
	return out
}

func (w temporalReportReplayCLIWaiter) WaitReportBatch(ctx context.Context, handle ports.WorkflowHandle) (reportReplayCLIWorkflowResult, error) {
	if w.client == nil {
		return reportReplayCLIWorkflowResult{}, fmt.Errorf("report replay workflow waiter: Temporal client must be non-nil")
	}
	workflowID := strings.TrimSpace(handle.WorkflowID)
	if workflowID == "" {
		return reportReplayCLIWorkflowResult{}, fmt.Errorf("report replay workflow waiter: workflow ID must be non-empty")
	}
	var result temporalpkg.ReportBatchWorkflowResult
	if err := w.client.GetWorkflow(ctx, workflowID, strings.TrimSpace(handle.RunID)).Get(ctx, &result); err != nil {
		return reportReplayCLIWorkflowResult{}, fmt.Errorf("wait report batch workflow: %w", err)
	}
	return reportReplayCLIWorkflowResult{
		SubReportIDs:               append([]int64(nil), result.SubReportIDs...),
		FinalReportID:              result.FinalReportID,
		NotificationIdempotencyKey: result.NotificationIdempotencyKey,
		ProviderMessageID:          result.ProviderMessageID,
		NotificationStatus:         result.NotificationStatus,
	}, nil
}

func envOrDefaultFrom(getenv getenvFunc, key, defaultVal string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

func reportReplayCLIUsage() string {
	return "usage: openclarion " + reportReplayCLICommand +
		" --window-start " + strconv.Quote("2026-05-28T10:00:00Z") +
		" --window-end " + strconv.Quote("2026-05-28T11:00:00Z") +
		" [--limit 10000] [--correlation-key key] [--workflow-id id] [--scenario single_alert|cascade|alert_storm] [--wait] [--wait-timeout 20m]"
}
