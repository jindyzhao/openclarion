import { describe, expect, it } from "vitest";

import {
  emptyReportWorkflowScheduleForm,
  formStateToWriteRequest,
  reportWorkflowScheduleLaunchHref,
  reportWorkflowScheduleLaunchIntentFromSearchParams,
  reportWorkflowScheduleLaunchIntentKey,
  reportWorkflowScheduleDraftReadiness,
  reportWorkflowScheduleEnablementReadiness,
  reportWorkflowScheduleProofOutcome,
  scheduleToFormState
} from "./format";
import type { ReportWorkflowSchedule } from "./types";
import { formatDurationSeconds } from "../format";

describe("report workflow schedule formatting", () => {
  it("builds write requests from validated form state", () => {
    const parsed = formStateToWriteRequest({
      name: " Daily report window ",
      reportWorkflowPolicyID: 7,
      temporalScheduleID: " openclarion-report-policy-7-daily ",
      intervalSeconds: 86400,
      offsetSeconds: 21600,
      replayWindowSeconds: 3600,
      replayDelaySeconds: 300,
      replayLimit: 10000,
      catchupWindowSeconds: 3600
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        name: "Daily report window",
        report_workflow_policy_id: 7,
        temporal_schedule_id: "openclarion-report-policy-7-daily",
        interval_seconds: 86400,
        offset_seconds: 21600,
        replay_window_seconds: 3600,
        replay_delay_seconds: 300,
        replay_limit: 10000,
        catchup_window_seconds: 3600
      }
    });
  });

  it("rejects offsets that are not less than interval", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowScheduleForm(),
      name: "Daily",
      reportWorkflowPolicyID: 1,
      temporalScheduleID: "schedule-1",
      intervalSeconds: 3600,
      offsetSeconds: 3600
    });

    expect(parsed).toEqual({ ok: false, message: "Offset must be less than interval." });
  });

  it("rejects replay windows that exceed the schedule interval", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowScheduleForm(),
      name: "Every minute",
      reportWorkflowPolicyID: 1,
      temporalScheduleID: "schedule-1",
      intervalSeconds: 60,
      replayWindowSeconds: 3600
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Replay window must be less than or equal to interval to avoid overlapping scheduled replay windows."
    });
  });

  it("rejects Temporal Schedule IDs with whitespace", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowScheduleForm(),
      name: "Daily",
      reportWorkflowPolicyID: 1,
      temporalScheduleID: "schedule one"
    });

    expect(parsed).toEqual({ ok: false, message: "Temporal Schedule ID must not contain whitespace." });
  });

  it("maps schedules back to form state", () => {
    expect(
      scheduleToFormState({
        id: 1,
        name: "Daily",
        report_workflow_policy_id: 7,
        temporal_schedule_id: "schedule-1",
        interval_seconds: 86400,
        offset_seconds: 0,
        replay_window_seconds: 3600,
        replay_delay_seconds: 300,
        replay_limit: 10000,
        catchup_window_seconds: 3600,
        enabled: false,
        enabled_at: null,
        disabled_at: null,
        created_at: "2026-06-06T02:00:00Z",
        updated_at: "2026-06-06T02:00:00Z"
      })
    ).toMatchObject({
      name: "Daily",
      reportWorkflowPolicyID: 7,
      temporalScheduleID: "schedule-1",
      intervalSeconds: 86400
    });
  });

  it("parses launch intents for overview-driven schedule setup", () => {
    expect(
      reportWorkflowScheduleLaunchIntentFromSearchParams({
        intent: "create-schedule",
        policy_id: "7"
      })
    ).toEqual({
      message: "Prepared an hourly replay schedule from the settings overview proof action.",
      name: "Hourly report replay",
      policyID: 7,
      temporalScheduleID: "openclarion-report-policy-7-hourly"
    });
    expect(
      reportWorkflowScheduleLaunchIntentFromSearchParams({
        intent: "create-schedule",
        policy_id: "not-a-number"
      })
    ).toMatchObject({
      policyID: null,
      temporalScheduleID: ""
    });
    expect(reportWorkflowScheduleLaunchIntentFromSearchParams({ intent: "unknown" })).toBeNull();
  });

  it("builds stable schedule launch hrefs and keys", () => {
    const intent = reportWorkflowScheduleLaunchIntentFromSearchParams({
      intent: "create-schedule",
      policy_id: "7"
    });

    expect(reportWorkflowScheduleLaunchHref({ intent: "create-schedule", policyID: 7 })).toBe(
      "/settings/report-workflow-schedules?intent=create-schedule&policy_id=7"
    );
    expect(reportWorkflowScheduleLaunchHref({ intent: "create-schedule" })).toBe(
      "/settings/report-workflow-schedules?intent=create-schedule"
    );
    expect(reportWorkflowScheduleLaunchIntentKey(intent)).toBe(
      "7:Hourly report replay:openclarion-report-policy-7-hourly"
    );
    expect(reportWorkflowScheduleLaunchIntentKey(null)).toBe("default");
  });

  it("reports schedule enablement readiness when the bound policy is enabled", () => {
    const readiness = reportWorkflowScheduleEnablementReadiness({
      policyEnabledIDs: new Set([7]),
      schedule: reportWorkflowSchedule({ report_workflow_policy_id: 7 })
    });

    expect(readiness).toEqual({
      blockers: [],
      detail: "Bound report workflow policy is enabled.",
      label: "Ready to enable.",
      status: "ready"
    });
  });

  it("blocks schedule enablement when the bound policy is disabled", () => {
    const readiness = reportWorkflowScheduleEnablementReadiness({
      policyEnabledIDs: new Set([3]),
      schedule: reportWorkflowSchedule({ report_workflow_policy_id: 7 })
    });

    expect(readiness).toEqual({
      blockers: ["Bound report workflow policy must be enabled before schedule enablement."],
      detail: "Bound report workflow policy must be enabled before schedule enablement.",
      label: "Policy not ready.",
      status: "blocked"
    });
  });

  it("blocks schedule enablement when replay windows overlap intervals", () => {
    const readiness = reportWorkflowScheduleEnablementReadiness({
      policyEnabledIDs: new Set([7]),
      schedule: reportWorkflowSchedule({
        interval_seconds: 60,
        replay_window_seconds: 3600,
        report_workflow_policy_id: 7
      })
    });

    expect(readiness).toEqual({
      blockers: ["Replay window must be less than or equal to interval to avoid overlapping scheduled replay windows."],
      detail: "Replay window must be less than or equal to interval to avoid overlapping scheduled replay windows.",
      label: "Replay window overlaps interval.",
      status: "blocked"
    });
  });

  it("marks draft schedule readiness as pending until a workflow policy is selected", () => {
    const readiness = reportWorkflowScheduleDraftReadiness({
      form: emptyReportWorkflowScheduleForm(),
      policyEnabledIDs: new Set([7])
    });

    expect(readiness).toEqual({
      detail: "Select the workflow policy that this schedule should replay.",
      label: "Select a report workflow policy.",
      status: "pending"
    });
  });

  it("blocks draft schedule readiness when the selected workflow policy is disabled", () => {
    const readiness = reportWorkflowScheduleDraftReadiness({
      form: {
        ...emptyReportWorkflowScheduleForm(),
        reportWorkflowPolicyID: 7
      },
      policyEnabledIDs: new Set([3])
    });

    expect(readiness).toEqual({
      detail: "Enable the selected report workflow policy before this schedule can be enabled.",
      label: "Selected policy is not enabled.",
      status: "blocked"
    });
  });

  it("blocks draft schedule readiness when replay windows overlap intervals", () => {
    const readiness = reportWorkflowScheduleDraftReadiness({
      form: {
        ...emptyReportWorkflowScheduleForm(),
        intervalSeconds: 60,
        name: "Every minute",
        replayWindowSeconds: 3600,
        reportWorkflowPolicyID: 7,
        temporalScheduleID: "schedule-1"
      },
      policyEnabledIDs: new Set([7])
    });

    expect(readiness).toEqual({
      detail: "Replay window must be less than or equal to interval to avoid overlapping scheduled replay windows.",
      label: "Replay window overlaps interval.",
      status: "blocked"
    });
  });

  it("marks draft schedule readiness ready when fields and policy are valid", () => {
    const readiness = reportWorkflowScheduleDraftReadiness({
      form: {
        ...emptyReportWorkflowScheduleForm(),
        name: "Daily report window",
        reportWorkflowPolicyID: 7,
        temporalScheduleID: "openclarion-report-policy-7-daily"
      },
      policyEnabledIDs: new Set([7])
    });

    expect(readiness).toEqual({
      detail: "Schedule fields and bound workflow policy are ready.",
      label: "Ready to save.",
      status: "ready"
    });
  });

  it("summarizes scheduled proof outcome for a valid recurring replay", () => {
    const outcome = reportWorkflowScheduleProofOutcome({
      form: {
        ...emptyReportWorkflowScheduleForm(),
        name: "Hourly report replay",
        reportWorkflowPolicyID: 7,
        temporalScheduleID: "openclarion-report-policy-7-hourly",
        intervalSeconds: 3600,
        offsetSeconds: 300,
        replayWindowSeconds: 1800,
        replayDelaySeconds: 120,
        replayLimit: 5000,
        catchupWindowSeconds: 7200
      },
      policyEnabledIDs: new Set([7]),
      policyLabels: {
        7: "#7 Automatic diagnosis workflow (single_alert, auto_room, enabled)"
      }
    });

    expect(outcome.status).toBe("ready");
    expect(outcome.detail).toBe(
      "This schedule can retain recurring proof by replaying bounded alert windows through the selected workflow policy."
    );
    expect(outcome.items.map((item) => [item.title, item.value, item.status])).toEqual([
      ["Policy", "#7 Automatic diagnosis workflow (single_alert, auto_room, enabled)", "ready"],
      ["Cadence", "Every 1h", "ready"],
      ["Replay sample", "30m window", "ready"],
      ["Catch-up", "2h", "ready"],
      ["Proof target", "openclarion-report-policy-7-hourly", "ready"]
    ]);
    expect(outcome.items[2]?.detail).toContain("capped at 5000 events");
  });

  it("blocks scheduled proof outcome when the policy or replay window is invalid", () => {
    const outcome = reportWorkflowScheduleProofOutcome({
      form: {
        ...emptyReportWorkflowScheduleForm(),
        name: "Bad schedule",
        reportWorkflowPolicyID: 7,
        temporalScheduleID: "bad schedule",
        intervalSeconds: 300,
        replayWindowSeconds: 3600
      },
      policyEnabledIDs: new Set([3]),
      policyLabels: {}
    });

    expect(outcome.status).toBe("blocked");
    expect(outcome.detail).toBe("Resolve blocked policy or timing settings before scheduled-trigger proof can run.");
    expect(outcome.items.map((item) => [item.title, item.status])).toEqual([
      ["Policy", "blocked"],
      ["Cadence", "ready"],
      ["Replay sample", "blocked"],
      ["Catch-up", "ready"],
      ["Proof target", "blocked"]
    ]);
  });

  it("formats whole-second durations compactly", () => {
    expect(formatDurationSeconds(90061)).toBe("1d 1h 1m 1s");
    expect(formatDurationSeconds(0)).toBe("0s");
  });
});

function reportWorkflowSchedule(
  overrides: Partial<ReportWorkflowSchedule> = {}
): ReportWorkflowSchedule {
  return {
    id: 1,
    name: "Daily",
    report_workflow_policy_id: 7,
    temporal_schedule_id: "schedule-1",
    interval_seconds: 86400,
    offset_seconds: 0,
    replay_window_seconds: 3600,
    replay_delay_seconds: 300,
    replay_limit: 10000,
    catchup_window_seconds: 3600,
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: "2026-06-06T02:00:00Z",
    updated_at: "2026-06-06T02:00:00Z",
    ...overrides
  };
}
