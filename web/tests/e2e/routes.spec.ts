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

test("alert source settings route lists and creates profiles", async ({ page }) => {
  await page.goto("/settings/alert-sources");

  await expect(page.getByRole("heading", { name: "Alert Sources" })).toBeVisible();
  await expect(page.getByText("Primary Prometheus")).toBeVisible();

  const primaryPrometheusRow = page.getByRole("row", { name: /Primary Prometheus/ });
  await primaryPrometheusRow.getByRole("button", { name: "Test" }).click();
  await expect(page.getByRole("status")).toContainText(
    "Secret-backed connection tests require a server-side secret resolver."
  );
  await expect(primaryPrometheusRow).toContainText("credentials_unavailable");

  const settingsForm = page.locator("form");
  await page.getByLabel("Name").fill("Team Alertmanager");
  await settingsForm.getByText("Alertmanager", { exact: true }).click();
  await settingsForm.getByText("Bearer", { exact: true }).click();
  await page.getByLabel("Base URL").fill("https://alertmanager-team.example.test");
  await page.getByLabel("Secret reference").fill("secret/openclarion/alertmanager-bearer");
  await page.getByLabel("Labels").fill("env=prod\nowner=sre");
  await page.getByLabel("Enabled").check();
  await page.getByRole("button", { name: "Save Profile" }).click();

  await expect(page.getByRole("status")).toContainText("Profile saved.");
  await expect(page.getByText("Team Alertmanager")).toBeVisible();
  await expect(page.getByText("secret/openclarion/alertmanager-bearer")).toBeVisible();
});

test("grouping policy settings route previews and creates policies", async ({ page }) => {
  await page.goto("/settings/grouping-policies");

  await expect(page.getByRole("heading", { name: "Grouping Policies" })).toBeVisible();
  await expect(page.getByText("Default alert grouping")).toBeVisible();

  const defaultPolicyRow = page.getByRole("row", { name: /Default alert grouping/ });
  await defaultPolicyRow.getByRole("button", { name: "Preview" }).click();
  await expect(page.getByRole("status")).toContainText("Preview scanned 3 events and matched 2.");
  await expect(defaultPolicyRow).toContainText("1 groups");
  await expect(page.getByText("HighCPU")).toBeVisible();
  await expect(page.getByText("101, 102")).toBeVisible();

  const settingsForm = page.locator("form");
  await page.getByLabel("Name").fill("Service grouping");
  await page.getByLabel("Dimension keys").fill("service\ncluster");
  await page.getByLabel("Severity key").fill("severity");
  await page.getByLabel("Source filter").fill("prometheus");
  await page.getByLabel("Enabled").check();
  await settingsForm.getByRole("button", { name: "Save Policy" }).click();

  await expect(page.getByRole("status")).toContainText("Policy saved.");
  await expect(page.getByText("Service grouping")).toBeVisible();
  await expect(page.getByText("cluster")).toBeVisible();
});
