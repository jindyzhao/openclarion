import type {
  ReportWorkflowSchedule,
  ReportWorkflowScheduleFormState,
  ReportWorkflowScheduleWriteRequest
} from "./types";
import { formatDurationSeconds } from "../format";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

type ScheduleEnablementReadinessStatus = "ready" | "blocked";
type ScheduleDraftReadinessStatus = "ready" | "pending" | "blocked";
type SearchParamValue = string | string[] | undefined;

export type ReportWorkflowScheduleLaunchIntentName = "create-schedule";

export type ReportWorkflowScheduleLaunchIntent = {
  message: string;
  name: string;
  policyID: number | null;
  temporalScheduleID: string;
};

export type ReportWorkflowScheduleEnablementReadiness = {
  blockers: string[];
  detail: string;
  label: string;
  status: ScheduleEnablementReadinessStatus;
};

export type ReportWorkflowScheduleDraftReadiness = {
  detail: string;
  label: string;
  status: ScheduleDraftReadinessStatus;
};

type ReportWorkflowScheduleProofOutcomeItem = {
  detail: string;
  status: ScheduleDraftReadinessStatus;
  title: string;
  value: string;
};

export type ReportWorkflowScheduleProofOutcome = {
  detail: string;
  items: ReportWorkflowScheduleProofOutcomeItem[];
  status: ScheduleDraftReadinessStatus;
};

export function emptyReportWorkflowScheduleForm(): ReportWorkflowScheduleFormState {
  return {
    name: "",
    reportWorkflowPolicyID: null,
    temporalScheduleID: "",
    intervalSeconds: 86400,
    offsetSeconds: 0,
    replayWindowSeconds: 3600,
    replayDelaySeconds: 300,
    replayLimit: 10000,
    catchupWindowSeconds: 3600
  };
}

export function reportWorkflowScheduleLaunchHref({
  intent,
  policyID
}: {
  intent: ReportWorkflowScheduleLaunchIntentName;
  policyID?: number | null;
}): string {
  const params = new URLSearchParams({ intent });
  if (positiveInteger(policyID ?? null)) {
    params.set("policy_id", String(policyID));
  }
  return `/settings/report-workflow-schedules?${params.toString()}`;
}

export function reportWorkflowScheduleLaunchIntentFromSearchParams(
  searchParams: Record<string, SearchParamValue>
): ReportWorkflowScheduleLaunchIntent | null {
  switch (firstSearchParamValue(searchParams.intent)) {
    case "create-schedule": {
      const policyID = positiveSearchParamInteger(firstSearchParamValue(searchParams.policy_id));
      return {
        message: "Prepared an hourly replay schedule from the settings overview proof action.",
        name: "Hourly report replay",
        policyID,
        temporalScheduleID: policyID === null ? "" : `openclarion-report-policy-${policyID}-hourly`
      };
    }
    default:
      return null;
  }
}

export function reportWorkflowScheduleLaunchIntentKey(
  launchIntent: ReportWorkflowScheduleLaunchIntent | null
): string {
  if (launchIntent === null) {
    return "default";
  }
  return `${launchIntent.policyID ?? "auto"}:${launchIntent.name}:${launchIntent.temporalScheduleID || "blank"}`;
}

export function scheduleToFormState(schedule: ReportWorkflowSchedule): ReportWorkflowScheduleFormState {
  return {
    name: schedule.name,
    reportWorkflowPolicyID: schedule.report_workflow_policy_id,
    temporalScheduleID: schedule.temporal_schedule_id,
    intervalSeconds: schedule.interval_seconds,
    offsetSeconds: schedule.offset_seconds,
    replayWindowSeconds: schedule.replay_window_seconds,
    replayDelaySeconds: schedule.replay_delay_seconds,
    replayLimit: schedule.replay_limit,
    catchupWindowSeconds: schedule.catchup_window_seconds
  };
}

export function reportWorkflowScheduleEnablementReadiness({
  policyEnabledIDs,
  schedule
}: {
  policyEnabledIDs: ReadonlySet<number>;
  schedule: ReportWorkflowSchedule;
}): ReportWorkflowScheduleEnablementReadiness {
  if (!policyEnabledIDs.has(schedule.report_workflow_policy_id)) {
    const blocker = "Bound report workflow policy must be enabled before schedule enablement.";
    return {
      blockers: [blocker],
      detail: blocker,
      label: "Policy not ready.",
      status: "blocked"
    };
  }

  const replayWindowBlocker = reportWorkflowScheduleReplayWindowBlocker({
    intervalSeconds: schedule.interval_seconds,
    replayWindowSeconds: schedule.replay_window_seconds
  });
  if (replayWindowBlocker !== null) {
    return {
      blockers: [replayWindowBlocker],
      detail: replayWindowBlocker,
      label: "Replay window overlaps interval.",
      status: "blocked"
    };
  }

  return {
    blockers: [],
    detail: "Bound report workflow policy is enabled.",
    label: "Ready to enable.",
    status: "ready"
  };
}

export function reportWorkflowScheduleDraftReadiness({
  form,
  policyEnabledIDs
}: {
  form: ReportWorkflowScheduleFormState;
  policyEnabledIDs: ReadonlySet<number>;
}): ReportWorkflowScheduleDraftReadiness {
  if (!positiveInteger(form.reportWorkflowPolicyID)) {
    return {
      detail: "Select the workflow policy that this schedule should replay.",
      label: "Select a report workflow policy.",
      status: "pending"
    };
  }
  if (!policyEnabledIDs.has(form.reportWorkflowPolicyID)) {
    return {
      detail: "Enable the selected report workflow policy before this schedule can be enabled.",
      label: "Selected policy is not enabled.",
      status: "blocked"
    };
  }

  const replayWindowBlocker = reportWorkflowScheduleReplayWindowBlocker({
    intervalSeconds: form.intervalSeconds,
    replayWindowSeconds: form.replayWindowSeconds
  });
  if (replayWindowBlocker !== null) {
    return {
      detail: replayWindowBlocker,
      label: "Replay window overlaps interval.",
      status: "blocked"
    };
  }

  const parsed = formStateToWriteRequest(form);
  if (!parsed.ok) {
    return {
      detail: parsed.message,
      label: "Complete schedule fields.",
      status: "pending"
    };
  }

  return {
    detail: "Schedule fields and bound workflow policy are ready.",
    label: "Ready to save.",
    status: "ready"
  };
}

export function reportWorkflowScheduleProofOutcome({
  form,
  policyEnabledIDs,
  policyLabels
}: {
  form: ReportWorkflowScheduleFormState;
  policyEnabledIDs: ReadonlySet<number>;
  policyLabels: Readonly<Record<number, string>>;
}): ReportWorkflowScheduleProofOutcome {
  const readiness = reportWorkflowScheduleDraftReadiness({ form, policyEnabledIDs });
  const policyStatus = schedulePolicyStatus(form.reportWorkflowPolicyID, policyEnabledIDs);
  const timingStatus = scheduleTimingStatus(form);
  const replayStatus = scheduleReplayStatus(form);
  const catchupStatus = positiveInteger(form.catchupWindowSeconds) ? "ready" : "pending";
  const temporalScheduleID = form.temporalScheduleID.trim();
  const temporalIDStatus: ScheduleDraftReadinessStatus = temporalScheduleID === ""
    ? "pending"
    : /\s/.test(temporalScheduleID) || temporalScheduleID.length > 200
      ? "blocked"
      : "ready";
  const items: ReportWorkflowScheduleProofOutcomeItem[] = [
    {
      detail:
        policyStatus === "ready"
          ? "The bound policy is enabled and can be scheduled."
          : policyStatus === "blocked"
            ? "Enable the bound report workflow policy before this schedule can run."
            : "Select an enabled report workflow policy before scheduling proof.",
      status: policyStatus,
      title: "Policy",
      value: schedulePolicyValue(form.reportWorkflowPolicyID, policyLabels)
    },
    {
      detail:
        timingStatus === "ready"
          ? `Starts every ${draftDurationLabel(form.intervalSeconds)} after offset ${draftDurationLabel(form.offsetSeconds)}.`
          : "Set a positive interval and an offset that is lower than the interval.",
      status: timingStatus,
      title: "Cadence",
      value: `Every ${draftDurationLabel(form.intervalSeconds)}`
    },
    {
      detail:
        replayStatus === "ready"
          ? `Each run samples a ${draftDurationLabel(form.replayWindowSeconds)} alert window after a ${draftDurationLabel(
              form.replayDelaySeconds
            )} delay, capped at ${form.replayLimit} events.`
          : "Replay window, delay, and limit must be valid, and the window cannot exceed the interval.",
      status: replayStatus,
      title: "Replay sample",
      value: `${draftDurationLabel(form.replayWindowSeconds)} window`
    },
    {
      detail:
        catchupStatus === "ready"
          ? `Temporal can catch up missed schedule starts within ${draftDurationLabel(form.catchupWindowSeconds)}.`
          : "Set a positive catch-up window for missed scheduled starts.",
      status: catchupStatus,
      title: "Catch-up",
      value: draftDurationLabel(form.catchupWindowSeconds)
    },
    {
      detail:
        temporalIDStatus === "ready"
          ? `Temporal Schedule ID ${temporalScheduleID} is ready for retained scheduled-trigger proof.`
          : "Provide a non-empty Temporal Schedule ID without whitespace.",
      status: temporalIDStatus,
      title: "Proof target",
      value: temporalScheduleID === "" ? "Not named" : temporalScheduleID
    }
  ];
  const status = aggregateScheduleReadiness(items.map((item) => item.status));

  return {
    detail: scheduleProofOutcomeDetail(readiness.status === "blocked" ? "blocked" : status),
    items,
    status: readiness.status === "blocked" ? "blocked" : status
  };
}

export function formStateToWriteRequest(
  form: ReportWorkflowScheduleFormState
): ParseResult<ReportWorkflowScheduleWriteRequest> {
  const name = form.name.trim();
  if (name === "") {
    return { ok: false, message: "Schedule name is required." };
  }
  if (name.length > 120) {
    return { ok: false, message: "Schedule name must be 120 characters or fewer." };
  }

  const temporalScheduleID = form.temporalScheduleID.trim();
  if (temporalScheduleID === "") {
    return { ok: false, message: "Temporal Schedule ID is required." };
  }
  if (/\s/.test(temporalScheduleID)) {
    return { ok: false, message: "Temporal Schedule ID must not contain whitespace." };
  }
  if (temporalScheduleID.length > 200) {
    return { ok: false, message: "Temporal Schedule ID must be 200 characters or fewer." };
  }

  if (!positiveInteger(form.reportWorkflowPolicyID)) {
    return { ok: false, message: "Select a report workflow policy." };
  }
  if (!positiveInteger(form.intervalSeconds) || form.intervalSeconds > maxDurationSeconds) {
    return { ok: false, message: "Interval must be between 1 and 31536000 seconds." };
  }
  if (!nonNegativeInteger(form.offsetSeconds) || form.offsetSeconds > maxDurationSeconds) {
    return { ok: false, message: "Offset must be between 0 and 31536000 seconds." };
  }
  if (form.offsetSeconds >= form.intervalSeconds) {
    return { ok: false, message: "Offset must be less than interval." };
  }
  if (!positiveInteger(form.replayWindowSeconds) || form.replayWindowSeconds > maxDurationSeconds) {
    return { ok: false, message: "Replay window must be between 1 and 31536000 seconds." };
  }
  const replayWindowBlocker = reportWorkflowScheduleReplayWindowBlocker({
    intervalSeconds: form.intervalSeconds,
    replayWindowSeconds: form.replayWindowSeconds
  });
  if (replayWindowBlocker !== null) {
    return { ok: false, message: replayWindowBlocker };
  }
  if (!nonNegativeInteger(form.replayDelaySeconds) || form.replayDelaySeconds > maxDurationSeconds) {
    return { ok: false, message: "Replay delay must be between 0 and 31536000 seconds." };
  }
  if (!positiveInteger(form.replayLimit) || form.replayLimit > 100000) {
    return { ok: false, message: "Replay limit must be between 1 and 100000." };
  }
  if (!positiveInteger(form.catchupWindowSeconds) || form.catchupWindowSeconds > maxDurationSeconds) {
    return { ok: false, message: "Catch-up window must be between 1 and 31536000 seconds." };
  }

  return {
    ok: true,
    value: {
      name,
      report_workflow_policy_id: form.reportWorkflowPolicyID,
      temporal_schedule_id: temporalScheduleID,
      interval_seconds: form.intervalSeconds,
      offset_seconds: form.offsetSeconds,
      replay_window_seconds: form.replayWindowSeconds,
      replay_delay_seconds: form.replayDelaySeconds,
      replay_limit: form.replayLimit,
      catchup_window_seconds: form.catchupWindowSeconds
    }
  };
}

export function reportWorkflowScheduleReplayWindowBlocker({
  intervalSeconds,
  replayWindowSeconds
}: {
  intervalSeconds: number | null;
  replayWindowSeconds: number | null;
}): string | null {
  if (!positiveInteger(intervalSeconds) || !positiveInteger(replayWindowSeconds)) {
    return null;
  }
  if (replayWindowSeconds > intervalSeconds) {
    return "Replay window must be less than or equal to interval to avoid overlapping scheduled replay windows.";
  }
  return null;
}

const maxDurationSeconds = 31536000;

function positiveInteger(value: number | null): value is number {
  return Number.isSafeInteger(value) && value !== null && value > 0;
}

function nonNegativeInteger(value: number | null): value is number {
  return Number.isSafeInteger(value) && value !== null && value >= 0;
}

function schedulePolicyStatus(
  policyID: number | null,
  policyEnabledIDs: ReadonlySet<number>
): ScheduleDraftReadinessStatus {
  if (!positiveInteger(policyID)) {
    return "pending";
  }
  return policyEnabledIDs.has(policyID) ? "ready" : "blocked";
}

function scheduleTimingStatus(form: ReportWorkflowScheduleFormState): ScheduleDraftReadinessStatus {
  if (!positiveInteger(form.intervalSeconds) || !nonNegativeInteger(form.offsetSeconds)) {
    return "pending";
  }
  return form.offsetSeconds < form.intervalSeconds ? "ready" : "blocked";
}

function scheduleReplayStatus(form: ReportWorkflowScheduleFormState): ScheduleDraftReadinessStatus {
  if (
    !positiveInteger(form.replayWindowSeconds) ||
    !nonNegativeInteger(form.replayDelaySeconds) ||
    !positiveInteger(form.replayLimit)
  ) {
    return "pending";
  }
  return reportWorkflowScheduleReplayWindowBlocker({
    intervalSeconds: form.intervalSeconds,
    replayWindowSeconds: form.replayWindowSeconds
  }) === null
    ? "ready"
    : "blocked";
}

function schedulePolicyValue(
  policyID: number | null,
  policyLabels: Readonly<Record<number, string>>
): string {
  if (!positiveInteger(policyID)) {
    return "Not selected";
  }
  return policyLabels[policyID] ?? `Policy #${policyID}`;
}

function draftDurationLabel(value: number | null): string {
  if (value === null || !Number.isSafeInteger(value) || value < 0) {
    return "Not set";
  }
  return formatDurationSeconds(value);
}

function aggregateScheduleReadiness(statuses: ScheduleDraftReadinessStatus[]): ScheduleDraftReadinessStatus {
  if (statuses.includes("blocked")) {
    return "blocked";
  }
  if (statuses.includes("pending")) {
    return "pending";
  }
  return "ready";
}

function scheduleProofOutcomeDetail(status: ScheduleDraftReadinessStatus): string {
  switch (status) {
    case "ready":
      return "This schedule can retain recurring proof by replaying bounded alert windows through the selected workflow policy.";
    case "pending":
      return "Complete the schedule fields before relying on recurring replay proof.";
    case "blocked":
      return "Resolve blocked policy or timing settings before scheduled-trigger proof can run.";
  }
}

function firstSearchParamValue(value: SearchParamValue): string | null {
  if (Array.isArray(value)) {
    return value[0]?.trim() || null;
  }
  return value?.trim() || null;
}

function positiveSearchParamInteger(value: string | null): number | null {
  if (value === null || !/^[1-9][0-9]*$/.test(value)) {
    return null;
  }
  const parsed = Number(value);
  return positiveInteger(parsed) ? parsed : null;
}
