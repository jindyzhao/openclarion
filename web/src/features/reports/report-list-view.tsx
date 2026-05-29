import Link from "next/link";

import { displayStatus, formatDateTime, severityClass } from "./format";
import { ReportShell } from "./report-shell";
import type { ApiResult } from "./types";
import type { components } from "@/lib/api/openapi";

type ReportListResponse = components["schemas"]["ReportListResponse"];

type ReportListViewProps = {
  result: ApiResult<ReportListResponse>;
};

export function ReportListView({ result }: ReportListViewProps) {
  return (
    <ReportShell current="reports">
      <section className="page-heading">
        <div>
          <h1>Reports</h1>
          <p>Final reports generated from persisted evidence snapshots.</p>
        </div>
        {result.ok ? (
          <div className="status-line">
            {displayStatus({ count: result.data.items.length, fetchedAt: new Date() })}
          </div>
        ) : null}
      </section>

      {!result.ok ? <ErrorNotice message={result.error.message} status={result.error.status} /> : null}

      {result.ok && result.data.items.length === 0 ? (
        <div className="notice">
          <strong>No reports available.</strong>
        </div>
      ) : null}

      {result.ok && result.data.items.length > 0 ? (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Title</th>
                <th>Severity</th>
                <th>Confidence</th>
                <th>Created</th>
                <th>Correlation</th>
              </tr>
            </thead>
            <tbody>
              {result.data.items.map((report) => (
                <tr key={report.id}>
                  <td className="title-cell">
                    <Link href={`/reports/${report.id}`}>{report.title}</Link>
                    <div className="muted">{report.executive_summary}</div>
                  </td>
                  <td>
                    <span className={severityClass(report.severity)}>{report.severity}</span>
                  </td>
                  <td>{report.confidence}</td>
                  <td>{formatDateTime(report.created_at)}</td>
                  <td className="muted">{report.correlation_key}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </ReportShell>
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
