import Link from "next/link";

import { ReportShell } from "@/features/reports/report-shell";
import type { ApiResult } from "@/lib/api/client";

import { formatCount, formatSuccessRate } from "./format";
import type { DashboardSummary } from "./api";

type DashboardViewProps = {
  result: ApiResult<DashboardSummary>;
};

export function DashboardView({ result }: DashboardViewProps) {
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
      {result.ok ? <Summary dashboard={result.data} /> : null}
    </ReportShell>
  );
}

function Summary({ dashboard }: { dashboard: DashboardSummary }) {
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
      </div>
    </div>
  );
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
