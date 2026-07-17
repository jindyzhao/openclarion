import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";
import { localizeDiagnosisCollaborationIdentityCoverage } from "./collaboration-copy";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.workspace",
});

describe("diagnosis collaboration copy", () => {
  it("localizes mixed directory identity coverage from structured counts", () => {
    expect(
      localizeDiagnosisCollaborationIdentityCoverage(
        {
          humanParticipants: 3,
          inactiveParticipants: 1,
          status: "review",
          syncedParticipants: 1,
          systemActors: 1,
          unsyncedParticipants: 1,
        },
        t,
      ),
    ).toEqual({
      detail: "依赖此诊断室进行多操作员身份归因前，请检查本地目录同步。",
      summary: "1/3 个活跃目录匹配、1 个未同步、1 个非活跃档案、1 个系统参与者",
    });
  });

  it("localizes system-only coverage", () => {
    expect(
      localizeDiagnosisCollaborationIdentityCoverage(
        {
          humanParticipants: 0,
          inactiveParticipants: 0,
          status: "empty",
          syncedParticipants: 0,
          systemActors: 2,
          unsyncedParticipants: 0,
        },
        t,
      ).summary,
    ).toBe("2 个系统参与者，无人工参与者");
  });
});
