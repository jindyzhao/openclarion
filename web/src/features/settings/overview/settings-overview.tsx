"use client";

import {
  ApiOutlined,
  AuditOutlined,
  BellOutlined,
  BranchesOutlined,
  CalendarOutlined,
  CheckCircleOutlined,
  DisconnectOutlined,
  ExperimentOutlined,
  LoginOutlined,
  PartitionOutlined,
  PlayCircleOutlined,
  RadarChartOutlined,
  ReloadOutlined,
  ToolOutlined,
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Col,
  Descriptions,
  Form,
  Input,
  Row,
  Space,
  Steps,
  Tag,
  Table,
  Typography,
} from "antd";
import type { TableColumnsType } from "antd";
import { useSearchParams } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import type { AlertEventSummary } from "@/features/alerts/api";
import { DiagnosisAuthModeSelector } from "@/features/diagnosis-room/auth-mode-selector";
import { diagnosisOIDCLoginHref } from "@/features/diagnosis-room/oidc-login";
import {
  alertSourceLaunchHref,
  alertSourceProfileIsThanosRule,
} from "../alert-sources/format";
import type { AlertSourceProfile } from "../alert-sources/types";
import { formatDateTime } from "../format";
import {
  diagnosisAuthBackendReadiness,
  diagnosisAuthBackendReadinessStatusLabel,
  diagnosisAuthBackendCredentialListLabel,
  diagnosisAuthBackendModeDisplayItems,
  diagnosisAuthBackendModeListLabel,
  diagnosisAuthBackendStatusModes,
  diagnosisAuthBrowserSessionAuthenticatedSummary,
  diagnosisAuthCoercedMode,
  diagnosisAuthBrowserSessionBlockReason,
  diagnosisAuthWeComSetupReadiness,
  type DiagnosisAuthBackendReadiness,
  type DiagnosisAuthBackendStatusSnapshot,
  diagnosisAuthInputReadiness,
  diagnosisAuthInputReadinessStatusLabel,
  diagnosisAuthLDAPBrowserSessionPromotionNotice,
  diagnosisAuthLDAPSetupReadiness,
  diagnosisAuthModeOptions,
  diagnosisAuthOIDCBFFMissingLabel,
  diagnosisAuthOIDCBFFReadinessDetail,
  diagnosisAuthRolloutReadiness,
  type DiagnosisAuthBackendCheck,
  type DiagnosisAuthInputValues,
  type DiagnosisAuthMode,
  type DiagnosisAuthTranslator,
  type DiagnosisAuthLDAPSetupReadiness,
  type DiagnosisAuthRolloutReadiness,
  diagnosisAuthRoleMappingGuidance,
  diagnosisAuthRoleMappingStatusReadiness,
  type DiagnosisAuthWeComSetupReadiness,
  type DiagnosisAuthWeComSetupReadinessItem,
} from "@/features/diagnosis-room/auth-readiness";
import {
  normalizedDiagnosisAuthorization,
  type DiagnosisAuthorization,
} from "@/features/diagnosis-room/authorization";
import {
  diagnosisNotificationDeliveryCoverage,
  diagnosisNotificationDeliveryCoveragePhaseColor,
  diagnosisNotificationTimelineAnchorID,
  type DiagnosisNotificationDeliveryCoverage,
} from "@/features/diagnosis-room/notification-content-proof";
import {
  checkDiagnosisAuthorization,
  clearDiagnosisBrowserSession,
  createDiagnosisBrowserSession,
  fetchDiagnosisBrowserSession,
  fetchDiagnosisAuthStatus,
  type DiagnosisBrowserSessionStatus,
  type DiagnosisAuthStatus,
} from "@/features/diagnosis-room/transport";
import {
  diagnosisRoomAnchorHref,
  diagnosisRoomWeComAuthErrorSearchParam,
  type DiagnosisRoomWeComAuthError,
} from "@/features/diagnosis-room/url-state";
import { diagnosisToolTemplateLaunchHref } from "../diagnosis-tool-templates/format";
import type { DiagnosisToolTemplate } from "../diagnosis-tool-templates/types";
import type {
  DirectoryDepartment,
  DirectorySyncRun,
  DirectoryUser,
  RBACAssignment,
} from "../directory-rbac/types";
import { groupingPolicyLaunchHref } from "../grouping-policies/format";
import type { GroupingPolicy } from "../grouping-policies/types";
import { useClientReady } from "../query-state";
import {
  currentRBACAuthorizationNeedsSignIn,
  useCurrentRBACAuthorizations,
  type CurrentRBACAuthorizationCheck,
  type CurrentRBACAuthorizationState,
} from "../rbac-capabilities";
import {
  channelToFormState,
  notificationChannelEditHref,
  notificationChannelAIProofReadiness,
  notificationChannelCredentialReadiness,
  notificationChannelEnterpriseWeChatRolloutReadiness,
  notificationChannelLaunchHref,
  notificationChannelTestProofBundleFromResults,
} from "../notification-channels/format";
import type {
  NotificationChannelProfile,
  NotificationChannelTestContentKind,
} from "../notification-channels/types";
import { reportWorkflowPolicyLaunchHref } from "../report-workflow-policies/format";
import type { ReportWorkflowPolicy } from "../report-workflow-policies/types";
import { reportWorkflowScheduleLaunchHref } from "../report-workflow-schedules/format";
import type { ReportWorkflowSchedule } from "../report-workflow-schedules/types";

type SettingsOverviewCounts = {
  alertSources: number | null;
  groupingPolicies: number | null;
  diagnosisToolTemplates: number | null;
  notificationChannels: number | null;
  rbacAssignments?: number | null;
  workflowPolicies: number | null;
  workflowSchedules: number | null;
};

const diagnosisAuthReadinessFocusTarget = "diagnosis-auth-readiness";
const diagnosisAuthReadinessLoginHref = diagnosisOIDCLoginHref(
  `/settings?focus=${diagnosisAuthReadinessFocusTarget}`,
);

const settingsDirectoryRBACLoginHref = diagnosisOIDCLoginHref(
  "/settings/directory-rbac",
);

const settingsOverviewRBACAuthorizationChecks = [
  { key: "directory-read", permission: "directory.read" },
  { key: "directory-sync", permission: "directory.manage" },
  { key: "rbac-manage", permission: "rbac.manage" },
  { key: "operations-read", permission: "operations.read" },
] as const satisfies readonly CurrentRBACAuthorizationCheck[];

type SettingsOverviewProps = {
  alerts?: AlertEventSummary[];
  alertSources: AlertSourceProfile[];
  counts: SettingsOverviewCounts;
  directoryDepartments?: DirectoryDepartment[];
  directorySyncRuns?: DirectorySyncRun[];
  directoryUsers?: DirectoryUser[];
  diagnosisToolTemplates: DiagnosisToolTemplate[];
  groupingPolicies: GroupingPolicy[];
  notificationChannels: NotificationChannelProfile[];
  rbacAssignments?: RBACAssignment[];
  workflowPolicies: ReportWorkflowPolicy[];
  workflowSchedules: ReportWorkflowSchedule[];
};

export type SettingsOverviewTranslator = ReturnType<
  typeof useTranslations<"SettingsOverview">
>;

type Stage = {
  count: number | null;
  countLabel: string;
  href: string;
  icon: ReactNode;
  key: "sources" | "grouping" | "tools" | "channels" | "workflow" | "schedules";
  title: string;
};

type ProofTarget = {
  actionHref: string;
  actionLabel: string;
  detail: string;
  evidence: string[];
  key: string;
  status: "ready" | "review" | "pending" | "blocked";
  title: string;
};

type ProofReadinessStatus = ProofTarget["status"];

type LiveProofReadinessItem = {
  detail: string;
  key: string;
  status: ProofReadinessStatus;
  title: string;
  value: string;
};

type LiveProofReadiness = {
  detail: string;
  items: LiveProofReadinessItem[];
  status: ProofReadinessStatus;
};

type IntegrationReadinessItem = {
  actionHref: string;
  actionLabel: string;
  detail: string;
  key:
    | "operator-auth"
    | "local-access"
    | "enterprise-wechat"
    | "alertmanager-auto-room";
  status: ProofReadinessStatus;
  title: string;
  value: string;
};

type IntegrationReadiness = {
  detail: string;
  items: IntegrationReadinessItem[];
  status: ProofReadinessStatus;
};

export type AlertIngestionWebhookProofReadiness = {
  href: string;
  status: "ready" | "review" | "blocked";
  value: string;
};

export type DiagnosisAuthLiveProof = {
  checkedAt?: string;
  detail: string;
  mode: DiagnosisAuthMode;
  roleAuthorized?: boolean;
  roles: string[];
  status: DiagnosisAuthBackendReadiness["status"];
  subject: string;
};

type WorkflowTopology = {
  activeSchedule: ReportWorkflowSchedule | null;
  activeAlertTools: DiagnosisToolTemplate[];
  alertSource: AlertSourceProfile | null;
  diagnosisTools: DiagnosisToolTemplate[];
  groupingPolicy: GroupingPolicy | null;
  metricSources: AlertSourceProfile[];
  metricTools: DiagnosisToolTemplate[];
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

export type AutoDiagnosisProofHistory = {
  alertID: number;
  alertName: string;
  alertSourceProfileID: number;
  coverage: DiagnosisNotificationDeliveryCoverage;
  href: string;
  occurredAt: string;
  roomSessionID: string;
  snapshotID: number;
};

type DiagnosisAuthProbeFormValues = DiagnosisAuthInputValues;

type DiagnosisAuthProbeLocalFailure =
  "backend_readiness" | "browser_session" | "credentials";

type DiagnosisAuthProbeResult = DiagnosisAuthBackendCheck & {
  localFailure?: DiagnosisAuthProbeLocalFailure;
};

type DiagnosisAuthStatusState = {
  loading: boolean;
  message: string;
  status: DiagnosisAuthStatus | null;
};

type DiagnosisBrowserSessionState = {
  loading: boolean;
  message: string;
  status: DiagnosisBrowserSessionStatus | null;
};

type DiagnosisAuthReadinessSummaryItem = {
  detail: string;
  key: "access" | "backend" | "entries" | "rollout" | "session" | "setup";
  label: string;
  status: ProofReadinessStatus;
  value: string;
};

export type DiagnosisAuthReadinessSummary = {
  detail: string;
  items: DiagnosisAuthReadinessSummaryItem[];
  label: string;
  status: ProofReadinessStatus;
};

type SettingsWeComReadinessStatus =
  "ready" | "review" | "blocked" | "loading" | "unavailable";

type SettingsWeComBrowserSessionReadinessItem = {
  detail: string;
  key: "session" | "provider" | "role" | "subject";
  label: string;
  status: SettingsWeComReadinessStatus;
  value: string;
};

export type SettingsWeComBrowserSessionReadiness = {
  checkedAt?: string;
  detail: string;
  items: SettingsWeComBrowserSessionReadinessItem[];
  label: string;
  status: SettingsWeComReadinessStatus;
};

type SettingsLocalAccessReadinessItem = {
  detail: string;
  key:
    | "directory-sync"
    | "directory-users"
    | "directory-departments"
    | "rbac-assignments";
  label: string;
  status: ProofReadinessStatus;
  value: string;
};

export type SettingsLocalAccessReadiness = {
  detail: string;
  items: SettingsLocalAccessReadinessItem[];
  label: string;
  status: ProofReadinessStatus;
};

export type SettingsOIDCBFFSetupReadiness = {
  detail: string;
  label: string;
  status: ProofReadinessStatus;
  value: string;
};

type SettingsCurrentRBACAccessItem = {
  key: string;
  label: string;
  status: ProofReadinessStatus;
  value: string;
};

export type SettingsCurrentRBACAccessSummary = {
  detail: string;
  items: SettingsCurrentRBACAccessItem[];
  label: string;
  needsSignIn: boolean;
  status: ProofReadinessStatus;
  subject: string;
};

export type SettingsLDAPConfigurationGuidance = {
  detail: string;
  envVar: string;
  label: string;
  value: string;
};

export function SettingsOverview({
  alerts = [],
  alertSources,
  counts,
  directoryDepartments = [],
  directorySyncRuns,
  directoryUsers = [],
  diagnosisToolTemplates,
  groupingPolicies,
  notificationChannels,
  rbacAssignments = [],
  workflowPolicies,
  workflowSchedules,
}: SettingsOverviewProps) {
  const locale = useLocale();
  const t = useTranslations("SettingsOverview");
  const authT = useTranslations("DiagnosisAuth");
  const searchParams = useSearchParams();
  const clientReady = useClientReady();
  const currentRBACAuthorization = useCurrentRBACAuthorizations(
    settingsOverviewRBACAuthorizationChecks,
    clientReady,
  );
  const currentRBACAccess = settingsCurrentRBACAccessSummary(
    currentRBACAuthorization.state,
    currentRBACAuthorization.isChecking,
    t,
    locale,
  );
  const focusTarget = searchParams.get("focus") ?? "";
  const weComAuthError = diagnosisRoomWeComAuthErrorSearchParam(searchParams);
  const [diagnosisAuthProof, setDiagnosisAuthProof] =
    useState<DiagnosisAuthLiveProof>(defaultDiagnosisAuthLiveProof());
  const [diagnosisAuthStatus, setDiagnosisAuthStatus] =
    useState<DiagnosisAuthStatusState>({
      loading: true,
      message: "",
      status: null,
    });
  const [diagnosisBrowserSession, setDiagnosisBrowserSession] =
    useState<DiagnosisBrowserSessionState>({
      loading: true,
      message: "",
      status: null,
    });
  const [diagnosisBrowserSessionClearing, setDiagnosisBrowserSessionClearing] =
    useState(false);
  const refreshDiagnosisBrowserSession = useCallback(async () => {
    setDiagnosisBrowserSession((current) => ({
      ...current,
      loading: true,
      message: "",
    }));
    setDiagnosisBrowserSession(await diagnosisBrowserSessionStateFromFetch());
  }, []);
  const signOutDiagnosisBrowserSession = useCallback(async () => {
    setDiagnosisBrowserSessionClearing(true);
    try {
      const result = await clearDiagnosisBrowserSession();
      if (!result.ok) {
        setDiagnosisBrowserSession({
          loading: false,
          message: result.error.message,
          status: null,
        });
        return;
      }
      setDiagnosisBrowserSession({
        loading: false,
        message: "",
        status: { authenticated: false },
      });
    } finally {
      setDiagnosisBrowserSessionClearing(false);
    }
  }, []);
  const handleDiagnosisBrowserSessionEstablished = useCallback(
    (status: DiagnosisBrowserSessionStatus) => {
      setDiagnosisBrowserSession({
        loading: false,
        message: "",
        status,
      });
    },
    [],
  );
  useEffect(() => {
    let mounted = true;
    void fetchDiagnosisAuthStatus().then((result) => {
      if (!mounted) {
        return;
      }
      if (!result.ok) {
        setDiagnosisAuthStatus({
          loading: false,
          message: result.error.message,
          status: null,
        });
        return;
      }
      setDiagnosisAuthStatus({
        loading: false,
        message: "",
        status: result.data,
      });
    });
    return () => {
      mounted = false;
    };
  }, []);
  useEffect(() => {
    let mounted = true;
    void diagnosisBrowserSessionStateFromFetch().then((next) => {
      if (!mounted) {
        return;
      }
      setDiagnosisBrowserSession(next);
    });
    return () => {
      mounted = false;
    };
  }, []);
  useEffect(() => {
    if (focusTarget !== diagnosisAuthReadinessFocusTarget) {
      return;
    }
    document
      .getElementById(diagnosisAuthReadinessFocusTarget)
      ?.scrollIntoView({ block: "start" });
  }, [focusTarget]);
  const stages: Stage[] = [
    {
      count: counts.alertSources,
      countLabel: t("stages.profiles"),
      href: "/settings/alert-sources",
      icon: <ApiOutlined aria-hidden="true" />,
      key: "sources",
      title: t("stages.alertSources"),
    },
    {
      count: counts.groupingPolicies,
      countLabel: t("stages.policies"),
      href: "/settings/grouping-policies",
      icon: <PartitionOutlined aria-hidden="true" />,
      key: "grouping",
      title: t("stages.grouping"),
    },
    {
      count: counts.diagnosisToolTemplates,
      countLabel: t("stages.templates"),
      href: "/settings/diagnosis-tool-templates",
      icon: <ToolOutlined aria-hidden="true" />,
      key: "tools",
      title: t("stages.diagnosisTools"),
    },
    {
      count: counts.notificationChannels,
      countLabel: t("stages.channels"),
      href: "/settings/notification-channels",
      icon: <BellOutlined aria-hidden="true" />,
      key: "channels",
      title: t("stages.notifications"),
    },
    {
      count: counts.workflowPolicies,
      countLabel: t("stages.policies"),
      href: "/settings/report-workflow-policies",
      icon: <BranchesOutlined aria-hidden="true" />,
      key: "workflow",
      title: t("stages.workflowPolicies"),
    },
    {
      count: counts.workflowSchedules,
      countLabel: t("stages.schedules"),
      href: "/settings/report-workflow-schedules",
      icon: <CalendarOutlined aria-hidden="true" />,
      key: "schedules",
      title: t("stages.schedulesTitle"),
    },
  ];
  const nextStageIndex = stages.findIndex(
    (stage) => !isReadyCount(stage.count),
  );
  const currentStep = nextStageIndex === -1 ? stages.length : nextStageIndex;
  const nextStage =
    nextStageIndex === -1 ? null : (stages[nextStageIndex] ?? null);
  const allConfigurationPresent = nextStage === null;
  const topology = buildWorkflowTopology({
    alertSources,
    diagnosisToolTemplates,
    groupingPolicies,
    notificationChannels,
    workflowPolicies,
    workflowSchedules,
  });
  const autoDiagnosisProofHistory = useMemo(
    () =>
      latestAutoDiagnosisProofHistoryForSource(
        alerts,
        topology.alertSource?.id ?? null,
        t,
      ),
    [alerts, t, topology.alertSource?.id],
  );
  const autoDiagnosisProofHistories = useMemo(
    () => autoDiagnosisProofHistoriesForAlerts(alerts, t, 5),
    [alerts, t],
  );
  const topologyActions = workflowTopologyActions(topology, t, locale);
  const nextTopologyAction = topologyActions[0] ?? null;
  const nextSetup =
    nextStage === null && nextTopologyAction !== null
      ? {
          actionLabel: t("actions.open"),
          detail: nextTopologyAction.detail,
          href: nextTopologyAction.href,
          priority: nextTopologyAction.priority,
          tagColor: actionPriorityColor(nextTopologyAction.priority),
          tagLabel: t(`priority.${nextTopologyAction.priority}`),
          title: nextTopologyAction.title,
        }
      : nextStage === null
        ? {
            actionLabel: "",
            detail: t("nextSetup.retainedProofDetail"),
            href: "",
            priority: null,
            tagColor: "gold",
            tagLabel: t("nextSetup.proofPending"),
            title: t("nextSetup.retainedProofTitle"),
          }
        : {
            actionLabel: t("actions.configure"),
            detail: t("nextSetup.configureDetail", { stage: nextStage.title }),
            href: nextStage.href,
            priority: null,
            tagColor: "gold",
            tagLabel: t("status.needsSetup"),
            title: nextStage.title,
          };
  const proofTargets = workflowProofTargets(
    topology,
    t,
    locale,
    autoDiagnosisProofHistory,
  );
  const liveProofReadiness = workflowLiveProofReadiness(
    topology,
    t,
    locale,
    diagnosisAuthProof,
  );
  const localAccessReadiness = settingsLocalAccessReadiness(
    {
    directoryDepartments,
    directorySyncRuns,
    directoryUsers,
    rbacAssignments,
    },
    t,
    locale,
  );
  const integrationReadiness = workflowIntegrationReadiness(
    topology,
    authT,
    t,
    locale,
    diagnosisAuthProof,
    localAccessReadiness,
    currentRBACAccess,
  );

  return (
    <div className="stack">
      <section className="panel">
        <div className="panel-header">
          <h2>{t("configuration.title")}</h2>
        </div>
        <div className="panel-body settings-overview-flow">
          <Steps
            aria-label={t("configuration.sequence")}
            className="settings-overview-steps"
            current={currentStep}
            items={[
              ...stages.map((stage, index) => ({
                icon: stage.icon,
                status: stepStatus(stage.count, index, currentStep),
                title: shortStageTitle(stage.key, t),
              })),
              {
                icon: <PlayCircleOutlined />,
                status: allConfigurationPresent ? "process" : "wait",
                title: t("stages.proof"),
              },
            ]}
            responsive={false}
            type="inline"
          />
        </div>
      </section>

      <section
        aria-label={t("nextSetup.ariaLabel")}
        className="panel settings-overview-next"
      >
        <div>
          <Typography.Text className="muted">
            {t("nextSetup.label")}
          </Typography.Text>
          <Typography.Title level={2}>{nextSetup.title}</Typography.Title>
          <Typography.Paragraph className="settings-overview-next-copy">
            {nextSetup.detail}
          </Typography.Paragraph>
        </div>
        {nextSetup.href === "" ? (
          <Tag
            color={nextSetup.tagColor}
            data-priority={nextSetup.priority ?? undefined}
            data-testid="settings-next-priority"
          >
            {nextSetup.tagLabel}
          </Tag>
        ) : (
          <Space wrap>
            <Tag
              color={nextSetup.tagColor}
              data-priority={nextSetup.priority ?? undefined}
              data-testid="settings-next-priority"
            >
              {nextSetup.tagLabel}
            </Tag>
            <Button
              href={nextSetup.href}
              icon={<RadarChartOutlined />}
              type="primary"
            >
              {nextSetup.actionLabel}
            </Button>
          </Space>
        )}
      </section>

      <IntegrationReadinessPanel readiness={integrationReadiness} />
      <WorkflowTopologyPanel topology={topology} />
      <DiagnosisAuthProbePanel
        backendStatus={diagnosisAuthStatus}
        browserSession={diagnosisBrowserSession}
        browserSessionClearing={diagnosisBrowserSessionClearing}
        localAccessReadiness={localAccessReadiness}
        onBrowserSessionEstablished={handleDiagnosisBrowserSessionEstablished}
        onProofChange={setDiagnosisAuthProof}
        onRefreshBrowserSession={() => void refreshDiagnosisBrowserSession()}
        onSignOutBrowserSession={() => void signOutDiagnosisBrowserSession()}
        weComAuthError={weComAuthError}
      />
      <AlertIngestionReadinessPanel
        autoDiagnosisProofHistory={autoDiagnosisProofHistory}
        topology={topology}
      />
      <LiveProofReadinessPanel readiness={liveProofReadiness} />
      <AutoDiagnosisProofHistoryPanel histories={autoDiagnosisProofHistories} />

      <Row aria-label={t("proofTargets.ariaLabel")} gutter={[16, 16]}>
        {proofTargets.map((target) => (
          <Col key={target.key} lg={12} xs={24}>
            <Card
              className="settings-overview-proof-card"
              title={
                <CardTitle
                  icon={proofTargetIcon(target.key)}
                  title={target.title}
                />
              }
            >
              <div className="settings-overview-proof-body">
                <Space direction="vertical" size={10}>
                  <Space wrap>
                    <Tag color={proofTargetStatusColor(target.status)}>
                      {proofTargetStatusLabel(target.status, t)}
                    </Tag>
                    {allConfigurationPresent ? (
                      <Tag color="gold">{t("status.configurationPresent")}</Tag>
                    ) : (
                      <Tag>{t("status.waitingForGraph")}</Tag>
                    )}
                  </Space>
                  <Typography.Paragraph className="settings-overview-proof-copy">
                    {target.detail}
                  </Typography.Paragraph>
                </Space>
                <div
                  className="settings-overview-proof-tags"
                  aria-label={t("proofTargets.evidenceAriaLabel", {
                    target: target.title,
                  })}
                >
                  {target.evidence.map((item) => (
                    <Tag key={item}>{item}</Tag>
                  ))}
                </div>
                <Button
                  block
                  href={target.actionHref}
                  icon={<RadarChartOutlined />}
                  type={target.status === "blocked" ? "default" : "primary"}
                >
                  {target.actionLabel}
                </Button>
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      <Row aria-label={t("settingsSurfaces")} gutter={[16, 16]}>
        {stages.map((stage) => (
          <Col key={stage.key} lg={8} md={12} xs={24}>
            <Card
              className="settings-overview-card"
              title={<CardTitle icon={stage.icon} title={stage.title} />}
            >
              <div className="settings-overview-card-body">
                <div>
                  <div className="settings-overview-count">
                    {formatCount(stage.count, locale)}
                  </div>
                  <Typography.Text className="muted">
                    {stage.countLabel}
                  </Typography.Text>
                </div>
                <ReadinessTag count={stage.count} />
              </div>
              <Button
                block
                href={stage.href}
                icon={<RadarChartOutlined />}
                type="default"
              >
                {t("actions.open")}
              </Button>
            </Card>
          </Col>
        ))}
      </Row>

      <Row aria-label={t("access.ariaLabel")} gutter={[16, 16]}>
        <Col lg={8} md={12} xs={24}>
          <Card
            className="settings-overview-card"
            title={
              <CardTitle
                icon={<AuditOutlined aria-hidden="true" />}
                title={t("access.title")}
              />
            }
          >
            <div className="settings-overview-card-body">
              <div>
                <div className="settings-overview-count">
                  {formatCount(
                    counts.rbacAssignments ?? rbacAssignments.length,
                    locale,
                  )}
                </div>
                <Typography.Text className="muted">
                  {t("access.rules")}
                </Typography.Text>
              </div>
              <Space size={6} wrap>
                <Tag
                  color={proofTargetStatusColor(localAccessReadiness.status)}
                >
                  {localAccessReadiness.label}
                </Tag>
                <Tag color={proofTargetStatusColor(currentRBACAccess.status)}>
                  {currentRBACAccess.label}
                </Tag>
                <Tag
                  color={proofTargetStatusColor(
                    settingsLocalAccessSyncStatus(localAccessReadiness),
                  )}
                >
                  {settingsLocalAccessSyncValue(localAccessReadiness, t)}
                </Tag>
                <Tag color={directoryUsers.length > 0 ? "green" : "gold"}>
                  {t("access.users", { count: directoryUsers.length })}
                </Tag>
                <Tag color={directoryDepartments.length > 0 ? "green" : "gold"}>
                  {t("access.departments", {
                    count: directoryDepartments.length,
                  })}
                </Tag>
                {currentRBACAccess.subject === "" ? null : (
                  <Tag>{currentRBACAccess.subject}</Tag>
                )}
              </Space>
            </div>
            <Space direction="vertical" size={8} style={{ width: "100%" }}>
              <Typography.Paragraph
                className="settings-overview-card-detail"
                type="secondary"
              >
                {currentRBACAccess.detail}
              </Typography.Paragraph>
              <Space wrap>
                {currentRBACAccess.items.map((item) => (
                  <Tag
                    color={proofTargetStatusColor(item.status)}
                    key={item.key}
                  >
                    {item.label}: {item.value}
                  </Tag>
                ))}
              </Space>
              <Button
                block
                href={
                  currentRBACAccess.needsSignIn
                    ? settingsDirectoryRBACLoginHref
                    : "/settings/directory-rbac"
                }
                icon={
                  currentRBACAccess.needsSignIn ? (
                    <LoginOutlined />
                  ) : (
                    <RadarChartOutlined />
                  )
                }
                type={currentRBACAccess.needsSignIn ? "primary" : "default"}
              >
                {currentRBACAccess.needsSignIn
                  ? t("actions.signInWithIAM")
                  : t("actions.open")}
              </Button>
            </Space>
          </Card>
        </Col>
      </Row>

      <Row aria-label={t("boundaries.ariaLabel")} gutter={[16, 16]}>
        <Col lg={8} xs={24}>
          <BoundaryPanel
            icon={<ExperimentOutlined />}
            status={t("boundaries.connectionTests.status")}
            title={t("boundaries.connectionTests.title")}
            text={t("boundaries.connectionTests.detail")}
          />
        </Col>
        <Col lg={8} xs={24}>
          <BoundaryPanel
            icon={<AuditOutlined />}
            status={t("boundaries.previews.status")}
            title={t("boundaries.previews.title")}
            text={t("boundaries.previews.detail")}
          />
        </Col>
        <Col lg={8} xs={24}>
          <BoundaryPanel
            icon={<CheckCircleOutlined />}
            status={t("boundaries.replay.status")}
            title={t("boundaries.replay.title")}
            text={t("boundaries.replay.detail")}
          />
        </Col>
      </Row>

      <Alert
        message={
          allConfigurationPresent
            ? t("gate.liveProofTitle")
            : t("gate.configurationTitle")
        }
        showIcon
        type={allConfigurationPresent ? "warning" : "info"}
        description={
          allConfigurationPresent
            ? t("gate.liveProofDetail")
            : t("gate.configurationDetail")
        }
      />
    </div>
  );
}

function IntegrationReadinessPanel({
  readiness,
}: {
  readiness: IntegrationReadiness;
}) {
  const t = useTranslations("SettingsOverview");
  return (
    <section
      aria-label={t("integration.ariaLabel")}
      className="panel settings-overview-integration"
    >
      <div className="panel-header settings-overview-integration-header">
        <h2>{t("integration.title")}</h2>
        <Tag color={proofTargetStatusColor(readiness.status)}>
          {proofTargetStatusLabel(readiness.status, t)}
        </Tag>
      </div>
      <div className="panel-body settings-overview-integration-body">
        <Typography.Paragraph className="settings-overview-proof-copy">
          {readiness.detail}
        </Typography.Paragraph>
        <div className="settings-overview-integration-grid">
          {readiness.items.map((item) => (
            <div className="settings-overview-integration-item" key={item.key}>
              <div>
                <Space size={8} wrap>
                  <Tag color={proofTargetStatusColor(item.status)}>
                    {proofTargetStatusLabel(item.status, t)}
                  </Tag>
                  <Typography.Text strong>{item.title}</Typography.Text>
                </Space>
                <Typography.Title
                  className="settings-overview-integration-value"
                  level={3}
                >
                  {item.value}
                </Typography.Title>
                <Typography.Paragraph className="settings-overview-action-copy">
                  {item.detail}
                </Typography.Paragraph>
              </div>
              <Button
                href={item.actionHref}
                icon={<RadarChartOutlined />}
                type={item.status === "blocked" ? "primary" : "default"}
              >
                {item.actionLabel}
              </Button>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function proofTargetIcon(key: string): ReactNode {
  switch (key) {
    case "alertmanager-auto-diagnosis":
      return <ApiOutlined aria-hidden="true" />;
    case "scheduled-trigger":
      return <CalendarOutlined aria-hidden="true" />;
    case "policy-replay":
    default:
      return <PlayCircleOutlined aria-hidden="true" />;
  }
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
  title,
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
      <Typography.Paragraph className="settings-overview-boundary-copy">
        {text}
      </Typography.Paragraph>
    </section>
  );
}

function DiagnosisAuthProbePanel({
  backendStatus,
  browserSession,
  browserSessionClearing,
  localAccessReadiness,
  onBrowserSessionEstablished,
  onRefreshBrowserSession,
  onProofChange,
  onSignOutBrowserSession,
  weComAuthError,
}: {
  backendStatus: DiagnosisAuthStatusState;
  browserSession: DiagnosisBrowserSessionState;
  browserSessionClearing: boolean;
  localAccessReadiness: SettingsLocalAccessReadiness;
  onBrowserSessionEstablished: (status: DiagnosisBrowserSessionStatus) => void;
  onRefreshBrowserSession: () => void;
  onProofChange: (proof: DiagnosisAuthLiveProof) => void;
  onSignOutBrowserSession: () => void;
  weComAuthError?: DiagnosisRoomWeComAuthError;
}) {
  const authT = useTranslations("DiagnosisAuth");
  const locale = useLocale();
  const [form] = Form.useForm<DiagnosisAuthProbeFormValues>();
  const [values, setValues] = useState<DiagnosisAuthProbeFormValues>({
    authMode: "session",
  });
  const [checking, setChecking] = useState(false);
  const [result, setResult] = useState<DiagnosisAuthProbeResult | null>(null);
  const backendSnapshot = useMemo(
    () => diagnosisAuthBackendStatusSnapshot(backendStatus.status),
    [backendStatus.status],
  );
  const authModeOptions = useMemo(
    () => diagnosisAuthModeOptions(backendSnapshot, authT),
    [authT, backendSnapshot],
  );
  const rawMode = values.authMode ?? "session";
  const mode = diagnosisAuthCoercedMode(rawMode, backendSnapshot);
  const effectiveValues: DiagnosisAuthProbeFormValues = useMemo(
    () => (mode === rawMode ? values : { authMode: mode }),
    [mode, rawMode, values],
  );
  const readiness = diagnosisAuthInputReadiness(effectiveValues, authT);
  const displayedResult = useMemo(
    () =>
      diagnosisAuthProbeDisplayedResult(
        effectiveValues,
        result,
        browserSession.status,
      ),
    [browserSession.status, effectiveValues, result],
  );
  const backendReadiness = diagnosisAuthBackendReadiness(
    {
      backendStatus: backendSnapshot,
      checking,
      lastCheck: displayedResult,
      values: effectiveValues,
    },
    authT,
  );
  const currentLiveProof = useMemo(
    () =>
      diagnosisAuthLiveProofFromProbeState(
        {
          backendStatus: backendSnapshot,
          browserSession: browserSession.status,
          checking,
          result,
          values: effectiveValues,
        },
        authT,
      ),
    [
      authT,
      backendSnapshot,
      browserSession.status,
      checking,
      effectiveValues,
      result,
    ],
  );
  const rolloutReadiness = diagnosisAuthRolloutReadiness(
    currentLiveProof,
    authT,
  );
  const checkAuthBlocked =
    diagnosisAuthProbeCheckBlocked({
      backendStatusAvailable: backendStatus.status !== null,
      backendStatus: backendReadiness.status,
      checking,
      inputStatus: readiness.status,
    }) || browserSessionBlockReason(effectiveValues) !== "";
  const preferredMode = diagnosisAuthCoercedMode(
    diagnosisAuthProbeModeFromBackendStatus(backendStatus.status) ?? "session",
    backendSnapshot,
  );
  const weComAuthErrorDetail =
    weComAuthError === undefined
      ? null
      : settingsWeComAuthErrorDetail(weComAuthError, authT);
  const weComSetupReadiness = diagnosisAuthWeComSetupReadiness(
    backendStatus.status,
    authT,
    backendStatus.loading,
  );
  const ldapSetupReadiness = diagnosisAuthLDAPSetupReadiness(
    backendStatus.status,
    authT,
    backendStatus.loading,
  );
  const oidcSetupReadiness = settingsOIDCBFFSetupReadiness(
    backendStatus.status,
    backendStatus.loading,
    authT,
  );
  const weComBrowserSessionReadiness = settingsWeComBrowserSessionReadiness(
    browserSession,
    authT,
    locale,
  );
  const authReadinessSummary = diagnosisAuthReadinessSummary(
    {
      backendReadiness,
      browserSession,
      ldapSetupReadiness,
      localAccessReadiness,
      mode,
      oidcSetupReadiness,
      rolloutReadiness,
      weComSetupReadiness,
    },
    authT,
  );

  useEffect(() => {
    if (backendSnapshot === undefined || checking || result !== null) {
      return;
    }
    if (mode === "session" && browserSession.loading) {
      return;
    }
    onProofChange(currentLiveProof);
  }, [
    backendSnapshot,
    browserSession.loading,
    checking,
    currentLiveProof,
    mode,
    onProofChange,
    result,
  ]);

  useEffect(() => {
    if (backendSnapshot === undefined) {
      return;
    }
    const currentMode = values.authMode ?? "session";
    const coercedMode = diagnosisAuthCoercedMode(currentMode, backendSnapshot);
    if (coercedMode === currentMode) {
      return;
    }
    form.setFieldsValue({
      authMode: coercedMode,
      bearerToken: "",
      ldapPassword: "",
      ldapUsername: "",
    });
  }, [backendSnapshot, form, values.authMode]);

  async function handleCheckAuth(submitted: DiagnosisAuthProbeFormValues) {
    const submittedMode = diagnosisAuthCoercedMode(
      submitted.authMode ?? "session",
      backendSnapshot,
    );
    const submittedValues: DiagnosisAuthProbeFormValues =
      submittedMode === (submitted.authMode ?? "session")
        ? submitted
        : { authMode: submittedMode };
    const submittedBackendReadiness = diagnosisAuthBackendReadiness(
      {
        backendStatus: backendSnapshot,
        checking: false,
        lastCheck: null,
        values: submittedValues,
      },
      authT,
    );
    if (submittedBackendReadiness.status === "blocked") {
      const failed: DiagnosisAuthProbeResult = {
        localFailure: "backend_readiness",
        message: "",
        mode: submittedMode,
        roles: [],
        status: "failed",
        subject: "",
      };
      setResult(failed);
      onProofChange(diagnosisAuthLiveProofFromResult(failed));
      return;
    }
    const sessionBlockReason = browserSessionBlockReason(submittedValues);
    if (sessionBlockReason !== "") {
      const failed: DiagnosisAuthProbeResult = {
        localFailure: "browser_session",
        message: "",
        mode: submittedMode,
        roles: [],
        status: "failed",
        subject: "",
      };
      setResult(failed);
      onProofChange(diagnosisAuthLiveProofFromResult(failed));
      return;
    }
    const authorization =
      diagnosisAuthorizationFromProbeValues(submittedValues);
    if (authorization === null) {
      const failed: DiagnosisAuthProbeResult = {
        localFailure: "credentials",
        message: "",
        mode: submittedMode,
        roles: [],
        status: "failed",
        subject: "",
      };
      setResult({
        ...failed,
      });
      onProofChange(diagnosisAuthLiveProofFromResult(failed));
      return;
    }

    onProofChange({
      detail: "",
      mode: submittedMode,
      roles: [],
      status: "checking",
      subject: "",
    });
    setChecking(true);
    try {
      const checked = await checkDiagnosisAuthorization(authorization);
      if (!checked.ok) {
        const failed: DiagnosisAuthProbeResult = {
          message: checked.error.message,
          mode: submittedMode,
          roles: [],
          status: "failed",
          subject: "",
        };
        setResult(failed);
        onProofChange(diagnosisAuthLiveProofFromResult(failed));
        return;
      }
      const success: DiagnosisAuthProbeResult = {
        checkedAt: checked.data.checked_at,
        message: "",
        mode: submittedMode,
        roleAuthorized: checked.data.role_authorized,
        roles: checked.data.roles,
        status: "success",
        subject: checked.data.subject,
      };
      if (
        authorization.mode === "basic" &&
        diagnosisAuthProbeResultAuthorized(success)
      ) {
        const session = await createDiagnosisBrowserSession(authorization);
        if (session.ok) {
          onBrowserSessionEstablished(session.data);
          const sessionResult =
            diagnosisAuthProbeResultFromBrowserSessionStatus(session.data);
          if (sessionResult !== null) {
            form.setFieldsValue({
              authMode: "session",
              ldapPassword: "",
            });
            setValues((current) => ({
              ...current,
              authMode: "session",
              ldapPassword: "",
            }));
            setResult(sessionResult);
            onProofChange(diagnosisAuthLiveProofFromResult(sessionResult));
            return;
          }
        }
      }
      setResult(success);
      onProofChange(diagnosisAuthLiveProofFromResult(success));
    } finally {
      setChecking(false);
    }
  }

  function browserSessionBlockReason(
    currentValues: DiagnosisAuthProbeFormValues,
  ): string {
    return diagnosisAuthProbeBrowserSessionBlockReason(
      currentValues,
      browserSession,
      authT,
    );
  }

  return (
    <section
      aria-label={authT("panel.ariaLabel")}
      className="panel settings-overview-auth"
      id={diagnosisAuthReadinessFocusTarget}
    >
      <div className="panel-header settings-overview-auth-header">
        <h2>{authT("panel.title")}</h2>
        <Space size={6} wrap>
          <Tag color={authProbeReadinessColor(readiness.status)}>
            {diagnosisAuthInputReadinessStatusLabel(readiness.status, authT)}
          </Tag>
          <Tag color={backendReadiness.color}>
            {diagnosisAuthBackendReadinessStatusLabel(
              backendReadiness.status,
              authT,
            )}
          </Tag>
          <Tag color={diagnosisAuthStatusColor(backendStatus)}>
            {diagnosisAuthStatusLabel(backendStatus, authT)}
          </Tag>
        </Space>
      </div>
      <div className="panel-body settings-overview-auth-body">
        <Row gutter={[16, 16]}>
          <Col lg={10} xs={24}>
            <Form<DiagnosisAuthProbeFormValues>
              form={form}
              initialValues={{
                authMode: "session",
                bearerToken: "",
                ldapPassword: "",
                ldapUsername: "",
              }}
              layout="vertical"
              onFinish={handleCheckAuth}
              onValuesChange={(_, allValues) => {
                const nextMode = diagnosisAuthCoercedMode(
                  allValues.authMode ?? "session",
                  backendSnapshot,
                );
                const nextValues: DiagnosisAuthProbeFormValues =
                  nextMode === (allValues.authMode ?? "session")
                    ? allValues
                    : { authMode: nextMode };
                setValues(nextValues);
                setResult(null);
                onProofChange(
                  diagnosisAuthLiveProofFromBackend(
                    {
                      backendStatus: backendSnapshot,
                      checking: false,
                      lastCheck: null,
                      values: nextValues,
                    },
                    authT,
                  ),
                );
              }}
            >
              <Form.Item label={authT("panel.authentication")} name="authMode">
                <DiagnosisAuthModeSelector options={authModeOptions} />
              </Form.Item>
              {mode === "wecom" ? (
                <Space direction="vertical" size={8} style={{ width: "100%" }}>
                  {weComAuthErrorDetail === null ? null : (
                    <Alert
                      closable
                      description={weComAuthErrorDetail.description}
                      message={weComAuthErrorDetail.message}
                      showIcon
                      type="warning"
                    />
                  )}
                  <Alert
                    action={
                      <Space wrap>
                        <Button
                          href={diagnosisAuthReadinessLoginHref}
                          icon={<LoginOutlined />}
                          type="primary"
                        >
                          {authT("panel.signInWithIAM")}
                        </Button>
                      </Space>
                    }
                    description={
                      <Space direction="vertical" size={6}>
                        <Typography.Text>
                          {authT("panel.weComMigratedDetail")}
                        </Typography.Text>
                      </Space>
                    }
                    message={authT("panel.weComMigratedMessage")}
                    showIcon
                    type="warning"
                  />
                  <DiagnosisAuthWeComSessionSummary
                    readiness={weComBrowserSessionReadiness}
                  />
                  <DiagnosisAuthWeComSetupSummary
                    readiness={weComSetupReadiness}
                  />
                </Space>
              ) : mode === "session" ? (
                <Alert
                  action={
                    <Space wrap>
                      {diagnosisBrowserSessionNeedsIAMSignIn(browserSession) ? (
                        <Button
                          href={diagnosisAuthReadinessLoginHref}
                          icon={<LoginOutlined />}
                          type="primary"
                        >
                          {authT("panel.signInWithIAM")}
                        </Button>
                      ) : null}
                      <Button
                        disabled={
                          browserSession.loading || browserSessionClearing
                        }
                        icon={<ReloadOutlined />}
                        onClick={onRefreshBrowserSession}
                      >
                        {authT("panel.refreshSession")}
                      </Button>
                      {browserSession.status?.authenticated === true ? (
                        <Button
                          disabled={browserSession.loading}
                          icon={<DisconnectOutlined />}
                          loading={browserSessionClearing}
                          onClick={onSignOutBrowserSession}
                        >
                          {authT("panel.signOut")}
                        </Button>
                      ) : null}
                    </Space>
                  }
                  description={diagnosisBrowserSessionStatusDetail(
                    browserSession,
                    authT,
                  )}
                  message={authT("panel.browserSession")}
                  showIcon
                  type={diagnosisBrowserSessionStatusAlertType(browserSession)}
                />
              ) : mode === "bearer" ? (
                <Form.Item
                  label={authT("panel.bearerToken")}
                  name="bearerToken"
                >
                  <Input.Password autoComplete="off" />
                </Form.Item>
              ) : (
                <>
                  <Form.Item
                    label={authT("panel.ldapUsername")}
                    name="ldapUsername"
                  >
                    <Input autoComplete="username" />
                  </Form.Item>
                  <Form.Item
                    label={authT("panel.ldapPassword")}
                    name="ldapPassword"
                  >
                    <Input.Password autoComplete="current-password" />
                  </Form.Item>
                  <DiagnosisAuthLDAPSetupSummary
                    readiness={ldapSetupReadiness}
                  />
                  <DiagnosisLDAPBrowserSessionPromotionNotice />
                </>
              )}
              <Space wrap>
                <Button
                  disabled={checkAuthBlocked}
                  htmlType="submit"
                  icon={<CheckCircleOutlined />}
                  loading={checking}
                  type="primary"
                >
                  {authT("panel.checkAuth")}
                </Button>
                <Button
                  disabled={checking}
                  onClick={() => {
                    form.resetFields();
                    form.setFieldsValue({
                      authMode: preferredMode,
                      bearerToken: "",
                      ldapPassword: "",
                      ldapUsername: "",
                    });
                    setValues({ authMode: preferredMode });
                    setResult(null);
                    onProofChange(defaultDiagnosisAuthLiveProof(preferredMode));
                  }}
                  type="default"
                >
                  {authT("panel.reset")}
                </Button>
              </Space>
            </Form>
          </Col>
          <Col lg={14} xs={24}>
            <Space direction="vertical" size={12} style={{ width: "100%" }}>
              <DiagnosisAuthReadinessSummaryPanel
                summary={authReadinessSummary}
              />
              <Alert
                description={backendReadiness.detail}
                message={backendReadiness.label}
                showIcon
                type={authProbeBackendAlertType(backendReadiness.status)}
              />
              <Alert
                description={diagnosisAuthStatusDetail(backendStatus, authT)}
                message={authT("panel.backendWiring")}
                showIcon
                type={diagnosisAuthStatusAlertType(backendStatus)}
              />
              {mode === "session" || mode === "wecom" ? (
                <Alert
                  action={
                    <Space wrap>
                      {diagnosisBrowserSessionNeedsIAMSignIn(browserSession) ? (
                        <Button
                          href={diagnosisAuthReadinessLoginHref}
                          icon={<LoginOutlined />}
                          type="primary"
                        >
                          {authT("panel.signInWithIAM")}
                        </Button>
                      ) : null}
                      <Button
                        disabled={
                          browserSession.loading || browserSessionClearing
                        }
                        icon={<ReloadOutlined />}
                        onClick={onRefreshBrowserSession}
                      >
                        {authT("panel.refreshSession")}
                      </Button>
                      {browserSession.status?.authenticated === true ? (
                        <Button
                          disabled={browserSession.loading}
                          icon={<DisconnectOutlined />}
                          loading={browserSessionClearing}
                          onClick={onSignOutBrowserSession}
                        >
                          {authT("panel.signOut")}
                        </Button>
                      ) : null}
                    </Space>
                  }
                  description={diagnosisBrowserSessionStatusDetail(
                    browserSession,
                    authT,
                  )}
                  message={
                    mode === "wecom"
                      ? authT("panel.weComBrowserSession")
                      : authT("panel.browserSession")
                  }
                  showIcon
                  type={diagnosisBrowserSessionStatusAlertType(browserSession)}
                />
              ) : null}
              <Alert
                description={rolloutReadiness.detail}
                message={rolloutReadiness.label}
                showIcon
                type={diagnosisAuthRolloutAlertType(rolloutReadiness.status)}
              />
              <Typography.Text type="secondary">
                {authT("panel.credentialsTransient")}
              </Typography.Text>
              <Typography.Text type="secondary">
                {readiness.detail}
              </Typography.Text>
              <Descriptions
                bordered
                column={1}
                items={diagnosisAuthProbeDescriptions(
                  readiness.mode,
                  displayedResult,
                  authT,
                  locale,
                )}
                size="small"
              />
              <Descriptions
                bordered
                column={1}
                items={diagnosisAuthRolloutDescriptions(
                  rolloutReadiness,
                  backendStatus,
                  authT,
                  locale,
                )}
                size="small"
              />
            </Space>
          </Col>
        </Row>
      </div>
    </section>
  );
}

export function defaultDiagnosisAuthLiveProof(
  mode: DiagnosisAuthMode = "session",
): DiagnosisAuthLiveProof {
  return {
    detail: "",
    mode,
    roles: [],
    status: "pending",
    subject: "",
  };
}

async function diagnosisBrowserSessionStateFromFetch(): Promise<DiagnosisBrowserSessionState> {
  return diagnosisBrowserSessionStateFromResult(
    await fetchDiagnosisBrowserSession(),
  );
}

export function diagnosisBrowserSessionStateFromResult(
  result: Awaited<ReturnType<typeof fetchDiagnosisBrowserSession>>,
): DiagnosisBrowserSessionState {
  if (!result.ok) {
    return {
      loading: false,
      message: result.error.message,
      status: null,
    };
  }
  return {
    loading: false,
    message: "",
    status: result.data,
  };
}

export function diagnosisAuthProbeBrowserSessionBlockReason(
  values: DiagnosisAuthInputValues,
  browserSession: {
    loading: boolean;
    status: DiagnosisBrowserSessionStatus | null;
  },
  t: DiagnosisAuthTranslator,
): string {
  const authenticated =
    browserSession.status?.authenticated === true
      ? browserSession.status
      : null;
  return diagnosisAuthBrowserSessionBlockReason(
    {
      intent: "check",
      sessionAuthenticated: authenticated !== null,
      sessionLoading: browserSession.loading,
      sessionMode: authenticated?.mode,
      sessionStatusAvailable:
        browserSession.loading || browserSession.status !== null,
      values,
    },
    t,
  );
}

function diagnosisAuthProbeDisplayedResult(
  values: DiagnosisAuthInputValues,
  result: DiagnosisAuthBackendCheck | null,
  browserSession: DiagnosisBrowserSessionStatus | null,
): DiagnosisAuthBackendCheck | null {
  if (result !== null) {
    return result;
  }
  const mode = values.authMode ?? "session";
  if (mode !== "session" && mode !== "wecom") {
    return null;
  }
  const proof = diagnosisAuthLiveProofFromBrowserSession(browserSession);
  if (proof === null) {
    return null;
  }
  if (mode === "wecom" && proof.mode !== "wecom") {
    return null;
  }
  return diagnosisAuthProbeResultFromLiveProof({
    ...proof,
    mode,
  });
}

export function diagnosisAuthLiveProofFromProbeState(
  {
    backendStatus,
    browserSession,
    checking,
    result,
    values,
  }: {
    backendStatus?: DiagnosisAuthBackendStatusSnapshot;
    browserSession: DiagnosisBrowserSessionStatus | null;
    checking: boolean;
    result: DiagnosisAuthBackendCheck | null;
    values: DiagnosisAuthInputValues;
  },
  t: DiagnosisAuthTranslator,
): DiagnosisAuthLiveProof {
  return diagnosisAuthLiveProofFromBackend(
    {
      backendStatus,
      checking,
      lastCheck: diagnosisAuthProbeDisplayedResult(
        values,
        result,
        browserSession,
      ),
      values,
    },
    t,
  );
}

export function diagnosisAuthProbeCheckBlocked({
  backendStatusAvailable = true,
  backendStatus,
  checking,
  inputStatus,
}: {
  backendStatusAvailable?: boolean;
  backendStatus: DiagnosisAuthBackendReadiness["status"];
  checking: boolean;
  inputStatus: ReturnType<typeof diagnosisAuthInputReadiness>["status"];
}): boolean {
  return (
    !backendStatusAvailable ||
    checking ||
    inputStatus !== "ready" ||
    backendStatus === "blocked"
  );
}

function diagnosisAuthLiveProofFromBackend(
  {
    backendStatus,
    checking,
    lastCheck,
    values,
  }: {
    backendStatus?: DiagnosisAuthBackendStatusSnapshot;
    checking: boolean;
    lastCheck: DiagnosisAuthProbeResult | null;
    values: DiagnosisAuthProbeFormValues;
  },
  t: DiagnosisAuthTranslator,
): DiagnosisAuthLiveProof {
  const inputReadiness = diagnosisAuthInputReadiness(values, t);
  const backendBlocked = diagnosisAuthBackendProofBlock(
    backendStatus,
    inputReadiness.mode,
  );
  if (backendBlocked !== null) {
    return backendBlocked;
  }
  const backendReadiness = diagnosisAuthBackendReadiness(
    {
      backendStatus,
      checking,
      lastCheck,
      values,
    },
    t,
  );
  if (
    backendReadiness.status === "verified" &&
    lastCheck?.status === "success"
  ) {
    return diagnosisAuthLiveProofFromResult(lastCheck);
  }
  return {
    detail: "",
    mode: inputReadiness.mode,
    roles: [],
    status: backendReadiness.status,
    subject: "",
  };
}

function diagnosisAuthBackendProofBlock(
  backendStatus: DiagnosisAuthBackendStatusSnapshot | undefined,
  mode: DiagnosisAuthMode,
): DiagnosisAuthLiveProof | null {
  if (backendStatus === undefined) {
    return null;
  }
  if (!backendStatus.configured || backendStatus.mode === "none") {
    return {
      detail: "",
      mode,
      roles: [],
      status: "blocked",
      subject: "",
    };
  }
  if (backendStatus.mode === "unknown") {
    return {
      detail: "",
      mode,
      roles: [],
      status: "blocked",
      subject: "",
    };
  }
  return null;
}

function diagnosisAuthLiveProofFromResult(
  result: DiagnosisAuthProbeResult,
): DiagnosisAuthLiveProof {
  const roleAuthorized = diagnosisAuthProbeResultAuthorized(result);
  const identityVerified = result.status === "success";
  return {
    checkedAt: result.checkedAt,
    detail: result.message,
    mode: result.mode,
    roleAuthorized,
    roles: result.roles,
    status: identityVerified ? "verified" : "failed",
    subject: result.subject,
  };
}

export function diagnosisAuthLiveProofFromBrowserSession(
  session: DiagnosisBrowserSessionStatus | null,
): DiagnosisAuthLiveProof | null {
  if (session === null || !session.authenticated) {
    return null;
  }
  const mode = diagnosisAuthProbeModeFromCheckMode(session.mode);
  if (mode === null) {
    return null;
  }
  const roleAuthorized =
    session.role_authorized ??
    session.roles.some((role) => role === "owner" || role === "admin");
  return {
    checkedAt: session.checked_at,
    detail: "",
    mode,
    roleAuthorized,
    roles: session.roles,
    status: "verified",
    subject: session.subject,
  };
}

export function diagnosisAuthProbeResultFromBrowserSessionStatus(
  session: DiagnosisBrowserSessionStatus,
): DiagnosisAuthProbeResult | null {
  const proof = diagnosisAuthLiveProofFromBrowserSession(session);
  if (proof === null) {
    return null;
  }
  return diagnosisAuthProbeResultFromLiveProof({
    ...proof,
    mode: "session",
  });
}

function diagnosisAuthProbeResultFromLiveProof(
  proof: DiagnosisAuthLiveProof | null,
): DiagnosisAuthProbeResult | null {
  if (proof === null) {
    return null;
  }
  return {
    checkedAt: proof.checkedAt,
    message: proof.detail,
    mode: proof.mode,
    roleAuthorized: proof.roleAuthorized,
    roles: proof.roles,
    status: proof.status === "verified" ? "success" : "failed",
    subject: proof.subject,
  };
}

function diagnosisAuthProbeModeFromCheckMode(
  mode: DiagnosisAuthStatus["mode"],
): DiagnosisAuthMode | null {
  switch (mode) {
    case "ldap":
      return "ldap";
    case "static":
      return "bearer";
    case "oidc":
      return "session";
    case "unknown":
    case "none":
      return null;
  }
}

function diagnosisAuthBrowserSessionSourceValueLabel(
  mode: DiagnosisAuthStatus["mode"],
  t: DiagnosisAuthTranslator,
): string {
  switch (mode) {
    case "ldap":
      return t("provider.ldap");
    case "static":
      return t("provider.staticBearerAuth");
    case "oidc":
      return t("provider.iamOIDC");
    case "unknown":
    case "none":
      return t("provider.configuredBackend");
  }
}

function diagnosisAuthProbeResultAuthorized(
  result: DiagnosisAuthProbeResult,
): boolean {
  if (result.roleAuthorized !== undefined) {
    return result.roleAuthorized;
  }
  return result.roles.some((role) => role === "owner" || role === "admin");
}

function diagnosisAuthorizationFromProbeValues(
  values: DiagnosisAuthProbeFormValues,
): DiagnosisAuthorization | null {
  const mode = values.authMode ?? "session";
  if (mode === "session" || mode === "wecom") {
    return normalizedDiagnosisAuthorization({ mode: "session" });
  }
  if (mode === "bearer") {
    return normalizedDiagnosisAuthorization({
      mode: "bearer",
      token: values.bearerToken ?? "",
    });
  }
  return normalizedDiagnosisAuthorization({
    mode: "basic",
    password: values.ldapPassword ?? "",
    username: values.ldapUsername ?? "",
  });
}

export function settingsLDAPTransportGuidance(
  t: DiagnosisAuthTranslator,
): SettingsLDAPConfigurationGuidance[] {
  return [
    {
      detail: t("guidance.ldap.transport.url.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_URL",
      label: t("guidance.ldap.transport.url.label"),
      value: t("guidance.ldap.transport.url.value"),
    },
    {
      detail: t("guidance.ldap.transport.startTLS.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_START_TLS",
      label: t("guidance.ldap.transport.startTLS.label"),
      value: t("guidance.ldap.transport.startTLS.value"),
    },
    {
      detail: t("guidance.ldap.transport.plaintext.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT",
      label: t("guidance.ldap.transport.plaintext.label"),
      value: t("guidance.ldap.transport.plaintext.value"),
    },
  ];
}

export function settingsLDAPDirectorySearchGuidance(
  t: DiagnosisAuthTranslator,
): SettingsLDAPConfigurationGuidance[] {
  return [
    {
      detail: t("guidance.ldap.search.baseDN.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
      label: t("guidance.ldap.search.baseDN.label"),
      value: t("guidance.ldap.search.baseDN.value"),
    },
    {
      detail: t("guidance.ldap.search.bindDN.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN",
      label: t("guidance.ldap.search.bindDN.label"),
      value: t("guidance.ldap.search.bindDN.value"),
    },
    {
      detail: t("guidance.ldap.search.bindPassword.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD",
      label: t("guidance.ldap.search.bindPassword.label"),
      value: t("guidance.ldap.search.bindPassword.value"),
    },
    {
      detail: t("guidance.ldap.search.userFilter.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER",
      label: t("guidance.ldap.search.userFilter.label"),
      value: t("guidance.ldap.search.userFilter.value"),
    },
    {
      detail: t("guidance.ldap.search.subjectAttribute.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE",
      label: t("guidance.ldap.search.subjectAttribute.label"),
      value: t("guidance.ldap.search.subjectAttribute.value"),
    },
  ];
}

export function settingsLDAPRoleMappingGuidance(
  t: DiagnosisAuthTranslator,
): SettingsLDAPConfigurationGuidance[] {
  return [
    {
      detail: t("guidance.ldap.roles.attribute.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE",
      label: t("guidance.ldap.roles.attribute.label"),
      value: t("guidance.ldap.roles.attribute.value"),
    },
    {
      detail: t("guidance.ldap.roles.ownerValues.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES",
      label: t("guidance.ldap.roles.ownerValues.label"),
      value: t("guidance.ldap.roles.ownerValues.value"),
    },
    {
      detail: t("guidance.ldap.roles.adminValues.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES",
      label: t("guidance.ldap.roles.adminValues.label"),
      value: t("guidance.ldap.roles.adminValues.value"),
    },
    {
      detail: t("guidance.ldap.roles.defaultRoles.detail"),
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES",
      label: t("guidance.ldap.roles.defaultRoles.label"),
      value: t("guidance.ldap.roles.defaultRoles.value"),
    },
  ];
}

export function settingsWeComBrowserSessionReadiness(
  browserSession: DiagnosisBrowserSessionState,
  t: DiagnosisAuthTranslator,
  locale: string,
): SettingsWeComBrowserSessionReadiness {
  if (browserSession.loading) {
    return {
      detail: t("weComSession.loading.detail"),
      items: [
        {
          detail: t("weComSession.loading.sessionDetail"),
          key: "session",
          label: t("weComSession.field.session"),
          status: "loading",
          value: t("roleStatus.loadingLabel"),
        },
        {
          detail: t("weComSession.loading.providerDetail"),
          key: "provider",
          label: t("weComSession.field.provider"),
          status: "loading",
          value: t("roleStatus.loadingLabel"),
        },
        {
          detail: t("weComSession.loading.roleDetail"),
          key: "role",
          label: t("weComSession.field.role"),
          status: "loading",
          value: t("roleStatus.loadingLabel"),
        },
        {
          detail: t("weComSession.loading.subjectDetail"),
          key: "subject",
          label: t("weComSession.field.subject"),
          status: "loading",
          value: t("roleStatus.loadingLabel"),
        },
      ],
      label: t("weComSession.loading.label"),
      status: "loading",
    };
  }
  if (browserSession.status === null) {
    const detail =
      browserSession.message.trim() || t("weComSession.unavailable.detail");
    return {
      detail,
      items: [
        {
          detail,
          key: "session",
          label: t("weComSession.field.session"),
          status: "unavailable",
          value: t("roleStatus.unavailableLabel"),
        },
        {
          detail: t("weComSession.unavailable.providerDetail"),
          key: "provider",
          label: t("weComSession.field.provider"),
          status: "unavailable",
          value: t("roleStatus.unavailableLabel"),
        },
        {
          detail: t("weComSession.unavailable.roleDetail"),
          key: "role",
          label: t("weComSession.field.role"),
          status: "unavailable",
          value: t("roleStatus.unavailableLabel"),
        },
        {
          detail: t("weComSession.unavailable.subjectDetail"),
          key: "subject",
          label: t("weComSession.field.subject"),
          status: "unavailable",
          value: t("roleStatus.unavailableLabel"),
        },
      ],
      label: t("weComSession.unavailable.label"),
      status: "unavailable",
    };
  }
  if (!browserSession.status.authenticated) {
    return {
      detail: t("weComSession.signedOut.detail"),
      items: [
        {
          detail: t("weComSession.signedOut.sessionDetail"),
          key: "session",
          label: t("weComSession.field.session"),
          status: "blocked",
          value: t("settings.summary.notSignedIn"),
        },
        {
          detail: t("weComSession.signedOut.providerDetail"),
          key: "provider",
          label: t("weComSession.field.provider"),
          status: "blocked",
          value: t("weComSession.signedOut.providerValue"),
        },
        {
          detail: t("weComSession.signedOut.roleDetail"),
          key: "role",
          label: t("weComSession.field.role"),
          status: "blocked",
          value: t("weComSession.signedOut.roleValue"),
        },
        {
          detail: t("weComSession.signedOut.subjectDetail"),
          key: "subject",
          label: t("weComSession.field.subject"),
          status: "unavailable",
          value: t("weComSession.notAvailable"),
        },
      ],
      label: t("weComSession.signedOut.label"),
      status: "blocked",
    };
  }

  const session = browserSession.status;
  const providerValue = diagnosisAuthBrowserSessionSourceValueLabel(
    session.mode,
    t,
  );
  const subject = session.subject.trim() || t("roleStatus.notReportedLabel");
  const roles =
    session.roles.length === 0
      ? t("common.noRoles")
      : new Intl.ListFormat(locale, {
          style: "long",
          type: "conjunction",
        }).format(session.roles);
  const status: SettingsWeComReadinessStatus = "blocked";
  return {
    checkedAt: session.checked_at,
    detail: t("weComSession.authenticated.detail"),
    items: [
      {
        detail: t("weComSession.authenticated.migratedSessionDetail"),
        key: "session",
        label: t("weComSession.field.session"),
        status,
        value: t("settings.summary.sourceSession", { source: providerValue }),
      },
      {
        detail: t("weComSession.authenticated.migratedProviderDetail"),
        key: "provider",
        label: t("weComSession.field.provider"),
        status: "blocked",
        value: providerValue,
      },
      {
        detail: t("weComSession.authenticated.roleDetail"),
        key: "role",
        label: t("weComSession.field.role"),
        status: "ready",
        value: roles,
      },
      {
        detail: t("weComSession.authenticated.subjectDetail"),
        key: "subject",
        label: t("weComSession.field.subject"),
        status: "ready",
        value: subject,
      },
    ],
    label: t("weComSession.authenticated.label"),
    status,
  };
}

function DiagnosisAuthWeComSessionSummary({
  readiness,
}: {
  readiness: SettingsWeComBrowserSessionReadiness;
}) {
  const authT = useTranslations("DiagnosisAuth");
  const locale = useLocale();
  return (
    <Alert
      description={
        <Space direction="vertical" size={6} style={{ width: "100%" }}>
          <Typography.Text>{readiness.detail}</Typography.Text>
          <Descriptions column={1} size="small">
            {readiness.items.map((item) => (
              <Descriptions.Item key={item.key} label={item.label}>
                <Space direction="vertical" size={4}>
                  <Space size={6} wrap>
                    <Tag color={settingsWeComReadinessStatusColor(item.status)}>
                      {settingsWeComReadinessStatusLabel(item.status, authT)}
                    </Tag>
                    <Typography.Text>{item.value}</Typography.Text>
                  </Space>
                  <Typography.Text type="secondary">
                    {item.detail}
                  </Typography.Text>
                </Space>
              </Descriptions.Item>
            ))}
          </Descriptions>
          {readiness.checkedAt === undefined ? null : (
            <Typography.Text type="secondary">
              {authT("probe.checkedAtText", {
                checkedAt: formatDateTime(readiness.checkedAt, locale),
              })}
            </Typography.Text>
          )}
        </Space>
      }
      message={readiness.label}
      showIcon
      type={settingsWeComReadinessAlertType(readiness.status)}
    />
  );
}

function diagnosisAuthProbeDescriptions(
  mode: DiagnosisAuthMode,
  result: DiagnosisAuthProbeResult | null,
  t: DiagnosisAuthTranslator,
  locale: string,
) {
  if (result === null) {
    return [
      {
        key: "mode",
        label: t("probe.field.mode"),
        children: authProbeModeLabel(mode, t),
      },
      {
        key: "result",
        label: t("probe.field.backendCheck"),
        children: t("probe.notChecked"),
      },
    ];
  }
  const roles =
    result.roles.length === 0
      ? t("common.noRoles")
      : new Intl.ListFormat(locale, {
          style: "long",
          type: "conjunction",
        }).format(result.roles);
  return [
    {
      key: "mode",
      label: t("probe.field.mode"),
      children: authProbeModeLabel(result.mode, t),
    },
    {
      key: "status",
      label: t("probe.field.backendCheck"),
      children: (
        <Space size={6} wrap>
          <Tag color={result.status === "success" ? "green" : "red"}>
            {result.status === "success"
              ? t("probe.success")
              : t("ui.backendStatus.failed")}
          </Tag>
          <Typography.Text
            type={result.status === "success" ? "secondary" : "danger"}
          >
            {diagnosisAuthProbeResultDetail(result, t)}
          </Typography.Text>
        </Space>
      ),
    },
    {
      key: "subject",
      label: t("probe.field.subject"),
      children:
        result.subject === "" ? t("probe.notAvailable") : result.subject,
    },
    {
      key: "roles",
      label: t("probe.field.roles"),
      children: roles,
    },
    {
      key: "role-authorized",
      label: t("probe.field.providerRoleMetadata"),
      children:
        (result.roleAuthorized ?? diagnosisAuthProbeResultAuthorized(result))
          ? t("probe.reported")
          : t("probe.notRequired"),
    },
    {
      key: "checked-at",
      label: t("probe.field.checkedAt"),
      children:
        result.checkedAt === undefined || result.checkedAt.trim() === ""
          ? t("probe.notAvailable")
          : formatDateTime(result.checkedAt, locale),
    },
  ];
}

export function diagnosisAuthProbeResultDetail(
  result: Pick<DiagnosisAuthProbeResult, "localFailure" | "message" | "status">,
  t: DiagnosisAuthTranslator,
): string {
  if (result.message.trim() !== "") {
    return result.message;
  }
  if (result.status === "success") {
    return t("feedback.checkSucceeded");
  }
  switch (result.localFailure) {
    case "backend_readiness":
      return t("feedback.backendReadinessBlocked");
    case "browser_session":
      return t("feedback.browserSessionBlocked");
    case "credentials":
      return t("feedback.credentialsInvalid");
    case undefined:
      return t("backend.checkFailedDetail");
  }
}

function diagnosisAuthRolloutDescriptions(
  readiness: DiagnosisAuthRolloutReadiness,
  backendStatus: DiagnosisAuthStatusState,
  t: DiagnosisAuthTranslator,
  locale: string,
) {
  return [
    {
      key: "required-provider",
      label: t("probe.field.requiredProvider"),
      children: t("probe.requiredProviderValue"),
    },
    {
      key: "backend-provider",
      label: t("probe.field.backendProvider"),
      children: diagnosisAuthStatusLabel(backendStatus, t),
    },
    {
      key: "supported-modes",
      label: t("probe.field.supportedModes"),
      children: (
        <DiagnosisAuthSupportedModesSummary status={backendStatus.status} />
      ),
    },
    {
      key: "proof-mode",
      label: t("probe.field.checkedMode"),
      children: authProbeModeLabel(readiness.mode, t),
    },
    {
      key: "owner-admin",
      label: t("probe.field.providerRoles"),
      children: (
        <Tag color={readiness.roleAuthorized === true ? "green" : "gold"}>
          {readiness.roleAuthorized === true
            ? t("probe.reported")
            : t("probe.optional")}
        </Tag>
      ),
    },
    {
      key: "backend-role-mapping",
      label: t("probe.field.backendMapping"),
      children: (
        <DiagnosisAuthRoleMappingSummary
          loading={backendStatus.loading}
          status={backendStatus.status}
        />
      ),
    },
    {
      key: "role-mapping-guidance",
      label: t("probe.field.roleMapping"),
      children: (
        <Typography.Text type="secondary">
          {diagnosisAuthRoleMappingGuidance(readiness, t)}
        </Typography.Text>
      ),
    },
    {
      key: "rollout-decision",
      label: t("probe.field.rolloutDecision"),
      children: (
        <Space size={6} wrap>
          <Tag color={diagnosisAuthRolloutTagColor(readiness.status)}>
            {diagnosisAuthRolloutStatusLabel(readiness.status, t)}
          </Tag>
          <Typography.Text type="secondary">
            {readiness.subject === ""
              ? t("probe.noSubjectChecked")
              : readiness.subject}
          </Typography.Text>
        </Space>
      ),
    },
    {
      key: "checked-at",
      label: t("probe.field.checkedAt"),
      children:
        readiness.checkedAt === undefined || readiness.checkedAt.trim() === ""
          ? t("probe.notAvailable")
          : formatDateTime(readiness.checkedAt, locale),
    },
  ];
}

function DiagnosisAuthRoleMappingSummary({
  loading,
  status,
}: {
  loading: boolean;
  status: DiagnosisAuthStatus | null;
}) {
  const authT = useTranslations("DiagnosisAuth");
  const readiness = diagnosisAuthRoleMappingStatusReadiness(
    status,
    authT,
    loading,
  );
  return (
    <Space direction="vertical" size={4}>
      <Tag color={readiness.color}>{readiness.label}</Tag>
      <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
    </Space>
  );
}

function DiagnosisAuthReadinessSummaryPanel({
  summary,
}: {
  summary: DiagnosisAuthReadinessSummary;
}) {
  return (
    <Alert
      description={
        <Space direction="vertical" size={6}>
          <Typography.Text>{summary.detail}</Typography.Text>
          <Space size={6} wrap>
            {summary.items.map((item) => (
              <Tag
                color={proofTargetStatusColor(item.status)}
                key={item.key}
                title={item.detail}
              >
                {item.label}: {item.value}
              </Tag>
            ))}
          </Space>
        </Space>
      }
      message={summary.label}
      showIcon
      type={diagnosisAuthReadinessSummaryAlertType(summary.status)}
    />
  );
}

function DiagnosisLDAPBrowserSessionPromotionNotice() {
  const authT = useTranslations("DiagnosisAuth");
  const notice = diagnosisAuthLDAPBrowserSessionPromotionNotice(authT);
  return (
    <Alert
      description={notice.detail}
      message={notice.message}
      showIcon
      type="info"
    />
  );
}

function DiagnosisAuthLDAPSetupSummary({
  readiness,
}: {
  readiness: DiagnosisAuthLDAPSetupReadiness;
}) {
  const authT = useTranslations("DiagnosisAuth");
  return (
    <Alert
      description={
        <Space direction="vertical" size={6} style={{ width: "100%" }}>
          <Typography.Text>{readiness.detail}</Typography.Text>
          <Descriptions column={1} size="small">
            {readiness.items.map((item) => (
              <Descriptions.Item key={item.key} label={item.label}>
                <Space direction="vertical" size={4}>
                  <Tag color={diagnosisAuthLDAPSetupItemColor(item.status)}>
                    {diagnosisAuthSetupStatusLabel(item.status, authT)}
                  </Tag>
                  <Typography.Text type="secondary">
                    {item.detail}
                  </Typography.Text>
                </Space>
              </Descriptions.Item>
            ))}
            <Descriptions.Item label={authT("guidance.ldap.group.transport")}>
              <DiagnosisAuthLDAPConfigurationGuidanceValue
                items={settingsLDAPTransportGuidance(authT)}
              />
            </Descriptions.Item>
            <Descriptions.Item label={authT("guidance.ldap.group.search")}>
              <DiagnosisAuthLDAPConfigurationGuidanceValue
                items={settingsLDAPDirectorySearchGuidance(authT)}
              />
            </Descriptions.Item>
            <Descriptions.Item label={authT("guidance.ldap.group.roles")}>
              <DiagnosisAuthLDAPConfigurationGuidanceValue
                items={settingsLDAPRoleMappingGuidance(authT)}
              />
            </Descriptions.Item>
          </Descriptions>
        </Space>
      }
      message={readiness.label}
      showIcon
      type={diagnosisAuthLDAPSetupAlertType(readiness.status)}
    />
  );
}

function DiagnosisAuthLDAPConfigurationGuidanceValue({
  items,
}: {
  items: SettingsLDAPConfigurationGuidance[];
}) {
  return (
    <Space direction="vertical" size={6}>
      {items.map((item) => (
        <Space direction="vertical" key={item.envVar} size={2}>
          <Space size={6} wrap>
            <Tag>{item.value}</Tag>
            <Typography.Text>{item.label}</Typography.Text>
          </Space>
          <Typography.Text type="secondary">{item.detail}</Typography.Text>
          <Typography.Text
            className="settings-event-ids settings-ldap-config-env"
            copyable={{ text: item.envVar }}
            type="secondary"
          >
            {item.envVar}
          </Typography.Text>
        </Space>
      ))}
    </Space>
  );
}

function DiagnosisAuthWeComSetupSummary({
  readiness,
}: {
  readiness: DiagnosisAuthWeComSetupReadiness;
}) {
  const authT = useTranslations("DiagnosisAuth");
  return (
    <Alert
      description={
        <Space direction="vertical" size={6} style={{ width: "100%" }}>
          <Typography.Text>{readiness.detail}</Typography.Text>
          <Descriptions column={1} size="small">
            {readiness.items.map((item) => (
              <Descriptions.Item key={item.key} label={item.label}>
                <Space direction="vertical" size={4}>
                  <Tag color={diagnosisAuthWeComSetupItemColor(item.status)}>
                    {diagnosisAuthSetupStatusLabel(item.status, authT)}
                  </Tag>
                  <Typography.Text type="secondary">
                    {item.detail}
                  </Typography.Text>
                </Space>
              </Descriptions.Item>
            ))}
          </Descriptions>
        </Space>
      }
      message={readiness.label}
      showIcon
      type={diagnosisAuthWeComSetupAlertType(readiness.status)}
    />
  );
}

export function settingsWeComAuthErrorDetail(
  error: DiagnosisRoomWeComAuthError,
  t: DiagnosisAuthTranslator,
): { description: string; message: string } {
  switch (error) {
    case "wecom_entry_unavailable":
      return {
        description: t("weComError.entryUnavailable.description"),
        message: t("weComError.entryUnavailable.message"),
      };
    case "wecom_auth_failed":
      return {
        description: t("weComError.authFailed.description"),
        message: t("weComError.authFailed.message"),
      };
    case "wecom_role_unauthorized":
      return {
        description: t("weComError.roleUnauthorized.description"),
        message: t("weComError.roleUnauthorized.message"),
      };
    case "wecom_callback_failed":
      return {
        description: t("weComError.callbackFailed.description"),
        message: t("weComError.callbackFailed.message"),
      };
    case "wecom_callback_missing":
      return {
        description: t("weComError.callbackMissing.description"),
        message: t("weComError.callbackMissing.message"),
      };
    case "wecom_login_failed":
      return {
        description: t("weComError.loginFailed.description"),
        message: t("weComError.loginFailed.message"),
      };
  }
}

function settingsWeComReadinessAlertType(status: SettingsWeComReadinessStatus) {
  switch (status) {
    case "ready":
      return "success";
    case "loading":
      return "info";
    case "review":
    case "unavailable":
      return "warning";
    case "blocked":
      return "error";
  }
}

function settingsWeComReadinessStatusColor(
  status: SettingsWeComReadinessStatus,
): string {
  switch (status) {
    case "ready":
      return "success";
    case "loading":
      return "processing";
    case "review":
      return "warning";
    case "blocked":
      return "error";
    case "unavailable":
      return "default";
  }
}

function settingsWeComReadinessStatusLabel(
  status: SettingsWeComReadinessStatus,
  t: DiagnosisAuthTranslator,
): string {
  switch (status) {
    case "ready":
      return t("ui.ready");
    case "review":
      return t("settings.summary.reviewStatus");
    case "blocked":
      return t("ui.blocked");
    case "loading":
      return t("roleStatus.loadingLabel");
    case "unavailable":
      return t("roleStatus.unavailableLabel");
  }
}

function diagnosisAuthLDAPSetupAlertType(
  status: DiagnosisAuthLDAPSetupReadiness["status"],
) {
  switch (status) {
    case "ready":
      return "success";
    case "blocked":
      return "error";
    case "loading":
    case "unavailable":
    case "review":
      return "warning";
  }
}

function diagnosisAuthLDAPSetupItemColor(
  status: DiagnosisAuthLDAPSetupReadiness["status"],
): string {
  switch (status) {
    case "ready":
      return "success";
    case "loading":
      return "processing";
    case "blocked":
      return "error";
    case "review":
      return "warning";
    case "unavailable":
      return "default";
  }
}

function diagnosisAuthWeComSetupAlertType(
  status: DiagnosisAuthWeComSetupReadiness["status"],
) {
  switch (status) {
    case "ready":
      return "success";
    case "blocked":
      return "error";
    case "loading":
    case "unavailable":
    case "review":
      return "warning";
  }
}

function diagnosisAuthWeComSetupItemColor(
  status: DiagnosisAuthWeComSetupReadiness["status"],
): string {
  switch (status) {
    case "ready":
      return "success";
    case "loading":
      return "processing";
    case "blocked":
      return "error";
    case "review":
      return "warning";
    case "unavailable":
      return "default";
  }
}

function DiagnosisAuthSupportedModesSummary({
  status,
}: {
  status: DiagnosisAuthStatus | null;
}) {
  const authT = useTranslations("DiagnosisAuth");
  const items = diagnosisAuthBackendModeDisplayItems(status, authT);
  if (items.length === 0) {
    return (
      <Typography.Text type="secondary">
        {authT("roleStatus.notReportedLabel")}
      </Typography.Text>
    );
  }
  return (
    <Space size={6} wrap>
      {items.map((item) => (
        <Tag color={item.color} key={item.mode}>
          {item.label}
        </Tag>
      ))}
    </Space>
  );
}

function diagnosisAuthRolloutAlertType(
  status: DiagnosisAuthRolloutReadiness["status"],
) {
  switch (status) {
    case "ready":
      return "success";
    case "review":
    case "pending":
      return "warning";
    case "blocked":
      return "error";
  }
}

function diagnosisAuthRolloutTagColor(
  status: DiagnosisAuthRolloutReadiness["status"],
): string {
  switch (status) {
    case "ready":
      return "green";
    case "review":
    case "pending":
      return "gold";
    case "blocked":
      return "red";
  }
}

function diagnosisAuthRolloutStatusLabel(
  status: DiagnosisAuthRolloutReadiness["status"],
  t: DiagnosisAuthTranslator,
): string {
  switch (status) {
    case "ready":
      return t("ui.ready");
    case "review":
      return t("settings.summary.reviewStatus");
    case "pending":
      return t("ui.pending");
    case "blocked":
      return t("ui.blocked");
  }
}

function authProbeModeLabel(
  mode: DiagnosisAuthMode,
  t: DiagnosisAuthTranslator,
): string {
  switch (mode) {
    case "ldap":
      return t("probe.mode.ldap");
    case "bearer":
      return t("probe.mode.bearer");
    case "session":
      return t("probe.mode.session");
    case "wecom":
      return t("probe.mode.weCom");
  }
}

function authProbeModeValueLabel(
  mode: DiagnosisAuthMode,
  t: DiagnosisAuthTranslator,
): string {
  switch (mode) {
    case "ldap":
      return t("ui.modeLDAP");
    case "bearer":
      return t("ui.modeBearer");
    case "session":
      return t("ui.modeSession");
    case "wecom":
      return t("provider.weCom");
  }
}

function authProbeReadinessColor(
  status: ReturnType<typeof diagnosisAuthInputReadiness>["status"],
): string {
  switch (status) {
    case "ready":
      return "green";
    case "pending":
      return "gold";
    case "blocked":
      return "red";
  }
}

function authProbeBackendAlertType(
  status: ReturnType<typeof diagnosisAuthBackendReadiness>["status"],
) {
  switch (status) {
    case "verified":
      return "success";
    case "checking":
    case "needs_check":
    case "pending":
      return "warning";
    case "failed":
    case "blocked":
      return "error";
  }
}

export function diagnosisAuthProbeModeFromBackendStatus(
  status: DiagnosisAuthStatus | null,
): DiagnosisAuthMode | null {
  const supportedModes = status?.supported_modes ?? [];
  if (supportedModes.includes("oidc") || supportedModes.includes("ldap")) {
    return "session";
  }
  if (supportedModes.includes("static")) {
    return "bearer";
  }
  switch (status?.mode) {
    case "ldap":
    case "oidc":
      return "session";
    case "static":
      return "bearer";
    case "none":
    case "unknown":
    case undefined:
      return null;
  }
}

function diagnosisAuthBackendStatusSnapshot(
  status: DiagnosisAuthStatus | null,
): DiagnosisAuthBackendStatusSnapshot | undefined {
  if (status === null) {
    return undefined;
  }
  return {
    configured: status.configured,
    mode: status.mode,
    supportedModes: status.supported_modes,
  };
}

function diagnosisAuthStatusLabel(
  state: DiagnosisAuthStatusState,
  t: DiagnosisAuthTranslator,
): string {
  if (state.loading) {
    return t("ui.backendLoading");
  }
  if (state.status === null) {
    return t("ui.backendUnavailable");
  }
  const modesLabel = diagnosisAuthBackendModeListLabel(state.status, t);
  if (modesLabel !== "") {
    return t("ui.backendMode", { modes: modesLabel });
  }
  switch (state.status.mode) {
    case "ldap":
      return t("ui.backendLDAP");
    case "static":
      return t("ui.backendStatic");
    case "oidc":
      return t("ui.backendOIDC");
    case "unknown":
      return t("ui.backendUnknown");
    case "none":
      return t("ui.backendNotConfigured");
  }
}

function diagnosisAuthStatusDetail(
  state: DiagnosisAuthStatusState,
  t: DiagnosisAuthTranslator,
): string {
  if (state.loading) {
    return t("ui.statusChecking");
  }
  if (state.status === null) {
    return t("ui.statusLoadFailed", { error: state.message });
  }
  if (!state.status.configured || state.status.mode === "none") {
    return t("backend.notConfigured");
  }
  const supportedModes = diagnosisAuthBackendStatusModes(state.status);
  if (supportedModes.length > 1) {
    return diagnosisAuthMixedStatusDetail(state.status, t);
  }
  switch (state.status.mode) {
    case "ldap":
      return t("settings.statusLDAP");
    case "static":
      return t("settings.statusStatic");
    case "oidc":
      if (state.status.oidc_bff?.status === "blocked") {
        return t("settings.statusOIDCBlocked", {
          detail: diagnosisAuthOIDCBFFReadinessDetail(state.status.oidc_bff, t),
        });
      }
      return t("settings.statusOIDCReady");
    case "unknown":
      return t("ui.statusUnknown");
  }
}

function diagnosisAuthMixedStatusDetail(
  status: DiagnosisAuthStatus,
  t: DiagnosisAuthTranslator,
): string {
  const credentialLabel = diagnosisAuthBackendCredentialListLabel(status, t);
  return t("ui.statusMixed", { credentials: credentialLabel });
}

function diagnosisAuthStatusColor(state: DiagnosisAuthStatusState): string {
  if (state.loading) {
    return "processing";
  }
  if (
    state.status === null ||
    !state.status.configured ||
    state.status.mode === "none"
  ) {
    return "red";
  }
  if (state.status.mode === "ldap") {
    return "blue";
  }
  if (
    state.status.mode === "static" ||
    (state.status.mode === "oidc" &&
      state.status.oidc_bff?.status !== "blocked")
  ) {
    return "gold";
  }
  return "default";
}

function diagnosisAuthStatusAlertType(state: DiagnosisAuthStatusState) {
  if (state.loading) {
    return "info";
  }
  if (
    state.status === null ||
    !state.status.configured ||
    state.status.mode === "none"
  ) {
    return "error";
  }
  if (state.status.mode === "ldap") {
    return "success";
  }
  if (
    state.status.mode === "oidc" &&
    state.status.oidc_bff?.status === "blocked"
  ) {
    return "error";
  }
  return "warning";
}

export function settingsOIDCBFFSetupReadiness(
  status: DiagnosisAuthStatus | null,
  loading: boolean,
  t: DiagnosisAuthTranslator,
): SettingsOIDCBFFSetupReadiness | undefined {
  if (loading || status === null) {
    return undefined;
  }
  const oidcAdvertised =
    status.mode === "oidc" ||
    diagnosisAuthBackendStatusModes(status).includes("oidc");
  if (!oidcAdvertised) {
    return undefined;
  }
  const readiness = status.oidc_bff;
  if (readiness === undefined) {
    return {
      detail: t("settings.oidcStatusMissingDetail"),
      label: t("settings.oidcBlockedLabel"),
      status: "blocked",
      value: t("settings.oidcStatusMissingValue"),
    };
  }
  if (readiness.status === "ready") {
    return {
      detail: t("settings.oidcReadyDetail"),
      label: t("settings.oidcReadyLabel"),
      status: "ready",
      value: t("settings.oidcReadyValue"),
    };
  }
  return {
    detail: t("settings.oidcBlockedDetail", {
      detail: diagnosisAuthOIDCBFFReadinessDetail(readiness, t),
    }),
    label: t("settings.oidcBlockedLabel"),
    status: "blocked",
    value: diagnosisOIDCBFFReadinessValue(readiness, t),
  };
}

function diagnosisOIDCBFFReadinessValue(
  readiness: NonNullable<DiagnosisAuthStatus["oidc_bff"]>,
  t: DiagnosisAuthTranslator,
): string {
  const [firstMissing] = readiness.missing;
  if (readiness.missing.length === 1 && firstMissing !== undefined) {
    return t("settings.oidcMissingOne", {
      item: diagnosisAuthOIDCBFFMissingLabel(firstMissing, t),
    });
  }
  return t("settings.oidcMissingCount", { count: readiness.missing.length });
}

export function settingsLocalAccessReadiness(
  {
  directoryDepartments,
  directorySyncRuns,
  directoryUsers,
  rbacAssignments,
}: {
  directoryDepartments: readonly DirectoryDepartment[];
  directorySyncRuns?: readonly DirectorySyncRun[];
  directoryUsers: readonly DirectoryUser[];
  rbacAssignments: readonly RBACAssignment[];
  },
  t: SettingsOverviewTranslator,
  locale: string,
): SettingsLocalAccessReadiness {
  const activeUsers = directoryUsers.filter((user) => user.active).length;
  const enabledAssignments = rbacAssignments.filter(
    (assignment) => assignment.enabled,
  ).length;
  const syncItem = settingsDirectorySyncReadinessItem(
    t,
    locale,
    directorySyncRuns,
  );
  const items: SettingsLocalAccessReadinessItem[] = [
    ...(syncItem === null ? [] : [syncItem]),
    {
      detail:
        activeUsers > 0
          ? t("localAccess.users.readyDetail")
          : t("localAccess.users.blockedDetail"),
      key: "directory-users",
      label: t("localAccess.users.label"),
      status: activeUsers > 0 ? "ready" : "blocked",
      value:
        activeUsers === directoryUsers.length
          ? t("localAccess.users.allActive", { count: activeUsers })
          : t("localAccess.users.activeRatio", {
              active: activeUsers,
              total: directoryUsers.length,
            }),
    },
    {
      detail:
        directoryDepartments.length > 0
          ? t("localAccess.departments.readyDetail")
          : t("localAccess.departments.reviewDetail"),
      key: "directory-departments",
      label: t("localAccess.departments.label"),
      status: directoryDepartments.length > 0 ? "ready" : "review",
      value: t("localAccess.departments.value", {
        count: directoryDepartments.length,
      }),
    },
    {
      detail:
        enabledAssignments > 0
          ? t("localAccess.assignments.readyDetail")
          : t("localAccess.assignments.blockedDetail"),
      key: "rbac-assignments",
      label: t("localAccess.assignments.label"),
      status: enabledAssignments > 0 ? "ready" : "blocked",
      value:
        enabledAssignments === rbacAssignments.length
          ? t("localAccess.assignments.allEnabled", {
              count: enabledAssignments,
            })
          : t("localAccess.assignments.enabledRatio", {
              enabled: enabledAssignments,
              total: rbacAssignments.length,
            }),
    },
  ];
  const status = diagnosisAuthSummaryStatus(items);
  return {
    detail: settingsLocalAccessReadinessDetail(status, items, t),
    items,
    label: settingsLocalAccessReadinessLabel(status, t),
    status,
  };
}

function settingsDirectorySyncReadinessItem(
  t: SettingsOverviewTranslator,
  locale: string,
  directorySyncRuns?: readonly DirectorySyncRun[],
): SettingsLocalAccessReadinessItem | null {
  if (directorySyncRuns === undefined) {
    return null;
  }
  const latestRun = latestDirectorySyncRun(directorySyncRuns);
  if (latestRun === null) {
    return {
      detail: t("localAccess.sync.noRunsDetail"),
      key: "directory-sync",
      label: t("localAccess.sync.label"),
      status: "review",
      value: t("localAccess.sync.noRuns"),
    };
  }
  const syncedAt = formatDateTime(latestRun.synced_at, locale);
  const status: string = latestRun.status;
  if (status === "succeeded") {
    return {
      detail: t("localAccess.sync.succeededDetail"),
      key: "directory-sync",
      label: t("localAccess.sync.label"),
      status: "ready",
      value: t("localAccess.sync.succeededValue", { date: syncedAt }),
    };
  }
  if (status === "failed") {
    const reason =
      latestRun.failure_code.trim() === ""
        ? t("localAccess.sync.unknownFailure")
        : latestRun.failure_code.trim();
    return {
      detail: t("localAccess.sync.failedDetail", { reason }),
      key: "directory-sync",
      label: t("localAccess.sync.label"),
      status: "blocked",
      value: t("localAccess.sync.failedValue", { date: syncedAt }),
    };
  }
  return {
    detail: t("localAccess.sync.unknownDetail"),
    key: "directory-sync",
    label: t("localAccess.sync.label"),
    status: "review",
    value: t("localAccess.sync.unknownValue", { date: syncedAt, status }),
  };
}

function latestDirectorySyncRun(
  directorySyncRuns: readonly DirectorySyncRun[],
): DirectorySyncRun | null {
  let latestRun: DirectorySyncRun | null = null;
  let latestTime = Number.NEGATIVE_INFINITY;
  for (const run of directorySyncRuns) {
    const syncedTime = Date.parse(run.synced_at);
    const createdTime = Date.parse(run.created_at);
    const runTime = Number.isFinite(syncedTime)
      ? syncedTime
      : Number.isFinite(createdTime)
        ? createdTime
        : Number.NEGATIVE_INFINITY;
    if (latestRun === null || runTime > latestTime) {
      latestRun = run;
      latestTime = runTime;
    }
  }
  return latestRun;
}

function settingsLocalAccessSyncStatus(
  readiness: SettingsLocalAccessReadiness,
): ProofReadinessStatus {
  return (
    readiness.items.find((item) => item.key === "directory-sync")?.status ??
    "review"
  );
}

function settingsLocalAccessSyncValue(
  readiness: SettingsLocalAccessReadiness,
  t: SettingsOverviewTranslator,
): string {
  return (
    readiness.items.find((item) => item.key === "directory-sync")?.value ??
    t("localAccess.sync.notLoaded")
  );
}

function settingsLocalAccessReadinessDetail(
  status: ProofReadinessStatus,
  items: readonly SettingsLocalAccessReadinessItem[],
  t: SettingsOverviewTranslator,
): string {
  if (status === "ready") {
    return t("localAccess.readyDetail");
  }
  const item =
    items.find((candidate) => candidate.status === status) ??
    items.find((candidate) => candidate.status !== "ready");
  return item?.detail ?? t("localAccess.reviewDetail");
}

function settingsLocalAccessReadinessLabel(
  status: ProofReadinessStatus,
  t: SettingsOverviewTranslator,
): string {
  switch (status) {
    case "ready":
      return t("localAccess.status.ready");
    case "review":
      return t("localAccess.status.review");
    case "pending":
      return t("localAccess.status.pending");
    case "blocked":
      return t("localAccess.status.blocked");
  }
}

function diagnosisLocalAccessSummaryValue(
  readiness: SettingsLocalAccessReadiness,
): string {
  const assignment = readiness.items.find(
    (item) => item.key === "rbac-assignments",
  );
  return assignment?.value ?? readiness.label;
}

export function settingsCurrentRBACAccessSummary(
  state: CurrentRBACAuthorizationState,
  isChecking: boolean,
  t: SettingsOverviewTranslator,
  locale: string,
): SettingsCurrentRBACAccessSummary {
  if (isChecking || state.kind === "loading") {
    return {
      detail: t("currentAccess.checkingDetail"),
      items: [],
      label: t("currentAccess.status.pending"),
      needsSignIn: false,
      status: "pending",
      subject: "",
    };
  }
  if (state.kind === "error") {
    const needsSignIn = currentRBACAuthorizationNeedsSignIn(state);
    return {
      detail: needsSignIn ? t("currentAccess.signInDetail") : state.message,
      items: [],
      label: needsSignIn
        ? t("currentAccess.status.signIn")
        : t("currentAccess.status.unavailable"),
      needsSignIn,
      status: needsSignIn ? "blocked" : "review",
      subject: "",
    };
  }
  const activeDirectoryProfile = state.directoryUsers.some(
    (user) => user.active && user.subject === state.subject,
  );
  const accessItems = settingsCurrentRBACAccessItems(state, t);
  const items: SettingsCurrentRBACAccessItem[] = [
    {
      key: "directory-profile",
      label: t("currentAccess.profile.label"),
      status: activeDirectoryProfile ? "ready" : "review",
      value: activeDirectoryProfile
        ? t("currentAccess.profile.synced")
        : t("currentAccess.profile.notSynced"),
    },
    ...accessItems,
  ];
  const deniedItems = accessItems.filter((item) => item.status !== "ready");
  if (deniedItems.length > 0) {
    return {
      detail: t("currentAccess.deniedDetail", {
        permissions: new Intl.ListFormat(locale, {
          style: "long",
          type: "conjunction",
        }).format(deniedItems.map((item) => item.label)),
      }),
      items,
      label: t("currentAccess.status.limited"),
      needsSignIn: false,
      status: "review",
      subject: state.subject,
    };
  }
  if (!activeDirectoryProfile) {
    return {
      detail: t("currentAccess.profileReviewDetail"),
      items,
      label: t("currentAccess.status.review"),
      needsSignIn: false,
      status: "review",
      subject: state.subject,
    };
  }
  return {
    detail: t("currentAccess.readyDetail"),
    items,
    label: t("currentAccess.status.ready"),
    needsSignIn: false,
    status: "ready",
    subject: state.subject,
  };
}

function settingsCurrentRBACAccessItems(
  state: Extract<CurrentRBACAuthorizationState, { kind: "ready" }>,
  t: SettingsOverviewTranslator,
): SettingsCurrentRBACAccessItem[] {
  return settingsOverviewRBACAuthorizationChecks.map((check) => {
    const allowed = state.allowed[check.key] === true;
    return {
      key: check.key,
      label: settingsCurrentRBACAccessItemLabel(check.key, t),
      status: allowed ? "ready" : "blocked",
      value: allowed ? t("currentAccess.allowed") : t("currentAccess.denied"),
    };
  });
}

function settingsCurrentRBACAccessItemLabel(
  key: (typeof settingsOverviewRBACAuthorizationChecks)[number]["key"],
  t: SettingsOverviewTranslator,
): string {
  switch (key) {
    case "directory-read":
      return t("currentAccess.permissions.directoryRead");
    case "directory-sync":
      return t("currentAccess.permissions.directorySync");
    case "rbac-manage":
      return t("currentAccess.permissions.rbacManage");
    case "operations-read":
      return t("currentAccess.permissions.operationsRead");
  }
}

export function diagnosisAuthReadinessSummary(
  {
    backendReadiness,
    browserSession,
    ldapSetupReadiness,
    localAccessReadiness,
    mode,
    oidcSetupReadiness,
    rolloutReadiness,
    weComSetupReadiness,
  }: {
    backendReadiness: DiagnosisAuthBackendReadiness;
    browserSession: DiagnosisBrowserSessionState;
    ldapSetupReadiness: DiagnosisAuthLDAPSetupReadiness;
    localAccessReadiness?: SettingsLocalAccessReadiness;
    mode: DiagnosisAuthMode;
    oidcSetupReadiness?: SettingsOIDCBFFSetupReadiness;
    rolloutReadiness: DiagnosisAuthRolloutReadiness;
    weComSetupReadiness: DiagnosisAuthWeComSetupReadiness;
  },
  t: DiagnosisAuthTranslator,
): DiagnosisAuthReadinessSummary {
  const items = [
    diagnosisAuthSummaryBackendItem(backendReadiness, t),
    diagnosisAuthSummarySetupItem(
      {
        ldapSetupReadiness,
        mode,
        oidcSetupReadiness,
        weComSetupReadiness,
      },
      t,
    ),
    diagnosisAuthSummarySessionItem(mode, browserSession, t),
    ...(localAccessReadiness === undefined
      ? []
      : [diagnosisAuthSummaryLocalAccessItem(localAccessReadiness, t)]),
    diagnosisAuthSummaryRolloutItem(rolloutReadiness, mode, browserSession, t),
  ];
  const status = diagnosisAuthSummaryStatus(items);
  return {
    detail: diagnosisAuthSummaryDetail(status, items, t),
    items,
    label: diagnosisAuthSummaryLabel(status, t),
    status,
  };
}

function diagnosisAuthSummaryLocalAccessItem(
  readiness: SettingsLocalAccessReadiness,
  t: DiagnosisAuthTranslator,
): DiagnosisAuthReadinessSummaryItem {
  return {
    detail: readiness.detail,
    key: "access",
    label: t("settings.summary.localAccess"),
    status: readiness.status,
    value: diagnosisLocalAccessSummaryValue(readiness),
  };
}

function diagnosisAuthSummaryBackendItem(
  readiness: DiagnosisAuthBackendReadiness,
  t: DiagnosisAuthTranslator,
): DiagnosisAuthReadinessSummaryItem {
  return {
    detail: readiness.detail,
    key: "backend",
    label: t("settings.summary.backend"),
    status:
      readiness.status === "verified"
        ? "ready"
        : readiness.status === "failed" || readiness.status === "blocked"
          ? "blocked"
          : "pending",
    value: diagnosisAuthBackendReadinessStatusLabel(readiness.status, t),
  };
}

function diagnosisAuthSummarySetupItem(
  {
    ldapSetupReadiness,
    mode,
    oidcSetupReadiness,
    weComSetupReadiness,
  }: {
    ldapSetupReadiness: DiagnosisAuthLDAPSetupReadiness;
    mode: DiagnosisAuthMode;
    oidcSetupReadiness?: SettingsOIDCBFFSetupReadiness;
    weComSetupReadiness: DiagnosisAuthWeComSetupReadiness;
  },
  t: DiagnosisAuthTranslator,
): DiagnosisAuthReadinessSummaryItem {
  if (mode === "wecom") {
    const setupGap = diagnosisAuthWeComSetupGapSummary(
      weComSetupReadiness,
      ["backend", "callback", "role_mapping"],
      t,
    );
    const status = diagnosisAuthWeComSetupGroupStatus(weComSetupReadiness, [
      "backend",
      "callback",
      "role_mapping",
    ]);
    return {
      detail:
        setupGap !== null
          ? setupGap.detail
          : t("settings.summary.weComSetupReadyDetail"),
      key: "setup",
      label: t("settings.summary.callbackRBAC"),
      status,
      value:
        setupGap !== null
          ? setupGap.value
          : t("settings.summary.callbackRBACReady"),
    };
  }
  if (mode === "ldap" || mode === "session") {
    if (mode === "session" && oidcSetupReadiness !== undefined) {
      return {
        detail: oidcSetupReadiness.detail,
        key: "setup",
        label: t("provider.iamOIDC"),
        status: oidcSetupReadiness.status,
        value: oidcSetupReadiness.value,
      };
    }
    return {
      detail: ldapSetupReadiness.detail,
      key: "setup",
      label:
        mode === "ldap"
          ? t("settings.summary.ldapSetup")
          : t("settings.summary.sourceSetup"),
      status: diagnosisAuthSetupStatus(ldapSetupReadiness.status),
      value: diagnosisAuthSetupStatusLabel(ldapSetupReadiness.status, t),
    };
  }
  return {
    detail: t("settings.summary.bearerSetupDetail"),
    key: "setup",
    label: t("settings.summary.operatorSSO"),
    status: "review",
    value: t("settings.summary.developmentOnly"),
  };
}

function diagnosisAuthSummarySessionItem(
  mode: DiagnosisAuthMode,
  browserSession: DiagnosisBrowserSessionState,
  t: DiagnosisAuthTranslator,
): DiagnosisAuthReadinessSummaryItem {
  if (browserSession.loading) {
    return {
      detail: t("browserSummary.loading"),
      key: "session",
      label: t("settings.summary.session"),
      status: "pending",
      value: t("settings.summary.checking"),
    };
  }
  if (browserSession.status === null) {
    return {
      detail:
        browserSession.message ||
        t("settings.summary.sessionUnavailableDetail"),
      key: "session",
      label: t("settings.summary.session"),
      status: "blocked",
      value: t("roleStatus.unavailableLabel"),
    };
  }
  if (!browserSession.status.authenticated) {
    return {
      detail:
        mode === "ldap"
          ? t("settings.summary.noLDAPSession")
          : t("settings.summary.noIAMSession"),
      key: "session",
      label: t("settings.summary.session"),
      status: "pending",
      value: t("settings.summary.notSignedIn"),
    };
  }
  const source = diagnosisAuthBrowserSessionSourceValueLabel(
    browserSession.status.mode,
    t,
  );
  if (mode === "wecom") {
    return {
      detail: t("settings.summary.weComSessionMigrated"),
      key: "session",
      label: t("settings.summary.session"),
      status: "blocked",
      value: t("settings.summary.sourceSession", { source }),
    };
  }
  return {
    detail: diagnosisBrowserSessionStatusDetail(browserSession, t),
    key: "session",
    label: t("settings.summary.session"),
    status: "ready",
    value: t("settings.summary.sourceReady", { source }),
  };
}

function diagnosisAuthSummaryRolloutItem(
  readiness: DiagnosisAuthRolloutReadiness,
  mode: DiagnosisAuthMode,
  browserSession: DiagnosisBrowserSessionState,
  t: DiagnosisAuthTranslator,
): DiagnosisAuthReadinessSummaryItem {
  if (mode === "wecom") {
    const proofGap = diagnosisAuthWeComProofGapSummary(
      readiness,
      browserSession,
      t,
    );
    if (proofGap !== null) {
      return {
        detail: proofGap.detail,
        key: "rollout",
        label: t("settings.summary.checkAuthProof"),
        status: proofGap.status,
        value: proofGap.value,
      };
    }
  }
  return {
    detail: readiness.detail,
    key: "rollout",
    label:
      mode === "wecom"
        ? t("settings.summary.checkAuthProof")
        : t("settings.summary.rollout"),
    status: readiness.status,
    value:
      readiness.subject.trim() === ""
        ? diagnosisAuthRolloutStatusLabel(readiness.status, t)
        : readiness.subject,
  };
}

function diagnosisAuthWeComProofGapSummary(
  _readiness: DiagnosisAuthRolloutReadiness,
  browserSession: DiagnosisBrowserSessionState,
  t: DiagnosisAuthTranslator,
): { detail: string; status: ProofReadinessStatus; value: string } | null {
  if (browserSession.loading) {
    return {
      detail: t("settings.summary.proofWaitingSession"),
      status: "pending",
      value: t("settings.summary.checkingSession"),
    };
  }
  return {
    detail: t("settings.summary.proofUseIAM"),
    status: "blocked",
    value: t("settings.summary.useIAMOIDC"),
  };
}

function diagnosisAuthSummaryStatus(
  items: readonly { status: ProofReadinessStatus }[],
): ProofReadinessStatus {
  if (items.some((item) => item.status === "blocked")) {
    return "blocked";
  }
  if (items.some((item) => item.status === "review")) {
    return "review";
  }
  if (items.some((item) => item.status === "pending")) {
    return "pending";
  }
  return "ready";
}

function diagnosisAuthSummaryDetail(
  status: ProofReadinessStatus,
  items: DiagnosisAuthReadinessSummaryItem[],
  t: DiagnosisAuthTranslator,
): string {
  if (status === "ready") {
    return t("settings.summary.readyDetail");
  }
  const blockingItem =
    items.find((item) => item.status === status) ??
    items.find((item) => item.status !== "ready");
  return blockingItem?.detail ?? t("settings.summary.reviewDetail");
}

function diagnosisAuthSummaryLabel(
  status: ProofReadinessStatus,
  t: DiagnosisAuthTranslator,
): string {
  switch (status) {
    case "ready":
      return t("settings.summary.readyLabel");
    case "review":
      return t("settings.summary.reviewLabel");
    case "pending":
      return t("settings.summary.pendingLabel");
    case "blocked":
      return t("settings.summary.blockedLabel");
  }
}

function diagnosisAuthReadinessSummaryAlertType(status: ProofReadinessStatus) {
  switch (status) {
    case "ready":
      return "success";
    case "review":
    case "pending":
      return "warning";
    case "blocked":
      return "error";
  }
}

function diagnosisAuthSetupStatus(
  status: DiagnosisAuthLDAPSetupReadiness["status"],
): ProofReadinessStatus {
  switch (status) {
    case "ready":
      return "ready";
    case "review":
      return "review";
    case "loading":
    case "unavailable":
      return "pending";
    case "blocked":
      return "blocked";
  }
}

function diagnosisAuthWeComSetupGroupStatus(
  readiness: DiagnosisAuthWeComSetupReadiness,
  keys: readonly DiagnosisAuthWeComSetupReadinessItem["key"][],
): ProofReadinessStatus {
  const items = readiness.items.filter((item) => keys.includes(item.key));
  if (items.length === 0) {
    return diagnosisAuthSetupStatus(readiness.status);
  }
  if (items.some((item) => item.status === "blocked")) {
    return "blocked";
  }
  if (items.some((item) => item.status === "review")) {
    return "review";
  }
  if (
    items.some(
      (item) => item.status === "loading" || item.status === "unavailable",
    )
  ) {
    return "pending";
  }
  return "ready";
}

function diagnosisAuthWeComSetupGapSummary(
  readiness: DiagnosisAuthWeComSetupReadiness,
  keys: readonly DiagnosisAuthWeComSetupReadinessItem["key"][],
  t: DiagnosisAuthTranslator,
): { detail: string; value: string } | null {
  const items = readiness.items.filter((candidate) =>
    keys.includes(candidate.key),
  );
  const item =
    items.find((candidate) => candidate.status === "blocked") ??
    items.find((candidate) => candidate.status === "review") ??
    items.find((candidate) => candidate.status === "loading") ??
    items.find((candidate) => candidate.status === "unavailable");
  if (item === undefined) {
    return null;
  }
  return {
    detail: item.detail || readiness.detail,
    value: t("settings.summary.itemStatus", {
      item: item.label,
      status: item.status,
    }),
  };
}

function diagnosisAuthSetupStatusLabel(
  status: DiagnosisAuthLDAPSetupReadiness["status"],
  t: DiagnosisAuthTranslator,
): string {
  switch (status) {
    case "ready":
      return t("ui.ready");
    case "review":
      return t("settings.summary.reviewStatus");
    case "loading":
      return t("roleStatus.loadingLabel");
    case "unavailable":
      return t("roleStatus.unavailableLabel");
    case "blocked":
      return t("ui.blocked");
  }
}

export function diagnosisBrowserSessionStatusDetail(
  state: DiagnosisBrowserSessionState,
  t: DiagnosisAuthTranslator,
): string {
  if (state.loading) {
    return t("settings.browserChecking");
  }
  if (state.status === null) {
    return t("settings.browserCheckFailed", { error: state.message });
  }
  if (!state.status.authenticated) {
    return t("settings.browserSignedOut");
  }
  const summary = diagnosisAuthBrowserSessionAuthenticatedSummary(
    {
      mode: state.status.mode,
      roles: state.status.roles,
      subject: state.status.subject,
    },
    t,
  );
  return t("settings.browserReady", { detail: summary.detail });
}

export function diagnosisBrowserSessionNeedsIAMSignIn(
  state: DiagnosisBrowserSessionState,
): boolean {
  return !state.loading && state.status?.authenticated === false;
}

function diagnosisBrowserSessionStatusAlertType(
  state: DiagnosisBrowserSessionState,
) {
  if (state.loading) {
    return "info";
  }
  if (state.status === null || !state.status.authenticated) {
    return "warning";
  }
  return "success";
}

function WorkflowTopologyPanel({ topology }: { topology: WorkflowTopology }) {
  const locale = useLocale();
  const t = useTranslations("SettingsOverview");
  const hasPolicy = topology.policy !== null;
  const steps = workflowTopologySteps(topology, t, locale);
  const firstUnready = steps.findIndex((step) => step.status !== "finish");
  const current = firstUnready === -1 ? steps.length : firstUnready;
  const actions = workflowTopologyActions(topology, t, locale);

  return (
    <section
      aria-label={t("topology.ariaLabel")}
      className="panel settings-overview-topology"
    >
      <div className="panel-header settings-overview-topology-header">
        <h2>{t("topology.title")}</h2>
        <Tag color={topologyStatusColor(topology.status)}>
          {topologyStatusLabel(topology.status, t)}
        </Tag>
      </div>
      <div className="panel-body settings-overview-topology-body">
        {hasPolicy ? (
          <>
            <div className="settings-overview-topology-flow">
              <Steps
                aria-label={t("topology.sequence")}
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
              items={workflowTopologyDescriptions(topology, t, locale)}
              size="small"
            />
            <TopologyActionList actions={actions} />
          </>
        ) : (
          <Alert
            message={t("topology.noPolicyTitle")}
            description={t("topology.noPolicyDetail")}
            showIcon
            type="info"
          />
        )}
      </div>
    </section>
  );
}

function TopologyActionList({ actions }: { actions: TopologyAction[] }) {
  const t = useTranslations("SettingsOverview");
  if (actions.length === 0) {
    return (
      <Alert
        message={t("topology.readyTitle")}
        description={t("topology.readyDetail")}
        showIcon
        type="success"
      />
    );
  }

  return (
    <section
      aria-label={t("topology.actionsAriaLabel")}
      className="settings-overview-action-list"
    >
      <div className="settings-overview-action-header">
        <Typography.Text strong>{t("topology.actionsTitle")}</Typography.Text>
        <Tag>{t("topology.openActions", { count: actions.length })}</Tag>
      </div>
      <div className="settings-overview-action-grid">
        {actions.map((action) => (
          <div className="settings-overview-action-item" key={action.key}>
            <div>
              <Space size={8} wrap>
                <Tag color={actionPriorityColor(action.priority)}>
                  {t(`priorityLabel.${action.priority}`)}
                </Tag>
                <Typography.Text strong>{action.title}</Typography.Text>
              </Space>
              <Typography.Paragraph className="settings-overview-action-copy">
                {action.detail}
              </Typography.Paragraph>
            </div>
            <Button
              href={action.href}
              icon={<RadarChartOutlined />}
              type={action.priority === "high" ? "primary" : "default"}
            >
              {t("actions.open")}
            </Button>
          </div>
        ))}
      </div>
    </section>
  );
}

function AlertIngestionReadinessPanel({
  autoDiagnosisProofHistory,
  topology,
}: {
  autoDiagnosisProofHistory: AutoDiagnosisProofHistory | null;
  topology: WorkflowTopology;
}) {
  const locale = useLocale();
  const t = useTranslations("SettingsOverview");
  const stages = alertIngestionStages(topology, t);
  const firstUnready = stages.findIndex((stage) => stage.status !== "finish");
  const current = firstUnready === -1 ? stages.length : firstUnready;
  const activeAlertTools = topology.activeAlertTools.length;
  const metricTools = topology.metricTools.length;
  const status = alertIngestionStatus(topology, activeAlertTools, metricTools);
  const intakeStatus =
    topology.alertSource?.enabled &&
    !autoRoomWebhookBlocked(topology.policy, topology.alertSource)
      ? "ready"
      : "blocked";
  const handoffStatus = autoRoomWebhookReady(
    topology.policy,
    topology.alertSource,
  )
    ? "ready"
    : "review";
  const webhookProof = alertIngestionWebhookProofReadiness(
    topology,
    t,
    locale,
    autoDiagnosisProofHistory,
  );

  return (
    <section
      aria-label={t("ingestion.ariaLabel")}
      className="panel settings-overview-ingestion"
    >
      <div className="panel-header settings-overview-ingestion-header">
        <h2>{t("ingestion.title")}</h2>
        <Tag color={topologyStatusColor(status)}>
          {topologyStatusLabel(status, t)}
        </Tag>
      </div>
      <div className="panel-body settings-overview-ingestion-body">
        <div className="settings-overview-ingestion-path">
          <Steps
            aria-label={t("ingestion.sequence")}
            className="settings-overview-ingestion-steps"
            current={current}
            items={stages}
            responsive={false}
            size="small"
          />
        </div>
        <Row aria-label={t("ingestion.countersAriaLabel")} gutter={[12, 12]}>
          <ReadinessCounter
            href="/settings/alert-sources"
            label={t("ingestion.counters.intake")}
            status={intakeStatus}
            value={
              topology.alertSource === null
                ? t("status.missing")
                : sourceKindLabel(topology.alertSource.kind, t)
            }
          />
          <ReadinessCounter
            href={
              activeAlertTools > 0
                ? "/settings/diagnosis-tool-templates"
                : diagnosisToolTemplateLaunchHref({
                    intent: "active-alert-tool",
                    sourceID: topology.alertSource?.id ?? null,
                  })
            }
            label={t("ingestion.counters.alertTools")}
            status={activeAlertTools > 0 ? "ready" : "review"}
            value={activeAlertTools}
          />
          <ReadinessCounter
            href={metricEvidenceConfigurationHref(topology)}
            label={t("ingestion.counters.metricEvidence")}
            status={metricTools > 0 ? "ready" : "review"}
            value={metricTools}
          />
          <ReadinessCounter
            href={
              handoffStatus === "ready"
                ? "/settings/report-workflow-policies"
                : reportWorkflowPolicyLaunchHref({
                    intent: "auto-room-follow-up",
                    sourceID: topology.alertSource?.id ?? null,
                  })
            }
            label={t("ingestion.counters.aiHandoff")}
            status={handoffStatus}
            value={
              topology.policy === null
                ? t("status.missing")
                : diagnosisFollowUpLabel(topology.policy.diagnosis_follow_up, t)
            }
          />
          <ReadinessCounter
            href={webhookProof.href}
            label={t("ingestion.counters.webhookProof")}
            status={webhookProof.status}
            value={webhookProof.value}
          />
        </Row>
      </div>
    </section>
  );
}

function LiveProofReadinessPanel({
  readiness,
}: {
  readiness: LiveProofReadiness;
}) {
  const t = useTranslations("SettingsOverview");
  const firstUnready = readiness.items.findIndex(
    (item) => item.status !== "ready",
  );
  const current = firstUnready === -1 ? readiness.items.length : firstUnready;

  return (
    <section
      aria-label={t("liveProof.ariaLabel")}
      className="panel settings-overview-live-proof"
    >
      <div className="panel-header settings-overview-live-proof-header">
        <h2>{t("liveProof.title")}</h2>
        <Tag color={proofTargetStatusColor(readiness.status)}>
          {proofTargetStatusLabel(readiness.status, t)}
        </Tag>
      </div>
      <div className="panel-body settings-overview-live-proof-body">
        <Typography.Paragraph className="settings-overview-proof-copy">
          {readiness.detail}
        </Typography.Paragraph>
        <div className="settings-overview-live-proof-steps-wrap">
          <Steps
            aria-label={t("liveProof.sequence")}
            className="settings-overview-live-proof-steps"
            current={current}
            items={readiness.items.map((item) => ({
              description: item.detail,
              status: proofReadinessStepStatus(item.status),
              title: item.title,
            }))}
            responsive={false}
            size="small"
          />
        </div>
        <Descriptions
          bordered
          column={{ lg: 4, md: 2, sm: 1, xs: 1 }}
          items={readiness.items.map((item) => ({
            key: item.key,
            label: item.title,
            children: (
              <Space direction="vertical" size={4}>
                <Tag color={proofTargetStatusColor(item.status)}>
                  {proofTargetStatusLabel(item.status, t)}
                </Tag>
                <Typography.Text strong>{item.value}</Typography.Text>
                <Typography.Text type="secondary">
                  {item.detail}
                </Typography.Text>
              </Space>
            ),
          }))}
          size="small"
        />
      </div>
    </section>
  );
}

function AutoDiagnosisProofHistoryPanel({
  histories,
}: {
  histories: AutoDiagnosisProofHistory[];
}) {
  const locale = useLocale();
  const t = useTranslations("SettingsOverview");
  const columns: TableColumnsType<AutoDiagnosisProofHistory> = [
    {
      key: "alert",
      render: (_, history) => (
        <Space direction="vertical" size={4}>
          <Typography.Text strong>{history.alertName}</Typography.Text>
          <Space size={[6, 6]} wrap>
            <Tag>{t("history.alertTag", { id: history.alertID })}</Tag>
            <Tag>{t("history.snapshotTag", { id: history.snapshotID })}</Tag>
          </Space>
        </Space>
      ),
      title: t("history.columns.alert"),
      width: 260,
    },
    {
      key: "coverage",
      render: (_, history) => {
        const coverage = localizedNotificationCoverage(history.coverage, t);
        return (
        <Space direction="vertical" size={4}>
          <Tag color={proofTargetStatusColor(history.coverage.status)}>
              {coverage.label}
          </Tag>
          <Typography.Text type="secondary">
              {t("history.phaseCount", {
                ready: history.coverage.readyCount,
                required: history.coverage.requiredCount,
              })}
          </Typography.Text>
        </Space>
        );
      },
      title: t("history.columns.coverage"),
      width: 170,
    },
    {
      key: "phases",
      render: (_, history) => (
        <Space size={[4, 4]} wrap>
          {localizedNotificationCoverage(history.coverage, t).phases.map(
            (phase) => (
            <Tag
              color={diagnosisNotificationDeliveryCoveragePhaseColor(
                phase.status,
              )}
              key={phase.key}
            >
                {phase.label}: {notificationPhaseStatusLabel(phase.status, t)}
            </Tag>
            ),
          )}
        </Space>
      ),
      title: t("history.columns.phases"),
    },
    {
      dataIndex: "occurredAt",
      key: "occurredAt",
      render: (value: string) => formatDateTime(value, locale),
      title: t("history.columns.latest"),
      width: 190,
    },
    {
      key: "action",
      render: (_, history) => (
        <Button href={history.href} size="small" type="link">
          {t("actions.reviewTimeline")}
        </Button>
      ),
      title: t("history.columns.action"),
      width: 140,
    },
  ];

  return (
    <section
      aria-label={t("history.ariaLabel")}
      className="panel settings-overview-auto-proof-history"
    >
      <div className="panel-header settings-overview-live-proof-header">
        <h2>{t("history.title")}</h2>
        <Tag color={histories.length > 0 ? "blue" : "default"}>
          {t("history.retained", { count: histories.length })}
        </Tag>
      </div>
      <div className="panel-body settings-overview-live-proof-body">
        <Typography.Paragraph className="settings-overview-proof-copy">
          {t("history.detail")}
        </Typography.Paragraph>
        <Table<AutoDiagnosisProofHistory>
          columns={columns}
          dataSource={histories}
          locale={{ emptyText: t("history.empty") }}
          pagination={false}
          rowKey={(history) =>
            `${history.alertID}:${history.snapshotID}:${history.roomSessionID}`
          }
          scroll={{ x: 940 }}
          size="small"
        />
      </div>
    </section>
  );
}

function ReadinessCounter({
  href,
  label,
  status,
  value,
}: {
  href: string;
  label: string;
  status: WorkflowTopology["status"];
  value: number | string;
}) {
  const t = useTranslations("SettingsOverview");
  return (
    <Col lg={6} sm={12} xs={24}>
      <a className="settings-overview-ingestion-counter" href={href}>
        <span className="settings-overview-ingestion-value">{value}</span>
        <span className="settings-overview-ingestion-footer">
          <Typography.Text className="muted">{label}</Typography.Text>
          <Tag color={topologyStatusColor(status)}>
            {topologyStatusLabel(status, t)}
          </Tag>
        </span>
      </a>
    </Col>
  );
}

export function buildWorkflowTopology({
  alertSources,
  diagnosisToolTemplates,
  groupingPolicies,
  notificationChannels,
  workflowPolicies,
  workflowSchedules,
}: {
  alertSources: AlertSourceProfile[];
  diagnosisToolTemplates: DiagnosisToolTemplate[];
  groupingPolicies: GroupingPolicy[];
  notificationChannels: NotificationChannelProfile[];
  workflowPolicies: ReportWorkflowPolicy[];
  workflowSchedules: ReportWorkflowSchedule[];
}): WorkflowTopology {
  const policy =
    workflowPolicies.find(
      (item) => item.enabled && item.diagnosis_follow_up === "auto_room",
    ) ??
    workflowPolicies.find(
      (item) => item.enabled && item.diagnosis_follow_up === "suggest_room",
    ) ??
    workflowPolicies.find((item) => item.enabled) ??
    workflowPolicies[0] ??
    null;
  const alertSource =
    policy === null
      ? null
      : (alertSources.find(
          (item) => item.id === policy.alert_source_profile_id,
        ) ?? null);
  const groupingPolicy =
    policy === null
      ? null
      : (groupingPolicies.find(
          (item) => item.id === policy.grouping_policy_id,
        ) ?? null);
  const notificationChannel =
    policy?.report_notification_channel_profile_id == null
      ? null
      : (notificationChannels.find(
          (item) => item.id === policy.report_notification_channel_profile_id,
        ) ?? null);
  const activeSchedule =
    policy === null
      ? null
      : (workflowSchedules.find(
          (item) =>
            item.report_workflow_policy_id === policy.id && item.enabled,
        ) ??
        workflowSchedules.find(
          (item) => item.report_workflow_policy_id === policy.id,
        ) ??
        null);
  const usableDiagnosisTools = diagnosisToolTemplates.filter((template) =>
    diagnosisToolRunsOnEnabledSource(template, alertSources),
  );
  const activeAlertTools =
    alertSource === null
      ? []
      : usableDiagnosisTools.filter(
          (template) =>
            template.tool === "active_alerts" &&
            template.alert_source_profile_id === alertSource.id,
        );
  const metricTools = usableDiagnosisTools.filter((template) =>
    isMetricTool(template.tool),
  );
  const diagnosisTools = uniqueTemplates([...activeAlertTools, ...metricTools]);
  const metricSources = alertSources.filter(isMetricEvidenceSource);
  const status = topologyStatus(
    policy,
    alertSource,
    groupingPolicy,
    notificationChannel,
    activeAlertTools,
    metricTools,
  );
  return {
    activeAlertTools,
    activeSchedule,
    alertSource,
    diagnosisTools,
    groupingPolicy,
    metricSources,
    metricTools,
    notificationChannel,
    policy,
    status,
  };
}

function alertIngestionStages(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
): WorkflowTopologyStep[] {
  const activeAlertTools = topology.activeAlertTools.length;
  const metricTools = topology.metricTools.length;
  const autoRoomReady = autoRoomWebhookReady(
    topology.policy,
    topology.alertSource,
  );
  const autoRoomBlocked = autoRoomWebhookBlocked(
    topology.policy,
    topology.alertSource,
  );
  return [
    {
      title: t("ingestion.steps.intake.title"),
      description:
        topology.alertSource === null
          ? t("status.missingSource")
          : autoRoomBlocked
            ? t("ingestion.steps.intake.requiresAlertmanager")
            : t("ingestion.steps.intake.source", {
                kind: sourceKindLabel(topology.alertSource.kind, t),
              }),
      status:
        topology.alertSource?.enabled && !autoRoomBlocked ? "finish" : "error",
    },
    {
      title: t("ingestion.steps.alerts.title"),
      description: t("ingestion.steps.alerts.detail", {
        count: activeAlertTools,
      }),
      status: activeAlertTools > 0 ? "finish" : "wait",
    },
    {
      title: t("ingestion.steps.metrics.title"),
      description: t("ingestion.steps.metrics.detail", {
        count: metricTools,
      }),
      status: metricTools > 0 ? "finish" : "wait",
    },
    {
      title: t("ingestion.steps.aiRoom.title"),
      description:
        topology.policy === null
          ? t("status.noPolicy")
          : diagnosisFollowUpLabel(topology.policy.diagnosis_follow_up, t),
      status: autoRoomReady ? "finish" : "wait",
    },
    {
      title: t("ingestion.steps.replay.title"),
      description: autoRoomReady
        ? t("ingestion.steps.replay.ready")
        : topology.policy?.enabled
          ? t("ingestion.steps.replay.policyEnabled")
          : t("ingestion.steps.replay.policyDraft"),
      status:
        autoRoomReady &&
        topology.groupingPolicy?.enabled &&
        activeAlertTools > 0 &&
        metricTools > 0
          ? "process"
          : "wait",
    },
  ];
}

export function alertIngestionStatus(
  topology: WorkflowTopology,
  activeAlertTools: number,
  metricTools: number,
): WorkflowTopology["status"] {
  if (
    topology.policy === null ||
    topology.alertSource === null ||
    !topology.alertSource.enabled ||
    !topology.policy.enabled
  ) {
    return "blocked";
  }
  if (autoRoomWebhookBlocked(topology.policy, topology.alertSource)) {
    return "blocked";
  }
  if (
    !autoRoomWebhookReady(topology.policy, topology.alertSource) ||
    activeAlertTools === 0 ||
    metricTools === 0
  ) {
    return "review";
  }
  return "ready";
}

function diagnosisToolRunsOnEnabledSource(
  template: DiagnosisToolTemplate,
  sources: AlertSourceProfile[],
): boolean {
  const source = sources.find(
    (candidate) => candidate.id === template.alert_source_profile_id,
  );
  if (source === undefined || !source.enabled || !template.enabled) {
    return false;
  }
  if (template.tool === "active_alerts") {
    return source.kind === "alertmanager" || source.kind === "prometheus";
  }
  return isMetricEvidenceSource(source);
}

function isMetricEvidenceSource(source: AlertSourceProfile): boolean {
  return (
    source.kind === "prometheus" && !alertSourceProfileIsThanosRule(source)
  );
}

function uniqueTemplates(
  templates: DiagnosisToolTemplate[],
): DiagnosisToolTemplate[] {
  const seen = new Set<number>();
  return templates.filter((template) => {
    if (seen.has(template.id)) {
      return false;
    }
    seen.add(template.id);
    return true;
  });
}

function isMetricTool(tool: DiagnosisToolTemplate["tool"]): boolean {
  return tool === "metric_query" || tool === "metric_range_query";
}

export function workflowTopologyActions(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
): TopologyAction[] {
  const actions: TopologyAction[] = [];

  if (topology.policy === null) {
    return [
      {
        key: "create-policy",
        title: t("topology.actions.createPolicy.title"),
        detail: t("topology.actions.createPolicy.detail"),
        href: reportWorkflowPolicyLaunchHref({
          intent: "create-auto-room-policy",
        }),
        priority: "high",
      },
    ];
  }

  if (!topology.policy.enabled) {
    actions.push({
      key: "enable-policy",
      title: t("topology.actions.enablePolicy.title"),
      detail: t("topology.actions.enablePolicy.detail"),
      href: "/settings/report-workflow-policies",
      priority: "high",
    });
  }
  if (topology.alertSource === null || !topology.alertSource.enabled) {
    actions.push({
      key: "alert-source",
      title: t("topology.actions.alertSource.title"),
      detail: t("topology.actions.alertSource.detail"),
      href: "/settings/alert-sources",
      priority: "high",
    });
  }
  if (topology.groupingPolicy === null || !topology.groupingPolicy.enabled) {
    actions.push({
      key: "grouping",
      title: t("topology.actions.grouping.title"),
      detail: t("topology.actions.grouping.detail"),
      href:
        topology.groupingPolicy === null
          ? groupingPolicyLaunchHref({ intent: "default-alert-grouping" })
          : "/settings/grouping-policies",
      priority: "high",
    });
  }
  if (!isAIRoomFollowUp(topology.policy.diagnosis_follow_up)) {
    actions.push({
      key: "room-follow-up",
      title: t("topology.actions.roomFollowUp.title"),
      detail: t("topology.actions.roomFollowUp.detail"),
      href: reportWorkflowPolicyLaunchHref({
        intent: "enable-ai-room-follow-up",
        sourceID: topology.alertSource?.id ?? null,
      }),
      priority: "medium",
    });
  }
  if (topology.policy.diagnosis_follow_up === "suggest_room") {
    actions.push({
      key: "auto-room-follow-up",
      title: t("topology.actions.autoRoomFollowUp.title"),
      detail: t("topology.actions.autoRoomFollowUp.detail"),
      href: reportWorkflowPolicyLaunchHref({
        intent: "auto-room-follow-up",
        sourceID: topology.alertSource?.id ?? null,
      }),
      priority: "medium",
    });
  }
  if (autoRoomWebhookBlocked(topology.policy, topology.alertSource)) {
    actions.push({
      key: "auto-room-alertmanager-source",
      title: t("topology.actions.alertmanagerSource.title"),
      detail: t("topology.actions.alertmanagerSource.detail"),
      href: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      priority: "high",
    });
  }
  if (topology.activeAlertTools.length === 0) {
    actions.push({
      key: "active-alert-tool",
      title: t("topology.actions.activeAlertTool.title"),
      detail: t("topology.actions.activeAlertTool.detail"),
      href: diagnosisToolTemplateLaunchHref({
        intent: "active-alert-tool",
        sourceID: topology.alertSource?.id ?? null,
      }),
      priority: "medium",
    });
  }
  if (topology.metricTools.length === 0) {
    const enabledMetricSources = topology.metricSources.filter(
      (source) => source.enabled,
    );
    const metricSourceID = metricEvidenceToolLaunchSourceID(topology);
    if (enabledMetricSources.length > 0) {
      actions.push({
        key: "metric-evidence-tool",
        title: t("topology.actions.metricEvidence.title"),
        detail: t("topology.actions.metricEvidence.detail"),
        href: diagnosisToolTemplateLaunchHref({
          intent: "metric-evidence-tool",
          sourceID: metricSourceID,
        }),
        priority: "medium",
      });
    } else if (topology.metricSources.length > 0) {
      actions.push({
        key: "enable-prometheus-metric-source",
        title: t("topology.actions.enableMetricSource.title"),
        detail: t("topology.actions.enableMetricSource.detail"),
        href: "/settings/alert-sources",
        priority: "medium",
      });
    } else {
      actions.push({
        key: "thanos-metric-source",
        title: t("topology.actions.createMetricSource.title"),
        detail: t("topology.actions.createMetricSource.detail"),
        href: alertSourceLaunchHref({ intent: "thanos-source" }),
        priority: "medium",
      });
    }
  }
  if (
    topology.notificationChannel !== null &&
    !topology.notificationChannel.enabled
  ) {
    actions.push({
      key: "notification-channel",
      title: t("topology.actions.enableChannel.title"),
      detail: t("topology.actions.enableChannel.detail"),
      href: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      priority: "medium",
    });
  }
  if (
    topology.notificationChannel === null &&
    topology.policy.diagnosis_follow_up === "auto_room"
  ) {
    actions.push({
      key: "notification-channel-bind",
      title: t("topology.actions.bindChannel.title"),
      detail: t("topology.actions.bindChannel.detail"),
      href: notificationChannelAutoRoomLaunchHref(topology),
      priority: "high",
    });
  }
  const missingDeliveryScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  if (
    topology.notificationChannel !== null &&
    missingDeliveryScopes.length > 0
  ) {
    actions.push({
      key: "notification-channel-scope",
      title: t("topology.actions.channelScopes.title"),
      detail: t("topology.actions.channelScopes.detail", {
        scopes: localizedDeliveryScopeList(missingDeliveryScopes, t, locale),
      }),
      href: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      priority: "medium",
    });
  }
  if (
    autoRoomChannelKindBlocked(topology.policy, topology.notificationChannel)
  ) {
    actions.push({
      key: "notification-channel-kind",
      title: t("topology.actions.channelKind.title"),
      detail: t("topology.actions.channelKind.detail"),
      href: notificationChannelAutoRoomLaunchHref(topology),
      priority: "high",
    });
  }
  if (topology.activeSchedule === null) {
    actions.push({
      key: "create-schedule",
      title: t("topology.actions.createSchedule.title"),
      detail: t("topology.actions.createSchedule.detail"),
      href: reportWorkflowScheduleLaunchHref({
        intent: "create-schedule",
        policyID: topology.policy.id,
      }),
      priority: "low",
    });
  } else if (!topology.activeSchedule.enabled) {
    actions.push({
      key: "enable-schedule",
      title: t("topology.actions.enableSchedule.title"),
      detail: t("topology.actions.enableSchedule.detail"),
      href: "/settings/report-workflow-schedules",
      priority: "low",
    });
  }

  if (
    topology.policy.enabled &&
    topology.alertSource?.enabled &&
    topology.groupingPolicy?.enabled
  ) {
    actions.push({
      key: "impact-preview",
      title: t("topology.actions.impactPreview.title"),
      detail: t("topology.actions.impactPreview.detail"),
      href: "/settings/report-workflow-policies",
      priority: "low",
    });
  }

  return actions.slice(0, 5);
}

function metricEvidenceToolLaunchSourceID(
  topology: WorkflowTopology,
): number | null {
  const enabledMetricSources = topology.metricSources.filter(
    (source) => source.enabled,
  );
  if (enabledMetricSources.length !== 1) {
    return null;
  }
  return enabledMetricSources[0]?.id ?? null;
}

export function metricEvidenceConfigurationHref(
  topology: WorkflowTopology,
): string {
  if (topology.metricTools.length > 0) {
    return "/settings/diagnosis-tool-templates";
  }
  const enabledMetricSources = topology.metricSources.filter(
    (source) => source.enabled,
  );
  if (enabledMetricSources.length > 0) {
    return diagnosisToolTemplateLaunchHref({
      intent: "metric-evidence-tool",
      sourceID: metricEvidenceToolLaunchSourceID(topology),
    });
  }
  if (topology.metricSources.length > 0) {
    return "/settings/alert-sources";
  }
  return alertSourceLaunchHref({ intent: "thanos-source" });
}

function notificationChannelAutoRoomLaunchHref(
  topology: WorkflowTopology,
): string {
  return notificationChannelLaunchHref({
    intent: "report-close-channel",
    workflowReturn: { sourceID: topology.alertSource?.id ?? null },
  });
}

function notificationChannelAutoRoomEditHref(
  topology: WorkflowTopology,
  channelID: number,
): string {
  return notificationChannelEditHref(channelID, {
    workflowReturn: { sourceID: topology.alertSource?.id ?? null },
  });
}

export function workflowProofTargets(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
  autoDiagnosisProofHistory: AutoDiagnosisProofHistory | null = null,
): ProofTarget[] {
  const replayTarget = workflowReplayProofTarget(topology, t, locale);
  const autoDiagnosisTarget = workflowAlertmanagerAutoDiagnosisProofTarget(
    topology,
    t,
    locale,
    autoDiagnosisProofHistory,
  );
  const scheduleTarget = workflowScheduleProofTarget(topology, t, locale);
  return [replayTarget, autoDiagnosisTarget, scheduleTarget];
}

export function alertIngestionWebhookProofReadiness(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
  proofHistory: AutoDiagnosisProofHistory | null = null,
): AlertIngestionWebhookProofReadiness {
  const target = workflowAlertmanagerAutoDiagnosisProofTarget(
    topology,
    t,
    locale,
    proofHistory,
  );
  if (proofHistory === null) {
    return {
      href: target.actionHref,
      status: target.status === "blocked" ? "blocked" : "review",
      value: t("status.missing"),
    };
  }
  return {
    href: target.actionHref,
    status: proofReadinessStatusForTopology(target.status),
    value: localizedNotificationCoverage(proofHistory.coverage, t).label,
  };
}

export function latestAutoDiagnosisProofHistory(
  alerts: AlertEventSummary[],
  t: SettingsOverviewTranslator,
): AutoDiagnosisProofHistory | null {
  return autoDiagnosisProofHistoriesForAlerts(alerts, t, 1)[0] ?? null;
}

export function latestAutoDiagnosisProofHistoryForSource(
  alerts: AlertEventSummary[],
  alertSourceProfileID: number | null,
  t: SettingsOverviewTranslator,
): AutoDiagnosisProofHistory | null {
  if (alertSourceProfileID === null || alertSourceProfileID <= 0) {
    return null;
  }
  return (
    autoDiagnosisProofHistoriesForAlerts(
      alerts,
      t,
      Number.MAX_SAFE_INTEGER,
    ).find(
      (history) => history.alertSourceProfileID === alertSourceProfileID,
    ) ?? null
  );
}

export function autoDiagnosisProofHistoriesForAlerts(
  alerts: AlertEventSummary[],
  t: SettingsOverviewTranslator,
  limit = 5,
): AutoDiagnosisProofHistory[] {
  const histories: AutoDiagnosisProofHistory[] = [];
  for (const alert of alerts) {
    for (const snapshot of alert.linked_evidence_snapshots) {
      if (snapshot.created_by_workflow !== "AlertmanagerWebhookAutoDiagnosis") {
        continue;
      }
      for (const room of snapshot.diagnosis_rooms) {
        const timeline = room.notification_timeline ?? [];
        const occurredAt =
          latestNotificationOccurredAt(timeline) ||
          room.last_activity_at ||
          room.updated_at ||
          snapshot.created_at ||
          alert.created_at;
        histories.push({
          alertID: alert.id,
          alertName: alertProofDisplayName(alert, t),
          alertSourceProfileID: alert.alert_source_profile_id,
          coverage: diagnosisNotificationDeliveryCoverage(timeline),
          href: diagnosisRoomAnchorHref({
            anchorID: diagnosisNotificationTimelineAnchorID,
            evidenceSnapshotID: room.evidence_snapshot_id,
            sessionID: room.session_id,
          }),
          occurredAt,
          roomSessionID: room.session_id,
          snapshotID: snapshot.id,
        });
      }
    }
  }
  histories.sort(
    (left, right) =>
      timestampSortValue(right.occurredAt) -
      timestampSortValue(left.occurredAt),
  );
  return histories.slice(0, Math.max(0, limit));
}

function proofReadinessStatusForTopology(
  status: ProofReadinessStatus,
): AlertIngestionWebhookProofReadiness["status"] {
  return status === "pending" ? "review" : status;
}

function latestNotificationOccurredAt(
  timeline: NonNullable<
    AlertEventSummary["linked_evidence_snapshots"][number]["diagnosis_rooms"][number]["notification_timeline"]
  >,
): string {
  for (let index = timeline.length - 1; index >= 0; index -= 1) {
    const occurredAt = timeline[index]?.occurred_at?.trim();
    if (occurredAt) {
      return occurredAt;
    }
  }
  return "";
}

function alertProofDisplayName(
  alert: AlertEventSummary,
  t: SettingsOverviewTranslator,
): string {
  return (
    alert.labels.alertname?.trim() ||
    alert.annotations.summary?.trim() ||
    t("history.alertFallback", { id: alert.id })
  );
}

function timestampSortValue(value: string): number {
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

export function workflowLiveProofReadiness(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
  diagnosisAuthProof: DiagnosisAuthLiveProof = defaultDiagnosisAuthLiveProof(),
): LiveProofReadiness {
  const replayTarget = workflowReplayProofTarget(topology, t, locale);
  const scheduleTarget = workflowScheduleProofTarget(topology, t, locale);
  const items: LiveProofReadinessItem[] = [
    {
      detail: replayTarget.detail,
      key: "policy-replay",
      status: replayTarget.status,
      title: t("liveProof.items.replay"),
      value: replayTarget.actionLabel,
    },
    workflowAIDiagnosisProofItem(topology, t),
    workflowDiagnosisAuthProofItem(diagnosisAuthProof, t),
    workflowNotificationProofItem(topology, t, locale),
    {
      detail: scheduleTarget.detail,
      key: "scheduled-trigger",
      status: scheduleTarget.status,
      title: t("liveProof.items.schedule"),
      value: scheduleTarget.actionLabel,
    },
  ];
  const status = aggregateProofReadiness(items.map((item) => item.status));
  return {
    detail: liveProofReadinessDetail(status, t),
    items,
    status,
  };
}

export function workflowIntegrationReadiness(
  topology: WorkflowTopology,
  authT: DiagnosisAuthTranslator,
  t: SettingsOverviewTranslator,
  locale: string,
  diagnosisAuthProof: DiagnosisAuthLiveProof = defaultDiagnosisAuthLiveProof(),
  localAccessReadiness?: SettingsLocalAccessReadiness,
  currentRBACAccess?: SettingsCurrentRBACAccessSummary,
): IntegrationReadiness {
  const items: IntegrationReadinessItem[] = [
    workflowOperatorAuthReadinessItem(diagnosisAuthProof, authT, t),
    ...(localAccessReadiness === undefined
      ? []
      : [
          workflowLocalAccessReadinessItem(
            localAccessReadiness,
            t,
            currentRBACAccess,
          ),
        ]),
    workflowEnterpriseWeChatReadinessItem(topology, t, locale),
    workflowAlertmanagerAutoRoomReadinessItem(topology, t),
  ];
  const status = aggregateProofReadiness(items.map((item) => item.status));
  return {
    detail: integrationReadinessDetail(status, t),
    items,
    status,
  };
}

function workflowLocalAccessReadinessItem(
  readiness: SettingsLocalAccessReadiness,
  t: SettingsOverviewTranslator,
  currentRBACAccess?: SettingsCurrentRBACAccessSummary,
): IntegrationReadinessItem {
  if (currentRBACAccess === undefined) {
    return {
      actionHref: "/settings/directory-rbac",
      actionLabel:
        readiness.status === "ready"
          ? t("actions.reviewAccess")
          : t("actions.fixAccess"),
      detail: readiness.detail,
      key: "local-access",
      status: readiness.status,
      title: t("integration.localAccess.title"),
      value: diagnosisLocalAccessSummaryValue(readiness),
    };
  }
  const status = aggregateProofReadiness([
    readiness.status,
    currentRBACAccess.status,
  ]);
  const currentAccessBlocked =
    currentRBACAccess.status === "blocked" ||
    currentRBACAccess.status === "review";
  const detail = currentAccessBlocked
    ? currentRBACAccess.detail
    : readiness.status === "ready"
      ? t("integration.localAccess.readyDetail")
      : readiness.detail;
  return {
    actionHref: currentRBACAccess.needsSignIn
      ? settingsDirectoryRBACLoginHref
      : "/settings/directory-rbac",
    actionLabel: currentRBACAccess.needsSignIn
      ? t("actions.signIn")
      : status === "ready"
        ? t("actions.reviewAccess")
        : t("actions.fixAccess"),
    detail,
    key: "local-access",
    status,
    title: t("integration.localAccess.title"),
    value:
      currentRBACAccess.subject === ""
        ? currentRBACAccess.label
        : currentRBACAccess.subject,
  };
}

function workflowOperatorAuthReadinessItem(
  proof: DiagnosisAuthLiveProof,
  authT: DiagnosisAuthTranslator,
  t: SettingsOverviewTranslator,
): IntegrationReadinessItem {
  const actionHref = "#diagnosis-auth-readiness";
  const readiness = diagnosisAuthRolloutReadiness(proof, authT);
  if (readiness.status === "ready") {
    return {
      actionHref,
      actionLabel: t("actions.reviewAuth"),
      detail: readiness.detail || t("integration.operatorAuth.readyDetail"),
      key: "operator-auth",
      status: "ready",
      title: t("integration.operatorAuth.title"),
      value:
        readiness.subject === ""
          ? t("integration.operatorAuth.verified", {
              mode: authProbeModeValueLabel(readiness.mode, authT),
            })
          : readiness.subject,
    };
  }
  if (readiness.status === "review") {
    return {
      actionHref,
      actionLabel: t("actions.checkSSO"),
      detail: readiness.detail,
      key: "operator-auth",
      status: "review",
      title: t("integration.operatorAuth.title"),
      value: t("integration.operatorAuth.verified", {
        mode: authProbeModeValueLabel(readiness.mode, authT),
      }),
    };
  }
  if (readiness.status === "blocked") {
    return {
      actionHref,
      actionLabel: t("actions.fixAuth"),
      detail: readiness.detail,
      key: "operator-auth",
      status: "blocked",
      title: t("integration.operatorAuth.title"),
      value: t("status.blocked"),
    };
  }
  return {
    actionHref,
    actionLabel: t("actions.checkAuth"),
    detail: readiness.detail,
    key: "operator-auth",
    status: "pending",
    title: t("integration.operatorAuth.title"),
    value: t("status.notChecked"),
  };
}

function workflowEnterpriseWeChatReadinessItem(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
): IntegrationReadinessItem {
  const actionHref = notificationChannelAutoRoomLaunchHref(topology);
  if (topology.policy === null) {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "create-auto-room-policy",
      }),
      actionLabel: t("actions.createPolicy"),
      detail: t("integration.weCom.noPolicyDetail"),
      key: "enterprise-wechat",
      status: "blocked",
      title: t("integration.weCom.title"),
      value: t("status.noPolicy"),
    };
  }
  if (topology.notificationChannel === null) {
    return {
      actionHref,
      actionLabel: t("actions.bindChannel"),
      detail: t("integration.weCom.noChannelDetail"),
      key: "enterprise-wechat",
      status: "blocked",
      title: t("integration.weCom.title"),
      value: t("status.noChannel"),
    };
  }
  if (!topology.notificationChannel.enabled) {
    return {
      actionHref: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      actionLabel: t("actions.enableChannel"),
      detail: t("integration.weCom.disabledDetail"),
      key: "enterprise-wechat",
      status: "blocked",
      title: t("integration.weCom.title"),
      value: topology.notificationChannel.name,
    };
  }
  if (topology.notificationChannel.kind !== "wecom") {
    return {
      actionHref,
      actionLabel: t("actions.switchChannel"),
      detail: t("integration.weCom.wrongKindDetail"),
      key: "enterprise-wechat",
      status: "blocked",
      title: t("integration.weCom.title"),
      value: topology.notificationChannel.name,
    };
  }
  const missingScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  if (missingScopes.length > 0) {
    return {
      actionHref: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      actionLabel: t("actions.reviewScopes"),
      detail: t("integration.weCom.missingScopesDetail", {
        scopes: localizedDeliveryScopeList(missingScopes, t, locale),
      }),
      key: "enterprise-wechat",
      status: "blocked",
      title: t("integration.weCom.title"),
      value: topology.notificationChannel.name,
    };
  }
  if (topology.policy.diagnosis_follow_up !== "auto_room") {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "auto-room-follow-up",
        sourceID: topology.alertSource?.id ?? null,
      }),
      actionLabel: t("actions.useAutoRoom"),
      detail: t("integration.weCom.followUpDetail"),
      key: "enterprise-wechat",
      status: "review",
      title: t("integration.weCom.title"),
      value: topology.notificationChannel.name,
    };
  }
  const rolloutReadiness = notificationChannelEnterpriseWeChatRolloutReadiness([
    topology.notificationChannel,
  ]);
  if (rolloutReadiness.status !== "ready") {
    return {
      actionHref: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      actionLabel:
        rolloutReadiness.status === "blocked"
          ? t("actions.fixChannel")
          : t("actions.testChannel"),
      detail: localizedNotificationProofGapDetail(
        rolloutReadiness,
        t,
        locale,
      ),
      key: "enterprise-wechat",
      status: rolloutReadiness.status === "blocked" ? "blocked" : "review",
      title: t("integration.weCom.title"),
      value: topology.notificationChannel.name,
    };
  }
  return {
    actionHref: notificationChannelAutoRoomEditHref(
      topology,
      topology.notificationChannel.id,
    ),
    actionLabel: t("actions.reviewChannel"),
    detail: t("integration.weCom.readyDetail"),
    key: "enterprise-wechat",
    status: "ready",
    title: t("integration.weCom.title"),
    value: topology.notificationChannel.name,
  };
}

function workflowAlertmanagerAutoRoomReadinessItem(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
): IntegrationReadinessItem {
  if (topology.policy === null) {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "create-auto-room-policy",
      }),
      actionLabel: t("actions.createPolicy"),
      detail: t("integration.autoRoom.noPolicyDetail"),
      key: "alertmanager-auto-room",
      status: "blocked",
      title: t("integration.autoRoom.title"),
      value: t("status.noPolicy"),
    };
  }
  if (!topology.policy.enabled) {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: t("actions.enablePolicy"),
      detail: t("integration.autoRoom.disabledPolicyDetail"),
      key: "alertmanager-auto-room",
      status: "blocked",
      title: t("integration.autoRoom.title"),
      value: topology.policy.name,
    };
  }
  if (topology.policy.diagnosis_follow_up !== "auto_room") {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "auto-room-follow-up",
        sourceID: topology.alertSource?.id ?? null,
      }),
      actionLabel: t("actions.useAutoRoom"),
      detail: t("integration.autoRoom.followUpDetail"),
      key: "alertmanager-auto-room",
      status: "review",
      title: t("integration.autoRoom.title"),
      value: diagnosisFollowUpLabel(topology.policy.diagnosis_follow_up, t),
    };
  }
  if (topology.alertSource === null || !topology.alertSource.enabled) {
    return {
      actionHref: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      actionLabel: t("actions.reviewSource"),
      detail: t("integration.autoRoom.noSourceDetail"),
      key: "alertmanager-auto-room",
      status: "blocked",
      title: t("integration.autoRoom.title"),
      value: t("status.noSource"),
    };
  }
  if (topology.alertSource.kind !== "alertmanager") {
    return {
      actionHref: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      actionLabel: t("actions.switchSource"),
      detail: t("integration.autoRoom.wrongSourceDetail"),
      key: "alertmanager-auto-room",
      status: "blocked",
      title: t("integration.autoRoom.title"),
      value: topology.alertSource.name,
    };
  }
  if (
    topology.activeAlertTools.length === 0 ||
    topology.metricTools.length === 0
  ) {
    return {
      actionHref: diagnosisToolTemplateLaunchHref({
        intent:
          topology.activeAlertTools.length === 0
            ? "active-alert-tool"
            : "metric-evidence-tool",
        sourceID:
          topology.activeAlertTools.length === 0
            ? topology.alertSource.id
            : metricEvidenceToolLaunchSourceID(topology),
      }),
      actionLabel: t("actions.reviewTools"),
      detail: t("integration.autoRoom.toolsDetail"),
      key: "alertmanager-auto-room",
      status: "review",
      title: t("integration.autoRoom.title"),
      value: t("integration.autoRoom.toolCounts", {
        alerts: topology.activeAlertTools.length,
        metrics: topology.metricTools.length,
      }),
    };
  }
  return {
    actionHref: "/settings/report-workflow-policies",
    actionLabel: t("actions.reviewWorkflow"),
    detail: t("integration.autoRoom.readyDetail"),
    key: "alertmanager-auto-room",
    status: "ready",
    title: t("integration.autoRoom.title"),
    value: topology.alertSource.name,
  };
}

function integrationReadinessDetail(
  status: ProofReadinessStatus,
  t: SettingsOverviewTranslator,
): string {
  switch (status) {
    case "ready":
      return t("integration.detail.ready");
    case "review":
      return t("integration.detail.review");
    case "pending":
      return t("integration.detail.pending");
    case "blocked":
      return t("integration.detail.blocked");
  }
}

function workflowDiagnosisAuthProofItem(
  proof: DiagnosisAuthLiveProof,
  t: SettingsOverviewTranslator,
): LiveProofReadinessItem {
  const modeLabel = settingsAuthModeLabel(proof.mode, t);
  const modeValue = modeLabel;
  switch (proof.status) {
    case "verified":
      return {
        detail:
          proof.detail ||
          t("liveProof.auth.acceptedDetail", { mode: modeLabel }),
        key: "diagnosis-auth",
        status: "ready",
        title: t("liveProof.items.auth"),
        value:
          proof.subject === ""
            ? t("liveProof.auth.verified", { mode: modeValue })
            : proof.subject,
      };
    case "failed":
    case "blocked":
      return {
        detail:
          proof.detail || t("liveProof.auth.failedDetail", { mode: modeLabel }),
        key: "diagnosis-auth",
        status: "blocked",
        title: t("liveProof.items.auth"),
        value: t("liveProof.auth.failed", { mode: modeValue }),
      };
    case "checking":
      return {
        detail: t("liveProof.auth.checkingDetail"),
        key: "diagnosis-auth",
        status: "pending",
        title: t("liveProof.items.auth"),
        value: t("liveProof.auth.checking", { mode: modeValue }),
      };
    case "needs_check":
    case "pending":
      return {
        detail:
          proof.detail ||
          t("liveProof.auth.notCheckedDetail", { mode: modeLabel }),
        key: "diagnosis-auth",
        status: "pending",
        title: t("liveProof.items.auth"),
        value: t("liveProof.auth.notChecked", { mode: modeValue }),
      };
  }
  return {
    detail: t("liveProof.auth.notCheckedDetail", { mode: modeLabel }),
    key: "diagnosis-auth",
    status: "pending",
    title: t("liveProof.items.auth"),
    value: t("liveProof.auth.notChecked", { mode: modeValue }),
  };
}

function workflowAIDiagnosisProofItem(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
): LiveProofReadinessItem {
  if (topology.policy === null) {
    return {
      detail: t("liveProof.ai.noPolicyDetail"),
      key: "ai-diagnosis",
      status: "blocked",
      title: t("liveProof.items.aiDiagnosis"),
      value: t("status.noPolicy"),
    };
  }
  if (topology.status === "blocked") {
    return {
      detail: t("liveProof.ai.blockedDetail"),
      key: "ai-diagnosis",
      status: "blocked",
      title: t("liveProof.items.aiDiagnosis"),
      value: diagnosisFollowUpLabel(topology.policy.diagnosis_follow_up, t),
    };
  }
  if (topology.policy.diagnosis_follow_up === "disabled") {
    return {
      detail: t("liveProof.ai.disabledDetail"),
      key: "ai-diagnosis",
      status: "blocked",
      title: t("liveProof.items.aiDiagnosis"),
      value: t("followUp.disabled"),
    };
  }
  if (topology.policy.diagnosis_follow_up === "suggest_room") {
    return {
      detail: t("liveProof.ai.suggestRoomDetail"),
      key: "ai-diagnosis",
      status: "review",
      title: t("liveProof.items.aiDiagnosis"),
      value: t("liveProof.ai.operatorHandoff"),
    };
  }
  if (
    topology.activeAlertTools.length === 0 ||
    topology.metricTools.length === 0
  ) {
    return {
      detail: t("liveProof.ai.toolsDetail"),
      key: "ai-diagnosis",
      status: "review",
      title: t("liveProof.items.aiDiagnosis"),
      value: t("integration.autoRoom.toolCounts", {
        alerts: topology.activeAlertTools.length,
        metrics: topology.metricTools.length,
      }),
    };
  }
  return {
    detail: t("liveProof.ai.readyDetail"),
    key: "ai-diagnosis",
    status: "ready",
    title: t("liveProof.items.aiDiagnosis"),
    value: t("followUp.autoRoom"),
  };
}

function workflowNotificationProofItem(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
): LiveProofReadinessItem {
  if (topology.policy === null) {
    return {
      detail: t("liveProof.notification.noPolicyDetail"),
      key: "notification",
      status: "blocked",
      title: t("liveProof.items.notification"),
      value: t("status.noPolicy"),
    };
  }
  if (topology.notificationChannel === null) {
    return {
      detail:
        topology.policy.diagnosis_follow_up === "auto_room"
          ? t("liveProof.notification.noAIChannelDetail")
          : t("liveProof.notification.noReportChannelDetail"),
      key: "notification",
      status:
        topology.policy.diagnosis_follow_up === "auto_room"
          ? "blocked"
          : "pending",
      title: t("liveProof.items.notification"),
      value: t("status.noChannel"),
    };
  }
  const missingScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  if (!topology.notificationChannel.enabled) {
    return {
      detail: t("liveProof.notification.disabledDetail"),
      key: "notification",
      status: "blocked",
      title: t("liveProof.items.notification"),
      value: topology.notificationChannel.name,
    };
  }
  if (
    notificationChannelCredentialReadiness(
      channelToFormState(topology.notificationChannel),
    ).status !== "ready"
  ) {
    return {
      detail: t("notificationProof.blockedDetail"),
      key: "notification",
      status: "blocked",
      title: t("liveProof.items.notification"),
      value: topology.notificationChannel.name,
    };
  }
  if (missingScopes.length > 0) {
    return {
      detail: t("liveProof.notification.missingScopesDetail", {
        scopes: localizedDeliveryScopeList(missingScopes, t, locale),
      }),
      key: "notification",
      status: "blocked",
      title: t("liveProof.items.notification"),
      value: topology.notificationChannel.name,
    };
  }
  if (
    autoRoomChannelKindBlocked(topology.policy, topology.notificationChannel)
  ) {
    return {
      detail: t("liveProof.notification.wrongKindDetail"),
      key: "notification",
      status: "blocked",
      title: t("liveProof.items.notification"),
      value: topology.notificationChannel.name,
    };
  }
  if (topology.policy.diagnosis_follow_up === "auto_room") {
    const proofReadiness = notificationChannelAIProofReadiness(
      topology.notificationChannel,
      notificationChannelTestProofBundleFromResults(
        topology.notificationChannel.latest_test_results,
      ),
    );
    if (proofReadiness.status !== "ready") {
      return {
        detail: localizedNotificationProofGapDetail(
          proofReadiness,
          t,
          locale,
        ),
        key: "notification",
        status: proofReadiness.status === "blocked" ? "blocked" : "review",
        title: t("liveProof.items.notification"),
        value: topology.notificationChannel.name,
      };
    }
  }
  return {
    detail:
      topology.policy.diagnosis_follow_up === "auto_room"
        ? t("liveProof.notification.autoRoomReadyDetail")
        : t("liveProof.notification.reportReadyDetail"),
    key: "notification",
    status: "ready",
    title: t("liveProof.items.notification"),
    value: topology.notificationChannel.name,
  };
}

function workflowAlertmanagerAutoDiagnosisProofTarget(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
  proofHistory: AutoDiagnosisProofHistory | null,
): ProofTarget {
  const evidence = [
    t("proofTargets.evidence.alertmanagerWebhook"),
    "auto_room",
    "LLM",
    t("proofTargets.evidence.assistantMessage"),
    t("proofTargets.evidence.finalConclusion"),
    t("proofTargets.evidence.enterpriseWeChat"),
  ];
  if (topology.policy === null) {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "create-auto-room-policy",
      }),
      actionLabel: t("actions.createPolicy"),
      detail: t("proofTargets.autoDiagnosis.noPolicyDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (!topology.policy.enabled) {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: t("actions.enablePolicy"),
      detail: t("proofTargets.autoDiagnosis.disabledPolicyDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (topology.policy.diagnosis_follow_up !== "auto_room") {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "auto-room-follow-up",
        sourceID: topology.alertSource?.id ?? null,
      }),
      actionLabel: t("actions.useAutoRoom"),
      detail: t("proofTargets.autoDiagnosis.followUpDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status:
        topology.policy.diagnosis_follow_up === "disabled"
          ? "blocked"
          : "review",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (topology.alertSource === null || !topology.alertSource.enabled) {
    return {
      actionHref: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      actionLabel: t("actions.reviewSource"),
      detail: t("proofTargets.autoDiagnosis.noSourceDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (topology.alertSource.kind !== "alertmanager") {
    return {
      actionHref: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      actionLabel: t("actions.switchSource"),
      detail: t("proofTargets.autoDiagnosis.wrongSourceDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (
    topology.activeAlertTools.length === 0 ||
    topology.metricTools.length === 0
  ) {
    return {
      actionHref: diagnosisToolTemplateLaunchHref({
        intent:
          topology.activeAlertTools.length === 0
            ? "active-alert-tool"
            : "metric-evidence-tool",
        sourceID:
          topology.activeAlertTools.length === 0
            ? topology.alertSource.id
            : metricEvidenceToolLaunchSourceID(topology),
      }),
      actionLabel: t("actions.reviewTools"),
      detail: t("proofTargets.autoDiagnosis.toolsDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "review",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (topology.notificationChannel === null) {
    return {
      actionHref: notificationChannelAutoRoomLaunchHref(topology),
      actionLabel: t("actions.bindChannel"),
      detail: t("proofTargets.autoDiagnosis.noChannelDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (!topology.notificationChannel.enabled) {
    return {
      actionHref: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      actionLabel: t("actions.enableChannel"),
      detail: t("proofTargets.autoDiagnosis.disabledChannelDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (topology.notificationChannel.kind !== "wecom") {
    return {
      actionHref: notificationChannelAutoRoomLaunchHref(topology),
      actionLabel: t("actions.switchChannel"),
      detail: t("proofTargets.autoDiagnosis.wrongChannelDetail"),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  const missingScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  if (missingScopes.length > 0) {
    return {
      actionHref: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      actionLabel: t("actions.reviewScopes"),
      detail: t("proofTargets.autoDiagnosis.missingScopesDetail", {
        scopes: localizedDeliveryScopeList(missingScopes, t, locale),
      }),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  const rolloutReadiness = notificationChannelEnterpriseWeChatRolloutReadiness([
    topology.notificationChannel,
  ]);
  if (rolloutReadiness.status !== "ready") {
    return {
      actionHref: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      actionLabel:
        rolloutReadiness.status === "blocked"
          ? t("actions.fixChannel")
          : t("actions.testChannel"),
      detail: localizedNotificationProofGapDetail(
        rolloutReadiness,
        t,
        locale,
      ),
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: rolloutReadiness.status === "blocked" ? "blocked" : "review",
      title: t("proofTargets.autoDiagnosis.title"),
    };
  }
  if (proofHistory !== null) {
    return workflowAlertmanagerAutoDiagnosisHistoryTarget(
      proofHistory,
      evidence,
      t,
    );
  }
  return {
    actionHref: reportWorkflowPolicyLaunchHref({
      intent: "alertmanager-auto-diagnosis-proof",
      sourceID: topology.alertSource.id,
    }),
    actionLabel: t("actions.openProofPath"),
    detail: t("proofTargets.autoDiagnosis.readyDetail"),
    evidence,
    key: "alertmanager-auto-diagnosis",
    status: "ready",
    title: t("proofTargets.autoDiagnosis.title"),
  };
}

function workflowAlertmanagerAutoDiagnosisHistoryTarget(
  proofHistory: AutoDiagnosisProofHistory,
  evidence: string[],
  t: SettingsOverviewTranslator,
): ProofTarget {
  const status = autoDiagnosisProofHistoryTargetStatus(proofHistory.coverage);
  const coverage = localizedNotificationCoverage(proofHistory.coverage, t);
  return {
    actionHref: proofHistory.href,
    actionLabel:
      proofHistory.coverage.status === "ready"
        ? t("actions.reviewProof")
        : t("actions.reviewDelivery"),
    detail: t("proofTargets.autoDiagnosis.historyDetail", {
      alert: proofHistory.alertName,
      detail: coverage.detail,
      room: proofHistory.roomSessionID,
    }),
    evidence: [
      ...evidence,
      t("history.alertTag", { id: proofHistory.alertID }),
      t("history.snapshotTag", { id: proofHistory.snapshotID }),
      coverage.label,
    ],
    key: "alertmanager-auto-diagnosis",
    status,
    title: t("proofTargets.autoDiagnosis.title"),
  };
}

function autoDiagnosisProofHistoryTargetStatus(
  coverage: DiagnosisNotificationDeliveryCoverage,
): ProofReadinessStatus {
  switch (coverage.status) {
    case "ready":
      return "ready";
    case "blocked":
      return "blocked";
    case "review":
      return "review";
    case "pending":
      return "pending";
  }
}

function workflowReplayProofTarget(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
): ProofTarget {
  const evidence = [
    "PostgreSQL",
    "Temporal",
    t("proofTargets.evidence.alertSource"),
    t("proofTargets.evidence.toolTemplate"),
    "LLM",
    t("proofTargets.evidence.notification"),
  ];
  if (topology.policy === null) {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: t("actions.createPolicy"),
      detail: t("proofTargets.replay.noPolicyDetail"),
      evidence,
      key: "policy-replay",
      status: "blocked",
      title: t("proofTargets.replay.title"),
    };
  }
  if (topology.status === "blocked") {
    const action = workflowTopologyActions(topology, t, locale)[0] ?? null;
    return {
      actionHref: action?.href ?? "/settings/report-workflow-policies",
      actionLabel:
        action === null ? t("actions.openWorkflow") : t("actions.resolve"),
      detail: action?.detail ?? t("proofTargets.replay.blockedDetail"),
      evidence,
      key: "policy-replay",
      status: "blocked",
      title: t("proofTargets.replay.title"),
    };
  }
  if (topology.status === "review") {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: t("actions.reviewReplay"),
      detail: t("proofTargets.replay.reviewDetail"),
      evidence,
      key: "policy-replay",
      status: "review",
      title: t("proofTargets.replay.title"),
    };
  }
  return {
    actionHref: "/settings/report-workflow-policies",
    actionLabel: t("actions.runReplay"),
    detail: t("proofTargets.replay.readyDetail"),
    evidence,
    key: "policy-replay",
    status: "ready",
    title: t("proofTargets.replay.title"),
  };
}

function workflowScheduleProofTarget(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
): ProofTarget {
  const evidence = [
    t("proofTargets.evidence.enabledSchedule"),
    t("proofTargets.evidence.temporalAction"),
    t("proofTargets.evidence.launcherWorkflow"),
    t("proofTargets.evidence.reportDelivery"),
  ];
  if (topology.policy === null || topology.status === "blocked") {
    const action = workflowTopologyActions(topology, t, locale)[0] ?? null;
    return {
      actionHref: action?.href ?? "/settings/report-workflow-policies",
      actionLabel:
        action === null ? t("actions.openWorkflow") : t("actions.resolve"),
      detail: action?.detail ?? t("proofTargets.schedule.blockedDetail"),
      evidence,
      key: "scheduled-trigger",
      status: "blocked",
      title: t("proofTargets.schedule.title"),
    };
  }
  if (topology.activeSchedule === null) {
    return {
      actionHref: reportWorkflowScheduleLaunchHref({
        intent: "create-schedule",
        policyID: topology.policy.id,
      }),
      actionLabel: t("actions.createSchedule"),
      detail: t("proofTargets.schedule.noScheduleDetail"),
      evidence,
      key: "scheduled-trigger",
      status: "pending",
      title: t("proofTargets.schedule.title"),
    };
  }
  if (!topology.activeSchedule.enabled) {
    return {
      actionHref: "/settings/report-workflow-schedules",
      actionLabel: t("actions.enableSchedule"),
      detail: t("proofTargets.schedule.disabledDetail"),
      evidence,
      key: "scheduled-trigger",
      status: "review",
      title: t("proofTargets.schedule.title"),
    };
  }
  if (topology.status === "review") {
    return {
      actionHref: "/settings/report-workflow-schedules",
      actionLabel: t("actions.reviewSchedule"),
      detail: t("proofTargets.schedule.reviewDetail"),
      evidence,
      key: "scheduled-trigger",
      status: "review",
      title: t("proofTargets.schedule.title"),
    };
  }
  return {
    actionHref: "/settings/report-workflow-schedules",
    actionLabel: t("actions.reviewSchedule"),
    detail: t("proofTargets.schedule.readyDetail"),
    evidence,
    key: "scheduled-trigger",
    status: "ready",
    title: t("proofTargets.schedule.title"),
  };
}

function topologyStatus(
  policy: ReportWorkflowPolicy | null,
  alertSource: AlertSourceProfile | null,
  groupingPolicy: GroupingPolicy | null,
  notificationChannel: NotificationChannelProfile | null,
  activeAlertTools: DiagnosisToolTemplate[],
  metricTools: DiagnosisToolTemplate[],
): WorkflowTopology["status"] {
  if (policy === null || alertSource === null || groupingPolicy === null) {
    return "blocked";
  }
  if (!policy.enabled || !alertSource.enabled || !groupingPolicy.enabled) {
    return "blocked";
  }
  if (autoRoomWebhookBlocked(policy, alertSource)) {
    return "blocked";
  }
  if (
    notificationChannel !== null &&
    (!notificationChannel.enabled ||
      requiredDeliveryScopesMissing(policy, notificationChannel).length > 0)
  ) {
    return "blocked";
  }
  if (
    notificationChannel === null &&
    policy.diagnosis_follow_up === "auto_room"
  ) {
    return "blocked";
  }
  if (autoRoomChannelKindBlocked(policy, notificationChannel)) {
    return "blocked";
  }
  if (
    !autoRoomWebhookReady(policy, alertSource) ||
    activeAlertTools.length === 0 ||
    metricTools.length === 0
  ) {
    return "review";
  }
  return "ready";
}

function workflowTopologySteps(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
): WorkflowTopologyStep[] {
  const missingDeliveryScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  return [
    {
      title: t("topology.steps.source.title"),
      description:
        topology.alertSource === null
          ? t("status.missingSource")
          : autoRoomWebhookBlocked(topology.policy, topology.alertSource)
            ? t("topology.steps.source.notAlertmanager", {
                source: topology.alertSource.name,
              })
            : topology.alertSource.name,
      status:
        topology.alertSource?.enabled &&
        !autoRoomWebhookBlocked(topology.policy, topology.alertSource)
          ? "finish"
          : "error",
    },
    {
      title: t("topology.steps.grouping.title"),
      description: topology.groupingPolicy?.name ?? t("status.missingGrouping"),
      status: topology.groupingPolicy?.enabled ? "finish" : "error",
    },
    {
      title: t("topology.steps.evidence.title"),
      description: t("topology.steps.evidence.detail", {
        alerts: topology.activeAlertTools.length,
        metrics: topology.metricTools.length,
      }),
      status:
        topology.activeAlertTools.length > 0 && topology.metricTools.length > 0
          ? "finish"
          : "wait",
    },
    {
      title: t("topology.steps.aiRoom.title"),
      description:
        topology.policy === null
          ? t("status.noPolicy")
          : diagnosisFollowUpLabel(topology.policy.diagnosis_follow_up, t),
      status: autoRoomWebhookReady(topology.policy, topology.alertSource)
        ? "finish"
        : "wait",
    },
    {
      title: t("topology.steps.delivery.title"),
      description:
        topology.notificationChannel === null
          ? topology.policy?.diagnosis_follow_up === "auto_room"
            ? t("status.noWeComChannel")
            : t("status.noReportChannel")
          : missingDeliveryScopes.length > 0
            ? t("topology.steps.delivery.missingScopes", {
                channel: topology.notificationChannel.name,
                scopes: localizedDeliveryScopeList(
                  missingDeliveryScopes,
                  t,
                  locale,
                ),
              })
            : autoRoomChannelKindBlocked(
                  topology.policy,
                  topology.notificationChannel,
                )
              ? t("topology.steps.delivery.notWeCom", {
                  channel: topology.notificationChannel.name,
                })
              : topology.notificationChannel.name,
      status:
        topology.notificationChannel === null
          ? topology.policy?.diagnosis_follow_up === "auto_room"
            ? "error"
            : "finish"
          : topology.notificationChannel.enabled &&
              missingDeliveryScopes.length === 0 &&
              !autoRoomChannelKindBlocked(
                topology.policy,
                topology.notificationChannel,
              )
            ? "finish"
            : "error",
    },
    {
      title: t("topology.steps.schedule.title"),
      description: topology.activeSchedule?.name ?? t("status.manualReplay"),
      status: topology.activeSchedule?.enabled ? "finish" : "wait",
    },
  ];
}

function workflowTopologyDescriptions(
  topology: WorkflowTopology,
  t: SettingsOverviewTranslator,
  locale: string,
) {
  const missingDeliveryScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  return [
    {
      key: "policy",
      label: t("topology.descriptions.policy"),
      children:
        topology.policy === null ? t("status.none") : topology.policy.name,
    },
    {
      key: "source",
      label: t("topology.descriptions.alertSource"),
      children:
        topology.alertSource === null
          ? t("status.missing")
          : configLabel(
              topology.alertSource.name,
              topology.alertSource.enabled,
              t,
            ),
    },
    {
      key: "grouping",
      label: t("topology.descriptions.grouping"),
      children:
        topology.groupingPolicy === null
          ? t("status.missing")
          : configLabel(
              topology.groupingPolicy.name,
              topology.groupingPolicy.enabled,
              t,
            ),
    },
    {
      key: "tools",
      label: t("topology.descriptions.evidenceTools"),
      children:
        topology.diagnosisTools.length === 0 ? (
          <Typography.Text type="secondary">
            {t("topology.descriptions.noTools")}
          </Typography.Text>
        ) : (
          <Space direction="vertical" size={4}>
            <Space wrap>
              <Tag
                color={
                  topology.activeAlertTools.length > 0 ? "blue" : "default"
                }
              >
                {t("topology.descriptions.activeAlerts", {
                  count: topology.activeAlertTools.length,
                })}
              </Tag>
              <Tag color={topology.metricTools.length > 0 ? "cyan" : "default"}>
                {t("topology.descriptions.metricEvidence", {
                  count: topology.metricTools.length,
                })}
              </Tag>
            </Space>
            <Space wrap>
              {topology.diagnosisTools.map((tool) => (
                <Tag key={tool.id}>{tool.name}</Tag>
              ))}
            </Space>
          </Space>
        ),
    },
    {
      key: "channel",
      label: t("topology.descriptions.deliveryChannel"),
      children:
        topology.notificationChannel === null
          ? topology.policy?.diagnosis_follow_up === "auto_room"
            ? t("status.missingWeComChannel")
            : t("status.optional")
          : configLabel(
              missingDeliveryScopes.length > 0
                ? t("topology.steps.delivery.missingScopes", {
                    channel: topology.notificationChannel.name,
                    scopes: localizedDeliveryScopeList(
                      missingDeliveryScopes,
                      t,
                      locale,
                    ),
                  })
                : autoRoomChannelKindBlocked(
                      topology.policy,
                      topology.notificationChannel,
                    )
                  ? t("topology.steps.delivery.notWeCom", {
                      channel: topology.notificationChannel.name,
                    })
                  : topology.notificationChannel.name,
              topology.notificationChannel.enabled &&
                missingDeliveryScopes.length === 0 &&
                !autoRoomChannelKindBlocked(
                  topology.policy,
                  topology.notificationChannel,
                ),
              t,
            ),
    },
    {
      key: "schedule",
      label: t("topology.descriptions.schedule"),
      children:
        topology.activeSchedule === null
          ? t("status.manualReplay")
          : configLabel(
              topology.activeSchedule.name,
              topology.activeSchedule.enabled,
              t,
            ),
    },
  ];
}

function autoRoomWebhookReady(
  policy: ReportWorkflowPolicy | null,
  alertSource: AlertSourceProfile | null,
): boolean {
  return Boolean(
    policy?.enabled &&
    policy.diagnosis_follow_up === "auto_room" &&
    alertSource?.enabled &&
    alertSource.kind === "alertmanager",
  );
}

function autoRoomWebhookBlocked(
  policy: ReportWorkflowPolicy | null,
  alertSource: AlertSourceProfile | null,
): boolean {
  return Boolean(
    policy?.diagnosis_follow_up === "auto_room" &&
    alertSource !== null &&
    alertSource.enabled &&
    alertSource.kind !== "alertmanager",
  );
}

function requiredDeliveryScopesMissing(
  policy: ReportWorkflowPolicy | null,
  channel: NotificationChannelProfile | null,
): NotificationChannelProfile["delivery_scopes"] {
  if (channel === null) {
    return [];
  }
  const required: NotificationChannelProfile["delivery_scopes"] =
    policy?.diagnosis_follow_up === "auto_room"
      ? ["report", "diagnosis_consultation", "diagnosis_close"]
      : ["report"];
  return required.filter((scope) => !channel.delivery_scopes.includes(scope));
}

function autoRoomChannelKindBlocked(
  policy: ReportWorkflowPolicy | null,
  channel: NotificationChannelProfile | null,
): boolean {
  return Boolean(
    policy?.diagnosis_follow_up === "auto_room" &&
    channel !== null &&
    channel.kind !== "wecom",
  );
}

function isAIRoomFollowUp(
  mode: ReportWorkflowPolicy["diagnosis_follow_up"],
): boolean {
  return mode === "suggest_room" || mode === "auto_room";
}

function localizedDeliveryScopeList(
  scopes: readonly NotificationChannelProfile["delivery_scopes"][number][],
  t: SettingsOverviewTranslator,
  locale: string,
): string {
  return new Intl.ListFormat(locale, {
    style: "long",
    type: "conjunction",
  }).format(scopes.map((scope) => deliveryScopeLabel(scope, t)));
}

function deliveryScopeLabel(
  scope: NotificationChannelProfile["delivery_scopes"][number],
  t: SettingsOverviewTranslator,
): string {
  switch (scope) {
    case "report":
      return t("deliveryScopes.report");
    case "diagnosis_consultation":
      return t("deliveryScopes.consultation");
    case "diagnosis_close":
      return t("deliveryScopes.close");
  }
}

function localizedNotificationProofGapDetail(
  readiness: {
    missingContentKinds: NotificationChannelTestContentKind[];
    status: "blocked" | "pending" | "ready" | "review";
  },
  t: SettingsOverviewTranslator,
  locale: string,
): string {
  if (readiness.status === "blocked") {
    return t("notificationProof.blockedDetail");
  }
  if (readiness.missingContentKinds.length === 0) {
    return t("notificationProof.reviewDetail");
  }
  const tests = new Intl.ListFormat(locale, {
    style: "long",
    type: "conjunction",
  }).format(
    readiness.missingContentKinds.map((kind) =>
      notificationTestContentKindLabel(kind, t),
    ),
  );
  return t("notificationProof.missingTestsDetail", { tests });
}

function notificationTestContentKindLabel(
  kind: NotificationChannelTestContentKind,
  t: SettingsOverviewTranslator,
): string {
  switch (kind) {
    case "transport_sample":
      return t("notificationProof.tests.transport");
    case "ai_diagnosis_sample":
      return t("notificationProof.tests.aiDiagnosis");
    case "diagnosis_close_sample":
      return t("notificationProof.tests.diagnosisClose");
  }
}

function sourceKindLabel(
  kind: AlertSourceProfile["kind"],
  t: SettingsOverviewTranslator,
): string {
  switch (kind) {
    case "alertmanager":
      return t("sourceKinds.alertmanager");
    case "prometheus":
      return t("sourceKinds.prometheus");
  }
}

function diagnosisFollowUpLabel(
  mode: ReportWorkflowPolicy["diagnosis_follow_up"],
  t: SettingsOverviewTranslator,
): string {
  switch (mode) {
    case "disabled":
      return t("followUp.disabled");
    case "suggest_room":
      return t("followUp.suggestRoom");
    case "auto_room":
      return t("followUp.autoRoom");
  }
}

function settingsAuthModeLabel(
  mode: DiagnosisAuthMode,
  t: SettingsOverviewTranslator,
): string {
  switch (mode) {
    case "bearer":
      return t("authModes.bearer");
    case "ldap":
      return t("authModes.ldap");
    case "wecom":
      return t("authModes.weCom");
    case "session":
      return t("authModes.session");
  }
}

function localizedNotificationCoverage(
  coverage: DiagnosisNotificationDeliveryCoverage,
  t: SettingsOverviewTranslator,
) {
  const phases = coverage.phases.map((phase) => ({
    ...phase,
    label: notificationPhaseLabel(phase.key, t),
  }));
  switch (coverage.status) {
    case "ready":
      return {
        ...coverage,
        detail: t("history.coverage.readyDetail"),
        label: t("history.coverage.readyLabel"),
        phases,
      };
    case "blocked":
      return {
        ...coverage,
        detail: t("history.coverage.blockedDetail"),
        label: t("history.coverage.blockedLabel"),
        phases,
      };
    case "pending":
      return {
        ...coverage,
        detail: t("history.coverage.pendingDetail"),
        label: t("history.coverage.pendingLabel"),
        phases,
      };
    case "review":
      return {
        ...coverage,
        detail: t("history.coverage.reviewDetail", {
          ready: coverage.readyCount,
          required: coverage.requiredCount,
        }),
        label: t("history.coverage.reviewLabel"),
        phases,
      };
  }
}

function notificationPhaseLabel(
  key: DiagnosisNotificationDeliveryCoverage["phases"][number]["key"],
  t: SettingsOverviewTranslator,
): string {
  switch (key) {
    case "assistant_update":
      return t("history.phases.assistantUpdate");
    case "final_conclusion":
      return t("history.phases.finalConclusion");
    case "close":
      return t("history.phases.close");
  }
}

function notificationPhaseStatusLabel(
  status: DiagnosisNotificationDeliveryCoverage["phases"][number]["status"],
  t: SettingsOverviewTranslator,
): string {
  switch (status) {
    case "delivered":
      return t("history.phaseStatus.delivered");
    case "failed":
      return t("history.phaseStatus.failed");
    case "missing":
      return t("history.phaseStatus.missing");
    case "pending":
      return t("history.phaseStatus.pending");
    case "unproven":
      return t("history.phaseStatus.unproven");
  }
}

function configLabel(
  name: string,
  enabled: boolean,
  t: SettingsOverviewTranslator,
) {
  return (
    <Space size={6} wrap>
      <Typography.Text>{name}</Typography.Text>
      <Tag color={enabled ? "green" : "default"}>
        {enabled ? t("status.enabled") : t("status.draft")}
      </Tag>
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

function topologyStatusLabel(
  status: WorkflowTopology["status"],
  t: SettingsOverviewTranslator,
): string {
  switch (status) {
    case "ready":
      return t("topology.status.ready");
    case "review":
      return t("topology.status.review");
    case "blocked":
      return t("topology.status.blocked");
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

function formatCount(count: number | null, locale: string): string {
  if (count === null) {
    return "-";
  }
  return new Intl.NumberFormat(locale).format(count);
}

function isReadyCount(count: number | null): boolean {
  return count !== null && count > 0;
}

function shortStageTitle(
  key: Stage["key"],
  t: SettingsOverviewTranslator,
): string {
  switch (key) {
    case "sources":
      return t("stages.short.source");
    case "workflow":
      return t("stages.short.policy");
    case "channels":
      return t("stages.short.channel");
    case "schedules":
      return t("stages.short.schedule");
    case "grouping":
      return t("stages.grouping");
    case "tools":
      return t("stages.diagnosisTools");
  }
}

function proofTargetStatusColor(status: ProofTarget["status"]): string {
  switch (status) {
    case "ready":
      return "green";
    case "review":
      return "gold";
    case "pending":
      return "default";
    case "blocked":
      return "red";
  }
}

function proofTargetStatusLabel(
  status: ProofTarget["status"],
  t: SettingsOverviewTranslator,
): string {
  switch (status) {
    case "ready":
      return t("status.readyToRun");
    case "review":
      return t("status.review");
    case "pending":
      return t("status.pending");
    case "blocked":
      return t("status.blocked");
  }
}

function aggregateProofReadiness(
  statuses: ProofReadinessStatus[],
): ProofReadinessStatus {
  if (statuses.includes("blocked")) {
    return "blocked";
  }
  if (statuses.includes("review")) {
    return "review";
  }
  if (statuses.includes("pending")) {
    return "pending";
  }
  return "ready";
}

function liveProofReadinessDetail(
  status: ProofReadinessStatus,
  t: SettingsOverviewTranslator,
): string {
  switch (status) {
    case "ready":
      return t("liveProof.detail.ready");
    case "review":
      return t("liveProof.detail.review");
    case "pending":
      return t("liveProof.detail.pending");
    case "blocked":
      return t("liveProof.detail.blocked");
  }
}

function proofReadinessStepStatus(
  status: ProofReadinessStatus,
): "error" | "finish" | "process" | "wait" {
  switch (status) {
    case "ready":
      return "finish";
    case "review":
      return "process";
    case "pending":
      return "wait";
    case "blocked":
      return "error";
  }
}

function stepStatus(
  count: number | null,
  index: number,
  currentStep: number,
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
  const t = useTranslations("SettingsOverview");
  if (count === null) {
    return <Tag color="red">{t("status.unavailable")}</Tag>;
  }
  if (count === 0) {
    return <Tag color="gold">{t("status.needsSetup")}</Tag>;
  }
  return <Tag color="green">{t("status.ready")}</Tag>;
}
