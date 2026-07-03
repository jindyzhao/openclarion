import type {
  AlertSourceKind,
  AlertSourceFormState,
  AlertSourceLabels,
  AlertSourceProfile,
  AlertSourceProfileWriteRequest,
} from "./types";
import {
  containsControl,
  containsControlOrWhitespace,
  utf8ByteLength,
} from "../validation";

type ParseResult<T> = { ok: true; value: T } | { ok: false; message: string };

type AlertSourceReadinessStatus = "ready" | "pending" | "blocked";
type SearchParamValue = string | string[] | undefined;

export type AlertSourceLaunchIntentName =
  | "alertmanager-source"
  | "prometheus-source"
  | "thanos-rule-source"
  | "thanos-source";

export type AlertSourceLaunchIntent = {
  baseURL: string;
  kind: AlertSourceKind;
  labelsText: string;
  message: string;
  name: string;
  workflowReturn: AlertSourceWorkflowReturn | null;
};

export type AlertSourceWorkflowReturn = {
  detail: string;
  href: string;
  label: string;
  sourceID: number | null;
};

export type AlertSourceWorkflowReturnOptions = {
  sourceID?: number | null;
};

export type AlertSourcePresetOption = {
  detail: string;
  form: AlertSourceFormState;
  intent: AlertSourceLaunchIntentName;
  label: string;
  message: string;
};

export type AlertSourceReadiness = {
  capabilities: string[];
  detail: string;
  label: string;
  status: AlertSourceReadinessStatus;
};

export type AlertSourceAIRoleReadiness = {
  detail: string;
  label: string;
  role: "alert_intake" | "metric_evidence";
  status: AlertSourceReadinessStatus;
};

export type AlertSourceOperatorStep = {
  detail: string;
  key: string;
  label: string;
  status: AlertSourceReadinessStatus;
};

export type AlertSourceClassificationHint = {
  detail: string;
  label: string;
  suggestedLabelsText: string;
};

export type AlertSourceConnectionTargetPreview = {
  label: string;
  value: string;
};

export type AlertSourceProviderGuidance = {
  detail: string;
  items: AlertSourceProviderGuidanceItem[];
  label: string;
};

type AlertSourceProviderGuidanceItem = {
  detail: string;
  key: string;
  label: string;
  value: string;
};

export type AlertmanagerWebhookDeliveryConfig = {
  authorization: string;
  contentType: string;
  detail: string;
  endpoint: string;
  endpointGuidance: string;
  endpointScope: "absolute" | "relative";
  method: "POST";
  receiverName: string;
  receiverYAML: string;
  routeGuidance: string;
  routeYAML: string;
  routingChecklist: AlertmanagerWebhookRoutingChecklistItem[];
};

type AlertmanagerWebhookRoutingChecklistItem = {
  detail: string;
  key: string;
  label: string;
};

const alertmanagerWebhookPathPrefix = "/api/v1/alert-sources";

export function emptyAlertSourceForm(): AlertSourceFormState {
  return {
    name: "",
    kind: "prometheus",
    baseURL: "",
    authMode: "none",
    secretRef: "",
    enabled: false,
    labelsText: "",
  };
}

export function alertSourceLaunchHref({
  baseURL,
  intent,
  workflowReturn,
}: {
  baseURL?: string;
  intent: AlertSourceLaunchIntentName;
  workflowReturn?: AlertSourceWorkflowReturnOptions;
}): string {
  const params = new URLSearchParams({ intent });
  if (baseURL !== undefined && baseURL !== "") {
    params.set("base_url", baseURL);
  }
  appendAlertSourceWorkflowReturn(params, workflowReturn);
  return `/settings/alert-sources?${params.toString()}`;
}

function alertSourceWorkflowReturnFromSearchParams(
  searchParams: Record<string, SearchParamValue>,
): AlertSourceWorkflowReturn | null {
  if (
    firstSearchParamValue(searchParams.workflow_return) !== "auto-room-enable"
  ) {
    return null;
  }
  const sourceID = positiveSearchParamInteger(
    firstSearchParamValue(searchParams.workflow_source_id),
  );
  const params = new URLSearchParams({
    intent: "enable-ai-room-follow-up",
  });
  if (sourceID !== null) {
    params.set("source_id", String(sourceID));
  }
  return {
    detail:
      "Return to workflow policies after the metric source is tested and evidence templates are ready.",
    href: `/settings/report-workflow-policies?${params.toString()}`,
    label: "Back to workflow",
    sourceID,
  };
}

export function alertSourceLaunchIntentFromSearchParams(
  searchParams: Record<string, SearchParamValue>,
): AlertSourceLaunchIntent | null {
  const baseURL = alertSourceLaunchBaseURLFromSearchParams(
    searchParams.base_url,
  );
  const workflowReturn = alertSourceWorkflowReturnFromSearchParams(searchParams);
  switch (firstSearchParamValue(searchParams.intent)) {
    case "alertmanager-source":
      return {
        baseURL,
        kind: "alertmanager",
        labelsText: "role=alert-intake\nsource=alertmanager",
        message:
          "Prepared an enabled Alertmanager source. Paste the base URL, then save and test it before binding workflows.",
        name: "Alertmanager alert intake",
        workflowReturn,
      };
    case "thanos-rule-source":
      return {
        baseURL,
        kind: "prometheus",
        labelsText: "role=alert-intake\nsource=thanos-rule",
        message:
          "Prepared an enabled Thanos Rule active-alert source. Paste the Thanos Rule alerts URL or API base URL, then save and test it before adding active-alert evidence. Use Alertmanager for webhook-triggered automatic diagnosis rooms.",
        name: "Thanos Rule active alerts",
        workflowReturn,
      };
    case "prometheus-source":
      return {
        baseURL,
        kind: "prometheus",
        labelsText: "role=metric-evidence\nsource=prometheus",
        message:
          "Prepared an enabled Prometheus-compatible source. Paste the base URL, then save and test it before adding metric evidence tools.",
        name: "Prometheus metric evidence",
        workflowReturn,
      };
    case "thanos-source":
      return {
        baseURL,
        kind: "prometheus",
        labelsText: "role=metric-evidence\nsource=thanos",
        message:
          "Prepared an enabled Thanos Query source. Paste the base URL, then save and test it before adding metric evidence tools.",
        name: "Thanos metric evidence",
        workflowReturn,
      };
    default:
      return null;
  }
}

export function alertSourceLaunchIntentKey(
  launchIntent: AlertSourceLaunchIntent | null,
): string {
  if (launchIntent === null) {
    return "default";
  }
  return `${launchIntent.kind}:${launchIntent.name}:${launchIntent.baseURL}:${launchIntent.workflowReturn?.sourceID ?? "none"}`;
}

function appendAlertSourceWorkflowReturn(
  params: URLSearchParams,
  workflowReturn: AlertSourceWorkflowReturnOptions | undefined,
) {
  if (workflowReturn === undefined) {
    return;
  }
  params.set("workflow_return", "auto-room-enable");
  if (
    Number.isSafeInteger(workflowReturn.sourceID) &&
    (workflowReturn.sourceID ?? 0) > 0
  ) {
    params.set("workflow_source_id", String(workflowReturn.sourceID));
  }
}

export function alertSourceLaunchInitialForm(
  launchIntent: AlertSourceLaunchIntent | null,
): AlertSourceFormState {
  if (launchIntent === null) {
    return emptyAlertSourceForm();
  }
  return {
    ...emptyAlertSourceForm(),
    baseURL: launchIntent.baseURL,
    enabled: true,
    kind: launchIntent.kind,
    labelsText: launchIntent.labelsText,
    name: launchIntent.name,
  };
}

export function alertSourcePresetOptions(): AlertSourcePresetOption[] {
  return [
    alertSourcePresetOption(
      "thanos-rule-source",
      "Thanos Rule",
      "Prometheus-compatible active alerts from Thanos Rule.",
    ),
    alertSourcePresetOption(
      "thanos-source",
      "Thanos Query",
      "Prometheus-compatible metric evidence.",
    ),
    alertSourcePresetOption(
      "alertmanager-source",
      "Alertmanager",
      "Generic Alertmanager-compatible active alerts and webhooks.",
    ),
    alertSourcePresetOption(
      "prometheus-source",
      "Prometheus",
      "Generic Prometheus-compatible alerts and metric evidence.",
    ),
  ];
}

export function profileToFormState(
  profile: AlertSourceProfile,
): AlertSourceFormState {
  return {
    name: profile.name,
    kind: profile.kind,
    baseURL: profile.base_url,
    authMode: profile.auth_mode,
    secretRef: profile.secret_ref,
    enabled: profile.enabled,
    labelsText: labelsToText(profile.labels),
  };
}

export function alertSourceReadiness(
  form: AlertSourceFormState,
): AlertSourceReadiness {
  const parsed = formStateToWriteRequest(form);
  const capabilities = alertSourceCapabilitiesForForm(form, parsed);
  if (!parsed.ok) {
    return {
      capabilities,
      detail: parsed.message,
      label: "Complete source configuration.",
      status: "pending",
    };
  }

  if (!parsed.value.enabled) {
    return {
      capabilities,
      detail:
        "The profile can be saved as a draft, but workflows and diagnosis tools require it to be enabled.",
      label: "Source will be saved as draft.",
      status: "pending",
    };
  }

  return {
    capabilities,
    detail:
      alertSourceLabelsIndicateThanosRule(parsed.value.labels)
        ? "Thanos Rule active-alert sources read firing alerts from /api/v1/alerts. Use Thanos Query for metric evidence and Alertmanager for webhook-triggered automatic diagnosis rooms."
        : parsed.value.kind === "alertmanager"
        ? "Alertmanager reads active alerts with silenced, inhibited, and unprocessed alerts filtered out."
        : "Prometheus-compatible sources support active alert reads and metric evidence collection.",
    label: "Source ready for workflows.",
    status: "ready",
  };
}

export function alertSourceClassificationHint(
  form: AlertSourceFormState,
): AlertSourceClassificationHint | null {
  if (
    form.kind !== "prometheus" ||
    alertSourceLabelsTextIndicatesThanosRule(form.labelsText)
  ) {
    return null;
  }
  if (!alertSourceURLLooksLikeRuleAlerts(form.baseURL)) {
    return null;
  }
  return {
    detail:
      "This looks like a rule-service active-alert URL. If it is Thanos Rule, use the Thanos Rule preset or add source=thanos-rule so OpenClarion skips metric probes and uses this source only for active-alert evidence.",
    label: "Review Thanos Rule classification.",
    suggestedLabelsText: "role=alert-intake\nsource=thanos-rule",
  };
}

export function alertSourceAIRoleReadiness(
  profile: AlertSourceProfile,
): AlertSourceAIRoleReadiness {
  if (profile.kind === "alertmanager") {
    const sourceLabel = alertmanagerCompatibleSourceLabel(profile);
    if (!profile.enabled) {
      return {
        detail: `Enable this ${sourceLabel} source before using it for alert webhook intake or active-alert evidence.`,
        label: "Alert intake disabled.",
        role: "alert_intake",
        status: "blocked",
      };
    }
    return {
      detail: `Ready for ${sourceLabel} webhook intake and active-alert evidence in automatic AI diagnosis workflows.`,
      label: "Alert intake ready.",
      role: "alert_intake",
      status: "ready",
    };
  }

  const sourceLabel = prometheusCompatibleSourceLabel(profile);
  if (alertSourceProfileIsThanosRule(profile)) {
    if (!profile.enabled) {
      return {
        detail:
          "Enable this Thanos Rule source before using it for active-alert evidence.",
        label: "Active alerts disabled.",
        role: "alert_intake",
        status: "blocked",
      };
    }
    return {
      detail:
        "Thanos Rule source is ready for active-alert evidence from /api/v1/alerts. Use Thanos Query for metric evidence and Alertmanager for webhook intake.",
      label: "Active alerts ready.",
      role: "alert_intake",
      status: "ready",
    };
  }
  if (!profile.enabled) {
    return {
      detail: `Enable this ${sourceLabel} source before using it for metric evidence collection.`,
      label: "Metric evidence disabled.",
      role: "metric_evidence",
      status: "blocked",
    };
  }
  return {
    detail: `${sourceLabel} source is ready for instant and range metric evidence tools.`,
    label: "Metric evidence ready.",
    role: "metric_evidence",
    status: "ready",
  };
}

export function alertSourceOperatorChecklist(
  form: AlertSourceFormState,
): AlertSourceOperatorStep[] {
  const parsed = formStateToWriteRequest(form);
  const kind = parsed.ok ? parsed.value.kind : form.kind;
  const thanosRule = parsed.ok
    ? alertSourceLabelsIndicateThanosRule(parsed.value.labels)
    : alertSourceFormLooksLikeThanosRule(form);
  const sourceReady = parsed.ok && parsed.value.enabled;
  const sourceStatus: AlertSourceReadinessStatus = parsed.ok
    ? parsed.value.enabled
      ? "ready"
      : "pending"
    : "blocked";
  const testStatus: AlertSourceReadinessStatus = sourceReady
    ? "pending"
    : "blocked";
  if (kind === "alertmanager") {
    return [
      {
        key: "source",
        label: "Source profile",
        detail: sourceReady
          ? "Enabled Alertmanager profile can be saved."
          : "Save an enabled Alertmanager profile first.",
        status: sourceStatus,
      },
      {
        key: "pull-test",
        label: "Active alert pull",
        detail:
          "Run Test to confirm active=true with silenced, inhibited, and unprocessed alerts excluded.",
        status: testStatus,
      },
      {
        key: "webhook",
        label: "Webhook intake",
        detail: sourceReady
          ? "Copy the persisted webhook endpoint from the source row."
          : "Webhook endpoint appears after save.",
        status: sourceReady ? "pending" : "blocked",
      },
      {
        key: "workflow",
        label: "Workflow binding",
        detail:
          "Bind this source to grouping, report replay, and diagnosis evidence workflows.",
        status: sourceReady ? "pending" : "blocked",
      },
    ];
  }
  if (kind === "prometheus" && thanosRule) {
    return [
      {
        key: "source",
        label: "Source profile",
        detail: sourceReady
          ? "Enabled Thanos Rule active-alert profile can be saved."
          : "Save an enabled Thanos Rule active-alert profile first.",
        status: sourceStatus,
      },
      {
        key: "alert-test",
        label: "Active alert pull",
        detail:
          "Run Test to confirm Thanos Rule /api/v1/alerts is reachable.",
        status: testStatus,
      },
      {
        key: "active-alert-tool",
        label: "Diagnosis tools",
        detail:
          "Use this source for active_alerts evidence templates. Use Thanos Query for metric_query and metric_range evidence.",
        status: sourceReady ? "pending" : "blocked",
      },
      {
        key: "workflow",
        label: "Workflow binding",
        detail:
          "Use Alertmanager as the webhook source for automatic diagnosis rooms; bind Thanos Rule as supplemental active-alert evidence.",
        status: sourceReady ? "pending" : "blocked",
      },
    ];
  }
  return [
    {
      key: "source",
      label: "Source profile",
      detail: sourceReady
        ? "Enabled Prometheus-compatible profile can be saved."
        : "Save an enabled Prometheus-compatible profile first.",
      status: sourceStatus,
    },
    {
      key: "alert-test",
      label: "Alert API",
      detail:
        "Run Test to confirm active alerts and the vector(1) metric probe both succeed.",
      status: testStatus,
    },
    {
      key: "metric-tools",
      label: "Diagnosis tools",
      detail:
        "Use this source for active_alerts, metric_query, and metric_range evidence templates.",
      status: sourceReady ? "pending" : "blocked",
    },
    {
      key: "workflow",
      label: "Workflow binding",
      detail:
        "Bind this source to report replay and diagnosis evidence workflows.",
      status: sourceReady ? "pending" : "blocked",
    },
  ];
}

export function alertSourceProviderGuidance(
  form: Pick<AlertSourceFormState, "kind" | "labelsText">,
): AlertSourceProviderGuidance {
  if (form.kind === "alertmanager") {
    return {
      detail:
        "Use Alertmanager when alerts should trigger automatic AI diagnosis rooms through webhook delivery. OpenClarion also tests the active alerts API with silenced, inhibited, and unprocessed alerts excluded.",
      items: [
        {
          detail:
            "Paste the Alertmanager route prefix, the UI alerts page, or /api/v2/alerts. OpenClarion stores the route prefix and tests the active alerts API.",
          key: "base-url",
          label: "Base URL",
          value: "Alertmanager route prefix",
        },
        {
          detail:
            "The persisted source row exposes the OpenClarion webhook receiver URL and scoped Alertmanager route YAML.",
          key: "webhook",
          label: "Webhook intake",
          value: "Receiver after save",
        },
        {
          detail:
            "Bind active_alerts, metric evidence, Enterprise WeChat delivery, and an auto-room workflow before rollout.",
          key: "workflow",
          label: "Workflow",
          value: "Automatic diagnosis trigger",
        },
      ],
      label: "Alertmanager integration",
    };
  }

  if (alertSourceFormLooksLikeThanosRule(form)) {
    return {
      detail:
        "Use Thanos Rule as supplemental active-alert evidence. Route webhook-triggered automatic rooms through Alertmanager and use Thanos Query for metric evidence.",
      items: [
        {
          detail:
            "Paste the Thanos Rule route prefix, /alerts, or /api/v1/alerts. OpenClarion stores the route prefix and only tests active alerts.",
          key: "base-url",
          label: "Base URL",
          value: "Thanos Rule alerts API",
        },
        {
          detail:
            "Keep source=thanos-rule so metric probes are skipped and the source is treated as active-alert evidence only.",
          key: "labels",
          label: "Labels",
          value: "source=thanos-rule",
        },
        {
          detail:
            "Create an active_alerts evidence template for this source; do not use it as the metric confidence source.",
          key: "workflow",
          label: "Workflow",
          value: "Supplemental active alerts",
        },
      ],
      label: "Thanos Rule integration",
    };
  }

  return {
    detail:
      "Use Prometheus-compatible sources, including Thanos Query, for metric evidence that raises diagnosis confidence after the initial alert report.",
    items: [
      {
        detail:
          "Paste the route prefix, graph page, /api/v1/query, or /api/v1/query_range. OpenClarion stores the route prefix and tests alerts plus vector(1).",
        key: "base-url",
        label: "Base URL",
        value: "Prometheus-compatible query API",
      },
      {
        detail:
          "Use source=thanos for Thanos Query or source=prometheus for a direct Prometheus server.",
        key: "labels",
        label: "Labels",
        value: "Metric evidence source",
      },
      {
        detail:
          "Create metric_query or metric_range_query evidence templates so AI diagnosis can request follow-up metrics before finalizing.",
        key: "workflow",
        label: "Workflow",
        value: "Confidence-building evidence",
      },
    ],
    label: "Prometheus-compatible integration",
  };
}

export function formStateToWriteRequest(
  form: AlertSourceFormState,
): ParseResult<AlertSourceProfileWriteRequest> {
  const name = form.name.trim();
  const baseURL = form.baseURL.trim();
  const secretRef = form.secretRef.trim();
  if (name === "") {
    return { ok: false, message: "Profile name is required." };
  }
  if (utf8ByteLength(name) > 120) {
    return { ok: false, message: "Profile name must be 120 bytes or fewer." };
  }
  const urlResult = validateBaseURL(baseURL);
  if (!urlResult.ok) {
    return urlResult;
  }
  const normalizedBaseURL = normalizedProfileBaseURL(
    form.kind,
    urlResult.value,
  );
  if (form.authMode === "none" && secretRef !== "") {
    return { ok: false, message: "Secret reference requires bearer auth." };
  }
  if (form.authMode === "bearer" && secretRef === "") {
    return { ok: false, message: "Bearer auth requires a secret reference." };
  }
  if (utf8ByteLength(secretRef) > 256) {
    return {
      ok: false,
      message: "Secret reference must be 256 bytes or fewer.",
    };
  }
  if (containsControlOrWhitespace(secretRef)) {
    return {
      ok: false,
      message:
        "Secret reference must not contain whitespace or control characters.",
    };
  }
  const labelsResult = parseLabelsText(form.labelsText);
  if (!labelsResult.ok) {
    return labelsResult;
  }
  return {
    ok: true,
    value: {
      name,
      kind: form.kind,
      base_url: normalizedBaseURL,
      auth_mode: form.authMode,
      ...(secretRef === "" ? {} : { secret_ref: secretRef }),
      enabled: form.enabled,
      labels: labelsResult.value,
    },
  };
}

export function alertSourceConnectionTarget({
  baseURL,
  kind,
  labelsText,
}: {
  baseURL: string;
  kind: AlertSourceKind;
  labelsText?: string;
}): ParseResult<string> {
  const targets = alertSourceConnectionTargets({ baseURL, kind, labelsText });
  if (!targets.ok) {
    return targets;
  }
  return { ok: true, value: targets.value[0]?.value ?? "" };
}

export function alertSourceCanonicalBaseURL({
  baseURL,
  kind,
}: {
  baseURL: string;
  kind: AlertSourceKind;
}): ParseResult<string> {
  const raw = baseURL.trim();
  const urlResult = validateBaseURL(raw);
  if (!urlResult.ok) {
    return urlResult;
  }
  return {
    ok: true,
    value: normalizedProfileBaseURL(kind, urlResult.value),
  };
}

export function alertSourceConnectionTargets({
  baseURL,
  kind,
  labelsText,
}: {
  baseURL: string;
  kind: AlertSourceKind;
  labelsText?: string;
}): ParseResult<AlertSourceConnectionTargetPreview[]> {
  const raw = baseURL.trim();
  const urlResult = validateBaseURL(raw);
  if (!urlResult.ok) {
    return urlResult;
  }
  const parsed = new URL(normalizedProfileBaseURL(kind, urlResult.value));
  if (kind === "alertmanager") {
    return {
      ok: true,
      value: [
        {
          label: "Active alerts",
          value: alertmanagerActiveAlertsTarget(parsed),
        },
      ],
    };
  }
  if (
    labelsText !== undefined &&
    alertSourceLabelsTextIndicatesThanosRule(labelsText)
  ) {
    return {
      ok: true,
      value: [{ label: "Active alerts", value: prometheusAlertsTarget(parsed) }],
    };
  }
  return {
    ok: true,
    value: [
      { label: "Active alerts", value: prometheusAlertsTarget(parsed) },
      { label: "Metric probe", value: prometheusMetricProbeTarget(parsed) },
    ],
  };
}

export function alertmanagerWebhookEndpoint(
  sourceID: number,
  publicAPIBaseURL = "",
): string {
  const path = `${alertmanagerWebhookPathPrefix}/${sourceID}/webhooks/alertmanager`;
  const base = publicAPIBaseURL.trim();
  if (base === "") {
    return path;
  }
  let parsed: URL;
  try {
    parsed = new URL(base);
  } catch {
    return path;
  }
  if (
    (parsed.protocol !== "http:" && parsed.protocol !== "https:") ||
    parsed.username !== "" ||
    parsed.password !== "" ||
    parsed.search !== "" ||
    parsed.hash !== ""
  ) {
    return path;
  }
  const prefix = parsed.pathname.replace(/\/+$/, "");
  parsed.pathname = prefix === "" ? path : `${prefix}${path}`;
  return parsed.toString();
}

export function alertmanagerWebhookPublicBaseURL(
  configuredPublicBaseURL: string | undefined,
  browserOrigin: string | null,
): string {
  const configured = configuredPublicBaseURL?.trim() ?? "";
  if (configured !== "") {
    return configured;
  }
  return browserOrigin ?? "";
}

export function alertmanagerWebhookDeliveryConfig(
  profile: AlertSourceProfile,
  publicAPIBaseURL = "",
): AlertmanagerWebhookDeliveryConfig | null {
  if (profile.kind !== "alertmanager") {
    return null;
  }
  const authorization =
    profile.auth_mode === "bearer"
      ? `Authorization: Bearer token resolved from ${profile.secret_ref || "the source secret reference"}`
      : "No Authorization header";
  const endpoint = alertmanagerWebhookEndpoint(profile.id, publicAPIBaseURL);
  const endpointScope = absoluteHTTPURL(endpoint) ? "absolute" : "relative";
  return {
    authorization,
    contentType: "application/json",
    detail:
      "Configure Alertmanager, or another Alertmanager webhook-compatible sender, to POST webhook v4 JSON to this endpoint. Thanos Rule alerts should normally route through Alertmanager first. Resolved, silenced, inhibited, and muted alerts are ignored during ingest.",
    endpoint,
    endpointGuidance:
      endpointScope === "absolute"
        ? "This receiver URL is absolute and can be copied into an external Alertmanager route."
        : "Set NEXT_PUBLIC_OPENCLARION_API_PUBLIC_BASE_URL to an externally reachable OpenClarion URL before copying this receiver to an external Alertmanager.",
    endpointScope,
    method: "POST",
    receiverName: alertmanagerWebhookReceiverName(profile.id),
    receiverYAML: alertmanagerWebhookReceiverYAML(profile, publicAPIBaseURL),
    routeGuidance:
      "Merge this child route under the existing Alertmanager route.routes list, then adjust matchers to the alert labels OpenClarion should diagnose.",
    routeYAML: alertmanagerWebhookRouteYAML(profile),
    routingChecklist: alertmanagerWebhookRoutingChecklist(profile),
  };
}

export function parseLabelsText(value: string): ParseResult<AlertSourceLabels> {
  const labels: AlertSourceLabels = {};
  const lines = value.split(/\r?\n/);
  for (let index = 0; index < lines.length; index += 1) {
    const rawLine = lines[index] ?? "";
    const line = rawLine.trim();
    if (line === "") {
      continue;
    }
    const separator = line.indexOf("=");
    if (separator <= 0) {
      return {
        ok: false,
        message: `Label line ${index + 1} must use key=value.`,
      };
    }
    const key = line.slice(0, separator).trim();
    const val = line.slice(separator + 1).trim();
    if (key === "") {
      return {
        ok: false,
        message: `Label line ${index + 1} has an empty key.`,
      };
    }
    if (Object.hasOwn(labels, key)) {
      return { ok: false, message: `Label key "${key}" is duplicated.` };
    }
    if (Object.keys(labels).length >= 32) {
      return { ok: false, message: "Labels must contain 32 entries or fewer." };
    }
    if (utf8ByteLength(key) > 64 || utf8ByteLength(val) > 128) {
      return {
        ok: false,
        message: `Label line ${index + 1} exceeds the allowed length.`,
      };
    }
    if (containsControl(key) || containsControl(val)) {
      return {
        ok: false,
        message: "Labels must not contain control characters.",
      };
    }
    labels[key] = val;
  }
  return { ok: true, value: labels };
}

export function labelsToText(labels: AlertSourceLabels): string {
  return Object.entries(labels)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
}

function alertSourcePresetOption(
  intent: AlertSourceLaunchIntentName,
  label: string,
  detail: string,
): AlertSourcePresetOption {
  const launchIntent = alertSourceLaunchIntentFromSearchParams({ intent });
  if (launchIntent === null) {
    return {
      detail,
      form: emptyAlertSourceForm(),
      intent,
      label,
      message: "",
    };
  }
  return {
    detail,
    form: alertSourceLaunchInitialForm(launchIntent),
    intent,
    label,
    message: launchIntent.message,
  };
}

function alertSourceLaunchBaseURLFromSearchParams(
  raw: SearchParamValue,
): string {
  const baseURL = firstSearchParamValue(raw);
  if (baseURL === null || baseURL.trim() !== baseURL) {
    return "";
  }
  const result = validateBaseURL(baseURL);
  if (!result.ok) {
    return "";
  }
  return baseURL;
}

function validateBaseURL(raw: string): ParseResult<string> {
  if (raw === "") {
    return { ok: false, message: "Base URL is required." };
  }
  if (utf8ByteLength(raw) > 2048) {
    return { ok: false, message: "Base URL must be 2048 bytes or fewer." };
  }
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    return { ok: false, message: "Base URL must be a valid URL." };
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return { ok: false, message: "Base URL scheme must be http or https." };
  }
  if (parsed.username !== "" || parsed.password !== "") {
    return { ok: false, message: "Base URL must not include userinfo." };
  }
  if (parsed.search !== "" || parsed.hash !== "") {
    return {
      ok: false,
      message: "Base URL must not include query or fragment.",
    };
  }
  return { ok: true, value: raw };
}

function alertSourceCapabilitiesForForm(
  form: AlertSourceFormState,
  parsed: ParseResult<AlertSourceProfileWriteRequest>,
): string[] {
  const kind = parsed.ok ? parsed.value.kind : form.kind;
  const thanosRule = parsed.ok
    ? alertSourceLabelsIndicateThanosRule(parsed.value.labels)
    : alertSourceFormLooksLikeThanosRule(form);
  return alertSourceCapabilities(kind, thanosRule);
}

function alertSourceCapabilities(
  kind: AlertSourceKind,
  thanosRule = false,
): string[] {
  switch (kind) {
    case "alertmanager":
      return [
        "Active alert listing",
        "Alertmanager webhook ingest",
        "Silenced/inhibited alerts ignored",
      ];
    case "prometheus":
      if (thanosRule) {
        return [
          "Active alert listing",
          "Thanos Rule alerts API",
          "Metric queries not required",
        ];
      }
      return [
        "Active alert listing",
        "Instant metric evidence",
        "Range metric evidence",
      ];
  }
}

function prometheusCompatibleSourceLabel(profile: AlertSourceProfile): string {
  const source = profile.labels.source?.trim().toLowerCase();
  if (source === "thanos-rule") {
    return "Thanos Rule";
  }
  if (source === "thanos") {
    return "Thanos Query";
  }
  return "Prometheus-compatible";
}

export function alertSourceProfileIsThanosRule(
  profile: Pick<AlertSourceProfile, "kind" | "labels">,
): boolean {
  return (
    profile.kind === "prometheus" &&
    alertSourceLabelsIndicateThanosRule(profile.labels)
  );
}

function alertSourceLabelsIndicateThanosRule(
  labels: AlertSourceLabels | null | undefined,
): boolean {
  return (labels?.source ?? "").trim().toLowerCase() === "thanos-rule";
}

function alertSourceFormLooksLikeThanosRule(
  form: Pick<AlertSourceFormState, "labelsText">,
): boolean {
  return alertSourceLabelsTextIndicatesThanosRule(form.labelsText);
}

function alertSourceLabelsTextIndicatesThanosRule(labelsText: string): boolean {
  const labels = parseLabelsText(labelsText);
  return labels.ok && alertSourceLabelsIndicateThanosRule(labels.value);
}

function alertSourceURLLooksLikeRuleAlerts(baseURL: string): boolean {
  let parsed: URL;
  try {
    parsed = new URL(baseURL.trim());
  } catch {
    return false;
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return false;
  }
  const hostname = parsed.hostname.toLowerCase();
  const pathname = parsed.pathname.replace(/\/+$/, "").toLowerCase();
  const ruleSurface =
    hostname.includes("rule") || pathname.split("/").includes("rule");
  const alertSurface =
    pathname.endsWith("/alerts") || pathname.endsWith("/api/v1/alerts");
  return ruleSurface && alertSurface;
}

export function alertmanagerCompatibleSourceLabel(
  profile: AlertSourceProfile,
): string {
  const source = profile.labels.source?.trim().toLowerCase();
  if (source === "thanos-rule") {
    return "Thanos Rule";
  }
  return "Alertmanager";
}

function prometheusAlertsTarget(base: URL): string {
  const target = new URL(base.toString());
  const path = prometheusRoutePrefix(target.pathname).replace(/\/+$/, "");
  target.pathname = `${path}/api/v1/alerts`;
  target.search = "";
  target.hash = "";
  return target.toString();
}

function prometheusMetricProbeTarget(base: URL): string {
  const target = new URL(base.toString());
  const path = prometheusRoutePrefix(target.pathname).replace(/\/+$/, "");
  target.pathname = `${path}/api/v1/query`;
  target.search = "";
  target.searchParams.set("query", "vector(1)");
  target.searchParams.set("limit", "1");
  target.hash = "";
  return target.toString();
}

function normalizedProfileBaseURL(kind: AlertSourceKind, raw: string): string {
  const parsed = new URL(raw);
  const normalizedPath =
    kind === "prometheus"
      ? prometheusRoutePrefix(parsed.pathname)
      : alertmanagerRoutePrefix(parsed.pathname);
  parsed.pathname = normalizedPath === "" ? "/" : normalizedPath;
  return urlWithoutSyntheticRootSlash(parsed);
}

function prometheusRoutePrefix(pathname: string): string {
  const path = pathname.replace(/\/+$/, "");
  const marker = "/api/v1";
  const index = path.lastIndexOf(marker);
  if (index < 0) {
    return stripKnownTerminalPath(path, [
      "/graph",
      "/alerts",
      "/rules",
      "/targets",
      "/service-discovery",
      "/status",
      "/flags",
      "/config",
      "/-/healthy",
      "/-/ready",
    ]);
  }
  const after = path.slice(index + marker.length);
  if (after !== "" && !after.startsWith("/")) {
    return path;
  }
  return path.slice(0, index);
}

function alertmanagerRoutePrefix(pathname: string): string {
  const path = pathname.replace(/\/+$/, "");
  const marker = "/api/v2";
  const index = path.lastIndexOf(marker);
  if (index < 0) {
    return stripKnownTerminalPath(path, [
      "/alerts",
      "/silences",
      "/status",
      "/receivers",
    ]);
  }
  const after = path.slice(index + marker.length);
  if (after !== "" && !after.startsWith("/")) {
    return path;
  }
  return path.slice(0, index);
}

function stripKnownTerminalPath(path: string, terminalPaths: string[]): string {
  for (const terminal of terminalPaths) {
    if (path === terminal) {
      return "";
    }
    if (path.endsWith(terminal)) {
      return path.slice(0, -terminal.length);
    }
  }
  return path;
}

function urlWithoutSyntheticRootSlash(parsed: URL): string {
  const serialized = parsed.toString();
  if (parsed.pathname === "/" && parsed.search === "" && parsed.hash === "") {
    return `${parsed.protocol}//${parsed.host}`;
  }
  return serialized;
}

function firstSearchParamValue(value: SearchParamValue): string | null {
  if (Array.isArray(value)) {
    return value[0] ?? null;
  }
  return value ?? null;
}

function positiveSearchParamInteger(value: string | null): number | null {
  if (value === null || value.trim() !== value || value === "") {
    return null;
  }
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
}

function alertmanagerActiveAlertsTarget(base: URL): string {
  const target = new URL(base.toString());
  const path = target.pathname.replace(/\/+$/, "");
  if (path === "") {
    target.pathname = "/api/v2/alerts";
  } else if (path.endsWith("/api/v2/alerts")) {
    target.pathname = path;
  } else if (path.endsWith("/api/v2")) {
    target.pathname = `${path}/alerts`;
  } else {
    target.pathname = `${path}/api/v2/alerts`;
  }
  target.search = "";
  target.searchParams.set("active", "true");
  target.searchParams.set("inhibited", "false");
  target.searchParams.set("silenced", "false");
  target.searchParams.set("unprocessed", "false");
  target.hash = "";
  return target.toString();
}

function alertmanagerWebhookReceiverName(sourceID: number): string {
  return `openclarion-source-${sourceID}`;
}

function alertmanagerWebhookReceiverYAML(
  profile: AlertSourceProfile,
  publicAPIBaseURL: string,
): string {
  const lines = [
    "receivers:",
    `  - name: ${yamlDoubleQuoted(alertmanagerWebhookReceiverName(profile.id))}`,
    "    webhook_configs:",
    `      - url: ${yamlDoubleQuoted(alertmanagerWebhookEndpoint(profile.id, publicAPIBaseURL))}`,
    "        send_resolved: false",
  ];
  if (profile.auth_mode === "bearer") {
    lines.push(
      "        http_config:",
      "          authorization:",
      "            type: Bearer",
      `            credentials: ${yamlDoubleQuoted(`<token from ${profile.secret_ref || "the source secret reference"}>`)}`,
    );
  }
  return lines.join("\n");
}

function alertmanagerWebhookRouteYAML(profile: AlertSourceProfile): string {
  const receiverName = alertmanagerWebhookReceiverName(profile.id);
  const matchers = alertmanagerWebhookRouteMatchers(profile);
  return [
    "route:",
    "  routes:",
    `    - receiver: ${yamlDoubleQuoted(receiverName)}`,
    "      matchers:",
    ...matchers.map((matcher) => `        - ${matcher}`),
    "      continue: true",
  ].join("\n");
}

function alertmanagerWebhookRouteMatchers(
  profile: AlertSourceProfile,
): string[] {
  const explicitMatchers = Object.entries(profile.labels ?? {})
    .filter(([key]) => key !== "role" && key !== "source")
    .filter(([key]) => alertmanagerMatcherLabelNameIsSafe(key))
    .sort(([left], [right]) => left.localeCompare(right))
    .slice(0, 4)
    .map(([key, value]) => `${key}=${yamlDoubleQuoted(value)}`);
  if (explicitMatchers.length > 0) {
    return explicitMatchers;
  }
  return ['severity=~"warning|critical"'];
}

function alertmanagerMatcherLabelNameIsSafe(value: string): boolean {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(value);
}

function alertmanagerWebhookRoutingChecklist(
  profile: AlertSourceProfile,
): AlertmanagerWebhookRoutingChecklistItem[] {
  const receiverName = alertmanagerWebhookReceiverName(profile.id);
  return [
    {
      detail: `Copy the receiver YAML into Alertmanager receivers as ${receiverName}.`,
      key: "receiver",
      label: "Add receiver",
    },
    {
      detail: `Add a scoped Alertmanager route that selects alerts OpenClarion should diagnose and sends them to ${receiverName}. Use continue: true only when existing downstream receivers should also run.`,
      key: "route",
      label: "Bind route",
    },
    {
      detail:
        "Reload Alertmanager, then run Test in OpenClarion and send a bounded synthetic alert to confirm webhook delivery.",
      key: "reload-test",
      label: "Reload and test",
    },
  ];
}

function yamlDoubleQuoted(value: string): string {
  return JSON.stringify(value);
}

function absoluteHTTPURL(raw: string): boolean {
  try {
    const parsed = new URL(raw);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.host !== "" &&
      parsed.username === "" &&
      parsed.password === ""
    );
  } catch {
    return false;
  }
}
