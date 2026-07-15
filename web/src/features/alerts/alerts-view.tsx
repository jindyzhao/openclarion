"use client";

import { CheckCircleOutlined, ExperimentOutlined, LinkOutlined, MessageOutlined, WarningOutlined } from "@ant-design/icons";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Alert, App as AntdApp, Button, Col, Descriptions, Empty, Row, Space, Statistic, Table, Tag, Typography } from "antd";
import type { DescriptionsProps, TableColumnsType } from "antd";
import type { Route } from "next";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState, type ReactNode } from "react";

import type { DiagnosisRoomSummary } from "@/features/diagnosis-room/api";
import { diagnosisRoomNextStep } from "@/features/diagnosis-room/next-step";
import { localizeDiagnosisRoomNextStep } from "@/features/diagnosis-room/next-step-copy";
import {
  diagnosisNotificationContentProofDisplay,
  diagnosisNotificationDeliveryCoverage,
  diagnosisNotificationDeliveryCoveragePhaseColor,
} from "@/features/diagnosis-room/notification-content-proof";
import { diagnosisRoomLinkHref } from "@/features/diagnosis-room/url-state";
import { localizeDiagnosisRoomStatus } from "@/features/diagnosis-room/status-copy";
import type { ReplayProofTraceStatus } from "@/features/report-replay/proof-trace";
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
  localizeAlertDiagnosisClosure,
  localizeAlertDiagnosisDeliveryReviewAction,
  localizeAlertDiagnosisEvidenceAction,
  localizeAlertDiagnosisEvidenceProgress,
  localizeAlertDiagnosisRoomPrimaryAction,
  localizeDiagnosisNotificationContentProof,
  localizeDiagnosisNotificationCoverage,
} from "./alerts-copy";
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
import {
  alertReplayProofNextAction,
  localizeAlertReplayProofNextAction,
  localizeReplayAcceptedMessage,
  localizeReportReplayProofTrace,
  type AlertReplayProofNextAction,
} from "./replay-copy";

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
  const t = useTranslations("Alerts");
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
      message.success(localizeReplayAcceptedMessage(result, t));
      router.refresh();
    },
    onError: (error) => {
      message.error(error.message);
    }
  });

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{t("title")}</h1>
          <p>{t("subtitle")}</p>
        </div>
        {alertsResult.ok ? (
          <Typography.Text className="status-line">
            {t("loaded", { count: loadedCount })}
          </Typography.Text>
        ) : null}
      </section>

      {!alertsResult.ok ? <ErrorNotice message={alertsResult.error.message} status={alertsResult.error.status} /> : null}

      {alertsResult.ok ? <AlertSummary alerts={alertsResult.data.items} /> : null}

      {latestReplayProof === null ? null : <AlertReplayProofPanel proof={latestReplayProof} />}

      {alertsResult.ok && alertsResult.data.items.length === 0 ? (
        <Empty description={t("noAlerts")} image={<WarningOutlined aria-hidden className="settings-empty-icon" />} />
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
    </>
  );
}

function AlertReplayProofPanel({ proof }: { proof: AlertReplayProof }) {
  const t = useTranslations("Alerts");
  const trace = localizeReportReplayProofTrace(
    proof.result,
    t,
  );
  const rooms = proof.result.auto_diagnosis?.rooms ?? [];
  const nextAction = alertReplayProofNextAction(proof.result);
  const nextActionCopy = localizeAlertReplayProofNextAction(nextAction, t);

  return (
    <section aria-label={t("latestReplayProofLabel")} className="alert-replay-proof panel">
      <div className="panel-header">
        <h2>{t("latestReplayProof")}</h2>
        <Space size={[6, 6]} wrap>
          <Tag>{alertName(proof.alert, t)}</Tag>
          <Tag color={replayTraceStatusColor(trace.status)}>
            {replayTraceStatusLabel(trace.status, t)}
          </Tag>
        </Space>
      </div>
      <div className="panel-body">
        <Typography.Text type="secondary">{trace.detail}</Typography.Text>
        <Typography.Text className="settings-event-ids" copyable type="secondary">
          {t("correlation", { key: proof.result.correlation_key })}
        </Typography.Text>
        <Alert
          className="alert-replay-next-action"
          description={
            <Space className="alert-replay-next-action-body" size={[8, 8]} wrap>
              <Typography.Text>{nextActionCopy.detail}</Typography.Text>
              {nextAction.href ? (
                <Button href={nextAction.href} icon={replayProofNextActionIcon(nextAction.kind)} size="small" type="primary">
                  {nextActionCopy.actionLabel}
                </Button>
              ) : null}
            </Space>
          }
          message={nextActionCopy.label}
          showIcon
          type={nextAction.type}
        />
        <div className="workflow-automation-grid">
          {trace.items.map((item) => (
            <div className="workflow-automation-item" key={item.title}>
              <div className="workflow-automation-item-header">
                <Typography.Text className="muted">{item.title}</Typography.Text>
                <Tag color={replayTraceStatusColor(item.status)}>
                  {replayTraceStatusLabel(item.status, t)}
                </Tag>
              </div>
              <Typography.Text strong>{item.value}</Typography.Text>
              <Typography.Text type="secondary">{item.detail}</Typography.Text>
              {item.actions && item.actions.length > 0 ? (
                <Space size={[4, 4]} wrap>
                  {item.actions.map((action) => (
                    <Button
                      href={action.href}
                      key={`${item.key}:${action.href}`}
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
          <Space aria-label={t("replayProofLinks")} size={[6, 6]} wrap>
            {proof.result.snapshots.map((snapshot) => (
              <Button href={diagnosisHref(snapshot.id)} icon={<ExperimentOutlined />} key={snapshot.id} size="small">
                {t("snapshot", { id: snapshot.id })}
              </Button>
            ))}
            {rooms.map((room) => (
              <Button href={replayDiagnosisRoomHref(room)} icon={<MessageOutlined />} key={room.session_id} size="small" type="primary">
                {t("room", { id: room.evidence_snapshot_id })}
              </Button>
            ))}
          </Space>
        ) : null}
      </div>
    </section>
  );
}

function AlertSummary({ alerts }: { alerts: AlertEventSummary[] }) {
  const t = useTranslations("Alerts");
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
    <Row aria-label={t("alertSummary")} className="alert-summary-grid" gutter={[12, 12]}>
      <MetricCard label={t("firing")} prefix={<WarningOutlined />} value={firing} />
      <MetricCard label={t("resolved")} value={resolved} />
      <MetricCard label={t("linkedAlerts")} prefix={<LinkOutlined />} value={linked} />
      <MetricCard label={t("aiRooms")} prefix={<MessageOutlined />} value={rooms} />
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
  const locale = useLocale();
  const t = useTranslations("Alerts");
  const columns: TableColumnsType<AlertEventSummary> = [
    {
      key: "alert",
      render: (_value, alert) => (
        <Space className="alert-meta-stack" direction="vertical" size={6}>
          <Typography.Text strong>{alertName(alert, t)}</Typography.Text>
          <Typography.Text type="secondary">{alertSummary(alert)}</Typography.Text>
          <LabelTags alert={alert} />
        </Space>
      ),
      title: t("alert"),
      width: 320
    },
    {
      key: "state",
      render: (_value, alert) => (
        <Space direction="vertical" size={6}>
          <Tag color={alert.status === "firing" ? "red" : "green"}>
            {localizeAlertStatus(alert.status, t)}
          </Tag>
          <Tag color={severityColor(alertSeverity(alert))}>
            {localizeAlertStatus(alertSeverity(alert), t)}
          </Tag>
        </Space>
      ),
      title: t("state"),
      width: 130
    },
    {
      key: "source",
      render: (_value, alert) => (
        <Space direction="vertical" size={4}>
          <Typography.Text strong>{alert.source}</Typography.Text>
          {alert.alert_source_profile_id > 0 ? (
            <Tag>{t("profile", { id: alert.alert_source_profile_id })}</Tag>
          ) : null}
          <Typography.Text className="settings-event-ids" copyable type="secondary">
            {alert.source_fingerprint}
          </Typography.Text>
        </Space>
      ),
      title: t("source"),
      width: 210
    },
    {
      key: "started",
      render: (_value, alert) => (
        <Space direction="vertical" size={4}>
          <Typography.Text>{formatDateTime(alert.starts_at, locale)}</Typography.Text>
          {alert.ends_at ? (
            <Typography.Text type="secondary">
              {t("endedAt", { time: formatDateTime(alert.ends_at, locale) })}
            </Typography.Text>
          ) : null}
        </Space>
      ),
      title: t("started"),
      width: 190
    },
    {
      key: "evidence",
      render: (_value, alert) => <SnapshotLinks linkedSnapshots={alert.linked_evidence_snapshots} />,
      title: t("evidence"),
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
      title: t("aiDiagnosis"),
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
      title: t("actions"),
      width: 170
    }
  ];

  return (
    <section aria-label={t("recentAlertsLabel")} className="alerts-table-panel">
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
  const locale = useLocale();
  const t = useTranslations("Alerts");
  return (
    <section
      aria-label={t("alertDetails", { name: alertName(alert, t) })}
      className="alert-detail-panel"
    >
      <Descriptions
        column={{ xs: 1, md: 2 }}
        items={alertDescriptionItems(alert, locale, t)}
        size="small"
      />
      <div className="alert-detail-grid">
        <KeyValueBlock
          emptyText={t("noLabels")}
          entries={sortedRecordEntries(alert.labels)}
          title={t("labels")}
        />
        <KeyValueBlock
          emptyText={t("noAnnotations")}
          entries={sortedRecordEntries(alert.annotations)}
          title={t("annotations")}
        />
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
  const t = useTranslations("Alerts");
  const tStatus = useTranslations("DiagnosisRoom.status");
  return (
    <section className="alert-detail-block">
      <div className="alert-detail-block-header">
        <Typography.Title level={3}>{t("evidenceHandoff")}</Typography.Title>
        <Tag color={linkedSnapshots.length > 0 ? "success" : "warning"}>
          {t("snapshotCount", { count: linkedSnapshots.length })}
        </Tag>
      </div>
      {linkedSnapshots.length === 0 ? (
        <Alert
          description={t("evidencePendingDetail")}
          message={t("evidencePendingTitle")}
          showIcon
          type="warning"
        />
      ) : (
        <div className="alert-detail-snapshot-list">
          {linkedSnapshots.map((snapshot) => (
            <section className="alert-detail-snapshot" key={snapshot.id}>
              <div className="alert-detail-snapshot-header">
                <Space size={[6, 6]} wrap>
                  <Typography.Text strong>{t("snapshot", { id: snapshot.id })}</Typography.Text>
                  <Tag color={snapshotStatusColor(snapshot.status)}>
                    {localizeDiagnosisRoomStatus(snapshot.status, tStatus)}
                  </Tag>
                  <Tag>{t("group", { id: snapshot.alert_group_id })}</Tag>
                </Space>
                <Button href={diagnosisHref(snapshot.id)} icon={<MessageOutlined />} size="small" type="link">
                  {t("openDiagnosis")}
                </Button>
              </div>
              <Typography.Text type="secondary">
                {snapshot.created_by_workflow || t("manual")}
              </Typography.Text>
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
                    detail={t("retainedNoRoom", { id: snapshot.id })}
                    label={t("manualRoomNeeded")}
                  />
                  <Typography.Text type="secondary">
                    {t("openToCreateRoom")}
                  </Typography.Text>
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
  const t = useTranslations("Alerts");
  const tStatus = useTranslations("DiagnosisRoom.status");
  if (linkedSnapshots.length === 0) {
    return <Typography.Text type="secondary">{t("noSnapshot")}</Typography.Text>;
  }
  return (
    <Space aria-label={t("linkedSnapshots")} direction="vertical" size={8}>
      {linkedSnapshots.map((snapshot) => (
        <Space direction="vertical" key={snapshot.id} size={4}>
          <Link href={diagnosisHref(snapshot.id)}>
            {t("snapshot", { id: snapshot.id })}
          </Link>
          <Space size={[6, 6]} wrap>
            <Tag color={snapshotStatusColor(snapshot.status)}>
              {localizeDiagnosisRoomStatus(snapshot.status, tStatus)}
            </Tag>
            <Tag icon={<ExperimentOutlined />}>
              {t("group", { id: snapshot.alert_group_id })}
            </Tag>
          </Space>
          <Typography.Text type="secondary">
            {snapshot.created_by_workflow || t("manual")}
          </Typography.Text>
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
  const t = useTranslations("Alerts");
  if (linkedSnapshots.length === 0) {
    return <Typography.Text type="secondary">{t("awaitingEvidence")}</Typography.Text>;
  }

  return (
    <Space aria-label={t("linkedRooms")} direction="vertical" size={8}>
      {linkedSnapshots.map((snapshot) => {
        if (snapshot.diagnosis_rooms.length === 0) {
          return (
            <Space direction="vertical" key={snapshot.id} size={4}>
              <Link href={diagnosisHref(snapshot.id)}>{t("openDiagnosis")}</Link>
              <DiagnosisNextStep
                color="warning"
                detail={t("createRoomDetail", { id: snapshot.id })}
                label={t("createRoom")}
              />
              <Typography.Text type="secondary">
                {t("evidenceNumber", { id: snapshot.id })}
              </Typography.Text>
            </Space>
          );
        }
        return snapshot.diagnosis_rooms.map((room) => (
          <Space className="alert-room-link" direction="vertical" key={room.chat_session_id} size={4}>
            <Space size={[8, 6]} wrap>
              <Link href={diagnosisRoomHref(room)}>{room.session_id}</Link>
              {room.latest_conclusion ? (
                <Link href={reviewDiagnosisRoomHref(room)}>
                  {t("reviewConclusion")}
                </Link>
              ) : null}
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
  const t = useTranslations("Alerts");
  const tStatus = useTranslations("DiagnosisRoom.status");
  return (
    <Space className="alert-room-meta" size={[6, 6]} wrap>
      <Tag color={roomStatusColor(room.room_status)}>
        {localizeDiagnosisRoomStatus(room.room_status, tStatus)}
      </Tag>
      <Tag color={taskStatusColor(room.task_status)}>
        {localizeDiagnosisRoomStatus(room.task_status, tStatus)}
      </Tag>
      {room.latest_conclusion ? (
        <>
          <Tag color="success">
            {localizeDiagnosisRoomStatus(room.latest_conclusion.status, tStatus)}
          </Tag>
          {room.latest_conclusion.confidence ? (
            <Tag color={confidenceColor(room.latest_conclusion.confidence)}>
              {localizeDiagnosisRoomStatus(
                room.latest_conclusion.confidence,
                tStatus,
              )}
            </Tag>
          ) : null}
          {room.latest_conclusion.requires_human_review ? (
            <Tag color="warning">{t("review")}</Tag>
          ) : null}
        </>
      ) : null}
      {!room.latest_conclusion && room.latest_progress ? <ProgressTags room={room} /> : null}
      <Typography.Text type="secondary">
        {t("turns", { count: room.turn_count })}
      </Typography.Text>
    </Space>
  );
}

function RoomCloseSummary({ room }: { room: DiagnosisRoomSummary }) {
  const locale = useLocale();
  const t = useTranslations("Alerts");
  if (room.room_status !== "closed") {
    return null;
  }
  const closeParts = [
    room.closed_at
      ? t("closedAt", { time: formatDateTime(room.closed_at, locale) })
      : t("closed"),
    room.close_reason
      ? t("closeReason", { reason: room.close_reason })
      : t("noCloseReason")
  ];
  return (
    <Typography.Text className="alert-room-close-summary" type="secondary">
      {closeParts.join(" / ")}
    </Typography.Text>
  );
}

function NotificationDeliverySummary({ room }: { room: DiagnosisRoomSummary }) {
  const locale = useLocale();
  const t = useTranslations("Alerts");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const latest = latestDiagnosisRoomNotification(room);
  if (!latest) {
    return (
      <Typography.Text className="alert-notification-summary" type="secondary">
        {t("noNotification")}
      </Typography.Text>
    );
  }
  const failed = diagnosisRoomNotificationFailed(latest.provider_status);
  const proof = diagnosisNotificationContentProofDisplay(latest);
  const proofCopy = localizeDiagnosisNotificationContentProof(
    latest,
    proof,
    locale,
    t,
  );
  const deliveryCoverage = diagnosisNotificationDeliveryCoverage(room.notification_timeline ?? []);
  const deliveryCopy = localizeDiagnosisNotificationCoverage(
    deliveryCoverage,
    t,
  );
  const reviewAction = alertDiagnosisDeliveryReviewAction(room);
  return (
    <Space
      aria-label={t("notificationDelivery", { session: room.session_id })}
      className="alert-notification-summary"
      size={[6, 6]}
      wrap
    >
      <Tag color={notificationStatusColor(latest.provider_status)}>
        {notificationEventLabel(latest.event_kind, t)}
      </Tag>
      <Tag color={notificationStatusColor(latest.provider_status)}>
        {localizeDiagnosisRoomStatus(latest.provider_status, tStatus)}
      </Tag>
      <Tag color={proof.color}>{proofCopy.label}</Tag>
      <Tag color={deliveryCoverage.color}>{deliveryCopy.label}</Tag>
      {deliveryCoverage.phases.map((phase, index) => (
        <Tag color={diagnosisNotificationDeliveryCoveragePhaseColor(phase.status)} key={phase.key}>
          {t("notificationPhaseStatus", {
            phase: deliveryCopy.phases[index]?.label ?? phase.label,
            status: deliveryCopy.phases[index]?.status ?? phase.status,
          })}
        </Tag>
      ))}
      {latest.confidence ? (
        <Tag color={confidenceColor(latest.confidence)}>
          {localizeDiagnosisRoomStatus(latest.confidence, tStatus)}
        </Tag>
      ) : null}
      {latest.requires_human_review ? <Tag color="warning">{t("review")}</Tag> : null}
      <Typography.Text type="secondary">
        {formatDateTime(latest.occurred_at, locale)}
      </Typography.Text>
      <Typography.Text type={deliveryCoverage.status === "blocked" ? "danger" : "secondary"}>
        {deliveryCopy.detail}
      </Typography.Text>
      {proof.hasProof ? (
        <Typography.Text type="secondary">{proofCopy.detail}</Typography.Text>
      ) : null}
      {latest.provider_message_id ? (
        <Typography.Text className="settings-event-ids" type="secondary">
          {latest.provider_message_id}
        </Typography.Text>
      ) : null}
      {failed ? (
        <Button href={diagnosisRoomNotificationChannelReviewHref(latest)} size="small" type="link">
          {t("reviewChannel")}
        </Button>
      ) : null}
      {reviewAction && !failed ? (
        <Button danger={reviewAction.danger} href={reviewAction.href} size="small" type="link">
          {localizeAlertDiagnosisDeliveryReviewAction(reviewAction, t).label}
        </Button>
      ) : null}
    </Space>
  );
}

function DiagnosisEvidenceRequestActions({ room }: { room: DiagnosisRoomSummary }) {
  const t = useTranslations("Alerts");
  const actions = alertDiagnosisEvidenceActions(room);
  if (actions.length === 0) {
    return null;
  }
  const visibleActions = actions.slice(0, 3);
  const remaining = actions.length - visibleActions.length;
  return (
    <Space
      aria-label={t("evidenceRequests", { session: room.session_id })}
      className="alert-evidence-actions"
      size={[6, 6]}
      wrap
    >
      <Typography.Text type="secondary">{t("requestedEvidence")}</Typography.Text>
      {visibleActions.map((action) => (
        <Space key={action.key} size={4}>
          <Button href={action.href} size="small" type="link">
            {localizeAlertDiagnosisEvidenceAction(action, t)} {action.label}
          </Button>
          <Tag color={action.kind === "executable" ? "processing" : "warning"}>
            {action.kind === "executable" ? action.tool : action.priority}
          </Tag>
        </Space>
      ))}
      {remaining > 0 ? <Tag>{t("more", { count: remaining })}</Tag> : null}
    </Space>
  );
}

function DiagnosisEvidenceProgressSummary({ room }: { room: DiagnosisRoomSummary }) {
  const locale = useLocale();
  const t = useTranslations("Alerts");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const summary = alertDiagnosisEvidenceProgressSummary(room);
  if (summary === null) {
    return null;
  }
  const summaryCopy = localizeAlertDiagnosisEvidenceProgress(
    summary,
    locale,
    t,
    tStatus,
  );
  return (
    <Space
      aria-label={t("evidenceProgress", { session: room.session_id })}
      className="alert-evidence-progress"
      size={[6, 6]}
      wrap
    >
      <Typography.Text type="secondary">{t("reviewProgress")}</Typography.Text>
      <Tag color={confidenceColor(summary.latestConfidence)}>
        {summaryCopy.confidenceLabel}
      </Tag>
      <Tag color={summary.openEvidence > 0 ? "warning" : "success"}>
        {summaryCopy.evidenceLabel}
      </Tag>
      <Typography.Text type="secondary">{summaryCopy.detail}</Typography.Text>
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
  const t = useTranslations("Alerts");
  const summary = alertDiagnosisClosureSummary(room);
  if (summary === null) {
    return null;
  }
  const summaryCopy = localizeAlertDiagnosisClosure(summary, t);
  return (
    <Space
      aria-label={t("closureStatus", { session: room.session_id })}
      className="alert-closure-summary"
      size={[6, 6]}
      wrap
    >
      <Typography.Text type="secondary">{t("aiClosure")}</Typography.Text>
      <Tag color={summary.traceability.color}>{summaryCopy.label}</Tag>
      {summary.confirmedBy === "" ? (
        <Tag color="warning">{t("confirmationNeeded")}</Tag>
      ) : (
        <DirectorySubjectTags
          directoryUsersBySubject={directoryUsersBySubject}
          label={t("confirmedBy")}
          subject={summary.confirmedBy}
        />
      )}
      <Tag color={summary.roomClosed ? "success" : "default"}>
        {summary.roomClosed ? t("roomClosed") : t("roomOpen")}
      </Tag>
      <Tag color={diagnosisNotificationDeliveryCoveragePhaseColor(summary.closeProofStatus)}>
        {summaryCopy.closeProofLabel}
      </Tag>
      <Typography.Text type="secondary">{summaryCopy.detail}</Typography.Text>
    </Space>
  );
}

function ProgressTags({ room }: { room: DiagnosisRoomSummary }) {
  const t = useTranslations("Alerts");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const progress = room.latest_progress;
  if (!progress) {
    return null;
  }
  const missingCount = progress.missing_evidence_requests?.length ?? 0;
  return (
    <>
      <Tag color={progress.conclusion_status === "needs_evidence" ? "warning" : "processing"}>
        {localizeDiagnosisRoomStatus(
          progress.conclusion_status || progress.status,
          tStatus,
        )}
      </Tag>
      <Tag color={confidenceColor(progress.confidence)}>
        {localizeDiagnosisRoomStatus(progress.confidence, tStatus)}
      </Tag>
      {missingCount > 0 ? (
        <Tag color="warning">{t("missing", { count: missingCount })}</Tag>
      ) : null}
      {progress.requires_human_review ? <Tag color="warning">{t("review")}</Tag> : null}
    </>
  );
}

function notificationEventLabel(
  eventKind: string,
  t: ReturnType<typeof useTranslations<"Alerts">>,
): string {
  switch (eventKind) {
    case "diagnosis_room.assistant_turn_notification_sent":
      return t("aiUpdateNotification");
    case "diagnosis_room.final_ready_notification_sent":
      return t("finalReadyNotification");
    case "diagnosis_room.close_notification_sent":
      return t("closeNotification");
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
  const locale = useLocale();
  const t = useTranslations("Alerts");
  const tNextStep = useTranslations("DiagnosisRoom.nextStep");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const snapshot = alert.linked_evidence_snapshots[0];
  if (!snapshot) {
    return (
      <Space className="alert-action-stack" direction="vertical" size={4}>
        <Button
          aria-label={t("replayWindowLabel", { name: alertName(alert, t) })}
          disabled={!clientReady || replaying}
          icon={<ExperimentOutlined />}
          loading={replaying}
          onClick={() => onReplayAlert(alert)}
          size="small"
        >
          {t("replayWindow")}
        </Button>
        <Typography.Text type="secondary">{t("replayWindowHint")}</Typography.Text>
      </Space>
    );
  }
  const rooms = alert.linked_evidence_snapshots.flatMap((item) => item.diagnosis_rooms);
  const action = alertDiagnosisRoomPrimaryAction(rooms);
  if (!action) {
    return (
      <Space className="alert-action-stack" direction="vertical" size={4}>
        <Button href={diagnosisHref(snapshot.id)} icon={<MessageOutlined />} size="small" type="primary">
          {t("openDiagnosis")}
        </Button>
        <Typography.Text type="secondary">{t("openDiagnosisHint")}</Typography.Text>
      </Space>
    );
  }
  const actionCopy = localizeAlertDiagnosisRoomPrimaryAction(
    action,
    locale,
    t,
    tNextStep,
    tStatus,
  );
  return (
    <Space className="alert-action-stack" direction="vertical" size={4}>
      <Button
        danger={action.danger}
        href={action.href}
        icon={primaryDiagnosisRoomActionIcon(action.iconKind)}
        size="small"
        type="primary"
      >
        {actionCopy.label}
      </Button>
      <Typography.Text type="secondary">{actionCopy.hint}</Typography.Text>
    </Space>
  );
}

function DiagnosisRoomNextStep({ room }: { room: DiagnosisRoomSummary }) {
  const locale = useLocale();
  const tNextStep = useTranslations("DiagnosisRoom.nextStep");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const step = diagnosisRoomNextStep(room);
  const copy = localizeDiagnosisRoomNextStep(
    step,
    locale,
    tNextStep,
    tStatus,
  );
  return <DiagnosisNextStep color={step.color} detail={copy.detail} label={copy.label} />;
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

function alertName(
  alert: AlertEventSummary,
  t: ReturnType<typeof useTranslations<"Alerts">>,
): string {
  return (
    alert.labels.alertname ||
    alert.labels.alert_name ||
    t("alertNumber", { id: alert.id })
  );
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

function replayTraceStatusColor(status: ReplayProofTraceStatus): string {
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

function replayTraceStatusLabel(
  status: ReplayProofTraceStatus,
  t: ReturnType<typeof useTranslations<"Alerts">>,
): string {
  switch (status) {
    case "ready":
      return t("replayStatus.ready");
    case "review":
      return t("replayStatus.review");
    case "pending":
      return t("replayStatus.pending");
    case "blocked":
      return t("replayStatus.blocked");
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
    alert_event_id: alert.id,
    scenario: "single_alert",
    correlation_key: `alert-replay-${alert.id}`
  };
}

function parsedDate(value: string): Date {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
}

const alertReplayPaddingMs = 60 * 1000;
const alertReplayOpenWindowMs = 5 * 60 * 1000;
const alertReplayLimit = 1000;

function alertDescriptionItems(
  alert: AlertEventSummary,
  locale: string,
  t: ReturnType<typeof useTranslations<"Alerts">>,
): DescriptionsProps["items"] {
  return [
    {
      key: "canonical",
      label: t("canonicalFingerprint"),
      children: (
        <Typography.Text className="settings-event-ids" copyable>
          {alert.canonical_fingerprint}
        </Typography.Text>
      )
    },
    {
      key: "source",
      label: t("sourceFingerprint"),
      children: (
        <Typography.Text className="settings-event-ids" copyable>
          {alert.source_fingerprint}
        </Typography.Text>
      )
    },
    {
      key: "started",
      label: t("started"),
      children: formatDateTime(alert.starts_at, locale)
    },
    {
      key: "ended",
      label: t("ended"),
      children: alert.ends_at
        ? formatDateTime(alert.ends_at, locale)
        : t("stillFiring")
    },
    {
      key: "source-name",
      label: t("source"),
      children: alert.source
    },
    {
      key: "source-profile",
      label: t("alertSourceProfile"),
      children:
        alert.alert_source_profile_id > 0
          ? `#${alert.alert_source_profile_id}`
          : t("legacyProvider")
    },
    {
      key: "created",
      label: t("ingested"),
      children: formatDateTime(alert.created_at, locale)
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
  const t = useTranslations("Alerts");
  return (
    <Alert
      description={message}
      message={status ? `HTTP ${status}` : t("requestFailed")}
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

function localizeAlertStatus(
  value: string,
  t: ReturnType<typeof useTranslations<"Alerts">>,
): string {
  switch (value.trim().toLowerCase()) {
    case "critical":
      return t("status.critical");
    case "firing":
      return t("status.firing");
    case "info":
      return t("status.info");
    case "resolved":
      return t("status.resolved");
    case "warning":
      return t("status.warning");
    default:
      return value;
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
