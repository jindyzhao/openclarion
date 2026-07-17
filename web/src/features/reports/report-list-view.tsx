"use client";

import { SearchOutlined } from "@ant-design/icons";
import { Grid, Input, Select, Table, Typography } from "antd";
import type { TableColumnsType } from "antd";
import Link from "next/link";
import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";

import { formatDateTime, severityClass } from "./format";
import {
  localizeReportConfidence,
  localizeReportSeverity,
} from "./report-list-copy";
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
  const locale = useLocale();
  const t = useTranslations("ReportList");
  const [query, setQuery] = useState("");
  const [severity, setSeverity] = useState<SeverityFilter>("all");
  const reports = result.ok ? result.data.items : emptyReports;
  const filteredReports = useMemo(
    () => filterReports(reports, query, severity, locale),
    [locale, query, reports, severity]
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
            {report.generation_status === "partial" ? (
              <Typography.Text type="warning">
                {t("partialCoverage", {
                  failed: report.failed_sub_report_count,
                  expected: report.expected_sub_report_count,
                })}
              </Typography.Text>
            ) : null}
            <Typography.Text className="report-list-summary" type="secondary">
              {report.executive_summary}
            </Typography.Text>
            <div className="report-list-mobile-meta">
              <span className={severityClass(report.severity)}>
                {localizeReportSeverity(report.severity, t)}
              </span>
              <span>{localizeReportConfidence(report.confidence, t)}</span>
              <span>{formatDateTime(report.created_at, locale)}</span>
            </div>
          </div>
        ),
        title: t("report")
      },
      {
        dataIndex: "severity",
        key: "severity",
        render: (value: FinalReportSummary["severity"]) => (
          <span className={severityClass(value)}>
            {localizeReportSeverity(value, t)}
          </span>
        ),
        responsive: ["md"],
        title: t("severity"),
        width: 120
      },
      {
        dataIndex: "confidence",
        key: "confidence",
        render: (value: string) => (
          <span className="report-list-confidence">
            {localizeReportConfidence(value, t)}
          </span>
        ),
        responsive: ["md"],
        title: t("confidence"),
        width: 120
      },
      {
        dataIndex: "created_at",
        key: "created_at",
        render: (value: string) => formatDateTime(value, locale),
        responsive: ["md"],
        title: t("created"),
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
        title: t("correlation"),
        width: 240
      }
    ],
    [locale, t]
  );

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{t("title")}</h1>
          <p>{t("subtitle")}</p>
        </div>
        {result.ok ? (
          <div className="status-line">{t("count", { count: reports.length })}</div>
        ) : null}
      </section>

      {!result.ok ? (
        <ErrorNotice
          message={result.error.message}
          status={result.error.status}
          title={t("requestFailed")}
        />
      ) : null}

      {result.ok ? (
        <section aria-label={t("workspace")} className="report-list-workspace">
          <div className="report-list-toolbar">
            <Input
              allowClear
              aria-label={t("searchLabel")}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t("searchPlaceholder")}
              prefix={<SearchOutlined aria-hidden />}
              value={query}
            />
            <Select<SeverityFilter>
              aria-label={t("severityFilterLabel")}
              onChange={setSeverity}
              options={[
                { label: t("allSeverities"), value: "all" },
                { label: t("severityValue.critical"), value: "critical" },
                { label: t("severityValue.warning"), value: "warning" },
                { label: t("severityValue.info"), value: "info" }
              ]}
              value={severity}
            />
            <Typography.Text className="report-list-result-count" type="secondary">
              {t("shown", { count: filteredReports.length })}
            </Typography.Text>
          </div>

          <Table<FinalReportSummary>
            columns={columns}
            dataSource={filteredReports}
            locale={{
              emptyText: reports.length === 0 ? t("noReports") : t("noMatches")
            }}
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

function filterReports(
  reports: FinalReportSummary[],
  query: string,
  severity: SeverityFilter,
  locale: string,
): FinalReportSummary[] {
  const normalizedQuery = query.trim().toLocaleLowerCase(locale);
  return reports.filter((report) => {
    if (severity !== "all" && report.severity !== severity) {
      return false;
    }
    if (normalizedQuery === "") {
      return true;
    }
    return [report.title, report.executive_summary, report.correlation_key]
      .join("\n")
      .toLocaleLowerCase(locale)
      .includes(normalizedQuery);
  });
}

function ErrorNotice({
  message,
  status,
  title,
}: {
  message: string;
  status?: number;
  title: string;
}) {
  return (
    <div className="notice" role="alert">
      <strong>{status ? `HTTP ${status}` : title}</strong>
      <div>{message}</div>
    </div>
  );
}
