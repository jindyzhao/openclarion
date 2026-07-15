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
          message: localizeWorkflowScheduleText(schedulePolicyPermissionBlockReason, locale)
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
    () => buildScheduleRelationOptions(reportWorkflowPoliciesResult, locale),
    [locale, reportWorkflowPoliciesResult]
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
          localizeWorkflowScheduleText(scheduleSavePermissionBlockReason, locale) ||
          t("notAuthorizedSave")
      });
      return;
    }
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeWorkflowScheduleText(parsed.message, locale) });
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
          message: localizeWorkflowScheduleMessages(readiness.blockers, locale),
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
          description={localizeWorkflowScheduleText(launchNotice, locale)}
          message={t("actionLoaded")}
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      {relationOptions.warnings.length > 0 ? (
        <Alert
          description={localizeWorkflowScheduleMessages(relationOptions.warnings, locale)}
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
                    locale={locale}
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
  locale,
  relationOptions,
  values
}: {
  locale: string;
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
      children: localizeWorkflowScheduleText(reportWorkflowScheduleFormCadenceValue(normalizeScheduleFormValues(values)), locale),
      key: "cadence",
      label: t("cadence")
    },
    {
      children: t("replaySummary", {
        delay: draftDuration(values.replayDelaySeconds, locale),
        limit: values.replayLimit ?? t("notSet"),
        window: draftDuration(values.replayWindowSeconds, locale),
      }),
      key: "replay",
      label: t("replay")
    },
    {
      children: draftDuration(values.catchupWindowSeconds, locale),
      key: "catchup",
      label: t("catchUp")
    }
  ];

  return (
    <div aria-label={t("readinessPreview")} className="settings-preview-panel">
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={scheduleReadinessColor(readiness.status)}>{scheduleReadinessLabel(readiness.status, locale)}</Tag>
          {values.temporalScheduleID ? <Tag>{values.temporalScheduleID}</Tag> : null}
        </Space>
        <Typography.Text strong>{localizeWorkflowScheduleText(readiness.label, locale)}</Typography.Text>
        <Typography.Text type="secondary">{localizeWorkflowScheduleText(readiness.detail, locale)}</Typography.Text>
        <Descriptions column={1} items={items} size="small" />
        <ScheduledProofOutcomePreview locale={locale} outcome={proofOutcome} />
      </Space>
    </div>
  );
}

function ScheduledProofOutcomePreview({
  locale,
  outcome
}: {
  locale: string;
  outcome: ReturnType<typeof reportWorkflowScheduleProofOutcome>;
}) {
  const t = useTranslations("WorkflowScheduleSettings");
  return (
    <div aria-label={t("proofOutcomeLabel")} className="settings-proof-outcome">
      <div className="settings-preview-header">
        <Typography.Text strong>{t("proofOutcome")}</Typography.Text>
        <Tag color={scheduleReadinessColor(outcome.status)}>{scheduleReadinessLabel(outcome.status, locale)}</Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowScheduleText(outcome.detail, locale)}</Typography.Text>
      <div className="workflow-automation-grid">
        {outcome.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{localizeWorkflowScheduleText(item.title, locale)}</Typography.Text>
              <Tag color={scheduleReadinessColor(item.status)}>{scheduleReadinessLabel(item.status, locale)}</Tag>
            </div>
            <Typography.Text strong>{localizeWorkflowScheduleText(item.value, locale)}</Typography.Text>
            <Typography.Text type="secondary">{localizeWorkflowScheduleText(item.detail, locale)}</Typography.Text>
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

function draftDuration(value: number | null | undefined, locale: string): string {
  if (value === null || value === undefined || !Number.isSafeInteger(value) || value < 0) {
    return locale === "zh-CN" ? "未设置" : "Not set";
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

function scheduleReadinessLabel(status: ReportWorkflowScheduleDraftReadiness["status"], locale = "en"): string {
  switch (status) {
    case "ready":
      return locale === "zh-CN" ? "就绪" : "Ready";
    case "pending":
      return locale === "zh-CN" ? "等待中" : "Pending";
    case "blocked":
      return locale === "zh-CN" ? "已阻塞" : "Blocked";
  }
}

function buildScheduleRelationOptions(
  policiesResult: ApiResult<ReportWorkflowPolicyListResponse>,
  locale: string,
): ScheduleRelationOptions {
  if (!policiesResult.ok) {
    return {
      policyEnabledIDs: new Set(),
      policyLabels: {},
      policyOptions: [],
      warnings: [locale === "zh-CN"
        ? `报告工作流策略加载失败：${policiesResult.error.message}。`
        : `Report workflow policies failed to load: ${policiesResult.error.message}.`]
    };
  }
  return {
    policyEnabledIDs: new Set(policiesResult.data.items.filter((policy) => policy.enabled).map((policy) => policy.id)),
    policyLabels: Object.fromEntries(policiesResult.data.items.map((policy) => [policy.id, policyLabel(policy, locale)])),
    policyOptions: policiesResult.data.items.map((policy) => relationOption(policy.id, policyLabel(policy, locale))),
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

function policyLabel(policy: ReportWorkflowPolicy, locale: string): string {
  return `#${policy.id} ${policy.name} (${localizeWorkflowScheduleText(policy.report_scenario, locale)}, ${localizeWorkflowScheduleText(policy.diagnosis_follow_up, locale)}, ${enabledLabel(policy.enabled, locale)})`;
}

function enabledLabel(enabled: boolean, locale: string): string {
  return locale === "zh-CN"
    ? enabled ? "已启用" : "已停用"
    : enabled ? "enabled" : "disabled";
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
              {localizeWorkflowScheduleText(reportWorkflowScheduleCadenceLabel(schedule.cadence), locale)}
            </Tag>
            <Typography.Text>{localizeWorkflowScheduleText(reportWorkflowScheduleCadenceValue(schedule), locale)}</Typography.Text>
          </Space>
          <Typography.Text type="secondary">{localizeWorkflowScheduleText(reportWorkflowScheduleCadenceDetail(schedule), locale)}</Typography.Text>
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
            {enabled ? nullableDate(schedule.enabled_at, locale) : nullableDate(schedule.disabled_at, locale)}
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
                locale={locale}
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
  locale,
  onEnable,
  policyEnabledIDs,
  schedule
}: {
  actionID: number | null;
  busy: boolean;
  canManage: boolean;
  locale: string;
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
    <Tooltip title={localizeWorkflowScheduleText(readiness.detail, locale)}>
      <span>{button}</span>
    </Tooltip>
  );
}

function nullableDate(value: string | null, locale: string): string {
  return value === null ? "-" : formatDateTime(value, locale);
}

function localizeWorkflowScheduleMessages(
  messages: readonly string[],
  locale: string,
): string {
  return messages
    .map((message) => localizeWorkflowScheduleText(message, locale))
    .join(" ");
}

function localizeWorkflowScheduleText(value: string, locale: string): string {
  if (locale !== "zh-CN") {
    return value;
  }
  const exact: Readonly<Record<string, string>> = {
    alert_storm: "告警风暴",
    auto_room: "自动创建诊断室",
    blocked: "已阻塞",
    cascade: "级联故障",
    disabled: "已停用",
    interval: "固定间隔",
    pending: "等待中",
    ready: "就绪",
    single_alert: "单告警",
    suggest_room: "建议创建诊断室",
    "Bound report workflow policy is enabled.": "绑定的报告工作流策略已启用。",
    "Bound report workflow policy must be enabled before schedule enablement.":
      "启用定时任务前，必须先启用绑定的报告工作流策略。",
    "Calendar day of month must be between 1 and 28.":
      "日历日期必须在 1 到 28 之间。",
    "Calendar day of week must be between 0 and 6.":
      "日历星期值必须在 0 到 6 之间。",
    "Calendar hour must be between 0 and 23.":
      "日历小时必须在 0 到 23 之间。",
    "Calendar minute must be between 0 and 59.":
      "日历分钟必须在 0 到 59 之间。",
    "Cadence": "执行周期",
    "Catch-up": "补偿执行",
    "Complete schedule fields.": "请完成定时任务字段。",
    "Daily": "每天",
    "Daily cadence must not include calendar day fields.":
      "每日周期不能包含日历日期字段。",
    "Enable the selected report workflow policy before this schedule can be enabled.":
      "启用此定时任务前，请先启用所选报告工作流策略。",
    Friday: "星期五",
    "Hourly report replay": "每小时报告重放",
    "Interval": "固定间隔",
    "Interval cadence must not include calendar fields.":
      "固定间隔周期不能包含日历字段。",
    "Interval must be between 1 and 31536000 seconds.":
      "间隔必须在 1 到 31536000 秒之间。",
    "Catch-up window must be between 1 and 31536000 seconds.":
      "补偿窗口必须在 1 到 31536000 秒之间。",
    Monday: "星期一",
    "Monthly": "每月",
    "Monthly cadence must not include calendar day of week.":
      "每月周期不能包含星期字段。",
    "Not named": "未命名",
    "Not selected": "未选择",
    "Not set": "未设置",
    "Offset must be 0 for calendar cadences.":
      "日历周期的偏移必须为 0。",
    "Offset must be between 0 and 31536000 seconds.":
      "偏移必须在 0 到 31536000 秒之间。",
    "Offset must be less than interval.": "偏移必须小于间隔。",
    "Policy": "策略",
    "Policy not ready.": "策略未就绪。",
    "Proof target": "证明目标",
    "Ready to enable.": "可以启用。",
    "Ready to save.": "可以保存。",
    "Replay sample": "重放样例",
    "Replay window overlaps interval.": "重放窗口与执行间隔重叠。",
    "Replay delay must be between 0 and 31536000 seconds.":
      "重放延迟必须在 0 到 31536000 秒之间。",
    "Replay limit must be between 1 and 100000.":
      "重放限制必须在 1 到 100000 之间。",
    "Replay window must be between 1 and 31536000 seconds.":
      "重放窗口必须在 1 到 31536000 秒之间。",
    Saturday: "星期六",
    "Schedule fields and bound workflow policy are ready.":
      "定时任务字段和绑定的工作流策略均已就绪。",
    "Selected policy is not enabled.": "所选策略尚未启用。",
    "Set a positive interval and an offset that is lower than the interval.":
      "请设置正数间隔，并确保偏移小于间隔。",
    "Set valid UTC calendar fields, a zero offset, and a positive replay guard interval.":
      "请设置有效的 UTC 日历字段、零偏移和正数重放保护间隔。",
    "Select a report workflow policy.": "请选择报告工作流策略。",
    "Select the workflow policy that this schedule should replay.":
      "请选择此定时任务需要重放的工作流策略。",
    "Weekly": "每周",
    "Weekly cadence must not include calendar day of month.":
      "每周周期不能包含月内日期字段。",
    Sunday: "星期日",
    Thursday: "星期四",
    Tuesday: "星期二",
    Wednesday: "星期三",
    Weekday: "星期",
    "Schedule name is required.": "定时任务名称为必填项。",
    "Schedule name must be 120 characters or fewer.": "定时任务名称不能超过 120 个字符。",
    "Prepared an hourly replay schedule from the settings overview proof action.":
      "已根据配置概览证明操作准备每小时重放定时任务。",
    "Temporal Schedule ID is required.": "Temporal 定时任务 ID 为必填项。",
    "Temporal Schedule ID must not contain whitespace.":
      "Temporal 定时任务 ID 不能包含空白符。",
    "Temporal Schedule ID must be 200 characters or fewer.":
      "Temporal 定时任务 ID 不能超过 200 个字符。",
    "Replay window must be less than or equal to cadence guard interval to avoid overlapping scheduled replay windows.":
      "重放窗口必须小于或等于周期保护间隔，以避免定时重放窗口重叠。",
    "This schedule can retain recurring proof by replaying bounded alert windows through the selected workflow policy.":
      "此定时任务可通过所选工作流策略重放有界告警窗口并保留周期性证明。",
    "Complete the schedule fields before relying on recurring replay proof.":
      "依赖周期性重放证明前，请完成定时任务字段。",
    "Resolve blocked policy or timing settings before scheduled-trigger proof can run.":
      "运行定时触发证明前，请解决已阻塞的策略或时间设置。",
  };
  if (exact[value] !== undefined) {
    return exact[value]!;
  }
  let match = value.match(/^Current user is not authorized to manage report workflow schedule #(\d+)\.$/);
  if (match) {
    return `当前用户无权管理报告工作流定时任务 #${match[1]}。`;
  }
  match = value.match(/^Every (.+)$/);
  if (match) {
    return `每 ${match[1]} 执行一次`;
  }
  match = value.match(/^Daily at (.+)$/);
  if (match) {
    return `每天 ${match[1]} 执行`;
  }
  match = value.match(/^Day (.+) at (.+)$/);
  if (match) {
    return `每月 ${match[1]} 日 ${match[2]} 执行`;
  }
  match = value.match(/^(Sunday|Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Weekday) at (.+)$/);
  if (match) {
    return `${localizeWorkflowScheduleText(match[1]!, locale)} ${match[2]} 执行`;
  }
  match = value.match(/^Starts every (.+) after offset (.+)\.$/);
  if (match) {
    return `每 ${match[1]} 启动一次，偏移 ${match[2]}。`;
  }
  match = value.match(/^(Daily|Weekly|Monthly) UTC calendar schedule with (.+) replay guard interval\.$/);
  if (match) {
    return `${localizeWorkflowScheduleText(match[1]!, locale)} UTC 日历定时任务，重放保护间隔为 ${match[2]}。`;
  }
  match = value.match(/^(.+) window$/);
  if (match) {
    return `${match[1]} 窗口`;
  }
  return value;
}
