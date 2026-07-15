import { describe, expect, it } from "vitest";

import {
  operatorEvidenceRangeValues,
  operatorEvidenceTemplateHasParameterizedQuery,
  operatorEvidenceTemplateQuery,
  operatorEvidenceTemplateSourceDisabledReason,
  safeOperatorEvidencePlaceholderValue,
} from "./operator-evidence";
import type { DiagnosisToolKind, DiagnosisToolTemplate } from "@/features/settings/diagnosis-tool-templates/types";

describe("operator evidence template query", () => {
  it("orders range values by stable field kind instead of display labels", () => {
    expect(operatorEvidenceRangeValues("window", 120, 60)).toEqual({
      stepSeconds: 60,
      windowSeconds: 120,
    });
    expect(operatorEvidenceRangeValues("step", 60, 120)).toEqual({
      stepSeconds: 60,
      windowSeconds: 120,
    });
  });

  it("expands safe alert labels and annotations into parameterized templates", () => {
    const result = operatorEvidenceTemplateQuery(
      diagnosisToolTemplate({
        query_template:
          `up{namespace="{{label.namespace}}",summary="{{annotation.summary}}"}`,
      }),
      {
        alert: {
          annotations: { summary: "Checkout latency high" },
          labels: { namespace: "xk-prod" },
        },
      },
    );

    expect(result).toEqual({
      missing: [],
      query: `up{namespace="xk-prod",summary="Checkout latency high"}`,
    });
  });

  it("reports missing placeholders without producing a runnable query", () => {
    const result = operatorEvidenceTemplateQuery(
      diagnosisToolTemplate({
        query_template: `up{namespace="{{label.namespace}}"}`,
      }),
      { alert: { annotations: {}, labels: {} } },
    );

    expect(result).toEqual({
      missing: ["label.namespace"],
      query: `up{namespace=""}`,
    });
  });

  it("rejects unsafe placeholder values before building recommended queries", () => {
    const unsafeValues = [
      `xk"prod`,
      `xk\\prod`,
      "xk\nprod",
      "a".repeat(201),
    ];

    for (const value of unsafeValues) {
      expect(safeOperatorEvidencePlaceholderValue(value)).toBe(false);
      expect(
        operatorEvidenceTemplateQuery(
          diagnosisToolTemplate({
            query_template: `up{namespace="{{label.namespace}}"}`,
          }),
          { alert: { annotations: {}, labels: { namespace: value } } },
        ),
      ).toEqual({
        missing: ["label.namespace (unsafe)"],
        query: `up{namespace=""}`,
      });
    }
  });

  it("detects only non-active templates with template delimiters as parameterized", () => {
    expect(
      operatorEvidenceTemplateHasParameterizedQuery(
        diagnosisToolTemplate({
          query_template: `up{namespace="{{label.namespace}}"}`,
          tool: "metric_query",
        }),
      ),
    ).toBe(true);
    expect(
      operatorEvidenceTemplateHasParameterizedQuery(
        diagnosisToolTemplate({ query_template: "", tool: "active_alerts" }),
      ),
    ).toBe(false);
  });

  it("disables templates outside the current alert source scope", () => {
    expect(
      operatorEvidenceTemplateSourceDisabledReason(
        diagnosisToolTemplate({ query_template: "", sourceProfileID: 2 }),
        7,
      ),
    ).toBe("Template source #2 is outside the current alert source #7.");

    expect(
      operatorEvidenceTemplateSourceDisabledReason(
        diagnosisToolTemplate({ query_template: "", sourceProfileID: 7 }),
        7,
      ),
    ).toBe("");

    expect(
      operatorEvidenceTemplateSourceDisabledReason(
        diagnosisToolTemplate({ query_template: "", sourceProfileID: 2 }),
        0,
      ),
    ).toBe("");
  });
});

function diagnosisToolTemplate({
  query_template,
  sourceProfileID = 2,
  tool = "metric_range_query",
}: {
  query_template: string;
  sourceProfileID?: number;
  tool?: DiagnosisToolKind;
}): DiagnosisToolTemplate {
  return {
    alert_source_profile_id: sourceProfileID,
    created_at: "2026-06-20T08:00:00Z",
    default_limit: 5,
    default_step_seconds: tool === "metric_range_query" ? 60 : 0,
    default_window_seconds: tool === "metric_range_query" ? 3600 : 0,
    disabled_at: null,
    enabled: true,
    enabled_at: "2026-06-20T08:00:00Z",
    id: 11,
    max_window_seconds: tool === "metric_range_query" ? 21600 : 0,
    name: "Namespace query",
    query_template,
    tool,
    updated_at: "2026-06-20T08:00:00Z",
  };
}
