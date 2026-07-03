"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  DirectoryDepartmentListResponse,
  DirectorySyncRequest,
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

export async function refreshDirectoryUsers(): Promise<
  ApiResult<DirectoryUserListResponse>
> {
  return requestSameOriginJSON<DirectoryUserListResponse>(
    "/api/config/directory/users?limit=100",
  );
}

export async function refreshDirectoryDepartments(): Promise<
  ApiResult<DirectoryDepartmentListResponse>
> {
  return requestSameOriginJSON<DirectoryDepartmentListResponse>(
    "/api/config/directory/departments?limit=100",
  );
}

export async function refreshDirectorySyncRuns(): Promise<
  ApiResult<DirectorySyncRunListResponse>
> {
  return requestSameOriginJSON<DirectorySyncRunListResponse>(
    "/api/config/directory/sync-runs?limit=10",
  );
}

export async function refreshRBACAssignments(): Promise<
  ApiResult<RBACAssignmentListResponse>
> {
  return requestSameOriginJSON<RBACAssignmentListResponse>(
    "/api/config/rbac/assignments?limit=100",
  );
}

export async function runDirectorySync(
  body: DirectorySyncRequest,
): Promise<ApiResult<DirectorySyncResponse>> {
  return requestSameOriginJSON<DirectorySyncResponse>(
    "/api/config/directory/sync",
    {
      method: "POST",
      body,
    },
  );
}

export async function submitRBACAssignment(
  body: RBACAssignmentWriteRequest,
): Promise<ApiResult<RBACAssignment>> {
  return requestSameOriginJSON<RBACAssignment>("/api/config/rbac/assignments", {
    method: "POST",
    body,
  });
}

export async function previewRBACAuthorization(
  body: RBACAuthorizeRequest,
): Promise<ApiResult<RBACAuthorizeResponse>> {
  return requestSameOriginJSON<RBACAuthorizeResponse>(
    "/api/config/rbac/authorize",
    {
      method: "POST",
      body,
    },
  );
}

export async function checkCurrentRBACAuthorizations(
  body: RBACCurrentAuthorizationRequest,
): Promise<ApiResult<RBACCurrentAuthorizationResponse>> {
  return requestSameOriginJSON<RBACCurrentAuthorizationResponse>(
    "/api/config/rbac/current-authorizations",
    {
      method: "POST",
      body,
    },
  );
}
