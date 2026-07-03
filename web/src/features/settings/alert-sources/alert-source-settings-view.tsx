"use client";

import {
  ApiOutlined,
  BranchesOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  ToolOutlined,
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  Descriptions,
  Empty,
  Form,
  Input,
  Row,
  Segmented,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import type { DescriptionsProps, TableColumnsType } from "antd";
import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";
import { useBrowserLocationOrigin } from "@/lib/react/use-browser-location-origin";

import { refreshDiagnosisToolTemplates } from "../diagnosis-tool-templates/client-api";
import type {
  DiagnosisToolTemplate,
  DiagnosisToolTemplateListResponse,
} from "../diagnosis-tool-templates/types";
import { formatDateTime } from "../format";
import { refreshReportWorkflowPolicies } from "../report-workflow-policies/client-api";
import type {
  ReportWorkflowPolicy,
  ReportWorkflowPolicyListResponse,
} from "../report-workflow-policies/types";
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
  useCurrentRBACAuthorizations,
  type CurrentRBACAuthorizationCheck,
} from "../rbac-capabilities";
import {
  refreshAlertSourceProfiles,
  submitAlertSourceProfile,
  testAlertSourceConnection,
} from "./client-api";
import {
  alertmanagerWebhookDeliveryConfig,
  alertmanagerWebhookPublicBaseURL,
  alertSourceAIRoleReadiness,
  alertSourceCanonicalBaseURL,
  alertSourceClassificationHint,
  alertSourceConnectionTargets,
  alertSourceLaunchInitialForm,
  alertSourceOperatorChecklist,
  alertSourceProviderGuidance,
  alertSourceProfileIsThanosRule,
  alertSourcePresetOptions,
  alertSourceReadiness,
  emptyAlertSourceForm,
  formStateToWriteRequest,
  profileToFormState,
  type AlertSourceConnectionTargetPreview,
  type AlertSourceOperatorStep,
  type AlertSourceLaunchIntent,
  type AlertSourcePresetOption,
  type AlertSourceReadiness,
  type AlertSourceWorkflowReturn,
} from "./format";
import {
  alertSourceAutomationSetupReadiness,
  alertSourceNextSetupActions,
  type AlertSourceAutomationBindings,
} from "./next-actions";
import type {
  AlertSourceConnectionTestResult,
  AlertSourceFormState,
  AlertSourceProfile,
  AlertSourceProfileListResponse,
  AlertSourceProfileWriteRequest,
} from "./types";

type AlertSourceSettingsManagerProps = {
  launchIntent?: AlertSourceLaunchIntent | null;
  result: ApiResult<AlertSourceProfileListResponse>;
};

const alertSourceProfilesQueryKey = ["settings", "alert-sources"] as const;
const alertSourceToolTemplatesQueryKey = [
  "settings",
  "alert-sources",
  "diagnosis-tool-templates",
] as const;
const alertSourceWorkflowPoliciesQueryKey = [
  "settings",
  "alert-sources",
  "report-workflow-policies",
] as const;

type SaveProfileVariables = {
  body: AlertSourceProfileWriteRequest;
  sourceID: number | null;
};

const alertSourceBaseAuthorizationChecks: CurrentRBACAuthorizationCheck[] = [
  { key: "alertSourceRead", permission: "alert_source.read" },
  { key: "alertSourceManage", permission: "alert_source.manage" },
];

export function AlertSourceSettingsManager({
  launchIntent = null,
  result,
}: AlertSourceSettingsManagerProps) {
  const [form] = Form.useForm<AlertSourceFormState>();
  const clientReady = useClientReady();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [testingID, setTestingID] = useState<number | null>(null);
  const [testResults, setTestResults] = useState<
    Record<number, AlertSourceConnectionTestResult>
  >({});
  const [launchNotice, setLaunchNotice] = useState<string | null>(
    launchIntent?.message ?? null,
  );
  const {
    errorStatus,
    items: profiles,
    notice,
    query,
    refresh,
    setNotice,
  } = useSettingsList({
    initialResult: result,
    queryKey: alertSourceProfilesQueryKey,
    queryFn: refreshAlertSourceProfiles,
    refreshMessage: "Profiles refreshed.",
    selectItems: (response) => response.items,
  });
  const saveProfile = useSettingsMutation<
    SaveProfileVariables,
    AlertSourceProfile
  >({
    invalidateQueryKey: alertSourceProfilesQueryKey,
    mutationFn: ({ sourceID, body }) =>
      submitAlertSourceProfile(sourceID, body),
  });
  const toolTemplatesQuery = useQuery({
    queryFn: refreshDiagnosisToolTemplates,
    queryKey: alertSourceToolTemplatesQueryKey,
  });
  const workflowPoliciesQuery = useQuery({
    queryFn: refreshReportWorkflowPolicies,
    queryKey: alertSourceWorkflowPoliciesQueryKey,
  });
  const initialFormValues = useMemo(
    () => alertSourceLaunchInitialForm(launchIntent),
    [launchIntent],
  );
  const workflowReturn = launchIntent?.workflowReturn ?? null;
  const presetOptions = useMemo(() => alertSourcePresetOptions(), []);
  const kind = Form.useWatch("kind", form) ?? "prometheus";
  const authMode = Form.useWatch("authMode", form) ?? "none";
  const baseURL = Form.useWatch("baseURL", form) ?? "";
  const labelsText = Form.useWatch("labelsText", form) ?? "";
  const browserLocationOrigin = useBrowserLocationOrigin();
  const connectionTargets = useMemo(
    () => alertSourceConnectionTargets({ baseURL, kind, labelsText }),
    [baseURL, kind, labelsText],
  );
  const canonicalBaseURL = useMemo(
    () => alertSourceCanonicalBaseURL({ baseURL, kind }),
    [baseURL, kind],
  );
  const publicAPIBaseURL = alertmanagerWebhookPublicBaseURL(
    process.env.NEXT_PUBLIC_OPENCLARION_API_PUBLIC_BASE_URL,
    browserLocationOrigin,
  );
  const automationBindingsBySourceID = useMemo(
    () =>
      alertSourceAutomationBindingsBySourceID(
        profiles,
        toolTemplatesQuery.data,
        workflowPoliciesQuery.data,
      ),
    [profiles, toolTemplatesQuery.data, workflowPoliciesQuery.data],
  );
  const authorizationChecks = useMemo(
    () => [
      ...alertSourceBaseAuthorizationChecks,
      ...profiles.map((profile) => ({
        key: alertSourceManageKey(profile.id),
        permission: "alert_source.manage" as const,
        scopeKey: String(profile.id),
        scopeKind: "alert_source" as const,
      })),
    ],
    [profiles],
  );
  const currentAuthorization = useCurrentRBACAuthorizations(
    authorizationChecks,
    clientReady,
  );
  const busy =
    !clientReady ||
    currentAuthorization.isChecking ||
    query.isFetching ||
    saveProfile.isPending;
  const canReadAlertSources = currentAuthorization.can("alertSourceRead");
  const canCreateAlertSource = currentAuthorization.can("alertSourceManage");
  const canSaveCurrentAlertSource =
    editingID === null
      ? canCreateAlertSource
      : currentAuthorization.can(alertSourceManageKey(editingID));
  const formPermissionNotice = settingsManagePermissionNotice({
    canManage: canSaveCurrentAlertSource,
    isChecking: !clientReady || currentAuthorization.isChecking,
    resourceLabel:
      editingID === null ? "alert source creation" : `alert source #${editingID}`,
  });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadAlertSources,
    errorStatus,
    isChecking: !clientReady || currentAuthorization.isChecking,
    resourceLabel: "alert sources",
  });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;

  const summary = useMemo(() => {
    const enabled = profiles.filter((profile) => profile.enabled).length;
    const prometheus = profiles.filter(
      (profile) => profile.kind === "prometheus",
    ).length;
    const alertmanager = profiles.filter(
      (profile) => profile.kind === "alertmanager",
    ).length;
    return { enabled, prometheus, alertmanager };
  }, [profiles]);

  async function handleRefresh() {
    await refresh();
  }

  async function handleSubmit(values: AlertSourceFormState) {
    const parsed = formStateToWriteRequest(normalizeFormValues(values));
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }

    try {
      await saveProfile.mutateAsync({
        sourceID: editingID,
        body: parsed.value,
      });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }

    form.setFieldsValue(emptyAlertSourceForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({
      kind: "info",
      message:
        workflowReturn === null
          ? "Profile saved."
          : "Profile saved. Run Test, create the required evidence tools, then return to workflow enablement.",
    });
  }

  async function handleTest(profile: AlertSourceProfile) {
    setTestingID(profile.id);
    const tested = await testAlertSourceConnection(profile.id);
    setTestingID(null);
    if (!tested.ok) {
      setNotice({ kind: "error", message: tested.error.message });
      return;
    }

    setTestResults((current) => ({ ...current, [profile.id]: tested.data }));
    setNotice({
      kind: noticeKindForConnectionStatus(tested.data.status),
      message: tested.data.message,
    });
  }

  function editProfile(profile: AlertSourceProfile) {
    setEditingID(profile.id);
    form.setFieldsValue(profileToFormState(profile));
    setLaunchNotice(null);
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyAlertSourceForm());
    setLaunchNotice(null);
    setNotice(null);
  }

  function applyPreset(preset: AlertSourcePresetOption) {
    setEditingID(null);
    form.setFieldsValue(preset.form);
    setLaunchNotice(preset.message);
    setNotice(null);
  }

  return (
    <div className="stack">
      <Row aria-label="Alert source metrics" gutter={[12, 12]}>
        <MetricCard label="Profiles" value={profiles.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Prometheus" value={summary.prometheus} />
        <MetricCard label="Alertmanager" value={summary.alertmanager} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} /> : null}
      {launchNotice ? (
        <Alert
          aria-label="Alert source launch preset"
          description={launchNotice}
          message="Source action loaded"
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      <AlertSourceWorkflowReturnPanel workflowReturn={workflowReturn} />

      <Row align="top" className="settings-console-grid" gutter={[16, 16]}>
        <Col lg={8} md={24} xs={24}>
          <Card
            extra={
              editingID === null ? null : (
                <Button
                  disabled={busy || !canCreateAlertSource}
                  icon={<PlusOutlined />}
                  onClick={resetForm}
                  type="default"
                >
                  New
                </Button>
              )
            }
            title={
              editingID === null
                ? "New Alert Source"
                : `Edit Source #${editingID}`
            }
          >
            {formPermissionNotice ? (
              <ReadOnlyModeAlert notice={formPermissionNotice} />
            ) : null}
            <Form<AlertSourceFormState>
              disabled={busy || !canSaveCurrentAlertSource}
              form={form}
              initialValues={initialFormValues}
              layout="vertical"
              onFinish={handleSubmit}
              onValuesChange={(changed: Partial<AlertSourceFormState>) => {
                if (changed.authMode === "none") {
                  form.setFieldValue("secretRef", "");
                }
              }}
            >
              <AlertSourcePresetControls
                disabled={busy || !canSaveCurrentAlertSource}
                onApply={applyPreset}
                presets={presetOptions}
              />

              <Form.Item
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Profile name is required." },
                  {
                    max: 120,
                    message: "Profile name must be 120 characters or fewer.",
                  },
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Kind"
                    name="kind"
                    rules={[{ required: true, message: "Kind is required." }]}
                  >
                    <Segmented
                      block
                      options={[
                        { value: "prometheus", label: "Prometheus" },
                        { value: "alertmanager", label: "Alertmanager" },
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Auth"
                    name="authMode"
                    rules={[
                      { required: true, message: "Auth mode is required." },
                    ]}
                  >
                    <Segmented
                      block
                      options={[
                        { value: "none", label: "None" },
                        { value: "bearer", label: "Bearer" },
                      ]}
                    />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item
                extra={
                  <ConnectionTargetPreview
                    canonicalBaseURL={canonicalBaseURL}
                    result={connectionTargets}
                  />
                }
                label="Base URL"
                name="baseURL"
                rules={[
                  { required: true, message: "Base URL is required." },
                  { type: "url", message: "Base URL must be a valid URL." },
                ]}
              >
                <Input autoComplete="url" />
              </Form.Item>

              <Form.Item
                label="Secret reference"
                name="secretRef"
                rules={[
                  ...(authMode === "bearer"
                    ? [
                        {
                          required: true,
                          message: "Bearer auth requires a secret reference.",
                        },
                      ]
                    : []),
                  {
                    pattern: /^\S*$/,
                    message: "Secret reference must not contain whitespace.",
                  },
                ]}
              >
                <Input autoComplete="off" disabled={authMode === "none"} />
              </Form.Item>

              <Form.Item label="Labels" name="labelsText">
                <Input.TextArea
                  autoSize={{ minRows: 4, maxRows: 8 }}
                  placeholder={"env=prod\nowner=platform"}
                />
              </Form.Item>

              <Form.Item name="enabled" valuePropName="checked">
                <Checkbox>Enabled</Checkbox>
              </Form.Item>

              <Form.Item noStyle shouldUpdate>
                {(sourceForm) => (
                  <AlertSourceReadinessPreview
                    values={normalizeFormValues(
                      sourceForm.getFieldsValue(true) as AlertSourceFormState,
                    )}
                  />
                )}
              </Form.Item>

              <Space wrap>
                <Button
                  disabled={busy || !canSaveCurrentAlertSource}
                  htmlType="submit"
                  icon={<SaveOutlined />}
                  loading={busy}
                  type="primary"
                >
                  Save Profile
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
              <Button
                disabled={busy || !canReadAlertSources}
                icon={<ReloadOutlined />}
                loading={busy}
                onClick={handleRefresh}
                type="default"
              >
                Refresh
              </Button>
            }
            title="Configured Sources"
          >
            <AlertSourceTable
              busy={busy}
              canRead={canReadAlertSources}
              canManageProfile={(profileID) =>
                currentAuthorization.can(alertSourceManageKey(profileID))
              }
              onEdit={editProfile}
              onTest={handleTest}
              automationBindingsBySourceID={automationBindingsBySourceID}
              profiles={profiles}
              publicAPIBaseURL={publicAPIBaseURL}
              testResults={testResults}
              testingID={testingID}
              workflowReturn={workflowReturn}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
}

function AlertSourceWorkflowReturnPanel({
  workflowReturn,
}: {
  workflowReturn: AlertSourceWorkflowReturn | null;
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
      aria-label="Alert source workflow return"
      description={workflowReturn.detail}
      message="Workflow return"
      role="status"
      showIcon
      type="info"
    />
  );
}

function AlertSourcePresetControls({
  disabled,
  onApply,
  presets,
}: {
  disabled: boolean;
  onApply: (preset: AlertSourcePresetOption) => void;
  presets: AlertSourcePresetOption[];
}) {
  return (
    <Form.Item
      label="Preset"
      tooltip="Fill the form for a common alert or metric source."
    >
      <Space size={[8, 8]} wrap>
        {presets.map((preset) => (
          <Button
            disabled={disabled}
            htmlType="button"
            key={preset.intent}
            onClick={() => onApply(preset)}
            title={preset.detail}
            type="default"
          >
            {preset.label}
          </Button>
        ))}
      </Space>
    </Form.Item>
  );
}

function ConnectionTargetPreview({
  canonicalBaseURL,
  result,
}: {
  canonicalBaseURL: ReturnType<typeof alertSourceCanonicalBaseURL>;
  result: ReturnType<typeof alertSourceConnectionTargets>;
}) {
  if (!result.ok) {
    return (
      <Typography.Text type="secondary">
        Connection target pending.
      </Typography.Text>
    );
  }
  return (
    <Space direction="vertical" size={2}>
      {canonicalBaseURL.ok ? (
        <Space align="start" direction="vertical" size={0}>
          <Typography.Text type="secondary">Saved base URL</Typography.Text>
          <Typography.Text
            className="settings-url-cell"
            copyable={{ text: canonicalBaseURL.value }}
          >
            {canonicalBaseURL.value}
          </Typography.Text>
        </Space>
      ) : null}
      <Typography.Text type="secondary">Connection targets</Typography.Text>
      {result.value.map((target) => (
        <Space align="start" direction="vertical" key={target.label} size={0}>
          <Typography.Text type="secondary">{target.label}</Typography.Text>
          <Typography.Text
            className="settings-url-cell"
            copyable={{ text: target.value }}
          >
            {target.value}
          </Typography.Text>
        </Space>
      ))}
    </Space>
  );
}

function AlertSourceReadinessPreview({
  values,
}: {
  values: AlertSourceFormState;
}) {
  const readiness = alertSourceReadiness(values);
  const classificationHint = alertSourceClassificationHint(values);
  const connection = alertSourceConnectionTargets({
    baseURL: values.baseURL,
    kind: values.kind,
    labelsText: values.labelsText,
  });
  const operatorChecklist = alertSourceOperatorChecklist(values);
  const providerGuidance = alertSourceProviderGuidance(values);
  const items: DescriptionsProps["items"] = [
    {
      children: values.enabled ? "Enabled" : "Draft",
      key: "state",
      label: "State",
    },
    {
      children: values.authMode === "bearer" ? "Bearer token" : "None",
      key: "auth",
      label: "Auth",
    },
    {
      children:
        values.kind === "alertmanager"
          ? "Pull active alerts and accept Alertmanager webhooks"
          : "Pull alerts and metric evidence",
      key: "usage",
      label: "Usage",
    },
    {
      children: connection.ok ? (
        <ConnectionTargetsDescription targets={connection.value} />
      ) : (
        "Pending"
      ),
      key: "target",
      label: "Test targets",
    },
  ];

  return (
    <div
      aria-label="Alert source readiness preview"
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={alertSourceReadinessColor(readiness.status)}>
            {alertSourceReadinessLabel(readiness.status)}
          </Tag>
          <Tag color={values.kind === "alertmanager" ? "red" : "blue"}>
            {values.kind}
          </Tag>
          {values.authMode === "bearer" ? (
            <Tag color="gold">Secret required</Tag>
          ) : (
            <Tag color="green">No auth</Tag>
          )}
        </Space>
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
        {classificationHint === null ? null : (
          <Alert
            description={
              <Space direction="vertical" size={4}>
                <Typography.Text>{classificationHint.detail}</Typography.Text>
                <Typography.Text
                  className="settings-code-cell"
                  copyable={{ text: classificationHint.suggestedLabelsText }}
                  type="secondary"
                >
                  {classificationHint.suggestedLabelsText}
                </Typography.Text>
              </Space>
            }
            message={classificationHint.label}
            showIcon
            type="warning"
          />
        )}
        <Space wrap>
          {readiness.capabilities.map((capability) => (
            <Tag key={capability}>{capability}</Tag>
          ))}
        </Space>
        <Alert
          description={
            <Space direction="vertical" size={6}>
              <Typography.Text>{providerGuidance.detail}</Typography.Text>
              {providerGuidance.items.map((item) => (
                <Space align="start" key={item.key} size={6}>
                  <Tag>{item.value}</Tag>
                  <Space direction="vertical" size={0}>
                    <Typography.Text>{item.label}</Typography.Text>
                    <Typography.Text type="secondary">
                      {item.detail}
                    </Typography.Text>
                  </Space>
                </Space>
              ))}
            </Space>
          }
          message={providerGuidance.label}
          showIcon
          type="info"
        />
        <Descriptions column={1} items={items} size="small" />
        <OperatorChecklist steps={operatorChecklist} />
      </Space>
    </div>
  );
}

function ConnectionTargetsDescription({
  targets,
}: {
  targets: AlertSourceConnectionTargetPreview[];
}) {
  return (
    <Space direction="vertical" size={2}>
      {targets.map((target) => (
        <Typography.Text
          className="settings-url-cell"
          key={target.label}
          type="secondary"
        >
          {target.label}: {target.value}
        </Typography.Text>
      ))}
    </Space>
  );
}

function OperatorChecklist({ steps }: { steps: AlertSourceOperatorStep[] }) {
  return (
    <Space
      aria-label="Alert source operator checklist"
      direction="vertical"
      size={6}
    >
      {steps.map((step) => (
        <Space align="start" key={step.key} size={8}>
          <Tag color={alertSourceReadinessColor(step.status)}>
            {alertSourceReadinessLabel(step.status)}
          </Tag>
          <Space direction="vertical" size={0}>
            <Typography.Text strong>{step.label}</Typography.Text>
            <Typography.Text type="secondary">{step.detail}</Typography.Text>
          </Space>
        </Space>
      ))}
    </Space>
  );
}

function alertSourceReadinessColor(
  status: AlertSourceReadiness["status"],
): string {
  switch (status) {
    case "ready":
      return "green";
    case "pending":
      return "default";
    case "blocked":
      return "red";
  }
}

function alertSourceReadinessLabel(
  status: AlertSourceReadiness["status"],
): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "pending":
      return "Pending";
    case "blocked":
      return "Blocked";
  }
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
  const type =
    notice.kind === "error"
      ? "error"
      : notice.kind === "warning"
        ? "warning"
        : "success";
  return (
    <Alert
      description={notice.message}
      message={
        notice.kind === "error"
          ? "Request failed"
          : notice.kind === "warning"
            ? "Connection review"
            : "Settings"
      }
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={type}
    />
  );
}

function AlertSourceTable({
  automationBindingsBySourceID,
  profiles,
  busy,
  canRead,
  canManageProfile,
  onEdit,
  onTest,
  testResults,
  testingID,
  publicAPIBaseURL,
  workflowReturn,
}: {
  automationBindingsBySourceID: Record<number, AlertSourceAutomationBindings>;
  profiles: AlertSourceProfile[];
  busy: boolean;
  canRead: boolean;
  canManageProfile: (profileID: number) => boolean;
  onEdit: (profile: AlertSourceProfile) => void;
  onTest: (profile: AlertSourceProfile) => void;
  testResults: Record<number, AlertSourceConnectionTestResult>;
  testingID: number | null;
  publicAPIBaseURL: string;
  workflowReturn: AlertSourceWorkflowReturn | null;
}) {
  const columns: TableColumnsType<AlertSourceProfile> = [
    {
      dataIndex: "name",
      key: "name",
      title: "Name",
      render: (_value, profile) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>{profile.name}</Typography.Text>
          <Typography.Text type="secondary">#{profile.id}</Typography.Text>
        </Space>
      ),
    },
    {
      dataIndex: "kind",
      key: "kind",
      title: "Kind",
      render: (_value, profile) => (
        <Tag color={profile.kind === "alertmanager" ? "red" : "blue"}>
          {profile.kind}
        </Tag>
      ),
    },
    {
      key: "ai_role",
      title: "AI Role",
      render: (_value, profile) => <AlertSourceAIRole profile={profile} />,
    },
    {
      dataIndex: "base_url",
      key: "base_url",
      title: "Endpoint",
      render: (_value, profile) => (
        <Typography.Text className="settings-url-cell">
          {profile.base_url}
        </Typography.Text>
      ),
    },
    {
      key: "ingest",
      title: "Ingest",
      render: (_value, profile) => (
        <IngestEndpoint profile={profile} publicAPIBaseURL={publicAPIBaseURL} />
      ),
    },
    {
      dataIndex: "auth_mode",
      key: "auth",
      title: "Auth",
      render: (_value, profile) => (
        <Space direction="vertical" size={2}>
          <Tag color={profile.auth_mode === "bearer" ? "gold" : "green"}>
            {profile.auth_mode}
          </Tag>
          {profile.secret_ref === "" ? null : (
            <Typography.Text className="settings-secret-ref" type="secondary">
              {profile.secret_ref}
            </Typography.Text>
          )}
        </Space>
      ),
    },
    {
      dataIndex: "enabled",
      key: "state",
      title: "State",
      render: (_value, profile) => (
        <Tag color={profile.enabled ? "green" : "default"}>
          {profile.enabled ? "enabled" : "disabled"}
        </Tag>
      ),
    },
    {
      dataIndex: "labels",
      key: "labels",
      title: "Labels",
      render: (_value, profile) => <Labels labels={profile.labels} />,
    },
    {
      key: "last_test",
      title: "Last Test",
      render: (_value, profile) => (
        <ConnectionTestResult result={testResults[profile.id]} />
      ),
    },
    {
      key: "next_setup",
      title: "Next Setup",
      render: (_value, profile) => (
        <AlertSourceNextSetupActions
          bindings={automationBindingsBySourceID[profile.id]}
          profile={profile}
          result={testResults[profile.id]}
          workflowReturn={workflowReturn}
        />
      ),
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: "Updated",
      render: (_value, profile) => formatDateTime(profile.updated_at),
    },
    {
      key: "action",
      title: "Action",
      render: (_value, profile) => {
        const canManage = canManageProfile(profile.id);
        return (
          <Space wrap>
            <Button
              disabled={
                busy ||
                !canManage ||
                (testingID !== null && testingID !== profile.id)
              }
              icon={<ApiOutlined />}
              loading={testingID === profile.id}
              onClick={() => onTest(profile)}
              type="link"
            >
              Test
            </Button>
            <Button
              disabled={busy || !canManage || testingID !== null}
              icon={<EditOutlined />}
              onClick={() => onEdit(profile)}
              type="link"
            >
              Edit
            </Button>
          </Space>
        );
      },
    },
  ];

  return (
    <Table<AlertSourceProfile>
      columns={columns}
      dataSource={profiles}
      loading={busy}
      locale={{
        emptyText: (
          <Empty
            description={settingsReadPermissionEmptyDescription({
              canRead,
              emptyDescription: "No alert sources configured.",
              resourceLabel: "alert sources",
            })}
          />
        ),
      }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1560 }}
    />
  );
}

function AlertSourceAIRole({ profile }: { profile: AlertSourceProfile }) {
  const readiness = alertSourceAIRoleReadiness(profile);
  return (
    <Space direction="vertical" size={2}>
      <Space size={[4, 4]} wrap>
        <Tag color={alertSourceReadinessColor(readiness.status)}>
          {alertSourceReadinessLabel(readiness.status)}
        </Tag>
        <Tag color={readiness.role === "alert_intake" ? "red" : "blue"}>
          {readiness.role === "alert_intake" ? "Alert intake" : "Metric evidence"}
        </Tag>
      </Space>
      <Typography.Text ellipsis={{ tooltip: readiness.detail }} type="secondary">
        {readiness.label}
      </Typography.Text>
    </Space>
  );
}

function AlertSourceNextSetupActions({
  bindings,
  profile,
  result,
  workflowReturn,
}: {
  bindings?: AlertSourceAutomationBindings;
  profile: AlertSourceProfile;
  result?: AlertSourceConnectionTestResult;
  workflowReturn: AlertSourceWorkflowReturn | null;
}) {
  const readiness = alertSourceAutomationSetupReadiness(
    profile,
    result,
    bindings,
  );
  return (
    <Space direction="vertical" size={4}>
      <Space size={[4, 4]} wrap>
        <Tag color={alertSourceReadinessColor(readiness.status)}>
          {readiness.label}
        </Tag>
      </Space>
      <Typography.Text ellipsis={{ tooltip: readiness.detail }} type="secondary">
        {readiness.detail}
      </Typography.Text>
      <Space size={[4, 4]} wrap>
        {readiness.steps.map((step) => (
          <Tooltip key={step.key} title={step.detail}>
            <Tag color={alertSourceReadinessColor(step.status)}>
              {step.label}
            </Tag>
          </Tooltip>
        ))}
      </Space>
      <Space wrap>
        {alertSourceNextSetupActions(profile, {
          bindings,
          workflowReturn,
        }).map((action) => (
          <Tooltip key={action.key} title={action.detail}>
            <Button
              href={action.href}
              icon={alertSourceNextSetupActionIcon(action.key)}
              size="small"
              type="link"
            >
              {action.label}
            </Button>
          </Tooltip>
        ))}
      </Space>
    </Space>
  );
}

function alertSourceAutomationBindingsBySourceID(
  profiles: AlertSourceProfile[],
  templatesResult?: ApiResult<DiagnosisToolTemplateListResponse>,
  policiesResult?: ApiResult<ReportWorkflowPolicyListResponse>,
): Record<number, AlertSourceAutomationBindings> {
  const toolTemplates =
    templatesResult?.ok === true ? templatesResult.data.items : null;
  const workflowPolicies =
    policiesResult?.ok === true ? policiesResult.data.items : null;
  return Object.fromEntries(
    profiles.map((profile) => [
      profile.id,
      alertSourceAutomationBindings(
        profile,
        profiles,
        toolTemplates,
        workflowPolicies,
      ),
    ]),
  );
}

function alertSourceAutomationBindings(
  profile: AlertSourceProfile,
  profiles: AlertSourceProfile[],
  toolTemplates: DiagnosisToolTemplate[] | null,
  workflowPolicies: ReportWorkflowPolicy[] | null,
): AlertSourceAutomationBindings {
  const sourceToolTemplates =
    toolTemplates?.filter(
      (template) => template.alert_source_profile_id === profile.id,
    ) ?? [];
  const sourceWorkflowPolicies =
    workflowPolicies?.filter(
      (policy) =>
        policy.alert_source_profile_id === profile.id &&
        policy.diagnosis_follow_up === "auto_room",
    ) ?? [];
  const metricEvidenceSourceIDs = new Set(
    profiles
      .filter(
        (candidate) =>
          candidate.enabled &&
          candidate.kind === "prometheus" &&
          !alertSourceProfileIsThanosRule(candidate),
      )
      .map((candidate) => candidate.id),
  );
  const workflowMetricTemplates =
    toolTemplates?.filter(
      (template) =>
        metricEvidenceSourceIDs.has(template.alert_source_profile_id) &&
        (template.tool === "metric_query" ||
          template.tool === "metric_range_query"),
    ) ?? [];
  return {
    activeAlertTemplateCount: sourceToolTemplates.filter(
      (template) => template.tool === "active_alerts",
    ).length,
    autoRoomPolicyCount: sourceWorkflowPolicies.length,
    enabledActiveAlertTemplateCount: sourceToolTemplates.filter(
      (template) => template.tool === "active_alerts" && template.enabled,
    ).length,
    enabledAutoRoomPolicyCount: sourceWorkflowPolicies.filter(
      (policy) => policy.enabled,
    ).length,
    enabledMetricTemplateCount: sourceToolTemplates.filter(
      (template) => template.tool === "metric_query" && template.enabled,
    ).length,
    enabledRangeMetricTemplateCount: sourceToolTemplates.filter(
      (template) => template.tool === "metric_range_query" && template.enabled,
    ).length,
    enabledWorkflowMetricTemplateCount: workflowMetricTemplates.filter(
      (template) => template.enabled,
    ).length,
    metricEvidenceSourceCount: metricEvidenceSourceIDs.size,
    metricTemplateCount: sourceToolTemplates.filter(
      (template) => template.tool === "metric_query",
    ).length,
    rangeMetricTemplateCount: sourceToolTemplates.filter(
      (template) => template.tool === "metric_range_query",
    ).length,
    toolTemplatesKnown: toolTemplates !== null,
    workflowPoliciesKnown: workflowPolicies !== null,
  };
}

function alertSourceNextSetupActionIcon(actionKey: string) {
  if (actionKey === "auto-room-workflow") {
    return <BranchesOutlined />;
  }
  if (actionKey === "notification-channel") {
    return <ApiOutlined />;
  }
  if (actionKey === "metric-evidence-source") {
    return <PlusOutlined />;
  }
  return <ToolOutlined />;
}

function IngestEndpoint({
  profile,
  publicAPIBaseURL,
}: {
  profile: AlertSourceProfile;
  publicAPIBaseURL: string;
}) {
  const config = alertmanagerWebhookDeliveryConfig(profile, publicAPIBaseURL);
  if (config === null) {
    return <Typography.Text type="secondary">Provider pull</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Space size={[4, 4]} wrap>
        <Tag color="red">Webhook</Tag>
        <Tag>{config.method}</Tag>
        <Tag>{config.contentType}</Tag>
        <Tag color={config.endpointScope === "absolute" ? "green" : "gold"}>
          {config.endpointScope === "absolute" ? "External URL" : "Relative URL"}
        </Tag>
      </Space>
      <Typography.Text
        className="settings-ingest-endpoint"
        copyable={{ text: config.endpoint }}
        ellipsis={{ tooltip: config.endpoint }}
      >
        {config.endpoint}
      </Typography.Text>
      <Typography.Text
        ellipsis={{ tooltip: config.endpointGuidance }}
        type={config.endpointScope === "absolute" ? "secondary" : "warning"}
      >
        {config.endpointGuidance}
      </Typography.Text>
      <Typography.Text type="secondary">{config.authorization}</Typography.Text>
      <Space align="start" direction="vertical" size={0}>
        <Typography.Text type="secondary">
          Receiver: {config.receiverName}
        </Typography.Text>
        <Typography.Paragraph
          className="settings-code-cell"
          copyable={{ text: config.receiverYAML }}
          ellipsis={{ rows: 3, expandable: true, symbol: "more" }}
        >
          {config.receiverYAML}
        </Typography.Paragraph>
      </Space>
      <Space align="start" direction="vertical" size={0}>
        <Typography.Text type="secondary">Scoped route</Typography.Text>
        <Typography.Paragraph
          className="settings-code-cell"
          copyable={{ text: config.routeYAML }}
          ellipsis={{ rows: 5, expandable: true, symbol: "more" }}
        >
          {config.routeYAML}
        </Typography.Paragraph>
        <Typography.Text
          ellipsis={{ tooltip: config.routeGuidance }}
          type="secondary"
        >
          {config.routeGuidance}
        </Typography.Text>
      </Space>
      <Space
        align="start"
        aria-label="Alertmanager routing checklist"
        direction="vertical"
        size={2}
      >
        <Typography.Text type="secondary">Apply in Alertmanager</Typography.Text>
        {config.routingChecklist.map((item) => (
          <Space align="start" key={item.key} size={6}>
            <Tag>{item.label}</Tag>
            <Typography.Text
              ellipsis={{ tooltip: item.detail }}
              type="secondary"
            >
              {item.detail}
            </Typography.Text>
          </Space>
        ))}
      </Space>
      <Typography.Text ellipsis={{ tooltip: config.detail }} type="secondary">
        {config.detail}
      </Typography.Text>
    </Space>
  );
}

function ConnectionTestResult({
  result,
}: {
  result?: AlertSourceConnectionTestResult;
}) {
  if (!result) {
    return <Typography.Text type="secondary">Not tested</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Tag color={connectionStatusColor(result.status)}>{result.status}</Tag>
      <Typography.Text type="secondary">{result.reason_code}</Typography.Text>
      <Typography.Text ellipsis={{ tooltip: result.message }} type="secondary">
        {result.message}
      </Typography.Text>
      <Typography.Text type="secondary">
        {formatDateTime(result.checked_at)}
      </Typography.Text>
      {result.status === "success" ? (
        <Typography.Text type="secondary">
          {result.observed_alerts} firing alerts
        </Typography.Text>
      ) : null}
    </Space>
  );
}

function Labels({ labels }: { labels: AlertSourceProfile["labels"] }) {
  const entries = Object.entries(labels).sort(([left], [right]) =>
    left.localeCompare(right),
  );
  if (entries.length === 0) {
    return <Typography.Text type="secondary">None</Typography.Text>;
  }
  return (
    <div className="label-stack">
      {entries.map(([key, value]) => (
        <Tag key={key}>
          {key}={value}
        </Tag>
      ))}
    </div>
  );
}

function normalizeFormValues(
  values: AlertSourceFormState,
): AlertSourceFormState {
  return {
    ...emptyAlertSourceForm(),
    ...values,
    enabled: Boolean(values.enabled),
    secretRef: values.secretRef ?? "",
    labelsText: values.labelsText ?? "",
  };
}

function alertSourceManageKey(profileID: number): string {
  return `alertSourceManage:${profileID}`;
}

function connectionStatusColor(
  status: AlertSourceConnectionTestResult["status"],
) {
  switch (status) {
    case "success":
      return "green";
    case "blocked":
      return "gold";
    case "unsupported":
      return "default";
    case "failed":
      return "red";
  }
}

function noticeKindForConnectionStatus(
  status: AlertSourceConnectionTestResult["status"],
): SettingsNotice["kind"] {
  switch (status) {
    case "success":
      return "info";
    case "blocked":
    case "unsupported":
      return "warning";
    case "failed":
      return "error";
  }
}
