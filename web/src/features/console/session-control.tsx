"use client";

import {
  ApartmentOutlined,
  CheckOutlined,
  DownOutlined,
  LoadingOutlined,
  LoginOutlined,
  LogoutOutlined,
  ReloadOutlined,
  UserOutlined,
  WarningOutlined,
} from "@ant-design/icons";
import {
  App as AntdApp,
  Avatar,
  Button,
  Dropdown,
  Space,
  Tooltip,
  Typography,
} from "antd";
import type { MenuProps } from "antd";
import { usePathname, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useMemo } from "react";

import { diagnosisOIDCLoginHref } from "@/features/diagnosis-room/oidc-login";
import { useClientReady } from "@/lib/react/use-client-ready";

import {
  consoleSessionLoginErrorKey,
  consoleSessionModeLabel,
  consoleSessionReturnTo,
  consoleSessionRolesLabel,
} from "./session-state";
import {
  useAccessibleTenantsQuery,
  useClearConsoleBrowserSession,
  useConsoleBrowserSessionQuery,
  useSwitchConsoleTenant,
} from "./use-browser-session";

export function ConsoleSessionControl() {
  const { message } = AntdApp.useApp();
  const t = useTranslations("Session");
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const clientReady = useClientReady();
  const browserSessionQuery = useConsoleBrowserSessionQuery();
  const clearBrowserSessionMutation = useClearConsoleBrowserSession();
  const authenticated =
    browserSessionQuery.data?.ok === true &&
    browserSessionQuery.data.data.authenticated;
  const accessibleTenantsQuery = useAccessibleTenantsQuery(authenticated);
  const switchTenantMutation = useSwitchConsoleTenant();
  const loginHref = useMemo(
    () =>
      diagnosisOIDCLoginHref(
        consoleSessionReturnTo(pathname, searchParams.toString()),
      ),
    [pathname, searchParams],
  );
  const result = browserSessionQuery.data;
  const loginErrorKey = consoleSessionLoginErrorKey(
    searchParams.get("oidc_auth_error"),
  );
  const loginError = loginErrorKey === null ? "" : t(loginErrorKey);

  async function refreshSession() {
    const refreshed = await browserSessionQuery.refetch();
    if (refreshed.data?.ok === false) {
      message.error(refreshed.data.error.message);
      return;
    }
    message.success(t("refreshed"));
  }

  function signOut() {
    clearBrowserSessionMutation.mutate(undefined, {
      onError: (error) => message.error(error.message),
      onSuccess: () => message.success(t("signedOut")),
    });
  }

  function switchTenant(tenantKey: string) {
    switchTenantMutation.mutate(tenantKey, {
      onError: (error) => message.error(error.message),
      onSuccess: (session) =>
        message.success(
          t("switched", { tenant: session.tenant_key }),
        ),
    });
  }

  if (!clientReady || (result === undefined && browserSessionQuery.isPending)) {
    return (
      <Button
        aria-label={t("checkingLabel")}
        className="app-console-session-loading"
        disabled
        icon={<LoadingOutlined spin />}
        type="text"
      >
        <span className="app-console-session-loading-label">
          {t("checking")}
        </span>
      </Button>
    );
  }

  if (result?.ok === true && result.data.authenticated) {
    const session = result.data;
    const sessionTenantID = session.tenant_id;
    const sessionTenantKey = session.tenant_key;
    const tenantResult = accessibleTenantsQuery.data;
    const tenants =
      tenantResult?.ok === true
        ? tenantResult.data.items.filter((tenant) => tenant.status === "active")
        : [];
    const tenantLoadFailed =
      tenantResult?.ok === false || accessibleTenantsQuery.isError;
    const currentTenant = tenants.find(
      (tenant) => tenant.id === sessionTenantID,
    );
    const rolesLabel = consoleSessionRolesLabel(session.roles, t("noRoles"));
    const items: MenuProps["items"] = [
      {
        disabled: true,
        key: "identity",
        label: (
          <div className="app-console-session-identity">
            <Typography.Text ellipsis strong title={session.subject}>
              {session.subject}
            </Typography.Text>
            <Typography.Text type="secondary">
              {t("authentication", {
                mode: consoleSessionModeLabel(session.mode, t("unknownMode")),
              })}
            </Typography.Text>
            <Typography.Text
              ellipsis
              title={currentTenant?.name ?? sessionTenantKey}
            >
              {currentTenant?.name ?? sessionTenantKey}
            </Typography.Text>
            <Typography.Text
              ellipsis
              title={rolesLabel}
              type="secondary"
            >
              {rolesLabel}
            </Typography.Text>
          </div>
        ),
      },
      { type: "divider" },
      {
        children:
          accessibleTenantsQuery.isFetching && tenants.length === 0
            ? [
                {
                  disabled: true,
                  icon: <LoadingOutlined spin />,
                  key: "tenant-loading",
                  label: t("loadingWorkspaces"),
                },
              ]
            : tenantLoadFailed || tenants.length === 0
              ? [
                  {
                    disabled: true,
                    icon: <WarningOutlined />,
                    key: "tenant-unavailable",
                    label: tenantLoadFailed
                      ? t("workspacesUnavailable")
                      : t("noWorkspaces"),
                  },
                ]
              : tenants.map((tenant) => ({
                  disabled:
                    tenant.id === sessionTenantID ||
                    switchTenantMutation.isPending,
                  icon:
                    tenant.id === sessionTenantID ? (
                      <CheckOutlined />
                    ) : undefined,
                  key: `tenant:${tenant.key}`,
                  label: tenant.name,
                })),
        icon: <ApartmentOutlined />,
        key: "tenants",
        label: currentTenant?.name ?? sessionTenantKey,
      },
      {
        disabled: browserSessionQuery.isFetching,
        icon: <ReloadOutlined spin={browserSessionQuery.isFetching} />,
        key: "refresh",
        label: t("refresh"),
      },
      {
        danger: true,
        disabled: clearBrowserSessionMutation.isPending,
        icon: <LogoutOutlined />,
        key: "sign-out",
        label: t("signOut"),
      },
    ];
    const handleMenuClick: MenuProps["onClick"] = ({ key }) => {
      if (key === "refresh") {
        void refreshSession();
      }
      if (key === "sign-out") {
        signOut();
      }
      if (key.startsWith("tenant:")) {
        const tenantKey = key.slice("tenant:".length);
        if (tenantKey !== sessionTenantKey) {
          switchTenant(tenantKey);
        }
      }
    };

    return (
      <Dropdown
        menu={{ items, onClick: handleMenuClick }}
        placement="bottomRight"
        trigger={["click"]}
      >
        <Button
          aria-label={t("accountMenu", { subject: session.subject })}
          className="app-console-session-trigger"
          icon={
            <Avatar
              className="app-console-session-avatar"
              icon={<UserOutlined />}
              size={24}
            />
          }
          type="text"
        >
          <span className="app-console-session-subject">{session.subject}</span>
          <DownOutlined
            aria-hidden
            className="app-console-session-chevron"
          />
        </Button>
      </Dropdown>
    );
  }

  const sessionStatusError =
    result?.ok === false
      ? result.error.message
      : browserSessionQuery.error instanceof Error
        ? browserSessionQuery.error.message
        : "";
  const statusDetail =
    sessionStatusError === ""
      ? loginError
      : t("unavailable", { error: sessionStatusError });

  return (
    <Space className="app-console-session-anonymous" size={2}>
      <Tooltip
        title={
          statusDetail === ""
            ? t("signInHint")
            : statusDetail
        }
      >
        <Button
          aria-label={t("signInLabel")}
          href={loginHref}
          icon={statusDetail === "" ? <LoginOutlined /> : <WarningOutlined />}
          type="primary"
        >
          <span className="app-console-session-sign-in-label">{t("signIn")}</span>
        </Button>
      </Tooltip>
      {sessionStatusError === "" ? null : (
        <Tooltip title={t("retry")}>
          <Button
            aria-label={t("retry")}
            icon={<ReloadOutlined />}
            loading={browserSessionQuery.isFetching}
            onClick={() => void refreshSession()}
            type="text"
          />
        </Tooltip>
      )}
    </Space>
  );
}

export function ConsoleSessionControlFallback() {
  const t = useTranslations("Session");
  return (
    <Button
      aria-label={t("loadingLabel")}
      className="app-console-session-loading"
      disabled
      icon={<LoadingOutlined spin />}
      type="text"
    />
  );
}
