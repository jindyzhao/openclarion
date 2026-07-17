import { Empty, Space, Tag, Timeline, Typography } from "antd";
import type { TimelineProps } from "antd";
import { useLocale, useTranslations } from "next-intl";

import { formatDateTime } from "@/features/reports/format";

import type {
  DiagnosisRoomAuditTimelineEntry,
  DiagnosisRoomSummary,
} from "./api";

const auditEventTranslationKeys = {
  "diagnosis_room.opened": "auditEventOpened",
  "diagnosis_room.turn_persisted": "auditEventTurnRetained",
  "diagnosis_room.evidence_collected": "auditEventEvidenceCollected",
  "diagnosis_room.supplemental_evidence_provided":
    "auditEventSupplementalEvidenceRetained",
  "diagnosis_room.final_conclusion_ready": "auditEventConclusionReady",
  "diagnosis_room.conclusion_approved": "auditEventConclusionApproved",
  "diagnosis_room.assistant_turn_notification_sent":
    "auditEventAIUpdateDelivered",
  "diagnosis_room.final_ready_notification_sent":
    "auditEventFinalReadyNotificationDelivered",
  "diagnosis_room.closed": "auditEventRoomClosed",
  "diagnosis_room.close_notification_sent":
    "auditEventCloseNotificationDelivered",
  "diagnosis_room.failed": "auditEventRoomFailed",
} as const;

const auditCategoryTranslationKeys = {
  lifecycle: "auditCategoryLifecycle",
  interaction: "auditCategoryInteraction",
  evidence: "auditCategoryEvidence",
  approval: "auditCategoryApproval",
  notification: "auditCategoryNotification",
  other: "auditCategoryOther",
} as const satisfies Record<
  DiagnosisRoomAuditTimelineEntry["category"],
  string
>;

export function diagnosisAuditEventTranslationKey(
  eventKind: string,
): (typeof auditEventTranslationKeys)[keyof typeof auditEventTranslationKeys] | null {
  return (
    auditEventTranslationKeys[
      eventKind as keyof typeof auditEventTranslationKeys
    ] ?? null
  );
}

export function diagnosisAuditCategoryTranslationKey(
  category: DiagnosisRoomAuditTimelineEntry["category"],
): (typeof auditCategoryTranslationKeys)[DiagnosisRoomAuditTimelineEntry["category"]] {
  return auditCategoryTranslationKeys[category];
}

export function diagnosisAuditTimelineColor(
  category: DiagnosisRoomAuditTimelineEntry["category"],
): string {
  switch (category) {
    case "lifecycle":
      return "blue";
    case "interaction":
      return "green";
    case "evidence":
      return "orange";
    case "approval":
      return "gold";
    case "notification":
      return "cyan";
    default:
      return "gray";
  }
}

export function diagnosisAuditTimelineEntryKey(
  entry: DiagnosisRoomAuditTimelineEntry,
): string {
  return `${entry.id}:${entry.event_kind}`;
}

export function DiagnosisAuditTimelineSection({
  room,
}: {
  room?: DiagnosisRoomSummary;
}) {
  const locale = useLocale();
  const t = useTranslations("DiagnosisRoom");
  const entries = room?.audit_timeline;
  if (entries === undefined) {
    return null;
  }

  const items: TimelineProps["items"] = entries.map((entry) => {
    const eventTranslationKey = diagnosisAuditEventTranslationKey(
      entry.event_kind,
    );
    return {
      key: diagnosisAuditTimelineEntryKey(entry),
      color: diagnosisAuditTimelineColor(entry.category),
      children: (
        <div className="diagnosis-audit-timeline-entry">
          <Space size={[6, 6]} wrap>
            <Typography.Text strong>
              {eventTranslationKey === null
                ? entry.event_kind
                : t(eventTranslationKey)}
            </Typography.Text>
            <Tag>{t(diagnosisAuditCategoryTranslationKey(entry.category))}</Tag>
            <Typography.Text code>#{entry.id}</Typography.Text>
          </Space>
          <Typography.Text type="secondary">
            {formatDateTime(entry.occurred_at, locale)}
            {entry.recorded_at !== entry.occurred_at
              ? ` / ${t("auditRecordedAt", {
                  time: formatDateTime(entry.recorded_at, locale),
                })}`
              : ""}
          </Typography.Text>
          {entry.actor_subject ? (
            <Typography.Text type="secondary">
              {t("auditActorSubject", { subject: entry.actor_subject })}
            </Typography.Text>
          ) : null}
        </div>
      ),
    };
  });

  return (
    <section
      aria-label={t("auditTimelineLabel")}
      className="diagnosis-audit-timeline"
    >
      <div className="diagnosis-audit-timeline-header">
        <Typography.Title level={3}>{t("auditTimeline")}</Typography.Title>
        <Typography.Text type="secondary">
          {t("immutableEventCount", { count: entries.length })}
        </Typography.Text>
      </div>
      {entries.length > 0 ? (
        <Timeline className="diagnosis-audit-timeline-list" items={items} />
      ) : (
        <Empty
          description={t("noLifecycleEvents")}
          image={Empty.PRESENTED_IMAGE_SIMPLE}
        />
      )}
    </section>
  );
}
