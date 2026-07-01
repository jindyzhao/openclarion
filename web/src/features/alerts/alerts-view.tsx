"use client";

import { CheckCircleOutlined, ExperimentOutlined, LinkOutlined, MessageOutlined, WarningOutlined } from "@ant-design/icons";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Alert, App as AntdApp, Button, Col, Descriptions, Empty, Row, Space, Statistic, Table, Tag, Typography } from "antd";
import type { DescriptionsProps, TableColumnsType } from "antd";
import type { Route } from "next";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState, type ReactNode } from "react";

import type { DiagnosisRoomSummary } from "@/features/diagnosis-room/api";
import { diagnosisRoomNextStep } from "@/features/diagnosis-room/next-step";
import {
  diagnosisNotificationContentProofDisplay,
  diagnosisNotificationDeliveryCoverage,
  diagnosisNotificationDeliveryCoveragePhaseColor,
} from "@/features/diagnosis-room/notification-content-proof";
import { diagnosisRoomLinkHref } from "@/features/diagnosis-room/url-state";
import { reportReplayProofTrace } from "@/features/report-replay/proof-trace";
import { ReportShell } from "@/features/reports/report-shell";
import { formatDateTime } from "@/features/reports/format";
import { refreshDirectoryUsers } from "@/features/settings/directory-rbac/client-api";
import { directoryUserSubjectIndex } from "@/features/settings/directory-rbac/format";
import { DirectorySubjectTags } from "@/features/settings/directory-rbac/subject-tags";
import type { DirectoryUser } from "@/features/settings/directory-rbac/types";
import type { ApiResult } from "@/lib/api/client";
import { useClientReady } from "@/lib/react/use-client-ready";

import type {
  AlertEventSummary,
  AlertEvidenceSnapshotLink,
  AlertListResponse,
  ReportReplayTriggerRequest,
  ReportReplayTriggerResponse
} from "./api";
import { triggerReportReplayAction } from "./client-api";
import {
  alertDiagnosisClosureSummary,
  alertDiagnosisEvidenceActions,
  alertDiagnosisEvidenceProgressSummary,
  alertDiagnosisRoomPrimaryAction,
  alertDiagnosisDeliveryReviewAction,
  diagnosisRoomNotificationFailed,
  diagnosisRoomNotificationChannelReviewHref,
  latestDiagnosisRoomNotification,
} from "./diagnosis-delivery";

type AlertsViewProps = {
  alertsResult: ApiResult<AlertListResponse>;
};

type AlertReplayProof = {
  alert: AlertEventSummary;
  result: ReportReplayTriggerResponse;
};

export function AlertsView({ alertsResult }: AlertsViewProps) {
  const { message } = AntdApp.useApp();
  const router = useRouter();
  const clientReady = useClientReady();
  const [latestReplayProof, setLatestReplayProof] = useState<AlertReplayProof | null>(null);
  const loadedCount = alertsResult.ok ? alertsResult.data.items.length : 0;
  const directoryUsersQuery = useQuery({
    queryFn: refreshDirectoryUsers,
    queryKey: ["settings", "directory", "users"],
    staleTime: 300_000,
  });
  const directoryUsersBySubject = useMemo(
    () =>
      directoryUserSubjectIndex(
        directoryUsersQuery.data?.ok === true
          ? directoryUsersQuery.data.data.items
          : [],
      ),
    [directoryUsersQuery.data],
  );
  const replayMutation = useMutation<ReportReplayTriggerResponse, Error, AlertEventSummary>({
    mutationFn: async (alert) => {
      const result = await triggerReportReplayAction(alertReplayRequest(alert));
      if (!result.ok) {
        throw new Error(result.error.message);
      }
      return result.data;
    },
    onSuccess: (result, alert) => {
      setLatestReplayProof({ alert, result });
      message.success(replayAcceptedMessage(result));
      router.refresh();
    },
    onError: (error) => {
      message.error(error.message);
    }
  });

  return (
    <ReportShell current="alerts">
      <section className="page-heading">
        <div>
          <h1>Alerts</h1>
          <p>Recent alert events with evidence snapshots and AI diagnosis handoffs.</p>
        </div>
        {alertsResult.ok ? (
          <Typography.Text className="status-line">
            {loadedCount} alert{loadedCount === 1 ? "" : "s"} loaded
          </Typography.Text>
        ) : null}
      </section>

      {!alertsResult.ok ? <ErrorNotice message={alertsResult.error.message} status={alertsResult.error.status} /> : null}

      {alertsResult.ok ? <AlertSummary alerts={alertsResult.data.items} /> : null}

      {latestReplayProof === null ? null : <AlertReplayProofPanel proof={latestReplayProof} />}

      {alertsResult.ok && alertsResult.data.items.length === 0 ? (
        <Empty description="No alerts available." image={<WarningOutlined aria-hidden className="settings-empty-icon" />} />
      ) : null}

      {alertsResult.ok && alertsResult.data.items.length > 0 ? (
        <AlertTable
          alerts={alertsResult.data.items}
          clientReady={clientReady}
          directoryUsersBySubject={directoryUsersBySubject}
          onReplayAlert={(alert) => replayMutation.mutate(alert)}
          replayingAlertID={replayMutation.isPending ? replayMutation.variables?.id ?? null : null}
        />
      ) : null}
    </ReportShell>
  );
}

function AlertReplayProofPanel({ proof }: { proof: AlertReplayProof }) {
  const trace = reportReplayProofTrace(proof.result);
  const rooms = proof.result.auto_diagnosis?.rooms ?? [];
  const nextAction = replayProofNextAction(proof.result);

  return (
    <section aria-label="Latest alert replay proof" className="alert-replay-proof panel">
      <div className="panel-header">
        <h2>Latest Replay Proof</h2>
        <Space size={[6, 6]} wrap>
          <Tag>{alertName(proof.alert)}</Tag>
          <Tag color={replayTraceStatusColor(trace.status)}>{replayTraceStatusLabel(trace.status)}</Tag>
        </Space>
      </div>
      <div className="panel-body">
        <Typography.Text type="secondary">{trace.detail}</Typography.Text>
        <Typography.Text className="settings-event-ids" copyable type="secondary">
          Correlation {proof.result.correlation_key}
        </Typography.Text>
        <Alert
          className="alert-replay-next-action"
          description={
            <Space className="alert-replay-next-action-body" size={[8, 8]} wrap>
              <Typography.Text>{nextAction.detail}</Typography.Text>
              {nextAction.href ? (
                <Button href={nextAction.href} icon={replayProofNextActionIcon(nextAction.kind)} size="small" type="primary">
                  {nextAction.actionLabel}
                </Button>
              ) : null}
            </Space>
          }
          message={nextAction.label}
          showIcon
          type={nextAction.type}
        />
        <div className="workflow-automation-grid">
          {trace.items.map((item) => (
            <div className="workflow-automation-item" key={item.title}>
              <div className="workflow-automation-item-header">
                <Typography.Text className="muted">{item.title}</Typography.Text>
                <Tag color={replayTraceStatusColor(item.status)}>{replayTraceStatusLabel(item.status)}</Tag>
              </div>
              <Typography.Text strong>{item.value}</Typography.Text>
              <Typography.Text type="secondary">{item.detail}</Typography.Text>
              {item.actions && item.actions.length > 0 ? (
                <Space size={[4, 4]} wrap>
                  {item.actions.map((action) => (
                    <Button
                      href={action.href}
                      key={`${item.title}:${action.href}`}
                      size="small"
                      type="link"
                    >
                      {action.label}
                    </Button>
                  ))}
                </Space>
              ) : null}
            </div>
          ))}
        </div>
        {proof.result.snapshots.length > 0 || rooms.length > 0 ? (
          <Space aria-label="Replay proof links" size={[6, 6]} wrap>
            {proof.result.snapshots.map((snapshot) => (
              <Button href={diagnosisHref(snapshot.id)} icon={<ExperimentOutlined />} key={snapshot.id} size="small">
                Snapshot #{snapshot.id}
              </Button>
            ))}
            {rooms.map((room) => (
              <Button href={replayDiagnosisRoomHref(room)} icon={<MessageOutlined />} key={room.session_id} size="small" type="primary">
                Room #{room.evidence_snapshot_id}
              </Button>
            ))}
          </Space>
        ) : null}
      </div>
    </section>
  );
}

function AlertSummary({ alerts }: { alerts: AlertEventSummary[] }) {
  const firing = alerts.filter((alert) => alert.status === "firing").length;
  const resolved = alerts.filter((alert) => alert.status === "resolved").length;
  const linked = alerts.filter((alert) => alert.linked_evidence_snapshots.length > 0).length;
  const rooms = alerts.reduce(
    (total, alert) =>
      total +
      alert.linked_evidence_snapshots.reduce((snapshotTotal, snapshot) => snapshotTotal + snapshot.diagnosis_rooms.length, 0),
    0
  );

  return (
    <Row aria-label="Alert summary" className="alert-summary-grid" gutter={[12, 12]}>
      <MetricCard label="Firing" prefix={<WarningOutlined />} value={firing} />
      <MetricCard label="Resolved" value={resolved} />
      <MetricCard label="Linked alerts" prefix={<LinkOutlined />} value={linked} />
      <MetricCard label="AI rooms" prefix={<MessageOutlined />} value={rooms} />
    </Row>
  );
}

function AlertTable({
  alerts,
  clientReady,
  directoryUsersBySubject,
  onReplayAlert,
  replayingAlertID
}: {
  alerts: AlertEventSummary[];
  clientReady: boolean;
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  onReplayAlert: (alert: AlertEventSummary) => void;
  replayingAlertID: number | null;
}) {
  const columns: TableColumnsType<AlertEventSummary> = [
    {
      key: "alert",
      render: (_value, alert) => (
        <Space className="alert-meta-stack" direction="vertical" size={6}>
          <Typography.Text strong>{alertName(alert)}</Typography.Text>
          <Typography.Text type="secondary">{alertSummary(alert)}</Typography.Text>
          <LabelTags alert={alert} />
        </Space>
      ),
      title: "Alert",
      width: 320
    },
    {
      key: "state",
      render: (_value, alert) => (
        <Space direction="vertical" size={6}>
          <Tag color={alert.status === "firing" ? "red" : "green"}>{alert.status}</Tag>
          <Tag color={severityColor(alertSeverity(alert))}>{alertSeverity(alert)}</Tag>
        </Space>
      ),
      title: "State",
      width: 130
    },
    {
      key: "source",
      render: (_value, alert) => (
        <Space direction="vertical" size={4}>
          <Typography.Text strong>{alert.source}</Typography.Text>
          {alert.alert_source_profile_id > 0 ? (
            <Tag>profile #{alert.alert_source_profile_id}</Tag>
          ) : null}
          <Typography.Text className="settings-event-ids" copyable type="secondary">
            {alert.source_fingerprint}
          </Typography.Text>
        </Space>
      ),
      title: "Source",
      width: 210
    },
    {
      key: "started",
      render: (_value, alert) => (
        <Space direction="vertical" size={4}>
          <Typography.Text>{formatDateTime(alert.starts_at)}</Typography.Text>
          {alert.ends_at ? <Typography.Text type="secondary">ended {formatDateTime(alert.ends_at)}</Typography.Text> : null}
        </Space>
      ),
      title: "Started",
      width: 190
    },
    {
      key: "evidence",
      render: (_value, alert) => <SnapshotLinks linkedSnapshots={alert.linked_evidence_snapshots} />,
      title: "Evidence",
      width: 210
    },
    {
      key: "diagnosis",
      render: (_value, alert) => (
        <DiagnosisLinks
          directoryUsersBySubject={directoryUsersBySubject}
          linkedSnapshots={alert.linked_evidence_snapshots}
        />
      ),
      title: "AI Diagnosis",
      width: 310
    },
    {
      key: "actions",
      render: (_value, alert) => (
        <AlertActions
          alert={alert}
          clientReady={clientReady}
          onReplayAlert={onReplayAlert}
          replaying={replayingAlertID === alert.id}
        />
      ),
      title: "Actions",
      width: 170
    }
  ];

  return (
    <section aria-label="Recent alerts" className="alerts-table-panel">
      <Table<AlertEventSummary>
        columns={columns}
        dataSource={alerts}
        expandable={{
          columnWidth: 48,
          defaultExpandedRowKeys: defaultExpandedAlertKeys(alerts),
          expandedRowRender: (alert) => (
            <AlertDetailPanel
              alert={alert}
              directoryUsersBySubject={directoryUsersBySubject}
            />
          )
        }}
        pagination={false}
        rowKey="id"
        scroll={{ x: 1540 }}
      />
    </section>
  );
}

function AlertDetailPanel({
  alert,
  directoryUsersBySubject,
}: {
  alert: AlertEventSummary;
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
}) {
  return (
    <section aria-label={`Alert details for ${alertName(alert)}`} className="alert-detail-panel">
      <Descriptions column={{ xs: 1, md: 2 }} items={alertDescriptionItems(alert)} size="small" />
      <div className="alert-detail-grid">
        <KeyValueBlock emptyText="No labels" entries={sortedRecordEntries(alert.labels)} title="Labels" />
        <KeyValueBlock emptyText="No annotations" entries={sortedRecordEntries(alert.annotations)} title="Annotations" />
        <EvidenceHandoffPanel
          directoryUsersBySubject={directoryUsersBySubject}
          linkedSnapshots={alert.linked_evidence_snapshots}
        />
      </div>
    </section>
  );
}

function EvidenceHandoffPanel({
  directoryUsersBySubject,
  linkedSnapshots,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  linkedSnapshots: AlertEvidenceSnapshotLink[];
}) {
  return (
    <section className="alert-detail-block">
      <div className="alert-detail-block-header">
        <Typography.Title level={3}>Evidence and AI handoff</Typography.Title>
        <Tag color={linkedSnapshots.length > 0 ? "success" : "warning"}>{linkedSnapshots.length} snapshot(s)</Tag>
      </div>
      {linkedSnapshots.length === 0 ? (
        <Alert
          description="No evidence snapshot is linked yet. Create or replay the alert workflow before opening an AI diagnosis room."
          message="Evidence pending"
          showIcon
          type="warning"
        />
      ) : (
        <div className="alert-detail-snapshot-list">
          {linkedSnapshots.map((snapshot) => (
            <section className="alert-detail-snapshot" key={snapshot.id}>
              <div className="alert-detail-snapshot-header">
                <Space size={[6, 6]} wrap>
                  <Typography.Text strong>Snapshot #{snapshot.id}</Typography.Text>
                  <Tag color={snapshotStatusColor(snapshot.status)}>{snapshot.status}</Tag>
                  <Tag>group #{snapshot.alert_group_id}</Tag>
                </Space>
                <Button href={diagnosisHref(snapshot.id)} icon={<MessageOutlined />} size="small" type="link">
                  Open diagnosis
                </Button>
              </div>
              <Typography.Text type="secondary">{snapshot.created_by_workflow || "manual"}</Typography.Text>
              {snapshot.diagnosis_rooms.length > 0 ? (
                <div className="alert-detail-room-list">
                  {snapshot.diagnosis_rooms.map((room) => (
                    <div className="alert-detail-room" key={room.chat_session_id}>
                      <Space size={[6, 6]} wrap>
                        <Button href={diagnosisRoomHref(room)} icon={<MessageOutlined />} size="small" type="link">
                          {room.session_id}
                        </Button>
                        <RoomTags room={room} />
                      </Space>
                      {room.latest_conclusion?.content ? (
                        <Typography.Paragraph className="alert-detail-conclusion">
                          {room.latest_conclusion.content}
                        </Typography.Paragraph>
                      ) : null}
                      <RoomCloseSummary room={room} />
                      <NotificationDeliverySummary room={room} />
                      <DiagnosisClosureSummary
                        directoryUsersBySubject={directoryUsersBySubject}
                        room={room}
                      />
                      <DiagnosisEvidenceProgressSummary room={room} />
                      <DiagnosisEvidenceRequestActions room={room} />
                    </div>
                  ))}
                </div>
              ) : (
                <Space direction="vertical" size={4}>
                  <DiagnosisNextStep
                    color="warning"
                    detail={`Evidence #${snapshot.id} is retained, but no AI room has been started yet.`}
                    label="Manual room needed"
                  />
                  <Typography.Text type="secondary">Open diagnosis to create an AI room from this snapshot.</Typography.Text>
                </Space>
              )}
            </section>
          ))}
        </div>
      )}
    </section>
  );
}

function KeyValueBlock({
  emptyText,
  entries,
  title
}: {
  emptyText: string;
  entries: Array<[string, string]>;
  title: string;
}) {
  return (
    <section className="alert-detail-block">
      <div className="alert-detail-block-header">
        <Typography.Title level={3}>{title}</Typography.Title>
        <Tag>{entries.length}</Tag>
      </div>
      {entries.length === 0 ? (
        <Typography.Text type="secondary">{emptyText}</Typography.Text>
      ) : (
        <Space className="alert-detail-key-values" size={[6, 6]} wrap>
          {entries.map(([key, value]) => (
            <Tag className="alert-detail-kv-tag" key={key}>
              {key}: {value}
            </Tag>
          ))}
        </Space>
      )}
    </section>
  );
}

function SnapshotLinks({ linkedSnapshots }: { linkedSnapshots: AlertEvidenceSnapshotLink[] }) {
  if (linkedSnapshots.length === 0) {
    return <Typography.Text type="secondary">No snapshot linked</Typography.Text>;
  }
  return (
    <Space aria-label="Linked evidence snapshots" direction="vertical" size={8}>
      {linkedSnapshots.map((snapshot) => (
        <Space direction="vertical" key={snapshot.id} size={4}>
          <Link href={diagnosisHref(snapshot.id)}>Snapshot #{snapshot.id}</Link>
          <Space size={[6, 6]} wrap>
            <Tag color={snapshotStatusColor(snapshot.status)}>{snapshot.status}</Tag>
            <Tag icon={<ExperimentOutlined />}>group #{snapshot.alert_group_id}</Tag>
          </Space>
          <Typography.Text type="secondary">{snapshot.created_by_workflow || "manual"}</Typography.Text>
        </Space>
      ))}
    </Space>
  );
}

function DiagnosisLinks({
  directoryUsersBySubject,
  linkedSnapshots,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  linkedSnapshots: AlertEvidenceSnapshotLink[];
}) {
  if (linkedSnapshots.length === 0) {
    return <Typography.Text type="secondary">Awaiting evidence</Typography.Text>;
  }

  return (
    <Space aria-label="Linked diagnosis rooms" direction="vertical" size={8}>
      {linkedSnapshots.map((snapshot) => {
        if (snapshot.diagnosis_rooms.length === 0) {
          return (
            <Space direction="vertical" key={snapshot.id} size={4}>
              <Link href={diagnosisHref(snapshot.id)}>Open diagnosis</Link>
              <DiagnosisNextStep
                color="warning"
                detail={`Create an AI room from evidence #${snapshot.id} before the alert can be reviewed.`}
                label="Create AI room"
              />
              <Typography.Text type="secondary">evidence #{snapshot.id}</Typography.Text>
            </Space>
          );
        }
        return snapshot.diagnosis_rooms.map((room) => (
          <Space className="alert-room-link" direction="vertical" key={room.chat_session_id} size={4}>
            <Space size={[8, 6]} wrap>
              <Link href={diagnosisRoomHref(room)}>{room.session_id}</Link>
              {room.latest_conclusion ? <Link href={reviewDiagnosisRoomHref(room)}>Review conclusion</Link> : null}
            </Space>
            <RoomTags room={room} />
            <DiagnosisRoomNextStep room={room} />
            <RoomCloseSummary room={room} />
            <NotificationDeliverySummary room={room} />
            <DiagnosisClosureSummary
              directoryUsersBySubject={directoryUsersBySubject}
              room={room}
            />
            <DiagnosisEvidenceProgressSummary room={room} />
            <DiagnosisEvidenceRequestActions room={room} />
            {room.latest_conclusion?.content ? (
              <Typography.Paragraph className="alert-room-conclusion" ellipsis={{ rows: 2, tooltip: room.latest_conclusion.content }}>
                {room.latest_conclusion.content}
              </Typography.Paragraph>
            ) : null}
          </Space>
        ));
      })}
    </Space>
  );
}

function RoomTags({ room }: { room: DiagnosisRoomSummary }) {
  return (
    <Space className="alert-room-meta" size={[6, 6]} wrap>
      <Tag color={roomStatusColor(room.room_status)}>{room.room_status}</Tag>
      <Tag color={taskStatusColor(room.task_status)}>{room.task_status}</Tag>
      {room.latest_conclusion ? (
        <>
          <Tag color="success">{room.latest_conclusion.status}</Tag>
          {room.latest_conclusion.confidence ? (
            <Tag color={confidenceColor(room.latest_conclusion.confidence)}>{room.latest_conclusion.confidence}</Tag>
          ) : null}
          {room.latest_conclusion.requires_human_review ? <Tag color="warning">review</Tag> : null}
        </>
      ) : null}
      {!room.latest_conclusion && room.latest_progress ? <ProgressTags room={room} /> : null}
      <Typography.Text type="secondary">{room.turn_count} turn(s)</Typography.Text>
    </Space>
  );
}

function RoomCloseSummary({ room }: { room: DiagnosisRoomSummary }) {
  if (room.room_status !== "closed") {
    return null;
  }
  const closeParts = [
    room.closed_at ? `Closed ${formatDateTime(room.closed_at)}` : "Closed",
    room.close_reason ? `reason: ${room.close_reason}` : "No close reason recorded"
  ];
  return (
    <Typography.Text className="alert-room-close-summary" type="secondary">
      {closeParts.join(" / ")}
    </Typography.Text>
  );
}

function NotificationDeliverySummary({ room }: { room: DiagnosisRoomSummary }) {
  const latest = latestDiagnosisRoomNotification(room);
  if (!latest) {
    return (
      <Typography.Text className="alert-notification-summary" type="secondary">
        No AI notification recorded
      </Typography.Text>
    );
  }
  const failed = diagnosisRoomNotificationFailed(latest.provider_status);
  const proof = diagnosisNotificationContentProofDisplay(latest);
  const deliveryCoverage = diagnosisNotificationDeliveryCoverage(room.notification_timeline ?? []);
  const reviewAction = alertDiagnosisDeliveryReviewAction(room);
  return (
    <Space
      aria-label={`Notification delivery for ${room.session_id}`}
      className="alert-notification-summary"
      size={[6, 6]}
      wrap
    >
      <Tag color={notificationStatusColor(latest.provider_status)}>{notificationEventLabel(latest.event_kind)}</Tag>
      <Tag color={notificationStatusColor(latest.provider_status)}>{latest.provider_status}</Tag>
      <Tag color={proof.color}>{proof.label}</Tag>
      <Tag color={deliveryCoverage.color}>{deliveryCoverage.label}</Tag>
      {deliveryCoverage.phases.map((phase) => (
        <Tag color={diagnosisNotificationDeliveryCoveragePhaseColor(phase.status)} key={phase.key}>
          {phase.label}: {phase.status}
        </Tag>
      ))}
      {latest.confidence ? <Tag color={confidenceColor(latest.confidence)}>{latest.confidence}</Tag> : null}
      {latest.requires_human_review ? <Tag color="warning">review</Tag> : null}
      <Typography.Text type="secondary">{formatDateTime(latest.occurred_at)}</Typography.Text>
      <Typography.Text type={deliveryCoverage.status === "blocked" ? "danger" : "secondary"}>
        {deliveryCoverage.detail}
      </Typography.Text>
      {proof.hasProof ? <Typography.Text type="secondary">{proof.detail}</Typography.Text> : null}
      {latest.provider_message_id ? (
        <Typography.Text className="settings-event-ids" type="secondary">
          {latest.provider_message_id}
        </Typography.Text>
      ) : null}
      {failed ? (
        <Button href={diagnosisRoomNotificationChannelReviewHref(latest)} size="small" type="link">
          Review channel
        </Button>
      ) : null}
      {reviewAction && !failed ? (
        <Button danger={reviewAction.danger} href={reviewAction.href} size="small" type="link">
          Review delivery proof
        </Button>
      ) : null}
    </Space>
  );
}

function DiagnosisEvidenceRequestActions({ room }: { room: DiagnosisRoomSummary }) {
  const actions = alertDiagnosisEvidenceActions(room);
  if (actions.length === 0) {
    return null;
  }
  const visibleActions = actions.slice(0, 3);
  const remaining = actions.length - visibleActions.length;
  return (
    <Space
      aria-label={`AI evidence requests for ${room.session_id}`}
      className="alert-evidence-actions"
      size={[6, 6]}
      wrap
    >
      <Typography.Text type="secondary">AI requested evidence</Typography.Text>
      {visibleActions.map((action) => (
        <Space key={action.key} size={4}>
          <Button href={action.href} size="small" type="link">
            {action.actionLabel} {action.label}
          </Button>
          <Tag color={action.kind === "executable" ? "processing" : "warning"}>
            {action.kind === "executable" ? action.tool : action.priority}
          </Tag>
        </Space>
      ))}
      {remaining > 0 ? <Tag>+{remaining} more</Tag> : null}
    </Space>
  );
}

function DiagnosisEvidenceProgressSummary({ room }: { room: DiagnosisRoomSummary }) {
  const summary = alertDiagnosisEvidenceProgressSummary(room);
  if (summary === null) {
    return null;
  }
  return (
    <Space
      aria-label={`AI evidence progress for ${room.session_id}`}
      className="alert-evidence-progress"
      size={[6, 6]}
      wrap
    >
      <Typography.Text type="secondary">AI review progress</Typography.Text>
      <Tag color={confidenceColor(summary.latestConfidence)}>
        {summary.confidenceLabel}
      </Tag>
      <Tag color={summary.openEvidence > 0 ? "warning" : "success"}>
        {summary.evidenceLabel}
      </Tag>
      <Typography.Text type="secondary">{summary.detail}</Typography.Text>
    </Space>
  );
}

function DiagnosisClosureSummary({
  directoryUsersBySubject,
  room,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  room: DiagnosisRoomSummary;
}) {
  const summary = alertDiagnosisClosureSummary(room);
  if (summary === null) {
    return null;
  }
  return (
    <Space
      aria-label={`AI closure status for ${room.session_id}`}
      className="alert-closure-summary"
      size={[6, 6]}
      wrap
    >
      <Typography.Text type="secondary">AI closure</Typography.Text>
      <Tag color={summary.color}>{summary.label}</Tag>
      {summary.confirmedBy === "" ? (
        <Tag color="warning">Confirmation needed</Tag>
      ) : (
        <DirectorySubjectTags
          directoryUsersBySubject={directoryUsersBySubject}
          label="Confirmed by"
          subject={summary.confirmedBy}
        />
      )}
      <Tag color={summary.roomClosed ? "success" : "default"}>
        {summary.roomClosed ? "Room closed" : "Room open"}
      </Tag>
      <Tag color={diagnosisNotificationDeliveryCoveragePhaseColor(summary.closeProofStatus)}>
        {summary.closeProofLabel}
      </Tag>
      <Typography.Text type="secondary">{summary.detail}</Typography.Text>
    </Space>
  );
}

function ProgressTags({ room }: { room: DiagnosisRoomSummary }) {
  const progress = room.latest_progress;
  if (!progress) {
    return null;
  }
  const missingCount = progress.missing_evidence_requests?.length ?? 0;
  return (
    <>
      <Tag color={progress.conclusion_status === "needs_evidence" ? "warning" : "processing"}>
        {progress.conclusion_status || progress.status}
      </Tag>
      <Tag color={confidenceColor(progress.confidence)}>{progress.confidence}</Tag>
      {missingCount > 0 ? <Tag color="warning">{missingCount} missing</Tag> : null}
      {progress.requires_human_review ? <Tag color="warning">review</Tag> : null}
    </>
  );
}

function notificationEventLabel(eventKind: string): string {
  switch (eventKind) {
    case "diagnosis_room.assistant_turn_notification_sent":
      return "AI update notification";
    case "diagnosis_room.final_ready_notification_sent":
      return "Final-ready notification";
    case "diagnosis_room.close_notification_sent":
      return "Close notification";
    default:
      return eventKind;
  }
}

function notificationStatusColor(status: string): string {
  if (diagnosisRoomNotificationFailed(status)) {
    return "error";
  }
  switch (status.toLowerCase()) {
    case "delivered":
    case "success":
    case "accepted":
    case "sent":
      return "success";
    case "pending":
    case "queued":
      return "processing";
    default:
      return "default";
  }
}

function AlertActions({
  alert,
  clientReady,
  onReplayAlert,
  replaying
}: {
  alert: AlertEventSummary;
  clientReady: boolean;
  onReplayAlert: (alert: AlertEventSummary) => void;
  replaying: boolean;
}) {
  const snapshot = alert.linked_evidence_snapshots[0];
  if (!snapshot) {
    return (
      <Space className="alert-action-stack" direction="vertical" size={4}>
        <Button
          aria-label={`Replay window for ${alertName(alert)}`}
          disabled={!clientReady || replaying}
          icon={<ExperimentOutlined />}
          loading={replaying}
          onClick={() => onReplayAlert(alert)}
          size="small"
        >
          Replay window
        </Button>
        <Typography.Text type="secondary">Replay alert window to create evidence.</Typography.Text>
      </Space>
    );
  }
  const rooms = alert.linked_evidence_snapshots.flatMap((item) => item.diagnosis_rooms);
  const action = alertDiagnosisRoomPrimaryAction(rooms);
  if (!action) {
    return (
      <Space className="alert-action-stack" direction="vertical" size={4}>
        <Button href={diagnosisHref(snapshot.id)} icon={<MessageOutlined />} size="small" type="primary">
          Open diagnosis
        </Button>
        <Typography.Text type="secondary">Create an AI room from the linked evidence.</Typography.Text>
      </Space>
    );
  }
  return (
    <Space className="alert-action-stack" direction="vertical" size={4}>
      <Button
        danger={action.danger}
        href={action.href}
        icon={primaryDiagnosisRoomActionIcon(action.iconKind)}
        size="small"
        type="primary"
      >
        {action.label}
      </Button>
      <Typography.Text type="secondary">{action.hint}</Typography.Text>
    </Space>
  );
}

function DiagnosisRoomNextStep({ room }: { room: DiagnosisRoomSummary }) {
  const step = diagnosisRoomNextStep(room);
  return <DiagnosisNextStep color={step.color} detail={step.detail} label={step.label} />;
}

function primaryDiagnosisRoomActionIcon(
  iconKind: NonNullable<ReturnType<typeof alertDiagnosisRoomPrimaryAction>>["iconKind"]
): ReactNode {
  switch (iconKind) {
    case "attention":
      return <WarningOutlined />;
    case "closed":
    case "review":
      return <CheckCircleOutlined />;
    case "room":
      return <MessageOutlined />;
  }
}

function DiagnosisNextStep({
  color,
  detail,
  label
}: {
  color: string;
  detail: string;
  label: string;
}) {
  return (
    <Space className="alert-next-step" size={[6, 6]} wrap>
      <Tag color={color}>{label}</Tag>
      <Typography.Text type="secondary">{detail}</Typography.Text>
    </Space>
  );
}

function LabelTags({ alert }: { alert: AlertEventSummary }) {
  const keys = ["namespace", "pod", "instance", "job", "service"];
  const tags = keys.flatMap((key) => {
    const value = alert.labels[key];
    return value ? [{ key, value }] : [];
  });
  if (tags.length === 0) {
    return null;
  }
  return (
    <Space className="alert-label-strip" size={[6, 6]} wrap>
      {tags.map((tag) => (
        <Tag key={tag.key}>
          {tag.key}: {tag.value}
        </Tag>
      ))}
    </Space>
  );
}

function alertName(alert: AlertEventSummary): string {
  return alert.labels.alertname || alert.labels.alert_name || `Alert #${alert.id}`;
}

function alertSummary(alert: AlertEventSummary): string {
  return (
    alert.annotations.summary ||
    alert.annotations.description ||
    alert.annotations.message ||
    alert.canonical_fingerprint
  );
}

function alertSeverity(alert: AlertEventSummary): string {
  const value = alert.labels.severity || alert.labels.priority || "info";
  return value.trim() === "" ? "info" : value;
}

function diagnosisRoomHref(room: DiagnosisRoomSummary): Route {
  return diagnosisRoomLinkHref({
    evidenceSnapshotID: room.evidence_snapshot_id,
    sessionID: room.session_id
  }) as Route;
}

function reviewDiagnosisRoomHref(room: DiagnosisRoomSummary): Route {
  return diagnosisRoomLinkHref({
    evidenceSnapshotID: room.evidence_snapshot_id,
    intent: "review_conclusion",
    sessionID: room.session_id
  }) as Route;
}

function diagnosisHref(snapshotID: number): Route {
  return diagnosisRoomLinkHref({
    evidenceSnapshotID: snapshotID,
    intent: "alert_review"
  }) as Route;
}

type AutoDiagnosisSummary = NonNullable<ReportReplayTriggerResponse["auto_diagnosis"]>;
type AutoDiagnosisRoom = NonNullable<AutoDiagnosisSummary["rooms"]>[number];

function replayDiagnosisRoomHref(room: AutoDiagnosisRoom): Route {
  return diagnosisRoomLinkHref({
    evidenceSnapshotID: room.evidence_snapshot_id,
    intent: "review_conclusion",
    sessionID: room.session_id
  }) as Route;
}

type AlertReplayProofNextAction = {
  actionLabel: string;
  detail: string;
  href?: Route;
  kind: "report" | "room" | "snapshot" | "unavailable";
  label: string;
  type: "info" | "warning";
};

function replayProofNextAction(result: ReportReplayTriggerResponse): AlertReplayProofNextAction {
  if (!result.started) {
    return {
      actionLabel: "Review replay setup",
      detail: "Replay did not start a report workflow, so there is no downstream notification proof to inspect yet.",
      kind: "unavailable",
      label: "Replay proof unavailable",
      type: "warning"
    };
  }

  const autoDiagnosis = result.auto_diagnosis;
  const room = autoDiagnosis?.rooms?.[0] ?? null;
  if (autoDiagnosis && autoDiagnosis.rooms_started > 0 && room) {
    return {
      actionLabel: "Open room timeline",
      detail: "Open the automatic diagnosis room and confirm assistant, final-ready, or close notifications in its retained timeline.",
      href: replayDiagnosisRoomHref(room),
      kind: "room",
      label: "Review room notification timeline",
      type: "warning"
    };
  }

  const snapshot = result.snapshots[0] ?? null;
  if (autoDiagnosis && autoDiagnosis.policies_matched > 0 && snapshot) {
    return {
      actionLabel: "Open diagnosis",
      detail: "Automatic diagnosis matched this replay but did not start a room; open the retained snapshot for manual AI handoff.",
      href: diagnosisHref(snapshot.id),
      kind: "snapshot",
      label: "Review AI handoff",
      type: "warning"
    };
  }

  return {
    actionLabel: "Open reports",
    detail: `Replay accepted the report workflow; verify final report notification delivery once a report with correlation ${result.correlation_key} is available.`,
    href: "/reports" as Route,
    kind: "report",
    label: "Review report delivery",
    type: "info"
  };
}

function replayProofNextActionIcon(kind: AlertReplayProofNextAction["kind"]): ReactNode {
  switch (kind) {
    case "report":
      return <CheckCircleOutlined />;
    case "room":
    case "snapshot":
      return <MessageOutlined />;
    case "unavailable":
      return <WarningOutlined />;
  }
}

function replayTraceStatusColor(status: ReturnType<typeof reportReplayProofTrace>["status"]): string {
  switch (status) {
    case "ready":
      return "success";
    case "review":
      return "warning";
    case "pending":
      return "default";
    case "blocked":
      return "error";
  }
}

function replayTraceStatusLabel(status: ReturnType<typeof reportReplayProofTrace>["status"]): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "review":
      return "Review";
    case "pending":
      return "Pending";
    case "blocked":
      return "Blocked";
  }
}

function alertReplayRequest(alert: AlertEventSummary): ReportReplayTriggerRequest {
  const startsAt = parsedDate(alert.starts_at);
  const windowStart = new Date(startsAt.getTime() - alertReplayPaddingMs);
  const rawEnd = alert.ends_at ? parsedDate(alert.ends_at) : new Date(startsAt.getTime() + alertReplayOpenWindowMs);
  const paddedEnd = new Date(rawEnd.getTime() + alertReplayPaddingMs);
  const windowEnd = paddedEnd > windowStart ? paddedEnd : new Date(startsAt.getTime() + alertReplayOpenWindowMs);

  return {
    window_start: windowStart.toISOString(),
    window_end: windowEnd.toISOString(),
    limit: alertReplayLimit,
    scenario: "single_alert",
    correlation_key: `alert-replay-${alert.id}`
  };
}

function replayAcceptedMessage(result: ReportReplayTriggerResponse): string {
  if (result.snapshots.length === 0) {
    return "Replay completed with no evidence snapshots.";
  }
  const snapshotSummary = `${pluralizeCount(result.snapshots.length, "evidence snapshot")}`;
  const autoDiagnosis = result.auto_diagnosis;
  if (autoDiagnosis && autoDiagnosis.rooms_started > 0) {
    return `Replay accepted with ${snapshotSummary}; AI diagnosis started ${pluralizeCount(
      autoDiagnosis.rooms_started,
      "room"
    )}.`;
  }
  if (autoDiagnosis && autoDiagnosis.policies_matched > 0) {
    return `Replay accepted with ${snapshotSummary}; AI diagnosis matched ${pluralizeCount(
      autoDiagnosis.policies_matched,
      "policy"
    )} but started no rooms.`;
  }
  return `Replay accepted with ${snapshotSummary}.`;
}

function pluralizeCount(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

function parsedDate(value: string): Date {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
}

const alertReplayPaddingMs = 60 * 1000;
const alertReplayOpenWindowMs = 5 * 60 * 1000;
const alertReplayLimit = 1000;

function alertDescriptionItems(alert: AlertEventSummary): DescriptionsProps["items"] {
  return [
    {
      key: "canonical",
      label: "Canonical fingerprint",
      children: (
        <Typography.Text className="settings-event-ids" copyable>
          {alert.canonical_fingerprint}
        </Typography.Text>
      )
    },
    {
      key: "source",
      label: "Source fingerprint",
      children: (
        <Typography.Text className="settings-event-ids" copyable>
          {alert.source_fingerprint}
        </Typography.Text>
      )
    },
    {
      key: "started",
      label: "Started",
      children: formatDateTime(alert.starts_at)
    },
    {
      key: "ended",
      label: "Ended",
      children: alert.ends_at ? formatDateTime(alert.ends_at) : "Still firing"
    },
    {
      key: "source-name",
      label: "Source",
      children: alert.source
    },
    {
      key: "source-profile",
      label: "Alert source profile",
      children:
        alert.alert_source_profile_id > 0
          ? `#${alert.alert_source_profile_id}`
          : "legacy provider-only"
    },
    {
      key: "created",
      label: "Ingested",
      children: formatDateTime(alert.created_at)
    }
  ];
}

function sortedRecordEntries(record: Record<string, string>): Array<[string, string]> {
  return Object.entries(record)
    .filter(([, value]) => value.trim() !== "")
    .sort(([left], [right]) => left.localeCompare(right));
}

function defaultExpandedAlertKeys(alerts: AlertEventSummary[]): number[] {
  const defaultAlert = alerts.find((alert) => alert.status === "firing") ?? alerts[0];
  return defaultAlert ? [defaultAlert.id] : [];
}

function MetricCard({
  label,
  prefix,
  value
}: {
  label: string;
  prefix?: ReactNode;
  value: number;
}) {
  return (
    <Col lg={6} sm={12} xs={24}>
      <section className="metric-card">
        <Statistic prefix={prefix} title={label} value={value} />
      </section>
    </Col>
  );
}

function ErrorNotice({ message, status }: { message: string; status?: number }) {
  return (
    <Alert
      description={message}
      message={status ? `HTTP ${status}` : "Request failed"}
      role="alert"
      showIcon
      type="warning"
    />
  );
}

function severityColor(severity: string): string {
  switch (severity.toLowerCase()) {
    case "critical":
      return "red";
    case "warning":
      return "gold";
    default:
      return "blue";
  }
}

function snapshotStatusColor(status: AlertEvidenceSnapshotLink["status"]): string {
  switch (status) {
    case "complete":
      return "green";
    case "partial":
      return "gold";
    case "failed":
      return "red";
  }
}

function roomStatusColor(status: DiagnosisRoomSummary["room_status"]): string {
  return status === "open" ? "green" : "default";
}

function taskStatusColor(status: DiagnosisRoomSummary["task_status"]): string {
  switch (status) {
    case "succeeded":
      return "green";
    case "failed":
      return "red";
    case "cancelled":
      return "default";
    case "running":
      return "processing";
    case "pending":
      return "gold";
  }
}

function confidenceColor(confidence: string): string {
  switch (confidence.toLowerCase()) {
    case "high":
      return "green";
    case "medium":
      return "gold";
    case "low":
      return "red";
    default:
      return "blue";
  }
}
