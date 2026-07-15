import { describe, expect, it } from "vitest";

import {
  diagnosisReadiness,
  reportConsultationAuditTimeline,
  reportDiagnosisHandoff,
  reportDiagnosisNextAction,
  reportEvidenceFollowUps,
  reportFinalNotificationReadiness,
  subReportDiagnosisReadiness,
  type ReportDiagnosisConclusion,
  type ReportDiagnosisProgress,
} from "./diagnosis-readiness";
import type { FinalReportDetail } from "./types";

type TimelineEntry = NonNullable<
  ReportDiagnosisConclusion["confidence_timeline"]
>[number];
type LinkedSubReport = FinalReportDetail["linked_sub_reports"][number];

describe("diagnosis readiness", () => {
  it("marks reviewed reports as ready for human review after evidence is resolved", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence: "high",
          requires_human_review: true,
          confidence_timeline: [
            timelineEntry({
              confidence: "low",
              conclusion_status: "needs_evidence",
              evidence_request_count: 1,
              evidence_requests: [
                {
                  reason: "Need deployment timing.",
                  tool: "metric_range_query",
                },
              ],
              missing_evidence_requests: [
                {
                  detail: "Attach the deployment window.",
                  label: "Deployment window",
                  priority: "high",
                },
              ],
              occurred_at: "2026-06-18T08:00:00Z",
            }),
            timelineEntry({
              confidence: "high",
              conclusion_status: "ready_for_review",
              evidence_collection_results: [
                {
                  collected_at: "2026-06-18T08:05:00Z",
                  status: "collected",
                  tool: "metric_range_query",
                },
              ],
              occurred_at: "2026-06-18T08:06:00Z",
            }),
          ],
          supplemental_evidence: [
            {
              detail: "Compare the deployment with the alert onset.",
              evidence:
                "Deployment started two minutes before latency crossed the warning threshold.",
              label: "Deployment window",
              priority: "high",
              provided_at: "2026-06-18T08:04:00Z",
            },
          ],
        }),
      }),
    ]);

    expect(diagnosisReadiness(report)).toMatchObject({
      attention: 0,
      blocked: false,
      canConfirm: true,
      collectedEvidence: 1,
      currentCollectionSuggestions: 0,
      currentMissingEvidence: 0,
      done: 0,
      evidenceRequests: 1,
      humanReviewRequired: 1,
      latestConfidence: "high",
      pending: 0,
      ready: 1,
      reviewed: 1,
      status: "human_review",
      supplementalEvidence: 1,
      total: 1,
    });
  });

  it("builds an AI consultation audit timeline from initial report to operator decision", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence: "high",
          confidence_timeline: [
            timelineEntry({
              confidence: "low",
              conclusion_status: "needs_evidence",
              evidence_requests: [
                {
                  query: "sum(rate(checkout_requests_total[5m]))",
                  reason: "Need traffic trend.",
                  tool: "metric_range_query",
                },
              ],
              missing_evidence_requests: [
                {
                  detail: "Attach the deployment window.",
                  label: "Deployment window",
                  priority: "high",
                },
              ],
              occurred_at: "2026-06-18T08:00:00Z",
            }),
            timelineEntry({
              confidence: "high",
              conclusion_status: "ready_for_review",
              evidence_collection_results: [
                {
                  collected_at: "2026-06-18T08:05:00Z",
                  status: "collected",
                  tool: "metric_range_query",
                },
              ],
              occurred_at: "2026-06-18T08:06:00Z",
            }),
          ],
          requires_human_review: true,
          supplemental_evidence: [
            {
              detail: "Compare the deployment with the alert onset.",
              evidence:
                "Deployment started two minutes before latency crossed the warning threshold.",
              label: "Deployment window",
              priority: "high",
              provided_at: "2026-06-18T08:04:00Z",
            },
          ],
        }),
      }),
    ]);

    expect(reportConsultationAuditTimeline(report)).toEqual([
      expect.objectContaining({
        confirmed: false,
        evidenceSnapshotID: 9001,
        hasDiagnosisState: true,
        initialConfidence: "low",
        readiness: expect.objectContaining({ status: "human_review" }),
        steps: [
          {
            key: "initial_report",
            status: "done",
          },
          {
            key: "supplemental_evidence",
            status: "done",
          },
          {
            key: "confidence_revision",
            status: "done",
          },
          {
            key: "final_decision",
            status: "pending",
          },
        ],
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      }),
    ]);
  });

  it("marks consultation audit steps pending before AI diagnosis starts", () => {
    const report = reportDetail([linkedSubReport()]);

    expect(reportConsultationAuditTimeline(report)).toEqual([
      expect.objectContaining({
        evidenceSnapshotID: 9001,
        hasDiagnosisState: false,
        readiness: expect.objectContaining({ status: "pending_diagnosis" }),
        steps: [
          expect.objectContaining({
            key: "initial_report",
            status: "pending",
          }),
          expect.objectContaining({
            key: "supplemental_evidence",
            status: "pending",
          }),
          expect.objectContaining({
            key: "confidence_revision",
            status: "pending",
          }),
          expect.objectContaining({
            key: "final_decision",
            status: "pending",
          }),
        ],
        subReportID: 501,
      }),
    ]);
  });

  it("marks operator-confirmed conclusions complete even when review was required", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confirmed_by: "owner-1",
          conclusion_version: "diagnosis-session-301:2",
          evidence_collection_suggestions: [
            {
              detail: "Collect the stale pre-confirmation latency trend.",
              label: "Stale latency trend",
              priority: "medium",
            },
          ],
          missing_evidence_requests: [
            {
              detail: "Attach the stale owner note.",
              label: "Stale owner note",
              priority: "high",
            },
          ],
          requires_human_review: true,
        }),
      }),
    ]);

    expect(subReportDiagnosisReadiness(report.linked_sub_reports[0]!)).toMatchObject({
      reviewed: true,
      currentCollectionSuggestions: 0,
      currentMissingEvidence: 0,
      status: "complete",
    });
    expect(diagnosisReadiness(report)).toMatchObject({
      attention: 0,
      blocked: false,
      canConfirm: false,
      currentCollectionSuggestions: 0,
      currentMissingEvidence: 0,
      done: 1,
      humanReviewRequired: 0,
      pending: 0,
      ready: 0,
      reviewed: 1,
      status: "complete",
    });
    expect(reportDiagnosisHandoff(report).steps).toContainEqual(
      expect.objectContaining({
        key: "operator_decision",
        status: "done",
      }),
    );
    expect(reportEvidenceFollowUps(report)).toEqual([]);
  });

  it("uses server-provided final report notification readiness", () => {
    const readiness = finalNotificationReadiness({
      detail: "All linked subreports have operator-confirmed AI conclusions.",
      notification_purpose: "final",
      ready: true,
      status: "ready",
      status_label: "Final notification ready",
    });

    expect(reportFinalNotificationReadiness(reportDetail([], readiness))).toEqual({
      notification_purpose: "final",
      ready: true,
      reason: { kind: "ready" },
      source: "api",
      status: "ready",
    });
  });

  it("derives final notification detail reasons without retaining backend prose", () => {
    expect(reportFinalNotificationReadiness(reportDetail([])).reason).toEqual({
      kind: "no_linked_subreports",
      reportID: 101,
    });
    expect(
      reportFinalNotificationReadiness(
        reportDetail([linkedSubReport({ evidence_snapshot_id: 0, title: "" })]),
      ).reason,
    ).toEqual({
      kind: "missing_evidence_snapshot",
      subReportID: 501,
      subReportTitle: "",
    });
    expect(
      reportFinalNotificationReadiness(
        reportDetail([
          linkedSubReport({
            diagnosis_conclusion: diagnosisConclusion(),
            title: "Vendor checkout latency",
          }),
        ]),
      ).reason,
    ).toEqual({
      kind: "unconfirmed_conclusion",
      subReportID: 501,
      subReportTitle: "Vendor checkout latency",
    });
    expect(
      reportFinalNotificationReadiness(
        reportDetail([
          linkedSubReport({
            diagnosis_conclusion: diagnosisConclusion({ confirmed_by: "operator:alice" }),
            diagnosis_progress: diagnosisProgress({
              recorded_at: "2026-06-18T08:07:00Z",
            }),
          }),
        ]),
      ).reason,
    ).toEqual({
      kind: "newer_diagnosis_progress",
      subReportID: 501,
      subReportTitle: "Checkout API latency",
    });
    expect(
      reportFinalNotificationReadiness(
        reportDetail([
          linkedSubReport({
            diagnosis_conclusion: diagnosisConclusion({ confirmed_by: "operator:alice" }),
          }),
        ]),
      ).reason,
    ).toEqual({ kind: "blocked" });
  });

  it("falls back safely when final report notification readiness is absent", () => {
    const report = {
      ...reportDetail([]),
      final_notification_readiness: undefined,
    } as unknown as FinalReportDetail;

    expect(reportFinalNotificationReadiness(report)).toEqual({
      notification_purpose: "final",
      ready: false,
      reason: { kind: "fallback" },
      source: "fallback",
      status: "blocked",
    });
  });

  it("keeps unconfirmed stored conclusions ready for operator confirmation", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence: "high",
          requires_human_review: false,
        }),
      }),
    ]);

    expect(subReportDiagnosisReadiness(report.linked_sub_reports[0]!)).toMatchObject({
      currentCollectionSuggestions: 0,
      currentMissingEvidence: 0,
      latestConfidence: "high",
      reviewed: true,
      status: "human_review",
    });
    expect(diagnosisReadiness(report)).toMatchObject({
      attention: 0,
      blocked: false,
      canConfirm: true,
      done: 0,
      humanReviewRequired: 0,
      pending: 0,
      ready: 1,
      reviewed: 1,
      status: "human_review",
      total: 1,
    });
    expect(reportDiagnosisHandoff(report).steps).toContainEqual(
      expect.objectContaining({
        key: "operator_decision",
        status: "pending",
      }),
    );
  });

  it("surfaces current evidence gaps from the latest AI turn", () => {
    const subReport = linkedSubReport({
      diagnosis_conclusion: diagnosisConclusion({
        confidence: "low",
        requires_human_review: true,
        confidence_timeline: [
          timelineEntry({
            confidence: "low",
            conclusion_status: "needs_evidence",
            evidence_collection_suggestions: [
              {
                detail: "Collect a bounded p95 query.",
                label: "Latency trend",
                priority: "medium",
              },
            ],
            missing_evidence_requests: [
              {
                detail: "Attach the latest deployment window.",
                label: "Deployment window",
                priority: "high",
              },
            ],
            occurred_at: "2026-06-18T08:10:00Z",
          }),
        ],
      }),
    });

    expect(subReportDiagnosisReadiness(subReport)).toMatchObject({
      currentCollectionSuggestions: 1,
      currentExecutableEvidenceRequests: 0,
      currentMissingEvidence: 1,
      status: "needs_evidence",
    });
    expect(diagnosisReadiness(reportDetail([subReport]))).toMatchObject({
      blocked: true,
      currentCollectionSuggestions: 1,
      currentExecutableEvidenceRequests: 0,
      currentMissingEvidence: 1,
      pending: 2,
      ready: 0,
      status: "needs_evidence",
    });
    expect(reportEvidenceFollowUps(reportDetail([subReport]))).toEqual([
      {
        detail: "Attach the latest deployment window.",
        evidenceSnapshotID: 9001,
        kind: "missing_evidence",
        label: "Deployment window",
        priority: "high",
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
      {
        detail: "Collect a bounded p95 query.",
        evidenceSnapshotID: 9001,
        kind: "collection_suggestion",
        label: "Latency trend",
        priority: "medium",
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
    ]);
  });

  it("uses final conclusion evidence requests when the conclusion stores unresolved evidence", () => {
    const subReport = linkedSubReport({
      diagnosis_conclusion: diagnosisConclusion({
        confidence: "medium",
        evidence_requests: [
          {
            limit: 5,
            reason: "Confirm whether sibling checkout alerts remain active.",
            tool: "active_alerts",
          },
        ],
        evidence_collection_suggestions: [
          {
            detail: "Collect the bounded post-rollback latency trend.",
            label: "Post-rollback latency",
            priority: "medium",
          },
        ],
        missing_evidence_requests: [
          {
            detail: "Attach service-owner confirmation before closing.",
            label: "Owner confirmation",
            priority: "high",
          },
        ],
        requires_human_review: true,
      }),
    });

    expect(subReportDiagnosisReadiness(subReport)).toMatchObject({
      currentCollectionSuggestions: 1,
      currentExecutableEvidenceRequests: 1,
      currentMissingEvidence: 1,
      evidenceRequests: 1,
      status: "needs_evidence",
    });
    expect(reportEvidenceFollowUps(reportDetail([subReport]))).toEqual([
      {
        detail: "",
        evidenceSnapshotID: 9001,
        kind: "evidence_request",
        label: "Confirm whether sibling checkout alerts remain active.",
        priority: "action",
        request: {
          limit: 5,
          reason: "Confirm whether sibling checkout alerts remain active.",
          tool: "active_alerts",
        },
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
      {
        detail: "Attach service-owner confirmation before closing.",
        evidenceSnapshotID: 9001,
        kind: "missing_evidence",
        label: "Owner confirmation",
        priority: "high",
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
      {
        detail: "Collect the bounded post-rollback latency trend.",
        evidenceSnapshotID: 9001,
        kind: "collection_suggestion",
        label: "Post-rollback latency",
        priority: "medium",
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
    ]);
  });

  it("omits executable evidence follow-ups after matching collection succeeds", () => {
    const subReport = linkedSubReport({
      diagnosis_conclusion: diagnosisConclusion({
        confidence: "medium",
        evidence_requests: [
          {
            alert_source_profile_id: 7,
            limit: 5,
            query: "sum(rate(checkout_requests_total[5m]))",
            reason: "Confirm whether checkout requests remain elevated.",
            tool: "metric_range_query",
            window_seconds: 1800,
          },
          {
            alert_source_profile_id: 7,
            limit: 5,
            query: "sum(rate(checkout_errors_total[5m]))",
            reason: "Confirm whether checkout errors remain elevated.",
            tool: "metric_range_query",
            window_seconds: 1800,
          },
        ],
        confidence_timeline: [
          timelineEntry({
            evidence_collection_results: [
              {
                alert_source_profile_id: 7,
                collected_at: "2026-06-18T08:06:00Z",
                limit: 5,
                query: "sum(rate(checkout_requests_total[5m]))",
                request_reason: "Confirm whether checkout requests remain elevated.",
                status: "collected",
                tool: "metric_range_query",
                window_seconds: 1800,
              },
              {
                alert_source_profile_id: 7,
                collected_at: "2026-06-18T08:06:10Z",
                limit: 5,
                query: "sum(rate(checkout_errors_total[5m]))",
                request_reason: "Confirm whether checkout errors remain elevated.",
                status: "failed",
                tool: "metric_range_query",
                window_seconds: 1800,
              },
            ],
          }),
        ],
        requires_human_review: true,
      }),
    });

    expect(reportEvidenceFollowUps(reportDetail([subReport]))).toEqual([
      {
        detail: "",
        evidenceSnapshotID: 9001,
        kind: "evidence_request",
        label: "Confirm whether checkout errors remain elevated.",
        priority: "action",
        request: {
          alert_source_profile_id: 7,
          limit: 5,
          query: "sum(rate(checkout_errors_total[5m]))",
          reason: "Confirm whether checkout errors remain elevated.",
          tool: "metric_range_query",
          window_seconds: 1800,
        },
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
    ]);
  });

  it("builds the report-level diagnosis handoff plan", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence: "high",
          requires_human_review: true,
        }),
      }),
      linkedSubReport({
        diagnosis_conclusion: undefined,
        diagnosis_progress: diagnosisProgress({
          confidence: "medium",
          conclusion_status: "needs_evidence",
          evidence_collection_suggestions: [
            {
              detail: "Collect a bounded JVM memory range query.",
              label: "JVM memory trend",
              priority: "medium",
            },
          ],
          missing_evidence_requests: [
            {
              detail: "Attach the latest deployment and restart context.",
              label: "Runtime context",
              priority: "high",
            },
          ],
          requires_human_review: true,
        }),
        evidence_snapshot_id: 9003,
        id: 502,
        title: "Checkout JVM memory",
      }),
    ]);

    expect(reportDiagnosisHandoff(report)).toMatchObject({
      evidenceSnapshotCount: 2,
      followUpCount: 2,
      reportID: 101,
      reportWorkflow: "FinalReportWorkflow",
      status: "needs_evidence",
      steps: [
        {
          key: "report_generation",
          status: "done",
        },
        {
          key: "evidence_snapshot",
          status: "done",
        },
        {
          key: "ai_consultation",
          status: "done",
        },
        {
          key: "evidence_follow_up",
          status: "pending",
        },
        {
          key: "operator_decision",
          status: "attention",
        },
      ],
    });
  });

  it("selects pending diagnosis before confirmable conclusions for the next report action", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence: "high",
          requires_human_review: false,
        }),
      }),
      linkedSubReport({
        id: 502,
        evidence_snapshot_id: 9002,
        title: "Database capacity",
      }),
    ]);

    expect(reportDiagnosisNextAction(report)).toEqual({
      evidenceSnapshotID: 9002,
      hasConclusion: false,
      hasProgress: false,
      readiness: expect.objectContaining({ status: "pending_diagnosis" }),
      subReportID: 502,
      title: "Database capacity",
    });
  });

  it("selects evidence blockers before confirmable conclusions for the next report action", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence: "high",
          requires_human_review: false,
        }),
      }),
      linkedSubReport({
        diagnosis_conclusion: undefined,
        diagnosis_progress: diagnosisProgress({
          confidence: "medium",
          conclusion_status: "needs_evidence",
          missing_evidence_requests: [
            {
              detail: "Attach the current database capacity ticket.",
              label: "Capacity ticket",
              priority: "high",
            },
          ],
          requires_human_review: true,
        }),
        evidence_snapshot_id: 9002,
        id: 502,
        title: "Database capacity",
      }),
    ]);

    expect(reportDiagnosisNextAction(report)).toEqual({
      evidenceSnapshotID: 9002,
      hasConclusion: false,
      hasProgress: true,
      readiness: expect.objectContaining({ status: "needs_evidence" }),
      subReportID: 502,
      title: "Database capacity",
    });
  });

  it("omits the next report action after all linked conclusions are complete", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confirmed_by: "owner-1",
          conclusion_version: "diagnosis-session-301:2",
          requires_human_review: true,
        }),
      }),
    ]);

    expect(reportDiagnosisNextAction(report)).toBeNull();
  });

  it("keeps pending subreports separate from reviewed conclusions", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence: "high",
          requires_human_review: false,
        }),
      }),
      linkedSubReport({
        id: 502,
        title: "Database capacity",
      }),
    ]);

    expect(diagnosisReadiness(report)).toMatchObject({
      blocked: true,
      canConfirm: false,
      done: 0,
      pending: 1,
      pendingSubReports: 1,
      ready: 1,
      reviewed: 1,
      status: "pending_diagnosis",
      total: 2,
    });
  });

  it("uses diagnosis progress before a final conclusion is recorded", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: undefined,
        diagnosis_progress: diagnosisProgress({
          confidence: "medium",
          conclusion_status: "needs_evidence",
          evidence_collection_suggestions: [
            {
              detail: "Collect a bounded JVM memory range query.",
              label: "JVM memory trend",
              priority: "medium",
            },
          ],
          evidence_request_count: 1,
          evidence_requests: [
            {
              query: "sum(container_memory_working_set_bytes{namespace=\"checkout\"})",
              reason: "Need JVM memory pressure evidence.",
              tool: "metric_range_query",
            },
          ],
          missing_evidence_requests: [
            {
              detail: "Attach the latest deployment and restart context.",
              label: "Runtime context",
              priority: "high",
            },
          ],
          requires_human_review: true,
        }),
      }),
    ]);

    expect(diagnosisReadiness(report)).toMatchObject({
      blocked: true,
      currentCollectionSuggestions: 1,
      currentExecutableEvidenceRequests: 1,
      currentMissingEvidence: 1,
      evidenceRequests: 1,
      humanReviewRequired: 1,
      latestConfidence: "medium",
      pending: 3,
      pendingSubReports: 0,
      ready: 0,
      reviewed: 1,
      status: "needs_evidence",
      total: 1,
    });
    expect(reportEvidenceFollowUps(report)).toEqual([
      {
        detail: "",
        evidenceSnapshotID: 9001,
        kind: "evidence_request",
        label: "Need JVM memory pressure evidence.",
        priority: "action",
        request: {
          query: "sum(container_memory_working_set_bytes{namespace=\"checkout\"})",
          reason: "Need JVM memory pressure evidence.",
          tool: "metric_range_query",
        },
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
      {
        detail: "Attach the latest deployment and restart context.",
        evidenceSnapshotID: 9001,
        kind: "missing_evidence",
        label: "Runtime context",
        priority: "high",
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
      {
        detail: "Collect a bounded JVM memory range query.",
        evidenceSnapshotID: 9001,
        kind: "collection_suggestion",
        label: "JVM memory trend",
        priority: "medium",
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
    ]);
  });

  it("uses newer diagnosis progress over a stale stored conclusion", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence: "high",
          recorded_at: "2026-06-18T08:06:10Z",
          requires_human_review: false,
        }),
        diagnosis_progress: diagnosisProgress({
          confidence: "medium",
          confidence_rationale:
            "Operator follow-up introduced a new evidence gap.",
          conclusion_status: "needs_evidence",
          missing_evidence_requests: [
            {
              detail: "Attach the latest database expansion ticket.",
              label: "Capacity change ticket",
              priority: "high",
            },
          ],
          occurred_at: "2026-06-18T08:08:00Z",
          recorded_at: "2026-06-18T08:08:01Z",
          requires_human_review: true,
        }),
      }),
    ]);

    expect(subReportDiagnosisReadiness(report.linked_sub_reports[0]!)).toMatchObject({
      currentMissingEvidence: 1,
      latestConfidence: "medium",
      status: "needs_evidence",
    });
    expect(diagnosisReadiness(report)).toMatchObject({
      blocked: true,
      canConfirm: false,
      currentMissingEvidence: 1,
      done: 0,
      latestConfidence: "medium",
      pending: 1,
      ready: 0,
      status: "needs_evidence",
    });
    expect(reportEvidenceFollowUps(report)).toEqual([
      expect.objectContaining({
        detail: "Attach the latest database expansion ticket.",
        kind: "missing_evidence",
        label: "Capacity change ticket",
      }),
    ]);
  });

  it("keeps collection-only suggestions residual without blocking confirmation", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: undefined,
        diagnosis_progress: diagnosisProgress({
          confidence: "medium",
          conclusion_status: "needs_evidence",
          evidence_collection_suggestions: [
            {
              detail: "Collect current active sibling alerts.",
              label: "Current alerts",
              priority: "medium",
            },
          ],
          requires_human_review: true,
        }),
      }),
    ]);

    expect(diagnosisReadiness(report)).toMatchObject({
      blocked: false,
      canConfirm: true,
      currentCollectionSuggestions: 1,
      currentExecutableEvidenceRequests: 0,
      currentMissingEvidence: 0,
      pending: 1,
      ready: 1,
      status: "human_review",
    });
    expect(reportDiagnosisHandoff(report).steps).toContainEqual(
      expect.objectContaining({
        key: "evidence_follow_up",
        status: "done",
      }),
    );
  });

  it("marks failed diagnosis progress before a final conclusion as failed", () => {
    const subReport = linkedSubReport({
      diagnosis_conclusion: undefined,
      diagnosis_progress: diagnosisProgress({
        failure_reason: "initial turn failed: llm request timed out",
        requires_human_review: true,
        status: "failed",
      }),
    });

    expect(subReportDiagnosisReadiness(subReport)).toMatchObject({
      failureReason: "initial turn failed: llm request timed out",
      latestConfidence: "failed",
      status: "failed",
    });
    expect(diagnosisReadiness(reportDetail([subReport]))).toMatchObject({
      attention: 1,
      blocked: true,
      canConfirm: false,
      failedSubReports: 1,
      latestConfidence: "failed",
      pending: 0,
      pendingSubReports: 0,
      ready: 0,
      reviewed: 1,
      status: "failed",
      total: 1,
    });
  });

  it("keeps evidence request totals numeric for legacy timeline entries", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: diagnosisConclusion({
          confidence_timeline: [
            legacyTimelineEntry({
              confidence: "medium",
              occurred_at: "2026-06-18T08:03:00Z",
            }),
          ],
        }),
      }),
    ]);

    expect(
      subReportDiagnosisReadiness(report.linked_sub_reports[0]!)
        .evidenceRequests,
    ).toBe(0);
    expect(diagnosisReadiness(report).evidenceRequests).toBe(0);
  });
});

function reportDetail(
  linkedSubReports: LinkedSubReport[],
  finalReadiness = finalNotificationReadiness(),
): FinalReportDetail {
  return {
    confidence: "high",
    content: {
      title: "Checkout latency incident",
    },
    correlation_key: "window:checkout-latency",
    created_at: "2026-06-18T08:00:00Z",
    created_by_workflow: "FinalReportWorkflow",
    executive_summary: "Checkout latency increased after deployment.",
    id: 101,
    final_notification_readiness: finalReadiness,
    linked_sub_reports: linkedSubReports,
    model: "example-llm-model",
    notification_deliveries: [],
    notification_text: "Checkout latency incident requires review.",
    output_mode: "json_schema",
    recommended_actions: [
      {
        detail: "Compare deployment timestamps with latency onset.",
        label: "Inspect deployment",
        priority: "high",
      },
    ],
    severity: "warning",
    sub_reports: linkedSubReports.map((subReport) => ({
      severity: subReport.severity,
      summary: subReport.summary,
      title: subReport.title,
    })),
    title: "Checkout latency incident",
  };
}

function finalNotificationReadiness(
  overrides: Partial<FinalReportDetail["final_notification_readiness"]> = {},
): FinalReportDetail["final_notification_readiness"] {
  return {
    detail: "Checkout API latency has no operator-confirmed AI conclusion yet.",
    notification_purpose: "handoff",
    ready: false,
    status: "blocked",
    status_label: "Final notification blocked",
    ...overrides,
  };
}

function linkedSubReport(
  overrides: Partial<LinkedSubReport> = {},
): LinkedSubReport {
  return {
    confidence: "high",
    content: {
      title: "Checkout API latency",
    },
    created_at: "2026-06-18T07:59:00Z",
    created_by_workflow: "ReportFanOutWorkflow",
    evidence_refs: ["alert:checkout-latency"],
    evidence_snapshot_id: 9001,
    findings: [
      {
        detail: "p95 latency stayed above the warning threshold.",
        evidence_id: "alert:checkout-latency",
        label: "High p95 latency",
      },
    ],
    id: 501,
    model: "example-llm-model",
    output_mode: "json_schema",
    recommended_actions: [
      {
        detail: "Compare deployment timestamps with latency onset.",
        label: "Inspect deployment",
        priority: "high",
      },
    ],
    scenario: "single_alert",
    severity: "warning",
    summary: "p95 latency exceeded the warning threshold.",
    title: "Checkout API latency",
    ...overrides,
  };
}

function diagnosisConclusion(
  overrides: Partial<ReportDiagnosisConclusion> = {},
): ReportDiagnosisConclusion {
  return {
    chat_session_id: 401,
    confidence: "high",
    content: "Checkout latency remains correlated with the payment deployment.",
    diagnosis_task_id: 301,
    event_kind: "diagnosis_room.final_conclusion_ready",
    recorded_at: "2026-06-18T08:06:10Z",
    requires_human_review: false,
    session_id: "diagnosis-session-301",
    source: "latest_assistant_turn",
    status: "available",
    ...overrides,
  };
}

function diagnosisProgress(
  overrides: Partial<ReportDiagnosisProgress> = {},
): ReportDiagnosisProgress {
  return {
    confidence: "low",
    diagnosis_task_id: 301,
    event_kind: "diagnosis_room.turn_persisted",
    evidence_request_count: 0,
    evidence_snapshot_id: 9001,
    occurred_at: "2026-06-18T08:02:00Z",
    recorded_at: "2026-06-18T08:02:01Z",
    requires_human_review: true,
    status: "in_progress",
    ...overrides,
  };
}

function timelineEntry(overrides: Partial<TimelineEntry> = {}): TimelineEntry {
  return {
    confidence: "medium",
    event_kind: "diagnosis_room.turn_persisted",
    evidence_request_count: 0,
    occurred_at: "2026-06-18T08:00:00Z",
    requires_human_review: true,
    ...overrides,
  };
}

function legacyTimelineEntry(
  overrides: Partial<TimelineEntry> = {},
): TimelineEntry {
  return {
    confidence: "medium",
    event_kind: "diagnosis_room.turn_persisted",
    occurred_at: "2026-06-18T08:00:00Z",
    requires_human_review: true,
    ...overrides,
  } as TimelineEntry;
}
