"use client";

import { ApiOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SaveOutlined } from "@ant-design/icons";
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
  Table,
  Tag,
  Typography
} from "antd";
import type { TableColumnsType } from "antd";
import { useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

import { formatDateTime } from "../format";
import {
  settingsErrorMessage,
  type SettingsNotice,
  useSettingsList,
  useSettingsMutation
} from "../query-state";
import { refreshAlertSourceProfiles, submitAlertSourceProfile, testAlertSourceConnection } from "./client-api";
import {
  emptyAlertSourceForm,
  formStateToWriteRequest,
  profileToFormState
} from "./format";
import type {
  AlertSourceConnectionTestResult,
  AlertSourceFormState,
  AlertSourceProfile,
  AlertSourceProfileListResponse,
  AlertSourceProfileWriteRequest
} from "./types";

type AlertSourceSettingsManagerProps = {
  result: ApiResult<AlertSourceProfileListResponse>;
};

const alertSourceProfilesQueryKey = ["settings", "alert-sources"] as const;

type SaveProfileVariables = {
  body: AlertSourceProfileWriteRequest;
  sourceID: number | null;
};

export function AlertSourceSettingsManager({ result }: AlertSourceSettingsManagerProps) {
  const [form] = Form.useForm<AlertSourceFormState>();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [testingID, setTestingID] = useState<number | null>(null);
  const [testResults, setTestResults] = useState<Record<number, AlertSourceConnectionTestResult>>({});
  const {
    items: profiles,
    notice,
    query,
    refresh,
    setNotice
  } = useSettingsList({
    initialResult: result,
    queryKey: alertSourceProfilesQueryKey,
    queryFn: refreshAlertSourceProfiles,
    refreshMessage: "Profiles refreshed.",
    selectItems: (response) => response.items
  });
  const saveProfile = useSettingsMutation<SaveProfileVariables, AlertSourceProfile>({
    invalidateQueryKey: alertSourceProfilesQueryKey,
    mutationFn: ({ sourceID, body }) => submitAlertSourceProfile(sourceID, body)
  });
  const busy = query.isFetching || saveProfile.isPending;
  const authMode = Form.useWatch("authMode", form) ?? "none";

  const summary = useMemo(() => {
    const enabled = profiles.filter((profile) => profile.enabled).length;
    const prometheus = profiles.filter((profile) => profile.kind === "prometheus").length;
    const alertmanager = profiles.filter((profile) => profile.kind === "alertmanager").length;
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
      await saveProfile.mutateAsync({ sourceID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }

    form.setFieldsValue(emptyAlertSourceForm());
    setEditingID(null);
    setNotice({ kind: "info", message: "Profile saved." });
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
    setNotice({ kind: noticeKindForConnectionStatus(tested.data.status), message: tested.data.message });
  }

  function editProfile(profile: AlertSourceProfile) {
    setEditingID(profile.id);
    form.setFieldsValue(profileToFormState(profile));
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyAlertSourceForm());
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
            title={editingID === null ? "New Alert Source" : `Edit Source #${editingID}`}
          >
            <Form<AlertSourceFormState>
              disabled={busy}
              form={form}
              initialValues={emptyAlertSourceForm()}
              layout="vertical"
              onFinish={handleSubmit}
              onValuesChange={(changed: Partial<AlertSourceFormState>) => {
                if (changed.authMode === "none") {
                  form.setFieldValue("secretRef", "");
                }
              }}
            >
              <Form.Item
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Profile name is required." },
                  { max: 120, message: "Profile name must be 120 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item label="Kind" name="kind" rules={[{ required: true, message: "Kind is required." }]}>
                    <Segmented
                      block
                      options={[
                        { value: "prometheus", label: "Prometheus" },
                        { value: "alertmanager", label: "Alertmanager" }
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item label="Auth" name="authMode" rules={[{ required: true, message: "Auth mode is required." }]}>
                    <Segmented
                      block
                      options={[
                        { value: "none", label: "None" },
                        { value: "bearer", label: "Bearer" }
                      ]}
                    />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item
                label="Base URL"
                name="baseURL"
                rules={[
                  { required: true, message: "Base URL is required." },
                  { type: "url", message: "Base URL must be a valid URL." }
                ]}
              >
                <Input autoComplete="url" />
              </Form.Item>

              <Form.Item
                label="Secret reference"
                name="secretRef"
                rules={[
                  ...(authMode === "bearer"
                    ? [{ required: true, message: "Bearer auth requires a secret reference." }]
                    : []),
                  { pattern: /^\S*$/, message: "Secret reference must not contain whitespace." }
                ]}
              >
                <Input autoComplete="off" disabled={authMode === "none"} />
              </Form.Item>

              <Form.Item label="Labels" name="labelsText">
                <Input.TextArea autoSize={{ minRows: 4, maxRows: 8 }} placeholder={"env=prod\nowner=platform"} />
              </Form.Item>

              <Form.Item name="enabled" valuePropName="checked">
                <Checkbox>Enabled</Checkbox>
              </Form.Item>

              <Space wrap>
                <Button htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
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
              <Button disabled={busy} icon={<ReloadOutlined />} loading={busy} onClick={handleRefresh} type="default">
                Refresh
              </Button>
            }
            title="Configured Sources"
          >
            <AlertSourceTable
              busy={busy}
              onEdit={editProfile}
              onTest={handleTest}
              profiles={profiles}
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
      message={notice.kind === "error" ? "Request failed" : notice.kind === "warning" ? "Connection review" : "Settings"}
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={type}
    />
  );
}

function AlertSourceTable({
  profiles,
  busy,
  onEdit,
  onTest,
  testResults,
  testingID
}: {
  profiles: AlertSourceProfile[];
  busy: boolean;
  onEdit: (profile: AlertSourceProfile) => void;
  onTest: (profile: AlertSourceProfile) => void;
  testResults: Record<number, AlertSourceConnectionTestResult>;
  testingID: number | null;
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
      )
    },
    {
      dataIndex: "kind",
      key: "kind",
      title: "Kind",
      render: (_value, profile) => <Tag color={profile.kind === "alertmanager" ? "red" : "blue"}>{profile.kind}</Tag>
    },
    {
      dataIndex: "base_url",
      key: "base_url",
      title: "Endpoint",
      render: (_value, profile) => (
        <Typography.Text className="settings-url-cell">{profile.base_url}</Typography.Text>
      )
    },
    {
      dataIndex: "auth_mode",
      key: "auth",
      title: "Auth",
      render: (_value, profile) => (
        <Space direction="vertical" size={2}>
          <Tag color={profile.auth_mode === "bearer" ? "gold" : "green"}>{profile.auth_mode}</Tag>
          {profile.secret_ref === "" ? null : (
            <Typography.Text className="settings-secret-ref" type="secondary">
              {profile.secret_ref}
            </Typography.Text>
          )}
        </Space>
      )
    },
    {
      dataIndex: "enabled",
      key: "state",
      title: "State",
      render: (_value, profile) => (
        <Tag color={profile.enabled ? "green" : "default"}>{profile.enabled ? "enabled" : "disabled"}</Tag>
      )
    },
    {
      dataIndex: "labels",
      key: "labels",
      title: "Labels",
      render: (_value, profile) => <Labels labels={profile.labels} />
    },
    {
      key: "last_test",
      title: "Last Test",
      render: (_value, profile) => <ConnectionTestResult result={testResults[profile.id]} />
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: "Updated",
      render: (_value, profile) => formatDateTime(profile.updated_at)
    },
    {
      key: "action",
      title: "Action",
      render: (_value, profile) => (
        <Space wrap>
          <Button
            disabled={busy || (testingID !== null && testingID !== profile.id)}
            icon={<ApiOutlined />}
            loading={testingID === profile.id}
            onClick={() => onTest(profile)}
            type="link"
          >
            Test
          </Button>
          <Button disabled={busy || testingID !== null} icon={<EditOutlined />} onClick={() => onEdit(profile)} type="link">
            Edit
          </Button>
        </Space>
      )
    }
  ];

  return (
    <Table<AlertSourceProfile>
      columns={columns}
      dataSource={profiles}
      loading={busy}
      locale={{ emptyText: <Empty description="No alert sources configured." /> }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1120 }}
    />
  );
}

function ConnectionTestResult({ result }: { result?: AlertSourceConnectionTestResult }) {
  if (!result) {
    return <Typography.Text type="secondary">Not tested</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Tag color={connectionStatusColor(result.status)}>{result.status}</Tag>
      <Typography.Text type="secondary">{result.reason_code}</Typography.Text>
      {result.status === "success" ? (
        <Typography.Text type="secondary">{result.observed_alerts} firing alerts</Typography.Text>
      ) : null}
    </Space>
  );
}

function Labels({ labels }: { labels: AlertSourceProfile["labels"] }) {
  const entries = Object.entries(labels).sort(([left], [right]) => left.localeCompare(right));
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

function normalizeFormValues(values: AlertSourceFormState): AlertSourceFormState {
  return {
    ...emptyAlertSourceForm(),
    ...values,
    enabled: Boolean(values.enabled),
    secretRef: values.secretRef ?? "",
    labelsText: values.labelsText ?? ""
  };
}

function connectionStatusColor(status: AlertSourceConnectionTestResult["status"]) {
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

function noticeKindForConnectionStatus(status: AlertSourceConnectionTestResult["status"]): SettingsNotice["kind"] {
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
