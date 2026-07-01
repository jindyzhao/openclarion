import type { components } from "@/lib/api/openapi";

export type ReportWorkflowPolicy = components["schemas"]["ReportWorkflowPolicy"];
export type ReportWorkflowPolicyImpactPreviewResult =
  components["schemas"]["ReportWorkflowPolicyImpactPreviewResult"];
export type ReportWorkflowPolicyListResponse = components["schemas"]["ReportWorkflowPolicyListResponse"];
export type ReportReplayTriggerResponse = components["schemas"]["ReportReplayTriggerResponse"];
export type ReportWorkflowPolicyReplayRequest = components["schemas"]["ReportWorkflowPolicyReplayRequest"];
export type ReportWorkflowPolicyWriteRequest = components["schemas"]["ReportWorkflowPolicyWriteRequest"];

export type ReportWorkflowPolicyFormState = {
  name: string;
  alertSourceProfileID: number | null;
  groupingPolicyID: number | null;
  reportNotificationChannelProfileID: number | undefined;
  triggerMode: "manual_replay";
  reportScenario: "single_alert" | "cascade" | "alert_storm";
  diagnosisFollowUp: "disabled" | "suggest_room" | "auto_room";
};

export type ReportWorkflowPolicyReplayFormState = {
  windowStart: string;
  windowEnd: string;
  limit: number | null;
  correlationKey: string;
  workflowID: string;
};
