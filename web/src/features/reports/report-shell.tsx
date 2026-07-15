"use client";

import {
  ApiOutlined,
  ApartmentOutlined,
  AppstoreOutlined,
  AuditOutlined,
  BellOutlined,
  BranchesOutlined,
  CalendarOutlined,
  FileTextOutlined,
  MenuFoldOutlined,
  MenuOutlined,
  MenuUnfoldOutlined,
  MessageOutlined,
  PartitionOutlined,
  SettingOutlined,
  ToolOutlined,
  WarningOutlined
} from "@ant-design/icons";
import { Button, Drawer, Layout, Menu, Tooltip, Typography } from "antd";
import type { MenuProps } from "antd";
import type { Route } from "next";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { Suspense, useMemo, useState, type ReactNode } from "react";

import { LocaleSwitcher } from "@/features/console/locale-switcher";
import {
  ConsoleSessionControl,
  ConsoleSessionControlFallback,
} from "@/features/console/session-control";

type ConsoleSection =
  | "dashboard"
  | "alerts"
  | "reports"
  | "diagnosis"
  | "settings"
  | "workspaces"
  | "directory-rbac"
  | "sources"
  | "grouping"
  | "tools"
  | "workflow"
  | "schedules"
  | "channels";

type ConsoleShellProps = Readonly<{
  children: ReactNode;
}>;

type NavigationItem = {
  href: Route;
  icon: ReactNode;
  key: ConsoleSection;
  labelKey: string;
};

type NavigationSection = {
  key: string;
  labelKey: string;
  items: NavigationItem[];
};

const navigationSections: NavigationSection[] = [
  {
    key: "workspace",
    labelKey: "sections.workspace",
    items: [
      { href: "/dashboard", icon: <AppstoreOutlined aria-hidden aria-label="" />, key: "dashboard", labelKey: "items.dashboard" },
      { href: "/alerts", icon: <WarningOutlined aria-hidden aria-label="" />, key: "alerts", labelKey: "items.alerts" },
      { href: "/reports", icon: <FileTextOutlined aria-hidden aria-label="" />, key: "reports", labelKey: "items.reports" },
      { href: "/diagnosis-room", icon: <MessageOutlined aria-hidden aria-label="" />, key: "diagnosis", labelKey: "items.diagnosis" }
    ]
  },
  {
    key: "automation",
    labelKey: "sections.automation",
    items: [
      {
        href: "/settings/report-workflow-policies",
        icon: <BranchesOutlined aria-hidden aria-label="" />,
        key: "workflow",
        labelKey: "items.workflow"
      },
      {
        href: "/settings/report-workflow-schedules",
        icon: <CalendarOutlined aria-hidden aria-label="" />,
        key: "schedules",
        labelKey: "items.schedules"
      }
    ]
  },
  {
    key: "configuration",
    labelKey: "sections.configuration",
    items: [
      { href: "/settings", icon: <SettingOutlined aria-hidden aria-label="" />, key: "settings", labelKey: "items.settings" },
      {
        href: "/settings/workspaces",
        icon: <ApartmentOutlined aria-hidden aria-label="" />,
        key: "workspaces",
        labelKey: "items.workspaces"
      },
      { href: "/settings/alert-sources", icon: <ApiOutlined aria-hidden aria-label="" />, key: "sources", labelKey: "items.sources" },
      {
        href: "/settings/grouping-policies",
        icon: <PartitionOutlined aria-hidden aria-label="" />,
        key: "grouping",
        labelKey: "items.grouping"
      },
      {
        href: "/settings/diagnosis-tool-templates",
        icon: <ToolOutlined aria-hidden aria-label="" />,
        key: "tools",
        labelKey: "items.tools"
      },
      {
        href: "/settings/notification-channels",
        icon: <BellOutlined aria-hidden aria-label="" />,
        key: "channels",
        labelKey: "items.channels"
      },
      {
        href: "/settings/directory-rbac",
        icon: <AuditOutlined aria-hidden aria-label="" />,
        key: "directory-rbac",
        labelKey: "items.access"
      }
    ]
  }
];

const navigationByKey = new Map(
  navigationSections.flatMap((section) =>
    section.items.map((item) => [item.key, { ...item, sectionLabelKey: section.labelKey }] as const)
  )
);

export function ConsoleShell({ children }: ConsoleShellProps) {
  const pathname = usePathname();
  const t = useTranslations("Shell");
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const current = consoleSectionForPathname(pathname);
  const currentItem = navigationByKey.get(current) ?? navigationByKey.get("dashboard")!;

  const menuItems = useMemo<MenuProps["items"]>(
    () =>
      navigationSections.map((section) => ({
        children: section.items.map((item) => ({
          icon: item.icon,
          key: item.key,
          label: (
            <Link href={item.href} prefetch={false}>
              {t(item.labelKey)}
            </Link>
          ),
          title: t(item.labelKey)
        })),
        key: section.key,
        label: t(section.labelKey),
        type: "group"
      })),
    [t]
  );

  function renderNavigation(isCollapsed: boolean) {
    return (
      <>
        <Link
          className="app-console-brand"
          href="/dashboard"
          onClick={() => setMobileOpen(false)}
          prefetch={false}
        >
          <span className="app-console-brand-mark" aria-hidden>
            OC
          </span>
          <span className="app-console-brand-copy">
            <Typography.Text className="app-console-title">OpenClarion</Typography.Text>
            <Typography.Text className="app-console-context">{t("brandContext")}</Typography.Text>
          </span>
        </Link>
        <Menu
          aria-label={t("navigation")}
          className="app-console-nav"
          inlineCollapsed={isCollapsed}
          items={menuItems}
          mode="inline"
          onClick={() => setMobileOpen(false)}
          selectedKeys={[current]}
        />
      </>
    );
  }

  return (
    <Layout className="app-console-shell">
      <Layout.Sider
        className="app-console-sider"
        collapsed={collapsed}
        collapsedWidth={68}
        theme="light"
        trigger={null}
        width={232}
      >
        <div className="app-console-sider-inner">
          <div className="app-console-navigation">{renderNavigation(collapsed)}</div>
          <Tooltip placement="right" title={collapsed ? t("showNavigation") : t("hideNavigation")}>
            <Button
              aria-label={collapsed ? t("showNavigation") : t("hideNavigation")}
              className="app-console-collapse"
              icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              onClick={() => setCollapsed((currentValue) => !currentValue)}
              type="text"
            >
              {collapsed ? null : t("hideNavigation")}
            </Button>
          </Tooltip>
        </div>
      </Layout.Sider>

      <Drawer
        className="app-console-mobile-drawer"
        destroyOnHidden
        onClose={() => setMobileOpen(false)}
        open={mobileOpen}
        placement="left"
        styles={{ body: { padding: 0 } }}
        title={t("navigationTitle")}
        width={280}
      >
        <div className="app-console-mobile-navigation">{renderNavigation(false)}</div>
      </Drawer>

      <Layout className="app-console-main">
        <Layout.Header className="app-console-header">
          <Button
            aria-label={t("openNavigation")}
            className="app-console-mobile-trigger"
            icon={<MenuOutlined />}
            onClick={() => setMobileOpen(true)}
            type="text"
          />
          <div className="app-console-location">
            <span className="app-console-location-section">{t(currentItem.labelKey)}</span>
            <span className="app-console-location-context">{t(currentItem.sectionLabelKey)}</span>
          </div>
          <div className="app-console-session">
            <LocaleSwitcher />
            <Suspense fallback={<ConsoleSessionControlFallback />}>
              <ConsoleSessionControl />
            </Suspense>
          </div>
        </Layout.Header>
        <Layout.Content className="app-console-content">{children}</Layout.Content>
      </Layout>
    </Layout>
  );
}

function consoleSectionForPathname(pathname: string): ConsoleSection {
  if (pathname.startsWith("/settings/report-workflow-policies")) {
    return "workflow";
  }
  if (pathname.startsWith("/settings/report-workflow-schedules")) {
    return "schedules";
  }
  if (pathname.startsWith("/settings/alert-sources")) {
    return "sources";
  }
  if (pathname.startsWith("/settings/grouping-policies")) {
    return "grouping";
  }
  if (pathname.startsWith("/settings/diagnosis-tool-templates")) {
    return "tools";
  }
  if (pathname.startsWith("/settings/notification-channels")) {
    return "channels";
  }
  if (pathname.startsWith("/settings/directory-rbac")) {
    return "directory-rbac";
  }
  if (pathname.startsWith("/settings/workspaces")) {
    return "workspaces";
  }
  if (pathname.startsWith("/settings")) {
    return "settings";
  }
  if (pathname.startsWith("/diagnosis-room")) {
    return "diagnosis";
  }
  if (pathname.startsWith("/reports")) {
    return "reports";
  }
  if (pathname.startsWith("/alerts")) {
    return "alerts";
  }
  return "dashboard";
}
