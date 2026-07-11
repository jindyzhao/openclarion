"use client";

import { SearchOutlined } from "@ant-design/icons";
import { Grid, Input, Select, Table, Typography } from "antd";
import type { TableColumnsType } from "antd";
import Link from "next/link";
import { useMemo, useState } from "react";

import { formatDateTime, severityClass } from "./format";
import type { ApiResult, FinalReportSummary } from "./types";
import type { components } from "@/lib/api/openapi";

type ReportListResponse = components["schemas"]["ReportListResponse"];

type ReportListViewProps = {
  result: ApiResult<ReportListResponse>;
};

type SeverityFilter = "all" | FinalReportSummary["severity"];
const emptyReports: FinalReportSummary[] = [];

export function ReportListView({ result }: ReportListViewProps) {
  const screens = Grid.useBreakpoint();
  const [query, setQuery] = useState("");
  const [severity, setSeverity] = useState<SeverityFilter>("all");
  const reports = result.ok ? result.data.items : emptyReports;
  const filteredReports = useMemo(
    () => filterReports(reports, query, severity),
    [query, reports, severity]
  );

  const columns = useMemo<TableColumnsType<FinalReportSummary>>(
    () => [
      {
        key: "report",
        render: (_value, report) => (
          <div className="report-list-primary">
            <Link className="report-list-title" href={`/reports/${report.id}`}>
              {report.title}
            </Link>
            <Typography.Text className="report-list-summary" type="secondary">
              {report.executive_summary}
            </Typography.Text>
            <div className="report-list-mobile-meta">
              <span className={severityClass(report.severity)}>{report.severity}</span>
              <span>{report.confidence}</span>
              <span>{formatDateTime(report.created_at)}</span>
            </div>
          </div>
        ),
        title: "Report"
      },
      {
        dataIndex: "severity",
        key: "severity",
        render: (value: FinalReportSummary["severity"]) => <span className={severityClass(value)}>{value}</span>,
        responsive: ["md"],
        title: "Severity",
        width: 120
      },
      {
        dataIndex: "confidence",
        key: "confidence",
        render: (value: string) => <span className="report-list-confidence">{value}</span>,
        responsive: ["md"],
        title: "Confidence",
        width: 120
      },
      {
        dataIndex: "created_at",
        key: "created_at",
        render: (value: string) => formatDateTime(value),
        responsive: ["md"],
        title: "Created",
        width: 190
      },
      {
        dataIndex: "correlation_key",
        ellipsis: { showTitle: false },
        key: "correlation_key",
        render: (value: string) => (
          <Typography.Text ellipsis={{ tooltip: value }} type="secondary">
            {value}
          </Typography.Text>
        ),
        responsive: ["md"],
        title: "Correlation",
        width: 240
      }
    ],
    []
  );

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>Reports</h1>
          <p>Final reports generated from persisted evidence snapshots.</p>
        </div>
        {result.ok ? <div className="status-line">{reports.length} report{reports.length === 1 ? "" : "s"}</div> : null}
      </section>

      {!result.ok ? <ErrorNotice message={result.error.message} status={result.error.status} /> : null}

      {result.ok ? (
        <section aria-label="Reports workspace" className="report-list-workspace">
          <div className="report-list-toolbar">
            <Input
              allowClear
              aria-label="Search reports"
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search title, summary, or correlation"
              prefix={<SearchOutlined aria-hidden />}
              value={query}
            />
            <Select<SeverityFilter>
              aria-label="Filter reports by severity"
              onChange={setSeverity}
              options={[
                { label: "All severities", value: "all" },
                { label: "Critical", value: "critical" },
                { label: "Warning", value: "warning" },
                { label: "Info", value: "info" }
              ]}
              value={severity}
            />
            <Typography.Text className="report-list-result-count" type="secondary">
              {filteredReports.length} shown
            </Typography.Text>
          </div>

          <Table<FinalReportSummary>
            columns={columns}
            dataSource={filteredReports}
            locale={{ emptyText: reports.length === 0 ? "No reports available." : "No reports match these filters." }}
            pagination={{ hideOnSinglePage: true, pageSize: 25, showSizeChanger: false }}
            rowKey="id"
            scroll={screens.md ? { x: 1090 } : undefined}
            size="middle"
          />
        </section>
      ) : null}
    </>
  );
}

function filterReports(reports: FinalReportSummary[], query: string, severity: SeverityFilter): FinalReportSummary[] {
  const normalizedQuery = query.trim().toLocaleLowerCase("en");
  return reports.filter((report) => {
    if (severity !== "all" && report.severity !== severity) {
      return false;
    }
    if (normalizedQuery === "") {
      return true;
    }
    return [report.title, report.executive_summary, report.correlation_key]
      .join("\n")
      .toLocaleLowerCase("en")
      .includes(normalizedQuery);
  });
}

function ErrorNotice({ message, status }: { message: string; status?: number }) {
  return (
    <div className="notice" role="alert">
      <strong>{status ? `HTTP ${status}` : "Request failed"}</strong>
      <div>{message}</div>
    </div>
  );
}
