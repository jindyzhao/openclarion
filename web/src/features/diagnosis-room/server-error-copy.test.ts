import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";
import { localizeDiagnosisServerErrorDisplay } from "./server-error-copy";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.serverError",
});

describe("diagnosis server error copy", () => {
  it("localizes confirmation recovery while retaining server detail", () => {
    expect(
      localizeDiagnosisServerErrorDisplay(
        {
          code: "confirm_rejected",
          message: "missing evidence",
        },
        t,
      ),
    ).toEqual({
      actionLabel: "审核证据任务",
      actionTitle: "跳转到包含阻止确认的证据或重新评估任务的审核队列。",
      description:
        "missing evidence 请打开审核队列，解决列出的证据或重新评估任务，必要时让 AI 重新评估，然后再次确认。",
      message: "暂时无法确认结论",
      type: "warning",
    });
  });

  it("localizes recoverable request guidance by error code", () => {
    expect(
      localizeDiagnosisServerErrorDisplay(
        { code: "llm_timeout", message: "upstream timed out" },
        t,
      ),
    ).toEqual({
      description:
        "upstream timed out 请查询最新诊断室状态；如果轮次没有推进，请重试操作员消息或提供范围更窄的证据。",
      message: "诊断请求失败：llm_timeout",
      type: "warning",
    });
  });
});
