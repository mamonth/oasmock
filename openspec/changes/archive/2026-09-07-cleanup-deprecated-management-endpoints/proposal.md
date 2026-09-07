## Why

The deprecation window opened by the async-management-api-extensions change has elapsed: `/_mock/ws/{push,consumers,disconnect}` and `/_mock/events/fire` were kept as one-release aliases, `/_mock/ws/schedule{,/{pushId}}` answer `410 Gone` guidance stubs, and the `x-send-events` loader shim kept the legacy AsyncAPI subscription key alive. Keeping both the canonical surface and its deprecated aliases doubles the management API contract, slows the openapi.yaml and router together, and forces every new endpoint decision to account for legacy vocabulary that not even the skill docs use anymore.

## What Changes

- **BREAKING** Remove the deprecated alias endpoints `POST /_mock/ws/push`, `GET /_mock/ws/consumers`, `POST /_mock/ws/disconnect`, and `POST /_mock/events/fire`. The canonical paths (`/_mock/async/{push,consumers,disconnect}`, `POST /_mock/events`) remain the only way to reach those behaviors.
- **BREAKING** Remove the removed-surface stubs `POST /_mock/ws/schedule` and `DELETE /_mock/ws/schedule/{pushId}`. They currently answer `410 Gone` with migration guidance; after the change any legacy `/_mock/ws/*` path returns a plain 404 like any unknown route.
- **BREAKING** Remove the `x-send-events` extension property and its loader mapping shim. Specs that still carry `x-send-events` load without error but the key is silently ignored — a message example is classified solely by its `x-mock-match`/`x-mock-interval`/reply trigger, never by the legacy key.
- Remove the dead OpenAPI surface from `api/openapi.yaml`: the deprecated aliases, the schedule 410 paths, and the now-unused `LegacyFireEventRequest`/`ScheduleRequest` schemas and `GoneExamples`/`GoneDelete` responses.
- Update docs (`README.md`, `docs/architecture.md`, `docs/extensions.md`, `CHANGELOG.md`) to stop advertising the deprecated vocabulary and document the removal.
- Update tests that pinned the deprecated aliases, the `410` schedule answers, and the `x-send-events` shim; migrate the few remaining fixture usages of `x-send-events` to the unified `x-mock-match`/`x-mock-interval` form.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `event-driver`: the `x-send-events` subscription-vocabulary requirement and its load-time mapping shim are removed from the spec surface. The remaining requirements are reworded to describe event-driven emission purely via `x-mock-match` against the event context and `x-mock-interval`.

## Impact

- `internal/server/server_routes.go` — drop the deprecated alias and 410-stub route registrations from `registerManagementRoutes`; keep only the canonical `/_mock/async/*`, `/_mock/events`, `/_mock/examples`, `/_mock/stream`, `/_mock/requests` surface.
- `internal/server/fire_event.go` — remove `handleFireEventLegacy` and its `fireEventRequest` legacy path; keep `handleEvents`/`dispatchFireEvent`.
- `internal/server/management_async.go` — remove `handleGoneSchedule`.
- `internal/server/event_server.go` — remove the `x-send-events` mapping shim (`derivedExamples` degenerates to a per-example passthrough) and its deprecation warnings; delete the `x-send-events` key from the classification path.
- `internal/server/send_events.go` and `send_events_test.go`, `x_send_events_shim_test.go`, `management_async_aliases_test.go` — deleted.
- Tests: `async_state_test.go`, `fire_event_endpoint_test.go`, `event_integration_test.go`, `internal/asyncapi/parse_test.go`, `test/asyncapi/management-api/management_api_test.go`, `test/_shared/resources/asyncapi-management.yaml`, `control_api_spec_sync_test.go` — updated to drop legacy references.
- `api/openapi.yaml` — delete the deprecated alias paths, schedule 410 paths and unused schemas/responses.
- Docs: `README.md`, `docs/architecture.md`, `docs/extensions.md`, `CHANGELOG.md`.
- Specs: canonical `openspec/specs/event-driver/spec.md` loses the `x-send-events` vocabulary; a delta spec records the change.