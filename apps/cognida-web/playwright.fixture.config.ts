import { defineConfig } from '@playwright/test'

// No real backend or account. Every API request is intercepted by this suite.
// Keep it separate from the durable real-service acceptance suite.
const baseURL = process.env.COGNIDA_FIXTURE_BASE_URL || 'http://127.0.0.1:62813'
const url = new URL(baseURL)
if (!['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname) || !['http:', 'https:'].includes(url.protocol) || url.username || url.password) {
  throw new Error('Fixture UI tests require an explicit loopback frontend without credentials')
}
export default defineConfig({
  testDir: './e2e-fixtures',
  workers: 1,
  retries: 0,
  timeout: 30_000,
  expect: { timeout: 8_000 },
  outputDir: './test-results-fixture',
  reporter: [['list']],
  use: {
    baseURL,
    channel: process.env.COGNIDA_FIXTURE_BROWSER === 'chromium' ? undefined : 'chrome',
    viewport: { width: 1440, height: 1000 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
})
