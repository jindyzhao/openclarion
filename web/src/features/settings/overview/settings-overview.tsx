"use client";

import {
  ApiOutlined,
  AuditOutlined,
  BellOutlined,
  BranchesOutlined,
  CalendarOutlined,
  CheckCircleOutlined,
  ExperimentOutlined,
  PartitionOutlined,
  PlayCircleOutlined,
  RadarChartOutlined,
  ToolOutlined
} from "@ant-design/icons";
import { Alert, Button, Card, Col, Descriptions, Row, Space, Steps, Tag, Typography } from "antd";
import type { ReactNode } from "react";

import type { AlertSourceProfile } from "../alert-sources/types";
import type { DiagnosisToolTemplate } from "../diagnosis-tool-templates/types";
import type { GroupingPolicy } from "../grouping-policies/types";
import type { NotificationChannelProfile } from "../notification-channels/types";
import type { ReportWorkflowPolicy } from "../report-workflow-policies/types";
import type { ReportWorkflowSchedule } from "../report-workflow-schedules/types";

type SettingsOverviewCounts = {
  alertSources: number | null;
  groupingPolicies: number | null;
  diagnosisToolTemplates: number | null;
  notificationChannels: number | null;
  workflowPolicies: number | null;
  workflowSchedules: number | null;
};

type SettingsOverviewProps = {
  alertSources: AlertSourceProfile[];
  counts: SettingsOverviewCounts;
  diagnosisToolTemplates: DiagnosisToolTemplate[];
  groupingPolicies: GroupingPolicy[];
  notificationChannels: NotificationChannelProfile[];
  workflowPolicies: ReportWorkflowPolicy[];
  workflowSchedules: ReportWorkflowSchedule[];
};

type Stage = {
  count: number | null;
  countLabel: string;
  href: string;
  icon: ReactNode;
  key: string;
  title: string;
};

type ProofTarget = {
  evidence: string[];
  icon: ReactNode;
  key: string;
  title: string;
};

type WorkflowTopology = {
  activeSchedule: ReportWorkflowSchedule | null;
  alertSource: AlertSourceProfile | null;
  diagnosisTools: DiagnosisToolTemplate[];
  groupingPolicy: GroupingPolicy | null;
  notificationChannel: NotificationChannelProfile | null;
  policy: ReportWorkflowPolicy | null;
  status: "ready" | "review" | "blocked";
};

type WorkflowTopologyStep = {
  description: string;
  status: "error" | "finish" | "process" | "wait";
  title: string;
};

type TopologyAction = {
  detail: string;
  href: string;
  key: string;
  priority: "high" | "medium" | "low";
  title: string;
};

export function SettingsOverview({
  alertSources,
  counts,
  diagnosisToolTemplates,
  groupingPolicies,
  notificationChannels,
  workflowPolicies,
  workflowSchedules
}: SettingsOverviewProps) {
  const stages: Stage[] = [
    {
      count: counts.alertSources,
      countLabel: "profiles",
      href: "/settings/alert-sources",
      icon: <ApiOutlined aria-hidden="true" />,
      key: "sources",
      title: "Alert sources"
    },
    {
      count: counts.groupingPolicies,
      countLabel: "policies",
      href: "/settings/grouping-policies",
      icon: <PartitionOutlined aria-hidden="true" />,
      key: "grouping",
      title: "Grouping"
    },
    {
      count: counts.diagnosisToolTemplates,
      countLabel: "templates",
      href: "/settings/diagnosis-tool-templates",
      icon: <ToolOutlined aria-hidden="true" />,
      key: "tools",
      title: "Diagnosis tools"
    },
    {
      count: counts.notificationChannels,
      countLabel: "channels",
      href: "/settings/notification-channels",
      icon: <BellOutlined aria-hidden="true" />,
      key: "channels",
      title: "Notifications"
    },
    {
      count: counts.workflowPolicies,
      countLabel: "policies",
      href: "/settings/report-workflow-policies",
      icon: <BranchesOutlined aria-hidden="true" />,
      key: "workflow",
      title: "Workflow policies"
    },
    {
      count: counts.workflowSchedules,
      countLabel: "schedules",
      href: "/settings/report-workflow-schedules",
      icon: <CalendarOutlined aria-hidden="true" />,
      key: "schedules",
      title: "Schedules"
    }
  ];
  const nextStageIndex = stages.findIndex((stage) => !isReadyCount(stage.count));
  const currentStep = nextStageIndex === -1 ? stages.length : nextStageIndex;
  const nextStage = nextStageIndex === -1 ? null : stages[nextStageIndex] ?? null;
  const allConfigurationPresent = nextStage === null;
  const topology = buildWorkflowTopology({
    alertSources,
    diagnosisToolTemplates,
    groupingPolicies,
    notificationChannels,
    workflowPolicies,
    workflowSchedules
  });
  const proofTargets: ProofTarget[] = [
    {
      evidence: ["PostgreSQL", "Temporal", "Alert source", "Tool template", "LLM", "Notification"],
      icon: <PlayCircleOutlined aria-hidden="true" />,
      key: "policy-replay",
      title: "Policy replay"
    },
    {
      evidence: ["Enabled schedule", "Temporal action", "Launcher workflow", "Report delivery"],
      icon: <CalendarOutlined aria-hidden="true" />,
      key: "scheduled-trigger",
      title: "Scheduled trigger"
    }
  ];

  return (
    <div className="stack">
      <section className="panel">
        <div className="panel-header">
          <h2>Configuration graph</h2>
        </div>
        <div className="panel-body settings-overview-flow">
          <Steps
            aria-label="Alert operations configuration sequence"
            className="settings-overview-steps"
            current={currentStep}
            items={[
              ...stages.map((stage, index) => ({
                icon: stage.icon,
                status: stepStatus(stage.count, index, currentStep),
                title: shortStageTitle(stage.title)
              })),
              {
                icon: <PlayCircleOutlined />,
                status: allConfigurationPresent ? "process" : "wait",
                title: "Proof"
              }
            ]}
            responsive={false}
            type="inline"
          />
        </div>
      </section>

      <section aria-label="Next setup stage" className="panel settings-overview-next">
        <div>
          <Typography.Text className="muted">Next setup</Typography.Text>
          <Typography.Title level={2}>
            {nextStage === null ? "Retained live proof" : nextStage.title}
          </Typography.Title>
        </div>
        {nextStage === null ? (
          <Tag color="gold">Proof pending</Tag>
        ) : (
          <Button href={nextStage.href} icon={<RadarChartOutlined />} type="primary">
            Configure
          </Button>
        )}
      </section>

      <WorkflowTopologyPanel topology={topology} />

      <Row aria-label="Retained proof targets" gutter={[16, 16]}>
        {proofTargets.map((target) => (
          <Col key={target.key} lg={12} xs={24}>
            <Card className="settings-overview-proof-card" title={<CardTitle icon={target.icon} title={target.title} />}>
              <div className="settings-overview-proof-body">
                <Tag color={allConfigurationPresent ? "gold" : "default"}>
                  {allConfigurationPresent ? "Pending external inputs" : "Waiting for graph"}
                </Tag>
                <div className="settings-overview-proof-tags" aria-label={`${target.title} evidence`}>
                  {target.evidence.map((item) => (
                    <Tag key={item}>{item}</Tag>
                  ))}
                </div>
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      <Row aria-label="Settings surfaces" gutter={[16, 16]}>
        {stages.map((stage) => (
          <Col key={stage.key} lg={8} md={12} xs={24}>
            <Card className="settings-overview-card" title={<CardTitle icon={stage.icon} title={stage.title} />}>
              <div className="settings-overview-card-body">
                <div>
                  <div className="settings-overview-count">{formatCount(stage.count)}</div>
                  <Typography.Text className="muted">{stage.countLabel}</Typography.Text>
                </div>
                <ReadinessTag count={stage.count} />
              </div>
              <Button block href={stage.href} icon={<RadarChartOutlined />} type="default">
                Open
              </Button>
            </Card>
          </Col>
        ))}
      </Row>

      <Row aria-label="Settings action boundaries" gutter={[16, 16]}>
        <Col lg={8} xs={24}>
          <BoundaryPanel
            icon={<ExperimentOutlined />}
            status="Bounded I/O"
            title="Connection tests"
            text="Source and channel tests stay behind backend action endpoints with sanitized results."
          />
        </Col>
        <Col lg={8} xs={24}>
          <BoundaryPanel
            icon={<AuditOutlined />}
            status="No workflow start"
            title="Previews"
            text="Grouping and impact previews read bounded persisted samples without provider calls or workflow starts."
          />
        </Col>
        <Col lg={8} xs={24}>
          <BoundaryPanel
            icon={<CheckCircleOutlined />}
            status="External proof pending"
            title="Replay and schedules"
            text="Policy replay and scheduled delivery require retained live proof against operator-provided services."
          />
        </Col>
      </Row>

      <Alert
        message={allConfigurationPresent ? "Live proof gate" : "Configuration gate"}
        showIcon
        type={allConfigurationPresent ? "warning" : "info"}
        description={
          allConfigurationPresent
            ? "Policy replay and scheduled-trigger acceptance still require retained proof against real PostgreSQL, Temporal, alert-source, LLM, and notification delivery inputs."
            : "Retained proof targets activate only after the alert source, grouping, notification, workflow policy, and schedule objects exist server-side."
        }
      />
    </div>
  );
}

function CardTitle({ icon, title }: { icon: ReactNode; title: string }) {
  return (
    <Space size={8}>
      {icon}
      <span>{title}</span>
    </Space>
  );
}

function BoundaryPanel({
  icon,
  status,
  text,
  title
}: {
  icon: ReactNode;
  status: string;
  text: string;
  title: string;
}) {
  return (
    <section className="panel settings-overview-boundary">
      <div className="settings-overview-boundary-title">
        <Space size={8}>
          {icon}
          <Typography.Text strong>{title}</Typography.Text>
        </Space>
        <Tag>{status}</Tag>
      </div>
      <Typography.Paragraph className="settings-overview-boundary-copy">{text}</Typography.Paragraph>
    </section>
  );
}

function WorkflowTopologyPanel({ topology }: { topology: WorkflowTopology }) {
  const hasPolicy = topology.policy !== null;
  const steps = workflowTopologySteps(topology);
  const firstUnready = steps.findIndex((step) => step.status !== "finish");
  const current = firstUnready === -1 ? steps.length : firstUnready;
  const actions = workflowTopologyActions(topology);

  return (
    <section aria-label="Active workflow topology" className="panel settings-overview-topology">
      <div className="panel-header settings-overview-topology-header">
        <h2>Active workflow topology</h2>
        <Tag color={topologyStatusColor(topology.status)}>{topology.status}</Tag>
      </div>
      <div className="panel-body settings-overview-topology-body">
        {hasPolicy ? (
          <>
            <div className="settings-overview-topology-flow">
              <Steps
                aria-label="Selected alert workflow topology"
                className="settings-overview-topology-steps"
                current={current}
                items={steps}
                responsive={false}
                size="small"
              />
            </div>
            <Descriptions
              bordered
              column={{ lg: 3, md: 2, sm: 1, xs: 1 }}
              items={workflowTopologyDescriptions(topology)}
              size="small"
            />
            <TopologyActionList actions={actions} />
          </>
        ) : (
          <Alert
            message="No workflow policy"
            description="Create a workflow policy after configuring an alert source and grouping policy."
            showIcon
            type="info"
          />
        )}
      </div>
    </section>
  );
}

function TopologyActionList({ actions }: { actions: TopologyAction[] }) {
  if (actions.length === 0) {
    return (
      <Alert
        message="Topology ready"
        description="The selected workflow chain is ready for retained live proof."
        showIcon
        type="success"
      />
    );
  }

  return (
    <section aria-label="Next topology actions" className="settings-overview-action-list">
      <div className="settings-overview-action-header">
        <Typography.Text strong>Next topology actions</Typography.Text>
        <Tag>{actions.length} open</Tag>
      </div>
      <div className="settings-overview-action-grid">
        {actions.map((action) => (
          <div className="settings-overview-action-item" key={action.key}>
            <div>
              <Space size={8} wrap>
                <Tag color={actionPriorityColor(action.priority)}>{action.priority}</Tag>
                <Typography.Text strong>{action.title}</Typography.Text>
              </Space>
              <Typography.Paragraph className="settings-overview-action-copy">{action.detail}</Typography.Paragraph>
            </div>
            <Button href={action.href} icon={<RadarChartOutlined />} type={action.priority === "high" ? "primary" : "default"}>
              Open
            </Button>
          </div>
        ))}
      </div>
    </section>
  );
}

function buildWorkflowTopology({
  alertSources,
  diagnosisToolTemplates,
  groupingPolicies,
  notificationChannels,
  workflowPolicies,
  workflowSchedules
}: {
  alertSources: AlertSourceProfile[];
  diagnosisToolTemplates: DiagnosisToolTemplate[];
  groupingPolicies: GroupingPolicy[];
  notificationChannels: NotificationChannelProfile[];
  workflowPolicies: ReportWorkflowPolicy[];
  workflowSchedules: ReportWorkflowSchedule[];
}): WorkflowTopology {
  const policy =
    workflowPolicies.find((item) => item.enabled && item.diagnosis_follow_up === "suggest_room") ??
    workflowPolicies.find((item) => item.enabled) ??
    workflowPolicies[0] ??
    null;
  const alertSource = policy === null ? null : alertSources.find((item) => item.id === policy.alert_source_profile_id) ?? null;
  const groupingPolicy = policy === null ? null : groupingPolicies.find((item) => item.id === policy.grouping_policy_id) ?? null;
  const notificationChannel =
    policy?.report_notification_channel_profile_id == null
      ? null
      : notificationChannels.find((item) => item.id === policy.report_notification_channel_profile_id) ?? null;
  const activeSchedule =
    policy === null
      ? null
      : workflowSchedules.find((item) => item.report_workflow_policy_id === policy.id && item.enabled) ??
        workflowSchedules.find((item) => item.report_workflow_policy_id === policy.id) ??
        null;
  const diagnosisTools =
    alertSource === null
      ? []
      : diagnosisToolTemplates.filter((item) => item.alert_source_profile_id === alertSource.id && item.enabled);
  const status = topologyStatus(policy, alertSource, groupingPolicy, notificationChannel, diagnosisTools);
  return { activeSchedule, alertSource, diagnosisTools, groupingPolicy, notificationChannel, policy, status };
}

function workflowTopologyActions(topology: WorkflowTopology): TopologyAction[] {
  const actions: TopologyAction[] = [];

  if (topology.policy === null) {
    return [
      {
        key: "create-policy",
        title: "Create workflow policy",
        detail: "Bind an alert source and grouping policy before replaying alert windows.",
        href: "/settings/report-workflow-policies",
        priority: "high"
      }
    ];
  }

  if (!topology.policy.enabled) {
    actions.push({
      key: "enable-policy",
      title: "Enable workflow policy",
      detail: "The selected workflow policy is still a draft and will not be used for replay or schedule proof.",
      href: "/settings/report-workflow-policies",
      priority: "high"
    });
  }
  if (topology.alertSource === null || !topology.alertSource.enabled) {
    actions.push({
      key: "alert-source",
      title: "Review alert source",
      detail: "The selected workflow needs an enabled alert source before alert windows can be replayed.",
      href: "/settings/alert-sources",
      priority: "high"
    });
  }
  if (topology.groupingPolicy === null || !topology.groupingPolicy.enabled) {
    actions.push({
      key: "grouping",
      title: "Review grouping policy",
      detail: "The selected workflow needs an enabled grouping policy to build incident groups.",
      href: "/settings/grouping-policies",
      priority: "high"
    });
  }
  if (topology.policy.diagnosis_follow_up !== "suggest_room") {
    actions.push({
      key: "room-follow-up",
      title: "Enable AI room follow-up",
      detail: "Set diagnosis follow-up to suggest room so reports hand off into the consultation workflow.",
      href: "/settings/report-workflow-policies",
      priority: "medium"
    });
  }
  if (topology.diagnosisTools.length === 0) {
    actions.push({
      key: "diagnosis-tools",
      title: "Enable diagnosis tools",
      detail: "Enable at least one tool template for the selected alert source so the AI room can collect evidence.",
      href: "/settings/diagnosis-tool-templates",
      priority: "medium"
    });
  }
  if (topology.notificationChannel !== null && !topology.notificationChannel.enabled) {
    actions.push({
      key: "notification-channel",
      title: "Enable report channel",
      detail: "The selected report channel is bound but disabled, so report delivery proof cannot complete.",
      href: "/settings/notification-channels",
      priority: "medium"
    });
  }
  if (topology.notificationChannel !== null && !hasReportScope(topology.notificationChannel)) {
    actions.push({
      key: "notification-channel-scope",
      title: "Review report channel scope",
      detail: "The selected report channel must include the report delivery scope before reports can be delivered.",
      href: "/settings/notification-channels",
      priority: "medium"
    });
  }
  if (topology.activeSchedule === null) {
    actions.push({
      key: "create-schedule",
      title: "Create workflow schedule",
      detail: "Add a schedule when the workflow needs retained scheduled proof instead of manual replay only.",
      href: "/settings/report-workflow-schedules",
      priority: "low"
    });
  } else if (!topology.activeSchedule.enabled) {
    actions.push({
      key: "enable-schedule",
      title: "Enable workflow schedule",
      detail: "The schedule exists as a draft; enable it after replay proof is accepted.",
      href: "/settings/report-workflow-schedules",
      priority: "low"
    });
  }

  if (topology.policy.enabled && topology.alertSource?.enabled && topology.groupingPolicy?.enabled) {
    actions.push({
      key: "impact-preview",
      title: "Run impact preview",
      detail: "Preview recent alert grouping before replaying a workflow window.",
      href: "/settings/report-workflow-policies",
      priority: "low"
    });
  }

  return actions.slice(0, 5);
}

function topologyStatus(
  policy: ReportWorkflowPolicy | null,
  alertSource: AlertSourceProfile | null,
  groupingPolicy: GroupingPolicy | null,
  notificationChannel: NotificationChannelProfile | null,
  diagnosisTools: DiagnosisToolTemplate[]
): WorkflowTopology["status"] {
  if (policy === null || alertSource === null || groupingPolicy === null) {
    return "blocked";
  }
  if (!policy.enabled || !alertSource.enabled || !groupingPolicy.enabled) {
    return "blocked";
  }
  if (notificationChannel !== null && (!notificationChannel.enabled || !hasReportScope(notificationChannel))) {
    return "blocked";
  }
  if (policy.diagnosis_follow_up !== "suggest_room" || diagnosisTools.length === 0) {
    return "review";
  }
  return "ready";
}

function workflowTopologySteps(topology: WorkflowTopology): WorkflowTopologyStep[] {
  return [
    {
      title: "Source",
      description: topology.alertSource?.name ?? "Missing source",
      status: topology.alertSource?.enabled ? "finish" : "error"
    },
    {
      title: "Grouping",
      description: topology.groupingPolicy?.name ?? "Missing grouping",
      status: topology.groupingPolicy?.enabled ? "finish" : "error"
    },
    {
      title: "Evidence",
      description: `${topology.diagnosisTools.length} enabled tools`,
      status: topology.diagnosisTools.length > 0 ? "finish" : "wait"
    },
    {
      title: "AI room",
      description: topology.policy?.diagnosis_follow_up ?? "No policy",
      status: topology.policy?.enabled && topology.policy.diagnosis_follow_up === "suggest_room" ? "finish" : "wait"
    },
    {
      title: "Delivery",
      description: topology.notificationChannel?.name ?? "No report channel",
      status:
        topology.notificationChannel === null ||
        (topology.notificationChannel.enabled && hasReportScope(topology.notificationChannel))
          ? "finish"
          : "error"
    },
    {
      title: "Schedule",
      description: topology.activeSchedule?.name ?? "Manual replay",
      status: topology.activeSchedule?.enabled ? "finish" : "wait"
    }
  ];
}

function workflowTopologyDescriptions(topology: WorkflowTopology) {
  return [
    {
      key: "policy",
      label: "Policy",
      children: topology.policy === null ? "None" : topology.policy.name
    },
    {
      key: "source",
      label: "Alert source",
      children: topology.alertSource === null ? "Missing" : configLabel(topology.alertSource.name, topology.alertSource.enabled)
    },
    {
      key: "grouping",
      label: "Grouping",
      children:
        topology.groupingPolicy === null
          ? "Missing"
          : configLabel(topology.groupingPolicy.name, topology.groupingPolicy.enabled)
    },
    {
      key: "tools",
      label: "Evidence tools",
      children:
        topology.diagnosisTools.length === 0 ? (
          <Typography.Text type="secondary">No enabled tools for selected source</Typography.Text>
        ) : (
          <Space wrap>
            {topology.diagnosisTools.map((tool) => (
              <Tag key={tool.id}>{tool.name}</Tag>
            ))}
          </Space>
        )
    },
    {
      key: "channel",
      label: "Report channel",
      children:
        topology.notificationChannel === null
          ? "Optional"
          : configLabel(topology.notificationChannel.name, topology.notificationChannel.enabled)
    },
    {
      key: "schedule",
      label: "Schedule",
      children:
        topology.activeSchedule === null
          ? "Manual replay"
          : configLabel(topology.activeSchedule.name, topology.activeSchedule.enabled)
    }
  ];
}

function hasReportScope(channel: NotificationChannelProfile): boolean {
  return channel.delivery_scopes.includes("report");
}

function configLabel(name: string, enabled: boolean) {
  return (
    <Space size={6} wrap>
      <Typography.Text>{name}</Typography.Text>
      <Tag color={enabled ? "green" : "default"}>{enabled ? "Enabled" : "Draft"}</Tag>
    </Space>
  );
}

function topologyStatusColor(status: WorkflowTopology["status"]): string {
  switch (status) {
    case "ready":
      return "green";
    case "review":
      return "gold";
    case "blocked":
      return "red";
  }
}

function actionPriorityColor(priority: TopologyAction["priority"]): string {
  switch (priority) {
    case "high":
      return "red";
    case "medium":
      return "gold";
    case "low":
      return "blue";
  }
}

function formatCount(count: number | null): string {
  if (count === null) {
    return "-";
  }
  return count.toString();
}

function isReadyCount(count: number | null): boolean {
  return count !== null && count > 0;
}

function shortStageTitle(title: string): string {
  switch (title) {
    case "Alert sources":
      return "Source";
    case "Workflow policies":
      return "Policy";
    case "Notifications":
      return "Channel";
    case "Schedules":
      return "Schedule";
    default:
      return title;
  }
}

function stepStatus(
  count: number | null,
  index: number,
  currentStep: number
): "error" | "finish" | "process" | "wait" {
  if (count === null) {
    return index === currentStep ? "error" : "wait";
  }
  if (count > 0) {
    return "finish";
  }
  return index === currentStep ? "process" : "wait";
}

function ReadinessTag({ count }: { count: number | null }) {
  if (count === null) {
    return <Tag color="red">Unavailable</Tag>;
  }
  if (count === 0) {
    return <Tag color="gold">Needs setup</Tag>;
  }
  return <Tag color="green">Ready</Tag>;
}
