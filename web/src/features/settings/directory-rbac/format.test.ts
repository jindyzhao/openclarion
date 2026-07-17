import { describe, expect, it } from "vitest";

import {
  assignmentFormToWriteRequest,
  authorizeFormToRequest,
  bootstrapAdminAssignmentForm,
  diagnosisRoomScopeOptions,
  directoryRBACNextStepNotice,
  directoryDepartmentSubjectOptions,
  directorySubjectIsSystem,
  directorySubjectProfile,
  directorySyncProjectionSummary,
  directorySyncFormToRequest,
  directoryUserDepartmentKeysText,
  directoryUserSubjectIndex,
  directoryUserSubjectOptions,
  latestDirectorySyncAt,
  permissionLabel,
  rbacResourceScopeOptions,
  rbacPermissionOptions,
} from "./format";
import type { AlertSourceProfile } from "../alert-sources/types";
import type { DiagnosisToolTemplate } from "../diagnosis-tool-templates/types";
import type { GroupingPolicy } from "../grouping-policies/types";
import type { NotificationChannelProfile } from "../notification-channels/types";
import type { ReportWorkflowPolicy } from "../report-workflow-policies/types";
import type { ReportWorkflowSchedule } from "../report-workflow-schedules/types";
import type { DirectoryDepartment, DirectoryUser } from "./types";

describe("directory and rbac settings formatting", () => {
  it("normalizes directory sync requests", () => {
    expect(
      directorySyncFormToRequest({
        pageSize: 100,
        updatedAfter: "",
      }),
    ).toEqual({
      ok: true,
      value: { page_size: 100 },
    });

    expect(
      directorySyncFormToRequest({
        pageSize: 50,
        updatedAfter: "2026-06-26T08:00:00Z",
      }),
    ).toEqual({
      ok: true,
      value: {
        page_size: 50,
        updated_after: "2026-06-26T08:00:00.000Z",
      },
    });
  });

  it("rejects directory sync page sizes outside the OpenAPI bound", () => {
    expect(
      directorySyncFormToRequest({
        pageSize: 500,
        updatedAfter: "",
      }),
    ).toEqual({
      ok: true,
      value: { page_size: 500 },
    });

    expect(
      directorySyncFormToRequest({
        pageSize: 501,
        updatedAfter: "",
      }),
    ).toEqual({
      ok: false,
      message: "Directory sync page size must be between 1 and 500.",
    });
  });

  it("validates rbac assignment scope shape", () => {
    expect(
      assignmentFormToWriteRequest({
        enabled: true,
        role: "responder",
        scopeKey: "room-1",
        scopeKind: "diagnosis_room",
        subjectKey: "dep-2",
        subjectKind: "department",
      }),
    ).toEqual({
      ok: true,
      value: {
        enabled: true,
        role: "responder",
        scope_key: "room-1",
        scope_kind: "diagnosis_room",
        subject_key: "dep-2",
        subject_kind: "department",
      },
    });

    expect(
      assignmentFormToWriteRequest({
        enabled: true,
        role: "viewer",
        scopeKey: "ignored",
        scopeKind: "global",
        subjectKey: "iam-user-1",
        subjectKind: "user",
      }),
    ).toMatchObject({
      ok: true,
      value: { scope_key: "" },
    });
  });

  it("builds a bootstrap admin assignment for the current signed-in subject", () => {
    expect(bootstrapAdminAssignmentForm(" iam-user-1 ")).toEqual({
      ok: true,
      value: {
        enabled: true,
        role: "admin",
        scopeKey: "",
        scopeKind: "global",
        subjectKey: "iam-user-1",
        subjectKind: "user",
      },
    });
    expect(bootstrapAdminAssignmentForm(" ")).toEqual({
      ok: false,
      message: "Current signed-in subject is empty.",
    });
  });

  it("deduplicates authorization department keys", () => {
    expect(
      authorizeFormToRequest({
        departmentKeysText: "dep-1, dep-2\ndep-1",
        permission: "diagnosis_room.participate",
        scopeKey: "room-1",
        scopeKind: "diagnosis_room",
        subject: "iam-user-1",
      }),
    ).toEqual({
      ok: true,
      value: {
        department_keys: ["dep-1", "dep-2"],
        permission: "diagnosis_room.participate",
        scope_key: "room-1",
        scope_kind: "diagnosis_room",
        subject: "iam-user-1",
      },
    });
  });

  it("exposes operations read in the rbac permission picker", () => {
    expect(rbacPermissionOptions).toContainEqual({
      label: "Operations read",
      value: "operations.read",
    });
    expect(permissionLabel("operations.read")).toBe("Operations read");
  });

  it("builds directory-backed subject picker options", () => {
    const users: DirectoryUser[] = [
      directoryUserFixture({
        active: true,
        department_external_ids: ["dep-1", "dep-2"],
        display_name: "Alice",
        email: "alice@example.test",
        subject: "iam-user-1",
      }),
      directoryUserFixture({
        active: false,
        department_external_ids: ["dep-3"],
        display_name: "Bob",
        email: "bob@example.test",
        subject: "iam-user-2",
      }),
    ];
    const departments: DirectoryDepartment[] = [
      directoryDepartmentFixture({
        display_name: "SRE",
        external_id: "dep-2",
        path: "IT/Platform/SRE",
      }),
    ];

    expect(directoryUserSubjectOptions(users)).toMatchObject([
      {
        disabled: false,
        label: "Alice",
        value: "iam-user-1",
      },
      {
        disabled: true,
        label: "Bob",
        value: "iam-user-2",
      },
    ]);
    expect(directoryDepartmentSubjectOptions(departments)).toMatchObject([
      {
        label: "SRE",
        value: "dep-2",
      },
    ]);
    expect(directoryUserDepartmentKeysText(users, "iam-user-1")).toBe(
      "dep-1, dep-2",
    );
  });

  it("builds diagnosis room scope picker options from recent rooms", () => {
    expect(
      diagnosisRoomScopeOptions([
        {
          evidence_snapshot_id: 247,
          latest_conclusion: { status: "drafted" },
          room_status: "open",
          session_id: " diagnosis-session-1 ",
          task_status: "running",
        },
        {
          room_status: "closed",
          session_id: "diagnosis-session-1",
          task_status: "completed",
        },
        {
          room_status: "open",
          session_id: " ",
          task_status: "running",
        },
        {
          evidence_snapshot_id: 301,
          latest_conclusion: { status: "confirmed" },
          room_status: "closed",
          session_id: "diagnosis-session-2",
          task_status: "completed",
        },
      ]),
    ).toEqual([
      {
        label: "diagnosis-session-1",
        search: "diagnosis-session-1 open running 247 drafted",
        value: "diagnosis-session-1",
      },
      {
        label: "diagnosis-session-2",
        search: "diagnosis-session-2 closed completed 301 confirmed",
        value: "diagnosis-session-2",
      },
    ]);
  });

  it("builds resource-backed scope picker options without private fields", () => {
    const options = rbacResourceScopeOptions({
      alertSources: [
        alertSourceFixture({
          enabled: false,
          id: 1,
          labels: { env: "prod" },
          name: "Primary Prometheus",
        }),
      ],
      diagnosisRooms: [
        {
          room_status: "open",
          session_id: "diagnosis-session-1",
          task_status: "running",
        },
      ],
      diagnosisToolTemplates: [
        diagnosisToolTemplateFixture({
          id: 6,
          name: "CPU range query",
          query_template: "sum(secret_metric{token=\"never-index\"})",
        }),
      ],
      groupingPolicies: [
        groupingPolicyFixture({
          dimension_keys: ["alertname", "service"],
          id: 2,
          name: "Default grouping",
          source_filter: ["prometheus"],
        }),
      ],
      notificationChannels: [
        notificationChannelFixture({
          delivery_scopes: ["report", "diagnosis_consultation"],
          id: 5,
          labels: { team: "sre" },
          name: "Ops WeCom",
          secret_ref: "fixture-ref-wecom",
        }),
      ],
      workflowPolicies: [
        reportWorkflowPolicyFixture({
          diagnosis_follow_up: "auto_room",
          id: 3,
          name: "Default report workflow",
          report_scenario: "cascade",
        }),
      ],
      workflowSchedules: [
        reportWorkflowScheduleFixture({
          id: 4,
          name: "Daily report window",
          temporal_schedule_id: "openclarion-report-daily",
        }),
      ],
    });

    expect(options.alert_source).toEqual([
      {
        label: "#1 Primary Prometheus (disabled)",
        search: "1 Primary Prometheus disabled prometheus bearer env prod",
        value: "1",
      },
    ]);
    expect(options.grouping_policy?.[0]).toMatchObject({
      label: "#2 Default grouping",
      search: "2 Default grouping enabled alertname service severity prometheus",
      value: "2",
    });
    expect(options.report_workflow?.[0]).toMatchObject({
      label: "#3 Default report workflow",
      search: "3 Default report workflow enabled 1 2 manual_replay cascade auto_room",
      value: "3",
    });
    expect(options.report_workflow_schedule?.[0]).toMatchObject({
      label: "#4 Daily report window",
      search:
        "4 Daily report window enabled 3 openclarion-report-daily 86400 0 3600 300 100 600",
      value: "4",
    });
    expect(options.notification_channel?.[0]).toMatchObject({
      label: "#5 Ops WeCom",
      search: "5 Ops WeCom enabled wecom report diagnosis_consultation team sre",
      value: "5",
    });
    expect(options.notification_channel?.[0]?.search).not.toContain("secret/");
    expect(options.diagnosis_tool_template?.[0]).toMatchObject({
      label: "#6 CPU range query",
      search: "6 CPU range query enabled 1 metric_range_query 5 3600 21600 60",
      value: "6",
    });
    expect(options.diagnosis_tool_template?.[0]?.search).not.toContain(
      "never-index",
    );
    expect(options.diagnosis_room).toEqual([
      {
        label: "diagnosis-session-1",
        search: "diagnosis-session-1 open running",
        value: "diagnosis-session-1",
      },
    ]);
  });

  it("maps audit subjects to local directory profiles", () => {
    const users: DirectoryUser[] = [
      directoryUserFixture({
        active: true,
        department: "Platform",
        department_path: "IT/Platform",
        department_paths: ["IT/Platform", "IT/Shared"],
        display_name: "Alice",
        job_title: "SRE",
        section: "Operations",
        subject: "iam-user-1",
      }),
      directoryUserFixture({
        active: false,
        display_name: "",
        subject: "iam-user-2",
        username: "bob",
      }),
    ];
    const index = directoryUserSubjectIndex(users);

    expect(directorySubjectProfile(" iam-user-1 ", index)).toEqual({
      active: true,
      detailTags: [
        "iam-user-1",
        "IT/Platform",
        "IT/Shared",
        "Platform",
        "Operations",
        "SRE",
      ],
      displayName: "Alice",
      matchedDirectoryUser: true,
      subject: "iam-user-1",
    });
    expect(directorySubjectProfile("external-user", index)).toEqual({
      active: null,
      detailTags: [],
      displayName: "external-user",
      matchedDirectoryUser: false,
      subject: "external-user",
    });
    expect(directorySubjectProfile("iam-user-2", index)).toMatchObject({
      active: false,
      displayName: "bob",
      matchedDirectoryUser: true,
      subject: "iam-user-2",
    });
  });

  it("classifies OpenClarion service subjects", () => {
    expect(directorySubjectIsSystem("openclarion:auto-diagnosis")).toBe(true);
    expect(directorySubjectIsSystem("openclarion.alertmanager-webhook:1")).toBe(
      true,
    );
    expect(directorySubjectIsSystem("iam-user-1")).toBe(false);
  });

  it("finds the latest directory sync timestamp", () => {
    const users: DirectoryUser[] = [
      directoryUserFixture({
        synced_at: "2026-06-26T08:00:00Z",
      }),
    ];
    const departments: DirectoryDepartment[] = [
      directoryDepartmentFixture({
        synced_at: "2026-06-26T09:00:00Z",
      }),
    ];

    expect(latestDirectorySyncAt(users, departments)).toBe(
      "2026-06-26T09:00:00.000Z",
    );
  });

  it("summarizes an empty directory sync projection", () => {
    expect(
      directorySyncProjectionSummary([], [], new Date("2026-06-27T09:00:00Z")),
    ).toMatchObject({
      activeUsers: 0,
      departmentCount: 0,
      inactiveUsers: 0,
      latestSyncAt: null,
      providerCount: 0,
      providers: [],
      status: "empty",
      statusLabel: "Not synced",
      userCount: 0,
    });
  });

  it("summarizes current directory sync projection coverage", () => {
    const users: DirectoryUser[] = [
      directoryUserFixture({
        active: true,
        provider: "ops_iam",
        synced_at: "2026-06-26T08:00:00Z",
      }),
      directoryUserFixture({
        active: false,
        id: 2,
        provider: "ops_iam",
        subject: "iam-user-2",
        synced_at: "2026-06-26T08:30:00Z",
      }),
    ];
    const departments: DirectoryDepartment[] = [
      directoryDepartmentFixture({
        provider: "ops_iam",
        synced_at: "2026-06-26T09:00:00Z",
      }),
      directoryDepartmentFixture({
        external_id: "dep-2",
        id: 2,
        provider: "backup_iam",
        synced_at: "2026-06-26T08:45:00Z",
      }),
    ];

    expect(
      directorySyncProjectionSummary(
        users,
        departments,
        new Date("2026-06-26T12:00:00Z"),
      ),
    ).toMatchObject({
      activeUsers: 1,
      departmentCount: 2,
      inactiveUsers: 1,
      latestSyncAt: "2026-06-26T09:00:00.000Z",
      providerCount: 2,
      providers: ["backup_iam", "ops_iam"],
      status: "current",
      statusLabel: "Current",
      userCount: 2,
    });
  });

  it("marks directory sync projection as stale after one day", () => {
    expect(
      directorySyncProjectionSummary(
        [
          directoryUserFixture({
            synced_at: "2026-06-26T08:00:00Z",
          }),
        ],
        [],
        new Date("2026-06-27T08:00:01Z"),
      ),
    ).toMatchObject({
      latestSyncAt: "2026-06-26T08:00:00.000Z",
      status: "stale",
      statusLabel: "Stale",
    });
  });

  it("guides the directory to RBAC setup path", () => {
    const emptySummary = directorySyncProjectionSummary(
      [],
      [],
      new Date("2026-06-27T09:00:00Z"),
    );
    expect(
      directoryRBACNextStepNotice({
        canManageDirectory: true,
        canManageRBAC: true,
        enabledAssignments: 0,
        summary: emptySummary,
      }),
    ).toEqual({
      action: "sync_directory",
      actionLabel: "Run full sync",
      detail:
        "Sync the IAM directory projection before assigning OpenClarion roles. Use an empty Updated after value for a full sync.",
      message: "Directory projection is required",
      status: "blocked",
    });
    expect(
      directoryRBACNextStepNotice({
        canManageDirectory: false,
        canManageRBAC: false,
        enabledAssignments: 0,
        summary: emptySummary,
      }),
    ).toMatchObject({
      action: "none",
      actionLabel: "Need directory manager",
      status: "blocked",
    });

    const syncedSummary = directorySyncProjectionSummary(
      [directoryUserFixture({ subject: "iam-user-1" })],
      [directoryDepartmentFixture({ external_id: "dep-1" })],
      new Date("2026-06-27T09:00:00Z"),
    );
    expect(
      directoryRBACNextStepNotice({
        canManageDirectory: true,
        canManageRBAC: true,
        enabledAssignments: 0,
        summary: syncedSummary,
      }),
    ).toMatchObject({
      action: "prepare_assignment",
      actionLabel: "Create assignment",
      status: "review",
    });
    expect(
      directoryRBACNextStepNotice({
        canManageDirectory: true,
        canManageRBAC: false,
        enabledAssignments: 0,
        summary: syncedSummary,
      }),
    ).toMatchObject({
      action: "none",
      actionLabel: "Need RBAC manager",
      status: "review",
    });
    expect(
      directoryRBACNextStepNotice({
        canManageDirectory: true,
        canManageRBAC: true,
        enabledAssignments: 2,
        summary: syncedSummary,
      }),
    ).toMatchObject({
      action: "open_diagnosis_room",
      actionLabel: "Open diagnosis room",
      status: "ready",
    });
  });
});

function directoryUserFixture(
  overrides: Partial<DirectoryUser> = {},
): DirectoryUser {
  return {
    active: true,
    created_at: "2026-06-26T07:00:00Z",
    department: "Platform",
    department_external_ids: ["dep-1"],
    department_path: "IT/Platform",
    department_paths: ["IT/Platform"],
    display_name: "Alice",
    email: "alice@example.test",
    external_id: "wecom-user-1",
    id: 1,
    job_title: "SRE",
    provider: "ops_iam",
    section: "SRE",
    source_updated_at: "2026-06-26T07:00:00Z",
    subject: "iam-user-1",
    synced_at: "2026-06-26T08:00:00Z",
    updated_at: "2026-06-26T08:00:00Z",
    username: "alice",
    ...overrides,
  };
}

function directoryDepartmentFixture(
  overrides: Partial<DirectoryDepartment> = {},
): DirectoryDepartment {
  return {
    created_at: "2026-06-26T07:30:00Z",
    display_name: "SRE",
    external_id: "dep-1",
    id: 1,
    level: 2,
    member_count: 1,
    name: "SRE",
    parent_external_id: "",
    parent_path: "IT",
    path: "IT/SRE",
    provider: "ops_iam",
    source: "iam",
    source_updated_at: "2026-06-26T07:30:00Z",
    synced_at: "2026-06-26T09:00:00Z",
    updated_at: "2026-06-26T09:00:00Z",
    ...overrides,
  };
}

function alertSourceFixture(
  overrides: Partial<AlertSourceProfile> = {},
): AlertSourceProfile {
  return {
    auth_mode: "bearer",
    base_url: "https://prometheus.example.test",
    created_at: "2026-06-05T08:00:00Z",
    enabled: true,
    id: 1,
    kind: "prometheus",
    labels: {},
    name: "Primary Prometheus",
    secret_ref: "secret/example/prometheus",
    updated_at: "2026-06-05T08:10:00Z",
    ...overrides,
  };
}

function groupingPolicyFixture(
  overrides: Partial<GroupingPolicy> = {},
): GroupingPolicy {
  return {
    created_at: "2026-06-05T08:00:00Z",
    dimension_keys: ["alertname"],
    enabled: true,
    id: 2,
    name: "Default grouping",
    severity_key: "severity",
    source_filter: [],
    updated_at: "2026-06-05T08:10:00Z",
    ...overrides,
  };
}

function reportWorkflowPolicyFixture(
  overrides: Partial<ReportWorkflowPolicy> = {},
): ReportWorkflowPolicy {
  return {
    alert_source_profile_id: 1,
    created_at: "2026-06-05T08:00:00Z",
    diagnosis_follow_up: "suggest_room",
    disabled_at: null,
    enabled: true,
    enabled_at: "2026-06-05T08:05:00Z",
    grouping_policy_id: 2,
    id: 3,
    max_failed_sub_reports: 0,
    name: "Default report workflow",
    report_notification_channel_profile_id: null,
    report_scenario: "single_alert",
    trigger_mode: "manual_replay",
    updated_at: "2026-06-05T08:10:00Z",
    ...overrides,
  };
}

function reportWorkflowScheduleFixture(
  overrides: Partial<ReportWorkflowSchedule> = {},
): ReportWorkflowSchedule {
  return {
    cadence: "interval",
    calendar_day_of_month: 0,
    calendar_day_of_week: 0,
    calendar_hour: 0,
    calendar_minute: 0,
    catchup_window_seconds: 600,
    created_at: "2026-06-06T02:00:00Z",
    disabled_at: null,
    enabled: true,
    enabled_at: "2026-06-06T02:05:00Z",
    id: 4,
    interval_seconds: 86400,
    name: "Daily report window",
    offset_seconds: 0,
    replay_delay_seconds: 300,
    replay_limit: 100,
    replay_window_seconds: 3600,
    report_workflow_policy_id: 3,
    temporal_schedule_id: "openclarion-report-daily",
    updated_at: "2026-06-06T02:10:00Z",
    ...overrides,
  };
}

function notificationChannelFixture(
  overrides: Partial<NotificationChannelProfile> = {},
): NotificationChannelProfile {
  return {
    created_at: "2026-06-05T09:00:00Z",
    delivery_scopes: ["report"],
    enabled: true,
    id: 5,
    kind: "wecom",
    labels: {},
    latest_test_results: [],
    name: "Ops WeCom",
    secret_ref: "secret/example/wecom",
    updated_at: "2026-06-05T09:10:00Z",
    ...overrides,
  };
}

function diagnosisToolTemplateFixture(
  overrides: Partial<DiagnosisToolTemplate> = {},
): DiagnosisToolTemplate {
  return {
    alert_source_profile_id: 1,
    created_at: "2026-06-08T08:00:00Z",
    default_limit: 5,
    default_step_seconds: 60,
    default_window_seconds: 3600,
    disabled_at: null,
    enabled: true,
    enabled_at: "2026-06-08T08:05:00Z",
    id: 6,
    max_window_seconds: 21600,
    name: "CPU range query",
    query_template: "rate(container_cpu_usage_seconds_total[5m])",
    tool: "metric_range_query",
    updated_at: "2026-06-08T08:10:00Z",
    ...overrides,
  };
}
