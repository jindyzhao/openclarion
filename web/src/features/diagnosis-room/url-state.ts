import { diagnosisEvidencePlanSearchParamNames } from "./evidence-plan-url";
import { diagnosisEvidencePlanURLQuery } from "./evidence-plan-url";
import type {
  DiagnosisConsultationEvidenceRequest,
  DiagnosisEvidenceRequest,
} from "./types";

const diagnosisSupplementalFollowUpSearchParamNames = [
  "follow_up_label",
  "follow_up_detail",
  "follow_up_priority",
] as const;

type DiagnosisRoomURLInput = {
  pathname: string;
  search: string;
};

export type DiagnosisRoomWeComAuthError =
  | "wecom_auth_failed"
  | "wecom_callback_failed"
  | "wecom_callback_missing"
  | "wecom_entry_unavailable"
  | "wecom_login_failed"
  | "wecom_role_unauthorized";

export type DiagnosisRoomWeComLaunchContext =
  | "app_conversation"
  | "wecom"
  | "workbench";

export type DiagnosisRoomIntent =
  | "alert_review"
  | "confidence_review"
  | "review_conclusion";

export type DiagnosisRoomLinkInput = {
  evidencePlan?: DiagnosisEvidenceRequest;
  evidenceSnapshotID?: number;
  intent?: DiagnosisRoomIntent;
  sessionID?: string;
  supplementalFollowUp?: DiagnosisConsultationEvidenceRequest;
};

export type DiagnosisRoomAnchorLinkInput = DiagnosisRoomLinkInput & {
  anchorID: string;
};

export type DiagnosisRoomSelectedRoomURLInput = DiagnosisRoomURLInput & {
  evidenceSnapshotID: number;
  keepReportContext: boolean;
  sessionID: string;
};

export type DiagnosisRoomOneShotURLInput = DiagnosisRoomURLInput & {
  authError?: boolean;
  evidencePlan?: boolean;
  supplementalFollowUp?: boolean;
  weComAutoLogin?: boolean;
};

export function diagnosisRoomWeComReturnTo({
  pathname,
  search,
}: DiagnosisRoomURLInput): string {
  const params = new URLSearchParams(search);
  const launchContext = normalizedDiagnosisRoomWeComLaunchContext(
    params.get("wecom_launch_context"),
  );
  params.set("auth_mode", "session");
  params.delete("wecom_auth_error");
  params.delete("wecom_auto_login");
  params.delete("wecom_launch_context");
  if (launchContext !== undefined) {
    params.set("wecom_launch_context", launchContext);
  }
  return diagnosisRoomURL(pathname, params);
}

export function diagnosisRoomWeComAuthErrorSearchParam(searchParams: {
  get(name: string): string | null;
}): DiagnosisRoomWeComAuthError | undefined {
  switch (searchParams.get("wecom_auth_error")) {
    case "wecom_auth_failed":
      return "wecom_auth_failed";
    case "wecom_callback_failed":
      return "wecom_callback_failed";
    case "wecom_callback_missing":
      return "wecom_callback_missing";
    case "wecom_entry_unavailable":
      return "wecom_entry_unavailable";
    case "wecom_login_failed":
      return "wecom_login_failed";
    case "wecom_role_unauthorized":
      return "wecom_role_unauthorized";
    default:
      return undefined;
  }
}

export function diagnosisRoomWeComAutoLoginSearchParam(searchParams: {
  get(name: string): string | null;
}): boolean {
  return searchParams.get("wecom_auto_login") === "1";
}

export function diagnosisRoomWeComLaunchContextSearchParam(searchParams: {
  get(name: string): string | null;
}): DiagnosisRoomWeComLaunchContext | undefined {
  return normalizedDiagnosisRoomWeComLaunchContext(
    searchParams.get("wecom_launch_context"),
  );
}

export function diagnosisRoomLinkHref({
  evidencePlan,
  evidenceSnapshotID,
  intent,
  sessionID,
  supplementalFollowUp,
}: DiagnosisRoomLinkInput): string {
  const params = new URLSearchParams();
  if (evidenceSnapshotID !== undefined) {
    params.set("evidence_snapshot_id", String(evidenceSnapshotID));
  }
  if (intent !== undefined) {
    params.set("intent", intent);
  }
  if (sessionID !== undefined && sessionID.trim() !== "") {
    params.set("session_id", sessionID);
  }
  if (evidencePlan !== undefined) {
    setQueryParams(params, diagnosisEvidencePlanURLQuery(evidencePlan));
  }
  if (supplementalFollowUp !== undefined) {
    setQueryParams(
      params,
      diagnosisSupplementalFollowUpURLQuery(supplementalFollowUp),
    );
  }
  return diagnosisRoomURL("/diagnosis-room", params);
}

function diagnosisSupplementalFollowUpURLQuery(
  request: DiagnosisConsultationEvidenceRequest,
): Record<string, string> {
  return {
    follow_up_detail: request.detail,
    follow_up_label: request.label,
    follow_up_priority: request.priority,
  };
}

export function diagnosisRoomAnchorHref({
  anchorID,
  ...input
}: DiagnosisRoomAnchorLinkInput): string {
  const href = diagnosisRoomLinkHref(input);
  const normalizedAnchorID = anchorID.trim().replace(/^#/, "");
  return normalizedAnchorID === ""
    ? href
    : `${href}#${encodeURIComponent(normalizedAnchorID)}`;
}

export function diagnosisRoomURLWithSelectedRoom({
  evidenceSnapshotID,
  keepReportContext,
  pathname,
  search,
  sessionID,
}: DiagnosisRoomSelectedRoomURLInput): string {
  const params = new URLSearchParams(search);
  params.set("session_id", sessionID);
  params.set("evidence_snapshot_id", String(evidenceSnapshotID));
  if (!keepReportContext) {
    clearReportContextParams(params);
    clearSupplementalFollowUpParams(params);
    clearEvidencePlanParams(params);
  }
  return diagnosisRoomURL(pathname, params);
}

export function diagnosisRoomURLWithoutOneShotParams({
  authError = false,
  evidencePlan = false,
  pathname,
  search,
  supplementalFollowUp = false,
  weComAutoLogin = false,
}: DiagnosisRoomOneShotURLInput): string {
  const params = new URLSearchParams(search);
  if (authError) {
    params.delete("wecom_auth_error");
    params.delete("oidc_auth_error");
  }
  if (weComAutoLogin) {
    params.delete("wecom_auto_login");
    if (params.get("auth_mode") === "wecom") {
      params.set("auth_mode", "session");
    }
    if (
      normalizedDiagnosisRoomWeComLaunchContext(
        params.get("wecom_launch_context"),
      ) === undefined
    ) {
      params.delete("wecom_launch_context");
    }
  }
  if (supplementalFollowUp) {
    clearSupplementalFollowUpParams(params);
  }
  if (evidencePlan) {
    clearEvidencePlanParams(params);
  }
  return diagnosisRoomURL(pathname, params);
}

function clearReportContextParams(params: URLSearchParams) {
  params.delete("report_id");
  params.delete("sub_report_id");
  params.delete("intent");
}

function clearSupplementalFollowUpParams(params: URLSearchParams) {
  diagnosisSupplementalFollowUpSearchParamNames.forEach((name) => {
    params.delete(name);
  });
}

function clearEvidencePlanParams(params: URLSearchParams) {
  diagnosisEvidencePlanSearchParamNames.forEach((name) => {
    params.delete(name);
  });
}

function diagnosisRoomURL(pathname: string, params: URLSearchParams): string {
  const query = params.toString();
  return query === "" ? pathname : `${pathname}?${query}`;
}

function normalizedDiagnosisRoomWeComLaunchContext(
  raw: string | null,
): DiagnosisRoomWeComLaunchContext | undefined {
  switch (raw) {
    case "app_conversation":
    case "wecom":
    case "workbench":
      return raw;
    default:
      return undefined;
  }
}

function setQueryParams(params: URLSearchParams, query: Record<string, string>) {
  Object.entries(query).forEach(([name, value]) => {
    params.set(name, value);
  });
}
