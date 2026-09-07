# Design: Cleanup deprecated management endpoints and `x-send-events`

## Context

The async-management-api-extensions change shipped the unified protocol-neutral management surface (`/_mock/async/*`, `/_mock/events`, `/_mock/examples`, `/_mock/stream`) alongside one-release compatibility: deprecated `/_mock/ws/{push,consumers,disconnect}` aliases, a deprecated `/_mock/events/fire` alias, `410 Gone` stubs for the removed `/_mock/ws/schedule{,/{pushId}}`, and a load-time `x-send-events` → `x-mock-match`/`x-mock-interval` mapping shim (see proposal.md — Why). The window has elapsed; those remnants are now the only dead weight in the surface.

The canonical requirements live in `openspec/specs/` (`event-driver`, `management-api`, `asyncapi-management`, `extensions`). Notably the deprecated alias endpoints were never spec'd as requirements — they exist only in `api/openapi.yaml`, the router, and a handful of tests/docs. So the endpoint teardown is behavior-neutral at the spec level; only the `x-send-events` subscription vocabulary is a spec-surface change (delta in `specs/event-driver/`).

## Goals / Non-Goals

**Goals:**
- Canonical-only management routing: leave `registerManagementRoutes` with exactly the `/_mock/async/*`, `/_mock/events`, `/_mock/examples`, `/_mock/stream`, `/_mock/requests` surface.
- Legacy paths (`/_mock/ws/*`, `/_mock/events/fire`) return the same plain 404 as any unknown route.
- `x-send-events` is no longer interpreted: the loader classifies message examples purely by `x-mock-match`/`x-mock-interval`, while a key still present in a spec loads without error.
- OpenAPI spec, docs, and tests no longer advertise deprecated vocabulary.

**Non-Goals:**
- Keep the `x-mock-params-match` alias handling (`extensions`, `mock-server-core` RS.MSC.8, `asyncapi-templating` RS.ATM.8) — a separate, still-supported deprecation.
- Change the canonical `/_mock/async/*`, `/_mock/events` (including its `type` discriminator), examples, or stream behavior.
- Add load-time validation/rejection for specs that still carry `x-send-events` (decision D2: silent ignore).
- Migrate or re-route any legacy path to a canonical one with a redirect — removal is final.

## Decisions

### D1: Remove the alias and 410-stub route registrations; legacy paths fall through to the default 404

Delete from `internal/server/server_routes.go::registerManagementRoutes`:
- `POST /_mock/events/fire` (→ `handleFireEventLegacy`)
- `POST /_mock/ws/push` (→ `handleAsyncPush`)
- `GET /_mock/ws/consumers` (→ `handleAsyncConsumers`)
- `POST /_mock/ws/disconnect` (→ `handleAsyncDisconnect`)
- `POST /_mock/ws/schedule` and `DELETE /_mock/ws/schedule/{pushId}` (→ `handleGoneSchedule`)

chi's not-found handling already returns 404 for unregistered paths, so no `404` handler needs to be added — removing the routes *is* the new behavior. Delete `handleGoneSchedule` in `internal/server/management_async.go` and `handleFireEventLegacy` in `internal/server/fire_event.go` (keeping `handleEvents` + `dispatchFireEvent`, which now only ever serve the canonical `fireEventRequest` with a required `type`).

**Alternatives considered:** keeping the 410 stubs so old schedule clients still get migration guidance. Rejected — the user chose full teardown; a documented one-release window was the migration vehicle, and `DELETE /_mock/examples/{id}` was already the pointer. Keeping stubs would leave half the dead surface alive.

### D2: Remove the `x-send-events` mapping shim; ignore the key at load

Delete `internal/server/send_events.go` (`SendEvent`, `parseSendEvents`, `xSendEventsKey`). In `internal/server/event_server.go`:
- `derivedExamples` currently expands one example into N derived message specs (one per `x-send-events` entry). Since the key is ignored, the derivation degenerates to a passthrough — delete `derivedExamples` and `cloneExtensions`, and have `classifyMessageExample` classify the example directly via `extensions.ClassifyTrigger`.
- Remove the two `slog.Warn("x-send-events is deprecated; ...")` branches and the `{on: cron}` without-wait error.
- The generic capture of `x-*` keys in `internal/asyncapi` remains untouched (the vendored parser still surfaces the key in `Example.Extensions`; nothing downstream reads it). This matches "silently ignore": the spec loads, the key rides along in the model, and classification ignores it.

**Alternatives considered:** reject-at-load for specs that still use `x-send-events`. Rejected by the user — silent ignore keeps lingering (already-migrated) specs loadable, consistent with how other unknown vendor extensions are tolerated. This difference is the whole reason the spec delta REMOVES the old mapping requirement rather than replacing it with a stricter one.

### D3: Prune `api/openapi.yaml` to the canonical surface

Remove the paths `/events/fire`, `/ws/push`, `/ws/consumers`, `/ws/schedule`, `/ws/schedule/{pushId}`, `/ws/disconnect` and their `operationId`s. Remove the now-unused `components.schemas.LegacyFireEventRequest`, `components.schemas.ScheduleRequest`, `components.responses.GoneExamples`, `components.responses.GoneDelete`. Keep `PushRequest`, `DisconnectRequest`, `ConsumersResponse`, and the envelope schemas (still used by canonical endpoints).

**Constraint:** `internal/server/control_api_spec_sync_test.go::TestControlAPISpecSync_ErrorResponses` asserts at least 15 error responses across all management operations. Removing the deprecated paths removes 7 (events/fire 400; ws/push 400+404; ws/schedule 410; ws/schedule/{pushId} 410; ws/disconnect 400+404) → lowers the count to 9. The test's `require.GreaterOrEqual(..., 15)` threshold must be lowered (e.g. to 9) in the same change that prunes the spec. `TestControlAPISpecSync_OpenAPI` auto-corrects since it requires both directions to match after the edit; its comment referencing the deprecated aliases must be updated.

### D4: Update tests in the same change

- Delete: `internal/server/send_events_test.go`, `internal/server/x_send_events_shim_test.go`, `internal/server/management_async_aliases_test.go`.
- Update: `async_state_test.go` (drops its `derivedExamples`/`x-send-events` case), `fire_event_endpoint_test.go` and `event_integration_test.go` (migrate fixtures to `x-mock-match`/`x-mock-interval`), `internal/asyncapi/parse_test.go` (drop the dedicated `x-send-events` capture assertion), `test/asyncapi/management-api/management_api_test.go` (remove the deprecated-alias and x-send-events-shim scenarios), `test/_shared/resources/asyncapi-management.yaml` (migrate its `x-send-events` example), `control_api_spec_sync_test.go`.
- Keep the deprecated-alias **runtime** coverage gone: the integration test that hits `/_mock/ws/push` etc. no longer expects 200/410 — it should assert plain 404 for those paths to pin D1.

### D5: Docs

`README.md` (remove the "legacy aliases still work / schedule answers 410" paragraph), `docs/architecture.md` (lines ~235, ~350, ~352: drop `x-send-events` and the alias/410 mentions), `docs/extensions.md` (delete the `x-send-events (deprecated)` section, keep the unified `x-mock-match`/`x-mock-interval`/`x-mock-delay` documentation, phrase the timing extensions as the sole recurrence mechanism), and a `CHANGELOG.md` entry announcing the removals.

## Risks / Trade-offs

- [Existing users still on legacy `/_mock/ws/*` or `/_mock/events/fire` break] → Mitigated: one-release deprecation already signposted the canonical paths; CHANGELOG documents the removal; error responses (when they hit a removed path) are now plain 404 — loud and unambiguous.
- [Specs still carrying `x-send-events` silently lose event-driven behavior] → By design (D2), consistent with the "silently ignored" decision and the spec-delta migration note; verbose users already saw the deprecation warning for a release.
- [Spec-sync test regresses when the OpenAPI surface shrinks] → D3 explicitly lowers the error-response threshold and relies on the bidirectional route-doc check, keeping docs and router in lockstep.
- [`derivedExamples` removal changes classification structure for `x-mock-match`-only examples] → Low: the passthrough path is exactly the current `len(events)==0` branch; the `classifyMessageExample` loop shape stays, just over one spec.

## Migration Plan

1. Code: D1 (routing + handlers), D2 (send_events deletion + event_server simplification).
2. Tests: D4 (delete/update) in the same commit so the suite stays green.
3. Contract: D3 (openapi.yaml prune + threshold).
4. Docs: D5.
5. Rollback: revert the change commit — the deprecated routes, shim and aliases are restored verbatim; no data migration is involved (no persistence).

## Open Questions

None — removal scope (whether to keep the 410 stubs) and the `x-send-events` post-removal behavior (silently ignore) were resolved with the user before planning.