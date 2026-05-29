import { defineConfig, devices } from "@playwright/test";

const host = "127.0.0.1";
const webPort = process.env.OPENCLARION_LIVE_WEB_PORT ?? "32101";
const providedBaseURL = process.env.OPENCLARION_LIVE_WEB_BASE_URL?.trim();
const localBaseURL = `http://${host}:${webPort}`;
const baseURL = providedBaseURL && providedBaseURL !== "" ? providedBaseURL : localBaseURL;
const isCI = Boolean(process.env.CI);

export default defineConfig({
  testDir: "./tests/live",
  fullyParallel: false,
  forbidOnly: isCI,
  retries: 0,
  workers: 1,
  timeout: 120_000,
  reporter: [["list"], ["html", { open: "never", outputFolder: "playwright-report-live" }]],
  outputDir: "test-results-live",
  use: {
    baseURL,
    screenshot: "only-on-failure",
    trace: "retain-on-failure"
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] }
    }
  ],
  webServer:
    providedBaseURL && providedBaseURL !== ""
      ? undefined
      : {
          name: "next-live",
          command: `npm run start -- --hostname ${host} --port ${webPort}`,
          url: localBaseURL,
          reuseExistingServer: !isCI,
          timeout: 120_000
        }
});
