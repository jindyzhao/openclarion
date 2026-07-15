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
import { useTranslations } from "next-intl";
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
  notificationChannelEditHref,
  notificationChannelAIProofReadiness,
  notificationChannelEnterpriseWeChatRolloutReadiness,
  notificationChannelLaunchHref,
  notificationChannelTestProofBundleFromResults,
} from "../notification-channels/format";
import type { NotificationChannelProfile } from "../notification-channels/types";
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

type Stage = {
  count: number | null;
  countLabel: string;
  href: string;
  icon: ReactNode;
  key: string;
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
  | "backend_readiness"
  | "browser_session"
  | "credentials";

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

export type SettingsWeComAppCallbackGuidance = {
  detail: string;
  envVar: string;
  label: string;
  value: string;
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
      countLabel: "profiles",
      href: "/settings/alert-sources",
      icon: <ApiOutlined aria-hidden="true" />,
      key: "sources",
      title: "Alert sources",
    },
    {
      count: counts.groupingPolicies,
      countLabel: "policies",
      href: "/settings/grouping-policies",
      icon: <PartitionOutlined aria-hidden="true" />,
      key: "grouping",
      title: "Grouping",
    },
    {
      count: counts.diagnosisToolTemplates,
      countLabel: "templates",
      href: "/settings/diagnosis-tool-templates",
      icon: <ToolOutlined aria-hidden="true" />,
      key: "tools",
      title: "Diagnosis tools",
    },
    {
      count: counts.notificationChannels,
      countLabel: "channels",
      href: "/settings/notification-channels",
      icon: <BellOutlined aria-hidden="true" />,
      key: "channels",
      title: "Notifications",
    },
    {
      count: counts.workflowPolicies,
      countLabel: "policies",
      href: "/settings/report-workflow-policies",
      icon: <BranchesOutlined aria-hidden="true" />,
      key: "workflow",
      title: "Workflow policies",
    },
    {
      count: counts.workflowSchedules,
      countLabel: "schedules",
      href: "/settings/report-workflow-schedules",
      icon: <CalendarOutlined aria-hidden="true" />,
      key: "schedules",
      title: "Schedules",
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
      ),
    [alerts, topology.alertSource?.id],
  );
  const autoDiagnosisProofHistories = useMemo(
    () => autoDiagnosisProofHistoriesForAlerts(alerts, 5),
    [alerts],
  );
  const topologyActions = workflowTopologyActions(topology);
  const nextTopologyAction = topologyActions[0] ?? null;
  const nextSetup =
    nextStage === null && nextTopologyAction !== null
      ? {
          actionLabel: "Open",
          detail: nextTopologyAction.detail,
          href: nextTopologyAction.href,
          tagColor: actionPriorityColor(nextTopologyAction.priority),
          tagLabel: `${nextTopologyAction.priority} priority`,
          title: nextTopologyAction.title,
        }
      : nextStage === null
        ? {
            actionLabel: "",
            detail:
              "Run and retain live proof against the configured alert source, workflow, LLM, and notification path.",
            href: "",
            tagColor: "gold",
            tagLabel: "Proof pending",
            title: "Retained live proof",
          }
        : {
            actionLabel: "Configure",
            detail: `Create or verify ${nextStage.title.toLowerCase()} before continuing the alert operations graph.`,
            href: nextStage.href,
            tagColor: "gold",
            tagLabel: "Needs setup",
            title: nextStage.title,
          };
  const proofTargets = workflowProofTargets(
    topology,
    autoDiagnosisProofHistory,
  );
  const liveProofReadiness = workflowLiveProofReadiness(
    topology,
    diagnosisAuthProof,
  );
  const localAccessReadiness = settingsLocalAccessReadiness({
    directoryDepartments,
    directorySyncRuns,
    directoryUsers,
    rbacAssignments,
  });
  const integrationReadiness = workflowIntegrationReadiness(
    topology,
    authT,
    diagnosisAuthProof,
    localAccessReadiness,
    currentRBACAccess,
  );

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
                title: shortStageTitle(stage.title),
              })),
              {
                icon: <PlayCircleOutlined />,
                status: allConfigurationPresent ? "process" : "wait",
                title: "Proof",
              },
            ]}
            responsive={false}
            type="inline"
          />
        </div>
      </section>

      <section
        aria-label="Next setup stage"
        className="panel settings-overview-next"
      >
        <div>
          <Typography.Text className="muted">Next setup</Typography.Text>
          <Typography.Title level={2}>{nextSetup.title}</Typography.Title>
          <Typography.Paragraph className="settings-overview-next-copy">
            {nextSetup.detail}
          </Typography.Paragraph>
        </div>
        {nextSetup.href === "" ? (
          <Tag color={nextSetup.tagColor}>{nextSetup.tagLabel}</Tag>
        ) : (
          <Space wrap>
            <Tag color={nextSetup.tagColor}>{nextSetup.tagLabel}</Tag>
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

      <Row aria-label="Retained proof targets" gutter={[16, 16]}>
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
                      {proofTargetStatusLabel(target.status)}
                    </Tag>
                    {allConfigurationPresent ? (
                      <Tag color="gold">Configuration present</Tag>
                    ) : (
                      <Tag>Waiting for graph</Tag>
                    )}
                  </Space>
                  <Typography.Paragraph className="settings-overview-proof-copy">
                    {target.detail}
                  </Typography.Paragraph>
                </Space>
                <div
                  className="settings-overview-proof-tags"
                  aria-label={`${target.title} evidence`}
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

      <Row aria-label="Settings surfaces" gutter={[16, 16]}>
        {stages.map((stage) => (
          <Col key={stage.key} lg={8} md={12} xs={24}>
            <Card
              className="settings-overview-card"
              title={<CardTitle icon={stage.icon} title={stage.title} />}
            >
              <div className="settings-overview-card-body">
                <div>
                  <div className="settings-overview-count">
                    {formatCount(stage.count)}
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
                Open
              </Button>
            </Card>
          </Col>
        ))}
      </Row>

      <Row aria-label="Access control surface" gutter={[16, 16]}>
        <Col lg={8} md={12} xs={24}>
          <Card
            className="settings-overview-card"
            title={
              <CardTitle
                icon={<AuditOutlined aria-hidden="true" />}
                title="Directory & RBAC"
              />
            }
          >
            <div className="settings-overview-card-body">
              <div>
                <div className="settings-overview-count">
                  {formatCount(
                    counts.rbacAssignments ?? rbacAssignments.length,
                  )}
                </div>
                <Typography.Text className="muted">rules</Typography.Text>
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
                  {settingsLocalAccessSyncValue(localAccessReadiness)}
                </Tag>
                <Tag color={directoryUsers.length > 0 ? "green" : "gold"}>
                  {directoryUsers.length} users
                </Tag>
                <Tag color={directoryDepartments.length > 0 ? "green" : "gold"}>
                  {directoryDepartments.length} departments
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
                {currentRBACAccess.needsSignIn ? "Sign in with IAM" : "Open"}
              </Button>
            </Space>
          </Card>
        </Col>
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
        message={
          allConfigurationPresent ? "Live proof gate" : "Configuration gate"
        }
        showIcon
        type={allConfigurationPresent ? "warning" : "info"}
        description={
          allConfigurationPresent
            ? "Policy replay, Alertmanager auto-diagnosis, and scheduled-trigger acceptance still require retained proof against real PostgreSQL, Temporal, alert-source, LLM, and notification delivery inputs."
            : "Retained proof targets activate only after the alert source, grouping, notification, workflow policy, and schedule objects exist server-side."
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
  return (
    <section
      aria-label="Operator rollout readiness"
      className="panel settings-overview-integration"
    >
      <div className="panel-header settings-overview-integration-header">
        <h2>Operator rollout readiness</h2>
        <Tag color={proofTargetStatusColor(readiness.status)}>
          {proofTargetStatusLabel(readiness.status)}
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
                    {proofTargetStatusLabel(item.status)}
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
      : settingsWeComAuthErrorDetail(weComAuthError);
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
  const weComBrowserSessionReadiness =
    settingsWeComBrowserSessionReadiness(browserSession);
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
      aria-label="Diagnosis auth readiness"
      className="panel settings-overview-auth"
      id={diagnosisAuthReadinessFocusTarget}
    >
      <div className="panel-header settings-overview-auth-header">
        <h2>Diagnosis auth readiness</h2>
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
              <Form.Item label="Authentication" name="authMode">
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
                          Sign in with IAM
                        </Button>
                      </Space>
                    }
                    description={
                      <Space direction="vertical" size={6}>
                        <Typography.Text>
                          Enterprise WeChat browser login has been replaced by
                          IAM OIDC. OpenClarion uses the IAM browser session for
                          operator identity and keeps Enterprise WeChat for app
                          messages, notification delivery, and diagnosis-room
                          collaboration callbacks.
                        </Typography.Text>
                      </Space>
                    }
                    message="Enterprise WeChat browser sign-in migrated"
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
                          Sign in with IAM
                        </Button>
                      ) : null}
                      <Button
                        disabled={
                          browserSession.loading || browserSessionClearing
                        }
                        icon={<ReloadOutlined />}
                        onClick={onRefreshBrowserSession}
                      >
                        Refresh session
                      </Button>
                      {browserSession.status?.authenticated === true ? (
                        <Button
                          disabled={browserSession.loading}
                          icon={<DisconnectOutlined />}
                          loading={browserSessionClearing}
                          onClick={onSignOutBrowserSession}
                        >
                          Sign out
                        </Button>
                      ) : null}
                    </Space>
                  }
                  description={diagnosisBrowserSessionStatusDetail(
                    browserSession,
                    authT,
                  )}
                  message="OpenClarion browser session"
                  showIcon
                  type={diagnosisBrowserSessionStatusAlertType(browserSession)}
                />
              ) : mode === "bearer" ? (
                <Form.Item label="Bearer token" name="bearerToken">
                  <Input.Password autoComplete="off" />
                </Form.Item>
              ) : (
                <>
                  <Form.Item label="LDAP username" name="ldapUsername">
                    <Input autoComplete="username" />
                  </Form.Item>
                  <Form.Item label="LDAP password" name="ldapPassword">
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
                  Check auth
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
                  Reset
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
                message="Backend diagnosis auth wiring"
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
                          Sign in with IAM
                        </Button>
                      ) : null}
                      <Button
                        disabled={
                          browserSession.loading || browserSessionClearing
                        }
                        icon={<ReloadOutlined />}
                        onClick={onRefreshBrowserSession}
                      >
                        Refresh session
                      </Button>
                      {browserSession.status?.authenticated === true ? (
                        <Button
                          disabled={browserSession.loading}
                          icon={<DisconnectOutlined />}
                          loading={browserSessionClearing}
                          onClick={onSignOutBrowserSession}
                        >
                          Sign out
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
                      ? "Enterprise WeChat browser session"
                      : "OpenClarion browser session"
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
                Credential values are used only for this backend check request
                and are not saved in OpenClarion settings.
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
): string {
  switch (mode) {
    case "ldap":
      return "LDAP";
    case "static":
      return "static bearer auth";
    case "oidc":
      return "IAM OIDC";
    case "unknown":
    case "none":
      return "the configured backend provider";
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

export function settingsWeComAppCallbackURL(
  origin: string | null,
): string | null {
  const normalizedOrigin = settingsNormalizedPublicOrigin(origin);
  if (normalizedOrigin === null) {
    return null;
  }
  return new URL(
    "/api/v1/diagnosis/wecom/app-callback",
    normalizedOrigin,
  ).toString();
}

function settingsNormalizedPublicOrigin(origin: string | null): string | null {
  const value = origin?.trim() ?? "";
  if (value === "") {
    return null;
  }
  try {
    const parsed = new URL(value);
    if (
      parsed.username !== "" ||
      parsed.password !== "" ||
      parsed.search !== "" ||
      parsed.hash !== "" ||
      (parsed.protocol !== "https:" && parsed.protocol !== "http:")
    ) {
      return null;
    }
    return parsed.origin;
  } catch {
    return null;
  }
}

export function settingsWeComAppCallbackGuidance(): SettingsWeComAppCallbackGuidance[] {
  return [
    {
      detail:
        "Shared verification token configured on the Enterprise WeChat app message receiver. It is used with timestamp, nonce, and encrypted payload data to validate callback signatures.",
      envVar: "OPENCLARION_WECOM_CALLBACK_TOKEN",
      label: "Callback token",
      value: "Signature verification",
    },
    {
      detail:
        "Base64 EncodingAESKey configured on the same Enterprise WeChat app message receiver. OpenClarion uses it to decrypt URL verification echoes and encrypted XML callbacks.",
      envVar: "OPENCLARION_WECOM_CALLBACK_ENCODING_AES_KEY",
      label: "Encoding AES key",
      value: "Encrypted callback body",
    },
    {
      detail:
        "Receive ID checked after callback decryption. Leave unset only when the Enterprise WeChat Corp ID is the expected receive ID.",
      envVar: "OPENCLARION_WECOM_CALLBACK_RECEIVE_ID",
      label: "Receive ID",
      value: "Corp or app receive ID",
    },
  ];
}

export function settingsLDAPTransportGuidance(): SettingsLDAPConfigurationGuidance[] {
  return [
    {
      detail:
        "Directory endpoint. Prefer ldaps://; ldap:// requires StartTLS unless plaintext is explicitly allowed for local development.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_URL",
      label: "LDAP URL",
      value: "Directory endpoint",
    },
    {
      detail:
        "Upgrades ldap:// connections before service bind, search, and user bind.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_START_TLS",
      label: "StartTLS",
      value: "Encrypted ldap:// transport",
    },
    {
      detail:
        "Explicit local-only plaintext allowance. Keep unset for production rollout.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT",
      label: "Plaintext allowance",
      value: "Development fallback",
    },
  ];
}

export function settingsLDAPDirectorySearchGuidance(): SettingsLDAPConfigurationGuidance[] {
  return [
    {
      detail:
        "Search root used to locate the operator entry before the user bind.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
      label: "Base DN",
      value: "Search root",
    },
    {
      detail:
        "Optional service account DN used for the initial directory search.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN",
      label: "Bind DN",
      value: "Search account",
    },
    {
      detail:
        "Optional service account password. Configure only together with the bind DN.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD",
      label: "Bind password",
      value: "Search credential",
    },
    {
      detail:
        "LDAP filter template. Use {username} for the escaped login name.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER",
      label: "User filter",
      value: "User lookup",
    },
    {
      detail:
        "Optional attribute used as the OpenClarion subject. Leave unset to use the entry DN.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE",
      label: "Subject attribute",
      value: "Principal subject",
    },
  ];
}

export function settingsLDAPRoleMappingGuidance(): SettingsLDAPConfigurationGuidance[] {
  return [
    {
      detail:
        "LDAP attribute read from the matched user entry to evaluate owner/admin mapping values.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE",
      label: "Role attribute",
      value: "Directory role source",
    },
    {
      detail: "Role attribute values that grant OpenClarion owner access.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES",
      label: "Owner role values",
      value: "Owner mapping",
    },
    {
      detail: "Role attribute values that grant OpenClarion admin access.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES",
      label: "Admin role values",
      value: "Admin mapping",
    },
    {
      detail:
        "Fallback OpenClarion roles for every authenticated LDAP user. Use only for controlled pilots or local checks.",
      envVar: "OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES",
      label: "Default roles",
      value: "Fallback role mapping",
    },
  ];
}

export function settingsWeComBrowserSessionReadiness(
  browserSession: DiagnosisBrowserSessionState,
): SettingsWeComBrowserSessionReadiness {
  if (browserSession.loading) {
    return {
      detail:
        "Checking whether this browser already has an OpenClarion session before Enterprise WeChat rollout proof.",
      items: [
        {
          detail: "OpenClarion is checking the current browser session.",
          key: "session",
          label: "Session",
          status: "loading",
          value: "Checking",
        },
        {
          detail:
            "Provider source is available after the browser session check finishes.",
          key: "provider",
          label: "Provider",
          status: "loading",
          value: "Checking",
        },
        {
          detail:
            "Local RBAC authorization can be evaluated after the browser session identity check finishes.",
          key: "role",
          label: "Role",
          status: "loading",
          value: "Checking",
        },
        {
          detail:
            "Operator subject is available after the browser session check finishes.",
          key: "subject",
          label: "Subject",
          status: "loading",
          value: "Checking",
        },
      ],
      label: "WeCom session check running",
      status: "loading",
    };
  }
  if (browserSession.status === null) {
    const detail =
      browserSession.message.trim() ||
      "OpenClarion browser session status is unavailable.";
    return {
      detail,
      items: [
        {
          detail,
          key: "session",
          label: "Session",
          status: "unavailable",
          value: "Unavailable",
        },
        {
          detail:
            "Provider source cannot be evaluated until the browser session endpoint responds.",
          key: "provider",
          label: "Provider",
          status: "unavailable",
          value: "Unavailable",
        },
        {
          detail:
            "Local RBAC authorization cannot be evaluated until the browser session endpoint responds.",
          key: "role",
          label: "Role",
          status: "unavailable",
          value: "Unavailable",
        },
        {
          detail:
            "Operator subject cannot be evaluated until the browser session endpoint responds.",
          key: "subject",
          label: "Subject",
          status: "unavailable",
          value: "Unavailable",
        },
      ],
      label: "WeCom session unavailable",
      status: "unavailable",
    };
  }
  if (!browserSession.status.authenticated) {
    return {
      detail:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Sign in with IAM before accepting OpenClarion browser-session proof.",
      items: [
        {
          detail: "No OpenClarion browser session is active.",
          key: "session",
          label: "Session",
          status: "blocked",
          value: "Not signed in",
        },
        {
          detail:
            "Enterprise WeChat identity is required for WeCom rollout proof.",
          key: "provider",
          label: "Provider",
          status: "blocked",
          value: "Enterprise WeChat required",
        },
        {
          detail:
            "Local RBAC authorization is checked after Enterprise WeChat sign-in.",
          key: "role",
          label: "Role",
          status: "blocked",
          value: "RBAC pending",
        },
        {
          detail: "No operator subject is available before sign-in.",
          key: "subject",
          label: "Subject",
          status: "unavailable",
          value: "Not available",
        },
      ],
      label: "WeCom session required",
      status: "blocked",
    };
  }

  const session = browserSession.status;
  const providerValue = diagnosisAuthBrowserSessionSourceValueLabel(
    session.mode,
  );
  const subject = session.subject.trim() || "Not reported";
  const roles =
    session.roles.length === 0 ? "No roles" : session.roles.join(", ");
  const providerReady = false;
  const status: SettingsWeComReadinessStatus = "blocked";
  return {
    checkedAt: session.checked_at,
    detail:
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the active IAM browser session for OpenClarion authorization and keep Enterprise WeChat only for app messages and notifications.",
    items: [
      {
        detail: providerReady
          ? "The active browser session was created by Enterprise WeChat."
          : "OpenClarion no longer accepts Enterprise WeChat as a browser session provider.",
        key: "session",
        label: "Session",
        status,
        value: `${providerValue} session`,
      },
      {
        detail: providerReady
          ? "Enterprise WeChat is the source provider for this browser session."
          : "Use IAM OIDC sign-in for browser authentication. Enterprise WeChat remains available for app messages and notification delivery.",
        key: "provider",
        label: "Provider",
        status: providerReady ? "ready" : "blocked",
        value: providerValue,
      },
      {
        detail:
          "Identity is verified. Diagnosis room permissions are assigned in OpenClarion local RBAC, not by provider role mapping.",
        key: "role",
        label: "Role",
        status: "ready",
        value: roles,
      },
      {
        detail: "Subject returned by the active browser session.",
        key: "subject",
        label: "Subject",
        status: "ready",
        value: subject,
      },
    ],
    label: "WeCom browser session replaced",
    status,
  };
}

function DiagnosisAuthWeComSessionSummary({
  readiness,
}: {
  readiness: SettingsWeComBrowserSessionReadiness;
}) {
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
                      {settingsWeComReadinessStatusLabel(item.status)}
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
              Checked at {readiness.checkedAt}
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
) {
  if (result === null) {
    return [
      {
        key: "mode",
        label: "Mode",
        children: authProbeModeLabel(mode),
      },
      {
        key: "result",
        label: "Backend check",
        children: "Not checked",
      },
    ];
  }
  const roles =
    result.roles.length === 0 ? "no roles" : result.roles.join(", ");
  return [
    {
      key: "mode",
      label: "Mode",
      children: authProbeModeLabel(result.mode),
    },
    {
      key: "status",
      label: "Backend check",
      children: (
        <Space size={6} wrap>
          <Tag color={result.status === "success" ? "green" : "red"}>
            {result.status === "success" ? "Success" : "Failed"}
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
      label: "Subject",
      children: result.subject === "" ? "Not available" : result.subject,
    },
    {
      key: "roles",
      label: "Roles",
      children: roles,
    },
    {
      key: "role-authorized",
      label: "Provider role metadata",
      children:
        (result.roleAuthorized ?? diagnosisAuthProbeResultAuthorized(result))
          ? "Reported"
          : "Not required",
    },
    {
      key: "checked-at",
      label: "Checked at",
      children: result.checkedAt?.trim() || "Not available",
    },
  ];
}

export function diagnosisAuthProbeResultDetail(
  result: Pick<
    DiagnosisAuthProbeResult,
    "localFailure" | "message" | "status"
  >,
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
) {
  return [
    {
      key: "required-provider",
      label: "Required provider",
      children: "LDAP or Enterprise WeChat authentication",
    },
    {
      key: "backend-provider",
      label: "Backend provider",
      children: diagnosisAuthStatusLabel(backendStatus, t),
    },
    {
      key: "supported-modes",
      label: "Supported modes",
      children: (
        <DiagnosisAuthSupportedModesSummary status={backendStatus.status} />
      ),
    },
    {
      key: "proof-mode",
      label: "Checked mode",
      children: authProbeModeLabel(readiness.mode),
    },
    {
      key: "owner-admin",
      label: "Provider roles",
      children: (
        <Tag color={readiness.roleAuthorized === true ? "green" : "gold"}>
          {readiness.roleAuthorized === true ? "Reported" : "Optional"}
        </Tag>
      ),
    },
    {
      key: "backend-role-mapping",
      label: "Backend mapping",
      children: (
        <DiagnosisAuthRoleMappingSummary
          loading={backendStatus.loading}
          status={backendStatus.status}
        />
      ),
    },
    {
      key: "role-mapping-guidance",
      label: "Role mapping",
      children: (
        <Typography.Text type="secondary">
          {diagnosisAuthRoleMappingGuidance(readiness, t)}
        </Typography.Text>
      ),
    },
    {
      key: "rollout-decision",
      label: "Rollout decision",
      children: (
        <Space size={6} wrap>
          <Tag color={diagnosisAuthRolloutTagColor(readiness.status)}>
            {diagnosisAuthRolloutStatusLabel(readiness.status, t)}
          </Tag>
          <Typography.Text type="secondary">
            {readiness.subject === ""
              ? "No subject checked"
              : readiness.subject}
          </Typography.Text>
        </Space>
      ),
    },
    {
      key: "checked-at",
      label: "Checked at",
      children: readiness.checkedAt?.trim() || "Not available",
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
            <Descriptions.Item label="Transport envs">
              <DiagnosisAuthLDAPConfigurationGuidanceValue
                items={settingsLDAPTransportGuidance()}
              />
            </Descriptions.Item>
            <Descriptions.Item label="Directory search envs">
              <DiagnosisAuthLDAPConfigurationGuidanceValue
                items={settingsLDAPDirectorySearchGuidance()}
              />
            </Descriptions.Item>
            <Descriptions.Item label="Role mapping envs">
              <DiagnosisAuthLDAPConfigurationGuidanceValue
                items={settingsLDAPRoleMappingGuidance()}
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
): { description: string; message: string } {
  switch (error) {
    case "wecom_entry_unavailable":
      return {
        description:
          "Enterprise WeChat browser login has been replaced by IAM OIDC. Sign in with IAM, then retry the diagnosis-room action.",
        message: "Enterprise WeChat login entry was not available.",
      };
    case "wecom_auth_failed":
      return {
        description:
          "Enterprise WeChat returned to OpenClarion, but the backend could not establish a diagnosis identity. Check the Enterprise WeChat application configuration, then run Check auth again.",
        message: "Enterprise WeChat authentication was not accepted.",
      };
    case "wecom_role_unauthorized":
      return {
        description:
          "Enterprise WeChat identified the operator, but OpenClarion local RBAC did not grant diagnosis room access. OpenClarion cleared any previous browser session; assign the operator to the right local role, then sign in again.",
        message: "Enterprise WeChat identity is not authorized locally.",
      };
    case "wecom_callback_failed":
      return {
        description:
          "OpenClarion could not complete the Enterprise WeChat callback. Check the Enterprise WeChat application configuration and callback endpoint.",
        message: "Enterprise WeChat callback could not be completed.",
      };
    case "wecom_callback_missing":
      return {
        description:
          "Enterprise WeChat browser login has been replaced by IAM OIDC. Sign in with IAM, then retry the diagnosis-room action.",
        message: "Enterprise WeChat callback was incomplete.",
      };
    case "wecom_login_failed":
      return {
        description:
          "OpenClarion could not start Enterprise WeChat login. Check the Enterprise WeChat app credentials, redirect URI, and provider endpoint reachability.",
        message: "Enterprise WeChat login could not be started.",
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
): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "review":
      return "Review";
    case "blocked":
      return "Blocked";
    case "loading":
      return "Loading";
    case "unavailable":
      return "Unavailable";
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
    return <Typography.Text type="secondary">None reported</Typography.Text>;
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

function authProbeModeLabel(mode: DiagnosisAuthMode): string {
  switch (mode) {
    case "ldap":
      return "LDAP basic auth";
    case "bearer":
      return "Bearer token";
    case "session":
      return "OpenClarion browser session";
    case "wecom":
      return "Enterprise WeChat authentication";
  }
}

function authProbeModeValueLabel(mode: DiagnosisAuthMode): string {
  switch (mode) {
    case "ldap":
      return "LDAP";
    case "bearer":
      return "Bearer";
    case "session":
      return "Browser session";
    case "wecom":
      return "Enterprise WeChat";
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
  if (
    supportedModes.includes("oidc") ||
    supportedModes.includes("ldap")
  ) {
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

export function settingsLocalAccessReadiness({
  directoryDepartments,
  directorySyncRuns,
  directoryUsers,
  rbacAssignments,
}: {
  directoryDepartments: readonly DirectoryDepartment[];
  directorySyncRuns?: readonly DirectorySyncRun[];
  directoryUsers: readonly DirectoryUser[];
  rbacAssignments: readonly RBACAssignment[];
}): SettingsLocalAccessReadiness {
  const activeUsers = directoryUsers.filter((user) => user.active).length;
  const enabledAssignments = rbacAssignments.filter(
    (assignment) => assignment.enabled,
  ).length;
  const syncItem = settingsDirectorySyncReadinessItem(directorySyncRuns);
  const items: SettingsLocalAccessReadinessItem[] = [
    ...(syncItem === null ? [] : [syncItem]),
    {
      detail:
        activeUsers > 0
          ? "Local IAM directory projection contains active operators for attribution, pickers, and user-scoped RBAC."
          : "Local IAM directory projection has no active users. Run directory sync before accepting diagnosis-room authorization readiness.",
      key: "directory-users",
      label: "Directory users",
      status: activeUsers > 0 ? "ready" : "blocked",
      value:
        activeUsers === directoryUsers.length
          ? `${activeUsers} active`
          : `${activeUsers}/${directoryUsers.length} active`,
    },
    {
      detail:
        directoryDepartments.length > 0
          ? "Local IAM department projection is available for department-scoped RBAC and collaboration context."
          : "No local department projection is available yet. User-scoped RBAC can still work, but department-scoped collaboration should be reviewed.",
      key: "directory-departments",
      label: "Departments",
      status: directoryDepartments.length > 0 ? "ready" : "review",
      value: `${directoryDepartments.length} departments`,
    },
    {
      detail:
        enabledAssignments > 0
          ? "OpenClarion has enabled local RBAC assignments for diagnosis-room authorization."
          : "No enabled local RBAC assignments exist. Bootstrap admins are only an emergency path; persist RBAC assignments before rollout.",
      key: "rbac-assignments",
      label: "RBAC rules",
      status: enabledAssignments > 0 ? "ready" : "blocked",
      value:
        enabledAssignments === rbacAssignments.length
          ? `${enabledAssignments} enabled`
          : `${enabledAssignments}/${rbacAssignments.length} enabled`,
    },
  ];
  const status = diagnosisAuthSummaryStatus(items);
  return {
    detail: settingsLocalAccessReadinessDetail(status, items),
    items,
    label: settingsLocalAccessReadinessLabel(status),
    status,
  };
}

function settingsDirectorySyncReadinessItem(
  directorySyncRuns?: readonly DirectorySyncRun[],
): SettingsLocalAccessReadinessItem | null {
  if (directorySyncRuns === undefined) {
    return null;
  }
  const latestRun = latestDirectorySyncRun(directorySyncRuns);
  if (latestRun === null) {
    return {
      detail:
        "No retained IAM directory sync run is available. Run a full sync before accepting local access readiness.",
      key: "directory-sync",
      label: "Directory sync",
      status: "review",
      value: "No runs",
    };
  }
  const syncedAt = formatDateTime(latestRun.synced_at);
  const status: string = latestRun.status;
  if (status === "succeeded") {
    return {
      detail:
        "Latest IAM directory sync succeeded, so local user and department projections have a retained sync record.",
      key: "directory-sync",
      label: "Directory sync",
      status: "ready",
      value: `Succeeded ${syncedAt}`,
    };
  }
  if (status === "failed") {
    const reason =
      latestRun.failure_code.trim() === ""
        ? "unknown failure"
        : latestRun.failure_code.trim();
    return {
      detail: `Latest IAM directory sync failed with ${reason}. Fix sync before accepting local access readiness.`,
      key: "directory-sync",
      label: "Directory sync",
      status: "blocked",
      value: `Failed ${syncedAt}`,
    };
  }
  return {
    detail:
      "Latest IAM directory sync has an unknown status. Review sync history before accepting local access readiness.",
    key: "directory-sync",
    label: "Directory sync",
    status: "review",
    value: `${status} ${syncedAt}`,
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
): string {
  return (
    readiness.items.find((item) => item.key === "directory-sync")?.value ??
    "Sync not loaded"
  );
}

function settingsLocalAccessReadinessDetail(
  status: ProofReadinessStatus,
  items: readonly SettingsLocalAccessReadinessItem[],
): string {
  if (status === "ready") {
    return "Local directory projection and RBAC assignments are ready for diagnosis-room authorization.";
  }
  const item =
    items.find((candidate) => candidate.status === status) ??
    items.find((candidate) => candidate.status !== "ready");
  return (
    item?.detail ?? "Review local directory projection and RBAC readiness."
  );
}

function settingsLocalAccessReadinessLabel(
  status: ProofReadinessStatus,
): string {
  switch (status) {
    case "ready":
      return "Local access ready";
    case "review":
      return "Local access review";
    case "pending":
      return "Local access pending";
    case "blocked":
      return "Local access blocked";
  }
}

function diagnosisLocalAccessSummaryValue(
  readiness: SettingsLocalAccessReadiness,
): string {
  const assignment = readiness.items.find(
    (item) => item.key === "rbac-assignments",
  );
  return (
    assignment?.value ?? settingsLocalAccessReadinessLabel(readiness.status)
  );
}

export function settingsCurrentRBACAccessSummary(
  state: CurrentRBACAuthorizationState,
  isChecking: boolean,
): SettingsCurrentRBACAccessSummary {
  if (isChecking || state.kind === "loading") {
    return {
      detail:
        "Checking the signed-in operator's local RBAC decisions through the browser session.",
      items: [],
      label: "Current access pending",
      needsSignIn: false,
      status: "pending",
      subject: "",
    };
  }
  if (state.kind === "error") {
    const needsSignIn = currentRBACAuthorizationNeedsSignIn(state);
    return {
      detail: needsSignIn
        ? "Sign in with IAM before checking local directory and RBAC permissions."
        : state.message,
      items: [],
      label: needsSignIn
        ? "Current access sign-in"
        : "Current access unavailable",
      needsSignIn,
      status: needsSignIn ? "blocked" : "review",
      subject: "",
    };
  }
  const activeDirectoryProfile = state.directoryUsers.some(
    (user) => user.active && user.subject === state.subject,
  );
  const accessItems = settingsCurrentRBACAccessItems(state);
  const items: SettingsCurrentRBACAccessItem[] = [
    {
      key: "directory-profile",
      label: "Profile",
      status: activeDirectoryProfile ? "ready" : "review",
      value: activeDirectoryProfile ? "Synced" : "Not synced",
    },
    ...accessItems,
  ];
  const deniedItems = accessItems.filter((item) => item.status !== "ready");
  if (deniedItems.length > 0) {
    return {
      detail: `Current subject is signed in, but ${deniedItems.map((item) => item.label).join(", ")} is denied by local RBAC.`,
      items,
      label: "Current access limited",
      needsSignIn: false,
      status: "review",
      subject: state.subject,
    };
  }
  if (!activeDirectoryProfile) {
    return {
      detail:
        "Current subject is authorized, but no active local directory profile is synced for attribution and department-scoped RBAC.",
      items,
      label: "Current access review",
      needsSignIn: false,
      status: "review",
      subject: state.subject,
    };
  }
  return {
    detail:
      "Current subject has the settings overview capabilities and an active local directory profile.",
    items,
    label: "Current access ready",
    needsSignIn: false,
    status: "ready",
    subject: state.subject,
  };
}

function settingsCurrentRBACAccessItems(
  state: Extract<CurrentRBACAuthorizationState, { kind: "ready" }>,
): SettingsCurrentRBACAccessItem[] {
  return settingsOverviewRBACAuthorizationChecks.map((check) => {
    const allowed = state.allowed[check.key] === true;
    return {
      key: check.key,
      label: settingsCurrentRBACAccessItemLabel(check.key),
      status: allowed ? "ready" : "blocked",
      value: allowed ? "Allowed" : "Denied",
    };
  });
}

function settingsCurrentRBACAccessItemLabel(key: string): string {
  switch (key) {
    case "directory-read":
      return "Directory read";
    case "directory-sync":
      return "Directory sync";
    case "rbac-manage":
      return "RBAC manage";
    case "operations-read":
      return "Operations read";
    default:
      return key;
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
  const hasPolicy = topology.policy !== null;
  const steps = workflowTopologySteps(topology);
  const firstUnready = steps.findIndex((step) => step.status !== "finish");
  const current = firstUnready === -1 ? steps.length : firstUnready;
  const actions = workflowTopologyActions(topology);

  return (
    <section
      aria-label="Active workflow topology"
      className="panel settings-overview-topology"
    >
      <div className="panel-header settings-overview-topology-header">
        <h2>Active workflow topology</h2>
        <Tag color={topologyStatusColor(topology.status)}>
          {topology.status}
        </Tag>
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
    <section
      aria-label="Next topology actions"
      className="settings-overview-action-list"
    >
      <div className="settings-overview-action-header">
        <Typography.Text strong>Next topology actions</Typography.Text>
        <Tag>{actions.length} open</Tag>
      </div>
      <div className="settings-overview-action-grid">
        {actions.map((action) => (
          <div className="settings-overview-action-item" key={action.key}>
            <div>
              <Space size={8} wrap>
                <Tag color={actionPriorityColor(action.priority)}>
                  {action.priority}
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
              Open
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
  const stages = alertIngestionStages(topology);
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
    autoDiagnosisProofHistory,
  );

  return (
    <section
      aria-label="Alert ingestion readiness"
      className="panel settings-overview-ingestion"
    >
      <div className="panel-header settings-overview-ingestion-header">
        <h2>Alert ingestion readiness</h2>
        <Tag color={topologyStatusColor(status)}>{status}</Tag>
      </div>
      <div className="panel-body settings-overview-ingestion-body">
        <div className="settings-overview-ingestion-path">
          <Steps
            aria-label="Alert to AI room configuration path"
            className="settings-overview-ingestion-steps"
            current={current}
            items={stages}
            responsive={false}
            size="small"
          />
        </div>
        <Row aria-label="Alert ingestion counters" gutter={[12, 12]}>
          <ReadinessCounter
            href="/settings/alert-sources"
            label="Intake source"
            status={intakeStatus}
            value={topology.alertSource?.kind ?? "missing"}
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
            label="Active alert tools"
            status={activeAlertTools > 0 ? "ready" : "review"}
            value={activeAlertTools}
          />
          <ReadinessCounter
            href={metricEvidenceConfigurationHref(topology)}
            label="Metric evidence"
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
            label="AI handoff"
            status={handoffStatus}
            value={topology.policy?.diagnosis_follow_up ?? "missing"}
          />
          <ReadinessCounter
            href={webhookProof.href}
            label="Webhook proof"
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
  const firstUnready = readiness.items.findIndex(
    (item) => item.status !== "ready",
  );
  const current = firstUnready === -1 ? readiness.items.length : firstUnready;

  return (
    <section
      aria-label="Live proof readiness"
      className="panel settings-overview-live-proof"
    >
      <div className="panel-header settings-overview-live-proof-header">
        <h2>Live proof readiness</h2>
        <Tag color={proofTargetStatusColor(readiness.status)}>
          {proofTargetStatusLabel(readiness.status)}
        </Tag>
      </div>
      <div className="panel-body settings-overview-live-proof-body">
        <Typography.Paragraph className="settings-overview-proof-copy">
          {readiness.detail}
        </Typography.Paragraph>
        <div className="settings-overview-live-proof-steps-wrap">
          <Steps
            aria-label="Replay to notification proof path"
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
                  {proofTargetStatusLabel(item.status)}
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
  const columns: TableColumnsType<AutoDiagnosisProofHistory> = [
    {
      key: "alert",
      render: (_, history) => (
        <Space direction="vertical" size={4}>
          <Typography.Text strong>{history.alertName}</Typography.Text>
          <Space size={[6, 6]} wrap>
            <Tag>alert #{history.alertID}</Tag>
            <Tag>snapshot #{history.snapshotID}</Tag>
          </Space>
        </Space>
      ),
      title: "Alert",
      width: 260,
    },
    {
      key: "coverage",
      render: (_, history) => (
        <Space direction="vertical" size={4}>
          <Tag color={proofTargetStatusColor(history.coverage.status)}>
            {history.coverage.label}
          </Tag>
          <Typography.Text type="secondary">
            {history.coverage.readyCount}/{history.coverage.requiredCount}{" "}
            phase(s)
          </Typography.Text>
        </Space>
      ),
      title: "Coverage",
      width: 170,
    },
    {
      key: "phases",
      render: (_, history) => (
        <Space size={[4, 4]} wrap>
          {history.coverage.phases.map((phase) => (
            <Tag
              color={diagnosisNotificationDeliveryCoveragePhaseColor(
                phase.status,
              )}
              key={phase.key}
            >
              {phase.label}: {phase.status}
            </Tag>
          ))}
        </Space>
      ),
      title: "AI Notification Phases",
    },
    {
      dataIndex: "occurredAt",
      key: "occurredAt",
      render: (value: string) => formatDateTime(value),
      title: "Latest",
      width: 190,
    },
    {
      key: "action",
      render: (_, history) => (
        <Button href={history.href} size="small" type="link">
          Review timeline
        </Button>
      ),
      title: "Action",
      width: 140,
    },
  ];

  return (
    <section
      aria-label="Recent auto-diagnosis proof history"
      className="panel settings-overview-auto-proof-history"
    >
      <div className="panel-header settings-overview-live-proof-header">
        <h2>Recent Auto-Diagnosis Proof</h2>
        <Tag color={histories.length > 0 ? "blue" : "default"}>
          {histories.length} retained
        </Tag>
      </div>
      <div className="panel-body settings-overview-live-proof-body">
        <Typography.Paragraph className="settings-overview-proof-copy">
          Recent AlertmanagerWebhookAutoDiagnosis rooms with retained Enterprise
          WeChat AI notification coverage.
        </Typography.Paragraph>
        <Table<AutoDiagnosisProofHistory>
          columns={columns}
          dataSource={histories}
          locale={{ emptyText: "No retained auto-diagnosis proof history yet" }}
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
  return (
    <Col lg={6} sm={12} xs={24}>
      <a className="settings-overview-ingestion-counter" href={href}>
        <span className="settings-overview-ingestion-value">{value}</span>
        <span className="settings-overview-ingestion-footer">
          <Typography.Text className="muted">{label}</Typography.Text>
          <Tag color={topologyStatusColor(status)}>{status}</Tag>
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
      title: "Intake",
      description:
        topology.alertSource === null
          ? "Missing source"
          : autoRoomBlocked
            ? "Auto room requires Alertmanager"
            : `${topology.alertSource.kind} source`,
      status:
        topology.alertSource?.enabled && !autoRoomBlocked ? "finish" : "error",
    },
    {
      title: "Alerts",
      description: `${activeAlertTools} active alert tools`,
      status: activeAlertTools > 0 ? "finish" : "wait",
    },
    {
      title: "Metrics",
      description: `${metricTools} metric tools`,
      status: metricTools > 0 ? "finish" : "wait",
    },
    {
      title: "AI room",
      description: topology.policy?.diagnosis_follow_up ?? "No policy",
      status: autoRoomReady ? "finish" : "wait",
    },
    {
      title: "Replay",
      description: autoRoomReady
        ? "Automatic diagnosis ready"
        : topology.policy?.enabled
          ? "Policy enabled"
          : "Policy draft",
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
): TopologyAction[] {
  const actions: TopologyAction[] = [];

  if (topology.policy === null) {
    return [
      {
        key: "create-policy",
        title: "Create automatic diagnosis workflow",
        detail:
          "Bind an Alertmanager source, grouping, and Enterprise WeChat delivery settings before replaying alert windows.",
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
      title: "Enable workflow policy",
      detail:
        "The selected workflow policy is still a draft and will not be used for replay or schedule proof.",
      href: "/settings/report-workflow-policies",
      priority: "high",
    });
  }
  if (topology.alertSource === null || !topology.alertSource.enabled) {
    actions.push({
      key: "alert-source",
      title: "Review alert source",
      detail:
        "The selected workflow needs an enabled alert source before alert windows can be replayed.",
      href: "/settings/alert-sources",
      priority: "high",
    });
  }
  if (topology.groupingPolicy === null || !topology.groupingPolicy.enabled) {
    actions.push({
      key: "grouping",
      title: "Review grouping policy",
      detail:
        "The selected workflow needs an enabled grouping policy to build incident groups.",
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
      title: "Enable AI room follow-up",
      detail:
        "Set diagnosis follow-up to auto room when webhook alerts should start the consultation workflow automatically.",
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
      title: "Use auto room for webhook handoff",
      detail:
        "Suggest room keeps an operator handoff step; auto room is required for Alertmanager webhook alerts to start AI consultation automatically.",
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
      title: "Bind an Alertmanager webhook source",
      detail:
        "Automatic diagnosis room starts require an enabled Alertmanager-compatible webhook source. Keep Thanos Rule for active-alert evidence and use Alertmanager for webhook intake.",
      href: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      priority: "high",
    });
  }
  if (topology.activeAlertTools.length === 0) {
    actions.push({
      key: "active-alert-tool",
      title: "Enable active alert tool",
      detail:
        "Enable an active_alerts template on the workflow alert source so the AI room can inspect current alerts.",
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
        title: "Enable metric evidence",
        detail:
          "Enable a Prometheus-compatible metric_query or metric_range_query template for evidence collection.",
        href: diagnosisToolTemplateLaunchHref({
          intent: "metric-evidence-tool",
          sourceID: metricSourceID,
        }),
        priority: "medium",
      });
    } else if (topology.metricSources.length > 0) {
      actions.push({
        key: "enable-prometheus-metric-source",
        title: "Enable Prometheus metric source",
        detail:
          "A Prometheus-compatible source exists as a draft; enable and test it before adding metric evidence tools.",
        href: "/settings/alert-sources",
        priority: "medium",
      });
    } else {
      actions.push({
        key: "thanos-metric-source",
        title: "Create Thanos metric source",
        detail:
          "Metric evidence needs an enabled Thanos Query or Prometheus-compatible source before range or instant queries can be configured.",
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
      title: "Enable AI delivery channel",
      detail:
        "The selected notification channel is bound but disabled, so AI update and report delivery proof cannot complete.",
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
      title: "Bind Enterprise WeChat AI delivery",
      detail:
        "Auto room proof requires an enabled Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes.",
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
      title: "Review AI delivery channel scopes",
      detail: `The selected notification channel is missing ${missingDeliveryScopes.join(", ")} scope before this workflow can deliver AI updates, close notifications, and reports.`,
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
      title: "Switch AI delivery to Enterprise WeChat",
      detail:
        "Auto room proof requires an Enterprise WeChat channel; generic webhook delivery cannot start the operator handoff path.",
      href: notificationChannelAutoRoomLaunchHref(topology),
      priority: "high",
    });
  }
  if (topology.activeSchedule === null) {
    actions.push({
      key: "create-schedule",
      title: "Create workflow schedule",
      detail:
        "Add a schedule when the workflow needs retained scheduled proof instead of manual replay only.",
      href: reportWorkflowScheduleLaunchHref({
        intent: "create-schedule",
        policyID: topology.policy.id,
      }),
      priority: "low",
    });
  } else if (!topology.activeSchedule.enabled) {
    actions.push({
      key: "enable-schedule",
      title: "Enable workflow schedule",
      detail:
        "The schedule exists as a draft; enable it after replay proof is accepted.",
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
      title: "Run impact preview",
      detail:
        "Preview recent alert grouping before replaying a workflow window.",
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
  autoDiagnosisProofHistory: AutoDiagnosisProofHistory | null = null,
): ProofTarget[] {
  const replayTarget = workflowReplayProofTarget(topology);
  const autoDiagnosisTarget = workflowAlertmanagerAutoDiagnosisProofTarget(
    topology,
    autoDiagnosisProofHistory,
  );
  const scheduleTarget = workflowScheduleProofTarget(topology);
  return [replayTarget, autoDiagnosisTarget, scheduleTarget];
}

export function alertIngestionWebhookProofReadiness(
  topology: WorkflowTopology,
  proofHistory: AutoDiagnosisProofHistory | null = null,
): AlertIngestionWebhookProofReadiness {
  const target = workflowAlertmanagerAutoDiagnosisProofTarget(
    topology,
    proofHistory,
  );
  if (proofHistory === null) {
    return {
      href: target.actionHref,
      status: target.status === "blocked" ? "blocked" : "review",
      value: "missing",
    };
  }
  return {
    href: target.actionHref,
    status: proofReadinessStatusForTopology(target.status),
    value: proofHistory.coverage.label,
  };
}

export function latestAutoDiagnosisProofHistory(
  alerts: AlertEventSummary[],
): AutoDiagnosisProofHistory | null {
  return autoDiagnosisProofHistoriesForAlerts(alerts, 1)[0] ?? null;
}

export function latestAutoDiagnosisProofHistoryForSource(
  alerts: AlertEventSummary[],
  alertSourceProfileID: number | null,
): AutoDiagnosisProofHistory | null {
  if (alertSourceProfileID === null || alertSourceProfileID <= 0) {
    return null;
  }
  return (
    autoDiagnosisProofHistoriesForAlerts(alerts, Number.MAX_SAFE_INTEGER).find(
      (history) => history.alertSourceProfileID === alertSourceProfileID,
    ) ?? null
  );
}

export function autoDiagnosisProofHistoriesForAlerts(
  alerts: AlertEventSummary[],
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
          alertName: alertProofDisplayName(alert),
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

function alertProofDisplayName(alert: AlertEventSummary): string {
  return (
    alert.labels.alertname?.trim() ||
    alert.annotations.summary?.trim() ||
    `alert #${alert.id}`
  );
}

function timestampSortValue(value: string): number {
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

export function workflowLiveProofReadiness(
  topology: WorkflowTopology,
  diagnosisAuthProof: DiagnosisAuthLiveProof = defaultDiagnosisAuthLiveProof(),
): LiveProofReadiness {
  const replayTarget = workflowReplayProofTarget(topology);
  const scheduleTarget = workflowScheduleProofTarget(topology);
  const items: LiveProofReadinessItem[] = [
    {
      detail: replayTarget.detail,
      key: "policy-replay",
      status: replayTarget.status,
      title: "Replay",
      value: replayTarget.actionLabel,
    },
    workflowAIDiagnosisProofItem(topology),
    workflowDiagnosisAuthProofItem(diagnosisAuthProof),
    workflowNotificationProofItem(topology),
    {
      detail: scheduleTarget.detail,
      key: "scheduled-trigger",
      status: scheduleTarget.status,
      title: "Schedule",
      value: scheduleTarget.actionLabel,
    },
  ];
  const status = aggregateProofReadiness(items.map((item) => item.status));
  return {
    detail: liveProofReadinessDetail(status),
    items,
    status,
  };
}

export function workflowIntegrationReadiness(
  topology: WorkflowTopology,
  t: DiagnosisAuthTranslator,
  diagnosisAuthProof: DiagnosisAuthLiveProof = defaultDiagnosisAuthLiveProof(),
  localAccessReadiness?: SettingsLocalAccessReadiness,
  currentRBACAccess?: SettingsCurrentRBACAccessSummary,
): IntegrationReadiness {
  const items: IntegrationReadinessItem[] = [
    workflowOperatorAuthReadinessItem(diagnosisAuthProof, t),
    ...(localAccessReadiness === undefined
      ? []
      : [
          workflowLocalAccessReadinessItem(
            localAccessReadiness,
            currentRBACAccess,
          ),
        ]),
    workflowEnterpriseWeChatReadinessItem(topology),
    workflowAlertmanagerAutoRoomReadinessItem(topology),
  ];
  const status = aggregateProofReadiness(items.map((item) => item.status));
  return {
    detail: integrationReadinessDetail(status),
    items,
    status,
  };
}

function workflowLocalAccessReadinessItem(
  readiness: SettingsLocalAccessReadiness,
  currentRBACAccess?: SettingsCurrentRBACAccessSummary,
): IntegrationReadinessItem {
  if (currentRBACAccess === undefined) {
    return {
      actionHref: "/settings/directory-rbac",
      actionLabel:
        readiness.status === "ready" ? "Review access" : "Fix access",
      detail: readiness.detail,
      key: "local-access",
      status: readiness.status,
      title: "Local access",
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
      ? "Local directory projection, RBAC assignments, and current operator access are ready for diagnosis-room authorization."
      : readiness.detail;
  return {
    actionHref: currentRBACAccess.needsSignIn
      ? settingsDirectoryRBACLoginHref
      : "/settings/directory-rbac",
    actionLabel: currentRBACAccess.needsSignIn
      ? "Sign in"
      : status === "ready"
        ? "Review access"
        : "Fix access",
    detail,
    key: "local-access",
    status,
    title: "Local access",
    value:
      currentRBACAccess.subject === ""
        ? currentRBACAccess.label
        : currentRBACAccess.subject,
  };
}

function workflowOperatorAuthReadinessItem(
  proof: DiagnosisAuthLiveProof,
  t: DiagnosisAuthTranslator,
): IntegrationReadinessItem {
  const actionHref = "#diagnosis-auth-readiness";
  const readiness = diagnosisAuthRolloutReadiness(proof, t);
  if (readiness.status === "ready") {
    return {
      actionHref,
      actionLabel: "Review auth",
      detail:
        readiness.detail ||
        "Operator identity is verified against the running backend. Diagnosis room permissions are enforced by OpenClarion local RBAC.",
      key: "operator-auth",
      status: "ready",
      title: "Operator auth",
      value:
        readiness.subject === ""
          ? `${authProbeModeValueLabel(readiness.mode)} verified`
          : readiness.subject,
    };
  }
  if (readiness.status === "review") {
    return {
      actionHref,
      actionLabel: "Check SSO",
      detail: readiness.detail,
      key: "operator-auth",
      status: "review",
      title: "Operator auth",
      value: `${authProbeModeValueLabel(readiness.mode)} verified`,
    };
  }
  if (readiness.status === "blocked") {
    return {
      actionHref,
      actionLabel: "Fix auth",
      detail: readiness.detail,
      key: "operator-auth",
      status: "blocked",
      title: "Operator auth",
      value: "Blocked",
    };
  }
  return {
    actionHref,
    actionLabel: "Check auth",
    detail: readiness.detail,
    key: "operator-auth",
    status: "pending",
    title: "Operator auth",
    value: "Not checked",
  };
}

function workflowEnterpriseWeChatReadinessItem(
  topology: WorkflowTopology,
): IntegrationReadinessItem {
  const actionHref = notificationChannelAutoRoomLaunchHref(topology);
  if (topology.policy === null) {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "create-auto-room-policy",
      }),
      actionLabel: "Create policy",
      detail:
        "Create an auto-room workflow policy before binding Enterprise WeChat delivery.",
      key: "enterprise-wechat",
      status: "blocked",
      title: "Enterprise WeChat",
      value: "No policy",
    };
  }
  if (topology.notificationChannel === null) {
    return {
      actionHref,
      actionLabel: "Bind channel",
      detail:
        "Bind an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes.",
      key: "enterprise-wechat",
      status: "blocked",
      title: "Enterprise WeChat",
      value: "No channel",
    };
  }
  if (!topology.notificationChannel.enabled) {
    return {
      actionHref: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      actionLabel: "Enable channel",
      detail:
        "Enable the bound Enterprise WeChat channel before live AI notification proof.",
      key: "enterprise-wechat",
      status: "blocked",
      title: "Enterprise WeChat",
      value: topology.notificationChannel.name,
    };
  }
  if (topology.notificationChannel.kind !== "wecom") {
    return {
      actionHref,
      actionLabel: "Switch channel",
      detail:
        "Automatic diagnosis rollout requires Enterprise WeChat; generic webhook proof is not enough.",
      key: "enterprise-wechat",
      status: "blocked",
      title: "Enterprise WeChat",
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
      actionLabel: "Review scopes",
      detail: `Add ${missingScopes.join(", ")} scope before accepting Enterprise WeChat rollout.`,
      key: "enterprise-wechat",
      status: "blocked",
      title: "Enterprise WeChat",
      value: topology.notificationChannel.name,
    };
  }
  if (topology.policy.diagnosis_follow_up !== "auto_room") {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "auto-room-follow-up",
        sourceID: topology.alertSource?.id ?? null,
      }),
      actionLabel: "Use auto room",
      detail:
        "Enterprise WeChat is bound, but the workflow still needs auto_room follow-up for automatic AI consultation.",
      key: "enterprise-wechat",
      status: "review",
      title: "Enterprise WeChat",
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
      actionLabel: "Test channel",
      detail: rolloutReadiness.detail,
      key: "enterprise-wechat",
      status: rolloutReadiness.status === "blocked" ? "blocked" : "review",
      title: "Enterprise WeChat",
      value: topology.notificationChannel.name,
    };
  }
  return {
    actionHref: notificationChannelAutoRoomEditHref(
      topology,
      topology.notificationChannel.id,
    ),
    actionLabel: "Review channel",
    detail:
      "Enterprise WeChat has the required scopes and AI diagnosis plus close notification sample proof.",
    key: "enterprise-wechat",
    status: "ready",
    title: "Enterprise WeChat",
    value: topology.notificationChannel.name,
  };
}

function workflowAlertmanagerAutoRoomReadinessItem(
  topology: WorkflowTopology,
): IntegrationReadinessItem {
  if (topology.policy === null) {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "create-auto-room-policy",
      }),
      actionLabel: "Create policy",
      detail:
        "Create an auto-room workflow policy before Alertmanager can start AI diagnosis.",
      key: "alertmanager-auto-room",
      status: "blocked",
      title: "Alertmanager auto-room",
      value: "No policy",
    };
  }
  if (!topology.policy.enabled) {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: "Enable policy",
      detail:
        "Enable the workflow policy before Alertmanager webhook deliveries can start AI rooms.",
      key: "alertmanager-auto-room",
      status: "blocked",
      title: "Alertmanager auto-room",
      value: topology.policy.name,
    };
  }
  if (topology.policy.diagnosis_follow_up !== "auto_room") {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "auto-room-follow-up",
        sourceID: topology.alertSource?.id ?? null,
      }),
      actionLabel: "Use auto room",
      detail:
        "Switch diagnosis follow-up to auto_room so Alertmanager webhook alerts start AI consultation automatically.",
      key: "alertmanager-auto-room",
      status: "review",
      title: "Alertmanager auto-room",
      value: topology.policy.diagnosis_follow_up,
    };
  }
  if (topology.alertSource === null || !topology.alertSource.enabled) {
    return {
      actionHref: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      actionLabel: "Review source",
      detail:
        "Bind an enabled Alertmanager-compatible source before webhook alerts can start AI diagnosis.",
      key: "alertmanager-auto-room",
      status: "blocked",
      title: "Alertmanager auto-room",
      value: "No source",
    };
  }
  if (topology.alertSource.kind !== "alertmanager") {
    return {
      actionHref: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      actionLabel: "Switch source",
      detail:
        "Auto-room webhook handoff requires an Alertmanager-compatible source, not a Prometheus metric source.",
      key: "alertmanager-auto-room",
      status: "blocked",
      title: "Alertmanager auto-room",
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
      actionLabel: "Review tools",
      detail:
        "Add active alert and metric evidence tools before accepting automatic AI diagnosis rollout.",
      key: "alertmanager-auto-room",
      status: "review",
      title: "Alertmanager auto-room",
      value: `${topology.activeAlertTools.length} alert / ${topology.metricTools.length} metric`,
    };
  }
  return {
    actionHref: "/settings/report-workflow-policies",
    actionLabel: "Review workflow",
    detail:
      "Enabled Alertmanager auto-room policy has alert and metric evidence tools ready for assistant-message plus final-conclusion proof.",
    key: "alertmanager-auto-room",
    status: "ready",
    title: "Alertmanager auto-room",
    value: topology.alertSource.name,
  };
}

function integrationReadinessDetail(status: ProofReadinessStatus): string {
  switch (status) {
    case "ready":
      return "Operator auth, Enterprise WeChat delivery, and Alertmanager auto-room handoff are ready for assistant-message plus final-conclusion live proof.";
    case "review":
      return "Core wiring is present, but at least one production rollout item still needs review before multi-step AI notification proof.";
    case "pending":
      return "Configuration is not blocked, but at least one rollout item still needs a live check.";
    case "blocked":
      return "Resolve blocked operator auth, Enterprise WeChat, or Alertmanager auto-room prerequisites before live proof.";
  }
}

function workflowDiagnosisAuthProofItem(
  proof: DiagnosisAuthLiveProof,
): LiveProofReadinessItem {
  const modeLabel = authProbeModeLabel(proof.mode);
  const modeValue = authProbeModeValueLabel(proof.mode);
  switch (proof.status) {
    case "verified":
      return {
        detail: proof.detail || `Backend diagnosis auth accepted ${modeLabel}.`,
        key: "diagnosis-auth",
        status: "ready",
        title: "Auth",
        value: proof.subject === "" ? `${modeValue} verified` : proof.subject,
      };
    case "failed":
    case "blocked":
      return {
        detail:
          proof.detail ||
          `Resolve ${modeLabel} diagnosis auth before live proof.`,
        key: "diagnosis-auth",
        status: "blocked",
        title: "Auth",
        value: `${modeValue} failed`,
      };
    case "checking":
      return {
        detail: "Backend diagnosis auth check is still running.",
        key: "diagnosis-auth",
        status: "pending",
        title: "Auth",
        value: `${modeValue} checking`,
      };
    case "needs_check":
    case "pending":
      return {
        detail:
          proof.detail ||
          `Run Check auth with ${modeLabel} before accepting live proof.`,
        key: "diagnosis-auth",
        status: "pending",
        title: "Auth",
        value: `${modeValue} not checked`,
      };
  }
  return {
    detail: `Run Check auth with ${modeLabel} before accepting live proof.`,
    key: "diagnosis-auth",
    status: "pending",
    title: "Auth",
    value: `${modeValue} not checked`,
  };
}

function workflowAIDiagnosisProofItem(
  topology: WorkflowTopology,
): LiveProofReadinessItem {
  if (topology.policy === null) {
    return {
      detail:
        "Create and enable an auto-room workflow policy before AI diagnosis proof can run.",
      key: "ai-diagnosis",
      status: "blocked",
      title: "AI diagnosis",
      value: "No policy",
    };
  }
  if (topology.status === "blocked") {
    return {
      detail:
        "Resolve blocked workflow topology before AI diagnosis proof can run.",
      key: "ai-diagnosis",
      status: "blocked",
      title: "AI diagnosis",
      value: topology.policy.diagnosis_follow_up,
    };
  }
  if (topology.policy.diagnosis_follow_up === "disabled") {
    return {
      detail:
        "Enable auto_room follow-up before replay can start AI diagnosis rooms.",
      key: "ai-diagnosis",
      status: "blocked",
      title: "AI diagnosis",
      value: "Disabled",
    };
  }
  if (topology.policy.diagnosis_follow_up === "suggest_room") {
    return {
      detail:
        "Suggest room keeps an operator handoff; switch to auto_room for replay-driven AI diagnosis proof.",
      key: "ai-diagnosis",
      status: "review",
      title: "AI diagnosis",
      value: "Operator handoff",
    };
  }
  if (
    topology.activeAlertTools.length === 0 ||
    topology.metricTools.length === 0
  ) {
    return {
      detail:
        "AI diagnosis proof needs active alert and metric evidence tools before replay.",
      key: "ai-diagnosis",
      status: "review",
      title: "AI diagnosis",
      value: `${topology.activeAlertTools.length} alert / ${topology.metricTools.length} metric`,
    };
  }
  return {
    detail:
      "Auto-room replay can start AI diagnosis with alert and metric evidence tools.",
    key: "ai-diagnosis",
    status: "ready",
    title: "AI diagnosis",
    value: "Auto room",
  };
}

function workflowNotificationProofItem(
  topology: WorkflowTopology,
): LiveProofReadinessItem {
  if (topology.policy === null) {
    return {
      detail:
        "Create a workflow policy before notification proof can be evaluated.",
      key: "notification",
      status: "blocked",
      title: "Notification",
      value: "No policy",
    };
  }
  if (topology.notificationChannel === null) {
    return {
      detail:
        topology.policy.diagnosis_follow_up === "auto_room"
          ? "Bind an Enterprise WeChat channel before accepting AI update notification proof."
          : "Bind a report notification channel before accepting delivery proof.",
      key: "notification",
      status:
        topology.policy.diagnosis_follow_up === "auto_room"
          ? "blocked"
          : "pending",
      title: "Notification",
      value: "No channel",
    };
  }
  const missingScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  if (!topology.notificationChannel.enabled) {
    return {
      detail:
        "Enable the bound notification channel before live notification proof.",
      key: "notification",
      status: "blocked",
      title: "Notification",
      value: topology.notificationChannel.name,
    };
  }
  if (missingScopes.length > 0) {
    return {
      detail: `Add ${missingScopes.join(", ")} scope before live AI notification proof.`,
      key: "notification",
      status: "blocked",
      title: "Notification",
      value: topology.notificationChannel.name,
    };
  }
  if (
    autoRoomChannelKindBlocked(topology.policy, topology.notificationChannel)
  ) {
    return {
      detail:
        "Switch the bound notification channel to Enterprise WeChat before attempting operator handoff proof.",
      key: "notification",
      status: "blocked",
      title: "Notification",
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
        detail: proofReadiness.detail,
        key: "notification",
        status: proofReadiness.status === "blocked" ? "blocked" : "review",
        title: "Notification",
        value: topology.notificationChannel.name,
      };
    }
  }
  return {
    detail:
      topology.policy.diagnosis_follow_up === "auto_room"
        ? "Enterprise WeChat channel scopes are ready for report, AI update, and close proof."
        : "Notification channel scopes are ready for report delivery proof.",
    key: "notification",
    status: "ready",
    title: "Notification",
    value: topology.notificationChannel.name,
  };
}

function workflowAlertmanagerAutoDiagnosisProofTarget(
  topology: WorkflowTopology,
  proofHistory: AutoDiagnosisProofHistory | null,
): ProofTarget {
  const evidence = [
    "Alertmanager webhook",
    "auto_room",
    "LLM",
    "Assistant message",
    "Final conclusion",
    "Enterprise WeChat",
  ];
  if (topology.policy === null) {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "create-auto-room-policy",
      }),
      actionLabel: "Create Policy",
      detail:
        "Create and enable an auto-room workflow policy before retaining Alertmanager auto-diagnosis proof.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: "Alertmanager auto-diagnosis",
    };
  }
  if (!topology.policy.enabled) {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: "Enable Policy",
      detail:
        "Enable the auto-room workflow policy before Alertmanager webhook proof can start AI diagnosis.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: "Alertmanager auto-diagnosis",
    };
  }
  if (topology.policy.diagnosis_follow_up !== "auto_room") {
    return {
      actionHref: reportWorkflowPolicyLaunchHref({
        intent: "auto-room-follow-up",
        sourceID: topology.alertSource?.id ?? null,
      }),
      actionLabel: "Use Auto Room",
      detail:
        "Switch diagnosis follow-up to auto_room before proving Alertmanager webhook to AI consultation delivery.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status:
        topology.policy.diagnosis_follow_up === "disabled"
          ? "blocked"
          : "review",
      title: "Alertmanager auto-diagnosis",
    };
  }
  if (topology.alertSource === null || !topology.alertSource.enabled) {
    return {
      actionHref: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      actionLabel: "Review Source",
      detail:
        "Bind an enabled Alertmanager-compatible source before webhook proof can start automatic AI diagnosis.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: "Alertmanager auto-diagnosis",
    };
  }
  if (topology.alertSource.kind !== "alertmanager") {
    return {
      actionHref: alertSourceLaunchHref({ intent: "alertmanager-source" }),
      actionLabel: "Switch Source",
      detail:
        "Automatic diagnosis proof starts from an Alertmanager-compatible webhook source, not a Prometheus metric source.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: "Alertmanager auto-diagnosis",
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
      actionLabel: "Review Tools",
      detail:
        "Add active alert and metric evidence tools before proving automatic AI diagnosis.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "review",
      title: "Alertmanager auto-diagnosis",
    };
  }
  if (topology.notificationChannel === null) {
    return {
      actionHref: notificationChannelAutoRoomLaunchHref(topology),
      actionLabel: "Bind Channel",
      detail:
        "Bind an Enterprise WeChat channel before proving AI diagnosis update and final conclusion delivery.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: "Alertmanager auto-diagnosis",
    };
  }
  if (!topology.notificationChannel.enabled) {
    return {
      actionHref: notificationChannelAutoRoomEditHref(
        topology,
        topology.notificationChannel.id,
      ),
      actionLabel: "Enable Channel",
      detail:
        "Enable the Enterprise WeChat channel before retaining auto-diagnosis notification proof.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: "Alertmanager auto-diagnosis",
    };
  }
  if (topology.notificationChannel.kind !== "wecom") {
    return {
      actionHref: notificationChannelAutoRoomLaunchHref(topology),
      actionLabel: "Switch Channel",
      detail:
        "Automatic AI diagnosis proof requires Enterprise WeChat, because generic webhook delivery does not prove operator handoff.",
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: "Alertmanager auto-diagnosis",
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
      actionLabel: "Review Scopes",
      detail: `Add ${missingScopes.join(", ")} scope before retaining automatic AI notification proof.`,
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: "blocked",
      title: "Alertmanager auto-diagnosis",
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
      actionLabel: "Test Channel",
      detail: rolloutReadiness.detail,
      evidence,
      key: "alertmanager-auto-diagnosis",
      status: rolloutReadiness.status === "blocked" ? "blocked" : "review",
      title: "Alertmanager auto-diagnosis",
    };
  }
  if (proofHistory !== null) {
    return workflowAlertmanagerAutoDiagnosisHistoryTarget(
      proofHistory,
      evidence,
    );
  }
  return {
    actionHref: reportWorkflowPolicyLaunchHref({
      intent: "alertmanager-auto-diagnosis-proof",
      sourceID: topology.alertSource.id,
    }),
    actionLabel: "Open Proof Path",
    detail:
      "Inputs are ready for retained Alertmanager webhook proof. Open the matching auto-room workflow, run bounded replay for retained proof, and use the live webhook smoke target for real Alertmanager ingress proof.",
    evidence,
    key: "alertmanager-auto-diagnosis",
    status: "ready",
    title: "Alertmanager auto-diagnosis",
  };
}

function workflowAlertmanagerAutoDiagnosisHistoryTarget(
  proofHistory: AutoDiagnosisProofHistory,
  evidence: string[],
): ProofTarget {
  const status = autoDiagnosisProofHistoryTargetStatus(proofHistory.coverage);
  return {
    actionHref: proofHistory.href,
    actionLabel:
      proofHistory.coverage.status === "ready"
        ? "Review Proof"
        : "Review Delivery",
    detail: `Latest retained auto-diagnosis proof for ${proofHistory.alertName} in room ${proofHistory.roomSessionID}: ${proofHistory.coverage.detail}`,
    evidence: [
      ...evidence,
      `alert #${proofHistory.alertID}`,
      `snapshot #${proofHistory.snapshotID}`,
      proofHistory.coverage.label,
    ],
    key: "alertmanager-auto-diagnosis",
    status,
    title: "Alertmanager auto-diagnosis",
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

function workflowReplayProofTarget(topology: WorkflowTopology): ProofTarget {
  const evidence = [
    "PostgreSQL",
    "Temporal",
    "Alert source",
    "Tool template",
    "LLM",
    "Notification",
  ];
  if (topology.policy === null) {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: "Create Policy",
      detail:
        "Create and enable a workflow policy before retaining manual replay proof.",
      evidence,
      key: "policy-replay",
      status: "blocked",
      title: "Policy replay",
    };
  }
  if (topology.status === "blocked") {
    const action = workflowTopologyActions(topology)[0] ?? null;
    return {
      actionHref: action?.href ?? "/settings/report-workflow-policies",
      actionLabel: action === null ? "Open Workflow" : "Resolve",
      detail:
        action?.detail ??
        "Resolve blocked workflow bindings before replay proof can run.",
      evidence,
      key: "policy-replay",
      status: "blocked",
      title: "Policy replay",
    };
  }
  if (topology.status === "review") {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: "Review Replay",
      detail:
        "Review workflow warnings, run impact preview, then retain a bounded policy replay proof.",
      evidence,
      key: "policy-replay",
      status: "review",
      title: "Policy replay",
    };
  }
  return {
    actionHref: "/settings/report-workflow-policies",
    actionLabel: "Run Replay",
    detail:
      "Workflow bindings are ready for impact preview and retained policy replay proof.",
    evidence,
    key: "policy-replay",
    status: "ready",
    title: "Policy replay",
  };
}

function workflowScheduleProofTarget(topology: WorkflowTopology): ProofTarget {
  const evidence = [
    "Enabled schedule",
    "Temporal action",
    "Launcher workflow",
    "Report delivery",
  ];
  if (topology.policy === null || topology.status === "blocked") {
    const action = workflowTopologyActions(topology)[0] ?? null;
    return {
      actionHref: action?.href ?? "/settings/report-workflow-policies",
      actionLabel: action === null ? "Open Workflow" : "Resolve",
      detail:
        action?.detail ??
        "Resolve workflow blockers before scheduled-trigger proof can run.",
      evidence,
      key: "scheduled-trigger",
      status: "blocked",
      title: "Scheduled trigger",
    };
  }
  if (topology.activeSchedule === null) {
    return {
      actionHref: reportWorkflowScheduleLaunchHref({
        intent: "create-schedule",
        policyID: topology.policy.id,
      }),
      actionLabel: "Create Schedule",
      detail:
        "Create a report workflow schedule after policy replay proof is accepted.",
      evidence,
      key: "scheduled-trigger",
      status: "pending",
      title: "Scheduled trigger",
    };
  }
  if (!topology.activeSchedule.enabled) {
    return {
      actionHref: "/settings/report-workflow-schedules",
      actionLabel: "Enable Schedule",
      detail:
        "The selected schedule is still a draft; enable it after replay proof is accepted.",
      evidence,
      key: "scheduled-trigger",
      status: "review",
      title: "Scheduled trigger",
    };
  }
  if (topology.status === "review") {
    return {
      actionHref: "/settings/report-workflow-schedules",
      actionLabel: "Review Schedule",
      detail:
        "Schedule metadata is enabled, but workflow review items remain before retained scheduled proof.",
      evidence,
      key: "scheduled-trigger",
      status: "review",
      title: "Scheduled trigger",
    };
  }
  return {
    actionHref: "/settings/report-workflow-schedules",
    actionLabel: "Review Schedule",
    detail:
      "An enabled schedule is ready for retained scheduled-trigger proof.",
    evidence,
    key: "scheduled-trigger",
    status: "ready",
    title: "Scheduled trigger",
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
): WorkflowTopologyStep[] {
  const missingDeliveryScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  return [
    {
      title: "Source",
      description:
        topology.alertSource === null
          ? "Missing source"
          : autoRoomWebhookBlocked(topology.policy, topology.alertSource)
            ? `${topology.alertSource.name} is not Alertmanager`
            : topology.alertSource.name,
      status:
        topology.alertSource?.enabled &&
        !autoRoomWebhookBlocked(topology.policy, topology.alertSource)
          ? "finish"
          : "error",
    },
    {
      title: "Grouping",
      description: topology.groupingPolicy?.name ?? "Missing grouping",
      status: topology.groupingPolicy?.enabled ? "finish" : "error",
    },
    {
      title: "Evidence",
      description: `${topology.activeAlertTools.length} active / ${topology.metricTools.length} metric`,
      status:
        topology.activeAlertTools.length > 0 && topology.metricTools.length > 0
          ? "finish"
          : "wait",
    },
    {
      title: "AI room",
      description: topology.policy?.diagnosis_follow_up ?? "No policy",
      status: autoRoomWebhookReady(topology.policy, topology.alertSource)
        ? "finish"
        : "wait",
    },
    {
      title: "Delivery",
      description:
        topology.notificationChannel === null
          ? topology.policy?.diagnosis_follow_up === "auto_room"
            ? "No Enterprise WeChat channel"
            : "No report channel"
          : missingDeliveryScopes.length > 0
            ? `${topology.notificationChannel.name} missing ${missingDeliveryScopes.join(", ")}`
            : autoRoomChannelKindBlocked(
                  topology.policy,
                  topology.notificationChannel,
                )
              ? `${topology.notificationChannel.name} is not WeCom`
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
      title: "Schedule",
      description: topology.activeSchedule?.name ?? "Manual replay",
      status: topology.activeSchedule?.enabled ? "finish" : "wait",
    },
  ];
}

function workflowTopologyDescriptions(topology: WorkflowTopology) {
  const missingDeliveryScopes = requiredDeliveryScopesMissing(
    topology.policy,
    topology.notificationChannel,
  );
  return [
    {
      key: "policy",
      label: "Policy",
      children: topology.policy === null ? "None" : topology.policy.name,
    },
    {
      key: "source",
      label: "Alert source",
      children:
        topology.alertSource === null
          ? "Missing"
          : configLabel(
              topology.alertSource.name,
              topology.alertSource.enabled,
            ),
    },
    {
      key: "grouping",
      label: "Grouping",
      children:
        topology.groupingPolicy === null
          ? "Missing"
          : configLabel(
              topology.groupingPolicy.name,
              topology.groupingPolicy.enabled,
            ),
    },
    {
      key: "tools",
      label: "Evidence tools",
      children:
        topology.diagnosisTools.length === 0 ? (
          <Typography.Text type="secondary">
            No enabled tools for selected source
          </Typography.Text>
        ) : (
          <Space direction="vertical" size={4}>
            <Space wrap>
              <Tag
                color={
                  topology.activeAlertTools.length > 0 ? "blue" : "default"
                }
              >
                Active alerts {topology.activeAlertTools.length}
              </Tag>
              <Tag color={topology.metricTools.length > 0 ? "cyan" : "default"}>
                Metric evidence {topology.metricTools.length}
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
      label: "Delivery channel",
      children:
        topology.notificationChannel === null
          ? topology.policy?.diagnosis_follow_up === "auto_room"
            ? "Missing Enterprise WeChat channel"
            : "Optional"
          : configLabel(
              missingDeliveryScopes.length > 0
                ? `${topology.notificationChannel.name} missing ${missingDeliveryScopes.join(", ")}`
                : autoRoomChannelKindBlocked(
                      topology.policy,
                      topology.notificationChannel,
                    )
                  ? `${topology.notificationChannel.name} is not WeCom`
                  : topology.notificationChannel.name,
              topology.notificationChannel.enabled &&
                missingDeliveryScopes.length === 0 &&
                !autoRoomChannelKindBlocked(
                  topology.policy,
                  topology.notificationChannel,
                ),
            ),
    },
    {
      key: "schedule",
      label: "Schedule",
      children:
        topology.activeSchedule === null
          ? "Manual replay"
          : configLabel(
              topology.activeSchedule.name,
              topology.activeSchedule.enabled,
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
): string[] {
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

function configLabel(name: string, enabled: boolean) {
  return (
    <Space size={6} wrap>
      <Typography.Text>{name}</Typography.Text>
      <Tag color={enabled ? "green" : "default"}>
        {enabled ? "Enabled" : "Draft"}
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

function proofTargetStatusLabel(status: ProofTarget["status"]): string {
  switch (status) {
    case "ready":
      return "Ready to run";
    case "review":
      return "Review";
    case "pending":
      return "Pending";
    case "blocked":
      return "Blocked";
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

function liveProofReadinessDetail(status: ProofReadinessStatus): string {
  switch (status) {
    case "ready":
      return "Configuration is ready to run replay, AI diagnosis, auth, notification, and schedule proof.";
    case "review":
      return "Configuration can run part of the proof path, but AI handoff, auth, or notification proof still needs review.";
    case "pending":
      return "Configuration has no blockers, but at least one proof target still needs to be created or run.";
    case "blocked":
      return "Resolve blocked workflow bindings before attempting retained live proof.";
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
  if (count === null) {
    return <Tag color="red">Unavailable</Tag>;
  }
  if (count === 0) {
    return <Tag color="gold">Needs setup</Tag>;
  }
  return <Tag color="green">Ready</Tag>;
}
