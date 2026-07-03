import {
  createReportWorkflowSchedule,
  fetchReportWorkflowSchedules
} from "@/features/settings/report-workflow-schedules/api";
import type { ReportWorkflowScheduleWriteRequest } from "@/features/settings/report-workflow-schedules/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  return authorizedBackendResultResponse(request, (headers) =>
    fetchReportWorkflowSchedules({ headers }),
  );
}

export async function POST(request: Request) {
  const body = await readRequestJSON<ReportWorkflowScheduleWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(
    request,
    (headers) => createReportWorkflowSchedule(body.data, { headers }),
    201,
  );
}
