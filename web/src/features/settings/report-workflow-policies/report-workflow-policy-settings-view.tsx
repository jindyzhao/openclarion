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
  useSettingsMutation,
} from "../query-state";
import { ReadOnlyModeAlert } from "../permission-notice";
import {
  useCurrentRBACAuthorizations,
  type CurrentRBACAuthorizationCheck,
} from "../rbac-capabilities";
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
    refreshMessage: "Policies refreshed.",
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
    ],
    [policies],
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
  const canSaveCurrentPolicy =
    editingID === null
      ? canCreatePolicy
      : currentAuthorization.can(reportWorkflowPolicyManageKey(editingID));
  const canPreviewDraftImpact = currentAuthorization.can("reportWorkflowManage");
  const formPermissionNotice = settingsManagePermissionNotice({
    canManage: canSaveCurrentPolicy,
    isChecking: !clientReady || currentAuthorization.isChecking,
    resourceLabel:
      editingID === null
        ? "report workflow policy creation"
        : `report workflow policy #${editingID}`,
  });
  const readPermissionNotice = settingsReadPermissionNotice({
    canRead: canReadPolicies,
    errorStatus,
    isChecking: !clientReady || currentAuthorization.isChecking,
    resourceLabel: "report workflow policies",
  });
  const visibleNotice =
    currentAuthorization.notice ?? readPermissionNotice ?? notice;
  const relationOptions = useMemo(
    () =>
      buildRelationOptions(
        alertSourcesResult,
        groupingPoliciesResult,
        notificationChannelsResult,
      ),
    [alertSourcesResult, groupingPoliciesResult, notificationChannelsResult],
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
  const selectedReportNotificationChannelProfileID = Form.useWatch(
    "reportNotificationChannelProfileID",
    form,
  );
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
      alertSourceOptionsForFollowUp(selectedDiagnosisFollowUp, relationOptions),
    [relationOptions, selectedDiagnosisFollowUp],
  );
  const notificationChannelOptions = useMemo(
    () =>
      notificationChannelOptionsForFollowUp(
        selectedDiagnosisFollowUp,
        relationOptions,
      ),
    [relationOptions, selectedDiagnosisFollowUp],
  );
  const draftFormState = useMemo<ReportWorkflowPolicyFormState>(
    () => ({
      name: selectedName,
      alertSourceProfileID: selectedAlertSourceID,
      groupingPolicyID: selectedGroupingPolicyID,
      reportNotificationChannelProfileID:
        selectedReportNotificationChannelProfileID,
      triggerMode: selectedTriggerMode,
      reportScenario: selectedReportScenario,
      diagnosisFollowUp: selectedDiagnosisFollowUp,
    }),
    [
      selectedAlertSourceID,
      selectedDiagnosisFollowUp,
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
      setNotice({ kind: "warning", message: "You are not authorized to save this policy." });
      return;
    }
    const parsed = formStateToWriteRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
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
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
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
          ? `Policy saved with enablement blockers: ${enablementBlockers.join(" ")}`
          : reviewItems.length > 0
          ? `Policy saved with review items: ${reviewItems.join(" ")}`
          : "Policy saved.",
    });
  }

  async function handleEnablement(
    policy: ReportWorkflowPolicy,
    enabled: boolean,
  ) {
    if (!currentAuthorization.can(reportWorkflowPolicyManageKey(policy.id))) {
      setNotice({ kind: "warning", message: "You are not authorized to change this policy." });
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
      setNotice({ kind: "error", message: readiness.detail });
      setActionID(null);
      return;
    }
    try {
      await enablementAction.mutateAsync({ policyID: policy.id, enabled });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
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
          ? `Policy enabled with review items: ${readiness.detail}`
          : enabled
            ? "Policy enabled."
            : "Policy disabled.",
    });
  }

  function editPolicy(policy: ReportWorkflowPolicy) {
    if (!currentAuthorization.can(reportWorkflowPolicyManageKey(policy.id))) {
      setNotice({ kind: "warning", message: "You are not authorized to edit this policy." });
      return;
    }
    setEditingID(policy.id);
    form.setFieldsValue(policyToFormState(policy));
    setLaunchNotice(null);
    setNotice(null);
  }

  function openReplay(policy: ReportWorkflowPolicy) {
    if (!currentAuthorization.can(reportWorkflowPolicyManageKey(policy.id))) {
      setNotice({ kind: "warning", message: "You are not authorized to replay this policy." });
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
      setNotice({ kind: "warning", message: "You are not authorized to preview this policy." });
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
      title: `Impact Preview #${policy.id}`,
      result: previewed.data,
    });
    setNotice({
      kind: previewed.data.status === "blocked" ? "warning" : "info",
      message: `Impact preview ${previewed.data.status}: ${previewed.data.groups_estimated} groups from ${previewed.data.events_matched} matching events.`,
    });
  }

  async function handleDraftImpactPreview() {
    if (!canPreviewDraftImpact) {
      setNotice({ kind: "warning", message: "You are not authorized to preview this draft." });
      return;
    }
    const parsed = formStateToWriteRequest(draftFormState);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
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
    setImpactPreview({ title: "Draft Impact Preview", result: previewed.data });
    setNotice({
      kind: previewed.data.status === "blocked" ? "warning" : "info",
      message: `Draft impact preview ${previewed.data.status}: ${previewed.data.groups_estimated} groups from ${previewed.data.events_matched} matching events.`,
    });
  }

  async function handleReplay(values: ReportWorkflowPolicyReplayFormState) {
    if (replayPolicy === null) {
      return;
    }
    if (!currentAuthorization.can(reportWorkflowPolicyManageKey(replayPolicy.id))) {
      setNotice({ kind: "warning", message: "You are not authorized to replay this policy." });
      return;
    }
    const parsed = formStateToReplayRequest(values);
    if (!parsed.ok) {
      setNotice({ kind: "error", message: parsed.message });
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
          ? "Replay accepted."
          : "Replay completed without report snapshots.",
      });
    } catch (error) {
      setNotice({ kind: "error", message: settingsErrorMessage(error) });
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
      <Row aria-label="Report workflow policy metrics" gutter={[12, 12]}>
        <MetricCard label="Policies" value={policies.length} />
        <MetricCard label="Enabled" value={summary.enabled} />
        <MetricCard label="Report channel" value={summary.reportChannel} />
        <MetricCard label="Room follow-up" value={summary.roomFollowUp} />
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
          aria-label="Report workflow launch preset"
          description={launchNotice}
          message="Workflow action loaded"
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
          description={relationOptions.warnings.join(" ")}
          message="Related configuration unavailable"
          role="status"
          showIcon
          type="warning"
        />
      ) : null}
      {!diagnosisToolTemplatesResult.ok ? (
        <Alert
          description={diagnosisToolTemplatesResult.error.message}
          message="Diagnosis tool templates unavailable"
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
                  New
                </Button>
              )
            }
            title={
              editingID === null
                ? "New Workflow Policy"
                : `Edit Policy #${editingID}`
            }
          >
            {formPermissionNotice ? (
              <ReadOnlyModeAlert notice={formPermissionNotice} />
            ) : null}
            <Form<ReportWorkflowPolicyFormState>
              disabled={busy || !canSaveCurrentPolicy}
              form={form}
              initialValues={initialFormValues}
              layout="vertical"
              onFinish={handleSubmit}
            >
              <Form.Item
                label="Name"
                name="name"
                rules={[
                  { required: true, message: "Policy name is required." },
                  {
                    max: 120,
                    message: "Policy name must be 120 characters or fewer.",
                  },
                ]}
              >
                <Input autoComplete="off" />
              </Form.Item>

              <Row gutter={12}>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Alert source"
                    name="alertSourceProfileID"
                    rules={[
                      { required: true, message: "Alert source is required." },
                    ]}
                  >
                    <Select
                      optionFilterProp="label"
                      options={alertSourceOptions}
                      placeholder="Select alert source"
                      showSearch
                    />
                  </Form.Item>
                </Col>
                <Col sm={12} xs={24}>
                  <Form.Item
                    label="Grouping policy"
                    name="groupingPolicyID"
                    rules={[
                      {
                        required: true,
                        message: "Grouping policy is required.",
                      },
                    ]}
                  >
                    <Select
                      optionFilterProp="label"
                      options={relationOptions.groupingPolicyOptions}
                      placeholder="Select grouping policy"
                      showSearch
                    />
                  </Form.Item>
                </Col>
              </Row>
              <AlertSourceIngressReadinessPreview
                readiness={alertSourceIngressReadiness}
              />

              <Form.Item
                label="Report channel"
                name="reportNotificationChannelProfileID"
              >
                <Select
                  allowClear
                  optionFilterProp="label"
                  options={notificationChannelOptions}
                  placeholder="No report channel"
                  showSearch
                />
              </Form.Item>
              <NotificationChannelReadinessPreview
                operatorReadiness={operatorChannelReadiness}
                readiness={reportNotificationChannelReadiness}
                selectedChannel={selectedReportNotificationChannel}
              />

              <Form.Item
                label="Trigger"
                name="triggerMode"
                rules={[{ required: true, message: "Trigger is required." }]}
              >
                <Segmented
                  block
                  options={[{ value: "manual_replay", label: "Manual replay" }]}
                />
              </Form.Item>

              <Form.Item
                label="Scenario"
                name="reportScenario"
                rules={[{ required: true, message: "Scenario is required." }]}
              >
                <Select
                  options={[
                    { value: "single_alert", label: "Single alert" },
                    { value: "cascade", label: "Cascade" },
                    { value: "alert_storm", label: "Alert storm" },
                  ]}
                />
              </Form.Item>

              <Form.Item
                label="Diagnosis follow-up"
                name="diagnosisFollowUp"
                rules={[
                  {
                    required: true,
                    message: "Diagnosis follow-up is required.",
                  },
                ]}
              >
                <Segmented
                  block
                  options={[
                    { value: "disabled", label: "Disabled" },
                    { value: "suggest_room", label: "Suggest room" },
                    { value: "auto_room", label: "Auto room" },
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
                  Save Policy
                </Button>
                <Button disabled={busy} onClick={resetForm} type="default">
                  Reset
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
                Refresh
              </Button>
            }
            title="Configured Policies"
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
      alertSources.map((source) => [source.id, alertSourceLabel(source)]),
    ),
    alertSourceLabelsByID: new Map(
      alertSources.map((source) => [source.id, source.labels]),
    ),
    alertSourceOptions: alertSources.map((source) =>
      relationOption(source.id, alertSourceLabel(source)),
    ),
    groupingPolicyEnabledIDs: new Set(
      groupingPolicies
        .filter((policy) => policy.enabled)
        .map((policy) => policy.id),
    ),
    groupingPolicyLabels: Object.fromEntries(
      groupingPolicies.map((policy) => [
        policy.id,
        groupingPolicyLabel(policy),
      ]),
    ),
    groupingPolicyOptions: groupingPolicies.map((policy) =>
      relationOption(policy.id, groupingPolicyLabel(policy)),
    ),
    notificationChannelLabels: Object.fromEntries(
      notificationChannels.map((channel) => [
        channel.id,
        notificationChannelLabel(channel),
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
      relationOption(channel.id, notificationChannelLabel(channel)),
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

    const reason = alertSourceAutoRoomBlockReason(sourceEnabled, sourceKind);
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
    const label = `${option.label} - ${hints.join(", ")}`;
    return {
      ...option,
      disabled: state.disabled,
      label,
      title: label,
    };
  });
}

function alertSourceLabel(source: AlertSourceProfile): string {
  return `#${source.id} ${source.name} (${source.kind}, ${enabledLabel(source.enabled)})`;
}

function groupingPolicyLabel(policy: GroupingPolicy): string {
  const dimensions =
    policy.dimension_keys.length === 0
      ? "no dimensions"
      : policy.dimension_keys.join(", ");
  return `#${policy.id} ${policy.name} (${dimensions}, ${enabledLabel(policy.enabled)})`;
}

function notificationChannelLabel(channel: NotificationChannelProfile): string {
  const scopes =
    channel.delivery_scopes.length === 0
      ? "no scopes"
      : channel.delivery_scopes.join(", ");
  return `#${channel.id} ${channel.name} (${scopes}, ${enabledLabel(channel.enabled)})`;
}

function enabledLabel(enabled: boolean): string {
  return enabled ? "enabled" : "disabled";
}

function relationLabel(
  labels: Record<number, string>,
  id: number,
  fallback: string,
): string {
  return labels[id] ?? fallback;
}

function reportWorkflowPolicyReadKey(policyID: number): string {
  return `reportWorkflowPolicyRead:${policyID}`;
}

function reportWorkflowPolicyManageKey(policyID: number): string {
  return `reportWorkflowPolicyManage:${policyID}`;
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
      aria-label="AI consultation workflow readiness"
      className="panel workflow-readiness-panel"
    >
      <div className="panel-header workflow-readiness-header">
        <h2>AI Consultation Workflow</h2>
        <Tag color={readinessTagColor(overallStatus)}>
          {readinessLabel(overallStatus)}
        </Tag>
      </div>
      <div className="panel-body workflow-readiness-body">
        {selectedPolicy === null ? (
          <Empty
            description="No workflow policy configured."
            image={<BranchesOutlined aria-hidden />}
          />
        ) : (
          <>
            <div className="workflow-readiness-selected">
              <div>
                <Typography.Text className="muted">
                  Selected policy
                </Typography.Text>
                <Typography.Title level={3}>
                  {selectedPolicy.name}
                </Typography.Title>
              </div>
              <Space wrap>
                <Tag color={selectedPolicy.enabled ? "green" : "default"}>
                  {selectedPolicy.enabled ? "Enabled" : "Draft"}
                </Tag>
                <Tag
                  color={followUpTagColor(selectedPolicy.diagnosis_follow_up)}
                >
                  {selectedPolicy.diagnosis_follow_up}
                </Tag>
                {impact ? (
                  <Tag color={impactStatusColor(impact.status)}>
                    Impact {impact.status}
                  </Tag>
                ) : (
                  <Tag>Impact pending</Tag>
                )}
              </Space>
            </div>

            <div className="workflow-readiness-steps-wrap">
              <Steps
                aria-label="Selected policy readiness"
                className="workflow-readiness-steps"
                current={currentStep}
                items={stages.map((stage) => ({
                  description: stage.detail,
                  status: readinessStepStatus(stage.status),
                  title: stage.title,
                }))}
                responsive={false}
              />
            </div>
          </>
        )}

        <Row aria-label="AI consultation workflow counters" gutter={[12, 12]}>
          <ReadinessMetric
            label="Room-ready policies"
            status="ready"
            value={activeRoomPolicies}
          />
          <ReadinessMetric
            label="Report delivery"
            status={reportDeliveryPolicies > 0 ? "ready" : "pending"}
            value={reportDeliveryPolicies}
          />
          <ReadinessMetric
            label="Ready previews"
            status={readyPreviews > 0 ? "ready" : "pending"}
            value={readyPreviews}
          />
          <ReadinessMetric
            label="Blocked previews"
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
  return (
    <Col lg={6} sm={12} xs={24}>
      <div className="workflow-readiness-metric">
        <div className="workflow-readiness-metric-value">{value}</div>
        <div className="workflow-readiness-metric-footer">
          <Typography.Text className="muted">{label}</Typography.Text>
          <Tag color={readinessTagColor(status)}>{readinessLabel(status)}</Tag>
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
  return (
    <div
      aria-label="Diagnosis tool readiness"
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status)}
          </Tag>
          <Tag color="blue">
            Active alerts {readiness.activeAlertsForSource}
          </Tag>
          <Tag color="cyan">
            Metric tools {readiness.enabledMetricTemplates}
          </Tag>
          <Tag color="purple">
            Range tools {readiness.enabledRangeTemplates}
          </Tag>
        </Space>
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
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
  return (
    <div
      aria-label="Workflow automation outcome"
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>Automation Outcome</Typography.Text>
        <Tag color={readinessTagColor(outcome.status)}>
          {readinessLabel(outcome.status)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{outcome.detail}</Typography.Text>
      <div className="workflow-automation-grid">
        {outcome.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{item.title}</Typography.Text>
              <Tag color={readinessTagColor(item.status)}>
                {readinessLabel(item.status)}
              </Tag>
            </div>
            <Typography.Text strong>{item.value}</Typography.Text>
            <Typography.Text type="secondary">{item.detail}</Typography.Text>
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
  const currentStep = readiness.items.findIndex(
    (item) => item.status !== "ready",
  );

  return (
    <div
      aria-label="Auto-room readiness checklist"
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Tag color={readinessTagColor(readiness.status)}>
          {readinessLabel(readiness.status)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
      {readiness.items.length > 0 ? (
        <Steps
          current={currentStep === -1 ? readiness.items.length : currentStep}
          direction="vertical"
          items={readiness.items.map((item) => ({
            description: (
              <Space direction="vertical" size={2}>
                <Typography.Text strong>{item.value}</Typography.Text>
                <Typography.Text type="secondary">
                  {item.detail}
                </Typography.Text>
              </Space>
            ),
            status: readinessStepStatus(item.status),
            title: item.title,
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
  return (
    <div
      aria-label="Workflow setup blueprint"
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>Setup Blueprint</Typography.Text>
        <Tag color={readinessTagColor(blueprint.status)}>
          {readinessLabel(blueprint.status)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{blueprint.detail}</Typography.Text>
      <div
        aria-label="Workflow setup chain"
        className="workflow-automation-grid"
      >
        {blueprint.phases.map((phase) => (
          <div className="workflow-automation-item" key={phase.key}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">
                {phase.title}
              </Typography.Text>
              <Tag color={readinessTagColor(phase.status)}>
                {readinessLabel(phase.status)}
              </Tag>
            </div>
            <Typography.Text strong>{phase.value}</Typography.Text>
            <Typography.Text type="secondary">{phase.detail}</Typography.Text>
          </div>
        ))}
      </div>
      {blueprint.actions.length === 0 ? (
        <Alert
          description="Save the policy, run impact preview, then replay a bounded window to retain proof."
          message={blueprint.label}
          showIcon
          type="success"
        />
      ) : (
        <div className="workflow-automation-grid">
          {blueprint.actions.map((action) => (
            <div className="workflow-automation-item" key={action.key}>
              <div className="workflow-automation-item-header">
                <Typography.Text className="muted">
                  {action.title}
                </Typography.Text>
                <Tag color={readinessTagColor(action.status)}>
                  {readinessLabel(action.status)}
                </Tag>
              </div>
              <Typography.Text type="secondary">
                {action.detail}
              </Typography.Text>
              {action.actionHref ? (
                <Button href={action.actionHref} size="small" type="link">
                  {action.actionLabel}
                </Button>
              ) : (
                <Typography.Text className="muted">
                  {action.actionLabel}
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
  const currentStep = plan.steps.findIndex((step) => step.status !== "ready");

  return (
    <div
      aria-label="Draft workflow execution plan"
      className="settings-preview-panel"
    >
      <div className="settings-preview-header">
        <Typography.Text strong>Draft Execution Plan</Typography.Text>
        <Tag color={readinessTagColor(plan.status)}>
          {readinessLabel(plan.status)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{plan.detail}</Typography.Text>
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
          title: step.title,
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
  if (step.title !== "Impact preview") {
    return step.detail;
  }
  const savedLoading = policy !== null && impactingID === policy.id;
  const draftPreviewDisabled =
    step.status === "blocked" || impactingID !== null || !canPreviewDraftImpact;
  const savedPreviewDisabled =
    draftImpacting || (impactingID !== null && !savedLoading) || !canPreviewPolicy;

  return (
    <Space direction="vertical" size={6}>
      <Typography.Text type="secondary">{step.detail}</Typography.Text>
      {policy !== null && !draftMatchesSaved ? (
        <Alert
          description="Preview draft estimates current form values without saving. Preview saved policy uses the last saved version."
          message="Unsaved edits are available only in draft preview."
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
            Preview draft
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
            Preview saved policy
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
  return (
    <div
      aria-label="Alert source webhook readiness"
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status)}
          </Tag>
          <Tag color="geekblue">Alertmanager webhook</Tag>
        </Space>
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
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
  return (
    <div
      aria-label="Notification channel readiness"
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={10}>
        <Space wrap>
          <Tag color={readinessTagColor(readiness.status)}>
            {readinessLabel(readiness.status)}
          </Tag>
          <Tag color={operatorChannelTagColor(operatorReadiness)}>
            {operatorReadiness.kindLabel}
          </Tag>
          {selectedChannel === null ? null : (
            <Tag color={selectedChannel.enabled ? "green" : "default"}>
              {selectedChannel.enabled ? "Enabled" : "Disabled"}
            </Tag>
          )}
          <Tag color="blue">Required {readiness.requiredScopes.join(", ")}</Tag>
          {readiness.missingScopes.length > 0 ? (
            <Tag color="red">Missing {readiness.missingScopes.join(", ")}</Tag>
          ) : null}
        </Space>
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
        <Typography.Text type="secondary">
          {operatorReadiness.detail}
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

function readinessLabel(status: ReadinessStatus): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "review":
      return "Review";
    case "pending":
      return "Pending";
    case "blocked":
      return "Blocked";
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
  return (
    <Alert
      description={notice.message}
      message={notice.kind === "error" ? "Request failed" : "Settings"}
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
            Enable policy #{primaryCandidate.policy.id}
          </Button>
        )
      }
      description={
        <Space direction="vertical" size={4}>
          <Typography.Text>
            {workflowReturnCandidateSummary(candidates)}
          </Typography.Text>
          <Space size={4} wrap>
            {candidates.slice(0, 4).map((candidate) => (
              <Tooltip key={candidate.policy.id} title={candidate.detail}>
                <Tag color={workflowReturnCandidateTagColor(candidate.action)}>
                  #{candidate.policy.id} {candidate.policy.name}:{" "}
                  {workflowReturnCandidateLabel(candidate.action)}
                </Tag>
              </Tooltip>
            ))}
          </Space>
        </Space>
      }
      message="AI room workflow candidates"
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
  const columns: TableColumnsType<ReportWorkflowPolicy> = [
    {
      key: "name",
      title: "Name",
      render: (_, policy) => (
        <Space direction="vertical" size={2}>
          <Space size={4} wrap>
            <Typography.Text strong>{policy.name}</Typography.Text>
            {highlightPolicyIDs.has(policy.id) ? (
              <Tag color="gold">Return target</Tag>
            ) : null}
          </Space>
          <Typography.Text type="secondary">
            {relationLabel(
              relationOptions.alertSourceLabels,
              policy.alert_source_profile_id,
              `Source #${policy.alert_source_profile_id}`,
            )}
          </Typography.Text>
          <Typography.Text type="secondary">
            {relationLabel(
              relationOptions.groupingPolicyLabels,
              policy.grouping_policy_id,
              `Grouping #${policy.grouping_policy_id}`,
            )}
          </Typography.Text>
        </Space>
      ),
    },
    {
      dataIndex: "report_scenario",
      key: "scenario",
      title: "Scenario",
      render: (scenario: ReportWorkflowPolicy["report_scenario"]) => (
        <Tag>{scenario}</Tag>
      ),
    },
    {
      dataIndex: "diagnosis_follow_up",
      key: "followup",
      title: "Follow-up",
      render: (mode: ReportWorkflowPolicy["diagnosis_follow_up"]) => (
        <Tag color={followUpTagColor(mode)}>{mode}</Tag>
      ),
    },
    {
      dataIndex: "report_notification_channel_profile_id",
      key: "reportChannel",
      title: "Report channel",
      render: (
        profileID: ReportWorkflowPolicy["report_notification_channel_profile_id"],
      ) =>
        profileID === null ? (
          <Tag>None</Tag>
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
      title: "State",
      render: (enabled: boolean, policy) => (
        <Space direction="vertical" size={2}>
          <Tag color={enabled ? "green" : "default"}>
            {enabled ? "Enabled" : "Draft"}
          </Tag>
          <Typography.Text type="secondary">
            {enabled
              ? nullableDate(policy.enabled_at)
              : nullableDate(policy.disabled_at)}
          </Typography.Text>
        </Space>
      ),
    },
    {
      key: "impact",
      title: "Impact",
      render: (_, policy) => (
        <ImpactSummary result={impactResults[policy.id]} />
      ),
    },
    {
      key: "enablement",
      title: "Enablement",
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
      title: "Repair",
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
      title: "Updated",
      render: (value: string) => formatDateTime(value),
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
              Edit
            </Button>
            <Button
              disabled={busy || actionID !== null || !policy.enabled || !canManage}
              icon={<ThunderboltOutlined />}
              onClick={() => onReplay(policy)}
              size="small"
            >
              Replay
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
              Impact
            </Button>
            {policy.enabled ? (
              <Button
                disabled={busy || actionID !== null || !canManage}
                icon={<PauseCircleOutlined />}
                loading={actionID === policy.id}
                onClick={() => onDisable(policy)}
                size="small"
              >
                Disable
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
      title: "Actions",
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
              emptyDescription: "No report workflow policies",
              resourceLabel: "report workflow policies",
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
          {readinessLabel(blueprint.status)}
        </Tag>
        <Typography.Text type="secondary">No repair needed</Typography.Text>
      </Space>
    );
  }
  return (
    <Space direction="vertical" size={4}>
      <Space wrap>
        <Tag color={readinessTagColor(blueprint.status)}>
          {readinessLabel(blueprint.status)}
        </Tag>
        <Tooltip title={blueprint.detail}>
          <Typography.Text type="secondary">{blueprint.label}</Typography.Text>
        </Tooltip>
      </Space>
      <Typography.Text type="secondary">
        {policyRepairSummaryDetail(blueprint)}
      </Typography.Text>
      {visibleLinkedActions.map((action) => (
        <Tooltip key={action.key} title={action.detail}>
          <Button
            href={action.actionHref}
            icon={<ToolOutlined />}
            size="small"
            type="link"
          >
            {action.actionLabel}
          </Button>
        </Tooltip>
      ))}
      {unlinkedActionCount > 0 ? (
        <Tooltip title={blueprint.detail}>
          <Typography.Text type="secondary">
            {unlinkedActionCount} manual repair item
            {unlinkedActionCount === 1 ? "" : "s"}
          </Typography.Text>
        </Tooltip>
      ) : null}
      {overflowLinkedActionCount > 0 ? (
        <Typography.Text type="secondary">
          +{overflowLinkedActionCount} more
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
      Enable
    </Button>
  );

  if (!blocked) {
    return button;
  }
  return (
    <Tooltip title={readiness.detail}>
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
        {readinessLabel(readiness.status)}
      </Tag>
      <Typography.Text type="secondary">{readiness.label}</Typography.Text>
      <Typography.Text type="secondary">
        {policyEnablementSummaryDetail(readiness)}
      </Typography.Text>
      {blockerCount + warningCount > 0 ? (
        <Space size={4} wrap>
          {blockerCount > 0 ? (
            <Tooltip title={readiness.blockers.join(" ")}>
              <Tag color="red">
                {blockerCount} blocker{blockerCount === 1 ? "" : "s"}
              </Tag>
            </Tooltip>
          ) : null}
          {warningCount > 0 ? (
            <Tooltip title={readiness.warnings.join(" ")}>
              <Tag color="gold">
                {warningCount} review item{warningCount === 1 ? "" : "s"}
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
  if (!result) {
    return <Typography.Text type="secondary">Not previewed</Typography.Text>;
  }
  return (
    <Space direction="vertical" size={2}>
      <Tag color={impactStatusColor(result.status)}>{result.status}</Tag>
      <Typography.Text type="secondary">
        {result.groups_estimated} groups / {result.events_matched} events
      </Typography.Text>
    </Space>
  );
}

type ImpactPreviewModalProps = {
  onCancel: () => void;
  preview: ImpactPreviewState | null;
};

function ImpactPreviewModal({ onCancel, preview }: ImpactPreviewModalProps) {
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
          Close
        </Button>
      }
      onCancel={onCancel}
      open={preview !== null}
      title={preview?.title ?? "Impact Preview"}
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
                <Typography.Text>{result.message}</Typography.Text>
                <Space direction="vertical" size={6}>
                  {reasons.map((reason) => (
                    <Space key={reason.code} size={[6, 4]} wrap>
                      <Tooltip title={reason.code}>
                        <Tag color={reason.tagColor}>{reason.label}</Tag>
                      </Tooltip>
                      <Typography.Text type="secondary">
                        {reason.detail}
                      </Typography.Text>
                    </Space>
                  ))}
                </Space>
              </Space>
            }
            message={
              <Tag color={impactStatusColor(result.status)}>
                {result.status}
              </Tag>
            }
            showIcon
            type={impactAlertType(result.status)}
          />

          <Row gutter={[12, 12]}>
            <Col sm={6} xs={24}>
              <Statistic title="Events scanned" value={result.events_scanned} />
            </Col>
            <Col sm={6} xs={24}>
              <Statistic title="Events matched" value={result.events_matched} />
            </Col>
            <Col sm={6} xs={24}>
              <Statistic
                title="Groups estimated"
                value={result.groups_estimated}
              />
            </Col>
            <Col sm={6} xs={24}>
              <Statistic
                title="AI diagnosis"
                value={diagnosisEstimate?.value ?? "-"}
              />
            </Col>
          </Row>

          {diagnosisEstimate === null ? null : (
            <Alert
              description={diagnosisEstimate.detail}
              message={
                <Space size={[6, 4]} wrap>
                  <Tag color={readinessTagColor(diagnosisEstimate.status)}>
                    {readinessLabel(diagnosisEstimate.status)}
                  </Tag>
                  <span>{diagnosisEstimate.label}</span>
                </Space>
              }
              showIcon
              type={readinessAlertType(diagnosisEstimate.status)}
            />
          )}

          <Row gutter={[12, 12]}>
            <Col md={8} xs={24}>
              <ReadinessLine
                label="Alert source"
                ready={result.alert_source_enabled}
                text={`#${result.alert_source_profile_id} ${result.alert_source_kind}/${result.alert_source_auth_mode}`}
              />
            </Col>
            <Col md={8} xs={24}>
              <ReadinessLine
                label="Grouping policy"
                ready={result.grouping_policy_enabled}
                text={`#${result.grouping_policy_id} ${result.grouping_dimension_keys.join(", ")}`}
              />
            </Col>
            <Col md={8} xs={24}>
              <ReadinessLine
                label="Report channel"
                ready={reportChannelReadiness?.ready ?? true}
                text={reportChannelReadiness?.text ?? "No report channel bound"}
              />
            </Col>
          </Row>

          <Table<ImpactPreviewGroup>
            columns={impactGroupColumns}
            dataSource={result.groups}
            locale={{ emptyText: <Empty description="No estimated groups." /> }}
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
  return (
    <Space direction="vertical" size={2}>
      <Typography.Text type="secondary">{label}</Typography.Text>
      <Space wrap>
        <Tag color={ready ? "green" : "red"}>{ready ? "ready" : "blocked"}</Tag>
        <Typography.Text>{text}</Typography.Text>
      </Space>
    </Space>
  );
}

const impactGroupColumns: TableColumnsType<ImpactPreviewGroup> = [
  {
    dataIndex: "dimensions",
    key: "dimensions",
    title: "Dimensions",
    render: (_value, group) => <DimensionTags values={group.dimensions} />,
  },
  {
    dataIndex: "severity",
    key: "severity",
    title: "Severity",
    render: (_value, group) => (
      <Tag color={severityColor(group.severity)}>{group.severity}</Tag>
    ),
  },
  {
    dataIndex: "event_count",
    key: "event_count",
    title: "Events",
  },
  {
    dataIndex: "first_seen_at",
    key: "first_seen_at",
    title: "First Seen",
    render: (_value, group) => formatDateTime(group.first_seen_at),
  },
  {
    dataIndex: "last_seen_at",
    key: "last_seen_at",
    title: "Last Seen",
    render: (_value, group) => formatDateTime(group.last_seen_at),
  },
  {
    dataIndex: "event_ids",
    key: "event_ids",
    title: "Event IDs",
    render: (_value, group) => (
      <Typography.Text className="settings-event-ids">
        {group.event_ids.join(", ")}
      </Typography.Text>
    ),
  },
];

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
  const proofTrace =
    result === null ? null : reportWorkflowPolicyReplayProofTrace(result);

  return (
    <Modal
      destroyOnHidden
      footer={null}
      onCancel={onCancel}
      open={policy !== null}
      title={policy === null ? "Replay Policy" : `Replay Policy #${policy.id}`}
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
              label="Window start"
              name="windowStart"
              rules={[{ required: true, message: "Window start is required." }]}
            >
              <Input autoComplete="off" placeholder="2026-06-05T08:00:00Z" />
            </Form.Item>
          </Col>
          <Col sm={12} xs={24}>
            <Form.Item
              label="Window end"
              name="windowEnd"
              rules={[{ required: true, message: "Window end is required." }]}
            >
              <Input autoComplete="off" placeholder="2026-06-05T09:00:00Z" />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item
          label="Limit"
          name="limit"
          rules={[{ required: true, message: "Limit is required." }]}
        >
          <InputNumber
            max={100000}
            min={1}
            precision={0}
            style={{ width: "100%" }}
          />
        </Form.Item>
        <Form.Item label="Correlation key" name="correlationKey">
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item label="Workflow ID" name="workflowID">
          <Input autoComplete="off" />
        </Form.Item>

        {result === null ? null : (
          <Alert
            className="settings-action-result"
            message={result.started ? "Workflow accepted" : "Replay completed"}
            showIcon
            type={result.started ? "success" : "warning"}
            description={
              <Space direction="vertical" size={2}>
                <Typography.Text>
                  {result.workflow_id === ""
                    ? "No workflow started"
                    : `${result.workflow_id} / ${result.run_id}`}
                </Typography.Text>
                <Typography.Text
                  className="settings-event-ids"
                  copyable
                  type="secondary"
                >
                  Correlation {result.correlation_key}
                </Typography.Text>
                <Typography.Text type="secondary">
                  Groups {result.stats.groups_built}, snapshots{" "}
                  {result.stats.snapshots_saved}
                </Typography.Text>
                {result.auto_diagnosis ? (
                  <Typography.Text type="secondary">
                    {autoDiagnosisReplaySummary(result.auto_diagnosis)}
                  </Typography.Text>
                ) : null}
                {result.auto_diagnosis &&
                result.auto_diagnosis.rooms_skipped > 0 ? (
                  <Typography.Text type="secondary">
                    {pluralizeCount(
                      result.auto_diagnosis.rooms_skipped,
                      "snapshot",
                    )}{" "}
                    retained for manual AI room creation after the automatic
                    room limit was reached.
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
            Start Replay
          </Button>
          <Button disabled={busy} onClick={onCancel} type="default">
            Close
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
  return (
    <div aria-label="Replay proof trace" className="settings-proof-outcome">
      <div className="settings-preview-header">
        <Typography.Text strong>Replay Proof Trace</Typography.Text>
        <Tag color={readinessTagColor(trace.status)}>
          {readinessLabel(trace.status)}
        </Tag>
      </div>
      <Typography.Text type="secondary">{trace.detail}</Typography.Text>
      <div className="workflow-automation-grid">
        {trace.items.map((item) => (
          <div className="workflow-automation-item" key={item.title}>
            <div className="workflow-automation-item-header">
              <Typography.Text className="muted">{item.title}</Typography.Text>
              <Tag color={readinessTagColor(item.status)}>
                {readinessLabel(item.status)}
              </Tag>
            </div>
            <Typography.Text strong>{item.value}</Typography.Text>
            <Typography.Text type="secondary">{item.detail}</Typography.Text>
            {item.actions && item.actions.length > 0 ? (
              <Space size={[4, 4]} wrap>
                {item.actions.map((action) => (
                  <Tooltip
                    key={`${item.title}:${action.href}`}
                    title={action.detail}
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
): string {
  const parts = [
    pluralizeCount(autoDiagnosis.policies_matched, "policy"),
    pluralizeCount(autoDiagnosis.snapshots, "snapshot"),
    `${pluralizeCount(autoDiagnosis.rooms_started, "room")} started`,
  ];
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
  const entries = Object.entries(values).sort(([left], [right]) =>
    left.localeCompare(right),
  );
  if (entries.length === 0) {
    return <Typography.Text type="secondary">None</Typography.Text>;
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

function nullableDate(value: string | null): string {
  if (value === null) {
    return "Not set";
  }
  return formatDateTime(value);
}
