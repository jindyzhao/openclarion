import type { useTranslations } from "next-intl";

import {
  diagnosisServerErrorPresentation,
  type DiagnosisServerError,
} from "./server-error";

export type DiagnosisServerErrorDisplay = {
  actionLabel?: string;
  actionTitle?: string;
  description: string;
  message: string;
  type: "error" | "warning";
};

export type DiagnosisServerErrorTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.serverError">
>;

export function localizeDiagnosisServerErrorDisplay(
  error: DiagnosisServerError | null,
  t: DiagnosisServerErrorTranslator,
): DiagnosisServerErrorDisplay | null {
  const presentation = diagnosisServerErrorPresentation(error);
  if (presentation === null) {
    return null;
  }
  switch (presentation.kind) {
    case "confirmation_rejected":
      return {
        actionLabel: t("reviewEvidenceTasks"),
        actionTitle: t("reviewEvidenceTasksTitle"),
        description: t("confirmationRejectedDescription", {
          detail: presentation.detail,
        }),
        message: t("confirmationRejected"),
        type: presentation.type,
      };
    case "recoverable":
      return {
        description: t("recoverableDescription", {
          detail: presentation.detail,
        }),
        message: t("requestFailed", { code: presentation.code }),
        type: presentation.type,
      };
    case "fatal":
      return {
        description: presentation.detail,
        message: t("requestFailed", { code: presentation.code }),
        type: presentation.type,
      };
  }
}
