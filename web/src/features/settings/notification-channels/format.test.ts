import { describe, expect, it } from "vitest";

import {
  channelToFormState,
  emptyNotificationChannelForm,
  formStateToWriteRequest,
  mergeNotificationChannelTestProofBundle,
  notificationChannelAIRoomReadiness,
  notificationChannelAIRoomUnavailableReason,
  notificationChannelAIProofInventoryReadiness,
  notificationChannelAIProofReadiness,
  notificationChannelAIProofReadyChannelIDs,
  notificationChannelAIProofRunnableChannelIDs,
  notificationChannelAIProofSummaries,
  notificationChannelEnterpriseWeChatRolloutReadiness,
  notificationChannelEditHref,
  notificationChannelEditIDFromSearchParams,
  notificationChannelCredentialReadiness,
  notificationChannelLaunchHref,
  notificationChannelLaunchInitialForm,
  notificationChannelLaunchIntentFromSearchParams,
  notificationChannelLaunchIntentKey,
  notificationChannelWorkflowReturnFromSearchParams,
  notificationChannelDeliveryReadiness,
  notificationChannelMissingAIProofContentKinds,
  notificationChannelPrimaryTestContentKind,
  notificationChannelTestProofBundleFromResults,
  notificationChannelTestContentKindLabel,
  notificationChannelTestSample,
} from "./format";
import type {
  NotificationChannelProfile,
  NotificationChannelTestContentKind,
  NotificationChannelTestResult,
} from "./types";

describe("notification channel form formatting", () => {
  it("parses notification channel launch intents from settings overview actions", () => {
    const diagnosisRoom = notificationChannelLaunchIntentFromSearchParams({
      intent: "diagnosis-room-channel",
    });
    expect(diagnosisRoom).toEqual({
      deliveryScopes: ["diagnosis_consultation", "diagnosis_close"],
      labelsText: "provider=wecom\nrole=ai-room-delivery\nscope=diagnosis-room",
      message:
        "Prepared an Enterprise WeChat channel for AI diagnosis updates and close notifications. Paste the secret reference before saving.",
      name: "AI diagnosis WeCom",
    });
    expect(notificationChannelLaunchInitialForm(diagnosisRoom)).toEqual({
      ...emptyNotificationChannelForm(),
      deliveryScopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      kind: "wecom",
      labelsText: "provider=wecom\nrole=ai-room-delivery\nscope=diagnosis-room",
      name: "AI diagnosis WeCom",
    });

    const reportClose = notificationChannelLaunchIntentFromSearchParams({
      intent: "report-close-channel",
    });
    expect(reportClose).toEqual({
      deliveryScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      labelsText:
        "provider=wecom\nrole=ai-room-delivery\nscope=report-consultation-close",
      message:
        "Prepared an Enterprise WeChat channel for final reports, automatic diagnosis updates, and close notifications. Paste the secret reference before saving.",
      name: "AI report and diagnosis WeCom",
    });
    expect(notificationChannelLaunchInitialForm(reportClose)).toEqual({
      ...emptyNotificationChannelForm(),
      deliveryScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      kind: "wecom",
      labelsText:
        "provider=wecom\nrole=ai-room-delivery\nscope=report-consultation-close",
      name: "AI report and diagnosis WeCom",
    });

    expect(
      notificationChannelLaunchIntentFromSearchParams({
        intent: "report-channel",
      })?.deliveryScopes,
    ).toEqual(["report"]);
    expect(
      notificationChannelLaunchIntentFromSearchParams({ intent: "unknown" }),
    ).toBeNull();
  });

  it("builds stable notification channel launch hrefs and keys", () => {
    const intent = notificationChannelLaunchIntentFromSearchParams({
      intent: "report-close-channel",
    });

    expect(
      notificationChannelLaunchHref({ intent: "diagnosis-room-channel" }),
    ).toBe("/settings/notification-channels?intent=diagnosis-room-channel");
    expect(
      notificationChannelLaunchHref({ intent: "report-close-channel" }),
    ).toBe("/settings/notification-channels?intent=report-close-channel");
    expect(
      notificationChannelLaunchHref({
        intent: "report-close-channel",
        workflowReturn: { sourceID: 3 },
      }),
    ).toBe(
      "/settings/notification-channels?intent=report-close-channel&workflow_return=auto-room-enable&workflow_source_id=3",
    );
    expect(notificationChannelEditHref(7)).toBe(
      "/settings/notification-channels?channel_id=7",
    );
    expect(
      notificationChannelEditHref(7, { workflowReturn: { sourceID: 3 } }),
    ).toBe(
      "/settings/notification-channels?channel_id=7&workflow_return=auto-room-enable&workflow_source_id=3",
    );
    expect(
      notificationChannelWorkflowReturnFromSearchParams({
        workflow_return: "auto-room-enable",
        workflow_source_id: "3",
      }),
    ).toEqual({
      detail:
        "Return to workflow policies after Enterprise WeChat channel scopes and AI delivery proof are ready.",
      href: "/settings/report-workflow-policies?intent=enable-ai-room-follow-up&source_id=3",
      label: "Back to workflow",
      sourceID: 3,
    });
    expect(
      notificationChannelWorkflowReturnFromSearchParams({
        workflow_return: "auto-room-enable",
        workflow_source_id: "invalid",
      }),
    ).toEqual({
      detail:
        "Return to workflow policies after Enterprise WeChat channel scopes and AI delivery proof are ready.",
      href: "/settings/report-workflow-policies?intent=enable-ai-room-follow-up",
      label: "Back to workflow",
      sourceID: null,
    });
    expect(
      notificationChannelWorkflowReturnFromSearchParams({
        workflow_return: "unknown",
      }),
    ).toBeNull();
    expect(notificationChannelEditIDFromSearchParams({ channel_id: "7" })).toBe(
      7,
    );
    expect(
      notificationChannelEditIDFromSearchParams({ channel_id: "0" }),
    ).toBeNull();
    expect(
      notificationChannelEditIDFromSearchParams({ channel_id: "not-a-number" }),
    ).toBeNull();
    expect(notificationChannelLaunchIntentKey(intent)).toBe(
      "AI report and diagnosis WeCom:report,diagnosis_consultation,diagnosis_close",
    );
    expect(notificationChannelLaunchIntentKey(null)).toBe("default");
  });

  it("builds webhook write requests with secret references only", () => {
    const parsed = formStateToWriteRequest({
      ...emptyNotificationChannelForm(),
      name: " Operations webhook ",
      secretRef: " secret/example/ops-webhook ",
      deliveryScopes: ["report", "report"],
      enabled: true,
      labelsText: "team=ops\nenv=test",
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        name: "Operations webhook",
        kind: "webhook",
        secret_ref: "secret/example/ops-webhook",
        delivery_scopes: ["report"],
        enabled: true,
        labels: {
          env: "test",
          team: "ops",
        },
      },
    });
  });

  it("rejects generic webhooks with diagnosis delivery scopes", () => {
    const parsed = formStateToWriteRequest({
      ...emptyNotificationChannelForm(),
      name: "Operations webhook",
      secretRef: "secret/example/ops-webhook",
      deliveryScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      enabled: true,
    });

    expect(parsed).toEqual({
      ok: false,
      message:
        "Diagnosis delivery scopes require an Enterprise WeChat channel.",
    });
  });

  it("builds Enterprise WeChat write requests with secret references only", () => {
    const parsed = formStateToWriteRequest({
      ...emptyNotificationChannelForm(),
      name: " Operations WeCom ",
      kind: "wecom",
      secretRef: " secret/example/ops-wecom ",
      deliveryScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      labelsText: "provider=wecom\nteam=ops",
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        name: "Operations WeCom",
        kind: "wecom",
        secret_ref: "secret/example/ops-wecom",
        delivery_scopes: [
          "diagnosis_close",
          "diagnosis_consultation",
          "report",
        ],
        enabled: true,
        labels: {
          provider: "wecom",
          team: "ops",
        },
      },
    });
  });

  it("rejects missing secret references", () => {
    const parsed = formStateToWriteRequest({
      ...emptyNotificationChannelForm(),
      name: "Operations webhook",
      secretRef: "",
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Secret reference is required.",
    });
  });

  it("rejects malformed labels", () => {
    const parsed = formStateToWriteRequest({
      ...emptyNotificationChannelForm(),
      name: "Operations webhook",
      secretRef: "secret/example/ops-webhook",
      labelsText: "owner",
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Labels must use key=value lines.",
    });
  });

  it("rejects secret and label values outside domain bounds", () => {
    expect(
      formStateToWriteRequest({
        ...emptyNotificationChannelForm(),
        name: "Operations webhook",
        secretRef: `secret/${"a".repeat(257)}`,
      }),
    ).toEqual({
      ok: false,
      message: "Secret reference must be 256 bytes or fewer.",
    });

    expect(
      formStateToWriteRequest({
        ...emptyNotificationChannelForm(),
        secretRef: "secret/example/ops-webhook",
        name: "é".repeat(61),
      }),
    ).toEqual({
      ok: false,
      message: "Channel name must be 120 bytes or fewer.",
    });

    expect(
      formStateToWriteRequest({
        ...emptyNotificationChannelForm(),
        name: "Duplicate label",
        secretRef: "secret/example/ops-webhook",
        labelsText: "team=ops\nteam=sre",
      }),
    ).toEqual({ ok: false, message: 'Label key "team" is duplicated.' });

    expect(
      formStateToWriteRequest({
        ...emptyNotificationChannelForm(),
        name: "Too many labels",
        secretRef: "secret/example/ops-webhook",
        labelsText: Array.from(
          { length: 33 },
          (_, index) => `k${index}=v`,
        ).join("\n"),
      }),
    ).toEqual({
      ok: false,
      message: "Labels must contain 32 entries or fewer.",
    });

    expect(
      formStateToWriteRequest({
        ...emptyNotificationChannelForm(),
        name: "Bad label control",
        secretRef: "secret/example/ops-webhook",
        labelsText: "team=ops\u0001",
      }),
    ).toEqual({
      ok: false,
      message: "Labels must not contain control characters.",
    });

    expect(
      formStateToWriteRequest({
        ...emptyNotificationChannelForm(),
        kind: "wecom",
        name: "Direct endpoint",
        secretRef: "https://endpoint.example.test/robot",
      }),
    ).toEqual({
      ok: false,
      message: "Secret reference must not be an endpoint URL.",
    });

    expect(
      formStateToWriteRequest({
        ...emptyNotificationChannelForm(),
        kind: "email",
        name: "Direct SMTP endpoint",
        secretRef:
          "smtps://smtp.example.test?from=alerts%40example.test&to=ops%40example.test",
      }),
    ).toEqual({
      ok: false,
      message: "Secret reference must not be an endpoint URL.",
    });
  });

  it("maps persisted channels back to edit form state", () => {
    const channel: NotificationChannelProfile = {
      id: 7,
      name: "Operations webhook",
      kind: "webhook",
      secret_ref: "secret/example/ops-webhook",
      delivery_scopes: ["report", "diagnosis_close"],
      enabled: false,
      labels: {
        team: "ops",
      },
      latest_test_results: [],
      created_at: "2026-06-05T09:00:00Z",
      updated_at: "2026-06-05T09:00:00Z",
    };

    expect(channelToFormState(channel)).toEqual({
      name: "Operations webhook",
      kind: "webhook",
      secretRef: "secret/example/ops-webhook",
      deliveryScopes: ["report", "diagnosis_close"],
      enabled: false,
      labelsText: "team=ops",
    });
  });

  it("marks delivery readiness for report-only channels as review", () => {
    expect(
      notificationChannelDeliveryReadiness(emptyNotificationChannelForm()),
    ).toEqual({
      detail:
        "The channel can be saved, but workflows using the missing scope will be blocked until it is added.",
      hasDiagnosisConsultationScope: false,
      hasDiagnosisCloseScope: false,
      hasReportScope: true,
      label: "Delivery scopes need review.",
      missingScopes: ["diagnosis_consultation", "diagnosis_close"],
      status: "review",
    });
  });

  it("marks delivery readiness pending when no scopes are selected", () => {
    expect(
      notificationChannelDeliveryReadiness({
        ...emptyNotificationChannelForm(),
        deliveryScopes: [],
      }),
    ).toEqual({
      detail:
        "Select report for final report delivery, diagnosis_consultation for AI diagnosis updates, and diagnosis_close for close notifications.",
      hasDiagnosisConsultationScope: false,
      hasDiagnosisCloseScope: false,
      hasReportScope: false,
      label: "Delivery scopes not selected.",
      missingScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      status: "pending",
    });
  });

  it("blocks generic webhooks with diagnosis delivery scopes", () => {
    expect(
      notificationChannelDeliveryReadiness({
        ...emptyNotificationChannelForm(),
        deliveryScopes: ["diagnosis_consultation", "diagnosis_close"],
      }),
    ).toEqual({
      detail:
        "Diagnosis consultation and close notifications require an Enterprise WeChat channel. Use report scope only for webhook, DingTalk, Feishu, Slack, or Email delivery.",
      hasDiagnosisConsultationScope: true,
      hasDiagnosisCloseScope: true,
      hasReportScope: false,
      label: "Enterprise WeChat required for diagnosis delivery.",
      missingScopes: ["report"],
      status: "blocked",
    });
  });

  it("marks diagnosis-only delivery scopes ready for WeCom diagnosis room channels", () => {
    expect(
      notificationChannelDeliveryReadiness({
        ...emptyNotificationChannelForm(),
        kind: "wecom",
        deliveryScopes: ["diagnosis_consultation", "diagnosis_close"],
      }),
    ).toEqual({
      detail:
        "The channel can support AI diagnosis updates and close notifications. Add report scope only when final report delivery should use this channel.",
      hasDiagnosisConsultationScope: true,
      hasDiagnosisCloseScope: true,
      hasReportScope: false,
      label: "Diagnosis delivery scopes ready.",
      missingScopes: ["report"],
      status: "ready",
    });
  });

  it("marks delivery readiness ready when report and diagnosis scopes are selected", () => {
    expect(
      notificationChannelDeliveryReadiness({
        ...emptyNotificationChannelForm(),
        kind: "wecom",
        deliveryScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      }),
    ).toEqual({
      detail:
        "The channel can support final reports, auto-room AI diagnosis updates, and close notifications.",
      hasDiagnosisConsultationScope: true,
      hasDiagnosisCloseScope: true,
      hasReportScope: true,
      label: "Delivery scopes ready.",
      missingScopes: [],
      status: "ready",
    });
  });

  it("describes credential contracts without exposing endpoint values", () => {
    expect(
      notificationChannelCredentialReadiness({
        ...emptyNotificationChannelForm(),
        kind: "wecom",
        secretRef: "secret/openclarion/ops-wecom",
      }),
    ).toEqual({
      detail:
        "Backend tests resolve this secret reference through OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON and require one HTTPS Enterprise WeChat robot webhook endpoint.",
      expectedCredential: "Enterprise WeChat robot webhook URL",
      kindLabel: "WeCom",
      label: "WeCom credential contract selected.",
      resolverEnvKey: "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
      secretRefExample: "secret/openclarion/ops-wecom",
      secretConfigured: true,
      status: "ready",
    });

    expect(
      notificationChannelCredentialReadiness({
        ...emptyNotificationChannelForm(),
        kind: "webhook",
        secretRef: "secret/openclarion/generic-webhook",
      }),
    ).toMatchObject({
      expectedCredential: "HTTP webhook URL",
      kindLabel: "Webhook",
      resolverEnvKey: "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
      secretRefExample: "secret/openclarion/ops-webhook",
      secretConfigured: true,
      status: "ready",
    });

    expect(
      notificationChannelCredentialReadiness({
        ...emptyNotificationChannelForm(),
        kind: "dingtalk",
        secretRef: "secret/openclarion/ops-dingtalk",
      }),
    ).toMatchObject({
      expectedCredential: "DingTalk robot webhook URL",
      kindLabel: "DingTalk",
      label: "DingTalk credential contract selected.",
      secretRefExample: "secret/openclarion/ops-dingtalk",
      secretConfigured: true,
      status: "ready",
    });

    expect(
      notificationChannelCredentialReadiness({
        ...emptyNotificationChannelForm(),
        kind: "feishu",
        secretRef: "secret/openclarion/ops-feishu",
      }),
    ).toMatchObject({
      expectedCredential: "Feishu or Lark custom bot webhook URL",
      kindLabel: "Feishu",
      label: "Feishu credential contract selected.",
      secretRefExample: "secret/openclarion/ops-feishu",
      secretConfigured: true,
      status: "ready",
    });

    expect(
      notificationChannelCredentialReadiness({
        ...emptyNotificationChannelForm(),
        kind: "slack",
        secretRef: "secret/openclarion/ops-slack",
      }),
    ).toMatchObject({
      expectedCredential: "Slack incoming webhook URL",
      kindLabel: "Slack",
      label: "Slack credential contract selected.",
      secretRefExample: "secret/openclarion/ops-slack",
      secretConfigured: true,
      status: "ready",
    });

    expect(
      notificationChannelCredentialReadiness({
        ...emptyNotificationChannelForm(),
        kind: "email",
        secretRef: "secret/openclarion/ops-email",
      }),
    ).toMatchObject({
      expectedCredential: "SMTP URL with from/to recipients",
      kindLabel: "Email",
      label: "Email credential contract selected.",
      secretRefExample: "secret/openclarion/ops-email",
      secretConfigured: true,
      status: "ready",
    });

    expect(
      notificationChannelCredentialReadiness(emptyNotificationChannelForm()),
    ).toMatchObject({
      expectedCredential: "HTTP webhook URL",
      label: "Credential secret not selected.",
      resolverEnvKey: "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
      secretRefExample: "secret/openclarion/ops-webhook",
      secretConfigured: false,
      status: "pending",
    });

    expect(
      notificationChannelCredentialReadiness({
        ...emptyNotificationChannelForm(),
        kind: "wecom",
        secretRef: "https://endpoint.example.test/robot",
      }),
    ).toEqual({
      detail:
        "Store endpoint URLs in server-side secret storage and enter only the secret reference here.",
      expectedCredential: "Enterprise WeChat robot webhook URL",
      kindLabel: "WeCom",
      label: "Endpoint URL cannot be stored as a secret reference.",
      resolverEnvKey: "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
      secretRefExample: "secret/openclarion/ops-wecom",
      secretConfigured: true,
      status: "blocked",
    });

    expect(
      notificationChannelCredentialReadiness({
        ...emptyNotificationChannelForm(),
        kind: "email",
        secretRef:
          "smtp://smtp.example.test?from=alerts%40example.test&to=ops%40example.test",
      }),
    ).toMatchObject({
      expectedCredential: "SMTP URL with from/to recipients",
      kindLabel: "Email",
      label: "Endpoint URL cannot be stored as a secret reference.",
      secretRefExample: "secret/openclarion/ops-email",
      secretConfigured: true,
      status: "blocked",
    });
  });

  it("describes the test notification sample from delivery scopes", () => {
    expect(
      notificationChannelTestSample(emptyNotificationChannelForm()),
    ).toEqual({
      detail: "Test sends a generic transport notification sample.",
      label: "Transport sample",
    });

    expect(
      notificationChannelTestSample({
        ...emptyNotificationChannelForm(),
        deliveryScopes: ["diagnosis_close"],
      }),
    ).toEqual({
      detail: "Test sends a diagnosis room close notification sample.",
      label: "Diagnosis close sample",
    });

    expect(
      notificationChannelTestSample({
        ...emptyNotificationChannelForm(),
        deliveryScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      }),
    ).toEqual({
      detail:
        "Test sends an AI diagnosis update sample, not a raw Alertmanager alert.",
      label: "AI diagnosis sample",
    });
  });

  it("summarizes Enterprise WeChat readiness for AI diagnosis rooms", () => {
    expect(
      notificationChannelAIRoomReadiness(
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          kind: "wecom",
        }),
      ),
    ).toEqual({
      detail:
        "Ready for AI diagnosis updates and close notifications through Enterprise WeChat.",
      label: "AI diagnosis delivery ready.",
      missingScopes: [],
      status: "ready",
      unavailableReason: "",
    });

    expect(
      notificationChannelAIRoomReadiness(
        channel({
          delivery_scopes: ["diagnosis_consultation"],
          enabled: true,
          kind: "wecom",
        }),
      ),
    ).toEqual({
      detail:
        "Add diagnosis_consultation and diagnosis_close scopes before using this channel for AI diagnosis rooms.",
      label: "AI diagnosis scopes need review.",
      missingScopes: ["diagnosis_close"],
      status: "review",
      unavailableReason: "missing diagnosis_close",
    });

    expect(
      notificationChannelAIRoomReadiness(
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: false,
          kind: "wecom",
        }),
      ),
    ).toMatchObject({
      label: "Channel disabled.",
      status: "blocked",
    });

    expect(
      notificationChannelAIRoomReadiness(
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          kind: "webhook",
        }),
      ),
    ).toMatchObject({
      label: "Enterprise WeChat required.",
      status: "blocked",
    });

    expect(
      notificationChannelAIRoomUnavailableReason(
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          kind: "wecom",
          secret_ref: "https://endpoint.example.test/robot",
        }),
      ),
    ).toBe("credential secret reference stores an endpoint URL");
  });

  it("tracks AI diagnosis channel test proof by content kind", () => {
    const current = mergeNotificationChannelTestProofBundle(
      undefined,
      channelTestResult({ content_kind: "ai_diagnosis_sample" }),
    );
    const merged = mergeNotificationChannelTestProofBundle(
      current,
      channelTestResult({ content_kind: "diagnosis_close_sample" }),
    );

    expect(merged.ai_diagnosis_sample?.content_kind).toBe(
      "ai_diagnosis_sample",
    );
    expect(merged.diagnosis_close_sample?.content_kind).toBe(
      "diagnosis_close_sample",
    );
  });

  it("keeps the newest channel test proof when content kinds repeat", () => {
    const newer = channelTestResult({
      checked_at: "2026-06-05T09:05:00Z",
      content_kind: "ai_diagnosis_sample",
      provider_message_id: "newer-proof",
    });
    const older = channelTestResult({
      checked_at: "2026-06-05T09:01:00Z",
      content_kind: "ai_diagnosis_sample",
      provider_message_id: "older-proof",
    });

    expect(
      notificationChannelTestProofBundleFromResults([newer, older])
        .ai_diagnosis_sample?.provider_message_id,
    ).toBe("newer-proof");
    expect(
      mergeNotificationChannelTestProofBundle(
        { ai_diagnosis_sample: newer },
        older,
      ).ai_diagnosis_sample?.provider_message_id,
    ).toBe("newer-proof");
    expect(
      mergeNotificationChannelTestProofBundle(
        {
          ai_diagnosis_sample: channelTestResult({
            checked_at: "not-a-timestamp",
            content_kind: "ai_diagnosis_sample",
            provider_message_id: "invalid-existing-proof",
          }),
        },
        newer,
      ).ai_diagnosis_sample?.provider_message_id,
    ).toBe("newer-proof");
  });

  it("requires current AI diagnosis and close sample proof for AI room delivery", () => {
    const aiRoom = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 7,
      kind: "wecom",
      updated_at: "2026-06-05T09:00:00Z",
    });

    expect(notificationChannelAIProofReadiness(aiRoom)).toEqual({
      detail:
        "Run AI diagnosis sample and Diagnosis close sample tests after the latest channel update before relying on this channel for AI diagnosis rooms.",
      label: "AI delivery test proof needs review.",
      missingContentKinds: ["ai_diagnosis_sample", "diagnosis_close_sample"],
      status: "review",
    });

    expect(
      notificationChannelAIProofReadiness(aiRoom, {
        ai_diagnosis_sample: channelTestResult({
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        diagnosis_close_sample: channelTestResult({
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      }),
    ).toEqual({
      detail:
        "AI diagnosis update and close notification samples both succeeded after the latest channel update.",
      label: "AI delivery test proof ready.",
      missingContentKinds: [],
      status: "ready",
    });

    expect(
      notificationChannelAIProofReadiness(aiRoom, {
        ai_diagnosis_sample: channelTestResult({
          checked_at: "2026-06-05T08:59:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        diagnosis_close_sample: channelTestResult({
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      }),
    ).toMatchObject({
      missingContentKinds: ["ai_diagnosis_sample"],
      status: "review",
    });
  });

  it("summarizes Enterprise WeChat AI proof inventory across configured channels", () => {
    const ready = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 11,
      kind: "wecom",
      latest_test_results: [
        channelTestResult({
          channel_id: 11,
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        channelTestResult({
          channel_id: 11,
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      ],
      updated_at: "2026-06-05T09:00:00Z",
    });
    const needsProof = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 12,
      kind: "wecom",
      latest_test_results: [],
    });
    const blocked = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 13,
      kind: "webhook",
      latest_test_results: [],
    });

    expect(
      notificationChannelAIProofInventoryReadiness([
        channel({ delivery_scopes: ["report"], id: 10, kind: "webhook" }),
      ]),
    ).toMatchObject({
      candidateChannelIDs: [],
      label: "No AI diagnosis delivery channel configured.",
      status: "pending",
    });

    expect(
      notificationChannelAIProofInventoryReadiness([
        ready,
        needsProof,
        blocked,
      ]),
    ).toEqual({
      blockedChannelIDs: [13],
      candidateChannelIDs: [11, 12, 13],
      detail:
        "1 AI diagnosis delivery channel can be used now; 2 candidate channels still need setup or proof review.",
      label: "1 AI delivery channel proof-ready.",
      missingContentKinds: ["ai_diagnosis_sample", "diagnosis_close_sample"],
      readyChannelIDs: [11],
      reviewChannelIDs: [12],
      status: "review",
    });

    expect(notificationChannelAIProofInventoryReadiness([ready])).toMatchObject(
      {
        blockedChannelIDs: [],
        candidateChannelIDs: [11],
        readyChannelIDs: [11],
        reviewChannelIDs: [],
        status: "ready",
      },
    );

    expect(
      notificationChannelAIProofInventoryReadiness([blocked]),
    ).toMatchObject({
      blockedChannelIDs: [13],
      candidateChannelIDs: [13],
      readyChannelIDs: [],
      reviewChannelIDs: [],
      status: "blocked",
    });

    expect(
      notificationChannelAIProofInventoryReadiness([
        channel({
          delivery_scopes: ["report"],
          enabled: true,
          id: 14,
          kind: "wecom",
        }),
      ]),
    ).toMatchObject({
      detail:
        "Add diagnosis_consultation and diagnosis_close scopes before collecting AI delivery proof for Enterprise WeChat.",
      missingContentKinds: [],
      reviewChannelIDs: [14],
      status: "review",
    });
  });

  it("selects only runnable Enterprise WeChat AI proof channels", () => {
    const ready = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 21,
      kind: "wecom",
      latest_test_results: [
        channelTestResult({
          channel_id: 21,
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        channelTestResult({
          channel_id: 21,
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      ],
    });
    const needsProof = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 22,
      kind: "wecom",
      latest_test_results: [],
    });
    const reportOnly = channel({
      delivery_scopes: ["report"],
      enabled: true,
      id: 23,
      kind: "wecom",
      latest_test_results: [],
    });
    const disabled = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: false,
      id: 24,
      kind: "wecom",
      latest_test_results: [],
    });
    const genericWebhook = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 25,
      kind: "webhook",
      latest_test_results: [],
    });

    expect(
      notificationChannelAIProofRunnableChannelIDs([
        ready,
        needsProof,
        reportOnly,
        disabled,
        genericWebhook,
      ]),
    ).toEqual([22]);
  });

  it("selects only proof-ready Enterprise WeChat AI delivery channels", () => {
    const ready = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 41,
      kind: "wecom",
      latest_test_results: [
        channelTestResult({
          channel_id: 41,
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        channelTestResult({
          channel_id: 41,
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      ],
    });
    const reportOnly = channel({
      delivery_scopes: ["report"],
      enabled: true,
      id: 42,
      kind: "wecom",
      latest_test_results: [],
    });
    const missingProof = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 43,
      kind: "wecom",
      latest_test_results: [],
    });
    const genericWebhook = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 44,
      kind: "webhook",
      latest_test_results: [],
    });

    expect(
      notificationChannelAIProofReadyChannelIDs([
        ready,
        reportOnly,
        missingProof,
        genericWebhook,
      ]),
    ).toEqual([41]);
  });

  it("summarizes Enterprise WeChat rollout readiness for automatic diagnosis", () => {
    const ready = channel({
      delivery_scopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 31,
      kind: "wecom",
      latest_test_results: [
        channelTestResult({
          channel_id: 31,
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        channelTestResult({
          channel_id: 31,
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      ],
      updated_at: "2026-06-05T09:00:00Z",
    });
    const missingReport = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 32,
      kind: "wecom",
      latest_test_results: [],
    });
    const needsProof = channel({
      delivery_scopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 33,
      kind: "wecom",
      latest_test_results: [],
    });
    const blocked = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 34,
      kind: "webhook",
      latest_test_results: [],
    });

    expect(
      notificationChannelEnterpriseWeChatRolloutReadiness([
        channel({ delivery_scopes: ["report"], id: 30, kind: "webhook" }),
      ]),
    ).toMatchObject({
      candidateChannelIDs: [],
      missingScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      status: "pending",
    });

    expect(
      notificationChannelEnterpriseWeChatRolloutReadiness([ready]),
    ).toMatchObject({
      blockedChannelIDs: [],
      candidateChannelIDs: [31],
      missingContentKinds: [],
      missingScopes: [],
      readyChannelIDs: [31],
      reviewChannelIDs: [],
      status: "ready",
    });

    expect(
      notificationChannelEnterpriseWeChatRolloutReadiness([missingReport]),
    ).toMatchObject({
      candidateChannelIDs: [32],
      missingScopes: ["report"],
      readyChannelIDs: [],
      reviewChannelIDs: [32],
      status: "review",
    });

    expect(
      notificationChannelEnterpriseWeChatRolloutReadiness([needsProof]),
    ).toMatchObject({
      candidateChannelIDs: [33],
      missingContentKinds: ["ai_diagnosis_sample", "diagnosis_close_sample"],
      readyChannelIDs: [],
      reviewChannelIDs: [33],
      status: "review",
    });

    expect(
      notificationChannelEnterpriseWeChatRolloutReadiness([blocked]),
    ).toMatchObject({
      blockedChannelIDs: [34],
      candidateChannelIDs: [34],
      readyChannelIDs: [],
      reviewChannelIDs: [],
      status: "blocked",
    });

    expect(
      notificationChannelEnterpriseWeChatRolloutReadiness([
        ready,
        missingReport,
        needsProof,
        blocked,
      ]),
    ).toMatchObject({
      blockedChannelIDs: [34],
      candidateChannelIDs: [31, 32, 33, 34],
      missingContentKinds: ["ai_diagnosis_sample", "diagnosis_close_sample"],
      missingScopes: ["report"],
      readyChannelIDs: [31],
      reviewChannelIDs: [32, 33],
      status: "review",
    });
  });

  it("summarizes persisted Enterprise WeChat AI proof status and timestamps", () => {
    const current = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 21,
      kind: "wecom",
      latest_test_results: [
        channelTestResult({
          channel_id: 21,
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        channelTestResult({
          channel_id: 21,
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      ],
      name: "Current WeCom",
      updated_at: "2026-06-05T09:00:00Z",
    });
    const staleAndFailed = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 22,
      kind: "wecom",
      latest_test_results: [
        channelTestResult({
          channel_id: 22,
          checked_at: "2026-06-05T08:59:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        channelTestResult({
          channel_id: 22,
          checked_at: "2026-06-05T09:03:00Z",
          content_kind: "diagnosis_close_sample",
          status: "failed",
        }),
      ],
      name: "Stale WeCom",
      updated_at: "2026-06-05T09:00:00Z",
    });
    const missing = channel({
      delivery_scopes: ["diagnosis_consultation"],
      enabled: true,
      id: 23,
      kind: "wecom",
      latest_test_results: [],
      name: "Missing WeCom",
    });

    expect(
      notificationChannelAIProofSummaries([
        channel({ delivery_scopes: ["report"], id: 20, kind: "wecom" }),
        current,
        staleAndFailed,
        missing,
      ]),
    ).toEqual([
      {
        channelID: 21,
        channelName: "Current WeCom",
        contents: [
          {
            checkedAt: "2026-06-05T09:01:00Z",
            contentKind: "ai_diagnosis_sample",
            label: "AI diagnosis sample",
            status: "current",
          },
          {
            checkedAt: "2026-06-05T09:02:00Z",
            contentKind: "diagnosis_close_sample",
            label: "Diagnosis close sample",
            status: "current",
          },
        ],
        missingContentKinds: [],
        status: "ready",
      },
      {
        channelID: 22,
        channelName: "Stale WeCom",
        contents: [
          {
            checkedAt: "2026-06-05T08:59:00Z",
            contentKind: "ai_diagnosis_sample",
            label: "AI diagnosis sample",
            status: "stale",
          },
          {
            checkedAt: "2026-06-05T09:03:00Z",
            contentKind: "diagnosis_close_sample",
            label: "Diagnosis close sample",
            status: "failed",
          },
        ],
        missingContentKinds: ["ai_diagnosis_sample", "diagnosis_close_sample"],
        status: "review",
      },
      {
        channelID: 23,
        channelName: "Missing WeCom",
        contents: [
          {
            contentKind: "ai_diagnosis_sample",
            label: "AI diagnosis sample",
            status: "missing",
          },
        ],
        missingContentKinds: ["ai_diagnosis_sample"],
        status: "review",
      },
    ]);
  });

  it("rejects malformed AI proof digests before marking WeCom proof ready", () => {
    const aiRoom = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 7,
      kind: "wecom",
      updated_at: "2026-06-05T09:00:00Z",
    });

    expect(
      notificationChannelAIProofReadiness(aiRoom, {
        ai_diagnosis_sample: channelTestResult({
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
          content_sha256: "A".repeat(64),
        }),
        diagnosis_close_sample: channelTestResult({
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
          content_sha256: "not-a-sha256",
        }),
      }),
    ).toMatchObject({
      missingContentKinds: ["ai_diagnosis_sample", "diagnosis_close_sample"],
      status: "review",
    });
  });

  it("selects the primary test sample from missing AI room proof", () => {
    const aiRoom = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 7,
      kind: "wecom",
      updated_at: "2026-06-05T09:00:00Z",
    });

    expect(notificationChannelPrimaryTestContentKind(aiRoom, undefined)).toBe(
      "ai_diagnosis_sample",
    );
    expect(
      notificationChannelPrimaryTestContentKind(aiRoom, {
        ai_diagnosis_sample: channelTestResult({
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
      }),
    ).toBe("diagnosis_close_sample");
    expect(
      notificationChannelPrimaryTestContentKind(aiRoom, {
        ai_diagnosis_sample: channelTestResult({
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        diagnosis_close_sample: channelTestResult({
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      }),
    ).toBeUndefined();
    expect(
      notificationChannelPrimaryTestContentKind(
        channel({ delivery_scopes: ["report"], kind: "webhook" }),
        undefined,
      ),
    ).toBeUndefined();
  });

  it("lists missing AI proof samples for one-click Enterprise WeChat proof collection", () => {
    const aiRoom = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 7,
      kind: "wecom",
      updated_at: "2026-06-05T09:00:00Z",
    });

    expect(notificationChannelMissingAIProofContentKinds(aiRoom)).toEqual([
      "ai_diagnosis_sample",
      "diagnosis_close_sample",
    ]);
    expect(
      notificationChannelMissingAIProofContentKinds(aiRoom, {
        ai_diagnosis_sample: channelTestResult({
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
      }),
    ).toEqual(["diagnosis_close_sample"]);
    expect(
      notificationChannelMissingAIProofContentKinds(aiRoom, {
        ai_diagnosis_sample: channelTestResult({
          checked_at: "2026-06-05T09:01:00Z",
          content_kind: "ai_diagnosis_sample",
        }),
        diagnosis_close_sample: channelTestResult({
          checked_at: "2026-06-05T09:02:00Z",
          content_kind: "diagnosis_close_sample",
        }),
      }),
    ).toEqual([]);
  });

  it("labels notification channel test content proof kinds", () => {
    expect(notificationChannelTestContentKindLabel("ai_diagnosis_sample")).toBe(
      "AI diagnosis sample",
    );
    expect(
      notificationChannelTestContentKindLabel("diagnosis_close_sample"),
    ).toBe("Diagnosis close sample");
    expect(notificationChannelTestContentKindLabel("transport_sample")).toBe(
      "Transport sample",
    );
    expect(notificationChannelTestContentKindLabel(undefined)).toBe(
      "Content proof missing",
    );
  });
});

function channel(
  overrides: Partial<NotificationChannelProfile>,
): NotificationChannelProfile {
  return {
    created_at: "2026-06-05T09:00:00Z",
    delivery_scopes: ["report"],
    enabled: true,
    id: 7,
    kind: "webhook",
    labels: {},
    latest_test_results: [],
    name: "Operations channel",
    secret_ref: "secret/example/channel",
    updated_at: "2026-06-05T09:00:00Z",
    ...overrides,
  };
}

function channelTestResult(
  overrides: Partial<NotificationChannelTestResult> & {
    content_kind: NotificationChannelTestContentKind;
  },
): NotificationChannelTestResult {
  const { content_kind: contentKind, ...rest } = overrides;
  return {
    channel_id: 7,
    checked_at: "2026-06-05T09:01:00Z",
    content_kind: contentKind,
    content_sha256: "a".repeat(64),
    kind: "wecom",
    message: "Notification channel test delivery succeeded.",
    provider_message_id: "provider-message-1",
    provider_status: "accepted",
    reason_code: "ok",
    status: "success",
    ...rest,
  };
}
