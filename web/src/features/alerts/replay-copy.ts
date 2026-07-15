import type { Route } from "next";

import { diagnosisRoomLinkHref } from "@/features/diagnosis-room/url-state";
import {
  reportReplayProofTrace,
  type ReportReplayProofTrace,
  type ReportReplayProofTraceItem,
} from "@/features/report-replay/proof-trace";
import { autoDiagnosisConfirmedSnapshotCount } from "@/features/report-replay/replay-response";

import type { ReportReplayTriggerResponse } from "./api";
import type { AlertsTranslator } from "./alerts-copy";

type AutoDiagnosisSummary = NonNullable<
  ReportReplayTriggerResponse["auto_diagnosis"]
>;
type AutoDiagnosisRoom = NonNullable<AutoDiagnosisSummary["rooms"]>[number];

export type AlertReplayProofNextAction =
  | {
      code: "unavailable";
      href?: undefined;
      kind: "unavailable";
      type: "warning";
    }
  | {
      code: "room_timeline";
      href: Route;
      kind: "room";
      type: "warning";
    }
  | {
      code: "report_delivery_confirmed";
      confirmedSnapshots: number;
      href: Route;
      kind: "report";
      type: "info";
    }
  | {
      code: "manual_handoff";
      href: Route;
      kind: "snapshot";
      type: "warning";
    }
  | {
      code: "report_delivery";
      correlationKey: string;
      href: Route;
      kind: "report";
      type: "info";
    };

export type LocalizedAlertReplayProofTrace = {
  detail: string;
  items: Array<{
    actions: Array<{ href: string; label: string }>;
    detail: string;
    key: ReportReplayProofTraceItem["key"];
    status: ReportReplayProofTraceItem["status"];
    title: string;
    value: string;
  }>;
  status: ReportReplayProofTrace["status"];
};

export function alertReplayProofNextAction(
  result: ReportReplayTriggerResponse,
): AlertReplayProofNextAction {
  if (!result.started) {
    return {
      code: "unavailable",
      kind: "unavailable",
      type: "warning",
    };
  }

  const autoDiagnosis = result.auto_diagnosis;
  const confirmed = autoDiagnosis
    ? autoDiagnosisConfirmedSnapshotCount(autoDiagnosis)
    : 0;
  const room = autoDiagnosis?.rooms?.[0] ?? null;
  if (autoDiagnosis && autoDiagnosis.rooms_started > 0 && room) {
    return {
      code: "room_timeline",
      href: replayDiagnosisRoomHref(room),
      kind: "room",
      type: "warning",
    };
  }
  if (
    autoDiagnosis &&
    autoDiagnosis.rooms_started === 0 &&
    autoDiagnosis.rooms_skipped === 0 &&
    confirmed > 0
  ) {
    return {
      code: "report_delivery_confirmed",
      confirmedSnapshots: confirmed,
      href: "/reports" as Route,
      kind: "report",
      type: "info",
    };
  }

  const snapshot = result.snapshots[0] ?? null;
  if (autoDiagnosis && autoDiagnosis.policies_matched > 0 && snapshot) {
    return {
      code: "manual_handoff",
      href: diagnosisHref(snapshot.id),
      kind: "snapshot",
      type: "warning",
    };
  }

  return {
    code: "report_delivery",
    correlationKey: result.correlation_key,
    href: "/reports" as Route,
    kind: "report",
    type: "info",
  };
}

export function localizeAlertReplayProofNextAction(
  action: AlertReplayProofNextAction,
  t: AlertsTranslator,
): {
  actionLabel: string;
  detail: string;
  label: string;
} {
  switch (action.code) {
    case "unavailable":
      return {
        actionLabel: t("reviewReplaySetup"),
        detail: t("replayUnavailableDetail"),
        label: t("replayUnavailable"),
      };
    case "room_timeline":
      return {
        actionLabel: t("openRoomTimeline"),
        detail: t("openRoomTimelineDetail"),
        label: t("reviewRoomTimeline"),
      };
    case "report_delivery_confirmed":
      return {
        actionLabel: t("openReports"),
        detail: t("confirmedReportDeliveryDetail", {
          count: action.confirmedSnapshots,
        }),
        label: t("reviewReportDelivery"),
      };
    case "manual_handoff":
      return {
        actionLabel: t("openDiagnosis"),
        detail: t("autoDiagnosisNoRoom"),
        label: t("reviewHandoff"),
      };
    case "report_delivery":
      return {
        actionLabel: t("openReports"),
        detail: t("reportDeliveryDetail", {
          correlation: action.correlationKey,
        }),
        label: t("reviewReportDelivery"),
      };
  }
}

export function localizeReplayAcceptedMessage(
  result: ReportReplayTriggerResponse,
  t: AlertsTranslator,
): string {
  if (result.snapshots.length === 0) {
    return t("replayNoSnapshots");
  }
  const autoDiagnosis = result.auto_diagnosis;
  const confirmed = autoDiagnosis
    ? autoDiagnosisConfirmedSnapshotCount(autoDiagnosis)
    : 0;
  if (autoDiagnosis && autoDiagnosis.rooms_started > 0) {
    return t("replayDiagnosisStarted", {
      rooms: autoDiagnosis.rooms_started,
      snapshots: result.snapshots.length,
    });
  }
  if (
    autoDiagnosis &&
    autoDiagnosis.rooms_started === 0 &&
    autoDiagnosis.rooms_skipped === 0 &&
    confirmed > 0
  ) {
    return t("replayAlreadyConfirmed", {
      confirmed,
      snapshots: result.snapshots.length,
    });
  }
  if (autoDiagnosis && autoDiagnosis.policies_matched > 0) {
    return t("replayDiagnosisMatched", {
      policies: autoDiagnosis.policies_matched,
      snapshots: result.snapshots.length,
    });
  }
  return t("replayWithSnapshots", { count: result.snapshots.length });
}

export function localizeReportReplayProofTrace(
  result: ReportReplayTriggerResponse,
  t: AlertsTranslator,
): LocalizedAlertReplayProofTrace {
  const trace = reportReplayProofTrace(result);
  return {
    detail: t(replayTraceDetailKeys[trace.status]),
    items: trace.items.map((item) => ({
      actions: (item.actions ?? []).map((action) => ({
        href: action.href,
        label:
          action.kind === "review_room"
            ? t("replayTrace.reviewRoom", {
                id: action.evidenceSnapshotID,
              })
            : t("replayTrace.createRoom", {
                id: action.evidenceSnapshotID,
              }),
      })),
      detail: localizedReplayTraceItemDetail(item, result, t),
      key: item.key,
      status: item.status,
      title: t(replayTraceTitleKeys[item.key]),
      value: localizedReplayTraceItemValue(item, result, t),
    })),
    status: trace.status,
  };
}

const replayTraceDetailKeys = {
  blocked: "replayTrace.detail.blocked",
  pending: "replayTrace.detail.pending",
  ready: "replayTrace.detail.ready",
  review: "replayTrace.detail.review",
} as const;

const replayTraceTitleKeys = {
  ai_diagnosis: "replayTrace.title.ai_diagnosis",
  evidence: "replayTrace.title.evidence",
  notification_proof: "replayTrace.title.notification_proof",
  trigger: "replayTrace.title.trigger",
} as const;

function localizedReplayTraceItemDetail(
  item: ReportReplayProofTraceItem,
  result: ReportReplayTriggerResponse,
  t: AlertsTranslator,
): string {
  switch (item.key) {
    case "trigger":
      return result.started
        ? t("replayTrace.triggerStarted", {
            correlation: result.correlation_key,
            run: result.run_id,
            workflow: result.workflow_id,
          })
        : t("replayTrace.triggerNotStarted", {
            correlation: result.correlation_key,
          });
    case "evidence":
      return result.snapshots.length > 0
        ? t("replayTrace.evidenceRetained", {
            events: result.stats.events_loaded,
            groups: result.stats.groups_built,
            snapshots: result.stats.snapshots_saved,
          })
        : t("replayTrace.evidenceMissing");
    case "ai_diagnosis":
      return localizedAIDiagnosisTraceDetail(result.auto_diagnosis, t);
    case "notification_proof":
      return localizedNotificationTraceDetail(result, t);
  }
}

function localizedReplayTraceItemValue(
  item: ReportReplayProofTraceItem,
  result: ReportReplayTriggerResponse,
  t: AlertsTranslator,
): string {
  switch (item.key) {
    case "trigger":
      return result.started
        ? t("replayTrace.workflowAccepted")
        : t("replayTrace.noWorkflow");
    case "evidence":
      return t("replayTrace.snapshotsSaved", {
        count: result.stats.snapshots_saved,
      });
    case "ai_diagnosis": {
      const autoDiagnosis = result.auto_diagnosis;
      if (!autoDiagnosis || autoDiagnosis.policies_matched === 0) {
        return t("replayTrace.notStarted");
      }
      if (autoDiagnosis.rooms_started > 0) {
        return t("replayTrace.roomCount", {
          count: autoDiagnosis.rooms_started,
        });
      }
      if (autoDiagnosisConfirmedSnapshotCount(autoDiagnosis) > 0) {
        return t("replayTrace.alreadyConfirmed");
      }
      return t("replayTrace.manualHandoff");
    }
    case "notification_proof":
      return localizedNotificationTraceValue(result, t);
  }
}

function localizedAIDiagnosisTraceDetail(
  autoDiagnosis: ReportReplayTriggerResponse["auto_diagnosis"] | undefined,
  t: AlertsTranslator,
): string {
  if (!autoDiagnosis) {
    return t("replayTrace.aiDataMissing");
  }
  if (autoDiagnosis.policies_matched === 0) {
    return t("replayTrace.aiPolicyMissing");
  }
  const confirmed = autoDiagnosisConfirmedSnapshotCount(autoDiagnosis);
  return [
    t("replayTrace.aiMatched", {
      policies: autoDiagnosis.policies_matched,
      rooms: autoDiagnosis.rooms_started,
      snapshots: autoDiagnosis.snapshots,
    }),
    confirmed > 0
      ? t("replayTrace.aiConfirmed", { count: confirmed })
      : null,
    autoDiagnosis.rooms_skipped > 0
      ? t("replayTrace.aiSkipped", { count: autoDiagnosis.rooms_skipped })
      : null,
  ]
    .filter((part): part is string => part !== null)
    .join(" ");
}

function localizedNotificationTraceDetail(
  result: ReportReplayTriggerResponse,
  t: AlertsTranslator,
): string {
  if (!result.started) {
    return t("replayTrace.notificationUnavailable");
  }
  const autoDiagnosis = result.auto_diagnosis;
  if (autoDiagnosis && autoDiagnosis.rooms_started > 0) {
    const confirmed = autoDiagnosisConfirmedSnapshotCount(autoDiagnosis);
    return [
      t("replayTrace.notificationRooms", {
        count: autoDiagnosis.rooms_started,
      }),
      confirmed > 0
        ? t("replayTrace.aiConfirmed", { count: confirmed })
        : null,
      autoDiagnosis.rooms_skipped > 0
        ? t("replayTrace.aiSkipped", { count: autoDiagnosis.rooms_skipped })
        : null,
    ]
      .filter((part): part is string => part !== null)
      .join(" ");
  }
  if (
    autoDiagnosis &&
    autoDiagnosis.rooms_skipped === 0 &&
    autoDiagnosisConfirmedSnapshotCount(autoDiagnosis) > 0
  ) {
    return t("replayTrace.notificationConfirmed", {
      count: autoDiagnosisConfirmedSnapshotCount(autoDiagnosis),
    });
  }
  if (autoDiagnosis && autoDiagnosis.policies_matched > 0) {
    return t("replayTrace.notificationNoRoom");
  }
  return t("replayTrace.notificationReportDelivery");
}

function localizedNotificationTraceValue(
  result: ReportReplayTriggerResponse,
  t: AlertsTranslator,
): string {
  if (!result.started) {
    return t("replayTrace.notAvailable");
  }
  const autoDiagnosis = result.auto_diagnosis;
  if (autoDiagnosis && autoDiagnosis.rooms_started > 0) {
    return t("replayTrace.roomTimelineCount", {
      count: autoDiagnosis.rooms_started,
    });
  }
  if (
    autoDiagnosis &&
    autoDiagnosis.rooms_skipped === 0 &&
    autoDiagnosisConfirmedSnapshotCount(autoDiagnosis) > 0
  ) {
    return t("replayTrace.alreadyConfirmed");
  }
  if (autoDiagnosis && autoDiagnosis.policies_matched > 0) {
    return t("replayTrace.noRoomTimeline");
  }
  return t("replayTrace.reportDelivery");
}

function replayDiagnosisRoomHref(room: AutoDiagnosisRoom): Route {
  return diagnosisRoomLinkHref({
    evidenceSnapshotID: room.evidence_snapshot_id,
    intent: "review_conclusion",
    sessionID: room.session_id,
  }) as Route;
}

function diagnosisHref(snapshotID: number): Route {
  return diagnosisRoomLinkHref({
    evidenceSnapshotID: snapshotID,
    intent: "alert_review",
  }) as Route;
}
