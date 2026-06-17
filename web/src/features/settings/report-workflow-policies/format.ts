import type {
  ReportWorkflowPolicy,
  ReportWorkflowPolicyFormState,
  ReportWorkflowPolicyReplayFormState,
  ReportWorkflowPolicyReplayRequest,
  ReportWorkflowPolicyWriteRequest
} from "./types";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

export function emptyReportWorkflowPolicyForm(): ReportWorkflowPolicyFormState {
  return {
    name: "",
    alertSourceProfileID: null,
    groupingPolicyID: null,
    reportNotificationChannelProfileID: undefined,
    triggerMode: "manual_replay",
    reportScenario: "single_alert",
    diagnosisFollowUp: "disabled"
  };
}

export function defaultReportWorkflowPolicyReplayForm(now = new Date()): ReportWorkflowPolicyReplayFormState {
  const end = new Date(now);
  const start = new Date(end.getTime() - 60 * 60 * 1000);
  return {
    windowStart: isoSeconds(start),
    windowEnd: isoSeconds(end),
    limit: 10000,
    correlationKey: "",
    workflowID: ""
  };
}

export function policyToFormState(policy: ReportWorkflowPolicy): ReportWorkflowPolicyFormState {
  return {
    name: policy.name,
    alertSourceProfileID: policy.alert_source_profile_id,
    groupingPolicyID: policy.grouping_policy_id,
    reportNotificationChannelProfileID: policy.report_notification_channel_profile_id ?? undefined,
    triggerMode: policy.trigger_mode,
    reportScenario: policy.report_scenario,
    diagnosisFollowUp: policy.diagnosis_follow_up
  };
}

export function formStateToReplayRequest(
  form: ReportWorkflowPolicyReplayFormState
): ParseResult<ReportWorkflowPolicyReplayRequest> {
  const windowStart = form.windowStart.trim();
  const windowEnd = form.windowEnd.trim();
  if (windowStart === "") {
    return { ok: false, message: "Window start is required." };
  }
  if (windowEnd === "") {
    return { ok: false, message: "Window end is required." };
  }
  const start = Date.parse(windowStart);
  if (!Number.isFinite(start)) {
    return { ok: false, message: "Window start must be a valid date-time." };
  }
  const end = Date.parse(windowEnd);
  if (!Number.isFinite(end)) {
    return { ok: false, message: "Window end must be a valid date-time." };
  }
  if (end <= start) {
    return { ok: false, message: "Window end must be after window start." };
  }
  if (!positiveInteger(form.limit) || form.limit > 100000) {
    return { ok: false, message: "Limit must be between 1 and 100000." };
  }

  const correlationKey = form.correlationKey.trim();
  const workflowID = form.workflowID.trim();
  return {
    ok: true,
    value: {
      window_start: isoSeconds(new Date(start)),
      window_end: isoSeconds(new Date(end)),
      limit: form.limit,
      ...(correlationKey === "" ? {} : { correlation_key: correlationKey }),
      ...(workflowID === "" ? {} : { workflow_id: workflowID })
    }
  };
}

export function formStateToWriteRequest(
  form: ReportWorkflowPolicyFormState
): ParseResult<ReportWorkflowPolicyWriteRequest> {
  const name = form.name.trim();
  if (name === "") {
    return { ok: false, message: "Policy name is required." };
  }
  if (name.length > 120) {
    return { ok: false, message: "Policy name must be 120 characters or fewer." };
  }
  if (!positiveInteger(form.alertSourceProfileID)) {
    return { ok: false, message: "Select an alert source." };
  }
  if (!positiveInteger(form.groupingPolicyID)) {
    return { ok: false, message: "Select a grouping policy." };
  }
  if (form.reportNotificationChannelProfileID !== undefined && !positiveInteger(form.reportNotificationChannelProfileID)) {
    return { ok: false, message: "Select a valid report notification channel." };
  }
  return {
    ok: true,
    value: {
      name,
      alert_source_profile_id: form.alertSourceProfileID,
      grouping_policy_id: form.groupingPolicyID,
      report_notification_channel_profile_id: form.reportNotificationChannelProfileID ?? null,
      trigger_mode: form.triggerMode,
      report_scenario: form.reportScenario,
      diagnosis_follow_up: form.diagnosisFollowUp
    }
  };
}

function positiveInteger(value: number | null): value is number {
  return Number.isSafeInteger(value) && value !== null && value > 0;
}

function isoSeconds(value: Date): string {
  return value.toISOString().replace(".000Z", "Z");
}
