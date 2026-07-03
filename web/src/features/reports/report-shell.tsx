"use client";

import {
  AppstoreOutlined,
  ApiOutlined,
  AuditOutlined,
  BellOutlined,
  BranchesOutlined,
  CalendarOutlined,
  FileTextOutlined,
  MessageOutlined,
  PartitionOutlined,
  SettingOutlined,
  ToolOutlined,
  WarningOutlined
} from "@ant-design/icons";
import { Layout, Menu, Typography } from "antd";
import type { MenuProps } from "antd";
import Link from "next/link";
import type { ReactNode } from "react";

type ReportShellProps = {
  children: ReactNode;
  current:
    | "dashboard"
    | "alerts"
    | "reports"
    | "diagnosis"
    | "settings"
    | "directory-rbac"
    | "sources"
    | "grouping"
    | "tools"
    | "workflow"
    | "schedules"
    | "channels";
};

const navItems: MenuProps["items"] = [
  {
    icon: <AppstoreOutlined aria-label="Dashboard navigation icon" />,
    key: "dashboard",
    label: <Link href="/dashboard">Dashboard</Link>
  },
  {
    icon: <WarningOutlined aria-label="Alerts navigation icon" />,
    key: "alerts",
    label: <Link href="/alerts">Alerts</Link>
  },
  {
    icon: <FileTextOutlined aria-label="Reports navigation icon" />,
    key: "reports",
    label: <Link href="/reports">Reports</Link>
  },
  {
    icon: <MessageOutlined aria-label="Diagnosis navigation icon" />,
    key: "diagnosis",
    label: <Link href="/diagnosis-room">Diagnosis</Link>
  },
  {
    icon: <SettingOutlined aria-label="Settings navigation icon" />,
    key: "settings",
    label: <Link href="/settings">Settings</Link>
  },
  {
    icon: <AuditOutlined aria-label="Directory and RBAC navigation icon" />,
    key: "directory-rbac",
    label: <Link href="/settings/directory-rbac">Access</Link>
  },
  {
    icon: <ApiOutlined aria-label="Alert sources navigation icon" />,
    key: "sources",
    label: <Link href="/settings/alert-sources">Sources</Link>
  },
  {
    icon: <PartitionOutlined aria-label="Grouping navigation icon" />,
    key: "grouping",
    label: <Link href="/settings/grouping-policies">Grouping</Link>
  },
  {
    icon: <ToolOutlined aria-label="Tools navigation icon" />,
    key: "tools",
    label: <Link href="/settings/diagnosis-tool-templates">Tools</Link>
  },
  {
    icon: <BranchesOutlined aria-label="Workflow navigation icon" />,
    key: "workflow",
    label: <Link href="/settings/report-workflow-policies">Workflow</Link>
  },
  {
    icon: <CalendarOutlined aria-label="Schedules navigation icon" />,
    key: "schedules",
    label: <Link href="/settings/report-workflow-schedules">Schedules</Link>
  },
  {
    icon: <BellOutlined aria-label="Channels navigation icon" />,
    key: "channels",
    label: <Link href="/settings/notification-channels">Channels</Link>
  }
];

export function ReportShell({ children, current }: ReportShellProps) {
  return (
    <Layout className="app-console-shell">
      <Layout.Header className="app-console-header">
        <Link className="app-console-brand" href="/dashboard">
          <span className="app-console-brand-mark">OC</span>
          <span className="app-console-brand-copy">
            <Typography.Text className="app-console-title">OpenClarion</Typography.Text>
            <Typography.Text className="app-console-context">Alert operations console</Typography.Text>
          </span>
        </Link>
        <Menu
          aria-label="Primary"
          className="app-console-nav"
          items={navItems}
          mode="horizontal"
          selectedKeys={[current]}
        />
      </Layout.Header>
      <Layout.Content className="app-console-content">{children}</Layout.Content>
    </Layout>
  );
}
