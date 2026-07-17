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
  reportWorkflowPolicyReplayProofTrace,
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
  const locale = useLocale();
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
        locale,
      ),
    [alertSourcesResult, groupingPoliciesResult, locale, notificationChannelsResult],
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
      : `Current user is not authorized to manage report workflow policy #${editingID}.`;
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
        : "Current user is not authorized to create report workflow policies."
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
          message: localizeWorkflowPolicyText(policyBindingPermissionBlockReason, locale),
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
      alertSourceOptionsForFollowUp(selectedDiagnosisFollowUp, relationOptions, locale),
    [locale, relationOptions, selectedDiagnosisFollowUp],
  );
  const notificationChannelOptions = useMemo(
    () =>
      notificationChannelOptionsForFollowUp(
        selectedDiagnosisFollowUp,
        relationOptions,
        locale,
      ),
    [locale, relationOptions, selectedDiagnosisFollowUp],
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
      selectedMaxFailedSubReports,
      selectedGroupingPolicyID,
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
          localizeWorkflowPolicyText(policySavePermissionBlockReason, locale) ||
          t("notAuthorizedSave"),
      });
      return;
    }
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: localizeWorkflowPolicyText(parsed.message, locale) });
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
              detail: localizeWorkflowPolicyMessages(enablementBlockers, locale),
            })
          : reviewItems.length > 0
          ? t("savedWithReview", {
              detail: localizeWorkflowPolicyMessages(reviewItems, locale),
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
      setNotice({ kind: "error", message: localizeWorkflowPolicyText(readiness.detail, locale) });
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
          ? t("enabledWithReview", { detail: localizeWorkflowPolicyText(readiness.detail, locale) })
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
        status: localizeWorkflowPolicyText(previewed.data.status, locale),
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
      setNotice({ kind: "error", message: localizeWorkflowPolicyText(parsed.message, locale) });
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
        status: localizeWorkflowPolicyText(previewed.data.status, locale),
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
      setNotice({ kind: "error", message: localizeWorkflowPolicyText(parsed.message, locale) });
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
          description={localizeWorkflowPolicyText(launchNotice, locale)}
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
          description={localizeWorkflowPolicyMessages(relationOptions.warnings, locale)}
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
  locale: string,
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
      alertSources.map((source) => [source.id, alertSourceLabel(source, locale)]),
    ),
    alertSourceLabelsByID: new Map(
      alertSources.map((source) => [source.id, source.labels]),
    ),
    alertSourceOptions: alertSources.map((source) =>
      relationOption(source.id, alertSourceLabel(source, locale)),
    ),
    groupingPolicyEnabledIDs: new Set(
      groupingPolicies
        .filter((policy) => policy.enabled)
        .map((policy) => policy.id),
    ),
    groupingPolicyLabels: Object.fromEntries(
      groupingPolicies.map((policy) => [
        policy.id,
        groupingPolicyLabel(policy, locale),
      ]),
    ),
    groupingPolicyOptions: groupingPolicies.map((policy) =>
      relationOption(policy.id, groupingPolicyLabel(policy, locale)),
    ),
    notificationChannelLabels: Object.fromEntries(
      notificationChannels.map((channel) => [
        channel.id,
        notificationChannelLabel(channel, locale),
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
      relationOption(channel.id, notificationChannelLabel(channel, locale)),
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
  locale: string,
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
      locale,
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
  locale: string,
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
    const label = `${option.label} - ${hints.map((hint) => localizeWorkflowPolicyText(hint, locale)).join(", ")}`;
    return {
      ...option,
      disabled: state.disabled,
      label,
      title: label,
    };
  });
}

function alertSourceLabel(source: AlertSourceProfile, locale: string): string {
  return `#${source.id} ${source.name} (${source.kind}, ${enabledLabel(source.enabled, locale)})`;
}

function groupingPolicyLabel(policy: GroupingPolicy, locale: string): string {
  const dimensions =
    policy.dimension_keys.length === 0
      ? locale === "zh-CN" ? "无维度" : "no dimensions"
      : policy.dimension_keys.join(", ");
  return `#${policy.id} ${policy.name} (${dimensions}, ${enabledLabel(policy.enabled, locale)})`;
}

function notificationChannelLabel(channel: NotificationChannelProfile, locale: string): string {
  const scopes =
    channel.delivery_scopes.length === 0
      ? locale === "zh-CN" ? "无范围" : "no scopes"
      : channel.delivery_scopes.map((scope) => localizeWorkflowPolicyText(scope, locale)).join(", ");
  return `#${channel.id} ${channel.name} (${scopes}, ${enabledLabel(channel.enabled, locale)})`;
}

function enabledLabel(enabled: boolean, locale: string): string {
  return locale === "zh-CN"
    ? enabled ? "已启用" : "已停用"
    : enabled ? "enabled" : "disabled";
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
  const locale = useLocale();
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
          {readinessLabel(overallStatus, locale)}
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
                  {localizeWorkflowPolicyText(selectedPolicy.diagnosis_follow_up, locale)}
                </Tag>
                {impact ? (
                  <Tag color={impactStatusColor(impact.status)}>
                    {t("impactStatus", { status: localizeWorkflowPolicyText(impact.status, locale) })}
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
                  description: localizeWorkflowPolicyText(stage.detail, locale),
                  status: readinessStepStatus(stage.status),
                  title: localizeWorkflowPolicyText(stage.title, locale),
                }))}
                responsive={false}
              />
            </div>
          </>
        )}

        <Row aria-label={t("workflowCounters")} gutter={[12, 12]}>
          <ReadinessMetric
            label={t("roomReadyPolicies")}
            locale={locale}
            status="ready"
            value={activeRoomPolicies}
          />
          <ReadinessMetric
            label={t("reportDelivery")}
            locale={locale}
            status={reportDeliveryPolicies > 0 ? "ready" : "pending"}
            value={reportDeliveryPolicies}
          />
          <ReadinessMetric
            label={t("readyPreviews")}
            locale={locale}
            status={readyPreviews > 0 ? "ready" : "pending"}
            value={readyPreviews}
          />
          <ReadinessMetric
            label={t("blockedPreviews")}
            locale={locale}
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
  locale,
  status,
  value,
}: {
  label: string;
  locale: string;
  status: ReadinessStatus;
  value: number;
}) {
  return (
    <Col lg={6} sm={12} xs={24}>
      <div className="workflow-readiness-metric">
        <div className="workflow-readiness-metric-value">{value}</div>
        <div className="workflow-readiness-metric-footer">
          <Typography.Text className="muted">{label}</Typography.Text>
          <Tag color={readinessTagColor(status)}>{readinessLabel(status, locale)}</Tag>
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
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("toolReadiness")}
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status, locale)}
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
        <Typography.Text strong>{localizeWorkflowPolicyText(readiness.label, locale)}</Typography.Text>
        <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.detail, locale)}</Typography.Text>
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
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("automationOutcomeLabel")}
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>{t("automationOutcome")}</Typography.Text>
        <Tag color={readinessTagColor(outcome.status)}>
          {readinessLabel(outcome.status, locale)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(outcome.detail, locale)}</Typography.Text>
      <div className="workflow-automation-grid">
        {outcome.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{localizeWorkflowPolicyText(item.title, locale)}</Typography.Text>
              <Tag color={readinessTagColor(item.status)}>
                {readinessLabel(item.status, locale)}
              </Tag>
            </div>
            <Typography.Text strong>{localizeWorkflowPolicyText(item.value, locale)}</Typography.Text>
            <Typography.Text type="secondary">{localizeWorkflowPolicyText(item.detail, locale)}</Typography.Text>
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
  const locale = useLocale();
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
        <Typography.Text strong>{localizeWorkflowPolicyText(readiness.label, locale)}</Typography.Text>
        <Tag color={readinessTagColor(readiness.status)}>
          {readinessLabel(readiness.status, locale)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.detail, locale)}</Typography.Text>
      {readiness.items.length > 0 ? (
        <Steps
          current={currentStep === -1 ? readiness.items.length : currentStep}
          direction="vertical"
          items={readiness.items.map((item) => ({
            description: (
              <Space direction="vertical" size={2}>
                <Typography.Text strong>{localizeWorkflowPolicyText(item.value, locale)}</Typography.Text>
                <Typography.Text type="secondary">
                  {localizeWorkflowPolicyText(item.detail, locale)}
                </Typography.Text>
              </Space>
            ),
            status: readinessStepStatus(item.status),
            title: localizeWorkflowPolicyText(item.title, locale),
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
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("setupBlueprintLabel")}
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>{t("setupBlueprint")}</Typography.Text>
        <Tag color={readinessTagColor(blueprint.status)}>
          {readinessLabel(blueprint.status, locale)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(blueprint.detail, locale)}</Typography.Text>
      <div
        aria-label={t("setupChain")}
        className="workflow-automation-grid"
      >
        {blueprint.phases.map((phase) => (
          <div className="workflow-automation-item" key={phase.key}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">
                {localizeWorkflowPolicyText(phase.title, locale)}
              </Typography.Text>
              <Tag color={readinessTagColor(phase.status)}>
                {readinessLabel(phase.status, locale)}
              </Tag>
            </div>
            <Typography.Text strong>{localizeWorkflowPolicyText(phase.value, locale)}</Typography.Text>
            <Typography.Text type="secondary">{localizeWorkflowPolicyText(phase.detail, locale)}</Typography.Text>
          </div>
        ))}
      </div>
      {blueprint.actions.length === 0 ? (
        <Alert
          description={t("savePreviewReplay")}
          message={localizeWorkflowPolicyText(blueprint.label, locale)}
          showIcon
          type="success"
        />
      ) : (
        <div className="workflow-automation-grid">
          {blueprint.actions.map((action) => (
            <div className="workflow-automation-item" key={action.key}>
              <div className="workflow-automation-item-header">
                <Typography.Text className="muted">
                  {localizeWorkflowPolicyText(action.title, locale)}
                </Typography.Text>
                <Tag color={readinessTagColor(action.status)}>
                  {readinessLabel(action.status, locale)}
                </Tag>
              </div>
              <Typography.Text type="secondary">
                {localizeWorkflowPolicyText(action.detail, locale)}
              </Typography.Text>
              {action.actionHref ? (
                <Button href={action.actionHref} size="small" type="link">
                  {localizeWorkflowPolicyText(action.actionLabel, locale)}
                </Button>
              ) : (
                <Typography.Text className="muted">
                  {localizeWorkflowPolicyText(action.actionLabel, locale)}
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
  const locale = useLocale();
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
          {readinessLabel(plan.status, locale)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(plan.detail, locale)}</Typography.Text>
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
          title: localizeWorkflowPolicyText(step.title, locale),
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
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  if (step.title !== "Impact preview") {
    return localizeWorkflowPolicyText(step.detail, locale);
  }
  const savedLoading = policy !== null && impactingID === policy.id;
  const draftPreviewDisabled =
    step.status === "blocked" || impactingID !== null || !canPreviewDraftImpact;
  const savedPreviewDisabled =
    draftImpacting || (impactingID !== null && !savedLoading) || !canPreviewPolicy;

  return (
    <Space direction="vertical" size={6}>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(step.detail, locale)}</Typography.Text>
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
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("webhookReadiness")}
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status, locale)}
          </Tag>
          <Tag color="geekblue">{t("alertmanagerWebhook")}</Tag>
        </Space>
        <Typography.Text strong>{localizeWorkflowPolicyText(readiness.label, locale)}</Typography.Text>
        <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.detail, locale)}</Typography.Text>
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
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div
      aria-label={t("channelReadiness")}
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status, locale)}
          </Tag>
          <Tag color={operatorChannelTagColor(operatorReadiness)}>
            {operatorReadiness.kindLabel}
          </Tag>
          {selectedChannel === null ? null : (
            <Tag color={selectedChannel.enabled ? "green" : "default"}>
              {selectedChannel.enabled ? t("enabled") : t("disabled")}
            </Tag>
          )}
          <Tag color="blue">{t("requiredScopes", { scopes: readiness.requiredScopes.map((scope) => localizeWorkflowPolicyText(scope, locale)).join(", ") })}</Tag>
          {readiness.missingScopes.length > 0 ? (
            <Tag color="red">{t("missingScopes", { scopes: readiness.missingScopes.map((scope) => localizeWorkflowPolicyText(scope, locale)).join(", ") })}</Tag>
          ) : null}
        </Space>
        <Typography.Text strong>{localizeWorkflowPolicyText(readiness.label, locale)}</Typography.Text>
        <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.detail, locale)}</Typography.Text>
        <Typography.Text type="secondary">
          {localizeWorkflowPolicyText(operatorReadiness.detail, locale)}
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

function readinessLabel(status: ReadinessStatus, locale = "en"): string {
  switch (status) {
    case "ready":
      return locale === "zh-CN" ? "就绪" : "Ready";
    case "review":
      return locale === "zh-CN" ? "需检查" : "Review";
    case "pending":
      return locale === "zh-CN" ? "等待中" : "Pending";
    case "blocked":
      return locale === "zh-CN" ? "已阻塞" : "Blocked";
  }
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
  const locale = useLocale();
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
            {localizeWorkflowPolicyText(workflowReturnCandidateSummary(candidates), locale)}
          </Typography.Text>
          <Space size={4} wrap>
            {candidates.slice(0, 4).map((candidate) => (
              <Tooltip key={candidate.policy.id} title={localizeWorkflowPolicyText(candidate.detail, locale)}>
                <Tag color={workflowReturnCandidateTagColor(candidate.action)}>
                  #{candidate.policy.id} {candidate.policy.name}:{" "}
                  {localizeWorkflowPolicyText(workflowReturnCandidateLabel(candidate.action), locale)}
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
  const parts = [
    `${candidates.length} matching AI room workflow${candidates.length === 1 ? "" : "s"} found`,
    enableable > 0 ? `${enableable} can be enabled` : "",
    enabled > 0 ? `${enabled} already enabled` : "",
    blocked > 0 ? `${blocked} blocked` : "",
  ].filter((part) => part !== "");
  return parts.join("; ") + ".";
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
        <Tag>{localizeWorkflowPolicyText(scenario, locale)}</Tag>
      ),
    },
    {
      dataIndex: "diagnosis_follow_up",
      key: "followup",
      title: t("followUp"),
      render: (mode: ReportWorkflowPolicy["diagnosis_follow_up"]) => (
        <Tag color={followUpTagColor(mode)}>{localizeWorkflowPolicyText(mode, locale)}</Tag>
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
              ? nullableDate(policy.enabled_at, locale)
              : nullableDate(policy.disabled_at, locale)}
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
  const locale = useLocale();
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
          {readinessLabel(blueprint.status, locale)}
        </Tag>
        <Typography.Text type="secondary">{t("noRepair")}</Typography.Text>
      </Space>
    );
  }
  return (
    <Space direction="vertical" size={4}>
      <Space wrap>
        <Tag color={readinessTagColor(blueprint.status)}>
          {readinessLabel(blueprint.status, locale)}
        </Tag>
        <Tooltip title={localizeWorkflowPolicyText(blueprint.detail, locale)}>
          <Typography.Text type="secondary">{localizeWorkflowPolicyText(blueprint.label, locale)}</Typography.Text>
        </Tooltip>
      </Space>
      <Typography.Text type="secondary">
        {localizeWorkflowPolicyText(policyRepairSummaryDetail(blueprint), locale)}
      </Typography.Text>
      {visibleLinkedActions.map((action) => (
        <Tooltip key={action.key} title={localizeWorkflowPolicyText(action.detail, locale)}>
          <Button
            href={action.actionHref}
            icon={<ToolOutlined />}
            size="small"
            type="link"
          >
            {localizeWorkflowPolicyText(action.actionLabel, locale)}
          </Button>
        </Tooltip>
      ))}
      {unlinkedActionCount > 0 ? (
        <Tooltip title={localizeWorkflowPolicyText(blueprint.detail, locale)}>
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
  const locale = useLocale();
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
    <Tooltip title={localizeWorkflowPolicyText(readiness.detail, locale)}>
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
  const locale = useLocale();
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
        {readinessLabel(readiness.status, locale)}
      </Tag>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(readiness.label, locale)}</Typography.Text>
      <Typography.Text type="secondary">
        {localizeWorkflowPolicyText(policyEnablementSummaryDetail(readiness), locale)}
      </Typography.Text>
      {blockerCount + warningCount > 0 ? (
        <Space size={4} wrap>
          {blockerCount > 0 ? (
            <Tooltip title={localizeWorkflowPolicyMessages(readiness.blockers, locale)}>
              <Tag color="red">
                {t("blockerCount", { count: blockerCount })}
              </Tag>
            </Tooltip>
          ) : null}
          {warningCount > 0 ? (
            <Tooltip title={localizeWorkflowPolicyMessages(readiness.warnings, locale)}>
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
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  if (!result) {
    return <Typography.Text type="secondary">{t("notPreviewed")}</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Tag color={impactStatusColor(result.status)}>{localizeWorkflowPolicyText(result.status, locale)}</Tag>
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
                <Typography.Text>{localizeWorkflowPolicyText(result.message, locale)}</Typography.Text>
                <Space direction="vertical" size={6}>
                  {reasons.map((reason) => (
                    <Space key={reason.code} size={[6, 4]} wrap>
                      <Tooltip title={reason.code}>
                        <Tag color={reason.tagColor}>
                          {localizeWorkflowPolicyText(reason.label, locale)}
                        </Tag>
                      </Tooltip>
                      <Typography.Text type="secondary">
                        {localizeWorkflowPolicyText(reason.detail, locale)}
                      </Typography.Text>
                    </Space>
                  ))}
                </Space>
              </Space>
            }
            message={
              <Tag color={impactStatusColor(result.status)}>
                {localizeWorkflowPolicyText(result.status, locale)}
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
              description={localizeWorkflowPolicyText(diagnosisEstimate.detail, locale)}
              message={
                <Space size={[6, 4]} wrap>
                  <Tag color={readinessTagColor(diagnosisEstimate.status)}>
                    {readinessLabel(diagnosisEstimate.status, locale)}
                  </Tag>
                  <span>{localizeWorkflowPolicyText(diagnosisEstimate.label, locale)}</span>
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
                text={localizeWorkflowPolicyText(reportChannelReadiness?.text ?? "No report channel bound", locale)}
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
  const locale = useLocale();
  return (
    <Space direction="vertical" size={2}>
      <Typography.Text type="secondary">{label}</Typography.Text>
      <Space wrap>
        <Tag color={ready ? "green" : "red"}>{localizeWorkflowPolicyText(ready ? "ready" : "blocked", locale)}</Tag>
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
      <Tag color={severityColor(group.severity)}>{localizeWorkflowPolicyText(group.severity, locale)}</Tag>
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
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  const proofTrace =
    result === null ? null : reportWorkflowPolicyReplayProofTrace(result);

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
                    {localizeWorkflowPolicyText(autoDiagnosisReplaySummary(result.auto_diagnosis), locale)}
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
  trace: ReturnType<typeof reportWorkflowPolicyReplayProofTrace>;
}) {
  const locale = useLocale();
  const t = useTranslations("WorkflowPolicySettings");
  return (
    <div aria-label={t("proofTraceLabel")} className="settings-proof-outcome">
      <div className="settings-preview-header">
        <Typography.Text strong>{t("proofTrace")}</Typography.Text>
        <Tag color={readinessTagColor(trace.status)}>
          {readinessLabel(trace.status, locale)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{localizeWorkflowPolicyText(trace.detail, locale)}</Typography.Text>
      <div className="workflow-automation-grid">
        {trace.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{localizeWorkflowPolicyText(item.title, locale)}</Typography.Text>
              <Tag color={readinessTagColor(item.status)}>
                {readinessLabel(item.status, locale)}
              </Tag>
            </div>
            <Typography.Text strong>{localizeWorkflowPolicyText(item.value, locale)}</Typography.Text>
            <Typography.Text type="secondary">{localizeWorkflowPolicyText(item.detail, locale)}</Typography.Text>
            {item.actions && item.actions.length > 0 ? (
              <Space size={[4, 4]} wrap>
                {item.actions.map((action) => (
                  <Tooltip
                    key={`${item.title}:${action.href}`}
                    title={localizeWorkflowPolicyText(action.detail, locale)}
                  >
                    <Button
                      href={action.href}
                      icon={<RadarChartOutlined />}
                      size="small"
                      type="link"
                    >
                      {localizeWorkflowPolicyText(action.label, locale)}
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
): string {
  const parts = [
    pluralizeCount(autoDiagnosis.policies_matched, "policy"),
    pluralizeCount(autoDiagnosis.snapshots, "snapshot"),
    `${pluralizeCount(autoDiagnosis.rooms_started, "room")} started`,
  ];
  const confirmedSnapshots =
    autoDiagnosisConfirmedSnapshotCount(autoDiagnosis);
  if (confirmedSnapshots > 0) {
    parts.push(
      `${pluralizeCount(confirmedSnapshots, "snapshot")} already confirmed`,
    );
  }
  if (autoDiagnosis.rooms_skipped > 0) {
    parts.push(
      `${pluralizeCount(autoDiagnosis.rooms_skipped, "snapshot")} skipped by safety cap`,
    );
  }
  return `AI diagnosis: ${parts.join(", ")}`;
}

function pluralizeCount(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
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

function localizeWorkflowPolicyText(value: string, locale: string): string {
  if (locale !== "zh-CN") {
    return value;
  }
  const exact: Readonly<Record<string, string>> = {
    "": "",
    alert_storm: "告警风暴",
    auto_room: "自动创建诊断室",
    blocked: "已阻塞",
    cascade: "级联故障",
    critical: "严重",
    diagnosis_close: "诊断关闭",
    diagnosis_consultation: "诊断会诊",
    disabled: "已停用",
    info: "提示",
    pending: "等待中",
    ready: "就绪",
    report: "报告",
    review: "需检查",
    single_alert: "单告警",
    suggest_room: "建议创建诊断室",
    unknown: "未知",
    warning: "警告",
    Blocked: "已阻塞",
    WeCom: "企业微信",
    "AI delivery proof missing": "缺少 AI 交付证明",
    "Active alert and metric collection tools are enabled.":
      "活动告警和指标采集工具均已启用。",
    "Active alert evidence tool": "活动告警证据工具",
    "Add AI scopes": "添加 AI 交付范围",
    "Add a Thanos Query or Prometheus metric evidence source, then use Recommended by sources to create metric_query or metric_range_query templates.":
      "请添加 Thanos Query 或 Prometheus 指标证据源，然后使用“按告警源推荐”创建 metric_query 或 metric_range_query 模板。",
    "Add a metric_query or metric_range_query template on the selected Prometheus-compatible source so AI can raise confidence with measured evidence.":
      "请在所选 Prometheus 兼容告警源上添加 metric_query 或 metric_range_query 模板，使 AI 能通过测量证据提高置信度。",
    "Add an active_alerts template bound to the workflow alert source so AI can confirm sibling firing alerts.":
      "请添加绑定到工作流告警源的 active_alerts 模板，使 AI 能确认同组触发告警。",
    "Add the diagnosis_close scope when auto_room should deliver close notifications.":
      "当 auto_room 需要交付关闭通知时，请添加 diagnosis_close 范围。",
    "Add the diagnosis_consultation scope when auto_room should deliver AI diagnosis updates.":
      "当 auto_room 需要交付 AI 诊断进展时，请添加 diagnosis_consultation 范围。",
    "Add the report delivery scope to the bound notification channel.":
      "请为绑定的通知渠道添加报告交付范围。",
    "Alert source disabled": "告警源已停用",
    "Alertmanager webhook deliveries can ingest firing alerts and start automatic diagnosis rooms.":
      "Alertmanager Webhook 交付可以接入触发中的告警并启动自动诊断室。",
    "Alertmanager webhook deliveries can ingest firing alerts; suggest_room still requires operator handoff.":
      "Alertmanager Webhook 交付可以接入触发中的告警；suggest_room 仍需操作员移交。",
    "Automatic diagnosis room delivery requires an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes.":
      "自动诊断室交付需要具有 report、diagnosis_consultation 和 diagnosis_close 范围的企业微信渠道。",
    "Automatic diagnosis room starts require an Alertmanager alert source because the webhook endpoint rejects non-Alertmanager profiles.":
      "自动诊断室启动需要 Alertmanager 告警源，因为 Webhook 端点会拒绝非 Alertmanager 配置。",
    "Automatic diagnosis rooms will not start until the blocked preview reasons are resolved.":
      "在解决预览中的阻塞原因前，自动诊断室不会启动。",
    "Bind a notification channel before enabling auto_room AI diagnosis updates.":
      "启用 auto_room AI 诊断进展前，请先绑定通知渠道。",
    "Bind an Alertmanager alert source before using auto_room diagnosis follow-up.":
      "使用 auto_room 诊断后续处理前，请先绑定 Alertmanager 告警源。",
    "Bind an enabled report channel with diagnosis_consultation and diagnosis_close scopes before using automatic diagnosis rooms.":
      "使用自动诊断室前，请绑定已启用且具有 diagnosis_consultation 和 diagnosis_close 范围的报告渠道。",
    "Bound alert source must be enabled before workflow policy enablement.":
      "启用工作流策略前，必须先启用绑定的告警源。",
    "Bound grouping policy must be enabled before workflow policy enablement.":
      "启用工作流策略前，必须先启用绑定的分组策略。",
    "Configuration bindings are usable and the bounded sample produced an impact estimate.":
      "配置绑定可用，有界样例已生成影响估算。",
    "Configure metric source": "配置指标源",
    "Configure source": "配置告警源",
    "Create AI channel": "创建 AI 通知渠道",
    "Create active-alert tool": "创建活动告警工具",
    "Create an enabled grouping policy before saving this workflow.":
      "保存此工作流前，请创建已启用的分组策略。",
    "Create grouping": "创建分组策略",
    "Create metric tool": "创建指标工具",
    "Create or select an enabled Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes, run AI diagnosis and close proof, then return to enable this workflow.":
      "请创建或选择已启用且具有 report、diagnosis_consultation 和 diagnosis_close 范围的企业微信渠道，运行 AI 诊断和关闭证明，然后返回启用此工作流。",
    "Diagnosis follow-up is disabled for this policy.":
      "此策略已停用诊断后续处理。",
    "Enable at least one active_alerts template and one metric template before relying on AI follow-up.":
      "依赖 AI 后续诊断前，请至少启用一个 active_alerts 模板和一个指标模板。",
    "Enable policy": "启用策略",
    "Enable the bound alert source before activating this workflow.":
      "激活此工作流前，请先启用绑定的告警源。",
    "Enable the bound alert source before relying on webhook ingestion.":
      "依赖 Webhook 接入前，请先启用绑定的告警源。",
    "Enable the bound grouping policy so sampled alerts can be grouped.":
      "请启用绑定的分组策略，以便对样例告警进行分组。",
    "Enable the bound notification channel before report delivery.":
      "报告交付前，请先启用绑定的通知渠道。",
    "Enabled diagnosis templates are bound only to disabled or incompatible sources.":
      "已启用的诊断模板仅绑定到已停用或不兼容的告警源。",
    "Enterprise WeChat channel": "企业微信渠道",
    "Fix form": "修正表单",
    "Limit must be between 1 and 100000.": "限制必须在 1 到 100000 之间。",
    "Loaded matching automatic diagnosis workflows for retained Alertmanager proof.":
      "已加载与保留的 Alertmanager 证明匹配的自动诊断工作流。",
    "Manual replay": "手动重放",
    "No channel": "无渠道",
    "No matching alert groups in this sample, so no automatic diagnosis room is expected.":
      "此样例中没有匹配的告警分组，因此预计不会创建自动诊断室。",
    "No notification channel profile is bound.": "未绑定通知渠道配置。",
    "No rooms": "无诊断室",
    "Open the selected Enterprise WeChat channel and run current AI diagnosis and diagnosis close sample tests before workflow policy enablement.":
      "启用工作流策略前，请打开所选企业微信渠道并运行当前 AI 诊断和诊断关闭样例测试。",
    "Open the selected Enterprise WeChat channel, run the current AI diagnosis and diagnosis close sample tests, then return to enable this workflow.":
      "请打开所选企业微信渠道，运行当前 AI 诊断和诊断关闭样例测试，然后返回启用此工作流。",
    "Policy bindings and diagnosis tool configuration are ready.":
      "策略绑定和诊断工具配置均已就绪。",
    "Policy name is required.": "策略名称为必填项。",
    "Policy name must be 120 characters or fewer.":
      "策略名称不能超过 120 个字符。",
    "Prepared an automatic diagnosis workflow from the settings overview create action.":
      "已根据配置概览创建操作准备自动诊断工作流。",
    "Prepared an automatic diagnosis workflow that needs an enabled Alertmanager source.":
      "已准备自动诊断工作流，但仍需要已启用的 Alertmanager 告警源。",
    "Prepared automatic AI diagnosis room handoff from the settings overview action.":
      "已根据配置概览操作准备自动 AI 诊断室移交。",
    "Prepared automatic diagnosis room handoff from the settings overview action.":
      "已根据配置概览操作准备自动诊断室移交。",
    "Prometheus sources support metric evidence, but they do not receive Alertmanager webhook deliveries.":
      "Prometheus 告警源支持指标证据，但不接收 Alertmanager Webhook 交付。",
    "Recent bounded samples did not match this source and grouping configuration.":
      "最近的有界样例未匹配此告警源和分组配置。",
    "Report only": "仅报告",
    "Review grouping": "检查分组策略",
    "Review source": "检查告警源",
    "Run AI proof": "运行 AI 证明",
    "Run current AI diagnosis and diagnosis close sample tests for the bound Enterprise WeChat channel.":
      "请为绑定的企业微信渠道运行当前 AI 诊断和诊断关闭样例测试。",
    "Select Auto room to enable automatic AI consultation readiness checks.":
      "请选择“自动诊断室”以启用自动 AI 会诊就绪检查。",
    "Select the alert source that receives the Alertmanager webhook.":
      "请选择接收 Alertmanager Webhook 的告警源。",
    "Selected notification channel can deliver final report notifications.":
      "所选通知渠道可以交付最终报告通知。",
    "Selected notification channel can deliver reports, auto-room AI diagnosis updates, and close notifications.":
      "所选通知渠道可以交付报告、自动诊断室 AI 诊断进展和关闭通知。",
    "Selected notification channel must be enabled before workflow policy enablement.":
      "启用工作流策略前，必须先启用所选通知渠道。",
    "Switch to WeCom": "切换到企业微信",
    "Thanos Rule active-alert sources can provide firing-alert evidence, but automatic diagnosis room starts require an Alertmanager webhook source. Select or create an Alertmanager source for workflow intake, then keep Thanos Rule for active_alerts evidence templates.":
      "Thanos Rule 活动告警源可以提供触发告警证据，但自动诊断室启动需要 Alertmanager Webhook 告警源。请为工作流接入选择或创建 Alertmanager 告警源，并保留 Thanos Rule 用于 active_alerts 证据模板。",
    "This policy does not request AI diagnosis handoff for matched alert groups.":
      "此策略不会为匹配的告警分组请求 AI 诊断移交。",
    "Use a trigger mode supported by impact preview before enabling this policy.":
      "启用此策略前，请使用影响预览支持的触发方式。",
    "Use an Enterprise WeChat channel before enabling auto_room AI diagnosis updates.":
      "启用 auto_room AI 诊断进展前，请使用企业微信渠道。",
    "Use an Enterprise WeChat channel for automatic diagnosis room delivery, then run AI diagnosis and close proof before enablement.":
      "自动诊断室交付请使用企业微信渠道，并在启用前运行 AI 诊断和关闭证明。",
    "Webhook firing alerts": "Webhook 触发告警",
    "A handoff is retained for an operator to create the AI diagnosis room.":
      "已保留移交，由操作员创建 AI 诊断室。",
    "Alertmanager alerts can produce evidence, start AI diagnosis rooms, and notify operators.":
      "Alertmanager 告警可以生成证据、启动 AI 诊断室并通知操作员。",
    "Alerts can prepare an AI handoff, but an operator still starts the diagnosis room.":
      "告警可以准备 AI 移交，但仍由操作员启动诊断室。",
    "All required bindings are selected; save the policy, run impact preview, then replay a bounded window.":
      "已选择所有必需绑定；请保存策略、运行影响预览，然后重放有界窗口。",
    "Auto-room path blocked.": "自动诊断室路径已阻塞。",
    "Auto-room path needs review.": "自动诊断室路径需要检查。",
    "Auto-room path pending.": "自动诊断室路径等待配置。",
    "Auto-room path ready.": "自动诊断室路径已就绪。",
    "Complete the required automatic diagnosis selections before enabling this path.":
      "启用此路径前，请完成必需的自动诊断选项。",
    "Enterprise WeChat can receive final report delivery while AI room handoff remains operator-controlled.":
      "企业微信可以接收最终报告交付，同时 AI 诊断室移交仍由操作员控制。",
    "Enterprise WeChat can receive final report delivery without starting or suggesting AI diagnosis rooms.":
      "企业微信可以接收最终报告交付，但不会启动或建议 AI 诊断室。",
    "Enterprise WeChat can receive final report delivery, AI diagnosis updates, final-ready notices, and close notifications.":
      "企业微信可以接收最终报告、AI 诊断进展、最终就绪提示和关闭通知。",
    "Matching Alertmanager webhooks can start AI diagnosis rooms, collect evidence, and notify the operator channel.":
      "匹配的 Alertmanager Webhook 可以启动 AI 诊断室、采集证据并通知操作员渠道。",
    "No AI diagnosis room will be suggested or started by this policy.":
      "此策略不会建议或启动 AI 诊断室。",
    "No diagnosis room will be suggested or started by this policy.":
      "此策略不会建议或启动诊断室。",
    "No report notification channel is bound.": "未绑定报告通知渠道。",
    "Operator handoff": "操作员移交",
    "Resolve blocked bindings before relying on this workflow automation path.":
      "依赖此工作流自动化路径前，请解决被阻塞的绑定。",
    "Resolve blocked intake, evidence, or notification requirements before automatic diagnosis rooms can run.":
      "自动诊断室运行前，请解决被阻塞的接入、证据或通知要求。",
    "Review the retained handoff or delivery gap before treating this workflow as fully automated.":
      "将此工作流视为完全自动化前，请检查保留的移交或交付缺口。",
    "Save the policy, preview impact, then replay a bounded window after enablement.":
      "请保存策略、预览影响，并在启用后重放有界窗口。",
    "The automatic diagnosis path can be saved, but one or more operator-facing choices need review before production use.":
      "自动诊断路径可以保存，但投入生产前仍需检查一个或多个面向操作员的选项。",
    "This matching AI room workflow is already enabled.":
      "此匹配的 AI 诊断室工作流已启用。",
    "This matching AI room workflow is ready to enable.":
      "此匹配的 AI 诊断室工作流可以启用。",
    "This policy generates report workflow output without AI diagnosis-room automation.":
      "此策略会生成报告工作流输出，但不启用 AI 诊断室自动化。",
    "Webhook auto-room": "Webhook 自动诊断室",
    "Webhook handoff": "Webhook 移交",
    "Workflow policy draft is ready for the next operator action.":
      "工作流策略草稿已可执行下一步操作。",
    "Workflow setup actions are ready.": "工作流配置操作已就绪。",
    "Workflow setup blocked.": "工作流配置已阻塞。",
    "Workflow setup needs review.": "工作流配置需要检查。",
    "Workflow setup pending.": "工作流配置等待完成。",
    "Workflow setup ready.": "工作流配置已就绪。",
    "AI consultation": "AI 会诊",
    "AI delivery proof": "AI 交付证明",
    "AI delivery proof missing.": "缺少 AI 交付证明。",
    "AI diagnosis disabled.": "AI 诊断已停用。",
    "AI evidence": "AI 证据",
    "AI handoff": "AI 移交",
    "AI room": "AI 诊断室",
    "Alert grouping policy": "告警分组策略",
    "Alert intake": "告警接入",
    "Alert source disabled.": "告警源已停用。",
    "Alert source required.": "需要告警源。",
    "Alertmanager intake": "Alertmanager 接入",
    "Alertmanager source required": "需要 Alertmanager 告警源",
    "Alertmanager webhook source required.": "需要 Alertmanager Webhook 告警源。",
    "Auto-room delivery blocked.": "自动诊断室交付已阻塞。",
    "Automatic diagnosis blocked.": "自动诊断已阻塞。",
    "Automatic diagnosis rooms disabled.": "自动诊断室已停用。",
    "Automatic diagnosis rooms estimated.": "已估算自动诊断室数量。",
    "Bound alert source": "已绑定告警源",
    "Bound grouping policy": "已绑定分组策略",
    "Delivery": "交付",
    "Delivery scopes": "交付范围",
    "Diagnosis close scope missing": "缺少诊断关闭范围",
    "Diagnosis consultation scope missing": "缺少诊断会诊范围",
    "Diagnosis room starts automatically": "自动启动诊断室",
    "Diagnosis room suggested": "建议创建诊断室",
    "Diagnosis tools need review.": "诊断工具需要检查。",
    "Enterprise WeChat channel required.": "需要企业微信渠道。",
    "Enterprise WeChat required": "需要企业微信",
    "Evidence": "证据",
    "Evidence collection": "证据采集",
    "Executable diagnosis tools ready.": "可执行诊断工具已就绪。",
    "Follow-up disabled": "后续处理已停用",
    "Generic webhook selected.": "已选择通用 Webhook。",
    "Grouping": "分组",
    "Grouping policy disabled": "分组策略已停用",
    "Grouping rule": "分组规则",
    "Impact not previewed": "尚未预览影响",
    "Impact preview": "影响预览",
    "Metric evidence source": "指标证据源",
    "Metric evidence tool": "指标证据工具",
    "No Alertmanager webhook ingress.": "没有 Alertmanager Webhook 接入。",
    "No automatic rooms expected.": "预计不会自动创建诊断室。",
    "No enabled diagnosis tools.": "没有已启用的诊断工具。",
    "No matching events": "没有匹配事件",
    "No report channel bound": "未绑定报告渠道",
    "No report channel selected.": "尚未选择报告渠道。",
    "No usable diagnosis tools.": "没有可用的诊断工具。",
    "Notification": "通知",
    "Notification channel disabled": "通知渠道已停用",
    "Notification channel disabled.": "通知渠道已停用。",
    "Notification channel required": "需要通知渠道",
    "Notification channel scope mismatch.": "通知渠道范围不匹配。",
    "Operator channel": "操作员通知渠道",
    "Operator handoff retained.": "已保留操作员移交。",
    "Operator notification": "操作员通知",
    "Policy can be enabled after review.": "检查后可以启用策略。",
    "Policy can be enabled.": "可以启用策略。",
    "Policy cannot be enabled.": "无法启用策略。",
    "Preview ready": "预览已就绪",
    "Proof": "证明",
    "Replay window": "重放窗口",
    "Report and AI-room channel": "报告与 AI 诊断室渠道",
    "Report and auto-room delivery ready.": "报告与自动诊断室交付已就绪。",
    "Report delivery ready.": "报告交付已就绪。",
    "Report notification channel": "报告通知渠道",
    "Report scope missing": "缺少报告范围",
    "Select a grouping policy.": "请选择分组策略。",
    "Select a valid report notification channel.": "请选择有效的报告通知渠道。",
    "Select an alert source.": "请选择告警源。",
    "Source": "告警源",
    "Trigger": "触发方式",
    "Trigger mode unsupported": "不支持此触发方式",
    "Webhook": "Webhook",
    "Webhook auto-room ingress blocked.": "Webhook 自动诊断室接入已阻塞。",
    "Webhook auto-room ingress ready.": "Webhook 自动诊断室接入已就绪。",
    "Webhook ingest ready.": "Webhook 接入已就绪。",
    "Webhook ingress not used.": "未使用 Webhook 接入。",
    "Window end is required.": "窗口结束时间为必填项。",
    "Window end must be a valid date-time.": "窗口结束时间必须是有效日期时间。",
    "Window end must be after window start.": "窗口结束时间必须晚于开始时间。",
    "Window start is required.": "窗口开始时间为必填项。",
    "Window start must be a valid date-time.": "窗口开始时间必须是有效日期时间。",
    "Workflow policy fields": "工作流策略字段",
  };
  if (exact[value] !== undefined) {
    return exact[value]!;
  }
  let match = value.match(/^Current user is not authorized to manage report workflow policy #(\d+)\.$/);
  if (match) {
    return `当前用户无权管理报告工作流策略 #${match[1]}。`;
  }
  match = value.match(/^Current user is not authorized to (.+)\.$/);
  if (match) {
    return `当前用户无权执行以下操作：${match[1]}。`;
  }
  match = value.match(/^(\d+) groups \/ (\d+) events$/);
  if (match) {
    return `${match[1]} 个分组 / ${match[2]} 个事件`;
  }
  match = value.match(/^(\d+) matching AI room workflows? found;?(.*)$/);
  if (match) {
    return `找到 ${match[1]} 个匹配的 AI 诊断室工作流${match[2] ? `；${match[2]}` : ""}`;
  }
  match = value.match(/^AI diagnosis: (.+)$/);
  if (match) {
    return `AI 诊断：${match[1]}`;
  }
  match = value.match(/^Alert sources failed to load: (.+)\.$/);
  if (match) {
    return `告警源加载失败：${match[1]}。`;
  }
  match = value.match(/^Grouping policies failed to load: (.+)\.$/);
  if (match) {
    return `分组策略加载失败：${match[1]}。`;
  }
  match = value.match(/^Notification channels failed to load: (.+)\.$/);
  if (match) {
    return `通知渠道加载失败：${match[1]}。`;
  }
  match = value.match(/^(\d+) active alert \/ (\d+) metric$/);
  if (match) {
    return `${match[1]} 个活动告警工具 / ${match[2]} 个指标工具`;
  }
  match = value.match(/^(\d+) estimated alert groups? can start automatic AI diagnosis rooms when this policy is replayed or receives matching Alertmanager webhooks\.$/);
  if (match) {
    return `当此策略被重放或收到匹配的 Alertmanager Webhook 时，预计 ${match[1]} 个告警分组可启动自动 AI 诊断室。`;
  }
  match = value.match(/^(\d+) rooms?$/);
  if (match) {
    return `${match[1]} 个诊断室`;
  }
  match = value.match(/^Add (.+) scope, run AI delivery proof, then return to enable this workflow\.$/);
  if (match) {
    return `请添加 ${match[1]!.split(" and ").map((scope) => localizeWorkflowPolicyText(scope, locale)).join(" 和 ")} 范围，运行 AI 交付证明，然后返回启用此工作流。`;
  }
  match = value.match(/^Missing (.+)\.$/);
  if (match) {
    return `缺少${match[1]!.split(" and ").map((item) => localizeWorkflowPolicyText(item, locale)).join("和")}。`;
  }
  match = value.match(/^Selected notification channel is missing (.+) scope\.$/);
  if (match) {
    return `所选通知渠道缺少 ${match[1]!.split(" and ").map((scope) => localizeWorkflowPolicyText(scope, locale)).join(" 和 ")} 范围。`;
  }
  match = value.match(/^(\d+) blocking setup actions? must be resolved before this workflow is ready\.$/);
  if (match) {
    return `此工作流就绪前，必须解决 ${match[1]} 个阻塞配置操作。`;
  }
  match = value.match(/^(\d+) setup actions? remain before the workflow can be exercised\.$/);
  if (match) {
    return `此工作流可以演练前，还剩 ${match[1]} 个配置操作。`;
  }
  match = value.match(/^(\d+) setup actions? should be reviewed before enablement\.$/);
  if (match) {
    return `启用前应检查 ${match[1]} 个配置操作。`;
  }
  match = value.match(/^Deliver report notifications through (.+)\.$/);
  if (match) {
    return `通过 ${match[1]} 交付报告通知。`;
  }
  match = value.match(/^This matching AI room workflow can be enabled after review: (.+)$/);
  if (match) {
    return `此匹配的 AI 诊断室工作流可在检查后启用：${localizeWorkflowPolicyText(match[1]!, locale)}`;
  }
  match = value.match(/^unselected (.+)$/);
  if (match) {
    return `未选择的${localizeWorkflowPolicyText(match[1]!, locale)}`;
  }
  match = value.match(/^AI room will be suggested for operator handoff\. (.+)$/);
  if (match) {
    return `将建议创建 AI 诊断室并移交操作员。${localizeWorkflowPolicyText(match[1]!, locale)}`;
  }
  const sentences = value.match(/[^.!?]+[.!?]?/g)?.map((part) => part.trim()) ?? [];
  if (sentences.length > 1) {
    const localized = sentences.map((sentence) =>
      localizeWorkflowPolicyText(sentence, locale),
    );
    if (localized.some((sentence, index) => sentence !== sentences[index])) {
      return localized.join("");
    }
  }
  return value;
}

function localizeWorkflowPolicyMessages(
  messages: readonly string[],
  locale: string,
): string {
  return messages
    .map((message) => localizeWorkflowPolicyText(message, locale))
    .join(" ");
}

function nullableDate(value: string | null, locale: string): string {
  if (value === null) {
    return locale === "zh-CN" ? "未设置" : "Not set";
  }
  return formatDateTime(value, locale);
}
