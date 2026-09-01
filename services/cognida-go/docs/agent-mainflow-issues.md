# Agent Main-Flow Issue Report

Scope: the agent main execution flow — the Eino engine under
`internal/service/agent/framework/` (ReAct tool loop, sinks, context folding,
self-repair guard, parallel delegation) plus the handler/DTO boundary
(`internal/handler/agent_handler.go`, `internal/service/agent/registry_orchestrator.go`).

Status: review only — no code changed. Findings verified by cross-checking two
independent review passes (tool-loop termination; streaming & concurrency)
against a direct read of the source.

---

## High

### #1 — Streaming output is polluted: intermediate / truncation-retry text is streamed live, then the wind-down answer is concatenated on top
- **Where:** `framework/eino_sink.go:109-119` × `framework/eino_toolloop.go:196-208, 277-290`
- `streamSink.generate` pushes content deltas live for **every** `sink.generate` call in the loop.
- On `TerminatedByTruncated` (`finish_reason=length`, no tool call), each of up to `maxTruncationRetries` (2) retries streams its **half-sentence truncated text** to the client; the wind-down pass then streams the real answer on top.
- The client concatenates all deltas and ends up with `[garbage #1] + [garbage #2] + [wind-down answer]`, directly contradicting the code's stated intent ("do not treat a half-sentence as the final answer") — the partials are already on the wire and cannot be recalled.
- Same divergence for `TerminatedByMaxIter` / `EmptyFinish` / `Deadline` / `RepairExhausted`: any narration on intermediate tool-call turns is delivered, then wind-down is appended.
- **Net effect:** the streaming client's accumulated text ≠ the persisted/buffered `response.Content` (wind-down only). A `Chat` caller and a `Stream` caller get materially different answers for the same run.
- Note: the `bufferedStreamSink` path (output guardrail active) is NOT affected — it sends content exactly once in `finish` (`eino_sink.go:249-259`). The bug is specific to the plain `streamSink`.

### #2 — `msg == nil` after tool rounds falls through to natural finish and delivers an empty answer
- **Where:** `framework/eino_toolloop.go:185-189`
- When `generate` returns `msg == nil`, the loop takes `naturalFinish` with empty content.
- If this happens after one or more tool rounds, tools have already executed but the final answer is empty — the user gets a blank reply instead of the wind-down fallback.
- `msg == nil` should be treated as an abnormal termination that triggers wind-down, not as a normal natural finish.

---

## Medium

### #3 — Token budget counts "cost" as "context size" and double-counts history each iteration
- **Where:** `framework/eino_toolloop.go:182` (`res.tokensUsed += usageTotalTokens(msg)`), pre-check `:162`, post-check `:253`
- Each iteration's `TotalTokens` (prompt + completion) is accumulated into `res.tokensUsed`.
- Because `TotalTokens` already grows with the conversation (it includes prior history), summing it per iteration re-counts the same context repeatedly, so `tokensUsed` far exceeds real spend and `tokenBudget` trips **early**.
- On long conversations the budget is exhausted prematurely, forcing frequent wind-down.

### #4 — Token budget is silently disabled when provider usage is absent
- **Where:** `framework/eino_toolloop.go:67-72` (`usageTotalTokens`)
- `usageTotalTokens` returns 0 when `ResponseMeta` / `Usage` is nil.
- Any provider that does not populate usage → `res.tokensUsed` stays 0 → the `tokenBudget` termination condition is **silently disabled**, leaving only maxIter / wallClock.
- Confirmed working only for the in-repo OpenAI client (`stream_options.include_usage=true`); ark / other wired providers must be verified individually.
- Prefer a capability probe at assembly time, or an explicit warning on nil usage, over a silent downgrade.

### #5 — Streaming path drops the `terminated_by` / `partial` end metadata
- **Where:** `internal/service/agent/registry_orchestrator.go:161-208`
- `streamSink.finish` packs `terminated_by` / `partial` / `max_reached` / `iterations` into the end chunk's `Metadata` (`eino_sink.go:167-181`), but the orchestrator's `switch event` handles only `tool_call` / `tool_result` / `error` — there is **no `case "end"`**.
- The streaming client therefore never learns the answer is a truncated/limit-hit degraded result.
- Combined with #1, a streaming user receives concatenated partial + wind-down text with **no degradation signal**, whereas the buffered `Response.Metadata` carries all of it.

### #6 — The malformed-args branch never feeds `guard.recordFailure`, blinding the self-repair guard to that failure mode
- **Where:** `framework/eino_tooling.go:41-53`
- When tool-argument JSON fails to parse, the code synthesizes an error observation and returns **without** calling `guard.recordFailure` — while the normal execution paths (`:95`, `:99`, `:103`) all record.
- Consequence: if the model repeatedly emits the same malformed-argument tool call, `failureGuard` never counts it, so neither `replanThreshold` (2) nor `windDownThreshold` (4) fires — exactly the loop the guard is meant to break.
- (The 1:1 `tool_call ↔ tool message` pairing itself is correctly preserved.)

### #7 — Handler async persistence is fire-and-forget and races with a reconnecting client
- **Where:** `internal/handler/agent_handler.go:~608` (`Text2SQLStream`; `KnowledgeStream ~900-1020` has the same shape)
- After the client disconnects, a detached goroutine with `ctx := context.Background()` persists the assistant message with no sync and no completion signal.
- The write races with a client that immediately reconnects to fetch history — it may read stale or empty state.
- The write also has no retry and no alert on failure.

---

## Low

### #8 — `delegate_parallel` spawns all goroutines before acquiring the semaphore, with no bound on input size
- **Where:** `framework/collab_parallel.go:92-121`
- All `N = len(delegations)` goroutines are spawned immediately; the semaphore is acquired **inside** each goroutine (`:97`). `maxConcurrency` bounds concurrent `executeDelegation`, not the goroutine count.
- There is no cap on `len(delegations)` beyond the empty check (`:82`). An LLM emitting a huge `delegations` array spawns that many goroutines at once, each also holding a `parentCollab.Clone()`.
- Add a hard upper bound on the input length before the spawn loop.

### #9 — `delegate_parallel` has no ctx-cancel short-circuit
- **Where:** `framework/collab_parallel.go:122`
- `wg.Wait()` blocks until all children finish; on client disconnect the tool call cannot return early and relies entirely on each `executeDelegation` honoring ctx.

### #10 — `delegate_parallel` produces a malformed result entry on panic
- **Where:** `framework/collab_parallel.go:94-120`
- `safego.Recover` swallows a panic in `executeDelegation`, but `results[idx]` is assigned **after** the call (`:109-119`), so a panicked slot keeps its zero value — `Status == ""` (neither `ok` nor `failed`), empty `AgentName` / `Goal`.
- The writeback loop counts it as failed (`!= "ok"`), but the serialized `results` entry is blank/inconsistent.
- Pre-fill `AgentName` / `Goal` before the spawn and have the recover write back `failed`.

### #11 — Framework `registry.Register` stores a nil placeholder while `Get` / `GetEinoTool` do not nil-guard
- **Where:** `framework/registry.go:45-54` (`r.tools[id] = nil // Placeholder`, `enabled=true`)
- The domain `Register` stores a nil placeholder yet marks it `enabled=true`; `Get` / `GetEinoTool` do not nil-guard (while `GetTools` / `GetToolsByNames` do, via `&& einoTool != nil`).
- Production uses `RegisterEinoTool`, so this is latent/low severity — but any caller taking the domain `Register` + `Get` path hits a nil deref.

---

## Needs Audit (not yet confirmed — dedicated check recommended)

### #A — Does `normalizeContext` folding/masking always preserve the `tool_call ↔ tool` 1:1 pairing?
- **Where:** `framework/eino_context_fold.go`
- Across the three-level compression (per-message capping / whole-conversation fold — Level A masking + Level B summary / stale-reasoning eviction), the cut must land on a turn head (non-Tool message) to keep `assistant.tool_calls ↔ tool message` paired; otherwise the provider returns 400.
- `recentTailCut` / `maskOldObservations` appear designed to preserve this, but it was not proven from the four main-loop files.
- Recommend a dedicated pairing-invariant audit across fold + mask + evict, with a unit test covering the case where a cut would otherwise land on a Tool message.

---

## Ruled Out (initial suspicions disproven)

- `failureGuard.recordSuccess` prefix collision — **safe**: the `tool + ":"` delimiter isolates cross-tool prefixes.
- Wind-down with `a.toolModel == nil` — **unreachable**: `hasTools` guarantees `toolModel != nil`.
