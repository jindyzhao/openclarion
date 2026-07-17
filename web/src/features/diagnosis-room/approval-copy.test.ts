import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";
import { localizeDiagnosisActorApprovalBlockReason } from "./approval-copy";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.approval",
});

describe("diagnosis approval copy", () => {
  it("localizes structured authority blockers", () => {
    expect(
      localizeDiagnosisActorApprovalBlockReason(
        {
          actorSubject: "iam:leader-2",
          approvalInFlight: false,
          approvals: [
            {
              actor_subject: "iam:leader-1",
              approved_at: "2026-07-16T00:00:00Z",
              authority: "leader",
              conclusion_digest: "a".repeat(64),
              id: 1,
              reason: "human_confirmed",
            },
          ],
          conclusionDigest: "a".repeat(64),
          mode: "owner_and_leader",
          ownerSubject: "iam:owner-1",
        },
        t,
      ),
    ).toBe("主管审批已满足。");
  });
});
