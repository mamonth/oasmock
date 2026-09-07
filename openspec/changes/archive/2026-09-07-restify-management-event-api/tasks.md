## 1. W1 — `POST /_mock/events`: drop `type`, rename `event` → `name`

- [x] 1.1 **Red**: update unit tests to pin the new contract — `internal/server/fire_event_endpoint_test.go`, `events_endpoint_test.go`, `manage_stream_test.go`, `management_async_lifecycle_test.go`, `event_delivery_test.go`, `add_example_runtime_test.go` post `{"name": "levelUp", ...}` and expect 200; add a case that a body using `"event"`/`"type"` (missing `name`) returns 400; remove the obsolete `type:"explode"` → 400 test (RS.MAPI.32 reworded); verify the new assertions fail against the current handler
- [x] 1.2 **Green**: in `internal/server/fire_event.go` drop `Type` from `fireEventRequest` and rename `Event` to `Name` (`json:"name"`); remove the missing/unsupported-`type` branches and error strings; validate `name` (`"missing required field 'name'"`); fire `req.Name`; respond `{"success": true, "name": req.Name}`; verify 1.1 passes and `go test ./internal/server/ -run FireEvent` is green
- [x] 1.3 Migrate remaining fixture calls: `legacy_route_teardown_test.go`, `test/asyncapi/management-api/management_api_test.go` (lines sending `type`/`event`), and any docs-embedded request bodies; verify `go test ./internal/server/ ./test/asyncapi/...` passes

## 2. W2 — async surface: `POST /_mock/async/messages` + `DELETE /_mock/async/consumers/{connectionId}`

- [x] 2.1 **Red**: update path references across tests from `POST /_mock/async/push` to `POST /_mock/async/messages` and from `POST /_mock/async/disconnect` (body `connectionId`) to `DELETE /_mock/async/consumers/{connectionId}` — `internal/server/management_async_test.go`, `management_async_lifecycle_test.go`, `legacy_route_teardown_test.go`, `test/asyncapi/management-api/management_api_test.go`; add disconnect coverage for query params `code`/`reason`/`abrupt` (RS.AMG.14-17); verify they fail (old routes still registered / new ones 404/405)
- [x] 2.2 **Green**: in `internal/server/server_routes.go` `registerManagementRoutes` register `r.Post("/_mock/async/messages", s.handleAsyncMessage)` and `r.Delete("/_mock/async/consumers/{connectionId}", s.handleAsyncDisconnect)`; verify the route registry matches (spec-sync route test)
- [x] 2.3 **Green**: in `internal/server/management_async.go` rename `handleAsyncPush`→`handleAsyncMessage`, `asyncPushRequest`→`asyncMessageRequest`, `decodeAsyncPush`→`decodeAsyncMessage`, and route-aware helpers; rework `handleAsyncDisconnect` to read `connectionId` via `chi.URLParam` and `code`/`reason`/`abrupt` from query (no body decode); keep `disconnectWS` close/abort semantics; verify 2.1 passes
- [x] 2.4 Update integration coverage in `test/asyncapi/management-api/management_api_test.go` for push and disconnect against the new paths/params; verify the full `make test-unit` profile passes

## 3. Contract sync

- [x] 3.1 **OpenAPI**: update `api/openapi.yaml` — `/events`: `FireEventRequest` `required: [name]`, drop `type`, `AsyncActionResponse.event` → `name`; rename path `/async/push` → `/async/messages` (operationId `postMessage`, schema `MessagesRequest`); replace `/async/disconnect` with `DELETE /async/consumers/{connectionId}` + `code`/`reason`/`abrupt` query parameters and remove the `DisconnectRequest` body schema; verify `TestControlAPISpecSync_OpenAPI` and `TestControlAPISpecSync_ErrorResponses` pass (route + error-body parity)
- [x] 3.2 **AsyncAPI spec**: confirm `api/asyncapi.yaml` stream envelope schemas are unaffected (`name` already used) and `TestControlAPISpecSync_EnvelopeFields`/`CrossFormat`/`AsyncAPI` pass; verify `go test ./internal/server/ -run ControlAPISpecSync`

## 4. Docs and changelog

- [x] 4.1 Update `README.md` management API list — `POST /_mock/events` fires with `{name, payload, delay, global}` (drop "type discriminator" wording), `POST /_mock/async/messages`, `DELETE /_mock/async/consumers/{connectionId}` with `?code=&reason=&abrupt=`; verify the section reads consistently with `api/openapi.yaml`
- [x] 4.2 Update `docs/architecture.md` "Async mocking management API" wording — type-discriminated fire → `name`-based fire, `messages` path, DELETE-consumer disconnect; verify no stale `/async/push|disconnect` references remain in docs
- [x] 4.3 Add a `CHANGELOG.md` entry under the next release block noting the **BREAKING** shape changes (`/events` drops `type` and renames `event`→`name`; `/async/push` → `/async/messages`; `/async/disconnect` → `DELETE /async/consumers/{connectionId}`); verify the entry matches the implemented surface

## 5. Full verification

- [x] 5.1 Run `make lint` and `make test-unit`; verify all pass with no drift between router, `api/openapi.yaml`, and tests
- [x] 5.2 Run the project build (`go build ./...`) and confirm the management surface (fire, push, disconnect, consumers) works end-to-end against the new contract