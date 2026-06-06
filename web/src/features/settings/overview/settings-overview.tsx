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
  tone: "active" | "draft" | "proof";
};

export function SettingsOverview({ counts }: SettingsOverviewProps) {
  const stages: Stage[] = [
    {
      count: counts.alertSources,
      countLabel: "profiles",
      href: "/settings/alert-sources",
      icon: <ApiOutlined aria-hidden="true" />,
      key: "sources",
      title: "Alert sources",
      tone: "active"
    },
    {
      count: counts.groupingPolicies,
      countLabel: "policies",
      href: "/settings/grouping-policies",
      icon: <PartitionOutlined aria-hidden="true" />,
      key: "grouping",
      title: "Grouping",
      tone: "active"
    },
    {
      count: counts.notificationChannels,
      countLabel: "channels",
      href: "/settings/notification-channels",
      icon: <BellOutlined aria-hidden="true" />,
      key: "channels",
      title: "Notifications",
      tone: "active"
    },
    {
      count: counts.workflowPolicies,
      countLabel: "policies",
      href: "/settings/report-workflow-policies",
      icon: <BranchesOutlined aria-hidden="true" />,
      key: "workflow",
      title: "Workflow policies",
      tone: "active"
    },
    {
      count: counts.workflowSchedules,
      countLabel: "schedules",
      href: "/settings/report-workflow-schedules",
      icon: <CalendarOutlined aria-hidden="true" />,
      key: "schedules",
      title: "Schedules",
      tone: "draft"
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
            current={3}
            items={[
              { icon: <ApiOutlined />, title: "Source" },
              { icon: <PartitionOutlined />, title: "Grouping" },
              { icon: <BellOutlined />, title: "Channel" },
              { icon: <BranchesOutlined />, title: "Policy" },
              { icon: <CalendarOutlined />, title: "Schedule" },
              { icon: <PlayCircleOutlined />, title: "Proof" }
            ]}
            responsive={false}
            type="inline"
          />
        </div>
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
                <Tag color={stage.tone === "draft" ? "gold" : "green"}>
                  {stage.tone === "draft" ? "server-owned" : "configured"}
                </Tag>
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
        message="Live proof gate"
        showIcon
        type="warning"
        description="Profile-driven replay and scheduled-trigger acceptance still require real PostgreSQL, Temporal, alert-source, LLM, and notification delivery inputs."
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
