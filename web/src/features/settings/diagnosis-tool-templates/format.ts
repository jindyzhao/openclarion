import type {
  DiagnosisToolKind,
  DiagnosisToolTemplate,
  DiagnosisToolTemplateFormState,
  DiagnosisToolTemplateWriteRequest
} from "./types";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

export const diagnosisToolKindLabels: Record<DiagnosisToolKind, string> = {
  active_alerts: "Active alerts",
  metric_query: "Instant metric",
  metric_range_query: "Range metric"
};

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
    return { ok: false, message: "Alert source ID must be a positive integer." };
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
