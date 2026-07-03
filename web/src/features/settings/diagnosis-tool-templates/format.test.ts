import { describe, expect, it } from "vitest";

import {
  diagnosisToolCoverage,
  diagnosisToolSaveCompatibility,
  diagnosisToolSourceReadiness,
  diagnosisToolSupportsSourceProfile,
  diagnosisToolTemplateRecommendations,
  diagnosisQueryTemplatePreview,
  diagnosisToolTemplateLaunchHref,
  diagnosisToolTemplateLaunchIntentFromSearchParams,
  diagnosisToolTemplateLaunchIntentKey,
  diagnosisToolTemplatePresets,
  emptyDiagnosisToolTemplateForm,
  formStateToWriteRequest,
  presetToFormState,
  templateToFormState
} from "./format";
import type { AlertSourceKind, AlertSourceProfile } from "../alert-sources/types";
import type { DiagnosisToolKind, DiagnosisToolTemplate } from "./types";

describe("diagnosis tool template formatting", () => {
  it("builds write requests from range metric form state", () => {
    const parsed = formStateToWriteRequest({
      name: " CPU saturation range ",
      alertSourceProfileID: 1,
      tool: "metric_range_query",
      queryTemplate: " rate(container_cpu_usage_seconds_total[5m]) ",
      defaultLimit: 5,
      defaultWindowSeconds: 3600,
      maxWindowSeconds: 21600,
      defaultStepSeconds: 60
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        name: "CPU saturation range",
        alert_source_profile_id: 1,
        tool: "metric_range_query",
        query_template: "rate(container_cpu_usage_seconds_total[5m])",
        default_limit: 5,
        default_window_seconds: 3600,
        max_window_seconds: 21600,
        default_step_seconds: 60
      }
    });
  });

  it("rejects active alert templates with metric query fields", () => {
    const parsed = formStateToWriteRequest({
      ...emptyDiagnosisToolTemplateForm(),
      name: "Active alerts",
      alertSourceProfileID: 1,
      queryTemplate: "up"
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Active alert templates must not include a query, windows, or step."
    });
  });

  it("rejects range step values above the default window", () => {
    const parsed = formStateToWriteRequest({
      ...emptyDiagnosisToolTemplateForm(),
      name: "CPU range",
      alertSourceProfileID: 1,
      tool: "metric_range_query",
      queryTemplate: "up",
      defaultLimit: 5,
      defaultWindowSeconds: 60,
      maxWindowSeconds: 120,
      defaultStepSeconds: 120
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Default step must be between 15 seconds and the default window."
    });
  });

  it("previews parameterized query templates", () => {
    const preview = diagnosisQueryTemplatePreview(
      `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`
    );

    expect(preview).toEqual({
      ok: true,
      message: "Parameterized query template.",
      placeholders: ["label.ORACLE_SID", "label.TABLESPACE"],
      previewQuery:
        `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sample_oracle_sid",TABLESPACE="sample_tablespace"}`
    });
  });

  it("builds valid write requests from standard presets", () => {
    for (const preset of diagnosisToolTemplatePresets) {
      const form = presetToFormState(preset, 5);
      const parsed = formStateToWriteRequest(form);

      expect(parsed, preset.id).toMatchObject({ ok: true });
      if (parsed.ok) {
        expect(parsed.value.alert_source_profile_id).toBe(5);
      }
    }
  });

  it("keeps standard presets distinct and operator ready", () => {
    const ids = new Set(diagnosisToolTemplatePresets.map((preset) => preset.id));
    const labels = new Set(diagnosisToolTemplatePresets.map((preset) => preset.label));
    const activeAlerts = diagnosisToolTemplatePresets.find((preset) => preset.id === "active-alerts-current-source");
    const podCPU = diagnosisToolTemplatePresets.find((preset) => preset.id === "kubernetes-pod-cpu-range");
    const jvmHeapUsed = diagnosisToolTemplatePresets.find((preset) => preset.id === "kubernetes-jvm-heap-used-range");
    const jvmHeapPct = diagnosisToolTemplatePresets.find((preset) => preset.id === "kubernetes-jvm-heap-usage-pct-range");
    const oracle = diagnosisToolTemplatePresets.find((preset) => preset.id === "oracle-tablespace-pct-used-range");

    expect(ids.size).toBe(diagnosisToolTemplatePresets.length);
    expect(labels.size).toBe(diagnosisToolTemplatePresets.length);
    expect(activeAlerts?.form).toMatchObject({
      tool: "active_alerts",
      queryTemplate: "",
      defaultLimit: 10
    });
    expect(podCPU?.form.queryTemplate).toContain(`{{label.namespace}}`);
    expect(podCPU?.form.queryTemplate).toContain(`{{label.pod}}`);
    expect(jvmHeapUsed?.form.queryTemplate).toContain("jvm_memory_used_bytes");
    expect(jvmHeapUsed?.form.queryTemplate).toContain(`namespace="{{label.namespace}}"`);
    expect(jvmHeapUsed?.form.queryTemplate).toContain(`kubernetes_pod_name=~"{{label.pod}}"`);
    expect(jvmHeapPct?.form.queryTemplate).toContain("jvm_memory_used_bytes");
    expect(jvmHeapPct?.form.queryTemplate).toContain("jvm_memory_max_bytes");
    expect(jvmHeapPct?.form.queryTemplate).toContain(`area="heap"`);
    expect(oracle?.form.queryTemplate).toContain(`{{label.ORACLE_SID}}`);
    expect(oracle?.form.queryTemplate).toContain(`{{label.TABLESPACE}}`);
  });

  it("recommends source-aware templates for alert and metric evidence", () => {
    const recommendations = diagnosisToolTemplateRecommendations([
      alertSourceProfile(1, "alertmanager", true),
      thanosRuleSourceProfile(2),
      alertSourceProfile(3, "prometheus", true, { source: "thanos" }),
      alertSourceProfile(4, "prometheus", false, { source: "prometheus" })
    ]);

    expect(
      recommendations.map((recommendation) => [
        recommendation.sourceID,
        recommendation.group,
        recommendation.presetID
      ])
    ).toEqual([
      [1, "Alertmanager active-alert intake", "active-alerts-current-source"],
      [2, "Thanos Rule active alerts", "active-alerts-current-source"],
      [3, "Thanos Query metric evidence", "kubernetes-pod-cpu-range"],
      [3, "Thanos Query metric evidence", "kubernetes-pod-memory-range"],
      [3, "Thanos Query metric evidence", "kubernetes-pod-restarts-range"],
      [3, "Thanos Query metric evidence", "kubernetes-jvm-heap-used-range"],
      [3, "Thanos Query metric evidence", "kubernetes-jvm-heap-usage-pct-range"],
      [3, "Thanos Query metric evidence", "oracle-tablespace-pct-used-instant"],
      [3, "Thanos Query metric evidence", "oracle-tablespace-pct-used-range"],
      [3, "Thanos Query metric evidence", "target-availability-instant"]
    ]);
    expect(recommendations.every((recommendation) => recommendation.id.includes(":"))).toBe(true);
    expect(recommendations.some((recommendation) => recommendation.sourceID === 4)).toBe(false);
  });

  it("parses launch intents for overview-driven template setup", () => {
    expect(
      diagnosisToolTemplateLaunchIntentFromSearchParams({
        intent: "active-alert-tool",
        source_id: "3"
      })
    ).toMatchObject({
      alertSourceProfileID: 3,
      message: "Prepared current active alerts from the settings overview action.",
      presetID: "active-alerts-current-source"
    });
    expect(
      diagnosisToolTemplateLaunchIntentFromSearchParams({
        intent: "metric-evidence-tool"
      })
    ).toMatchObject({
      alertSourceProfileID: null,
      message: "Prepared Kubernetes pod CPU range from the settings overview action.",
      presetID: "kubernetes-pod-cpu-range"
    });
    expect(
      diagnosisToolTemplateLaunchIntentFromSearchParams({
        preset: "oracle-tablespace-pct-used-range",
        source_id: "not-a-number"
      })
    ).toMatchObject({
      alertSourceProfileID: null,
      message: "Prepared Oracle tablespace pct used range from the URL preset.",
      presetID: "oracle-tablespace-pct-used-range"
    });
    expect(diagnosisToolTemplateLaunchIntentFromSearchParams({ intent: "unknown" })).toBeNull();
  });

  it("builds stable launch hrefs and keys", () => {
    const activeHref = diagnosisToolTemplateLaunchHref({ intent: "active-alert-tool", sourceID: 3 });
    const metricHref = diagnosisToolTemplateLaunchHref({ intent: "metric-evidence-tool" });
    const workflowHref = diagnosisToolTemplateLaunchHref({
      intent: "metric-evidence-tool",
      sourceID: 5,
      workflowReturn: { sourceID: 3 }
    });

    expect(activeHref).toBe("/settings/diagnosis-tool-templates?intent=active-alert-tool&source_id=3");
    expect(metricHref).toBe("/settings/diagnosis-tool-templates?intent=metric-evidence-tool");
    expect(workflowHref).toBe(
      "/settings/diagnosis-tool-templates?intent=metric-evidence-tool&source_id=5&workflow_return=auto-room-enable&workflow_source_id=3"
    );
    expect(
      diagnosisToolTemplateLaunchIntentFromSearchParams({
        intent: "metric-evidence-tool",
        source_id: "5",
        workflow_return: "auto-room-enable",
        workflow_source_id: "3"
      })?.workflowReturn
    ).toMatchObject({
      href: "/settings/report-workflow-policies?intent=enable-ai-room-follow-up&source_id=3",
      label: "Back to workflow",
      sourceID: 3
    });
    expect(
      diagnosisToolTemplateLaunchIntentKey(
        diagnosisToolTemplateLaunchIntentFromSearchParams({ intent: "active-alert-tool", source_id: "3" })
      )
    ).toBe("active-alerts-current-source:3:none");
    expect(diagnosisToolTemplateLaunchIntentKey(null)).toBe("default");
  });

  it("reports metric source compatibility from Prometheus-compatible profiles", () => {
    const readiness = diagnosisToolSourceReadiness({
      alertSourceProfileID: 2,
      sources: [alertSourceProfile(1, "alertmanager"), alertSourceProfile(2, "prometheus")],
      tool: "metric_range_query"
    });

    expect(readiness).toMatchObject({
      compatibleSourceCount: 1,
      label: "Source compatible.",
      selectedSourceKind: "prometheus",
      status: "ready"
    });
  });

  it("blocks metric evidence templates on Thanos Rule active-alert sources", () => {
    const readiness = diagnosisToolSourceReadiness({
      alertSourceProfileID: 2,
      sources: [
        alertSourceProfile(1, "prometheus"),
        thanosRuleSourceProfile(2)
      ],
      tool: "metric_range_query"
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Range metric cannot run against Thanos Rule active-alert sources. Use a Thanos Query or Prometheus metric source."
      ],
      compatibleSourceCount: 1,
      label: "Selected source is incompatible.",
      selectedSourceKind: "prometheus",
      status: "blocked"
    });
    expect(
      diagnosisToolSaveCompatibility({
        alertSourceProfileID: 2,
        sources: [
          alertSourceProfile(1, "prometheus"),
          thanosRuleSourceProfile(2)
        ],
        tool: "metric_query"
      })
    ).toEqual({
      ok: false,
      message:
        "Instant metric cannot run against Thanos Rule active-alert sources. Use a Thanos Query or Prometheus metric source."
    });
  });

  it("reports pending evidence coverage when no templates are enabled", () => {
    expect(
      diagnosisToolCoverage({
        sources: [alertSourceProfile(1, "prometheus")],
        templates: [diagnosisToolTemplate(1, 1, "active_alerts", false)]
      })
    ).toEqual({
      activeAlertTemplates: 0,
      detail: "Enable active alert and metric templates before relying on AI follow-up.",
      enabledTemplates: 0,
      label: "No enabled evidence tools.",
      metricTemplates: 0,
      rangeMetricTemplates: 0,
      sourceNames: [],
      status: "pending"
    });
  });

  it("reports review evidence coverage when metric tools are missing", () => {
    expect(
      diagnosisToolCoverage({
        sources: [alertSourceProfile(1, "alertmanager")],
        templates: [diagnosisToolTemplate(1, 1, "active_alerts", true)]
      })
    ).toMatchObject({
      activeAlertTemplates: 1,
      detail: "Missing metric evidence for AI follow-up.",
      enabledTemplates: 1,
      metricTemplates: 0,
      sourceNames: ["Source 1"],
      status: "review"
    });
  });

  it("reports ready evidence coverage when active alert and metric tools are enabled", () => {
    expect(
      diagnosisToolCoverage({
        sources: [alertSourceProfile(1, "alertmanager"), alertSourceProfile(2, "prometheus")],
        templates: [
          diagnosisToolTemplate(1, 1, "active_alerts", true),
          diagnosisToolTemplate(2, 2, "metric_range_query", true)
        ]
      })
    ).toMatchObject({
      activeAlertTemplates: 1,
      detail: "AI follow-up has active alert collection and metric evidence tools.",
      enabledTemplates: 2,
      label: "Evidence coverage ready.",
      metricTemplates: 1,
      rangeMetricTemplates: 1,
      sourceNames: ["Source 1", "Source 2"],
      status: "ready"
    });
  });

  it("does not count Thanos Rule metric templates as usable metric coverage", () => {
    expect(
      diagnosisToolCoverage({
        sources: [thanosRuleSourceProfile(1), alertSourceProfile(2, "prometheus")],
        templates: [
          diagnosisToolTemplate(1, 1, "active_alerts", true),
          diagnosisToolTemplate(2, 1, "metric_range_query", true)
        ]
      })
    ).toMatchObject({
      activeAlertTemplates: 1,
      detail: "Missing metric evidence for AI follow-up.",
      metricTemplates: 0,
      status: "review"
    });
  });

  it("blocks metric tools on Alertmanager profiles", () => {
    const readiness = diagnosisToolSourceReadiness({
      alertSourceProfileID: 1,
      sources: [alertSourceProfile(1, "alertmanager"), alertSourceProfile(2, "prometheus")],
      tool: "metric_query"
    });

    expect(readiness).toMatchObject({
      label: "Selected source is incompatible.",
      selectedSourceKind: "alertmanager",
      status: "blocked"
    });
  });

  it("rejects saving templates with incompatible selected source kinds", () => {
    const readiness = diagnosisToolSaveCompatibility({
      alertSourceProfileID: 1,
      sources: [alertSourceProfile(1, "alertmanager"), alertSourceProfile(2, "prometheus")],
      tool: "metric_query"
    });

    expect(readiness).toEqual({
      ok: false,
      message: "Instant metric cannot run against Alertmanager."
    });
  });

  it("allows saving compatible templates on disabled sources as drafts", () => {
    const readiness = diagnosisToolSaveCompatibility({
      alertSourceProfileID: 1,
      sources: [alertSourceProfile(1, "alertmanager", false)],
      tool: "active_alerts"
    });

    expect(readiness).toEqual({ ok: true });
  });

  it("blocks template enablement on disabled compatible source profiles", () => {
    const readiness = diagnosisToolSourceReadiness({
      alertSourceProfileID: 2,
      sources: [alertSourceProfile(1, "alertmanager"), alertSourceProfile(2, "prometheus", false)],
      tool: "metric_query"
    });

    expect(readiness).toMatchObject({
      blockers: ["Bound alert source must be enabled before template enablement."],
      compatibleSourceCount: 0,
      label: "Selected source is disabled.",
      selectedSourceKind: "prometheus",
      status: "blocked"
    });
  });

  it("allows active alerts on Alertmanager and Prometheus-compatible profiles", () => {
    expect(
      diagnosisToolSourceReadiness({
        alertSourceProfileID: 1,
        sources: [alertSourceProfile(1, "alertmanager"), thanosRuleSourceProfile(2)],
        tool: "active_alerts"
      })
    ).toMatchObject({ compatibleSourceCount: 2, status: "ready" });
    expect(diagnosisToolSupportsSourceProfile("active_alerts", thanosRuleSourceProfile(2))).toBe(true);
    expect(diagnosisToolSupportsSourceProfile("metric_query", thanosRuleSourceProfile(2))).toBe(false);
  });

  it("rejects unquoted parameterized query templates", () => {
    const preview = diagnosisQueryTemplatePreview(`up{job={{label.job}}}`);

    expect(preview).toMatchObject({
      ok: false,
      message: "Placeholders must use {{label.NAME}} or {{annotation.NAME}} inside quoted PromQL label values."
    });
  });

  it("rejects invalid placeholders before submit", () => {
    const parsed = formStateToWriteRequest({
      ...emptyDiagnosisToolTemplateForm(),
      name: "Bad query",
      alertSourceProfileID: 1,
      tool: "metric_query",
      queryTemplate: `up{job={{label.job}}}`
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Placeholders must use {{label.NAME}} or {{annotation.NAME}} inside quoted PromQL label values."
    });
  });

  it("maps templates back to form state", () => {
    expect(
      templateToFormState({
        id: 1,
        name: "CPU range",
        alert_source_profile_id: 1,
        tool: "metric_range_query",
        query_template: "up",
        default_limit: 5,
        default_window_seconds: 3600,
        max_window_seconds: 21600,
        default_step_seconds: 60,
        enabled: false,
        enabled_at: null,
        disabled_at: null,
        created_at: "2026-06-08T08:00:00Z",
        updated_at: "2026-06-08T08:00:00Z"
      })
    ).toMatchObject({
      name: "CPU range",
      alertSourceProfileID: 1,
      tool: "metric_range_query",
      defaultWindowSeconds: 3600
    });
  });
});

function alertSourceProfile(
  id: number,
  kind: AlertSourceKind,
  enabled = true,
  labels: AlertSourceProfile["labels"] = {}
): AlertSourceProfile {
  return {
    auth_mode: "none",
    base_url: `https://source-${id}.example.test`,
    created_at: "2026-06-08T08:00:00Z",
    enabled,
    id,
    kind,
    labels,
    name: `Source ${id}`,
    secret_ref: "",
    updated_at: "2026-06-08T08:00:00Z"
  };
}

function thanosRuleSourceProfile(id: number, enabled = true): AlertSourceProfile {
  return alertSourceProfile(id, "prometheus", enabled, { source: "thanos-rule" });
}

function diagnosisToolTemplate(
  id: number,
  sourceID: number,
  tool: DiagnosisToolKind,
  enabled: boolean
): DiagnosisToolTemplate {
  return {
    id,
    name: `Template ${id}`,
    alert_source_profile_id: sourceID,
    tool,
    query_template: tool === "active_alerts" ? "" : "up",
    default_limit: 5,
    default_window_seconds: tool === "metric_range_query" ? 3600 : 0,
    max_window_seconds: tool === "metric_range_query" ? 21600 : 0,
    default_step_seconds: tool === "metric_range_query" ? 60 : 0,
    enabled,
    enabled_at: enabled ? "2026-06-08T08:00:00Z" : null,
    disabled_at: enabled ? null : "2026-06-08T08:00:00Z",
    created_at: "2026-06-08T08:00:00Z",
    updated_at: "2026-06-08T08:00:00Z"
  };
}
