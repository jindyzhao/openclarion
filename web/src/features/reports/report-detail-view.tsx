import Link from "next/link";

import { formatDateTime, severityClass } from "./format";
import { ReportShell } from "./report-shell";
import type { ApiResult, FinalReportDetail } from "./types";

type ReportDetailViewProps = {
  reportId: string;
  result: ApiResult<FinalReportDetail>;
};

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

      <section className="panel">
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
                    href={`/diagnosis-room?evidence_snapshot_id=${subReport.evidence_snapshot_id}`}
                  >
                    Start diagnosis
                  </Link>
                </div>
                <div className="muted">Evidence snapshot #{subReport.evidence_snapshot_id}</div>
                <p>{subReport.summary}</p>
                <span className={severityClass(subReport.severity)}>{subReport.severity}</span>{" "}
                <span className="muted">{subReport.confidence} confidence</span>
                {subReport.diagnosis_conclusion ? (
                  <DiagnosisConclusion conclusion={subReport.diagnosis_conclusion} />
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      </section>
    </div>
  );
}

function DiagnosisConclusion({
  conclusion
}: {
  conclusion: NonNullable<FinalReportDetail["linked_sub_reports"][number]["diagnosis_conclusion"]>;
}) {
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
    </div>
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
