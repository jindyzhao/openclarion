import type { components } from "@/lib/api/openapi";

export type DirectorySyncRequest =
  components["schemas"]["DirectorySyncRequest"];
export type DirectorySyncResponse =
  components["schemas"]["DirectorySyncResponse"];
export type DirectorySyncRun = components["schemas"]["DirectorySyncRun"];
export type DirectorySyncRunListResponse =
  components["schemas"]["DirectorySyncRunListResponse"];
export type DirectoryDepartment =
  components["schemas"]["DirectoryDepartment"];
export type DirectoryDepartmentListResponse =
  components["schemas"]["DirectoryDepartmentListResponse"];
export type DirectoryUser = components["schemas"]["DirectoryUser"];
export type DirectoryUserListResponse =
  components["schemas"]["DirectoryUserListResponse"];
export type RBACAssignment = components["schemas"]["RBACAssignment"];
export type RBACAssignmentListResponse =
  components["schemas"]["RBACAssignmentListResponse"];
export type RBACAssignmentWriteRequest =
  components["schemas"]["RBACAssignmentWriteRequest"];
export type RBACAuthorizeRequest =
  components["schemas"]["RBACAuthorizeRequest"];
export type RBACAuthorizeResponse =
  components["schemas"]["RBACAuthorizeResponse"];
export type RBACCurrentAuthorizationRequest =
  components["schemas"]["RBACCurrentAuthorizationRequest"];
export type RBACCurrentAuthorizationResponse =
  components["schemas"]["RBACCurrentAuthorizationResponse"];
export type RBACPermission = components["schemas"]["RBACPermission"];
export type RBACRole = components["schemas"]["RBACRole"];
export type RBACScopeKind = components["schemas"]["RBACScopeKind"];
export type RBACSubjectKind = components["schemas"]["RBACSubjectKind"];

export type DirectorySyncFormState = {
  pageSize: number;
  updatedAfter: string;
};

export type RBACAssignmentFormState = {
  enabled: boolean;
  role: RBACRole;
  scopeKey: string;
  scopeKind: RBACScopeKind;
  subjectKey: string;
  subjectKind: RBACSubjectKind;
};

export type RBACAuthorizeFormState = {
  departmentKeysText: string;
  permission: RBACPermission;
  scopeKey: string;
  scopeKind: RBACScopeKind;
  subject: string;
};
