# Design: Restify the management async surface

## Context

Today's `/_mock` surface mixes styles (see `api/openapi.yaml` and `internal/server/{fire_event,management_async,manage_ws}.go`): noun resources (`/examples`, `/requests`, `/async/consumers`, `/stream`) sit beside verb paths (`/async/push`, `/async/disconnect`) and a resource POST that smuggles an action discriminator (`POST /_mock/events` with a required `type: "fire"`). The `type` field is vestigial — it was kept when the `/events/fire` action alias was removed in the legacy-cleanup change. The fire-event DTO also names its identity `event` while the rest of the project (runtime template `{$event.name}`, `EventSource.Name`, stream envelope `name`, `eventBus.fire(name,…)`, spec language "named event") uniformly says `name`.

Motivation is in `proposal.md`.

## Goals / Non-Goals

**Goals:**
- One uniform resource model: HTTP methods act on noun-path resources (`events`, `async/messages`, `async/consumers`, `examples`, `requests`, `stream`).
- No action/function discriminators in request DTOs; a resource POST *is* the action.
- Single event-identity vocabulary (`name`) across wire contract, templates, envelopes, broker, and docs.
- Direct breaking cut-over with no one-release aliases (consistent with the 2026-09-07 legacy-cleanup precedent).

**Non-Goals:**
- Relocating `POST /_mock/events` or `/_mock/stream` under `/async/*` — reasserts design D1 of the async-management change: events and the management stream are cross-cutting and protocol-neutral, while `/async/*` is channel-operation-specific.
- Renaming `GET /_mock/stream` filter query parameters (`?events=`, `?channels=`) — query-param renaming for stylistic purity is over-complication (case-sensitive, zero behavioral gain).
- Removing `manageEnvelope.Type` / `manageConsumerEnvelope.Action` / `manageScheduleEnvelope.Action` — legitimate tagged-union discriminators on a notification channel, not RPC dispatch.
- Any change to example selection, delivery, delay, or schema semantics.

## Decisions

### D1: `POST /_mock/events` fires without a `type` discriminator
A POST to the `/events` collection *is* the fire action; the one-of-one `type: "fire"` field adds a required field with a single legal value and implies an extensible action router that does not exist.
- **Alternative considered**: keep `type` for future variants — rejected: speculative generality; RS.MAPI.32's "unsupported event type" exists only to guard it.
- **Alternative considered**: move firing to a verb path `POST /_mock/async/fire` — rejected: reintroduces the verb-style and abandons events-as-a-resource.

### D2: Fire-event identity renamed `event` → `name`
The wire field becomes `name`, matching the runtime template `{$event.name}`, `EventSource.Name`, the stream envelope `name`, `eventBus.fire(name,…)`, and `x-mock-match: {'{$event.name}': …}`. The success response carries the same `name` (formerly `AsyncActionResponse.event`).
- **Alternative considered**: rename to `type` — rejected: collides with the removed discriminator and misdescribes the field.
- **Alternative considered**: keep `event` — rejected: the on-wire DTO is the only place off-vocabulary.

### D3: `POST /_mock/async/push` → `POST /_mock/async/messages`
"Deliver a message to channel consumers" becomes a POST of a **message** resource — the same "POST an entity, the server routes it" reading as `events`/`examples`. Body (`channel`, `connectionId`, `payload`, `delay`) and behavior are unchanged; only the path and internal handler naming change.
- **Alternative considered**: keep the verb path as accepted RPC — rejected: leaves the noun/verb split as an undocumented convention.
- **Alternative considered**: `POST /_mock/async/channels/{address}/messages` — rejected for this change: channel-in-path is a larger surface rework; the flat `/async/messages` matches the flat `/events` and `/examples` style.

### D4: `POST /_mock/async/disconnect` → `DELETE /_mock/async/consumers/{connectionId}` with query parameters
`connectionId` moves from the body to the path (symmetry with `DELETE /_mock/examples/{exampleId}`). Optional control data becomes query parameters — `code` (int WS close code, default 1000), `reason` (string), `abrupt` (bool, default `false`) — so the request has no body at all.
- **Alternative considered**: `POST /_mock/async/consumers/{connectionId}/disconnect` action-subresource — rejected: adds a second path shape and splits disconnect from the consumer resource a DELETE already expresses.
- **Alternative considered**: `DELETE` with a JSON body for `code`/`reason`/`abrupt` — rejected: DELETE bodies are inconsistently supported by clients/intermediaries; query parameters are the canonical DELETE channel for optional control data.
- **Alternative considered**: keep the verb path — rejected (see D3).

### D5: Envelope discriminators and stream filters stay
`manageEnvelope.Type` (event|push|consumer|schedule), the lifecycle `Action` fields, and the `?events=`/`?channels=` stream filters are unchanged. These are either tagged-union discriminators on a unary notification stream (their legitimate use) or established query semantics (multi-glob filters) where renaming adds breaking surface for no contract gain.

## Risks / Trade-offs

- [Management-API clients break (three request shapes)] → Direct cut-over follows the repo's 2026-09-07 precedent; each change is loud (404 on the old path, 400/field error on the new) and documented in `CHANGELOG.md`; this is a mock-tool control API, not a stable public contract.
- [Renames ripple across many test bodies] → Predominantly mechanical string substitutions; the spec-sync tests (`control_api_spec_sync_test.go`) re-derive the surface and fail loudly on any route/field drift.
- [DELETE-with-query-params is unusual-looking] → It is the widest-compatible way to carry optional control data on DELETE; the OpenAPI doc and README spell out `code`/`reason`/`abrupt`.

## Migration Plan

1. W1 (`/events`): update `fireEventRequest` (drop `Type`, rename `Event`→`Name`), error/propagation, response, `api/openapi.yaml`, specs, tests, docs.
2. W2 (`/async`): rename `push`→`messages` route+handler+DTO, rework `disconnect` into `DELETE /async/consumers/{connectionId}` reading path+query, `api/openapi.yaml`, specs, tests, docs.
3. Keep the whole change single-delivery so the resource model lands coherently rather than endpoint-by-endpoint.
4. Gate on `make test-unit`, `make lint`, and spec-sync tests.
5. Rollback: revert the change commit; no alias back-compat is retained.

## Open Questions

None. The schema guards (oneOf/required), envelope kinds, and stream filters are unchanged; any deferred concern (e.g. future message sub-resources) does not alter this change's specs, approach, or task breakdown.