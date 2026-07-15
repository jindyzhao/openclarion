import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

const diagnosisSessionCookieName = "openclarion_diagnosis_session";

test.beforeEach(async ({ baseURL, context }) => {
  await context.addCookies([
    {
      httpOnly: true,
      name: diagnosisSessionCookieName,
      sameSite: "Lax",
      url: playwrightBaseURL(baseURL),
      value: "session.token.one",
    },
  ]);
});

function evidenceSnapshotInput(page: Page) {
  return page.getByRole("spinbutton", { name: /Evidence snapshot/ });
}

async function checkConnectionAuth(page: Page) {
  const connectionControls = page.getByLabel("Connection controls");
  const checkAuthButton = connectionControls.getByRole("button", {
    name: /Check auth/,
  }).first();
  await expect(checkAuthButton).toBeEnabled();
  await checkAuthButton.click();
  await expect(page.getByText(/Authenticated as operator-1/).first()).toBeVisible();
}

async function refreshConnectionState(page: Page) {
  await page
    .getByLabel("Connection controls")
    .getByRole("button", { name: /Refresh State/ })
    .first()
    .click();
}

function playwrightBaseURL(baseURL: string | undefined) {
  return (
    baseURL ??
    `http://127.0.0.1:${process.env.OPENCLARION_PLAYWRIGHT_WEB_PORT ?? "32100"}`
  );
}

test("dashboard route renders mocked operational summary", async ({ page }) => {
  await page.goto("/dashboard");

  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
  await expect(page.getByLabel("Dashboard metrics")).toContainText(
    "Firing alerts",
  );
  await expect(page.getByText("92%")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Report Delivery" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "AI Diagnosis Rooms" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "AI Handoff Backlog" }),
  ).toBeVisible();
  const handoffBacklog = page.getByLabel("AI handoff backlog", {
    exact: true,
  });
  const handoffHealth = page.getByLabel("AI handoff backlog health");
  await expect(handoffHealth).toContainText("Snapshots needing room");
  await expect(handoffHealth).toContainText("Rooms started");
  await expect(handoffBacklog).toContainText("PaymentErrorRateHigh");
  await expect(handoffBacklog).toContainText("manual room");
  const manualRoomLink = handoffBacklog.getByRole("link", {
    name: "Create room #9004",
  });
  await expect(manualRoomLink).toHaveAttribute(
    "href",
    "/diagnosis-room?evidence_snapshot_id=9004&intent=alert_review",
  );
  const manualRoomHref = await manualRoomLink.getAttribute("href");
  if (manualRoomHref === null) {
    throw new Error("manual diagnosis room link missing href");
  }
  const roomHealth = page.getByLabel("AI diagnosis room health");
  await expect(roomHealth).toContainText("Attention");
  await expect(roomHealth).toContainText("Ready");
  await expect(roomHealth).toContainText("Notification failures");
  const recentRooms = page.getByLabel("Recent diagnosis rooms");
  await expect(recentRooms).toContainText("AI delivery incomplete");
  await expect(recentRooms).toContainText("Final-ready notification");
  await expect(recentRooms).toContainText("delivered");
  await expect(recentRooms).toContainText("Notification failed");
  await expect(recentRooms).toContainText("Review channel");
  await expect(recentRooms).toContainText("diagnosis-session-closed-finalized");
  await expect(recentRooms).toContainText("AI proof missing");
  await expect(recentRooms).toContainText("Close notification");
  const roomLink = page.getByRole("link", {
    name: "diagnosis-session-auto-p1-s9001",
  });
  await expect(roomLink).toBeVisible();
  await expect(roomLink).toHaveAttribute(
    "href",
    /\/diagnosis-room\?evidence_snapshot_id=9001&session_id=diagnosis-session-auto-p1-s9001/,
  );

  const roomHref = await roomLink.getAttribute("href");
  if (roomHref === null) {
    throw new Error("diagnosis room link missing href");
  }
  await page.goto(roomHref);
  await expect(page).toHaveURL(/\/diagnosis-room\?/);
  await expect(page.getByLabel("Session ID")).toHaveValue(
    "diagnosis-session-auto-p1-s9001",
  );
  await expect(evidenceSnapshotInput(page)).toHaveValue("9001");
  await expect(page.getByLabel("Alert context")).toContainText(
    "CheckoutLatencyHigh",
  );
  await expect(page.getByLabel("Alert context")).toContainText(
    "service: checkout",
  );
  await expect(page.getByLabel("Alert context")).toContainText(
    "Alert source profile",
  );
  await expect(page.getByLabel("Alert context")).toContainText("#1");
  await expect(page.getByLabel("Message")).toHaveValue(
    /Start an AI diagnosis for CheckoutLatencyHigh/,
  );
  await expect(
    page.getByRole("button", {
      name: "Selected diagnosis-session-auto-p1-s9001",
    }),
  ).toBeDisabled();

  await page.goto(manualRoomHref);
  await expect(page).toHaveURL(
    /\/diagnosis-room\?evidence_snapshot_id=9004&intent=alert_review/,
  );
  await expect(page.getByText("Alert diagnosis")).toBeVisible();
  await expect(evidenceSnapshotInput(page)).toHaveValue("9004");
  await expect(page.getByLabel("Alert context")).toContainText(
    "PaymentErrorRateHigh",
  );
  await expect(page.getByLabel("Alert context")).toContainText(
    "service: payments",
  );
  await expect(page.getByText("No diagnosis room linked")).toBeVisible();
  await expect(page.getByLabel("Message")).toHaveValue(
    /Start an AI diagnosis for PaymentErrorRateHigh/,
  );
});

test("alerts route links alert events to evidence snapshots and diagnosis rooms", async ({
  page,
}) => {
  await page.goto("/alerts");

  await expect(page.getByRole("heading", { name: "Alerts" })).toBeVisible();
  await expect(page.getByLabel("Alert summary")).toContainText("Linked alerts");
  await expect(page.getByLabel("Recent alerts")).toContainText(
    "CheckoutLatencyHigh",
  );
  const checkoutRow = page.locator('tr[data-row-key="7001"]');
  await expect(checkoutRow).toContainText("profile #1");
  await expect(checkoutRow).toContainText("Snapshot #9001");
  await expect(checkoutRow).toContainText("diagnosis-session-auto-p1-s9001");
  await expect(checkoutRow).toContainText(
    "Checkout latency is correlated with downstream saturation.",
  );
  await expect(checkoutRow).toContainText("high");
  await expect(checkoutRow).toContainText("Final-ready notification");
  await expect(checkoutRow).toContainText("delivered");
  await expect(checkoutRow).toContainText(
    "diagnosis-session-notification-failed",
  );
  await expect(checkoutRow).toContainText("Notification failed");
  await expect(checkoutRow).toContainText("Review channel");
  const alertActionStack = checkoutRow.locator(".alert-action-stack");
  await expect(alertActionStack).toContainText("Review channel");
  await expect(alertActionStack).toContainText(
    "Review failed AI notification delivery before relying on downstream handoff.",
  );
  await expect(
    alertActionStack.getByRole("link", { name: "Review channel" }),
  ).toHaveAttribute("href", "/settings/notification-channels?channel_id=2");
  await expect(checkoutRow).toContainText("diagnosis-session-closed-finalized");
  await expect(checkoutRow).toContainText("Close notification");
  await expect(checkoutRow).toContainText(
    "operator_confirmed_final_conclusion",
  );
  await expect(checkoutRow).toContainText("wecom-msg-close-closed");
  await expect(checkoutRow).toContainText(
    "Checkout latency was confirmed after operator evidence review and the room was closed.",
  );
  const reviewConclusionLink = checkoutRow
    .getByRole("link", { name: "Review conclusion" })
    .first();
  await expect(reviewConclusionLink).toHaveAttribute(
    "href",
    /\/diagnosis-room\?evidence_snapshot_id=9001&intent=review_conclusion&session_id=diagnosis-session-auto-p1-s9001/,
  );
  const checkoutDetails = page.getByLabel(
    "Alert details for CheckoutLatencyHigh",
  );
  await expect(checkoutDetails).toContainText("Canonical fingerprint");
  await expect(checkoutDetails).toContainText("Labels");
  await expect(checkoutDetails).toContainText("Annotations");
  await expect(checkoutDetails).toContainText(
    "Checkout p95 latency is above the warning threshold.",
  );
  await expect(checkoutDetails).toContainText("Evidence and AI handoff");
  await expect(checkoutDetails).toContainText("Open diagnosis");
  await reviewConclusionLink.click();
  await expect(page).toHaveURL(
    /\/diagnosis-room\?evidence_snapshot_id=9001&intent=review_conclusion&session_id=diagnosis-session-auto-p1-s9001/,
  );
  const diagnosisHandoff = page.getByLabel("Diagnosis handoff");
  await expect(diagnosisHandoff).toContainText("Diagnosis Handoff");
  await expect(diagnosisHandoff).toContainText("AI delivery incomplete");
  await expect(diagnosisHandoff).toContainText("Use review prompt");
  await expect(diagnosisHandoff).toContainText(
    "Checkout latency is correlated with downstream saturation.",
  );
  await expect(page.getByLabel("Message")).toHaveValue(
    /Verify the current diagnosis conclusion/,
  );
  await page.goto("/alerts");

  const diskRow = page.locator('tr[data-row-key="7002"]');
  await expect(diskRow).toContainText("No snapshot linked");
  await expect(diskRow).toContainText("Awaiting evidence");
  await expect(diskRow).toContainText(
    "Replay alert window to create evidence.",
  );
  const replayWindowButton = diskRow.getByRole("button", {
    name: "Replay window",
  });
  await expect(replayWindowButton).toBeEnabled();
  await replayWindowButton.click();
  await expect(
    page.getByText(
      "Replay accepted with 1 evidence snapshot; AI diagnosis started 1 room.",
    ),
  ).toBeVisible();
  const latestReplayProof = page.getByLabel("Latest alert replay proof");
  await expect(latestReplayProof).toContainText("Latest Replay Proof");
  await expect(latestReplayProof).toContainText("NodeDiskPressure");
  await expect(latestReplayProof).toContainText("Workflow accepted");
  await expect(latestReplayProof).toContainText(
    "Correlation alert-replay-7002",
  );
  await expect(latestReplayProof).toContainText("1 saved");
  await expect(latestReplayProof).toContainText("1 room");
  await expect(latestReplayProof).toContainText(
    "Review room notification timeline",
  );
  await expect(latestReplayProof).toContainText("Room timeline");
  await expect(latestReplayProof).toContainText(
    "confirm assistant, final-ready, or close notifications",
  );
  await expect(
    latestReplayProof.getByRole("link", { name: "Open room timeline" }),
  ).toHaveAttribute(
    "href",
    /\/diagnosis-room\?evidence_snapshot_id=9002&intent=review_conclusion&session_id=diagnosis-session-auto-p1-s9002/,
  );
  await expect(
    latestReplayProof.getByRole("link", { name: "Review room #9002" }),
  ).toHaveAttribute(
    "href",
    /\/diagnosis-room\?evidence_snapshot_id=9002&intent=review_conclusion&session_id=diagnosis-session-auto-p1-s9002/,
  );
  await expect(
    latestReplayProof.getByRole("link", { name: "Snapshot #9002" }),
  ).toHaveAttribute(
    "href",
    "/diagnosis-room?evidence_snapshot_id=9002&intent=alert_review",
  );
  await expect(
    latestReplayProof
      .getByLabel("Replay proof links")
      .getByRole("link", { name: /Room #9002/ }),
  ).toHaveAttribute(
    "href",
    /\/diagnosis-room\?evidence_snapshot_id=9002&intent=review_conclusion&session_id=diagnosis-session-auto-p1-s9002/,
  );
  await page.goto("/alerts");
  const linkedDiskRow = page.locator('tr[data-row-key="7002"]');
  await expect(linkedDiskRow).toContainText("Snapshot #9002");
  await expect(linkedDiskRow).toContainText("diagnosis-session-auto-p1-s9002");
  await expect(linkedDiskRow).toContainText("AI review queued");

  await linkedDiskRow
    .getByRole("link", { name: "diagnosis-session-auto-p1-s9002" })
    .click();
  await expect(page).toHaveURL(/\/diagnosis-room\?/);
  await expect(page.getByLabel("Session ID")).toHaveValue(
    "diagnosis-session-auto-p1-s9002",
  );
  await expect(evidenceSnapshotInput(page)).toHaveValue("9002");
  await expect(page.getByLabel("Alert context")).toContainText(
    "NodeDiskPressure",
  );
  await expect(page.getByLabel("Message")).toHaveValue(
    /Start an AI diagnosis for NodeDiskPressure/,
  );

  await page.goto("/alerts");
  const refreshedCheckoutRow = page.locator('tr[data-row-key="7001"]');
  await refreshedCheckoutRow
    .getByRole("link", { name: "diagnosis-session-auto-p1-s9001" })
    .click();
  await expect(page).toHaveURL(/\/diagnosis-room\?/);
  await expect(page.getByLabel("Session ID")).toHaveValue(
    "diagnosis-session-auto-p1-s9001",
  );
  await expect(evidenceSnapshotInput(page)).toHaveValue("9001");
  await expect(page.getByLabel("Alert context")).toContainText(
    "CheckoutLatencyHigh",
  );
  await expect(page.getByLabel("Alert context")).toContainText(
    "Snapshot created",
  );
  await expect(page.getByLabel("Alert context")).toContainText(
    "Alert source profile",
  );
  await expect(page.getByLabel("Alert context")).toContainText("#1");
  await expect(page.getByLabel("Message")).toHaveValue(
    /Start an AI diagnosis for CheckoutLatencyHigh/,
  );
  await expect(
    page.getByRole("button", {
      name: "Selected diagnosis-session-auto-p1-s9001",
    }),
  ).toBeDisabled();
});

test("report routes render list, detail, and evidence traceability", async ({
  page,
}) => {
  await page.goto("/reports");

  await expect(page.getByRole("heading", { name: "Reports" })).toBeVisible();
  const reportLink = page.getByRole("link", {
    name: "Checkout latency incident",
  });
  await expect(reportLink).toBeVisible();

  const reportSearch = page.getByLabel("Search reports");
  await reportSearch.fill("missing report");
  await expect(page.getByText("No reports match these filters.")).toBeVisible();
  await reportSearch.fill("checkout latency");
  await expect(reportLink).toBeVisible();

  const severityFilter = page
    .getByLabel("Filter reports by severity")
    .first();
  await severityFilter.click();
  await page.getByTitle("Info").click();
  await expect(page.getByText("No reports match these filters.")).toBeVisible();
  await severityFilter.click();
  await page.getByTitle("Warning").click();
  await expect(reportLink).toBeVisible();

  await reportLink.click();
  await expect(page).toHaveURL(/\/reports\/101$/);
  await expect(
    page.getByRole("heading", { name: "Checkout latency incident" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Evidence Traceability" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Diagnosis Readiness" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Report Delivery Proof" }),
  ).toBeVisible();
  const deliveryProof = page.getByLabel("Final report notification delivery proof");
  await expect(deliveryProof).toContainText("Delivered");
  await expect(deliveryProof).toContainText(
    "final_report:101/notification/final",
  );
  await expect(deliveryProof).toContainText("wecom-final-report-101");
  await expect(deliveryProof).toContainText("accepted");
  await expect(page.getByLabel("Diagnosis readiness")).toContainText(
    "Reviewed subreports",
  );
  await expect(page.getByLabel("Diagnosis readiness")).toContainText("2 / 2");
  await expect(page.getByLabel("Diagnosis readiness")).toContainText(
    "Human review",
  );
  await expect(page.getByLabel("Report AI review status")).toContainText(
    "Needs evidence",
  );
  await expect(page.getByLabel("Report AI review status")).toContainText(
    "AI is waiting for 2 missing evidence items and 1 executable evidence task. 2 residual collection suggestions remain documented but do not block confirmation.",
  );
  await expect(page.getByLabel("Diagnosis readiness")).toContainText(
    "Evidence requested",
  );
  await expect(page.getByLabel("Diagnosis readiness")).toContainText(
    "Evidence still needed",
  );
  await expect(page.getByLabel("Diagnosis readiness")).toContainText(
    "Supplemental evidence",
  );
  await expect(page.getByLabel("Diagnosis readiness")).toContainText(
    "Latest confidence",
  );
  await expect(page.getByLabel("Diagnosis readiness")).toContainText("high");
  const handoffPlan = page.getByLabel("Report diagnosis handoff plan");
  await expect(handoffPlan).toContainText("Report handoff plan");
  await expect(handoffPlan).toContainText("Evidence follow-up required");
  await expect(handoffPlan).toContainText("Report generated");
  await expect(handoffPlan).toContainText(
    "Final report #101 was generated by FinalReportWorkflow.",
  );
  await expect(handoffPlan).toContainText("Evidence snapshots");
  await expect(handoffPlan).toContainText(
    "2 evidence snapshots linked to the AI diagnosis path.",
  );
  await expect(handoffPlan).toContainText("AI consultation");
  await expect(handoffPlan).toContainText(
    "2 of 2 linked subreports have AI diagnosis state.",
  );
  await expect(handoffPlan).toContainText("Evidence follow-up");
  await expect(handoffPlan).toContainText(
    "Resolve 2 missing evidence items and 1 executable evidence task before final confirmation.",
  );
  await expect(handoffPlan).toContainText("Operator decision");
  const nextDiagnosisAction = page.getByLabel("Next diagnosis action");
  await expect(nextDiagnosisAction).toContainText("Needs evidence");
  await expect(nextDiagnosisAction).toContainText("Checkout JVM memory");
  await expect(nextDiagnosisAction).toContainText(
    "AI requested 2 missing evidence items and 1 executable evidence task. 2 residual collection suggestions remain documented but do not block confirmation.",
  );
  await expect(
    nextDiagnosisAction.getByRole("link", { name: "Resolve evidence" }),
  ).toHaveAttribute(
    "href",
    "/diagnosis-room?evidence_snapshot_id=9003&intent=confidence_review&report_id=101&sub_report_id=502&session_id=diagnosis-session-302",
  );
  await expect(
    page.getByRole("heading", { name: "Decision Records" }),
  ).toBeVisible();
  const decisionRecords = page.getByLabel("Report decision records");
  await expect(decisionRecords).toContainText("Checkout API latency");
  await expect(decisionRecords).toContainText("Conclusion stored");
  await expect(decisionRecords).toContainText("diagnosis-session-301");
  await expect(decisionRecords).toContainText("diagnosis-room-final-ready.v1");
  await expect(decisionRecords).toContainText(
    "Final AI conclusion is stored and waiting for operator confirmation.",
  );
  await expect(decisionRecords).toContainText("Final-ready notification");
  await expect(decisionRecords).toContainText("wecom-report-final-ready-301");
  await expect(
    decisionRecords.getByRole("link", { name: "Confirm in diagnosis room" }),
  ).toHaveAttribute(
    "href",
    "/diagnosis-room?evidence_snapshot_id=9001&intent=review_conclusion&report_id=101&sub_report_id=501&session_id=diagnosis-session-301",
  );
  await expect(decisionRecords).toContainText("Checkout JVM memory");
  await expect(decisionRecords).toContainText("Evidence required");
  await expect(
    decisionRecords.getByRole("link", {
      name: "Resolve evidence in diagnosis room",
    }),
  ).toHaveAttribute(
    "href",
    "/diagnosis-room?evidence_snapshot_id=9003&intent=confidence_review&report_id=101&sub_report_id=502&session_id=diagnosis-session-302",
  );
  const nextEvidenceActions = page.getByLabel(
    "Next diagnosis evidence actions",
  );
  await expect(nextEvidenceActions).toContainText("5");
  await expect(nextEvidenceActions).toContainText("Runtime context");
  await expect(nextEvidenceActions).toContainText(
    "Attach recent restart and deployment details before confirming the diagnosis.",
  );
  await expect(nextEvidenceActions).toContainText("Owner mitigation");
  await expect(nextEvidenceActions).toContainText(
    "Attach the service owner's mitigation note before final confirmation.",
  );
  await expect(nextEvidenceActions).toContainText(
    "Need checkout JVM heap trend for the incident window.",
  );
  await expect(nextEvidenceActions).toContainText(
    "metric_range_query / query jvm_memory_used_bytes",
  );
  await expect(nextEvidenceActions).toContainText(
    "2 more evidence actions in the traceability list.",
  );
  await expect(page.getByText("Evidence snapshot #9001")).toBeVisible();
  await expect(page.getByText("Evidence snapshot #9003")).toBeVisible();
  await expect(
    page.getByLabel("Checkout API latency AI review status"),
  ).toContainText("Needs confirmation");
  await expect(
    page.getByLabel("Checkout JVM memory AI review status"),
  ).toContainText("Needs evidence");
  const diagnosisConclusion = page.getByLabel("Diagnosis conclusion");
  const diagnosisProgress = page.getByLabel("Diagnosis progress");
  await expect(diagnosisConclusion).toContainText(
    "Checkout latency remains correlated with the payment deployment.",
  );
  await expect(diagnosisConclusion).toContainText("human review");
  await expect(diagnosisConclusion).toContainText(
    "diagnosis-room-final-ready.v1",
  );
  await expect(diagnosisConclusion).toContainText("Evidence");
  await expect(diagnosisConclusion).toContainText("#9001");
  await expect(
    diagnosisConclusion.getByLabel("Confidence timeline"),
  ).toContainText("low confidence");
  await expect(
    diagnosisConclusion.getByLabel("Confidence timeline"),
  ).toContainText("needs_evidence");
  await expect(
    diagnosisConclusion.getByLabel("Confidence timeline"),
  ).toContainText(
    "Latency evidence is present but deployment timing is missing.",
  );
  await expect(
    diagnosisConclusion.getByLabel("Requested evidence"),
  ).toContainText("metric_range_query");
  await expect(
    diagnosisConclusion.getByLabel("Requested evidence"),
  ).toContainText("Need checkout deployment timing.");
  await expect(
    diagnosisConclusion.getByLabel("Requested evidence"),
  ).toContainText(
    "histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
  );
  await expect(
    diagnosisConclusion.getByLabel("Requested evidence"),
  ).toContainText("source #7");
  await expect(
    diagnosisConclusion.getByLabel("Requested evidence"),
  ).toContainText("collected");
  await expect(
    diagnosisConclusion.getByLabel("Collected evidence"),
  ).toContainText("Metric range collection succeeded.");
  await expect(
    diagnosisConclusion.getByLabel("Collected evidence"),
  ).toContainText("2 metric series");
  await expect(
    diagnosisConclusion.getByLabel("Collected evidence"),
  ).toContainText("prometheus");
  await expect(
    diagnosisConclusion.getByLabel("Missing evidence"),
  ).toContainText("Deployment window");
  await expect(
    diagnosisConclusion.getByLabel("Missing evidence"),
  ).toContainText(
    "Provide checkout deployment timing before raising confidence.",
  );
  await expect(
    diagnosisConclusion.getByLabel("Collection suggestions"),
  ).toContainText("Latency trend");
  await expect(
    diagnosisConclusion.getByLabel("Collection suggestions"),
  ).toContainText(
    "Collect a bounded checkout p95 range query for the incident window.",
  );
  await expect(
    diagnosisConclusion.getByLabel("Confidence timeline"),
  ).toContainText("high confidence");
  await expect(
    diagnosisConclusion.getByLabel("Confidence timeline"),
  ).toContainText("ready_for_review");
  await expect(
    diagnosisConclusion.getByLabel("Confidence timeline"),
  ).toContainText("Deployment evidence explains the latency onset.");
  await expect(
    diagnosisConclusion.getByLabel("Supplemental evidence"),
  ).toContainText("Deployment window");
  await expect(
    diagnosisConclusion.getByLabel("Supplemental evidence"),
  ).toContainText(
    "The payment deployment started two minutes before checkout p95 crossed the warning threshold.",
  );
  await expect(
    diagnosisConclusion.getByLabel("Conclusion context references"),
  ).toContainText("chat_session:401/turn:501");
  await expect(
    diagnosisConclusion.getByLabel("Deployment window context references"),
  ).toContainText("chat_session:401/turn:501");
  await expect(diagnosisProgress).toContainText(
    "JVM pressure is visible, but restart and deployment context is still missing.",
  );
  await expect(diagnosisProgress).toContainText("diagnosis-session-302");
  await expect(diagnosisProgress.getByLabel("Missing evidence")).toContainText(
    "Runtime context",
  );
  await expect(diagnosisProgress.getByLabel("Missing evidence")).toContainText(
    "Owner mitigation",
  );
  await expect(
    diagnosisProgress.getByLabel("Collection suggestions"),
  ).toContainText("JVM heap trend");
  await expect(
    diagnosisProgress.getByLabel("Collection suggestions"),
  ).toContainText("Pod restart history");
  await nextEvidenceActions
    .getByRole("link", { name: "Use in diagnosis" })
    .first()
    .click();
  await expect(page).toHaveURL(/\/diagnosis-room\?/);
  const readinessFollowUpURL = new URL(page.url());
  expect(readinessFollowUpURL.pathname).toBe("/diagnosis-room");
  expect(readinessFollowUpURL.searchParams.get("evidence_snapshot_id")).toBe(
    "9003",
  );
  expect(readinessFollowUpURL.searchParams.get("report_id")).toBe("101");
  expect(readinessFollowUpURL.searchParams.get("sub_report_id")).toBe("502");
  expect(readinessFollowUpURL.searchParams.get("session_id")).toBe(
    "diagnosis-session-302",
  );
  expect(readinessFollowUpURL.searchParams.get("intent")).toBe(
    "confidence_review",
  );
  expect(readinessFollowUpURL.searchParams.get("follow_up_label")).toBe(
    "Runtime context",
  );
  expect(readinessFollowUpURL.searchParams.get("follow_up_priority")).toBe(
    "high",
  );
  await expect(page.getByText("Report #101 diagnosis")).toBeVisible();
  await expect(evidenceSnapshotInput(page)).toHaveValue("9003");
  const readinessSupplementalEntry = page.getByLabel(
    "Supplemental evidence entry",
  );
  await expect(readinessSupplementalEntry).toContainText("Runtime context");
  await expect(readinessSupplementalEntry).toContainText(
    "Attach recent restart and deployment details before confirming the diagnosis.",
  );
  await expect(page.getByLabel("Message")).toBeDisabled();
  await expect(page.getByLabel("Message")).toHaveValue(
    /Review evidence snapshot #9003/,
  );
  await page.goto("/reports/101");
  await page
    .locator(".subreport-item")
    .filter({ hasText: "Checkout JVM memory" })
    .getByRole("link", { name: "Resolve evidence" })
    .click();
  await expect(page).toHaveURL(/\/diagnosis-room\?/);
  const progressURL = new URL(page.url());
  expect(progressURL.searchParams.get("evidence_snapshot_id")).toBe("9003");
  expect(progressURL.searchParams.get("report_id")).toBe("101");
  expect(progressURL.searchParams.get("sub_report_id")).toBe("502");
  expect(progressURL.searchParams.get("session_id")).toBe(
    "diagnosis-session-302",
  );
  await page.goto("/reports/101");

  await page
    .getByLabel("Diagnosis conclusion")
    .getByLabel("Missing evidence")
    .getByRole("link", { name: "Use in diagnosis" })
    .click();
  await expect(page).toHaveURL(/\/diagnosis-room\?/);
  const followUpURL = new URL(page.url());
  expect(followUpURL.pathname).toBe("/diagnosis-room");
  expect(followUpURL.searchParams.get("evidence_snapshot_id")).toBe("9001");
  expect(followUpURL.searchParams.get("report_id")).toBe("101");
  expect(followUpURL.searchParams.get("sub_report_id")).toBe("501");
  expect(followUpURL.searchParams.get("session_id")).toBe(
    "diagnosis-session-301",
  );
  expect(followUpURL.searchParams.get("intent")).toBe("confidence_review");
  expect(followUpURL.searchParams.get("follow_up_label")).toBe(
    "Deployment window",
  );
  expect(followUpURL.searchParams.get("follow_up_priority")).toBe("high");
  await expect(page.getByText("Report #101 diagnosis")).toBeVisible();
  await expect(
    page.getByText(
      "Prepared from report #101, subreport #501 using evidence snapshot #9001. Connect to continue diagnosis session diagnosis-session-301.",
    ),
  ).toBeVisible();
  const pendingSupplemental = page.getByLabel("Supplemental evidence entry");
  await expect(pendingSupplemental).toContainText("Supplemental Evidence");
  await expect(pendingSupplemental).toContainText("Deployment window");
  await expect(pendingSupplemental).toContainText(
    "Provide checkout deployment timing before raising confidence.",
  );
  await expect(evidenceSnapshotInput(page)).toHaveValue("9001");
  await expect(page.getByLabel("Message")).toBeDisabled();
  await expect(page.getByLabel("Message")).toHaveValue(
    /CheckoutLatencyHigh/,
  );

  await page.goto("/reports/101");
  await page
    .locator(".subreport-item")
    .filter({ hasText: "Checkout API latency" })
    .getByRole("link", { name: "Confirm diagnosis" })
    .click();
  await expect(page).toHaveURL(
    /\/diagnosis-room\?evidence_snapshot_id=9001&intent=review_conclusion&report_id=101&sub_report_id=501&session_id=diagnosis-session-301$/,
  );
  await expect(page.getByText("Report #101 diagnosis")).toBeVisible();
  await expect(
    page.getByText(
      "Prepared from report #101, subreport #501 using evidence snapshot #9001. Connect to continue diagnosis session diagnosis-session-301.",
    ),
  ).toBeVisible();
  await expect(evidenceSnapshotInput(page)).toHaveValue("9001");
  await expect(page.getByLabel("Message")).toHaveValue(
    /Verify the current diagnosis conclusion/,
  );
  const reportLinkedHandoff = page.getByLabel("Diagnosis handoff");
  await expect(reportLinkedHandoff).toContainText("AI delivery incomplete");
  await expect(reportLinkedHandoff).toContainText("Latest AI conclusion");
  await expect(reportLinkedHandoff).toContainText(
    "Checkout latency remains correlated with the payment deployment.",
  );
  await expect(reportLinkedHandoff).toContainText(
    "Final-ready notification / delivered",
  );
  const reportLinkedRoomState = page.getByLabel("Room state");
  await expect(reportLinkedRoomState).toContainText(
    "Retained final conclusion",
  );
  await expect(reportLinkedRoomState).toContainText(
    "diagnosis-room-final-ready.v1",
  );
  const reportLinkedNotificationTimeline = page.getByLabel(
    "Notification timeline",
  );
  await expect(reportLinkedNotificationTimeline).toContainText(
    "Final-ready notification",
  );
  await expect(reportLinkedNotificationTimeline).toContainText(
    "wecom-report-final-ready-301",
  );
  await expect(
    page.getByRole("link", { name: "Back to report" }),
  ).toHaveAttribute("href", "/reports/101?diagnosis_review=1#diagnosis-readiness");
  await page.getByRole("link", { name: "Back to report" }).click();
  await expect(page).toHaveURL("/reports/101?diagnosis_review=1#diagnosis-readiness");
  await expect(page.getByLabel("Diagnosis review return")).toContainText(
    "Latest report data has been loaded.",
  );
});

test("diagnosis room route connects, submits a turn, and approves the conclusion", async ({
  page,
}) => {
  test.setTimeout(60_000);

  const enableTemplateResponse = await page.request.post(
    "/api/config/diagnosis-tool-templates/1/enable",
  );
  expect(enableTemplateResponse.ok()).toBeTruthy();

  await page.goto("/diagnosis-room");

  await expect(
    page.getByRole("heading", { name: "Diagnosis Room" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Create diagnosis room" }),
  ).toBeVisible();
  await checkConnectionAuth(page);
  const workQueueFilter = page.getByLabel("Diagnosis work queue filter");
  await expect(workQueueFilter).toContainText(/All \d+/);
  await expect(workQueueFilter).toContainText("Needs room 1");
  await expect(workQueueFilter).toContainText("Attention 4");
  await workQueueFilter.getByText("Needs room 1").click();
  const handoffBacklog = page.getByLabel("AI handoff backlog", {
    exact: true,
  });
  await expect(handoffBacklog).toContainText("PaymentErrorRateHigh");
  await expect(handoffBacklog).toContainText("needs room");
  await expect(page.getByLabel("Recent diagnosis rooms")).toHaveCount(0);
  await workQueueFilter.getByText("Attention 4").click();
  await expect(
    page.getByLabel("AI handoff backlog", { exact: true }),
  ).toHaveCount(0);
  const attentionRooms = page.getByLabel("Recent diagnosis rooms");
  await expect(attentionRooms).toContainText("Notification failed");
  await expect(attentionRooms).toContainText("Workflow unavailable");
  await expect(attentionRooms).not.toContainText("Review conclusion");
  await workQueueFilter.getByText(/^All \d+$/).click();
  await page
    .getByRole("button", { name: "Prepare handoff snapshot 9004" })
    .click();
  await expect(evidenceSnapshotInput(page)).toHaveValue("9004");
  await expect(page.getByLabel("Message")).toHaveValue(
    /Start an AI diagnosis for PaymentErrorRateHigh/,
  );
  await expect(
    page.getByText(
      "Prepared handoff snapshot #9004 for diagnosis room creation.",
    ),
  ).toBeVisible();
  const unavailableWorkflowRoom = page
    .locator(".diagnosis-room-list-item")
    .filter({ hasText: "diagnosis-session-orphaned-workflow" });
  await expect(unavailableWorkflowRoom).toContainText("Workflow unavailable");
  await expect(unavailableWorkflowRoom).toContainText("workflow not_found");
  await expect(unavailableWorkflowRoom).toContainText(
    "Temporal reports workflow status not_found.",
  );
  const unavailableUseButton = page.getByRole("button", {
    name: "Use diagnosis-session-orphaned-workflow",
  });
  await expect(unavailableUseButton).toBeDisabled();
  const unavailableAction = page.locator(
    'span[aria-label="Workflow is not_found; inspect or restart it before opening."]',
  );
  await unavailableAction.hover();
  await expect(
    page.getByText(
      "Workflow is not_found; inspect or restart it before opening.",
    ),
  ).toBeVisible();
  const prepareRebuildButton = page.getByRole("button", {
    name: "Prepare rebuild diagnosis-session-orphaned-workflow",
  });
  await expect(prepareRebuildButton).toBeEnabled();
  await prepareRebuildButton.click();
  await expect(evidenceSnapshotInput(page)).toHaveValue("9001");
  await expect(
    page.getByText(
      "Unavailable room closed. Create a replacement room with the prepared evidence snapshot.",
    ),
  ).toBeVisible();
  await expect(
    page.getByText("Prepared replacement room from evidence snapshot #9001."),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Closed unavailable diagnosis room diagnosis-session-orphaned-workflow.",
    ),
  ).toBeVisible();
  await expect(unavailableWorkflowRoom).toContainText("Closed");
  await expect(unavailableWorkflowRoom).toContainText("closed");
  await expect(unavailableWorkflowRoom).toContainText("cancelled");

  const failedNotificationRoom = page
    .locator(".diagnosis-room-list-item")
    .filter({ hasText: "diagnosis-session-notification-failed" });
  await expect(failedNotificationRoom).toContainText("Notification failed");
  await expect(failedNotificationRoom).toContainText("notify failed");
  await expect(
    failedNotificationRoom.getByRole("link", {
      name: "Review notification channel for diagnosis-session-notification-failed",
    }),
  ).toHaveAttribute("href", "/settings/notification-channels?channel_id=2");
  await failedNotificationRoom
    .getByRole("button", {
      name: "Use diagnosis-session-notification-failed",
    })
    .click();
  const failedNotificationHandoff = page.getByLabel("Diagnosis handoff");
  await expect(failedNotificationHandoff).toContainText(
    "Notification delivery failed",
  );
  await expect(failedNotificationHandoff).toContainText(
    "Final-ready notification failed",
  );
  await expect(failedNotificationHandoff).toContainText("Latest notification");
  await expect(failedNotificationHandoff).toContainText(
    "Final-ready notification / failed",
  );
  await expect(
    failedNotificationHandoff.getByRole("link", {
      name: "Review notification channel",
    }),
  ).toHaveAttribute("href", "/settings/notification-channels?channel_id=2");

  const closedRoom = page
    .locator(".diagnosis-room-list-item")
    .filter({ hasText: "diagnosis-session-closed-finalized" });
  await expect(closedRoom).toContainText("AI proof missing");
  await expect(closedRoom).toContainText("closed");
  await expect(closedRoom).toContainText("Close notification");
  const reviewClosedRoomButton = closedRoom.getByRole("button", {
    name: "Review diagnosis-session-closed-finalized",
  });
  await expect(reviewClosedRoomButton).toBeEnabled();
  await reviewClosedRoomButton.click();
  await expect(page).toHaveURL(
    /\/diagnosis-room\?.*session_id=diagnosis-session-closed-finalized/,
  );
  const closedRoomHandoff = page.getByLabel("Diagnosis handoff");
  await expect(closedRoomHandoff).toContainText("AI delivery incomplete");
  await expect(closedRoomHandoff).toContainText("Room status");
  await expect(closedRoomHandoff).toContainText("closed");
  await expect(closedRoomHandoff).toContainText(
    "Close notification / delivered",
  );
  const roomStateCard = page.getByLabel("Room state");
  await expect(roomStateCard).toContainText("Retained final conclusion");
  await expect(roomStateCard).toContainText(
    "diagnosis-session-closed-finalized:2",
  );
  await expect(roomStateCard).toContainText("not required");
  await expect(roomStateCard).toContainText(
    "Checkout latency was confirmed after operator evidence review and the room was closed.",
  );
  await expect(page.getByRole("button", { name: /Connect/ })).toBeDisabled();

  const existingRoom = page
    .locator(".diagnosis-room-list-item")
    .filter({ hasText: "diagnosis-session-auto-p1-s9001" });
  const useExistingRoomButton = existingRoom.getByRole("button", {
    name: "Use diagnosis-session-auto-p1-s9001",
  });
  await expect(useExistingRoomButton).toBeEnabled();
  await useExistingRoomButton.click();
  await expect(page).toHaveURL(
    /\/diagnosis-room\?.*session_id=diagnosis-session-auto-p1-s9001/,
  );
  const selectedRoomURL = new URL(page.url());
  expect(selectedRoomURL.searchParams.get("session_id")).toBe(
    "diagnosis-session-auto-p1-s9001",
  );
  expect(selectedRoomURL.searchParams.get("evidence_snapshot_id")).toBe("9001");
  await expect(page.getByLabel("Session ID")).toHaveValue(
    "diagnosis-session-auto-p1-s9001",
  );
  await expect(evidenceSnapshotInput(page)).toHaveValue("9001");
  await page.reload();
  await expect(page.getByLabel("Alert context")).toContainText(
    "Alert source profile",
  );
  await expect(page.getByLabel("Alert context")).toContainText("#1");
  await expect(
    page.getByRole("button", {
      name: "Selected diagnosis-session-auto-p1-s9001",
    }),
  ).toBeDisabled();
  await expect(
    page.getByRole("heading", { name: "Notification Timeline" }),
  ).toBeVisible();
  const notificationTimeline = page.getByLabel("Notification timeline");
  await expect(notificationTimeline).toContainText("AI update notification");
  await expect(notificationTimeline).toContainText("Final-ready notification");
  await expect(notificationTimeline).toContainText("delivered");
  await page.goto(
    "/diagnosis-room?evidence_snapshot_id=9001&session_id=diagnosis-session-42",
  );
  await expect(page.getByLabel("Session ID")).toHaveValue(
    "diagnosis-session-42",
  );
  await checkConnectionAuth(page);
  await expect(page.getByRole("button", { name: /Connect/ })).toBeEnabled();
  await page.getByRole("button", { name: /Connect/ }).click();

  await expect(
    page.getByRole("status", { name: "Connection status" }),
  ).toHaveText("connected");
  await expect(page.getByText("owner-1", { exact: true }).first()).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Approve Conclusion" }),
  ).toBeDisabled();
  await reviewClosedRoomButton.click();
  await expect(
    page.getByRole("status", { name: "Connection status" }),
  ).toHaveText(/^(closed|idle)$/);
  await expect(page.getByText("owner-1", { exact: true }).first()).toBeHidden();
  await expect(roomStateCard).toContainText("Retained final conclusion");
  await expect(roomStateCard).toContainText(
    "diagnosis-session-closed-finalized:2",
  );
  await expect(page.getByRole("button", { name: /Connect/ })).toBeDisabled();

  await page.goto(
    "/diagnosis-room?evidence_snapshot_id=9001&session_id=diagnosis-session-42",
  );
  await expect(page.getByLabel("Session ID")).toHaveValue(
    "diagnosis-session-42",
  );
  await checkConnectionAuth(page);
  await expect(page.getByRole("button", { name: /Connect/ })).toBeEnabled();
  await page.getByRole("button", { name: /Connect/ }).click();
  await expect(
    page.getByRole("status", { name: "Connection status" }),
  ).toHaveText("connected");
  await expect(page.getByText("owner-1", { exact: true }).first()).toBeVisible();

  await page
    .getByLabel("Connection controls")
    .getByRole("button", { name: /Disconnect$/ })
    .click();
  await expect(
    page.getByRole("status", { name: "Connection status" }),
  ).toHaveText(/^(closed|idle)$/);
  await reviewClosedRoomButton.click();
  await expect(page.getByLabel("Session ID")).toHaveValue(
    "diagnosis-session-closed-finalized",
  );
  await expect(roomStateCard).toContainText("Retained final conclusion");
  await expect(roomStateCard).toContainText(
    "diagnosis-session-closed-finalized:2",
  );
  await expect(page.getByRole("button", { name: /Connect/ })).toBeDisabled();

  await page.goto(
    "/diagnosis-room?evidence_snapshot_id=9001&session_id=diagnosis-session-42",
  );
  await expect(page.getByLabel("Session ID")).toHaveValue(
    "diagnosis-session-42",
  );
  await checkConnectionAuth(page);
  await expect(page.getByRole("button", { name: /Connect/ })).toBeEnabled();
  await page.getByRole("button", { name: /Connect/ }).click();
  await expect(
    page.getByRole("status", { name: "Connection status" }),
  ).toHaveText("connected");
  await expect(page.getByText("owner-1", { exact: true }).first()).toBeVisible();

  await page.getByLabel("Message").fill("Trigger backend error.");
  const sendButton = page.getByRole("button", { name: "Send" });
  await sendButton.click();
  await expect(
    page.getByText("Diagnosis request failed: mock_backend_error"),
  ).toBeVisible();
  const diagnosisServerError = page.locator(".diagnosis-server-error");
  await expect(
    diagnosisServerError.getByText(
      "mock backend rejected the diagnosis request",
    ),
  ).toBeVisible();
  await expect(sendButton).toBeEnabled();

  await page.getByLabel("Message").fill("Trigger confirm rejection.");
  await sendButton.click();
  await expect(
    page.getByText("Conclusion cannot be confirmed yet"),
  ).toBeVisible();
  await expect(diagnosisServerError).toContainText(
    "resolve missing evidence requests before confirming",
  );
  await expect(diagnosisServerError).toContainText(
    "Open the review queue, add the requested operator evidence, ask AI to reassess submitted evidence when needed, then retry confirmation.",
  );
  await expect(diagnosisServerError).toContainText("Review evidence tasks");
  await expect(sendButton).toBeEnabled();

  await page.getByLabel("Message").fill("Trigger retained state error.");
  await sendButton.click();
  await expect(
    page.getByRole("status", { name: "Connection status" }),
  ).toHaveText("connected");
  await expect(
    page.getByText("Diagnosis request failed: llm_timeout"),
  ).toBeVisible();
  await expect(diagnosisServerError).toContainText(
    "upstream LLM request timed out",
  );
  await expect(diagnosisServerError).toContainText(
    "Query the latest room state",
  );
  await expect(diagnosisServerError).toContainText(
    "retry the operator message",
  );
  await expect(page.getByLabel("Message")).toBeEnabled();
  await expect(sendButton).toBeEnabled();

  await page
    .getByLabel("Message")
    .fill("Summarize the current checkout alert.");
  await sendButton.click();
  await expect(
    page.getByText("Diagnosis request failed: mock_backend_error"),
  ).toBeHidden();
  await expect(
    page.getByText("Conclusion cannot be confirmed yet"),
  ).toBeHidden();
  await expect(
    page.getByText("Diagnosis request failed: llm_timeout"),
  ).toBeHidden();

  await expect(
    page.getByText("Summarize the current checkout alert.", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Mock diagnosis response for: Summarize the current checkout alert.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    page.getByText("AI review in progress", { exact: true }),
  ).toBeHidden();
  await expect(sendButton).toBeEnabled();
  await expect(
    page.getByText("Consultation Insight", { exact: true }),
  ).toBeVisible();
  const consultationProgress = page.locator(
    '[aria-label="Diagnosis consultation progress"]',
  );
  await expect(consultationProgress).toContainText("Confidence");
  await expect(consultationProgress).toContainText(
    "Supplemental evidence requested",
  );
  await expect(consultationProgress).toContainText("Collect missing evidence");
  const confidenceTimeline = page.getByLabel("Confidence timeline");
  await expect(confidenceTimeline).toContainText("Confidence Timeline");
  await expect(confidenceTimeline).toContainText("medium confidence");
  await expect(confidenceTimeline).toContainText("needs_evidence");
  await expect(page.locator('[aria-label="Evidence readiness"]')).toContainText(
    "Collected",
  );
  await expect(
    page.getByText("needs_evidence", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("Executable Evidence Plan", { exact: true }),
  ).toBeVisible();
  const executableEvidencePlan = page.locator(
    'section[aria-label="Executable Evidence Plan"]',
  );
  await expect(
    executableEvidencePlan.getByText("Current active alerts", { exact: true }),
  ).toBeVisible();
  await expect(executableEvidencePlan).toContainText("template #5");
  await expect(executableEvidencePlan).toContainText("profile #3");
  await expect(executableEvidencePlan).toContainText("limit: 7");
  const collectionResults = page.locator(
    'section[aria-label="Collection Results"]',
  );
  await expect(
    collectionResults.getByText("Collection Results", { exact: true }),
  ).toBeVisible();
  await expect(
    collectionResults.getByText("Active alert collection succeeded.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(collectionResults).toContainText("template #5");
  await expect(collectionResults).toContainText("profile #3");
  await expect(collectionResults).toContainText("source: alertmanager");
  await expect(collectionResults).toContainText("limit: 7");
  await expect(page.getByText("CPUHigh / prod")).toBeVisible();
  const evidenceTimeline = page.getByLabel("Evidence timeline");
  await expect(evidenceTimeline).toContainText("Evidence Timeline");
  await expect(evidenceTimeline).toContainText("Turn 1 evidence");
  await expect(evidenceTimeline).toContainText("operator_turn");
  await expect(evidenceTimeline).toContainText("active_alerts");
  await expect(evidenceTimeline).toContainText("template #5");
  await expect(evidenceTimeline).toContainText("profile #3");
  await expect(evidenceTimeline).toContainText("limit: 7");
  await expect(evidenceTimeline).toContainText("collected");
  await expect(evidenceTimeline).toContainText("ok");
  await expect(
    page.getByText("Restart cause", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("Metric window", { exact: true }).first(),
  ).toBeVisible();
  const reviewQueue = page.getByLabel("Diagnosis review queue");
  await expect(reviewQueue).toContainText("Review Queue");
  await expect(reviewQueue).toContainText(
    "Collect evidence: metric_range_query",
  );
  await expect(reviewQueue).toContainText("Restart cause");
  await expect(reviewQueue).toContainText("Collected 1 alert.");
  const operatorEvidence = page.getByLabel("Operator evidence collection");
  const recommendedEvidence = operatorEvidence.getByLabel(
    "Recommended operator evidence",
  );
  await expect(recommendedEvidence).toContainText("CPU saturation range");
  await expect(recommendedEvidence).toContainText("Uses template #1");
  await expect(recommendedEvidence).toContainText("matches alert source");
  await expect(recommendedEvidence).toContainText("source match");
  await recommendedEvidence
    .getByRole("button", { name: "Use recommendation CPU saturation range" })
    .click();
  await expect(operatorEvidence).toContainText(
    "Template #1: CPU saturation range",
  );
  await expect(operatorEvidence.getByLabel("Reason")).toHaveValue(
    "Collect CPU saturation range.",
  );
  await expect(operatorEvidence.getByLabel("Query")).toHaveValue(
    "rate(container_cpu_usage_seconds_total[5m])",
  );
  await expect(operatorEvidence.getByLabel("Window seconds")).toHaveValue(
    "3600",
  );
  await expect(operatorEvidence.getByLabel("Step seconds")).toHaveValue("60");
  await expect(operatorEvidence.getByLabel("Limit")).toHaveValue("5");
  const disableTemplateResponse = await page.request.post(
    "/api/config/diagnosis-tool-templates/1/disable",
  );
  expect(disableTemplateResponse.ok()).toBeTruthy();
  await reviewQueue
    .getByRole("button", {
      name: "Use collection plan for metric_range_query",
    })
    .click();
  await expect(
    page.getByText("Collecting planned evidence for metric_range_query."),
  ).toBeVisible();
  await expect(page.getByText("Collecting planned evidence.")).toBeVisible();
  await expect(
    page.getByText(
      "Mock diagnosis response after planned evidence collection.",
    ),
  ).toBeVisible();
  await expect(evidenceTimeline).toContainText("manual_evidence_collection");
  await expect(evidenceTimeline).toContainText("metric_range_query");
  await expect(evidenceTimeline).toContainText("collected");
  await expect(evidenceTimeline).toContainText("ok");
  await refreshConnectionState(page);
  await expect(
    page.getByText("Loaded state: open, 2 turn(s).").first(),
  ).toBeVisible();
  await expect(reviewQueue).toContainText("Restart cause");
  await expect(reviewQueue).toContainText("Collected 2 metric series.");
  await reviewQueue
    .getByRole("button", { name: "Use follow-up for Restart cause" })
    .click();
  const supplementalEntry = page.locator(
    '[aria-label="Supplemental evidence entry"]',
  );
  await expect(supplementalEntry).toContainText("Supplemental Evidence");
  await expect(supplementalEntry).toContainText(
    "Collect previous container logs",
  );
  await expect(
    page
      .getByText("Prepared supplemental evidence follow-up for Restart cause.")
      .first(),
  ).toBeVisible();
  await expect(page.getByText("Turn 1 completed.")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Approve Conclusion" }),
  ).toBeDisabled();

  await supplementalEntry
    .getByLabel("Evidence", { exact: true })
    .fill(
      "Previous pod logs show a rolling restart after image pull backoff recovery.",
    );
  await page
    .getByRole("button", { name: "Submit supplemental evidence" })
    .click();
  await expect(
    page.getByText("Submitted supplemental evidence for Restart cause."),
  ).toBeVisible();
  await expect(
    page
      .getByText(
        "Previous pod logs show a rolling restart after image pull backoff recovery.",
        { exact: true },
      )
      .first(),
  ).toBeVisible();
  await expect(
    page.getByText("Mock supplemental evidence response for: Restart cause", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.getByText("Turn 3 completed.")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Approve Conclusion" }),
  ).toBeEnabled();

  await expect(
    page.getByText("Loaded state: open, 3 turn(s).").first(),
  ).toBeVisible();
  await expect(
    page.getByText("ready_for_review", { exact: true }).first(),
  ).toBeVisible();
  await expect(consultationProgress).toContainText(
    "Conclusion ready for review",
  );
  await expect(consultationProgress).toContainText("Owner confirmation");
  await expect(confidenceTimeline).toContainText("high confidence");
  await expect(confidenceTimeline).toContainText("ready_for_review");
  const supplementalHistory = page.getByLabel("Supplemental evidence history");
  await expect(supplementalHistory).toContainText(
    "Supplemental Evidence History",
  );
  await expect(supplementalHistory).toContainText("Restart cause");
  await expect(supplementalHistory).toContainText(
    "Previous pod logs show a rolling restart",
  );
  await expect(
    page.getByText("Owner confirmation", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Approve Conclusion" }),
  ).toBeEnabled();

  await page.getByRole("button", { name: "Approve Conclusion" }).click();
  await expect(page.getByText("Recording conclusion approval.")).toBeVisible();
  await expect(
    page.getByText("Loaded state: closed, 3 turn(s)."),
  ).toBeVisible();
  await expect(
    page.getByText("Final conclusion", { exact: true }),
  ).toBeVisible();
  const finalConclusion = page.locator(".diagnosis-conclusion");
  await expect(finalConclusion).toContainText(
    "Mock supplemental evidence response for: Restart cause",
  );
  await expect(
    roomStateCard.getByRole("row", {
      name: /Close reason.*human_confirmed/,
    }),
  ).toBeVisible();
  const conclusionApproval = page.getByLabel("Conclusion approval");
  await expect(conclusionApproval).toContainText("Quorum satisfied");
  await expect(conclusionApproval).toContainText("owner-1");
  await expect(
    conclusionApproval.getByText("human_confirmed", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("diagnosis-room-close.v1", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Confirmed by", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Approve Conclusion" }),
  ).toBeDisabled();
});

test("diagnosis room reopens latest request when supplemental evidence remains unresolved", async ({
  page,
}) => {
  test.setTimeout(60_000);

  await page.goto("/diagnosis-room");

  await page.getByLabel("Session ID").fill("diagnosis-session-unresolved");
  await evidenceSnapshotInput(page).fill("9001");
  await checkConnectionAuth(page);
  await page.getByRole("button", { name: /Connect/ }).click();
  await expect(
    page.getByRole("status", { name: "Connection status" }),
  ).toHaveText("connected");

  await page
    .getByLabel("Message")
    .fill("Review restart evidence completeness.");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(
    page.getByText(
      "Mock diagnosis response for: Review restart evidence completeness.",
      { exact: true },
    ),
  ).toBeVisible();

  const reviewQueue = page.getByLabel("Diagnosis review queue");
  await expect(reviewQueue).toContainText("Missing evidence: Restart cause");
  const reviewActionPlan = page.getByLabel("Review queue action plan");
  await expect(reviewActionPlan).toContainText("Confirmation action plan");
  await expect(reviewActionPlan).toContainText(
    "Resolve missing evidence requests before confirming.",
  );
  await expect(reviewActionPlan).toContainText(
    "Missing evidence: Restart cause",
  );
  await reviewQueue
    .getByRole("button", { name: "Use follow-up for Restart cause" })
    .click();

  const supplementalEntry = page.locator(
    '[aria-label="Supplemental evidence entry"]',
  );
  await supplementalEntry
    .getByLabel("Evidence", { exact: true })
    .fill(
      "Still insufficient: previous logs confirm a restart but do not include the termination timestamp.",
    );
  await page
    .getByRole("button", { name: "Submit supplemental evidence" })
    .click();
  await expect(
    page.getByText("Submitted supplemental evidence for Restart cause."),
  ).toBeVisible();
  await expect(reviewQueue).toContainText("Supplemental evidence still blocking");
  await expect(reviewQueue).toContainText("Submitted evidence: Restart cause");
  await expect(
    reviewQueue.getByRole("heading", {
      name: /Missing evidence: Restart cause/,
    }),
  ).toBeVisible();
  await expect(
    reviewQueue.getByRole("heading", {
      name: /Submitted evidence: Restart cause/,
    }),
  ).toBeVisible();
  await expect(reviewQueue).toContainText(
    "Attach previous container logs with restart timestamps and the observed termination reason.",
  );
  await expect(
    page.getByRole("button", { name: "Approve Conclusion" }),
  ).toBeDisabled();
  const confirmBlockReason = page.locator(".diagnosis-confirm-block-reason");
  await expect(confirmBlockReason).toContainText("Confirmation blocked");
  await expect(confirmBlockReason).toContainText(
    "Resolve missing evidence requests before confirming.",
  );
  await expect(reviewActionPlan).toContainText(
    "Resolve missing evidence requests before confirming.",
  );
  await expect(reviewActionPlan).toContainText(
    "Missing evidence: Restart cause",
  );
  const supplementalHistory = page.getByLabel("Supplemental evidence history");
  await expect(supplementalHistory).toContainText(
    "Supplemental Evidence History",
  );
  await expect(supplementalHistory).toContainText("latest request");
  await expect(supplementalHistory).toContainText(
    "Latest request: Attach previous container logs with restart timestamps and the observed termination reason.",
  );

  await reviewQueue
    .getByRole("button", { name: "Use follow-up for Restart cause" })
    .click();
  await expect(supplementalEntry).toContainText(
    "Attach previous container logs with restart timestamps and the observed termination reason.",
  );
});

test("diagnosis room keeps collected evidence visible after auto follow-up", async ({
  page,
}) => {
  test.setTimeout(60_000);

  await page.goto("/diagnosis-room");

  await page.getByLabel("Session ID").fill("diagnosis-session-auto-follow-up");
  await evidenceSnapshotInput(page).fill("9001");
  await checkConnectionAuth(page);
  await page.getByRole("button", { name: /Connect/ }).click();
  await expect(
    page.getByRole("status", { name: "Connection status" }),
  ).toHaveText("connected");

  await page.getByLabel("Message").fill("Trigger auto evidence fallback.");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(
    page.getByText("Mock auto evidence follow-up response.", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Turn 2 completed.", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Auto evidence follow-up completed 1 turn(s).", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.locator('[aria-label="Evidence readiness"]')).toContainText(
    "Plan",
  );
  await expect(page.locator('[aria-label="Evidence readiness"]')).toContainText(
    "Collected",
  );
  await expect(
    page.getByText("Executable Evidence Plan", { exact: true }),
  ).toBeVisible();
  const autoExecutableEvidencePlan = page.locator(
    'section[aria-label="Executable Evidence Plan"]',
  );
  await expect(
    autoExecutableEvidencePlan.getByText("Current active alerts", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(autoExecutableEvidencePlan).toContainText("template #5");
  await expect(autoExecutableEvidencePlan).toContainText("profile #3");
  await expect(autoExecutableEvidencePlan).toContainText("limit: 7");
  const autoCollectionResults = page.locator(
    'section[aria-label="Collection Results"]',
  );
  await expect(
    autoCollectionResults.getByText("Collection Results", { exact: true }),
  ).toBeVisible();
  await expect(
    autoCollectionResults.getByText("Active alert collection succeeded.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(autoCollectionResults).toContainText("template #5");
  await expect(autoCollectionResults).toContainText("profile #3");
  await expect(autoCollectionResults).toContainText("source: alertmanager");
  await expect(autoCollectionResults).toContainText("limit: 7");
  await expect(page.getByText("CPUHigh / prod")).toBeVisible();
  const autoEvidenceTimeline = page.getByLabel("Evidence timeline");
  await expect(autoEvidenceTimeline).toContainText("Evidence Timeline");
  await expect(autoEvidenceTimeline).toContainText("Turn 1 evidence");
  await expect(autoEvidenceTimeline).toContainText("operator_turn");
  await expect(autoEvidenceTimeline).toContainText("active_alerts");
  await expect(autoEvidenceTimeline).toContainText("template #5");
  await expect(autoEvidenceTimeline).toContainText("profile #3");
  await expect(autoEvidenceTimeline).toContainText("limit: 7");
  await expect(autoEvidenceTimeline).toContainText("collected");
  await expect(autoEvidenceTimeline).toContainText("ok");

  await refreshConnectionState(page);
  await expect(
    page.getByText("Loaded state: open, 2 turn(s).").first(),
  ).toBeVisible();
  await expect(
    autoExecutableEvidencePlan.getByText("Current active alerts", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    autoCollectionResults.getByText("Active alert collection succeeded.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(autoEvidenceTimeline).toContainText("Turn 1 evidence");
});

test("settings overview route renders the alert operations configuration graph", async ({
  page,
}) => {
  await page.goto("/settings");

  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
  await expect(
    page.getByLabel("Alert operations configuration sequence"),
  ).toContainText("Source");
  await expect(page.getByLabel("Settings surfaces")).toContainText(
    "Alert sources",
  );
  await expect(page.getByLabel("Settings surfaces")).toContainText(
    "Diagnosis tools",
  );
  await expect(page.getByLabel("Settings surfaces")).toContainText(
    "Workflow policies",
  );
  await expect(page.getByLabel("Next setup stage")).toContainText(
    "Use auto room for webhook handoff",
  );
  await expect(page.getByLabel("Next setup stage")).toContainText(
    "Suggest room keeps an operator handoff step",
  );
  await expect(page.getByLabel("Next setup stage")).toContainText(
    "medium priority",
  );
  await expect(page.getByLabel("Retained proof targets")).toContainText(
    "Policy replay",
  );
  await expect(page.getByLabel("Retained proof targets")).toContainText(
    "Scheduled trigger",
  );
  await expect(page.getByLabel("Retained proof targets")).toContainText(
    "Review workflow warnings",
  );
  await expect(page.getByLabel("Retained proof targets")).toContainText(
    "The selected schedule is still a draft",
  );
  await expect(
    page.getByLabel("Retained proof targets").getByRole("link", {
      name: "Review Replay",
    }),
  ).toHaveAttribute("href", "/settings/report-workflow-policies");
  await expect(
    page.getByLabel("Retained proof targets").getByRole("link", {
      name: "Enable Schedule",
    }),
  ).toHaveAttribute("href", "/settings/report-workflow-schedules");
  await expect(page.getByLabel("Retained proof targets")).toContainText(
    "Configuration present",
  );
  await expect(page.getByLabel("Active workflow topology")).toContainText(
    "Default report workflow",
  );
  await expect(page.getByLabel("Active workflow topology")).toContainText(
    "Primary Prometheus",
  );
  await expect(page.getByLabel("Active workflow topology")).toContainText(
    "review",
  );
  await expect(
    page.getByLabel("Selected alert workflow topology"),
  ).toContainText("AI room");
  const ingestionReadiness = page.getByLabel("Alert ingestion readiness");
  await expect(ingestionReadiness).toContainText("Alert ingestion readiness");
  await expect(ingestionReadiness).toContainText("review");
  await expect(ingestionReadiness).toContainText("prometheus");
  await expect(ingestionReadiness).toContainText("suggest_room");
  await expect(ingestionReadiness).toContainText("Active alert tools");
  await expect(ingestionReadiness).toContainText("Metric evidence");
  await expect(
    page.getByLabel("Alert to AI room configuration path"),
  ).toContainText("AI room");
  await expect(page.getByLabel("Next topology actions")).toContainText(
    "Use auto room for webhook handoff",
  );
  await expect(page.getByLabel("Next topology actions")).toContainText(
    "Enable active alert tool",
  );
  await expect(page.getByLabel("Next topology actions")).toContainText(
    "Run impact preview",
  );
  const autoRoomAction = page
    .getByLabel("Next topology actions")
    .locator(".settings-overview-action-item")
    .filter({ hasText: "Use auto room for webhook handoff" });
  await expect(
    autoRoomAction.getByRole("link", { name: "Open" }),
  ).toHaveAttribute(
    "href",
    /\/settings\/report-workflow-policies\?intent=auto-room-follow-up&source_id=\d+/,
  );
  const activeToolAction = page
    .getByLabel("Next topology actions")
    .locator(".settings-overview-action-item")
    .filter({ hasText: "Enable active alert tool" });
  await expect(
    activeToolAction.getByRole("link", { name: "Open" }),
  ).toHaveAttribute(
    "href",
    /\/settings\/diagnosis-tool-templates\?intent=active-alert-tool&source_id=\d+/,
  );
  await expect(page.getByLabel("Settings surfaces")).toContainText("Ready");
  await expect(page.getByText("Live proof gate")).toBeVisible();
  await expect(page.getByText(/configuration objects/)).toBeVisible();
});

test("alert source settings route lists and creates profiles", async ({
  page,
}) => {
  await page.goto("/settings/alert-sources");

  await expect(
    page.getByRole("heading", { name: "Alert Sources" }),
  ).toBeVisible();
  await expect(page.getByText("Primary Prometheus")).toBeVisible();
  const sourceReadiness = page.getByLabel("Alert source readiness preview");
  await expect(sourceReadiness).toContainText("Complete source configuration.");
  await expect(sourceReadiness).toContainText("Profile name is required.");
  await expect(sourceReadiness).toContainText("Workflow binding");
  const stagingAlertmanagerRow = page.getByRole("row", {
    name: /Staging Alertmanager/,
  });
  await expect(stagingAlertmanagerRow).toContainText(
    "/api/v1/alert-sources/2/webhooks/alertmanager",
  );
  const stagingAlertmanagerTestButton = stagingAlertmanagerRow.getByRole(
    "button",
    { name: "Test" },
  );
  await expect(stagingAlertmanagerTestButton).toBeEnabled();
  await stagingAlertmanagerTestButton.click();
  await expect(page.getByRole("status")).toContainText(
    "Alertmanager alert listing succeeded.",
  );
  await expect(stagingAlertmanagerRow).toContainText("ok");
  await expect(stagingAlertmanagerRow).toContainText(
    "Alertmanager alert listing succeeded.",
  );
  await expect(stagingAlertmanagerRow).toContainText("Jun 5, 2026, 4:00 AM");
  await expect(stagingAlertmanagerRow).toContainText("1 firing alert");

  const primaryPrometheusRow = page.getByRole("row", {
    name: /Primary Prometheus/,
  });
  const primaryPrometheusTestButton = primaryPrometheusRow.getByRole("button", {
    name: "Test",
  });
  await expect(primaryPrometheusTestButton).toBeEnabled();
  await primaryPrometheusTestButton.click();
  await expect(page.getByRole("status")).toContainText(
    "Secret-backed connection tests require a server-side secret resolver.",
  );
  await expect(primaryPrometheusRow).toContainText("credentials_unavailable");
  await expect(primaryPrometheusRow).toContainText(
    "Secret-backed connection tests require a server-side secret resolver.",
  );
  await expect(primaryPrometheusRow).toContainText("Jun 5, 2026, 4:00 AM");

  await page.goto("/settings/alert-sources?intent=alertmanager-source");
  await expect(page.getByLabel("Alert source launch preset")).toContainText(
    "Prepared an enabled Alertmanager source.",
  );
  await expect(page.getByLabel("Name")).toHaveValue(
    "Alertmanager alert intake",
  );
  await expect(page.getByLabel("Enabled")).toBeChecked();
  await expect(page.getByLabel("Labels")).toHaveValue(
    "role=alert-intake\nsource=alertmanager",
  );
  await expect(page.getByLabel("Alert source readiness preview")).toContainText(
    "Base URL is required.",
  );
  await expect(page.getByLabel("Alert source readiness preview")).toContainText(
    "Alertmanager webhook ingest",
  );

  await page.goto("/settings/alert-sources");
  const settingsForm = page.locator("form");
  await page.getByLabel("Name").fill("Team Alertmanager");
  await settingsForm
    .locator(".ant-form-item")
    .filter({ hasText: /^Kind/ })
    .locator(".ant-segmented-item")
    .filter({ hasText: /^Alertmanager$/ })
    .click();
  await settingsForm
    .locator(".ant-segmented-item")
    .filter({ hasText: /^Bearer$/ })
    .click();
  await page
    .getByLabel("Base URL")
    .fill("https://alertmanager-team.example.test/api/v2/alerts");
  await expect(
    page.getByText("Connection targets", { exact: true }),
  ).toBeVisible();
  await expect(page.locator("#baseURL_extra")).toContainText(
    "https://alertmanager-team.example.test/api/v2/alerts?active=true&inhibited=false&silenced=false&unprocessed=false",
  );
  await expect(sourceReadiness).toContainText("Complete source configuration.");
  await page
    .getByLabel("Secret reference")
    .fill("secret/openclarion/alertmanager-bearer");
  await page.getByLabel("Labels").fill("env=prod\nowner=sre");
  await page.getByLabel("Enabled").check();
  await expect(sourceReadiness).toContainText("Source ready for workflows.");
  await expect(sourceReadiness).toContainText(
    "Alertmanager reads active alerts with silenced, inhibited, and unprocessed alerts filtered out.",
  );
  await expect(sourceReadiness).toContainText("Active alert pull");
  await expect(sourceReadiness).toContainText("Webhook intake");
  await expect(sourceReadiness).toContainText("Workflow binding");
  await expect(sourceReadiness).toContainText("Alertmanager webhook ingest");
  await expect(sourceReadiness).toContainText(
    "Pull active alerts and accept Alertmanager webhooks",
  );
  await page.getByRole("button", { name: "Save Profile" }).click();

  await expect(page.getByRole("status")).toContainText("Profile saved.");
  const teamAlertmanagerRow = page.getByRole("row", {
    name: /Team Alertmanager/,
  });
  await expect(teamAlertmanagerRow).toContainText(
    "https://alertmanager-team.example.test",
  );
  await expect(
    teamAlertmanagerRow.getByText("secret/openclarion/alertmanager-bearer", {
      exact: true,
    }),
  ).toBeVisible();
});

test("grouping policy settings route previews and creates policies", async ({
  page,
}) => {
  await page.goto("/settings/grouping-policies");

  await expect(
    page.getByRole("heading", { name: "Grouping Policies" }),
  ).toBeVisible();
  await expect(page.getByText("Default alert grouping")).toBeVisible();

  const defaultPolicyRow = page.getByRole("row", {
    name: /Default alert grouping/,
  });
  await defaultPolicyRow.getByRole("button", { name: "Preview" }).click();
  await expect(page.getByRole("status")).toContainText(
    "Preview scanned 3 events and matched 2.",
  );
  await expect(defaultPolicyRow).toContainText("1 group");
  await expect(page.getByText("HighCPU")).toBeVisible();
  await expect(page.getByText("101, 102")).toBeVisible();

  const settingsForm = page.locator("form");
  await page.goto("/settings/grouping-policies?intent=default-alert-grouping");
  await expect(page.getByLabel("Grouping policy launch preset")).toContainText(
    "Prepared a default alert grouping policy",
  );
  await expect(page.getByLabel("Name")).toHaveValue("Default alert grouping");
  await expect(page.getByLabel("Dimension keys")).toHaveValue(
    "alertname\nservice\nnamespace\npod",
  );
  await expect(page.getByLabel("Severity key")).toHaveValue("severity");
  await expect(page.getByLabel("Source filter")).toHaveValue("");
  await expect(page.getByLabel("Enabled")).toBeChecked();

  await page.goto("/settings/grouping-policies");
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

test("report workflow policy settings route creates and toggles policies", async ({
  page,
}) => {
  const readyAlertmanagerResponse = await page.request.post(
    "/api/config/alert-sources",
    {
      data: {
        auth_mode: "none",
        base_url: "https://alertmanager-ready.example.test",
        enabled: true,
        kind: "alertmanager",
        labels: { env: "prod" },
        name: "Ready Alertmanager",
        secret_ref: "",
      },
    },
  );
  expect(readyAlertmanagerResponse.ok()).toBeTruthy();
  const readyAlertmanager = await readyAlertmanagerResponse.json();
  const activeAlertsResponse = await page.request.post(
    "/api/config/diagnosis-tool-templates",
    {
      data: {
        alert_source_profile_id: readyAlertmanager.id,
        default_limit: 5,
        default_step_seconds: 0,
        default_window_seconds: 0,
        max_window_seconds: 0,
        name: "Ready active alerts",
        query_template: "",
        tool: "active_alerts",
      },
    },
  );
  expect(activeAlertsResponse.ok()).toBeTruthy();
  const activeAlerts = await activeAlertsResponse.json();
  const activeAlertsEnableResponse = await page.request.post(
    `/api/config/diagnosis-tool-templates/${activeAlerts.id}/enable`,
  );
  expect(activeAlertsEnableResponse.ok()).toBeTruthy();

  const metricToolResponse = await page.request.post(
    "/api/config/diagnosis-tool-templates",
    {
      data: {
        alert_source_profile_id: 1,
        default_limit: 5,
        default_step_seconds: 60,
        default_window_seconds: 3600,
        max_window_seconds: 21600,
        name: "Ready metric range",
        query_template: "rate(container_cpu_usage_seconds_total[5m])",
        tool: "metric_range_query",
      },
    },
  );
  expect(metricToolResponse.ok()).toBeTruthy();
  const metricTool = await metricToolResponse.json();
  const metricToolEnableResponse = await page.request.post(
    `/api/config/diagnosis-tool-templates/${metricTool.id}/enable`,
  );
  expect(metricToolEnableResponse.ok()).toBeTruthy();

  const readyChannelResponse = await page.request.post(
    "/api/config/notification-channels",
    {
      data: {
        delivery_scopes: [
          "report",
          "diagnosis_consultation",
          "diagnosis_close",
        ],
        enabled: true,
        kind: "wecom",
        labels: { team: "ops" },
        latest_test_results: [
          {
            checked_at: "2026-06-05T10:05:00Z",
            content_kind: "ai_diagnosis_sample",
            content_sha256:
              "5c6ffbdd40d9556b73a21e63c3e0e9047c7f534c2ab09dc7ed89b889f0d011e7",
            message: "AI diagnosis sample delivery succeeded.",
            provider_message_id: "msg-ai-proof",
            provider_status: "accepted",
            reason_code: "ok",
            status: "success",
          },
          {
            checked_at: "2026-06-05T10:06:00Z",
            content_kind: "diagnosis_close_sample",
            content_sha256:
              "7ab91a0fdbe811d97fef04a77ff1f5f49f7f1e2f94f51f9789f5c7897fbb6b9a",
            message: "Diagnosis close sample delivery succeeded.",
            provider_message_id: "msg-close-proof",
            provider_status: "accepted",
            reason_code: "ok",
            status: "success",
          },
        ],
        name: "Operations close WeCom",
        secret_ref: "secret/example/ops-close-webhook",
      },
    },
  );
  expect(readyChannelResponse.ok()).toBeTruthy();
  const readyChannel = await readyChannelResponse.json();
  const readyPolicyResponse = await page.request.post(
    "/api/config/report-workflow-policies",
    {
      data: {
        alert_source_profile_id: readyAlertmanager.id,
        diagnosis_follow_up: "auto_room",
        grouping_policy_id: 1,
        name: "Ready automatic diagnosis workflow",
        report_notification_channel_profile_id: readyChannel.id,
        report_scenario: "single_alert",
        trigger_mode: "manual_replay",
      },
    },
  );
  expect(readyPolicyResponse.ok()).toBeTruthy();
  const readyPolicy = await readyPolicyResponse.json();

  const autoPolicyResponse = await page.request.post(
    "/api/config/report-workflow-policies",
    {
      data: {
        alert_source_profile_id: 1,
        diagnosis_follow_up: "auto_room",
        grouping_policy_id: 1,
        name: "Automatic diagnosis workflow",
        report_notification_channel_profile_id: null,
        report_scenario: "single_alert",
        trigger_mode: "manual_replay",
      },
    },
  );
  expect(autoPolicyResponse.ok()).toBeTruthy();
  const blockedDeliveryPolicyResponse = await page.request.post(
    "/api/config/report-workflow-policies",
    {
      data: {
        alert_source_profile_id: 1,
        diagnosis_follow_up: "disabled",
        grouping_policy_id: 1,
        name: "Blocked delivery workflow",
        report_notification_channel_profile_id: 1,
        report_scenario: "single_alert",
        trigger_mode: "manual_replay",
      },
    },
  );
  expect(blockedDeliveryPolicyResponse.ok()).toBeTruthy();

  await page.goto("/settings/report-workflow-policies");

  await expect(
    page.getByRole("heading", { name: "Workflow Policies" }),
  ).toBeVisible();
  await expect(
    page.getByRole("row", { name: /Default report workflow/ }),
  ).toBeVisible();
  await expect(
    page.getByLabel("AI consultation workflow readiness"),
  ).toContainText("Default report workflow");
  await expect(
    page.getByLabel("AI consultation workflow readiness"),
  ).toContainText("AI room");
  await expect(
    page.getByLabel("AI consultation workflow counters"),
  ).toContainText("Room-ready policies");
  const automaticDiagnosisRow = page
    .getByRole("row", { name: /Automatic diagnosis workflow/ })
    .first();
  await expect(automaticDiagnosisRow).toContainText("auto_room");
  const readyAutomaticRow = page
    .getByRole("row", { name: /Ready automatic diagnosis workflow/ })
    .first();
  await expect(readyAutomaticRow).toContainText("Ready Alertmanager");
  await expect(readyAutomaticRow).toContainText("Operations close WeCom");
  await expect(readyAutomaticRow).toContainText("auto_room");
  await readyAutomaticRow.getByRole("button", { name: "Impact" }).click();
  await expect(page.getByRole("status")).toContainText("Impact preview ready");
  const readyImpactDialog = page.getByRole("dialog", {
    name: /Impact Preview/,
  });
  await expect(readyImpactDialog).toContainText("Preview ready");
  await expect(readyImpactDialog).toContainText("HighCPU");
  await expect(readyImpactDialog).toContainText("scopes and proof ready");
  await readyImpactDialog
    .locator("button")
    .filter({ hasText: "Close" })
    .click();
  await expect(readyImpactDialog).toBeHidden();
  await readyAutomaticRow.getByRole("button", { name: "Enable" }).click();
  await expect(page.getByRole("status")).toContainText("Policy enabled.");
  await expect(readyAutomaticRow).toContainText("Enabled");
  await expect(
    page.getByLabel("AI consultation workflow readiness"),
  ).toContainText("Ready automatic diagnosis workflow");
  await expect(
    page.getByLabel("AI consultation workflow readiness"),
  ).toContainText("Ready");
  await readyAutomaticRow.getByRole("button", { name: "Replay" }).click();
  const readyReplayDialog = page.getByRole("dialog", {
    name: /Replay Policy/,
  });
  await readyReplayDialog.getByRole("button", { name: "Start Replay" }).click();
  await expect(readyReplayDialog).toContainText("Workflow accepted");
  await expect(readyReplayDialog).toContainText(
    "AI diagnosis: 1 policy, 2 snapshots, 1 room started, 1 snapshot skipped by safety cap",
  );
  await expect(readyReplayDialog).toContainText(
    `Correlation report-workflow-policy:${readyPolicy.id}:manual-replay`,
  );
  await expect(readyReplayDialog).toContainText(
    "1 snapshot retained for manual AI room creation after the automatic room limit was reached.",
  );
  const replayProofTrace = readyReplayDialog.getByLabel("Replay proof trace");
  await expect(replayProofTrace).toContainText("Replay Proof Trace");
  await expect(replayProofTrace).toContainText("Workflow accepted");
  await expect(replayProofTrace).toContainText("2 saved");
  await expect(replayProofTrace).toContainText("1 room");
  await expect(replayProofTrace).toContainText("Room timeline");
  await expect(replayProofTrace).toContainText(
    "diagnosis-room notification timeline",
  );
  const diagnosisRoomLink = readyReplayDialog.getByRole("link", {
    name: /Review room #7/,
  });
  await expect(diagnosisRoomLink).toHaveAttribute(
    "href",
    /\/diagnosis-room\?evidence_snapshot_id=7&intent=review_conclusion&session_id=diagnosis-session-auto-p\d+-s7/,
  );
  const manualRoomLink = readyReplayDialog.getByRole("link", {
    name: /Create room #8/,
  });
  await expect(manualRoomLink).toHaveAttribute(
    "href",
    "/diagnosis-room?evidence_snapshot_id=8&intent=alert_review",
  );
  await readyReplayDialog
    .locator("button")
    .filter({ hasText: "Close" })
    .click();
  await expect(readyReplayDialog).toBeHidden();

  const blockedDeliveryRow = page
    .getByRole("row", { name: /Blocked delivery workflow/ })
    .first();
  await expect(blockedDeliveryRow).toContainText("Operations WeCom");
  await blockedDeliveryRow.getByRole("button", { name: "Impact" }).click();
  await expect(page.getByRole("status")).toContainText(
    "Impact preview blocked",
  );
  const blockedImpactDialog = page.getByRole("dialog", {
    name: /Impact Preview/,
  });
  await expect(blockedImpactDialog).toContainText(
    "Notification channel disabled",
  );
  await expect(blockedImpactDialog).toContainText(
    "Enable the bound notification channel before report delivery.",
  );
  await blockedImpactDialog
    .locator("button")
    .filter({ hasText: "Close" })
    .click();

  const settingsForm = page.locator("form");
  await page.goto(
    "/settings/report-workflow-policies?intent=create-auto-room-policy",
  );
  await expect(page.getByLabel("Report workflow launch preset")).toContainText(
    "Prepared an automatic diagnosis workflow from the settings overview create action.",
  );
  await expect(page.getByLabel("Name")).toHaveValue(
    "Automatic diagnosis workflow",
  );
  await expect(settingsForm).toContainText("Ready Alertmanager");
  await expect(settingsForm).toContainText("Default alert grouping");
  await expect(settingsForm).toContainText("Operations close WeCom");
  await expect(page.getByLabel("Alert source webhook readiness")).toContainText(
    "Webhook auto-room ingress ready.",
  );
  await expect(page.getByLabel("Notification channel readiness")).toContainText(
    "Report and auto-room delivery ready.",
  );

  await page.goto(
    `/settings/report-workflow-policies?intent=auto-room-follow-up&source_id=${readyAlertmanager.id}`,
  );
  await expect(page.getByLabel("Report workflow launch preset")).toContainText(
    "Prepared automatic diagnosis room handoff from the settings overview action.",
  );
  await expect(page.getByLabel("Name")).toHaveValue(
    "Automatic diagnosis workflow",
  );
  await expect(settingsForm).toContainText("Ready Alertmanager");
  await expect(settingsForm).toContainText("Operations close WeCom");
  await expect(page.getByLabel("Alert source webhook readiness")).toContainText(
    "Webhook auto-room ingress ready.",
  );
  await expect(page.getByLabel("Notification channel readiness")).toContainText(
    "Report and auto-room delivery ready.",
  );
  await expect(page.getByLabel("Diagnosis tool readiness")).toContainText(
    "Executable diagnosis tools ready.",
  );
  const automationOutcome = page.getByLabel("Workflow automation outcome");
  await expect(automationOutcome).toContainText("Automation Outcome");
  await expect(automationOutcome).toContainText("Webhook auto-room");
  await expect(automationOutcome).toContainText("Tool collection");
  await expect(automationOutcome).toContainText("Automatic");
  await expect(automationOutcome).toContainText("Report and AI updates");
  const draftWorkflowPlan = page.getByLabel("Draft workflow execution plan");
  await expect(draftWorkflowPlan).toContainText("Save policy");
  await expect(draftWorkflowPlan).toContainText("Impact preview");
  await expect(draftWorkflowPlan).toContainText("Replay window");
  await expect(draftWorkflowPlan).toContainText("AI handoff");
  await expect(draftWorkflowPlan).toContainText("Ready Alertmanager");
  await expect(draftWorkflowPlan).toContainText("Operations close WeCom");

  await page.goto("/settings/report-workflow-policies");
  await page.getByLabel("Name").fill("Cascade report workflow");
  const alertSourceSelect = settingsForm.getByRole("combobox", {
    name: /Alert source/,
  });
  await alertSourceSelect.click();
  await alertSourceSelect.fill("Primary Prometheus");
  await alertSourceSelect.press("Enter");
  const groupingPolicySelect = settingsForm.getByRole("combobox", {
    name: /Grouping policy/,
  });
  await groupingPolicySelect.click();
  await groupingPolicySelect.fill("Default alert grouping");
  await groupingPolicySelect.press("Enter");
  const diagnosisFollowUpControl = settingsForm
    .locator(".ant-form-item")
    .filter({ hasText: /^Diagnosis follow-up/ })
    .locator(".ant-segmented-item");
  await diagnosisFollowUpControl.filter({ hasText: /^Suggest room$/ }).click();
  await expect(page.getByLabel("Alert source webhook readiness")).toContainText(
    "No Alertmanager webhook ingress.",
  );
  await expect(page.getByLabel("Diagnosis tool readiness")).toContainText(
    "Missing active_alerts for the selected source.",
  );

  await diagnosisFollowUpControl.filter({ hasText: /^Auto room$/ }).click();
  await expect(page.getByLabel("Alert source webhook readiness")).toContainText(
    "Webhook auto-room ingress blocked.",
  );
  await expect(settingsForm).toContainText(
    "Automatic diagnosis room starts require an Alertmanager alert source because the webhook endpoint rejects non-Alertmanager profiles.",
  );

  await diagnosisFollowUpControl.filter({ hasText: /^Disabled$/ }).click();
  await settingsForm.getByRole("button", { name: "Save Policy" }).click();
  await expect(page.getByRole("status")).toContainText("Policy saved.");
  const cascadeRow = page
    .getByRole("row", { name: /Cascade report workflow/ })
    .first();
  await expect(cascadeRow).toContainText("Draft");

  await cascadeRow.getByRole("button", { name: "Impact" }).click();
  await expect(page.getByRole("status")).toContainText("Impact preview ready");
  const impactDialog = page.getByRole("dialog", { name: /Impact Preview/ });
  await expect(impactDialog).toContainText("Preview ready");
  await expect(impactDialog).toContainText(
    "Configuration bindings are usable and the bounded sample produced an impact estimate.",
  );
  await expect(impactDialog).toContainText("HighCPU");
  await expect(
    page.getByLabel("AI consultation workflow counters"),
  ).toContainText("Ready previews");
  await impactDialog.locator("button").filter({ hasText: "Close" }).click();
  await expect(impactDialog).toBeHidden();

  await cascadeRow.getByRole("button", { name: "Enable" }).click();
  await expect(page.getByRole("status")).toContainText(
    "Policy enabled with review items: No notification channel profile is bound.",
  );
  await expect(cascadeRow).toContainText("Enabled");

  await cascadeRow.getByRole("button", { name: "Replay" }).click();
  const replayDialog = page.getByRole("dialog", { name: /Replay Policy/ });
  await replayDialog.getByRole("button", { name: "Start Replay" }).click();
  await expect(replayDialog).toContainText("Workflow accepted");
  await expect(replayDialog).toContainText("report-batch-policy-smoke");
  await replayDialog.locator("button").filter({ hasText: "Close" }).click();
  await expect(replayDialog).toBeHidden();

  await cascadeRow.getByRole("button", { name: "Disable" }).click();
  await expect(page.getByRole("status")).toContainText("Policy disabled.");
  await expect(cascadeRow).toContainText("Draft");

  await settingsForm
    .getByRole("textbox", { name: /Name/ })
    .fill("Disabled source workflow");
  const disabledSourceSelect = settingsForm.getByRole("combobox", {
    name: /Alert source/,
  });
  await disabledSourceSelect.click();
  await disabledSourceSelect.fill("Staging Alertmanager");
  await disabledSourceSelect.press("Enter");
  const disabledSourceGroupingSelect = settingsForm.getByRole("combobox", {
    name: /Grouping policy/,
  });
  await disabledSourceGroupingSelect.click();
  await disabledSourceGroupingSelect.fill("Default alert grouping");
  await disabledSourceGroupingSelect.press("Enter");
  await settingsForm.getByRole("button", { name: "Save Policy" }).click();
  await expect(page.getByRole("status")).toContainText("Policy saved.");
  const disabledSourceRow = page
    .getByRole("row", { name: /Disabled source workflow/ })
    .first();
  await expect(disabledSourceRow).toContainText("Draft");
  await expect(
    disabledSourceRow.getByRole("button", { name: "Enable" }),
  ).toBeDisabled();
});

test("report workflow schedule settings route creates and toggles schedules", async ({
  page,
}) => {
  await page.goto("/settings/report-workflow-schedules");

  await expect(
    page.getByRole("heading", { name: "Workflow Schedules" }),
  ).toBeVisible();
  await expect(page.getByText("Daily report window")).toBeVisible();
  await expect(
    page.getByText("openclarion-report-policy-1-daily"),
  ).toBeVisible();
  const scheduleReadiness = page.getByLabel("Schedule readiness preview");
  await expect(scheduleReadiness).toContainText(
    "Select a report workflow policy.",
  );
  const disabledPolicyScheduleRow = page.getByRole("row", {
    name: /Disabled policy report window/,
  });
  const blockedEnableButton = disabledPolicyScheduleRow.getByRole("button", {
    name: "Enable",
  });
  await expect(blockedEnableButton).toBeDisabled();
  await blockedEnableButton.hover();
  await expect(page.getByRole("tooltip")).toContainText(
    "Bound report workflow policy must be enabled before schedule enablement.",
  );

  const settingsForm = page.locator("form");
  await page.goto(
    "/settings/report-workflow-schedules?intent=create-schedule&policy_id=1",
  );
  await expect(
    page.getByLabel("Report workflow schedule launch preset"),
  ).toContainText(
    "Prepared an hourly replay schedule from the settings overview proof action.",
  );
  await expect(page.getByLabel("Name")).toHaveValue("Hourly report replay");
  await expect(page.getByLabel("Temporal Schedule ID")).toHaveValue(
    "openclarion-report-policy-1-hourly",
  );
  await expect(page.getByLabel("Interval seconds")).toHaveValue("3600");
  await expect(page.getByLabel("Schedule readiness preview")).toContainText(
    "Ready to save.",
  );
  await expect(settingsForm).toContainText("Default report workflow");
  const scheduledProofOutcome = page.getByLabel("Scheduled proof outcome");
  await expect(scheduledProofOutcome).toContainText("Scheduled Proof Outcome");
  await expect(scheduledProofOutcome).toContainText(
    "This schedule can retain recurring proof",
  );
  await expect(scheduledProofOutcome).toContainText("Every 1h");
  await expect(scheduledProofOutcome).toContainText("1h window");
  await expect(scheduledProofOutcome).toContainText(
    "openclarion-report-policy-1-hourly",
  );

  await page.goto("/settings/report-workflow-schedules");
  await page.getByLabel("Name").fill("Six-hour report window");
  const workflowPolicySelect = settingsForm.getByRole("combobox", {
    name: /Report workflow policy/,
  });
  await workflowPolicySelect.click();
  await workflowPolicySelect.fill("Default report workflow");
  await workflowPolicySelect.press("Enter");
  await page
    .getByLabel("Temporal Schedule ID")
    .fill("openclarion-report-policy-1-6h");
  await page.getByLabel("Interval seconds").fill("21600");
  await page.getByLabel("Offset seconds").fill("0");
  await page.getByLabel("Replay window seconds").fill("3600");
  await page.getByLabel("Replay delay seconds").fill("300");
  await page.getByLabel("Replay limit").fill("10000");
  await page.getByLabel("Catch-up seconds").fill("1800");
  await expect(scheduleReadiness).toContainText("Ready to save.");
  await expect(scheduleReadiness).toContainText("#1 Default report workflow");
  await expect(scheduleReadiness).toContainText("Every 6h");
  await expect(scheduleReadiness).toContainText(
    "Starts every 6h after offset 0s.",
  );
  await expect(scheduleReadiness).toContainText(
    "Window 1h / delay 5m / limit 10000",
  );
  await expect(scheduleReadiness).toContainText("30m");
  await expect(scheduledProofOutcome).toContainText("Every 6h");
  await expect(scheduledProofOutcome).toContainText("1h window");
  await expect(scheduledProofOutcome).toContainText("5m delay");
  await expect(scheduledProofOutcome).toContainText("30m");
  await expect(scheduledProofOutcome).toContainText(
    "openclarion-report-policy-1-6h",
  );
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

test("diagnosis tool template settings route creates and toggles templates", async ({
  page,
}) => {
  const templatesResponse = await page.request.get(
    "/api/config/diagnosis-tool-templates",
  );
  expect(templatesResponse.ok()).toBeTruthy();
  const templates = await templatesResponse.json();
  for (const template of templates.items ?? []) {
    if (!template.enabled) {
      continue;
    }
    const disableResponse = await page.request.post(
      `/api/config/diagnosis-tool-templates/${template.id}/disable`,
    );
    expect(disableResponse.ok()).toBeTruthy();
  }

  await page.goto("/settings/diagnosis-tool-templates");

  await expect(
    page.getByRole("heading", { name: "Diagnosis Tools" }),
  ).toBeVisible();
  await expect(page.getByText("CPU saturation range")).toBeVisible();
  const evidenceCoverage = page.getByLabel("AI evidence tool coverage");
  await expect(evidenceCoverage).toContainText("No enabled evidence tools.");
  await expect(evidenceCoverage).toContainText(
    "Enable active alert and metric templates before relying on AI follow-up.",
  );

  const settingsForm = page.locator("form");
  const sourceCompatibility = page.getByLabel("Source compatibility");
  await expect(sourceCompatibility).toContainText(
    "Select a compatible source.",
  );
  await expect(sourceCompatibility).toContainText(/\d+ compatible sources?/);
  await page.goto(
    "/settings/diagnosis-tool-templates?intent=metric-evidence-tool",
  );
  await expect(page.getByLabel("Diagnosis tool launch preset")).toContainText(
    "Prepared Kubernetes pod CPU range from the settings overview action.",
  );
  await expect(page.getByLabel("Name")).toHaveValue("Kubernetes pod CPU range");
  await expect(page.getByLabel("Query template")).toHaveValue(
    /container_cpu_usage_seconds_total/,
  );
  await expect(page.getByLabel("Source compatibility")).toContainText(
    "Source compatible.",
  );

  await page.goto("/settings/diagnosis-tool-templates");
  await page.getByLabel("Name").fill("Memory pressure range");
  const alertSourceSelect = settingsForm.getByRole("combobox", {
    name: /Alert source/,
  });
  await settingsForm
    .locator(".ant-form-item")
    .filter({ hasText: /^Tool/ })
    .locator(".ant-segmented-item")
    .filter({ hasText: /^Range$/ })
    .click();
  await expect(sourceCompatibility).toContainText(/\d+ compatible sources?/);
  await expect(sourceCompatibility).toContainText(
    "Select Prometheus-compatible",
  );
  await alertSourceSelect.click();
  await alertSourceSelect.fill("Primary Prometheus");
  await alertSourceSelect.press("Enter");
  await expect(sourceCompatibility).toContainText("Source compatible.");
  await page
    .getByLabel("Query template")
    .fill("container_memory_working_set_bytes");
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
  await expect(evidenceCoverage).toContainText(
    "Evidence coverage needs review.",
  );
  await expect(evidenceCoverage).toContainText(
    "Missing active alert collection for AI follow-up.",
  );
  await expect(evidenceCoverage).toContainText("Metric tools 1");

  await templateRow.getByRole("button", { name: "Disable" }).click();
  await expect(page.getByRole("status")).toContainText("Template disabled.");
  await expect(templateRow).toContainText("Draft");

  await page.getByLabel("Name").fill("Disabled source active alerts");
  await alertSourceSelect.click();
  await alertSourceSelect.fill("Staging Alertmanager");
  await alertSourceSelect.press("Enter");
  await settingsForm.getByRole("button", { name: "Save Template" }).click();
  await expect(page.getByRole("status")).toContainText("Template saved.");
  const disabledSourceTemplateRow = page
    .getByRole("row", { name: /Disabled source active alerts/ })
    .first();
  await expect(disabledSourceTemplateRow).toContainText("Draft");
  const blockedTemplateEnableButton = disabledSourceTemplateRow.getByRole(
    "button",
    { name: "Enable" },
  );
  await expect(blockedTemplateEnableButton).toBeDisabled();
  await blockedTemplateEnableButton.hover();
  await expect(page.getByRole("tooltip")).toContainText(
    "Bound alert source must be enabled before template enablement.",
  );
});

test("notification channel settings route lists and creates channels", async ({
  page,
}) => {
  await page.goto("/settings/notification-channels");

  await expect(
    page.getByRole("heading", { name: "Notification Channels" }),
  ).toBeVisible();
  await expect(page.getByText("Operations WeCom")).toBeVisible();
  await expect(page.getByText("secret/example/ops-wecom")).toBeVisible();
  const channelReadiness = page.getByLabel(
    "Notification channel delivery readiness",
  );
  await expect(channelReadiness).toContainText("Delivery scopes need review.");
  await expect(channelReadiness).toContainText(
    "Missing scopes: diagnosis_consultation, diagnosis_close",
  );

  await page.goto("/settings/notification-channels?channel_id=1");
  await expect(
    page.getByLabel("Notification channel launch preset"),
  ).toContainText("Loaded notification channel #1 for delivery review.");
  await expect(page.getByText("Edit Channel #1")).toBeVisible();
  await expect(page.getByLabel("Name")).toHaveValue("Operations WeCom");
  await expect(page.getByLabel("Secret reference")).toHaveValue(
    "secret/example/ops-wecom",
  );

  await page.goto("/settings/notification-channels");
  const operationsRow = page.getByRole("row", { name: /Operations WeCom/ });
  const operationsTestButton = operationsRow
    .getByRole("button", { name: /Test$/ })
    .first();
  await expect(operationsTestButton).toBeEnabled();
  await operationsTestButton.click();
  await expect(
    page
      .getByRole("status")
      .filter({
        hasText:
          "Secret-backed notification channel tests require a server-side secret resolver.",
      }),
  ).toBeVisible();
  await expect(operationsRow).toContainText("Not tested");

  await page.goto(
    "/settings/notification-channels?intent=report-close-channel",
  );
  await expect(
    page.getByLabel("Notification channel launch preset"),
  ).toContainText(
    "Prepared an Enterprise WeChat channel for final reports, automatic diagnosis updates, and close notifications.",
  );
  await expect(page.getByLabel("Name")).toHaveValue(
    "AI report and diagnosis WeCom",
  );
  await expect(page.getByRole("checkbox", { name: "Reports" })).toBeChecked();
  await expect(
    page.getByRole("checkbox", { name: "Diagnosis consultation" }),
  ).toBeChecked();
  await expect(
    page.getByRole("checkbox", { name: "Diagnosis close" }),
  ).toBeChecked();
  await expect(page.getByLabel("Labels")).toHaveValue(
    "provider=wecom\nrole=ai-room-delivery\nscope=report-consultation-close",
  );
  await expect(page.getByLabel("Enabled")).toBeChecked();
  await expect(
    page.getByLabel("Notification channel delivery readiness"),
  ).toContainText("Delivery scopes ready.");
  await expect(
    page.getByLabel("Notification channel delivery readiness"),
  ).toContainText(/Credential\s*Missing/);

  await page.goto("/settings/notification-channels");
  const settingsForm = page.locator("form");
  await expect(page.getByLabel("Name")).toBeEnabled();
  await settingsForm
    .locator(".ant-form-item")
    .filter({ hasText: /^Kind/ })
    .locator(".ant-segmented-item")
    .filter({ hasText: /^WeCom$/ })
    .click();
  await page.getByLabel("Name").fill("Incident WeCom");
  await page
    .getByLabel("Secret reference")
    .fill("secret/example/incident-webhook");
  await settingsForm.getByLabel("Diagnosis consultation").check();
  await settingsForm.getByLabel("Diagnosis close").check();
  await expect(channelReadiness).toContainText("Delivery scopes ready.");
  await expect(channelReadiness).toContainText("Final reports");
  await expect(channelReadiness).toContainText("AI diagnosis updates");
  await expect(channelReadiness).toContainText("Auto-room close");
  await page.getByLabel("Labels").fill("team=ops");
  await page.getByLabel("Enabled").check();
  await settingsForm.getByRole("button", { name: "Save Channel" }).click();

  await expect(
    page.getByRole("status").filter({ hasText: "Channel saved." }),
  ).toBeVisible();
  const incidentRow = page.getByRole("row", { name: /Incident WeCom/ });
  await expect(incidentRow).toContainText("secret/example/incident-webhook");
  await expect(incidentRow).toContainText("diagnosis_consultation");
  await expect(incidentRow).toContainText("diagnosis_close");
  await expect(incidentRow).toContainText("Enabled");

  await page.getByLabel("Name").fill("Incident Slack");
  await settingsForm
    .locator(".ant-form-item")
    .filter({ hasText: /^Kind/ })
    .locator(".ant-segmented-item")
    .filter({ hasText: /^Slack$/ })
    .click();
  await page.getByLabel("Secret reference").fill("secret/example/ops-slack");
  await settingsForm.getByLabel("Diagnosis consultation").uncheck();
  await settingsForm.getByLabel("Diagnosis close").uncheck();
  await settingsForm.getByLabel("Reports").check();
  await settingsForm.getByRole("button", { name: "Save Channel" }).click();

  await expect(
    page.getByRole("status").filter({ hasText: "Channel saved." }),
  ).toBeVisible();
  const slackRow = page.getByRole("row", { name: /Incident Slack/ });
  await expect(slackRow).toContainText("slack");
  await expect(slackRow).toContainText("secret/example/ops-slack");
});
