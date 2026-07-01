import type { DiagnosisRoomSummary } from "@/features/diagnosis-room/api";

import type { FinalReportDetail } from "./types";

type LinkedSubReport = FinalReportDetail["linked_sub_reports"][number];

export type ReportDiagnosisRoomHref = {
  pathname: "/diagnosis-room";
  query: Record<string, string>;
};

export function reportDiagnosisRoomHref(
  report: FinalReportDetail,
  subReport: LinkedSubReport,
  rooms: DiagnosisRoomSummary[] = [],
): ReportDiagnosisRoomHref {
  return {
    pathname: "/diagnosis-room",
    query: {
      evidence_snapshot_id: String(subReport.evidence_snapshot_id),
      intent: subReport.diagnosis_conclusion
        ? "review_conclusion"
        : "confidence_review",
      report_id: String(report.id),
      sub_report_id: String(subReport.id),
      ...reportDiagnosisRoomSessionQuery(subReport, rooms),
    },
  };
}

export function reportDiagnosisRoomSessionQuery(
  subReport: LinkedSubReport,
  rooms: DiagnosisRoomSummary[] = [],
): Record<string, string> {
  const sessionID =
    subReport.diagnosis_room?.session_id ??
    subReport.diagnosis_conclusion?.session_id ??
    subReport.diagnosis_progress?.session_id ??
    reportDiagnosisRoomForSubReport(subReport, rooms)?.session_id;
  return sessionID ? { session_id: sessionID } : {};
}

export function reportDiagnosisRoomForSubReport(
  subReport: LinkedSubReport,
  rooms: DiagnosisRoomSummary[] = [],
): DiagnosisRoomSummary | undefined {
  if (subReport.diagnosis_room) {
    return subReport.diagnosis_room;
  }
  const sessionID =
    subReport.diagnosis_conclusion?.session_id ??
    subReport.diagnosis_progress?.session_id;
  if (sessionID) {
    return rooms.find((room) => room.session_id === sessionID);
  }
  return reportDiagnosisRoomForEvidenceSnapshot(subReport.evidence_snapshot_id, rooms);
}

function reportDiagnosisRoomForEvidenceSnapshot(
  evidenceSnapshotID: number,
  rooms: DiagnosisRoomSummary[],
): DiagnosisRoomSummary | undefined {
  return rooms
    .filter((room) => room.evidence_snapshot_id === evidenceSnapshotID)
    .sort(compareDiagnosisRoomsForReuse)[0];
}

function compareDiagnosisRoomsForReuse(
  left: DiagnosisRoomSummary,
  right: DiagnosisRoomSummary,
): number {
  if (left.room_status !== right.room_status) {
    return left.room_status === "open" ? -1 : 1;
  }
  return Date.parse(roomReuseTimestamp(right)) - Date.parse(roomReuseTimestamp(left));
}

function roomReuseTimestamp(room: DiagnosisRoomSummary): string {
  return room.last_activity_at || room.updated_at || room.started_at;
}
