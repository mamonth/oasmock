# Tasks: Cleanup deprecated management endpoints and `x-send-events`

TDD workflow for every item below (per AGENTS.md design-first/TDD rule):
1. **Red** — write or edit the test for the parent/consumer first (mocking the new/edited interface), prove it fails.
2. **Red** — add the interface-level unit test (mock its dependencies), prove it fails.
3. **Green** — implement the interface until all tests pass (`go test ./...`).
4. **Refactor** — keep cognitive complexity low, cohesion high; re-run tests + lint.

Related spec scenarios: RS.EVT.5, RS.EVT.18 (removed), RS.MAPI.22, RS.MAPI.32.

## 1. Route teardown (deprecated aliases + schedule 410 stubs)

- [x] 1.1 **Red**: add failing unit/integration tests pinning that the legacy paths `POST /_mock/ws/push`, `GET /_mock/ws/consumers`, `POST /_mock/ws/disconnect`, `POST /_mock/ws/schedule`, `DELETE /_mock/ws/schedule/{pushId}` and `POST /_mock/events/fire` answer HTTP 404 (any existing alias/410 expectation already there is flipped); verify they fail (routes currently register)
- [x] 1.2 **Green**: in `internal/server/server_routes.go` `registerManagementRoutes`, remove the deprecated alias routes and the `/_mock/ws/schedule{,/{pushId}}` 410-stub registrations; drop `handleGoneSchedule` in `internal/server/management_async.go` and `handleFireEventLegacy` in `internal/server/fire_event.go` (keeping `handleEvents`/`dispatchFireEvent`); verify 1.1 passes and `POST /_mock/events` still requires the `type` discriminator (RS.MAPI.22, RS.MAPI.32)
- [x] 1.3 **Red**: write failing tests that the canonical surface is unchanged — `POST /_mock/events` (with/without `type`), `/_mock/async/{push,consumers,disconnect}`, `/_mock/examples`, `DELETE /_mock/examples/{id}`, `/_mock/stream` all still work as documented; verify the canonical-behavior subset passes after 1.2 (they already did — this pins no regression)
- [x] 1.4 **Refactor**: delete `internal/server/management_async_aliases_test.go` and remove any now-dead helpers/imports it isolated; verify `go test ./internal/server/` and `go vet ./...` pass

## 2. Remove the `x-send-events` mapping shim (silently ignore the key)

- [x] 2.1 **Red**: write failing unit tests that a message example carrying `x-send-events: [{on: <name>}]` or `[{on: cron, wait: N}]` is classified by its trigger extensions alone (`x-mock-match`/`x-mock-interval`) — i.e. the key produces no subscription, no interval job and no load error when neither extension is present (RS.EVT.18 now removed); verify they fail against the current shim
- [x] 2.2 **Green**: delete `internal/server/send_events.go` (`SendEvent`, `parseSendEvents`, `xSendEventsKey`); in `internal/server/event_server.go` drop the `derivedExamples`/`cloneExtensions` derivation and the two deprecation `slog.Warn` branches plus the `{on: cron}` missing-wait error, letting `classifyMessageExample` classify the example directly via `extensions.ClassifyTrigger`; verify 2.1 passes and the mapping-shim-focused unit tests are removed
- [x] 2.3 **Red/Green**: update `internal/server/async_state_test.go` (drop the `derivedExamples`/`x-send-events` case) and `internal/asyncapi/parse_test.go` (drop the dedicated `x-send-events` capture assertion — generic `x-*` capture remains); delete `internal/server/send_events_test.go` and `internal/server/x_send_events_shim_test.go`; verify `go test ./internal/server/` and `go test ./internal/asyncapi/` pass
- [x] 2.4 **Integration**: update `test/asyncapi/management-api/management_api_test.go` (remove the legacy `x-send-events` shim scenario and the deprecated-alias scenario; migrate its fixture resources to `x-mock-match`/`x-mock-interval`) and `test/_shared/resources/asyncapi-management.yaml` (replace the `x-send-events` example with the unified form); verify `make test-integration` passes (skips when `-short`)

## 3. Contract and docs cleanup

- [x] 3.1 **Contract**: prune `api/openapi.yaml` — delete paths `/events/fire`, `/ws/push`, `/ws/consumers`, `/ws/schedule`, `/ws/schedule/{pushId}`, `/ws/disconnect` and remove unused `LegacyFireEventRequest`, `ScheduleRequest` schemas and `GoneExamples`/`GoneDelete` responses; lower the `TestControlAPISpecSync_ErrorResponses` threshold from 15 to 9 in `internal/server/control_api_spec_sync_test.go` and update its comment to drop the deprecated-alias/410 wording; verify `make test-unit` and the spec-sync tests pass
- [x] 3.2 **Docs**: update `README.md` (remove the "legacy aliases still work / schedule answers 410" paragraph), `docs/architecture.md` (drop `x-send-events` and the alias/410 mentions in the extension list, event-broker paragraph and management-API line), and `docs/extensions.md` (delete the `x-send-events (deprecated)` section, phrase timing recurrence solely via `x-mock-interval`); verify the docs contain no remaining `x-send-events`/deprecated-alias claims (`grep` clean)
- [x] 3.3 **Changelog**: add a `CHANGELOG.md` entry under the next release block noting the API removals (`/_mock/ws/*`, `/_mock/events/fire`, schedule stubs) and `x-send-events` handling (silently ignored); verify the entry matches the actual changes

## 4. Final verification

- [x] 4.1 Run `go test ./...` (unit + integration) and confirm the full suite passes
- [x] 4.2 Run `make lint` (golangci-lint) and verify no new findings
- [x] 4.3 Run `python3 scripts/check_test_headers.py` and confirm all remaining/updated tests carry Gherkin Scenario headers referencing the related spec codes; confirm no spec still references the removed `RS.EVT.18` behavior as implemented
- [x] 4.4 Cross-check the archived `2026-09-06-async-management-api-extensions` task notes (tasks 1.2, 7.1, 8.1) to confirm every deprecated alias, 410 stub, and `x-send-events` reference introduced there is now covered by this cleanup