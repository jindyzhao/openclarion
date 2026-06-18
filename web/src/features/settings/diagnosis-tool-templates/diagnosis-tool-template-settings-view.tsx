"use client";

import {
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
  Typography
} from "antd";
import type { TableColumnsType } from "antd";
import { useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

import { formatDateTime, formatDurationSeconds } from "../format";
import {
  settingsErrorMessage,
  type SettingsNotice,
  useSettingsList,
  useSettingsMutation
} from "../query-state";
import type { AlertSourceProfile, AlertSourceProfileListResponse } from "../alert-sources/types";
import {
  disableDiagnosisToolTemplateAction,
  enableDiagnosisToolTemplateAction,
  refreshDiagnosisToolTemplates,
  submitDiagnosisToolTemplate
} from "./client-api";
import {
  defaultFormForTool,
  diagnosisToolSourceReadiness,
  diagnosisQueryTemplatePreview,
  diagnosisToolKindLabels,
  diagnosisToolSupportsSourceKind,
  diagnosisToolTemplatePresets,
  emptyDiagnosisToolTemplateForm,
  formStateToWriteRequest,
  presetToFormState,
  templateToFormState
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
  result: ApiResult<DiagnosisToolTemplateListResponse>;
};

const diagnosisToolTemplatesQueryKey = ["settings", "diagnosis-tool-templates"] as const;

type SaveTemplateVariables = {
  body: DiagnosisToolTemplateWriteRequest;
  templateID: number | null;
};

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

type ToolTemplateRelationOptions = {
  alertSources: AlertSourceProfile[];
  alertSourceLabels: Record<number, string>;
  warnings: string[];
};

export function DiagnosisToolTemplateSettingsManager({
  alertSourcesResult,
  result
}: DiagnosisToolTemplateSettingsManagerProps) {
  const [form] = Form.useForm<DiagnosisToolTemplateFormState>();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [actionID, setActionID] = useState<number | null>(null);
  const {
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
  const busy = query.isFetching || saveTemplate.isPending || enablementAction.isPending;
  const relationOptions = useMemo(() => buildToolTemplateRelationOptions(alertSourcesResult), [alertSourcesResult]);
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
    const readiness = diagnosisToolSourceReadiness({
      alertSourceProfileID: values.alertSourceProfileID,
      sources: relationOptions.alertSources,
      tool: values.tool
    });
    if (readiness.status !== "ready") {
      setNotice({ kind: "error", message: readiness.detail });
      return;
    }

    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
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
    setNotice({ kind: "info", message: "Template saved." });
  }

  async function handleEnablement(template: DiagnosisToolTemplate, enabled: boolean) {
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
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyDiagnosisToolTemplateForm());
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
    const preset = diagnosisToolTemplatePresets.find((candidate) => candidate.id === presetID);
    if (preset === undefined) {
      return;
    }
    const currentSourceID = form.getFieldValue("alertSourceProfileID") ?? null;
    form.setFieldsValue(
      presetToFormState(preset, compatibleSourceIDForTool(preset.form.tool, currentSourceID, relationOptions))
    );
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

      {notice ? <Notice notice={notice} /> : null}
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
                <Button disabled={busy} icon={<PlusOutlined />} onClick={resetForm} type="default">
                  New
                </Button>
              )
            }
            title={editingID === null ? "New Tool Template" : `Edit Template #${editingID}`}
          >
            <Form<DiagnosisToolTemplateFormState>
              disabled={busy}
              form={form}
              initialValues={emptyDiagnosisToolTemplateForm()}
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
                    <InputNumber
                      max={selectedTool === "active_alerts" ? 10 : 20}
                      min={1}
                      precision={0}
                      style={{ width: "100%" }}
                    />
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
                <Button htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
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
              <Button disabled={busy} icon={<ReloadOutlined />} loading={busy} onClick={handleRefresh} type="default">
                Refresh
              </Button>
            }
            title="Configured Tool Templates"
          >
            <DiagnosisToolTemplateTable
              actionID={actionID}
              busy={busy}
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
    alertSourceLabels: Object.fromEntries(
      alertSourcesResult.data.items.map((source) => [source.id, alertSourceLabel(source)])
    ),
    warnings: []
  };
}

function sourceOptionsForTool(
  tool: DiagnosisToolKind,
  relationOptions: ToolTemplateRelationOptions
): RelationSelectOption[] {
  return relationOptions.alertSources.map((source) => {
    const compatible = diagnosisToolSupportsSourceKind(tool, source.kind);
    const label = alertSourceLabel(source);
    return {
      disabled: !compatible,
      label: compatible ? label : `${label} - incompatible`,
      title: compatible ? label : `${label} is not compatible with ${diagnosisToolKindLabels[tool]}`,
      value: source.id
    };
  });
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
  if (source === undefined || !diagnosisToolSupportsSourceKind(tool, source.kind)) {
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
  onDisable: (template: DiagnosisToolTemplate) => void;
  onEdit: (template: DiagnosisToolTemplate) => void;
  onEnable: (template: DiagnosisToolTemplate) => void;
  relationOptions: ToolTemplateRelationOptions;
  templates: DiagnosisToolTemplate[];
};

function DiagnosisToolTemplateTable({
  actionID,
  busy,
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
              {formatDurationSeconds(template.default_window_seconds)} window /{" "}
              {formatDurationSeconds(template.default_step_seconds)} step
            </Typography.Text>
          ) : (
            <Typography.Text type="secondary">
              {template.tool === "active_alerts" ? "active alerts" : "instant query"}
            </Typography.Text>
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
      render: (_, template) => (
        <Space wrap>
          <Button
            disabled={busy || actionID !== null}
            icon={<EditOutlined />}
            onClick={() => onEdit(template)}
            size="small"
          >
            Edit
          </Button>
          {template.enabled ? (
            <Button
              disabled={busy || actionID !== null}
              icon={<PauseCircleOutlined />}
              loading={actionID === template.id}
              onClick={() => onDisable(template)}
              size="small"
            >
              Disable
            </Button>
          ) : (
            <Button
              disabled={busy || actionID !== null}
              icon={<PlayCircleOutlined />}
              loading={actionID === template.id}
              onClick={() => onEnable(template)}
              size="small"
              type="primary"
            >
              Enable
            </Button>
          )}
        </Space>
      ),
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
            description="No diagnosis tool templates"
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
