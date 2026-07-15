import type {
  DiagnosisRoomNotificationTimelineEntry,
  DiagnosisRoomSummary,
} from "@/features/diagnosis-room/api";

import { reportDiagnosisRoomForSubReport } from "./report-diagnosis-room-link";
import {
  subReportDiagnosisState,
  subReportDiagnosisReadiness,
  type ReportDiagnosisConclusion,
  type ReportDiagnosisState,
} from "./diagnosis-readiness";
import type { FinalReportDetail } from "./types";

type ReportLinkedSubReport = FinalReportDetail["linked_sub_reports"][number];
type ReportDecisionRecordStatus =
  | "confirmed"
  | "failed"
  | "needs_evidence"
  | "pending_diagnosis"
  | "recorded"
  | "room_closed"
  | "running";

export type ReportDecisionRecord = {
  confirmedBy: string;
  evidenceSnapshotID: number;
  notificationChannelProfileID: number | null;
  notificationEventKind: string;
  notificationFailed: boolean;
  notificationOccurredAt: string;
  notificationProviderMessageID: string;
  notificationProviderStatus: string;
  readiness: ReturnType<typeof subReportDiagnosisReadiness>;
  recordedAt: string;
  requiresHumanReview: boolean;
  roomClosedAt: string;
  roomCloseReason: string;
  roomLinked: boolean;
  roomStatus: string;
  roomTurnCount: number;
  sessionID: string;
  status: ReportDecisionRecordStatus;
  subReportID: number;
  title: string;
  version: string;
};

export function reportDiagnosisDecisionRecords(
  report: FinalReportDetail,
  rooms: DiagnosisRoomSummary[] = [],
): ReportDecisionRecord[] {
  return report.linked_sub_reports.map((subReport) =>
    reportDiagnosisDecisionRecord(subReport, rooms),
  );
}

function reportDiagnosisDecisionRecord(
  subReport: ReportLinkedSubReport,
  rooms: DiagnosisRoomSummary[],
): ReportDecisionRecord {
  const state = subReportDiagnosisState(subReport);
  const conclusion = diagnosisConclusionState(state);
  const room = reportDiagnosisRoomForSubReport(subReport, rooms) ?? null;
  const sessionID = diagnosisStateSessionID(state) ?? room?.session_id;
  const status = decisionRecordStatus(subReport, room);
  const notification = room ? decisionNotification(room, status) : null;
  const readiness = subReportDiagnosisReadiness(subReport);
  return {
    confirmedBy: conclusion?.confirmed_by ?? "",
    evidenceSnapshotID: subReport.evidence_snapshot_id,
    notificationChannelProfileID: notification?.notification_channel_profile_id ?? null,
    notificationEventKind: notification?.event_kind ?? "",
    notificationFailed: notification ? failedNotificationStatus(notification.provider_status) : false,
    notificationOccurredAt: notification?.occurred_at ?? "",
    notificationProviderMessageID: notification?.provider_message_id ?? "",
    notificationProviderStatus: notification?.provider_status ?? "",
    readiness,
    recordedAt: diagnosisStateRecordedAt(state),
    requiresHumanReview: conclusion?.requires_human_review ?? state?.requires_human_review ?? false,
    roomClosedAt: room?.closed_at ?? room?.updated_at ?? "",
    roomCloseReason: room?.close_reason ?? "",
    roomLinked: room !== null,
    roomStatus: room?.room_status ?? "",
    roomTurnCount: room?.turn_count ?? 0,
    sessionID: sessionID ?? "",
    status,
    subReportID: subReport.id,
    title: subReport.title,
    version: conclusion?.conclusion_version ?? "",
  };
}

function decisionRecordStatus(
  subReport: ReportLinkedSubReport,
  room: DiagnosisRoomSummary | null,
): ReportDecisionRecordStatus {
  const readiness = subReportDiagnosisReadiness(subReport);
  const state = subReportDiagnosisState(subReport);
  const conclusion = diagnosisConclusionState(state);
  if (readiness.status === "failed") {
    return "failed";
  }
  if (readiness.status === "needs_evidence") {
    return "needs_evidence";
  }
  if (readiness.status === "pending_diagnosis") {
    return "pending_diagnosis";
  }
  if (conclusion?.confirmed_by) {
    return "confirmed";
  }
  if (room?.room_status === "closed") {
    return "room_closed";
  }
  if (conclusion) {
    return "recorded";
  }
  return "running";
}

function diagnosisStateSessionID(state: ReportDiagnosisState | undefined): string | undefined {
  return state?.session_id;
}

function diagnosisStateRecordedAt(state: ReportDiagnosisState | undefined): string {
  return state?.recorded_at ?? "";
}

function diagnosisConclusionState(
  state: ReportDiagnosisState | undefined,
): ReportDiagnosisConclusion | undefined {
  return state && "content" in state ? state : undefined;
}

function decisionNotification(
  room: DiagnosisRoomSummary,
  status: ReportDecisionRecordStatus,
): DiagnosisRoomNotificationTimelineEntry | null {
  const timeline = room.notification_timeline ?? [];
  const preferredEventKinds = preferredNotificationEventKinds(status);
  for (const eventKind of preferredEventKinds) {
    const notification = latestNotificationByEventKind(timeline, eventKind);
    if (notification) {
      return notification;
    }
  }
  return timeline.length > 0 ? timeline[timeline.length - 1] ?? null : null;
}

function preferredNotificationEventKinds(status: ReportDecisionRecordStatus): string[] {
  switch (status) {
    case "confirmed":
    case "room_closed":
      return [
        "diagnosis_room.close_notification_sent",
        "diagnosis_room.final_ready_notification_sent",
      ];
    case "recorded":
      return ["diagnosis_room.final_ready_notification_sent"];
    case "failed":
    case "needs_evidence":
    case "pending_diagnosis":
    case "running":
      return ["diagnosis_room.assistant_turn_notification_sent"];
  }
}

function latestNotificationByEventKind(
  timeline: DiagnosisRoomNotificationTimelineEntry[],
  eventKind: string,
): DiagnosisRoomNotificationTimelineEntry | null {
  for (let index = timeline.length - 1; index >= 0; index -= 1) {
    const notification = timeline[index];
    if (notification?.event_kind === eventKind) {
      return notification;
    }
  }
  return null;
}

function failedNotificationStatus(status: string): boolean {
  switch (status.trim().toLowerCase()) {
    case "error":
    case "failed":
      return true;
    default:
      return false;
  }
}
