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
  const [form] = Form.useForm<DiagnosisToolTemplateFormState>();
  const clientReady = useClientReady();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [actionID, setActionID] = useState<number | null>(null);
  const [launchNotice, setLaunchNotice] = useState<string | null>(launchIntent?.message ?? null);
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
    refreshMessage: "Templates refreshed.",
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
    resourceLabel:
      editingID === null
        ? "diagnosis tool template creation"
        : `diagnosis tool template #${editingID}`,
  });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadTemplates,
    errorStatus,
    isChecking: !clientReady || currentAuthorization.isChecking,
    resourceLabel: "diagnosis tool templates",
  });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;
  const relationOptions = useMemo(() => buildToolTemplateRelationOptions(alertSourcesResult), [alertSourcesResult]);
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
    () => recommendationSelectOptions(recommendations),
    [recommendations]
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
    () => sourceOptionsForTool(selectedTool, relationOptions),
    [relationOptions, selectedTool]
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
      setNotice({ kind: "error", message: parsed.message });
      return;
    }
    const saveCompatibility = diagnosisToolSaveCompatibility({
      alertSourceProfileID: values.alertSourceProfileID,
      sources: relationOptions.alertSources,
      tool: values.tool
    });
    if (!saveCompatibility.ok) {
      setNotice({ kind: "error", message: saveCompatibility.message });
      return;
    }

    try {
      await saveTemplate.mutateAsync({ templateID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }

    form.setFieldsValue(emptyDiagnosisToolTemplateForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({
      kind: "info",
      message:
        workflowReturn === null
          ? "Template saved."
          : "Template saved. Enable the required evidence templates before returning to workflow enablement."
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
        setNotice({ kind: "error", message: readiness.blockers.join(" ") || readiness.detail });
        return;
      }
    }

    setActionID(template.id);
    try {
      await enablementAction.mutateAsync({ templateID: template.id, enabled });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      setActionID(null);
      return;
    }
    setActionID(null);
    setNotice({ kind: enabled ? "info" : "warning", message: enabled ? "Template enabled." : "Template disabled." });
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
      <Row aria-label="Diagnosis tool template metrics" gutter={[12, 12]}>
        <MetricCard label="Templates" value={templates.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Metric-backed" value={summary.metrics} />
        <MetricCard label="Range" value={summary.range} />
      </Row>

      {launchNotice ? (
        <Alert
          aria-label="Diagnosis tool launch preset"
          description={launchNotice}
          message="Preset loaded"
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      {visibleNotice ? <Notice notice={visibleNotice} /> : null}
      <DiagnosisToolWorkflowReturnPanel workflowReturn={workflowReturn} />
      {relationOptions.warnings.length > 0 ? (
        <Alert
          description={relationOptions.warnings.join(" ")}
          message="Related configuration unavailable"
          role="status"
          showIcon
          type="warning"
        />
      ) : null}
      <EvidenceCoveragePanel coverage={coverage} />

      <Row align="top" className="settings-console-grid" gutter={[16, 16]}>
        <Col lg={8} md={24} xs={24}>
          <Card
            extra={
              editingID === null ? null : (
                <Button disabled={busy || !canCreateTemplate} icon={<PlusOutlined />} onClick={resetForm} type="default">
                  New
                </Button>
              )
            }
            title={editingID === null ? "New Tool Template" : `Edit Template #${editingID}`}
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
              <Form.Item label="Preset">
                <Select
                  allowClear
                  onChange={(value: string | undefined) => {
                    if (value !== undefined) {
                      applyPreset(value);
                    }
                  }}
                  options={diagnosisToolTemplatePresets.map((preset) => ({
                    label: preset.label,
                    value: preset.id
                  }))}
                  placeholder="Apply a standard template"
                  value={undefined}
                />
              </Form.Item>

              <Form.Item label="Recommended by sources">
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
                      ? "No enabled compatible sources"
                      : "Apply a source-aware recommendation"
                  }
                  showSearch
                  value={undefined}
                />
              </Form.Item>

              <Form.Item
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Template name is required." },
                  { max: 120, message: "Template name must be 120 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item
                label="Alert source"
                name="alertSourceProfileID"
                rules={[{ required: true, message: "Alert source is required." }]}
              >
                <Select
                  optionFilterProp="label"
                  options={sourceOptions}
                  placeholder="Select alert source"
                  showSearch
                />
              </Form.Item>

              <Form.Item label="Tool" name="tool" rules={[{ required: true, message: "Tool is required." }]}>
                <Segmented
                  block
                  options={[
                    { value: "active_alerts", label: "Alerts" },
                    { value: "metric_query", label: "Instant" },
                    { value: "metric_range_query", label: "Range" }
                  ]}
                />
              </Form.Item>
              <SourceCompatibilityPreview readiness={sourceReadiness} />

              <Form.Item
                label="Query template"
                name="queryTemplate"
                rules={[
                  ...(queryTool ? [{ required: true, message: "Query template is required." }] : []),
                  { max: 500, message: "Query template must be 500 characters or fewer." }
                ]}
              >
                <Input.TextArea
                  autoSize={{ minRows: 3, maxRows: 6 }}
                  disabled={!queryTool}
                  placeholder="rate(container_cpu_usage_seconds_total[5m])"
                />
              </Form.Item>
              {queryTool && queryTemplate.trim() !== "" ? <QueryTemplatePreview preview={queryPreview} /> : null}

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Default limit"
                    name="defaultLimit"
                    rules={[{ required: true, message: "Default limit is required." }]}
                  >
                    <InputNumber max={selectedTool === "active_alerts" ? 10 : 20} min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Step seconds"
                    name="defaultStepSeconds"
                    rules={[{ required: true, message: "Default step is required." }]}
                  >
                    <InputNumber disabled={!rangeTool} min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Default window seconds"
                    name="defaultWindowSeconds"
                    rules={[{ required: true, message: "Default window is required." }]}
                  >
                    <InputNumber disabled={!rangeTool} min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Max window seconds"
                    name="maxWindowSeconds"
                    rules={[{ required: true, message: "Max window is required." }]}
                  >
                    <InputNumber disabled={!rangeTool} min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Space wrap>
                <Button disabled={busy || !canSaveCurrentTemplate} htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
                  Save Template
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
              <Button disabled={busy || !canReadTemplates} icon={<ReloadOutlined />} loading={busy} onClick={handleRefresh} type="default">
                Refresh
              </Button>
            }
            title="Configured Tool Templates"
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
  workflowReturn
}: {
  workflowReturn: DiagnosisToolTemplateWorkflowReturn | null;
}) {
  if (workflowReturn === null) {
    return null;
  }
  return (
    <Alert
      action={
        <Button href={workflowReturn.href} icon={<BranchesOutlined />} type="primary">
          {workflowReturn.label}
        </Button>
      }
      aria-label="Diagnosis tool workflow return"
      description={workflowReturn.detail}
      message="Workflow return"
      role="status"
      showIcon
      type="info"
    />
  );
}

function EvidenceCoveragePanel({ coverage }: { coverage: DiagnosisToolCoverage }) {
  return (
    <Alert
      aria-label="AI evidence tool coverage"
      description={
        <Space direction="vertical" size={8}>
          <Typography.Text>{coverage.detail}</Typography.Text>
          <Space wrap>
            <Tag color="blue">Active alerts {coverage.activeAlertTemplates}</Tag>
            <Tag color="cyan">Metric tools {coverage.metricTemplates}</Tag>
            <Tag color="purple">Range tools {coverage.rangeMetricTemplates}</Tag>
            <Tag>Enabled {coverage.enabledTemplates}</Tag>
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
          <Tag color={coverageStatusColor(coverage.status)}>{coverageStatusLabel(coverage.status)}</Tag>
          <Typography.Text strong>{coverage.label}</Typography.Text>
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

function coverageStatusLabel(status: DiagnosisToolCoverage["status"]): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "review":
      return "Review";
    case "pending":
      return "Pending";
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

function QueryTemplatePreview({ preview }: { preview: ReturnType<typeof diagnosisQueryTemplatePreview> }) {
  return (
    <div aria-label="Query template preview" className="settings-preview-panel">
      <Space direction="vertical" size={8}>
        <Space wrap>
          <Tag color={preview.ok ? "green" : "red"}>{preview.ok ? "Valid" : "Invalid"}</Tag>
          {preview.placeholders.length === 0 ? (
            <Tag color="default">Static</Tag>
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
            <Typography.Text type="secondary">Example query</Typography.Text>
            <Typography.Text className="settings-query-preview" copyable>
              {preview.previewQuery}
            </Typography.Text>
          </Space>
        ) : (
          <Alert message={preview.message} showIcon type="error" />
        )}
      </Space>
    </div>
  );
}

function SourceCompatibilityPreview({
  readiness
}: {
  readiness: ReturnType<typeof diagnosisToolSourceReadiness>;
}) {
  return (
    <div aria-label="Source compatibility" className="settings-preview-panel">
      <Space direction="vertical" size={8}>
        <Space wrap>
          <Tag color={sourceReadinessColor(readiness.status)}>{readiness.status}</Tag>
          <Tag>{readiness.compatibleSourceCount} compatible source(s)</Tag>
          {readiness.requiredKinds.map((kind) => (
            <Tag color={kind === "prometheus" ? "cyan" : "blue"} key={kind}>
              {kind}
            </Tag>
          ))}
        </Space>
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
        {readiness.status === "blocked" ? (
          <Button href="/settings/alert-sources" size="small" type="default">
            Open Alert Sources
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

function buildToolTemplateRelationOptions(
  alertSourcesResult: ApiResult<AlertSourceProfileListResponse>
): ToolTemplateRelationOptions {
  if (!alertSourcesResult.ok) {
    return {
      alertSources: [],
      alertSourceLabels: {},
      warnings: [`Alert sources failed to load: ${alertSourcesResult.error.message}.`]
    };
  }
  return {
    alertSources: alertSourcesResult.data.items,
    alertSourceLabels: Object.fromEntries(alertSourcesResult.data.items.map((source) => [source.id, alertSourceLabel(source)])),
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
  relationOptions: ToolTemplateRelationOptions
): RelationSelectOption[] {
  return relationOptions.alertSources.map((source) => {
    const compatible = diagnosisToolSupportsSourceProfile(tool, source);
    const label = alertSourceLabel(source);
    return {
      disabled: !compatible,
      label: compatible ? label : `${label} - incompatible`,
      title: compatible ? label : `${label} is not compatible with ${diagnosisToolKindLabels[tool]}`,
      value: source.id
    };
  });
}

function recommendationSelectOptions(
  recommendations: ReturnType<typeof diagnosisToolTemplateRecommendations>
): RecommendationSelectGroup[] {
  const groups = new Map<string, RecommendationSelectOption[]>();
  recommendations.forEach((recommendation) => {
    const options = groups.get(recommendation.group) ?? [];
    options.push({
      label: `${recommendation.label} - ${recommendation.sourceName}`,
      title: recommendation.detail,
      value: recommendation.id
    });
    groups.set(recommendation.group, options);
  });
  return Array.from(groups.entries()).map(([label, options]) => ({
    label,
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

function alertSourceLabel(source: AlertSourceProfile): string {
  return `#${source.id} ${source.name} (${source.kind}, ${enabledLabel(source.enabled)})`;
}

function enabledLabel(enabled: boolean): string {
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
  templates
}: DiagnosisToolTemplateTableProps) {
  const columns: TableColumnsType<DiagnosisToolTemplate> = [
    {
      key: "name",
      title: "Name",
      render: (_, template) => (
        <Space direction="vertical" size={2}>
          <Typography.Text strong>{template.name}</Typography.Text>
          <Typography.Text type="secondary">
            {relationLabel(
              relationOptions.alertSourceLabels,
              template.alert_source_profile_id,
              `Source #${template.alert_source_profile_id}`
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
      title: "Tool",
      render: (tool: DiagnosisToolKind) => <Tag color={toolTagColor(tool)}>{diagnosisToolKindLabels[tool]}</Tag>
    },
    {
      key: "bounds",
      title: "Bounds",
      render: (_, template) => (
        <Space direction="vertical" size={2}>
          <Typography.Text>limit {template.default_limit}</Typography.Text>
          {template.tool === "metric_range_query" ? (
            <Typography.Text type="secondary">
              {formatDurationSeconds(template.default_window_seconds)} window / {formatDurationSeconds(template.default_step_seconds)} step
            </Typography.Text>
          ) : (
            <Typography.Text type="secondary">{template.tool === "active_alerts" ? "active alerts" : "instant query"}</Typography.Text>
          )}
        </Space>
      )
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: "State",
      render: (enabled: boolean, template) => (
        <Space direction="vertical" size={2}>
          <Tag color={enabled ? "green" : "default"}>{enabled ? "Enabled" : "Draft"}</Tag>
          <Typography.Text type="secondary">
            {enabled ? nullableDate(template.enabled_at) : nullableDate(template.disabled_at)}
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
              Edit
            </Button>
            {template.enabled ? (
              <Button
                disabled={busy || actionID !== null || !canManage}
                icon={<PauseCircleOutlined />}
                loading={actionID === template.id}
                onClick={() => onDisable(template)}
                size="small"
              >
                Disable
              </Button>
            ) : (
              <EnableTemplateButton
                actionID={actionID}
                busy={busy}
                canManage={canManage}
                onEnable={onEnable}
                relationOptions={relationOptions}
                template={template}
              />
            )}
          </Space>
        );
      },
      title: "Actions"
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
              emptyDescription: "No diagnosis tool templates",
              resourceLabel: "diagnosis tool templates",
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
  onEnable,
  relationOptions,
  template
}: {
  actionID: number | null;
  busy: boolean;
  canManage: boolean;
  onEnable: (template: DiagnosisToolTemplate) => void;
  relationOptions: ToolTemplateRelationOptions;
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
      Enable
    </Button>
  );
  if (!blocked) {
    return button;
  }
  return (
    <Tooltip title={readiness.blockers.join(" ") || readiness.detail}>
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

function nullableDate(value: string | null): string {
  return value === null ? "-" : formatDateTime(value);
}
