import { describe, expect, it } from "vitest";

import {
  alertSourceCanonicalBaseURL,
  alertSourceAIRoleReadiness,
  alertSourceClassificationHint,
  alertmanagerWebhookDeliveryConfig,
  alertmanagerWebhookEndpoint,
  alertmanagerWebhookPublicBaseURL,
  alertSourceLaunchHref,
  alertSourceLaunchInitialForm,
  alertSourceLaunchIntentFromSearchParams,
  alertSourceLaunchIntentKey,
  alertSourceConnectionTarget,
  alertSourceConnectionTargets,
  alertSourceOperatorChecklist,
  alertSourcePresetOptions,
  alertSourceProviderGuidance,
  alertSourceReadiness,
  emptyAlertSourceForm,
  formStateToWriteRequest,
  labelsToText,
  parseLabelsText,
} from "./format";
import type { AlertSourceProfile } from "./types";

describe("alert source settings formatting", () => {
  it("parses alert source launch intents from settings overview actions", () => {
    const alertmanager = alertSourceLaunchIntentFromSearchParams({
      intent: "alertmanager-source",
    });
    expect(alertmanager).toEqual({
      baseURL: "",
      kind: "alertmanager",
      labelsText: "role=alert-intake\nsource=alertmanager",
      message:
        "Prepared an enabled Alertmanager source. Paste the base URL, then save and test it before binding workflows.",
      name: "Alertmanager alert intake",
      workflowReturn: null,
    });
    expect(alertSourceLaunchInitialForm(alertmanager)).toEqual({
      ...emptyAlertSourceForm(),
      enabled: true,
      kind: "alertmanager",
      labelsText: "role=alert-intake\nsource=alertmanager",
      name: "Alertmanager alert intake",
    });

    expect(
      alertSourceLaunchIntentFromSearchParams({ intent: "prometheus-source" })
        ?.kind,
    ).toBe("prometheus");
    const thanosRule = alertSourceLaunchIntentFromSearchParams({
      intent: "thanos-rule-source",
    });
    expect(thanosRule).toEqual({
      baseURL: "",
      kind: "prometheus",
      labelsText: "role=alert-intake\nsource=thanos-rule",
      message:
        "Prepared an enabled Thanos Rule active-alert source. Paste the Thanos Rule alerts URL or API base URL, then save and test it before adding active-alert evidence. Use Alertmanager for webhook-triggered automatic diagnosis rooms.",
      name: "Thanos Rule active alerts",
      workflowReturn: null,
    });
    expect(alertSourceLaunchInitialForm(thanosRule)).toEqual({
      ...emptyAlertSourceForm(),
      enabled: true,
      kind: "prometheus",
      labelsText: "role=alert-intake\nsource=thanos-rule",
      name: "Thanos Rule active alerts",
    });
    const thanos = alertSourceLaunchIntentFromSearchParams({
      intent: "thanos-source",
    });
    expect(thanos).toEqual({
      baseURL: "",
      kind: "prometheus",
      labelsText: "role=metric-evidence\nsource=thanos",
      message:
        "Prepared an enabled Thanos Query source. Paste the base URL, then save and test it before adding metric evidence tools.",
      name: "Thanos metric evidence",
      workflowReturn: null,
    });
    expect(alertSourceLaunchInitialForm(thanos)).toEqual({
      ...emptyAlertSourceForm(),
      enabled: true,
      kind: "prometheus",
      labelsText: "role=metric-evidence\nsource=thanos",
      name: "Thanos metric evidence",
    });
    expect(
      alertSourceLaunchIntentFromSearchParams({ intent: "unknown" }),
    ).toBeNull();
  });

  it("builds stable alert source launch hrefs and keys", () => {
    const intent = alertSourceLaunchIntentFromSearchParams({
      intent: "alertmanager-source",
    });

    expect(alertSourceLaunchHref({ intent: "alertmanager-source" })).toBe(
      "/settings/alert-sources?intent=alertmanager-source",
    );
    expect(alertSourceLaunchHref({ intent: "thanos-source" })).toBe(
      "/settings/alert-sources?intent=thanos-source",
    );
    expect(alertSourceLaunchHref({ intent: "thanos-rule-source" })).toBe(
      "/settings/alert-sources?intent=thanos-rule-source",
    );
    expect(
      alertSourceLaunchHref({
        baseURL: "https://query.example.test",
        intent: "thanos-source",
      }),
    ).toBe(
      "/settings/alert-sources?intent=thanos-source&base_url=https%3A%2F%2Fquery.example.test",
    );
    expect(
      alertSourceLaunchHref({
        intent: "thanos-source",
        workflowReturn: { sourceID: 3 },
      }),
    ).toBe(
      "/settings/alert-sources?intent=thanos-source&workflow_return=auto-room-enable&workflow_source_id=3",
    );
    expect(
      alertSourceLaunchIntentFromSearchParams({
        intent: "thanos-source",
        workflow_return: "auto-room-enable",
        workflow_source_id: "3",
      })?.workflowReturn,
    ).toMatchObject({
      href: "/settings/report-workflow-policies?intent=enable-ai-room-follow-up&source_id=3",
      label: "Back to workflow",
      sourceID: 3,
    });
    expect(alertSourceLaunchIntentKey(intent)).toBe(
      "alertmanager:Alertmanager alert intake::none",
    );
    expect(alertSourceLaunchIntentKey(null)).toBe("default");
  });

  it("prefills safe alert source base URLs from launch search params", () => {
    const thanos = alertSourceLaunchIntentFromSearchParams({
      intent: "thanos-source",
      base_url: "https://query.example.test",
    });

    expect(thanos).toEqual({
      baseURL: "https://query.example.test",
      kind: "prometheus",
      labelsText: "role=metric-evidence\nsource=thanos",
      message:
        "Prepared an enabled Thanos Query source. Paste the base URL, then save and test it before adding metric evidence tools.",
      name: "Thanos metric evidence",
      workflowReturn: null,
    });
    expect(alertSourceLaunchInitialForm(thanos)).toEqual({
      ...emptyAlertSourceForm(),
      baseURL: "https://query.example.test",
      enabled: true,
      kind: "prometheus",
      labelsText: "role=metric-evidence\nsource=thanos",
      name: "Thanos metric evidence",
    });
    expect(alertSourceLaunchIntentKey(thanos)).toBe(
      "prometheus:Thanos metric evidence:https://query.example.test:none",
    );
  });

  it("ignores unsafe launch base URLs", () => {
    for (const baseURL of [
      " https://query.example.test",
      "https://user@example.test",
      "https://query.example.test?tenant=prod",
      "https://query.example.test#alerts",
      "ssh://query.example.test",
    ]) {
      const intent = alertSourceLaunchIntentFromSearchParams({
        intent: "thanos-source",
        base_url: baseURL,
      });

      expect(alertSourceLaunchInitialForm(intent).baseURL).toBe("");
    }
  });

  it("builds alert source presets for common alert and metric sources", () => {
    expect(alertSourcePresetOptions()).toEqual([
      {
        detail: "Prometheus-compatible active alerts from Thanos Rule.",
        form: {
          ...emptyAlertSourceForm(),
          enabled: true,
          kind: "prometheus",
          labelsText: "role=alert-intake\nsource=thanos-rule",
          name: "Thanos Rule active alerts",
        },
        intent: "thanos-rule-source",
        label: "Thanos Rule",
        message:
          "Prepared an enabled Thanos Rule active-alert source. Paste the Thanos Rule alerts URL or API base URL, then save and test it before adding active-alert evidence. Use Alertmanager for webhook-triggered automatic diagnosis rooms.",
      },
      {
        detail: "Prometheus-compatible metric evidence.",
        form: {
          ...emptyAlertSourceForm(),
          enabled: true,
          kind: "prometheus",
          labelsText: "role=metric-evidence\nsource=thanos",
          name: "Thanos metric evidence",
        },
        intent: "thanos-source",
        label: "Thanos Query",
        message:
          "Prepared an enabled Thanos Query source. Paste the base URL, then save and test it before adding metric evidence tools.",
      },
      {
        detail: "Generic Alertmanager-compatible active alerts and webhooks.",
        form: {
          ...emptyAlertSourceForm(),
          enabled: true,
          kind: "alertmanager",
          labelsText: "role=alert-intake\nsource=alertmanager",
          name: "Alertmanager alert intake",
        },
        intent: "alertmanager-source",
        label: "Alertmanager",
        message:
          "Prepared an enabled Alertmanager source. Paste the base URL, then save and test it before binding workflows.",
      },
      {
        detail: "Generic Prometheus-compatible alerts and metric evidence.",
        form: {
          ...emptyAlertSourceForm(),
          enabled: true,
          kind: "prometheus",
          labelsText: "role=metric-evidence\nsource=prometheus",
          name: "Prometheus metric evidence",
        },
        intent: "prometheus-source",
        label: "Prometheus",
        message:
          "Prepared an enabled Prometheus-compatible source. Paste the base URL, then save and test it before adding metric evidence tools.",
      },
    ]);
  });

  it("parses labels from key-value lines", () => {
    const result = parseLabelsText("owner=platform\nenv=prod\n");

    expect(result).toEqual({
      ok: true,
      value: {
        owner: "platform",
        env: "prod",
      },
    });
  });

  it("rejects malformed and duplicate labels", () => {
    expect(parseLabelsText("owner").ok).toBe(false);
    expect(parseLabelsText("owner=platform\nowner=sre").ok).toBe(false);
  });

  it("formats labels in stable key order", () => {
    expect(labelsToText({ owner: "platform", env: "prod" })).toBe(
      "env=prod\nowner=platform",
    );
  });

  it("builds a profile write request without secret values", () => {
    const form = {
      ...emptyAlertSourceForm(),
      name: "Primary Prometheus",
      baseURL: "https://prometheus.example.test",
      authMode: "bearer" as const,
      secretRef: "secret/openclarion/prometheus-bearer",
      enabled: true,
      labelsText: "env=prod",
    };

    expect(formStateToWriteRequest(form)).toEqual({
      ok: true,
      value: {
        name: "Primary Prometheus",
        kind: "prometheus",
        base_url: "https://prometheus.example.test",
        auth_mode: "bearer",
        secret_ref: "secret/openclarion/prometheus-bearer",
        enabled: true,
        labels: { env: "prod" },
      },
    });
  });

  it("marks alert source readiness pending until required fields are complete", () => {
    expect(alertSourceReadiness(emptyAlertSourceForm())).toEqual({
      capabilities: [
        "Active alert listing",
        "Instant metric evidence",
        "Range metric evidence",
      ],
      detail: "Profile name is required.",
      label: "Complete source configuration.",
      status: "pending",
    });
  });

  it("marks enabled Prometheus-compatible sources ready for alert and metric evidence", () => {
    expect(
      alertSourceReadiness({
        ...emptyAlertSourceForm(),
        name: "Primary Prometheus",
        baseURL: "https://prometheus.example.test",
        enabled: true,
      }),
    ).toEqual({
      capabilities: [
        "Active alert listing",
        "Instant metric evidence",
        "Range metric evidence",
      ],
      detail:
        "Prometheus-compatible sources support active alert reads and metric evidence collection.",
      label: "Source ready for workflows.",
      status: "ready",
    });
  });

  it("marks enabled Thanos Rule sources as active-alert only", () => {
    expect(
      alertSourceReadiness({
        ...emptyAlertSourceForm(),
        name: "Thanos Rule active alerts",
        baseURL: "https://thanos-rule.example.test/alerts",
        enabled: true,
        labelsText: "role=alert-intake\nsource=thanos-rule",
      }),
    ).toEqual({
      capabilities: [
        "Active alert listing",
        "Thanos Rule alerts API",
        "Metric queries not required",
      ],
      detail:
        "Thanos Rule active-alert sources read firing alerts from /api/v1/alerts. Use Thanos Query for metric evidence and Alertmanager for webhook-triggered automatic diagnosis rooms.",
      label: "Source ready for workflows.",
      status: "ready",
    });
  });

  it("marks enabled Alertmanager sources ready for active-only alert reads and webhook ingest", () => {
    expect(
      alertSourceReadiness({
        ...emptyAlertSourceForm(),
        kind: "alertmanager",
        name: "Operations Alertmanager",
        baseURL: "https://alertmanager.example.test/api/v2/alerts",
        enabled: true,
      }),
    ).toEqual({
      capabilities: [
        "Active alert listing",
        "Alertmanager webhook ingest",
        "Silenced/inhibited alerts ignored",
      ],
      detail:
        "Alertmanager reads active alerts with silenced, inhibited, and unprocessed alerts filtered out.",
      label: "Source ready for workflows.",
      status: "ready",
    });
  });

  it("summarizes AI diagnosis roles for persisted alert sources", () => {
    expect(
      alertSourceAIRoleReadiness(
        alertSourceProfile({ id: 7, kind: "alertmanager" }),
      ),
    ).toEqual({
      detail:
        "Ready for Alertmanager webhook intake and active-alert evidence in automatic AI diagnosis workflows.",
      label: "Alert intake ready.",
      role: "alert_intake",
      status: "ready",
    });
    expect(
      alertSourceAIRoleReadiness(
        alertSourceProfile({
          enabled: false,
          kind: "alertmanager",
        }),
      ),
    ).toMatchObject({
      label: "Alert intake disabled.",
      role: "alert_intake",
      status: "blocked",
    });
    expect(
      alertSourceAIRoleReadiness(
        alertSourceProfile({
          kind: "prometheus",
          labels: { source: "thanos-rule" },
        }),
      ),
    ).toEqual({
      detail:
        "Thanos Rule source is ready for active-alert evidence from /api/v1/alerts. Use Thanos Query for metric evidence and Alertmanager for webhook intake.",
      label: "Active alerts ready.",
      role: "alert_intake",
      status: "ready",
    });
    expect(
      alertSourceAIRoleReadiness(
        alertSourceProfile({
          kind: "prometheus",
          labels: { source: "thanos" },
        }),
      ),
    ).toEqual({
      detail:
        "Thanos Query source is ready for instant and range metric evidence tools.",
      label: "Metric evidence ready.",
      role: "metric_evidence",
      status: "ready",
    });
    expect(
      alertSourceAIRoleReadiness(
        alertSourceProfile({
          enabled: false,
          kind: "prometheus",
        }),
      ),
    ).toMatchObject({
      label: "Metric evidence disabled.",
      role: "metric_evidence",
      status: "blocked",
    });
  });

  it("derives the Prometheus operator binding checklist without persisting workflow state", () => {
    expect(
      alertSourceOperatorChecklist({
        ...emptyAlertSourceForm(),
        name: "Thanos Query",
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query",
        enabled: true,
      }),
    ).toEqual([
      {
        key: "source",
        label: "Source profile",
        detail: "Enabled Prometheus-compatible profile can be saved.",
        status: "ready",
      },
      {
        key: "alert-test",
        label: "Alert API",
        detail:
          "Run Test to confirm active alerts and the vector(1) metric probe both succeed.",
        status: "pending",
      },
      {
        key: "metric-tools",
        label: "Diagnosis tools",
        detail:
          "Use this source for active_alerts, metric_query, and metric_range evidence templates.",
        status: "pending",
      },
      {
        key: "workflow",
        label: "Workflow binding",
        detail:
          "Bind this source to report replay and diagnosis evidence workflows.",
        status: "pending",
      },
    ]);
  });

  it("derives the Thanos Rule active-alert checklist without metric or webhook steps", () => {
    expect(
      alertSourceOperatorChecklist({
        ...emptyAlertSourceForm(),
        name: "Thanos Rule active alerts",
        kind: "prometheus",
        baseURL: "https://thanos-rule.example.test/alerts",
        labelsText: "role=alert-intake\nsource=thanos-rule",
        enabled: true,
      }),
    ).toEqual([
      {
        key: "source",
        label: "Source profile",
        detail: "Enabled Thanos Rule active-alert profile can be saved.",
        status: "ready",
      },
      {
        key: "alert-test",
        label: "Active alert pull",
        detail:
          "Run Test to confirm Thanos Rule /api/v1/alerts is reachable.",
        status: "pending",
      },
      {
        key: "active-alert-tool",
        label: "Diagnosis tools",
        detail:
          "Use this source for active_alerts evidence templates. Use Thanos Query for metric_query and metric_range evidence.",
        status: "pending",
      },
      {
        key: "workflow",
        label: "Workflow binding",
        detail:
          "Use Alertmanager as the webhook source for automatic diagnosis rooms; bind Thanos Rule as supplemental active-alert evidence.",
        status: "pending",
      },
    ]);
  });

  it("blocks Alertmanager workflow binding until an enabled source can be saved", () => {
    expect(
      alertSourceOperatorChecklist({
        ...emptyAlertSourceForm(),
        kind: "alertmanager",
        name: "Operations Alertmanager",
        baseURL: "https://alertmanager.example.test/api/v2/alerts",
        enabled: false,
      }),
    ).toEqual([
      {
        key: "source",
        label: "Source profile",
        detail: "Save an enabled Alertmanager profile first.",
        status: "pending",
      },
      {
        key: "pull-test",
        label: "Active alert pull",
        detail:
          "Run Test to confirm active=true with silenced, inhibited, and unprocessed alerts excluded.",
        status: "blocked",
      },
      {
        key: "webhook",
        label: "Webhook intake",
        detail: "Webhook endpoint appears after save.",
        status: "blocked",
      },
      {
        key: "workflow",
        label: "Workflow binding",
        detail:
          "Bind this source to grouping, report replay, and diagnosis evidence workflows.",
        status: "blocked",
      },
    ]);
  });

  it("describes Alertmanager integration setup from the source form", () => {
    expect(
      alertSourceProviderGuidance({
        ...emptyAlertSourceForm(),
        kind: "alertmanager",
      }),
    ).toEqual({
      detail:
        "Use Alertmanager when alerts should trigger automatic AI diagnosis rooms through webhook delivery. OpenClarion also tests the active alerts API with silenced, inhibited, and unprocessed alerts excluded.",
      items: [
        {
          detail:
            "Paste the Alertmanager route prefix, the UI alerts page, or /api/v2/alerts. OpenClarion stores the route prefix and tests the active alerts API.",
          key: "base-url",
          label: "Base URL",
          value: "Alertmanager route prefix",
        },
        {
          detail:
            "The persisted source row exposes the OpenClarion webhook receiver URL and scoped Alertmanager route YAML.",
          key: "webhook",
          label: "Webhook intake",
          value: "Receiver after save",
        },
        {
          detail:
            "Bind active_alerts, metric evidence, Enterprise WeChat delivery, and an auto-room workflow before rollout.",
          key: "workflow",
          label: "Workflow",
          value: "Automatic diagnosis trigger",
        },
      ],
      label: "Alertmanager integration",
    });
  });

  it("describes Thanos Rule as supplemental active-alert evidence", () => {
    expect(
      alertSourceProviderGuidance({
        ...emptyAlertSourceForm(),
        kind: "prometheus",
        labelsText: "role=alert-intake\nsource=thanos-rule",
      }),
    ).toEqual({
      detail:
        "Use Thanos Rule as supplemental active-alert evidence. Route webhook-triggered automatic rooms through Alertmanager and use Thanos Query for metric evidence.",
      items: [
        {
          detail:
            "Paste the Thanos Rule route prefix, /alerts, or /api/v1/alerts. OpenClarion stores the route prefix and only tests active alerts.",
          key: "base-url",
          label: "Base URL",
          value: "Thanos Rule alerts API",
        },
        {
          detail:
            "Keep source=thanos-rule so metric probes are skipped and the source is treated as active-alert evidence only.",
          key: "labels",
          label: "Labels",
          value: "source=thanos-rule",
        },
        {
          detail:
            "Create an active_alerts evidence template for this source; do not use it as the metric confidence source.",
          key: "workflow",
          label: "Workflow",
          value: "Supplemental active alerts",
        },
      ],
      label: "Thanos Rule integration",
    });
  });

  it("describes Prometheus-compatible sources as confidence evidence", () => {
    expect(
      alertSourceProviderGuidance({
        ...emptyAlertSourceForm(),
        kind: "prometheus",
        labelsText: "role=metric-evidence\nsource=thanos",
      }),
    ).toEqual({
      detail:
        "Use Prometheus-compatible sources, including Thanos Query, for metric evidence that raises diagnosis confidence after the initial alert report.",
      items: [
        {
          detail:
            "Paste the route prefix, graph page, /api/v1/query, or /api/v1/query_range. OpenClarion stores the route prefix and tests alerts plus vector(1).",
          key: "base-url",
          label: "Base URL",
          value: "Prometheus-compatible query API",
        },
        {
          detail:
            "Use source=thanos for Thanos Query or source=prometheus for a direct Prometheus server.",
          key: "labels",
          label: "Labels",
          value: "Metric evidence source",
        },
        {
          detail:
            "Create metric_query or metric_range_query evidence templates so AI diagnosis can request follow-up metrics before finalizing.",
          key: "workflow",
          label: "Workflow",
          value: "Confidence-building evidence",
        },
      ],
      label: "Prometheus-compatible integration",
    });
  });

  it("previews the provider connection target without storing provider calls as form state", () => {
    expect(
      alertSourceCanonicalBaseURL({
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query/api/v1/query",
      }),
    ).toEqual({
      ok: true,
      value: "https://thanos.example.test/query",
    });
    expect(
      alertSourceCanonicalBaseURL({
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query/",
      }),
    ).toEqual({
      ok: true,
      value: "https://thanos.example.test/query",
    });
    expect(
      alertSourceCanonicalBaseURL({
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/alerts",
      }),
    ).toEqual({
      ok: true,
      value: "https://alertmanager.example.test",
    });
    expect(
      alertSourceCanonicalBaseURL({
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/alertmanager/",
      }),
    ).toEqual({
      ok: true,
      value: "https://alertmanager.example.test/alertmanager",
    });

    expect(
      alertSourceConnectionTargets({
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query/",
      }),
    ).toEqual({
      ok: true,
      value: [
        {
          label: "Active alerts",
          value: "https://thanos.example.test/query/api/v1/alerts",
        },
        {
          label: "Metric probe",
          value:
            "https://thanos.example.test/query/api/v1/query?query=vector%281%29&limit=1",
        },
      ],
    });
    expect(
      alertSourceConnectionTarget({
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query/",
      }),
    ).toEqual({
      ok: true,
      value: "https://thanos.example.test/query/api/v1/alerts",
    });
    expect(
      alertSourceConnectionTarget({
        kind: "prometheus",
        baseURL: "https://thanos.example.test/api/v1/query",
      }),
    ).toEqual({
      ok: true,
      value: "https://thanos.example.test/api/v1/alerts",
    });
    expect(
      alertSourceConnectionTarget({
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query/api/v1/query_range",
      }),
    ).toEqual({
      ok: true,
      value: "https://thanos.example.test/query/api/v1/alerts",
    });
    expect(
      alertSourceConnectionTarget({
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test",
      }),
    ).toEqual({
      ok: true,
      value:
        "https://alertmanager.example.test/api/v2/alerts?active=true&inhibited=false&silenced=false&unprocessed=false",
    });
    expect(
      alertSourceConnectionTarget({
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/api/v2",
      }),
    ).toEqual({
      ok: true,
      value:
        "https://alertmanager.example.test/api/v2/alerts?active=true&inhibited=false&silenced=false&unprocessed=false",
    });
    expect(
      alertSourceConnectionTarget({
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/api/v2/alerts",
      }),
    ).toEqual({
      ok: true,
      value:
        "https://alertmanager.example.test/api/v2/alerts?active=true&inhibited=false&silenced=false&unprocessed=false",
    });
  });

  it("builds copyable Alertmanager webhook endpoints from an optional public API base URL", () => {
    expect(
      alertmanagerWebhookPublicBaseURL(
        "https://public-openclarion.example.test",
        "https://console.example.test",
      ),
    ).toBe("https://public-openclarion.example.test");
    expect(
      alertmanagerWebhookPublicBaseURL(undefined, "https://console.example.test"),
    ).toBe("https://console.example.test");
    expect(alertmanagerWebhookPublicBaseURL(undefined, null)).toBe("");

    expect(alertmanagerWebhookEndpoint(7)).toBe(
      "/api/v1/alert-sources/7/webhooks/alertmanager",
    );
    expect(
      alertmanagerWebhookEndpoint(7, "https://openclarion.example.test"),
    ).toBe(
      "https://openclarion.example.test/api/v1/alert-sources/7/webhooks/alertmanager",
    );
    expect(
      alertmanagerWebhookEndpoint(
        7,
        "https://openclarion.example.test/control-plane/",
      ),
    ).toBe(
      "https://openclarion.example.test/control-plane/api/v1/alert-sources/7/webhooks/alertmanager",
    );
    expect(
      alertmanagerWebhookEndpoint(7, "https://user@openclarion.example.test"),
    ).toBe("/api/v1/alert-sources/7/webhooks/alertmanager");
    expect(
      alertmanagerWebhookEndpoint(
        7,
        "https://openclarion.example.test?token=secret",
      ),
    ).toBe("/api/v1/alert-sources/7/webhooks/alertmanager");
    expect(
      alertmanagerWebhookEndpoint(7, "ftp://openclarion.example.test"),
    ).toBe("/api/v1/alert-sources/7/webhooks/alertmanager");
  });

  it("builds Alertmanager webhook delivery instructions without secret values", () => {
    expect(
      alertmanagerWebhookDeliveryConfig(
        alertSourceProfile({
          auth_mode: "bearer",
          id: 7,
          kind: "alertmanager",
          labels: {
            env: "prod",
            owner: "platform",
            role: "alert-intake",
            source: "alertmanager",
          },
          secret_ref: "secret/openclarion/alertmanager-webhook",
        }),
        "https://openclarion.example.test",
      ),
    ).toEqual({
      authorization:
        "Authorization: Bearer token resolved from secret/openclarion/alertmanager-webhook",
      contentType: "application/json",
      detail:
        "Configure Alertmanager, or another Alertmanager webhook-compatible sender, to POST webhook v4 JSON to this endpoint. Thanos Rule alerts should normally route through Alertmanager first. Resolved, silenced, inhibited, and muted alerts are ignored during ingest.",
      endpoint:
        "https://openclarion.example.test/api/v1/alert-sources/7/webhooks/alertmanager",
      endpointGuidance:
        "This receiver URL is absolute and can be copied into an external Alertmanager route.",
      endpointScope: "absolute",
      method: "POST",
      receiverName: "openclarion-source-7",
      receiverYAML: [
        "receivers:",
        '  - name: "openclarion-source-7"',
        "    webhook_configs:",
        '      - url: "https://openclarion.example.test/api/v1/alert-sources/7/webhooks/alertmanager"',
        "        send_resolved: false",
        "        http_config:",
        "          authorization:",
        "            type: Bearer",
        '            credentials: "<token from secret/openclarion/alertmanager-webhook>"',
      ].join("\n"),
      routeGuidance:
        "Merge this child route under the existing Alertmanager route.routes list, then adjust matchers to the alert labels OpenClarion should diagnose.",
      routeYAML: [
        "route:",
        "  routes:",
        '    - receiver: "openclarion-source-7"',
        "      matchers:",
        '        - env="prod"',
        '        - owner="platform"',
        "      continue: true",
      ].join("\n"),
      routingChecklist: [
        {
          detail:
            "Copy the receiver YAML into Alertmanager receivers as openclarion-source-7.",
          key: "receiver",
          label: "Add receiver",
        },
        {
          detail:
            "Add a scoped Alertmanager route that selects alerts OpenClarion should diagnose and sends them to openclarion-source-7. Use continue: true only when existing downstream receivers should also run.",
          key: "route",
          label: "Bind route",
        },
        {
          detail:
            "Reload Alertmanager, then run Test in OpenClarion and send a bounded synthetic alert to confirm webhook delivery.",
          key: "reload-test",
          label: "Reload and test",
        },
      ],
    });

    const noAuthConfig = alertmanagerWebhookDeliveryConfig(
      alertSourceProfile({ auth_mode: "none", id: 12, kind: "alertmanager" }),
    );
    expect(noAuthConfig?.authorization).toBe("No Authorization header");
    expect(noAuthConfig?.endpointGuidance).toBe(
      "Set NEXT_PUBLIC_OPENCLARION_API_PUBLIC_BASE_URL to an externally reachable OpenClarion URL before copying this receiver to an external Alertmanager.",
    );
    expect(noAuthConfig?.endpointScope).toBe("relative");
    expect(noAuthConfig?.receiverYAML).toBe(
      [
        "receivers:",
        '  - name: "openclarion-source-12"',
        "    webhook_configs:",
        '      - url: "/api/v1/alert-sources/12/webhooks/alertmanager"',
        "        send_resolved: false",
      ].join("\n"),
    );
    expect(noAuthConfig?.routeYAML).toBe(
      [
        "route:",
        "  routes:",
        '    - receiver: "openclarion-source-12"',
        "      matchers:",
        '        - severity=~"warning|critical"',
        "      continue: true",
      ].join("\n"),
    );
    expect(
      alertmanagerWebhookDeliveryConfig(
        alertSourceProfile({ kind: "prometheus" }),
      ),
    ).toBeNull();
  });

  it("normalizes pasted Prometheus API endpoints before storing profile metadata", () => {
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Thanos Query",
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query/api/v1/query_range",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Thanos Query",
        kind: "prometheus",
        base_url: "https://thanos.example.test/query",
        auth_mode: "none",
        enabled: true,
        labels: {},
      },
    });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Thanos Query UI",
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query/graph",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Thanos Query UI",
        kind: "prometheus",
        base_url: "https://thanos.example.test/query",
        auth_mode: "none",
        enabled: true,
        labels: {},
      },
    });
    expect(
      alertSourceConnectionTarget({
        kind: "prometheus",
        baseURL: "https://thanos.example.test/query/alerts",
      }),
    ).toEqual({
      ok: true,
      value: "https://thanos.example.test/query/api/v1/alerts",
    });
    expect(
      alertSourceConnectionTargets({
        kind: "prometheus",
        baseURL: "https://thanos-rule.example.test/alerts",
        labelsText: "role=alert-intake\nsource=thanos-rule",
      }),
    ).toEqual({
      ok: true,
      value: [
        {
          label: "Active alerts",
          value: "https://thanos-rule.example.test/api/v1/alerts",
        },
      ],
    });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Thanos Rule active alerts",
        kind: "prometheus",
        baseURL: "https://thanos-rule.example.test/alerts",
        labelsText: "role=alert-intake\nsource=thanos-rule",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Thanos Rule active alerts",
        kind: "prometheus",
        base_url: "https://thanos-rule.example.test",
        auth_mode: "none",
        enabled: true,
        labels: { role: "alert-intake", source: "thanos-rule" },
      },
    });
  });

  it("prompts operators to classify likely Thanos Rule alert URLs", () => {
    expect(
      alertSourceClassificationHint({
        ...emptyAlertSourceForm(),
        kind: "prometheus",
        baseURL: "https://thanos-rule.example.test/alerts",
      }),
    ).toEqual({
      detail:
        "This looks like a rule-service active-alert URL. If it is Thanos Rule, use the Thanos Rule preset or add source=thanos-rule so OpenClarion skips metric probes and uses this source only for active-alert evidence.",
      label: "Review Thanos Rule classification.",
      suggestedLabelsText: "role=alert-intake\nsource=thanos-rule",
    });
    expect(
      alertSourceClassificationHint({
        ...emptyAlertSourceForm(),
        kind: "prometheus",
        baseURL: "https://thanos.example.test/rule/api/v1/alerts",
      })?.label,
    ).toBe("Review Thanos Rule classification.");
    expect(
      alertSourceClassificationHint({
        ...emptyAlertSourceForm(),
        kind: "prometheus",
        baseURL: "https://thanos-rule.example.test/alerts",
        labelsText: "role=alert-intake\nsource=thanos-rule",
      }),
    ).toBeNull();
    expect(
      alertSourceClassificationHint({
        ...emptyAlertSourceForm(),
        kind: "prometheus",
        baseURL: "https://prometheus.example.test/alerts",
      }),
    ).toBeNull();
    expect(
      alertSourceClassificationHint({
        ...emptyAlertSourceForm(),
        kind: "alertmanager",
        baseURL: "https://alertmanager-rule.example.test/alerts",
      }),
    ).toBeNull();
  });

  it("normalizes pasted Alertmanager API endpoints before storing profile metadata", () => {
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Operations Alertmanager",
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/api/v2/alerts",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Operations Alertmanager",
        kind: "alertmanager",
        base_url: "https://alertmanager.example.test",
        auth_mode: "none",
        enabled: true,
        labels: {},
      },
    });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Alertmanager UI",
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/alerts",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Alertmanager UI",
        kind: "alertmanager",
        base_url: "https://alertmanager.example.test",
        auth_mode: "none",
        enabled: true,
        labels: {},
      },
    });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Route-prefixed Alertmanager",
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/alertmanager/alerts",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Route-prefixed Alertmanager",
        kind: "alertmanager",
        base_url: "https://alertmanager.example.test/alertmanager",
        auth_mode: "none",
        enabled: true,
        labels: {},
      },
    });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Route-prefixed Alertmanager API",
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/alertmanager/api/v2",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Route-prefixed Alertmanager API",
        kind: "alertmanager",
        base_url: "https://alertmanager.example.test/alertmanager",
        auth_mode: "none",
        enabled: true,
        labels: {},
      },
    });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Route-prefixed Alertmanager slash",
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/alertmanager/",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Route-prefixed Alertmanager slash",
        kind: "alertmanager",
        base_url: "https://alertmanager.example.test/alertmanager",
        auth_mode: "none",
        enabled: true,
        labels: {},
      },
    });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Alertmanager groups API",
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/api/v2/alerts/groups",
        enabled: true,
      }),
    ).toEqual({
      ok: true,
      value: {
        name: "Alertmanager groups API",
        kind: "alertmanager",
        base_url: "https://alertmanager.example.test",
        auth_mode: "none",
        enabled: true,
        labels: {},
      },
    });
  });

  it("enforces auth and URL boundaries before submit", () => {
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Bad URL",
        baseURL: "https://user@example.test",
      }).ok,
    ).toBe(false);
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Bearer without secret",
        baseURL: "https://prometheus.example.test",
        authMode: "bearer",
      }).ok,
    ).toBe(false);
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "None with secret",
        baseURL: "https://prometheus.example.test",
        secretRef: "secret/openclarion/prometheus-bearer",
      }).ok,
    ).toBe(false);
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Long URL",
        baseURL: `https://prometheus.example.test/${"a".repeat(2048)}`,
      }),
    ).toEqual({ ok: false, message: "Base URL must be 2048 bytes or fewer." });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        name: "Long secret",
        baseURL: "https://prometheus.example.test",
        authMode: "bearer",
        secretRef: `secret/${"a".repeat(257)}`,
      }),
    ).toEqual({
      ok: false,
      message: "Secret reference must be 256 bytes or fewer.",
    });
    expect(
      formStateToWriteRequest({
        ...emptyAlertSourceForm(),
        baseURL: "https://prometheus.example.test",
        name: "é".repeat(61),
      }),
    ).toEqual({
      ok: false,
      message: "Profile name must be 120 bytes or fewer.",
    });
    expect(
      alertSourceConnectionTarget({
        kind: "alertmanager",
        baseURL: "https://alertmanager.example.test/api/v2/alerts?active=true",
      }),
    ).toEqual({
      ok: false,
      message: "Base URL must not include query or fragment.",
    });
  });

  it("enforces label domain boundaries before submit", () => {
    expect(
      parseLabelsText(
        Array.from({ length: 33 }, (_, index) => `k${index}=v`).join("\n"),
      ),
    ).toEqual({
      ok: false,
      message: "Labels must contain 32 entries or fewer.",
    });
    expect(parseLabelsText(`${"k".repeat(65)}=v`)).toEqual({
      ok: false,
      message: "Label line 1 exceeds the allowed length.",
    });
    expect(parseLabelsText(`team=ops\u0001`)).toEqual({
      ok: false,
      message: "Labels must not contain control characters.",
    });
  });
});

function alertSourceProfile(
  overrides: Partial<AlertSourceProfile> = {},
): AlertSourceProfile {
  return {
    auth_mode: "none",
    base_url: "https://source.example.test",
    created_at: "2026-06-21T00:00:00Z",
    enabled: true,
    id: 1,
    kind: "prometheus",
    labels: {},
    name: "Source",
    secret_ref: "",
    updated_at: "2026-06-21T00:00:00Z",
    ...overrides,
  };
}
