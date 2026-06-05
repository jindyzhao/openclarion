"use client";

import {
  EditOutlined,
  MessageOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  SendOutlined
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
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
  Typography
} from "antd";
import type { CheckboxOptionType, TableColumnsType } from "antd";
import { useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

import { formatDateTime } from "../format";
import {
  settingsErrorMessage,
  type SettingsNotice,
  useSettingsList,
  useSettingsMutation
} from "../query-state";
import {
  refreshNotificationChannelProfiles,
  submitNotificationChannelProfile,
  testNotificationChannel
} from "./client-api";
import {
  channelToFormState,
  emptyNotificationChannelForm,
  formStateToWriteRequest
} from "./format";
import type {
  NotificationChannelFormState,
  NotificationChannelProfile,
  NotificationChannelProfileListResponse,
  NotificationChannelTestResult,
  NotificationChannelProfileWriteRequest,
  NotificationDeliveryScope
} from "./types";

type NotificationChannelSettingsManagerProps = {
  result: ApiResult<NotificationChannelProfileListResponse>;
};

const deliveryScopeOptions: CheckboxOptionType<NotificationDeliveryScope>[] = [
  { label: "Reports", value: "report" },
  { label: "Diagnosis close", value: "diagnosis_close" }
];

const notificationChannelProfilesQueryKey = ["settings", "notification-channels"] as const;

type SaveChannelVariables = {
  body: NotificationChannelProfileWriteRequest;
  channelID: number | null;
};

export function NotificationChannelSettingsManager({ result }: NotificationChannelSettingsManagerProps) {
  const [form] = Form.useForm<NotificationChannelFormState>();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [testingID, setTestingID] = useState<number | null>(null);
  const [testResults, setTestResults] = useState<Record<number, NotificationChannelTestResult>>({});
  const {
    items: channels,
    notice,
    query,
    refresh,
    setNotice
  } = useSettingsList({
    initialResult: result,
    queryKey: notificationChannelProfilesQueryKey,
    queryFn: refreshNotificationChannelProfiles,
    refreshMessage: "Channels refreshed.",
    selectItems: (response) => response.items
  });
  const saveChannel = useSettingsMutation<SaveChannelVariables, NotificationChannelProfile>({
    invalidateQueryKey: notificationChannelProfilesQueryKey,
    mutationFn: ({ channelID, body }) => submitNotificationChannelProfile(channelID, body)
  });
  const busy = query.isFetching || saveChannel.isPending;

  const summary = useMemo(() => {
    const enabled = channels.filter((channel) => channel.enabled).length;
    const report = channels.filter((channel) => channel.delivery_scopes.includes("report")).length;
    const diagnosisClose = channels.filter((channel) => channel.delivery_scopes.includes("diagnosis_close")).length;
    return { enabled, report, diagnosisClose };
  }, [channels]);

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
      await saveChannel.mutateAsync({ channelID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }

    form.setFieldsValue(emptyNotificationChannelForm());
    setEditingID(null);
    setNotice({ kind: "info", message: "Channel saved." });
  }

  async function handleTest(channel: NotificationChannelProfile) {
    setTestingID(channel.id);
    const tested = await testNotificationChannel(channel.id);
    setTestingID(null);
    if (!tested.ok) {
      setNotice({ kind: "error", message: tested.error.message });
      return;
    }

    setTestResults((current) => ({ ...current, [channel.id]: tested.data }));
    setNotice({ kind: noticeKindForChannelTestStatus(tested.data.status), message: tested.data.message });
  }

  function editChannel(channel: NotificationChannelProfile) {
    setEditingID(channel.id);
    form.setFieldsValue(channelToFormState(channel));
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyNotificationChannelForm());
    setNotice(null);
  }

  return (
    <div className="stack">
      <Row aria-label="Notification channel metrics" gutter={[12, 12]}>
        <MetricCard label="Channels" value={channels.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Report scope" value={summary.report} />
        <MetricCard label="Close scope" value={summary.diagnosisClose} />
      </Row>

      {notice ? <Notice notice={notice} /> : null}

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
            title={editingID === null ? "New Channel" : `Edit Channel #${editingID}`}
          >
            <Form<NotificationChannelFormState>
              disabled={busy}
              form={form}
              initialValues={emptyNotificationChannelForm()}
              layout="vertical"
              onFinish={handleSubmit}
            >
              <Form.Item
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Channel name is required." },
                  { max: 120, message: "Channel name must be 120 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item label="Kind" name="kind" rules={[{ required: true, message: "Kind is required." }]}>
                <Segmented block options={[{ value: "webhook", label: "Webhook" }]} />
              </Form.Item>

              <Form.Item
                label="Secret reference"
                name="secretRef"
                rules={[{ required: true, message: "Secret reference is required." }]}
              >
                <Input autoComplete="off" placeholder="secret/example/ops-webhook" />
              </Form.Item>

              <Form.Item
                label="Delivery scopes"
                name="deliveryScopes"
                rules={[{ required: true, message: "Select at least one delivery scope." }]}
              >
                <Checkbox.Group options={deliveryScopeOptions} />
              </Form.Item>

              <Form.Item label="Labels" name="labelsText">
                <Input.TextArea autoSize={{ minRows: 3, maxRows: 6 }} placeholder="team=ops" />
              </Form.Item>

              <Form.Item label="Enabled" name="enabled" valuePropName="checked">
                <Switch />
              </Form.Item>

              <Space wrap>
                <Button htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
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
              <Button disabled={busy} icon={<ReloadOutlined />} loading={busy} onClick={handleRefresh} type="default">
                Refresh
              </Button>
            }
            title="Configured Channels"
          >
            <NotificationChannelTable
              busy={busy}
              channels={channels}
              onEdit={editChannel}
              onTest={handleTest}
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
  const type = notice.kind === "error" ? "error" : notice.kind === "warning" ? "warning" : "success";
  return (
    <Alert
      description={notice.message}
      message={notice.kind === "error" ? "Request failed" : notice.kind === "warning" ? "Test review" : "Settings"}
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={type}
    />
  );
}

type NotificationChannelTableProps = {
  busy: boolean;
  channels: NotificationChannelProfile[];
  onEdit: (channel: NotificationChannelProfile) => void;
  onTest: (channel: NotificationChannelProfile) => void;
  testResults: Record<number, NotificationChannelTestResult>;
  testingID: number | null;
};

function NotificationChannelTable({
  busy,
  channels,
  onEdit,
  onTest,
  testResults,
  testingID
}: NotificationChannelTableProps) {
  const columns: TableColumnsType<NotificationChannelProfile> = [
    {
      key: "name",
      title: "Name",
      render: (_, channel) => (
        <Space direction="vertical" size={2}>
          <Typography.Text strong>{channel.name}</Typography.Text>
          <Typography.Text type="secondary">{channel.secret_ref}</Typography.Text>
        </Space>
      )
    },
    {
      dataIndex: "kind",
      key: "kind",
      title: "Kind",
      render: (kind: NotificationChannelProfile["kind"]) => <Tag>{kind}</Tag>
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
      )
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: "State",
      render: (enabled: boolean) => <Tag color={enabled ? "green" : "default"}>{enabled ? "Enabled" : "Disabled"}</Tag>
    },
    {
      key: "last_test",
      title: "Last Test",
      render: (_, channel) => <ChannelTestResult result={testResults[channel.id]} />
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: "Updated",
      render: (value: string) => formatDateTime(value)
    },
    {
      key: "actions",
      render: (_, channel) => (
        <Space wrap>
          <Button
            disabled={busy || (testingID !== null && testingID !== channel.id)}
            icon={<SendOutlined />}
            loading={testingID === channel.id}
            onClick={() => onTest(channel)}
            type="link"
          >
            Test
          </Button>
          <Button disabled={busy || testingID !== null} icon={<EditOutlined />} onClick={() => onEdit(channel)} type="link">
            Edit
          </Button>
        </Space>
      ),
      title: "Actions"
    }
  ];

  return (
    <Table<NotificationChannelProfile>
      columns={columns}
      dataSource={channels}
      loading={busy}
      locale={{
        emptyText: (
          <Empty
            description="No notification channels"
            image={<MessageOutlined aria-hidden className="settings-empty-icon" />}
          />
        )
      }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1080 }}
    />
  );
}

function ChannelTestResult({ result }: { result?: NotificationChannelTestResult }) {
  if (!result) {
    return <Typography.Text type="secondary">Not tested</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Tag color={channelTestStatusColor(result.status)}>{result.status}</Tag>
      <Typography.Text type="secondary">{result.reason_code}</Typography.Text>
      {result.provider_status === "" ? null : (
        <Typography.Text type="secondary">{result.provider_status}</Typography.Text>
      )}
    </Space>
  );
}

function channelTestStatusColor(status: NotificationChannelTestResult["status"]) {
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

function noticeKindForChannelTestStatus(status: NotificationChannelTestResult["status"]): SettingsNotice["kind"] {
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
