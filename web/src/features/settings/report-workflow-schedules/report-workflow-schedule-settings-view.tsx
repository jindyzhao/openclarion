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
import { useLocale, useTranslations } from "next-intl";
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

const dayOfMonthOptions = Array.from({ length: 28 }, (_, index) => ({
  label: String(index + 1),
  value: index + 1
}));

type SaveScheduleVariables = {
  body: ReportWorkflowScheduleWriteRequest;
  scheduleID: number | null;
};

type WorkflowScheduleTranslator = ReturnType<
  typeof useTranslations<"WorkflowScheduleSettings">
>;

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
  const locale = useLocale();
  const t = useTranslations("WorkflowScheduleSettings");
  const common = useTranslations("Common");
  const cadenceOptions = useMemo(() => [
    { label: t("interval"), value: "interval" },
    { label: t("daily"), value: "daily" },
    { label: t("weekly"), value: "weekly" },
    { label: t("monthly"), value: "monthly" },
  ], [t]);
  const dayOfWeekOptions = useMemo(() => [
    t("sunday"), t("monday"), t("tuesday"), t("wednesday"),
    t("thursday"), t("friday"), t("saturday"),
  ].map((label, value) => ({ label, value })), [t]);
  const [form] = Form.useForm<ReportWorkflowScheduleFormState>();
  const clientReady = useClientReady();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [actionID, setActionID] = useState<number | null>(null);
  const [launchNotice, setLaunchNotice] = useState<string | null>(
    launchIntent?.message ?? null,
  );
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
    refreshMessage: t("refreshed"),
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
  const createPermissionDenied =
    !authorizationChecking && editingID === null && !canCreateSchedule;
  const formPermissionNotice = settingsManagePermissionNotice({
    canManage:
      !createPermissionDenied && currentScheduleManagePermissionBlockReason === "",
    isChecking: authorizationChecking,
    message: common("formReadOnly", {
      resource:
        editingID === null
          ? t("creationResource")
          : t("scheduleResource", { id: editingID }),
    }),
  }) ??
    (schedulePolicyPermissionBlockReason === ""
      ? null
      : {
          kind: "warning" as const,
          message: localizeWorkflowScheduleText(schedulePolicyPermissionBlockReason, t)
        });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadSchedules,
    errorStatus,
    isChecking: authorizationChecking,
    message: common("readAccessLimited", {
      resource: t("schedulesResource"),
    }),
  });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;
  const relationOptions = useMemo(
    () => buildScheduleRelationOptions(reportWorkflowPoliciesResult, t),
    [reportWorkflowPoliciesResult, t]
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
          localizeWorkflowScheduleText(scheduleSavePermissionBlockReason, t) ||
          t("notAuthorizedSave")
      });
      return;
    }
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeWorkflowScheduleText(parsed.message, t) });
      return;
    }

    try {
      await saveSchedule.mutateAsync({ scheduleID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      return;
    }

    form.setFieldsValue(emptyReportWorkflowScheduleForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({ kind: "info", message: t("saved") });
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
        setNotice({
          kind: "error",
          message: localizeWorkflowScheduleMessages(readiness.blockers, t),
        });
        return;
      }
    }

    setActionID(schedule.id);
    try {
      await enablementAction.mutateAsync({ scheduleID: schedule.id, enabled });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      setActionID(null);
      return;
    }
    setActionID(null);
    setNotice({ kind: enabled ? "info" : "warning", message: enabled ? t("enabledNotice") : t("disabledNotice") });
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
      <Row aria-label={t("metricsLabel")} gutter={[12, 12]}>
        <MetricCard label={t("schedules")} value={schedules.length} />
        <MetricCard label={t("enabled")} value={summary.enabled} />
        <MetricCard label={t("calendar")} value={summary.calendar} />
        <MetricCard label={t("monthly")} value={summary.monthly} />
        <MetricCard label={t("policies")} value={summary.policyCount} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} /> : null}
      {launchNotice ? (
        <Alert
          aria-label={t("launchPreset")}
          description={localizeWorkflowScheduleText(launchNotice, t)}
          message={t("actionLoaded")}
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      {relationOptions.warnings.length > 0 ? (
        <Alert
          description={localizeWorkflowScheduleMessages(relationOptions.warnings, t)}
          message={t("relatedUnavailable")}
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
                  {t("new")}
                </Button>
              )
            }
            title={editingID === null ? t("newSchedule") : t("editSchedule", { id: editingID })}
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
                label={t("name")}
                name="name"
                rules={[
                  { required: true, message: t("nameRequired") },
                  { max: 120, message: t("nameLength") }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item
                label={t("workflowPolicy")}
                name="reportWorkflowPolicyID"
                rules={[{ required: true, message: t("policyRequired") }]}
              >
                <Select
                  optionFilterProp="label"
                  options={relationOptions.policyOptions}
                  placeholder={t("selectPolicy")}
                  showSearch
                />
              </Form.Item>

              <Form.Item
                label={t("temporalScheduleId")}
                name="temporalScheduleID"
                rules={[
                  { required: true, message: t("temporalIdRequired") },
                  { max: 200, message: t("temporalIdLength") }
                ]}
              >
                <Input autoComplete="off" placeholder="openclarion-report-policy-1-daily" />
              </Form.Item>

              <Form.Item
                label={t("cadence")}
                name="cadence"
                rules={[{ required: true, message: t("cadenceRequired") }]}
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
                              label={t("intervalSeconds")}
                              name="intervalSeconds"
                              rules={[{ required: true, message: t("intervalRequired") }]}
                            >
                              <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                            </Form.Item>
                          </Col>
                          <Col sm={12} xs={24}>
                            <Form.Item
                              label={t("offsetSeconds")}
                              name="offsetSeconds"
                              rules={[{ required: true, message: t("offsetRequired") }]}
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
                            label={t("utcHour")}
                            name="calendarHour"
                            rules={[{ required: true, message: t("hourRequired") }]}
                          >
                            <InputNumber max={23} min={0} precision={0} style={{ width: "100%" }} />
                          </Form.Item>
                        </Col>
                        <Col sm={12} xs={24}>
                          <Form.Item
                            label={t("utcMinute")}
                            name="calendarMinute"
                            rules={[{ required: true, message: t("minuteRequired") }]}
                          >
                            <InputNumber max={59} min={0} precision={0} style={{ width: "100%" }} />
                          </Form.Item>
                        </Col>
                      </Row>
                      <Row gutter={12}>
                        {cadence === "weekly" ? (
                          <Col sm={12} xs={24}>
                            <Form.Item
                              label={t("dayOfWeek")}
                              name="calendarDayOfWeek"
                              rules={[{ required: true, message: t("dayOfWeekRequired") }]}
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
                              label={t("dayOfMonth")}
                              name="calendarDayOfMonth"
                              rules={[{ required: true, message: t("dayOfMonthRequired") }]}
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
                            label={t("replayGuardSeconds")}
                            name="intervalSeconds"
                            rules={[{ required: true, message: t("replayGuardRequired") }]}
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
                    label={t("replayWindowSeconds")}
                    name="replayWindowSeconds"
                    rules={[{ required: true, message: t("replayWindowRequired") }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("replayDelaySeconds")}
                    name="replayDelaySeconds"
                    rules={[{ required: true, message: t("replayDelayRequired") }]}
                  >
                    <InputNumber min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("replayLimit")}
                    name="replayLimit"
                    rules={[{ required: true, message: t("replayLimitRequired") }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("catchUpSeconds")}
                    name="catchupWindowSeconds"
                    rules={[{ required: true, message: t("catchUpRequired") }]}
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
                  {t("saveSchedule")}
                </Button>
                <Button disabled={busy} onClick={resetForm} type="default">
                  {t("reset")}
                </Button>
              </Space>
            </Form>
          </Card>
        </Col>

        <Col lg={16} md={24} xs={24}>
          <Card
            extra={
              <Button disabled={busy || !canReadSchedules} icon={<ReloadOutlined />} loading={busy} onClick={handleRefresh} type="default">
                {t("refresh")}
              </Button>
            }
            title={t("configuredSchedules")}
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
              locale={locale}
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
  const t = useTranslations("WorkflowScheduleSettings");
  return (
    <Alert
      description={notice.message}
      message={notice.kind === "error" ? t("requestFailed") : t("settings")}
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
  const t = useTranslations("WorkflowScheduleSettings");
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
      ? t("notSelected")
      : relationLabel(
          relationOptions.policyLabels,
          values.reportWorkflowPolicyID,
          t("policyNumber", { id: values.reportWorkflowPolicyID })
        );
  const items: DescriptionsProps["items"] = [
    {
      children: policyLabel,
      key: "policy",
      label: t("policy")
    },
    {
      children: localizeWorkflowScheduleText(reportWorkflowScheduleFormCadenceValue(normalizeScheduleFormValues(values)), t),
      key: "cadence",
      label: t("cadence")
    },
    {
      children: t("replaySummary", {
        delay: draftDuration(values.replayDelaySeconds, t),
        limit: values.replayLimit ?? t("notSet"),
        window: draftDuration(values.replayWindowSeconds, t),
      }),
      key: "replay",
      label: t("replay")
    },
    {
      children: draftDuration(values.catchupWindowSeconds, t),
      key: "catchup",
      label: t("catchUp")
    }
  ];

  return (
    <div aria-label={t("readinessPreview")} className="settings-preview-panel">
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={scheduleReadinessColor(readiness.status)}>{scheduleReadinessLabel(readiness.status, t)}</Tag>
          {values.temporalScheduleID ? <Tag>{values.temporalScheduleID}</Tag> : null}
        </Space>
        <Typography.Text strong>{localizeWorkflowScheduleText(readiness.label, t)}</Typography.Text>
        <Typography.Text type="secondary">{localizeWorkflowScheduleText(readiness.detail, t)}</Typography.Text>
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
  const t = useTranslations("WorkflowScheduleSettings");
  return (
    <div aria-label={t("proofOutcomeLabel")} className="settings-proof-outcome">
      <div className="settings-preview-header">
        <Typography.Text strong>{t("proofOutcome")}</Typography.Text>
        <Tag color={scheduleReadinessColor(outcome.status)}>{scheduleReadinessLabel(outcome.status, t)}</Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowScheduleText(outcome.detail, t)}</Typography.Text>
      <div className="workflow-automation-grid">
        {outcome.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{localizeWorkflowScheduleText(item.title, t)}</Typography.Text>
              <Tag color={scheduleReadinessColor(item.status)}>{scheduleReadinessLabel(item.status, t)}</Tag>
            </div>
            <Typography.Text strong>{localizeWorkflowScheduleText(item.value, t)}</Typography.Text>
            <Typography.Text type="secondary">{localizeWorkflowScheduleText(item.detail, t)}</Typography.Text>
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

function draftDuration(
  value: number | null | undefined,
  t: WorkflowScheduleTranslator,
): string {
  if (value === null || value === undefined || !Number.isSafeInteger(value) || value < 0) {
    return t("catalog.notSet");
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

function scheduleReadinessLabel(
  status: ReportWorkflowScheduleDraftReadiness["status"],
  t: WorkflowScheduleTranslator,
): string {
  return t(`readinessStatus.${status}`);
}

function buildScheduleRelationOptions(
  policiesResult: ApiResult<ReportWorkflowPolicyListResponse>,
  t: WorkflowScheduleTranslator,
): ScheduleRelationOptions {
  if (!policiesResult.ok) {
    return {
      policyEnabledIDs: new Set(),
      policyLabels: {},
      policyOptions: [],
      warnings: [t("runtimePattern.policiesFailed", {
        error: policiesResult.error.message,
      })]
    };
  }
  return {
    policyEnabledIDs: new Set(policiesResult.data.items.filter((policy) => policy.enabled).map((policy) => policy.id)),
    policyLabels: Object.fromEntries(policiesResult.data.items.map((policy) => [policy.id, policyLabel(policy, t)])),
    policyOptions: policiesResult.data.items.map((policy) => relationOption(policy.id, policyLabel(policy, t))),
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

function policyLabel(
  policy: ReportWorkflowPolicy,
  t: WorkflowScheduleTranslator,
): string {
  return `#${policy.id} ${policy.name} (${localizeWorkflowScheduleText(policy.report_scenario, t)}, ${localizeWorkflowScheduleText(policy.diagnosis_follow_up, t)}, ${enabledLabel(policy.enabled, t)})`;
}

function enabledLabel(enabled: boolean, t: WorkflowScheduleTranslator): string {
  return localizeWorkflowScheduleText(enabled ? "enabled" : "disabled", t);
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
  locale: string;
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
  schedules,
  locale,
}: ReportWorkflowScheduleTableProps) {
  const t = useTranslations("WorkflowScheduleSettings");
  const common = useTranslations("Common");
  const columns: TableColumnsType<ReportWorkflowSchedule> = [
    {
      key: "name",
      title: t("name"),
      render: (_, schedule) => (
        <Space direction="vertical" size={2}>
          <Typography.Text strong>{schedule.name}</Typography.Text>
          <Typography.Text type="secondary">
            {relationLabel(
              relationOptions.policyLabels,
              schedule.report_workflow_policy_id,
              t("policyNumber", { id: schedule.report_workflow_policy_id })
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
      title: t("cadence"),
      render: (_, schedule) => (
        <Space direction="vertical" size={2}>
          <Space size={6} wrap>
            <Tag color={schedule.cadence === "interval" ? "blue" : "purple"}>
              {localizeWorkflowScheduleText(reportWorkflowScheduleCadenceLabel(schedule.cadence), t)}
            </Tag>
            <Typography.Text>{localizeWorkflowScheduleText(reportWorkflowScheduleCadenceValue(schedule), t)}</Typography.Text>
          </Space>
          <Typography.Text type="secondary">{localizeWorkflowScheduleText(reportWorkflowScheduleCadenceDetail(schedule), t)}</Typography.Text>
        </Space>
      )
    },
    {
      key: "replay",
      title: t("replay"),
      render: (_, schedule) => {
        const replayWindowBlocker = reportWorkflowScheduleReplayWindowBlocker({
          intervalSeconds: schedule.interval_seconds,
          replayWindowSeconds: schedule.replay_window_seconds
        });
        return (
          <Space direction="vertical" size={2}>
            <Space size={6} wrap>
              <Typography.Text>{t("windowDuration", { duration: formatDurationSeconds(schedule.replay_window_seconds) })}</Typography.Text>
              {replayWindowBlocker === null ? null : <Tag color="red">{t("overlappingWindow")}</Tag>}
            </Space>
            <Typography.Text type="secondary">
              {t("delayLimit", { delay: formatDurationSeconds(schedule.replay_delay_seconds), limit: schedule.replay_limit })}
            </Typography.Text>
          </Space>
        );
      }
    },
    {
      dataIndex: "catchup_window_seconds",
      key: "catchup",
      title: t("catchUp"),
      render: (seconds: number) => formatDurationSeconds(seconds)
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: t("state"),
      render: (enabled: boolean, schedule) => (
        <Space direction="vertical" size={2}>
          <Tag color={enabled ? "green" : "default"}>{enabled ? t("enabled") : t("draft")}</Tag>
          <Typography.Text type="secondary">
            {enabled ? nullableDate(schedule.enabled_at, locale, t) : nullableDate(schedule.disabled_at, locale, t)}
          </Typography.Text>
        </Space>
      )
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: t("updated"),
      render: (value: string) => formatDateTime(value, locale)
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
              {t("edit")}
            </Button>
            {schedule.enabled ? (
              <Button
                disabled={busy || actionID !== null || !canManage}
                icon={<PauseCircleOutlined />}
                loading={actionID === schedule.id}
                onClick={() => onDisable(schedule)}
                size="small"
              >
                {t("disable")}
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
      title: t("actions")
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
              deniedDescription: common("noReadAccess", {
                resource: t("schedulesResource"),
              }),
              emptyDescription: t("noSchedules"),
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
  const t = useTranslations("WorkflowScheduleSettings");
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
      {t("enable")}
    </Button>
  );
  if (!blocked) {
    return button;
  }
  return (
    <Tooltip title={localizeWorkflowScheduleText(readiness.detail, t)}>
      <span>{button}</span>
    </Tooltip>
  );
}

function nullableDate(
  value: string | null,
  locale: string,
  t: WorkflowScheduleTranslator,
): string {
  return value === null ? t("catalog.notSet") : formatDateTime(value, locale);
}

function localizeWorkflowScheduleMessages(
  messages: readonly string[],
  t: WorkflowScheduleTranslator,
): string {
  return messages
    .map((message) => localizeWorkflowScheduleText(message, t))
    .join(" ");
}

const workflowScheduleRuntimeTextKeys = {
    "alert_storm": "runtimeText.alertStorm",
    "auto_room": "runtimeText.autoRoom",
    "blocked": "runtimeText.blocked",
    "cascade": "runtimeText.cascade",
    "disabled": "runtimeText.disabled",
    "enabled": "enabled",
    "interval": "runtimeText.interval",
    "pending": "runtimeText.pending",
    "ready": "runtimeText.ready",
    "single_alert": "runtimeText.singleAlert",
    "suggest_room": "runtimeText.suggestRoom",
    "Bound report workflow policy is enabled.": "runtimeText.boundReportWorkflowPolicyIsEnabled",
    "Bound report workflow policy must be enabled before schedule enablement.": "runtimeText.boundReportWorkflowPolicyMustBeEnabledBeforeScheduleEnablement",
    "The bound policy is enabled and can be scheduled.": "runtimeText.boundPolicyIsEnabledAndCanBeScheduled",
    "Enable the bound report workflow policy before this schedule can run.": "runtimeText.enableTheBoundReportWorkflowPolicyBeforeThisScheduleCanRun",
    "Select an enabled report workflow policy before scheduling proof.": "runtimeText.selectAnEnabledReportWorkflowPolicyBeforeSchedulingProof",
    "Calendar day of month must be between 1 and 28.": "runtimeText.calendarDayOfMonthMustBeBetween1And28",
    "Calendar day of week must be between 0 and 6.": "runtimeText.calendarDayOfWeekMustBeBetween0And6",
    "Calendar hour must be between 0 and 23.": "runtimeText.calendarHourMustBeBetween0And23",
    "Calendar minute must be between 0 and 59.": "runtimeText.calendarMinuteMustBeBetween0And59",
    "Cadence": "runtimeText.cadence",
    "Catch-up": "runtimeText.catchUp",
    "Complete schedule fields.": "runtimeText.completeScheduleFields",
    "Daily": "runtimeText.daily",
    "Daily cadence must not include calendar day fields.": "runtimeText.dailyCadenceMustNotIncludeCalendarDayFields",
    "Enable the selected report workflow policy before this schedule can be enabled.": "runtimeText.enableTheSelectedReportWorkflowPolicyBeforeThisScheduleCanBeEnabled",
    "Friday": "runtimeText.friday",
    "Hourly report replay": "runtimeText.hourlyReportReplay",
    "Interval": "runtimeText.interval2",
    "Interval cadence must not include calendar fields.": "runtimeText.intervalCadenceMustNotIncludeCalendarFields",
    "Interval must be between 1 and 31536000 seconds.": "runtimeText.intervalMustBeBetween1And31536000Seconds",
    "Catch-up window must be between 1 and 31536000 seconds.": "runtimeText.catchUpWindowMustBeBetween1And31536000Seconds",
    "Monday": "runtimeText.monday",
    "Monthly": "runtimeText.monthly",
    "Monthly cadence must not include calendar day of week.": "runtimeText.monthlyCadenceMustNotIncludeCalendarDayOfWeek",
    "Not named": "runtimeText.notNamed",
    "Not selected": "runtimeText.notSelected",
    "Not set": "runtimeText.notSet",
    "Offset must be 0 for calendar cadences.": "runtimeText.offsetMustBe0ForCalendarCadences",
    "Offset must be between 0 and 31536000 seconds.": "runtimeText.offsetMustBeBetween0And31536000Seconds",
    "Offset must be less than interval.": "runtimeText.offsetMustBeLessThanInterval",
    "Policy": "runtimeText.policy",
    "Policy not ready.": "runtimeText.policyNotReady",
    "Proof target": "runtimeText.proofTarget",
    "Ready to enable.": "runtimeText.readyToEnable",
    "Ready to save.": "runtimeText.readyToSave",
    "Replay sample": "runtimeText.replaySample",
    "Replay window overlaps interval.": "runtimeText.replayWindowOverlapsInterval",
    "Replay window, delay, and limit must be valid, and the window cannot exceed the cadence guard interval.": "runtimeText.replayWindowDelayAndLimitMustBeValid",
    "Replay delay must be between 0 and 31536000 seconds.": "runtimeText.replayDelayMustBeBetween0And31536000Seconds",
    "Replay limit must be between 1 and 100000.": "runtimeText.replayLimitMustBeBetween1And100000",
    "Replay window must be between 1 and 31536000 seconds.": "runtimeText.replayWindowMustBeBetween1And31536000Seconds",
    "Saturday": "runtimeText.saturday",
    "Schedule fields and bound workflow policy are ready.": "runtimeText.scheduleFieldsAndBoundWorkflowPolicyAreReady",
    "Set a positive catch-up window for missed scheduled starts.": "runtimeText.setAPositiveCatchUpWindowForMissedScheduledStarts",
    "Selected policy is not enabled.": "runtimeText.selectedPolicyIsNotEnabled",
    "Set a positive interval and an offset that is lower than the interval.": "runtimeText.setAPositiveIntervalAndAnOffsetThatIsLowerThanThe",
    "Set valid UTC calendar fields, a zero offset, and a positive replay guard interval.": "runtimeText.setValidUtcCalendarFieldsAZeroOffsetAndAPositiveReplay",
    "Select a report workflow policy.": "runtimeText.selectAReportWorkflowPolicy",
    "Select the workflow policy that this schedule should replay.": "runtimeText.selectTheWorkflowPolicyThatThisScheduleShouldReplay",
    "Weekly": "runtimeText.weekly",
    "Weekly cadence must not include calendar day of month.": "runtimeText.weeklyCadenceMustNotIncludeCalendarDayOfMonth",
    "Sunday": "runtimeText.sunday",
    "Thursday": "runtimeText.thursday",
    "Tuesday": "runtimeText.tuesday",
    "Wednesday": "runtimeText.wednesday",
    "Weekday": "runtimeText.weekday",
    "Schedule name is required.": "runtimeText.scheduleNameIsRequired",
    "Schedule name must be 120 characters or fewer.": "runtimeText.scheduleNameMustBe120CharactersOrFewer",
    "Prepared an hourly replay schedule from the settings overview proof action.": "runtimeText.preparedAnHourlyReplayScheduleFromTheSettingsOverviewProofAction",
    "Temporal Schedule ID is required.": "runtimeText.temporalScheduleIdIsRequired",
    "Temporal Schedule ID must not contain whitespace.": "runtimeText.temporalScheduleIdMustNotContainWhitespace",
    "Temporal Schedule ID must be 200 characters or fewer.": "runtimeText.temporalScheduleIdMustBe200CharactersOrFewer",
    "Provide a non-empty Temporal Schedule ID without whitespace.": "runtimeText.provideANonEmptyTemporalScheduleIdWithoutWhitespace",
    "Replay window must be less than or equal to cadence guard interval to avoid overlapping scheduled replay windows.": "runtimeText.replayWindowMustBeLessThanOrEqualToCadenceGuardInterval",
    "This schedule can retain recurring proof by replaying bounded alert windows through the selected workflow policy.": "runtimeText.thisScheduleCanRetainRecurringProofByReplayingBoundedAlertWindowsThrough",
    "Complete the schedule fields before relying on recurring replay proof.": "runtimeText.completeTheScheduleFieldsBeforeRelyingOnRecurringReplayProof",
    "Resolve blocked policy or timing settings before scheduled-trigger proof can run.": "runtimeText.resolveBlockedPolicyOrTimingSettingsBeforeScheduledTriggerProofCanRun",
} as const;

export function localizeWorkflowScheduleText(value: string, t: WorkflowScheduleTranslator): string {
  const key =
    workflowScheduleRuntimeTextKeys[
      value as keyof typeof workflowScheduleRuntimeTextKeys
    ];
  if (key !== undefined) {
    return t(key);
  }
  let match = value.match(/^Current user is not authorized to manage report workflow schedule #(\d+)\.$/);
  if (match) {
    return t("runtimePattern.unauthorizedSchedule", { id: match[1]! });
  }
  match = value.match(/^Every (.+)$/);
  if (match) {
    return t("runtimePattern.everyInterval", { interval: match[1]! });
  }
  match = value.match(/^Daily at (.+)$/);
  if (match) {
    return t("runtimePattern.dailyAt", { time: match[1]! });
  }
  match = value.match(/^Day (.+) at (.+)$/);
  if (match) {
    return t("runtimePattern.monthlyAt", {
      day: match[1]!,
      time: match[2]!,
    });
  }
  match = value.match(/^(Sunday|Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Weekday) at (.+)$/);
  if (match) {
    return t("runtimePattern.weeklyAt", {
      weekday: localizeWorkflowScheduleText(match[1]!, t),
      time: match[2]!,
    });
  }
  match = value.match(/^Starts every (.+) after offset (.+)\.$/);
  if (match) {
    return t("runtimePattern.intervalWithOffset", {
      interval: match[1]!,
      offset: match[2]!,
    });
  }
  match = value.match(/^Each run samples a (.+) alert window after a (.+) delay, capped at (\d+) events\.$/);
  if (match) {
    return t("runtimePattern.replaySampleDetail", {
      delay: match[2]!,
      limit: Number(match[3]),
      window: match[1]!,
    });
  }
  match = value.match(/^Temporal can catch up missed schedule starts within (.+)\.$/);
  if (match) {
    return t("runtimePattern.catchUpDetail", { window: match[1]! });
  }
  match = value.match(/^Temporal Schedule ID (.+) is ready for retained scheduled-trigger proof\.$/);
  if (match) {
    return t("runtimePattern.temporalIdReady", { id: match[1]! });
  }
  match = value.match(/^(Daily|Weekly|Monthly) UTC calendar schedule with (.+) replay guard interval\.$/);
  if (match) {
    return t("runtimePattern.calendarGuard", {
      cadence: localizeWorkflowScheduleText(match[1]!, t),
      guard: match[2]!,
    });
  }
  match = value.match(/^(.+) window$/);
  if (match) {
    return t("runtimePattern.window", { duration: match[1]! });
  }
  match = value.match(/^Policy #(\d+)$/);
  if (match) {
    return t("runtimePattern.policyNumber", { id: match[1]! });
  }
  return value;
}
