import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";
import {
  diagnosisConsultationConclusionLifecycleStatus,
  diagnosisConsultationReassessmentStatus,
} from "./consultation-progress";
import {
  localizeDiagnosisCollectionProgressSummary,
  localizeDiagnosisConsultationLifecycle,
  localizeDiagnosisConsultationReassessment,
  localizeDiagnosisSupplementalEvidenceSummary,
} from "./consultation-progress-copy";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.consultationProgress",
});
const tStatus = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.status",
});

describe("diagnosis consultation progress copy", () => {
  it("localizes reassessment evidence and collection summaries", () => {
    const collectionResult = {
      collected_at: "2026-07-16T00:00:00Z",
      message: "",
      observed_alerts: 1,
      reason_code: "",
      request: { reason: "Inspect saturation", tool: "metric_query" },
      status: "collected",
      tool: "metric_query",
    };
    const reassessment = localizeDiagnosisConsultationReassessment(
      diagnosisConsultationReassessmentStatus({
        autoFollowUpCount: 1,
        collectionResults: [collectionResult],
        confidenceTimeline: [
          {
            confidence: "high",
            conclusion_status: "ready_for_review",
            evidence_collection_results: [collectionResult],
            occurred_at: "2026-07-16T00:01:00Z",
            requires_human_review: true,
            turn_count: 2,
          },
        ],
      }),
      1,
      t,
      tStatus,
    );

    expect(reassessment.label).toBe("AI 已重新评估证据");
    expect(reassessment.detail).toContain("最新置信度：高");
    expect(reassessment.detail).toContain("结论状态：等待审核");
    expect(localizeDiagnosisSupplementalEvidenceSummary(1, 2, t)).toBe(
      "1 项可执行请求和 2 项补充请求。",
    );
    expect(localizeDiagnosisCollectionProgressSummary([collectionResult], t)).toBe(
      "共 1 项：已采集 1 项、失败 0 项、跳过 0 项、不支持 0 项。",
    );
  });

  it("localizes retained conclusion delivery lifecycle", () => {
    const lifecycle = diagnosisConsultationConclusionLifecycleStatus({
      finalConclusion: {
        confidence: "high",
        confidence_rationale: "Evidence complete",
        confirmed_by: "iam:user-1",
        recorded_at: "2026-07-16T00:02:00Z",
        source: "assistant",
        status: "available",
      },
      notificationDelivery: {
        detail: "Delivery complete",
        status: "ready",
      },
    });

    expect(
      localizeDiagnosisConsultationLifecycle(
        lifecycle,
        t,
        "AI 交付证明完整。",
      ),
    ).toMatchObject({
      detail: "已保留操作员确认的结论，且交付证明完整。AI 交付证明完整。",
      label: "结论已交付",
    });
  });
});
