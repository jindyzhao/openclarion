import { afterEach, describe, expect, it, vi } from "vitest";

import { previewReportWorkflowPolicyDraftImpactAction } from "./client-api";
import type { ReportWorkflowPolicyWriteRequest } from "./types";

describe("report workflow policy client API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("posts draft impact preview requests to the same-origin route", async () => {
    const body: ReportWorkflowPolicyWriteRequest = {
      name: "Automatic diagnosis workflow",
      alert_source_profile_id: 1,
      grouping_policy_id: 2,
      report_notification_channel_profile_id: 3,
      trigger_mode: "manual_replay",
      report_scenario: "single_alert",
      diagnosis_follow_up: "auto_room"
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              policy_id: 0,
              status: "ready",
              reason_codes: ["ok"],
              message: "Report workflow policy impact preview is ready.",
              checked_at: "2026-06-05T10:00:00Z",
              trigger_mode: "manual_replay",
              report_scenario: "single_alert",
              diagnosis_follow_up: "auto_room",
              alert_source_profile_id: 1,
              alert_source_kind: "alertmanager",
              alert_source_auth_mode: "none",
              alert_source_enabled: true,
              grouping_policy_id: 2,
              grouping_policy_enabled: true,
              grouping_dimension_keys: ["alertname"],
              grouping_severity_key: "severity",
              grouping_source_filter: ["alertmanager"],
              report_notification_channel_profile_id: 3,
              report_notification_channel_bound: true,
              report_notification_channel_enabled: true,
              report_notification_channel_has_report_scope: true,
              report_notification_channel_has_diagnosis_consultation_scope: true,
              report_notification_channel_has_diagnosis_close_scope: true,
              events_scanned: 2,
              events_matched: 2,
              groups_estimated: 1,
              groups: []
            }),
            { headers: { "content-type": "application/json" }, status: 200 }
          )
      )
    );

    await expect(previewReportWorkflowPolicyDraftImpactAction(body)).resolves.toMatchObject({
      ok: true,
      data: {
        policy_id: 0,
        status: "ready"
      }
    });

    expect(fetch).toHaveBeenCalledWith(
      "/api/config/report-workflow-policies/impact-preview",
      expect.objectContaining({
        body: JSON.stringify(body),
        cache: "no-store",
        method: "POST"
      })
    );
  });
});
