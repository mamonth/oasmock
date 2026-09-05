## Why

The runtime control plane for async mocking is fragmented and protocol-locked: management endpoints live under `/_mock/ws/*` even though they drive both `ws` and `http` AsyncAPI channels, the only way to make a message fire in response to an event is to edit the AsyncAPI spec (`x-send-events`), the recurring-schedule endpoint is a one-variant bound duplicate of the `cron` built-in, and the spec-side built-in triggers (`cron`/`connect`/`receive`) are parsed but never actually fired. There is also no reactive channel for tests running mock clients — they can only poll HTTP.

Compounding this, example selection is fragmented across two vocabulary systems: `x-mock-match` selects an example against the HTTP/message context, while `x-send-events on:` is effectively a coarse `{$event.name}` equality on top of the same matcher (`x-send-events` duplicates `x-mock-match`). Unifying them into match-driven selection with an event context makes sync and async examples behave identically and lets payload- and per-connection conditions drive emission.

This change makes the async control surface protocol-neutral and complete: one unified `/_mock/examples` endpoint handles sync and async message injection, example selection uses one `x-mock-match` extension across request/event/connection contexts, recurrence is a timing extension instead of a parallel endpoint, and a general management WebSocket stream lets tests subscribe to runtime events.

## What Changes

- **Protocol-neutral async prefix**: `/_mock/ws/*` → `/_mock/async/*` for `push`, `consumers`, `disconnect` (these operate on both `ws` and `http` AsyncAPI channels). Old `/_mock/ws/*` paths stay as deprecated aliases.
- **Unified example selection (`x-mock-match`)**: the existing matcher is extended to select *and* target async examples. An event context exposes `{$event.name}` (event identity: named-event name or built-in `connect`/`receive`), `{$event.data}` (whole payload) alongside the unchanged `{$event.*}` payload fields, and `{$event.*}`-based conditions mark an example as event-driven. Conditions referencing `{$connection.*}` are a **per-connection recipient filter** evaluated in a two-phase partition (common conditions once per fire, connection conditions per candidate; absent connection conditions → broadcast as today).
- **Timing siblings**: recurrence and delay leave `x-mock-match` pure — new `x-mock-interval` (ms, periodic emission cadence) and `x-mock-delay` (ms, delayed emission after a fire). `cron` is **no longer an event**: periodic emission is expressed with `x-mock-interval`.
- **`x-send-events` deprecated**: a loader-time mapping shim translates `{on, wait}` into the match-identity + timing equivalent with a verbose-mode warning; removal deferred one release.
- **Unified example injection (`POST /_mock/examples`)**: `AddExampleRequest` gains `match` (runtime `x-mock-match`), `interval`, and `delay` for AsyncAPI targets — the runtime mirror of the extensions — in place of a dedicated `sendEvents` field. Strict, context-aware validation rejects wrong combinations (`path` vs `channel`, `match`/`interval` on a non-AsyncAPI target, `interval` alongside an event `match`, non-positive `interval`).
- **Example removal (`DELETE /_mock/examples/{exampleId}`)**: removes a dynamic example and cancels its recurring delivery (replaces the old schedule-stop).
- **Single event resource (`POST /_mock/events`)**: replaces the ambiguous `/_mock/events/fire` action path with a `type` discriminator (`"fire"` for now, extensible later); `/events/fire` stays as a deprecated alias.
- **Consumers get-all**: `GET /_mock/async/consumers` no longer requires `channel`; omitting it returns consumers across all channels.
- **Built-in trigger wiring**: the remaining built-ins `connect` and `receive` (today parsed but inert) are actually fired by the server from WebSocket/SignalR lifecycle and inbound-message hooks; periodic emission is driven by the generalized scheduler from `x-mock-interval`. The recurring schedule endpoint (`/_mock/ws/schedule*`) is **BREAKING**: removed, `410 Gone` pointing to `POST /_mock/examples` with `match`/`interval`.
- **Management WebSocket stream (`GET /_mock/stream`, `ws://host/_mock/stream`)**: connect-time event/channel filters; server pushes envelopes for fired events, message pushes, consumer connection lifecycle, and schedule start/stop. V1 is notifications-only.
- **Per-delivery templating**: delivered/scheduled messages (spec or runtime) are templated at emission time, so `{$event.*}`/`{$state.*}`/`{$env.*}` resolve against current state.

## Capabilities

### New Capabilities
<!-- none — behavior lands inside the four modified capabilities below. -->

### Modified Capabilities
- `extensions`: `x-mock-match` gains an event context (`{$event.name}`, `{$event.data}`, payload), per-connection recipient matching (`{$connection.*}`, two-phase partition), and timing sibling extensions `x-mock-interval`/`x-mock-delay`; event-driven examples are classified by `{$event.*}` match presence.
- `event-driver`: AsyncAPI message examples emit through `x-mock-match` against the event context (identity `{$event.name}`, built-ins `connect`/`receive`, periodic `x-mock-interval`); `x-send-events` is deprecated with a mapping shim; built-ins are actually fired.
- `management-api`: `POST /_mock/examples` gains `match`/`interval`/`delay` runtime async examples, `DELETE /_mock/examples/{exampleId}`, strict single-trigger field validation, and the fire-event endpoint becomes `POST /_mock/events` with a `type` discriminator.
- `asyncapi-management`: consumers can be listed without a channel filter; recurring delivery is expressed via `interval` on `/_mock/examples` (schedule endpoint removed); a general management WebSocket stream (`/_mock/stream`) exposes runtime event/consumer/schedule notifications.

## Impact

- `internal/extensions/` — `match.go`/`example_value.go`: event-context conditions, per-connection partition helper, `{$event.name}`/`{$event.data}` exposure contract; `extract.go`: deprecation-read path for `x-send-events`.
- `internal/runtime/` — `EventSource` gains identity (`name`) and whole-payload (`data`) access; a `ConnectionSource` (`{$connection.*}`) for per-connection matching.
- `internal/loader/` + `internal/server/event_server.go` — trigger classification at load (event-driven via `{$event.*}` match vs `x-mock-interval` vs reply), `x-send-events` mapping shim, drop of the subscription-key grouping (`groupSubscribedExamples`/`messageDeliverable`).
- `internal/server/engine.go` — fire-time selection runs the shared `SelectAsyncExample` pipeline against the event context; two-phase recipient partition at delivery.
- `internal/server/server.go` — management route registration (renames, removed schedule, new `/events`, `/stream`, `/examples/{exampleId}`), build-time wiring.
- `internal/server/server_management.go` — unified `AddExampleRequest` (`match`/`interval`/`delay`) + context-aware single-trigger validation; runtime registration path; example removal.
- `internal/server/scheduler.go` — generalize `pushScheduler` to per-example `x-mock-interval` jobs with per-delivery templating; emits schedule start/stop notifications.
- `internal/server/ws_adapter.go` / `internal/server/signalr_hub.go` — connect/receive trigger hooks; consumer lifecycle notifications; connection metadata capture at upgrade (query/headers) for `{$connection.*}`.
- `internal/server/manage_ws.go` (new) — `/_mock/stream` upgrade handler, filters, envelope encoder.
- `internal/server/fire_event.go` → `handleEvents` with `type` discriminator.
- `api/openapi.yaml` — `/async/*` paths, `/events` (`type`), `/examples` `match`/`interval`/`delay` + `DELETE`, `/stream` contract + envelope schemas, deprecated aliases + `x-send-events` note, `410` on schedule, `consumers.channel` optional.
- Tests — extensions pipeline unit tests (event context, partition, timing), trigger classification, runtime registration, built-in firing, scheduler intervals, consumers get-all, stream filters/envelopes; integration under `test/asyncapi/management-api/`.
- Docs — `docs/extensions.md` (event context + timing + `{$connection.*}` sections), `docs/architecture.md` async-management rewrite, `CHANGELOG.md`.