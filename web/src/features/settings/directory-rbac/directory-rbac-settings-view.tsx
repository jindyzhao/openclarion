"use client";

import {
  AuditOutlined,
  EditOutlined,
  LoginOutlined,
  ReloadOutlined,
  SaveOutlined,
} from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import {
  Alert,
  AutoComplete,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  Row,
  Select,
  Space,
  Statistic,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
} from "antd";
import type { TableColumnsType, TabsProps } from "antd";
import { useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

import { refreshDiagnosisRooms } from "@/features/diagnosis-room/client-api";
import { diagnosisOIDCLoginHref } from "@/features/diagnosis-room/oidc-login";
import { refreshAlertSourceProfiles } from "../alert-sources/client-api";
import type { AlertSourceProfileListResponse } from "../alert-sources/types";
import { refreshDiagnosisToolTemplates } from "../diagnosis-tool-templates/client-api";
import type { DiagnosisToolTemplateListResponse } from "../diagnosis-tool-templates/types";
import { formatDateTime } from "../format";
import { refreshGroupingPolicies } from "../grouping-policies/client-api";
import type { GroupingPolicyListResponse } from "../grouping-policies/types";
import { refreshNotificationChannelProfiles } from "../notification-channels/client-api";
import type { NotificationChannelProfileListResponse } from "../notification-channels/types";
import {
  settingsErrorMessage,
  settingsManagePermissionNotice,
  settingsReadPermissionEmptyDescription,
  settingsReadPermissionNotice,
  type SettingsNotice,
  useClientReady,
  useSettingsList,
  useSettingsMutation,
} from "../query-state";
import { ReadOnlyModeAlert } from "../permission-notice";
import {
  currentRBACAuthorizationNeedsSignIn,
  useCurrentRBACAuthorizations,
  type CurrentRBACAuthorizationCheck,
} from "../rbac-capabilities";
import { refreshReportWorkflowPolicies } from "../report-workflow-policies/client-api";
import type { ReportWorkflowPolicyListResponse } from "../report-workflow-policies/types";
import { refreshReportWorkflowSchedules } from "../report-workflow-schedules/client-api";
import type { ReportWorkflowScheduleListResponse } from "../report-workflow-schedules/types";
import {
  previewRBACAuthorization,
  refreshDirectoryDepartments,
  refreshDirectorySyncRuns,
  refreshDirectoryUsers,
  refreshRBACAssignments,
  runDirectorySync,
  submitRBACAssignment,
} from "./client-api";
import {
  assignmentFormToWriteRequest,
  assignmentScopeLabel,
  bootstrapAdminAssignmentForm,
  authorizeFormToRequest,
  directoryRBACNextStepNotice,
  directoryDepartmentLabel,
  directoryDepartmentSubjectOptions,
  directorySyncProjectionSummary,
  directoryUserDepartmentKeysText,
  directoryUserSubjectOptions,
  directoryUserSubjectIndex,
  directorySyncFormToRequest,
  directoryUserLabel,
  emptyDirectorySyncForm,
  emptyRBACAssignmentForm,
  emptyRBACAuthorizeForm,
  permissionLabel,
  rbacResourceScopeOptions,
  rbacPermissionOptions,
  rbacRoleOptions,
  rbacScopeKindOptions,
  rbacSubjectKindOptions,
  roleLabel,
  scopeLabel,
  scopeRequiresKey,
  subjectKindLabel,
  uniqueNonEmpty,
} from "./format";
import { DirectorySubjectTags } from "./subject-tags";
import type {
  DirectoryRBACNextStepNotice,
  DirectorySelectOption,
  DirectorySyncProjectionSummary,
} from "./format";
import type {
  DirectoryDepartment,
  DirectoryDepartmentListResponse,
  DirectorySyncFormState,
  DirectorySyncRequest,
  DirectorySyncResponse,
  DirectorySyncRun,
  DirectorySyncRunListResponse,
  DirectoryUser,
  DirectoryUserListResponse,
  RBACAssignment,
  RBACAssignmentFormState,
  RBACAssignmentListResponse,
  RBACAssignmentWriteRequest,
  RBACAuthorizeFormState,
  RBACAuthorizeRequest,
  RBACAuthorizeResponse,
  RBACScopeKind,
} from "./types";

type DirectoryRBACSettingsManagerProps = {
  alertSourcesResult: ApiResult<AlertSourceProfileListResponse>;
  assignmentsResult: ApiResult<RBACAssignmentListResponse>;
  departmentsResult: ApiResult<DirectoryDepartmentListResponse>;
  diagnosisToolTemplatesResult: ApiResult<DiagnosisToolTemplateListResponse>;
  groupingPoliciesResult: ApiResult<GroupingPolicyListResponse>;
  notificationChannelsResult: ApiResult<NotificationChannelProfileListResponse>;
  syncRunsResult: ApiResult<DirectorySyncRunListResponse>;
  usersResult: ApiResult<DirectoryUserListResponse>;
  workflowPoliciesResult: ApiResult<ReportWorkflowPolicyListResponse>;
  workflowSchedulesResult: ApiResult<ReportWorkflowScheduleListResponse>;
};

const directoryUsersQueryKey = ["settings", "directory", "users"] as const;
const directoryDepartmentsQueryKey = [
  "settings",
  "directory",
  "departments",
] as const;
const directorySyncRunsQueryKey = ["settings", "directory", "sync-runs"] as const;
const rbacAssignmentsQueryKey = ["settings", "rbac", "assignments"] as const;
const alertSourcesQueryKey = ["settings", "rbac", "alert-sources"] as const;
const groupingPoliciesQueryKey = [
  "settings",
  "rbac",
  "grouping-policies",
] as const;
const reportWorkflowPoliciesQueryKey = [
  "settings",
  "rbac",
  "report-workflow-policies",
] as const;
const reportWorkflowSchedulesQueryKey = [
  "settings",
  "rbac",
  "report-workflow-schedules",
] as const;
const notificationChannelsQueryKey = [
  "settings",
  "rbac",
  "notification-channels",
] as const;
const diagnosisToolTemplatesQueryKey = [
  "settings",
  "rbac",
  "diagnosis-tool-templates",
] as const;
const diagnosisRoomsQueryKey = [
  "settings",
  "rbac",
  "diagnosis-room-scope-options",
] as const;

type SaveAssignmentVariables = {
  body: RBACAssignmentWriteRequest;
};

type SyncDirectoryVariables = {
  body: DirectorySyncRequest;
};

type PreviewState =
  | { kind: "empty" }
  | { kind: "ready"; request: RBACAuthorizeRequest; response: RBACAuthorizeResponse };

const directoryRBACAuthorizationChecks: CurrentRBACAuthorizationCheck[] = [
  { key: "directoryRead", permission: "directory.read" },
  { key: "directoryManage", permission: "directory.manage" },
  { key: "rbacManage", permission: "rbac.manage" },
];
const directoryRBACSettingsLoginHref = diagnosisOIDCLoginHref(
  "/settings/directory-rbac",
);

export function DirectoryRBACSettingsManager({
  alertSourcesResult,
  assignmentsResult,
  departmentsResult,
  diagnosisToolTemplatesResult,
  groupingPoliciesResult,
  notificationChannelsResult,
  syncRunsResult,
  usersResult,
  workflowPoliciesResult,
  workflowSchedulesResult,
}: DirectoryRBACSettingsManagerProps) {
  const [syncForm] = Form.useForm<DirectorySyncFormState>();
  const [assignmentForm] = Form.useForm<RBACAssignmentFormState>();
  const [authorizeForm] = Form.useForm<RBACAuthorizeFormState>();
  const clientReady = useClientReady();
  const [notice, setNotice] = useState<SettingsNotice | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [preview, setPreview] = useState<PreviewState>({ kind: "empty" });
  const currentAuthorization = useCurrentRBACAuthorizations(
    directoryRBACAuthorizationChecks,
    clientReady,
  );
  const {
    errorStatus: usersErrorStatus,
    items: users,
    notice: usersNotice,
    query: usersQuery,
  } = useSettingsList({
    initialResult: usersResult,
    queryFn: refreshDirectoryUsers,
    queryKey: directoryUsersQueryKey,
    refreshMessage: "Users refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    errorStatus: departmentsErrorStatus,
    items: departments,
    notice: departmentsNotice,
    query: departmentsQuery,
  } = useSettingsList({
    initialResult: departmentsResult,
    queryFn: refreshDirectoryDepartments,
    queryKey: directoryDepartmentsQueryKey,
    refreshMessage: "Departments refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    errorStatus: assignmentsErrorStatus,
    items: assignments,
    notice: assignmentsNotice,
    query: assignmentsQuery,
  } = useSettingsList({
    initialResult: assignmentsResult,
    queryFn: refreshRBACAssignments,
    queryKey: rbacAssignmentsQueryKey,
    refreshMessage: "RBAC assignments refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    errorStatus: syncRunsErrorStatus,
    items: syncRuns,
    notice: syncRunsNotice,
    query: syncRunsQuery,
  } = useSettingsList({
    initialResult: syncRunsResult,
    queryFn: refreshDirectorySyncRuns,
    queryKey: directorySyncRunsQueryKey,
    refreshMessage: "Directory sync runs refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    items: alertSources,
    notice: alertSourcesNotice,
    query: alertSourcesQuery,
  } = useSettingsList({
    initialResult: alertSourcesResult,
    queryFn: refreshAlertSourceProfiles,
    queryKey: alertSourcesQueryKey,
    refreshMessage: "Alert sources refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    items: groupingPolicies,
    notice: groupingPoliciesNotice,
    query: groupingPoliciesQuery,
  } = useSettingsList({
    initialResult: groupingPoliciesResult,
    queryFn: refreshGroupingPolicies,
    queryKey: groupingPoliciesQueryKey,
    refreshMessage: "Grouping policies refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    items: workflowPolicies,
    notice: workflowPoliciesNotice,
    query: workflowPoliciesQuery,
  } = useSettingsList({
    initialResult: workflowPoliciesResult,
    queryFn: refreshReportWorkflowPolicies,
    queryKey: reportWorkflowPoliciesQueryKey,
    refreshMessage: "Report workflows refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    items: workflowSchedules,
    notice: workflowSchedulesNotice,
    query: workflowSchedulesQuery,
  } = useSettingsList({
    initialResult: workflowSchedulesResult,
    queryFn: refreshReportWorkflowSchedules,
    queryKey: reportWorkflowSchedulesQueryKey,
    refreshMessage: "Report workflow schedules refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    items: notificationChannels,
    notice: notificationChannelsNotice,
    query: notificationChannelsQuery,
  } = useSettingsList({
    initialResult: notificationChannelsResult,
    queryFn: refreshNotificationChannelProfiles,
    queryKey: notificationChannelsQueryKey,
    refreshMessage: "Notification channels refreshed.",
    selectItems: (response) => response.items,
  });
  const {
    items: diagnosisToolTemplates,
    notice: diagnosisToolTemplatesNotice,
    query: diagnosisToolTemplatesQuery,
  } = useSettingsList({
    initialResult: diagnosisToolTemplatesResult,
    queryFn: refreshDiagnosisToolTemplates,
    queryKey: diagnosisToolTemplatesQueryKey,
    refreshMessage: "Diagnosis tool templates refreshed.",
    selectItems: (response) => response.items,
  });
  const canManageDirectory = currentAuthorization.can("directoryManage");
  const canReadDirectory = currentAuthorization.can("directoryRead");
  const canManageRBAC = currentAuthorization.can("rbacManage");
  const authorizationChecking =
    !clientReady || currentAuthorization.isChecking;
  const directoryReadPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadDirectory,
    errorStatus:
      usersErrorStatus ?? departmentsErrorStatus ?? syncRunsErrorStatus,
    isChecking: authorizationChecking,
    resourceLabel: "directory users, departments, and sync runs",
  });
  const directoryManagePermissionNotice = settingsManagePermissionNotice({
    canManage: canManageDirectory,
    isChecking: authorizationChecking,
    resourceLabel: "directory sync",
  });
  const rbacManagePermissionNotice = settingsManagePermissionNotice({
    canManage: canManageRBAC,
    isChecking: authorizationChecking,
    resourceLabel: "RBAC assignments and authorization previews",
  });
  const rbacReadPermissionNotice = settingsReadPermissionNotice({
    canRead: canManageRBAC,
    errorStatus: assignmentsErrorStatus,
    isChecking: authorizationChecking,
    resourceLabel: "RBAC assignments",
  });
  const diagnosisRoomsQuery = useQuery({
    enabled: clientReady && canManageRBAC,
    queryFn: () => refreshDiagnosisRooms(100),
    queryKey: diagnosisRoomsQueryKey,
    staleTime: 300_000,
  });
  const saveAssignment = useSettingsMutation<
    SaveAssignmentVariables,
    RBACAssignment
  >({
    invalidateQueryKey: rbacAssignmentsQueryKey,
    mutationFn: ({ body }) => submitRBACAssignment(body),
  });
  const syncDirectory = useSettingsMutation<
    SyncDirectoryVariables,
    DirectorySyncResponse
  >({
    invalidateQueryKeys: [
      directoryUsersQueryKey,
      directoryDepartmentsQueryKey,
      directorySyncRunsQueryKey,
    ],
    mutationFn: ({ body }) => runDirectorySync(body),
  });
  const assignmentScopeKind =
    Form.useWatch("scopeKind", assignmentForm) ?? "diagnosis_room";
  const assignmentSubjectKind =
    Form.useWatch("subjectKind", assignmentForm) ?? "department";
  const authorizeScopeKind =
    Form.useWatch("scopeKind", authorizeForm) ?? "diagnosis_room";
  const syncSummary = useMemo(
    () => directorySyncProjectionSummary(users, departments),
    [departments, users],
  );
  const activeUsers = useMemo(
    () => users.filter((user) => user.active).length,
    [users],
  );
  const enabledAssignments = useMemo(
    () => assignments.filter((assignment) => assignment.enabled).length,
    [assignments],
  );
  const nextStepNotice = directoryRBACNextStepNotice({
    canManageDirectory,
    canManageRBAC,
    enabledAssignments,
    summary: syncSummary,
  });
  const userSubjectOptions = useMemo(
    () => directoryUserSubjectOptions(users),
    [users],
  );
  const departmentSubjectOptions = useMemo(
    () => directoryDepartmentSubjectOptions(departments),
    [departments],
  );
  const directoryUsersBySubject = useMemo(
    () => directoryUserSubjectIndex(users),
    [users],
  );
  const departmentsByExternalID = useMemo(() => {
    const index = new Map<string, DirectoryDepartment>();
    departments.forEach((department) => {
      const externalID = department.external_id.trim();
      if (externalID !== "" && !index.has(externalID)) {
        index.set(externalID, department);
      }
    });
    return index;
  }, [departments]);
  const resourceScopeSelectOptions = useMemo(
    () =>
      rbacResourceScopeOptions({
        alertSources,
        diagnosisRooms:
          diagnosisRoomsQuery.data?.ok === true
            ? diagnosisRoomsQuery.data.data.items
            : [],
        diagnosisToolTemplates,
        groupingPolicies,
        notificationChannels,
        workflowPolicies,
        workflowSchedules,
      }),
    [
      alertSources,
      diagnosisRoomsQuery.data,
      diagnosisToolTemplates,
      groupingPolicies,
      notificationChannels,
      workflowPolicies,
      workflowSchedules,
    ],
  );
  const resourceScopeLoadingByKind: Partial<Record<RBACScopeKind, boolean>> = {
    alert_source: alertSourcesQuery.isFetching,
    diagnosis_room: diagnosisRoomsQuery.isFetching,
    diagnosis_tool_template: diagnosisToolTemplatesQuery.isFetching,
    grouping_policy: groupingPoliciesQuery.isFetching,
    notification_channel: notificationChannelsQuery.isFetching,
    report_workflow: workflowPoliciesQuery.isFetching,
    report_workflow_schedule: workflowSchedulesQuery.isFetching,
  };
  const resourceScopeFailureNotice =
    alertSourcesNotice ??
    groupingPoliciesNotice ??
    workflowPoliciesNotice ??
    workflowSchedulesNotice ??
    notificationChannelsNotice ??
    diagnosisToolTemplatesNotice;
  const assignmentSubjectOptions =
    assignmentSubjectKind === "user"
      ? userSubjectOptions
      : departmentSubjectOptions;
  const diagnosisRoomsNotice: SettingsNotice | null =
    diagnosisRoomsQuery.error !== null
      ? { kind: "warning", message: settingsErrorMessage(diagnosisRoomsQuery.error) }
      : diagnosisRoomsQuery.data?.ok === false
        ? { kind: "warning", message: diagnosisRoomsQuery.data.error.message }
        : null;
  const assignmentEmptyDescription = settingsReadPermissionEmptyDescription({
    canRead: canManageRBAC,
    emptyDescription: "No assignments",
    resourceLabel: "RBAC assignments",
  });
  const departmentEmptyDescription = settingsReadPermissionEmptyDescription({
    canRead: canReadDirectory,
    emptyDescription: "No departments",
    resourceLabel: "directory departments",
  });
  const syncRunEmptyDescription = settingsReadPermissionEmptyDescription({
    canRead: canReadDirectory,
    emptyDescription: "No sync runs",
    resourceLabel: "directory sync runs",
  });
  const userEmptyDescription = settingsReadPermissionEmptyDescription({
    canRead: canReadDirectory,
    emptyDescription: "No users",
    resourceLabel: "directory users",
  });
  const visibleNotice =
    notice ??
    currentAuthorization.notice ??
    directoryReadPermissionNotice ??
    rbacReadPermissionNotice ??
    usersNotice ??
    departmentsNotice ??
    syncRunsNotice ??
    assignmentsNotice ??
    resourceScopeFailureNotice ??
    diagnosisRoomsNotice;
  const busy =
    !clientReady ||
    currentAuthorization.isChecking ||
    syncDirectory.isPending ||
    previewing ||
    saveAssignment.isPending ||
    usersQuery.isFetching ||
    departmentsQuery.isFetching ||
    syncRunsQuery.isFetching ||
    assignmentsQuery.isFetching ||
    alertSourcesQuery.isFetching ||
    groupingPoliciesQuery.isFetching ||
    workflowPoliciesQuery.isFetching ||
    workflowSchedulesQuery.isFetching ||
    notificationChannelsQuery.isFetching ||
    diagnosisToolTemplatesQuery.isFetching;

  async function handleRefreshAll() {
    const refreshes: Array<Promise<unknown>> = [
      usersQuery.refetch(),
      departmentsQuery.refetch(),
      syncRunsQuery.refetch(),
      assignmentsQuery.refetch(),
      alertSourcesQuery.refetch(),
      groupingPoliciesQuery.refetch(),
      workflowPoliciesQuery.refetch(),
      workflowSchedulesQuery.refetch(),
      notificationChannelsQuery.refetch(),
      diagnosisToolTemplatesQuery.refetch(),
    ];
    if (canManageRBAC) {
      refreshes.push(diagnosisRoomsQuery.refetch());
    }
    await Promise.all(refreshes);
    setNotice({ kind: "info", message: "Directory and RBAC data refreshed." });
  }

  async function handleDirectorySync(values: DirectorySyncFormState) {
    const parsed = directorySyncFormToRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }
    let synced: DirectorySyncResponse;
    try {
      synced = await syncDirectory.mutateAsync({ body: parsed.value });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }
    setNotice({
      kind: "info",
      message: `Directory sync completed: ${synced.users_upserted} users and ${synced.departments_upserted} departments upserted; ${synced.users_deactivated} stale users deactivated.`,
    });
  }

  async function handleAssignmentSubmit(values: RBACAssignmentFormState) {
    const parsed = assignmentFormToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }
    try {
      await saveAssignment.mutateAsync({ body: parsed.value });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }
    await assignmentsQuery.refetch();
    assignmentForm.setFieldsValue(emptyRBACAssignmentForm());
    setNotice({ kind: "info", message: "RBAC assignment saved." });
  }

  async function handleAuthorize(values: RBACAuthorizeFormState) {
    const parsed = authorizeFormToRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }
    setPreviewing(true);
    const authorized = await previewRBACAuthorization(parsed.value);
    setPreviewing(false);
    if (!authorized.ok) {
      setNotice({ kind: "error", message: authorized.error.message });
      return;
    }
    setPreview({ kind: "ready", request: parsed.value, response: authorized.data });
    setNotice({
      kind: "info",
      message: authorized.data.allowed
        ? "Authorization preview allowed the request."
        : "Authorization preview denied the request.",
    });
  }

  function handleNextStepAction(action: DirectoryRBACNextStepNotice["action"]) {
    switch (action) {
      case "sync_directory":
        syncForm.setFieldsValue(emptyDirectorySyncForm());
        void handleDirectorySync(emptyDirectorySyncForm());
        return;
      case "prepare_assignment": {
        if (currentAuthorization.state.kind !== "ready") {
          setNotice({
            kind: "warning",
            message: "Current signed-in subject is not ready for assignment.",
          });
          return;
        }
        const assignment = bootstrapAdminAssignmentForm(
          currentAuthorization.state.subject,
        );
        if (!assignment.ok) {
          setNotice({
            kind: "warning",
            message: assignment.message,
          });
          return;
        }
        assignmentForm.setFieldsValue(assignment.value);
        setNotice({
          kind: "info",
          message:
            "Loaded a bootstrap global admin assignment for the current subject. Save it, then replace it with scoped team rules when rollout is complete.",
        });
        return;
      }
      case "open_diagnosis_room":
        window.location.assign("/diagnosis-room?auth_mode=session");
        return;
      case "none":
        return;
    }
  }

  function editAssignment(assignment: RBACAssignment) {
    assignmentForm.setFieldsValue({
      enabled: assignment.enabled,
      role: assignment.role,
      scopeKey: assignment.scope_key,
      scopeKind: assignment.scope_kind,
      subjectKey: assignment.subject_key,
      subjectKind: assignment.subject_kind,
    });
    setNotice({
      kind: "info",
      message: `Loaded RBAC assignment #${assignment.id}.`,
    });
  }

  return (
    <div className="stack">
      <Row aria-label="Directory and RBAC metrics" gutter={[12, 12]}>
        <MetricCard label="Users" value={users.length} />
        <MetricCard label="Active users" value={activeUsers} />
        <MetricCard label="Departments" value={departments.length} />
        <MetricCard label="Enabled rules" value={enabledAssignments} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} /> : null}
      <CurrentAuthorizationStatus
        authorization={currentAuthorization}
        checks={directoryRBACAuthorizationChecks}
      />
      <DirectoryRBACNextStepAlert
        busy={busy}
        notice={nextStepNotice}
        onAction={handleNextStepAction}
      />

      <Row align="top" className="settings-console-grid" gutter={[16, 16]}>
        <Col lg={8} md={24} xs={24}>
          <Space direction="vertical" size={16} style={{ width: "100%" }}>
            <Card
              extra={
                <Button
                  disabled={busy || !canReadDirectory}
                  icon={<ReloadOutlined />}
                  onClick={() => void handleRefreshAll()}
                >
                  Refresh
                </Button>
              }
              title="Directory Sync"
            >
              {directoryManagePermissionNotice ? (
                <ReadOnlyModeAlert notice={directoryManagePermissionNotice} />
              ) : null}
              <Form<DirectorySyncFormState>
                disabled={busy || !canManageDirectory}
                form={syncForm}
                initialValues={emptyDirectorySyncForm()}
                layout="vertical"
                onFinish={(values) => void handleDirectorySync(values)}
              >
                <Form.Item
                  label="Page size"
                  name="pageSize"
                  rules={[
                    { required: true, message: "Page size is required." },
                    {
                      type: "number",
                      min: 1,
                      max: 500,
                      message: "Page size must be between 1 and 500.",
                      transform: Number,
                    },
                  ]}
                >
                  <Input type="number" />
                </Form.Item>
                <Form.Item label="Updated after" name="updatedAfter">
                  <Input autoComplete="off" placeholder="2026-06-26T08:00:00Z" />
                </Form.Item>
                <Button
                  htmlType="submit"
                  icon={<ReloadOutlined />}
                  disabled={busy || !canManageDirectory}
                  loading={syncDirectory.isPending}
                  type="primary"
                >
                  Sync
                </Button>
              </Form>
              <DirectorySyncSummary
                result={syncDirectory.data ?? null}
                summary={syncSummary}
              />
            </Card>

            <Card title="Authorization Preview">
              {rbacManagePermissionNotice ? (
                <ReadOnlyModeAlert notice={rbacManagePermissionNotice} />
              ) : null}
              <Form<RBACAuthorizeFormState>
                disabled={busy || !canManageRBAC}
                form={authorizeForm}
                initialValues={emptyRBACAuthorizeForm()}
                layout="vertical"
                onFinish={(values) => void handleAuthorize(values)}
              >
                <Form.Item
                  label="Subject"
                  name="subject"
                  rules={[{ required: true, message: "Subject is required." }]}
                >
                  <Select
                    allowClear
                    onChange={(subject?: string) => {
                      authorizeForm.setFieldValue(
                        "departmentKeysText",
                        directoryUserDepartmentKeysText(users, subject ?? ""),
                      );
                    }}
                    optionFilterProp="search"
                    options={userSubjectOptions}
                    placeholder="Select a synced user"
                    showSearch
                  />
                </Form.Item>
                <Form.Item label="Department keys" name="departmentKeysText">
                  <Input.TextArea
                    autoSize={{ minRows: 2, maxRows: 6 }}
                    placeholder="dep-1, dep-2"
                  />
                </Form.Item>
                <Form.Item
                  label="Permission"
                  name="permission"
                  rules={[
                    { required: true, message: "Permission is required." },
                  ]}
                >
                  <Select options={rbacPermissionOptions} />
                </Form.Item>
                <Form.Item
                  label="Scope kind"
                  name="scopeKind"
                  rules={[
                    { required: true, message: "Scope kind is required." },
                  ]}
                >
                  <Select
                    onChange={() => {
                      authorizeForm.setFieldValue("scopeKey", "");
                    }}
                    options={rbacScopeKindOptions}
                  />
                </Form.Item>
                <Form.Item
                  label="Scope key"
                  name="scopeKey"
                  rules={[
                    {
                      required: scopeRequiresKey(authorizeScopeKind),
                      message: "Scope key is required.",
                    },
                  ]}
                >
                  <RBACScopeKeyInput
                    disabled={!scopeRequiresKey(authorizeScopeKind)}
                    loading={
                      resourceScopeLoadingByKind[authorizeScopeKind] ?? false
                    }
                    optionsByScope={resourceScopeSelectOptions}
                    scopeKind={authorizeScopeKind}
                  />
                </Form.Item>
                <Button
                  htmlType="submit"
                  icon={<AuditOutlined />}
                  disabled={busy || !canManageRBAC}
                  loading={previewing}
                  type="primary"
                >
                  Preview
                </Button>
              </Form>
              <AuthorizationPreview preview={preview} />
            </Card>
          </Space>
        </Col>

        <Col lg={16} md={24} xs={24}>
          <Space direction="vertical" size={16} style={{ width: "100%" }}>
            <Card title="RBAC Assignment">
              {rbacManagePermissionNotice ? (
                <ReadOnlyModeAlert notice={rbacManagePermissionNotice} />
              ) : null}
              <Form<RBACAssignmentFormState>
                disabled={busy || !canManageRBAC}
                form={assignmentForm}
                initialValues={emptyRBACAssignmentForm()}
                layout="vertical"
                onFinish={(values) => void handleAssignmentSubmit(values)}
              >
                <Row gutter={[12, 0]}>
                  <Col md={8} xs={24}>
                    <Form.Item
                      label="Subject kind"
                      name="subjectKind"
                      rules={[
                        {
                          required: true,
                          message: "Subject kind is required.",
                        },
                      ]}
                    >
                      <Select
                        onChange={() => {
                          assignmentForm.setFieldValue("subjectKey", "");
                        }}
                        options={rbacSubjectKindOptions}
                      />
                    </Form.Item>
                  </Col>
                  <Col md={16} xs={24}>
                    <Form.Item
                      label="Subject key"
                      name="subjectKey"
                      rules={[
                        {
                          required: true,
                          message: "Subject key is required.",
                        },
                      ]}
                    >
                      <Select
                        allowClear
                        optionFilterProp="search"
                        options={assignmentSubjectOptions}
                        placeholder={
                          assignmentSubjectKind === "user"
                            ? "Select a synced user"
                            : "Select a synced department"
                        }
                        showSearch
                      />
                    </Form.Item>
                  </Col>
                  <Col md={8} xs={24}>
                    <Form.Item
                      label="Role"
                      name="role"
                      rules={[{ required: true, message: "Role is required." }]}
                    >
                      <Select options={rbacRoleOptions} />
                    </Form.Item>
                  </Col>
                  <Col md={8} xs={24}>
                    <Form.Item
                      label="Scope kind"
                      name="scopeKind"
                      rules={[
                        {
                          required: true,
                          message: "Scope kind is required.",
                        },
                      ]}
                    >
                      <Select
                        onChange={() => {
                          assignmentForm.setFieldValue("scopeKey", "");
                        }}
                        options={rbacScopeKindOptions}
                      />
                    </Form.Item>
                  </Col>
                  <Col md={8} xs={24}>
                    <Form.Item
                      label="Scope key"
                      name="scopeKey"
                      rules={[
                        {
                          required: scopeRequiresKey(assignmentScopeKind),
                          message: "Scope key is required.",
                        },
                      ]}
                    >
                      <RBACScopeKeyInput
                        disabled={!scopeRequiresKey(assignmentScopeKind)}
                        loading={
                          resourceScopeLoadingByKind[assignmentScopeKind] ?? false
                        }
                        optionsByScope={resourceScopeSelectOptions}
                        scopeKind={assignmentScopeKind}
                      />
                    </Form.Item>
                  </Col>
                  <Col md={8} xs={24}>
                    <Form.Item
                      label="Enabled"
                      name="enabled"
                      valuePropName="checked"
                    >
                      <Switch />
                    </Form.Item>
                  </Col>
                </Row>
                <Space wrap>
                  <Button
                    htmlType="submit"
                    icon={<SaveOutlined />}
                    disabled={busy || !canManageRBAC}
                    loading={saveAssignment.isPending}
                    type="primary"
                  >
                    Save
                  </Button>
                  <Button
                    disabled={busy || !canManageRBAC}
                    onClick={() =>
                      assignmentForm.setFieldsValue(emptyRBACAssignmentForm())
                    }
                  >
                    Reset
                  </Button>
                </Space>
              </Form>
            </Card>

            <Tabs
              items={directoryTabs({
                assignments,
                assignmentEmptyDescription,
                canEditAssignments: canManageRBAC,
                departmentEmptyDescription,
                departments,
                departmentsByExternalID,
                directoryUsersBySubject,
                onEditAssignment: editAssignment,
                syncRunEmptyDescription,
                syncRuns,
                userEmptyDescription,
                users,
              })}
            />
          </Space>
        </Col>
      </Row>
    </div>
  );
}

function RBACScopeKeyInput({
  disabled,
  loading,
  optionsByScope,
  scopeKind,
}: {
  disabled: boolean;
  loading: boolean;
  optionsByScope: Partial<Record<RBACScopeKind, DirectorySelectOption[]>>;
  scopeKind: RBACScopeKind;
}) {
  const description = scopeKindDescription(scopeKind);
  const options = optionsByScope[scopeKind] ?? [];
  if (!loading && options.length === 0) {
    return (
      <Input
        autoComplete="off"
        disabled={disabled}
        placeholder={`Enter ${description}`}
      />
    );
  }
  return (
    <AutoComplete
      disabled={disabled}
      filterOption={(input, option) => {
        const candidate = option as DirectorySelectOption | undefined;
        return candidate === undefined
          ? false
          : candidate.search.toLowerCase().includes(input.trim().toLowerCase());
      }}
      options={options}
      placeholder={loading ? `Loading ${description}` : `Select or enter ${description}`}
      style={{ width: "100%" }}
    />
  );
}

function scopeKindDescription(scopeKind: RBACScopeKind): string {
  return (
    rbacScopeKindOptions
      .find((option) => option.value === scopeKind)
      ?.label.toLowerCase() ?? "scope key"
  );
}

function directoryTabs({
  assignments,
  assignmentEmptyDescription,
  canEditAssignments,
  departmentEmptyDescription,
  departments,
  departmentsByExternalID,
  directoryUsersBySubject,
  onEditAssignment,
  syncRunEmptyDescription,
  syncRuns,
  userEmptyDescription,
  users,
}: {
  assignments: RBACAssignment[];
  assignmentEmptyDescription: string;
  canEditAssignments: boolean;
  departmentEmptyDescription: string;
  departments: DirectoryDepartment[];
  departmentsByExternalID: ReadonlyMap<string, DirectoryDepartment>;
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  onEditAssignment: (assignment: RBACAssignment) => void;
  syncRunEmptyDescription: string;
  syncRuns: DirectorySyncRun[];
  userEmptyDescription: string;
  users: DirectoryUser[];
}): TabsProps["items"] {
  return [
    {
      children: (
        <AssignmentTable
          assignments={assignments}
          canEdit={canEditAssignments}
          departmentsByExternalID={departmentsByExternalID}
          directoryUsersBySubject={directoryUsersBySubject}
          emptyDescription={assignmentEmptyDescription}
          onEdit={onEditAssignment}
        />
      ),
      key: "assignments",
      label: "Assignments",
    },
    {
      children: (
        <UserTable emptyDescription={userEmptyDescription} users={users} />
      ),
      key: "users",
      label: "Users",
    },
    {
      children: (
        <DepartmentTable
          departments={departments}
          emptyDescription={departmentEmptyDescription}
        />
      ),
      key: "departments",
      label: "Departments",
    },
    {
      children: (
        <SyncRunTable
          emptyDescription={syncRunEmptyDescription}
          runs={syncRuns}
        />
      ),
      key: "sync-runs",
      label: "Sync runs",
    },
  ];
}

function AssignmentTable({
  assignments,
  canEdit,
  departmentsByExternalID,
  directoryUsersBySubject,
  emptyDescription,
  onEdit,
}: {
  assignments: RBACAssignment[];
  canEdit: boolean;
  departmentsByExternalID: ReadonlyMap<string, DirectoryDepartment>;
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  emptyDescription: string;
  onEdit: (assignment: RBACAssignment) => void;
}) {
  const columns: TableColumnsType<RBACAssignment> = [
    {
      dataIndex: "subject_key",
      key: "subject",
      render: (_: string, assignment) => (
        <AssignmentSubject
          assignment={assignment}
          departmentsByExternalID={departmentsByExternalID}
          directoryUsersBySubject={directoryUsersBySubject}
        />
      ),
      title: "Subject",
    },
    {
      dataIndex: "role",
      key: "role",
      render: (role: RBACAssignment["role"]) => (
        <Tag color={roleTagColor(role)}>{roleLabel(role)}</Tag>
      ),
      title: "Role",
    },
    {
      dataIndex: "scope_key",
      key: "scope",
      render: (_: string, assignment) => assignmentScopeLabel(assignment),
      title: "Scope",
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      render: (enabled: boolean) => (
        <Tag color={enabled ? "green" : "default"}>
          {enabled ? "Enabled" : "Disabled"}
        </Tag>
      ),
      title: "State",
      width: 120,
    },
    {
      dataIndex: "updated_at",
      key: "updated_at",
      render: (value: string, assignment) => (
        <Space direction="vertical" size={0}>
          <Typography.Text>{formatDateTime(value)}</Typography.Text>
          <Typography.Text type="secondary">
            {assignment.updated_by}
          </Typography.Text>
        </Space>
      ),
      title: "Updated",
      width: 220,
    },
    {
      key: "actions",
      render: (_: unknown, assignment) => (
        <Button
          disabled={!canEdit}
          icon={<EditOutlined />}
          onClick={() => onEdit(assignment)}
          size="small"
          type="default"
        >
          Edit
        </Button>
      ),
      title: "Action",
      width: 120,
    },
  ];
  return (
    <Table<RBACAssignment>
      columns={columns}
      dataSource={assignments}
      locale={{ emptyText: <Empty description={emptyDescription} /> }}
      pagination={{ pageSize: 10, size: "small" }}
      rowKey="id"
      scroll={{ x: 840 }}
      size="small"
    />
  );
}

function AssignmentSubject({
  assignment,
  departmentsByExternalID,
  directoryUsersBySubject,
}: {
  assignment: RBACAssignment;
  departmentsByExternalID: ReadonlyMap<string, DirectoryDepartment>;
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
}) {
  if (assignment.subject_kind === "user") {
    return (
      <DirectorySubjectTags
        directoryUsersBySubject={directoryUsersBySubject}
        label={subjectKindLabel(assignment.subject_kind)}
        subject={assignment.subject_key}
      />
    );
  }

  const subjectKey = assignment.subject_key.trim();
  const department = departmentsByExternalID.get(subjectKey);
  const displayName =
    department === undefined
      ? subjectKey
      : directoryDepartmentLabel(department);
  return (
    <Space size={[6, 6]} wrap>
      <Typography.Text type="secondary">
        {subjectKindLabel(assignment.subject_kind)}
      </Typography.Text>
      <Tag color={department === undefined ? "default" : "processing"}>
        {displayName}
      </Tag>
      {displayName !== subjectKey ? <Tag>{subjectKey}</Tag> : null}
      {department?.path ? <Tag>{department.path}</Tag> : null}
      {department === undefined ? <Tag color="default">not synced</Tag> : null}
    </Space>
  );
}

function UserTable({
  emptyDescription,
  users,
}: {
  emptyDescription: string;
  users: DirectoryUser[];
}) {
  const columns: TableColumnsType<DirectoryUser> = [
    {
      dataIndex: "display_name",
      key: "user",
      render: (_: string, user) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>{directoryUserLabel(user)}</Typography.Text>
          <Typography.Text copyable type="secondary">
            {user.subject}
          </Typography.Text>
        </Space>
      ),
      title: "User",
    },
    {
      dataIndex: "department_path",
      key: "department",
      render: (value: string, user) =>
        uniqueNonEmpty([
          value,
          ...user.department_paths,
          user.department,
        ]).join(", "),
      title: "Department",
    },
    {
      dataIndex: "active",
      key: "active",
      render: (active: boolean) => (
        <Tag color={active ? "green" : "default"}>
          {active ? "Active" : "Inactive"}
        </Tag>
      ),
      title: "State",
      width: 120,
    },
    {
      dataIndex: "synced_at",
      key: "synced_at",
      render: (value: string) => formatDateTime(value),
      title: "Synced",
      width: 180,
    },
  ];
  return (
    <Table<DirectoryUser>
      columns={columns}
      dataSource={users}
      locale={{ emptyText: <Empty description={emptyDescription} /> }}
      pagination={{ pageSize: 10, size: "small" }}
      rowKey="id"
      scroll={{ x: 760 }}
      size="small"
    />
  );
}

function DepartmentTable({
  departments,
  emptyDescription,
}: {
  departments: DirectoryDepartment[];
  emptyDescription: string;
}) {
  const columns: TableColumnsType<DirectoryDepartment> = [
    {
      dataIndex: "display_name",
      key: "department",
      render: (_: string, department) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>
            {directoryDepartmentLabel(department)}
          </Typography.Text>
          <Typography.Text copyable type="secondary">
            {department.external_id}
          </Typography.Text>
        </Space>
      ),
      title: "Department",
    },
    {
      dataIndex: "path",
      key: "path",
      title: "Path",
    },
    {
      dataIndex: "member_count",
      key: "member_count",
      title: "Members",
      width: 110,
    },
    {
      dataIndex: "synced_at",
      key: "synced_at",
      render: (value: string) => formatDateTime(value),
      title: "Synced",
      width: 180,
    },
  ];
  return (
    <Table<DirectoryDepartment>
      columns={columns}
      dataSource={departments}
      locale={{ emptyText: <Empty description={emptyDescription} /> }}
      pagination={{ pageSize: 10, size: "small" }}
      rowKey="id"
      scroll={{ x: 760 }}
      size="small"
    />
  );
}

function SyncRunTable({
  emptyDescription,
  runs,
}: {
  emptyDescription: string;
  runs: DirectorySyncRun[];
}) {
  const columns: TableColumnsType<DirectorySyncRun> = [
    {
      dataIndex: "synced_at",
      key: "synced_at",
      render: (value: string) => formatDateTime(value),
      title: "Synced",
      width: 180,
    },
    {
      dataIndex: "provider",
      key: "provider",
      title: "Provider",
      width: 140,
    },
    {
      dataIndex: "status",
      key: "status",
      render: (status: string) => (
        <Tag color={directorySyncRunStatusColor(status)}>{status}</Tag>
      ),
      title: "Status",
      width: 120,
    },
    {
      key: "pages",
      render: (_: unknown, run) => (
        <Space wrap>
          <Tag color="blue">{run.department_pages} department pages</Tag>
          <Tag color="cyan">{run.user_pages} user pages</Tag>
        </Space>
      ),
      title: "Pages",
    },
    {
      key: "upserts",
      render: (_: unknown, run) => (
        <Space wrap>
          <Tag color="green">{run.departments_upserted} departments</Tag>
          <Tag color="green">{run.users_upserted} users</Tag>
        </Space>
      ),
      title: "Upserts",
    },
    {
      dataIndex: "page_size",
      key: "page_size",
      title: "Page size",
      width: 110,
    },
    {
      dataIndex: "updated_after",
      key: "updated_after",
      render: (value?: string) =>
        value === undefined ? "Full sync" : formatDateTime(value),
      title: "Updated after",
      width: 180,
    },
    {
      key: "failure",
      render: (_: unknown, run) =>
        run.status === "failed" ? (
          <Space direction="vertical" size={0}>
            <Typography.Text>{run.failure_code}</Typography.Text>
            <Typography.Text type="secondary">
              {run.failure_message}
            </Typography.Text>
          </Space>
        ) : (
          <Typography.Text type="secondary">None</Typography.Text>
        ),
      title: "Failure",
      width: 260,
    },
  ];
  return (
    <Table<DirectorySyncRun>
      columns={columns}
      dataSource={runs}
      locale={{ emptyText: <Empty description={emptyDescription} /> }}
      pagination={{ pageSize: 10, size: "small" }}
      rowKey="id"
      scroll={{ x: 1240 }}
      size="small"
    />
  );
}

function directorySyncRunStatusColor(status: string): string {
  switch (status) {
    case "succeeded":
      return "green";
    case "failed":
      return "red";
    default:
      return "default";
  }
}

function DirectorySyncSummary({
  result,
  summary,
}: {
  result: DirectorySyncResponse | null;
  summary: DirectorySyncProjectionSummary;
}) {
  const providerText =
    summary.providers.length === 0 ? "No provider" : summary.providers.join(", ");
  return (
    <div className="settings-preview-panel">
      <Space direction="vertical" size={4}>
        <Space wrap>
          <Typography.Text type="secondary">Local projection</Typography.Text>
          <Tag color={directorySyncStatusColor(summary.status)}>
            {summary.statusLabel}
          </Tag>
        </Space>
        <Typography.Text>
          {summary.latestSyncAt === null
            ? "Not synced"
            : formatDateTime(summary.latestSyncAt)}
        </Typography.Text>
        <Typography.Text type="secondary">{providerText}</Typography.Text>
      </Space>
      <Space wrap>
        <Tag>{summary.userCount} users</Tag>
        <Tag>{summary.departmentCount} departments</Tag>
        <Tag color={summary.inactiveUsers === 0 ? "green" : "orange"}>
          {summary.inactiveUsers} inactive users
        </Tag>
        <Tag>{summary.providerCount} providers</Tag>
      </Space>
      {result === null ? null : (
        <Space wrap>
          <Tag color="purple">Run {formatDateTime(result.synced_at)}</Tag>
          <Tag color="blue">{result.user_pages} user pages</Tag>
          <Tag color="cyan">{result.department_pages} department pages</Tag>
          <Tag color="green">{result.users_upserted} users</Tag>
          <Tag color={result.users_deactivated === 0 ? "green" : "orange"}>
            {result.users_deactivated} deactivated
          </Tag>
          <Tag color="green">{result.departments_upserted} departments</Tag>
        </Space>
      )}
    </div>
  );
}

function directorySyncStatusColor(
  status: DirectorySyncProjectionSummary["status"],
): string {
  switch (status) {
    case "current":
      return "green";
    case "empty":
      return "default";
    case "stale":
      return "orange";
  }
}

function AuthorizationPreview({ preview }: { preview: PreviewState }) {
  if (preview.kind === "empty") {
    return null;
  }
  return (
    <Alert
      className="settings-preview-panel"
      description={
        <Space direction="vertical" size={4}>
          <Typography.Text>
            {permissionLabel(preview.request.permission)} on{" "}
            {scopeLabel(
              preview.request.scope_kind,
              preview.request.scope_key,
            )}
          </Typography.Text>
          <Typography.Text type="secondary">
            Checked {formatDateTime(preview.response.checked_at)}
          </Typography.Text>
        </Space>
      }
      message={preview.response.allowed ? "Allowed" : "Denied"}
      showIcon
      type={preview.response.allowed ? "success" : "warning"}
    />
  );
}

function MetricCard({ label, value }: { label: string; value: number }) {
  return (
    <Col lg={6} md={12} xs={24}>
      <Card className="settings-stat-card">
        <Statistic title={label} value={value} />
      </Card>
    </Col>
  );
}

function Notice({ notice }: { notice: SettingsNotice }) {
  return (
    <Alert
      message={notice.message}
      role="status"
      showIcon
      type={notice.kind === "error" ? "error" : notice.kind}
    />
  );
}

function DirectoryRBACNextStepAlert({
  busy,
  notice,
  onAction,
}: {
  busy: boolean;
  notice: DirectoryRBACNextStepNotice;
  onAction: (action: DirectoryRBACNextStepNotice["action"]) => void;
}) {
  return (
    <Alert
      action={
        <Button
          disabled={busy || notice.action === "none"}
          icon={directoryRBACNextStepActionIcon(notice.action)}
          onClick={() => onAction(notice.action)}
          size="small"
          type={notice.status === "ready" ? "primary" : "default"}
        >
          {notice.actionLabel}
        </Button>
      }
      description={notice.detail}
      message={notice.message}
      showIcon
      type={directoryRBACNextStepAlertType(notice.status)}
    />
  );
}

function directoryRBACNextStepActionIcon(
  action: DirectoryRBACNextStepNotice["action"],
) {
  switch (action) {
    case "sync_directory":
      return <ReloadOutlined />;
    case "prepare_assignment":
      return <EditOutlined />;
    case "open_diagnosis_room":
      return <LoginOutlined />;
    case "none":
      return undefined;
  }
}

function directoryRBACNextStepAlertType(
  status: DirectoryRBACNextStepNotice["status"],
): "success" | "warning" {
  switch (status) {
    case "ready":
      return "success";
    case "blocked":
    case "review":
      return "warning";
  }
}

function CurrentAuthorizationStatus({
  authorization,
  checks,
}: {
  authorization: ReturnType<typeof useCurrentRBACAuthorizations>;
  checks: readonly CurrentRBACAuthorizationCheck[];
}) {
  if (authorization.isChecking || authorization.state.kind === "loading") {
    return (
      <Alert
        description="Checking local RBAC decisions for the signed-in operator."
        message="Current authorization"
        showIcon
        type="info"
      />
    );
  }
  if (authorization.state.kind === "error") {
    const action =
      currentRBACAuthorizationNeedsSignIn(authorization.state) ? (
        <Button
          href={directoryRBACSettingsLoginHref}
          icon={<LoginOutlined />}
          type="primary"
        >
          Sign in with IAM
        </Button>
      ) : undefined;
    return (
      <Alert
        action={action}
        description={authorization.state.message}
        message="Current authorization unavailable"
        showIcon
        type="warning"
      />
    );
  }

  const hasDeniedCheck = checks.some((check) => !authorization.can(check.key));
  const directoryUsers = authorization.state.directoryUsers;
  const activeDirectoryUsers = directoryUsers.filter((user) => user.active);
  const primaryDirectoryUser = activeDirectoryUsers[0] ?? directoryUsers[0];
  const hasDirectoryProjection = directoryUsers.length > 0;
  return (
    <Alert
      description={
        <Space direction="vertical" size={8}>
          <Space size={6} wrap>
            <Typography.Text type="secondary">Subject</Typography.Text>
            <Typography.Text copyable>
              {authorization.state.subject}
            </Typography.Text>
          </Space>
          <Space size={6} wrap>
            <Typography.Text type="secondary">Directory profile</Typography.Text>
            {primaryDirectoryUser ? (
              <>
                <Typography.Text>
                  {directoryUserLabel(primaryDirectoryUser)}
                </Typography.Text>
                <Tag color={primaryDirectoryUser.active ? "green" : "orange"}>
                  {primaryDirectoryUser.active ? "Active" : "Inactive"}
                </Tag>
              </>
            ) : (
              <Tag color="orange">Not synced</Tag>
            )}
          </Space>
          <Space size={6} wrap>
            <Typography.Text type="secondary">Department keys</Typography.Text>
            {authorization.state.departmentKeys.length > 0 ? (
              authorization.state.departmentKeys.map((departmentKey) => (
                <Tag key={departmentKey}>{departmentKey}</Tag>
              ))
            ) : (
              <Tag color="orange">None</Tag>
            )}
          </Space>
          <Space wrap>
            {checks.map((check) => {
              const allowed = authorization.can(check.key);
              return (
                <Tag
                  color={allowed ? "green" : "red"}
                  key={check.key}
                >
                  {permissionLabel(check.permission)}{" "}
                  {allowed ? "Allowed" : "Denied"}
                </Tag>
              );
            })}
          </Space>
        </Space>
      }
      message="Current authorization"
      showIcon
      type={hasDeniedCheck || !hasDirectoryProjection ? "warning" : "success"}
    />
  );
}

function roleTagColor(role: RBACAssignment["role"]): string {
  switch (role) {
    case "admin":
      return "red";
    case "operator":
      return "blue";
    case "responder":
      return "green";
    case "viewer":
      return "default";
  }
}
