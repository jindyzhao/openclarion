import Link from "next/link";

import { formatDateTime, severityClass } from "./format";
import { ReportShell } from "./report-shell";
import type { ApiResult, FinalReportDetail } from "./types";

type ReportDetailViewProps = {
  reportId: string;
  result: ApiResult<FinalReportDetail>;
};

type ReportDiagnosisConclusion = NonNullable<
  FinalReportDetail["linked_sub_reports"][number]["diagnosis_conclusion"]
>;
type DiagnosisRoomHref = ReturnType<typeof diagnosisRoomHref>;
type ReportConsultationEvidenceRequest = NonNullable<
  NonNullable<ReportDiagnosisConclusion["confidence_timeline"]>[number]["missing_evidence_requests"]
>[number];

export function ReportDetailView({ reportId, result }: ReportDetailViewProps) {
  return (
    <ReportShell current="reports">
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
      {result.ok ? <Detail report={result.data} /> : null}
    </ReportShell>
  );
}

function Detail({ report }: { report: FinalReportDetail }) {
  const readiness = diagnosisReadiness(report);
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
            {report.created_by_workflow} · {formatDateTime(report.created_at)}
          </div>
        </div>
      </section>

      <aside className="panel">
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

      <aside className="panel">
        <div className="panel-header">
          <h2>Diagnosis Readiness</h2>
        </div>
        <div className="panel-body">
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

      <section className="panel detail-grid-wide">
        <div className="panel-header">
          <h2>Evidence Traceability</h2>
        </div>
        <div className="panel-body">
          <ul className="subreport-list">
            {report.linked_sub_reports.map((subReport) => (
              <li className="subreport-item" key={subReport.id}>
                <div className="subreport-item-header">
                  <h3>{subReport.title}</h3>
                  <Link
                    className="link-button"
                    href={diagnosisRoomHref(report, subReport)}
                  >
                    {subReport.diagnosis_conclusion ? "Review diagnosis" : "Start diagnosis"}
                  </Link>
                </div>
                <div className="muted">Evidence snapshot #{subReport.evidence_snapshot_id}</div>
                <p>{subReport.summary}</p>
                <span className={severityClass(subReport.severity)}>{subReport.severity}</span>{" "}
                <span className="muted">{subReport.confidence} confidence</span>
                {subReport.diagnosis_conclusion ? (
                  <DiagnosisConclusion
                    conclusion={subReport.diagnosis_conclusion}
                    diagnosisHref={diagnosisRoomHref(report, subReport)}
                  />
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      </section>
    </div>
  );
}

function diagnosisRoomHref(
  report: FinalReportDetail,
  subReport: FinalReportDetail["linked_sub_reports"][number]
) {
  return {
    pathname: "/diagnosis-room",
    query: {
      evidence_snapshot_id: String(subReport.evidence_snapshot_id),
      report_id: String(report.id),
      sub_report_id: String(subReport.id),
      intent: subReport.diagnosis_conclusion ? "review_conclusion" : "confidence_review"
    }
  };
}

function diagnosisReadiness(report: FinalReportDetail) {
  const conclusions = report.linked_sub_reports
    .map((subReport) => subReport.diagnosis_conclusion)
    .filter((conclusion): conclusion is ReportDiagnosisConclusion => conclusion !== undefined);
  const supplementalEvidence = conclusions.reduce(
    (count, conclusion) => count + (conclusion.supplemental_evidence?.length ?? 0),
    0
  );
  const humanReviewRequired = conclusions.filter((conclusion) => conclusion.requires_human_review).length;
  const latestConclusion = conclusions.reduce<ReportDiagnosisConclusion | undefined>((latest, conclusion) => {
    if (!latest) {
      return conclusion;
    }
    return Date.parse(conclusion.recorded_at) >= Date.parse(latest.recorded_at) ? conclusion : latest;
  }, undefined);
  return {
    total: report.linked_sub_reports.length,
    reviewed: conclusions.length,
    humanReviewRequired,
    supplementalEvidence,
    latestConfidence: latestConclusion?.confidence ?? "pending"
  };
}

function DiagnosisConclusion({
  conclusion,
  diagnosisHref
}: {
  conclusion: ReportDiagnosisConclusion;
  diagnosisHref: DiagnosisRoomHref;
}) {
  const metadata = diagnosisConclusionMetadata(conclusion);
  const supplementalRefs = conclusion.supplemental_context_refs ?? [];
  const confidenceTimeline = conclusion.confidence_timeline ?? [];
  const supplementalEvidence = conclusion.supplemental_evidence ?? [];
  return (
    <div aria-label="Diagnosis conclusion" className="subreport-conclusion">
      <div className="subreport-conclusion-header">
        <strong>AI diagnosis conclusion</strong>
        <span className="muted">{conclusion.confidence ? `${conclusion.confidence} confidence` : "confidence pending"}</span>
      </div>
      <p>{conclusion.content}</p>
      <div className="muted">
        {conclusion.session_id} · {formatDateTime(conclusion.recorded_at)}
      </div>
      <div className="subreport-conclusion-meta">
        <span className="label-chip">{conclusion.source}</span>
        {conclusion.reason ? <span className="label-chip">{conclusion.reason}</span> : null}
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
      {confidenceTimeline.length > 0 ? (
        <section aria-label="Confidence timeline" className="confidence-timeline">
          <strong>Confidence timeline</strong>
          <ol className="confidence-timeline-list">
            {confidenceTimeline.map((item) => (
              <li className="confidence-timeline-item" key={confidenceTimelineKey(item)}>
                <div className="confidence-timeline-heading">
                  <strong>{item.confidence} confidence</strong>
                  <span className="label-chip">{item.conclusion_status ?? item.event_kind}</span>
                </div>
                {item.confidence_rationale ? <p>{item.confidence_rationale}</p> : null}
                <div className="muted">
                  {item.evidence_request_count} evidence request{item.evidence_request_count === 1 ? "" : "s"} ·{" "}
                  {item.requires_human_review ? "human review required" : "human review cleared"} ·{" "}
                  {formatDateTime(item.occurred_at)}
                </div>
                <TimelineEvidenceRequests diagnosisHref={diagnosisHref} item={item} />
              </li>
            ))}
          </ol>
        </section>
      ) : null}
      {supplementalEvidence.length > 0 ? (
        <section aria-label="Supplemental evidence" className="supplemental-evidence">
          <strong>Supplemental evidence</strong>
          <ul className="supplemental-evidence-list">
            {supplementalEvidence.map((item) => (
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
      ) : null}
    </div>
  );
}

function supplementalEvidenceKey(
  item: NonNullable<ReportDiagnosisConclusion["supplemental_evidence"]>[number]
) {
  return item.user_message_id ?? item.assistant_message_id ?? `${item.label}:${item.provided_at}`;
}

function confidenceTimelineKey(
  item: NonNullable<ReportDiagnosisConclusion["confidence_timeline"]>[number]
) {
  return item.assistant_message_id ?? `${item.event_kind}:${item.occurred_at}`;
}

function TimelineEvidenceRequests({
  diagnosisHref,
  item
}: {
  diagnosisHref: DiagnosisRoomHref;
  item: NonNullable<ReportDiagnosisConclusion["confidence_timeline"]>[number];
}) {
  const evidenceRequests = item.evidence_requests ?? [];
  const missingRequests = item.missing_evidence_requests ?? [];
  const collectionSuggestions = item.evidence_collection_suggestions ?? [];
  if (evidenceRequests.length === 0 && missingRequests.length === 0 && collectionSuggestions.length === 0) {
    return null;
  }
  return (
    <div className="timeline-evidence">
      {evidenceRequests.length > 0 ? (
        <section aria-label="Requested evidence" className="timeline-evidence-section">
          <strong>Requested evidence</strong>
          <ul className="timeline-evidence-list">
            {evidenceRequests.map((request) => (
              <li className="timeline-evidence-item" key={evidenceRequestKey(request)}>
                <div className="timeline-evidence-heading">
                  <span className="label-chip">{request.tool}</span>
                  <strong>{request.reason}</strong>
                </div>
                {evidenceRequestDetail(request) ? <code>{evidenceRequestDetail(request)}</code> : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      <ConsultationEvidenceList diagnosisHref={diagnosisHref} label="Missing evidence" items={missingRequests} />
      <ConsultationEvidenceList
        diagnosisHref={diagnosisHref}
        label="Collection suggestions"
        items={collectionSuggestions}
      />
    </div>
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

function evidenceRequestDetail(
  request: NonNullable<
    NonNullable<ReportDiagnosisConclusion["confidence_timeline"]>[number]["evidence_requests"]
  >[number]
) {
  const parts = [
    request.query ? `query: ${request.query}` : undefined,
    request.template_id !== undefined ? `template #${request.template_id}` : undefined,
    request.window_seconds !== undefined ? `window ${request.window_seconds}s` : undefined,
    request.step_seconds !== undefined ? `step ${request.step_seconds}s` : undefined,
    request.limit !== undefined ? `limit ${request.limit}` : undefined
  ].filter((part): part is string => part !== undefined);
  return parts.join(" · ");
}

function evidenceRequestKey(
  request: NonNullable<
    NonNullable<ReportDiagnosisConclusion["confidence_timeline"]>[number]["evidence_requests"]
  >[number]
) {
  return [
    request.tool,
    request.reason,
    request.query ?? "",
    request.template_id ?? "",
    request.window_seconds ?? "",
    request.step_seconds ?? "",
    request.limit ?? ""
  ].join(":");
}

function consultationEvidenceKey(
  item: ReportConsultationEvidenceRequest
) {
  return `${item.priority}:${item.label}:${item.detail}`;
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

function diagnosisConclusionMetadata(conclusion: ReportDiagnosisConclusion) {
  const items: Array<{ label: string; value: string }> = [
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
    items.push({ label: "Confirmed by", value: conclusion.confirmed_by });
  }
  return items;
}

function ErrorNotice({ message, status }: { message: string; status?: number }) {
  return (
    <div className="notice" role="alert">
      <strong>{status ? `HTTP ${status}` : "Request failed"}</strong>
      <div>{message}</div>
    </div>
  );
}
