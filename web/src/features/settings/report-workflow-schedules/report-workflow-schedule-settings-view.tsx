"use client";

import {
  CalendarOutlined,
  EditOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined
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
import {
  disableReportWorkflowScheduleAction,
  enableReportWorkflowScheduleAction,
  refreshReportWorkflowSchedules,
  submitReportWorkflowSchedule
} from "./client-api";
import {
  emptyReportWorkflowScheduleForm,
  formStateToWriteRequest,
  formatDurationSeconds,
  scheduleToFormState
} from "./format";
import type {
  ReportWorkflowSchedule,
  ReportWorkflowScheduleFormState,
  ReportWorkflowScheduleListResponse,
  ReportWorkflowScheduleWriteRequest
} from "./types";

type ReportWorkflowScheduleSettingsManagerProps = {
  result: ApiResult<ReportWorkflowScheduleListResponse>;
};

const reportWorkflowSchedulesQueryKey = ["settings", "report-workflow-schedules"] as const;

type SaveScheduleVariables = {
  body: ReportWorkflowScheduleWriteRequest;
  scheduleID: number | null;
};

type EnablementVariables = {
  enabled: boolean;
  scheduleID: number;
};

export function ReportWorkflowScheduleSettingsManager({ result }: ReportWorkflowScheduleSettingsManagerProps) {
  const [form] = Form.useForm<ReportWorkflowScheduleFormState>();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [actionID, setActionID] = useState<number | null>(null);
  const {
    items: schedules,
    notice,
    query,
    refresh,
    setNotice
  } = useSettingsList({
    initialResult: result,
    queryKey: reportWorkflowSchedulesQueryKey,
    queryFn: refreshReportWorkflowSchedules,
    refreshMessage: "Schedules refreshed.",
    selectItems: (response) => response.items
  });
  const saveSchedule = useSettingsMutation<SaveScheduleVariables, ReportWorkflowSchedule>({
    invalidateQueryKey: reportWorkflowSchedulesQueryKey,
    mutationFn: ({ scheduleID, body }) => submitReportWorkflowSchedule(scheduleID, body)
  });
  const enablementAction = useSettingsMutation<EnablementVariables, ReportWorkflowSchedule>({
    invalidateQueryKey: reportWorkflowSchedulesQueryKey,
    mutationFn: ({ scheduleID, enabled }) =>
      enabled ? enableReportWorkflowScheduleAction(scheduleID) : disableReportWorkflowScheduleAction(scheduleID)
  });
  const busy = query.isFetching || saveSchedule.isPending || enablementAction.isPending;

  const summary = useMemo(() => {
    const enabled = schedules.filter((schedule) => schedule.enabled).length;
    const daily = schedules.filter((schedule) => schedule.interval_seconds === 86400).length;
    const policyCount = new Set(schedules.map((schedule) => schedule.report_workflow_policy_id)).size;
    return { enabled, daily, policyCount };
  }, [schedules]);

  async function handleRefresh() {
    await refresh();
  }

  async function handleSubmit(values: ReportWorkflowScheduleFormState) {
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
      return;
    }

    try {
      await saveSchedule.mutateAsync({ scheduleID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      return;
    }

    form.setFieldsValue(emptyReportWorkflowScheduleForm());
    setEditingID(null);
    setNotice({ kind: "info", message: "Schedule saved." });
  }

  async function handleEnablement(schedule: ReportWorkflowSchedule, enabled: boolean) {
    setActionID(schedule.id);
    try {
      await enablementAction.mutateAsync({ scheduleID: schedule.id, enabled });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
      setActionID(null);
      return;
    }
    setActionID(null);
    setNotice({ kind: enabled ? "info" : "warning", message: enabled ? "Schedule enabled." : "Schedule disabled." });
  }

  function editSchedule(schedule: ReportWorkflowSchedule) {
    setEditingID(schedule.id);
    form.setFieldsValue(scheduleToFormState(schedule));
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyReportWorkflowScheduleForm());
    setNotice(null);
  }

  return (
    <div className="stack">
      <Row aria-label="Report workflow schedule metrics" gutter={[12, 12]}>
        <MetricCard label="Schedules" value={schedules.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Daily" value={summary.daily} />
        <MetricCard label="Policies" value={summary.policyCount} />
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
            title={editingID === null ? "New Schedule" : `Edit Schedule #${editingID}`}
          >
            <Form<ReportWorkflowScheduleFormState>
              disabled={busy}
              form={form}
              initialValues={emptyReportWorkflowScheduleForm()}
              layout="vertical"
              onFinish={handleSubmit}
            >
              <Form.Item
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Schedule name is required." },
                  { max: 120, message: "Schedule name must be 120 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item
                label="Report workflow policy ID"
                name="reportWorkflowPolicyID"
                rules={[{ required: true, message: "Report workflow policy ID is required." }]}
              >
                <InputNumber min={1} precision={0} style={{ width: "100%" }} />
              </Form.Item>

              <Form.Item
                label="Temporal Schedule ID"
                name="temporalScheduleID"
                rules={[
                  { required: true, message: "Temporal Schedule ID is required." },
                  { max: 200, message: "Temporal Schedule ID must be 200 characters or fewer." }
                ]}
              >
                <Input autoComplete="off" placeholder="openclarion-report-policy-1-daily" />
              </Form.Item>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Interval seconds"
                    name="intervalSeconds"
                    rules={[{ required: true, message: "Interval is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Offset seconds"
                    name="offsetSeconds"
                    rules={[{ required: true, message: "Offset is required." }]}
                  >
                    <InputNumber min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Replay window seconds"
                    name="replayWindowSeconds"
                    rules={[{ required: true, message: "Replay window is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Replay delay seconds"
                    name="replayDelaySeconds"
                    rules={[{ required: true, message: "Replay delay is required." }]}
                  >
                    <InputNumber min={0} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Replay limit"
                    name="replayLimit"
                    rules={[{ required: true, message: "Replay limit is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Catch-up seconds"
                    name="catchupWindowSeconds"
                    rules={[{ required: true, message: "Catch-up window is required." }]}
                  >
                    <InputNumber min={1} precision={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>

              <Space wrap>
                <Button htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
                  Save Schedule
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
            title="Configured Schedules"
          >
            <ReportWorkflowScheduleTable
              actionID={actionID}
              busy={busy}
              onDisable={(schedule) => handleEnablement(schedule, false)}
              onEdit={editSchedule}
              onEnable={(schedule) => handleEnablement(schedule, true)}
              schedules={schedules}
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

type ReportWorkflowScheduleTableProps = {
  actionID: number | null;
  busy: boolean;
  onDisable: (schedule: ReportWorkflowSchedule) => void;
  onEdit: (schedule: ReportWorkflowSchedule) => void;
  onEnable: (schedule: ReportWorkflowSchedule) => void;
  schedules: ReportWorkflowSchedule[];
};

function ReportWorkflowScheduleTable({
  actionID,
  busy,
  onDisable,
  onEdit,
  onEnable,
  schedules
}: ReportWorkflowScheduleTableProps) {
  const columns: TableColumnsType<ReportWorkflowSchedule> = [
    {
      key: "name",
      title: "Name",
      render: (_, schedule) => (
        <Space direction="vertical" size={2}>
          <Typography.Text strong>{schedule.name}</Typography.Text>
          <Typography.Text type="secondary">Policy #{schedule.report_workflow_policy_id}</Typography.Text>
          <Typography.Text className="settings-event-ids" type="secondary">
            {schedule.temporal_schedule_id}
          </Typography.Text>
        </Space>
      )
    },
    {
      key: "cadence",
      title: "Cadence",
      render: (_, schedule) => (
        <Space direction="vertical" size={2}>
          <Tag color="blue">{formatDurationSeconds(schedule.interval_seconds)}</Tag>
          <Typography.Text type="secondary">offset {formatDurationSeconds(schedule.offset_seconds)}</Typography.Text>
        </Space>
      )
    },
    {
      key: "replay",
      title: "Replay",
      render: (_, schedule) => (
        <Space direction="vertical" size={2}>
          <Typography.Text>{formatDurationSeconds(schedule.replay_window_seconds)} window</Typography.Text>
          <Typography.Text type="secondary">
            delay {formatDurationSeconds(schedule.replay_delay_seconds)} / limit {schedule.replay_limit}
          </Typography.Text>
        </Space>
      )
    },
    {
      dataIndex: "catchup_window_seconds",
      key: "catchup",
      title: "Catch-up",
      render: (seconds: number) => formatDurationSeconds(seconds)
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: "State",
      render: (enabled: boolean, schedule) => (
        <Space direction="vertical" size={2}>
          <Tag color={enabled ? "green" : "default"}>{enabled ? "Enabled" : "Draft"}</Tag>
          <Typography.Text type="secondary">
            {enabled ? nullableDate(schedule.enabled_at) : nullableDate(schedule.disabled_at)}
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
      render: (_, schedule) => (
        <Space wrap>
          <Button disabled={busy || actionID !== null} icon={<EditOutlined />} onClick={() => onEdit(schedule)} size="small">
            Edit
          </Button>
          {schedule.enabled ? (
            <Button
              disabled={busy || actionID !== null}
              icon={<PauseCircleOutlined />}
              loading={actionID === schedule.id}
              onClick={() => onDisable(schedule)}
              size="small"
            >
              Disable
            </Button>
          ) : (
            <Button
              disabled={busy || actionID !== null}
              icon={<PlayCircleOutlined />}
              loading={actionID === schedule.id}
              onClick={() => onEnable(schedule)}
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
    <Table<ReportWorkflowSchedule>
      columns={columns}
      dataSource={schedules}
      loading={busy}
      locale={{
        emptyText: (
          <Empty
            description="No report workflow schedules"
            image={<CalendarOutlined aria-hidden className="settings-empty-icon" />}
          />
        )
      }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1040 }}
    />
  );
}

function nullableDate(value: string | null): string {
  return value === null ? "-" : formatDateTime(value);
}
