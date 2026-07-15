import Link from "next/link";
import type { Route } from "next";
import { useLocale, useTranslations } from "next-intl";

import type {
  DiagnosisHandoffListResponse,
  DiagnosisRoomListResponse,
  DiagnosisRoomNotificationTimelineEntry,
  DiagnosisRoomSummary
} from "@/features/diagnosis-room/api";
import type { AlertEventSummary } from "@/features/alerts/api";
import {
  diagnosisRoomNextStep,
  latestFailedDiagnosisRoomNotification
} from "@/features/diagnosis-room/next-step";
import { localizeDiagnosisRoomNextStep } from "@/features/diagnosis-room/next-step-copy";
import { localizeDiagnosisRoomStatus } from "@/features/diagnosis-room/status-copy";
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
  const t = useTranslations("Dashboard");
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>{t("title")}</h1>
          <p>{t("subtitle")}</p>
        </div>
        <Link className="status-line" href="/reports">
          {t("viewReports")}
        </Link>
      </div>

      {!result.ok ? <ErrorNotice message={result.error.message} status={result.error.status} /> : null}
      {result.ok ? <Summary dashboard={result.data} handoffsResult={handoffsResult} roomsResult={roomsResult} /> : null}
    </>
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
  const locale = useLocale();
  const t = useTranslations("Dashboard");
  return (
    <div className="overview-stack">
      <section className="overview-metrics" aria-label={t("metricsLabel")}>
        <MetricCard href="/alerts" label={t("firingAlerts")} value={formatCount(dashboard.alerts.firing, locale)} />
        <MetricCard href="/alerts" label={t("recentAlerts")} value={formatCount(dashboard.alerts.total_recent, locale)} />
        <MetricCard
          href="/reports"
          label={t("reportSuccess")}
          value={
            formatSuccessRate(dashboard.reports.success_rate, locale) ??
            t("notAvailable")
          }
        />
        <MetricCard href="/reports" label={t("recentReports")} value={formatCount(dashboard.reports.total_recent, locale)} />
      </section>

      <div className="overview-primary-grid">
        <section className="panel">
          <div className="panel-header dashboard-panel-header">
            <h2>{t("operationalHealth")}</h2>
            <Link className="status-line" href="/alerts">
              {t("inspectAlerts")}
            </Link>
          </div>
          <div className="overview-health-groups">
            <div className="overview-health-group">
              <h3>{t("alertState")}</h3>
              <dl className="stat-list">
                <Stat label={t("firing")} value={dashboard.alerts.firing} locale={locale} />
                <Stat label={t("resolved")} value={dashboard.alerts.resolved} locale={locale} />
              </dl>
            </div>
            <div className="overview-health-group">
              <h3>{t("reportDelivery")}</h3>
              <dl className="stat-list">
                <Stat label={t("delivered")} value={dashboard.reports.delivered} locale={locale} />
                <Stat label={t("failed")} value={dashboard.reports.failed} locale={locale} />
                <Stat label={t("pending")} value={dashboard.reports.pending} locale={locale} />
                <Stat label={t("noDeliveryRow")} value={dashboard.reports.missing_delivery} locale={locale} />
              </dl>
            </div>
            <div className="overview-health-group">
              <h3>{t("reportSeverity")}</h3>
              <dl className="stat-list">
                <Stat label={t("critical")} value={dashboard.reports.severity.critical} locale={locale} />
                <Stat label={t("warning")} value={dashboard.reports.severity.warning} locale={locale} />
                <Stat label={t("info")} value={dashboard.reports.severity.info} locale={locale} />
              </dl>
            </div>
          </div>
        </section>

        <section className="panel overview-handoff-panel">
          <div className="panel-header dashboard-panel-header">
            <h2>{t("handoffBacklog")}</h2>
            <Link className="status-line" href="/alerts">
              {t("viewAlerts")}
            </Link>
          </div>
          <div className="panel-body">
            <AIHandoffBacklog result={handoffsResult} stats={dashboard.diagnosis} />
          </div>
        </section>
      </div>

      <section className="panel overview-diagnosis-panel">
        <div className="panel-header dashboard-panel-header">
          <h2>{t("diagnosisRooms")}</h2>
          <Link className="status-line" href="/diagnosis-room">
            {t("openWorkQueue")}
          </Link>
        </div>
        <div className="panel-body">
          <DiagnosisRoomList result={roomsResult} />
        </div>
      </section>
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
  const locale = useLocale();
  const t = useTranslations("Dashboard");
  if (!result.ok) {
    if (result.error.status === 403) {
      return (
        <PermissionLimitedNotice
          detail={t("handoffRestrictedDetail")}
          title={t("handoffRestrictedTitle")}
        />
      );
    }
    return <ErrorNotice message={result.error.message} status={result.error.status} />;
  }
  if (stats.snapshots_needing_room === 0) {
    return (
      <div className="notice">
        <strong>{t("noHandoff")}</strong>
      </div>
    );
  }
  const backlog = result.data.items;
  return (
    <div className="dashboard-room-stack">
      <dl aria-label={t("handoffHealth")} className="stat-list">
        <Stat label={t("snapshotsNeedingRoom")} value={stats.snapshots_needing_room} locale={locale} />
        <Stat label={t("affectedAlerts")} value={stats.affected_alerts_needing_room} locale={locale} />
        <Stat label={t("roomsStarted")} value={stats.rooms_started} locale={locale} />
      </dl>
      {backlog.length === 0 ? (
        <div className="notice">
          <strong>{t("backlogDetailsUnavailable")}</strong>
        </div>
      ) : (
        <ul aria-label={t("backlogLabel")} className="dashboard-room-list">
          {backlog.map((item) => {
            const primaryAlert = item.alerts[0] ?? null;
            return (
              <li className="dashboard-room-item" key={item.evidence_snapshot.id}>
                <div className="dashboard-room-main">
                  <Link href={diagnosisSnapshotHref(item.evidence_snapshot.id)}>
                    {t("createRoom", { id: item.evidence_snapshot.id })}
                  </Link>
                  <div className="muted">
                    {handoffAlertLabel(item.alerts, t)} | {t("group", { id: item.evidence_snapshot.alert_group_id })} |{" "}
                    {item.evidence_snapshot.created_by_workflow || t("manual")}
                  </div>
                </div>
                <div className="dashboard-room-meta">
                  <span className={`pill ${severityPillClass(primaryAlert ? alertSeverity(primaryAlert) : "info")}`}>
                    {localizedSeverity(
                      primaryAlert ? alertSeverity(primaryAlert) : "info",
                      t,
                    )}
                  </span>
                  <span className="pill pill-warning">{t("manualRoom")}</span>
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
  const locale = useLocale();
  const t = useTranslations("Dashboard");
  const tNextStep = useTranslations("DiagnosisRoom.nextStep");
  const tStatus = useTranslations("DiagnosisRoom.status");
  if (!result.ok) {
    if (result.error.status === 403) {
      return (
        <PermissionLimitedNotice
          detail={t("roomsRestrictedDetail")}
          title={t("roomsRestrictedTitle")}
        />
      );
    }
    return <ErrorNotice message={result.error.message} status={result.error.status} />;
  }
  if (result.data.items.length === 0) {
    return (
      <div className="notice">
        <strong>{t("noRooms")}</strong>
      </div>
    );
  }
  const health = diagnosisRoomHealth(result.data.items);
  return (
    <div className="dashboard-room-stack">
      <dl aria-label={t("roomHealth")} className="dashboard-room-health">
        <RoomHealthStat label={t("attention")} value={health.attention} locale={locale} />
        <RoomHealthStat label={t("ready")} value={health.ready} locale={locale} />
        <RoomHealthStat label={t("active")} value={health.active} locale={locale} />
        <RoomHealthStat label={t("closed")} value={health.closed} locale={locale} />
        <RoomHealthStat label={t("notificationFailures")} value={health.notificationFailures} locale={locale} />
      </dl>
      <ul aria-label={t("recentRoomsLabel")} className="dashboard-room-list">
        {result.data.items.map((room) => {
          const step = diagnosisRoomNextStep(room);
          const stepCopy = localizeDiagnosisRoomNextStep(
            step,
            locale,
            tNextStep,
            tStatus,
          );
          return (
            <li className="dashboard-room-item" key={room.chat_session_id}>
              <div className="dashboard-room-main">
                <Link href={diagnosisRoomHref(room)}>{room.session_id}</Link>
                <div className="muted">
                  {t("roomMeta", {
                    task: room.diagnosis_task_id,
                    evidence: room.evidence_snapshot_id,
                    updated: formatDateTime(room.updated_at, locale),
                  })}
                </div>
                <div className="dashboard-room-next-step">
                  <span className={`pill ${statusPillClass(step.color)}`}>{stepCopy.label}</span>
                  <span className="muted">{stepCopy.detail}</span>
                </div>
                <DashboardRoomNotification room={room} />
              </div>
              <div className="dashboard-room-meta">
                <span className={room.room_status === "open" ? "pill pill-ok" : "pill pill-info"}>
                  {localizeDiagnosisRoomStatus(room.room_status, tStatus)}
                </span>
                <span className="pill pill-info">
                  {localizeDiagnosisRoomStatus(room.task_status, tStatus)}
                </span>
                {room.latest_conclusion ? (
                  <>
                    <span className="pill pill-ok">
                      {localizeDiagnosisRoomStatus(room.latest_conclusion.status, tStatus)}
                    </span>
                    {room.latest_conclusion.confidence ? (
                      <span className={`pill ${confidencePillClass(room.latest_conclusion.confidence)}`}>
                        {localizeDiagnosisRoomStatus(room.latest_conclusion.confidence, tStatus)}
                      </span>
                    ) : null}
                    {room.latest_conclusion.requires_human_review ? (
                      <span className="pill pill-warning">{t("review")}</span>
                    ) : null}
                  </>
                ) : null}
                <span className="muted">{t("turns", { count: room.turn_count })}</span>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function RoomHealthStat({ label, locale, value }: { label: string; locale: string; value: number }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{formatCount(value, locale)}</dd>
    </div>
  );
}

function DashboardRoomNotification({ room }: { room: DiagnosisRoomSummary }) {
  const t = useTranslations("Dashboard");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const notification = latestDiagnosisRoomNotification(room);
  if (!notification) {
    return (
      <div className="dashboard-room-notification muted">
        <span>{t("noNotification")}</span>
      </div>
    );
  }
  const failed = notificationFailed(notification.provider_status);
  return (
    <div className="dashboard-room-notification">
      <span className={`pill ${statusPillClass(failed ? "error" : "success")}`}>
        {notificationEventLabel(notification.event_kind, t)}
      </span>
      <span className={`pill ${statusPillClass(failed ? "error" : "success")}`}>
        {localizeDiagnosisRoomStatus(notification.provider_status, tStatus)}
      </span>
      {notification.provider_message_id ? (
        <span className="muted">{notification.provider_message_id}</span>
      ) : null}
      {failed ? (
        <a className="status-line" href={notificationChannelReviewHref(notification)}>
          {t("reviewChannel")}
        </a>
      ) : null}
    </div>
  );
}

export function diagnosisRoomHealth(rooms: DiagnosisRoomSummary[]) {
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

function notificationFailed(status: string): boolean {
  switch (status.toLowerCase()) {
    case "failed":
    case "error":
      return true;
    default:
      return false;
  }
}

function notificationEventLabel(
  eventKind: string,
  t: ReturnType<typeof useTranslations<"Dashboard">>,
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

function handoffAlertLabel(
  alerts: AlertEventSummary[],
  t: ReturnType<typeof useTranslations<"Dashboard">>,
): string {
  const first = alerts[0];
  if (!first) {
    return t("noLinkedAlert");
  }
  const suffix = alerts.length > 1 ? t("moreAlerts", { count: alerts.length - 1 }) : "";
  return `${alertName(first, t)}${suffix}`;
}

function alertName(
  alert: AlertEventSummary,
  t: ReturnType<typeof useTranslations<"Dashboard">>,
): string {
  return (
    alert.labels.alertname ||
    alert.labels.alert_name ||
    t("alertNumber", { id: alert.id })
  );
}

function localizedSeverity(
  severity: string,
  t: ReturnType<typeof useTranslations<"Dashboard">>,
): string {
  switch (severity.toLowerCase()) {
    case "critical":
      return t("critical");
    case "warning":
      return t("warning");
    case "info":
      return t("info");
    default:
      return severity;
  }
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

function MetricCard({ href, label, value }: { href: Route; label: string; value: string }) {
  return (
    <Link className="overview-metric" href={href}>
      <div className="metric-value">{value}</div>
      <div className="metric-label">{label}</div>
    </Link>
  );
}

function Stat({ label, locale = "en", value }: { label: string; locale?: string; value: number }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{formatCount(value, locale)}</dd>
    </div>
  );
}

function ErrorNotice({ message, status }: { message: string; status?: number }) {
  const t = useTranslations("Dashboard");
  return (
    <div className="notice">
      <strong>{status ? `HTTP ${status}` : t("requestFailed")}</strong>
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
