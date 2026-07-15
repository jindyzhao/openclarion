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
      setNotice({ kind: "error", message: localizeAlertSourceText(parsed.message, locale) });
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
          description={localizeAlertSourceText(launchNotice, locale)}
          message={t("actionLoaded")}
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      <AlertSourceWorkflowReturnPanel locale={locale} t={t} workflowReturn={workflowReturn} />

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
                locale={locale}
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
                    locale={locale}
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
                    locale={locale}
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
  locale,
  t,
  workflowReturn,
}: {
  locale: string;
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
          {localizeAlertSourceText(workflowReturn.label, locale)}
        </Button>
      }
      aria-label={t("workflowReturnLabel")}
      description={localizeAlertSourceText(workflowReturn.detail, locale)}
      message={t("workflowReturn")}
      role="status"
      showIcon
      type="info"
    />
  );
}

function AlertSourcePresetControls({
  disabled,
  locale,
  onApply,
  presets,
  t,
}: {
  disabled: boolean;
  locale: string;
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
            title={localizeAlertSourceText(preset.detail, locale)}
            type="default"
          >
            {localizeAlertSourceText(preset.label, locale)}
          </Button>
        ))}
      </Space>
    </Form.Item>
  );
}

function ConnectionTargetPreview({
  canonicalBaseURL,
  locale,
  result,
  t,
}: {
  canonicalBaseURL: ReturnType<typeof alertSourceCanonicalBaseURL>;
  locale: string;
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
            {localizeAlertSourceText(target.label, locale)}
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
  locale,
  t,
  values,
}: {
  locale: string;
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
        <ConnectionTargetsDescription locale={locale} targets={connection.value} />
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
        <Typography.Text strong>{localizeAlertSourceText(readiness.label, locale)}</Typography.Text>
        <Typography.Text type="secondary">{localizeAlertSourceText(readiness.detail, locale)}</Typography.Text>
        {classificationHint === null ? null : (
          <Alert
            description={
              <Space direction="vertical" size={4}>
                <Typography.Text>{localizeAlertSourceText(classificationHint.detail, locale)}</Typography.Text>
                <Typography.Text
                  className="settings-code-cell"
                  copyable={{ text: classificationHint.suggestedLabelsText }}
                  type="secondary"
                >
                  {classificationHint.suggestedLabelsText}
                </Typography.Text>
              </Space>
            }
            message={localizeAlertSourceText(classificationHint.label, locale)}
            showIcon
            type="warning"
          />
        )}
        <Space wrap>
          {readiness.capabilities.map((capability) => (
            <Tag key={capability}>{localizeAlertSourceText(capability, locale)}</Tag>
          ))}
        </Space>
        <Alert
          description={
            <Space direction="vertical" size={6}>
              <Typography.Text>{localizeAlertSourceText(providerGuidance.detail, locale)}</Typography.Text>
              {providerGuidance.items.map((item) => (
                <Space align="start" key={item.key} size={6}>
                  <Tag>{item.value}</Tag>
                  <Space direction="vertical" size={0}>
                    <Typography.Text>{localizeAlertSourceText(item.label, locale)}</Typography.Text>
                    <Typography.Text type="secondary">
                      {localizeAlertSourceText(item.detail, locale)}
                    </Typography.Text>
                  </Space>
                </Space>
              ))}
            </Space>
          }
          message={localizeAlertSourceText(providerGuidance.label, locale)}
          showIcon
          type="info"
        />
        <Descriptions column={1} items={items} size="small" />
        <OperatorChecklist locale={locale} steps={operatorChecklist} t={t} />
      </Space>
    </div>
  );
}

function ConnectionTargetsDescription({
  locale,
  targets,
}: {
  locale: string;
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
          {localizeAlertSourceText(target.label, locale)}: {target.value}
        </Typography.Text>
      ))}
    </Space>
  );
}

function OperatorChecklist({
  locale,
  steps,
  t,
}: {
  locale: string;
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
            <Typography.Text strong>{localizeAlertSourceText(step.label, locale)}</Typography.Text>
            <Typography.Text type="secondary">{localizeAlertSourceText(step.detail, locale)}</Typography.Text>
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
      render: (_value, profile) => <AlertSourceAIRole locale={locale} profile={profile} t={t} />,
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
        <IngestEndpoint locale={locale} profile={profile} publicAPIBaseURL={publicAPIBaseURL} t={t} />
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
          locale={locale}
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
  locale,
  profile,
  t,
}: {
  locale: string;
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
        ellipsis={{ tooltip: localizeAlertSourceText(readiness.detail, locale) }}
        type="secondary"
      >
        {localizeAlertSourceText(readiness.label, locale)}
      </Typography.Text>
    </Space>
  );
}

function AlertSourceNextSetupActions({
  bindings,
  locale,
  profile,
  result,
  workflowReturn,
}: {
  bindings?: AlertSourceAutomationBindings;
  locale: string;
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
          {localizeAlertSourceText(readiness.label, locale)}
        </Tag>
      </Space>
      <Typography.Text ellipsis={{ tooltip: localizeAlertSourceText(readiness.detail, locale) }} type="secondary">
        {localizeAlertSourceText(readiness.detail, locale)}
      </Typography.Text>
      <Space size={[4, 4]} wrap>
        {readiness.steps.map((step) => (
          <Tooltip key={step.key} title={localizeAlertSourceText(step.detail, locale)}>
            <Tag color={alertSourceReadinessColor(step.status)}>
              {localizeAlertSourceText(step.label, locale)}
            </Tag>
          </Tooltip>
        ))}
      </Space>
      <Space wrap>
        {alertSourceNextSetupActions(profile, {
          bindings,
          workflowReturn,
        }).map((action) => (
          <Tooltip key={action.key} title={localizeAlertSourceText(action.detail, locale)}>
            <Button
              href={action.href}
              icon={alertSourceNextSetupActionIcon(action.key)}
              size="small"
              type="link"
            >
              {localizeAlertSourceText(action.label, locale)}
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
  locale,
  profile,
  publicAPIBaseURL,
  t,
}: {
  locale: string;
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
        ellipsis={{ tooltip: localizeAlertSourceText(config.endpointGuidance, locale) }}
        type={config.endpointScope === "absolute" ? "secondary" : "warning"}
      >
        {localizeAlertSourceText(config.endpointGuidance, locale)}
      </Typography.Text>
      <Typography.Text type="secondary">{localizeAlertSourceText(config.authorization, locale)}</Typography.Text>
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
          ellipsis={{ tooltip: localizeAlertSourceText(config.routeGuidance, locale) }}
          type="secondary"
        >
          {localizeAlertSourceText(config.routeGuidance, locale)}
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
            <Tag>{localizeAlertSourceText(item.label, locale)}</Tag>
            <Typography.Text
              ellipsis={{ tooltip: localizeAlertSourceText(item.detail, locale) }}
              type="secondary"
            >
              {localizeAlertSourceText(item.detail, locale)}
            </Typography.Text>
          </Space>
        ))}
      </Space>
      <Typography.Text
        ellipsis={{ tooltip: localizeAlertSourceText(config.detail, locale) }}
        type="secondary"
      >
        {localizeAlertSourceText(config.detail, locale)}
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
        {localizeAlertSourceText(result.status, locale)}
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

function localizeAlertSourceText(value: string, locale: string): string {
  if (locale !== "zh-CN") {
    return value;
  }
  const exact: Readonly<Record<string, string>> = {
    blocked: "已阻塞",
    failed: "失败",
    success: "成功",
    unsupported: "不支持",
    "Add receiver": "添加接收器",
    "Base URL is required.": "基础 URL 为必填项。",
    "Base URL must be 2048 bytes or fewer.": "基础 URL 不能超过 2048 字节。",
    "Base URL must be a valid URL.": "基础 URL 必须是有效 URL。",
    "Base URL must not include query or fragment.":
      "基础 URL 不能包含查询参数或片段。",
    "Base URL must not include userinfo.": "基础 URL 不能包含用户信息。",
    "Base URL scheme must be http or https.":
      "基础 URL 协议必须是 http 或 https。",
    "Bind active_alerts, metric evidence, Enterprise WeChat delivery, and an auto-room workflow before rollout.":
      "上线前请绑定 active_alerts、指标证据、企业微信交付和自动诊断室工作流。",
    "Bind this source to grouping, report replay, and diagnosis evidence workflows.":
      "请将此告警源绑定到分组、报告重放和诊断证据工作流。",
    "Bind this source to report replay and diagnosis evidence workflows.":
      "请将此告警源绑定到报告重放和诊断证据工作流。",
    "Configure Alertmanager, or another Alertmanager webhook-compatible sender, to POST webhook v4 JSON to this endpoint. Thanos Rule alerts should normally route through Alertmanager first. Resolved, silenced, inhibited, and muted alerts are ignored during ingest.":
      "请配置 Alertmanager 或其他兼容 Alertmanager Webhook 的发送方，将 Webhook v4 JSON 通过 POST 发送到此端点。Thanos Rule 告警通常应先路由到 Alertmanager。接入时会忽略已恢复、静默、抑制和停用的告警。",
    "Configure a WeCom channel with report, diagnosis_consultation, and diagnosis_close scopes; workflow setup verifies delivery proof.":
      "请配置具有 report、diagnosis_consultation 和 diagnosis_close 范围的企业微信渠道；工作流配置会验证交付证明。",
    "Create a WeCom report and AI-room channel for report, diagnosis consultation, and close notifications.":
      "请创建企业微信报告与 AI 诊断室渠道，用于报告、诊断会诊和关闭通知。",
    "Create a metric evidence template bound to this Prometheus-compatible source.":
      "请创建绑定到此 Prometheus 兼容告警源的指标证据模板。",
    "Create a Prometheus-compatible metric evidence source, usually Thanos Query, and add metric_query or metric_range_query templates.":
      "请创建 Prometheus 兼容的指标证据源（通常为 Thanos Query），并添加 metric_query 或 metric_range_query 模板。",
    "Create an active_alerts evidence template bound to this Prometheus-compatible source.":
      "请创建绑定到此 Prometheus 兼容告警源的 active_alerts 证据模板。",
    "Create an active_alerts evidence template bound to this Thanos Rule source.":
      "请创建绑定到此 Thanos Rule 告警源的 active_alerts 证据模板。",
    "Create an active_alerts evidence template for this source; do not use it as the metric confidence source.":
      "请为此告警源创建 active_alerts 证据模板；不要将其用作指标置信度来源。",
    "Create an active_alerts evidence template for this source.":
      "请为此告警源创建 active_alerts 证据模板。",
    "Create metric_query or metric_range_query evidence templates so AI diagnosis can request follow-up metrics before finalizing.":
      "请创建 metric_query 或 metric_range_query 证据模板，使 AI 诊断能在定稿前请求后续指标。",
    "Create or enable a Prometheus-compatible metric evidence source, usually Thanos Query, for AI confidence-building queries.":
      "请创建或启用 Prometheus 兼容的指标证据源（通常为 Thanos Query），用于提升 AI 诊断置信度的查询。",
    "Create or update an automatic diagnosis workflow after the alert tool is ready.":
      "告警工具就绪后，请创建或更新自动诊断工作流。",
    "Enable this Thanos Rule source before using it for active-alert evidence.":
      "将此 Thanos Rule 告警源用于活动告警证据前，请先启用。",
    "Keep source=thanos-rule so metric probes are skipped and the source is treated as active-alert evidence only.":
      "请保留 source=thanos-rule，以跳过指标探测并仅将此告警源用作活动告警证据。",
    "Labels must contain 32 entries or fewer.": "标签不能超过 32 项。",
    "Labels must not contain control characters.": "标签不能包含控制字符。",
    "Merge this child route under the existing Alertmanager route.routes list, then adjust matchers to the alert labels OpenClarion should diagnose.":
      "请将此子路由合并到现有 Alertmanager route.routes 列表中，并按 OpenClarion 应诊断的告警标签调整匹配器。",
    "Paste the Alertmanager route prefix, the UI alerts page, or /api/v2/alerts. OpenClarion stores the route prefix and tests the active alerts API.":
      "请粘贴 Alertmanager 路由前缀、UI 告警页或 /api/v2/alerts。OpenClarion 会保存路由前缀并测试活动告警 API。",
    "Paste the Thanos Rule route prefix, /alerts, or /api/v1/alerts. OpenClarion stores the route prefix and only tests active alerts.":
      "请粘贴 Thanos Rule 路由前缀、/alerts 或 /api/v1/alerts。OpenClarion 会保存路由前缀并仅测试活动告警。",
    "Paste the route prefix, graph page, /api/v1/query, or /api/v1/query_range. OpenClarion stores the route prefix and tests alerts plus vector(1).":
      "请粘贴路由前缀、图表页、/api/v1/query 或 /api/v1/query_range。OpenClarion 会保存路由前缀，并测试告警与 vector(1)。",
    "Prepared an enabled Thanos Rule active-alert source. Paste the Thanos Rule alerts URL or API base URL, then save and test it before adding active-alert evidence. Use Alertmanager for webhook-triggered automatic diagnosis rooms.":
      "已准备启用的 Thanos Rule 活动告警源。请填写 Thanos Rule 告警 URL 或 API 基础 URL，保存并测试后再添加活动告警证据。由 Webhook 触发的自动诊断室请使用 Alertmanager。",
    "Profile name is required.": "配置名称为必填项。",
    "Profile name must be 120 bytes or fewer.": "配置名称不能超过 120 字节。",
    "Provider test passed and active_alerts evidence is bound to this Thanos Rule source.":
      "提供方测试已通过，active_alerts 证据已绑定到此 Thanos Rule 告警源。",
    "Provider test passed and evidence templates exist for active alerts plus metric collection.":
      "提供方测试已通过，活动告警和指标采集证据模板均已存在。",
    "Provider test passed and internal AI diagnosis bindings include active alerts, metric evidence, and an automatic workflow. Apply the Alertmanager receiver route, then retain webhook delivery proof before rollout.":
      "提供方测试已通过，内部 AI 诊断绑定已包含活动告警、指标证据和自动工作流。请应用 Alertmanager 接收器路由，并在上线前保留 Webhook 交付证明。",
    "Provider test passed. Copy the receiver config, create active_alerts plus metric evidence templates, then bind an automatic diagnosis workflow.":
      "提供方测试已通过。请复制接收器配置，创建 active_alerts 与指标证据模板，然后绑定自动诊断工作流。",
    "Provider test passed. Create an active_alerts template for this Thanos Rule source; use Thanos Query for metric evidence and Alertmanager for webhook-triggered automatic rooms.":
      "提供方测试已通过。请为此 Thanos Rule 告警源创建 active_alerts 模板；指标证据使用 Thanos Query，由 Webhook 触发的自动诊断室使用 Alertmanager。",
    "Provider test passed. Create metric and active_alerts templates so diagnosis rooms can collect evidence from this source.":
      "提供方测试已通过。请创建指标和 active_alerts 模板，使诊断室能够从此告警源采集证据。",
    "Reload Alertmanager, then run Test in OpenClarion and send a bounded synthetic alert to confirm webhook delivery.":
      "请重载 Alertmanager，然后在 OpenClarion 中运行测试并发送有界合成告警，以确认 Webhook 交付。",
    "Return to workflow policies after the metric source is tested and evidence templates are ready.":
      "指标源测试通过且证据模板就绪后，请返回工作流策略。",
    "Run Test to confirm Thanos Rule /api/v1/alerts is reachable.":
      "请运行测试以确认 Thanos Rule /api/v1/alerts 可访问。",
    "Run Test to confirm active alerts and the vector(1) metric probe both succeed.":
      "请运行测试以确认活动告警和 vector(1) 指标探测均成功。",
    "Run Test to confirm active=true with silenced, inhibited, and unprocessed alerts excluded.":
      "请运行测试以确认 active=true，并排除静默、抑制和未处理的告警。",
    "Thanos Rule source is ready for active-alert evidence from /api/v1/alerts. Use Thanos Query for metric evidence and Alertmanager for webhook intake.":
      "Thanos Rule 告警源已可通过 /api/v1/alerts 提供活动告警证据。指标证据请使用 Thanos Query，Webhook 接入请使用 Alertmanager。",
    "The persisted source row exposes the OpenClarion webhook receiver URL and scoped Alertmanager route YAML.":
      "保存后的告警源记录会显示 OpenClarion Webhook 接收器 URL 和限定范围的 Alertmanager 路由 YAML。",
    "The profile can be saved as a draft, but workflows and diagnosis tools require it to be enabled.":
      "此配置可以保存为草稿，但工作流和诊断工具要求先启用。",
    "This looks like a rule-service active-alert URL. If it is Thanos Rule, use the Thanos Rule preset or add source=thanos-rule so OpenClarion skips metric probes and uses this source only for active-alert evidence.":
      "此 URL 看起来是规则服务的活动告警地址。如果它属于 Thanos Rule，请使用 Thanos Rule 预设或添加 source=thanos-rule，使 OpenClarion 跳过指标探测并仅将其用于活动告警证据。",
    "Use Alertmanager as the webhook source for automatic diagnosis rooms; bind Thanos Rule as supplemental active-alert evidence.":
      "自动诊断室的 Webhook 告警源请使用 Alertmanager，并将 Thanos Rule 绑定为补充活动告警证据。",
    "Use Alertmanager when alerts should trigger automatic AI diagnosis rooms through webhook delivery. OpenClarion also tests the active alerts API with silenced, inhibited, and unprocessed alerts excluded.":
      "当告警需要通过 Webhook 交付触发自动 AI 诊断室时，请使用 Alertmanager。OpenClarion 还会测试活动告警 API，并排除静默、抑制和未处理的告警。",
    "Use Prometheus-compatible sources, including Thanos Query, for metric evidence that raises diagnosis confidence after the initial alert report.":
      "请使用 Prometheus 兼容告警源（包括 Thanos Query）提供指标证据，以在初始告警报告后提高诊断置信度。",
    "Use Thanos Rule as supplemental active-alert evidence. Route webhook-triggered automatic rooms through Alertmanager and use Thanos Query for metric evidence.":
      "请将 Thanos Rule 用作补充活动告警证据。由 Webhook 触发的自动诊断室通过 Alertmanager 路由，指标证据使用 Thanos Query。",
    "Use source=thanos for Thanos Query or source=prometheus for a direct Prometheus server.":
      "Thanos Query 请使用 source=thanos，直连 Prometheus 服务器请使用 source=prometheus。",
    "Use this source for active_alerts evidence templates. Use Thanos Query for metric_query and metric_range evidence.":
      "请将此告警源用于 active_alerts 证据模板，并使用 Thanos Query 提供 metric_query 和 metric_range 证据。",
    "Use this source for active_alerts, metric_query, and metric_range evidence templates.":
      "请将此告警源用于 active_alerts、metric_query 和 metric_range 证据模板。",
    "source=thanos-rule": "source=thanos-rule",
    "Active alert evidence complete": "活动告警证据已配置",
    "Active alert evidence ready": "可配置活动告警证据",
    "Active alert listing": "活动告警列表",
    "Active alert pull": "拉取活动告警",
    "Active alerts": "活动告警",
    "Active alerts disabled.": "活动告警已停用。",
    "Active alerts ready.": "活动告警已就绪。",
    "alert webhook intake or active-alert evidence":
      "告警 Webhook 接入或活动告警证据",
    "AI Channel": "AI 通知渠道",
    "AI channel": "AI 通知渠道",
    "Alert API": "告警 API",
    "Alert Tool": "告警工具",
    "Alert intake disabled.": "告警接入已停用。",
    "Alert intake ready.": "告警接入已就绪。",
    "Alert tool": "告警工具",
    "Alertmanager": "Alertmanager",
    "Alertmanager alert intake": "Alertmanager 告警接入",
    "Alertmanager integration": "Alertmanager 集成",
    "Alertmanager route prefix": "Alertmanager 路由前缀",
    "Alertmanager webhook ingest": "Alertmanager Webhook 接入",
    "Auto Workflow": "自动诊断工作流",
    "Auto workflow": "自动诊断工作流",
    "Automatic diagnosis trigger": "自动诊断触发器",
    "Back to workflow": "返回工作流",
    "Base URL": "基础 URL",
    "Bearer auth requires a secret reference.": "Bearer 认证需要密钥引用。",
    "Bind route": "绑定路由",
    "Complete source configuration.": "请完成告警源配置。",
    "Confidence-building evidence": "用于提升置信度的证据",
    "Connection test": "连接测试",
    "Connection test blocked": "连接测试已阻塞",
    "Connection test required": "需要连接测试",
    "Create metric evidence templates for instant and range queries.":
      "请创建即时查询和范围查询的指标证据模板。",
    "Diagnosis tools": "诊断工具",
    "Enable Workflow": "启用工作流",
    "Enabled source": "已启用告警源",
    "Evidence setup complete": "证据配置已完成",
    "Evidence setup ready": "可以配置证据",
    "Generic Alertmanager-compatible active alerts and webhooks.":
      "通用的 Alertmanager 兼容活动告警与 Webhook。",
    "Generic Prometheus-compatible alerts and metric evidence.":
      "通用的 Prometheus 兼容告警与指标证据。",
    "Instant metric evidence": "即时指标证据",
    "Labels": "标签",
    "Last connection test did not pass.": "最近一次连接测试未通过。",
    "Last connection test passed.": "最近一次连接测试已通过。",
    "Load diagnosis tool templates to check active_alerts coverage.":
      "请加载诊断工具模板以检查 active_alerts 覆盖情况。",
    "Load diagnosis tool templates to check metric evidence coverage.":
      "请加载诊断工具模板以检查指标证据覆盖情况。",
    "Load report workflow policies to check whether an enabled automatic workflow already binds an Enterprise WeChat AI channel.":
      "请加载报告工作流策略，检查已启用的自动工作流是否绑定企业微信 AI 渠道。",
    "Load workflow policies to check automatic diagnosis binding.":
      "请加载工作流策略以检查自动诊断绑定。",
    "Metric Source": "指标源",
    "Metric Tool": "指标工具",
    "Metric evidence": "指标证据",
    "Metric evidence disabled.": "指标证据已停用。",
    "Metric evidence ready.": "指标证据已就绪。",
    "Metric evidence source": "指标证据源",
    "metric evidence collection": "指标证据采集",
    "Metric probe": "指标探测",
    "Metric queries not required": "无需指标查询",
    "Metric tools": "指标工具",
    "No Authorization header": "不发送 Authorization 请求头",
    "Prepared an enabled Alertmanager source. Paste the base URL, then save and test it before binding workflows.":
      "已准备启用的 Alertmanager 告警源。请填写基础 URL，保存并测试后再绑定工作流。",
    "Prepared an enabled Prometheus-compatible source. Paste the base URL, then save and test it before adding metric evidence tools.":
      "已准备启用的 Prometheus 兼容告警源。请填写基础 URL，保存并测试后再添加指标证据工具。",
    "Prepared an enabled Thanos Query source. Paste the base URL, then save and test it before adding metric evidence tools.":
      "已准备启用的 Thanos Query 告警源。请填写基础 URL，保存并测试后再添加指标证据工具。",
    "Prometheus": "Prometheus",
    "Prometheus metric evidence": "Prometheus 指标证据",
    "Prometheus-compatible": "Prometheus 兼容",
    "Prometheus-compatible active alerts from Thanos Rule.":
      "来自 Thanos Rule 的 Prometheus 兼容活动告警。",
    "Prometheus-compatible integration": "Prometheus 兼容集成",
    "Prometheus-compatible metric evidence.": "Prometheus 兼容指标证据。",
    "Prometheus-compatible query API": "Prometheus 兼容查询 API",
    "Range metric evidence": "范围指标证据",
    "Receiver after save": "保存后生成接收器",
    "Receiver route": "接收器路由",
    "Reload and test": "重载并测试",
    "Review Workflow": "检查工作流",
    "Review Thanos Rule classification.": "检查 Thanos Rule 分类。",
    "Run Test to verify provider reachability and credentials.":
      "请运行测试以验证提供方可达性和凭据。",
    "Secret reference requires bearer auth.": "只有 Bearer 认证可以使用密钥引用。",
    "Secret reference must be 256 bytes or fewer.": "密钥引用不能超过 256 字节。",
    "Secret reference must not contain whitespace or control characters.":
      "密钥引用不能包含空白符或控制字符。",
    "Silenced/inhibited alerts ignored": "忽略静默或抑制的告警",
    "Source disabled": "告警源已停用",
    "Source profile": "告警源配置",
    "Source ready for workflows.": "告警源已可用于工作流。",
    "Source will be saved as draft.": "告警源将保存为草稿。",
    "Supplemental active alerts": "补充活动告警",
    "Thanos Query": "Thanos Query",
    "Thanos Rule": "Thanos Rule",
    "Thanos Rule active alerts": "Thanos Rule 活动告警",
    "Thanos Rule alerts API": "Thanos Rule 告警 API",
    "Thanos Rule integration": "Thanos Rule 集成",
    "Webhook intake": "Webhook 接入",
    "Webhook proof": "Webhook 交付证明",
    "Webhook proof needed": "需要 Webhook 交付证明",
    "Webhook setup ready": "可以配置 Webhook",
    "Workflow": "工作流",
    "Workflow binding": "工作流绑定",
  };
  if (exact[value] !== undefined) {
    return exact[value]!;
  }
  let match = value.match(/^Authorization: Bearer token resolved from (.+)$/);
  if (match) {
    return `Authorization：Bearer 令牌从 ${match[1]} 解析`;
  }
  match = value.match(/^Enable this (.+) source before using it for (.+)\.$/);
  if (match) {
    return `将此 ${localizeAlertSourceText(match[1]!, locale)} 告警源用于${localizeAlertSourceText(match[2]!, locale)}前，请先启用。`;
  }
  match = value.match(/^Run Test for this (.+) source before relying on AI diagnosis workflow setup\.$/);
  if (match) {
    return `依赖 AI 诊断工作流配置前，请先测试此 ${localizeAlertSourceText(match[1]!, locale)} 告警源。`;
  }
  match = value.match(/^Last connection test ended with (.+)\. Resolve the provider or credential issue before continuing setup\.$/);
  if (match) {
    return `最近一次连接测试结果为 ${localizeAlertSourceText(match[1]!, locale)}。继续配置前，请解决提供方或凭据问题。`;
  }
  match = value.match(/^Label line (\d+) must use key=value\.$/);
  if (match) {
    return `标签第 ${match[1]} 行必须使用 key=value 格式。`;
  }
  match = value.match(/^Label line (\d+) has an empty key\.$/);
  if (match) {
    return `标签第 ${match[1]} 行的键为空。`;
  }
  match = value.match(/^Label line (\d+) exceeds the allowed length\.$/);
  if (match) {
    return `标签第 ${match[1]} 行超过允许长度。`;
  }
  match = value.match(/^Label key "(.+)" is duplicated\.$/);
  if (match) {
    return `标签键“${match[1]}”重复。`;
  }
  match = value.match(/^(.+) source is ready for instant and range metric evidence tools\.$/);
  if (match) {
    return `${localizeAlertSourceText(match[1]!, locale)} 告警源已可用于即时和范围指标证据工具。`;
  }
  match = value.match(/^Create an active_alerts evidence template bound to this (.+) source\.$/);
  if (match) {
    return `请创建绑定到此 ${localizeAlertSourceText(match[1]!, locale)} 告警源的 active_alerts 证据模板。`;
  }
  match = value.match(/^Create or update an automatic diagnosis workflow that uses this (.+) webhook source\.$/);
  if (match) {
    return `请创建或更新使用此 ${localizeAlertSourceText(match[1]!, locale)} Webhook 告警源的自动诊断工作流。`;
  }
  match = value.match(/^Enable this (.+) source, save it, then run Test before configuring AI diagnosis setup\.$/);
  if (match) {
    return `请启用并保存此 ${localizeAlertSourceText(match[1]!, locale)} 告警源，然后在配置 AI 诊断前运行测试。`;
  }
  match = value.match(/^Ready for (.+) webhook intake and active-alert evidence in automatic AI diagnosis workflows\.$/);
  if (match) {
    return `已可在自动 AI 诊断工作流中使用 ${localizeAlertSourceText(match[1]!, locale)} Webhook 接入和活动告警证据。`;
  }
  match = value.match(/^Copy the receiver YAML into Alertmanager receivers as (.+)\.$/);
  if (match) {
    return `请将接收器 YAML 复制到 Alertmanager receivers，并命名为 ${match[1]}。`;
  }
  match = value.match(/^Add a scoped Alertmanager route that selects alerts OpenClarion should diagnose and sends them to (.+)\. Use continue: true only when existing downstream receivers should also run\.$/);
  if (match) {
    return `请添加限定范围的 Alertmanager 路由，选择 OpenClarion 应诊断的告警并发送到 ${match[1]}。仅当现有下游接收器也应继续运行时才使用 continue: true。`;
  }
  return value;
}
