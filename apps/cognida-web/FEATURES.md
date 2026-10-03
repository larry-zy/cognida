# Data lifecycle workspaces

The web client uses the authenticated `/api/v1` endpoints. Build it with `npm ci && npm run build`; run `npm test` and `npm run lint` before delivery. The Go service serves the built `dist` directory when configured with the existing web deployment layout. Run MySQL versioned migrations before starting the new features; no runtime schema creation is performed.

## Main flows

| Route | Available workflow |
| --- | --- |
| `/catalog` | Sync datasource schema, optional sampled column profiles, search, owner/tags/classification, business glossary including column links, lineage traversal and impact, semantic model rebuild. Identity cards describe their method, sample limitations, risks, and related assets. |
| `/semantic-models` | Browse drafts/published/deprecated models. Edit logical tables, dimensions, measures, metrics, relations, aliases and value mappings, then save/publish/deprecate. |
| `/experts` | Create a text/table asset or import a bounded web/knowledge/datasource/collection/result/evaluation input, invoke an independently registered specialist, inspect evidence and approval requests, cancel work. |
| `/pipelines` | Start a registered template with immutable asset references. Inspect stage statuses/gates/approvals, cancel/retry, and follow SSE progress with polling fallback. |
| `/analysis` | Analyze published assets and view method, recommendation, evidence, assumptions, risks and verification status. Missing confidence or verification is explicitly unknown. |
| `/analysis/chat/:conversationId?` | Existing datasource conversation experience; `/data-agent/:conversationId?` remains an alias. Default analysis tools do not grant mutation rights. Chart points support mouse/keyboard drill-down into a read-only follow-up conversation. |
| `/subscriptions` | Create/disable scheduled reports or metric watch, inspect run/error history, read in-app notifications. Inputs default to a fixed published snapshot; optional latest-published mode is explicitly restricted to a matching approved source. |
| `/labeling` | Create annotation/tagging tasks from versioned assets, inspect provenance/confidence, edit labels/entities/relations/tags, approve/reject, export approved annotations as authenticated NDJSON. |
| `/features` | Register field-derived numerical/categorical/vector features, create immutable definition versions, compute values from source rows, write entity values, query a selected version at a time boundary. |
| `/suggestions` | Discover real source candidates or submit evidence-backed proposals. Human review applies knowledge, metric synonyms or experience; target/error states stay visible. |
| `/evaluation` | Compare two completed same-dataset/task-type evaluations using paired scores, minimum evidence and regression thresholds; display interval/gate/limitations without changing production flags. |

## Membership and state

The workspace switcher lists current active memberships. Creating a workspace creates an owner membership. Switching obtains new server-signed access/refresh tokens; refresh preserves the selected workspace. The page tree is remounted on identity changes, cached knowledge/session state is reset, and old-identity failed requests are not retried under the new identity. A failed switch retains the previous identity.

Settings already supplies a persisted Chinese/English locale. New navigation, key form labels and core actions use the shared reactive workbench lexicon. User-authored content is not translated. Detailed diagnostic messages preserve the server's original text.

## Operational requirements and error handling

- Lifecycle, governance and collection workers must be enabled for queued work to run. `BACKGROUND_WORKERS_ENABLED=false` is intended for isolated HTTP/UI tests, not a functioning scheduled deployment.
- Publication is approval-gated. Users can inspect pending approvals; only authorized tenant administrators can approve/reject. Changing metadata never substitutes for publication authorization.
- Source/evidence inputs use `asset_id:version`. Data payloads remain in object storage and are resolved by the server with current authorization and integrity checks.
- Creation/actions reuse an idempotency key after uncertain failures; a completed explicit new submission receives a fresh key. Version conflicts are shown to the user and do not silently overwrite other edits.
- Run failures and partial results remain inspectable. Missing optional services fail visibly; the UI does not manufacture successful results or confidence.
- Static homepage example statistics were replaced with direct workspace links. Counts and values elsewhere come from endpoint results.

## Verification

Frontend unit coverage includes identity refresh/switch races, cache isolation, streaming lifecycle, idempotent retries, semantic form submission, lifecycle result envelopes, suggestion discovery/review, versioned governance references, chart keyboard/mouse drill-down and reactive localization.

The 2026-10 review used only the dedicated `cognida_review_20261003` database. Actual HTTP and browser checks covered semantic draft/publication, workspace switching/isolation, catalog sync/search/governance/glossary, annotation approval and feature computation/readback. Service startup and shutdown remain the responsibility of the test owner; never reuse production credentials or run background workers against a mixed fixture database.

## Reliability refinements

Asset and feature-version selectors fetch one cursor page at a time, with an explicit load-more action. Existing saved references remain selectable even when their page is not loaded. Changing the source or leaving the page invalidates stale option responses; repeated load-more clicks share one request.

Requests carry an in-memory identity generation. A response from a previous login or workspace cannot update the new session, while normal token refresh keeps valid in-flight responses usable. Refresh requests are shared only within one identity. Execution detail updates are coalesced and apply only to the selected run; SSE reconnects on transient disconnects and is closed on terminal state, token replacement, or unmount. Publishing a semantic model keeps the target selected at submission time even if another model is opened before the save finishes.
