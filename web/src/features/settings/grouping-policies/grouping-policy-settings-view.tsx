"use client";

import {
  EditOutlined,
  PartitionOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined
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
import { refreshGroupingPolicies, runGroupingPolicyPreview, submitGroupingPolicy } from "./client-api";
import {
  emptyGroupingPolicyForm,
  formStateToWriteRequest,
  policyToFormState
} from "./format";
import type {
  GroupingPolicy,
  GroupingPolicyFormState,
  GroupingPolicyListResponse,
  GroupingPolicyPreviewGroup,
  GroupingPolicyPreviewResult
} from "./types";

type GroupingPolicySettingsManagerProps = {
  result: ApiResult<GroupingPolicyListResponse>;
};

type NoticeState = {
  kind: "info" | "error";
  message: string;
};

export function GroupingPolicySettingsManager({ result }: GroupingPolicySettingsManagerProps) {
  const [form] = Form.useForm<GroupingPolicyFormState>();
  const [policies, setPolicies] = useState<GroupingPolicy[]>(result.ok ? result.data.items : []);
  const [editingID, setEditingID] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [previewingID, setPreviewingID] = useState<number | null>(null);
  const [previewResults, setPreviewResults] = useState<Record<number, GroupingPolicyPreviewResult>>({});
  const [selectedPreviewID, setSelectedPreviewID] = useState<number | null>(null);
  const [notice, setNotice] = useState<NoticeState | null>(
    result.ok ? null : { kind: "error", message: result.error.message }
  );

  const summary = useMemo(() => {
    const enabled = policies.filter((policy) => policy.enabled).length;
    const scoped = policies.filter((policy) => policy.source_filter.length > 0).length;
    const maxDimensions = policies.reduce((current, policy) => Math.max(current, policy.dimension_keys.length), 0);
    return { enabled, scoped, maxDimensions };
  }, [policies]);

  async function handleRefresh() {
    setBusy(true);
    const refreshed = await refreshGroupingPolicies();
    setBusy(false);
    if (!refreshed.ok) {
      setNotice({ kind: "error", message: refreshed.error.message });
      return;
    }
    setPolicies(refreshed.data.items);
    setNotice({ kind: "info", message: "Policies refreshed." });
  }

  async function handleSubmit(values: GroupingPolicyFormState) {
    const parsed = formStateToWriteRequest(normalizeFormValues(values));
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }

    setBusy(true);
    const saved = await submitGroupingPolicy(editingID, parsed.value);
    setBusy(false);
    if (!saved.ok) {
      setNotice({ kind: "error", message: saved.error.message });
      return;
    }

    setPolicies((current) => upsertPolicy(current, saved.data));
    form.setFieldsValue(emptyGroupingPolicyForm());
    setEditingID(null);
    setNotice({ kind: "info", message: "Policy saved." });
  }

  async function handlePreview(policy: GroupingPolicy) {
    setPreviewingID(policy.id);
    const previewed = await runGroupingPolicyPreview(policy.id);
    setPreviewingID(null);
    if (!previewed.ok) {
      setNotice({ kind: "error", message: previewed.error.message });
      return;
    }

    setPreviewResults((current) => ({ ...current, [policy.id]: previewed.data }));
    setSelectedPreviewID(policy.id);
    setNotice({
      kind: "info",
      message: `Preview scanned ${previewed.data.events_scanned} events and matched ${previewed.data.events_matched}.`
    });
  }

  function editPolicy(policy: GroupingPolicy) {
    setEditingID(policy.id);
    form.setFieldsValue(policyToFormState(policy));
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyGroupingPolicyForm());
    setNotice(null);
  }

  const selectedPreview = selectedPreviewID === null ? undefined : previewResults[selectedPreviewID];

  return (
    <div className="stack">
      <Row aria-label="Grouping policy metrics" gutter={[12, 12]}>
        <MetricCard label="Policies" value={policies.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Source scoped" value={summary.scoped} />
        <MetricCard label="Max dimensions" value={summary.maxDimensions} />
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
            title={editingID === null ? "New Grouping Policy" : `Edit Policy #${editingID}`}
          >
            <Form<GroupingPolicyFormState>
              disabled={busy}
              form={form}
              initialValues={emptyGroupingPolicyForm()}
              layout="vertical"
              onFinish={handleSubmit}
            >
              <Form.Item
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Policy name is required." },
                  { max: 120, message: "Policy name must be 120 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item
                label="Dimension keys"
                name="dimensionKeysText"
                rules={[{ required: true, message: "At least one dimension key is required." }]}
              >
                <Input.TextArea autoSize={{ minRows: 4, maxRows: 8 }} placeholder={"alertname\nservice"} />
              </Form.Item>

              <Form.Item
                label="Severity key"
                name="severityKey"
                rules={[
                  { required: true, message: "Severity key is required." },
                  { max: 64, message: "Severity key must be 64 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item label="Source filter" name="sourceFilterText">
                <Input.TextArea autoSize={{ minRows: 3, maxRows: 6 }} placeholder={"prometheus\nalertmanager"} />
              </Form.Item>

              <Form.Item name="enabled" valuePropName="checked">
                <Checkbox>Enabled</Checkbox>
              </Form.Item>

              <Space wrap>
                <Button htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
                  Save Policy
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
            title="Configured Policies"
          >
            <GroupingPolicyTable
              busy={busy}
              onEdit={editPolicy}
              onPreview={handlePreview}
              policies={policies}
              previewingID={previewingID}
              previewResults={previewResults}
            />
            <PreviewPanel result={selectedPreview} />
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

function Notice({ notice }: { notice: NoticeState }) {
  return (
    <Alert
      description={notice.message}
      message={notice.kind === "error" ? "Request failed" : "Settings"}
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={notice.kind === "error" ? "error" : "success"}
    />
  );
}

function GroupingPolicyTable({
  policies,
  busy,
  onEdit,
  onPreview,
  previewResults,
  previewingID
}: {
  policies: GroupingPolicy[];
  busy: boolean;
  onEdit: (policy: GroupingPolicy) => void;
  onPreview: (policy: GroupingPolicy) => void;
  previewResults: Record<number, GroupingPolicyPreviewResult>;
  previewingID: number | null;
}) {
  const columns: TableColumnsType<GroupingPolicy> = [
    {
      dataIndex: "name",
      key: "name",
      title: "Name",
      render: (_value, policy) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>{policy.name}</Typography.Text>
          <Typography.Text type="secondary">#{policy.id}</Typography.Text>
        </Space>
      )
    },
    {
      dataIndex: "dimension_keys",
      key: "dimensions",
      title: "Dimensions",
      render: (_value, policy) => <TokenList emptyText="None" values={policy.dimension_keys} />
    },
    {
      dataIndex: "severity_key",
      key: "severity",
      title: "Severity",
      render: (_value, policy) => <Tag color="gold">{policy.severity_key}</Tag>
    },
    {
      dataIndex: "source_filter",
      key: "source_filter",
      title: "Sources",
      render: (_value, policy) => <TokenList emptyText="All sources" values={policy.source_filter} />
    },
    {
      dataIndex: "enabled",
      key: "state",
      title: "State",
      render: (_value, policy) => (
        <Tag color={policy.enabled ? "green" : "default"}>{policy.enabled ? "enabled" : "disabled"}</Tag>
      )
    },
    {
      key: "last_preview",
      title: "Last Preview",
      render: (_value, policy) => <PreviewSummary result={previewResults[policy.id]} />
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: "Updated",
      render: (_value, policy) => formatDateTime(policy.updated_at)
    },
    {
      key: "action",
      title: "Action",
      render: (_value, policy) => (
        <Space wrap>
          <Button
            disabled={busy || (previewingID !== null && previewingID !== policy.id)}
            icon={<PlayCircleOutlined />}
            loading={previewingID === policy.id}
            onClick={() => onPreview(policy)}
            type="link"
          >
            Preview
          </Button>
          <Button disabled={busy || previewingID !== null} icon={<EditOutlined />} onClick={() => onEdit(policy)} type="link">
            Edit
          </Button>
        </Space>
      )
    }
  ];

  return (
    <Table<GroupingPolicy>
      columns={columns}
      dataSource={policies}
      loading={busy}
      locale={{ emptyText: <Empty description="No grouping policies configured." /> }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1120 }}
    />
  );
}

function PreviewSummary({ result }: { result?: GroupingPolicyPreviewResult }) {
  if (!result) {
    return <Typography.Text type="secondary">Not previewed</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Typography.Text>{result.groups.length} groups</Typography.Text>
      <Typography.Text type="secondary">
        {result.events_matched}/{result.events_scanned} events
      </Typography.Text>
    </Space>
  );
}

function PreviewPanel({ result }: { result?: GroupingPolicyPreviewResult }) {
  if (!result) {
    return null;
  }
  return (
    <div className="settings-preview-panel">
      <div className="settings-preview-header">
        <Space align="center">
          <PartitionOutlined />
          <Typography.Text strong>Latest Preview</Typography.Text>
        </Space>
        <Typography.Text type="secondary">
          {result.events_matched}/{result.events_scanned} events matched
        </Typography.Text>
      </div>
      <Table<GroupingPolicyPreviewGroup>
        columns={previewColumns}
        dataSource={result.groups}
        locale={{ emptyText: <Empty description="No preview groups." /> }}
        pagination={false}
        rowKey={(group) => group.group_key}
        scroll={{ x: 940 }}
        size="small"
      />
    </div>
  );
}

const previewColumns: TableColumnsType<GroupingPolicyPreviewGroup> = [
  {
    dataIndex: "dimensions",
    key: "dimensions",
    title: "Dimensions",
    render: (_value, group) => <DimensionTags values={group.dimensions} />
  },
  {
    dataIndex: "severity",
    key: "severity",
    title: "Severity",
    render: (_value, group) => <Tag color={severityColor(group.severity)}>{group.severity}</Tag>
  },
  {
    dataIndex: "event_count",
    key: "event_count",
    title: "Events"
  },
  {
    dataIndex: "first_seen_at",
    key: "first_seen_at",
    title: "First Seen",
    render: (_value, group) => formatDateTime(group.first_seen_at)
  },
  {
    dataIndex: "last_seen_at",
    key: "last_seen_at",
    title: "Last Seen",
    render: (_value, group) => formatDateTime(group.last_seen_at)
  },
  {
    dataIndex: "event_ids",
    key: "event_ids",
    title: "Event IDs",
    render: (_value, group) => (
      <Typography.Text className="settings-event-ids">{group.event_ids.join(", ")}</Typography.Text>
    )
  }
];

function TokenList({ values, emptyText }: { values: string[]; emptyText: string }) {
  if (values.length === 0) {
    return <Typography.Text type="secondary">{emptyText}</Typography.Text>;
  }
  return (
    <div className="label-stack">
      {values.map((value) => (
        <Tag key={value}>{value}</Tag>
      ))}
    </div>
  );
}

function DimensionTags({ values }: { values: Record<string, string> }) {
  const entries = Object.entries(values).sort(([left], [right]) => left.localeCompare(right));
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

function upsertPolicy(current: GroupingPolicy[], saved: GroupingPolicy): GroupingPolicy[] {
  const next = [saved, ...current.filter((policy) => policy.id !== saved.id)];
  return next.sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at));
}

function normalizeFormValues(values: GroupingPolicyFormState): GroupingPolicyFormState {
  return {
    ...emptyGroupingPolicyForm(),
    ...values,
    enabled: Boolean(values.enabled),
    dimensionKeysText: values.dimensionKeysText ?? "",
    severityKey: values.severityKey ?? "",
    sourceFilterText: values.sourceFilterText ?? ""
  };
}

function severityColor(severity: GroupingPolicyPreviewGroup["severity"]) {
  switch (severity) {
    case "critical":
      return "red";
    case "warning":
      return "gold";
    case "info":
      return "blue";
    case "unknown":
      return "default";
  }
}
