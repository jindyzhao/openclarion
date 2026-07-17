import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";
import zhCN from "../../../messages/zh-CN.json";
import {
  localizeSupplementalEvidenceReassessmentMessage,
  localizeSupplementalEvidenceResidualBoundaryTemplate,
  localizeSupplementalEvidenceSubmissionMessage,
} from "./supplemental-evidence-copy";
import type { DiagnosisConsultationEvidenceRequest } from "./types";

const tEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "DiagnosisRoom.supplementalEvidencePrompt",
});
const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.supplementalEvidencePrompt",
});
const tStatus = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.status",
});
const tStatusEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "DiagnosisRoom.status",
});

describe("supplemental evidence copy", () => {
  it("localizes submission instructions while retaining executable context", () => {
    const message = localizeSupplementalEvidenceSubmissionMessage(
      request(),
      "Operator verified the query output.",
      t,
      tStatus,
    );

    expect(message).toContain("补充证据更新");
    expect(message).toContain("优先级：高");
    expect(message).toContain("工具：metric_range_query");
    expect(message).toContain("查询：rate(cpu_usage[5m])");
    expect(message).toContain("Operator verified the query output.");
    expect(message).toContain("不得捏造不可用的事实");
  });

  it("localizes residual boundaries and retained reassessment evidence", () => {
    const residual = localizeSupplementalEvidenceResidualBoundaryTemplate(
      request(),
      t,
    );
    const reassessment = localizeSupplementalEvidenceReassessmentMessage(
      {
        latestAssistantSequence: 2,
        records: [
          {
            assistant_message_id: "assistant-1",
            assistant_sequence: 1,
            assistant_turn_id: 1,
            detail: request().detail,
            evidence: "Operator verified the query output.",
            label: request().label,
            priority: request().priority,
            provided_at: "2026-07-16T00:02:00Z",
            user_message_id: "user-1",
            user_sequence: 1,
            user_turn_id: 2,
          },
        ],
      },
      t,
      tStatus,
    );

    expect(residual).toContain("操作员接受此项作为审核时的剩余不确定性");
    expect(reassessment).toContain("重新评估已保留的诊断证据");
    expect(reassessment).toContain("1. CPU trend（高）");
    expect(reassessment).toContain(
      "证据摘要：Operator verified the query output.",
    );
  });

  it("keeps executable request context in localized submission instructions", () => {
    const message = localizeSupplementalEvidenceSubmissionMessage(
      request(),
      "Operator provided query output.",
      tEn,
      tStatusEn,
    );

    expect(message).toContain(
      [
        "Original executable evidence request:",
        "Tool: metric_range_query",
        "Reason: Inspect CPU saturation.",
        "Query: rate(cpu_usage[5m])",
        "Template: 5",
        "Alert source profile: 7",
        "Window: 300s",
        "Step: 60s",
        "Limit: 20",
      ].join("\n"),
    );
    expect(message).toContain(
      "Evidence provided:\nOperator provided query output.",
    );
    expect(message).toContain(
      "Keep final reserved for conclusions without unresolved evidence, and never fabricate unavailable facts.",
    );
  });

  it("retains only pending records and bounded executable evidence", () => {
    const message = localizeSupplementalEvidenceReassessmentMessage(
      {
        collectionResults: [
          {
            collected_at: "2026-06-18T00:10:00Z",
            message: "PromQL query returned two saturated CPU series.",
            observed_alerts: 0,
            observed_metric_series: 2,
            query: "rate(cpu_usage[5m])",
            reason_code: "ok",
            request: request().source_request!,
            status: "collected",
            tool: "metric_range_query",
          },
        ],
        latestAssistantSequence: 4,
        records: [
          supplementalEvidenceRecord({
            assistant_sequence: 4,
            evidence: "Already reviewed.",
            label: "Reviewed evidence",
          }),
          supplementalEvidenceRecord({
            assistant_sequence: 3,
            evidence: `${"CPU sample ".repeat(60)}tail`,
            label: "CPU range sample",
          }),
        ],
      },
      tEn,
      tStatusEn,
    );
    const excerptLine = message
      .split("\n")
      .find((line) => line.startsWith("Evidence excerpt: "));

    expect(message).toContain(
      "Executable evidence collection retained for reassessment:",
    );
    expect(message).toContain("1. metric_range_query (collected)");
    expect(message).toContain(
      "Message: PromQL query returned two saturated CPU series.",
    );
    expect(message).not.toContain("Reviewed evidence");
    expect(excerptLine).toBeDefined();
    expect(excerptLine?.endsWith("...")).toBe(true);
    expect(excerptLine!.length).toBeLessThanOrEqual(
      "Evidence excerpt: ".length + 360,
    );
    expect(excerptLine).not.toContain("tail");
  });
});

function request(): DiagnosisConsultationEvidenceRequest {
  return {
    detail: "Provide verified CPU trend evidence.",
    label: "CPU trend",
    priority: "high",
    source_request: {
      alert_source_profile_id: 7,
      limit: 20,
      query: "rate(cpu_usage[5m])",
      reason: "Inspect CPU saturation.",
      step_seconds: 60,
      template_id: 5,
      tool: "metric_range_query",
      window_seconds: 300,
    },
  };
}

function supplementalEvidenceRecord(
  overrides: Partial<{
    assistant_sequence: number;
    evidence: string;
    label: string;
  }> = {},
) {
  return {
    assistant_message_id: "assistant-1",
    assistant_sequence: overrides.assistant_sequence ?? 0,
    assistant_turn_id: 0,
    detail: "Attach supporting operator evidence.",
    evidence: overrides.evidence ?? "Operator evidence.",
    label: overrides.label ?? "Operator action",
    priority: "medium",
    provided_at: "2026-06-18T00:00:00Z",
    user_message_id: "user-1",
    user_sequence: 1,
    user_turn_id: 1,
  };
}
