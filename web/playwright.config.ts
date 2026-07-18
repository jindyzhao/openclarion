import { defineConfig, devices } from "@playwright/test";

// Playwright forces colors in its web servers and workers. Remove the
// conflicting inherited opt-out before those child environments are created.
delete process.env.NO_COLOR;

const host = "127.0.0.1";
const webPort = process.env.OPENCLARION_PLAYWRIGHT_WEB_PORT ?? "32100";
const apiPort = process.env.OPENCLARION_PLAYWRIGHT_API_PORT ?? "38280";
const baseURL = `http://${host}:${webPort}`;
const apiBaseURL = `http://${host}:${apiPort}`;
const isCI = Boolean(process.env.CI);
const browserChannel = process.env.OPENCLARION_PLAYWRIGHT_CHANNEL;

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: true,
  forbidOnly: isCI,
  retries: isCI ? 1 : 0,
  workers: isCI ? 1 : undefined,
  reporter: [["list"], ["html", { open: "never" }]],
  outputDir: "test-results",
  use: {
    baseURL,
    trace: "on-first-retry"
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        ...(browserChannel ? { channel: browserChannel } : {})
      }
    }
  ],
  webServer: [
    {
      name: "mock-api",
      command: `OPENCLARION_MOCK_API_PORT=${apiPort} node tests/e2e/mock-api.mjs`,
      url: `${apiBaseURL}/healthz`,
      reuseExistingServer: !isCI,
      timeout: 30_000
    },
    {
      name: "next",
      command: `OPENCLARION_API_BASE_URL=${apiBaseURL} OPENCLARION_BROWSER_WS_BASE_URL=${apiBaseURL} npm run start -- --hostname ${host} --port ${webPort}`,
      url: baseURL,
      reuseExistingServer: !isCI,
      timeout: 120_000
    }
  ]
});
