import { describe, expect, it } from "vitest";

import type { ReportReplayTriggerResponse } from "./api";
import { replayAcceptedMessage, replayProofNextAction } from "./alerts-view";

describe("alert replay confirmed-only feedback", () => {
  it("routes confirmed-only replays to report delivery without a manual AI handoff", () => {
    const result = reportReplayTriggerResponse({
      auto_diagnosis: {
        policies_matched: 1,
        rooms: [],
        rooms_skipped: 0,
        rooms_started: 0,
        skipped_snapshot_ids: [],
        snapshots: 1
      }
    });

    expect(replayProofNextAction(result)).toMatchObject({
      actionLabel: "Open reports",
      href: "/reports",
      kind: "report",
      label: "Review report delivery",
      type: "info"
    });
    expect(replayProofNextAction(result).detail).toContain("already has a human-confirmed conclusion");
    expect(replayAcceptedMessage(result)).toBe(
      "Replay accepted with 1 evidence snapshot; 1 snapshot already has a human-confirmed conclusion, so no new diagnosis room was started."
    );
  });
});

function reportReplayTriggerResponse(
  overrides: Partial<ReportReplayTriggerResponse> = {}
): ReportReplayTriggerResponse {
  return {
    correlation_key: "alert-replay-101",
    run_id: "run-policy-smoke",
    snapshots: [{ id: 101, group_index: 0, event_count: 3 }],
    started: true,
    stats: {
      events_loaded: 3,
      failed: 0,
      groups_built: 1,
      groups_closed: 0,
      groups_existing: 0,
      groups_refreshed: 0,
      groups_saved: 1,
      ingested: {
        duplicate: 0,
        failed: 0,
        saved: 3,
        total: 3
      },
      snapshots_duplicate: 0,
      snapshots_saved: 1
    },
    workflow_id: "report-batch-policy-smoke",
    ...overrides
  };
}
