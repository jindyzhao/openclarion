import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";
import zhCN from "../../../messages/zh-CN.json";

const tEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "DiagnosisRoom.workspace",
});
const tZhCN = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.workspace",
});

describe("diagnosis room workspace messages", () => {
  it("localizes workspace actions and validation feedback", () => {
    expect(tEn("createRoom")).toBe("Create Diagnosis Room");
    expect(tZhCN("createRoom")).toBe("创建诊断室");
    expect(tZhCN("evidenceSnapshotPositive")).toBe(
      "证据快照 ID 必须是正整数。",
    );
    expect(tZhCN("collectOperatorEvidence")).toBe("采集操作员证据");
  });

  it("formats catalog-owned counts while preserving external values", () => {
    expect(tEn("roomCount", { count: 1 })).toBe("1 room");
    expect(tEn("roomCount", { count: 2 })).toBe("2 rooms");
    expect(tZhCN("roomCount", { count: 2 })).toBe("2 个诊断室");

    const detail = tZhCN("noRoomLinkedDetail", {
      alert: "checkout-p99",
      id: 42,
    });
    expect(detail).toContain("checkout-p99");
    expect(detail).toContain("#42");
    expect(detail).not.toContain("no diagnosis room");
  });
});
