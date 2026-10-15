# Isolated lifecycle source fixtures

Run from `services/cognida-go` after loading the disposable environment's
`runtime.env`. Pass its directory containing `account.json` (the authenticated
test account). The account identity must match the real profile API response.
Use a distinct test tenant ID when validating against a shared repaired vector
service so existing tenant data stays separate. Both API and Redis must use loopback, the database
name must start with `cognida_review_`, and `ISOLATED=true` is required.

```sh
ISOLATED=true go run ./cmd/seed-lifecycle-sources -dir /path/to/isolated-run
ISOLATED=true go run ./cmd/seed-lifecycle-sources -dir /path/to/isolated-run -status-only
```

The command creates a three-row SQL table, registers it through the datasource
API, creates an owned session, and runs the real `sql_execute` tool. It verifies
the two aggregate rows persisted in Redis. Repeating the command reuses the
upstream resources and refreshes the result handle (default lifetime: 30 minutes).

It also creates a two-sample QA dataset and an evaluation task through the
normal API. The existing evaluation worker performs actual generation and
Python ROUGE/BLEU grading. Model/provider errors remain real failures; the
command never updates task status or inserts evaluation results itself.

Non-sensitive IDs, actual evaluation status, and upstream descriptions are saved
to `source-fixtures.json`. Credentials are read only from the environment and
account file and are excluded from that manifest.

After browser verification and once evaluation is no longer running:

```sh
ISOLATED=true go run ./cmd/seed-lifecycle-sources -dir /path/to/isolated-run -cleanup
```

Cleanup calls the real delete APIs, removes the tracked Redis result keys, drops
only the manifest's strictly validated fixture table, and renames the manifest
to `source-fixtures.json.cleaned`. Existing API soft-delete behavior is retained;
drop the disposable review database when tearing down the whole environment.
