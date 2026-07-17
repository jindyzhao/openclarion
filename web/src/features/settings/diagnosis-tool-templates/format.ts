import type {
  AlertSourceKind,
  AlertSourceProfile
} from "../alert-sources/types";
import type {
  DiagnosisToolKind,
  DiagnosisToolTemplate,
  DiagnosisToolTemplateFormState,
  DiagnosisToolTemplateWriteRequest
} from "./types";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

export type DiagnosisQueryTemplatePreview = {
  ok: boolean;
  message: string;
  placeholders: string[];
  previewQuery: string;
};

export type DiagnosisToolTemplatePreset = {
  id: string;
  label: string;
  form: Omit<DiagnosisToolTemplateFormState, "alertSourceProfileID">;
};

export type DiagnosisToolTemplateRecommendation = {
  group: string;
  id: string;
  label: string;
  presetID: string;
  sourceID: number;
  sourceName: string;
};

export type DiagnosisToolTemplateLaunchIntent = {
  alertSourceProfileID: number | null;
  message: string;
  presetID: string;
  workflowReturn: DiagnosisToolTemplateWorkflowReturn | null;
};

export type DiagnosisToolTemplateLaunchIntentName = "active-alert-tool" | "metric-evidence-tool";

export type DiagnosisToolTemplateWorkflowReturn = {
  detail: string;
  href: string;
  label: string;
  sourceID: number | null;
};

export type DiagnosisToolTemplateWorkflowReturnOptions = {
  sourceID?: number | null;
};

type SearchParamValue = string | string[] | undefined;

type DiagnosisToolSourceReadinessStatus = "ready" | "pending" | "blocked";
type DiagnosisToolCoverageStatus = "ready" | "review" | "pending";

export type DiagnosisToolSourceReadiness = {
  blockers: string[];
  compatibleSourceCount: number;
  detail: string;
  label: string;
  requiredKinds: AlertSourceKind[];
  selectedSourceKind: AlertSourceKind | null;
  status: DiagnosisToolSourceReadinessStatus;
};

export type DiagnosisToolSaveCompatibility =
  | { ok: true }
  | { ok: false; message: string };

export type DiagnosisToolCoverage = {
  activeAlertTemplates: number;
  detail: string;
  enabledTemplates: number;
  label: string;
  metricTemplates: number;
  rangeMetricTemplates: number;
  sourceNames: string[];
  status: DiagnosisToolCoverageStatus;
};

const diagnosisQueryPlaceholderPattern = /\{\{\s*(label|annotation)\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}/g;
const targetAvailabilityQuery = `up{job="{{label.job}}"}`;
const kubernetesPodRestartQuery =
  `increase(kube_pod_container_status_restarts_total{namespace="{{label.namespace}}",pod="{{label.pod}}"}[5m])`;
const kubernetesPodCPUQuery =
  `sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="{{label.namespace}}",pod="{{label.pod}}",container!="",container!="POD"}[5m]))`;
const kubernetesPodMemoryQuery =
  `sum by (pod) (container_memory_working_set_bytes{namespace="{{label.namespace}}",pod="{{label.pod}}",container!="",container!="POD"})`;
const kubernetesJVMHeapUsedQuery =
  `sum by (namespace,kubernetes_pod_name) (jvm_memory_used_bytes{namespace="{{label.namespace}}",kubernetes_pod_name=~"{{label.pod}}",area="heap"})`;
const kubernetesJVMHeapUsagePctQuery =
  `100 * sum by (namespace,kubernetes_pod_name) (jvm_memory_used_bytes{namespace="{{label.namespace}}",kubernetes_pod_name=~"{{label.pod}}",area="heap"}) / sum by (namespace,kubernetes_pod_name) (jvm_memory_max_bytes{namespace="{{label.namespace}}",kubernetes_pod_name=~"{{label.pod}}",area="heap"} > 0)`;
const oracleTablespacePctUsedQuery =
  `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`;

export const diagnosisToolKindLabels: Record<DiagnosisToolKind, string> = {
  active_alerts: "Active alerts",
  metric_query: "Instant metric",
  metric_range_query: "Range metric"
};

export const diagnosisToolTemplatePresets: DiagnosisToolTemplatePreset[] = [
  {
    id: "active-alerts-current-source",
    label: "Current active alerts",
    form: {
      name: "Current active alerts",
      tool: "active_alerts",
      queryTemplate: "",
      defaultLimit: 10,
      defaultWindowSeconds: 0,
      maxWindowSeconds: 0,
      defaultStepSeconds: 0
    }
  },
  {
    id: "target-availability-instant",
    label: "Target availability instant",
    form: {
      name: "Target availability instant",
      tool: "metric_query",
      queryTemplate: targetAvailabilityQuery,
      defaultLimit: 10,
      defaultWindowSeconds: 0,
      maxWindowSeconds: 0,
      defaultStepSeconds: 0
    }
  },
  {
    id: "kubernetes-pod-restarts-range",
    label: "Kubernetes pod restarts range",
    form: {
      name: "Kubernetes pod restarts range",
      tool: "metric_range_query",
      queryTemplate: kubernetesPodRestartQuery,
      defaultLimit: 10,
      defaultWindowSeconds: 3600,
      maxWindowSeconds: 21600,
      defaultStepSeconds: 60
    }
  },
  {
    id: "kubernetes-pod-cpu-range",
    label: "Kubernetes pod CPU range",
    form: {
      name: "Kubernetes pod CPU range",
      tool: "metric_range_query",
      queryTemplate: kubernetesPodCPUQuery,
      defaultLimit: 10,
      defaultWindowSeconds: 3600,
      maxWindowSeconds: 21600,
      defaultStepSeconds: 60
    }
  },
  {
    id: "kubernetes-pod-memory-range",
    label: "Kubernetes pod memory range",
    form: {
      name: "Kubernetes pod memory range",
      tool: "metric_range_query",
      queryTemplate: kubernetesPodMemoryQuery,
      defaultLimit: 10,
      defaultWindowSeconds: 3600,
      maxWindowSeconds: 21600,
      defaultStepSeconds: 60
    }
  },
  {
    id: "kubernetes-jvm-heap-used-range",
    label: "Kubernetes JVM heap used range",
    form: {
      name: "Kubernetes JVM heap used range",
      tool: "metric_range_query",
      queryTemplate: kubernetesJVMHeapUsedQuery,
      defaultLimit: 10,
      defaultWindowSeconds: 3600,
      maxWindowSeconds: 21600,
      defaultStepSeconds: 60
    }
  },
  {
    id: "kubernetes-jvm-heap-usage-pct-range",
    label: "Kubernetes JVM heap usage pct range",
    form: {
      name: "Kubernetes JVM heap usage pct range",
      tool: "metric_range_query",
      queryTemplate: kubernetesJVMHeapUsagePctQuery,
      defaultLimit: 10,
      defaultWindowSeconds: 3600,
      maxWindowSeconds: 21600,
      defaultStepSeconds: 60
    }
  },
  {
    id: "oracle-tablespace-pct-used-instant",
    label: "Oracle tablespace pct used instant",
    form: {
      name: "Oracle tablespace pct used instant",
      tool: "metric_query",
      queryTemplate: oracleTablespacePctUsedQuery,
      defaultLimit: 5,
      defaultWindowSeconds: 0,
      maxWindowSeconds: 0,
      defaultStepSeconds: 0
    }
  },
  {
    id: "oracle-tablespace-pct-used-range",
    label: "Oracle tablespace pct used range",
    form: {
      name: "Oracle tablespace pct used range",
      tool: "metric_range_query",
      queryTemplate: oracleTablespacePctUsedQuery,
      defaultLimit: 5,
      defaultWindowSeconds: 3600,
      maxWindowSeconds: 21600,
      defaultStepSeconds: 60
    }
  }
];

export function diagnosisToolTemplatePresetByID(presetID: string): DiagnosisToolTemplatePreset | null {
  return diagnosisToolTemplatePresets.find((preset) => preset.id === presetID) ?? null;
}

export function diagnosisToolTemplateRecommendations(
  sources: AlertSourceProfile[]
): DiagnosisToolTemplateRecommendation[] {
  return sources
    .filter((source) => source.enabled)
    .flatMap((source) =>
      recommendedPresetIDsForSource(source).map((presetID) => {
        const preset = diagnosisToolTemplatePresetByID(presetID);
        if (preset === null) {
          return null;
        }
        return {
          group: recommendationGroupForSource(source),
          id: `${source.id}:${preset.id}`,
          label: preset.label,
          presetID: preset.id,
          sourceID: source.id,
          sourceName: source.name
        };
      })
    )
    .filter((item): item is DiagnosisToolTemplateRecommendation => item !== null);
}

export function diagnosisToolTemplateLaunchHref({
  intent,
  sourceID,
  workflowReturn
}: {
  intent: DiagnosisToolTemplateLaunchIntentName;
  sourceID?: number | null;
  workflowReturn?: DiagnosisToolTemplateWorkflowReturnOptions;
}): string {
  const params = new URLSearchParams({ intent });
  if (positiveInteger(sourceID ?? null)) {
    params.set("source_id", String(sourceID));
  }
  appendDiagnosisToolTemplateWorkflowReturn(params, workflowReturn);
  return `/settings/diagnosis-tool-templates?${params.toString()}`;
}

function diagnosisToolTemplateWorkflowReturnFromSearchParams(
  searchParams: Record<string, SearchParamValue>
): DiagnosisToolTemplateWorkflowReturn | null {
  if (firstSearchParamValue(searchParams.workflow_return) !== "auto-room-enable") {
    return null;
  }
  const sourceID = positiveSearchParamInteger(firstSearchParamValue(searchParams.workflow_source_id));
  const params = new URLSearchParams({
    intent: "enable-ai-room-follow-up"
  });
  if (sourceID !== null) {
    params.set("source_id", String(sourceID));
  }
  return {
    detail:
      "Return to workflow policies after the required evidence templates are saved and enabled.",
    href: `/settings/report-workflow-policies?${params.toString()}`,
    label: "Back to workflow",
    sourceID
  };
}

export function diagnosisToolTemplateLaunchIntentFromSearchParams(
  searchParams: Record<string, SearchParamValue>
): DiagnosisToolTemplateLaunchIntent | null {
  const sourceID = positiveSearchParamInteger(firstSearchParamValue(searchParams.source_id));
  const presetID = firstSearchParamValue(searchParams.preset);
  const workflowReturn = diagnosisToolTemplateWorkflowReturnFromSearchParams(searchParams);
  if (presetID !== null) {
    const preset = diagnosisToolTemplatePresetByID(presetID);
    if (preset !== null) {
      return {
        alertSourceProfileID: sourceID,
        message: `Prepared ${preset.label} from the URL preset.`,
        presetID: preset.id,
        workflowReturn
      };
    }
  }

  switch (firstSearchParamValue(searchParams.intent)) {
    case "active-alert-tool":
      return {
        alertSourceProfileID: sourceID,
        message: "Prepared current active alerts from the settings overview action.",
        presetID: "active-alerts-current-source",
        workflowReturn
      };
    case "metric-evidence-tool":
      return {
        alertSourceProfileID: sourceID,
        message: "Prepared Kubernetes pod CPU range from the settings overview action.",
        presetID: "kubernetes-pod-cpu-range",
        workflowReturn
      };
    default:
      return null;
  }
}

export function diagnosisToolTemplateLaunchIntentKey(
  launchIntent: DiagnosisToolTemplateLaunchIntent | null
): string {
  if (launchIntent === null) {
    return "default";
  }
  return `${launchIntent.presetID}:${launchIntent.alertSourceProfileID ?? "auto"}:${launchIntent.workflowReturn?.sourceID ?? "none"}`;
}

function appendDiagnosisToolTemplateWorkflowReturn(
  params: URLSearchParams,
  workflowReturn: DiagnosisToolTemplateWorkflowReturnOptions | undefined
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

export function emptyDiagnosisToolTemplateForm(): DiagnosisToolTemplateFormState {
  return {
    name: "",
    alertSourceProfileID: null,
    tool: "active_alerts",
    queryTemplate: "",
    defaultLimit: 5,
    defaultWindowSeconds: 0,
    maxWindowSeconds: 0,
    defaultStepSeconds: 0
  };
}

export function presetToFormState(
  preset: DiagnosisToolTemplatePreset,
  alertSourceProfileID: number | null
): DiagnosisToolTemplateFormState {
  return {
    ...preset.form,
    alertSourceProfileID
  };
}

export function templateToFormState(template: DiagnosisToolTemplate): DiagnosisToolTemplateFormState {
  return {
    name: template.name,
    alertSourceProfileID: template.alert_source_profile_id,
    tool: template.tool,
    queryTemplate: template.query_template,
    defaultLimit: template.default_limit,
    defaultWindowSeconds: template.default_window_seconds,
    maxWindowSeconds: template.max_window_seconds,
    defaultStepSeconds: template.default_step_seconds
  };
}

export function defaultFormForTool(tool: DiagnosisToolKind): Partial<DiagnosisToolTemplateFormState> {
  switch (tool) {
    case "active_alerts":
      return {
        queryTemplate: "",
        defaultLimit: 5,
        defaultWindowSeconds: 0,
        maxWindowSeconds: 0,
        defaultStepSeconds: 0
      };
    case "metric_query":
      return {
        defaultLimit: 5,
        defaultWindowSeconds: 0,
        maxWindowSeconds: 0,
        defaultStepSeconds: 0
      };
    case "metric_range_query":
      return {
        defaultLimit: 5,
        defaultWindowSeconds: 3600,
        maxWindowSeconds: 21600,
        defaultStepSeconds: 60
      };
  }
}

function diagnosisToolCompatibleSourceKinds(tool: DiagnosisToolKind): AlertSourceKind[] {
  switch (tool) {
    case "active_alerts":
      return ["alertmanager", "prometheus"];
    case "metric_query":
    case "metric_range_query":
      return ["prometheus"];
  }
}

function recommendedPresetIDsForSource(source: AlertSourceProfile): string[] {
  if (source.kind === "alertmanager" || alertSourceProfileLabelsIndicateThanosRule(source.labels)) {
    return ["active-alerts-current-source"];
  }
  const sourceLabel = String(source.labels.source ?? "").toLowerCase();
  if (sourceLabel === "thanos" || sourceLabel === "prometheus") {
    return [
      "kubernetes-pod-cpu-range",
      "kubernetes-pod-memory-range",
      "kubernetes-pod-restarts-range",
      "kubernetes-jvm-heap-used-range",
      "kubernetes-jvm-heap-usage-pct-range",
      "oracle-tablespace-pct-used-instant",
      "oracle-tablespace-pct-used-range",
      "target-availability-instant"
    ];
  }
  return [
    "target-availability-instant",
    "kubernetes-pod-cpu-range",
    "kubernetes-pod-memory-range"
  ];
}

function recommendationGroupForSource(source: AlertSourceProfile): string {
  if (source.kind === "alertmanager") {
    return "Alertmanager active-alert intake";
  }
  if (alertSourceProfileLabelsIndicateThanosRule(source.labels)) {
    return "Thanos Rule active alerts";
  }
  const sourceLabel = String(source.labels.source ?? "").toLowerCase();
  if (sourceLabel === "thanos") {
    return "Thanos Query metric evidence";
  }
  if (sourceLabel === "prometheus") {
    return "Prometheus metric evidence";
  }
  return "Prometheus-compatible metric evidence";
}

export function diagnosisToolSupportsSourceKind(tool: DiagnosisToolKind, kind: AlertSourceKind): boolean {
  return diagnosisToolCompatibleSourceKinds(tool).includes(kind);
}

export function diagnosisToolSupportsSourceProfile(
  tool: DiagnosisToolKind,
  source: Pick<AlertSourceProfile, "kind" | "labels">
): boolean {
  if (alertSourceProfileLabelsIndicateThanosRule(source.labels)) {
    return tool === "active_alerts";
  }
  return diagnosisToolSupportsSourceKind(tool, source.kind);
}

export function diagnosisToolCoverage({
  sources,
  templates
}: {
  sources: AlertSourceProfile[];
  templates: DiagnosisToolTemplate[];
}): DiagnosisToolCoverage {
  const sourceByID = new Map(sources.map((source) => [source.id, source]));
  const enabled = templates.filter((template) => template.enabled);
  const usable = enabled.filter((template) => {
    const source = sourceByID.get(template.alert_source_profile_id);
    return source !== undefined && source.enabled && diagnosisToolSupportsSourceProfile(template.tool, source);
  });
  const activeAlerts = usable.filter((template) => template.tool === "active_alerts");
  const metrics = usable.filter((template) => template.tool === "metric_query" || template.tool === "metric_range_query");
  const rangeMetrics = usable.filter((template) => template.tool === "metric_range_query");
  const sourceNames = uniqueStrings(
    usable
      .map((template) => sourceByID.get(template.alert_source_profile_id)?.name ?? "")
      .filter((name) => name !== "")
  );

  if (enabled.length === 0) {
    return {
      activeAlertTemplates: 0,
      detail: "Enable active alert and metric templates before relying on AI follow-up.",
      enabledTemplates: 0,
      label: "No enabled evidence tools.",
      metricTemplates: 0,
      rangeMetricTemplates: 0,
      sourceNames,
      status: "pending"
    };
  }
  if (activeAlerts.length > 0 && metrics.length > 0) {
    return {
      activeAlertTemplates: activeAlerts.length,
      detail: "AI follow-up has active alert collection and metric evidence tools.",
      enabledTemplates: enabled.length,
      label: "Evidence coverage ready.",
      metricTemplates: metrics.length,
      rangeMetricTemplates: rangeMetrics.length,
      sourceNames,
      status: "ready"
    };
  }

  const missing = [
    activeAlerts.length === 0 ? "active alert collection" : "",
    metrics.length === 0 ? "metric evidence" : ""
  ].filter((item) => item !== "");
  return {
    activeAlertTemplates: activeAlerts.length,
    detail: `Missing ${missing.join(" and ")} for AI follow-up.`,
    enabledTemplates: enabled.length,
    label: "Evidence coverage needs review.",
    metricTemplates: metrics.length,
    rangeMetricTemplates: rangeMetrics.length,
    sourceNames,
    status: "review"
  };
}

export function diagnosisToolSourceReadiness({
  alertSourceProfileID,
  sources,
  tool
}: {
  alertSourceProfileID: number | null;
  sources: AlertSourceProfile[];
  tool: DiagnosisToolKind;
}): DiagnosisToolSourceReadiness {
  const requiredKinds = diagnosisToolCompatibleSourceKinds(tool);
  const compatibleSources = sources.filter((source) => diagnosisToolSupportsSourceProfile(tool, source));
  const enabledCompatibleSources = compatibleSources.filter((source) => source.enabled);
  const selectedSource =
    alertSourceProfileID === null ? null : sources.find((source) => source.id === alertSourceProfileID) ?? null;

  if (selectedSource !== null && !diagnosisToolSupportsSourceProfile(tool, selectedSource)) {
    const blocker = sourceProfileCompatibilityBlocker(tool, selectedSource);
    return {
      blockers: [blocker],
      compatibleSourceCount: enabledCompatibleSources.length,
      detail: blocker,
      label: "Selected source is incompatible.",
      requiredKinds,
      selectedSourceKind: selectedSource.kind,
      status: "blocked"
    };
  }
  if (selectedSource !== null && !selectedSource.enabled) {
    const blocker = "Bound alert source must be enabled before template enablement.";
    return {
      blockers: [blocker],
      compatibleSourceCount: enabledCompatibleSources.length,
      detail: blocker,
      label: "Selected source is disabled.",
      requiredKinds,
      selectedSourceKind: selectedSource.kind,
      status: "blocked"
    };
  }
  if (enabledCompatibleSources.length === 0) {
    const blocker =
      `${diagnosisToolKindLabels[tool]} needs an enabled ${sourceKindList(requiredKinds)} source before it can collect evidence.`;
    return {
      blockers: [blocker],
      compatibleSourceCount: 0,
      detail: blocker,
      label: "No enabled compatible alert source.",
      requiredKinds,
      selectedSourceKind: selectedSource?.kind ?? null,
      status: "blocked"
    };
  }
  if (selectedSource === null) {
    return {
      blockers: ["Select a compatible alert source."],
      compatibleSourceCount: enabledCompatibleSources.length,
      detail: `Select ${sourceKindList(requiredKinds)} for this tool.`,
      label: "Select a compatible source.",
      requiredKinds,
      selectedSourceKind: null,
      status: "pending"
    };
  }
  return {
    blockers: [],
    compatibleSourceCount: enabledCompatibleSources.length,
    detail:
      tool === "active_alerts"
        ? "Active alert collection can read Alertmanager or Prometheus-compatible alert APIs."
        : "Metric evidence runs against Prometheus-compatible query APIs, including Thanos Query endpoints; Thanos Rule active-alert sources are excluded.",
    label: "Source compatible.",
    requiredKinds,
    selectedSourceKind: selectedSource.kind,
    status: "ready"
  };
}

export function diagnosisToolSaveCompatibility({
  alertSourceProfileID,
  sources,
  tool
}: {
  alertSourceProfileID: number | null;
  sources: AlertSourceProfile[];
  tool: DiagnosisToolKind;
}): DiagnosisToolSaveCompatibility {
  const selectedSource =
    alertSourceProfileID === null ? null : sources.find((source) => source.id === alertSourceProfileID) ?? null;
  if (selectedSource !== null && !diagnosisToolSupportsSourceProfile(tool, selectedSource)) {
    return { ok: false, message: sourceProfileCompatibilityBlocker(tool, selectedSource) };
  }
  return { ok: true };
}

export function formStateToWriteRequest(
  form: DiagnosisToolTemplateFormState
): ParseResult<DiagnosisToolTemplateWriteRequest> {
  const name = form.name.trim();
  if (name === "") {
    return { ok: false, message: "Template name is required." };
  }
  if (name.length > 120) {
    return { ok: false, message: "Template name must be 120 characters or fewer." };
  }
  if (!positiveInteger(form.alertSourceProfileID)) {
    return { ok: false, message: "Select an alert source." };
  }
  if (!isDiagnosisToolKind(form.tool)) {
    return { ok: false, message: "Tool kind is unsupported." };
  }
  if (!positiveInteger(form.defaultLimit)) {
    return { ok: false, message: "Default limit must be a positive integer." };
  }

  const queryTemplate = form.queryTemplate.trim();
  if (queryTemplate.length > 500) {
    return { ok: false, message: "Query template must be 500 characters or fewer." };
  }
  if (/[\u0000-\u001f\u007f]/.test(queryTemplate)) {
    return { ok: false, message: "Query template must be a single line." };
  }
  const queryPreview = diagnosisQueryTemplatePreview(queryTemplate);
  if (!queryPreview.ok) {
    return { ok: false, message: queryPreview.message };
  }

  const range = parseRangeBounds(form);
  if (!range.ok) {
    return range;
  }

  if (form.tool === "active_alerts") {
    if (queryTemplate !== "" || !rangeFieldsAreZero(range.value)) {
      return { ok: false, message: "Active alert templates must not include a query, windows, or step." };
    }
    if (form.defaultLimit > 10) {
      return { ok: false, message: "Active alert templates support a default limit between 1 and 10." };
    }
  }

  if (form.tool === "metric_query") {
    if (queryTemplate === "") {
      return { ok: false, message: "Metric query templates require a query template." };
    }
    if (!rangeFieldsAreZero(range.value)) {
      return { ok: false, message: "Instant metric templates must not include windows or step." };
    }
    if (form.defaultLimit > 20) {
      return { ok: false, message: "Metric templates support a default limit between 1 and 20." };
    }
  }

  if (form.tool === "metric_range_query") {
    if (queryTemplate === "") {
      return { ok: false, message: "Range metric templates require a query template." };
    }
    if (form.defaultLimit > 20) {
      return { ok: false, message: "Metric templates support a default limit between 1 and 20." };
    }
    if (!integerInRange(range.value.defaultWindowSeconds, 15, 21600)) {
      return { ok: false, message: "Default window must be between 15 and 21600 seconds." };
    }
    if (!integerInRange(range.value.maxWindowSeconds, 15, 21600)) {
      return { ok: false, message: "Max window must be between 15 and 21600 seconds." };
    }
    if (range.value.maxWindowSeconds < range.value.defaultWindowSeconds) {
      return { ok: false, message: "Max window must be greater than or equal to default window." };
    }
    if (!integerInRange(range.value.defaultStepSeconds, 15, range.value.defaultWindowSeconds)) {
      return { ok: false, message: "Default step must be between 15 seconds and the default window." };
    }
  }

  return {
    ok: true,
    value: {
      name,
      alert_source_profile_id: form.alertSourceProfileID,
      tool: form.tool,
      query_template: queryTemplate,
      default_limit: form.defaultLimit,
      default_window_seconds: range.value.defaultWindowSeconds,
      max_window_seconds: range.value.maxWindowSeconds,
      default_step_seconds: range.value.defaultStepSeconds
    }
  };
}

export function diagnosisQueryTemplatePreview(queryTemplate: string): DiagnosisQueryTemplatePreview {
  const query = queryTemplate.trim();
  if (query === "") {
    return {
      ok: true,
      message: "No query template.",
      placeholders: [],
      previewQuery: ""
    };
  }
  if (!containsTemplateDelimiter(query)) {
    return {
      ok: true,
      message: "Static query template.",
      placeholders: [],
      previewQuery: query
    };
  }

  const matches = [...query.matchAll(diagnosisQueryPlaceholderPattern)];
  if (matches.length === 0) {
    return invalidQueryTemplatePreview(query);
  }

  const placeholders: string[] = [];
  let previewQuery = "";
  let last = 0;
  for (const match of matches) {
    const start = match.index ?? 0;
    const end = start + match[0].length;
    if (containsTemplateDelimiter(query.slice(last, start)) || !placeholderIsQuotedValue(query, start, end)) {
      return invalidQueryTemplatePreview(query);
    }
    const kind = match[1];
    const key = match[2];
    if (kind === undefined || key === undefined) {
      return invalidQueryTemplatePreview(query);
    }
    const placeholder = `${kind}.${key}`;
    placeholders.push(placeholder);
    previewQuery += query.slice(last, start);
    previewQuery += samplePlaceholderValue(key);
    last = end;
  }
  if (containsTemplateDelimiter(query.slice(last))) {
    return invalidQueryTemplatePreview(query);
  }
  previewQuery += query.slice(last);

  return {
    ok: true,
    message: "Parameterized query template.",
    placeholders: [...new Set(placeholders)],
    previewQuery
  };
}

type RangeBounds = {
  defaultWindowSeconds: number;
  maxWindowSeconds: number;
  defaultStepSeconds: number;
};

function parseRangeBounds(form: DiagnosisToolTemplateFormState): ParseResult<RangeBounds> {
  const bounds = {
    defaultWindowSeconds: form.defaultWindowSeconds,
    maxWindowSeconds: form.maxWindowSeconds,
    defaultStepSeconds: form.defaultStepSeconds
  };
  for (const [label, value] of Object.entries(bounds)) {
    if (!Number.isSafeInteger(value) || value === null || value < 0) {
      return { ok: false, message: `${rangeLabel(label)} must be a non-negative integer.` };
    }
  }
  return { ok: true, value: bounds as RangeBounds };
}

function rangeFieldsAreZero(bounds: RangeBounds): boolean {
  return bounds.defaultWindowSeconds === 0 && bounds.maxWindowSeconds === 0 && bounds.defaultStepSeconds === 0;
}

function isDiagnosisToolKind(value: string): value is DiagnosisToolKind {
  return value === "active_alerts" || value === "metric_query" || value === "metric_range_query";
}

function sourceKindLabel(kind: AlertSourceKind): string {
  switch (kind) {
    case "alertmanager":
      return "Alertmanager";
    case "prometheus":
      return "Prometheus-compatible";
  }
}

function sourceKindList(kinds: AlertSourceKind[]): string {
  return kinds.map(sourceKindLabel).join(" or ");
}

function sourceProfileCompatibilityBlocker(
  tool: DiagnosisToolKind,
  source: Pick<AlertSourceProfile, "kind" | "labels">
): string {
  if (alertSourceProfileLabelsIndicateThanosRule(source.labels) && tool !== "active_alerts") {
    return `${diagnosisToolKindLabels[tool]} cannot run against Thanos Rule active-alert sources. Use a Thanos Query or Prometheus metric source.`;
  }
  return `${diagnosisToolKindLabels[tool]} cannot run against ${sourceKindLabel(source.kind)}.`;
}

function alertSourceProfileLabelsIndicateThanosRule(
  labels: AlertSourceProfile["labels"] | null | undefined
): boolean {
  return (labels?.source ?? "").trim().toLowerCase() === "thanos-rule";
}

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values)];
}

function firstSearchParamValue(value: SearchParamValue): string | null {
  if (Array.isArray(value)) {
    return value[0]?.trim() || null;
  }
  return value?.trim() || null;
}

function positiveSearchParamInteger(value: string | null): number | null {
  if (value === null || !/^[1-9][0-9]*$/.test(value)) {
    return null;
  }
  const parsed = Number(value);
  return positiveInteger(parsed) ? parsed : null;
}

function positiveInteger(value: number | null): value is number {
  return Number.isSafeInteger(value) && value !== null && value > 0;
}

function integerInRange(value: number, min: number, max: number): boolean {
  return Number.isSafeInteger(value) && value >= min && value <= max;
}

function rangeLabel(key: string): string {
  switch (key) {
    case "defaultWindowSeconds":
      return "Default window";
    case "maxWindowSeconds":
      return "Max window";
    case "defaultStepSeconds":
      return "Default step";
    default:
      return "Range field";
  }
}

function containsTemplateDelimiter(value: string): boolean {
  return value.includes("{{") || value.includes("}}");
}

function placeholderIsQuotedValue(query: string, start: number, end: number): boolean {
  return start > 0 && end < query.length && query[start - 1] === "\"" && query[end] === "\"";
}

function samplePlaceholderValue(key: string): string {
  return `sample_${key.toLowerCase()}`;
}

function invalidQueryTemplatePreview(query: string): DiagnosisQueryTemplatePreview {
  return {
    ok: false,
    message: "Placeholders must use {{label.NAME}} or {{annotation.NAME}} inside quoted PromQL label values.",
    placeholders: [],
    previewQuery: query
  };
}
