import { defineConfig, devices } from '@playwright/test';

// Browser tests of the real app (RULES §8): the Go server of
// internal/app/e2e_browser_test.go serves the built UI with saved site
// pages, so nothing reaches the network. Run with `npm run e2e`: it builds
// the UI once and runs each project against its own fresh server (one
// library each).
const PORT = 18765;
const CONTROL_PORT = 18766;

export default defineConfig({
  testDir: 'e2e',
  fullyParallel: false,
  workers: 1, // one server and one library for all tests
  forbidOnly: true, // a forgotten test.only would skip the rest
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    locale: 'pt-BR',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    // Poco F4 GT: 1080×2400 at 2.75 → 393×873 CSS pixels.
    {
      name: 'phone',
      use: {
        ...devices['Pixel 7'],
        viewport: { width: 393, height: 873 },
        deviceScaleFactor: 2.75,
      },
    },
  ],
  webServer: {
    command: `cd .. && E2E_PORT=${PORT} E2E_CONTROL_PORT=${CONTROL_PORT} go test -tags e2eweb -run TestServeForBrowser -count=1 -timeout 0 ./internal/app`,
    url: `http://127.0.0.1:${PORT}/api/v1/health`,
    timeout: 120_000,
    reuseExistingServer: false,
  },
});

export const controlURL = `http://127.0.0.1:${CONTROL_PORT}`;
