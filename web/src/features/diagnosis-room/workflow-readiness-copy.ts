import type { useTranslations } from "next-intl";

import type {
  DiagnosisWorkflowReadinessDetailKey,
  DiagnosisWorkflowReadinessItem,
  DiagnosisWorkflowReadinessStatus,
} from "./workflow-readiness";
import {
  localizeDiagnosisRoomStatus,
  type DiagnosisRoomStatusTranslator,
} from "./status-copy";

type WorkflowReadinessTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.workflowReadiness">
>;

export function localizeDiagnosisWorkflowReadinessItem(
  item: DiagnosisWorkflowReadinessItem,
  t: WorkflowReadinessTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): DiagnosisWorkflowReadinessItem {
  return {
    ...item,
    detail: localizeDetail(item, t, tStatus),
    label: localizeLabel(item.key, t),
    metric:
      item.metricValues === undefined
        ? item.metric
        : t("evidenceMetric", item.metricValues),
  };
}

export function localizeDiagnosisWorkflowReadinessStatus(
  status: DiagnosisWorkflowReadinessStatus,
  t: WorkflowReadinessTranslator,
): string {
  switch (status) {
    case "attention":
      return t("statusAttention");
    case "blocked":
      return t("statusBlocked");
    case "pending":
      return t("statusPending");
    case "ready":
      return t("statusReady");
  }
}

function localizeLabel(
  key: DiagnosisWorkflowReadinessItem["key"],
  t: WorkflowReadinessTranslator,
): string {
  switch (key) {
    case "identity":
      return t("labelIdentity");
    case "room":
      return t("labelRoom");
    case "connection":
      return t("labelConnection");
    case "permissions":
      return t("labelPermissions");
    case "evidence":
      return t("labelEvidence");
    case "conclusion":
      return t("labelConclusion");
  }
}

function localizeDetail(
  item: DiagnosisWorkflowReadinessItem,
  t: WorkflowReadinessTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): string {
  if (item.detailKey === "conclusionBlockReason") {
    return item.detail;
  }
  if (item.detailKey === "conclusionExternal") {
    switch (item.status) {
      case "ready":
        return t("conclusionExternalReady");
      case "blocked":
        return t("conclusionExternalBlocked");
      case "attention":
        return t("conclusionExternalAttention");
      case "pending":
        return t("conclusionExternalPending");
    }
  }
  const values = { ...item.detailValues };
  if (
    item.detailKey === "roomReady" ||
    item.detailKey === "roomWorkflowUnavailable" ||
    item.detailKey === "connectionWorkflowUnavailable" ||
    item.detailKey === "conclusionStatus"
  ) {
    values.status = localizeDiagnosisRoomStatus(
      String(item.detailValues?.status ?? "unknown"),
      tStatus,
    );
  }
  return t(detailMessageKey(item.detailKey), values);
}

function detailMessageKey(
  key: Exclude<
    DiagnosisWorkflowReadinessDetailKey,
    "conclusionBlockReason" | "conclusionExternal"
  >,
) {
  return key;
}
