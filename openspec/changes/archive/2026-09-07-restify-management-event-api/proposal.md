## Why

The management HTTP API mixes REST-style noun resources (`/examples`, `/requests`, `/async/consumers`, `/stream`) with action/function residues: `POST /_mock/events` carries a one-of-one `type: "fire"` discriminator, `POST /_mock/async/disconnect` is a verb path with its target in the body, and the fire-event DTO names its identity `event` while everywhere else in the project (templates `{$event.name}`, stream envelopes, broker, docs) it is `name`. This doubles the contract vocabulary and signals RPC dispatch where the design is resource-based.

## What Changes

- **BREAKING** `POST /_mock/events`: remove the required `type` discriminator (`"fire"` for V1 — the endpoint *is* the fire action) and rename `event` to `name`. The request becomes `{name, payload, delay, global}` and the success response becomes `{success, name}`. Missing `name` → 400; the "unsupported event type" branch (RS.MAPI.32) is removed.
- **BREAKING** `POST /_mock/async/push` → `POST /_mock/async/messages`: same body `{channel, connectionId, payload, delay}`, same semantics — delivering to consumers becomes a POST of a *message* resource, symmetric with `POST /_mock/events` and `POST /_mock/examples`.
- **BREAKING** `POST /_mock/async/disconnect` → `DELETE /_mock/async/consumers/{connectionId}`: the consumer id moves from the body to the path (symmetrical with `DELETE /_mock/examples/{exampleId}`); optional control data (`code`, `reason`, `abrupt`) becomes query parameters. No DELETE body.
- Align the wire vocabulary with the domain: `AsyncActionResponse.event` → `name` (fire-event endpoints only).
- Update `api/openapi.yaml` and `api/asyncapi.yaml` (if affected), `docs/architecture.md`, `README.md`, `CHANGELOG.md`, and all tests to the new paths/fields. Direct breaking cut-over, no one-release aliases (matches the recent legacy-surface cleanup precedent).

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `management-api`: the fire-event requirement changes — `POST /_mock/events` drops the `type` discriminator and renames `event` to `name` (request and response); the invalid-`type` scenario is removed.
- `asyncapi-management`: the push and connection-lifecycle requirements change — delivery targets the new `POST /_mock/async/messages` path, and force-disconnect moves to `DELETE /_mock/async/consumers/{connectionId}` with `code`/`reason`/`abrupt` query parameters.

## Impact

- `internal/server/fire_event.go` — `fireEventRequest` loses `Type`, renames `Event`→`Name`; drop the type validation/error branches; response body `{success, name}`.
- `internal/server/server_routes.go` — `registerManagementRoutes`: `POST /_mock/async/push` → `/async/messages`; `POST /_mock/async/disconnect` → `DELETE /_mock/async/consumers/{connectionId}`.
- `internal/server/management_async.go` — rename `handleAsyncPush`→`handleAsyncMessage`, `asyncPushRequest`→`asyncMessageRequest`, helpers (`decodeAsyncPush`, `evaluatePushPayload`); `handleAsyncDisconnect` reads `connectionId` from the path (chi URLParam) and `code`/`reason`/`abrupt` from query.
- `api/openapi.yaml` — `/events` (FireEventRequest required `[name]`, drop `type`), `/async/push`→`/async/messages`, `/async/disconnect`→`DELETE /async/consumers/{connectionId}` with query params; `AsyncActionResponse.event`→`name`; remove `DisconnectRequest` body schema.
- Specs: `openspec/specs/management-api/spec.md` (RS.MAPI.22/23/32), `openspec/specs/asyncapi-management/spec.md` (RS.AMG.1-7, 10-11, 14-17) get delta specs.
- Docs: `docs/architecture.md`, `README.md`, `CHANGELOG.md`.
- Tests: `fire_event_endpoint_test.go`, `events_endpoint_test.go`, `manage_stream_test.go`, `management_async_lifecycle_test.go`, `management_async_test.go`, `async_push_regression_test.go`, `event_delivery_test.go`, `legacy_route_teardown_test.go`, `add_example_runtime_test.go`, `test/asyncapi/management-api/management_api_test.go`, and spec-sync tests (`control_api_spec_sync_test.go` re-derives the surface automatically).