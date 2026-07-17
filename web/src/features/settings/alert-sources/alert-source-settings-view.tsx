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
import { useLocale, useTranslations } from "next-intl";
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

type AlertSourceTranslator = ReturnType<typeof useTranslations<"AlertSourceSettings">>;

const alertSourceBaseAuthorizationChecks: CurrentRBACAuthorizationCheck[] = [
  { key: "alertSourceRead", permission: "alert_source.read" },
  { key: "alertSourceManage", permission: "alert_source.manage" },
];

export function AlertSourceSettingsManager({
  launchIntent = null,
  result,
}: AlertSourceSettingsManagerProps) {
  const locale = useLocale();
  const t = useTranslations("AlertSourceSettings");
  const common = useTranslations("Common");
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
    refreshMessage: t("refreshed"),
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
    message: common("formReadOnly", {
      resource:
        editingID === null
          ? t("creationResource")
          : t("sourceResource", { id: editingID }),
    }),
  });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadAlertSources,
    errorStatus,
    isChecking: !clientReady || currentAuthorization.isChecking,
    message: common("readAccessLimited", {
      resource: t("sourcesResource"),
    }),
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
      setNotice({ kind: "error", message: localizeAlertSourceText(parsed.message, t) });
      return;
    }

    try {
      await saveProfile.mutateAsync({
        sourceID: editingID,
        body: parsed.value,
      });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      return;
    }

    form.setFieldsValue(emptyAlertSourceForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({
      kind: "info",
      message:
        workflowReturn === null
          ? t("saved")
          : t("savedForWorkflow"),
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
      <Row aria-label={t("metricsLabel")} gutter={[12, 12]}>
        <MetricCard label={t("profiles")} value={profiles.length} />
        <MetricCard label={t("enabled")} value={summary.enabled} />
        <MetricCard label="Prometheus" value={summary.prometheus} />
        <MetricCard label="Alertmanager" value={summary.alertmanager} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} t={t} /> : null}
      {launchNotice ? (
        <Alert
          aria-label={t("launchPreset")}
          description={localizeAlertSourceText(launchNotice, t)}
          message={t("actionLoaded")}
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      <AlertSourceWorkflowReturnPanel t={t} workflowReturn={workflowReturn} />

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
                  {t("new")}
                </Button>
              )
            }
            title={
              editingID === null
                ? t("newSource")
                : t("editSource", { id: editingID })
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
                t={t}
              />

              <Form.Item
                label={t("name")}
                name="name"
                rules={[
                  { required: true, message: t("nameRequired") },
                  {
                    max: 120,
                    message: t("nameLength"),
                  },
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("kind")}
                    name="kind"
                    rules={[{ required: true, message: t("kindRequired") }]}
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
                    label={t("auth")}
                    name="authMode"
                    rules={[
                      { required: true, message: t("authRequired") },
                    ]}
                  >
                    <Segmented
                      block
                      options={[
                        { value: "none", label: t("none") },
                        { value: "bearer", label: t("bearer") },
                      ]}
                    />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item
                extra={
                  <ConnectionTargetPreview
                    canonicalBaseURL={canonicalBaseURL}
                    t={t}
                    result={connectionTargets}
                  />
                }
                label={t("baseUrl")}
                name="baseURL"
                rules={[
                  { required: true, message: t("baseUrlRequired") },
                  { type: "url", message: t("baseUrlValid") },
                ]}
              >
                <Input autoComplete="url" />
              </Form.Item>

              <Form.Item
                label={t("secretReference")}
                name="secretRef"
                rules={[
                  ...(authMode === "bearer"
                    ? [
                        {
                          required: true,
                          message: t("secretRequired"),
                        },
                      ]
                    : []),
                  {
                    pattern: /^\S*$/,
                    message: t("secretWhitespace"),
                  },
                ]}
              >
                <Input autoComplete="off" disabled={authMode === "none"} />
              </Form.Item>

              <Form.Item label={t("labels")} name="labelsText">
                <Input.TextArea
                  autoSize={{ minRows: 4, maxRows: 8 }}
                  placeholder={"env=prod\nowner=platform"}
                />
              </Form.Item>

              <Form.Item name="enabled" valuePropName="checked">
                <Checkbox>{t("enabled")}</Checkbox>
              </Form.Item>

              <Form.Item noStyle shouldUpdate>
                {(sourceForm) => (
                  <AlertSourceReadinessPreview
                    t={t}
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
                  {t("saveProfile")}
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
              <Button
                disabled={busy || !canReadAlertSources}
                icon={<ReloadOutlined />}
                loading={busy}
                onClick={handleRefresh}
                type="default"
              >
                {t("refresh")}
              </Button>
            }
            title={t("configuredSources")}
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
              locale={locale}
              t={t}
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
  t,
  workflowReturn,
}: {
  t: AlertSourceTranslator;
  workflowReturn: AlertSourceWorkflowReturn | null;
}) {
  if (workflowReturn === null) {
    return null;
  }
  return (
    <Alert
      action={
        <Button href={workflowReturn.href} icon={<BranchesOutlined />} type="primary">
          {localizeAlertSourceText(workflowReturn.label, t)}
        </Button>
      }
      aria-label={t("workflowReturnLabel")}
      description={localizeAlertSourceText(workflowReturn.detail, t)}
      message={t("workflowReturn")}
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
  t,
}: {
  disabled: boolean;
  onApply: (preset: AlertSourcePresetOption) => void;
  presets: AlertSourcePresetOption[];
  t: AlertSourceTranslator;
}) {
  return (
    <Form.Item
      label={t("preset")}
      tooltip={t("presetTooltip")}
    >
      <Space size={[8, 8]} wrap>
        {presets.map((preset) => (
          <Button
            disabled={disabled}
            htmlType="button"
            key={preset.intent}
            onClick={() => onApply(preset)}
            title={localizeAlertSourceText(preset.detail, t)}
            type="default"
          >
            {localizeAlertSourceText(preset.label, t)}
          </Button>
        ))}
      </Space>
    </Form.Item>
  );
}

function ConnectionTargetPreview({
  canonicalBaseURL,
  result,
  t,
}: {
  canonicalBaseURL: ReturnType<typeof alertSourceCanonicalBaseURL>;
  result: ReturnType<typeof alertSourceConnectionTargets>;
  t: AlertSourceTranslator;
}) {
  if (!result.ok) {
    return (
      <Typography.Text type="secondary">
        {t("connectionPending")}
      </Typography.Text>
    );
  }
  return (
    <Space direction="vertical" size={2}>
      {canonicalBaseURL.ok ? (
        <Space align="start" direction="vertical" size={0}>
          <Typography.Text type="secondary">{t("savedBaseUrl")}</Typography.Text>
          <Typography.Text
            className="settings-url-cell"
            copyable={{ text: canonicalBaseURL.value }}
          >
            {canonicalBaseURL.value}
          </Typography.Text>
        </Space>
      ) : null}
      <Typography.Text type="secondary">{t("connectionTargets")}</Typography.Text>
      {result.value.map((target) => (
        <Space align="start" direction="vertical" key={target.label} size={0}>
          <Typography.Text type="secondary">
            {localizeAlertSourceText(target.label, t)}
          </Typography.Text>
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
  t,
  values,
}: {
  t: AlertSourceTranslator;
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
      children: values.enabled ? t("enabled") : t("draft"),
      key: "state",
      label: t("state"),
    },
    {
      children: values.authMode === "bearer" ? t("bearerToken") : t("none"),
      key: "auth",
      label: t("auth"),
    },
    {
      children:
        values.kind === "alertmanager"
          ? t("alertmanagerUsage")
          : t("prometheusUsage"),
      key: "usage",
      label: t("usage"),
    },
    {
      children: connection.ok ? (
        <ConnectionTargetsDescription t={t} targets={connection.value} />
      ) : (
        t("pending")
      ),
      key: "target",
      label: t("testTargets"),
    },
  ];

  return (
    <div
      aria-label={t("readinessPreview")}
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={alertSourceReadinessColor(readiness.status)}>
            {alertSourceReadinessLabel(readiness.status, t)}
          </Tag>
          <Tag color={values.kind === "alertmanager" ? "red" : "blue"}>
            {values.kind}
          </Tag>
          {values.authMode === "bearer" ? (
            <Tag color="gold">{t("secretRequiredTag")}</Tag>
          ) : (
            <Tag color="green">{t("noAuth")}</Tag>
          )}
        </Space>
        <Typography.Text strong>{localizeAlertSourceText(readiness.label, t)}</Typography.Text>
        <Typography.Text type="secondary">{localizeAlertSourceText(readiness.detail, t)}</Typography.Text>
        {classificationHint === null ? null : (
          <Alert
            description={
              <Space direction="vertical" size={4}>
                <Typography.Text>{localizeAlertSourceText(classificationHint.detail, t)}</Typography.Text>
                <Typography.Text
                  className="settings-code-cell"
                  copyable={{ text: classificationHint.suggestedLabelsText }}
                  type="secondary"
                >
                  {classificationHint.suggestedLabelsText}
                </Typography.Text>
              </Space>
            }
            message={localizeAlertSourceText(classificationHint.label, t)}
            showIcon
            type="warning"
          />
        )}
        <Space wrap>
          {readiness.capabilities.map((capability) => (
            <Tag key={capability}>{localizeAlertSourceText(capability, t)}</Tag>
          ))}
        </Space>
        <Alert
          description={
            <Space direction="vertical" size={6}>
              <Typography.Text>{localizeAlertSourceText(providerGuidance.detail, t)}</Typography.Text>
              {providerGuidance.items.map((item) => (
                <Space align="start" key={item.key} size={6}>
                  <Tag>{localizeAlertSourceText(item.value, t)}</Tag>
                  <Space direction="vertical" size={0}>
                    <Typography.Text>{localizeAlertSourceText(item.label, t)}</Typography.Text>
                    <Typography.Text type="secondary">
                      {localizeAlertSourceText(item.detail, t)}
                    </Typography.Text>
                  </Space>
                </Space>
              ))}
            </Space>
          }
          message={localizeAlertSourceText(providerGuidance.label, t)}
          showIcon
          type="info"
        />
        <Descriptions column={1} items={items} size="small" />
        <OperatorChecklist steps={operatorChecklist} t={t} />
      </Space>
    </div>
  );
}

function ConnectionTargetsDescription({
  t,
  targets,
}: {
  t: AlertSourceTranslator;
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
          {localizeAlertSourceText(target.label, t)}: {target.value}
        </Typography.Text>
      ))}
    </Space>
  );
}

function OperatorChecklist({
  steps,
  t,
}: {
  steps: AlertSourceOperatorStep[];
  t: AlertSourceTranslator;
}) {
  return (
    <Space
      aria-label={t("operatorChecklist")}
      direction="vertical"
      size={6}
    >
      {steps.map((step) => (
        <Space align="start" key={step.key} size={8}>
          <Tag color={alertSourceReadinessColor(step.status)}>
            {alertSourceReadinessLabel(step.status, t)}
          </Tag>
          <Space direction="vertical" size={0}>
            <Typography.Text strong>{localizeAlertSourceText(step.label, t)}</Typography.Text>
            <Typography.Text type="secondary">{localizeAlertSourceText(step.detail, t)}</Typography.Text>
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
  t: AlertSourceTranslator,
): string {
  switch (status) {
    case "ready":
      return t("ready");
    case "pending":
      return t("pending");
    case "blocked":
      return t("blocked");
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

function Notice({ notice, t }: { notice: SettingsNotice; t: AlertSourceTranslator }) {
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
          ? t("requestFailed")
          : notice.kind === "warning"
            ? t("connectionReview")
            : t("settings")
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
  locale,
  t,
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
  locale: string;
  t: AlertSourceTranslator;
  workflowReturn: AlertSourceWorkflowReturn | null;
}) {
  const common = useTranslations("Common");
  const columns: TableColumnsType<AlertSourceProfile> = [
    {
      dataIndex: "name",
      key: "name",
      title: t("name"),
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
      title: t("kind"),
      render: (_value, profile) => (
        <Tag color={profile.kind === "alertmanager" ? "red" : "blue"}>
          {profile.kind}
        </Tag>
      ),
    },
    {
      key: "ai_role",
      title: t("aiRole"),
      render: (_value, profile) => <AlertSourceAIRole profile={profile} t={t} />,
    },
    {
      dataIndex: "base_url",
      key: "base_url",
      title: t("endpoint"),
      render: (_value, profile) => (
        <Typography.Text className="settings-url-cell">
          {profile.base_url}
        </Typography.Text>
      ),
    },
    {
      key: "ingest",
      title: t("ingest"),
      render: (_value, profile) => (
        <IngestEndpoint profile={profile} publicAPIBaseURL={publicAPIBaseURL} t={t} />
      ),
    },
    {
      dataIndex: "auth_mode",
      key: "auth",
      title: t("auth"),
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
      title: t("state"),
      render: (_value, profile) => (
        <Tag color={profile.enabled ? "green" : "default"}>
          {profile.enabled ? t("enabled") : t("disabled")}
        </Tag>
      ),
    },
    {
      dataIndex: "labels",
      key: "labels",
      title: t("labels"),
      render: (_value, profile) => <Labels labels={profile.labels} t={t} />,
    },
    {
      key: "last_test",
      title: t("lastTest"),
      render: (_value, profile) => (
        <ConnectionTestResult locale={locale} result={testResults[profile.id]} t={t} />
      ),
    },
    {
      key: "next_setup",
      title: t("nextSetup"),
      render: (_value, profile) => (
        <AlertSourceNextSetupActions
          bindings={automationBindingsBySourceID[profile.id]}
          profile={profile}
          result={testResults[profile.id]}
          t={t}
          workflowReturn={workflowReturn}
        />
      ),
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: t("updated"),
      render: (_value, profile) => formatDateTime(profile.updated_at, locale),
    },
    {
      key: "action",
      title: t("action"),
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
              {t("test")}
            </Button>
            <Button
              disabled={busy || !canManage || testingID !== null}
              icon={<EditOutlined />}
              onClick={() => onEdit(profile)}
              type="link"
            >
              {t("edit")}
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
              deniedDescription: common("noReadAccess", {
                resource: t("sourcesResource"),
              }),
              emptyDescription: t("noSources"),
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

function AlertSourceAIRole({
  profile,
  t,
}: {
  profile: AlertSourceProfile;
  t: AlertSourceTranslator;
}) {
  const readiness = alertSourceAIRoleReadiness(profile);
  return (
    <Space direction="vertical" size={2}>
      <Space size={[4, 4]} wrap>
        <Tag color={alertSourceReadinessColor(readiness.status)}>
          {alertSourceReadinessLabel(readiness.status, t)}
        </Tag>
        <Tag color={readiness.role === "alert_intake" ? "red" : "blue"}>
          {readiness.role === "alert_intake" ? t("alertIntake") : t("metricEvidence")}
        </Tag>
      </Space>
      <Typography.Text
        ellipsis={{ tooltip: localizeAlertSourceText(readiness.detail, t) }}
        type="secondary"
      >
        {localizeAlertSourceText(readiness.label, t)}
      </Typography.Text>
    </Space>
  );
}

function AlertSourceNextSetupActions({
  bindings,
  profile,
  result,
  t,
  workflowReturn,
}: {
  bindings?: AlertSourceAutomationBindings;
  profile: AlertSourceProfile;
  result?: AlertSourceConnectionTestResult;
  t: AlertSourceTranslator;
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
          {localizeAlertSourceText(readiness.label, t)}
        </Tag>
      </Space>
      <Typography.Text ellipsis={{ tooltip: localizeAlertSourceText(readiness.detail, t) }} type="secondary">
        {localizeAlertSourceText(readiness.detail, t)}
      </Typography.Text>
      <Space size={[4, 4]} wrap>
        {readiness.steps.map((step) => (
          <Tooltip key={step.key} title={localizeAlertSourceText(step.detail, t)}>
            <Tag color={alertSourceReadinessColor(step.status)}>
              {localizeAlertSourceText(step.label, t)}
            </Tag>
          </Tooltip>
        ))}
      </Space>
      <Space wrap>
        {alertSourceNextSetupActions(profile, {
          bindings,
          workflowReturn,
        }).map((action) => (
          <Tooltip key={action.key} title={localizeAlertSourceText(action.detail, t)}>
            <Button
              href={action.href}
              icon={alertSourceNextSetupActionIcon(action.key)}
              size="small"
              type="link"
            >
              {localizeAlertSourceText(action.label, t)}
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
  t,
}: {
  profile: AlertSourceProfile;
  publicAPIBaseURL: string;
  t: AlertSourceTranslator;
}) {
  const config = alertmanagerWebhookDeliveryConfig(profile, publicAPIBaseURL);
  if (config === null) {
    return <Typography.Text type="secondary">{t("providerPull")}</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Space size={[4, 4]} wrap>
        <Tag color="red">{t("webhook")}</Tag>
        <Tag>{config.method}</Tag>
        <Tag>{config.contentType}</Tag>
        <Tag color={config.endpointScope === "absolute" ? "green" : "gold"}>
          {config.endpointScope === "absolute" ? t("externalUrl") : t("relativeUrl")}
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
        ellipsis={{ tooltip: localizeAlertSourceText(config.endpointGuidance, t) }}
        type={config.endpointScope === "absolute" ? "secondary" : "warning"}
      >
        {localizeAlertSourceText(config.endpointGuidance, t)}
      </Typography.Text>
      <Typography.Text type="secondary">{localizeAlertSourceText(config.authorization, t)}</Typography.Text>
      <Space align="start" direction="vertical" size={0}>
        <Typography.Text type="secondary">
          {t("receiver", { name: config.receiverName })}
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
        <Typography.Text type="secondary">{t("scopedRoute")}</Typography.Text>
        <Typography.Paragraph
          className="settings-code-cell"
          copyable={{ text: config.routeYAML }}
          ellipsis={{ rows: 5, expandable: true, symbol: "more" }}
        >
          {config.routeYAML}
        </Typography.Paragraph>
        <Typography.Text
          ellipsis={{ tooltip: localizeAlertSourceText(config.routeGuidance, t) }}
          type="secondary"
        >
          {localizeAlertSourceText(config.routeGuidance, t)}
        </Typography.Text>
      </Space>
      <Space
        align="start"
        aria-label={t("routingChecklist")}
        direction="vertical"
        size={2}
      >
        <Typography.Text type="secondary">{t("applyInAlertmanager")}</Typography.Text>
        {config.routingChecklist.map((item) => (
          <Space align="start" key={item.key} size={6}>
            <Tag>{localizeAlertSourceText(item.label, t)}</Tag>
            <Typography.Text
              ellipsis={{ tooltip: localizeAlertSourceText(item.detail, t) }}
              type="secondary"
            >
              {localizeAlertSourceText(item.detail, t)}
            </Typography.Text>
          </Space>
        ))}
      </Space>
      <Typography.Text
        ellipsis={{ tooltip: localizeAlertSourceText(config.detail, t) }}
        type="secondary"
      >
        {localizeAlertSourceText(config.detail, t)}
      </Typography.Text>
    </Space>
  );
}

function ConnectionTestResult({
  locale,
  result,
  t,
}: {
  locale: string;
  result?: AlertSourceConnectionTestResult;
  t: AlertSourceTranslator;
}) {
  if (!result) {
    return <Typography.Text type="secondary">{t("notTested")}</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Tag color={connectionStatusColor(result.status)}>
        {localizeAlertSourceText(result.status, t)}
      </Tag>
      <Typography.Text type="secondary">{result.reason_code}</Typography.Text>
      <Typography.Text ellipsis={{ tooltip: result.message }} type="secondary">
        {result.message}
      </Typography.Text>
      <Typography.Text type="secondary">
        {formatDateTime(result.checked_at, locale)}
      </Typography.Text>
      {result.status === "success" ? (
        <Typography.Text type="secondary">
          {t("firingAlerts", { count: result.observed_alerts })}
        </Typography.Text>
      ) : null}
    </Space>
  );
}

function Labels({
  labels,
  t,
}: {
  labels: AlertSourceProfile["labels"];
  t: AlertSourceTranslator;
}) {
  const entries = Object.entries(labels).sort(([left], [right]) =>
    left.localeCompare(right),
  );
  if (entries.length === 0) {
    return <Typography.Text type="secondary">{t("none")}</Typography.Text>;
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

const alertSourceRuntimeTextKeys = {
    "blocked": "runtimeText.blocked",
    "failed": "runtimeText.failed",
    "success": "runtimeText.success",
    "unsupported": "runtimeText.unsupported",
    "Add receiver": "runtimeText.addReceiver",
    "Base URL is required.": "runtimeText.baseUrlIsRequired",
    "Base URL must be 2048 bytes or fewer.": "runtimeText.baseUrlMustBe2048BytesOrFewer",
    "Base URL must be a valid URL.": "runtimeText.baseUrlMustBeAValidUrl",
    "Base URL must not include query or fragment.": "runtimeText.baseUrlMustNotIncludeQueryOrFragment",
    "Base URL must not include userinfo.": "runtimeText.baseUrlMustNotIncludeUserinfo",
    "Base URL scheme must be http or https.": "runtimeText.baseUrlSchemeMustBeHttpOrHttps",
    "Bind active_alerts, metric evidence, Enterprise WeChat delivery, and an auto-room workflow before rollout.": "runtimeText.bindActiveAlertsMetricEvidenceEnterpriseWechatDeliveryAndAnAutoRoom",
    "Bind this source to grouping, report replay, and diagnosis evidence workflows.": "runtimeText.bindThisSourceToGroupingReportReplayAndDiagnosisEvidenceWorkflows",
    "Bind this source to report replay and diagnosis evidence workflows.": "runtimeText.bindThisSourceToReportReplayAndDiagnosisEvidenceWorkflows",
    "Configure Alertmanager, or another Alertmanager webhook-compatible sender, to POST webhook v4 JSON to this endpoint. Thanos Rule alerts should normally route through Alertmanager first. Resolved, silenced, inhibited, and muted alerts are ignored during ingest.": "runtimeText.configureAlertmanagerOrAnotherAlertmanagerWebhookCompatibleSenderToPostWebhookV4",
    "Configure a WeCom channel with report, diagnosis_consultation, and diagnosis_close scopes; workflow setup verifies delivery proof.": "runtimeText.configureAWecomChannelWithReportDiagnosisConsultationAndDiagnosisCloseScopes",
    "Create a WeCom report and AI-room channel for report, diagnosis consultation, and close notifications.": "runtimeText.createAWecomReportAndAiRoomChannelForReportDiagnosisConsultation",
    "Create a metric evidence template bound to this Prometheus-compatible source.": "runtimeText.createAMetricEvidenceTemplateBoundToThisPrometheusCompatibleSource",
    "Create a Prometheus-compatible metric evidence source, usually Thanos Query, and add metric_query or metric_range_query templates.": "runtimeText.createAPrometheusCompatibleMetricEvidenceSourceUsuallyThanosQueryAndAdd",
    "Create an active_alerts evidence template bound to this Prometheus-compatible source.": "runtimeText.createAnActiveAlertsEvidenceTemplateBoundToThisPrometheusCompatibleSource",
    "Create an active_alerts evidence template bound to this Thanos Rule source.": "runtimeText.createAnActiveAlertsEvidenceTemplateBoundToThisThanosRuleSource",
    "Create an active_alerts evidence template for this source; do not use it as the metric confidence source.": "runtimeText.createAnActiveAlertsEvidenceTemplateForThisSourceDoNotUse",
    "Create an active_alerts evidence template for this source.": "runtimeText.createAnActiveAlertsEvidenceTemplateForThisSource",
    "Create metric_query or metric_range_query evidence templates so AI diagnosis can request follow-up metrics before finalizing.": "runtimeText.createMetricQueryOrMetricRangeQueryEvidenceTemplatesSoAiDiagnosis",
    "Create or enable a Prometheus-compatible metric evidence source, usually Thanos Query, for AI confidence-building queries.": "runtimeText.createOrEnableAPrometheusCompatibleMetricEvidenceSourceUsuallyThanosQuery",
    "Create or update an automatic diagnosis workflow after the alert tool is ready.": "runtimeText.createOrUpdateAnAutomaticDiagnosisWorkflowAfterTheAlertToolIs",
    "Enable this Thanos Rule source before using it for active-alert evidence.": "runtimeText.enableThisThanosRuleSourceBeforeUsingItForActiveAlertEvidence",
    "Keep source=thanos-rule so metric probes are skipped and the source is treated as active-alert evidence only.": "runtimeText.keepSourceThanosRuleSoMetricProbesAreSkippedAndTheSource",
    "Labels must contain 32 entries or fewer.": "runtimeText.labelsMustContain32EntriesOrFewer",
    "Labels must not contain control characters.": "runtimeText.labelsMustNotContainControlCharacters",
    "Merge this child route under the existing Alertmanager route.routes list, then adjust matchers to the alert labels OpenClarion should diagnose.": "runtimeText.mergeThisChildRouteUnderTheExistingAlertmanagerRouteRoutesListThen",
    "Paste the Alertmanager route prefix, the UI alerts page, or /api/v2/alerts. OpenClarion stores the route prefix and tests the active alerts API.": "runtimeText.pasteTheAlertmanagerRoutePrefixTheUiAlertsPageOrApiV2",
    "Paste the Thanos Rule route prefix, /alerts, or /api/v1/alerts. OpenClarion stores the route prefix and only tests active alerts.": "runtimeText.pasteTheThanosRuleRoutePrefixAlertsOrApiV1AlertsOpenclarion",
    "Paste the route prefix, graph page, /api/v1/query, or /api/v1/query_range. OpenClarion stores the route prefix and tests alerts plus vector(1).": "runtimeText.pasteTheRoutePrefixGraphPageApiV1QueryOrApiV1",
    "Prepared an enabled Thanos Rule active-alert source. Paste the Thanos Rule alerts URL or API base URL, then save and test it before adding active-alert evidence. Use Alertmanager for webhook-triggered automatic diagnosis rooms.": "runtimeText.preparedAnEnabledThanosRuleActiveAlertSourcePasteTheThanosRule",
    "Profile name is required.": "runtimeText.profileNameIsRequired",
    "Profile name must be 120 bytes or fewer.": "runtimeText.profileNameMustBe120BytesOrFewer",
    "Provider test passed and active_alerts evidence is bound to this Thanos Rule source.": "runtimeText.providerTestPassedAndActiveAlertsEvidenceIsBoundToThisThanos",
    "Provider test passed and evidence templates exist for active alerts plus metric collection.": "runtimeText.providerTestPassedAndEvidenceTemplatesExistForActiveAlertsPlusMetric",
    "Provider test passed and internal AI diagnosis bindings include active alerts, metric evidence, and an automatic workflow. Apply the Alertmanager receiver route, then retain webhook delivery proof before rollout.": "runtimeText.providerTestPassedAndInternalAiDiagnosisBindingsIncludeActiveAlertsMetric",
    "Provider test passed. Copy the receiver config, create active_alerts plus metric evidence templates, then bind an automatic diagnosis workflow.": "runtimeText.providerTestPassedCopyTheReceiverConfigCreateActiveAlertsPlusMetric",
    "Provider test passed. Create an active_alerts template for this Thanos Rule source; use Thanos Query for metric evidence and Alertmanager for webhook-triggered automatic rooms.": "runtimeText.providerTestPassedCreateAnActiveAlertsTemplateForThisThanosRule",
    "Provider test passed. Create metric and active_alerts templates so diagnosis rooms can collect evidence from this source.": "runtimeText.providerTestPassedCreateMetricAndActiveAlertsTemplatesSoDiagnosisRooms",
    "Reload Alertmanager, then run Test in OpenClarion and send a bounded synthetic alert to confirm webhook delivery.": "runtimeText.reloadAlertmanagerThenRunTestInOpenclarionAndSendABoundedSynthetic",
    "Return to workflow policies after the metric source is tested and evidence templates are ready.": "runtimeText.returnToWorkflowPoliciesAfterTheMetricSourceIsTestedAndEvidence",
    "Run Test to confirm Thanos Rule /api/v1/alerts is reachable.": "runtimeText.runTestToConfirmThanosRuleApiV1AlertsIsReachable",
    "Run Test to confirm active alerts and the vector(1) metric probe both succeed.": "runtimeText.runTestToConfirmActiveAlertsAndTheVector1MetricProbe",
    "Run Test to confirm active=true with silenced, inhibited, and unprocessed alerts excluded.": "runtimeText.runTestToConfirmActiveTrueWithSilencedInhibitedAndUnprocessedAlerts",
    "Thanos Rule source is ready for active-alert evidence from /api/v1/alerts. Use Thanos Query for metric evidence and Alertmanager for webhook intake.": "runtimeText.thanosRuleSourceIsReadyForActiveAlertEvidenceFromApiV1",
    "The persisted source row exposes the OpenClarion webhook receiver URL and scoped Alertmanager route YAML.": "runtimeText.thePersistedSourceRowExposesTheOpenclarionWebhookReceiverUrlAndScoped",
    "This receiver URL is absolute and can be copied into an external Alertmanager route.": "runtimeText.thisReceiverUrlIsAbsoluteAndCanBeCopiedIntoAnExternalAlertmanagerRoute",
    "The profile can be saved as a draft, but workflows and diagnosis tools require it to be enabled.": "runtimeText.theProfileCanBeSavedAsADraftButWorkflowsAndDiagnosis",
    "This looks like a rule-service active-alert URL. If it is Thanos Rule, use the Thanos Rule preset or add source=thanos-rule so OpenClarion skips metric probes and uses this source only for active-alert evidence.": "runtimeText.thisLooksLikeARuleServiceActiveAlertUrlIfItIs",
    "Use Alertmanager as the webhook source for automatic diagnosis rooms; bind Thanos Rule as supplemental active-alert evidence.": "runtimeText.useAlertmanagerAsTheWebhookSourceForAutomaticDiagnosisRoomsBindThanos",
    "Use Alertmanager when alerts should trigger automatic AI diagnosis rooms through webhook delivery. OpenClarion also tests the active alerts API with silenced, inhibited, and unprocessed alerts excluded.": "runtimeText.useAlertmanagerWhenAlertsShouldTriggerAutomaticAiDiagnosisRoomsThroughWebhook",
    "Use Prometheus-compatible sources, including Thanos Query, for metric evidence that raises diagnosis confidence after the initial alert report.": "runtimeText.usePrometheusCompatibleSourcesIncludingThanosQueryForMetricEvidenceThatRaises",
    "Use Thanos Rule as supplemental active-alert evidence. Route webhook-triggered automatic rooms through Alertmanager and use Thanos Query for metric evidence.": "runtimeText.useThanosRuleAsSupplementalActiveAlertEvidenceRouteWebhookTriggeredAutomatic",
    "Use source=thanos for Thanos Query or source=prometheus for a direct Prometheus server.": "runtimeText.useSourceThanosForThanosQueryOrSourcePrometheusForADirect",
    "Use this source for active_alerts evidence templates. Use Thanos Query for metric_query and metric_range evidence.": "runtimeText.useThisSourceForActiveAlertsEvidenceTemplatesUseThanosQueryFor",
    "Use this source for active_alerts, metric_query, and metric_range evidence templates.": "runtimeText.useThisSourceForActiveAlertsMetricQueryAndMetricRangeEvidence",
    "source=thanos-rule": "runtimeText.sourceThanosRule",
    "Active alert evidence complete": "runtimeText.activeAlertEvidenceComplete",
    "Active alert evidence ready": "runtimeText.activeAlertEvidenceReady",
    "Active alert listing": "runtimeText.activeAlertListing",
    "Active alert pull": "runtimeText.activeAlertPull",
    "Active alerts": "runtimeText.activeAlerts",
    "Active alerts disabled.": "runtimeText.activeAlertsDisabled",
    "Active alerts ready.": "runtimeText.activeAlertsReady",
    "alert webhook intake or active-alert evidence": "runtimeText.alertWebhookIntakeOrActiveAlertEvidence",
    "AI Channel": "runtimeText.aiChannel",
    "AI channel": "runtimeText.aiChannel2",
    "Alert API": "runtimeText.alertApi",
    "Alert Tool": "runtimeText.alertTool",
    "Alert intake disabled.": "runtimeText.alertIntakeDisabled",
    "Alert intake ready.": "runtimeText.alertIntakeReady",
    "Alert tool": "runtimeText.alertTool2",
    "Alertmanager": "runtimeText.alertmanager",
    "Alertmanager alert intake": "runtimeText.alertmanagerAlertIntake",
    "Alertmanager integration": "runtimeText.alertmanagerIntegration",
    "Alertmanager route prefix": "runtimeText.alertmanagerRoutePrefix",
    "Alertmanager reads active alerts with silenced, inhibited, and unprocessed alerts filtered out.": "runtimeText.alertmanagerReadsFilteredActiveAlerts",
    "Alertmanager webhook ingest": "runtimeText.alertmanagerWebhookIngest",
    "Auto Workflow": "runtimeText.autoWorkflow",
    "Auto workflow": "runtimeText.autoWorkflow2",
    "Automatic diagnosis trigger": "runtimeText.automaticDiagnosisTrigger",
    "Back to workflow": "runtimeText.backToWorkflow",
    "Base URL": "runtimeText.baseUrl",
    "Bearer auth requires a secret reference.": "runtimeText.bearerAuthRequiresASecretReference",
    "Bind route": "runtimeText.bindRoute",
    "Complete source configuration.": "runtimeText.completeSourceConfiguration",
    "Copy the persisted webhook endpoint from the source row.": "runtimeText.copyPersistedWebhookEndpoint",
    "Confidence-building evidence": "runtimeText.confidenceBuildingEvidence",
    "Connection test": "runtimeText.connectionTest",
    "Connection test blocked": "runtimeText.connectionTestBlocked",
    "Connection test required": "runtimeText.connectionTestRequired",
    "Create metric evidence templates for instant and range queries.": "runtimeText.createMetricEvidenceTemplatesForInstantAndRangeQueries",
    "Diagnosis tools": "runtimeText.diagnosisTools",
    "Enable Workflow": "runtimeText.enableWorkflow",
    "Enabled source": "runtimeText.enabledSource",
    "Enabled Alertmanager profile can be saved.": "runtimeText.enabledAlertmanagerProfileCanBeSaved",
    "Enabled Prometheus-compatible profile can be saved.": "runtimeText.enabledPrometheusCompatibleProfileCanBeSaved",
    "Enabled Thanos Rule active-alert profile can be saved.": "runtimeText.enabledThanosRuleProfileCanBeSaved",
    "Evidence setup complete": "runtimeText.evidenceSetupComplete",
    "Evidence setup ready": "runtimeText.evidenceSetupReady",
    "Generic Alertmanager-compatible active alerts and webhooks.": "runtimeText.genericAlertmanagerCompatibleActiveAlertsAndWebhooks",
    "Generic Prometheus-compatible alerts and metric evidence.": "runtimeText.genericPrometheusCompatibleAlertsAndMetricEvidence",
    "Instant metric evidence": "runtimeText.instantMetricEvidence",
    "Labels": "runtimeText.labels",
    "Last connection test did not pass.": "runtimeText.lastConnectionTestDidNotPass",
    "Last connection test passed.": "runtimeText.lastConnectionTestPassed",
    "Load diagnosis tool templates to check active_alerts coverage.": "runtimeText.loadDiagnosisToolTemplatesToCheckActiveAlertsCoverage",
    "Load diagnosis tool templates to check metric evidence coverage.": "runtimeText.loadDiagnosisToolTemplatesToCheckMetricEvidenceCoverage",
    "Load report workflow policies to check whether an enabled automatic workflow already binds an Enterprise WeChat AI channel.": "runtimeText.loadReportWorkflowPoliciesToCheckWhetherAnEnabledAutomaticWorkflowAlready",
    "Load workflow policies to check automatic diagnosis binding.": "runtimeText.loadWorkflowPoliciesToCheckAutomaticDiagnosisBinding",
    "Metric Source": "runtimeText.metricSource",
    "Metric Tool": "runtimeText.metricTool",
    "Metric evidence": "runtimeText.metricEvidence",
    "Metric evidence disabled.": "runtimeText.metricEvidenceDisabled",
    "Metric evidence ready.": "runtimeText.metricEvidenceReady",
    "Metric evidence source": "runtimeText.metricEvidenceSource",
    "metric evidence collection": "runtimeText.metricEvidenceCollection",
    "Metric probe": "runtimeText.metricProbe",
    "Metric queries not required": "runtimeText.metricQueriesNotRequired",
    "Metric tools": "runtimeText.metricTools",
    "No Authorization header": "runtimeText.noAuthorizationHeader",
    "Prepared an enabled Alertmanager source. Paste the base URL, then save and test it before binding workflows.": "runtimeText.preparedAnEnabledAlertmanagerSourcePasteTheBaseUrlThenSaveAnd",
    "Prepared an enabled Prometheus-compatible source. Paste the base URL, then save and test it before adding metric evidence tools.": "runtimeText.preparedAnEnabledPrometheusCompatibleSourcePasteTheBaseUrlThenSave",
    "Prepared an enabled Thanos Query source. Paste the base URL, then save and test it before adding metric evidence tools.": "runtimeText.preparedAnEnabledThanosQuerySourcePasteTheBaseUrlThenSave",
    "Prometheus": "runtimeText.prometheus",
    "Prometheus metric evidence": "runtimeText.prometheusMetricEvidence",
    "Prometheus-compatible": "runtimeText.prometheusCompatible",
    "Prometheus-compatible active alerts from Thanos Rule.": "runtimeText.prometheusCompatibleActiveAlertsFromThanosRule",
    "Prometheus-compatible integration": "runtimeText.prometheusCompatibleIntegration",
    "Prometheus-compatible metric evidence.": "runtimeText.prometheusCompatibleMetricEvidence",
    "Prometheus-compatible query API": "runtimeText.prometheusCompatibleQueryApi",
    "Prometheus-compatible sources support active alert reads and metric evidence collection.": "runtimeText.prometheusCompatibleSourcesSupportAlertsAndMetrics",
    "Range metric evidence": "runtimeText.rangeMetricEvidence",
    "Receiver after save": "runtimeText.receiverAfterSave",
    "Receiver route": "runtimeText.receiverRoute",
    "Reload and test": "runtimeText.reloadAndTest",
    "Review Workflow": "runtimeText.reviewWorkflow",
    "Review Thanos Rule classification.": "runtimeText.reviewThanosRuleClassification",
    "Run Test to verify provider reachability and credentials.": "runtimeText.runTestToVerifyProviderReachabilityAndCredentials",
    "Save an enabled Prometheus-compatible profile first.": "runtimeText.saveAnEnabledPrometheusCompatibleProfileFirst",
    "Save an enabled Alertmanager profile first.": "runtimeText.saveAnEnabledAlertmanagerProfileFirst",
    "Save an enabled Thanos Rule active-alert profile first.": "runtimeText.saveAnEnabledThanosRuleProfileFirst",
    "Set NEXT_PUBLIC_OPENCLARION_API_PUBLIC_BASE_URL to an externally reachable OpenClarion URL before copying this receiver to an external Alertmanager.": "runtimeText.setPublicBaseUrlBeforeCopyingReceiver",
    "Secret reference requires bearer auth.": "runtimeText.secretReferenceRequiresBearerAuth",
    "Secret reference must be 256 bytes or fewer.": "runtimeText.secretReferenceMustBe256BytesOrFewer",
    "Secret reference must not contain whitespace or control characters.": "runtimeText.secretReferenceMustNotContainWhitespaceOrControlCharacters",
    "Silenced/inhibited alerts ignored": "runtimeText.silencedInhibitedAlertsIgnored",
    "Source disabled": "runtimeText.sourceDisabled",
    "Source profile": "runtimeText.sourceProfile",
    "Source ready for workflows.": "runtimeText.sourceReadyForWorkflows",
    "Source will be saved as draft.": "runtimeText.sourceWillBeSavedAsDraft",
    "Supplemental active alerts": "runtimeText.supplementalActiveAlerts",
    "Thanos Query": "runtimeText.thanosQuery",
    "Thanos Rule": "runtimeText.thanosRule",
    "Thanos Rule active alerts": "runtimeText.thanosRuleActiveAlerts",
    "Thanos Rule alerts API": "runtimeText.thanosRuleAlertsApi",
    "Thanos Rule active-alert sources read firing alerts from /api/v1/alerts. Use Thanos Query for metric evidence and Alertmanager for webhook-triggered automatic diagnosis rooms.": "runtimeText.thanosRuleSourcesReadFiringAlerts",
    "Thanos metric evidence": "runtimeText.thanosMetricEvidence",
    "Webhook endpoint appears after save.": "runtimeText.webhookEndpointAppearsAfterSave",
    "the source secret reference": "runtimeText.sourceSecretReference",
    "Thanos Rule integration": "runtimeText.thanosRuleIntegration",
    "Webhook intake": "runtimeText.webhookIntake",
    "Webhook proof": "runtimeText.webhookProof",
    "Webhook proof needed": "runtimeText.webhookProofNeeded",
    "Webhook setup ready": "runtimeText.webhookSetupReady",
    "Workflow": "runtimeText.workflow",
    "Workflow binding": "runtimeText.workflowBinding",
    "Copy the receiver YAML from the Ingest column, bind it to a scoped Alertmanager route, and reload Alertmanager.": "runtimeText.copyReceiverYamlFromIngest",
    "Receiver YAML appears after the source is saved.": "runtimeText.receiverYamlAppearsAfterSave",
    "Send a bounded synthetic firing alert or wait for a controlled route match, then confirm OpenClarion ingested the webhook and started the expected automatic diagnosis room.": "runtimeText.sendBoundedSyntheticAlert",
    "Webhook delivery proof requires an enabled source and a successful provider connection test first.": "runtimeText.webhookProofRequiresEnabledSource",
} as const;

export function localizeAlertSourceText(value: string, t: AlertSourceTranslator): string {
  const key =
    alertSourceRuntimeTextKeys[
      value as keyof typeof alertSourceRuntimeTextKeys
    ];
  if (key !== undefined) {
    return t(key);
  }
  let match = value.match(/^Authorization: Bearer token resolved from (.+)$/);
  if (match) {
    return t("runtimePattern.authorizationBearerResolved", {
      source: match[1]!,
    });
  }
  match = value.match(/^Enable this (.+) source before using it for (.+)\.$/);
  if (match) {
    return t("runtimePattern.enableSourceBeforeUse", {
      purpose: localizeAlertSourceText(match[2]!, t),
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Run Test for this (.+) source before relying on AI diagnosis workflow setup\.$/);
  if (match) {
    return t("runtimePattern.runTestBeforeAISetup", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Last connection test ended with (.+)\. Resolve the provider or credential issue before continuing setup\.$/);
  if (match) {
    return t("runtimePattern.lastConnectionTestEnded", {
      status: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Label line (\d+) must use key=value\.$/);
  if (match) {
    return t("runtimePattern.labelLineKeyValue", { line: match[1]! });
  }
  match = value.match(/^Label line (\d+) has an empty key\.$/);
  if (match) {
    return t("runtimePattern.labelLineEmptyKey", { line: match[1]! });
  }
  match = value.match(/^Label line (\d+) exceeds the allowed length\.$/);
  if (match) {
    return t("runtimePattern.labelLineTooLong", { line: match[1]! });
  }
  match = value.match(/^Label key "(.+)" is duplicated\.$/);
  if (match) {
    return t("runtimePattern.labelKeyDuplicated", { key: match[1]! });
  }
  match = value.match(/^(.+) source is ready for instant and range metric evidence tools\.$/);
  if (match) {
    return t("runtimePattern.sourceReadyForMetricEvidence", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Create an active_alerts evidence template bound to this (.+) source\.$/);
  if (match) {
    return t("runtimePattern.createActiveAlertsTemplate", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Create or update an automatic diagnosis workflow that uses this (.+) webhook source\.$/);
  if (match) {
    return t("runtimePattern.createAutomaticWorkflow", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Enable this (.+) source, save it, then run Test before configuring AI diagnosis setup\.$/);
  if (match) {
    return t("runtimePattern.enableSaveAndTest", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Ready for (.+) webhook intake and active-alert evidence in automatic AI diagnosis workflows\.$/);
  if (match) {
    return t("runtimePattern.readyForWebhookIntake", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Copy the receiver YAML into Alertmanager receivers as (.+)\.$/);
  if (match) {
    return t("runtimePattern.copyReceiverYAML", { name: match[1]! });
  }
  match = value.match(/^Add a scoped Alertmanager route that selects alerts OpenClarion should diagnose and sends them to (.+)\. Use continue: true only when existing downstream receivers should also run\.$/);
  if (match) {
    return t("runtimePattern.addScopedRoute", { receiver: match[1]! });
  }
  match = value.match(/^Review the enabled automatic diagnosis workflow that uses this (.+) webhook source\.$/);
  if (match) {
    return t("runtimePattern.reviewEnabledAutomaticWorkflow", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^Enable or review the existing automatic diagnosis workflow that uses this (.+) webhook source\.$/);
  if (match) {
    return t("runtimePattern.enableOrReviewAutomaticWorkflow", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^(.+) source is saved and enabled\.$/);
  if (match) {
    return t("runtimePattern.sourceSavedAndEnabled", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^(.+) source must be enabled before workflow setup\.$/);
  if (match) {
    return t("runtimePattern.sourceMustBeEnabledBeforeWorkflowSetup", {
      source: localizeAlertSourceText(match[1]!, t),
    });
  }
  match = value.match(/^(\d+) enabled active_alerts template\(s\) are bound to this source\.$/);
  if (match) {
    return t("runtimePattern.enabledActiveAlertTemplatesBound", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) active_alerts template\(s\) exist for this source but are disabled\.$/);
  if (match) {
    return t("runtimePattern.activeAlertTemplatesDisabled", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) enabled metric evidence template\(s\) are bound to this source\.$/);
  if (match) {
    return t("runtimePattern.enabledMetricTemplatesBound", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) metric evidence template\(s\) exist for this source but are disabled\.$/);
  if (match) {
    return t("runtimePattern.metricTemplatesDisabled", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) enabled metric evidence template\(s\) are available across Prometheus-compatible sources\.$/);
  if (match) {
    return t("runtimePattern.enabledMetricTemplatesAvailable", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) metric evidence source\(s\) exist\. Create a metric_query or metric_range_query template before relying on automatic diagnosis confidence\.$/);
  if (match) {
    return t("runtimePattern.metricEvidenceSourcesNeedTemplate", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) enabled automatic diagnosis workflow\(s\) are bound to this source; workflow enablement covers the required WeCom scopes and AI delivery proof\.$/);
  if (match) {
    return t("runtimePattern.enabledAutomaticWorkflowsBoundWithProof", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) automatic diagnosis workflow\(s\) exist for this source but are disabled; enablement requires WeCom report, diagnosis_consultation, diagnosis_close scopes, and current AI proof\.$/);
  if (match) {
    return t("runtimePattern.automaticWorkflowsDisabledWithProof", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) enabled automatic diagnosis workflow\(s\) are bound to this source\.$/);
  if (match) {
    return t("runtimePattern.enabledAutomaticWorkflowsBound", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) automatic diagnosis workflow\(s\) exist for this source but are disabled\.$/);
  if (match) {
    return t("runtimePattern.automaticWorkflowsDisabled", {
      count: Number(match[1]),
    });
  }
  return value;
}
