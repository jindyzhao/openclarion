import { describe, expect, it } from "vitest";

import {
  diagnosisToolSourceReadiness,
  diagnosisQueryTemplatePreview,
  diagnosisToolTemplatePresets,
  emptyDiagnosisToolTemplateForm,
  formStateToWriteRequest,
  presetToFormState,
  templateToFormState
} from "./format";
import type { AlertSourceKind, AlertSourceProfile } from "../alert-sources/types";

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
      previewQuery: `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sample_oracle_sid",TABLESPACE="sample_tablespace"}`
    });
  });

  it("builds valid write requests from standard presets", () => {
    for (const preset of diagnosisToolTemplatePresets) {
      const form = presetToFormState(preset, 5);
      const parsed = formStateToWriteRequest(form);

      expect(parsed, preset.id).toMatchObject({ ok: true });
      if (parsed.ok) {
        expect(parsed.value.alert_source_profile_id).toBe(5);
        expect(parsed.value.query_template).toBe(
          `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`
        );
      }
    }
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

  it("allows active alerts on Alertmanager and Prometheus-compatible profiles", () => {
    expect(
      diagnosisToolSourceReadiness({
        alertSourceProfileID: 1,
        sources: [alertSourceProfile(1, "alertmanager"), alertSourceProfile(2, "prometheus")],
        tool: "active_alerts"
      })
    ).toMatchObject({ compatibleSourceCount: 2, status: "ready" });
  });

  it("rejects invalid placeholder placement", () => {
    const preview = diagnosisQueryTemplatePreview(`up{job={{label.job}}}`);

    expect(preview).toMatchObject({
      ok: false,
      message: "Placeholders must use {{label.NAME}} or {{annotation.NAME}} inside quoted PromQL label values."
    });
  });

  it("rejects placeholders outside label matchers", () => {
    const preview = diagnosisQueryTemplatePreview(`label_replace(up, "dst", "{{label.job}}", "src", "(.*)")`);

    expect(preview).toMatchObject({
      ok: false,
      message: "Placeholders must use {{label.NAME}} or {{annotation.NAME}} inside quoted PromQL label values."
    });
  });

  it("rejects placeholders in regex matchers", () => {
    const preview = diagnosisQueryTemplatePreview(`up{job=~"{{label.job}}"}`);

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

function alertSourceProfile(id: number, kind: AlertSourceKind): AlertSourceProfile {
  return {
    auth_mode: "none",
    base_url: `https://source-${id}.example.test`,
    created_at: "2026-06-08T08:00:00Z",
    enabled: true,
    id,
    kind,
    labels: {},
    name: `Source ${id}`,
    secret_ref: "",
    updated_at: "2026-06-08T08:00:00Z"
  };
}
