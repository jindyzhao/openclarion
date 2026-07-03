import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";

import type {
  DirectoryDepartmentListResponse,
  DirectorySyncRequestBody,
  DirectorySyncResponse,
  DirectorySyncRunListResponse,
  DirectoryUserListResponse,
  RBACAssignment,
  RBACAssignmentListResponse,
  RBACAssignmentWriteRequest,
  RBACAuthorizeRequest,
  RBACAuthorizeResponse,
  RBACCurrentAuthorizationRequest,
  RBACCurrentAuthorizationResponse,
} from "./types";

export type DirectoryListOptions = {
  limit?: number;
  provider?: string;
};

export async function syncDirectoryProjection(
  body: DirectorySyncRequestBody,
  options: Pick<RequestJSONOptions, "headers"> = {},
): Promise<ApiResult<DirectorySyncResponse>> {
  return requestJSON<DirectorySyncResponse>("/api/v1/config/directory/sync", {
    method: "POST",
    body,
    headers: options.headers,
  });
}

export async function fetchDirectorySyncRuns(
  options: DirectoryListOptions = {},
  requestOptions: Pick<RequestJSONOptions, "headers"> = {},
): Promise<ApiResult<DirectorySyncRunListResponse>> {
  return requestJSON<DirectorySyncRunListResponse>(
    `/api/v1/config/directory/sync-runs${directoryListQuery(options)}`,
    { headers: requestOptions.headers },
  );
}

export async function fetchDirectoryUsers(
  options: DirectoryListOptions = {},
  requestOptions: Pick<RequestJSONOptions, "headers"> = {},
): Promise<ApiResult<DirectoryUserListResponse>> {
  return requestJSON<DirectoryUserListResponse>(
    `/api/v1/config/directory/users${directoryListQuery(options)}`,
    { headers: requestOptions.headers },
  );
}

export async function fetchDirectoryDepartments(
  options: DirectoryListOptions = {},
  requestOptions: Pick<RequestJSONOptions, "headers"> = {},
): Promise<ApiResult<DirectoryDepartmentListResponse>> {
  return requestJSON<DirectoryDepartmentListResponse>(
    `/api/v1/config/directory/departments${directoryListQuery(options)}`,
    { headers: requestOptions.headers },
  );
}

export async function fetchRBACAssignments(
  limit = 100,
  options: Pick<RequestJSONOptions, "headers"> = {},
): Promise<ApiResult<RBACAssignmentListResponse>> {
  return requestJSON<RBACAssignmentListResponse>(
    `/api/v1/config/rbac/assignments?limit=${encodeURIComponent(limit)}`,
    { headers: options.headers },
  );
}

export async function upsertRBACAssignment(
  body: RBACAssignmentWriteRequest,
  options: Pick<RequestJSONOptions, "headers"> = {},
): Promise<ApiResult<RBACAssignment>> {
  return requestJSON<RBACAssignment>("/api/v1/config/rbac/assignments", {
    method: "POST",
    body,
    headers: options.headers,
  });
}

export async function authorizeRBAC(
  body: RBACAuthorizeRequest,
  options: Pick<RequestJSONOptions, "headers"> = {},
): Promise<ApiResult<RBACAuthorizeResponse>> {
  return requestJSON<RBACAuthorizeResponse>("/api/v1/config/rbac/authorize", {
    method: "POST",
    body,
    headers: options.headers,
  });
}

export async function authorizeCurrentRBAC(
  body: RBACCurrentAuthorizationRequest,
  options: Pick<RequestJSONOptions, "headers"> = {},
): Promise<ApiResult<RBACCurrentAuthorizationResponse>> {
  return requestJSON<RBACCurrentAuthorizationResponse>(
    "/api/v1/config/rbac/current-authorizations",
    {
      method: "POST",
      body,
      headers: options.headers,
    },
  );
}

function directoryListQuery(options: DirectoryListOptions): string {
  const query = new URLSearchParams();
  if (options.limit !== undefined) {
    query.set("limit", String(options.limit));
  }
  const provider = options.provider?.trim();
  if (provider !== undefined && provider !== "") {
    query.set("provider", provider);
  }
  const encoded = query.toString();
  return encoded === "" ? "" : `?${encoded}`;
}
