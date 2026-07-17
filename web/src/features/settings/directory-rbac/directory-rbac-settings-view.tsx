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
import { useLocale, useTranslations } from "next-intl";
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
  currentRBACAuthorizationErrorDetail,
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

type DirectoryTranslator = ReturnType<typeof useTranslations<"DirectorySettings">>;

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
  const t = useTranslations("DirectorySettings");
  const common = useTranslations("Common");
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
    refreshMessage: t("usersRefreshed"),
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
    refreshMessage: t("departmentsRefreshed"),
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
    refreshMessage: t("assignmentsRefreshed"),
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
    refreshMessage: t("syncRunsRefreshed"),
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
    refreshMessage: t("sourcesRefreshed"),
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
    refreshMessage: t("groupingRefreshed"),
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
    refreshMessage: t("workflowsRefreshed"),
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
    refreshMessage: t("schedulesRefreshed"),
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
    refreshMessage: t("channelsRefreshed"),
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
    refreshMessage: t("toolsRefreshed"),
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
    message: common("readAccessLimited", {
      resource: t("directoryResources"),
    }),
  });
  const directoryManagePermissionNotice = settingsManagePermissionNotice({
    canManage: canManageDirectory,
    isChecking: authorizationChecking,
    message: common("formReadOnly", {
      resource: t("directorySyncResource"),
    }),
  });
  const rbacManagePermissionNotice = settingsManagePermissionNotice({
    canManage: canManageRBAC,
    isChecking: authorizationChecking,
    message: common("formReadOnly", {
      resource: t("rbacManageResource"),
    }),
  });
  const rbacReadPermissionNotice = settingsReadPermissionNotice({
    canRead: canManageRBAC,
    errorStatus: assignmentsErrorStatus,
    isChecking: authorizationChecking,
    message: common("readAccessLimited", {
      resource: t("assignmentsResource"),
    }),
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
      ? {
          kind: "warning",
          message: settingsErrorMessage(
            diagnosisRoomsQuery.error,
            common("requestFailed"),
          ),
        }
      : diagnosisRoomsQuery.data?.ok === false
        ? { kind: "warning", message: diagnosisRoomsQuery.data.error.message }
        : null;
  const assignmentEmptyDescription = settingsReadPermissionEmptyDescription({
    canRead: canManageRBAC,
    deniedDescription: common("noReadAccess", {
      resource: t("assignmentsResource"),
    }),
    emptyDescription: t("noAssignments"),
  });
  const departmentEmptyDescription = settingsReadPermissionEmptyDescription({
    canRead: canReadDirectory,
    deniedDescription: common("noReadAccess", {
      resource: t("departmentsResource"),
    }),
    emptyDescription: t("noDepartments"),
  });
  const syncRunEmptyDescription = settingsReadPermissionEmptyDescription({
    canRead: canReadDirectory,
    deniedDescription: common("noReadAccess", {
      resource: t("syncRunsResource"),
    }),
    emptyDescription: t("noSyncRuns"),
  });
  const userEmptyDescription = settingsReadPermissionEmptyDescription({
    canRead: canReadDirectory,
    deniedDescription: common("noReadAccess", {
      resource: t("usersResource"),
    }),
    emptyDescription: t("noUsers"),
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
    setNotice({ kind: "info", message: t("allRefreshed") });
  }

  async function handleDirectorySync(values: DirectorySyncFormState) {
    const parsed = directorySyncFormToRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeDirectoryText(parsed.message, t) });
      return;
    }
    let synced: DirectorySyncResponse;
    try {
      synced = await syncDirectory.mutateAsync({ body: parsed.value });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      return;
    }
    setNotice({
      kind: "info",
      message: t("syncCompleted", {
        departments: synced.departments_upserted,
        users: synced.users_upserted,
        deactivated: synced.users_deactivated,
      }),
    });
  }

  async function handleAssignmentSubmit(values: RBACAssignmentFormState) {
    const parsed = assignmentFormToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeDirectoryText(parsed.message, t) });
      return;
    }
    try {
      await saveAssignment.mutateAsync({ body: parsed.value });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      return;
    }
    await assignmentsQuery.refetch();
    assignmentForm.setFieldsValue(emptyRBACAssignmentForm());
    setNotice({ kind: "info", message: t("assignmentSaved") });
  }

  async function handleAuthorize(values: RBACAuthorizeFormState) {
    const parsed = authorizeFormToRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeDirectoryText(parsed.message, t) });
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
        ? t("previewAllowed")
        : t("previewDenied"),
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
            message: t("subjectNotReady"),
          });
          return;
        }
        const assignment = bootstrapAdminAssignmentForm(
          currentAuthorization.state.subject,
        );
        if (!assignment.ok) {
          setNotice({
            kind: "warning",
            message: localizeDirectoryText(assignment.message, t),
          });
          return;
        }
        assignmentForm.setFieldsValue(assignment.value);
        setNotice({
          kind: "info",
          message:
            t("bootstrapLoaded"),
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
      message: t("assignmentLoaded", { id: assignment.id }),
    });
  }

  return (
    <div className="stack">
      <Row aria-label={t("metricsLabel")} gutter={[12, 12]}>
        <MetricCard label={t("users")} value={users.length} />
        <MetricCard label={t("activeUsers")} value={activeUsers} />
        <MetricCard label={t("departments")} value={departments.length} />
        <MetricCard label={t("enabledRules")} value={enabledAssignments} />
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
                  {t("refresh")}
                </Button>
              }
              title={t("directorySync")}
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
                  label={t("pageSize")}
                  name="pageSize"
                  rules={[
                    { required: true, message: t("pageSizeRequired") },
                    {
                      type: "number",
                      min: 1,
                      max: 500,
                      message: t("pageSizeRange"),
                      transform: Number,
                    },
                  ]}
                >
                  <Input type="number" />
                </Form.Item>
                <Form.Item label={t("updatedAfter")} name="updatedAfter">
                  <Input autoComplete="off" placeholder="2026-06-26T08:00:00Z" />
                </Form.Item>
                <Button
                  htmlType="submit"
                  icon={<ReloadOutlined />}
                  disabled={busy || !canManageDirectory}
                  loading={syncDirectory.isPending}
                  type="primary"
                >
                  {t("sync")}
                </Button>
              </Form>
              <DirectorySyncSummary
                result={syncDirectory.data ?? null}
                summary={syncSummary}
              />
            </Card>

            <Card title={t("authorizationPreview")}>
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
                  label={t("subject")}
                  name="subject"
                  rules={[{ required: true, message: t("subjectRequired") }]}
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
                    placeholder={t("selectUser")}
                    showSearch
                  />
                </Form.Item>
                <Form.Item label={t("departmentKeys")} name="departmentKeysText">
                  <Input.TextArea
                    autoSize={{ minRows: 2, maxRows: 6 }}
                    placeholder="dep-1, dep-2"
                  />
                </Form.Item>
                <Form.Item
                  label={t("permission")}
                  name="permission"
                  rules={[
                    { required: true, message: t("permissionRequired") },
                  ]}
                >
                  <Select options={localizedDirectoryOptions(rbacPermissionOptions, t)} />
                </Form.Item>
                <Form.Item
                  label={t("scopeKind")}
                  name="scopeKind"
                  rules={[
                    { required: true, message: t("scopeKindRequired") },
                  ]}
                >
                  <Select
                    onChange={() => {
                      authorizeForm.setFieldValue("scopeKey", "");
                    }}
                    options={localizedDirectoryOptions(rbacScopeKindOptions, t)}
                  />
                </Form.Item>
                <Form.Item
                  label={t("scopeKey")}
                  name="scopeKey"
                  rules={[
                    {
                      required: scopeRequiresKey(authorizeScopeKind),
                      message: t("scopeKeyRequired"),
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
                  {t("preview")}
                </Button>
              </Form>
              <AuthorizationPreview preview={preview} />
            </Card>
          </Space>
        </Col>

        <Col lg={16} md={24} xs={24}>
          <Space direction="vertical" size={16} style={{ width: "100%" }}>
            <Card title={t("rbacAssignment")}>
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
                      label={t("subjectKind")}
                      name="subjectKind"
                      rules={[
                        {
                          required: true,
                          message: t("subjectKindRequired"),
                        },
                      ]}
                    >
                      <Select
                        onChange={() => {
                          assignmentForm.setFieldValue("subjectKey", "");
                        }}
                        options={localizedDirectoryOptions(rbacSubjectKindOptions, t)}
                      />
                    </Form.Item>
                  </Col>
                  <Col md={16} xs={24}>
                    <Form.Item
                      label={t("subjectKey")}
                      name="subjectKey"
                      rules={[
                        {
                          required: true,
                          message: t("subjectKeyRequired"),
                        },
                      ]}
                    >
                      <Select
                        allowClear
                        optionFilterProp="search"
                        options={assignmentSubjectOptions}
                        placeholder={
                          assignmentSubjectKind === "user"
                            ? t("selectUser")
                            : t("selectDepartment")
                        }
                        showSearch
                      />
                    </Form.Item>
                  </Col>
                  <Col md={8} xs={24}>
                    <Form.Item
                      label={t("role")}
                      name="role"
                      rules={[{ required: true, message: t("roleRequired") }]}
                    >
                      <Select options={localizedDirectoryOptions(rbacRoleOptions, t)} />
                    </Form.Item>
                  </Col>
                  <Col md={8} xs={24}>
                    <Form.Item
                      label={t("scopeKind")}
                      name="scopeKind"
                      rules={[
                        {
                          required: true,
                          message: t("scopeKindRequired"),
                        },
                      ]}
                    >
                      <Select
                        onChange={() => {
                          assignmentForm.setFieldValue("scopeKey", "");
                        }}
                        options={localizedDirectoryOptions(rbacScopeKindOptions, t)}
                      />
                    </Form.Item>
                  </Col>
                  <Col md={8} xs={24}>
                    <Form.Item
                      label={t("scopeKey")}
                      name="scopeKey"
                      rules={[
                        {
                          required: scopeRequiresKey(assignmentScopeKind),
                          message: t("scopeKeyRequired"),
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
                      label={t("enabled")}
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
                    {t("save")}
                  </Button>
                  <Button
                    disabled={busy || !canManageRBAC}
                    onClick={() =>
                      assignmentForm.setFieldsValue(emptyRBACAssignmentForm())
                    }
                  >
                    {t("reset")}
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
                t,
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
  const t = useTranslations("DirectorySettings");
  const description = scopeKindDescription(scopeKind, t);
  const options = (optionsByScope[scopeKind] ?? []).map((option) => ({
    ...option,
    label: localizeDirectoryText(option.label, t),
  }));
  if (!loading && options.length === 0) {
    return (
      <Input
        autoComplete="off"
        disabled={disabled}
        placeholder={t("enterScope", { description })}
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
      placeholder={loading ? t("loadingScope", { description }) : t("selectScope", { description })}
      style={{ width: "100%" }}
    />
  );
}

function scopeKindDescription(
  scopeKind: RBACScopeKind,
  t: DirectoryTranslator,
): string {
  const label =
    rbacScopeKindOptions.find((option) => option.value === scopeKind)?.label ??
    "scope key";
  return localizeDirectoryText(label, t);
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
  t,
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
  t: DirectoryTranslator;
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
      label: t("assignments"),
    },
    {
      children: (
        <UserTable emptyDescription={userEmptyDescription} users={users} />
      ),
      key: "users",
      label: t("users"),
    },
    {
      children: (
        <DepartmentTable
          departments={departments}
          emptyDescription={departmentEmptyDescription}
        />
      ),
      key: "departments",
      label: t("departments"),
    },
    {
      children: (
        <SyncRunTable
          emptyDescription={syncRunEmptyDescription}
          runs={syncRuns}
        />
      ),
      key: "sync-runs",
      label: t("syncRuns"),
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
  const locale = useLocale();
  const t = useTranslations("DirectorySettings");
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
      title: t("subject"),
    },
    {
      dataIndex: "role",
      key: "role",
      render: (role: RBACAssignment["role"]) => (
        <Tag color={roleTagColor(role)}>{localizeDirectoryText(roleLabel(role), t)}</Tag>
      ),
      title: t("role"),
    },
    {
      dataIndex: "scope_key",
      key: "scope",
      render: (_: string, assignment) => localizeDirectoryText(assignmentScopeLabel(assignment), t),
      title: t("scope"),
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      render: (enabled: boolean) => (
        <Tag color={enabled ? "green" : "default"}>
          {enabled ? t("enabled") : t("disabled")}
        </Tag>
      ),
      title: t("state"),
      width: 120,
    },
    {
      dataIndex: "updated_at",
      key: "updated_at",
      render: (value: string, assignment) => (
        <Space direction="vertical" size={0}>
          <Typography.Text>{formatDateTime(value, locale)}</Typography.Text>
          <Typography.Text type="secondary">
            {assignment.updated_by}
          </Typography.Text>
        </Space>
      ),
      title: t("updated"),
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
          {t("edit")}
        </Button>
      ),
      title: t("action"),
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
  const t = useTranslations("DirectorySettings");
  if (assignment.subject_kind === "user") {
    return (
      <DirectorySubjectTags
        directoryUsersBySubject={directoryUsersBySubject}
        label={localizeDirectoryText(subjectKindLabel(assignment.subject_kind), t)}
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
        {localizeDirectoryText(subjectKindLabel(assignment.subject_kind), t)}
      </Typography.Text>
      <Tag color={department === undefined ? "default" : "processing"}>
        {displayName}
      </Tag>
      {displayName !== subjectKey ? <Tag>{subjectKey}</Tag> : null}
      {department?.path ? <Tag>{department.path}</Tag> : null}
      {department === undefined ? <Tag color="default">{t("notSynced")}</Tag> : null}
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
  const locale = useLocale();
  const t = useTranslations("DirectorySettings");
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
      title: t("user"),
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
      title: t("department"),
    },
    {
      dataIndex: "active",
      key: "active",
      render: (active: boolean) => (
        <Tag color={active ? "green" : "default"}>
          {active ? t("active") : t("inactive")}
        </Tag>
      ),
      title: t("state"),
      width: 120,
    },
    {
      dataIndex: "synced_at",
      key: "synced_at",
      render: (value: string) => formatDateTime(value, locale),
      title: t("synced"),
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
  const locale = useLocale();
  const t = useTranslations("DirectorySettings");
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
      title: t("department"),
    },
    {
      dataIndex: "path",
      key: "path",
      title: t("path"),
    },
    {
      dataIndex: "member_count",
      key: "member_count",
      title: t("members"),
      width: 110,
    },
    {
      dataIndex: "synced_at",
      key: "synced_at",
      render: (value: string) => formatDateTime(value, locale),
      title: t("synced"),
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
  const locale = useLocale();
  const t = useTranslations("DirectorySettings");
  const columns: TableColumnsType<DirectorySyncRun> = [
    {
      dataIndex: "synced_at",
      key: "synced_at",
      render: (value: string) => formatDateTime(value, locale),
      title: t("synced"),
      width: 180,
    },
    {
      dataIndex: "provider",
      key: "provider",
      title: t("provider"),
      width: 140,
    },
    {
      dataIndex: "status",
      key: "status",
      render: (status: string) => (
        <Tag color={directorySyncRunStatusColor(status)}>{localizeDirectoryText(status, t)}</Tag>
      ),
      title: t("status"),
      width: 120,
    },
    {
      key: "pages",
      render: (_: unknown, run) => (
        <Space wrap>
          <Tag color="blue">{t("departmentPages", { count: run.department_pages })}</Tag>
          <Tag color="cyan">{t("userPages", { count: run.user_pages })}</Tag>
        </Space>
      ),
      title: t("pages"),
    },
    {
      key: "upserts",
      render: (_: unknown, run) => (
        <Space wrap>
          <Tag color="green">{t("departmentCount", { count: run.departments_upserted })}</Tag>
          <Tag color="green">{t("userCount", { count: run.users_upserted })}</Tag>
        </Space>
      ),
      title: t("upserts"),
    },
    {
      dataIndex: "page_size",
      key: "page_size",
      title: t("pageSize"),
      width: 110,
    },
    {
      dataIndex: "updated_after",
      key: "updated_after",
      render: (value?: string) =>
        value === undefined ? t("fullSync") : formatDateTime(value, locale),
      title: t("updatedAfter"),
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
          <Typography.Text type="secondary">{t("none")}</Typography.Text>
        ),
      title: t("failure"),
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
  const locale = useLocale();
  const t = useTranslations("DirectorySettings");
  const providerText =
    summary.providers.length === 0 ? t("noProvider") : summary.providers.join(", ");
  return (
    <div className="settings-preview-panel">
      <Space direction="vertical" size={4}>
        <Space wrap>
          <Typography.Text type="secondary">{t("localProjection")}</Typography.Text>
          <Tag color={directorySyncStatusColor(summary.status)}>
            {localizeDirectoryText(summary.statusLabel, t)}
          </Tag>
        </Space>
        <Typography.Text>
          {summary.latestSyncAt === null
            ? t("notSynced")
            : formatDateTime(summary.latestSyncAt, locale)}
        </Typography.Text>
        <Typography.Text type="secondary">{providerText}</Typography.Text>
      </Space>
      <Space wrap>
        <Tag>{t("userCount", { count: summary.userCount })}</Tag>
        <Tag>{t("departmentCount", { count: summary.departmentCount })}</Tag>
        <Tag color={summary.inactiveUsers === 0 ? "green" : "orange"}>
          {t("inactiveUserCount", { count: summary.inactiveUsers })}
        </Tag>
        <Tag>{t("providerCount", { count: summary.providerCount })}</Tag>
      </Space>
      {result === null ? null : (
        <Space wrap>
          <Tag color="purple">{t("runAt", { time: formatDateTime(result.synced_at, locale) })}</Tag>
          <Tag color="blue">{t("userPages", { count: result.user_pages })}</Tag>
          <Tag color="cyan">{t("departmentPages", { count: result.department_pages })}</Tag>
          <Tag color="green">{t("userCount", { count: result.users_upserted })}</Tag>
          <Tag color={result.users_deactivated === 0 ? "green" : "orange"}>
            {t("deactivatedCount", { count: result.users_deactivated })}
          </Tag>
          <Tag color="green">{t("departmentCount", { count: result.departments_upserted })}</Tag>
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
  const locale = useLocale();
  const t = useTranslations("DirectorySettings");
  if (preview.kind === "empty") {
    return null;
  }
  return (
    <Alert
      className="settings-preview-panel"
      description={
        <Space direction="vertical" size={4}>
          <Typography.Text>
            {localizeDirectoryText(permissionLabel(preview.request.permission), t)} {t("onScope")}{" "}
            {localizeDirectoryText(scopeLabel(
              preview.request.scope_kind,
              preview.request.scope_key,
            ), t)}
          </Typography.Text>
          <Typography.Text type="secondary">
            {t("checkedAt", { time: formatDateTime(preview.response.checked_at, locale) })}
          </Typography.Text>
        </Space>
      }
      message={preview.response.allowed ? t("allowed") : t("denied")}
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
  const t = useTranslations("DirectorySettings");
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
          {localizeDirectoryText(notice.actionLabel, t)}
        </Button>
      }
      description={localizeDirectoryText(notice.detail, t)}
      message={localizeDirectoryText(notice.message, t)}
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
  const common = useTranslations("Common");
  const t = useTranslations("DirectorySettings");
  if (authorization.isChecking || authorization.state.kind === "loading") {
    return (
      <Alert
        description={t("checkingAuthorization")}
        message={t("currentAuthorization")}
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
          {t("signInIam")}
        </Button>
      ) : undefined;
    return (
      <Alert
        action={action}
        description={currentRBACAuthorizationErrorDetail(
          authorization.state,
          common("authenticationRequired"),
        )}
        message={t("authorizationUnavailable")}
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
            <Typography.Text type="secondary">{t("subject")}</Typography.Text>
            <Typography.Text copyable>
              {authorization.state.subject}
            </Typography.Text>
          </Space>
          <Space size={6} wrap>
            <Typography.Text type="secondary">{t("directoryProfile")}</Typography.Text>
            {primaryDirectoryUser ? (
              <>
                <Typography.Text>
                  {directoryUserLabel(primaryDirectoryUser)}
                </Typography.Text>
                <Tag color={primaryDirectoryUser.active ? "green" : "orange"}>
                  {primaryDirectoryUser.active ? t("active") : t("inactive")}
                </Tag>
              </>
            ) : (
              <Tag color="orange">{t("notSynced")}</Tag>
            )}
          </Space>
          <Space size={6} wrap>
            <Typography.Text type="secondary">{t("departmentKeys")}</Typography.Text>
            {authorization.state.departmentKeys.length > 0 ? (
              authorization.state.departmentKeys.map((departmentKey) => (
                <Tag key={departmentKey}>{departmentKey}</Tag>
              ))
            ) : (
              <Tag color="orange">{t("none")}</Tag>
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
                  {localizeDirectoryText(permissionLabel(check.permission), t)}{" "}
                  {allowed ? t("allowed") : t("denied")}
                </Tag>
              );
            })}
          </Space>
        </Space>
      }
      message={t("currentAuthorization")}
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
    case "leader":
      return "gold";
    case "responder":
      return "green";
    case "viewer":
      return "default";
  }
}

function localizedDirectoryOptions<T extends { label: string }>(
  options: readonly T[],
  t: DirectoryTranslator,
): T[] {
  return options.map((option) => ({
    ...option,
    label: localizeDirectoryText(option.label, t),
  }));
}

const directoryRuntimeTextKeys = {
    "failed": "runtimeText.failed",
    "succeeded": "runtimeText.succeeded",
    "Admin": "runtimeText.admin",
    "Alert source": "runtimeText.alertSource",
    "Alert source manage": "runtimeText.alertSourceManage",
    "Alert source read": "runtimeText.alertSourceRead",
    "Create assignment": "runtimeText.createAssignment",
    "Current": "runtimeText.current",
    "Department": "runtimeText.department",
    "Diagnosis room": "runtimeText.diagnosisRoom",
    "Diagnosis room administer": "runtimeText.diagnosisRoomAdminister",
    "Diagnosis room approve": "runtimeText.diagnosisRoomApprove",
    "Diagnosis room participate": "runtimeText.diagnosisRoomParticipate",
    "Diagnosis room read": "runtimeText.diagnosisRoomRead",
    "Diagnosis tool template": "runtimeText.diagnosisToolTemplate",
    "Diagnosis tool template manage": "runtimeText.diagnosisToolTemplateManage",
    "Diagnosis tool template read": "runtimeText.diagnosisToolTemplateRead",
    "Directory manage": "runtimeText.directoryManage",
    "Directory read": "runtimeText.directoryRead",
    "Global": "runtimeText.global",
    "Grouping policy": "runtimeText.groupingPolicy",
    "Grouping policy manage": "runtimeText.groupingPolicyManage",
    "Grouping policy read": "runtimeText.groupingPolicyRead",
    "Leader": "runtimeText.leader",
    "Need directory manager": "runtimeText.needDirectoryManager",
    "Need RBAC manager": "runtimeText.needRbacManager",
    "Not synced": "runtimeText.notSynced",
    "Notification channel": "runtimeText.notificationChannel",
    "Notification channel manage": "runtimeText.notificationChannelManage",
    "Notification channel read": "runtimeText.notificationChannelRead",
    "Notification channel test": "runtimeText.notificationChannelTest",
    "Operator": "runtimeText.operator",
    "Open diagnosis room": "runtimeText.openDiagnosisRoom",
    "Operations read": "runtimeText.operationsRead",
    "RBAC manage": "runtimeText.rbacManage",
    "Report workflow": "runtimeText.reportWorkflow",
    "Report workflow manage": "runtimeText.reportWorkflowManage",
    "Report workflow read": "runtimeText.reportWorkflowRead",
    "Report workflow schedule": "runtimeText.reportWorkflowSchedule",
    "Responder": "runtimeText.responder",
    "Run full sync": "runtimeText.runFullSync",
    "Stale": "runtimeText.stale",
    "User": "runtimeText.user",
    "Viewer": "runtimeText.viewer",
    "Access control is ready for manual checks": "runtimeText.accessControlIsReadyForManualChecks",
    "Current signed-in subject is empty.": "runtimeText.currentSignedInSubjectIsEmpty",
    "Directory projection is required": "runtimeText.directoryProjectionIsRequired",
    "Directory sync page size must be between 1 and 500.": "runtimeText.directorySyncPageSizeMustBeBetween1And500",
    "Local RBAC assignments are required": "runtimeText.localRbacAssignmentsAreRequired",
    "Scope key is required.": "runtimeText.scopeKeyIsRequired",
    "Subject is required.": "runtimeText.subjectIsRequired",
    "Subject key is required.": "runtimeText.subjectKeyIsRequired",
    "Updated-after timestamp must be a valid date-time value.": "runtimeText.updatedAfterTimestampMustBeAValidDateTimeValue",
    "Directory projection and enabled RBAC assignments are present. Use Authorization Preview for a specific subject, permission, and scope before manual diagnosis-room testing.": "runtimeText.directoryProjectionAndEnabledRbacAssignmentsArePresentUseAuthorizationPreviewFor",
    "Sync the IAM directory projection before assigning OpenClarion roles. Use an empty Updated after value for a full sync.": "runtimeText.syncTheIamDirectoryProjectionBeforeAssigningOpenclarionRolesUseAnEmpty",
    "A directory manager must sync IAM users and departments before local RBAC can be assigned reliably.": "runtimeText.aDirectoryManagerMustSyncIamUsersAndDepartmentsBeforeLocalRbac",
    "An RBAC manager must create local role assignments before operators can use diagnosis-room permissions.": "runtimeText.anRbacManagerMustCreateLocalRoleAssignmentsBeforeOperatorsCanUse",
    "Create at least one enabled global or scoped assignment so signed-in operators can create, read, or participate in diagnosis rooms. Start by saving a bootstrap assignment for the current signed-in subject, then replace it with scoped team rules.": "runtimeText.createAtLeastOneEnabledAssignment",
} as const;

export function localizeDirectoryText(value: string, t: DirectoryTranslator): string {
  const key =
    directoryRuntimeTextKeys[value as keyof typeof directoryRuntimeTextKeys];
  if (key !== undefined) {
    return t(key);
  }
  let match = value.match(/^(Alert source|Department|Diagnosis room|Diagnosis tool template|Grouping policy|Notification channel|Report workflow|Report workflow schedule) \/ (.+)$/);
  if (match) {
    return t("runtimePattern.scopedValue", {
      key: match[2]!,
      kind: localizeDirectoryText(match[1]!, t),
    });
  }
  match = value.match(/^(.+) \(disabled\)$/);
  if (match) {
    return t("runtimePattern.disabledOption", { label: match[1]! });
  }
  return value;
}
