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

type DiagnosisToolSourceReadinessStatus = "ready" | "pending" | "blocked";

export type DiagnosisToolSourceReadiness = {
  compatibleSourceCount: number;
  detail: string;
  label: string;
  requiredKinds: AlertSourceKind[];
  selectedSourceKind: AlertSourceKind | null;
  status: DiagnosisToolSourceReadinessStatus;
};

const diagnosisQueryPlaceholderPattern = /\{\{\s*(label|annotation)\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}/g;
const labelMatcherPrefixPattern = /(?:^|[,{])\s*[A-Za-z_][A-Za-z0-9_]*\s*(=|!=|=~|!~)\s*$/;
const oracleTablespacePctUsedQuery =
  `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`;

export const diagnosisToolKindLabels: Record<DiagnosisToolKind, string> = {
  active_alerts: "Active alerts",
  metric_query: "Instant metric",
  metric_range_query: "Range metric"
};

export const diagnosisToolTemplatePresets: DiagnosisToolTemplatePreset[] = [
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

export function diagnosisToolSupportsSourceKind(tool: DiagnosisToolKind, kind: AlertSourceKind): boolean {
  return diagnosisToolCompatibleSourceKinds(tool).includes(kind);
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
  const compatibleSources = sources.filter((source) => diagnosisToolSupportsSourceKind(tool, source.kind));
  const selectedSource =
    alertSourceProfileID === null ? null : sources.find((source) => source.id === alertSourceProfileID) ?? null;

  if (compatibleSources.length === 0) {
    return {
      compatibleSourceCount: 0,
      detail: `${diagnosisToolKindLabels[tool]} needs ${sourceKindList(requiredKinds)} before it can collect evidence.`,
      label: "No compatible alert source.",
      requiredKinds,
      selectedSourceKind: selectedSource?.kind ?? null,
      status: "blocked"
    };
  }
  if (selectedSource === null) {
    return {
      compatibleSourceCount: compatibleSources.length,
      detail: `Select ${sourceKindList(requiredKinds)} for this tool.`,
      label: "Select a compatible source.",
      requiredKinds,
      selectedSourceKind: null,
      status: "pending"
    };
  }
  if (!diagnosisToolSupportsSourceKind(tool, selectedSource.kind)) {
    return {
      compatibleSourceCount: compatibleSources.length,
      detail: `${diagnosisToolKindLabels[tool]} cannot run against ${sourceKindLabel(selectedSource.kind)}.`,
      label: "Selected source is incompatible.",
      requiredKinds,
      selectedSourceKind: selectedSource.kind,
      status: "blocked"
    };
  }
  return {
    compatibleSourceCount: compatibleSources.length,
    detail:
      tool === "active_alerts"
        ? "Active alert collection can read Alertmanager or Prometheus-compatible alert APIs."
        : "Metric evidence runs against Prometheus-compatible query APIs, including Thanos query endpoints.",
    label: "Source compatible.",
    requiredKinds,
    selectedSourceKind: selectedSource.kind,
    status: "ready"
  };
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

  const matches = Array.from(query.matchAll(diagnosisQueryPlaceholderPattern));
  if (matches.length === 0) {
    return invalidQueryTemplatePreview(query);
  }

  const placeholders: string[] = [];
  const seenPlaceholders = new Set<string>();
  let lastIndex = 0;
  let previewQuery = "";
  for (const match of matches) {
    if (match.index === undefined) {
      return invalidQueryTemplatePreview(query);
    }
    if (containsTemplateDelimiter(query.slice(lastIndex, match.index))) {
      return invalidQueryTemplatePreview(query);
    }
    const endIndex = match.index + match[0].length;
    if (!placeholderIsQuotedValue(query, match.index, endIndex)) {
      return invalidQueryTemplatePreview(query);
    }
    const kind = match[1];
    const key = match[2];
    if (kind === undefined || key === undefined) {
      return invalidQueryTemplatePreview(query);
    }
    const placeholder = `${kind}.${key}`;
    if (!seenPlaceholders.has(placeholder)) {
      placeholders.push(placeholder);
      seenPlaceholders.add(placeholder);
    }
    previewQuery += query.slice(lastIndex, match.index);
    previewQuery += samplePlaceholderValue(key);
    lastIndex = endIndex;
  }
  if (containsTemplateDelimiter(query.slice(lastIndex))) {
    return invalidQueryTemplatePreview(query);
  }
  previewQuery += query.slice(lastIndex);
  return {
    ok: true,
    message: "Parameterized query template.",
    placeholders,
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
  const matcher = labelMatcherPrefixPattern.exec(query.slice(0, start - 1));
  return (
    start > 0 &&
    end < query.length &&
    query[start - 1] === `"` &&
    query[end] === `"` &&
    matcher !== null &&
    (matcher[1] === "=" || matcher[1] === "!=")
  );
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
