import { describe, expect, it } from "vitest";

import {
  reportEvidenceCollectionResultForRequest,
  reportEvidenceRequestDetail,
  reportEvidenceRequestKey,
  type ReportEvidenceRequestDisplay,
} from "./report-evidence-display";

describe("report evidence display", () => {
  it("shows alert source profile metadata on requested evidence", () => {
    expect(
      reportEvidenceRequestDetail({
        alert_source_profile_id: 7,
        limit: 5,
        query: "histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
        reason: "Need checkout deployment timing.",
        step_seconds: 60,
        template_id: 88,
        tool: "metric_range_query",
        window_seconds: 1800,
      }),
    ).toBe(
      "query: histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m])) / template #88 / source #7 / window 1800s / step 60s / limit 5",
    );
  });

  it("keeps requested evidence keys distinct by alert source profile", () => {
    const request: ReportEvidenceRequestDisplay = {
      limit: 5,
      query: "sum(rate(checkout_requests_total[5m]))",
      reason: "Need the deployment window.",
      tool: "metric_range_query",
    };

    expect(
      reportEvidenceRequestKey({
        ...request,
        alert_source_profile_id: 7,
      }),
    ).not.toBe(
      reportEvidenceRequestKey({
        ...request,
        alert_source_profile_id: 8,
      }),
    );
  });

  it("matches collected results back to the requested evidence identity", () => {
    const request: ReportEvidenceRequestDisplay = {
      alert_source_profile_id: 7,
      limit: 5,
      query: "sum(rate(checkout_requests_total[5m]))",
      reason: "Need the deployment window.",
      step_seconds: 60,
      tool: "metric_range_query",
      window_seconds: 1800,
    };

    expect(
      reportEvidenceCollectionResultForRequest(request, [
        {
          alert_source_profile_id: 8,
          limit: 5,
          query: "sum(rate(checkout_requests_total[5m]))",
          request_reason: "Need the deployment window.",
          status: "collected",
          tool: "metric_range_query",
        },
        {
          alert_source_profile_id: 7,
          limit: 5,
          query: "sum(rate(checkout_requests_total[5m]))",
          request_reason: "Need the deployment window.",
          step_seconds: 60,
          status: "collected",
          tool: "metric_range_query",
          window_seconds: 1800,
        },
      ]),
    ).toMatchObject({
      alert_source_profile_id: 7,
      status: "collected",
    });
  });

  it("matches collected results with backend-enriched optional metadata", () => {
    const request: ReportEvidenceRequestDisplay = {
      alert_source_profile_id: 7,
      limit: 5,
      query: "sum(rate(checkout_requests_total[5m]))",
      reason: "Need the deployment window.",
      step_seconds: 60,
      tool: "metric_range_query",
      window_seconds: 1800,
    };
    const result = {
      alert_source_profile_id: 7,
      limit: 5,
      query: "sum(rate(checkout_requests_total[5m]))",
      request_reason: "Need the deployment window.",
      status: "collected",
      step_seconds: 60,
      template_id: 88,
      tool: "metric_range_query",
      window_seconds: 1800,
    };

    expect(reportEvidenceCollectionResultForRequest(request, [result])).toEqual(result);
    expect(
      reportEvidenceCollectionResultForRequest(
        {
          ...request,
          template_id: 77,
        },
        [result],
      ),
    ).toBeUndefined();
  });
});
