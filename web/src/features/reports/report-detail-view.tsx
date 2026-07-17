import Link from "next/link";
import { useLocale, useTranslations } from "next-intl";
import type { ReactNode } from "react";

import type { DiagnosisRoomListResponse } from "@/features/diagnosis-room/api";
import { diagnosisEvidencePlanURLQuery } from "@/features/diagnosis-room/evidence-plan-url";
import type { DiagnosisReviewReturnState } from "@/features/diagnosis-room/report-return";
import { localizeDiagnosisRoomStatus } from "@/features/diagnosis-room/status-copy";
import {
  directorySubjectIsSystem,
  directorySubjectProfile,
  directoryUserSubjectIndex,
} from "@/features/settings/directory-rbac/format";
import type {
  DirectoryUser,
  DirectoryUserListResponse,
} from "@/features/settings/directory-rbac/types";
import type { NotificationChannelProfileListResponse } from "@/features/settings/notification-channels/types";

import {
  diagnosisReadiness,
  reportConsultationAuditTimeline,
  reportDiagnosisHandoff,
  reportDiagnosisNextAction,
  reportEvidenceFollowUps,
  reportFinalNotificationReadiness,
  subReportDiagnosisReadiness,
  type ReportConsultationAuditItem,
  type ReportDiagnosisConclusion,
  type ReportDiagnosisHandoff,
  type ReportDiagnosisNextAction,
  type ReportDiagnosisProgress,
  type ReportEvidenceFollowUp,
} from "./diagnosis-readiness";
import {
  reportDiagnosisRoomHref,
  type ReportDiagnosisRoomHref,
} from "./report-diagnosis-room-link";
import { formatDateTime, severityClass } from "./format";
import {
  reportEvidenceCollectionResultForRequest,
  reportEvidenceRequestKey,
} from "./report-evidence-display";
import {
  localizeDecisionRecordDiagnosisAction,
  localizeReportConclusionReason,
  localizeReportConclusionSource,
  localizeReportConsultationAuditItem,
  localizeReportDecisionRecord,
  localizeReportDiagnosisHandoff,
  localizeReportDiagnosisNextAction,
  localizeReportDiagnosisReviewReturnNotice,
  localizeReportEvidenceCollectionResultDetail,
  localizeReportEvidenceFollowUpKind,
  localizeReportEvidenceRequestDetail,
  localizeReportReadiness,
  localizeSubReportDiagnosisAction,
  localizeSubReportReadiness,
  type ReportDetailTranslator,
} from "./report-detail-copy";
import { ReportDeliveryProofPanel } from "./report-delivery-proof-panel";
import {
  reportDiagnosisDecisionRecords,
  type ReportDecisionRecord,
} from "./report-decision-records";
import { reportDiagnosisReviewReturnNotice } from "./report-return-notice";
import {
  localizeReportConfidence,
  localizeReportSeverity,
} from "./report-list-copy";
import { notificationChannelEditHref } from "@/features/settings/notification-channels/format";
import type { ApiResult, FinalReportDetail } from "./types";

type ReportDetailViewProps = {
  directoryUsersResult: ApiResult<DirectoryUserListResponse>;
  diagnosisReviewReturn?: DiagnosisReviewReturnState;
  notificationChannelsResult: ApiResult<NotificationChannelProfileListResponse>;
  reportId: string;
  result: ApiResult<FinalReportDetail>;
  roomsResult: ApiResult<DiagnosisRoomListResponse>;
};

type DiagnosisRoomHref = ReportDiagnosisRoomHref;
type ReportConsultationEvidenceRequest = NonNullable<
  ReportDiagnosisProgress["missing_evidence_requests"]
>[number];
type ReportEvidenceCollectionResult = NonNullable<
  NonNullable<ReportDiagnosisConclusion["confidence_timeline"]>[number]["evidence_collection_results"]
>[number];
type ReportEvidenceRequest = NonNullable<
  ReportDiagnosisProgress["evidence_requests"]
>[number];
type ReportConfidenceTimelineEntry =
  | NonNullable<ReportDiagnosisConclusion["confidence_timeline"]>[number]
  | NonNullable<ReportDiagnosisProgress["confidence_timeline"]>[number];
type ReportSupplementalEvidence = NonNullable<
  ReportDiagnosisConclusion["supplemental_evidence"] | ReportDiagnosisProgress["supplemental_evidence"]
>[number];

export function ReportDetailView({
  directoryUsersResult,
  diagnosisReviewReturn = "none",
  notificationChannelsResult,
  reportId,
  result,
  roomsResult
}: ReportDetailViewProps) {
  const t = useTranslations("ReportDetail");
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>
            {result.ok ? result.data.title : t("fallbackTitle", { id: reportId })}
          </h1>
          <p>{result.ok ? result.data.correlation_key : t("detail")}</p>
        </div>
        <Link className="status-line" href="/reports">
          {t("back")}
        </Link>
      </div>

      {!result.ok ? <ErrorNotice message={result.error.message} status={result.error.status} /> : null}
      {diagnosisReviewReturn !== "none" && result.ok ? (
        <DiagnosisReviewReturnNotice
          finalNotificationReadiness={reportFinalNotificationReadiness(
            result.data,
          )}
          kind={reportDiagnosisReviewReturnNotice(
            diagnosisReviewReturn,
            reportFinalNotificationReadiness(result.data),
          )}
        />
      ) : null}
      {result.ok ? (
        <Detail
          directoryUsersResult={directoryUsersResult}
          notificationChannelsResult={notificationChannelsResult}
          report={result.data}
          roomsResult={roomsResult}
        />
      ) : null}
    </>
  );
}

function DiagnosisReviewReturnNotice({
  finalNotificationReadiness,
  kind,
}: {
  finalNotificationReadiness: ReturnType<
    typeof reportFinalNotificationReadiness
  >;
  kind: ReturnType<typeof reportDiagnosisReviewReturnNotice>;
}) {
  const t = useTranslations("ReportDetail");
  const notice = localizeReportDiagnosisReviewReturnNotice(
    kind,
    finalNotificationReadiness,
    t,
  );
  return (
    <div aria-label={t("reviewReturn")} className="notice" role="status">
      <strong>{notice.title}</strong>
      <div>{notice.detail}</div>
    </div>
  );
}

function Detail({
  directoryUsersResult,
  notificationChannelsResult,
  report,
  roomsResult
}: {
  directoryUsersResult: ApiResult<DirectoryUserListResponse>;
  notificationChannelsResult: ApiResult<NotificationChannelProfileListResponse>;
  report: FinalReportDetail;
  roomsResult: ApiResult<DiagnosisRoomListResponse>;
}) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const tList = useTranslations("ReportList");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const readiness = diagnosisReadiness(report);
  const readinessCopy = localizeReportReadiness(readiness, locale, t);
  const finalNotificationReadiness = reportFinalNotificationReadiness(report);
  const evidenceFollowUps = reportEvidenceFollowUps(report);
  const handoff = reportDiagnosisHandoff(report);
  const consultationAuditTimeline = reportConsultationAuditTimeline(report);
  const nextDiagnosisAction = reportDiagnosisNextAction(report);
  const rooms = roomsResult.ok ? roomsResult.data.items : [];
  const directoryUsersBySubject = directoryUserSubjectIndex(
    directoryUsersResult.ok ? directoryUsersResult.data.items : [],
  );
  const decisionRecords = reportDiagnosisDecisionRecords(
    report,
    rooms,
  );
  return (
    <div className="detail-grid">
      <section className="panel">
        <div className="panel-header">
          <h2>{t("summary")}</h2>
        </div>
        <div className="panel-body stack">
          <div>
            <span className={severityClass(report.severity)}>
              {localizeReportSeverity(report.severity, tList)}
            </span>{" "}
            <span className="muted">
              {t("confidence", {
                value: localizeReportConfidence(report.confidence, tList),
              })}
            </span>
          </div>
          <p>{report.executive_summary}</p>
          {report.generation_status === "partial" ? (
            <div className="notice" role="status">
              <strong>{t("partialReport")}</strong>
              <div>
                {t("partialCoverage", {
                  successful: report.successful_sub_report_count,
                  expected: report.expected_sub_report_count,
                  failed: report.failed_sub_report_count,
                })}
              </div>
            </div>
          ) : null}
          <p className="muted">{report.notification_text}</p>
          <div className="muted">
            {report.created_by_workflow} / {formatDateTime(report.created_at, locale)}
          </div>
        </div>
      </section>

      <aside className="panel" id="diagnosis-readiness">
        <div className="panel-header">
          <h2>{t("recommendedActions")}</h2>
        </div>
        <div className="panel-body">
          <ol className="action-list">
            {report.recommended_actions.map((action) => (
              <li key={`${action.priority}:${action.label}`}>
                <strong>{action.label}</strong>
                <div className="muted">{action.detail}</div>
              </li>
            ))}
          </ol>
        </div>
      </aside>

      <aside className="panel" id="report-delivery-proof">
        <div className="panel-header">
          <h2>{t("diagnosisReadiness")}</h2>
        </div>
        <div className="panel-body">
          <div
            aria-label={t("reviewStatus")}
            className={`diagnosis-readiness-summary diagnosis-readiness-summary-${readiness.status}`}
          >
            <span className="label-chip">{t("aiReview")}</span>
            <strong>{readinessCopy.statusLabel}</strong>
            <p>{readinessCopy.statusDetail}</p>
          </div>
          <div
            aria-label={t("reviewQueue")}
            className="diagnosis-readiness-queue"
          >
            <div className="subreport-conclusion-meta">
              <span
                className={`label-chip diagnosis-status-${readiness.status}`}
              >
                {readinessCopy.queueLabel}
              </span>
              {readiness.blocked ? (
                <span className="label-chip diagnosis-status-needs_evidence">
                  {t("blocked")}
                </span>
              ) : null}
              {readiness.canConfirm ? (
                <span className="label-chip diagnosis-status-human_review">
                  {t("confirmable")}
                </span>
              ) : null}
            </div>
            <p className="muted">{readinessCopy.queueDetail}</p>
            <dl className="stat-list diagnosis-readiness-queue-stats">
              <div>
                <dt>{t("attention")}</dt>
                <dd>{readiness.attention}</dd>
              </div>
              <div>
                <dt>{t("pending")}</dt>
                <dd>{readiness.pending}</dd>
              </div>
              <div>
                <dt>{t("ready")}</dt>
                <dd>{readiness.ready}</dd>
              </div>
              <div>
                <dt>{t("done")}</dt>
                <dd>{readiness.done}</dd>
              </div>
            </dl>
          </div>
          <DiagnosisHandoffPlan handoff={handoff} readiness={readiness} />
          <DiagnosisNextAction
            action={nextDiagnosisAction}
            report={report}
            rooms={rooms}
          />
          <EvidenceFollowUpSummary
            items={evidenceFollowUps}
            report={report}
            rooms={rooms}
          />
          <dl aria-label={t("readinessLabel")} className="stat-list">
            <div>
              <dt>{t("reviewedSubreports")}</dt>
              <dd>
                {readiness.reviewed} / {readiness.total}
              </dd>
            </div>
            <div>
              <dt>{t("humanReview")}</dt>
              <dd>{readiness.humanReviewRequired}</dd>
            </div>
            <div>
              <dt>{t("evidenceRequested")}</dt>
              <dd>{readiness.evidenceRequests}</dd>
            </div>
            <div>
              <dt>{t("evidenceStillNeeded")}</dt>
              <dd>{readiness.currentMissingEvidence}</dd>
            </div>
            <div>
              <dt>{t("executableEvidenceOpen")}</dt>
              <dd>{readiness.currentExecutableEvidenceRequests}</dd>
            </div>
            <div>
              <dt>{t("residualSuggestions")}</dt>
              <dd>{readiness.currentCollectionSuggestions}</dd>
            </div>
            <div>
              <dt>{t("supplementalEvidence")}</dt>
              <dd>{readiness.supplementalEvidence}</dd>
            </div>
            <div>
              <dt>{t("latestConfidence")}</dt>
              <dd>
                {localizeDiagnosisRoomStatus(
                  readiness.latestConfidence,
                  tStatus,
                )}
              </dd>
            </div>
          </dl>
        </div>
      </aside>

      <aside className="panel">
        <div className="panel-header">
          <h2>{t("deliveryProof")}</h2>
        </div>
        <div className="panel-body">
          <ReportDeliveryProofPanel
            deliveries={report.notification_deliveries}
            finalNotificationReadiness={finalNotificationReadiness}
            notificationChannels={
              notificationChannelsResult.ok ? notificationChannelsResult.data.items : []
            }
            notificationChannelsError={
              notificationChannelsResult.ok
                ? undefined
                : notificationChannelsResult.error.message
            }
            notificationPurpose={finalNotificationReadiness.notification_purpose}
            reportID={report.id}
          />
        </div>
      </aside>

      <section className="panel detail-grid-wide">
        <div className="panel-header">
          <h2>{t("consultationAudit")}</h2>
        </div>
        <div className="panel-body">
          <ConsultationAuditTimeline
            items={consultationAuditTimeline}
            report={report}
            rooms={rooms}
          />
        </div>
      </section>

      <section className="panel detail-grid-wide">
        <div className="panel-header">
          <h2>{t("traceability")}</h2>
        </div>
        <div className="panel-body">
          <ul className="subreport-list">
            {report.linked_sub_reports.map((subReport) => {
              const subReportReadiness = subReportDiagnosisReadiness(subReport);
              const subReportReadinessCopy = localizeSubReportReadiness(
                subReportReadiness,
                locale,
                t,
              );
              const diagnosisHref = reportDiagnosisRoomHref(report, subReport, rooms);
              return (
                <li className="subreport-item" key={subReport.id}>
                  <div className="subreport-item-header">
                    <h3>{subReport.title}</h3>
                    <Link
                      className="link-button"
                      href={diagnosisHref}
                    >
                      {localizeSubReportDiagnosisAction(
                        {
                          hasConclusion:
                            subReport.diagnosis_conclusion !== undefined,
                          hasProgress: subReport.diagnosis_progress !== undefined,
                          readiness: subReportReadiness,
                        },
                        t,
                      )}
                    </Link>
                  </div>
                  <div className="muted">
                    {t("evidenceSnapshot", {
                      id: subReport.evidence_snapshot_id,
                    })}
                  </div>
                  <p>{subReport.summary}</p>
                  <span className={severityClass(subReport.severity)}>
                    {localizeReportSeverity(subReport.severity, tList)}
                  </span>{" "}
                  <span className="muted">
                    {t("confidence", {
                      value: localizeReportConfidence(
                        subReport.confidence,
                        tList,
                      ),
                    })}
                  </span>
                  <div
                    aria-label={t("subreportReviewStatus", {
                      title: subReport.title,
                    })}
                    className="subreport-ai-status"
                  >
                    <span className={`label-chip diagnosis-status-${subReportReadiness.status}`}>
                      {subReportReadinessCopy.label}
                    </span>
                    <span className="muted">
                      {subReportReadinessCopy.detail}
                    </span>
                  </div>
                  {subReport.diagnosis_conclusion ? (
                    <DiagnosisConclusion
                      conclusion={subReport.diagnosis_conclusion}
                      directoryUsersBySubject={directoryUsersBySubject}
                      diagnosisHref={diagnosisHref}
                    />
                  ) : subReport.diagnosis_progress ? (
                    <DiagnosisProgress
                      diagnosisHref={diagnosisHref}
                      progress={subReport.diagnosis_progress}
                    />
                  ) : null}
                </li>
              );
            })}
          </ul>
        </div>
      </section>

      <section className="panel detail-grid-wide">
        <div className="panel-header">
          <h2>{t("decisionRecords")}</h2>
        </div>
        <div className="panel-body">
          <ReportDecisionRecords
            directoryUsersBySubject={directoryUsersBySubject}
            records={decisionRecords}
            report={report}
            roomsResult={roomsResult}
          />
        </div>
      </section>
    </div>
  );
}

function ConsultationAuditTimeline({
  items,
  report,
  rooms,
}: {
  items: ReportConsultationAuditItem[];
  report: FinalReportDetail;
  rooms: DiagnosisRoomListResponse["items"];
}) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const tStatus = useTranslations("DiagnosisRoom.status");
  if (items.length === 0) {
    return <p className="muted">{t("noConsultationAudit")}</p>;
  }
  return (
    <ul
      aria-label={t("auditTimeline")}
      className="report-decision-record-list"
    >
      {items.map((item) => {
        const copy = localizeReportConsultationAuditItem(
          item,
          locale,
          t,
          tStatus,
        );
        const subReport = report.linked_sub_reports.find(
          (entry) => entry.id === item.subReportID,
        );
        return (
          <li className="report-decision-record" key={item.subReportID}>
            <div className="report-decision-record-header">
              <div>
                <h3>{item.subReportTitle}</h3>
                <div className="muted">
                  {t("evidenceNumber", { id: item.evidenceSnapshotID })}
                </div>
              </div>
              <span
                className={`label-chip diagnosis-status-${item.readiness.status}`}
              >
                {copy.statusLabel}
              </span>
            </div>
            <p className="muted">{copy.statusDetail}</p>
            <ol className="diagnosis-handoff-step-list">
              {copy.steps.map((step) => (
                <li
                  className={`diagnosis-handoff-step diagnosis-handoff-step-${step.status}`}
                  key={step.key}
                >
                  <div className="diagnosis-handoff-step-heading">
                    <span className={`label-chip diagnosis-handoff-status-${step.status}`}>
                      {step.statusLabel}
                    </span>
                    <strong>{step.label}</strong>
                  </div>
                  <p className="muted">{step.detail}</p>
                </li>
              ))}
            </ol>
            {subReport ? (
              <div className="report-decision-record-actions">
                <Link
                  className="link-button"
                  href={reportDiagnosisRoomHref(report, subReport, rooms)}
                >
                  {localizeSubReportDiagnosisAction(
                    {
                      hasConclusion:
                        subReport.diagnosis_conclusion !== undefined,
                      hasProgress: subReport.diagnosis_progress !== undefined,
                      readiness: item.readiness,
                    },
                    t,
                  )}
                </Link>
              </div>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}

function DiagnosisNextAction({
  action,
  report,
  rooms,
}: {
  action: ReportDiagnosisNextAction | null;
  report: FinalReportDetail;
  rooms: DiagnosisRoomListResponse["items"];
}) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  if (!action) {
    return null;
  }
  const subReport = report.linked_sub_reports.find(
    (item) => item.id === action.subReportID,
  );
  if (!subReport) {
    return null;
  }
  const copy = localizeReportDiagnosisNextAction(action, locale, t);
  return (
    <section
      aria-label={t("nextDiagnosisAction")}
      className="diagnosis-readiness-followups"
    >
      <div className="diagnosis-readiness-followup-heading">
        <strong>{t("nextDiagnosisAction")}</strong>
        <span
          className={`label-chip diagnosis-status-${action.readiness.status}`}
        >
          {copy.statusLabel}
        </span>
      </div>
      <div className="diagnosis-readiness-followup-item">
        <div className="diagnosis-readiness-followup-footer">
          <strong>{action.title}</strong>
          <Link
            className="timeline-evidence-action link-button"
            href={reportDiagnosisRoomHref(report, subReport, rooms)}
          >
            {copy.actionLabel}
          </Link>
        </div>
        <p className="muted">{copy.detail}</p>
        <div className="muted">
          {t("evidenceNumber", { id: action.evidenceSnapshotID })}
        </div>
      </div>
    </section>
  );
}

function DiagnosisHandoffPlan({
  handoff,
  readiness,
}: {
  handoff: ReportDiagnosisHandoff;
  readiness: ReturnType<typeof diagnosisReadiness>;
}) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const copy = localizeReportDiagnosisHandoff(
    handoff,
    readiness,
    locale,
    t,
  );
  return (
    <section
      aria-label={t("handoffPlanLabel")}
      className="diagnosis-handoff-plan"
    >
      <div className="diagnosis-handoff-plan-heading">
        <strong>{t("handoffPlan")}</strong>
        <span className="label-chip">{copy.statusLabel}</span>
      </div>
      <p className="muted">{copy.statusDetail}</p>
      <ol className="diagnosis-handoff-step-list">
        {copy.steps.map((step) => (
          <li className={`diagnosis-handoff-step diagnosis-handoff-step-${step.status}`} key={step.key}>
            <div className="diagnosis-handoff-step-heading">
              <span className={`label-chip diagnosis-handoff-status-${step.status}`}>
                {step.statusLabel}
              </span>
              <strong>{step.label}</strong>
            </div>
            <p className="muted">{step.detail}</p>
          </li>
        ))}
      </ol>
    </section>
  );
}

function ReportDecisionRecords({
  directoryUsersBySubject,
  report,
  records,
  roomsResult
}: {
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  report: FinalReportDetail;
  records: ReportDecisionRecord[];
  roomsResult: ApiResult<DiagnosisRoomListResponse>;
}) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const rooms = roomsResult.ok ? roomsResult.data.items : [];
  const roomsRestricted = !roomsResult.ok && roomsResult.error.status === 403;
  return (
    <div className="report-decision-records">
      {roomsRestricted ? (
        <PermissionLimitedNotice
          detail={t("roomProofRestrictedDetail")}
          title={t("roomProofRestrictedTitle")}
        />
      ) : !roomsResult.ok ? (
        <div className="notice" role="status">
          <strong>{t("roomProofUnavailable")}</strong>
          <div>{roomsResult.error.message}</div>
        </div>
      ) : null}
      <ul
        aria-label={t("recordsLabel")}
        className="report-decision-record-list"
      >
        {records.map((record) => {
          const copy = localizeReportDecisionRecord(
            record,
            locale,
            t,
            tStatus,
          );
          return (
          <li className="report-decision-record" key={record.subReportID}>
            <div className="report-decision-record-header">
              <div>
                <h3>{record.title}</h3>
                <div className="muted">
                  {t("evidenceNumber", { id: record.evidenceSnapshotID })}
                  {record.sessionID ? ` / ${record.sessionID}` : ""}
                </div>
              </div>
              <span className={`label-chip report-decision-status-${record.status}`}>
                {copy.statusLabel}
              </span>
            </div>
            <p>{copy.detail}</p>
            <dl className="subreport-conclusion-details report-decision-record-details">
              <ReportDecisionRecordDetail
                label={t("version")}
                value={record.version || "-"}
              />
              <ReportDecisionRecordDetail
                label={t("recorded")}
                value={
                  record.recordedAt
                    ? formatDateTime(record.recordedAt, locale)
                    : "-"
                }
              />
              <ReportDecisionRecordDetail
                label={t("confirmedBy")}
                value={
                  <ReportDirectorySubject
                    directoryUsersBySubject={directoryUsersBySubject}
                    subject={record.confirmedBy}
                  />
                }
              />
              <ReportDecisionRecordDetail
                label={t("roomStatus")}
                value={copy.roomStatus}
              />
              <ReportDecisionRecordDetail
                label={t("roomClose")}
                value={copy.roomCloseDetail}
              />
              <ReportDecisionRecordDetail
                label={t("notification")}
                value={copy.notificationLabel}
              />
            </dl>
            <p className="muted">{copy.notificationDetail}</p>
            <div className="report-decision-record-actions">
              <Link
                className="link-button"
                href={decisionRecordDiagnosisHref(report, record, rooms)}
              >
                {localizeDecisionRecordDiagnosisAction(record, t)}
              </Link>
              {record.notificationFailed ? (
                <a
                  className="status-line"
                  href={decisionRecordNotificationHref(record)}
                >
                  {t("reviewNotificationChannel")}
                </a>
              ) : null}
            </div>
          </li>
          );
        })}
      </ul>
    </div>
  );
}

function ReportDecisionRecordDetail({
  label,
  value
}: {
  label: string;
  value: ReactNode;
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function decisionRecordDiagnosisHref(
  report: FinalReportDetail,
  record: ReportDecisionRecord,
  rooms: DiagnosisRoomListResponse["items"],
) {
  const subReport = report.linked_sub_reports.find((item) => item.id === record.subReportID);
  if (subReport) {
    return reportDiagnosisRoomHref(report, subReport, rooms);
  }
  return {
    pathname: "/diagnosis-room",
    query: {
      evidence_snapshot_id: String(record.evidenceSnapshotID),
      ...(record.sessionID ? { session_id: record.sessionID } : {})
    }
  };
}

function decisionRecordNotificationHref(record: ReportDecisionRecord) {
  return record.notificationChannelProfileID === null
    ? "/settings/notification-channels"
    : notificationChannelEditHref(record.notificationChannelProfileID);
}

function EvidenceFollowUpSummary({
  items,
  report,
  rooms,
}: {
  items: ReportEvidenceFollowUp[];
  report: FinalReportDetail;
  rooms: DiagnosisRoomListResponse["items"];
}) {
  const t = useTranslations("ReportDetail");
  if (items.length === 0) {
    return null;
  }
  const visibleItems = items.slice(0, 3);
  const hiddenItemCount = items.length - visibleItems.length;
  return (
    <section
      aria-label={t("nextEvidenceActionsLabel")}
      className="diagnosis-readiness-followups"
    >
      <div className="diagnosis-readiness-followup-heading">
        <strong>{t("nextEvidenceActions")}</strong>
        <span className="label-chip">{items.length}</span>
      </div>
      <ul className="diagnosis-readiness-followup-list">
        {visibleItems.map((item) => {
          const subReport = report.linked_sub_reports.find((entry) => entry.id === item.subReportID);
          if (!subReport) {
            return null;
          }
          return (
            <li className="diagnosis-readiness-followup-item" key={evidenceFollowUpKey(item)}>
              <div className="timeline-evidence-heading">
                <span className="label-chip">
                  {localizeReportEvidenceFollowUpKind(item.kind, t)}
                </span>
                <span className="label-chip">{item.priority}</span>
                <strong>{item.label}</strong>
              </div>
              <p className="muted">
                {item.kind === "evidence_request" && item.request
                  ? localizeReportEvidenceRequestDetail(item.request, t)
                  : item.detail}
              </p>
              <div className="diagnosis-readiness-followup-footer">
                <span className="muted">
                  {t("followUpMeta", {
                    id: item.evidenceSnapshotID,
                    title: item.subReportTitle,
                  })}
                </span>
                <Link
                  className="timeline-evidence-action link-button"
                  href={evidenceFollowUpHref(
                    reportDiagnosisRoomHref(report, subReport, rooms),
                    item,
                  )}
                >
                  {item.kind === "evidence_request"
                    ? t("usePlan")
                    : t("useInDiagnosis")}
                </Link>
              </div>
            </li>
          );
        })}
      </ul>
      {hiddenItemCount > 0 ? (
        <p className="muted">
          {t("moreEvidenceActions", { count: hiddenItemCount })}
        </p>
      ) : null}
    </section>
  );
}

function DiagnosisConclusion({
  conclusion,
  directoryUsersBySubject,
  diagnosisHref
}: {
  conclusion: ReportDiagnosisConclusion;
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  diagnosisHref: DiagnosisRoomHref;
}) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const metadata = diagnosisConclusionMetadata(
    conclusion,
    directoryUsersBySubject,
    t,
  );
  const supplementalRefs = conclusion.supplemental_context_refs ?? [];
  const confidenceTimeline = conclusion.confidence_timeline ?? [];
  const supplementalEvidence = conclusion.supplemental_evidence ?? [];
  const findings = conclusion.findings ?? [];
  const recommendedActions = conclusion.recommended_actions ?? [];
  const sourceLabel = localizeReportConclusionSource(conclusion.source, t);
  const reasonLabel = localizeReportConclusionReason(conclusion.reason, t);
  return (
    <div aria-label={t("diagnosisConclusionLabel")} className="subreport-conclusion">
      <div className="subreport-conclusion-header">
        <strong>{t("diagnosisConclusion")}</strong>
        <span className="muted">
          {conclusion.confidence
            ? t("confidence", {
                value: localizeDiagnosisRoomStatus(
                  conclusion.confidence,
                  tStatus,
                ),
              })
            : t("confidencePending")}
        </span>
      </div>
      <p>{conclusion.content}</p>
      {conclusion.confidence_rationale ? <p className="muted">{conclusion.confidence_rationale}</p> : null}
      <div className="muted">
        {conclusion.session_id} / {formatDateTime(conclusion.recorded_at, locale)}
      </div>
      <div className="subreport-conclusion-meta">
        {sourceLabel ? <span className="label-chip">{sourceLabel}</span> : null}
        {reasonLabel ? <span className="label-chip">{reasonLabel}</span> : null}
        {conclusion.requires_human_review ? (
          <span className="label-chip">{t("humanReviewTag")}</span>
        ) : null}
      </div>
      {metadata.length > 0 ? (
        <dl className="subreport-conclusion-details">
          {metadata.map((item) => (
            <div key={item.label}>
              <dt>{item.label}</dt>
              <dd>{item.value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {supplementalRefs.length > 0 ? (
        <div
          aria-label={t("contextReferences")}
          className="subreport-conclusion-meta"
        >
          {supplementalRefs.map((ref) => (
            <span className="label-chip" key={ref}>
              {ref}
            </span>
          ))}
        </div>
      ) : null}
      <DiagnosisConclusionStringList
        ariaLabel={t("conclusionListFor", { label: t("findings") })}
        label={t("findings")}
        items={findings}
      />
      <DiagnosisConclusionStringList
        ariaLabel={t("conclusionListFor", { label: t("actionsList") })}
        label={t("actionsList")}
        items={recommendedActions}
      />
      <TimelineEvidenceRequests diagnosisHref={diagnosisHref} item={conclusion} />
      {confidenceTimeline.length > 0 ? (
        <ConfidenceTimeline diagnosisHref={diagnosisHref} items={confidenceTimeline} />
      ) : null}
      <SupplementalEvidenceList items={supplementalEvidence} />
    </div>
  );
}

function DiagnosisConclusionStringList({
  ariaLabel,
  label,
  items
}: {
  ariaLabel: string;
  label: string;
  items: string[];
}) {
  const visibleItems = items.map((item) => item.trim()).filter((item) => item !== "");
  if (visibleItems.length === 0) {
    return null;
  }
  return (
    <section aria-label={ariaLabel} className="timeline-evidence-section">
      <strong>{label}</strong>
      <ul className="timeline-evidence-list">
        {visibleItems.map((item, index) => (
          <li className="timeline-evidence-item" key={`${item}:${index}`}>
            {item}
          </li>
        ))}
      </ul>
    </section>
  );
}

function DiagnosisProgress({
  diagnosisHref,
  progress
}: {
  diagnosisHref: DiagnosisRoomHref;
  progress: ReportDiagnosisProgress;
}) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const confidenceTimeline = progress.confidence_timeline ?? [];
  const supplementalEvidence = progress.supplemental_evidence ?? [];
  return (
    <div aria-label={t("diagnosisProgressLabel")} className="subreport-conclusion">
      <div className="subreport-conclusion-header">
        <strong>{t("diagnosisProgress")}</strong>
        <span className="muted">
          {t("confidence", {
            value: localizeDiagnosisRoomStatus(progress.confidence, tStatus),
          })}
        </span>
      </div>
      {progress.confidence_rationale ? <p>{progress.confidence_rationale}</p> : null}
      <div className="muted">
        {progress.session_id ??
          t("taskReference", { id: progress.diagnosis_task_id })} /{" "}
        {formatDateTime(progress.occurred_at, locale)}
      </div>
      <div className="subreport-conclusion-meta">
        <span className="label-chip">
          {localizeDiagnosisRoomStatus(progress.status, tStatus)}
        </span>
        {progress.conclusion_status ? (
          <span className="label-chip">
            {localizeDiagnosisRoomStatus(progress.conclusion_status, tStatus)}
          </span>
        ) : null}
        {progress.requires_human_review ? (
          <span className="label-chip">{t("humanReviewTag")}</span>
        ) : null}
      </div>
      <dl className="subreport-conclusion-details">
        <div>
          <dt>{t("task")}</dt>
          <dd>{progress.diagnosis_task_id}</dd>
        </div>
        <div>
          <dt>{t("evidence")}</dt>
          <dd>#{progress.evidence_snapshot_id}</dd>
        </div>
        <div>
          <dt>{t("requests")}</dt>
          <dd>{progress.evidence_request_count}</dd>
        </div>
      </dl>
      {confidenceTimeline.length > 0 ? (
        <ConfidenceTimeline diagnosisHref={diagnosisHref} items={confidenceTimeline} />
      ) : (
        <TimelineEvidenceRequests diagnosisHref={diagnosisHref} item={progress} />
      )}
      <SupplementalEvidenceList items={supplementalEvidence} />
    </div>
  );
}

function ConfidenceTimeline({
  diagnosisHref,
  items
}: {
  diagnosisHref: DiagnosisRoomHref;
  items: ReportConfidenceTimelineEntry[];
}) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const tStatus = useTranslations("DiagnosisRoom.status");
  return (
    <section aria-label={t("confidenceTimeline")} className="confidence-timeline">
      <strong>{t("confidenceTimeline")}</strong>
      <ol className="confidence-timeline-list">
        {items.map((item) => (
          <li className="confidence-timeline-item" key={confidenceTimelineKey(item)}>
            <div className="confidence-timeline-heading">
              <strong>
                {t("confidence", {
                  value: localizeDiagnosisRoomStatus(
                    item.confidence,
                    tStatus,
                  ),
                })}
              </strong>
              <span className="label-chip">
                {localizeDiagnosisRoomStatus(
                  item.conclusion_status ?? item.event_kind,
                  tStatus,
                )}
              </span>
            </div>
            {item.confidence_rationale ? <p>{item.confidence_rationale}</p> : null}
            <div className="muted">
              {t("evidenceRequestCount", {
                count: item.evidence_request_count,
              })} /{" "}
              {item.requires_human_review
                ? t("humanReviewRequired")
                : t("humanReviewCleared")} /{" "}
              {formatDateTime(item.occurred_at, locale)}
            </div>
            <TimelineEvidenceRequests diagnosisHref={diagnosisHref} item={item} />
          </li>
        ))}
      </ol>
    </section>
  );
}

function SupplementalEvidenceList({ items }: { items: ReportSupplementalEvidence[] }) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  if (items.length === 0) {
    return null;
  }
  return (
    <section aria-label={t("supplementalEvidence")} className="supplemental-evidence">
      <strong>{t("supplementalEvidence")}</strong>
      <ul className="supplemental-evidence-list">
        {items.map((item) => (
          <li className="supplemental-evidence-item" key={supplementalEvidenceKey(item)}>
            <div className="supplemental-evidence-heading">
              <strong>{item.label}</strong>
              <span className="label-chip">{item.priority}</span>
            </div>
            <p className="muted">{item.detail}</p>
            <p className="supplemental-evidence-text">{item.evidence}</p>
            <div className="muted">
              {t("providedAt", {
                time: formatDateTime(item.provided_at, locale),
              })}
            </div>
            {item.context_refs && item.context_refs.length > 0 ? (
              <div
                aria-label={t("contextReferencesFor", { label: item.label })}
                className="subreport-conclusion-meta"
              >
                {item.context_refs.map((ref) => (
                  <span className="label-chip" key={ref}>
                    {ref}
                  </span>
                ))}
              </div>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}

function supplementalEvidenceKey(item: ReportSupplementalEvidence) {
  return item.user_message_id ?? item.assistant_message_id ?? `${item.label}:${item.provided_at}`;
}

function confidenceTimelineKey(item: ReportConfidenceTimelineEntry) {
  return item.assistant_message_id ?? `${item.event_kind}:${item.occurred_at}`;
}

function TimelineEvidenceRequests({
  diagnosisHref,
  item
}: {
  diagnosisHref: DiagnosisRoomHref;
  item: ReportConfidenceTimelineEntry | ReportDiagnosisProgress | ReportDiagnosisConclusion;
}) {
  const t = useTranslations("ReportDetail");
  const tStatus = useTranslations("DiagnosisRoom.status");
  const evidenceRequests = item.evidence_requests ?? [];
  const collectionResults =
    "evidence_collection_results" in item ? item.evidence_collection_results ?? [] : [];
  const missingRequests = item.missing_evidence_requests ?? [];
  const collectionSuggestions = item.evidence_collection_suggestions ?? [];
  if (
    evidenceRequests.length === 0 &&
    collectionResults.length === 0 &&
    missingRequests.length === 0 &&
    collectionSuggestions.length === 0
  ) {
    return null;
  }
  return (
    <div className="timeline-evidence">
      {evidenceRequests.length > 0 ? (
        <section aria-label={t("requestedEvidence")} className="timeline-evidence-section">
          <strong>{t("requestedEvidence")}</strong>
          <ul className="timeline-evidence-list">
            {evidenceRequests.map((request) => {
              const collectionResult = reportEvidenceCollectionResultForRequest(
                request,
                collectionResults,
              );
              return (
                <li className="timeline-evidence-item" key={reportEvidenceRequestKey(request)}>
                  <div className="timeline-evidence-heading">
                    <span className="label-chip">{request.tool}</span>
                    <span className="label-chip">
                      {localizeDiagnosisRoomStatus(
                        collectionResult?.status ?? "pending",
                        tStatus,
                      )}
                    </span>
                    <strong>{request.reason}</strong>
                  </div>
                  {localizeReportEvidenceRequestDetail(request, t) ? (
                    <code>{localizeReportEvidenceRequestDetail(request, t)}</code>
                  ) : null}
                  {collectionResult?.status === "collected" ? null : (
                    <Link className="timeline-evidence-action link-button" href={evidencePlanHref(diagnosisHref, request)}>
                      {t("usePlan")}
                    </Link>
                  )}
                </li>
              );
            })}
          </ul>
        </section>
      ) : null}
      <CollectedEvidenceList items={collectionResults} />
      <ConsultationEvidenceList
        diagnosisHref={diagnosisHref}
        label={t("missingEvidence")}
        items={missingRequests}
      />
      <ConsultationEvidenceList
        diagnosisHref={diagnosisHref}
        label={t("collectionSuggestions")}
        items={collectionSuggestions}
      />
    </div>
  );
}

function CollectedEvidenceList({ items }: { items: ReportEvidenceCollectionResult[] }) {
  const locale = useLocale();
  const t = useTranslations("ReportDetail");
  const tStatus = useTranslations("DiagnosisRoom.status");
  if (items.length === 0) {
    return null;
  }
  return (
    <section aria-label={t("collectedEvidence")} className="timeline-evidence-section">
      <strong>{t("collectedEvidence")}</strong>
      <ul className="timeline-evidence-list">
        {items.map((item) => (
          <li className="timeline-evidence-item" key={evidenceCollectionResultKey(item)}>
            <div className="timeline-evidence-heading">
              <span className="label-chip">
                {localizeDiagnosisRoomStatus(item.status, tStatus)}
              </span>
              <strong>{item.message ?? item.request_reason ?? item.tool}</strong>
            </div>
            {item.message && item.request_reason ? <p className="muted">{item.request_reason}</p> : null}
            {localizeReportEvidenceCollectionResultDetail(item, t) ? (
              <code>
                {localizeReportEvidenceCollectionResultDetail(item, t)}
              </code>
            ) : null}
            <div className="muted">
              {t("collectedAt", {
                time: formatDateTime(item.collected_at, locale),
              })}
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

function ConsultationEvidenceList({
  diagnosisHref,
  label,
  items
}: {
  diagnosisHref: DiagnosisRoomHref;
  label: string;
  items: ReportConsultationEvidenceRequest[];
}) {
  const t = useTranslations("ReportDetail");
  if (items.length === 0) {
    return null;
  }
  return (
    <section aria-label={label} className="timeline-evidence-section">
      <strong>{label}</strong>
      <ul className="timeline-evidence-list">
        {items.map((item) => (
          <li className="timeline-evidence-item" key={consultationEvidenceKey(item)}>
            <div className="timeline-evidence-heading">
              <span className="label-chip">{item.priority}</span>
              <strong>{item.label}</strong>
            </div>
            <p className="muted">{item.detail}</p>
            <Link className="timeline-evidence-action link-button" href={supplementalFollowUpHref(diagnosisHref, item)}>
              {t("useInDiagnosis")}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

function evidenceCollectionResultKey(item: ReportEvidenceCollectionResult) {
  return [
    item.tool,
    item.status,
    item.reason_code ?? "",
    item.query ?? "",
    item.template_id ?? "",
    item.alert_source_profile_id ?? "",
    item.collected_at
  ].join(":");
}

function consultationEvidenceKey(
  item: ReportConsultationEvidenceRequest
) {
  return `${item.priority}:${item.label}:${item.detail}`;
}

function evidenceFollowUpKey(item: ReportEvidenceFollowUp) {
  return [
    item.subReportID,
    item.kind,
    item.priority,
    item.label,
    item.detail,
    item.request ? reportEvidenceRequestKey(item.request) : ""
  ].join(":");
}

function evidenceFollowUpHref(baseHref: DiagnosisRoomHref, item: ReportEvidenceFollowUp) {
  return item.kind === "evidence_request" && item.request
    ? evidencePlanHref(baseHref, item.request)
    : supplementalFollowUpHref(baseHref, item);
}

function supplementalFollowUpHref(baseHref: DiagnosisRoomHref, item: ReportConsultationEvidenceRequest) {
  return {
    pathname: baseHref.pathname,
    query: {
      ...baseHref.query,
      intent: "confidence_review",
      follow_up_detail: item.detail,
      follow_up_label: item.label,
      follow_up_priority: item.priority
    }
  };
}

function evidencePlanHref(baseHref: DiagnosisRoomHref, item: ReportEvidenceRequest) {
  return {
    pathname: baseHref.pathname,
    query: {
      ...baseHref.query,
      intent: "confidence_review",
      ...diagnosisEvidencePlanURLQuery(item)
    }
  };
}

function diagnosisConclusionMetadata(
  conclusion: ReportDiagnosisConclusion,
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>,
  t: ReportDetailTranslator,
) {
  const items: Array<{ label: string; value: ReactNode }> = [
    { label: t("task"), value: String(conclusion.diagnosis_task_id) },
    { label: t("chat"), value: String(conclusion.chat_session_id) },
    { label: t("event"), value: conclusion.event_kind }
  ];
  if (conclusion.evidence_snapshot_id !== undefined) {
    items.push({
      label: t("evidence"),
      value: `#${conclusion.evidence_snapshot_id}`,
    });
  }
  if (conclusion.conclusion_version) {
    items.push({ label: t("version"), value: conclusion.conclusion_version });
  }
  if (conclusion.confirmed_by) {
    items.push({
      label: t("confirmedBy"),
      value: (
        <ReportDirectorySubject
          directoryUsersBySubject={directoryUsersBySubject}
          subject={conclusion.confirmed_by}
        />
      ),
    });
  }
  return items;
}

function ReportDirectorySubject({
  directoryUsersBySubject,
  subject,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  subject?: string;
}) {
  const t = useTranslations("ReportDetail");
  const normalizedSubject = subject?.trim() ?? "";
  if (normalizedSubject === "") {
    return "-";
  }
  const profile = directorySubjectProfile(
    normalizedSubject,
    directoryUsersBySubject,
  );
  const isSystem = directorySubjectIsSystem(normalizedSubject);
  return (
    <span className="report-subject-chip-list">
      <span
        className={`label-chip report-subject-chip-${
          profile.matchedDirectoryUser ? "matched" : "fallback"
        }`}
      >
        {profile.displayName}
      </span>
      {profile.displayName !== profile.subject ? (
        <span className="label-chip">{profile.subject}</span>
      ) : null}
      {profile.detailTags.slice(0, 2).map((tag) => (
        <span className="label-chip" key={tag}>
          {tag}
        </span>
      ))}
      {profile.active === false ? (
        <span className="label-chip report-subject-chip-inactive">
          {t("inactive")}
        </span>
      ) : null}
      {!isSystem && !profile.matchedDirectoryUser ? (
        <span className="label-chip">{t("notSynced")}</span>
      ) : null}
      {isSystem ? <span className="label-chip">{t("system")}</span> : null}
    </span>
  );
}

function ErrorNotice({ message, status }: { message: string; status?: number }) {
  const t = useTranslations("ReportDetail");
  return (
    <div className="notice" role="alert">
      <strong>{status ? `HTTP ${status}` : t("requestFailed")}</strong>
      <div>{message}</div>
    </div>
  );
}

function PermissionLimitedNotice({
  detail,
  title
}: {
  detail: string;
  title: string;
}) {
  return (
    <div className="notice" role="status">
      <strong>{title}</strong>
      <div>{detail}</div>
    </div>
  );
}
