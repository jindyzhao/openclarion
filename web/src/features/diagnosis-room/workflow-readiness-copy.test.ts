import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";
import {
  localizeDiagnosisWorkflowReadinessItem,
  localizeDiagnosisWorkflowReadinessStatus,
} from "./workflow-readiness-copy";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.workflowReadiness",
});
const tStatus = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.status",
});

describe("diagnosis workflow readiness copy", () => {
  it("localizes structured room status and evidence counts", () => {
    const room = localizeDiagnosisWorkflowReadinessItem(
      {
        detail: "Room room-1 is open.",
        detailKey: "roomReady",
        detailValues: { session: "room-1", status: "open" },
        key: "room",
        label: "Room",
        status: "ready",
      },
      t,
      tStatus,
    );
    const evidence = localizeDiagnosisWorkflowReadinessItem(
      {
        detail: "1 executable plan, 2 missing requests, and 3 suggestions remain open.",
        detailKey: "evidenceOpen",
        detailValues: { missing: 2, plans: 1, suggestions: 3 },
        key: "evidence",
        label: "Evidence",
        metric: "1/2 collected",
        metricValues: { collected: 1, total: 2 },
        status: "attention",
      },
      t,
      tStatus,
    );

    expect(room.detail).toBe("诊断室 room-1 当前为进行中状态。");
    expect(evidence.detail).toContain("2 项缺失请求");
    expect(evidence.metric).toBe("已采集 1/2");
    expect(localizeDiagnosisWorkflowReadinessStatus("attention", t)).toBe(
      "需关注",
    );
  });

  it("preserves an already-localized conclusion blocker", () => {
    const blocker = localizeDiagnosisWorkflowReadinessItem(
      {
        detail: "当前操作员无权批准此结论。",
        detailKey: "conclusionBlockReason",
        key: "conclusion",
        label: "Conclusion",
        status: "blocked",
      },
      t,
      tStatus,
    );

    expect(blocker.detail).toBe("当前操作员无权批准此结论。");
    expect(blocker.label).toBe("结论");
  });
});
