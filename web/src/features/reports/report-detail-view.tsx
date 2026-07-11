import Link from "next/link";
import type { ReactNode } from "react";

import type { DiagnosisRoomListResponse } from "@/features/diagnosis-room/api";
import { diagnosisEvidencePlanURLQuery } from "@/features/diagnosis-room/evidence-plan-url";
import {
  diagnosisFinalConclusionReasonLabel,
  diagnosisFinalConclusionSourceLabel,
} from "@/features/diagnosis-room/final-conclusion";
import type { DiagnosisReviewReturnState } from "@/features/diagnosis-room/report-return";
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
  subReportDiagnosisActionLabel,
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
  reportEvidenceRequestDetail,
  reportEvidenceRequestKey,
} from "./report-evidence-display";
import { ReportDeliveryProofPanel } from "./report-delivery-proof-panel";
import {
  reportDiagnosisDecisionRecords,
  type ReportDecisionRecord,
} from "./report-decision-records";
import { reportDiagnosisReviewReturnNotice } from "./report-return-notice";
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
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>{result.ok ? result.data.title : `Report ${reportId}`}</h1>
          <p>{result.ok ? result.data.correlation_key : "Report detail"}</p>
        </div>
        <Link className="status-line" href="/reports">
          Back to reports
        </Link>
      </div>

      {!result.ok ? <ErrorNotice message={result.error.message} status={result.error.status} /> : null}
      {diagnosisReviewReturn !== "none" && result.ok ? (
        <DiagnosisReviewReturnNotice
          notice={reportDiagnosisReviewReturnNotice(
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
  notice,
}: {
  notice: ReturnType<typeof reportDiagnosisReviewReturnNotice>;
}) {
  return (
    <div aria-label="Diagnosis review return" className="notice" role="status">
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
  const readiness = diagnosisReadiness(report);
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
          <h2>Summary</h2>
        </div>
        <div className="panel-body stack">
          <div>
            <span className={severityClass(report.severity)}>{report.severity}</span>{" "}
            <span className="muted">{report.confidence} confidence</span>
          </div>
          <p>{report.executive_summary}</p>
          <p className="muted">{report.notification_text}</p>
          <div className="muted">
            {report.created_by_workflow} / {formatDateTime(report.created_at)}
          </div>
        </div>
      </section>

      <aside className="panel" id="diagnosis-readiness">
        <div className="panel-header">
          <h2>Recommended Actions</h2>
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
          <h2>Diagnosis Readiness</h2>
        </div>
        <div className="panel-body">
          <div
            aria-label="Report AI review status"
            className={`diagnosis-readiness-summary diagnosis-readiness-summary-${readiness.status}`}
          >
            <span className="label-chip">AI review</span>
            <strong>{readiness.statusLabel}</strong>
            <p>{readiness.statusDetail}</p>
          </div>
          <div
            aria-label="Report diagnosis review queue"
            className="diagnosis-readiness-queue"
          >
            <div className="subreport-conclusion-meta">
              <span
                className={`label-chip diagnosis-status-${readiness.status}`}
              >
                {readiness.reviewQueueLabel}
              </span>
              {readiness.blockingReason ? (
                <span className="label-chip diagnosis-status-needs_evidence">
                  blocked
                </span>
              ) : null}
              {readiness.canConfirm ? (
                <span className="label-chip diagnosis-status-human_review">
                  confirmable
                </span>
              ) : null}
            </div>
            <p className="muted">{readiness.reviewQueueDetail}</p>
            <dl className="stat-list diagnosis-readiness-queue-stats">
              <div>
                <dt>Attention</dt>
                <dd>{readiness.attention}</dd>
              </div>
              <div>
                <dt>Pending</dt>
                <dd>{readiness.pending}</dd>
              </div>
              <div>
                <dt>Ready</dt>
                <dd>{readiness.ready}</dd>
              </div>
              <div>
                <dt>Done</dt>
                <dd>{readiness.done}</dd>
              </div>
            </dl>
          </div>
          <DiagnosisHandoffPlan handoff={handoff} />
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
          <dl aria-label="Diagnosis readiness" className="stat-list">
            <div>
              <dt>Reviewed subreports</dt>
              <dd>
                {readiness.reviewed} / {readiness.total}
              </dd>
            </div>
            <div>
              <dt>Human review</dt>
              <dd>{readiness.humanReviewRequired}</dd>
            </div>
            <div>
              <dt>Evidence requested</dt>
              <dd>{readiness.evidenceRequests}</dd>
            </div>
            <div>
              <dt>Evidence still needed</dt>
              <dd>{readiness.currentMissingEvidence}</dd>
            </div>
            <div>
              <dt>Executable evidence open</dt>
              <dd>{readiness.currentExecutableEvidenceRequests}</dd>
            </div>
            <div>
              <dt>Residual suggestions</dt>
              <dd>{readiness.currentCollectionSuggestions}</dd>
            </div>
            <div>
              <dt>Supplemental evidence</dt>
              <dd>{readiness.supplementalEvidence}</dd>
            </div>
            <div>
              <dt>Latest confidence</dt>
              <dd>{readiness.latestConfidence}</dd>
            </div>
          </dl>
        </div>
      </aside>

      <aside className="panel">
        <div className="panel-header">
          <h2>Report Delivery Proof</h2>
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
          <h2>AI Consultation Audit</h2>
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
          <h2>Evidence Traceability</h2>
        </div>
        <div className="panel-body">
          <ul className="subreport-list">
            {report.linked_sub_reports.map((subReport) => {
              const subReportReadiness = subReportDiagnosisReadiness(subReport);
              const diagnosisHref = reportDiagnosisRoomHref(report, subReport, rooms);
              return (
                <li className="subreport-item" key={subReport.id}>
                  <div className="subreport-item-header">
                    <h3>{subReport.title}</h3>
                    <Link
                      className="link-button"
                      href={diagnosisHref}
                    >
                      {subReportDiagnosisActionLabel(subReport)}
                    </Link>
                  </div>
                  <div className="muted">Evidence snapshot #{subReport.evidence_snapshot_id}</div>
                  <p>{subReport.summary}</p>
                  <span className={severityClass(subReport.severity)}>{subReport.severity}</span>{" "}
                  <span className="muted">{subReport.confidence} confidence</span>
                  <div
                    aria-label={`${subReport.title} AI review status`}
                    className="subreport-ai-status"
                  >
                    <span className={`label-chip diagnosis-status-${subReportReadiness.status}`}>
                      {subReportReadiness.statusLabel}
                    </span>
                    <span className="muted">{subReportReadiness.statusDetail}</span>
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
          <h2>Decision Records</h2>
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
  if (items.length === 0) {
    return (
      <p className="muted">
        No linked subreports are available for AI consultation audit.
      </p>
    );
  }
  return (
    <ul aria-label="AI consultation audit timeline" className="report-decision-record-list">
      {items.map((item) => {
        const subReport = report.linked_sub_reports.find(
          (entry) => entry.id === item.subReportID,
        );
        return (
          <li className="report-decision-record" key={item.subReportID}>
            <div className="report-decision-record-header">
              <div>
                <h3>{item.subReportTitle}</h3>
                <div className="muted">
                  Evidence #{item.evidenceSnapshotID}
                </div>
              </div>
              <span className={`label-chip diagnosis-status-${item.status}`}>
                {item.statusLabel}
              </span>
            </div>
            <p className="muted">{item.statusDetail}</p>
            <ol className="diagnosis-handoff-step-list">
              {item.steps.map((step) => (
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
                  {subReportDiagnosisActionLabel(subReport)}
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
  if (!action) {
    return null;
  }
  const subReport = report.linked_sub_reports.find(
    (item) => item.id === action.subReportID,
  );
  if (!subReport) {
    return null;
  }
  return (
    <section aria-label="Next diagnosis action" className="diagnosis-readiness-followups">
      <div className="diagnosis-readiness-followup-heading">
        <strong>Next diagnosis action</strong>
        <span className={`label-chip diagnosis-status-${action.status}`}>
          {action.statusLabel}
        </span>
      </div>
      <div className="diagnosis-readiness-followup-item">
        <div className="diagnosis-readiness-followup-footer">
          <strong>{action.title}</strong>
          <Link
            className="timeline-evidence-action link-button"
            href={reportDiagnosisRoomHref(report, subReport, rooms)}
          >
            {action.actionLabel}
          </Link>
        </div>
        <p className="muted">{action.detail}</p>
        <div className="muted">Evidence #{action.evidenceSnapshotID}</div>
      </div>
    </section>
  );
}

function DiagnosisHandoffPlan({ handoff }: { handoff: ReportDiagnosisHandoff }) {
  return (
    <section aria-label="Report diagnosis handoff plan" className="diagnosis-handoff-plan">
      <div className="diagnosis-handoff-plan-heading">
        <strong>Report handoff plan</strong>
        <span className="label-chip">{handoff.statusLabel}</span>
      </div>
      <p className="muted">{handoff.statusDetail}</p>
      <ol className="diagnosis-handoff-step-list">
        {handoff.steps.map((step) => (
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
  const rooms = roomsResult.ok ? roomsResult.data.items : [];
  const roomsRestricted = !roomsResult.ok && roomsResult.error.status === 403;
  return (
    <div className="report-decision-records">
      {roomsRestricted ? (
        <PermissionLimitedNotice
          detail="This report is visible through operations access, but your account cannot read diagnosis room proof or use recent rooms as link fallbacks."
          title="Diagnosis room proof is restricted."
        />
      ) : !roomsResult.ok ? (
        <div className="notice" role="status">
          <strong>Diagnosis room proof unavailable.</strong>
          <div>{roomsResult.error.message}</div>
        </div>
      ) : null}
      <ul aria-label="Report decision records" className="report-decision-record-list">
        {records.map((record) => (
          <li className="report-decision-record" key={record.subReportID}>
            <div className="report-decision-record-header">
              <div>
                <h3>{record.title}</h3>
                <div className="muted">
                  Evidence #{record.evidenceSnapshotID}
                  {record.sessionID ? ` / ${record.sessionID}` : ""}
                </div>
              </div>
              <span className={`label-chip report-decision-status-${record.status}`}>
                {record.statusLabel}
              </span>
            </div>
            <p>{record.detail}</p>
            <dl className="subreport-conclusion-details report-decision-record-details">
              <ReportDecisionRecordDetail label="Version" value={record.version || "-"} />
              <ReportDecisionRecordDetail
                label="Recorded"
                value={record.recordedAt ? formatDateTime(record.recordedAt) : "-"}
              />
              <ReportDecisionRecordDetail
                label="Confirmed by"
                value={
                  <ReportDirectorySubject
                    directoryUsersBySubject={directoryUsersBySubject}
                    subject={record.confirmedBy}
                  />
                }
              />
              <ReportDecisionRecordDetail label="Room status" value={record.roomStatus} />
              <ReportDecisionRecordDetail label="Room close" value={record.roomCloseDetail} />
              <ReportDecisionRecordDetail label="Notification" value={record.notificationLabel} />
            </dl>
            <p className="muted">{record.notificationDetail}</p>
            <div className="report-decision-record-actions">
              <Link
                className="link-button"
                href={decisionRecordDiagnosisHref(report, record, rooms)}
              >
                {decisionRecordDiagnosisActionLabel(record)}
              </Link>
              {record.notificationFailed ? (
                <a
                  className="status-line"
                  href={decisionRecordNotificationHref(record)}
                >
                  Review notification channel
                </a>
              ) : null}
            </div>
          </li>
        ))}
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

function decisionRecordDiagnosisActionLabel(record: ReportDecisionRecord) {
  switch (record.status) {
    case "confirmed":
      return "Review confirmed diagnosis";
    case "recorded":
      return "Confirm in diagnosis room";
    case "failed":
      return "Review failed diagnosis";
    case "needs_evidence":
      return "Resolve evidence in diagnosis room";
    case "pending_diagnosis":
    case "running":
      return "Continue diagnosis";
    case "room_closed":
      return "Review closed room";
  }
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
  if (items.length === 0) {
    return null;
  }
  const visibleItems = items.slice(0, 3);
  const hiddenItemCount = items.length - visibleItems.length;
  return (
    <section aria-label="Next diagnosis evidence actions" className="diagnosis-readiness-followups">
      <div className="diagnosis-readiness-followup-heading">
        <strong>Next evidence actions</strong>
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
                <span className="label-chip">{evidenceFollowUpLabel(item.kind)}</span>
                <span className="label-chip">{item.priority}</span>
                <strong>{item.label}</strong>
              </div>
              <p className="muted">{item.detail}</p>
              <div className="diagnosis-readiness-followup-footer">
                <span className="muted">
                  {item.subReportTitle} / Evidence #{item.evidenceSnapshotID}
                </span>
                <Link
                  className="timeline-evidence-action link-button"
                  href={evidenceFollowUpHref(
                    reportDiagnosisRoomHref(report, subReport, rooms),
                    item,
                  )}
                >
                  {item.kind === "evidence_request" ? "Use plan in diagnosis" : "Use in diagnosis"}
                </Link>
              </div>
            </li>
          );
        })}
      </ul>
      {hiddenItemCount > 0 ? (
        <p className="muted">
          {hiddenItemCount} more evidence action{hiddenItemCount === 1 ? "" : "s"} in the traceability list.
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
  const metadata = diagnosisConclusionMetadata(
    conclusion,
    directoryUsersBySubject,
  );
  const supplementalRefs = conclusion.supplemental_context_refs ?? [];
  const confidenceTimeline = conclusion.confidence_timeline ?? [];
  const supplementalEvidence = conclusion.supplemental_evidence ?? [];
  const findings = conclusion.findings ?? [];
  const recommendedActions = conclusion.recommended_actions ?? [];
  const sourceLabel = diagnosisFinalConclusionSourceLabel(conclusion.source);
  const reasonLabel = diagnosisFinalConclusionReasonLabel(conclusion.reason);
  return (
    <div aria-label="Diagnosis conclusion" className="subreport-conclusion">
      <div className="subreport-conclusion-header">
        <strong>AI diagnosis conclusion</strong>
        <span className="muted">{conclusion.confidence ? `${conclusion.confidence} confidence` : "confidence pending"}</span>
      </div>
      <p>{conclusion.content}</p>
      {conclusion.confidence_rationale ? <p className="muted">{conclusion.confidence_rationale}</p> : null}
      <div className="muted">
        {conclusion.session_id} / {formatDateTime(conclusion.recorded_at)}
      </div>
      <div className="subreport-conclusion-meta">
        {sourceLabel ? <span className="label-chip">{sourceLabel}</span> : null}
        {reasonLabel ? <span className="label-chip">{reasonLabel}</span> : null}
        {conclusion.requires_human_review ? <span className="label-chip">human review</span> : null}
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
        <div aria-label="Conclusion context references" className="subreport-conclusion-meta">
          {supplementalRefs.map((ref) => (
            <span className="label-chip" key={ref}>
              {ref}
            </span>
          ))}
        </div>
      ) : null}
      <DiagnosisConclusionStringList label="Findings" items={findings} />
      <DiagnosisConclusionStringList label="Recommended actions" items={recommendedActions} />
      <TimelineEvidenceRequests diagnosisHref={diagnosisHref} item={conclusion} />
      {confidenceTimeline.length > 0 ? (
        <ConfidenceTimeline diagnosisHref={diagnosisHref} items={confidenceTimeline} />
      ) : null}
      <SupplementalEvidenceList items={supplementalEvidence} />
    </div>
  );
}

function DiagnosisConclusionStringList({
  label,
  items
}: {
  label: string;
  items: string[];
}) {
  const visibleItems = items.map((item) => item.trim()).filter((item) => item !== "");
  if (visibleItems.length === 0) {
    return null;
  }
  return (
    <section aria-label={`Conclusion ${label.toLowerCase()}`} className="timeline-evidence-section">
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
  const confidenceTimeline = progress.confidence_timeline ?? [];
  const supplementalEvidence = progress.supplemental_evidence ?? [];
  return (
    <div aria-label="Diagnosis progress" className="subreport-conclusion">
      <div className="subreport-conclusion-header">
        <strong>AI diagnosis progress</strong>
        <span className="muted">{progress.confidence} confidence</span>
      </div>
      {progress.confidence_rationale ? <p>{progress.confidence_rationale}</p> : null}
      <div className="muted">
        {progress.session_id ?? `task #${progress.diagnosis_task_id}`} / {formatDateTime(progress.occurred_at)}
      </div>
      <div className="subreport-conclusion-meta">
        <span className="label-chip">{progress.status}</span>
        {progress.conclusion_status ? <span className="label-chip">{progress.conclusion_status}</span> : null}
        {progress.requires_human_review ? <span className="label-chip">human review</span> : null}
      </div>
      <dl className="subreport-conclusion-details">
        <div>
          <dt>Task</dt>
          <dd>{progress.diagnosis_task_id}</dd>
        </div>
        <div>
          <dt>Evidence</dt>
          <dd>#{progress.evidence_snapshot_id}</dd>
        </div>
        <div>
          <dt>Requests</dt>
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
  return (
    <section aria-label="Confidence timeline" className="confidence-timeline">
      <strong>Confidence timeline</strong>
      <ol className="confidence-timeline-list">
        {items.map((item) => (
          <li className="confidence-timeline-item" key={confidenceTimelineKey(item)}>
            <div className="confidence-timeline-heading">
              <strong>{item.confidence} confidence</strong>
              <span className="label-chip">{item.conclusion_status ?? item.event_kind}</span>
            </div>
            {item.confidence_rationale ? <p>{item.confidence_rationale}</p> : null}
            <div className="muted">
              {item.evidence_request_count} evidence request{item.evidence_request_count === 1 ? "" : "s"} /{" "}
              {item.requires_human_review ? "human review required" : "human review cleared"} /{" "}
              {formatDateTime(item.occurred_at)}
            </div>
            <TimelineEvidenceRequests diagnosisHref={diagnosisHref} item={item} />
          </li>
        ))}
      </ol>
    </section>
  );
}

function SupplementalEvidenceList({ items }: { items: ReportSupplementalEvidence[] }) {
  if (items.length === 0) {
    return null;
  }
  return (
    <section aria-label="Supplemental evidence" className="supplemental-evidence">
      <strong>Supplemental evidence</strong>
      <ul className="supplemental-evidence-list">
        {items.map((item) => (
          <li className="supplemental-evidence-item" key={supplementalEvidenceKey(item)}>
            <div className="supplemental-evidence-heading">
              <strong>{item.label}</strong>
              <span className="label-chip">{item.priority}</span>
            </div>
            <p className="muted">{item.detail}</p>
            <p className="supplemental-evidence-text">{item.evidence}</p>
            <div className="muted">Provided {formatDateTime(item.provided_at)}</div>
            {item.context_refs && item.context_refs.length > 0 ? (
              <div aria-label={`${item.label} context references`} className="subreport-conclusion-meta">
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
        <section aria-label="Requested evidence" className="timeline-evidence-section">
          <strong>Requested evidence</strong>
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
                      {collectionResult?.status ?? "pending"}
                    </span>
                    <strong>{request.reason}</strong>
                  </div>
                  {reportEvidenceRequestDetail(request) ? <code>{reportEvidenceRequestDetail(request)}</code> : null}
                  {collectionResult?.status === "collected" ? null : (
                    <Link className="timeline-evidence-action link-button" href={evidencePlanHref(diagnosisHref, request)}>
                      Use plan in diagnosis
                    </Link>
                  )}
                </li>
              );
            })}
          </ul>
        </section>
      ) : null}
      <CollectedEvidenceList items={collectionResults} />
      <ConsultationEvidenceList diagnosisHref={diagnosisHref} label="Missing evidence" items={missingRequests} />
      <ConsultationEvidenceList
        diagnosisHref={diagnosisHref}
        label="Collection suggestions"
        items={collectionSuggestions}
      />
    </div>
  );
}

function CollectedEvidenceList({ items }: { items: ReportEvidenceCollectionResult[] }) {
  if (items.length === 0) {
    return null;
  }
  return (
    <section aria-label="Collected evidence" className="timeline-evidence-section">
      <strong>Collected evidence</strong>
      <ul className="timeline-evidence-list">
        {items.map((item) => (
          <li className="timeline-evidence-item" key={evidenceCollectionResultKey(item)}>
            <div className="timeline-evidence-heading">
              <span className="label-chip">{item.status}</span>
              <strong>{item.message ?? item.request_reason ?? item.tool}</strong>
            </div>
            {item.message && item.request_reason ? <p className="muted">{item.request_reason}</p> : null}
            {evidenceCollectionResultDetail(item) ? <code>{evidenceCollectionResultDetail(item)}</code> : null}
            <div className="muted">Collected {formatDateTime(item.collected_at)}</div>
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
              Use in diagnosis
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

function evidenceCollectionResultDetail(item: ReportEvidenceCollectionResult) {
  const parts = [
    item.tool,
    item.reason_code ? `reason: ${item.reason_code}` : undefined,
    item.query ? `query: ${item.query}` : undefined,
    item.template_id !== undefined ? `template #${item.template_id}` : undefined,
    item.alert_source_profile_id !== undefined ? `source #${item.alert_source_profile_id}` : undefined,
    item.alert_source_kind ? `source: ${item.alert_source_kind}` : undefined,
    item.observed_alerts !== undefined ? `${item.observed_alerts} alert${item.observed_alerts === 1 ? "" : "s"}` : undefined,
    item.observed_metric_series !== undefined
      ? `${item.observed_metric_series} metric series`
      : undefined,
    item.window_seconds !== undefined ? `window ${item.window_seconds}s` : undefined,
    item.step_seconds !== undefined ? `step ${item.step_seconds}s` : undefined,
    item.limit !== undefined ? `limit ${item.limit}` : undefined
  ].filter((part): part is string => part !== undefined);
  return parts.join(" / ");
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

function evidenceFollowUpLabel(kind: ReportEvidenceFollowUp["kind"]) {
  switch (kind) {
    case "collection_suggestion":
      return "Collection suggestion";
    case "evidence_request":
      return "Evidence plan";
    case "missing_evidence":
      return "Missing evidence";
  }
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
) {
  const items: Array<{ label: string; value: ReactNode }> = [
    { label: "Task", value: String(conclusion.diagnosis_task_id) },
    { label: "Chat", value: String(conclusion.chat_session_id) },
    { label: "Event", value: conclusion.event_kind }
  ];
  if (conclusion.evidence_snapshot_id !== undefined) {
    items.push({ label: "Evidence", value: `#${conclusion.evidence_snapshot_id}` });
  }
  if (conclusion.conclusion_version) {
    items.push({ label: "Version", value: conclusion.conclusion_version });
  }
  if (conclusion.confirmed_by) {
    items.push({
      label: "Confirmed by",
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
        <span className="label-chip report-subject-chip-inactive">inactive</span>
      ) : null}
      {!isSystem && !profile.matchedDirectoryUser ? (
        <span className="label-chip">not synced</span>
      ) : null}
      {isSystem ? <span className="label-chip">system</span> : null}
    </span>
  );
}

function ErrorNotice({ message, status }: { message: string; status?: number }) {
  return (
    <div className="notice" role="alert">
      <strong>{status ? `HTTP ${status}` : "Request failed"}</strong>
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
