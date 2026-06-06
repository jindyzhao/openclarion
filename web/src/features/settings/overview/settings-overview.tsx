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
  RadarChartOutlined
} from "@ant-design/icons";
import { Alert, Button, Card, Col, Row, Space, Steps, Tag, Typography } from "antd";
import type { ReactNode } from "react";

type SettingsOverviewCounts = {
  alertSources: number | null;
  groupingPolicies: number | null;
  notificationChannels: number | null;
  workflowPolicies: number | null;
  workflowSchedules: number | null;
};

type SettingsOverviewProps = {
  counts: SettingsOverviewCounts;
};

type Stage = {
  count: number | null;
  countLabel: string;
  href: string;
  icon: ReactNode;
  key: string;
  title: string;
};

export function SettingsOverview({ counts }: SettingsOverviewProps) {
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
            ? "Profile-driven replay and scheduled-trigger acceptance still require real PostgreSQL, Temporal, alert-source, LLM, and notification delivery inputs."
            : "Retained proof starts only after the alert source, grouping, notification, workflow policy, and schedule objects exist server-side."
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
