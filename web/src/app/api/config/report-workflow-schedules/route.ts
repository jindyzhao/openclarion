import {
  createReportWorkflowSchedule,
  fetchReportWorkflowSchedules
} from "@/features/settings/report-workflow-schedules/api";
import type { ReportWorkflowScheduleWriteRequest } from "@/features/settings/report-workflow-schedules/types";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET() {
  return apiResultResponse(await fetchReportWorkflowSchedules());
}

export async function POST(request: Request) {
  const body = await readRequestJSON<ReportWorkflowScheduleWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await createReportWorkflowSchedule(body.data), 201);
}
