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
  detail: string;
  evidenceSnapshotID: number;
  notificationChannelProfileID: number | null;
  notificationDetail: string;
  notificationFailed: boolean;
  notificationLabel: string;
  notificationProviderStatus: string;
  recordedAt: string;
  requiresHumanReview: boolean;
  roomCloseDetail: string;
  roomStatus: string;
  sessionID: string;
  status: ReportDecisionRecordStatus;
  statusLabel: string;
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
  return {
    confirmedBy: conclusion?.confirmed_by ?? "",
    detail: decisionRecordDetail(subReport, room),
    evidenceSnapshotID: subReport.evidence_snapshot_id,
    notificationChannelProfileID: notification?.notification_channel_profile_id ?? null,
    notificationDetail: notificationDetail(notification, room, sessionID),
    notificationFailed: notification ? failedNotificationStatus(notification.provider_status) : false,
    notificationLabel: notification ? notificationEventLabel(notification.event_kind) : "No exact delivery proof",
    notificationProviderStatus: notification?.provider_status ?? "",
    recordedAt: diagnosisStateRecordedAt(state),
    requiresHumanReview: conclusion?.requires_human_review ?? state?.requires_human_review ?? false,
    roomCloseDetail: roomCloseDetail(room),
    roomStatus: room?.room_status ?? "not linked",
    sessionID: sessionID ?? "",
    status,
    statusLabel: decisionRecordStatusLabel(status),
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

function decisionRecordDetail(
  subReport: ReportLinkedSubReport,
  room: DiagnosisRoomSummary | null,
): string {
  const readiness = subReportDiagnosisReadiness(subReport);
  const conclusion = diagnosisConclusionState(subReportDiagnosisState(subReport));
  if (readiness.status === "needs_evidence") {
    return readiness.statusDetail;
  }
  if (readiness.status === "failed") {
    return readiness.statusDetail;
  }
  if (conclusion?.confirmed_by) {
    return "Final AI conclusion is confirmed by the recorded operator.";
  }
  if (room?.room_status === "closed") {
    return `Diagnosis room closed with ${room.close_reason || "no close reason"} after ${room.turn_count} turn(s).`;
  }
  if (conclusion) {
    return "Final AI conclusion is stored and waiting for operator confirmation.";
  }
  return "AI consultation must record a conclusion before this subreport can be finalized.";
}

function decisionRecordStatusLabel(status: ReportDecisionRecordStatus): string {
  switch (status) {
    case "confirmed":
      return "Confirmed";
    case "failed":
      return "Failed";
    case "needs_evidence":
      return "Evidence required";
    case "pending_diagnosis":
      return "Diagnosis pending";
    case "recorded":
      return "Conclusion stored";
    case "room_closed":
      return "Room closed";
    case "running":
      return "AI consultation running";
  }
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

function notificationDetail(
  notification: DiagnosisRoomNotificationTimelineEntry | null,
  room: DiagnosisRoomSummary | null,
  sessionID: string | undefined,
): string {
  if (!sessionID) {
    return "No diagnosis session is linked to this subreport yet.";
  }
  if (!room) {
    return "No exact diagnosis room delivery proof is available for this report session.";
  }
  if (!notification) {
    return "The exact diagnosis room has no retained notification delivery event.";
  }
  const parts = [
    notification.provider_status,
    notification.provider_message_id ? `provider message ${notification.provider_message_id}` : "",
    `recorded ${notification.occurred_at}`,
  ].filter(Boolean);
  return parts.join(" / ");
}

function roomCloseDetail(room: DiagnosisRoomSummary | null): string {
  if (!room) {
    return "No exact diagnosis room lifecycle record.";
  }
  if (room.room_status !== "closed") {
    return "Diagnosis room is still open.";
  }
  return `${room.close_reason || "closed"} at ${room.closed_at ?? room.updated_at}`;
}

function notificationEventLabel(eventKind: string): string {
  switch (eventKind) {
    case "diagnosis_room.assistant_turn_notification_sent":
      return "AI update notification";
    case "diagnosis_room.final_ready_notification_sent":
      return "Final-ready notification";
    case "diagnosis_room.close_notification_sent":
      return "Close notification";
    default:
      return eventKind;
  }
}

function failedNotificationStatus(status: string): boolean {
  switch (status.toLowerCase()) {
    case "error":
    case "failed":
      return true;
    default:
      return false;
  }
}
