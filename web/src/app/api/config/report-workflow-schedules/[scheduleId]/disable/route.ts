import { disableReportWorkflowSchedule } from "@/features/settings/report-workflow-schedules/api";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ scheduleId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(request: Request, context: RouteContext) {
  const { scheduleId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(scheduleId, "Report workflow schedule ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  return authorizedBackendResultResponse(request, (headers) =>
    disableReportWorkflowSchedule(parsedID.data, { headers }),
  );
}
