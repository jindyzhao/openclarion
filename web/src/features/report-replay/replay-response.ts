import type { components } from "@/lib/api/openapi";

export type ReportReplayTriggerResponse =
  components["schemas"]["ReportReplayTriggerResponse"];

type ReportReplayStats = components["schemas"]["ReportReplayStats"];
type ReportReplaySnapshotRef =
  components["schemas"]["ReportReplaySnapshotRef"];
type AutoDiagnosisSummary =
  components["schemas"]["AlertmanagerWebhookAutoDiagnosisSummary"];
type AutoDiagnosisRoom =
  components["schemas"]["AlertmanagerWebhookAutoDiagnosisRoom"];

export function autoDiagnosisConfirmedSnapshotCount(
  autoDiagnosis: AutoDiagnosisSummary,
): number {
  return Math.max(
    0,
    autoDiagnosis.snapshots -
      autoDiagnosis.rooms_started -
      autoDiagnosis.rooms_skipped,
  );
}

export function normalizedReportReplayTriggerResponse(
  value: unknown,
): ReportReplayTriggerResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  const started = value.started;
  const correlationKey = value.correlation_key;
  const workflowID = value.workflow_id;
  const runID = value.run_id;
  const stats = normalizedReportReplayStats(value.stats);
  const snapshots = normalizedArray(
    value.snapshots,
    normalizedReportReplaySnapshotRef,
  );
  const autoDiagnosis = normalizedAutoDiagnosisSummary(value.auto_diagnosis);
  if (
    typeof started !== "boolean" ||
    !isCleanText(correlationKey) ||
    !isOptionalCleanText(workflowID) ||
    !isOptionalCleanText(runID) ||
    stats === null ||
    snapshots === null ||
    autoDiagnosis === null
  ) {
    return null;
  }
  if (started && snapshots.length > 0 && (workflowID === "" || runID === "")) {
    return null;
  }
  if (autoDiagnosis !== undefined) {
    const snapshotIDs = new Set(snapshots.map((snapshot) => snapshot.id));
    if (
      autoDiagnosis.snapshots !== snapshots.length ||
      (autoDiagnosis.rooms?.some(
        (room) => !snapshotIDs.has(room.evidence_snapshot_id),
      ) ?? false) ||
      autoDiagnosis.skipped_snapshot_ids.some(
        (snapshotID) => !snapshotIDs.has(snapshotID),
      )
    ) {
      return null;
    }
  }
  return {
    ...(autoDiagnosis === undefined
      ? {}
      : { auto_diagnosis: autoDiagnosis }),
    correlation_key: correlationKey,
    run_id: runID,
    snapshots,
    started,
    stats,
    workflow_id: workflowID,
  };
}

function normalizedReportReplayStats(value: unknown): ReportReplayStats | null {
  if (!isRecord(value)) {
    return null;
  }
  const ingested = normalizedReportReplayIngestStats(value.ingested);
  if (
    ingested === null ||
    !isNonNegativeInteger(value.events_loaded) ||
    !isNonNegativeInteger(value.groups_built) ||
    !isNonNegativeInteger(value.groups_saved) ||
    !isNonNegativeInteger(value.groups_refreshed) ||
    !isNonNegativeInteger(value.groups_existing) ||
    !isNonNegativeInteger(value.snapshots_saved) ||
    !isNonNegativeInteger(value.snapshots_duplicate) ||
    !isNonNegativeInteger(value.groups_closed) ||
    !isNonNegativeInteger(value.failed)
  ) {
    return null;
  }
  return {
    events_loaded: value.events_loaded,
    failed: value.failed,
    groups_built: value.groups_built,
    groups_closed: value.groups_closed,
    groups_existing: value.groups_existing,
    groups_refreshed: value.groups_refreshed,
    groups_saved: value.groups_saved,
    ingested,
    snapshots_duplicate: value.snapshots_duplicate,
    snapshots_saved: value.snapshots_saved,
  };
}

function normalizedReportReplayIngestStats(
  value: unknown,
): ReportReplayStats["ingested"] | null {
  if (
    !isRecord(value) ||
    !isNonNegativeInteger(value.total) ||
    !isNonNegativeInteger(value.saved) ||
    !isNonNegativeInteger(value.duplicate) ||
    !isNonNegativeInteger(value.failed)
  ) {
    return null;
  }
  return {
    duplicate: value.duplicate,
    failed: value.failed,
    saved: value.saved,
    total: value.total,
  };
}

function normalizedReportReplaySnapshotRef(
  value: unknown,
): ReportReplaySnapshotRef | null {
  if (
    !isRecord(value) ||
    !isPositiveInteger(value.id) ||
    !isNonNegativeInteger(value.group_index) ||
    !isPositiveInteger(value.event_count)
  ) {
    return null;
  }
  return {
    event_count: value.event_count,
    group_index: value.group_index,
    id: value.id,
  };
}

function normalizedAutoDiagnosisSummary(
  value: unknown,
): AutoDiagnosisSummary | undefined | null {
  if (value === undefined) {
    return undefined;
  }
  if (
    !isRecord(value) ||
    !isNonNegativeInteger(value.policies_matched) ||
    !isNonNegativeInteger(value.snapshots) ||
    !isNonNegativeInteger(value.rooms_started) ||
    !isNonNegativeInteger(value.rooms_skipped)
  ) {
    return null;
  }
  const rooms = normalizedArray(value.rooms ?? [], normalizedAutoDiagnosisRoom);
  const skippedSnapshotIDs = normalizedArray(
    value.skipped_snapshot_ids,
    normalizedPositiveInteger,
  );
  if (
    rooms === null ||
    rooms.length !== value.rooms_started ||
    skippedSnapshotIDs === null ||
    skippedSnapshotIDs.length !== value.rooms_skipped ||
    value.rooms_started + value.rooms_skipped > value.snapshots
  ) {
    return null;
  }
  return {
    policies_matched: value.policies_matched,
    rooms,
    rooms_skipped: value.rooms_skipped,
    rooms_started: value.rooms_started,
    skipped_snapshot_ids: skippedSnapshotIDs,
    snapshots: value.snapshots,
  };
}

function normalizedAutoDiagnosisRoom(value: unknown): AutoDiagnosisRoom | null {
  if (
    !isRecord(value) ||
    !isPositiveInteger(value.policy_id) ||
    !isPositiveInteger(value.evidence_snapshot_id) ||
    !isCleanText(value.session_id) ||
    !isCleanText(value.initial_message_id) ||
    !isCleanText(value.workflow_id) ||
    !isCleanText(value.run_id)
  ) {
    return null;
  }
  return {
    evidence_snapshot_id: value.evidence_snapshot_id,
    initial_message_id: value.initial_message_id,
    policy_id: value.policy_id,
    run_id: value.run_id,
    session_id: value.session_id,
    workflow_id: value.workflow_id,
  };
}

function normalizedArray<T>(
  value: unknown,
  normalizeItem: (item: unknown) => T | null,
): T[] | null {
  if (!Array.isArray(value)) {
    return null;
  }
  const out: T[] = [];
  for (const item of value) {
    const normalized = normalizeItem(item);
    if (normalized === null) {
      return null;
    }
    out.push(normalized);
  }
  return out;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isPositiveInteger(value: unknown): value is number {
  return isNonNegativeInteger(value) && value > 0;
}

function normalizedPositiveInteger(value: unknown): number | null {
  return isPositiveInteger(value) ? value : null;
}

function isNonNegativeInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && typeof value === "number" && value >= 0;
}

function isOptionalCleanText(value: unknown): value is string {
  return value === "" || isCleanText(value);
}

function isCleanText(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.trim() === value &&
    value !== "" &&
    !/[\u0000-\u001f\u007f]/u.test(value)
  );
}
