import { describe, expect, it } from "vitest";

import {
  alertSourceAutomationSetupReadiness,
  alertSourceNextSetupActions,
} from "./next-actions";
import type {
  AlertSourceConnectionTestResult,
  AlertSourceProfile,
} from "./types";

describe("alert source next setup actions", () => {
  it("routes Alertmanager sources to alert evidence and automatic diagnosis workflow setup", () => {
    expect(
      alertSourceNextSetupActions(alertSourceProfile({ id: 7, kind: "alertmanager" })),
    ).toEqual([
      {
        detail:
          "Create an active_alerts evidence template bound to this Alertmanager source.",
        href: "/settings/diagnosis-tool-templates?intent=active-alert-tool&source_id=7",
        key: "active-alert-tool",
        label: "Alert Tool",
      },
      {
        detail:
          "Create or enable a Prometheus-compatible metric evidence source, usually Thanos Query, for AI confidence-building queries.",
        href: "/settings/alert-sources?intent=thanos-source",
        key: "metric-evidence-source",
        label: "Metric Source",
      },
      {
        detail:
          "Create or update an automatic diagnosis workflow that uses this Alertmanager webhook source.",
        href: "/settings/report-workflow-policies?intent=auto-room-follow-up&source_id=7",
        key: "auto-room-workflow",
        label: "Auto Workflow",
      },
      {
        detail:
          "Create a WeCom report and AI-room channel for report, diagnosis consultation, and close notifications.",
        href: "/settings/notification-channels?intent=report-close-channel&workflow_return=auto-room-enable&workflow_source_id=7",
        key: "notification-channel",
        label: "AI Channel",
      },
    ]);
  });

  it("routes existing disabled Alertmanager workflows to enablement review", () => {
    const action = alertSourceNextSetupActions(
      alertSourceProfile({ id: 7, kind: "alertmanager" }),
      {
        bindings: {
          ...emptyKnownBindings(),
          autoRoomPolicyCount: 1,
        },
      },
    ).find((candidate) => candidate.key === "auto-room-workflow");

    expect(action).toEqual({
      detail:
        "Enable or review the existing automatic diagnosis workflow that uses this Alertmanager webhook source.",
      href: "/settings/report-workflow-policies?intent=enable-ai-room-follow-up&source_id=7",
      key: "auto-room-workflow",
      label: "Enable Workflow",
    });
  });

  it("routes existing enabled Alertmanager workflows to review candidates", () => {
    const action = alertSourceNextSetupActions(
      alertSourceProfile({ id: 7, kind: "alertmanager" }),
      {
        bindings: {
          ...emptyKnownBindings(),
          autoRoomPolicyCount: 1,
          enabledAutoRoomPolicyCount: 1,
        },
      },
    ).find((candidate) => candidate.key === "auto-room-workflow");

    expect(action).toEqual({
      detail:
        "Review the enabled automatic diagnosis workflow that uses this Alertmanager webhook source.",
      href: "/settings/report-workflow-policies?intent=enable-ai-room-follow-up&source_id=7",
      key: "auto-room-workflow",
      label: "Review Workflow",
    });
  });

  it("labels Thanos Rule active-alert next actions explicitly", () => {
    expect(
      alertSourceNextSetupActions(
        alertSourceProfile({
          id: 9,
          kind: "prometheus",
          labels: { source: "thanos-rule" },
        }),
      ),
    ).toEqual([
      {
        detail:
          "Create an active_alerts evidence template bound to this Thanos Rule source.",
        href: "/settings/diagnosis-tool-templates?intent=active-alert-tool&source_id=9",
        key: "active-alert-tool",
        label: "Alert Tool",
      },
    ]);
  });

  it("routes Prometheus-compatible sources to metric and active alert evidence templates", () => {
    expect(
      alertSourceNextSetupActions(alertSourceProfile({ id: 11, kind: "prometheus" })),
    ).toEqual([
      {
        detail:
          "Create a metric evidence template bound to this Prometheus-compatible source.",
        href: "/settings/diagnosis-tool-templates?intent=metric-evidence-tool&source_id=11",
        key: "metric-evidence-tool",
        label: "Metric Tool",
      },
      {
        detail:
          "Create an active_alerts evidence template bound to this Prometheus-compatible source.",
        href: "/settings/diagnosis-tool-templates?intent=active-alert-tool&source_id=11",
        key: "active-alert-tool",
        label: "Alert Tool",
      },
    ]);
  });

  it("carries workflow return context into source and tool setup actions", () => {
    expect(
      alertSourceNextSetupActions(
        alertSourceProfile({ id: 11, kind: "prometheus" }),
        { workflowReturn: { sourceID: 7 } },
      ).map((action) => [action.key, action.href]),
    ).toEqual([
      [
        "metric-evidence-tool",
        "/settings/diagnosis-tool-templates?intent=metric-evidence-tool&source_id=11&workflow_return=auto-room-enable&workflow_source_id=7",
      ],
      [
        "active-alert-tool",
        "/settings/diagnosis-tool-templates?intent=active-alert-tool&source_id=11&workflow_return=auto-room-enable&workflow_source_id=7",
      ],
    ]);
  });

  it("blocks setup for disabled alert sources before workflow configuration", () => {
    expect(
      alertSourceAutomationSetupReadiness(
        alertSourceProfile({
          enabled: false,
          kind: "prometheus",
          labels: { source: "thanos-rule" },
        }),
        undefined,
        emptyKnownBindings(),
      ),
    ).toEqual({
      detail:
        "Enable this Thanos Rule source, save it, then run Test before configuring AI diagnosis setup.",
      label: "Source disabled",
      status: "blocked",
      steps: [
        {
          detail: "Thanos Rule source must be enabled before workflow setup.",
          key: "source",
          label: "Enabled source",
          status: "blocked",
        },
        {
          detail: "Run Test to verify provider reachability and credentials.",
          key: "connection-test",
          label: "Connection test",
          status: "pending",
        },
        {
          detail: "Create an active_alerts evidence template for this source.",
          key: "active-alert-tool",
          label: "Alert tool",
          status: "blocked",
        },
      ],
    });
  });

  it("keeps setup blocked when the latest connection test failed", () => {
    expect(
      alertSourceAutomationSetupReadiness(
        alertSourceProfile({ kind: "alertmanager" }),
        connectionTestResult({ status: "failed" }),
        emptyKnownBindings(),
      ),
    ).toEqual({
      detail:
        "Last connection test ended with failed. Resolve the provider or credential issue before continuing setup.",
      label: "Connection test blocked",
      status: "blocked",
      steps: [
        {
          detail: "Alertmanager source is saved and enabled.",
          key: "source",
          label: "Enabled source",
          status: "ready",
        },
        {
          detail: "Last connection test did not pass.",
          key: "connection-test",
          label: "Connection test",
          status: "blocked",
        },
        {
          detail:
            "Copy the receiver YAML from the Ingest column, bind it to a scoped Alertmanager route, and reload Alertmanager.",
          key: "receiver-config",
          label: "Receiver route",
          status: "pending",
        },
        {
          detail:
            "Webhook delivery proof requires an enabled source and a successful provider connection test first.",
          key: "webhook-delivery-proof",
          label: "Webhook proof",
          status: "blocked",
        },
        {
          detail:
            "Configure a WeCom channel with report, diagnosis_consultation, and diagnosis_close scopes; workflow setup verifies delivery proof.",
          key: "notification-channel",
          label: "AI channel",
          status: "blocked",
        },
        {
          detail: "Create an active_alerts evidence template for this source.",
          key: "active-alert-tool",
          label: "Alert tool",
          status: "blocked",
        },
        {
          detail:
            "Create a Prometheus-compatible metric evidence source, usually Thanos Query, and add metric_query or metric_range_query templates.",
          key: "metric-evidence",
          label: "Metric evidence",
          status: "blocked",
        },
        {
          detail:
            "Create or update an automatic diagnosis workflow after the alert tool is ready.",
          key: "auto-room-workflow",
          label: "Auto workflow",
          status: "blocked",
        },
      ],
    });
  });

  it("shows Alertmanager webhook setup gaps after a successful test", () => {
    expect(
      alertSourceAutomationSetupReadiness(
        alertSourceProfile({ id: 7, kind: "alertmanager" }),
        connectionTestResult(),
        emptyKnownBindings(),
      ),
    ).toEqual({
      detail:
        "Provider test passed. Copy the receiver config, create active_alerts plus metric evidence templates, then bind an automatic diagnosis workflow.",
      label: "Webhook setup ready",
      status: "pending",
      steps: [
        {
          detail: "Alertmanager source is saved and enabled.",
          key: "source",
          label: "Enabled source",
          status: "ready",
        },
        {
          detail: "Last connection test passed.",
          key: "connection-test",
          label: "Connection test",
          status: "ready",
        },
        {
          detail:
            "Copy the receiver YAML from the Ingest column, bind it to a scoped Alertmanager route, and reload Alertmanager.",
          key: "receiver-config",
          label: "Receiver route",
          status: "pending",
        },
        {
          detail:
            "Send a bounded synthetic firing alert or wait for a controlled route match, then confirm OpenClarion ingested the webhook and started the expected automatic diagnosis room.",
          key: "webhook-delivery-proof",
          label: "Webhook proof",
          status: "pending",
        },
        {
          detail:
            "Configure a WeCom channel with report, diagnosis_consultation, and diagnosis_close scopes; workflow setup verifies delivery proof.",
          key: "notification-channel",
          label: "AI channel",
          status: "pending",
        },
        {
          detail: "Create an active_alerts evidence template for this source.",
          key: "active-alert-tool",
          label: "Alert tool",
          status: "pending",
        },
        {
          detail:
            "Create a Prometheus-compatible metric evidence source, usually Thanos Query, and add metric_query or metric_range_query templates.",
          key: "metric-evidence",
          label: "Metric evidence",
          status: "pending",
        },
        {
          detail:
            "Create or update an automatic diagnosis workflow after the alert tool is ready.",
          key: "auto-room-workflow",
          label: "Auto workflow",
          status: "pending",
        },
      ],
    });
  });

  it("shows Prometheus evidence template gaps after a successful test", () => {
    expect(
      alertSourceAutomationSetupReadiness(
        alertSourceProfile({ id: 11, kind: "prometheus" }),
        connectionTestResult(),
        emptyKnownBindings(),
      ),
    ).toMatchObject({
      detail:
        "Provider test passed. Create metric and active_alerts templates so diagnosis rooms can collect evidence from this source.",
      label: "Evidence setup ready",
      status: "pending",
      steps: [
        { key: "source", status: "ready" },
        { key: "connection-test", status: "ready" },
        { key: "metric-evidence-tool", status: "pending" },
        { key: "active-alert-tool", status: "pending" },
      ],
    });
  });

  it("shows Thanos Rule active-alert template gaps after a successful test", () => {
    expect(
      alertSourceAutomationSetupReadiness(
        alertSourceProfile({
          id: 9,
          kind: "prometheus",
          labels: { source: "thanos-rule" },
        }),
        connectionTestResult(),
        emptyKnownBindings(),
      ),
    ).toMatchObject({
      detail:
        "Provider test passed. Create an active_alerts template for this Thanos Rule source; use Thanos Query for metric evidence and Alertmanager for webhook-triggered automatic rooms.",
      label: "Active alert evidence ready",
      status: "pending",
      steps: [
        { key: "source", status: "ready" },
        { key: "connection-test", status: "ready" },
        { key: "active-alert-tool", status: "pending" },
      ],
    });
  });

  it("marks internal Alertmanager automation bindings ready when templates and workflows exist", () => {
    expect(
      alertSourceAutomationSetupReadiness(
        alertSourceProfile({ id: 7, kind: "alertmanager" }),
        connectionTestResult(),
        {
          ...emptyKnownBindings(),
          activeAlertTemplateCount: 1,
          autoRoomPolicyCount: 1,
          enabledActiveAlertTemplateCount: 1,
          enabledAutoRoomPolicyCount: 1,
          enabledWorkflowMetricTemplateCount: 1,
          metricEvidenceSourceCount: 1,
        },
      ),
    ).toEqual({
      detail:
        "Provider test passed and internal AI diagnosis bindings include active alerts, metric evidence, and an automatic workflow. Apply the Alertmanager receiver route, then retain webhook delivery proof before rollout.",
      label: "Webhook proof needed",
      status: "pending",
      steps: [
        {
          detail: "Alertmanager source is saved and enabled.",
          key: "source",
          label: "Enabled source",
          status: "ready",
        },
        {
          detail: "Last connection test passed.",
          key: "connection-test",
          label: "Connection test",
          status: "ready",
        },
        {
          detail:
            "Copy the receiver YAML from the Ingest column, bind it to a scoped Alertmanager route, and reload Alertmanager.",
          key: "receiver-config",
          label: "Receiver route",
          status: "pending",
        },
        {
          detail:
            "Send a bounded synthetic firing alert or wait for a controlled route match, then confirm OpenClarion ingested the webhook and started the expected automatic diagnosis room.",
          key: "webhook-delivery-proof",
          label: "Webhook proof",
          status: "pending",
        },
        {
          detail:
            "1 enabled automatic diagnosis workflow(s) are bound to this source; workflow enablement covers the required WeCom scopes and AI delivery proof.",
          key: "notification-channel",
          label: "AI channel",
          status: "ready",
        },
        {
          detail:
            "1 enabled active_alerts template(s) are bound to this source.",
          key: "active-alert-tool",
          label: "Alert tool",
          status: "ready",
        },
        {
          detail:
            "1 enabled metric evidence template(s) are available across Prometheus-compatible sources.",
          key: "metric-evidence",
          label: "Metric evidence",
          status: "ready",
        },
        {
          detail:
            "1 enabled automatic diagnosis workflow(s) are bound to this source.",
          key: "auto-room-workflow",
          label: "Auto workflow",
          status: "ready",
        },
      ],
    });
  });

  it("marks Prometheus evidence setup complete when active and metric templates exist", () => {
    expect(
      alertSourceAutomationSetupReadiness(
        alertSourceProfile({ id: 11, kind: "prometheus" }),
        connectionTestResult(),
        {
          ...emptyKnownBindings(),
          activeAlertTemplateCount: 1,
          enabledActiveAlertTemplateCount: 1,
          enabledRangeMetricTemplateCount: 1,
          rangeMetricTemplateCount: 1,
        },
      ),
    ).toMatchObject({
      detail:
        "Provider test passed and evidence templates exist for active alerts plus metric collection.",
      label: "Evidence setup complete",
      status: "ready",
      steps: [
        { key: "source", status: "ready" },
        { key: "connection-test", status: "ready" },
        { key: "metric-evidence-tool", status: "ready" },
        { key: "active-alert-tool", status: "ready" },
      ],
    });
  });

  it("marks Thanos Rule active-alert evidence complete when the alert tool exists", () => {
    expect(
      alertSourceAutomationSetupReadiness(
        alertSourceProfile({
          id: 9,
          kind: "prometheus",
          labels: { source: "thanos-rule" },
        }),
        connectionTestResult(),
        {
          ...emptyKnownBindings(),
          activeAlertTemplateCount: 1,
          enabledActiveAlertTemplateCount: 1,
        },
      ),
    ).toMatchObject({
      detail:
        "Provider test passed and active_alerts evidence is bound to this Thanos Rule source.",
      label: "Active alert evidence complete",
      status: "ready",
      steps: [
        { key: "source", status: "ready" },
        { key: "connection-test", status: "ready" },
        { key: "active-alert-tool", status: "ready" },
      ],
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

function connectionTestResult(
  overrides: Partial<AlertSourceConnectionTestResult> = {},
): AlertSourceConnectionTestResult {
  return {
    auth_mode: "none",
    checked_at: "2026-06-21T00:00:00Z",
    kind: "prometheus",
    message: "Connection test succeeded.",
    observed_alerts: 1,
    reason_code: "ok",
    source_id: 1,
    status: "success",
    ...overrides,
  };
}

function emptyKnownBindings() {
  return {
    activeAlertTemplateCount: 0,
    autoRoomPolicyCount: 0,
    enabledActiveAlertTemplateCount: 0,
    enabledAutoRoomPolicyCount: 0,
    enabledMetricTemplateCount: 0,
    enabledRangeMetricTemplateCount: 0,
    enabledWorkflowMetricTemplateCount: 0,
    metricEvidenceSourceCount: 0,
    metricTemplateCount: 0,
    rangeMetricTemplateCount: 0,
    toolTemplatesKnown: true,
    workflowPoliciesKnown: true,
  };
}
