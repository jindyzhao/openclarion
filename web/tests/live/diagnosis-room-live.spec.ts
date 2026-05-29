import { expect, test } from "@playwright/test";
import { createHash } from "node:crypto";
import { writeFile } from "node:fs/promises";

const apiBaseURL = requiredEnv("OPENCLARION_LIVE_API_BASE_URL");
const sessionID = requiredEnv("OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID");
const bearerToken = requiredEnv("OPENCLARION_LIVE_BEARER_TOKEN");
const message =
  process.env.OPENCLARION_LIVE_DIAGNOSIS_MESSAGE?.trim() ||
  `Live browser acceptance ${new Date().toISOString()}`;

test("diagnosis room completes one turn against a live backend", async ({ page }) => {
  await page.goto("/diagnosis-room");

  await expect(page.getByRole("heading", { name: "Diagnosis Room" })).toBeVisible();
  await page.getByLabel("API base URL").fill(apiBaseURL);
  await page.getByLabel("Session ID").fill(sessionID);
  await page.getByLabel("Bearer token").fill(bearerToken);
  await page.getByRole("button", { exact: true, name: "Connect" }).click();

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
  await expect(assistantTurns).toHaveCount(assistantTurnsBefore + 1, { timeout: 120_000 });
  const completedTurnLog = page.locator(".diagnosis-log li").filter({ hasText: /Turn \d+ completed\./ }).first();
  await expect(completedTurnLog).toBeVisible();
  const connectionStatus = page.getByRole("status", { name: "Connection status" });
  await expect(connectionStatus).toHaveText("connected");

  await writeLiveBrowserProof({
    state_loaded: await loadedStateLog.isVisible(),
    turn_result_observed: await completedTurnLog.isVisible(),
    assistant_turns_before: assistantTurnsBefore,
    assistant_turns_after: await assistantTurns.count(),
    transcript_messages_before: transcriptTurnsBefore,
    transcript_messages_after: await page.locator(".diagnosis-turn").count(),
    connection_status_after_turn: ((await connectionStatus.textContent()) ?? "").trim(),
    submitted_message_visible: await submittedMessage.isVisible(),
    submitted_message_length: message.length,
    submitted_message_sha256: sha256Hex(message),
    completed_turn_text: ((await completedTurnLog.textContent()) ?? "").trim()
  });
});

function requiredEnv(name: string): string {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required for live diagnosis-room browser smoke`);
  }
  return value;
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
