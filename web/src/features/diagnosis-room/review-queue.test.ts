import { describe, expect, it } from "vitest";

import {
  diagnosisReviewQueueActionGate,
  diagnosisReviewQueueActionPlan,
  diagnosisReviewQueueBlockingReason,
  diagnosisReviewQueueConnectionGateAllowsPreparation,
  diagnosisReviewQueueItems,
  diagnosisReviewQueueNextAction,
  diagnosisReviewQueuePostEvidenceStatus,
  diagnosisReviewQueueReassessmentInput,
  diagnosisReviewQueueSummary,
  diagnosisReviewQueueTaskProgress,
  finalConclusionReviewQueueInput
} from "./review-queue";
import type {
  DiagnosisEvidenceCollectionResult,
  DiagnosisFinalConclusion,
  DiagnosisSupplementalEvidenceRecord
} from "./types";

describe("diagnosis review queue", () => {
  it("classifies review queue action gates", () => {
    expect(
      diagnosisReviewQueueActionGate({
        actionDisabledReason: "",
        connected: true
      })
    ).toEqual({
      disabled: false,
      kind: "ready",
      reason: ""
    });

    expect(
      diagnosisReviewQueueActionGate({
        actionDisabledReason:
          "Connect to a diagnosis room before sending evidence updates.",
        connected: false
      })
    ).toEqual({
      disabled: true,
      kind: "connection",
      reason: "Connect to a diagnosis room before sending evidence updates."
    });

    expect(
      diagnosisReviewQueueActionGate({
        actionDisabledReason:
          "Current user is not authorized to participate in this diagnosis room.",
        connected: true
      })
    ).toEqual({
      disabled: true,
      kind: "blocked",
      reason:
        "Current user is not authorized to participate in this diagnosis room."
    });

    expect(
      diagnosisReviewQueueActionGate({
        actionDisabledReason: "",
        connected: false
      })
    ).toEqual({
      disabled: true,
      kind: "connection",
      reason:
        "Open a live diagnosis-room connection before running review queue actions."
    });
  });

  it("maps saved review queue evidence into reassessment input", () => {
    const collectionResult = evidenceResult({
      request: {
        query: "up",
        reason: "Check service availability.",
        tool: "metric_query"
      },
      status: "collected",
      tool: "metric_query"
    });
    const supplementalRecord = supplementalEvidenceRecord({
      assistant_sequence: 3,
      label: "Owner remediation",
      priority: "high"
    });

    expect(
      diagnosisReviewQueueReassessmentInput({
        canConfirmConclusion: false,
        collectionResults: [collectionResult],
        conclusionStatus: "needs_evidence",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        latestAssistantSequence: 3,
        missingEvidenceRequests: [],
        requiresHumanReview: true,
        supplementalEvidence: [supplementalRecord]
      })
    ).toEqual({
      collectionResults: [collectionResult],
      latestAssistantSequence: 3,
      records: [supplementalRecord]
    });
  });

  it("allows follow-up preparation only for connection gates", () => {
    expect(
      diagnosisReviewQueueConnectionGateAllowsPreparation({
        actionDisabledReason:
          "Connect to a diagnosis room before sending evidence updates.",
        connected: false
      })
    ).toBe(true);

    expect(
      diagnosisReviewQueueConnectionGateAllowsPreparation({
        actionDisabledReason:
          "Open a live diagnosis-room connection before running review queue actions.",
        connected: false
      })
    ).toBe(true);

    expect(
      diagnosisReviewQueueConnectionGateAllowsPreparation({
        actionDisabledReason:
          "Current user is not authorized to participate in this diagnosis room.",
        connected: false
      })
    ).toBe(false);

    expect(
      diagnosisReviewQueueConnectionGateAllowsPreparation({
        actionDisabledReason: "",
        connected: true
      })
    ).toBe(false);
  });

  it("prioritizes failed collection and missing evidence before confirmation", () => {
    const items = diagnosisReviewQueueItems({
      canConfirmConclusion: true,
      collectionResults: [
        evidenceResult({ status: "collected", tool: "active_alerts" }),
        evidenceResult({ status: "failed", tool: "metric_query", message: "PromQL request failed." })
      ],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      missingEvidenceRequests: [
        {
          detail: "Attach the latest database capacity action.",
          label: "DB capacity action",
          priority: "high"
        }
      ],
      requiresHumanReview: true
    });

    expect(items.map((item) => item.kind)).toEqual([
      "collection_result",
      "supplemental_evidence",
      "collection_result"
    ]);
    expect(items[0]?.status).toBe("attention");
    expect(items[1]?.status).toBe("attention");
    expect(items[2]?.status).toBe("done");
  });

  it("adds confirmation only after blocking evidence work is resolved", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [
        evidenceResult({
          request: { reason: "Read tablespace utilization", tool: "metric_query" },
          status: "collected",
          tool: "metric_query"
        })
      ],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [
        { reason: "Read tablespace utilization", tool: "metric_query" }
      ],
      missingEvidenceRequests: [],
      requiresHumanReview: true
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);

    expect(items.map((item) => item.kind)).toEqual([
      "confirm",
      "collection_result"
    ]);
    expect(items[0]).toMatchObject({
      kind: "confirm",
      status: "ready",
      title: "Confirm conclusion"
    });
    expect(summary).toMatchObject({
      attention: 0,
      blockingReason: "",
      canConfirm: true,
      done: 1,
      message: "AI conclusion is ready for operator confirmation.",
      pending: 0,
      ready: 1,
      total: 2
    });
    expect(diagnosisReviewQueueActionPlan(items, summary)).toMatchObject({
      actions: [
        {
          status: "ready",
          title: "Confirm conclusion"
        }
      ],
      message: "Review the ready confirmation item, then retain the conclusion.",
      remaining: 0,
      status: "ready"
    });
  });

  it("blocks confirmation when planned evidence is skipped", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [
        evidenceResult({
          reason_code: "template_query_mismatch",
          request: {
            query: `up{job="api"}`,
            reason: "Read service availability",
            tool: "metric_query"
          },
          status: "skipped",
          tool: "metric_query"
        })
      ],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [
        {
          query: `up{job="api"}`,
          reason: "Read service availability",
          tool: "metric_query"
        }
      ],
      missingEvidenceRequests: [],
      requiresHumanReview: true
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);

    expect(items.map((item) => item.kind)).toEqual(["collection_result"]);
    expect(items[0]).toMatchObject({
      kind: "collection_result",
      recoveryRequest: {
        detail:
          "Reason: template_query_mismatch. Detail: Evidence collected. Original request: Read service availability. Provide verified alternative evidence or explain why this evidence cannot be collected as requested.",
        label: "metric_query evidence recovery",
        priority: "high",
        source_request: {
          query: `up{job="api"}`,
          reason: "Read service availability",
          tool: "metric_query"
        }
      },
      retryable: false,
      status: "attention",
      tag: "skipped",
      title: "metric_query evidence"
    });
    expect(summary).toMatchObject({
      blockingReason: "Resolve metric_query evidence collection before confirming.",
      canConfirm: false,
      pending: 0,
      ready: 0,
      total: 1
    });
    expect(diagnosisReviewQueueActionPlan(items, summary)).toMatchObject({
      actions: [
        {
          status: "attention",
          title: "metric_query evidence"
        }
      ],
      status: "blocked"
    });
  });

  it("derives the next action from unresolved review queue work", () => {
    expect(
      diagnosisReviewQueueNextAction({
        canConfirmConclusion: true,
        collectionResults: [
          evidenceResult({
            status: "failed",
            tool: "metric_query"
          })
        ],
        conclusionStatus: "ready_for_review",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        missingEvidenceRequests: [],
        requiresHumanReview: true
      })
    ).toBe("Resolve evidence collection");

    expect(
      diagnosisReviewQueueNextAction({
        canConfirmConclusion: true,
        collectionResults: [
          evidenceResult({
            request: {
              query: `up{job="worker"}`,
              reason: "Read worker availability",
              tool: "metric_query"
            },
            status: "collected",
            tool: "metric_query"
          })
        ],
        conclusionStatus: "ready_for_review",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [
          {
            query: `up{job="api"}`,
            reason: "Read api availability",
            tool: "metric_query"
          }
        ],
        missingEvidenceRequests: [],
        requiresHumanReview: true
      })
    ).toBe("Run evidence collection");
  });

  it("allows reviewed supplemental evidence to move the next action to confirmation", () => {
    expect(
      diagnosisReviewQueueNextAction({
        canConfirmConclusion: true,
        collectionResults: [],
        conclusionStatus: "ready_for_review",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        latestAssistantSequence: 4,
        missingEvidenceRequests: [
          {
            detail: "Attach the latest database capacity action.",
            label: "DB capacity action",
            priority: "high"
          }
        ],
        requiresHumanReview: true,
        supplementalEvidence: [
          supplementalEvidenceRecord({
            assistant_sequence: 4,
            detail: "Attach the latest database capacity action.",
            label: "DB capacity action"
          })
        ]
      })
    ).toBe("Ready for confirmation");
  });

  it("keeps collection suggestions visible before final confirmation", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [
        {
          detail: "Collect a bounded CPU range before retaining the conclusion.",
          label: "CPU range sample",
          priority: "medium"
        }
      ],
      evidenceRequests: [],
      missingEvidenceRequests: [],
      requiresHumanReview: true
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);
    const postEvidenceStatus = diagnosisReviewQueuePostEvidenceStatus(input);

    expect(diagnosisReviewQueueNextAction(input)).toBe(
      "Review collection suggestions"
    );
    expect(items.map((item) => item.kind)).toEqual([
      "supplemental_evidence",
      "confirm"
    ]);
    expect(summary).toMatchObject({
      blockingReason: "",
      canConfirm: true,
      pending: 1,
      ready: 1
    });
    expect(diagnosisReviewQueueActionPlan(items, summary)).toMatchObject({
      message:
        "Review the listed evidence suggestions before retaining the conclusion; confirm only after accepting any residual uncertainty.",
      status: "pending"
    });
    expect(
      diagnosisReviewQueueTaskProgress(items, summary, postEvidenceStatus)
    ).toMatchObject({
      status: "pending",
      statusLabel: "Evidence tasks pending",
      summary:
        "Supply operator evidence: 1 operator evidence task still needs submission or review."
    });
  });

  it("marks transient collection failures as retryable", () => {
    const items = diagnosisReviewQueueItems({
      canConfirmConclusion: false,
      collectionResults: [
        evidenceResult({
          reason_code: "provider_unavailable",
          status: "skipped",
          tool: "active_alerts"
        }),
        evidenceResult({
          reason_code: "provider_failed",
          status: "failed",
          tool: "metric_query"
        }),
        evidenceResult({
          reason_code: "source_kind_mismatch",
          status: "unsupported",
          tool: "metric_range_query"
        })
      ],
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      missingEvidenceRequests: [],
      requiresHumanReview: true
    });

    expect(items).toHaveLength(3);
    expect(items[0]).toMatchObject({
      kind: "collection_result",
      recoveryRequest: undefined,
      retryable: true,
      status: "attention",
      tag: "skipped"
    });
    expect(items[1]).toMatchObject({
      kind: "collection_result",
      recoveryRequest: undefined,
      retryable: true,
      status: "attention",
      tag: "failed"
    });
    expect(items[2]).toMatchObject({
      kind: "collection_result",
      recoveryRequest: {
        label: "metric_range_query evidence recovery",
        priority: "high"
      },
      retryable: false,
      status: "attention",
      tag: "unsupported"
    });
  });

  it("matches planned evidence against top-level collection result identifiers", () => {
    const items = diagnosisReviewQueueItems({
      canConfirmConclusion: true,
      collectionResults: [
        evidenceResult({
          alert_source_profile_id: 7,
          request: {
            limit: 5,
            reason: "Read active alerts",
            tool: "active_alerts"
          },
          template_id: 42,
          tool: "active_alerts"
        })
      ],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [
        {
          alert_source_profile_id: 7,
          limit: 5,
          reason: "Read active alerts",
          template_id: 42,
          tool: "active_alerts"
        }
      ],
      missingEvidenceRequests: [],
      requiresHumanReview: true
    });

    expect(items.map((item) => item.kind)).toEqual([
      "confirm",
      "collection_result"
    ]);
  });

  it("adds a pending executable collection item when planned evidence is not collected", () => {
    const input = {
      canConfirmConclusion: false,
      collectionResults: [
        evidenceResult({
          request: { reason: "Read active alerts", tool: "active_alerts" },
          status: "collected",
          tool: "active_alerts"
        })
      ],
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [
        { reason: "Read active alerts", tool: "active_alerts" },
        { reason: "Read tablespace utilization", tool: "metric_query" }
      ],
      missingEvidenceRequests: [],
      requiresHumanReview: true
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);

    expect(items.map((item) => item.kind)).toEqual(["executable_evidence", "collection_result"]);
    expect(summary).toMatchObject({
      blockingReason: "Collect planned executable evidence before confirming.",
      canConfirm: false,
      done: 1,
      pending: 1,
      total: 2
    });
    expect(items[0]).toMatchObject({
      detail: "Reason: Read tablespace utilization.",
      kind: "executable_evidence",
      status: "pending",
      tag: "metric_query",
      title: "Collect evidence: metric_query"
    });
    if (items[0]?.kind !== "executable_evidence") {
      throw new Error("expected executable evidence queue item");
    }
    expect(items[0].request).toEqual({
      reason: "Read tablespace utilization",
      tool: "metric_query"
    });
  });

  it("deduplicates repeated pending executable evidence plans", () => {
    const input = {
      canConfirmConclusion: false,
      collectionResults: [],
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [
        {
          query: `up{job="api"}`,
          reason: "Read service availability.",
          tool: "metric_query"
        },
        {
          query: `up{job="api"}`,
          reason: "Read service availability.",
          tool: "metric_query"
        },
        {
          query: `up{job="worker"}`,
          reason: "Read worker availability.",
          tool: "metric_query"
        }
      ],
      missingEvidenceRequests: [],
      requiresHumanReview: true
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);

    expect(items).toHaveLength(2);
    expect(items.map((item) => item.kind)).toEqual(["executable_evidence", "executable_evidence"]);
    expect(items.map((item) => item.key)).toEqual([
      `executable:no-template:no-profile:metric_query:Read service availability.:up{job="api"}:no-window:no-step:no-limit:0`,
      `executable:no-template:no-profile:metric_query:Read worker availability.:up{job="worker"}:no-window:no-step:no-limit:1`
    ]);
    expect(summary).toMatchObject({
      blockingReason: "Collect planned executable evidence before confirming.",
      pending: 2,
      total: 2
    });
  });

  it("builds a bounded action plan from the highest-priority queue items", () => {
    const input = {
      canConfirmConclusion: false,
      collectionResults: [
        evidenceResult({
          request: { reason: "Read tablespace utilization", tool: "metric_query" },
          status: "failed",
          tool: "metric_query",
          message: "PromQL request failed."
        })
      ],
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [
        {
          detail: "Collect recent active alerts from the source.",
          label: "Recent active alerts",
          priority: "medium"
        }
      ],
      evidenceRequests: [
        { reason: "Read active alerts", tool: "active_alerts" },
        { reason: "Read tablespace utilization", tool: "metric_query" }
      ],
      missingEvidenceRequests: [
        {
          detail: "Attach the latest database capacity action.",
          label: "DB capacity action",
          priority: "high"
        }
      ],
      requiresHumanReview: true
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);

    expect(diagnosisReviewQueueActionPlan(items, summary)).toMatchObject({
      actions: [
        {
          status: "attention",
          title: "metric_query evidence"
        },
        {
          status: "attention",
          title: "Missing evidence: DB capacity action"
        },
        {
          status: "pending",
          title: "Collection suggestion: Recent active alerts"
        }
      ],
      message: "Resolve metric_query evidence collection before confirming.",
      remaining: 1,
      status: "blocked"
    });
  });

  it("builds a phased task progress view for blocked evidence work", () => {
    const input = {
      canConfirmConclusion: false,
      collectionResults: [
        evidenceResult({
          request: { reason: "Read tablespace utilization", tool: "metric_query" },
          status: "failed",
          tool: "metric_query",
          message: "PromQL request failed."
        })
      ],
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [
        {
          detail: "Collect recent active alerts from the source.",
          label: "Recent active alerts",
          priority: "medium"
        }
      ],
      evidenceRequests: [
        { reason: "Read active alerts", tool: "active_alerts" },
        { reason: "Read tablespace utilization", tool: "metric_query" }
      ],
      missingEvidenceRequests: [
        {
          detail: "Attach the latest database capacity action.",
          label: "DB capacity action",
          priority: "high"
        }
      ],
      requiresHumanReview: true
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);
    const postEvidenceStatus = diagnosisReviewQueuePostEvidenceStatus(input);

    expect(
      diagnosisReviewQueueTaskProgress(items, summary, postEvidenceStatus)
    ).toEqual({
      completed: 0,
      percent: 0,
      phases: [
        {
          action: {
            itemKey:
              "collection:metric_query:failed:ok:2026-06-18T00:00:00Z:no-template:no-query",
            kind: "use_evidence_plan",
            label: "Retry collection"
          },
          detail:
            "1 evidence collection item needs operator recovery before confirmation.",
          key: "collect_evidence",
          label: "Collect evidence",
          status: "attention",
          statusLabel: "Attention"
        },
        {
          action: {
            itemKey: "attention-high-DB capacity action-0",
            kind: "use_follow_up",
            label: "Use follow-up"
          },
          detail: "1 operator evidence item still blocks confirmation.",
          key: "supply_operator_evidence",
          label: "Supply operator evidence",
          status: "attention",
          statusLabel: "Attention"
        },
        {
          detail:
            "AI reassessment waits for the open evidence tasks to be completed.",
          key: "ai_reassessment",
          label: "AI reassessment",
          status: "pending",
          statusLabel: "Pending"
        },
        {
          detail: "Resolve metric_query evidence collection before confirming.",
          key: "confirm_conclusion",
          label: "Confirm conclusion",
          status: "attention",
          statusLabel: "Attention"
        }
      ],
      status: "blocked",
      statusLabel: "Evidence tasks blocked",
      summary:
        "Collect evidence: 1 evidence collection item needs operator recovery before confirmation.",
      total: 4
    });
  });

  it("marks the phased task progress ready after latest evidence review clears blockers", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [
        evidenceResult({
          request: { reason: "Read tablespace utilization", tool: "metric_query" },
          status: "collected",
          tool: "metric_query"
        })
      ],
      confidence: "high",
      confidenceProgress: {
        label: "Confidence improved from medium to high",
        status: "improved" as const
      },
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [
        { reason: "Read tablespace utilization", tool: "metric_query" }
      ],
      latestAssistantSequence: 6,
      missingEvidenceRequests: [
        {
          detail: "Attach previous container logs.",
          label: "Restart cause",
          priority: "medium"
        }
      ],
      requiresHumanReview: true,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          assistant_sequence: 6,
          label: "Restart cause",
          provided_at: "2026-06-18T00:05:00Z"
        })
      ]
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);
    const postEvidenceStatus = diagnosisReviewQueuePostEvidenceStatus(input);

    expect(
      diagnosisReviewQueueTaskProgress(items, summary, postEvidenceStatus)
    ).toMatchObject({
      completed: 3,
      percent: 75,
      phases: [
        {
          key: "collect_evidence",
          status: "done"
        },
        {
          key: "supply_operator_evidence",
          status: "done"
        },
        {
          key: "ai_reassessment",
          status: "done"
        },
        {
          key: "confirm_conclusion",
          action: {
            itemKey: "confirm-conclusion",
            kind: "confirm",
            label: "Confirm"
          },
          status: "ready"
        }
      ],
      status: "ready",
      statusLabel: "Conclusion ready",
      summary: "Confirm conclusion: Operator can confirm and retain the AI conclusion.",
      total: 4
    });
  });

  it("asks AI to reassess retained executable evidence before continuing manually", () => {
    const input = {
      canConfirmConclusion: false,
      collectionResults: [
        evidenceResult({
          request: {
            query: `up{job="api"}`,
            reason: "Read service availability",
            tool: "metric_query"
          },
          status: "collected",
          tool: "metric_query"
        })
      ],
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [
        {
          query: `up{job="api"}`,
          reason: "Read service availability",
          tool: "metric_query"
        }
      ],
      missingEvidenceRequests: [],
      requiresHumanReview: true
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);
    const postEvidenceStatus = diagnosisReviewQueuePostEvidenceStatus(input);

    expect(diagnosisReviewQueueNextAction(input)).toBe("Ask AI to reassess");
    expect(
      diagnosisReviewQueueTaskProgress(items, summary, postEvidenceStatus)
    ).toMatchObject({
      phases: [
        {
          key: "collect_evidence",
          status: "done"
        },
        {
          key: "supply_operator_evidence",
          status: "done"
        },
        {
          action: {
            kind: "request_reassessment",
            label: "Ask AI to reassess"
          },
          detail:
            "Collected executable evidence is retained; ask AI to reassess confidence and conclusion status if the automatic evidence follow-up did not produce a ready conclusion.",
          key: "ai_reassessment",
          status: "pending"
        },
        {
          key: "confirm_conclusion",
          status: "pending"
        }
      ],
      status: "pending",
      summary:
        "AI reassessment: Collected executable evidence is retained; ask AI to reassess confidence and conclusion status if the automatic evidence follow-up did not produce a ready conclusion."
    });
  });

  it("returns a continue item when there is no immediate queue work", () => {
    const items = diagnosisReviewQueueItems({
      canConfirmConclusion: false,
      collectionResults: [],
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      missingEvidenceRequests: [],
      requiresHumanReview: false
    });

    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "continue",
      status: "pending",
      title: "Continue diagnosis"
    });
  });

  it("does not clear missing evidence with unrelated supplemental evidence", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      missingEvidenceRequests: [
        {
          detail: "Attach previous container logs.",
          label: "Restart cause",
          priority: "medium"
        }
      ],
      requiresHumanReview: true,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          detail: "The deployment owner confirmed no release was in progress.",
          label: "Deployment window",
          provided_at: "2026-06-18T00:05:00Z"
        })
      ]
    };

    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);

    expect(items.map((item) => item.kind)).toEqual([
      "supplemental_evidence",
      "supplemental_evidence_record"
    ]);
    expect(items.some((item) => item.kind === "confirm")).toBe(false);
    expect(summary).toMatchObject({
      blockingReason: "Resolve missing evidence requests before confirming.",
      canConfirm: false
    });
  });

  it("waits for AI review when matching supplemental evidence is stale", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      latestAssistantSequence: 6,
      missingEvidenceRequests: [
        {
          detail: "Attach previous container logs.",
          label: "Restart cause",
          priority: "medium"
        }
      ],
      requiresHumanReview: true,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          assistant_sequence: 4,
          label: "Restart cause",
          provided_at: "2026-06-18T00:05:00Z"
        })
      ]
    };

    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);

    expect(items.map((item) => item.kind)).toEqual([
      "supplemental_evidence_record"
    ]);
    expect(items[0]).toMatchObject({
      kind: "supplemental_evidence_record",
      status: "pending",
      title: "Submitted evidence awaiting AI review: Restart cause"
    });
    expect(summary).toMatchObject({
      blockingReason:
        "Wait for AI reassessment of submitted supplemental evidence before confirming.",
      canConfirm: false
    });
  });

  it("waits for AI review when the latest assistant sequence is unavailable", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      missingEvidenceRequests: [
        {
          detail: "Attach previous container logs.",
          label: "Restart cause",
          priority: "medium"
        }
      ],
      requiresHumanReview: true,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          assistant_sequence: 4,
          label: "Restart cause",
          provided_at: "2026-06-18T00:05:00Z"
        })
      ]
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);
    const postEvidenceStatus = diagnosisReviewQueuePostEvidenceStatus(input);

    expect(items.map((item) => item.kind)).toEqual([
      "supplemental_evidence_record"
    ]);
    expect(items[0]).toMatchObject({
      kind: "supplemental_evidence_record",
      status: "pending",
      title: "Submitted evidence awaiting AI review: Restart cause"
    });
    expect(summary).toMatchObject({
      blockingReason:
        "Wait for AI reassessment of submitted supplemental evidence before confirming.",
      canConfirm: false
    });
    expect(postEvidenceStatus).toMatchObject({
      reviewed: 0,
      status: "submitted",
      submitted: 1
    });
  });

  it("allows latest-reviewed supplemental evidence to close residual ready gaps", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      latestAssistantSequence: 6,
      missingEvidenceRequests: [
        {
          detail: "Attach previous container logs.",
          label: "Restart cause",
          priority: "medium"
        }
      ],
      requiresHumanReview: true,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          assistant_sequence: 6,
          label: "Restart cause",
          provided_at: "2026-06-18T00:05:00Z"
        })
      ]
    };

    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);

    expect(items.map((item) => item.kind)).toEqual([
      "confirm",
      "supplemental_evidence_record"
    ]);
    expect(items[1]).toMatchObject({
      kind: "supplemental_evidence_record",
      status: "done",
      title: "Submitted evidence: Restart cause"
    });
    expect(summary).toMatchObject({
      blockingReason: "",
      canConfirm: true
    });
  });

  it("does not clear a latest missing evidence request with same-label different-detail evidence", () => {
    const input = {
      canConfirmConclusion: true,
      collectionResults: [],
      conclusionStatus: "ready_for_review",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      latestAssistantSequence: 6,
      missingEvidenceRequests: [
        {
          detail: "Attach current previous container logs.",
          label: "Restart cause",
          priority: "medium"
        }
      ],
      requiresHumanReview: true,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          assistant_sequence: 6,
          detail: "Attach previous container logs.",
          label: "Restart cause",
          provided_at: "2026-06-18T00:05:00Z"
        })
      ]
    };

    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);
    const postEvidenceStatus = diagnosisReviewQueuePostEvidenceStatus(input);

    expect(items.map((item) => item.kind)).toEqual([
      "supplemental_evidence",
      "supplemental_evidence_record"
    ]);
    expect(items[0]).toMatchObject({
      kind: "supplemental_evidence",
      status: "attention",
      title: "Missing evidence: Restart cause"
    });
    expect(items[1]).toMatchObject({
      kind: "supplemental_evidence_record",
      status: "done",
      title: "Submitted evidence: Restart cause"
    });
    expect(summary).toMatchObject({
      blockingReason: "Resolve missing evidence requests before confirming.",
      canConfirm: false
    });
    expect(postEvidenceStatus).toMatchObject({
      reviewed: 1,
      status: "blocked",
      submitted: 1,
      unresolved: 0
    });
  });

  it("marks submitted supplemental evidence as unresolved when AI still requests it", () => {
    const items = diagnosisReviewQueueItems({
      canConfirmConclusion: false,
      collectionResults: [],
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      latestAssistantSequence: 4,
      missingEvidenceRequests: [
        {
          detail: "Attach previous container logs.",
          label: "Restart cause",
          priority: "medium"
        }
      ],
      requiresHumanReview: true,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          label: "Deployment window",
          provided_at: "2026-06-18T00:00:00Z"
        }),
        supplementalEvidenceRecord({
          label: "Restart cause",
          provided_at: "2026-06-18T00:05:00Z"
        })
      ]
    });

    expect(items.map((item) => item.kind)).toEqual([
      "supplemental_evidence_record",
      "supplemental_evidence_record"
    ]);
    expect(items[0]).toMatchObject({
      detail:
        "Attach previous container logs. AI still lists this evidence as missing after the latest review. Submitted in user turn 3 and reviewed by assistant turn 4.",
      kind: "supplemental_evidence_record",
      status: "attention",
      tag: "medium",
      title: "Submitted evidence still requested: Restart cause"
    });
    if (items[0]?.kind !== "supplemental_evidence_record") {
      throw new Error("expected unresolved supplemental evidence record");
    }
    expect(items[0].unresolvedRequest).toMatchObject({
      label: "Restart cause",
      priority: "medium"
    });
    expect(items[1]).toMatchObject({
      kind: "supplemental_evidence_record",
      status: "done",
      title: "Submitted evidence: Deployment window"
    });
  });

  it("marks submitted supplemental evidence as pending until latest AI review", () => {
    const input = {
      canConfirmConclusion: false,
      collectionResults: [],
      confidence: "medium",
      confidenceProgress: {
        label: "Confidence remained medium",
        status: "stable" as const
      },
      conclusionStatus: "needs_evidence",
      evidenceCollectionSuggestions: [],
      evidenceRequests: [],
      latestAssistantSequence: 5,
      missingEvidenceRequests: [
        {
          detail: "Attach previous container logs.",
          label: "Restart cause",
          priority: "medium"
        }
      ],
      requiresHumanReview: true,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          assistant_sequence: 4,
          label: "Restart cause",
          provided_at: "2026-06-18T00:05:00Z"
        })
      ]
    };
    const items = diagnosisReviewQueueItems(input);
    const summary = diagnosisReviewQueueSummary(items, input);
    const postEvidenceStatus = diagnosisReviewQueuePostEvidenceStatus(input);
    const taskProgress = diagnosisReviewQueueTaskProgress(
      items,
      summary,
      postEvidenceStatus
    );

    expect(items).toMatchObject([
      {
        kind: "supplemental_evidence_record",
        status: "pending",
        title: "Submitted evidence awaiting AI review: Restart cause"
      }
    ]);
    expect(summary).toMatchObject({
      blockingReason:
        "Wait for AI reassessment of submitted supplemental evidence before confirming.",
      canConfirm: false,
      pending: 1
    });
    expect(postEvidenceStatus).toEqual({
      color: "processing",
      detail:
        "1 supplemental evidence update submitted; 0 reviewed by the latest AI turn. Confidence remained medium. Latest confidence: medium. Continue the conversation so AI can reassess the submitted evidence before confirmation.",
      label: "Supplemental evidence awaiting AI review",
      reviewed: 0,
      status: "submitted",
      submitted: 1,
      unresolved: 0
    });
    expect(
      taskProgress.phases.find((phase) => phase.key === "ai_reassessment")
        ?.action
    ).toEqual({
      kind: "request_reassessment",
      label: "Ask AI to reassess"
    });
  });

  it("summarizes submitted supplemental evidence that still blocks confirmation", () => {
    expect(
      diagnosisReviewQueuePostEvidenceStatus({
        canConfirmConclusion: true,
        collectionResults: [],
        confidence: "medium",
        confidenceProgress: {
          label: "Confidence remained medium",
          status: "stable"
        },
        conclusionStatus: "needs_evidence",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        latestAssistantSequence: 5,
        missingEvidenceRequests: [
          {
            detail: "Attach previous container logs.",
            label: "Restart cause",
            priority: "medium"
          }
        ],
        requiresHumanReview: true,
        supplementalEvidence: [
          supplementalEvidenceRecord({
            assistant_sequence: 5,
            label: "Restart cause",
            provided_at: "2026-06-18T00:05:00Z"
          })
        ]
      })
    ).toEqual({
      color: "warning",
      detail:
        "1 supplemental evidence update submitted; 1 reviewed by the latest AI turn. Confidence remained medium. Latest confidence: medium. Resolve latest request for Restart cause before confirming.",
      label: "Supplemental evidence still blocking",
      reviewed: 1,
      status: "blocked",
      submitted: 1,
      unresolved: 1
    });
  });

  it("summarizes submitted supplemental evidence that is ready to retain", () => {
    expect(
      diagnosisReviewQueuePostEvidenceStatus({
        canConfirmConclusion: true,
        collectionResults: [],
        confidence: "high",
        confidenceProgress: {
          label: "Confidence improved from medium to high",
          status: "improved"
        },
        conclusionStatus: "ready_for_review",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        latestAssistantSequence: 6,
        missingEvidenceRequests: [
          {
            detail: "Attach previous container logs.",
            label: "Restart cause",
            priority: "medium"
          }
        ],
        requiresHumanReview: true,
        supplementalEvidence: [
          supplementalEvidenceRecord({
            assistant_sequence: 6,
            label: "Restart cause",
            provided_at: "2026-06-18T00:05:00Z"
          })
        ]
      })
    ).toEqual({
      color: "success",
      detail:
        "1 supplemental evidence update submitted; 1 reviewed by the latest AI turn. Confidence improved from medium to high. Latest confidence: high. No remaining review blockers; the conclusion can be confirmed and retained.",
      label: "Supplemental evidence reviewed",
      reviewed: 1,
      status: "ready",
      submitted: 1,
      unresolved: 0
    });
  });

  it("returns blocking reasons for unresolved evidence before confirmation", () => {
    expect(
      diagnosisReviewQueueBlockingReason({
        canConfirmConclusion: true,
        collectionResults: [
          evidenceResult({ status: "failed", tool: "metric_query" })
        ],
        conclusionStatus: "needs_evidence",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        missingEvidenceRequests: [],
        requiresHumanReview: true
      })
    ).toBe("Resolve metric_query evidence collection before confirming.");

    expect(
      diagnosisReviewQueueBlockingReason({
        canConfirmConclusion: true,
        collectionResults: [],
        conclusionStatus: "needs_evidence",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        latestAssistantSequence: 4,
        missingEvidenceRequests: [
          {
            detail: "Attach the latest database capacity action.",
            label: "DB capacity action",
            priority: "high"
          }
        ],
        requiresHumanReview: true
      })
    ).toBe("Resolve missing evidence requests before confirming.");

    expect(
      diagnosisReviewQueueBlockingReason({
        canConfirmConclusion: true,
        collectionResults: [],
        conclusionStatus: "needs_evidence",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        latestAssistantSequence: 4,
        missingEvidenceRequests: [
          {
            detail: "Attach previous container logs.",
            label: "Restart cause",
            priority: "medium"
          }
        ],
        requiresHumanReview: true,
        supplementalEvidence: [
          supplementalEvidenceRecord({
            label: "Restart cause",
            provided_at: "2026-06-18T00:05:00Z"
          })
        ]
      })
    ).toBe("Resolve latest request for Restart cause before confirming.");

    expect(
      diagnosisReviewQueueBlockingReason({
        canConfirmConclusion: true,
        collectionResults: [],
        conclusionStatus: "ready_for_review",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        latestAssistantSequence: 4,
        missingEvidenceRequests: [
          {
            detail: "Attach previous container logs.",
            label: "Restart cause",
            priority: "medium"
          }
        ],
        requiresHumanReview: true,
        supplementalEvidence: [
          supplementalEvidenceRecord({
            label: "Restart cause",
            provided_at: "2026-06-18T00:05:00Z"
          })
        ]
      })
    ).toBe("");

    expect(
      diagnosisReviewQueueBlockingReason({
        canConfirmConclusion: true,
        collectionResults: [],
        conclusionStatus: "ready_for_review",
        evidenceCollectionSuggestions: [],
        evidenceRequests: [
          { reason: "Read tablespace utilization", tool: "metric_query" }
        ],
        missingEvidenceRequests: [],
        requiresHumanReview: true
      })
    ).toBe("Collect planned executable evidence before confirming.");

    expect(
      diagnosisReviewQueueBlockingReason({
        canConfirmConclusion: true,
        collectionResults: [],
        conclusionStatus: "ready_for_review",
        evidenceCollectionSuggestions: [
          {
            detail: "Confirm with the service owner.",
            label: "Owner confirmation",
            priority: "low"
          }
        ],
        evidenceRequests: [],
        missingEvidenceRequests: [],
        requiresHumanReview: true
      })
    ).toBe("");
  });

  it("maps final conclusion evidence gaps into confirmation blockers", () => {
    const finalConclusion: DiagnosisFinalConclusion = {
      assistant_sequence: 4,
      content: "Diagnosis is bounded but still needs operator evidence.",
      missing_evidence_requests: [
        {
          detail: "Attach owner approval before closure.",
          label: "Owner approval",
          priority: "high"
        }
      ],
      requires_human_review: true,
      source: "latest_assistant_turn",
      status: "available"
    };
    const missingInput = finalConclusionReviewQueueInput({
      finalConclusion
    });
    expect(missingInput).not.toBeNull();
    expect(diagnosisReviewQueueBlockingReason(missingInput!)).toBe(
      "Resolve missing evidence requests before confirming."
    );

    const reviewedInput = finalConclusionReviewQueueInput({
      finalConclusion,
      supplementalEvidence: [
        supplementalEvidenceRecord({
          assistant_sequence: 4,
          detail: "Attach owner approval before closure.",
          label: "Owner approval"
        })
      ]
    });
    expect(diagnosisReviewQueueBlockingReason(reviewedInput!)).toBe("");

    const executableInput = finalConclusionReviewQueueInput({
      finalConclusion: {
        content: "Diagnosis still needs an executable check.",
        evidence_requests: [
          {
            query: "up",
            reason: "Read current service availability.",
            tool: "metric_query"
          }
        ],
        requires_human_review: true,
        source: "latest_assistant_turn",
        status: "available"
      }
    });
    expect(diagnosisReviewQueueBlockingReason(executableInput!)).toBe(
      "Collect planned executable evidence before confirming."
    );
    expect(diagnosisReviewQueueItems(executableInput!)).toMatchObject([
      {
        kind: "executable_evidence",
        status: "pending",
        tag: "metric_query",
        title: "Collect evidence: metric_query"
      }
    ]);
  });
});

function evidenceResult(
  overrides: Partial<DiagnosisEvidenceCollectionResult> = {}
): DiagnosisEvidenceCollectionResult {
  return {
    collected_at: "2026-06-18T00:00:00Z",
    message: "Evidence collected.",
    observed_alerts: 0,
    reason_code: "ok",
    request: {
      reason: "Collect evidence",
      tool: overrides.tool ?? "active_alerts"
    },
    status: "collected",
    tool: "active_alerts",
    ...overrides
  };
}

function supplementalEvidenceRecord(
  overrides: Partial<DiagnosisSupplementalEvidenceRecord> = {}
): DiagnosisSupplementalEvidenceRecord {
  return {
    assistant_message_id: "msg-2/assistant",
    assistant_sequence: 4,
    assistant_turn_id: 12,
    detail: "Attach previous container logs.",
    evidence: "Previous container logs show image pull recovery.",
    label: "Restart cause",
    priority: "high",
    provided_at: "2026-06-18T00:00:00Z",
    user_message_id: "msg-2",
    user_sequence: 3,
    user_turn_id: 11,
    ...overrides
  };
}
