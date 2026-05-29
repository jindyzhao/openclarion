import { expect, test } from "@playwright/test";

test("dashboard route renders mocked operational summary", async ({ page }) => {
  await page.goto("/dashboard");

  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
  await expect(page.getByLabel("Dashboard metrics")).toContainText("Firing alerts");
  await expect(page.getByText("92%")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Report Delivery" })).toBeVisible();
});

test("report routes render list, detail, and evidence traceability", async ({ page }) => {
  await page.goto("/reports");

  await expect(page.getByRole("heading", { name: "Reports" })).toBeVisible();
  const reportLink = page.getByRole("link", { name: "Checkout latency incident" });
  await expect(reportLink).toBeVisible();

  await reportLink.click();
  await expect(page).toHaveURL(/\/reports\/101$/);
  await expect(page.getByRole("heading", { name: "Checkout latency incident" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Evidence Traceability" })).toBeVisible();
  await expect(page.getByText("Evidence snapshot #9001")).toBeVisible();
});

test("diagnosis room route connects, queries state, and submits a turn", async ({ page }) => {
  const apiPort = process.env.OPENCLARION_PLAYWRIGHT_API_PORT ?? "38280";

  await page.goto("/diagnosis-room");

  await expect(page.getByRole("heading", { name: "Diagnosis Room" })).toBeVisible();
  await page.getByLabel("API base URL").fill(`http://127.0.0.1:${apiPort}`);
  await page.getByLabel("Session ID").fill("diagnosis-session-42");
  await page.getByLabel("Bearer token").fill("test-bearer-value");
  await page.getByRole("button", { exact: true, name: "Connect" }).click();

  await expect(page.getByRole("status", { name: "Connection status" })).toHaveText("connected");
  await expect(page.getByText("owner-1", { exact: true })).toBeVisible();

  await page.getByLabel("Message").fill("Summarize the current checkout alert.");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("Summarize the current checkout alert.", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Mock diagnosis response for: Summarize the current checkout alert.", { exact: true })
  ).toBeVisible();
  await expect(page.getByText("Turn 1 completed.")).toBeVisible();
});
