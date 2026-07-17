import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";
import zhCN from "../../../messages/zh-CN.json";
import {
  diagnosisFinalConclusionConfidenceProgress,
  diagnosisFinalConclusionRetentionState,
  diagnosisFinalConclusionReviewItems,
  diagnosisFinalConclusionTraceabilityStatus,
} from "./final-conclusion";
import {
  localizeFinalConclusionConfidenceProgress,
  localizeFinalConclusionRetention,
  localizeFinalConclusionReviewItems,
  localizeFinalConclusionText,
  localizeFinalConclusionTraceability,
} from "./final-conclusion-copy";

const tEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "DiagnosisRoom.finalConclusion",
});
const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.finalConclusion",
});
const tStatus = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.status",
});
const tStatusEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "DiagnosisRoom.status",
});
const conclusion = {
  confidence: "high",
  confidence_rationale: "证据链完整。",
  evidence_collection_suggestions: [],
  evidence_requests: [],
  missing_evidence_requests: [],
  status: "available",
};

describe("final conclusion presentation copy", () => {
  it("localizes retention and review state from conclusion fields", () => {
    const retention = localizeFinalConclusionRetention(
      diagnosisFinalConclusionRetentionState(conclusion),
      conclusion,
      t,
      (value) => value,
    );
    const review = localizeFinalConclusionReviewItems(
      diagnosisFinalConclusionReviewItems(conclusion),
      conclusion,
      t,
      tStatus,
      (value) => value,
    );

    expect(retention.label).toBe("结论已就绪");
    expect(review.find((item) => item.key === "confidence-rationale")).toMatchObject({
      detail: "证据链完整。",
      title: "置信度：高",
    });
    expect(review.find((item) => item.key === "evidence-gaps")?.title).toBe(
      "证据缺口已清除",
    );
  });

  it("localizes confidence progress and closure traceability", () => {
    const progress = localizeFinalConclusionConfidenceProgress(
      diagnosisFinalConclusionConfidenceProgress(conclusion, [
        { confidence: "low" },
      ]),
      t,
      tStatus,
    );
    const retained = {
      ...conclusion,
      confirmed_by: "iam:user-1",
      recorded_at: "2026-07-16T00:00:00Z",
    };
    const traceability = localizeFinalConclusionTraceability(
      diagnosisFinalConclusionTraceabilityStatus({
        conclusion: retained,
        notificationDelivery: {
          detail: "complete",
          label: "complete",
          readyCount: 3,
          requiredCount: 3,
          status: "ready",
        },
      }),
      t,
      {
        detail: "AI 交付证明完整。",
        label: "AI 交付完整",
        readyCount: 3,
        requiredCount: 3,
      },
    );

    expect(progress.label).toBe("置信度已提高");
    expect(progress.detail).toContain("低");
    expect(progress.detail).toContain("高");
    expect(traceability.label).toBe("关闭可追溯性完整");
    expect(traceability.notificationLabel).toBe("AI 交付完整");
  });

  it("keeps pending conclusion and delivery states semantically distinct", () => {
    const pendingConclusion = localizeFinalConclusionTraceability(
      diagnosisFinalConclusionTraceabilityStatus({ conclusion: null }),
      t,
    );
    const retained = {
      ...conclusion,
      confirmed_by: "iam:user-1",
      recorded_at: "2026-07-16T00:00:00Z",
    };
    const deliveryNotStarted = localizeFinalConclusionTraceability(
      diagnosisFinalConclusionTraceabilityStatus({
        conclusion: retained,
        notificationDelivery: {
          detail: "not started",
          label: "not started",
          readyCount: 1,
          requiredCount: 3,
          status: "pending",
        },
      }),
      t,
      {
        detail: "尚未开始",
        label: "等待中",
        readyCount: 1,
        requiredCount: 3,
      },
    );

    expect(pendingConclusion).toMatchObject({
      detail: "AI 保留可审核最终结论后，才能检查操作员确认和通知交付证明。",
      kind: "conclusion_pending",
      label: "关闭可追溯性等待中",
    });
    expect(deliveryNotStarted).toMatchObject({
      detail: "已保留操作员确认的结论；AI 交付证明尚未开始（3 个阶段中已完成 1 个）。",
      kind: "delivery_not_started",
      label: "关闭交付证明等待中",
    });
  });

  it("retains the selected rationale and formats recorded timestamps", () => {
    const progress = localizeFinalConclusionConfidenceProgress(
      diagnosisFinalConclusionConfidenceProgress(
        { confidence: "high", status: "available" },
        [
          {
            confidence: "low",
            confidence_rationale: "Timeline rationale remains verbatim.",
          },
        ],
      ),
      tEn,
      tStatusEn,
    );
    const retained = {
      ...conclusion,
      confirmed_by: "iam:user-1",
      recorded_at: "2026-07-16T00:00:00Z",
    };
    const retention = localizeFinalConclusionRetention(
      diagnosisFinalConclusionRetentionState(retained),
      retained,
      tEn,
      () => "Jul 16, 2026, 12:00 AM",
    );

    expect(progress.detail).toBe(
      "Confidence improved from low to high. Timeline rationale remains verbatim.",
    );
    expect(retention.detail).toContain("Jul 16, 2026, 12:00 AM");
  });

  it("localizes controlled fallback text and preserves external content", () => {
    expect(
      localizeFinalConclusionText(
        { content: "  Operator-authored conclusion.  " },
        t,
        tStatus,
      ),
    ).toBe("Operator-authored conclusion.");
    expect(
      localizeFinalConclusionText(
        { reason: "assistant_marked_ready_for_review" },
        t,
        tStatus,
      ),
    ).toBe("AI 已标记为可审核");
    expect(
      localizeFinalConclusionText(
        { confidence: "high", status: "available" },
        t,
        tStatus,
      ),
    ).toBe("可用（高）");
    expect(
      localizeFinalConclusionText(
        { reason: "provider_specific_reason" },
        t,
        tStatus,
      ),
    ).toBe("provider_specific_reason");
  });
});
