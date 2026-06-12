import { describe, expect, it } from "vitest";

import {
  emptyDiagnosisToolTemplateForm,
  formStateToWriteRequest,
  templateToFormState
} from "./format";

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
