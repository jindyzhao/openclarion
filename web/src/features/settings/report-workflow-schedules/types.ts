import type { components } from "@/lib/api/openapi";

export type ReportWorkflowSchedule = components["schemas"]["ReportWorkflowSchedule"];
export type ReportWorkflowScheduleListResponse = components["schemas"]["ReportWorkflowScheduleListResponse"];
export type ReportWorkflowScheduleWriteRequest = components["schemas"]["ReportWorkflowScheduleWriteRequest"];

export type ReportWorkflowScheduleFormState = {
  name: string;
  reportWorkflowPolicyID: number | null;
  temporalScheduleID: string;
  intervalSeconds: number | null;
  offsetSeconds: number | null;
  replayWindowSeconds: number | null;
  replayDelaySeconds: number | null;
  replayLimit: number | null;
  catchupWindowSeconds: number | null;
};
