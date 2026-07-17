import type { useTranslations } from "next-intl";

import {
  diagnosisActorApprovalBlocker,
} from "./approval-state";
import type { DiagnosisApprovalAuthority } from "./types";

export type DiagnosisApprovalTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.approval">
>;

type DiagnosisActorApprovalInput = Parameters<
  typeof diagnosisActorApprovalBlocker
>[0];

export function localizeDiagnosisActorApprovalBlockReason(
  input: DiagnosisActorApprovalInput,
  t: DiagnosisApprovalTranslator,
): string {
  const blocker = diagnosisActorApprovalBlocker(input);
  if (blocker === null) {
    return "";
  }
  switch (blocker.kind) {
    case "approval_in_flight":
      return t("inProgress");
    case "already_approved":
      return t("alreadyApproved");
    case "authority_satisfied":
      return t("authoritySatisfied", {
        authority: localizeApprovalAuthority(blocker.authority, t),
      });
  }
}

function localizeApprovalAuthority(
  authority: DiagnosisApprovalAuthority,
  t: DiagnosisApprovalTranslator,
): string {
  return authority === "owner" ? t("owner") : t("leader");
}
