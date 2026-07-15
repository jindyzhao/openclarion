"use client";

import {
  BranchesOutlined,
  EditOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  ToolOutlined
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  Row,
  Segmented,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography
} from "antd";
import type { TableColumnsType } from "antd";
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
import type { AlertSourceProfile, AlertSourceProfileListResponse } from "../alert-sources/types";
import {
  disableDiagnosisToolTemplateAction,
  enableDiagnosisToolTemplateAction,
  refreshDiagnosisToolTemplates,
  submitDiagnosisToolTemplate
} from "./client-api";
import {
  defaultFormForTool,
  diagnosisToolCoverage,
  diagnosisToolTemplateRecommendations,
  diagnosisToolSourceReadiness,
  diagnosisToolSaveCompatibility,
  diagnosisQueryTemplatePreview,
  diagnosisToolKindLabels,
  diagnosisToolSupportsSourceProfile,
  diagnosisToolTemplatePresetByID,
  diagnosisToolTemplatePresets,
  emptyDiagnosisToolTemplateForm,
  formStateToWriteRequest,
  presetToFormState,
  templateToFormState,
  type DiagnosisToolCoverage,
  type DiagnosisToolTemplateLaunchIntent,
  type DiagnosisToolTemplateWorkflowReturn
} from "./format";
import type {
  DiagnosisToolKind,
  DiagnosisToolTemplate,
  DiagnosisToolTemplateFormState,
  DiagnosisToolTemplateListResponse,
  DiagnosisToolTemplateWriteRequest
} from "./types";

type DiagnosisToolTemplateSettingsManagerProps = {
  alertSourcesResult: ApiResult<AlertSourceProfileListResponse>;
  launchIntent?: DiagnosisToolTemplateLaunchIntent | null;
  result: ApiResult<DiagnosisToolTemplateListResponse>;
};

const diagnosisToolTemplatesQueryKey = ["settings", "diagnosis-tool-templates"] as const;

type SaveTemplateVariables = {
  body: DiagnosisToolTemplateWriteRequest;
  templateID: number | null;
};

type DiagnosisToolTranslator = ReturnType<typeof useTranslations<"DiagnosisToolSettings">>;

const diagnosisToolTemplateBaseAuthorizationChecks: CurrentRBACAuthorizationCheck[] =
  [
    {
      key: "diagnosisToolTemplateRead",
      permission: "diagnosis_tool_template.read"
    },
    {
      key: "diagnosisToolTemplateManage",
      permission: "diagnosis_tool_template.manage"
    }
  ];

type EnablementVariables = {
  enabled: boolean;
  templateID: number;
};

type RelationSelectOption = {
  disabled?: boolean;
  label: string;
  title: string;
  value: number;
};

type RecommendationSelectOption = {
  label: string;
  title: string;
  value: string;
};

type ToolTemplateRelationOptions = {
  alertSources: AlertSourceProfile[];
  alertSourceLabels: Record<number, string>;
  warnings: string[];
};

type RecommendationSelectGroup = {
  label: string;
  options: RecommendationSelectOption[];
};

export function DiagnosisToolTemplateSettingsManager({
  alertSourcesResult,
  launchIntent = null,
  result
}: DiagnosisToolTemplateSettingsManagerProps) {
  const locale = useLocale();
  const t = useTranslations("DiagnosisToolSettings");
  const common = useTranslations("Common");
  const [form] = Form.useForm<DiagnosisToolTemplateFormState>();
  const clientReady = useClientReady();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [actionID, setActionID] = useState<number | null>(null);
  const [launchNotice, setLaunchNotice] = useState<string | null>(
    launchIntent?.message ?? null,
  );
  const {
    errorStatus,
    items: templates,
    notice,
    query,
    refresh,
    setNotice
  } = useSettingsList({
    initialResult: result,
    queryKey: diagnosisToolTemplatesQueryKey,
    queryFn: refreshDiagnosisToolTemplates,
    refreshMessage: t("refreshed"),
    selectItems: (response) => response.items
  });
  const saveTemplate = useSettingsMutation<SaveTemplateVariables, DiagnosisToolTemplate>({
    invalidateQueryKey: diagnosisToolTemplatesQueryKey,
    mutationFn: ({ templateID, body }) => submitDiagnosisToolTemplate(templateID, body)
  });
  const enablementAction = useSettingsMutation<EnablementVariables, DiagnosisToolTemplate>({
    invalidateQueryKey: diagnosisToolTemplatesQueryKey,
    mutationFn: ({ templateID, enabled }) =>
      enabled ? enableDiagnosisToolTemplateAction(templateID) : disableDiagnosisToolTemplateAction(templateID)
  });
  const authorizationChecks = useMemo(
    () => [
      ...diagnosisToolTemplateBaseAuthorizationChecks,
      ...templates.map((template) => ({
        key: diagnosisToolTemplateManageKey(template.id),
        permission: "diagnosis_tool_template.manage" as const,
        scopeKey: String(template.id),
        scopeKind: "diagnosis_tool_template" as const
      }))
    ],
    [templates]
  );
  const currentAuthorization = useCurrentRBACAuthorizations(
    authorizationChecks,
    clientReady
  );
  const busy =
    !clientReady ||
    currentAuthorization.isChecking ||
    query.isFetching ||
    saveTemplate.isPending ||
    enablementAction.isPending;
  const canReadTemplates = currentAuthorization.can("diagnosisToolTemplateRead");
  const canCreateTemplate = currentAuthorization.can("diagnosisToolTemplateManage");
  const canSaveCurrentTemplate =
    editingID === null
      ? canCreateTemplate
      : currentAuthorization.can(diagnosisToolTemplateManageKey(editingID));
  const formPermissionNotice = settingsManagePermissionNotice({
    canManage: canSaveCurrentTemplate,
    isChecking: !clientReady || currentAuthorization.isChecking,
    message: common("formReadOnly", {
      resource:
        editingID === null
          ? t("creationResource")
          : t("templateResource", { id: editingID }),
    }),
  });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadTemplates,
    errorStatus,
    isChecking: !clientReady || currentAuthorization.isChecking,
    message: common("readAccessLimited", {
      resource: t("templatesResource"),
    }),
  });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;
  const relationOptions = useMemo(
    () => buildToolTemplateRelationOptions(alertSourcesResult, locale),
    [alertSourcesResult, locale],
  );
  const initialFormValues = useMemo(
    () => diagnosisToolTemplateLaunchInitialForm(launchIntent, relationOptions),
    [launchIntent, relationOptions]
  );
  const coverage = useMemo(
    () => diagnosisToolCoverage({ sources: relationOptions.alertSources, templates }),
    [relationOptions.alertSources, templates]
  );
  const recommendations = useMemo(
    () => diagnosisToolTemplateRecommendations(relationOptions.alertSources),
    [relationOptions.alertSources]
  );
  const recommendationOptions = useMemo(
    () => recommendationSelectOptions(recommendations, locale),
    [locale, recommendations]
  );
  const selectedTool = Form.useWatch("tool", form) ?? "active_alerts";
  const selectedAlertSourceID = Form.useWatch("alertSourceProfileID", form) ?? null;
  const queryTemplate = Form.useWatch("queryTemplate", form) ?? "";
  const rangeTool = selectedTool === "metric_range_query";
  const queryTool = selectedTool !== "active_alerts";
  const queryPreview = useMemo(() => diagnosisQueryTemplatePreview(queryTemplate), [queryTemplate]);
  const sourceReadiness = useMemo(
    () =>
      diagnosisToolSourceReadiness({
        alertSourceProfileID: selectedAlertSourceID,
        sources: relationOptions.alertSources,
        tool: selectedTool
      }),
    [relationOptions.alertSources, selectedAlertSourceID, selectedTool]
  );
  const sourceOptions = useMemo(
    () => sourceOptionsForTool(selectedTool, relationOptions, locale),
    [locale, relationOptions, selectedTool]
  );
  const workflowReturn = launchIntent?.workflowReturn ?? null;

  const summary = useMemo(() => {
    const enabled = templates.filter((template) => template.enabled).length;
    const metrics = templates.filter((template) => template.tool !== "active_alerts").length;
    const range = templates.filter((template) => template.tool === "metric_range_query").length;
    return { enabled, metrics, range };
  }, [templates]);

  async function handleRefresh() {
    await refresh();
  }

  async function handleSubmit(values: DiagnosisToolTemplateFormState) {
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeDiagnosisToolText(parsed.message, locale) });
      return;
    }
    const saveCompatibility = diagnosisToolSaveCompatibility({
      alertSourceProfileID: values.alertSourceProfileID,
      sources: relationOptions.alertSources,
      tool: values.tool
    });
    if (!saveCompatibility.ok) {
      setNotice({ kind: "error", message: localizeDiagnosisToolText(saveCompatibility.message, locale) });
      return;
    }

    try {
      await saveTemplate.mutateAsync({ templateID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      return;
    }

    form.setFieldsValue(emptyDiagnosisToolTemplateForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({
      kind: "info",
      message:
        workflowReturn === null
          ? t("saved")
          : t("savedForWorkflow")
    });
  }

  async function handleEnablement(template: DiagnosisToolTemplate, enabled: boolean) {
    if (enabled) {
      const readiness = diagnosisToolSourceReadiness({
        alertSourceProfileID: template.alert_source_profile_id,
        sources: relationOptions.alertSources,
        tool: template.tool
      });
      if (readiness.status !== "ready") {
        setNotice({
          kind: "error",
          message: localizeDiagnosisToolMessages(
            readiness.blockers,
            readiness.detail,
            locale,
          ),
        });
        return;
      }
    }

    setActionID(template.id);
    try {
      await enablementAction.mutateAsync({ templateID: template.id, enabled });
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

  function editTemplate(template: DiagnosisToolTemplate) {
    setEditingID(template.id);
    form.setFieldsValue(templateToFormState(template));
    setLaunchNotice(null);
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyDiagnosisToolTemplateForm());
    setLaunchNotice(null);
    setNotice(null);
  }

  function handleValuesChange(changed: Partial<DiagnosisToolTemplateFormState>) {
    if (changed.tool === undefined) {
      return;
    }
    const currentSourceID = form.getFieldValue("alertSourceProfileID") ?? null;
    form.setFieldsValue({
      ...defaultFormForTool(changed.tool),
      alertSourceProfileID: compatibleSourceIDForTool(changed.tool, currentSourceID, relationOptions)
    });
  }

  function applyPreset(presetID: string) {
    const preset = diagnosisToolTemplatePresetByID(presetID);
    if (preset === null) {
      return;
    }
    const currentSourceID = form.getFieldValue("alertSourceProfileID") ?? null;
    form.setFieldsValue(
      presetToFormState(preset, compatibleSourceIDForTool(preset.form.tool, currentSourceID, relationOptions))
    );
    setLaunchNotice(null);
    setNotice(null);
  }

  function applyRecommendation(recommendationID: string) {
    const recommendation = recommendations.find((item) => item.id === recommendationID);
    if (recommendation === undefined) {
      return;
    }
    const preset = diagnosisToolTemplatePresetByID(recommendation.presetID);
    if (preset === null) {
      return;
    }
    form.setFieldsValue(presetToFormState(preset, recommendation.sourceID));
    setLaunchNotice(recommendation.detail);
    setNotice(null);
  }

  return (
    <div className="stack">
      <Row aria-label={t("metricsLabel")} gutter={[12, 12]}>
        <MetricCard label={t("templates")} value={templates.length} />
        <MetricCard label={t("enabled")} value={summary.enabled} />
        <MetricCard label={t("metricBacked")} value={summary.metrics} />
        <MetricCard label={t("range")} value={summary.range} />
      </Row>

      {launchNotice ? (
        <Alert
          aria-label={t("launchPreset")}
          description={localizeDiagnosisToolText(launchNotice, locale)}
          message={t("presetLoaded")}
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      {visibleNotice ? <Notice notice={visibleNotice} t={t} /> : null}
      <DiagnosisToolWorkflowReturnPanel locale={locale} t={t} workflowReturn={workflowReturn} />
      {relationOptions.warnings.length > 0 ? (
        <Alert
          description={localizeDiagnosisToolMessages(relationOptions.warnings, "", locale)}
          message={t("relatedUnavailable")}
          role="status"
          showIcon
          type="warning"
        />
      ) : null}
      <EvidenceCoveragePanel coverage={coverage} locale={locale} t={t} />

      <Row align="top" className="settings-console-grid" gutter={[16, 16]}>
        <Col lg={8} md={24} xs={24}>
          <Card
            extra={
              editingID === null ? null : (
                <Button disabled={busy || !canCreateTemplate} icon={<PlusOutlined />} onClick={resetForm} type="default">
                  {t("new")}
                </Button>
              )
            }
            title={editingID === null ? t("newTemplate") : t("editTemplate", { id: editingID })}
          >
            {formPermissionNotice ? (
              <ReadOnlyModeAlert notice={formPermissionNotice} />
            ) : null}
            <Form<DiagnosisToolTemplateFormState>
              disabled={busy || !canSaveCurrentTemplate}
              form={form}
              initialValues={initialFormValues}
              layout="vertical"
              onFinish={handleSubmit}
              onValuesChange={handleValuesChange}
            >
              <Form.Item label={t("preset")}>
                <Select
                  allowClear
                  onChange={(value: string | undefined) => {
                    if (value !== undefined) {
                      applyPreset(value);
                    }
                  }}
                  options={diagnosisToolTemplatePresets.map((preset) => ({
                    label: localizeDiagnosisToolText(preset.label, locale),
                    value: preset.id
                  }))}
                  placeholder={t("applyTemplate")}
                  value={undefined}
                />
              </Form.Item>

              <Form.Item label={t("recommendedBySources")}>
                <Select
                  allowClear
                  disabled={recommendationOptions.length === 0}
                  onChange={(value: string | undefined) => {
                    if (value !== undefined) {
                      applyRecommendation(value);
                    }
                  }}
                  optionFilterProp="label"
                  options={recommendationOptions}
                  placeholder={
                    recommendationOptions.length === 0
                      ? t("noCompatibleSources")
                      : t("applyRecommendation")
                  }
                  showSearch
                  value={undefined}
                />
              </Form.Item>

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
                label={t("alertSource")}
                name="alertSourceProfileID"
                rules={[{ required: true, message: t("sourceRequired") }]}
              >
                <Select
                  optionFilterProp="label"
                  options={sourceOptions}
                  placeholder={t("selectSource")}
                  showSearch
                />
              </Form.Item>

              <Form.Item label={t("tool")} name="tool" rules={[{ required: true, message: t("toolRequired") }]}>
                <Segmented
                  block
                  options={[
                    { value: "active_alerts", label: t("alerts") },
                    { value: "metric_query", label: t("instant") },
                    { value: "metric_range_query", label: t("range") }
                  ]}
                />
              </Form.Item>
              <SourceCompatibilityPreview locale={locale} readiness={sourceReadiness} t={t} />

              <Form.Item
                label={t("queryTemplate")}
                name="queryTemplate"
                rules={[
                  ...(queryTool ? [{ required: true, message: t("queryRequired") }] : []),
                  { max: 500, message: t("queryLength") }
                ]}
              >
                <Input.TextArea
                  autoSize={{ minRows: 3, maxRows: 6 }}
                  disabled={!queryTool}
                  placeholder="rate(container_cpu_usage_seconds_total[5m])"
                />
              </Form.Item>
              {queryTool && queryTemplate.trim() !== "" ? <QueryTemplatePreview locale={locale} preview={queryPreview} t={t} /> : null}

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("defaultLimit")}
                    name="defaultLimit"
                    rules={[{ required: true, message: t("defaultLimitRequired") }]}
                  >
                    <InputNumber max={selectedTool === "active_alerts" ? 10 : 20} min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("stepSeconds")}
                    name="defaultStepSeconds"
                    rules={[{ required: true, message: t("defaultStepRequired") }]}
                  >
                    <InputNumber disabled={!rangeTool} min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("defaultWindowSeconds")}
                    name="defaultWindowSeconds"
                    rules={[{ required: true, message: t("defaultWindowRequired") }]}
                  >
                    <InputNumber disabled={!rangeTool} min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("maxWindowSeconds")}
                    name="maxWindowSeconds"
                    rules={[{ required: true, message: t("maxWindowRequired") }]}
                  >
                    <InputNumber disabled={!rangeTool} min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Space wrap>
                <Button disabled={busy || !canSaveCurrentTemplate} htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
                  {t("saveTemplate")}
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
              <Button disabled={busy || !canReadTemplates} icon={<ReloadOutlined />} loading={busy} onClick={handleRefresh} type="default">
                {t("refresh")}
              </Button>
            }
            title={t("configuredTemplates")}
          >
            <DiagnosisToolTemplateTable
              actionID={actionID}
              busy={busy}
              canRead={canReadTemplates}
              canManageTemplate={(templateID) =>
                currentAuthorization.can(diagnosisToolTemplateManageKey(templateID))
              }
              onDisable={(template) => handleEnablement(template, false)}
              onEdit={editTemplate}
              onEnable={(template) => handleEnablement(template, true)}
              relationOptions={relationOptions}
              locale={locale}
              t={t}
              templates={templates}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
}

function diagnosisToolTemplateManageKey(templateID: number): string {
  return `diagnosisToolTemplateManage:${templateID}`;
}

function DiagnosisToolWorkflowReturnPanel({
  locale,
  t,
  workflowReturn
}: {
  locale: string;
  t: DiagnosisToolTranslator;
  workflowReturn: DiagnosisToolTemplateWorkflowReturn | null;
}) {
  if (workflowReturn === null) {
    return null;
  }
  return (
    <Alert
      action={
        <Button href={workflowReturn.href} icon={<BranchesOutlined />} type="primary">
          {localizeDiagnosisToolText(workflowReturn.label, locale)}
        </Button>
      }
      aria-label={t("workflowReturnLabel")}
      description={localizeDiagnosisToolText(workflowReturn.detail, locale)}
      message={t("workflowReturn")}
      role="status"
      showIcon
      type="info"
    />
  );
}

function EvidenceCoveragePanel({
  coverage,
  locale,
  t,
}: {
  coverage: DiagnosisToolCoverage;
  locale: string;
  t: DiagnosisToolTranslator;
}) {
  return (
    <Alert
      aria-label={t("coverageLabel")}
      description={
        <Space direction="vertical" size={8}>
          <Typography.Text>{localizeDiagnosisToolText(coverage.detail, locale)}</Typography.Text>
          <Space wrap>
            <Tag color="blue">{t("activeAlertCount", { count: coverage.activeAlertTemplates })}</Tag>
            <Tag color="cyan">{t("metricToolCount", { count: coverage.metricTemplates })}</Tag>
            <Tag color="purple">{t("rangeToolCount", { count: coverage.rangeMetricTemplates })}</Tag>
            <Tag>{t("enabledCount", { count: coverage.enabledTemplates })}</Tag>
          </Space>
          {coverage.sourceNames.length > 0 ? (
            <Space wrap>
              {coverage.sourceNames.slice(0, 4).map((name) => (
                <Tag key={name}>{name}</Tag>
              ))}
              {coverage.sourceNames.length > 4 ? <Tag>+{coverage.sourceNames.length - 4}</Tag> : null}
            </Space>
          ) : null}
        </Space>
      }
      message={
        <Space wrap>
          <Tag color={coverageStatusColor(coverage.status)}>{coverageStatusLabel(coverage.status, t)}</Tag>
          <Typography.Text strong>{localizeDiagnosisToolText(coverage.label, locale)}</Typography.Text>
        </Space>
      }
      showIcon
      type={coverageAlertType(coverage.status)}
    />
  );
}

function coverageStatusColor(status: DiagnosisToolCoverage["status"]): string {
  switch (status) {
    case "ready":
      return "green";
    case "review":
      return "gold";
    case "pending":
      return "default";
  }
}

function coverageStatusLabel(
  status: DiagnosisToolCoverage["status"],
  t: DiagnosisToolTranslator,
): string {
  switch (status) {
    case "ready":
      return t("ready");
    case "review":
      return t("review");
    case "pending":
      return t("pending");
  }
}

function coverageAlertType(status: DiagnosisToolCoverage["status"]) {
  switch (status) {
    case "ready":
      return "success";
    case "review":
      return "warning";
    case "pending":
      return "info";
  }
}

function QueryTemplatePreview({
  locale,
  preview,
  t,
}: {
  locale: string;
  preview: ReturnType<typeof diagnosisQueryTemplatePreview>;
  t: DiagnosisToolTranslator;
}) {
  return (
    <div aria-label={t("queryPreview")} className="settings-preview-panel">
      <Space direction="vertical" size={8}>
        <Space wrap>
          <Tag color={preview.ok ? "green" : "red"}>{preview.ok ? t("valid") : t("invalid")}</Tag>
          {preview.placeholders.length === 0 ? (
            <Tag color="default">{t("static")}</Tag>
          ) : (
            preview.placeholders.map((placeholder) => (
              <Tag color="geekblue" key={placeholder}>
                {placeholder}
              </Tag>
            ))
          )}
        </Space>
        {preview.ok ? (
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary">{t("exampleQuery")}</Typography.Text>
            <Typography.Text className="settings-query-preview" copyable>
              {preview.previewQuery}
            </Typography.Text>
          </Space>
        ) : (
          <Alert message={localizeDiagnosisToolText(preview.message, locale)} showIcon type="error" />
        )}
      </Space>
    </div>
  );
}

function SourceCompatibilityPreview({
  locale,
  readiness,
  t,
}: {
  locale: string;
  readiness: ReturnType<typeof diagnosisToolSourceReadiness>;
  t: DiagnosisToolTranslator;
}) {
  return (
    <div aria-label={t("sourceCompatibility")} className="settings-preview-panel">
      <Space direction="vertical" size={8}>
        <Space wrap>
          <Tag color={sourceReadinessColor(readiness.status)}>{localizeDiagnosisToolText(readiness.status, locale)}</Tag>
          <Tag>{t("compatibleSources", { count: readiness.compatibleSourceCount })}</Tag>
          {readiness.requiredKinds.map((kind) => (
            <Tag color={kind === "prometheus" ? "cyan" : "blue"} key={kind}>
              {kind}
            </Tag>
          ))}
        </Space>
        <Typography.Text strong>{localizeDiagnosisToolText(readiness.label, locale)}</Typography.Text>
        <Typography.Text type="secondary">{localizeDiagnosisToolText(readiness.detail, locale)}</Typography.Text>
        {readiness.status === "blocked" ? (
          <Button href="/settings/alert-sources" size="small" type="default">
            {t("openSources")}
          </Button>
        ) : null}
      </Space>
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

function Notice({ notice, t }: { notice: SettingsNotice; t: DiagnosisToolTranslator }) {
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

function buildToolTemplateRelationOptions(
  alertSourcesResult: ApiResult<AlertSourceProfileListResponse>,
  locale: string,
): ToolTemplateRelationOptions {
  if (!alertSourcesResult.ok) {
    return {
      alertSources: [],
      alertSourceLabels: {},
      warnings: [locale === "zh-CN"
        ? `告警源加载失败：${alertSourcesResult.error.message}。`
        : `Alert sources failed to load: ${alertSourcesResult.error.message}.`]
    };
  }
  return {
    alertSources: alertSourcesResult.data.items,
    alertSourceLabels: Object.fromEntries(alertSourcesResult.data.items.map((source) => [source.id, alertSourceLabel(source, locale)])),
    warnings: []
  };
}

function diagnosisToolTemplateLaunchInitialForm(
  launchIntent: DiagnosisToolTemplateLaunchIntent | null,
  relationOptions: ToolTemplateRelationOptions
): DiagnosisToolTemplateFormState {
  if (launchIntent === null) {
    return emptyDiagnosisToolTemplateForm();
  }
  const preset = diagnosisToolTemplatePresetByID(launchIntent.presetID);
  if (preset === null) {
    return emptyDiagnosisToolTemplateForm();
  }
  return presetToFormState(
    preset,
    launchSourceIDForTool(preset.form.tool, launchIntent.alertSourceProfileID, relationOptions)
  );
}

function launchSourceIDForTool(
  tool: DiagnosisToolKind,
  sourceID: number | null,
  relationOptions: ToolTemplateRelationOptions
): number | null {
  const requestedSourceID = compatibleSourceIDForTool(tool, sourceID, relationOptions);
  if (requestedSourceID !== null) {
    return requestedSourceID;
  }
  const enabledSource = relationOptions.alertSources.find(
    (source) => source.enabled && diagnosisToolSupportsSourceProfile(tool, source)
  );
  if (enabledSource !== undefined) {
    return enabledSource.id;
  }
  return (
    relationOptions.alertSources.find((source) => diagnosisToolSupportsSourceProfile(tool, source))?.id ?? null
  );
}

function sourceOptionsForTool(
  tool: DiagnosisToolKind,
  relationOptions: ToolTemplateRelationOptions,
  locale: string,
): RelationSelectOption[] {
  return relationOptions.alertSources.map((source) => {
    const compatible = diagnosisToolSupportsSourceProfile(tool, source);
    const label = alertSourceLabel(source, locale);
    return {
      disabled: !compatible,
      label: compatible ? label : locale === "zh-CN" ? `${label} - 不兼容` : `${label} - incompatible`,
      title: compatible ? label : locale === "zh-CN"
        ? `${label} 与${localizeDiagnosisToolText(diagnosisToolKindLabels[tool], locale)}不兼容`
        : `${label} is not compatible with ${diagnosisToolKindLabels[tool]}`,
      value: source.id
    };
  });
}

function recommendationSelectOptions(
  recommendations: ReturnType<typeof diagnosisToolTemplateRecommendations>,
  locale: string,
): RecommendationSelectGroup[] {
  const groups = new Map<string, RecommendationSelectOption[]>();
  recommendations.forEach((recommendation) => {
    const options = groups.get(recommendation.group) ?? [];
    options.push({
      label: `${localizeDiagnosisToolText(recommendation.label, locale)} - ${recommendation.sourceName}`,
      title: localizeDiagnosisToolText(recommendation.detail, locale),
      value: recommendation.id
    });
    groups.set(recommendation.group, options);
  });
  return Array.from(groups.entries()).map(([label, options]) => ({
    label: localizeDiagnosisToolText(label, locale),
    options
  }));
}

function compatibleSourceIDForTool(
  tool: DiagnosisToolKind,
  sourceID: number | null,
  relationOptions: ToolTemplateRelationOptions
): number | null {
  if (sourceID === null) {
    return null;
  }
  const source = relationOptions.alertSources.find((candidate) => candidate.id === sourceID);
  if (source === undefined || !diagnosisToolSupportsSourceProfile(tool, source)) {
    return null;
  }
  return sourceID;
}

function alertSourceLabel(source: AlertSourceProfile, locale: string): string {
  return `#${source.id} ${source.name} (${source.kind}, ${enabledLabel(source.enabled, locale)})`;
}

function enabledLabel(enabled: boolean, locale: string): string {
  if (locale === "zh-CN") {
    return enabled ? "已启用" : "已停用";
  }
  return enabled ? "enabled" : "disabled";
}

function relationLabel(labels: Record<number, string>, id: number, fallback: string): string {
  return labels[id] ?? fallback;
}

type DiagnosisToolTemplateTableProps = {
  actionID: number | null;
  busy: boolean;
  canRead: boolean;
  canManageTemplate: (templateID: number) => boolean;
  onDisable: (template: DiagnosisToolTemplate) => void;
  onEdit: (template: DiagnosisToolTemplate) => void;
  onEnable: (template: DiagnosisToolTemplate) => void;
  relationOptions: ToolTemplateRelationOptions;
  locale: string;
  t: DiagnosisToolTranslator;
  templates: DiagnosisToolTemplate[];
};

function DiagnosisToolTemplateTable({
  actionID,
  busy,
  canRead,
  canManageTemplate,
  onDisable,
  onEdit,
  onEnable,
  relationOptions,
  locale,
  t,
  templates
}: DiagnosisToolTemplateTableProps) {
  const common = useTranslations("Common");
  const columns: TableColumnsType<DiagnosisToolTemplate> = [
    {
      key: "name",
      title: t("name"),
      render: (_, template) => (
        <Space direction="vertical" size={2}>
          <Typography.Text strong>{template.name}</Typography.Text>
          <Typography.Text type="secondary">
            {relationLabel(
              relationOptions.alertSourceLabels,
              template.alert_source_profile_id,
              t("sourceNumber", { id: template.alert_source_profile_id })
            )}
          </Typography.Text>
          {template.query_template === "" ? null : (
            <Typography.Text className="settings-event-ids" type="secondary">
              {template.query_template}
            </Typography.Text>
          )}
        </Space>
      )
    },
    {
      dataIndex: "tool",
      key: "tool",
      title: t("tool"),
      render: (tool: DiagnosisToolKind) => <Tag color={toolTagColor(tool)}>{localizeDiagnosisToolText(diagnosisToolKindLabels[tool], locale)}</Tag>
    },
    {
      key: "bounds",
      title: t("bounds"),
      render: (_, template) => (
        <Space direction="vertical" size={2}>
          <Typography.Text>{t("limit", { value: template.default_limit })}</Typography.Text>
          {template.tool === "metric_range_query" ? (
            <Typography.Text type="secondary">
              {t("windowStep", {
                step: formatDurationSeconds(template.default_step_seconds),
                window: formatDurationSeconds(template.default_window_seconds),
              })}
            </Typography.Text>
          ) : (
            <Typography.Text type="secondary">{template.tool === "active_alerts" ? t("activeAlerts") : t("instantQuery")}</Typography.Text>
          )}
        </Space>
      )
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: t("state"),
      render: (enabled: boolean, template) => (
        <Space direction="vertical" size={2}>
          <Tag color={enabled ? "green" : "default"}>{enabled ? t("enabled") : t("draft")}</Tag>
          <Typography.Text type="secondary">
            {enabled ? nullableDate(template.enabled_at, locale) : nullableDate(template.disabled_at, locale)}
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
      render: (_, template) => {
        const canManage = canManageTemplate(template.id);
        return (
          <Space wrap>
            <Button
              disabled={busy || actionID !== null || !canManage}
              icon={<EditOutlined />}
              onClick={() => onEdit(template)}
              size="small"
            >
              {t("edit")}
            </Button>
            {template.enabled ? (
              <Button
                disabled={busy || actionID !== null || !canManage}
                icon={<PauseCircleOutlined />}
                loading={actionID === template.id}
                onClick={() => onDisable(template)}
                size="small"
              >
                {t("disable")}
              </Button>
            ) : (
              <EnableTemplateButton
                actionID={actionID}
                busy={busy}
                canManage={canManage}
                onEnable={onEnable}
                relationOptions={relationOptions}
                locale={locale}
                t={t}
                template={template}
              />
            )}
          </Space>
        );
      },
      title: t("actions")
    }
  ];

  return (
    <Table<DiagnosisToolTemplate>
      columns={columns}
      dataSource={templates}
      loading={busy}
      locale={{
        emptyText: (
          <Empty
            description={settingsReadPermissionEmptyDescription({
              canRead,
              deniedDescription: common("noReadAccess", {
                resource: t("templatesResource"),
              }),
              emptyDescription: t("noTemplates"),
            })}
            image={<ToolOutlined aria-hidden className="settings-empty-icon" />}
          />
        )
      }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1080 }}
    />
  );
}

function EnableTemplateButton({
  actionID,
  busy,
  canManage,
  locale,
  onEnable,
  relationOptions,
  t,
  template
}: {
  actionID: number | null;
  busy: boolean;
  canManage: boolean;
  locale: string;
  onEnable: (template: DiagnosisToolTemplate) => void;
  relationOptions: ToolTemplateRelationOptions;
  t: DiagnosisToolTranslator;
  template: DiagnosisToolTemplate;
}) {
  const readiness = diagnosisToolSourceReadiness({
    alertSourceProfileID: template.alert_source_profile_id,
    sources: relationOptions.alertSources,
    tool: template.tool
  });
  const blocked = readiness.status !== "ready";
  const button = (
    <Button
      disabled={busy || actionID !== null || blocked || !canManage}
      icon={<PlayCircleOutlined />}
      loading={actionID === template.id}
      onClick={() => onEnable(template)}
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
    <Tooltip
      title={localizeDiagnosisToolMessages(
        readiness.blockers,
        readiness.detail,
        locale,
      )}
    >
      <span>{button}</span>
    </Tooltip>
  );
}

function toolTagColor(tool: DiagnosisToolKind): string {
  switch (tool) {
    case "active_alerts":
      return "blue";
    case "metric_query":
      return "cyan";
    case "metric_range_query":
      return "purple";
  }
}

function sourceReadinessColor(status: ReturnType<typeof diagnosisToolSourceReadiness>["status"]): string {
  switch (status) {
    case "ready":
      return "green";
    case "pending":
      return "gold";
    case "blocked":
      return "red";
  }
}

function nullableDate(value: string | null, locale: string): string {
  return value === null ? "-" : formatDateTime(value, locale);
}

function localizeDiagnosisToolMessages(
  messages: readonly string[],
  fallback: string,
  locale: string,
): string {
  const values = messages.length > 0 ? messages : [fallback];
  return values
    .filter((value) => value !== "")
    .map((value) => localizeDiagnosisToolText(value, locale))
    .join(" ");
}

function localizeDiagnosisToolText(value: string, locale: string): string {
  if (locale !== "zh-CN") {
    return value;
  }
  const exact: Readonly<Record<string, string>> = {
    blocked: "已阻塞",
    disabled: "已停用",
    enabled: "已启用",
    pending: "等待中",
    ready: "就绪",
    review: "需检查",
    "Active alerts": "活动告警",
    "active alerts": "活动告警",
    "AI follow-up has active alert collection and metric evidence tools.":
      "AI 后续诊断已具备活动告警采集和指标证据工具。",
    "Active alert collection can read Alertmanager or Prometheus-compatible alert APIs.":
      "活动告警采集可读取 Alertmanager 或 Prometheus 兼容的告警 API。",
    "Active alert templates must not include a query, windows, or step.":
      "活动告警模板不能包含查询、窗口或步长。",
    "Active alert templates support a default limit between 1 and 10.":
      "活动告警模板的默认条数限制必须在 1 到 10 之间。",
    "Alertmanager": "Alertmanager",
    "Alertmanager or Prometheus-compatible": "Alertmanager 或 Prometheus 兼容告警源",
    "Alertmanager active-alert intake": "Alertmanager 活动告警接入",
    "Back to workflow": "返回工作流",
    "Bound alert source must be enabled before template enablement.":
      "启用模板前，必须先启用绑定的告警源。",
    "Current active alerts": "当前活动告警",
    "Default limit must be a positive integer.": "默认条数限制必须是正整数。",
    "Default step": "默认步长",
    "Default step must be between 15 seconds and the default window.":
      "默认步长必须在 15 秒与默认窗口之间。",
    "Default window must be between 15 and 21600 seconds.":
      "默认窗口必须在 15 到 21600 秒之间。",
    "Default window": "默认窗口",
    "Enable active alert and metric templates before relying on AI follow-up.":
      "依赖 AI 后续诊断前，请先启用活动告警和指标模板。",
    "Evidence coverage needs review.": "证据覆盖需要检查。",
    "Evidence coverage ready.": "证据覆盖已就绪。",
    "Instant metric": "即时指标",
    "Instant metric templates must not include windows or step.":
      "即时指标模板不能包含窗口或步长。",
    "Kubernetes JVM heap usage pct range": "Kubernetes JVM 堆使用率范围",
    "Kubernetes JVM heap used range": "Kubernetes JVM 已用堆范围",
    "Kubernetes pod CPU range": "Kubernetes Pod CPU 范围",
    "Kubernetes pod memory range": "Kubernetes Pod 内存范围",
    "Kubernetes pod restarts range": "Kubernetes Pod 重启次数范围",
    "Max window must be between 15 and 21600 seconds.":
      "最大窗口必须在 15 到 21600 秒之间。",
    "Max window": "最大窗口",
    "Max window must be greater than or equal to default window.":
      "最大窗口必须大于或等于默认窗口。",
    "Metric evidence runs against Prometheus-compatible query APIs, including Thanos Query endpoints; Thanos Rule active-alert sources are excluded.":
      "指标证据使用 Prometheus 兼容查询 API，包括 Thanos Query 端点；不包含 Thanos Rule 活动告警源。",
    "metric evidence": "指标证据",
    "Metric query templates require a query template.": "指标查询模板需要查询模板。",
    "Metric templates support a default limit between 1 and 20.":
      "指标模板的默认条数限制必须在 1 到 20 之间。",
    "No enabled compatible alert source.": "没有已启用的兼容告警源。",
    "No enabled evidence tools.": "没有已启用的证据工具。",
    "No query template.": "没有查询模板。",
    "Oracle tablespace pct used instant": "Oracle 表空间使用率即时查询",
    "Oracle tablespace pct used range": "Oracle 表空间使用率范围查询",
    "Parameterized query template.": "参数化查询模板。",
    "Placeholders must use {{label.NAME}} or {{annotation.NAME}} inside quoted PromQL label values.":
      "占位符必须在带引号的 PromQL 标签值中使用 {{label.NAME}} 或 {{annotation.NAME}}。",
    "Prometheus metric evidence": "Prometheus 指标证据",
    "Prometheus-compatible": "Prometheus 兼容",
    "Prometheus-compatible or Alertmanager": "Prometheus 兼容告警源或 Alertmanager",
    "Prometheus-compatible metric evidence": "Prometheus 兼容指标证据",
    "Query template must be 500 characters or fewer.": "查询模板不能超过 500 个字符。",
    "Query template must be a single line.": "查询模板必须为单行。",
    "Range field": "范围字段",
    "Range metric": "范围指标",
    "Range metric templates require a query template.": "范围指标模板需要查询模板。",
    "Select a compatible alert source.": "请选择兼容的告警源。",
    "Select a compatible source.": "请选择兼容的告警源。",
    "Select an alert source.": "请选择告警源。",
    "Selected source is disabled.": "所选告警源已停用。",
    "Selected source is incompatible.": "所选告警源不兼容。",
    "Source compatible.": "告警源兼容。",
    "Static query template.": "静态查询模板。",
    "Target availability instant": "目标可用性即时查询",
    "Template name is required.": "模板名称为必填项。",
    "Template name must be 120 characters or fewer.": "模板名称不能超过 120 个字符。",
    "Prepared Kubernetes pod CPU range from the settings overview action.":
      "已根据配置概览操作准备 Kubernetes Pod CPU 范围模板。",
    "Prepared current active alerts from the settings overview action.":
      "已根据配置概览操作准备当前活动告警模板。",
    "Return to workflow policies after the required evidence templates are saved and enabled.":
      "保存并启用所需证据模板后，请返回工作流策略。",
    "Thanos Query metric evidence": "Thanos Query 指标证据",
    "Thanos Rule active alerts": "Thanos Rule 活动告警",
    "Tool kind is unsupported.": "不支持此工具类型。",
  };
  if (exact[value] !== undefined) {
    return exact[value]!;
  }
  let match = value.match(/^Prepared (.+) from the URL preset\.$/);
  if (match) {
    return `已根据 URL 预设准备${localizeDiagnosisToolText(match[1]!, locale)}。`;
  }
  match = value.match(/^Missing (.+) for AI follow-up\.$/);
  if (match) {
    return `AI 后续诊断缺少${match[1]!
      .split(" and ")
      .map((item) => localizeDiagnosisToolText(item, locale))
      .join("和")}。`;
  }
  match = value.match(/^Select (.+) for this tool\.$/);
  if (match) {
    return `请为此工具选择${localizeDiagnosisToolText(match[1]!, locale)}。`;
  }
  match = value.match(/^Alert sources failed to load: (.+)\.$/);
  if (match) {
    return `告警源加载失败：${match[1]}。`;
  }
  match = value.match(/^(.+) must be a non-negative integer\.$/);
  if (match) {
    return `${localizeDiagnosisToolText(match[1]!, locale)}必须是非负整数。`;
  }
  match = value.match(/^(.+) cannot run against Thanos Rule active-alert sources\. Use a Thanos Query or Prometheus metric source\.$/);
  if (match) {
    return `${localizeDiagnosisToolText(match[1]!, locale)}不能针对 Thanos Rule 活动告警源运行。请使用 Thanos Query 或 Prometheus 指标源。`;
  }
  match = value.match(/^(.+) cannot run against (.+)\.$/);
  if (match) {
    return `${localizeDiagnosisToolText(match[1]!, locale)}不能针对${localizeDiagnosisToolText(match[2]!, locale)}运行。`;
  }
  return value;
}
