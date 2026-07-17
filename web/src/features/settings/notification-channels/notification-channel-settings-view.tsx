"use client";

import {
  BranchesOutlined,
  CheckCircleOutlined,
  DownOutlined,
  EditOutlined,
  MessageOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  SendOutlined,
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  Descriptions,
  Dropdown,
  Empty,
  Form,
  Input,
  Row,
  Segmented,
  Space,
  Statistic,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import type {
  CheckboxOptionType,
  DescriptionsProps,
  MenuProps,
  TableColumnsType,
} from "antd";
import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

import { formatDateTime } from "../format";
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
  refreshNotificationChannelProfiles,
  submitNotificationChannelProfile,
  testNotificationChannel,
} from "./client-api";
import {
  channelToFormState,
  emptyNotificationChannelForm,
  formStateToWriteRequest,
  mergeNotificationChannelTestProofBundle,
  notificationChannelAIRoomReadiness,
  notificationChannelAIProofInventoryReadiness,
  notificationChannelAIProofReadiness,
  notificationChannelAIProofRunnableChannelIDs,
  notificationChannelAIProofSummaries,
  notificationChannelCredentialReadiness,
  notificationChannelEnterpriseWeChatRolloutReadiness,
  notificationChannelLaunchInitialForm,
  notificationChannelDeliveryReadiness,
  notificationChannelMissingAIProofContentKinds,
  notificationChannelSecretResolverEnvKey,
  notificationChannelTestProofBundleFromResults,
  notificationChannelTestContentKindLabel,
  notificationChannelPrimaryTestContentKind,
  notificationChannelTestSample,
  type NotificationChannelAIProofInventoryReadiness,
  type NotificationChannelAIRoomReadiness,
  type NotificationChannelAIProofSummary,
  type NotificationChannelAIProofReadiness,
  type NotificationChannelCredentialReadiness,
  type NotificationChannelDeliveryReadiness,
  type NotificationChannelEnterpriseWeChatRolloutReadiness,
  type NotificationChannelLaunchIntent,
  type NotificationChannelTestProofBundle,
  type NotificationChannelWorkflowReturn,
} from "./format";
import type {
  NotificationChannelFormState,
  NotificationChannelProfile,
  NotificationChannelProfileListResponse,
  NotificationChannelTestContentKind,
  NotificationChannelTestResult,
  NotificationChannelProfileWriteRequest,
  NotificationDeliveryScope,
} from "./types";

type NotificationChannelSettingsManagerProps = {
  launchEditChannelID?: number | null;
  launchIntent?: NotificationChannelLaunchIntent | null;
  result: ApiResult<NotificationChannelProfileListResponse>;
  workflowReturn?: NotificationChannelWorkflowReturn | null;
};

const channelKindOptions: Array<{
  value: NotificationChannelFormState["kind"];
  label: string;
}> = [
  { value: "wecom", label: "WeCom" },
  { value: "dingtalk", label: "DingTalk" },
  { value: "feishu", label: "Feishu" },
  { value: "slack", label: "Slack" },
  { value: "email", label: "Email" },
  { value: "webhook", label: "Webhook" },
];

const notificationChannelProfilesQueryKey = [
  "settings",
  "notification-channels",
] as const;

type SaveChannelVariables = {
  body: NotificationChannelProfileWriteRequest;
  channelID: number | null;
};

type NotificationChannelTranslator = ReturnType<typeof useTranslations<"NotificationChannelSettings">>;

function localizedDeliveryScopeOptions(
  t: NotificationChannelTranslator,
): CheckboxOptionType<NotificationDeliveryScope>[] {
  return [
    { label: t("reports"), value: "report" },
    { label: t("diagnosisConsultation"), value: "diagnosis_consultation" },
    { label: t("diagnosisClose"), value: "diagnosis_close" },
  ];
}

const notificationChannelBaseAuthorizationChecks: CurrentRBACAuthorizationCheck[] =
  [
    { key: "notificationChannelRead", permission: "notification_channel.read" },
    {
      key: "notificationChannelManage",
      permission: "notification_channel.manage",
    },
  ];

type AIProofRunResult =
  | {
      ok: true;
      testedContentKinds: number;
    }
  | {
      notice: SettingsNotice;
      ok: false;
    };

function initialNotificationChannelNotice(
  launchIntent: NotificationChannelLaunchIntent | null,
  launchEditChannelID: number | null,
  initialEditChannel: NotificationChannelProfile | null,
): string | null {
  if (initialEditChannel !== null) {
    return `Loaded notification channel #${initialEditChannel.id} for delivery review.`;
  }
  if (launchEditChannelID !== null) {
    return `Notification channel #${launchEditChannelID} was not found. Select an existing channel or create a replacement.`;
  }
  return launchIntent?.message ?? null;
}

export function NotificationChannelSettingsManager({
  launchEditChannelID = null,
  launchIntent = null,
  result,
  workflowReturn = null,
}: NotificationChannelSettingsManagerProps) {
  const locale = useLocale();
  const t = useTranslations("NotificationChannelSettings");
  const common = useTranslations("Common");
  const [form] = Form.useForm<NotificationChannelFormState>();
  const clientReady = useClientReady();
  const initialEditChannel = useMemo(
    () =>
      launchEditChannelID === null || !result.ok
        ? null
        : (result.data.items.find(
            (channel) => channel.id === launchEditChannelID,
          ) ?? null),
    [launchEditChannelID, result],
  );
  const [editingID, setEditingID] = useState<number | null>(
    initialEditChannel?.id ?? null,
  );
  const [testingID, setTestingID] = useState<number | null>(null);
  const [testResults, setTestResults] = useState<
    Record<number, NotificationChannelTestProofBundle>
  >({});
  const [launchNotice, setLaunchNotice] = useState<string | null>(
    initialNotificationChannelNotice(
      launchIntent,
      launchEditChannelID,
      initialEditChannel,
    ),
  );
  const deliveryScopeOptions = useMemo(() => localizedDeliveryScopeOptions(t), [t]);
  const {
    errorStatus,
    items: channels,
    notice,
    query,
    refresh,
    setNotice,
  } = useSettingsList({
    initialResult: result,
    queryKey: notificationChannelProfilesQueryKey,
    queryFn: refreshNotificationChannelProfiles,
    refreshMessage: t("refreshed"),
    selectItems: (response) => response.items,
  });
  const saveChannel = useSettingsMutation<
    SaveChannelVariables,
    NotificationChannelProfile
  >({
    invalidateQueryKey: notificationChannelProfilesQueryKey,
    mutationFn: ({ channelID, body }) =>
      submitNotificationChannelProfile(channelID, body),
  });
  const authorizationChecks = useMemo(
    () => [
      ...notificationChannelBaseAuthorizationChecks,
      ...channels.flatMap((channel) => [
        {
          key: notificationChannelManageKey(channel.id),
          permission: "notification_channel.manage" as const,
          scopeKey: String(channel.id),
          scopeKind: "notification_channel" as const,
        },
        {
          key: notificationChannelTestKey(channel.id),
          permission: "notification_channel.test" as const,
          scopeKey: String(channel.id),
          scopeKind: "notification_channel" as const,
        },
      ]),
    ],
    [channels],
  );
  const currentAuthorization = useCurrentRBACAuthorizations(
    authorizationChecks,
    clientReady,
  );
  const busy =
    !clientReady ||
    currentAuthorization.isChecking ||
    query.isFetching ||
    saveChannel.isPending;
  const canReadChannels = currentAuthorization.can("notificationChannelRead");
  const canCreateChannel = currentAuthorization.can("notificationChannelManage");
  const canSaveCurrentChannel =
    editingID === null
      ? canCreateChannel
      : currentAuthorization.can(notificationChannelManageKey(editingID));
  const formPermissionNotice = settingsManagePermissionNotice({
    canManage: canSaveCurrentChannel,
    isChecking: !clientReady || currentAuthorization.isChecking,
    message: common("formReadOnly", {
      resource:
        editingID === null
          ? t("creationResource")
          : t("channelResource", { id: editingID }),
    }),
  });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadChannels,
    errorStatus,
    isChecking: !clientReady || currentAuthorization.isChecking,
    message: common("readAccessLimited", {
      resource: t("channelsResource"),
    }),
  });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;
  const initialFormValues = useMemo(
    () =>
      initialEditChannel
        ? channelToFormState(initialEditChannel)
        : notificationChannelLaunchInitialForm(launchIntent),
    [initialEditChannel, launchIntent],
  );

  const summary = useMemo(() => {
    const enabled = channels.filter((channel) => channel.enabled).length;
    const report = channels.filter((channel) =>
      channel.delivery_scopes.includes("report"),
    ).length;
    const diagnosisConsultation = channels.filter((channel) =>
      channel.delivery_scopes.includes("diagnosis_consultation"),
    ).length;
    const diagnosisClose = channels.filter((channel) =>
      channel.delivery_scopes.includes("diagnosis_close"),
    ).length;
    return { enabled, report, diagnosisConsultation, diagnosisClose };
  }, [channels]);
  const aiProofInventory = useMemo(
    () => notificationChannelAIProofInventoryReadiness(channels, testResults),
    [channels, testResults],
  );
  const enterpriseWeChatRollout = useMemo(
    () =>
      notificationChannelEnterpriseWeChatRolloutReadiness(
        channels,
        testResults,
      ),
    [channels, testResults],
  );
  const aiProofSummaries = useMemo(
    () => notificationChannelAIProofSummaries(channels, testResults),
    [channels, testResults],
  );
  const aiProofRunnableChannelIDs = useMemo(
    () => notificationChannelAIProofRunnableChannelIDs(channels, testResults),
    [channels, testResults],
  );
  const aiProofRunnableChannels = useMemo(
    () =>
      aiProofRunnableChannelIDs
        .map((channelID) =>
          channels.find((channel) => channel.id === channelID),
        )
        .filter(
          (channel): channel is NotificationChannelProfile =>
            channel !== undefined,
        ),
    [aiProofRunnableChannelIDs, channels],
  );
  const authorizedAIProofRunnableChannels = useMemo(
    () =>
      aiProofRunnableChannels.filter((channel) =>
        currentAuthorization.can(notificationChannelTestKey(channel.id)),
      ),
    [aiProofRunnableChannels, currentAuthorization],
  );

  async function handleRefresh() {
    await refresh();
  }

  async function handleSubmit(values: NotificationChannelFormState) {
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeNotificationChannelText(parsed.message, t) });
      return;
    }

    try {
      await saveChannel.mutateAsync({
        channelID: editingID,
        body: parsed.value,
      });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      return;
    }

    form.setFieldsValue(emptyNotificationChannelForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({ kind: "info", message: t("saved") });
  }

  function recordTestResult(
    channelID: number,
    result: NotificationChannelTestResult,
  ) {
    setTestResults((current) => ({
      ...current,
      [channelID]: mergeNotificationChannelTestProofBundle(
        current[channelID] ??
          notificationChannelTestProofBundleFromResults(
            channels.find((channel) => channel.id === channelID)
              ?.latest_test_results,
          ),
        result,
      ),
    }));
  }

  async function handleTest(
    channel: NotificationChannelProfile,
    contentKind?: NotificationChannelTestContentKind,
  ) {
    setTestingID(channel.id);
    try {
      const tested = await testNotificationChannel(channel.id, contentKind);
      if (!tested.ok) {
        setNotice({ kind: "error", message: tested.error.message });
        return;
      }

      recordTestResult(channel.id, tested.data);
      setNotice({
        kind: noticeKindForChannelTestStatus(tested.data.status),
        message: tested.data.message,
      });
    } finally {
      setTestingID(null);
    }
  }

  async function handleTestAIProof(channel: NotificationChannelProfile) {
    const proof = notificationChannelTestProofForChannel(channel, testResults);
    const missingContentKinds = notificationChannelMissingAIProofContentKinds(
      channel,
      proof,
    );
    if (missingContentKinds.length === 0) {
      setNotice({
        kind: "info",
        message: t("proofCurrent"),
      });
      return;
    }

    setTestingID(channel.id);
    try {
      const result = await runAIProofTestsForChannel(
        channel,
        missingContentKinds,
      );
      if (!result.ok) {
        setNotice(result.notice);
        return;
      }
    } finally {
      setTestingID(null);
    }

    setNotice({ kind: "info", message: t("proofCompleted") });
  }

  async function handleTestAllAIProof() {
    if (authorizedAIProofRunnableChannels.length === 0) {
      setNotice({
        kind: "info",
        message: t("noProofPending"),
      });
      return;
    }

    let testedChannels = 0;
    let testedContentKinds = 0;
    try {
      for (const channel of authorizedAIProofRunnableChannels) {
        const proof = notificationChannelTestProofForChannel(
          channel,
          testResults,
        );
        const missingContentKinds =
          notificationChannelMissingAIProofContentKinds(channel, proof);
        if (missingContentKinds.length === 0) {
          continue;
        }
        setTestingID(channel.id);
        const result = await runAIProofTestsForChannel(
          channel,
          missingContentKinds,
        );
        if (!result.ok) {
          setNotice(result.notice);
          return;
        }
        testedChannels += 1;
        testedContentKinds += result.testedContentKinds;
      }
    } finally {
      setTestingID(null);
    }

    setNotice({
      kind: "info",
      message: t("proofBatchCompleted", {
        channels: testedChannels,
        samples: testedContentKinds,
      }),
    });
  }

  async function runAIProofTestsForChannel(
    channel: NotificationChannelProfile,
    missingContentKinds: NotificationChannelTestContentKind[],
  ): Promise<AIProofRunResult> {
    let testedContentKinds = 0;
    for (const contentKind of missingContentKinds) {
      const tested = await testNotificationChannel(channel.id, contentKind);
      if (!tested.ok) {
        return {
          notice: { kind: "error", message: tested.error.message },
          ok: false,
        };
      }
      recordTestResult(channel.id, tested.data);
      testedContentKinds += 1;
      if (tested.data.status !== "success") {
        return {
          notice: {
            kind: noticeKindForChannelTestStatus(tested.data.status),
            message: tested.data.message,
          },
          ok: false,
        };
      }
    }
    return { ok: true, testedContentKinds };
  }

  function editChannel(channel: NotificationChannelProfile) {
    setEditingID(channel.id);
    form.setFieldsValue(channelToFormState(channel));
    setLaunchNotice(null);
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyNotificationChannelForm());
    setLaunchNotice(null);
    setNotice(null);
  }

  return (
    <div className="stack">
      <Row aria-label={t("metricsLabel")} gutter={[12, 12]}>
        <MetricCard label={t("channels")} value={channels.length} />
        <MetricCard label={t("enabled")} value={summary.enabled} />
        <MetricCard label={t("reportScope")} value={summary.report} />
        <MetricCard
          label={t("consultationScope")}
          value={summary.diagnosisConsultation}
        />
        <MetricCard label={t("closeScope")} value={summary.diagnosisClose} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} t={t} /> : null}
      {launchNotice ? (
        <Alert
          aria-label={t("launchPreset")}
          description={localizeNotificationChannelText(launchNotice, t)}
          message={t("actionLoaded")}
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      <NotificationChannelEnterpriseWeChatRolloutPanel
        onRunMissingProof={handleTestAllAIProof}
        readiness={enterpriseWeChatRollout}
        runDisabled={
          busy ||
          testingID !== null ||
          authorizedAIProofRunnableChannels.length === 0
        }
        runLoading={testingID !== null}
        runnableChannelCount={authorizedAIProofRunnableChannels.length}
        t={t}
        workflowReturn={workflowReturn}
      />
      <NotificationChannelAIProofInventoryPanel
        onRunMissingProof={handleTestAllAIProof}
        readiness={aiProofInventory}
        runDisabled={
          busy ||
          testingID !== null ||
          authorizedAIProofRunnableChannels.length === 0
        }
        runLoading={testingID !== null}
        runnableChannelCount={authorizedAIProofRunnableChannels.length}
        locale={locale}
        summaries={aiProofSummaries}
        t={t}
      />

      <Row align="top" className="settings-console-grid" gutter={[16, 16]}>
        <Col lg={8} md={24} xs={24}>
          <Card
            extra={
              editingID === null ? null : (
                <Button
                  disabled={busy || !canCreateChannel}
                  icon={<PlusOutlined />}
                  onClick={resetForm}
                  type="default"
                >
                  {t("new")}
                </Button>
              )
            }
            title={
              editingID === null ? t("newChannel") : t("editChannel", { id: editingID })
            }
          >
            {formPermissionNotice ? (
              <ReadOnlyModeAlert notice={formPermissionNotice} />
            ) : null}
            <Form<NotificationChannelFormState>
              disabled={busy || !canSaveCurrentChannel}
              form={form}
              initialValues={initialFormValues}
              layout="vertical"
              onFinish={handleSubmit}
            >
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

              <Form.Item
                label={t("kind")}
                name="kind"
                rules={[{ required: true, message: t("kindRequired") }]}
              >
                <Segmented block options={channelKindOptions} />
              </Form.Item>

              <Form.Item
                label={t("secretReference")}
                name="secretRef"
                extra={
                  <Typography.Text type="secondary">
                    {t("secretHint", { env: notificationChannelSecretResolverEnvKey })}
                  </Typography.Text>
                }
                rules={[
                  { required: true, message: t("secretRequired") },
                ]}
              >
                <Input
                  autoComplete="off"
                  placeholder="secret/example/ops-webhook"
                />
              </Form.Item>

              <Form.Item
                label={t("deliveryScopes")}
                name="deliveryScopes"
                rules={[
                  {
                    required: true,
                    message: t("scopeRequired"),
                  },
                ]}
              >
                <Checkbox.Group options={deliveryScopeOptions} />
              </Form.Item>

              <Form.Item label={t("labels")} name="labelsText">
                <Input.TextArea
                  autoSize={{ minRows: 3, maxRows: 6 }}
                  placeholder="team=ops"
                />
              </Form.Item>

              <Form.Item label={t("enabled")} name="enabled" valuePropName="checked">
                <Switch />
              </Form.Item>

              <Form.Item noStyle shouldUpdate>
                {(channelForm) => (
                  <NotificationChannelDeliveryPreview
                    t={t}
                    values={
                      channelForm.getFieldsValue(
                        true,
                      ) as NotificationChannelFormState
                    }
                  />
                )}
              </Form.Item>

              <Space wrap>
                <Button
                  disabled={busy || !canSaveCurrentChannel}
                  htmlType="submit"
                  icon={<SaveOutlined />}
                  loading={busy}
                  type="primary"
                >
                  {t("saveChannel")}
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
                disabled={busy || !canReadChannels}
                icon={<ReloadOutlined />}
                loading={busy}
                onClick={handleRefresh}
                type="default"
              >
                {t("refresh")}
              </Button>
            }
            title={t("configuredChannels")}
          >
            <NotificationChannelTable
              busy={busy}
              canRead={canReadChannels}
              canManageChannel={(channelID) =>
                currentAuthorization.can(notificationChannelManageKey(channelID))
              }
              canTestChannel={(channelID) =>
                currentAuthorization.can(notificationChannelTestKey(channelID))
              }
              channels={channels}
              onEdit={editChannel}
              onTest={handleTest}
              onTestAIProof={handleTestAIProof}
              testResults={testResults}
              testingID={testingID}
              locale={locale}
              t={t}
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

function Notice({ notice, t }: { notice: SettingsNotice; t: NotificationChannelTranslator }) {
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
            ? t("testReview")
            : t("settings")
      }
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={type}
    />
  );
}

function NotificationChannelEnterpriseWeChatRolloutPanel({
  onRunMissingProof,
  readiness,
  runDisabled,
  runLoading,
  runnableChannelCount,
  t,
  workflowReturn,
}: {
  onRunMissingProof: () => void;
  readiness: NotificationChannelEnterpriseWeChatRolloutReadiness;
  runDisabled: boolean;
  runLoading: boolean;
  runnableChannelCount: number;
  t: NotificationChannelTranslator;
  workflowReturn: NotificationChannelWorkflowReturn | null;
}) {
  const canRunProof =
    readiness.missingContentKinds.length > 0 && runnableChannelCount > 0;
  const canReturnToWorkflow =
    workflowReturn !== null && readiness.status === "ready";
  return (
    <Alert
      aria-label={t("wecomRolloutLabel")}
      action={
        canRunProof || canReturnToWorkflow ? (
          <Space wrap>
            {canRunProof ? (
              <Button
                disabled={runDisabled}
                icon={<CheckCircleOutlined />}
                loading={runLoading}
                onClick={onRunMissingProof}
                size="small"
              >
                {t("runMissingProof")}
              </Button>
            ) : null}
            {canReturnToWorkflow ? (
              <Button
                href={workflowReturn.href}
                icon={<BranchesOutlined />}
                size="small"
                type="primary"
              >
                {localizeNotificationChannelText(workflowReturn.label, t)}
              </Button>
            ) : null}
          </Space>
        ) : null
      }
      description={
        <Space direction="vertical" size={8}>
          <Typography.Text>{localizeNotificationChannelText(readiness.detail, t)}</Typography.Text>
          {workflowReturn !== null ? (
            <Typography.Text type="secondary">
              {canReturnToWorkflow
                ? localizeNotificationChannelText(workflowReturn.detail, t)
                : t("finishWecomSetup")}
            </Typography.Text>
          ) : null}
          <Descriptions
            column={1}
            items={[
              {
                children: readiness.candidateChannelIDs.length,
                key: "candidates",
                label: t("candidates"),
              },
              {
                children: readiness.readyChannelIDs.length,
                key: "ready",
                label: t("ready"),
              },
              {
                children: readiness.reviewChannelIDs.length,
                key: "review",
                label: t("review"),
              },
              {
                children: readiness.blockedChannelIDs.length,
                key: "blocked",
                label: t("blocked"),
              },
              {
                children:
                  readiness.missingScopes.length === 0
                    ? t("none")
                    : readiness.missingScopes.map((scope) => localizeNotificationChannelText(scope, t)).join(", "),
                key: "missingScopes",
                label: t("missingScopes"),
              },
              {
                children:
                  readiness.missingContentKinds.length === 0
                    ? t("none")
                    : readiness.missingContentKinds
                        .map((kind) => localizeNotificationChannelText(notificationChannelTestContentKindLabel(kind), t))
                        .join(", "),
                key: "missingProof",
                label: t("missingProof"),
              },
            ]}
            size="small"
          />
        </Space>
      }
      message={localizeNotificationChannelText(readiness.label, t)}
      role={readiness.status === "blocked" ? "alert" : "status"}
      showIcon
      type={enterpriseWeChatRolloutAlertType(readiness.status)}
    />
  );
}

function enterpriseWeChatRolloutAlertType(
  status: NotificationChannelEnterpriseWeChatRolloutReadiness["status"],
) {
  switch (status) {
    case "ready":
      return "success";
    case "review":
      return "warning";
    case "blocked":
      return "error";
    case "pending":
      return "info";
  }
}

function NotificationChannelAIProofInventoryPanel({
  locale,
  onRunMissingProof,
  readiness,
  runDisabled,
  runLoading,
  runnableChannelCount,
  summaries,
  t,
}: {
  locale: string;
  onRunMissingProof: () => void;
  readiness: NotificationChannelAIProofInventoryReadiness;
  runDisabled: boolean;
  runLoading: boolean;
  runnableChannelCount: number;
  summaries: NotificationChannelAIProofSummary[];
  t: NotificationChannelTranslator;
}) {
  return (
    <Alert
      aria-label={t("proofReadinessLabel")}
      action={
        runnableChannelCount === 0 ? null : (
          <Button
            disabled={runDisabled}
            icon={<CheckCircleOutlined />}
            loading={runLoading}
            onClick={onRunMissingProof}
            size="small"
          >
            {t("runMissingProof")}
          </Button>
        )
      }
      description={
        <Space direction="vertical" size={4}>
          <Typography.Text>{localizeNotificationChannelText(readiness.detail, t)}</Typography.Text>
          {readiness.candidateChannelIDs.length > 0 ? (
            <Typography.Text type="secondary">
              {t("proofSummary", {
                blocked: readiness.blockedChannelIDs.length,
                candidates: readiness.candidateChannelIDs.length,
                ready: readiness.readyChannelIDs.length,
                review: readiness.reviewChannelIDs.length,
              })}
            </Typography.Text>
          ) : null}
          {readiness.missingContentKinds.length > 0 ? (
            <Typography.Text type="secondary">
              {t("missingProof")}: {" "}
              {readiness.missingContentKinds
                .map((kind) => localizeNotificationChannelText(notificationChannelTestContentKindLabel(kind), t))
                .join(", ")}
              .
            </Typography.Text>
          ) : null}
          {summaries.length > 0 ? (
            <Space direction="vertical" size={2}>
              {summaries.map((summary) => (
                <AIProofSummaryRow key={summary.channelID} locale={locale} summary={summary} t={t} />
              ))}
            </Space>
          ) : null}
        </Space>
      }
      message={localizeNotificationChannelText(readiness.label, t)}
      role={readiness.status === "blocked" ? "alert" : "status"}
      showIcon
      type={aiProofInventoryAlertType(readiness.status)}
    />
  );
}

function AIProofSummaryRow({
  locale,
  summary,
  t,
}: {
  locale: string;
  summary: NotificationChannelAIProofSummary;
  t: NotificationChannelTranslator;
}) {
  return (
    <Space size={[4, 4]} wrap>
      <Typography.Text strong>
        #{summary.channelID} {summary.channelName}
      </Typography.Text>
      <Tag color={channelAIProofReadinessColor(summary.status)}>
        {channelAIProofReadinessLabel(summary.status, t)}
      </Tag>
      {summary.contents.map((content) => (
        <Tag
          color={aiProofContentSummaryColor(content.status)}
          key={content.contentKind}
        >
          {localizeNotificationChannelText(content.label, t)}: {aiProofContentSummaryLabel(content.status, t)}
          {content.checkedAt ? ` ${formatDateTime(content.checkedAt, locale)}` : ""}
        </Tag>
      ))}
    </Space>
  );
}

function aiProofContentSummaryColor(
  status: NotificationChannelAIProofSummary["contents"][number]["status"],
): string {
  switch (status) {
    case "current":
      return "green";
    case "stale":
    case "blocked":
      return "gold";
    case "missing":
    case "unsupported":
      return "default";
    case "failed":
    case "invalid":
      return "red";
  }
}

function aiProofContentSummaryLabel(
  status: NotificationChannelAIProofSummary["contents"][number]["status"],
  t: NotificationChannelTranslator,
): string {
  return t(`proofContentStatus.${status}`);
}

function aiProofInventoryAlertType(
  status: NotificationChannelAIProofInventoryReadiness["status"],
) {
  switch (status) {
    case "ready":
      return "success";
    case "review":
      return "warning";
    case "blocked":
      return "error";
    case "pending":
      return "info";
  }
}

function NotificationChannelDeliveryPreview({
  t,
  values,
}: {
  t: NotificationChannelTranslator;
  values: NotificationChannelFormState;
}) {
  const normalized = normalizeNotificationChannelFormValues(values);
  const readiness = notificationChannelDeliveryReadiness(normalized);
  const credentials = notificationChannelCredentialReadiness(normalized);
  const testSample = notificationChannelTestSample(normalized);
  const items: DescriptionsProps["items"] = [
    {
      children: localizeNotificationChannelText(credentials.kindLabel, t),
      key: "kind",
      label: t("adapter"),
    },
    {
      children: normalized.enabled ? t("enabled") : t("draft"),
      key: "state",
      label: t("state"),
    },
    {
      children: credentials.secretConfigured
        ? credentials.expectedCredential
        : t("missing"),
      key: "credential",
      label: t("credential"),
    },
    {
      children: credentials.resolverEnvKey,
      key: "resolver",
      label: t("serverResolver"),
    },
    {
      children: credentials.secretRefExample,
      key: "secretRefExample",
      label: t("secretExample"),
    },
    {
      children: readiness.hasReportScope ? t("ready") : t("missing"),
      key: "report",
      label: t("finalReports"),
    },
    {
      children: readiness.hasDiagnosisConsultationScope ? t("ready") : t("missing"),
      key: "diagnosisConsultation",
      label: t("diagnosisUpdates"),
    },
    {
      children: readiness.hasDiagnosisCloseScope ? t("ready") : t("missing"),
      key: "diagnosisClose",
      label: t("autoRoomClose"),
    },
    {
      children: localizeNotificationChannelText(testSample.label, t),
      key: "testSample",
      label: t("testSample"),
    },
  ];

  return (
    <div
      aria-label={t("deliveryReadiness")}
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={channelDeliveryReadinessColor(readiness.status)}>
            {channelDeliveryReadinessLabel(readiness.status, t)}
          </Tag>
          <Tag color={channelCredentialReadinessColor(credentials.status)}>
            {localizeNotificationChannelText(credentials.kindLabel, t)}
          </Tag>
          <Tag color={readiness.hasReportScope ? "blue" : "default"}>
            {t("reports")}
          </Tag>
          <Tag
            color={readiness.hasDiagnosisConsultationScope ? "cyan" : "default"}
          >
            {t("diagnosisConsultation")}
          </Tag>
          <Tag color={readiness.hasDiagnosisCloseScope ? "purple" : "default"}>
            {t("diagnosisClose")}
          </Tag>
        </Space>
        <Typography.Text strong>{localizeNotificationChannelText(readiness.label, t)}</Typography.Text>
        <Typography.Text type="secondary">{localizeNotificationChannelText(readiness.detail, t)}</Typography.Text>
        {readiness.missingScopes.length > 0 ? (
          <Typography.Text type="secondary">
            {t("missingScopes")}: {readiness.missingScopes.map((scope) => localizeNotificationChannelText(scope, t)).join(", ")}
          </Typography.Text>
        ) : null}
        <Typography.Text type="secondary">{localizeNotificationChannelText(credentials.detail, t)}</Typography.Text>
        <Typography.Text type="secondary">{localizeNotificationChannelText(testSample.detail, t)}</Typography.Text>
        <Descriptions column={1} items={items} size="small" />
      </Space>
    </div>
  );
}

function normalizeNotificationChannelFormValues(
  values: Partial<NotificationChannelFormState>,
): NotificationChannelFormState {
  const defaults = emptyNotificationChannelForm();
  return {
    name: values.name ?? defaults.name,
    kind: values.kind ?? defaults.kind,
    secretRef: values.secretRef ?? defaults.secretRef,
    deliveryScopes: values.deliveryScopes ?? defaults.deliveryScopes,
    enabled: values.enabled ?? defaults.enabled,
    labelsText: values.labelsText ?? defaults.labelsText,
  };
}

function notificationChannelManageKey(channelID: number): string {
  return `notificationChannelManage:${channelID}`;
}

function notificationChannelTestKey(channelID: number): string {
  return `notificationChannelTest:${channelID}`;
}

function channelDeliveryReadinessColor(
  status: NotificationChannelDeliveryReadiness["status"],
): string {
  switch (status) {
    case "blocked":
      return "red";
    case "ready":
      return "green";
    case "review":
      return "gold";
    case "pending":
      return "default";
  }
}

function channelDeliveryReadinessLabel(
  status: NotificationChannelDeliveryReadiness["status"],
  t: NotificationChannelTranslator,
): string {
  switch (status) {
    case "blocked":
      return t("blocked");
    case "ready":
      return t("ready");
    case "review":
      return t("review");
    case "pending":
      return t("pending");
  }
}

function channelCredentialReadinessColor(
  status: NotificationChannelCredentialReadiness["status"],
): string {
  switch (status) {
    case "ready":
      return "blue";
    case "blocked":
      return "red";
    case "pending":
      return "default";
  }
}

type NotificationChannelTableProps = {
  busy: boolean;
  canRead: boolean;
  canManageChannel: (channelID: number) => boolean;
  canTestChannel: (channelID: number) => boolean;
  channels: NotificationChannelProfile[];
  onEdit: (channel: NotificationChannelProfile) => void;
  onTest: (
    channel: NotificationChannelProfile,
    contentKind?: NotificationChannelTestContentKind,
  ) => void;
  onTestAIProof: (channel: NotificationChannelProfile) => void;
  testResults: Record<number, NotificationChannelTestProofBundle>;
  testingID: number | null;
  locale: string;
  t: NotificationChannelTranslator;
};

function NotificationChannelTable({
  busy,
  canRead,
  canManageChannel,
  canTestChannel,
  channels,
  onEdit,
  onTest,
  onTestAIProof,
  testResults,
  testingID,
  locale,
  t,
}: NotificationChannelTableProps) {
  const common = useTranslations("Common");
  const columns: TableColumnsType<NotificationChannelProfile> = [
    {
      key: "name",
      title: t("name"),
      render: (_, channel) => (
        <Space direction="vertical" size={2}>
          <Typography.Text strong>{channel.name}</Typography.Text>
          <Typography.Text type="secondary">
            {channel.secret_ref}
          </Typography.Text>
        </Space>
      ),
    },
    {
      dataIndex: "kind",
      key: "kind",
      title: t("kind"),
      render: (kind: NotificationChannelProfile["kind"]) => <Tag>{kind}</Tag>,
    },
    {
      dataIndex: "delivery_scopes",
      key: "delivery_scopes",
      title: t("scopes"),
      render: (scopes: NotificationDeliveryScope[]) => (
        <Space wrap>
          {scopes.map((scope) => (
            <Tag color={scope === "report" ? "blue" : "purple"} key={scope}>
              {localizeNotificationChannelText(scope, t)}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: t("state"),
      render: (enabled: boolean) => (
        <Tag color={enabled ? "green" : "default"}>
          {enabled ? t("enabled") : t("disabled")}
        </Tag>
      ),
    },
    {
      key: "ai_diagnosis",
      title: t("aiDiagnosis"),
      render: (_, channel) => (
        <ChannelAIRoomReadiness
          channel={channel}
          testProof={notificationChannelTestProofForChannel(
            channel,
            testResults,
          )}
          t={t}
        />
      ),
    },
    {
      key: "last_test",
      title: t("testProof"),
      render: (_, channel) => (
        <ChannelTestProof locale={locale} t={t}
          proof={notificationChannelTestProofForChannel(channel, testResults)}
        />
      ),
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: t("updated"),
      render: (value: string) => formatDateTime(value, locale),
    },
    {
      key: "actions",
      render: (_, channel) => {
        const proof = notificationChannelTestProofForChannel(
          channel,
          testResults,
        );
        const missingAIProofContentKinds =
          notificationChannelMissingAIProofContentKinds(channel, proof);
        const canManage = canManageChannel(channel.id);
        const canTest = canTestChannel(channel.id);
        const rowBusy =
          busy || (testingID !== null && testingID !== channel.id);
        return (
          <Space wrap>
            <Button
              disabled={rowBusy || !canTest}
              icon={<SendOutlined />}
              loading={testingID === channel.id}
              onClick={() =>
                onTest(
                  channel,
                  notificationChannelPrimaryTestContentKind(channel, proof),
                )
              }
              type="link"
            >
              {t("test")}
            </Button>
            {channel.kind === "wecom" &&
            missingAIProofContentKinds.length > 0 ? (
              <Button
                disabled={rowBusy || !canTest}
                icon={<CheckCircleOutlined />}
                loading={testingID === channel.id}
                onClick={() => onTestAIProof(channel)}
                type="link"
              >
                {t("aiProof")}
              </Button>
            ) : null}
            <Dropdown
              disabled={rowBusy || !canTest}
              menu={{
                items: notificationChannelTestMenuItems(channel, t),
                onClick: ({ key }) =>
                  onTest(channel, key as NotificationChannelTestContentKind),
              }}
              trigger={["click"]}
            >
              <Button
                aria-label={t("selectTestSample", { name: channel.name })}
                disabled={rowBusy || !canTest}
                icon={<DownOutlined />}
                type="link"
              />
            </Dropdown>
            <Button
              disabled={busy || !canManage || testingID !== null}
              icon={<EditOutlined />}
              onClick={() => onEdit(channel)}
              type="link"
            >
              {t("edit")}
            </Button>
          </Space>
        );
      },
      title: t("actions"),
    },
  ];

  return (
    <Table<NotificationChannelProfile>
      columns={columns}
      dataSource={channels}
      loading={busy}
      locale={{
        emptyText: (
          <Empty
            description={settingsReadPermissionEmptyDescription({
              canRead,
              deniedDescription: common("noReadAccess", {
                resource: t("channelsResource"),
              }),
              emptyDescription: t("noChannels"),
            })}
            image={
              <MessageOutlined aria-hidden className="settings-empty-icon" />
            }
          />
        ),
      }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1220 }}
    />
  );
}

function ChannelAIRoomReadiness({
  channel,
  t,
  testProof,
}: {
  channel: NotificationChannelProfile;
  t: NotificationChannelTranslator;
  testProof?: NotificationChannelTestProofBundle;
}) {
  const readiness = notificationChannelAIRoomReadiness(channel);
  const proofReadiness = notificationChannelAIProofReadiness(
    channel,
    testProof,
  );
  return (
    <Space direction="vertical" size={2}>
      <Tag color={channelAIRoomReadinessColor(readiness.status)}>
        {channelAIRoomReadinessLabel(readiness.status, t)}
      </Tag>
      <Tag color={channelAIProofReadinessColor(proofReadiness.status)}>
        {channelAIProofReadinessLabel(proofReadiness.status, t)}
      </Tag>
      <Typography.Text
        ellipsis={{
          tooltip: localizeNotificationChannelText(readiness.detail, t),
        }}
        type="secondary"
      >
        {localizeNotificationChannelText(readiness.label, t)}
      </Typography.Text>
      <Typography.Text
        ellipsis={{
          tooltip: localizeNotificationChannelText(proofReadiness.detail, t),
        }}
        type="secondary"
      >
        {localizeNotificationChannelText(proofReadiness.label, t)}
      </Typography.Text>
      {readiness.missingScopes.length > 0 ? (
        <Typography.Text type="secondary">
          {t("missing")} {readiness.missingScopes.map((scope) => localizeNotificationChannelText(scope, t)).join(", ")}
        </Typography.Text>
      ) : null}
      {proofReadiness.missingContentKinds.length > 0 ? (
        <Typography.Text type="secondary">
          {t("test")} {" "}
          {proofReadiness.missingContentKinds
            .map((kind) => localizeNotificationChannelText(notificationChannelTestContentKindLabel(kind), t))
            .join(", ")}
        </Typography.Text>
      ) : null}
    </Space>
  );
}

function notificationChannelTestProofForChannel(
  channel: NotificationChannelProfile,
  testResults: Record<number, NotificationChannelTestProofBundle>,
): NotificationChannelTestProofBundle {
  return (
    testResults[channel.id] ??
    notificationChannelTestProofBundleFromResults(channel.latest_test_results)
  );
}

function channelAIRoomReadinessColor(
  status: NotificationChannelAIRoomReadiness["status"],
): string {
  switch (status) {
    case "ready":
      return "green";
    case "review":
      return "gold";
    case "blocked":
      return "red";
  }
}

function channelAIRoomReadinessLabel(
  status: NotificationChannelAIRoomReadiness["status"],
  t: NotificationChannelTranslator,
): string {
  switch (status) {
    case "ready":
      return t("ready");
    case "review":
      return t("review");
    case "blocked":
      return t("blocked");
  }
}

function channelAIProofReadinessColor(
  status: NotificationChannelAIProofReadiness["status"],
): string {
  switch (status) {
    case "ready":
      return "green";
    case "review":
      return "gold";
    case "blocked":
      return "red";
  }
}

function channelAIProofReadinessLabel(
  status: NotificationChannelAIProofReadiness["status"],
  t: NotificationChannelTranslator,
): string {
  switch (status) {
    case "ready":
      return t("proofReady");
    case "review":
      return t("proofReview");
    case "blocked":
      return t("proofBlocked");
  }
}

function notificationChannelTestMenuItems(
  channel: NotificationChannelProfile,
  t: NotificationChannelTranslator,
): MenuProps["items"] {
  const items: Array<{
    key: NotificationChannelTestContentKind;
    label: string;
  }> = [];
  if (channel.delivery_scopes.includes("diagnosis_consultation")) {
    items.push({ key: "ai_diagnosis_sample", label: t("diagnosisSample") });
  }
  if (channel.delivery_scopes.includes("diagnosis_close")) {
    items.push({
      key: "diagnosis_close_sample",
      label: t("closeSample"),
    });
  }
  items.push({ key: "transport_sample", label: t("transportSample") });
  return items;
}

function ChannelTestProof({
  locale,
  proof,
  t,
}: {
  locale: string;
  proof?: NotificationChannelTestProofBundle;
  t: NotificationChannelTranslator;
}) {
  const results = notificationChannelTestProofResults(proof);
  if (results.length === 0) {
    return <Typography.Text type="secondary">{t("notTested")}</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      {results.map((result) => (
        <Space
          key={result.content_kind ?? result.checked_at}
          size={[4, 4]}
          wrap
        >
          <Tag color={channelTestStatusColor(result.status)}>
            {localizeNotificationChannelText(notificationChannelTestContentKindLabel(result.content_kind), t)}
          </Tag>
          <Tag color={channelTestStatusColor(result.status)}>
            {localizeNotificationChannelText(result.status, t)}
          </Tag>
          {result.content_sha256 ? (
            <Typography.Text type="secondary">
              {result.content_sha256.slice(0, 12)}
            </Typography.Text>
          ) : null}
          <Typography.Text type="secondary">
            {formatDateTime(result.checked_at, locale)}
          </Typography.Text>
          {result.provider_status === "" ? null : (
            <Typography.Text type="secondary">
              {result.provider_status}
            </Typography.Text>
          )}
        </Space>
      ))}
    </Space>
  );
}

function notificationChannelTestProofResults(
  proof: NotificationChannelTestProofBundle | undefined,
): NotificationChannelTestResult[] {
  if (proof === undefined) {
    return [];
  }
  return [
    proof.ai_diagnosis_sample,
    proof.diagnosis_close_sample,
    proof.transport_sample,
  ].filter(
    (result): result is NotificationChannelTestResult => result !== undefined,
  );
}

function channelTestStatusColor(
  status: NotificationChannelTestResult["status"],
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

function noticeKindForChannelTestStatus(
  status: NotificationChannelTestResult["status"],
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

const notificationChannelRuntimeTextKeys = {
    "blocked": "runtimeText.blocked",
    "diagnosis_close": "runtimeText.diagnosisClose",
    "diagnosis_consultation": "runtimeText.diagnosisConsultation",
    "failed": "runtimeText.failed",
    "pending": "runtimeText.pending",
    "ready": "runtimeText.ready",
    "report": "runtimeText.report",
    "review": "runtimeText.review",
    "success": "runtimeText.success",
    "unsupported": "runtimeText.unsupported",
    "AI delivery proof needs review.": "runtimeText.aiDeliveryProofNeedsReview",
    "AI delivery test proof needs review.": "runtimeText.aiDeliveryTestProofNeedsReview",
    "AI delivery test proof not applicable.": "runtimeText.aiDeliveryTestProofNotApplicable",
    "AI delivery test proof ready.": "runtimeText.aiDeliveryTestProofReady",
    "AI diagnosis delivery channel setup blocked.": "runtimeText.aiDiagnosisDeliveryChannelSetupBlocked",
    "AI diagnosis delivery ready.": "runtimeText.aiDiagnosisDeliveryReady",
    "AI diagnosis sample": "runtimeText.aiDiagnosisSample",
    "AI diagnosis WeCom": "runtimeText.aiDiagnosisWeCom",
    "AI report and diagnosis WeCom": "runtimeText.aiReportAndDiagnosisWeCom",
    "AI diagnosis update and close notification samples both succeeded after the latest channel update.": "runtimeText.aiDiagnosisUpdateAndCloseNotificationSamplesBothSucceededAfterTheLatest",
    "AI diagnosis updates and close notifications require an Enterprise WeChat channel.": "runtimeText.aiDiagnosisUpdatesAndCloseNotificationsRequireAnEnterpriseWechatChannel",
    "AI diagnosis scopes need review.": "runtimeText.aiDiagnosisScopesNeedReview",
    "Add diagnosis_consultation and diagnosis_close scopes before using this channel for AI diagnosis rooms.": "runtimeText.addDiagnosisConsultationAndDiagnosisCloseScopesBeforeUsingThisChannelFor",
    "Add diagnosis_consultation and diagnosis_close scopes before collecting AI delivery proof for Enterprise WeChat.": "runtimeText.addDiagnosisScopesBeforeCollectingProof",
    "Add report, diagnosis_consultation, and diagnosis_close scopes to one Enterprise WeChat channel before accepting automatic diagnosis rollout.": "runtimeText.addAllScopesBeforeAutomaticDiagnosisRollout",
    "All AI diagnosis delivery candidates have current AI diagnosis and close notification sample proof.": "runtimeText.allAiDeliveryCandidatesHaveCurrentProof",
    "Back to workflow": "runtimeText.backToWorkflow",
    "Channel disabled.": "runtimeText.channelDisabled",
    "Channel name is required.": "runtimeText.channelNameIsRequired",
    "Channel name must be 120 bytes or fewer.": "runtimeText.channelNameMustBe120BytesOrFewer",
    "Content proof missing": "runtimeText.contentProofMissing",
    "Configured AI diagnosis delivery candidates are blocked by kind, state, scope, or secret-reference readiness.": "runtimeText.configuredAiDiagnosisDeliveryCandidatesAreBlockedByKindStateScopeOr",
    "Configured Enterprise WeChat rollout candidates are blocked by kind, state, or credential readiness.": "runtimeText.configuredEnterpriseWechatRolloutCandidatesAreBlockedByKindStateOrCredential",
    "Create or enable an Enterprise WeChat channel with diagnosis_consultation and diagnosis_close scopes before enabling automatic diagnosis room delivery.": "runtimeText.createOrEnableAnEnterpriseWechatChannelWithDiagnosisConsultationAndDiagnosis",
    "Create or enable an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes before accepting automatic diagnosis rollout.": "runtimeText.createOrEnableAnEnterpriseWechatChannelWithReportDiagnosisConsultationAnd",
    "Credential secret not selected.": "runtimeText.credentialSecretNotSelected",
    "Delivery scopes need review.": "runtimeText.deliveryScopesNeedReview",
    "Delivery scopes not selected.": "runtimeText.deliveryScopesNotSelected",
    "Delivery scopes ready.": "runtimeText.deliveryScopesReady",
    "Diagnosis consultation and close notifications require an Enterprise WeChat channel. Use report scope only for webhook, DingTalk, Feishu, Slack, or Email delivery.": "runtimeText.diagnosisConsultationAndCloseNotificationsRequireAnEnterpriseWechatChannelUseReport",
    "Diagnosis close sample": "runtimeText.diagnosisCloseSample",
    "Diagnosis delivery scopes ready.": "runtimeText.diagnosisDeliveryScopesReady",
    "Diagnosis delivery scopes require an Enterprise WeChat channel.": "runtimeText.diagnosisDeliveryScopesRequireAnEnterpriseWechatChannel",
    "DingTalk robot webhook URL": "runtimeText.dingtalkRobotWebhookUrl",
    "DingTalk credential contract selected.": "runtimeText.dingtalkCredentialContractSelected",
    "Email credential contract selected.": "runtimeText.emailCredentialContractSelected",
    "Endpoint URL cannot be stored as a secret reference.": "runtimeText.endpointUrlCannotBeStoredAsASecretReference",
    "Enable this channel before it can deliver AI diagnosis updates or close notifications.": "runtimeText.enableThisChannelBeforeItCanDeliverAiDiagnosisUpdatesOrClose",
    "Enterprise WeChat required.": "runtimeText.enterpriseWechatRequired",
    "Enterprise WeChat required for diagnosis delivery.": "runtimeText.enterpriseWechatRequiredForDiagnosisDelivery",
    "Enterprise WeChat robot webhook URL": "runtimeText.enterpriseWechatRobotWebhookUrl",
    "Enterprise WeChat rollout channel blocked.": "runtimeText.enterpriseWechatRolloutChannelBlocked",
    "Enterprise WeChat rollout needs review.": "runtimeText.enterpriseWechatRolloutNeedsReview",
    "Enterprise WeChat rollout proof is ready for automatic diagnosis: report delivery, AI diagnosis updates, and close notifications are all covered.": "runtimeText.enterpriseWechatRolloutProofReady",
    "Feishu credential contract selected.": "runtimeText.feishuCredentialContractSelected",
    "Feishu or Lark custom bot webhook URL": "runtimeText.feishuOrLarkCustomBotWebhookUrl",
    "HTTP webhook URL": "runtimeText.httpWebhookUrl",
    "Label keys must be non-empty.": "runtimeText.labelKeysMustBeNonEmpty",
    "Labels exceed the allowed length.": "runtimeText.labelsExceedTheAllowedLength",
    "Labels must contain 32 entries or fewer.": "runtimeText.labelsMustContain32EntriesOrFewer",
    "Labels must not contain control characters.": "runtimeText.labelsMustNotContainControlCharacters",
    "Labels must use key=value lines.": "runtimeText.labelsMustUseKeyValueLines",
    "No AI diagnosis delivery channel configured.": "runtimeText.noAiDiagnosisDeliveryChannelConfigured",
    "No Enterprise WeChat rollout channel configured.": "runtimeText.noEnterpriseWechatRolloutChannelConfigured",
    "Prepared an Enterprise WeChat channel for AI diagnosis updates and close notifications. Paste the secret reference before saving.": "runtimeText.preparedAnEnterpriseWechatChannelForAiDiagnosisUpdatesAndCloseNotifications",
    "Prepared an Enterprise WeChat channel for final report delivery. Paste the secret reference before saving.": "runtimeText.preparedAnEnterpriseWechatChannelForFinalReportDeliveryPasteTheSecret",
    "Prepared an Enterprise WeChat channel for final reports, automatic diagnosis updates, and close notifications. Paste the secret reference before saving.": "runtimeText.preparedAnEnterpriseWechatChannelForFinalReportsAutomaticDiagnosisUpdatesAnd",
    "Ready for AI diagnosis updates and close notifications through Enterprise WeChat.": "runtimeText.readyForAiDiagnosisUpdatesAndCloseNotificationsThroughEnterpriseWechat",
    "Return to workflow policies after Enterprise WeChat channel scopes and AI delivery proof are ready.": "runtimeText.returnToWorkflowPoliciesAfterEnterpriseWechatChannelScopesAndAiDelivery",
    "Run current AI diagnosis sample and diagnosis close sample tests before accepting Enterprise WeChat automatic diagnosis rollout.": "runtimeText.runAiAndCloseSamplesBeforeRollout",
    "Run current AI diagnosis sample and diagnosis close sample tests before relying on Enterprise WeChat for automatic diagnosis room delivery.": "runtimeText.runAiAndCloseSamplesBeforeDelivery",
    "Report delivery WeCom": "runtimeText.reportDeliveryWecom",
    "Secret reference is required.": "runtimeText.secretReferenceIsRequired",
    "Secret reference must be 256 bytes or fewer.": "runtimeText.secretReferenceMustBe256BytesOrFewer",
    "Secret reference must not be an endpoint URL.": "runtimeText.secretReferenceMustNotBeAnEndpointUrl",
    "Secret reference must not contain whitespace or control characters.": "runtimeText.secretReferenceMustNotContainWhitespaceOrControlCharacters",
    "Slack incoming webhook URL": "runtimeText.slackIncomingWebhookUrl",
    "SMTP URL with from/to recipients": "runtimeText.smtpUrlWithFromToRecipients",
    "Select at least one delivery scope.": "runtimeText.selectAtLeastOneDeliveryScope",
    "Select diagnosis_consultation and diagnosis_close scopes before collecting AI delivery test proof.": "runtimeText.selectDiagnosisConsultationAndDiagnosisCloseScopesBeforeCollectingAiDeliveryTest",
    "Select report for final report delivery, diagnosis_consultation for AI diagnosis updates, and diagnosis_close for close notifications.": "runtimeText.selectReportForFinalReportDeliveryDiagnosisConsultationForAiDiagnosisUpdates",
    "Slack credential contract selected.": "runtimeText.slackCredentialContractSelected",
    "Store endpoint URLs in server-side secret storage and enter only the secret reference here.": "runtimeText.storeEndpointUrlsInServerSideSecretStorageAndEnterOnlyThe",
    "Test sends a diagnosis room close notification sample.": "runtimeText.testSendsADiagnosisRoomCloseNotificationSample",
    "Test sends a generic transport notification sample.": "runtimeText.testSendsAGenericTransportNotificationSample",
    "Test sends an AI diagnosis update sample, not a raw Alertmanager alert.": "runtimeText.testSendsAnAiDiagnosisUpdateSampleNotARawAlertmanagerAlert",
    "Transport sample": "runtimeText.transportSample",
    "The channel can be saved, but workflows using the missing scope will be blocked until it is added.": "runtimeText.theChannelCanBeSavedButWorkflowsUsingTheMissingScopeWill",
    "The channel can support AI diagnosis updates and close notifications. Add report scope only when final report delivery should use this channel.": "runtimeText.theChannelCanSupportAiDiagnosisUpdatesAndCloseNotificationsAddReport",
    "The channel can support final reports, auto-room AI diagnosis updates, and close notifications.": "runtimeText.theChannelCanSupportFinalReportsAutoRoomAiDiagnosisUpdatesAnd",
    "WeCom credential contract selected.": "runtimeText.wecomCredentialContractSelected",
    "Webhook credential contract selected.": "runtimeText.webhookCredentialContractSelected",
    "credential secret reference stores an endpoint URL": "runtimeText.credentialSecretReferenceStoresEndpoint",
    "missing credential secret reference": "runtimeText.missingCredentialSecretReference",
    "not Enterprise WeChat": "runtimeText.notEnterpriseWechat",
} as const;

export function localizeNotificationChannelText(value: string, t: NotificationChannelTranslator): string {
  const key =
    notificationChannelRuntimeTextKeys[
      value as keyof typeof notificationChannelRuntimeTextKeys
    ];
  if (key !== undefined) {
    return t(key);
  }
  let match = value.match(/^Loaded notification channel #(\d+) for delivery review\.$/);
  if (match) {
    return t("runtimePattern.loadedChannelForReview", { id: match[1]! });
  }
  match = value.match(/^Notification channel #(\d+) was not found\. Select an existing channel or create a replacement\.$/);
  if (match) {
    return t("runtimePattern.channelNotFound", { id: match[1]! });
  }
  match = value.match(/^Run (.+) tests after the latest channel update before relying on this channel for AI diagnosis rooms\.$/);
  if (match) {
    const tests = match[1]!
      .split(" and ")
      .map((item) => localizeNotificationChannelText(item, t))
      .join(" / ");
    return t("runtimePattern.runTestsAfterUpdate", { tests });
  }
  match = value.match(/^Select a deployment-managed secret reference before testing the channel, then map it in (.+)\.$/);
  if (match) {
    return t("runtimePattern.mapSecretReference", { setting: match[1]! });
  }
  match = value.match(/^(\d+) AI delivery channels? proof-ready\.$/);
  if (match) {
    return t("runtimePattern.aiDeliveryChannelsReady", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) Enterprise WeChat rollout channels? ready\.$/);
  if (match) {
    return t("runtimePattern.weComRolloutChannelsReady", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^Label key "(.+)" is duplicated\.$/);
  if (match) {
    return t("runtimePattern.labelKeyDuplicated", { key: match[1]! });
  }
  match = value.match(/^Backend tests resolve this secret reference through (.+) and construct a generic webhook provider from the endpoint\.$/);
  if (match) {
    return t("runtimePattern.backendGenericWebhook", { setting: match[1]! });
  }
  match = value.match(/^Backend tests resolve this secret reference through (.+) and require one DingTalk robot webhook endpoint\.$/);
  if (match) {
    return t("runtimePattern.backendDingTalk", { setting: match[1]! });
  }
  match = value.match(/^Backend tests resolve this secret reference through (.+) and require one Feishu or Lark custom bot webhook endpoint\.$/);
  if (match) {
    return t("runtimePattern.backendFeishu", { setting: match[1]! });
  }
  match = value.match(/^Backend tests resolve this secret reference through (.+) and require one HTTPS Enterprise WeChat robot webhook endpoint\.$/);
  if (match) {
    return t("runtimePattern.backendWeCom", { setting: match[1]! });
  }
  match = value.match(/^Backend tests resolve this secret reference through (.+) and require one SMTP URL with from and to query parameters\.$/);
  if (match) {
    return t("runtimePattern.backendEmail", { setting: match[1]! });
  }
  match = value.match(/^Backend tests resolve this secret reference through (.+) and require one Slack incoming webhook endpoint\.$/);
  if (match) {
    return t("runtimePattern.backendSlack", { setting: match[1]! });
  }
  match = value.match(/^(\d+) AI diagnosis delivery channels? can be used now; (\d+) candidate channels? still need setup or proof review\.$/);
  if (match) {
    return t("runtimePattern.aiDeliveryChannelsAvailable", {
      ready: Number(match[1]),
      review: Number(match[2]),
    });
  }
  match = value.match(/^(\d+) Enterprise WeChat rollout channels? can be used now; (\d+) candidate channels? still need scope, state, credential, or proof review\.$/);
  if (match) {
    return t("runtimePattern.weComRolloutChannelsAvailable", {
      ready: Number(match[1]),
      review: Number(match[2]),
    });
  }
  return value;
}
