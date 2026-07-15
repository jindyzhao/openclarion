"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";

import { localizeDiagnosisRoomStatus } from "@/features/diagnosis-room/status-copy";
import type { NotificationChannelProfile } from "@/features/settings/notification-channels/types";

import { retryReportNotificationAction } from "./client-api";
import { formatDateTime } from "./format";
import {
  defaultReportNotificationRetryChannelValue,
  reportDeliveryProofCurrentDeliveries,
  reportDeliveryProofCanSubmit,
  reportDeliveryProofState,
  reportNotificationDeliveryForPurpose,
  reportNotificationRetryChannelOptions,
  selectedReportNotificationRetryChannelID,
  upsertReportNotificationDeliveryOverlay,
  type ReportDeliveryProofOverlay,
} from "./report-delivery-proof";
import {
  localizeFinalNotificationReadiness,
  localizeReportDeliveryProofRetryLabel,
  localizeReportDeliveryProofState,
  localizeReportNotificationPurpose,
  localizeReportNotificationRetryChannelOption,
  localizeReportNotificationRetrySuccessMessage,
} from "./report-detail-copy";
import type { ReportFinalNotificationReadiness } from "./diagnosis-readiness";
import type { FinalReportDetail, ReportNotificationPurpose } from "./types";

type ReportNotificationDelivery =
  FinalReportDetail["notification_deliveries"][number];

type RetryNotice = {
  kind: "error" | "success";
  message: string;
};

export function ReportDeliveryProofPanel({
  deliveries,
  finalNotificationReadiness,
  notificationChannels,
  notificationChannelsError,
  notificationPurpose = "final",
  reportID,
}: {
  deliveries: ReportNotificationDelivery[];
  finalNotificationReadiness?: ReportFinalNotificationReadiness;
  notificationChannels: NotificationChannelProfile[];
  notificationChannelsError?: string;
  notificationPurpose?: ReportNotificationPurpose;
  reportID: number;
}) {
  const router = useRouter();
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const channelOptions = useMemo(
    () => reportNotificationRetryChannelOptions(notificationChannels),
    [notificationChannels],
  );
  const [localDeliveryOverlay, setLocalDeliveryOverlay] =
    useState<ReportDeliveryProofOverlay | null>(null);
  const currentDeliveries = useMemo(
    () =>
      reportDeliveryProofCurrentDeliveries(
        deliveries,
        localDeliveryOverlay,
        reportID,
      ),
    [deliveries, localDeliveryOverlay, reportID],
  );
  const latest = reportNotificationDeliveryForPurpose(
    currentDeliveries,
    notificationPurpose,
  );
  const [notice, setNotice] = useState<RetryNotice | null>(null);
  const [retrying, setRetrying] = useState(false);
  const [selectedChannelValue, setSelectedChannelValue] = useState(
    defaultReportNotificationRetryChannelValue(
      channelOptions,
      latest?.report_notification_channel_profile_id ?? null,
    ),
  );
  const notificationLabel = localizeReportNotificationPurpose(
    notificationPurpose,
    t,
  );
  const state = reportDeliveryProofState(latest, notificationPurpose);
  const stateCopy = localizeReportDeliveryProofState(state, t);
  const canRetry = reportDeliveryProofCanSubmit(
    latest,
    notificationPurpose,
    finalNotificationReadiness,
  );
  const effectiveSelectedChannelValue = channelOptions.some(
    (option) => option.value === selectedChannelValue,
  )
    ? selectedChannelValue
    : defaultReportNotificationRetryChannelValue(
        channelOptions,
        latest?.report_notification_channel_profile_id ?? null,
      );
  const selectedChannelID = selectedReportNotificationRetryChannelID(
    channelOptions,
    effectiveSelectedChannelValue,
  );
  const selectedChannel = channelOptions.find(
    (option) => option.value === effectiveSelectedChannelValue,
  );

  async function handleRetry() {
    if (!canRetry || retrying) {
      return;
    }
    setNotice(null);
    setRetrying(true);
    const result = await retryReportNotificationAction(
      reportID,
      selectedChannelID,
      notificationPurpose,
    );
    setRetrying(false);
    if (!result.ok) {
      setNotice({ kind: "error", message: result.error.message });
      return;
    }
    setLocalDeliveryOverlay((overlay) =>
      upsertReportNotificationDeliveryOverlay(overlay, reportID, result.data.delivery),
    );
    setNotice({
      kind: "success",
      message: localizeReportNotificationRetrySuccessMessage(
        result.data.retry_state,
        notificationPurpose,
        t,
      ),
    });
    router.refresh();
  }

  return (
    <section
      aria-label={t("deliveryProofFor", { notification: notificationLabel })}
      className="report-delivery-proof"
    >
      <div className="subreport-conclusion-meta">
        <span className={`label-chip report-delivery-status-${state.status}`}>
          {stateCopy.statusLabel}
        </span>
        <span className="label-chip">
          {t("deliveriesRetained", { count: currentDeliveries.length })}
        </span>
      </div>
      <p className="muted">{stateCopy.detail}</p>
      {finalNotificationReadiness ? (
        <FinalNotificationReadiness
          readiness={finalNotificationReadiness}
        />
      ) : null}
      <div className="report-delivery-proof-actions">
        {state.actionHref ? (
          <Link className="status-line" href={state.actionHref}>
            {stateCopy.actionLabel}
          </Link>
        ) : null}
        {canRetry ? (
          <>
            <label className="report-delivery-proof-channel">
              <span>{t("notificationChannel")}</span>
              <select
                disabled={retrying}
                onChange={(event) => setSelectedChannelValue(event.currentTarget.value)}
                value={effectiveSelectedChannelValue}
              >
                {channelOptions.map((option) => {
                  const copy = localizeReportNotificationRetryChannelOption(
                    option,
                    t,
                  );
                  return (
                    <option key={option.value} value={option.value}>
                      {copy.label}
                    </option>
                  );
                })}
              </select>
            </label>
            <button
              className="link-button"
              disabled={retrying}
              onClick={handleRetry}
              type="button"
            >
              {retrying
                ? t("sending")
                : localizeReportDeliveryProofRetryLabel(
                    latest !== null,
                    notificationPurpose,
                    t,
                  )}
            </button>
          </>
        ) : null}
      </div>
      {canRetry && selectedChannel ? (
        <div className="muted">
          {localizeReportNotificationRetryChannelOption(selectedChannel, t).detail}
        </div>
      ) : null}
      {canRetry && notificationChannelsError ? (
        <div className="report-delivery-proof-feedback report-delivery-proof-feedback-error" role="alert">
          {notificationChannelsError}
        </div>
      ) : null}
      {notice ? (
        <div
          className={`report-delivery-proof-feedback report-delivery-proof-feedback-${notice.kind}`}
          role={notice.kind === "error" ? "alert" : "status"}
        >
          {notice.message}
        </div>
      ) : null}
      {latest ? (
        <dl className="subreport-conclusion-details">
          <ReportDeliveryProofDetail
            label={t("fieldStatus")}
            value={localizeDiagnosisRoomStatus(latest.status, tStatus)}
          />
          <ReportDeliveryProofDetail
            label={t("provider")}
            value={
              latest.provider_status ? latest.provider_status : "-"
            }
          />
          <ReportDeliveryProofDetail
            label={t("messageId")}
            value={latest.provider_message_id ?? "-"}
          />
          <ReportDeliveryProofDetail
            label={t("deliveredAt")}
            value={
              latest.delivered_at
                ? formatDateTime(latest.delivered_at, locale)
                : "-"
            }
          />
          <ReportDeliveryProofDetail
            label={t("updatedAt")}
            value={formatDateTime(latest.updated_at, locale)}
          />
        </dl>
      ) : null}
      {currentDeliveries.length > 0 ? (
        <ul
          aria-label={t("deliveriesFor", { notification: notificationLabel })}
          className="report-delivery-proof-list"
        >
          {currentDeliveries.map((delivery) => (
            <li className="report-delivery-proof-item" key={delivery.id}>
              <strong>{delivery.idempotency_key}</strong>
              <span className={`label-chip report-delivery-status-${delivery.status}`}>
                {localizeDiagnosisRoomStatus(delivery.status, tStatus)}
              </span>
              <span className="muted">
                {delivery.failure_reason ??
                  (delivery.provider_status
                    ? delivery.provider_status
                    : undefined) ??
                  (delivery.delivered_at
                    ? t("deliveredOn", {
                        time: formatDateTime(delivery.delivered_at, locale),
                      })
                    : t("providerResultPending"))}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}

function FinalNotificationReadiness({
  readiness,
}: {
  readiness: ReportFinalNotificationReadiness;
}) {
  const t = useTranslations("ReportDetail");
  const copy = localizeFinalNotificationReadiness(readiness, t);
  return (
    <div
      className={`report-delivery-readiness report-delivery-readiness-${readiness.status}`}
    >
      <span className="label-chip">{copy.label}</span>
      <span>{copy.detail}</span>
    </div>
  );
}

function ReportDeliveryProofDetail({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}
