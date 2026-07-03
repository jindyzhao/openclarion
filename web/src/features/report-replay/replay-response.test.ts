import { describe, expect, it } from "vitest";

import { normalizedReportReplayTriggerResponse } from "./replay-response";

describe("report replay response normalization", () => {
  it("accepts valid replay responses with automatic diagnosis rooms", () => {
    expect(
      normalizedReportReplayTriggerResponse({
        ...validReplayResponse(),
        auto_diagnosis: {
          policies_matched: 1,
          snapshots: 1,
          rooms_started: 1,
          rooms_skipped: 0,
          rooms: [
            {
              policy_id: 7,
              evidence_snapshot_id: 17,
              session_id: "diagnosis-session-auto-p7-s17",
              initial_message_id: "diagnosis-auto-initial-p7-s17",
              workflow_id: "diagnosis-room-diagnosis-session-auto-p7-s17",
              run_id: "run-17",
            },
          ],
        },
      }),
    ).toMatchObject({
      auto_diagnosis: {
        rooms_started: 1,
        rooms: [
          {
            session_id: "diagnosis-session-auto-p7-s17",
          },
        ],
      },
      correlation_key: "alert-replay-7002",
      snapshots: [{ id: 9002, group_index: 0, event_count: 1 }],
      started: true,
    });
  });

  it("accepts no-op replay responses without workflow ids when no snapshots exist", () => {
    expect(
      normalizedReportReplayTriggerResponse({
        ...validReplayResponse({
          run_id: "",
          snapshots: [],
          started: false,
          workflow_id: "",
        }),
      }),
    ).toMatchObject({
      run_id: "",
      snapshots: [],
      started: false,
      workflow_id: "",
    });
  });

  it("rejects started replay responses that omit workflow identity", () => {
    expect(
      normalizedReportReplayTriggerResponse(
        validReplayResponse({ run_id: "", workflow_id: "" }),
      ),
    ).toBeNull();
  });

  it("rejects malformed automatic diagnosis room counters", () => {
    expect(
      normalizedReportReplayTriggerResponse({
        ...validReplayResponse(),
        auto_diagnosis: {
          policies_matched: 1,
          snapshots: 1,
          rooms_started: 1,
          rooms_skipped: 0,
          rooms: [],
        },
      }),
    ).toBeNull();
  });

  it("rejects invalid counters and snapshot refs", () => {
    expect(
      normalizedReportReplayTriggerResponse(
        validReplayResponse({
          stats: {
            ...validReplayStats(),
            events_loaded: -1,
          },
        }),
      ),
    ).toBeNull();
    expect(
      normalizedReportReplayTriggerResponse(
        validReplayResponse({
          snapshots: [{ id: 0, group_index: 0, event_count: 1 }],
        }),
      ),
    ).toBeNull();
  });
});

function validReplayResponse(
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    correlation_key: "alert-replay-7002",
    run_id: "run-alert-replay",
    snapshots: [{ id: 9002, group_index: 0, event_count: 1 }],
    started: true,
    stats: validReplayStats(),
    workflow_id: "report-batch-alert-replay",
    ...overrides,
  };
}

function validReplayStats(): Record<string, unknown> {
  return {
    events_loaded: 1,
    failed: 0,
    groups_built: 1,
    groups_closed: 1,
    groups_existing: 0,
    groups_refreshed: 0,
    groups_saved: 1,
    ingested: { total: 1, saved: 1, duplicate: 0, failed: 0 },
    snapshots_duplicate: 0,
    snapshots_saved: 1,
  };
}
