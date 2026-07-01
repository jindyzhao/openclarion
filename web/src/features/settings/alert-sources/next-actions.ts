import { diagnosisToolTemplateLaunchHref } from "../diagnosis-tool-templates/format";
import { notificationChannelLaunchHref } from "../notification-channels/format";
import { reportWorkflowPolicyLaunchHref } from "../report-workflow-policies/format";
import {
  alertSourceLaunchHref,
  alertSourceProfileIsThanosRule,
  alertmanagerCompatibleSourceLabel,
} from "./format";
import type {
  AlertSourceConnectionTestResult,
  AlertSourceProfile,
} from "./types";

type AlertSourceSetupStatus = "ready" | "pending" | "blocked";

type AlertSourceSetupChecklistItem = {
  detail: string;
  key: string;
  label: string;
  status: AlertSourceSetupStatus;
};

export type AlertSourceNextSetupAction = {
  detail: string;
  href: string;
  key: string;
  label: string;
};

export type AlertSourceAutomationSetupReadiness = {
  detail: string;
  label: string;
  status: AlertSourceSetupStatus;
  steps: AlertSourceSetupChecklistItem[];
};

export type AlertSourceAutomationBindings = {
  activeAlertTemplateCount: number;
  autoRoomPolicyCount: number;
  enabledActiveAlertTemplateCount: number;
  enabledAutoRoomPolicyCount: number;
  enabledMetricTemplateCount: number;
  enabledRangeMetricTemplateCount: number;
  enabledWorkflowMetricTemplateCount: number;
  metricEvidenceSourceCount: number;
  metricTemplateCount: number;
  rangeMetricTemplateCount: number;
  toolTemplatesKnown: boolean;
  workflowPoliciesKnown: boolean;
};

export type AlertSourceNextSetupActionOptions = {
  bindings?: AlertSourceAutomationBindings;
  workflowReturn?: { sourceID?: number | null } | null;
};

export function alertSourceAutomationSetupReadiness(
  profile: AlertSourceProfile,
  testResult?: AlertSourceConnectionTestResult,
  bindings: AlertSourceAutomationBindings = unknownAlertSourceAutomationBindings(),
): AlertSourceAutomationSetupReadiness {
  const sourceLabel =
    profile.kind === "alertmanager"
      ? alertmanagerCompatibleSourceLabel(profile)
      : alertSourceProfileIsThanosRule(profile)
        ? "Thanos Rule"
      : "Prometheus-compatible";
  const sourceReady = profile.enabled;
  const testReady = testResult?.status === "success";
  const testBlocked =
    testResult !== undefined && testResult.status !== "success";
  const steps = alertSourceAutomationSetupSteps({
    bindings,
    profile,
    sourceLabel,
    sourceReady,
    testBlocked,
    testReady,
  });

  if (!sourceReady) {
    return {
      detail: `Enable this ${sourceLabel} source, save it, then run Test before configuring AI diagnosis setup.`,
      label: "Source disabled",
      status: "blocked",
      steps,
    };
  }

  if (testBlocked) {
    return {
      detail: `Last connection test ended with ${testResult.status}. Resolve the provider or credential issue before continuing setup.`,
      label: "Connection test blocked",
      status: "blocked",
      steps,
    };
  }

  if (!testReady) {
    return {
      detail: `Run Test for this ${sourceLabel} source before relying on AI diagnosis workflow setup.`,
      label: "Connection test required",
      status: "pending",
      steps,
    };
  }

  if (profile.kind === "alertmanager") {
    const activeToolReady = bindings.enabledActiveAlertTemplateCount > 0;
    const autoRoomReady = bindings.enabledAutoRoomPolicyCount > 0;
    const metricEvidenceReady = bindings.enabledWorkflowMetricTemplateCount > 0;
    if (testReady && activeToolReady && autoRoomReady && metricEvidenceReady) {
      return {
        detail:
          "Provider test passed and internal AI diagnosis bindings include active alerts, metric evidence, and an automatic workflow. Apply the Alertmanager receiver route, then retain webhook delivery proof before rollout.",
        label: "Webhook proof needed",
        status: "pending",
        steps,
      };
    }
    return {
      detail:
        "Provider test passed. Copy the receiver config, create active_alerts plus metric evidence templates, then bind an automatic diagnosis workflow.",
      label: "Webhook setup ready",
      status: "pending",
      steps,
    };
  }

  if (alertSourceProfileIsThanosRule(profile)) {
    const activeToolReady = bindings.enabledActiveAlertTemplateCount > 0;
    if (testReady && activeToolReady) {
      return {
        detail:
          "Provider test passed and active_alerts evidence is bound to this Thanos Rule source.",
        label: "Active alert evidence complete",
        status: "ready",
        steps,
      };
    }
    return {
      detail:
        "Provider test passed. Create an active_alerts template for this Thanos Rule source; use Thanos Query for metric evidence and Alertmanager for webhook-triggered automatic rooms.",
      label: "Active alert evidence ready",
      status: "pending",
      steps,
    };
  }

  const metricToolsReady =
    bindings.enabledMetricTemplateCount + bindings.enabledRangeMetricTemplateCount >
    0;
  const activeToolReady = bindings.enabledActiveAlertTemplateCount > 0;
  if (testReady && metricToolsReady && activeToolReady) {
    return {
      detail:
        "Provider test passed and evidence templates exist for active alerts plus metric collection.",
      label: "Evidence setup complete",
      status: "ready",
      steps,
    };
  }
  return {
    detail:
      "Provider test passed. Create metric and active_alerts templates so diagnosis rooms can collect evidence from this source.",
    label: "Evidence setup ready",
    status: "pending",
    steps,
  };
}

export function alertSourceNextSetupActions(
  profile: AlertSourceProfile,
  options: AlertSourceNextSetupActionOptions = {},
): AlertSourceNextSetupAction[] {
  const workflowReturn = options.workflowReturn ?? undefined;
  if (profile.kind === "alertmanager") {
    const sourceLabel = alertmanagerCompatibleSourceLabel(profile);
    return [
      {
        detail: `Create an active_alerts evidence template bound to this ${sourceLabel} source.`,
        href: diagnosisToolTemplateLaunchHref({
          intent: "active-alert-tool",
          sourceID: profile.id,
          workflowReturn,
        }),
        key: "active-alert-tool",
        label: "Alert Tool",
      },
      {
        detail:
          "Create or enable a Prometheus-compatible metric evidence source, usually Thanos Query, for AI confidence-building queries.",
        href: alertSourceLaunchHref({ intent: "thanos-source", workflowReturn }),
        key: "metric-evidence-source",
        label: "Metric Source",
      },
      alertSourceAutoRoomWorkflowAction(profile, sourceLabel, options.bindings),
      {
        detail:
          "Create a WeCom report and AI-room channel for report, diagnosis consultation, and close notifications.",
        href: notificationChannelLaunchHref({
          intent: "report-close-channel",
          workflowReturn: { sourceID: profile.id },
        }),
        key: "notification-channel",
        label: "AI Channel",
      },
    ];
  }

  if (alertSourceProfileIsThanosRule(profile)) {
    return [
      {
        detail:
          "Create an active_alerts evidence template bound to this Thanos Rule source.",
        href: diagnosisToolTemplateLaunchHref({
          intent: "active-alert-tool",
          sourceID: profile.id,
          workflowReturn,
        }),
        key: "active-alert-tool",
        label: "Alert Tool",
      },
    ];
  }

  return [
    {
      detail:
        "Create a metric evidence template bound to this Prometheus-compatible source.",
      href: diagnosisToolTemplateLaunchHref({
        intent: "metric-evidence-tool",
        sourceID: profile.id,
        workflowReturn,
      }),
      key: "metric-evidence-tool",
      label: "Metric Tool",
    },
    {
      detail:
        "Create an active_alerts evidence template bound to this Prometheus-compatible source.",
      href: diagnosisToolTemplateLaunchHref({
        intent: "active-alert-tool",
        sourceID: profile.id,
        workflowReturn,
      }),
      key: "active-alert-tool",
      label: "Alert Tool",
    },
  ];
}

function alertSourceAutoRoomWorkflowAction(
  profile: AlertSourceProfile,
  sourceLabel: string,
  bindings: AlertSourceAutomationBindings | undefined,
): AlertSourceNextSetupAction {
  const existingWorkflowKnown =
    bindings?.workflowPoliciesKnown === true && bindings.autoRoomPolicyCount > 0;
  if (existingWorkflowKnown) {
    const enabledWorkflowExists = bindings.enabledAutoRoomPolicyCount > 0;
    return {
      detail: enabledWorkflowExists
        ? `Review the enabled automatic diagnosis workflow that uses this ${sourceLabel} webhook source.`
        : `Enable or review the existing automatic diagnosis workflow that uses this ${sourceLabel} webhook source.`,
      href: reportWorkflowPolicyLaunchHref({
        intent: "enable-ai-room-follow-up",
        sourceID: profile.id,
      }),
      key: "auto-room-workflow",
      label: enabledWorkflowExists ? "Review Workflow" : "Enable Workflow",
    };
  }

  return {
    detail: `Create or update an automatic diagnosis workflow that uses this ${sourceLabel} webhook source.`,
    href: reportWorkflowPolicyLaunchHref({
      intent: "auto-room-follow-up",
      sourceID: profile.id,
    }),
    key: "auto-room-workflow",
    label: "Auto Workflow",
  };
}

function alertSourceAutomationSetupSteps({
  bindings,
  profile,
  sourceLabel,
  sourceReady,
  testBlocked,
  testReady,
}: {
  bindings: AlertSourceAutomationBindings;
  profile: AlertSourceProfile;
  sourceLabel: string;
  sourceReady: boolean;
  testBlocked: boolean;
  testReady: boolean;
}): AlertSourceSetupChecklistItem[] {
  const steps: AlertSourceSetupChecklistItem[] = [
    {
      detail: sourceReady
        ? `${sourceLabel} source is saved and enabled.`
        : `${sourceLabel} source must be enabled before workflow setup.`,
      key: "source",
      label: "Enabled source",
      status: sourceReady ? "ready" : "blocked",
    },
    {
      detail: testReady
        ? "Last connection test passed."
        : testBlocked
          ? "Last connection test did not pass."
          : "Run Test to verify provider reachability and credentials.",
      key: "connection-test",
      label: "Connection test",
      status: testReady ? "ready" : testBlocked ? "blocked" : "pending",
    },
  ];

  if (profile.kind === "alertmanager") {
    const activeToolStatus = setupStatusFromKnownCount({
      blocked: !testReady,
      count: bindings.enabledActiveAlertTemplateCount,
      known: bindings.toolTemplatesKnown,
    });
    const autoWorkflowStatus = setupStatusFromKnownCount({
      blocked: !testReady,
      count: bindings.enabledAutoRoomPolicyCount,
      known: bindings.workflowPoliciesKnown,
    });
    const notificationChannelStatus = setupStatusFromKnownCount({
      blocked: !testReady,
      count: bindings.enabledAutoRoomPolicyCount,
      known: bindings.workflowPoliciesKnown,
    });
    const metricEvidenceStatus = setupStatusFromKnownCount({
      blocked: !testReady,
      count: bindings.enabledWorkflowMetricTemplateCount,
      known: bindings.toolTemplatesKnown,
    });
    steps.push(
      {
        detail: sourceReady
          ? "Copy the receiver YAML from the Ingest column, bind it to a scoped Alertmanager route, and reload Alertmanager."
          : "Receiver YAML appears after the source is saved.",
        key: "receiver-config",
        label: "Receiver route",
        status: sourceReady ? "pending" : "blocked",
      },
      {
        detail:
          testReady && sourceReady
            ? "Send a bounded synthetic firing alert or wait for a controlled route match, then confirm OpenClarion ingested the webhook and started the expected automatic diagnosis room."
            : "Webhook delivery proof requires an enabled source and a successful provider connection test first.",
        key: "webhook-delivery-proof",
        label: "Webhook proof",
        status: testReady && sourceReady ? "pending" : "blocked",
      },
      {
        detail: notificationChannelStepDetail(bindings),
        key: "notification-channel",
        label: "AI channel",
        status: notificationChannelStatus,
      },
      {
        detail: alertToolStepDetail(bindings),
        key: "active-alert-tool",
        label: "Alert tool",
        status: activeToolStatus,
      },
      {
        detail: workflowMetricEvidenceStepDetail(bindings),
        key: "metric-evidence",
        label: "Metric evidence",
        status: metricEvidenceStatus,
      },
      {
        detail: autoRoomWorkflowStepDetail(bindings),
        key: "auto-room-workflow",
        label: "Auto workflow",
        status: autoWorkflowStatus,
      },
    );
    return steps;
  }

  if (alertSourceProfileIsThanosRule(profile)) {
    const activeToolStatus = setupStatusFromKnownCount({
      blocked: !testReady,
      count: bindings.enabledActiveAlertTemplateCount,
      known: bindings.toolTemplatesKnown,
    });
    steps.push({
      detail: alertToolStepDetail(bindings),
      key: "active-alert-tool",
      label: "Alert tool",
      status: activeToolStatus,
    });
    return steps;
  }

  const metricToolStatus = setupStatusFromKnownCount({
    blocked: !testReady,
    count:
      bindings.enabledMetricTemplateCount +
      bindings.enabledRangeMetricTemplateCount,
    known: bindings.toolTemplatesKnown,
  });
  const activeToolStatus = setupStatusFromKnownCount({
    blocked: !testReady,
    count: bindings.enabledActiveAlertTemplateCount,
    known: bindings.toolTemplatesKnown,
  });
  steps.push(
    {
      detail: metricToolStepDetail(bindings),
      key: "metric-evidence-tool",
      label: "Metric tools",
      status: metricToolStatus,
    },
    {
      detail: alertToolStepDetail(bindings),
      key: "active-alert-tool",
      label: "Alert tool",
      status: activeToolStatus,
    },
  );
  return steps;
}

function unknownAlertSourceAutomationBindings(): AlertSourceAutomationBindings {
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
    toolTemplatesKnown: false,
    workflowPoliciesKnown: false,
  };
}

function setupStatusFromKnownCount({
  blocked,
  count,
  known,
}: {
  blocked: boolean;
  count: number;
  known: boolean;
}): AlertSourceSetupStatus {
  if (blocked) {
    return "blocked";
  }
  if (!known) {
    return "pending";
  }
  return count > 0 ? "ready" : "pending";
}

function alertToolStepDetail(bindings: AlertSourceAutomationBindings): string {
  if (!bindings.toolTemplatesKnown) {
    return "Load diagnosis tool templates to check active_alerts coverage.";
  }
  if (bindings.enabledActiveAlertTemplateCount > 0) {
    return `${bindings.enabledActiveAlertTemplateCount} enabled active_alerts template(s) are bound to this source.`;
  }
  if (bindings.activeAlertTemplateCount > 0) {
    return `${bindings.activeAlertTemplateCount} active_alerts template(s) exist for this source but are disabled.`;
  }
  return "Create an active_alerts evidence template for this source.";
}

function metricToolStepDetail(
  bindings: AlertSourceAutomationBindings,
): string {
  if (!bindings.toolTemplatesKnown) {
    return "Load diagnosis tool templates to check metric evidence coverage.";
  }
  const enabledMetricTemplates =
    bindings.enabledMetricTemplateCount + bindings.enabledRangeMetricTemplateCount;
  if (enabledMetricTemplates > 0) {
    return `${enabledMetricTemplates} enabled metric evidence template(s) are bound to this source.`;
  }
  const metricTemplates =
    bindings.metricTemplateCount + bindings.rangeMetricTemplateCount;
  if (metricTemplates > 0) {
    return `${metricTemplates} metric evidence template(s) exist for this source but are disabled.`;
  }
  return "Create metric evidence templates for instant and range queries.";
}

function workflowMetricEvidenceStepDetail(
  bindings: AlertSourceAutomationBindings,
): string {
  if (!bindings.toolTemplatesKnown) {
    return "Load diagnosis tool templates to check metric evidence coverage.";
  }
  if (bindings.enabledWorkflowMetricTemplateCount > 0) {
    return `${bindings.enabledWorkflowMetricTemplateCount} enabled metric evidence template(s) are available across Prometheus-compatible sources.`;
  }
  if (bindings.metricEvidenceSourceCount > 0) {
    return `${bindings.metricEvidenceSourceCount} metric evidence source(s) exist. Create a metric_query or metric_range_query template before relying on automatic diagnosis confidence.`;
  }
  return "Create a Prometheus-compatible metric evidence source, usually Thanos Query, and add metric_query or metric_range_query templates.";
}

function notificationChannelStepDetail(
  bindings: AlertSourceAutomationBindings,
): string {
  if (!bindings.workflowPoliciesKnown) {
    return "Load report workflow policies to check whether an enabled automatic workflow already binds an Enterprise WeChat AI channel.";
  }
  if (bindings.enabledAutoRoomPolicyCount > 0) {
    return `${bindings.enabledAutoRoomPolicyCount} enabled automatic diagnosis workflow(s) are bound to this source; workflow enablement covers the required WeCom scopes and AI delivery proof.`;
  }
  if (bindings.autoRoomPolicyCount > 0) {
    return `${bindings.autoRoomPolicyCount} automatic diagnosis workflow(s) exist for this source but are disabled; enablement requires WeCom report, diagnosis_consultation, diagnosis_close scopes, and current AI proof.`;
  }
  return "Configure a WeCom channel with report, diagnosis_consultation, and diagnosis_close scopes; workflow setup verifies delivery proof.";
}

function autoRoomWorkflowStepDetail(
  bindings: AlertSourceAutomationBindings,
): string {
  if (!bindings.workflowPoliciesKnown) {
    return "Load workflow policies to check automatic diagnosis binding.";
  }
  if (bindings.enabledAutoRoomPolicyCount > 0) {
    return `${bindings.enabledAutoRoomPolicyCount} enabled automatic diagnosis workflow(s) are bound to this source.`;
  }
  if (bindings.autoRoomPolicyCount > 0) {
    return `${bindings.autoRoomPolicyCount} automatic diagnosis workflow(s) exist for this source but are disabled.`;
  }
  return "Create or update an automatic diagnosis workflow after the alert tool is ready.";
}
