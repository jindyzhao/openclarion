import type { useTranslations } from "next-intl";

import type { DiagnosisCollaborationIdentityCoverage } from "./collaboration";

export type DiagnosisCollaborationTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.workspace">
>;

export function localizeDiagnosisCollaborationIdentityCoverage(
  coverage: DiagnosisCollaborationIdentityCoverage,
  t: DiagnosisCollaborationTranslator,
): { detail: string; summary: string } {
  const detail = t(`identityCoverageDetail.${coverage.status}`);
  if (coverage.humanParticipants === 0) {
    return {
      detail,
      summary: t("identityCoverageOnlySystem", {
        count: coverage.systemActors,
      }),
    };
  }
  const parts = [
    t("identityCoverageActiveMatches", {
      human: coverage.humanParticipants,
      synced: coverage.syncedParticipants,
    }),
  ];
  if (coverage.unsyncedParticipants > 0) {
    parts.push(
      t("identityCoverageUnsynced", {
        count: coverage.unsyncedParticipants,
      }),
    );
  }
  if (coverage.inactiveParticipants > 0) {
    parts.push(
      t("identityCoverageInactive", {
        count: coverage.inactiveParticipants,
      }),
    );
  }
  if (coverage.systemActors > 0) {
    parts.push(
      t("identityCoverageSystem", { count: coverage.systemActors }),
    );
  }
  return { detail, summary: parts.join(t("listSeparator")) };
}
