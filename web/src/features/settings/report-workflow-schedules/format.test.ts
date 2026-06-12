import { describe, expect, it } from "vitest";

import {
  emptyReportWorkflowScheduleForm,
  formStateToWriteRequest,
  scheduleToFormState
} from "./format";
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

  it("formats whole-second durations compactly", () => {
    expect(formatDurationSeconds(90061)).toBe("1d 1h 1m 1s");
    expect(formatDurationSeconds(0)).toBe("0s");
  });
});
