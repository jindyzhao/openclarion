import {
  createReportWorkflowPolicy,
  fetchReportWorkflowPolicies
} from "@/features/settings/report-workflow-policies/api";
import type { ReportWorkflowPolicyWriteRequest } from "@/features/settings/report-workflow-policies/types";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET() {
  return apiResultResponse(await fetchReportWorkflowPolicies());
}

export async function POST(request: Request) {
  const body = await readRequestJSON<ReportWorkflowPolicyWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await createReportWorkflowPolicy(body.data), 201);
}
