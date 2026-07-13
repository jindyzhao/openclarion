import { diagnosisRoomLinkHref } from "@/features/diagnosis-room/url-state";
import type { components } from "@/lib/api/openapi";

import { autoDiagnosisConfirmedSnapshotCount } from "./replay-response";

type ReportReplayTriggerResponse = components["schemas"]["ReportReplayTriggerResponse"];
type ReplayProofTraceStatus = "ready" | "review" | "pending" | "blocked";

type ReportReplayProofTraceAction = {
  detail: string;
  href: string;
  label: string;
};

type ReportReplayProofTraceItem = {
  actions?: ReportReplayProofTraceAction[];
  detail: string;
  status: ReplayProofTraceStatus;
  title: string;
  value: string;
};

export type ReportReplayProofTrace = {
  detail: string;
  items: ReportReplayProofTraceItem[];
  status: ReplayProofTraceStatus;
};

export function reportReplayProofTrace(result: ReportReplayTriggerResponse): ReportReplayProofTrace {
  const autoDiagnosis = result.auto_diagnosis;
  const items: ReportReplayProofTraceItem[] = [
    {
      detail: result.started
        ? `Report workflow ${result.workflow_id} accepted run ${result.run_id} for correlation ${result.correlation_key}.`
        : `Replay did not start a report workflow for correlation ${result.correlation_key} because no evidence snapshots were available.`,
      status: result.started ? "ready" : "pending",
      title: "Trigger",
      value: result.started ? "Workflow accepted" : "No workflow"
    },
    {
      detail:
        result.snapshots.length > 0
          ? `${result.stats.events_loaded} events loaded, ${result.stats.groups_built} groups built, ${result.stats.snapshots_saved} snapshots saved.`
          : "No evidence snapshots were created for this replay window.",
      status: result.snapshots.length > 0 ? (result.stats.failed > 0 ? "review" : "ready") : "pending",
      title: "Evidence",
      value: `${result.stats.snapshots_saved} saved`
    },
    {
      detail: replayAutoDiagnosisTraceDetail(autoDiagnosis),
      status: replayAutoDiagnosisTraceStatus(autoDiagnosis),
      title: "AI diagnosis",
      value: replayAutoDiagnosisTraceValue(autoDiagnosis)
    }
  ];
  items.push(replayNotificationProofTrace(result));
  const status = aggregateReplayReadiness(items.map((item) => item.status));

  return {
    detail: replayProofTraceDetail(status),
    items,
    status
  };
}

function replayNotificationProofTrace(result: ReportReplayTriggerResponse): ReportReplayProofTraceItem {
  if (!result.started) {
    return {
      detail: "Notification proof is unavailable until replay starts a report workflow.",
      status: "pending",
      title: "Notification proof",
      value: "Not available"
    };
  }

  const autoDiagnosis = result.auto_diagnosis;
  if (autoDiagnosis && autoDiagnosis.rooms_started > 0) {
    return {
      actions: replayNotificationProofActions(result),
      detail: replayRoomNotificationTraceDetail(autoDiagnosis),
      status: "review",
      title: "Notification proof",
      value: autoDiagnosis.rooms_started === 1 ? "Room timeline" : "Room timelines"
    };
  }

  if (
    autoDiagnosis &&
    autoDiagnosis.rooms_skipped === 0 &&
    autoDiagnosisConfirmedSnapshotCount(autoDiagnosis) > 0
  ) {
    const confirmed = autoDiagnosisConfirmedSnapshotCount(autoDiagnosis);
    return {
      detail: `${pluralizeCount(confirmed, "snapshot")} already ${confirmed === 1 ? "has" : "have"} a human-confirmed conclusion, so no new diagnosis room or consultation notification was required.`,
      status: "ready",
      title: "Notification proof",
      value: "Already confirmed"
    };
  }

  if (autoDiagnosis && autoDiagnosis.policies_matched > 0) {
    return {
      actions: replayNotificationProofActions(result),
      detail:
        "Automatic diagnosis matched this replay, but no room workflow was accepted. Review the AI diagnosis handoff before checking notification delivery.",
      status: "review",
      title: "Notification proof",
      value: "No room timeline"
    };
  }

  return {
    detail:
      "Replay accepted the report workflow. Verify asynchronous notification delivery in the final report delivery timeline because this response does not include delivery results.",
    status: "pending",
    title: "Notification proof",
    value: "Report delivery"
  };
}

function replayNotificationProofActions(result: ReportReplayTriggerResponse): ReportReplayProofTraceAction[] {
  const autoDiagnosis = result.auto_diagnosis;
  if (!autoDiagnosis) {
    return [];
  }
  return [
    ...(autoDiagnosis.rooms ?? []).map((room) => ({
      detail: "Open the automatic diagnosis room and review its notification timeline.",
      href: diagnosisRoomURL(room),
      label: `Review room #${room.evidence_snapshot_id}`
    })),
    ...skippedAutoDiagnosisSnapshots(result).map((snapshot) => ({
      detail: "Open the retained evidence snapshot and create a manual diagnosis room.",
      href: diagnosisSnapshotURL(snapshot.id),
      label: `Create room #${snapshot.id}`
    }))
  ];
}

function skippedAutoDiagnosisSnapshots(result: ReportReplayTriggerResponse) {
  const autoDiagnosis = result.auto_diagnosis;
  if (!autoDiagnosis || autoDiagnosis.rooms_skipped <= 0) {
    return [];
  }
  const snapshotsByID = new Map(result.snapshots.map((snapshot) => [snapshot.id, snapshot]));
  return autoDiagnosis.skipped_snapshot_ids.flatMap((snapshotID) => {
    const snapshot = snapshotsByID.get(snapshotID);
    return snapshot === undefined ? [] : [snapshot];
  });
}

function diagnosisRoomURL(
  room: NonNullable<NonNullable<ReportReplayTriggerResponse["auto_diagnosis"]>["rooms"]>[number]
): string {
  return diagnosisRoomLinkHref({
    evidenceSnapshotID: room.evidence_snapshot_id,
    intent: "review_conclusion",
    sessionID: room.session_id
  });
}

function diagnosisSnapshotURL(snapshotID: number): string {
  return diagnosisRoomLinkHref({
    evidenceSnapshotID: snapshotID,
    intent: "alert_review"
  });
}

function replayRoomNotificationTraceDetail(
  autoDiagnosis: NonNullable<ReportReplayTriggerResponse["auto_diagnosis"]>
): string {
  const started = pluralizeCount(autoDiagnosis.rooms_started, "automatic diagnosis room");
  const skipped =
    autoDiagnosis.rooms_skipped > 0
      ? ` ${pluralizeCount(autoDiagnosis.rooms_skipped, "snapshot")} ${autoDiagnosis.rooms_skipped === 1 ? "remains" : "remain"} without automatic room timelines because the safety cap was reached.`
      : "";
  const confirmed = autoDiagnosisConfirmedSnapshotCount(autoDiagnosis);
  const confirmedDetail =
    confirmed > 0
      ? ` ${pluralizeCount(confirmed, "snapshot")} already ${confirmed === 1 ? "has" : "have"} a human-confirmed conclusion.`
      : "";
  return `${started} started; verify assistant and final-ready notifications in the diagnosis-room notification timeline.${confirmedDetail}${skipped}`;
}

function replayAutoDiagnosisTraceStatus(
  autoDiagnosis: ReportReplayTriggerResponse["auto_diagnosis"] | undefined
): ReplayProofTraceStatus {
  if (!autoDiagnosis || autoDiagnosis.policies_matched === 0) {
    return "pending";
  }
  if (autoDiagnosis.rooms_skipped > 0) {
    return "review";
  }
  return autoDiagnosis.rooms_started > 0 || autoDiagnosisConfirmedSnapshotCount(autoDiagnosis) > 0
    ? "ready"
    : "review";
}

function replayAutoDiagnosisTraceValue(
  autoDiagnosis: ReportReplayTriggerResponse["auto_diagnosis"] | undefined
): string {
  if (!autoDiagnosis || autoDiagnosis.policies_matched === 0) {
    return "Not started";
  }
  if (autoDiagnosis.rooms_started > 0) {
    return `${autoDiagnosis.rooms_started} room${autoDiagnosis.rooms_started === 1 ? "" : "s"}`;
  }
  if (autoDiagnosisConfirmedSnapshotCount(autoDiagnosis) > 0) {
    return "Already confirmed";
  }
  return "Manual handoff";
}

function replayAutoDiagnosisTraceDetail(
  autoDiagnosis: ReportReplayTriggerResponse["auto_diagnosis"] | undefined
): string {
  if (!autoDiagnosis) {
    return "Replay response did not include automatic diagnosis data.";
  }
  if (autoDiagnosis.policies_matched === 0) {
    return "No automatic diagnosis policy matched this replay output.";
  }
  const skipped = autoDiagnosis.rooms_skipped > 0
    ? ` ${pluralizeCount(autoDiagnosis.rooms_skipped, "snapshot")} ${autoDiagnosis.rooms_skipped === 1 ? "remains" : "remain"} for manual room creation.`
    : "";
  const confirmed = autoDiagnosisConfirmedSnapshotCount(autoDiagnosis);
  const confirmedDetail =
    confirmed > 0
      ? ` ${pluralizeCount(confirmed, "snapshot")} already ${confirmed === 1 ? "has" : "have"} a human-confirmed conclusion.`
      : "";
  return `${autoDiagnosis.policies_matched} policy matched ${autoDiagnosis.snapshots} snapshots and started ${autoDiagnosis.rooms_started} rooms.${confirmedDetail}${skipped}`;
}

function pluralizeCount(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

function aggregateReplayReadiness(statuses: ReplayProofTraceStatus[]): ReplayProofTraceStatus {
  if (statuses.includes("blocked")) {
    return "blocked";
  }
  if (statuses.includes("review")) {
    return "review";
  }
  if (statuses.includes("pending")) {
    return "pending";
  }
  return "ready";
}

function replayProofTraceDetail(status: ReplayProofTraceStatus): string {
  switch (status) {
    case "ready":
      return "Replay accepted, evidence was retained, and automatic diagnosis has proof in this response.";
    case "review":
      return "Replay produced usable proof, with downstream AI or notification evidence to review.";
    case "pending":
      return "Replay proof is partial; verify asynchronous notification delivery in downstream timelines.";
    case "blocked":
      return "Replay proof is blocked.";
  }
}
