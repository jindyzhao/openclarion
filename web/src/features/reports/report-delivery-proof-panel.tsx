"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import type { NotificationChannelProfile } from "@/features/settings/notification-channels/types";

import { retryReportNotificationAction } from "./client-api";
import { formatDateTime } from "./format";
import {
  defaultReportNotificationRetryChannelValue,
  reportDeliveryProofCurrentDeliveries,
  reportDeliveryProofCanSubmit,
  reportDeliveryProofRetryLabel,
  reportDeliveryProofState,
  reportNotificationDeliveryForPurpose,
  reportNotificationPurposeLabel,
  reportNotificationRetrySuccessMessage,
  reportNotificationRetryChannelOptions,
  selectedReportNotificationRetryChannelID,
  upsertReportNotificationDeliveryOverlay,
  type ReportDeliveryProofOverlay,
} from "./report-delivery-proof";
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
  const notificationLabel = reportNotificationPurposeLabel(notificationPurpose);
  const state = reportDeliveryProofState(latest, notificationPurpose);
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
      message: reportNotificationRetrySuccessMessage(
        result.data.retry_state,
        notificationPurpose,
      ),
    });
    router.refresh();
  }

  return (
    <section
      aria-label={`${sentenceCase(notificationLabel)} delivery proof`}
      className="report-delivery-proof"
    >
      <div className="subreport-conclusion-meta">
        <span className={`label-chip report-delivery-status-${state.status}`}>
          {state.statusLabel}
        </span>
        <span className="label-chip">{currentDeliveries.length} retained</span>
      </div>
      <p className="muted">{state.detail}</p>
      {finalNotificationReadiness ? (
        <div
          className={`report-delivery-readiness report-delivery-readiness-${finalNotificationReadiness.status}`}
        >
          <span className="label-chip">{finalNotificationReadiness.status_label}</span>
          <span>{finalNotificationReadiness.detail}</span>
        </div>
      ) : null}
      <div className="report-delivery-proof-actions">
        {state.actionHref ? (
          <Link className="status-line" href={state.actionHref}>
            {state.actionLabel}
          </Link>
        ) : null}
        {canRetry ? (
          <>
            <label className="report-delivery-proof-channel">
              <span>Notification channel</span>
              <select
                disabled={retrying}
                onChange={(event) => setSelectedChannelValue(event.currentTarget.value)}
                value={effectiveSelectedChannelValue}
              >
                {channelOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
            <button
              className="link-button"
              disabled={retrying}
              onClick={handleRetry}
              type="button"
            >
              {retrying ? "Sending..." : reportDeliveryProofRetryLabel(latest, notificationPurpose)}
            </button>
          </>
        ) : null}
      </div>
      {canRetry && selectedChannel ? (
        <div className="muted">{selectedChannel.detail}</div>
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
          <ReportDeliveryProofDetail label="Status" value={latest.status} />
          <ReportDeliveryProofDetail
            label="Provider"
            value={latest.provider_status ?? "-"}
          />
          <ReportDeliveryProofDetail
            label="Message ID"
            value={latest.provider_message_id ?? "-"}
          />
          <ReportDeliveryProofDetail
            label="Delivered"
            value={latest.delivered_at ? formatDateTime(latest.delivered_at) : "-"}
          />
          <ReportDeliveryProofDetail label="Updated" value={formatDateTime(latest.updated_at)} />
        </dl>
      ) : null}
      {currentDeliveries.length > 0 ? (
        <ul
          aria-label={`${sentenceCase(notificationLabel)} deliveries`}
          className="report-delivery-proof-list"
        >
          {currentDeliveries.map((delivery) => (
            <li className="report-delivery-proof-item" key={delivery.id}>
              <strong>{delivery.idempotency_key}</strong>
              <span className={`label-chip report-delivery-status-${delivery.status}`}>
                {delivery.status}
              </span>
              <span className="muted">
                {delivery.failure_reason ??
                  delivery.provider_status ??
                  (delivery.delivered_at
                    ? `delivered ${formatDateTime(delivery.delivered_at)}`
                    : "provider result pending")}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}

function sentenceCase(value: string): string {
  return value.length === 0 ? value : `${value[0]!.toUpperCase()}${value.slice(1)}`;
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
