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

const deliveryScopeOptions: CheckboxOptionType<NotificationDeliveryScope>[] = [
  { label: "Reports", value: "report" },
  { label: "Diagnosis consultation", value: "diagnosis_consultation" },
  { label: "Diagnosis close", value: "diagnosis_close" },
];

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
    refreshMessage: "Channels refreshed.",
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
    resourceLabel:
      editingID === null
        ? "notification channel creation"
        : `notification channel #${editingID}`,
  });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadChannels,
    errorStatus,
    isChecking: !clientReady || currentAuthorization.isChecking,
    resourceLabel: "notification channels",
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
      setNotice({ kind: "error", message: parsed.message });
      return;
    }

    try {
      await saveChannel.mutateAsync({
        channelID: editingID,
        body: parsed.value,
      });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }

    form.setFieldsValue(emptyNotificationChannelForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({ kind: "info", message: "Channel saved." });
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
        message: "AI delivery test proof is current.",
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

    setNotice({ kind: "info", message: "AI delivery proof tests completed." });
  }

  async function handleTestAllAIProof() {
    if (authorizedAIProofRunnableChannels.length === 0) {
      setNotice({
        kind: "info",
        message: "No authorized AI delivery proof tests are pending.",
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
      message: `AI delivery proof tests completed for ${testedChannels} channel${testedChannels === 1 ? "" : "s"} and ${testedContentKinds} sample${testedContentKinds === 1 ? "" : "s"}.`,
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
      <Row aria-label="Notification channel metrics" gutter={[12, 12]}>
        <MetricCard label="Channels" value={channels.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Report scope" value={summary.report} />
        <MetricCard
          label="Consultation scope"
          value={summary.diagnosisConsultation}
        />
        <MetricCard label="Close scope" value={summary.diagnosisClose} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} /> : null}
      {launchNotice ? (
        <Alert
          aria-label="Notification channel launch preset"
          description={launchNotice}
          message="Channel action loaded"
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
        summaries={aiProofSummaries}
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
                  New
                </Button>
              )
            }
            title={
              editingID === null ? "New Channel" : `Edit Channel #${editingID}`
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
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Channel name is required." },
                  {
                    max: 120,
                    message: "Channel name must be 120 characters or fewer.",
                  },
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item
                label="Kind"
                name="kind"
                rules={[{ required: true, message: "Kind is required." }]}
              >
                <Segmented block options={channelKindOptions} />
              </Form.Item>

              <Form.Item
                label="Secret reference"
                name="secretRef"
                extra={
                  <Typography.Text type="secondary">
                    Store the real endpoint in{" "}
                    {notificationChannelSecretResolverEnvKey}; enter only the
                    secret key here.
                  </Typography.Text>
                }
                rules={[
                  { required: true, message: "Secret reference is required." },
                ]}
              >
                <Input
                  autoComplete="off"
                  placeholder="secret/example/ops-webhook"
                />
              </Form.Item>

              <Form.Item
                label="Delivery scopes"
                name="deliveryScopes"
                rules={[
                  {
                    required: true,
                    message: "Select at least one delivery scope.",
                  },
                ]}
              >
                <Checkbox.Group options={deliveryScopeOptions} />
              </Form.Item>

              <Form.Item label="Labels" name="labelsText">
                <Input.TextArea
                  autoSize={{ minRows: 3, maxRows: 6 }}
                  placeholder="team=ops"
                />
              </Form.Item>

              <Form.Item label="Enabled" name="enabled" valuePropName="checked">
                <Switch />
              </Form.Item>

              <Form.Item noStyle shouldUpdate>
                {(channelForm) => (
                  <NotificationChannelDeliveryPreview
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
                  Save Channel
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
                disabled={busy || !canReadChannels}
                icon={<ReloadOutlined />}
                loading={busy}
                onClick={handleRefresh}
                type="default"
              >
                Refresh
              </Button>
            }
            title="Configured Channels"
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
            ? "Test review"
            : "Settings"
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
  workflowReturn,
}: {
  onRunMissingProof: () => void;
  readiness: NotificationChannelEnterpriseWeChatRolloutReadiness;
  runDisabled: boolean;
  runLoading: boolean;
  runnableChannelCount: number;
  workflowReturn: NotificationChannelWorkflowReturn | null;
}) {
  const canRunProof =
    readiness.missingContentKinds.length > 0 && runnableChannelCount > 0;
  const canReturnToWorkflow =
    workflowReturn !== null && readiness.status === "ready";
  return (
    <Alert
      aria-label="Enterprise WeChat automatic diagnosis rollout readiness"
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
                Run missing AI proof
              </Button>
            ) : null}
            {canReturnToWorkflow ? (
              <Button
                href={workflowReturn.href}
                icon={<BranchesOutlined />}
                size="small"
                type="primary"
              >
                {workflowReturn.label}
              </Button>
            ) : null}
          </Space>
        ) : null
      }
      description={
        <Space direction="vertical" size={8}>
          <Typography.Text>{readiness.detail}</Typography.Text>
          {workflowReturn !== null ? (
            <Typography.Text type="secondary">
              {canReturnToWorkflow
                ? workflowReturn.detail
                : "Finish Enterprise WeChat channel setup and AI delivery proof before returning to workflow enablement."}
            </Typography.Text>
          ) : null}
          <Descriptions
            column={1}
            items={[
              {
                children: readiness.candidateChannelIDs.length,
                key: "candidates",
                label: "Candidates",
              },
              {
                children: readiness.readyChannelIDs.length,
                key: "ready",
                label: "Ready",
              },
              {
                children: readiness.reviewChannelIDs.length,
                key: "review",
                label: "Review",
              },
              {
                children: readiness.blockedChannelIDs.length,
                key: "blocked",
                label: "Blocked",
              },
              {
                children:
                  readiness.missingScopes.length === 0
                    ? "None"
                    : readiness.missingScopes.join(", "),
                key: "missingScopes",
                label: "Missing scopes",
              },
              {
                children:
                  readiness.missingContentKinds.length === 0
                    ? "None"
                    : readiness.missingContentKinds
                        .map(notificationChannelTestContentKindLabel)
                        .join(", "),
                key: "missingProof",
                label: "Missing proof",
              },
            ]}
            size="small"
          />
        </Space>
      }
      message={readiness.label}
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
  onRunMissingProof,
  readiness,
  runDisabled,
  runLoading,
  runnableChannelCount,
  summaries,
}: {
  onRunMissingProof: () => void;
  readiness: NotificationChannelAIProofInventoryReadiness;
  runDisabled: boolean;
  runLoading: boolean;
  runnableChannelCount: number;
  summaries: NotificationChannelAIProofSummary[];
}) {
  return (
    <Alert
      aria-label="Enterprise WeChat AI delivery proof readiness"
      action={
        runnableChannelCount === 0 ? null : (
          <Button
            disabled={runDisabled}
            icon={<CheckCircleOutlined />}
            loading={runLoading}
            onClick={onRunMissingProof}
            size="small"
          >
            Run missing AI proof
          </Button>
        )
      }
      description={
        <Space direction="vertical" size={4}>
          <Typography.Text>{readiness.detail}</Typography.Text>
          {readiness.candidateChannelIDs.length > 0 ? (
            <Typography.Text type="secondary">
              Candidates: {readiness.candidateChannelIDs.length}; proof-ready:{" "}
              {readiness.readyChannelIDs.length}; review:{" "}
              {readiness.reviewChannelIDs.length}; blocked:{" "}
              {readiness.blockedChannelIDs.length}.
            </Typography.Text>
          ) : null}
          {readiness.missingContentKinds.length > 0 ? (
            <Typography.Text type="secondary">
              Missing proof:{" "}
              {readiness.missingContentKinds
                .map(notificationChannelTestContentKindLabel)
                .join(", ")}
              .
            </Typography.Text>
          ) : null}
          {summaries.length > 0 ? (
            <Space direction="vertical" size={2}>
              {summaries.map((summary) => (
                <AIProofSummaryRow key={summary.channelID} summary={summary} />
              ))}
            </Space>
          ) : null}
        </Space>
      }
      message={readiness.label}
      role={readiness.status === "blocked" ? "alert" : "status"}
      showIcon
      type={aiProofInventoryAlertType(readiness.status)}
    />
  );
}

function AIProofSummaryRow({
  summary,
}: {
  summary: NotificationChannelAIProofSummary;
}) {
  return (
    <Space size={[4, 4]} wrap>
      <Typography.Text strong>
        #{summary.channelID} {summary.channelName}
      </Typography.Text>
      <Tag color={channelAIProofReadinessColor(summary.status)}>
        {channelAIProofReadinessLabel(summary.status)}
      </Tag>
      {summary.contents.map((content) => (
        <Tag
          color={aiProofContentSummaryColor(content.status)}
          key={content.contentKind}
        >
          {content.label}: {aiProofContentSummaryLabel(content.status)}
          {content.checkedAt ? ` ${formatDateTime(content.checkedAt)}` : ""}
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
): string {
  switch (status) {
    case "current":
      return "current";
    case "stale":
      return "stale";
    case "missing":
      return "missing";
    case "failed":
      return "failed";
    case "blocked":
      return "blocked";
    case "unsupported":
      return "unsupported";
    case "invalid":
      return "invalid";
  }
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
  values,
}: {
  values: NotificationChannelFormState;
}) {
  const normalized = normalizeNotificationChannelFormValues(values);
  const readiness = notificationChannelDeliveryReadiness(normalized);
  const credentials = notificationChannelCredentialReadiness(normalized);
  const testSample = notificationChannelTestSample(normalized);
  const items: DescriptionsProps["items"] = [
    {
      children: credentials.kindLabel,
      key: "kind",
      label: "Adapter",
    },
    {
      children: normalized.enabled ? "Enabled" : "Draft",
      key: "state",
      label: "State",
    },
    {
      children: credentials.secretConfigured
        ? credentials.expectedCredential
        : "Missing",
      key: "credential",
      label: "Credential",
    },
    {
      children: credentials.resolverEnvKey,
      key: "resolver",
      label: "Server resolver",
    },
    {
      children: credentials.secretRefExample,
      key: "secretRefExample",
      label: "Secret ref example",
    },
    {
      children: readiness.hasReportScope ? "Ready" : "Missing",
      key: "report",
      label: "Final reports",
    },
    {
      children: readiness.hasDiagnosisConsultationScope ? "Ready" : "Missing",
      key: "diagnosisConsultation",
      label: "AI diagnosis updates",
    },
    {
      children: readiness.hasDiagnosisCloseScope ? "Ready" : "Missing",
      key: "diagnosisClose",
      label: "Auto-room close",
    },
    {
      children: testSample.label,
      key: "testSample",
      label: "Test sample",
    },
  ];

  return (
    <div
      aria-label="Notification channel delivery readiness"
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={channelDeliveryReadinessColor(readiness.status)}>
            {channelDeliveryReadinessLabel(readiness.status)}
          </Tag>
          <Tag color={channelCredentialReadinessColor(credentials.status)}>
            {credentials.kindLabel}
          </Tag>
          <Tag color={readiness.hasReportScope ? "blue" : "default"}>
            Report
          </Tag>
          <Tag
            color={readiness.hasDiagnosisConsultationScope ? "cyan" : "default"}
          >
            Diagnosis consultation
          </Tag>
          <Tag color={readiness.hasDiagnosisCloseScope ? "purple" : "default"}>
            Diagnosis close
          </Tag>
        </Space>
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
        {readiness.missingScopes.length > 0 ? (
          <Typography.Text type="secondary">
            Missing scopes: {readiness.missingScopes.join(", ")}
          </Typography.Text>
        ) : null}
        <Typography.Text type="secondary">{credentials.detail}</Typography.Text>
        <Typography.Text type="secondary">{testSample.detail}</Typography.Text>
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
): string {
  switch (status) {
    case "blocked":
      return "Blocked";
    case "ready":
      return "Ready";
    case "review":
      return "Review";
    case "pending":
      return "Pending";
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
}: NotificationChannelTableProps) {
  const columns: TableColumnsType<NotificationChannelProfile> = [
    {
      key: "name",
      title: "Name",
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
      title: "Kind",
      render: (kind: NotificationChannelProfile["kind"]) => <Tag>{kind}</Tag>,
    },
    {
      dataIndex: "delivery_scopes",
      key: "delivery_scopes",
      title: "Scopes",
      render: (scopes: NotificationDeliveryScope[]) => (
        <Space wrap>
          {scopes.map((scope) => (
            <Tag color={scope === "report" ? "blue" : "purple"} key={scope}>
              {scope}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: "State",
      render: (enabled: boolean) => (
        <Tag color={enabled ? "green" : "default"}>
          {enabled ? "Enabled" : "Disabled"}
        </Tag>
      ),
    },
    {
      key: "ai_diagnosis",
      title: "AI Diagnosis",
      render: (_, channel) => (
        <ChannelAIRoomReadiness
          channel={channel}
          testProof={notificationChannelTestProofForChannel(
            channel,
            testResults,
          )}
        />
      ),
    },
    {
      key: "last_test",
      title: "Test Proof",
      render: (_, channel) => (
        <ChannelTestProof
          proof={notificationChannelTestProofForChannel(channel, testResults)}
        />
      ),
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: "Updated",
      render: (value: string) => formatDateTime(value),
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
              Test
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
                AI proof
              </Button>
            ) : null}
            <Dropdown
              disabled={rowBusy || !canTest}
              menu={{
                items: notificationChannelTestMenuItems(channel),
                onClick: ({ key }) =>
                  onTest(channel, key as NotificationChannelTestContentKind),
              }}
              trigger={["click"]}
            >
              <Button
                aria-label={`Select test sample for ${channel.name}`}
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
              Edit
            </Button>
          </Space>
        );
      },
      title: "Actions",
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
              emptyDescription: "No notification channels",
              resourceLabel: "notification channels",
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
  testProof,
}: {
  channel: NotificationChannelProfile;
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
        {channelAIRoomReadinessLabel(readiness.status)}
      </Tag>
      <Tag color={channelAIProofReadinessColor(proofReadiness.status)}>
        {channelAIProofReadinessLabel(proofReadiness.status)}
      </Tag>
      <Typography.Text
        ellipsis={{ tooltip: readiness.detail }}
        type="secondary"
      >
        {readiness.label}
      </Typography.Text>
      <Typography.Text
        ellipsis={{ tooltip: proofReadiness.detail }}
        type="secondary"
      >
        {proofReadiness.label}
      </Typography.Text>
      {readiness.missingScopes.length > 0 ? (
        <Typography.Text type="secondary">
          Missing {readiness.missingScopes.join(", ")}
        </Typography.Text>
      ) : null}
      {proofReadiness.missingContentKinds.length > 0 ? (
        <Typography.Text type="secondary">
          Test{" "}
          {proofReadiness.missingContentKinds
            .map(notificationChannelTestContentKindLabel)
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
): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "review":
      return "Review";
    case "blocked":
      return "Blocked";
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
): string {
  switch (status) {
    case "ready":
      return "Proof ready";
    case "review":
      return "Proof review";
    case "blocked":
      return "Proof blocked";
  }
}

function notificationChannelTestMenuItems(
  channel: NotificationChannelProfile,
): MenuProps["items"] {
  const items: Array<{
    key: NotificationChannelTestContentKind;
    label: string;
  }> = [];
  if (channel.delivery_scopes.includes("diagnosis_consultation")) {
    items.push({ key: "ai_diagnosis_sample", label: "AI diagnosis sample" });
  }
  if (channel.delivery_scopes.includes("diagnosis_close")) {
    items.push({
      key: "diagnosis_close_sample",
      label: "Diagnosis close sample",
    });
  }
  items.push({ key: "transport_sample", label: "Transport sample" });
  return items;
}

function ChannelTestProof({
  proof,
}: {
  proof?: NotificationChannelTestProofBundle;
}) {
  const results = notificationChannelTestProofResults(proof);
  if (results.length === 0) {
    return <Typography.Text type="secondary">Not tested</Typography.Text>;
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
            {notificationChannelTestContentKindLabel(result.content_kind)}
          </Tag>
          <Tag color={channelTestStatusColor(result.status)}>
            {result.status}
          </Tag>
          {result.content_sha256 ? (
            <Typography.Text type="secondary">
              {result.content_sha256.slice(0, 12)}
            </Typography.Text>
          ) : null}
          <Typography.Text type="secondary">
            {formatDateTime(result.checked_at)}
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
