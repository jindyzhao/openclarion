import Link from "next/link";

import type {
  DiagnosisHandoffListResponse,
  DiagnosisRoomListResponse,
  DiagnosisRoomNotificationTimelineEntry,
  DiagnosisRoomSummary
} from "@/features/diagnosis-room/api";
import type { AlertEventSummary } from "@/features/alerts/api";
import { diagnosisRoomNextStep } from "@/features/diagnosis-room/next-step";
import { ReportShell } from "@/features/reports/report-shell";
import { formatDateTime } from "@/features/reports/format";
import { notificationChannelEditHref } from "@/features/settings/notification-channels/format";
import type { ApiResult } from "@/lib/api/client";

import { formatCount, formatSuccessRate } from "./format";
import type { DashboardSummary } from "./api";

type DashboardViewProps = {
  handoffsResult: ApiResult<DiagnosisHandoffListResponse>;
  roomsResult: ApiResult<DiagnosisRoomListResponse>;
  result: ApiResult<DashboardSummary>;
};

export function DashboardView({ handoffsResult, result, roomsResult }: DashboardViewProps) {
  return (
    <ReportShell current="dashboard">
      <div className="page-heading">
        <div>
          <h1>Dashboard</h1>
          <p>Recent alert load and report delivery health.</p>
        </div>
        <Link className="status-line" href="/reports">
          View reports
        </Link>
      </div>

      {!result.ok ? <ErrorNotice message={result.error.message} status={result.error.status} /> : null}
      {result.ok ? <Summary dashboard={result.data} handoffsResult={handoffsResult} roomsResult={roomsResult} /> : null}
    </ReportShell>
  );
}

function Summary({
  dashboard,
  handoffsResult,
  roomsResult
}: {
  dashboard: DashboardSummary;
  handoffsResult: ApiResult<DiagnosisHandoffListResponse>;
  roomsResult: ApiResult<DiagnosisRoomListResponse>;
}) {
  return (
    <div className="stack">
      <section className="metric-grid" aria-label="Dashboard metrics">
        <MetricCard label="Firing alerts" value={formatCount(dashboard.alerts.firing)} />
        <MetricCard label="Recent alerts" value={formatCount(dashboard.alerts.total_recent)} />
        <MetricCard label="Report success" value={formatSuccessRate(dashboard.reports.success_rate)} />
        <MetricCard label="Recent reports" value={formatCount(dashboard.reports.total_recent)} />
      </section>

      <div className="dashboard-grid">
        <section className="panel">
          <div className="panel-header">
            <h2>Alert State</h2>
          </div>
          <div className="panel-body">
            <dl className="stat-list">
              <Stat label="Firing" value={dashboard.alerts.firing} />
              <Stat label="Resolved" value={dashboard.alerts.resolved} />
            </dl>
          </div>
        </section>

        <section className="panel">
          <div className="panel-header">
            <h2>Report Delivery</h2>
          </div>
          <div className="panel-body">
            <dl className="stat-list">
              <Stat label="Delivered" value={dashboard.reports.delivered} />
              <Stat label="Failed" value={dashboard.reports.failed} />
              <Stat label="Pending" value={dashboard.reports.pending} />
              <Stat label="No delivery row" value={dashboard.reports.missing_delivery} />
            </dl>
          </div>
        </section>

        <section className="panel">
          <div className="panel-header">
            <h2>Report Severity</h2>
          </div>
          <div className="panel-body">
            <dl className="stat-list">
              <Stat label="Critical" value={dashboard.reports.severity.critical} />
              <Stat label="Warning" value={dashboard.reports.severity.warning} />
              <Stat label="Info" value={dashboard.reports.severity.info} />
            </dl>
          </div>
        </section>

        <section className="panel">
          <div className="panel-header dashboard-panel-header">
            <h2>AI Handoff Backlog</h2>
            <Link className="status-line" href="/alerts">
              View alerts
            </Link>
          </div>
          <div className="panel-body">
            <AIHandoffBacklog result={handoffsResult} stats={dashboard.diagnosis} />
          </div>
        </section>

        <section className="panel dashboard-wide-panel">
          <div className="panel-header dashboard-panel-header">
            <h2>AI Diagnosis Rooms</h2>
            <Link className="status-line" href="/diagnosis-room">
              View all
            </Link>
          </div>
          <div className="panel-body">
            <DiagnosisRoomList result={roomsResult} />
          </div>
        </section>
      </div>
    </div>
  );
}

function AIHandoffBacklog({
  result,
  stats
}: {
  result: ApiResult<DiagnosisHandoffListResponse>;
  stats: DashboardSummary["diagnosis"];
}) {
  if (!result.ok) {
    if (result.error.status === 403) {
      return (
        <PermissionLimitedNotice
          detail="Your account can read operational dashboard metrics, but it cannot inspect or act on the diagnosis handoff backlog."
          title="AI handoff backlog is restricted"
        />
      );
    }
    return <ErrorNotice message={result.error.message} status={result.error.status} />;
  }
  if (stats.snapshots_needing_room === 0) {
    return (
      <div className="notice">
        <strong>No manual handoff backlog.</strong>
      </div>
    );
  }
  const backlog = result.data.items;
  return (
    <div className="dashboard-room-stack">
      <dl aria-label="AI handoff backlog health" className="stat-list">
        <Stat label="Snapshots needing room" value={stats.snapshots_needing_room} />
        <Stat label="Affected alerts" value={stats.affected_alerts_needing_room} />
        <Stat label="Rooms started" value={stats.rooms_started} />
      </dl>
      {backlog.length === 0 ? (
        <div className="notice">
          <strong>Backlog exists, but alert details are not available in the current list.</strong>
        </div>
      ) : (
        <ul aria-label="AI handoff backlog" className="dashboard-room-list">
          {backlog.map((item) => {
            const primaryAlert = item.alerts[0] ?? null;
            return (
              <li className="dashboard-room-item" key={item.evidence_snapshot.id}>
                <div className="dashboard-room-main">
                  <Link href={diagnosisSnapshotHref(item.evidence_snapshot.id)}>
                    Create room #{item.evidence_snapshot.id}
                  </Link>
                  <div className="muted">
                    {handoffAlertLabel(item.alerts)} | group #{item.evidence_snapshot.alert_group_id} |{" "}
                    {item.evidence_snapshot.created_by_workflow || "manual"}
                  </div>
                </div>
                <div className="dashboard-room-meta">
                  <span className={`pill ${severityPillClass(primaryAlert ? alertSeverity(primaryAlert) : "info")}`}>
                    {primaryAlert ? alertSeverity(primaryAlert) : "info"}
                  </span>
                  <span className="pill pill-warning">manual room</span>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function DiagnosisRoomList({ result }: { result: ApiResult<DiagnosisRoomListResponse> }) {
  if (!result.ok) {
    if (result.error.status === 403) {
      return (
        <PermissionLimitedNotice
          detail="Your account can read operational dashboard metrics, but it cannot read diagnosis room details."
          title="AI diagnosis rooms are restricted"
        />
      );
    }
    return <ErrorNotice message={result.error.message} status={result.error.status} />;
  }
  if (result.data.items.length === 0) {
    return (
      <div className="notice">
        <strong>No diagnosis rooms available.</strong>
      </div>
    );
  }
  const health = diagnosisRoomHealth(result.data.items);
  return (
    <div className="dashboard-room-stack">
      <dl aria-label="AI diagnosis room health" className="dashboard-room-health">
        <RoomHealthStat label="Attention" value={health.attention} />
        <RoomHealthStat label="Ready" value={health.ready} />
        <RoomHealthStat label="Active" value={health.active} />
        <RoomHealthStat label="Closed" value={health.closed} />
        <RoomHealthStat label="Notification failures" value={health.notificationFailures} />
      </dl>
      <ul aria-label="Recent diagnosis rooms" className="dashboard-room-list">
        {result.data.items.map((room) => {
          const step = diagnosisRoomNextStep(room);
          return (
            <li className="dashboard-room-item" key={room.chat_session_id}>
              <div className="dashboard-room-main">
                <Link href={diagnosisRoomHref(room)}>{room.session_id}</Link>
                <div className="muted">
                  task #{room.diagnosis_task_id} | evidence #{room.evidence_snapshot_id} | updated{" "}
                  {formatDateTime(room.updated_at)}
                </div>
                <div className="dashboard-room-next-step">
                  <span className={`pill ${statusPillClass(step.color)}`}>{step.label}</span>
                  <span className="muted">{step.detail}</span>
                </div>
                <DashboardRoomNotification room={room} />
              </div>
              <div className="dashboard-room-meta">
                <span className={room.room_status === "open" ? "pill pill-ok" : "pill pill-info"}>{room.room_status}</span>
                <span className="pill pill-info">{room.task_status}</span>
                {room.latest_conclusion ? (
                  <>
                    <span className="pill pill-ok">{room.latest_conclusion.status}</span>
                    {room.latest_conclusion.confidence ? (
                      <span className={`pill ${confidencePillClass(room.latest_conclusion.confidence)}`}>
                        {room.latest_conclusion.confidence}
                      </span>
                    ) : null}
                    {room.latest_conclusion.requires_human_review ? (
                      <span className="pill pill-warning">review</span>
                    ) : null}
                  </>
                ) : null}
                <span className="muted">{room.turn_count} turn(s)</span>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function RoomHealthStat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{formatCount(value)}</dd>
    </div>
  );
}

function DashboardRoomNotification({ room }: { room: DiagnosisRoomSummary }) {
  const notification = latestDiagnosisRoomNotification(room);
  if (!notification) {
    return (
      <div className="dashboard-room-notification muted">
        <span>No AI notification recorded</span>
      </div>
    );
  }
  const failed = notificationFailed(notification.provider_status);
  return (
    <div className="dashboard-room-notification">
      <span className={`pill ${statusPillClass(failed ? "error" : "success")}`}>
        {notificationEventLabel(notification.event_kind)}
      </span>
      <span className={`pill ${statusPillClass(failed ? "error" : "success")}`}>{notification.provider_status}</span>
      {notification.provider_message_id ? (
        <span className="muted">{notification.provider_message_id}</span>
      ) : null}
      {failed ? (
        <a className="status-line" href={notificationChannelReviewHref(notification)}>
          Review channel
        </a>
      ) : null}
    </div>
  );
}

function diagnosisRoomHealth(rooms: DiagnosisRoomSummary[]) {
  return rooms.reduce(
    (counts, room) => {
      const step = diagnosisRoomNextStep(room);
      counts[step.bucket] += 1;
      if (latestFailedDiagnosisRoomNotification(room)) {
        counts.notificationFailures += 1;
      }
      return counts;
    },
    { active: 0, attention: 0, closed: 0, notificationFailures: 0, ready: 0 }
  );
}

function latestDiagnosisRoomNotification(room: DiagnosisRoomSummary): DiagnosisRoomNotificationTimelineEntry | null {
  const timeline = room.notification_timeline ?? [];
  return timeline.length > 0 ? timeline[timeline.length - 1] ?? null : null;
}

function latestFailedDiagnosisRoomNotification(room: DiagnosisRoomSummary): DiagnosisRoomNotificationTimelineEntry | null {
  const notification = latestDiagnosisRoomNotification(room);
  return notification && notificationFailed(notification.provider_status) ? notification : null;
}

function notificationFailed(status: string): boolean {
  switch (status.toLowerCase()) {
    case "failed":
    case "error":
      return true;
    default:
      return false;
  }
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

function notificationChannelReviewHref(notification: DiagnosisRoomNotificationTimelineEntry): string {
  return notification.notification_channel_profile_id === undefined
    ? "/settings/notification-channels"
    : notificationChannelEditHref(notification.notification_channel_profile_id);
}

function statusPillClass(color: string) {
  switch (color.toLowerCase()) {
    case "success":
      return "pill-ok";
    case "warning":
    case "gold":
      return "pill-warning";
    case "error":
      return "pill-critical";
    case "processing":
      return "pill-info";
    default:
      return "pill-info";
  }
}

function diagnosisRoomHref(room: DiagnosisRoomSummary) {
  return {
    pathname: "/diagnosis-room",
    query: {
      evidence_snapshot_id: String(room.evidence_snapshot_id),
      session_id: room.session_id
    }
  };
}

function diagnosisSnapshotHref(snapshotID: number) {
  return {
    pathname: "/diagnosis-room",
    query: {
      evidence_snapshot_id: String(snapshotID),
      intent: "alert_review"
    }
  };
}

function handoffAlertLabel(alerts: AlertEventSummary[]): string {
  const first = alerts[0];
  if (!first) {
    return "No linked alert";
  }
  const suffix = alerts.length > 1 ? ` + ${alerts.length - 1} more` : "";
  return `${alertName(first)}${suffix}`;
}

function alertName(alert: AlertEventSummary): string {
  return alert.labels.alertname || alert.labels.alert_name || `Alert #${alert.id}`;
}

function alertSeverity(alert: AlertEventSummary): string {
  const value = alert.labels.severity || alert.labels.priority || "info";
  return value.trim() === "" ? "info" : value;
}

function severityPillClass(severity: string): string {
  switch (severity.toLowerCase()) {
    case "critical":
      return "pill-critical";
    case "warning":
      return "pill-warning";
    case "info":
      return "pill-info";
    default:
      return "pill-info";
  }
}

function confidencePillClass(confidence: string) {
  switch (confidence.toLowerCase()) {
    case "high":
      return "pill-ok";
    case "medium":
      return "pill-warning";
    case "low":
      return "pill-critical";
    default:
      return "pill-info";
  }
}

function MetricCard({ label, value }: { label: string; value: string }) {
  return (
    <section className="metric-card">
      <div className="metric-value">{value}</div>
      <div className="metric-label">{label}</div>
    </section>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{formatCount(value)}</dd>
    </div>
  );
}

function ErrorNotice({ message, status }: { message: string; status?: number }) {
  return (
    <div className="notice">
      <strong>{status ? `HTTP ${status}` : "Request failed"}</strong>
      <div>{message}</div>
    </div>
  );
}

function PermissionLimitedNotice({ detail, title }: { detail: string; title: string }) {
  return (
    <div className="notice">
      <strong>{title}</strong>
      <div>{detail}</div>
    </div>
  );
}
