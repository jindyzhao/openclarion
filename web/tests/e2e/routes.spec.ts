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
  await expect(page.getByLabel("Diagnosis conclusion")).toContainText(
    "Checkout latency remains correlated with the payment deployment."
  );
  await expect(page.getByLabel("Diagnosis conclusion")).toContainText("human review");

  await page.getByRole("link", { name: "Review diagnosis" }).click();
  await expect(page).toHaveURL(
    /\/diagnosis-room\?evidence_snapshot_id=9001&report_id=101&sub_report_id=501&intent=review_conclusion$/
  );
  await expect(page.getByText("Report #101 diagnosis")).toBeVisible();
  await expect(page.getByText("Opened from report #101, subreport #501 using evidence snapshot #9001.")).toBeVisible();
  await expect(page.getByLabel("Evidence snapshot")).toHaveValue("9001");
  await expect(page.getByLabel("Message")).toHaveValue(/Verify the current diagnosis conclusion/);
});

test("diagnosis room route connects, queries state, and submits a turn", async ({ page }) => {
  await page.goto("/diagnosis-room");

  await expect(page.getByRole("heading", { name: "Diagnosis Room" })).toBeVisible();
  await page.getByLabel("Session ID").fill("diagnosis-session-42");
  await page.getByLabel("Bearer token").fill("test-bearer-value");
  await page.getByRole("button", { name: /Connect/ }).click();

  await expect(page.getByRole("status", { name: "Connection status" })).toHaveText("connected");
  await expect(page.getByText("owner-1", { exact: true })).toBeVisible();

  await page.getByLabel("Message").fill("Summarize the current checkout alert.");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("Summarize the current checkout alert.", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Mock diagnosis response for: Summarize the current checkout alert.", { exact: true })
  ).toBeVisible();
  await expect(page.getByText("Consultation Insight", { exact: true })).toBeVisible();
  await expect(page.getByText("needs_evidence", { exact: true })).toBeVisible();
  await expect(page.getByText("Executable Evidence Plan", { exact: true })).toBeVisible();
  await expect(page.getByText("Current active alerts", { exact: true })).toBeVisible();
  await expect(page.getByText("Collection Results", { exact: true })).toBeVisible();
  await expect(page.getByText("Active alert collection succeeded.", { exact: true })).toBeVisible();
  await expect(page.getByText("CPUHigh / prod")).toBeVisible();
  await expect(page.getByText("Restart cause", { exact: true })).toBeVisible();
  await expect(page.getByText("Metric window", { exact: true })).toBeVisible();
  await expect(page.getByText("Turn 1 completed.")).toBeVisible();

  await page.getByRole("button", { name: "Refresh State" }).click();
  await expect(page.getByText("Loaded state: open, 1 turn(s).")).toBeVisible();
  await expect(page.getByText("Restart cause", { exact: true })).toBeVisible();
});

test("settings overview route renders the alert operations configuration graph", async ({ page }) => {
  await page.goto("/settings");

  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
  await expect(page.getByLabel("Alert operations configuration sequence")).toContainText("Source");
  await expect(page.getByLabel("Settings surfaces")).toContainText("Alert sources");
  await expect(page.getByLabel("Settings surfaces")).toContainText("Diagnosis tools");
  await expect(page.getByLabel("Settings surfaces")).toContainText("Workflow policies");
  await expect(page.getByLabel("Next setup stage")).toContainText("Retained live proof");
  await expect(page.getByLabel("Next setup stage")).toContainText("Proof pending");
  await expect(page.getByLabel("Retained proof targets")).toContainText("Policy replay");
  await expect(page.getByLabel("Retained proof targets")).toContainText("Scheduled trigger");
  await expect(page.getByLabel("Retained proof targets")).toContainText("Pending external inputs");
  await expect(page.getByLabel("Active workflow topology")).toContainText("Default report workflow");
  await expect(page.getByLabel("Active workflow topology")).toContainText("Primary Prometheus");
  await expect(page.getByLabel("Selected alert workflow topology")).toContainText("AI room");
  await expect(page.getByLabel("Next topology actions")).toContainText("Enable diagnosis tools");
  await expect(page.getByLabel("Next topology actions")).toContainText("Run impact preview");
  await expect(page.getByLabel("Settings surfaces")).toContainText("Ready");
  await expect(page.getByText("Live proof gate")).toBeVisible();
  await expect(page.getByText(/configuration objects/)).toBeVisible();
});

test("alert source settings route lists and creates profiles", async ({ page }) => {
  await page.goto("/settings/alert-sources");

  await expect(page.getByRole("heading", { name: "Alert Sources" })).toBeVisible();
  await expect(page.getByText("Primary Prometheus")).toBeVisible();
  const stagingAlertmanagerRow = page.getByRole("row", { name: /Staging Alertmanager/ });
  await expect(stagingAlertmanagerRow).toContainText("/api/v1/alert-sources/2/webhooks/alertmanager");

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

test("report workflow policy settings route creates and toggles policies", async ({ page }) => {
  await page.goto("/settings/report-workflow-policies");

  await expect(page.getByRole("heading", { name: "Workflow Policies" })).toBeVisible();
  await expect(page.getByRole("row", { name: /Default report workflow/ })).toBeVisible();
  await expect(page.getByLabel("AI consultation workflow readiness")).toContainText("Default report workflow");
  await expect(page.getByLabel("AI consultation workflow readiness")).toContainText("AI room");
  await expect(page.getByLabel("AI consultation workflow counters")).toContainText("Room-ready policies");

  const settingsForm = page.locator("form");
  await page.getByLabel("Name").fill("Cascade report workflow");
  const alertSourceSelect = settingsForm.getByRole("combobox", { name: /Alert source/ });
  await alertSourceSelect.click();
  await alertSourceSelect.fill("Primary Prometheus");
  await alertSourceSelect.press("Enter");
  const groupingPolicySelect = settingsForm.getByRole("combobox", { name: /Grouping policy/ });
  await groupingPolicySelect.click();
  await groupingPolicySelect.fill("Default alert grouping");
  await groupingPolicySelect.press("Enter");
  await settingsForm.getByText("Suggest room", { exact: true }).click();
  await settingsForm.getByRole("button", { name: "Save Policy" }).click();

  await expect(page.getByRole("status")).toContainText("Policy saved.");
  const cascadeRow = page.getByRole("row", { name: /Cascade report workflow/ }).first();
  await expect(cascadeRow).toContainText("Draft");

  await cascadeRow.getByRole("button", { name: "Impact" }).click();
  await expect(page.getByRole("status")).toContainText("Impact preview ready");
  const impactDialog = page.getByRole("dialog", { name: /Impact Preview/ });
  await expect(impactDialog).toContainText("ok");
  await expect(impactDialog).toContainText("HighCPU");
  await expect(page.getByLabel("AI consultation workflow counters")).toContainText("Ready previews");
  await impactDialog.locator("button").filter({ hasText: "Close" }).click();
  await expect(impactDialog).toBeHidden();

  await cascadeRow.getByRole("button", { name: "Enable" }).click();
  await expect(page.getByRole("status")).toContainText("Policy enabled.");
  await expect(cascadeRow).toContainText("Enabled");

  await cascadeRow.getByRole("button", { name: "Replay" }).click();
  const replayDialog = page.getByRole("dialog", { name: /Replay Policy/ });
  await replayDialog.getByRole("button", { name: "Start Replay" }).click();
  await expect(page.getByRole("status")).toContainText("Replay accepted.");
  await expect(replayDialog).toContainText("report-batch-policy-smoke");
  await replayDialog.locator("button").filter({ hasText: "Close" }).click();
  await expect(replayDialog).toBeHidden();

  await cascadeRow.getByRole("button", { name: "Disable" }).click();
  await expect(page.getByRole("status")).toContainText("Policy disabled.");
  await expect(cascadeRow).toContainText("Draft");
});

test("report workflow schedule settings route creates and toggles schedules", async ({ page }) => {
  await page.goto("/settings/report-workflow-schedules");

  await expect(page.getByRole("heading", { name: "Workflow Schedules" })).toBeVisible();
  await expect(page.getByText("Daily report window")).toBeVisible();
  await expect(page.getByText("openclarion-report-policy-1-daily")).toBeVisible();

  const settingsForm = page.locator("form");
  await page.getByLabel("Name").fill("Six-hour report window");
  const workflowPolicySelect = settingsForm.getByRole("combobox", { name: /Report workflow policy/ });
  await workflowPolicySelect.click();
  await workflowPolicySelect.fill("Default report workflow");
  await workflowPolicySelect.press("Enter");
  await page.getByLabel("Temporal Schedule ID").fill("openclarion-report-policy-1-6h");
  await page.getByLabel("Interval seconds").fill("21600");
  await page.getByLabel("Offset seconds").fill("0");
  await page.getByLabel("Replay window seconds").fill("3600");
  await page.getByLabel("Replay delay seconds").fill("300");
  await page.getByLabel("Replay limit").fill("10000");
  await page.getByLabel("Catch-up seconds").fill("1800");
  await settingsForm.getByRole("button", { name: "Save Schedule" }).click();

  await expect(page.getByRole("status")).toContainText("Schedule saved.");
  const scheduleRow = page.getByRole("row", { name: /Six-hour report window/ });
  await expect(scheduleRow).toContainText("6h");
  await expect(scheduleRow).toContainText("Draft");

  await scheduleRow.getByRole("button", { name: "Enable" }).click();
  await expect(page.getByRole("status")).toContainText("Schedule enabled.");
  await expect(scheduleRow).toContainText("Enabled");

  await scheduleRow.getByRole("button", { name: "Disable" }).click();
  await expect(page.getByRole("status")).toContainText("Schedule disabled.");
  await expect(scheduleRow).toContainText("Draft");
});

test("diagnosis tool template settings route creates and toggles templates", async ({ page }) => {
  await page.goto("/settings/diagnosis-tool-templates");

  await expect(page.getByRole("heading", { name: "Diagnosis Tools" })).toBeVisible();
  await expect(page.getByText("CPU saturation range")).toBeVisible();

  const settingsForm = page.locator("form");
  await page.getByLabel("Name").fill("Memory pressure range");
  const alertSourceSelect = settingsForm.getByRole("combobox", { name: /Alert source/ });
  await alertSourceSelect.click();
  await alertSourceSelect.fill("Primary Prometheus");
  await alertSourceSelect.press("Enter");
  await settingsForm.getByText("Range", { exact: true }).click();
  await page.getByLabel("Query template").fill("container_memory_working_set_bytes");
  await page.getByLabel("Default limit").fill("5");
  await page.getByLabel("Step seconds").fill("60");
  await page.getByLabel("Default window seconds").fill("3600");
  await page.getByLabel("Max window seconds").fill("21600");
  await settingsForm.getByRole("button", { name: "Save Template" }).click();

  await expect(page.getByRole("status")).toContainText("Template saved.");
  const templateRow = page.getByRole("row", { name: /Memory pressure range/ });
  await expect(templateRow).toContainText("Range metric");
  await expect(templateRow).toContainText("Draft");

  await templateRow.getByRole("button", { name: "Enable" }).click();
  await expect(page.getByRole("status")).toContainText("Template enabled.");
  await expect(templateRow).toContainText("Enabled");

  await templateRow.getByRole("button", { name: "Disable" }).click();
  await expect(page.getByRole("status")).toContainText("Template disabled.");
  await expect(templateRow).toContainText("Draft");
});

test("notification channel settings route lists and creates channels", async ({ page }) => {
  await page.goto("/settings/notification-channels");

  await expect(page.getByRole("heading", { name: "Notification Channels" })).toBeVisible();
  await expect(page.getByText("Operations webhook")).toBeVisible();
  await expect(page.getByText("secret/example/ops-webhook")).toBeVisible();

  const operationsRow = page.getByRole("row", { name: /Operations webhook/ });
  await operationsRow.getByRole("button", { name: "Test" }).click();
  await expect(page.getByRole("status")).toContainText(
    "Secret-backed notification channel tests require a server-side secret resolver."
  );
  await expect(operationsRow).toContainText("credentials_unavailable");

  const settingsForm = page.locator("form");
  await page.getByLabel("Name").fill("Incident webhook");
  await page.getByLabel("Secret reference").fill("secret/example/incident-webhook");
  await settingsForm.getByLabel("Diagnosis close").check();
  await page.getByLabel("Labels").fill("team=ops");
  await page.getByLabel("Enabled").check();
  await settingsForm.getByRole("button", { name: "Save Channel" }).click();

  await expect(page.getByRole("status")).toContainText("Channel saved.");
  const incidentRow = page.getByRole("row", { name: /Incident webhook/ });
  await expect(incidentRow).toContainText("secret/example/incident-webhook");
  await expect(incidentRow).toContainText("diagnosis_close");
  await expect(incidentRow).toContainText("Enabled");
});
