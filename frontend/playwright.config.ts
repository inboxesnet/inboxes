import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false, // Tests share state (authenticated sessions)
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: "html",
  timeout: 30000,
  use: {
    baseURL: process.env.E2E_BASE_URL || "http://localhost:3000",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  // Uncomment to auto-start servers:
  // webServer: [
  //   {
  //     command: "cd ../backend && go run ./cmd/api",
  //     url: "http://localhost:8080/api/health",
  //     reuseExistingServer: !process.env.CI,
  //   },
  //   {
  //     command: "npm run dev",
  //     url: "http://localhost:3000",
  //     reuseExistingServer: !process.env.CI,
  //   },
  // ],
});
