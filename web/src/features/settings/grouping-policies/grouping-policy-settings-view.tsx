"use client";

import {
  EditOutlined,
  PartitionOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  Empty,
  Form,
  Input,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
  Typography
} from "antd";
import type { TableColumnsType } from "antd";
import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

import { formatDateTime } from "../format";
import {
  settingsErrorMessage,
  settingsManagePermissionNotice,
  settingsReadPermissionEmptyDescription,
  settingsReadPermissionNotice,
  type SettingsNotice,
  useClientReady,
  useSettingsList,
  useSettingsMutation
} from "../query-state";
import { ReadOnlyModeAlert } from "../permission-notice";
import {
  useCurrentRBACAuthorizations,
  type CurrentRBACAuthorizationCheck
} from "../rbac-capabilities";
import { refreshGroupingPolicies, runGroupingPolicyPreview, submitGroupingPolicy } from "./client-api";
import {
  emptyGroupingPolicyForm,
  formStateToWriteRequest,
  groupingPolicyLaunchInitialForm,
  policyToFormState,
  type GroupingPolicyLaunchIntent,
  type GroupingPolicyValidationError
} from "./format";
import type {
  GroupingPolicy,
  GroupingPolicyFormState,
  GroupingPolicyListResponse,
  GroupingPolicyPreviewGroup,
  GroupingPolicyPreviewResult,
  GroupingPolicyWriteRequest
} from "./types";

type GroupingPolicySettingsManagerProps = {
  launchIntent?: GroupingPolicyLaunchIntent | null;
  result: ApiResult<GroupingPolicyListResponse>;
};

const groupingPoliciesQueryKey = ["settings", "grouping-policies"] as const;

type SavePolicyVariables = {
  body: GroupingPolicyWriteRequest;
  policyID: number | null;
};

type GroupingTranslator = ReturnType<typeof useTranslations<"GroupingSettings">>;

const groupingPolicyBaseAuthorizationChecks: CurrentRBACAuthorizationCheck[] = [
  { key: "groupingPolicyRead", permission: "grouping_policy.read" },
  { key: "groupingPolicyManage", permission: "grouping_policy.manage" }
];

export function GroupingPolicySettingsManager({
  launchIntent = null,
  result
}: GroupingPolicySettingsManagerProps) {
  const locale = useLocale();
  const t = useTranslations("GroupingSettings");
  const common = useTranslations("Common");
  const [form] = Form.useForm<GroupingPolicyFormState>();
  const clientReady = useClientReady();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [previewingID, setPreviewingID] = useState<number | null>(null);
  const [previewResults, setPreviewResults] = useState<Record<number, GroupingPolicyPreviewResult>>({});
  const [selectedPreviewID, setSelectedPreviewID] = useState<number | null>(null);
  const [launchNotice, setLaunchNotice] = useState<string | null>(
    launchIntent?.message ?? null,
  );
  const {
    errorStatus,
    items: policies,
    notice,
    query,
    refresh,
    setNotice
  } = useSettingsList({
    initialResult: result,
    queryKey: groupingPoliciesQueryKey,
    queryFn: refreshGroupingPolicies,
    refreshMessage: t("refreshed"),
    selectItems: (response) => response.items
  });
  const savePolicy = useSettingsMutation<SavePolicyVariables, GroupingPolicy>({
    invalidateQueryKey: groupingPoliciesQueryKey,
    mutationFn: ({ policyID, body }) => submitGroupingPolicy(policyID, body)
  });
  const authorizationChecks = useMemo(
    () => [
      ...groupingPolicyBaseAuthorizationChecks,
      ...policies.flatMap((policy) => [
        {
          key: groupingPolicyReadKey(policy.id),
          permission: "grouping_policy.read" as const,
          scopeKey: String(policy.id),
          scopeKind: "grouping_policy" as const
        },
        {
          key: groupingPolicyManageKey(policy.id),
          permission: "grouping_policy.manage" as const,
          scopeKey: String(policy.id),
          scopeKind: "grouping_policy" as const
        }
      ])
    ],
    [policies]
  );
  const currentAuthorization = useCurrentRBACAuthorizations(
    authorizationChecks,
    clientReady
  );
  const busy =
    !clientReady ||
    currentAuthorization.isChecking ||
    query.isFetching ||
    savePolicy.isPending;
  const canReadGroupingPolicies = currentAuthorization.can("groupingPolicyRead");
  const canCreateGroupingPolicy = currentAuthorization.can("groupingPolicyManage");
  const canSaveCurrentGroupingPolicy =
    editingID === null
      ? canCreateGroupingPolicy
      : currentAuthorization.can(groupingPolicyManageKey(editingID));
  const formPermissionNotice = settingsManagePermissionNotice({
    canManage: canSaveCurrentGroupingPolicy,
    isChecking: !clientReady || currentAuthorization.isChecking,
    message: common("formReadOnly", {
      resource:
        editingID === null
          ? t("creationResource")
          : t("policyResource", { id: editingID }),
    }),
  });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadGroupingPolicies,
    errorStatus,
    isChecking: !clientReady || currentAuthorization.isChecking,
    message: common("readAccessLimited", {
      resource: t("policiesResource"),
    }),
  });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;
  const initialFormValues = useMemo(() => groupingPolicyLaunchInitialForm(launchIntent), [launchIntent]);

  const summary = useMemo(() => {
    const enabled = policies.filter((policy) => policy.enabled).length;
    const scoped = policies.filter((policy) => policy.source_filter.length > 0).length;
    const maxDimensions = policies.reduce((current, policy) => Math.max(current, policy.dimension_keys.length), 0);
    return { enabled, scoped, maxDimensions };
  }, [policies]);

  async function handleRefresh() {
    await refresh();
  }

  async function handleSubmit(values: GroupingPolicyFormState) {
    const parsed = formStateToWriteRequest(normalizeFormValues(values));
    if (!parsed.ok) {
      setNotice({
        kind: "error",
        message: localizeGroupingValidationError(parsed.error, t)
      });
      return;
    }

    try {
      await savePolicy.mutateAsync({ policyID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      return;
    }

    form.setFieldsValue(emptyGroupingPolicyForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({ kind: "info", message: t("saved") });
  }

  async function handlePreview(policy: GroupingPolicy) {
    setPreviewingID(policy.id);
    const previewed = await runGroupingPolicyPreview(policy.id);
    setPreviewingID(null);
    if (!previewed.ok) {
      setNotice({ kind: "error", message: previewed.error.message });
      return;
    }

    setPreviewResults((current) => ({ ...current, [policy.id]: previewed.data }));
    setSelectedPreviewID(policy.id);
    setNotice({
      kind: "info",
      message: t("previewScanned", {
        matched: previewed.data.events_matched,
        scanned: previewed.data.events_scanned,
      })
    });
  }

  function editPolicy(policy: GroupingPolicy) {
    setEditingID(policy.id);
    form.setFieldsValue(policyToFormState(policy));
    setLaunchNotice(null);
    setNotice(null);
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyGroupingPolicyForm());
    setLaunchNotice(null);
    setNotice(null);
  }

  const selectedPreview = selectedPreviewID === null ? undefined : previewResults[selectedPreviewID];

  return (
    <div className="stack">
      <Row aria-label={t("metricsLabel")} gutter={[12, 12]}>
        <MetricCard label={t("policies")} value={policies.length} />
        <MetricCard label={t("enabled")} value={summary.enabled} />
        <MetricCard label={t("sourceScoped")} value={summary.scoped} />
        <MetricCard label={t("maxDimensions")} value={summary.maxDimensions} />
      </Row>

      {visibleNotice ? <Notice notice={visibleNotice} t={t} /> : null}
      {launchNotice ? (
        <Alert
          aria-label={t("launchPreset")}
          description={localizeGroupingMessage(launchNotice, t)}
          message={t("actionLoaded")}
          role="status"
          showIcon
          type="info"
        />
      ) : null}

      <Row align="top" className="settings-console-grid" gutter={[16, 16]}>
        <Col lg={8} md={24} xs={24}>
          <Card
            extra={
              editingID === null ? null : (
                <Button disabled={busy || !canCreateGroupingPolicy} icon={<PlusOutlined />} onClick={resetForm} type="default">
                  {t("new")}
                </Button>
              )
            }
            title={editingID === null ? t("newPolicy") : t("editPolicy", { id: editingID })}
          >
            {formPermissionNotice ? (
              <ReadOnlyModeAlert notice={formPermissionNotice} />
            ) : null}
            <Form<GroupingPolicyFormState>
              disabled={busy || !canSaveCurrentGroupingPolicy}
              form={form}
              initialValues={initialFormValues}
              layout="vertical"
              onFinish={handleSubmit}
            >
              <Form.Item
                label={t("name")}
                name="name"
                rules={[
                  { required: true, message: t("nameRequired") },
                  { max: 120, message: t("nameLength") }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item
                label={t("dimensionKeys")}
                name="dimensionKeysText"
                rules={[{ required: true, message: t("dimensionRequired") }]}
              >
                <Input.TextArea autoSize={{ minRows: 4, maxRows: 8 }} placeholder={"alertname\nservice"} />
              </Form.Item>

              <Form.Item
                label={t("severityKey")}
                name="severityKey"
                rules={[
                  { required: true, message: t("severityRequired") },
                  { max: 64, message: t("severityLength") }
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Form.Item label={t("sourceFilter")} name="sourceFilterText">
                <Input.TextArea autoSize={{ minRows: 3, maxRows: 6 }} placeholder={"prometheus\nalertmanager"} />
              </Form.Item>

              <Form.Item name="enabled" valuePropName="checked">
                <Checkbox>{t("enabled")}</Checkbox>
              </Form.Item>

              <Space wrap>
                <Button disabled={busy || !canSaveCurrentGroupingPolicy} htmlType="submit" icon={<SaveOutlined />} loading={busy} type="primary">
                  {t("savePolicy")}
                </Button>
                <Button disabled={busy} onClick={resetForm} type="default">
                  {t("reset")}
                </Button>
              </Space>
            </Form>
          </Card>
        </Col>

        <Col lg={16} md={24} xs={24}>
          <Card
            extra={
              <Button disabled={busy || !canReadGroupingPolicies} icon={<ReloadOutlined />} loading={busy} onClick={handleRefresh} type="default">
                {t("refresh")}
              </Button>
            }
            title={t("configuredPolicies")}
          >
            <GroupingPolicyTable
              busy={busy}
              canRead={canReadGroupingPolicies}
              canEditPolicy={(policyID) => currentAuthorization.can(groupingPolicyManageKey(policyID))}
              canPreviewPolicy={(policyID) => currentAuthorization.can(groupingPolicyReadKey(policyID))}
              onEdit={editPolicy}
              onPreview={handlePreview}
              policies={policies}
              previewingID={previewingID}
              previewResults={previewResults}
              locale={locale}
              t={t}
            />
            <PreviewPanel locale={locale} result={selectedPreview} t={t} />
          </Card>
        </Col>
      </Row>
    </div>
  );
}

function MetricCard({ label, value }: { label: string; value: number }) {
  return (
    <Col lg={6} sm={12} xs={24}>
      <Card className="settings-stat-card">
        <Statistic title={label} value={value} />
      </Card>
    </Col>
  );
}

function Notice({ notice, t }: { notice: SettingsNotice; t: GroupingTranslator }) {
  const type = notice.kind === "error" ? "error" : notice.kind === "warning" ? "warning" : "success";
  return (
    <Alert
      description={notice.message}
      message={notice.kind === "error" ? t("requestFailed") : t("settings")}
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={type}
    />
  );
}

function GroupingPolicyTable({
  policies,
  busy,
  canRead,
  canEditPolicy,
  canPreviewPolicy,
  onEdit,
  onPreview,
  previewResults,
  previewingID,
  locale,
  t,
}: {
  policies: GroupingPolicy[];
  busy: boolean;
  canRead: boolean;
  canEditPolicy: (policyID: number) => boolean;
  canPreviewPolicy: (policyID: number) => boolean;
  onEdit: (policy: GroupingPolicy) => void;
  onPreview: (policy: GroupingPolicy) => void;
  previewResults: Record<number, GroupingPolicyPreviewResult>;
  previewingID: number | null;
  locale: string;
  t: GroupingTranslator;
}) {
  const common = useTranslations("Common");
  const columns: TableColumnsType<GroupingPolicy> = [
    {
      dataIndex: "name",
      key: "name",
      title: t("name"),
      render: (_value, policy) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>{policy.name}</Typography.Text>
          <Typography.Text type="secondary">#{policy.id}</Typography.Text>
        </Space>
      )
    },
    {
      dataIndex: "dimension_keys",
      key: "dimensions",
      title: t("dimensions"),
      render: (_value, policy) => <TokenList emptyText={t("none")} values={policy.dimension_keys} />
    },
    {
      dataIndex: "severity_key",
      key: "severity",
      title: t("severity"),
      render: (_value, policy) => <Tag color="gold">{policy.severity_key}</Tag>
    },
    {
      dataIndex: "source_filter",
      key: "source_filter",
      title: t("sources"),
      render: (_value, policy) => <TokenList emptyText={t("allSources")} values={policy.source_filter} />
    },
    {
      dataIndex: "enabled",
      key: "state",
      title: t("state"),
      render: (_value, policy) => (
        <Tag color={policy.enabled ? "green" : "default"}>
          {policy.enabled ? t("enabled") : t("disabled")}
        </Tag>
      )
    },
    {
      key: "last_preview",
      title: t("lastPreview"),
      render: (_value, policy) => <PreviewSummary result={previewResults[policy.id]} t={t} />
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: t("updated"),
      render: (_value, policy) => formatDateTime(policy.updated_at, locale)
    },
    {
      key: "action",
      title: t("action"),
      render: (_value, policy) => {
        const canEdit = canEditPolicy(policy.id);
        const canPreview = canPreviewPolicy(policy.id);
        return (
          <Space wrap>
            <Button
              disabled={
                busy ||
                !canPreview ||
                (previewingID !== null && previewingID !== policy.id)
              }
              icon={<PlayCircleOutlined />}
              loading={previewingID === policy.id}
              onClick={() => onPreview(policy)}
              type="link"
            >
              {t("preview")}
            </Button>
            <Button
              disabled={busy || !canEdit || previewingID !== null}
              icon={<EditOutlined />}
              onClick={() => onEdit(policy)}
              type="link"
            >
              {t("edit")}
            </Button>
          </Space>
        );
      }
    }
  ];

  return (
    <Table<GroupingPolicy>
      columns={columns}
      dataSource={policies}
      loading={busy}
      locale={{
        emptyText: (
          <Empty
            description={settingsReadPermissionEmptyDescription({
              canRead,
              deniedDescription: common("noReadAccess", {
                resource: t("policiesResource"),
              }),
              emptyDescription: t("noPolicies"),
            })}
          />
        )
      }}
      pagination={false}
      rowKey="id"
      scroll={{ x: 1120 }}
    />
  );
}

function PreviewSummary({ result, t }: { result?: GroupingPolicyPreviewResult; t: GroupingTranslator }) {
  if (!result) {
    return <Typography.Text type="secondary">{t("notPreviewed")}</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Typography.Text>{t("groupCount", { count: result.groups.length })}</Typography.Text>
      <Typography.Text type="secondary">
        {t("eventRatio", { matched: result.events_matched, scanned: result.events_scanned })}
      </Typography.Text>
    </Space>
  );
}

function PreviewPanel({
  locale,
  result,
  t,
}: {
  locale: string;
  result?: GroupingPolicyPreviewResult;
  t: GroupingTranslator;
}) {
  if (!result) {
    return null;
  }
  return (
    <div className="settings-preview-panel">
      <div className="settings-preview-header">
        <Space align="center">
          <PartitionOutlined />
          <Typography.Text strong>{t("latestPreview")}</Typography.Text>
        </Space>
        <Typography.Text type="secondary">
          {t("eventsMatched", { matched: result.events_matched, scanned: result.events_scanned })}
        </Typography.Text>
      </div>
      <Table<GroupingPolicyPreviewGroup>
        columns={previewColumns(locale, t)}
        dataSource={result.groups}
        locale={{ emptyText: <Empty description={t("noPreviewGroups")} /> }}
        pagination={false}
        rowKey={(group) => group.group_key}
        scroll={{ x: 940 }}
        size="small"
      />
    </div>
  );
}

function previewColumns(
  locale: string,
  t: GroupingTranslator,
): TableColumnsType<GroupingPolicyPreviewGroup> {
  return [
  {
    dataIndex: "dimensions",
    key: "dimensions",
    title: t("dimensions"),
    render: (_value, group) => <DimensionTags t={t} values={group.dimensions} />
  },
  {
    dataIndex: "severity",
    key: "severity",
    title: t("severity"),
    render: (_value, group) => (
      <Tag color={severityColor(group.severity)}>{t(`severity_${group.severity}`)}</Tag>
    )
  },
  {
    dataIndex: "event_count",
    key: "event_count",
    title: t("events")
  },
  {
    dataIndex: "first_seen_at",
    key: "first_seen_at",
    title: t("firstSeen"),
    render: (_value, group) => formatDateTime(group.first_seen_at, locale)
  },
  {
    dataIndex: "last_seen_at",
    key: "last_seen_at",
    title: t("lastSeen"),
    render: (_value, group) => formatDateTime(group.last_seen_at, locale)
  },
  {
    dataIndex: "event_ids",
    key: "event_ids",
    title: t("eventIds"),
    render: (_value, group) => (
      <Typography.Text className="settings-event-ids">{group.event_ids.join(", ")}</Typography.Text>
    )
  }
  ];
}

function TokenList({ values, emptyText }: { values: string[]; emptyText: string }) {
  if (values.length === 0) {
    return <Typography.Text type="secondary">{emptyText}</Typography.Text>;
  }
  return (
    <div className="label-stack">
      {values.map((value) => (
        <Tag key={value}>{value}</Tag>
      ))}
    </div>
  );
}

function DimensionTags({ t, values }: { t: GroupingTranslator; values: Record<string, string> }) {
  const entries = Object.entries(values).sort(([left], [right]) => left.localeCompare(right));
  if (entries.length === 0) {
    return <Typography.Text type="secondary">{t("none")}</Typography.Text>;
  }
  return (
    <div className="label-stack">
      {entries.map(([key, value]) => (
        <Tag key={key}>
          {key}={value}
        </Tag>
      ))}
    </div>
  );
}

function normalizeFormValues(values: GroupingPolicyFormState): GroupingPolicyFormState {
  return {
    ...emptyGroupingPolicyForm(),
    ...values,
    enabled: Boolean(values.enabled),
    dimensionKeysText: values.dimensionKeysText ?? "",
    severityKey: values.severityKey ?? "",
    sourceFilterText: values.sourceFilterText ?? ""
  };
}

function groupingPolicyReadKey(policyID: number): string {
  return `groupingPolicyRead:${policyID}`;
}

function groupingPolicyManageKey(policyID: number): string {
  return `groupingPolicyManage:${policyID}`;
}

function severityColor(severity: GroupingPolicyPreviewGroup["severity"]) {
  switch (severity) {
    case "critical":
      return "red";
    case "warning":
      return "gold";
    case "info":
      return "blue";
    case "unknown":
      return "default";
  }
}

function localizeGroupingMessage(
  message: string,
  t: GroupingTranslator,
): string {
  const exact: Readonly<Record<string, string>> = {
    "Prepared a default alert grouping policy for alert name, service, namespace, and pod dimensions.":
      t("defaultPrepared")
  };
  return exact[message] ?? message;
}

function localizeGroupingValidationError(
  error: GroupingPolicyValidationError,
  t: GroupingTranslator
): string {
  switch (error.code) {
    case "policy_name_required":
      return t("nameRequired");
    case "policy_name_too_long":
      return t("nameLength", { count: error.limit });
    case "token_list_required":
      return t("dimensionRequired");
    case "token_list_too_long":
      return t("listLength", {
        count: error.limit,
        field: groupingPolicyFieldLabel(error.field, t)
      });
    case "token_invalid_characters":
      return t("invalidCharacters", {
        field: groupingPolicyFieldLabel(error.field, t)
      });
    case "token_too_long":
      return t("byteLength", {
        count: error.limit,
        field: groupingPolicyFieldLabel(error.field, t)
      });
    case "severity_required":
      return t("severityRequired");
    case "severity_invalid_characters":
      return t("severityCharacters");
    case "severity_too_long":
      return t("severityLength", { count: error.limit });
  }
}

function groupingPolicyFieldLabel(
  field: "dimension_key" | "source_filter",
  t: GroupingTranslator
): string {
  return field === "dimension_key" ? t("dimensionKeys") : t("sourceFilter");
}
