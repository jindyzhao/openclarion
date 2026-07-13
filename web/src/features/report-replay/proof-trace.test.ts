import { describe, expect, it } from "vitest";

import type { components } from "@/lib/api/openapi";
import { reportReplayProofTrace } from "./proof-trace";

type ReportReplayTriggerResponse = components["schemas"]["ReportReplayTriggerResponse"];

describe("report replay proof trace", () => {
  it("points notification proof at diagnosis room timelines when auto rooms start", () => {
    const trace = reportReplayProofTrace(
      reportReplayTriggerResponse({
        auto_diagnosis: {
          policies_matched: 1,
          snapshots: 2,
          rooms_started: 2,
          rooms_skipped: 0,
          skipped_snapshot_ids: [],
          rooms: [
            autoDiagnosisRoom(101, "diagnosis-session-auto-p7-s101"),
            autoDiagnosisRoom(102, "diagnosis-session-auto-p7-s102")
          ]
        },
        snapshots: [
          { id: 101, group_index: 0, event_count: 2 },
          { id: 102, group_index: 1, event_count: 1 }
        ],
        stats: {
          ...reportReplayTriggerResponse().stats,
          groups_built: 2,
          snapshots_saved: 2
        }
      })
    );

    const notificationProof = proofItem(trace, "Notification proof");

    expect(trace.status).toBe("review");
    expect(notificationProof).toMatchObject({
      status: "review",
      value: "Room timelines"
    });
    expect(notificationProof.detail).toContain("2 automatic diagnosis rooms started");
    expect(notificationProof.detail).toContain("diagnosis-room notification timeline");
    expect(notificationProof.actions).toEqual([
      {
        detail: "Open the automatic diagnosis room and review its notification timeline.",
        href: "/diagnosis-room?evidence_snapshot_id=101&intent=review_conclusion&session_id=diagnosis-session-auto-p7-s101",
        label: "Review room #101"
      },
      {
        detail: "Open the automatic diagnosis room and review its notification timeline.",
        href: "/diagnosis-room?evidence_snapshot_id=102&intent=review_conclusion&session_id=diagnosis-session-auto-p7-s102",
        label: "Review room #102"
      }
    ]);
    expect(proofItem(trace, "Trigger").detail).toContain("correlation alert-replay-101");
  });

  it("keeps safety-capped snapshots visible in notification proof", () => {
    const trace = reportReplayProofTrace(
      reportReplayTriggerResponse({
        auto_diagnosis: {
          policies_matched: 1,
          snapshots: 2,
          rooms_started: 1,
          rooms_skipped: 1,
          skipped_snapshot_ids: [102],
          rooms: [autoDiagnosisRoom(101, "diagnosis-session-auto-p7-s101")]
        },
        snapshots: [
          { id: 101, group_index: 0, event_count: 2 },
          { id: 102, group_index: 1, event_count: 1 }
        ]
      })
    );

    const notificationProof = proofItem(trace, "Notification proof");

    expect(notificationProof).toMatchObject({
      status: "review",
      value: "Room timeline"
    });
    expect(notificationProof.detail).toContain("1 snapshot remains without automatic room timelines");
    expect(notificationProof.actions).toEqual([
      {
        detail: "Open the automatic diagnosis room and review its notification timeline.",
        href: "/diagnosis-room?evidence_snapshot_id=101&intent=review_conclusion&session_id=diagnosis-session-auto-p7-s101",
        label: "Review room #101"
      },
      {
        detail: "Open the retained evidence snapshot and create a manual diagnosis room.",
        href: "/diagnosis-room?evidence_snapshot_id=102&intent=alert_review",
        label: "Create room #102"
      }
    ]);
  });

  it("uses explicit safety-cap snapshot ids around confirmed snapshots", () => {
    const trace = reportReplayProofTrace(
      reportReplayTriggerResponse({
        auto_diagnosis: {
          policies_matched: 1,
          snapshots: 4,
          rooms_started: 1,
          rooms_skipped: 1,
          skipped_snapshot_ids: [103],
          rooms: [autoDiagnosisRoom(102, "diagnosis-session-auto-p7-s102")]
        },
        snapshots: [
          { id: 101, group_index: 0, event_count: 2 },
          { id: 102, group_index: 1, event_count: 1 },
          { id: 103, group_index: 2, event_count: 1 },
          { id: 104, group_index: 3, event_count: 1 }
        ]
      })
    );

    expect(proofItem(trace, "Notification proof").actions).toEqual([
      {
        detail: "Open the automatic diagnosis room and review its notification timeline.",
        href: "/diagnosis-room?evidence_snapshot_id=102&intent=review_conclusion&session_id=diagnosis-session-auto-p7-s102",
        label: "Review room #102"
      },
      {
        detail: "Open the retained evidence snapshot and create a manual diagnosis room.",
        href: "/diagnosis-room?evidence_snapshot_id=103&intent=alert_review",
        label: "Create room #103"
      }
    ]);
  });

  it("treats confirmed-only auto diagnosis as complete without a manual handoff", () => {
    const trace = reportReplayProofTrace(
      reportReplayTriggerResponse({
        auto_diagnosis: {
          policies_matched: 1,
          snapshots: 2,
          rooms_started: 0,
          rooms_skipped: 0,
          skipped_snapshot_ids: [],
          rooms: []
        },
        snapshots: [
          { id: 101, group_index: 0, event_count: 2 },
          { id: 102, group_index: 1, event_count: 1 }
        ]
      })
    );

    expect(trace.status).toBe("ready");
    expect(proofItem(trace, "AI diagnosis")).toMatchObject({
      status: "ready",
      value: "Already confirmed"
    });
    expect(proofItem(trace, "AI diagnosis").detail).toContain(
      "2 snapshots already have a human-confirmed conclusion"
    );
    const notificationProof = proofItem(trace, "Notification proof");
    expect(notificationProof).toMatchObject({
      status: "ready",
      value: "Already confirmed"
    });
    expect(notificationProof.actions).toBeUndefined();
  });

  it("falls back to final report delivery proof when no automatic room starts", () => {
    const trace = reportReplayProofTrace(reportReplayTriggerResponse());
    const notificationProof = proofItem(trace, "Notification proof");

    expect(trace.status).toBe("pending");
    expect(notificationProof).toMatchObject({
      status: "pending",
      value: "Report delivery"
    });
    expect(notificationProof.detail).toContain("final report delivery timeline");
  });

  it("keeps notification proof unavailable when replay starts no workflow", () => {
    const trace = reportReplayProofTrace(
      reportReplayTriggerResponse({
        run_id: "",
        snapshots: [],
        started: false,
        stats: {
          ...reportReplayTriggerResponse().stats,
          events_loaded: 0,
          groups_built: 0,
          snapshots_saved: 0
        },
        workflow_id: ""
      })
    );

    expect(proofItem(trace, "Notification proof")).toMatchObject({
      status: "pending",
      value: "Not available"
    });
  });
});

function proofItem(trace: ReturnType<typeof reportReplayProofTrace>, title: string) {
  const item = trace.items.find((candidate) => candidate.title === title);
  if (!item) {
    throw new Error(`Missing proof item: ${title}`);
  }
  return item;
}

function autoDiagnosisRoom(evidenceSnapshotID: number, sessionID: string) {
  return {
    evidence_snapshot_id: evidenceSnapshotID,
    initial_message_id: `diagnosis-auto-initial-p7-s${evidenceSnapshotID}`,
    policy_id: 7,
    run_id: `run-diagnosis-${evidenceSnapshotID}`,
    session_id: sessionID,
    workflow_id: `diagnosis-room-${sessionID}`
  };
}

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
