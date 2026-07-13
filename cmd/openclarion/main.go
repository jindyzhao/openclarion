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

	enumspb "go.temporal.io/api/enums/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservicepb "go.temporal.io/api/workflowservice/v1"
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
	authldap "github.com/openclarion/openclarion/internal/providers/auth/ldap"
	authoidc "github.com/openclarion/openclarion/internal/providers/auth/oidc"
	authstatic "github.com/openclarion/openclarion/internal/providers/auth/static"
	httpcmdb "github.com/openclarion/openclarion/internal/providers/cmdb/http"
	netboxcmdb "github.com/openclarion/openclarion/internal/providers/cmdb/netbox"
	containerdocker "github.com/openclarion/openclarion/internal/providers/container/docker"
	directoryiam "github.com/openclarion/openclarion/internal/providers/directory/iam"
	imemail "github.com/openclarion/openclarion/internal/providers/im/email"
	imwebhook "github.com/openclarion/openclarion/internal/providers/im/webhook"
	"github.com/openclarion/openclarion/internal/providers/im/wecomcallback"
	openaillm "github.com/openclarion/openclarion/internal/providers/llm/openai"
	metricsalertmanager "github.com/openclarion/openclarion/internal/providers/metrics/alertmanager"
	metricsprometheus "github.com/openclarion/openclarion/internal/providers/metrics/prometheus"
	secretenvmap "github.com/openclarion/openclarion/internal/providers/secrets/envmap"
	"github.com/openclarion/openclarion/internal/strictjson"
	transporthttp "github.com/openclarion/openclarion/internal/transport/http"
	"github.com/openclarion/openclarion/internal/usecases/alertdiagnosis"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertmanagerwebhook"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/alertsourcecheck"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisnotification"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomclose"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/directorysync"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelcheck"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelprovider"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	rbacusecase "github.com/openclarion/openclarion/internal/usecases/rbac"
	"github.com/openclarion/openclarion/internal/usecases/reportnotification"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

type getenvFunc func(string) string

const (
	temporalTaskQueueEnv = "OPENCLARION_TEMPORAL_TASK_QUEUE"
	publicBaseURLEnv     = "OPENCLARION_PUBLIC_BASE_URL"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	alertSourceSecretRefsEnv = "OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	notificationChannelSecretRefsEnv   = "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON"
	autoDiagnosisMaxRoomsPerTriggerEnv = "OPENCLARION_AUTO_DIAGNOSIS_MAX_ROOMS_PER_TRIGGER"
	cmdbHTTPURLEnv                     = "OPENCLARION_CMDB_HTTP_URL"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	cmdbHTTPBearerTokenEnv    = "OPENCLARION_CMDB_HTTP_BEARER_TOKEN"
	cmdbHTTPTimeoutEnv        = "OPENCLARION_CMDB_HTTP_TIMEOUT_SECONDS"
	cmdbNetBoxURLEnv          = "OPENCLARION_CMDB_NETBOX_URL"
	cmdbNetBoxLookupLabelEnv  = "OPENCLARION_CMDB_NETBOX_LOOKUP_LABEL"
	cmdbNetBoxLookupFilterEnv = "OPENCLARION_CMDB_NETBOX_LOOKUP_FILTER"
	cmdbNetBoxObjectTypeEnv   = "OPENCLARION_CMDB_NETBOX_OBJECT_TYPE"
	cmdbNetBoxCustomFieldsEnv = "OPENCLARION_CMDB_NETBOX_ATTRIBUTE_CUSTOM_FIELDS"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	cmdbNetBoxTokenSchemeEnv = "OPENCLARION_CMDB_NETBOX_TOKEN_SCHEME"
	cmdbNetBoxHTTPTimeoutEnv = "OPENCLARION_CMDB_NETBOX_TIMEOUT_SECONDS"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	cmdbNetBoxAPITokenEnv = "OPENCLARION_CMDB_NETBOX_API_TOKEN"
	diagnosisAuthModeEnv  = "OPENCLARION_DIAGNOSIS_AUTH_MODE"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	diagnosisStaticBearerTokenEnv = "OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN"
	diagnosisStaticSubjectEnv     = "OPENCLARION_DIAGNOSIS_STATIC_SUBJECT"
	diagnosisStaticRolesEnv       = "OPENCLARION_DIAGNOSIS_STATIC_ROLES"
	diagnosisLDAPURLEnv           = "OPENCLARION_DIAGNOSIS_LDAP_URL"
	diagnosisLDAPBaseDNEnv        = "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN"
	diagnosisLDAPBindDNEnv        = "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	diagnosisLDAPBindPasswordEnv     = "OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD"
	diagnosisLDAPUserFilterEnv       = "OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER"
	diagnosisLDAPSubjectAttributeEnv = "OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE"
	diagnosisLDAPRoleAttributeEnv    = "OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE"
	diagnosisLDAPOwnerRoleValuesEnv  = "OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES"
	diagnosisLDAPAdminRoleValuesEnv  = "OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES"
	diagnosisLDAPDefaultRolesEnv     = "OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES"
	diagnosisLDAPStartTLSEnv         = "OPENCLARION_DIAGNOSIS_LDAP_START_TLS"
	diagnosisLDAPAllowPlaintextEnv   = "OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT"
	diagnosisOIDCIssuerURLEnv        = "OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL"
	diagnosisOIDCClientIDEnv         = "OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID"
	diagnosisOIDCRoleClaimEnv        = "OPENCLARION_DIAGNOSIS_OIDC_ROLE_CLAIM"
	diagnosisOIDCOwnerRolesEnv       = "OPENCLARION_DIAGNOSIS_OIDC_OWNER_ROLES"
	diagnosisOIDCAdminRolesEnv       = "OPENCLARION_DIAGNOSIS_OIDC_ADMIN_ROLES"
	diagnosisOIDCSigningAlgsEnv      = "OPENCLARION_DIAGNOSIS_OIDC_SIGNING_ALGS"
	iamOIDCIssuerEnv                 = "OPENCLARION_IAM_OIDC_ISSUER"
	iamOIDCClientIDEnv               = "OPENCLARION_IAM_OIDC_CLIENT_ID"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	iamOIDCClientSecretEnv  = "OPENCLARION_IAM_OIDC_CLIENT_SECRET"
	standardOIDCIssuerEnv   = "OIDC_ISSUER"
	standardOIDCClientIDEnv = "OIDC_CLIENT_ID"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	standardOIDCClientSecretEnv = "OIDC_CLIENT_SECRET"
	iamDirectoryProviderNameEnv = "OPENCLARION_IAM_DIRECTORY_PROVIDER_NAME"
	iamDirectoryIssuerEnv       = "OPENCLARION_IAM_DIRECTORY_ISSUER"
	iamDirectoryBaseURLEnv      = "OPENCLARION_IAM_DIRECTORY_BASE_URL"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	iamDirectoryTokenURLEnv = "OPENCLARION_IAM_DIRECTORY_TOKEN_URL"
	iamDirectoryClientIDEnv = "OPENCLARION_IAM_DIRECTORY_CLIENT_ID"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	iamDirectoryClientSecretEnv   = "OPENCLARION_IAM_DIRECTORY_CLIENT_SECRET"
	iamDirectoryScopesEnv         = "OPENCLARION_IAM_DIRECTORY_SCOPES"
	standardDirectoryScopesEnv    = "DIRECTORY_SCOPES"
	iamDirectorySyncIntervalEnv   = "OPENCLARION_IAM_DIRECTORY_SYNC_INTERVAL_SECONDS"
	rbacBootstrapAdminSubjectsEnv = "OPENCLARION_RBAC_BOOTSTRAP_ADMIN_SUBJECTS"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	diagnosisSessionSigningKeyEnv = "OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY"
	diagnosisWeComCorpIDEnv       = "OPENCLARION_WECOM_CORP_ID"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	diagnosisWeComCallbackTokenEnv = "OPENCLARION_WECOM_CALLBACK_TOKEN"
	// #nosec G101 -- environment variable name only; values are read at runtime.
	diagnosisWeComCallbackEncodingAESKeyEnv = "OPENCLARION_WECOM_CALLBACK_ENCODING_AES_KEY"
	diagnosisWeComCallbackReceiveIDEnv      = "OPENCLARION_WECOM_CALLBACK_RECEIVE_ID"
	reportLLMHTTPTimeoutSecondsEnv          = "OPENCLARION_M4_REPORT_LLM_HTTP_TIMEOUT_SECONDS"
	sandboxEgressProxyURLEnv                = "OPENCLARION_SANDBOX_EGRESS_PROXY_URL"
	reportReplayCLICommand                  = "report-replay"
	reportPolicyReplayCLICommand            = "report-policy-replay"
	reportScheduleLiveSmokeCLICommand       = "report-schedule-live-smoke"
	workflowBacklogCLICommand               = "workflow-backlog"
	diagnosisRoomListCLICommand             = "diagnosis-room-list"
	reportReplayCLICreatedByWorkflow        = "ReportReplayCLI"
	defaultReportReplayCLILimit             = 10000
	defaultWorkflowBacklogCLILimit          = 50
	maxWorkflowBacklogCLILimit              = 500
	defaultDiagnosisRoomListCLILimit        = 100
	maxDiagnosisRoomListCLILimit            = 500
	defaultReportReplayCLIWait              = 20 * time.Minute
	defaultReportLLMHTTPTimeout             = 260 * time.Second
	defaultCMDBHTTPTimeout                  = 10 * time.Second
	defaultReportScheduleLiveSmokeWait      = 30 * time.Minute
	defaultReportScheduleLiveSmokePoll      = 5 * time.Second
	minIAMDirectorySyncInterval             = time.Minute

	diagnosisRoomCloseCLICommand    = "diagnosis-room-close"
	defaultDiagnosisRoomCloseReason = "live_smoke_completed"
	defaultDiagnosisRoomCloseWait   = 2 * time.Minute

	diagnosisRoomCloseEventClosedKind           = "diagnosis_room.closed"
	diagnosisRoomCloseEventNotificationSentKind = "diagnosis_room.close_notification_sent"
)

var reportReplayCLINowUTC = func() time.Time {
	return time.Now().UTC()
}

var reportScheduleLiveSmokeCLINowUTC = func() time.Time {
	return time.Now().UTC()
}

var workflowBacklogCLINowUTC = func() time.Time {
	return time.Now().UTC()
}

var diagnosisAuthNowUTC = func() time.Time {
	return time.Now().UTC()
}

var diagnosisNotificationRetryNowUTC = func() time.Time {
	return time.Now().UTC()
}

var defaultWorkflowBacklogCLIWorkflowTypes = []string{
	"ReportBatchWorkflow",
	"ReportFanOutWorkflow",
	"FinalReportWorkflow",
	"ReportPolicyScheduleLauncherWorkflow",
	"DiagnosisRoomWorkflow",
}

var diagnosisRoomCloseCLINowUTC = func() time.Time {
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
	case reportPolicyReplayCLICommand:
		return runReportPolicyReplayCLI(ctx, logger, os.Getenv, args[1:], stdout)
	case reportScheduleLiveSmokeCLICommand:
		return runReportScheduleLiveSmokeCLI(ctx, logger, os.Getenv, args[1:], stdout)
	case workflowBacklogCLICommand:
		return runWorkflowBacklogCLI(ctx, logger, os.Getenv, args[1:], stdout)
	case diagnosisRoomListCLICommand:
		return runDiagnosisRoomListCLI(ctx, logger, os.Getenv, args[1:], stdout)
	case diagnosisRoomCloseCLICommand:
		return runDiagnosisRoomCloseCLI(ctx, logger, os.Getenv, args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q (expected: serve, %s, %s, %s, %s, %s, or %s)", args[0], reportReplayCLICommand, reportPolicyReplayCLICommand, reportScheduleLiveSmokeCLICommand, workflowBacklogCLICommand, diagnosisRoomListCLICommand, diagnosisRoomCloseCLICommand)
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
	temporalTaskQueue, err := temporalTaskQueueFromEnv(os.Getenv)
	if err != nil {
		return err
	}
	logger.Info("configured Temporal task queue", "task_queue", temporalTaskQueue)

	reportStarter, err := temporalpkg.NewReportStarter(tc, temporalpkg.WithReportStarterTaskQueue(temporalTaskQueue))
	if err != nil {
		return err
	}
	scheduleRegistrar, err := temporalpkg.NewReportWorkflowScheduleRegistrar(
		tc,
		temporalpkg.WithReportWorkflowScheduleRegistrarTaskQueue(temporalTaskQueue),
	)
	if err != nil {
		return err
	}
	diagnosisRoomStarter, err := temporalpkg.NewDiagnosisRoomStarter(tc, temporalpkg.WithDiagnosisRoomStarterTaskQueue(temporalTaskQueue))
	if err != nil {
		return err
	}

	activityOptions, err := reportActivityOptionsFromEnv(ctx, logger, os.Getenv, uowFactory, reportStarter, diagnosisRoomStarter, httpTracing)
	if err != nil {
		return err
	}
	diagnosisActivityOptions, err := diagnosisActivityOptionsFromEnv(logger, os.Getenv, httpTracing)
	if err != nil {
		return err
	}
	activityOptions = append(activityOptions, diagnosisActivityOptions...)
	w, err := temporalpkg.NewWorkerWithTaskQueue(tc, uowFactory, temporalTaskQueue, activityOptions...)
	if err != nil {
		return err
	}
	if err := w.Start(); err != nil {
		return fmt.Errorf("start temporal worker: %w", err)
	}
	defer w.Stop()
	reconcileResult, err := reconcileReportWorkflowSchedules(ctx, uowFactory, scheduleRegistrar)
	if err != nil {
		return fmt.Errorf("reconcile report workflow schedules: %w", err)
	}
	if reconcileResult.Total > 0 {
		logger.Info(
			"reconciled report workflow schedules",
			"total", reconcileResult.Total,
			"created", reconcileResult.Created,
			"updated", reconcileResult.Updated,
		)
	}

	diagnosisRoomClient, err := temporalpkg.NewDiagnosisRoomClient(tc)
	if err != nil {
		return err
	}
	ticketStore, err := repository.NewDiagnosisAuthTicketStore(client)
	if err != nil {
		return fmt.Errorf("configure diagnosis WebSocket ticket store: %w", err)
	}
	serverOptions, originPolicy, err := httpServerOptionsFromEnv(logger, os.Getenv, uowFactory, reportStarter, diagnosisRoomClient, diagnosisRoomStarter, ticketStore, scheduleRegistrar, httpTracing)
	if err != nil {
		return err
	}
	periodicDirectorySyncer, periodicDirectorySyncInterval, periodicDirectorySyncConfigured, err := periodicDirectorySyncerFromEnv(os.Getenv, uowFactory, httpTracing)
	if err != nil {
		return err
	}
	if periodicDirectorySyncConfigured {
		logger.Info("configured periodic IAM directory sync", "interval", periodicDirectorySyncInterval.String(), "provider", directoryProviderNameFromEnv(os.Getenv))
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
	if periodicDirectorySyncConfigured {
		group.Go(func() error {
			return runPeriodicDirectorySync(groupCtx, logger, periodicDirectorySyncer, periodicDirectorySyncInterval)
		})
	}
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

func temporalTaskQueueFromEnv(getenv getenvFunc) (string, error) {
	if getenv == nil {
		return temporalpkg.TaskQueue, nil
	}
	raw := getenv(temporalTaskQueueEnv)
	taskQueue := strings.TrimSpace(raw)
	if taskQueue == "" {
		return temporalpkg.TaskQueue, nil
	}
	if taskQueue != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", temporalTaskQueueEnv)
	}
	if strings.ContainsAny(taskQueue, "\r\n\t") {
		return "", fmt.Errorf("%s must not contain control whitespace", temporalTaskQueueEnv)
	}
	return taskQueue, nil
}

func reportActivityOptionsFromEnv(
	ctx context.Context,
	logger *slog.Logger,
	getenv getenvFunc,
	uowFactory ports.UnitOfWorkFactory,
	starter ports.ReportWorkflowStarter,
	diagnosisStarter ports.DiagnosisRoomWorkflowStarter,
	httpTracing *observabilitytracing.HTTPTracing,
) ([]temporalpkg.ActivityOption, error) {
	var opts []temporalpkg.ActivityOption
	cmdbProvider, cmdbProviderKind, err := cmdbProviderFromEnv(getenv, httpTracing)
	if err != nil {
		return nil, err
	}

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
		timeout, err := positiveDurationSecondsFromEnv(getenv, reportLLMHTTPTimeoutSecondsEnv, defaultReportLLMHTTPTimeout)
		if err != nil {
			return nil, err
		}
		provider, err := openaillm.NewProviderWithCapabilityDetection(ctx, openaillm.Config{
			BaseURL:    strings.TrimSpace(getenv("OPENCLARION_LLM_BASE_URL")),
			APIKey:     strings.TrimSpace(getenv("OPENCLARION_LLM_API_KEY")),
			Model:      model,
			HTTPClient: outboundHTTPClient(httpTracing, timeout),
		})
		if err != nil {
			return nil, fmt.Errorf("configure report LLM provider: %w", err)
		}
		opts = append(opts, temporalpkg.WithLLMProvider(provider))
		logger.Info("configured report LLM provider", "provider", "openai-compatible", "output_mode", provider.OutputMode())
	}

	imProvider, imFormat, imConfigured, err := reportIMProviderFromEnv(getenv, httpTracing)
	if imConfigured {
		if err != nil {
			return nil, err
		}
		opts = append(opts, temporalpkg.WithIMProvider(imProvider))
		logger.Info("configured report IM provider", "provider", "webhook", "format", imFormat)
	}

	notificationChannelSecretResolver, err := notificationChannelSecretResolverFromEnv(getenv)
	if err != nil {
		return nil, err
	}
	if notificationChannelSecretResolver != nil {
		if uowFactory == nil {
			return nil, fmt.Errorf("%s requires a unit of work factory", notificationChannelSecretRefsEnv)
		}
		builder, err := notificationchannelprovider.NewBuilder(
			notificationChannelWebhookFactory(httpTracing),
			notificationchannelprovider.WithSecretResolver(notificationChannelSecretResolver),
			notificationchannelprovider.WithEmailFactory(notificationChannelEmailFactory()),
		)
		if err != nil {
			return nil, fmt.Errorf("configure notification channel provider builder: %w", err)
		}
		resolver, err := notificationchannelprovider.NewResolver(uowFactory, builder)
		if err != nil {
			return nil, fmt.Errorf("configure notification channel provider resolver: %w", err)
		}
		opts = append(opts, temporalpkg.WithNotificationChannelProviderResolver(resolver))
		logger.Info("configured notification channel provider resolver", "provider", "webhook,email")
	}

	if !llmConfigured || (!imConfigured && notificationChannelSecretResolver == nil) {
		logger.Warn("report provider wiring is incomplete; report workflows require OPENCLARION_LLM_* and either OPENCLARION_IM_WEBHOOK_* for unbound delivery or OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON for profile-bound delivery before production use")
	}
	if notificationChannelSecretResolver != nil && !imConfigured {
		logger.Info("legacy report IM provider is not configured; unbound report notification delivery requires OPENCLARION_IM_WEBHOOK_*")
	}

	if uowFactory != nil && starter != nil {
		secretResolver, err := alertSourceSecretResolverFromEnv(getenv)
		if err != nil {
			return nil, err
		}
		var providerBuilderOptions []alertsourceprovider.Option
		if secretResolver != nil {
			providerBuilderOptions = append(providerBuilderOptions, alertsourceprovider.WithSecretResolver(secretResolver))
		}
		alertSourceProviders, err := alertsourceprovider.NewBuilder(alertSourceProviderFactories(httpTracing), providerBuilderOptions...)
		if err != nil {
			return nil, fmt.Errorf("configure scheduled report policy provider builder: %w", err)
		}
		policyReplayerOptions := []reportpolicytrigger.Option{}
		if cmdbProvider != nil {
			policyReplayerOptions = append(policyReplayerOptions, reportpolicytrigger.WithCMDBProvider(cmdbProvider))
		}
		if diagnosisStarter != nil {
			autoDiagnosisOptions, triggerErr := autoDiagnosisOptionsFromEnv(getenv)
			if triggerErr != nil {
				return nil, triggerErr
			}
			if cmdbProvider != nil {
				autoDiagnosisOptions = append(autoDiagnosisOptions, alertdiagnosis.WithCMDBProvider(cmdbProvider))
			}
			autoDiagnosisTrigger, triggerErr := alertdiagnosis.NewService(uowFactory, diagnosisStarter, autoDiagnosisOptions...)
			if triggerErr != nil {
				return nil, fmt.Errorf("configure scheduled report policy auto diagnosis trigger: %w", triggerErr)
			}
			policyReplayerOptions = append(policyReplayerOptions, reportpolicytrigger.WithAutoDiagnosisTrigger(autoDiagnosisTrigger))
		}
		policyReplayer, err := reportpolicytrigger.NewService(uowFactory, starter, alertSourceProviders, policyReplayerOptions...)
		if err != nil {
			return nil, fmt.Errorf("configure scheduled report policy replayer: %w", err)
		}
		opts = append(opts, temporalpkg.WithReportPolicyReplayer(policyReplayer))
		logger.Info("configured scheduled report policy replayer", "providers", "profile")
		if cmdbProviderKind != "" {
			logger.Info("configured scheduled replay CMDB enrichment", "provider", cmdbProviderKind)
		}
	}
	return opts, nil
}

func cmdbProviderFromEnv(
	getenv getenvFunc,
	httpTracing *observabilitytracing.HTTPTracing,
) (ports.CMDBProvider, string, error) {
	if getenv == nil {
		return nil, "", nil
	}
	httpConfigured := anyEnv(getenv, cmdbHTTPURLEnv, cmdbHTTPBearerTokenEnv, cmdbHTTPTimeoutEnv)
	netBoxConfigured := anyEnv(getenv,
		cmdbNetBoxURLEnv,
		cmdbNetBoxAPITokenEnv,
		cmdbNetBoxTokenSchemeEnv,
		cmdbNetBoxLookupLabelEnv,
		cmdbNetBoxLookupFilterEnv,
		cmdbNetBoxObjectTypeEnv,
		cmdbNetBoxCustomFieldsEnv,
		cmdbNetBoxHTTPTimeoutEnv,
	)
	if httpConfigured && netBoxConfigured {
		return nil, "", fmt.Errorf("HTTP and NetBox CMDB provider configuration are mutually exclusive")
	}
	if !httpConfigured && !netBoxConfigured {
		return nil, "", nil
	}
	if netBoxConfigured {
		return netBoxCMDBProviderFromEnv(getenv, httpTracing)
	}

	endpoint := strings.TrimSpace(getenv(cmdbHTTPURLEnv))
	if endpoint == "" {
		return nil, "", fmt.Errorf("%s is required when configuring the HTTP CMDB provider", cmdbHTTPURLEnv)
	}
	timeout, err := positiveDurationSecondsFromEnv(getenv, cmdbHTTPTimeoutEnv, defaultCMDBHTTPTimeout)
	if err != nil {
		return nil, "", err
	}
	provider, err := httpcmdb.NewProvider(httpcmdb.Config{
		URL:         endpoint,
		BearerToken: strings.TrimSpace(getenv(cmdbHTTPBearerTokenEnv)),
		HTTPClient:  outboundHTTPClient(httpTracing, timeout),
	})
	if err != nil {
		return nil, "", fmt.Errorf("configure HTTP CMDB provider: %w", err)
	}
	return provider, "http", nil
}

func netBoxCMDBProviderFromEnv(
	getenv getenvFunc,
	httpTracing *observabilitytracing.HTTPTracing,
) (ports.CMDBProvider, string, error) {
	baseURL := strings.TrimSpace(getenv(cmdbNetBoxURLEnv))
	if baseURL == "" {
		return nil, "", fmt.Errorf("%s is required when configuring the NetBox CMDB provider", cmdbNetBoxURLEnv)
	}
	lookupLabel := getenv(cmdbNetBoxLookupLabelEnv)
	if strings.TrimSpace(lookupLabel) == "" {
		return nil, "", fmt.Errorf("%s is required when configuring the NetBox CMDB provider", cmdbNetBoxLookupLabelEnv)
	}
	timeout, err := positiveDurationSecondsFromEnv(getenv, cmdbNetBoxHTTPTimeoutEnv, defaultCMDBHTTPTimeout)
	if err != nil {
		return nil, "", err
	}
	provider, err := netboxcmdb.NewProvider(netboxcmdb.Config{
		BaseURL:               baseURL,
		APIToken:              getenv(cmdbNetBoxAPITokenEnv),
		TokenScheme:           netboxcmdb.TokenScheme(getenv(cmdbNetBoxTokenSchemeEnv)),
		LookupLabel:           lookupLabel,
		LookupFilter:          getenv(cmdbNetBoxLookupFilterEnv),
		ObjectType:            netboxcmdb.ObjectType(getenv(cmdbNetBoxObjectTypeEnv)),
		AttributeCustomFields: optionalCSVValues(getenv(cmdbNetBoxCustomFieldsEnv)),
		HTTPClient:            outboundHTTPClient(httpTracing, timeout),
	})
	if err != nil {
		return nil, "", fmt.Errorf("configure NetBox CMDB provider: %w", err)
	}
	return provider, "netbox", nil
}

func reportIMProviderFromEnv(
	getenv getenvFunc,
	httpTracing *observabilitytracing.HTTPTracing,
) (ports.IMProvider, string, bool, error) {
	imConfigured := anyEnv(getenv,
		"OPENCLARION_IM_WEBHOOK_URL",
		"OPENCLARION_IM_WEBHOOK_BEARER_TOKEN",
		"OPENCLARION_IM_WEBHOOK_FORMAT",
	)
	if !imConfigured {
		return nil, "", false, nil
	}
	url := strings.TrimSpace(getenv("OPENCLARION_IM_WEBHOOK_URL"))
	if url == "" {
		return nil, "", true, fmt.Errorf("OPENCLARION_IM_WEBHOOK_URL is required when configuring the report IM provider")
	}
	format := strings.TrimSpace(getenv("OPENCLARION_IM_WEBHOOK_FORMAT"))
	provider, err := imwebhook.NewProvider(imwebhook.Config{
		URL:         url,
		BearerToken: strings.TrimSpace(getenv("OPENCLARION_IM_WEBHOOK_BEARER_TOKEN")),
		Format:      format,
		HTTPClient:  outboundHTTPClient(httpTracing, 10*time.Second),
	})
	if err != nil {
		return nil, "", true, fmt.Errorf("configure report IM provider: %w", err)
	}
	if format == "" {
		format = "generic"
	}
	return provider, strings.ToLower(format), true, nil
}

func notificationChannelWebhookFactory(
	httpTracing *observabilitytracing.HTTPTracing,
) notificationchannelprovider.WebhookFactory {
	return func(
		_ domain.NotificationChannelProfile,
		credentials notificationchannelprovider.WebhookCredentials,
	) (ports.IMProvider, error) {
		return imwebhook.NewProvider(imwebhook.Config{
			URL:        credentials.URL,
			Format:     credentials.Format,
			HTTPClient: outboundHTTPClient(httpTracing, 10*time.Second),
		})
	}
}

func notificationChannelEmailFactory() notificationchannelprovider.EmailFactory {
	return func(
		_ domain.NotificationChannelProfile,
		credentials notificationchannelprovider.EmailCredentials,
	) (ports.IMProvider, error) {
		return imemail.NewProviderFromURL(credentials.URL)
	}
}

func diagnosisActivityOptionsFromEnv(
	logger *slog.Logger,
	getenv getenvFunc,
	httpTracing *observabilitytracing.HTTPTracing,
) ([]temporalpkg.ActivityOption, error) {
	var opts []temporalpkg.ActivityOption
	publicBaseURL, err := publicBaseURLFromEnv(getenv)
	if err != nil {
		return nil, err
	}
	if publicBaseURL != nil {
		opts = append(opts, temporalpkg.WithPublicBaseURL(publicBaseURL))
		logger.Info("configured public base URL for operator links", "host", publicBaseURL.Host)
	}
	secretResolver, err := alertSourceSecretResolverFromEnv(getenv)
	if err != nil {
		return nil, err
	}
	var providerBuilderOptions []alertsourceprovider.Option
	if secretResolver != nil {
		providerBuilderOptions = append(providerBuilderOptions, alertsourceprovider.WithSecretResolver(secretResolver))
	}
	alertSourceProviders, err := alertsourceprovider.NewBuilder(alertSourceProviderFactories(httpTracing), providerBuilderOptions...)
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis evidence provider builder: %w", err)
	}
	opts = append(opts, temporalpkg.WithAlertSourceProviderBuilder(alertSourceProviders))

	sandboxConfigured := anyEnv(getenv,
		"OPENCLARION_SANDBOX_IMAGE_REF",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT",
		"OPENCLARION_SANDBOX_COMMAND_JSON",
		"OPENCLARION_SANDBOX_WORKSPACE_ROOT",
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED",
		"OPENCLARION_SANDBOX_EGRESS_NETWORK",
		sandboxEgressProxyURLEnv,
	)
	if !sandboxConfigured {
		logger.Warn("diagnosis sandbox provider is not configured; diagnosis-room turns require OPENCLARION_SANDBOX_IMAGE_REF and OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT before live use")
		return opts, nil
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
	containerCredentials, err := diagnosisContainerCredentialsFromEnv(getenv)
	if err != nil {
		return nil, err
	}

	var providerOpts []containerdocker.ProviderOption
	if workspaceRoot := strings.TrimSpace(getenv("OPENCLARION_SANDBOX_WORKSPACE_ROOT")); workspaceRoot != "" {
		providerOpts = append(providerOpts, containerdocker.WithWorkspaceRoot(workspaceRoot))
	}
	allowedEgress := csvValues(getenv("OPENCLARION_SANDBOX_EGRESS_ALLOWED"))
	if len(allowedEgress) == 0 {
		return nil, fmt.Errorf("OPENCLARION_SANDBOX_EGRESS_ALLOWED is required when configuring the diagnosis sandbox provider")
	}
	egressProxyURL := strings.TrimSpace(getenv(sandboxEgressProxyURLEnv))
	if egressProxyURL == "" {
		return nil, fmt.Errorf("%s is required when configuring the diagnosis sandbox provider", sandboxEgressProxyURLEnv)
	}
	if err := validateSandboxEgressCoversURL(
		getenv("OPENCLARION_DIAGNOSIS_LLM_BASE_URL"),
		allowedEgress,
	); err != nil {
		return nil, err
	}
	networkMode := strings.TrimSpace(getenv("OPENCLARION_SANDBOX_EGRESS_NETWORK"))
	if networkMode == "" {
		networkMode = containerdocker.DefaultAllowlistNetworkMode
	}
	enforcer, err := containerdocker.NewStaticAllowlistEnforcer(networkMode, allowedEgress)
	if err != nil {
		return nil, fmt.Errorf("configure sandbox egress enforcer: %w", err)
	}
	providerOpts = append(providerOpts, containerdocker.WithEgressEnforcer(enforcer))
	networkPolicy := ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: allowedEgress,
	}

	provider, err := containerdocker.NewProviderFromEnv(containerdocker.Config{
		ImageRef:             imageRef,
		ReadonlyRootFS:       true,
		NoNewPrivileges:      true,
		Command:              command,
		AllowlistNetworkMode: networkMode,
		EgressProxyURL:       egressProxyURL,
	}, agentConfigRoot, providerOpts...)
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis sandbox provider: %w", err)
	}
	logger.Info("configured diagnosis sandbox provider", "provider", "docker")
	opts = append(opts,
		temporalpkg.WithContainerProvider(provider),
		temporalpkg.WithContainerCredentials(containerCredentials),
		temporalpkg.WithContainerNetworkPolicy(networkPolicy),
	)
	return opts, nil
}

func validateSandboxEgressCoversURL(rawURL string, allowedTargets []string) error {
	trimmedURL := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmedURL)
	if rawURL != trimmedURL || err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("OPENCLARION_DIAGNOSIS_LLM_BASE_URL must be an absolute http or https URL without userinfo or surrounding whitespace")
	}
	allowed, err := ports.NormalizeContainerEgressTargets(allowedTargets)
	if err != nil {
		return fmt.Errorf("OPENCLARION_SANDBOX_EGRESS_ALLOWED: %w", err)
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	defaultPort := "80"
	if parsed.Scheme == "https" {
		defaultPort = "443"
	}
	if port == "" {
		port = defaultPort
	}
	exact := host + ":" + port
	for _, target := range allowed {
		if target == exact || (target == host && port == defaultPort) {
			return nil
		}
	}
	return fmt.Errorf("OPENCLARION_DIAGNOSIS_LLM_BASE_URL host must be listed in OPENCLARION_SANDBOX_EGRESS_ALLOWED")
}

func publicBaseURLFromEnv(getenv getenvFunc) (*url.URL, error) {
	if getenv == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(getenv(publicBaseURLEnv))
	if raw == "" {
		return nil, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid URL", publicBaseURLEnv)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%s scheme must be http or https", publicBaseURLEnv)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("%s host must be non-empty", publicBaseURLEnv)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%s must not include userinfo", publicBaseURLEnv)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%s must not include query or fragment", publicBaseURLEnv)
	}
	return parsed, nil
}

func diagnosisContainerCredentialsFromEnv(getenv getenvFunc) ([]temporalpkg.ContainerCredentialTemplate, error) {
	requiredKeys := []string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL",
	}
	credentials := make([]temporalpkg.ContainerCredentialTemplate, 0, len(requiredKeys)+2)
	for _, key := range requiredKeys {
		value := strings.TrimSpace(getenv(key))
		if value == "" {
			return nil, fmt.Errorf("%s is required when configuring the diagnosis sandbox provider", key)
		}
		credentials = append(credentials, temporalpkg.ContainerCredentialTemplate{
			Name:  key,
			Value: value,
		})
	}
	if rawTimeout := strings.TrimSpace(getenv("OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS")); rawTimeout != "" {
		if _, err := positiveDurationSecondsFromEnv(getenv, "OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS", 0); err != nil {
			return nil, err
		}
		credentials = append(credentials, temporalpkg.ContainerCredentialTemplate{
			Name:  "OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS",
			Value: rawTimeout,
		})
	}
	if outputMode := strings.TrimSpace(getenv("OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE")); outputMode != "" {
		switch ports.LLMOutputMode(outputMode) {
		case ports.LLMOutputModeJSONSchema, ports.LLMOutputModeJSONObject:
			credentials = append(credentials, temporalpkg.ContainerCredentialTemplate{
				Name:  "OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE",
				Value: outputMode,
			})
		default:
			return nil, fmt.Errorf("OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE must be %q or %q", ports.LLMOutputModeJSONSchema, ports.LLMOutputModeJSONObject)
		}
	}
	return credentials, nil
}

func httpServerOptionsFromEnv(
	logger *slog.Logger,
	getenv getenvFunc,
	uowFactory ports.UnitOfWorkFactory,
	starter ports.ReportWorkflowStarter,
	diagnosisWorkflows ports.DiagnosisRoomWorkflowClient,
	diagnosisStarter ports.DiagnosisRoomWorkflowStarter,
	diagnosisTickets diagnosisauth.Store,
	scheduleSyncer transporthttp.ReportWorkflowScheduleSynchronizer,
	httpTracing *observabilitytracing.HTTPTracing,
) ([]transporthttp.ServerOption, *browserOriginPolicy, error) {
	var opts []transporthttp.ServerOption
	originPolicy, err := browserOriginPolicyFromEnv(getenv)
	if err != nil {
		return nil, nil, err
	}
	cmdbProvider, cmdbProviderKind, err := cmdbProviderFromEnv(getenv, httpTracing)
	if err != nil {
		return nil, nil, err
	}

	rbacAuthorizer, err := rbacusecase.NewService(uowFactory)
	if err != nil {
		return nil, nil, fmt.Errorf("configure local RBAC authorizer: %w", err)
	}
	opts = append(opts, transporthttp.WithRBACAuthorizer(rbacAuthorizer))
	logger.Info("configured local RBAC authorizer")
	if bootstrapSubjects := optionalCSVValues(getenv(rbacBootstrapAdminSubjectsEnv)); len(bootstrapSubjects) > 0 {
		opts = append(opts, transporthttp.WithLocalRBACBootstrapAdminSubjects(bootstrapSubjects))
		logger.Warn("configured local RBAC bootstrap admin subjects", "count", len(bootstrapSubjects))
	}

	directorySyncer, directoryConfigured, err := directorySyncerFromEnv(getenv, uowFactory, httpTracing)
	if err != nil {
		return nil, nil, err
	}
	if directoryConfigured {
		providerName := directoryProviderNameFromEnv(getenv)
		opts = append(opts, transporthttp.WithDirectorySyncer(directorySyncer, providerName))
		logger.Info("configured IAM directory syncer", "provider", providerName)
	}

	secretResolver, err := alertSourceSecretResolverFromEnv(getenv)
	if err != nil {
		return nil, nil, err
	}
	var providerBuilderOptions []alertsourceprovider.Option
	if secretResolver != nil {
		providerBuilderOptions = append(providerBuilderOptions, alertsourceprovider.WithSecretResolver(secretResolver))
	}
	alertSourceProviders, err := alertsourceprovider.NewBuilder(alertSourceProviderFactories(httpTracing), providerBuilderOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("configure alert source provider builder: %w", err)
	}
	alertSourceTester, err := alertsourcecheck.NewService(alertSourceProviders,
		alertsourcecheck.WithClock(func() time.Time { return time.Now().UTC() }),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("configure alert source connection tester: %w", err)
	}
	opts = append(opts, transporthttp.WithAlertSourceConnectionTester(alertSourceTester))

	webhookOptions := []alertmanagerwebhook.Option{}
	if secretResolver != nil {
		webhookOptions = append(webhookOptions, alertmanagerwebhook.WithSecretResolver(secretResolver))
	}
	var autoDiagnosisTrigger *alertdiagnosis.Service
	if diagnosisStarter != nil {
		var triggerErr error
		autoDiagnosisOptions, optionsErr := autoDiagnosisOptionsFromEnv(getenv)
		if optionsErr != nil {
			return nil, nil, optionsErr
		}
		if cmdbProvider != nil {
			autoDiagnosisOptions = append(autoDiagnosisOptions, alertdiagnosis.WithCMDBProvider(cmdbProvider))
		}
		autoDiagnosisTrigger, triggerErr = alertdiagnosis.NewService(uowFactory, diagnosisStarter, autoDiagnosisOptions...)
		if triggerErr != nil {
			return nil, nil, fmt.Errorf("configure alertmanager auto diagnosis trigger: %w", triggerErr)
		}
		webhookOptions = append(webhookOptions, alertmanagerwebhook.WithAutoDiagnosisTrigger(autoDiagnosisTrigger))
	}
	webhookIngestor, err := alertmanagerwebhook.NewService(uowFactory, webhookOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("configure alertmanager webhook ingestor: %w", err)
	}
	opts = append(opts, transporthttp.WithAlertmanagerWebhookIngestor(webhookIngestor))
	logger.Info("configured alertmanager webhook ingestor", "provider", "profile")

	notificationChannelSecretResolver, err := notificationChannelSecretResolverFromEnv(getenv)
	if err != nil {
		return nil, nil, err
	}
	notificationBuilderOptions := []notificationchannelprovider.Option{
		notificationchannelprovider.WithEmailFactory(notificationChannelEmailFactory()),
	}
	if notificationChannelSecretResolver != nil {
		notificationBuilderOptions = append(
			notificationBuilderOptions,
			notificationchannelprovider.WithSecretResolver(notificationChannelSecretResolver),
		)
	}
	notificationProviderBuilder, err := notificationchannelprovider.NewBuilder(
		notificationChannelWebhookFactory(httpTracing),
		notificationBuilderOptions...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("configure notification channel provider builder: %w", err)
	}
	var notificationProviderResolver ports.NotificationChannelProviderResolver
	if notificationChannelSecretResolver != nil {
		if uowFactory == nil {
			return nil, nil, fmt.Errorf("%s requires a unit of work factory", notificationChannelSecretRefsEnv)
		}
		notificationProviderResolver, err = notificationchannelprovider.NewResolver(uowFactory, notificationProviderBuilder)
		if err != nil {
			return nil, nil, fmt.Errorf("configure notification channel provider resolver: %w", err)
		}
	}
	notificationChannelTester, err := notificationchannelcheck.NewService(
		notificationProviderBuilder,
		notificationchannelcheck.WithClock(func() time.Time { return time.Now().UTC() }),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("configure notification channel tester: %w", err)
	}
	opts = append(opts, transporthttp.WithNotificationChannelTester(notificationChannelTester))

	reportNotificationOptions := []reportnotification.Option{}
	imProvider, imFormat, imConfigured, err := reportIMProviderFromEnv(getenv, httpTracing)
	if imConfigured {
		if err != nil {
			return nil, nil, err
		}
		reportNotificationOptions = append(reportNotificationOptions, reportnotification.WithIMProvider(imProvider))
		logger.Info("configured report notification retry IM provider", "provider", "webhook", "format", imFormat)
	}
	if notificationChannelSecretResolver != nil {
		reportNotificationOptions = append(
			reportNotificationOptions,
			reportnotification.WithNotificationChannelProviderResolver(notificationProviderResolver),
		)
		logger.Info("configured report notification retry provider resolver", "provider", "webhook,email")
	}
	if len(reportNotificationOptions) > 0 {
		reportNotifier, err := reportnotification.NewService(uowFactory, reportNotificationOptions...)
		if err != nil {
			return nil, nil, fmt.Errorf("configure report notification retry service: %w", err)
		}
		opts = append(opts, transporthttp.WithReportNotificationSender(reportNotifier))
	} else {
		logger.Warn("report notification retry is disabled; configure OPENCLARION_IM_WEBHOOK_* or OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON to enable manual final-report notification retries")
	}
	if notificationProviderResolver != nil {
		diagnosisNotificationRetrier, err := diagnosisnotification.NewService(
			uowFactory,
			notificationProviderResolver,
			diagnosisnotification.WithClock(diagnosisNotificationRetryNowUTC),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("configure diagnosis notification retry service: %w", err)
		}
		opts = append(opts, transporthttp.WithDiagnosisRoomNotificationRetrier(diagnosisNotificationRetrier))
		logger.Info("configured diagnosis notification retry provider resolver", "provider", "webhook,email")
	} else {
		logger.Warn("diagnosis notification retry is disabled; configure OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON to enable manual diagnosis-room notification retries")
	}

	policyTriggerOptions := []reportpolicytrigger.Option{}
	if cmdbProvider != nil {
		policyTriggerOptions = append(policyTriggerOptions, reportpolicytrigger.WithCMDBProvider(cmdbProvider))
	}
	if autoDiagnosisTrigger != nil {
		policyTriggerOptions = append(policyTriggerOptions, reportpolicytrigger.WithAutoDiagnosisTrigger(autoDiagnosisTrigger))
	}
	policyTrigger, err := reportpolicytrigger.NewService(uowFactory, starter, alertSourceProviders, policyTriggerOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("configure report workflow policy replay trigger: %w", err)
	}
	opts = append(opts, transporthttp.WithReportWorkflowPolicyReplayTrigger(policyTrigger))
	logger.Info("configured report workflow policy replay trigger", "providers", "profile")
	if scheduleSyncer != nil {
		opts = append(opts, transporthttp.WithReportWorkflowScheduleSynchronizer(scheduleSyncer))
		logger.Info("configured report workflow schedule synchronizer", "provider", "temporal")
	}
	weComAppCallback, err := diagnosisWeComAppCallbackFromEnv(getenv)
	if err != nil {
		return nil, nil, err
	}
	if weComAppCallback != nil {
		opts = append(opts, transporthttp.WithDiagnosisWeComAppCallback(weComAppCallback))
		logger.Info("configured Enterprise WeChat app callback verifier")
		if diagnosisWorkflows != nil {
			opts = append(opts, transporthttp.WithDiagnosisWeComAppCallbackWorkflowRouter(diagnosisWorkflows))
			logger.Info("configured Enterprise WeChat app callback message router")
		} else {
			logger.Warn("Enterprise WeChat app callback message routing is disabled; diagnosis workflow client is not configured")
		}
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
		reportTriggerOptions := []reporttrigger.Option{}
		if cmdbProvider != nil {
			reportTriggerOptions = append(reportTriggerOptions, reporttrigger.WithCMDBProvider(cmdbProvider))
		}
		service, err := reporttrigger.NewService(provider, uowFactory, starter, reportTriggerOptions...)
		if err != nil {
			return nil, nil, fmt.Errorf("configure report HTTP trigger service: %w", err)
		}
		logger.Info("configured report HTTP trigger", "provider", "prometheus")
		opts = append(opts, transporthttp.WithReportReplayTrigger(service))
	}
	if cmdbProviderKind != "" {
		logger.Info("configured HTTP replay CMDB enrichment", "provider", cmdbProviderKind)
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

func directorySyncerFromEnv(
	getenv getenvFunc,
	uowFactory ports.UnitOfWorkFactory,
	httpTracing *observabilitytracing.HTTPTracing,
) (configuredDirectorySyncer, bool, error) {
	if !iamDirectorySyncConfigured(getenv) {
		return nil, false, nil
	}
	if uowFactory == nil {
		return nil, true, fmt.Errorf("IAM directory sync requires a unit of work factory")
	}
	issuerURL := firstNonEmptyEnv(getenv, iamDirectoryIssuerEnv, iamOIDCIssuerEnv, standardOIDCIssuerEnv)
	if issuerURL == "" {
		return nil, true, fmt.Errorf("%s, %s, or %s is required when IAM directory sync is configured", iamDirectoryIssuerEnv, iamOIDCIssuerEnv, standardOIDCIssuerEnv)
	}
	clientID := firstNonEmptyEnv(getenv, iamDirectoryClientIDEnv, iamOIDCClientIDEnv, standardOIDCClientIDEnv)
	if clientID == "" {
		return nil, true, fmt.Errorf("%s, %s, or %s is required when IAM directory sync is configured", iamDirectoryClientIDEnv, iamOIDCClientIDEnv, standardOIDCClientIDEnv)
	}
	clientSecret := firstNonEmptyEnv(getenv, iamDirectoryClientSecretEnv, iamOIDCClientSecretEnv, standardOIDCClientSecretEnv)
	if clientSecret == "" {
		return nil, true, fmt.Errorf("%s, %s, or %s is required when IAM directory sync is configured", iamDirectoryClientSecretEnv, iamOIDCClientSecretEnv, standardOIDCClientSecretEnv)
	}

	providerCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := directoryiam.NewProvider(providerCtx, directoryiam.Config{
		IssuerURL:        issuerURL,
		DirectoryBaseURL: getenv(iamDirectoryBaseURLEnv),
		TokenURL:         getenv(iamDirectoryTokenURLEnv),
		ClientID:         clientID,
		ClientSecret:     clientSecret,
		Scopes: optionalCSVValues(firstNonEmptyEnv(
			getenv,
			iamDirectoryScopesEnv,
			standardDirectoryScopesEnv,
		)),
		HTTPClient: outboundHTTPClient(httpTracing, 10*time.Second),
	})
	if err != nil {
		return nil, true, fmt.Errorf("configure IAM directory provider: %w", err)
	}
	providerName := directoryProviderNameFromEnv(getenv)
	syncer, err := directorysync.NewService(uowFactory, map[string]ports.DirectoryProvider{
		providerName: provider,
	})
	if err != nil {
		return nil, true, fmt.Errorf("configure IAM directory syncer: %w", err)
	}
	return defaultProviderDirectorySyncer{provider: providerName, syncer: syncer}, true, nil
}

func periodicDirectorySyncerFromEnv(
	getenv getenvFunc,
	uowFactory ports.UnitOfWorkFactory,
	httpTracing *observabilitytracing.HTTPTracing,
) (configuredDirectorySyncer, time.Duration, bool, error) {
	interval, enabled, err := directorySyncIntervalFromEnv(getenv)
	if err != nil {
		return nil, 0, true, err
	}
	if !enabled {
		return nil, 0, false, nil
	}
	syncer, configured, err := directorySyncerFromEnv(getenv, uowFactory, httpTracing)
	if err != nil {
		return nil, 0, true, err
	}
	if !configured {
		return nil, 0, true, fmt.Errorf("%s requires IAM directory sync configuration", iamDirectorySyncIntervalEnv)
	}
	return syncer, interval, true, nil
}

func directorySyncIntervalFromEnv(getenv getenvFunc) (time.Duration, bool, error) {
	if strings.TrimSpace(getenv(iamDirectorySyncIntervalEnv)) == "" {
		return 0, false, nil
	}
	interval, err := positiveDurationSecondsFromEnv(getenv, iamDirectorySyncIntervalEnv, 0)
	if err != nil {
		return 0, true, err
	}
	if interval < minIAMDirectorySyncInterval {
		return 0, true, fmt.Errorf("%s must be at least %s", iamDirectorySyncIntervalEnv, minIAMDirectorySyncInterval)
	}
	return interval, true, nil
}

func iamDirectorySyncConfigured(getenv getenvFunc) bool {
	return anyEnv(getenv,
		iamDirectoryProviderNameEnv,
		iamDirectoryIssuerEnv,
		iamDirectoryBaseURLEnv,
		iamDirectoryTokenURLEnv,
		iamDirectoryClientIDEnv,
		iamDirectoryClientSecretEnv,
		iamDirectoryScopesEnv,
		standardDirectoryScopesEnv,
	)
}

func directoryProviderNameFromEnv(getenv getenvFunc) string {
	providerName := strings.TrimSpace(getenv(iamDirectoryProviderNameEnv))
	if providerName == "" {
		return "ops_iam"
	}
	return providerName
}

type configuredDirectorySyncer interface {
	Sync(context.Context, directorysync.SyncRequest) (directorysync.Result, error)
}

type defaultProviderDirectorySyncer struct {
	provider string
	syncer   *directorysync.Service
}

func (s defaultProviderDirectorySyncer) Sync(ctx context.Context, req directorysync.SyncRequest) (directorysync.Result, error) {
	if s.syncer == nil {
		return directorysync.Result{}, fmt.Errorf("IAM directory sync requires a configured service")
	}
	if strings.TrimSpace(req.Provider) == "" {
		req.Provider = s.provider
	}
	return s.syncer.Sync(ctx, req)
}

type periodicDirectorySyncer interface {
	Sync(context.Context, directorysync.SyncRequest) (directorysync.Result, error)
}

func runPeriodicDirectorySync(
	ctx context.Context,
	logger *slog.Logger,
	syncer periodicDirectorySyncer,
	interval time.Duration,
) error {
	if syncer == nil {
		return fmt.Errorf("periodic IAM directory sync requires a syncer")
	}
	if interval <= 0 {
		return fmt.Errorf("periodic IAM directory sync requires a positive interval")
	}
	runOnce := func(trigger string) bool {
		startedAt := time.Now()
		result, err := syncer.Sync(ctx, directorysync.SyncRequest{})
		if err != nil {
			if ctx.Err() != nil {
				return false
			}
			logger.Warn("periodic IAM directory sync failed", "trigger", trigger, "error", err)
			return true
		}
		logger.Info(
			"periodic IAM directory sync completed",
			"trigger", trigger,
			"department_pages", result.DepartmentPages,
			"user_pages", result.UserPages,
			"departments_upserted", result.DepartmentsUpserted,
			"users_upserted", result.UsersUpserted,
			"duration", time.Since(startedAt).String(),
		)
		return true
	}
	if !runOnce("startup") {
		return nil
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !runOnce("interval") {
				return nil
			}
		}
	}
}

func alertSourceProviderFactories(
	httpTracing *observabilitytracing.HTTPTracing,
) alertsourceprovider.ProviderFactories {
	prometheusProfileFactory := func(
		profile domain.AlertSourceProfile,
		credentials alertsourceprovider.Credentials,
	) (ports.ActiveAlertProvider, error) {
		providerOpts := []metricsprometheus.Option{
			metricsprometheus.WithRoundTripperDecorator(outboundTransportDecorator(httpTracing)),
		}
		if credentials.BearerToken != "" {
			providerOpts = append(providerOpts, metricsprometheus.WithBearer(credentials.BearerToken))
		}
		return metricsprometheus.NewProvider(profile.BaseURL, providerOpts...)
	}
	alertmanagerProfileFactory := func(
		profile domain.AlertSourceProfile,
		credentials alertsourceprovider.Credentials,
	) (ports.ActiveAlertProvider, error) {
		providerOpts := []metricsalertmanager.Option{
			metricsalertmanager.WithRoundTripperDecorator(outboundTransportDecorator(httpTracing)),
		}
		if credentials.BearerToken != "" {
			providerOpts = append(providerOpts, metricsalertmanager.WithBearer(credentials.BearerToken))
		}
		return metricsalertmanager.NewProvider(profile.BaseURL, providerOpts...)
	}
	return alertsourceprovider.ProviderFactories{
		domain.AlertSourceKindPrometheus:   prometheusProfileFactory,
		domain.AlertSourceKindAlertmanager: alertmanagerProfileFactory,
	}
}

type reportWorkflowScheduleReconciler interface {
	Reconcile(context.Context, []domain.ReportWorkflowSchedule) (temporalpkg.ReportWorkflowScheduleReconcileResult, error)
}

func reconcileReportWorkflowSchedules(
	ctx context.Context,
	uowFactory ports.UnitOfWorkFactory,
	reconciler reportWorkflowScheduleReconciler,
) (temporalpkg.ReportWorkflowScheduleReconcileResult, error) {
	if uowFactory == nil {
		return temporalpkg.ReportWorkflowScheduleReconcileResult{}, fmt.Errorf("report workflow schedule reconciliation requires a unit of work factory: %w", domain.ErrInvariantViolation)
	}
	if reconciler == nil {
		return temporalpkg.ReportWorkflowScheduleReconcileResult{}, fmt.Errorf("report workflow schedule reconciliation requires a synchronizer: %w", domain.ErrInvariantViolation)
	}

	limit := temporalpkg.DefaultReportWorkflowScheduleReconcileLimit
	var schedules []domain.ReportWorkflowSchedule
	err := uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if uow == nil {
			return fmt.Errorf("report workflow schedule reconciliation: unit of work is nil: %w", domain.ErrInvariantViolation)
		}
		var lerr error
		schedules, lerr = uow.Config().ListReportWorkflowSchedules(ctx, limit+1)
		return lerr
	})
	if err != nil {
		return temporalpkg.ReportWorkflowScheduleReconcileResult{}, err
	}
	if len(schedules) > limit {
		return temporalpkg.ReportWorkflowScheduleReconcileResult{}, fmt.Errorf("report workflow schedule reconciliation exceeds %d schedules: %w", limit, domain.ErrInvariantViolation)
	}
	return reconciler.Reconcile(ctx, schedules)
}

func alertSourceSecretResolverFromEnv(getenv getenvFunc) (ports.SecretResolver, error) {
	raw := strings.TrimSpace(getenv(alertSourceSecretRefsEnv))
	if raw == "" {
		return nil, nil
	}
	resolver, err := secretenvmap.NewResolverFromJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is invalid: %w", alertSourceSecretRefsEnv, err)
	}
	return resolver, nil
}

func notificationChannelSecretResolverFromEnv(getenv getenvFunc) (ports.SecretResolver, error) {
	raw := strings.TrimSpace(getenv(notificationChannelSecretRefsEnv))
	if raw == "" {
		return nil, nil
	}
	resolver, err := secretenvmap.NewResolverFromJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is invalid: %w", notificationChannelSecretRefsEnv, err)
	}
	return resolver, nil
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
	authProvider, authProviderName, err := diagnosisAuthProviderFromEnv(getenv, httpTracing)
	if err != nil {
		return nil, err
	}
	authProviderNames := []string{}
	if strings.TrimSpace(authProviderName) != "" {
		authProviderNames = append(authProviderNames, authProviderName)
	}
	var sessionIssuer *diagnosisauth.SessionTokenService
	if strings.TrimSpace(getenv(diagnosisSessionSigningKeyEnv)) != "" {
		sessionIssuer, err = diagnosisSessionTokenServiceFromEnv(getenv)
		if err != nil {
			return nil, err
		}
	}
	if authProvider == nil {
		logger.Warn("diagnosis WebSocket auth is disabled; configure IAM OIDC or set OPENCLARION_DIAGNOSIS_AUTH_MODE explicitly to static or ldap")
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

	ticketService, err := diagnosisauth.NewService(tickets, diagnosisauth.DefaultTicketPolicy(), nil)
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis WebSocket ticket service: %w", err)
	}
	roomStartService, err := diagnosisroomstart.NewService(uowFactory, starter)
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis room starter service: %w", err)
	}
	opts := []transporthttp.ServerOption{
		transporthttp.WithDiagnosisAuth(authProvider, ticketService, diagnosisChatSessionResolver{uowFactory: uowFactory}, authProviderNames...),
		transporthttp.WithDiagnosisRoomStarter(roomStartService),
		transporthttp.WithDiagnosisRoomWorkflowClient(workflows),
	}
	if sessionIssuer != nil {
		opts = append(opts, transporthttp.WithDiagnosisAuthSessionIssuer(sessionIssuer))
	} else {
		logger.Warn("diagnosis browser session issuance is disabled; set OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY to enable IAM OIDC or LDAP browser sessions")
	}
	if visibility, ok := workflows.(ports.DiagnosisRoomWorkflowVisibilityLookup); ok {
		opts = append(opts, transporthttp.WithDiagnosisRoomWorkflowVisibilityLookup(visibility))
		roomCloseService, err := diagnosisroomclose.NewService(uowFactory, visibility)
		if err != nil {
			return nil, fmt.Errorf("configure diagnosis room close service: %w", err)
		}
		opts = append(opts, transporthttp.WithDiagnosisRoomCloser(roomCloseService))
	}
	if originPolicy != nil {
		opts = append(opts, transporthttp.WithDiagnosisWebSocketOriginCheck(originPolicy.CheckWebSocketOrigin))
	}
	logger.Info("configured diagnosis WebSocket auth and relay", "provider", authProviderName, "providers", strings.Join(authProviderNames, ","))
	return opts, nil
}

func diagnosisAuthProviderFromEnv(
	getenv getenvFunc,
	httpTracing *observabilitytracing.HTTPTracing,
) (ports.AuthProvider, string, error) {
	mode := strings.ToLower(strings.TrimSpace(getenv(diagnosisAuthModeEnv)))
	if mode == "" {
		switch {
		case diagnosisOIDCAuthConfigured(getenv):
			mode = "oidc"
		case diagnosisStaticAuthConfigured(getenv):
			mode = "static"
		default:
			return nil, "", nil
		}
	}
	switch mode {
	case "ldap":
		defaultRoles, err := diagnosisAuthRolesFromCSVEnv(getenv, diagnosisLDAPDefaultRolesEnv, false)
		if err != nil {
			return nil, "", err
		}
		startTLS, err := boolFromEnv(getenv, diagnosisLDAPStartTLSEnv)
		if err != nil {
			return nil, "", err
		}
		allowPlaintext, err := boolFromEnv(getenv, diagnosisLDAPAllowPlaintextEnv)
		if err != nil {
			return nil, "", err
		}
		authProvider, err := authldap.NewProvider(authldap.Config{
			URL:                    getenv(diagnosisLDAPURLEnv),
			BaseDN:                 getenv(diagnosisLDAPBaseDNEnv),
			BindDN:                 getenv(diagnosisLDAPBindDNEnv),
			BindPassword:           getenv(diagnosisLDAPBindPasswordEnv),
			UserFilter:             getenv(diagnosisLDAPUserFilterEnv),
			SubjectAttribute:       getenv(diagnosisLDAPSubjectAttributeEnv),
			RoleAttribute:          getenv(diagnosisLDAPRoleAttributeEnv),
			OwnerRoleValues:        optionalCSVValues(getenv(diagnosisLDAPOwnerRoleValuesEnv)),
			AdminRoleValues:        optionalCSVValues(getenv(diagnosisLDAPAdminRoleValuesEnv)),
			DefaultRoles:           defaultRoles,
			StartTLS:               startTLS,
			AllowInsecurePlaintext: allowPlaintext,
		})
		if err != nil {
			return nil, "", fmt.Errorf("configure diagnosis LDAP auth provider: %w", err)
		}
		return authProvider, "ldap", nil
	case "oidc":
		oidcCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		authProvider, err := authoidc.NewProvider(oidcCtx, authoidc.Config{
			IssuerURL:            firstNonEmptyEnv(getenv, iamOIDCIssuerEnv, standardOIDCIssuerEnv, diagnosisOIDCIssuerURLEnv),
			ClientID:             firstNonEmptyEnv(getenv, iamOIDCClientIDEnv, standardOIDCClientIDEnv, diagnosisOIDCClientIDEnv),
			RoleClaim:            strings.TrimSpace(getenv(diagnosisOIDCRoleClaimEnv)),
			OwnerRoleValues:      optionalCSVValues(getenv(diagnosisOIDCOwnerRolesEnv)),
			AdminRoleValues:      optionalCSVValues(getenv(diagnosisOIDCAdminRolesEnv)),
			SupportedSigningAlgs: optionalCSVValues(getenv(diagnosisOIDCSigningAlgsEnv)),
			HTTPClient:           outboundHTTPClient(httpTracing, 10*time.Second),
		})
		if err != nil {
			return nil, "", fmt.Errorf("configure diagnosis OIDC auth provider: %w", err)
		}
		return authProvider, "oidc", nil
	case "static":
		roles, err := diagnosisAuthRolesFromCSVEnv(getenv, diagnosisStaticRolesEnv, true)
		if err != nil {
			return nil, "", err
		}
		authProvider, err := authstatic.NewProvider(authstatic.Config{
			Token:   strings.TrimSpace(getenv(diagnosisStaticBearerTokenEnv)),
			Subject: strings.TrimSpace(getenv(diagnosisStaticSubjectEnv)),
			Roles:   roles,
		})
		if err != nil {
			return nil, "", fmt.Errorf("configure diagnosis static auth provider: %w", err)
		}
		return authProvider, "static", nil
	case "wecom":
		return nil, "", fmt.Errorf("%s=wecom is not supported for browser login; use IAM OIDC and keep Enterprise WeChat callback settings only for application messages", diagnosisAuthModeEnv)
	default:
		return nil, "", fmt.Errorf("%s must be ldap, static, or oidc", diagnosisAuthModeEnv)
	}
}

func diagnosisWeComAppCallbackFromEnv(getenv getenvFunc) (*wecomcallback.Verifier, error) {
	token := strings.TrimSpace(getenv(diagnosisWeComCallbackTokenEnv))
	encodingAESKey := strings.TrimSpace(getenv(diagnosisWeComCallbackEncodingAESKeyEnv))
	receiveID := strings.TrimSpace(getenv(diagnosisWeComCallbackReceiveIDEnv))
	if token == "" && encodingAESKey == "" {
		return nil, nil
	}
	if token == "" || encodingAESKey == "" {
		return nil, fmt.Errorf("%s and %s must be configured together", diagnosisWeComCallbackTokenEnv, diagnosisWeComCallbackEncodingAESKeyEnv)
	}
	if receiveID == "" {
		receiveID = strings.TrimSpace(getenv(diagnosisWeComCorpIDEnv))
	}
	if receiveID == "" {
		return nil, fmt.Errorf("%s or %s is required when Enterprise WeChat app callback is configured", diagnosisWeComCallbackReceiveIDEnv, diagnosisWeComCorpIDEnv)
	}
	verifier, err := wecomcallback.NewVerifier(wecomcallback.Config{
		Token:          token,
		EncodingAESKey: encodingAESKey,
		ReceiveID:      receiveID,
	})
	if err != nil {
		return nil, fmt.Errorf("configure Enterprise WeChat app callback verifier: %w", err)
	}
	return verifier, nil
}

func diagnosisSessionTokenServiceFromEnv(getenv getenvFunc) (*diagnosisauth.SessionTokenService, error) {
	service, err := diagnosisauth.NewSessionTokenService(
		diagnosisauth.DefaultSessionTokenPolicy(getenv(diagnosisSessionSigningKeyEnv)),
		diagnosisAuthNowUTC,
	)
	if err != nil {
		return nil, fmt.Errorf("configure diagnosis session auth provider: %w", err)
	}
	return service, nil
}

func diagnosisOIDCAuthConfigured(getenv getenvFunc) bool {
	return anyEnv(getenv,
		diagnosisOIDCIssuerURLEnv,
		diagnosisOIDCClientIDEnv,
		diagnosisOIDCRoleClaimEnv,
		diagnosisOIDCOwnerRolesEnv,
		diagnosisOIDCAdminRolesEnv,
		diagnosisOIDCSigningAlgsEnv,
		iamOIDCIssuerEnv,
		iamOIDCClientIDEnv,
		standardOIDCIssuerEnv,
		standardOIDCClientIDEnv,
	)
}

func diagnosisStaticAuthConfigured(getenv getenvFunc) bool {
	return anyEnv(getenv,
		diagnosisStaticBearerTokenEnv,
		diagnosisStaticSubjectEnv,
		diagnosisStaticRolesEnv,
	)
}

func diagnosisAuthRolesFromCSVEnv(getenv getenvFunc, key string, required bool) ([]ports.AuthRole, error) {
	values := csvValues(getenv(key))
	if len(values) == 0 {
		if required {
			return nil, fmt.Errorf("%s is required when diagnosis auth roles are enabled", key)
		}
		return nil, nil
	}
	roles := make([]ports.AuthRole, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(value) {
		case string(ports.AuthRoleOwner):
			roles = append(roles, ports.AuthRoleOwner)
		case string(ports.AuthRoleAdmin):
			roles = append(roles, ports.AuthRoleAdmin)
		default:
			return nil, fmt.Errorf("%s contains unsupported role %q", key, value)
		}
	}
	return roles, nil
}

func outboundHTTPClient(httpTracing *observabilitytracing.HTTPTracing, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultReportLLMHTTPTimeout
	}
	if httpTracing == nil {
		return &http.Client{Timeout: timeout}
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

func firstNonEmptyEnv(getenv getenvFunc, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func positiveDurationSecondsFromEnv(getenv getenvFunc, key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer number of seconds", key)
	}
	maxSeconds := int64(1<<63-1) / int64(time.Second)
	if seconds > maxSeconds {
		return 0, fmt.Errorf("%s exceeds maximum supported duration", key)
	}
	return time.Duration(seconds) * time.Second, nil
}

func positiveIntFromEnv(getenv getenvFunc, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func boolFromEnv(getenv getenvFunc, key string) (bool, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func autoDiagnosisOptionsFromEnv(getenv getenvFunc) ([]alertdiagnosis.Option, error) {
	maxRooms, err := positiveIntFromEnv(
		getenv,
		autoDiagnosisMaxRoomsPerTriggerEnv,
		alertdiagnosis.DefaultMaxRoomsPerTrigger,
	)
	if err != nil {
		return nil, err
	}
	if maxRooms > alertdiagnosis.MaxRoomsPerTriggerLimit {
		return nil, fmt.Errorf("%s must be between 1 and %d", autoDiagnosisMaxRoomsPerTriggerEnv, alertdiagnosis.MaxRoomsPerTriggerLimit)
	}
	return []alertdiagnosis.Option{alertdiagnosis.WithMaxRoomsPerTrigger(maxRooms)}, nil
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
		return "", fmt.Errorf("parse origin")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("origin must not include userinfo")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("origin must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("origin must be absolute")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("origin must not include path, query, or fragment")
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

type reportPolicyReplayCLIConfig struct {
	PolicyID       domain.ReportWorkflowPolicyID
	WindowStart    time.Time
	WindowEnd      time.Time
	Limit          int
	CorrelationKey string
	WorkflowID     string
	Wait           bool
	WaitTimeout    time.Duration
}

type reportScheduleLiveSmokeCLIConfig struct {
	ScheduleID         domain.ReportWorkflowScheduleID
	PolicyID           domain.ReportWorkflowPolicyID
	TemporalScheduleID string
	ObservedAfter      time.Time
	WaitTimeout        time.Duration
}

type reportReplayCLITrigger interface {
	ReplayAndStart(ctx context.Context, req reporttrigger.Request) (reporttrigger.Result, error)
}

type reportPolicyReplayCLITrigger interface {
	ReplayAndStartDetailed(ctx context.Context, req reportpolicytrigger.Request) (reportpolicytrigger.Result, error)
}

type reportReplayCLIWorkflowWaiter interface {
	WaitReportBatch(ctx context.Context, handle ports.WorkflowHandle) (reportReplayCLIWorkflowResult, error)
}

type reportScheduleLiveSmokeWaiter interface {
	WaitReportSchedule(ctx context.Context, schedule domain.ReportWorkflowSchedule, cfg reportScheduleLiveSmokeCLIConfig) (reportScheduleLiveSmokeWaitResult, error)
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
	PolicyID       int64  `json:"policy_id,omitempty"`
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

type reportScheduleLiveSmokeCLIOutput struct {
	CheckedAt            string                             `json:"checked_at"`
	Request              reportScheduleLiveSmokeCLIRequest  `json:"request"`
	PersistedSchedule    reportScheduleLiveSmokeCLISchedule `json:"persisted_schedule"`
	Waited               bool                               `json:"waited"`
	ScheduleAction       reportScheduleLiveSmokeCLIAction   `json:"schedule_action"`
	LauncherWorkflow     reportScheduleLiveSmokeCLILauncher `json:"launcher_workflow"`
	ReportWorkflowResult *reportReplayCLIWorkflowResult     `json:"report_workflow_result,omitempty"`
}

type reportScheduleLiveSmokeCLIRequest struct {
	ScheduleID         int64  `json:"schedule_id"`
	PolicyID           int64  `json:"policy_id"`
	TemporalScheduleID string `json:"temporal_schedule_id,omitempty"`
	ObservedAfter      string `json:"observed_after"`
	WaitTimeout        string `json:"wait_timeout"`
}

type reportScheduleLiveSmokeCLISchedule struct {
	ID                     int64  `json:"id"`
	ReportWorkflowPolicyID int64  `json:"report_workflow_policy_id"`
	TemporalScheduleID     string `json:"temporal_schedule_id"`
	Enabled                bool   `json:"enabled"`
	Interval               string `json:"interval"`
	Offset                 string `json:"offset"`
	ReplayWindow           string `json:"replay_window"`
	ReplayDelay            string `json:"replay_delay"`
	ReplayLimit            int    `json:"replay_limit"`
	CatchupWindow          string `json:"catchup_window"`
}

type reportScheduleLiveSmokeCLIAction struct {
	ScheduleTime string `json:"schedule_time"`
	ActualTime   string `json:"actual_time"`
	WorkflowID   string `json:"workflow_id"`
	RunID        string `json:"run_id"`
}

type reportScheduleLiveSmokeCLILauncher struct {
	ScheduleID                 int64  `json:"schedule_id"`
	ReportWorkflowPolicyID     int64  `json:"report_workflow_policy_id"`
	TemporalScheduleID         string `json:"temporal_schedule_id"`
	FireTime                   string `json:"fire_time"`
	WindowStart                string `json:"window_start"`
	WindowEnd                  string `json:"window_end"`
	CorrelationKey             string `json:"correlation_key"`
	WorkflowID                 string `json:"workflow_id"`
	EventsLoaded               int    `json:"events_loaded"`
	Snapshots                  int    `json:"snapshots"`
	ReportBatchWorkflowStarted bool   `json:"report_batch_workflow_started"`
	ReportBatchWorkflowID      string `json:"report_batch_workflow_id"`
	ReportBatchRunID           string `json:"report_batch_run_id"`
}

type reportScheduleLiveSmokeWaitResult struct {
	ScheduleAction       reportScheduleLiveSmokeCLIAction
	LauncherWorkflow     temporalpkg.ReportPolicyScheduleLauncherWorkflowResult
	ReportWorkflowResult *reportReplayCLIWorkflowResult
}

type workflowBacklogCLIConfig struct {
	Query         string
	Limit         int
	WorkflowTypes []string
	Status        string
}

type workflowBacklogCLIOutput struct {
	CheckedAt      string                    `json:"checked_at"`
	Request        workflowBacklogCLIRequest `json:"request"`
	Returned       int                       `json:"returned"`
	Truncated      bool                      `json:"truncated"`
	CountsByType   map[string]int            `json:"counts_by_type"`
	CountsByStatus map[string]int            `json:"counts_by_status"`
	Workflows      []workflowBacklogCLIItem  `json:"workflows"`
}

type workflowBacklogCLIRequest struct {
	Query         string   `json:"query"`
	Limit         int      `json:"limit"`
	WorkflowTypes []string `json:"workflow_types,omitempty"`
	Status        string   `json:"status,omitempty"`
}

type workflowBacklogCLIItem struct {
	WorkflowID       string `json:"workflow_id"`
	RunID            string `json:"run_id"`
	WorkflowType     string `json:"workflow_type"`
	Status           string `json:"status"`
	TaskQueue        string `json:"task_queue,omitempty"`
	StartTime        string `json:"start_time,omitempty"`
	CloseTime        string `json:"close_time,omitempty"`
	ExecutionTime    string `json:"execution_time,omitempty"`
	AgeSeconds       int64  `json:"age_seconds,omitempty"`
	HistoryLength    int64  `json:"history_length,omitempty"`
	HistorySizeBytes int64  `json:"history_size_bytes,omitempty"`
	ParentWorkflowID string `json:"parent_workflow_id,omitempty"`
	ParentRunID      string `json:"parent_run_id,omitempty"`
	RootWorkflowID   string `json:"root_workflow_id,omitempty"`
	RootRunID        string `json:"root_run_id,omitempty"`
}

type diagnosisRoomListCLIConfig struct {
	Limit int
	Queue string
}

type diagnosisRoomListCLIOutput struct {
	CheckedAt     string                          `json:"checked_at"`
	Request       diagnosisRoomListCLIRequest     `json:"request"`
	Returned      int                             `json:"returned"`
	CountsByQueue map[string]int                  `json:"counts_by_queue"`
	Rooms         []diagnosisRoomListCLIOutputRow `json:"rooms"`
}

type diagnosisRoomListCLIRequest struct {
	Limit int    `json:"limit"`
	Queue string `json:"queue"`
}

type diagnosisRoomListCLIOutputRow struct {
	SessionID          string                            `json:"session_id"`
	ChatSessionID      int64                             `json:"chat_session_id"`
	DiagnosisTaskID    int64                             `json:"diagnosis_task_id"`
	EvidenceSnapshotID int64                             `json:"evidence_snapshot_id"`
	WorkflowID         string                            `json:"workflow_id"`
	RunID              string                            `json:"run_id"`
	TaskStatus         string                            `json:"task_status"`
	RoomStatus         string                            `json:"room_status"`
	TurnCount          int                               `json:"turn_count"`
	StartedAt          string                            `json:"started_at"`
	LastActivityAt     string                            `json:"last_activity_at"`
	ClosedAt           string                            `json:"closed_at,omitempty"`
	CloseReason        string                            `json:"close_reason,omitempty"`
	LatestConclusion   *diagnosisRoomListCLIConclusion   `json:"latest_conclusion,omitempty"`
	LatestNotification *diagnosisRoomListCLINotification `json:"latest_notification,omitempty"`
	NextStep           diagnosisRoomListCLINextStep      `json:"next_step"`
}

type diagnosisRoomListCLIConclusion struct {
	EventKind           string `json:"event_kind"`
	Status              string `json:"status,omitempty"`
	Confidence          string `json:"confidence,omitempty"`
	RequiresHumanReview *bool  `json:"requires_human_review,omitempty"`
	OccurredAt          string `json:"occurred_at"`
}

type diagnosisRoomListCLINotification struct {
	EventKind      string `json:"event_kind"`
	ProviderStatus string `json:"provider_status,omitempty"`
	OccurredAt     string `json:"occurred_at"`
}

type diagnosisRoomListCLINextStep struct {
	Queue  string `json:"queue"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

type diagnosisRoomListRoom struct {
	Room               domain.ChatSessionWithTask
	LatestConclusion   *diagnosisRoomListCLIConclusion
	LatestNotification *diagnosisRoomListCLINotification
}

type diagnosisRoomCloseCLIConfig struct {
	SessionID   string
	RunID       string
	Reason      string
	WaitTimeout time.Duration
}

type diagnosisRoomCloseCLIOutput struct {
	CheckedAt         string                                 `json:"checked_at"`
	Request           diagnosisRoomCloseCLIRequest           `json:"request"`
	Signaled          bool                                   `json:"signaled"`
	Workflow          diagnosisRoomCloseCLIWorkflow          `json:"workflow"`
	CloseEvent        diagnosisRoomCloseCLIEvent             `json:"close_event"`
	NotificationEvent diagnosisRoomCloseCLINotificationEvent `json:"notification_event"`
}

type diagnosisRoomCloseCLIRequest struct {
	SessionID   string `json:"session_id"`
	WorkflowID  string `json:"workflow_id"`
	RunID       string `json:"run_id,omitempty"`
	Reason      string `json:"reason"`
	WaitTimeout string `json:"wait_timeout"`
}

type diagnosisRoomCloseCLIWorkflow struct {
	SessionID       string                               `json:"session_id"`
	ChatSessionID   int64                                `json:"chat_session_id"`
	DiagnosisTaskID int64                                `json:"diagnosis_task_id"`
	Status          string                               `json:"status"`
	TurnCount       int                                  `json:"turn_count"`
	ClosedAt        string                               `json:"closed_at"`
	CloseReason     string                               `json:"close_reason"`
	FinalConclusion diagnosisRoomCloseCLIFinalConclusion `json:"final_conclusion"`
}

type diagnosisRoomCloseCLIEvent struct {
	ID                int64                                `json:"id"`
	Kind              string                               `json:"kind"`
	OccurredAt        string                               `json:"occurred_at"`
	ConclusionVersion string                               `json:"conclusion_version,omitempty"`
	FinalConclusion   diagnosisRoomCloseCLIFinalConclusion `json:"final_conclusion,omitempty"`
}

type diagnosisRoomCloseCLIFinalConclusion struct {
	Status                  string   `json:"status"`
	Source                  string   `json:"source"`
	Reason                  string   `json:"reason,omitempty"`
	EvidenceSnapshotID      int64    `json:"evidence_snapshot_id,omitempty"`
	ConclusionVersion       string   `json:"conclusion_version,omitempty"`
	RecordedAt              string   `json:"recorded_at,omitempty"`
	ConfirmedBy             string   `json:"confirmed_by,omitempty"`
	SupplementalContextRefs []string `json:"supplemental_context_refs,omitempty"`
	AssistantTurnID         int64    `json:"assistant_turn_id,omitempty"`
	AssistantMessageID      string   `json:"assistant_message_id,omitempty"`
	AssistantSequence       int      `json:"assistant_sequence,omitempty"`
	AssistantOccurredAt     string   `json:"assistant_occurred_at,omitempty"`
	Content                 string   `json:"content,omitempty"`
	Confidence              string   `json:"confidence,omitempty"`
	RequiresHumanReview     *bool    `json:"requires_human_review,omitempty"`
}

type diagnosisRoomCloseCLINotificationEvent struct {
	ID                int64  `json:"id"`
	Kind              string `json:"kind"`
	OccurredAt        string `json:"occurred_at"`
	IdempotencyKey    string `json:"idempotency_key"`
	ProviderMessageID string `json:"provider_message_id,omitempty"`
	ProviderStatus    string `json:"provider_status"`
}

type diagnosisRoomCloseNotificationPayload struct {
	Kind                         string                                   `json:"kind"`
	Source                       string                                   `json:"source"`
	SessionID                    string                                   `json:"session_id"`
	DiagnosisTaskID              int64                                    `json:"diagnosis_task_id"`
	ChatSessionID                int64                                    `json:"chat_session_id"`
	EvidenceSnapshotID           int64                                    `json:"evidence_snapshot_id,omitempty"`
	AlertGroupID                 int64                                    `json:"alert_group_id,omitempty"`
	NotificationChannelProfileID int64                                    `json:"notification_channel_profile_id,omitempty"`
	OwnerSubject                 string                                   `json:"owner_subject"`
	TurnCount                    int                                      `json:"turn_count"`
	CloseReason                  string                                   `json:"close_reason"`
	FinalConclusion              temporalpkg.DiagnosisRoomFinalConclusion `json:"final_conclusion,omitempty"`
	IdempotencyKey               string                                   `json:"idempotency_key"`
	ProviderMessageID            string                                   `json:"provider_message_id"`
	ProviderStatus               string                                   `json:"provider_status"`
	ProviderRaw                  json.RawMessage                          `json:"provider_raw,omitempty"`
}

type diagnosisRoomCloseEventPayload struct {
	Kind               string                                   `json:"kind"`
	Source             string                                   `json:"source"`
	SessionID          string                                   `json:"session_id"`
	ChatSessionID      int64                                    `json:"chat_session_id"`
	DiagnosisTaskID    int64                                    `json:"diagnosis_task_id"`
	EvidenceSnapshotID int64                                    `json:"evidence_snapshot_id,omitempty"`
	OwnerSubject       string                                   `json:"owner_subject"`
	Status             string                                   `json:"status"`
	TurnCount          int                                      `json:"turn_count"`
	CloseReason        string                                   `json:"close_reason"`
	ClosedAt           time.Time                                `json:"closed_at"`
	FinalConclusion    temporalpkg.DiagnosisRoomFinalConclusion `json:"final_conclusion"`
	ConclusionVersion  string                                   `json:"conclusion_version"`
}

type diagnosisRoomCloseEvents struct {
	CloseEvent        domain.DiagnosisTaskEvent
	NotificationEvent domain.DiagnosisTaskEvent
	ClosePayload      diagnosisRoomCloseEventPayload
	Notification      diagnosisRoomCloseNotificationPayload
}

type diagnosisRoomCloseWorkflowWaiter interface {
	SignalAndWaitDiagnosisRoomClose(ctx context.Context, cfg diagnosisRoomCloseCLIConfig) (temporalpkg.DiagnosisRoomWorkflowResult, error)
}

type diagnosisRoomCloseEventsLoader interface {
	LoadDiagnosisRoomCloseEvents(ctx context.Context, taskID domain.DiagnosisTaskID) (diagnosisRoomCloseEvents, error)
}

type workflowBacklogLister interface {
	ListWorkflow(ctx context.Context, request *workflowservicepb.ListWorkflowExecutionsRequest) (*workflowservicepb.ListWorkflowExecutionsResponse, error)
}

type diagnosisRoomListLoader interface {
	ListDiagnosisRooms(ctx context.Context, limit int) ([]diagnosisRoomListRoom, error)
}

type temporalDiagnosisRoomCloseWaiter struct {
	client temporalclient.Client
}

type postgresDiagnosisRoomCloseEventsLoader struct {
	factory ports.UnitOfWorkFactory
}

type postgresDiagnosisRoomListLoader struct {
	factory ports.UnitOfWorkFactory
}

type temporalReportReplayCLIWaiter struct {
	client temporalclient.Client
}

type temporalReportScheduleLiveSmokeWaiter struct {
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

	temporalAddr := temporalHostPortFrom(getenv)
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
	cmdbProvider, _, err := cmdbProviderFromEnv(getenv, httpTracing)
	if err != nil {
		return err
	}
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
	temporalTaskQueue, err := temporalTaskQueueFromEnv(getenv)
	if err != nil {
		return err
	}

	starter, err := temporalpkg.NewReportStarter(tc, temporalpkg.WithReportStarterTaskQueue(temporalTaskQueue))
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
	reportTriggerOptions := []reporttrigger.Option{}
	if cmdbProvider != nil {
		reportTriggerOptions = append(reportTriggerOptions, reporttrigger.WithCMDBProvider(cmdbProvider))
	}
	service, err := reporttrigger.NewService(provider, repository.NewFactory(entClient), starter, reportTriggerOptions...)
	if err != nil {
		return fmt.Errorf("configure report CLI trigger service: %w", err)
	}
	return runReportReplayCLITrigger(ctx, service, temporalReportReplayCLIWaiter{client: tc}, cfg, stdout)
}

func runReportPolicyReplayCLI(
	ctx context.Context,
	logger *slog.Logger,
	getenv getenvFunc,
	args []string,
	stdout io.Writer,
) error {
	cfg, err := parseReportPolicyReplayCLIArgs(args)
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
	cmdbProvider, _, err := cmdbProviderFromEnv(getenv, httpTracing)
	if err != nil {
		return err
	}
	temporalInterceptors, err := temporalClientInterceptors(httpTracing)
	if err != nil {
		return err
	}
	tc, err := temporalclient.Dial(temporalclient.Options{
		HostPort:     temporalHostPortFrom(getenv),
		Logger:       temporallog.NewStructuredLogger(logger),
		Interceptors: temporalInterceptors,
	})
	if err != nil {
		return fmt.Errorf("dial temporal: %w", err)
	}
	defer tc.Close()
	temporalTaskQueue, err := temporalTaskQueueFromEnv(getenv)
	if err != nil {
		return err
	}
	starter, err := temporalpkg.NewReportStarter(tc, temporalpkg.WithReportStarterTaskQueue(temporalTaskQueue))
	if err != nil {
		return err
	}
	diagnosisStarter, err := temporalpkg.NewDiagnosisRoomStarter(tc, temporalpkg.WithDiagnosisRoomStarterTaskQueue(temporalTaskQueue))
	if err != nil {
		return err
	}

	secretResolver, err := alertSourceSecretResolverFromEnv(getenv)
	if err != nil {
		return err
	}
	var providerBuilderOptions []alertsourceprovider.Option
	if secretResolver != nil {
		providerBuilderOptions = append(providerBuilderOptions, alertsourceprovider.WithSecretResolver(secretResolver))
	}
	alertSourceProviders, err := alertsourceprovider.NewBuilder(alertSourceProviderFactories(httpTracing), providerBuilderOptions...)
	if err != nil {
		return fmt.Errorf("configure report policy CLI provider builder: %w", err)
	}
	uowFactory := repository.NewFactory(entClient)
	autoDiagnosisOptions, err := autoDiagnosisOptionsFromEnv(getenv)
	if err != nil {
		return err
	}
	if cmdbProvider != nil {
		autoDiagnosisOptions = append(autoDiagnosisOptions, alertdiagnosis.WithCMDBProvider(cmdbProvider))
	}
	autoDiagnosisTrigger, err := alertdiagnosis.NewService(uowFactory, diagnosisStarter, autoDiagnosisOptions...)
	if err != nil {
		return fmt.Errorf("configure report policy CLI auto diagnosis trigger: %w", err)
	}
	policyTriggerOptions := []reportpolicytrigger.Option{
		reportpolicytrigger.WithAutoDiagnosisTrigger(autoDiagnosisTrigger),
	}
	if cmdbProvider != nil {
		policyTriggerOptions = append(policyTriggerOptions, reportpolicytrigger.WithCMDBProvider(cmdbProvider))
	}
	service, err := reportpolicytrigger.NewService(
		uowFactory,
		starter,
		alertSourceProviders,
		policyTriggerOptions...,
	)
	if err != nil {
		return fmt.Errorf("configure report policy CLI trigger service: %w", err)
	}
	return runReportPolicyReplayCLITrigger(ctx, service, temporalReportReplayCLIWaiter{client: tc}, cfg, stdout)
}

func runReportScheduleLiveSmokeCLI(
	ctx context.Context,
	logger *slog.Logger,
	getenv getenvFunc,
	args []string,
	stdout io.Writer,
) error {
	cfg, err := parseReportScheduleLiveSmokeCLIArgs(args)
	if err != nil {
		return err
	}
	if cfg.ObservedAfter.IsZero() {
		cfg.ObservedAfter = reportScheduleLiveSmokeCLINowUTC()
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

	factory := repository.NewFactory(entClient)
	var schedule domain.ReportWorkflowSchedule
	if err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if uow == nil {
			return fmt.Errorf("report schedule live smoke: unit of work is nil: %w", domain.ErrInvariantViolation)
		}
		var serr error
		schedule, serr = uow.Config().FindReportWorkflowScheduleByID(ctx, cfg.ScheduleID)
		return serr
	}); err != nil {
		return fmt.Errorf("load report workflow schedule: %w", err)
	}

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
		HostPort:     temporalHostPortFrom(getenv),
		Logger:       temporallog.NewStructuredLogger(logger),
		Interceptors: temporalInterceptors,
	})
	if err != nil {
		return fmt.Errorf("dial temporal: %w", err)
	}
	defer tc.Close()
	return runReportScheduleLiveSmokeCLIWithDependencies(ctx, temporalReportScheduleLiveSmokeWaiter{client: tc}, schedule, cfg, stdout)
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

func parseReportPolicyReplayCLIArgs(args []string) (reportPolicyReplayCLIConfig, error) {
	var rawStart, rawEnd string
	var rawPolicyID int64
	cfg := reportPolicyReplayCLIConfig{Limit: defaultReportReplayCLILimit}
	fs := flag.NewFlagSet(reportPolicyReplayCLICommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int64Var(&rawPolicyID, "policy-id", 0, "enabled report workflow policy ID")
	fs.StringVar(&rawStart, "window-start", "", "inclusive replay window start (RFC3339)")
	fs.StringVar(&rawEnd, "window-end", "", "exclusive replay window end (RFC3339)")
	fs.IntVar(&cfg.Limit, "limit", defaultReportReplayCLILimit, "maximum alert events to load")
	fs.StringVar(&cfg.CorrelationKey, "correlation-key", "", "optional final-report correlation key")
	fs.StringVar(&cfg.WorkflowID, "workflow-id", "", "optional Temporal workflow ID")
	fs.BoolVar(&cfg.Wait, "wait", false, "wait for the report workflow to complete")
	fs.DurationVar(&cfg.WaitTimeout, "wait-timeout", defaultReportReplayCLIWait, "maximum duration to wait when --wait is set")
	if err := fs.Parse(args); err != nil {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("%w\n%s", err, reportPolicyReplayCLIUsage())
	}
	if fs.NArg() != 0 {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("unexpected positional arguments: %s\n%s", strings.Join(fs.Args(), " "), reportPolicyReplayCLIUsage())
	}
	if rawPolicyID <= 0 {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("--policy-id must be > 0 (got %d)\n%s", rawPolicyID, reportPolicyReplayCLIUsage())
	}
	if strings.TrimSpace(rawStart) == "" {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("--window-start is required\n%s", reportPolicyReplayCLIUsage())
	}
	windowStart, err := time.Parse(time.RFC3339, strings.TrimSpace(rawStart))
	if err != nil {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("parse --window-start: %w\n%s", err, reportPolicyReplayCLIUsage())
	}
	if strings.TrimSpace(rawEnd) == "" {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("--window-end is required\n%s", reportPolicyReplayCLIUsage())
	}
	windowEnd, err := time.Parse(time.RFC3339, strings.TrimSpace(rawEnd))
	if err != nil {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("parse --window-end: %w\n%s", err, reportPolicyReplayCLIUsage())
	}
	if cfg.Limit <= 0 {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("--limit must be > 0 (got %d)\n%s", cfg.Limit, reportPolicyReplayCLIUsage())
	}
	if cfg.Wait && cfg.WaitTimeout <= 0 {
		return reportPolicyReplayCLIConfig{}, fmt.Errorf("--wait-timeout must be > 0 when --wait is set (got %s)\n%s", cfg.WaitTimeout, reportPolicyReplayCLIUsage())
	}
	cfg.PolicyID = domain.ReportWorkflowPolicyID(rawPolicyID)
	cfg.WindowStart = windowStart
	cfg.WindowEnd = windowEnd
	cfg.CorrelationKey = strings.TrimSpace(cfg.CorrelationKey)
	cfg.WorkflowID = strings.TrimSpace(cfg.WorkflowID)
	return cfg, nil
}

func parseReportScheduleLiveSmokeCLIArgs(args []string) (reportScheduleLiveSmokeCLIConfig, error) {
	var rawScheduleID, rawPolicyID int64
	var rawObservedAfter string
	cfg := reportScheduleLiveSmokeCLIConfig{WaitTimeout: defaultReportScheduleLiveSmokeWait}
	fs := flag.NewFlagSet(reportScheduleLiveSmokeCLICommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int64Var(&rawScheduleID, "schedule-id", 0, "persisted report workflow schedule ID")
	fs.Int64Var(&rawPolicyID, "policy-id", 0, "expected bound report workflow policy ID")
	fs.StringVar(&cfg.TemporalScheduleID, "temporal-schedule-id", "", "optional expected Temporal Schedule ID")
	fs.StringVar(&rawObservedAfter, "observed-after", "", "only accept schedule actions at or after this RFC3339 time")
	fs.DurationVar(&cfg.WaitTimeout, "wait-timeout", defaultReportScheduleLiveSmokeWait, "maximum duration to wait for a schedule action and report delivery")
	if err := fs.Parse(args); err != nil {
		return reportScheduleLiveSmokeCLIConfig{}, fmt.Errorf("%w\n%s", err, reportScheduleLiveSmokeCLIUsage())
	}
	if fs.NArg() != 0 {
		return reportScheduleLiveSmokeCLIConfig{}, fmt.Errorf("unexpected positional arguments: %s\n%s", strings.Join(fs.Args(), " "), reportScheduleLiveSmokeCLIUsage())
	}
	if rawScheduleID <= 0 {
		return reportScheduleLiveSmokeCLIConfig{}, fmt.Errorf("--schedule-id must be > 0 (got %d)\n%s", rawScheduleID, reportScheduleLiveSmokeCLIUsage())
	}
	if rawPolicyID <= 0 {
		return reportScheduleLiveSmokeCLIConfig{}, fmt.Errorf("--policy-id must be > 0 (got %d)\n%s", rawPolicyID, reportScheduleLiveSmokeCLIUsage())
	}
	cfg.TemporalScheduleID = strings.TrimSpace(cfg.TemporalScheduleID)
	if cfg.TemporalScheduleID != "" {
		if err := validateCLIIdentifier("--temporal-schedule-id", cfg.TemporalScheduleID); err != nil {
			return reportScheduleLiveSmokeCLIConfig{}, fmt.Errorf("%w\n%s", err, reportScheduleLiveSmokeCLIUsage())
		}
	}
	if strings.TrimSpace(rawObservedAfter) != "" {
		observedAfter, err := time.Parse(time.RFC3339, strings.TrimSpace(rawObservedAfter))
		if err != nil {
			return reportScheduleLiveSmokeCLIConfig{}, fmt.Errorf("parse --observed-after: %w\n%s", err, reportScheduleLiveSmokeCLIUsage())
		}
		cfg.ObservedAfter = observedAfter.UTC()
	}
	if cfg.WaitTimeout <= 0 {
		return reportScheduleLiveSmokeCLIConfig{}, fmt.Errorf("--wait-timeout must be > 0 (got %s)\n%s", cfg.WaitTimeout, reportScheduleLiveSmokeCLIUsage())
	}
	cfg.ScheduleID = domain.ReportWorkflowScheduleID(rawScheduleID)
	cfg.PolicyID = domain.ReportWorkflowPolicyID(rawPolicyID)
	return cfg, nil
}

func validateCLIIdentifier(label, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be non-empty", label)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", label)
	}
	if strings.ContainsAny(value, "\r\n\t ") {
		return fmt.Errorf("%s must not contain whitespace", label)
	}
	return nil
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

func runReportPolicyReplayCLITrigger(
	ctx context.Context,
	trigger reportPolicyReplayCLITrigger,
	waiter reportReplayCLIWorkflowWaiter,
	cfg reportPolicyReplayCLIConfig,
	stdout io.Writer,
) error {
	if trigger == nil {
		return fmt.Errorf("report policy replay trigger must be configured")
	}
	result, err := trigger.ReplayAndStartDetailed(ctx, reportpolicytrigger.Request{
		PolicyID:       cfg.PolicyID,
		WindowStart:    cfg.WindowStart,
		WindowEnd:      cfg.WindowEnd,
		Limit:          cfg.Limit,
		CorrelationKey: cfg.CorrelationKey,
		WorkflowID:     cfg.WorkflowID,
	})
	if err != nil {
		return err
	}
	scenario := reportprompt.Scenario(result.Policy.ReportScenario)
	if !scenario.Valid() {
		return fmt.Errorf("report policy replay resolved unsupported scenario %q", result.Policy.ReportScenario)
	}
	replayCfg := reportReplayCLIConfig{
		WindowStart:    cfg.WindowStart,
		WindowEnd:      cfg.WindowEnd,
		Limit:          cfg.Limit,
		CorrelationKey: cfg.CorrelationKey,
		WorkflowID:     cfg.WorkflowID,
		Scenario:       scenario,
		Wait:           cfg.Wait,
		WaitTimeout:    cfg.WaitTimeout,
	}
	out := reportReplayCLIOutputFromResult(result.Trigger, replayCfg)
	out.Request.PolicyID = int64(cfg.PolicyID)
	if cfg.Wait && result.Trigger.Started {
		if waiter == nil {
			return fmt.Errorf("report replay workflow waiter must be configured when --wait is set")
		}
		waitCtx, cancel := context.WithTimeout(ctx, cfg.WaitTimeout)
		defer cancel()
		workflowResult, err := waiter.WaitReportBatch(waitCtx, result.Trigger.Workflow)
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
		return fmt.Errorf("write report policy replay output: %w", err)
	}
	return nil
}

func runReportScheduleLiveSmokeCLIWithDependencies(
	ctx context.Context,
	waiter reportScheduleLiveSmokeWaiter,
	schedule domain.ReportWorkflowSchedule,
	cfg reportScheduleLiveSmokeCLIConfig,
	stdout io.Writer,
) error {
	if waiter == nil {
		return fmt.Errorf("report schedule live smoke waiter must be configured")
	}
	if schedule.ID != cfg.ScheduleID {
		return fmt.Errorf("report schedule live smoke: loaded schedule id %d does not match request %d", schedule.ID, cfg.ScheduleID)
	}
	if schedule.ReportWorkflowPolicyID != cfg.PolicyID {
		return fmt.Errorf("report schedule live smoke: schedule policy id %d does not match request %d", schedule.ReportWorkflowPolicyID, cfg.PolicyID)
	}
	if cfg.TemporalScheduleID != "" && strings.TrimSpace(schedule.TemporalScheduleID) != cfg.TemporalScheduleID {
		return fmt.Errorf("report schedule live smoke: schedule Temporal ID %q does not match request %q", strings.TrimSpace(schedule.TemporalScheduleID), cfg.TemporalScheduleID)
	}
	if strings.TrimSpace(schedule.TemporalScheduleID) == "" {
		return fmt.Errorf("report schedule live smoke: persisted Temporal Schedule ID must be non-empty")
	}
	if !schedule.Enabled {
		return fmt.Errorf("report schedule live smoke: persisted schedule %d must be enabled", schedule.ID)
	}
	if cfg.ObservedAfter.IsZero() {
		cfg.ObservedAfter = reportScheduleLiveSmokeCLINowUTC()
	}

	waitCtx, cancel := context.WithTimeout(ctx, cfg.WaitTimeout)
	defer cancel()
	result, err := waiter.WaitReportSchedule(waitCtx, schedule, cfg)
	if err != nil {
		return err
	}
	out := reportScheduleLiveSmokeCLIOutputFromResult(schedule, cfg, result)
	out.CheckedAt = reportScheduleLiveSmokeCLINowUTC().Format(time.RFC3339Nano)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("write report schedule live smoke output: %w", err)
	}
	return nil
}

func reportScheduleLiveSmokeCLIOutputFromResult(
	schedule domain.ReportWorkflowSchedule,
	cfg reportScheduleLiveSmokeCLIConfig,
	result reportScheduleLiveSmokeWaitResult,
) reportScheduleLiveSmokeCLIOutput {
	launcher := result.LauncherWorkflow
	return reportScheduleLiveSmokeCLIOutput{
		Request: reportScheduleLiveSmokeCLIRequest{
			ScheduleID:         int64(cfg.ScheduleID),
			PolicyID:           int64(cfg.PolicyID),
			TemporalScheduleID: cfg.TemporalScheduleID,
			ObservedAfter:      cfg.ObservedAfter.UTC().Format(time.RFC3339Nano),
			WaitTimeout:        cfg.WaitTimeout.String(),
		},
		PersistedSchedule: reportScheduleLiveSmokeCLISchedule{
			ID:                     int64(schedule.ID),
			ReportWorkflowPolicyID: int64(schedule.ReportWorkflowPolicyID),
			TemporalScheduleID:     strings.TrimSpace(schedule.TemporalScheduleID),
			Enabled:                schedule.Enabled,
			Interval:               schedule.Interval.String(),
			Offset:                 schedule.Offset.String(),
			ReplayWindow:           schedule.ReplayWindow.String(),
			ReplayDelay:            schedule.ReplayDelay.String(),
			ReplayLimit:            schedule.ReplayLimit,
			CatchupWindow:          schedule.CatchupWindow.String(),
		},
		Waited:         true,
		ScheduleAction: result.ScheduleAction,
		LauncherWorkflow: reportScheduleLiveSmokeCLILauncher{
			ScheduleID:                 launcher.ScheduleID,
			ReportWorkflowPolicyID:     launcher.ReportWorkflowPolicyID,
			TemporalScheduleID:         strings.TrimSpace(launcher.TemporalScheduleID),
			FireTime:                   launcher.FireTime.UTC().Format(time.RFC3339Nano),
			WindowStart:                launcher.WindowStart.UTC().Format(time.RFC3339Nano),
			WindowEnd:                  launcher.WindowEnd.UTC().Format(time.RFC3339Nano),
			CorrelationKey:             strings.TrimSpace(launcher.CorrelationKey),
			WorkflowID:                 strings.TrimSpace(launcher.WorkflowID),
			EventsLoaded:               launcher.EventsLoaded,
			Snapshots:                  launcher.Snapshots,
			ReportBatchWorkflowStarted: launcher.ReportBatchWorkflowStarted,
			ReportBatchWorkflowID:      strings.TrimSpace(launcher.ReportBatchWorkflowID),
			ReportBatchRunID:           strings.TrimSpace(launcher.ReportBatchRunID),
		},
		ReportWorkflowResult: result.ReportWorkflowResult,
	}
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

func (w temporalReportScheduleLiveSmokeWaiter) WaitReportSchedule(
	ctx context.Context,
	schedule domain.ReportWorkflowSchedule,
	cfg reportScheduleLiveSmokeCLIConfig,
) (reportScheduleLiveSmokeWaitResult, error) {
	if w.client == nil {
		return reportScheduleLiveSmokeWaitResult{}, fmt.Errorf("report schedule live smoke waiter: Temporal client must be non-nil")
	}
	scheduleID := strings.TrimSpace(schedule.TemporalScheduleID)
	if scheduleID == "" {
		return reportScheduleLiveSmokeWaitResult{}, fmt.Errorf("report schedule live smoke waiter: Temporal Schedule ID must be non-empty")
	}
	handle := w.client.ScheduleClient().GetHandle(ctx, scheduleID)
	if handle == nil {
		return reportScheduleLiveSmokeWaitResult{}, fmt.Errorf("report schedule live smoke waiter: Temporal schedule handle is nil for %q", scheduleID)
	}

	ticker := time.NewTicker(defaultReportScheduleLiveSmokePoll)
	defer ticker.Stop()
	for {
		result, ok, err := w.describeAndWaitReportSchedule(ctx, handle, cfg.ObservedAfter)
		if err != nil {
			return reportScheduleLiveSmokeWaitResult{}, err
		}
		if ok {
			return result, nil
		}
		select {
		case <-ctx.Done():
			return reportScheduleLiveSmokeWaitResult{}, fmt.Errorf("wait for report schedule action %q after %s: %w", scheduleID, cfg.ObservedAfter.UTC().Format(time.RFC3339Nano), ctx.Err())
		case <-ticker.C:
		}
	}
}

func (w temporalReportScheduleLiveSmokeWaiter) describeAndWaitReportSchedule(
	ctx context.Context,
	handle temporalclient.ScheduleHandle,
	observedAfter time.Time,
) (reportScheduleLiveSmokeWaitResult, bool, error) {
	desc, err := handle.Describe(ctx)
	if err != nil {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("describe report schedule: %w", err)
	}
	if desc.Schedule.State != nil && desc.Schedule.State.Paused {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("report schedule %q is paused", handle.GetID())
	}
	action, ok := newestScheduleActionAtOrAfter(desc.Info.RecentActions, observedAfter)
	if !ok {
		return reportScheduleLiveSmokeWaitResult{}, false, nil
	}
	workflowID := strings.TrimSpace(action.StartWorkflowResult.WorkflowID)
	runID := strings.TrimSpace(action.StartWorkflowResult.FirstExecutionRunID)
	if workflowID == "" {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("report schedule %q action has empty workflow ID", handle.GetID())
	}
	if runID == "" {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("report schedule %q action has empty run ID", handle.GetID())
	}

	var launcher temporalpkg.ReportPolicyScheduleLauncherWorkflowResult
	if err := w.client.GetWorkflow(ctx, workflowID, runID).Get(ctx, &launcher); err != nil {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("wait schedule launcher workflow %q/%q: %w", workflowID, runID, err)
	}
	if !launcher.ReportBatchWorkflowStarted {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("schedule launcher workflow %q/%q did not start a report batch workflow", workflowID, runID)
	}
	reportWorkflowID := strings.TrimSpace(launcher.ReportBatchWorkflowID)
	reportRunID := strings.TrimSpace(launcher.ReportBatchRunID)
	if reportWorkflowID == "" {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("schedule launcher workflow %q/%q returned empty report batch workflow ID", workflowID, runID)
	}
	if reportRunID == "" {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("schedule launcher workflow %q/%q returned empty report batch run ID", workflowID, runID)
	}
	var reportResult temporalpkg.ReportBatchWorkflowResult
	if err := w.client.GetWorkflow(ctx, reportWorkflowID, reportRunID).Get(ctx, &reportResult); err != nil {
		return reportScheduleLiveSmokeWaitResult{}, false, fmt.Errorf("wait report batch workflow %q/%q: %w", reportWorkflowID, reportRunID, err)
	}
	mappedReport := reportReplayCLIWorkflowResult{
		SubReportIDs:               append([]int64(nil), reportResult.SubReportIDs...),
		FinalReportID:              reportResult.FinalReportID,
		NotificationIdempotencyKey: reportResult.NotificationIdempotencyKey,
		ProviderMessageID:          reportResult.ProviderMessageID,
		NotificationStatus:         reportResult.NotificationStatus,
	}
	return reportScheduleLiveSmokeWaitResult{
		ScheduleAction: reportScheduleLiveSmokeCLIAction{
			ScheduleTime: action.ScheduleTime.UTC().Format(time.RFC3339Nano),
			ActualTime:   action.ActualTime.UTC().Format(time.RFC3339Nano),
			WorkflowID:   workflowID,
			RunID:        runID,
		},
		LauncherWorkflow:     launcher,
		ReportWorkflowResult: &mappedReport,
	}, true, nil
}

func newestScheduleActionAtOrAfter(
	actions []temporalclient.ScheduleActionResult,
	observedAfter time.Time,
) (temporalclient.ScheduleActionResult, bool) {
	observedAfter = observedAfter.UTC()
	var newest temporalclient.ScheduleActionResult
	var newestActual time.Time
	found := false
	for _, action := range actions {
		if action.StartWorkflowResult == nil || action.ActualTime.IsZero() {
			continue
		}
		actualTime := action.ActualTime.UTC()
		if actualTime.Before(observedAfter) {
			continue
		}
		if !found || actualTime.After(newestActual) {
			newest = action
			newestActual = actualTime
			found = true
		}
	}
	return newest, found
}

func runWorkflowBacklogCLI(
	ctx context.Context,
	logger *slog.Logger,
	getenv getenvFunc,
	args []string,
	stdout io.Writer,
) error {
	cfg, err := parseWorkflowBacklogCLIArgs(args)
	if err != nil {
		return err
	}
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
		HostPort:     temporalHostPortFrom(getenv),
		Logger:       temporallog.NewStructuredLogger(logger),
		Interceptors: temporalInterceptors,
	})
	if err != nil {
		return fmt.Errorf("dial temporal: %w", err)
	}
	defer tc.Close()

	return runWorkflowBacklogCLIWithDependencies(ctx, tc, cfg, stdout)
}

func parseWorkflowBacklogCLIArgs(args []string) (workflowBacklogCLIConfig, error) {
	cfg := workflowBacklogCLIConfig{
		Limit:         defaultWorkflowBacklogCLILimit,
		Status:        "running",
		WorkflowTypes: append([]string(nil), defaultWorkflowBacklogCLIWorkflowTypes...),
	}
	rawTypes := strings.Join(defaultWorkflowBacklogCLIWorkflowTypes, ",")
	allTypes := false
	fs := flag.NewFlagSet(workflowBacklogCLICommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Query, "query", "", "raw Temporal visibility query")
	fs.IntVar(&cfg.Limit, "limit", defaultWorkflowBacklogCLILimit, "maximum workflows to return")
	fs.StringVar(&rawTypes, "workflow-types", rawTypes, "comma-separated workflow type names")
	fs.StringVar(&cfg.Status, "status", "running", "workflow status filter: running or all")
	fs.BoolVar(&allTypes, "all-types", false, "do not filter by OpenClarion workflow type")
	if err := fs.Parse(args); err != nil {
		return workflowBacklogCLIConfig{}, fmt.Errorf("%w\n%s", err, workflowBacklogCLIUsage())
	}
	if fs.NArg() != 0 {
		return workflowBacklogCLIConfig{}, fmt.Errorf("unexpected positional arguments: %s\n%s", strings.Join(fs.Args(), " "), workflowBacklogCLIUsage())
	}
	if cfg.Limit <= 0 || cfg.Limit > maxWorkflowBacklogCLILimit {
		return workflowBacklogCLIConfig{}, fmt.Errorf("--limit must be between 1 and %d (got %d)\n%s", maxWorkflowBacklogCLILimit, cfg.Limit, workflowBacklogCLIUsage())
	}
	cfg.Query = strings.TrimSpace(cfg.Query)
	if strings.ContainsAny(cfg.Query, "\r\n") {
		return workflowBacklogCLIConfig{}, fmt.Errorf("--query must be a single-line Temporal visibility query\n%s", workflowBacklogCLIUsage())
	}
	if cfg.Query != "" {
		cfg.Status = ""
		cfg.WorkflowTypes = nil
		return cfg, nil
	}
	cfg.Status = strings.ToLower(strings.TrimSpace(cfg.Status))
	switch cfg.Status {
	case "running", "all":
	default:
		return workflowBacklogCLIConfig{}, fmt.Errorf("--status must be running or all (got %q)\n%s", cfg.Status, workflowBacklogCLIUsage())
	}
	if allTypes {
		cfg.WorkflowTypes = nil
	} else {
		types, err := parseWorkflowBacklogCLIWorkflowTypes(rawTypes)
		if err != nil {
			return workflowBacklogCLIConfig{}, fmt.Errorf("%w\n%s", err, workflowBacklogCLIUsage())
		}
		cfg.WorkflowTypes = types
	}
	if cfg.Status == "all" && len(cfg.WorkflowTypes) == 0 {
		return workflowBacklogCLIConfig{}, fmt.Errorf("--status all with --all-types is too broad; use --query for explicit namespace-wide scans\n%s", workflowBacklogCLIUsage())
	}
	return cfg, nil
}

func parseWorkflowBacklogCLIWorkflowTypes(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if !isWorkflowBacklogCLIWorkflowType(value) {
			return nil, fmt.Errorf("--workflow-types contains unsupported workflow type %q", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--workflow-types must include at least one workflow type unless --all-types is set")
	}
	return out, nil
}

func isWorkflowBacklogCLIWorkflowType(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == ':':
		default:
			return false
		}
	}
	return true
}

func runWorkflowBacklogCLIWithDependencies(
	ctx context.Context,
	lister workflowBacklogLister,
	cfg workflowBacklogCLIConfig,
	stdout io.Writer,
) error {
	if lister == nil {
		return fmt.Errorf("workflow backlog lister must be configured")
	}
	query := workflowBacklogCLIVisibilityQuery(cfg)
	checkedAt := workflowBacklogCLINowUTC()
	out := workflowBacklogCLIOutput{
		CheckedAt: checkedAt.Format(time.RFC3339Nano),
		Request: workflowBacklogCLIRequest{
			Query:         query,
			Limit:         cfg.Limit,
			WorkflowTypes: append([]string(nil), cfg.WorkflowTypes...),
			Status:        cfg.Status,
		},
		CountsByType:   make(map[string]int),
		CountsByStatus: make(map[string]int),
		Workflows:      make([]workflowBacklogCLIItem, 0, cfg.Limit),
	}
	var nextPageToken []byte
	for len(out.Workflows) < cfg.Limit {
		pageSize := cfg.Limit - len(out.Workflows)
		if pageSize > 100 {
			pageSize = 100
		}
		resp, err := lister.ListWorkflow(ctx, &workflowservicepb.ListWorkflowExecutionsRequest{
			Query:         query,
			PageSize:      int32(pageSize),
			NextPageToken: nextPageToken,
		})
		if err != nil {
			return fmt.Errorf("list temporal workflows: %w", err)
		}
		for _, info := range resp.GetExecutions() {
			if len(out.Workflows) >= cfg.Limit {
				out.Truncated = true
				break
			}
			item := workflowBacklogCLIItemFromInfo(info, checkedAt)
			out.Workflows = append(out.Workflows, item)
			out.CountsByType[item.WorkflowType]++
			out.CountsByStatus[item.Status]++
		}
		nextPageToken = resp.GetNextPageToken()
		if len(nextPageToken) == 0 {
			break
		}
		if len(out.Workflows) >= cfg.Limit {
			out.Truncated = true
			break
		}
	}
	out.Returned = len(out.Workflows)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("write workflow backlog output: %w", err)
	}
	return nil
}

func workflowBacklogCLIVisibilityQuery(cfg workflowBacklogCLIConfig) string {
	if cfg.Query != "" {
		return cfg.Query
	}
	clauses := make([]string, 0, 2)
	if cfg.Status == "running" {
		clauses = append(clauses, `ExecutionStatus = "Running"`)
	}
	if len(cfg.WorkflowTypes) > 0 {
		typeClauses := make([]string, 0, len(cfg.WorkflowTypes))
		for _, workflowType := range cfg.WorkflowTypes {
			typeClauses = append(typeClauses, `WorkflowType = "`+workflowType+`"`)
		}
		if len(typeClauses) == 1 {
			clauses = append(clauses, typeClauses[0])
		} else {
			clauses = append(clauses, "("+strings.Join(typeClauses, " OR ")+")")
		}
	}
	return strings.Join(clauses, " AND ")
}

func workflowBacklogCLIItemFromInfo(info *workflowpb.WorkflowExecutionInfo, checkedAt time.Time) workflowBacklogCLIItem {
	if info == nil {
		return workflowBacklogCLIItem{Status: workflowExecutionStatusString(enumspb.WORKFLOW_EXECUTION_STATUS_UNSPECIFIED)}
	}
	item := workflowBacklogCLIItem{
		WorkflowID:       info.GetExecution().GetWorkflowId(),
		RunID:            info.GetExecution().GetRunId(),
		WorkflowType:     info.GetType().GetName(),
		Status:           workflowExecutionStatusString(info.GetStatus()),
		TaskQueue:        info.GetTaskQueue(),
		HistoryLength:    info.GetHistoryLength(),
		HistorySizeBytes: info.GetHistorySizeBytes(),
	}
	if ts := info.GetStartTime(); ts != nil {
		startedAt := ts.AsTime().UTC()
		item.StartTime = startedAt.Format(time.RFC3339Nano)
		if age := checkedAt.Sub(startedAt); age > 0 {
			item.AgeSeconds = int64(age / time.Second)
		}
	}
	if ts := info.GetCloseTime(); ts != nil {
		item.CloseTime = ts.AsTime().UTC().Format(time.RFC3339Nano)
	}
	if ts := info.GetExecutionTime(); ts != nil {
		item.ExecutionTime = ts.AsTime().UTC().Format(time.RFC3339Nano)
	}
	if parent := info.GetParentExecution(); parent != nil {
		item.ParentWorkflowID = parent.GetWorkflowId()
		item.ParentRunID = parent.GetRunId()
	}
	if root := info.GetRootExecution(); root != nil {
		item.RootWorkflowID = root.GetWorkflowId()
		item.RootRunID = root.GetRunId()
	}
	return item
}

func workflowExecutionStatusString(status enumspb.WorkflowExecutionStatus) string {
	switch status {
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return "running"
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return "completed"
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
		return "failed"
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return "canceled"
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return "terminated"
	case enumspb.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return "continued_as_new"
	case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return "timed_out"
	default:
		return "unspecified"
	}
}

func runDiagnosisRoomListCLI(
	ctx context.Context,
	logger *slog.Logger,
	getenv getenvFunc,
	args []string,
	stdout io.Writer,
) error {
	cfg, err := parseDiagnosisRoomListCLIArgs(args)
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
	return runDiagnosisRoomListCLIWithDependencies(
		ctx,
		postgresDiagnosisRoomListLoader{factory: repository.NewFactory(entClient)},
		cfg,
		stdout,
	)
}

func parseDiagnosisRoomListCLIArgs(args []string) (diagnosisRoomListCLIConfig, error) {
	cfg := diagnosisRoomListCLIConfig{
		Limit: defaultDiagnosisRoomListCLILimit,
		Queue: "all",
	}
	fs := flag.NewFlagSet(diagnosisRoomListCLICommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&cfg.Limit, "limit", defaultDiagnosisRoomListCLILimit, "maximum diagnosis rooms to return")
	fs.StringVar(&cfg.Queue, "queue", "all", "queue filter: all, attention, ready, active, or closed")
	if err := fs.Parse(args); err != nil {
		return diagnosisRoomListCLIConfig{}, fmt.Errorf("%w\n%s", err, diagnosisRoomListCLIUsage())
	}
	if fs.NArg() != 0 {
		return diagnosisRoomListCLIConfig{}, fmt.Errorf("unexpected positional arguments: %s\n%s", strings.Join(fs.Args(), " "), diagnosisRoomListCLIUsage())
	}
	if cfg.Limit <= 0 || cfg.Limit > maxDiagnosisRoomListCLILimit {
		return diagnosisRoomListCLIConfig{}, fmt.Errorf("--limit must be between 1 and %d (got %d)\n%s", maxDiagnosisRoomListCLILimit, cfg.Limit, diagnosisRoomListCLIUsage())
	}
	cfg.Queue = strings.ToLower(strings.TrimSpace(cfg.Queue))
	if !diagnosisRoomListQueueValid(cfg.Queue) {
		return diagnosisRoomListCLIConfig{}, fmt.Errorf("--queue must be all, attention, ready, active, or closed (got %q)\n%s", cfg.Queue, diagnosisRoomListCLIUsage())
	}
	return cfg, nil
}

func runDiagnosisRoomListCLIWithDependencies(
	ctx context.Context,
	loader diagnosisRoomListLoader,
	cfg diagnosisRoomListCLIConfig,
	stdout io.Writer,
) error {
	if loader == nil {
		return fmt.Errorf("diagnosis room list loader must be configured")
	}
	rooms, err := loader.ListDiagnosisRooms(ctx, cfg.Limit)
	if err != nil {
		return err
	}
	out := diagnosisRoomListCLIOutput{
		CheckedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Request:       diagnosisRoomListCLIRequest{Limit: cfg.Limit, Queue: cfg.Queue},
		CountsByQueue: diagnosisRoomListQueueCounts(),
		Rooms:         make([]diagnosisRoomListCLIOutputRow, 0, len(rooms)),
	}
	for _, room := range rooms {
		row := diagnosisRoomListCLIOutputRowFromRoom(room)
		out.CountsByQueue[row.NextStep.Queue]++
		if cfg.Queue != "all" && row.NextStep.Queue != cfg.Queue {
			continue
		}
		out.Rooms = append(out.Rooms, row)
	}
	out.Returned = len(out.Rooms)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("write diagnosis room list output: %w", err)
	}
	return nil
}

func (l postgresDiagnosisRoomListLoader) ListDiagnosisRooms(ctx context.Context, limit int) ([]diagnosisRoomListRoom, error) {
	if l.factory == nil {
		return nil, fmt.Errorf("diagnosis room list loader: unit of work factory must be configured")
	}
	var out []diagnosisRoomListRoom
	err := l.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		rooms, err := uow.Diagnosis().ListChatSessions(ctx, limit)
		if err != nil {
			return err
		}
		out = make([]diagnosisRoomListRoom, len(rooms))
		for i, room := range rooms {
			conclusion, err := latestDiagnosisRoomListConclusion(ctx, uow.Diagnosis(), room.Task.ID)
			if err != nil {
				return err
			}
			notification, err := latestDiagnosisRoomListNotification(ctx, uow.Diagnosis(), room.Task.ID)
			if err != nil {
				return err
			}
			out[i] = diagnosisRoomListRoom{
				Room:               room,
				LatestConclusion:   conclusion,
				LatestNotification: notification,
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list diagnosis rooms: %w", err)
	}
	return out, nil
}

func latestDiagnosisRoomListConclusion(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) (*diagnosisRoomListCLIConclusion, error) {
	events, err := repo.ListEventsByTaskAndKind(ctx, taskID, "diagnosis_room.final_conclusion_ready", 1)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	conclusion, err := diagnosisRoomListConclusionFromEvent(events[0])
	if err != nil {
		return nil, err
	}
	return &conclusion, nil
}

func latestDiagnosisRoomListNotification(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) (*diagnosisRoomListCLINotification, error) {
	kinds := []string{
		"diagnosis_room.assistant_turn_notification_sent",
		"diagnosis_room.final_ready_notification_sent",
		"diagnosis_room.close_notification_sent",
	}
	var latest *domain.DiagnosisTaskEvent
	for _, kind := range kinds {
		events, err := repo.ListEventsByTaskAndKind(ctx, taskID, kind, 1)
		if err != nil {
			return nil, err
		}
		if len(events) == 0 {
			continue
		}
		event := events[0]
		if latest == nil ||
			event.OccurredAt.After(latest.OccurredAt) ||
			(event.OccurredAt.Equal(latest.OccurredAt) && event.ID > latest.ID) {
			latest = &event
		}
	}
	if latest == nil {
		return nil, nil
	}
	notification, err := diagnosisRoomListNotificationFromEvent(*latest)
	if err != nil {
		return nil, err
	}
	return &notification, nil
}

func diagnosisRoomListCLIOutputRowFromRoom(room diagnosisRoomListRoom) diagnosisRoomListCLIOutputRow {
	session := room.Room.Session
	task := room.Room.Task
	row := diagnosisRoomListCLIOutputRow{
		SessionID:          session.SessionKey,
		ChatSessionID:      int64(session.ID),
		DiagnosisTaskID:    int64(task.ID),
		EvidenceSnapshotID: int64(task.EvidenceSnapshotID),
		WorkflowID:         task.WorkflowID,
		RunID:              task.RunID,
		TaskStatus:         string(task.Status),
		RoomStatus:         string(session.Status),
		TurnCount:          session.TurnCount,
		StartedAt:          session.StartedAt.UTC().Format(time.RFC3339Nano),
		LastActivityAt:     session.LastActivityAt.UTC().Format(time.RFC3339Nano),
		CloseReason:        session.CloseReason,
		LatestConclusion:   room.LatestConclusion,
		LatestNotification: room.LatestNotification,
	}
	if session.ClosedAt != nil {
		row.ClosedAt = session.ClosedAt.UTC().Format(time.RFC3339Nano)
	}
	row.NextStep = diagnosisRoomListNextStep(row)
	return row
}

func diagnosisRoomListNextStep(room diagnosisRoomListCLIOutputRow) diagnosisRoomListCLINextStep {
	if room.RoomStatus != string(domain.ChatSessionStatusClosed) &&
		room.LatestNotification != nil &&
		diagnosisRoomListNotificationFailed(room.LatestNotification.ProviderStatus) {
		return diagnosisRoomListCLINextStep{
			Queue:  "attention",
			Label:  "Notification failed",
			Detail: "Review the failed diagnosis-room notification before relying on downstream handoff.",
		}
	}
	if room.TaskStatus == string(domain.DiagnosisStatusFailed) {
		return diagnosisRoomListCLINextStep{
			Queue:  "attention",
			Label:  "Workflow failed",
			Detail: "Inspect the workflow failure and decide whether to restart the diagnosis room.",
		}
	}
	if room.RoomStatus == string(domain.ChatSessionStatusClosed) || room.TaskStatus == string(domain.DiagnosisStatusCancelled) {
		detail := room.CloseReason
		if detail == "" {
			detail = "Diagnosis room is closed."
		}
		return diagnosisRoomListCLINextStep{Queue: "closed", Label: "Closed", Detail: detail}
	}
	if room.LatestConclusion == nil {
		if room.TurnCount > 0 {
			return diagnosisRoomListCLINextStep{
				Queue:  "active",
				Label:  "Continue AI review",
				Detail: "Continue the AI conversation or collect the requested evidence.",
			}
		}
		return diagnosisRoomListCLINextStep{
			Queue:  "active",
			Label:  "Start AI review",
			Detail: "Send the first prompt so AI can produce a diagnosis report.",
		}
	}
	if room.LatestConclusion.RequiresHumanReview != nil && *room.LatestConclusion.RequiresHumanReview {
		return diagnosisRoomListCLINextStep{
			Queue:  "attention",
			Label:  "Human review",
			Detail: "Review the AI conclusion and add verified operator evidence if confidence is not sufficient.",
		}
	}
	switch strings.ToLower(room.LatestConclusion.Confidence) {
	case "low", "medium":
		return diagnosisRoomListCLINextStep{
			Queue:  "attention",
			Label:  "Improve confidence",
			Detail: "Collect more evidence before final confirmation.",
		}
	default:
		return diagnosisRoomListCLINextStep{
			Queue:  "ready",
			Label:  "Review conclusion",
			Detail: "AI produced a conclusion. Review it before closing the room.",
		}
	}
}

func diagnosisRoomListConclusionFromEvent(event domain.DiagnosisTaskEvent) (diagnosisRoomListCLIConclusion, error) {
	var payload struct {
		Kind            string `json:"kind"`
		FinalConclusion struct {
			Status              string `json:"status"`
			Confidence          string `json:"confidence"`
			RequiresHumanReview *bool  `json:"requires_human_review"`
		} `json:"final_conclusion"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return diagnosisRoomListCLIConclusion{}, fmt.Errorf("parse diagnosis conclusion event %d: %w", event.ID, err)
	}
	eventKind := strings.TrimSpace(payload.Kind)
	if eventKind == "" {
		eventKind = event.Kind
	}
	return diagnosisRoomListCLIConclusion{
		EventKind:           eventKind,
		Status:              strings.TrimSpace(payload.FinalConclusion.Status),
		Confidence:          strings.TrimSpace(payload.FinalConclusion.Confidence),
		RequiresHumanReview: payload.FinalConclusion.RequiresHumanReview,
		OccurredAt:          event.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func diagnosisRoomListNotificationFromEvent(event domain.DiagnosisTaskEvent) (diagnosisRoomListCLINotification, error) {
	var payload struct {
		Kind           string `json:"kind"`
		ProviderStatus string `json:"provider_status"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return diagnosisRoomListCLINotification{}, fmt.Errorf("parse diagnosis notification event %d: %w", event.ID, err)
	}
	eventKind := strings.TrimSpace(payload.Kind)
	if eventKind == "" {
		eventKind = event.Kind
	}
	return diagnosisRoomListCLINotification{
		EventKind:      eventKind,
		ProviderStatus: strings.TrimSpace(payload.ProviderStatus),
		OccurredAt:     event.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func diagnosisRoomListQueueCounts() map[string]int {
	return map[string]int{
		"attention": 0,
		"ready":     0,
		"active":    0,
		"closed":    0,
	}
}

func diagnosisRoomListQueueValid(queue string) bool {
	switch queue {
	case "all", "attention", "ready", "active", "closed":
		return true
	default:
		return false
	}
}

func diagnosisRoomListNotificationFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error":
		return true
	default:
		return false
	}
}

func runDiagnosisRoomCloseCLI(
	ctx context.Context,
	logger *slog.Logger,
	getenv getenvFunc,
	args []string,
	stdout io.Writer,
) error {
	cfg, err := parseDiagnosisRoomCloseCLIArgs(args)
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

	temporalAddr := temporalHostPortFrom(getenv)
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

	return runDiagnosisRoomCloseCLIWithDependencies(
		ctx,
		temporalDiagnosisRoomCloseWaiter{client: tc},
		postgresDiagnosisRoomCloseEventsLoader{factory: repository.NewFactory(entClient)},
		cfg,
		stdout,
	)
}

func parseDiagnosisRoomCloseCLIArgs(args []string) (diagnosisRoomCloseCLIConfig, error) {
	cfg := diagnosisRoomCloseCLIConfig{
		Reason:      defaultDiagnosisRoomCloseReason,
		WaitTimeout: defaultDiagnosisRoomCloseWait,
	}
	fs := flag.NewFlagSet(diagnosisRoomCloseCLICommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.SessionID, "session-id", "", "diagnosis room session id")
	fs.StringVar(&cfg.RunID, "run-id", "", "optional Temporal run id")
	fs.StringVar(&cfg.Reason, "reason", defaultDiagnosisRoomCloseReason, "close reason")
	fs.DurationVar(&cfg.WaitTimeout, "wait-timeout", defaultDiagnosisRoomCloseWait, "maximum duration to wait for workflow close")
	if err := fs.Parse(args); err != nil {
		return diagnosisRoomCloseCLIConfig{}, fmt.Errorf("%w\n%s", err, diagnosisRoomCloseCLIUsage())
	}
	if fs.NArg() != 0 {
		return diagnosisRoomCloseCLIConfig{}, fmt.Errorf("unexpected positional arguments: %s\n%s", strings.Join(fs.Args(), " "), diagnosisRoomCloseCLIUsage())
	}
	if strings.TrimSpace(cfg.SessionID) == "" {
		return diagnosisRoomCloseCLIConfig{}, fmt.Errorf("--session-id is required\n%s", diagnosisRoomCloseCLIUsage())
	}
	if strings.TrimSpace(cfg.SessionID) != cfg.SessionID {
		return diagnosisRoomCloseCLIConfig{}, fmt.Errorf("--session-id must not contain leading or trailing whitespace\n%s", diagnosisRoomCloseCLIUsage())
	}
	cfg.RunID = strings.TrimSpace(cfg.RunID)
	if strings.TrimSpace(cfg.Reason) == "" {
		return diagnosisRoomCloseCLIConfig{}, fmt.Errorf("--reason must be non-empty\n%s", diagnosisRoomCloseCLIUsage())
	}
	if strings.TrimSpace(cfg.Reason) != cfg.Reason {
		return diagnosisRoomCloseCLIConfig{}, fmt.Errorf("--reason must not contain leading or trailing whitespace\n%s", diagnosisRoomCloseCLIUsage())
	}
	if cfg.WaitTimeout <= 0 {
		return diagnosisRoomCloseCLIConfig{}, fmt.Errorf("--wait-timeout must be > 0 (got %s)\n%s", cfg.WaitTimeout, diagnosisRoomCloseCLIUsage())
	}
	return cfg, nil
}

func runDiagnosisRoomCloseCLIWithDependencies(
	ctx context.Context,
	waiter diagnosisRoomCloseWorkflowWaiter,
	loader diagnosisRoomCloseEventsLoader,
	cfg diagnosisRoomCloseCLIConfig,
	stdout io.Writer,
) error {
	if waiter == nil {
		return fmt.Errorf("diagnosis room close workflow waiter must be configured")
	}
	if loader == nil {
		return fmt.Errorf("diagnosis room close event loader must be configured")
	}
	workflowResult, err := waiter.SignalAndWaitDiagnosisRoomClose(ctx, cfg)
	if err != nil {
		return err
	}
	events, err := loader.LoadDiagnosisRoomCloseEvents(ctx, domain.DiagnosisTaskID(workflowResult.DiagnosisTaskID))
	if err != nil {
		return err
	}
	out, err := diagnosisRoomCloseCLIOutputFromResult(cfg, workflowResult, events)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("write diagnosis room close output: %w", err)
	}
	return nil
}

func (w temporalDiagnosisRoomCloseWaiter) SignalAndWaitDiagnosisRoomClose(ctx context.Context, cfg diagnosisRoomCloseCLIConfig) (temporalpkg.DiagnosisRoomWorkflowResult, error) {
	if w.client == nil {
		return temporalpkg.DiagnosisRoomWorkflowResult{}, fmt.Errorf("diagnosis room close waiter: Temporal client must be non-nil")
	}
	workflowID, err := temporalpkg.DiagnosisRoomWorkflowID(cfg.SessionID)
	if err != nil {
		return temporalpkg.DiagnosisRoomWorkflowResult{}, err
	}
	if err := w.client.SignalWorkflow(
		ctx,
		workflowID,
		cfg.RunID,
		temporalpkg.DiagnosisRoomCloseSignal,
		temporalpkg.DiagnosisRoomCloseRequest{Reason: cfg.Reason},
	); err != nil {
		return temporalpkg.DiagnosisRoomWorkflowResult{}, fmt.Errorf("signal diagnosis room close: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, cfg.WaitTimeout)
	defer cancel()
	var result temporalpkg.DiagnosisRoomWorkflowResult
	if err := w.client.GetWorkflow(waitCtx, workflowID, cfg.RunID).Get(waitCtx, &result); err != nil {
		return temporalpkg.DiagnosisRoomWorkflowResult{}, fmt.Errorf("wait diagnosis room close: %w", err)
	}
	return result, nil
}

func (l postgresDiagnosisRoomCloseEventsLoader) LoadDiagnosisRoomCloseEvents(ctx context.Context, taskID domain.DiagnosisTaskID) (diagnosisRoomCloseEvents, error) {
	if l.factory == nil {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("diagnosis room close event loader: unit of work factory must be configured")
	}
	if taskID == 0 {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("diagnosis room close event loader: diagnosis_task_id must be non-zero")
	}
	var out diagnosisRoomCloseEvents
	err := l.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, taskID, 1000)
		if err != nil {
			return err
		}
		for _, event := range events {
			switch event.Kind {
			case diagnosisRoomCloseEventClosedKind:
				out.CloseEvent = event
			case diagnosisRoomCloseEventNotificationSentKind:
				out.NotificationEvent = event
			}
		}
		return nil
	})
	if err != nil {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("load diagnosis room close events: %w", err)
	}
	if out.CloseEvent.ID == 0 {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("diagnosis room close event is missing for task %d", taskID)
	}
	if out.NotificationEvent.ID == 0 {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("diagnosis room close notification event is missing for task %d", taskID)
	}
	if err := strictjson.Unmarshal(out.CloseEvent.Payload, &out.ClosePayload); err != nil {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("decode diagnosis room close event payload: %w", err)
	}
	if out.ClosePayload.ConclusionVersion != "diagnosis-room-close.v1" {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("diagnosis room close event conclusion_version = %q, want diagnosis-room-close.v1", out.ClosePayload.ConclusionVersion)
	}
	if strings.TrimSpace(out.ClosePayload.FinalConclusion.Status) == "" {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("diagnosis room close event missing final_conclusion.status")
	}
	if err := strictjson.Unmarshal(out.NotificationEvent.Payload, &out.Notification); err != nil {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("decode diagnosis room close notification event payload: %w", err)
	}
	if strings.TrimSpace(out.Notification.IdempotencyKey) == "" {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("diagnosis room close notification event missing idempotency_key")
	}
	if strings.TrimSpace(out.Notification.ProviderStatus) == "" {
		return diagnosisRoomCloseEvents{}, fmt.Errorf("diagnosis room close notification event missing provider_status")
	}
	return out, nil
}

func diagnosisRoomCloseCLIOutputFromResult(
	cfg diagnosisRoomCloseCLIConfig,
	result temporalpkg.DiagnosisRoomWorkflowResult,
	events diagnosisRoomCloseEvents,
) (diagnosisRoomCloseCLIOutput, error) {
	workflowID, err := temporalpkg.DiagnosisRoomWorkflowID(cfg.SessionID)
	if err != nil {
		return diagnosisRoomCloseCLIOutput{}, err
	}
	if result.Status != "closed" {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room status = %q, want closed", result.Status)
	}
	if result.SessionID != cfg.SessionID {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close result session_id = %q, want %q", result.SessionID, cfg.SessionID)
	}
	if result.DiagnosisTaskID <= 0 {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close result missing diagnosis_task_id")
	}
	if result.ChatSessionID <= 0 {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close result missing chat_session_id")
	}
	if result.TurnCount < 0 {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close result turn_count must be >= 0")
	}
	if result.ClosedAt == nil || result.ClosedAt.IsZero() {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close result missing closed_at")
	}
	if result.CloseReason != cfg.Reason {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close reason = %q, want %q", result.CloseReason, cfg.Reason)
	}
	if result.FinalConclusion == nil {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close result missing final_conclusion")
	}
	if events.CloseEvent.ID == 0 {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close event is missing")
	}
	if events.CloseEvent.TaskID != domain.DiagnosisTaskID(result.DiagnosisTaskID) {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close event task_id = %d, want %d", events.CloseEvent.TaskID, result.DiagnosisTaskID)
	}
	if events.CloseEvent.Kind != diagnosisRoomCloseEventClosedKind {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close event kind = %q, want %s", events.CloseEvent.Kind, diagnosisRoomCloseEventClosedKind)
	}
	if err := validateDiagnosisRoomClosePayload(result, events.ClosePayload); err != nil {
		return diagnosisRoomCloseCLIOutput{}, err
	}
	if events.NotificationEvent.ID == 0 {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close notification event is missing")
	}
	if events.NotificationEvent.TaskID != domain.DiagnosisTaskID(result.DiagnosisTaskID) {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close notification event task_id = %d, want %d", events.NotificationEvent.TaskID, result.DiagnosisTaskID)
	}
	if events.NotificationEvent.Kind != diagnosisRoomCloseEventNotificationSentKind {
		return diagnosisRoomCloseCLIOutput{}, fmt.Errorf("diagnosis room close notification event kind = %q, want %s", events.NotificationEvent.Kind, diagnosisRoomCloseEventNotificationSentKind)
	}
	return diagnosisRoomCloseCLIOutput{
		CheckedAt: diagnosisRoomCloseCLINowUTC().Format(time.RFC3339Nano),
		Request: diagnosisRoomCloseCLIRequest{
			SessionID:   cfg.SessionID,
			WorkflowID:  workflowID,
			RunID:       cfg.RunID,
			Reason:      cfg.Reason,
			WaitTimeout: cfg.WaitTimeout.String(),
		},
		Signaled: true,
		Workflow: diagnosisRoomCloseCLIWorkflow{
			SessionID:       result.SessionID,
			ChatSessionID:   result.ChatSessionID,
			DiagnosisTaskID: result.DiagnosisTaskID,
			Status:          result.Status,
			TurnCount:       result.TurnCount,
			ClosedAt:        retainedProofTime(*result.ClosedAt).Format(time.RFC3339Nano),
			CloseReason:     result.CloseReason,
			FinalConclusion: diagnosisRoomCloseCLIFinalConclusionFromTemporal(*result.FinalConclusion),
		},
		CloseEvent: diagnosisRoomCloseCLIEvent{
			ID:                int64(events.CloseEvent.ID),
			Kind:              events.CloseEvent.Kind,
			OccurredAt:        events.CloseEvent.OccurredAt.UTC().Format(time.RFC3339Nano),
			ConclusionVersion: events.ClosePayload.ConclusionVersion,
			FinalConclusion:   diagnosisRoomCloseCLIFinalConclusionFromTemporal(events.ClosePayload.FinalConclusion),
		},
		NotificationEvent: diagnosisRoomCloseCLINotificationEvent{
			ID:                int64(events.NotificationEvent.ID),
			Kind:              events.NotificationEvent.Kind,
			OccurredAt:        events.NotificationEvent.OccurredAt.UTC().Format(time.RFC3339Nano),
			IdempotencyKey:    events.Notification.IdempotencyKey,
			ProviderMessageID: events.Notification.ProviderMessageID,
			ProviderStatus:    events.Notification.ProviderStatus,
		},
	}, nil
}

func validateDiagnosisRoomClosePayload(
	result temporalpkg.DiagnosisRoomWorkflowResult,
	payload diagnosisRoomCloseEventPayload,
) error {
	if payload.Kind != diagnosisRoomCloseEventClosedKind {
		return fmt.Errorf("diagnosis room close event payload kind = %q, want %s", payload.Kind, diagnosisRoomCloseEventClosedKind)
	}
	if payload.SessionID != result.SessionID {
		return fmt.Errorf("diagnosis room close event payload session_id = %q, want %q", payload.SessionID, result.SessionID)
	}
	if payload.ChatSessionID != result.ChatSessionID {
		return fmt.Errorf("diagnosis room close event payload chat_session_id = %d, want %d", payload.ChatSessionID, result.ChatSessionID)
	}
	if payload.DiagnosisTaskID != result.DiagnosisTaskID {
		return fmt.Errorf("diagnosis room close event payload diagnosis_task_id = %d, want %d", payload.DiagnosisTaskID, result.DiagnosisTaskID)
	}
	if payload.Status != result.Status {
		return fmt.Errorf("diagnosis room close event payload status = %q, want %q", payload.Status, result.Status)
	}
	if payload.TurnCount != result.TurnCount {
		return fmt.Errorf("diagnosis room close event payload turn_count = %d, want %d", payload.TurnCount, result.TurnCount)
	}
	if payload.CloseReason != result.CloseReason {
		return fmt.Errorf("diagnosis room close event payload close_reason = %q, want %q", payload.CloseReason, result.CloseReason)
	}
	if result.ClosedAt == nil || result.ClosedAt.IsZero() {
		return fmt.Errorf("diagnosis room close result missing closed_at")
	}
	if !sameRetainedProofTime(payload.ClosedAt, *result.ClosedAt) {
		return fmt.Errorf("diagnosis room close event payload closed_at = %s, want %s",
			retainedProofTime(payload.ClosedAt).Format(time.RFC3339Nano),
			retainedProofTime(*result.ClosedAt).Format(time.RFC3339Nano))
	}
	if result.FinalConclusion == nil {
		return fmt.Errorf("diagnosis room close result missing final_conclusion")
	}
	return compareDiagnosisRoomFinalConclusion(*result.FinalConclusion, payload.FinalConclusion)
}

func compareDiagnosisRoomFinalConclusion(
	result temporalpkg.DiagnosisRoomFinalConclusion,
	payload temporalpkg.DiagnosisRoomFinalConclusion,
) error {
	if strings.TrimSpace(result.Status) == "" {
		return fmt.Errorf("diagnosis room close result final_conclusion.status must be non-empty")
	}
	if payload.Status != result.Status {
		return fmt.Errorf("diagnosis room close event final_conclusion.status = %q, want %q", payload.Status, result.Status)
	}
	if payload.Source != result.Source {
		return fmt.Errorf("diagnosis room close event final_conclusion.source = %q, want %q", payload.Source, result.Source)
	}
	if payload.Reason != result.Reason {
		return fmt.Errorf("diagnosis room close event final_conclusion.reason = %q, want %q", payload.Reason, result.Reason)
	}
	if payload.AssistantTurnID != result.AssistantTurnID {
		return fmt.Errorf("diagnosis room close event final_conclusion.assistant_turn_id = %d, want %d", payload.AssistantTurnID, result.AssistantTurnID)
	}
	if payload.AssistantMessageID != result.AssistantMessageID {
		return fmt.Errorf("diagnosis room close event final_conclusion.assistant_message_id = %q, want %q", payload.AssistantMessageID, result.AssistantMessageID)
	}
	if payload.AssistantSequence != result.AssistantSequence {
		return fmt.Errorf("diagnosis room close event final_conclusion.assistant_sequence = %d, want %d", payload.AssistantSequence, result.AssistantSequence)
	}
	if !sameOptionalTime(payload.AssistantOccurredAt, result.AssistantOccurredAt) {
		return fmt.Errorf("diagnosis room close event final_conclusion.assistant_occurred_at does not match workflow result")
	}
	if payload.EvidenceSnapshotID != result.EvidenceSnapshotID {
		return fmt.Errorf("diagnosis room close event final_conclusion.evidence_snapshot_id = %d, want %d", payload.EvidenceSnapshotID, result.EvidenceSnapshotID)
	}
	if payload.ConclusionVersion != result.ConclusionVersion {
		return fmt.Errorf("diagnosis room close event final_conclusion.conclusion_version = %q, want %q", payload.ConclusionVersion, result.ConclusionVersion)
	}
	if !sameOptionalTime(payload.RecordedAt, result.RecordedAt) {
		return fmt.Errorf("diagnosis room close event final_conclusion.recorded_at does not match workflow result")
	}
	if payload.ConfirmedBy != result.ConfirmedBy {
		return fmt.Errorf("diagnosis room close event final_conclusion.confirmed_by = %q, want %q", payload.ConfirmedBy, result.ConfirmedBy)
	}
	if !sameStrings(payload.SupplementalContextRefs, result.SupplementalContextRefs) {
		return fmt.Errorf("diagnosis room close event final_conclusion.supplemental_context_refs does not match workflow result")
	}
	if payload.Content != result.Content {
		return fmt.Errorf("diagnosis room close event final_conclusion.content does not match workflow result")
	}
	if payload.Confidence != result.Confidence {
		return fmt.Errorf("diagnosis room close event final_conclusion.confidence = %q, want %q", payload.Confidence, result.Confidence)
	}
	if !sameOptionalBool(payload.RequiresHumanReview, result.RequiresHumanReview) {
		return fmt.Errorf("diagnosis room close event final_conclusion.requires_human_review does not match workflow result")
	}
	return nil
}

func diagnosisRoomCloseCLIFinalConclusionFromTemporal(
	in temporalpkg.DiagnosisRoomFinalConclusion,
) diagnosisRoomCloseCLIFinalConclusion {
	out := diagnosisRoomCloseCLIFinalConclusion{
		Status:                  in.Status,
		Source:                  in.Source,
		Reason:                  in.Reason,
		EvidenceSnapshotID:      in.EvidenceSnapshotID,
		ConclusionVersion:       in.ConclusionVersion,
		ConfirmedBy:             in.ConfirmedBy,
		SupplementalContextRefs: append([]string(nil), in.SupplementalContextRefs...),
		AssistantTurnID:         in.AssistantTurnID,
		AssistantMessageID:      in.AssistantMessageID,
		AssistantSequence:       in.AssistantSequence,
		Content:                 in.Content,
		Confidence:              in.Confidence,
	}
	if in.RecordedAt != nil {
		out.RecordedAt = retainedProofTime(*in.RecordedAt).Format(time.RFC3339Nano)
	}
	if in.AssistantOccurredAt != nil {
		out.AssistantOccurredAt = retainedProofTime(*in.AssistantOccurredAt).Format(time.RFC3339Nano)
	}
	if in.RequiresHumanReview != nil {
		requiresHumanReview := *in.RequiresHumanReview
		out.RequiresHumanReview = &requiresHumanReview
	}
	return out
}

func sameOptionalTime(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return sameRetainedProofTime(*a, *b)
	}
}

func retainedProofTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}

func sameRetainedProofTime(a, b time.Time) bool {
	return retainedProofTime(a).Equal(retainedProofTime(b))
}

func sameOptionalBool(a, b *bool) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func diagnosisRoomCloseCLIUsage() string {
	return "usage: openclarion " + diagnosisRoomCloseCLICommand +
		" --session-id " + strconv.Quote("diagnosis-session-...") +
		" [--run-id run] [--reason live_smoke_completed] [--wait-timeout 2m]"
}

func workflowBacklogCLIUsage() string {
	return "usage: openclarion " + workflowBacklogCLICommand +
		" [--limit 50] [--status running|all] [--workflow-types ReportBatchWorkflow,DiagnosisRoomWorkflow] [--all-types] [--query " +
		strconv.Quote(`ExecutionStatus = "Running"`) + "]"
}

func diagnosisRoomListCLIUsage() string {
	return "usage: openclarion " + diagnosisRoomListCLICommand +
		" [--limit 100] [--queue all|attention|ready|active|closed]"
}

func temporalHostPortFrom(getenv getenvFunc) string {
	if v := strings.TrimSpace(getenv("TEMPORAL_HOST_PORT")); v != "" {
		return v
	}
	return "localhost:7233"
}

func reportReplayCLIUsage() string {
	return "usage: openclarion " + reportReplayCLICommand +
		" --window-start " + strconv.Quote("2026-05-28T10:00:00Z") +
		" --window-end " + strconv.Quote("2026-05-28T11:00:00Z") +
		" [--limit 10000] [--correlation-key key] [--workflow-id id] [--scenario single_alert|cascade|alert_storm] [--wait] [--wait-timeout 20m]"
}

func reportPolicyReplayCLIUsage() string {
	return "usage: openclarion " + reportPolicyReplayCLICommand +
		" --policy-id 123" +
		" --window-start " + strconv.Quote("2026-05-28T10:00:00Z") +
		" --window-end " + strconv.Quote("2026-05-28T11:00:00Z") +
		" [--limit 10000] [--correlation-key key] [--workflow-id id] [--wait] [--wait-timeout 20m]"
}

func reportScheduleLiveSmokeCLIUsage() string {
	return "usage: openclarion " + reportScheduleLiveSmokeCLICommand +
		" --schedule-id 123 --policy-id 456" +
		" [--temporal-schedule-id openclarion-report-policy-456-daily]" +
		" [--observed-after " + strconv.Quote("2026-06-06T00:00:00Z") + "]" +
		" [--wait-timeout 30m]"
}
