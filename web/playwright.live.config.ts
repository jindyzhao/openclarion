import { defineConfig, devices } from "@playwright/test";

const host = "127.0.0.1";
const webPort = process.env.OPENCLARION_LIVE_WEB_PORT ?? "32101";
const providedBaseURL = process.env.OPENCLARION_LIVE_WEB_BASE_URL?.trim();
const localBaseURL = `http://${host}:${webPort}`;
const baseURL = providedBaseURL && providedBaseURL !== "" ? providedBaseURL : localBaseURL;
const isCI = Boolean(process.env.CI);
const liveAPIBaseURL = process.env.OPENCLARION_LIVE_API_BASE_URL?.trim() ?? "";
const liveWSBaseURL = process.env.OPENCLARION_LIVE_BROWSER_WS_BASE_URL?.trim() || liveAPIBaseURL;
const liveTurnTimeoutMS = positiveIntegerEnv("OPENCLARION_LIVE_TURN_TIMEOUT_MS", 120_000);
const liveTestTimeoutMS = positiveIntegerEnv("OPENCLARION_LIVE_TEST_TIMEOUT_MS", liveTurnTimeoutMS + 60_000);
const liveWebServerTimeoutMS = positiveIntegerEnv("OPENCLARION_LIVE_WEB_SERVER_TIMEOUT_MS", 120_000);

export default defineConfig({
  testDir: "./tests/live",
  fullyParallel: false,
  forbidOnly: isCI,
  retries: 0,
  workers: 1,
  timeout: liveTestTimeoutMS,
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
          command: `OPENCLARION_API_BASE_URL=${shellQuote(liveAPIBaseURL)} OPENCLARION_BROWSER_WS_BASE_URL=${shellQuote(liveWSBaseURL)} npm run start -- --hostname ${host} --port ${webPort}`,
          url: localBaseURL,
          reuseExistingServer: !isCI,
          timeout: liveWebServerTimeoutMS
        }
});

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function positiveIntegerEnv(name: string, fallback: number): number {
  const raw = process.env[name]?.trim();
  if (!raw) {
    return fallback;
  }
  if (!/^[1-9][0-9]*$/.test(raw)) {
    throw new Error(`${name} must be a positive integer`);
  }
  return Number(raw);
}
