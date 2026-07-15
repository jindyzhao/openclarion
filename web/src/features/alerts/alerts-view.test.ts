import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";
import zhCN from "../../../messages/zh-CN.json";

import type { ReportReplayTriggerResponse } from "./api";
import {
  alertReplayProofNextAction,
  localizeAlertReplayProofNextAction,
  localizeReplayAcceptedMessage,
  localizeReportReplayProofTrace,
} from "./replay-copy";

const tEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "Alerts",
});
const tZh = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "Alerts",
});

describe("alert replay presentation copy", () => {
  it("routes confirmed-only replays to report delivery without a manual AI handoff", () => {
    const result = reportReplayTriggerResponse({
      auto_diagnosis: {
        policies_matched: 1,
        rooms: [],
        rooms_skipped: 0,
        rooms_started: 0,
        skipped_snapshot_ids: [],
        snapshots: 1,
      },
    });

    const action = alertReplayProofNextAction(result);
    expect(action).toMatchObject({
      code: "report_delivery_confirmed",
      confirmedSnapshots: 1,
      href: "/reports",
      kind: "report",
      type: "info",
    });
    expect(localizeAlertReplayProofNextAction(action, tEn)).toEqual({
      actionLabel: "Open reports",
      detail:
        "1 snapshot already has a human-confirmed conclusion, so no new diagnosis room or manual AI handoff is required.",
      label: "Review report delivery",
    });
    expect(localizeReplayAcceptedMessage(result, tEn)).toBe(
      "Replay accepted with 1 evidence snapshot; 1 snapshot already has a human-confirmed conclusion, so no new diagnosis room was started.",
    );
  });

  it("renders confirmed-only replay feedback from the Chinese catalog", () => {
    const result = reportReplayTriggerResponse({
      auto_diagnosis: {
        policies_matched: 1,
        rooms: [],
        rooms_skipped: 0,
        rooms_started: 0,
        skipped_snapshot_ids: [],
        snapshots: 1,
      },
    });

    const message = localizeReplayAcceptedMessage(result, tZh);
    const action = localizeAlertReplayProofNextAction(
      alertReplayProofNextAction(result),
      tZh,
    );

    expect(message).toContain("人工确认");
    expect(message).not.toContain("human-confirmed");
    expect(action.label).toBe("检查报告交付");
    expect(action.detail).toContain("无需创建新的诊断室");
  });

  it("localizes structured replay proof without matching generated English text", () => {
    const result = reportReplayTriggerResponse({
      auto_diagnosis: {
        policies_matched: 1,
        rooms: [
          {
            evidence_snapshot_id: 101,
            initial_message_id: "diagnosis-auto-initial-101",
            policy_id: 7,
            run_id: "run-diagnosis-101",
            session_id: "diagnosis-session-auto-101",
            workflow_id: "diagnosis-room-auto-101",
          },
        ],
        rooms_skipped: 0,
        rooms_started: 1,
        skipped_snapshot_ids: [],
        snapshots: 1,
      },
    });
    const localized = localizeReportReplayProofTrace(
      result,
      tZh,
    );

    expect(localized.detail).toContain("下游 AI");
    expect(localized.items.map((item) => item.title)).toEqual([
      "触发",
      "证据",
      "AI 诊断",
      "通知证明",
    ]);
    expect(localized.items[0]?.detail).toContain(result.workflow_id);
    expect(localized.items[2]?.detail).toContain("启动 1 个诊断室");
    expect(localized.items[3]?.actions).toEqual([
      {
        href:
          "/diagnosis-room?evidence_snapshot_id=101&intent=review_conclusion&session_id=diagnosis-session-auto-101",
        label: "检查诊断室 #101",
      },
    ]);
  });
});

function reportReplayTriggerResponse(
  overrides: Partial<ReportReplayTriggerResponse> = {},
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
        total: 3,
      },
      snapshots_duplicate: 0,
      snapshots_saved: 1,
    },
    workflow_id: "report-batch-policy-smoke",
    ...overrides,
  };
}
