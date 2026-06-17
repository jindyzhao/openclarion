import { describe, expect, it } from "vitest";

import {
  defaultReportWorkflowPolicyReplayForm,
  emptyReportWorkflowPolicyForm,
  formStateToReplayRequest,
  formStateToWriteRequest,
  policyToFormState
} from "./format";
import type { ReportWorkflowPolicy } from "./types";

describe("report workflow policy form formatting", () => {
  it("builds write requests without enabled state", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowPolicyForm(),
      name: " Default report workflow ",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      reportScenario: "cascade",
      diagnosisFollowUp: "suggest_room"
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        name: "Default report workflow",
        alert_source_profile_id: 1,
        grouping_policy_id: 2,
        report_notification_channel_profile_id: 3,
        trigger_mode: "manual_replay",
        report_scenario: "cascade",
        diagnosis_follow_up: "suggest_room"
      }
    });
  });

  it("rejects missing bound profile IDs", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowPolicyForm(),
      name: "Default report workflow",
      alertSourceProfileID: null,
      groupingPolicyID: 2
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Select an alert source."
    });
  });

  it("rejects invalid optional report notification channel IDs", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowPolicyForm(),
      name: "Default report workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 0
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Select a valid report notification channel."
    });
  });

  it("maps policy rows back to edit form state", () => {
    const policy: ReportWorkflowPolicy = {
      id: 7,
      name: "Default report workflow",
      alert_source_profile_id: 1,
      grouping_policy_id: 2,
      report_notification_channel_profile_id: 3,
      trigger_mode: "manual_replay",
      report_scenario: "single_alert",
      diagnosis_follow_up: "disabled",
      enabled: false,
      enabled_at: null,
      disabled_at: null,
      created_at: "2026-06-05T08:00:00Z",
      updated_at: "2026-06-05T08:00:00Z"
    };

    expect(policyToFormState(policy)).toEqual({
      name: "Default report workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      triggerMode: "manual_replay",
      reportScenario: "single_alert",
      diagnosisFollowUp: "disabled"
    });
  });

  it("builds replay requests from bounded windows", () => {
    const parsed = formStateToReplayRequest({
      windowStart: "2026-06-05T08:00:00Z",
      windowEnd: "2026-06-05T09:00:00Z",
      limit: 25,
      correlationKey: " incident-42 ",
      workflowID: " report-batch-42 "
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        window_start: "2026-06-05T08:00:00Z",
        window_end: "2026-06-05T09:00:00Z",
        limit: 25,
        correlation_key: "incident-42",
        workflow_id: "report-batch-42"
      }
    });
  });

  it("rejects invalid replay windows", () => {
    const parsed = formStateToReplayRequest({
      ...defaultReportWorkflowPolicyReplayForm(new Date("2026-06-05T09:00:00Z")),
      windowEnd: "2026-06-05T08:00:00Z"
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Window end must be after window start."
    });
  });

  it("defaults replay windows to the previous hour", () => {
    expect(defaultReportWorkflowPolicyReplayForm(new Date("2026-06-05T09:00:00Z"))).toEqual({
      windowStart: "2026-06-05T08:00:00Z",
      windowEnd: "2026-06-05T09:00:00Z",
      limit: 10000,
      correlationKey: "",
      workflowID: ""
    });
  });
});
