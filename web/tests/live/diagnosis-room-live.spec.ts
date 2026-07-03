import { type Locator, type Page, expect, test } from "@playwright/test";
import { Buffer } from "node:buffer";
import { createHash } from "node:crypto";
import { writeFile } from "node:fs/promises";

type LiveToolRequest = {
  template_id?: number;
  alert_source_profile_id?: number;
  tool: "active_alerts" | "metric_query" | "metric_range_query";
  reason: string;
  query?: string;
  window_seconds?: number;
  step_seconds?: number;
  limit?: number;
};

type LiveBackendRoomSnapshot = {
  collectionResults: LiveEvidenceCollectionResult[];
  evidenceTimeline: LiveEvidenceTimelineEntry[];
  inFlight: boolean;
  latestError?: LiveBackendRoomError;
  status: string;
  turnCount: number;
};

type LiveBackendRoomError = {
  code?: string;
  message?: string;
};

type LiveEvidenceCollectionResult = {
  request?: Partial<LiveToolRequest>;
  template_id?: number;
  alert_source_profile_id?: number;
  tool?: string;
  status?: string;
  reason_code?: string;
  query?: string;
  window_seconds?: number;
  step_seconds?: number;
  limit?: number;
};

type LiveEvidenceTimelineEntry = {
  evidence_collection_results?: LiveEvidenceCollectionResult[];
};

type LiveAuth =
  | { mode: "bearer"; token: string }
  | { mode: "ldap"; username: string; password: string };

const sessionID = requiredEnv("OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID");
const liveAPIBaseURL = requiredEnv("OPENCLARION_LIVE_API_BASE_URL").replace(/\/+$/, "");
const liveAuth = liveAuthEnv();
const seededToolRequests = liveToolRequestsEnv("OPENCLARION_LIVE_TOOL_REQUESTS_JSON");
const stagedOperatorToolRequests = liveToolRequestsEnv("OPENCLARION_LIVE_OPERATOR_TOOL_REQUESTS_JSON");
const message = process.env.OPENCLARION_LIVE_DIAGNOSIS_MESSAGE?.trim() || defaultLiveDiagnosisMessage(seededToolRequests);
const liveTurnTimeoutMS = positiveIntegerEnv("OPENCLARION_LIVE_TURN_TIMEOUT_MS", 300_000);
const confirmConclusion = truthyEnv("OPENCLARION_LIVE_CONFIRM_CONCLUSION");
const collectPlannedEvidence = truthyEnv("OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE");
const submitSupplementalEvidence = truthyEnv("OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE");
const requireSupplementalEvidence = truthyEnv("OPENCLARION_LIVE_REQUIRE_SUPPLEMENTAL_EVIDENCE");
const supplementalEvidenceTextOverride = process.env.OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT?.trim();
const supplementalEvidenceTemplate =
  process.env.OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE?.trim() ||
  [
    'Live smoke supplemental evidence for "{label}".',
    "Request priority: {priority}.",
    "Requested detail: {detail}.",
    "Operator reviewed this requested follow-up against the retained alert snapshot.",
    "If the requested artifact is unavailable in this validation window, treat that as an explicit residual-review caveat rather than repeating the same request.",
    "Use this targeted operator update to reassess confidence and produce ready_for_review with requires_human_review=true when no additional executable evidence is needed."
  ].join("\n");

test("diagnosis room completes one turn against a live backend", async ({ page }) => {
  await page.goto("/diagnosis-room");

  await expect(page.getByRole("heading", { name: "Diagnosis Room" })).toBeVisible();
  await page.getByLabel("Session ID").fill(sessionID);
  await fillLiveAuthorization(page, liveAuth);
  await page.getByRole("button", { name: /Connect/ }).click();

  await expect(page.getByRole("status", { name: "Connection status" })).toHaveText("connected", {
    timeout: 30_000
  });
  const loadedStateLogs = page.getByText(/Loaded state:/);
  await expect(loadedStateLogs.first()).toBeVisible({ timeout: 30_000 });

  const assistantTurns = page.locator(".diagnosis-turn-assistant");
  const assistantTurnsBefore = await assistantTurns.count();
  const transcriptTurnsBefore = await page.locator(".diagnosis-turn").count();

  await page.getByLabel("Message").fill(message);
  await page.getByRole("button", { name: "Send" }).click();

  const submittedMessage = page.getByText(message, { exact: true });
  await expect(submittedMessage).toBeVisible();
  const observedAssistantTurnsAfter = await waitForAssistantTurnOrWorkflowError(
    page,
    assistantTurns,
    assistantTurnsBefore,
    liveTurnTimeoutMS
  );
  let assistantTurnsAfter = Math.max(await assistantTurns.count(), observedAssistantTurnsAfter);
  let assistantTurnDelta = assistantTurnsAfter - assistantTurnsBefore;
  let completedTurnEvidenceText = await waitForTurnCompletionEvidence(page, assistantTurnsAfter);
  const connectionStatus = page.getByRole("status", { name: "Connection status" });
  await expect(connectionStatus).toHaveText("connected");
  const consultationInsight = page.getByText("Consultation Insight", { exact: true });
  await expect(consultationInsight).toBeVisible();
  const consultationProgress = page.locator('[aria-label="Diagnosis consultation progress"]');
  await expect(consultationProgress).toBeVisible();
  const diagnosisConfidence = consultationProgress.locator('[aria-label="Diagnosis confidence"]');
  await expect(diagnosisConfidence).toBeVisible();
  const evidenceReadiness = page.locator('[aria-label="Evidence readiness"]');
  await expect(evidenceReadiness).toBeVisible();
  const evidencePlanSection = insightSectionByHeading(page, "Executable Evidence Plan");
  await expect(evidencePlanSection).toBeVisible();
  const evidencePlanCount = await evidencePlanSection.locator(".diagnosis-evidence-item").count();
  let toolRequestSeedProof = await evidencePlanSeedProof(evidencePlanSection, seededToolRequests);
  const collectionResultSection = insightSectionByHeading(page, "Collection Results");
  await expect(collectionResultSection).toBeVisible();
  let evidenceCollectionResultCount = await collectionResultSection.locator(".diagnosis-evidence-item").count();
  const evidenceCollectionSummary = collectionResultSection.locator('[aria-label="Evidence collection summary"]');
  const evidenceCollectionSummaryVisible = await evidenceCollectionSummary.isVisible();
  let confidenceAriaValue = ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim();
  const seededOperatorCollectionProof = await maybeCollectSeededOperatorEvidence(
    page,
    assistantTurns,
    assistantTurnsAfter,
    diagnosisConfidence,
    collectionResultSection,
    evidenceCollectionResultCount,
    toolRequestSeedProof
  );
  if (typeof seededOperatorCollectionProof.tool_request_seed_matched_count === "number") {
    toolRequestSeedProof = {
      tool_request_seed_matched_count: seededOperatorCollectionProof.tool_request_seed_matched_count,
      tool_request_seed_missing: String(seededOperatorCollectionProof.tool_request_seed_missing ?? "")
    };
  }
  if (typeof seededOperatorCollectionProof.operator_seed_collection_assistant_turns_after === "number") {
    assistantTurnsAfter = seededOperatorCollectionProof.operator_seed_collection_assistant_turns_after;
    assistantTurnDelta = assistantTurnsAfter - assistantTurnsBefore;
    confidenceAriaValue = ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim();
    if (typeof seededOperatorCollectionProof.operator_seed_collection_result_count_after === "number") {
      evidenceCollectionResultCount = seededOperatorCollectionProof.operator_seed_collection_result_count_after;
    }
  }
  const stagedOperatorCollectionProof = await maybeCollectStagedOperatorEvidence(
    page,
    assistantTurns,
    assistantTurnsAfter,
    diagnosisConfidence,
    collectionResultSection,
    evidenceCollectionResultCount
  );
  if (typeof stagedOperatorCollectionProof.operator_staged_collection_assistant_turns_after === "number") {
    assistantTurnsAfter = stagedOperatorCollectionProof.operator_staged_collection_assistant_turns_after;
    assistantTurnDelta = assistantTurnsAfter - assistantTurnsBefore;
    confidenceAriaValue = ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim();
    if (typeof stagedOperatorCollectionProof.operator_staged_collection_result_count_after === "number") {
      evidenceCollectionResultCount = stagedOperatorCollectionProof.operator_staged_collection_result_count_after;
    }
  }
  if (
    seededToolRequests.length > 0 &&
    toolRequestSeedProof.tool_request_seed_matched_count !== seededToolRequests.length
  ) {
    throw new Error(`seeded tool requests were not covered: ${toolRequestSeedProof.tool_request_seed_missing}`);
  }
  const plannedEvidenceCollectionProof = await maybeCollectPlannedEvidence(
    page,
    assistantTurns,
    assistantTurnsAfter,
    diagnosisConfidence,
    collectionResultSection,
    evidenceCollectionResultCount
  );
  if (typeof plannedEvidenceCollectionProof.planned_evidence_assistant_turns_after === "number") {
    assistantTurnsAfter = plannedEvidenceCollectionProof.planned_evidence_assistant_turns_after;
    assistantTurnDelta = assistantTurnsAfter - assistantTurnsBefore;
    confidenceAriaValue = ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim();
    if (typeof plannedEvidenceCollectionProof.planned_evidence_collection_result_count_after === "number") {
      evidenceCollectionResultCount = plannedEvidenceCollectionProof.planned_evidence_collection_result_count_after;
    }
  }
  const supplementalEvidenceProof = await maybeSubmitSupplementalEvidence(
    page,
    assistantTurns,
    assistantTurnsAfter,
    diagnosisConfidence
  );
  if (typeof supplementalEvidenceProof.supplemental_assistant_turns_after === "number") {
    assistantTurnsAfter = supplementalEvidenceProof.supplemental_assistant_turns_after;
    assistantTurnDelta = assistantTurnsAfter - assistantTurnsBefore;
    completedTurnEvidenceText = await waitForTurnCompletionEvidence(page, assistantTurnsAfter);
    confidenceAriaValue = ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim();
  }
  const evidenceCollectionSummaryText = await optionalVisibleText(evidenceCollectionSummary);
  const confirmationProof: Record<string, boolean | string> = {
    confirm_conclusion_requested: confirmConclusion
  };
  if (confirmConclusion) {
    const confirmButton = page.getByRole("button", { name: "Confirm Conclusion" });
    const confirmButtonEnabled = await confirmButton.isEnabled();
    confirmationProof.confirm_conclusion_available = confirmButtonEnabled;
    if (!confirmButtonEnabled) {
      await expect(confirmButton).toBeDisabled();
      confirmationProof.confirm_conclusion_blocked = true;
      confirmationProof.confirm_conclusion_block_reason = "diagnosis_not_ready_for_confirmation";
    } else {
      await expect(confirmButton).toBeEnabled();
      await confirmButton.click();
      await expect(page.getByText("Confirming final conclusion.")).toBeVisible();

      const closedStatePattern = new RegExp(`Loaded state: closed, ${assistantTurnsAfter} turn\\(s\\)\\.`);
      const closedStateLog = page.getByText(closedStatePattern).last();
      await expect(closedStateLog).toBeVisible({ timeout: 30_000 });
      const finalConclusion = page.locator(".diagnosis-conclusion");
      await expect(finalConclusion).toBeVisible();
      await expect(page.getByText("Final conclusion", { exact: true })).toBeVisible();
      await expect(page.getByText("human_confirmed", { exact: true })).toBeVisible();
      await expect(page.getByText("diagnosis-room-close.v1", { exact: true })).toBeVisible();
      await expect(confirmButton).toBeDisabled();

      confirmationProof.final_conclusion_confirmed = true;
      confirmationProof.final_conclusion_visible = await finalConclusion.isVisible();
      confirmationProof.confirmed_state_text = normalizeProofText((await closedStateLog.textContent()) ?? "");
      confirmationProof.connection_status_after_confirm = ((await connectionStatus.textContent()) ?? "").trim();
      confirmationProof.confirm_button_disabled_after_confirm = await confirmButton.isDisabled();
      confirmationProof.close_reason_visible = await page.getByText("human_confirmed", { exact: true }).isVisible();
      confirmationProof.conclusion_version_visible = await page
        .getByText("diagnosis-room-close.v1", { exact: true })
        .isVisible();
    }
  }
  completedTurnEvidenceText = await waitForTurnCompletionEvidence(page, assistantTurnsAfter);

  await writeLiveBrowserProof({
    state_loaded: (await loadedStateLogs.count()) > 0,
    turn_result_observed: completedTurnEvidenceText !== "",
    assistant_turns_before: assistantTurnsBefore,
    assistant_turns_after: assistantTurnsAfter,
    assistant_turn_delta: assistantTurnDelta,
    transcript_messages_before: transcriptTurnsBefore,
    transcript_messages_after: await page.locator(".diagnosis-turn").count(),
    connection_status_after_turn: ((await connectionStatus.textContent()) ?? "").trim(),
    submitted_message_visible: await submittedMessage.isVisible(),
    submitted_message_length: message.length,
    submitted_message_sha256: sha256Hex(message),
    completed_turn_text: completedTurnEvidenceText,
    consultation_insight_visible: await consultationInsight.isVisible(),
    consultation_progress_visible: await consultationProgress.isVisible(),
    evidence_readiness_visible: await evidenceReadiness.isVisible(),
    confidence: confidenceFromAriaValue(confidenceAriaValue),
    confidence_aria_value: confidenceAriaValue,
    evidence_readiness_text: normalizeProofText((await evidenceReadiness.textContent()) ?? ""),
    tool_request_seed_requested: seededToolRequests.length > 0,
    tool_request_seed_count: seededToolRequests.length,
    ...toolRequestSeedProof,
    ...seededOperatorCollectionProof,
    ...stagedOperatorCollectionProof,
    evidence_plan_count: evidencePlanCount,
    evidence_collection_result_count: evidenceCollectionResultCount,
    evidence_collection_summary_visible: evidenceCollectionSummaryVisible || evidenceCollectionSummaryText !== "",
    evidence_collection_summary_text: evidenceCollectionSummaryText,
    ...plannedEvidenceCollectionProof,
    ...supplementalEvidenceProof,
    ...confirmationProof
  });
});

function requiredEnv(name: string): string {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required for live diagnosis-room browser smoke`);
  }
  return value;
}

function positiveIntegerEnv(name: string, fallback: number): number {
  const value = process.env[name]?.trim();
  if (!value) {
    return fallback;
  }
  if (!/^[1-9][0-9]*$/.test(value)) {
    throw new Error(`${name} must be a positive integer`);
  }
  return Number(value);
}

function truthyEnv(name: string): boolean {
  const value = process.env[name]?.trim().toLowerCase();
  return value === "1" || value === "true" || value === "yes";
}

function liveAuthEnv(): LiveAuth {
  const rawMode = process.env.OPENCLARION_LIVE_AUTH_MODE?.trim().toLowerCase();
  const inferredMode =
    process.env.OPENCLARION_LIVE_LDAP_USERNAME !== undefined ||
    process.env.OPENCLARION_LIVE_LDAP_PASSWORD !== undefined
      ? "ldap"
      : "bearer";
  const mode = rawMode || inferredMode;
  if (mode === "ldap") {
    const username = requiredEnv("OPENCLARION_LIVE_LDAP_USERNAME");
    const password = requiredSecretEnv("OPENCLARION_LIVE_LDAP_PASSWORD");
    if (/[\s]/.test(username)) {
      throw new Error("OPENCLARION_LIVE_LDAP_USERNAME must not contain whitespace");
    }
    if (/[\r\n]/.test(password)) {
      throw new Error("OPENCLARION_LIVE_LDAP_PASSWORD must not contain CR or LF");
    }
    return { mode: "ldap", username, password };
  }
  if (mode !== "bearer") {
    throw new Error("OPENCLARION_LIVE_AUTH_MODE must be ldap or bearer");
  }
  const token = bearerTokenEnv();
  return { mode: "bearer", token };
}

function bearerTokenEnv(): string {
  const raw = requiredEnv("OPENCLARION_LIVE_BEARER_TOKEN");
  const match = /^Bearer\s+(.+)$/i.exec(raw);
  const token = (match ? match[1] : raw).trim();
  if (token === "" || /[\s]/.test(token)) {
    throw new Error("OPENCLARION_LIVE_BEARER_TOKEN must be a single bearer token or Bearer header");
  }
  return token;
}

function requiredSecretEnv(name: string): string {
  const value = process.env[name];
  if (value === undefined || value === "") {
    throw new Error(`${name} is required for live diagnosis-room browser smoke`);
  }
  return value;
}

function liveAuthorizationHeader(auth: LiveAuth): string {
  if (auth.mode === "bearer") {
    return `Bearer ${auth.token}`;
  }
  return `Basic ${Buffer.from(`${auth.username}:${auth.password}`, "utf8").toString("base64")}`;
}

async function fillLiveAuthorization(page: Page, auth: LiveAuth): Promise<void> {
  const connectionCard = page.locator(".settings-overview-card").filter({ hasText: "Connection" });
  if (auth.mode === "bearer") {
    await connectionCard.getByText("Bearer", { exact: true }).click();
    await page.getByLabel("Bearer token").fill(auth.token);
    return;
  }
  await connectionCard.getByText("LDAP", { exact: true }).click();
  await page.getByLabel("LDAP username").fill(auth.username);
  await page.getByLabel("LDAP password").fill(auth.password);
}

function insightSectionByHeading(page: Page, title: string): Locator {
  return page
    .locator(".diagnosis-insight-section")
    .filter({ has: page.getByRole("heading", { name: title }) })
    .first();
}

async function evidencePlanSeedProof(
  evidencePlanSection: Locator,
  seededRequests: LiveToolRequest[]
): Promise<Record<string, number | string>> {
  if (seededRequests.length === 0) {
    return {
      tool_request_seed_matched_count: 0,
      tool_request_seed_missing: ""
    };
  }
  const itemTexts = (await evidencePlanSection.locator(".diagnosis-evidence-item").allTextContents())
    .map(normalizeProofText);
  const missing = seededRequests
    .filter((request) => !itemTexts.some((itemText) => evidencePlanItemMatchesSeed(itemText, request)))
    .map(evidencePlanSeedLabel);
  return {
    tool_request_seed_matched_count: seededRequests.length - missing.length,
    tool_request_seed_missing: missing.join("; ")
  };
}

function evidencePlanItemMatchesSeed(itemText: string, request: LiveToolRequest): boolean {
  if (!itemText.includes(request.tool)) {
    return false;
  }
  if (request.template_id !== undefined && !itemText.includes(`template #${request.template_id}`)) {
    return false;
  }
  if (
    request.alert_source_profile_id !== undefined &&
    !itemText.includes(`profile #${request.alert_source_profile_id}`)
  ) {
    return false;
  }
  if (request.query !== undefined && !itemText.includes(request.query)) {
    return false;
  }
  if (request.limit !== undefined && !itemText.includes(`limit: ${request.limit}`)) {
    return false;
  }
  if (request.window_seconds !== undefined && !itemText.includes(`window: ${request.window_seconds}s`)) {
    return false;
  }
  if (request.step_seconds !== undefined && !itemText.includes(`step: ${request.step_seconds}s`)) {
    return false;
  }
  return true;
}

function evidencePlanItemMatchesSeedIdentity(itemText: string, request: LiveToolRequest): boolean {
  if (!itemText.includes(request.tool)) {
    return false;
  }
  if (request.template_id !== undefined && !itemText.includes(`template #${request.template_id}`)) {
    return false;
  }
  if (
    request.alert_source_profile_id !== undefined &&
    !itemText.includes(`profile #${request.alert_source_profile_id}`)
  ) {
    return false;
  }
  if (request.query !== undefined && !itemText.includes(request.query)) {
    return false;
  }
  return true;
}

function evidencePlanItemMatchesOperationalIdentity(itemText: string, request: LiveToolRequest): boolean {
  if (!itemText.includes(request.tool)) {
    return false;
  }
  if (request.template_id !== undefined && !itemText.includes(`template #${request.template_id}`)) {
    return false;
  }
  if (
    request.alert_source_profile_id !== undefined &&
    !itemText.includes(`profile #${request.alert_source_profile_id}`)
  ) {
    return false;
  }
  return true;
}

function evidencePlanSeedLabel(request: LiveToolRequest): string {
  const parts = [request.tool];
  if (request.template_id !== undefined) {
    parts.push(`template #${request.template_id}`);
  }
  if (request.alert_source_profile_id !== undefined) {
    parts.push(`profile #${request.alert_source_profile_id}`);
  }
  if (request.query !== undefined) {
    parts.push(`query ${request.query}`);
  }
  return parts.join(" ");
}

function liveToolRequestsEnv(name: string): LiveToolRequest[] {
  const raw = process.env[name]?.trim();
  if (!raw) {
    return [];
  }
  let decoded: unknown;
  try {
    decoded = JSON.parse(raw);
  } catch {
    throw new Error(`${name} must be valid JSON`);
  }
  if (!Array.isArray(decoded)) {
    throw new Error(`${name} must be a JSON array`);
  }
  if (decoded.length === 0) {
    throw new Error(`${name} must include at least one request when set`);
  }
  if (decoded.length > 5) {
    throw new Error(`${name} must include no more than 5 requests`);
  }
  return decoded.map((item, index) => liveToolRequestFromValue(name, item, index));
}

function liveToolRequestFromValue(name: string, value: unknown, index: number): LiveToolRequest {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${name}[${index}] must be an object`);
  }
  const record = value as Record<string, unknown>;
  const allowedFields = new Set([
    "template_id",
    "alert_source_profile_id",
    "tool",
    "reason",
    "query",
    "window_seconds",
    "step_seconds",
    "limit"
  ]);
  for (const key of Object.keys(record)) {
    if (!allowedFields.has(key)) {
      throw new Error(`${name}[${index}] includes an unsupported field`);
    }
  }

  const request: LiveToolRequest = {
    tool: liveToolKind(name, index, record.tool),
    reason: liveToolStringField(name, index, "reason", record.reason)
  };
  const templateID = liveToolOptionalIntegerField(name, index, "template_id", record.template_id);
  const alertSourceProfileID = liveToolOptionalIntegerField(
    name,
    index,
    "alert_source_profile_id",
    record.alert_source_profile_id
  );
  const query = liveToolOptionalStringField(name, index, "query", record.query);
  const windowSeconds = liveToolOptionalIntegerField(name, index, "window_seconds", record.window_seconds);
  const stepSeconds = liveToolOptionalIntegerField(name, index, "step_seconds", record.step_seconds);
  const limit = liveToolOptionalIntegerField(name, index, "limit", record.limit);
  if (templateID !== undefined) {
    request.template_id = templateID;
  }
  if (alertSourceProfileID !== undefined) {
    request.alert_source_profile_id = alertSourceProfileID;
  }
  if (query !== undefined) {
    request.query = query;
  }
  if (windowSeconds !== undefined) {
    request.window_seconds = windowSeconds;
  }
  if (stepSeconds !== undefined) {
    request.step_seconds = stepSeconds;
  }
  if (limit !== undefined) {
    request.limit = limit;
  }
  validateLiveToolRequest(name, index, request);
  return request;
}

function liveToolKind(name: string, index: number, value: unknown): LiveToolRequest["tool"] {
  if (value === "active_alerts" || value === "metric_query" || value === "metric_range_query") {
    return value;
  }
  throw new Error(`${name}[${index}].tool is unsupported`);
}

function liveToolStringField(name: string, index: number, field: string, value: unknown): string {
  if (typeof value !== "string") {
    throw new Error(`${name}[${index}].${field} must be a string`);
  }
  return value;
}

function liveToolOptionalStringField(name: string, index: number, field: string, value: unknown): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  return liveToolStringField(name, index, field, value);
}

function liveToolOptionalIntegerField(name: string, index: number, field: string, value: unknown): number | undefined {
  if (value === undefined) {
    return undefined;
  }
  if (typeof value !== "number" || !Number.isInteger(value)) {
    throw new Error(`${name}[${index}].${field} must be an integer`);
  }
  return value;
}

function validateLiveToolRequest(name: string, index: number, request: LiveToolRequest): void {
  if ((request.template_id ?? 0) < 0 || (request.alert_source_profile_id ?? 0) < 0) {
    throw new Error(`${name}[${index}] identifiers must be positive when set`);
  }
  if ((request.template_id ?? 0) > 0 && (request.alert_source_profile_id ?? 0) === 0) {
    throw new Error(`${name}[${index}] template_id requires alert_source_profile_id`);
  }
  if (request.reason.trim() === "") {
    throw new Error(`${name}[${index}].reason must be non-empty`);
  }
  if (request.reason !== request.reason.trim() || /[\r\n\t]/.test(request.reason)) {
    throw new Error(`${name}[${index}].reason must be a single trimmed line`);
  }
  if (new TextEncoder().encode(request.reason).length > 500) {
    throw new Error(`${name}[${index}].reason exceeds 500 bytes`);
  }
  if (request.query !== undefined) {
    if (request.query !== request.query.trim() || /[\r\n\t]/.test(request.query)) {
      throw new Error(`${name}[${index}].query must be a single trimmed line`);
    }
    if (new TextEncoder().encode(request.query).length > 500) {
      throw new Error(`${name}[${index}].query exceeds 500 bytes`);
    }
  }

  switch (request.tool) {
    case "active_alerts":
      if (request.query !== undefined || request.window_seconds !== undefined || request.step_seconds !== undefined) {
        throw new Error(`${name}[${index}] active_alerts must not include query, window_seconds, or step_seconds`);
      }
      validateLiveToolLimit(name, index, request.limit, 10, "active_alerts");
      return;
    case "metric_query":
      if ((request.template_id ?? 0) === 0 && request.query === undefined) {
        throw new Error(`${name}[${index}] metric_query requires query or template_id`);
      }
      if (request.window_seconds !== undefined || request.step_seconds !== undefined) {
        throw new Error(`${name}[${index}] metric_query must not include window_seconds or step_seconds`);
      }
      validateLiveToolLimit(name, index, request.limit, 20, "metric_query");
      return;
    case "metric_range_query":
      if ((request.template_id ?? 0) === 0 && request.query === undefined) {
        throw new Error(`${name}[${index}] metric_range_query requires query or template_id`);
      }
      if ((request.template_id ?? 0) === 0 && (request.window_seconds === undefined || request.step_seconds === undefined)) {
        throw new Error(`${name}[${index}] metric_range_query requires window_seconds and step_seconds without template_id`);
      }
      if (request.window_seconds !== undefined || request.step_seconds !== undefined) {
        validateLiveToolRange(name, index, request.window_seconds ?? 0, request.step_seconds ?? 0);
      }
      validateLiveToolLimit(name, index, request.limit, 20, "metric_range_query");
      return;
  }
}

function validateLiveToolLimit(
  name: string,
  index: number,
  limit: number | undefined,
  max: number,
  tool: string
): void {
  if (limit === undefined || limit === 0) {
    return;
  }
  if (limit < 1 || limit > max) {
    throw new Error(`${name}[${index}] ${tool} limit must be between 1 and ${max} when set`);
  }
}

function validateLiveToolRange(name: string, index: number, windowSeconds: number, stepSeconds: number): void {
  if (windowSeconds < 15 || windowSeconds > 21_600) {
    throw new Error(`${name}[${index}].window_seconds must be between 15 and 21600`);
  }
  if (stepSeconds < 15 || stepSeconds > 21_600) {
    throw new Error(`${name}[${index}].step_seconds must be between 15 and 21600`);
  }
  if (stepSeconds > windowSeconds) {
    throw new Error(`${name}[${index}].step_seconds must not exceed window_seconds`);
  }
}

function defaultLiveDiagnosisMessage(toolRequests: LiveToolRequest[]): string {
  const timestamp = new Date().toISOString();
  if (toolRequests.length === 0) {
    return `Live browser acceptance ${timestamp}`;
  }
  return [
    `Live browser acceptance ${timestamp}.`,
    "Use the available diagnosis context to request this executable evidence plan on this turn when a matching evidence_request_example exists.",
    "Return matching entries in evidence_requests before finalizing; do not satisfy requested executable items only with missing_evidence_requests or evidence_collection_suggestions.",
    "Do not output tool_request_suggestions at all.",
    "Preserve each requested template_id in evidence_requests, do not copy these entries into tool_request_suggestions, and do not add alert_source_profile_id to any output object.",
    "After collected results arrive, reassess confidence before producing the final conclusion.",
    `Requested evidence plan: ${JSON.stringify(llmPromptToolRequests(toolRequests))}`
  ].join(" ");
}

function llmPromptToolRequests(toolRequests: LiveToolRequest[]): Array<Omit<LiveToolRequest, "alert_source_profile_id">> {
  return toolRequests.map((request) => {
    const promptRequest = { ...request };
    delete promptRequest.alert_source_profile_id;
    return promptRequest;
  });
}

async function waitForAssistantTurnOrWorkflowError(
  page: Page,
  assistantTurns: Locator,
  assistantTurnsBefore: number,
  timeoutMS: number,
  collectionFailure?: { request: LiveToolRequest; section: Locator }
): Promise<number> {
  const deadline = Date.now() + timeoutMS;
  const workflowError = page.locator(".diagnosis-log-error").first();
  const refreshState = page.getByRole("button", { name: "Refresh State" });
  let nextStateRefreshAt = Date.now() + 10_000;
  let nextBackendPollAt = Date.now() + 15_000;

  while (Date.now() < deadline) {
    const observedUITurns = await observedUITurnCount(page, assistantTurns);
    if (observedUITurns > assistantTurnsBefore) {
      return observedUITurns;
    }

    if ((await workflowError.count()) > 0) {
      const errorText = normalizeProofText((await workflowError.textContent()) ?? "");
      if (errorText !== "") {
        throw new Error(`diagnosis workflow error before assistant turn: ${truncateProofText(errorText, 1000)}`);
      }
    }

    if (collectionFailure !== undefined) {
      const failureText = await latestEvidenceCollectionFailure(
        collectionFailure.section,
        collectionFailure.request
      );
      if (failureText !== "") {
        throw new Error(
          `evidence collection did not produce an assistant turn: ${truncateProofText(failureText, 1000)}`
        );
      }
    }

    if (Date.now() >= nextStateRefreshAt && (await refreshState.isEnabled().catch(() => false))) {
      await refreshState.click();
      nextStateRefreshAt = Date.now() + 10_000;
    }

    if (Date.now() >= nextBackendPollAt) {
      const backendState = await liveBackendRoomSnapshot();
      if (!backendState.inFlight && backendState.latestError !== undefined) {
        throw new Error(`diagnosis workflow error before assistant turn: ${backendErrorText(backendState)}`);
      }
      if (backendState.turnCount > assistantTurnsBefore && !backendState.inFlight) {
        const refreshedTurnCount = await waitForLoadedStateTurnCount(page, assistantTurnsBefore, 30_000);
        return Math.max(backendState.turnCount, refreshedTurnCount);
      }
      nextBackendPollAt = Date.now() + 15_000;
    }

    await page.waitForTimeout(1_000);
  }

  throw new Error("assistant turn count should increase after submitting a diagnosis message");
}

async function latestEvidenceCollectionFailure(section: Locator, request: LiveToolRequest): Promise<string> {
  const itemTexts = (await section.locator(".diagnosis-evidence-item").allTextContents()).map(normalizeProofText);
  for (const itemText of itemTexts) {
    if (!/\b(?:failed|skipped|unsupported)\b/.test(itemText)) {
      continue;
    }
    if (!evidencePlanItemMatchesOperationalIdentity(itemText, request)) {
      continue;
    }
    return itemText;
  }
  return "";
}

async function evidenceCollectionCoverageTexts(page: Page, collectionResultSection: Locator): Promise<string[]> {
  const texts = [
    ...(await collectionResultSection.locator(".diagnosis-evidence-item").allTextContents())
  ];
  const evidenceTimeline = page.getByLabel("Evidence timeline");
  if (await optionalVisible(evidenceTimeline)) {
    texts.push(...(await evidenceTimeline.locator("li").allTextContents()));
    texts.push(...(await evidenceTimeline.locator(".ant-list-item").allTextContents()));
    texts.push(...(await evidenceTimeline.locator(".diagnosis-evidence-timeline-entry").allTextContents()));
    texts.push(...(await evidenceTimeline.locator(".diagnosis-evidence-timeline-chip").allTextContents()));
    texts.push(...(await evidenceTimeline.locator(".diagnosis-evidence-item").allTextContents()));
  }
  texts.push(...(await backendEvidenceCollectionCoverageTexts()));
  return Array.from(new Set(texts.map(normalizeProofText).filter((text) => text !== "")));
}

async function waitForEvidenceCollectionCoverageTexts(
  page: Page,
  collectionResultSection: Locator,
  request: LiveToolRequest
): Promise<string[]> {
  const deadline = Date.now() + 30_000;
  let texts: string[] = [];
  while (Date.now() < deadline) {
    texts = await evidenceCollectionCoverageTexts(page, collectionResultSection);
    const terminalFailure = texts.find((itemText) => evidenceCollectionFailureMatches(itemText, request));
    if (terminalFailure !== undefined) {
      throw new Error(`evidence collection failed: ${truncateProofText(terminalFailure, 1000)}`);
    }
    if (texts.some((itemText) => evidenceCollectionResultMatches(itemText, request))) {
      return texts;
    }
    await page.waitForTimeout(500);
  }
  return texts;
}

function evidenceCollectionResultMatches(itemText: string, request: LiveToolRequest): boolean {
  if (!evidencePlanItemMatchesOperationalIdentity(itemText, request)) {
    return false;
  }
  const compactText = itemText.replace(/\s+/g, "");
  return (
    (/\bcollected\b/.test(itemText) || compactText.includes("collected")) &&
    (/\bok\b/.test(itemText) || compactText.includes("ok"))
  );
}

function evidenceCollectionFailureMatches(itemText: string, request: LiveToolRequest): boolean {
  if (!evidencePlanItemMatchesOperationalIdentity(itemText, request)) {
    return false;
  }
  return /\b(?:failed|skipped|unsupported)\b/.test(itemText);
}

async function observedUITurnCount(page: Page, assistantTurns: Locator): Promise<number> {
  return Math.max(await assistantTurns.count(), await latestLoadedStateTurnCount(page), await latestRoomStateTurnCount(page));
}

async function latestLoadedStateTurnCount(page: Page): Promise<number> {
  const texts = await page.getByText(/Loaded state: .*, \d+ turn\(s\)\./).allTextContents();
  let latest = 0;
  for (const text of texts) {
    const match = /Loaded state: .*, (\d+) turn\(s\)\./.exec(text);
    if (match) {
      latest = Math.max(latest, Number(match[1]));
    }
  }
  return latest;
}

async function latestRoomStateTurnCount(page: Page): Promise<number> {
  const rows = await page.locator(".ant-descriptions-item").allTextContents();
  let latest = 0;
  for (const row of rows) {
    const text = normalizeProofText(row);
    const match = /^Turns\s*:\s*([0-9]+)$/.exec(text);
    if (match) {
      latest = Math.max(latest, Number(match[1]));
    }
  }
  return latest;
}

async function waitForLoadedStateTurnCount(page: Page, previousTurnCount: number, timeoutMS: number): Promise<number> {
  const deadline = Date.now() + timeoutMS;
  const refreshState = page.getByRole("button", { name: "Refresh State" });
  while (Date.now() < deadline) {
    const loadedTurnCount = Math.max(await latestLoadedStateTurnCount(page), await latestRoomStateTurnCount(page));
    if (loadedTurnCount > previousTurnCount) {
      return loadedTurnCount;
    }
    if (await refreshState.isEnabled().catch(() => false)) {
      await refreshState.click();
    }
    await page.waitForTimeout(1_000);
  }
  return 0;
}

async function liveBackendRoomSnapshot(): Promise<LiveBackendRoomSnapshot> {
  const fallback: LiveBackendRoomSnapshot = {
    collectionResults: [],
    evidenceTimeline: [],
    inFlight: true,
    status: "unknown",
    turnCount: 0
  };
  try {
    const ticketResponse = await fetch(`${liveAPIBaseURL}/api/v1/diagnosis/ws-ticket`, {
      body: JSON.stringify({ session_id: sessionID }),
      headers: {
        authorization: liveAuthorizationHeader(liveAuth),
        "content-type": "application/json"
      },
      method: "POST"
    });
    if (!ticketResponse.ok) {
      return fallback;
    }
    const ticketBody = (await ticketResponse.json()) as { ticket?: string };
    if (!ticketBody.ticket) {
      return fallback;
    }
    return await queryLiveBackendRoomSnapshot(ticketBody.ticket);
  } catch {
    return fallback;
  }
}

async function queryLiveBackendRoomSnapshot(ticket: string): Promise<LiveBackendRoomSnapshot> {
  const fallback: LiveBackendRoomSnapshot = {
    collectionResults: [],
    evidenceTimeline: [],
    inFlight: true,
    status: "unknown",
    turnCount: 0
  };
  const wsURL = `${liveAPIBaseURL.replace(/^http/, "ws")}/ws/diagnosis?session_id=${encodeURIComponent(
    sessionID
  )}&ticket=${encodeURIComponent(ticket)}`;
  return await new Promise<LiveBackendRoomSnapshot>((resolve) => {
    const ws = new WebSocket(wsURL);
    const timeout = setTimeout(() => {
      ws.close();
      resolve(fallback);
    }, 15_000);
    ws.addEventListener("open", () => {
      ws.send(JSON.stringify({ type: "query_state" }));
    });
    ws.addEventListener("error", () => {
      clearTimeout(timeout);
      resolve(fallback);
    });
    ws.addEventListener("message", (event) => {
      const frame = JSON.parse(String(event.data)) as {
        evidence_collection_results?: LiveEvidenceCollectionResult[];
        evidence_timeline?: LiveEvidenceTimelineEntry[];
        in_flight?: boolean;
        latest_error?: LiveBackendRoomError;
        status?: string;
        turn_count?: number;
        type?: string;
      };
      if (frame.type !== "state") {
        return;
      }
      clearTimeout(timeout);
      ws.close();
      resolve({
        collectionResults: frame.evidence_collection_results ?? [],
        evidenceTimeline: frame.evidence_timeline ?? [],
        inFlight: frame.in_flight ?? true,
        latestError: frame.latest_error,
        status: frame.status ?? "unknown",
        turnCount: frame.turn_count ?? 0
      });
    });
  });
}

async function backendEvidenceCollectionCoverageTexts(): Promise<string[]> {
  const snapshot = await liveBackendRoomSnapshot();
  return allBackendEvidenceCollectionResults(snapshot).map(evidenceCollectionResultProofText);
}

function allBackendEvidenceCollectionResults(snapshot: LiveBackendRoomSnapshot): LiveEvidenceCollectionResult[] {
  return [
    ...snapshot.collectionResults,
    ...snapshot.evidenceTimeline.flatMap((entry) => entry.evidence_collection_results ?? [])
  ];
}

function collectedBackendEvidenceCollectionResults(snapshot: LiveBackendRoomSnapshot): LiveEvidenceCollectionResult[] {
  return allBackendEvidenceCollectionResults(snapshot).filter((result) => result.status === "collected");
}

function evidenceCollectionResultProofText(result: LiveEvidenceCollectionResult): string {
  const request = result.request ?? {};
  const parts = [
    result.tool ?? request.tool ?? "",
    result.status ?? "",
    result.reason_code ?? ""
  ];
  const templateID = result.template_id ?? request.template_id;
  const profileID = result.alert_source_profile_id ?? request.alert_source_profile_id;
  const query = result.query ?? request.query;
  const limit = result.limit ?? request.limit;
  const windowSeconds = result.window_seconds ?? request.window_seconds;
  const stepSeconds = result.step_seconds ?? request.step_seconds;
  if (templateID !== undefined) {
    parts.push(`template #${templateID}`);
  }
  if (profileID !== undefined) {
    parts.push(`profile #${profileID}`);
  }
  if (limit !== undefined) {
    parts.push(`limit: ${limit}`);
  }
  if (windowSeconds !== undefined) {
    parts.push(`window: ${windowSeconds}s`);
  }
  if (stepSeconds !== undefined) {
    parts.push(`step: ${stepSeconds}s`);
  }
  if (query !== undefined) {
    parts.push(query);
  }
  return parts.filter((part) => part !== "").join(" ");
}

async function maybeSubmitSupplementalEvidence(
  page: Page,
  assistantTurns: Locator,
  assistantTurnsAfterInitialTurn: number,
  diagnosisConfidence: Locator
): Promise<Record<string, boolean | number | string>> {
  const proof: Record<string, boolean | number | string> = {
    supplemental_evidence_requested: submitSupplementalEvidence,
    supplemental_evidence_required: requireSupplementalEvidence
  };
  if (!submitSupplementalEvidence) {
    return proof;
  }

  const followUpButtons = page.getByRole("button", { name: /^Use follow-up for / });
  const followUpButtonCount = await followUpButtons.count();
  proof.supplemental_follow_up_count = followUpButtonCount;
  proof.supplemental_follow_up_available = followUpButtonCount > 0;
  if (followUpButtonCount === 0) {
    proof.supplemental_block_reason = "no_supplemental_follow_up_available";
    if (requireSupplementalEvidence) {
      throw new Error("supplemental evidence is required but no Use follow-up action is available");
    }
    return proof;
  }

  const firstFollowUpButton = followUpButtons.first();
  const requestAriaLabel = ((await firstFollowUpButton.getAttribute("aria-label")) ?? "").trim();
  const requestLabel = requestAriaLabel.replace(/^Use follow-up for\s+/, "").trim();
  proof.supplemental_request_label = truncateProofText(requestLabel || requestAriaLabel, 256);
  proof.supplemental_assistant_turns_before = assistantTurnsAfterInitialTurn;
  proof.supplemental_confidence_before = confidenceFromAriaValue(
    ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
  );
  await expect(firstFollowUpButton).toBeEnabled();
  await firstFollowUpButton.click();

  const entryPanel = page.locator('[aria-label="Supplemental evidence entry"]');
  await expect(entryPanel).toBeVisible();
  const requestDetail = normalizeProofText(
    (await entryPanel.locator(".diagnosis-supplemental-entry-header .ant-typography").last().textContent()) ?? ""
  );
  const requestPriority = normalizeProofText(
    (await entryPanel.locator(".diagnosis-supplemental-entry-header .ant-tag").first().textContent()) ?? ""
  );
  const supplementalEvidenceText = supplementalEvidenceForRequest({
    detail: requestDetail,
    label: requestLabel || requestAriaLabel,
    priority: requestPriority
  });
  await entryPanel.getByLabel("Evidence").fill(supplementalEvidenceText);
  const historyItems = page.locator('[aria-label="Supplemental evidence history"] .diagnosis-evidence-item');
  const historyItemsBefore = await historyItems.count();
  proof.supplemental_history_count_before = historyItemsBefore;

  await entryPanel.getByRole("button", { name: "Submit supplemental evidence" }).click();
  await expect(page.getByText(/Submitted supplemental evidence for /).last()).toBeVisible();
  proof.supplemental_evidence_submitted = true;
  proof.supplemental_evidence_length = supplementalEvidenceText.length;
  proof.supplemental_evidence_sha256 = sha256Hex(supplementalEvidenceText);

    const observedTurnsAfterSupplemental = await waitForAssistantTurnOrWorkflowError(
      page,
      assistantTurns,
      assistantTurnsAfterInitialTurn,
      liveTurnTimeoutMS
    );
    const assistantTurnsAfterSupplemental = Math.max(await assistantTurns.count(), observedTurnsAfterSupplemental);
  proof.supplemental_assistant_turns_after = assistantTurnsAfterSupplemental;
  proof.supplemental_assistant_turn_delta = assistantTurnsAfterSupplemental - assistantTurnsAfterInitialTurn;
  proof.supplemental_confidence_after = confidenceFromAriaValue(
    ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
  );

  await page.getByRole("button", { name: "Refresh State" }).click();
  const refreshedCompletionEvidence = await waitForTurnCompletionEvidence(page, assistantTurnsAfterSupplemental);
  const refreshedTurnCount = completionEvidenceTurnCount(refreshedCompletionEvidence);
  const assistantTurnsAfterRefresh = await assistantTurns.count();
  const latestAssistantTurnsAfterSupplemental = Math.max(
    assistantTurnsAfterSupplemental,
    refreshedTurnCount ?? 0,
    assistantTurnsAfterRefresh
  );
  proof.supplemental_assistant_turns_after = latestAssistantTurnsAfterSupplemental;
  proof.supplemental_assistant_turn_delta = latestAssistantTurnsAfterSupplemental - assistantTurnsAfterInitialTurn;
  proof.supplemental_completion_evidence_after = truncateProofText(refreshedCompletionEvidence, 128);
  await expect.poll(async () => historyItems.count(), { timeout: 30_000 }).toBeGreaterThan(historyItemsBefore);
  proof.supplemental_history_visible = await page
    .locator('[aria-label="Supplemental evidence history"]')
    .isVisible();
  proof.supplemental_history_count_after = await historyItems.count();
  const reviewQueue = page.getByLabel("Diagnosis review queue");
  await expect(reviewQueue).toBeVisible({ timeout: 30_000 });
  proof.supplemental_review_queue_visible = await reviewQueue.isVisible();
  proof.supplemental_review_queue_item_count = await reviewQueue.locator(".ant-list-item").count();
  const confirmButton = page.getByRole("button", { name: "Confirm Conclusion" });
  const confirmAvailableAfterSupplemental = await confirmButton.isEnabled();
  proof.supplemental_confirm_conclusion_available_after = confirmAvailableAfterSupplemental;
  if (!confirmAvailableAfterSupplemental) {
    await expect(confirmButton).toBeDisabled();
    const confirmBlockReason = page.locator(".diagnosis-confirm-block-reason");
    await expect(confirmBlockReason).toBeVisible({ timeout: 30_000 });
    proof.supplemental_confirm_block_reason_after = truncateProofText(
      normalizeProofText((await confirmBlockReason.textContent()) ?? ""),
      128
    );
  }

  return proof;
}

async function maybeCollectPlannedEvidence(
  page: Page,
  assistantTurns: Locator,
  assistantTurnsAfterInitialTurn: number,
  diagnosisConfidence: Locator,
  collectionResultSection: Locator,
  evidenceCollectionResultCountBefore: number
): Promise<Record<string, boolean | number | string>> {
  const proof: Record<string, boolean | number | string> = {
    planned_evidence_collection_requested: collectPlannedEvidence
  };
  if (!collectPlannedEvidence) {
    return proof;
  }

  const reviewQueue = page.getByLabel("Diagnosis review queue");
  await expect(reviewQueue).toBeVisible({ timeout: 30_000 });
  const planButtons = reviewQueue.getByRole("button", { name: /^Use collection plan for / });
  const planButtonCount = await planButtons.count();
  proof.planned_evidence_collection_action_count = planButtonCount;
  proof.planned_evidence_collection_available = planButtonCount > 0;
  if (planButtonCount === 0) {
    let collectionSummary = "";
    let collectionResultCount = 0;
    let finalConclusionVisible = false;
    let readyForConfirmationVisible = false;
    let backendTurnCount = assistantTurnsAfterInitialTurn;
    let backendCollectionResultCount = 0;
    let backendCollectedResultCount = 0;
    let collectionSatisfied = false;
    let alreadyFinal = false;

    await expect
      .poll(
        async () => {
          collectionSummary = await optionalVisibleText(
            collectionResultSection.locator('[aria-label="Evidence collection summary"]')
          );
          collectionResultCount = await collectionResultSection.locator(".diagnosis-evidence-item").count();
          finalConclusionVisible = await optionalVisible(page.locator(".diagnosis-conclusion"));
          readyForConfirmationVisible = await optionalVisible(
            page.getByText("Ready for confirmation", { exact: true })
          );

          const backendSnapshot = await liveBackendRoomSnapshot();
          const backendCollectionResults = allBackendEvidenceCollectionResults(backendSnapshot);
          backendTurnCount = backendSnapshot.turnCount;
          backendCollectionResultCount = backendCollectionResults.length;
          backendCollectedResultCount = collectedBackendEvidenceCollectionResults(backendSnapshot).length;

          const uiCollected =
            collectionResultCount > 0 &&
            !/^0\/[1-9][0-9]* collected/.test(collectionSummary);
          collectionSatisfied = uiCollected || backendCollectedResultCount > 0;
          alreadyFinal = finalConclusionVisible || readyForConfirmationVisible;
          return collectionSatisfied || alreadyFinal;
        },
        {
          message: "planned evidence should already be collected or ready for final confirmation",
          timeout: 30_000
        }
      )
      .toBeTruthy();

    proof.planned_evidence_collection_result_count_before = evidenceCollectionResultCountBefore;
    proof.planned_evidence_collection_result_count_after = Math.max(
      collectionResultCount,
      backendCollectionResultCount
    );
    proof.planned_evidence_backend_collection_result_count = backendCollectionResultCount;
    proof.planned_evidence_backend_collected_result_count = backendCollectedResultCount;
    proof.planned_evidence_collection_summary_visible = collectionSummary !== "";
    proof.planned_evidence_final_conclusion_visible = finalConclusionVisible;
    proof.planned_evidence_ready_for_confirmation_visible = readyForConfirmationVisible;
    if (collectionSummary !== "") {
      proof.planned_evidence_collection_summary_text = truncateProofText(collectionSummary, 256);
    }
    if (collectionSatisfied) {
      proof.planned_evidence_collection_mode = "auto_collected";
      proof.planned_evidence_collection_satisfied = true;
      proof.planned_evidence_assistant_turns_after = Math.max(
        assistantTurnsAfterInitialTurn,
        backendTurnCount,
        await assistantTurns.count()
      );
      proof.planned_evidence_confidence_after = confidenceFromAriaValue(
        ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
      );
      return proof;
    }
    if (alreadyFinal) {
      proof.planned_evidence_collection_mode = "already_final";
      proof.planned_evidence_collection_satisfied = true;
      proof.planned_evidence_assistant_turns_after = Math.max(
        assistantTurnsAfterInitialTurn,
        backendTurnCount,
        await assistantTurns.count()
      );
      proof.planned_evidence_confidence_after = confidenceFromAriaValue(
        ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
      );
      return proof;
    }
    throw new Error("planned evidence collection was requested but no Use collection plan action is available");
  }

  const firstPlanButton = planButtons.first();
  const actionLabel = ((await firstPlanButton.getAttribute("aria-label")) ?? "").trim();
  const tool = actionLabel.replace(/^Use collection plan for\s+/, "").trim();
  if (tool !== "active_alerts" && tool !== "metric_query" && tool !== "metric_range_query") {
    throw new Error("planned evidence collection action has an unsupported tool label");
  }

  proof.planned_evidence_collection_tool = tool;
  proof.planned_evidence_collection_mode = "manual_update";
  proof.planned_evidence_collection_satisfied = true;
  proof.planned_evidence_assistant_turns_before = assistantTurnsAfterInitialTurn;
  proof.planned_evidence_confidence_before = confidenceFromAriaValue(
    ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
  );
  proof.planned_evidence_collection_result_count_before = evidenceCollectionResultCountBefore;

  await expect(firstPlanButton).toBeEnabled();
  await firstPlanButton.click();
  await expect(page.getByText(`Collecting planned evidence for ${tool}.`, { exact: true }).last()).toBeVisible({
    timeout: 30_000
  });
  proof.planned_evidence_collection_triggered = true;

  const observedTurnsAfterCollection = await waitForAssistantTurnOrWorkflowError(
    page,
    assistantTurns,
    assistantTurnsAfterInitialTurn,
    liveTurnTimeoutMS
  );
  const assistantTurnsAfterCollection = Math.max(await assistantTurns.count(), observedTurnsAfterCollection);
  proof.planned_evidence_assistant_turns_after = assistantTurnsAfterCollection;
  proof.planned_evidence_assistant_turn_delta = assistantTurnsAfterCollection - assistantTurnsAfterInitialTurn;
  proof.planned_evidence_confidence_after = confidenceFromAriaValue(
    ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
  );
  proof.planned_evidence_collection_result_count_after = await collectionResultSection
    .locator(".diagnosis-evidence-item")
    .count();

  await expect(page.getByText(new RegExp(`Loaded state: .*, ${assistantTurnsAfterCollection} turn\\(s\\)\\.`)).last())
    .toBeVisible({ timeout: 30_000 });
  const evidenceTimeline = page.getByLabel("Evidence timeline");
  await expect(evidenceTimeline).toContainText("manual_evidence_collection", { timeout: 30_000 });
  proof.planned_evidence_timeline_visible = await evidenceTimeline.isVisible();

  return proof;
}

async function maybeCollectSeededOperatorEvidence(
  page: Page,
  assistantTurns: Locator,
  assistantTurnsAfterInitialTurn: number,
  diagnosisConfidence: Locator,
  collectionResultSection: Locator,
  evidenceCollectionResultCountBefore: number,
  currentSeedProof: Record<string, number | string>
): Promise<Record<string, boolean | number | string>> {
  const seedMatchedCount = Number(currentSeedProof.tool_request_seed_matched_count ?? 0);
  const proof: Record<string, boolean | number | string> = {
    operator_seed_collection_requested:
      seededToolRequests.length > 0 && seedMatchedCount !== seededToolRequests.length
  };
  if (!proof.operator_seed_collection_requested) {
    return proof;
  }

  const operatorPanel = page.getByLabel("Operator evidence collection");
  await expect(operatorPanel).toBeVisible({ timeout: 30_000 });
  proof.operator_seed_collection_count = seededToolRequests.length;
  proof.operator_seed_collection_result_count_before = evidenceCollectionResultCountBefore;
  proof.operator_seed_collection_assistant_turns_before = assistantTurnsAfterInitialTurn;
  proof.operator_seed_collection_confidence_before = confidenceFromAriaValue(
    ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
  );

  let currentAssistantTurns = assistantTurnsAfterInitialTurn;
  for (const request of seededToolRequests) {
    await fillOperatorEvidenceRequest(page, operatorPanel, request);
    await operatorPanel.getByRole("button", { name: "Collect operator evidence" }).click();
    await expect(page.getByText(`Collecting operator evidence for ${request.tool}.`, { exact: true }).last())
      .toBeVisible({ timeout: 30_000 });
    proof.operator_seed_collection_triggered = true;

    currentAssistantTurns = await waitForAssistantTurnOrWorkflowError(
      page,
      assistantTurns,
      currentAssistantTurns,
      liveTurnTimeoutMS,
      { request, section: collectionResultSection }
    );
    currentAssistantTurns = Math.max(await assistantTurns.count(), currentAssistantTurns);
    await expect(page.getByText(new RegExp(`Loaded state: .*, ${currentAssistantTurns} turn\\(s\\)\\.`)).last())
      .toBeVisible({ timeout: 30_000 });
  }

  const resultTexts = await evidenceCollectionCoverageTexts(page, collectionResultSection);
  const missing = seededToolRequests
    .filter((request) => !resultTexts.some((itemText) => evidenceCollectionResultMatches(itemText, request)))
    .map(evidencePlanSeedLabel);
  proof.tool_request_seed_matched_count = seededToolRequests.length - missing.length;
  proof.tool_request_seed_missing = missing.join("; ");
  proof.operator_seed_collection_assistant_turns_after = currentAssistantTurns;
  proof.operator_seed_collection_assistant_turn_delta = currentAssistantTurns - assistantTurnsAfterInitialTurn;
  proof.operator_seed_collection_confidence_after = confidenceFromAriaValue(
    ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
  );
  proof.operator_seed_collection_result_count_after = await collectionResultSection
    .locator(".diagnosis-evidence-item")
    .count();
  const collectionSummary = collectionResultSection.locator('[aria-label="Evidence collection summary"]');
  if (await collectionSummary.isVisible()) {
    proof.operator_seed_collection_summary_text = truncateProofText(
      normalizeProofText((await collectionSummary.textContent()) ?? ""),
      256
    );
  }

  return proof;
}

async function maybeCollectStagedOperatorEvidence(
  page: Page,
  assistantTurns: Locator,
  assistantTurnsAfterInitialTurn: number,
  diagnosisConfidence: Locator,
  collectionResultSection: Locator,
  evidenceCollectionResultCountBefore: number
): Promise<Record<string, boolean | number | string>> {
  const proof: Record<string, boolean | number | string> = {
    operator_staged_collection_requested: stagedOperatorToolRequests.length > 0,
    operator_staged_collection_count: stagedOperatorToolRequests.length
  };
  if (stagedOperatorToolRequests.length === 0) {
    return proof;
  }

  const operatorPanel = page.getByLabel("Operator evidence collection");
  await expect(operatorPanel).toBeVisible({ timeout: 30_000 });
  proof.operator_staged_collection_result_count_before = evidenceCollectionResultCountBefore;
  proof.operator_staged_collection_assistant_turns_before = assistantTurnsAfterInitialTurn;
  proof.operator_staged_collection_confidence_before = confidenceFromAriaValue(
    ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
  );

  let currentAssistantTurns = assistantTurnsAfterInitialTurn;
  let matchedCount = 0;
  const missing: string[] = [];
  const collectionModes: string[] = [];
  for (const request of stagedOperatorToolRequests) {
    const planButton = await matchingReviewQueuePlanButton(page, request);
    if (planButton !== null) {
      await expect(planButton).toBeEnabled();
      await planButton.click();
      await expect(page.getByText(`Collecting planned evidence for ${request.tool}.`, { exact: true }).last())
        .toBeVisible({ timeout: 30_000 });
      collectionModes.push("review_queue_plan");
    } else {
      await fillOperatorEvidenceRequest(page, operatorPanel, request);
      const collectButton = operatorPanel.getByRole("button", { name: "Collect operator evidence" });
      await expect(collectButton).toBeEnabled({ timeout: 30_000 });
      await collectButton.click();
      await expect(page.getByText(`Collecting operator evidence for ${request.tool}.`, { exact: true }).last())
        .toBeVisible({ timeout: 30_000 });
      collectionModes.push("operator_form");
    }
    proof.operator_staged_collection_triggered = true;

    currentAssistantTurns = await waitForAssistantTurnOrWorkflowError(
      page,
      assistantTurns,
      currentAssistantTurns,
      liveTurnTimeoutMS,
      { request, section: collectionResultSection }
    );
    currentAssistantTurns = Math.max(await assistantTurns.count(), currentAssistantTurns);
    await expect(page.getByText(new RegExp(`Loaded state: .*, ${currentAssistantTurns} turn\\(s\\)\\.`)).last())
      .toBeVisible({ timeout: 30_000 });
    const resultTexts = await waitForEvidenceCollectionCoverageTexts(page, collectionResultSection, request);
    if (resultTexts.some((itemText) => evidenceCollectionResultMatches(itemText, request))) {
      matchedCount += 1;
    } else {
      missing.push(evidencePlanSeedLabel(request));
    }
  }

  proof.operator_staged_collection_matched_count = matchedCount;
  proof.operator_staged_collection_missing = missing.join("; ");
  proof.operator_staged_collection_modes = collectionModes.join(",");
  proof.operator_staged_collection_assistant_turns_after = currentAssistantTurns;
  proof.operator_staged_collection_assistant_turn_delta = currentAssistantTurns - assistantTurnsAfterInitialTurn;
  proof.operator_staged_collection_confidence_after = confidenceFromAriaValue(
    ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim()
  );
  proof.operator_staged_collection_result_count_after = await collectionResultSection
    .locator(".diagnosis-evidence-item")
    .count();
  const collectionSummary = collectionResultSection.locator('[aria-label="Evidence collection summary"]');
  if (await collectionSummary.isVisible()) {
    proof.operator_staged_collection_summary_text = truncateProofText(
      normalizeProofText((await collectionSummary.textContent()) ?? ""),
      256
    );
  }

  if (missing.length > 0) {
    throw new Error(`staged operator evidence requests were not covered: ${proof.operator_staged_collection_missing}`);
  }
  return proof;
}

async function fillOperatorEvidenceRequest(page: Page, operatorPanel: Locator, request: LiveToolRequest): Promise<void> {
  const selectedTemplate = await maybeSelectOperatorEvidenceTemplate(page, operatorPanel, request);
  if (!selectedTemplate) {
    await operatorPanel.locator(".diagnosis-operator-evidence-tool-select .ant-select-selector").click();
    await clickVisibleSelectOption(page, request.tool);
  }
  await operatorPanel.getByLabel("Reason").fill(request.reason);
  await fillOperatorEvidenceField(operatorPanel, "Template ID", request.template_id);
  await fillOperatorEvidenceField(operatorPanel, "Alert source profile", request.alert_source_profile_id);
  await fillOperatorEvidenceQueryField(operatorPanel, request, selectedTemplate);
  await fillOperatorEvidenceField(operatorPanel, "Window seconds", request.window_seconds);
  await fillOperatorEvidenceField(operatorPanel, "Step seconds", request.step_seconds);
  await fillOperatorEvidenceField(operatorPanel, "Limit", request.limit);
}

async function matchingReviewQueuePlanButton(page: Page, request: LiveToolRequest): Promise<Locator | null> {
  const reviewQueue = page.getByLabel("Diagnosis review queue");
  if (!(await optionalVisible(reviewQueue))) {
    return null;
  }
  const items = reviewQueue.locator(".ant-list-item");
  const itemCount = await items.count();
  for (let index = 0; index < itemCount; index += 1) {
    const item = items.nth(index);
    const itemText = normalizeProofText((await item.textContent()) ?? "");
    if (!evidencePlanItemMatchesSeedIdentity(itemText, request)) {
      continue;
    }
    const button = item.getByRole("button", { name: /^Use collection plan for / }).first();
    if ((await button.count()) === 0) {
      continue;
    }
    if (await button.isEnabled().catch(() => false)) {
      return button;
    }
  }
  return null;
}

async function maybeSelectOperatorEvidenceTemplate(
  page: Page,
  operatorPanel: Locator,
  request: LiveToolRequest
): Promise<boolean> {
  if (request.template_id === undefined) {
    return false;
  }
  const templateIDField = operatorPanel.getByLabel("Template ID");
  if ((await templateIDField.count()) > 0 && (await optionalVisible(templateIDField))) {
    const currentTemplateID = await templateIDField.inputValue().catch(() => "");
    if (currentTemplateID === String(request.template_id)) {
      return true;
    }
  }
  const templateSelect = operatorPanel.getByRole("combobox", { name: "Operator evidence template" });
  if ((await templateSelect.count()) === 0) {
    return false;
  }
  const templateSelectRoot = operatorPanel.locator('.ant-select[aria-label="Operator evidence template"]').first();
  const templateSelectTrigger = templateSelectRoot.locator(".ant-select-selector").first();
  if ((await templateSelectTrigger.count()) > 0) {
    await templateSelectTrigger.click({ force: true });
  } else {
    await templateSelect.focus();
    await page.keyboard.press("ArrowDown");
  }
  const visibleDropdown = page.locator(".ant-select-dropdown").filter({ visible: true }).last();
  const optionPattern = new RegExp(`^\\s*#${request.template_id}\\b`);
  const option = visibleDropdown.locator(".ant-select-item-option-content", { hasText: optionPattern }).first();
  if ((await option.count()) === 0) {
    await page.keyboard.press("Escape");
    return false;
  }
  await expect(option).toBeVisible({ timeout: 30_000 });
  await option.click();
  await expect(templateIDField).toHaveValue(String(request.template_id), {
    timeout: 30_000
  });
  return true;
}

async function clickVisibleSelectOption(page: Page, label: string): Promise<void> {
  const exactLabel = new RegExp(`^\\s*${escapeRegExp(label)}\\s*$`);
  const visibleDropdown = page.locator(".ant-select-dropdown").filter({ visible: true }).last();
  const antOption = visibleDropdown.locator(".ant-select-item-option-content", { hasText: exactLabel }).first();
  if ((await antOption.count()) > 0) {
    await antOption.click();
    return;
  }
  const roleOption = page.getByRole("option", { name: label }).filter({ visible: true }).first();
  if ((await roleOption.count()) > 0) {
    await roleOption.click();
    return;
  }
  await page.getByText(exactLabel).filter({ visible: true }).first().click();
}

async function fillOperatorEvidenceField(
  operatorPanel: Locator,
  label: string,
  value: number | string | undefined
): Promise<void> {
  if (value === undefined) {
    return;
  }
  const field = operatorPanel.getByLabel(label);
  if ((await field.count()) === 0) {
    return;
  }
  if (!(await optionalVisible(field))) {
    return;
  }
  await expect(field).toBeEnabled({ timeout: 30_000 });
  await field.fill(String(value));
}

async function fillOperatorEvidenceQueryField(
  operatorPanel: Locator,
  request: LiveToolRequest,
  selectedTemplate: boolean
): Promise<void> {
  if (request.query === undefined) {
    return;
  }
  const field = operatorPanel.getByLabel("Query");
  if ((await field.count()) === 0) {
    return;
  }
  if (!(await optionalVisible(field))) {
    return;
  }
  if (selectedTemplate) {
    const currentValue = (await field.inputValue().catch(() => "")).trim();
    const requestedValue = request.query.trim();
    if (currentValue !== "" && currentValue !== requestedValue) {
      throw new Error(
        `operator evidence request query does not match selected template #${request.template_id}; omit query or use the exact template query`
      );
    }
  }
  await expect(field).toBeEnabled({ timeout: 30_000 });
  await field.fill(request.query);
}

function completedTurnLog(page: Page, turnCount: number): Locator {
  return page.getByText(`Turn ${turnCount} completed.`, { exact: true }).last();
}

function loadedStateTurnLog(page: Page, turnCount: number): Locator {
  return page.getByText(new RegExp(`Loaded state: .*, ${turnCount} turn\\(s\\)\\.`)).last();
}

async function waitForTurnCompletionEvidence(page: Page, turnCount: number): Promise<string> {
  const deadline = Date.now() + 30_000;
  const completedLog = completedTurnLog(page, turnCount);
  const stateLog = loadedStateTurnLog(page, turnCount);
  const completionLogs = page.getByText(/^(?:Turn \d+ completed\.|Loaded state: .*, \d+ turn\(s\)\.)$/);
  const refreshState = page.getByRole("button", { name: "Refresh State" });
  let nextStateRefreshAt = Date.now();
  let nextBackendPollAt = Date.now() + 5_000;
  while (Date.now() < deadline) {
    for (const locator of [completedLog, stateLog]) {
      if (await locator.isVisible().catch(() => false)) {
        const text = ((await locator.textContent()) ?? "").trim();
        if (text !== "") {
          return text;
        }
      }
    }
    const completionLogCount = await completionLogs.count();
    for (let index = completionLogCount - 1; index >= 0; index -= 1) {
      const locator = completionLogs.nth(index);
      if (!(await locator.isVisible().catch(() => false))) {
        continue;
      }
      const text = ((await locator.textContent()) ?? "").trim();
      const completedTurnCount = completionEvidenceTurnCount(text);
      if (completedTurnCount !== undefined && completedTurnCount >= turnCount) {
        return text;
      }
    }
    if (Date.now() >= nextStateRefreshAt && (await refreshState.isEnabled().catch(() => false))) {
      await refreshState.click();
      nextStateRefreshAt = Date.now() + 2_000;
    }
    const roomStateTurnCount = await latestRoomStateTurnCount(page);
    if (roomStateTurnCount >= turnCount) {
      return `Room state: ${roomStateTurnCount} turn(s).`;
    }
    if (Date.now() >= nextBackendPollAt) {
      const backendState = await liveBackendRoomSnapshot();
      if (!backendState.inFlight && backendState.latestError !== undefined) {
        throw new Error(`diagnosis workflow error before turn completion: ${backendErrorText(backendState)}`);
      }
      if (backendState.turnCount >= turnCount && !backendState.inFlight) {
        return `Backend state: ${backendState.status}, ${backendState.turnCount} turn(s).`;
      }
      nextBackendPollAt = Date.now() + 5_000;
    }
    await page.waitForTimeout(500);
  }
  throw new Error(`turn ${turnCount} or newer completion evidence should be visible`);
}

function backendErrorText(snapshot: LiveBackendRoomSnapshot): string {
  const code = snapshot.latestError?.code?.trim() || "workflow_error";
  const message = snapshot.latestError?.message?.trim() || "diagnosis workflow failed";
  return truncateProofText(`${code}: ${message}`, 1000);
}

function completionEvidenceTurnCount(text: string): number | undefined {
  const completedMatch = /^Turn ([1-9][0-9]*) completed\.$/.exec(text);
  if (completedMatch) {
    return Number(completedMatch[1]);
  }
  const loadedMatch = /^Loaded state: .*, ([1-9][0-9]*) turn\(s\)\.$/.exec(text);
  if (loadedMatch) {
    return Number(loadedMatch[1]);
  }
  return undefined;
}

function supplementalEvidenceForRequest(request: { detail: string; label: string; priority: string }): string {
  if (supplementalEvidenceTextOverride) {
    return supplementalEvidenceTextOverride;
  }
  return supplementalEvidenceTemplate
    .replaceAll("{label}", request.label || "requested evidence")
    .replaceAll("{detail}", request.detail || "not provided")
    .replaceAll("{priority}", request.priority || "unknown");
}

async function writeLiveBrowserProof(proof: Record<string, boolean | number | string>): Promise<void> {
  const path = process.env.OPENCLARION_LIVE_BROWSER_PROOF_PATH?.trim();
  if (!path) {
    return;
  }
  await writeFile(path, `${JSON.stringify(proof, null, 2)}\n`, "utf8");
}

function sha256Hex(value: string): string {
  return createHash("sha256").update(value, "utf8").digest("hex");
}

function confidenceFromAriaValue(value: string): string {
  const [confidence] = value.split(/\s+/, 1);
  return confidence ?? "";
}

function normalizeProofText(value: string): string {
  return value.replace(/\s+/g, " ").trim();
}

async function optionalVisible(locator: Locator): Promise<boolean> {
  if ((await locator.count()) === 0) {
    return false;
  }
  return locator.first().isVisible().catch(() => false);
}

async function optionalVisibleText(locator: Locator): Promise<string> {
  if (!(await optionalVisible(locator))) {
    return "";
  }
  return normalizeProofText((await locator.first().textContent().catch(() => "")) ?? "");
}

function truncateProofText(value: string, maxLength: number): string {
  if (value.length <= maxLength) {
    return value;
  }
  return `${value.slice(0, maxLength)}...`;
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
