# Frontend state-race regression (HTTP/SSE fixtures)

This suite runs the actual Vue application in Chrome against controlled API responses. It tests browser interactions and request ordering, **not** real database/model execution. It uses synthetic tokens and intercepts every `/api/v1/` request; unexpected API calls fail the test instead of reaching Vite's backend proxy. It is separate from `e2e/` and requires no real account or backend services.

```sh
npm ci
npm run dev -- --host 127.0.0.1 --port 62813 --strictPort
# In another terminal, with the frontend already running:
COGNIDA_FIXTURE_BASE_URL=http://127.0.0.1:62813 npm run test:e2e:fixture
# Stop the Vite process after the run.
```

Chrome is the local default. CI may install Playwright Chromium and use `COGNIDA_FIXTURE_BROWSER=chromium`. The explicit frontend URL must be loopback. `npm run test:e2e:check` type-checks this suite as well as the separate real-service suite.

Covered interactions:

- Collapse/reopen the navigation (same-workspace membership refresh) preserves an unsaved target and candidate; a real workspace switch clears them.
- Data Agent's first session URL sync preserves the explicitly selected datasource, displayed answer, and incomplete/save-failed terminal receipt; the next request uses the same datasource.
- A delayed session-history response cannot replace a newly completed turn.
- An old SSE 401 and delayed failed refresh cannot log out a newer workspace, and no old request is replayed.

Unit/component tests additionally cover old login/abort during refresh, A→B→A history ordering, unmount, actual router-instance reuse/identity reset, all five same-identity page watchers, and safe unknown terminal-reason rendering. The receipt warning is based on the live stream's actual metadata; this frontend-only change does not add metadata to historical backend messages. Selecting a datasource remains a page choice, not a new persisted session setting.

Artifacts are in `test-results-fixture/` (ignored). No whole-case retries are configured.
