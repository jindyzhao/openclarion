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
  Form,
  Input,
  Modal,
  Space,
  Tooltip,
  Typography,
} from "antd";
import type { MenuProps } from "antd";
import { useQueryClient } from "@tanstack/react-query";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useMemo, useState } from "react";

import {
  normalizedDiagnosisAuthorization,
  type DiagnosisAuthorization,
} from "@/features/diagnosis-room/authorization";
import { diagnosisOIDCLoginHref } from "@/features/diagnosis-room/oidc-login";
import { createDiagnosisBrowserSession } from "@/features/diagnosis-room/transport";
import { useClientReady } from "@/lib/react/use-client-ready";

import {
  consoleSessionLoginErrorKey,
  consoleSessionLoginMode,
  consoleSessionModeLabel,
  consoleSessionRefreshFailure,
  consoleSessionReturnTo,
  consoleSessionRolesLabel,
  replaceConsoleQueryCacheAfterAuthentication,
} from "./session-state";
import {
  useAccessibleTenantsQuery,
  useClearConsoleBrowserSession,
  useConsoleAuthStatusQuery,
  useConsoleBrowserSessionQuery,
  useSwitchConsoleTenant,
} from "./use-browser-session";

type ConsoleSignInFormValues = {
  bearerToken?: string;
  ldapPassword?: string;
  ldapUsername?: string;
};

export function ConsoleSessionControl() {
  const { message } = AntdApp.useApp();
  const commonT = useTranslations("Common");
  const t = useTranslations("Session");
  const [signInForm] = Form.useForm<ConsoleSignInFormValues>();
  const queryClient = useQueryClient();
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const clientReady = useClientReady();
  const [signInOpen, setSignInOpen] = useState(false);
  const [signInPending, setSignInPending] = useState(false);
  const browserSessionQuery = useConsoleBrowserSessionQuery();
  const clearBrowserSessionMutation = useClearConsoleBrowserSession();
  const authenticated =
    browserSessionQuery.data?.ok === true &&
    browserSessionQuery.data.data.authenticated;
  const authStatusQuery = useConsoleAuthStatusQuery(!authenticated);
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
    const refreshedSession = await browserSessionQuery.refetch();
    if (refreshedSession.error !== null) {
      message.error(
        t("unavailable", {
          error:
            refreshedSession.error instanceof Error
              ? refreshedSession.error.message
              : commonT("requestFailed"),
        }),
      );
      return;
    }
    const refreshedAuthStatus =
      refreshedSession.data?.ok === true &&
      !refreshedSession.data.data.authenticated
        ? await authStatusQuery.refetch()
        : undefined;
    if (
      refreshedAuthStatus !== undefined &&
      refreshedAuthStatus.error !== null
    ) {
      message.error(
        t("authStatusUnavailable", {
          error:
            refreshedAuthStatus.error instanceof Error
              ? refreshedAuthStatus.error.message
              : commonT("requestFailed"),
        }),
      );
      return;
    }
    const failure = consoleSessionRefreshFailure(
      refreshedSession.data,
      refreshedAuthStatus?.data,
    );
    if (failure !== null) {
      const error = failure.error ?? commonT("requestFailed");
      message.error(
        failure.source === "auth-status"
          ? t("authStatusUnavailable", { error })
          : t("unavailable", { error }),
      );
      return;
    }
    message.success(t("refreshed"));
  }

  function closeSignIn() {
    if (signInPending) {
      return;
    }
    signInForm.resetFields();
    setSignInOpen(false);
  }

  async function signIn(values: ConsoleSignInFormValues) {
    const status = authStatusQuery.data;
    const loginMode = consoleSessionLoginMode(
      status?.ok === true ? status.data : undefined,
    );
    let authorization: DiagnosisAuthorization | null = null;
    if (loginMode === "ldap") {
      authorization = normalizedDiagnosisAuthorization({
        mode: "basic",
        password: values.ldapPassword ?? "",
        username: values.ldapUsername ?? "",
      });
    }
    if (loginMode === "static") {
      authorization = normalizedDiagnosisAuthorization({
        mode: "bearer",
        token: values.bearerToken ?? "",
      });
    }
    if (authorization === null) {
      message.error(t("credentialsInvalid"));
      return;
    }

    setSignInPending(true);
    try {
      const result = await createDiagnosisBrowserSession(authorization);
      if (!result.ok) {
        message.error(t("signInFailed", { error: result.error.message }));
        return;
      }
      if (!result.data.authenticated) {
        message.error(t("sessionNotReturned"));
        return;
      }
      await replaceConsoleQueryCacheAfterAuthentication(
        queryClient,
        result.data,
      );
      signInForm.resetFields();
      setSignInOpen(false);
      router.refresh();
      message.success(t("signedIn", { subject: result.data.subject }));
    } finally {
      setSignInPending(false);
    }
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
  const authStatusError =
    authStatusQuery.data?.ok === false
      ? authStatusQuery.data.error.message
      : authStatusQuery.error instanceof Error
        ? authStatusQuery.error.message
        : "";
  const loginMode = consoleSessionLoginMode(
    authStatusQuery.data?.ok === true
      ? authStatusQuery.data.data
      : undefined,
  );
  const signInHint =
    loginMode === "ldap"
      ? t("signInHints.ldap")
      : loginMode === "static"
        ? t("signInHints.static")
        : loginMode === "oidc"
          ? t("signInHints.oidc")
          : t("signInHints.unavailable");
  const statusDetail =
    sessionStatusError === ""
      ? loginError === ""
        ? authStatusError === ""
          ? ""
          : t("authStatusUnavailable", { error: authStatusError })
        : loginError
      : t("unavailable", { error: sessionStatusError });
  const fallbackSignIn = loginMode === "ldap" || loginMode === "static";
  const signInDisabled =
    loginMode === "unavailable" || authStatusQuery.isPending;

  return (
    <>
      <Space className="app-console-session-anonymous" size={2}>
        <Tooltip title={statusDetail === "" ? signInHint : statusDetail}>
          <Button
            aria-label={t("signInLabel")}
            disabled={signInDisabled}
            href={loginMode === "oidc" ? loginHref : undefined}
            icon={
              statusDetail === "" && !signInDisabled ? (
                <LoginOutlined />
              ) : (
                <WarningOutlined />
              )
            }
            loading={authStatusQuery.isPending}
            onClick={
              fallbackSignIn ? () => setSignInOpen(true) : undefined
            }
            type="primary"
          >
            <span className="app-console-session-sign-in-label">
              {t("signIn")}
            </span>
          </Button>
        </Tooltip>
        {sessionStatusError === "" && authStatusError === "" ? null : (
          <Tooltip title={t("retry")}>
            <Button
              aria-label={t("retry")}
              icon={<ReloadOutlined />}
              loading={
                browserSessionQuery.isFetching || authStatusQuery.isFetching
              }
              onClick={() => void refreshSession()}
              type="text"
            />
          </Tooltip>
        )}
      </Space>
      <Modal
        cancelButtonProps={{ disabled: signInPending }}
        cancelText={t("cancel")}
        destroyOnHidden
        maskClosable={!signInPending}
        okButtonProps={{ loading: signInPending }}
        okText={t("signIn")}
        onCancel={closeSignIn}
        onOk={() => signInForm.submit()}
        open={signInOpen && fallbackSignIn}
        title={
          loginMode === "ldap"
            ? t("ldapSignInTitle")
            : t("staticSignInTitle")
        }
      >
        <Form<ConsoleSignInFormValues>
          form={signInForm}
          layout="vertical"
          onFinish={(values) => void signIn(values)}
          preserve={false}
        >
          {loginMode === "ldap" ? (
            <>
              <Form.Item
                label={t("ldapUsername")}
                name="ldapUsername"
                rules={[
                  { required: true, whitespace: true },
                  { max: 256 },
                  {
                    pattern: /^[^\s:]+$/,
                    message: t("ldapUsernameInvalid"),
                  },
                ]}
              >
                <Input autoComplete="username" disabled={signInPending} />
              </Form.Item>
              <Form.Item
                label={t("ldapPassword")}
                name="ldapPassword"
                rules={[
                  { required: true },
                  { max: 4096 },
                  {
                    pattern: /^[^\u0000\r\n]+$/,
                    message: t("ldapPasswordInvalid"),
                  },
                ]}
              >
                <Input.Password
                  autoComplete="current-password"
                  disabled={signInPending}
                />
              </Form.Item>
            </>
          ) : (
            <Form.Item
              label={t("bearerToken")}
              name="bearerToken"
              rules={[
                { required: true, whitespace: true },
                { max: 4096 },
                {
                  pattern: /^\S+$/,
                  message: t("bearerTokenInvalid"),
                },
              ]}
            >
              <Input.Password autoComplete="off" disabled={signInPending} />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </>
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
