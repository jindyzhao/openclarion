import { fetchAlerts, type AlertEventSummary } from "@/features/alerts/api";
import {
  fetchDiagnosisHandoffs,
  fetchDiagnosisRooms,
  type DiagnosisHandoffListResponse,
} from "@/features/diagnosis-room/api";
import { DiagnosisRoomView, type DiagnosisAlertContext } from "@/features/diagnosis-room/diagnosis-room-view";
import { boundedURLTextValue } from "@/features/diagnosis-room/url-state";
import type { RequestJSONOptions } from "@/lib/api/client";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

type DiagnosisRoomPageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export default async function DiagnosisRoomPage({ searchParams }: DiagnosisRoomPageProps) {
  const params = await searchParams;
  const evidenceSnapshotID = positiveIntegerSearchParam(params.evidence_snapshot_id);
  const sessionID = boundedURLTextValue(firstSearchParam(params.session_id), 128);
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const [recentRoomsResult, handoffsResult] = await Promise.all([
    fetchDiagnosisRooms(20, backendRequestOptions),
    fetchDiagnosisHandoffs(100, backendRequestOptions)
  ]);
  const alertContext =
    evidenceSnapshotID === undefined
      ? undefined
      : await resolveAlertContextForSnapshot(
          evidenceSnapshotID,
          handoffsResult,
          backendRequestOptions,
        );

  return (
    <DiagnosisRoomView
      alertContext={alertContext}
      handoffsResult={handoffsResult}
      initialEvidenceSnapshotID={evidenceSnapshotID}
      initialSessionID={sessionID}
      recentRoomsResult={recentRoomsResult}
    />
  );
}

async function resolveAlertContextForSnapshot(
  evidenceSnapshotID: number,
  handoffsResult: Awaited<ReturnType<typeof fetchDiagnosisHandoffs>>,
  backendRequestOptions: Pick<RequestJSONOptions, "headers">,
): Promise<DiagnosisAlertContext | undefined> {
  if (handoffsResult.ok) {
    const handoffContext = alertContextForHandoffSnapshot(handoffsResult.data.items, evidenceSnapshotID);
    if (handoffContext !== undefined) {
      return handoffContext;
    }
  }

  const alertsResult = await fetchAlerts(100, backendRequestOptions);
  if (!alertsResult.ok) {
    return undefined;
  }
  return alertContextForSnapshot(alertsResult.data.items, evidenceSnapshotID);
}

function alertContextForHandoffSnapshot(
  handoffs: DiagnosisHandoffListResponse["items"],
  evidenceSnapshotID: number
): DiagnosisAlertContext | undefined {
  const handoff = handoffs.find((item) => item.evidence_snapshot.id === evidenceSnapshotID);
  const alert = handoff?.alerts[0];
  if (handoff === undefined || alert === undefined) {
    return undefined;
  }
  return { alert, snapshot: handoff.evidence_snapshot };
}

function alertContextForSnapshot(
  alerts: AlertEventSummary[],
  evidenceSnapshotID: number
): DiagnosisAlertContext | undefined {
  for (const alert of alerts) {
    const snapshot = alert.linked_evidence_snapshots.find((item) => item.id === evidenceSnapshotID);
    if (snapshot) {
      return { alert, snapshot };
    }
  }
  return undefined;
}

function positiveIntegerSearchParam(value: string | string[] | undefined): number | undefined {
  const raw = Array.isArray(value) ? value[0] : value;
  if (raw === undefined || !/^[0-9]+$/.test(raw.trim())) {
    return undefined;
  }
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined;
}

function firstSearchParam(
  value: string | string[] | undefined,
): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}
