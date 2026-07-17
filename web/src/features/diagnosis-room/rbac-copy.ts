import type { useTranslations } from "next-intl";

import { diagnosisRoomRBACBlocker } from "./rbac-capabilities";

export type DiagnosisRoomRBACTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.rbac">
>;

type DiagnosisRoomRBACBlockInput = Parameters<
  typeof diagnosisRoomRBACBlocker
>[0];

export function localizeDiagnosisRoomRBACBlockReason(
  input: DiagnosisRoomRBACBlockInput,
  t: DiagnosisRoomRBACTranslator,
): string {
  const blocker = diagnosisRoomRBACBlocker(input);
  if (blocker === null) {
    return "";
  }
  if (blocker.kind === "checking") {
    return t("checking");
  }
  switch (blocker.action) {
    case "administer":
      return t("deniedAdminister");
    case "approve":
      return t("deniedApprove");
    case "create":
      return t("deniedCreate");
    case "participate":
      return t("deniedParticipate");
    case "read":
      return t("deniedRead");
  }
}
