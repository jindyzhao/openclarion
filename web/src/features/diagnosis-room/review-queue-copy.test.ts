import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";
import {
  diagnosisReviewQueueActionPlan,
  diagnosisReviewQueueItems,
  diagnosisReviewQueuePostEvidenceStatus,
  diagnosisReviewQueueSummary,
  diagnosisReviewQueueTaskProgress,
  type DiagnosisReviewQueueInput,
} from "./review-queue";
import {
  localizeDiagnosisReviewQueueActionPlan,
  localizeDiagnosisReviewQueueBlocker,
  localizeDiagnosisReviewQueueItem,
  localizeDiagnosisReviewQueueNextAction,
  localizeDiagnosisReviewQueuePostEvidenceStatus,
  localizeDiagnosisReviewQueueSummary,
  localizeDiagnosisReviewQueueTaskProgress,
} from "./review-queue-copy";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.reviewQueue",
});
const tStatus = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.status",
});

function baseInput(
  overrides: Partial<DiagnosisReviewQueueInput> = {},
): DiagnosisReviewQueueInput {
  return {
    canConfirmConclusion: false,
    collectionResults: [],
    evidenceCollectionSuggestions: [],
    evidenceRequests: [],
    missingEvidenceRequests: [],
    requiresHumanReview: false,
    supplementalEvidence: [],
    ...overrides,
  };
}

describe("diagnosis review queue copy", () => {
  it("localizes a failed collection blocker and action plan from structure", () => {
    const input = baseInput({
      collectionResults: [
        {
          collected_at: "2026-07-16T00:00:00Z",
          message: "provider unavailable",
          observed_alerts: 0,
          reason_code: "provider_unavailable",
          request: {
            reason: "Inspect saturation",
            tool: "metric_query",
          },
          status: "failed",
          tool: "metric_query",
        },
      ],
    });
    const rawItems = diagnosisReviewQueueItems(input);
    const rawSummary = diagnosisReviewQueueSummary(rawItems, input);
    const items = rawItems.map((item) =>
      localizeDiagnosisReviewQueueItem(item, input, t, tStatus),
    );
    const summary = localizeDiagnosisReviewQueueSummary(rawSummary, input, t);
    const plan = localizeDiagnosisReviewQueueActionPlan(
      diagnosisReviewQueueActionPlan(rawItems, rawSummary),
      rawSummary,
      input,
      items,
      t,
    );

    expect(localizeDiagnosisReviewQueueBlocker(input, t)).toBe(
      "确认前请解决 metric_query 证据采集问题。",
    );
    expect(items[0]).toMatchObject({
      detail: "原因：provider_unavailable。 详情：provider unavailable",
      tag: "失败",
      title: "metric_query 证据",
    });
    expect(summary.message).toBe("确认前请解决 metric_query 证据采集问题。");
    expect(plan.actions[0]?.title).toBe("metric_query 证据");
  });

  it("localizes submitted evidence review status and task phases", () => {
    const request = {
      detail: "Provide the deployment diff",
      label: "Deployment diff",
      priority: "high",
    } as const;
    const input = baseInput({
      confidence: "medium",
      confidenceProgress: { label: "Confidence improved", status: "improved" },
      latestAssistantSequence: 3,
      missingEvidenceRequests: [request],
      supplementalEvidence: [
        {
          assistant_message_id: "assistant-2",
          assistant_sequence: 2,
          assistant_turn_id: 22,
          detail: request.detail,
          evidence: "Deployment changed from v1 to v2",
          label: request.label,
          priority: request.priority,
          provided_at: "2026-07-16T00:00:00Z",
          user_message_id: "user-2",
          user_sequence: 2,
          user_turn_id: 21,
        },
      ],
    });
    const rawItems = diagnosisReviewQueueItems(input);
    const rawSummary = diagnosisReviewQueueSummary(rawItems, input);
    const rawPost = diagnosisReviewQueuePostEvidenceStatus(input);
    const post = localizeDiagnosisReviewQueuePostEvidenceStatus(
      rawPost,
      input,
      t,
      tStatus,
    );
    const progress = localizeDiagnosisReviewQueueTaskProgress(
      diagnosisReviewQueueTaskProgress(rawItems, rawSummary, rawPost),
      rawItems,
      rawSummary,
      input,
      post,
      t,
    );

    expect(post.label).toBe("补充证据等待 AI 审核");
    expect(post.detail).toContain("最新置信度为中");
    expect(progress.statusLabel).toBe("证据任务已阻塞");
    expect(progress.phases.map((phase) => phase.label)).toEqual([
      "采集证据",
      "提供操作员证据",
      "AI 重新评估",
      "确认结论",
    ]);
    expect(progress.summary).not.toMatch(/[A-Za-z]{3,}/);
  });

  it("derives localized next actions from semantic queue state", () => {
    expect(
      localizeDiagnosisReviewQueueNextAction(
        baseInput({
          collectionResults: [
            {
              collected_at: "2026-07-16T00:00:00Z",
              message: "provider unavailable",
              observed_alerts: 0,
              reason_code: "provider_unavailable",
              request: { reason: "Inspect saturation", tool: "metric_query" },
              status: "failed",
              tool: "metric_query",
            },
          ],
        }),
        t,
      ),
    ).toBe("解决证据采集问题");
    expect(
      localizeDiagnosisReviewQueueNextAction(
        baseInput({
          collectionResults: [],
          evidenceRequests: [
            {
              query: "up",
              reason: "Read service availability",
              tool: "metric_query",
            },
          ],
        }),
        t,
      ),
    ).toBe("执行证据采集");
    expect(
      localizeDiagnosisReviewQueueNextAction(
        baseInput({ conclusionStatus: "ready_for_review" }),
        t,
      ),
    ).toBe("可以确认");
  });
});
