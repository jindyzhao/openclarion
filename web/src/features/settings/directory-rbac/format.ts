import type { AlertSourceProfile } from "../alert-sources/types";
import type { DiagnosisToolTemplate } from "../diagnosis-tool-templates/types";
import type { GroupingPolicy } from "../grouping-policies/types";
import type { NotificationChannelProfile } from "../notification-channels/types";
import type { ReportWorkflowPolicy } from "../report-workflow-policies/types";
import type { ReportWorkflowSchedule } from "../report-workflow-schedules/types";
import type {
  DirectoryDepartment,
  DirectorySyncFormState,
  DirectorySyncRequest,
  DirectoryUser,
  RBACAssignment,
  RBACAssignmentFormState,
  RBACAssignmentWriteRequest,
  RBACAuthorizeFormState,
  RBACAuthorizeRequest,
  RBACPermission,
  RBACRole,
  RBACScopeKind,
  RBACSubjectKind,
} from "./types";

type ParseResult<T> =
  | { ok: true; value: T }
  | { message: string; ok: false };

export type DirectorySelectOption = {
  disabled?: boolean;
  label: string;
  search: string;
  value: string;
};

type DirectorySyncProjectionStatus = "current" | "empty" | "stale";

export type DirectorySyncProjectionSummary = {
  activeUsers: number;
  departmentCount: number;
  inactiveUsers: number;
  latestSyncAt: string | null;
  providerCount: number;
  providers: string[];
  status: DirectorySyncProjectionStatus;
  statusLabel: string;
  userCount: number;
};

export type DirectoryRBACNextStepNotice = {
  action: "none" | "open_diagnosis_room" | "prepare_assignment" | "sync_directory";
  actionLabel: string;
  detail: string;
  message: string;
  status: "ready" | "review" | "blocked";
};

export type DirectorySubjectProfile = {
  active: boolean | null;
  detailTags: string[];
  displayName: string;
  matchedDirectoryUser: boolean;
  subject: string;
};

export type DiagnosisRoomScopeOptionInput = {
  evidence_snapshot_id?: number;
  latest_conclusion?: {
    status?: string;
  };
  room_status?: string;
  session_id: string;
  task_status?: string;
};

export type RBACResourceScopeOptionInput = {
  alertSources: readonly AlertSourceProfile[];
  diagnosisRooms: readonly DiagnosisRoomScopeOptionInput[];
  diagnosisToolTemplates: readonly DiagnosisToolTemplate[];
  groupingPolicies: readonly GroupingPolicy[];
  notificationChannels: readonly NotificationChannelProfile[];
  workflowPolicies: readonly ReportWorkflowPolicy[];
  workflowSchedules: readonly ReportWorkflowSchedule[];
};

const directorySyncStaleAfterMs = 24 * 60 * 60 * 1000;

export const rbacSubjectKindOptions: Array<{
  label: string;
  value: RBACSubjectKind;
}> = [
  { label: "User", value: "user" },
  { label: "Department", value: "department" },
];

export const rbacRoleOptions: Array<{ label: string; value: RBACRole }> = [
  { label: "Admin", value: "admin" },
  { label: "Operator", value: "operator" },
  { label: "Responder", value: "responder" },
  { label: "Viewer", value: "viewer" },
];

export const rbacScopeKindOptions: Array<{
  label: string;
  value: RBACScopeKind;
}> = [
  { label: "Global", value: "global" },
  { label: "Diagnosis room", value: "diagnosis_room" },
  { label: "Alert source", value: "alert_source" },
  { label: "Grouping policy", value: "grouping_policy" },
  { label: "Report workflow", value: "report_workflow" },
  { label: "Report workflow schedule", value: "report_workflow_schedule" },
  { label: "Notification channel", value: "notification_channel" },
  { label: "Diagnosis tool template", value: "diagnosis_tool_template" },
];

export const rbacPermissionOptions: Array<{
  label: string;
  value: RBACPermission;
}> = [
  { label: "Directory read", value: "directory.read" },
  { label: "Directory manage", value: "directory.manage" },
  { label: "RBAC manage", value: "rbac.manage" },
  { label: "Operations read", value: "operations.read" },
  { label: "Diagnosis room read", value: "diagnosis_room.read" },
  { label: "Diagnosis room participate", value: "diagnosis_room.participate" },
  { label: "Diagnosis room administer", value: "diagnosis_room.administer" },
  { label: "Alert source read", value: "alert_source.read" },
  { label: "Alert source manage", value: "alert_source.manage" },
  { label: "Grouping policy read", value: "grouping_policy.read" },
  { label: "Grouping policy manage", value: "grouping_policy.manage" },
  { label: "Report workflow read", value: "report_workflow.read" },
  { label: "Report workflow manage", value: "report_workflow.manage" },
  { label: "Notification channel read", value: "notification_channel.read" },
  { label: "Notification channel manage", value: "notification_channel.manage" },
  { label: "Notification channel test", value: "notification_channel.test" },
  {
    label: "Diagnosis tool template read",
    value: "diagnosis_tool_template.read",
  },
  {
    label: "Diagnosis tool template manage",
    value: "diagnosis_tool_template.manage",
  },
];

export function emptyDirectorySyncForm(): DirectorySyncFormState {
  return {
    pageSize: 100,
    updatedAfter: "",
  };
}

export function emptyRBACAssignmentForm(): RBACAssignmentFormState {
  return {
    enabled: true,
    role: "responder",
    scopeKey: "",
    scopeKind: "diagnosis_room",
    subjectKey: "",
    subjectKind: "department",
  };
}

export function bootstrapAdminAssignmentForm(
  subject: string,
): ParseResult<RBACAssignmentFormState> {
  const subjectKey = subject.trim();
  if (subjectKey === "") {
    return { ok: false, message: "Current signed-in subject is empty." };
  }
  return {
    ok: true,
    value: {
      enabled: true,
      role: "admin",
      scopeKey: "",
      scopeKind: "global",
      subjectKey,
      subjectKind: "user",
    },
  };
}

export function emptyRBACAuthorizeForm(): RBACAuthorizeFormState {
  return {
    departmentKeysText: "",
    permission: "diagnosis_room.participate",
    scopeKey: "",
    scopeKind: "diagnosis_room",
    subject: "",
  };
}

export function directorySyncFormToRequest(
  values: DirectorySyncFormState,
): ParseResult<DirectorySyncRequest> {
  const pageSize = Number(values.pageSize);
  if (!Number.isSafeInteger(pageSize) || pageSize < 1 || pageSize > 500) {
    return {
      ok: false,
      message: "Directory sync page size must be between 1 and 500.",
    };
  }
  const updatedAfter = values.updatedAfter.trim();
  if (updatedAfter === "") {
    return { ok: true, value: { page_size: pageSize } };
  }
  const parsedDate = new Date(updatedAfter);
  if (Number.isNaN(parsedDate.getTime())) {
    return {
      ok: false,
      message: "Updated-after timestamp must be a valid date-time value.",
    };
  }
  return {
    ok: true,
    value: {
      page_size: pageSize,
      updated_after: parsedDate.toISOString(),
    },
  };
}

export function assignmentFormToWriteRequest(
  values: RBACAssignmentFormState,
): ParseResult<RBACAssignmentWriteRequest> {
  const subjectKey = values.subjectKey.trim();
  if (subjectKey === "") {
    return { ok: false, message: "Subject key is required." };
  }
  const scopeKey = normalizeScopeKey(values.scopeKind, values.scopeKey);
  if (!scopeKey.ok) {
    return scopeKey;
  }
  return {
    ok: true,
    value: {
      enabled: Boolean(values.enabled),
      role: values.role,
      scope_key: scopeKey.value,
      scope_kind: values.scopeKind,
      subject_key: subjectKey,
      subject_kind: values.subjectKind,
    },
  };
}

export function authorizeFormToRequest(
  values: RBACAuthorizeFormState,
): ParseResult<RBACAuthorizeRequest> {
  const subject = values.subject.trim();
  if (subject === "") {
    return { ok: false, message: "Subject is required." };
  }
  const scopeKey = normalizeScopeKey(values.scopeKind, values.scopeKey);
  if (!scopeKey.ok) {
    return scopeKey;
  }
  return {
    ok: true,
    value: {
      department_keys: splitDepartmentKeys(values.departmentKeysText),
      permission: values.permission,
      scope_key: scopeKey.value,
      scope_kind: values.scopeKind,
      subject,
    },
  };
}

function scopeKindLabel(kind: RBACScopeKind): string {
  return (
    rbacScopeKindOptions.find((option) => option.value === kind)?.label ?? kind
  );
}

export function roleLabel(role: RBACRole): string {
  return rbacRoleOptions.find((option) => option.value === role)?.label ?? role;
}

export function permissionLabel(permission: RBACPermission): string {
  return (
    rbacPermissionOptions.find((option) => option.value === permission)
      ?.label ?? permission
  );
}

export function scopeLabel(kind: RBACScopeKind, key: string): string {
  if (kind === "global") {
    return "Global";
  }
  return `${scopeKindLabel(kind)} / ${key}`;
}

export function assignmentScopeLabel(assignment: RBACAssignment): string {
  return scopeLabel(assignment.scope_kind, assignment.scope_key);
}

export function subjectKindLabel(kind: RBACSubjectKind): string {
  return (
    rbacSubjectKindOptions.find((option) => option.value === kind)?.label ??
    kind
  );
}

export function directoryUserLabel(user: DirectoryUser): string {
  return firstNonEmpty(user.display_name, user.username, user.email, user.subject);
}

export function directoryDepartmentLabel(
  department: DirectoryDepartment,
): string {
  return firstNonEmpty(
    department.display_name,
    department.name,
    department.path,
    department.external_id,
  );
}

export function latestDirectorySyncAt(
  users: DirectoryUser[],
  departments: DirectoryDepartment[],
): string | null {
  const timestamps = [...users, ...departments]
    .map((item) => Date.parse(item.synced_at))
    .filter((value) => Number.isFinite(value));
  if (timestamps.length === 0) {
    return null;
  }
  return new Date(Math.max(...timestamps)).toISOString();
}

export function directorySyncProjectionSummary(
  users: DirectoryUser[],
  departments: DirectoryDepartment[],
  now: Date = new Date(),
): DirectorySyncProjectionSummary {
  const latestSyncAt = latestDirectorySyncAt(users, departments);
  const providerSet = new Set<string>();
  for (const item of [...users, ...departments]) {
    const provider = item.provider.trim();
    if (provider !== "") {
      providerSet.add(provider);
    }
  }
  const activeUsers = users.filter((user) => user.active).length;
  const status = directorySyncProjectionStatus(latestSyncAt, now);
  return {
    activeUsers,
    departmentCount: departments.length,
    inactiveUsers: users.length - activeUsers,
    latestSyncAt,
    providerCount: providerSet.size,
    providers: Array.from(providerSet).sort((left, right) =>
      left.localeCompare(right),
    ),
    status,
    statusLabel: directorySyncProjectionStatusLabel(status),
    userCount: users.length,
  };
}

export function directoryRBACNextStepNotice({
  canManageDirectory,
  canManageRBAC,
  enabledAssignments,
  summary,
}: {
  canManageDirectory: boolean;
  canManageRBAC: boolean;
  enabledAssignments: number;
  summary: DirectorySyncProjectionSummary;
}): DirectoryRBACNextStepNotice {
  if (summary.userCount === 0 || summary.departmentCount === 0) {
    return {
      action: canManageDirectory ? "sync_directory" : "none",
      actionLabel: canManageDirectory ? "Run full sync" : "Need directory manager",
      detail: canManageDirectory
        ? "Sync the IAM directory projection before assigning OpenClarion roles. Use an empty Updated after value for a full sync."
        : "A directory manager must sync IAM users and departments before local RBAC can be assigned reliably.",
      message: "Directory projection is required",
      status: "blocked",
    };
  }
  if (enabledAssignments === 0) {
    return {
      action: canManageRBAC ? "prepare_assignment" : "none",
      actionLabel: canManageRBAC ? "Create assignment" : "Need RBAC manager",
      detail: canManageRBAC
        ? "Create at least one enabled global or scoped assignment so signed-in operators can create, read, or participate in diagnosis rooms. Start by saving a bootstrap assignment for the current signed-in subject, then replace it with scoped team rules."
        : "An RBAC manager must create local role assignments before operators can use diagnosis-room permissions.",
      message: "Local RBAC assignments are required",
      status: "review",
    };
  }
  return {
    action: "open_diagnosis_room",
    actionLabel: "Open diagnosis room",
    detail:
      "Directory projection and enabled RBAC assignments are present. Use Authorization Preview for a specific subject, permission, and scope before manual diagnosis-room testing.",
    message: "Access control is ready for manual checks",
    status: "ready",
  };
}

export function scopeRequiresKey(scopeKind: RBACScopeKind): boolean {
  return scopeKind !== "global";
}

export function directoryUserSubjectOptions(
  users: DirectoryUser[],
): DirectorySelectOption[] {
  return users.map((user) => {
    const label = directoryUserLabel(user);
    return {
      disabled: !user.active,
      label: label === "" ? user.subject : label,
      search: [
        label,
        user.subject,
        user.username,
        user.email,
        user.department,
        user.department_path,
        ...user.department_paths,
        ...user.department_external_ids,
      ].join(" "),
      value: user.subject,
    };
  });
}

export function directoryDepartmentSubjectOptions(
  departments: DirectoryDepartment[],
): DirectorySelectOption[] {
  return departments.map((department) => {
    const label = directoryDepartmentLabel(department);
    return {
      label: label === "" ? department.external_id : label,
      search: [
        label,
        department.external_id,
        department.name,
        department.display_name,
        department.path,
        department.parent_path,
      ].join(" "),
      value: department.external_id,
    };
  });
}

export function diagnosisRoomScopeOptions(
  rooms: readonly DiagnosisRoomScopeOptionInput[],
): DirectorySelectOption[] {
  const seen = new Set<string>();
  const options: DirectorySelectOption[] = [];
  for (const room of rooms) {
    const sessionID = room.session_id.trim();
    if (sessionID === "" || seen.has(sessionID)) {
      continue;
    }
    seen.add(sessionID);
    options.push({
      label: sessionID,
      search: uniqueNonEmpty([
        sessionID,
        room.room_status ?? "",
        room.task_status ?? "",
        room.evidence_snapshot_id === undefined
          ? ""
          : String(room.evidence_snapshot_id),
        room.latest_conclusion?.status ?? "",
      ]).join(" "),
      value: sessionID,
    });
  }
  return options;
}

export function rbacResourceScopeOptions(
  input: RBACResourceScopeOptionInput,
): Partial<Record<RBACScopeKind, DirectorySelectOption[]>> {
  return {
    alert_source: input.alertSources.map((source) =>
      numericScopeOption({
        enabled: source.enabled,
        id: source.id,
        name: source.name,
        searchParts: [
          source.kind,
          source.auth_mode,
          ...labelSearchParts(source.labels),
        ],
      }),
    ),
    diagnosis_room: diagnosisRoomScopeOptions(input.diagnosisRooms),
    diagnosis_tool_template: input.diagnosisToolTemplates.map((template) =>
      numericScopeOption({
        enabled: template.enabled,
        id: template.id,
        name: template.name,
        searchParts: [
          String(template.alert_source_profile_id),
          template.tool,
          String(template.default_limit),
          String(template.default_window_seconds),
          String(template.max_window_seconds),
          String(template.default_step_seconds),
        ],
      }),
    ),
    grouping_policy: input.groupingPolicies.map((policy) =>
      numericScopeOption({
        enabled: policy.enabled,
        id: policy.id,
        name: policy.name,
        searchParts: [
          ...policy.dimension_keys,
          policy.severity_key,
          ...policy.source_filter,
        ],
      }),
    ),
    notification_channel: input.notificationChannels.map((channel) =>
      numericScopeOption({
        enabled: channel.enabled,
        id: channel.id,
        name: channel.name,
        searchParts: [
          channel.kind,
          ...channel.delivery_scopes,
          ...labelSearchParts(channel.labels),
        ],
      }),
    ),
    report_workflow: input.workflowPolicies.map((policy) =>
      numericScopeOption({
        enabled: policy.enabled,
        id: policy.id,
        name: policy.name,
        searchParts: [
          String(policy.alert_source_profile_id),
          String(policy.grouping_policy_id),
          policy.report_notification_channel_profile_id === null
            ? ""
            : String(policy.report_notification_channel_profile_id),
          policy.trigger_mode,
          policy.report_scenario,
          policy.diagnosis_follow_up,
        ],
      }),
    ),
    report_workflow_schedule: input.workflowSchedules.map((schedule) =>
      numericScopeOption({
        enabled: schedule.enabled,
        id: schedule.id,
        name: schedule.name,
        searchParts: [
          String(schedule.report_workflow_policy_id),
          schedule.temporal_schedule_id,
          String(schedule.interval_seconds),
          String(schedule.offset_seconds),
          String(schedule.replay_window_seconds),
          String(schedule.replay_delay_seconds),
          String(schedule.replay_limit),
          String(schedule.catchup_window_seconds),
        ],
      }),
    ),
  };
}

export function directoryUserDepartmentKeysText(
  users: DirectoryUser[],
  subject: string,
): string {
  const normalizedSubject = subject.trim();
  if (normalizedSubject === "") {
    return "";
  }
  const user = users.find((item) => item.subject === normalizedSubject);
  if (user === undefined) {
    return "";
  }
  return user.department_external_ids
    .map((key) => key.trim())
    .filter((key, index, keys) => key !== "" && keys.indexOf(key) === index)
    .join(", ");
}

export function directoryUserSubjectIndex(
  users: DirectoryUser[],
): ReadonlyMap<string, DirectoryUser> {
  const index = new Map<string, DirectoryUser>();
  for (const user of users) {
    const subject = user.subject.trim();
    if (subject === "" || index.has(subject)) {
      continue;
    }
    index.set(subject, user);
  }
  return index;
}

export function directorySubjectProfile(
  subject: string,
  usersBySubject: ReadonlyMap<string, DirectoryUser>,
): DirectorySubjectProfile {
  const normalizedSubject = subject.trim();
  const user = usersBySubject.get(normalizedSubject);
  if (user === undefined) {
    return {
      active: null,
      detailTags: [],
      displayName: normalizedSubject,
      matchedDirectoryUser: false,
      subject: normalizedSubject,
    };
  }
  const displayName = directoryUserLabel(user);
  return {
    active: user.active,
    detailTags: uniqueNonEmpty([
      displayName === normalizedSubject ? "" : normalizedSubject,
      user.department_path,
      ...user.department_paths,
      user.department,
      user.section,
      user.job_title,
    ]),
    displayName: displayName || normalizedSubject,
    matchedDirectoryUser: true,
    subject: normalizedSubject,
  };
}

export function directorySubjectIsSystem(subject: string): boolean {
  const normalizedSubject = subject.trim();
  return (
    normalizedSubject === "openclarion:auto-diagnosis" ||
    normalizedSubject.startsWith("openclarion.") ||
    normalizedSubject.startsWith("openclarion:")
  );
}

function normalizeScopeKey(
  scopeKind: RBACScopeKind,
  rawScopeKey: string,
): ParseResult<string> {
  const scopeKey = rawScopeKey.trim();
  if (scopeKind === "global") {
    return { ok: true, value: "" };
  }
  if (scopeKey === "") {
    return { ok: false, message: "Scope key is required." };
  }
  return { ok: true, value: scopeKey };
}

function directorySyncProjectionStatus(
  latestSyncAt: string | null,
  now: Date,
): DirectorySyncProjectionStatus {
  if (latestSyncAt === null) {
    return "empty";
  }
  const latestTime = Date.parse(latestSyncAt);
  const nowTime = now.getTime();
  if (
    Number.isFinite(latestTime) &&
    Number.isFinite(nowTime) &&
    nowTime - latestTime > directorySyncStaleAfterMs
  ) {
    return "stale";
  }
  return "current";
}

function directorySyncProjectionStatusLabel(
  status: DirectorySyncProjectionStatus,
): string {
  switch (status) {
    case "current":
      return "Current";
    case "empty":
      return "Not synced";
    case "stale":
      return "Stale";
  }
}

function splitDepartmentKeys(value: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of value.split(/[\n,]+/)) {
    const key = raw.trim();
    if (key === "" || seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(key);
  }
  return out;
}

function firstNonEmpty(...values: string[]): string {
  for (const value of values) {
    const trimmed = value.trim();
    if (trimmed !== "") {
      return trimmed;
    }
  }
  return "";
}

function numericScopeOption({
  enabled,
  id,
  name,
  searchParts,
}: {
  enabled?: boolean;
  id: number;
  name: string;
  searchParts: string[];
}): DirectorySelectOption {
  const value = String(id);
  const displayName = firstNonEmpty(name, value);
  const state = enabled === undefined ? "" : enabled ? "enabled" : "disabled";
  return {
    label: `#${value} ${displayName}${enabled === false ? " (disabled)" : ""}`,
    search: uniqueNonEmpty([value, displayName, state, ...searchParts]).join(" "),
    value,
  };
}

function labelSearchParts(labels: Record<string, string> | undefined): string[] {
  if (labels === undefined) {
    return [];
  }
  return Object.entries(labels).flatMap(([key, value]) => [key, value]);
}

export function uniqueNonEmpty(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (trimmed === "" || seen.has(trimmed)) {
      continue;
    }
    seen.add(trimmed);
    out.push(trimmed);
  }
  return out;
}
