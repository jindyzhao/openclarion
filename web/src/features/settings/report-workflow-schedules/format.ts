import type {
  ReportWorkflowSchedule,
  ReportWorkflowScheduleFormState,
  ReportWorkflowScheduleWriteRequest
} from "./types";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

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
    return { ok: false, message: "Report workflow policy ID must be a positive integer." };
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

export function formatDurationSeconds(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return "0s";
  }
  const units = [
    { label: "d", value: 86400 },
    { label: "h", value: 3600 },
    { label: "m", value: 60 },
    { label: "s", value: 1 }
  ];
  let remaining = Math.floor(seconds);
  const parts: string[] = [];
  for (const unit of units) {
    const count = Math.floor(remaining / unit.value);
    if (count === 0) {
      continue;
    }
    parts.push(`${count}${unit.label}`);
    remaining -= count * unit.value;
  }
  return parts.length === 0 ? "0s" : parts.join(" ");
}

const maxDurationSeconds = 31536000;

function positiveInteger(value: number | null): value is number {
  return Number.isSafeInteger(value) && value !== null && value > 0;
}

function nonNegativeInteger(value: number | null): value is number {
  return Number.isSafeInteger(value) && value !== null && value >= 0;
}
