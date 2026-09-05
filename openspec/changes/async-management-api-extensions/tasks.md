# Tasks: Async management API extensions

TDD workflow for every item below (per AGENTS.md design-first/TDD rule):
1. **Red** — write or edit the test for the parent/consumer first (mocking the new/edited interface), prove it fails.
2. **Red** — add the interface-level unit test (mock its dependencies), prove it fails.
3. **Green** — implement the interface until all tests pass (`go test ./...`).
4. **Refactor** — keep cognitive complexity low, cohesion high; re-run tests + lint.

## 1. Route re-organization (renames, `/events`, schedule removal)

- [x] 1.1 **Red**: update path references in `internal/server/management_async_test.go`, `internal/server/management_async_lifecycle_test.go`, and `internal/server/fire_event_endpoint_test.go` from `/_mock/ws/*` to `/_mock/async/*` and from `/_mock/events/fire` to `/_mock/events`; verify `go test ./internal/server/` fails (routes 404/405)
- [x] 1.2 **Red**: add failing tests that pin the deprecated aliases (`/_mock/ws/push`, `/_mock/ws/consumers`, `/_mock/ws/disconnect`, `/_mock/events/fire` still work) and that `/_mock/ws/schedule` + `/_mock/ws/schedule/{pushId}` answer `410 Gone` with the `POST /_mock/examples` guidance body; verify they fail
- [x] 1.3 **Green**: in `internal/server/server.go` `registerManagementRoutes`, register the canonical `/_mock/async/*` paths plus the deprecated alias routes and the `410` answers for schedule in `internal/server/management_async.go`; verify 1.1 and 1.2 tests pass
- [x] 1.4 **Red**: write unit tests for `/events` that `type` is required and `"fire"` is the only accepted value (missing/unknown → 400, `type:"fire"` reproduces previous fire behavior), asserting payload templating and delay semantics are unchanged (RS.MAPI.22-23, RS.MAPI.32); verify they fail
- [x] 1.5 **Green**: replace `handleFireEvent` with `handleEvents` in `internal/server/fire_event.go` adding the `type` discriminator and reusing the existing fire path; verify 1.4 tests and the retained alias test pass, then run lint/typecheck

## 2. Unified match pipeline (event context, timing, recipient partition)

- [x] 2.1 **Red**: write failing `internal/runtime` tests that `EventSource` exposes `{$event.name}` (identity) and `{$event.data}` (whole payload) while `{$event.<field>}` payload access is unchanged (RS.EXT.18-19); verify they fail
- [x] 2.2 **Green**: extend `EventSource` (`internal/runtime/expression.go`) with reserved `name`/`data` accessors without mutating the payload; verify 2.1 passes and existing event-expression tests still pass
- [x] 2.3 **Red**: write failing `internal/runtime` tests for a new `ConnectionSource` resolving `{$connection.id}`, `{$connection.channel}`, `{$connection.query.<key>}`, `{$connection.header.<key>}` (RS.EXT.27); verify they fail
- [x] 2.4 **Green**: add `ConnectionSource` and register it in the async-driven evaluators; verify 2.3 passes
- [x] 2.5 **Red**: write failing `internal/extensions` tests — matching against an event context (identity + payload, literal and JSON-schema; RS.EXT.18-19, RS.EXT.21), and fail-closed with a verbose warning when the context is unavailable (RS.EXT.29); verify they fail
- [x] 2.6 **Green**: extend the match evaluation (`internal/extensions/match.go` / evaluator construction in `internal/server/engine.go`) to accept event/connection contexts; verify 2.5 passes
- [x] 2.7 **Red**: write failing `internal/extensions` tests for the per-connection condition partition helper — conditions referencing `{$connection.*}` on either side land in the connection bucket, others in the common bucket; empty connection bucket → broadcast fast path (RS.EXT.24-25); verify they fail
- [x] 2.8 **Green**: implement the partition helper; verify 2.7 passes and existing `match_test.go`/`parity_test.go` stay green
- [x] 2.9 **Red**: write failing tests parsing/decorating `x-mock-match` timing siblings — `x-mock-interval` (positive ms) marks a periodically driven example, `x-mock-delay` (ms) delays event emission (RS.EXT.22-23); verify they fail
- [x] 2.10 **Green**: add `x-mock-interval`/`x-mock-delay` as example extensions consumed by the classification path; verify 2.9 passes

## 3. Trigger classification, broker, scheduler, built-ins

- [x] 3.1 **Red**: write failing unit tests that classify a spec example at load — event-driven iff `x-mock-match` references `{$event.*}`, periodically driven iff `x-mock-interval`, mixed match contexts or dual triggers rejected with a clear load error (RS.EXT.20, RS.EXT.28); verify they fail
- [x] 3.2 **Green**: implement trigger classification in the AsyncAPI load/registration path (`internal/server/event_server.go`, replacing `collectSchemaSubscriptions`/`messageDeliverable` grouping) plus the `x-send-events` mapping shim with a verbose deprecation note (RS.EVT.18); verify 3.1 and existing loader tests pass
- [x] 3.3 **Red**: write failing broker tests — event-driven examples registered by identity + schema scope resolve only for their schema (and globally), and a cheap `hasSubscribers(name, schema)` returns false when none; verify they fail
- [x] 3.4 **Green**: refit `eventBroker` (`internal/server/event_broker.go`) to register/reoslve match-identified examples; verify 3.3 passes
- [x] 3.5 **Red**: write failing scheduler tests — a per-example interval job delivers at its cadence and stops on cancel/shutdown; verify they fail
- [x] 3.6 **Green**: generalize `internal/server/scheduler.go` into per-example `{id, interval, deliver func()}` jobs wired from classification and runtime `interval`, with shutdown coverage (design D4); verify 3.5 passes
- [x] 3.7 **Red**: write failing tests for built-in firing — `connect` fires schema-local on consumer connection with the single connecting connection as recipient, `receive` fires schema-local on inbound traffic carrying the inbound message in the event context, both gated on `hasSubscribers` (RS.EVT.9, RS.EVT.11, RS.EXT.26); verify they fail
- [x] 3.8 **Green**: wire `connect` (ws adapter + `signalr_hub.go` connect) and `receive` (ws adapter read loop + SignalR dispatch) hooks; verify 3.7 passes
- [x] 3.9 **Red**: write failing integration-style unit tests for fire-time selection — the shared `SelectAsyncExample` pipeline runs against the event context and delivers through the two-phase partition (targeted `'{$connection.id}': '{$event.connectionId}'`, broadcast fast path) (RS.EVT.19, RS.EXT.24-25); verify they fail
- [x] 3.10 **Green**: run selection + delivery through the partition (design D6) with connection metadata captured at upgrade (`wsConnection`, SignalR connection); verify 3.9 passes

## 4. Unified example injection (`/examples`)

- [x] 4.1 **Red**: write failing validation tests asserting `POST /_mock/examples` rejects `path`+`channel`, `path` with `match`/`interval` but no async target, `interval` alongside an event `match`, and a non-positive `interval`, with 400 (RS.MAPI.27-29), while existing valid request shapes still pass; verify they fail
- [x] 4.2 **Green**: in `internal/server/server_management.go`, replace `addExampleRequestSchema` with the `oneOf` sync/async two-branch schema and extend the decoded struct with `match`/`interval`/`delay` plus the single-trigger check; verify 4.1 tests pass and existing add-example tests still pass
- [x] 4.3 **Red**: write failing HTTP tests — `handleAddExample` with `match` (event) or `interval` (recurring) registers a live async-driven example returning `{success, id}` and delivers on fire/at cadence with per-connection targeting (RS.MAPI.24-26, RS.MAPI.33), while the no-`match`/`interval` path keeps the inbound-reply registry behavior; verify they fail
- [x] 4.4 **Green**: route `handleAddExample` through the match/interval classification path from section 3; verify 4.3 tests pass
- [x] 4.5 **Red**: write failing tests for `DELETE /_mock/examples/{exampleId}` — removes a registry example, cancels an interval example (no further deliveries), returns 404 for an unknown id (RS.MAPI.30-31, RS.MAPI.25); verify they fail
- [x] 4.6 **Green**: implement `DELETE /_mock/examples/{exampleId}` via the `Server` registry map from 3.2/3.6; verify 4.5 tests pass

## 5. Consumers get-all and push regression

- [x] 5.1 **Red**: write failing unit tests for `handleAsyncConsumers` — `channel` omitted returns the flat union of all raw-ws connections (across channels) and all hub channels' open streams, empty when none (RS.AMG.22, RS.AMG.8-9); verify they fail
- [x] 5.2 **Green**: make `channel` optional in `handleAsyncConsumers` (`internal/server/management_async.go`); verify 5.1 tests pass and the existing `GET /_mock/async/consumers?channel=/alerts` test still passes
- [x] 5.3 **Red**: add an HTTP regression test asserting one-shot `POST /_mock/async/push` (immediate, delayed, targeted, broadcast) is unchanged after the rename; pin it to the new canonical path and verify it fails only on the old path there
- [x] 5.4 **Green**: confirm push handlers are intact on the canonical path; verify 5.3 passes

## 6. Management WebSocket stream (`/_mock/stream`)

- [x] 6.1 **Red**: write failing tests for `manage_ws.go` — a ws client connects and receives envelopes, a plain HTTP GET returns 405 (RS.AMG.28), and connect-time `events`/`channels` filters (comma-separated, `*` glob) are parsed; verify they fail
- [x] 6.2 **Green**: implement `internal/server/manage_ws.go` (upgrade with `Connection: Upgrade` check, filter parsing, ping/pong loop, dedicated `manageWSRegistry` excluded from `/_mock/async/consumers`); verify 6.1 passes
- [x] 6.3 **Red**: write failing tests for an `eventBus` observer hook emitting `event` and `push` envelopes filtered per-subscriber (name/payload/schema/global) on `fire` and `deliver` (RS.AMG.24-25); verify they fail
- [x] 6.4 **Green**: add the observer hook to `eventBus` (`event_broker.go`/`event_server.go`) and subscribe `/_mock/stream` connections to it; verify 6.3 passes
- [x] 6.5 **Red**: write failing tests that consumer connect/disconnect (ws adapter + SignalR) and interval start/stop (from scheduler jobs) emit `consumer`/`schedule` envelopes (RS.AMG.26-27); verify they fail
- [x] 6.6 **Green**: wire lifecycle and scheduler start/stop hooks into envelope emission; verify 6.5 passes

## 7. OpenAPI contract and docs

- [x] 7.1 Update `api/openapi.yaml` — `/_mock/async/{push,consumers,disconnect}`, `/events` with `EventRequest.type` enum, `/examples` `match`/`interval`/`delay` + `DELETE /examples/{exampleId}`, `/stream` prose + envelope schemas in `components`, `consumers.channel` optional, deprecated markers on `/ws/*` and `/events/fire`, `410` description on `/ws/schedule*`; verify the file parses and any project openapi lint/validation used in CI passes
- [x] 7.2 Update `docs/extensions.md` (event-context matching, `{$connection.*}` partition, `x-mock-interval`/`x-mock-delay`, `x-send-events` deprecation), `docs/architecture.md` (async-management + event sections), and add a `CHANGELOG.md` entry; verify docs build/lint passes

## 8. Integration verification (black-box, TDD as one red→green cycle)

- [x] 8.1 **Red**: add integration tests under `test/asyncapi/management-api/` (skip on `testing.Short()`) covering — runtime `match` on `{$event.name}` fired via `POST /_mock/events`; runtime `interval` recurrence then stop via `DELETE /examples/{id}`; per-connection targeting (`{$connection.id}`); `connect`/`receive` built-ins (spec + runtime); `x-send-events` shim still emitting; a `/_mock/stream` subscriber with filters receiving event/push/consumer/schedule envelopes; schedule alias `410`; and the deprecated alias still working; verify they fail against the pre-change surface
- [x] 8.2 **Green**: run the full suite against the implemented server so 8.1 integration tests pass along with all earlier unit tests — `go test ./...`, project lint/typecheck targets from the `Makefile`, coverage threshold (≥70%) maintained, and `openspec validate async-management-api-extensions` still passes