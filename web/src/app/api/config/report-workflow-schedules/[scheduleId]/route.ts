import { replaceReportWorkflowSchedule } from "@/features/settings/report-workflow-schedules/api";
import type { ReportWorkflowScheduleWriteRequest } from "@/features/settings/report-workflow-schedules/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ scheduleId: string }>;
};

export const dynamic = "force-dynamic";

export async function PUT(request: Request, context: RouteContext) {
  const { scheduleId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(scheduleId, "Report workflow schedule ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  const body = await readRequestJSON<ReportWorkflowScheduleWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(request, (headers) =>
    replaceReportWorkflowSchedule(parsedID.data, body.data, { headers }),
  );
}
