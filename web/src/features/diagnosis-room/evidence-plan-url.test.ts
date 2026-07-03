import { describe, expect, it } from "vitest";

import {
  diagnosisEvidencePlanIdentity,
  diagnosisEvidencePlanSearchParam,
  diagnosisEvidencePlanURLQuery,
} from "./evidence-plan-url";

describe("diagnosis evidence plan URL", () => {
  it("round-trips executable metric range evidence plans", () => {
    const query = diagnosisEvidencePlanURLQuery({
      alert_source_profile_id: 7,
      limit: 5,
      query: "histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
      reason: "Need checkout deployment timing.",
      step_seconds: 60,
      template_id: 88,
      tool: "metric_range_query",
      window_seconds: 1800,
    });

    expect(query).toEqual({
      evidence_limit: "5",
      evidence_query:
        "histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
      evidence_reason: "Need checkout deployment timing.",
      evidence_source_profile_id: "7",
      evidence_step_seconds: "60",
      evidence_template_id: "88",
      evidence_tool: "metric_range_query",
      evidence_window_seconds: "1800",
    });
    expect(diagnosisEvidencePlanSearchParam(new URLSearchParams(query))).toEqual({
      alert_source_profile_id: 7,
      limit: 5,
      query: "histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
      reason: "Need checkout deployment timing.",
      step_seconds: 60,
      template_id: 88,
      tool: "metric_range_query",
      window_seconds: 1800,
    });
  });

  it("rejects invalid or non-executable query parameter combinations", () => {
    expect(
      diagnosisEvidencePlanSearchParam(
        new URLSearchParams({
          evidence_query: "up",
          evidence_reason: "Need current availability.",
          evidence_tool: "unknown",
        }),
      ),
    ).toBeUndefined();
    expect(
      diagnosisEvidencePlanSearchParam(
        new URLSearchParams({
          evidence_reason: "Need current availability.",
          evidence_tool: "metric_query",
        }),
      ),
    ).toBeUndefined();
    expect(
      diagnosisEvidencePlanSearchParam(
        new URLSearchParams({
          evidence_query: "up",
          evidence_reason: "Need current sibling alerts.",
          evidence_tool: "active_alerts",
        }),
      ),
    ).toBeUndefined();
  });

  it("keeps evidence plan identities distinct by source profile", () => {
    const base = {
      limit: 5,
      reason: "Need active sibling alerts.",
      tool: "active_alerts",
    };

    expect(
      diagnosisEvidencePlanIdentity({
        ...base,
        alert_source_profile_id: 7,
      }),
    ).not.toBe(
      diagnosisEvidencePlanIdentity({
        ...base,
        alert_source_profile_id: 8,
      }),
    );
  });
});
