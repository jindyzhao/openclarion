"use client";

import {
  EditOutlined,
  PlusOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  TeamOutlined,
} from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Statistic,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import type { TableColumnsType } from "antd";
import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";

import {
  accessibleTenantsQueryKey,
  useConsoleBrowserSessionQuery,
} from "@/features/console/use-browser-session";
import type { ApiResult } from "@/lib/api/client";

import { formatDateTime } from "../format";
import {
  settingsErrorMessage,
  type SettingsNotice,
  useSettingsList,
  useSettingsMutation,
} from "../query-state";
import {
  refreshTenantMemberships,
  refreshTenants,
  submitTenant,
  submitTenantMembership,
  submitTenantStatus,
} from "./client-api";
import {
  membershipDisableBlocked,
  selectedWorkspaceID,
  workspaceStatusChangeBlocked,
} from "./selection";
import type {
  Tenant,
  TenantCreateRequest,
  TenantListResponse,
  TenantMembership,
  TenantMembershipListResponse,
  TenantMembershipWriteRequest,
  TenantStatusUpdateRequest,
} from "./types";

type WorkspaceSettingsManagerProps = {
  result: ApiResult<TenantListResponse>;
};

type CreateWorkspaceForm = {
  key: string;
  name: string;
};

type MembershipForm = {
  enabled: boolean;
  role: TenantMembershipWriteRequest["role"];
  subject: string;
};

type StatusMutationVariables = {
  body: TenantStatusUpdateRequest;
  tenantID: number;
};

type MembershipMutationVariables = {
  body: TenantMembershipWriteRequest;
  tenantID: number;
};

type WorkspaceTranslator = ReturnType<typeof useTranslations<"WorkspaceSettings">>;

const tenantQueryKey = ["settings", "workspaces", "tenants"] as const;
const membershipQueryPrefix = [
  "settings",
  "workspaces",
  "memberships",
] as const;

function membershipQueryKey(tenantID: number) {
  return [...membershipQueryPrefix, tenantID] as const;
}

export function WorkspaceSettingsManager({
  result,
}: WorkspaceSettingsManagerProps) {
  const locale = useLocale();
  const t = useTranslations("WorkspaceSettings");
  const [createForm] = Form.useForm<CreateWorkspaceForm>();
  const [membershipForm] = Form.useForm<MembershipForm>();
  const [createOpen, setCreateOpen] = useState(false);
  const [membershipOpen, setMembershipOpen] = useState(false);
  const [editingMembership, setEditingMembership] =
    useState<TenantMembership | null>(null);
  const [selectedTenantID, setSelectedTenantID] = useState<number | null>(null);
  const [actionNotice, setActionNotice] = useState<SettingsNotice | null>(null);
  const {
    items: tenants,
    notice: listNotice,
    query: tenantQuery,
    refresh,
  } = useSettingsList({
    initialResult: result,
    queryKey: tenantQueryKey,
    queryFn: refreshTenants,
    refreshMessage: t("refreshed"),
    selectItems: (response) => response.items,
  });
  const sessionQuery = useConsoleBrowserSessionQuery();
  const authenticatedSession =
    sessionQuery.data?.ok === true && sessionQuery.data.data.authenticated
      ? sessionQuery.data.data
      : null;
  const currentTenantID = authenticatedSession?.tenant_id ?? null;
  const currentSubject = authenticatedSession?.subject ?? null;
  const sessionTenantKnown =
    sessionQuery.data?.ok === true && !sessionQuery.isFetching;
  const effectiveSelectedTenantID = selectedWorkspaceID(
    tenants,
    selectedTenantID,
    currentTenantID,
  );
  const selectedTenant = tenants.find(
    (tenant) => tenant.id === effectiveSelectedTenantID,
  );
  const membershipsQuery = useQuery({
    enabled: effectiveSelectedTenantID !== null,
    queryFn: () => loadMemberships(effectiveSelectedTenantID as number),
    queryKey: membershipQueryKey(effectiveSelectedTenantID ?? 0),
    retry: false,
  });
  const createMutation = useSettingsMutation<TenantCreateRequest, Tenant>({
    invalidateQueryKeys: [tenantQueryKey, accessibleTenantsQueryKey],
    mutationFn: submitTenant,
  });
  const statusMutation = useSettingsMutation<StatusMutationVariables, Tenant>({
    invalidateQueryKeys: [tenantQueryKey, accessibleTenantsQueryKey],
    mutationFn: ({ body, tenantID }) => submitTenantStatus(tenantID, body),
  });
  const membershipMutation = useSettingsMutation<
    MembershipMutationVariables,
    TenantMembership
  >({
    invalidateQueryKey: membershipQueryPrefix,
    mutationFn: ({ body, tenantID }) =>
      submitTenantMembership(tenantID, body),
  });

  const metrics = useMemo(
    () => ({
      active: tenants.filter((tenant) => tenant.status === "active").length,
      disabled: tenants.filter((tenant) => tenant.status === "disabled")
        .length,
      total: tenants.length,
    }),
    [tenants],
  );
  const memberships = membershipsQuery.data?.items ?? [];
  const ownerCount = memberships.filter(
    (membership) => membership.enabled && membership.role === "owner",
  ).length;
  const visibleNotice = actionNotice ?? listNotice;
  const busy =
    createMutation.isPending ||
    statusMutation.isPending ||
    tenantQuery.isFetching;

  async function handleRefresh() {
    setActionNotice(null);
    await refresh();
  }

  async function handleCreate(values: CreateWorkspaceForm) {
    try {
      const created = await createMutation.mutateAsync({
        key: values.key.trim(),
        name: values.name.trim(),
      });
      setSelectedTenantID(created.id);
      setCreateOpen(false);
      createForm.resetFields();
      setActionNotice({ kind: "info", message: t("created") });
    } catch (error) {
      setActionNotice({
        kind: "error",
        message: settingsErrorMessage(error),
      });
    }
  }

  async function handleStatusChange(tenant: Tenant) {
    const status = tenant.status === "active" ? "disabled" : "active";
    try {
      await statusMutation.mutateAsync({ tenantID: tenant.id, body: { status } });
      setActionNotice({
        kind: "info",
        message: status === "active" ? t("enabledNotice") : t("disabledNotice"),
      });
    } catch (error) {
      setActionNotice({
        kind: "error",
        message: settingsErrorMessage(error),
      });
    }
  }

  function openMembershipEditor(membership: TenantMembership | null) {
    setEditingMembership(membership);
    membershipForm.setFields([{ name: "enabled", errors: [] }]);
    membershipForm.setFieldsValue(
      membership === null
        ? { enabled: true, role: "member", subject: "" }
        : {
            enabled: membership.enabled,
            role: membership.role,
            subject: membership.subject,
          },
    );
    setMembershipOpen(true);
  }

  async function handleMembership(values: MembershipForm) {
    if (effectiveSelectedTenantID === null) {
      return;
    }
    const body = {
      enabled: values.enabled,
      role: values.role,
      subject: values.subject.trim(),
    };
    if (
      membershipDisableBlocked({
        currentSubject,
        currentTenantID,
        enabled: body.enabled,
        selectedTenantID: effectiveSelectedTenantID,
        sessionTenantKnown,
        subject: body.subject,
      })
    ) {
      membershipForm.setFields([
        {
          name: "enabled",
          errors: [
            t(
              sessionTenantKnown
                ? "activeMembershipRequired"
                : "verifySessionBeforeMembership",
            ),
          ],
        },
      ]);
      return;
    }
    try {
      await membershipMutation.mutateAsync({
        tenantID: effectiveSelectedTenantID,
        body,
      });
      setMembershipOpen(false);
      setEditingMembership(null);
      membershipForm.resetFields();
      setActionNotice({ kind: "info", message: t("membershipSaved") });
    } catch (error) {
      setActionNotice({
        kind: "error",
        message: settingsErrorMessage(error),
      });
    }
  }

  return (
    <div className="stack">
      <Row aria-label={t("metricsLabel")} gutter={[12, 12]}>
        <Metric label={t("visible")} value={metrics.total} />
        <Metric label={t("active")} value={metrics.active} />
        <Metric label={t("disabled")} value={metrics.disabled} />
        <Metric label={t("selectedOwners")} value={ownerCount} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} /> : null}

      <Row align="top" gutter={[16, 16]}>
        <Col lg={10} xs={24}>
          <Card
            extra={
              <Space>
                <Button
                  aria-label={t("refresh")}
                  icon={<ReloadOutlined />}
                  loading={tenantQuery.isFetching}
                  onClick={() => void handleRefresh()}
                />
                <Button
                  icon={<PlusOutlined />}
                  onClick={() => {
                    setActionNotice(null);
                    setCreateOpen(true);
                  }}
                  type="primary"
                >
                  {t("newWorkspace")}
                </Button>
              </Space>
            }
            title={t("workspaces")}
          >
            <WorkspaceTable
              busy={busy}
              currentTenantID={currentTenantID}
              onSelect={setSelectedTenantID}
              onStatusChange={(tenant) => void handleStatusChange(tenant)}
              selectedTenantID={effectiveSelectedTenantID}
              sessionTenantKnown={sessionTenantKnown}
              t={t}
              tenants={tenants}
            />
          </Card>
        </Col>

        <Col lg={14} xs={24}>
          <Card
            extra={
              <Button
                disabled={selectedTenant === undefined}
                icon={<PlusOutlined />}
                onClick={() => openMembershipEditor(null)}
                type="primary"
              >
                {t("addMember")}
              </Button>
            }
            title={
              selectedTenant === undefined
                ? t("members")
                : t("workspaceMembers", { name: selectedTenant.name })
            }
          >
            {membershipsQuery.error ? (
              <Alert
                message={settingsErrorMessage(membershipsQuery.error)}
                role="alert"
                showIcon
                type="warning"
              />
            ) : (
              <MembershipTable
                loading={
                  membershipsQuery.isFetching || membershipMutation.isPending
                }
                memberships={memberships}
                onEdit={openMembershipEditor}
                locale={locale}
                t={t}
              />
            )}
          </Card>
        </Col>
      </Row>

      <Modal
        destroyOnHidden
        okButtonProps={{ loading: createMutation.isPending }}
        okText={t("create")}
        onCancel={() => setCreateOpen(false)}
        onOk={() => createForm.submit()}
        open={createOpen}
        title={t("newWorkspace")}
      >
        <Form<CreateWorkspaceForm>
          form={createForm}
          layout="vertical"
          onFinish={(values) => void handleCreate(values)}
        >
          <Form.Item
            label={t("name")}
            name="name"
            rules={[
              { required: true, whitespace: true },
              {
                validator: (_rule, value: unknown) =>
                  typeof value !== "string" || Array.from(value).length <= 120
                    ? Promise.resolve()
                    : Promise.reject(new Error(t("nameLength"))),
              },
            ]}
          >
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item
            label={t("key")}
            name="key"
            rules={[
              { required: true },
              { max: 63 },
              {
                pattern: /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/,
                message: t("keyValidation"),
              },
            ]}
          >
            <Input autoComplete="off" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        destroyOnHidden
        okButtonProps={{ loading: membershipMutation.isPending }}
        okText={t("save")}
        onCancel={() => setMembershipOpen(false)}
        onOk={() => membershipForm.submit()}
        open={membershipOpen}
        title={editingMembership === null ? t("addMember") : t("editMembership")}
      >
        <Form<MembershipForm>
          form={membershipForm}
          layout="vertical"
          onFinish={(values) => void handleMembership(values)}
        >
          <Form.Item
            label={t("subject")}
            name="subject"
            rules={[
              { required: true, whitespace: true },
              { max: 256 },
            ]}
          >
            <Input
              autoComplete="off"
              disabled={editingMembership !== null}
            />
          </Form.Item>
          <Form.Item label={t("role")} name="role" rules={[{ required: true }]}>
            <Select
              options={[
                { label: t("owner"), value: "owner" },
                { label: t("member"), value: "member" },
              ]}
            />
          </Form.Item>
          <Form.Item label={t("enabled")} name="enabled" valuePropName="checked">
            <Switch
              disabled={
                editingMembership !== null &&
                membershipDisableBlocked({
                  currentSubject,
                  currentTenantID,
                  enabled: false,
                  selectedTenantID: effectiveSelectedTenantID,
                  sessionTenantKnown,
                  subject: editingMembership.subject,
                })
              }
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}

function WorkspaceTable({
  busy,
  currentTenantID,
  onSelect,
  onStatusChange,
  selectedTenantID,
  sessionTenantKnown,
  t,
  tenants,
}: {
  busy: boolean;
  currentTenantID: number | null;
  onSelect: (tenantID: number) => void;
  onStatusChange: (tenant: Tenant) => void;
  selectedTenantID: number | null;
  sessionTenantKnown: boolean;
  t: WorkspaceTranslator;
  tenants: Tenant[];
}) {
  const columns: TableColumnsType<Tenant> = [
    {
      key: "workspace",
      title: t("workspace"),
      render: (_value, tenant) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>{tenant.name}</Typography.Text>
          <Typography.Text type="secondary">{tenant.key}</Typography.Text>
        </Space>
      ),
    },
    {
      dataIndex: "status",
      key: "status",
      title: t("status"),
      render: (status: Tenant["status"]) => (
        <Tag color={status === "active" ? "green" : "default"}>
          {status === "active" ? t("active") : t("disabled")}
        </Tag>
      ),
    },
    {
      key: "actions",
      title: t("actions"),
      render: (_value, tenant) => {
        const statusBlocked = workspaceStatusChangeBlocked(
          tenant,
          currentTenantID,
          sessionTenantKnown,
        );
        return (
          <Space wrap>
            <Button
              icon={<TeamOutlined />}
              onClick={() => onSelect(tenant.id)}
              type="link"
            >
              {t("members")}
            </Button>
            <Popconfirm
              disabled={busy || statusBlocked}
              okText={tenant.status === "active" ? t("disable") : t("enable")}
              onConfirm={() => onStatusChange(tenant)}
              title={tenant.status === "active"
                ? t("disableConfirm", { name: tenant.name })
                : t("enableConfirm", { name: tenant.name })}
            >
              <Button
                danger={tenant.status === "active"}
                disabled={busy || statusBlocked}
                icon={<PoweroffOutlined />}
                type="link"
              >
                {tenant.status === "active" ? t("disable") : t("enable")}
              </Button>
            </Popconfirm>
          </Space>
        );
      },
    },
  ];
  return (
    <Table<Tenant>
      columns={columns}
      dataSource={tenants}
      loading={busy}
      locale={{ emptyText: <Empty description={t("noWorkspaces")} /> }}
      onRow={(tenant) => ({ onClick: () => onSelect(tenant.id) })}
      pagination={false}
      rowClassName={(tenant) =>
        tenant.id === selectedTenantID ? "settings-table-row-focus" : ""
      }
      rowKey="id"
      scroll={{ x: 620 }}
      size="small"
    />
  );
}

function MembershipTable({
  locale,
  loading,
  memberships,
  onEdit,
  t,
}: {
  locale: string;
  loading: boolean;
  memberships: TenantMembership[];
  onEdit: (membership: TenantMembership) => void;
  t: WorkspaceTranslator;
}) {
  const columns: TableColumnsType<TenantMembership> = [
    {
      dataIndex: "subject",
      key: "subject",
      title: t("subject"),
      render: (subject: string) => (
        <Typography.Text ellipsis={{ tooltip: subject }}>
          {subject}
        </Typography.Text>
      ),
    },
    {
      dataIndex: "role",
      key: "role",
      title: t("role"),
      render: (role: TenantMembership["role"]) => (
        <Tag color={role === "owner" ? "gold" : "blue"}>
          {role === "owner" ? t("owner") : t("member")}
        </Tag>
      ),
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: t("status"),
      render: (enabled: boolean) => (
        <Tag color={enabled ? "green" : "default"}>
          {enabled ? t("enabled") : t("disabled")}
        </Tag>
      ),
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: t("updated"),
      render: (updatedAt: string) => formatDateTime(updatedAt, locale),
    },
    {
      key: "action",
      title: t("action"),
      render: (_value, membership) => (
        <Button
          icon={<EditOutlined />}
          onClick={() => onEdit(membership)}
          type="link"
        >
          {t("edit")}
        </Button>
      ),
    },
  ];
  return (
    <Table<TenantMembership>
      columns={columns}
      dataSource={memberships}
      loading={loading}
      locale={{ emptyText: <Empty description={t("noMemberships")} /> }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 720 }}
      size="small"
    />
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <Col lg={6} sm={12} xs={24}>
      <Card className="settings-stat-card" size="small">
        <Statistic title={label} value={value} />
      </Card>
    </Col>
  );
}

function Notice({ notice }: { notice: SettingsNotice }) {
  return (
    <Alert
      message={notice.message}
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={notice.kind}
    />
  );
}

async function loadMemberships(
  tenantID: number,
): Promise<TenantMembershipListResponse> {
  const result = await refreshTenantMemberships(tenantID);
  if (!result.ok) {
    throw new Error(result.error.message);
  }
  return result.data;
}
