"use client";

import {
  BranchesOutlined,
  EditOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  RadarChartOutlined,
  ReloadOutlined,
  SaveOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Segmented,
  Select,
  Space,
  Statistic,
  Steps,
  Table,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import type { FormInstance, TableColumnsType } from "antd";
import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";

import {
  localizeReportReplayProofTrace,
  type LocalizedAlertReplayProofTrace,
} from "@/features/alerts/replay-copy";
import { autoDiagnosisConfirmedSnapshotCount } from "@/features/report-replay/replay-response";
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
  useSettingsMutation,
} from "../query-state";
import { ReadOnlyModeAlert } from "../permission-notice";
import {
  useCurrentRBACAuthorizations,
  type CurrentRBACAuthorizationCheck,
} from "../rbac-capabilities";
import {
  reportWorkflowPolicyBindingPermissionBlockReason,
  reportWorkflowPolicyManageKey,
  reportWorkflowPolicyReadKey,
  reportWorkflowPolicyRelationAuthorizationChecks,
} from "./rbac-gates";
import type {
  AlertSourceKind,
  AlertSourceLabels,
  AlertSourceProfile,
  AlertSourceProfileListResponse,
} from "../alert-sources/types";
import type { DiagnosisToolTemplateListResponse } from "../diagnosis-tool-templates/types";
import type {
  GroupingPolicy,
  GroupingPolicyListResponse,
} from "../grouping-policies/types";
import type {
  NotificationChannelProfile,
  NotificationChannelProfileListResponse,
} from "../notification-channels/types";
import { notificationChannelAIProofReadyChannelIDs } from "../notification-channels/format";
import {
  disableReportWorkflowPolicyAction,
  enableReportWorkflowPolicyAction,
  previewReportWorkflowPolicyDraftImpactAction,
  previewReportWorkflowPolicyImpactAction,
  refreshReportWorkflowPolicies,
  submitReportWorkflowPolicy,
  triggerReportWorkflowPolicyReplayAction,
} from "./client-api";
import {
  alertSourceIngressReadinessForSelection,
  diagnosisToolReadinessForSelection,
  defaultReportWorkflowPolicyReplayForm,
  emptyReportWorkflowPolicyForm,
  formStateToReplayRequest,
  formStateToWriteRequest,
  policyToFormState,
  preferredReportNotificationChannelIDForFollowUp,
  reportWorkflowPolicyAutomationOutcome,
  reportWorkflowPolicyAutoRoomReadiness,
  reportWorkflowPolicyImpactDiagnosisEstimate,
  reportWorkflowPolicyImpactReason,
  reportWorkflowPolicyImpactReportChannelReadiness,
  reportWorkflowPolicyDraftPlan,
  reportWorkflowPolicyEnablementReadiness,
  reportWorkflowPolicyFormMatchesPolicy,
  reportWorkflowPolicyRepairBlueprint,
  reportWorkflowPolicySetupBlueprint,
  reportWorkflowPolicyWorkflowReturnCandidates,
  reportNotificationChannelReadinessForSelection,
  reportWorkflowNotificationChannelOptionState,
  reportWorkflowNotificationChannelOperatorReadiness,
  type ReportWorkflowPolicyLaunchIntent,
  type ReportWorkflowPolicyWorkflowReturnCandidate,
} from "./format";
import type {
  ReportReplayTriggerResponse,
  ReportWorkflowPolicy,
  ReportWorkflowPolicyFormState,
  ReportWorkflowPolicyImpactPreviewResult,
  ReportWorkflowPolicyListResponse,
  ReportWorkflowPolicyReplayFormState,
  ReportWorkflowPolicyReplayRequest,
  ReportWorkflowPolicyWriteRequest,
} from "./types";

type ReportWorkflowPolicySettingsManagerProps = {
  alertSourcesResult: ApiResult<AlertSourceProfileListResponse>;
  groupingPoliciesResult: ApiResult<GroupingPolicyListResponse>;
  notificationChannelsResult: ApiResult<NotificationChannelProfileListResponse>;
  diagnosisToolTemplatesResult: ApiResult<DiagnosisToolTemplateListResponse>;
  launchIntent?: ReportWorkflowPolicyLaunchIntent | null;
  result: ApiResult<ReportWorkflowPolicyListResponse>;
};

const reportWorkflowPoliciesQueryKey = [
  "settings",
  "report-workflow-policies",
] as const;

type SavePolicyVariables = {
  body: ReportWorkflowPolicyWriteRequest;
  policyID: number | null;
};

type WorkflowPolicyTranslator = ReturnType<typeof useTranslations<"WorkflowPolicySettings">>;

const reportWorkflowPolicyBaseAuthorizationChecks: CurrentRBACAuthorizationCheck[] =
  [
    { key: "reportWorkflowRead", permission: "report_workflow.read" },
    { key: "reportWorkflowManage", permission: "report_workflow.manage" },
  ];

type EnablementVariables = {
  enabled: boolean;
  policyID: number;
};

type ReplayVariables = {
  body: ReportWorkflowPolicyReplayRequest;
  policyID: number;
};

type ImpactPreviewGroup =
  ReportWorkflowPolicyImpactPreviewResult["groups"][number];

type ImpactPreviewState = {
  result: ReportWorkflowPolicyImpactPreviewResult;
  title: string;
};

type RelationSelectOption = {
  disabled?: boolean;
  label: string;
  title: string;
  value: number;
};

type WorkflowRelationOptions = {
  alertSourceEnabledIDs: Set<number>;
  alertSourceKindsByID: Map<number, AlertSourceKind>;
  alertSourceLabels: Record<number, string>;
  alertSourceLabelsByID: Map<number, AlertSourceLabels>;
  alertSourceOptions: RelationSelectOption[];
  groupingPolicyEnabledIDs: Set<number>;
  groupingPolicyLabels: Record<number, string>;
  groupingPolicyOptions: RelationSelectOption[];
  notificationChannelLabels: Record<number, string>;
  notificationChannels: NotificationChannelProfile[];
  notificationChannelsByID: Map<number, NotificationChannelProfile>;
  notificationChannelKindsByID: Map<number, NotificationChannelProfile["kind"]>;
  notificationChannelOptions: RelationSelectOption[];
  notificationChannelEnabledIDs: Set<number>;
  diagnosisAIProofNotificationChannelIDs: Set<number>;
  diagnosisConsultationNotificationChannelIDs: Set<number>;
  diagnosisCloseNotificationChannelIDs: Set<number>;
  reportNotificationChannelIDs: Set<number>;
  warnings: string[];
};

export function ReportWorkflowPolicySettingsManager({
  alertSourcesResult,
  diagnosisToolTemplatesResult,
  groupingPoliciesResult,
  launchIntent = null,
  notificationChannelsResult,
  result,
}: ReportWorkflowPolicySettingsManagerProps) {
  const t = useTranslations("WorkflowPolicySettings");
  const common = useTranslations("Common");
  const [form] = Form.useForm<ReportWorkflowPolicyFormState>();
  const [replayForm] = Form.useForm<ReportWorkflowPolicyReplayFormState>();
  const clientReady = useClientReady();
  const [editingID, setEditingID] = useState<number | null>(null);
  const [actionID, setActionID] = useState<number | null>(null);
  const [draftImpacting, setDraftImpacting] = useState(false);
  const [impactingID, setImpactingID] = useState<number | null>(null);
  const [impactPreview, setImpactPreview] = useState<ImpactPreviewState | null>(
    null,
  );
  const [impactResults, setImpactResults] = useState<
    Record<number, ReportWorkflowPolicyImpactPreviewResult>
  >({});
  const [launchNotice, setLaunchNotice] = useState<string | null>(
    launchIntent?.message ?? null,
  );
  const [replayPolicy, setReplayPolicy] = useState<ReportWorkflowPolicy | null>(
    null,
  );
  const [replayResult, setReplayResult] =
    useState<ReportReplayTriggerResponse | null>(null);
  const {
    errorStatus,
    items: policies,
    notice,
    query,
    refresh,
    setNotice,
  } = useSettingsList({
    initialResult: result,
    queryKey: reportWorkflowPoliciesQueryKey,
    queryFn: refreshReportWorkflowPolicies,
    refreshMessage: t("refreshed"),
    selectItems: (response) => response.items,
  });
  const savePolicy = useSettingsMutation<
    SavePolicyVariables,
    ReportWorkflowPolicy
  >({
    invalidateQueryKey: reportWorkflowPoliciesQueryKey,
    mutationFn: ({ policyID, body }) =>
      submitReportWorkflowPolicy(policyID, body),
  });
  const enablementAction = useSettingsMutation<
    EnablementVariables,
    ReportWorkflowPolicy
  >({
    invalidateQueryKey: reportWorkflowPoliciesQueryKey,
    mutationFn: ({ policyID, enabled }) =>
      enabled
        ? enableReportWorkflowPolicyAction(policyID)
        : disableReportWorkflowPolicyAction(policyID),
  });
  const replayAction = useSettingsMutation<
    ReplayVariables,
    ReportReplayTriggerResponse
  >({
    invalidateQueryKey: reportWorkflowPoliciesQueryKey,
    mutationFn: ({ policyID, body }) =>
      triggerReportWorkflowPolicyReplayAction(policyID, body),
  });
  const authorizationChecks = useMemo(
    () => [
      ...reportWorkflowPolicyBaseAuthorizationChecks,
      ...policies.flatMap((policy) => [
        {
          key: reportWorkflowPolicyReadKey(policy.id),
          permission: "report_workflow.read" as const,
          scopeKey: String(policy.id),
          scopeKind: "report_workflow" as const,
        },
        {
          key: reportWorkflowPolicyManageKey(policy.id),
          permission: "report_workflow.manage" as const,
          scopeKey: String(policy.id),
          scopeKind: "report_workflow" as const,
        },
      ]),
      ...reportWorkflowPolicyRelationAuthorizationChecks({
        alertSourceProfileIDs: alertSourcesResult.ok
          ? alertSourcesResult.data.items.map((source) => source.id)
          : [],
        groupingPolicyIDs: groupingPoliciesResult.ok
          ? groupingPoliciesResult.data.items.map((policy) => policy.id)
          : [],
        notificationChannelProfileIDs: notificationChannelsResult.ok
          ? notificationChannelsResult.data.items.map((channel) => channel.id)
          : [],
      }),
    ],
    [
      alertSourcesResult,
      groupingPoliciesResult,
      notificationChannelsResult,
      policies,
    ],
  );
  const currentAuthorization = useCurrentRBACAuthorizations(
    authorizationChecks,
    clientReady,
  );
  const busy =
    !clientReady ||
    currentAuthorization.isChecking ||
    query.isFetching ||
    savePolicy.isPending ||
    enablementAction.isPending ||
    replayAction.isPending ||
    draftImpacting ||
    impactingID !== null;
  const canReadPolicies = currentAuthorization.can("reportWorkflowRead");
  const canCreatePolicy = currentAuthorization.can("reportWorkflowManage");
  const canPreviewDraftImpact = currentAuthorization.can("reportWorkflowManage");
  const authorizationChecking = !clientReady || currentAuthorization.isChecking;
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadPolicies,
    errorStatus,
    isChecking: authorizationChecking,
    message: common("readAccessLimited", {
      resource: t("policiesResource"),
    }),
  });
  const relationOptions = useMemo(
    () =>
      buildRelationOptions(
        alertSourcesResult,
        groupingPoliciesResult,
        notificationChannelsResult,
        t,
      ),
    [alertSourcesResult, groupingPoliciesResult, notificationChannelsResult, t],
  );
  const diagnosisToolTemplates = diagnosisToolTemplatesResult.ok
    ? diagnosisToolTemplatesResult.data.items
    : null;
  const initialFormValues = useMemo(
    () => reportWorkflowPolicyLaunchInitialForm(launchIntent, relationOptions),
    [launchIntent, relationOptions],
  );
  const selectedName = Form.useWatch("name", form) ?? "";
  const selectedAlertSourceID =
    Form.useWatch("alertSourceProfileID", form) ?? null;
  const selectedGroupingPolicyID =
    Form.useWatch("groupingPolicyID", form) ?? null;
  const selectedTriggerMode =
    Form.useWatch("triggerMode", form) ?? "manual_replay";
  const selectedReportScenario =
    Form.useWatch("reportScenario", form) ?? "single_alert";
  const selectedDiagnosisFollowUp =
    Form.useWatch("diagnosisFollowUp", form) ?? "disabled";
  const selectedMaxFailedSubReports =
    Form.useWatch("maxFailedSubReports", form) ?? 0;
  const selectedReportNotificationChannelProfileID = Form.useWatch(
    "reportNotificationChannelProfileID",
    form,
  );
  const currentPolicyManagePermissionBlockReason = authorizationChecking
    ? ""
    : editingID === null ||
        currentAuthorization.can(reportWorkflowPolicyManageKey(editingID))
      ? ""
      : t("runtimePattern.unauthorizedPolicy", { id: editingID });
  const policyBindingPermissionBlockReason =
    authorizationChecking
      ? ""
      : reportWorkflowPolicyBindingPermissionBlockReason({
          alertSourceProfileID: selectedAlertSourceID,
          can: currentAuthorization.can,
          editingPolicyID: editingID,
          groupingPolicyID: selectedGroupingPolicyID,
          reportNotificationChannelProfileID:
            selectedReportNotificationChannelProfileID,
        });
  const policySavePermissionBlockReason =
    authorizationChecking
      ? ""
      : editingID === null
        ? canCreatePolicy
          ? ""
          : t("notAuthorizedCreate")
        : currentPolicyManagePermissionBlockReason ||
          policyBindingPermissionBlockReason;
  const createPermissionDenied =
    !authorizationChecking && editingID === null && !canCreatePolicy;
  const canEditCurrentPolicyForm =
    authorizationChecking ||
    (editingID === null
      ? canCreatePolicy
      : currentPolicyManagePermissionBlockReason === "");
  const canSaveCurrentPolicy = policySavePermissionBlockReason === "";
  const formPermissionNotice =
    settingsManagePermissionNotice({
      canManage:
        !createPermissionDenied && currentPolicyManagePermissionBlockReason === "",
      isChecking: authorizationChecking,
      message: common("formReadOnly", {
        resource:
          editingID === null
            ? t("creationResource")
            : t("policyResource", { id: editingID }),
      }),
    }) ??
    (policyBindingPermissionBlockReason === ""
      ? null
      : {
          kind: "warning" as const,
          message: localizeWorkflowPolicyText(policyBindingPermissionBlockReason, t),
        });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;
  const alertSourceIngressReadiness = useMemo(
    () =>
      alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
        alertSourceKindsByID: relationOptions.alertSourceKindsByID,
        alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
        alertSourceProfileID: selectedAlertSourceID,
        diagnosisFollowUp: selectedDiagnosisFollowUp,
      }),
    [relationOptions, selectedAlertSourceID, selectedDiagnosisFollowUp],
  );
  const diagnosisToolReadiness = useMemo(
    () =>
      diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
        alertSourceKindsByID: relationOptions.alertSourceKindsByID,
        alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
        alertSourceProfileID: selectedAlertSourceID,
        diagnosisFollowUp: selectedDiagnosisFollowUp,
        templates: diagnosisToolTemplatesResult.ok
          ? diagnosisToolTemplatesResult.data.items
          : null,
      }),
    [
      diagnosisToolTemplatesResult,
      relationOptions,
      selectedAlertSourceID,
      selectedDiagnosisFollowUp,
    ],
  );
  const reportNotificationChannelReadiness = useMemo(
    () =>
      reportNotificationChannelReadinessForSelection({
        diagnosisAIProofNotificationChannelIDs:
          relationOptions.diagnosisAIProofNotificationChannelIDs,
        diagnosisConsultationNotificationChannelIDs:
          relationOptions.diagnosisConsultationNotificationChannelIDs,
        diagnosisCloseNotificationChannelIDs:
          relationOptions.diagnosisCloseNotificationChannelIDs,
        diagnosisFollowUp: selectedDiagnosisFollowUp,
        notificationChannelEnabledIDs:
          relationOptions.notificationChannelEnabledIDs,
        notificationChannelKindsByID:
          relationOptions.notificationChannelKindsByID,
        reportNotificationChannelIDs:
          relationOptions.reportNotificationChannelIDs,
        reportNotificationChannelProfileID:
          selectedReportNotificationChannelProfileID,
      }),
    [
      relationOptions,
      selectedDiagnosisFollowUp,
      selectedReportNotificationChannelProfileID,
    ],
  );
  const selectedReportNotificationChannel = useMemo(
    () =>
      selectedReportNotificationChannelProfileID === undefined
        ? null
        : (relationOptions.notificationChannelsByID.get(
            selectedReportNotificationChannelProfileID,
          ) ?? null),
    [
      relationOptions.notificationChannelsByID,
      selectedReportNotificationChannelProfileID,
    ],
  );
  const operatorChannelReadiness = useMemo(
    () =>
      reportWorkflowNotificationChannelOperatorReadiness({
        channel: selectedReportNotificationChannel,
        diagnosisFollowUp: selectedDiagnosisFollowUp,
        readiness: reportNotificationChannelReadiness,
      }),
    [
      reportNotificationChannelReadiness,
      selectedDiagnosisFollowUp,
      selectedReportNotificationChannel,
    ],
  );
  const alertSourceOptions = useMemo(
    () =>
      alertSourceOptionsForFollowUp(selectedDiagnosisFollowUp, relationOptions, t),
    [relationOptions, selectedDiagnosisFollowUp, t],
  );
  const notificationChannelOptions = useMemo(
    () =>
      notificationChannelOptionsForFollowUp(
        selectedDiagnosisFollowUp,
        relationOptions,
        t,
      ),
    [relationOptions, selectedDiagnosisFollowUp, t],
  );
  const draftFormState = useMemo<ReportWorkflowPolicyFormState>(
    () => ({
      name: selectedName,
      alertSourceProfileID: selectedAlertSourceID,
      groupingPolicyID: selectedGroupingPolicyID,
      reportNotificationChannelProfileID:
        selectedReportNotificationChannelProfileID,
      maxFailedSubReports: selectedMaxFailedSubReports,
      triggerMode: selectedTriggerMode,
      reportScenario: selectedReportScenario,
      diagnosisFollowUp: selectedDiagnosisFollowUp,
    }),
    [
      selectedAlertSourceID,
      selectedDiagnosisFollowUp,
      selectedGroupingPolicyID,
      selectedMaxFailedSubReports,
      selectedName,
      selectedReportNotificationChannelProfileID,
      selectedReportScenario,
      selectedTriggerMode,
    ],
  );
  const draftPlan = useMemo(
    () =>
      reportWorkflowPolicyDraftPlan({
        alertSourceIngressReadiness,
        alertSourceLabels: relationOptions.alertSourceLabels,
        diagnosisToolReadiness,
        editingPolicyID: editingID,
        form: draftFormState,
        groupingPolicyLabels: relationOptions.groupingPolicyLabels,
        notificationChannelLabels: relationOptions.notificationChannelLabels,
        reportNotificationChannelReadiness,
      }),
    [
      alertSourceIngressReadiness,
      diagnosisToolReadiness,
      draftFormState,
      editingID,
      relationOptions.alertSourceLabels,
      relationOptions.groupingPolicyLabels,
      relationOptions.notificationChannelLabels,
      reportNotificationChannelReadiness,
    ],
  );
  const automationOutcome = useMemo(
    () =>
      reportWorkflowPolicyAutomationOutcome({
        alertSourceIngressReadiness,
        alertSourceLabels: relationOptions.alertSourceLabels,
        diagnosisToolReadiness,
        form: draftFormState,
        groupingPolicyLabels: relationOptions.groupingPolicyLabels,
        notificationChannelLabels: relationOptions.notificationChannelLabels,
        reportNotificationChannelReadiness,
      }),
    [
      alertSourceIngressReadiness,
      diagnosisToolReadiness,
      draftFormState,
      relationOptions.alertSourceLabels,
      relationOptions.groupingPolicyLabels,
      relationOptions.notificationChannelLabels,
      reportNotificationChannelReadiness,
    ],
  );
  const autoRoomReadiness = useMemo(
    () =>
      reportWorkflowPolicyAutoRoomReadiness({
        alertSourceIngressReadiness,
        diagnosisToolReadiness,
        form: draftFormState,
        operatorChannelReadiness,
        reportNotificationChannelReadiness,
      }),
    [
      alertSourceIngressReadiness,
      diagnosisToolReadiness,
      draftFormState,
      operatorChannelReadiness,
      reportNotificationChannelReadiness,
    ],
  );
  const setupBlueprint = useMemo(
    () =>
      reportWorkflowPolicySetupBlueprint({
        alertSourceIngressReadiness,
        alertSourceKindsByID: relationOptions.alertSourceKindsByID,
        alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
        diagnosisToolReadiness,
        form: draftFormState,
        reportNotificationChannelReadiness,
      }),
    [
      alertSourceIngressReadiness,
      diagnosisToolReadiness,
      draftFormState,
      relationOptions.alertSourceKindsByID,
      relationOptions.alertSourceLabelsByID,
      reportNotificationChannelReadiness,
    ],
  );
  const editingPolicy = useMemo(
    () =>
      editingID === null
        ? null
        : (policies.find((policy) => policy.id === editingID) ?? null),
    [editingID, policies],
  );
  const editingPolicyDraftMatchesSaved = useMemo(
    () => reportWorkflowPolicyFormMatchesPolicy(draftFormState, editingPolicy),
    [draftFormState, editingPolicy],
  );

  const summary = useMemo(() => {
    const enabled = policies.filter((policy) => policy.enabled).length;
    const reportChannel = policies.filter(
      (policy) => policy.report_notification_channel_profile_id !== null,
    ).length;
    const roomFollowUp = policies.filter((policy) =>
      isAIRoomFollowUp(policy.diagnosis_follow_up),
    ).length;
    return { enabled, reportChannel, roomFollowUp };
  }, [policies]);
  const workflowReturnCandidates = useMemo(
    () =>
      reportWorkflowPolicyWorkflowReturnCandidates({
        alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
        alertSourceKindsByID: relationOptions.alertSourceKindsByID,
        alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
        diagnosisAIProofNotificationChannelIDs:
          relationOptions.diagnosisAIProofNotificationChannelIDs,
        diagnosisConsultationNotificationChannelIDs:
          relationOptions.diagnosisConsultationNotificationChannelIDs,
        diagnosisCloseNotificationChannelIDs:
          relationOptions.diagnosisCloseNotificationChannelIDs,
        groupingPolicyEnabledIDs: relationOptions.groupingPolicyEnabledIDs,
        launchIntent,
        notificationChannelEnabledIDs:
          relationOptions.notificationChannelEnabledIDs,
        notificationChannelKindsByID:
          relationOptions.notificationChannelKindsByID,
        policies,
        reportNotificationChannelIDs:
          relationOptions.reportNotificationChannelIDs,
        templates: diagnosisToolTemplates,
      }),
    [diagnosisToolTemplates, launchIntent, policies, relationOptions],
  );
  const workflowReturnPolicyIDs = useMemo(
    () =>
      new Set(
        workflowReturnCandidates.map((candidate) => candidate.policy.id),
      ),
    [workflowReturnCandidates],
  );

  async function handleRefresh() {
    await refresh();
  }

  async function handleSubmit(values: ReportWorkflowPolicyFormState) {
    if (!canSaveCurrentPolicy) {
      setNotice({
        kind: "warning",
        message:
          localizeWorkflowPolicyText(policySavePermissionBlockReason, t) ||
          t("notAuthorizedSave"),
      });
      return;
    }
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeWorkflowPolicyText(parsed.message, t) });
      return;
    }
    const deliveryReadiness = reportNotificationChannelReadinessForSelection({
      diagnosisAIProofNotificationChannelIDs:
        relationOptions.diagnosisAIProofNotificationChannelIDs,
      diagnosisConsultationNotificationChannelIDs:
        relationOptions.diagnosisConsultationNotificationChannelIDs,
      diagnosisCloseNotificationChannelIDs:
        relationOptions.diagnosisCloseNotificationChannelIDs,
      diagnosisFollowUp: values.diagnosisFollowUp,
      notificationChannelEnabledIDs:
        relationOptions.notificationChannelEnabledIDs,
      notificationChannelKindsByID:
        relationOptions.notificationChannelKindsByID,
      reportNotificationChannelIDs:
        relationOptions.reportNotificationChannelIDs,
      reportNotificationChannelProfileID:
        values.reportNotificationChannelProfileID,
    });
    const ingressReadiness = alertSourceIngressReadinessForSelection({
      alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
      alertSourceKindsByID: relationOptions.alertSourceKindsByID,
      alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
      alertSourceProfileID: values.alertSourceProfileID,
      diagnosisFollowUp: values.diagnosisFollowUp,
    });
    const toolReadiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
      alertSourceKindsByID: relationOptions.alertSourceKindsByID,
      alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
      alertSourceProfileID: values.alertSourceProfileID,
      diagnosisFollowUp: values.diagnosisFollowUp,
      templates: diagnosisToolTemplatesResult.ok
        ? diagnosisToolTemplatesResult.data.items
        : null,
    });

    try {
      await savePolicy.mutateAsync({ policyID: editingID, body: parsed.value });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      return;
    }

    const reviewItems = [
      deliveryReadiness.status === "review" ? deliveryReadiness.detail : "",
      ingressReadiness.status === "review" ? ingressReadiness.detail : "",
      toolReadiness.status === "review" ? toolReadiness.detail : "",
    ].filter((item) => item !== "");
    const enablementBlockers = [
      deliveryReadiness.status === "blocked" ? deliveryReadiness.detail : "",
      ingressReadiness.status === "blocked" ? ingressReadiness.detail : "",
      toolReadiness.status === "blocked" ? toolReadiness.detail : "",
    ].filter((item) => item !== "");
    form.setFieldsValue(emptyReportWorkflowPolicyForm());
    setEditingID(null);
    setLaunchNotice(null);
    setNotice({
      kind:
        enablementBlockers.length > 0 || reviewItems.length > 0
          ? "warning"
          : "info",
      message:
        enablementBlockers.length > 0
          ? t("savedWithBlockers", {
              detail: localizeWorkflowPolicyMessages(enablementBlockers, t),
            })
          : reviewItems.length > 0
          ? t("savedWithReview", {
              detail: localizeWorkflowPolicyMessages(reviewItems, t),
            })
          : t("saved"),
    });
  }

  async function handleEnablement(
    policy: ReportWorkflowPolicy,
    enabled: boolean,
  ) {
    if (!currentAuthorization.can(reportWorkflowPolicyManageKey(policy.id))) {
      setNotice({ kind: "warning", message: t("notAuthorizedChange") });
      return;
    }
    setActionID(policy.id);
    const readiness = reportWorkflowPolicyEnablementReadiness({
      alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
      alertSourceKindsByID: relationOptions.alertSourceKindsByID,
      alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
      diagnosisAIProofNotificationChannelIDs:
        relationOptions.diagnosisAIProofNotificationChannelIDs,
      diagnosisConsultationNotificationChannelIDs:
        relationOptions.diagnosisConsultationNotificationChannelIDs,
      diagnosisCloseNotificationChannelIDs:
        relationOptions.diagnosisCloseNotificationChannelIDs,
      groupingPolicyEnabledIDs: relationOptions.groupingPolicyEnabledIDs,
      notificationChannelEnabledIDs:
        relationOptions.notificationChannelEnabledIDs,
      notificationChannelKindsByID:
        relationOptions.notificationChannelKindsByID,
      policy,
      reportNotificationChannelIDs:
        relationOptions.reportNotificationChannelIDs,
      templates: diagnosisToolTemplatesResult.ok
        ? diagnosisToolTemplatesResult.data.items
        : null,
    });
    if (enabled && readiness.status === "blocked") {
      setNotice({ kind: "error", message: localizeWorkflowPolicyText(readiness.detail, t) });
      setActionID(null);
      return;
    }
    try {
      await enablementAction.mutateAsync({ policyID: policy.id, enabled });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
      setActionID(null);
      return;
    }
    setActionID(null);
    setNotice({
      kind:
        enabled && readiness.status === "review"
          ? "warning"
          : enabled
            ? "info"
            : "warning",
      message:
        enabled && readiness.status === "review"
          ? t("enabledWithReview", { detail: localizeWorkflowPolicyText(readiness.detail, t) })
          : enabled
            ? t("enabledNotice")
            : t("disabledNotice"),
    });
  }

  function editPolicy(policy: ReportWorkflowPolicy) {
    if (!currentAuthorization.can(reportWorkflowPolicyManageKey(policy.id))) {
      setNotice({ kind: "warning", message: t("notAuthorizedEdit") });
      return;
    }
    setEditingID(policy.id);
    form.setFieldsValue(policyToFormState(policy));
    setLaunchNotice(null);
    setNotice(null);
  }

  function openReplay(policy: ReportWorkflowPolicy) {
    if (!currentAuthorization.can(reportWorkflowPolicyManageKey(policy.id))) {
      setNotice({ kind: "warning", message: t("notAuthorizedReplay") });
      return;
    }
    setReplayPolicy(policy);
    setReplayResult(null);
    replayForm.setFieldsValue(defaultReportWorkflowPolicyReplayForm());
    setNotice(null);
  }

  function closeReplay() {
    if (replayAction.isPending) {
      return;
    }
    setReplayPolicy(null);
    setReplayResult(null);
    replayForm.resetFields();
  }

  function closeImpactPreview() {
    setImpactPreview(null);
  }

  async function handleImpactPreview(policy: ReportWorkflowPolicy) {
    if (!currentAuthorization.can(reportWorkflowPolicyReadKey(policy.id))) {
      setNotice({ kind: "warning", message: t("notAuthorizedPreview") });
      return;
    }
    setImpactingID(policy.id);
    const previewed = await previewReportWorkflowPolicyImpactAction(policy.id);
    setImpactingID(null);
    if (!previewed.ok) {
      setNotice({ kind: "error", message: previewed.error.message });
      return;
    }
    setImpactResults((current) => ({
      ...current,
      [policy.id]: previewed.data,
    }));
    setImpactPreview({
      title: t("impactPreviewNumber", { id: policy.id }),
      result: previewed.data,
    });
    setNotice({
      kind: previewed.data.status === "blocked" ? "warning" : "info",
      message: t("impactPreviewNotice", {
        events: previewed.data.events_matched,
        groups: previewed.data.groups_estimated,
        status: localizeWorkflowPolicyText(previewed.data.status, t),
      }),
    });
  }

  async function handleDraftImpactPreview() {
    if (!canPreviewDraftImpact) {
      setNotice({ kind: "warning", message: t("notAuthorizedDraftPreview") });
      return;
    }
    const parsed = formStateToWriteRequest(draftFormState);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeWorkflowPolicyText(parsed.message, t) });
      return;
    }
    setDraftImpacting(true);
    const previewed = await previewReportWorkflowPolicyDraftImpactAction(
      parsed.value,
    );
    setDraftImpacting(false);
    if (!previewed.ok) {
      setNotice({ kind: "error", message: previewed.error.message });
      return;
    }
    setImpactPreview({ title: t("draftImpactPreview"), result: previewed.data });
    setNotice({
      kind: previewed.data.status === "blocked" ? "warning" : "info",
      message: t("draftImpactPreviewNotice", {
        events: previewed.data.events_matched,
        groups: previewed.data.groups_estimated,
        status: localizeWorkflowPolicyText(previewed.data.status, t),
      }),
    });
  }

  async function handleReplay(values: ReportWorkflowPolicyReplayFormState) {
    if (replayPolicy === null) {
      return;
    }
    if (!currentAuthorization.can(reportWorkflowPolicyManageKey(replayPolicy.id))) {
      setNotice({ kind: "warning", message: t("notAuthorizedReplay") });
      return;
    }
    const parsed = formStateToReplayRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeWorkflowPolicyText(parsed.message, t) });
      return;
    }
    try {
      const replayed = await replayAction.mutateAsync({
        policyID: replayPolicy.id,
        body: parsed.value,
      });
      setReplayResult(replayed);
      setNotice({
        kind: replayed.started ? "info" : "warning",
        message: replayed.started
          ? t("replayAccepted")
          : t("replayNoSnapshots"),
      });
    } catch (error) {
      setNotice({
        kind: "error",
        message: settingsErrorMessage(error, common("requestFailed")),
      });
    }
  }

  function resetForm() {
    setEditingID(null);
    form.setFieldsValue(emptyReportWorkflowPolicyForm());
    setLaunchNotice(null);
    setNotice(null);
  }

  return (
    <div className="stack">
      <Row aria-label={t("metricsLabel")} gutter={[12, 12]}>
        <MetricCard label={t("policies")} value={policies.length} />
        <MetricCard label={t("enabled")} value={summary.enabled} />
        <MetricCard label={t("reportChannel")} value={summary.reportChannel} />
        <MetricCard label={t("roomFollowUp")} value={summary.roomFollowUp} />
      </Row>

      <WorkflowReadinessPanel
        diagnosisToolTemplates={
          diagnosisToolTemplatesResult.ok
            ? diagnosisToolTemplatesResult.data.items
            : null
        }
        impactResults={impactResults}
        policies={policies}
        relationOptions={relationOptions}
      />

      {launchNotice ? (
        <Alert
          aria-label={t("launchPreset")}
          description={localizeWorkflowPolicyText(launchNotice, t)}
          message={t("actionLoaded")}
          role="status"
          showIcon
          type="info"
        />
      ) : null}
      {workflowReturnCandidates.length > 0 ? (
        <WorkflowReturnCandidateNotice
          actionID={actionID}
          busy={busy}
          canManagePolicy={(policyID) =>
            currentAuthorization.can(reportWorkflowPolicyManageKey(policyID))
          }
          candidates={workflowReturnCandidates}
          onEnable={(policy) => handleEnablement(policy, true)}
        />
      ) : null}
      {relationOptions.warnings.length > 0 ? (
        <Alert
          description={localizeWorkflowPolicyMessages(relationOptions.warnings, t)}
          message={t("relatedUnavailable")}
          role="status"
          showIcon
          type="warning"
        />
      ) : null}
      {!diagnosisToolTemplatesResult.ok ? (
        <Alert
          description={diagnosisToolTemplatesResult.error.message}
          message={t("toolsUnavailable")}
          role="status"
          showIcon
          type="warning"
        />
      ) : null}

      {visibleNotice ? <Notice notice={visibleNotice} /> : null}

      <Row align="top" className="settings-console-grid" gutter={[16, 16]}>
        <Col lg={8} md={24} xs={24}>
          <Card
            extra={
              editingID === null ? null : (
                <Button
                  disabled={busy || !canCreatePolicy}
                  icon={<PlusOutlined />}
                  onClick={resetForm}
                  type="default"
                >
                  {t("new")}
                </Button>
              )
            }
            title={
              editingID === null
                ? t("newPolicy")
                : t("editPolicy", { id: editingID })
            }
          >
            {formPermissionNotice ? (
              <ReadOnlyModeAlert notice={formPermissionNotice} />
            ) : null}
            <Form<ReportWorkflowPolicyFormState>
              disabled={busy || !canEditCurrentPolicyForm}
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
                  {
                    max: 120,
                    message: t("nameLength"),
                  },
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("alertSource")}
                    name="alertSourceProfileID"
                    rules={[
                      { required: true, message: t("sourceRequired") },
                    ]}
                  >
                    <Select
                      optionFilterProp="label"
                      options={alertSourceOptions}
                      placeholder={t("selectSource")}
                      showSearch
                    />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("groupingPolicy")}
                    name="groupingPolicyID"
                    rules={[
                      {
                        required: true,
                        message: t("groupingRequired"),
                      },
                    ]}
                  >
                    <Select
                      optionFilterProp="label"
                      options={relationOptions.groupingPolicyOptions}
                      placeholder={t("selectGrouping")}
                      showSearch
                    />
                  </Form.Item>
                </Col>
              </Row>
              <AlertSourceIngressReadinessPreview
                readiness={alertSourceIngressReadiness}
              />

              <Form.Item
                label={t("reportChannel")}
                name="reportNotificationChannelProfileID"
              >
                <Select
                  allowClear
                  optionFilterProp="label"
                  options={notificationChannelOptions}
                  placeholder={t("noReportChannel")}
                  showSearch
                />
              </Form.Item>
              <NotificationChannelReadinessPreview
                operatorReadiness={operatorChannelReadiness}
                readiness={reportNotificationChannelReadiness}
                selectedChannel={selectedReportNotificationChannel}
              />

              <Form.Item
                label={t("trigger")}
                name="triggerMode"
                rules={[{ required: true, message: t("triggerRequired") }]}
              >
                <Segmented
                  block
                  options={[{ value: "manual_replay", label: t("manualReplay") }]}
                />
              </Form.Item>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("scenario")}
                    name="reportScenario"
                    rules={[{ required: true, message: t("scenarioRequired") }]}
                  >
                    <Select
                      options={[
                        { value: "single_alert", label: t("singleAlert") },
                        { value: "cascade", label: t("cascade") },
                        { value: "alert_storm", label: t("alertStorm") },
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label={t("maxFailedSubReports")}
                    name="maxFailedSubReports"
                    rules={[
                      { required: true, message: t("maxFailedSubReportsRequired") },
                      {
                        type: "number",
                        min: 0,
                        max: 100000,
                        message: t("maxFailedSubReportsRange"),
                      },
                    ]}
                  >
                    <InputNumber
                      max={100000}
                      min={0}
                      precision={0}
                      style={{ width: "100%" }}
                    />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item
                label={t("diagnosisFollowUp")}
                name="diagnosisFollowUp"
                rules={[
                  {
                    required: true,
                    message: t("followUpRequired"),
                  },
                ]}
              >
                <Segmented
                  block
                  options={[
                    { value: "disabled", label: t("disabled") },
                    { value: "suggest_room", label: t("suggestRoom") },
                    { value: "auto_room", label: t("autoRoom") },
                  ]}
                />
              </Form.Item>
              <DiagnosisToolReadinessPreview
                readiness={diagnosisToolReadiness}
              />
              <AutoRoomReadinessPreview readiness={autoRoomReadiness} />
              <WorkflowAutomationOutcomePreview outcome={automationOutcome} />
              <WorkflowSetupBlueprintPreview blueprint={setupBlueprint} />
              <DraftWorkflowPlanPreview
                draftImpacting={draftImpacting}
                draftMatchesSaved={editingPolicyDraftMatchesSaved}
                impactingID={impactingID}
                canPreviewDraftImpact={canPreviewDraftImpact}
                canPreviewPolicy={
                  editingID === null
                    ? false
                    : currentAuthorization.can(reportWorkflowPolicyReadKey(editingID))
                }
                onDraftImpactPreview={handleDraftImpactPreview}
                onImpactPreview={handleImpactPreview}
                plan={draftPlan}
                policy={editingPolicy}
              />

              <Space wrap>
                <Button
                  disabled={busy || !canSaveCurrentPolicy}
                  htmlType="submit"
                  icon={<SaveOutlined />}
                  loading={busy}
                  type="primary"
                >
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
              <Button
                disabled={busy || !canReadPolicies}
                icon={<ReloadOutlined />}
                loading={busy}
                onClick={handleRefresh}
                type="default"
              >
                {t("refresh")}
              </Button>
            }
            title={t("configuredPolicies")}
          >
            <ReportWorkflowPolicyTable
              actionID={actionID}
              busy={busy}
              canRead={canReadPolicies}
              canManagePolicy={(policyID) =>
                currentAuthorization.can(reportWorkflowPolicyManageKey(policyID))
              }
              canReadPolicy={(policyID) =>
                currentAuthorization.can(reportWorkflowPolicyReadKey(policyID))
              }
              onDisable={(policy) => handleEnablement(policy, false)}
              onEdit={editPolicy}
              onEnable={(policy) => handleEnablement(policy, true)}
              onImpactPreview={handleImpactPreview}
              onReplay={openReplay}
              impactResults={impactResults}
              impactingID={impactingID}
              diagnosisToolTemplates={
                diagnosisToolTemplatesResult.ok
                  ? diagnosisToolTemplatesResult.data.items
                  : null
              }
              policies={policies}
              highlightPolicyIDs={workflowReturnPolicyIDs}
              relationOptions={relationOptions}
            />
          </Card>
        </Col>
      </Row>

      <ReplayPolicyModal
        busy={replayAction.isPending}
        form={replayForm}
        onCancel={closeReplay}
        onSubmit={handleReplay}
        policy={replayPolicy}
        result={replayResult}
      />
      <ImpactPreviewModal
        onCancel={closeImpactPreview}
        preview={impactPreview}
      />
    </div>
  );
}

type ReadinessStatus = "ready" | "review" | "pending" | "blocked";

type WorkflowStage = {
  detail: string;
  status: ReadinessStatus;
  title: string;
};

function buildRelationOptions(
  alertSourcesResult: ApiResult<AlertSourceProfileListResponse>,
  groupingPoliciesResult: ApiResult<GroupingPolicyListResponse>,
  notificationChannelsResult: ApiResult<NotificationChannelProfileListResponse>,
  t: WorkflowPolicyTranslator,
): WorkflowRelationOptions {
  const warnings: string[] = [];
  const alertSources = alertSourcesResult.ok
    ? alertSourcesResult.data.items
    : [];
  const groupingPolicies = groupingPoliciesResult.ok
    ? groupingPoliciesResult.data.items
    : [];
  const notificationChannels = notificationChannelsResult.ok
    ? notificationChannelsResult.data.items
    : [];
  const reportNotificationChannels = notificationChannels.filter((channel) =>
    channel.delivery_scopes.includes("report"),
  );
  const diagnosisConsultationNotificationChannels = notificationChannels.filter(
    (channel) => channel.delivery_scopes.includes("diagnosis_consultation"),
  );
  const diagnosisCloseNotificationChannels = notificationChannels.filter(
    (channel) => channel.delivery_scopes.includes("diagnosis_close"),
  );

  if (!alertSourcesResult.ok) {
    warnings.push(
      `Alert sources failed to load: ${alertSourcesResult.error.message}.`,
    );
  }
  if (!groupingPoliciesResult.ok) {
    warnings.push(
      `Grouping policies failed to load: ${groupingPoliciesResult.error.message}.`,
    );
  }
  if (!notificationChannelsResult.ok) {
    warnings.push(
      `Notification channels failed to load: ${notificationChannelsResult.error.message}.`,
    );
  }

  return {
    alertSourceEnabledIDs: new Set(
      alertSources
        .filter((source) => source.enabled)
        .map((source) => source.id),
    ),
    alertSourceKindsByID: new Map(
      alertSources.map((source) => [source.id, source.kind]),
    ),
    alertSourceLabels: Object.fromEntries(
      alertSources.map((source) => [source.id, alertSourceLabel(source, t)]),
    ),
    alertSourceLabelsByID: new Map(
      alertSources.map((source) => [source.id, source.labels]),
    ),
    alertSourceOptions: alertSources.map((source) =>
      relationOption(source.id, alertSourceLabel(source, t)),
    ),
    groupingPolicyEnabledIDs: new Set(
      groupingPolicies
        .filter((policy) => policy.enabled)
        .map((policy) => policy.id),
    ),
    groupingPolicyLabels: Object.fromEntries(
      groupingPolicies.map((policy) => [
        policy.id,
        groupingPolicyLabel(policy, t),
      ]),
    ),
    groupingPolicyOptions: groupingPolicies.map((policy) =>
      relationOption(policy.id, groupingPolicyLabel(policy, t)),
    ),
    notificationChannelLabels: Object.fromEntries(
      notificationChannels.map((channel) => [
        channel.id,
        notificationChannelLabel(channel, t),
      ]),
    ),
    notificationChannels,
    notificationChannelsByID: new Map(
      notificationChannels.map((channel) => [channel.id, channel]),
    ),
    notificationChannelKindsByID: new Map(
      notificationChannels.map((channel) => [channel.id, channel.kind]),
    ),
    notificationChannelOptions: reportNotificationChannels.map((channel) =>
      relationOption(channel.id, notificationChannelLabel(channel, t)),
    ),
    notificationChannelEnabledIDs: new Set(
      notificationChannels
        .filter((channel) => channel.enabled)
        .map((channel) => channel.id),
    ),
    diagnosisConsultationNotificationChannelIDs: new Set(
      diagnosisConsultationNotificationChannels.map((channel) => channel.id),
    ),
    diagnosisCloseNotificationChannelIDs: new Set(
      diagnosisCloseNotificationChannels.map((channel) => channel.id),
    ),
    diagnosisAIProofNotificationChannelIDs: new Set(
      notificationChannelAIProofReadyChannelIDs(notificationChannels),
    ),
    reportNotificationChannelIDs: new Set(
      reportNotificationChannels.map((channel) => channel.id),
    ),
    warnings,
  };
}

function reportWorkflowPolicyLaunchInitialForm(
  launchIntent: ReportWorkflowPolicyLaunchIntent | null,
  relationOptions: WorkflowRelationOptions,
): ReportWorkflowPolicyFormState {
  if (launchIntent === null) {
    return emptyReportWorkflowPolicyForm();
  }
  return {
    ...emptyReportWorkflowPolicyForm(),
    alertSourceProfileID: launchAlertSourceIDForFollowUp(
      launchIntent.diagnosisFollowUp,
      launchIntent.alertSourceProfileID,
      relationOptions,
    ),
    diagnosisFollowUp: launchIntent.diagnosisFollowUp,
    groupingPolicyID: launchGroupingPolicyID(relationOptions),
    name: launchIntent.name,
    reportNotificationChannelProfileID: launchNotificationChannelIDForFollowUp(
      launchIntent.diagnosisFollowUp,
      relationOptions,
    ),
  };
}

function launchAlertSourceIDForFollowUp(
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  sourceID: number | null,
  relationOptions: WorkflowRelationOptions,
): number | null {
  if (
    sourceID !== null &&
    alertSourceAllowedForFollowUp(diagnosisFollowUp, sourceID, relationOptions)
  ) {
    return sourceID;
  }
  if (diagnosisFollowUp === "auto_room") {
    return (
      relationOptions.alertSourceOptions.find(
        (option) =>
          relationOptions.alertSourceEnabledIDs.has(option.value) &&
          relationOptions.alertSourceKindsByID.get(option.value) ===
            "alertmanager",
      )?.value ?? null
    );
  }
  return firstEnabledOptionID(
    relationOptions.alertSourceOptions,
    relationOptions.alertSourceEnabledIDs,
  );
}

function launchGroupingPolicyID(
  relationOptions: WorkflowRelationOptions,
): number | null {
  const enabled = relationOptions.groupingPolicyOptions.filter((option) =>
    relationOptions.groupingPolicyEnabledIDs.has(option.value),
  );
  if (enabled.length === 0) {
    return null;
  }
  const defaultPolicy = enabled.find((option) =>
    (relationOptions.groupingPolicyLabels[option.value] ?? option.label)
      .toLowerCase()
      .includes("default"),
  );
  return (
    defaultPolicy?.value ??
    enabled.reduce((lowest, option) =>
      option.value < lowest.value ? option : lowest,
    ).value
  );
}

function alertSourceAllowedForFollowUp(
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  sourceID: number,
  relationOptions: WorkflowRelationOptions,
): boolean {
  if (!relationOptions.alertSourceEnabledIDs.has(sourceID)) {
    return false;
  }
  if (diagnosisFollowUp !== "auto_room") {
    return true;
  }
  return relationOptions.alertSourceKindsByID.get(sourceID) === "alertmanager";
}

function launchNotificationChannelIDForFollowUp(
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  relationOptions: WorkflowRelationOptions,
): number | undefined {
  return preferredReportNotificationChannelIDForFollowUp({
    channels: relationOptions.notificationChannels,
    diagnosisAIProofNotificationChannelIDs:
      relationOptions.diagnosisAIProofNotificationChannelIDs,
    diagnosisFollowUp,
  });
}

function firstEnabledOptionID(
  options: RelationSelectOption[],
  enabledIDs: ReadonlySet<number>,
): number | null {
  return options.find((option) => enabledIDs.has(option.value))?.value ?? null;
}

function relationOption(value: number, label: string): RelationSelectOption {
  return { value, label, title: label };
}

function alertSourceOptionsForFollowUp(
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  relationOptions: WorkflowRelationOptions,
  t: WorkflowPolicyTranslator,
): RelationSelectOption[] {
  return relationOptions.alertSourceOptions.map((option) => {
    if (diagnosisFollowUp !== "auto_room") {
      return option;
    }
    const sourceKind = relationOptions.alertSourceKindsByID.get(option.value);
    const sourceEnabled = relationOptions.alertSourceEnabledIDs.has(
      option.value,
    );
    if (sourceEnabled && sourceKind === "alertmanager") {
      return option;
    }

    const reason = localizeWorkflowPolicyText(
      alertSourceAutoRoomBlockReason(sourceEnabled, sourceKind),
      t,
    );
    const label = `${option.label} - ${reason}`;
    return {
      ...option,
      disabled: true,
      label,
      title: label,
    };
  });
}

function alertSourceAutoRoomBlockReason(
  sourceEnabled: boolean,
  sourceKind: AlertSourceKind | undefined,
): string {
  if (!sourceEnabled && sourceKind !== "alertmanager") {
    return "disabled / not Alertmanager";
  }
  if (!sourceEnabled) {
    return "disabled for auto_room";
  }
  if (sourceKind === undefined) {
    return "unknown source kind";
  }
  return "not Alertmanager";
}

function notificationChannelOptionsForFollowUp(
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  relationOptions: WorkflowRelationOptions,
  t: WorkflowPolicyTranslator,
): RelationSelectOption[] {
  return relationOptions.notificationChannelOptions.map((option) => {
    const state = reportWorkflowNotificationChannelOptionState({
      diagnosisAIProofNotificationChannelIDs:
        relationOptions.diagnosisAIProofNotificationChannelIDs,
      diagnosisCloseNotificationChannelIDs:
        relationOptions.diagnosisCloseNotificationChannelIDs,
      diagnosisConsultationNotificationChannelIDs:
        relationOptions.diagnosisConsultationNotificationChannelIDs,
      diagnosisFollowUp,
      notificationChannelEnabledIDs:
        relationOptions.notificationChannelEnabledIDs,
      notificationChannelKindsByID:
        relationOptions.notificationChannelKindsByID,
      notificationChannelProfileID: option.value,
    });
    const hints = [...state.reasons, ...state.reviewReasons];
    if (hints.length === 0) {
      return option;
    }
    const label = `${option.label} - ${hints.map((hint) => localizeWorkflowPolicyText(hint, t)).join(", ")}`;
    return {
      ...option,
      disabled: state.disabled,
      label,
      title: label,
    };
  });
}

function alertSourceLabel(source: AlertSourceProfile, t: WorkflowPolicyTranslator): string {
  return `#${source.id} ${source.name} (${source.kind}, ${enabledLabel(source.enabled, t)})`;
}

function groupingPolicyLabel(policy: GroupingPolicy, t: WorkflowPolicyTranslator): string {
  const dimensions =
    policy.dimension_keys.length === 0
      ? t("catalog.noDimensions")
      : policy.dimension_keys.join(", ");
  return `#${policy.id} ${policy.name} (${dimensions}, ${enabledLabel(policy.enabled, t)})`;
}

function notificationChannelLabel(
  channel: NotificationChannelProfile,
  t: WorkflowPolicyTranslator,
): string {
  const scopes =
    channel.delivery_scopes.length === 0
      ? t("catalog.noScopes")
      : channel.delivery_scopes.map((scope) => localizeWorkflowPolicyText(scope, t)).join(", ");
  return `#${channel.id} ${channel.name} (${scopes}, ${enabledLabel(channel.enabled, t)})`;
}

function enabledLabel(enabled: boolean, t: WorkflowPolicyTranslator): string {
  return localizeWorkflowPolicyText(enabled ? "enabled" : "disabled", t);
}

function relationLabel(
  labels: Record<number, string>,
  id: number,
  fallback: string,
): string {
  return labels[id] ?? fallback;
}

function WorkflowReadinessPanel({
  diagnosisToolTemplates,
  impactResults,
  policies,
  relationOptions,
}: {
  diagnosisToolTemplates: DiagnosisToolTemplateListResponse["items"] | null;
  impactResults: Record<number, ReportWorkflowPolicyImpactPreviewResult>;
  policies: ReportWorkflowPolicy[];
  relationOptions: WorkflowRelationOptions;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  const selectedPolicy = selectReadinessPolicy(policies);
  const impact =
    selectedPolicy === null ? undefined : impactResults[selectedPolicy.id];
  const stages =
    selectedPolicy === null
      ? []
      : workflowStages(
          selectedPolicy,
          impact,
          relationOptions,
          diagnosisToolTemplates,
        );
  const firstUnreadyStage = stages.findIndex(
    (stage) => stage.status !== "ready",
  );
  const currentStep =
    firstUnreadyStage === -1 ? stages.length : firstUnreadyStage;
  const activeRoomPolicies = policies.filter(
    (policy) => policy.enabled && isAIRoomFollowUp(policy.diagnosis_follow_up),
  ).length;
  const reportDeliveryPolicies = policies.filter(
    (policy) =>
      policy.enabled && policy.report_notification_channel_profile_id !== null,
  ).length;
  const readyPreviews = policies.filter(
    (policy) => impactResults[policy.id]?.status === "ready",
  ).length;
  const blockedPreviews = policies.filter(
    (policy) => impactResults[policy.id]?.status === "blocked",
  ).length;
  const overallStatus =
    selectedPolicy === null ? "pending" : workflowOverallStatus(stages);

  return (
    <section
      aria-label={t("workflowReadinessLabel")}
      className="panel workflow-readiness-panel"
    >
      <div className="panel-header workflow-readiness-header">
        <h2>{t("aiConsultationWorkflow")}</h2>
        <Tag color={readinessTagColor(overallStatus)}>
          {readinessLabel(overallStatus, t)}
        </Tag>
      </div>
      <div className="panel-body workflow-readiness-body">
        {selectedPolicy === null ? (
          <Empty
            description={t("noPolicyConfigured")}
            image={<BranchesOutlined aria-hidden />}
          />
        ) : (
          <>
            <div className="workflow-readiness-selected">
              <div>
                <Typography.Text className="muted">
                  {t("selectedPolicy")}
                </Typography.Text>
                <Typography.Title level={3}>
                  {selectedPolicy.name}
                </Typography.Title>
              </div>
              <Space wrap>
                <Tag color={selectedPolicy.enabled ? "green" : "default"}>
                  {selectedPolicy.enabled ? t("enabled") : t("draft")}
                </Tag>
                <Tag
                  color={followUpTagColor(selectedPolicy.diagnosis_follow_up)}
                >
                  {localizeWorkflowPolicyText(selectedPolicy.diagnosis_follow_up, t)}
                </Tag>
                {impact ? (
                  <Tag color={impactStatusColor(impact.status)}>
                    {t("impactStatus", { status: localizeWorkflowPolicyText(impact.status, t) })}
                  </Tag>
                ) : (
                  <Tag>{t("impactPending")}</Tag>
                )}
              </Space>
            </div>

            <div className="workflow-readiness-steps-wrap">
              <Steps
                aria-label={t("selectedReadiness")}
                className="workflow-readiness-steps"
                current={currentStep}
                items={stages.map((stage) => ({
                  description: localizeWorkflowPolicyText(stage.detail, t),
                  status: readinessStepStatus(stage.status),
                  title: localizeWorkflowPolicyText(stage.title, t),
                }))}
                responsive={false}
              />
            </div>
          </>
        )}

        <Row aria-label={t("workflowCounters")} gutter={[12, 12]}>
          <ReadinessMetric
            label={t("roomReadyPolicies")}
            status="ready"
            value={activeRoomPolicies}
          />
          <ReadinessMetric
            label={t("reportDelivery")}
            status={reportDeliveryPolicies > 0 ? "ready" : "pending"}
            value={reportDeliveryPolicies}
          />
          <ReadinessMetric
            label={t("readyPreviews")}
            status={readyPreviews > 0 ? "ready" : "pending"}
            value={readyPreviews}
          />
          <ReadinessMetric
            label={t("blockedPreviews")}
            status={blockedPreviews > 0 ? "blocked" : "ready"}
            value={blockedPreviews}
          />
        </Row>
      </div>
    </section>
  );
}

function ReadinessMetric({
  label,
  status,
  value,
}: {
  label: string;
  status: ReadinessStatus;
  value: number;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <Col lg={6} sm={12} xs={24}>
      <div className="workflow-readiness-metric">
        <div className="workflow-readiness-metric-value">{value}</div>
        <div className="workflow-readiness-metric-footer">
          <Typography.Text className="muted">{label}</Typography.Text>
          <Tag color={readinessTagColor(status)}>{readinessLabel(status, t)}</Tag>
        </div>
      </div>
    </Col>
  );
}

function DiagnosisToolReadinessPreview({
  readiness,
}: {
  readiness: ReturnType<typeof diagnosisToolReadinessForSelection>;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("toolReadiness")}
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status, t)}
          </Tag>
          <Tag color="blue">
            {t("activeAlertCount", { count: readiness.activeAlertsForSource })}
          </Tag>
          <Tag color="cyan">
            {t("metricToolCount", { count: readiness.enabledMetricTemplates })}
          </Tag>
          <Tag color="purple">
            {t("rangeToolCount", { count: readiness.enabledRangeTemplates })}
          </Tag>
        </Space>
        <Typography.Text strong>{localizeWorkflowPolicyText(readiness.label, t)}</Typography.Text>
        <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.detail, t)}</Typography.Text>
        {readiness.templateNames.length > 0 ? (
          <Space wrap>
            {readiness.templateNames.slice(0, 4).map((name) => (
              <Tag key={name}>{name}</Tag>
            ))}
            {readiness.templateNames.length > 4 ? (
              <Tag>+{readiness.templateNames.length - 4}</Tag>
            ) : null}
          </Space>
        ) : null}
      </Space>
    </div>
  );
}

function WorkflowAutomationOutcomePreview({
  outcome,
}: {
  outcome: ReturnType<typeof reportWorkflowPolicyAutomationOutcome>;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("automationOutcomeLabel")}
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>{t("automationOutcome")}</Typography.Text>
        <Tag color={readinessTagColor(outcome.status)}>
          {readinessLabel(outcome.status, t)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(outcome.detail, t)}</Typography.Text>
      <div className="workflow-automation-grid">
        {outcome.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{localizeWorkflowPolicyText(item.title, t)}</Typography.Text>
              <Tag color={readinessTagColor(item.status)}>
                {readinessLabel(item.status, t)}
              </Tag>
            </div>
            <Typography.Text strong>{localizeWorkflowPolicyText(item.value, t)}</Typography.Text>
            <Typography.Text type="secondary">{localizeWorkflowPolicyText(item.detail, t)}</Typography.Text>
          </div>
        ))}
      </div>
    </div>
  );
}

function AutoRoomReadinessPreview({
  readiness,
}: {
  readiness: ReturnType<typeof reportWorkflowPolicyAutoRoomReadiness>;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  const currentStep = readiness.items.findIndex(
    (item) => item.status !== "ready",
  );

  return (
    <div
      aria-label={t("autoRoomChecklist")}
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>{localizeWorkflowPolicyText(readiness.label, t)}</Typography.Text>
        <Tag color={readinessTagColor(readiness.status)}>
          {readinessLabel(readiness.status, t)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.detail, t)}</Typography.Text>
      {readiness.items.length > 0 ? (
        <Steps
          current={currentStep === -1 ? readiness.items.length : currentStep}
          direction="vertical"
          items={readiness.items.map((item) => ({
            description: (
              <Space direction="vertical" size={2}>
                <Typography.Text strong>{localizeWorkflowPolicyText(item.value, t)}</Typography.Text>
                <Typography.Text type="secondary">
                  {localizeWorkflowPolicyText(item.detail, t)}
                </Typography.Text>
              </Space>
            ),
            status: readinessStepStatus(item.status),
            title: localizeWorkflowPolicyText(item.title, t),
          }))}
          size="small"
        />
      ) : null}
    </div>
  );
}

function WorkflowSetupBlueprintPreview({
  blueprint,
}: {
  blueprint: ReturnType<typeof reportWorkflowPolicySetupBlueprint>;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("setupBlueprintLabel")}
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>{t("setupBlueprint")}</Typography.Text>
        <Tag color={readinessTagColor(blueprint.status)}>
          {readinessLabel(blueprint.status, t)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(blueprint.detail, t)}</Typography.Text>
      <div
        aria-label={t("setupChain")}
        className="workflow-automation-grid"
      >
        {blueprint.phases.map((phase) => (
          <div className="workflow-automation-item" key={phase.key}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">
                {localizeWorkflowPolicyText(phase.title, t)}
              </Typography.Text>
              <Tag color={readinessTagColor(phase.status)}>
                {readinessLabel(phase.status, t)}
              </Tag>
            </div>
            <Typography.Text strong>{localizeWorkflowPolicyText(phase.value, t)}</Typography.Text>
            <Typography.Text type="secondary">{localizeWorkflowPolicyText(phase.detail, t)}</Typography.Text>
          </div>
        ))}
      </div>
      {blueprint.actions.length === 0 ? (
        <Alert
          description={t("savePreviewReplay")}
          message={localizeWorkflowPolicyText(blueprint.label, t)}
          showIcon
          type="success"
        />
      ) : (
        <div className="workflow-automation-grid">
          {blueprint.actions.map((action) => (
            <div className="workflow-automation-item" key={action.key}>
              <div className="workflow-automation-item-header">
                <Typography.Text className="muted">
                  {localizeWorkflowPolicyText(action.title, t)}
                </Typography.Text>
                <Tag color={readinessTagColor(action.status)}>
                  {readinessLabel(action.status, t)}
                </Tag>
              </div>
              <Typography.Text type="secondary">
                {localizeWorkflowPolicyText(action.detail, t)}
              </Typography.Text>
              {action.actionHref ? (
                <Button href={action.actionHref} size="small" type="link">
                  {localizeWorkflowPolicyText(action.actionLabel, t)}
                </Button>
              ) : (
                <Typography.Text className="muted">
                  {localizeWorkflowPolicyText(action.actionLabel, t)}
                </Typography.Text>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function DraftWorkflowPlanPreview({
  canPreviewDraftImpact,
  canPreviewPolicy,
  draftImpacting,
  draftMatchesSaved,
  impactingID,
  onDraftImpactPreview,
  onImpactPreview,
  plan,
  policy,
}: {
  canPreviewDraftImpact: boolean;
  canPreviewPolicy: boolean;
  draftImpacting: boolean;
  draftMatchesSaved: boolean;
  impactingID: number | null;
  onDraftImpactPreview: () => void;
  onImpactPreview: (policy: ReportWorkflowPolicy) => void;
  plan: ReturnType<typeof reportWorkflowPolicyDraftPlan>;
  policy: ReportWorkflowPolicy | null;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  const currentStep = plan.steps.findIndex((step) => step.status !== "ready");

  return (
    <div
      aria-label={t("draftPlanLabel")}
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>{t("draftPlan")}</Typography.Text>
        <Tag color={readinessTagColor(plan.status)}>
          {readinessLabel(plan.status, t)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(plan.detail, t)}</Typography.Text>
      <Steps
        current={currentStep === -1 ? plan.steps.length : currentStep}
        direction="vertical"
        items={plan.steps.map((step) => ({
          description: (
            <StepDescription
              draftImpacting={draftImpacting}
              draftMatchesSaved={draftMatchesSaved}
              impactingID={impactingID}
              canPreviewDraftImpact={canPreviewDraftImpact}
              canPreviewPolicy={canPreviewPolicy}
              onDraftImpactPreview={onDraftImpactPreview}
              onImpactPreview={onImpactPreview}
              policy={policy}
              step={step}
            />
          ),
          status: readinessStepStatus(step.status),
          title: localizeWorkflowPolicyText(step.title, t),
        }))}
        size="small"
      />
    </div>
  );
}

function StepDescription({
  canPreviewDraftImpact,
  canPreviewPolicy,
  draftImpacting,
  draftMatchesSaved,
  impactingID,
  onDraftImpactPreview,
  onImpactPreview,
  policy,
  step,
}: {
  canPreviewDraftImpact: boolean;
  canPreviewPolicy: boolean;
  draftImpacting: boolean;
  draftMatchesSaved: boolean;
  impactingID: number | null;
  onDraftImpactPreview: () => void;
  onImpactPreview: (policy: ReportWorkflowPolicy) => void;
  policy: ReportWorkflowPolicy | null;
  step: ReturnType<typeof reportWorkflowPolicyDraftPlan>["steps"][number];
}) {
  const t = useTranslations("WorkflowPolicySettings");
  if (step.key !== "impact-preview") {
    return localizeWorkflowPolicyText(step.detail, t);
  }
  const savedLoading = policy !== null && impactingID === policy.id;
  const draftPreviewDisabled =
    step.status === "blocked" || impactingID !== null || !canPreviewDraftImpact;
  const savedPreviewDisabled =
    draftImpacting || (impactingID !== null && !savedLoading) || !canPreviewPolicy;

  return (
    <Space direction="vertical" size={6}>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(step.detail, t)}</Typography.Text>
      {policy !== null && !draftMatchesSaved ? (
        <Alert
          description={t("draftPreviewDifference")}
          message={t("unsavedDraftOnly")}
          showIcon
          type="warning"
        />
      ) : null}
      <Space wrap>
        {policy === null || !draftMatchesSaved ? (
          <Button
            disabled={draftPreviewDisabled}
            htmlType="button"
            icon={<RadarChartOutlined />}
            loading={draftImpacting}
            onClick={onDraftImpactPreview}
            size="small"
            type="link"
          >
            {t("previewDraft")}
          </Button>
        ) : null}
        {policy !== null ? (
          <Button
            disabled={savedPreviewDisabled}
            htmlType="button"
            icon={<RadarChartOutlined />}
            loading={savedLoading}
            onClick={() => onImpactPreview(policy)}
            size="small"
            type="link"
          >
            {t("previewSaved")}
          </Button>
        ) : null}
      </Space>
    </Space>
  );
}

function AlertSourceIngressReadinessPreview({
  readiness,
}: {
  readiness: ReturnType<typeof alertSourceIngressReadinessForSelection>;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("webhookReadiness")}
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status, t)}
          </Tag>
          <Tag color="geekblue">{t("alertmanagerWebhook")}</Tag>
        </Space>
        <Typography.Text strong>{localizeWorkflowPolicyText(readiness.label, t)}</Typography.Text>
        <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.detail, t)}</Typography.Text>
      </Space>
    </div>
  );
}

function NotificationChannelReadinessPreview({
  operatorReadiness,
  readiness,
  selectedChannel,
}: {
  operatorReadiness: ReturnType<
    typeof reportWorkflowNotificationChannelOperatorReadiness
  >;
  readiness: ReturnType<typeof reportNotificationChannelReadinessForSelection>;
  selectedChannel: NotificationChannelProfile | null;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("channelReadiness")}
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status, t)}
          </Tag>
          <Tag color={operatorChannelTagColor(operatorReadiness)}>
            {localizeWorkflowPolicyText(operatorReadiness.kindLabel, t)}
          </Tag>
          {selectedChannel === null ? null : (
            <Tag color={selectedChannel.enabled ? "green" : "default"}>
              {selectedChannel.enabled ? t("enabled") : t("disabled")}
            </Tag>
          )}
          <Tag color="blue">{t("requiredScopes", { scopes: readiness.requiredScopes.map((scope) => localizeWorkflowPolicyText(scope, t)).join(", ") })}</Tag>
          {readiness.missingScopes.length > 0 ? (
            <Tag color="red">{t("missingScopes", { scopes: readiness.missingScopes.map((scope) => localizeWorkflowPolicyText(scope, t)).join(", ") })}</Tag>
          ) : null}
        </Space>
        <Typography.Text strong>{localizeWorkflowPolicyText(readiness.label, t)}</Typography.Text>
        <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.detail, t)}</Typography.Text>
        <Typography.Text type="secondary">
          {localizeWorkflowPolicyText(operatorReadiness.detail, t)}
        </Typography.Text>
      </Space>
    </div>
  );
}

function operatorChannelTagColor(
  readiness: ReturnType<
    typeof reportWorkflowNotificationChannelOperatorReadiness
  >,
): string {
  if (readiness.kindLabel === "WeCom" && readiness.status === "ready") {
    return "green";
  }
  if (readiness.kindLabel === "WeCom") {
    return "cyan";
  }
  if (readiness.kindLabel === "Webhook") {
    return "gold";
  }
  return "default";
}

function selectReadinessPolicy(
  policies: ReportWorkflowPolicy[],
): ReportWorkflowPolicy | null {
  return (
    policies.find(
      (policy) => policy.enabled && policy.diagnosis_follow_up === "auto_room",
    ) ??
    policies.find(
      (policy) =>
        policy.enabled && policy.diagnosis_follow_up === "suggest_room",
    ) ??
    policies.find((policy) => policy.enabled) ??
    policies[0] ??
    null
  );
}

function isAIRoomFollowUp(
  mode: ReportWorkflowPolicy["diagnosis_follow_up"],
): boolean {
  return mode === "suggest_room" || mode === "auto_room";
}

function followUpReadinessDetail(
  mode: ReportWorkflowPolicy["diagnosis_follow_up"],
): string {
  switch (mode) {
    case "auto_room":
      return "Diagnosis room starts automatically";
    case "suggest_room":
      return "Diagnosis room suggested";
    case "disabled":
      return "Follow-up disabled";
  }
}

function followUpTagColor(
  mode: ReportWorkflowPolicy["diagnosis_follow_up"],
): string {
  switch (mode) {
    case "auto_room":
      return "geekblue";
    case "suggest_room":
      return "blue";
    case "disabled":
      return "default";
  }
}

function workflowStages(
  policy: ReportWorkflowPolicy,
  impact: ReportWorkflowPolicyImpactPreviewResult | undefined,
  relationOptions: WorkflowRelationOptions,
  diagnosisToolTemplates: DiagnosisToolTemplateListResponse["items"] | null,
): WorkflowStage[] {
  const ingressReadiness = alertSourceIngressReadinessForSelection({
    alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
    alertSourceKindsByID: relationOptions.alertSourceKindsByID,
    alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
    alertSourceProfileID: policy.alert_source_profile_id,
    diagnosisFollowUp: policy.diagnosis_follow_up,
  });
  const diagnosisToolReadiness = diagnosisToolReadinessForSelection({
    alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
    alertSourceKindsByID: relationOptions.alertSourceKindsByID,
    alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
    alertSourceProfileID: policy.alert_source_profile_id,
    diagnosisFollowUp: policy.diagnosis_follow_up,
    templates: diagnosisToolTemplates,
  });
  const deliveryReadiness = reportNotificationChannelReadinessForSelection({
    diagnosisAIProofNotificationChannelIDs:
      relationOptions.diagnosisAIProofNotificationChannelIDs,
    diagnosisConsultationNotificationChannelIDs:
      relationOptions.diagnosisConsultationNotificationChannelIDs,
    diagnosisCloseNotificationChannelIDs:
      relationOptions.diagnosisCloseNotificationChannelIDs,
    diagnosisFollowUp: policy.diagnosis_follow_up,
    notificationChannelEnabledIDs:
      relationOptions.notificationChannelEnabledIDs,
    notificationChannelKindsByID: relationOptions.notificationChannelKindsByID,
    reportNotificationChannelIDs: relationOptions.reportNotificationChannelIDs,
    reportNotificationChannelProfileID:
      policy.report_notification_channel_profile_id,
  });
  const reportChannelLabel =
    policy.report_notification_channel_profile_id === null
      ? "No report channel"
      : relationLabel(
          relationOptions.notificationChannelLabels,
          policy.report_notification_channel_profile_id,
          `Report channel #${policy.report_notification_channel_profile_id}`,
        );
  return [
    {
      title: "Source",
      detail: relationLabel(
        relationOptions.alertSourceLabels,
        policy.alert_source_profile_id,
        `Alert source #${policy.alert_source_profile_id}`,
      ),
      status: relationOptions.alertSourceEnabledIDs.has(
        policy.alert_source_profile_id,
      )
        ? "ready"
        : "blocked",
    },
    {
      title: "Webhook",
      detail: ingressReadiness.detail,
      status: ingressReadiness.status,
    },
    {
      title: "Evidence",
      detail: diagnosisToolReadiness.detail,
      status: diagnosisToolReadiness.status,
    },
    {
      title: "Grouping",
      detail: relationLabel(
        relationOptions.groupingPolicyLabels,
        policy.grouping_policy_id,
        `Grouping policy #${policy.grouping_policy_id}`,
      ),
      status: relationOptions.groupingPolicyEnabledIDs.has(
        policy.grouping_policy_id,
      )
        ? "ready"
        : "blocked",
    },
    {
      title: "AI room",
      detail: followUpReadinessDetail(policy.diagnosis_follow_up),
      status: isAIRoomFollowUp(policy.diagnosis_follow_up)
        ? "ready"
        : "blocked",
    },
    {
      title: "Delivery",
      detail:
        policy.report_notification_channel_profile_id === null ||
        deliveryReadiness.status === "ready"
          ? reportChannelLabel
          : `${reportChannelLabel}: ${deliveryReadiness.detail}`,
      status: deliveryReadiness.status,
    },
    {
      title: "Proof",
      detail: impact
        ? `${impact.groups_estimated} groups / ${impact.events_matched} events`
        : "Impact not previewed",
      status: impact ? impact.status : "pending",
    },
  ];
}

function workflowOverallStatus(stages: WorkflowStage[]): ReadinessStatus {
  if (stages.some((stage) => stage.status === "blocked")) {
    return "blocked";
  }
  if (stages.some((stage) => stage.status === "review")) {
    return "review";
  }
  if (stages.some((stage) => stage.status === "pending")) {
    return "pending";
  }
  return "ready";
}

function readinessStepStatus(status: ReadinessStatus) {
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

function readinessTagColor(status: ReadinessStatus) {
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

function readinessLabel(
  status: ReadinessStatus,
  t: WorkflowPolicyTranslator,
): string {
  return t(`readinessStatus.${status}`);
}

function readinessAlertType(status: ReadinessStatus) {
  switch (status) {
    case "ready":
      return "success";
    case "review":
      return "warning";
    case "pending":
      return "info";
    case "blocked":
      return "error";
  }
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

function Notice({ notice }: { notice: SettingsNotice }) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <Alert
      description={notice.message}
      message={notice.kind === "error" ? t("requestFailed") : t("settings")}
      role={notice.kind === "error" ? "alert" : "status"}
      showIcon
      type={notice.kind}
    />
  );
}

function WorkflowReturnCandidateNotice({
  actionID,
  busy,
  canManagePolicy,
  candidates,
  onEnable,
}: {
  actionID: number | null;
  busy: boolean;
  canManagePolicy: (policyID: number) => boolean;
  candidates: ReportWorkflowPolicyWorkflowReturnCandidate[];
  onEnable: (policy: ReportWorkflowPolicy) => void;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  const primaryCandidate =
    candidates.find(
      (candidate) =>
        candidate.action === "enable" || candidate.action === "review",
    ) ?? null;
  return (
    <Alert
      action={
        primaryCandidate === null ? undefined : (
          <Button
            disabled={
              busy ||
              actionID !== null ||
              !canManagePolicy(primaryCandidate.policy.id)
            }
            icon={<PlayCircleOutlined />}
            loading={actionID === primaryCandidate.policy.id}
            onClick={() => onEnable(primaryCandidate.policy)}
            size="small"
            type="primary"
          >
            {t("enablePolicy", { id: primaryCandidate.policy.id })}
          </Button>
        )
      }
      description={
        <Space direction="vertical" size={4}>
          <Typography.Text>
            {workflowReturnCandidateSummary(candidates, t)}
          </Typography.Text>
          <Space size={4} wrap>
            {candidates.slice(0, 4).map((candidate) => (
              <Tooltip key={candidate.policy.id} title={localizeWorkflowPolicyText(candidate.detail, t)}>
                <Tag color={workflowReturnCandidateTagColor(candidate.action)}>
                  #{candidate.policy.id} {candidate.policy.name}:{" "}
                  {localizeWorkflowPolicyText(workflowReturnCandidateLabel(candidate.action), t)}
                </Tag>
              </Tooltip>
            ))}
          </Space>
        </Space>
      }
      message={t("roomCandidates")}
      role="status"
      showIcon
      type={workflowReturnCandidateAlertType(candidates)}
    />
  );
}

function workflowReturnCandidateSummary(
  candidates: ReportWorkflowPolicyWorkflowReturnCandidate[],
  t: WorkflowPolicyTranslator,
): string {
  const enableable = candidates.filter(
    (candidate) =>
      candidate.action === "enable" || candidate.action === "review",
  ).length;
  const enabled = candidates.filter(
    (candidate) => candidate.action === "already_enabled",
  ).length;
  const blocked = candidates.filter(
    (candidate) => candidate.action === "blocked",
  ).length;
  return t("workflowCandidateSummary", {
    blocked,
    enableable,
    enabled,
    total: candidates.length,
  });
}

function workflowReturnCandidateLabel(
  action: ReportWorkflowPolicyWorkflowReturnCandidate["action"],
): string {
  switch (action) {
    case "already_enabled":
      return "enabled";
    case "blocked":
      return "blocked";
    case "enable":
      return "ready";
    case "review":
      return "review";
  }
}

function workflowReturnCandidateTagColor(
  action: ReportWorkflowPolicyWorkflowReturnCandidate["action"],
): string {
  switch (action) {
    case "already_enabled":
      return "blue";
    case "blocked":
      return "red";
    case "enable":
      return "green";
    case "review":
      return "gold";
  }
}

function workflowReturnCandidateAlertType(
  candidates: ReportWorkflowPolicyWorkflowReturnCandidate[],
): "success" | "info" | "warning" {
  if (
    candidates.some(
      (candidate) =>
        candidate.action === "enable" || candidate.action === "review",
    )
  ) {
    return "success";
  }
  if (candidates.some((candidate) => candidate.action === "already_enabled")) {
    return "info";
  }
  return "warning";
}

type ReportWorkflowPolicyTableProps = {
  actionID: number | null;
  busy: boolean;
  canRead: boolean;
  canManagePolicy: (policyID: number) => boolean;
  canReadPolicy: (policyID: number) => boolean;
  highlightPolicyIDs: ReadonlySet<number>;
  impactResults: Record<number, ReportWorkflowPolicyImpactPreviewResult>;
  impactingID: number | null;
  onDisable: (policy: ReportWorkflowPolicy) => void;
  onEdit: (policy: ReportWorkflowPolicy) => void;
  onEnable: (policy: ReportWorkflowPolicy) => void;
  onImpactPreview: (policy: ReportWorkflowPolicy) => void;
  onReplay: (policy: ReportWorkflowPolicy) => void;
  diagnosisToolTemplates: DiagnosisToolTemplateListResponse["items"] | null;
  policies: ReportWorkflowPolicy[];
  relationOptions: WorkflowRelationOptions;
};

function ReportWorkflowPolicyTable({
  actionID,
  busy,
  canRead,
  canManagePolicy,
  canReadPolicy,
  highlightPolicyIDs,
  impactResults,
  impactingID,
  onDisable,
  onEdit,
  onEnable,
  onImpactPreview,
  onReplay,
  diagnosisToolTemplates,
  policies,
  relationOptions,
}: ReportWorkflowPolicyTableProps) {
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  const common = useTranslations("Common");
  const columns: TableColumnsType<ReportWorkflowPolicy> = [
    {
      key: "name",
      title: t("name"),
      render: (_, policy) => (
        <Space direction="vertical" size={2}>
          <Space size={4} wrap>
            <Typography.Text strong>{policy.name}</Typography.Text>
            {highlightPolicyIDs.has(policy.id) ? (
              <Tag color="gold">{t("returnTarget")}</Tag>
            ) : null}
          </Space>
          <Typography.Text type="secondary">
            {relationLabel(
              relationOptions.alertSourceLabels,
              policy.alert_source_profile_id,
              t("sourceNumber", { id: policy.alert_source_profile_id }),
            )}
          </Typography.Text>
          <Typography.Text type="secondary">
            {relationLabel(
              relationOptions.groupingPolicyLabels,
              policy.grouping_policy_id,
              t("groupingNumber", { id: policy.grouping_policy_id }),
            )}
          </Typography.Text>
        </Space>
      ),
    },
    {
      dataIndex: "report_scenario",
      key: "scenario",
      title: t("scenario"),
      render: (scenario: ReportWorkflowPolicy["report_scenario"]) => (
        <Tag>{localizeWorkflowPolicyText(scenario, t)}</Tag>
      ),
    },
    {
      dataIndex: "diagnosis_follow_up",
      key: "followup",
      title: t("followUp"),
      render: (mode: ReportWorkflowPolicy["diagnosis_follow_up"]) => (
        <Tag color={followUpTagColor(mode)}>{localizeWorkflowPolicyText(mode, t)}</Tag>
      ),
    },
    {
      dataIndex: "max_failed_sub_reports",
      key: "maxFailedSubReports",
      title: t("failureTolerance"),
      render: (count: number) => t("failedSubReportCount", { count }),
    },
    {
      dataIndex: "report_notification_channel_profile_id",
      key: "reportChannel",
      title: t("reportChannel"),
      render: (
        profileID: ReportWorkflowPolicy["report_notification_channel_profile_id"],
      ) =>
        profileID === null ? (
          <Tag>{t("none")}</Tag>
        ) : (
          <Tag color="geekblue">
            {relationLabel(
              relationOptions.notificationChannelLabels,
              profileID,
              `#${profileID}`,
            )}
          </Tag>
        ),
    },
    {
      dataIndex: "enabled",
      key: "enabled",
      title: t("state"),
      render: (enabled: boolean, policy) => (
        <Space direction="vertical" size={2}>
          <Tag color={enabled ? "green" : "default"}>
            {enabled ? t("enabled") : t("draft")}
          </Tag>
          <Typography.Text type="secondary">
            {enabled
              ? nullableDate(policy.enabled_at, locale, t)
              : nullableDate(policy.disabled_at, locale, t)}
          </Typography.Text>
        </Space>
      ),
    },
    {
      key: "impact",
      title: t("impact"),
      render: (_, policy) => (
        <ImpactSummary result={impactResults[policy.id]} />
      ),
    },
    {
      key: "enablement",
      title: t("enablement"),
      render: (_, policy) => (
        <PolicyEnablementSummary
          diagnosisToolTemplates={diagnosisToolTemplates}
          policy={policy}
          relationOptions={relationOptions}
        />
      ),
    },
    {
      key: "repair",
      title: t("repair"),
      render: (_, policy) => (
        <PolicyRepairActions
          diagnosisToolTemplates={diagnosisToolTemplates}
          policy={policy}
          relationOptions={relationOptions}
        />
      ),
    },
    {
      dataIndex: "updated_at",
      key: "updated",
      title: t("updated"),
      render: (value: string) => formatDateTime(value, locale),
    },
    {
      key: "actions",
      render: (_, policy) => {
        const canManage = canManagePolicy(policy.id);
        const canRead = canReadPolicy(policy.id);
        return (
          <Space wrap>
            <Button
              disabled={busy || actionID !== null || !canManage}
              icon={<EditOutlined />}
              onClick={() => onEdit(policy)}
              size="small"
            >
              {t("edit")}
            </Button>
            <Button
              disabled={busy || actionID !== null || !policy.enabled || !canManage}
              icon={<ThunderboltOutlined />}
              onClick={() => onReplay(policy)}
              size="small"
            >
              {t("replay")}
            </Button>
            <Button
              disabled={
                busy ||
                (impactingID !== null && impactingID !== policy.id) ||
                !canRead
              }
              icon={<RadarChartOutlined />}
              loading={impactingID === policy.id}
              onClick={() => onImpactPreview(policy)}
              size="small"
            >
              {t("impact")}
            </Button>
            {policy.enabled ? (
              <Button
                disabled={busy || actionID !== null || !canManage}
                icon={<PauseCircleOutlined />}
                loading={actionID === policy.id}
                onClick={() => onDisable(policy)}
                size="small"
              >
                {t("disable")}
              </Button>
            ) : (
              <EnablePolicyButton
                actionID={actionID}
                busy={busy}
                canManage={canManage}
                diagnosisToolTemplates={diagnosisToolTemplates}
                onEnable={onEnable}
                policy={policy}
                relationOptions={relationOptions}
              />
            )}
          </Space>
        );
      },
      title: t("actions"),
    },
  ];

  return (
    <Table<ReportWorkflowPolicy>
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
            image={
              <BranchesOutlined aria-hidden className="settings-empty-icon" />
            }
          />
        ),
      }}
      pagination={false}
      rowKey="id"
      rowClassName={(policy) =>
        highlightPolicyIDs.has(policy.id)
          ? "settings-table-row-focus"
          : ""
      }
      scroll={{ x: 1340 }}
    />
  );
}

function PolicyRepairActions({
  diagnosisToolTemplates,
  policy,
  relationOptions,
}: {
  diagnosisToolTemplates: DiagnosisToolTemplateListResponse["items"] | null;
  policy: ReportWorkflowPolicy;
  relationOptions: WorkflowRelationOptions;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  const blueprint = reportWorkflowPolicyRepairBlueprint({
    alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
    alertSourceKindsByID: relationOptions.alertSourceKindsByID,
    alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
    diagnosisAIProofNotificationChannelIDs:
      relationOptions.diagnosisAIProofNotificationChannelIDs,
    diagnosisConsultationNotificationChannelIDs:
      relationOptions.diagnosisConsultationNotificationChannelIDs,
    diagnosisCloseNotificationChannelIDs:
      relationOptions.diagnosisCloseNotificationChannelIDs,
    groupingPolicyEnabledIDs: relationOptions.groupingPolicyEnabledIDs,
    notificationChannelEnabledIDs:
      relationOptions.notificationChannelEnabledIDs,
    notificationChannelKindsByID: relationOptions.notificationChannelKindsByID,
    policy,
    reportNotificationChannelIDs: relationOptions.reportNotificationChannelIDs,
    templates: diagnosisToolTemplates,
  });
  const linkedActions = blueprint.actions.filter(
    (action) => action.actionHref !== undefined,
  );
  const visibleLinkedActions = linkedActions.slice(0, 3);
  const unlinkedActionCount = blueprint.actions.length - linkedActions.length;
  const overflowLinkedActionCount =
    linkedActions.length - visibleLinkedActions.length;
  if (blueprint.actions.length === 0) {
    return (
      <Space direction="vertical" size={2}>
        <Tag color={readinessTagColor(blueprint.status)}>
          {readinessLabel(blueprint.status, t)}
        </Tag>
        <Typography.Text type="secondary">{t("noRepair")}</Typography.Text>
      </Space>
    );
  }
  return (
    <Space direction="vertical" size={4}>
      <Space wrap>
        <Tag color={readinessTagColor(blueprint.status)}>
          {readinessLabel(blueprint.status, t)}
        </Tag>
        <Tooltip title={localizeWorkflowPolicyText(blueprint.detail, t)}>
          <Typography.Text type="secondary">{localizeWorkflowPolicyText(blueprint.label, t)}</Typography.Text>
        </Tooltip>
      </Space>
      <Typography.Text type="secondary">
        {localizeWorkflowPolicyText(policyRepairSummaryDetail(blueprint), t)}
      </Typography.Text>
      {visibleLinkedActions.map((action) => (
        <Tooltip key={action.key} title={localizeWorkflowPolicyText(action.detail, t)}>
          <Button
            href={action.actionHref}
            icon={<ToolOutlined />}
            size="small"
            type="link"
          >
            {localizeWorkflowPolicyText(action.actionLabel, t)}
          </Button>
        </Tooltip>
      ))}
      {unlinkedActionCount > 0 ? (
        <Tooltip title={localizeWorkflowPolicyText(blueprint.detail, t)}>
          <Typography.Text type="secondary">
            {t("manualRepairItems", { count: unlinkedActionCount })}
          </Typography.Text>
        </Tooltip>
      ) : null}
      {overflowLinkedActionCount > 0 ? (
        <Typography.Text type="secondary">
          {t("moreItems", { count: overflowLinkedActionCount })}
        </Typography.Text>
      ) : null}
    </Space>
  );
}

function EnablePolicyButton({
  actionID,
  busy,
  canManage,
  diagnosisToolTemplates,
  onEnable,
  policy,
  relationOptions,
}: {
  actionID: number | null;
  busy: boolean;
  canManage: boolean;
  diagnosisToolTemplates: DiagnosisToolTemplateListResponse["items"] | null;
  onEnable: (policy: ReportWorkflowPolicy) => void;
  policy: ReportWorkflowPolicy;
  relationOptions: WorkflowRelationOptions;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  const readiness = policyEnablementReadiness(
    policy,
    relationOptions,
    diagnosisToolTemplates,
  );
  const blocked = readiness.status === "blocked";
  const disabled = busy || actionID !== null || blocked || !canManage;
  const button = (
    <Button
      disabled={disabled}
      icon={<PlayCircleOutlined />}
      loading={actionID === policy.id}
      onClick={() => onEnable(policy)}
      size="small"
      type="primary"
    >
      {t("enable")}
    </Button>
  );

  if (!blocked) {
    return button;
  }
  return (
    <Tooltip title={localizeWorkflowPolicyText(readiness.detail, t)}>
      <span>{button}</span>
    </Tooltip>
  );
}

function PolicyEnablementSummary({
  diagnosisToolTemplates,
  policy,
  relationOptions,
}: {
  diagnosisToolTemplates: DiagnosisToolTemplateListResponse["items"] | null;
  policy: ReportWorkflowPolicy;
  relationOptions: WorkflowRelationOptions;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  const readiness = policyEnablementReadiness(
    policy,
    relationOptions,
    diagnosisToolTemplates,
  );
  const blockerCount = readiness.blockers.length;
  const warningCount = readiness.warnings.length;
  return (
    <Space direction="vertical" size={2}>
      <Tag color={readinessTagColor(readiness.status)}>
        {readinessLabel(readiness.status, t)}
      </Tag>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.label, t)}</Typography.Text>
      <Typography.Text type="secondary">
        {localizeWorkflowPolicyText(policyEnablementSummaryDetail(readiness), t)}
      </Typography.Text>
      {blockerCount + warningCount > 0 ? (
        <Space size={4} wrap>
          {blockerCount > 0 ? (
            <Tooltip title={localizeWorkflowPolicyMessages(readiness.blockers, t)}>
              <Tag color="red">
                {t("blockerCount", { count: blockerCount })}
              </Tag>
            </Tooltip>
          ) : null}
          {warningCount > 0 ? (
            <Tooltip title={localizeWorkflowPolicyMessages(readiness.warnings, t)}>
              <Tag color="gold">
                {t("reviewItemCount", { count: warningCount })}
              </Tag>
            </Tooltip>
          ) : null}
        </Space>
      ) : null}
    </Space>
  );
}

function policyEnablementReadiness(
  policy: ReportWorkflowPolicy,
  relationOptions: WorkflowRelationOptions,
  diagnosisToolTemplates: DiagnosisToolTemplateListResponse["items"] | null,
) {
  return reportWorkflowPolicyEnablementReadiness({
    alertSourceEnabledIDs: relationOptions.alertSourceEnabledIDs,
    alertSourceKindsByID: relationOptions.alertSourceKindsByID,
    alertSourceLabelsByID: relationOptions.alertSourceLabelsByID,
    diagnosisAIProofNotificationChannelIDs:
      relationOptions.diagnosisAIProofNotificationChannelIDs,
    diagnosisConsultationNotificationChannelIDs:
      relationOptions.diagnosisConsultationNotificationChannelIDs,
    diagnosisCloseNotificationChannelIDs:
      relationOptions.diagnosisCloseNotificationChannelIDs,
    groupingPolicyEnabledIDs: relationOptions.groupingPolicyEnabledIDs,
    notificationChannelEnabledIDs:
      relationOptions.notificationChannelEnabledIDs,
    notificationChannelKindsByID: relationOptions.notificationChannelKindsByID,
    policy,
    reportNotificationChannelIDs: relationOptions.reportNotificationChannelIDs,
    templates: diagnosisToolTemplates,
  });
}

function policyEnablementSummaryDetail(
  readiness: ReturnType<typeof reportWorkflowPolicyEnablementReadiness>,
): string {
  return readiness.blockers[0] ?? readiness.warnings[0] ?? readiness.detail;
}

function policyRepairSummaryDetail(
  blueprint: ReturnType<typeof reportWorkflowPolicyRepairBlueprint>,
): string {
  return blueprint.actions[0]?.detail ?? blueprint.detail;
}

function ImpactSummary({
  result,
}: {
  result?: ReportWorkflowPolicyImpactPreviewResult;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  if (!result) {
    return <Typography.Text type="secondary">{t("notPreviewed")}</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Tag color={impactStatusColor(result.status)}>{localizeWorkflowPolicyText(result.status, t)}</Tag>
      <Typography.Text type="secondary">
        {t("impactCounts", { events: result.events_matched, groups: result.groups_estimated })}
      </Typography.Text>
    </Space>
  );
}

type ImpactPreviewModalProps = {
  onCancel: () => void;
  preview: ImpactPreviewState | null;
};

function ImpactPreviewModal({ onCancel, preview }: ImpactPreviewModalProps) {
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  const result = preview?.result ?? null;
  const reasons =
    result?.reason_codes.map((reason) =>
      reportWorkflowPolicyImpactReason(reason),
    ) ?? [];
  const diagnosisEstimate =
    result === null
      ? null
      : reportWorkflowPolicyImpactDiagnosisEstimate(result);
  const reportChannelReadiness =
    result === null
      ? null
      : reportWorkflowPolicyImpactReportChannelReadiness(result);

  return (
    <Modal
      destroyOnHidden
      footer={
        <Button icon={<RadarChartOutlined />} onClick={onCancel} type="primary">
          {t("close")}
        </Button>
      }
      onCancel={onCancel}
      open={preview !== null}
      title={preview?.title ?? t("impactPreview")}
      width={920}
    >
      {result === null ? null : (
        <Space
          className="settings-impact-preview"
          direction="vertical"
          size="middle"
        >
          <Alert
            description={
              <Space direction="vertical" size={4}>
                <Typography.Text>{localizeWorkflowPolicyText(result.message, t)}</Typography.Text>
                <Space direction="vertical" size={6}>
                  {reasons.map((reason) => (
                    <Space key={reason.code} size={[6, 4]} wrap>
                      <Tooltip title={reason.code}>
                        <Tag color={reason.tagColor}>
                          {localizeWorkflowPolicyText(reason.label, t)}
                        </Tag>
                      </Tooltip>
                      <Typography.Text type="secondary">
                        {localizeWorkflowPolicyText(reason.detail, t)}
                      </Typography.Text>
                    </Space>
                  ))}
                </Space>
              </Space>
            }
            message={
              <Tag color={impactStatusColor(result.status)}>
                {localizeWorkflowPolicyText(result.status, t)}
              </Tag>
            }
            showIcon
            type={impactAlertType(result.status)}
          />

          <Row gutter={[12, 12]}>
            <Col sm={6} xs={24}>
              <Statistic title={t("eventsScanned")} value={result.events_scanned} />
            </Col>
            <Col sm={6} xs={24}>
              <Statistic title={t("eventsMatched")} value={result.events_matched} />
            </Col>
            <Col sm={6} xs={24}>
              <Statistic
                title={t("groupsEstimated")}
                value={result.groups_estimated}
              />
            </Col>
            <Col sm={6} xs={24}>
              <Statistic
                title={t("aiDiagnosis")}
                value={diagnosisEstimate?.value ?? "-"}
              />
            </Col>
          </Row>

          {diagnosisEstimate === null ? null : (
            <Alert
              description={localizeWorkflowPolicyText(diagnosisEstimate.detail, t)}
              message={
                <Space size={[6, 4]} wrap>
                  <Tag color={readinessTagColor(diagnosisEstimate.status)}>
                    {readinessLabel(diagnosisEstimate.status, t)}
                  </Tag>
                  <span>{localizeWorkflowPolicyText(diagnosisEstimate.label, t)}</span>
                </Space>
              }
              showIcon
              type={readinessAlertType(diagnosisEstimate.status)}
            />
          )}

          <Row gutter={[12, 12]}>
            <Col md={8} xs={24}>
              <ReadinessLine
                label={t("alertSource")}
                ready={result.alert_source_enabled}
                text={`#${result.alert_source_profile_id} ${result.alert_source_kind}/${result.alert_source_auth_mode}`}
              />
            </Col>
            <Col md={8} xs={24}>
              <ReadinessLine
                label={t("groupingPolicy")}
                ready={result.grouping_policy_enabled}
                text={`#${result.grouping_policy_id} ${result.grouping_dimension_keys.join(", ")}`}
              />
            </Col>
            <Col md={8} xs={24}>
              <ReadinessLine
                label={t("reportChannel")}
                ready={reportChannelReadiness?.ready ?? true}
                text={localizeWorkflowPolicyText(reportChannelReadiness?.text ?? "No report channel bound", t)}
              />
            </Col>
          </Row>

          <Table<ImpactPreviewGroup>
            columns={impactGroupColumns(locale, t)}
            dataSource={result.groups}
            locale={{ emptyText: <Empty description={t("noEstimatedGroups")} /> }}
            pagination={false}
            rowKey={(group) => group.group_key}
            scroll={{ x: 940 }}
            size="small"
          />
        </Space>
      )}
    </Modal>
  );
}

function ReadinessLine({
  label,
  ready,
  text,
}: {
  label: string;
  ready: boolean;
  text: string;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <Space direction="vertical" size={2}>
      <Typography.Text type="secondary">{label}</Typography.Text>
      <Space wrap>
        <Tag color={ready ? "green" : "red"}>{localizeWorkflowPolicyText(ready ? "ready" : "blocked", t)}</Tag>
        <Typography.Text>{text}</Typography.Text>
      </Space>
    </Space>
  );
}

function impactGroupColumns(
  locale: string,
  t: WorkflowPolicyTranslator,
): TableColumnsType<ImpactPreviewGroup> {
  return [
  {
    dataIndex: "dimensions",
    key: "dimensions",
    title: t("dimensions"),
    render: (_value, group) => <DimensionTags values={group.dimensions} />,
  },
  {
    dataIndex: "severity",
    key: "severity",
    title: t("severity"),
    render: (_value, group) => (
      <Tag color={severityColor(group.severity)}>{localizeWorkflowPolicyText(group.severity, t)}</Tag>
    ),
  },
  {
    dataIndex: "event_count",
    key: "event_count",
    title: t("events"),
  },
  {
    dataIndex: "first_seen_at",
    key: "first_seen_at",
    title: t("firstSeen"),
    render: (_value, group) => formatDateTime(group.first_seen_at, locale),
  },
  {
    dataIndex: "last_seen_at",
    key: "last_seen_at",
    title: t("lastSeen"),
    render: (_value, group) => formatDateTime(group.last_seen_at, locale),
  },
  {
    dataIndex: "event_ids",
    key: "event_ids",
    title: t("eventIds"),
    render: (_value, group) => (
      <Typography.Text className="settings-event-ids">
        {group.event_ids.join(", ")}
      </Typography.Text>
    ),
  },
  ];
}

type ReplayPolicyModalProps = {
  busy: boolean;
  form: FormInstance<ReportWorkflowPolicyReplayFormState>;
  onCancel: () => void;
  onSubmit: (values: ReportWorkflowPolicyReplayFormState) => void;
  policy: ReportWorkflowPolicy | null;
  result: ReportReplayTriggerResponse | null;
};

function ReplayPolicyModal({
  busy,
  form,
  onCancel,
  onSubmit,
  policy,
  result,
}: ReplayPolicyModalProps) {
  const t = useTranslations("WorkflowPolicySettings");
  const alertsT = useTranslations("Alerts");
  const proofTrace =
    result === null ? null : localizeReportReplayProofTrace(result, alertsT);

  return (
    <Modal
      destroyOnHidden
      footer={null}
      onCancel={onCancel}
      open={policy !== null}
      title={policy === null ? t("replayPolicy") : t("replayPolicyNumber", { id: policy.id })}
    >
      <Form<ReportWorkflowPolicyReplayFormState>
        disabled={busy}
        form={form}
        layout="vertical"
        onFinish={onSubmit}
      >
        <Row gutter={12}>
          <Col sm={12} xs={24}>
            <Form.Item
              label={t("windowStart")}
              name="windowStart"
              rules={[{ required: true, message: t("windowStartRequired") }]}
            >
              <Input autoComplete="off" placeholder="2026-06-05T08:00:00Z" />
            </Form.Item>
          </Col>
          <Col sm={12} xs={24}>
            <Form.Item
              label={t("windowEnd")}
              name="windowEnd"
              rules={[{ required: true, message: t("windowEndRequired") }]}
            >
              <Input autoComplete="off" placeholder="2026-06-05T09:00:00Z" />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item
          label={t("limit")}
          name="limit"
          rules={[{ required: true, message: t("limitRequired") }]}
        >
          <InputNumber
            max={100000}
            min={1}
            precision={0}
            style={{ width: "100%" }}
          />
        </Form.Item>
        <Form.Item label={t("correlationKey")} name="correlationKey">
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item label={t("workflowId")} name="workflowID">
          <Input autoComplete="off" />
        </Form.Item>

        {result === null ? null : (
          <Alert
            className="settings-action-result"
            message={result.started ? t("workflowAccepted") : t("replayCompleted")}
            showIcon
            type={result.started ? "success" : "warning"}
            description={
              <Space direction="vertical" size={2}>
                <Typography.Text>
                  {result.workflow_id === ""
                    ? t("noWorkflowStarted")
                    : `${result.workflow_id} / ${result.run_id}`}
                </Typography.Text>
                <Typography.Text
                  className="settings-event-ids"
                  copyable
                  type="secondary"
                >
                  {t("correlation", { key: result.correlation_key })}
                </Typography.Text>
                <Typography.Text type="secondary">
                  {t("replayCounts", { groups: result.stats.groups_built, snapshots: result.stats.snapshots_saved })}
                </Typography.Text>
                {result.auto_diagnosis ? (
                  <Typography.Text type="secondary">
                    {autoDiagnosisReplaySummary(result.auto_diagnosis, t)}
                  </Typography.Text>
                ) : null}
                {result.auto_diagnosis &&
                result.auto_diagnosis.rooms_skipped > 0 ? (
                  <Typography.Text type="secondary">
                    {t("retainedManualRooms", {
                      count: result.auto_diagnosis.rooms_skipped,
                    })}
                  </Typography.Text>
                ) : null}
                {proofTrace === null ? null : (
                  <ReplayProofTrace trace={proofTrace} />
                )}
              </Space>
            }
          />
        )}

        <Space wrap>
          <Button
            htmlType="submit"
            icon={<ThunderboltOutlined />}
            loading={busy}
            type="primary"
          >
            {t("startReplay")}
          </Button>
          <Button disabled={busy} onClick={onCancel} type="default">
            {t("close")}
          </Button>
        </Space>
      </Form>
    </Modal>
  );
}

function ReplayProofTrace({
  trace,
}: {
  trace: LocalizedAlertReplayProofTrace;
}) {
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div aria-label={t("proofTraceLabel")} className="settings-proof-outcome">
      <div className="settings-preview-header">
        <Typography.Text strong>{t("proofTrace")}</Typography.Text>
        <Tag color={readinessTagColor(trace.status)}>
          {readinessLabel(trace.status, t)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{trace.detail}</Typography.Text>
      <div className="workflow-automation-grid">
        {trace.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{item.title}</Typography.Text>
              <Tag color={readinessTagColor(item.status)}>
                {readinessLabel(item.status, t)}
              </Tag>
            </div>
            <Typography.Text strong>{item.value}</Typography.Text>
            <Typography.Text type="secondary">{item.detail}</Typography.Text>
            {item.actions && item.actions.length > 0 ? (
              <Space size={[4, 4]} wrap>
                {item.actions.map((action) => (
                  <Tooltip
                    key={`${item.title}:${action.href}`}
                    title={item.detail}
                  >
                    <Button
                      href={action.href}
                      icon={<RadarChartOutlined />}
                      size="small"
                      type="link"
                    >
                      {action.label}
                    </Button>
                  </Tooltip>
                ))}
              </Space>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );
}

function autoDiagnosisReplaySummary(
  autoDiagnosis: NonNullable<ReportReplayTriggerResponse["auto_diagnosis"]>,
  t: WorkflowPolicyTranslator,
): string {
  const confirmedSnapshots =
    autoDiagnosisConfirmedSnapshotCount(autoDiagnosis);
  return t("autoDiagnosisReplaySummary", {
    confirmed: confirmedSnapshots,
    policies: autoDiagnosis.policies_matched,
    rooms: autoDiagnosis.rooms_started,
    skipped: autoDiagnosis.rooms_skipped,
    snapshots: autoDiagnosis.snapshots,
  });
}

function impactStatusColor(
  status: ReportWorkflowPolicyImpactPreviewResult["status"],
) {
  switch (status) {
    case "ready":
      return "green";
    case "review":
      return "gold";
    case "blocked":
      return "red";
  }
}

function impactAlertType(
  status: ReportWorkflowPolicyImpactPreviewResult["status"],
) {
  switch (status) {
    case "ready":
      return "success";
    case "review":
      return "warning";
    case "blocked":
      return "error";
  }
}

function DimensionTags({ values }: { values: Record<string, string> }) {
  const t = useTranslations("WorkflowPolicySettings");
  const entries = Object.entries(values).sort(([left], [right]) =>
    left.localeCompare(right),
  );
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

function severityColor(severity: ImpactPreviewGroup["severity"]) {
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

const workflowPolicyRuntimeTextKeys = {
    "alert_storm": "runtimeText.alertStorm",
    "auto_room": "runtimeText.autoRoom",
    "blocked": "runtimeText.blocked",
    "cascade": "runtimeText.cascade",
    "critical": "runtimeText.critical",
    "diagnosis_close": "runtimeText.diagnosisClose",
    "diagnosis_consultation": "runtimeText.diagnosisConsultation",
    "disabled": "runtimeText.disabled",
    "info": "runtimeText.info",
    "pending": "runtimeText.pending",
    "ready": "runtimeText.ready",
    "report": "runtimeText.report",
    "review": "runtimeText.review",
    "single_alert": "runtimeText.singleAlert",
    "suggest_room": "runtimeText.suggestRoom",
    "unknown": "runtimeText.unknown",
    "warning": "runtimeText.warning",
    "enabled": "runtimeText.enabled",
    "Alert storm": "alertStorm",
    "Alert-storm": "alertStorm",
    "Cascade": "cascade",
    "Disabled": "disabled",
    "Single alert": "singleAlert",
    "Single-alert": "singleAlert",
    "Automatic": "runtimeText.automatic",
    "Report-only": "runtimeText.reportOnly",
    "Grouping policy": "runtimeText.groupingPolicy",
    "Notification channel": "runtimeText.notificationChannel",
    "Policy": "runtimeText.policy",
    "Report channel": "runtimeText.reportChannel",
    "Alert source": "runtimeText.alertSource",
    "Alert source profile": "runtimeText.alertSourceProfile",
    "Alertmanager webhook source": "runtimeText.alertmanagerWebhookSource",
    "Automatic diagnosis workflow": "runtimeText.automaticDiagnosisWorkflow",
    "Channel review": "runtimeText.channelReview",
    "Choose the Alertmanager source that will trigger this workflow.": "runtimeText.chooseAlertmanagerSource",
    "Configure channel": "runtimeText.configureChannel",
    "Create or select an enabled WeCom channel with report, diagnosis_consultation, and diagnosis_close scopes.": "runtimeText.createOrSelectEnabledWeComChannel",
    "Diagnosis evidence tools are not requested while AI follow-up is disabled.": "runtimeText.diagnosisEvidenceToolsNotRequested",
    "Diagnosis follow-up disabled.": "runtimeText.diagnosisFollowUpDisabled",
    "Diagnosis templates unavailable.": "runtimeText.diagnosisTemplatesUnavailable",
    "Edit channel": "runtimeText.editChannel",
    "Enable channel": "runtimeText.enableChannel",
    "Enable suggest_room or auto_room to check executable diagnosis tools.": "runtimeText.enableFollowUpToCheckTools",
    "Enterprise WeChat channel needs attention.": "runtimeText.enterpriseWechatChannelNeedsAttention",
    "Enterprise WeChat channel not selected.": "runtimeText.enterpriseWechatChannelNotSelected",
    "Enterprise WeChat delivery selected.": "runtimeText.enterpriseWechatDeliverySelected",
    "Evidence tools are not requested while diagnosis follow-up is disabled.": "runtimeText.evidenceToolsNotRequested",
    "Generic webhook delivery is supported; select a WeCom channel when operator group notification should land in Enterprise WeChat.": "runtimeText.genericWebhookDeliverySupported",
    "Grouping runs before report generation so related alerts share one report and consultation room.": "runtimeText.groupingRunsBeforeReportGeneration",
    "Impact preview needs valid required workflow fields.": "runtimeText.impactPreviewNeedsValidFields",
    "Manual replay uses the selected alert source without starting AI follow-up from webhooks.": "runtimeText.manualReplayUsesSelectedSource",
    "No handoff": "runtimeText.noHandoff",
    "No matching alert groups in this sample, so no diagnosis handoff is expected.": "runtimeText.noMatchingGroupsForHandoff",
    "No report channel": "runtimeText.noReportChannel",
    "Not selected": "runtimeText.notSelected",
    "Operator channel optional.": "runtimeText.operatorChannelOptional",
    "Replay is blocked until the policy can be saved and enabled.": "runtimeText.replayBlockedUntilEnabled",
    "Report and AI updates": "runtimeText.reportAndAiUpdates",
    "Report notification": "runtimeText.reportNotification",
    "Resolve blocked workflow configuration.": "runtimeText.resolveBlockedWorkflowConfiguration",
    "Resolve save blockers before this policy can be enabled.": "runtimeText.resolveSaveBlockers",
    "Review workflow warnings before enablement.": "runtimeText.reviewWorkflowWarnings",
    "Run draft or saved impact preview to estimate matched alert groups and expose blocked enablement reasons before replay.": "runtimeText.runImpactPreviewBeforeReplay",
    "Save changes, then enable this policy from the configured policies table.": "runtimeText.saveChangesThenEnable",
    "Save policy": "runtimeText.savePolicy",
    "Save the policy first, then enable it from the configured policies table.": "runtimeText.savePolicyFirstThenEnable",
    "Select a WeCom channel when final reports should notify the operator group through Enterprise WeChat.": "runtimeText.selectWeComForFinalReports",
    "Select or create an enabled grouping policy before saving this workflow.": "runtimeText.selectEnabledGroupingPolicy",
    "This policy does not start AI diagnosis from Alertmanager webhook deliveries.": "runtimeText.policyDoesNotStartAiDiagnosis",
    "Tool collection": "runtimeText.toolCollection",
    "Tool template data could not be loaded.": "runtimeText.toolTemplateDataUnavailable",
    "WeCom delivery and proof": "runtimeText.weComDeliveryAndProof",
    "active_alerts for the selected source": "runtimeText.activeAlertsForSelectedSource",
    "alert source": "runtimeText.alertSourceLower",
    "grouping policy": "runtimeText.groupingPolicyLower",
    "metric_query or metric_range_query": "runtimeText.metricQueryTools",
    "missing AI proof": "runtimeText.missingAiProof",
    "missing diagnosis_close": "runtimeText.missingDiagnosisClose",
    "missing diagnosis_consultation": "runtimeText.missingDiagnosisConsultation",
    "notification channel": "runtimeText.notificationChannelLower",
    "requires Enterprise WeChat": "runtimeText.requiresEnterpriseWechat",
    "Blocked": "runtimeText.blocked2",
    "WeCom": "runtimeText.wecom",
    "AI delivery proof missing": "runtimeText.aiDeliveryProofMissing",
    "Active alert and metric collection tools are enabled.": "runtimeText.activeAlertAndMetricCollectionToolsAreEnabled",
    "Active alert evidence tool": "runtimeText.activeAlertEvidenceTool",
    "Add AI scopes": "runtimeText.addAiScopes",
    "Add a Thanos Query or Prometheus metric evidence source, then use Recommended by sources to create metric_query or metric_range_query templates.": "runtimeText.addAThanosQueryOrPrometheusMetricEvidenceSourceThenUseRecommended",
    "Add a metric_query or metric_range_query template on the selected Prometheus-compatible source so AI can raise confidence with measured evidence.": "runtimeText.addAMetricQueryOrMetricRangeQueryTemplateOnTheSelected",
    "Add an active_alerts template bound to the workflow alert source so AI can confirm sibling firing alerts.": "runtimeText.addAnActiveAlertsTemplateBoundToTheWorkflowAlertSourceSo",
    "Add the diagnosis_close scope when auto_room should deliver close notifications.": "runtimeText.addTheDiagnosisCloseScopeWhenAutoRoomShouldDeliverCloseNotifications",
    "Add the diagnosis_consultation scope when auto_room should deliver AI diagnosis updates.": "runtimeText.addTheDiagnosisConsultationScopeWhenAutoRoomShouldDeliverAiDiagnosis",
    "Add the report delivery scope to the bound notification channel.": "runtimeText.addTheReportDeliveryScopeToTheBoundNotificationChannel",
    "Alert source disabled": "runtimeText.alertSourceDisabled",
    "Alertmanager webhook deliveries can ingest firing alerts and start automatic diagnosis rooms.": "runtimeText.alertmanagerWebhookDeliveriesCanIngestFiringAlertsAndStartAutomaticDiagnosisRooms",
    "Alertmanager webhook deliveries can ingest firing alerts; suggest_room still requires operator handoff.": "runtimeText.alertmanagerWebhookDeliveriesCanIngestFiringAlertsSuggestRoomStillRequiresOperator",
    "Automatic diagnosis room delivery requires an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes.": "runtimeText.automaticDiagnosisRoomDeliveryRequiresAnEnterpriseWechatChannelWithReportDiagnosis",
    "Automatic diagnosis room starts require an Alertmanager alert source because the webhook endpoint rejects non-Alertmanager profiles.": "runtimeText.automaticDiagnosisRoomStartsRequireAnAlertmanagerAlertSourceBecauseTheWebhook",
    "Automatic diagnosis rooms will not start until the blocked preview reasons are resolved.": "runtimeText.automaticDiagnosisRoomsWillNotStartUntilTheBlockedPreviewReasonsAre",
    "Bind a notification channel before enabling auto_room AI diagnosis updates.": "runtimeText.bindANotificationChannelBeforeEnablingAutoRoomAiDiagnosisUpdates",
    "Bind an Alertmanager alert source before using auto_room diagnosis follow-up.": "runtimeText.bindAnAlertmanagerAlertSourceBeforeUsingAutoRoomDiagnosisFollowUp",
    "Bind an enabled report channel with diagnosis_consultation and diagnosis_close scopes before using automatic diagnosis rooms.": "runtimeText.bindAnEnabledReportChannelWithDiagnosisConsultationAndDiagnosisCloseScopes",
    "Bound alert source must be enabled before workflow policy enablement.": "runtimeText.boundAlertSourceMustBeEnabledBeforeWorkflowPolicyEnablement",
    "Bound grouping policy must be enabled before workflow policy enablement.": "runtimeText.boundGroupingPolicyMustBeEnabledBeforeWorkflowPolicyEnablement",
    "Configuration bindings are usable and the bounded sample produced an impact estimate.": "runtimeText.configurationBindingsAreUsableAndTheBoundedSampleProducedAnImpactEstimate",
    "Configure metric source": "runtimeText.configureMetricSource",
    "Configure source": "runtimeText.configureSource",
    "Create AI channel": "runtimeText.createAiChannel",
    "Create active-alert tool": "runtimeText.createActiveAlertTool",
    "Create an enabled grouping policy before saving this workflow.": "runtimeText.createAnEnabledGroupingPolicyBeforeSavingThisWorkflow",
    "Create grouping": "runtimeText.createGrouping",
    "Create metric tool": "runtimeText.createMetricTool",
    "Create or select an enabled Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes, run AI diagnosis and close proof, then return to enable this workflow.": "runtimeText.createOrSelectAnEnabledEnterpriseWechatChannelWithReportDiagnosisConsultation",
    "Diagnosis follow-up is disabled for this policy.": "runtimeText.diagnosisFollowUpIsDisabledForThisPolicy",
    "Enable at least one active_alerts template and one metric template before relying on AI follow-up.": "runtimeText.enableAtLeastOneActiveAlertsTemplateAndOneMetricTemplateBefore",
    "Enable policy": "runtimeText.enablePolicy",
    "Enable the bound alert source before activating this workflow.": "runtimeText.enableTheBoundAlertSourceBeforeActivatingThisWorkflow",
    "Enable the bound alert source before relying on webhook ingestion.": "runtimeText.enableTheBoundAlertSourceBeforeRelyingOnWebhookIngestion",
    "Enable the bound grouping policy so sampled alerts can be grouped.": "runtimeText.enableTheBoundGroupingPolicySoSampledAlertsCanBeGrouped",
    "Enable the bound notification channel before report delivery.": "runtimeText.enableTheBoundNotificationChannelBeforeReportDelivery",
    "Enabled diagnosis templates are bound only to disabled or incompatible sources.": "runtimeText.enabledDiagnosisTemplatesAreBoundOnlyToDisabledOrIncompatibleSources",
    "Enterprise WeChat channel": "runtimeText.enterpriseWechatChannel",
    "Fix form": "runtimeText.fixForm",
    "Limit must be between 1 and 100000.": "runtimeText.limitMustBeBetween1And100000",
    "Maximum failed SubReports must be between 0 and 100000.": "runtimeText.maximumFailedSubreportsMustBeBetween0And100000",
    "Loaded matching automatic diagnosis workflows for retained Alertmanager proof.": "runtimeText.loadedMatchingAutomaticDiagnosisWorkflowsForRetainedAlertmanagerProof",
    "Manual replay": "runtimeText.manualReplay",
    "No channel": "runtimeText.noChannel",
    "No matching alert groups in this sample, so no automatic diagnosis room is expected.": "runtimeText.noMatchingAlertGroupsInThisSampleSoNoAutomaticDiagnosisRoom",
    "No notification channel profile is bound.": "runtimeText.noNotificationChannelProfileIsBound",
    "No rooms": "runtimeText.noRooms",
    "Open the selected Enterprise WeChat channel and run current AI diagnosis and diagnosis close sample tests before workflow policy enablement.": "runtimeText.openTheSelectedEnterpriseWechatChannelAndRunCurrentAiDiagnosisAnd",
    "Open the selected Enterprise WeChat channel, run the current AI diagnosis and diagnosis close sample tests, then return to enable this workflow.": "runtimeText.openTheSelectedEnterpriseWechatChannelRunTheCurrentAiDiagnosisAnd",
    "Policy bindings and diagnosis tool configuration are ready.": "runtimeText.policyBindingsAndDiagnosisToolConfigurationAreReady",
    "Policy name is required.": "runtimeText.policyNameIsRequired",
    "Policy name must be 120 characters or fewer.": "runtimeText.policyNameMustBe120CharactersOrFewer",
    "Prepared an automatic diagnosis workflow from the settings overview create action.": "runtimeText.preparedAnAutomaticDiagnosisWorkflowFromTheSettingsOverviewCreateAction",
    "Prepared an automatic diagnosis workflow that needs an enabled Alertmanager source.": "runtimeText.preparedAnAutomaticDiagnosisWorkflowThatNeedsAnEnabledAlertmanagerSource",
    "Prepared automatic AI diagnosis room handoff from the settings overview action.": "runtimeText.preparedAutomaticAiDiagnosisRoomHandoffFromTheSettingsOverviewAction",
    "Prepared automatic diagnosis room handoff from the settings overview action.": "runtimeText.preparedAutomaticDiagnosisRoomHandoffFromTheSettingsOverviewAction",
    "Prometheus sources support metric evidence, but they do not receive Alertmanager webhook deliveries.": "runtimeText.prometheusSourcesSupportMetricEvidenceButTheyDoNotReceiveAlertmanagerWebhook",
    "Recent bounded samples did not match this source and grouping configuration.": "runtimeText.recentBoundedSamplesDidNotMatchThisSourceAndGroupingConfiguration",
    "Report only": "runtimeText.reportOnly",
    "Review grouping": "runtimeText.reviewGrouping",
    "Review source": "runtimeText.reviewSource",
    "Run AI proof": "runtimeText.runAiProof",
    "Run current AI diagnosis and diagnosis close sample tests for the bound Enterprise WeChat channel.": "runtimeText.runCurrentAiDiagnosisAndDiagnosisCloseSampleTestsForTheBound",
    "Select Auto room to enable automatic AI consultation readiness checks.": "runtimeText.selectAutoRoomToEnableAutomaticAiConsultationReadinessChecks",
    "Select the alert source that receives the Alertmanager webhook.": "runtimeText.selectTheAlertSourceThatReceivesTheAlertmanagerWebhook",
    "Selected notification channel can deliver final report notifications.": "runtimeText.selectedNotificationChannelCanDeliverFinalReportNotifications",
    "Selected notification channel can deliver reports, auto-room AI diagnosis updates, and close notifications.": "runtimeText.selectedNotificationChannelCanDeliverReportsAutoRoomAiDiagnosisUpdatesAnd",
    "Selected notification channel must be enabled before workflow policy enablement.": "runtimeText.selectedNotificationChannelMustBeEnabledBeforeWorkflowPolicyEnablement",
    "Switch to WeCom": "runtimeText.switchToWecom",
    "Thanos Rule active-alert sources can provide firing-alert evidence, but automatic diagnosis room starts require an Alertmanager webhook source. Select or create an Alertmanager source for workflow intake, then keep Thanos Rule for active_alerts evidence templates.": "runtimeText.thanosRuleActiveAlertSourcesCanProvideFiringAlertEvidenceButAutomatic",
    "This policy does not request AI diagnosis handoff for matched alert groups.": "runtimeText.thisPolicyDoesNotRequestAiDiagnosisHandoffForMatchedAlertGroups",
    "Use a trigger mode supported by impact preview before enabling this policy.": "runtimeText.useATriggerModeSupportedByImpactPreviewBeforeEnablingThisPolicy",
    "Use an Enterprise WeChat channel before enabling auto_room AI diagnosis updates.": "runtimeText.useAnEnterpriseWechatChannelBeforeEnablingAutoRoomAiDiagnosisUpdates",
    "Use an Enterprise WeChat channel for automatic diagnosis room delivery, then run AI diagnosis and close proof before enablement.": "runtimeText.useAnEnterpriseWechatChannelForAutomaticDiagnosisRoomDeliveryThenRun",
    "Webhook firing alerts": "runtimeText.webhookFiringAlerts",
    "A handoff is retained for an operator to create the AI diagnosis room.": "runtimeText.aHandoffIsRetainedForAnOperatorToCreateTheAiDiagnosis",
    "Alertmanager alerts can produce evidence, start AI diagnosis rooms, and notify operators.": "runtimeText.alertmanagerAlertsCanProduceEvidenceStartAiDiagnosisRoomsAndNotifyOperators",
    "Alerts can prepare an AI handoff, but an operator still starts the diagnosis room.": "runtimeText.alertsCanPrepareAnAiHandoffButAnOperatorStillStartsThe",
    "All required bindings are selected; save the policy, run impact preview, then replay a bounded window.": "runtimeText.allRequiredBindingsAreSelectedSaveThePolicyRunImpactPreviewThen",
    "Auto-room path blocked.": "runtimeText.autoRoomPathBlocked",
    "Auto-room path needs review.": "runtimeText.autoRoomPathNeedsReview",
    "Auto-room path pending.": "runtimeText.autoRoomPathPending",
    "Auto-room path ready.": "runtimeText.autoRoomPathReady",
    "Complete the required automatic diagnosis selections before enabling this path.": "runtimeText.completeTheRequiredAutomaticDiagnosisSelectionsBeforeEnablingThisPath",
    "Enterprise WeChat can receive final report delivery while AI room handoff remains operator-controlled.": "runtimeText.enterpriseWechatCanReceiveFinalReportDeliveryWhileAiRoomHandoffRemains",
    "Enterprise WeChat can receive final report delivery without starting or suggesting AI diagnosis rooms.": "runtimeText.enterpriseWechatCanReceiveFinalReportDeliveryWithoutStartingOrSuggestingAi",
    "Enterprise WeChat can receive final report delivery, AI diagnosis updates, final-ready notices, and close notifications.": "runtimeText.enterpriseWechatCanReceiveFinalReportDeliveryAiDiagnosisUpdatesFinalReady",
    "Matching Alertmanager webhooks can start AI diagnosis rooms, collect evidence, and notify the operator channel.": "runtimeText.matchingAlertmanagerWebhooksCanStartAiDiagnosisRoomsCollectEvidenceAndNotify",
    "No AI diagnosis room will be suggested or started by this policy.": "runtimeText.noAiDiagnosisRoomWillBeSuggestedOrStartedByThisPolicy",
    "No diagnosis room will be suggested or started by this policy.": "runtimeText.noDiagnosisRoomWillBeSuggestedOrStartedByThisPolicy",
    "No report notification channel is bound.": "runtimeText.noReportNotificationChannelIsBound",
    "Operator handoff": "runtimeText.operatorHandoff",
    "Resolve blocked bindings before relying on this workflow automation path.": "runtimeText.resolveBlockedBindingsBeforeRelyingOnThisWorkflowAutomationPath",
    "Resolve blocked intake, evidence, or notification requirements before automatic diagnosis rooms can run.": "runtimeText.resolveBlockedIntakeEvidenceOrNotificationRequirementsBeforeAutomaticDiagnosisRoomsCan",
    "Review the retained handoff or delivery gap before treating this workflow as fully automated.": "runtimeText.reviewTheRetainedHandoffOrDeliveryGapBeforeTreatingThisWorkflowAs",
    "Save the policy, preview impact, then replay a bounded window after enablement.": "runtimeText.saveThePolicyPreviewImpactThenReplayABoundedWindowAfterEnablement",
    "The automatic diagnosis path can be saved, but one or more operator-facing choices need review before production use.": "runtimeText.theAutomaticDiagnosisPathCanBeSavedButOneOrMoreOperator",
    "This matching AI room workflow is already enabled.": "runtimeText.thisMatchingAiRoomWorkflowIsAlreadyEnabled",
    "This matching AI room workflow is ready to enable.": "runtimeText.thisMatchingAiRoomWorkflowIsReadyToEnable",
    "This policy generates report workflow output without AI diagnosis-room automation.": "runtimeText.thisPolicyGeneratesReportWorkflowOutputWithoutAiDiagnosisRoomAutomation",
    "Webhook auto-room": "runtimeText.webhookAutoRoom",
    "Webhook handoff": "runtimeText.webhookHandoff",
    "Workflow policy draft is ready for the next operator action.": "runtimeText.workflowPolicyDraftIsReadyForTheNextOperatorAction",
    "Workflow setup actions are ready.": "runtimeText.workflowSetupActionsAreReady",
    "Workflow setup blocked.": "runtimeText.workflowSetupBlocked",
    "Workflow setup needs review.": "runtimeText.workflowSetupNeedsReview",
    "Workflow setup pending.": "runtimeText.workflowSetupPending",
    "Workflow setup ready.": "runtimeText.workflowSetupReady",
    "AI consultation": "runtimeText.aiConsultation",
    "AI delivery proof": "runtimeText.aiDeliveryProof",
    "AI delivery proof missing.": "runtimeText.aiDeliveryProofMissing2",
    "AI diagnosis disabled.": "runtimeText.aiDiagnosisDisabled",
    "AI evidence": "runtimeText.aiEvidence",
    "AI handoff": "runtimeText.aiHandoff",
    "AI room": "runtimeText.aiRoom",
    "Alert grouping policy": "runtimeText.alertGroupingPolicy",
    "Alert intake": "runtimeText.alertIntake",
    "Alert source disabled.": "runtimeText.alertSourceDisabled2",
    "Alert source required.": "runtimeText.alertSourceRequired",
    "Alertmanager intake": "runtimeText.alertmanagerIntake",
    "Alertmanager source required": "runtimeText.alertmanagerSourceRequired",
    "Alertmanager webhook source required.": "runtimeText.alertmanagerWebhookSourceRequired",
    "Auto-room delivery blocked.": "runtimeText.autoRoomDeliveryBlocked",
    "Automatic diagnosis blocked.": "runtimeText.automaticDiagnosisBlocked",
    "Automatic diagnosis rooms disabled.": "runtimeText.automaticDiagnosisRoomsDisabled",
    "Automatic diagnosis rooms estimated.": "runtimeText.automaticDiagnosisRoomsEstimated",
    "Bound alert source": "runtimeText.boundAlertSource",
    "Bound grouping policy": "runtimeText.boundGroupingPolicy",
    "Delivery": "runtimeText.delivery",
    "Delivery scopes": "runtimeText.deliveryScopes",
    "Diagnosis close scope missing": "runtimeText.diagnosisCloseScopeMissing",
    "Diagnosis consultation scope missing": "runtimeText.diagnosisConsultationScopeMissing",
    "Diagnosis room starts automatically": "runtimeText.diagnosisRoomStartsAutomatically",
    "Diagnosis room suggested": "runtimeText.diagnosisRoomSuggested",
    "Diagnosis tools need review.": "runtimeText.diagnosisToolsNeedReview",
    "Enterprise WeChat channel required.": "runtimeText.enterpriseWechatChannelRequired",
    "Enterprise WeChat required": "runtimeText.enterpriseWechatRequired",
    "Evidence": "runtimeText.evidence",
    "Evidence collection": "runtimeText.evidenceCollection",
    "Executable diagnosis tools ready.": "runtimeText.executableDiagnosisToolsReady",
    "Follow-up disabled": "runtimeText.followUpDisabled",
    "Generic webhook selected.": "runtimeText.genericWebhookSelected",
    "Grouping": "runtimeText.grouping",
    "Grouping policy disabled": "runtimeText.groupingPolicyDisabled",
    "Grouping rule": "runtimeText.groupingRule",
    "Impact not previewed": "runtimeText.impactNotPreviewed",
    "Impact preview": "runtimeText.impactPreview",
    "Metric evidence source": "runtimeText.metricEvidenceSource",
    "Metric evidence tool": "runtimeText.metricEvidenceTool",
    "No Alertmanager webhook ingress.": "runtimeText.noAlertmanagerWebhookIngress",
    "No automatic rooms expected.": "runtimeText.noAutomaticRoomsExpected",
    "No enabled diagnosis tools.": "runtimeText.noEnabledDiagnosisTools",
    "No matching events": "runtimeText.noMatchingEvents",
    "No report channel bound": "runtimeText.noReportChannelBound",
    "No report channel selected.": "runtimeText.noReportChannelSelected",
    "No usable diagnosis tools.": "runtimeText.noUsableDiagnosisTools",
    "Notification": "runtimeText.notification",
    "Notification channel disabled": "runtimeText.notificationChannelDisabled",
    "Notification channel disabled.": "runtimeText.notificationChannelDisabled2",
    "Notification channel required": "runtimeText.notificationChannelRequired",
    "Notification channel scope mismatch.": "runtimeText.notificationChannelScopeMismatch",
    "Operator channel": "runtimeText.operatorChannel",
    "Operator handoff retained.": "runtimeText.operatorHandoffRetained",
    "Operator notification": "runtimeText.operatorNotification",
    "Policy can be enabled after review.": "runtimeText.policyCanBeEnabledAfterReview",
    "Policy can be enabled.": "runtimeText.policyCanBeEnabled",
    "Policy cannot be enabled.": "runtimeText.policyCannotBeEnabled",
    "Preview ready": "runtimeText.previewReady",
    "Proof": "runtimeText.proof",
    "Replay window": "runtimeText.replayWindow",
    "Report and AI-room channel": "runtimeText.reportAndAiRoomChannel",
    "Report and auto-room delivery ready.": "runtimeText.reportAndAutoRoomDeliveryReady",
    "Report delivery ready.": "runtimeText.reportDeliveryReady",
    "Report notification channel": "runtimeText.reportNotificationChannel",
    "Report scope missing": "runtimeText.reportScopeMissing",
    "Select a grouping policy.": "runtimeText.selectAGroupingPolicy",
    "Select a valid report notification channel.": "runtimeText.selectAValidReportNotificationChannel",
    "Select an alert source.": "runtimeText.selectAnAlertSource",
    "Source": "runtimeText.source",
    "Trigger": "runtimeText.trigger",
    "Trigger mode unsupported": "runtimeText.triggerModeUnsupported",
    "Webhook": "runtimeText.webhook",
    "Webhook auto-room ingress blocked.": "runtimeText.webhookAutoRoomIngressBlocked",
    "Webhook auto-room ingress ready.": "runtimeText.webhookAutoRoomIngressReady",
    "Webhook ingest ready.": "runtimeText.webhookIngestReady",
    "Webhook ingress not used.": "runtimeText.webhookIngressNotUsed",
    "Window end is required.": "runtimeText.windowEndIsRequired",
    "Window end must be a valid date-time.": "runtimeText.windowEndMustBeAValidDateTime",
    "Window end must be after window start.": "runtimeText.windowEndMustBeAfterWindowStart",
    "Window start is required.": "runtimeText.windowStartIsRequired",
    "Window start must be a valid date-time.": "runtimeText.windowStartMustBeAValidDateTime",
    "Workflow policy fields": "runtimeText.workflowPolicyFields",
} as const;

export function localizeWorkflowPolicyText(value: string, t: WorkflowPolicyTranslator): string {
  const key =
    workflowPolicyRuntimeTextKeys[
      value as keyof typeof workflowPolicyRuntimeTextKeys
    ];
  if (key !== undefined) {
    return t(key);
  }
  let match = value.match(/^Current user is not authorized to manage report workflow policy #(\d+)\.$/);
  if (match) {
    return t("runtimePattern.unauthorizedPolicy", { id: match[1]! });
  }
  match = value.match(/^Current user is not authorized to (.+)\.$/);
  if (match) {
    return t("runtimePattern.unauthorizedAction", {
      action: localizeWorkflowPolicyAuthorizationAction(match[1]!, t),
    });
  }
  match = value.match(/^(\d+) groups \/ (\d+) events$/);
  if (match) {
    return t("runtimePattern.groupEventCount", {
      events: Number(match[2]),
      groups: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) matching AI room workflows? found;?(.*)$/);
  if (match) {
    const detail = match[2]?.trim() ?? "";
    return detail === ""
      ? t("runtimePattern.matchingWorkflows", { count: Number(match[1]) })
      : t("runtimePattern.matchingWorkflowsWithDetail", {
          count: Number(match[1]),
          detail: localizeWorkflowPolicyText(detail, t),
        });
  }
  match = value.match(/^AI diagnosis: (.+)$/);
  if (match) {
    return t("runtimePattern.aiDiagnosis", { detail: match[1]! });
  }
  match = value.match(/^Alert sources failed to load: (.+)\.$/);
  if (match) {
    return t("runtimePattern.alertSourcesFailed", { error: match[1]! });
  }
  match = value.match(/^Grouping policies failed to load: (.+)\.$/);
  if (match) {
    return t("runtimePattern.groupingPoliciesFailed", { error: match[1]! });
  }
  match = value.match(/^Notification channels failed to load: (.+)\.$/);
  if (match) {
    return t("runtimePattern.notificationChannelsFailed", {
      error: match[1]!,
    });
  }
  match = value.match(/^(\d+) active alert \/ (\d+) metric$/);
  if (match) {
    return t("runtimePattern.diagnosisToolCount", {
      alerts: Number(match[1]),
      metrics: Number(match[2]),
    });
  }
  match = value.match(/^(\d+) estimated alert groups? can start automatic AI diagnosis rooms when this policy is replayed or receives matching Alertmanager webhooks\.$/);
  if (match) {
    return t("runtimePattern.estimatedGroups", { count: Number(match[1]) });
  }
  match = value.match(/^(\d+) rooms?$/);
  if (match) {
    return t("runtimePattern.roomCount", { count: Number(match[1]) });
  }
  match = value.match(/^Add (.+) scope, run AI delivery proof, then return to enable this workflow\.$/);
  if (match) {
    return t("runtimePattern.addScopes", {
      scopes: match[1]!
        .split(" and ")
        .map((scope) => localizeWorkflowPolicyText(scope, t))
        .join(" / "),
    });
  }
  match = value.match(/^Missing (.+)\.$/);
  if (match) {
    return t("runtimePattern.missingItems", {
      items: match[1]!
        .split(" and ")
        .map((item) => localizeWorkflowPolicyText(item, t))
        .join(" / "),
    });
  }
  match = value.match(/^Selected notification channel is missing (.+) scope\.$/);
  if (match) {
    return t("runtimePattern.selectedChannelMissingScopes", {
      scopes: match[1]!
        .split(" and ")
        .map((scope) => localizeWorkflowPolicyText(scope, t))
        .join(" / "),
    });
  }
  match = value.match(/^(\d+) blocking setup actions? must be resolved before this workflow is ready\.$/);
  if (match) {
    return t("runtimePattern.blockingActions", { count: Number(match[1]) });
  }
  match = value.match(/^(\d+) setup actions? remain before the workflow can be exercised\.$/);
  if (match) {
    return t("runtimePattern.remainingActions", { count: Number(match[1]) });
  }
  match = value.match(/^(\d+) setup actions? should be reviewed before enablement\.$/);
  if (match) {
    return t("runtimePattern.reviewActions", { count: Number(match[1]) });
  }
  match = value.match(/^Deliver report notifications through (.+)\.$/);
  if (match) {
    return t("runtimePattern.deliverThrough", { channel: match[1]! });
  }
  match = value.match(/^This matching AI room workflow can be enabled after review: (.+)$/);
  if (match) {
    return t("runtimePattern.matchingWorkflowAfterReview", {
      detail: localizeWorkflowPolicyText(match[1]!, t),
    });
  }
  match = value.match(/^unselected (.+)$/);
  if (match) {
    return t("runtimePattern.unselected", {
      item: localizeWorkflowPolicyText(match[1]!, t),
    });
  }
  match = value.match(/^AI room will be suggested for operator handoff\. (.+)$/);
  if (match) {
    return t("runtimePattern.aiRoomSuggested", {
      detail: localizeWorkflowPolicyText(match[1]!, t),
    });
  }
  match = value.match(/^Saves (.+) for (.+) grouped by (.+)\.$/);
  if (match) {
    return t("runtimePattern.savesPolicyBindings", {
      grouping: localizeWorkflowPolicyResourceLabel(match[3]!, t),
      name: match[1]!,
      source: localizeWorkflowPolicyResourceLabel(match[2]!, t),
    });
  }
  match = value.match(/^Enable after reviewing: (.+)$/);
  if (match) {
    return t("runtimePattern.enableAfterReviewing", {
      detail: localizeWorkflowPolicyText(match[1]!, t),
    });
  }
  match = value.match(/^Update policy #(\d+)$/);
  if (match) {
    return t("runtimePattern.updatePolicy", { id: match[1]! });
  }
  match = value.match(/^Replay bounded (.+) windows after the policy is enabled\.$/);
  if (match) {
    return t("runtimePattern.replayBoundedScenario", {
      scenario: localizeWorkflowPolicyText(match[1]!, t),
    });
  }
  match = value.match(/^(.+) reports use (.+) and (.+)\.$/);
  if (match) {
    return t("runtimePattern.reportsUseSourceAndGrouping", {
      grouping: localizeWorkflowPolicyResourceLabel(match[3]!, t),
      scenario: localizeWorkflowPolicyText(match[1]!, t),
      source: localizeWorkflowPolicyResourceLabel(match[2]!, t),
    });
  }
  match = value.match(/^(\d+) estimated alert groups? can be retained for operator-created diagnosis rooms\.$/);
  if (match) {
    return t("runtimePattern.estimatedHandoffGroups", {
      count: Number(match[1]),
    });
  }
  match = value.match(/^(\d+) handoffs?$/);
  if (match) {
    return t("runtimePattern.handoffCount", { count: Number(match[1]) });
  }
  match = value.match(/^#(\d+) disabled$/);
  if (match) {
    return t("runtimePattern.channelDisabled", { id: match[1]! });
  }
  match = value.match(/^#(\d+) missing (.+)$/);
  if (match) {
    return t("runtimePattern.channelMissing", {
      detail: match[2]!
        .split(", ")
        .map((item) => localizeWorkflowPolicyScope(item, t))
        .join(" / "),
      id: match[1]!,
    });
  }
  match = value.match(/^#(\d+) scopes and proof ready$/);
  if (match) {
    return t("runtimePattern.channelReady", { id: match[1]! });
  }
  match = value.match(/^(Alert source|Grouping policy|Notification channel|Policy|Report channel) #(\d+)$/);
  if (match) {
    return t("runtimePattern.resourceNumber", {
      id: match[2]!,
      resource: localizeWorkflowPolicyText(match[1]!, t),
    });
  }
  match = value.match(/^(.+): (.+)$/);
  if (match) {
    const detail = localizeWorkflowPolicyText(match[2]!, t);
    if (detail !== match[2]) {
      return t("runtimePattern.labeledDetail", {
        detail,
        label: match[1]!,
      });
    }
  }
  const sentences = value.match(/[^.!?]+[.!?]?/g)?.map((part) => part.trim()) ?? [];
  if (sentences.length > 1) {
    const localized = sentences.map((sentence) =>
      localizeWorkflowPolicyText(sentence, t),
    );
    if (localized.some((sentence, index) => sentence !== sentences[index])) {
      return localized.join(" ");
    }
  }
  return value;
}

function localizeWorkflowPolicyAuthorizationAction(
  value: string,
  t: WorkflowPolicyTranslator,
): string {
  return value
    .split(", ")
    .map((action) => {
      let match = action.match(/^read alert source #(\d+)$/);
      if (match) {
        return t("runtimePattern.readAlertSource", { id: match[1]! });
      }
      match = action.match(/^read grouping policy #(\d+)$/);
      if (match) {
        return t("runtimePattern.readGroupingPolicy", { id: match[1]! });
      }
      match = action.match(/^test notification channel #(\d+)$/);
      if (match) {
        return t("runtimePattern.testNotificationChannel", { id: match[1]! });
      }
      return action;
    })
    .join(t("runtimePattern.actionSeparator"));
}

function localizeWorkflowPolicyScope(
  value: string,
  t: WorkflowPolicyTranslator,
): string {
  switch (value) {
    case "report":
      return t("runtimeText.reportScope");
    case "diagnosis_consultation":
      return t("runtimeText.diagnosisConsultationScope");
    case "diagnosis_close":
      return t("runtimeText.diagnosisCloseScope");
    default:
      return localizeWorkflowPolicyText(value, t);
  }
}

function localizeWorkflowPolicyResourceLabel(
  value: string,
  t: WorkflowPolicyTranslator,
): string {
  const match = value.match(
    /^(alert source|grouping policy|notification channel) #(\d+)$/,
  );
  if (!match) {
    return value;
  }
  return t("runtimePattern.resourceNumber", {
    id: match[2]!,
    resource: localizeWorkflowPolicyText(match[1]!, t),
  });
}

function localizeWorkflowPolicyMessages(
  messages: readonly string[],
  t: WorkflowPolicyTranslator,
): string {
  return messages
    .map((message) => localizeWorkflowPolicyText(message, t))
    .join(" ");
}

function nullableDate(
  value: string | null,
  locale: string,
  t: WorkflowPolicyTranslator,
): string {
  if (value === null) {
    return t("catalog.notSet");
  }
  return formatDateTime(value, locale);
}
