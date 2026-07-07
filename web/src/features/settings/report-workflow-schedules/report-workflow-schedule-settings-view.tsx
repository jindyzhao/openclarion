"use client";

import {
  CalendarOutlined,
  EditOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  Form,
  Input,
  InputNumber,
  Row,
  Select,
  Segmented,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography
} from "antd";
import type { DescriptionsProps, TableColumnsType } from "antd";
import { useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

import { formatDateTime, formatDurationSeconds } from "../format";
import {
  settingsErrorMessage,
  settingsManagePermissionNotice,
  settingsReadPermissionEmptyDescription,
  settingsReadPermissionNotice,
  type SettingsNotice,
  useClientReady,
  useSettingsList,
  useSettingsMutation
} from "../query-state";
import { ReadOnlyModeAlert } from "../permission-notice";
import {
  useCurrentRBACAuthorizations,
  type CurrentRBACAuthorizationCheck
} from "../rbac-capabilities";
import {
  reportWorkflowScheduleManageKey,
  reportWorkflowSchedulePolicyAuthorizationChecks,
  reportWorkflowSchedulePolicyPermissionBlockReason
} from "./rbac-gates";
import type {
  ReportWorkflowPolicy,
  ReportWorkflowPolicyListResponse
} from "../report-workflow-policies/types";
import {
  disableReportWorkflowScheduleAction,
  enableReportWorkflowScheduleAction,
  refreshReportWorkflowSchedules,
  submitReportWorkflowSchedule
} from "./client-api";
import {
  emptyReportWorkflowScheduleForm,
  formStateToWriteRequest,
  reportWorkflowScheduleCadenceDefaults,
  reportWorkflowScheduleCadenceDetail,
  reportWorkflowScheduleCadenceLabel,
  reportWorkflowScheduleCadenceValue,
  reportWorkflowScheduleDraftReadiness,
  reportWorkflowScheduleEnablementReadiness,
  reportWorkflowScheduleFormCadenceValue,
  reportWorkflowScheduleProofOutcome,
  reportWorkflowScheduleReplayWindowBlocker,
  scheduleToFormState,
  type ReportWorkflowScheduleDraftReadiness,
  type ReportWorkflowScheduleLaunchIntent
} from "./format";
import type {
  ReportWorkflowSchedule,
  ReportWorkflowScheduleFormState,
  ReportWorkflowScheduleListResponse,
  ReportWorkflowScheduleWriteRequest
} from "./types";

type ReportWorkflowScheduleSettingsManagerProps = {
  launchIntent?: ReportWorkflowScheduleLaunchIntent | null;
  reportWorkflowPoliciesResult: ApiResult<ReportWorkflowPolicyListResponse>;
  result: ApiResult<ReportWorkflowScheduleListResponse>;
};

const reportWorkflowSchedulesQueryKey = ["settings", "report-workflow-schedules"] as const;

const cadenceOptions = [
  { label: "Interval", value: "interval" },
  { label: "Daily", value: "daily" },
  { label: "Weekly", value: "weekly" },
  { label: "Monthly", value: "monthly" }
];

const dayOfWeekOptions = [
  { label: "Sunday", value: 0 },
  { label: "Monday", value: 1 },
  { label: "Tuesday", value: 2 },
  { label: "Wednesday", value: 3 },
  { label: "Thursday", value: 4 },
  { label: "Friday", value: 5 },
  { label: "Saturday", value: 6 }
];

const dayOfMonthOptions = Array.from({ length: 28 }, (_, index) => ({
  label: String(index + 1),
  value: index + 1
}));

type SaveScheduleVariables = {
  body: ReportWorkflowScheduleWriteRequest;
  scheduleID: number | null;
};

const reportWorkflowScheduleBaseAuthorizationChecks: CurrentRBACAuthorizationCheck[] = [
  { key: "reportWorkflowRead", permission: "report_workflow.read" },
  { key: "reportWorkflowManage", permission: "report_workflow.manage" }
];

type EnablementVariables = {
  enabled: boolean;
  scheduleID: number;
};

type RelationSelectOption = {
  label: string;
  title: string;
  value: number;
};

type ScheduleRelationOptions = {
  policyEnabledIDs: ReadonlySet<number>;
  policyLabels: Record<number, string>;
  policyOptions: RelationSelectOption[];
  warnings: string[];
};

export function ReportWorkflowScheduleSettingsManager({
  launchIntent = null,
  reportWorkflowPoliciesResult,
  result
}: ReportWorkflowScheduleSettingsManagerProps) {
  const [form] = Form.useForm<ReportWorkflowScheduleFormState>();
  const clientReady = useClientReady();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [actionID, setActionID] = useState<number | null>(null);
  const [launchNotice, setLaunchNotice] = useState<string | null>(launchIntent?.message ?? null);
  const {
    errorStatus,
    items: schedules,
    notice,
    query,
    refresh,
    setNotice
  } = useSettingsList({
    initialResult: result,
    queryKey: reportWorkflowSchedulesQueryKey,
    queryFn: refreshReportWorkflowSchedules,
    refreshMessage: "Schedules refreshed.",
    selectItems: (response) => response.items
  });
  const saveSchedule = useSettingsMutation<SaveScheduleVariables, ReportWorkflowSchedule>({
    invalidateQueryKey: reportWorkflowSchedulesQueryKey,
    mutationFn: ({ scheduleID, body }) => submitReportWorkflowSchedule(scheduleID, body)
  });
  const enablementAction = useSettingsMutation<EnablementVariables, ReportWorkflowSchedule>({
    invalidateQueryKey: reportWorkflowSchedulesQueryKey,
    mutationFn: ({ scheduleID, enabled }) =>
      enabled ? enableReportWorkflowScheduleAction(scheduleID) : disableReportWorkflowScheduleAction(scheduleID)
  });
  const authorizationChecks = useMemo(
    () => [
      ...reportWorkflowScheduleBaseAuthorizationChecks,
      ...schedules.map((schedule) => ({
        key: reportWorkflowScheduleManageKey(schedule.id),
        permission: "report_workflow.manage" as const,
        scopeKey: String(schedule.id),
        scopeKind: "report_workflow_schedule" as const
      })),
      ...reportWorkflowSchedulePolicyAuthorizationChecks(
        reportWorkflowPoliciesResult.ok
          ? reportWorkflowPoliciesResult.data.items.map((policy) => policy.id)
          : []
      )
    ],
    [reportWorkflowPoliciesResult, schedules]
  );
  const currentAuthorization = useCurrentRBACAuthorizations(authorizationChecks, clientReady);
  const busy =
    !clientReady ||
    currentAuthorization.isChecking ||
    query.isFetching ||
    saveSchedule.isPending ||
    enablementAction.isPending;
  const canReadSchedules = currentAuthorization.can("reportWorkflowRead");
  const canCreateSchedule = currentAuthorization.can("reportWorkflowManage");
  const selectedReportWorkflowPolicyID = Form.useWatch("reportWorkflowPolicyID", form) ?? null;
  const authorizationChecking = !clientReady || currentAuthorization.isChecking;
  const currentScheduleManagePermissionBlockReason = authorizationChecking
    ? ""
    : editingID === null || currentAuthorization.can(reportWorkflowScheduleManageKey(editingID))
      ? ""
      : `Current user is not authorized to manage report workflow schedule #${editingID}.`;
  const schedulePolicyPermissionBlockReason =
    authorizationChecking
      ? ""
      : reportWorkflowSchedulePolicyPermissionBlockReason({
          can: currentAuthorization.can,
          editingScheduleID: editingID,
          reportWorkflowPolicyID: selectedReportWorkflowPolicyID
        });
  const scheduleSavePermissionBlockReason =
    authorizationChecking
      ? ""
      : editingID === null
        ? canCreateSchedule
          ? ""
          : "Current user is not authorized to create report workflow schedules."
        : currentScheduleManagePermissionBlockReason ||
          schedulePolicyPermissionBlockReason;
  const canEditCurrentScheduleForm =
    authorizationChecking ||
    (editingID === null
      ? canCreateSchedule
      : currentScheduleManagePermissionBlockReason === "");
  const canSaveCurrentSchedule = scheduleSavePermissionBlockReason === "";
  const formPermissionNotice = settingsManagePermissionNotice({
    canManage:
      scheduleSavePermissionBlockReason ===
      "Current user is not authorized to create report workflow schedules."
        ? false
        : currentScheduleManagePermissionBlockReason === "",
    isChecking: authorizationChecking,
    resourceLabel:
      editingID === null
        ? "report workflow schedule creation"
        : `report workflow schedule #${editingID}`,
  }) ??
    (schedulePolicyPermissionBlockReason === ""
      ? null
      : {
          kind: "warning" as const,
          message: schedulePolicyPermissionBlockReason
        });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadSchedules,
    errorStatus,
    isChecking: authorizationChecking,
    resourceLabel: "report workflow schedules",
  });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;
  const relationOptions = useMemo(
    () => buildScheduleRelationOptions(reportWorkflowPoliciesResult),
    [reportWorkflowPoliciesResult]
  );
  const initialFormValues = useMemo(
    () => reportWorkflowScheduleLaunchInitialForm(launchIntent, relationOptions),
    [launchIntent, relationOptions]
  );

  const summary = useMemo(() => {
    const enabled = schedules.filter((schedule) => schedule.enabled).length;
    const calendar = schedules.filter((schedule) => schedule.cadence !== "interval").length;
    const monthly = schedules.filter((schedule) => schedule.cadence === "monthly").length;
    const policyCount = new Set(schedules.map((schedule) => schedule.report_workflow_policy_id)).size;
    return { calendar, enabled, monthly, policyCount };
  }, [schedules]);

  async function handleRefresh() {
    await refresh();
  }

  async function handleSubmit(values: ReportWorkflowScheduleFormState) {
    if (!canSaveCurrentSchedule) {
      setNotice({
        kind: "warning",
        message:
          scheduleSavePermissionBlockReason ||
          "You are not authorized to save this schedule."
      });
      return;
    }
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }

    try {
      await saveSchedule.mutateAsync({ scheduleID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }

    form.setFieldsValue(emptyReportWorkflowScheduleForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({ kind: "info", message: "Schedule saved." });
  }

  function handleScheduleFormValuesChange(changedValues: Partial<ReportWorkflowScheduleFormState>) {
    if (changedValues.cadence === undefined) {
      return;
    }
    form.setFieldsValue(reportWorkflowScheduleCadenceDefaults(changedValues.cadence));
  }

  async function handleEnablement(schedule: ReportWorkflowSchedule, enabled: boolean) {
    if (enabled) {
      const readiness = reportWorkflowScheduleEnablementReadiness({
        policyEnabledIDs: relationOptions.policyEnabledIDs,
        schedule
      });
      if (readiness.status === "blocked") {
        setNotice({ kind: "error", message: readiness.blockers.join(" ") });
        return;
      }
    }

    setActionID(schedule.id);
    try {
      await enablementAction.mutateAsync({ scheduleID: schedule.id, enabled });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      setActionID(null);
      return;
    }
    setActionID(null);
    setNotice({ kind: enabled ? "info" : "warning", message: enabled ? "Schedule enabled." : "Schedule disabled." });
  }

  function editSchedule(schedule: ReportWorkflowSchedule) {
    setEditingID(schedule.id);
    form.setFieldsValue(scheduleToFormState(schedule));
    setLaunchNotice(null);
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyReportWorkflowScheduleForm());
    setLaunchNotice(null);
    setNotice(null);
  }

  return (
    <div className="stack">
      <Row aria-label="Report workflow schedule metrics" gutter={[12, 12]}>
        <MetricCard label="Schedules" value={schedules.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Calendar" value={summary.calendar} />
        <MetricCard label="Monthly" value={summary.monthly} />
        <MetricCard label="Policies" value={summary.policyCount} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} /> : null}
      {launchNotice ? (
        <Alert
          aria-label="Report workflow schedule launch preset"
          description={launchNotice}
          message="Schedule action loaded"
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      {relationOptions.warnings.length > 0 ? (
        <Alert
          description={relationOptions.warnings.join(" ")}
          message="Related configuration unavailable"
          role="status"
          showIcon
          type="warning"
        />
      ) : null}

      <Row align="top" className="settings-console-grid" gutter={[16, 16]}>
        <Col lg={8} md={24} xs={24}>
          <Card
            extra={
              editingID === null ? null : (
                <Button disabled={busy || !canCreateSchedule} icon={<PlusOutlined />} onClick={resetForm} type="default">
                  New
                </Button>
              )
            }
            title={editingID === null ? "New Schedule" : `Edit Schedule #${editingID}`}
          >
            {formPermissionNotice ? (
              <ReadOnlyModeAlert notice={formPermissionNotice} />
            ) : null}
            <Form<ReportWorkflowScheduleFormState>
              disabled={busy || !canEditCurrentScheduleForm}
              form={form}
              initialValues={initialFormValues}
              layout="vertical"
              onFinish={handleSubmit}
              onValuesChange={handleScheduleFormValuesChange}
            >
              <Form.Item
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Schedule name is required." },
                  { max: 120, message: "Schedule name must be 120 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item
                label="Report workflow policy"
                name="reportWorkflowPolicyID"
                rules={[{ required: true, message: "Report workflow policy is required." }]}
              >
                <Select
                  optionFilterProp="label"
                  options={relationOptions.policyOptions}
                  placeholder="Select workflow policy"
                  showSearch
                />
              </Form.Item>

              <Form.Item
                label="Temporal Schedule ID"
                name="temporalScheduleID"
                rules={[
                  { required: true, message: "Temporal Schedule ID is required." },
                  { max: 200, message: "Temporal Schedule ID must be 200 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" placeholder="openclarion-report-policy-1-daily" />
              </Form.Item>

              <Form.Item
                label="Cadence"
                name="cadence"
                rules={[{ required: true, message: "Cadence is required." }]}
              >
                <Segmented block options={cadenceOptions} />
              </Form.Item>

              <Form.Item noStyle shouldUpdate={(previous, current) => previous.cadence !== current.cadence}>
                {(scheduleForm) => {
                  const cadence = scheduleForm.getFieldValue("cadence") as ReportWorkflowScheduleFormState["cadence"];
                  if (cadence === "interval") {
                    return (
                      <>
                        <Row gutter={12}>
                          <Col sm={12} xs={24}>
                            <Form.Item
                              label="Interval seconds"
                              name="intervalSeconds"
                              rules={[{ required: true, message: "Interval is required." }]}
                            >
                              <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                            </Form.Item>
                          </Col>
                          <Col sm={12} xs={24}>
                            <Form.Item
                              label="Offset seconds"
                              name="offsetSeconds"
                              rules={[{ required: true, message: "Offset is required." }]}
                            >
                              <InputNumber min={0} precision={0} style={{ width: "100%" }} />
                            </Form.Item>
                          </Col>
                        </Row>
                        <Form.Item hidden name="calendarHour">
                          <InputNumber />
                        </Form.Item>
                        <Form.Item hidden name="calendarMinute">
                          <InputNumber />
                        </Form.Item>
                        <Form.Item hidden name="calendarDayOfWeek">
                          <InputNumber />
                        </Form.Item>
                        <Form.Item hidden name="calendarDayOfMonth">
                          <InputNumber />
                        </Form.Item>
                      </>
                    );
                  }
                  return (
                    <>
                      <Row gutter={12}>
                        <Col sm={12} xs={24}>
                          <Form.Item
                            label="UTC hour"
                            name="calendarHour"
                            rules={[{ required: true, message: "UTC hour is required." }]}
                          >
                            <InputNumber max={23} min={0} precision={0} style={{ width: "100%" }} />
                          </Form.Item>
                        </Col>
                        <Col sm={12} xs={24}>
                          <Form.Item
                            label="UTC minute"
                            name="calendarMinute"
                            rules={[{ required: true, message: "UTC minute is required." }]}
                          >
                            <InputNumber max={59} min={0} precision={0} style={{ width: "100%" }} />
                          </Form.Item>
                        </Col>
                      </Row>
                      <Row gutter={12}>
                        {cadence === "weekly" ? (
                          <Col sm={12} xs={24}>
                            <Form.Item
                              label="Day of week"
                              name="calendarDayOfWeek"
                              rules={[{ required: true, message: "Day of week is required." }]}
                            >
                              <Select options={dayOfWeekOptions} />
                            </Form.Item>
                          </Col>
                        ) : (
                          <Form.Item hidden name="calendarDayOfWeek">
                            <InputNumber />
                          </Form.Item>
                        )}
                        {cadence === "monthly" ? (
                          <Col sm={12} xs={24}>
                            <Form.Item
                              label="Day of month"
                              name="calendarDayOfMonth"
                              rules={[{ required: true, message: "Day of month is required." }]}
                            >
                              <Select options={dayOfMonthOptions} />
                            </Form.Item>
                          </Col>
                        ) : (
                          <Form.Item hidden name="calendarDayOfMonth">
                            <InputNumber />
                          </Form.Item>
                        )}
                        <Col sm={12} xs={24}>
                          <Form.Item
                            label="Replay guard seconds"
                            name="intervalSeconds"
                            rules={[{ required: true, message: "Replay guard is required." }]}
                          >
                            <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                          </Form.Item>
                        </Col>
                      </Row>
                      <Form.Item hidden name="offsetSeconds">
                        <InputNumber />
                      </Form.Item>
                    </>
                  );
                }}
              </Form.Item>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Replay window seconds"
                    name="replayWindowSeconds"
                    rules={[{ required: true, message: "Replay window is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Replay delay seconds"
                    name="replayDelaySeconds"
                    rules={[{ required: true, message: "Replay delay is required." }]}
                  >
                    <InputNumber min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Replay limit"
                    name="replayLimit"
                    rules={[{ required: true, message: "Replay limit is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Catch-up seconds"
                    name="catchupWindowSeconds"
                    rules={[{ required: true, message: "Catch-up window is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item noStyle shouldUpdate>
                {(scheduleForm) => (
                  <ScheduleDraftPreview
                    relationOptions={relationOptions}
                    values={scheduleForm.getFieldsValue(true) as ReportWorkflowScheduleFormState}
                  />
                )}
              </Form.Item>

              <Space wrap>
                <Button disabled={busy || !canSaveCurrentSchedule} htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
                  Save Schedule
                </Button>
                <Button disabled={busy} onClick={resetForm} type="default">
                  Reset
                </Button>
              </Space>
            </Form>
          </Card>
        </Col>

        <Col lg={16} md={24} xs={24}>
          <Card
            extra={
              <Button disabled={busy || !canReadSchedules} icon={<ReloadOutlined />} loading={busy} onClick={handleRefresh} type="default">
                Refresh
              </Button>
            }
            title="Configured Schedules"
          >
            <ReportWorkflowScheduleTable
              actionID={actionID}
              busy={busy}
              canRead={canReadSchedules}
              canManageSchedule={(scheduleID) => currentAuthorization.can(reportWorkflowScheduleManageKey(scheduleID))}
              onDisable={(schedule) => handleEnablement(schedule, false)}
              onEdit={editSchedule}
              onEnable={(schedule) => handleEnablement(schedule, true)}
              relationOptions={relationOptions}
              schedules={schedules}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
}

function MetricCard({ label, value }: { label: string; value: number }) {
  return (
    <Col lg={6} sm={12} xs={24}>
      <Card className="settings-stat-card">
        <Statistic title={label} value={value} />
      </Card>
    </Col>
  );
}

function Notice({ notice }: { notice: SettingsNotice }) {
  return (
    <Alert
      description={notice.message}
      message={notice.kind === "error" ? "Request failed" : "Settings"}
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={notice.kind}
    />
  );
}

function ScheduleDraftPreview({
  relationOptions,
  values
}: {
  relationOptions: ScheduleRelationOptions;
  values: ReportWorkflowScheduleFormState;
}) {
  const readiness = reportWorkflowScheduleDraftReadiness({
    form: normalizeScheduleFormValues(values),
    policyEnabledIDs: relationOptions.policyEnabledIDs
  });
  const proofOutcome = reportWorkflowScheduleProofOutcome({
    form: normalizeScheduleFormValues(values),
    policyEnabledIDs: relationOptions.policyEnabledIDs,
    policyLabels: relationOptions.policyLabels
  });
  const policyLabel =
    values.reportWorkflowPolicyID === null || values.reportWorkflowPolicyID === undefined
      ? "Not selected"
      : relationLabel(
          relationOptions.policyLabels,
          values.reportWorkflowPolicyID,
          `Policy #${values.reportWorkflowPolicyID}`
        );
  const items: DescriptionsProps["items"] = [
    {
      children: policyLabel,
      key: "policy",
      label: "Policy"
    },
    {
      children: reportWorkflowScheduleFormCadenceValue(normalizeScheduleFormValues(values)),
      key: "cadence",
      label: "Cadence"
    },
    {
      children: `Window ${draftDuration(values.replayWindowSeconds)} / delay ${draftDuration(values.replayDelaySeconds)} / limit ${
        values.replayLimit ?? "Not set"
      }`,
      key: "replay",
      label: "Replay"
    },
    {
      children: draftDuration(values.catchupWindowSeconds),
      key: "catchup",
      label: "Catch-up"
    }
  ];

  return (
    <div aria-label="Schedule readiness preview" className="settings-preview-panel">
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={scheduleReadinessColor(readiness.status)}>{scheduleReadinessLabel(readiness.status)}</Tag>
          {values.temporalScheduleID ? <Tag>{values.temporalScheduleID}</Tag> : null}
        </Space>
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
        <Descriptions column={1} items={items} size="small" />
        <ScheduledProofOutcomePreview outcome={proofOutcome} />
      </Space>
    </div>
  );
}

function ScheduledProofOutcomePreview({
  outcome
}: {
  outcome: ReturnType<typeof reportWorkflowScheduleProofOutcome>;
}) {
  return (
    <div aria-label="Scheduled proof outcome" className="settings-proof-outcome">
      <div className="settings-preview-header">
        <Typography.Text strong>Scheduled Proof Outcome</Typography.Text>
        <Tag color={scheduleReadinessColor(outcome.status)}>{scheduleReadinessLabel(outcome.status)}</Tag>
      </div>
      <Typography.Text type="secondary">{outcome.detail}</Typography.Text>
      <div className="workflow-automation-grid">
        {outcome.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{item.title}</Typography.Text>
              <Tag color={scheduleReadinessColor(item.status)}>{scheduleReadinessLabel(item.status)}</Tag>
            </div>
            <Typography.Text strong>{item.value}</Typography.Text>
            <Typography.Text type="secondary">{item.detail}</Typography.Text>
          </div>
        ))}
      </div>
    </div>
  );
}

function normalizeScheduleFormValues(
  values: Partial<ReportWorkflowScheduleFormState>
): ReportWorkflowScheduleFormState {
  const defaults = emptyReportWorkflowScheduleForm();
  return {
    name: values.name ?? defaults.name,
    reportWorkflowPolicyID: values.reportWorkflowPolicyID ?? defaults.reportWorkflowPolicyID,
    temporalScheduleID: values.temporalScheduleID ?? defaults.temporalScheduleID,
    cadence: values.cadence ?? defaults.cadence,
    calendarHour: values.calendarHour ?? defaults.calendarHour,
    calendarMinute: values.calendarMinute ?? defaults.calendarMinute,
    calendarDayOfWeek: values.calendarDayOfWeek ?? defaults.calendarDayOfWeek,
    calendarDayOfMonth: values.calendarDayOfMonth ?? defaults.calendarDayOfMonth,
    intervalSeconds: values.intervalSeconds ?? defaults.intervalSeconds,
    offsetSeconds: values.offsetSeconds ?? defaults.offsetSeconds,
    replayWindowSeconds: values.replayWindowSeconds ?? defaults.replayWindowSeconds,
    replayDelaySeconds: values.replayDelaySeconds ?? defaults.replayDelaySeconds,
    replayLimit: values.replayLimit ?? defaults.replayLimit,
    catchupWindowSeconds: values.catchupWindowSeconds ?? defaults.catchupWindowSeconds
  };
}

function draftDuration(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isSafeInteger(value) || value < 0) {
    return "Not set";
  }
  return formatDurationSeconds(value);
}

function scheduleReadinessColor(status: ReportWorkflowScheduleDraftReadiness["status"]): string {
  switch (status) {
    case "ready":
      return "green";
    case "pending":
      return "default";
    case "blocked":
      return "red";
  }
}

function scheduleReadinessLabel(status: ReportWorkflowScheduleDraftReadiness["status"]): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "pending":
      return "Pending";
    case "blocked":
      return "Blocked";
  }
}

function buildScheduleRelationOptions(
  policiesResult: ApiResult<ReportWorkflowPolicyListResponse>
): ScheduleRelationOptions {
  if (!policiesResult.ok) {
    return {
      policyEnabledIDs: new Set(),
      policyLabels: {},
      policyOptions: [],
      warnings: [`Report workflow policies failed to load: ${policiesResult.error.message}.`]
    };
  }
  return {
    policyEnabledIDs: new Set(policiesResult.data.items.filter((policy) => policy.enabled).map((policy) => policy.id)),
    policyLabels: Object.fromEntries(policiesResult.data.items.map((policy) => [policy.id, policyLabel(policy)])),
    policyOptions: policiesResult.data.items.map((policy) => relationOption(policy.id, policyLabel(policy))),
    warnings: []
  };
}

function reportWorkflowScheduleLaunchInitialForm(
  launchIntent: ReportWorkflowScheduleLaunchIntent | null,
  relationOptions: ScheduleRelationOptions
): ReportWorkflowScheduleFormState {
  if (launchIntent === null) {
    return emptyReportWorkflowScheduleForm();
  }
  return {
    ...emptyReportWorkflowScheduleForm(),
    intervalSeconds: 3600,
    name: launchIntent.name,
    reportWorkflowPolicyID: launchPolicyID(launchIntent.policyID, relationOptions),
    temporalScheduleID: launchIntent.temporalScheduleID
  };
}

function launchPolicyID(
  policyID: number | null,
  relationOptions: ScheduleRelationOptions
): number | null {
  if (policyID !== null && relationOptions.policyEnabledIDs.has(policyID)) {
    return policyID;
  }
  return relationOptions.policyOptions.find((option) => relationOptions.policyEnabledIDs.has(option.value))?.value ?? null;
}

function relationOption(value: number, label: string): RelationSelectOption {
  return { value, label, title: label };
}

function policyLabel(policy: ReportWorkflowPolicy): string {
  return `#${policy.id} ${policy.name} (${policy.report_scenario}, ${policy.diagnosis_follow_up}, ${enabledLabel(policy.enabled)})`;
}

function enabledLabel(enabled: boolean): string {
  return enabled ? "enabled" : "disabled";
}

function relationLabel(labels: Record<number, string>, id: number, fallback: string): string {
  return labels[id] ?? fallback;
}

type ReportWorkflowScheduleTableProps = {
  actionID: number | null;
  busy: boolean;
  canRead: boolean;
  canManageSchedule: (scheduleID: number) => boolean;
  onDisable: (schedule: ReportWorkflowSchedule) => void;
  onEdit: (schedule: ReportWorkflowSchedule) => void;
  onEnable: (schedule: ReportWorkflowSchedule) => void;
  relationOptions: ScheduleRelationOptions;
  schedules: ReportWorkflowSchedule[];
};

function ReportWorkflowScheduleTable({
  actionID,
  busy,
  canRead,
  canManageSchedule,
  onDisable,
  onEdit,
  onEnable,
  relationOptions,
  schedules
}: ReportWorkflowScheduleTableProps) {
  const columns: TableColumnsType<ReportWorkflowSchedule> = [
    {
      key: "name",
      title: "Name",
      render: (_, schedule) => (
        <Space direction="vertical" size={2}>
          <Typography.Text strong>{schedule.name}</Typography.Text>
          <Typography.Text type="secondary">
            {relationLabel(
              relationOptions.policyLabels,
              schedule.report_workflow_policy_id,
              `Policy #${schedule.report_workflow_policy_id}`
            )}
          </Typography.Text>
          <Typography.Text className="settings-event-ids" type="secondary">
            {schedule.temporal_schedule_id}
          </Typography.Text>
        </Space>
      )
    },
    {
      key: "cadence",
      title: "Cadence",
      render: (_, schedule) => (
        <Space direction="vertical" size={2}>
          <Space size={6} wrap>
            <Tag color={schedule.cadence === "interval" ? "blue" : "purple"}>
              {reportWorkflowScheduleCadenceLabel(schedule.cadence)}
            </Tag>
            <Typography.Text>{reportWorkflowScheduleCadenceValue(schedule)}</Typography.Text>
          </Space>
          <Typography.Text type="secondary">{reportWorkflowScheduleCadenceDetail(schedule)}</Typography.Text>
        </Space>
      )
    },
    {
      key: "replay",
      title: "Replay",
      render: (_, schedule) => {
        const replayWindowBlocker = reportWorkflowScheduleReplayWindowBlocker({
          intervalSeconds: schedule.interval_seconds,
          replayWindowSeconds: schedule.replay_window_seconds
        });
        return (
          <Space direction="vertical" size={2}>
            <Space size={6} wrap>
              <Typography.Text>{formatDurationSeconds(schedule.replay_window_seconds)} window</Typography.Text>
              {replayWindowBlocker === null ? null : <Tag color="red">Overlapping window</Tag>}
            </Space>
            <Typography.Text type="secondary">
              delay {formatDurationSeconds(schedule.replay_delay_seconds)} / limit {schedule.replay_limit}
            </Typography.Text>
          </Space>
        );
      }
    },
    {
      dataIndex: "catchup_window_seconds",
      key: "catchup",
      title: "Catch-up",
      render: (seconds: number) => formatDurationSeconds(seconds)
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: "State",
      render: (enabled: boolean, schedule) => (
        <Space direction="vertical" size={2}>
          <Tag color={enabled ? "green" : "default"}>{enabled ? "Enabled" : "Draft"}</Tag>
          <Typography.Text type="secondary">
            {enabled ? nullableDate(schedule.enabled_at) : nullableDate(schedule.disabled_at)}
          </Typography.Text>
        </Space>
      )
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: "Updated",
      render: (value: string) => formatDateTime(value)
    },
    {
      key: "actions",
      render: (_, schedule) => {
        const canManage = canManageSchedule(schedule.id);
        return (
          <Space wrap>
            <Button
              disabled={busy || actionID !== null || !canManage}
              icon={<EditOutlined />}
              onClick={() => onEdit(schedule)}
              size="small"
            >
              Edit
            </Button>
            {schedule.enabled ? (
              <Button
                disabled={busy || actionID !== null || !canManage}
                icon={<PauseCircleOutlined />}
                loading={actionID === schedule.id}
                onClick={() => onDisable(schedule)}
                size="small"
              >
                Disable
              </Button>
            ) : (
              <EnableScheduleButton
                actionID={actionID}
                busy={busy}
                canManage={canManage}
                onEnable={onEnable}
                policyEnabledIDs={relationOptions.policyEnabledIDs}
                schedule={schedule}
              />
            )}
          </Space>
        );
      },
      title: "Actions"
    }
  ];

  return (
    <Table<ReportWorkflowSchedule>
      columns={columns}
      dataSource={schedules}
      loading={busy}
      locale={{
        emptyText: (
          <Empty
            description={settingsReadPermissionEmptyDescription({
              canRead,
              emptyDescription: "No report workflow schedules",
              resourceLabel: "report workflow schedules",
            })}
            image={<CalendarOutlined aria-hidden className="settings-empty-icon" />}
          />
        )
      }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1040 }}
    />
  );
}

function EnableScheduleButton({
  actionID,
  busy,
  canManage,
  onEnable,
  policyEnabledIDs,
  schedule
}: {
  actionID: number | null;
  busy: boolean;
  canManage: boolean;
  onEnable: (schedule: ReportWorkflowSchedule) => void;
  policyEnabledIDs: ReadonlySet<number>;
  schedule: ReportWorkflowSchedule;
}) {
  const readiness = reportWorkflowScheduleEnablementReadiness({ policyEnabledIDs, schedule });
  const blocked = readiness.status === "blocked";
  const button = (
    <Button
      disabled={busy || actionID !== null || blocked || !canManage}
      icon={<PlayCircleOutlined />}
      loading={actionID === schedule.id}
      onClick={() => onEnable(schedule)}
      size="small"
      type="primary"
    >
      Enable
    </Button>
  );
  if (!blocked) {
    return button;
  }
  return (
    <Tooltip title={readiness.detail}>
      <span>{button}</span>
    </Tooltip>
  );
}

function nullableDate(value: string | null): string {
  return value === null ? "-" : formatDateTime(value);
}
