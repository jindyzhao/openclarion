import { expect, test } from "@playwright/test";
import { createHash } from "node:crypto";
import { writeFile } from "node:fs/promises";

const sessionID = requiredEnv("OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID");
const bearerToken = requiredEnv("OPENCLARION_LIVE_BEARER_TOKEN");
const message =
  process.env.OPENCLARION_LIVE_DIAGNOSIS_MESSAGE?.trim() ||
  `Live browser acceptance ${new Date().toISOString()}`;
const liveTurnTimeoutMS = positiveIntegerEnv("OPENCLARION_LIVE_TURN_TIMEOUT_MS", 120_000);

test("diagnosis room completes one turn against a live backend", async ({ page }) => {
  await page.goto("/diagnosis-room");

  await expect(page.getByRole("heading", { name: "Diagnosis Room" })).toBeVisible();
  await page.getByLabel("Session ID").fill(sessionID);
  await page.getByLabel("Bearer token").fill(bearerToken);
  await page.getByRole("button", { name: /Connect/ }).click();

  await expect(page.getByRole("status", { name: "Connection status" })).toHaveText("connected", {
    timeout: 30_000
  });
  const loadedStateLog = page.getByText(/Loaded state:/);
  await expect(loadedStateLog).toBeVisible({ timeout: 30_000 });

  const assistantTurns = page.locator(".diagnosis-turn-assistant");
  const assistantTurnsBefore = await assistantTurns.count();
  const transcriptTurnsBefore = await page.locator(".diagnosis-turn").count();

  await page.getByLabel("Message").fill(message);
  await page.getByRole("button", { name: "Send" }).click();

  const submittedMessage = page.getByText(message, { exact: true });
  await expect(submittedMessage).toBeVisible();
  await expect
    .poll(async () => assistantTurns.count(), {
      message: "assistant turn count should increase after submitting a diagnosis message",
      timeout: liveTurnTimeoutMS
    })
    .toBeGreaterThan(assistantTurnsBefore);
  const assistantTurnsAfter = await assistantTurns.count();
  const assistantTurnDelta = assistantTurnsAfter - assistantTurnsBefore;
  const completedTurnLog = page.getByText(/Turn \d+ completed\./).first();
  await expect(completedTurnLog).toBeVisible();
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
  const confidenceAriaValue = ((await diagnosisConfidence.getAttribute("aria-valuetext")) ?? "").trim();

  await writeLiveBrowserProof({
    state_loaded: await loadedStateLog.isVisible(),
    turn_result_observed: await completedTurnLog.isVisible(),
    assistant_turns_before: assistantTurnsBefore,
    assistant_turns_after: assistantTurnsAfter,
    assistant_turn_delta: assistantTurnDelta,
    transcript_messages_before: transcriptTurnsBefore,
    transcript_messages_after: await page.locator(".diagnosis-turn").count(),
    connection_status_after_turn: ((await connectionStatus.textContent()) ?? "").trim(),
    submitted_message_visible: await submittedMessage.isVisible(),
    submitted_message_length: message.length,
    submitted_message_sha256: sha256Hex(message),
    completed_turn_text: ((await completedTurnLog.textContent()) ?? "").trim(),
    consultation_insight_visible: await consultationInsight.isVisible(),
    consultation_progress_visible: await consultationProgress.isVisible(),
    evidence_readiness_visible: await evidenceReadiness.isVisible(),
    confidence: confidenceFromAriaValue(confidenceAriaValue),
    confidence_aria_value: confidenceAriaValue,
    evidence_readiness_text: normalizeProofText((await evidenceReadiness.textContent()) ?? "")
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
