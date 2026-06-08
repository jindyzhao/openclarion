"use client";

import {
  BranchesOutlined,
  EditOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  RadarChartOutlined,
  ReloadOutlined,
  SaveOutlined,
  ThunderboltOutlined
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
  Modal,
  Row,
  Segmented,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography
} from "antd";
import type { FormInstance, TableColumnsType } from "antd";
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
  disableReportWorkflowPolicyAction,
  enableReportWorkflowPolicyAction,
  previewReportWorkflowPolicyImpactAction,
  refreshReportWorkflowPolicies,
  submitReportWorkflowPolicy,
  triggerReportWorkflowPolicyReplayAction
} from "./client-api";
import {
  defaultReportWorkflowPolicyReplayForm,
  emptyReportWorkflowPolicyForm,
  formStateToReplayRequest,
  formStateToWriteRequest,
  policyToFormState
} from "./format";
import type {
  ReportReplayTriggerResponse,
  ReportWorkflowPolicy,
  ReportWorkflowPolicyFormState,
  ReportWorkflowPolicyImpactPreviewResult,
  ReportWorkflowPolicyListResponse,
  ReportWorkflowPolicyReplayFormState,
  ReportWorkflowPolicyReplayRequest,
  ReportWorkflowPolicyWriteRequest
} from "./types";

type ReportWorkflowPolicySettingsManagerProps = {
  result: ApiResult<ReportWorkflowPolicyListResponse>;
};

const reportWorkflowPoliciesQueryKey = ["settings", "report-workflow-policies"] as const;

type SavePolicyVariables = {
  body: ReportWorkflowPolicyWriteRequest;
  policyID: number | null;
};

type EnablementVariables = {
  enabled: boolean;
  policyID: number;
};

type ReplayVariables = {
  body: ReportWorkflowPolicyReplayRequest;
  policyID: number;
};

type ImpactPreviewGroup = ReportWorkflowPolicyImpactPreviewResult["groups"][number];

export function ReportWorkflowPolicySettingsManager({ result }: ReportWorkflowPolicySettingsManagerProps) {
  const [form] = Form.useForm<ReportWorkflowPolicyFormState>();
  const [replayForm] = Form.useForm<ReportWorkflowPolicyReplayFormState>();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [actionID, setActionID] = useState<number | null>(null);
  const [impactingID, setImpactingID] = useState<number | null>(null);
  const [impactPolicy, setImpactPolicy] = useState<ReportWorkflowPolicy | null>(null);
  const [impactResults, setImpactResults] = useState<Record<number, ReportWorkflowPolicyImpactPreviewResult>>({});
  const [replayPolicy, setReplayPolicy] = useState<ReportWorkflowPolicy | null>(null);
  const [replayResult, setReplayResult] = useState<ReportReplayTriggerResponse | null>(null);
  const {
    items: policies,
    notice,
    query,
    refresh,
    setNotice
  } = useSettingsList({
    initialResult: result,
    queryKey: reportWorkflowPoliciesQueryKey,
    queryFn: refreshReportWorkflowPolicies,
    refreshMessage: "Policies refreshed.",
    selectItems: (response) => response.items
  });
  const savePolicy = useSettingsMutation<SavePolicyVariables, ReportWorkflowPolicy>({
    invalidateQueryKey: reportWorkflowPoliciesQueryKey,
    mutationFn: ({ policyID, body }) => submitReportWorkflowPolicy(policyID, body)
  });
  const enablementAction = useSettingsMutation<EnablementVariables, ReportWorkflowPolicy>({
    invalidateQueryKey: reportWorkflowPoliciesQueryKey,
    mutationFn: ({ policyID, enabled }) =>
      enabled ? enableReportWorkflowPolicyAction(policyID) : disableReportWorkflowPolicyAction(policyID)
  });
  const replayAction = useSettingsMutation<ReplayVariables, ReportReplayTriggerResponse>({
    invalidateQueryKey: reportWorkflowPoliciesQueryKey,
    mutationFn: ({ policyID, body }) => triggerReportWorkflowPolicyReplayAction(policyID, body)
  });
  const busy =
    query.isFetching ||
    savePolicy.isPending ||
    enablementAction.isPending ||
    replayAction.isPending ||
    impactingID !== null;

  const summary = useMemo(() => {
    const enabled = policies.filter((policy) => policy.enabled).length;
    const reportChannel = policies.filter((policy) => policy.report_notification_channel_profile_id !== null).length;
    const roomFollowUp = policies.filter((policy) => policy.diagnosis_follow_up === "suggest_room").length;
    return { enabled, reportChannel, roomFollowUp };
  }, [policies]);

  async function handleRefresh() {
    await refresh();
  }

  async function handleSubmit(values: ReportWorkflowPolicyFormState) {
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }

    try {
      await savePolicy.mutateAsync({ policyID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }

    form.setFieldsValue(emptyReportWorkflowPolicyForm());
    setEditingID(null);
    setNotice({ kind: "info", message: "Policy saved." });
  }

  async function handleEnablement(policy: ReportWorkflowPolicy, enabled: boolean) {
    setActionID(policy.id);
    try {
      await enablementAction.mutateAsync({ policyID: policy.id, enabled });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      setActionID(null);
      return;
    }
    setActionID(null);
    setNotice({ kind: enabled ? "info" : "warning", message: enabled ? "Policy enabled." : "Policy disabled." });
  }

  function editPolicy(policy: ReportWorkflowPolicy) {
    setEditingID(policy.id);
    form.setFieldsValue(policyToFormState(policy));
    setNotice(null);
  }

  function openReplay(policy: ReportWorkflowPolicy) {
    setReplayPolicy(policy);
    setReplayResult(null);
    replayForm.setFieldsValue(defaultReportWorkflowPolicyReplayForm());
    setNotice(null);
  }

  function closeReplay() {
    if (replayAction.isPending) {
      return;
    }
    setReplayPolicy(null);
    setReplayResult(null);
    replayForm.resetFields();
  }

  function closeImpactPreview() {
    setImpactPolicy(null);
  }

  async function handleImpactPreview(policy: ReportWorkflowPolicy) {
    setImpactingID(policy.id);
    const previewed = await previewReportWorkflowPolicyImpactAction(policy.id);
    setImpactingID(null);
    if (!previewed.ok) {
      setNotice({ kind: "error", message: previewed.error.message });
      return;
    }
    setImpactResults((current) => ({ ...current, [policy.id]: previewed.data }));
    setImpactPolicy(policy);
    setNotice({
      kind: previewed.data.status === "blocked" ? "warning" : "info",
      message: `Impact preview ${previewed.data.status}: ${previewed.data.groups_estimated} groups from ${previewed.data.events_matched} matching events.`
    });
  }

  async function handleReplay(values: ReportWorkflowPolicyReplayFormState) {
    if (replayPolicy === null) {
      return;
    }
    const parsed = formStateToReplayRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }
    try {
      const replayed = await replayAction.mutateAsync({ policyID: replayPolicy.id, body: parsed.value });
      setReplayResult(replayed);
      setNotice({
        kind: replayed.started ? "info" : "warning",
        message: replayed.started ? "Replay accepted." : "Replay completed without report snapshots."
      });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
    }
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyReportWorkflowPolicyForm());
    setNotice(null);
  }

  return (
    <div className="stack">
      <Row aria-label="Report workflow policy metrics" gutter={[12, 12]}>
        <MetricCard label="Policies" value={policies.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Report channel" value={summary.reportChannel} />
        <MetricCard label="Room follow-up" value={summary.roomFollowUp} />
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
            title={editingID === null ? "New Workflow Policy" : `Edit Policy #${editingID}`}
          >
            <Form<ReportWorkflowPolicyFormState>
              disabled={busy}
              form={form}
              initialValues={emptyReportWorkflowPolicyForm()}
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

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Alert source ID"
                    name="alertSourceProfileID"
                    rules={[{ required: true, message: "Alert source profile ID is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Grouping policy ID"
                    name="groupingPolicyID"
                    rules={[{ required: true, message: "Grouping policy ID is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item label="Report channel ID" name="reportNotificationChannelProfileID">
                <InputNumber min={1} precision={0} style={{ width: "100%" }} />
              </Form.Item>

              <Form.Item label="Trigger" name="triggerMode" rules={[{ required: true, message: "Trigger is required." }]}>
                <Segmented block options={[{ value: "manual_replay", label: "Manual replay" }]} />
              </Form.Item>

              <Form.Item
                label="Scenario"
                name="reportScenario"
                rules={[{ required: true, message: "Scenario is required." }]}
              >
                <Select
                  options={[
                    { value: "single_alert", label: "Single alert" },
                    { value: "cascade", label: "Cascade" },
                    { value: "alert_storm", label: "Alert storm" }
                  ]}
                />
              </Form.Item>

              <Form.Item
                label="Diagnosis follow-up"
                name="diagnosisFollowUp"
                rules={[{ required: true, message: "Diagnosis follow-up is required." }]}
              >
                <Segmented
                  block
                  options={[
                    { value: "disabled", label: "Disabled" },
                    { value: "suggest_room", label: "Suggest room" }
                  ]}
                />
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
            <ReportWorkflowPolicyTable
              actionID={actionID}
              busy={busy}
              onDisable={(policy) => handleEnablement(policy, false)}
              onEdit={editPolicy}
              onEnable={(policy) => handleEnablement(policy, true)}
              onImpactPreview={handleImpactPreview}
              onReplay={openReplay}
              impactResults={impactResults}
              impactingID={impactingID}
              policies={policies}
            />
          </Card>
        </Col>
      </Row>

      <ReplayPolicyModal
        busy={replayAction.isPending}
        form={replayForm}
        onCancel={closeReplay}
        onSubmit={handleReplay}
        policy={replayPolicy}
        result={replayResult}
      />
      <ImpactPreviewModal
        onCancel={closeImpactPreview}
        policy={impactPolicy}
        result={impactPolicy === null ? null : impactResults[impactPolicy.id] ?? null}
      />
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

type ReportWorkflowPolicyTableProps = {
  actionID: number | null;
  busy: boolean;
  impactResults: Record<number, ReportWorkflowPolicyImpactPreviewResult>;
  impactingID: number | null;
  onDisable: (policy: ReportWorkflowPolicy) => void;
  onEdit: (policy: ReportWorkflowPolicy) => void;
  onEnable: (policy: ReportWorkflowPolicy) => void;
  onImpactPreview: (policy: ReportWorkflowPolicy) => void;
  onReplay: (policy: ReportWorkflowPolicy) => void;
  policies: ReportWorkflowPolicy[];
};

function ReportWorkflowPolicyTable({
  actionID,
  busy,
  impactResults,
  impactingID,
  onDisable,
  onEdit,
  onEnable,
  onImpactPreview,
  onReplay,
  policies
}: ReportWorkflowPolicyTableProps) {
  const columns: TableColumnsType<ReportWorkflowPolicy> = [
    {
      key: "name",
      title: "Name",
      render: (_, policy) => (
        <Space direction="vertical" size={2}>
          <Typography.Text strong>{policy.name}</Typography.Text>
          <Typography.Text type="secondary">
            Source #{policy.alert_source_profile_id} / Grouping #{policy.grouping_policy_id}
          </Typography.Text>
        </Space>
      )
    },
    {
      dataIndex: "report_scenario",
      key: "scenario",
      title: "Scenario",
      render: (scenario: ReportWorkflowPolicy["report_scenario"]) => <Tag>{scenario}</Tag>
    },
    {
      dataIndex: "diagnosis_follow_up",
      key: "followup",
      title: "Follow-up",
      render: (mode: ReportWorkflowPolicy["diagnosis_follow_up"]) => (
        <Tag color={mode === "suggest_room" ? "blue" : "default"}>{mode}</Tag>
      )
    },
    {
      dataIndex: "report_notification_channel_profile_id",
      key: "reportChannel",
      title: "Report channel",
      render: (profileID: ReportWorkflowPolicy["report_notification_channel_profile_id"]) =>
        profileID === null ? <Tag>None</Tag> : <Tag color="geekblue">#{profileID}</Tag>
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: "State",
      render: (enabled: boolean, policy) => (
        <Space direction="vertical" size={2}>
          <Tag color={enabled ? "green" : "default"}>{enabled ? "Enabled" : "Draft"}</Tag>
          <Typography.Text type="secondary">
            {enabled ? nullableDate(policy.enabled_at) : nullableDate(policy.disabled_at)}
          </Typography.Text>
        </Space>
      )
    },
    {
      key: "impact",
      title: "Impact",
      render: (_, policy) => <ImpactSummary result={impactResults[policy.id]} />
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: "Updated",
      render: (value: string) => formatDateTime(value)
    },
    {
      key: "actions",
      render: (_, policy) => (
        <Space wrap>
          <Button disabled={busy || actionID !== null} icon={<EditOutlined />} onClick={() => onEdit(policy)} size="small">
            Edit
          </Button>
          <Button
            disabled={busy || actionID !== null || !policy.enabled}
            icon={<ThunderboltOutlined />}
            onClick={() => onReplay(policy)}
            size="small"
          >
            Replay
          </Button>
          <Button
            disabled={busy || (impactingID !== null && impactingID !== policy.id)}
            icon={<RadarChartOutlined />}
            loading={impactingID === policy.id}
            onClick={() => onImpactPreview(policy)}
            size="small"
          >
            Impact
          </Button>
          {policy.enabled ? (
            <Button
              disabled={busy || actionID !== null}
              icon={<PauseCircleOutlined />}
              loading={actionID === policy.id}
              onClick={() => onDisable(policy)}
              size="small"
            >
              Disable
            </Button>
          ) : (
            <Button
              disabled={busy || actionID !== null}
              icon={<PlayCircleOutlined />}
              loading={actionID === policy.id}
              onClick={() => onEnable(policy)}
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
    <Table<ReportWorkflowPolicy>
      columns={columns}
      dataSource={policies}
      loading={busy}
      locale={{
        emptyText: (
          <Empty
            description="No report workflow policies"
            image={<BranchesOutlined aria-hidden className="settings-empty-icon" />}
          />
        )
      }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1040 }}
    />
  );
}

function ImpactSummary({ result }: { result?: ReportWorkflowPolicyImpactPreviewResult }) {
  if (!result) {
    return <Typography.Text type="secondary">Not previewed</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Tag color={impactStatusColor(result.status)}>{result.status}</Tag>
      <Typography.Text type="secondary">
        {result.groups_estimated} groups / {result.events_matched} events
      </Typography.Text>
    </Space>
  );
}

type ImpactPreviewModalProps = {
  onCancel: () => void;
  policy: ReportWorkflowPolicy | null;
  result: ReportWorkflowPolicyImpactPreviewResult | null;
};

function ImpactPreviewModal({ onCancel, policy, result }: ImpactPreviewModalProps) {
  return (
    <Modal
      destroyOnHidden
      footer={
        <Button icon={<RadarChartOutlined />} onClick={onCancel} type="primary">
          Close
        </Button>
      }
      onCancel={onCancel}
      open={policy !== null}
      title={policy === null ? "Impact Preview" : `Impact Preview #${policy.id}`}
      width={920}
    >
      {result === null ? null : (
        <Space className="settings-impact-preview" direction="vertical" size="middle">
          <Alert
            description={
              <Space direction="vertical" size={4}>
                <Typography.Text>{result.message}</Typography.Text>
                <Space wrap>
                  {result.reason_codes.map((reason) => (
                    <Tag key={reason}>{reason}</Tag>
                  ))}
                </Space>
              </Space>
            }
            message={<Tag color={impactStatusColor(result.status)}>{result.status}</Tag>}
            showIcon
            type={impactAlertType(result.status)}
          />

          <Row gutter={[12, 12]}>
            <Col sm={8} xs={24}>
              <Statistic title="Events scanned" value={result.events_scanned} />
            </Col>
            <Col sm={8} xs={24}>
              <Statistic title="Events matched" value={result.events_matched} />
            </Col>
            <Col sm={8} xs={24}>
              <Statistic title="Groups estimated" value={result.groups_estimated} />
            </Col>
          </Row>

          <Row gutter={[12, 12]}>
            <Col md={8} xs={24}>
              <ReadinessLine
                label="Alert source"
                ready={result.alert_source_enabled}
                text={`#${result.alert_source_profile_id} ${result.alert_source_kind}/${result.alert_source_auth_mode}`}
              />
            </Col>
            <Col md={8} xs={24}>
              <ReadinessLine
                label="Grouping policy"
                ready={result.grouping_policy_enabled}
                text={`#${result.grouping_policy_id} ${result.grouping_dimension_keys.join(", ")}`}
              />
            </Col>
            <Col md={8} xs={24}>
              <ReadinessLine
                label="Report channel"
                ready={
                  !result.report_notification_channel_bound ||
                  (result.report_notification_channel_enabled &&
                    result.report_notification_channel_has_report_scope)
                }
                text={reportChannelImpactText(result)}
              />
            </Col>
          </Row>

          <Table<ImpactPreviewGroup>
            columns={impactGroupColumns}
            dataSource={result.groups}
            locale={{ emptyText: <Empty description="No estimated groups." /> }}
            pagination={false}
            rowKey={(group) => group.group_key}
            scroll={{ x: 940 }}
            size="small"
          />
        </Space>
      )}
    </Modal>
  );
}

function ReadinessLine({ label, ready, text }: { label: string; ready: boolean; text: string }) {
  return (
    <Space direction="vertical" size={2}>
      <Typography.Text type="secondary">{label}</Typography.Text>
      <Space wrap>
        <Tag color={ready ? "green" : "red"}>{ready ? "ready" : "blocked"}</Tag>
        <Typography.Text>{text}</Typography.Text>
      </Space>
    </Space>
  );
}

const impactGroupColumns: TableColumnsType<ImpactPreviewGroup> = [
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

type ReplayPolicyModalProps = {
  busy: boolean;
  form: FormInstance<ReportWorkflowPolicyReplayFormState>;
  onCancel: () => void;
  onSubmit: (values: ReportWorkflowPolicyReplayFormState) => void;
  policy: ReportWorkflowPolicy | null;
  result: ReportReplayTriggerResponse | null;
};

function ReplayPolicyModal({ busy, form, onCancel, onSubmit, policy, result }: ReplayPolicyModalProps) {
  return (
    <Modal
      destroyOnHidden
      footer={null}
      onCancel={onCancel}
      open={policy !== null}
      title={policy === null ? "Replay Policy" : `Replay Policy #${policy.id}`}
    >
      <Form<ReportWorkflowPolicyReplayFormState> disabled={busy} form={form} layout="vertical" onFinish={onSubmit}>
        <Row gutter={12}>
          <Col sm={12} xs={24}>
            <Form.Item
              label="Window start"
              name="windowStart"
              rules={[{ required: true, message: "Window start is required." }]}
            >
              <Input autoComplete="off" placeholder="2026-06-05T08:00:00Z" />
            </Form.Item>
          </Col>
          <Col sm={12} xs={24}>
            <Form.Item
              label="Window end"
              name="windowEnd"
              rules={[{ required: true, message: "Window end is required." }]}
            >
              <Input autoComplete="off" placeholder="2026-06-05T09:00:00Z" />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item label="Limit" name="limit" rules={[{ required: true, message: "Limit is required." }]}>
          <InputNumber max={100000} min={1} precision={0} style={{ width: "100%" }} />
        </Form.Item>
        <Form.Item label="Correlation key" name="correlationKey">
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item label="Workflow ID" name="workflowID">
          <Input autoComplete="off" />
        </Form.Item>

        {result === null ? null : (
          <Alert
            className="settings-action-result"
            message={result.started ? "Workflow accepted" : "Replay completed"}
            showIcon
            type={result.started ? "success" : "warning"}
            description={
              <Space direction="vertical" size={2}>
                <Typography.Text>
                  {result.workflow_id === "" ? "No workflow started" : `${result.workflow_id} / ${result.run_id}`}
                </Typography.Text>
                <Typography.Text type="secondary">
                  Groups {result.stats.groups_built}, snapshots {result.stats.snapshots_saved}
                </Typography.Text>
              </Space>
            }
          />
        )}

        <Space wrap>
          <Button htmlType="submit" icon={<ThunderboltOutlined />} loading={busy} type="primary">
            Start Replay
          </Button>
          <Button disabled={busy} onClick={onCancel} type="default">
            Close
          </Button>
        </Space>
      </Form>
    </Modal>
  );
}

function impactStatusColor(status: ReportWorkflowPolicyImpactPreviewResult["status"]) {
  switch (status) {
    case "ready":
      return "green";
    case "review":
      return "gold";
    case "blocked":
      return "red";
  }
}

function impactAlertType(status: ReportWorkflowPolicyImpactPreviewResult["status"]) {
  switch (status) {
    case "ready":
      return "success";
    case "review":
      return "warning";
    case "blocked":
      return "error";
  }
}

function reportChannelImpactText(result: ReportWorkflowPolicyImpactPreviewResult): string {
  if (!result.report_notification_channel_bound || result.report_notification_channel_profile_id === null) {
    return "No report channel bound";
  }
  return `#${result.report_notification_channel_profile_id} report scope ${result.report_notification_channel_has_report_scope ? "yes" : "no"}`;
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

function severityColor(severity: ImpactPreviewGroup["severity"]) {
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

function nullableDate(value: string | null): string {
  if (value === null) {
    return "Not set";
  }
  return formatDateTime(value);
}
