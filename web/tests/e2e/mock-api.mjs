import { createHash } from "node:crypto";
import { createServer } from "node:http";

const port = Number.parseInt(process.env.OPENCLARION_MOCK_API_PORT ?? "18080", 10);
if (!Number.isSafeInteger(port) || port <= 0) {
  throw new Error(`Invalid OPENCLARION_MOCK_API_PORT: ${process.env.OPENCLARION_MOCK_API_PORT}`);
}

const report = {
  id: 101,
  correlation_key: "window:checkout-latency",
  title: "Checkout latency incident",
  executive_summary: "Checkout latency increased after the payment deployment.",
  severity: "warning",
  confidence: "high",
  sub_reports: [
    {
      title: "Checkout API latency",
      severity: "warning",
      summary: "p95 latency exceeded the warning threshold."
    }
  ],
  recommended_actions: [
    {
      label: "Inspect deployment",
      detail: "Compare checkout deployment timestamps with latency onset.",
      priority: "high"
    }
  ],
  notification_text: "Checkout latency incident requires review.",
  content: {
    title: "Checkout latency incident"
  },
  model: "gpt-4.1-mini",
  output_mode: "json_schema",
  created_by_workflow: "FinalReportWorkflow",
  created_at: "2026-05-28T06:00:00Z",
  linked_sub_reports: [
    {
      id: 501,
      evidence_snapshot_id: 9001,
      scenario: "single_alert",
      title: "Checkout API latency",
      summary: "p95 latency exceeded the warning threshold.",
      severity: "warning",
      confidence: "high",
      findings: [
        {
          label: "High p95 latency",
          detail: "p95 latency stayed above the warning threshold.",
          evidence_id: "alert:checkout-latency"
        }
      ],
      recommended_actions: [
        {
          label: "Inspect deployment",
          detail: "Compare checkout deployment timestamps with latency onset.",
          priority: "high"
        }
      ],
      evidence_refs: ["alert:checkout-latency"],
      content: {
        title: "Checkout API latency"
      },
      model: "gpt-4.1-mini",
      output_mode: "json_schema",
      created_by_workflow: "ReportFanOutWorkflow",
      created_at: "2026-05-28T05:59:00Z"
    }
  ]
};

const dashboard = {
  generated_at: "2026-05-28T06:01:00Z",
  alerts: {
    total_recent: 24,
    firing: 7,
    resolved: 17
  },
  reports: {
    total_recent: 12,
    delivered: 11,
    failed: 1,
    pending: 0,
    missing_delivery: 0,
    success_rate: 0.9167,
    severity: {
      info: 3,
      warning: 8,
      critical: 1
    }
  }
};

let nextAlertSourceID = 3;
const alertSources = [
  {
    id: 1,
    name: "Primary Prometheus",
    kind: "prometheus",
    base_url: "https://prometheus.example.test",
    auth_mode: "bearer",
    secret_ref: "secret/openclarion/prometheus-bearer",
    enabled: true,
    labels: {
      env: "prod",
      owner: "platform"
    },
    created_at: "2026-05-28T06:00:00Z",
    updated_at: "2026-05-28T06:00:00Z"
  },
  {
    id: 2,
    name: "Staging Alertmanager",
    kind: "alertmanager",
    base_url: "https://alertmanager-staging.example.test",
    auth_mode: "none",
    secret_ref: "",
    enabled: false,
    labels: {
      env: "staging"
    },
    created_at: "2026-05-28T06:00:00Z",
    updated_at: "2026-05-28T06:00:00Z"
  }
];

let nextGroupingPolicyID = 2;
const groupingPolicies = [
  {
    id: 1,
    name: "Default alert grouping",
    dimension_keys: ["alertname", "service"],
    severity_key: "severity",
    source_filter: ["prometheus"],
    enabled: true,
    created_at: "2026-06-05T04:00:00Z",
    updated_at: "2026-06-05T04:00:00Z"
  }
];

let nextReportWorkflowPolicyID = 2;
const reportWorkflowPolicies = [
  {
    id: 1,
    name: "Default report workflow",
    alert_source_profile_id: 1,
    grouping_policy_id: 1,
    report_notification_channel_profile_id: null,
    trigger_mode: "manual_replay",
    report_scenario: "single_alert",
    diagnosis_follow_up: "suggest_room",
    enabled: true,
    enabled_at: "2026-06-05T08:05:00Z",
    disabled_at: null,
    created_at: "2026-06-05T08:00:00Z",
    updated_at: "2026-06-05T08:05:00Z"
  }
];

let nextReportWorkflowScheduleID = 2;
const reportWorkflowSchedules = [
  {
    id: 1,
    name: "Daily report window",
    report_workflow_policy_id: 1,
    temporal_schedule_id: "openclarion-report-policy-1-daily",
    interval_seconds: 86400,
    offset_seconds: 21600,
    replay_window_seconds: 3600,
    replay_delay_seconds: 300,
    replay_limit: 10000,
    catchup_window_seconds: 3600,
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: "2026-06-06T02:00:00Z",
    updated_at: "2026-06-06T02:00:00Z"
  }
];

let nextDiagnosisToolTemplateID = 2;
const diagnosisToolTemplates = [
  {
    id: 1,
    name: "CPU saturation range",
    alert_source_profile_id: 1,
    tool: "metric_range_query",
    query_template: "rate(container_cpu_usage_seconds_total[5m])",
    default_limit: 5,
    default_window_seconds: 3600,
    max_window_seconds: 21600,
    default_step_seconds: 60,
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: "2026-06-08T08:00:00Z",
    updated_at: "2026-06-08T08:00:00Z"
  }
];

let nextNotificationChannelID = 2;
const notificationChannels = [
  {
    id: 1,
    name: "Operations webhook",
    kind: "webhook",
    secret_ref: "secret/example/ops-webhook",
    delivery_scopes: ["report"],
    enabled: false,
    labels: {
      team: "ops"
    },
    created_at: "2026-06-05T09:00:00Z",
    updated_at: "2026-06-05T09:00:00Z"
  }
];

const server = createServer((request, response) => {
  const url = new URL(request.url ?? "/", `http://127.0.0.1:${port}`);
  if (request.method === "OPTIONS") {
    writeJSON(response, 204, null);
    return;
  }

  if (url.pathname === "/api/v1/config/alert-sources") {
    handleAlertSourceCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/grouping-policies") {
    handleGroupingPolicyCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/report-workflow-policies") {
    handleReportWorkflowPolicyCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/report-workflow-schedules") {
    handleReportWorkflowScheduleCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/diagnosis-tool-templates") {
    handleDiagnosisToolTemplateCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/notification-channels") {
    handleNotificationChannelCollection(request, response);
    return;
  }
  const alertSourceTestMatch = url.pathname.match(/^\/api\/v1\/config\/alert-sources\/(\d+)\/test$/);
  if (alertSourceTestMatch) {
    handleAlertSourceConnectionTest(request, response, Number.parseInt(alertSourceTestMatch[1], 10));
    return;
  }
  const alertSourceMatch = url.pathname.match(/^\/api\/v1\/config\/alert-sources\/(\d+)$/);
  if (alertSourceMatch) {
    handleAlertSourceProfile(request, response, Number.parseInt(alertSourceMatch[1], 10));
    return;
  }
  const groupingPolicyPreviewMatch = url.pathname.match(/^\/api\/v1\/config\/grouping-policies\/(\d+)\/preview$/);
  if (groupingPolicyPreviewMatch) {
    handleGroupingPolicyPreview(request, response, Number.parseInt(groupingPolicyPreviewMatch[1], 10));
    return;
  }
  const groupingPolicyMatch = url.pathname.match(/^\/api\/v1\/config\/grouping-policies\/(\d+)$/);
  if (groupingPolicyMatch) {
    handleGroupingPolicy(request, response, Number.parseInt(groupingPolicyMatch[1], 10));
    return;
  }
  const reportWorkflowPolicyEnableMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)\/enable$/
  );
  if (reportWorkflowPolicyEnableMatch) {
    handleReportWorkflowPolicyEnablement(request, response, Number.parseInt(reportWorkflowPolicyEnableMatch[1], 10), true);
    return;
  }
  const reportWorkflowPolicyDisableMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)\/disable$/
  );
  if (reportWorkflowPolicyDisableMatch) {
    handleReportWorkflowPolicyEnablement(
      request,
      response,
      Number.parseInt(reportWorkflowPolicyDisableMatch[1], 10),
      false
    );
    return;
  }
  const reportWorkflowPolicyImpactPreviewMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)\/impact-preview$/
  );
  if (reportWorkflowPolicyImpactPreviewMatch) {
    handleReportWorkflowPolicyImpactPreview(
      request,
      response,
      Number.parseInt(reportWorkflowPolicyImpactPreviewMatch[1], 10)
    );
    return;
  }
  const reportWorkflowPolicyReplayMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)\/replay-window$/
  );
  if (reportWorkflowPolicyReplayMatch) {
    handleReportWorkflowPolicyReplay(request, response, Number.parseInt(reportWorkflowPolicyReplayMatch[1], 10));
    return;
  }
  const reportWorkflowPolicyMatch = url.pathname.match(/^\/api\/v1\/config\/report-workflow-policies\/(\d+)$/);
  if (reportWorkflowPolicyMatch) {
    handleReportWorkflowPolicy(request, response, Number.parseInt(reportWorkflowPolicyMatch[1], 10));
    return;
  }
  const reportWorkflowScheduleEnableMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-schedules\/(\d+)\/enable$/
  );
  if (reportWorkflowScheduleEnableMatch) {
    handleReportWorkflowScheduleEnablement(
      request,
      response,
      Number.parseInt(reportWorkflowScheduleEnableMatch[1], 10),
      true
    );
    return;
  }
  const reportWorkflowScheduleDisableMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-schedules\/(\d+)\/disable$/
  );
  if (reportWorkflowScheduleDisableMatch) {
    handleReportWorkflowScheduleEnablement(
      request,
      response,
      Number.parseInt(reportWorkflowScheduleDisableMatch[1], 10),
      false
    );
    return;
  }
  const reportWorkflowScheduleMatch = url.pathname.match(/^\/api\/v1\/config\/report-workflow-schedules\/(\d+)$/);
  if (reportWorkflowScheduleMatch) {
    handleReportWorkflowSchedule(request, response, Number.parseInt(reportWorkflowScheduleMatch[1], 10));
    return;
  }
  const diagnosisToolTemplateEnableMatch = url.pathname.match(
    /^\/api\/v1\/config\/diagnosis-tool-templates\/(\d+)\/enable$/
  );
  if (diagnosisToolTemplateEnableMatch) {
    handleDiagnosisToolTemplateEnablement(
      request,
      response,
      Number.parseInt(diagnosisToolTemplateEnableMatch[1], 10),
      true
    );
    return;
  }
  const diagnosisToolTemplateDisableMatch = url.pathname.match(
    /^\/api\/v1\/config\/diagnosis-tool-templates\/(\d+)\/disable$/
  );
  if (diagnosisToolTemplateDisableMatch) {
    handleDiagnosisToolTemplateEnablement(
      request,
      response,
      Number.parseInt(diagnosisToolTemplateDisableMatch[1], 10),
      false
    );
    return;
  }
  const diagnosisToolTemplateMatch = url.pathname.match(/^\/api\/v1\/config\/diagnosis-tool-templates\/(\d+)$/);
  if (diagnosisToolTemplateMatch) {
    handleDiagnosisToolTemplate(request, response, Number.parseInt(diagnosisToolTemplateMatch[1], 10));
    return;
  }
  const notificationChannelTestMatch = url.pathname.match(/^\/api\/v1\/config\/notification-channels\/(\d+)\/test$/);
  if (notificationChannelTestMatch) {
    handleNotificationChannelTest(request, response, Number.parseInt(notificationChannelTestMatch[1], 10));
    return;
  }
  const notificationChannelMatch = url.pathname.match(/^\/api\/v1\/config\/notification-channels\/(\d+)$/);
  if (notificationChannelMatch) {
    handleNotificationChannel(request, response, Number.parseInt(notificationChannelMatch[1], 10));
    return;
  }

  switch (url.pathname) {
    case "/healthz":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, { status: "ok" });
      return;
    case "/api/v1/dashboard":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, dashboard);
      return;
    case "/api/v1/reports":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, { items: [report] });
      return;
    case "/api/v1/reports/101":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, report);
      return;
    case "/api/v1/diagnosis/ws-ticket":
      if (request.method !== "POST") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      if (!request.headers.authorization?.startsWith("Bearer ")) {
        writeJSON(response, 401, { error: "authentication failed" });
        return;
      }
      readJSON(request)
        .then((body) => {
          const sessionID = typeof body.session_id === "string" ? body.session_id.trim() : "";
          if (sessionID === "") {
            writeJSON(response, 400, { error: "session_id must be non-empty" });
            return;
          }
          writeJSON(response, 201, {
            ticket: `ticket-${sessionID}`,
            session_id: sessionID,
            expires_at: "2026-05-28T10:00:30Z"
          });
        })
        .catch((error) => {
          writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
        });
      return;
    default:
      writeJSON(response, 404, { error: `not found: ${url.pathname}` });
  }
});

server.on("upgrade", (request, socket) => {
  const url = new URL(request.url ?? "/", `http://127.0.0.1:${port}`);
  if (url.pathname !== "/ws/diagnosis") {
    socket.destroy();
    return;
  }
  const key = request.headers["sec-websocket-key"];
  const sessionID = url.searchParams.get("session_id") ?? "";
  const ticket = url.searchParams.get("ticket") ?? "";
  if (typeof key !== "string" || sessionID === "" || ticket === "") {
    socket.destroy();
    return;
  }
  const accept = createHash("sha1")
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest("base64");
  socket.write(
    [
      "HTTP/1.1 101 Switching Protocols",
      "Upgrade: websocket",
      "Connection: Upgrade",
      `Sec-WebSocket-Accept: ${accept}`,
      "\r\n"
    ].join("\r\n")
  );

  const state = {
    turnCount: 0,
    conversation: []
  };
  sendWebSocketJSON(socket, {
    type: "ready",
    session_id: sessionID,
    subject: "owner-1"
  });

  let buffer = Buffer.alloc(0);
  socket.on("data", (chunk) => {
    buffer = Buffer.concat([buffer, chunk]);
    for (;;) {
      const decoded = decodeWebSocketFrame(buffer);
      if (!decoded) {
        return;
      }
      buffer = buffer.subarray(decoded.bytes);
      if (decoded.opcode === 8) {
        socket.end();
        return;
      }
      if (decoded.opcode !== 1) {
        continue;
      }
      handleDiagnosisFrame(socket, sessionID, state, decoded.payload);
    }
  });
});

server.listen(port, "127.0.0.1");

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    server.close(() => {
      process.exit(0);
    });
  });
}

function writeJSON(response, status, body) {
  response.writeHead(status, {
    "access-control-allow-headers": "authorization, content-type, accept",
    "access-control-allow-methods": "GET, POST, PUT, OPTIONS",
    "access-control-allow-origin": "*",
    "content-type": "application/json"
  });
  response.end(body === null ? "" : JSON.stringify(body));
}

function readJSON(request) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    request.on("data", (chunk) => chunks.push(chunk));
    request.on("end", () => {
      try {
        resolve(JSON.parse(Buffer.concat(chunks).toString("utf8")));
      } catch (error) {
        reject(error);
      }
    });
    request.on("error", reject);
  });
}

function handleAlertSourceCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: alertSources });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const profile = alertSourceProfileFromBody(nextAlertSourceID, body);
      nextAlertSourceID += 1;
      alertSources.unshift(profile);
      writeJSON(response, 201, profile);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleAlertSourceProfile(request, response, profileID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = alertSources.findIndex((profile) => profile.id === profileID);
      if (index < 0) {
        writeJSON(response, 404, { error: "alert source profile not found" });
        return;
      }
      const current = alertSources[index];
      const profile = alertSourceProfileFromBody(profileID, body, current.created_at);
      alertSources[index] = profile;
      writeJSON(response, 200, profile);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleAlertSourceConnectionTest(request, response, profileID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const profile = alertSources.find((item) => item.id === profileID);
  if (!profile) {
    writeJSON(response, 404, { error: "alert source profile not found" });
    return;
  }
  if (profile.auth_mode === "bearer") {
    writeJSON(response, 200, {
      source_id: profile.id,
      kind: profile.kind,
      auth_mode: profile.auth_mode,
      status: "blocked",
      reason_code: "credentials_unavailable",
      message: "Secret-backed connection tests require a server-side secret resolver.",
      checked_at: "2026-06-05T04:00:00Z",
      observed_alerts: 0
    });
    return;
  }
  if (profile.kind === "alertmanager") {
    writeJSON(response, 200, {
      source_id: profile.id,
      kind: profile.kind,
      auth_mode: profile.auth_mode,
      status: "unsupported",
      reason_code: "unsupported_kind",
      message: "Alertmanager connection tests require the Alertmanager adapter.",
      checked_at: "2026-06-05T04:00:00Z",
      observed_alerts: 0
    });
    return;
  }
  writeJSON(response, 200, {
    source_id: profile.id,
    kind: profile.kind,
    auth_mode: profile.auth_mode,
    status: "success",
    reason_code: "ok",
    message: "Prometheus alert listing succeeded.",
    checked_at: "2026-06-05T04:00:00Z",
    observed_alerts: 2
  });
}

function alertSourceProfileFromBody(id, body, createdAt = "2026-05-28T06:02:00Z") {
  const now = "2026-05-28T06:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    kind: body.kind === "alertmanager" ? "alertmanager" : "prometheus",
    base_url: String(body.base_url ?? ""),
    auth_mode: body.auth_mode === "bearer" ? "bearer" : "none",
    secret_ref: typeof body.secret_ref === "string" ? body.secret_ref : "",
    enabled: Boolean(body.enabled),
    labels: body.labels && typeof body.labels === "object" && !Array.isArray(body.labels) ? body.labels : {},
    created_at: createdAt,
    updated_at: now
  };
}

function handleGroupingPolicyCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: groupingPolicies });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const policy = groupingPolicyFromBody(nextGroupingPolicyID, body);
      nextGroupingPolicyID += 1;
      groupingPolicies.unshift(policy);
      writeJSON(response, 201, policy);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleGroupingPolicy(request, response, policyID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = groupingPolicies.findIndex((policy) => policy.id === policyID);
      if (index < 0) {
        writeJSON(response, 404, { error: "grouping policy not found" });
        return;
      }
      const current = groupingPolicies[index];
      const policy = groupingPolicyFromBody(policyID, body, current.created_at);
      groupingPolicies[index] = policy;
      writeJSON(response, 200, policy);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleGroupingPolicyPreview(request, response, policyID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const policy = groupingPolicies.find((item) => item.id === policyID);
  if (!policy) {
    writeJSON(response, 404, { error: "grouping policy not found" });
    return;
  }
  writeJSON(response, 200, {
    policy_id: policy.id,
    events_scanned: 3,
    events_matched: policy.source_filter.includes("prometheus") || policy.source_filter.length === 0 ? 2 : 0,
    groups:
      policy.source_filter.includes("prometheus") || policy.source_filter.length === 0
        ? [
            {
              group_key: "0000000000000000000000000000000000000000000000000000000000000001",
              dimensions: {
                alertname: "HighCPU",
                service: "checkout"
              },
              severity: "critical",
              event_count: 2,
              first_seen_at: "2026-06-05T04:00:00Z",
              last_seen_at: "2026-06-05T04:01:00Z",
              event_ids: [101, 102]
            }
          ]
        : []
  });
}

function groupingPolicyFromBody(id, body, createdAt = "2026-06-05T04:02:00Z") {
  const now = "2026-06-05T04:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    dimension_keys: Array.isArray(body.dimension_keys) ? body.dimension_keys.map(String) : [],
    severity_key: String(body.severity_key ?? ""),
    source_filter: Array.isArray(body.source_filter) ? body.source_filter.map(String) : [],
    enabled: Boolean(body.enabled),
    created_at: createdAt,
    updated_at: now
  };
}

function handleReportWorkflowPolicyCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: reportWorkflowPolicies });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const policy = reportWorkflowPolicyFromBody(nextReportWorkflowPolicyID, body);
      nextReportWorkflowPolicyID += 1;
      reportWorkflowPolicies.unshift(policy);
      writeJSON(response, 201, policy);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleReportWorkflowPolicy(request, response, policyID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = reportWorkflowPolicies.findIndex((policy) => policy.id === policyID);
      if (index < 0) {
        writeJSON(response, 404, { error: "report workflow policy not found" });
        return;
      }
      const current = reportWorkflowPolicies[index];
      const policy = {
        ...reportWorkflowPolicyFromBody(policyID, body, current.created_at),
        enabled: current.enabled,
        enabled_at: current.enabled_at,
        disabled_at: current.disabled_at
      };
      reportWorkflowPolicies[index] = policy;
      writeJSON(response, 200, policy);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleReportWorkflowPolicyEnablement(request, response, policyID, enabled) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const index = reportWorkflowPolicies.findIndex((policy) => policy.id === policyID);
  if (index < 0) {
    writeJSON(response, 404, { error: "report workflow policy not found" });
    return;
  }
  const policy = reportWorkflowPolicies[index];
  if (enabled) {
    const source = alertSources.find((item) => item.id === policy.alert_source_profile_id);
    const grouping = groupingPolicies.find((item) => item.id === policy.grouping_policy_id);
    if (!source?.enabled || !grouping?.enabled) {
      writeJSON(response, 400, {
        error: "report workflow policy: alert source profile must be enabled before workflow policy enablement"
      });
      return;
    }
  }
  const now = "2026-06-05T08:05:00Z";
  const updated = {
    ...policy,
    enabled,
    enabled_at: enabled ? now : null,
    disabled_at: enabled ? null : now,
    updated_at: now
  };
  reportWorkflowPolicies[index] = updated;
  writeJSON(response, 200, updated);
}

function handleReportWorkflowPolicyImpactPreview(request, response, policyID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const policy = reportWorkflowPolicies.find((item) => item.id === policyID);
  if (!policy) {
    writeJSON(response, 404, { error: "report workflow policy not found" });
    return;
  }
  const source = alertSources.find((item) => item.id === policy.alert_source_profile_id);
  const grouping = groupingPolicies.find((item) => item.id === policy.grouping_policy_id);
  const channel =
    policy.report_notification_channel_profile_id === null
      ? null
      : notificationChannels.find((item) => item.id === policy.report_notification_channel_profile_id) ?? null;
  if (!source || !grouping) {
    writeJSON(response, 404, { error: "report workflow policy binding not found" });
    return;
  }
  const reasonCodes = [];
  if (!source.enabled) {
    reasonCodes.push("alert_source_disabled");
  }
  if (!grouping.enabled) {
    reasonCodes.push("grouping_policy_disabled");
  }
  if (channel && !channel.enabled) {
    reasonCodes.push("notification_channel_disabled");
  }
  if (channel && !channel.delivery_scopes.includes("report")) {
    reasonCodes.push("notification_channel_missing_report_scope");
  }
  const blocked = reasonCodes.length > 0;
  const status = blocked ? "blocked" : "ready";
  writeJSON(response, 200, {
    policy_id: policy.id,
    status,
    reason_codes: blocked ? reasonCodes : ["ok"],
    message: blocked
      ? "Report workflow policy impact preview is blocked by configuration readiness."
      : "Report workflow policy impact preview is ready.",
    checked_at: "2026-06-05T10:00:00Z",
    trigger_mode: policy.trigger_mode,
    report_scenario: policy.report_scenario,
    diagnosis_follow_up: policy.diagnosis_follow_up,
    alert_source_profile_id: source.id,
    alert_source_kind: source.kind,
    alert_source_auth_mode: source.auth_mode,
    alert_source_enabled: source.enabled,
    grouping_policy_id: grouping.id,
    grouping_policy_enabled: grouping.enabled,
    grouping_dimension_keys: grouping.dimension_keys,
    grouping_severity_key: grouping.severity_key,
    grouping_source_filter: grouping.source_filter,
    report_notification_channel_profile_id: policy.report_notification_channel_profile_id,
    report_notification_channel_bound: channel !== null,
    report_notification_channel_enabled: channel?.enabled ?? false,
    report_notification_channel_has_report_scope: channel?.delivery_scopes.includes("report") ?? false,
    events_scanned: 2,
    events_matched: blocked ? 0 : 1,
    groups_estimated: blocked ? 0 : 1,
    groups: blocked
      ? []
      : [
          {
            group_key: "0000000000000000000000000000000000000000000000000000000000000001",
            dimensions: {
              alertname: "HighCPU",
              service: "checkout"
            },
            severity: "critical",
            event_count: 1,
            first_seen_at: "2026-06-05T04:00:00Z",
            last_seen_at: "2026-06-05T04:01:00Z",
            event_ids: [101]
          }
        ]
  });
}

function handleReportWorkflowPolicyReplay(request, response, policyID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const policy = reportWorkflowPolicies.find((item) => item.id === policyID);
      if (!policy) {
        writeJSON(response, 404, { error: "report workflow policy not found" });
        return;
      }
      if (!policy.enabled) {
        writeJSON(response, 400, { error: "report policy trigger: report workflow policy must be enabled before replay" });
        return;
      }
      if (!body.window_start || !body.window_end) {
        writeJSON(response, 400, { error: "window_start and window_end are required" });
        return;
      }
      writeJSON(response, 202, {
        started: true,
        workflow_id: "report-batch-policy-smoke",
        run_id: "run-policy-smoke",
        stats: {
          ingested: {
            total: 1,
            saved: 1,
            duplicate: 0,
            failed: 0
          },
          events_loaded: 1,
          groups_built: 1,
          groups_saved: 1,
          groups_refreshed: 0,
          groups_existing: 0,
          snapshots_saved: 1,
          snapshots_duplicate: 0,
          groups_closed: 1,
          failed: 0
        },
        snapshots: [
          {
            id: 7,
            group_index: 0,
            event_count: 1
          }
        ]
      });
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function reportWorkflowPolicyFromBody(id, body, createdAt = "2026-06-05T08:02:00Z") {
  const now = "2026-06-05T08:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    alert_source_profile_id: Number.parseInt(String(body.alert_source_profile_id ?? "0"), 10),
    grouping_policy_id: Number.parseInt(String(body.grouping_policy_id ?? "0"), 10),
    report_notification_channel_profile_id:
      body.report_notification_channel_profile_id === null || body.report_notification_channel_profile_id === undefined
        ? null
        : Number.parseInt(String(body.report_notification_channel_profile_id), 10),
    trigger_mode: "manual_replay",
    report_scenario: ["single_alert", "cascade", "alert_storm"].includes(body.report_scenario)
      ? body.report_scenario
      : "single_alert",
    diagnosis_follow_up: body.diagnosis_follow_up === "suggest_room" ? "suggest_room" : "disabled",
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: createdAt,
    updated_at: now
  };
}

function handleReportWorkflowScheduleCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: reportWorkflowSchedules });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const schedule = reportWorkflowScheduleFromBody(nextReportWorkflowScheduleID, body);
      nextReportWorkflowScheduleID += 1;
      reportWorkflowSchedules.unshift(schedule);
      writeJSON(response, 201, schedule);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleReportWorkflowSchedule(request, response, scheduleID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = reportWorkflowSchedules.findIndex((schedule) => schedule.id === scheduleID);
      if (index < 0) {
        writeJSON(response, 404, { error: "report workflow schedule not found" });
        return;
      }
      const current = reportWorkflowSchedules[index];
      const schedule = {
        ...reportWorkflowScheduleFromBody(scheduleID, body, current.created_at),
        enabled: current.enabled,
        enabled_at: current.enabled_at,
        disabled_at: current.disabled_at
      };
      reportWorkflowSchedules[index] = schedule;
      writeJSON(response, 200, schedule);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleReportWorkflowScheduleEnablement(request, response, scheduleID, enabled) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const index = reportWorkflowSchedules.findIndex((schedule) => schedule.id === scheduleID);
  if (index < 0) {
    writeJSON(response, 404, { error: "report workflow schedule not found" });
    return;
  }
  const schedule = reportWorkflowSchedules[index];
  if (enabled) {
    const policy = reportWorkflowPolicies.find((item) => item.id === schedule.report_workflow_policy_id);
    if (!policy?.enabled) {
      writeJSON(response, 400, {
        error: "report workflow schedule: report workflow policy must be enabled before schedule enablement"
      });
      return;
    }
  }
  const now = "2026-06-06T02:05:00Z";
  const updated = {
    ...schedule,
    enabled,
    enabled_at: enabled ? now : null,
    disabled_at: enabled ? null : now,
    updated_at: now
  };
  reportWorkflowSchedules[index] = updated;
  writeJSON(response, 200, updated);
}

function reportWorkflowScheduleFromBody(id, body, createdAt = "2026-06-06T02:02:00Z") {
  const now = "2026-06-06T02:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    report_workflow_policy_id: Number.parseInt(String(body.report_workflow_policy_id ?? "0"), 10),
    temporal_schedule_id: String(body.temporal_schedule_id ?? ""),
    interval_seconds: Number.parseInt(String(body.interval_seconds ?? "0"), 10),
    offset_seconds: Number.parseInt(String(body.offset_seconds ?? "0"), 10),
    replay_window_seconds: Number.parseInt(String(body.replay_window_seconds ?? "0"), 10),
    replay_delay_seconds: Number.parseInt(String(body.replay_delay_seconds ?? "0"), 10),
    replay_limit: Number.parseInt(String(body.replay_limit ?? "0"), 10),
    catchup_window_seconds: Number.parseInt(String(body.catchup_window_seconds ?? "0"), 10),
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: createdAt,
    updated_at: now
  };
}

function handleDiagnosisToolTemplateCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: diagnosisToolTemplates });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const template = diagnosisToolTemplateFromBody(nextDiagnosisToolTemplateID, body);
      nextDiagnosisToolTemplateID += 1;
      diagnosisToolTemplates.unshift(template);
      writeJSON(response, 201, template);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleDiagnosisToolTemplate(request, response, templateID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = diagnosisToolTemplates.findIndex((template) => template.id === templateID);
      if (index < 0) {
        writeJSON(response, 404, { error: "diagnosis tool template not found" });
        return;
      }
      const current = diagnosisToolTemplates[index];
      const template = {
        ...diagnosisToolTemplateFromBody(templateID, body, current.created_at),
        enabled: current.enabled,
        enabled_at: current.enabled_at,
        disabled_at: current.disabled_at
      };
      diagnosisToolTemplates[index] = template;
      writeJSON(response, 200, template);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleDiagnosisToolTemplateEnablement(request, response, templateID, enabled) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const index = diagnosisToolTemplates.findIndex((template) => template.id === templateID);
  if (index < 0) {
    writeJSON(response, 404, { error: "diagnosis tool template not found" });
    return;
  }
  const template = diagnosisToolTemplates[index];
  if (enabled) {
    const source = alertSources.find((item) => item.id === template.alert_source_profile_id);
    if (!source?.enabled || source.kind !== "prometheus") {
      writeJSON(response, 400, {
        error: "diagnosis tool template: bound Prometheus alert source must be enabled before template enablement"
      });
      return;
    }
  }
  const now = "2026-06-08T08:05:00Z";
  const updated = {
    ...template,
    enabled,
    enabled_at: enabled ? now : null,
    disabled_at: enabled ? null : now,
    updated_at: now
  };
  diagnosisToolTemplates[index] = updated;
  writeJSON(response, 200, updated);
}

function diagnosisToolTemplateFromBody(id, body, createdAt = "2026-06-08T08:02:00Z") {
  const now = "2026-06-08T08:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    alert_source_profile_id: Number.parseInt(String(body.alert_source_profile_id ?? "0"), 10),
    tool: ["active_alerts", "metric_query", "metric_range_query"].includes(body.tool) ? body.tool : "active_alerts",
    query_template: String(body.query_template ?? ""),
    default_limit: Number.parseInt(String(body.default_limit ?? "0"), 10),
    default_window_seconds: Number.parseInt(String(body.default_window_seconds ?? "0"), 10),
    max_window_seconds: Number.parseInt(String(body.max_window_seconds ?? "0"), 10),
    default_step_seconds: Number.parseInt(String(body.default_step_seconds ?? "0"), 10),
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: createdAt,
    updated_at: now
  };
}

function handleNotificationChannelCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: notificationChannels });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const channel = notificationChannelFromBody(nextNotificationChannelID, body);
      nextNotificationChannelID += 1;
      notificationChannels.unshift(channel);
      writeJSON(response, 201, channel);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleNotificationChannel(request, response, channelID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = notificationChannels.findIndex((channel) => channel.id === channelID);
      if (index < 0) {
        writeJSON(response, 404, { error: "notification channel profile not found" });
        return;
      }
      const current = notificationChannels[index];
      const channel = notificationChannelFromBody(channelID, body, current.created_at);
      notificationChannels[index] = channel;
      writeJSON(response, 200, channel);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleNotificationChannelTest(request, response, channelID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const channel = notificationChannels.find((item) => item.id === channelID);
  if (!channel) {
    writeJSON(response, 404, { error: "notification channel profile not found" });
    return;
  }
  writeJSON(response, 200, {
    channel_id: channel.id,
    kind: channel.kind,
    status: "blocked",
    reason_code: "credentials_unavailable",
    message: "Secret-backed notification channel tests require a server-side secret resolver.",
    checked_at: "2026-06-05T10:00:00Z",
    provider_message_id: "",
    provider_status: ""
  });
}

function notificationChannelFromBody(id, body, createdAt = "2026-06-05T09:02:00Z") {
  const now = "2026-06-05T09:03:00Z";
  const scopes = Array.isArray(body.delivery_scopes) ? body.delivery_scopes.map(String) : [];
  return {
    id,
    name: String(body.name ?? ""),
    kind: "webhook",
    secret_ref: String(body.secret_ref ?? ""),
    delivery_scopes: scopes.filter((scope) => scope === "report" || scope === "diagnosis_close"),
    enabled: Boolean(body.enabled),
    labels: body.labels && typeof body.labels === "object" && !Array.isArray(body.labels) ? body.labels : {},
    created_at: createdAt,
    updated_at: now
  };
}

function handleDiagnosisFrame(socket, sessionID, state, payload) {
  let frame;
  try {
    frame = JSON.parse(payload);
  } catch {
    sendWebSocketJSON(socket, { type: "error", code: "bad_frame", message: "invalid JSON frame" });
    return;
  }
  if (frame.type === "query_state") {
    sendWebSocketJSON(socket, diagnosisState(sessionID, state));
    return;
  }
  if (frame.type === "submit_turn") {
    const text = typeof frame.message === "string" ? frame.message.trim() : "";
    const messageID = typeof frame.message_id === "string" ? frame.message_id : "";
    if (text === "" || messageID === "") {
      sendWebSocketJSON(socket, { type: "error", code: "invalid_request", message: "message is required" });
      return;
    }
    state.turnCount += 1;
    state.conversation.push({ role: "user", content: text });
    const assistant = `Mock diagnosis response for: ${text}`;
    state.conversation.push({ role: "assistant", content: assistant });
    sendWebSocketJSON(socket, {
      type: "turn_result",
      session_id: sessionID,
      chat_session_id: 42,
      message_id: messageID,
      assistant_message_id: `${messageID}/assistant`,
      user_turn_id: state.turnCount * 2 - 1,
      assistant_turn_id: state.turnCount * 2,
      user_sequence: state.turnCount * 2 - 1,
      assistant_sequence: state.turnCount * 2,
      turn_count: state.turnCount,
      context_bytes: 512,
      status: "open",
      assistant_message: assistant,
      requires_human_review: true,
      confidence: "medium"
    });
    return;
  }
  sendWebSocketJSON(socket, { type: "error", code: "bad_frame", message: "unsupported frame type" });
}

function diagnosisState(sessionID, state) {
  return {
    type: "state",
    session_id: sessionID,
    chat_session_id: 42,
    diagnosis_task_id: 7,
    owner_subject: "owner-1",
    status: "open",
    turn_count: state.turnCount,
    started_at: "2026-05-28T10:00:00Z",
    last_activity_at: "2026-05-28T10:00:05Z",
    in_flight: false,
    seen_message_ids: [],
    conversation: state.conversation
  };
}

function sendWebSocketJSON(socket, payload) {
  socket.write(encodeWebSocketFrame(JSON.stringify(payload)));
}

function encodeWebSocketFrame(text) {
  const payload = Buffer.from(text);
  if (payload.length < 126) {
    return Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
  }
  const header = Buffer.alloc(4);
  header[0] = 0x81;
  header[1] = 126;
  header.writeUInt16BE(payload.length, 2);
  return Buffer.concat([header, payload]);
}

function decodeWebSocketFrame(buffer) {
  if (buffer.length < 2) {
    return null;
  }
  const opcode = buffer[0] & 0x0f;
  const masked = (buffer[1] & 0x80) !== 0;
  let length = buffer[1] & 0x7f;
  let offset = 2;
  if (length === 126) {
    if (buffer.length < offset + 2) {
      return null;
    }
    length = buffer.readUInt16BE(offset);
    offset += 2;
  } else if (length === 127) {
    if (buffer.length < offset + 8) {
      return null;
    }
    const bigLength = buffer.readBigUInt64BE(offset);
    if (bigLength > BigInt(Number.MAX_SAFE_INTEGER)) {
      throw new Error("WebSocket frame is too large");
    }
    length = Number(bigLength);
    offset += 8;
  }
  if (!masked) {
    return null;
  }
  if (buffer.length < offset + 4 + length) {
    return null;
  }
  const mask = buffer.subarray(offset, offset + 4);
  offset += 4;
  const payload = Buffer.alloc(length);
  for (let index = 0; index < length; index += 1) {
    payload[index] = buffer[offset + index] ^ mask[index % 4];
  }
  return {
    bytes: offset + length,
    opcode,
    payload: payload.toString("utf8")
  };
}
