import { defineConfig } from '@playwright/test'

// No server or credentials are inferred: this suite writes durable test assets.
if (process.env.COGNIDA_E2E_ISOLATED !== '1') {
  throw new Error('Set COGNIDA_E2E_ISOLATED=1 only for a dedicated, disposable test environment.')
}
for (const key of ['COGNIDA_E2E_BASE_URL', 'COGNIDA_E2E_API_URL', 'COGNIDA_E2E_ACCOUNT_FILE']) {
  if (!process.env[key]) throw new Error(`Missing explicit environment setting: ${key}`)
}
for (const key of ['COGNIDA_E2E_BASE_URL', 'COGNIDA_E2E_API_URL']) {
  const url = new URL(process.env[key]!)
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) throw new Error(`${key} must be an HTTP(S) origin without credentials`)
}
const timeout = Number(process.env.COGNIDA_E2E_TIMEOUT_MS || 300_000)
if (!Number.isFinite(timeout) || timeout < 10_000) throw new Error('COGNIDA_E2E_TIMEOUT_MS must be at least 10000')

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.spec.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout,
  expect: { timeout: 15_000 },
  outputDir: './test-results',
  reporter: [['list'], ['html', { outputFolder: 'playwright-report', open: 'never' }]],
  use: {
    baseURL: process.env.COGNIDA_E2E_BASE_URL,
    channel: 'chrome',
    viewport: { width: 1440, height: 1000 },
    locale: 'zh-CN',
    actionTimeout: 20_000,
    navigationTimeout: 30_000,
    screenshot: 'only-on-failure',
    // Login uses real credentials. Do not persist auth headers or login bodies in traces.
    trace: 'off',
    video: 'off',
  },
})
